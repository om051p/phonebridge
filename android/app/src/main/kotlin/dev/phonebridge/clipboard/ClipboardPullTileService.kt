package dev.phonebridge.clipboard

import android.annotation.SuppressLint
import android.app.PendingIntent
import android.content.Intent
import android.os.Build
import android.os.Handler
import android.os.Looper
import android.service.quicksettings.Tile
import android.service.quicksettings.TileService
import android.util.Log
import androidx.annotation.RequiresApi

/**
 * Quick Settings Tile service providing Tier-2 explicit manual clipboard pull (DEC-023).
 *
 * Pushes the phone's current clip to the paired desktop on demand, for users
 * who have not selected the companion keyboard.
 *
 * The tile itself cannot read the clipboard: Android 10+ grants the read only
 * to the uid that owns the focused window, and while the shade is open that is
 * SystemUI. Reading inline here silently returned null on every tap, so the
 * tile launches [ClipboardSyncActivity] — a focus-holding, invisible activity —
 * and lets that perform the read.
 */
@RequiresApi(Build.VERSION_CODES.N)
class ClipboardPullTileService : TileService() {

    companion object {
        private const val TAG = "ClipboardPullTile"
    }

    private val mainHandler = Handler(Looper.getMainLooper())

    override fun onClick() {
        super.onClick()
        Log.i(TAG, "Quick Settings Tile clicked; requesting a foreground clipboard sync")

        val launched = launchSyncActivity()
        if (!launched) {
            // Fallback keeps the tap from being a no-op: it only succeeds when
            // this process already owns the focused window.
            val ok = AndroidClipboardAdapter.triggerManualPull()
            Log.i(TAG, "Inline clipboard pull fallback; forwarded=$ok")
        }

        // Flash tile state to provide visual feedback to the user
        val tile = qsTile ?: return
        tile.state = Tile.STATE_ACTIVE
        tile.updateTile()

        mainHandler.postDelayed({
            updateTileState()
        }, 600)
    }

    // The pre-34 fallback is the point: on older builds the PendingIntent
    // overload does not exist, so the deprecated Intent overload IS the only
    // way to collapse the shade and still launch (Lint: StartActivityAndCollapseDeprecated).
    @SuppressLint("StartActivityAndCollapseDeprecated")
    private fun launchSyncActivity(): Boolean {
        val intent = Intent(this, ClipboardSyncActivity::class.java).apply {
            addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TOP)
        }
        return try {
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.UPSIDE_DOWN_CAKE) {
                val pending = PendingIntent.getActivity(
                    this,
                    0,
                    intent,
                    PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT
                )
                startActivityAndCollapse(pending)
            } else {
                @Suppress("DEPRECATION")
                startActivityAndCollapse(intent)
            }
            true
        } catch (t: Throwable) {
            Log.w(TAG, "Could not launch clipboard sync activity: ${t.message}")
            false
        }
    }

    override fun onStartListening() {
        super.onStartListening()
        updateTileState()
    }

    private fun updateTileState() {
        val tile = qsTile ?: return
        val state = AndroidClipboardAdapter.state

        tile.label = "Sync Clipboard"
        // DORMANT means "ready for an explicit push", not broken: both running
        // states can forward a clip the moment the user asks for one.
        tile.state = when (state) {
            AdapterState.AMBIENT_ACTIVE, AdapterState.WRITE_ONLY_DORMANT -> Tile.STATE_ACTIVE
            AdapterState.UNAVAILABLE -> Tile.STATE_UNAVAILABLE
            AdapterState.STOPPED -> Tile.STATE_INACTIVE
        }
        tile.updateTile()
    }
}
