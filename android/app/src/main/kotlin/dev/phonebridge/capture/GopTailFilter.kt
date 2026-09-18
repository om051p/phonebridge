package dev.phonebridge.capture

/**
 * GopTailFilter implements the prediction-safe GOP-tail dropping rule ratified in DEC-020 and DEC-021.
 *
 * Background & Proof:
 *  - Android hardware H.264 encoders ignore KEY_MAX_FPS_TO_ENCODER on many devices (e.g. SM7475)
 *    and encode at panel refresh rate (~119-120 fps).
 *  - Naive 1-of-4 striding drops intermediate P-frames whose references are later required,
 *    corrupting ~93.3% of kept P-frames (even when decoder error concealment hides it).
 *  - Because the hardware encoder produces strictly IPPP streams without B-frames or reordering,
 *    keeping a contiguous prefix of each GOP (the IDR + first K AUs) is prediction-safe by
 *    construction (every kept frame references only earlier kept frames in the same GOP).
 *    Verified pixel-exact: SSIM = 1.000000, 0 imperfect frames.
 */
class GopTailFilter(
    val keepFrames: Int = CaptureConfig.DEFAULT_KEEP_FRAMES
) {
    var sinceKey: Int = 0
        private set

    var totalFrames: Long = 0L
        private set

    var admittedFrames: Long = 0L
        private set

    var droppedFrames: Long = 0L
        private set

    var keyframes: Long = 0L
        private set

    /**
     * Evaluates whether the given AU should be admitted into the transport pipeline.
     *
     * @param isKey true if the AU is an IDR/keyframe (or contains CSD/parameter sets).
     * @return true if the AU is admitted; false if it belongs to the GOP tail and must be dropped.
     */
    @Synchronized
    fun shouldAdmit(isKey: Boolean): Boolean {
        totalFrames++
        if (isKey) {
            sinceKey = 0
            keyframes++
            admittedFrames++
            return true
        }

        sinceKey++
        val admit = sinceKey < keepFrames
        if (admit) {
            admittedFrames++
        } else {
            droppedFrames++
        }
        return admit
    }

    @Synchronized
    fun reset() {
        sinceKey = 0
        totalFrames = 0L
        admittedFrames = 0L
        droppedFrames = 0L
        keyframes = 0L
    }
}
