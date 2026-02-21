package com.clipsync.android.clipboard

import android.content.ClipboardManager
import android.content.Context
import android.content.Intent
import android.graphics.PixelFormat
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.provider.Settings
import android.util.Log
import android.view.LayoutInflater
import android.view.View
import android.view.ViewTreeObserver
import android.view.WindowManager
import android.app.Activity
import com.clipsync.android.R
import com.clipsync.android.service.ClipSyncService

/**
 * Invisible floating activity that briefly gains foreground focus
 * to read clipboard content on Android 10+.
 *
 * Uses ClipCascade's overlay focus-stealing approach:
 * 1. Create TYPE_APPLICATION_OVERLAY view (initially NOT_FOCUSABLE)
 * 2. Toggle off NOT_FOCUSABLE → overlay steals input focus
 * 3. OnGlobalLayoutListener fires → read clipboard
 * 4. Cleanup and finish
 *
 * Fallback: if overlay creation fails, try reading on onWindowFocusChanged.
 *
 * Requires SYSTEM_ALERT_WINDOW permission (granted via ADB).
 */
class ClipboardFloatingActivity : Activity() {

    companion object {
        private const val TAG = "ClipFloatingActivity"
        private const val SAFETY_TIMEOUT_MS = 5000L

        fun getIntent(context: Context): Intent {
            return Intent(context.applicationContext, ClipboardFloatingActivity::class.java).apply {
                flags = Intent.FLAG_ACTIVITY_NEW_TASK or
                        Intent.FLAG_ACTIVITY_CLEAR_TASK or
                        Intent.FLAG_ACTIVITY_EXCLUDE_FROM_RECENTS
            }
        }
    }

    private var windowManager: WindowManager? = null
    private var clipboardManager: ClipboardManager? = null
    private var floatingView: View? = null
    private var globalLayoutListener: ViewTreeObserver.OnGlobalLayoutListener? = null
    private var isViewAttached = false
    private val handler = Handler(Looper.getMainLooper())
    @Volatile
    private var clipboardRead = false

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        Log.w(TAG, "=== Activity onCreate ===")  // Use WARN level to ensure visibility

        clipboardManager = getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
        windowManager = getSystemService(Context.WINDOW_SERVICE) as WindowManager

        // Check overlay permission
        val hasOverlay = Settings.canDrawOverlays(this)
        Log.w(TAG, "Overlay permission: $hasOverlay")

        // Safety timeout
        handler.postDelayed({
            if (!clipboardRead) {
                Log.w(TAG, "Safety timeout reached, finishing")
                cleanup()
            }
        }, SAFETY_TIMEOUT_MS)

