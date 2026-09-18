package dev.phonebridge.security

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.nio.ByteBuffer
import java.nio.ByteOrder
import java.nio.charset.StandardCharsets
import java.security.KeyFactory
import java.security.KeyPairGenerator
import java.security.MessageDigest
import java.security.Signature
import java.security.spec.PKCS8EncodedKeySpec
import java.security.spec.X509EncodedKeySpec

class CryptoTest {
    @Test
    fun testEd25519Available() {
        val kpg = KeyPairGenerator.getInstance("Ed25519")
        val kp = kpg.generateKeyPair()
        assertNotNull(kp.public)
        assertNotNull(kp.private)

        val pubEncoded = kp.public.encoded
        assertEquals(44, pubEncoded.size)
        val rawPub = pubEncoded.copyOfRange(12, 44)
        assertEquals(32, rawPub.size)

        val privEncoded = kp.private.encoded
        assertEquals(48, privEncoded.size)
        val rawSeed = privEncoded.copyOfRange(16, 48)
        assertEquals(32, rawSeed.size)

        val x509Header = byteArrayOf(
            0x30, 0x2a, 0x30, 0x05, 0x06, 0x03, 0x2b, 0x65, 0x70, 0x03, 0x21, 0x00
        )
        val reconstructedPubBytes = x509Header + rawPub
        val kf = KeyFactory.getInstance("Ed25519")
        val reconstructedPub = kf.generatePublic(X509EncodedKeySpec(reconstructedPubBytes))

        val pkcs8Header = byteArrayOf(
            0x30, 0x2e, 0x02, 0x01, 0x00, 0x30, 0x05, 0x06, 0x03, 0x2b, 0x65, 0x70, 0x04, 0x22, 0x04, 0x20
        )
        val reconstructedPrivBytes = pkcs8Header + rawSeed
        val reconstructedPriv = kf.generatePrivate(PKCS8EncodedKeySpec(reconstructedPrivBytes))

        val signer = Signature.getInstance("Ed25519")
        signer.initSign(reconstructedPriv)
        val msg = "test cross compatibility message".toByteArray()
        signer.update(msg)
        val sig = signer.sign()
        assertEquals(64, sig.size)

        val verifier = Signature.getInstance("Ed25519")
        verifier.initVerify(reconstructedPub)
        verifier.update(msg)
        assertTrue(verifier.verify(sig))
    }

    @Test
    fun testCalculateSAS() {
        val keyA = ByteArray(32) { 1 }
        val keyB = ByteArray(32) { 2 }
        val token = "token123"

        val sas1 = calculateSAS(keyA, keyB, token)
        val sas2 = calculateSAS(keyB, keyA, token)

        assertEquals("SAS must be symmetric", sas1, sas2)
        assertEquals("SAS must be 6 digits", 6, sas1.length)
    }

    private fun compareUnsignedBytes(a: ByteArray, b: ByteArray): Int {
        val minLen = minOf(a.size, b.size)
        for (i in 0 until minLen) {
            val byteA = a[i].toInt() and 0xFF
            val byteB = b[i].toInt() and 0xFF
            if (byteA != byteB) {
                return byteA.compareTo(byteB)
            }
        }
        return a.size.compareTo(b.size)
    }

    private fun calculateSAS(localPub: ByteArray, remotePub: ByteArray, token: String): String {
        if (localPub.isEmpty() || remotePub.isEmpty()) {
            return "000000"
        }
        var k1 = localPub
        var k2 = remotePub
        if (compareUnsignedBytes(k1, k2) > 0) {
            k1 = remotePub
            k2 = localPub
        }
        val md = MessageDigest.getInstance("SHA-256")
        md.update(k1)
        md.update(k2)
        md.update(token.toByteArray(StandardCharsets.UTF_8))
        val digest = md.digest()

        val buf = ByteBuffer.wrap(digest, 0, 4).order(ByteOrder.BIG_ENDIAN)
        val intVal = buf.int.toLong() and 0xFFFFFFFFL
        val sasVal = intVal % 1_000_000L
        return String.format("%06d", sasVal)
    }
}
