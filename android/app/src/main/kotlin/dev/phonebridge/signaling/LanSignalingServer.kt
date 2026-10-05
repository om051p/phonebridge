package dev.phonebridge.signaling

import android.util.Log
import dev.phonebridge.bridge.GoBridge
import dev.phonebridge.security.AuthValidator
import dev.phonebridge.security.CryptoUtils
import dev.phonebridge.security.DeviceIdentityManager
import dev.phonebridge.security.TrustStore
import dev.phonebridge.security.TrustedDeviceRecord
import org.json.JSONObject
import java.io.BufferedReader
import java.io.InputStreamReader
import java.io.OutputStream
import java.net.ServerSocket
import java.net.Socket
import java.nio.charset.StandardCharsets
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.ExecutorService
import java.util.concurrent.Executors
import java.util.concurrent.atomic.AtomicBoolean

/**
 * LanSignalingServer exposes an HTTP/1.1 REST signaling and pairing endpoint on the local network.
 *
 * Implements the approved LAN signaling protocol with mutual Ed25519 authentication:
 * - GET  /health          -> 200 {"status":"ok"}
 * - POST /pairing/request -> 200 {"display_name":"...","platform":"...","public_key":"...","sas":"..."}
 *                           or 409 {"error":"already trusted"}
 * - POST /pairing/confirm -> 200 {"status":"paired"} once the receiving user
 *                           approved via [respondToPairing]; 202 {"status":"pending"}
 *                           while they have not decided; 400 on rejection or expiry.
 * - POST /session/offer   -> 200 {"type":"offer","sdp":"...","accepted":true,"actual":{...}} (Authenticated)
 *                            or a typed refusal: 409 {"accepted":false,"code":"...","message":"..."}
 * - POST /session/answer  -> 200 {"status":"ok"} (Authenticated)
 * - POST /session/stop    -> 200 {"status":"ok"} (Authenticated)
 *
 * The session endpoints are ratified by DEC-022: parameters are settled before
 * the offer exists, and the answer states what this device will actually apply.
 */
