package dev.phonebridge.discovery

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Deterministic tests for the platform browse state machine.
 *
 * The browse client seam stands in for NsdManager, so peer mapping, resolve
 * serialization, loss handling and recovery are exercised without the Android
 * framework (device-side behaviour stays covered by the signalling/NSD device
 * tests).
 */
class NsdBrowserTest {

    private class FakeBrowseClient : NsdBrowseClient {
        var startCalls = 0
        var stopCalls = 0
        var startResult = true

        var onFound: ((String) -> Unit)? = null
        var onLost: ((String) -> Unit)? = null
        var onFailure: ((Int) -> Unit)? = null

        val resolveCalls = mutableListOf<String>()
        private var pendingResolved: ((NsdPeer) -> Unit)? = null
        private var pendingFailed: ((Int) -> Unit)? = null

        val resolveInFlight: Boolean get() = pendingResolved != null

        override fun startDiscovery(
            onServiceFound: (String) -> Unit,
            onServiceLost: (String) -> Unit,
            onFailure: (Int) -> Unit,
        ): Boolean {
            startCalls++
            onFound = onServiceFound
            onLost = onServiceLost
            this.onFailure = onFailure
            return startResult
        }

        override fun resolve(
            serviceName: String,
            onResolved: (NsdPeer) -> Unit,
            onFailed: (Int) -> Unit,
        ) {
            resolveCalls += serviceName
            pendingResolved = onResolved
            pendingFailed = onFailed
        }

        override fun stopDiscovery() {
            stopCalls++
        }

        fun found(serviceName: String) {
            onFound?.invoke(serviceName)
        }

        fun lost(serviceName: String) {
            onLost?.invoke(serviceName)
        }

        fun fail(code: Int) {
            onFailure?.invoke(code)
        }

        fun complete(peer: NsdPeer) {
            val cb = pendingResolved ?: error("no resolve in flight")
            pendingResolved = null
            pendingFailed = null
            cb(peer)
        }

        fun failResolve(code: Int) {
            val cb = pendingFailed ?: error("no resolve in flight")
            pendingResolved = null
            pendingFailed = null
            cb(code)
        }
    }

    private fun peer(
        id: String = "aa67e88a",
        name: String = "x1",
        host: String = "192.168.0.236",
        port: Int = 7804,
    ) = NsdPeer(
        serviceName = "PhoneBridge-$id",
        deviceId = id,
        deviceName = name,
        model = "x1",
        version = "1",
        host = host,
        port = port,
    )

    @Test
    fun `start is idempotent while discovery is live`() {
        val client = FakeBrowseClient()
        val browser = NsdBrowser(client)

        assertTrue(browser.start())
        assertTrue(browser.start())
        assertTrue(browser.start())

        assertEquals(1, client.startCalls)
        assertTrue(browser.isBrowsing)
    }

    @Test
    fun `resolved peer is exposed in the ui shape`() {
        val client = FakeBrowseClient()
        val browser = NsdBrowser(client)
        browser.start()

        client.found("PhoneBridge-aa67e88a")
        assertEquals(listOf("PhoneBridge-aa67e88a"), client.resolveCalls)

        client.complete(peer())

        val rows = browser.snapshot()
        assertEquals(1, rows.size)
        assertEquals("aa67e88a", rows[0]["id"])
        assertEquals("x1", rows[0]["name"])
        assertEquals("192.168.0.236", rows[0]["host"])
        assertEquals(7804, rows[0]["port"])
        assertEquals(false, rows[0]["isStale"])
        assertEquals("1", rows[0]["version"])
        assertEquals("x1", rows[0]["model"])
    }

    @Test
    fun `instances without an id address or port are not offered`() {
        val client = FakeBrowseClient()
        val browser = NsdBrowser(client)
        browser.start()

        client.found("PhoneBridge-noid")
        client.complete(peer(id = "", host = "192.168.0.9"))
        assertTrue(browser.snapshot().isEmpty())

        client.found("PhoneBridge-noaddr")
        client.complete(peer(id = "bb22", host = ""))
        assertTrue(browser.snapshot().isEmpty())

        client.found("PhoneBridge-noport")
        client.complete(peer(id = "cc33", port = 0))
        assertTrue(browser.snapshot().isEmpty())
    }

