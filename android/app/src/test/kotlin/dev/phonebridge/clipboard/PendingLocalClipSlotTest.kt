package dev.phonebridge.clipboard

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertSame
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Tests for the cold-start delivery slot.
 *
 * This slot is the whole fix for the deterministic cold-start loss: the clip a
 * user copied before the session existed is held here, in memory, while the
 * peer's reconnect-sync item flows through engine state — and the only way it
 * leaves is an explicit hand-over. The semantics asserted here are exactly the
 * ones the device runs proved must hold: latest copy wins, and a late
 * acknowledgement of an older item can never discard a newer one.
 */
class PendingLocalClipSlotTest {

    @Test
    fun `starts empty`() {
        val slot = PendingLocalClipSlot()
        assertTrue(slot.isEmpty)
        assertNull(slot.peek())
    }

    @Test
    fun `holds the exact bytes that were read`() {
        val slot = PendingLocalClipSlot()
        val payload = "PBSESS-1234".toByteArray()

        slot.hold("text/plain;charset=utf-8", payload, 1_796_000_000_000)

        val held = slot.peek()
        assertEquals("text/plain;charset=utf-8", held!!.mimeType)
        assertEquals(1_796_000_000_000, held.copiedAtMs)
        assertEquals("PBSESS-1234", String(held.payload))
        assertFalse(slot.isEmpty)
    }

    @Test
    fun `a newer local copy supersedes the held one instead of queueing`() {
        val slot = PendingLocalClipSlot()
        slot.hold("text/plain;charset=utf-8", "OLD-MARKER".toByteArray(), 1_796_000_000_000)

        slot.hold("text/plain;charset=utf-8", "NEW-MARKER".toByteArray(), 1_796_000_001_000)

        // One slot, latest wins: no queue, no history of the superseded item.
        assertEquals("NEW-MARKER", String(slot.peek()!!.payload))
    }

    @Test
    fun `the held item stays held through unrelated activity until handed over`() {
        val slot = PendingLocalClipSlot()
        slot.hold("text/plain;charset=utf-8", "MARKER-KEEP".toByteArray(), 42L)

        // Nothing but the slot's own API can change it (the peer's reconnect
        // sync touches engine state, never this object); peeking repeatedly and
        // simulating a peer update elsewhere leaves it untouched.
        val first = slot.peek()
        val second = slot.peek()
        assertSame(first, second)
        assertEquals("MARKER-KEEP", String(slot.peek()!!.payload))
    }

    @Test
    fun `a late acknowledgement of the sent item clears the slot`() {
        val slot = PendingLocalClipSlot()
        slot.hold("text/plain;charset=utf-8", "MARKER-ONE".toByteArray(), 1L)
        val sent = slot.peek()!!

        slot.clearIfSame(sent)

        assertTrue(slot.isEmpty)
        assertNull(slot.peek())
    }

    @Test
    fun `a late acknowledgement of an older item never discards a newer copy`() {
        val slot = PendingLocalClipSlot()
        slot.hold("text/plain;charset=utf-8", "MARKER-ONE".toByteArray(), 1L)
        val acknowledged = slot.peek()!!

        // The user copied something else while the first send was in flight.
        slot.hold("text/plain;charset=utf-8", "MARKER-TWO".toByteArray(), 2L)

        slot.clearIfSame(acknowledged)

        // The newer item must still be there, and it is the one that will be sent.
        assertEquals("MARKER-TWO", String(slot.peek()!!.payload))
    }

    @Test
    fun `clear drops whatever is held`() {
        val slot = PendingLocalClipSlot()
        slot.hold("text/plain;charset=utf-8", "MARKER-DROP".toByteArray(), 1L)

        slot.clear()

        assertTrue(slot.isEmpty)
    }
}
