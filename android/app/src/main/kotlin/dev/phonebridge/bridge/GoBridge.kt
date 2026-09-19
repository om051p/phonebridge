package dev.phonebridge.bridge

/**
 * GoBridge is the in-process JNI gateway between Android (Kotlin) and the Go core engine.
 * Ratified under DEC-019 (Spike 02).
 *
 * Two planes (Step 3):
 *  - Control plane: [invoke] stays the generic request/response path; frame data
 *    NEVER routes through it.
 *  - Media data plane: [mediaOnFrame] is the dedicated high-throughput JNI entry
 *    point feeding the production Go WebRTC transport (DEC-020/021 pipeline:
 *    bounded queue -> PSI -> RFC 6184 packetizer -> shaper -> Pion track).
 *
 * Media lifecycle (mirrors the Go state machine in core/cmd/android/transport.go):
 *
 *     idle --mediaInit--> initialized --mediaCreateOffer/mediaSetAnswer--> negotiated
 *          --mediaStart--> streaming --mediaStop--> stopped --mediaRelease--> idle
 *
 * Semantics guaranteed by the Go side:
 *  - Calls before readiness are safe no-ops with documented returns
 *    ([mediaOnFrame] returns false = frame not admitted); misuse errors throw
 *    IllegalStateException from JNI (DEC-019 rule).
 *  - Backpressure: [mediaOnFrame] never blocks. The bounded AU queue (256)
 *    drops non-key frames when full (returns false) and evicts oldest frames
 *    to admit keyframes (the decoder's only re-entry point).
 *  - [mediaStop] discards queued frames by design (teardown owes nothing).
 *  - AUs are Annex-B H.264 access units with 4-byte start codes; the buffer is
 *    copied across the boundary and may be reused by the caller on return.
 */
object GoBridge {
    private var isLoaded = false

    init {
        tryLoad()
    }

    private fun tryLoad() {
        try {
            System.loadLibrary("phonebridge_core")
            isLoaded = true
        } catch (e: UnsatisfiedLinkError) {
            // Before the .so is placed in jniLibs (headless unit-tests, host
            // JVM harness) the load fails gracefully; [loaded] retries, so a
            // later System.load of the library by absolute path still heals
            // the state (the JVM dedups by soname).
            isLoaded = false
        }
    }

    val loaded: Boolean
        get() {
            if (isLoaded) return true
            tryLoad() // self-healing: a test harness may have pre-loaded the natives
            return isLoaded
        }

    fun start(storageDir: String? = null): Boolean {
        check(isLoaded) { "libphonebridge_core.so is not loaded" }
        return nativeStart(storageDir)
    }

    fun stop(): Boolean {
        check(isLoaded) { "libphonebridge_core.so is not loaded" }
        return nativeStop()
    }

    fun invoke(method: String, payload: ByteArray? = null): ByteArray? {
        check(isLoaded) { "libphonebridge_core.so is not loaded" }
        return nativeInvoke(method, payload)
    }

    fun subscribe(listener: EventListener, count: Int, intervalMs: Int): Boolean {
        check(isLoaded) { "libphonebridge_core.so is not loaded" }
        return nativeSubscribe(listener, count, intervalMs)
    }

    fun trimMemory(level: Int) {
        if (isLoaded) {
            nativeTrimMemory(level)
        }
    }

    // ------------------------------------------------------------------
    // Media transport (Step 3, DEC-020/021). One-shot per session:
    // init -> offer/answer -> start -> (onFrame)* -> stop -> release.
    // ------------------------------------------------------------------

    /** Builds the sender + Pion PeerConnection. Throws on double init. */
    fun mediaInit() {
        check(isLoaded) { "libphonebridge_core.so is not loaded" }
        nativeMediaInit()
    }

    /**
     * Creates the SDP offer (non-trickle; blocks up to ~2 s for ICE
     * gathering). Returns the JSON `{"type":"offer","sdp":"..."}` blob —
     * the same wire shape Spike 04 used. The phone is the offerer.
     */
    fun mediaCreateOffer(): ByteArray {
        check(isLoaded) { "libphonebridge_core.so is not loaded" }
        val out = nativeMediaCreateOffer() ?: error("nativeMediaCreateOffer returned null (pending JNI exception)")
        return out
    }

    /** Applies the remote answer (JSON `{"type":"answer","sdp":"..."}`). */
    fun mediaSetAnswer(answerJson: ByteArray) {
        check(isLoaded) { "libphonebridge_core.so is not loaded" }
        require(answerJson.isNotEmpty()) { "answer JSON is empty" }
        nativeMediaSetAnswer(answerJson)
    }

    /**
     * Starts the single-writer send loop. Call only after the answer is
     * applied and the connection is established; idempotent while streaming.
     */
    fun mediaStart() {
        check(isLoaded) { "libphonebridge_core.so is not loaded" }
        nativeMediaStart()
    }

