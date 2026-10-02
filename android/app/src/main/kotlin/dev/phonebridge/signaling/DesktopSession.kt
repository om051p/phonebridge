package dev.phonebridge.signaling

import dev.phonebridge.security.DeviceIdentityManager
import org.json.JSONObject
import java.net.HttpURLConnection
import java.net.URL

/**
 * A typed refusal (or transport failure) from the desktop's DEC-022 endpoints.
 *
 * [code] is the daemon's wire code (`SESSION_BUSY`, `PERMISSION_DENIED`, ...) so
 * callers can branch on the outcome instead of parsing messages; [message] is
 * already the human description produced by [DesktopSession.describeFailure].
 */
class DesktopSessionException(
    val code: String,
    val httpStatus: Int,
    val response: String,
    message: String,
) : Exception(message)

/**
 * The phone→desktop half of the DEC-022 signaling contract, extracted so the
 * Quick Settings cold-start restore and the UI's screen-share CONNECT post the
 * exact same signed request instead of growing a second implementation.
 *
 * The phone is always the SDP offerer (it contributes the video m-line, and an
 * answerer cannot invent one); the desktop answers `POST /session/peer-offer`.
 * Every request is Ed25519-signed with the device identity, which is what makes
 * the desktop's trust check able to authorize it.
 */
object DesktopSession {

    // The path is part of the signature, so it is defined once here and never
    // assembled piecemeal by callers.
    const val PEER_OFFER_PATH = "/session/peer-offer"
    const val STOP_PATH = "/session/stop"
    const val SIGNALING_VERSION = 1

    // LAN HTTP budget: the offer exchange is a local round trip; the desktop
    // answers without waiting for the transport to come up (AnswerPeerOffer
    // watches the connect window asynchronously), so these bound a dead peer.
    private const val CONNECT_TIMEOUT_MS = 5_000
    private const val READ_TIMEOUT_MS = 8_000
    private const val STOP_TIMEOUT_MS = 3_000

    /**
     * Normalises a discovered signaling URL into the endpoint to POST to, or
     * null when it is unusable. Host and port come from the peer's mDNS record,
     * so a malformed one is rejected rather than dialed blindly.
     */
    fun normaliseEndpoint(endpoint: String): String? {
        val base = endpoint.trim().trimEnd('/')
        val url = runCatching { URL(base) }.getOrNull()
        if (base.isEmpty() || url == null || url.host.isNullOrEmpty() || url.port <= 0) {
            return null
        }
        return base
    }

    /**
     * Unwraps `mediaCreateOffer`'s blob ({`type`,`sdp`}) into the bare SDP the
     * wire payload carries as the `offer.sdp` member. Embedding the blob
     * verbatim double-encodes it and the desktop sees a leading quote instead
     * of `v=` (found in Phase 2 acceptance).
     *
     * Throws when the blob carries no SDP — a fabricated empty offer would only
     * move the failure to the peer.
     */
    fun sdpFromOfferBlob(blob: ByteArray): String {
        val sdp = JSONObject(String(blob, Charsets.UTF_8)).optString("sdp")
        if (sdp.isBlank()) {
            throw IllegalStateException("mediaCreateOffer returned no sdp field")
        }
        return sdp
    }

