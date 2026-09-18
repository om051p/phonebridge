package dev.phonebridge.signaling

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

    private val testHandler = object : LanSignalingServer.SignalingHandler {
        override fun handleOffer(): ByteArray {
            offerCalled = true
            return """{"type":"offer","sdp":"mock-sdp-offer"}""".toByteArray(StandardCharsets.UTF_8)
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

    @Before
    fun setUp() {
        offerCalled = false
        answerCalled = false
        stopCalled = false
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
}
