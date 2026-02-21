package com.clipsync.android.clipboard

import android.content.ClipboardManager
import android.content.Context
import android.content.Intent
import android.graphics.PixelFormat
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.util.Log
import android.view.View
import android.view.ViewTreeObserver
import android.view.WindowManager
import android.app.Activity
import com.clipsync.android.service.ClipSyncService

/**
 * Invisible floating activity that briefly gains foreground focus
 * to read clipboard content on Android 10+.
 *
 * Based on ClipCascade's ClipboardFloatingActivity approach.
 */
class ClipboardFloatingActivity : Activity() {

    companion object {
        private const val TAG = "ClipFloatingActivity"

        fun getIntent(context: Context): Intent {
            return Intent(context.applicationContext, ClipboardFloatingActivity::class.java).apply {
                flags = Intent.FLAG_ACTIVITY_NEW_TASK or
                        Intent.FLAG_ACTIVITY_CLEAR_TASK or
                        Intent.FLAG_ACTIVITY_EXCLUDE_FROM_RECENTS
            }
        }
    }

    private lateinit var windowManager: WindowManager
    private lateinit var clipboardManager: ClipboardManager
    private var floatingView: View? = null
    private var isViewAttached = false

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        clipboardManager = getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
        windowManager = getSystemService(Context.WINDOW_SERVICE) as WindowManager

        try {
            createFloatingView()
            makeFloatingViewInFocus()

            // Use layout listener to ensure view is attached before reading clipboard
            floatingView?.viewTreeObserver?.addOnGlobalLayoutListener(
                object : ViewTreeObserver.OnGlobalLayoutListener {
                    override fun onGlobalLayout() {
                        try {
                            floatingView?.viewTreeObserver?.removeOnGlobalLayoutListener(this)
                            readAndSendClipboard()
                        } catch (e: Exception) {
                            Log.e(TAG, "Error in layout listener: ${e.message}")
                        } finally {
                            makeFloatingViewOutOfFocus()
                            removeFloatingView()
                        }
                    }
                }
            )
        } catch (e: Exception) {
            Log.e(TAG, "Error creating floating view: ${e.message}")
            finish()
        }
    }

    private fun createFloatingView() {
        floatingView = View(this).apply {
            alpha = 0f // Fully invisible
        }

        val params = WindowManager.LayoutParams(
            1, 1, // 1x1 pixel
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
    }

    private fun makeFloatingViewInFocus() {
        if (isViewAttached && floatingView != null) {
            val params = floatingView!!.layoutParams as WindowManager.LayoutParams
            params.flags = params.flags and WindowManager.LayoutParams.FLAG_NOT_FOCUSABLE.inv()
            windowManager.updateViewLayout(floatingView, params)
        }
    }

    private fun makeFloatingViewOutOfFocus() {
        if (isViewAttached && floatingView != null) {
            try {
                val params = floatingView!!.layoutParams as WindowManager.LayoutParams
                params.flags = params.flags or WindowManager.LayoutParams.FLAG_NOT_FOCUSABLE
                windowManager.updateViewLayout(floatingView, params)
            } catch (e: Exception) {
                Log.e(TAG, "Error making view out of focus: ${e.message}")
            }
        }
    }

    private fun readAndSendClipboard() {
        try {
            val clip = clipboardManager.primaryClip
            if (clip != null && clip.itemCount > 0) {
                val description = clip.description ?: return
                val mimeType = description.getMimeType(0) ?: return

                if (mimeType.startsWith("text/")) {
                    val item = clip.getItemAt(0)
                    val content = item.text?.toString()
                    if (!content.isNullOrBlank()) {
                        Log.d(TAG, "Read clipboard (${content.length} chars)")
                        // Send to ClipSyncService
                        val service = ClipSyncService.instance
                        if (service != null && service.clipboardHelper.shouldUpload(content)) {
                            service.onClipboardChanged(content)
                        }
                    }
                }
            }
        } catch (e: Exception) {
            Log.e(TAG, "Failed to read clipboard: ${e.message}")
        }
    }

    private fun removeFloatingView() {
        if (isViewAttached && floatingView != null) {
            try {
                windowManager.removeViewImmediate(floatingView)
            } catch (e: Exception) {
                Log.e(TAG, "Error removing floating view: ${e.message}")
            }
            isViewAttached = false
        }
        finish()
    }

    override fun onDestroy() {
        super.onDestroy()
        if (isViewAttached && floatingView != null) {
            try {
                windowManager.removeViewImmediate(floatingView)
            } catch (_: Exception) {}
            isViewAttached = false
        }
    }
}
