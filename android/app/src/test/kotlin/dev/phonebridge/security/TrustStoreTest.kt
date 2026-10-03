package dev.phonebridge.security

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import java.io.File

class TrustStoreTest {

    @get:Rule
    val tempFolder = TemporaryFolder()

    @Test
    fun testLifecycleAndPersistence() {
        val storeFile = File(tempFolder.root, "trusted_devices.json")
        val store1 = TrustStore(storeFile)

        val devId = "device-abc-123"
        val rawPub = ByteArray(32) { 0x42 }

        // 1. Initially untrusted
        assertFalse("must be untrusted initially", store1.isTrusted(devId))
        assertNull(store1.get(devId))

        // 2. Add trusted
        val rec = TrustedDeviceRecord(
            deviceId = devId,
            displayName = "Linux Laptop",
            platform = "linux",
            rawPublicKey = rawPub,
            pairedAtMs = 1700000000L,
            lastSeenMs = 1700000000L,
            revoked = false
        )
        store1.addTrusted(rec)
        assertTrue("must be trusted after add", store1.isTrusted(devId))

        val retrieved = store1.get(devId)
        assertNotNull(retrieved)
        assertEquals("Linux Laptop", retrieved?.displayName)
        assertEquals(false, retrieved?.revoked)

        // 3. Survives restart / reload
        val store2 = TrustStore(storeFile)
        assertTrue("must remain trusted after reload", store2.isTrusted(devId))
        assertEquals(1, store2.list().size)

        // 4. Revocation
        assertTrue("revoke must succeed", store2.revoke(devId))
        assertFalse("revoked device must not be trusted", store2.isTrusted(devId))
        assertTrue("record still exists but revoked", store2.get(devId)?.revoked == true)

        // 5. Revocation persists
        val store3 = TrustStore(storeFile)
        assertFalse("revoked status must persist", store3.isTrusted(devId))
        assertTrue(store3.get(devId)?.revoked == true)

        // 6. Removal
        assertTrue(store3.remove(devId))
        assertNull(store3.get(devId))
        assertEquals(0, store3.list().size)
    }

    @Test
    fun testChangeListenerFiresOnMutations() {
        val storeFile = File(tempFolder.root, "trusted_listener.json")
        val store = TrustStore(storeFile)
        val rawPub = ByteArray(32) { 0x42 }
        val rec = TrustedDeviceRecord(
            deviceId = "dev-1", displayName = "PC", platform = "linux",
            rawPublicKey = rawPub, pairedAtMs = 1L, lastSeenMs = 1L, revoked = false
        )
        var fires = 0
        TrustStore.changeListener = { fires++ }
        try {
            store.addTrusted(rec)
            store.revoke("dev-1")
            store.remove("dev-1")
        } finally {
            TrustStore.changeListener = null
        }
        assertEquals("listener must fire once per mutation", 3, fires)
    }

    @Test
    fun testReloadPicksUpOtherInstanceWrites() {
        val storeFile = File(tempFolder.root, "trusted_reload.json")
        val store1 = TrustStore(storeFile)
        val store2 = TrustStore(storeFile)
        assertEquals(0, store2.list().size)

        store1.addTrusted(
            TrustedDeviceRecord(
                deviceId = "dev-2", displayName = "PC", platform = "linux",
                rawPublicKey = ByteArray(32) { 0x07 },
                pairedAtMs = 1L, lastSeenMs = 1L, revoked = false
            )
        )
        // store2 has a stale in-memory view until it reloads.
        store2.reload()
        assertTrue(store2.isTrusted("dev-2"))
        assertEquals(1, store2.list().size)
    }
}
