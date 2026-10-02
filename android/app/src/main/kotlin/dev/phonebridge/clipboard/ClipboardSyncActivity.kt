package dev.phonebridge.clipboard

import android.app.Activity
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.util.Log
import dev.phonebridge.bridge.GoBridge
import dev.phonebridge.security.DeviceIdentityManager
import dev.phonebridge.security.TrustStore
import dev.phonebridge.service.PhoneBridgeService
import dev.phonebridge.signaling.DesktopSession
import dev.phonebridge.signaling.SessionRestorer
import dev.phonebridge.signaling.awaitCondition
import dev.phonebridge.signaling.awaitTrustedPeerEndpoint
import dev.phonebridge.signaling.mediaStatsIndicateConnected
import dev.phonebridge.signaling.pickTrustedPeerEndpoint
import org.json.JSONObject
import java.io.File
import java.util.concurrent.Semaphore
import java.util.concurrent.TimeUnit

/**
 * Invisible one-shot clipboard sync surface (DEC-023 Tier-2 entry point).
 *
 * A Quick Settings tile cannot read the clipboard on its own. From Android 10
 * on the read is granted only to the uid that owns the *focused* window
 * (AOSP `ClipboardService.clipboardAccessAllowed`), and with the shade open
 * that window belongs to SystemUI — so the tile's old inline read always came
 * back null while looking perfectly wired up. This activity is launched
 * instead: translucent and history-less, it takes focus for exactly one read,
 * hands the clip to the Go engine, and finishes before anything is drawn.
 *
 * A COLD tap is the hard half: force-stop left no session, so the engine had
 * nowhere to send the clip ("No clipboard transport registered") and the tap
 * silently did nothing until the user opened the app and pressed CONNECT.
 * This activity therefore drives the EXISTING bootstrap after the read —
 * service → mDNS browse → trusted peer → the same signed DEC-022 peer-offer
 * the UI's CONNECT uses ([SessionRestorer]) — gated on real state (peer
 * resolved, PeerConnection `connected`), never on arbitrary sleeps and never
 * with a parallel WebRTC implementation.
 *
 * The just-read bytes are held by the clipboard adapter's pending slot, not by
 * the engine's current item: on a fresh session the peer's reconnect-sync item
 * can replace `currentItem` before the local channel-open flush runs, which is
 * how the first three device runs lost the user's clip. Once the transport is
 * connected this activity hands the held bytes over exactly once
 * ([AndroidClipboardAdapter.flushPendingLocalClip]) and only then finishes.
 *
 * Registers in AndroidManifest.xml; without that entry the launch throws and
 * the tile silently falls back to a read it is not allowed to make.
 */
class ClipboardSyncActivity : Activity() {

    companion object {
        private const val TAG = "ClipboardSync"

        /**
         * Hard ceiling for the pre-focus phase.
         *
         * Strictly longer than the startup wait ([CLIPBOARD_STARTUP_STEP_MS] ×
         * [CLIPBOARD_STARTUP_MAX_STEPS]): a watchdog that could expire while the
         * wait is still running would kill the activity and drop the very tap
         * the wait exists to protect. Once focus arrives the wait always
         * terminates, so this only covers the case where focus never arrives.
         */
        private const val WATCHDOG_MS =
            CLIPBOARD_STARTUP_STEP_MS * CLIPBOARD_STARTUP_MAX_STEPS + 1_500L

        /**
         * Crash net for the whole post-focus transaction (read → peer → offer →
         * connected → flush). The normal path finishes in a few seconds; this
         * only exists so an invisible, focus-holding activity can never outlive
         * a restore that has wedged. Every step inside is individually bounded
         * well below this.
         */
        private const val RESTORE_WATCHDOG_MS = 60_000L

        /** Budget for the trusted peer to resolve (event-driven, see below). */
        private const val PEER_BUDGET_MS = 10_000L

        /** Steps to wait for the PeerConnection to reach `connected` after the answer. */
        private const val TRANSPORT_STEP_MS = 250L
        private const val TRANSPORT_MAX_STEPS = 40

        /** Steps to observe the clipboard channel-open flush (send counter bump). */
        private const val FLUSH_STEP_MS = 150L
        private const val FLUSH_MAX_STEPS = 10
    }

    private val handler = Handler(Looper.getMainLooper())
    private val watchdog = Runnable { finishOnce("watchdog expired before window focus") }
    private val restoreDeadline = Runnable { finishOnce("restore deadline expired") }

