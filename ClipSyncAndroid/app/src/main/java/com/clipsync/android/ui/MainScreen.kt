package com.clipsync.android.ui

import android.content.Context
import android.content.Intent
import android.net.Uri
import android.os.PowerManager
import android.provider.Settings
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Accessibility
import androidx.compose.material.icons.filled.BatteryChargingFull
import androidx.compose.material.icons.filled.CheckCircle
import androidx.compose.material.icons.filled.Cloud
import androidx.compose.material.icons.filled.CloudOff
import androidx.compose.material.icons.filled.ContentPaste
import androidx.compose.material.icons.filled.Edit
import androidx.compose.material.icons.filled.Logout
import androidx.compose.material.icons.filled.PhoneAndroid
import androidx.compose.material.icons.filled.Warning
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.clipsync.android.data.AppConfig
import com.clipsync.android.service.ClipAccessibilityService

@Composable
fun MainScreen(
    config: AppConfig,
    isConnected: Boolean,
    onLogout: () -> Unit,
    onRenameDevice: (String) -> Unit
) {
    val context = LocalContext.current
    var showRenameDialog by remember { mutableStateOf(false) }
    var newDeviceName by remember { mutableStateOf(config.deviceName) }

    val isAccessibilityEnabled = ClipAccessibilityService.isRunning
    val isBatteryOptimized = !isIgnoringBatteryOptimization(context)

    Column(
        modifier = Modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(24.dp),
        verticalArrangement = Arrangement.spacedBy(16.dp)
    ) {
        // Header
        Row(verticalAlignment = Alignment.CenterVertically) {
            Icon(
                Icons.Default.ContentPaste,
                contentDescription = null,
                tint = MaterialTheme.colorScheme.primary,
                modifier = Modifier.size(32.dp)
            )
            Spacer(modifier = Modifier.width(12.dp))
            Text("ClipSync", fontSize = 24.sp, fontWeight = FontWeight.Bold)
        }

        // Connection Status Card
        StatusCard(
            icon = if (isConnected) Icons.Default.Cloud else Icons.Default.CloudOff,
            title = if (isConnected) "已连接" else "未连接",
            subtitle = config.serverUrl,
            isGood = isConnected
        )

        // Account Card
        Card(
            modifier = Modifier.fillMaxWidth(),
            colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface)
        ) {
            Column(modifier = Modifier.padding(16.dp)) {
                Text("账号信息", fontWeight = FontWeight.SemiBold, fontSize = 16.sp)
                Spacer(modifier = Modifier.height(8.dp))
                InfoRow("用户名", config.username)

                Row(verticalAlignment = Alignment.CenterVertically) {
                    Column(modifier = Modifier.weight(1f)) {
                        Text("设备名称", fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurface.copy(0.5f))
                        Text(config.deviceName.ifBlank { android.os.Build.MODEL }, fontSize = 14.sp)
                    }
                    IconButton(onClick = {
                        newDeviceName = config.deviceName.ifBlank { android.os.Build.MODEL }
                        showRenameDialog = true
                    }) {
                        Icon(Icons.Default.Edit, contentDescription = "重命名", modifier = Modifier.size(18.dp))
                    }
                }
            }
        }

        // Permissions & Setup
        Text("权限与设置", fontWeight = FontWeight.SemiBold, fontSize = 16.sp)

        // Accessibility
        SetupItem(
            icon = Icons.Default.Accessibility,
            title = "无障碍服务",
            description = if (isAccessibilityEnabled) "已开启 — 剪贴板监控运行中" else "未开启 — 需要此权限来监控剪贴板",
            isReady = isAccessibilityEnabled,
            actionLabel = if (isAccessibilityEnabled) "已开启" else "去开启",
            onAction = if (isAccessibilityEnabled) null else ({
                val intent = Intent(Settings.ACTION_ACCESSIBILITY_SETTINGS)
                context.startActivity(intent)
            })
        )

        // Battery optimization
        SetupItem(
            icon = Icons.Default.BatteryChargingFull,
            title = "忽略电池优化",
            description = if (!isBatteryOptimized) "已忽略 — 后台同步不受限" else "未设置 — 系统可能会限制后台同步",
            isReady = !isBatteryOptimized,
            actionLabel = if (!isBatteryOptimized) "已设置" else "去设置",
            onAction = if (!isBatteryOptimized) null else ({
                val intent = Intent(Settings.ACTION_REQUEST_IGNORE_BATTERY_OPTIMIZATIONS).apply {
                    data = Uri.parse("package:${context.packageName}")
                }
                context.startActivity(intent)
            })
        )

        // Autostart hint
        SetupItem(
            icon = Icons.Default.PhoneAndroid,
            title = "自启动管理",
            description = "部分手机需要手动允许自启动（华为/小米/OPPO 等）\n设置 → 应用管理 → ClipSync → 自启动",
            isReady = true,
            actionLabel = "打开设置",
            onAction = {
                val intent = Intent(Settings.ACTION_APPLICATION_DETAILS_SETTINGS).apply {
                    data = Uri.parse("package:${context.packageName}")
                }
                context.startActivity(intent)
            }
        )

        // ADB-based background monitoring section
        val isLogcatActive = com.clipsync.android.service.ClipSyncService.instance?.isLogcatMonitorActive == true
        val hasOverlay = com.clipsync.android.clipboard.LogcatClipboardMonitor.hasOverlayPermission(context)

        Spacer(modifier = Modifier.height(4.dp))
        Text("后台剪贴板监控（ADB 激活）", fontWeight = FontWeight.SemiBold, fontSize = 16.sp)
        Text(
            "Android 10+ 限制后台读取剪贴板，通过 ADB 授权可解除此限制",
            fontSize = 12.sp,
            color = MaterialTheme.colorScheme.onSurface.copy(0.5f)
        )

        // Status card for ADB monitoring
        StatusCard(
            icon = if (isLogcatActive) Icons.Default.CheckCircle else Icons.Default.Warning,
            title = if (isLogcatActive) "后台监控已激活" else "后台监控未激活",
            subtitle = if (isLogcatActive) "通过 Logcat 监控剪贴板变更" else "需要通过 ADB 授权来启用",
            isGood = isLogcatActive
        )

        // ADB commands card — always visible
        Card(
            modifier = Modifier.fillMaxWidth(),
            colors = CardDefaults.cardColors(
                containerColor = MaterialTheme.colorScheme.surfaceVariant
            )
        ) {
            Column(modifier = Modifier.padding(16.dp)) {
                Text("在电脑上执行以下 ADB 命令：", fontWeight = FontWeight.SemiBold, fontSize = 13.sp)
                Spacer(modifier = Modifier.height(8.dp))

                AdbCommandRow("1. 授权日志读取权限：",
                    "adb -d shell pm grant com.clipsync.android android.permission.READ_LOGS")
                Spacer(modifier = Modifier.height(6.dp))

                AdbCommandRow("2. 允许悬浮窗权限：",
                    "adb -d shell appops set com.clipsync.android SYSTEM_ALERT_WINDOW allow")
                Spacer(modifier = Modifier.height(6.dp))

                AdbCommandRow("3. 重启应用使权限生效：",
                    "adb -d shell am force-stop com.clipsync.android")
            }
        }

        // Overlay permission status
        SetupItem(
            icon = Icons.Default.PhoneAndroid,
            title = "悬浮窗权限",
            description = if (hasOverlay) "已授权 — 后台可读取剪贴板" else "未授权 — 可通过 ADB 授权或手动开启",
            isReady = hasOverlay,
            actionLabel = if (hasOverlay) "已开启" else "去开启",
            onAction = if (hasOverlay) null else ({
                val intent = Intent(Settings.ACTION_MANAGE_OVERLAY_PERMISSION).apply {
                    data = Uri.parse("package:${context.packageName}")
                }
                context.startActivity(intent)
            })
        )

        Spacer(modifier = Modifier.height(8.dp))

        // Logout button
        OutlinedButton(
            onClick = onLogout,
            modifier = Modifier.fillMaxWidth(),
            colors = ButtonDefaults.outlinedButtonColors(
                contentColor = MaterialTheme.colorScheme.error
            )
        ) {
            Icon(Icons.Default.Logout, contentDescription = null, modifier = Modifier.size(18.dp))
            Spacer(modifier = Modifier.width(8.dp))
            Text("退出登录")
        }

        Spacer(modifier = Modifier.height(32.dp))
    }

    // Rename Dialog
    if (showRenameDialog) {
        AlertDialog(
            onDismissRequest = { showRenameDialog = false },
            title = { Text("修改设备名称") },
            text = {
                OutlinedTextField(
                    value = newDeviceName,
                    onValueChange = { newDeviceName = it },
                    label = { Text("设备名称") },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth()
                )
            },
            confirmButton = {
                TextButton(onClick = {
                    if (newDeviceName.isNotBlank()) {
                        onRenameDevice(newDeviceName.trim())
                        showRenameDialog = false
                    }
                }) { Text("确认") }
            },
            dismissButton = {
                TextButton(onClick = { showRenameDialog = false }) { Text("取消") }
            }
        )
    }
}

