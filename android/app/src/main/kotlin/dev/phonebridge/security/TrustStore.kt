package dev.phonebridge.security

import org.json.JSONArray
import org.json.JSONObject
import java.io.File
import java.nio.charset.StandardCharsets

data class TrustedDeviceRecord(
    val deviceId: String,
    val displayName: String,
    val platform: String,
    val rawPublicKey: ByteArray,
    val pairedAtMs: Long,
    val lastSeenMs: Long,
    val revoked: Boolean
)

/**
 * Thread-safe persistent trust store for PhoneBridge peers on Android.
 *
 * Persists to trusted_devices.json using atomic file write (tmp + rename)
 * to prevent corruption.
 */
class TrustStore(private val storeFile: File) {

    companion object {
        /**
         * Fired (off the store lock) after every trust mutation: add, revoke,
         * remove. Lets UI layers refresh instead of showing stale trust state.
         * A throwing listener must never break persistence, so dispatch is
         * guarded.
         */
        @Volatile
        var changeListener: (() -> Unit)? = null

        private fun notifyChanged() {
            try {
                changeListener?.invoke()
            } catch (_: Throwable) {
                // Listener failures must not break trust persistence.
            }
        }
    }

    private val lock = Any()
    private val devices = mutableMapOf<String, TrustedDeviceRecord>()

    init {
        load()
    }

    private fun load() {
        synchronized(lock) {
            devices.clear()
            if (!storeFile.exists()) return

            try {
                val jsonStr = storeFile.readText(StandardCharsets.UTF_8)
                val root = JSONObject(jsonStr)
                val arr = root.optJSONArray("devices") ?: JSONArray()
                for (i in 0 until arr.length()) {
                    val obj = arr.getJSONObject(i)
                    val devId = obj.getString("device_id")
                    val record = TrustedDeviceRecord(
                        deviceId = devId,
                        displayName = obj.optString("display_name", ""),
                        platform = obj.optString("platform", "linux"),
                        rawPublicKey = CryptoUtils.fromHex(obj.getString("public_key")),
                        pairedAtMs = obj.optLong("paired_at_ms", System.currentTimeMillis()),
                        lastSeenMs = obj.optLong("last_seen_ms", System.currentTimeMillis()),
                        revoked = obj.optBoolean("revoked", false)
                    )
                    devices[devId] = record
                }
            } catch (_: Throwable) {
                // Ignore load error, start fresh
            }
        }
    }

    private fun save() {
        val root = JSONObject()
        val arr = JSONArray()
        for (rec in devices.values) {
            val obj = JSONObject()
            obj.put("device_id", rec.deviceId)
            obj.put("display_name", rec.displayName)
            obj.put("platform", rec.platform)
            obj.put("public_key", CryptoUtils.toHex(rec.rawPublicKey))
            obj.put("paired_at_ms", rec.pairedAtMs)
            obj.put("last_seen_ms", rec.lastSeenMs)
            obj.put("revoked", rec.revoked)
            arr.put(obj)
        }
        root.put("devices", arr)

        val tmpFile = File(storeFile.parentFile, "${storeFile.name}.tmp")
        tmpFile.writeText(root.toString(2), StandardCharsets.UTF_8)
        if (!tmpFile.renameTo(storeFile)) {
            storeFile.delete()
            tmpFile.renameTo(storeFile)
        }
    }

    fun isTrusted(deviceId: String): Boolean {
        synchronized(lock) {
            val rec = devices[deviceId] ?: return false
            return !rec.revoked
        }
    }

    fun get(deviceId: String): TrustedDeviceRecord? {
        synchronized(lock) {
            return devices[deviceId]
        }
    }

