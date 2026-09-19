package dev.phonebridge.spike05ime

import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import android.inputmethodservice.InputMethodService
import android.os.Handler
import android.os.Looper
import android.os.SystemClock
import android.util.Log
import android.view.View
import android.widget.Button
import android.widget.LinearLayout
import android.widget.TextView

/**
 * Spike 05 minimal companion IME.
 *
 * Purpose: measure whether the *input-method* clipboard exemption survives on
 * HyperOS. On AOSP, the focused IME is allowed to read the clipboard even though
 * an ordinary background app is not — which makes an IME the only sanctioned
 * mechanism for reading another app's clipboard without stealing focus.
 *
 * This IME is deliberately minimal: a small view with explicit probe buttons,
 * plus an optional auto-probe that runs on every clipboard change. It is NOT a
 * product IME (no keyboard, no text input, no user-facing value) and exists only
 * to answer the spike's question.
 *
 * Evidence lines are emitted on tag Spike05Ime.
 */
class ProbeImeService : InputMethodService() {

    companion object {
        const val TAG = "Spike05Ime"

        @Volatile var active: Boolean = false
            private set

        /** When true, probe on every clipboard change while the IME is shown. */
        @Volatile var autoProbe: Boolean = true

        /**
         * Period between periodic probes, in ms. 0 disables the periodic probe
         * entirely — which is the *production* shape (event-driven only, via the
         * change listener). The periodic probe exists so the spike can sample
         * the read state; it is not something a product IME would do.
         */
        @Volatile var periodicMs: Long = 0

        /**
         * Write a payload from the IME's own process while the IME is *not*
         * shown. This is the Linux -> Android sync direction; the harness reads
         * it back from a different app to prove the write landed.
         */
        fun writeFromIme(ctx: Context, text: String) {
            try {
                val cm = ctx.getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
                cm.setPrimaryClip(ClipData.newPlainText("spike05ime", text))
                lastWrittenDigest = text.hashCode()
                Log.i(
                    TAG,
                    "RESULT op=ime_write status=ok bytes=${text.toByteArray(Charsets.UTF_8).size} " +
                        "digest=${Integer.toHexString(text.hashCode())}",
                )
            } catch (t: Throwable) {
                Log.e(TAG, "RESULT op=ime_write status=error detail=${t.message}")
            }
        }

        /** True when [text] is the payload this process last wrote (an echo). */
        fun isOwnWrite(text: String): Boolean = text.hashCode() == lastWrittenDigest

        /**
         * Echo mode: on every clipboard change, write the content straight back.
         * This is the naive sync implementation and it exists solely to measure
         * whether it oscillates. The production design must NOT do this; the
         * spike needs to know how bad the failure mode actually is.
         */
        @Volatile var echoMode: Boolean = false

        /**
         * When true, echo mode suppresses a change whose digest equals the last
         * payload this process wrote. This is the candidate loop-prevention rule
         * and the spike must prove it actually terminates the oscillation.
         */
        @Volatile var echoSuppress: Boolean = false

        /** Digest of the last payload written by this process. */
        @Volatile private var lastWrittenDigest: Int = 0
    }

    private lateinit var status: TextView
    private var listener: ClipboardManager.OnPrimaryClipChangedListener? = null
    private var changeCount = 0
    private val handler = Handler(Looper.getMainLooper())
    private val periodic = object : Runnable {
        override fun run() {
            probe("periodic")
            if (periodicMs > 0) handler.postDelayed(this, periodicMs)
        }
    }

    private fun cm(): ClipboardManager =
        getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager

    override fun onCreate() {
        super.onCreate()
        active = true
        Log.i(TAG, "IME onCreate pid=${android.os.Process.myPid()}")
        registerListener()
        // Start probing from the moment the service exists, NOT from the moment
        // the keyboard is shown. The product-relevant question is whether a
        // *selected but not shown* IME can still observe the clipboard; if the
        // probe only started on onStartInputView we could never answer it.
        if (periodicMs > 0) handler.postDelayed(periodic, periodicMs)
    }

    override fun onDestroy() {
        active = false
        handler.removeCallbacks(periodic)
        unregisterListener()
        Log.i(TAG, "IME onDestroy")
        super.onDestroy()
    }

