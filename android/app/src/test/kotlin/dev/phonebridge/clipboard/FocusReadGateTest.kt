package dev.phonebridge.clipboard

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Tests for the automatic focus-triggered read gate (the no-keyboard clipboard
 * trigger).
 *
 * Why this policy exists: from Android 10 on, an app may read the clipboard
 * only while it owns the focused window, so the app reads on every focus gain —
 * and focus gain flaps constantly (dialogs, pickers, returning from another
 * app). The gate keeps that honest: at most one read per window, and exactly
 * one read even when several triggers race.
 *
 * The gate is instantiated fresh per test on purpose: the production adapter
 * owns a single process-wide instance, so per-instance state is what makes the
 * policy testable without touching the singleton.
 */
class FocusReadGateTest {

    private val oneSecond = 1_000L

    @Test
    fun `the first focus read is always due`() {
        val gate = FocusReadGate(oneSecond)
        var reads = 0

        val forwarded = gate.run(1_000L) { reads++; true }

        assertTrue("a gate that has never read must allow the first read", forwarded)
        assertEquals(1, reads)
    }

    @Test
    fun `a second focus read inside the window is skipped`() {
        val gate = FocusReadGate(oneSecond)
        var reads = 0
        gate.run(1_000L) { reads++; true }

        val forwarded = gate.run(1_999L) { reads++; true }

        assertFalse("a trigger 999 ms later must not read again", forwarded)
        assertEquals(1, reads)
    }

    @Test
    fun `a focus read at the window boundary is due again`() {
        val gate = FocusReadGate(oneSecond)
        var reads = 0
        gate.run(1_000L) { reads++; true }

        val forwarded = gate.run(2_000L) { reads++; true }

        assertTrue("a trigger exactly one interval later must read again", forwarded)
        assertEquals(2, reads)
    }

    @Test
    fun `a focus read that finds nothing still consumes the window`() {
        // Nothing copied yet is the common case when the app comes to the
        // foreground; re-running the read on every flap would cost a binder
        // round-trip and a Go engine update for nothing.
        val gate = FocusReadGate(oneSecond)
        var reads = 0

        assertFalse(gate.run(5_000L) { reads++; false })
        assertFalse(gate.run(5_200L) { reads++; false })

        assertEquals("the read must run once per window even when it yields nothing", 1, reads)
    }

    @Test
    fun `two triggers at the same instant collapse to one read`() {
        // Focus can arrive from more than one path for the same foreground
        // moment (the Flutter activity plus the one-shot sync surface), so the
        // second trigger must find the window already claimed. The atomic claim
        // itself is not reproducible in a single-threaded test; this pins the
        // observable contract.
        val gate = FocusReadGate(oneSecond)
        var reads = 0

        val claimed = gate.run(7_000L) { reads++; true }
        val second = gate.run(7_000L) { reads++; true }

        assertTrue(claimed)
        assertFalse("the same window must not be claimed twice", second)
        assertEquals(1, reads)
    }
}
