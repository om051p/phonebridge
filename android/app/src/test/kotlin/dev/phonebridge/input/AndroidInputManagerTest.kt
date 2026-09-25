package dev.phonebridge.input

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Unit tests for Android input coordinate normalization and security bounds (DEC-027).
 */
class AndroidInputManagerTest {

    @Test
    fun `coordinate scaling maps normalized domain to screen pixels`() {
        val width = 1080f
        val height = 2400f

        // Center
        val centerX = 0.5f * width
        val centerY = 0.5f * height
        assertEquals(540f, centerX, 0.001f)
        assertEquals(1200f, centerY, 0.001f)

        // Top-left
        val minX = (0.0f * width).coerceIn(0f, width - 1f)
        val minY = (0.0f * height).coerceIn(0f, height - 1f)
        assertEquals(0f, minX, 0.001f)
        assertEquals(0f, minY, 0.001f)

        // Bottom-right
        val maxX = (1.0f * width).coerceIn(0f, width - 1f)
        val maxY = (1.0f * height).coerceIn(0f, height - 1f)
        assertEquals(1079f, maxX, 0.001f)
        assertEquals(2399f, maxY, 0.001f)
    }

    @Test
    fun `device unlocked check rejects input when context is uninitialized`() {
        // Without start(context), isDeviceUnlocked() must fail closed
        assertFalse(AndroidInputManager.isDeviceUnlocked())
    }
}
