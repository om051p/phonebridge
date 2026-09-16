package dev.phonebridge.spike03

import android.media.MediaCodecInfo
import android.media.MediaCodecList
import android.os.Build
import android.util.Range

/**
 * Encoder inventory and selection.
 *
 * The spike answers "does a usable hardware H.264 encoder exist, and is it
 * consistent?" by enumerating MediaCodecList rather than assuming.
 */
object Caps {

    val MIMES = listOf("video/avc", "video/hevc", "video/av01", "video/x-vnd.on2.vp8", "video/x-vnd.on2.vp9")

    /** Test configs used for capability probing in [inventory]. */
    val PROBE_SIZES = listOf(
        Triple(1080, 2400, 30),
        Triple(2400, 1080, 30),
        Triple(1920, 1080, 30),
        Triple(1280, 720, 30),
        Triple(1280, 720, 60),
        Triple(1920, 1080, 60),
        Triple(2560, 1440, 30),
        Triple(540, 1200, 30),
        Triple(3840, 2160, 30),
    )

    class Candidate(
        val info: MediaCodecInfo,
        val caps: MediaCodecInfo.VideoCapabilities?,
        val mime: String,
    ) {
        val name: String get() = info.name
        val hardware: Boolean
            get() = if (Build.VERSION.SDK_INT >= 29) info.isHardwareAccelerated() else false
        val softwareOnly: Boolean
            get() = if (Build.VERSION.SDK_INT >= 29) info.isSoftwareOnly() else name.contains("sw", true) || name.contains("google", true)
        val vendor: Boolean
            get() = if (Build.VERSION.SDK_INT >= 29) info.isVendor() else false

        fun supportsSize(w: Int, h: Int): Boolean = try { caps?.isSizeSupported(w, h) ?: false } catch (t: Throwable) { false }
        fun supportsSizeRate(w: Int, h: Int, fps: Int): Boolean =
            try { caps?.areSizeAndRateSupported(w, h, fps.toDouble()) ?: false } catch (t: Throwable) { false }
    }

    fun videoEncoders(mime: String): List<Candidate> {
        val out = mutableListOf<Candidate>()
        for (info in MediaCodecList(MediaCodecList.REGULAR_CODECS).codecInfos) {
            if (!info.isEncoder) continue
            if (!info.supportedTypes.any { it.equals(mime, true) }) continue
            out.add(Candidate(info, videoCaps(info, mime), mime))
        }
        return out
    }

    private fun videoCaps(info: MediaCodecInfo, mime: String): MediaCodecInfo.VideoCapabilities? = try {
        info.getCapabilitiesForType(mime).videoCapabilities
    } catch (t: Throwable) {
        null
    }

    /**
     * Pick an encoder for (mime, w, h, fps): prefer hardware, then exact size+rate
     * support, then size support only. Returns the candidate plus a human-readable
     * reason string (recorded in the evidence).
     */
    fun select(mime: String, w: Int, h: Int, fps: Int): Pair<Candidate?, String> {
        val all = videoEncoders(mime)
        if (all.isEmpty()) return null to "no encoder declares $mime"
        val exact = all.filter { it.supportsSizeRate(w, h, fps) }
        val sizeOnly = all.filter { it.supportsSize(w, h) }
        fun rank(c: Candidate) = (if (c.hardware) 0 else 100) + (if (c.softwareOnly) 1000 else 0)
        val pool = when {
            exact.isNotEmpty() -> exact.sortedBy { rank(it) }
            sizeOnly.isNotEmpty() -> sizeOnly.sortedBy { rank(it) }
            else -> all.sortedBy { rank(it) }
        }
        val picked = pool.first()
        val reason = when {
            exact.contains(picked) -> "hardware+size+rate match from ${all.size} candidate(s)"
            sizeOnly.contains(picked) -> "size match only (no candidate advertises ${fps}fps at ${w}x$h) from ${all.size} candidate(s)"
            else -> "no capability match for ${w}x$h@$fps; selected best available"
        }
        return picked to reason
    }

