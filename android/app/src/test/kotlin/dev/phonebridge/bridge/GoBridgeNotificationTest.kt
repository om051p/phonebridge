package dev.phonebridge.bridge

import java.io.File
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Assume.assumeTrue
import org.junit.Test

/**
 * JVM-level JNI tests for GoBridge Notifications mirroring subsystem (DEC-028, Phase 8 v0.1).
 * Tests Kotlin -> JNI -> Go notification bridge lifecycle and dispatch.
 */
class GoBridgeNotificationTest {

    private val libLoaded: Boolean by lazy {
        if (GoBridge.loaded) return@lazy true
        val searchDirs = mutableListOf<String>()
        System.getProperty("java.library.path")?.split(File.pathSeparator)?.let { searchDirs += it }
        var dir: File? = System.getProperty("user.dir")?.let { File(it) }
        repeat(6) {
            val d = dir ?: return@repeat
            searchDirs += File(d, "core/build").path
            dir = d.parentFile
        }
        for (d in searchDirs) {
            val f = File(d, "libphonebridge_core.so")
            if (f.isFile) {
                try {
                    System.load(f.absolutePath)
                    return@lazy GoBridge.loaded
                } catch (e: UnsatisfiedLinkError) {
                    // ignore
                }
            }
        }
        false
    }

    private fun requireEngine() {
        assumeTrue("host libphonebridge_core.so not found", libLoaded)
        assertTrue("GoBridge.start failed", GoBridge.start(null))
    }

    private class TestNotificationCallback : NotificationHostCallback {
        var lastDismissedKey: String? = null
        override fun onDismiss(key: String): Boolean {
            lastDismissedKey = key
            return true
        }
    }

    @Test
    fun `notification initialization and lifecycle`() {
        requireEngine()
        try {
            val callback = TestNotificationCallback()
            assertTrue(GoBridge.notificationInit(callback))

            // Posting when channel is not open drops safely and returns true (or handled safely)
            val postedOk = GoBridge.notificationPost(
                key = "0|com.example.chat|1|null|100",
                packageName = "com.example.chat",
                appName = "ExampleChat",
                title = "Alice",
                text = "Hello world",
                subText = "",
                postTimeMs = System.currentTimeMillis(),
                isOngoing = false,
                isClearable = true,
                category = "msg",
            )
            assertTrue(postedOk)

            val removedOk = GoBridge.notificationRemove(
                key = "0|com.example.chat|1|null|100",
                packageName = "com.example.chat",
                reason = 1,
            )
            assertTrue(removedOk)

            val stats = GoBridge.notificationStats()
            assertNotNull(stats)

            GoBridge.notificationStop()
        } finally {
            GoBridge.stop()
        }
    }
}
