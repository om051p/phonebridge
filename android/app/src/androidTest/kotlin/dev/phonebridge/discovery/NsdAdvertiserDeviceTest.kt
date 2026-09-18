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
}
