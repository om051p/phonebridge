package dev.phonebridge.signaling

import org.json.JSONObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.nio.charset.StandardCharsets

/**
 * DEC-022 phone-side negotiation rules.
 *
 * The theme of these tests: the phone either agrees to exactly what was asked
 * for, or refuses with a typed reason. It never substitutes a different
 * geometry, because DEC-020 makes the capture geometry consent-bound and a
 * session that captures something other than the negotiated tuple would make
 * the whole handshake meaningless.
 */
class SessionNegotiationTest {

    private val capabilities = DeviceMediaCapabilities(
        codecs = listOf("h264"),
        maxWidth = 4096,
        maxHeight = 4096,
        maxFps = 240,
        widthAlignment = 2,
        heightAlignment = 2,
    )

    private fun liveCapture(
        width: Int = 720,
        height: Int = 1600,
        encodedFps: Double = 120.0,
        gopAus: Int = 30,
        keepFrames: Int = 8,
    ) = LiveCapture(
        width = width,
        height = height,
        bitrateKbps = 2500,
        codec = "h264",
        encodedFps = encodedFps,
        gopAus = gopAus,
        keepFrames = keepFrames,
    )

    private fun request(
        requested: SessionMediaParams = SessionMediaParams(),
        protocolVersion: Int = SessionNegotiation.PROTOCOL_VERSION,
        hasVersion: Boolean = true,
        min: Int = 0,
        max: Int = 0,
    ) = SessionOfferRequest(
        protocolVersion = protocolVersion,
        minVersion = min,
        maxVersion = max,
        capabilities = listOf("SCREEN"),
        requested = requested,
        hasVersion = hasVersion,
    )

    private fun negotiate(req: SessionOfferRequest, live: LiveCapture? = null) =
        SessionNegotiation.negotiate(req, live, capabilities)

    // ------------------------------------------------------------- compatibility

    @Test
    fun `matching version and range are accepted`() {
        val result = negotiate(request(protocolVersion = 1, min = 1, max = 1), liveCapture())
        assertTrue("expected acceptance, got $result", result is SessionNegotiationResult.Accepted)
    }

    @Test
    fun `version inside the stated range is accepted`() {
        val result = negotiate(request(protocolVersion = 7, min = 1, max = 3), liveCapture())
        assertTrue("expected acceptance, got $result", result is SessionNegotiationResult.Accepted)
    }

    @Test
    fun `version mismatch is refused without a fallback`() {
        val result = negotiate(request(protocolVersion = 9), liveCapture())
        assertTrue(result is SessionNegotiationResult.Rejected)
        result as SessionNegotiationResult.Rejected
        assertEquals(SessionNegotiation.CODE_INCOMPATIBLE_VERSION, result.code)
        assertEquals(409, result.httpStatus)
    }

    @Test
    fun `version outside the stated range is refused`() {
        val result = negotiate(request(protocolVersion = 9, min = 7, max = 8), liveCapture())
        result as SessionNegotiationResult.Rejected
        assertEquals(SessionNegotiation.CODE_INCOMPATIBLE_VERSION, result.code)
    }

    @Test
    fun `a peer that reports no version is still served`() {
        // Pre-DEC-022 builds post no body and must keep working; the negotiation
        // fields simply go unreported rather than being invented.
        val result = negotiate(request(hasVersion = false), liveCapture())
        assertTrue(result is SessionNegotiationResult.Accepted)
    }

    // ---------------------------------------------------------------- parameters

    @Test
    fun `geometry matching the live capture is accepted and reported as actual`() {
        val result = negotiate(
            request(requested = SessionMediaParams(width = 720, height = 1600, fps = 30, bitrateKbps = 2500)),
            liveCapture(),
        ) as SessionNegotiationResult.Accepted

        val actual = result.actual
        assertNotNull("actual tuple must be reported while capturing", actual)
        assertEquals(720, actual!!.width)
        assertEquals(1600, actual.height)
        assertEquals("h264", actual.codec)
    }

    @Test
    fun `reported fps is the delivered rate not the requested or configured one`() {
        // DEC-020: the encoder ignores the requested frame rate and runs at its own
        // rate, so reporting 30 simply because 30 was asked for would be a lie.
        val actual = (
            negotiate(
                request(requested = SessionMediaParams(width = 720, height = 1600, fps = 30)),
                liveCapture(encodedFps = 120.0, gopAus = 30, keepFrames = 8),
            ) as SessionNegotiationResult.Accepted
            ).actual!!

        // 120 encoded fps / 30-AU GOP * 8 kept AUs = 32 delivered fps.
        assertEquals(32, actual.fps)
    }

