package com.clipsync.android.clipboard

import android.content.ClipboardManager
import android.content.Context
import android.content.pm.PackageManager
import android.os.Build
import android.util.Log
import androidx.core.content.ContextCompat
import java.io.BufferedReader
import java.io.InputStreamReader
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale

/**
 * Monitors system logcat for clipboard change events.
 * Requires READ_LOGS permission (granted via ADB).
 *
 * Based on ClipCascade's approach:
 * - Watch logcat for ClipboardService entries containing our package name
 * - When detected, launch ClipboardFloatingActivity to get foreground status
 * - The activity reads clipboard and triggers the callback
 */
class LogcatClipboardMonitor(
    private val context: Context,
    private val onClipboardChanged: (String) -> Unit
) {
    companion object {
        private const val TAG = "LogcatClipMonitor"

        fun hasReadLogsPermission(context: Context): Boolean {
            return ContextCompat.checkSelfPermission(
                context, android.Manifest.permission.READ_LOGS
            ) == PackageManager.PERMISSION_GRANTED
        }

        fun hasOverlayPermission(context: Context): Boolean {
            return android.provider.Settings.canDrawOverlays(context)
        }
    }

    private var logcatThread: Thread? = null
    private var logcatProcess: Process? = null
    @Volatile
    private var stopLogcat = false
    private var lastActivityStartTime: Long = 0
    private val activityDebounceTime: Long = 1000 // ms

    fun start() {
        if (logcatThread != null) return
        stopLogcat = false

        logcatThread = Thread {
            try {
                Log.d(TAG, "Logcat monitor started")
                val timeStamp = SimpleDateFormat("yyyy-MM-dd HH:mm:ss.SSS", Locale.getDefault())
                    .format(Date())

                logcatProcess = Runtime.getRuntime().exec(
                    arrayOf("logcat", "-T", timeStamp, "ClipboardService:E", "*:S")
                )

                val reader = BufferedReader(InputStreamReader(logcatProcess!!.inputStream))
                reader.use { br ->
                    var line: String? = null
                    while (!stopLogcat && br.readLine().also { line = it } != null) {
                        if (line!!.contains(context.packageName)) {
                            // Clipboard change from our own app — skip
                            continue
                        }
                        // Clipboard change from another app detected
                        val currentTime = System.currentTimeMillis()
                        if (currentTime - lastActivityStartTime > activityDebounceTime) {
                            lastActivityStartTime = currentTime
                            Log.d(TAG, "Clipboard change detected, launching floating activity")
                            context.startActivity(
                                ClipboardFloatingActivity.getIntent(context)
                            )
                        }
                    }
                }
            } catch (e: InterruptedException) {
                Log.d(TAG, "Logcat monitor interrupted")
            } catch (e: Exception) {
                Log.e(TAG, "Logcat monitor error: ${e.message}")
            } finally {
                try { logcatProcess?.destroy() } catch (_: Exception) {}
                stopLogcat = false
                Log.d(TAG, "Logcat monitor stopped")
            }
        }.apply {
            isDaemon = true
            start()
        }
    }

    fun stop() {
        stopLogcat = true
        try { logcatThread?.interrupt() } catch (_: Exception) {}
        try { logcatProcess?.destroy() } catch (_: Exception) {}
        logcatThread = null
        logcatProcess = null
    }
}