    /**
     * Hot path: hands one Annex-B H.264 access unit to the Go pipeline.
     * Never blocks. Returns true when the AU was admitted (queued or sent),
     * false when it was dropped (queue full, non-key) or the transport is
     * not initialized. The array is copied; it may be reused on return.
     */
    fun mediaOnFrame(ptsUs: Long, au: ByteArray, keyframe: Boolean): Boolean {
        check(isLoaded) { "libphonebridge_core.so is not loaded" }
        if (au.isEmpty()) return false
        return nativeMediaOnFrame(ptsUs, au, keyframe)
    }

    /**
     * Tells the peer that this device hit a typed, sender-side failure
     * (DEC-022), so it can classify the cause instead of inferring one from a
     * stream that stops.
     *
     * Only meaningful while a transport is negotiated: before that there is no
     * control channel, and the failure belongs in the signalling answer.
     * Returns false when the report could not be sent — the caller is already
     * on an error path, so a failed report is logged rather than escalated.
     */
    fun mediaReportSessionError(code: String, message: String): Boolean {
        if (!isLoaded) return false
        if (code.isEmpty()) return false
        return nativeMediaReportSessionError(code, message)
    }

    /**
     * Stops the stream: discards queued frames, stops the writer, closes the
     * PeerConnection. Idempotent and safe before init.
     */
    fun mediaStop() {
        if (isLoaded) {
            nativeMediaStop()
        }
    }

    /** Returns to idle so a fresh session can be built. Idempotent. */
    fun mediaRelease() {
        if (isLoaded) {
            nativeMediaRelease()
        }
    }

    /**
     * Diagnostics snapshot (JSON): transport/PC state, pushed/dropped/sent
     * AU counts, reinjected IDRs, send errors, worst push→send latency.
     * Counters survive [mediaStop].
     */
    fun mediaStats(): ByteArray? {
        if (!isLoaded) return null
        return nativeMediaStats()
    }

    // ------------------------------------------------------------------
    // Clipboard plane (DEC-023, Phase 3 Step 4).
    // Go clipboard.Engine acts as single synchronization authority.
    // ------------------------------------------------------------------

    fun clipboardInit(callback: ClipboardHostCallback): Boolean {
        check(isLoaded) { "libphonebridge_core.so is not loaded" }
        return nativeClipboardInit(callback)
    }

    fun clipboardStop() {
        if (isLoaded) {
            nativeClipboardStop()
        }
    }

    fun clipboardOnLocalCopy(mimeType: String, payload: ByteArray, copiedAtMs: Long): Boolean {
        if (!isLoaded) return false
        return nativeClipboardOnLocalCopy(mimeType, payload, copiedAtMs)
    }

    fun clipboardOnRemoteBytes(payload: ByteArray): Boolean {
        if (!isLoaded) return false
        return nativeClipboardOnRemoteBytes(payload)
    }

    fun clipboardStats(): ByteArray? {
        if (!isLoaded) return null
        return nativeClipboardStats()
    }

    @JvmStatic
    private external fun nativeStart(storageDir: String?): Boolean

    @JvmStatic
    private external fun nativeStop(): Boolean

    @JvmStatic
    private external fun nativeInvoke(method: String, payload: ByteArray?): ByteArray?

    @JvmStatic
    private external fun nativeSubscribe(listener: EventListener, count: Int, intervalMs: Int): Boolean

    @JvmStatic
    private external fun nativeTrimMemory(level: Int)

    // Media plane natives (implemented in core/cmd/android/main.go; names
    // AND return types must match the exported JNI symbols exactly).
    @JvmStatic
    private external fun nativeMediaInit(): Boolean

    @JvmStatic
    private external fun nativeMediaCreateOffer(): ByteArray?

    @JvmStatic
    private external fun nativeMediaSetAnswer(answerJson: ByteArray): Boolean

    @JvmStatic
    private external fun nativeMediaStart(): Boolean

    @JvmStatic
    private external fun nativeMediaOnFrame(ptsUs: Long, au: ByteArray, keyframe: Boolean): Boolean

    @JvmStatic
    private external fun nativeMediaReportSessionError(code: String, message: String): Boolean

    @JvmStatic
    private external fun nativeMediaStop()

    @JvmStatic
    private external fun nativeMediaRelease()

    @JvmStatic
    private external fun nativeMediaStats(): ByteArray?

    // Clipboard plane natives (implemented in core/cmd/android/main.go)
    @JvmStatic
    private external fun nativeClipboardInit(callback: ClipboardHostCallback): Boolean

    @JvmStatic
    private external fun nativeClipboardStop()

    @JvmStatic
    private external fun nativeClipboardOnLocalCopy(mimeType: String, payload: ByteArray, copiedAtMs: Long): Boolean

    @JvmStatic
    private external fun nativeClipboardOnRemoteBytes(payload: ByteArray): Boolean

    @JvmStatic
    private external fun nativeClipboardStats(): ByteArray?
}

interface ClipboardHostCallback {
    fun onWritePlatformClipboard(mimeType: String, payload: ByteArray): Boolean
    fun onSendClipboardUpdate(payload: ByteArray): Boolean
    fun onOversizedPayload(size: Int)
}