    @Test
    fun `geometry differing from the live capture is refused, never substituted`() {
        val result = negotiate(
            request(requested = SessionMediaParams(width = 1080, height = 2400)),
            liveCapture(width = 720, height = 1600),
        ) as SessionNegotiationResult.Rejected

        assertEquals(SessionNegotiation.CODE_UNSUPPORTED_MEDIA_PARAMS, result.code)
        assertEquals(409, result.httpStatus)
        assertTrue(
            "the reason must name the consent boundary: ${result.message}",
            result.message.contains("consent"),
        )
    }

    @Test
    fun `geometry beyond the encoder bounds is refused`() {
        val result = negotiate(
            request(requested = SessionMediaParams(width = 8000, height = 8000)),
            live = null,
        ) as SessionNegotiationResult.Rejected
        assertEquals(SessionNegotiation.CODE_UNSUPPORTED_MEDIA_PARAMS, result.code)
    }

    @Test
    fun `geometry violating encoder alignment is refused`() {
        val result = negotiate(
            request(requested = SessionMediaParams(width = 721, height = 1601)),
            live = null,
        ) as SessionNegotiationResult.Rejected
        assertEquals(SessionNegotiation.CODE_UNSUPPORTED_MEDIA_PARAMS, result.code)
    }

    @Test
    fun `unsupported codec is refused`() {
        val result = negotiate(
            request(requested = SessionMediaParams(codec = "vp9")),
            liveCapture(),
        ) as SessionNegotiationResult.Rejected
        assertEquals(SessionNegotiation.CODE_UNSUPPORTED_MEDIA_PARAMS, result.code)
        assertTrue(result.message.contains("vp9"))
    }

    @Test
    fun `codec aliases for h264 are accepted`() {
        for (alias in listOf("h264", "H264", "video/avc", "avc", "h.264")) {
            val result = negotiate(
                request(requested = SessionMediaParams(codec = alias, width = 720, height = 1600)),
                liveCapture(),
            )
            assertTrue("alias '$alias' should be accepted, got $result", result is SessionNegotiationResult.Accepted)
        }
    }

    @Test
    fun `negative parameters are invalid`() {
        assertTrue(negotiate(request(requested = SessionMediaParams(fps = -1)), liveCapture()) is SessionNegotiationResult.Rejected)
        assertTrue(
            negotiate(request(requested = SessionMediaParams(bitrateKbps = -5)), liveCapture())
                is SessionNegotiationResult.Rejected,
        )
    }

    @Test
    fun `absurd frame rate is refused rather than silently clamped`() {
        val result = negotiate(request(requested = SessionMediaParams(fps = 10_000)), liveCapture())
        result as SessionNegotiationResult.Rejected
        assertEquals(SessionNegotiation.CODE_UNSUPPORTED_MEDIA_PARAMS, result.code)
    }

    @Test
    fun `half specified geometry is invalid`() {
        val result = negotiate(request(requested = SessionMediaParams(width = 720)), liveCapture())
        result as SessionNegotiationResult.Rejected
        assertEquals(SessionNegotiation.CODE_INVALID_ARGUMENT, result.code)
    }

    @Test
    fun `advisory fps and bitrate are tolerated and reported as applied`() {
        // fps and bitrate are advisory (DEC-020 loose bitrate control), so a
        // disagreement is reported, not refused: the actual tuple carries what
        // the device really does.
        val result = negotiate(
            request(requested = SessionMediaParams(width = 720, height = 1600, fps = 60, bitrateKbps = 8000)),
            liveCapture(),
        ) as SessionNegotiationResult.Accepted

        assertEquals(32, result.actual?.fps)
        assertEquals(2500, result.actual?.bitrateKbps)
        assertTrue(result.message.contains("advisory"))
    }

    // -------------------------------------------------------------- actual tuple

    @Test
    fun `no capture means no actual tuple is claimed`() {
        val result = negotiate(request(requested = SessionMediaParams(width = 720, height = 1600)), live = null)
            as SessionNegotiationResult.Accepted

        // Reporting the request back as "actual" would be the silent substitution
        // this contract exists to prevent.
        assertNull(result.actual)
        assertTrue(result.message.contains("not running"))
    }

    @Test
    fun `no request and no capture is accepted without an actual tuple`() {
        val result = negotiate(request(), live = null) as SessionNegotiationResult.Accepted
        assertNull(result.actual)
    }

    @Test
    fun `a device without screen capability refuses everything`() {
        val result = SessionNegotiation.negotiate(
            request(),
            liveCapture(),
            DeviceMediaCapabilities(supportsScreen = false, codecs = emptyList()),
        ) as SessionNegotiationResult.Rejected
        assertEquals(SessionNegotiation.CODE_UNSUPPORTED_MEDIA_PARAMS, result.code)
    }

