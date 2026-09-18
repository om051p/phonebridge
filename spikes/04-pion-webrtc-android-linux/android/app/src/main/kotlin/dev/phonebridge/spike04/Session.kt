package dev.phonebridge.spike04

import android.content.Context
import android.content.Intent
import android.graphics.PixelFormat
import android.hardware.display.DisplayManager
import android.hardware.display.VirtualDisplay
import android.media.MediaCodec
import android.media.MediaCodecInfo
import android.media.MediaCodecList
import android.media.MediaFormat
import android.media.projection.MediaProjection
import android.media.projection.MediaProjectionManager
import android.os.Build
import android.os.Bundle
import android.os.Handler
import android.view.Surface
import java.io.File
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale
import java.util.concurrent.atomic.AtomicBoolean

/** Session configuration, parsed from the launching Intent (host-driven). */
class SessionConfig(
    val label: String,
    val scenario: String,
    val mime: String,
    val width: Int,
    val height: Int,
    val fps: Int,
    val bitrate: Int,
    val bitrateMode: String,
    val keyIntervalSec: Int,
    /** Value written to KEY_FRAME_RATE. Drives GOP math on this encoder but does NOT
     *  throttle actual encoding (proven: KEY_FRAME_RATE=30 still encoded at ~120 fps).
     *  Defaults to [fps]. */
    val keyFrameRate: Int,
    /** GOP-tail keep count: contiguous AUs kept after each IDR. */
    val keepFrames: Int,
    val seconds: Int,
    /** Throttle strategy: "encoder" (KEY_MAX_FPS_TO_ENCODER), "gop" (drop-before-push with sync frames). */
    val throttle: String,
    val signalingUrl: String,
    val landscape: Boolean,
    /** Go-side burst shaper: sustained RTP send ceiling in kbps (0 = unshaped). */
    val shapeKbps: Int,
    /** Go-side burst shaper: token-bucket depth in kbit. */
    val shapeBurstK: Int,
    /** Go-side transport-level SPS/PPS re-injection ahead of every forwarded IDR. */
    val psiReinject: Boolean,
) {
    fun describe(): Map<String, Any?> = linkedMapOf(
        "label" to label,
        "scenario" to scenario,
        "mime" to mime,
        "width" to width,
        "height" to height,
        "fps" to fps,
        "bitrate_bps" to bitrate,
        "bitrate_mode_requested" to bitrateMode,
        "key_interval_s" to keyIntervalSec,
        "key_frame_rate" to keyFrameRate,
        "expected_gop_aus" to (keyFrameRate * keyIntervalSec),
        "keep_frames" to keepFrames,
        "seconds" to seconds,
        "throttle" to throttle,
        "signaling_url" to signalingUrl,
        "landscape" to landscape,
        "go_shape_kbps" to shapeKbps,
        "go_shape_burst_kbit" to shapeBurstK,
        "go_psi_reinject" to psiReinject,
    )

    companion object {
        const val E_SCENARIO = "scenario"
        const val E_LABEL = "label"
        const val E_MIME = "mime"
        const val E_WIDTH = "width"
        const val E_HEIGHT = "height"
        const val E_FPS = "fps"
        const val E_BITRATE = "bitrate"
        const val E_BITRATE_MODE = "bitrateMode"
        const val E_KEY_INT = "keyInterval"
        const val E_FRAME_RATE = "frameRate"
        const val E_KEEP_FRAMES = "keepFrames"
        const val E_SECONDS = "seconds"
        const val E_THROTTLE = "throttle"
        const val E_SIGNALING = "signaling"
        const val E_LANDSCAPE = "landscape"
        const val E_SHAPE_KBPS = "shapeKbps"
        const val E_SHAPE_BURST = "shapeBurstK"
        const val E_PSI_REINJECT = "psiReinject"

        fun fromIntent(i: Intent?): SessionConfig = SessionConfig(
            label = i?.getStringExtra(E_LABEL) ?: "default",
            scenario = i?.getStringExtra(E_SCENARIO) ?: "session",
            mime = i?.getStringExtra(E_MIME) ?: "video/avc",
            width = i?.getIntExtra(E_WIDTH, 720) ?: 720,
            height = i?.getIntExtra(E_HEIGHT, 1600) ?: 1600,
            fps = i?.getIntExtra(E_FPS, 30) ?: 30,
            bitrate = i?.getIntExtra(E_BITRATE, 2_500_000) ?: 2_500_000,
            bitrateMode = i?.getStringExtra(E_BITRATE_MODE) ?: "cbr",
            keyIntervalSec = i?.getIntExtra(E_KEY_INT, 2) ?: 2,
            keyFrameRate = i?.getIntExtra(E_FRAME_RATE, i?.getIntExtra(E_FPS, 30) ?: 30) ?: 30,
            keepFrames = i?.getIntExtra(E_KEEP_FRAMES, 15) ?: 15,
            seconds = i?.getIntExtra(E_SECONDS, 20) ?: 20,
            throttle = i?.getStringExtra(E_THROTTLE) ?: "encoder",
            signalingUrl = i?.getStringExtra(E_SIGNALING) ?: "",
            landscape = i?.getBooleanExtra(E_LANDSCAPE, false) ?: false,
            shapeKbps = i?.getIntExtra(E_SHAPE_KBPS, 0) ?: 0,
            shapeBurstK = i?.getIntExtra(E_SHAPE_BURST, 3000) ?: 3000,
            psiReinject = i?.getBooleanExtra(E_PSI_REINJECT, true) ?: true,
        )
    }
}

