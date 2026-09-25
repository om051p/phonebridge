package dev.phonebridge.input

import android.accessibilityservice.AccessibilityService
import android.accessibilityservice.GestureDescription
import android.content.Intent
import android.graphics.Path
import android.os.Build
import android.os.SystemClock
import android.util.Log
import android.view.accessibility.AccessibilityEvent
import kotlin.math.abs
import kotlin.math.hypot

/**
 * PhoneBridge AccessibilityService (DEC-027, Phase 7 Remote Input v0.1).
 *
 * Dispatches simulated touch gestures and global navigation actions on Android.
 * ZERO-LOGGING RULE: Absolute prohibition on logging coordinates or keystrokes.
 */
class PhoneBridgeAccessibilityService : AccessibilityService() {

    companion object {
        private const val TAG = "PhoneBridgeAcc"

        @Volatile
        private var instance: PhoneBridgeAccessibilityService? = null

        fun getInstance(): PhoneBridgeAccessibilityService? = instance

        const val ACTION_DOWN = 0
        const val ACTION_UP = 1
        const val ACTION_MOVE = 2
        const val ACTION_CANCEL = 3
    }

    private var downX = 0f
    private var downY = 0f
    private var lastX = 0f
    private var lastY = 0f
    private var downTimeMs = 0L
    private var activePath: Path? = null
    private var isTrackingTouch = false

    override fun onServiceConnected() {
        super.onServiceConnected()
        instance = this
        Log.i(TAG, "PhoneBridgeAccessibilityService connected")
    }

    override fun onUnbind(intent: Intent?): Boolean {
        Log.i(TAG, "PhoneBridgeAccessibilityService unbound")
        instance = null
        return super.onUnbind(intent)
    }

    override fun onDestroy() {
        if (instance == this) {
            instance = null
        }
        super.onDestroy()
    }

    override fun onAccessibilityEvent(event: AccessibilityEvent?) {
        // No-op: Remote input only performs gestures and global actions.
    }

    override fun onInterrupt() {
        isTrackingTouch = false
        activePath = null
    }

    /**
     * Handles incoming normalized touch events from Go bridge.
     */
    fun onTouchEvent(action: Int, pointerId: Int, normX: Float, normY: Float, pressure: Float): Boolean {
        val dm = resources.displayMetrics
        val width = dm.widthPixels.toFloat()
        val height = dm.heightPixels.toFloat()

        val px = (normX * width).coerceIn(0f, width - 1f)
        val py = (normY * height).coerceIn(0f, height - 1f)

        when (action) {
            ACTION_DOWN -> {
                downX = px
                downY = py
                lastX = px
                lastY = py
                downTimeMs = SystemClock.uptimeMillis()
                val p = Path()
                p.moveTo(px, py)
                activePath = p
                isTrackingTouch = true
                return true
            }
            ACTION_MOVE -> {
                if (!isTrackingTouch) {
                    return false
                }
                activePath?.lineTo(px, py)
                lastX = px
                lastY = py
                return true
            }
            ACTION_UP -> {
                if (!isTrackingTouch) {
                    return false
                }
                isTrackingTouch = false
                val path = activePath ?: Path().apply { moveTo(px, py) }
                path.lineTo(px, py)

                val elapsed = SystemClock.uptimeMillis() - downTimeMs
                val duration = elapsed.coerceIn(50L, 2000L)

                val dist = hypot((px - downX).toDouble(), (py - downY).toDouble()).toFloat()
                if (dist < 10f) {
                    // Tap or long press
                    val gesturePath = Path().apply { moveTo(downX, downY) }
                    val tapDuration = if (elapsed >= 500L) 500L else 50L
                    return dispatchPath(gesturePath, tapDuration)
                }

                // Drag / swipe
                return dispatchPath(path, duration)
            }
            ACTION_CANCEL -> {
                isTrackingTouch = false
                activePath = null
                return true
            }
            else -> return false
        }
    }

    /**
     * Dispatches a scroll event as a quick swipe gesture.
     */
    fun onScrollEvent(normX: Float, normY: Float, deltaX: Float, deltaY: Float): Boolean {
        val dm = resources.displayMetrics
        val width = dm.widthPixels.toFloat()
        val height = dm.heightPixels.toFloat()

        val startX = (normX * width).coerceIn(0f, width - 1f)
        val startY = (normY * height).coerceIn(0f, height - 1f)

        // Scrolling in one direction moves content in that direction, meaning touch swipe is opposite
        val scrollScale = 500f // Scaling factor for scroll delta
        val endX = (startX - deltaX * scrollScale).coerceIn(0f, width - 1f)
        val endY = (startY - deltaY * scrollScale).coerceIn(0f, height - 1f)

        val path = Path().apply {
            moveTo(startX, startY)
            lineTo(endX, endY)
        }
        return dispatchPath(path, 150L)
    }

    /**
     * Performs an Android global navigation action (Back, Home, Recents, etc.).
     */
    fun onGlobalAction(actionType: Int): Boolean {
        val success = performGlobalAction(actionType)
        Log.d(TAG, "performGlobalAction type=$actionType success=$success")
        return success
    }

    private fun dispatchPath(path: Path, durationMs: Long): Boolean {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.N) {
            return false
        }
        val stroke = GestureDescription.StrokeDescription(path, 0, durationMs)
        val gesture = GestureDescription.Builder().addStroke(stroke).build()
        val dispatched = dispatchGesture(gesture, null, null)
        Log.d(TAG, "dispatchGesture dispatched=$dispatched durationMs=$durationMs")
        return dispatched
    }
}
