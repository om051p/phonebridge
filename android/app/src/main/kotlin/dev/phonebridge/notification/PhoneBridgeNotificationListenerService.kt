package dev.phonebridge.notification

import android.app.Notification
import android.content.pm.PackageManager
import android.os.Build
import android.service.notification.NotificationListenerService
import android.service.notification.StatusBarNotification
import android.util.Log
import java.util.concurrent.ConcurrentHashMap

/**
 * PhoneBridgeNotificationListenerService captures system notifications and mirrors them
 * to the Linux desktop client over WebRTC (DEC-028, Phase 8 v0.1).
 *
 * Enforces strict security & privacy gates:
 * 1. Self-Notification Suppression: Discards PhoneBridge's own ongoing service notification.
 * 2. Screen-Lock Guard: Drops VISIBILITY_SECRET notifications when device is locked.
 * 3. Zero-Logging Rule: Strictly prohibits logging notification title, text, or subtext.
 */
class PhoneBridgeNotificationListenerService : NotificationListenerService() {

    companion object {
        private const val TAG = "PhoneBridgeNotifSvc"
        private val appNameCache = ConcurrentHashMap<String, String>()
    }

    override fun onListenerConnected() {
        super.onListenerConnected()
        Log.i(TAG, "NotificationListenerService connected")
        AndroidNotificationManager.registerService(this)
    }

    override fun onListenerDisconnected() {
        super.onListenerDisconnected()
        Log.i(TAG, "NotificationListenerService disconnected")
        AndroidNotificationManager.unregisterService(this)
    }

    override fun onNotificationPosted(sbn: StatusBarNotification?) {
        super.onNotificationPosted(sbn)
        if (sbn == null) return
        handlePosted(sbn)
    }

    override fun onNotificationRemoved(sbn: StatusBarNotification?, rankingMap: RankingMap?, reason: Int) {
        super.onNotificationRemoved(sbn, rankingMap, reason)
        if (sbn == null) return
        handleRemoved(sbn, reason)
    }

    override fun onNotificationRemoved(sbn: StatusBarNotification?) {
        super.onNotificationRemoved(sbn)
        if (sbn == null) return
        handleRemoved(sbn, 0)
    }

    fun syncActiveNotifications() {
        try {
            val sbns = activeNotifications ?: return
            Log.i(TAG, "Syncing active notifications: total=${sbns.size}")
            for (sbn in sbns) {
                handlePosted(sbn)
            }
        } catch (t: Throwable) {
            Log.w(TAG, "Failed to query active notifications: ${t.message}")
        }
    }

    private fun handlePosted(sbn: StatusBarNotification) {
        val pkg = sbn.packageName ?: return

        // 1. Self-Notification Suppression
        if (pkg == packageName) {
            return
        }

        val notification = sbn.notification ?: return

        // 2. Screen-Lock Guard
        val isLocked = AndroidNotificationManager.isDeviceLocked(this)
        if (isLocked) {
            if (notification.visibility == Notification.VISIBILITY_SECRET) {
                Log.d(TAG, "Suppressed secret notification while locked: pkg=$pkg")
                return
            }
        }

        val key = sbn.key ?: "${pkg}|${sbn.id}"
        val appName = resolveAppName(pkg)
        val extras = notification.extras

        val title = extras?.getCharSequence(Notification.EXTRA_TITLE)?.toString() ?: ""
        val text = extras?.getCharSequence(Notification.EXTRA_TEXT)?.toString() ?: ""
        val subText = extras?.getCharSequence(Notification.EXTRA_SUB_TEXT)?.toString() ?: ""
        val postTimeMs = sbn.postTime
        val isOngoing = (notification.flags and Notification.FLAG_ONGOING_EVENT) != 0
        val isClearable = sbn.isClearable
        val category = notification.category ?: ""

        AndroidNotificationManager.onNotificationPosted(
            key = key,
            packageName = pkg,
            appName = appName,
            title = title,
            text = text,
            subText = subText,
            postTimeMs = postTimeMs,
            isOngoing = isOngoing,
            isClearable = isClearable,
            category = category,
        )
    }

    private fun handleRemoved(sbn: StatusBarNotification, reason: Int) {
        val pkg = sbn.packageName ?: return

        // 1. Self-Notification Suppression
        if (pkg == packageName) {
            return
        }

        val key = sbn.key ?: "${pkg}|${sbn.id}"
        AndroidNotificationManager.onNotificationRemoved(
            key = key,
            packageName = pkg,
            reason = reason,
        )
    }

    private fun resolveAppName(packageName: String): String {
        appNameCache[packageName]?.let { return it }
        val name = try {
            val pm = packageManager
            val appInfo = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
                pm.getApplicationInfo(packageName, PackageManager.ApplicationInfoFlags.of(0))
            } else {
                @Suppress("DEPRECATION")
                pm.getApplicationInfo(packageName, 0)
            }
            pm.getApplicationLabel(appInfo).toString()
        } catch (t: Throwable) {
            packageName
        }
        appNameCache[packageName] = name
        return name
    }
}
