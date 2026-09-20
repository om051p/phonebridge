package dev.phonebridge.ui

import android.os.Build
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.util.Log
import dev.phonebridge.bridge.GoBridge
import dev.phonebridge.capture.CaptureConfig
import dev.phonebridge.service.PhoneBridgeService
import io.flutter.embedding.android.FlutterActivity
import io.flutter.embedding.engine.FlutterEngine
import io.flutter.plugin.common.EventChannel
import io.flutter.plugin.common.MethodCall
import io.flutter.plugin.common.MethodChannel
import java.net.HttpURLConnection
import java.net.URL

class MainActivity : FlutterActivity(), MethodChannel.MethodCallHandler, EventChannel.StreamHandler {

    companion object {
        private const val TAG = "MainActivity"
        private const val CONTROL_CHANNEL = "dev.phonebridge/control"
        private const val EVENTS_CHANNEL = "dev.phonebridge/events"
        private const val STATS_INTERVAL_MS = 1000L
    }

    private var eventSink: EventChannel.EventSink? = null
    private val mainHandler = Handler(Looper.getMainLooper())
    private var pendingReceiverUrl: String? = null

    private val statsRunnable = object : Runnable {
        override fun run() {
            eventSink?.let { sink ->
                try {
                    sink.success(collectStats())
                } catch (e: Exception) {
                    Log.w(TAG, "Failed to emit stats event: ${e.message}")
                }
                mainHandler.postDelayed(this, STATS_INTERVAL_MS)
            }
        }
    }

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
                initiateWebRtcHandshake(pendingReceiverUrl!!)
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

    override fun onMethodCall(call: MethodCall, result: MethodChannel.Result) {
        when (call.method) {
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
                    "maxPayloadSize" to dev.phonebridge.clipboard.AndroidClipboardAdapter.MAX_PAYLOAD_SIZE
                ))
            }
            "triggerClipboardPull" -> {
                val ok = dev.phonebridge.clipboard.AndroidClipboardAdapter.triggerManualPull()
                result.success(ok)
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
                    pendingReceiverUrl = null
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
            else -> result.notImplemented()
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
            "imeSelected" to dev.phonebridge.clipboard.AndroidClipboardAdapter.checkImeSelected(this)
        )
    }

    private fun initiateWebRtcHandshake(receiverUrl: String) {
        Thread {
            try {
                // Allow media transport to initialize
                Thread.sleep(400)
                if (!GoBridge.loaded) {
                    Log.w(TAG, "Cannot negotiate WebRTC: GoBridge not loaded")
                    return@Thread
                }
                Log.i(TAG, "Initiating WebRTC offer to $receiverUrl...")
                val offerBytes = GoBridge.mediaCreateOffer()
                val endpoint = if (receiverUrl.endsWith("/offer")) receiverUrl else "$receiverUrl/offer"
                val conn = (URL(endpoint).openConnection() as HttpURLConnection).apply {
                    requestMethod = "POST"
                    doOutput = true
                    connectTimeout = 4000
                    readTimeout = 4000
                    setRequestProperty("Content-Type", "application/json")
                }
                conn.outputStream.use { it.write(offerBytes) }
                val code = conn.responseCode
                if (code == 200) {
                    val answerBytes = conn.inputStream.use { it.readBytes() }
                    GoBridge.mediaSetAnswer(answerBytes)
                    GoBridge.mediaStart()
                    Log.i(TAG, "WebRTC connected to receiver at $endpoint: streaming started")
                } else {
                    Log.w(TAG, "Receiver returned HTTP $code")
                }
            } catch (t: Throwable) {
                Log.w(TAG, "Receiver WebRTC handshake skipped/failed: ${t.message}")
            }
        }.start()
    }

    override fun onListen(arguments: Any?, events: EventChannel.EventSink?) {
        eventSink = events
        // Emit immediate initial state
        try {
            events?.success(collectStats())
        } catch (e: Exception) {
            Log.w(TAG, "Initial stats emission failed: ${e.message}")
        }
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
