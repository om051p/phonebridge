package dev.phonebridge.signaling

import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import dev.phonebridge.bridge.GoBridge
import org.json.JSONObject
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import java.io.BufferedReader
import java.io.InputStreamReader
import java.net.HttpURLConnection
import java.net.URL
import java.nio.charset.StandardCharsets
import java.util.concurrent.atomic.AtomicReference

/**
 * On-device proof of the DEC-022 phone-side contract: the production
 * [LanSignalingServer.DefaultSignalingHandler] answers a real HTTP session
 * request with a real SDP offer built by the Go/Pion transport, and either
 * accepts with the applied media tuple or refuses with a typed code.
 *
 * This is the phone half of "parameters are settled before the offer exists":
 * the negotiation is decided here, and only an accepted request is allowed to
 * touch the media transport.
 */
@RunWith(AndroidJUnit4::class)
class SessionNegotiationDeviceTest {

    private val port = 17855
    private lateinit var server: LanSignalingServer
    private val liveCapture = AtomicReference<LiveCapture?>(null)

    @Before
    fun setUp() {
        val context = InstrumentationRegistry.getInstrumentation().targetContext
        if (GoBridge.loaded) {
            GoBridge.start(context.filesDir.absolutePath)
        }
        liveCapture.set(
            LiveCapture(
                width = 720,
                height = 1600,
                bitrateKbps = 2500,
                codec = "h264",
                encodedFps = 120.0,
                gopAus = 30,
                keepFrames = 8,
                gopEstimated = false,
            ),
        )

        val handler = LanSignalingServer.DefaultSignalingHandler(
            liveCapture = { liveCapture.get() },
            capabilities = {
                DeviceMediaCapabilities(
                    codecs = listOf("h264"),
                    maxWidth = 4096,
                    maxHeight = 4096,
                    maxFps = 240,
                    supportsScreen = true,
                )
            },
        )
        server = LanSignalingServer(port = port, handler = handler)
        assertTrue("signalling server must start", server.start())
    }

    @After
    fun tearDown() {
        server.stop()
        if (GoBridge.loaded) {
            GoBridge.mediaStop()
            GoBridge.mediaRelease()
            GoBridge.stop()
        }
    }

    private fun postOffer(body: String): Pair<Int, String> {
        val conn = URL("http://127.0.0.1:$port/session/offer").openConnection() as HttpURLConnection
        conn.requestMethod = "POST"
        conn.doOutput = true
        conn.outputStream.write(body.toByteArray(StandardCharsets.UTF_8))
        val code = conn.responseCode
        val stream = if (code in 200..299) conn.inputStream else conn.errorStream
        val text = BufferedReader(InputStreamReader(stream, StandardCharsets.UTF_8)).readText()
        return code to text
    }

    @Test
    fun acceptedRequestYieldsARealOfferAndTheAppliedTuple() {
        val (code, body) = postOffer(
            """{"protocol_version":1,"version":{"min":1,"max":1},"capabilities":["SCREEN"],
                "requested":{"width":720,"height":1600,"fps":30,"bitrate_kbps":2500,"codec":"h264"}}""",
        )
        assertEquals(200, code)
        val json = JSONObject(body)

        assertTrue("expected acceptance: $body", json.getBoolean("accepted"))
        assertEquals("OK", json.getString("code"))
        assertEquals(1, json.getInt("protocol_version"))

        // A real offer from the Go/Pion transport, not a placeholder.
        val sdp = json.getString("sdp")
        assertTrue("offer must carry the H.264 track: $sdp", sdp.contains("H264"))
        assertTrue("offer must be an offer: $sdp", sdp.contains("\"type\":\"offer\""))

        // The applied tuple is reported, and its fps is the *delivered* rate
        // (120 encoded fps through a 30-AU GOP keeping 8 AUs ≈ 32 fps), not the
        // 30 that was requested — DEC-020 records that the platform ignores the
        // requested frame rate, so echoing it back would be a fiction.
        val actual = json.getJSONObject("actual")
        assertEquals(720, actual.getInt("width"))
        assertEquals(1600, actual.getInt("height"))
        assertEquals(32, actual.getInt("fps"))

        val caps = json.getJSONArray("capabilities").getJSONObject(0)
        assertTrue(caps.getBoolean("supports_screen"))
        assertEquals("h264", caps.getJSONArray("codecs").getString(0))
    }

    @Test
    fun geometryConflictIsRefusedWithATypedCodeAndNoOffer() {
        // The live capture is 720x1600; asking for 1080x2400 cannot be satisfied
        // without a new consent, so the phone must refuse rather than silently
        // capture at the size it already has.
        val (code, body) = postOffer("""{"protocol_version":1,"requested":{"width":1080,"height":2400}}""")
        assertEquals(409, code)
        val json = JSONObject(body)

        assertFalse("a refusal must not claim acceptance", json.getBoolean("accepted"))
        assertEquals("UNSUPPORTED_MEDIA_PARAMS", json.getString("code"))
        assertFalse("a refusal must not carry an offer", json.has("sdp"))
    }

    @Test
    fun incompatibleVersionIsRefused() {
        val (code, body) = postOffer("""{"protocol_version":99,"requested":{"width":720,"height":1600}}""")
        assertEquals(409, code)
        assertEquals("INCOMPATIBLE_VERSION", JSONObject(body).getString("code"))
    }

    @Test
    fun noCaptureMeansNoClaimedActualTuple() {
        liveCapture.set(null)
        val (code, body) = postOffer("""{"protocol_version":1,"requested":{"width":720,"height":1600}}""")
        assertEquals(200, code)
        val json = JSONObject(body)
        assertTrue(json.getBoolean("accepted"))
        assertFalse("an unmeasured tuple must not be reported: $body", json.has("actual"))
    }
}
