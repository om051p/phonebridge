package dev.phonebridge.bridge

import java.io.File
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertTrue
import org.junit.Assume.assumeTrue
import org.junit.Test

/**
 * JVM-level JNI smoke tests (Step 3 verification). These run on the host JVM
 * (linux/x86_64) against the SAME c-shared sources that produce
 * libphonebridge_core.so for Android — the package is built with
 * `-tags jni -buildmode=c-shared` exactly like Spike 02's host-JVM harness.
 *
 * The library is resolved through `java.library.path` under its canonical
 * name `libphonebridge_core.so` (see android/app/build.gradle.kts
 * `unitTests` jvmArgs + `core/Makefile host-lib`), so GoBridge's own
 * `System.loadLibrary("phonebridge_core")` path is what's exercised.
 *
 * They prove the DEC-019 two-plane contract end to end:
 *   Kotlin/JVM -> JNI -> Go MediaTransport -> production Sender queue
 * including the dedicated data-plane entry point (frames never route through
 * invoke) and the documented lifecycle/backpressure semantics.
 *
 * Tests SKIP (not fail) when the host library isn't on java.library.path:
 * build it with `make -C core host-lib`.
 */
class GoBridgeMediaTest {

    private val libLoaded: Boolean by lazy {
        if (GoBridge.loaded) return@lazy true
        // Pre-load the host library by absolute path; GoBridge's (self-healing)
        // System.loadLibrary then resolves the already-registered soname.
        // Search order: java.library.path entries (set by Gradle), then a
        // walk-up from the JVM's CWD (varies between IDE/Gradle invocations).
        val searchDirs = mutableListOf<String>()
        System.getProperty("java.library.path")?.split(File.pathSeparator)?.let { searchDirs += it }
        var dir: File? = File(System.getProperty("user.dir"))
        repeat(6) {
            val d = dir ?: return@repeat
            searchDirs += File(d, "core/build").path
            dir = d.parentFile
        }
        for (d in searchDirs) {
            val f = File(d, "libphonebridge_core.so")
            if (f.isFile) {
                try {
                    System.load(f.absolutePath)
                    foundLibPath = f.absolutePath
                    return@lazy GoBridge.loaded
                } catch (e: UnsatisfiedLinkError) {
                    foundLibPath = "load failed: ${e.message}"
                }
            }
        }
        false
    }

    private var foundLibPath: String? = null

    private fun requireEngine() {
        assumeTrue(
            "host libphonebridge_core.so not found (make -C core host-lib); user.dir=" +
                System.getProperty("user.dir") + " searchedFrom=" + (foundLibPath ?: "n/a"),
            libLoaded
        )
        assertTrue("GoBridge.start failed", GoBridge.start(null))
    }

    @Test
    fun `engine starts and control plane works`() {
        requireEngine()
        try {
            val pong = GoBridge.invoke("ping", "x".toByteArray())
            assertNotEquals(null, pong)
            assertEquals("pong:x", pong!!.decodeToString())
            // Media lifecycle before init: safe no-ops.
            assertFalse(GoBridge.mediaOnFrame(0, byteArrayOf(0x41, 1), false))
            GoBridge.mediaStop()
        } finally {
            GoBridge.stop()
        }
    }

    @Test
    fun `media lifecycle and frames cross JNI into the production queue`() {
        requireEngine()
        try {
            // Frame before init: documented no-op (not admitted).
            assertFalse(GoBridge.mediaOnFrame(0, byteArrayOf(0, 0, 0, 1, 0x41, 1), false))

            GoBridge.mediaInit()

            // Frames queue after init, before start (never blocks).
            val pAU = byteArrayOf(0, 0, 0, 1, 0x41, 1, 2, 3)
            assertTrue(GoBridge.mediaOnFrame(1000, pAU, false))
            // Keyframe admitted even on an edge queue.
            val idrAU = byteArrayOf(0, 0, 0, 1, 0x65, 1, 2, 3)
            assertTrue(GoBridge.mediaOnFrame(33334, idrAU, true))
            // Empty AU rejected at the wrapper.
            assertFalse(GoBridge.mediaOnFrame(0, ByteArray(0), false))

            // Stats prove the AUs reached the production Sender through JNI.
            val stats = String(GoBridge.mediaStats()!!)
            assertTrue("stats missing pushedAUs: $stats", stats.contains("\"pushedAUs\":2"))
            // PeerConnection exists right after init (Pion state "new") and
            // nothing has been sent yet (writer not started).
            assertTrue("pcState: $stats", stats.contains("\"pcState\":\"new\""))
            assertTrue("sentAUs must be 0 pre-start: $stats", stats.contains("\"sentAUs\":0"))

            // Offer is real SDP with the H.264 track.
            val offer = GoBridge.mediaCreateOffer()
            val offerStr = String(offer)
            assertTrue("offer type: $offerStr", offerStr.contains("\"type\":\"offer\""))
            assertTrue("offer SDP missing H264: $offerStr", offerStr.contains("H264"))
            assertTrue("offer SDP missing track id: $offerStr", offerStr.contains("phonebridge-video"))

            // stop -> release -> re-init (fresh session after release).
            GoBridge.mediaStop()
            GoBridge.mediaRelease()
            GoBridge.mediaInit()
        } finally {
            GoBridge.mediaStop()
            GoBridge.mediaRelease()
            GoBridge.stop()
        }
    }

    @Test
    fun `backpressure - keyframe burst evicts and pushes keep succeeding`() {
        requireEngine()
        try {
            GoBridge.mediaInit()
            // Flood the bounded queue with non-key AUs; Push never blocks.
            var admitted = 0
            for (i in 0 until 300) {
                val au = byteArrayOf(0, 0, 0, 1, 0x41, i.toByte())
                if (GoBridge.mediaOnFrame(i * 33334L, au, false)) admitted++
            }
            // 256-slot queue: ~256 admitted, the rest dropped and counted.
            assertTrue("admitted=$admitted", admitted in 200..256)
            val stats = String(GoBridge.mediaStats()!!)
            assertTrue("droppedAUs missing: $stats", stats.contains("\"droppedAUs\":"))
            // Keyframe after the flood must be admitted (evicts oldest).
            val idr = byteArrayOf(0, 0, 0, 1, 0x65, 9)
            assertTrue(GoBridge.mediaOnFrame(9999999L, idr, true))
        } finally {
            GoBridge.mediaStop()
            GoBridge.mediaRelease()
            GoBridge.stop()
        }
    }

    @Test
    fun `invoke stays the control plane - unknown method throws`() {
        requireEngine()
        try {
            try {
                GoBridge.invoke("no-such-method")
                throw AssertionError("unknown method must throw IllegalStateException")
            } catch (expected: IllegalStateException) {
                // DEC-019 panic/error convention.
            }
        } finally {
            GoBridge.stop()
        }
    }
}
