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

    // ------------------------------------------------------------------
    // File transfer plane (DEC-024, Phase 4 Step 4).
    // Kotlin owns storage (MediaStore IS_PENDING / direct Downloads fallback);
    // Go writes bytes through the descriptor the host hands over and asks the
    // host to publish or delete the entry. init/stop register and release the
    // host; send/cancel/list/stats ride the generic invoke("transfer:*")
    // control plane, because they are low-frequency request/response calls.
    // ------------------------------------------------------------------

    /**
     * Registers [host] as the storage side of the transfer plane and builds
     * the Go engine. [peerDeviceId] attributes subsequent transfers in the
     * activity history (empty until a session targets a device).
     */
    fun transferInit(host: TransferHostCallback, peerDeviceId: String): Boolean {
        check(isLoaded) { "libphonebridge_core.so is not loaded" }
        return nativeTransferInit(host, peerDeviceId)
    }

    /** Releases the host and interrupts in-flight transfers (DEC-024: no resume). */
    fun transferStop() {
        if (isLoaded) {
            nativeTransferStop()
        }
    }

    /** Records the signaling target so transfer history shows who sent what. */
    fun transferSetPeer(peerDeviceId: String): Boolean {
        if (!isLoaded) return false
        return nativeTransferSetPeer(peerDeviceId)
    }

    /** Offers a local file to the peer; returns JSON {transfer_id|error,reason}. */
    fun transferSend(path: String, filename: String? = null): ByteArray? {
        if (!isLoaded) return null
        val body = buildString {
            append("{")
            append("\"path\":\"").append(path.replace("\\", "\\\\").replace("\"", "\\\"")).append("\"")
            if (!filename.isNullOrEmpty()) {
                append(",\"filename\":\"").append(filename.replace("\\", "\\\\").replace("\"", "\\\"")).append("\"")
            }
            append("}")
        }
        return invoke("transfer:send", body.toByteArray(Charsets.UTF_8))
    }

    /** Cancels an in-flight transfer; returns JSON {cancelled|error}. */
    fun transferCancel(transferId: String): ByteArray? {
        if (!isLoaded) return null
        return invoke("transfer:cancel", "{\"transfer_id\":\"$transferId\"}".toByteArray(Charsets.UTF_8))
    }

    /** Lists in-flight transfers plus history as a JSON array. */
    fun transferList(): ByteArray? {
        if (!isLoaded) return null
        return invoke("transfer:list")
    }

    /** Returns the diagnostic snapshot as JSON. */
    fun transferStats(): ByteArray? {
        if (!isLoaded) return null
        return invoke("transfer:stats")
    }

    // ------------------------------------------------------------------
    // Remote Input plane (DEC-027, Phase 7 v0.1).
    // Dispatches normalized pointer/touch, key, text, scroll, and global
    // actions received from Linux desktop WebRTC "input" DataChannel.
    // ------------------------------------------------------------------

    fun inputInit(host: InputHostCallback): Boolean {
        check(isLoaded) { "libphonebridge_core.so is not loaded" }
        return nativeInputInit(host)
    }

    fun inputStop() {
        if (isLoaded) {
            nativeInputStop()
        }
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

    // Clipboard plane natives (implemented in core/cmd/android/transfer_jni.go)
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

    // Transfer plane natives (implemented in core/cmd/android/transfer_jni.go;
    // names AND return types must match the exported JNI symbols exactly).
    @JvmStatic
    private external fun nativeTransferInit(host: TransferHostCallback, peerDeviceId: String): Boolean

    @JvmStatic
    private external fun nativeTransferStop()

    @JvmStatic
    private external fun nativeTransferSetPeer(peerDeviceId: String): Boolean

    // Input plane natives (implemented in core/cmd/android/input_jni.go)
    @JvmStatic
    private external fun nativeInputInit(host: InputHostCallback): Boolean

    @JvmStatic
    private external fun nativeInputStop()
}

/**
 * InputHostCallback receives remote input dispatched from the Go input bridge (DEC-027).
 * Coordinates are normalized in [0.0, 1.0].
 * Absolute prohibition: NEVER log raw coordinates, keystrokes, or text.
 */
interface InputHostCallback {
    fun onTouch(action: Int, pointerId: Int, normX: Float, normY: Float, pressure: Float): Boolean
    fun onKey(action: Int, keyCode: Int, metaState: Int): Boolean
    fun onText(text: String): Boolean
    fun onScroll(normX: Float, normY: Float, deltaX: Float, deltaY: Float): Boolean
    fun onGlobalAction(actionType: Int): Boolean
}

/**
 * TransferHostCallback is the storage side of the file-transfer plane (DEC-024).
 * Kotlin owns where a received file lands (MediaStore IS_PENDING on API 29+,
 * direct Downloads writes on 26-28); Go writes bytes through the descriptor
 * [onOpenPendingFd] returns and never loads the whole file into memory.
 *
 * Method names and signatures are pinned by the JNI glue in
 * core/cmd/android/transfer_jni.go — renaming either side breaks the plane.
 */
interface TransferHostCallback {
    /**
     * Creates the pending destination entry and returns its handle (an opaque
     * string), or null when the platform refuses the download.
     */
    fun onBeginDownload(filename: String, mimeType: String, sizeBytes: Long): String?

    /**
     * Opens the pending entry for writing and returns a ParcelFileDescriptor
     * detaching an int fd. The Go side dups the descriptor and owns its copy;
     * this fd must be closed here as soon as the call returns.
     */
    fun onOpenPendingFd(handle: String): Int

    /**
     * Publishes the finished file (IS_PENDING=0) and returns the user-visible
     * display name, or null when the publish failed (the Go side then asks for
     * the entry to be deleted).
     */
    fun onCommitDownload(handle: String): String?

    /** Deletes the pending entry and everything written to it. Must be idempotent. */
    fun onAbortDownload(handle: String)

    /**
     * Free space on the destination volume, or a negative value when unknown
     * (the Go free-space policy is then skipped rather than guessed).
     */
    fun onFreeSpaceBytes(): Long

    /** Reports a frame larger than the protocol limit (peer misbehaviour). */
    fun onOversizedFrame(size: Int)
}

interface ClipboardHostCallback {
    fun onWritePlatformClipboard(mimeType: String, payload: ByteArray): Boolean
    fun onSendClipboardUpdate(payload: ByteArray): Boolean
    fun onOversizedPayload(size: Int)
}

