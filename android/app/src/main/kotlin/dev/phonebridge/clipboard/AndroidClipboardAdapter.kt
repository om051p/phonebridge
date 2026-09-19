package dev.phonebridge.clipboard

import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import android.os.Handler
import android.os.Looper
import android.os.SystemClock
import android.provider.Settings
import android.util.Log
import dev.phonebridge.bridge.ClipboardHostCallback
import dev.phonebridge.bridge.GoBridge
import java.util.concurrent.CopyOnWriteArrayList
import java.util.concurrent.atomic.AtomicBoolean
import java.util.concurrent.atomic.AtomicReference

/**
 * Android operational states for clipboard synchronization (DEC-023).
 */
enum class AdapterState {
    /** PhoneBridgeService is inactive; Go runtime and clipboard engine are off. */
    STOPPED,

    /** Service running; remote writes succeed; ambient reads dormant; Tier-2 pull available. */
    WRITE_ONLY_DORMANT,

    /** Companion IME is default input method and active window session is bound (mVisibleBound=true). */
    AMBIENT_ACTIVE,

    /** Platform permission denied, security exception, or clipboard manager unavailable. */
    UNAVAILABLE
}

/**
 * Production Android Clipboard Adapter (DEC-023, Phase 3 Step 4).
 *
 * Implements the Android platform boundary delegating all synchronization,
 * SHA-256 echo suppression, conflict arbitration, and MIME normalization
 * to the pure Go clipboard.Engine.
 *
 * Enforces the strict 768 KiB (786,432 bytes) application payload ceiling.
 * Adheres strictly to the Zero Logging Rule: clipboard text is never logged.
 */
object AndroidClipboardAdapter : ClipboardHostCallback {

    private const val TAG = "AndroidClipboard"

    /** Strict application payload ceiling: 768 KiB (786,432 bytes). */
    const val MAX_PAYLOAD_SIZE: Int = 786432

    private val currentState = AtomicReference(AdapterState.STOPPED)
    private val isStarted = AtomicBoolean(false)

    private val mainHandler: Handler? by lazy {
        try {
            Handler(Looper.getMainLooper())
        } catch (_: Throwable) {
            null
        }
    }
    private var appContext: Context? = null
    private var clipboardManager: ClipboardManager? = null

    // Window and IME binding state
    @Volatile private var imeBound = false
    @Volatile private var imeSelected = false

    // Echo suppression write-in-flight window
    @Volatile private var writeInFlight = false
    @Volatile private var lastWriteUptimeMs = 0L

    // Optional transport sender hook for WebRTC DataChannel forwarding
    @Volatile var transportSender: ((ByteArray) -> Boolean)? = null

    // State listeners for UI and service diagnostics
    private val stateListeners = CopyOnWriteArrayList<(AdapterState) -> Unit>()
    private val oversizedListeners = CopyOnWriteArrayList<(Int) -> Unit>()

    val state: AdapterState
        get() = currentState.get()

    fun addStateListener(listener: (AdapterState) -> Unit) {
        stateListeners.add(listener)
        listener(currentState.get())
    }

    fun removeStateListener(listener: (AdapterState) -> Unit) {
        stateListeners.remove(listener)
    }

    fun addOversizedListener(listener: (Int) -> Unit) {
        oversizedListeners.add(listener)
    }

    fun removeOversizedListener(listener: (Int) -> Unit) {
        oversizedListeners.remove(listener)
    }

    /**
     * Initializes the adapter and connects to Go engine via GoBridge.
     * Idempotent and thread-safe.
     */
    fun start(context: Context) {
        if (!isStarted.compareAndSet(false, true)) {
            return
        }

        val app = context.applicationContext
        appContext = app
        clipboardManager = app.getSystemService(Context.CLIPBOARD_SERVICE) as? ClipboardManager

        if (clipboardManager == null) {
            Log.e(TAG, "ClipboardManager unavailable on system")
            transitionTo(AdapterState.UNAVAILABLE)
            return
        }

        // Initialize Go bridge
        try {
            if (GoBridge.loaded) {
                val ok = GoBridge.clipboardInit(this)
                if (!ok) {
                    Log.e(TAG, "Failed to initialize Go clipboard engine")
                    transitionTo(AdapterState.UNAVAILABLE)
                    return
                }
            }
        } catch (t: Throwable) {
            Log.e(TAG, "Exception initializing Go clipboard engine: ${t.message}", t)
            transitionTo(AdapterState.UNAVAILABLE)
            return
        }

        checkImeSelected(app)
        recomputeState()
        Log.i(TAG, "AndroidClipboardAdapter started; initial state=${state}")
    }

    /**
     * Stops the adapter, releases resources, and tears down Go clipboard engine.
     */
    fun stop() {
        if (!isStarted.compareAndSet(true, false)) {
            return
        }

        try {
            if (GoBridge.loaded) {
                GoBridge.clipboardStop()
            }
        } catch (t: Throwable) {
            Log.w(TAG, "Exception stopping Go clipboard engine: ${t.message}")
        }

        transitionTo(AdapterState.STOPPED)
        Log.i(TAG, "AndroidClipboardAdapter stopped")
    }

    /**
     * Updates companion IME window binding state (`mVisibleBound`).
     */
    fun setImeBound(bound: Boolean) {
        imeBound = bound
        recomputeState()
    }

    /**
     * Updates companion IME selected state.
     */
    fun setImeSelected(selected: Boolean) {
        imeSelected = selected
        recomputeState()
    }

    /**
     * Checks if PhoneBridge companion IME is the default input method.
     */
    fun checkImeSelected(context: Context): Boolean {
        return try {
            val defaultIme = Settings.Secure.getString(
                context.contentResolver,
                Settings.Secure.DEFAULT_INPUT_METHOD
            )
            val selected = defaultIme != null && defaultIme.contains(context.packageName)
            imeSelected = selected
            recomputeState()
            selected
        } catch (t: Throwable) {
            Log.w(TAG, "Failed to inspect default input method: ${t.message}")
            false
        }
    }

