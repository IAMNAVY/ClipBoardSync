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
 *
 * IMPORTANT: The system ClipboardService logs are always present regardless
 * of app release/debug mode. These are SYSTEM logs, not app logs.
 *
 * Release mode considerations:
 * - The logcat process may die silently; a watchdog restarts it
 * - Process death can happen due to battery optimization
 * - The logcat binary is always available on the device
 */
class LogcatClipboardMonitor(
    private val context: Context
) {
    companion object {
        private const val TAG = "LogcatClipMonitor"
        private const val WATCHDOG_INTERVAL_MS = 30_000L // Check every 30s
        private const val RESTART_DELAY_MS = 2_000L

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
    private var watchdogThread: Thread? = null
    private var logcatProcess: Process? = null
    @Volatile
    private var stopRequested = false
    @Volatile
    private var isLogcatRunning = false
    private var lastActivityStartTime: Long = 0
    private val activityDebounceTime: Long = 1500 // ms — slightly longer debounce for release stability
    @Volatile
    private var consecutiveFailures = 0
    private val maxConsecutiveFailures = 10

    fun start() {
        if (logcatThread != null) return
        stopRequested = false
        consecutiveFailures = 0

        startLogcatThread()
        startWatchdog()
    }

    /**
     * Start the main logcat monitoring thread.
     */
    private fun startLogcatThread() {
        logcatThread = Thread {
            while (!stopRequested && consecutiveFailures < maxConsecutiveFailures) {
                try {
                    isLogcatRunning = true
                    runLogcatMonitor()
                } catch (e: InterruptedException) {
                    Log.d(TAG, "Logcat monitor interrupted")
                    break
                } catch (e: Exception) {
                    Log.e(TAG, "Logcat monitor error: ${e.message}")
                    consecutiveFailures++
                }

                isLogcatRunning = false

                if (stopRequested) break

                // Wait before restarting
                try {
                    Log.d(TAG, "Logcat process ended, restarting in ${RESTART_DELAY_MS}ms (failure #$consecutiveFailures)")
                    Thread.sleep(RESTART_DELAY_MS)
                } catch (e: InterruptedException) {
                    break
                }
            }

            if (consecutiveFailures >= maxConsecutiveFailures) {
                Log.e(TAG, "Too many consecutive failures ($maxConsecutiveFailures), giving up")
            }
            isLogcatRunning = false
            Log.d(TAG, "Logcat monitor thread ended")
        }.apply {
            isDaemon = true
            name = "ClipSync-LogcatMonitor"
            start()
        }
    }

    /**
     * Watchdog thread that periodically checks if the logcat monitor is still running.
     * If it's not, it restarts the monitor thread.
     */
    private fun startWatchdog() {
        watchdogThread = Thread {
            Log.d(TAG, "Watchdog started")
            while (!stopRequested) {
                try {
                    Thread.sleep(WATCHDOG_INTERVAL_MS)
                } catch (e: InterruptedException) {
                    break
                }

                if (stopRequested) break

                // Check if logcat thread is still alive
                if (logcatThread == null || logcatThread?.isAlive != true) {
                    Log.w(TAG, "Watchdog: logcat thread died, restarting...")
                    consecutiveFailures = 0 // Reset on watchdog restart
                    logcatThread = null
                    startLogcatThread()
                } else if (!isLogcatRunning) {
                    Log.w(TAG, "Watchdog: logcat not running despite thread alive, process may have died")
                }
            }
            Log.d(TAG, "Watchdog stopped")
        }.apply {
            isDaemon = true
            name = "ClipSync-Watchdog"
            start()
        }
    }

    /**
     * Run a single logcat monitoring session.
     * This blocks until the logcat process ends or an error occurs.
     */
    private fun runLogcatMonitor() {
        Log.d(TAG, "Starting logcat monitor session")
        val timeStamp = SimpleDateFormat("yyyy-MM-dd HH:mm:ss.SSS", Locale.getDefault())
            .format(Date())

        // Use ProcessBuilder for better control
        val processBuilder = ProcessBuilder(
            "logcat", "-T", timeStamp, "ClipboardService:E", "*:S"
        )
        processBuilder.redirectErrorStream(true)

        logcatProcess = processBuilder.start()
        val process = logcatProcess ?: throw IllegalStateException("Failed to start logcat process")

        val reader = BufferedReader(InputStreamReader(process.inputStream))
        try {
            var line: String?
            while (!stopRequested) {
                line = reader.readLine()
                if (line == null) {
                    // Process ended
                    Log.d(TAG, "Logcat process output stream ended")
                    break
                }

                // Reset failure counter on successful read
                consecutiveFailures = 0

                // ClipboardService logs an error containing our package name
                // when we try to read clipboard from background and fail.
                // This is our signal that clipboard changed.
                if (line.contains(context.packageName)) {
                    val currentTime = System.currentTimeMillis()
                    if (currentTime - lastActivityStartTime > activityDebounceTime) {
                        lastActivityStartTime = currentTime
                        Log.d(TAG, "Clipboard denial detected, launching floating activity")
                        try {
                            context.startActivity(
                                ClipboardFloatingActivity.getIntent(context)
                            )
                        } catch (e: Exception) {
                            Log.e(TAG, "Failed to start floating activity: ${e.message}")
                        }
                    }
                }
            }
        } finally {
            try { reader.close() } catch (_: Exception) {}
            try { process.destroy() } catch (_: Exception) {}
            logcatProcess = null
        }
    }

    fun stop() {
        stopRequested = true
        try { watchdogThread?.interrupt() } catch (_: Exception) {}
        try { logcatThread?.interrupt() } catch (_: Exception) {}
        try { logcatProcess?.destroy() } catch (_: Exception) {}
        watchdogThread = null
        logcatThread = null
        logcatProcess = null
        isLogcatRunning = false
        Log.d(TAG, "Monitor stopped")
    }

    /**
     * Check if the monitor is actively running.
     */
    fun isActive(): Boolean = isLogcatRunning && !stopRequested
}
