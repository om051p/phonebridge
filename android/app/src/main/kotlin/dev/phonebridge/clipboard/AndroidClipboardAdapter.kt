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
import dev.phonebridge.signaling.mediaStatsIndicateConnected
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

    /**
     * Automatic focus-triggered read gate.
     *
     * One process-wide instance on purpose: the read window is a property of
     * the app, not of the surface that happened to gain focus.
     */
    private val focusReadGate = FocusReadGate()

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

        // The engine is gone; the slot never outlives it (and is never persisted).
        pendingLocalClipSlot.clear()
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

    /**
     * Reads and forwards the current platform clip from a foreground moment.
     *
     * Being on screen is the one moment Android 10+ permits this read without
     * the companion keyboard being selected, which makes it the no-keyboard
     * phone→PC trigger; the app calls it when it gains window focus. The gate
     * collapses focus flaps to at most one read per window, and explicit pulls
     * ([triggerManualPull]) bypass it.
     *
     * Returns true when an item was read and handed to the Go engine.
     */
    fun readCurrentClipOnFocus(nowMs: Long = SystemClock.uptimeMillis()): Boolean {
        // Nothing to gate before the engine exists. The service starts
        // asynchronously while a window is gaining focus, so a cold launch can
        // reach here first; consuming the window then would swallow the very
        // copy the trigger exists to forward. The caller retries instead.
        if (state == AdapterState.STOPPED) {
            return false
        }
        return focusReadGate.run(nowMs) { readAndForwardCurrentClip() }
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
     * With a live transport the item goes straight to the engine (this is the
     * warm path). With no transport — a cold tile tap or a cold app launch —
     * the exact bytes are held in [pendingLocalClipSlot] instead of being
     * submitted into engine state the peer's reconnect-sync can overwrite;
     * [flushPendingLocalClip] sends them once a transport exists.
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
            val mimeType = "text/plain;charset=utf-8"

            if (!transportConnected()) {
                // Cold transport. Holding here — instead of submitting into the
                // engine's live state — is what keeps the exact bytes safe while
                // the session is negotiated: the peer's reconnect-sync item can
                // replace engine.currentItem during that window, and it can
                // never touch this slot.
                pendingLocalClipSlot.hold(mimeType, bytes, nowMs)
                Log.i(TAG, "Held local clipboard item until a clipboard transport exists: bytes=${bytes.size}")
                return false
            }

            // A fresh local copy supersedes anything still held from an earlier
            // transport-less moment (latest-wins policy of the slot).
            pendingLocalClipSlot.clear()
            val ok = GoBridge.clipboardOnLocalCopy(mimeType, bytes, nowMs)
            if (ok) {
                Log.i(TAG, "Forwarded local clipboard item to Go engine: bytes=${bytes.size}")
            } else {
                // The engine did not take the item across the transport; hold the
                // exact bytes so the next transport-ready moment retries them.
                pendingLocalClipSlot.hold(mimeType, bytes, nowMs)
                Log.w(TAG, "Local clipboard item held for retry: bytes=${bytes.size}")
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

    // ------------------------------------------------------------------
    // Pending local clipboard (cold-start delivery slot)
    // ------------------------------------------------------------------

    /**
     * The single in-memory pending item, shared by every entry point (tile,
     * focus read, IME) so a cold start delivers what the user actually copied.
     *
     * The Go engine's current item is not usable as this buffer: on a fresh
     * session the peer's reconnect-sync item legitimately replaces it before the
     * local channel-open flush runs, which is how cold reads were lost (verified
     * on device: 3 of 3 cold taps never reached the desktop). This slot lives
     * outside that exchange, so the peer's item can update engine state freely
     * and can never clear or overwrite what is held here.
     */
    private val pendingLocalClipSlot = PendingLocalClipSlot()

    /** Whether an already-read local item is waiting for a live transport. */
    val hasPendingLocalClip: Boolean
        get() = !pendingLocalClipSlot.isEmpty

    /**
     * Hands the held item to the Go engine once the clipboard transport is
     * connected, clearing the slot only after the engine took it.
     *
     * Returns true when there is nothing to do (no item held, or the held item
     * was accepted); false while the item must stay held, which is what the
     * bounded transport-ready wait in ClipboardSyncActivity retries on. The
     * accept path always sends exactly once: an accepted item clears the slot,
     * so no later call can re-send it.
     */
    fun flushPendingLocalClip(): Boolean {
        if (!syncEnabled) {
            return false
        }
        val pending = pendingLocalClipSlot.peek() ?: return true
        if (!GoBridge.loaded || !transportConnected()) {
            return false
        }

        val ok = GoBridge.clipboardOnLocalCopy(pending.mimeType, pending.payload, pending.copiedAtMs)
        if (ok) {
            pendingLocalClipSlot.clearIfSame(pending)
            Log.i(TAG, "Pending local clipboard item sent to the peer: bytes=${pending.payload.size}")
        } else {
            Log.w(TAG, "Pending local clipboard item not sent yet: bytes=${pending.payload.size}")
        }
        return ok
    }

    /**
     * Whether the phone's media transport reports a live connected session.
     *
     * This is the same real-state observation the session restorer gates on
     * (pion PeerConnection `connected` over a negotiated transport), so the
     * pending slot is only ever handed over when a send can actually leave the
     * device — never on a timer.
     */
    private fun transportConnected(): Boolean =
        mediaStatsIndicateConnected(GoBridge.mediaStats()?.let { String(it, Charsets.UTF_8) })
}
