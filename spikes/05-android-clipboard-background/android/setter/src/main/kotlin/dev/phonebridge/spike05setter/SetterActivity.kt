package dev.phonebridge.spike05setter

import android.app.Activity
import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import android.content.Intent
import android.os.Bundle
import android.util.Log
import android.view.WindowManager
import android.view.inputmethod.InputMethodManager
import android.widget.EditText
import android.widget.LinearLayout
import android.widget.TextView

/**
 * Spike 05 peer app (distinct applicationId).
 *
 * Driven over adb so the harness can make a *different* process the clipboard
 * owner, and can move focus to/from a different app:
 *
 *   write: am start -n dev.phonebridge.spike05setter/.SetterActivity \
 *            --es op write --es label <label> --es text <payload>
 *   read:  am start -n dev.phonebridge.spike05setter/.SetterActivity \
 *            --es op read  --es label <label>
 *
 * Results are emitted as logcat lines tagged Spike05Setter so the harness can
 * parse them with `adb logcat -d -s Spike05Setter`. This app is a *peer*, not
 * the subject under test; it never runs in the background on purpose.
 */
class SetterActivity : Activity() {

    companion object {
        const val TAG = "Spike05Setter"
        const val E_OP = "op"
        const val E_LABEL = "label"
        const val E_TEXT = "text"
        const val E_AUTO_FINISH = "auto_finish"
        const val E_SIZE = "size"
        const val E_URI = "uri"
        const val E_HTML = "html"
        const val E_ITEMS = "items"

        /**
         * Default HTML payload generated in-process. Passing markup through
         * `am start --es` is unreliable — the device shell mangles angle
         * brackets, so an HTML write sent that way silently never happens and
         * would look like a platform refusal.
         */
        const val DEFAULT_HTML = "<b>Bold</b> and <i>italic</i>"
    }

    /**
     * Deterministic in-process payload of [n] chars. It is a repeating pattern
     * rather than a single repeated char so that a truncated or partially
     * delivered clip produces a different digest instead of a plausible one.
     */
    private fun payloadOf(n: Int): String {
        val sb = StringBuilder(n)
        var i = 0
        while (sb.length < n) {
            sb.append((('a'.code + (i % 26))).toChar())
            i++
        }
        return sb.toString()
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        val op = intent.getStringExtra(E_OP) ?: "noop"
        val label = intent.getStringExtra(E_LABEL) ?: "unlabelled"

        // The IME experiment needs a *focused editable* view owned by this app so
        // the input method is actually shown over a process other than its own.
        if (op == "edit") {
            showEditTarget(label)
            return
        }

        val tv = TextView(this).apply {
            text = "Spike05Setter\nop=$op\nlabel=$label\n(package=${packageName})"
            textSize = 14f
            setPadding(24, 24, 24, 24)
        }
        setContentView(tv)

        // CRITICAL: the clipboard op must run *after* this window has input
        // focus. Running it in onCreate races the focus grant and produces a
        // spurious "empty" read even on a healthy foreground app — which would
        // look exactly like a platform restriction and corrupt the matrix.
        window.decorView.post {
            runOp(op, label)
            if (intent.getBooleanExtra(E_AUTO_FINISH, false)) {
                window.decorView.postDelayed({ finish() }, 600)
            }
        }
    }

