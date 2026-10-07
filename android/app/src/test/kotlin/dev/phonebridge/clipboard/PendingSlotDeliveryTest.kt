package dev.phonebridge.clipboard

import dev.phonebridge.bridge.ClipboardHostCallback
import dev.phonebridge.bridge.GoBridge
import dev.phonebridge.signaling.DesktopSessionException
import dev.phonebridge.signaling.SessionRestorer
import java.io.File
import java.security.MessageDigest
import java.util.concurrent.CopyOnWriteArrayList
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Assume.assumeTrue
import org.junit.Test

/**
 * Focused cold-start delivery tests (TEST A/B/D/E/G of the handoff contract),
 * driving the REAL Go clipboard engine through JNI exactly the way
 * [AndroidClipboardAdapter] drives it: a locally read clip is held in a
 * [PendingLocalClipSlot] while the session comes up, the peer's reconnect-sync
 * item flows through engine state meanwhile, and the slot hands its bytes over
 * only once the transport accepted them.
 *
 * These are the semantics the 3-of-3 device repro demanded: the marker a user
 * copied before the transport existed must reach the peer exactly once, no
 * matter what the peer's own stale item, a busy session slot, a duplicate
 * channel-open, or a failed first send does in between.
 */
class PendingSlotDeliveryTest {

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

    /**
     * Mirrors [AndroidClipboardAdapter.flushPendingLocalClip]: hand the held
     * bytes to the engine, clear the slot only after the engine took them.
     */
    private fun flushViaEngine(slot: PendingLocalClipSlot): Boolean {
        val pending = slot.peek() ?: return true
        if (!GoBridge.loaded) return false
        val ok = GoBridge.clipboardOnLocalCopy(pending.mimeType, pending.payload, pending.copiedAtMs)
        if (ok) {
            slot.clearIfSame(pending)
        }
        return ok
    }

    /** Send success is controllable so TEST G can simulate a dead transport. */
    private class CountingCallback(var sendSucceeds: Boolean = true) : ClipboardHostCallback {
        val platformWrites = CopyOnWriteArrayList<Pair<String, ByteArray>>()
        val sentPayloads = CopyOnWriteArrayList<ByteArray>()

        override fun onWritePlatformClipboard(mimeType: String, payload: ByteArray): Boolean {
            platformWrites.add(mimeType to payload)
            return true
        }

        override fun onSendClipboardUpdate(payload: ByteArray): Boolean {
            if (!sendSucceeds) return false
            sentPayloads.add(payload)
            return true
        }

        override fun onOversizedPayload(size: Int) {}
        override fun onClipboardTransportOpen() {}
    }

    /** TEST A — the peer's reconnect-sync item must never reach the slot. */
    @Test
    fun `remote reconnect item reaches the platform but never touches the pending slot`() {
        requireEngine()
        val callback = CountingCallback()
        val slot = PendingLocalClipSlot()
        try {
            assertTrue(GoBridge.clipboardInit(callback))

            val marker = "PB-COLD-A-MARKER"
            slot.hold("text/plain;charset=utf-8", marker.toByteArray(), System.currentTimeMillis())

            // The desktop's stale reconnect item arrives over the transport.
            val remotePayload = "STALE-DESKTOP-ITEM".toByteArray()
            assertTrue(
                GoBridge.clipboardOnRemoteBytes(
                    makeClipboardUpdate("text/plain;charset=utf-8", remotePayload, 1_000L)
                )
            )

            // The remote item was applied to the platform…
            assertEquals(1, callback.platformWrites.size)
            assertEquals("STALE-DESKTOP-ITEM", callback.platformWrites[0].second.decodeToString())

            // …and the slot still holds the exact local item, byte for byte.
            assertEquals(marker, slot.peek()!!.payload.decodeToString())
            assertEquals(0, callback.sentPayloads.size)
        } finally {
            GoBridge.clipboardStop()
            GoBridge.stop()
        }
    }

