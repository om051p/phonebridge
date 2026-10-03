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

    fun addTrusted(record: TrustedDeviceRecord) {
        synchronized(lock) {
            devices[record.deviceId] = record
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
     * change made by another instance.
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
