package dev.phonebridge.spike04

/**
 * Kotlin -> Go JNI boundary for the Spike 04 Pion engine
 * (libphonebridge_spike04.so, built with -buildmode=c-shared).
 *
 * Direction: Android is the SDP OFFERER; the Linux receiver answers a
 * spike-local HTTP exchange. All frame pushes are Annex-B access units
 * (CSD concatenated ahead of the first IDR), timestamps are MediaCodec
 * presentationTimeUs (monotonic, maps 1:1 onto the 90 kHz RTP clock).
 *
 * Isolated spike code — not production.
 */
object GoBridge {
    @Volatile
    var loaded: Boolean = false
        private set

    private fun load(): Boolean {
        if (loaded) return true
        return try {
            System.loadLibrary("phonebridge_spike04")
            loaded = true
            S4Log.i("SPIKE04_GO libphonebridge_spike04.so loaded")
            true
        } catch (t: UnsatisfiedLinkError) {
            S4Log.e("SPIKE04_GO load failed: ${t.message}")
            false
        } catch (t: Throwable) {
            S4Log.e("SPIKE04_GO load threw: ${t.message}")
            false
        }
    }

    /**
     * Create the Go engine + peer connection. Must precede all other calls.
     *
     * [shapeKbps] sustained RTP send-rate ceiling for the burst shaper
     * (0 = unshaped); [shapeBurstK] token-bucket depth in kbit; [psiReinject]
     * enables transport-level SPS/PPS re-injection ahead of every forwarded IDR.
     */
    fun start(shapeKbps: Int, shapeBurstK: Int, psiReinject: Boolean): Boolean {
        if (!load()) return false
        return nativeStart(shapeKbps, shapeBurstK, psiReinject)
    }

    /** Non-trickle offer (all ICE candidates gathered). */
    fun createOffer(): String = nativeCreateOffer()

    /** Apply the Linux receiver's answer; ICE/DTLS negotiation begins. */
    fun setRemoteAnswer(answer: String): Boolean = nativeSetRemoteAnswer(answer)

    /**
     * Push one encoded access unit. [ptsUs] is MediaCodec presentationTimeUs;
     * [key] marks access units carrying CSD/IDR so the Go queue treats them
     * as drop-protected.
     */
    fun pushFrame(ptsUs: Long, au: ByteArray, key: Boolean) = nativeOnFrame(ptsUs, au, key)

    /** JSON snapshot of engine counters (push/drop/send/queue/latency). */
    fun stats(): String = nativeStats()

    /** Close the peer connection and stop the send loop. */
    fun stop(): Boolean = nativeStop()

    // ------------------------------------------------------------------ natives

    private external fun nativeStart(shapeKbps: Int, shapeBurstK: Int, psiReinject: Boolean): Boolean
    private external fun nativeCreateOffer(): String
    private external fun nativeSetRemoteAnswer(answerSdp: String): Boolean

    /** Alternate direction (Android answers) — kept for symmetry experiments. */
    private external fun nativeOffer(offerSdp: String): String

    private external fun nativeOnFrame(ptsUs: Long, data: ByteArray, key: Boolean)
    private external fun nativeStats(): String
    private external fun nativeStop(): Boolean
}
