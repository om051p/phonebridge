package dev.phonebridge.bridge

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Manual probe (run on the host JVM): pushes a CSD AU BEFORE MediaInit, then
 * inits a transport and inspects the stats JSON. Proves admission-side PSI
 * learning works in the SHIPPED c-shared library, not just in unit tests.
 * Run: ./gradlew testReleaseUnitTest --tests "*GoBridgePsiProbeTest*" -i
 */
class GoBridgePsiProbeTest {
    private val start = byteArrayOf(0x00.toByte(), 0x00.toByte(), 0x00.toByte(), 0x01.toByte())

    @Test fun psiLearnsBeforeInitInShippedLibrary() {
        val libLoaded = GoBridge.loaded
        assertTrue("host lib not loaded (build core/build/libphonebridge_core.so first)", libLoaded)
        GoBridge.start(System.getProperty("java.io.tmpdir"))

        val sps = byteArrayOf(0x67.toByte(), 0x64.toByte(), 0x00.toByte(), 0x20.toByte(), 0xAC.toByte(), 0xB4.toByte(), 0x05.toByte(), 0xA0.toByte())
        val pps = byteArrayOf(0x68.toByte(), 0xEE.toByte(), 0x06.toByte(), 0xF2.toByte(), 0xC0.toByte())
        // CSD AU before any mediaInit: frames are refused, PSI must be learned.
        // Annex-B framing with 4-byte start codes, exactly as the Kotlin
        // capture pipeline hands AUs to JNI.
        val au = start + sps + start + pps
        val admitted = GoBridge.mediaOnFrame(1L, au, true)
        println("PROBE admitted-before-init=$admitted")
        assertFalse("frames must not be admitted before mediaInit", admitted)

        GoBridge.mediaInit()
        GoBridge.mediaCreateOffer() // full transport so stats are meaningful
        val stats = String(GoBridge.mediaStats()!!, Charsets.UTF_8)
        println("PROBE stats=$stats")
        assertTrue("stats must parse", stats.contains("pcState"))
        assertTrue("PSI learned before init must survive: psiHaveSPSPP=true, got $stats",
            stats.contains("\"psiHaveSPSPP\":true"))
        assertTrue("cached SPS 8 B expected, got $stats", stats.contains("\"psiSPSBytes\":8"))
        assertTrue("cached PPS 5 B expected, got $stats", stats.contains("\"psiPPSBytes\":5"))

        // The decisive probe: push an IDR now that the transport was rebuilt
        // (mediaRelease+mediaInit like a real offer does) — the sender must
        // re-inject the learned SPS/PPS from BEFORE init.
        GoBridge.mediaStop()
        GoBridge.mediaRelease()
        GoBridge.mediaInit()
        val idr = start + byteArrayOf(0x65.toByte(), 0x88.toByte(), 0x80.toByte(), 0x11.toByte())
        val admittedIdr = GoBridge.mediaOnFrame(2L, idr, true)
        println("PROBE idr-admitted-after-rebuild=$admittedIdr")
        val stats2 = String(GoBridge.mediaStats()!!, Charsets.UTF_8)
        println("PROBE stats2=$stats2")
        assertTrue("bare IDR must be admitted (re-injection from pre-init cache)", stats2.contains("\"pushedAUs\":1"))
        GoBridge.mediaStop()
        GoBridge.mediaRelease()
    }
}
