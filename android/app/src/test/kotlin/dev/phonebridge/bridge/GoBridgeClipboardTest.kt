package dev.phonebridge.bridge

import java.io.File
import java.security.MessageDigest
import java.util.concurrent.CopyOnWriteArrayList
import java.util.concurrent.atomic.AtomicInteger
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Assume.assumeTrue
import org.junit.Test

/**
 * JVM-level JNI tests for GoBridge clipboard subsystem (Phase 3 Step 4).
 * Exercises Kotlin -> JNI -> Go clipboard.Engine -> Kotlin Host Callback.
 */
class GoBridgeClipboardTest {

    private val libLoaded: Boolean by lazy {
        if (GoBridge.loaded) return@lazy true
        val searchDirs = mutableListOf<String>()
        System.getProperty("java.library.path")?.split(File.pathSeparator)?.let { searchDirs += it }
        var dir: File? = System.getProperty("user.dir")?.let { File(it) }
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
                    return@lazy GoBridge.loaded
                } catch (e: UnsatisfiedLinkError) {
                    // ignore
                }
            }
        }
        false
    }

    private fun requireEngine() {
        assumeTrue("host libphonebridge_core.so not found", libLoaded)
        assertTrue("GoBridge.start failed", GoBridge.start(null))
    }

    private class TestClipboardCallback : ClipboardHostCallback {
        val platformWrites = CopyOnWriteArrayList<Pair<String, ByteArray>>()
        val sentUpdates = CopyOnWriteArrayList<ByteArray>()
        val oversizedEvents = AtomicInteger(0)

        override fun onWritePlatformClipboard(mimeType: String, payload: ByteArray): Boolean {
            platformWrites.add(mimeType to payload)
            return true
        }

        override fun onSendClipboardUpdate(payload: ByteArray): Boolean {
            sentUpdates.add(payload)
            return true
        }

        override fun onOversizedPayload(size: Int) {
            oversizedEvents.incrementAndGet()
        }
    }

    @Test
    fun `clipboard initialization and lifecycle`() {
        requireEngine()
        try {
            val callback = TestClipboardCallback()
            assertTrue(GoBridge.clipboardInit(callback))

            val stats = GoBridge.clipboardStats()
            assertNotNull(stats)
            val statsJson = stats!!.decodeToString()
            assertTrue(statsJson.contains("\"initialized\":true"))
            assertTrue(statsJson.contains("\"role\":\"Mobile\""))

            GoBridge.clipboardStop()
        } finally {
            GoBridge.stop()
        }
    }

    @Test
    fun `local copy dispatches outbound update`() {
        requireEngine()
        try {
            val callback = TestClipboardCallback()
            assertTrue(GoBridge.clipboardInit(callback))

            val payload = "Hello from Android JNI test".toByteArray(Charsets.UTF_8)
            val nowMs = System.currentTimeMillis()

            assertTrue(GoBridge.clipboardOnLocalCopy("text/plain;charset=utf-8", payload, nowMs))
            assertEquals(1, callback.sentUpdates.size)
            assertTrue(callback.sentUpdates[0].isNotEmpty())

            val stats = GoBridge.clipboardStats()?.decodeToString() ?: ""
            assertTrue(stats.contains("\"local_copies\":1"))

            GoBridge.clipboardStop()
        } finally {
            GoBridge.stop()
        }
    }

    @Test
    fun `oversized payload strictly rejected at 768 KiB ceiling`() {
        requireEngine()
        try {
            val callback = TestClipboardCallback()
            assertTrue(GoBridge.clipboardInit(callback))

            val limit = 786432 // 768 KiB exactly

            // Allowed: exactly 768 KiB
            val maxAllowed = ByteArray(limit) { 'x'.code.toByte() }
            assertTrue(GoBridge.clipboardOnLocalCopy("text/plain", maxAllowed, System.currentTimeMillis()))
            assertEquals(1, callback.sentUpdates.size)
            assertEquals(0, callback.oversizedEvents.get())

            // Rejected: 768 KiB + 1 byte
            val oversized = ByteArray(limit + 1) { 'y'.code.toByte() }
            assertFalse(GoBridge.clipboardOnLocalCopy("text/plain", oversized, System.currentTimeMillis()))
            assertEquals(1, callback.oversizedEvents.get())

            GoBridge.clipboardStop()
        } finally {
            GoBridge.stop()
        }
    }

    private fun makeClipboardUpdate(mime: String, payload: ByteArray, copiedAtMs: Long): ByteArray {
        val digest = MessageDigest.getInstance("SHA-256").digest(payload)
        val mimeBytes = mime.toByteArray(Charsets.UTF_8)
        val out = java.io.ByteArrayOutputStream()

        // field 1: mime_type (string, wire type 2) -> tag = 0x0a
        out.write(0x0a)
        writeVarint(out, mimeBytes.size.toLong())
        out.write(mimeBytes)

        // field 2: payload (bytes, wire type 2) -> tag = 0x12
        out.write(0x12)
        writeVarint(out, payload.size.toLong())
        out.write(payload)

        // field 3: sha256_digest (bytes, wire type 2) -> tag = 0x1a
        out.write(0x1a)
        writeVarint(out, digest.size.toLong())
        out.write(digest)

        // field 4: copied_at_ms (uint64, wire type 0) -> tag = 0x20
        out.write(0x20)
        writeVarint(out, copiedAtMs)

        return out.toByteArray()
    }

    private fun writeVarint(out: java.io.ByteArrayOutputStream, v: Long) {
        var value = v
        while (true) {
            if ((value and 0x7FL.inv()) == 0L) {
                out.write(value.toInt())
                return
            } else {
                out.write(((value and 0x7F) or 0x80).toInt())
                value = value ushr 7
            }
        }
    }

    @Test
    fun `remote bytes trigger platform write and echo suppression`() {
        requireEngine()
        try {
            val callback = TestClipboardCallback()
            assertTrue(GoBridge.clipboardInit(callback))

            val text = "Echo suppression across JNI boundary"
            val textBytes = text.toByteArray(Charsets.UTF_8)
            val remoteWire = makeClipboardUpdate("text/plain;charset=utf-8", textBytes, 1000L)

            // 1. Feed remote update into Go engine
            assertTrue(GoBridge.clipboardOnRemoteBytes(remoteWire))
            assertEquals(1, callback.platformWrites.size)
            assertEquals("text/plain;charset=utf-8", callback.platformWrites[0].first)
            assertEquals(text, callback.platformWrites[0].second.decodeToString())

            // 2. Simulate platform change listener firing for the write we just applied
            // (Simulated echo callback with identical content)
            assertTrue(GoBridge.clipboardOnLocalCopy("text/plain;charset=utf-8", textBytes, 1050L))

            // Echo must be suppressed: NO new outbound updates sent across transport!
            assertEquals(0, callback.sentUpdates.size)

            GoBridge.clipboardStop()
        } finally {
            GoBridge.stop()
        }
    }
}
