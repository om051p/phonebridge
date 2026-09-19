package dev.phonebridge.spike05

import android.app.ActivityManager
import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import android.os.Handler
import android.os.Looper
import android.os.SystemClock
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit

/**
 * Spike 05 probe engine — the subject under test.
 *
 * Every measurement goes through here so that foreground, background,
 * foreground-service and destroyed-process states exercise *identical* code
 * paths and differ only in the ambient app state. That is the whole point of
 * the spike: Android's clipboard policy is a function of app state, not of API.
 *
 * Result lines are emitted as `RESULT op=... label=... status=... k=v` on
 * logcat tag Spike05 and simultaneously appended to a JSON evidence file, so
 * the harness can read either the live stream or the durable artifact.
 */
object ClipboardProbe {

    /** App-state vocabulary. The state is asserted by the caller, then verified. */
    const val STATE_FOREGROUND = "foreground"
    const val STATE_BACKGROUND = "background"
    const val STATE_FGS = "foreground_service"
    const val STATE_UNKNOWN = "unknown"

    /** Result status vocabulary — deliberately small and unambiguous. */
    const val OK = "ok"              // operation completed and returned content
    const val EMPTY = "empty"        // API returned null / no clip (the Q+ background denial)
    const val DENIED = "denied"      // API threw a security/access exception
    const val ERROR = "error"        // unexpected exception
    const val SKIPPED = "skipped"    // precondition not met (e.g. listener never fired)

    private var appState: String = STATE_UNKNOWN

    fun setAppState(state: String) {
        appState = state
        S5Log.i("APPSTATE $state")
    }

    fun currentAppState(): String = appState

    private fun cm(ctx: Context): ClipboardManager =
        ctx.getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager

    /**
     * Reports whether the OS considers this process in the foreground right now.
     * Used to *verify* the asserted state rather than trusting the harness.
     */
    fun processImportance(ctx: Context): Map<String, Any?> = try {
        val am = ctx.getSystemService(Context.ACTIVITY_SERVICE) as ActivityManager
        val p = am.runningAppProcesses?.firstOrNull { it.pid == android.os.Process.myPid() }
        linkedMapOf(
            "importance" to (p?.importance ?: -1),
            "importance_name" to importanceName(p?.importance ?: -1),
            "is_foreground" to ((p?.importance ?: 99) <= ActivityManager.RunningAppProcessInfo.IMPORTANCE_FOREGROUND_SERVICE),
        )
    } catch (t: Throwable) {
        mapOf("error" to t.toString())
    }

    private fun importanceName(i: Int): String = when (i) {
        ActivityManager.RunningAppProcessInfo.IMPORTANCE_FOREGROUND -> "FOREGROUND"
        ActivityManager.RunningAppProcessInfo.IMPORTANCE_FOREGROUND_SERVICE -> "FOREGROUND_SERVICE"
        ActivityManager.RunningAppProcessInfo.IMPORTANCE_VISIBLE -> "VISIBLE"
        ActivityManager.RunningAppProcessInfo.IMPORTANCE_SERVICE -> "SERVICE"
        ActivityManager.RunningAppProcessInfo.IMPORTANCE_CACHED -> "CACHED"
        ActivityManager.RunningAppProcessInfo.IMPORTANCE_EMPTY -> "EMPTY"
        else -> "OTHER($i)"
    }

    // ------------------------------------------------------------------ reads

    /**
     * The primary probe. On Android 10+ a non-focused, non-IME app is expected
     * to receive `null` here even though the clipboard genuinely has content —
     * that is the restriction the whole product design hinges on.
     */
    fun readContent(ctx: Context, label: String): String {
        val t0 = SystemClock.elapsedRealtimeNanos()
        return try {
            val clip = cm(ctx).primaryClip
            val ms = (SystemClock.elapsedRealtimeNanos() - t0) / 1_000_000.0
            if (clip == null || clip.itemCount == 0) {
                S5Log.result("read_content", label, EMPTY, "ms" to f(ms), "state" to appState)
                EMPTY
            } else {
                val item = clip.getItemAt(0)
                val text = item.coerceToText(ctx)?.toString() ?: ""
                S5Log.result(
                    "read_content", label, OK,
                    "ms" to f(ms),
                    "state" to appState,
                    "items" to clip.itemCount,
                    "label_desc" to clip.description?.label,
                    "bytes" to text.toByteArray(Charsets.UTF_8).size,
                    "digest" to ProcStats.hashOf(text),
                    "preview" to preview(text),
                )
                OK
            }
        } catch (t: SecurityException) {
            S5Log.result("read_content", label, DENIED, "detail" to "${t.javaClass.simpleName}:${t.message}", "state" to appState)
            DENIED
        } catch (t: Throwable) {
            S5Log.result("read_content", label, ERROR, "detail" to "${t.javaClass.simpleName}:${t.message}", "state" to appState)
            ERROR
        }
    }