    /**
     * Posts the phone's SDP offer to the desktop and returns the answer SDP.
     *
     * Throws [DesktopSessionException] with the typed refusal code on a non-2xx
     * outcome, or a plain exception for transport-level failures.
     */
    fun postPeerOffer(
        identity: DeviceIdentityManager,
        endpoint: String,
        offerSdp: String,
    ): String {
        val base = normaliseEndpoint(endpoint)
            ?: throw DesktopSessionException(
                code = "INVALID_ENDPOINT",
                httpStatus = 0,
                response = "",
                message = "The desktop address is not usable: $endpoint",
            )

        // The phone can only contribute a screen stream, so it advertises
        // exactly that rather than claiming the desktop's clipboard/input
        // planes; the clipboard and control channels ride the offer itself.
        val body = JSONObject().apply {
            put("protocol_version", SIGNALING_VERSION)
            put("version", JSONObject().apply {
                put("min", SIGNALING_VERSION)
                put("max", SIGNALING_VERSION)
            })
            put("capabilities", org.json.JSONArray().put("SCREEN"))
            put("signaling_port", LanSignalingServer.DEFAULT_PORT)
            put("offer", JSONObject().apply {
                put("type", "offer")
                put("sdp", offerSdp)
            })
        }.toString().toByteArray(Charsets.UTF_8)

        val (code, response) = post(identity, base, PEER_OFFER_PATH, body, READ_TIMEOUT_MS)
        if (code !in 200..299) {
            throw DesktopSessionException(
                code = parseErrorCode(response),
                httpStatus = code,
                response = response,
                message = describeFailure(parseErrorCode(response), response, code),
            )
        }
        val answerSdp = runCatching { JSONObject(response).optString("sdp") }.getOrDefault("")
        if (answerSdp.isEmpty()) {
            throw DesktopSessionException(
                code = "EMPTY_ANSWER",
                httpStatus = code,
                response = response,
                message = "The desktop accepted the session but returned no SDP answer",
            )
        }
        return answerSdp
    }

    /**
     * Tells the desktop the session is over, so it releases the session slot
     * instead of answering the next offer SESSION_BUSY against a dead peer.
     * Best effort by design: the caller is already on a repair path.
     */
    fun postStop(
        identity: DeviceIdentityManager,
        endpoint: String,
        reason: String = "the phone stopped sharing",
        reasonCode: String = "OK",
    ) {
        val base = normaliseEndpoint(endpoint) ?: return
        val body = JSONObject().apply {
            put("reason", reason)
            put("reason_code", reasonCode)
        }.toString().toByteArray(Charsets.UTF_8)
        post(identity, base, STOP_PATH, body, STOP_TIMEOUT_MS)
    }

    /**
     * One signed POST; returns (status, body) for both success and error
     * streams. The signature covers [path] exactly as the server sees it.
     */
    private fun post(
        identity: DeviceIdentityManager,
        base: String,
        path: String,
        body: ByteArray,
        readTimeoutMs: Int,
    ): Pair<Int, String> {
        val headers = identity.signRequest("POST", path, body)
        val conn = (URL(base + path).openConnection() as HttpURLConnection).apply {
            requestMethod = "POST"
            doOutput = true
            connectTimeout = CONNECT_TIMEOUT_MS
            readTimeout = readTimeoutMs
            setRequestProperty("Content-Type", "application/json")
            headers.forEach { (name, value) -> setRequestProperty(name, value) }
        }
        try {
            conn.outputStream.use { it.write(body) }
            val status = conn.responseCode
            val payload = (if (status in 200..299) conn.inputStream else conn.errorStream)
                ?.use { String(it.readBytes(), Charsets.UTF_8) } ?: ""
            return status to payload
        } finally {
            conn.disconnect()
        }
    }

    /** Extracts the daemon's typed code from an error body, else a generic one. */
    private fun parseErrorCode(response: String): String {
        val payload = runCatching { JSONObject(response) }.getOrNull() ?: return "UNKNOWN"
        val code = payload.optString("code")
        return if (code.isNotEmpty()) code else "UNKNOWN"
    }

    /**
     * Turns a typed refusal into something actionable for a user, matching the
     * shape the phone itself uses when refusing a desktop offer. Unknown codes
     * fall back to the message the desktop actually sent.
     */
    fun describeFailure(code: String?, response: String, httpStatus: Int): String {
        return when (code) {
            "SESSION_BUSY" -> "The desktop is already in a session"
            "PERMISSION_DENIED" -> "This phone is not paired with the desktop"
            "INCOMPATIBLE_VERSION" -> "The desktop runs an incompatible session protocol"
            "TRANSPORT_FAILED" -> "The desktop could not open the media transport"
            "UNSUPPORTED_MEDIA_PARAMS" -> "The desktop rejected the requested capture format"
            else -> runCatching { JSONObject(response).optString("message") }.getOrNull()
                ?.takeIf { it.isNotEmpty() }
                ?: "The desktop refused the session (HTTP $httpStatus)"
        }
    }
}
