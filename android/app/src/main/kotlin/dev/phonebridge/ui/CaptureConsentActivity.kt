package dev.phonebridge.ui

import android.app.Activity
import android.content.Context
import android.content.Intent
import android.graphics.Canvas
import android.graphics.Color
import android.graphics.Paint
import android.media.projection.MediaProjectionManager
import android.os.Bundle
import android.os.SystemClock
import android.util.Log
import android.view.View
import android.view.WindowManager
import android.widget.FrameLayout
import dev.phonebridge.capture.CaptureConfig
import dev.phonebridge.service.PhoneBridgeService
import java.util.Locale

/**
 * CaptureConsentActivity is the consent gateway required by Android 14+ / Android 15.
 *
 * It initiates the MediaProjection permission dialog and forwards the resulting
 * consent token to PhoneBridgeService via ACTION_START_CAPTURE.
 *
 * Supports optional animated test pattern (EXTRA_ANIMATE) to drive 120 fps panel refresh
 * for on-device GOP-tail throttling verification.
 */
class CaptureConsentActivity : Activity() {

    companion object {
        private const val TAG = "CaptureConsentActivity"
        private const val REQ_MEDIA_PROJECTION = 9001
        const val EXTRA_STOP = "dev.phonebridge.extra.STOP"
        const val EXTRA_ANIMATE = "dev.phonebridge.extra.ANIMATE"

        fun start(context: Context, config: CaptureConfig? = null, animate: Boolean = false) {
            val intent = Intent(context, CaptureConsentActivity::class.java).apply {
                addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
                if (config != null) {
                    putExtras(config.toBundle())
                }
                putExtra(EXTRA_ANIMATE, animate)
            }
            context.startActivity(intent)
        }

        fun stop(context: Context) {
            val intent = Intent(context, CaptureConsentActivity::class.java).apply {
                addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
                putExtra(EXTRA_STOP, true)
            }
            context.startActivity(intent)
        }
    }

    private var captureConfig = CaptureConfig()
    private var shouldAnimate = false
    private var contentView: AnimatedContentView? = null

    override fun onNewIntent(intent: Intent?) {
        super.onNewIntent(intent)
        if (intent?.getBooleanExtra(EXTRA_STOP, false) == true) {
            Log.i(TAG, "Stop request received via onNewIntent; stopping capture in PhoneBridgeService")
            PhoneBridgeService.stopCapture(this)
            finish()
        }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        if (intent?.getBooleanExtra(EXTRA_STOP, false) == true) {
            Log.i(TAG, "Stop request received; stopping capture in PhoneBridgeService")
            PhoneBridgeService.stopCapture(this)
            finish()
            return
        }

        captureConfig = CaptureConfig.fromIntent(intent)
        shouldAnimate = intent?.getBooleanExtra(EXTRA_ANIMATE, false) == true

        if (shouldAnimate) {
            window.addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON)
            val cv = AnimatedContentView(this)
            contentView = cv
            val root = FrameLayout(this).apply {
                addView(cv, FrameLayout.LayoutParams(-1, -1))
            }
            setContentView(root)
        }

        Log.i(TAG, "Requesting MediaProjection consent (animate=$shouldAnimate)...")
        val mpm = getSystemService(Context.MEDIA_PROJECTION_SERVICE) as MediaProjectionManager
        @Suppress("DEPRECATION")
        startActivityForResult(mpm.createScreenCaptureIntent(), REQ_MEDIA_PROJECTION)
    }

    @Deprecated("Deprecated in Java")
    override fun onActivityResult(requestCode: Int, resultCode: Int, data: Intent?) {
        super.onActivityResult(requestCode, resultCode, data)
        if (requestCode != REQ_MEDIA_PROJECTION) return

        if (resultCode == RESULT_OK && data != null) {
            Log.i(TAG, "MediaProjection consent granted (resultCode=$resultCode)")
            PhoneBridgeService.startCapture(this, resultCode, data, captureConfig)
            if (!shouldAnimate) {
                finish()
            }
        } else {
            Log.w(TAG, "MediaProjection consent denied (resultCode=$resultCode)")
            finish()
        }
    }

    /**
     * Deterministic high-motion animation derived from Spike 03/04:
     * Sweeps a vertical band across the display at panel refresh (~120 fps) to verify
     * hardware encoder behavior under continuous composition.
     */
    private class AnimatedContentView(ctx: Context) : View(ctx) {
        private val paint = Paint(Paint.ANTI_ALIAS_FLAG)
        private val start = SystemClock.elapsedRealtime()
        var drawnFrames = 0L

        override fun onDraw(canvas: Canvas) {
            val t = (SystemClock.elapsedRealtime() - start) / 1000f
            val w = width.toFloat()
            val h = height.toFloat()
            val hue = (t * 90f) % 360f
            canvas.drawColor(Color.HSVToColor(floatArrayOf(hue, 0.45f, 0.30f)))
            paint.color = Color.HSVToColor(floatArrayOf((hue + 180f) % 360f, 0.85f, 1f))
            val band = w / 5f
            val x = ((t * 320f) % (w + band)) - band
            canvas.drawRect(x, 0f, x + band, h, paint)
            paint.color = Color.WHITE
            paint.textSize = 40f
            canvas.drawText(
                String.format(Locale.US, "PhoneBridge Step 4 t=%.1fs drawn=%d", t, drawnFrames),
                32f,
                90f,
                paint
            )
            drawnFrames++
            postInvalidateOnAnimation()
        }
    }
}