    override fun onCreateInputView(): View {
        Log.i(TAG, "IME onCreateInputView")
        val root = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            setBackgroundColor(0xFF202020.toInt())
            setPadding(16, 16, 16, 16)
        }
        status = TextView(this).apply {
            setTextColor(0xFFFFFFFF.toInt())
            textSize = 12f
            text = "Spike05 IME — probe ready"
        }
        root.addView(status)
        root.addView(Button(this).apply {
            text = "Read clipboard now"
            setOnClickListener { probe("manual") }
        })
        root.addView(Button(this).apply {
            text = "Write marker"
            setOnClickListener { writeMarker() }
        })
        return root
    }

    override fun onStartInputView(info: android.view.inputmethod.EditorInfo?, restarting: Boolean) {
        super.onStartInputView(info, restarting)
        // The moment of truth: the IME has just been given input focus while
        // another app owns the window. Can we read the clipboard?
        Log.i(TAG, "IME onStartInputView restarting=$restarting pkg=${info?.packageName}")
        Handler(Looper.getMainLooper()).postDelayed({ probe("on_start_input") }, 300)
        handler.removeCallbacks(periodic)
        if (periodicMs > 0) handler.postDelayed(periodic, periodicMs)
    }

    override fun onFinishInputView(finishingInput: Boolean) {
        // Deliberately do NOT cancel the periodic probe: the spike must measure
        // whether the exemption is tied to the IME being *shown* or merely to the
        // process being the selected input method. The probe continues and
        // reports its own importance, which is the discriminating variable.
        Log.i(TAG, "IME onFinishInputView finishing=$finishingInput")
        super.onFinishInputView(finishingInput)
    }

    override fun onWindowHidden() {
        Log.i(TAG, "IME onWindowHidden")
        super.onWindowHidden()
    }

    override fun onWindowShown() {
        Log.i(TAG, "IME onWindowShown")
        super.onWindowShown()
    }

    private fun registerListener() {
        val l = ClipboardManager.OnPrimaryClipChangedListener {
            changeCount++
            Log.i(TAG, "RESULT op=ime_change n=$changeCount")
            if (echoMode) {
                // Naive sync: write back whatever arrived. If Android re-notifies
                // on a write from this same process, this oscillates forever.
                val text = try { cm().primaryClip?.getItemAt(0)?.coerceToText(this)?.toString() } catch (t: Throwable) { null }
                if (text != null) {
                    if (echoSuppress && isOwnWrite(text)) {
                        Log.i(TAG, "RESULT op=ime_echo_suppressed n=$changeCount")
                    } else {
                        writeFromIme(this, text)
                    }
                }
            }
            if (autoProbe) probe("on_change_$changeCount")
        }
        try {
            cm().addPrimaryClipChangedListener(l)
            listener = l
            Log.i(TAG, "RESULT op=ime_listener_register status=ok")
        } catch (t: Throwable) {
            Log.e(TAG, "RESULT op=ime_listener_register status=error detail=${t.message}")
        }
    }

    private fun unregisterListener() {
        listener?.let { try { cm().removePrimaryClipChangedListener(it) } catch (t: Throwable) { } }
        listener = null
    }

    private fun probe(reason: String) {
        val t0 = SystemClock.elapsedRealtimeNanos()
        val imp = importanceName()
        try {
            val clip = cm().primaryClip
            val ms = (SystemClock.elapsedRealtimeNanos() - t0) / 1_000_000.0
            if (clip == null || clip.itemCount == 0) {
                Log.i(TAG, "RESULT op=ime_read reason=$reason status=empty imp=$imp ms=${fmt(ms)}")
            } else {
                val text = clip.getItemAt(0).coerceToText(this)?.toString() ?: ""
                Log.i(
                    TAG,
                    "RESULT op=ime_read reason=$reason status=ok imp=$imp ms=${fmt(ms)} " +
                        "items=${clip.itemCount} desc=${clip.description?.label} " +
                        "bytes=${text.toByteArray(Charsets.UTF_8).size} digest=${Integer.toHexString(text.hashCode())}",
                )
            }
        } catch (t: SecurityException) {
            Log.e(TAG, "RESULT op=ime_read reason=$reason status=denied imp=$imp detail=${t.message}")
        } catch (t: Throwable) {
            Log.e(TAG, "RESULT op=ime_read reason=$reason status=error imp=$imp detail=${t.message}")
        }
        Handler(Looper.getMainLooper()).post {
            if (::status.isInitialized) status.text = "Spike05 IME — probed ($reason)"
        }
    }

    /**
     * The IME's own process importance. A *shown* IME is IMPORTANCE_FOREGROUND;
     * once hidden it degrades — and that transition is the difference between
     * "the exemption is about being the IME" and "the exemption is about being
     * visible", which the spike must distinguish.
     */
    private fun importanceName(): String = try {
        val am = getSystemService(Context.ACTIVITY_SERVICE) as android.app.ActivityManager
        val info = android.app.ActivityManager.RunningAppProcessInfo()
        android.app.ActivityManager.getMyMemoryState(info)
        val imp = info.importance
        when (imp) {
            android.app.ActivityManager.RunningAppProcessInfo.IMPORTANCE_FOREGROUND -> "FOREGROUND"
            android.app.ActivityManager.RunningAppProcessInfo.IMPORTANCE_FOREGROUND_SERVICE -> "FOREGROUND_SERVICE"
            android.app.ActivityManager.RunningAppProcessInfo.IMPORTANCE_VISIBLE -> "VISIBLE"
            android.app.ActivityManager.RunningAppProcessInfo.IMPORTANCE_SERVICE -> "SERVICE"
            android.app.ActivityManager.RunningAppProcessInfo.IMPORTANCE_CACHED -> "CACHED"
            else -> imp.toString()
        }
    } catch (t: Throwable) {
        "unknown"
    }

    private fun writeMarker() {
        try {
            val v = "ime-marker-${System.currentTimeMillis()}"
            cm().setPrimaryClip(ClipData.newPlainText("spike05ime", v))
            Log.i(TAG, "RESULT op=ime_write status=ok digest=${Integer.toHexString(v.hashCode())}")
        } catch (t: Throwable) {
            Log.e(TAG, "RESULT op=ime_write status=error detail=${t.message}")
        }
    }

    private fun fmt(d: Double) = String.format(java.util.Locale.US, "%.3f", d)
}
