package dev.phonebridge.clipboard

import java.util.concurrent.atomic.AtomicReference

/**
 * The one in-memory slot that carries a locally-read clipboard item across a
 * session bootstrap (DEC-023 cold start).
 *
 * Why this exists: the Go clipboard engine's `currentItem` is live clipboard
 * state, not a delivery buffer. On a freshly established session the peer's
 * reconnect-sync item can be applied before the local channel-open flush runs,
 * which replaces `currentItem` — a clip read a second earlier, while the
 * transport was still down, would then be silently lost (verified on device:
 * the phone's just-copied marker never reached the desktop in 3 of 3 cold
 * starts). This slot lives on the Android side, outside that exchange: the
 * peer's item may update engine state freely and can never clear or overwrite
 * what is held here.
 *
 * Lifetime contract (deliberately minimal):
 *  - holds the LATEST read item only — a newer local copy supersedes the held
 *    one, so this is a slot, never a queue or history;
 *  - survives entirely in memory: nothing is written to disk;
 *  - is cleared by [clearIfSame] only after the item it names was handed to
 *    the connected transport (or knowingly superseded), so a late
 *    acknowledgement can never discard a newer held copy;
 *  - carries no clipboard content anywhere except its own byte array, so no
 *    caller can log what it holds beyond counts.
 */
internal class PendingLocalClipSlot {

    /** One read clipboard item: the exact bytes, their MIME type, and the read time. */
    class Item(val mimeType: String, val payload: ByteArray, val copiedAtMs: Long)

    private val held = AtomicReference<Item?>(null)

    /** Whether any item is currently held. */
    val isEmpty: Boolean
        get() = held.get() == null

    /** The held item, or null. The caller must treat the payload as opaque bytes. */
    fun peek(): Item? = held.get()

    /**
     * Holds [payload] as the pending local item. Latest wins by construction:
     * an item already held is superseded rather than queued.
     */
    fun hold(mimeType: String, payload: ByteArray, copiedAtMs: Long) {
        held.set(Item(mimeType, payload, copiedAtMs))
    }

    /**
     * Clears the slot after the transport took [sent]. Compare-and-set on
     * purpose: if a newer local copy arrived while the send acknowledgement was
     * in flight, that newer item survives.
     */
    fun clearIfSame(sent: Item) {
        held.compareAndSet(sent, null)
    }

    /** Drops whatever is held (engine stopped, or the item is knowingly superseded). */
    fun clear() {
        held.set(null)
    }
}
