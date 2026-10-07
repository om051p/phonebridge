package dev.phonebridge.security

import dev.phonebridge.signaling.LanSignalingServer
import dev.phonebridge.signaling.SessionNegotiationResult
import dev.phonebridge.signaling.SessionOfferAnswer
import dev.phonebridge.signaling.SessionOfferRequest
import org.json.JSONObject
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import java.io.File
import java.io.OutputStreamWriter
import java.net.HttpURLConnection
import java.net.URL
import java.nio.charset.StandardCharsets

class LanSignalingSecurityTest {

    @get:Rule
    val tempFolder = TemporaryFolder()

    private val testPort = 18804
    private lateinit var server: LanSignalingServer
    private lateinit var trustStore: TrustStore
    private lateinit var androidIdentity: DeviceIdentityManager
    private lateinit var clientKeyPair: java.security.KeyPair
    private lateinit var clientRawPub: ByteArray
    private lateinit var clientDeviceId: String

    private val testHandler = object : LanSignalingServer.SignalingHandler {
        override fun handleOffer(request: SessionOfferRequest): SessionOfferAnswer =
            SessionOfferAnswer(
                sdp = "mock-sdp-offer",
                result = SessionNegotiationResult.Accepted(actual = null, message = "test"),
            )
        override fun handleAnswer(answerJson: ByteArray): Boolean = true
        override fun handleStop(reason: String) {}
    }

    @Before
    fun setUp() {
        val storeFile = File(tempFolder.root, "trusted_devices.json")
        trustStore = TrustStore(storeFile)

        // Generate Android server identity
        val srvKp = CryptoUtils.generateKeyPair()
        val srvRawPub = CryptoUtils.rawPublicKey(srvKp.public)
        val srvDevId = CryptoUtils.fingerprint(srvRawPub)
        androidIdentity = DeviceIdentityManager(
            deviceId = srvDevId,
            displayName = "Android Test Host",
            platform = "android",
            rawPublicKey = srvRawPub,
            privateKey = srvKp.private
        )

        // Generate Linux client identity
        clientKeyPair = CryptoUtils.generateKeyPair()
        clientRawPub = CryptoUtils.rawPublicKey(clientKeyPair.public)
        clientDeviceId = CryptoUtils.fingerprint(clientRawPub)

        server = LanSignalingServer(
            port = testPort,
            handler = testHandler,
            identityManager = androidIdentity,
            trustStore = trustStore
        )
        assertTrue("server must start", server.start())
    }

    @After
    fun tearDown() {
        server.stop()
    }

