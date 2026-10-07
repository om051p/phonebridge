package dev.phonebridge.capture

/**
 * BitratePolicy determines device-adapted video encoding parameters.
 *
 * Problem:
 * Android hardware video encoders encode at the panel's physical refresh rate
 * (e.g. 90 fps on Pixel 4 XL, 120 fps on SM7475) regardless of KEY_FRAME_RATE
 * or KEY_MAX_FPS_TO_ENCODER (DEC-020). Under standard 60 Hz bitrate allocations
 * (e.g. 2.5 Mbps CBR), high-refresh devices starve each encoded frame of bits
 * (~29 kbit/frame on Pixel vs ~55 kbit/frame on 60 Hz POCO F1), forcing the
 * encoder to apply aggressive quantization that blurs text and fine UI lines.
 *
 * Policy:
 *  - High-refresh displays (90 Hz, 120 Hz) scale the bitrate proportionally to
 *    maintain the target bit-per-frame budget, rounded up to 500 kbps steps.
 *  - 60 Hz displays retain the standard base bitrate without modification.
 *  - Codec 2.0 (c2.*) encoders enable AVCProfileHigh and Level 4.1 where
 *    supported, bypassing the Level 1.0 constraint clamp that limits I-frame
 *    allocation.
 *  - Legacy OMX (OMX.*) encoders stay on Baseline to prevent I-frame buffer
 *    expansion faults.
 */
object BitratePolicy {
    const val BASELINE_REFRESH_RATE = 60.0f
    const val MAX_ADAPTED_BITRATE = 6_000_000 // 6.0 Mbps ceiling

    /**
     * Adapts [baseBitrate] to the display's [refreshRate].
     *
     * @param baseBitrate configured bitrate in bps (e.g. 2_500_000)
     * @param refreshRate display compositor refresh rate in Hz (e.g. 60.0, 90.0, 120.0)
     * @return adapted bitrate in bps
     */
    fun adaptBitrate(baseBitrate: Int, refreshRate: Float): Int {
        if (refreshRate <= BASELINE_REFRESH_RATE + 1.0f) {
            return baseBitrate
        }

        val factor = (refreshRate / BASELINE_REFRESH_RATE).coerceIn(1.0f, 2.0f)
        val raw = baseBitrate * factor

        // Step up in 500 kbps increments to provide comfortable quantization headroom
        val step = 500_000
        val stepped = (Math.ceil(raw / step.toDouble()) * step).toInt()

        return stepped.coerceIn(baseBitrate, MAX_ADAPTED_BITRATE)
    }

    /**
     * Determines whether AVCProfileHigh should be enabled for [codecName].
     *
     * Only modern Codec 2.0 (c2.*) encoders are upgraded; legacy OMX encoders
     * are preserved on Baseline to prevent rate-control instability.
     */
    fun shouldEnableHighProfile(codecName: String, supportsHigh: Boolean): Boolean {
        return codecName.startsWith("c2.", ignoreCase = true) && supportsHigh
    }
}