    /** Full inventory of video encoders for all candidate mimes + probe matrix. */
    fun inventory(): Map<String, Any?> {
        val byMime = linkedMapOf<String, Any?>()
        for (mime in MIMES) {
            val cands = videoEncoders(mime)
            byMime[mime] = cands.map { describe(it) }
        }
        return linkedMapOf(
            "encoder_count_total" to MediaCodecList(MediaCodecList.REGULAR_CODECS).codecInfos.count { it.isEncoder },
            "encoders_by_mime" to byMime,
            "probe_matrix" to probeMatrix(),
        )
    }

    private fun probeMatrix(): List<Map<String, Any?>> {
        val avc = videoEncoders("video/avc")
        return PROBE_SIZES.map { (w, h, fps) ->
            linkedMapOf<String, Any?>(
                "config" to "${w}x$h@$fps",
                "avc_candidates" to avc.map { c ->
                    linkedMapOf<String, Any?>(
                        "name" to c.name,
                        "hardware" to c.hardware,
                        "size_supported" to c.supportsSize(w, h),
                        "size_rate_supported" to c.supportsSizeRate(w, h, fps),
                    )
                },
                "selected" to select("video/avc", w, h, fps).first?.name,
            )
        }
    }

    fun describe(c: Candidate): Map<String, Any?> {
        val caps = c.caps
        val m = linkedMapOf<String, Any?>(
            "name" to c.name,
            "canonical_name" to (if (Build.VERSION.SDK_INT >= 29) c.info.canonicalName else c.name),
            "hardware_accelerated" to c.hardware,
            "software_only" to c.softwareOnly,
            "vendor" to c.vendor,
        )
        if (caps != null) {
            runCatching {
                m["width_range"] = rangeStr(caps.supportedWidths)
                m["height_range"] = rangeStr(caps.supportedHeights)
                m["frame_rate_range"] = rangeStr(caps.supportedFrameRates)
                m["bitrate_range"] = "${caps.bitrateRange.lower}..${caps.bitrateRange.upper}"
                m["width_alignment"] = caps.widthAlignment
                m["height_alignment"] = caps.heightAlignment
                m["max_supported_instances"] = c.info.getCapabilitiesForType(c.mime).maxSupportedInstances
                m["bitrate_modes_supported"] = bitrateModes(c.info, c.mime)
                m["profiles_levels"] = profiles(c.info, c.mime)
            }.onFailure { m["caps_error"] = it.toString() }
        } else {
            m["caps_error"] = "no VideoCapabilities"
        }
        return m
    }

    private fun bitrateModes(info: MediaCodecInfo, mime: String): List<String> {
        return try {
            val eb = info.getCapabilitiesForType(mime).encoderCapabilities
            listOf(
                MediaCodecInfo.EncoderCapabilities.BITRATE_MODE_CBR to "CBR",
                MediaCodecInfo.EncoderCapabilities.BITRATE_MODE_VBR to "VBR",
                MediaCodecInfo.EncoderCapabilities.BITRATE_MODE_CQ to "CQ",
            ).filter { eb.isBitrateModeSupported(it.first) }.map { it.second }
        } catch (t: Throwable) {
            emptyList()
        }
    }

    private fun profiles(info: MediaCodecInfo, mime: String): List<String> = try {
        info.getCapabilitiesForType(mime).profileLevels.map { pl ->
            "${profileName(pl.profile)}/level${pl.level}"
        }
    } catch (t: Throwable) {
        emptyList()
    }

    private fun profileName(p: Int): String = when (p) {
        1 -> "Baseline"
        2 -> "Main"
        4 -> "High"
        8 -> "High10"
        16 -> "High422"
        32 -> "High444"
        0x1000 -> "ConstrainedBaseline"
        0x2000 -> "ConstrainedHigh"
        0x4000 -> "HEVC_Main"
        0x8000 -> "HEVC_Main10"
        else -> "profile$p"
    }

    private fun rangeStr(r: Range<Int>?): String? = if (r == null) null else "${r.lower}..${r.upper}"
}
