package dev.phonebridge.capture

import android.content.Context
import android.hardware.display.DisplayManager
import android.hardware.display.VirtualDisplay
import android.media.MediaCodec
import android.media.MediaCodecInfo
import android.media.MediaFormat
import android.media.projection.MediaProjection
import android.os.Build
import android.os.Handler
import android.os.HandlerThread
import android.os.SystemClock
import android.util.Log
import android.view.Surface
import dev.phonebridge.bridge.GoBridge
import dev.phonebridge.signaling.LiveCapture
import java.util.Locale
import java.util.concurrent.atomic.AtomicBoolean
import java.util.concurrent.atomic.AtomicLong

/**
 * ScreenCaptureEngine orchestrates the production screen capture and encoding pipeline:
 *
 *   MediaProjection (consented)
 *     → VirtualDisplay
 *     → Hardware H.264 MediaCodec (Surface input, Annex-B output)
 *     → GopTailFilter (prediction-safe GOP-tail dropping)
 *     → GoBridge.mediaOnFrame() (dedicated JNI data plane)
 *
 * Ratified under DEC-020 and DEC-021.
 */
class ScreenCaptureEngine(
    private val context: Context,
    val config: CaptureConfig = CaptureConfig(),
    private val listener: Listener? = null,
) {
    interface Listener {
        fun onCaptureStarted()
        fun onCaptureStopped(reason: String)
        fun onCaptureError(error: Throwable)

        /**
         * The system withdrew the MediaProjection consent (DEC-022). This is
         * distinct from a capture failure and from a transport failure: the
         * link may be perfectly healthy while there is no longer a screen to
         * send. Called before [onCaptureStopped] so the peer can be told the
         * typed reason while the transport is still alive.
         */
        fun onConsentRevoked(reason: String) {}
    }

    companion object {
        private const val TAG = "ScreenCaptureEngine"
        private const val DEQUEUE_TIMEOUT_US = 10_000L // 10 ms
    }

    private val isRunning = AtomicBoolean(false)
    private var drainThread: Thread? = null
    private var handlerThread: HandlerThread? = null
    private var handler: Handler? = null

    private var projection: MediaProjection? = null
    private var projectionCallback: MediaProjection.Callback? = null
    private var virtualDisplay: VirtualDisplay? = null
    private var codec: MediaCodec? = null
    private var inputSurface: Surface? = null

    val gopFilter = GopTailFilter(config.keepFrames)

    // Metrics & Diagnostics
    val encodedFrames = AtomicLong(0L)
    val keyframes = AtomicLong(0L)
    val transportAdmittedFrames = AtomicLong(0L)
    val transportDroppedFrames = AtomicLong(0L)
    val firstPtsUs = AtomicLong(-1L)
    val lastPtsUs = AtomicLong(0L)
    private val startedAtMs = AtomicLong(0L)

    var selectedCodecName: String = ""
        private set
    var isHardwareCodec: Boolean = false
        private set

    val isCapturing: Boolean
        get() = isRunning.get()

    /**
     * The measured state of this capture session, for session negotiation
     * (DEC-022). Frame rates and GOP length are taken from the encoder's own
     * counters rather than from the requested configuration, because DEC-020
     * records that this platform ignores the requested frame rate (it encoded
     * ~120 fps against a 30 fps request) — a configured value would be a
     * fiction.
     */
    fun liveCapture(): LiveCapture {
        val encoded = encodedFrames.get()
        val key = keyframes.get()
        val started = startedAtMs.get()
        val elapsedSec = if (started > 0L) {
            ((SystemClock.elapsedRealtime() - started).coerceAtLeast(1L)) / 1000.0
        } else {
            0.0
        }
        val encodedFps = if (elapsedSec > 0.0) encoded / elapsedSec else 0.0

        val gopEstimated = key < 1
        val gopAus = if (gopEstimated) {
            config.expectedGopAus
        } else {
            Math.round(encoded.toDouble() / key.toDouble()).toInt().coerceAtLeast(1)
        }

        return LiveCapture(
            width = config.width,
            height = config.height,
            bitrateKbps = config.bitrate / 1000,
            codec = if (config.mime == "video/avc") "h264" else config.mime,
            encodedFps = encodedFps,
            gopAus = gopAus,
            keepFrames = config.keepFrames,
            gopEstimated = gopEstimated,
        )
    }

    /**
     * Starts the capture pipeline using the provided consented [MediaProjection].
     * Must only be called AFTER consent is granted and the Foreground Service is active.
     */
    @Synchronized
    fun start(mediaProjection: MediaProjection): Boolean {
        if (isRunning.get()) {
            Log.w(TAG, "ScreenCaptureEngine is already running")
            return true
        }

        projection = mediaProjection

        val ht = HandlerThread("phonebridge-capture-cb").also { it.start() }
        handlerThread = ht
        val h = Handler(ht.looper)
        handler = h

        val cb = object : MediaProjection.Callback() {
            override fun onStop() {
                Log.i(TAG, "MediaProjection.Callback: onStop received from system")
                if (isRunning.get()) {
                    // Report the typed reason while the transport is still up;
                    // otherwise the peer sees a stream that simply stops and
                    // cannot tell a revocation from a network fault.
                    try {
                        listener?.onConsentRevoked("projection_stopped_by_system")
                    } catch (t: Throwable) {
                        Log.w(TAG, "onConsentRevoked listener threw: ${t.message}")
                    }
                }
                stop("projection_stopped_by_system")
            }
        }
        projectionCallback = cb
        mediaProjection.registerCallback(cb, h)

        val selection = CodecSelector.select(config)
        selectedCodecName = selection.name
        isHardwareCodec = selection.isHardware
        Log.i(TAG, "Selected encoder: ${selection.name} (hardware=${selection.isHardware}) reason: ${selection.reason}")

        if (selection.candidate == null) {
            Log.e(TAG, "No encoder available: ${selection.reason}")
            listener?.onCaptureError(IllegalStateException(selection.reason))
            cleanup()
            return false
        }

        try {
            val format = buildMediaFormat(selection.candidate)
            val c = MediaCodec.createByCodecName(selection.name)
            c.configure(format, null, null, MediaCodec.CONFIGURE_FLAG_ENCODE)
            val surface = c.createInputSurface()
            c.start()

            codec = c
            inputSurface = surface

            val vd = mediaProjection.createVirtualDisplay(
                "PhoneBridge-Display",
                config.width,
                config.height,
                config.dpi,
                DisplayManager.VIRTUAL_DISPLAY_FLAG_AUTO_MIRROR,
                surface,
                object : VirtualDisplay.Callback() {
                    override fun onPaused() { Log.d(TAG, "VirtualDisplay paused") }
                    override fun onResumed() { Log.d(TAG, "VirtualDisplay resumed") }
                    override fun onStopped() { Log.d(TAG, "VirtualDisplay stopped") }
                },
                h
            )
            virtualDisplay = vd

            isRunning.set(true)
            startedAtMs.set(SystemClock.elapsedRealtime())
            gopFilter.reset()
            encodedFrames.set(0L)
            keyframes.set(0L)
            transportAdmittedFrames.set(0L)
            transportDroppedFrames.set(0L)
            firstPtsUs.set(-1L)
            lastPtsUs.set(0L)

            val dt = Thread({ drainLoop() }, "phonebridge-codec-drain").also { it.start() }
            drainThread = dt

            Log.i(TAG, "ScreenCaptureEngine started: ${config.width}x${config.height} @ ${config.fps}fps")
            listener?.onCaptureStarted()
            return true
        } catch (t: Throwable) {
            Log.e(TAG, "Failed to start capture pipeline: ${t.message}", t)
            listener?.onCaptureError(t)
            cleanup()
            return false
        }
    }

    /**
     * Gracefully stops the capture pipeline and releases all resources.
     */
    @Synchronized
    fun stop(reason: String = "user_stopped") {
        if (!isRunning.compareAndSet(true, false)) {
            return
        }
        Log.i(TAG, "Stopping ScreenCaptureEngine (reason: $reason)...")

        cleanup()
        listener?.onCaptureStopped(reason)
        Log.i(TAG, "ScreenCaptureEngine stopped. Encoded: ${encodedFrames.get()}, Sent: ${transportAdmittedFrames.get()}, GOP-dropped: ${gopFilter.droppedFrames}")
    }

    private fun drainLoop() {
        val c = codec ?: return
        val bufferInfo = MediaCodec.BufferInfo()
        var pendingCsd: ByteArray? = null

        var lastStatsLogMs = SystemClock.elapsedRealtime()
        var lastLoggedEncoded = 0L
        var lastLoggedSent = 0L

        while (isRunning.get()) {
            val idx = try {
                c.dequeueOutputBuffer(bufferInfo, DEQUEUE_TIMEOUT_US)
            } catch (t: Throwable) {
                if (isRunning.get()) {
                    Log.e(TAG, "dequeueOutputBuffer exception", t)
                    listener?.onCaptureError(t)
                }
                break
            }

            if (idx == MediaCodec.INFO_OUTPUT_FORMAT_CHANGED) {
                val newFormat = c.outputFormat
                Log.i(TAG, "MediaCodec output format changed: $newFormat")
            } else if (idx >= 0) {
                try {
                    if (bufferInfo.size > 0) {
                        val isCsd = (bufferInfo.flags and MediaCodec.BUFFER_FLAG_CODEC_CONFIG) != 0
                        val isKey = (bufferInfo.flags and MediaCodec.BUFFER_FLAG_KEY_FRAME) != 0

                        val byteBuffer = c.getOutputBuffer(idx)
                        if (byteBuffer != null) {
                            byteBuffer.position(bufferInfo.offset)
                            byteBuffer.limit(bufferInfo.offset + bufferInfo.size)

                            val payload = ByteArray(bufferInfo.size)
                            byteBuffer.get(payload)

                            if (isCsd) {
                                // Cache CSD (SPS/PPS) to prepend to the first emitted AU
                                pendingCsd = payload
                                Log.d(TAG, "Captured CSD (${payload.size} bytes)")
                            } else {
                                val totalEnc = encodedFrames.incrementAndGet()
                                val ptsUs = bufferInfo.presentationTimeUs
                                if (firstPtsUs.get() < 0L) {
                                    firstPtsUs.set(ptsUs)
                                }
                                lastPtsUs.set(ptsUs)

                                if (isKey) {
                                    keyframes.incrementAndGet()
                                }

                                val hadCsd = pendingCsd != null
                                var au = payload
                                if (pendingCsd != null) {
                                    val combined = ByteArray(pendingCsd.size + payload.size)
                                    System.arraycopy(pendingCsd, 0, combined, 0, pendingCsd.size)
                                    System.arraycopy(payload, 0, combined, pendingCsd.size, payload.size)
                                    au = combined
                                    pendingCsd = null
                                    Log.d(TAG, "Prepended CSD to first AU (total: ${au.size} bytes)")
                                }

                                val isKeyframeOrCsd = isKey || hadCsd

                                // Apply prediction-safe GOP-tail selection
                                val shouldPush = gopFilter.shouldAdmit(isKeyframeOrCsd)
                                if (shouldPush) {
                                    // Hand off to Go transport via JNI (hot path: non-blocking)
                                    val admitted = GoBridge.mediaOnFrame(ptsUs, au, isKeyframeOrCsd)
                                    if (admitted) {
                                        transportAdmittedFrames.incrementAndGet()
                                    } else {
                                        transportDroppedFrames.incrementAndGet()
                                    }
                                }

                                val nowMs = SystemClock.elapsedRealtime()
                                if (nowMs - lastStatsLogMs >= 2000L) {
                                    val sec = (nowMs - lastStatsLogMs) / 1000.0
                                    val dEnc = totalEnc - lastLoggedEncoded
                                    val totalSent = transportAdmittedFrames.get()
                                    val dSent = totalSent - lastLoggedSent
                                    val encFps = dEnc / sec
                                    val sentFps = dSent / sec
                                    Log.i(
                                        TAG,
                                        String.format(
                                            Locale.US,
                                            "CAPTURE_STATS: encoded=%d (%.1f fps), delivered=%d (%.1f fps), gop_dropped=%d, keyframes=%d",
                                            totalEnc, encFps, totalSent, sentFps, gopFilter.droppedFrames, keyframes.get()
                                        )
                                    )
                                    lastStatsLogMs = nowMs
                                    lastLoggedEncoded = totalEnc
                                    lastLoggedSent = totalSent
                                }
                            }
                        }
                    }
                } finally {
                    try {
                        c.releaseOutputBuffer(idx, false)
                    } catch (t: Throwable) {
                        Log.w(TAG, "releaseOutputBuffer failed: ${t.message}")
                    }
                }
            }
        }
    }

    private fun buildMediaFormat(candidate: CodecSelector.CodecCandidate): MediaFormat {
        val f = MediaFormat.createVideoFormat(config.mime, config.width, config.height)
        f.setInteger(MediaFormat.KEY_COLOR_FORMAT, MediaCodecInfo.CodecCapabilities.COLOR_FormatSurface)
        f.setInteger(MediaFormat.KEY_BIT_RATE, config.bitrate)
        f.setInteger(MediaFormat.KEY_FRAME_RATE, config.keyFrameRate)
        f.setInteger(MediaFormat.KEY_I_FRAME_INTERVAL, config.keyIntervalSec)

        if (Build.VERSION.SDK_INT >= 23) {
            f.setInteger(MediaFormat.KEY_PRIORITY, 0) // Realtime priority
        }

        // Apply bitrate mode if supported
        val encoderCaps = candidate.caps?.encoderCapabilities
        if (encoderCaps != null) {
            if (encoderCaps.isBitrateModeSupported(config.bitrateMode)) {
                f.setInteger(MediaFormat.KEY_BITRATE_MODE, config.bitrateMode)
            } else if (encoderCaps.isBitrateModeSupported(MediaCodecInfo.EncoderCapabilities.BITRATE_MODE_VBR)) {
                f.setInteger(MediaFormat.KEY_BITRATE_MODE, MediaCodecInfo.EncoderCapabilities.BITRATE_MODE_VBR)
            }
        }

        return f
    }

    private fun cleanup() {
        try {
            drainThread?.join(500)
        } catch (_: Throwable) {}
        drainThread = null

        try {
            virtualDisplay?.release()
        } catch (t: Throwable) {
            Log.w(TAG, "virtualDisplay release: ${t.message}")
        }
        virtualDisplay = null

        try {
            inputSurface?.release()
        } catch (t: Throwable) {
            Log.w(TAG, "inputSurface release: ${t.message}")
        }
        inputSurface = null

        try {
            codec?.stop()
        } catch (t: Throwable) {
            Log.w(TAG, "codec stop: ${t.message}")
        }
        try {
            codec?.release()
        } catch (t: Throwable) {
            Log.w(TAG, "codec release: ${t.message}")
        }
        codec = null

        try {
            // Unregister BEFORE stopping: stopping the projection fires the
            // system callback, and a deliberate stop must not be reported to the
            // peer as a withdrawn consent (DEC-022). Only a stop we did not ask
            // for reaches onConsentRevoked.
            projectionCallback?.let { projection?.unregisterCallback(it) }
            projection?.stop()
        } catch (t: Throwable) {
            Log.w(TAG, "projection stop: ${t.message}")
        }
        projection = null
        projectionCallback = null

        handlerThread?.quitSafely()
        handlerThread = null
        handler = null
    }
}
