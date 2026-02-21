package com.clipsync.android.service

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.util.Log
import com.clipsync.android.data.PrefsManager
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch

/**
 * Receives BOOT_COMPLETED broadcast and starts ClipSyncService
 * if the user has a valid login session.
 */
class BootReceiver : BroadcastReceiver() {

    companion object {
        private const val TAG = "BootReceiver"
    }

    override fun onReceive(context: Context?, intent: Intent?) {
        if (context == null) return
        if (intent?.action != Intent.ACTION_BOOT_COMPLETED) return

        Log.d(TAG, "Boot completed, checking config...")

        CoroutineScope(Dispatchers.IO).launch {
            val prefs = PrefsManager(context)
            val config = prefs.configFlow.first()
            if (config.isLoggedIn) {
                Log.d(TAG, "Valid session found, starting service")
                ClipSyncService.start(context)
            } else {
                Log.d(TAG, "No valid session, skipping auto-start")
            }
        }
    }
}
