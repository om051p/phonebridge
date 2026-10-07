package dev.phonebridge.input

import android.accessibilityservice.AccessibilityService
import android.accessibilityservice.GestureDescription
import android.content.Context
import android.content.Intent
import android.graphics.Path
import android.os.Build
import android.os.Bundle
import android.os.SystemClock
import android.util.DisplayMetrics
import android.util.Log
import android.view.WindowManager
import android.view.accessibility.AccessibilityEvent
import android.view.accessibility.AccessibilityNodeInfo
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

        const val ACTION_DOWN = 1
        const val ACTION_MOVE = 2
        const val ACTION_UP = 3
        const val ACTION_CANCEL = 4

        @Volatile
        private var captureWidth: Float = 720f

        @Volatile
        private var captureHeight: Float = 1600f

        fun setCaptureDimensions(width: Int, height: Int) {
            if (width > 0 && height > 0) {
                captureWidth = width.toFloat()
                captureHeight = height.toFloat()
            }
        }
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
     * Retrieves true physical display size in pixels.
     */
    fun getPhysicalScreenSize(): Pair<Float, Float> {
        return try {
            val wm = getSystemService(Context.WINDOW_SERVICE) as? WindowManager
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R && wm != null) {
                val bounds = wm.maximumWindowMetrics.bounds
                Pair(bounds.width().toFloat(), bounds.height().toFloat())
            } else if (wm != null) {
                val dm = DisplayMetrics()
                @Suppress("DEPRECATION")
                wm.defaultDisplay.getRealMetrics(dm)
                Pair(dm.widthPixels.toFloat(), dm.heightPixels.toFloat())
            } else {
                val dm = resources.displayMetrics
                Pair(dm.widthPixels.toFloat(), dm.heightPixels.toFloat())
            }
        } catch (t: Throwable) {
            val dm = resources.displayMetrics
            Pair(dm.widthPixels.toFloat(), dm.heightPixels.toFloat())
        }
    }

    /**
     * Maps normalized coordinates [0.0, 1.0] from virtual display frame to physical screen pixels.
     * Accurately accounts for aspect ratio, letterboxing, pillarboxing, and crop offsets.
     */
    fun mapToPhysicalCoordinates(normX: Float, normY: Float): Pair<Float, Float> {
        val (physW, physH) = getPhysicalScreenSize()
        val captW = captureWidth
        val captH = captureHeight

        val physAspect = if (physH > 0f) physW / physH else 1f
        val captAspect = if (captH > 0f) captW / captH else 1f

        val contentW: Float
        val contentH: Float
        val cropOffsetX: Float
        val cropOffsetY: Float

        if (physAspect > captAspect) {
            // Physical screen is wider than capture frame: fits by width, letterboxed vertically
            contentW = captW
            contentH = captW / physAspect
            cropOffsetX = 0f
            cropOffsetY = (captH - contentH) / 2f
        } else {
            // Physical screen is taller than capture frame: fits by height, pillarboxed horizontally
            contentH = captH
            contentW = captH * physAspect
            cropOffsetX = (captW - contentW) / 2f
            cropOffsetY = 0f
        }

        val captX = normX * captW
        val captY = normY * captH

        val contentX = (captX - cropOffsetX).coerceIn(0f, contentW)
        val contentY = (captY - cropOffsetY).coerceIn(0f, contentH)

        val physNormX = if (contentW > 0f) (contentX / contentW).coerceIn(0f, 1f) else 0.5f
        val physNormY = if (contentH > 0f) (contentY / contentH).coerceIn(0f, 1f) else 0.5f

        val px = (physNormX * physW).coerceIn(0f, physW - 1f)
        val py = (physNormY * physH).coerceIn(0f, physH - 1f)
        return Pair(px, py)
    }

    /**
     * Handles incoming normalized touch events from Go bridge.
     */
    fun onTouchEvent(action: Int, pointerId: Int, normX: Float, normY: Float, pressure: Float): Boolean {
        val (px, py) = mapToPhysicalCoordinates(normX, normY)

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
        val (startX, startY) = mapToPhysicalCoordinates(normX, normY)
        val (physW, physH) = getPhysicalScreenSize()

        // Scrolling in one direction moves content in that direction, meaning touch swipe is opposite
        val scrollScale = (physH * 0.25f).coerceAtLeast(300f)
        val endX = (startX - deltaX * scrollScale).coerceIn(0f, physW - 1f)
        val endY = (startY - deltaY * scrollScale).coerceIn(0f, physH - 1f)

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

    /**
     * Commits text into the currently focused input field via accessibility.
     */
    fun appendText(text: String): Boolean {
        val root = rootInActiveWindow ?: return false
        val focused = findFocus(AccessibilityNodeInfo.FOCUS_INPUT)
            ?: root.findFocus(AccessibilityNodeInfo.FOCUS_INPUT)
            ?: return false
        val currentText = focused.text?.toString() ?: ""
        val args = Bundle().apply {
            putCharSequence(AccessibilityNodeInfo.ACTION_ARGUMENT_SET_TEXT_CHARSEQUENCE, currentText + text)
        }
        return focused.performAction(AccessibilityNodeInfo.ACTION_SET_TEXT, args)
    }

    /**
     * Deletes the trailing character from the currently focused input field.
     */
    fun deleteLastChar(): Boolean {
        val root = rootInActiveWindow ?: return false
        val focused = findFocus(AccessibilityNodeInfo.FOCUS_INPUT)
            ?: root.findFocus(AccessibilityNodeInfo.FOCUS_INPUT)
            ?: return false
        val currentText = focused.text?.toString() ?: ""
        if (currentText.isEmpty()) return false
        val newText = currentText.substring(0, currentText.length - 1)
        val args = Bundle().apply {
            putCharSequence(AccessibilityNodeInfo.ACTION_ARGUMENT_SET_TEXT_CHARSEQUENCE, newText)
        }
        return focused.performAction(AccessibilityNodeInfo.ACTION_SET_TEXT, args)
    }

    /**
     * Triggers enter / primary action on the currently focused input field.
     */
    fun performEnterAction(): Boolean {
        val root = rootInActiveWindow ?: return false
        val focused = findFocus(AccessibilityNodeInfo.FOCUS_INPUT)
            ?: root.findFocus(AccessibilityNodeInfo.FOCUS_INPUT)
            ?: return false
        return focused.performAction(AccessibilityNodeInfo.ACTION_CLICK)
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
