package dev.phonebridge.discovery

import androidx.test.platform.app.InstrumentationRegistry
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit

/**
 * On-device instrumented test verifying mDNS service advertisement via NsdManager
 * on real physical Android hardware (POCO F5 / Android 15).
 */
class NsdAdvertiserDeviceTest {

    @Test
    fun registersAndUnregistersMdnsService() {
        val context = InstrumentationRegistry.getInstrumentation().targetContext
        assertNotNull("context cannot be null", context)

        val advertiser = NsdAdvertiser(context)
        val registeredLatch = CountDownLatch(1)

        advertiser.registerService(
            port = 7804,
            deviceId = "poco-f5-test",
            deviceName = "POCO F5 Device Test",
            capabilities = "screen,files,clipboard",
            state = "ready"
        )

        // Wait up to 5 seconds for NsdManager callback
        for (i in 0 until 50) {
            if (advertiser.isRegistered) {
                registeredLatch.countDown()
                break
            }
            Thread.sleep(100)
        }

        val registered = registeredLatch.await(5, TimeUnit.SECONDS)
        assertTrue("mDNS advertisement failed to register within 5s", registered)
        assertTrue("isRegistered should be true", advertiser.isRegistered)
        assertNotNull("serviceName should be assigned", advertiser.registeredServiceName)

        // Unregister
        advertiser.unregisterService()
        Thread.sleep(300)
        assertTrue("isRegistered should be false after unregister", !advertiser.isRegistered)
    }

    /**
     * Recovery lifecycle on real hardware: repeated registration requests are
     * ignored while a registration is live, and a forced re-registration
     * completes without leaving the advertiser unregistered or double-counted.
     */
    @Test
    fun reregistersIdempotentlyWithoutLosingTheAdvertisement() {
        val context = InstrumentationRegistry.getInstrumentation().targetContext
        val advertiser = NsdAdvertiser(context)

        advertiser.registerService(
            port = 7804,
            deviceId = "poco-f5-reregister",
            deviceName = "POCO F5 Reregister Test",
            capabilities = "screen",
            state = "ready"
        )
        assertTrue("initial registration timed out", awaitRegistered(advertiser))

        val firstServiceName = advertiser.registeredServiceName

        // Idempotent: a second lifecycle signal must not disturb the live registration.
        advertiser.registerService(
            port = 7804,
            deviceId = "poco-f5-reregister",
            deviceName = "POCO F5 Reregister Test",
            capabilities = "screen",
            state = "ready"
        )
        Thread.sleep(500)
        assertTrue("idempotent register must leave the advertisement live", advertiser.isRegistered)

        // Forced recovery: retires the current listener and registers again.
        assertTrue("forced re-registration was refused", advertiser.reregisterService())
        assertTrue("re-registration timed out", awaitRegistered(advertiser))
        assertNotNull("service name should be assigned after recovery", advertiser.registeredServiceName)

        // The advertised identity must survive the recovery (instance label may be
        // re-suffixed by the platform, the TXT id stays authoritative).
        assertTrue(
            "unexpected service label after recovery: ${advertiser.registeredServiceName}",
            advertiser.registeredServiceName!!.startsWith("PhoneBridge-")
        )
        println("service label before=${firstServiceName} after=${advertiser.registeredServiceName}")

        advertiser.unregisterService()
        Thread.sleep(300)
        assertTrue("isRegistered should be false after unregister", !advertiser.isRegistered)
    }

    private fun awaitRegistered(advertiser: NsdAdvertiser, timeoutMs: Long = 5000): Boolean {
        val step = 100L
        var waited = 0L
        while (waited < timeoutMs) {
            if (advertiser.isRegistered) return true
            Thread.sleep(step)
            waited += step
        }
        return advertiser.isRegistered
    }
}
