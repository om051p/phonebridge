package dev.phonebridge.spike03

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
 * Spike 03 host activity.
 *
 * Two roles:
 *  - consent gateway: starts the mediaProjection foreground service, then requests
 *    the screen-capture consent that only an Activity can obtain;
 *  - content generator: draws deterministic, high-motion content so the encoder
 *    always has real frames to encode, and exposes FLAG_SECURE / background probes.
 *
 * Scenarios are driven from the host over adb (see tools/spike03.sh).
 */
class MainActivity : Activity() {

    companion object {
        private const val REQ_CONSENT = 9001
        private const val REQ_NOTIFICATIONS = 9002
        const val DEFAULT_LABEL = "default-1080x2400"
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
                S3Log.i("SPIKE03_STATUS content ${if (freeze) "frozen" else "animating"}")
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
            scenario == "baseline" -> runBaseline(cfgIntent)
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
            putExtra(SessionConfig.E_WIDTH, 1080)
            putExtra(SessionConfig.E_HEIGHT, 2400)
            putExtra(SessionConfig.E_FPS, 30)
            putExtra(SessionConfig.E_SECONDS, 10)
            putExtra(SessionConfig.E_CYCLES, 5)
            putExtra(SessionConfig.E_SYNC_PROBE, true)
            putExtra(SessionConfig.E_SECURE_PROBE, true)
        }
        scenario = "session"
        label = DEFAULT_LABEL
        pendingConfig = i
        beginSession(i)
    }

    private fun setStatus(s: String) {
        status.text = s
        S3Log.i("SPIKE03_STATUS $s")
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
        S3Log.i("SPIKE03_STATE caps_start")
        Thread {
            try {
                val payload = linkedMapOf<String, Any?>(
                    "spike" to "03-android-mediaprojection-encoder",
                    "scenario" to "caps",
                    "generated_at" to Results.isoNow(),
                    "device" to Device.info(this),
                    "battery" to Device.battery(this),
                    "thermal" to ProcStats.thermal(),
                    "caps" to Caps.inventory(),
                )
                val path = Results.write(this, "spike03-caps-${Results.stamp()}.json", payload)
                S3Log.i("SPIKE03_RESULT_FILE $path")
                S3Log.i("SPIKE03_DONE caps $label ok")
                runOnUiThread { setStatus("caps written: $path") }
            } catch (t: Throwable) {
                S3Log.e("SPIKE03_ERROR caps", t)
                S3Log.i("SPIKE03_DONE caps $label failed")
            }
        }.start()
    }

    /**
     * Control run: the animated content + a live process, but no MediaProjection and
     * no encoder. Everything the capture run measures is compared against this.
     */
    private fun runBaseline(cfgIntent: Intent) {
        val seconds = cfgIntent.getIntExtra(SessionConfig.E_SECONDS, 10)
        val label = cfgIntent.getStringExtra(SessionConfig.E_LABEL) ?: "baseline"
        setStatus("baseline: animating ${seconds}s with no capture")
        S3Log.i("SPIKE03_STATE baseline_start $label")
        Thread {
            try {
                val cpuStart = ProcStats.appCpuMs()
                val devStart = ProcStats.deviceCpuJiffies()
                val pssStart = ProcStats.pss()
                val rssStart = ProcStats.rssKb()
                val t0 = ProcStats.clockMs()
                val buckets = mutableListOf<Map<String, Any?>>()
                var lastCpu = cpuStart
                var lastAt = t0
                var lastRss = rssStart ?: -1L
                while (ProcStats.clockMs() - t0 < seconds * 1000L) {
                    Thread.sleep(1000)
                    val now = ProcStats.clockMs()
                    val cpu = ProcStats.appCpuMs()
                    val rss = ProcStats.rssKb() ?: -1L
                    val sec = (now - lastAt) / 1000.0
                    buckets.add(
                        linkedMapOf<String, Any?>(
                            "t_ms" to (lastAt - t0),
                            "app_cpu_ms" to (cpu - lastCpu),
                            "app_cpu_pct" to (cpu - lastCpu) / (sec * 1000.0) * 100.0,
                            "rss_kb" to rss,
                            "rss_delta_kb" to rss - lastRss,
                        ),
                    )
                    lastCpu = cpu
                    lastAt = now
                    lastRss = rss
                }
                val elapsed = (ProcStats.clockMs() - t0) / 1000.0
                val cpuEnd = ProcStats.appCpuMs()
                val devEnd = ProcStats.deviceCpuJiffies()
                val payload = linkedMapOf<String, Any?>(
                    "spike" to "03-android-mediaprojection-encoder",
                    "scenario" to "baseline",
                    "status" to "ok",
                    "generated_at" to Results.isoNow(),
                    "device" to Device.info(this),
                    "config" to linkedMapOf<String, Any?>("label" to label, "seconds" to seconds),
                    "baseline" to linkedMapOf<String, Any?>(
                        "elapsed_s" to elapsed,
                        "app_cpu_ms" to (cpuEnd - cpuStart),
                        "app_cpu_pct_of_one_core" to (cpuEnd - cpuStart) / (elapsed * 1000.0) * 100.0,
                        "cores" to Runtime.getRuntime().availableProcessors(),
                        "device_busy_pct" to if (devStart != null && devEnd != null) {
                            val tot = devEnd.first - devStart.first
                            val busy = devEnd.second - devStart.second
                            if (tot > 0) busy * 100.0 / tot else null
                        } else null,
                        "memory" to linkedMapOf<String, Any?>(
                            "pss_before_kb" to pssStart,
                            "pss_after_kb" to ProcStats.pss(),
                            "rss_before_kb" to (rssStart ?: -1),
                            "rss_after_kb" to (ProcStats.rssKb() ?: -1),
                            "peak_rss_kb" to ProcStats.peakRssKb(),
                            "threads" to ProcStats.threads(),
                        ),
                        "thermal" to ProcStats.thermal(),
                        "buckets" to buckets,
                    ),
                    "notes" to listOf(
                        "Control run: 120Hz animated content on screen, no MediaProjection, no encoder.",
                        "Subtract this from capture runs to attribute CPU/memory to the capture pipeline.",
                    ),
                )
                val path = Results.write(this, "spike03-baseline-${Results.stamp()}.json", payload)
                S3Log.i("SPIKE03_RESULT_FILE $path")
                S3Log.i("SPIKE03_DONE baseline $label ok")
            } catch (t: Throwable) {
                S3Log.e("SPIKE03_ERROR baseline", t)
                S3Log.i("SPIKE03_DONE baseline $label failed")
            }
        }.start()
    }

    private fun beginSession(cfgIntent: Intent) {
        val landscape = cfgIntent.getBooleanExtra(SessionConfig.E_LANDSCAPE, false)
        requestedOrientation =
            if (landscape) ActivityInfo.SCREEN_ORIENTATION_LANDSCAPE else ActivityInfo.SCREEN_ORIENTATION_PORTRAIT
        S3Log.i("SPIKE03_STATE session_begin $scenario/$label")
        // ORDER MATTERS (API 35, targetSdk 35): the mediaProjection foreground service
        // may only be started AFTER consent grants the PROJECT_MEDIA appop, otherwise
        // startForeground() throws SecurityException. Older targets start it first.
        val fgsFirst = cfgIntent.getBooleanExtra("fgsBeforeConsent", false)
        if (fgsFirst) {
            setStatus("NEGATIVE PROBE: starting mediaProjection FGS before consent")
            try {
                CaptureService.prepare(this)
            } catch (t: Throwable) {
                S3Log.e("SPIKE03_PROBE fgs_before_consent threw", t)
            }
        }
        Handler(Looper.getMainLooper()).postDelayed({
            setStatus("requesting MediaProjection consent")
            // Pause the animated content while the system dialog is up: uiautomator
            // only dumps a hierarchy once the UI is idle, and our content never is.
            content.paused = true
            S3Log.i("SPIKE03_CONSENT_PROMPT")
            val mpm = getSystemService(MEDIA_PROJECTION_SERVICE) as MediaProjectionManager
            @Suppress("DEPRECATION")
            startActivityForResult(mpm.createScreenCaptureIntent(), REQ_CONSENT)
        }, 600)
    }

    @Deprecated("Deprecated in Java")
    override fun onActivityResult(requestCode: Int, resultCode: Int, data: Intent?) {
        super.onActivityResult(requestCode, resultCode, data)
        if (requestCode != REQ_CONSENT) return
        S3Log.i("SPIKE03_CONSENT_RESULT resultCode=$resultCode hasData=${data != null}")
        content.paused = false
        content.invalidate()
        val cfg = pendingConfig
        if (resultCode != RESULT_OK || data == null || cfg == null) {
            setStatus("consent denied")
            S3Log.i("SPIKE03_DONE $scenario $label consent_denied")
            return
        }
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
            canvas.drawText(String.format(Locale.US, "spike03 t=%.1fs drawn=%d", t, drawnFrames), 32f, 90f, paint)
            drawnFrames++
            if (!paused) postInvalidateOnAnimation()
        }
    }
}
