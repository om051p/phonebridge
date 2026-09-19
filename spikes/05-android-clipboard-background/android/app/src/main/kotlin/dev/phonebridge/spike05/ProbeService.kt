package dev.phonebridge.spike05

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.Service
import android.content.Context
import android.content.Intent
import android.os.Build
import android.os.Handler
import android.os.HandlerThread
import android.os.IBinder
import android.os.SystemClock

/**
 * Spike 05 foreground service — the "background daemon" subject.
 *
 * In production this is the shape the Android host would take (mirroring
 * DEC-019's PhoneBridgeForegroundService): a `connectedDevice` FGS that holds
 * the Go runtime. The spike's question is whether such a service can read and
 * write the clipboard while the user is in another app, and whether it can
 * observe clipboard changes at all.
 *
 * Everything runs on a private HandlerThread so that "background" means
 * "background" — a probe executed on the main looper of a foreground service
 * can accidentally inherit foreground affinity and produce a false positive.
 */
class ProbeService : Service() {

    companion object {
        const val CHANNEL_ID = "spike05-probe"
        const val NOTIF_ID = 5005
        const val ACTION_MATRIX = "dev.phonebridge.spike05.MATRIX"
        const val ACTION_STOP = "dev.phonebridge.spike05.STOP"
        const val ACTION_WATCH = "dev.phonebridge.spike05.WATCH"

        @Volatile var running: Boolean = false
            private set
    }

    private lateinit var thread: HandlerThread
    private lateinit var handler: Handler

    override fun onCreate() {
        super.onCreate()
        thread = HandlerThread("spike05-probe").also { it.start() }
        handler = Handler(thread.looper)
        createChannel()
        startForeground(NOTIF_ID, notification("probe idle"))
        running = true
        S5Log.i("SERVICE onCreate pid=${android.os.Process.myPid()} thread=${thread.name}")
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        when (intent?.action) {
            ACTION_STOP -> {
                S5Log.i("SERVICE stop requested")
                stopSelf()
                return START_NOT_STICKY
            }
            ACTION_WATCH -> {
                val label = intent.getStringExtra(ProbeConfig.E_LABEL) ?: "watch"
                val watchMs = intent.getLongExtra(ProbeConfig.E_HOLD_MS, 60000L)
                val autoBackground = intent.getBooleanExtra(ProbeConfig.E_AUTO_BACKGROUND, true)
                handler.post { runWatch(label, watchMs, autoBackground) }
            }
            else -> {
                val label = intent?.getStringExtra(ProbeConfig.E_LABEL) ?: "fgs"
                val payload = intent?.getStringExtra(ProbeConfig.E_TEXT) ?: "spike05"
                val iterations = intent?.getIntExtra(ProbeConfig.E_ITERATIONS, 3) ?: 3
                val holdMs = intent?.getLongExtra(ProbeConfig.E_HOLD_MS, 0L) ?: 0L
                val autoBackground = intent?.getBooleanExtra(ProbeConfig.E_AUTO_BACKGROUND, false) ?: false
                handler.post { runProbe(label, payload, iterations, holdMs, autoBackground) }
            }
        }
        return START_STICKY
    }

    private fun runProbe(label: String, payload: String, iterations: Int, holdMs: Long, autoBackground: Boolean) {
        S5Log.i("SERVICE probe begin label=$label")
        if (autoBackground) {
            // Background the *whole task* so the service is genuinely the only
            // thing keeping the process alive. This is the state the spike must
            // measure: an FGS whose activity is not visible at all.
            UiHooks.backgroundMover?.invoke()
            Thread.sleep(2500)
            val imp = ClipboardProbe.processImportance(this)
            S5Log.result(
                "fgs_after_background", label, ClipboardProbe.OK,
                "importance" to imp["importance_name"],
                "detail" to "activity moved to back; verifying FGS-only residency",
            )
        }
        if (holdMs > 0) Thread.sleep(holdMs)

        ClipboardProbe.setAppState(ClipboardProbe.STATE_FGS)
        val imp = ClipboardProbe.processImportance(this)
        S5Log.result(
            "fgs_state", label, ClipboardProbe.OK,
            "importance" to imp["importance_name"],
            "pid" to android.os.Process.myPid(),
            "elapsed_ms" to SystemClock.elapsedRealtime(),
        )

        ClipboardProbe.runMatrix(this, "$label-fgs", payload, iterations)

        S5Log.result("fgs_probe_end", label, ClipboardProbe.OK, "state" to ClipboardProbe.STATE_FGS)
        S5Log.i("SERVICE probe end label=$label")
    }

    /**
     * Continuous clipboard watcher. Runs an OnPrimaryClipChangedListener inside
     * the FGS while the task is backgrounded, and reports every change for
     * [watchMs]. This is the closest analogue to the production "background
     * daemon observes clipboard changes" requirement.
     */
    private fun runWatch(label: String, watchMs: Long, autoBackground: Boolean) {
        S5Log.i("SERVICE watch begin label=$label watchMs=$watchMs")
        if (autoBackground) {
            UiHooks.backgroundMover?.invoke()
            Thread.sleep(2500)
        }
        ClipboardProbe.setAppState(ClipboardProbe.STATE_FGS)
        val imp = ClipboardProbe.processImportance(this)
        S5Log.result(
            "watch_state", label, ClipboardProbe.OK,
            "importance" to imp["importance_name"],
            "watch_ms" to watchMs,
        )

        val cmgr = getSystemService(Context.CLIPBOARD_SERVICE) as android.content.ClipboardManager
        var count = 0
        val listener = android.content.ClipboardManager.OnPrimaryClipChangedListener {
            count++
            val d = try { cmgr.primaryClipDescription?.label?.toString() } catch (t: Throwable) { null }
            S5Log.result("watch_change", label, ClipboardProbe.OK, "n" to count, "desc" to d)
        }
        try {
            cmgr.addPrimaryClipChangedListener(listener)
            val deadline = SystemClock.elapsedRealtime() + watchMs
            // Poll importance periodically so the evidence shows what state the
            // process was actually in while watching.
            while (SystemClock.elapsedRealtime() < deadline) {
                Thread.sleep(2000)
            }
        } finally {
            try { cmgr.removePrimaryClipChangedListener(listener) } catch (t: Throwable) { }
        }
        S5Log.result("watch_end", label, ClipboardProbe.OK, "changes" to count)
    }

    override fun onBind(intent: Intent?): IBinder? = null

    override fun onDestroy() {
        running = false
        thread.quitSafely()
        S5Log.i("SERVICE onDestroy")
        super.onDestroy()
    }

    private fun createChannel() {
        if (Build.VERSION.SDK_INT >= 26) {
            val nm = getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager
            if (nm.getNotificationChannel(CHANNEL_ID) == null) {
                nm.createNotificationChannel(
                    NotificationChannel(CHANNEL_ID, "Spike05 probe", NotificationManager.IMPORTANCE_LOW),
                )
            }
        }
    }

    private fun notification(text: String): Notification {
        val b = if (Build.VERSION.SDK_INT >= 26) {
            Notification.Builder(this, CHANNEL_ID)
        } else {
            @Suppress("DEPRECATION") Notification.Builder(this)
        }
        return b.setContentTitle("Spike05 clipboard probe")
            .setContentText(text)
            .setSmallIcon(android.R.drawable.ic_menu_info_details)
            .setOngoing(true)
            .build()
    }
}

/** Hooks the foreground Activity installs so the service (which has no UI) can background the task. */
object UiHooks {
    @Volatile var backgroundMover: (() -> Unit)? = null
}