    /**
     * `getPrimaryClipDescription()` is the interesting middle ground: AOSP has
     * historically allowed the *description* (MIME types, no content) where the
     * content read is denied. If that holds, a background daemon can at least
     * learn "something changed and it was text" without seeing the payload.
     */
    fun readDescription(ctx: Context, label: String): String {
        val t0 = SystemClock.elapsedRealtimeNanos()
        return try {
            val d = cm(ctx).primaryClipDescription
            val ms = (SystemClock.elapsedRealtimeNanos() - t0) / 1_000_000.0
            if (d == null) {
                S5Log.result("read_description", label, EMPTY, "ms" to f(ms), "state" to appState)
                EMPTY
            } else {
                val mimes = (0 until d.mimeTypeCount).map { d.getMimeType(it) }
                S5Log.result(
                    "read_description", label, OK,
                    "ms" to f(ms),
                    "state" to appState,
                    "label_desc" to d.label,
                    "mimes" to mimes.joinToString(","),
                    "has_text" to d.hasMimeType("text/plain"),
                )
                OK
            }
        } catch (t: SecurityException) {
            S5Log.result("read_description", label, DENIED, "detail" to "${t.javaClass.simpleName}:${t.message}", "state" to appState)
            DENIED
        } catch (t: Throwable) {
            S5Log.result("read_description", label, ERROR, "detail" to "${t.javaClass.simpleName}:${t.message}", "state" to appState)
            ERROR
        }
    }

    /** `hasPrimaryClip()` — the cheapest liveness signal, if it survives in background. */
    fun hasClip(ctx: Context, label: String): String = try {
        val has = cm(ctx).hasPrimaryClip()
        S5Log.result("has_clip", label, OK, "has" to has, "state" to appState)
        OK
    } catch (t: Throwable) {
        S5Log.result("has_clip", label, ERROR, "detail" to "${t.javaClass.simpleName}:${t.message}", "state" to appState)
        ERROR
    }

    // ----------------------------------------------------------------- writes

    /**
     * Write probe. Background clipboard *write* is the second half of the
     * restriction question: AOSP blocks it from non-focused apps, and an OEM
     * build may differ again.
     */
    fun write(ctx: Context, label: String, text: String): String {
        val t0 = SystemClock.elapsedRealtimeNanos()
        return try {
            cm(ctx).setPrimaryClip(ClipData.newPlainText("spike05", text))
            val ms = (SystemClock.elapsedRealtimeNanos() - t0) / 1_000_000.0
            S5Log.result(
                "write", label, OK,
                "ms" to f(ms),
                "state" to appState,
                "bytes" to text.toByteArray(Charsets.UTF_8).size,
                "digest" to ProcStats.hashOf(text),
            )
            OK
        } catch (t: SecurityException) {
            S5Log.result("write", label, DENIED, "detail" to "${t.javaClass.simpleName}:${t.message}", "state" to appState)
            DENIED
        } catch (t: Throwable) {
            S5Log.result("write", label, ERROR, "detail" to "${t.javaClass.simpleName}:${t.message}", "state" to appState)
            ERROR
        }
    }

    /**
     * Write-then-read-back in the same call: distinguishes "the write silently
     * no-op'd" from "the write landed but we cannot read it back", which are
     * very different product outcomes.
     */
    fun writeReadback(ctx: Context, label: String, text: String): String {
        val w = write(ctx, "$label-write", text)
        if (w != OK) {
            S5Log.result("write_readback", label, w, "stage" to "write", "state" to appState)
            return w
        }
        val r = readContent(ctx, "$label-read")
        S5Log.result("write_readback", label, r, "stage" to "read", "state" to appState)
        return r
    }

    // ---------------------------------------------------------------- listener