    private fun signRequest(
        method: String,
        path: String,
        body: ByteArray,
        timestampMs: Long = System.currentTimeMillis(),
        nonce: String = "nonce-${System.nanoTime()}",
        keyPair: java.security.KeyPair = clientKeyPair,
        devId: String = clientDeviceId
    ): Map<String, String> {
        val bodyHash = CryptoUtils.toHex(CryptoUtils.sha256(body))
        val material = "$method\n$path\n$timestampMs\n$nonce\n$bodyHash"
        val sig = CryptoUtils.sign(keyPair.private, material.toByteArray(StandardCharsets.UTF_8))
        return mapOf(
            AuthValidator.HEADER_DEVICE_ID to devId,
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
    fun testCompletePairingHandshake() {
        // 1. Send pairing request
        val token = "token-secret-123"
        val reqPayload = JSONObject().apply {
            put("display_name", "Linux Host")
            put("platform", "linux")
            put("public_key", CryptoUtils.toHex(clientRawPub))
            put("pairing_token", token)
        }
        val (reqCode, reqResp) = executeHttp("POST", "/pairing/request", reqPayload.toString().toByteArray())
        assertEquals(200, reqCode)
        val acceptJson = JSONObject(reqResp)
        val sas = acceptJson.getString("sas")
        assertEquals(6, sas.length)

        // Verify SAS symmetry
        val expectedSAS = CryptoUtils.calculateSAS(clientRawPub, androidIdentity.rawPublicKey, token)
        assertEquals(expectedSAS, sas)

        // 2. Client confirms pairing with signature over "$token:$sas".
        // Phase 2: before the receiving user decides, the confirm must hold
        // as 202 pending and commit nothing.
        val sigMaterial = "$token:$sas".toByteArray(StandardCharsets.UTF_8)
        val sig = CryptoUtils.sign(clientKeyPair.private, sigMaterial)

        val confirmPayload = JSONObject().apply {
            put("device_id", clientDeviceId)
            put("pairing_token", token)
            put("sas", sas)
            put("confirmed", true)
            put("signature", CryptoUtils.toHex(sig))
        }
        val (pendingCode, pendingBody) = executeHttp("POST", "/pairing/confirm", confirmPayload.toString().toByteArray())
        assertEquals(202, pendingCode)
        assertEquals("pending", JSONObject(pendingBody).getString("status"))
        assertFalse("no trust before receiver approval", trustStore.isTrusted(clientDeviceId))

        // 3. The receiving user accepts; the same confirm now pairs.
        assertTrue(server.respondToPairing(token, true))
        val (confCode, _) = executeHttp("POST", "/pairing/confirm", confirmPayload.toString().toByteArray())
        assertEquals(200, confCode)

        // 4. Verify device is now trusted in trust store
        assertTrue("client must be trusted in trust store", trustStore.isTrusted(clientDeviceId))
    }

    @Test
    fun testExpiredPairingTokenRejected() {
        val token = "token-expired-1"
        val reqPayload = JSONObject().apply {
            put("display_name", "Linux Host")
            put("platform", "linux")
            put("public_key", CryptoUtils.toHex(clientRawPub))
            put("pairing_token", token)
        }
        val (reqCode, reqResp) = executeHttp("POST", "/pairing/request", reqPayload.toString().toByteArray())
        assertEquals(200, reqCode)
        val sas = JSONObject(reqResp).getString("sas")

        // Age the pending token past the TTL.
        val pending = server.pendingPairings[token]
        assertNotNull(pending)
        server.pendingPairings[token] = pending!!.copy(
            createdAt = System.currentTimeMillis() - 6 * 60 * 1000L
        )

        // Confirm with an otherwise-valid signature.
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
        assertEquals(400, confCode)
        assertTrue("expired token must not commit trust", !trustStore.isTrusted(clientDeviceId))
    }

    @Test
    fun testPairingWrongSASRejected() {        val token = "token-wrong-sas"
        val reqPayload = JSONObject().apply {
            put("display_name", "Linux Host")
            put("platform", "linux")
            put("public_key", CryptoUtils.toHex(clientRawPub))
            put("pairing_token", token)
        }
        val (reqCode, _) = executeHttp("POST", "/pairing/request", reqPayload.toString().toByteArray())
        assertEquals(200, reqCode)

        // Confirm with wrong SAS
        val wrongSAS = "999999"
        val sigMaterial = "$token:$wrongSAS".toByteArray(StandardCharsets.UTF_8)
        val sig = CryptoUtils.sign(clientKeyPair.private, sigMaterial)

        val confirmPayload = JSONObject().apply {
            put("device_id", clientDeviceId)
            put("pairing_token", token)
            put("sas", wrongSAS)
            put("confirmed", true)
            put("signature", CryptoUtils.toHex(sig))
        }
        val (confCode, _) = executeHttp("POST", "/pairing/confirm", confirmPayload.toString().toByteArray())
        assertEquals(400, confCode)
        assertTrue("client must not be trusted", !trustStore.isTrusted(clientDeviceId))
    }

    @Test
    fun testUnknownDeviceRejectedFromSignaling() {
        // Unknown device attempts to request offer
        val headers = signRequest("POST", "/session/offer", ByteArray(0))
        val (code, _) = executeHttp("POST", "/session/offer", ByteArray(0), headers)
        assertEquals(403, code)
    }

    @Test
    fun testAuthenticatedSessionOfferSuccess() {
        // Add client as trusted peer first
        trustStore.addTrusted(
            TrustedDeviceRecord(
                deviceId = clientDeviceId,
                displayName = "Linux Host",
                platform = "linux",
                rawPublicKey = clientRawPub,
                pairedAtMs = System.currentTimeMillis(),
                lastSeenMs = System.currentTimeMillis(),
                revoked = false
            )
        )

        val headers = signRequest("POST", "/session/offer", ByteArray(0))
        val (code, resp) = executeHttp("POST", "/session/offer", ByteArray(0), headers)
        assertEquals(200, code)
        assertTrue(resp.contains("mock-sdp-offer"))
    }

    @Test
    fun testReplayAttackRejected() {
        trustStore.addTrusted(
            TrustedDeviceRecord(
                deviceId = clientDeviceId,
                displayName = "Linux Host",
                platform = "linux",
                rawPublicKey = clientRawPub,
                pairedAtMs = System.currentTimeMillis(),
                lastSeenMs = System.currentTimeMillis(),
                revoked = false
            )
        )

        val headers = signRequest("POST", "/session/offer", ByteArray(0), nonce = "fixed-replay-nonce")
        val (code1, _) = executeHttp("POST", "/session/offer", ByteArray(0), headers)
        assertEquals(200, code1)

        // Second request with identical headers & nonce must be rejected
        val (code2, _) = executeHttp("POST", "/session/offer", ByteArray(0), headers)
        assertEquals(401, code2)
    }

    @Test
    fun testRevokedDeviceRejected() {
        trustStore.addTrusted(
            TrustedDeviceRecord(
                deviceId = clientDeviceId,
                displayName = "Linux Host",
                platform = "linux",
                rawPublicKey = clientRawPub,
                pairedAtMs = System.currentTimeMillis(),
                lastSeenMs = System.currentTimeMillis(),
                revoked = false
            )
        )

        // Revoke device
        trustStore.revoke(clientDeviceId)

        val headers = signRequest("POST", "/session/offer", ByteArray(0))
        val (code, _) = executeHttp("POST", "/session/offer", ByteArray(0), headers)
        assertEquals(403, code)
    }

    @Test
    fun testTrustCommittedByAnotherInstanceIsHonored() {
        // Physical BUG-1: the server's verifier holds a long-lived TrustStore
        // instance while pairing approvals land through the UI instance
        // sharing the same file. The verifier must re-read before
        // authenticating, or a freshly paired peer is rejected with 403 while
        // re-pairing is short-circuited with 409 (no recovery path).
        // Here the server starts with an empty (stale) view; the trust commit
        // arrives via a second instance, exactly like a UI approval.
        TrustStore(File(tempFolder.root, "trusted_devices.json")).addTrusted(
            TrustedDeviceRecord(
                deviceId = clientDeviceId,
                displayName = "Linux Host",
                platform = "linux",
                rawPublicKey = clientRawPub,
                pairedAtMs = System.currentTimeMillis(),
                lastSeenMs = System.currentTimeMillis(),
                revoked = false
            )
        )

        val headers = signRequest("POST", "/session/offer", ByteArray(0))
        val (code, resp) = executeHttp("POST", "/session/offer", ByteArray(0), headers)
        assertEquals("trust committed by another instance must authenticate", 200, code)
        assertTrue(resp.contains("mock-sdp-offer"))
    }

    @Test
    fun testRevocationByAnotherInstanceIsEnforced() {
        // Converse direction: a revocation committed through the UI instance
        // must take effect on the next authenticated request, not on the next
        // process restart — otherwise a revoked peer keeps session access.
        trustStore.addTrusted(
            TrustedDeviceRecord(
                deviceId = clientDeviceId,
                displayName = "Linux Host",
                platform = "linux",
                rawPublicKey = clientRawPub,
                pairedAtMs = System.currentTimeMillis(),
                lastSeenMs = System.currentTimeMillis(),
                revoked = false
            )
        )
        TrustStore(File(tempFolder.root, "trusted_devices.json")).revoke(clientDeviceId)

        val headers = signRequest("POST", "/session/offer", ByteArray(0))
        val (code, resp) = executeHttp("POST", "/session/offer", ByteArray(0), headers)
        assertEquals("revocation by another instance must be enforced", 403, code)
        assertTrue(resp.contains("has been revoked"))
    }

    @Test
    fun testTamperedSignatureRejected() {
        trustStore.addTrusted(
            TrustedDeviceRecord(
                deviceId = clientDeviceId,
                displayName = "Linux Host",
                platform = "linux",
                rawPublicKey = clientRawPub,
                pairedAtMs = System.currentTimeMillis(),
                lastSeenMs = System.currentTimeMillis(),
                revoked = false
            )
        )

        val headers = signRequest("POST", "/session/offer", ByteArray(0)).toMutableMap()
        headers[AuthValidator.HEADER_SIGNATURE] = CryptoUtils.toHex(ByteArray(64) { 0x01 })

        val (code, _) = executeHttp("POST", "/session/offer", ByteArray(0), headers)
        assertEquals(401, code)
    }
}
