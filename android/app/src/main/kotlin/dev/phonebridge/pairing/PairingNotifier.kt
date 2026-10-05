package dev.phonebridge.pairing

import android.Manifest
import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Build
import android.util.Log
import androidx.core.app.NotificationCompat
import androidx.core.app.NotificationManagerCompat
import androidx.core.content.ContextCompat
import dev.phonebridge.signaling.LanSignalingServer

/**
 * PairingNotifier raises the local "Pairing Request" notification when another
 * device asks to pair while the app is not (or is) open.
 *
 * Rules that make this safe:
 * - The notification OPENS the pending request — it never accepts anything.
 *   The only decision surface is the in-app Pairing Request dialog.
 * - One fixed notification id: a newer request replaces the older one's
 *   notification, so a retry storm can never stack notifications. The native
 *   duplicate protection guarantees at most one pending request per peer.
 * - The SAS is deliberately NOT shown in the notification: notifications can
 *   surface on the lock screen. Compare codes inside the app.
 * - Names and counts only in logs — never tokens or SAS values.
 */
object PairingNotifier {

    private const val TAG = "PairingNotifier"
    private const val CHANNEL_ID = "phonebridge_pairing"
    private const val NOTIFICATION_ID = 4001

    const val EXTRA_PAIRING_TOKEN = "dev.phonebridge.extra.PAIRING_TOKEN"

    fun ensureChannel(context: Context) {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.O) return
        val manager = context.getSystemService(Context.NOTIFICATION_SERVICE) as? NotificationManager ?: return
        val channel = NotificationChannel(
            CHANNEL_ID,
            "Pairing requests",
            NotificationManager.IMPORTANCE_HIGH,
        )
        channel.description = "Requests from other devices that want to pair with this one"
        manager.createNotificationChannel(channel)
    }

    fun cancel(context: Context) {
        NotificationManagerCompat.from(context).cancel(NOTIFICATION_ID)
    }

    /**
     * Posts the Pairing Request notification. Best effort: a denied POST_NOTI-
     * FICATIONS permission or a disabled channel skips the notification (the
     * in-app dialog and the Devices tab remain the surfaces); nothing throws.
     */
    fun notifyPairingRequest(context: Context, info: LanSignalingServer.PairingRequestInfo) {
        ensureChannel(context)
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU &&
            ContextCompat.checkSelfPermission(context, Manifest.permission.POST_NOTIFICATIONS) !=
            PackageManager.PERMISSION_GRANTED
        ) {
            Log.i(TAG, "POST_NOTIFICATIONS not granted; skipping pairing notification")
            return
        }

        // Opens MainActivity (singleTop), which forwards the token to Flutter
        // and navigates to the Devices tab where the request is answered.
        // FLAG_UPDATE_CURRENT keeps one live PendingIntent; CANCEL_ON_..._
        // removes it once used so a stale tap cannot reopen an answered token.
        val intent = Intent(context, dev.phonebridge.ui.MainActivity::class.java).apply {
            flags = Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_SINGLE_TOP
            putExtra(EXTRA_PAIRING_TOKEN, info.token)
        }
        val pending = PendingIntent.getActivity(
            context,
            0,
            intent,
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
        )

        val notification: Notification = NotificationCompat.Builder(context, CHANNEL_ID)
            .setSmallIcon(android.R.drawable.stat_sys_warning)
            .setContentTitle("PhoneBridge — Pairing Request")
            .setContentText("${info.remoteName} wants to pair with this device. Tap to review.")
            .setStyle(
                NotificationCompat.BigTextStyle()
                    .bigText("${info.remoteName} wants to pair with this device. Tap to review the request and verify the pairing code."),
            )
            .setPriority(NotificationCompat.PRIORITY_HIGH)
            .setAutoCancel(true)
            .setContentIntent(pending)
            .build()

        try {
            NotificationManagerCompat.from(context).notify(NOTIFICATION_ID, notification)
        } catch (t: Throwable) {
            Log.w(TAG, "pairing notification failed: ${t.message}")
        }
    }
}
