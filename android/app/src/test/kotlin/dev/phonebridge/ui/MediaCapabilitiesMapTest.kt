package dev.phonebridge.ui

import dev.phonebridge.signaling.DeviceMediaCapabilities
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Tests for the screen-capability channel projection.
 *
 * The map must carry the encoder-selected advertisement verbatim: zero bounds
 * pass through as 0 ("no stated limit"), because the "supported or not"
 * judgement belongs to the UI's capability model, not to this projection.
 * An empty map is never produced here — the channel handler answers with an
 * empty map only when reading the capabilities threw, which Flutter parses
 * as unknown ("Checking…").
 */
class MediaCapabilitiesMapTest {

    @Test
    fun `advertisement projects verbatim onto the channel map`() {
        val caps = DeviceMediaCapabilities(
            codecs = listOf("h264"),
            maxWidth = 1080,
            maxHeight = 2400,
            maxFps = 30,
            supportsScreen = true,
        )

        val map = mediaCapabilitiesMap(caps)

        assertEquals(listOf("h264"), map["codecs"])
        assertEquals(1080, map["maxWidth"])
        assertEquals(2400, map["maxHeight"])
        assertEquals(30, map["maxFps"])
        assertEquals(true, map["supportsScreen"])
    }

    @Test
    fun `zero bounds pass through as zero, never as unsupported`() {
        val caps = DeviceMediaCapabilities(
            codecs = listOf("h264"),
            maxWidth = 0,
            maxHeight = 0,
            maxFps = 0,
            supportsScreen = true,
        )

        val map = mediaCapabilitiesMap(caps)

        assertEquals(0, map["maxWidth"])
        assertEquals(0, map["maxHeight"])
        assertEquals(0, map["maxFps"])
        assertEquals(true, map["supportsScreen"])
    }

    @Test
    fun `no screen support is reported honestly`() {
        val caps = DeviceMediaCapabilities(
            codecs = emptyList(),
            supportsScreen = false,
        )

        val map = mediaCapabilitiesMap(caps)

        assertFalse(map["supportsScreen"] as Boolean)
        assertTrue((map["codecs"] as List<*>).isEmpty())
    }
}
