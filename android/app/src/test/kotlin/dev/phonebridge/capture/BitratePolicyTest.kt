package dev.phonebridge.capture

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class BitratePolicyTest {

    @Test
    fun `60 Hz display maintains base bitrate unmodified`() {
        val adapted = BitratePolicy.adaptBitrate(2_500_000, 60.0f)
        assertEquals(2_500_000, adapted)
    }

    @Test
    fun `fractional 60 Hz display (59_94 Hz) maintains base bitrate unmodified`() {
        val adapted = BitratePolicy.adaptBitrate(2_500_000, 59.94f)
        assertEquals(2_500_000, adapted)
    }

    @Test
    fun `90 Hz display adapts 2_5 Mbps to 4_0 Mbps`() {
        val adapted = BitratePolicy.adaptBitrate(2_500_000, 90.0f)
        // 2.5 * 1.5 = 3.75 -> stepped up to 4.0 Mbps
        assertEquals(4_000_000, adapted)
    }

    @Test
    fun `120 Hz display adapts 2_5 Mbps to 5_0 Mbps`() {
        val adapted = BitratePolicy.adaptBitrate(2_500_000, 120.0f)
        // 2.5 * 2.0 = 5.0 Mbps
        assertEquals(5_000_000, adapted)
    }

    @Test
    fun `custom higher bitrate is scaled and clamped at max ceiling`() {
        val adapted = BitratePolicy.adaptBitrate(5_000_000, 90.0f)
        // 5.0 * 1.5 = 7.5 -> capped at 6.0 Mbps
        assertEquals(BitratePolicy.MAX_ADAPTED_BITRATE, adapted)
    }

    @Test
    fun `Codec 2_0 encoder enables High Profile when supported`() {
        assertTrue(BitratePolicy.shouldEnableHighProfile("c2.qti.avc.encoder", true))
        assertTrue(BitratePolicy.shouldEnableHighProfile("c2.android.avc.encoder", true))
    }

    @Test
    fun `Codec 2_0 encoder refuses High Profile when unsupported`() {
        assertFalse(BitratePolicy.shouldEnableHighProfile("c2.qti.avc.encoder", false))
    }

    @Test
    fun `legacy OMX encoder refuses High Profile even if hardware claims support`() {
        assertFalse(BitratePolicy.shouldEnableHighProfile("OMX.qcom.video.encoder.avc", true))
        assertFalse(BitratePolicy.shouldEnableHighProfile("OMX.google.h264.encoder", true))
    }
}
