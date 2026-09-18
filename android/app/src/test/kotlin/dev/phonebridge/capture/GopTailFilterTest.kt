package dev.phonebridge.capture

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class GopTailFilterTest {

    @Test
    fun `idr frames are always admitted and reset sinceKey counter`() {
        val filter = GopTailFilter(keepFrames = 8)
        assertTrue(filter.shouldAdmit(isKey = true))
        assertEquals(0, filter.sinceKey)
        assertEquals(1L, filter.admittedFrames)
        assertEquals(0L, filter.droppedFrames)
        assertEquals(1L, filter.keyframes)
        assertEquals(1L, filter.totalFrames)
    }

    @Test
    fun `contiguous prefix of GOP is admitted and tail is dropped`() {
        val keep = 8
        val gopSize = 30
        val filter = GopTailFilter(keepFrames = keep)

        // Frame 0: IDR
        assertTrue("Frame 0 (IDR) must be admitted", filter.shouldAdmit(isKey = true))

        // Frames 1..7: P-frames within keep limit
        for (i in 1 until keep) {
            assertTrue("Frame $i (P-frame) must be admitted", filter.shouldAdmit(isKey = false))
            assertEquals(i, filter.sinceKey)
        }
        assertEquals(8L, filter.admittedFrames)
        assertEquals(0L, filter.droppedFrames)

        // Frames 8..29: GOP tail P-frames must be dropped
        for (i in keep until gopSize) {
            assertFalse("Frame $i (tail P-frame) must be dropped", filter.shouldAdmit(isKey = false))
            assertEquals(i, filter.sinceKey)
        }
        assertEquals(8L, filter.admittedFrames)
        assertEquals(22L, filter.droppedFrames)
        assertEquals(30L, filter.totalFrames)
    }

    @Test
    fun `multi GOP sequence maintains exact cadence and prediction safety`() {
        val keep = 8
        val gopSize = 30
        val gopCount = 5
        val filter = GopTailFilter(keepFrames = keep)

        for (g in 0 until gopCount) {
            assertTrue("GOP $g IDR must be admitted", filter.shouldAdmit(isKey = true))
            for (p in 1 until gopSize) {
                val shouldAdmit = filter.shouldAdmit(isKey = false)
                if (p < keep) {
                    assertTrue("GOP $g frame $p should be admitted", shouldAdmit)
                } else {
                    assertFalse("GOP $g frame $p should be dropped", shouldAdmit)
                }
            }
        }

        val expectedAdmitted = (keep * gopCount).toLong()
        val expectedDropped = ((gopSize - keep) * gopCount).toLong()
        val expectedTotal = (gopSize * gopCount).toLong()

        assertEquals(expectedAdmitted, filter.admittedFrames)
        assertEquals(expectedDropped, filter.droppedFrames)
        assertEquals(expectedTotal, filter.totalFrames)
        assertEquals(gopCount.toLong(), filter.keyframes)
    }

    @Test
    fun `reset clears all counters`() {
        val filter = GopTailFilter(keepFrames = 4)
        filter.shouldAdmit(isKey = true)
        filter.shouldAdmit(isKey = false)
        filter.shouldAdmit(isKey = false)
        filter.shouldAdmit(isKey = false)
        filter.shouldAdmit(isKey = false) // dropped

        assertEquals(4L, filter.admittedFrames)
        assertEquals(1L, filter.droppedFrames)

        filter.reset()
        assertEquals(0, filter.sinceKey)
        assertEquals(0L, filter.admittedFrames)
        assertEquals(0L, filter.droppedFrames)
        assertEquals(0L, filter.totalFrames)
        assertEquals(0L, filter.keyframes)
    }
}
