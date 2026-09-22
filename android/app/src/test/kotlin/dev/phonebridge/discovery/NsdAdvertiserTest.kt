package dev.phonebridge.discovery

import android.net.nsd.NsdManager
import android.net.nsd.NsdServiceInfo
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Deterministic tests for the NsdAdvertiser registration state machine.
 *
 * The registrar seam stands in for NsdManager so registration success, failure,
 * silent loss and late callbacks are all exercised without the Android
 * framework. Device-side behaviour stays covered by NsdAdvertiserDeviceTest.
 */
class NsdAdvertiserTest {

    private companion object {
        const val REGISTRATION_ERROR_CODE = 3
    }

    /** Fake registrar that models the platform's callback behaviour and ordering. */
    private class FakeRegistrar : NsdRegistrar {
        val registerCalls = mutableListOf<NsdServiceInfo>()
        val listeners = mutableListOf<NsdManager.RegistrationListener>()
        val unregisterCalls = mutableListOf<NsdManager.RegistrationListener>()
        private val live = mutableSetOf<NsdManager.RegistrationListener>()
        val liveListeners: Int get() = live.size
        var autoConfirm = true
        var failNextRegistration = false

        override fun register(info: NsdServiceInfo, protocol: Int, listener: NsdManager.RegistrationListener) {
            registerCalls += info
            listeners += listener
            if (failNextRegistration) {
                failNextRegistration = false
                listener.onRegistrationFailed(info, REGISTRATION_ERROR_CODE)
                return
            }
            live += listener
            if (autoConfirm) {
                listener.onServiceRegistered(info)
            }
        }

        override fun unregister(listener: NsdManager.RegistrationListener) {
            unregisterCalls += listener
            // Android throws IllegalArgumentException for a listener it does not
            // have registered; the advertiser must tolerate that during recovery.
            if (!live.remove(listener)) {
                throw IllegalArgumentException("listener not registered")
            }
            listener.onServiceUnregistered(NsdServiceInfo())
        }

        /** Simulates the platform tearing the advertisement down with no callback. */
        fun simulateSilentLoss() {
            live.clear()
        }

        fun lastListener(): NsdManager.RegistrationListener = listeners.last()
    }

    private fun advertiser(registrar: FakeRegistrar) = NsdAdvertiser(registrar as NsdRegistrar)

    private fun NsdAdvertiser.registerDefaults() = registerService(
        port = 7804,
        deviceId = "test-device",
        deviceName = "POCO F5",
        capabilities = "screen",
        state = "ready"
    )

    @Test
    fun `first registration reaches the registrar exactly once`() {
        val registrar = FakeRegistrar()
        val adv = advertiser(registrar)

        assertTrue(adv.registerDefaults())
        assertTrue(adv.isRegistered)
        assertFalse(adv.isRegistrationPending)
        assertEquals(1, registrar.registerCalls.size)
        assertEquals(1, registrar.liveListeners)
    }

    @Test
    fun `repeated register while registered is ignored`() {
        val registrar = FakeRegistrar()
        val adv = advertiser(registrar)

        adv.registerDefaults()
        repeat(3) { assertTrue(adv.registerDefaults()) }

        assertEquals("duplicate registrations must not reach the platform", 1, registrar.registerCalls.size)
        assertEquals(1, registrar.liveListeners)
    }

    @Test
    fun `register while pending is ignored until the platform callback arrives`() {
        val registrar = FakeRegistrar().apply { autoConfirm = false }
        val adv = advertiser(registrar)

        assertTrue(adv.registerDefaults())
        assertTrue(adv.isRegistrationPending)
        assertFalse(adv.isRegistered)

        // Repeated lifecycle signals while the callback is outstanding must not
        // create a second registration.
        assertTrue(adv.registerDefaults())
        assertTrue(adv.ensureRegistered())
        assertEquals(1, registrar.registerCalls.size)

        // Platform confirms; the pending registration becomes live.
        registrar.lastListener().onServiceRegistered(registrar.registerCalls.single())
        assertTrue(adv.isRegistered)
        assertFalse(adv.isRegistrationPending)
        assertEquals(1, registrar.liveListeners)
    }

    @Test
    fun `ensureRegistered is a no-op when registration is live`() {
        val registrar = FakeRegistrar()
        val adv = advertiser(registrar)

        adv.registerDefaults()
        assertTrue(adv.ensureRegistered())

        assertEquals(1, registrar.registerCalls.size)
        assertEquals(0, registrar.unregisterCalls.size)
    }

