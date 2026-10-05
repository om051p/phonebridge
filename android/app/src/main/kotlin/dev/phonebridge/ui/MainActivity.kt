package dev.phonebridge.ui

import android.os.Build
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.util.Log
import dev.phonebridge.bridge.GoBridge
import dev.phonebridge.capture.CaptureConfig
import dev.phonebridge.clipboard.AndroidClipboardAdapter
import dev.phonebridge.clipboard.CLIPBOARD_STARTUP_MAX_STEPS
import dev.phonebridge.clipboard.CLIPBOARD_STARTUP_STEP_MS
import dev.phonebridge.clipboard.shouldWaitForAdapterStartup
import dev.phonebridge.input.PhoneBridgeAccessibilityService
import dev.phonebridge.notification.AndroidNotificationManager
import dev.phonebridge.service.PhoneBridgeService
import dev.phonebridge.signaling.DesktopSession
import dev.phonebridge.signaling.DesktopSessionException
import dev.phonebridge.signaling.DeviceMediaCapabilities
import io.flutter.embedding.android.FlutterActivity
import io.flutter.embedding.engine.FlutterEngine
import io.flutter.plugin.common.EventChannel
import io.flutter.plugin.common.MethodCall
import io.flutter.plugin.common.MethodChannel
import org.json.JSONArray
import org.json.JSONObject

class MainActivity : FlutterActivity(), MethodChannel.MethodCallHandler, EventChannel.StreamHandler {

    companion object {
        private const val TAG = "MainActivity"
        private const val CONTROL_CHANNEL = "dev.phonebridge/control"
        private const val EVENTS_CHANNEL = "dev.phonebridge/events"
        private const val STATS_INTERVAL_MS = 1000L
    }

    private var eventSink: EventChannel.EventSink? = null
    private val mainHandler = Handler(Looper.getMainLooper())

    /**
     * The current phone-initiated session attempt's identity and pending
     * desktop endpoint. Replaces the bare nullable URL: a failure consumes
     * the attempt exactly once (generation-guarded), so a failure
     * notification can never re-arm the same dial and stale callbacks can
     * never clear a newer attempt.
     */
    private val sessionAttempt = SessionAttemptState()

    /// Steps already spent waiting for a cold-started service to bring the
    /// clipboard adapter up during the current foreground transition.
    private var clipboardFocusSyncSteps = 0

    /// Fingerprint of the last stats map that was actually pushed, minus the
    /// timestamp. The tick stays at 1 Hz, but an idle phone only sends when
    /// something changed: Flutter's handlers and rebuilds are the expensive part
    /// of an event nobody acts on. Capturing always pushes, because the UI
    /// derives currentFps from successive timestampMs values.
    private var lastPushedStatsFingerprint: String? = null

