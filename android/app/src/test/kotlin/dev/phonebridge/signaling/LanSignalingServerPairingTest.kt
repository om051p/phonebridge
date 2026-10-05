package dev.phonebridge.signaling

import dev.phonebridge.security.CryptoUtils
import dev.phonebridge.security.DeviceIdentityManager
import dev.phonebridge.security.TrustStore
import org.json.JSONObject
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import java.io.File
import java.net.HttpURLConnection
import java.net.URL
import java.nio.charset.StandardCharsets

/**
 * The Phase 2 bidirectional-pairing contract of the receiver half: no trust
 * without the receiving user's explicit approval, duplicate suppression,
 * already-trusted short-circuit, single-use tokens, and expiry sweeping.
 */
class LanSignalingServerPairingTest {

    private lateinit var server: LanSignalingServer
    private lateinit var trustStore: TrustStore
    private lateinit var trustStoreDir: File
    private val testPort = 17805

    private val receiverIdentity: DeviceIdentityManager = run {
        val kp = CryptoUtils.generateKeyPair()
        DeviceIdentityManager(
            deviceId = CryptoUtils.fingerprint(CryptoUtils.rawPublicKey(kp.public)),
            displayName = "Receiver Phone",
            platform = "android",
            rawPublicKey = CryptoUtils.rawPublicKey(kp.public),
            privateKey = kp.private,
        )
    }

    private val requesterIdentity: DeviceIdentityManager = run {
        val kp = CryptoUtils.generateKeyPair()
        DeviceIdentityManager(
            deviceId = CryptoUtils.fingerprint(CryptoUtils.rawPublicKey(kp.public)),
            displayName = "Requester Device",
            platform = "linux",
            rawPublicKey = CryptoUtils.rawPublicKey(kp.public),
            privateKey = kp.private,
        )
    }

    private var notifiedRequests = mutableListOf<String>()

    @Before
    fun setUp() {
        trustStoreDir = File(System.getProperty("java.io.tmpdir"), "pb-pairing-test-${System.nanoTime()}")
        trustStoreDir.mkdirs()
        trustStore = TrustStore(File(trustStoreDir, "trusted_devices.json"))
        notifiedRequests = mutableListOf()
        server = LanSignalingServer(
            port = testPort,
            identityManager = receiverIdentity,
            trustStore = trustStore,
        )
        server.onPairingRequest = { info -> notifiedRequests.add(info.token) }
        assertTrue("server should start", server.start())
    }

    @After
    fun tearDown() {
        server.stop()
    }

    /** Posts /pairing/request as the requester identity. */
    private fun postRequest(token: String): Pair<Int, JSONObject?> {
        val conn = pairingRequestConnection(token)
        val code = conn.responseCode
        val body = (if (code in 200..399) conn.inputStream else conn.errorStream)
            .bufferedReader().readText()
        return Pair(code, if (body.isBlank()) null else JSONObject(body))
    }

    private fun pairingRequestConnection(token: String): HttpURLConnection {
        val conn = URL("http://127.0.0.1:$testPort/pairing/request").openConnection() as HttpURLConnection
        conn.requestMethod = "POST"
        conn.doOutput = true
        val payload = JSONObject().apply {
            put("display_name", requesterIdentity.displayName)
            put("platform", requesterIdentity.platform)
            put("public_key", CryptoUtils.toHex(requesterIdentity.rawPublicKey))
            put("pairing_token", token)
        }
        conn.outputStream.use { it.write(payload.toString().toByteArray(StandardCharsets.UTF_8)) }
        return conn
    }

    /** Posts /pairing/confirm signed by the requester identity. */
    private fun postConfirm(token: String, sas: String, confirmed: Boolean): Pair<Int, JSONObject?> {
        val conn = confirmConnection(token, sas, confirmed)
        val code = conn.responseCode
        val body = (if (code in 200..399) conn.inputStream else conn.errorStream)
            .bufferedReader().readText()
        return Pair(code, if (body.isBlank()) null else JSONObject(body))
    }

    private fun confirmConnection(token: String, sas: String, confirmed: Boolean): HttpURLConnection {
        val conn = URL("http://127.0.0.1:$testPort/pairing/confirm").openConnection() as HttpURLConnection
        conn.requestMethod = "POST"
        conn.doOutput = true
        val sig = requesterIdentity.sign("$token:$sas".toByteArray(StandardCharsets.UTF_8))
        val payload = JSONObject().apply {
            put("device_id", requesterIdentity.deviceId)
            put("pairing_token", token)
            put("sas", sas)
            put("confirmed", confirmed)
            put("signature", CryptoUtils.toHex(sig))
        }
        conn.outputStream.use { it.write(payload.toString().toByteArray(StandardCharsets.UTF_8)) }
        return conn
    }

