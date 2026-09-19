package dev.phonebridge.spike05ime

import android.app.Activity
import android.os.Bundle
import android.util.Log

/**
 * Adb-facing control surface for the spike IME.
 *
 * Statics on [ProbeImeService] can only be set from inside the IME's own
 * process, and the IME has no visible UI when it is not shown. This activity
 * lives in the same package/process, so launching it over adb lets the harness
 * flip the probe configuration while the IME stays selected:
 *
 *   adb shell am start -n dev.phonebridge.spike05ime/.ImeControlActivity \
 *     --ez periodic true --ei period_ms 2000
 *
 * It finishes immediately and is not part of any product surface.
 */
class ImeControlActivity : Activity() {

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        if (intent.getBooleanExtra("crash", false)) {
            // Crash the process on purpose. This kills the IME *without* clearing
            // the default-IME setting (unlike force-stop), so the harness can
            // measure whether the system restarts a dead-but-still-selected IME
            // on its own, or only when the keyboard is next needed.
            Log.i(ProbeImeService.TAG, "RESULT op=ime_crash status=intentional")
            throw RuntimeException("spike05 intentional IME crash")
        }
        val periodic = intent.getBooleanExtra("periodic", ProbeImeService.periodicMs > 0)
        val periodMs = intent.getIntExtra("period_ms", 2000).toLong()
        ProbeImeService.periodicMs = if (periodic) periodMs else 0L
        Log.i(
            ProbeImeService.TAG,
            "RESULT op=ime_control status=ok periodic=${ProbeImeService.periodicMs > 0} " +
                "period_ms=${ProbeImeService.periodicMs} auto_probe=${ProbeImeService.autoProbe}",
        )
        // Drive an IME clipboard WRITE while the IME is not shown — the
        // Linux -> Android sync direction. The peer app then reads it back to
        // prove the write actually landed.
        val write = intent.getStringExtra("write")
        if (write != null) {
            ProbeImeService.writeFromIme(applicationContext, write)
        }
        if (intent.hasExtra("echo")) {
            ProbeImeService.echoMode = intent.getBooleanExtra("echo", false)
            Log.i(ProbeImeService.TAG, "RESULT op=ime_echo_mode on=${ProbeImeService.echoMode}")
        }
        if (intent.hasExtra("suppress")) {
            ProbeImeService.echoSuppress = intent.getBooleanExtra("suppress", false)
            Log.i(ProbeImeService.TAG, "RESULT op=ime_echo_suppress on=${ProbeImeService.echoSuppress}")
        }
        finish()
    }
}
