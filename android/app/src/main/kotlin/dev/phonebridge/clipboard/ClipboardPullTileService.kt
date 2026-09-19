package dev.phonebridge.clipboard

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
 * Enables user to pull platform clipboard into PhoneBridge sync engine on-demand
 * when ambient IME observation is dormant or unavailable.
 */
@RequiresApi(Build.VERSION_CODES.N)
class ClipboardPullTileService : TileService() {

    companion object {
        private const val TAG = "ClipboardPullTile"
    }

    private val mainHandler = Handler(Looper.getMainLooper())

    override fun onClick() {
        super.onClick()
        Log.i(TAG, "Quick Settings Tile clicked; triggering manual clipboard pull")

        val tile = qsTile ?: return
        val ok = AndroidClipboardAdapter.triggerManualPull()

        // Flash tile state to provide visual feedback to the user
        tile.state = if (ok) Tile.STATE_ACTIVE else Tile.STATE_INACTIVE
        tile.updateTile()

        mainHandler.postDelayed({
            updateTileState()
        }, 600)
    }

    override fun onStartListening() {
        super.onStartListening()
        updateTileState()
    }

    private fun updateTileState() {
        val tile = qsTile ?: return
        val state = AndroidClipboardAdapter.state

        tile.label = "Sync Clipboard"
        tile.state = when (state) {
            AdapterState.AMBIENT_ACTIVE -> Tile.STATE_ACTIVE
            AdapterState.WRITE_ONLY_DORMANT -> Tile.STATE_INACTIVE
            AdapterState.UNAVAILABLE -> Tile.STATE_UNAVAILABLE
            AdapterState.STOPPED -> Tile.STATE_INACTIVE
        }
        tile.updateTile()
    }
}
