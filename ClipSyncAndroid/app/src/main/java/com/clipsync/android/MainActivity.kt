package com.clipsync.android

import android.Manifest
import android.content.pm.PackageManager
import android.os.Build
import android.os.Bundle
import android.widget.Toast
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.core.content.ContextCompat
import com.clipsync.android.data.AppConfig
import com.clipsync.android.data.PrefsManager
import com.clipsync.android.data.SyncMode
import com.clipsync.android.service.ClipSyncService
import com.clipsync.android.ui.LoginScreen
import com.clipsync.android.ui.MainScreen
import com.clipsync.android.ui.theme.ClipSyncTheme
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch

class MainActivity : ComponentActivity() {

    private lateinit var prefs: PrefsManager
    private var isConnected by mutableStateOf(false)
    private var connectionError by mutableStateOf("")

    private val notificationPermissionLauncher =
        registerForActivityResult(ActivityResultContracts.RequestPermission()) { granted ->
            if (!granted) {
                Toast.makeText(this, "通知权限被拒绝，后台状态显示可能受限", Toast.LENGTH_SHORT).show()
            }
        }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        prefs = PrefsManager(this)

        // Request notification permission for Android 13+
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
            if (ContextCompat.checkSelfPermission(this, Manifest.permission.POST_NOTIFICATIONS)
                != PackageManager.PERMISSION_GRANTED
            ) {
                notificationPermissionLauncher.launch(Manifest.permission.POST_NOTIFICATIONS)
            }
        }

        enableEdgeToEdge()

        setContent {
            ClipSyncTheme {
                val config by prefs.configFlow.collectAsState(initial = AppConfig())

                if (config.isLoggedIn) {
                    MainScreen(
                        config = config,
                        isConnected = isConnected,
                        connectionError = connectionError,
                        onLogout = { handleLogout() },
                        onRenameDevice = { newName -> handleRenameDevice(newName) },
                        onSyncModeChanged = { mode -> handleSyncModeChanged(mode) },
                        onReconnect = { handleReconnect() }
                    )
                } else {
                    LoginScreen(
                        onLoginSuccess = { serverUrl, username, token ->
                            handleLoginSuccess(serverUrl, username, token)
                        }
                    )
                }
            }
        }
    }

    override fun onResume() {
        super.onResume()
        // Bind to service to observe status
        ClipSyncService.instance?.let { service ->
            isConnected = service.isConnected
            connectionError = service.lastErrorMessage
            service.onStatusChanged = { connected ->
                runOnUiThread {
                    isConnected = connected
                    if (connected) connectionError = ""
                }
            }
            service.onForceDisconnected = { reason ->
                runOnUiThread {
                    Toast.makeText(this, "已被强制下线: $reason", Toast.LENGTH_LONG).show()
                    isConnected = false
                    connectionError = "已被强制下线: $reason"
                }
            }
            service.onDeviceRenamed = { newName ->
                runOnUiThread {
                    Toast.makeText(this, "设备已被重命名为: $newName", Toast.LENGTH_SHORT).show()
                }
            }
            service.onErrorMessageChanged = { error ->
                runOnUiThread { connectionError = error }
            }
        }
    }

    override fun onPause() {
        super.onPause()
        ClipSyncService.instance?.let { service ->
            service.onStatusChanged = null
            service.onForceDisconnected = null
            service.onDeviceRenamed = null
            service.onErrorMessageChanged = null
        }
    }

    private fun handleLoginSuccess(serverUrl: String, username: String, token: String) {
        CoroutineScope(Dispatchers.IO).launch {
            prefs.saveConfig(
                AppConfig(
                    serverUrl = serverUrl,
                    username = username,
                    token = token,
                    deviceName = Build.MODEL
                )
            )
            // Start the foreground service
            ClipSyncService.start(this@MainActivity)

            // Wait a moment for service to initialize, then bind callbacks
            kotlinx.coroutines.delay(500)
            runOnUiThread {
                ClipSyncService.instance?.let { service ->
                    service.onStatusChanged = { connected ->
                        runOnUiThread {
                            isConnected = connected
                            if (connected) connectionError = ""
                        }
                    }
                    service.onForceDisconnected = { reason ->
                        runOnUiThread {
                            Toast.makeText(this@MainActivity, "已被强制下线: $reason", Toast.LENGTH_LONG).show()
                            isConnected = false
                            connectionError = "已被强制下线: $reason"
                        }
                    }
                    service.onDeviceRenamed = { newName ->
                        runOnUiThread {
                            Toast.makeText(this@MainActivity, "设备已被重命名为: $newName", Toast.LENGTH_SHORT).show()
                        }
                    }
                    service.onErrorMessageChanged = { error ->
                        runOnUiThread { connectionError = error }
                    }
                }
            }
        }
    }

    private fun handleLogout() {
        ClipSyncService.stop(this)
        CoroutineScope(Dispatchers.IO).launch {
            prefs.clearAuth()
        }
        isConnected = false
        connectionError = ""
    }

    private fun handleReconnect() {
        connectionError = ""
        val service = ClipSyncService.instance
        if (service != null) {
            service.reconnect()
            Toast.makeText(this, "正在重新连接...", Toast.LENGTH_SHORT).show()
        } else {
            // Service not running, restart it
            ClipSyncService.start(this)
            Toast.makeText(this, "正在重启服务...", Toast.LENGTH_SHORT).show()
        }
    }

    private fun handleRenameDevice(newName: String) {
        CoroutineScope(Dispatchers.IO).launch {
            prefs.updateDeviceName(newName)
        }
        // Update WsClient's device name; reconnect to apply
        ClipSyncService.instance?.let { service ->
            service.reconnect()
        }
        Toast.makeText(this, "设备名已更新为: $newName（重连生效）", Toast.LENGTH_SHORT).show()
    }

    private fun handleSyncModeChanged(mode: SyncMode) {
        CoroutineScope(Dispatchers.IO).launch {
            prefs.updateSyncMode(mode)
            // Refresh service config AFTER save completes
            ClipSyncService.instance?.refreshConfig()
        }
        val label = when (mode) {
            SyncMode.BIDIRECTIONAL -> "手机 ↔ 云端"
            SyncMode.UPLOAD_ONLY -> "手机 → 云端"
            SyncMode.DOWNLOAD_ONLY -> "云端 → 手机"
            SyncMode.OFF -> "已关闭"
        }
        Toast.makeText(this, "同步方向: $label", Toast.LENGTH_SHORT).show()
    }
}
