package dev.phonebridge.service

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.Service
import android.content.Context
import android.content.Intent
import android.content.pm.ServiceInfo
import android.media.projection.MediaProjectionManager
import android.os.Build
import android.os.IBinder
import android.util.Log
import androidx.core.app.NotificationCompat
import dev.phonebridge.bridge.GoBridge
import dev.phonebridge.clipboard.AndroidClipboardAdapter
import dev.phonebridge.transfer.AndroidTransferHostRegistry
import dev.phonebridge.capture.CaptureConfig
import dev.phonebridge.capture.CodecSelector
import dev.phonebridge.capture.ScreenCaptureEngine
import dev.phonebridge.discovery.NsdAdvertiser
import dev.phonebridge.security.DeviceIdentityManager
import dev.phonebridge.security.TrustStore
import dev.phonebridge.signaling.DeviceMediaCapabilities
import dev.phonebridge.signaling.LanSignalingServer
import dev.phonebridge.signaling.SessionNegotiation
import android.provider.Settings
import java.io.File

/**
 * PhoneBridgeService is the Android Foreground Service that hosts the Go core engine
 * and owns the screen capture pipeline.
 *
 * Ratified under DEC-019 and DEC-020.
 */
class PhoneBridgeService : Service() {

    companion object {
        private const val TAG = "PhoneBridgeService"

        const val CHANNEL_ID = "phonebridge_service_channel"
        const val NOTIFICATION_ID = 1001

        const val ACTION_START = "dev.phonebridge.action.START"
        const val ACTION_STOP = "dev.phonebridge.action.STOP"
        const val ACTION_START_CAPTURE = "dev.phonebridge.action.START_CAPTURE"
        const val ACTION_STOP_CAPTURE = "dev.phonebridge.action.STOP_CAPTURE"

        const val EXTRA_RESULT_CODE = "dev.phonebridge.extra.RESULT_CODE"
        const val EXTRA_RESULT_DATA = "dev.phonebridge.extra.RESULT_DATA"

        // Active engine instance exposed for instrumentation tests and health checks
        @Volatile
        var activeCaptureEngine: ScreenCaptureEngine? = null
            private set

        @Volatile
        var stateListener: ((Boolean, String?) -> Unit)? = null

        fun startService(context: Context) {
            val intent = Intent(context, PhoneBridgeService::class.java).apply {
                action = ACTION_START
            }
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
                context.startForegroundService(intent)
            } else {
                context.startService(intent)
            }
        }

        fun stopService(context: Context) {
            val intent = Intent(context, PhoneBridgeService::class.java).apply {
                action = ACTION_STOP
            }
            context.startService(intent)
        }

