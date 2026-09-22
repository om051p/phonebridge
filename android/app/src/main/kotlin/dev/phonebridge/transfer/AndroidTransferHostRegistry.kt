package dev.phonebridge.transfer

import android.content.Context
import android.util.Log
import dev.phonebridge.bridge.GoBridge

/**
 * AndroidTransferHostRegistry owns the process-wide transfer plane lifecycle,
 * mirroring AndroidClipboardAdapter: the foreground service starts it once the
 * Go engine is up and releases it on destroy. [stopIfStarted] goes through
 * GoBridge.transferStop, which interrupts in-flight transfers (DEC-024 has no
 * resume) and deletes every pending MediaStore row the host still tracks — so a
 * service kill cannot leak a descriptor or leave a half-received download in
 * the user's Downloads.
 */
object AndroidTransferHostRegistry {

    private const val TAG = "AndroidTransferHost"

    @Volatile
    private var host: AndroidTransferHost? = null

    /** True once the Go transfer engine has been initialized with our host. */
    @Volatile
    var isRunning: Boolean = false
        private set

    /**
     * Registers the host with the Go engine exactly once per service lifetime.
     * Safe to call on every service start; [onServiceDestroy] is the only thing
     * that resets the state.
     */
    fun ensureStarted(context: Context) {
        if (isRunning) return
        if (!GoBridge.loaded) return
        val appContext = context.applicationContext
        synchronized(this) {
            if (isRunning) return
            try {
                host = AndroidTransferHost(appContext)
                // The peer id is learned later, when signaling targets a device;
                // transferSetPeer updates it without rebuilding the engine.
                val ok = GoBridge.transferInit(host!!, "")
                if (ok) {
                    isRunning = true
                    Log.i(TAG, "transfer plane initialized (MediaStore pending on API 29+)")
                } else {
                    host = null
                    Log.w(TAG, "GoBridge.transferInit returned false")
                }
            } catch (t: Throwable) {
                host = null
                Log.w(TAG, "transfer plane init failed: ${t.message}")
            }
        }
    }

    /**
     * Records the signaling target so received/sent files are attributed to a
     * device in the activity history.
     */
    fun setPeerDeviceId(deviceId: String) {
        if (!isRunning) return
        try {
            GoBridge.transferSetPeer(deviceId)
        } catch (t: Throwable) {
            Log.w(TAG, "transferSetPeer failed: ${t.message}")
        }
    }

    /**
     * Tears the plane down: Go interrupts in-flight transfers, aborts their
     * pending entries through the host, and releases the global reference.
     */
    fun stopIfStarted() {
        if (!isRunning) return
        synchronized(this) {
            if (!isRunning) return
            try {
                GoBridge.transferStop()
            } catch (t: Throwable) {
                Log.w(TAG, "transferStop failed: ${t.message}")
            }
            host = null
            isRunning = false
        }
    }

    /** Alias used by service teardown for symmetry with the clipboard adapter. */
    fun release() = stopIfStarted()
}
