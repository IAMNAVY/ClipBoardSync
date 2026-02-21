package com.clipsync.android.clipboard

import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import android.util.Log

/**
 * Manages clipboard read/write with an anti-loop mechanism to prevent
 * re-uploading content that was just written from a remote source.
 *
 * The anti-loop is based on content comparison (idempotent), not one-shot flags,
 * so multiple callers can safely check without race conditions.
 */
class ClipboardHelper(private val context: Context) {

    companion object {
        private const val TAG = "ClipboardHelper"
    }

    /** Content last received from the remote server and written to clipboard */
    @Volatile
    private var lastRemoteWrite: String = ""

    /** Content last successfully uploaded to the server */
    @Volatile
    private var lastUploadedContent: String = ""

    private val clipboardManager: ClipboardManager
        get() = context.getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager

    /**
     * Write content from a remote source to local clipboard.
     * Marks this content so it won't be re-uploaded.
     */
    fun writeClipboard(content: String) {
        lastRemoteWrite = content
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
     * Check if the given content should be uploaded.
     * This is idempotent — safe to call multiple times with the same content.
     * Returns true only if the content is:
     *   - not blank
     *   - not the same as what was last received from server
     *   - not the same as what was last uploaded
     */
    fun shouldUpload(content: String): Boolean {
        if (content.isBlank()) return false

        // Don't re-upload content we just wrote from the server
        if (content == lastRemoteWrite) {
            Log.d(TAG, "Skipping: matches last remote write (anti-loop)")
            return false
        }

        // Don't upload the same content twice
        if (content == lastUploadedContent) {
            Log.d(TAG, "Skipping: already uploaded this content")
            return false
        }

        return true
    }

    /**
     * Mark content as successfully uploaded to prevent duplicate uploads.
     */
    fun markUploaded(content: String) {
        lastUploadedContent = content
    }
}
