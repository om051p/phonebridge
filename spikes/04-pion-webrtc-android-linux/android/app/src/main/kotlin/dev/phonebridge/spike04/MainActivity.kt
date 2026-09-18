package dev.phonebridge.spike04

import android.app.Activity
import android.content.Context
import android.content.Intent
import android.content.pm.ActivityInfo
import android.content.pm.PackageManager
import android.graphics.Canvas
import android.graphics.Color
import android.graphics.Paint
import android.media.projection.MediaProjectionManager
import android.os.Build
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.os.SystemClock
import android.view.Gravity
import android.view.View
import android.view.WindowManager
import android.widget.Button
import android.widget.FrameLayout
import android.widget.LinearLayout
import android.widget.TextView
import java.util.Locale

/**
 * Spike 04 host activity: consent gateway + deterministic animated content,
 * adapted from the Spike 03 pattern. Sessions are driven from the host over
 * adb (tools/spike04.sh); a manual "Run" button covers interactive probing.
 */
class MainActivity : Activity() {

    companion object {
        private const val REQ_CONSENT = 9004
        private const val REQ_NOTIFICATIONS = 9005
        const val DEFAULT_LABEL = "default-720x1600"
    }

    private lateinit var status: TextView
    private lateinit var content: ContentView
    private var pendingConfig: Intent? = null
    private var scenario: String = ""
    private var label: String = DEFAULT_LABEL

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        window.addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON)
        buildUi()
        UiHooks.secureSetter = { on -> runOnUiThread { applySecure(on) } }
        UiHooks.backgroundMover = { runOnUiThread { moveTaskToBack(true) } }
        UiHooks.contentFreezer = { freeze ->
            runOnUiThread {
                content.paused = freeze
                if (!freeze) content.invalidate()
                S4Log.i("SPIKE04_STATUS content ${if (freeze) "frozen" else "animating"}")
            }
        }

        if (Build.VERSION.SDK_INT >= 33 &&
            checkSelfPermission(android.Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED
        ) {
            requestPermissions(arrayOf(android.Manifest.permission.POST_NOTIFICATIONS), REQ_NOTIFICATIONS)
        }

        val cfgIntent = Intent(intent)
        scenario = cfgIntent.getStringExtra(SessionConfig.E_SCENARIO) ?: ""
        label = cfgIntent.getStringExtra(SessionConfig.E_LABEL) ?: DEFAULT_LABEL

        when {
            scenario == "caps" -> runCaps()
            scenario.isNotEmpty() -> {
                pendingConfig = cfgIntent
                beginSession(cfgIntent)
            }
            else -> setStatus("idle — tap Run for default session, or drive over adb")
        }
    }

    override fun onDestroy() {
        UiHooks.secureSetter = null
        UiHooks.backgroundMover = null
        UiHooks.contentFreezer = null
        super.onDestroy()
    }

    // ------------------------------------------------------------------- UI

    private fun buildUi() {
        val root = FrameLayout(this)
        content = ContentView(this)
        root.addView(content, FrameLayout.LayoutParams(-1, -1))

        status = TextView(this).apply {
            setTextColor(Color.WHITE)
            textSize = 12f
            setBackgroundColor(0x99000000.toInt())
            setPadding(16, 12, 16, 12)
        }
        root.addView(
            status,
            FrameLayout.LayoutParams(-2, -2).apply { gravity = Gravity.TOP or Gravity.START },
        )

        val runButton = Button(this).apply {
            text = "Run default session"
            setOnClickListener { runDefault() }
        }
        val bar = LinearLayout(this).apply { orientation = LinearLayout.HORIZONTAL; addView(runButton) }
        root.addView(
            bar,
            FrameLayout.LayoutParams(-2, -2).apply { gravity = Gravity.BOTTOM or Gravity.START },
        )
        setContentView(root)
    }

    private fun runDefault() {
        val i = Intent(this, MainActivity::class.java).apply {
            putExtra(SessionConfig.E_SCENARIO, "session")
            putExtra(SessionConfig.E_LABEL, DEFAULT_LABEL)
            putExtra(SessionConfig.E_WIDTH, 720)
            putExtra(SessionConfig.E_HEIGHT, 1600)
            putExtra(SessionConfig.E_FPS, 30)
            putExtra(SessionConfig.E_SECONDS, 10)
            putExtra(SessionConfig.E_SIGNALING, defaultSignalingUrl())
        }
        scenario = "session"
        label = DEFAULT_LABEL
        pendingConfig = i
        beginSession(i)
    }

    private fun defaultSignalingUrl(): String {
        // LAN host convention used by the driver; overridden per-run over adb.
        return "http://192.168.1.10:7804/offer"
    }

    private fun setStatus(s: String) {
        status.text = s
        S4Log.i("SPIKE04_STATUS $s")
    }

    private fun applySecure(on: Boolean) {
        if (on) {
            window.addFlags(WindowManager.LayoutParams.FLAG_SECURE)
        } else {
            window.clearFlags(WindowManager.LayoutParams.FLAG_SECURE)
        }
        setStatus(if (on) "FLAG_SECURE ON (expected: black in capture)" else "FLAG_SECURE OFF")
    }

    // --------------------------------------------------------------- flows

    private fun runCaps() {
        setStatus("enumerating encoders")
        S4Log.i("SPIKE04_STATE caps_start")
        Thread {
            try {
                val payload = linkedMapOf<String, Any?>(
                    "spike" to "04-pion-webrtc-android-linux",
                    "scenario" to "caps",
                    "generated_at" to Results.isoNow(),
                    "device" to Device.info(this),
                    "battery" to Device.battery(this),
                    "thermal" to ProcStats.thermal(),
                    "caps" to Caps.inventory(),
                )
                val path = Results.write(this, "spike04-caps-${Results.stamp()}.json", payload)
                S4Log.i("SPIKE04_RESULT_FILE $path")
                S4Log.i("SPIKE04_DONE caps $label ok")
                runOnUiThread { setStatus("caps written: $path") }
            } catch (t: Throwable) {
                S4Log.e("SPIKE04_ERROR caps", t)
                S4Log.i("SPIKE04_DONE caps $label failed")
            }
        }.start()
    }

    private fun beginSession(cfgIntent: Intent) {
        requestedOrientation = if (cfgIntent.getBooleanExtra(SessionConfig.E_LANDSCAPE, false)) {
            ActivityInfo.SCREEN_ORIENTATION_LANDSCAPE
        } else {
            ActivityInfo.SCREEN_ORIENTATION_PORTRAIT
        }
        S4Log.i("SPIKE04_STATE session_begin $scenario/$label")
        Handler(Looper.getMainLooper()).postDelayed({
            setStatus("requesting MediaProjection consent")
            // Pause animation while the system dialog is up so uiautomator can
            // dump an idle hierarchy (the consent automation depends on it).
            content.paused = true
            S4Log.i("SPIKE04_CONSENT_PROMPT")
            val mpm = getSystemService(MEDIA_PROJECTION_SERVICE) as MediaProjectionManager
            @Suppress("DEPRECATION")
            startActivityForResult(mpm.createScreenCaptureIntent(), REQ_CONSENT)
        }, 600)
    }

    @Deprecated("Deprecated in Java")
    override fun onActivityResult(requestCode: Int, resultCode: Int, data: Intent?) {
        super.onActivityResult(requestCode, resultCode, data)
        if (requestCode != REQ_CONSENT) return
        S4Log.i("SPIKE04_CONSENT_RESULT resultCode=$resultCode hasData=${data != null}")
        content.paused = false
        content.invalidate()
        val cfg = pendingConfig
        if (resultCode != RESULT_OK || data == null || cfg == null) {
            setStatus("consent denied")
            S4Log.i("SPIKE04_DONE $scenario $label consent_denied")
            return
        }
        // Android 14+ order (Spike 03 finding): consent FIRST, then FGS with
        // type mediaProjection — CaptureService.start goes foreground + captures.
        setStatus("consent granted — starting foreground service")
        CaptureService.start(this, cfg, resultCode, data)
        setStatus("capturing — do not lock or unplug")
    }

    // ------------------------------------------------------- content view

    /** Deterministic high-motion content: a sweeping band over a shifting background. */
    private class ContentView(ctx: Context) : View(ctx) {
        private val paint = Paint(Paint.ANTI_ALIAS_FLAG)
        private val start = SystemClock.elapsedRealtime()
        var drawnFrames = 0L

        /** Paused while a system dialog needs an idle UI hierarchy. */
        @Volatile var paused = false

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
            canvas.drawText(String.format(Locale.US, "spike04 t=%.1fs drawn=%d", t, drawnFrames), 32f, 90f, paint)
            drawnFrames++
            if (!paused) postInvalidateOnAnimation()
        }
    }
}
