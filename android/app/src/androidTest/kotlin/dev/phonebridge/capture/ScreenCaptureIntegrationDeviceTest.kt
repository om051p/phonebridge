package dev.phonebridge.capture

import android.content.Context
import androidx.test.platform.app.InstrumentationRegistry
import dev.phonebridge.bridge.GoBridge
import dev.phonebridge.service.PhoneBridgeService
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * On-device integration tests for the screen capture service and JNI data plane.
 */
class ScreenCaptureIntegrationDeviceTest {

    private val context: Context
        get() = InstrumentationRegistry.getInstrumentation().targetContext

    @Test
    fun goBridgeAndJniMediaPlaneAreReadyOnDevice() {
        assertTrue("GoBridge native library must be loaded", GoBridge.loaded)
        assertTrue("GoBridge engine must start", GoBridge.start(null))

        try {
            GoBridge.mediaInit()

            // Non-key AU
            val pAu = byteArrayOf(0, 0, 0, 1, 0x41, 1, 2, 3)
            assertTrue("Non-key AU admitted after init", GoBridge.mediaOnFrame(1000L, pAu, false))

            // IDR AU
            val idrAu = byteArrayOf(0, 0, 0, 1, 0x65, 1, 2, 3)
            assertTrue("Keyframe AU admitted", GoBridge.mediaOnFrame(33334L, idrAu, true))

            val stats = GoBridge.mediaStats()
            assertNotNull("mediaStats must return JSON", stats)
            val statsStr = String(stats!!)
            assertTrue("stats must record pushedAUs: $statsStr", statsStr.contains("\"pushedAUs\":2"))

            GoBridge.mediaStop()
            GoBridge.mediaRelease()
        } finally {
            GoBridge.mediaStop()
            GoBridge.mediaRelease()
            GoBridge.stop()
        }
    }

    @Test
    fun gopTailFilterOperatesCorrectlyOnDeviceRuntime() {
        val filter = GopTailFilter(keepFrames = 8)
        assertTrue(filter.shouldAdmit(isKey = true))
        for (i in 1..7) {
            assertTrue(filter.shouldAdmit(isKey = false))
        }
        for (i in 8..29) {
            assertFalse(filter.shouldAdmit(isKey = false))
        }
        assertEquals(8L, filter.admittedFrames)
        assertEquals(22L, filter.droppedFrames)
    }

    @Test
    fun phoneBridgeServiceHandlesStartAndStopLifecycle() {
        PhoneBridgeService.startService(context)
        // Service starts and establishes background lifecycle
        PhoneBridgeService.stopService(context)
    }
}
