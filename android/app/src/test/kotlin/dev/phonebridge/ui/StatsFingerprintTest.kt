package dev.phonebridge.ui

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Test

/**
 * Tests for the stats-tick dedupe key (I3 of the comms audit).
 *
 * The 1 Hz stats tick used to push a full map every second even when nothing
 * changed, so Flutter handled and rebuilt an event nobody acted on. The tick now
 * compares fingerprints and only pushes when something is new — while a capture
 * is running it always pushes, because the UI derives currentFps from successive
 * timestampMs values.
 *
 * Two properties matter:
 *  - the timestamp must NOT be part of the key (it changes every tick, which
 *    would make every push look like a change), and
 *  - every other field MUST be, or a real change would never be pushed.
 */
class StatsFingerprintTest {

    private fun canonicalStats(): Map<String, Any?> = mapOf(
        "isCapturing" to true,
        "encodedFrames" to 100L,
        "keyframes" to 4L,
        "admittedFrames" to 96L,
        "droppedFrames" to 4L,
        "codec" to "video/avc",
        "isHardwareCodec" to true,
        "durationUs" to 1_000_000L,
        "goStatsJson" to "{}",
        "timestampMs" to 1_700_000_000_000L,
        "clipboardState" to "AMBIENT_ACTIVE",
        "imeSelected" to true,
        "enabled" to true,
        "lastError" to null
    )

    @Test
    fun `timestamp alone does not change the fingerprint`() {
        val first = statsFingerprintOf(canonicalStats())
        val second = statsFingerprintOf(canonicalStats() + ("timestampMs" to 1_700_000_001_000L))
        assertEquals(
            "timestampMs must be excluded, otherwise idle ticks always look changed",
            first,
            second
        )
    }

    @Test
    fun `identical stats produce identical fingerprints`() {
        assertEquals(statsFingerprintOf(canonicalStats()), statsFingerprintOf(canonicalStats()))
    }

    @Test
    fun `every emitted field changes the fingerprint`() {
        val baseline = statsFingerprintOf(canonicalStats())
        // Every field collectStats emits, minus timestampMs.
        val changes: Map<String, Any?> = mapOf(
            "isCapturing" to false,
            "encodedFrames" to 101L,
            "keyframes" to 5L,
            "admittedFrames" to 97L,
            "droppedFrames" to 5L,
            "codec" to "video/hevc",
            "isHardwareCodec" to false,
            "durationUs" to 2_000_000L,
            "goStatsJson" to """{"rtt":42}""",
            "clipboardState" to "WRITE_ONLY_DORMANT",
            "imeSelected" to false,
            "enabled" to false,
            "lastError" to "capture failed"
        )
        for ((key, value) in changes) {
            val mutated = canonicalStats() + (key to value)
            assertNotEquals(
                "changing $key must change the fingerprint, or the UI would never see it",
                baseline,
                statsFingerprintOf(mutated)
            )
        }
    }

    @Test
    fun `a cleared error is distinguishable from having one`() {
        val withError = statsFingerprintOf(canonicalStats() + ("lastError" to "boom"))
        val withoutError = statsFingerprintOf(canonicalStats())
        assertNotEquals(withError, withoutError)
    }
}
