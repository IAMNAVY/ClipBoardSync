package com.clipsync.android.service

import android.accessibilityservice.AccessibilityService
import android.os.Handler
import android.os.Looper
import android.util.Log
import android.view.accessibility.AccessibilityEvent

/**
 * Accessibility service that monitors clipboard changes.
 * On Android 10+, background apps cannot read the clipboard directly.
 * This service works around that restriction by detecting clipboard-related events.
 *
 * Uses debouncing (500ms) to avoid high-frequency triggers that can cause
 * the system to report the service as "abnormal" and kill it.
 */
class ClipAccessibilityService : AccessibilityService() {

    companion object {
        private const val TAG = "ClipA11yService"
        private const val DEBOUNCE_MS = 500L

        @Volatile
        var isRunning: Boolean = false
            private set
    }

    private var lastContent: String = ""
    private val handler = Handler(Looper.getMainLooper())
    private var pendingCheck: Runnable? = null

    override fun onServiceConnected() {
        super.onServiceConnected()
        isRunning = true
        Log.d(TAG, "Accessibility service connected")
    }

    override fun onAccessibilityEvent(event: AccessibilityEvent?) {
        if (event == null) return

        // Only respond to text selection changes which typically indicate a copy action.
        // Avoid TYPE_WINDOW_CONTENT_CHANGED to prevent high-frequency firing.
        when (event.eventType) {
            AccessibilityEvent.TYPE_VIEW_TEXT_SELECTION_CHANGED -> {
                scheduleClipboardCheck()
            }
        }
    }

    override fun onInterrupt() {
        // Called when the system interrupts the service. Just log, never crash here.
        Log.w(TAG, "Accessibility service interrupted")
    }

    override fun onDestroy() {
        isRunning = false
        pendingCheck?.let { handler.removeCallbacks(it) }
        pendingCheck = null
        Log.d(TAG, "Accessibility service destroyed")
        super.onDestroy()
    }

    /**
     * Debounced clipboard check. Coalesces rapid-fire events into a single check.
     */
    private fun scheduleClipboardCheck() {
        pendingCheck?.let { handler.removeCallbacks(it) }
        val runnable = Runnable { checkClipboard() }
        pendingCheck = runnable
        handler.postDelayed(runnable, DEBOUNCE_MS)
    }

    private fun checkClipboard() {
        try {
            val service = ClipSyncService.instance ?: return

            val content = service.clipboardHelper.readClipboard()
            // readClipboard() returns null when in background (Android 10+ restriction)
            if (content.isNullOrBlank() || content == lastContent) return
            lastContent = content

            if (service.clipboardHelper.shouldUpload(content)) {
                Log.d(TAG, "Clipboard changed (${content.length} chars), uploading...")
                service.onClipboardChanged(content)
            }
        } catch (e: Exception) {
            // Swallow all exceptions to prevent the system from killing the service
            Log.e(TAG, "Error in checkClipboard: ${e.message}")
        }
    }
}
