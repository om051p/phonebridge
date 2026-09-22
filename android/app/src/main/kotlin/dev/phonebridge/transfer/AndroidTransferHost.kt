package dev.phonebridge.transfer

import android.content.ContentValues
import android.content.Context
import android.os.Build
import android.os.Environment
import android.os.ParcelFileDescriptor
import android.provider.MediaStore
import android.util.Log
import dev.phonebridge.bridge.TransferHostCallback
import java.io.File

/**
 * AndroidTransferHost implements the Kotlin side of the file-transfer plane
 * (DEC-024, Phase 4 Step 4).
 *
 * Storage split:
 *  - API 29+: MediaStore.Downloads with IS_PENDING=1. The row is created in
 *    [onBeginDownload], written by Go through the descriptor from
 *    [onOpenPendingFd], and published (IS_PENDING=0) in [onCommitDownload].
 *    A failed publish or an abort deletes the row, so no partial file is ever
 *    visible to the user or to other apps.
 *  - API 26-28: direct writes into the public Downloads directory, which is
 *    the app-allowed location without WRITE_EXTERNAL_STORAGE on those levels.
 *    A pending file lives under the hidden ".phonebridge-partial" prefix and is
 *    renamed into place on commit — the same staged-commit semantics the Go
 *    FileDestination uses on Linux.
 *
 * Every handle is mapped to its pending entry in [entries]; one mutex guards
 * the map, because the Go side may call from arbitrary threads and a local
 * cancel races the writer. Nothing here ever reads file contents: bytes move
 * descriptor→descriptor in Go, so Kotlin memory stays flat no matter how large
 * the file is.
 */
class AndroidTransferHost(private val context: Context) : TransferHostCallback {

    companion object {
        private const val TAG = "AndroidTransferHost"

        /** Directory fallback used below API 29 (public Downloads). */
        private const val FALLBACK_DIR = "Download"

        /** Prefix marking an unfinished fallback file (mirrors the Linux staging name). */
        private const val PARTIAL_PREFIX = ".phonebridge-partial-"

        /** MIME used when the sender does not declare one. */
        private const val DEFAULT_MIME = "application/octet-stream"
    }

    /** One begun download: either a MediaStore row or a fallback file. */
    private sealed interface Entry

    private class MediaStoreEntry(val uri: android.net.Uri) : Entry

    private class FallbackEntry(val partial: File, val target: File) : Entry

    private val lock = Any()
    private val entries = HashMap<String, Entry>()
    private var nextHandle = 0L

    // ---------------------------------------------------------------- begin