    @Test
    fun `unknown capability bounds are not treated as a refusal`() {
        val result = SessionNegotiation.negotiate(
            request(requested = SessionMediaParams(width = 1080, height = 2400)),
            liveCapture(width = 1080, height = 2400),
            DeviceMediaCapabilities(), // no bounds reported
        )
        assertTrue("absent bounds must not reject a valid request, got $result", result is SessionNegotiationResult.Accepted)
    }

    // ------------------------------------------------------------- response shape

    @Test
    fun `accepted response carries the offer, the actual tuple and the code`() {
        val answer = SessionOfferAnswer(
            sdp = "v=0-offer",
            result = SessionNegotiationResult.Accepted(
                actual = SessionMediaParams(width = 720, height = 1600, fps = 32, bitrateKbps = 2500, codec = "h264"),
                message = "ok",
            ),
            capabilities = capabilities,
        )
        val json = JSONObject(SessionNegotiationJson.offerResponse(answer))
        assertEquals(1, json.getInt("protocol_version"))
        assertEquals("OK", json.getString("code"))
        assertTrue(json.getBoolean("accepted"))
        assertEquals("v=0-offer", json.getString("sdp"))
        assertEquals(720, json.getJSONObject("actual").getInt("width"))
        assertEquals(true, json.getJSONArray("capabilities").getJSONObject(0).getBoolean("supports_screen"))
    }

    @Test
    fun `accepted response omits an unreported actual tuple`() {
        val answer = SessionOfferAnswer(
            sdp = "v=0-offer",
            result = SessionNegotiationResult.Accepted(actual = null, message = "not capturing"),
            capabilities = capabilities,
        )
        val json = JSONObject(SessionNegotiationJson.offerResponse(answer))
        assertTrue("an unknown tuple must be absent, not zero-filled", !json.has("actual"))
    }

    @Test
    fun `rejected response carries no offer`() {
        val answer = SessionOfferAnswer(
            sdp = "",
            result = SessionNegotiationResult.Rejected(409, SessionNegotiation.CODE_INCOMPATIBLE_VERSION, "nope"),
            capabilities = capabilities,
        )
        val json = JSONObject(SessionNegotiationJson.offerResponse(answer))
        assertEquals(false, json.getBoolean("accepted"))
        assertEquals("INCOMPATIBLE_VERSION", json.getString("code"))
        assertTrue(!json.has("sdp"))
    }

    // ------------------------------------------------------------------- parsing

    @Test
    fun `request parsing reads version, capabilities and the requested tuple`() {
        val body = """
            {"protocol_version":1,"version":{"min":1,"max":2},"capabilities":["SCREEN","FILES"],
             "requested":{"width":720,"height":1600,"fps":30,"bitrate_kbps":4000,"codec":"h264"}}
        """.trimIndent()
        val parsed = SessionOfferRequest.parse(JSONObject(body))

        assertEquals(1, parsed.protocolVersion)
        assertEquals(1, parsed.minVersion)
        assertEquals(2, parsed.maxVersion)
        assertEquals(listOf("SCREEN", "FILES"), parsed.capabilities)
        assertEquals(720, parsed.requested.width)
        assertEquals(4000, parsed.requested.bitrateKbps)
        assertTrue(parsed.hasVersion)
    }

    @Test
    fun `request parsing tolerates a peer that omits everything`() {
        val parsed = SessionOfferRequest.parse(JSONObject("{}"))
        assertTrue(parsed.requested.isZero())
        assertEquals(false, parsed.hasVersion)
        assertTrue(parsed.capabilities.isEmpty())
    }

    @Test
    fun `zero valued parameters are omitted when serialized`() {
        val json = SessionMediaParams(width = 720, height = 1600).toJson()
        assertEquals(720, json.getInt("width"))
        assertTrue("absent fields must stay absent", !json.has("fps") && !json.has("codec"))
    }

    @Test
    fun `delivered fps falls back safely before any GOP is measured`() {
        val live = liveCapture(encodedFps = 0.0, gopAus = 0)
        assertEquals(0, live.deliveredFps) // no measurement yet: report nothing rather than a guess
    }

    @Test
    fun `gop shorter than the keep count delivers every encoded frame`() {
        // A 4-AU GOP with 8 kept frames must not report more than the encoder
        // produces.
        val live = liveCapture(encodedFps = 120.0, gopAus = 4, keepFrames = 8)
        assertEquals(120, live.deliveredFps)
    }

    @Test
    fun `legacy body is treated as a pre-DEC-022 peer`() {
        val server = LanSignalingServer(port = 0)
        val parsed = server.parseOfferRequest("".toByteArray(StandardCharsets.UTF_8))
        assertEquals(false, parsed.hasVersion)
        assertEquals(0, parsed.requested.width)
    }
}
