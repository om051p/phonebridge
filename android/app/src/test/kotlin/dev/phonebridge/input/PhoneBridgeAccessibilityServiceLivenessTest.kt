package dev.phonebridge.input

import org.junit.Assert.assertNull
import org.junit.Test

/**
 * Liveness contract for the remote-input service.
 *
 * The singleton is non-null only while the platform accessibility service is
 * actually bound. The UI reads this (via the permissions channel) instead of
 * inferring "active" from the Settings.Secure enabled-services flag, which
 * records configuration rather than runtime state.
 */
class PhoneBridgeAccessibilityServiceLivenessTest {

    @Test
    fun `no instance until the platform binds the service`() {
        assertNull(PhoneBridgeAccessibilityService.getInstance())
    }
}
