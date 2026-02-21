package com.clipsync.android.clipboard

import android.content.pm.PackageManager
import android.content.Context
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
 * How it works (same as ClipCascade):
 * 1. The standard OnPrimaryClipChangedListener fires when clipboard changes
 * 2. If the app is in background, reading clipboard data fails
 * 3. ClipboardService logs an ERROR mentioning our package name
 * 4. We detect this error in logcat
 * 5. We launch ClipboardFloatingActivity to gain foreground focus and read clipboard
 */
class LogcatClipboardMonitor(
    private val context: Context
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
                        // ClipboardService logs an error containing our package name
                        // when we try to read clipboard from background and fail.
                        // This is our signal that clipboard changed.
                        if (line!!.contains(context.packageName)) {
                            val currentTime = System.currentTimeMillis()
                            if (currentTime - lastActivityStartTime > activityDebounceTime) {
                                lastActivityStartTime = currentTime
                                Log.d(TAG, "Clipboard error detected for our app, launching floating activity")
                                context.startActivity(
                                    ClipboardFloatingActivity.getIntent(context)
                                )
                            }
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
