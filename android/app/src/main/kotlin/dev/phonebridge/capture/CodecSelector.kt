package dev.phonebridge.capture

import android.media.MediaCodecInfo
import android.media.MediaCodecList
import android.os.Build

/**
 * CodecSelector selects the optimal hardware H.264 video encoder by interrogating
 * the platform's MediaCodecList.
 *
 * Implements DEC-020:
 *  - Preference: Hardware-accelerated primary (e.g. c2.qti.avc.encoder on SM7475).
 *  - Format: Surface-input encoding (COLOR_FormatSurface).
 *  - AV1 excluded; software-only encoders excluded unless no hardware exists.
 */
object CodecSelector {

    data class CodecCandidate(
        val info: MediaCodecInfo,
        val mime: String,
        val caps: MediaCodecInfo.CodecCapabilities?,
    ) {
        val name: String get() = info.name

        val isHardware: Boolean
            get() = if (Build.VERSION.SDK_INT >= 29) info.isHardwareAccelerated else false

        val isSoftwareOnly: Boolean
            get() = if (Build.VERSION.SDK_INT >= 29) {
                info.isSoftwareOnly
            } else {
                name.contains("google", ignoreCase = true) ||
                name.contains("android", ignoreCase = true) ||
                name.contains("sw", ignoreCase = true)
            }

        val supportsSurface: Boolean
            get() = caps?.colorFormats?.contains(MediaCodecInfo.CodecCapabilities.COLOR_FormatSurface) == true

        fun supportsSize(w: Int, h: Int): Boolean = try {
            caps?.videoCapabilities?.isSizeSupported(w, h) ?: false
        } catch (_: Throwable) {
            false
        }

        fun supportsSizeAndRate(w: Int, h: Int, fps: Int): Boolean = try {
            caps?.videoCapabilities?.areSizeAndRateSupported(w, h, fps.toDouble()) ?: false
        } catch (_: Throwable) {
            false
        }
    }

    data class Selection(
        val candidate: CodecCandidate?,
        val name: String,
        val isHardware: Boolean,
        val reason: String,
    )

    fun enumerate(mime: String = CaptureConfig.DEFAULT_MIME): List<CodecCandidate> {
        val out = mutableListOf<CodecCandidate>()
        val list = MediaCodecList(MediaCodecList.REGULAR_CODECS)
        for (info in list.codecInfos) {
            if (!info.isEncoder) continue
            val hasMime = info.supportedTypes.any { it.equals(mime, ignoreCase = true) }
            if (!hasMime) continue

            val caps = try {
                info.getCapabilitiesForType(mime)
            } catch (_: Throwable) {
                null
            }
            out.add(CodecCandidate(info, mime, caps))
        }
        return out
    }

    /**
     * Selects the best hardware H.264 encoder for the target parameters.
     */
    fun select(config: CaptureConfig): Selection {
        val candidates = enumerate(config.mime)
        if (candidates.isEmpty()) {
            return Selection(null, "", false, "No encoder available declaring ${config.mime}")
        }

        // 1. Must support Surface input (MediaProjection renders to Surface)
        val surfaceCapable = candidates.filter { it.supportsSurface }
        val pool = surfaceCapable.ifEmpty { candidates }

        // 2. Classify candidates
        val exactMatch = pool.filter { it.supportsSizeAndRate(config.width, config.height, config.fps) }
        val sizeMatch = pool.filter { it.supportsSize(config.width, config.height) }

        fun rank(c: CodecCandidate): Int {
            var score = 0
            if (!c.isHardware) score += 100
            if (c.isSoftwareOnly) score += 1000
            return score
        }

        val targetGroup = when {
            exactMatch.isNotEmpty() -> exactMatch
            sizeMatch.isNotEmpty() -> sizeMatch
            else -> pool
        }

        val chosen = targetGroup.minByOrNull { rank(it) }
            ?: return Selection(null, "", false, "No viable candidate found")

        val reason = when {
            exactMatch.contains(chosen) && chosen.isHardware ->
                "Hardware encoder with exact size+rate support (${chosen.name})"
            sizeMatch.contains(chosen) && chosen.isHardware ->
                "Hardware encoder with size support (${chosen.name})"
            chosen.isHardware ->
                "Hardware encoder best match (${chosen.name})"
            else ->
                "Software fallback encoder (${chosen.name})"
        }

        return Selection(
            candidate = chosen,
            name = chosen.name,
            isHardware = chosen.isHardware,
            reason = reason,
        )
    }
}
