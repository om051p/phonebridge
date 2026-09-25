package dev.phonebridge.bridge

import java.io.File
import java.util.concurrent.CopyOnWriteArrayList
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Assume.assumeTrue
import org.junit.Test

/**
 * JVM-level JNI tests for GoBridge Remote Input subsystem (DEC-027, Phase 7 v0.1).
 * Tests Kotlin -> JNI -> Go input bridge lifecycle and host callbacks.
 */
class GoBridgeInputTest {

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

    private class TestInputCallback : InputHostCallback {
        val touches = CopyOnWriteArrayList<List<Float>>()
        val keys = CopyOnWriteArrayList<List<Int>>()
        val texts = CopyOnWriteArrayList<String>()
        val scrolls = CopyOnWriteArrayList<List<Float>>()
        val actions = CopyOnWriteArrayList<Int>()

        override fun onTouch(action: Int, pointerId: Int, normX: Float, normY: Float, pressure: Float): Boolean {
            touches.add(listOf(action.toFloat(), pointerId.toFloat(), normX, normY, pressure))
            return true
        }

        override fun onKey(action: Int, keyCode: Int, metaState: Int): Boolean {
            keys.add(listOf(action, keyCode, metaState))
            return true
        }

        override fun onText(text: String): Boolean {
            texts.add(text)
            return true
        }

        override fun onScroll(normX: Float, normY: Float, deltaX: Float, deltaY: Float): Boolean {
            scrolls.add(listOf(normX, normY, deltaX, deltaY))
            return true
        }

        override fun onGlobalAction(actionType: Int): Boolean {
            actions.add(actionType)
            return true
        }
    }

    @Test
    fun `input initialization and lifecycle`() {
        requireEngine()
        try {
            val callback = TestInputCallback()
            assertTrue(GoBridge.inputInit(callback))

            GoBridge.inputStop()
        } finally {
            GoBridge.stop()
        }
    }
}