    /** singleTask: repeated `am start` reuses this instance, so re-run the op. */
    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        setIntent(intent)
        val op = intent.getStringExtra(E_OP) ?: "noop"
        val label = intent.getStringExtra(E_LABEL) ?: "unlabelled"
        Log.i(TAG, "RESULT op=new_intent label=$label op_name=$op")
        if (op == "edit") {
            showEditTarget(label)
            return
        }
        window.decorView.post {
            runOp(op, label)
            if (intent.getBooleanExtra(E_AUTO_FINISH, false)) {
                window.decorView.postDelayed({ finish() }, 600)
            }
        }
    }

    /**
     * Host a focused, editable field so the *selected* IME is shown while this
     * app owns the window. The IME then probes the clipboard from a process that
     * is neither the clipboard owner nor the focused app — exactly the condition
     * the spike needs to measure.
     */
    private fun showEditTarget(label: String) {
        window.setSoftInputMode(WindowManager.LayoutParams.SOFT_INPUT_STATE_ALWAYS_VISIBLE)
        val edit = EditText(this).apply {
            hint = "spike05 focus target"
            textSize = 14f
        }
        val root = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            setPadding(24, 24, 24, 24)
            addView(TextView(this@SetterActivity).apply {
                text = "Spike05Setter edit target\nlabel=$label"
                textSize = 13f
            })
            addView(edit)
        }
        setContentView(root)
        edit.requestFocus()
        edit.post {
            val imm = getSystemService(Context.INPUT_METHOD_SERVICE) as InputMethodManager
            val shown = imm.showSoftInput(edit, InputMethodManager.SHOW_IMPLICIT)
            Log.i(TAG, "RESULT op=edit_target label=$label status=ok shown=$shown")
        }
    }

    private fun runOp(op: String, label: String) {
        Log.i(TAG, "RESULT op=focus_ready label=$label has_focus=${window.decorView.hasWindowFocus()}")
        when (op) {
            "write" -> doWrite(label, intent.getStringExtra(E_TEXT) ?: "")
            // Large payloads cannot cross `am start --es` (ARG_MAX), so a size
            // is passed instead and the payload is generated in-process. Without
            // this, a multi-KB sweep silently reads a stale clip and looks like
            // a platform limit.
            "writegen" -> {
                val n = intent.getIntExtra(E_SIZE, 0)
                doWrite(label, payloadOf(n))
            }
            "read" -> doRead(label)
            "writeread" -> { doWrite("$label-write", intent.getStringExtra(E_TEXT) ?: ""); doRead("$label-read") }
            "clear" -> doClear(label)
            "hide" -> doHideIme(label)
            // Non-text clip shapes: the production contract carries *text*, but
            // the platform clipboard can hold URIs / HTML / multiple items, and
            // the sync design must know what it would be handing to a peer.
            "writeuri" -> doWriteUri(label)
            "writehtml" -> doWriteHtml(label, intent.getStringExtra(E_HTML) ?: DEFAULT_HTML)
            "writemulti" -> doWriteMulti(label, intent.getIntExtra(E_ITEMS, 3))
            else -> Log.i(TAG, "RESULT op=noop label=$label status=ok detail=activity_started")
        }
    }

    private fun cm(): ClipboardManager =
        getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager

    private fun doWrite(label: String, text: String) {
        val t0 = System.nanoTime()
        try {
            val clip = ClipData.newPlainText("spike05", text)
            cm().setPrimaryClip(clip)
            val ms = (System.nanoTime() - t0) / 1_000_000.0
            Log.i(
                TAG,
                "RESULT op=write label=$label status=ok ms=${fmt(ms)} " +
                    "bytes=${text.toByteArray(Charsets.UTF_8).size} echo=${hashOf(text)}",
            )
        } catch (t: Throwable) {
            Log.e(TAG, "RESULT op=write label=$label status=error detail=${t.javaClass.simpleName}:${t.message}")
        }
    }

    private fun doRead(label: String) {
        val t0 = System.nanoTime()
        try {
            val c = cm().primaryClip
            val ms = (System.nanoTime() - t0) / 1_000_000.0
            if (c == null || c.itemCount == 0) {
                Log.i(TAG, "RESULT op=read label=$label status=empty ms=${fmt(ms)}")
                return
            }
            val item = c.getItemAt(0)
            val text = item.coerceToText(this)?.toString() ?: ""
            // Logcat truncates long lines (~4 KiB), so a large payload cannot be
            // verified from the raw text. Log a digest + length instead; the
            // harness compares it against the writer's own digest.
            val preview = if (text.length <= 64) quote(text) else "\"<${text.length} chars>\""
            Log.i(
                TAG,
                "RESULT op=read label=$label status=ok ms=${fmt(ms)} " +
                    "items=${c.itemCount} desc=${c.description?.label} mime=${item.htmlText?.let { "text/html" } ?: "text/plain"} " +
                    "bytes=${text.toByteArray(Charsets.UTF_8).size} echo=${hashOf(text)} text=$preview",
            )
        } catch (t: Throwable) {
            Log.e(TAG, "RESULT op=read label=$label status=error detail=${t.javaClass.simpleName}:${t.message}")
        }
    }

    /** A clip whose only item is a content:// URI (no text). */
    private fun doWriteUri(label: String) {
        try {
            val uri = intent.getStringExtra(E_URI) ?: "content://media/external/images/media/1"
            val clip = ClipData.newRawUri("spike05uri", android.net.Uri.parse(uri))
            cm().setPrimaryClip(clip)
            Log.i(TAG, "RESULT op=write_uri label=$label status=ok uri=$uri")
        } catch (t: Throwable) {
            Log.e(TAG, "RESULT op=write_uri label=$label status=error detail=${t.javaClass.simpleName}:${t.message}")
        }
    }

    /** An HTML-text clip: coerceToText() should yield the plain-text rendering. */
    private fun doWriteHtml(label: String, html: String) {
        try {
            val clip = ClipData.newHtmlText("spike05html", html.replace(Regex("<[^>]*>"), ""), html)
            cm().setPrimaryClip(clip)
            Log.i(TAG, "RESULT op=write_html label=$label status=ok html=$html")
        } catch (t: Throwable) {
            Log.e(TAG, "RESULT op=write_html label=$label status=error detail=${t.javaClass.simpleName}:${t.message}")
        }
    }

    /** A multi-item clip: getItemAt(0) is only the first of several items. */
    private fun doWriteMulti(label: String, n: Int) {
        try {
            val first = ClipData.newPlainText("spike05m0", "MULTI-ITEM-0")
            for (i in 1 until n) first.addItem(ClipData.Item("MULTI-ITEM-$i"))
            cm().setPrimaryClip(first)
            Log.i(TAG, "RESULT op=write_multi label=$label status=ok items=$n first=MULTI-ITEM-0")
        } catch (t: Throwable) {
            Log.e(TAG, "RESULT op=write_multi label=$label status=error detail=${t.javaClass.simpleName}:${t.message}")
        }
    }

    private fun doClear(label: String) {
        try {
            // Best-effort; on modern Android there is no public "clear" API and
            // clearing via setPrimaryClip(empty) is itself a write.
            cm().setPrimaryClip(ClipData.newPlainText("spike05", ""))
            Log.i(TAG, "RESULT op=clear label=$label status=ok detail=wrote_empty_clip")
        } catch (t: Throwable) {
            Log.e(TAG, "RESULT op=clear label=$label status=error detail=${t.javaClass.simpleName}:${t.message}")
        }
    }

    private fun doHideIme(label: String) {
        try {
            val imm = getSystemService(Context.INPUT_METHOD_SERVICE) as InputMethodManager
            imm.hideSoftInputFromWindow(window.decorView.windowToken, 0)
            Log.i(TAG, "RESULT op=hide_ime label=$label status=ok")
        } catch (t: Throwable) {
            Log.e(TAG, "RESULT op=hide_ime label=$label status=error detail=${t.javaClass.simpleName}:${t.message}")
        }
    }

    private fun fmt(d: Double) = String.format(java.util.Locale.US, "%.3f", d)

    /** Stable short digest so payloads can be matched without logging content. */
    private fun hashOf(s: String): String =
        Integer.toHexString(s.hashCode()).take(8)

    private fun quote(s: String): String =
        "\"" + s.replace("\\", "\\\\").replace("\n", "\\n").replace("\"", "\\\"") + "\""
}
