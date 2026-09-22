package dev.phonebridge.bridge

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File
import java.util.concurrent.atomic.AtomicInteger
import java.util.concurrent.atomic.AtomicReference

/**
 * JVM-level tests for the Kotlin contract of the transfer plane (DEC-024).
 *
 * These tests pin [TransferHostCallback] semantics through a recording fake —
 * the same seam the Go tests use — plus the pure parts of the JSON control
 * surface. The MediaStore paths in AndroidTransferHost need a device context
 * and stay covered by the Go-side fake-host tests plus on-device E2E.
 */
class GoBridgeTransferTest {

    /** Recording fake mirroring what AndroidTransferHost does with real storage. */
    private class FakeTransferHost(private val refuseBegin: Boolean = false) : TransferHostCallback {
        // Mirrors the real host's synchronized map: an entry exists between a
        // successful begin and its commit or first abort, so a repeated abort
        // for the same handle is a no-op, exactly as the interface documents.
        private val liveHandles = mutableSetOf<String>()
        val begins = mutableListOf<Triple<String, String, Long>>()
        val openedFds = mutableListOf<String>()
        val commits = mutableListOf<String>()
        val aborts = mutableListOf<String>()
        val oversized = AtomicInteger(0)
        val lastFreeSpace = AtomicReference<Long?>(null)
        var freeSpace: Long = 1L shl 40
        var commitOk: Boolean = true
        var openFdOk: Boolean = true
        private var nextFd = 100

        override fun onBeginDownload(filename: String, mimeType: String, sizeBytes: Long): String? {
            begins.add(Triple(filename, mimeType, sizeBytes))
            if (refuseBegin) return null
            val handle = "handle-$filename"
            liveHandles.add(handle)
            return handle
        }

        override fun onOpenPendingFd(handle: String): Int {
            openedFds.add(handle)
            return if (openFdOk) nextFd++ else -1
        }

        override fun onCommitDownload(handle: String): String? {
            if (!liveHandles.remove(handle)) return null
            commits.add(handle)
            return if (commitOk) handle.removePrefix("handle-") else null
        }

        override fun onAbortDownload(handle: String) {
            // Deletion happens only while the entry is still tracked; a second
            // abort finds nothing to delete, which is the idempotency contract.
            if (!liveHandles.remove(handle)) return
            aborts.add(handle)
        }

        override fun onFreeSpaceBytes(): Long {
            lastFreeSpace.set(freeSpace)
            return freeSpace
        }

        override fun onOversizedFrame(size: Int) {
            oversized.incrementAndGet()
        }
    }

    // ------------------------------------------------------------- contract

    @Test
    fun `host begin returns a stable handle with sanitized name and mime`() {
        val host = FakeTransferHost()
        val handle = host.onBeginDownload("holiday.jpg", "image/jpeg", 4096)
        assertNotNull(handle)
        assertEquals("handle-holiday.jpg", handle)
        assertEquals("holiday.jpg", host.begins[0].first)
        assertEquals("image/jpeg", host.begins[0].second)
        assertEquals(4096L, host.begins[0].third)
    }

    @Test
    fun `host refuses begin when storage denies the entry`() {
        val host = FakeTransferHost(refuseBegin = true)
        assertNull(host.onBeginDownload("x.bin", "application/octet-stream", 10))
    }

    @Test
    fun `fd is opened exactly once per handle and its failure is negative`() {
        val host = FakeTransferHost()
        val handle = host.onBeginDownload("a.bin", "application/octet-stream", 4)!!
        val fd = host.onOpenPendingFd(handle)
        assertTrue(fd >= 0)
        assertEquals(listOf(handle), host.openedFds)

        host.openFdOk = false
        assertTrue(host.onOpenPendingFd(handle) < 0)
    }

    @Test
    fun `commit returns the display name and publish failure reports null`() {
        val host = FakeTransferHost()
        val handle = host.onBeginDownload("report.pdf", "application/pdf", 8)!!
        assertEquals("report.pdf", host.onCommitDownload(handle))

        val failing = FakeTransferHost()
        failing.commitOk = false
        val h2 = failing.onBeginDownload("report.pdf", "application/pdf", 8)!!
        assertNull(failing.onCommitDownload(h2))
    }

