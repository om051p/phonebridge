package dev.phonebridge.security

import java.nio.charset.StandardCharsets

/**
 * Validates request signatures and replay protection on incoming HTTP signaling requests.
 */
class AuthValidator(
    private val trustStore: TrustStore,
    private val replayWindowMs: Long = 30_000L
) {
    companion object {
        const val HEADER_DEVICE_ID = "X-PhoneBridge-Device-ID"
        const val HEADER_TIMESTAMP = "X-PhoneBridge-Timestamp"
        const val HEADER_NONCE = "X-PhoneBridge-Nonce"
        const val HEADER_SIGNATURE = "X-PhoneBridge-Signature"
    }

    private val seenNonces = mutableMapOf<String, Long>()
    private val lock = Any()

    sealed class AuthResult {
        data class Success(val deviceId: String) : AuthResult()
        data class Failure(val statusCode: Int, val message: String) : AuthResult()
    }

    fun verify(
        method: String,
        path: String,
        body: ByteArray,
        getHeader: (String) -> String?
    ): AuthResult {
        val deviceId = getHeader(HEADER_DEVICE_ID)?.trim()
        val tsStr = getHeader(HEADER_TIMESTAMP)?.trim()
        val nonce = getHeader(HEADER_NONCE)?.trim()
        val sigHex = getHeader(HEADER_SIGNATURE)?.trim()

        if (deviceId.isNullOrEmpty() || tsStr.isNullOrEmpty() || nonce.isNullOrEmpty() || sigHex.isNullOrEmpty()) {
            return AuthResult.Failure(401, "Missing required authentication headers")
        }

        val timestampMs = tsStr.toLongOrNull()
            ?: return AuthResult.Failure(400, "Invalid timestamp header")

        val now = System.currentTimeMillis()
        if (Math.abs(now - timestampMs) > replayWindowMs) {
            return AuthResult.Failure(401, "Timestamp outside allowed tolerance window")
        }

        synchronized(lock) {
            // Evict expired nonces
            val cutoff = now - replayWindowMs
            seenNonces.entries.removeIf { it.value < cutoff }

            if (seenNonces.containsKey(nonce)) {
                return AuthResult.Failure(401, "Replayed request nonce detected")
            }
            seenNonces[nonce] = now
        }

        val record = trustStore.get(deviceId)
            ?: return AuthResult.Failure(403, "Device $deviceId is not trusted: pairing required")

        if (record.revoked) {
            return AuthResult.Failure(403, "Device $deviceId has been revoked")
        }

        val bodyHash = CryptoUtils.toHex(CryptoUtils.sha256(body))
        val material = "$method\n$path\n$timestampMs\n$nonce\n$bodyHash"
        val materialBytes = material.toByteArray(StandardCharsets.UTF_8)

        val sigBytes = try {
            CryptoUtils.fromHex(sigHex)
        } catch (_: Throwable) {
            return AuthResult.Failure(400, "Invalid signature encoding")
        }

        val peerPublicKey = try {
            CryptoUtils.parsePublicKey(record.rawPublicKey)
        } catch (_: Throwable) {
            return AuthResult.Failure(500, "Corrupt stored public key for device")
        }

        if (!CryptoUtils.verify(peerPublicKey, materialBytes, sigBytes)) {
            return AuthResult.Failure(401, "Invalid request signature")
        }

        return AuthResult.Success(deviceId)
    }
}
