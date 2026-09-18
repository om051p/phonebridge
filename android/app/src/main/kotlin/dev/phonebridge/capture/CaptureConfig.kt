package dev.phonebridge.capture

import android.content.Intent
import android.media.MediaCodecInfo
import android.os.Bundle

/**
 * CaptureConfig encapsulates the video capture, encoding, and GOP throttle parameters.
 *
 * Defaults match the validated DEC-020 and DEC-021 operating point on SM7475:
 *  - 720x1600 resolution (scaled or native)
 *  - 2.5 Mbps CBR bitrate
 *  - GOP 30: KEY_FRAME_RATE=30, KEY_I_FRAME_INTERVAL=1 sec
 *  - GOP-tail keep: 8 contiguous AUs per GOP (~32 delivered AUs/sec, prediction-safe)
 */
data class CaptureConfig(
    val width: Int = DEFAULT_WIDTH,
    val height: Int = DEFAULT_HEIGHT,
    val fps: Int = DEFAULT_FPS,
    val bitrate: Int = DEFAULT_BITRATE,
    val keyIntervalSec: Int = DEFAULT_KEY_INTERVAL_SEC,
    val keyFrameRate: Int = DEFAULT_KEY_FRAME_RATE,
    val keepFrames: Int = DEFAULT_KEEP_FRAMES,
    val bitrateMode: Int = DEFAULT_BITRATE_MODE,
    val dpi: Int = DEFAULT_DPI,
    val mime: String = DEFAULT_MIME,
) {
    val expectedGopAus: Int
        get() = keyFrameRate * keyIntervalSec

    fun toBundle(): Bundle = Bundle().apply {
        putInt(EXTRA_WIDTH, width)
        putInt(EXTRA_HEIGHT, height)
        putInt(EXTRA_FPS, fps)
        putInt(EXTRA_BITRATE, bitrate)
        putInt(EXTRA_KEY_INTERVAL_SEC, keyIntervalSec)
        putInt(EXTRA_KEY_FRAME_RATE, keyFrameRate)
        putInt(EXTRA_KEEP_FRAMES, keepFrames)
        putInt(EXTRA_BITRATE_MODE, bitrateMode)
        putInt(EXTRA_DPI, dpi)
        putString(EXTRA_MIME, mime)
    }

    companion object {
        const val DEFAULT_WIDTH = 720
        const val DEFAULT_HEIGHT = 1600
        const val DEFAULT_FPS = 30
        const val DEFAULT_BITRATE = 2_500_000 // 2.5 Mbps
        const val DEFAULT_KEY_INTERVAL_SEC = 1
        const val DEFAULT_KEY_FRAME_RATE = 30 // KEY_FRAME_RATE * KEY_I_FRAME_INTERVAL = 30 AUs per GOP
        const val DEFAULT_KEEP_FRAMES = 8    // Keep first 8 frames per GOP -> ~32 delivered AUs/sec
        const val DEFAULT_BITRATE_MODE = MediaCodecInfo.EncoderCapabilities.BITRATE_MODE_CBR
        const val DEFAULT_DPI = 320
        const val DEFAULT_MIME = "video/avc"

        const val EXTRA_WIDTH = "dev.phonebridge.capture.WIDTH"
        const val EXTRA_HEIGHT = "dev.phonebridge.capture.HEIGHT"
        const val EXTRA_FPS = "dev.phonebridge.capture.FPS"
        const val EXTRA_BITRATE = "dev.phonebridge.capture.BITRATE"
        const val EXTRA_KEY_INTERVAL_SEC = "dev.phonebridge.capture.KEY_INTERVAL_SEC"
        const val EXTRA_KEY_FRAME_RATE = "dev.phonebridge.capture.KEY_FRAME_RATE"
        const val EXTRA_KEEP_FRAMES = "dev.phonebridge.capture.KEEP_FRAMES"
        const val EXTRA_BITRATE_MODE = "dev.phonebridge.capture.BITRATE_MODE"
        const val EXTRA_DPI = "dev.phonebridge.capture.DPI"
        const val EXTRA_MIME = "dev.phonebridge.capture.MIME"

        fun fromIntent(intent: Intent?): CaptureConfig {
            if (intent == null) return CaptureConfig()
            return CaptureConfig(
                width = intent.getIntExtra(EXTRA_WIDTH, DEFAULT_WIDTH),
                height = intent.getIntExtra(EXTRA_HEIGHT, DEFAULT_HEIGHT),
                fps = intent.getIntExtra(EXTRA_FPS, DEFAULT_FPS),
                bitrate = intent.getIntExtra(EXTRA_BITRATE, DEFAULT_BITRATE),
                keyIntervalSec = intent.getIntExtra(EXTRA_KEY_INTERVAL_SEC, DEFAULT_KEY_INTERVAL_SEC),
                keyFrameRate = intent.getIntExtra(EXTRA_KEY_FRAME_RATE, DEFAULT_KEY_FRAME_RATE),
                keepFrames = intent.getIntExtra(EXTRA_KEEP_FRAMES, DEFAULT_KEEP_FRAMES),
                bitrateMode = intent.getIntExtra(EXTRA_BITRATE_MODE, DEFAULT_BITRATE_MODE),
                dpi = intent.getIntExtra(EXTRA_DPI, DEFAULT_DPI),
                mime = intent.getStringExtra(EXTRA_MIME) ?: DEFAULT_MIME,
            )
        }
    }
}
