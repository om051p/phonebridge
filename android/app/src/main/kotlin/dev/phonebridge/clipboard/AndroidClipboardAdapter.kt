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
    /**
     * TTL for the "is the companion IME the default one" answer.
     *
     * Reading it is a binder round-trip to Settings.Secure, and the stats tick
     * asks once a second — a syscall per second for a value that only changes
     * when the user changes keyboards. The TTL bounds staleness to 2 s so the
     * cache can never be the reason the UI shows an outdated answer, and
     * [invalidateImeCheck] drops it early when the app comes back from the
     * input-method settings (the fastest a human can change it).
     */
    private const val IME_CHECK_TTL_MS = 2_000L

    @Volatile private var imeBound = false
    @Volatile private var imeSelected = false
    // Written AFTER imeSelected, read after the timestamp, so a reader that sees
    // a fresh timestamp also sees the value that came with it.
    @Volatile private var imeCheckedAtMs = 0L

    // Echo suppression write-in-flight window
    @Volatile private var writeInFlight = false
    @Volatile private var lastWriteUptimeMs = 0L

    // User intent for clipboard synchronization (the app's master switch).
    // Defaults to on because the adapter only starts when the service is active;
    // both directions are gated so flipping the switch actually stops sync
    // instead of only changing a label.
    @Volatile private var syncEnabled = true

    /** Whether clipboard synchronization is currently enabled by the user. */
    val enabled: Boolean
        get() = syncEnabled

    /**
     * Enables or disables clipboard synchronization in both directions.
     *
     * Disabling stops local clip observation ([readAndForwardCurrentClip] becomes
     * a no-op) and refuses inbound platform writes, so no clip crosses the wire
     * while it is off. A remote clip that arrives is refused rather than silently
     * applied, which keeps the peer's view of this device honest.
     */
    fun setSyncEnabled(value: Boolean) {
        syncEnabled = value
        Log.i(TAG, "Clipboard synchronization ${if (value) "enabled" else "disabled"} by user")
    }

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
     * Checks if PhoneBridge companion IME is the default input method, from a
     * short-lived cache (see [IME_CHECK_TTL_MS]).
     */
    fun checkImeSelected(context: Context): Boolean {
        return try {
            val selected = cachedImeAnswer(SystemClock.elapsedRealtime()) {
                val defaultIme = Settings.Secure.getString(
                    context.contentResolver,
                    Settings.Secure.DEFAULT_INPUT_METHOD
                )
                defaultIme != null && defaultIme.contains(context.packageName)
            }
            recomputeState()
            selected
        } catch (t: Throwable) {
            Log.w(TAG, "Failed to inspect default input method: ${t.message}")
            false
        }
    }

    /**
     * Returns the cached answer, calling [read] only when the cache is stale.
     *
     * Kept separate from the Settings lookup so the caching policy itself is
     * testable without a Context: [read] is only ever invoked when the answer
     * has aged out, and a failed read leaves the cache empty rather than
     * remembering an answer we never got.
     */
    internal fun cachedImeAnswer(nowMs: Long, read: () -> Boolean): Boolean {
        val checkedAt = imeCheckedAtMs
        if (checkedAt != 0L && nowMs - checkedAt < IME_CHECK_TTL_MS) {
            return imeSelected
        }
        val selected = read()
        // Value first, timestamp second: a reader that sees a fresh timestamp
        // must see the value it was recorded with.
        imeSelected = selected
        imeCheckedAtMs = nowMs
        return selected
    }

    /**
     * Drops the cached IME answer so the next check reads Settings again.
     *
     * Called when the app returns to the foreground, which is when a user can
     * plausibly have changed the default keyboard.
     */
    fun invalidateImeCheck() {
        imeCheckedAtMs = 0L
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
        if (!syncEnabled) {
            Log.i(TAG, "Refusing inbound clip write: clipboard synchronization is disabled")
            return false
        }

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
        if (!syncEnabled) {
            return false
        }
        val sender = transportSender
        if (sender != null) {
            return sender(payload)
        }
        // Nothing registered to carry the update. Reporting success here (the
        // previous behaviour) made a dropped clip indistinguishable from a
        // delivered one: the Go engine treated the update as sent and the caller
        // logged it as forwarded. Fail instead, so the drop is visible and the
        // item stays eligible for the reconnect sync on the next channel open.
        Log.w(TAG, "No clipboard transport registered; outbound update dropped (bytes=${payload.size})")
        return false
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
        if (!syncEnabled) {
            return false
        }

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
            } else {
                // Not silent: with no peer session (or sync disabled) the update
                // is held in the Go engine's current item and only re-sent when a
                // clipboard channel opens, so this is diagnosable rather than lost.
                Log.w(TAG, "Local clipboard item was not forwarded to a peer: bytes=${bytes.size}")
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
