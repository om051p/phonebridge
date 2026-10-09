package dev.phonebridge.notification

import android.app.KeyguardManager
import android.content.Context
import android.os.PowerManager
import android.provider.Settings
import android.util.Log
import androidx.core.app.NotificationManagerCompat
import dev.phonebridge.bridge.GoBridge
import dev.phonebridge.bridge.NotificationHostCallback

/**
 * AndroidNotificationManager coordinates notification mirroring from Android to Linux (DEC-028).
 *
 * Enforces security & privacy gates:
 * 1. Screen-Lock Guard: Drops or masks sensitive notifications if device is locked.
 * 2. Self-Notification Suppression: Filters dev.phonebridge's own service notification.
 * 3. Zero-Logging Rule: Strictly prohibits logging notification title, text, or subtext.
 */
object AndroidNotificationManager : NotificationHostCallback {

    private const val TAG = "AndroidNotifMgr"

    @Volatile
    private var appContext: Context? = null

    @Volatile
    private var isStarted = false

    @Volatile
    private var listenerService: PhoneBridgeNotificationListenerService? = null

    fun start(context: Context) {
        appContext = context.applicationContext
        if (!isStarted) {
            try {
                if (GoBridge.notificationInit(this)) {
                    isStarted = true
                    Log.i(TAG, "AndroidNotificationManager initialized with GoBridge")
                    // If listener is already connected, sync current active notifications
                    listenerService?.syncActiveNotifications()
                } else {
                    Log.w(TAG, "GoBridge.notificationInit returned false")
                }
            } catch (t: Throwable) {
                Log.e(TAG, "Failed to initialize GoBridge notifications: ${t.message}", t)
            }
        }
    }

    fun stop() {
        if (isStarted) {
            try {
                GoBridge.notificationStop()
            } catch (t: Throwable) {
                Log.w(TAG, "GoBridge.notificationStop error: ${t.message}")
            }
            isStarted = false
        }
        appContext = null
        Log.i(TAG, "AndroidNotificationManager stopped")
    }

    fun registerService(service: PhoneBridgeNotificationListenerService) {
        listenerService = service
        Log.i(TAG, "PhoneBridgeNotificationListenerService registered")
        if (isStarted) {
            service.syncActiveNotifications()
        }
    }

    fun unregisterService(service: PhoneBridgeNotificationListenerService) {
        if (listenerService == service) {
            listenerService = null
            Log.i(TAG, "PhoneBridgeNotificationListenerService unregistered")
        }
    }

    /**
     * Runtime liveness: true only while the platform listener service is
     * actually bound (connected without a matching disconnect). User-granted
     * access (Settings) is necessary but not sufficient — the UI must not
     * read "granted" as "active".
     */
    fun isListenerConnected(): Boolean = listenerService != null

    /**
     * Checks whether the device is currently locked.
     */
    fun isDeviceLocked(context: Context): Boolean {
        val km = context.getSystemService(Context.KEYGUARD_SERVICE) as? KeyguardManager
        return km?.isKeyguardLocked ?: false
    }

    /**
     * Checks whether Notification Listener special access has been granted to PhoneBridge.
     */
    fun isNotificationAccessGranted(context: Context): Boolean {
        return try {
            val enabledListeners = NotificationManagerCompat.getEnabledListenerPackages(context)
            enabledListeners.contains(context.packageName)
        } catch (t: Throwable) {
            val listeners = Settings.Secure.getString(
                context.contentResolver,
                "enabled_notification_listeners"
            ) ?: ""
            listeners.contains(context.packageName)
        }
    }

    fun onNotificationPosted(
        key: String,
        packageName: String,
        appName: String,
        title: String,
        text: String,
        subText: String,
        postTimeMs: Long,
        isOngoing: Boolean,
        isClearable: Boolean,
        category: String,
    ): Boolean {
        if (!isStarted) return false
        // Zero-logging rule: Only log metadata, character lengths, never content.
        Log.d(TAG, "dispatch post: pkg=$packageName key=$key titleLen=${title.length} textLen=${text.length}")
        return GoBridge.notificationPost(
            key, packageName, appName, title, text, subText,
            postTimeMs, isOngoing, isClearable, category
        )
    }

    fun onNotificationRemoved(
        key: String,
        packageName: String,
        reason: Int,
    ): Boolean {
        if (!isStarted) return false
        Log.d(TAG, "dispatch remove: pkg=$packageName key=$key reason=$reason")
        return GoBridge.notificationRemove(key, packageName, reason)
    }

    override fun onDismiss(key: String): Boolean {
        return dismissNotification(key)
    }

    fun dismissNotification(key: String): Boolean {
        val service = listenerService
        if (service == null) {
            Log.w(TAG, "Cannot dismiss notification: listenerService is null")
            return false
        }
        return try {
            service.cancelNotification(key)
            Log.d(TAG, "Requested cancelNotification for key=$key")
            true
        } catch (t: Throwable) {
            Log.e(TAG, "Failed to cancelNotification for key=$key: ${t.message}")
            false
        }
    }
}