/** Writes evidence JSON where the host driver can `adb pull` it. */
object Results {
    fun stamp(): String = SimpleDateFormat("yyyyMMdd-HHmmss", Locale.US).format(Date())

    fun isoNow(): String = SimpleDateFormat("yyyy-MM-dd'T'HH:mm:ss.SSSZ", Locale.US).format(Date())

    fun write(ctx: Context, name: String, payload: Map<String, Any?>): String {
        val dir = File(ctx.getExternalFilesDir(null), "results").apply { mkdirs() }
        val f = File(dir, name)
        f.writeText(Json.encode(payload))
        return f.absolutePath
    }
}

/**
 * Spike 04 session: MediaProjection → VirtualDisplay → encoder Surface → H.264
 * → **JNI → Go/Pion** (the Spike 03 → Spike 04 delta).
 *
 * The capture core is the Spike 03 pattern (kept intact on purpose); what is new:
 *   - frame-rate throttling experiment: "encoder" = KEY_MAX_FPS_TO_ENCODER (the
 *     encoder discards input frames itself, so the H.264 predictive chain stays
 *     intact); "gop" = drop-before-push between IDR frames (predictive-chain-safe
 *     fallback for devices whose encoder ignores MAX_FPS_TO_ENCODER);
 *   - every access unit is handed to Go over the DEC-019-style JNI boundary
 *     (Annex-B, CSD prepended to the first AU, monotonic PTS in µs);
 *   - framing forensics: start-code census, NAL-type census of CSD and the first
 *     AU, SPS/PPS hex dumps — all logged so the run transcript settles unknowns
 *     #1 and #2 without pulling any binary.
 */