        if (hasOverlay) {
            tryOverlayApproach()
        } else {
            Log.w(TAG, "No overlay permission, trying direct read with focus")
            // Fallback: try reading when activity gets focus
            tryDirectRead()
        }
    }

    /**
     * Primary approach: use a TYPE_APPLICATION_OVERLAY to steal focus.
     * This is ClipCascade's proven approach.
     */
    private fun tryOverlayApproach() {
        try {
            val inflater = getSystemService(Context.LAYOUT_INFLATER_SERVICE) as LayoutInflater
            floatingView = inflater.inflate(R.layout.floating_view_layout, null)

            val params = WindowManager.LayoutParams(
                WindowManager.LayoutParams.WRAP_CONTENT,
                WindowManager.LayoutParams.WRAP_CONTENT,
                WindowManager.LayoutParams.TYPE_APPLICATION_OVERLAY,
                WindowManager.LayoutParams.FLAG_NOT_FOCUSABLE or
                        WindowManager.LayoutParams.FLAG_WATCH_OUTSIDE_TOUCH,
                PixelFormat.TRANSLUCENT
            ).apply {
                x = 0
                y = 0
            }

            windowManager?.addView(floatingView, params)
            isViewAttached = true
            Log.w(TAG, "Overlay view added successfully")

            // Remove NOT_FOCUSABLE to steal focus
            val updatedParams = floatingView!!.layoutParams as WindowManager.LayoutParams
            updatedParams.flags = updatedParams.flags and WindowManager.LayoutParams.FLAG_NOT_FOCUSABLE.inv()
            windowManager?.updateViewLayout(floatingView, updatedParams)
            Log.w(TAG, "Overlay set to focusable")

            // Wait for layout to complete (= focus acquired)
            globalLayoutListener = ViewTreeObserver.OnGlobalLayoutListener {
                Log.w(TAG, "OnGlobalLayout fired!")
                try {
                    floatingView?.viewTreeObserver?.removeOnGlobalLayoutListener(globalLayoutListener)
                } catch (_: Exception) {}

                readClipboardAndUpload()

                // Cleanup
                restoreAndRemoveOverlay()
            }

            floatingView?.viewTreeObserver?.addOnGlobalLayoutListener(globalLayoutListener)
            Log.w(TAG, "GlobalLayoutListener registered, waiting for layout...")

        } catch (e: Exception) {
            Log.e(TAG, "Overlay approach failed: ${e.message}", e)
            // Fallback
            tryDirectRead()
        }
    }

    /**
     * Fallback approach: just try reading on the next frame when Activity has focus.
     */
    private fun tryDirectRead() {
        handler.postDelayed({
            Log.w(TAG, "Trying direct clipboard read...")
            readClipboardAndUpload()
            finish()
        }, 300) // Small delay to let activity settle
    }

    override fun onWindowFocusChanged(hasFocus: Boolean) {
        super.onWindowFocusChanged(hasFocus)
        Log.w(TAG, "onWindowFocusChanged: hasFocus=$hasFocus, clipboardRead=$clipboardRead")

        if (hasFocus && !clipboardRead && !isViewAttached) {
            // If we're using the direct approach and got focus, try reading
            handler.post {
                readClipboardAndUpload()
                finish()
            }
        }
    }

    private fun readClipboardAndUpload() {
        if (clipboardRead) return
        clipboardRead = true

        try {
            val clip = clipboardManager?.primaryClip
            Log.w(TAG, "primaryClip: ${if (clip != null) "${clip.itemCount} items" else "null"}")

            if (clip != null && clip.itemCount > 0) {
                val description = clip.description
                val mimeType = description?.getMimeType(0)
                Log.w(TAG, "mimeType: $mimeType")

                if (mimeType != null && mimeType.startsWith("text/")) {
                    val item = clip.getItemAt(0)
                    val content = item.text?.toString()
                    if (!content.isNullOrBlank()) {
                        Log.w(TAG, "SUCCESS: Read clipboard (${content.length} chars)")
                        val service = ClipSyncService.instance
                        if (service != null) {
                            if (service.clipboardHelper.shouldUpload(content)) {
                                service.onClipboardChanged(content)
                                Log.w(TAG, "Clipboard sent for upload")
                            } else {
                                Log.w(TAG, "Skipped (already uploaded or from remote)")
                            }
                        } else {
                            Log.e(TAG, "ClipSyncService instance is null!")
                        }
                    } else {
                        Log.w(TAG, "Clipboard text is blank or null")
                    }
                } else {
                    Log.w(TAG, "Not text content, skipping")
                }
            } else {
                Log.e(TAG, "FAILED: primaryClip is null (denied by system)")
            }
        } catch (e: Exception) {
            Log.e(TAG, "Exception reading clipboard: ${e.message}", e)
        }
    }

    private fun restoreAndRemoveOverlay() {
        if (isViewAttached && floatingView != null) {
            try {
                val params = floatingView!!.layoutParams as WindowManager.LayoutParams
                params.flags = params.flags or WindowManager.LayoutParams.FLAG_NOT_FOCUSABLE
                windowManager?.updateViewLayout(floatingView, params)
            } catch (e: Exception) {
                Log.e(TAG, "Error restoring focus flag: ${e.message}")
            }

            try {
                windowManager?.removeViewImmediate(floatingView)
            } catch (e: Exception) {
                Log.e(TAG, "Error removing overlay view: ${e.message}")
            }
            isViewAttached = false
        }
        finish()
    }

    private fun cleanup() {
        handler.removeCallbacksAndMessages(null)
        if (isViewAttached && floatingView != null) {
            try {
                windowManager?.removeViewImmediate(floatingView)
            } catch (_: Exception) {}
            isViewAttached = false
        }
        finish()
    }

    override fun onDestroy() {
        handler.removeCallbacksAndMessages(null)
        if (isViewAttached && floatingView != null) {
            try {
                windowManager?.removeViewImmediate(floatingView)
            } catch (_: Exception) {}
            isViewAttached = false
        }
        Log.w(TAG, "=== Activity onDestroy ===")
        super.onDestroy()
    }
}
