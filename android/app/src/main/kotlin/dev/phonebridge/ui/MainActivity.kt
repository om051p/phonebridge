package dev.phonebridge.ui

import android.os.Build
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.util.Log
import dev.phonebridge.bridge.GoBridge
import dev.phonebridge.capture.CaptureConfig
import dev.phonebridge.service.PhoneBridgeService
import dev.phonebridge.signaling.LanSignalingServer
import io.flutter.embedding.android.FlutterActivity
import io.flutter.embedding.engine.FlutterEngine
import io.flutter.plugin.common.EventChannel
import io.flutter.plugin.common.MethodCall
import io.flutter.plugin.common.MethodChannel
import org.json.JSONArray
import org.json.JSONObject
import java.net.HttpURLConnection
import java.net.URL

class MainActivity : FlutterActivity(), MethodChannel.MethodCallHandler, EventChannel.StreamHandler {

    companion object {
        private const val TAG = "MainActivity"
        private const val CONTROL_CHANNEL = "dev.phonebridge/control"
        private const val EVENTS_CHANNEL = "dev.phonebridge/events"
        private const val STATS_INTERVAL_MS = 1000L

        // The production DEC-022 signaling contract. The path is part of the
        // signature, so it is defined once here and never assembled piecemeal.
        private const val PEER_OFFER_PATH = "/session/peer-offer"
        private const val STOP_PATH = "/session/stop"
        private const val SIGNALING_VERSION = 1
    }

    private var eventSink: EventChannel.EventSink? = null
    private val mainHandler = Handler(Looper.getMainLooper())
    private var pendingReceiverUrl: String? = null

    /// Fingerprint of the last stats map that was actually pushed, minus the
    /// timestamp. The tick stays at 1 Hz, but an idle phone only sends when
    /// something changed: Flutter's handlers and rebuilds are the expensive part
    /// of an event nobody acts on. Capturing always pushes, because the UI
    /// derives currentFps from successive timestampMs values.
    private var lastPushedStatsFingerprint: String? = null

    private val statsRunnable = object : Runnable {
        override fun run() {
            eventSink?.let { sink ->
                try {
                    val stats = collectStats()
                    val fingerprint = statsFingerprintOf(stats)
                    if (stats["isCapturing"] == true || fingerprint != lastPushedStatsFingerprint) {
                        lastPushedStatsFingerprint = fingerprint
                        sink.success(stats)
                    }
                } catch (e: Exception) {
                    Log.w(TAG, "Failed to emit stats event: ${e.message}")
                }
                emitTransferEvents()
                mainHandler.postDelayed(this, STATS_INTERVAL_MS)
            }
        }
    }

    /// Last emitted fingerprint (state|bytes) per transfer id, so the 1 s tick
    /// only pushes an event when something actually changed. Flutter's
    /// TransferController dedupes as well, but we do not spam the channel.
    private val transferFingerprints = HashMap<String, String>()

    private var methodChannel: MethodChannel? = null

    private val navReceiver = object : android.content.BroadcastReceiver() {
        override fun onReceive(context: android.content.Context?, intent: android.content.Intent?) {
            val action = intent?.action ?: return
            if (action == "dev.phonebridge.NAVIGATE") {
                val tab = intent.getIntExtra("tab", 0)
                mainHandler.post {
                    methodChannel?.invokeMethod("onNavigateTab", mapOf("tab" to tab))
                }
            } else if (action == "dev.phonebridge.NAVIGATE_ROUTE") {
                val route = intent.getStringExtra("route") ?: "/"
                mainHandler.post {
                    methodChannel?.invokeMethod("onNavigateRoute", mapOf("route" to route))
                }
            } else if (action == "dev.phonebridge.TRIGGER_ACTION") {
                val cmd = intent.getStringExtra("cmd") ?: return
                val args = intent.getStringExtra("args")
                mainHandler.post {
                    methodChannel?.invokeMethod("onTriggerAction", mapOf("cmd" to cmd, "args" to args))
                }
            }
        }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        val pm = getSystemService(android.content.Context.POWER_SERVICE) as? android.os.PowerManager
        @Suppress("DEPRECATION")
        val wl = pm?.newWakeLock(
            android.os.PowerManager.SCREEN_BRIGHT_WAKE_LOCK or
                android.os.PowerManager.ACQUIRE_CAUSES_WAKEUP or
                android.os.PowerManager.ON_AFTER_RELEASE,
            "phonebridge:wake"
        )
        wl?.acquire(10000L)

        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O_MR1) {
            setShowWhenLocked(true)
            setTurnScreenOn(true)
        } else {
            @Suppress("DEPRECATION")
            window.addFlags(
                android.view.WindowManager.LayoutParams.FLAG_SHOW_WHEN_LOCKED or
                android.view.WindowManager.LayoutParams.FLAG_TURN_SCREEN_ON
            )
        }
        window.addFlags(android.view.WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON)

