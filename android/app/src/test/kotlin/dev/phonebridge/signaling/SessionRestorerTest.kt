package dev.phonebridge.signaling

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Cold-start restore logic tests, all on the JVM.
 *
 * The restore is pure Kotlin with injected edges (peer snapshot, HTTP, Go
 * transport), so every decision that matters for the Quick Settings tile —
 * which peer may be dialed, how the wait ends, when a busy desktop is
 * released, what counts as a connected transport — is asserted here without a
 * device.
 */
class SessionRestorerTest {

    // ------------------------------------------------------------------
    // pickTrustedPeerEndpoint — the security boundary of the restore
    // ------------------------------------------------------------------

    private fun row(
        id: String = "aa67e88a",
        host: String = "192.168.0.236",
        port: Int = 7804,
        isStale: Boolean = false,
    ): Map<String, Any?> = mapOf(
        "id" to id,
        "name" to "x1",
        "host" to host,
        "port" to port,
        "isStale" to isStale,
    )

    @Test
    fun `picks the first dialable trusted peer`() {
        val endpoint = pickTrustedPeerEndpoint(
            peers = listOf(row(id = "other"), row(id = "aa67e88a")),
            trustedIds = setOf("aa67e88a", "other"),
            localDeviceId = "self",
        )
        assertEquals("http://192.168.0.236:7804", endpoint)
    }

    @Test
    fun `never dials this device even when its own id is trusted`() {
        // The phone resolves its own advertisement over NSD; dialing it would
        // make a tile tap "restore" a session with itself.
        assertNull(
            pickTrustedPeerEndpoint(
                peers = listOf(row(id = "self")),
                trustedIds = setOf("self"),
                localDeviceId = "self",
            )
        )
    }

    @Test
    fun `ignores untrusted revoked stale and malformed rows`() {
        val peers = listOf(
            row(id = "untrusted"),
            row(id = "revoked-but-listed", isStale = true),
            row(id = "nohost", host = ""),
            row(id = "badport", port = 0),
            row(id = "good"),
        )
        assertEquals(
            "http://192.168.0.236:7804",
            pickTrustedPeerEndpoint(peers, setOf("revoked-but-listed", "good"), localDeviceId = "self")
        )
    }

    @Test
    fun `an empty trust set dials nothing`() {
        assertNull(pickTrustedPeerEndpoint(listOf(row()), emptySet(), localDeviceId = "self"))
    }

    // ------------------------------------------------------------------
    // awaitTrustedPeerEndpoint — event-driven, absolutely bounded
    // ------------------------------------------------------------------

    @Test
    fun `a peer already in the snapshot resolves without waiting for an event`() {
        var eventsAsked = 0
        val endpoint = awaitTrustedPeerEndpoint(
            pick = { "http://192.168.0.236:7804" },
            awaitEvent = { eventsAsked++; false },
            budgetMs = 10_000,
        )
        assertEquals("http://192.168.0.236:7804", endpoint)
        assertEquals("a warm browse must not pay an event wait", 0, eventsAsked)
    }

    @Test
    fun `a resolve event wakes the wait`() {
        var firstPick = true
        val endpoint = awaitTrustedPeerEndpoint(
            pick = {
                if (firstPick) {
                    firstPick = false
                    null
                } else {
                    "http://192.168.0.236:7804"
                }
            },
            awaitEvent = { true },
            budgetMs = 10_000,
        )
        assertEquals("http://192.168.0.236:7804", endpoint)
    }

    @Test
    fun `the wait ends null when the event never fires within the budget`() {
        val endpoint = awaitTrustedPeerEndpoint(
            pick = { null },
            awaitEvent = { false },
            budgetMs = 10_000,
        )
        assertNull(endpoint)
    }

    @Test
    fun `an event storm cannot outlive the absolute deadline`() {
        var now = 0L
        var waits = 0
        val endpoint = awaitTrustedPeerEndpoint(
            pick = { null },
            awaitEvent = { waits++; now += 60; true }, // events keep coming, peer never resolves
            budgetMs = 100,
            nowMs = { now },
        )
        assertNull(endpoint)
        assertEquals("each wake re-checks the deadline instead of restarting the budget", 2, waits)
    }

    // ------------------------------------------------------------------
    // awaitCondition — the bounded state observation
    // ------------------------------------------------------------------

    @Test
    fun `reports ready without a single step when the state already holds`() {
        var observations = 0
        assertTrue(awaitCondition({ observations++ >= 0 }, stepMs = 50, maxSteps = 40))
        assertEquals(1, observations)
    }

    @Test
    fun `observes state until it holds`() {
        var observations = 0
        assertTrue(
            awaitCondition(
                isReady = { ++observations >= 3 },
                stepMs = 1,
                maxSteps = 10,
            )
        )
        assertEquals(3, observations)
    }

    @Test
    fun `budget exhaustion reports not ready rather than waiting forever`() {
        assertFalse(awaitCondition({ false }, stepMs = 1, maxSteps = 3))
    }

    // ------------------------------------------------------------------
    // mediaStatsIndicateConnected — what "transport ready" really means
    // ------------------------------------------------------------------

    @Test
    fun `a connected negotiated or streaming transport reads as connected`() {
        assertTrue(mediaStatsIndicateConnected("""{"pcState":"connected","transportState":2}"""))
        assertTrue(mediaStatsIndicateConnected("""{"pcState":"connected","transportState":3}"""))
    }