class Session(
    private val ctx: Context,
    private val cfg: SessionConfig,
    private val handler: Handler,
) {
    private val steps = mutableListOf<Map<String, Any?>>()
    private val errors = mutableListOf<Map<String, Any?>>()
    private val probes = mutableListOf<Map<String, Any?>>()
    private val notes = mutableListOf<String>()
    private val vdEvents = mutableListOf<Map<String, Any?>>()
    private val phases = linkedMapOf<String, Any?>()

    private var projection: MediaProjection? = null
    private var projectionCallback: MediaProjection.Callback? = null
    private var vd: VirtualDisplay? = null
    private var codec: MediaCodec? = null
    private var inputSurface: Surface? = null
    private var chosen: Caps.Candidate? = null
    private var selectedReason: String = ""
    private var bitrateMode: Int? = null
    @Volatile private var projectionStopped = false
    private var outputFormat: Map<String, Any?>? = null

    private val sessionStartMs = ProcStats.clockMs()
    private val bridgeReady = AtomicBoolean(false)

    // throttle experiment state
    private var throttleApplied: Map<String, Any?> = linkedMapOf()

    // JNI bridge
    private var goReady = false

    private fun step(name: String, outcome: String, detail: String = "", ms: Long = -1, extra: Map<String, Any?>? = null) {
        val m = linkedMapOf<String, Any?>("step" to name, "outcome" to outcome)
        if (detail.isNotEmpty()) m["detail"] = detail
        if (ms >= 0) m["ms"] = ms
        if (extra != null) m.putAll(extra)
        steps.add(m)
        S4Log.i("SPIKE04_STEP $name -> $outcome${if (detail.isEmpty()) "" else " | $detail"}")
    }

    private fun error(where: String, t: Throwable) {
        errors.add(linkedMapOf("where" to where, "at_ms" to (ProcStats.clockMs() - sessionStartMs), "error" to describe(t)))
        S4Log.e("SPIKE04_ERROR $where: ${describe(t)}")
    }

    private fun probe(name: String, atMs: Long, detail: String = "") {
        val m = linkedMapOf<String, Any?>("probe" to name, "at_ms" to atMs)
        if (detail.isNotEmpty()) m["detail"] = detail
        probes.add(m)
        S4Log.i("SPIKE04_PROBE $name @${atMs}ms $detail")
    }

    private fun describe(t: Throwable): String = "${t.javaClass.name}: ${t.message ?: "(no message)"}"

    private fun note(s: String) {
        notes.add(s)
        S4Log.i("SPIKE04_NOTE $s")
    }

    // ------------------------------------------------------------------ NAL forensics

    /** Start-code + NAL-type census of an Annex-B buffer (also detects AVCC-style length prefixes). */
    private fun nalCensus(buf: ByteArray, name: String) {
        if (buf.isEmpty()) { S4Log.i("SPIKE04_NAL $name empty"); return }
        var sc3 = 0; var sc4 = 0
        val types = linkedMapOf<Int, Int>()
        var i = 0
        val n = buf.size
        val starts = mutableListOf<Int>()
        while (i < n - 2) {
            if (buf[i] == 0.toByte() && buf[i + 1] == 0.toByte() && buf[i + 2] == 1.toByte()) {
                val payload = i + 3
                if (i > 0 && buf[i - 1] == 0.toByte()) sc4++ else sc3++
                starts.add(payload)
                if (payload < n) {
                    val t = (buf[payload].toInt() and 0x1F)
                    types[t] = (types[t] ?: 0) + 1
                }
                i += 2
            } else i++
        }
        val tail = buf.drop(maxOf(0, n - 4)).joinToString("") { String.format("%02x", it) }
        S4Log.i(
            "SPIKE04_NAL $name bytes=${buf.size} sc3=$sc3 sc4=$sc4 tail=$tail " +
                "types=" + types.entries.joinToString(",") { "${it.key}:${it.value}" },
        )
        if (name == "csd" || name == "first_au") {
            // hex-dump the first 96 bytes and every start-code boundary (≤ 24 NALs) for SPS/PPS analysis
            val hexHead = buf.take(96).joinToString("") { String.format("%02x", it) }
            S4Log.i("SPIKE04_HEX $name head=$hexHead")
            starts.take(24).forEachIndexed { k, p ->
                val len = (if (k + 1 < starts.size) starts[k + 1] else n) - p
                val nal = buf.drop(p).take(minOf(len - 1, 160))
                val hex = nal.joinToString("") { String.format("%02x", it) }
                val t = if (buf.isNotEmpty()) buf[p].toInt() and 0x1F else -1
                S4Log.i("SPIKE04_NALHEX $name[$k] type=$t len=${len - 1} $hex")
            }
        }
    }

    // ------------------------------------------------------------------ projection

    fun attachProjection(resultCode: Int, data: Intent): Boolean {
        val t0 = ProcStats.clockMs()
        return try {
            val mpm = ctx.getSystemService(Context.MEDIA_PROJECTION_SERVICE) as MediaProjectionManager
            val p = mpm.getMediaProjection(resultCode, data)
                ?: throw IllegalStateException("getMediaProjection returned null")
            projection = p
            val cb = object : MediaProjection.Callback() {
                override fun onStop() {
                    projectionStopped = true
                    S4Log.i("SPIKE04_EVENT projection_onStop")
                }
            }
            projectionCallback = cb
            p.registerCallback(cb, handler)
            step("projection.attach", "ok", "getMediaProjection + registerCallback", ProcStats.clockMs() - t0)
            true
        } catch (t: Throwable) {
            error("projection.attach", t)
            step("projection.attach", "failed", describe(t), ProcStats.clockMs() - t0)
            false
        }
    }

    // ------------------------------------------------------------------ encoder

    private fun pickBitrateMode(name: String): Int? {
        return try {
            val info = MediaCodecList(MediaCodecList.REGULAR_CODECS).codecInfos.firstOrNull { it.name == name }
            val eb = info?.getCapabilitiesForType(cfg.mime)?.encoderCapabilities ?: return null
            val want = cfg.bitrateMode.lowercase(Locale.US)
            val cbr = MediaCodecInfo.EncoderCapabilities.BITRATE_MODE_CBR
            val vbr = MediaCodecInfo.EncoderCapabilities.BITRATE_MODE_VBR
            when {
                want == "default" -> null
                want == "vbr" && eb.isBitrateModeSupported(vbr) -> vbr
                want == "cbr" && eb.isBitrateModeSupported(cbr) -> cbr
                eb.isBitrateModeSupported(cbr) -> cbr
                eb.isBitrateModeSupported(vbr) -> vbr
                else -> null
            }
        } catch (t: Throwable) { null }
    }

    private fun modeName(mode: Int?): String = when (mode) {
        null -> "default(unset)"
        MediaCodecInfo.EncoderCapabilities.BITRATE_MODE_CBR -> "CBR"
        MediaCodecInfo.EncoderCapabilities.BITRATE_MODE_VBR -> "VBR"
        MediaCodecInfo.EncoderCapabilities.BITRATE_MODE_CQ -> "CQ"
        else -> "mode$mode"
    }

    private fun buildFormat(codecName: String): MediaFormat {
        val f = MediaFormat.createVideoFormat(cfg.mime, cfg.width, cfg.height)
        f.setInteger(MediaFormat.KEY_COLOR_FORMAT, MediaCodecInfo.CodecCapabilities.COLOR_FormatSurface)
        f.setInteger(MediaFormat.KEY_BIT_RATE, cfg.bitrate)
        // KEY_FRAME_RATE is only a hint on this encoder: it does not throttle the
        // actual encode (which runs at the panel rate) but it IS multiplied into the
        // GOP length, so it is the lever for GOP retuning.
        f.setInteger(MediaFormat.KEY_FRAME_RATE, cfg.keyFrameRate)
        // Spike 03 finding: I-frame interval is frame-based, measured as
        // KEY_I_FRAME_INTERVAL * KEY_FRAME_RATE access units.
        f.setInteger(MediaFormat.KEY_I_FRAME_INTERVAL, cfg.keyIntervalSec)
        if (Build.VERSION.SDK_INT >= 23) f.setInteger(MediaFormat.KEY_PRIORITY, 0)
        val mode = pickBitrateMode(codecName)
        if (mode != null) {
            f.setInteger(MediaFormat.KEY_BITRATE_MODE, mode)
            bitrateMode = mode
        }
        return f
    }

    private fun formatToMap(f: MediaFormat): Map<String, Any?> {
        val out = linkedMapOf<String, Any?>()
        for (k in f.keys) {
            val v: Any? = runCatching { f.getInteger(k) as Any }
                .getOrElse { runCatching { f.getLong(k) as Any }
                    .getOrElse { runCatching { f.getFloat(k) as Any }
                        .getOrElse { runCatching { f.getString(k) as Any }.getOrNull() } } }
            out[k] = if (v is ByteArray) "csd(${v.size}B)" else v
        }
        return out
    }

    private fun createEncoder(tag: String = "encoder.start"): Boolean {
        val t0 = ProcStats.clockMs()
        if (chosen == null) {
            val (cand, reason) = Caps.select(cfg.mime, cfg.width, cfg.height, cfg.fps)
            if (cand == null) {
                step("$tag.select", "failed", reason)
                return false
            }
            chosen = cand
            selectedReason = reason
            step("encoder.select", "ok", "${cand.name} — $reason")
        }
        val name = chosen!!.name
        val format = buildFormat(name)
        // Throttle experiment, strategy "encoder": cap the encoder's input rate.
        // The encoder discards surplus input BEFORE encoding, so the H.264
        // predictive chain (P-frames referencing the latest kept frame) stays valid.
        if (cfg.throttle == "encoder" && Build.VERSION.SDK_INT >= 23) {
            format.setInteger(MediaFormat.KEY_MAX_FPS_TO_ENCODER, cfg.fps)
            throttleApplied = linkedMapOf("strategy" to "encoder", "key" to "KEY_MAX_FPS_TO_ENCODER", "fps" to cfg.fps)
            step("encoder.max_fps_to_encoder", "ok", "capped input at ${cfg.fps} fps")
        } else if (cfg.throttle == "stride" || cfg.throttle == "gop") {
            throttleApplied = linkedMapOf(
                "strategy" to "stride",
                "mechanism" to "drop-before-push, every Nth AU",
                "prediction_safe" to false,
                "keep_every" to keepEvery,
            )
        } else if (cfg.throttle == "tail") {
            throttleApplied = linkedMapOf(
                "strategy" to "tail",
                "mechanism" to "drop-before-push, contiguous prefix after each IDR",
                "prediction_safe" to true,
                "keep_frames_per_gop" to cfg.keepFrames,
                "key_frame_rate" to cfg.keyFrameRate,
                "i_frame_interval_s" to cfg.keyIntervalSec,
                "expected_gop_aus" to (cfg.keyFrameRate * cfg.keyIntervalSec),
            )
        } else if (cfg.throttle == "gopdrop") {
            throttleApplied = linkedMapOf(
                "strategy" to "gopdrop",
                "mechanism" to "drop-before-push, whole GOPs retained",
                "prediction_safe" to true,
                "keep_one_gop_in" to gopDropN,
            )
        } else if (cfg.throttle == "none") {
            throttleApplied = linkedMapOf("strategy" to "none", "mechanism" to "full rate reference")
        }
        var c: MediaCodec? = null
        try { c = MediaCodec.createByCodecName(name) } catch (t: Throwable) {
            error("$tag.create", t); step("$tag.create", "failed", describe(t)); return false
        }
        try { c.configure(format, null, null, MediaCodec.CONFIGURE_FLAG_ENCODE) } catch (t: Throwable) {
            error("$tag.configure", t); step("$tag.configure", "failed", describe(t))
            runCatching { c.release() }; return false
        }
        val s = try { c.createInputSurface() } catch (t: Throwable) {
            error("$tag.createInputSurface", t); step("$tag.createInputSurface", "failed", describe(t))
            runCatching { c.release() }; return false
        }
        try { c.start() } catch (t: Throwable) {
            error("$tag.start", t); step("$tag.start", "failed", describe(t))
            runCatching { s.release() }; runCatching { c.release() }; return false
        }
        codec = c
        inputSurface = s
        step(tag, "ok", "$name ${cfg.width}x${cfg.height}@${cfg.fps} ${cfg.bitrate / 1000}kbps mode=${modeName(bitrateMode)} throttle=${cfg.throttle}",
            ProcStats.clockMs() - t0, mapOf("format" to formatToMap(format)))
        return true
    }

    private fun createVirtualDisplay(surface: Surface): VirtualDisplay? {
        val p = projection ?: run { step("vd.create", "failed", "no projection"); return null }
        val dpi = ctx.resources.displayMetrics.densityDpi
        val t0 = ProcStats.clockMs()
        return try {
            val d = p.createVirtualDisplay(
                "spike04-vd", cfg.width, cfg.height, dpi,
                DisplayManager.VIRTUAL_DISPLAY_FLAG_AUTO_MIRROR,
                surface,
                object : VirtualDisplay.Callback() {
                    override fun onPaused() { vdEvents.add(mapOf("event" to "onPaused", "at_ms" to ProcStats.clockMs())); S4Log.i("SPIKE04_EVENT vd_onPaused") }
                    override fun onResumed() { vdEvents.add(mapOf("event" to "onResumed", "at_ms" to ProcStats.clockMs())); S4Log.i("SPIKE04_EVENT vd_onResumed") }
                    override fun onStopped() { vdEvents.add(mapOf("event" to "onStopped", "at_ms" to ProcStats.clockMs())); S4Log.i("SPIKE04_EVENT vd_onStopped") }
                },
                handler,
            )
            vd = d
            step("vd.create", "ok", "${cfg.width}x${cfg.height} dpi=$dpi displayId=${d.display?.displayId}", ProcStats.clockMs() - t0)
            d
        } catch (t: Throwable) {
            error("vd.create", t)
            step("vd.create", "failed", describe(t), ProcStats.clockMs() - t0)
            null
        }
    }

    // ------------------------------------------------------------------ Go bridge

    private fun startGoBridge(): Boolean {
        return try {
            goReady = GoBridge.start(cfg.shapeKbps, cfg.shapeBurstK, cfg.psiReinject)
            if (goReady) step("go.start", "ok", "Pion engine created (peer connection pending)" +
                    (if (cfg.shapeKbps > 0) ", shaper=${cfg.shapeKbps}kbps/burst${cfg.shapeBurstK}k" else ", unshaped") +
                    (if (cfg.psiReinject) ", psi=reinject" else ", psi=off"))
            else step("go.start", "failed", "GoBridge.start returned false")
            goReady
        } catch (t: Throwable) {
            error("go.start", t)
            step("go.start", "failed", describe(t))
            false
        }
    }

    /**
     * Spike-local signaling: POST our Pion offer to the Linux receiver, take the
     * answer back, hand it to Go (non-trickle ICE). No production protocol, no
     * server beyond the receiver's own HTTP endpoint.
     */
    private fun exchangeSignaling(): Boolean {
        if (cfg.signalingUrl.isEmpty()) {
            step("signaling", "skipped", "no signaling url (not streaming)")
            return false
        }
        val t0 = ProcStats.clockMs()
        return try {
            val offer = GoBridge.createOffer()
            S4Log.i("SPIKE04_SIGNAL offer_bytes=${offer.length}")
            val (code, body) = httpPostJson(cfg.signalingUrl, """{"sdp":${jsonQuote(offer)}}""")
            if (code != 200) {
                step("signaling", "failed", "HTTP $code: ${body.take(200)}")
                return false
            }
            val answer = extractSdp(body)
            if (answer.isEmpty()) {
                step("signaling", "failed", "no sdp in answer: ${body.take(200)}")
                return false
            }
            GoBridge.setRemoteAnswer(answer)
            step("signaling", "ok", "offer=${offer.length}B answer=${answer.length}B -> ${cfg.signalingUrl}", ProcStats.clockMs() - t0)
            true
        } catch (t: Throwable) {
            error("signaling", t)
            step("signaling", "failed", describe(t))
            false
        }
    }

    private fun httpPostJson(url: String, body: String): Pair<Int, String> {
        val conn = (java.net.URL(url).openConnection() as java.net.HttpURLConnection).apply {
            requestMethod = "POST"
            doOutput = true
            setRequestProperty("Content-Type", "application/json")
            connectTimeout = 5000
            readTimeout = 15000
        }
        conn.outputStream.use { it.write(body.toByteArray()) }
        val code = conn.responseCode
        val stream = if (code in 200..299) conn.inputStream else conn.errorStream
        val text = stream?.bufferedReader()?.readText() ?: ""
        return code to text
    }

    private fun jsonQuote(s: String): String {
        val sb = StringBuilder(s.length + 16)
        sb.append('"')
        for (c in s) {
            when (c) {
                '"' -> sb.append("\\\"")
                '\\' -> sb.append("\\\\")
                '\n' -> sb.append("\\n")
                '\r' -> sb.append("\\r")
                '\t' -> sb.append("\\t")
                else -> if (c < ' ') sb.append(String.format(Locale.US, "\\u%04x", c.code)) else sb.append(c)
            }
        }
        sb.append('"')
        return sb.toString()
    }

    private fun extractSdp(body: String): String {
        // minimal extraction of {"sdp":"..."} without a JSON parser dependency
        val key = "\"sdp\":\""
        val i = body.indexOf(key)
        if (i < 0) return ""
        val sb = StringBuilder()
        var j = i + key.length
        while (j < body.length) {
            val c = body[j]
            if (c == '\\' && j + 1 < body.length) {
                when (body[j + 1]) {
                    'n' -> sb.append('\n')
                    'r' -> sb.append('\r')
                    't' -> sb.append('\t')
                    '"' -> sb.append('"')
                    '\\' -> sb.append('\\')
                    else -> { sb.append(body[j + 1]) }
                }
                j += 2
            } else if (c == '"') {
                break
            } else {
                sb.append(c)
                j++
            }
        }
        return sb.toString()
    }

    fun build(status: String): Map<String, Any?> = linkedMapOf(
        "spike" to "04-pion-webrtc-android-linux",
        "scenario" to cfg.scenario,
        "status" to status,
        "generated_at" to Results.isoNow(),
        "total_elapsed_ms" to (ProcStats.clockMs() - sessionStartMs),
        "device" to Device.info(ctx),
        "config" to cfg.describe(),
        "codec_selected" to (chosen?.let {
            linkedMapOf<String, Any?>("name" to it.name, "hardware_accelerated" to it.hardware, "software_only" to it.softwareOnly, "selection_reason" to selectedReason)
        } ?: emptyMap<String, Any?>()),
        "steps" to steps,
        "probes" to probes,
        "errors" to errors,
        "notes" to notes,
        "phases" to phases,
        "virtual_display_events" to vdEvents,
        "go_bridge" to linkedMapOf(
            "loaded" to GoBridge.loaded,
            "used" to goReady,
            "stats_json" to if (goReady) GoBridge.stats() else "",
        ),
        "throttle" to throttleApplied,
        "output_format" to outputFormat,
    )

    // ------------------------------------------------------------------ run

    fun run(): Map<String, Any?> {
        val phasesLocal = phases
        val startedMs = ProcStats.clockMs()

        if (!startGoBridge()) {
            phasesLocal["abort"] = "go bridge unavailable"
            return finish("go_bridge_failed", startedMs)
        }

        val signaled = exchangeSignaling()

        val data = intentResultData
        if (data == null) {
            step("projection.attach", "failed", "consent data missing")
            return finish("consent_data_missing", startedMs)
        }
        if (!attachProjection(intentResultCode, data)) {
            return finish("projection_failed", startedMs)
        }
        if (!createEncoder()) {
            return finish("encoder_failed", startedMs)
        }
        val surface = inputSurface ?: return finish("no_surface", startedMs)
        if (createVirtualDisplay(surface) == null) {
            return finish("vd_failed", startedMs)
        }

        val capture = capturePhase()
        phasesLocal["capture"] = capture
        return finish(capture["status"] as? String ?: "unknown", startedMs)
    }

    private var intentResultCode: Int = 0
    private var intentResultData: Intent? = null

    fun setConsent(rc: Int, data: Intent?) {
        intentResultCode = rc
        intentResultData = data
    }

    private fun finish(status: String, startedMs: Long): Map<String, Any?> {
        phases["shutdown_ms"] = shutdown()
        val snap = GoBridge.stats()
        if (snap.isNotEmpty()) S4Log.i("SPIKE04_GOSTATS $snap")
        return linkedMapOf(
            "status" to status,
            "elapsed_ms" to (ProcStats.clockMs() - startedMs),
            "go_stats" to snap,
        )
    }

    private fun shutdown(): Long {
        val t0 = ProcStats.clockMs()
        try {
            inputSurface?.release()
            codec?.stop(); codec?.release()
            vd?.release()
            projection?.stop()
        } catch (t: Throwable) {
            error("shutdown", t)
        }
        try { GoBridge.stop() } catch (t: Throwable) { error("go.stop", t) }
        return ProcStats.clockMs() - t0
    }

    // ------------------------------------------------------------------ capture + push

    private fun capturePhase(): Map<String, Any?> {
        val c = codec ?: return mapOf("error" to "no codec")
        val m = linkedMapOf<String, Any?>()
        val info = MediaCodec.BufferInfo()
        val startMs = ProcStats.clockMs()
        val endMs = startMs + cfg.seconds * 1000L
        val latencies = mutableListOf<Double>()
        val buckets = mutableListOf<Map<String, Any?>>()
        val drainErrors = mutableListOf<Map<String, Any?>>()
        val pushSendSamples = mutableListOf<Double>()
        var frames = 0L
        var bytes = 0L
        var keyframes = 0L
        var pushedAUs = 0L
        var csdBytes = 0L
        var lastFrameMs = 0L
        var firstFrameMs = -1L
        var negativeLatency = 0
        var ptsFirstMs = -1.0
        var ptsLastMs = 0.0
        var csdSeen = false
        var bucketStart = startMs
        var bFrames = 0L; var bBytes = 0L; var bPushed = 0L

        val cpuStart = ProcStats.appCpuMs()
        val pssStart = ProcStats.pss()
        val rssStart = ProcStats.rssKb()

        fun emitBucket(nowMs: Long) {
            val sec = (nowMs - bucketStart) / 1000.0
            if (sec <= 0.0) return
            buckets.add(linkedMapOf(
                "t_ms" to (bucketStart - startMs),
                "frames" to bFrames,
                "fps" to bFrames / sec,
                "pushed" to bPushed,
                "bytes" to bBytes,
                "kbps" to bBytes * 8 / 1000.0 / sec,
            ))
            bucketStart = nowMs
            bFrames = 0; bBytes = 0; bPushed = 0
        }

        // gop-throttle state: drop-before-push between IDR frames
        var sinceKey = 0

        while (true) {
            val now = ProcStats.clockMs()
            if (now >= endMs || projectionStopped) break
            val idx = try { c.dequeueOutputBuffer(info, 10_000) } catch (t: Throwable) {
                drainErrors.add(mapOf("at_ms" to (now - startMs), "error" to describe(t)))
                S4Log.e("SPIKE04_ERROR dequeue: ${describe(t)}")
                break
            }
            val now2 = ProcStats.clockMs()
            if (idx == MediaCodec.INFO_OUTPUT_FORMAT_CHANGED) {
                outputFormat = formatToMap(c.outputFormat)
                S4Log.i("SPIKE04_EVENT output_format_changed")
            } else if (idx >= 0 && info.size > 0) {
                val isCsd = info.flags and MediaCodec.BUFFER_FLAG_CODEC_CONFIG != 0
                val isKey = info.flags and MediaCodec.BUFFER_FLAG_KEY_FRAME != 0
                val buf = ByteArray(info.size)
                c.getOutputBuffer(idx)?.let { it.position(info.offset); it.get(buf) }

                if (isCsd) {
                    csdSeen = true
                    csdBytes += info.size
                    nalCensus(buf, "csd")
                    S4Log.i("SPIKE04_CSD bytes=${info.size} base64=${android.util.Base64.encodeToString(buf, android.util.Base64.NO_WRAP)}")
                    // hold CSD; prepend to the first AU we push
                    pendingCsd = buf
                } else {
                    frames++
                    bytes += info.size
                    val latMs = (System.nanoTime() - info.presentationTimeUs * 1000L) / 1e6
                    val ptsMs = info.presentationTimeUs / 1000.0
                    if (ptsFirstMs < 0) ptsFirstMs = ptsMs
                    ptsLastMs = ptsMs
                    if (latMs < -5) negativeLatency++
                    if (latencies.size < 20_000) latencies.add(latMs)
                    if (firstFrameMs < 0) firstFrameMs = now2 - startMs
                    if (isKey && lastKeyAtMs > 0) {
                        // keyframe interval in *measured* time
                        keyIntervals.add((now2 - lastKeyAtMs).toDouble())
                        lastKeyAtMs = now2
                    }
                    if (isKey) {
                        keyframes++
                        sinceKey = 0
                        gopIndex++
                        lastKeyAtMs = now2
                    } else {
                        sinceKey++
                    }

                    val hadCsd = pendingCsd != null
                    var au = buf
                    if (pendingCsd != null) {
                        au = ByteArray(pendingCsd!!.size + buf.size)
                        pendingCsd!!.copyInto(au, 0)
                        buf.copyInto(au, pendingCsd!!.size)
                        nalCensus(au, "first_au")
                        pendingCsd = null
                    }
                    if (frames == 1L) nalCensus(buf, "first_plain_au")

                    var pushed = true
                    when (cfg.throttle) {
                        // NAIVE STRIDE: keep the IDR and every Nth AU after it, drop the rest
                        // BEFORE push. Rate-effective, but it drops P-frames whose references
                        // were themselves dropped, so the predictive chain is broken by
                        // construction. Kept only as the negative control.
                        "stride", "gop" -> {
                            pushed = isKey || (sinceKey % keepEvery == 0)
                            if (!pushed) gopDropped++
                        }
                        // GOP-TAIL: keep a contiguous prefix after each IDR and drop the tail
                        // of the GOP. Every kept frame references only earlier KEPT frames
                        // (IDR .. sinceKey-1), so the predictive chain stays intact. This works
                        // because the stream is IPPP (no B-frames) and each GOP is IDR-anchored.
                        "tail" -> {
                            pushed = isKey || (sinceKey < cfg.keepFrames)
                            if (!pushed) gopDropped++
                        }
                        // WHOLE-GOP: retain complete GOPs at a reduced cadence. Also
                        // prediction-safe (each retained GOP is self-contained), but delivery
                        // is bursty at GOP granularity instead of smooth.
                        "gopdrop" -> {
                            pushed = gopIndex % gopDropN == 1L
                            if (!pushed) gopDropped++
                        }
                        else -> { /* none / encoder: push every AU */ }
                    }
                    if (pushed) {
                        GoBridge.pushFrame(info.presentationTimeUs, au, isKey || hadCsd)
                        pushedAUs++
                        bPushed++
                    }
                    bFrames++
                    bBytes += info.size
                }
                c.releaseOutputBuffer(idx, false)
            }

            if (now2 - bucketStart >= 1000) emitBucket(now2)
        }
        emitBucket(ProcStats.clockMs())

        val elapsedSec = (ProcStats.clockMs() - startMs) / 1000.0
        val cpuEnd = ProcStats.appCpuMs()
        m["elapsed_s"] = elapsedSec
        m["status"] = if (projectionStopped) "projection_stopped" else "ok"
        m["frames"] = frames
        m["fps_avg"] = if (elapsedSec > 0) frames / elapsedSec else 0.0
        m["bytes_total"] = bytes
        m["bitrate_kbps_avg"] = if (elapsedSec > 0) bytes * 8 / 1000.0 / elapsedSec else 0.0
        m["keyframes"] = keyframes
        m["csd_seen"] = csdSeen
        m["csd_bytes"] = csdBytes
        m["pushed_aus"] = pushedAUs
        m["gop_dropped"] = gopDropped
        m["first_frame_ms"] = firstFrameMs
        m["latency_proxy_ms"] = Stats.summary(latencies)
        m["negative_latency_samples"] = negativeLatency
        m["pts"] = linkedMapOf(
            "first_ms" to ptsFirstMs,
            "last_ms" to ptsLastMs,
            "clock" to "CLOCK_MONOTONIC microseconds (System.nanoTime timebase)",
        )
        m["drain_errors"] = drainErrors
        m["buckets"] = buckets
        m["output_format"] = outputFormat
        m["cpu"] = linkedMapOf(
            "app_cpu_ms" to (cpuEnd - cpuStart),
            "app_cpu_pct_of_one_core" to if (elapsedSec > 0) (cpuEnd - cpuStart) / (elapsedSec * 1000.0) * 100.0 else null,
        )
        m["memory"] = linkedMapOf(
            "pss_before_kb" to pssStart,
            "pss_after_kb" to ProcStats.pss(),
            "rss_before_kb" to (rssStart ?: -1),
            "rss_after_kb" to (ProcStats.rssKb() ?: -1),
        )
        m["throttle_applied"] = throttleApplied
        return m
    }

    private val keepEvery = 4 // stride throttle: keep IDR + every 4th frame after it
    private val gopDropN = 4 // gopdrop throttle: keep one whole GOP in every N (tail keep count comes from cfg)
    private var gopIndex = 0L
    private var pendingCsd: ByteArray? = null
    private var lastKeyAtMs = 0L
    private val keyIntervals = mutableListOf<Double>()
    private var gopDropped = 0L
}
