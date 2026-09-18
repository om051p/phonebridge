package dev.phonebridge.signaling

import androidx.test.platform.app.InstrumentationRegistry
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import java.net.HttpURLConnection
import java.net.URL
import java.nio.charset.StandardCharsets

class LanSignalingServerDeviceTest {

    private lateinit var server: LanSignalingServer
    private val testPort = 18804

    @Before
    fun setUp() {
        server = LanSignalingServer(port = testPort)
        assertTrue("server should start on device", server.start())
    }

    @After
    fun tearDown() {
        server.stop()
    }

    @Test
    fun runsHttpSignalingServerOnDevice() {
        val url = URL("http://127.0.0.1:$testPort/health")
        val conn = url.openConnection() as HttpURLConnection
        conn.connectTimeout = 3000
        conn.readTimeout = 3000
        conn.requestMethod = "GET"
        assertEquals(200, conn.responseCode)
        val body = conn.inputStream.bufferedReader().readText()
        assertTrue("health response should contain ok", body.contains(""""status":"ok""""))
    }
}