    private val statsRunnable = object : Runnable {
        override fun run() {
            // A session can come up while this UI is on screen (the user just
            // pressed CONNECT), which is the moment a held local item can go
            // out. Single null check while nothing is held.
            AndroidClipboardAdapter.flushPendingLocalClip()
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

    /// Serial executor for pairing HTTP (request/confirm). The confirm call
    /// legitimately blocks polling on the receiver's 202 pending for up to the
    /// token TTL, so it must never run on the platform channel thread.
    private val pairingExecutor = java.util.concurrent.Executors.newSingleThreadExecutor { r ->
        Thread(r, "phonebridge-pairing").apply { isDaemon = true }
    }

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
        // Cold-start entry from the Pairing Request notification.
        handlePairingIntent(intent)
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

    /**
     * Keyboard-free phone→PC clipboard sync.
     *
     * Android 10+ lets an app read the clipboard only while it owns the focused
     * window, so becoming visible is the one moment this app can pick up what
     * the user just copied with their own keyboard selected. The adapter's gate
     * debounces the focus flaps, and no clipboard content is ever logged or
     * stored beyond the engine's single in-memory item.
     */
    override fun onWindowFocusChanged(hasFocus: Boolean) {
        super.onWindowFocusChanged(hasFocus)
        if (!hasFocus) return
        clipboardFocusSyncSteps = 0
        syncClipboardFromFocus()
    }

    ///
    /// One focus-triggered read, retried while the service is still coming up.
    ///
    /// The service starts asynchronously from [onStart], so a launch can gain
    /// focus before the clipboard adapter exists; the same bounded wait the
    /// Quick Settings tile uses covers that cold start instead of dropping the
    /// user's copy. Exhausting the budget still attempts the read, so the
    /// transition is never silently ignored.
    ///
    private fun syncClipboardFromFocus() {
        if (shouldWaitForAdapterStartup(
                AndroidClipboardAdapter.state,
                clipboardFocusSyncSteps,
                CLIPBOARD_STARTUP_MAX_STEPS
            )
        ) {
            clipboardFocusSyncSteps++
            mainHandler.postDelayed({ syncClipboardFromFocus() }, CLIPBOARD_STARTUP_STEP_MS)
            return
        }
        // A cold read leaves the item held until a transport exists; this
        // foreground moment with a live session is when it can go out. Runs
        // before the read so a newer copy supersedes it if the user copied
        // something else meanwhile.
        AndroidClipboardAdapter.flushPendingLocalClip()
        if (AndroidClipboardAdapter.readCurrentClipOnFocus()) {
            emitClipboardState()
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

            // A capture signal dials at most what the current attempt holds:
            // a consumed (failed) or invalidated (stopped) attempt dials
            // nothing, so a failure notification can never re-arm this path.
            sessionAttempt.dialForCaptureStart(isCapturing)?.let { (url, attemptGen) ->
                initiateDesktopSession(url, attemptGen)
            }
        }

        // Trust mutations (pairing commits included) can land from the
        // background responder path with no activity alive. Reload our view
        // of the shared store file and push a trustChanged event so Flutter
        // refreshes instead of showing stale UNPAIRED state.
        dev.phonebridge.security.TrustStore.changeListener = {
            mainHandler.post {
                try {
                    trustStore.reload()
                } catch (e: Exception) {
                    Log.w(TAG, "Trust reload failed: ${e.message}")
                }
                // Primary push: a direct method call, which does not depend
                // on EventChannel subscription timing (the events sink is
                // shared by several broadcast subscriptions).
                try {
                    methodChannel?.invokeMethod("trustChanged", null)
                } catch (e: Exception) {
                    Log.w(TAG, "trustChanged invoke failed: ${e.message}")
                }
                // Backup push on the events channel for any raw-events
                // subscriber holding the current sink.
                try {
                    eventSink?.success(mapOf("trustChanged" to true))
                } catch (e: Exception) {
                    Log.w(TAG, "trustChanged emission failed: ${e.message}")
                }
            }
        }

        // Inbound pairing (Phase 2): a request arrived at this device's
        // signaling server. Same dual push as trustChanged — the Flutter side
        // refreshes its inbound snapshot and (via AppScaffold) prompts the
        // Pairing Request dialog exactly once per request.
        PhoneBridgeService.pairingListener = { info ->
            mainHandler.post {
                try {
                    methodChannel?.invokeMethod("pairingChanged", null)
                } catch (e: Exception) {
                    Log.w(TAG, "pairingChanged invoke failed: ${e.message}")
                }
                try {
                    eventSink?.success(mapOf("pairingChanged" to true))
                } catch (e: Exception) {
                    Log.w(TAG, "pairingChanged emission failed: ${e.message}")
                }
            }
        }
    }

    override fun onNewIntent(intent: android.content.Intent) {
        super.onNewIntent(intent)
        handlePairingIntent(intent)
    }

    /**
     * The Pairing Request notification opens the app with the pending token;
     * this routes to the Devices tab and refreshes the inbound snapshot so
     * the dialog is the first thing the user sees. Tapping a notification
     * NEVER accepts anything — the decision stays in the dialog.
     */
    private fun handlePairingIntent(intent: android.content.Intent?) {
        val token = intent?.getStringExtra(dev.phonebridge.pairing.PairingNotifier.EXTRA_PAIRING_TOKEN) ?: return
        dev.phonebridge.pairing.PairingNotifier.cancel(applicationContext)
        mainHandler.post {
            try {
                methodChannel?.invokeMethod("onNavigateTab", mapOf("tab" to 1))
            } catch (e: Exception) {
                Log.w(TAG, "pairing intent navigate failed: ${e.message}")
            }
            try {
                methodChannel?.invokeMethod("pairingChanged", null)
            } catch (e: Exception) {
                Log.w(TAG, "pairingChanged invoke failed: ${e.message}")
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
            // Runtime liveness, distinct from the granted/configured flags
            // above: permission granted != service running. Each reads the
            // existing service singleton, so no new manager is introduced.
            "notificationServiceActive" to AndroidNotificationManager.isListenerConnected(),
            "accessibilityServiceActive" to (PhoneBridgeAccessibilityService.getInstance() != null),
            "foregroundServiceActive" to PhoneBridgeService.isRunning,
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
            // ---- Bidirectional pairing (Phase 2) ---------------------------
            // Requester half: PairingClient performs the HTTP exchange off the
            // main thread (a confirm poll can legally wait minutes on the
            // receiver's 202 pending) and replies when it finishes.
            "pairDevice" -> {
                val endpoint = call.argument<String>("endpoint")
                val identity = identityManager
                if (endpoint.isNullOrEmpty() || identity == null) {
                    result.error("INVALID_ARGUMENT", "endpoint is required", null)
                } else {
                    pairingExecutor.execute {
                        val outcome = dev.phonebridge.signaling.PairingClient.requestPairing(
                            endpoint, identity, trustStore
                        )
                        mainHandler.post {
                            when (outcome) {
                                is dev.phonebridge.signaling.PairingClient.PairingOutcome.Ready ->
                                    result.success(mapOf(
                                        "status" to "ready",
                                        "token" to outcome.token,
                                        "deviceName" to outcome.remoteName,
                                        "sas" to outcome.sas,
                                    ))
                                is dev.phonebridge.signaling.PairingClient.PairingOutcome.AlreadyTrusted ->
                                    result.success(mapOf("status" to "alreadyTrusted"))
                                is dev.phonebridge.signaling.PairingClient.PairingOutcome.Failed ->
                                    result.success(mapOf("status" to "failed", "message" to outcome.message))
                            }
                        }
                    }
                }
            }
            "confirmPairing" -> {
                val token = call.argument<String>("token")
                val confirmed = call.argument<Boolean>("confirmed") ?: false
                val identity = identityManager
                if (token.isNullOrEmpty() || identity == null) {
                    result.error("INVALID_ARGUMENT", "token is required", null)
                } else {
                    pairingExecutor.execute {
                        val ok = dev.phonebridge.signaling.PairingClient.confirmPairing(
                            token, identity, trustStore, confirmed
                        )
                        mainHandler.post {
                            result.success(ok)
                            // A pairing commit (or revoke elsewhere) changed
                            // trust; refresh Flutter's devices view.
                            if (ok && confirmed) {
                                try {
                                    methodChannel?.invokeMethod("trustChanged", null)
                                } catch (e: Exception) {
                                    Log.w(TAG, "trustChanged invoke failed: ${e.message}")
                                }
                            }
                        }
                    }
                }
            }
            // Receiver half: the pending inbound requests live in the
            // signaling server owned by the service; the UI reads and answers.
            // A response NEVER pairs by itself — the requester's signed
            // confirm must still arrive and verify.
            "listInboundPairings" -> {
                val server = PhoneBridgeService.activeSignalingServer
                result.success(server?.listPendingPairings()?.map { info ->
                    mapOf(
                        "token" to info.token,
                        "remoteName" to info.remoteName,
                        "remotePlatform" to info.remotePlatform,
                        "sas" to info.sas,
                        "createdAtMs" to info.createdAtMs,
                    )
                } ?: emptyList<Map<String, Any?>>())
            }
            "respondInboundPairing" -> {
                val token = call.argument<String>("token")
                val approved = call.argument<Boolean>("approved") ?: false
                if (token.isNullOrEmpty()) {
                    result.error("INVALID_ARGUMENT", "token is required", null)
                } else {
                    val ok = PhoneBridgeService.activeSignalingServer?.respondToPairing(token, approved) ?: false
                    if (ok) {
                        // The request was answered (or is gone): the
                        // "Pairing Request" notification has served its purpose.
                        dev.phonebridge.pairing.PairingNotifier.cancel(applicationContext)
                    }
                    result.success(ok)
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

                    // An explicit start retires any prior attempt (including a
                    // failed one): only this attempt's callbacks may dial or
                    // consume.
                    sessionAttempt.beginAttempt(receiverUrl)

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
                    val stoppingPeer = sessionAttempt.pendingUrl
                    sessionAttempt.invalidate()
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
            // ---- Screen capabilities (runtime truth for the quality presets) --
            // The same advertisement the signaling server serves
            // (PhoneBridgeService.currentMediaCapabilities, CodecSelector-read),
            // so the UI gates presets on the encoder that will actually run.
            // An empty map means "could not be determined" (unknown), never
            // "unsupported": the UI renders "Checking…" instead of guessing.
            "getMediaCapabilities" -> {
                try {
                    result.success(mediaCapabilitiesMap(PhoneBridgeService.currentMediaCapabilities()))
                } catch (e: Exception) {
                    Log.w(TAG, "getMediaCapabilities failed: ${e.message}")
                    result.success(emptyMap<String, Any?>())
                }
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

    /// Snapshots the LAN peer list into the camelCase map shape
    /// DiscoveredDevice.fromMap expects, in camelCase already because the
    /// platform browse reports it that way and the Go plane's snake_case JSON is
    /// translated here. Empty when nothing is on the LAN yet — an empty list is a
    /// real answer here, never an error, because "no peers discovered" is the
    /// normal startup state.
    ///
    /// The platform browse (NsdManager) is the source of truth: Android denies an
    /// app the netlink socket the Go core's mDNS client needs, so the core's
    /// browse cannot start on a phone and its (always empty) list is only kept as
    /// a fallback for builds where it does work.
    private fun discoveredDeviceMaps(): List<Map<String, Any?>> {
        // This device advertises itself over NSD (TXT `id`) and the platform
        // browse hears that advertisement back, so the phone resolves its own
        // record. Listing yourself as a connectable peer is both confusing and
        // impossible to act on, so the local id is filtered here — the same
        // self-filter the Linux daemon gets from discovery.Config.DeviceID.
        val localId = identityManager?.deviceId
        val out = ArrayList<Map<String, Any?>>()
        val seen = HashSet<String>()

        fun add(id: String, name: String, model: String, version: String, host: String, port: Int, isStale: Boolean) {
            if (id.isEmpty() || host.isEmpty() || port <= 0) return
            if (localId != null && id == localId) return
            if (!seen.add(id)) return
            out.add(
                mapOf(
                    "id" to id,
                    "name" to name.ifEmpty { id },
                    "model" to model,
                    "version" to version,
                    "host" to host,
                    "port" to port,
                    "isStale" to isStale,
                )
            )
        }

        for (row in PhoneBridgeService.platformPeers()) {
            add(
                id = row["id"] as? String ?: "",
                name = row["name"] as? String ?: "",
                model = row["model"] as? String ?: "",
                version = row["version"] as? String ?: "",
                host = row["host"] as? String ?: "",
                port = (row["port"] as? Number)?.toInt() ?: 0,
                isStale = row["isStale"] as? Boolean ?: false,
            )
        }

        if (GoBridge.loaded) {
            val bytes = GoBridge.discoveryList()
            if (bytes != null) {
                val rows = JSONArray(String(bytes, Charsets.UTF_8))
                for (i in 0 until rows.length()) {
                    val obj = rows.optJSONObject(i) ?: continue
                    add(
                        id = obj.optString("id"),
                        name = obj.optString("name"),
                        model = obj.optString("model"),
                        version = obj.optString("version"),
                        host = obj.optString("host"),
                        port = obj.optInt("port"),
                        isStale = obj.optBoolean("is_stale", false),
                    )
                }
            }
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
    private fun initiateDesktopSession(receiverUrl: String, attemptGeneration: Long) {
        val base = DesktopSession.normaliseEndpoint(receiverUrl)
        if (base == null) {
            failSessionAttempt(attemptGeneration, "The desktop address is not usable: $receiverUrl")
            return
        }
        Thread {
            try {
                if (!GoBridge.loaded) {
                    failSessionAttempt(attemptGeneration, "Native transport is not loaded")
                    return@Thread
                }
                val identity = identityManager
                if (identity == null) {
                    failSessionAttempt(attemptGeneration, "Device identity is unavailable, so the session cannot be authenticated")
                    return@Thread
                }

                val offerSdp = try {
                    DesktopSession.sdpFromOfferBlob(GoBridge.mediaCreateOffer())
                } catch (t: Throwable) {
                    failSessionAttempt(attemptGeneration, "Screen capture produced no SDP offer")
                    return@Thread
                }

                // The shared DEC-022 client posts the exact same signed offer
                // the Quick Settings cold-start restore uses, so the two entry
                // points can never drift apart on the wire.
                val answerSdp = DesktopSession.postPeerOffer(identity, base, offerSdp)
                // mediaSetAnswer expects the transport's SDP blob, not a bare SDP
                // string, so rebuild the same shape the offer side used.
                val answerBlob = JSONObject().apply {
                    put("type", "answer")
                    put("sdp", answerSdp)
                }.toString().toByteArray(Charsets.UTF_8)
                GoBridge.mediaSetAnswer(answerBlob)
                GoBridge.mediaStart()
                Log.i(TAG, "DEC-022 session established with desktop at $base")
            } catch (e: DesktopSessionException) {
                // Typed refusal: [message] is already the desktop's own
                // human-readable description of the wire code.
                failSessionAttempt(attemptGeneration, e.message ?: "The desktop refused the session (HTTP ${e.httpStatus})")
            } catch (t: Throwable) {
                failSessionAttempt(attemptGeneration, "Could not start the session: ${t.message ?: t.javaClass.simpleName}")
            }
        }.start()
    }

    /**
     * Ends one failed attempt, exactly once. Consuming the pending endpoint
     * (when this failure is still current — not a duplicate, not stale from
     * an attempt a newer start or stop already retired) is what breaks the
     * re-entry cycle: the failure notification below re-invokes the capture
     * listener, but with nothing pending it dials nothing. Reporting itself
     * is unchanged: the Flutter layer reads `lastError` from the next stats
     * event, which is how every other native failure is reported.
     */
    private fun failSessionAttempt(attemptGeneration: Long, reason: String) {
        sessionAttempt.consumeOnFailure(attemptGeneration)
        reportSessionFailure(reason)
    }

    /**
     * Surfaces a failed session start to the UI. Report-only by contract: it
     * must never start (or re-arm) a session — the dial lives solely in the
     * capture-state listener, gated on a pending attempt.
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
     * Tells the desktop the session is over, so it releases the session
     * immediately instead of holding it until its own timeout expires - a held
     * session is what makes the next offer fail as SESSION_BUSY. Best effort:
     * the desktop also times the session out on its own.
     */
    private fun notifySessionStop(receiverUrl: String?) {
        val base = DesktopSession.normaliseEndpoint(receiverUrl.orEmpty()) ?: return
        val identity = identityManager ?: return
        Thread {
            try {
                DesktopSession.postStop(identity, base)
                Log.i(TAG, "Session stop notice delivered to desktop")
            } catch (t: Throwable) {
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
        PhoneBridgeService.pairingListener = null
        dev.phonebridge.security.TrustStore.changeListener = null
        pairingExecutor.shutdownNow()
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

/**
 * Projects a [DeviceMediaCapabilities] advertisement onto the channel map
 * shape Flutter parses. Pure (no Android types) so it is unit-testable on
 * the host JVM; zero bounds pass through as 0 ("no stated limit", never
 * "unsupported" — that judgement belongs to the UI's capability model).
 */
internal fun mediaCapabilitiesMap(caps: DeviceMediaCapabilities): Map<String, Any?> = mapOf(
    "codecs" to caps.codecs,
    "maxWidth" to caps.maxWidth,
    "maxHeight" to caps.maxHeight,
    "maxFps" to caps.maxFps,
    "supportsScreen" to caps.supportsScreen,
)

/**
 * One phone-initiated session attempt's identity and pending endpoint.
 *
 * The capture-state listener dials the desktop when capture is up; a failure
 * must consume the attempt exactly once so the failure notification can
 * never re-arm the same dial (the hardware-observed ~300/s re-entry loop).
 * A newer attempt — or a stop — retires the generation, so duplicate and
 * stale failure callbacks are harmless no-ops and can never clear a newer
 * attempt's endpoint. Reading the dial is idempotent (stop needs the URL
 * later for the desktop stop notice); only failure consumes.
 *
 * All members are synchronized: starts land on the main thread while failure
 * reports arrive from the session thread.
 */
internal class SessionAttemptState {
    private var generation: Long = 0

    @Volatile
    internal var pendingUrl: String? = null
        private set

    /** An explicit start (or Tech stop, via [invalidate]) retires any prior attempt. */
    @Synchronized
    fun beginAttempt(url: String?): Long {
        generation++
        pendingUrl = url
        return generation
    }

    /** A stop retires the attempt and drops its endpoint. */
    @Synchronized
    fun invalidate() {
        generation++
        pendingUrl = null
    }

    /**
     * Consumes the failed attempt exactly once. Returns true only for the
     * first failure of the current generation; duplicates and stale
     * generations return false and change nothing.
     */
    @Synchronized
    fun consumeOnFailure(failedGeneration: Long): Boolean {
        if (failedGeneration != generation) return false
        generation++
        pendingUrl = null
        return true
    }

    /** The dial for a capture signal, or null when there is nothing to dial. */
    @Synchronized
    fun dialForCaptureStart(isCapturing: Boolean): Pair<String, Long>? {
        val url = pendingUrl
        return if (isCapturing && !url.isNullOrBlank()) url to generation else null
    }
}
