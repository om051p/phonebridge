package dev.phonebridge.spike03

import android.content.Context
import android.content.Intent
import android.graphics.PixelFormat
import android.hardware.display.DisplayManager
import android.hardware.display.VirtualDisplay
import android.media.ImageReader
import android.media.MediaCodec
import android.media.MediaCodecInfo
import android.media.MediaCodecList
import android.media.MediaFormat
import android.media.projection.MediaProjection
import android.media.projection.MediaProjectionManager
import android.os.Build
import android.os.Bundle
import android.os.Handler
import android.os.SystemClock
import android.view.Surface
import java.io.File
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale

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
    val seconds: Int,
    val frameRateHint: Boolean,
    val frameRateHintAlways: Boolean,
    val syncProbe: Boolean,
    val secureProbe: Boolean,
    val backgroundProbe: Boolean,
    val staticProbe: Boolean,
    val bitrateProbe: Boolean,
    val cycles: Int,
    val landscape: Boolean,
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
        "seconds" to seconds,
        "frame_rate_hint" to frameRateHint,
        "frame_rate_hint_always" to frameRateHintAlways,
        "sync_frame_probe" to syncProbe,
        "flag_secure_probe" to secureProbe,
        "background_probe" to backgroundProbe,
        "static_content_probe" to staticProbe,
        "bitrate_change_probe" to bitrateProbe,
        "encoder_cycles" to cycles,
        "landscape" to landscape,
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
        const val E_SECONDS = "seconds"
        const val E_FRAME_RATE_HINT = "frameRateHint"
        const val E_FRAME_RATE_HINT_ALWAYS = "frameRateHintAlways"
        const val E_SYNC_PROBE = "syncProbe"
        const val E_SECURE_PROBE = "secureProbe"
        const val E_BACKGROUND_PROBE = "backgroundProbe"
        const val E_STATIC_PROBE = "staticProbe"
        const val E_BITRATE_PROBE = "bitrateProbe"
        const val E_CYCLES = "cycles"
        const val E_LANDSCAPE = "landscape"

        fun fromIntent(i: Intent?): SessionConfig = SessionConfig(
            label = i?.getStringExtra(E_LABEL) ?: "default",
            scenario = i?.getStringExtra(E_SCENARIO) ?: "session",
            mime = i?.getStringExtra(E_MIME) ?: "video/avc",
            width = i?.getIntExtra(E_WIDTH, 1080) ?: 1080,
            height = i?.getIntExtra(E_HEIGHT, 2400) ?: 2400,
            fps = i?.getIntExtra(E_FPS, 30) ?: 30,
            bitrate = i?.getIntExtra(E_BITRATE, 6_000_000) ?: 6_000_000,
            bitrateMode = i?.getStringExtra(E_BITRATE_MODE) ?: "cbr",
            keyIntervalSec = i?.getIntExtra(E_KEY_INT, 1) ?: 1,
            seconds = i?.getIntExtra(E_SECONDS, 10) ?: 10,
            frameRateHint = i?.getBooleanExtra(E_FRAME_RATE_HINT, false) ?: false,
            frameRateHintAlways = i?.getBooleanExtra(E_FRAME_RATE_HINT_ALWAYS, false) ?: false,
            syncProbe = i?.getBooleanExtra(E_SYNC_PROBE, false) ?: false,
            secureProbe = i?.getBooleanExtra(E_SECURE_PROBE, false) ?: false,
            backgroundProbe = i?.getBooleanExtra(E_BACKGROUND_PROBE, false) ?: false,
            staticProbe = i?.getBooleanExtra(E_STATIC_PROBE, false) ?: false,
            bitrateProbe = i?.getBooleanExtra(E_BITRATE_PROBE, false) ?: false,
            cycles = i?.getIntExtra(E_CYCLES, 0) ?: 0,
            landscape = i?.getBooleanExtra(E_LANDSCAPE, false) ?: false,
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
 * One capture session: MediaProjection → VirtualDisplay → (MediaCodec input) Surface → H.264.
 *
 * Every step is timed, every failure captured, and the raw evidence is emitted as JSON.
 * Nothing here is production code.
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
    private var lastCapture: Map<String, Any?> = linkedMapOf()
    private val sessionStartMs = ProcStats.clockMs()

    // probe timeline (ms since capture start)
    private var syncReqAt = -1L
    private var syncKeyAt = -1L
    private var secureOnAt = -1L
    private var secureOffAt = -1L
    private var backgroundAt = -1L
    private var staticOnAt = -1L
    private var staticOffAt = -1L
    private var bitrateChangedAt = -1L

    private var resultCodeSaved = 0
    private var resultDataSaved: Intent? = null

    // ---------------------------------------------------------------- plumbing

    private fun step(
        name: String,
        outcome: String,
        detail: String = "",
        ms: Long = -1,
        extra: Map<String, Any?>? = null,
    ) {
        val m = linkedMapOf<String, Any?>("step" to name, "outcome" to outcome)
        if (detail.isNotEmpty()) m["detail"] = detail
        if (ms >= 0) m["ms"] = ms
        if (extra != null) m.putAll(extra)
        steps.add(m)
        S3Log.i("SPIKE03_STEP $name -> $outcome${if (detail.isEmpty()) "" else " | $detail"}")
    }

    private fun error(where: String, t: Throwable) {
        errors.add(linkedMapOf("where" to where, "at_ms" to (ProcStats.clockMs() - sessionStartMs), "error" to describe(t)))
        S3Log.e("SPIKE03_ERROR $where: ${describe(t)}")
    }

    private fun probe(name: String, atMs: Long, detail: String = "") {
        val m = linkedMapOf<String, Any?>("probe" to name, "at_ms" to atMs)
        if (detail.isNotEmpty()) m["detail"] = detail
        probes.add(m)
        S3Log.i("SPIKE03_PROBE $name @${atMs}ms $detail")
    }

    private fun describe(t: Throwable): String =
        "${t.javaClass.name}: ${t.message ?: "(no message)"}"

    private fun note(s: String) {
        notes.add(s)
        S3Log.i("SPIKE03_NOTE $s")
    }

    // ------------------------------------------------------------- projection

    fun attachProjection(resultCode: Int, data: Intent): Boolean {
        resultCodeSaved = resultCode
        resultDataSaved = data
        val t0 = ProcStats.clockMs()
        return try {
            val mpm = ctx.getSystemService(Context.MEDIA_PROJECTION_SERVICE) as MediaProjectionManager
            val p = mpm.getMediaProjection(resultCode, data)
                ?: throw IllegalStateException("getMediaProjection returned null")
            projection = p
            val cb = object : MediaProjection.Callback() {
                override fun onStop() {
                    projectionStopped = true
                    S3Log.i("SPIKE03_EVENT projection_onStop")
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

    // ---------------------------------------------------------------- encoder

    /**
     * Bitrate mode is configurable because it changes what the stream tells us:
     * CBR (streaming realism) holds bytes/frame roughly constant regardless of
     * content, while VBR lets content changes (e.g. FLAG_SECURE black frames)
     * show up as bitrate changes.
     */
    private fun pickBitrateMode(name: String): Int? {
        return try {
            val info = MediaCodecList(MediaCodecList.REGULAR_CODECS).codecInfos.firstOrNull { it.name == name }
            val eb = info?.getCapabilitiesForType(cfg.mime)?.encoderCapabilities
            if (eb == null) return null
            val want = cfg.bitrateMode.lowercase(Locale.US)
            fun supports(m: Int) = eb.isBitrateModeSupported(m)
            val cbr = MediaCodecInfo.EncoderCapabilities.BITRATE_MODE_CBR
            val vbr = MediaCodecInfo.EncoderCapabilities.BITRATE_MODE_VBR
            when {
                want == "default" -> null
                want == "vbr" && supports(vbr) -> vbr
                want == "cbr" && supports(cbr) -> cbr
                supports(cbr) -> cbr
                supports(vbr) -> vbr
                else -> null
            }
        } catch (t: Throwable) {
            null
        }
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
        f.setInteger(MediaFormat.KEY_FRAME_RATE, cfg.fps)
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

    /** Creates + configures + starts an encoder; installs it as the session codec. */
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
            step(
                "encoder.select", "ok", "${cand.name} — $reason", -1,
                mapOf("codec" to cand.name, "hardware" to cand.hardware, "software_only" to cand.softwareOnly),
            )
        }
        val name = chosen!!.name
        val format = buildFormat(name)
        var c: MediaCodec? = null
        try {
            c = MediaCodec.createByCodecName(name)
        } catch (t: Throwable) {
            error("$tag.create", t); step("$tag.create", "failed", describe(t)); return false
        }
        try {
            c.configure(format, null, null, MediaCodec.CONFIGURE_FLAG_ENCODE)
        } catch (t: Throwable) {
            error("$tag.configure", t); step("$tag.configure", "failed", describe(t))
            runCatching { c.release() }; return false
        }
        val s = try {
            c.createInputSurface()
        } catch (t: Throwable) {
            error("$tag.createInputSurface", t); step("$tag.createInputSurface", "failed", describe(t))
            runCatching { c.stop() }; runCatching { c.release() }; return false
        }
        try {
            c.start()
        } catch (t: Throwable) {
            error("$tag.start", t); step("$tag.start", "failed", describe(t))
            runCatching { s.release() }; runCatching { c.release() }; return false
        }
        codec = c
        inputSurface = s
        step(
            tag, "ok",
            "$name ${cfg.width}x${cfg.height}@${cfg.fps} ${cfg.bitrate / 1000}kbps mode=${modeName(bitrateMode)}",
            ProcStats.clockMs() - t0,
            mapOf("format" to formatToMap(format)),
        )
        return true
    }

    /** Surface.setFrameRate() hint (API 30+) — a capture-rate lever distinct from KEY_FRAME_RATE. */
    private fun applyFrameRateHint(): Map<String, Any?> {
        val s = inputSurface ?: return mapOf("outcome" to "no surface")
        if (Build.VERSION.SDK_INT < 30) return mapOf("outcome" to "unsupported_api", "api" to Build.VERSION.SDK_INT)
        val t0 = ProcStats.clockMs()
        val strategy =
            if (cfg.frameRateHintAlways) Surface.CHANGE_FRAME_RATE_ALWAYS else Surface.CHANGE_FRAME_RATE_ONLY_IF_SEAMLESS
        val strategyName = if (cfg.frameRateHintAlways) "CHANGE_FRAME_RATE_ALWAYS" else "CHANGE_FRAME_RATE_ONLY_IF_SEAMLESS"
        return try {
            s.setFrameRate(cfg.fps.toFloat(), Surface.FRAME_RATE_COMPATIBILITY_FIXED_SOURCE, strategy)
            val r = mapOf<String, Any?>(
                "outcome" to "ok",
                "ms" to (ProcStats.clockMs() - t0),
                "requested_fps" to cfg.fps,
                "strategy" to strategyName,
            )
            step("surface.setFrameRate", "ok", "hint ${cfg.fps}fps via $strategyName", ProcStats.clockMs() - t0)
            r
        } catch (t: Throwable) {
            step("surface.setFrameRate", "failed", describe(t), ProcStats.clockMs() - t0)
            error("surface.setFrameRate", t)
            mapOf<String, Any?>("outcome" to "failed", "error" to describe(t))
        }
    }

    // --------------------------------------------------------- virtual display

    private fun createVirtualDisplay(
        surface: Surface,
        w: Int = cfg.width,
        h: Int = cfg.height,
        tag: String = "vd.create",
    ): VirtualDisplay? {
        val p = projection ?: run { step(tag, "failed", "no projection"); return null }
        val dpi = ctx.resources.displayMetrics.densityDpi
        val t0 = ProcStats.clockMs()
        return try {
            val d = p.createVirtualDisplay(
                "spike03-vd", w, h, dpi,
                DisplayManager.VIRTUAL_DISPLAY_FLAG_AUTO_MIRROR,
                surface,
                object : VirtualDisplay.Callback() {
                    override fun onPaused() {
                        vdEvents.add(mapOf("event" to "onPaused", "at_ms" to ProcStats.clockMs()))
                        S3Log.i("SPIKE03_EVENT vd_onPaused")
                    }

                    override fun onResumed() {
                        vdEvents.add(mapOf("event" to "onResumed", "at_ms" to ProcStats.clockMs()))
                        S3Log.i("SPIKE03_EVENT vd_onResumed")
                    }

                    override fun onStopped() {
                        vdEvents.add(mapOf("event" to "onStopped", "at_ms" to ProcStats.clockMs()))
                        S3Log.i("SPIKE03_EVENT vd_onStopped")
                    }
                },
                handler,
            )
            vd = d
            step(tag, "ok", "${w}x$h dpi=$dpi displayId=${d.display?.displayId}", ProcStats.clockMs() - t0)
            d
        } catch (t: Throwable) {
            error(tag, t)
            step(tag, "failed", describe(t), ProcStats.clockMs() - t0)
            null
        }
    }

    // ---------------------------------------------------------------- capture

    private fun requestSyncFrame(): Map<String, Any?> = try {
        val b = Bundle().apply { putInt(MediaCodec.PARAMETER_KEY_REQUEST_SYNC_FRAME, 0) }
        codec?.setParameters(b)
        mapOf("outcome" to "ok")
    } catch (t: Throwable) {
        error("codec.requestSyncFrame", t)
        mapOf("outcome" to "failed", "error" to describe(t))
    }

    private fun capturePhase(): Map<String, Any?> {
        val c = codec ?: return mapOf("error" to "no codec")
        val m = linkedMapOf<String, Any?>()
        val info = MediaCodec.BufferInfo()
        val startMs = ProcStats.clockMs()
        val endMs = startMs + cfg.seconds * 1000L
        val latencies = mutableListOf<Double>()
        val gaps = mutableListOf<Double>()
        val keyIntervals = mutableListOf<Double>()
        val buckets = mutableListOf<Map<String, Any?>>()
        val drainErrors = mutableListOf<Map<String, Any?>>()
        var frames = 0L
        var bytes = 0L
        var keyframes = 0L
        var configBuffers = 0L
        var lastFrameMs = 0L
        var lastKeyMs = 0L
        var firstFrameMs = -1L
        var negativeLatency = 0
        var ptsFirstMs = -1.0
        var ptsLastMs = 0.0
        var ptsMinMs = Double.MAX_VALUE
        var ptsMaxMs = -1.0
        var bucketStart = startMs
        var bFrames = 0L
        var bBytes = 0L
        var bKeys = 0L
        var bCpuStart = ProcStats.appCpuMs()
        var bRssStart = ProcStats.rssKb() ?: -1L

        val cpuStart = ProcStats.appCpuMs()
        val devCpuStart = ProcStats.deviceCpuJiffies()
        val pssStart = ProcStats.pss()
        val rssStart = ProcStats.rssKb()
        val thermalStart = ProcStats.thermal()

        fun emitBucket(nowMs: Long) {
            val sec = (nowMs - bucketStart) / 1000.0
            if (sec <= 0.0) return
            val cpuNow = ProcStats.appCpuMs()
            val rssNow = ProcStats.rssKb() ?: -1L
            buckets.add(
                linkedMapOf<String, Any?>(
                    "t_ms" to (bucketStart - startMs),
                    "frames" to bFrames,
                    "fps" to bFrames / sec,
                    "bytes" to bBytes,
                    "kbps" to bBytes * 8 / 1000.0 / sec,
                    "keyframes" to bKeys,
                    "bytes_per_frame" to if (bFrames > 0) bBytes.toDouble() / bFrames else 0.0,
                    "app_cpu_ms" to (cpuNow - bCpuStart),
                    "app_cpu_pct" to (cpuNow - bCpuStart) / (sec * 1000.0) * 100.0,
                    "rss_kb" to rssNow,
                    "rss_delta_kb" to if (bRssStart in 1..rssNow) rssNow - bRssStart else 0L,
                ),
            )
            bCpuStart = cpuNow
            bRssStart = rssNow
        }

        while (true) {
            val now = ProcStats.clockMs()
            if (now >= endMs) break
            val idx = try {
                c.dequeueOutputBuffer(info, 10_000)
            } catch (t: Throwable) {
                drainErrors.add(mapOf("at_ms" to (now - startMs), "error" to describe(t)))
                S3Log.e("SPIKE03_ERROR dequeue: ${describe(t)}")
                break
            }
            val now2 = ProcStats.clockMs()
            if (idx == MediaCodec.INFO_OUTPUT_FORMAT_CHANGED) {
                outputFormat = formatToMap(c.outputFormat)
                S3Log.i("SPIKE03_EVENT output_format_changed")
            } else if (idx >= 0) {
                if (info.size > 0) {
                    frames++
                    bytes += info.size
                    // presentationTimeUs is CLOCK_MONOTONIC microseconds (set by the
                    // composer), so compare against nanoTime(), NOT elapsedRealtimeNanos().
                    val latMs = (System.nanoTime() - info.presentationTimeUs * 1000L) / 1e6
                    val ptsMs = info.presentationTimeUs / 1000.0
                    if (ptsFirstMs < 0) ptsFirstMs = ptsMs
                    ptsLastMs = ptsMs
                    if (ptsMs < ptsMinMs) ptsMinMs = ptsMs
                    if (ptsMs > ptsMaxMs) ptsMaxMs = ptsMs
                    if (latMs < -5) negativeLatency++
                    if (latencies.size < 20_000) latencies.add(latMs)
                    if (firstFrameMs < 0) firstFrameMs = now2 - startMs
                    if (lastFrameMs > 0) gaps.add((now2 - lastFrameMs).toDouble())
                    lastFrameMs = now2
                    if (info.flags and MediaCodec.BUFFER_FLAG_KEY_FRAME != 0) {
                        keyframes++
                        if (lastKeyMs > 0) keyIntervals.add((now2 - lastKeyMs).toDouble())
                        lastKeyMs = now2
                        if (syncReqAt >= 0 && syncKeyAt < 0) {
                            syncKeyAt = now2 - startMs
                            probe("sync_frame", syncReqAt, "sync keyframe after ${syncKeyAt - syncReqAt} ms")
                        }
                    }
                    if (info.flags and MediaCodec.BUFFER_FLAG_CODEC_CONFIG != 0) configBuffers++
                    if (info.flags and MediaCodec.BUFFER_FLAG_END_OF_STREAM != 0) probe("encoder_eos", now2 - startMs)
                    bFrames++
                    bBytes += info.size
                    if (info.flags and MediaCodec.BUFFER_FLAG_KEY_FRAME != 0) bKeys++
                }
                c.releaseOutputBuffer(idx, false)
            }

            val el = now2 - startMs
            val totalMs = cfg.seconds * 1000L
            if (cfg.syncProbe && syncReqAt < 0 && el >= totalMs * 4 / 10) {
                requestSyncFrame()
                syncReqAt = el
                probe("sync_frame_request", el, "setParameters(REQUEST_SYNC_FRAME)")
            }
            if (cfg.secureProbe && secureOnAt < 0 && el >= totalMs * 5 / 10) {
                UiHooks.secureSetter?.invoke(true)
                secureOnAt = el
                probe("flag_secure_on", el, "activity window FLAG_SECURE set")
            }
            if (cfg.secureProbe && secureOnAt >= 0 && secureOffAt < 0 && el >= secureOnAt + 3000) {
                UiHooks.secureSetter?.invoke(false)
                secureOffAt = el
                probe("flag_secure_off", el, "FLAG_SECURE cleared")
            }
            if (cfg.backgroundProbe && backgroundAt < 0 && el >= totalMs * 8 / 10) {
                UiHooks.backgroundMover?.invoke()
                backgroundAt = el
                probe("move_task_to_back", el, "activity backgrounded while capturing")
            }
            if (cfg.staticProbe && staticOnAt < 0 && el >= totalMs * 3 / 10) {
                UiHooks.contentFreezer?.invoke(true)
                staticOnAt = el
                probe("content_frozen", el, "on-screen animation frozen — tests damage-driven composition")
            }
            if (cfg.staticProbe && staticOnAt >= 0 && staticOffAt < 0 && el >= staticOnAt + 3000) {
                UiHooks.contentFreezer?.invoke(false)
                staticOffAt = el
                probe("content_resumed", el, "animation resumed")
            }
            if (cfg.bitrateProbe && bitrateChangedAt < 0 && el >= totalMs * 7 / 10) {
                val target = cfg.bitrate / 3
                val ok = try {
                    c.setParameters(Bundle().apply { putInt(MediaCodec.PARAMETER_KEY_VIDEO_BITRATE, target) })
                    true
                } catch (t: Throwable) {
                    error("codec.setVideoBitrate", t)
                    false
                }
                bitrateChangedAt = el
                probe(
                    "bitrate_change", el,
                    "PARAMETER_KEY_VIDEO_BITRATE ${cfg.bitrate / 1000} -> ${target / 1000} kbps (applied=$ok)",
                )
            }
            if (now2 - bucketStart >= 1000) {
                emitBucket(now2)
                bucketStart = now2
                bFrames = 0; bBytes = 0; bKeys = 0
            }
        }
        emitBucket(ProcStats.clockMs())

        val endClock = ProcStats.clockMs()
        val elapsedSec = (endClock - startMs) / 1000.0
        val cpuEnd = ProcStats.appCpuMs()
        val devCpuEnd = ProcStats.deviceCpuJiffies()
        val pssEnd = ProcStats.pss()
        val rssEnd = ProcStats.rssKb()
        val thermalEnd = ProcStats.thermal()

        m["elapsed_s"] = elapsedSec
        m["frames"] = frames
        m["fps_avg"] = if (elapsedSec > 0) frames / elapsedSec else 0.0
        m["frames_expected_at_cfg_fps"] = cfg.fps * elapsedSec
        m["bytes_total"] = bytes
        m["bitrate_kbps_avg"] = if (elapsedSec > 0) bytes * 8 / 1000.0 / elapsedSec else 0.0
        m["bytes_per_frame_avg"] = if (frames > 0) bytes.toDouble() / frames else 0.0
        m["keyframes"] = keyframes
        m["codec_config_buffers"] = configBuffers
        m["first_frame_ms"] = firstFrameMs
        m["keyframe_intervals_ms"] = Stats.summary(keyIntervals)
        m["latency_proxy_ms"] = Stats.summary(latencies)
        m["frame_gap_ms"] = Stats.summary(gaps)
        m["negative_latency_samples"] = negativeLatency
        m["latency_proxy_definition"] =
            "nanoTime() at dequeue minus buffer presentationTimeUs — covers composer-timestamp to " +
                "encoded-buffer-available (encoder pipeline + queue). NOT glass-to-glass."
        val ptsSpanMs = if (ptsFirstMs >= 0) ptsLastMs - ptsFirstMs else 0.0
        m["pts"] = linkedMapOf<String, Any?>(
            "first_ms" to ptsFirstMs,
            "last_ms" to ptsLastMs,
            "min_ms" to (if (ptsMinMs == Double.MAX_VALUE) null else ptsMinMs),
            "max_ms" to ptsMaxMs,
            "span_ms" to ptsSpanMs,
            "implied_input_fps" to if (ptsSpanMs > 0) frames / (ptsSpanMs / 1000.0) else null,
            "clock" to "CLOCK_MONOTONIC microseconds (System.nanoTime timebase)",
        )
        m["drain_errors"] = drainErrors
        m["buckets"] = buckets
        m["output_format"] = outputFormat
        m["cpu"] = linkedMapOf<String, Any?>(
            "app_cpu_ms" to (cpuEnd - cpuStart),
            "app_cpu_pct_of_one_core" to if (elapsedSec > 0) (cpuEnd - cpuStart) / (elapsedSec * 1000.0) * 100.0 else null,
            "cores" to Runtime.getRuntime().availableProcessors(),
            "device" to if (devCpuStart != null && devCpuEnd != null) {
                val dTot = devCpuEnd.first - devCpuStart.first
                val dBusy = devCpuEnd.second - devCpuStart.second
                linkedMapOf<String, Any?>(
                    "busy_pct" to if (dTot > 0) dBusy * 100.0 / dTot else null,
                    "total_jiffies" to dTot,
                    "busy_jiffies" to dBusy,
                )
            } else "unavailable",
        )
        m["memory"] = linkedMapOf<String, Any?>(
            "pss_before_kb" to pssStart,
            "pss_after_kb" to pssEnd,
            "rss_before_kb" to (rssStart ?: -1),
            "rss_after_kb" to (rssEnd ?: -1),
            "rss_delta_kb" to if (rssStart != null && rssEnd != null) rssEnd - rssStart else null,
            "peak_rss_kb" to ProcStats.peakRssKb(),
            "threads" to ProcStats.threads(),
        )
        m["thermal"] = linkedMapOf<String, Any?>("zones_before" to thermalStart, "zones_after" to thermalEnd)
        m["battery"] = Device.battery(ctx)
        m["surface_frame_rate_hint"] = if (cfg.frameRateHint) lastFrameRateHint else "not_requested"
        m["flag_secure_effect"] = if (cfg.secureProbe) windowComparison(buckets, "secure", secureOnAt, secureOffAt) else "not_requested"
        m["static_content_effect"] =
            if (cfg.staticProbe) windowComparison(buckets, "static", staticOnAt, staticOffAt) else "not_requested"
        m["bitrate_change_effect"] =
            if (cfg.bitrateProbe && bitrateChangedAt >= 0) {
                windowComparison(buckets, "bitrate", bitrateChangedAt, bitrateChangedAt + 100_000)
            } else "not_requested"
        lastCapture = m
        return m
    }

    private var lastFrameRateHint: Map<String, Any?> = emptyMap()

    /** Compare per-second buckets before / during / after a probe window. */
    private fun windowComparison(
        buckets: List<Map<String, Any?>>,
        name: String,
        secureOnAt: Long,
        secureOffAt: Long,
    ): Map<String, Any?> {
        if (secureOnAt < 0 || secureOffAt < 0) return mapOf("error" to "$name probe did not complete")
        fun window(from: Long, to: Long): Map<String, Any?> {
            val sel = buckets.filter { b ->
                val t = (b["t_ms"] as? Long ?: 0L)
                t >= from && t < to
            }
            if (sel.isEmpty()) return mapOf("buckets" to 0)
            val fr = sel.sumOf { (it["frames"] as? Long ?: 0L) }
            val by = sel.sumOf { (it["bytes"] as? Long ?: 0L) }
            return linkedMapOf<String, Any?>(
                "buckets" to sel.size,
                "frames" to fr,
                "bytes" to by,
                "fps" to sel.sumOf { (it["fps"] as? Double ?: 0.0) } / sel.size,
                "kbps" to sel.sumOf { (it["kbps"] as? Double ?: 0.0) } / sel.size,
                "bytes_per_frame" to if (fr > 0) by.toDouble() / fr else 0.0,
            )
        }
        return linkedMapOf<String, Any?>(
            "secure_window_ms" to listOf(secureOnAt, secureOffAt),
            "before" to window(maxOf(0, secureOnAt - 2000), secureOnAt),
            "during" to window(secureOnAt, secureOffAt),
            "after" to window(secureOffAt, minOf(secureOffAt + 2000, Long.MAX_VALUE)),
        )
    }

    // ---------------------------------------------------------------- probes

    /** Short drain used to verify capture resumed after an in-session encoder restart. */
    private fun drainFor(ms: Long): Map<String, Any?> {
        val c = codec ?: return mapOf("error" to "no codec")
        val info = MediaCodec.BufferInfo()
        val start = ProcStats.clockMs()
        var frames = 0L
        var bytes = 0L
        val lat = mutableListOf<Double>()
        while (ProcStats.clockMs() - start < ms) {
            val idx = try {
                c.dequeueOutputBuffer(info, 10_000)
            } catch (t: Throwable) {
                return mapOf("error" to describe(t), "frames" to frames, "bytes" to bytes)
            }
            if (idx >= 0) {
                if (info.size > 0) {
                    frames++
                    bytes += info.size
                    lat.add((SystemClock.elapsedRealtimeNanos() - info.presentationTimeUs * 1000L) / 1e6)
                }
                c.releaseOutputBuffer(idx, false)
            }
        }
        val sec = (ProcStats.clockMs() - start) / 1000.0
        return linkedMapOf<String, Any?>(
            "frames" to frames,
            "bytes" to bytes,
            "fps" to if (sec > 0) frames / sec else 0.0,
            "latency_ms" to Stats.summary(lat),
        )
    }

    private fun newReaderSurface(w: Int, h: Int): ImageReader =
        ImageReader.newInstance(w, h, PixelFormat.RGBA_8888, 2)

    private fun lifecycleProbes() {
        val d = vd
        if (d == null) {
            note("lifecycle probes skipped: no VirtualDisplay")
            return
        }

        // A. A second concurrent encoder instance (tests MAX_SUPPORTED_INSTANCES in practice).
        run {
            val t0 = ProcStats.clockMs()
            var second: MediaCodec? = null
            try {
                val name = chosen!!.name
                second = MediaCodec.createByCodecName(name)
                second.configure(buildFormat(name), null, null, MediaCodec.CONFIGURE_FLAG_ENCODE)
                second.createInputSurface()
                second.start()
                step("probe.encoder_second_instance", "ok", "2 concurrent instances of $name", ProcStats.clockMs() - t0)
            } catch (t: Throwable) {
                step("probe.encoder_second_instance", "failed", describe(t), ProcStats.clockMs() - t0)
                error("probe.encoder_second_instance", t)
            } finally {
                runCatching { second?.stop() }
                runCatching { second?.release() }
            }
        }

        // B. A second VirtualDisplay from the same MediaProjection token.
        run {
            val t0 = ProcStats.clockMs()
            val reader = newReaderSurface(cfg.width, cfg.height)
            var second: VirtualDisplay? = null
            try {
                second = projection!!.createVirtualDisplay(
                    "spike03-vd-2", cfg.width, cfg.height, ctx.resources.displayMetrics.densityDpi,
                    DisplayManager.VIRTUAL_DISPLAY_FLAG_AUTO_MIRROR, reader.surface, null, handler,
                )
                step("probe.second_virtual_display", "ok", "second VD created (displayId=${second.display?.displayId})", ProcStats.clockMs() - t0)
                note("platform allowed a second VirtualDisplay on the same MediaProjection session")
            } catch (t: Throwable) {
                step("probe.second_virtual_display", "denied", describe(t), ProcStats.clockMs() - t0)
            } finally {
                runCatching { second?.release() }
                runCatching { reader.close() }
            }
        }

        // C. setSurface with a same-size consuming surface (ImageReader).
        run {
            val t0 = ProcStats.clockMs()
            val reader = newReaderSurface(cfg.width, cfg.height)
            try {
                d.setSurface(reader.surface)
                step("probe.set_surface_same_size", "ok", "surface swapped to same-size ImageReader", ProcStats.clockMs() - t0)
                Thread.sleep(300)
            } catch (t: Throwable) {
                step("probe.set_surface_same_size", "denied", describe(t), ProcStats.clockMs() - t0)
            } finally {
                runCatching { reader.close() }
            }
        }

        // D. Encoder restart inside the same projection session (must survive for adaptive encoding).
        run {
            val t0 = ProcStats.clockMs()
            var outcome = "ok"
            var detail = ""
            try {
                runCatching { codec?.stop() }
                runCatching { codec?.release() }
                runCatching { inputSurface?.release() }
                codec = null
                inputSurface = null
                if (!createEncoder("probe.encoder_restart")) {
                    outcome = "failed"; detail = "encoder recreate failed"
                } else {
                    d.setSurface(inputSurface!!)
                    if (cfg.frameRateHint) applyFrameRateHint()
                }
            } catch (t: Throwable) {
                outcome = "failed"
                detail = describe(t)
                error("probe.encoder_restart", t)
            }
            step("probe.encoder_restart", outcome, detail.ifEmpty { "codec recreated and bound to the live VirtualDisplay" }, ProcStats.clockMs() - t0)
            var post = drainFor(3000)
            // If the stream did not resume, retry the surface hand-off once: this
            // distinguishes "needs a nudge" from "cannot recover in-session".
            if (cfg.scenario == "lifecycle" && (post["frames"] as? Long ?: 0L) == 0L) {
                runCatching { d.setSurface(inputSurface) }
                val retry = drainFor(3000)
                phases["post_restart_capture_retry"] = retry
                step(
                    "probe.encoder_restart_retry", if ((retry["frames"] as? Long ?: 0L) > 0L) "ok" else "failed",
                    "second setSurface + drain: frames=${retry["frames"]}",
                )
                if ((retry["frames"] as? Long ?: 0L) > 0L) post = retry
            }
            phases["post_restart_capture"] = post
            probe("post_restart_frames", ProcStats.clockMs() - sessionStartMs, "frames=${post["frames"]} fps=${post["fps"]}")
        }

        // E. setSurface with a different-size surface.
        run {
            val t0 = ProcStats.clockMs()
            val reader = newReaderSurface(cfg.width / 2, cfg.height / 2)
            try {
                d.setSurface(reader.surface)
                step("probe.set_surface_resized", "ok", "surface swapped to ${cfg.width / 2}x${cfg.height / 2}", ProcStats.clockMs() - t0)
                Thread.sleep(300)
            } catch (t: Throwable) {
                step("probe.set_surface_resized", "denied", describe(t), ProcStats.clockMs() - t0)
            } finally {
                runCatching { reader.close() }
            }
            // restore encoder surface
            runCatching { d.setSurface(inputSurface) }
        }

        // F. resize() manifest probe (documented as disallowed for MediaProjection VDs on API 34+).
        run {
            val t0 = ProcStats.clockMs()
            val dpi = ctx.resources.displayMetrics.densityDpi
            var resized = false
            try {
                d.resize(cfg.width / 2, cfg.height / 2, dpi)
                resized = true
                step("probe.vd_resize", "ok", "resized ${cfg.width}x${cfg.height} -> ${cfg.width / 2}x${cfg.height / 2}", ProcStats.clockMs() - t0)
            } catch (t: Throwable) {
                step("probe.vd_resize", "denied", describe(t), ProcStats.clockMs() - t0)
            }
            if (resized) {
                val post = drainFor(1000)
                phases["post_resize_capture"] = post
                runCatching { d.resize(cfg.width, cfg.height, dpi) }
            }
        }
    }

    // --------------------------------------------------------------- shutdown

    private fun shutdown(): Map<String, Any?> {
        val m = linkedMapOf<String, Any?>()
        var t0 = ProcStats.clockMs()
        try {
            vd?.release()
            step("vd.release", "ok", "", ProcStats.clockMs() - t0)
        } catch (t: Throwable) {
            error("vd.release", t); step("vd.release", "failed", describe(t), ProcStats.clockMs() - t0)
        }
        t0 = ProcStats.clockMs()
        try {
            codec?.stop()
            codec?.release()
            inputSurface?.release()
            step("encoder.stop_release", "ok", "", ProcStats.clockMs() - t0)
        } catch (t: Throwable) {
            error("encoder.stop_release", t); step("encoder.stop_release", "failed", describe(t), ProcStats.clockMs() - t0)
        }
        codec = null
        inputSurface = null

        // Stop the projection and measure how long the onStop callback takes.
        // NOTE: the callback must stay registered across stop() — unregistering first
        // is itself an error path, and it also invalidates the post-stop probes.
        val cbStart = ProcStats.clockMs()
        try {
            projection?.stop()
        } catch (t: Throwable) {
            error("projection.stop", t)
        }
        while (!projectionStopped && ProcStats.clockMs() - cbStart < 2000) Thread.sleep(20)
        m["onstop_callback_ms"] = if (projectionStopped) ProcStats.clockMs() - cbStart else -1
        m["onstop_fired"] = projectionStopped
        step("projection.stop", if (projectionStopped) "ok" else "no_callback", "onStop fired=$projectionStopped", ProcStats.clockMs() - cbStart)

        // Post-stop probe 1: is the stopped token still able to create a display?
        // (The callback stays registered, so a failure here is about the token,
        // not about the missing-callback precondition.)
        val reader = newReaderSurface(cfg.width, cfg.height)
        try {
            projection!!.createVirtualDisplay(
                "spike03-after-stop", cfg.width, cfg.height, ctx.resources.displayMetrics.densityDpi,
                DisplayManager.VIRTUAL_DISPLAY_FLAG_AUTO_MIRROR, reader.surface, null, handler,
            )
            step("post_stop.create_virtual_display", "ok", "stopped projection still usable (unexpected)")
        } catch (t: Throwable) {
            step("post_stop.create_virtual_display", "denied", describe(t))
        } finally {
            runCatching { reader.close() }
        }

        // Post-stop probe 2: can a NEW projection be obtained from the same consent
        // Intent, and does it actually capture (i.e. restart without re-prompting)?
        try {
            val mpm = ctx.getSystemService(Context.MEDIA_PROJECTION_SERVICE) as MediaProjectionManager
            val again = mpm.getMediaProjection(resultCodeSaved, resultDataSaved!!)
            step("post_stop.get_media_projection", if (again == null) "null" else "ok", "second token from the same consent Intent")
            if (again != null) {
                val cb2 = object : MediaProjection.Callback() {
                    override fun onStop() {
                        S3Log.i("SPIKE03_EVENT projection2_onStop")
                    }
                }
                try {
                    again.registerCallback(cb2, handler)
                } catch (t: Throwable) {
                    step("post_stop.register_callback_2", "failed", describe(t))
                }
                val r2 = newReaderSurface(cfg.width, cfg.height)
                try {
                    val vd2 = again.createVirtualDisplay(
                        "spike03-reuse", cfg.width, cfg.height, ctx.resources.displayMetrics.densityDpi,
                        DisplayManager.VIRTUAL_DISPLAY_FLAG_AUTO_MIRROR, r2.surface, null, handler,
                    )
                    Thread.sleep(500)
                    step(
                        "post_stop.reuse_token_virtual_display", "ok",
                        "capture restarted on a fresh token WITHOUT new consent (displayId=${vd2.display?.displayId})",
                    )
                    runCatching { vd2.release() }
                } catch (t: Throwable) {
                    step("post_stop.reuse_token_virtual_display", "denied", describe(t))
                } finally {
                    runCatching { r2.close() }
                }
                runCatching { again.unregisterCallback(cb2) }
                runCatching { again.stop() }
            }
        } catch (t: Throwable) {
            step("post_stop.get_media_projection", "denied", describe(t))
        }
        try {
            projectionCallback?.let { projection?.unregisterCallback(it) }
        } catch (t: Throwable) {
            error("projection.unregisterCallback", t)
        }
        return m
    }

    // -------------------------------------------------------------------- run

    private fun phase(name: String, body: () -> Unit) {
        val t0 = ProcStats.clockMs()
        try {
            body()
        } catch (t: Throwable) {
            error("phase.$name", t)
        } finally {
            phases["${name}_ms"] = ProcStats.clockMs() - t0
        }
    }

    fun run(): Map<String, Any?> {
        if (cfg.cycles > 0) phase("encoder_cycles") { encoderCycles(cfg.cycles) }

        var ok = true
        phase("encoder_start") { ok = createEncoder() }
        if (cfg.frameRateHint) lastFrameRateHint = applyFrameRateHint()
        if (!ok) {
            note("encoder could not be started; skipping capture")
            return build("encoder_failed")
        }

        phase("virtual_display") {
            val d = createVirtualDisplay(inputSurface!!)
            if (d == null) ok = false
        }
        if (!ok || vd == null) {
            note("VirtualDisplay could not be created; skipping capture")
            phase("shutdown") { shutdown() }
            return build("virtual_display_failed")
        }

        phase("capture") { lastCapture = capturePhase() }
        if (cfg.scenario == "lifecycle") phase("lifecycle_probes") { lifecycleProbes() }
        phase("shutdown") { shutdown() }

        // memory released after shutdown (codec/surface/VD/threads)
        Thread.sleep(200)
        phases["pss_after_shutdown"] = ProcStats.pss()
        phases["rss_after_shutdown_kb"] = ProcStats.rssKb() ?: -1
        return build(if (errors.isEmpty()) "ok" else "ok_with_errors")
    }

    /** configure → start → stop → release, N times, to find persistent encoder errors. */
    private fun encoderCycles(n: Int) {
        val times = mutableListOf<Double>()
        val failures = mutableListOf<Map<String, Any?>>()
        val name = chosen?.name ?: Caps.select(cfg.mime, cfg.width, cfg.height, cfg.fps).first?.name ?: return
        chosen = chosen ?: Caps.videoEncoders(cfg.mime).firstOrNull { it.name == name }
        for (i in 1..n) {
            val t0 = SystemClock.elapsedRealtimeNanos()
            var c: MediaCodec? = null
            var s: Surface? = null
            try {
                c = MediaCodec.createByCodecName(name)
                c.configure(buildFormat(name), null, null, MediaCodec.CONFIGURE_FLAG_ENCODE)
                s = c.createInputSurface()
                c.start()
                c.stop()
                c.release()
                s.release()
                times.add((SystemClock.elapsedRealtimeNanos() - t0) / 1e6)
            } catch (t: Throwable) {
                failures.add(mapOf("cycle" to i, "error" to describe(t)))
                runCatching { s?.release() }
                runCatching { c?.release() }
            }
        }
        phases["encoder_cycles"] = linkedMapOf<String, Any?>(
            "requested" to n,
            "completed" to times.size,
            "failures" to failures,
            "cycle_ms" to Stats.summary(times),
        )
        step(
            "encoder.cycles", if (failures.isEmpty()) "ok" else "partial",
            "$n configure/start/stop/release cycles, ${failures.size} failure(s)",
            -1, mapOf("cycle_ms" to Stats.summary(times)),
        )
    }

    fun build(status: String): Map<String, Any?> = linkedMapOf(
        "spike" to "03-android-mediaprojection-encoder",
        "status" to status,
        "generated_at" to Results.isoNow(),
        "total_elapsed_ms" to (ProcStats.clockMs() - sessionStartMs),
        "device" to Device.info(ctx),
        "config" to cfg.describe(),
        "codec_selected" to (
            chosen?.let {
                linkedMapOf<String, Any?>(
                    "name" to it.name,
                    "mime" to cfg.mime,
                    "hardware_accelerated" to it.hardware,
                    "software_only" to it.softwareOnly,
                    "vendor" to it.vendor,
                    "selection_reason" to selectedReason,
                    "bitrate_mode" to modeName(bitrateMode),
                )
            } ?: emptyMap<String, Any?>()
            ),
        "steps" to steps,
        "probes" to probes,
        "phases" to phases,
        "virtual_display_events" to vdEvents,
        "capture" to lastCapture,
        "errors" to errors,
        "notes" to notes,
    )
}
