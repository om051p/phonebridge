package dev.phonebridge.clipboard

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Tests for the Quick Settings tile's cold-start wait policy.
 *
 * A tile tap can arrive while PhoneBridgeService is still starting, and the
 * clipboard adapter only accepts reads once the service has started it. The
 * wait is bounded on purpose: waiting forever would leave an invisible,
 * focus-holding activity on the user's screen, while not waiting at all would
 * silently drop the tap — the failure the tile fell into before.
 *
 * The activity itself needs an instrumented test; this covers the decision
 * that drives it.
 */
class ClipboardSyncStartupTest {

    @Test
    fun `the read waits while the adapter is still starting and budget remains`() {
        assertTrue(
            shouldWaitForAdapterStartup(
                state = AdapterState.STOPPED,
                stepsUsed = 0,
                maxSteps = 10
            )
        )
    }

    @Test
    fun `the wait ends when the budget is exhausted so the tap is answered`() {
        assertFalse(
            "an exhausted wait must fall through to the read, not drop the tap",
            shouldWaitForAdapterStartup(
                state = AdapterState.STOPPED,
                stepsUsed = 10,
                maxSteps = 10
            )
        )
    }

    @Test
    fun `no wait is needed once the adapter is running`() {
        assertFalse(
            shouldWaitForAdapterStartup(
                state = AdapterState.WRITE_ONLY_DORMANT,
                stepsUsed = 0,
                maxSteps = 10
            )
        )
        assertFalse(
            shouldWaitForAdapterStartup(
                state = AdapterState.AMBIENT_ACTIVE,
                stepsUsed = 0,
                maxSteps = 10
            )
        )
    }
}