    @Test
    fun `a stale connected over an idle released or stopped transport is not connected`() {
        // MediaStatsJSON falls back to the LAST known pcState once the session
        // object is gone — trusting it would skip a needed restore on a cold
        // start, which is exactly the bug this gate exists to prevent.
        assertFalse(mediaStatsIndicateConnected("""{"pcState":"connected","transportState":0}"""))
        assertFalse(mediaStatsIndicateConnected("""{"pcState":"connected","transportState":1}"""))
        assertFalse(mediaStatsIndicateConnected("""{"pcState":"connected","transportState":4}"""))
    }

    @Test
    fun `connecting failed and unknown peer-connection states are not connected`() {
        assertFalse(mediaStatsIndicateConnected("""{"pcState":"connecting","transportState":2}"""))
        assertFalse(mediaStatsIndicateConnected("""{"pcState":"failed","transportState":3}"""))
        assertFalse(mediaStatsIndicateConnected("""{"pcState":"","transportState":2}"""))
        assertFalse(mediaStatsIndicateConnected("""{"transportState":2}"""))
    }

    @Test
    fun `null empty or malformed stats never read as connected`() {
        assertFalse(mediaStatsIndicateConnected(null))
        assertFalse(mediaStatsIndicateConnected(""))
        assertFalse(mediaStatsIndicateConnected("not json"))
        assertFalse(mediaStatsIndicateConnected("{}"))
    }

    // ------------------------------------------------------------------
    // SessionRestorer.restore — outcome discipline
    // ------------------------------------------------------------------

    private class Recorder {
        var offers = 0
        var stops = 0
        var applied: String? = null
        var busyUntil = 0 // number of leading peer-offer calls refused as busy
        var stopThrows = false
        var offerThrows = false

        fun restorer(transportReady: Boolean = false) = SessionRestorer(
            endpoint = "http://192.168.0.236:7804",
            transportReady = { transportReady },
            createTransportOffer = {
                if (offerThrows) throw IllegalStateException("no offer")
                "OFFER-Sdp"
            },
            postOffer = { _, _ ->
                offers++
                if (offers <= busyUntil) {
                    throw DesktopSessionException("SESSION_BUSY", 409, "", "busy")
                }
                "ANSWER-Sdp"
            },
            postStop = {
                stops++
                if (stopThrows) throw IllegalStateException("stop failed")
            },
            applyAnswer = { applied = it },
            log = { /* quiet */ },
        )
    }

    @Test
    fun `a live transport short-circuits without tearing anything down`() {
        val rec = Recorder()
        val outcome = rec.restorer(transportReady = true).restore()
        assertTrue("expected AlreadyConnected, got $outcome", outcome is SessionRestorer.Outcome.AlreadyConnected)
        assertEquals("a warm tile tap must not re-offer", 0, rec.offers)
        assertEquals(0, rec.stops)
    }

    @Test
    fun `restores through the existing signed peer-offer path`() {
        val rec = Recorder()
        val outcome = rec.restorer().restore()
        assertTrue("expected Restored, got $outcome", outcome is SessionRestorer.Outcome.Restored)
        assertEquals("http://192.168.0.236:7804", (outcome as SessionRestorer.Outcome.Restored).endpoint)
        assertEquals(1, rec.offers)
        assertEquals("the answer must be applied before the flow reports success", "ANSWER-Sdp", rec.applied)
        assertEquals("a clean offer must never release the desktop's slot", 0, rec.stops)
    }

    @Test
    fun `a stale desktop slot is released exactly once and the offer retried`() {
        val rec = Recorder().apply { busyUntil = 1 }
        val outcome = rec.restorer().restore()
        assertTrue("expected Restored after one release, got $outcome", outcome is SessionRestorer.Outcome.Restored)
        assertEquals(2, rec.offers)
        assertEquals("only the stale slot is released — once", 1, rec.stops)
    }

    @Test
    fun `a second busy refusal fails instead of looping`() {
        val rec = Recorder().apply { busyUntil = Int.MAX_VALUE }
        val outcome = rec.restorer().restore()
        assertTrue("expected Failed, got $outcome", outcome is SessionRestorer.Outcome.Failed)
        assertEquals("the retry is bounded to one re-offer", 2, rec.offers)
        assertEquals(1, rec.stops)
    }

    @Test
    fun `a non-busy refusal fails without touching the desktop slot`() {
        val rec = Recorder()
        val failing = SessionRestorer(
            endpoint = "http://192.168.0.236:7804",
            transportReady = { false },
            createTransportOffer = { "OFFER-Sdp" },
            postOffer = { _, _ -> throw DesktopSessionException("PERMISSION_DENIED", 403, "", "not paired") },
            postStop = { rec.stops++ },
            applyAnswer = { rec.applied = it },
            log = { },
        )
        val outcome = failing.restore()
        assertTrue(outcome is SessionRestorer.Outcome.Failed)
        assertEquals(0, rec.stops)
    }

    @Test
    fun `a failed slot release still gets its one retry`() {
        val rec = Recorder().apply {
            busyUntil = 1
            stopThrows = true
        }
        val outcome = rec.restorer().restore()
        assertTrue("expected Restored despite the failed stop, got $outcome", outcome is SessionRestorer.Outcome.Restored)
        assertEquals(1, rec.stops)
    }

    @Test
    fun `an offer that cannot be built fails before any dial`() {
        val rec = Recorder().apply { offerThrows = true }
        val outcome = rec.restorer().restore()
        assertTrue(outcome is SessionRestorer.Outcome.Failed)
        assertEquals("no offer means nothing to send", 0, rec.offers)
        assertNull(rec.applied)
    }
}
