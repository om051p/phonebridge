package dev.phonebridge.security

import java.nio.ByteBuffer
import java.nio.ByteOrder
import java.nio.charset.StandardCharsets
import java.security.KeyFactory
import java.security.KeyPair
import java.security.KeyPairGenerator
import java.security.MessageDigest
import java.security.PrivateKey
import java.security.PublicKey
import java.security.Signature
import java.security.spec.PKCS8EncodedKeySpec
import java.security.spec.X509EncodedKeySpec

/**
 * Low-level cryptographic primitives for PhoneBridge identity, SAS calculation,
 * and authenticated HTTP signaling.
 */
object CryptoUtils {

    private val bcProvider: java.security.Provider = org.bouncycastle.jce.provider.BouncyCastleProvider()

    init {
        try {
            java.security.Security.removeProvider("BC")
            java.security.Security.insertProviderAt(bcProvider, 1)
        } catch (_: Throwable) {
            // Ignore if provider registration is restricted by security manager
        }
    }

    private fun getKeyPairGenerator(): KeyPairGenerator {
        return try {
            KeyPairGenerator.getInstance("Ed25519")
        } catch (_: Throwable) {
            KeyPairGenerator.getInstance("Ed25519", bcProvider)
        }
    }

    private fun getKeyFactory(): KeyFactory {
        return try {
            KeyFactory.getInstance("Ed25519")
        } catch (_: Throwable) {
            KeyFactory.getInstance("Ed25519", bcProvider)
        }
    }

    private fun getSignature(): Signature {
        return try {
            Signature.getInstance("Ed25519")
        } catch (_: Throwable) {
            Signature.getInstance("Ed25519", bcProvider)
        }
    }

    // Standard ASN.1 prefix for Ed25519 X.509 SubjectPublicKeyInfo (12 bytes)
    val ED25519_X509_HEADER = byteArrayOf(
        0x30, 0x2a, 0x30, 0x05, 0x06, 0x03, 0x2b, 0x65, 0x70, 0x03, 0x21, 0x00
    )

    // Standard ASN.1 prefix for Ed25519 PKCS#8 PrivateKeyInfo (16 bytes)
    val ED25519_PKCS8_HEADER = byteArrayOf(
        0x30, 0x2e, 0x02, 0x01, 0x00, 0x30, 0x05, 0x06, 0x03, 0x2b, 0x65, 0x70, 0x04, 0x22, 0x04, 0x20
    )

    fun generateKeyPair(): KeyPair {
        val kpg = getKeyPairGenerator()
        return kpg.generateKeyPair()
    }

    fun rawPublicKey(publicKey: PublicKey): ByteArray {
        val encoded = publicKey.encoded
        if (encoded.size == 44 && encoded.startsWith(ED25519_X509_HEADER)) {
            return encoded.copyOfRange(12, 44)
        }
        return encoded
    }

    fun rawPrivateKeySeed(privateKey: PrivateKey): ByteArray {
        val encoded = privateKey.encoded
        if (encoded.size == 48 && encoded.startsWith(ED25519_PKCS8_HEADER)) {
            return encoded.copyOfRange(16, 48)
        }
        return encoded
    }

    fun parsePublicKey(rawPub: ByteArray): PublicKey {
        val kf = getKeyFactory()
        val spec = if (rawPub.size == 32) {
            X509EncodedKeySpec(ED25519_X509_HEADER + rawPub)
        } else {
            X509EncodedKeySpec(rawPub)
        }
        return kf.generatePublic(spec)
    }

    fun parsePrivateKey(rawSeed: ByteArray): PrivateKey {
        val kf = getKeyFactory()
        val spec = if (rawSeed.size == 32) {
            PKCS8EncodedKeySpec(ED25519_PKCS8_HEADER + rawSeed)
        } else {
            PKCS8EncodedKeySpec(rawSeed)
        }
        return kf.generatePrivate(spec)
    }

    fun fingerprint(rawPub: ByteArray): String {
        val md = MessageDigest.getInstance("SHA-256")
        val digest = md.digest(rawPub)
        return toHex(digest)
    }

    fun sign(privateKey: PrivateKey, message: ByteArray): ByteArray {
        val signer = getSignature()
        signer.initSign(privateKey)
        signer.update(message)
        return signer.sign()
    }

    fun verify(publicKey: PublicKey, message: ByteArray, signature: ByteArray): Boolean {
        return try {
            val verifier = getSignature()
            verifier.initVerify(publicKey)
            verifier.update(message)
            verifier.verify(signature)
        } catch (_: Throwable) {
            false
        }
    }

    fun compareUnsignedBytes(a: ByteArray, b: ByteArray): Int {
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

    /**
     * Derives a symmetric 6-digit SAS code identical on initiator and responder.
     * digest = SHA256(min(pubA, pubB) || max(pubA, pubB) || token)
     */
    fun calculateSAS(localPub: ByteArray, remotePub: ByteArray, token: String): String {
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

    fun toHex(bytes: ByteArray): String {
        val sb = StringBuilder(bytes.size * 2)
        for (b in bytes) {
            sb.append(String.format("%02x", b.toInt() and 0xFF))
        }
        return sb.toString()
    }

    fun fromHex(hex: String): ByteArray {
        val clean = hex.trim()
        val len = clean.length
        val data = ByteArray(len / 2)
        for (i in 0 until len step 2) {
            data[i / 2] = ((Character.digit(clean[i], 16) shl 4) +
                Character.digit(clean[i + 1], 16)).toByte()
        }
        return data
    }

    fun sha256(bytes: ByteArray): ByteArray {
        val md = MessageDigest.getInstance("SHA-256")
        return md.digest(bytes)
    }

    private fun ByteArray.startsWith(prefix: ByteArray): Boolean {
        if (this.size < prefix.size) return false
        for (i in prefix.indices) {
            if (this[i] != prefix[i]) return false
        }
        return true
    }
}