    /** TEST B — once the transport is ready, the held local item goes out exactly once. */
    @Test
    fun `flush after the transport is ready sends the held local item once and clears the slot`() {
        requireEngine()
        val callback = CountingCallback()
        val slot = PendingLocalClipSlot()
        try {
            assertTrue(GoBridge.clipboardInit(callback))

            // Section 11 sequence: LOCAL A, then REMOTE B, then transport ready.
            val marker = "PB-COLD-B-MARKER"
            slot.hold("text/plain;charset=utf-8", marker.toByteArray(), System.currentTimeMillis())
            assertTrue(
                GoBridge.clipboardOnRemoteBytes(
                    makeClipboardUpdate("text/plain;charset=utf-8", "STALE-DESKTOP-ITEM".toByteArray(), 1_000L)
                )
            )

            // Transport ready: the flush carries the LOCAL item, not the peer's.
            assertTrue(flushViaEngine(slot))
            assertEquals(1, callback.sentPayloads.size)
            assertEquals(marker, payloadFromWire(callback.sentPayloads[0]).decodeToString())

            // Delivered means done: the slot is empty and a repeated flush
            // (second callback, 1 Hz tick) cannot submit anything again.
            assertTrue(slot.isEmpty)
            assertTrue(flushViaEngine(slot))
            assertEquals("a delivered item must never be re-sent", 1, callback.sentPayloads.size)
        } finally {
            GoBridge.clipboardStop()
            GoBridge.stop()
        }
    }

    /** TEST D — a SESSION_BUSY release and re-offer leave the pending item intact. */
    @Test
    fun `stale session release and re-offer leave the pending item intact for a single submission`() {
        requireEngine()
        val callback = CountingCallback()
        val slot = PendingLocalClipSlot()
        try {
            assertTrue(GoBridge.clipboardInit(callback))

            val marker = "PB-COLD-D-MARKER"
            slot.hold("text/plain;charset=utf-8", marker.toByteArray(), System.currentTimeMillis())

            // The desktop still holds the dead session's slot: the first offer
            // is refused SESSION_BUSY, the restorer releases it once and
            // re-offers exactly once.
            var offers = 0
            var stops = 0
            val restorer = SessionRestorer(
                endpoint = "http://10.0.0.2:8080",
                transportReady = { offers >= 2 },
                createTransportOffer = { "offer-$offers" },
                postOffer = { _, _ ->
                    offers++
                    if (offers == 1) {
                        throw DesktopSessionException("SESSION_BUSY", 409, "{}", "The desktop is already in a session")
                    }
                    "answer-sdp"
                },
                postStop = { stops++ },
                applyAnswer = { },
            )
            val outcome = restorer.restore()
            assertTrue("restore must succeed after one release", outcome is SessionRestorer.Outcome.Restored)
            assertEquals("the stale slot is released exactly once", 1, stops)
            assertEquals("the offer is made exactly twice, never a blind retry", 2, offers)

            // The whole signaling cycle left the pending item untouched.
            assertEquals(marker, slot.peek()!!.payload.decodeToString())

            // Transport is up: one submission, the local item, slot cleared.
            assertTrue(flushViaEngine(slot))
            assertEquals(1, callback.sentPayloads.size)
            assertEquals(marker, payloadFromWire(callback.sentPayloads[0]).decodeToString())
            assertTrue(slot.isEmpty)
        } finally {
            GoBridge.clipboardStop()
            GoBridge.stop()
        }
    }

    /** TEST E — the newest local read supersedes the held one; only it is submitted. */
    @Test
    fun `latest local read wins and only the newest item is ever submitted`() {
        requireEngine()
        val callback = CountingCallback()
        val slot = PendingLocalClipSlot()
        try {
            assertTrue(GoBridge.clipboardInit(callback))

            slot.hold("text/plain;charset=utf-8", "PB-COLD-E-FIRST".toByteArray(), 1_000L)
            slot.hold("text/plain;charset=utf-8", "PB-COLD-E-SECOND".toByteArray(), 2_000L)

            assertTrue(flushViaEngine(slot))
            assertEquals("exactly one item goes over the wire", 1, callback.sentPayloads.size)
            assertEquals(
                "the submitted item is the latest local read",
                "PB-COLD-E-SECOND",
                payloadFromWire(callback.sentPayloads[0]).decodeToString(),
            )
            assertTrue(slot.isEmpty)
        } finally {
            GoBridge.clipboardStop()
            GoBridge.stop()
        }
    }

