package dev.phonebridge.signaling

import android.os.SystemClock
import android.util.Log
import org.json.JSONObject

private const val TAG = "SessionRestorer"

/**
 * True when the phone's media-stats snapshot describes a live, connected
 * transport: the pion PeerConnection reports `connected` AND the transport
 * state machine is negotiated or streaming (2..3).
 *
 * Both halves are required. `MediaStatsJSON` falls back to the LAST known
 * pcState once the session object is gone, so an idle or stopped transport can
 * still advertise a stale `connected` — trusting that would wrongly
 * short-circuit a restore (and skip the flush) on exactly the cold start this
 * gate protects. Null, empty, or unparseable input is "not connected", so a
 * dead engine never reads as ready.
 */
internal fun mediaStatsIndicateConnected(statsJson: String?): Boolean {
    if (statsJson.isNullOrEmpty()) return false
    return try {
        val stats = JSONObject(statsJson)
        stats.optString("pcState") == "connected" &&
            stats.optInt("transportState", -1) in 2..3
    } catch (_: Throwable) {
        false
    }
}

/**
 * Picks the signaling endpoint of a trusted, dialable peer — never the local
 * device, never an untrusted or revoked one, never a stale record.
 *
 * Returns `http://host:port` built from the mDNS record, or null when nothing
 * usable is discovered yet. Kept as a pure function because the trust filter is
 * the security boundary of the whole cold-start restore: an app that accepted
 * its own advertisement (the phone resolves itself over NSD) or an unpaired
 * neighbor here would dial the wrong device.
 */
internal fun pickTrustedPeerEndpoint(
    peers: List<Map<String, Any?>>,
    trustedIds: Set<String>,
    localDeviceId: String?,
): String? {
    if (trustedIds.isEmpty()) return null
    for (row in peers) {
        val id = row["id"] as? String ?: continue
        if (id.isEmpty() || id == localDeviceId || id !in trustedIds) continue
        if (row["isStale"] == true) continue
        val host = row["host"] as? String ?: continue
        if (host.isBlank()) continue
        val port = (row["port"] as? Number)?.toInt() ?: continue
        if (port <= 0) continue
        return "http://$host:$port"
    }
    return null
}

/**
 * Waits for a trusted peer to appear, waking on real discovery events instead
 * of polling on a timer: [awaitEvent] blocks until the next peer-resolved event
 * fires or the remaining budget elapses, and the pick runs after every wake.
 *
 * Bounded on both ends — an event storm cannot spin it (each wake re-picks and
 * the deadline is absolute), and a silent browse cannot outlive [budgetMs].
 * Returns null when the budget runs out with no trusted peer.
 */
internal fun awaitTrustedPeerEndpoint(
    pick: () -> String?,
    awaitEvent: (timeoutMs: Long) -> Boolean,
    budgetMs: Long,
    nowMs: () -> Long = { SystemClock.uptimeMillis() },
): String? {
    pick()?.let { return it }
    val deadline = nowMs() + budgetMs
    while (true) {
        val remaining = deadline - nowMs()
        if (remaining <= 0) return null
        if (!awaitEvent(remaining)) return null
        pick()?.let { return it }
    }
}

/**
 * Waits until [isReady] holds, in bounded steps.
 *
 * Each step re-observes the *actual* state (pion connection state, the send
 * counter — there is no Kotlin-side callback for either), so success is always
 * a state observation rather than the passage of time; the step budget only
 * caps a state that never arrives. This is the one wait in the cold-start
 * restore: no arbitrary sleeps, no unbounded retry.
 *
 * Returns true once [isReady] holds, false when the budget runs out or the
 * wait is interrupted.
 */
internal fun awaitCondition(
    isReady: () -> Boolean,
    stepMs: Long,
    maxSteps: Int,
): Boolean {
    if (isReady()) return true
    repeat(maxSteps) {
        try {
            Thread.sleep(stepMs)
        } catch (interrupted: InterruptedException) {
            Thread.currentThread().interrupt()
            return isReady()
        }
        if (isReady()) return true
    }
    return false
}

/**
 * Restores a session to the desktop over the EXISTING DEC-022 peer-offer path —
 * the same signed `POST /session/peer-offer` the UI's screen-share CONNECT uses,
 * with a transport-only offer (`mediaRelease`+`mediaInit`+`mediaCreateOffer`,
 * exactly what `LanSignalingServer.DefaultSignalingHandler.handleOffer` does
 * when the desktop dials the phone). No capture, no MediaProjection, no second
 * WebRTC implementation: the caller injects those existing entry points.
 *
 * Designed for the Quick Settings cold-start flow, where force-stop left the
 * desktop holding a session slot against a dead peer: a typed `SESSION_BUSY`
 * is answered by releasing that slot once (`/session/stop`) and re-offering
 * exactly once — never a blind retry loop.
 *
 * Threading: blocking (HTTP + ICE gathering); call it off the main thread.
 */
class SessionRestorer(
    private val endpoint: String,
    private val transportReady: () -> Boolean,
    private val createTransportOffer: () -> String,
    private val postOffer: (endpoint: String, offerSdp: String) -> String,
    private val postStop: (endpoint: String) -> Unit,
    private val applyAnswer: (answerSdp: String) -> Unit,
    private val log: (String) -> Unit = { Log.i(TAG, it) },
) {

    sealed class Outcome {
        /** A live session already exists; nothing was torn down or re-offered. */
        data class AlreadyConnected(val endpoint: String) : Outcome()

        /** A fresh session was negotiated and its answer applied. */
        data class Restored(val endpoint: String) : Outcome()

        /** The restore could not complete; the pending clip stays in the engine. */
        data class Failed(val reason: String) : Outcome()
    }

    fun restore(): Outcome {
        if (transportReady()) {
            return Outcome.AlreadyConnected(endpoint)
        }

        var releasedStaleSlot = false
        while (true) {
            log("negotiating transport-only session with $endpoint (no capture, no MediaProjection)")
            val offerSdp = try {
                createTransportOffer()
            } catch (t: Throwable) {
                return Outcome.Failed("could not build a transport offer: ${t.message}")
            }

            try {
                val answerSdp = postOffer(endpoint, offerSdp)
                applyAnswer(answerSdp)
                log("answer applied; media transport started for $endpoint")
                return Outcome.Restored(endpoint)
            } catch (e: DesktopSessionException) {
                if (e.code == "SESSION_BUSY" && !releasedStaleSlot) {
                    releasedStaleSlot = true
                    log("desktop still holds the previous session slot; releasing it and re-offering once")
                    try {
                        postStop(endpoint)
                    } catch (t: Throwable) {
                        log("session release failed (continuing with one retry): ${t.message}")
                    }
                    continue
                }
                return Outcome.Failed("desktop refused the session (${e.code}): ${e.message}")
            } catch (t: Throwable) {
                return Outcome.Failed("session offer failed: ${t.message}")
            }
        }
    }
}