    @Test
    fun `failed registration is reported and retried with remembered parameters`() {
        val registrar = FakeRegistrar().apply { failNextRegistration = true }
        val adv = advertiser(registrar)
        var failureCode: Int? = null
        adv.onRegistrationFailed = { failureCode = it }

        assertFalse(adv.registerDefaults())
        assertFalse(adv.isRegistered)
        assertFalse(adv.isRegistrationPending)
        assertEquals(REGISTRATION_ERROR_CODE, failureCode)

        // Recovery must work without the caller supplying parameters again.
        assertTrue(adv.ensureRegistered())
        assertTrue(adv.isRegistered)
        assertEquals(2, registrar.registerCalls.size)
        assertEquals(1, registrar.liveListeners)
    }

    @Test
    fun `force reregister yields exactly one live registration and retires the old listener`() {
        val registrar = FakeRegistrar()
        val adv = advertiser(registrar)

        adv.registerDefaults()
        assertTrue(adv.isRegistered)

        assertTrue(adv.reregisterService())

        assertTrue(adv.isRegistered)
        assertEquals("exactly one new registration per recovery", 2, registrar.registerCalls.size)
        assertEquals("the previous listener must be retired", 1, registrar.unregisterCalls.size)
        assertEquals("never more than one live registration", 1, registrar.liveListeners)
    }

    @Test
    fun `late callback from a retired listener cannot clear the current registration`() {
        val registrar = FakeRegistrar()
        val adv = advertiser(registrar)

        adv.registerDefaults()
        val retiredListener = registrar.lastListener()

        adv.reregisterService()
        assertTrue(adv.isRegistered)

        // The platform delivers the retired listener's unregistration late.
        retiredListener.onServiceUnregistered(NsdServiceInfo())

        assertTrue("stale callback must not clear the live registration", adv.isRegistered)
    }

    @Test
    fun `silent platform loss is repaired by the recovery path`() {
        val registrar = FakeRegistrar()
        val adv = advertiser(registrar)

        adv.registerDefaults()
        assertTrue(adv.isRegistered)

        // Platform tears the advertisement down without telling the app.
        registrar.simulateSilentLoss()
        assertEquals(0, registrar.liveListeners)
        assertTrue("the app cannot observe silent loss; state stays optimistic", adv.isRegistered)

        // A lifecycle signal triggers recovery, which replaces the registration.
        assertTrue(adv.reregisterService())
        assertEquals(1, registrar.liveListeners)
        assertEquals(2, registrar.registerCalls.size)
    }

    @Test
    fun `unregister clears state and allows a fresh registration`() {
        val registrar = FakeRegistrar()
        val adv = advertiser(registrar)

        adv.registerDefaults()
        adv.unregisterService()

        assertFalse(adv.isRegistered)
        assertFalse(adv.isRegistrationPending)
        assertNull(adv.registeredServiceName)
        assertEquals(1, registrar.unregisterCalls.size)
        assertEquals(0, registrar.liveListeners)

        assertTrue(adv.registerDefaults())
        assertEquals(2, registrar.registerCalls.size)
        assertEquals(1, registrar.liveListeners)
    }

    @Test
    fun `repeated start reregister stop cycles never accumulate live listeners`() {
        val registrar = FakeRegistrar()
        val adv = advertiser(registrar)

        repeat(5) {
            adv.registerService(
                port = 7804,
                deviceId = "cycle-device",
                deviceName = "POCO F5",
                capabilities = "screen"
            )
            adv.ensureRegistered()
            adv.reregisterService()
            assertEquals("one live registration per cycle", 1, registrar.liveListeners)
            adv.unregisterService()
            assertEquals("fully retired after stop", 0, registrar.liveListeners)
        }

        assertEquals("registrations match the cycles exactly", 10, registrar.registerCalls.size)
        assertEquals("every registration was retired", 10, registrar.unregisterCalls.size)
    }

    @Test
    fun `recovery without remembered parameters does not register`() {
        val registrar = FakeRegistrar()
        val adv = advertiser(registrar)

        assertFalse(adv.reregisterService())
        assertFalse(adv.ensureRegistered())
        assertEquals(0, registrar.registerCalls.size)
    }

    @Test
    fun `unregister before any registration is a no-op`() {
        val registrar = FakeRegistrar()
        val adv = advertiser(registrar)

        adv.unregisterService()

        assertEquals(0, registrar.unregisterCalls.size)
        assertFalse(adv.isRegistered)
    }
}