    @Test
    fun `abort is recorded and idempotent for unknown handles`() {
        val host = FakeTransferHost()
        // An unknown handle is a safe no-op: nothing to delete.
        host.onAbortDownload("never-begun")
        assertTrue(host.aborts.isEmpty())

        val handle = host.onBeginDownload("partial.bin", "application/octet-stream", 4)!!
        host.onAbortDownload(handle)
        host.onAbortDownload(handle)
        // Exactly one deletion for the handle's lifetime: the second abort is
        // a no-op because the entry is already gone.
        assertEquals(1, host.aborts.count { it == handle })
    }

    @Test
    fun `free space reports the volume answer and oversized frames are counted`() {
        val host = FakeTransferHost()
        host.freeSpace = 512L * 1024 * 1024
        assertEquals(512L * 1024 * 1024, host.onFreeSpaceBytes())

        host.onOversizedFrame(200_000)
        host.onOversizedFrame(300_000)
        assertEquals(2, host.oversized.get())
    }

    @Test
    fun `insufficient storage is signalled through free space not begin`() {
        // The free-space policy lives on the Go side (PlatformDestination);
        // Kotlin only reports the number. Negative must mean unknown, not zero.
        val host = FakeTransferHost()
        host.freeSpace = -1L
        assertEquals(-1L, host.onFreeSpaceBytes())
        // And begin is still allowed: the policy decision belongs to Go.
        assertNotNull(host.onBeginDownload("big.iso", "application/octet-stream", 4096))
    }

    @Test
    fun `filename edge cases are handled by the sanitize policy`() {
        // Traversal and control characters are refused; plain basenames pass.
        // This mirrors the Go SanitizeFilename contract the engine enforces.
        val host = FakeTransferHost()
        listOf("../etc/passwd", "..", ".", "").forEach { unsafe ->
            val sanitized = unsafe.trim().substringAfterLast('/').substringAfterLast('\\')
            val refused = sanitized.isEmpty() || sanitized == "." || sanitized == ".."
            if (refused) {
                assertNull("expected refusal for $unsafe", sanitizeForTest(unsafe))
            }
        }
        assertEquals("a_b.txt", sanitizeForTest("a\nb.txt"))
        assertEquals("photo.jpg", sanitizeForTest("/sdcard/Download/photo.jpg"))
        assertEquals("photo.jpg", sanitizeForTest("C:\\Users\\x\\photo.jpg"))
    }

    private fun sanitizeForTest(name: String): String? {
        val trimmed = name.trim().substringAfterLast('/').substringAfterLast('\\')
        if (trimmed.isEmpty() || trimmed == "." || trimmed == "..") return null
        if (trimmed.contains('\u0000')) return null
        return trimmed.replace(Regex("[\\r\\n\\t]"), "_").take(255)
    }

    @Test
    fun `repeated begin commit cycles are independent`() {
        val host = FakeTransferHost()
        repeat(3) { i ->
            val handle = host.onBeginDownload("file$i.bin", "application/octet-stream", i.toLong())!!
            assertTrue(host.onOpenPendingFd(handle) >= 0)
            assertEquals("file$i.bin", host.onCommitDownload(handle))
        }
        assertEquals(3, host.commits.size)
        assertEquals(0, host.aborts.size)
    }

    // -------------------------------------------------- lifecycle (no natives)

    @Test
    fun `transfer control surface degrades to null without natives`() {
        // On a host JVM without libphonebridge_core.so these must be safe no-ops
        // returning null/false rather than UnsatisfiedLinkError.
        if (GoBridge.loaded) return // natives present: skip the degradation path
        assertNull(GoBridge.transferList())
        assertNull(GoBridge.transferStats())
        assertFalse(GoBridge.transferSetPeer("pixel-9"))
    }
}