    @Test
    fun `resolutions are serialized and queue in order`() {
        val client = FakeBrowseClient()
        val browser = NsdBrowser(client)
        browser.start()

        client.found("PhoneBridge-one")
        client.found("PhoneBridge-two")

        // The platform refuses a second concurrent resolve, so the browser must
        // not issue one until the first completes.
        assertEquals(listOf("PhoneBridge-one"), client.resolveCalls)

        client.complete(peer(id = "one"))
        assertEquals(listOf("PhoneBridge-one", "PhoneBridge-two"), client.resolveCalls)

        client.complete(peer(id = "two"))
        assertEquals(2, browser.snapshot().size)
    }

    @Test
    fun `a failed resolve does not block the queue`() {
        val client = FakeBrowseClient()
        val browser = NsdBrowser(client)
        browser.start()

        client.found("PhoneBridge-broken")
        client.found("PhoneBridge-good")
        client.failResolve(3)
        assertEquals(listOf("PhoneBridge-broken", "PhoneBridge-good"), client.resolveCalls)

        client.complete(peer(id = "good"))
        assertEquals(1, browser.snapshot().size)
        assertEquals("good", browser.snapshot()[0]["id"])
    }

    @Test
    fun `a lost service removes its peer`() {
        val client = FakeBrowseClient()
        val browser = NsdBrowser(client)
        browser.start()

        client.found("PhoneBridge-aa67e88a")
        client.complete(peer())
        assertEquals(1, browser.snapshot().size)

        client.lost("PhoneBridge-aa67e88a")
        assertTrue(browser.snapshot().isEmpty())
    }

    @Test
    fun `discovery failure is reported and clears the browsing flag`() {
        val client = FakeBrowseClient()
        val browser = NsdBrowser(client)
        var reported = -1
        browser.onBrowseFailed = { reported = it }

        browser.start()
        assertTrue(browser.isBrowsing)

        client.fail(5)
        assertEquals(5, reported)
        assertFalse(browser.isBrowsing)
    }

    @Test
    fun `restart retires the old session and clears the snapshot`() {
        val client = FakeBrowseClient()
        val browser = NsdBrowser(client)
        browser.start()
        client.found("PhoneBridge-aa67e88a")
        client.complete(peer())
        assertEquals(1, browser.snapshot().size)

        assertTrue(browser.restart())

        assertTrue(browser.snapshot().isEmpty())
        assertEquals(2, client.startCalls)
        assertEquals(1, client.stopCalls)
    }

    @Test
    fun `late callbacks from a retired session are ignored`() {
        val client = FakeBrowseClient()
        val browser = NsdBrowser(client)
        browser.start()

        client.found("PhoneBridge-aa67e88a")
        // Restarting bumps the generation; the in-flight resolve belongs to the
        // retired session and must not add a row to the new one.
        browser.restart()
        client.complete(peer())

        assertTrue(browser.snapshot().isEmpty())
    }

    @Test
    fun `stop clears state and reports not browsing`() {
        val client = FakeBrowseClient()
        val browser = NsdBrowser(client)
        browser.start()
        client.found("PhoneBridge-aa67e88a")
        client.complete(peer())

        browser.stop()

        assertFalse(browser.isBrowsing)
        assertTrue(browser.snapshot().isEmpty())
        assertEquals(1, client.stopCalls)
    }

    @Test
    fun `a refused start reports failure rather than browsing`() {
        val client = FakeBrowseClient().apply { startResult = false }
        val browser = NsdBrowser(client)

        assertFalse(browser.start())
        assertFalse(browser.isBrowsing)
    }

    @Test
    fun `no nsd support degrades to an empty snapshot`() {
        val browser = NsdBrowser(null as NsdBrowseClient?)

        assertFalse(browser.start())
        assertFalse(browser.isBrowsing)
        assertTrue(browser.snapshot().isEmpty())
    }
}