        /**
         * Starts screen capture. Must only be called after consent has been acquired
         * via MediaProjectionManager.createScreenCaptureIntent().
         */
        fun startCapture(
            context: Context,
            resultCode: Int,
            resultData: Intent,
            config: CaptureConfig = CaptureConfig()
        ) {
            val intent = Intent(context, PhoneBridgeService::class.java).apply {
                action = ACTION_START_CAPTURE
                putExtra(EXTRA_RESULT_CODE, resultCode)
                putExtra(EXTRA_RESULT_DATA, resultData)
                putExtras(config.toBundle())
            }
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
                context.startForegroundService(intent)
            } else {
                context.startService(intent)
            }
        }

        fun stopCapture(context: Context) {
            val intent = Intent(context, PhoneBridgeService::class.java).apply {
                action = ACTION_STOP_CAPTURE
            }
            context.startService(intent)
        }
    }

    private var captureEngine: ScreenCaptureEngine? = null
    private var signalingServer: LanSignalingServer? = null
    private var nsdAdvertiser: NsdAdvertiser? = null

    override fun onCreate() {
        super.onCreate()
        createNotificationChannel()
        startLanServices()
    }

    private fun startLanServices() {
        try {
            val identityManager = DeviceIdentityManager.loadOrGenerate(applicationContext)
            val trustStore = TrustStore(File(applicationContext.filesDir, "trusted_devices.json"))

            // DEC-022: the phone answers with what it will actually apply, so the
            // negotiation handler reads the live capture pipeline's *measured*
            // state rather than a configured constant (the encoder ignores the
            // requested frame rate, per DEC-020).
            val handler = LanSignalingServer.DefaultSignalingHandler(
                liveCapture = { captureEngine?.takeIf { it.isCapturing }?.liveCapture() },
                capabilities = { deviceMediaCapabilities() },
            )

            val server = LanSignalingServer(
                port = LanSignalingServer.DEFAULT_PORT,
                handler = handler,
                identityManager = identityManager,
                trustStore = trustStore
            )
            if (server.start()) {
                signalingServer = server
                Log.i(TAG, "LanSignalingServer started on port ${server.port} with deviceId=${identityManager.deviceId}")
            }
            val advertiser = NsdAdvertiser(applicationContext)
            nsdAdvertiser = advertiser
            val deviceId = identityManager.deviceId
            advertiser.registerService(
                port = LanSignalingServer.DEFAULT_PORT,
                deviceId = deviceId,
                deviceName = identityManager.displayName,
                capabilities = "screen",
                state = "ready"
            )
            Log.i(TAG, "NsdAdvertiser registered for device $deviceId")
        } catch (t: Throwable) {
            Log.e(TAG, "Failed to start LAN services: ${t.message}", t)
        }
    }

    /**
     * Advertises what this device can capture, taken from the encoder the
     * selector actually picks (DEC-020: hardware H.264, Surface input). Values
     * stay at 0 where the platform does not report a bound, which the
     * negotiation treats as "no stated limit" rather than "unsupported".
     */
    private fun deviceMediaCapabilities(): DeviceMediaCapabilities {
        return try {
            val selection = CodecSelector.select(CaptureConfig())
            val caps = selection.candidate?.caps?.videoCapabilities
            val codecName = if (selection.candidate != null) "h264" else ""
            DeviceMediaCapabilities(
                codecs = if (codecName.isEmpty()) emptyList() else listOf(codecName),
                maxWidth = caps?.supportedWidths?.upper ?: 0,
                maxHeight = caps?.supportedHeights?.upper ?: 0,
                maxFps = caps?.supportedFrameRates?.upper?.toInt() ?: 0,
                widthAlignment = caps?.widthAlignment ?: 0,
                heightAlignment = caps?.heightAlignment ?: 0,
                supportsScreen = selection.candidate != null,
            )
        } catch (t: Throwable) {
            Log.w(TAG, "Failed to read encoder capabilities: ${t.message}")
            // Undisclosed capabilities must not become a false refusal: report
            // screen support and no bounds so the platform check decides.
            DeviceMediaCapabilities()
        }
    }

    private fun stopLanServices() {
        try {
            nsdAdvertiser?.unregisterService()
            nsdAdvertiser = null
            signalingServer?.stop()
            signalingServer = null
        } catch (t: Throwable) {
            Log.w(TAG, "Error stopping LAN services: ${t.message}")
        }
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        when (intent?.action) {
            ACTION_STOP -> {
                stopCaptureInternal("service_stopped")
                stopGoEngine()
                stopForeground(STOP_FOREGROUND_REMOVE)
                stopSelf()
                return START_NOT_STICKY
            }
            ACTION_START_CAPTURE -> {
                val rc = intent.getIntExtra(EXTRA_RESULT_CODE, 0)
                val data: Intent? = if (Build.VERSION.SDK_INT >= 33) {
                    intent.getParcelableExtra(EXTRA_RESULT_DATA, Intent::class.java)
                } else {
                    @Suppress("DEPRECATION")
                    intent.getParcelableExtra(EXTRA_RESULT_DATA)
                }

                if (data == null) {
                    Log.e(TAG, "Cannot start capture: resultData intent is null")
                    return START_STICKY
                }

                // Elevate foreground service type to include mediaProjection
                startForegroundWithNotification(isCapturing = true)
                startGoEngine()

                val config = CaptureConfig.fromIntent(intent)
                startCaptureInternal(rc, data, config)
                return START_STICKY
            }
            ACTION_STOP_CAPTURE -> {
                stopCaptureInternal("stop_capture_requested")
                startForegroundWithNotification(isCapturing = false)
                return START_STICKY
            }
            else -> {
                startForegroundWithNotification(isCapturing = captureEngine?.isCapturing == true)
                startGoEngine()
                return START_STICKY
            }
        }
    }

    override fun onTrimMemory(level: Int) {
        super.onTrimMemory(level)
        GoBridge.trimMemory(level)
    }

    override fun onDestroy() {
        stopCaptureInternal("service_destroyed")
        stopLanServices()
        stopGoEngine()
        AndroidTransferHostRegistry.release()
        super.onDestroy()
    }

    override fun onBind(intent: Intent?): IBinder? = null

    private fun startCaptureInternal(resultCode: Int, resultData: Intent, config: CaptureConfig) {
        if (captureEngine?.isCapturing == true) {
            Log.w(TAG, "Screen capture is already active; stopping previous session first")
            stopCaptureInternal("restarting")
        }

        val mpm = getSystemService(Context.MEDIA_PROJECTION_SERVICE) as? MediaProjectionManager
        if (mpm == null) {
            Log.e(TAG, "MediaProjectionManager not available")
            return
        }

        val projection = try {
            mpm.getMediaProjection(resultCode, resultData)
        } catch (t: Throwable) {
            Log.e(TAG, "mpm.getMediaProjection threw exception: ${t.message}", t)
            null
        }

        if (projection == null) {
            Log.e(TAG, "getMediaProjection returned null (invalid or expired token)")
            return
        }

        // Initialize Go media transport so onFrame() can admit AUs into the queue
        if (GoBridge.loaded) {
            try {
                GoBridge.mediaRelease()
                GoBridge.mediaInit()
                Log.i(TAG, "Go media transport initialized for capture session")
            } catch (t: Throwable) {
                Log.e(TAG, "Failed to initialize Go media transport: ${t.message}", t)
            }
        }

        val engine = ScreenCaptureEngine(
            context = applicationContext,
            config = config,
            listener = object : ScreenCaptureEngine.Listener {
                override fun onCaptureStarted() {
                    Log.i(TAG, "Screen capture started successfully")
                    stateListener?.invoke(true, null)
                }

                override fun onCaptureStopped(reason: String) {
                    Log.i(TAG, "Screen capture stopped: $reason")
                    if (captureEngine === activeCaptureEngine) {
                        activeCaptureEngine = null
                    }
                    if (GoBridge.loaded) {
                        GoBridge.mediaStop()
                        GoBridge.mediaRelease()
                    }
                    stateListener?.invoke(false, reason)
                }

                override fun onCaptureError(error: Throwable) {
                    Log.e(TAG, "Screen capture error: ${error.message}", error)
                    // Typed, and reported before teardown: a capture failure is
                    // not a transport failure, and the peer must be able to tell
                    // them apart (DEC-022).
                    reportSessionError(
                        SessionNegotiation.CODE_CAPTURE_FAILED,
                        error.message ?: "capture failed",
                    )
                    stateListener?.invoke(false, error.message)
                }

                override fun onConsentRevoked(reason: String) {
                    Log.w(TAG, "MediaProjection consent revoked: $reason")
                    reportSessionError(
                        SessionNegotiation.CODE_CONSENT_REVOKED,
                        "screen capture consent was withdrawn",
                    )
                }
            }
        )

        captureEngine = engine
        activeCaptureEngine = engine

        if (!engine.start(projection)) {
            Log.e(TAG, "Failed to start ScreenCaptureEngine")
            captureEngine = null
            activeCaptureEngine = null
            if (GoBridge.loaded) {
                GoBridge.mediaStop()
                GoBridge.mediaRelease()
            }
        }
    }

    private fun stopCaptureInternal(reason: String) {
        captureEngine?.stop(reason)
        captureEngine = null
        activeCaptureEngine = null
        if (GoBridge.loaded) {
            GoBridge.mediaStop()
            GoBridge.mediaRelease()
        }
    }

    /**
     * Sends a typed sender-side failure to the connected peer. Best effort by
     * design: if it cannot be delivered (no peer, no negotiated transport), the
     * teardown that follows is what the peer ultimately observes.
     */
    private fun reportSessionError(code: String, message: String) {
        if (!GoBridge.loaded) return
        val sent = try {
            GoBridge.mediaReportSessionError(code, message)
        } catch (t: Throwable) {
            Log.w(TAG, "Failed to report $code: ${t.message}")
            false
        }
        if (!sent) {
            Log.i(TAG, "Session error $code could not be delivered to a peer (no active session)")
        }
    }

    private fun startGoEngine() {
        if (GoBridge.loaded) {
            val storageDir = filesDir.absolutePath
            GoBridge.start(storageDir)
            AndroidClipboardAdapter.start(applicationContext)
            AndroidTransferHostRegistry.ensureStarted(applicationContext)
        }
    }

    private fun stopGoEngine() {
        if (GoBridge.loaded) {
            AndroidTransferHostRegistry.stopIfStarted()
            AndroidClipboardAdapter.stop()
            GoBridge.mediaStop()
            GoBridge.mediaRelease()
            GoBridge.stop()
        }
    }

    private fun startForegroundWithNotification(isCapturing: Boolean) {
        val notification = createNotification(isCapturing)
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
            val fgsType = if (isCapturing) {
                ServiceInfo.FOREGROUND_SERVICE_TYPE_CONNECTED_DEVICE or
                    ServiceInfo.FOREGROUND_SERVICE_TYPE_MEDIA_PROJECTION
            } else {
                ServiceInfo.FOREGROUND_SERVICE_TYPE_CONNECTED_DEVICE
            }
            startForeground(NOTIFICATION_ID, notification, fgsType)
        } else {
            startForeground(NOTIFICATION_ID, notification)
        }
    }

    private fun createNotification(isCapturing: Boolean): Notification {
        val contentText = if (isCapturing) {
            "Sharing screen and connected in background"
        } else {
            "Connected and running in background"
        }
        val icon = if (isCapturing) {
            android.R.drawable.ic_menu_camera
        } else {
            android.R.drawable.stat_notify_sync
        }

        return NotificationCompat.Builder(this, CHANNEL_ID)
            .setContentTitle("PhoneBridge")
            .setContentText(contentText)
            .setSmallIcon(icon)
            .setOngoing(true)
            .setPriority(NotificationCompat.PRIORITY_LOW)
            .build()
    }

    private fun createNotificationChannel() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            val channel = NotificationChannel(
                CHANNEL_ID,
                "PhoneBridge Background Service",
                NotificationManager.IMPORTANCE_LOW
            ).apply {
                description = "Ongoing notification required for background connectivity and screen capture"
            }
            val manager = getSystemService(NotificationManager::class.java)
            manager?.createNotificationChannel(channel)
        }
    }
}
