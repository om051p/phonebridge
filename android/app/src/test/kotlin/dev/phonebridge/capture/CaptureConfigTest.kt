package dev.phonebridge.capture

import org.junit.Assert.assertEquals
import org.junit.Test

class CaptureConfigTest {

    @Test
    fun `default values match DEC-020 and DEC-021 ratified parameters`() {
        val cfg = CaptureConfig()
        assertEquals(720, cfg.width)
        assertEquals(1600, cfg.height)
        assertEquals(30, cfg.fps)
        assertEquals(2_500_000, cfg.bitrate)
        assertEquals(1, cfg.keyIntervalSec)
        assertEquals(30, cfg.keyFrameRate)
        assertEquals(8, cfg.keepFrames)
        assertEquals(30, cfg.expectedGopAus)
        assertEquals("video/avc", cfg.mime)
    }

    @Test
    fun `expectedGopAus calculates correctly for custom GOPs`() {
        val cfg15 = CaptureConfig(keyFrameRate = 15, keyIntervalSec = 1)
        assertEquals(15, cfg15.expectedGopAus)

        val cfg60 = CaptureConfig(keyFrameRate = 30, keyIntervalSec = 2)
        assertEquals(60, cfg60.expectedGopAus)
    }
}
