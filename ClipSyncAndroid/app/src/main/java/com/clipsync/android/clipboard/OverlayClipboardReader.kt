package com.clipsync.android.clipboard

import android.content.ClipboardManager
import android.content.Context
import android.graphics.PixelFormat
import android.os.Handler
import android.os.Looper
import android.provider.Settings
import android.util.Log
import android.view.LayoutInflater
import android.view.View
import android.view.ViewTreeObserver
import android.view.WindowManager
import com.clipsync.android.R

/**
 * Reads the clipboard from background using an invisible overlay window.
 *
 * Unlike ClipboardFloatingActivity, this does NOT start a new Activity
 * and does NOT interrupt the user's current app. It creates a tiny
 * TYPE_APPLICATION_OVERLAY window, briefly steals input focus to read
 * the clipboard, then immediately cleans up.
 *
 * Requires SYSTEM_ALERT_WINDOW permission (granted via ADB).
 */
class OverlayClipboardReader(private val context: Context) {

    companion object {
        private const val TAG = "OverlayClipReader"
        private const val TIMEOUT_MS = 3000L
    }

    private val handler = Handler(Looper.getMainLooper())
    private val windowManager = context.getSystemService(Context.WINDOW_SERVICE) as WindowManager
    private val clipboardManager = context.getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager

    @Volatile
    private var isReading = false
    private var floatingView: View? = null
    private var isViewAttached = false
    private var layoutListener: ViewTreeObserver.OnGlobalLayoutListener? = null

    /**
     * Try to read the clipboard using the overlay focus trick.
     * Must be called from the main thread.
     *
     * @param onResult Called with clipboard text content, or null if failed.
     */
    fun tryRead(onResult: (String?) -> Unit) {
        if (isReading) {
            Log.w(TAG, "Already reading, skipping")
            onResult(null)
            return
        }

        if (!Settings.canDrawOverlays(context)) {
            Log.w(TAG, "No overlay permission")
            onResult(null)
            return
        }

        isReading = true

        // Safety timeout
        handler.postDelayed({
            if (isReading) {
                Log.w(TAG, "Timeout, cleaning up")
                cleanup()
                onResult(null)
            }
        }, TIMEOUT_MS)

        try {
            // 1. Create overlay view (initially NOT focusable)
            val inflater = context.getSystemService(Context.LAYOUT_INFLATER_SERVICE) as LayoutInflater
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

            windowManager.addView(floatingView, params)
            isViewAttached = true

            // 2. Remove NOT_FOCUSABLE to steal input focus
            val updatedParams = floatingView!!.layoutParams as WindowManager.LayoutParams
            updatedParams.flags = updatedParams.flags and WindowManager.LayoutParams.FLAG_NOT_FOCUSABLE.inv()
            windowManager.updateViewLayout(floatingView, updatedParams)

            // 3. Wait for layout = focus acquired, then read clipboard
            layoutListener = ViewTreeObserver.OnGlobalLayoutListener {
                try {
                    floatingView?.viewTreeObserver?.removeOnGlobalLayoutListener(layoutListener)
                } catch (_: Exception) {}

                val content = readClipboardText()
                cleanup()
                onResult(content)
            }

            floatingView?.viewTreeObserver?.addOnGlobalLayoutListener(layoutListener)

        } catch (e: Exception) {
            Log.e(TAG, "Failed to create overlay: ${e.message}")
            cleanup()
            onResult(null)
        }
    }

    private fun readClipboardText(): String? {
        return try {
            val clip = clipboardManager.primaryClip
            if (clip != null && clip.itemCount > 0) {
                val mimeType = clip.description?.getMimeType(0)
                if (mimeType != null && mimeType.startsWith("text/")) {
                    val text = clip.getItemAt(0).text?.toString()
                    if (!text.isNullOrBlank()) {
                        Log.d(TAG, "Read clipboard OK (${text.length} chars)")
                        text
                    } else null
                } else null
            } else {
                Log.w(TAG, "primaryClip is null (still denied?)")
                null
            }
        } catch (e: Exception) {
            Log.e(TAG, "Error reading clipboard: ${e.message}")
            null
        }
    }

    private fun cleanup() {
        handler.removeCallbacksAndMessages(null)
        isReading = false

        if (isViewAttached && floatingView != null) {
            // Restore NOT_FOCUSABLE before removing
            try {
                val params = floatingView!!.layoutParams as WindowManager.LayoutParams
                params.flags = params.flags or WindowManager.LayoutParams.FLAG_NOT_FOCUSABLE
                windowManager.updateViewLayout(floatingView, params)
            } catch (_: Exception) {}

            try {
                windowManager.removeViewImmediate(floatingView)
            } catch (_: Exception) {}
        }

        floatingView = null
        layoutListener = null
        isViewAttached = false
    }
}
