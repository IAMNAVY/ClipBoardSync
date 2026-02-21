package com.clipsync.android.clipboard

import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import android.util.Log

/**
 * Manages clipboard read/write with an anti-loop mechanism to prevent
 * re-uploading content that was just written from a remote source.
 */
class ClipboardHelper(private val context: Context) {

    companion object {
        private const val TAG = "ClipboardHelper"
    }

    @Volatile
    private var lastRemoteWrite: String = ""

    @Volatile
    private var skipNextChange: Boolean = false

    private val clipboardManager: ClipboardManager
        get() = context.getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager

    /**
     * Write content from a remote source to local clipboard.
     * Sets anti-loop flags so the accessibility service won't re-upload it.
     */
    fun writeClipboard(content: String) {
        lastRemoteWrite = content
        skipNextChange = true
        try {
            val clip = ClipData.newPlainText("ClipSync", content)
            clipboardManager.setPrimaryClip(clip)
            Log.d(TAG, "Wrote remote content to clipboard (${content.length} chars)")
        } catch (e: Exception) {
            Log.e(TAG, "Failed to write clipboard: ${e.message}")
        }
    }

    /**
     * Read current clipboard text content.
     */
    fun readClipboard(): String? {
        return try {
            val clip = clipboardManager.primaryClip
            if (clip != null && clip.itemCount > 0) {
                clip.getItemAt(0).text?.toString()
            } else null
        } catch (e: Exception) {
            Log.e(TAG, "Failed to read clipboard: ${e.message}")
            null
        }
    }

    /**
     * Check if the given content should be uploaded (i.e., it's not from a remote write).
     * Returns true if the content is a genuine local clipboard change.
     */
    fun shouldUpload(content: String): Boolean {
        if (content.isBlank()) return false

        if (skipNextChange) {
            skipNextChange = false
            if (content == lastRemoteWrite) {
                Log.d(TAG, "Skipping remote write content (anti-loop)")
                return false
            }
        }
        return true
    }
}