    /** TEST G — a failed send keeps the pending item; the engine owns redelivery. */
    @Test
    fun `a failed send keeps the pending item and the engine stays owner of redelivery`() {
        requireEngine()
        val callback = CountingCallback(sendSucceeds = false)
        val slot = PendingLocalClipSlot()
        try {
            assertTrue(GoBridge.clipboardInit(callback))

            val marker = "PB-COLD-G-MARKER"
            val markerBytes = marker.toByteArray()

            // Transport accepts nothing: the submission must fail and report it.
            assertFalse(GoBridge.clipboardOnLocalCopy("text/plain;charset=utf-8", markerBytes, 42L))
            assertEquals("nothing reached the wire", 0, callback.sentPayloads.size)

            // The adapter protocol keeps holding after a failed hand-over.
            slot.hold("text/plain;charset=utf-8", markerBytes, 42L)

            // The engine already owns the item even though nothing was sent:
            // it records state before the transport call, so the digest is in
            // its current item the moment a submission reached it at all.
            val statsAfterFailure = GoBridge.clipboardStats()?.decodeToString() ?: ""
            assertTrue(
                "a submission that reached the engine must leave the item in engine state",
                statsAfterFailure.contains("\"current_digest\":\"${sha256Hex(markerBytes)}\""),
            )

            // The next flush is the identical-digest hand-over: true, with
            // nothing on the wire, and the slot clears because engine state —
            // carried by the channel-open resync (pinned at engine level by
            // TestE2E_DuplicateChannelOpen_PeerAppliesOnce) — now owns
            // redelivery. No inbound peer item can arrive before that channel
            // opens, so no reachable path loses the item in between.
            assertTrue(flushViaEngine(slot))
            assertEquals("the no-op retry must not put anything on the wire", 0, callback.sentPayloads.size)
            assertTrue(slot.isEmpty)
            val stats = GoBridge.clipboardStats()?.decodeToString() ?: ""
            assertTrue(
                "the engine must still own the item for the channel-open resync",
                stats.contains("\"current_digest\":\"${sha256Hex(markerBytes)}\""),
            )
        } finally {
            GoBridge.clipboardStop()
            GoBridge.stop()
        }
    }

    // ------------------------------------------------------------------
    // Helpers: ClipboardUpdate wire format (same encoding GoBridgeClipboardTest uses).
    // ------------------------------------------------------------------

    private fun makeClipboardUpdate(mime: String, payload: ByteArray, copiedAtMs: Long): ByteArray {
        val digest = MessageDigest.getInstance("SHA-256").digest(payload)
        val mimeBytes = mime.toByteArray(Charsets.UTF_8)
        val out = java.io.ByteArrayOutputStream()

        out.write(0x0a) // field 1: mime_type, wire type 2
        writeVarint(out, mimeBytes.size.toLong())
        out.write(mimeBytes)

        out.write(0x12) // field 2: payload, wire type 2
        writeVarint(out, payload.size.toLong())
        out.write(payload)

        out.write(0x1a) // field 3: sha256_digest, wire type 2
        writeVarint(out, digest.size.toLong())
        out.write(digest)

        out.write(0x20) // field 4: copied_at_ms, wire type 0
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

    /**
     * Extracts the payload (field 2) from a marshaled ClipboardUpdate, so the
     * assertions can prove WHICH item went over the wire — the whole point of
     * the cold-start contract. ClipboardUpdate only has scalar/bytes fields in
     * 1..4, so a minimal walk is exact here.
     */
    private fun payloadFromWire(wire: ByteArray): ByteArray {
        var i = 0
        while (i < wire.size) {
            val key = wire[i].toInt() and 0xFF
            i++
            val field = key ushr 3
            val wireType = key and 0x07
            when (wireType) {
                0 -> {
                    while ((wire[i].toInt() and 0x80) != 0) i++
                    i++
                }
                2 -> {
                    var len = 0
                    var shift = 0
                    while (true) {
                        val b = wire[i].toInt()
                        i++
                        len = len or ((b and 0x7F) shl shift)
                        shift += 7
                        if ((b and 0x80) == 0) break
                    }
                    if (field == 2) return wire.copyOfRange(i, i + len)
                    i += len
                }
                else -> error("unexpected wire type $wireType at byte $i")
            }
        }
        error("payload field not found in wire bytes")
    }

    private fun sha256Hex(bytes: ByteArray): String =
        MessageDigest.getInstance("SHA-256").digest(bytes).joinToString("") { "%02x".format(it) }
}