    /**
     * Clipboard change listener reachability + detection latency.
     *
     * Registers an OnPrimaryClipChangedListener, waits for [expect] firings up
     * to [timeoutMs], and reports how many arrived and how long they took. A
     * listener that fires in the background is the difference between
     * "poll the clipboard" (banned) and "react to changes" (allowed).
     */
    fun listenerProbe(
        ctx: Context,
        label: String,
        expect: Int = 1,
        timeoutMs: Long = 3000,
    ): String {
        val latch = CountDownLatch(expect)
        val stamps = java.util.Collections.synchronizedList(mutableListOf<Long>())
        val started = SystemClock.elapsedRealtimeNanos()
        val listener = ClipboardManager.OnPrimaryClipChangedListener {
            stamps.add(SystemClock.elapsedRealtimeNanos())
            latch.countDown()
        }
        val cmgr = cm(ctx)
        return try {
            cmgr.addPrimaryClipChangedListener(listener)
            S5Log.result(
                "listener_register", label, OK,
                "state" to appState,
                "process_importance" to (processImportance(ctx)["importance_name"] ?: "?"),
            )
            val fired = latch.await(timeoutMs, TimeUnit.MILLISECONDS)
            val latencies = stamps.map { (it - started) / 1_000_000.0 }
            if (fired) {
                S5Log.result(
                    "listener_fire", label, OK,
                    "state" to appState,
                    "count" to stamps.size,
                    "first_ms" to f(latencies.first()),
                    "last_ms" to f(latencies.last()),
                )
                OK
            } else {
                S5Log.result(
                    "listener_fire", label, EMPTY,
                    "state" to appState,
                    "count" to stamps.size,
                    "timeout_ms" to timeoutMs,
                    "detail" to "no callback within timeout",
                )
                EMPTY
            }
        } catch (t: Throwable) {
            S5Log.result("listener_fire", label, ERROR, "detail" to "${t.javaClass.simpleName}:${t.message}", "state" to appState)
            ERROR
        } finally {
            try { cmgr.removePrimaryClipChangedListener(listener) } catch (t: Throwable) { /* best effort */ }
        }
    }

    /**
     * Full matrix sweep for one app state. This is the single call the host
     * script triggers per state; it exercises every operation so the resulting
     * matrix is complete rather than sampled.
     */
    fun runMatrix(ctx: Context, label: String, payload: String, iterations: Int) {
        val state = appState
        S5Log.result("matrix_begin", label, OK, "state" to state, "iterations" to iterations)
        val pssBefore = ProcStats.pss()
        val cpuBefore = ProcStats.appCpuMs()
        val t0 = SystemClock.elapsedRealtime()

        // 1. Description read (cheapest, historically least restricted).
        repeat(iterations) { readDescription(ctx, "$label-desc-$it") }

        // 2. Content read of whatever is already on the clipboard.
        repeat(iterations) { readContent(ctx, "$label-read-$it") }

        // 3. Liveness.
        hasClip(ctx, "$label-has")

        // 4. Write, then read back.
        repeat(iterations) { i ->
            writeReadback(ctx, "$label-wr-$i", "$payload-$state-$i")
        }

        // 5. Listener reachability, measured against a write we perform ourselves.
        listenerProbe(ctx, "$label-listener", expect = 1, timeoutMs = 3000)

        val elapsed = SystemClock.elapsedRealtime() - t0
        val cpuAfter = ProcStats.appCpuMs()
        S5Log.result(
            "matrix_end", label, OK,
            "state" to state,
            "elapsed_ms" to elapsed,
            "app_cpu_ms" to (cpuAfter - cpuBefore),
            "pss_before_kb" to (pssBefore["total_pss_kb"] ?: -1),
            "pss_after_kb" to (ProcStats.pss()["total_pss_kb"] ?: -1),
        )
    }

    private fun f(d: Double) = String.format(java.util.Locale.US, "%.3f", d)

    /** Short, escaped preview — never the full clipboard body. */
    private fun preview(s: String): String {
        val cut = if (s.length > 40) s.substring(0, 40) + "…" else s
        return "\"" + cut.replace("\\", "\\\\").replace("\n", "\\n").replace("\"", "\\\"") + "\""
    }

    /** Runs [block] on the main looper and blocks the caller until it returns. */
    fun onMain(block: () -> Unit) {
        if (Looper.myLooper() == Looper.getMainLooper()) { block(); return }
        val latch = CountDownLatch(1)
        Handler(Looper.getMainLooper()).post { try { block() } finally { latch.countDown() } }
        latch.await(5, TimeUnit.SECONDS)
    }
}
