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
        private const val NOTIFICATION_ID = 1

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

    // Clipboard listener for background monitoring
    private var clipboardManager: ClipboardManager? = null
    private val clipChangedListener = ClipboardManager.OnPrimaryClipChangedListener {
        handleClipboardChange()
    }

    @Volatile
    var isConnected: Boolean = false
        private set

    // Callbacks for MainActivity to observe state changes
    var onStatusChanged: ((Boolean) -> Unit)? = null
    var onForceDisconnected: ((String) -> Unit)? = null
    var onDeviceRenamed: ((String) -> Unit)? = null

    override fun onCreate() {
        super.onCreate()
        instance = this
        prefs = PrefsManager(this)
        clipboardHelper = ClipboardHelper(this)
        createNotificationChannel()
        startForeground(NOTIFICATION_ID, buildNotification("正在连接..."))
        Log.d(TAG, "Service created")

        // Register clipboard change listener on main thread
        clipboardManager = getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
        clipboardManager?.addPrimaryClipChangedListener(clipChangedListener)
        Log.d(TAG, "Clipboard listener registered")

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
        // Unregister clipboard listener
        clipboardManager?.removePrimaryClipChangedListener(clipChangedListener)
        clipboardManager = null
        Log.d(TAG, "Clipboard listener unregistered")

        wsClient?.stop()
        wsClient = null
        scope.cancel()
        Log.d(TAG, "Service destroyed")
        super.onDestroy()
    }

    /**
     * Called when system clipboard changes (via OnPrimaryClipChangedListener).
     * This fires reliably even when the app is in the background.
     */
    private fun handleClipboardChange() {
        try {
            val content = clipboardHelper.readClipboard()
            if (content.isNullOrBlank()) return

            if (clipboardHelper.shouldUpload(content)) {
                Log.d(TAG, "Clipboard changed in background (${content.length} chars), uploading...")
                onClipboardChanged(content)
            }
        } catch (e: Exception) {
            Log.e(TAG, "Error handling clipboard change: ${e.message}")
        }
    }

    private fun startWebSocket(config: AppConfig) {
        wsClient?.stop()
        wsClient = WsClient(
            serverUrl = config.serverUrl,
            token = config.token,
            deviceName = config.deviceName.ifBlank { android.os.Build.MODEL },
            onClip = { content ->
                clipboardHelper.writeClipboard(content)
            },
            onStatusChange = { connected ->
                isConnected = connected
                val statusText = if (connected) "已连接" else "已断开，重连中..."
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
            }
        )
        wsClient?.start()
    }

    /**
     * Upload clipboard content to server.
     * Called by clipboard listener or accessibility service.
     */
    fun onClipboardChanged(content: String) {
        if (!clipboardHelper.shouldUpload(content)) return

        scope.launch {
            val config = prefs.configFlow.first()
            if (!config.isLoggedIn) return@launch

            val deviceName = config.deviceName.ifBlank { android.os.Build.MODEL }
            val result = ApiClient.pushClipboard(config.serverUrl, config.token, content, deviceName)
            if (result.isSuccess) {
                Log.d(TAG, "Clipboard uploaded (${content.length} chars)")
            } else {
                Log.e(TAG, "Clipboard upload failed: ${result.exceptionOrNull()?.message}")
            }
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

    private fun createNotificationChannel() {
        val channel = NotificationChannel(
            CHANNEL_ID,
            getString(R.string.notification_channel_name),
            NotificationManager.IMPORTANCE_LOW
        ).apply {
            description = getString(R.string.notification_channel_desc)
        }
        val nm = getSystemService(NotificationManager::class.java)
        nm.createNotificationChannel(channel)
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
}