class LanSignalingServer(
    val port: Int = DEFAULT_PORT,
    private val handler: SignalingHandler = DefaultSignalingHandler(),
    private val identityManager: DeviceIdentityManager? = null,
    val trustStore: TrustStore? = null
) {
    companion object {
        private const val TAG = "LanSignalingServer"
        const val DEFAULT_PORT = 7804

        /**
         * How long a pairing token stays valid. Expired tokens are rejected
         * and removed at confirm time; the user simply pairs again
         * (idempotent: trust commits upsert by device ID).
         */
        internal const val PENDING_PAIRING_TTL_MS = 5 * 60 * 1000L
    }

    interface SignalingHandler {
        /**
         * Judges the negotiation request and, when it is accepted, creates the
         * SDP offer. A rejected request must not touch the media transport.
         */
        fun handleOffer(request: SessionOfferRequest): SessionOfferAnswer

        /** Applies the remote SDP answer JSON {"type":"answer","sdp":"..."} and starts streaming */
        fun handleAnswer(answerJson: ByteArray): Boolean

        /** Stops media transport */
        fun handleStop(reason: String)
    }

    /**
     * Default handler binding to GoBridge JNI.
     *
     * @param liveCapture reports the capture pipeline's *measured* state, or
     *        null when nothing is being captured. Defaults to "not capturing"
     *        so an unwired handler cannot claim a tuple it has not applied.
     * @param capabilities what the selected encoder can actually do.
     */
    class DefaultSignalingHandler(
        private val liveCapture: () -> LiveCapture? = { null },
        private val capabilities: () -> DeviceMediaCapabilities = { DeviceMediaCapabilities() },
    ) : SignalingHandler {
        override fun handleOffer(request: SessionOfferRequest): SessionOfferAnswer {
            val caps = capabilities()
            val result = SessionNegotiation.negotiate(request, liveCapture(), caps)
            if (result is SessionNegotiationResult.Rejected) {
                // Refusals never build a peer connection: an unnegotiable request
                // must leave any existing transport untouched.
                return SessionOfferAnswer(sdp = "", result = result, capabilities = caps)
            }
            if (!GoBridge.loaded) {
                // No native transport means no real offer. A fabricated SDP
                // would only move the failure to an unparseable-offer error on
                // the peer; fail with the typed transport code instead.
                return SessionOfferAnswer(
                    sdp = "",
                    result = SessionNegotiationResult.Rejected(
                        httpStatus = 503,
                        code = SessionNegotiation.CODE_TRANSPORT_FAILED,
                        message = "native transport not loaded",
                    ),
                    capabilities = caps,
                )
            }
            // Transport-only repair: this rebuilds the peer connection, and is
            // deliberately not allowed to touch the capture pipeline, because
            // DEC-020 records that restarting capture cannot resume delivery and
            // a geometry change needs a fresh consent.
            GoBridge.mediaRelease()
            GoBridge.mediaInit()
            val offerBytes = GoBridge.mediaCreateOffer()
            // GoBridge.mediaCreateOffer returns the transport's SDP JSON blob
            // {"type":"offer","sdp":"v=0..."}; the wire payload carries the SDP
            // TEXT as the sdp member, so unwrap the blob here. Embedding the
            // blob verbatim double-encodes it and the peer sees a leading
            // quote instead of "v=0" (found in Phase 2 acceptance).
            val offerObj = JSONObject(String(offerBytes, StandardCharsets.UTF_8))
            val sdp = offerObj.optString("sdp")
            if (sdp.isBlank()) {
                throw IllegalStateException("mediaCreateOffer returned no sdp field")
            }
            return SessionOfferAnswer(
                sdp = sdp,
                result = result,
                capabilities = caps,
            )
        }

        override fun handleAnswer(answerJson: ByteArray): Boolean {
            if (!GoBridge.loaded) return true
            GoBridge.mediaSetAnswer(answerJson)
            GoBridge.mediaStart()
            return true
        }

        override fun handleStop(reason: String) {
            if (GoBridge.loaded) {
                GoBridge.mediaStop()
                GoBridge.mediaRelease()
            }
        }
    }

    internal data class PendingPairing(
        val token: String,
        val remoteName: String,
        val remotePlatform: String,
        val remotePub: ByteArray,
        val sas: String,
        val createdAt: Long = System.currentTimeMillis(),
        // The receiving user's explicit decision: null while the request is
        // still awaiting them, true/false once they accept/reject. Trust is
        // committed only after approved == true AND a valid confirm — the
        // same contract as the Go signaling server (Phase 2).
        val approved: Boolean? = null,
    )

    /** Receiver-side view of one pending pairing request, surfaced to the UI. */
    data class PairingRequestInfo(
        val token: String,
        val remoteName: String,
        val remotePlatform: String,
        val sas: String,
        val createdAtMs: Long,
    )

    private var serverSocket: ServerSocket? = null
    private var executor: ExecutorService? = null
    private val isRunning = AtomicBoolean(false)
    private val authValidator = if (trustStore != null) AuthValidator(trustStore) else null
    internal val pendingPairings = ConcurrentHashMap<String, PendingPairing>()

    /**
     * Fired (off the server lock) whenever a NEW inbound pairing request is
     * stored, so the service can raise the Pairing Request notification and
     * tell the UI. Never fired for a retry that supersedes an older request —
     * the duplicate protection below guarantees one dialog per peer.
     */
    @Volatile
    var onPairingRequest: ((PairingRequestInfo) -> Unit)? = null

    /**
     * Sweeps expired pending pairings. Called from every read path and from
     * request/confirm handling, so an expired request disappears even with no
     * confirm arriving to reap it.
     */
    private fun sweepExpiredLocked(): Boolean {
        val now = System.currentTimeMillis()
        var removed = false
        val it = pendingPairings.entries.iterator()
        while (it.hasNext()) {
            val entry = it.next()
            if (now - entry.value.createdAt > PENDING_PAIRING_TTL_MS) {
                it.remove()
                removed = true
            }
        }
        return removed
    }

    /**
     * Snapshots the inbound pairing requests still awaiting (or decided-but-
     * unconsumed) for the UI. Expired entries are swept and omitted, so the
     * UI auto-dismisses stale dialogs by refreshing this list.
     */
    fun listPendingPairings(): List<PairingRequestInfo> {
        sweepExpiredLocked()
        return pendingPairings.values
            .map { PairingRequestInfo(it.token, it.remoteName, it.remotePlatform, it.sas, it.createdAt) }
            .sortedBy { it.createdAtMs }
    }

    /**
     * Records the receiving user's explicit accept/reject for one pending
     * request. False when the token is unknown or already expired. The
     * decision takes effect on the requester's next confirm poll; approval
     * alone never commits trust.
     */
    fun respondToPairing(token: String, approved: Boolean): Boolean {
        sweepExpiredLocked()
        val current = pendingPairings[token] ?: return false
        pendingPairings[token] = current.copy(approved = approved)
        return true
    }

    val running: Boolean
        get() = isRunning.get()

    /**
     * Starts listening on the configured port in a background thread.
     */
    @Synchronized
    fun start(): Boolean {
        if (isRunning.get()) return true

        return try {
            val ss = ServerSocket(port)
            ss.reuseAddress = true
            serverSocket = ss
            val exec = Executors.newCachedThreadPool()
            executor = exec
            isRunning.set(true)

            exec.execute {
                acceptLoop(ss)
            }
            logI(TAG, "LanSignalingServer listening on port $port")
            true
        } catch (t: Throwable) {
            logE(TAG, "Failed to bind signaling ServerSocket on port $port: ${t.message}", t)
            false
        }
    }

    /**
     * Stops the HTTP server.
     */
    @Synchronized
    fun stop() {
        if (!isRunning.compareAndSet(true, false)) return

        try {
            serverSocket?.close()
        } catch (_: Throwable) {}
        serverSocket = null

        executor?.shutdownNow()
        executor = null
        logI(TAG, "LanSignalingServer stopped")
    }

    private fun logI(tag: String, msg: String) {
        try {
            Log.i(tag, msg)
        } catch (_: Throwable) {
            println("INFO: [$tag] $msg")
        }
    }

    private fun logW(tag: String, msg: String) {
        try {
            Log.w(tag, msg)
        } catch (_: Throwable) {
            println("WARN: [$tag] $msg")
        }
    }

    private fun logE(tag: String, msg: String, t: Throwable? = null) {
        try {
            Log.e(tag, msg, t)
        } catch (_: Throwable) {
            println("ERROR: [$tag] $msg ${t?.message ?: ""}")
        }
    }

    private fun acceptLoop(ss: ServerSocket) {
        while (isRunning.get() && !ss.isClosed) {
            try {
                val client = ss.accept()
                executor?.execute {
                    handleClient(client)
                }
            } catch (t: Throwable) {
                if (isRunning.get()) {
                    logW(TAG, "Accept error: ${t.message}")
                }
            }
        }
    }

    private fun handleClient(socket: Socket) {
        try {
            socket.soTimeout = 10000 // 10s socket read timeout
            val input = BufferedReader(InputStreamReader(socket.getInputStream(), StandardCharsets.UTF_8))
            val output = socket.getOutputStream()

            val requestLine = input.readLine() ?: return
            val parts = requestLine.split(" ")
            if (parts.size < 2) {
                sendResponse(output, 400, "Bad Request", "text/plain", "Invalid HTTP request line".toByteArray())
                return
            }

            val method = parts[0].uppercase()
            val path = parts[1]

            val headers = mutableMapOf<String, String>()
            var contentLength = 0
            var line = input.readLine()
            while (!line.isNullOrEmpty()) {
                val header = line.split(":", limit = 2)
                if (header.size == 2) {
                    val k = header[0].trim().lowercase()
                    val v = header[1].trim()
                    headers[k] = v
                    if (k == "content-length") {
                        contentLength = v.toIntOrNull() ?: 0
                    }
                }
                line = input.readLine()
            }

            val body = if (contentLength > 0) {
                val buf = CharArray(contentLength)
                var readTotal = 0
                while (readTotal < contentLength) {
                    val r = input.read(buf, readTotal, contentLength - readTotal)
                    if (r == -1) break
                    readTotal += r
                }
                String(buf, 0, readTotal).toByteArray(StandardCharsets.UTF_8)
            } else {
                ByteArray(0)
            }

            dispatch(method, path, headers, body, output)
        } catch (t: Throwable) {
            logW(TAG, "Error handling client ${socket.inetAddress}: ${t.message}")
        } finally {
            try {
                socket.close()
            } catch (_: Throwable) {}
        }
    }

    private fun dispatch(
        method: String,
        path: String,
        headers: Map<String, String>,
        body: ByteArray,
        out: OutputStream
    ) {
        when {
            method == "GET" && path == "/health" -> {
                val resp = """{"status":"ok"}""".toByteArray(StandardCharsets.UTF_8)
                sendResponse(out, 200, "OK", "application/json", resp)
            }

            method == "POST" && path == "/pairing/request" -> {
                handlePairingRequest(body, out)
            }

            method == "POST" && path == "/pairing/confirm" -> {
                handlePairingConfirm(body, out)
            }

            method == "POST" && (path == "/session/offer" || path == "/session/answer" || path == "/session/stop") -> {
                if (authValidator != null) {
                    val authResult = authValidator.verify(method, path, body) { headerName ->
                        headers[headerName.lowercase()]
                    }
                    when (authResult) {
                        is AuthValidator.AuthResult.Failure -> {
                            val err = """{"error":"${authResult.message}"}""".toByteArray(StandardCharsets.UTF_8)
                            val statusStr = if (authResult.statusCode == 401) "Unauthorized" else "Forbidden"
                            sendResponse(out, authResult.statusCode, statusStr, "application/json", err)
                            return
                        }
                        is AuthValidator.AuthResult.Success -> {
                            trustStore?.touchLastSeen(authResult.deviceId)
                        }
                    }
                }

                when (path) {
                    "/session/offer" -> {
                        try {
                            val request = parseOfferRequest(body)
                            val answer = handler.handleOffer(request)
                            val payload = SessionNegotiationJson.offerResponse(answer)
                                .toByteArray(StandardCharsets.UTF_8)
                            when (val result = answer.result) {
                                is SessionNegotiationResult.Rejected -> sendResponse(
                                    out,
                                    result.httpStatus,
                                    if (result.httpStatus == 409) "Conflict" else "Bad Request",
                                    "application/json",
                                    payload,
                                )
                                is SessionNegotiationResult.Accepted -> sendResponse(
                                    out, 200, "OK", "application/json", payload,
                                )
                            }
                        } catch (t: Throwable) {
                            logW(TAG, "/session/offer failed: ${t.message}")
                            val err = JSONObject()
                                .put("accepted", false)
                                .put("code", "CAPTURE_FAILED")
                                .put("message", t.message ?: "offer failed")
                                .toString()
                                .toByteArray(StandardCharsets.UTF_8)
                            sendResponse(out, 500, "Internal Server Error", "application/json", err)
                        }
                    }
                    "/session/answer" -> {
                        try {
                            val ok = handler.handleAnswer(body)
                            if (ok) {
                                val resp = """{"status":"ok"}""".toByteArray(StandardCharsets.UTF_8)
                                sendResponse(out, 200, "OK", "application/json", resp)
                            } else {
                                val err = """{"error":"failed to apply answer"}""".toByteArray(StandardCharsets.UTF_8)
                                sendResponse(out, 400, "Bad Request", "application/json", err)
                            }
                        } catch (t: Throwable) {
                            val err = """{"error":"${t.message}"}""".toByteArray(StandardCharsets.UTF_8)
                            sendResponse(out, 500, "Internal Server Error", "application/json", err)
                        }
                    }
                    "/session/stop" -> {
                        val reason = if (body.isNotEmpty()) String(body, StandardCharsets.UTF_8) else "stopped"
                        handler.handleStop(reason)
                        val resp = """{"status":"ok"}""".toByteArray(StandardCharsets.UTF_8)
                        sendResponse(out, 200, "OK", "application/json", resp)
                    }
                }
            }

            else -> {
                sendResponse(out, 404, "Not Found", "text/plain", "Endpoint not found".toByteArray())
            }
        }
    }

    /**
     * Parses the offer body tolerantly: an empty or malformed body is treated as
     * an older peer that predates DEC-022, which still gets a usable offer rather
     * than a hard failure. The negotiation fields then simply go unreported.
     */
    internal fun parseOfferRequest(body: ByteArray): SessionOfferRequest {
        if (body.isEmpty()) return SessionOfferRequest()
        return try {
            SessionOfferRequest.parse(JSONObject(String(body, StandardCharsets.UTF_8)))
        } catch (t: Throwable) {
            logW(TAG, "Malformed session offer body (${t.message}); treating as a pre-DEC-022 peer")
            SessionOfferRequest()
        }
    }

    private fun handlePairingRequest(body: ByteArray, out: OutputStream) {
        if (identityManager == null) {
            val err = """{"error":"device identity not configured"}""".toByteArray(StandardCharsets.UTF_8)
            sendResponse(out, 503, "Service Unavailable", "application/json", err)
            return
        }

        try {
            val json = JSONObject(String(body, StandardCharsets.UTF_8))
            val remoteName = json.optString("display_name", "Remote Device")
            val remotePlatform = json.optString("platform", "linux")
            val remotePubHex = json.getString("public_key")
            val token = json.getString("pairing_token")

            val remotePub = CryptoUtils.fromHex(remotePubHex)
            if (remotePub.size != 32) {
                val err = """{"error":"invalid public key length"}""".toByteArray(StandardCharsets.UTF_8)
                sendResponse(out, 400, "Bad Request", "application/json", err)
                return
            }

            // Re-pair of an already-trusted key is redundant, never a new
            // request: 409 lets the requester say "already trusted" instead of
            // opening an approval dialog for a peer both sides already trust.
            // A revoked key re-pairs through the normal approval flow. The
            // store is re-read first: the UI's trust mutations (revoke) land
            // in the shared file, and this server's in-memory instance must
            // not answer 409 from a stale view of trust.
            trustStore?.reload()
            val existing = trustStore?.findByPublicKey(remotePub)
            if (existing != null && !existing.revoked) {
                val err = """{"error":"already trusted"}""".toByteArray(StandardCharsets.UTF_8)
                sendResponse(out, 409, "Conflict", "application/json", err)
                return
            }

            val sas = CryptoUtils.calculateSAS(identityManager.rawPublicKey, remotePub, token)

            // Duplicate-request protection: one pending request per remote
            // key. A retry (new token, same peer) supersedes the older one
            // instead of stacking dialogs on the receiver.
            val it = pendingPairings.entries.iterator()
            while (it.hasNext()) {
                val entry = it.next()
                if (entry.value.remotePub.size == remotePub.size &&
                    java.security.MessageDigest.isEqual(entry.value.remotePub, remotePub)
                ) {
                    it.remove()
                }
            }

            pendingPairings[token] = PendingPairing(
                token = token,
                remoteName = remoteName,
                remotePlatform = remotePlatform,
                remotePub = remotePub,
                sas = sas
            )

            val resp = JSONObject()
            resp.put("display_name", identityManager.displayName)
            resp.put("platform", identityManager.platform)
            resp.put("public_key", CryptoUtils.toHex(identityManager.rawPublicKey))
            resp.put("sas", sas)
            sendResponse(out, 200, "OK", "application/json", resp.toString().toByteArray(StandardCharsets.UTF_8))

            // Off the request path: the notification and UI hook must never
            // delay the 200 the requester is waiting on, and a throwing
            // listener must not fail an already-answered request.
            try {
                onPairingRequest?.invoke(
                    PairingRequestInfo(token, remoteName, remotePlatform, sas, System.currentTimeMillis())
                )
            } catch (t: Throwable) {
                logW(TAG, "pairing request listener failed: ${t.message}")
            }
        } catch (t: Throwable) {
            val err = """{"error":"${t.message}"}""".toByteArray(StandardCharsets.UTF_8)
            sendResponse(out, 400, "Bad Request", "application/json", err)
        }
    }

    private fun handlePairingConfirm(body: ByteArray, out: OutputStream) {
        try {
            val json = JSONObject(String(body, StandardCharsets.UTF_8))
            val devId = json.getString("device_id")
            val token = json.getString("pairing_token")
            val sas = json.getString("sas")
            val confirmed = json.getBoolean("confirmed")
            val sigHex = json.getString("signature")

            sweepExpiredLocked()
            val pending = pendingPairings[token]
            if (pending == null || pending.sas != sas) {
                val err = """{"error":"invalid or expired pairing token/sas"}""".toByteArray(StandardCharsets.UTF_8)
                sendResponse(out, 400, "Bad Request", "application/json", err)
                return
            }
            if (System.currentTimeMillis() - pending.createdAt > PENDING_PAIRING_TTL_MS) {
                pendingPairings.remove(token)
                val err = """{"error":"pairing token expired"}""".toByteArray(StandardCharsets.UTF_8)
                sendResponse(out, 400, "Bad Request", "application/json", err)
                return
            }

            if (!confirmed) {
                // Requester-side rejection withdraws the request: the
                // receiver's pending dialog disappears on its next refresh
                // instead of asking about a peer that already walked away.
                pendingPairings.remove(token)
                val err = """{"error":"pairing rejected by user"}""".toByteArray(StandardCharsets.UTF_8)
                sendResponse(out, 400, "Bad Request", "application/json", err)
                return
            }

            // The receiving user has not decided yet: hold the token (202) so
            // the requester polls. Trust is committed only after an explicit
            // approval — never by the confirm alone.
            if (pending.approved == null) {
                val resp = """{"status":"pending"}""".toByteArray(StandardCharsets.UTF_8)
                sendResponse(out, 202, "Accepted", "application/json", resp)
                return
            }
            if (!pending.approved) {
                pendingPairings.remove(token)
                val err = """{"error":"pairing rejected by user"}""".toByteArray(StandardCharsets.UTF_8)
                sendResponse(out, 400, "Bad Request", "application/json", err)
                return
            }

            val sigMaterial = "$token:$sas".toByteArray(StandardCharsets.UTF_8)
            val sig = CryptoUtils.fromHex(sigHex)
            val peerPubKey = CryptoUtils.parsePublicKey(pending.remotePub)

            if (!CryptoUtils.verify(peerPubKey, sigMaterial, sig)) {
                pendingPairings.remove(token)
                val err = """{"error":"invalid confirmation signature"}""".toByteArray(StandardCharsets.UTF_8)
                sendResponse(out, 401, "Unauthorized", "application/json", err)
                return
            }

            // Never trust the requester's claimed device_id blindly: the
            // authenticated identity is pending.remotePub (the key this token
            // was issued for and the signature verified against). A mismatch
            // would fork a second logical trust record for the same key.
            if (devId != CryptoUtils.fingerprint(pending.remotePub)) {
                pendingPairings.remove(token)
                val err = """{"error":"device_id does not match authenticated public key"}""".toByteArray(StandardCharsets.UTF_8)
                sendResponse(out, 400, "Bad Request", "application/json", err)
                return
            }

            // Single-use token: a successful pairing consumes it, so a
            // replayed confirm can never commit trust twice.
            pendingPairings.remove(token)

            if (trustStore != null) {
                val rec = TrustedDeviceRecord(
                    deviceId = devId,
                    displayName = pending.remoteName,
                    platform = pending.remotePlatform,
                    rawPublicKey = pending.remotePub,
                    pairedAtMs = System.currentTimeMillis(),
                    lastSeenMs = System.currentTimeMillis(),
                    revoked = false
                )
                trustStore.upsertCanonical(rec)
            }

            val resp = """{"status":"paired"}""".toByteArray(StandardCharsets.UTF_8)
            sendResponse(out, 200, "OK", "application/json", resp)
        } catch (t: Throwable) {
            val err = """{"error":"${t.message}"}""".toByteArray(StandardCharsets.UTF_8)
            sendResponse(out, 400, "Bad Request", "application/json", err)
        }
    }

    private fun sendResponse(out: OutputStream, code: Int, status: String, contentType: String, body: ByteArray) {
        val header = "HTTP/1.1 $code $status\r\n" +
            "Content-Type: $contentType\r\n" +
            "Content-Length: ${body.size}\r\n" +
            "Connection: close\r\n\r\n"
        out.write(header.toByteArray(StandardCharsets.UTF_8))
        if (body.isNotEmpty()) {
            out.write(body)
        }
        out.flush()
    }
}
