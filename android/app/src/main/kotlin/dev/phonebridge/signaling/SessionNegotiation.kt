package dev.phonebridge.signaling

import org.json.JSONArray
import org.json.JSONObject

/**
 * LAN session negotiation (DEC-022), phone side.
 *
 * The phone owns the capture pipeline, so it is authoritative for the media
 * tuple: the initiator asks, the phone answers with what it will actually
 * apply — or refuses, with a typed code and a reason. It never silently
 * substitutes a different geometry, because DEC-020 makes the geometry a
 * consent-bound property: changing it requires a new MediaProjection consent,
 * so quietly capturing at a different size than the one requested would make
 * the negotiated tuple a lie.
 *
 * The logic here is pure (no Android types) so the rules can be unit-tested on
 * the host JVM; the platform values (codec capabilities, live capture state)
 * are supplied by the Android layer.
 */

/** Wire media tuple, mirroring phonebridge.v1.MediaParams. */
data class SessionMediaParams(
    val width: Int = 0,
    val height: Int = 0,
    val fps: Int = 0,
    val bitrateKbps: Int = 0,
    val codec: String = "",
) {
    /** A zero field means "no preference / not reported", never a request for zero. */
    fun isZero(): Boolean =
        width == 0 && height == 0 && fps == 0 && bitrateKbps == 0 && codec.isEmpty()

    fun toJson(): JSONObject = JSONObject().apply {
        if (width > 0) put("width", width)
        if (height > 0) put("height", height)
        if (fps > 0) put("fps", fps)
        if (bitrateKbps > 0) put("bitrate_kbps", bitrateKbps)
        if (codec.isNotEmpty()) put("codec", codec)
    }

    companion object {
        fun fromJson(obj: JSONObject?): SessionMediaParams {
            if (obj == null) return SessionMediaParams()
            return SessionMediaParams(
                width = obj.optInt("width", 0),
                height = obj.optInt("height", 0),
                fps = obj.optInt("fps", 0),
                bitrateKbps = obj.optInt("bitrate_kbps", 0),
                codec = obj.optString("codec", ""),
            )
        }
    }
}

/**
 * What this device can capture, derived from the selected encoder's real
 * [android.media.MediaCodecInfo.VideoCapabilities] — not from a constant. The
 * alignment fields take a 0 value to mean "unknown", in which case only the
 * upper bounds are enforced.
 */
data class DeviceMediaCapabilities(
    val codecs: List<String> = listOf("h264"),
    val maxWidth: Int = 0,
    val maxHeight: Int = 0,
    val maxFps: Int = 0,
    val widthAlignment: Int = 0,
    val heightAlignment: Int = 0,
    val supportsScreen: Boolean = true,
) {
    /**
     * @return true when the size is supported, or when the capability bounds are
     *         unknown (a device that did not report a limit cannot be said to
     *         have one).
     */
    fun supportsSize(width: Int, height: Int): Boolean {
        if (width <= 0 || height <= 0) return false
        if (maxWidth > 0 && width > maxWidth) return false
        if (maxHeight > 0 && height > maxHeight) return false
        if (widthAlignment > 1 && width % widthAlignment != 0) return false
        if (heightAlignment > 1 && height % heightAlignment != 0) return false
        return true
    }
}

/**
 * The capture pipeline's measured state. [encodedFps] and [gopAus] are measured
 * from the encoder's own counters, because DEC-020 records that the platform
 * ignores the requested frame rate: on the validated device the encoder runs at
 * ~120 fps whatever KEY_FRAME_RATE says, so a configured value cannot be
 * reported as the actual one.
 */
data class LiveCapture(
    val width: Int,
    val height: Int,
    val bitrateKbps: Int,
    val codec: String,
    /** Measured encoder output rate (AUs/second). */
    val encodedFps: Double,
    /** Measured AUs per GOP, or the configured expectation before the first keyframe. */
    val gopAus: Int,
    /** AUs the GOP-tail throttle keeps per GOP. */
    val keepFrames: Int,
    /** True when [gopAus] is still the configured expectation rather than a measurement. */
    val gopEstimated: Boolean = false,
) {
    /** The rate the receiver will actually see after prediction-safe throttling. */
    val deliveredFps: Int
        get() {
            if (gopAus <= 0 || encodedFps <= 0.0) return 0
            val kept = minOf(keepFrames, gopAus).toDouble()
            return Math.round(encodedFps * kept / gopAus).toInt()
        }

    /** The tuple this device is actually applying, for the negotiation answer. */
    fun actualParams(): SessionMediaParams = SessionMediaParams(
        width = width,
        height = height,
        fps = deliveredFps,
        bitrateKbps = bitrateKbps,
        codec = if (codec.isEmpty()) "h264" else codec,
    )
}

