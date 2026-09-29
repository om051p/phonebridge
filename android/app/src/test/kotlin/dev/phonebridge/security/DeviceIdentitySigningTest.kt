package dev.phonebridge.security

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.nio.charset.StandardCharsets

/**
 * Pins the outbound request signing the phone uses to reach the Linux daemon.
 *
 * The daemon verifies every signaling request by recomputing a canonical string
 * from the raw request and checking the Ed25519 signature against it. A phone
 * that builds that string differently is rejected as unauthenticated - a silent,
 * hard-to-diagnose failure that looks like a network problem. Because Ed25519
 * signing is deterministic, asserting the exact signature hex for fixed inputs
 * pins the canonical material byte for byte.
 *
 * The expected values come from the Go side,
 * core/pkg/crypto/crypto_test.go::TestVerifyRequest_AcceptsAndroidPeerOfferSignature,
 * which asserts the same hex verifies through the daemon's own VerifyRequest.
 * Together the two tests prove the phone and the daemon agree on the wire.
 */
class DeviceIdentitySigningTest {

    private companion object {
        // Deterministic Ed25519 seed 0x01..0x20.
        val SEED = ByteArray(32) { (it + 1).toByte() }
        const val DEVICE_ID = "65b60673d6ed884bf01c2c222d82ada0740f29ac3355d6a925c81f17f47a27b8"
        const val PUBLIC_KEY_HEX = "79b5562e8fe654f94078b112e8a98ba7901f853ae695bed7e0e3910bad049664"
        const val BODY_HASH_HEX = "93a23971a914e5eacbf0a8d25154cda309c3c1c72fbb9914d47c60f3cb681588"
        const val TIMESTAMP_MS = 1790000000000L
        const val NONCE = "0f1e2d3c4b5a69788796a5b4c3d2e1f0"
        const val SIGNATURE_HEX =
            "3bb75ade65382eb8acb806e721df9fff2560d768d757c762e0b5f8fababb5cdf" +
                "e8915d1e47e6380eb8bfbed1ac9b79e09ce072c4b1d03c24f87db3ee766e430d"
        const val METHOD = "POST"
        const val PATH = "/session/peer-offer"

        val BODY = "{\"hello\":\"world\"}".toByteArray(StandardCharsets.UTF_8)
    }

    private fun identity(): DeviceIdentityManager {
        val rawPublicKey = CryptoUtils.fromHex(PUBLIC_KEY_HEX)
        return DeviceIdentityManager(
            deviceId = CryptoUtils.fingerprint(rawPublicKey),
            displayName = "Android Phone",
            platform = "android",
            rawPublicKey = rawPublicKey,
            privateKey = CryptoUtils.parsePrivateKey(SEED)
        )
    }

    @Test
    fun identityDerivedFromTheVectorSeedMatchesTheGoSide() {
        val identity = identity()
        assertEquals(DEVICE_ID, identity.deviceId)
        assertEquals(DEVICE_ID, CryptoUtils.toHex(CryptoUtils.sha256(CryptoUtils.fromHex(PUBLIC_KEY_HEX))))
    }

    @Test
    fun signedRequestReproducesTheGoVector() {
        val headers = identity().signRequest(METHOD, PATH, BODY, TIMESTAMP_MS, NONCE)
        assertEquals(SIGNATURE_HEX, headers[AuthValidator.HEADER_SIGNATURE])
    }

    @Test
    fun signatureVerifiesAgainstTheDocumentedMaterial() {
        val identity = identity()
        val headers = identity.signRequest(METHOD, PATH, BODY, TIMESTAMP_MS, NONCE)
        val material = buildString {
            append(METHOD).append('\n')
            append(PATH).append('\n')
            append(TIMESTAMP_MS).append('\n')
            append(NONCE).append('\n')
            append(BODY_HASH_HEX)
        }
        assertTrue(
            "signature must verify over \"method\\npath\\ntimestamp\\nnonce\\nbodyHash\"",
            CryptoUtils.verify(
                identity.publicKey,
                material.toByteArray(StandardCharsets.UTF_8),
                CryptoUtils.fromHex(headers.getValue(AuthValidator.HEADER_SIGNATURE))
            )
        )
    }

    @Test
    fun headersCarryEveryFieldTheDaemonReads() {
        val headers = identity().signRequest(METHOD, PATH, BODY, TIMESTAMP_MS, NONCE)
        assertEquals(
            setOf(
                AuthValidator.HEADER_DEVICE_ID,
                AuthValidator.HEADER_TIMESTAMP,
                AuthValidator.HEADER_NONCE,
                AuthValidator.HEADER_SIGNATURE
            ),
            headers.keys
        )
        assertEquals(DEVICE_ID, headers[AuthValidator.HEADER_DEVICE_ID])
        assertEquals(TIMESTAMP_MS.toString(), headers[AuthValidator.HEADER_TIMESTAMP])
        assertEquals(NONCE, headers[AuthValidator.HEADER_NONCE])
        assertEquals(128, headers.getValue(AuthValidator.HEADER_SIGNATURE).length)
    }

    /**
     * The body hash and the path are both part of the signature. If either were
     * dropped, a valid signed request could be replayed against a different
     * endpoint or with a swapped payload.
     */
    @Test
    fun signatureBindsBothThePathAndTheBody() {
        val identity = identity()
        val baseline = identity.signRequest(METHOD, PATH, BODY, TIMESTAMP_MS, NONCE)
        val otherPath = identity.signRequest(METHOD, "/session/stop", BODY, TIMESTAMP_MS, NONCE)
        val otherBody = identity.signRequest(
            METHOD,
            PATH,
            "{\"hello\":\"there\"}".toByteArray(StandardCharsets.UTF_8),
            TIMESTAMP_MS,
            NONCE
        )
        val otherNonce = identity.signRequest(METHOD, PATH, BODY, TIMESTAMP_MS, "ffffffffffffffffffffffffffffffff")

        assertNotEquals(baseline[AuthValidator.HEADER_SIGNATURE], otherPath[AuthValidator.HEADER_SIGNATURE])
        assertNotEquals(baseline[AuthValidator.HEADER_SIGNATURE], otherBody[AuthValidator.HEADER_SIGNATURE])
        assertNotEquals(baseline[AuthValidator.HEADER_SIGNATURE], otherNonce[AuthValidator.HEADER_SIGNATURE])
    }

    /** The daemon's nonce cache keys on hex, and issues 16 bytes like we do. */
    @Test
    fun generatedNoncesAreUniqueSixteenByteHex() {
        val identity = identity()
        val nonces = (1..64).map { identity.newNonceHex() }
        assertEquals(64, nonces.toSet().size)
        nonces.forEach { nonce ->
            assertEquals(32, nonce.length)
            assertTrue("nonce must be lowercase hex", nonce.matches(Regex("[0-9a-f]{32}")))
        }
    }

    @Test
    fun defaultTimestampIsCurrentTimeInMilliseconds() {
        val before = System.currentTimeMillis()
        val headers = identity().signRequest(METHOD, PATH, BODY)
        val stamped = headers.getValue(AuthValidator.HEADER_TIMESTAMP).toLong()
        val after = System.currentTimeMillis()
        assertTrue("timestamp must be epoch milliseconds", stamped in before..after)
    }
}