    private var done = false
    private var startupSteps = 0
    private var restoreStarted = false

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        // A cold tap (tile pressed while the service was stopped) needs the
        // service, and therefore the Go engine, before a read can be forwarded.
        try {
            PhoneBridgeService.startService(this)
            Log.i(TAG, "Cold-start clipboard sync: service start requested")
        } catch (t: Throwable) {
            Log.w(TAG, "Could not start PhoneBridgeService: ${t.message}")
        }
        handler.postDelayed(watchdog, WATCHDOG_MS)
    }

    override fun onWindowFocusChanged(hasFocus: Boolean) {
        super.onWindowFocusChanged(hasFocus)
        if (!hasFocus) {
            return
        }
        // Focus is what makes the read legal; from here the pre-focus watchdog
        // has nothing left to protect, and the restore deadline takes over.
        handler.removeCallbacks(watchdog)
        handler.postDelayed(restoreDeadline, RESTORE_WATCHDOG_MS)
        attemptRead()
    }

    private fun attemptRead() {
        if (done) {
            return
        }
        // The adapter is only usable after the service has started it; give a
        // cold start a bounded moment rather than dropping the user's tap.
        if (shouldWaitForAdapterStartup(
                AndroidClipboardAdapter.state,
                startupSteps,
                CLIPBOARD_STARTUP_MAX_STEPS
            )
        ) {
            startupSteps++
            handler.postDelayed({ attemptRead() }, CLIPBOARD_STARTUP_STEP_MS)
            return
        }

        // Read FIRST, while this window legally holds focus. On a cold start
        // the send fails (no transport) but the Go engine retains the item as
        // its single in-memory current item — the restore below then flushes it
        // at channel open. Restoring first would spend the focus window on
        // signaling and could miss a clipboard the user is about to overwrite.
        val ok = AndroidClipboardAdapter.triggerManualPull()
        Log.i(TAG, "Foreground clipboard read finished; forwarded=$ok")
        startRestore()
    }

    /**
     * Brings up (or verifies) the session on a background thread.
     *
     * Runs even when [ok] was false: the engine may still hold an item from an
     * earlier attempt, and a session without a pending item costs the same
     * bounded work. Bounded at every stage — discovery budget, HTTP timeouts,
     * transport state budget, flush budget — and hard-capped by
     * [RESTORE_WATCHDOG]; there is no unbounded retry anywhere.
     */
    private fun startRestore() {
        if (restoreStarted || done) {
            return
        }
        restoreStarted = true
        Thread({ runRestore() }, "clip-session-restore").start()
    }

    private fun runRestore() {
        try {
            if (!GoBridge.loaded) {
                finishOnMain("native engine not loaded")
                return
            }

            if (transportConnected()) {
                // Warm tile tap on a live session: the read above went straight
                // over the open DataChannel. One flush attempt covers the corner
                // where that send failed and the item is still held.
                AndroidClipboardAdapter.flushPendingLocalClip()
                Log.i(TAG, "Session bootstrap: transport already connected; nothing to restore")
                finishOnMain(null)
                return
            }

            val identity = try {
                DeviceIdentityManager.loadOrGenerate(applicationContext)
            } catch (t: Throwable) {
                finishOnMain("device identity unavailable: ${t.message}")
                return
            }

            // The security boundary of the restore: only a paired, non-revoked
            // desktop may be dialed — never this device, never a stale record.
            val trusted = TrustStore(File(applicationContext.filesDir, "trusted_devices.json"))
                .list()
                .filterNot { it.revoked }
                .map { it.deviceId }
                .toSet()
            if (trusted.isEmpty()) {
                finishOnMain("no trusted desktop paired")
                return
            }

            // The service starts the browse before the adapter accepts a read
            // (same main-thread call chain), so by the time we are here the
            // browser exists; a null one means the service never came up.
            val browser = PhoneBridgeService.platformBrowser()
            if (browser == null) {
                finishOnMain("discovery is not up")
                return
            }

            // Event-driven peer wait: pick first, then block on the resolve
            // event rather than polling the snapshot. tryAcquire with the
            // remaining budget keeps the deadline absolute.
            val resolveSignal = Semaphore(0)
            val previousHook = browser.onPeerResolved
            browser.onPeerResolved = { resolveSignal.release() }
            val endpoint = try {
                awaitTrustedPeerEndpoint(
                    pick = { pickTrustedPeerEndpoint(browser.snapshot(), trusted, identity.deviceId) },
                    awaitEvent = { timeoutMs ->
                        resolveSignal.tryAcquire(timeoutMs, TimeUnit.MILLISECONDS)
                    },
                    budgetMs = PEER_BUDGET_MS,
                )
            } finally {
                browser.onPeerResolved = previousHook
            }
            if (endpoint == null) {
                finishOnMain("no trusted desktop resolved within ${PEER_BUDGET_MS}ms")
                return
            }
            Log.i(TAG, "Session bootstrap: trusted peer resolved → $endpoint")

            // The EXISTING DEC-022 path: the same signed peer-offer the UI's
            // CONNECT posts, with the transport-only offer shape
            // (mediaRelease+mediaInit+mediaCreateOffer) the signaling server
            // already uses for desktop-dialed repair. No MediaProjection: the
            // transport never touches the capture pipeline.
            val restorer = SessionRestorer(
                endpoint = endpoint,
                transportReady = { transportConnected() },
                createTransportOffer = {
                    GoBridge.mediaRelease()
                    GoBridge.mediaInit()
                    DesktopSession.sdpFromOfferBlob(GoBridge.mediaCreateOffer())
                },
                postOffer = { ep, sdp -> DesktopSession.postPeerOffer(identity, ep, sdp) },
                postStop = { ep -> DesktopSession.postStop(identity, ep) },
                applyAnswer = { answerSdp ->
                    // mediaSetAnswer expects the transport's SDP blob, not a
                    // bare SDP string — same shape the offer side used.
                    val answerBlob = JSONObject().apply {
                        put("type", "answer")
                        put("sdp", answerSdp)
                    }.toString().toByteArray(Charsets.UTF_8)
                    GoBridge.mediaSetAnswer(answerBlob)
                    GoBridge.mediaStart()
                },
            )
            when (val outcome = restorer.restore()) {
                is SessionRestorer.Outcome.AlreadyConnected ->
                    Log.i(TAG, "Session bootstrap: a live session already existed at ${outcome.endpoint}")
                is SessionRestorer.Outcome.Restored ->
                    Log.i(TAG, "Session bootstrap: session connected with ${outcome.endpoint}")
                is SessionRestorer.Outcome.Failed -> {
                    finishOnMain("session restore failed: ${outcome.reason}")
                    return
                }
            }

            // Real state gate: the answer only means the transport TRIED. The
            // clipboard DataChannel opens off the connected PeerConnection, so
            // delivery cannot be claimed before pcState reports connected.
            if (!awaitCondition({ transportConnected() }, TRANSPORT_STEP_MS, TRANSPORT_MAX_STEPS)) {
                finishOnMain("transport did not reach connected state")
                return
            }
            Log.i(TAG, "Session bootstrap: transport connected")

            // The item read before the transport existed is held in the
            // clipboard adapter's in-memory pending slot — never in the engine's
            // current item, which the peer's reconnect sync legitimately
            // overwrites. Hand the exact bytes over now that the transport is
            // connected; the slot clears only once the engine took them. The
            // retry is bounded and state-gated (the slot itself), not a timer.
            val flushed = awaitCondition(
                { AndroidClipboardAdapter.flushPendingLocalClip() },
                FLUSH_STEP_MS,
                FLUSH_MAX_STEPS,
            )
            if (flushed) {
                Log.i(TAG, "Session bootstrap: pending local clipboard handed to the transport")
            } else {
                // Not lost: the slot keeps the bytes for the next transport-ready
                // moment (another tile tap, or a focus read on a live session).
                Log.w(TAG, "Session bootstrap: pending local clipboard still held")
            }
            finishOnMain(null)
        } catch (t: Throwable) {
            finishOnMain("session restore crashed: ${t.message}")
        }
    }

    /** True when the phone's media transport is negotiated/streaming AND connected. */
    private fun transportConnected(): Boolean =
        mediaStatsIndicateConnected(GoBridge.mediaStats()?.let { String(it, Charsets.UTF_8) })

    private fun finishOnMain(reason: String?) {
        handler.post { finishOnce(reason) }
    }

    private fun finishOnce(reason: String?) {
        if (done) {
            return
        }
        done = true
        if (reason != null) {
            Log.w(TAG, "Foreground clipboard sync abandoned: $reason")
        } else {
            Log.i(TAG, "Foreground clipboard sync finished")
        }
        finish()
    }

    override fun onDestroy() {
        handler.removeCallbacksAndMessages(null)
        super.onDestroy()
    }
}

/**
 * How long a clipboard entry point waits for a cold-started service to bring
 * the adapter up, and the step between checks.
 *
 * Shared by both entry points (the Quick Settings tile's activity and the
 * focus-triggered read when the app comes to the front) so a cold start costs
 * the same bounded moment everywhere instead of dropping the user's copy.
 */
internal const val CLIPBOARD_STARTUP_STEP_MS = 150L
internal const val CLIPBOARD_STARTUP_MAX_STEPS = 10

/**
 * Whether the one-shot read must keep waiting for the clipboard adapter to come
 * up (and the service, and with it the Go engine, to be started by it).
 *
 * Bounded on purpose at both ends: waiting forever would leave an invisible,
 * focus-holding activity on the user's screen, while not waiting at all would
 * answer a cold tap with "nothing forwarded" even though the user's clip is
 * right there. Once the budget is used up the read is attempted anyway, so the
 * tap is answered rather than dropped.
 */
internal fun shouldWaitForAdapterStartup(
    state: AdapterState,
    stepsUsed: Int,
    maxSteps: Int
): Boolean = state == AdapterState.STOPPED && stepsUsed < maxSteps
