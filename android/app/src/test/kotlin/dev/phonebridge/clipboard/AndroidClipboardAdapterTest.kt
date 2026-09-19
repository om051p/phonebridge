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
}
