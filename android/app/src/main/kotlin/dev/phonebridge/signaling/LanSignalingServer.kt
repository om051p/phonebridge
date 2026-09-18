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
 * - POST /pairing/confirm -> 200 {"status":"paired"}
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
                return SessionOfferAnswer(
                    sdp = "v=0\r\no=- 0 0 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\n",
                    result = result,
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
            return SessionOfferAnswer(
                sdp = String(offerBytes, StandardCharsets.UTF_8),
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

    private data class PendingPairing(
        val token: String,
        val remoteName: String,
        val remotePlatform: String,
        val remotePub: ByteArray,
        val sas: String,
        val createdAt: Long = System.currentTimeMillis()
    )

    private var serverSocket: ServerSocket? = null
    private var executor: ExecutorService? = null
    private val isRunning = AtomicBoolean(false)
    private val authValidator = if (trustStore != null) AuthValidator(trustStore) else null
    private val pendingPairings = ConcurrentHashMap<String, PendingPairing>()

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
                            trustStore?.get(authResult.deviceId)?.let { existing ->
                                trustStore.addTrusted(existing.copy(lastSeenMs = System.currentTimeMillis()))
                            }
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

            val sas = CryptoUtils.calculateSAS(identityManager.rawPublicKey, remotePub, token)
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

            val pending = pendingPairings.remove(token)
            if (pending == null || pending.sas != sas) {
                val err = """{"error":"invalid or expired pairing token/sas"}""".toByteArray(StandardCharsets.UTF_8)
                sendResponse(out, 400, "Bad Request", "application/json", err)
                return
            }

            if (!confirmed) {
                val err = """{"error":"pairing rejected by user"}""".toByteArray(StandardCharsets.UTF_8)
                sendResponse(out, 400, "Bad Request", "application/json", err)
                return
            }

            val sigMaterial = "$token:$sas".toByteArray(StandardCharsets.UTF_8)
            val sig = CryptoUtils.fromHex(sigHex)
            val peerPubKey = CryptoUtils.parsePublicKey(pending.remotePub)

            if (!CryptoUtils.verify(peerPubKey, sigMaterial, sig)) {
                val err = """{"error":"invalid confirmation signature"}""".toByteArray(StandardCharsets.UTF_8)
                sendResponse(out, 401, "Unauthorized", "application/json", err)
                return
            }

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
                trustStore.addTrusted(rec)
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
