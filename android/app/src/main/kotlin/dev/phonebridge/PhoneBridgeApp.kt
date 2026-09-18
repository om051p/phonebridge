package dev.phonebridge

import android.app.Application
import android.util.Log
import dev.phonebridge.service.PhoneBridgeService

/**
 * PhoneBridgeApp is the primary Application class for the PhoneBridge Android host.
 */
class PhoneBridgeApp : Application() {
    override fun onCreate() {
        super.onCreate()
        try {
            PhoneBridgeService.startService(this)
            Log.i("PhoneBridgeApp", "PhoneBridgeService started from Application.onCreate")
        } catch (t: Throwable) {
            Log.e("PhoneBridgeApp", "Failed to start PhoneBridgeService: ${t.message}", t)
        }
    }
}
