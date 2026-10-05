package dev.phonebridge.signaling

import android.util.Log
import dev.phonebridge.security.CryptoUtils
import dev.phonebridge.security.DeviceIdentityManager
import dev.phonebridge.security.TrustStore
import dev.phonebridge.security.TrustedDeviceRecord
import org.json.JSONObject
import java.io.BufferedReader
import java.io.InputStreamReader
import java.net.HttpURLConnection
import java.net.URL
import java.nio.charset.StandardCharsets
import java.util.concurrent.ConcurrentHashMap

/**
 * PairingClient is the REQUESTER half of bidirectional pairing (Phase 2): this
 * device asks a discovered peer's signaling server to pair, shows the user the
 * SAS, and — after their explicit confirm — completes the handshake with the
 * same poll contract the Go requester uses.
 *
 * Wire contract (mirrors core/pkg/crypto/pairing.go):
 * 1. POST /pairing/request  -> 200 {display_name, platform, public_key, sas}
 *    or 409 {"error":"already trusted"} when the peer already trusts this key.
 * 2. User verifies the 6-digit SAS on both screens.
 * 3. POST /pairing/confirm (signed over token:sas) -> 200 paired,
 *    202 pending while the receiving user decides (poll every [POLL_MS] up to
 *    the 5-minute token TTL), 400 rejected/expired.
 *
 * Trust is committed locally only after the peer answers 200; nothing is
 * trusted on request or on SAS match alone. Logs carry names and counts only —
 * never tokens or SAS values.
 */
class PairingClient private constructor() {