/** A parsed POST /session/offer body. */
data class SessionOfferRequest(
    val protocolVersion: Int = 0,
    val minVersion: Int = 0,
    val maxVersion: Int = 0,
    val capabilities: List<String> = emptyList(),
    val requested: SessionMediaParams = SessionMediaParams(),
    /**
     * True when the peer sent a protocol_version field at all. Absent means an
     * older build that predates DEC-022; it is accepted without the negotiation
     * fields being echoed, so a rollout is not gated on both sides upgrading at
     * once. It is NOT treated as version 1.
     */
    val hasVersion: Boolean = false,
) {
    companion object {
        fun parse(obj: JSONObject?): SessionOfferRequest {
            if (obj == null) return SessionOfferRequest()
            val caps = mutableListOf<String>()
            val arr = obj.optJSONArray("capabilities")
            if (arr != null) {
                for (i in 0 until arr.length()) {
                    arr.optString(i).takeIf { it.isNotEmpty() }?.let { caps.add(it) }
                }
            }
            val versionObj = obj.optJSONObject("version")
            return SessionOfferRequest(
                protocolVersion = obj.optInt("protocol_version", 0),
                minVersion = versionObj?.optInt("min", 0) ?: 0,
                maxVersion = versionObj?.optInt("max", 0) ?: 0,
                capabilities = caps,
                requested = SessionMediaParams.fromJson(obj.optJSONObject("requested")),
                hasVersion = obj.has("protocol_version"),
            )
        }
    }
}

/** The typed answer to a session request. */
sealed interface SessionNegotiationResult {
    /**
     * @param actual the tuple this device will apply, or null when capture is not
     *               running yet and it therefore cannot be reported honestly.
     */
    data class Accepted(val actual: SessionMediaParams?, val message: String) : SessionNegotiationResult

    data class Rejected(val httpStatus: Int, val code: String, val message: String) : SessionNegotiationResult
}

/** A handler's answer: the SDP offer, its typed outcome, and the device's advertised capabilities. */
data class SessionOfferAnswer(
    val sdp: String,
    val result: SessionNegotiationResult,
    val capabilities: DeviceMediaCapabilities = DeviceMediaCapabilities(),
)

/**
 * The negotiation rules (DEC-022). Ordering matters: compatibility is settled
 * before the request is judged, so a peer speaking a different contract gets a
 * version error rather than a confusing parameter error, and a rejected request
 * never reaches the point of creating a peer connection.
 */
object SessionNegotiation {
    const val PROTOCOL_VERSION = 1

    // Typed codes, matching phonebridge.v1.Code / the core's engine.Code.
    const val CODE_OK = "OK"
    const val CODE_INCOMPATIBLE_VERSION = "INCOMPATIBLE_VERSION"
    const val CODE_UNSUPPORTED_MEDIA_PARAMS = "UNSUPPORTED_MEDIA_PARAMS"
    const val CODE_INVALID_ARGUMENT = "INVALID_ARGUMENT"
    const val CODE_CONSENT_REVOKED = "CONSENT_REVOKED"
    const val CODE_CAPTURE_FAILED = "CAPTURE_FAILED"
    const val CODE_TRANSPORT_FAILED = "TRANSPORT_FAILED"

    /**
     * Reserved: reported when this device already serves a different peer. The
     * single-peer product cannot reach it yet; it exists so the code is defined
     * in one place when multi-peer support lands.
     */
    const val CODE_SESSION_BUSY = "SESSION_BUSY"

    private const val HTTP_CONFLICT = 409
    private const val HTTP_BAD_REQUEST = 400