    override fun onBeginDownload(filename: String, mimeType: String, sizeBytes: Long): String? {
        val safeName = sanitize(filename) ?: run {
            Log.w(TAG, "refusing unsafe filename: $filename")
            return null
        }
        val mime = mimeType.ifBlank { DEFAULT_MIME }

        val handle = synchronized(lock) { "xfer-${nextHandle++}" }
        return if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
            beginMediaStore(handle, safeName, mime, sizeBytes)
        } else {
            beginFallback(handle, safeName)
        }.also { created ->
            if (created == null) {
                Log.w(TAG, "begin refused for $safeName (size=$sizeBytes)")
            }
        }
    }

    private fun beginMediaStore(handle: String, filename: String, mime: String, sizeBytes: Long): String? {
        val values = ContentValues().apply {
            put(MediaStore.MediaColumns.DISPLAY_NAME, filename)
            put(MediaStore.MediaColumns.MIME_TYPE, mime)
            if (sizeBytes > 0) {
                put(MediaStore.MediaColumns.SIZE, sizeBytes)
            }
            put(MediaStore.MediaColumns.IS_PENDING, 1)
        }
        return try {
            val resolver = context.contentResolver
            val uri = resolver.insert(MediaStore.Downloads.EXTERNAL_CONTENT_URI, values) ?: return null
            synchronized(lock) { entries[handle] = MediaStoreEntry(uri) }
            handle
        } catch (t: Throwable) {
            Log.w(TAG, "MediaStore insert failed: ${t.message}")
            null
        }
    }

    private fun beginFallback(handle: String, filename: String): String? {
        val downloads = Environment.getExternalStoragePublicDirectory(Environment.DIRECTORY_DOWNLOADS)
        return try {
            if (!downloads.exists()) downloads.mkdirs()
            if (!downloads.isDirectory) return null
            val target = uniqueTarget(downloads, filename)
            val partial = File(downloads, PARTIAL_PREFIX + target.name)
            if (partial.exists() && !partial.delete()) return null
            if (!partial.createNewFile()) return null
            synchronized(lock) { entries[handle] = FallbackEntry(partial, target) }
            handle
        } catch (t: Throwable) {
            Log.w(TAG, "fallback begin failed: ${t.message}")
            null
        }
    }

    // ------------------------------------------------------------------- fd

    override fun onOpenPendingFd(handle: String): Int {
        val entry = synchronized(lock) { entries[handle] } ?: return -1
        return try {
            val pfd: ParcelFileDescriptor = when (entry) {
                is MediaStoreEntry ->
                    context.contentResolver.openFileDescriptor(entry.uri, "rw")
                        ?: return -1
                is FallbackEntry ->
                    ParcelFileDescriptor.open(entry.partial, ParcelFileDescriptor.MODE_READ_WRITE)
            }
            // detachFd transfers ownership of the raw int to the caller (Go):
            // Go dups it for its own use, and this process must close the
            // original exactly once — here, immediately, because Go already
            // holds a dup by the time this call returns.
            val fd = pfd.detachFd()
            pfd.close()
            fd
        } catch (t: Throwable) {
            Log.w(TAG, "open pending fd failed: ${t.message}")
            -1
        }
    }

    // --------------------------------------------------------------- commit

    override fun onCommitDownload(handle: String): String? {
        val entry = synchronized(lock) { entries.remove(handle) } ?: return null
        return try {
            when (entry) {
                is MediaStoreEntry -> commitMediaStore(entry)
                is FallbackEntry -> commitFallback(entry)
            }
        } catch (t: Throwable) {
            Log.w(TAG, "commit failed: ${t.message}")
            // Leave the entry deleted-or-pending consistently: clean up here so
            // a half-committed row cannot leak.
            deleteEntry(entry)
            null
        }
    }

    private fun commitMediaStore(entry: MediaStoreEntry): String? {
        val resolver = context.contentResolver
        val name = queryDisplayName(entry.uri)
        val values = ContentValues().apply { put(MediaStore.MediaColumns.IS_PENDING, 0) }
        val updated = resolver.update(entry.uri, values, null, null) > 0
        if (!updated) {
            Log.w(TAG, "MediaStore publish (IS_PENDING=0) failed for $entry.uri")
            return null
        }
        return name
    }

    private fun commitFallback(entry: FallbackEntry): String? {
        // The rename is the publish: partial bytes are invisible until now.
        if (!entry.partial.renameTo(entry.target)) {
            Log.w(TAG, "fallback publish failed for ${entry.target.name}")
            return null
        }
        return entry.target.name
    }

    // ---------------------------------------------------------------- abort

    override fun onAbortDownload(handle: String) {
        val entry = synchronized(lock) { entries.remove(handle) } ?: return
        deleteEntry(entry)
    }

    private fun deleteEntry(entry: Entry) {
        try {
            when (entry) {
                is MediaStoreEntry -> context.contentResolver.delete(entry.uri, null, null)
                is FallbackEntry -> entry.partial.delete()
            }
        } catch (t: Throwable) {
            Log.w(TAG, "delete pending entry failed: ${t.message}")
        }
    }

    // ----------------------------------------------------------- free space

    override fun onFreeSpaceBytes(): Long {
        return try {
            val dir = Environment.getExternalStoragePublicDirectory(Environment.DIRECTORY_DOWNLOADS)
            dir.usableSpace.takeIf { it > 0 } ?: -1L
        } catch (t: Throwable) {
            -1L
        }
    }

    // ----------------------------------------------------------- oversized

    override fun onOversizedFrame(size: Int) {
        Log.w(TAG, "peer sent an oversized transfer frame ($size bytes)")
    }

    // ------------------------------------------------------------- helpers

    private fun queryDisplayName(uri: android.net.Uri): String? {
        return try {
            context.contentResolver.query(
                uri,
                arrayOf(MediaStore.MediaColumns.DISPLAY_NAME),
                null, null, null
            )?.use { cursor ->
                if (cursor.moveToFirst()) cursor.getString(0) else null
            }
        } catch (t: Throwable) {
            null
        }
    }

    /** Plain-basename policy, matching the Go SanitizeFilename rules. */
    private fun sanitize(name: String): String? {
        val trimmed = name.trim().substringAfterLast('/').substringAfterLast('\\')
        if (trimmed.isEmpty() || trimmed == "." || trimmed == "..") return null
        if (trimmed.contains('\u0000')) return null
        val cleaned = trimmed.replace(Regex("[\\r\\n\\t]"), "_")
        return cleaned.take(255)
    }

    /** Picks a target name that does not collide with an existing file. */
    private fun uniqueTarget(dir: File, filename: String): File {
        var candidate = File(dir, filename)
        if (!candidate.exists()) return candidate
        val dot = filename.lastIndexOf('.')
        val base = if (dot > 0) filename.substring(0, dot) else filename
        val ext = if (dot > 0) filename.substring(dot) else ""
        var n = 1
        while (candidate.exists()) {
            candidate = File(dir, "$base ($n)$ext")
            n++
        }
        return candidate
    }
}
