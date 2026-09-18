package dev.phonebridge.bridge

import androidx.test.platform.app.InstrumentationRegistry
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * On-device instrumented tests (Step 3 verification): run on real Android
 * via `./gradlew :app:connectedDebugAndroidTest` with the arm64-v8a
 * libphonebridge_core.so packaged from jniLibs. Prove the DEC-019 two-plane
 * contract on the actual platform: engine lifecycle, control plane, and
 * synthetic AUs crossing JNI into the production Go WebRTC transport.
 */
class GoBridgeMediaDeviceTest {

    private fun checkLoaded() {
        val context = InstrumentationRegistry.getInstrumentation().targetContext
        assertTrue("app context missing", context != null)
        assertTrue("libphonebridge_core.so failed to load from jniLibs", GoBridge.loaded)
    }

    @Test
    fun libraryLoadsAndEngineStarts() {
        checkLoaded()
        assertTrue(GoBridge.start(null))
        try {
            val pong = GoBridge.invoke("ping", "x".toByteArray())
            assertNotEquals(pong, null)
            assertEquals("pong:x", pong!!.decodeToString())
            val state = String(GoBridge.invoke("state")!!)
            assertEquals("1", state)
        } finally {
            GoBridge.stop()
        }
    }

    @Test
    fun mediaLifecycleAndFramesCrossJni() {
        checkLoaded()
        assertTrue(GoBridge.start(null))
        try {
            // Before init: documented no-op.
            assertFalse(GoBridge.mediaOnFrame(0, byteArrayOf(0, 0, 0, 1, 0x41, 1), false))

            GoBridge.mediaInit()
            val pAU = byteArrayOf(0, 0, 0, 1, 0x41, 1, 2, 3)
            assertTrue(GoBridge.mediaOnFrame(1000, pAU, false))
            val idrAU = byteArrayOf(0, 0, 0, 1, 0x65, 1, 2, 3)
            assertTrue(GoBridge.mediaOnFrame(33334, idrAU, true))

            val stats = String(GoBridge.mediaStats()!!)
            assertTrue("stats missing pushedAUs: $stats", stats.contains("\"pushedAUs\":2"))
            assertTrue("pcState: $stats", stats.contains("\"pcState\":\"new\""))

            val offer = String(GoBridge.mediaCreateOffer())
            assertTrue("offer type: $offer", offer.contains("\"type\":\"offer\""))
            assertTrue("offer SDP missing H264: $offer", offer.contains("H264"))

            // Backpressure on device: flood the queue; keyframe still admitted.
            var admitted = 0
            for (i in 0 until 300) {
                val au = byteArrayOf(0, 0, 0, 1, 0x41, i.toByte())
                if (GoBridge.mediaOnFrame(i * 33334L, au, false)) admitted++
            }
            assertTrue("admitted=$admitted", admitted in 200..256)
            assertTrue(GoBridge.mediaOnFrame(9999999L, byteArrayOf(0, 0, 0, 1, 0x65, 9), true))

            GoBridge.mediaStop()
            GoBridge.mediaRelease()
        } finally {
            GoBridge.mediaStop()
            GoBridge.mediaRelease()
            GoBridge.stop()
        }
    }
}