        val filter = android.content.IntentFilter().apply {
            addAction("dev.phonebridge.NAVIGATE")
            addAction("dev.phonebridge.NAVIGATE_ROUTE")
            addAction("dev.phonebridge.TRIGGER_ACTION")
        }
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
            registerReceiver(navReceiver, filter, android.content.Context.RECEIVER_EXPORTED)
        } else {
            registerReceiver(navReceiver, filter)
        }
        requestNotificationPermission()
    }

    override fun onStart() {
        super.onStart()
        // Returning from the input-method settings is when the default keyboard
        // can have changed, so the cached answer must not outlive it.
        dev.phonebridge.clipboard.AndroidClipboardAdapter.invalidateImeCheck()
        try {
            PhoneBridgeService.startService(this)
            Log.i(TAG, "PhoneBridgeService started from MainActivity.onStart")
        } catch (t: Throwable) {
            Log.e(TAG, "Failed to start PhoneBridgeService from MainActivity.onStart: ${t.message}", t)
        }
    }

    override fun configureFlutterEngine(flutterEngine: FlutterEngine) {
        super.configureFlutterEngine(flutterEngine)

        val channel = MethodChannel(flutterEngine.dartExecutor.binaryMessenger, CONTROL_CHANNEL)
        channel.setMethodCallHandler(this)
        this.methodChannel = channel

        EventChannel(flutterEngine.dartExecutor.binaryMessenger, EVENTS_CHANNEL)
            .setStreamHandler(this)

        // Listen for capture state changes from PhoneBridgeService
        PhoneBridgeService.stateListener = { isCapturing, errorReason ->
            mainHandler.post {
                val stats = collectStats().toMutableMap()
                if (errorReason != null) {
                    stats["lastError"] = errorReason
                }
                try {
                    eventSink?.success(stats)
                } catch (e: Exception) {
                    Log.w(TAG, "EventSink error: ${e.message}")
                }
            }

            if (isCapturing && !pendingReceiverUrl.isNullOrBlank()) {
                initiateDesktopSession(pendingReceiverUrl!!)
            }
        }
    }

    private val trustStore by lazy {
        dev.phonebridge.security.TrustStore(java.io.File(applicationContext.filesDir, "trusted_devices.json"))
    }
    private val identityManager by lazy {
        try {
            dev.phonebridge.security.DeviceIdentityManager.loadOrGenerate(applicationContext)
        } catch (e: Exception) {
            null
        }
    }

    private fun getDeviceState(): Map<String, Any?> {
        val engine = PhoneBridgeService.activeCaptureEngine
        return mapOf(
            "model" to Build.MODEL,
            "manufacturer" to Build.MANUFACTURER,
            "sdkInt" to Build.VERSION.SDK_INT,
            "isCapturing" to (engine?.isCapturing == true),
            "goEngineLoaded" to GoBridge.loaded,
            "codec" to (engine?.selectedCodecName ?: "none"),
            "isHardwareCodec" to (engine?.isHardwareCodec ?: false),
            "deviceId" to (identityManager?.deviceId ?: "unknown"),
            "displayName" to (identityManager?.displayName ?: Build.MODEL ?: "Android Device"),
            "clipboardState" to dev.phonebridge.clipboard.AndroidClipboardAdapter.state.name,
            "imeSelected" to dev.phonebridge.clipboard.AndroidClipboardAdapter.checkImeSelected(this)
        )
    }

    private fun isNotificationPermissionGranted(): Boolean {
        return if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
            checkSelfPermission(android.Manifest.permission.POST_NOTIFICATIONS) == android.content.pm.PackageManager.PERMISSION_GRANTED
        } else {
            true
        }
    }

    private fun isNotificationListenerEnabled(): Boolean {
        val cn = android.content.ComponentName(this, dev.phonebridge.notification.PhoneBridgeNotificationListenerService::class.java)
        val flat = android.provider.Settings.Secure.getString(contentResolver, "enabled_notification_listeners")
        return flat != null && flat.contains(cn.flattenToString())
    }

    private fun isAccessibilityEnabled(): Boolean {
        val cn = android.content.ComponentName(this, dev.phonebridge.input.PhoneBridgeAccessibilityService::class.java)
        val enabled = android.provider.Settings.Secure.getString(contentResolver, android.provider.Settings.Secure.ENABLED_ACCESSIBILITY_SERVICES)
        return enabled != null && enabled.contains(cn.flattenToString())
    }

    private fun getPermissionsStatus(): Map<String, Any> {
        return mapOf(
            "postNotifications" to isNotificationPermissionGranted(),
            "notificationListener" to isNotificationListenerEnabled(),
            "accessibility" to isAccessibilityEnabled(),
            "sdkInt" to Build.VERSION.SDK_INT
        )
    }

    private fun requestNotificationPermission() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
            if (!isNotificationPermissionGranted()) {
                requestPermissions(arrayOf(android.Manifest.permission.POST_NOTIFICATIONS), 1001)
            }
        }
    }

    override fun onMethodCall(call: MethodCall, result: MethodChannel.Result) {
        when (call.method) {
            "getPermissionsStatus" -> {
                result.success(getPermissionsStatus())
            }
            "requestNotificationPermission" -> {
                requestNotificationPermission()
                result.success(true)
            }
            "openNotificationListenerSettings" -> {
                try {
                    startActivity(android.content.Intent(android.provider.Settings.ACTION_NOTIFICATION_LISTENER_SETTINGS))
                    result.success(true)
                } catch (e: Exception) {
                    result.error("SETTINGS_ERROR", e.message, null)
                }
            }
            "openAccessibilitySettings" -> {
                try {
                    startActivity(android.content.Intent(android.provider.Settings.ACTION_ACCESSIBILITY_SETTINGS))
                    result.success(true)
                } catch (e: Exception) {
                    result.error("SETTINGS_ERROR", e.message, null)
                }
            }
            "openAppDetailsSettings" -> {
                try {
                    val intent = android.content.Intent(android.provider.Settings.ACTION_APPLICATION_DETAILS_SETTINGS).apply {
                        data = android.net.Uri.fromParts("package", packageName, null)
                    }
                    startActivity(intent)
                    result.success(true)
                } catch (e: Exception) {
                    result.error("SETTINGS_ERROR", e.message, null)
                }
            }
            "getDeviceState" -> {
                result.success(getDeviceState())
            }
            "getDeviceIdentity" -> {
                val id = identityManager
                if (id != null) {
                    result.success(mapOf(
                        "deviceId" to id.deviceId,
                        "displayName" to id.displayName,
                        "platform" to id.platform
                    ))
                } else {
                    result.success(mapOf(
                        "deviceId" to "unknown",
                        "displayName" to (Build.MODEL ?: "Android Device"),
                        "platform" to "android"
                    ))
                }
            }
            "getTrustedDevices" -> {
                try {
                    val list = trustStore.list().map { rec ->
                        mapOf(
                            "deviceId" to rec.deviceId,
                            "displayName" to rec.displayName,
                            "platform" to rec.platform,
                            "pairedAtMs" to rec.pairedAtMs,
                            "lastSeenMs" to rec.lastSeenMs,
                            "revoked" to rec.revoked
                        )
                    }
                    result.success(list)
                } catch (e: Exception) {
                    result.error("TRUST_STORE_ERROR", e.message, null)
                }
            }
            "revokeDevice" -> {
                val deviceId = call.argument<String>("deviceId")
                if (deviceId != null) {
                    val ok = trustStore.revoke(deviceId)
                    result.success(ok)
                } else {
                    result.error("INVALID_ARGUMENT", "deviceId is required", null)
                }
            }
            "removeDevice" -> {
                val deviceId = call.argument<String>("deviceId")
                if (deviceId != null) {
                    val ok = trustStore.remove(deviceId)
                    result.success(ok)
                } else {
                    result.error("INVALID_ARGUMENT", "deviceId is required", null)
                }
            }
            "getClipboardStatus" -> {
                val isImeSelected = dev.phonebridge.clipboard.AndroidClipboardAdapter.checkImeSelected(this)
                val state = dev.phonebridge.clipboard.AndroidClipboardAdapter.state.name
                result.success(mapOf(
                    "state" to state,
                    "imeSelected" to isImeSelected,
                    "enabled" to dev.phonebridge.clipboard.AndroidClipboardAdapter.enabled,
                    "maxPayloadSize" to dev.phonebridge.clipboard.AndroidClipboardAdapter.MAX_PAYLOAD_SIZE
                ))
            }
            "triggerClipboardPull" -> {
                val ok = dev.phonebridge.clipboard.AndroidClipboardAdapter.triggerManualPull()
                result.success(ok)
            }
            "setClipboardSyncEnabled" -> {
                val enabled = call.argument<Boolean>("enabled") ?: true
                dev.phonebridge.clipboard.AndroidClipboardAdapter.setSyncEnabled(enabled)
                emitClipboardState()
                result.success(true)
            }
            "openInputMethodSettings" -> {
                // Ambient clipboard observation needs the companion IME enabled
                // AND selected; without it Android never dispatches clipboard
                // changes to a background app, so phone copies cannot reach the
                // PC (Spike 05). This opens the screen where it is enabled.
                try {
                    startActivity(android.content.Intent(android.provider.Settings.ACTION_INPUT_METHOD_SETTINGS))
                    result.success(true)
                } catch (e: Exception) {
                    Log.e(TAG, "openInputMethodSettings failed", e)
                    result.error("SETTINGS_ERROR", e.message, null)
                }
            }
            "showInputMethodPicker" -> {
                try {
                    val imm = getSystemService(android.content.Context.INPUT_METHOD_SERVICE) as? android.view.inputmethod.InputMethodManager
                    if (imm == null) {
                        result.success(false)
                    } else {
                        imm.showInputMethodPicker()
                        result.success(true)
                    }
                } catch (e: Exception) {
                    Log.e(TAG, "showInputMethodPicker failed", e)
                    result.success(false)
                }
            }
            // ---- LAN discovery (DEC-007) ----------------------------------
            // The Go core browses _phonebridge._tcp and answers with the peers
            // it found (the Android build had no browse half before, so the
            // Devices tab could never list the PC).
            "getDiscoveredDevices" -> {
                try {
                    result.success(discoveredDeviceMaps())
                } catch (e: Exception) {
                    Log.e(TAG, "getDiscoveredDevices failed", e)
                    result.success(emptyList<Map<String, Any?>>())
                }
            }
            "startCapture" -> {
                try {
                    val receiverUrl = call.argument<String>("receiverUrl")
                    val width = call.argument<Int>("width") ?: 1080
                    val height = call.argument<Int>("height") ?: 2400
                    val fps = call.argument<Int>("fps") ?: 60
                    val bitrateKbps = call.argument<Int>("bitrateKbps") ?: 8000

                    pendingReceiverUrl = receiverUrl

                    val config = CaptureConfig(
                        width = width,
                        height = height,
                        fps = fps,
                        bitrate = bitrateKbps * 1000
                    )

                    Log.i(TAG, "Requesting capture consent: ${width}x${height}@${fps}fps, ${bitrateKbps}kbps, receiver=$receiverUrl")
                    CaptureConsentActivity.start(this, config)
                    result.success(true)
                } catch (e: Exception) {
                    Log.e(TAG, "startCapture failed", e)
                    result.error("START_CAPTURE_ERROR", e.message, null)
                }
            }
            "stopCapture" -> {
                try {
                    Log.i(TAG, "Stopping capture via PhoneBridgeService")
                    val stoppingPeer = pendingReceiverUrl
                    pendingReceiverUrl = null
                    // Release the desktop's side of the session now rather than
                    // leaving it to time out as SESSION_BUSY for the next offer.
                    notifySessionStop(stoppingPeer)
                    PhoneBridgeService.stopCapture(this)
                    result.success(true)
                } catch (e: Exception) {
                    Log.e(TAG, "stopCapture failed", e)
                    result.error("STOP_CAPTURE_ERROR", e.message, null)
                }
            }
            "getMediaStats" -> {
                result.success(collectStats())
            }
            // ---- File transfers (DEC-024, Phase 4 Step 4/5 wiring) ----------
            // Low-frequency request/response over the existing control channel,
            // forwarded to Go through the generic invoke("transfer:*") surface.
            "listTransfers" -> {
                try {
                    result.success(listTransferMaps())
                } catch (e: Exception) {
                    Log.e(TAG, "listTransfers failed", e)
                    result.error("TRANSFER_ERROR", e.message, null)
                }
            }
            "sendFile" -> {
                val path = call.argument<String>("localPath")
                val filename = call.argument<String>("filename")
                val deviceId = call.argument<String>("deviceId")
                if (path.isNullOrEmpty()) {
                    result.error("INVALID_ARGUMENT", "localPath is required", null)
                } else if (!GoBridge.loaded) {
                    result.error("unavailable", "native transfer channel not wired", null)
                } else {
                    try {
                        if (!deviceId.isNullOrEmpty()) {
                            GoBridge.transferSetPeer(deviceId)
                        }
                        val bytes = GoBridge.transferSend(path, filename)
                        if (bytes == null) {
                            result.error("unavailable", "native transfer channel not wired", null)
                        } else {
                            result.success(sendResultMap(JSONObject(String(bytes, Charsets.UTF_8))))
                            // Surface the new PENDING row without waiting a tick.
                            emitTransferEvents()
                        }
                    } catch (e: Exception) {
                        Log.e(TAG, "sendFile failed", e)
                        result.error("TRANSFER_ERROR", e.message, null)
                    }
                }
            }
            "cancelTransfer" -> {
                val transferId = call.argument<String>("transferId")
                if (transferId.isNullOrEmpty()) {
                    result.error("INVALID_ARGUMENT", "transferId is required", null)
                } else if (!GoBridge.loaded) {
                    result.success(false)
                } else {
                    try {
                        val bytes = GoBridge.transferCancel(transferId)
                        val json = bytes?.let { JSONObject(String(it, Charsets.UTF_8)) }
                        // Success only when the backend confirmed; the UI reflects
                        // the real state from the follow-up event, never locally.
                        result.success(json?.optBoolean("cancelled", false) ?: false)
                        emitTransferEvents()
                    } catch (e: Exception) {
                        Log.w(TAG, "cancelTransfer failed: ${e.message}")
                        result.success(false)
                    }
                }
            }
            else -> result.notImplemented()
        }
    }

    /// Snapshots the Go mDNS peer list (snake_case JSON) into the camelCase map
    /// shape DiscoveredDevice.fromMap expects. Empty when the core is not loaded
    /// or nothing is on the LAN yet — an empty list is a real answer here, never
    /// an error, because "no peers discovered" is the normal startup state.
    private fun discoveredDeviceMaps(): List<Map<String, Any?>> {
        if (!GoBridge.loaded) return emptyList()
        val bytes = GoBridge.discoveryList() ?: return emptyList()
        val rows = JSONArray(String(bytes, Charsets.UTF_8))
        // This device advertises itself over NSD (TXT `id`), and the Go browse
        // joins the same multicast group, so the phone resolves its own record.
        // Listing yourself as a connectable peer is both confusing and
        // impossible to act on, so the local id is filtered here — the same
        // self-filter the Linux daemon gets from discovery.Config.DeviceID.
        val localId = identityManager?.deviceId
        val out = ArrayList<Map<String, Any?>>(rows.length())
        for (i in 0 until rows.length()) {
            val obj = rows.optJSONObject(i) ?: continue
            val id = obj.optString("id")
            if (id.isEmpty()) continue
            if (localId != null && id == localId) continue
            out.add(
                mapOf(
                    "id" to id,
                    "name" to obj.optString("name", id),
                    "model" to obj.optString("model"),
                    "version" to obj.optString("version"),
                    "host" to obj.optString("host"),
                    "port" to obj.optInt("port"),
                    "isStale" to obj.optBoolean("is_stale", false),
                )
            )
        }
        return out
    }

    /// Re-emits the current clipboard fields immediately so a control action
    /// (e.g. toggling sync) is reflected without waiting for the 1 s tick.
    private fun emitClipboardState() {
        try {
            eventSink?.success(collectStats())
        } catch (e: Exception) {
            Log.w(TAG, "Clipboard state emission failed: ${e.message}")
        }
    }

    /// Snapshots the Go transfer list (snake_case JSON) into the camelCase map
    /// shape TransferItem.fromMap expects. Empty when the core is not loaded.
    private fun listTransferMaps(): List<Map<String, Any?>> {
        if (!GoBridge.loaded) return emptyList()
        val bytes = GoBridge.transferList() ?: return emptyList()
        val rows = JSONArray(String(bytes, Charsets.UTF_8))
        val out = ArrayList<Map<String, Any?>>(rows.length())
        for (i in 0 until rows.length()) {
            rows.optJSONObject(i)?.let { out.add(transferRowMap(it)) }
        }
        return out
    }

    private fun transferRowMap(obj: JSONObject): Map<String, Any?> = mapOf(
        "transferId" to obj.optString("transfer_id"),
        "direction" to obj.optString("direction"),
        "state" to obj.optString("state"),
        "peerDeviceId" to obj.optString("peer_device_id"),
        "filename" to obj.optString("filename"),
        "mimeType" to obj.optString("mime_type"),
        "sizeBytes" to obj.optLong("size_bytes"),
        "bytesTransferred" to obj.optLong("bytes_transferred"),
        "startedAtMs" to obj.optLong("started_at_ms"),
        "finishedAtMs" to obj.optLong("finished_at_ms"),
        "reasonCode" to obj.optString("reason"),
        "errorMessage" to obj.optString("error_message"),
        "savedName" to obj.optString("saved_name")
    )

    /// Maps the transfer:send JSON reply ({transfer_id, state} on success,
    /// {error, reason, code} on a typed failure) into the shape
    /// TransferSendResult.fromMap understands. Errors keep an empty transferId
    /// so the Dart side classifies them as a failed send.
    private fun sendResultMap(json: JSONObject): Map<String, Any?> = mapOf(
        "transferId" to json.optString("transfer_id"),
        "state" to json.optString("state"),
        "reasonCode" to json.optString("reason"),
        "errorMessage" to json.optString("error")
    )

    /// Pushes one {"transfer": <row>} event per transfer whose state or byte
    /// count changed since the last emission, through the existing
    /// dev.phonebridge/events channel (the same sink the stats use).
    private fun emitTransferEvents() {
        val sink = eventSink ?: return
        if (!GoBridge.loaded) return
        try {
            val bytes = GoBridge.transferList() ?: return
            val rows = JSONArray(String(bytes, Charsets.UTF_8))
            for (i in 0 until rows.length()) {
                val obj = rows.optJSONObject(i) ?: continue
                val id = obj.optString("transfer_id")
                if (id.isEmpty()) continue
                val fingerprint = obj.optString("state") + "|" + obj.optLong("bytes_transferred")
                if (transferFingerprints[id] == fingerprint) continue
                transferFingerprints[id] = fingerprint
                sink.success(mapOf("transfer" to transferRowMap(obj)))
            }
        } catch (e: Exception) {
            Log.w(TAG, "transfer event emission failed: ${e.message}")
        }
    }

    private fun collectStats(): Map<String, Any?> {
        val engine = PhoneBridgeService.activeCaptureEngine
        val isCapturing = engine?.isCapturing == true
        val encodedFrames = engine?.encodedFrames?.get() ?: 0L
        val keyframes = engine?.keyframes?.get() ?: 0L
        val admittedFrames = engine?.transportAdmittedFrames?.get() ?: 0L
        val droppedFrames = engine?.transportDroppedFrames?.get() ?: 0L
        val codec = engine?.selectedCodecName ?: "none"
        val isHardware = engine?.isHardwareCodec ?: false
        val lastPts = engine?.lastPtsUs?.get() ?: 0L
        val firstPts = engine?.firstPtsUs?.get() ?: 0L

        val goStatsJson = if (GoBridge.loaded) {
            GoBridge.mediaStats()?.let { String(it, Charsets.UTF_8) } ?: "{}"
        } else "{}"

        return mapOf(
            "isCapturing" to isCapturing,
            "encodedFrames" to encodedFrames,
            "keyframes" to keyframes,
            "admittedFrames" to admittedFrames,
            "droppedFrames" to droppedFrames,
            "codec" to codec,
            "isHardwareCodec" to isHardware,
            "durationUs" to if (lastPts > firstPts && firstPts > 0) (lastPts - firstPts) else 0L,
            "goStatsJson" to goStatsJson,
            "timestampMs" to System.currentTimeMillis(),
            "clipboardState" to dev.phonebridge.clipboard.AndroidClipboardAdapter.state.name,
            "imeSelected" to dev.phonebridge.clipboard.AndroidClipboardAdapter.checkImeSelected(this),
            // The adapter is the owner of this flag, so the UI reads the truth
            // from here instead of tracking its own optimistic copy.
            "enabled" to dev.phonebridge.clipboard.AndroidClipboardAdapter.enabled
        )
    }

    /**
     * Starts a session with the desktop over the production DEC-022 contract.
     *
     * The phone owns the capture pipeline, so it is the SDP offerer: it brings
     * its own offer to `POST /session/peer-offer` and the desktop answers in the
     * response. This replaced a POST to `/offer`, which is a route of the
     * standalone receiver binary and returns 404 against the daemon - and the
     * failure was only logged, so the UI showed a live share while no session
     * existed.
     *
     * The request is Ed25519-signed with the phone's device identity, which is
     * what makes the desktop's trust check able to authorize it. Runs off the
     * main thread: offer creation blocks up to ~2 s for ICE gathering.
     */
    private fun initiateDesktopSession(receiverUrl: String) {
        val base = normaliseEndpoint(receiverUrl) ?: return
        Thread {
            try {
                if (!GoBridge.loaded) {
                    reportSessionFailure("Native transport is not loaded")
                    return@Thread
                }
                val identity = identityManager
                if (identity == null) {
                    reportSessionFailure("Device identity is unavailable, so the session cannot be authenticated")
                    return@Thread
                }

                val offerBlob = GoBridge.mediaCreateOffer()
                val offerSdp = runCatching {
                    String(offerBlob, Charsets.UTF_8).let { JSONObject(it).optString("sdp") }
                }.getOrDefault("")
                if (offerSdp.isEmpty()) {
                    reportSessionFailure("Screen capture produced no SDP offer")
                    return@Thread
                }

                val body = JSONObject().apply {
                    put("protocol_version", SIGNALING_VERSION)
                    put("version", JSONObject().apply {
                        put("min", SIGNALING_VERSION)
                        put("max", SIGNALING_VERSION)
                    })
                    // The phone can only contribute a screen stream, so it
                    // advertises exactly that rather than claiming the desktop's
                    // clipboard/input planes.
                    put("capabilities", JSONArray().put("SCREEN"))
                    put("signaling_port", LanSignalingServer.DEFAULT_PORT)
                    put("offer", JSONObject().apply {
                        put("type", "offer")
                        put("sdp", offerSdp)
                    })
                }.toString().toByteArray(Charsets.UTF_8)

                val headers = identity.signRequest("POST", PEER_OFFER_PATH, body)

                val conn = (URL(base + PEER_OFFER_PATH).openConnection() as HttpURLConnection).apply {
                    requestMethod = "POST"
                    doOutput = true
                    connectTimeout = 5000
                    readTimeout = 8000
                    setRequestProperty("Content-Type", "application/json")
                    headers.forEach { (name, value) -> setRequestProperty(name, value) }
                }
                val code: Int
                val response: String
                try {
                    conn.outputStream.use { it.write(body) }
                    code = conn.responseCode
                    response = (if (code in 200..299) conn.inputStream else conn.errorStream)
                        ?.use { String(it.readBytes(), Charsets.UTF_8) } ?: ""
                } finally {
                    conn.disconnect()
                }

                if (code !in 200..299) {
                    reportSessionFailure(describePeerOfferFailure(code, response))
                    return@Thread
                }

                val answerSdp = runCatching { JSONObject(response).optString("sdp") }.getOrDefault("")
                if (answerSdp.isEmpty()) {
                    reportSessionFailure("The desktop accepted the session but returned no SDP answer")
                    return@Thread
                }
                // mediaSetAnswer expects the transport's SDP blob, not a bare SDP
                // string, so rebuild the same shape the offer side used.
                val answerBlob = JSONObject().apply {
                    put("type", "answer")
                    put("sdp", answerSdp)
                }.toString().toByteArray(Charsets.UTF_8)
                GoBridge.mediaSetAnswer(answerBlob)
                GoBridge.mediaStart()
                Log.i(TAG, "DEC-022 session established with desktop at $base")
            } catch (t: Throwable) {
                reportSessionFailure("Could not start the session: ${t.message ?: t.javaClass.simpleName}")
            }
        }.start()
    }

    /**
     * Normalises a discovered signaling URL into the endpoint we post to, or
     * null if it is unusable. Host and port come from the peer's mDNS record, so
     * a malformed one is reported rather than silently skipped.
     */
    private fun normaliseEndpoint(receiverUrl: String): String? {
        val base = receiverUrl.trim().trimEnd('/')
        val url = runCatching { URL(base) }.getOrNull()
        if (base.isEmpty() || url == null || url.host.isNullOrEmpty() || url.port <= 0) {
            reportSessionFailure("The desktop address is not usable: $receiverUrl")
            return null
        }
        return base
    }

    /**
     * Surfaces a failed session start to the UI. The Flutter layer reads
     * `lastError` from the next stats event, which is how every other native
     * failure is reported, so this stays on the existing channel.
     */
    private fun reportSessionFailure(reason: String) {
        Log.w(TAG, "Session start failed: $reason")
        mainHandler.post {
            PhoneBridgeService.stateListener?.invoke(
                PhoneBridgeService.activeCaptureEngine?.isCapturing == true,
                reason
            )
        }
    }

    /**
     * Turns a typed refusal into something actionable. The desktop answers with
     * a code from the shared DEC-022 vocabulary plus a human message, matching
     * the shape the phone itself uses when refusing a desktop offer.
     */
    private fun describePeerOfferFailure(code: Int, response: String): String {
        val payload = runCatching { JSONObject(response) }.getOrNull()
        return when (payload?.optString("code")) {
            "SESSION_BUSY" -> "The desktop is already in a session"
            "PERMISSION_DENIED" -> "This phone is not paired with the desktop"
            "INCOMPATIBLE_VERSION" -> "The desktop runs an incompatible session protocol"
            "TRANSPORT_FAILED" -> "The desktop could not open the media transport"
            "UNSUPPORTED_MEDIA_PARAMS" -> "The desktop rejected the requested capture format"
            else -> payload?.optString("message")?.takeIf { it.isNotEmpty() }
                ?: "The desktop refused the session (HTTP $code)"
        }
    }

    /**
     * Tells the desktop the session is over, so it releases the session
     * immediately instead of holding it until its own timeout expires - a held
     * session is what makes the next offer fail as SESSION_BUSY.
     */
    private fun notifySessionStop(receiverUrl: String?) {
        val base = receiverUrl?.trim()?.trimEnd('/').orEmpty()
        val identity = identityManager
        if (base.isEmpty() || identity == null) {
            return
        }
        Thread {
            try {
                val body = JSONObject().apply {
                    put("reason", "the phone stopped sharing")
                    put("reason_code", "OK")
                }.toString().toByteArray(Charsets.UTF_8)
                val headers = identity.signRequest("POST", STOP_PATH, body)
                val conn = (URL(base + STOP_PATH).openConnection() as HttpURLConnection).apply {
                    requestMethod = "POST"
                    doOutput = true
                    connectTimeout = 3000
                    readTimeout = 3000
                    setRequestProperty("Content-Type", "application/json")
                    headers.forEach { (name, value) -> setRequestProperty(name, value) }
                }
                try {
                    conn.outputStream.use { it.write(body) }
                    Log.i(TAG, "Session stop notice answered with HTTP ${conn.responseCode}")
                } finally {
                    conn.disconnect()
                }
            } catch (t: Throwable) {
                // Best effort: the desktop also times the session out on its own.
                Log.i(TAG, "Session stop notice not delivered: ${t.message}")
            }
        }.start()
    }

    override fun onListen(arguments: Any?, events: EventChannel.EventSink?) {
        eventSink = events
        // A fresh listener needs the current transfer states again: forget the
        // emission fingerprints so the first tick re-pushes every row.
        transferFingerprints.clear()
        // Emit immediate initial state
        try {
            events?.success(collectStats())
        } catch (e: Exception) {
            Log.w(TAG, "Initial stats emission failed: ${e.message}")
        }
        emitTransferEvents()
        mainHandler.postDelayed(statsRunnable, STATS_INTERVAL_MS)
    }

    override fun onCancel(arguments: Any?) {
        mainHandler.removeCallbacks(statsRunnable)
        eventSink = null
    }

    override fun onDestroy() {
        mainHandler.removeCallbacks(statsRunnable)
        PhoneBridgeService.stateListener = null
        try {
            unregisterReceiver(navReceiver)
        } catch (_: Exception) {}
        super.onDestroy()
    }
}

/**
 * Everything in a stats map except the timestamp, so two ticks with nothing
 * changed compare equal. The stats tick uses it to decide whether an idle phone
 * has anything worth sending: Flutter's handlers and rebuilds cost more than the
 * map itself, and a push nobody acts on is pure channel traffic.
 *
 * Defined at file scope so it can be unit tested without an Activity. Every
 * field [MainActivity] emits in collectStats must appear here: a field left out
 * would be a change the UI never hears about.
 */
internal fun statsFingerprintOf(stats: Map<String, Any?>): String = buildString {
    append(stats["isCapturing"])
    append('|'); append(stats["encodedFrames"])
    append('|'); append(stats["keyframes"])
    append('|'); append(stats["admittedFrames"])
    append('|'); append(stats["droppedFrames"])
    append('|'); append(stats["codec"])
    append('|'); append(stats["isHardwareCodec"])
    append('|'); append(stats["durationUs"])
    append('|'); append(stats["goStatsJson"])
    append('|'); append(stats["clipboardState"])
    append('|'); append(stats["imeSelected"])
    append('|'); append(stats["enabled"])
    append('|'); append(stats["lastError"])
}
