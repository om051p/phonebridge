package dev.phonebridge.capture

import android.media.MediaCodec
import android.media.MediaCodecInfo
import android.media.MediaFormat
import android.os.Build
import androidx.test.platform.app.InstrumentationRegistry
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * On-device hardware codec verification (Step 4 validation).
 *
 * Runs on real hardware (SM7475 / marblein) to verify:
 *  1. Hardware H.264 video encoder is present and discovered.
 *  2. Hardware encoder supports COLOR_FormatSurface.
 *  3. Target resolution (720x1600 @ 30fps) is supported.
 *  4. Hardware encoder starts and creates an input Surface successfully.
 */
class CodecCapabilitiesDeviceTest {

    @Test
    fun hardwareH264EncoderIsDiscoveredAndSupportsSurface() {
        val candidates = CodecSelector.enumerate("video/avc")
        assertTrue("At least one H.264 encoder must be present", candidates.isNotEmpty())

        val selection = CodecSelector.select(CaptureConfig(width = 720, height = 1600, fps = 30))
        assertNotNull("CodecSelector must select a candidate", selection.candidate)
        assertTrue(
            "Selected encoder must be hardware-accelerated on target device (selected: ${selection.name}, reason: ${selection.reason})",
            selection.isHardware
        )
        assertTrue(
            "Selected encoder must support Surface input (COLOR_FormatSurface)",
            selection.candidate!!.supportsSurface
        )
    }

    @Test
    fun hardwareCodecCanBeConfiguredAndCreateInputSurface() {
        val cfg = CaptureConfig(width = 720, height = 1600, fps = 30, bitrate = 2_500_000)
        val selection = CodecSelector.select(cfg)
        val candidate = selection.candidate
        assertNotNull("Selection candidate required", candidate)

        val format = MediaFormat.createVideoFormat(cfg.mime, cfg.width, cfg.height).apply {
            setInteger(MediaFormat.KEY_COLOR_FORMAT, MediaCodecInfo.CodecCapabilities.COLOR_FormatSurface)
            setInteger(MediaFormat.KEY_BIT_RATE, cfg.bitrate)
            setInteger(MediaFormat.KEY_FRAME_RATE, cfg.keyFrameRate)
            setInteger(MediaFormat.KEY_I_FRAME_INTERVAL, cfg.keyIntervalSec)
            if (Build.VERSION.SDK_INT >= 23) {
                setInteger(MediaFormat.KEY_PRIORITY, 0)
            }
        }

        val codec = MediaCodec.createByCodecName(selection.name)
        try {
            codec.configure(format, null, null, MediaCodec.CONFIGURE_FLAG_ENCODE)
            val surface = codec.createInputSurface()
            assertNotNull("createInputSurface must return a valid surface", surface)
            assertTrue("surface must be valid", surface.isValid)

            codec.start()
            // Codec started successfully with input surface
            surface.release()
        } finally {
            try {
                codec.stop()
            } catch (_: Throwable) {}
            try {
                codec.release()
            } catch (_: Throwable) {}
        }
    }
}
