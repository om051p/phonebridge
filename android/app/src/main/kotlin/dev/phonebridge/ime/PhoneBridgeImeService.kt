package dev.phonebridge.ime

import android.content.ClipboardManager
import android.content.Context
import android.graphics.Color
import android.inputmethodservice.InputMethodService
import android.os.Build
import android.util.Log
import android.view.Gravity
import android.view.View
import android.view.ViewGroup
import android.view.inputmethod.EditorInfo
import android.view.inputmethod.InputMethodManager
import android.widget.Button
import android.widget.LinearLayout
import android.widget.TextView
import dev.phonebridge.clipboard.AndroidClipboardAdapter

/**
 * PhoneBridge companion InputMethodService (DEC-023 Tier 1).
 *
 * Provides ambient clipboard observation when selected as the default input method
 * and bound to an active foreground window session (mVisibleBound=true).
 *
 * Displays a minimal 44dp accessory strip with a "Switch Keyboard" action when
 * an input view is requested. Never behaves like a replacement software keyboard,
 * and never intercepts, logs, or alters keystrokes.
 */
class PhoneBridgeImeService : InputMethodService() {

    companion object {
        private const val TAG = "PhoneBridgeIme"
        private const val ACCESSORY_BAR_HEIGHT_DP = 44
    }

    private var clipboardManager: ClipboardManager? = null
    private var clipListener: ClipboardManager.OnPrimaryClipChangedListener? = null

    override fun onCreate() {
        super.onCreate()
        Log.i(TAG, "PhoneBridgeImeService onCreate pid=${android.os.Process.myPid()}")

        val cm = getSystemService(Context.CLIPBOARD_SERVICE) as? ClipboardManager
        clipboardManager = cm

        if (cm != null) {
            val listener = ClipboardManager.OnPrimaryClipChangedListener {
                Log.d(TAG, "OnPrimaryClipChangedListener triggered in companion IME")
                AndroidClipboardAdapter.readAndForwardCurrentClip()
            }
            try {
                cm.addPrimaryClipChangedListener(listener)
                clipListener = listener
            } catch (t: Throwable) {
                Log.w(TAG, "Failed to register OnPrimaryClipChangedListener: ${t.message}")
            }
        }

        AndroidClipboardAdapter.setImeSelected(true)
    }

    override fun onDestroy() {
        Log.i(TAG, "PhoneBridgeImeService onDestroy")
        clipListener?.let { listener ->
            try {
                clipboardManager?.removePrimaryClipChangedListener(listener)
            } catch (t: Throwable) {
                Log.w(TAG, "Failed to remove primary clip listener: ${t.message}")
            }
        }
        clipListener = null

        AndroidClipboardAdapter.setImeBound(false)
        AndroidClipboardAdapter.setImeSelected(false)
        super.onDestroy()
    }

    override fun onStartInput(attribute: EditorInfo?, restarting: Boolean) {
        super.onStartInput(attribute, restarting)
        Log.d(TAG, "onStartInput restarting=$restarting pkg=${attribute?.packageName}")
        AndroidClipboardAdapter.setImeBound(true)
        AndroidClipboardAdapter.setImeSelected(true)
    }

    override fun onFinishInput() {
        Log.d(TAG, "onFinishInput")
        AndroidClipboardAdapter.setImeBound(false)
        super.onFinishInput()
    }

    override fun onCreateInputView(): View {
        val density = resources.displayMetrics.density
        val barHeight = (ACCESSORY_BAR_HEIGHT_DP * density).toInt()

        val root = LinearLayout(this).apply {
            orientation = LinearLayout.HORIZONTAL
            layoutParams = ViewGroup.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT,
                barHeight
            )
            gravity = Gravity.CENTER_VERTICAL
            setBackgroundColor(Color.parseColor("#1E1E1E"))
            val pad = (8 * density).toInt()
            setPadding(pad, 0, pad, 0)
        }

        val label = TextView(this).apply {
            text = "PhoneBridge Clipboard Sync"
            setTextColor(Color.WHITE)
            textSize = 13f
            layoutParams = LinearLayout.LayoutParams(0, ViewGroup.LayoutParams.WRAP_CONTENT, 1.0f)
        }
        root.addView(label)

        val switchButton = Button(this).apply {
            text = "Switch Keyboard"
            textSize = 12f
            setTextColor(Color.WHITE)
            setBackgroundColor(Color.parseColor("#333333"))
            setOnClickListener {
                switchBackToPreviousIme()
            }
        }
        root.addView(switchButton)

        return root
    }

    private fun switchBackToPreviousIme() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.P) {
            val switched = try {
                switchToPreviousInputMethod()
            } catch (t: Throwable) {
                false
            }
            if (!switched) {
                showPickerFallback()
            }
        } else {
            showPickerFallback()
        }
    }

    private fun showPickerFallback() {
        val imm = getSystemService(Context.INPUT_METHOD_SERVICE) as? InputMethodManager
        imm?.showInputMethodPicker()
    }
}
