package dev.phonebridge.signaling

import androidx.test.platform.app.InstrumentationRegistry
import dev.phonebridge.security.AuthValidator
import dev.phonebridge.security.CryptoUtils
import dev.phonebridge.security.DeviceIdentityManager
import dev.phonebridge.security.TrustStore
import org.json.JSONObject
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import java.io.File
import java.net.HttpURLConnection
import java.net.URL
import java.nio.charset.StandardCharsets

/**
 * Validates on-device Keystore-backed Ed25519 identity, TrustStore,
 * LAN pairing handshake (SAS), and authenticated signaling with anti-replay.
 * Ratified under DEC-007 and DEC-011.
 */
class LanSignalingSecurityDeviceTest {

    private val testPort = 18805
    private lateinit var server: LanSignalingServer
    private lateinit var trustStore: TrustStore
    private lateinit var androidIdentity: DeviceIdentityManager
    private lateinit var clientKeyPair: java.security.KeyPair
    private lateinit var clientRawPub: ByteArray
    private lateinit var clientDeviceId: String

    private val testHandler = object : LanSignalingServer.SignalingHandler {
        override fun handleOffer(): ByteArray {
            return """{"type":"offer","sdp":"v=0\r\no=- 123 2 IN IP4 127.0.0.1"}""".toByteArray(StandardCharsets.UTF_8)
        }
        override fun handleAnswer(answerJson: ByteArray): Boolean = true
        override fun handleStop(reason: String) {}
    }

    @Before
    fun setUp() {
        val context = InstrumentationRegistry.getInstrumentation().targetContext
        val storeFile = File(context.filesDir, "test_trusted_devices.json")
        if (storeFile.exists()) {
            storeFile.delete()
        }
        trustStore = TrustStore(storeFile)

        // Generate on-device Keystore-backed identity
        androidIdentity = DeviceIdentityManager.loadOrGenerate(context)
        assertNotNull("device identity must be generated with Keystore backing", androidIdentity)
        assertEquals(64, androidIdentity.deviceId.length)

        // Generate simulated client identity
        clientKeyPair = CryptoUtils.generateKeyPair()
        clientRawPub = CryptoUtils.rawPublicKey(clientKeyPair.public)
        clientDeviceId = CryptoUtils.fingerprint(clientRawPub)

        server = LanSignalingServer(
            port = testPort,
            handler = testHandler,
            identityManager = androidIdentity,
            trustStore = trustStore
        )
        assertTrue("LanSignalingServer should start on device", server.start())
    }

    @After
    fun tearDown() {
        if (::server.isInitialized) {
            server.stop()
        }
    }

    private fun signRequest(
        method: String,
        path: String,
        body: ByteArray,
        timestampMs: Long = System.currentTimeMillis(),
        nonce: String = "dev-nonce-${System.nanoTime()}"
    ): Map<String, String> {
        val bodyHash = CryptoUtils.toHex(CryptoUtils.sha256(body))
        val material = "$method\n$path\n$timestampMs\n$nonce\n$bodyHash"
        val sig = CryptoUtils.sign(clientKeyPair.private, material.toByteArray(StandardCharsets.UTF_8))
        return mapOf(
            AuthValidator.HEADER_DEVICE_ID to clientDeviceId,
            AuthValidator.HEADER_TIMESTAMP to timestampMs.toString(),
            AuthValidator.HEADER_NONCE to nonce,
            AuthValidator.HEADER_SIGNATURE to CryptoUtils.toHex(sig)
        )
    }

    private fun executeHttp(
        method: String,
        path: String,
        body: ByteArray? = null,
        headers: Map<String, String> = emptyMap()
    ): Pair<Int, String> {
        val url = URL("http://127.0.0.1:$testPort$path")
        val conn = url.openConnection() as HttpURLConnection
        conn.requestMethod = method
        conn.connectTimeout = 3000
        conn.readTimeout = 3000
        for ((k, v) in headers) {
            conn.setRequestProperty(k, v)
        }
        if (body != null && body.isNotEmpty()) {
            conn.doOutput = true
            conn.setRequestProperty("Content-Type", "application/json")
            conn.outputStream.use { it.write(body) }
        }
        val code = conn.responseCode
        val stream = if (code in 200..299) conn.inputStream else conn.errorStream
        val text = stream?.bufferedReader(StandardCharsets.UTF_8)?.use { it.readText() } ?: ""
        return Pair(code, text)
    }

    @Test
    fun testOnDevicePairingAndAuthenticatedSignalingFlow() {
        // 1. Unauthenticated request to /session/offer is rejected (HTTP 401/403)
        val (unauthCode, _) = executeHttp("POST", "/session/offer", ByteArray(0))
        assertEquals(401, unauthCode)

        // 2. Perform pairing handshake
        val token = "token-device-test-${System.currentTimeMillis()}"
        val reqPayload = JSONObject().apply {
            put("display_name", "Linux Test Desktop")
            put("platform", "linux")
            put("public_key", CryptoUtils.toHex(clientRawPub))
            put("pairing_token", token)
        }
        val (reqCode, reqResp) = executeHttp("POST", "/pairing/request", reqPayload.toString().toByteArray())
        assertEquals(200, reqCode)
        val acceptJson = JSONObject(reqResp)
        val sas = acceptJson.getString("sas")
        assertEquals(6, sas.length)

        // Verify symmetric SAS matches
        val expectedSAS = CryptoUtils.calculateSAS(clientRawPub, androidIdentity.rawPublicKey, token)
        assertEquals(expectedSAS, sas)

        // 3. Confirm pairing with Ed25519 signature over "$token:$sas"
        val sigMaterial = "$token:$sas".toByteArray(StandardCharsets.UTF_8)
        val sig = CryptoUtils.sign(clientKeyPair.private, sigMaterial)
        val confirmPayload = JSONObject().apply {
            put("device_id", clientDeviceId)
            put("pairing_token", token)
            put("sas", sas)
            put("confirmed", true)
            put("signature", CryptoUtils.toHex(sig))
        }
        val (confCode, _) = executeHttp("POST", "/pairing/confirm", confirmPayload.toString().toByteArray())
        assertEquals(200, confCode)
        assertTrue("client must be trusted in on-device trust store", trustStore.isTrusted(clientDeviceId))

        // 4. Authenticated request to /session/offer succeeds (HTTP 200)
        val nonce = "device-test-nonce-1"
        val headers = signRequest("POST", "/session/offer", ByteArray(0), nonce = nonce)
        val (offerCode, offerResp) = executeHttp("POST", "/session/offer", ByteArray(0), headers)
        assertEquals(200, offerCode)
        assertTrue("response contains SDP offer", offerResp.contains("v=0"))

        // 5. Anti-replay defense: Replaying the identical request fails with HTTP 401
        val (replayCode, _) = executeHttp("POST", "/session/offer", ByteArray(0), headers)
        assertEquals(401, replayCode)

        // 6. Revocation defense: Revoke the client in trust store
        trustStore.revoke(clientDeviceId)
        val newHeaders = signRequest("POST", "/session/offer", ByteArray(0), nonce = "device-test-nonce-2")
        val (revokedCode, _) = executeHttp("POST", "/session/offer", ByteArray(0), newHeaders)
        assertEquals(403, revokedCode)
    }
}