    private fun recomputeState() {
        if (!isStarted.get()) {
            transitionTo(AdapterState.STOPPED)
            return
        }

        if (clipboardManager == null) {
            transitionTo(AdapterState.UNAVAILABLE)
            return
        }

        if (imeSelected && imeBound) {
            transitionTo(AdapterState.AMBIENT_ACTIVE)
        } else {
            transitionTo(AdapterState.WRITE_ONLY_DORMANT)
        }
    }

    private fun transitionTo(newState: AdapterState) {
        val prev = currentState.getAndSet(newState)
        if (prev != newState) {
            Log.i(TAG, "AdapterState transition: $prev -> $newState")
            for (l in stateListeners) {
                try {
                    l(newState)
                } catch (t: Throwable) {
                    Log.w(TAG, "Listener error on state change: ${t.message}")
                }
            }
        }
    }

    // ------------------------------------------------------------------
    // Inbound Platform Write (Remote update applied to Android clipboard)
    // ------------------------------------------------------------------

    override fun onWritePlatformClipboard(mimeType: String, payload: ByteArray): Boolean {
        if (payload.size > MAX_PAYLOAD_SIZE) {
            onOversizedPayload(payload.size)
            return false
        }

        val cm = clipboardManager ?: return false
        val text = String(payload, Charsets.UTF_8)

        // Mark write in-flight to suppress platform listener echo
        writeInFlight = true
        lastWriteUptimeMs = SystemClock.uptimeMillis()

        val handler = mainHandler
        val writeAction: () -> Unit = {
            try {
                val clip = ClipData.newPlainText("phonebridge", text)
                cm.setPrimaryClip(clip)
                Log.i(TAG, "Platform clipboard write applied: bytes=${payload.size} mime=$mimeType")
            } catch (se: SecurityException) {
                Log.e(TAG, "SecurityException writing platform clipboard: ${se.message}")
                transitionTo(AdapterState.UNAVAILABLE)
            } catch (t: Throwable) {
                Log.e(TAG, "Failed to setPrimaryClip: ${t.message}")
            } finally {
                // Clear write-in-flight flag after brief window
                if (handler != null) {
                    handler.postDelayed({
                        writeInFlight = false
                    }, 400)
                } else {
                    writeInFlight = false
                }
            }
        }

        if (handler != null) {
            handler.post { writeAction() }
        } else {
            writeAction()
        }

        return true
    }

    // ------------------------------------------------------------------
    // Outbound Transport (Go engine sending update to peer)
    // ------------------------------------------------------------------

    override fun onSendClipboardUpdate(payload: ByteArray): Boolean {
        val sender = transportSender
        if (sender != null) {
            return sender(payload)
        }
        // If transport sender not registered, accept write safely
        return true
    }

    // ------------------------------------------------------------------
    // Oversized Payload Notification
    // ------------------------------------------------------------------

    override fun onOversizedPayload(size: Int) {
        Log.w(TAG, "Oversized clipboard payload dropped: $size bytes (limit: $MAX_PAYLOAD_SIZE bytes)")
        for (l in oversizedListeners) {
            try {
                l(size)
            } catch (t: Throwable) {
                Log.w(TAG, "Error in oversizedListener: ${t.message}")
            }
        }
    }

    // ------------------------------------------------------------------
    // Local Clip Observation and Tier-2 Explicit Pull
    // ------------------------------------------------------------------

    /**
     * Reads the current platform clipboard item and forwards to Go engine.
     * Called by companion IME on clip changed or by TileService on manual pull.
     *
     * Returns true if a valid item was read and forwarded, false otherwise.
     */
    fun readAndForwardCurrentClip(): Boolean {
        val cm = clipboardManager ?: return false
        val context = appContext ?: return false

        // Suppress reading immediately after our own platform write
        if (writeInFlight || (SystemClock.uptimeMillis() - lastWriteUptimeMs) < 300) {
            Log.d(TAG, "Suppressed reading local clip due to active platform write in-flight")
            return false
        }

        return try {
            if (!cm.hasPrimaryClip()) {
                return false
            }

            val clip = cm.primaryClip ?: return false
            if (clip.itemCount <= 0) {
                return false
            }

            val item = clip.getItemAt(0)
            val charSeq = item.coerceToText(context) ?: return false
            val text = charSeq.toString()
            val bytes = text.toByteArray(Charsets.UTF_8)

            if (bytes.size > MAX_PAYLOAD_SIZE) {
                onOversizedPayload(bytes.size)
                return false
            }

            if (!GoBridge.loaded) {
                return false
            }

            val nowMs = System.currentTimeMillis()
            val ok = GoBridge.clipboardOnLocalCopy("text/plain;charset=utf-8", bytes, nowMs)
            if (ok) {
                Log.i(TAG, "Forwarded local clipboard item to Go engine: bytes=${bytes.size}")
            }
            ok
        } catch (se: SecurityException) {
            Log.w(TAG, "SecurityException reading primary clip: ${se.message}")
            transitionTo(AdapterState.UNAVAILABLE)
            false
        } catch (t: Throwable) {
            Log.w(TAG, "Failed to read primary clip: ${t.message}")
            false
        }
    }

    /**
     * Tier-2 explicit manual pull fallback (DEC-023).
     * Invoked from Quick Settings Tile or UI when ambient observation is dormant.
     */
    fun triggerManualPull(): Boolean {
        Log.i(TAG, "Tier-2 explicit clipboard pull triggered")
        return readAndForwardCurrentClip()
    }
}