@Composable
private fun StatusCard(icon: ImageVector, title: String, subtitle: String, isGood: Boolean) {
    val color = if (isGood) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.error
    Card(
        modifier = Modifier.fillMaxWidth(),
        colors = CardDefaults.cardColors(containerColor = color.copy(alpha = 0.1f))
    ) {
        Row(
            modifier = Modifier.padding(16.dp),
            verticalAlignment = Alignment.CenterVertically
        ) {
            Icon(icon, contentDescription = null, tint = color, modifier = Modifier.size(32.dp))
            Spacer(modifier = Modifier.width(12.dp))
            Column {
                Text(title, fontWeight = FontWeight.SemiBold, color = color, fontSize = 16.sp)
                Text(subtitle, fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurface.copy(0.6f))
            }
        }
    }
}

@Composable
private fun InfoRow(label: String, value: String) {
    Column(modifier = Modifier.padding(vertical = 4.dp)) {
        Text(label, fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurface.copy(0.5f))
        Text(value, fontSize = 14.sp)
    }
}

@Composable
private fun SetupItem(
    icon: ImageVector,
    title: String,
    description: String,
    isReady: Boolean,
    actionLabel: String,
    onAction: (() -> Unit)?
) {
    Card(
        modifier = Modifier.fillMaxWidth(),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface)
    ) {
        Row(
            modifier = Modifier.padding(16.dp),
            verticalAlignment = Alignment.Top
        ) {
            Icon(
                if (isReady) Icons.Default.CheckCircle else Icons.Default.Warning,
                contentDescription = null,
                tint = if (isReady) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.error,
                modifier = Modifier.size(24.dp)
            )
            Spacer(modifier = Modifier.width(12.dp))
            Column(modifier = Modifier.weight(1f)) {
                Text(title, fontWeight = FontWeight.SemiBold, fontSize = 14.sp)
                Spacer(modifier = Modifier.height(2.dp))
                Text(description, fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurface.copy(0.6f))
            }
            if (onAction != null) {
                Spacer(modifier = Modifier.width(8.dp))
                Button(onClick = onAction, modifier = Modifier.height(32.dp)) {
                    Text(actionLabel, fontSize = 12.sp)
                }
            }
        }
    }
}

@Composable
private fun AdbCommandRow(label: String, command: String) {
    Column {
        Text(label, fontSize = 12.sp, fontWeight = FontWeight.Medium)
        Card(
            modifier = Modifier.fillMaxWidth(),
            colors = CardDefaults.cardColors(
                containerColor = MaterialTheme.colorScheme.background
            )
        ) {
            Text(
                text = command,
                modifier = Modifier.padding(8.dp),
                fontSize = 11.sp,
                fontFamily = androidx.compose.ui.text.font.FontFamily.Monospace,
                color = MaterialTheme.colorScheme.primary
            )
        }
    }
}

private fun isIgnoringBatteryOptimization(context: Context): Boolean {
    val pm = context.getSystemService(Context.POWER_SERVICE) as PowerManager
    return pm.isIgnoringBatteryOptimizations(context.packageName)
}
