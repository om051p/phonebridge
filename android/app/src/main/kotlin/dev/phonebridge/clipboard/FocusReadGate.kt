package dev.phonebridge.clipboard

import java.util.concurrent.atomic.AtomicLong

/**
 * Debounces automatic focus-triggered clipboard reads (DEC-023 no-keyboard path).
 *
 * Why this policy exists: from Android 10 on, an app may read the clipboard
 * only while it owns the focused window (AOSP `ClipboardService.
 * clipboardAccessAllowed` → `mWm.isUidFocused(uid)`), unless it is the default
 * IME or holds the `signature|role` `READ_CLIPBOARD_IN_BACKGROUND` permission
 * that only system components are granted. Both exceptions are out of reach for
 * the default experience, so "the app is on screen" is the one moment a
 * phone-side copy can reach the PC with the user's own keyboard selected.
 *
 * Window focus, though, flaps constantly — dialogs, pickers, coming back from
 * any other app — and every read costs a binder round-trip plus a Go engine
 * update. This gate collapses the flaps to at most one read per window and
 * claims the window atomically, so two triggers arriving together cannot both
 * read.
 *
 * Explicit user requests ([AndroidClipboardAdapter.triggerManualPull]) bypass
 * this gate on purpose: a tap is not a flap.
 */
internal class FocusReadGate(
    private val minIntervalMs: Long = DEFAULT_MIN_INTERVAL_MS
) {
    /**
     * Timestamp of the read that claimed the current window, or [NEVER].
     *
     * Atomic because the window is claimed with a compare-and-set: a plain
     * read-then-write would let two concurrent triggers both observe a stale
     * timestamp and both run a read.
     */
    private val lastReadAtMs = AtomicLong(NEVER)

    /**
     * Runs [read] if the window is open, and reports whether it ran.
     *
     * The window is consumed regardless of what [read] returns: "nothing copied
     * yet" is the common case when the app comes to the foreground, and
     * retrying it on every flap would do the work the gate exists to avoid.
     */
    fun run(nowMs: Long, read: () -> Boolean): Boolean {
        val last = lastReadAtMs.get()
        if (last != NEVER && nowMs - last < minIntervalMs) {
            return false
        }
        if (!lastReadAtMs.compareAndSet(last, nowMs)) {
            return false
        }
        return read()
    }

    companion object {
        /**
         * Default spacing between two automatic reads.
         *
         * One second is far below what a human notices and far above the rate
         * at which focus events arrive, and it bounds the binder traffic this
         * path can generate.
         */
        const val DEFAULT_MIN_INTERVAL_MS = 1_000L

        /**
         * Sentinel for "no read has run yet".
         *
         * 0 is safe as a sentinel because `SystemClock.uptimeMillis()` is
         * strictly positive for any running process, and treating it as "read
         * at time zero" would swallow the first copy of every session.
         */
        private const val NEVER = 0L
    }
}
