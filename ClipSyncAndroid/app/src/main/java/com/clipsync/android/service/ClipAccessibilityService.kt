package com.clipsync.android.service

import android.accessibilityservice.AccessibilityService
import android.util.Log
import android.view.accessibility.AccessibilityEvent

/**
 * Accessibility service that monitors clipboard changes.
 * On Android 10+, background apps cannot read the clipboard directly.
 * This service works around that restriction by detecting clipboard-related events.
 */
class ClipAccessibilityService : AccessibilityService() {

    companion object {
        private const val TAG = "ClipA11yService"

        @Volatile
        var isRunning: Boolean = false
            private set
    }

    private var lastContent: String = ""

    override fun onServiceConnected() {
        super.onServiceConnected()
        isRunning = true
        Log.d(TAG, "Accessibility service connected")
    }

    override fun onAccessibilityEvent(event: AccessibilityEvent?) {
        if (event == null) return

        // Listen for clipboard change events or view text selection events
        // that might indicate a copy action
        when (event.eventType) {
            AccessibilityEvent.TYPE_VIEW_TEXT_SELECTION_CHANGED,
            AccessibilityEvent.TYPE_VIEW_TEXT_CHANGED -> {
                checkClipboard()
            }
            // Catch-all: periodically check when any event fires
            else -> {
                // Only check on content change events to reduce overhead
                if (event.eventType == AccessibilityEvent.TYPE_WINDOW_CONTENT_CHANGED) {
                    checkClipboard()
                }
            }
        }
    }

    override fun onInterrupt() {
        Log.w(TAG, "Accessibility service interrupted")
    }

    override fun onDestroy() {
        isRunning = false
        Log.d(TAG, "Accessibility service destroyed")
        super.onDestroy()
    }

    private fun checkClipboard() {
        val service = ClipSyncService.instance ?: return

        try {
            val content = service.clipboardHelper.readClipboard() ?: return
            if (content.isBlank() || content == lastContent) return
            lastContent = content

            if (service.clipboardHelper.shouldUpload(content)) {
                Log.d(TAG, "Clipboard changed (${content.length} chars), uploading...")
                service.onClipboardChanged(content)
            }
        } catch (e: Exception) {
            Log.e(TAG, "Error checking clipboard: ${e.message}")
        }
    }
}