    /**
     * Returns the record holding the given raw public key, if any. Revoked
     * records are returned too — the caller decides whether a revoked match
     * counts (re-pair through approval) or not (already-trusted short-circuit).
     * Comparison is constant-time; key material must not leak through timing.
     */
    fun findByPublicKey(rawPub: ByteArray): TrustedDeviceRecord? {
        synchronized(lock) {
            for (rec in devices.values) {
                if (rec.rawPublicKey.size == rawPub.size && rawPub.isNotEmpty() &&
                    java.security.MessageDigest.isEqual(rec.rawPublicKey, rawPub)
                ) {
                    return rec
                }
            }
            return null
        }
    }

    /**
     * Stores a trusted record. The device ID must be the canonical
     * public-key fingerprint; a blank ID is canonicalized, a mismatched ID
     * throws so a second logical record for the same key can never be
     * created (connection-audit Phase A). Callers reconciling a legacy or
     * discovery-derived ID must use [upsertCanonical].
     */
    fun addTrusted(record: TrustedDeviceRecord) {
        val canonical = CryptoUtils.fingerprint(record.rawPublicKey)
        val id = record.deviceId.ifEmpty { canonical }
        require(id == canonical) {
            "device_id does not match public-key fingerprint"
        }
        synchronized(lock) {
            devices[id] = record.copy(deviceId = id, revoked = false)
            save()
        }
        notifyChanged()
    }

    /**
     * Stores a trusted record under its canonical fingerprint, folding any
     * legacy row holding the same key under a different ID. Same-key rows
     * are the same logical device (earliest pairing time preserved);
     * different-key rows are never touched.
     */
    fun upsertCanonical(record: TrustedDeviceRecord) {
        val canonical = CryptoUtils.fingerprint(record.rawPublicKey)
        synchronized(lock) {
            var earliest = record.pairedAtMs
            // The canonical row itself counts: re-pairing it must not move
            // the original pairing time forward.
            devices[canonical]?.let { existing ->
                if (existing.pairedAtMs < earliest) earliest = existing.pairedAtMs
            }
            val it = devices.entries.iterator()
            while (it.hasNext()) {
                val e = it.next()
                if (e.key == canonical) continue
                val v = e.value
                if (v.rawPublicKey.size == record.rawPublicKey.size &&
                    v.rawPublicKey.isNotEmpty() &&
                    java.security.MessageDigest.isEqual(v.rawPublicKey, record.rawPublicKey)
                ) {
                    if (v.pairedAtMs < earliest) earliest = v.pairedAtMs
                    it.remove()
                }
            }
            devices[canonical] = record.copy(
                deviceId = canonical,
                pairedAtMs = earliest,
                lastSeenMs = System.currentTimeMillis(),
                revoked = false,
            )
            save()
        }
        notifyChanged()
    }

    fun revoke(deviceId: String): Boolean {
        val ok = synchronized(lock) {
            val existing = devices[deviceId] ?: return false
            devices[deviceId] = existing.copy(revoked = true)
            save()
            true
        }
        if (ok) notifyChanged()
        return ok
    }

    /**
     * Refreshes LastSeen for a known record without altering identity or
     * revocation state. Presence touches must never un-revoke or re-key.
     */
    fun touchLastSeen(deviceId: String) {
        synchronized(lock) {
            val existing = devices[deviceId] ?: return
            devices[deviceId] = existing.copy(lastSeenMs = System.currentTimeMillis())
            save()
        }
    }

    fun remove(deviceId: String): Boolean {
        val ok = synchronized(lock) {
            if (devices.remove(deviceId) != null) {
                save()
                true
            } else {
                false
            }
        }
        if (ok) notifyChanged()
        return ok
    }

    /**
     * Re-reads the store file. Separate in-process instances (service vs UI)
     * share the file but not memory; readers call this when notified of a
     * change made by another instance. Callers that authenticate against
     * long-lived instances (notably the LAN signaling verifier) must reload
     * before every trust decision: a pairing committed through the UI
     * instance is invisible to the service instance otherwise, and a
     * revocation committed elsewhere must take effect on the next request,
     * never on the next process restart.
     */
    fun reload() {
        load()
    }

    fun list(): List<TrustedDeviceRecord> {
        synchronized(lock) {
            return devices.values.toList()
        }
    }
}
