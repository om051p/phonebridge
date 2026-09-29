package dev.phonebridge.clipboard

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Unit tests for AndroidClipboardAdapter state machine and constants (DEC-023).
 */
class AndroidClipboardAdapterTest {

    @Test
    fun `exact application payload ceiling is 768 KiB`() {
        assertEquals(786432, AndroidClipboardAdapter.MAX_PAYLOAD_SIZE)
    }

    @Test
    fun `initial state is STOPPED`() {
        assertEquals(AdapterState.STOPPED, AndroidClipboardAdapter.state)
    }

    @Test
    fun `oversized platform write is rejected without dispatch`() {
        val oversized = ByteArray(AndroidClipboardAdapter.MAX_PAYLOAD_SIZE + 1)
        var oversizedNotified = 0
        val listener: (Int) -> Unit = { oversizedNotified = it }

        AndroidClipboardAdapter.addOversizedListener(listener)
        try {
            val accepted = AndroidClipboardAdapter.onWritePlatformClipboard("text/plain", oversized)
            assertFalse(accepted)
            assertEquals(AndroidClipboardAdapter.MAX_PAYLOAD_SIZE + 1, oversizedNotified)
        } finally {
            AndroidClipboardAdapter.removeOversizedListener(listener)
        }
    }

    @Test
    fun `send clipboard update forwards to transport sender`() {
        var sentBytes: ByteArray? = null
        AndroidClipboardAdapter.transportSender = { payload ->
            sentBytes = payload
            true
        }

        try {
            val testPayload = byteArrayOf(1, 2, 3, 4)
            val result = AndroidClipboardAdapter.onSendClipboardUpdate(testPayload)
            assertTrue(result)
            assertEquals(testPayload.toList(), sentBytes?.toList())
        } finally {
            AndroidClipboardAdapter.transportSender = null
        }
    }

    @Test
    fun `send clipboard update fails when no transport is registered`() {
        // Reporting success with nothing to carry the update made a dropped clip
        // indistinguishable from a delivered one: the Go engine treated it as
        // sent and the item never became eligible for the reconnect sync.
        AndroidClipboardAdapter.setSyncEnabled(true)
        AndroidClipboardAdapter.transportSender = null
        try {
            assertFalse(AndroidClipboardAdapter.onSendClipboardUpdate(byteArrayOf(1, 2, 3)))
        } finally {
            AndroidClipboardAdapter.transportSender = null
            AndroidClipboardAdapter.setSyncEnabled(true)
        }
    }

    @Test
    fun `disabling sync refuses outbound updates even with a transport`() {
        AndroidClipboardAdapter.setSyncEnabled(false)
        AndroidClipboardAdapter.transportSender = { true }
        try {
            assertFalse(AndroidClipboardAdapter.onSendClipboardUpdate(byteArrayOf(9)))
            assertFalse(AndroidClipboardAdapter.enabled)
        } finally {
            AndroidClipboardAdapter.transportSender = null
            AndroidClipboardAdapter.setSyncEnabled(true)
        }
    }

    @Test
    fun `disabling sync refuses inbound platform writes`() {
        AndroidClipboardAdapter.setSyncEnabled(false)
        try {
            assertFalse(
                AndroidClipboardAdapter.onWritePlatformClipboard("text/plain", byteArrayOf(4))
            )
        } finally {
            AndroidClipboardAdapter.setSyncEnabled(true)
        }
    }

    @Test
    fun `adapter state transitions trigger registered listeners`() {
        val observedStates = mutableListOf<AdapterState>()
        val listener: (AdapterState) -> Unit = { observedStates.add(it) }

        AndroidClipboardAdapter.addStateListener(listener)
        try {
            assertTrue(observedStates.contains(AdapterState.STOPPED))
        } finally {
            AndroidClipboardAdapter.removeStateListener(listener)
        }
    }

    // ---------------------------------------------------------------------
    // IME lookup cache (I3 of the comms audit)
    //
    // checkImeSelected reads Settings.Secure, a binder round-trip, and the 1 Hz
    // stats tick used to make that call every second for a value that only
    // changes when the user changes keyboards. These tests pin the caching
    // policy: a fresh answer is reused, a stale one is re-read, and an explicit
    // invalidation (app returning to the foreground) wins over the TTL.
    // ---------------------------------------------------------------------

    private fun resetImeCache() {
        AndroidClipboardAdapter.invalidateImeCheck()
        AndroidClipboardAdapter.setImeSelected(false)
    }

    @Test
    fun `ime answer is not re-read within the cache ttl`() {
        resetImeCache()
        var reads = 0
        val reader = { reads++; true }

        assertTrue(AndroidClipboardAdapter.cachedImeAnswer(1_000L, reader))
        assertTrue(AndroidClipboardAdapter.cachedImeAnswer(1_500L, reader))
        assertTrue(AndroidClipboardAdapter.cachedImeAnswer(2_999L, reader))

        assertEquals("a 2s-old answer must be reused instead of re-querying Settings", 1, reads)
    }

    @Test
    fun `ime answer is re-read once it ages out`() {
        resetImeCache()
        var reads = 0
        val reader = { reads++; true }

        assertTrue(AndroidClipboardAdapter.cachedImeAnswer(1_000L, reader))
        assertTrue(
            "the TTL boundary itself must be re-read",
            AndroidClipboardAdapter.cachedImeAnswer(3_001L, reader)
        )
        assertEquals(2, reads)
    }

    @Test
    fun `invalidation forces the next read`() {
        resetImeCache()
        var reads = 0
        val reader = { reads++; true }

        assertTrue(AndroidClipboardAdapter.cachedImeAnswer(1_000L, reader))
        AndroidClipboardAdapter.invalidateImeCheck()
        assertTrue(AndroidClipboardAdapter.cachedImeAnswer(1_100L, reader))

        assertEquals("invalidateImeCheck must drop the cache even inside the TTL", 2, reads)
    }

    @Test
    fun `a fresh read result is what the next cached call returns`() {
        resetImeCache()
        assertFalse("first read", AndroidClipboardAdapter.cachedImeAnswer(1_000L) { false })
        // Inside the TTL the reader's answer must not be adopted: the cached
        // value wins, which is what proves no Settings query happened.
        assertFalse(
            "within the TTL the cached value must win over a would-be re-read",
            AndroidClipboardAdapter.cachedImeAnswer(1_100L) { true }
        )
        AndroidClipboardAdapter.invalidateImeCheck()
        assertTrue(
            "after invalidation the fresh read must be adopted",
            AndroidClipboardAdapter.cachedImeAnswer(1_200L) { true }
        )
        resetImeCache()
    }
}