    @Test
    fun confirmBeforeApprovalHoldsPendingAndCommitsNothing() {
        val (reqCode, accept) = postRequest("tok-hold")
        assertEquals(200, reqCode)
        val sas = accept!!.getString("sas")

        val (confirmCode, body) = postConfirm("tok-hold", sas, confirmed = true)
        assertEquals(202, confirmCode)
        assertEquals("pending", body!!.getString("status"))
        assertFalse("trust must NOT commit without receiver approval", trustStore.isTrusted(requesterIdentity.deviceId))
        assertEquals("pending request must remain for the dialog", 1, server.listPendingPairings().size)
    }

    @Test
    fun approveThenConfirmPairsBothContract() {
        val (_, accept) = postRequest("tok-pair")
        val sas = accept!!.getString("sas")
        assertEquals(202, postConfirm("tok-pair", sas, true).first)

        assertTrue(server.respondToPairing("tok-pair", true))
        val (code, body) = postConfirm("tok-pair", sas, true)
        assertEquals(200, code)
        assertEquals("paired", body!!.getString("status"))
        assertTrue(trustStore.isTrusted(requesterIdentity.deviceId))
        val rec = trustStore.get(requesterIdentity.deviceId)
        assertNotNull(rec)
        assertEquals("Requester Device", rec!!.displayName)
        assertEquals("linux", rec.platform)
        assertEquals("single-use token: paired request is consumed", 0, server.listPendingPairings().size)
    }

    @Test
    fun rejectDecisionSurfacesAndCommitsNothing() {
        val (_, accept) = postRequest("tok-reject")
        val sas = accept!!.getString("sas")
        assertTrue(server.respondToPairing("tok-reject", false))

        val (code, _) = postConfirm("tok-reject", sas, true)
        assertEquals(400, code)
        assertFalse(trustStore.isTrusted(requesterIdentity.deviceId))
        assertEquals("rejected request must not linger as pending", 0, server.listPendingPairings().size)
    }

    @Test
    fun duplicateRequestFromSamePeerSupersedes() {
        assertEquals(200, postRequest("tok-old").first)
        assertEquals(200, postRequest("tok-new").first)

        val pending = server.listPendingPairings()
        assertEquals("exactly one dialog per peer", 1, pending.size)
        assertEquals("tok-new", pending[0].token)
        assertFalse("stale token must be dead after supersede", server.respondToPairing("tok-old", true))
    }

    @Test
    fun alreadyTrustedKeyGetsConflictAndNoPendingEntry() {
        trustStore.addTrusted(
            dev.phonebridge.security.TrustedDeviceRecord(
                deviceId = requesterIdentity.deviceId,
                displayName = requesterIdentity.displayName,
                platform = requesterIdentity.platform,
                rawPublicKey = requesterIdentity.rawPublicKey,
                pairedAtMs = 0L,
                lastSeenMs = 0L,
                revoked = false,
            ),
        )
        val (code, _) = postRequest("tok-redundant")
        assertEquals(409, code)
        assertEquals(0, server.listPendingPairings().size)
    }

    @Test
    fun revocationByAnotherStoreInstanceIsSeenByTheServer() {
        // The UI mutates trust in ITS OWN TrustStore instance (same file).
        // The server must re-read the file before the already-trusted check,
        // or a revoked peer is wrongly told "already trusted".
        trustStore.addTrusted(
            dev.phonebridge.security.TrustedDeviceRecord(
                deviceId = requesterIdentity.deviceId,
                displayName = requesterIdentity.displayName,
                platform = requesterIdentity.platform,
                rawPublicKey = requesterIdentity.rawPublicKey,
                pairedAtMs = 0L,
                lastSeenMs = 0L,
                revoked = false,
            ),
        )
        // A second instance over the same file revokes (the UI path).
        TrustStore(java.io.File(trustStoreDir, "trusted_devices.json")).revoke(requesterIdentity.deviceId)

        val (code, _) = postRequest("tok-post-revoke")
        assertEquals("revoked key must re-pair through approval, not 409", 200, code)
        assertEquals(1, server.listPendingPairings().size)
    }

