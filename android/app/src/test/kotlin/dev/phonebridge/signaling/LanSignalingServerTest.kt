package dev.phonebridge.signaling

import org.json.JSONObject
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import java.io.BufferedReader
import java.io.InputStreamReader
import java.net.HttpURLConnection
import java.net.URL
import java.nio.charset.StandardCharsets

class LanSignalingServerTest {

    private lateinit var server: LanSignalingServer
    private var testPort = 17804

    private var offerCalled = false
    private var answerCalled = false
    private var stopCalled = false
    private var lastRequest: SessionOfferRequest? = null

    private val testHandler = object : LanSignalingServer.SignalingHandler {
        override fun handleOffer(request: SessionOfferRequest): SessionOfferAnswer {
            offerCalled = true
            lastRequest = request
            if (rejectWith != null) {
                return SessionOfferAnswer(sdp = "", result = rejectWith!!)
            }
            return SessionOfferAnswer(
                sdp = "mock-sdp-offer",
                result = SessionNegotiationResult.Accepted(
                    actual = SessionMediaParams(width = 720, height = 1600, fps = 30, bitrateKbps = 2500, codec = "h264"),
                    message = "capture 720x1600",
                ),
            )
        }

        override fun handleAnswer(answerJson: ByteArray): Boolean {
            answerCalled = true
            val str = String(answerJson, StandardCharsets.UTF_8)
            return str.contains("mock-sdp-answer")
        }

        override fun handleStop(reason: String) {
            stopCalled = true
        }
    }

    private var rejectWith: SessionNegotiationResult.Rejected? = null

    @Before
    fun setUp() {
        offerCalled = false
        answerCalled = false
        stopCalled = false
        lastRequest = null
        rejectWith = null
        server = LanSignalingServer(port = testPort, handler = testHandler)
        assertTrue("server should start", server.start())
        assertTrue("server should be running", server.running)
    }

    @After
    fun tearDown() {
        server.stop()
        assertTrue("server should not be running after stop", !server.running)
    }

    @Test
    fun testHealthEndpoint() {
        val url = URL("http://127.0.0.1:$testPort/health")
        val conn = url.openConnection() as HttpURLConnection
        conn.requestMethod = "GET"
        assertEquals(200, conn.responseCode)
        val body = conn.inputStream.bufferedReader().readText()
        assertTrue(body.contains(""""status":"ok""""))
    }

    @Test
    fun testOfferEndpoint() {
        val url = URL("http://127.0.0.1:$testPort/session/offer")
        val conn = url.openConnection() as HttpURLConnection
        conn.requestMethod = "POST"
        conn.doOutput = true
        assertEquals(200, conn.responseCode)
        val body = conn.inputStream.bufferedReader().readText()
        assertTrue(offerCalled)
        assertTrue(body.contains("mock-sdp-offer"))
    }

    @Test
    fun testOfferEndpointReportsNegotiatedParameters() {
        val url = URL("http://127.0.0.1:$testPort/session/offer")
        val conn = url.openConnection() as HttpURLConnection
        conn.requestMethod = "POST"
        conn.doOutput = true
        conn.outputStream.write(
            """{"protocol_version":1,"version":{"min":1,"max":1},"capabilities":["SCREEN"],"requested":{"width":720,"height":1600,"fps":30}}"""
                .toByteArray(StandardCharsets.UTF_8),
        )
        assertEquals(200, conn.responseCode)
        val body = conn.inputStream.bufferedReader().readText()
        val json = JSONObject(body)

        assertTrue("offer must be present", json.getString("sdp").contains("mock-sdp-offer"))
        assertTrue("accepted flag", json.getBoolean("accepted"))
        assertEquals(1, json.getInt("protocol_version"))
        assertEquals(30, json.getJSONObject("actual").getInt("fps"))
        assertEquals("h264", json.getJSONObject("actual").getString("codec"))

        // The request must reach the handler parsed, not as an opaque blob.
        assertEquals(1, lastRequest?.protocolVersion)
        assertEquals(720, lastRequest?.requested?.width)
        assertEquals(listOf("SCREEN"), lastRequest?.capabilities)
    }

    @Test
    fun testOfferRejectionIsTypedAndCarriesNoOffer() {
        rejectWith = SessionNegotiationResult.Rejected(
            httpStatus = 409,
            code = SessionNegotiation.CODE_UNSUPPORTED_MEDIA_PARAMS,
            message = "encoder cannot capture 4000x4000",
        )

        val url = URL("http://127.0.0.1:$testPort/session/offer")
        val conn = url.openConnection() as HttpURLConnection
        conn.requestMethod = "POST"
        conn.doOutput = true
        conn.outputStream.write("""{"protocol_version":1}""".toByteArray(StandardCharsets.UTF_8))

        assertEquals(409, conn.responseCode)
        val body = conn.errorStream.bufferedReader().readText()
        val json = JSONObject(body)
        assertTrue("refusal must not claim acceptance", !json.getBoolean("accepted"))
        assertEquals("UNSUPPORTED_MEDIA_PARAMS", json.getString("code"))
        assertTrue("a refusal must not carry an offer", !json.has("sdp"))
    }

    @Test
    fun testOfferWithLegacyBodyStillWorks() {
        // A pre-DEC-022 peer posts no body at all and must still get an offer.
        val url = URL("http://127.0.0.1:$testPort/session/offer")
        val conn = url.openConnection() as HttpURLConnection
        conn.requestMethod = "POST"
        conn.doOutput = true
        assertEquals(200, conn.responseCode)
        val body = conn.inputStream.bufferedReader().readText()
        assertTrue(body.contains("mock-sdp-offer"))
        assertEquals(false, lastRequest?.hasVersion)
    }

    @Test
    fun testAnswerEndpoint() {
        val url = URL("http://127.0.0.1:$testPort/session/answer")
        val conn = url.openConnection() as HttpURLConnection
        conn.requestMethod = "POST"
        conn.doOutput = true
        conn.outputStream.write("""{"type":"answer","sdp":"mock-sdp-answer"}""".toByteArray(StandardCharsets.UTF_8))
        assertEquals(200, conn.responseCode)
        val body = conn.inputStream.bufferedReader().readText()
        assertTrue(answerCalled)
        assertTrue(body.contains(""""status":"ok""""))
    }

    @Test
    fun testStopEndpoint() {
        val url = URL("http://127.0.0.1:$testPort/session/stop")
        val conn = url.openConnection() as HttpURLConnection
        conn.requestMethod = "POST"
        conn.doOutput = true
        conn.outputStream.write("""{"reason":"user disconnect"}""".toByteArray(StandardCharsets.UTF_8))
        assertEquals(200, conn.responseCode)
        val body = conn.inputStream.bufferedReader().readText()
        assertTrue(stopCalled)
        assertTrue(body.contains(""""status":"ok""""))
    }

    @Test
    fun testDefaultHandlerStopNotifiesHostCaptureTeardown() {
        // Regression: a peer-ended session (POST /session/stop) must reach the
        // host so it can stop the Kotlin capture pipeline. Tearing down only the
        // Go media transport left the screen encoder running after a
        // desktop-side disconnect (physical leak).
        val reasons = mutableListOf<String>()
        val handler = LanSignalingServer.DefaultSignalingHandler(
            onRemoteStop = { reasons.add(it) },
        )
        handler.handleStop("peer disconnected")
        assertEquals(listOf("peer disconnected"), reasons)
    }
}