    companion object {
        private const val TAG = "PairingClient"

        /** Pace between confirm polls while the receiver reports 202 pending. */
        internal const val POLL_MS = 3_000L

        /** Cap on the whole poll wait: the receiver's token TTL is 5 minutes. */
        internal const val MAX_POLL_MS = 5 * 60 * 1000L

        private val outbound = ConcurrentHashMap<String, OutboundPairing>()

        /**
         * Requests pairing with the peer at [endpoint] ("host:port"). Returns
         * the handshake to present to the user (remote name + SAS), or a
         * terminal non-pending outcome.
         */
        fun requestPairing(
            endpoint: String,
            identityManager: DeviceIdentityManager,
            trustStore: TrustStore?,
        ): PairingOutcome {
            if (endpoint.isBlank()) return PairingOutcome.Failed("No endpoint for the selected device")
            val token = newToken()
            val payload = JSONObject().apply {
                put("display_name", identityManager.displayName)
                put("platform", identityManager.platform)
                put("public_key", CryptoUtils.toHex(identityManager.rawPublicKey))
                put("pairing_token", token)
            }

            val response = post("http://$endpoint/pairing/request", payload.toString())
                ?: return PairingOutcome.Failed("Could not reach $endpoint")
            if (response.code == 409) {
                // The peer already trusts this device's key: redundant request,
                // not a dialog. Report it so the UI can say "already trusted".
                return PairingOutcome.AlreadyTrusted
            }
            if (response.code != 200) {
                return PairingOutcome.Failed("Pairing request refused (${response.code})")
            }
            val body = try {
                JSONObject(response.body)
            } catch (t: Throwable) {
                return PairingOutcome.Failed("Malformed pairing response")
            }
            val remotePub = try {
                CryptoUtils.fromHex(body.getString("public_key"))
            } catch (t: Throwable) {
                return PairingOutcome.Failed("Malformed pairing response")
            }
            if (remotePub.size != 32) {
                return PairingOutcome.Failed("Malformed pairing response")
            }

            // Both sides derive the SAS from the same (pub, pub, token) triple;
            // a mismatch means the peer is not who the exchange says it is.
            val expectedSas = CryptoUtils.calculateSAS(identityManager.rawPublicKey, remotePub, token)
            val remoteSas = body.optString("sas")
            if (remoteSas != expectedSas) {
                return PairingOutcome.Failed("SAS verification failed")
            }

            val pending = OutboundPairing(
                endpoint = endpoint,
                token = token,
                sas = expectedSas,
                remoteName = body.optString("display_name", "Remote Device"),
                remotePlatform = body.optString("platform", ""),
                remotePub = remotePub,
                createdAt = System.currentTimeMillis(),
            )
            outbound[pending.token] = pending
            return PairingOutcome.Ready(pending.token, pending.remoteName, expectedSas)
        }

        /**
         * Confirms (or rejects) a pending outbound pairing and waits for the
         * receiving user's decision. Returns true only when the peer answered
         * 200 paired — which is when trust is committed on BOTH sides.
         */
        fun confirmPairing(
            token: String,
            identityManager: DeviceIdentityManager,
            trustStore: TrustStore?,
            confirmed: Boolean,
        ): Boolean {
            val pending = outbound.remove(token) ?: return false
            if (System.currentTimeMillis() - pending.createdAt > MAX_POLL_MS) return false

            val sigMaterial = "${pending.token}:${pending.sas}".toByteArray(StandardCharsets.UTF_8)
            val signature = identityManager.sign(sigMaterial)
            val payload = JSONObject().apply {
                put("device_id", identityManager.deviceId)
                put("pairing_token", pending.token)
                put("sas", pending.sas)
                put("confirmed", confirmed)
                put("signature", CryptoUtils.toHex(signature))
            }

            // The requester's own rejection still reaches the receiver so its
            // pending dialog is withdrawn instead of lingering to expiry.
            val deadline = System.currentTimeMillis() + MAX_POLL_MS
            while (true) {
                val response = post("http://${pending.endpoint}/pairing/confirm", payload.toString())
                    ?: return false
                when (response.code) {
                    202 -> {
                        if (System.currentTimeMillis() > deadline) return false
                        try {
                            Thread.sleep(POLL_MS)
                        } catch (_: InterruptedException) {
                            return false
                        }
                    }
                    200 -> {
                        if (confirmed && trustStore != null) {
                            // The peer is identified by the fingerprint of the
                            // key its accept response carried — the same id
                            // the peer commits for us.
                            trustStore.addTrusted(
                                TrustedDeviceRecord(
                                    deviceId = CryptoUtils.fingerprint(pending.remotePub),
                                    displayName = pending.remoteName,
                                    platform = pending.remotePlatform,
                                    rawPublicKey = pending.remotePub,
                                    pairedAtMs = System.currentTimeMillis(),
                                    lastSeenMs = System.currentTimeMillis(),
                                    revoked = false,
                                ),
                            )
                        }
                        return confirmed
                    }
                    else -> return false
                }
            }
        }

        private fun newToken(): String {
            val bytes = ByteArray(16)
            java.security.SecureRandom().nextBytes(bytes)
            // Hex, matching the Go requester's token shape.
            return bytes.joinToString("") { "%02x".format(it) }
        }

        private fun post(url: String, body: String): HttpResult? {
            var connection: HttpURLConnection? = null
            return try {
                val conn = URL(url).openConnection() as HttpURLConnection
                connection = conn
                conn.requestMethod = "POST"
                conn.connectTimeout = 10_000
                conn.readTimeout = 10_000
                conn.doOutput = true
                conn.setRequestProperty("Content-Type", "application/json")
                conn.setFixedLengthStreamingMode(body.toByteArray(StandardCharsets.UTF_8).size)
                conn.outputStream.use { it.write(body.toByteArray(StandardCharsets.UTF_8)) }
                val code = conn.responseCode
                val stream = if (code in 200..399) conn.inputStream else conn.errorStream
                val text = stream?.bufferedReader(StandardCharsets.UTF_8)?.use(BufferedReader::readText) ?: ""
                HttpResult(code, text)
            } catch (t: Throwable) {
                Log.w(TAG, "pairing POST failed: ${t.message}")
                null
            } finally {
                connection?.disconnect()
            }
        }
    }

    private class HttpResult(val code: Int, val body: String)

    /** One outbound pairing awaiting the local user's SAS confirm. */
    data class OutboundPairing(
        val endpoint: String,
        val token: String,
        val sas: String,
        val remoteName: String,
        val remotePlatform: String,
        val remotePub: ByteArray,
        val createdAt: Long,
    )

    /** Result of [requestPairing]; only [Ready] leads to a SAS dialog. */
    sealed class PairingOutcome {
        data class Ready(val token: String, val remoteName: String, val sas: String) : PairingOutcome()
        object AlreadyTrusted : PairingOutcome()
        data class Failed(val message: String) : PairingOutcome()
    }
}
