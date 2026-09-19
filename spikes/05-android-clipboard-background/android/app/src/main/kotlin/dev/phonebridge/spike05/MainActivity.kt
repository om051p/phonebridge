package dev.phonebridge.spike05

import android.app.Activity
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Build
import android.os.Bundle
import android.os.SystemClock
import android.view.Gravity
import android.view.WindowManager
import android.widget.Button
import android.widget.FrameLayout
import android.widget.LinearLayout
import android.widget.TextView

/**
 * Spike 05 host activity: drives the probe scenarios and provides the
 * foreground state for the matrix.
 *
 * Scenarios (selected by the host script via `--es scenario <name>`):
 *   matrix       — run the full matrix while the activity is foreground
 *   background   — run the matrix from a backgrounded activity (no FGS)
 *   fgs          — start ProbeService and let it run the matrix
 *   listener     — listener-only probe, foreground
 *   lifecycle    — exercise onPause/onResume focus transitions with a listener
 *
 * Everything is also reachable from the on-screen button for interactive probing.
 */
class MainActivity : Activity() {

    companion object {
        private const val REQ_NOTIFICATIONS = 9005
        const val DEFAULT_PAYLOAD = "spike05-payload"
    }

    private lateinit var status: TextView

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        window.addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON)
        buildUi()
        UiHooks.backgroundMover = { runOnUiThread { moveTaskToBack(true) } }

        if (Build.VERSION.SDK_INT >= 33 &&
            checkSelfPermission(android.Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED
        ) {
            requestPermissions(arrayOf(android.Manifest.permission.POST_NOTIFICATIONS), REQ_NOTIFICATIONS)
        }

        val scenario = intent.getStringExtra(ProbeConfig.E_SCENARIO) ?: ""
        val label = intent.getStringExtra(ProbeConfig.E_LABEL) ?: "default"
        val payload = intent.getStringExtra(ProbeConfig.E_TEXT) ?: DEFAULT_PAYLOAD
        val iterations = intent.getIntExtra(ProbeConfig.E_ITERATIONS, 3)

        S5Log.i("ACTIVITY onCreate scenario=$scenario label=$label")
        S5Log.result("activity_create", label, ClipboardProbe.OK, "scenario" to scenario, "pid" to android.os.Process.myPid())

        dispatchScenario(scenario, label, payload, iterations)
    }

    /**
     * The harness reuses one activity instance (`singleTask`), so a second
     * `am start` arrives here rather than in onCreate. Without this the matrix
     * would only ever run on the first launch of the session.
     */
    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        setIntent(intent)
        val scenario = intent.getStringExtra(ProbeConfig.E_SCENARIO) ?: ""
        val label = intent.getStringExtra(ProbeConfig.E_LABEL) ?: "default"
        val payload = intent.getStringExtra(ProbeConfig.E_TEXT) ?: DEFAULT_PAYLOAD
        val iterations = intent.getIntExtra(ProbeConfig.E_ITERATIONS, 3)
        S5Log.i("ACTIVITY onNewIntent scenario=$scenario label=$label")
        S5Log.result("activity_intent", label, ClipboardProbe.OK, "scenario" to scenario)
        if (scenario.isNotEmpty()) dispatchScenario(scenario, label, payload, iterations)
    }

    private fun dispatchScenario(scenario: String, label: String, payload: String, iterations: Int) {
        when (scenario) {
            "matrix" -> runForegroundMatrix(label, payload, iterations)
            "background" -> runBackgroundMatrix(label, payload, iterations)
            "fgs" -> startFgs(label, payload, iterations, holdMs = 0L, autoBackground = false)
            "fgs_bg" -> startFgs(label, payload, iterations, holdMs = 0L, autoBackground = true)
            "watch" -> startWatch(label)
            "listener" -> runListenerOnly(label)
            "lifecycle" -> runLifecycleProbe(label, payload)
            else -> setStatus("idle — drive over adb with --es scenario matrix|background|fgs|listener|lifecycle")
        }
    }

    override fun onDestroy() {
        UiHooks.backgroundMover = null
        super.onDestroy()
    }

    override fun onResume() {
        super.onResume()
        // The activity regaining focus is itself an event worth recording:
        // it tells the harness exactly when foreground read permission returned.
        S5Log.result(
            "focus", "activity", ClipboardProbe.OK,
            "event" to "resume",
            "elapsed_ms" to SystemClock.elapsedRealtime(),
            "state" to ClipboardProbe.currentAppState(),
        )
    }

    override fun onPause() {
        S5Log.result(
            "focus", "activity", ClipboardProbe.OK,
            "event" to "pause",
            "elapsed_ms" to SystemClock.elapsedRealtime(),
            "state" to ClipboardProbe.currentAppState(),
        )
        super.onPause()
    }

    // ------------------------------------------------------------- scenarios

    private fun runForegroundMatrix(label: String, payload: String, iterations: Int) {
        setStatus("foreground matrix running…")
        ClipboardProbe.setAppState(ClipboardProbe.STATE_FOREGROUND)
        Thread {
            // Give the window a moment to actually gain focus before probing.
            Thread.sleep(800)
            ClipboardProbe.runMatrix(this, "$label-fg", payload, iterations)
            runOnUiThread { setStatus("foreground matrix done — see logcat Spike05") }
        }.start()
    }

    /**
     * The critical negative case: an activity that has been moved to the back.
     * No foreground service, so the process is cached — this is the state in
     * which AOSP denies clipboard access.
     */
    private fun runBackgroundMatrix(label: String, payload: String, iterations: Int) {
        setStatus("background matrix: will move to back in 1s…")
        Thread {
            Thread.sleep(1000)
            runOnUiThread { moveTaskToBack(true) }
            Thread.sleep(2000) // let the task actually settle in the background
            ClipboardProbe.setAppState(ClipboardProbe.STATE_BACKGROUND)
            ClipboardProbe.runMatrix(this, "$label-bg", payload, iterations)
        }.start()
    }

    private fun startFgs(label: String, payload: String, iterations: Int, holdMs: Long, autoBackground: Boolean) {
        setStatus("starting foreground service…")
        val i = Intent(this, ProbeService::class.java).apply {
            putExtra(ProbeConfig.E_LABEL, label)
            putExtra(ProbeConfig.E_TEXT, payload)
            putExtra(ProbeConfig.E_ITERATIONS, iterations)
            putExtra(ProbeConfig.E_HOLD_MS, holdMs)
            putExtra(ProbeConfig.E_AUTO_BACKGROUND, autoBackground)
        }
        if (Build.VERSION.SDK_INT >= 26) startForegroundService(i) else startService(i)
        S5Log.result("fgs_start", label, ClipboardProbe.OK, "hold_ms" to holdMs, "auto_background" to autoBackground)
    }

    private fun startWatch(label: String) {
        setStatus("starting FGS watcher…")
        val i = Intent(this, ProbeService::class.java).apply {
            action = ProbeService.ACTION_WATCH
            putExtra(ProbeConfig.E_LABEL, label)
            putExtra(ProbeConfig.E_HOLD_MS, 60000L)
            putExtra(ProbeConfig.E_AUTO_BACKGROUND, true)
        }
        if (Build.VERSION.SDK_INT >= 26) startForegroundService(i) else startService(i)
        S5Log.result("watch_start", label, ClipboardProbe.OK, "watch_ms" to 60000L)
    }

    private fun runListenerOnly(label: String) {
        setStatus("listener probe (foreground)…")
        ClipboardProbe.setAppState(ClipboardProbe.STATE_FOREGROUND)
        Thread {
            Thread.sleep(500)
            ClipboardProbe.listenerProbe(this, "$label-fg", expect = 1, timeoutMs = 3000)
        }.start()
    }

    /**
     * Focus-transition probe: register a listener, then background and
     * foreground the activity while the harness writes to the clipboard from
     * the peer app. Measures whether detection survives loss of focus.
     */
    private fun runLifecycleProbe(label: String, payload: String) {
        setStatus("lifecycle probe: backgrounding…")
        Thread {
            Thread.sleep(500)
            ClipboardProbe.setAppState(ClipboardProbe.STATE_FOREGROUND)
            ClipboardProbe.write(this, "$label-fg-seed", payload)
            runOnUiThread { moveTaskToBack(true) }
            Thread.sleep(2000)
            ClipboardProbe.setAppState(ClipboardProbe.STATE_BACKGROUND)
            S5Log.result("lifecycle", label, ClipboardProbe.OK, "phase" to "backgrounded")
            ClipboardProbe.listenerProbe(this, "$label-bg", expect = 1, timeoutMs = 4000)
            runOnUiThread {
                startActivity(Intent(this, MainActivity::class.java).apply {
                    addFlags(Intent.FLAG_ACTIVITY_REORDER_TO_FRONT)
                    putExtra(ProbeConfig.E_SCENARIO, "")
                })
            }
            Thread.sleep(1500)
            ClipboardProbe.setAppState(ClipboardProbe.STATE_FOREGROUND)
            S5Log.result("lifecycle", label, ClipboardProbe.OK, "phase" to "refocused")
            ClipboardProbe.readContent(this, "$label-refocus")
        }.start()
    }

    // -------------------------------------------------------------------- UI

    private fun buildUi() {
        val root = FrameLayout(this)
        status = TextView(this).apply {
            setTextColor(0xFF000000.toInt())
            textSize = 13f
            setPadding(24, 24, 24, 24)
            text = "Spike05 — idle"
        }
        root.addView(status, FrameLayout.LayoutParams(-1, -2).apply { gravity = Gravity.TOP })

        val run = Button(this).apply {
            text = "Run foreground matrix"
            setOnClickListener { runForegroundMatrix("manual", DEFAULT_PAYLOAD, 3) }
        }
        val back = Button(this).apply {
            text = "Background + matrix"
            setOnClickListener { runBackgroundMatrix("manual", DEFAULT_PAYLOAD, 3) }
        }
        val fgs = Button(this).apply {
            text = "Start FGS probe"
            setOnClickListener { startFgs("manual", DEFAULT_PAYLOAD, 3, 0L, false) }
        }
        val bar = LinearLayout(this).apply {
            orientation = LinearLayout.HORIZONTAL
            addView(run); addView(back); addView(fgs)
        }
        root.addView(bar, FrameLayout.LayoutParams(-2, -2).apply { gravity = Gravity.BOTTOM })
        setContentView(root)
    }

    private fun setStatus(s: String) = runOnUiThread {
        status.text = "Spike05 — $s"
        S5Log.i("STATUS $s")
    }
}
