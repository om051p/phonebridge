package dev.phonebridge.signaling

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertThrows
import org.junit.Test

/**
 * Wire-shape tests for the shared DEC-022 client used by both the UI CONNECT
 * and the Quick Settings cold-start restore. The endpoint normalisation and
 * blob unwrapping here are the two places a malformed peer record or a
 * double-encoded SDP would otherwise reach the desktop.
 */
class DesktopSessionTest {

    @Test
    fun `a usable signaling url is normalised to the bare base`() {
        assertEquals(
            "http://192.168.0.236:7804",
            DesktopSession.normaliseEndpoint(" http://192.168.0.236:7804/ "),
        )
        assertEquals(
            "http://192.168.0.236:7804",
            DesktopSession.normaliseEndpoint("http://192.168.0.236:7804"),
        )
    }

    @Test
    fun `unusable urls are rejected rather than dialed blindly`() {
        assertNull(DesktopSession.normaliseEndpoint(""))
        assertNull(DesktopSession.normaliseEndpoint("   "))
        assertNull(DesktopSession.normaliseEndpoint("not a url"))
        assertNull(DesktopSession.normaliseEndpoint("192.168.0.236:7804"))
        assertNull(DesktopSession.normaliseEndpoint("http://example.com")) // no port
    }

    @Test
    fun `the sdp is unwrapped from the transport blob`() {
        val sdp = "v=0\r\no=- 1 1 IN IP4 0.0.0.0\r\n"
        val blob = """{"type":"offer","sdp":"v=0\r\no=- 1 1 IN IP4 0.0.0.0\r\n"}"""
        assertEquals(sdp, DesktopSession.sdpFromOfferBlob(blob.toByteArray()))
    }

    @Test
    fun `a blob without sdp is refused instead of fabricating an offer`() {
        assertThrows(IllegalStateException::class.java) {
            DesktopSession.sdpFromOfferBlob("""{"type":"offer"}""".toByteArray())
        }
        assertThrows(IllegalStateException::class.java) {
            DesktopSession.sdpFromOfferBlob("""{"type":"offer","sdp":"  "}""".toByteArray())
        }
    }

    @Test
    fun `typed refusals map to actionable messages`() {
        val cases = mapOf(
            "SESSION_BUSY" to "The desktop is already in a session",
            "PERMISSION_DENIED" to "This phone is not paired with the desktop",
            "INCOMPATIBLE_VERSION" to "The desktop runs an incompatible session protocol",
            "TRANSPORT_FAILED" to "The desktop could not open the media transport",
            "UNSUPPORTED_MEDIA_PARAMS" to "The desktop rejected the requested capture format",
        )
        for ((code, expected) in cases) {
            assertEquals(expected, DesktopSession.describeFailure(code, "", 500))
        }
    }

    @Test
    fun `unknown codes fall back to the peer message then to the http status`() {
        assertEquals(
            "something specific",
            DesktopSession.describeFailure("OTHER", """{"message":"something specific"}""", 502),
        )
        assertEquals(
            "The desktop refused the session (HTTP 502)",
            DesktopSession.describeFailure("OTHER", "not json", 502),
        )
        assertEquals(
            "The desktop refused the session (HTTP 500)",
            DesktopSession.describeFailure(null, "", 500),
        )
    }
}
