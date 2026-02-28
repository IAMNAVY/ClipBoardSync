package com.clipsync.android.service

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.content.ClipboardManager
import android.content.Context
import android.content.Intent
import android.os.Handler
import android.os.IBinder
import android.os.Looper
import android.util.Log
import com.clipsync.android.MainActivity
import com.clipsync.android.R
import com.clipsync.android.clipboard.ClipboardHelper
import com.clipsync.android.clipboard.LogcatClipboardMonitor
import com.clipsync.android.clipboard.OverlayClipboardReader
import com.clipsync.android.data.AppConfig
import com.clipsync.android.data.PrefsManager
import com.clipsync.android.network.ApiClient
import com.clipsync.android.network.WsClient
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch

class ClipSyncService : Service() {

    companion object {
        private const val TAG = "ClipSyncService"
        private const val CHANNEL_ID = "clipsync_service"
        private const val SYNC_CHANNEL_ID = "clipsync_sync"
        private const val NOTIFICATION_ID = 1
        private const val SYNC_NOTIFICATION_ID = 2

        @Volatile
        var instance: ClipSyncService? = null

        fun start(context: Context) {
            val intent = Intent(context, ClipSyncService::class.java)
            context.startForegroundService(intent)
        }

        fun stop(context: Context) {
            val intent = Intent(context, ClipSyncService::class.java)
            context.stopService(intent)
        }
    }

    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Main)
    private lateinit var prefs: PrefsManager
    lateinit var clipboardHelper: ClipboardHelper
        private set
    private var wsClient: WsClient? = null
    private var currentConfig: AppConfig = AppConfig()
    private val mainHandler = Handler(Looper.getMainLooper())

    // Standard clipboard listener (works when app is foreground)
    private var clipboardManager: ClipboardManager? = null
    private val clipChangedListener = ClipboardManager.OnPrimaryClipChangedListener {
        handleClipboardChange()
    }

    // Logcat-based clipboard monitor (works in background with ADB permissions)
    private var logcatMonitor: LogcatClipboardMonitor? = null

    // Overlay-based clipboard reader (non-intrusive, no Activity needed)
    private var overlayReader: OverlayClipboardReader? = null

    @Volatile
    var isConnected: Boolean = false
        private set

    @Volatile
    var isLogcatMonitorActive: Boolean = false
        private set

    @Volatile
    var lastErrorMessage: String = ""
        private set

    // Callbacks for MainActivity to observe state changes
    var onStatusChanged: ((Boolean) -> Unit)? = null
    var onForceDisconnected: ((String) -> Unit)? = null
    var onDeviceRenamed: ((String) -> Unit)? = null
    var onErrorMessageChanged: ((String) -> Unit)? = null

    override fun onCreate() {
        super.onCreate()
        instance = this
        prefs = PrefsManager(this)
        clipboardHelper = ClipboardHelper(this)
        overlayReader = OverlayClipboardReader(this)
        createNotificationChannel()
        startForeground(NOTIFICATION_ID, buildNotification("正在连接..."))
        Log.d(TAG, "Service created")

        // Register standard clipboard listener (foreground only)
        clipboardManager = getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
        clipboardManager?.addPrimaryClipChangedListener(clipChangedListener)
        Log.d(TAG, "Standard clipboard listener registered")

        // Try to start logcat monitor if ADB permissions are granted
        startLogcatMonitorIfAvailable()

        scope.launch {
            val config = prefs.configFlow.first()
            currentConfig = config
            if (config.isLoggedIn) {
                startWebSocket(config)
            }
        }
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        return START_STICKY
    }

    override fun onBind(intent: Intent?): IBinder? = null

    override fun onDestroy() {
        instance = null
        clipboardManager?.removePrimaryClipChangedListener(clipChangedListener)
        clipboardManager = null
        logcatMonitor?.stop()
        logcatMonitor = null
        isLogcatMonitorActive = false
        Log.d(TAG, "Clipboard monitors stopped")

        wsClient?.stop()
        wsClient = null
        scope.cancel()
        Log.d(TAG, "Service destroyed")
        super.onDestroy()
    }

    /**
     * Start logcat-based clipboard monitor if READ_LOGS permission is available.
     * Can also restart a dead monitor.
     */
    fun startLogcatMonitorIfAvailable() {
        // If already running and active, skip
        if (logcatMonitor?.isActive() == true) return

        // Stop any existing dead monitor
        logcatMonitor?.stop()
        logcatMonitor = null

        if (LogcatClipboardMonitor.hasReadLogsPermission(this)) {
            if (!LogcatClipboardMonitor.hasOverlayPermission(this)) {
                Log.w(TAG, "READ_LOGS granted but overlay permission missing, logcat monitor may not work fully")
            }
            Log.d(TAG, "READ_LOGS permission available, starting logcat monitor")
            logcatMonitor = LogcatClipboardMonitor(this)
            logcatMonitor?.start()
            isLogcatMonitorActive = true
            updateNotification(if (isConnected) "已连接（后台监控已激活）" else "已断开，重连中...")
        } else {
            Log.d(TAG, "READ_LOGS permission not available, logcat monitor disabled")
            isLogcatMonitorActive = false
        }
    }

    /** Debounce for overlay clipboard reading */
    @Volatile
    private var lastOverlayReadTime: Long = 0
    private val overlayReadDebounce: Long = 2000 // ms

    /**
     * Called when system clipboard changes via standard listener.
     * This fires even when the app is in background, but reading
     * clipboard will fail (returns null) on Android 10+.
     * When that happens, we use OverlayClipboardReader to create
     * a tiny invisible overlay window to steal focus and read.
     */
    private fun handleClipboardChange() {
        try {
            // Check if upload is enabled
            if (!currentConfig.shouldUpload) return

            val content = clipboardHelper.readClipboard()
            if (content != null && content.isNotBlank()) {
                // Foreground: successfully read clipboard
                if (clipboardHelper.shouldUpload(content)) {
                    Log.d(TAG, "Standard listener detected change (${content.length} chars)")
                    onClipboardChanged(content)
                }
            } else {
                // Background: clipboard read was denied
                // Use overlay window to steal focus and read (non-intrusive)
                val now = System.currentTimeMillis()
                if (now - lastOverlayReadTime > overlayReadDebounce) {
                    lastOverlayReadTime = now
                    Log.d(TAG, "Clipboard read denied (background), using overlay reader")
                    mainHandler.post {
                        overlayReader?.tryRead { overlayContent ->
                            if (overlayContent != null && overlayContent.isNotBlank()) {
                                if (clipboardHelper.shouldUpload(overlayContent)) {
                                    Log.d(TAG, "Overlay read success (${overlayContent.length} chars)")
                                    onClipboardChanged(overlayContent)
                                }
                            } else {
                                Log.w(TAG, "Overlay read returned null")
                            }
                        }
                    }
                }
            }
        } catch (e: Exception) {
            Log.e(TAG, "Error handling clipboard change: ${e.message}")
        }
    }

    /**
     * Public method for LogcatClipboardMonitor to request an overlay clipboard read.
     * Must be called on the main thread.
     */
    fun readClipboardViaOverlay() {
        if (!currentConfig.shouldUpload) return

        val now = System.currentTimeMillis()
        if (now - lastOverlayReadTime < overlayReadDebounce) return
        lastOverlayReadTime = now

        overlayReader?.tryRead { content ->
            if (content != null && content.isNotBlank()) {
                if (clipboardHelper.shouldUpload(content)) {
                    Log.d(TAG, "Overlay read (logcat trigger) success (${content.length} chars)")
                    onClipboardChanged(content)
                }
            }
        }
    }

    private fun startWebSocket(config: AppConfig) {
        wsClient?.stop()
        wsClient = WsClient(
            serverUrl = config.serverUrl,
            token = config.token,
            deviceName = config.deviceName.ifBlank { android.os.Build.MODEL },
            onClip = { content ->
                if (currentConfig.shouldDownload) {
                    clipboardHelper.writeClipboard(content)
                    // Show sync notification with auto-dismiss
                    showSyncNotification(content)
                } else {
                    Log.d(TAG, "Download disabled, ignoring remote clipboard")
                }
            },
            onStatusChange = { connected ->
                isConnected = connected
                if (connected) lastErrorMessage = ""
                val suffix = if (isLogcatMonitorActive) "（后台监控已激活）" else ""
                val statusText = if (connected) "已连接$suffix" else "已断开，重连中..."
                updateNotification(statusText)
                onStatusChanged?.invoke(connected)
            },
            onDeviceRenamed = { newName ->
                Log.d(TAG, "Device renamed to: $newName")
                scope.launch {
                    prefs.updateDeviceName(newName)
                }
                onDeviceRenamed?.invoke(newName)
            },
            onForceDisconnect = { reason ->
                Log.w(TAG, "Force disconnected: $reason")
                scope.launch {
                    prefs.clearAuth()
                }
                updateNotification("已被强制下线")
                onForceDisconnected?.invoke(reason)
            },
            onErrorMessage = { error ->
                lastErrorMessage = error
                onErrorMessageChanged?.invoke(error)
            },
            onTokenExpired = {
                handleTokenRenewal()
            }
        )
        wsClient?.start()
    }

    /**
     * Upload clipboard content to server.
     * Caller must check shouldUpload() before calling this.
     */
    fun onClipboardChanged(content: String) {
        scope.launch {
            val config = prefs.configFlow.first()
            if (!config.isLoggedIn) return@launch
            if (!config.shouldUpload) {
                Log.d(TAG, "Upload disabled, skipping clipboard upload")
                return@launch
            }

            val deviceName = config.deviceName.ifBlank { android.os.Build.MODEL }
            val result = ApiClient.pushClipboard(config.serverUrl, config.token, content, deviceName)
            if (result.isSuccess) {
                clipboardHelper.markUploaded(content)
                Log.d(TAG, "Clipboard uploaded (${content.length} chars)")
            } else {
                val errMsg = result.exceptionOrNull()?.message ?: ""
                Log.e(TAG, "Clipboard upload failed: $errMsg")
                if (errMsg.contains("HTTP 401") || errMsg.contains("401 Unauthorized")) {
                    Log.d(TAG, "Detected 401 during upload, triggering token renewal...")
                    handleTokenRenewal()
                }
            }
        }
    }

    /**
     * Called when sync mode is changed from UI. Refreshes currentConfig.
     */
    fun refreshConfig() {
        scope.launch {
            currentConfig = prefs.configFlow.first()
            Log.d(TAG, "Config refreshed, syncMode: ${currentConfig.syncModeEnum}")
        }
    }

    fun reconnect() {
        scope.launch {
            val config = prefs.configFlow.first()
            currentConfig = config
            if (config.isLoggedIn) {
                startWebSocket(config)
            }
        }
    }

    /**
     * Attempts to renew the access token using the refresh token first,
     * falling back to password re-login if refresh fails.
     */
    private fun handleTokenRenewal() {
        scope.launch {
            val config = prefs.configFlow.first()
            val serverUrl = config.serverUrl
            val refreshTok = config.refreshToken
            val username = config.username
            val password = config.password

            if (serverUrl.isBlank()) {
                Log.w(TAG, "Cannot renew token: missing server URL")
                return@launch
            }

            // Strategy 1: Try refresh token (preferred — no password needed)
            if (refreshTok.isNotBlank()) {
                Log.d(TAG, "Token expired, attempting renewal via refresh token...")
                val refreshResult = ApiClient.refreshAccessToken(serverUrl, refreshTok)
                refreshResult.fold(
                    onSuccess = { resp ->
                        Log.d(TAG, "Refresh token renewal succeeded")
                        val newConfig = config.copy(
                            token = resp.token,
                            refreshToken = resp.refresh_token
                        )
                        prefs.saveConfig(newConfig)
                        currentConfig = newConfig
                        wsClient?.updateToken(resp.token)
                        return@launch
                    },
                    onFailure = { e ->
                        Log.w(TAG, "Refresh token renewal failed: ${e.message}, trying password...")
                    }
                )
            }

            // Strategy 2: Fall back to password re-login
            if (username.isBlank() || password.isBlank()) {
                Log.w(TAG, "Cannot renew token: missing saved credentials")
                return@launch
            }

            Log.d(TAG, "Attempting token renewal via password login...")
            val result = ApiClient.login(serverUrl, username, password)
            result.fold(
                onSuccess = { resp ->
                    Log.d(TAG, "Password login renewal succeeded")
                    val newConfig = config.copy(
                        token = resp.token,
                        refreshToken = resp.refresh_token
                    )
                    prefs.saveConfig(newConfig)
                    currentConfig = newConfig
                    wsClient?.updateToken(resp.token)
                },
                onFailure = { e ->
                    Log.e(TAG, "Token renewal failed: ${e.message}")
                }
            )
        }
    }

    /**
     * Updates the token in the running WsClient (used after external renewal).
     */
    fun updateToken(newToken: String) {
        wsClient?.updateToken(newToken)
    }

    private fun createNotificationChannel() {
        // Foreground service channel (low importance, no sound)
        val serviceChannel = NotificationChannel(
            CHANNEL_ID,
            getString(R.string.notification_channel_name),
            NotificationManager.IMPORTANCE_LOW
        ).apply {
            description = getString(R.string.notification_channel_desc)
        }

        // Sync notification channel (default importance, with sound)
        val syncChannel = NotificationChannel(
            SYNC_CHANNEL_ID,
            "剪贴板同步通知",
            NotificationManager.IMPORTANCE_DEFAULT
        ).apply {
            description = "收到远端剪贴板内容时的通知"
        }

        val nm = getSystemService(NotificationManager::class.java)
        nm.createNotificationChannel(serviceChannel)
        nm.createNotificationChannel(syncChannel)
    }

    private fun buildNotification(statusText: String): Notification {
        val intent = Intent(this, MainActivity::class.java).apply {
            flags = Intent.FLAG_ACTIVITY_SINGLE_TOP
        }
        val pendingIntent = PendingIntent.getActivity(
            this, 0, intent, PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT
        )

        return Notification.Builder(this, CHANNEL_ID)
            .setContentTitle("ClipSync")
            .setContentText(statusText)
            .setSmallIcon(R.drawable.ic_notification)
            .setContentIntent(pendingIntent)
            .setOngoing(true)
            .build()
    }

    private fun updateNotification(statusText: String) {
        val nm = getSystemService(NotificationManager::class.java)
        nm.notify(NOTIFICATION_ID, buildNotification(statusText))
    }

    /**
     * Shows a temporary sync notification that auto-dismisses after 3 seconds.
     */
    private fun showSyncNotification(content: String) {
        val preview = if (content.length > 30) content.substring(0, 30) + "..." else content
        val intent = Intent(this, MainActivity::class.java).apply {
            flags = Intent.FLAG_ACTIVITY_SINGLE_TOP
        }
        val pendingIntent = PendingIntent.getActivity(
            this, 1, intent, PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT
        )

        val notification = Notification.Builder(this, SYNC_CHANNEL_ID)
            .setContentTitle("📋 已接收")
            .setContentText(preview)
            .setSmallIcon(R.drawable.ic_notification)
            .setContentIntent(pendingIntent)
            .setAutoCancel(true)
            .build()

        val nm = getSystemService(NotificationManager::class.java)
        nm.notify(SYNC_NOTIFICATION_ID, notification)

        // Auto-dismiss after 3 seconds
        mainHandler.postDelayed({
            nm.cancel(SYNC_NOTIFICATION_ID)
        }, 3000)
    }
}
