package dev.phonebridge.spike03

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.Service
import android.content.Context
import android.content.Intent
import android.content.pm.ServiceInfo
import android.os.Build
import android.os.Handler
import android.os.HandlerThread
import android.os.IBinder

/**
 * Foreground service (type mediaProjection) that owns the capture session.
 *
 * Android 14+ ordering requirement implemented here:
 *   1. start foreground service with type mediaProjection
 *   2. request consent (Activity)
 *   3. getMediaProjection() + registerCallback()
 *   4. createVirtualDisplay()
 */
class CaptureService : Service() {

    companion object {
        const val ACTION_PREPARE = "dev.phonebridge.spike03.PREPARE"
        const val ACTION_CAPTURE = "dev.phonebridge.spike03.CAPTURE"
        const val EXTRA_RESULT_CODE = "resultCode"
        const val EXTRA_RESULT_DATA = "resultData"
        const val CHANNEL_ID = "spike03-capture"
        const val NOTIFICATION_ID = 3003

        fun prepare(ctx: Context) {
            ctx.startForegroundService(Intent(ctx, CaptureService::class.java).setAction(ACTION_PREPARE))
        }

        fun start(ctx: Context, configIntent: Intent, resultCode: Int, data: Intent) {
            val i = Intent(ctx, CaptureService::class.java).setAction(ACTION_CAPTURE)
            i.putExtras(configIntent)
            i.putExtra(EXTRA_RESULT_CODE, resultCode)
            i.putExtra(EXTRA_RESULT_DATA, data)
            ctx.startService(i)
        }
    }

    private lateinit var workerThread: HandlerThread
    private lateinit var handler: Handler

    override fun onCreate() {
        super.onCreate()
        workerThread = HandlerThread("spike03-worker").also { it.start() }
        handler = Handler(workerThread.looper)
        createChannel()
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        when (intent?.action) {
            ACTION_PREPARE -> {
                goForeground("Preparing capture")
                S3Log.i("SPIKE03_STATE service_foreground_prepare")
            }
            ACTION_CAPTURE -> {
                goForeground("Capturing")
                val rc = intent.getIntExtra(EXTRA_RESULT_CODE, 0)
                val data: Intent? = if (Build.VERSION.SDK_INT >= 33) {
                    intent.getParcelableExtra(EXTRA_RESULT_DATA, Intent::class.java)
                } else {
                    @Suppress("DEPRECATION") intent.getParcelableExtra(EXTRA_RESULT_DATA)
                }
                S3Log.i("SPIKE03_STATE service_capture_start")
                runCapture(intent, rc, data)
            }
            else -> S3Log.w("SPIKE03_STATE service_unknown_action=${intent?.action}")
        }
        return START_NOT_STICKY
    }

    override fun onDestroy() {
        workerThread.quitSafely()
        super.onDestroy()
    }

    override fun onBind(intent: Intent?): IBinder? = null

    private fun goForeground(text: String) {
        val n = Notification.Builder(this, CHANNEL_ID)
            .setContentTitle("Spike 03 — MediaProjection capture")
            .setContentText(text)
            .setSmallIcon(android.R.drawable.ic_menu_camera)
            .setOngoing(true)
            .build()
        if (Build.VERSION.SDK_INT >= 29) {
            startForeground(NOTIFICATION_ID, n, ServiceInfo.FOREGROUND_SERVICE_TYPE_MEDIA_PROJECTION)
        } else {
            startForeground(NOTIFICATION_ID, n)
        }
    }

    private fun createChannel() {
        if (Build.VERSION.SDK_INT >= 26) {
            val mgr = getSystemService(NotificationManager::class.java)
            mgr?.createNotificationChannel(
                NotificationChannel(CHANNEL_ID, "Spike 03 capture", NotificationManager.IMPORTANCE_LOW),
            )
        }
    }

    private fun runCapture(configIntent: Intent, resultCode: Int, data: Intent?) {
        val cfg = SessionConfig.fromIntent(configIntent)
        if (data == null) {
            S3Log.e("SPIKE03_ERROR consent result data missing")
            S3Log.i("SPIKE03_DONE ${cfg.scenario} ${cfg.label} consent_data_missing")
            finishSelf()
            return
        }
        Thread {
            val session = Session(applicationContext, cfg, handler)
            var status = "unknown"
            try {
                if (!session.attachProjection(resultCode, data)) {
                    status = "projection_failed"
                } else {
                    val result = session.run()
                    status = (result["status"] as? String) ?: "unknown"
                }
            } catch (t: Throwable) {
                status = "exception"
                S3Log.e("SPIKE03_ERROR unhandled session failure", t)
            } finally {
                try {
                    val path = Results.write(
                        applicationContext,
                        "spike03-${cfg.scenario}-${cfg.label}-${Results.stamp()}.json",
                        session.build(status),
                    )
                    S3Log.i("SPIKE03_RESULT_FILE $path")
                } catch (t: Throwable) {
                    S3Log.e("SPIKE03_ERROR write_result", t)
                }
                S3Log.i("SPIKE03_DONE ${cfg.scenario} ${cfg.label} $status")
                finishSelf()
            }
        }.start()
    }

    private fun finishSelf() {
        handler.post {
            try {
                stopForeground(STOP_FOREGROUND_REMOVE)
            } catch (t: Throwable) {
                S3Log.w("stopForeground failed: $t")
            }
            stopSelf()
        }
    }
}