    fun negotiate(
        request: SessionOfferRequest,
        live: LiveCapture?,
        capabilities: DeviceMediaCapabilities,
    ): SessionNegotiationResult {
        if (!capabilities.supportsScreen) {
            return SessionNegotiationResult.Rejected(
                HTTP_CONFLICT,
                CODE_UNSUPPORTED_MEDIA_PARAMS,
                "this device has no screen-capture capability",
            )
        }

        // 1. Compatibility. A peer that states a range is compatible when this
        //    build's version falls inside it; otherwise the versions must match.
        //    There is no downgrade path this milestone, by decision.
        if (request.hasVersion) {
            val rangeStated = request.minVersion > 0 || request.maxVersion > 0
            val inRange = rangeStated &&
                PROTOCOL_VERSION >= request.minVersion &&
                (request.maxVersion == 0 || PROTOCOL_VERSION <= request.maxVersion)
            val exact = request.protocolVersion == PROTOCOL_VERSION
            if (!inRange && !exact) {
                return SessionNegotiationResult.Rejected(
                    HTTP_CONFLICT,
                    CODE_INCOMPATIBLE_VERSION,
                    "peer speaks session protocol ${request.protocolVersion}" +
                        (if (rangeStated) " (range ${request.minVersion}-${request.maxVersion})" else "") +
                        ", this device speaks $PROTOCOL_VERSION",
                )
            }
        }

        val req = request.requested

        // 2. Codec: this build implements exactly one capture path (DEC-021).
        if (req.codec.isNotEmpty() && !isSupportedCodec(req.codec, capabilities)) {
            return SessionNegotiationResult.Rejected(
                HTTP_CONFLICT,
                CODE_UNSUPPORTED_MEDIA_PARAMS,
                "codec '${req.codec}' is not supported (this device encodes ${capabilities.codecs.joinToString()})",
            )
        }

        // 3. Frame rate and bitrate are advisory (DEC-020: the platform's frame
        //    rate request is ignored and its bitrate control is loose), so they
        //    are range-checked and then reported as applied, never enforced.
        if (req.fps < 0 || req.bitrateKbps < 0) {
            return SessionNegotiationResult.Rejected(
                HTTP_BAD_REQUEST,
                CODE_INVALID_ARGUMENT,
                "fps and bitrate_kbps must not be negative",
            )
        }
        if (req.fps > MAX_REASONABLE_FPS) {
            return SessionNegotiationResult.Rejected(
                HTTP_CONFLICT,
                CODE_UNSUPPORTED_MEDIA_PARAMS,
                "requested ${req.fps} fps exceeds the supported maximum of $MAX_REASONABLE_FPS",
            )
        }

        // 4. Geometry is consent-bound, so it must be satisfiable exactly.
        val geometryRequested = req.width > 0 || req.height > 0
        if (geometryRequested) {
            if (req.width <= 0 || req.height <= 0) {
                return SessionNegotiationResult.Rejected(
                    HTTP_BAD_REQUEST,
                    CODE_INVALID_ARGUMENT,
                    "width and height must both be present",
                )
            }
            if (live != null && (live.width != req.width || live.height != req.height)) {
                return SessionNegotiationResult.Rejected(
                    HTTP_CONFLICT,
                    CODE_UNSUPPORTED_MEDIA_PARAMS,
                    "capture is already running at ${live.width}x${live.height}; " +
                        "a geometry change needs a new screen-capture consent (DEC-020)",
                )
            }
            if (!capabilities.supportsSize(req.width, req.height)) {
                return SessionNegotiationResult.Rejected(
                    HTTP_CONFLICT,
                    CODE_UNSUPPORTED_MEDIA_PARAMS,
                    "encoder cannot capture ${req.width}x${req.height}",
                )
            }
        } else if (live == null) {
            // No geometry asked for and nothing captured yet: there is no actual
            // tuple to report, and inferring one would be exactly the silent
            // substitution this contract exists to prevent.
            return SessionNegotiationResult.Accepted(
                actual = null,
                message = "capture is not running; the applied parameters will be reported once sharing starts",
            )
        }

        val actual = live?.actualParams()
        val message = when {
            live == null ->
                "requested parameters accepted; capture is not running yet"
            actual == null ->
                "accepted"
            else ->
                "capture ${live.width}x${live.height}, encoder ~${Math.round(live.encodedFps)} fps, " +
                    "GOP ${live.gopAus} AUs${if (live.gopEstimated) " (expected, not yet measured)" else " (measured)"}, " +
                    "delivering ~${live.deliveredFps} fps; fps and bitrate are advisory (DEC-020)"
        }
        return SessionNegotiationResult.Accepted(actual = actual, message = message)
    }

    /** Highest frame rate this contract will even consider. */
    const val MAX_REASONABLE_FPS = 240

    private fun isSupportedCodec(codec: String, capabilities: DeviceMediaCapabilities): Boolean {
        val normalized = codec.lowercase().trim()
        val aliases = when (normalized) {
            "video/avc", "avc", "h.264" -> "h264"
            else -> normalized
        }
        return capabilities.codecs.any { it.lowercase() == aliases } ||
            capabilities.codecs.any { it.lowercase() == normalized }
    }
}

/** Small helpers for composing the HTTP response body. */
internal object SessionNegotiationJson {
    fun capabilitiesArray(capabilities: DeviceMediaCapabilities): JSONArray = JSONArray().apply {
        put(
            JSONObject().apply {
                put("codecs", JSONArray(capabilities.codecs))
                if (capabilities.maxWidth > 0) put("max_width", capabilities.maxWidth)
                if (capabilities.maxHeight > 0) put("max_height", capabilities.maxHeight)
                if (capabilities.maxFps > 0) put("max_fps", capabilities.maxFps)
                put("supports_screen", capabilities.supportsScreen)
            },
        )
    }

    /**
     * Builds the /session/offer answer. A rejected request carries no `sdp` and
     * no `accepted:true`, so a peer cannot mistake a refusal for a usable offer.
     */
    fun offerResponse(answer: SessionOfferAnswer): String {
        val obj = JSONObject()
        obj.put("protocol_version", SessionNegotiation.PROTOCOL_VERSION)
        obj.put("capabilities", capabilitiesArray(answer.capabilities))
        when (val result = answer.result) {
            is SessionNegotiationResult.Accepted -> {
                obj.put("type", "offer")
                obj.put("sdp", answer.sdp)
                obj.put("accepted", true)
                result.actual?.let { obj.put("actual", it.toJson()) }
                if (result.message.isNotEmpty()) obj.put("message", result.message)
                obj.put("code", SessionNegotiation.CODE_OK)
            }
            is SessionNegotiationResult.Rejected -> {
                obj.put("accepted", false)
                obj.put("code", result.code)
                obj.put("message", result.message)
            }
        }
        return obj.toString()
    }
}