    @Test
    fun revokedKeyRePairsThroughApproval() {
        trustStore.addTrusted(
            dev.phonebridge.security.TrustedDeviceRecord(
                deviceId = requesterIdentity.deviceId,
                displayName = requesterIdentity.displayName,
                platform = requesterIdentity.platform,
                rawPublicKey = requesterIdentity.rawPublicKey,
                pairedAtMs = 0L,
                lastSeenMs = 0L,
                revoked = false,
            ),
        )
        trustStore.revoke(requesterIdentity.deviceId)
        val (code, _) = postRequest("tok-repair")
        assertEquals("revoked keys are NOT already-trusted: approval required", 200, code)

        val sas = server.listPendingPairings()[0].sas
        assertTrue(server.respondToPairing("tok-repair", true))
        assertEquals(200, postConfirm("tok-repair", sas, true).first)
        assertTrue(trustStore.isTrusted(requesterIdentity.deviceId))
    }

    @Test
    fun confirmWithMismatchedDeviceIdIsRejectedAndCommitsNothing() {
        val (_, accept) = postRequest("tok-mismatch")
        val sas = accept!!.getString("sas")
        assertTrue(server.respondToPairing("tok-mismatch", true))

        // Same valid signature, but a claimed device_id that is not the
        // fingerprint of the authenticated key.
        val conn = URL("http://127.0.0.1:$testPort/pairing/confirm").openConnection() as HttpURLConnection
        conn.requestMethod = "POST"
        conn.doOutput = true
        val sig = requesterIdentity.sign("tok-mismatch:$sas".toByteArray(StandardCharsets.UTF_8))
        val payload = JSONObject().apply {
            put("device_id", "totally-different-id")
            put("pairing_token", "tok-mismatch")
            put("sas", sas)
            put("confirmed", true)
            put("signature", CryptoUtils.toHex(sig))
        }
        conn.outputStream.use { it.write(payload.toString().toByteArray(StandardCharsets.UTF_8)) }
        assertEquals(400, conn.responseCode)
        assertEquals(0, trustStore.list().size)
    }

    @Test
    fun requesterRejectionWithdrawsPendingRequest() {
        val (_, accept) = postRequest("tok-withdraw")
        val sas = accept!!.getString("sas")
        val (code, _) = postConfirm("tok-withdraw", sas, confirmed = false)
        assertEquals(400, code)
        assertEquals("receiver's pending dialog is withdrawn", 0, server.listPendingPairings().size)
    }

    @Test
    fun confirmReplayAfterPairingIsRejected() {
        val (_, accept) = postRequest("tok-replay")
        val sas = accept!!.getString("sas")
        assertTrue(server.respondToPairing("tok-replay", true))
        assertEquals(200, postConfirm("tok-replay", sas, true).first)
        assertEquals("replayed confirm must not pair twice", 400, postConfirm("tok-replay", sas, true).first)
    }

    @Test
    fun expiredRequestIsSweptFromReadsAndConfirm() {
        val (_, accept) = postRequest("tok-expired")
        val sas = accept!!.getString("sas")

        // Age the entry past the TTL directly (internal view) and verify every
        // read path sweeps it instead of surfacing a dead request.
        val aged = server.pendingPairings["tok-expired"]!!.copy(
            createdAt = System.currentTimeMillis() - 6 * 60 * 1000L
        )
        server.pendingPairings["tok-expired"] = aged

        assertEquals(0, server.listPendingPairings().size)
        assertEquals(400, postConfirm("tok-expired", sas, true).first)
        assertFalse(server.respondToPairing("tok-expired", true))
        assertFalse(trustStore.isTrusted(requesterIdentity.deviceId))
    }

    @Test
    fun respondToUnknownTokenIsFalse() {
        assertFalse(server.respondToPairing("no-such-token", true))
    }

    @Test
    fun newRequestFiresListenerExactlyOncePerRequest() {
        postRequest("tok-listen-1")
        assertEquals(1, notifiedRequests.size)
        postRequest("tok-listen-2")
        assertEquals("retry from the same peer fires once (supersede)", 2, notifiedRequests.size)
        assertEquals("tok-listen-2", notifiedRequests.last())
    }

    @Test
    fun sasMatchesCryptoUtilsComputation() {
        val (_, accept) = postRequest("tok-sas")
        val expected = CryptoUtils.calculateSAS(
            receiverIdentity.rawPublicKey,
            requesterIdentity.rawPublicKey,
            "tok-sas",
        )
        assertEquals(expected, accept!!.getString("sas"))
    }
}
