package dev.phonebridge.ui

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Regression tests for the session-attempt lifecycle (hardware bug: a failed
 * start re-armed itself through the capture-state callback at ~300
 * iterations/sec until stopCapture cleared the pending URL).
 *
 * The contract, enforced by [SessionAttemptState] and wired into
 * MainActivity's capture-state listener:
 *  - one failed attempt produces exactly one failure (endpoint consumed);
 *  - a failure notification never re-arms the same dial;
 *  - stop/invalidate and duplicate/stale callbacks are harmless;
 *  - only an explicit new start can dial again.
 */
class SessionAttemptStateTest {

    @Test
    fun `failed attempt consumes its endpoint exactly once`() {
        val state = SessionAttemptState()
        val gen = state.beginAttempt("http://192.168.0.236:7804")

        assertTrue(state.consumeOnFailure(gen))
        assertNull(state.pendingUrl)
        // Duplicate failure callbacks are harmless no-ops.
        assertFalse(state.consumeOnFailure(gen))
        assertNull(state.pendingUrl)
    }

    @Test
    fun `failure callback cannot re-arm the dial`() {
        val state = SessionAttemptState()
        val gen = state.beginAttempt("http://192.168.0.236:7804")

        // First capture signal dials.
        assertEquals("http://192.168.0.236:7804" to gen, state.dialForCaptureStart(true))
        // The attempt fails: endpoint consumed.
        assertTrue(state.consumeOnFailure(gen))
        // Every later capture signal — the storm that looped forever —
        // dials nothing.
        repeat(1000) {
            assertNull(state.dialForCaptureStart(true))
            assertFalse(state.consumeOnFailure(gen))
        }
    }

    @Test
    fun `explicit start after failure dials normally`() {
        val state = SessionAttemptState()
        val failed = state.beginAttempt("http://192.168.0.236:7804")
        assertTrue(state.consumeOnFailure(failed))

        val retry = state.beginAttempt("http://192.168.0.236:7804")
        assertTrue(retry != failed)
        assertEquals("http://192.168.0.236:7804" to retry, state.dialForCaptureStart(true))
    }

    @Test
    fun `stop after failure is harmless`() {
        val state = SessionAttemptState()
        val gen = state.beginAttempt("http://192.168.0.236:7804")
        assertTrue(state.consumeOnFailure(gen))

        state.invalidate()
        assertNull(state.dialForCaptureStart(true))
        assertNull(state.pendingUrl)
    }

    @Test
    fun `stale failure from attempt A cannot clear attempt B`() {
        val state = SessionAttemptState()
        val genA = state.beginAttempt("http://192.168.0.236:7804")
        val genB = state.beginAttempt("http://192.168.0.236:7804")

        // A's late failure must not touch B's pending endpoint.
        assertFalse(state.consumeOnFailure(genA))
        assertEquals("http://192.168.0.236:7804" to genB, state.dialForCaptureStart(true))
        // B's own failure still consumes exactly once.
        assertTrue(state.consumeOnFailure(genB))
        assertNull(state.dialForCaptureStart(true))
    }

    @Test
    fun `stop racing a failure invalidates the attempt`() {
        val state = SessionAttemptState()
        val gen = state.beginAttempt("http://192.168.0.236:7804")
        state.invalidate()

        assertFalse(state.consumeOnFailure(gen))
        assertNull(state.dialForCaptureStart(true))
    }

    @Test
    fun `no endpoint means no dial even while capturing`() {
        val state = SessionAttemptState()
        state.beginAttempt(null)

        assertNull(state.dialForCaptureStart(true))
        assertNull(state.dialForCaptureStart(false))
    }

    @Test
    fun `not capturing means no dial`() {
        val state = SessionAttemptState()
        state.beginAttempt("http://192.168.0.236:7804")

        assertNull(state.dialForCaptureStart(false))
    }
}
