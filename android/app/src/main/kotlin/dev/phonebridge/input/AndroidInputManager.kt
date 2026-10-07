package dev.phonebridge.input

import android.accessibilityservice.AccessibilityService
import android.app.KeyguardManager
import android.content.Context
import android.os.PowerManager
import android.util.Log
import android.view.KeyEvent
import dev.phonebridge.bridge.GoBridge
import dev.phonebridge.bridge.InputHostCallback
import dev.phonebridge.ime.PhoneBridgeImeService

/**
 * AndroidInputManager coordinates incoming remote input from GoBridge (DEC-027).
 *
 * Enforces security gates:
 * 1. Screen-Lock Guard: Drops input if KeyguardManager.isKeyguardLocked or !PowerManager.isInteractive.
 * 2. Zero-Logging Rule: Never logs coordinates, keystrokes, or text.
 */
object AndroidInputManager : InputHostCallback {

    private const val TAG = "AndroidInputManager"

    // Protobuf KeyEvent.Action constants
    const val KEY_ACTION_DOWN = 1
    const val KEY_ACTION_UP = 2

    @Volatile
    private var appContext: Context? = null

    @Volatile
    private var isStarted = false

    fun start(context: Context) {
        appContext = context.applicationContext
        if (!isStarted) {
            try {
                if (GoBridge.inputInit(this)) {
                    isStarted = true
                    Log.i(TAG, "AndroidInputManager initialized with GoBridge")
                } else {
                    Log.w(TAG, "GoBridge.inputInit returned false")
                }
            } catch (t: Throwable) {
                Log.e(TAG, "Failed to initialize GoBridge input: ${t.message}", t)
            }
        }
    }

    fun stop() {
        if (isStarted) {
            try {
                GoBridge.inputStop()
            } catch (t: Throwable) {
                Log.w(TAG, "GoBridge.inputStop error: ${t.message}")
            }
            isStarted = false
        }
        appContext = null
        Log.i(TAG, "AndroidInputManager stopped")
    }

    /**
     * Screen-Lock Guard (DEC-027): Returns true only if the device is awake and unlocked.
     */
    fun isDeviceUnlocked(): Boolean {
        val ctx = appContext ?: return false
        val pm = ctx.getSystemService(Context.POWER_SERVICE) as? PowerManager
        if (pm != null && !pm.isInteractive) {
            Log.w(TAG, "Input dropped: device screen is off (non-interactive)")
            return false
        }
        val km = ctx.getSystemService(Context.KEYGUARD_SERVICE) as? KeyguardManager
        if (km != null && km.isKeyguardLocked) {
            Log.w(TAG, "Input dropped: keyguard is locked")
            return false
        }
        return true
    }

    override fun onTouch(action: Int, pointerId: Int, normX: Float, normY: Float, pressure: Float): Boolean {
        if (!isDeviceUnlocked()) return false
        val acc = PhoneBridgeAccessibilityService.getInstance()
        if (acc == null) {
            Log.w(TAG, "Input dropped: AccessibilityService not connected")
            return false
        }
        Log.d(TAG, "onTouch action=$action pointerId=$pointerId")
        return acc.onTouchEvent(action, pointerId, normX, normY, pressure)
    }

    override fun onKey(action: Int, keyCode: Int, metaState: Int): Boolean {
        if (!isDeviceUnlocked()) return false
        Log.d(TAG, "onKey action=$action keyCode=$keyCode")
        val acc = PhoneBridgeAccessibilityService.getInstance()
        if (keyCode == KeyEvent.KEYCODE_BACK || keyCode == KeyEvent.KEYCODE_ESCAPE) {
            if (action == KEY_ACTION_UP || action == KeyEvent.ACTION_UP) {
                return acc?.onGlobalAction(AccessibilityService.GLOBAL_ACTION_BACK) ?: false
            }
            return true
        }
        if (keyCode == KeyEvent.KEYCODE_HOME) {
            if (action == KEY_ACTION_UP || action == KeyEvent.ACTION_UP) {
                return acc?.onGlobalAction(AccessibilityService.GLOBAL_ACTION_HOME) ?: false
            }
            return true
        }
        if (keyCode == KeyEvent.KEYCODE_DEL) {
            if (action == KEY_ACTION_DOWN || action == KeyEvent.ACTION_DOWN) {
                if (PhoneBridgeImeService.sendKeyEvent(KeyEvent.KEYCODE_DEL)) {
                    return true
                }
                return acc?.deleteLastChar() ?: false
            }
            return true
        }
        if (keyCode == KeyEvent.KEYCODE_ENTER) {
            if (action == KEY_ACTION_DOWN || action == KeyEvent.ACTION_DOWN) {
                if (PhoneBridgeImeService.sendKeyEvent(KeyEvent.KEYCODE_ENTER)) {
                    return true
                }
                return acc?.performEnterAction() ?: false
            }
            return true
        }
        return false
    }

    override fun onText(text: String): Boolean {
        if (!isDeviceUnlocked()) return false
        Log.d(TAG, "onText commit length=${text.length}")
        if (PhoneBridgeImeService.commitText(text)) {
            return true
        }
        val acc = PhoneBridgeAccessibilityService.getInstance()
        return acc?.appendText(text) ?: false
    }

    override fun onScroll(normX: Float, normY: Float, deltaX: Float, deltaY: Float): Boolean {
        if (!isDeviceUnlocked()) return false
        val acc = PhoneBridgeAccessibilityService.getInstance()
        if (acc == null) {
            Log.w(TAG, "Input dropped: AccessibilityService not connected")
            return false
        }
        Log.d(TAG, "onScroll")
        return acc.onScrollEvent(normX, normY, deltaX, deltaY)
    }

    override fun onGlobalAction(actionType: Int): Boolean {
        if (!isDeviceUnlocked()) return false
        val acc = PhoneBridgeAccessibilityService.getInstance()
        if (acc == null) {
            Log.w(TAG, "Input dropped: AccessibilityService not connected")
            return false
        }
        Log.d(TAG, "onGlobalAction type=$actionType")
        return acc.onGlobalAction(actionType)
    }
}
