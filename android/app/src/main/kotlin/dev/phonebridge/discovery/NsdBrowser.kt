package dev.phonebridge.discovery

import android.content.Context
import android.net.nsd.NsdManager
import android.net.nsd.NsdServiceInfo
import android.util.Log
import java.util.concurrent.ConcurrentHashMap

/**
 * One resolved DNS-SD instance, normalized away from the Android framework
 * types so the browse state machine is unit-testable.
 */
internal data class NsdPeer(
    val serviceName: String,
    val deviceId: String?,
    val deviceName: String?,
    val model: String?,
    val version: String?,
    val host: String?,
    val port: Int,
)

/**
 * Browse seam over Android's platform DNS-SD.
 *
 * The production implementation wraps [NsdManager] (service discovery, the
 * resolve round-trip for one instance, TXT decoding, and the platform's
 * one-resolve-at-a-time constraint); tests inject a fake so peer-map behaviour
 * is exercised without the framework.
 */
internal interface NsdBrowseClient {
    /** Starts discovery. Returns false when the platform refuses to start. */
    fun startDiscovery(
        onServiceFound: (String) -> Unit,
        onServiceLost: (String) -> Unit,
        onFailure: (Int) -> Unit,
    ): Boolean

    /** Resolves one discovered instance to an address/port/TXT record. */
    fun resolve(
        serviceName: String,
        onResolved: (NsdPeer) -> Unit,
        onFailed: (Int) -> Unit,
    )

    fun stopDiscovery()
}

private class PlatformNsdBrowseClient(private val nsdManager: NsdManager) : NsdBrowseClient {

    private class Found(val info: NsdServiceInfo)

    private val found = ConcurrentHashMap<String, Found>()

    override fun startDiscovery(
        onServiceFound: (String) -> Unit,
        onServiceLost: (String) -> Unit,
        onFailure: (Int) -> Unit,
    ): Boolean {
        val listener = object : NsdManager.DiscoveryListener {
            override fun onDiscoveryStarted(serviceType: String) {
                Log.i(TAG, "Platform discovery started for $serviceType")
            }

            override fun onServiceFound(serviceInfo: NsdServiceInfo) {
                val name = serviceInfo.serviceName ?: return
                found[name] = Found(serviceInfo)
                onServiceFound(name)
            }

            override fun onServiceLost(serviceInfo: NsdServiceInfo) {
                val name = serviceInfo.serviceName ?: return
                found.remove(name)
                onServiceLost(name)
            }

            override fun onDiscoveryStopped(serviceType: String) {
                Log.i(TAG, "Platform discovery stopped for $serviceType")
                found.clear()
            }

            override fun onStartDiscoveryFailed(serviceType: String, errorCode: Int) {
                Log.w(TAG, "Platform discovery failed to start for $serviceType: $errorCode")
                onFailure(errorCode)
            }

            override fun onStopDiscoveryFailed(serviceType: String, errorCode: Int) {
                // Stopping is best-effort; the listener is already retired by
                // the caller, so a failure here only needs to be visible.
                Log.w(TAG, "Platform discovery failed to stop for $serviceType: $errorCode")
            }
        }

        return try {
            nsdManager.discoverServices(NsdBrowser.SERVICE_TYPE, NsdManager.PROTOCOL_DNS_SD, listener)
            true
        } catch (t: Throwable) {
            Log.e(TAG, "Exception during nsdManager.discoverServices: ${t.message}", t)
            false
        }
    }

    override fun resolve(
        serviceName: String,
        onResolved: (NsdPeer) -> Unit,
        onFailed: (Int) -> Unit,
    ) {
        val info = found[serviceName]?.info
        if (info == null) {
            // The instance vanished between discovery and resolution.
            onFailed(NsdManager.FAILURE_INTERNAL_ERROR)
            return
        }

        val listener = object : NsdManager.ResolveListener {
            override fun onResolveFailed(serviceInfo: NsdServiceInfo, errorCode: Int) {
                onFailed(errorCode)
            }

            override fun onServiceResolved(serviceInfo: NsdServiceInfo) {
                onResolved(peerFrom(serviceInfo))
            }
        }

        try {
            nsdManager.resolveService(info, listener)
        } catch (t: Throwable) {
            Log.w(TAG, "Exception during nsdManager.resolveService: ${t.message}")
            onFailed(-1)
        }
    }

    override fun stopDiscovery() {
        // NsdManager owns the listener; the browser drops its reference and the
        // platform stops delivering to it. Kept for the seam's symmetry.
    }

    private fun peerFrom(info: NsdServiceInfo): NsdPeer {
        fun txt(key: String): String? =
            info.attributes?.get(key)?.toString(Charsets.UTF_8)?.takeIf { it.isNotBlank() }

        return NsdPeer(
            serviceName = info.serviceName ?: "",
            deviceId = txt("id"),
            deviceName = txt("name"),
            model = txt("model"),
            version = txt("v"),
            // The UI composes an http:// URL from host:port, so only a literal
            // address is usable here — an unresolved instance is dropped by the
            // browser rather than rendered as an undialable row.
            host = info.host?.hostAddress,
            port = info.port,
        )
    }

    private companion object {
        const val TAG = "NsdBrowser"
    }
}

private const val TAG = "NsdBrowser"

/**
 * NsdBrowser lists PhoneBridge peers on the LAN through Android's platform
 * DNS-SD implementation.
 *
 * Why not the Go core's own mDNS client: it enumerates interfaces with a netlink
 * route socket, and Android denies that to every unprivileged app
 * (`avc: denied { bind } ... tclass=netlink_route_socket`), so the core browse
 * can never start on a phone and the Devices tab can never list a PC. Android's
 * [NsdManager] is the supported enumeration path — the same one this app already
 * uses to advertise itself (DEC-007) — so browsing goes through it here.
 *
 * Peers are keyed by the TXT `id` (the peer's Ed25519 device id) and exposed with
 * the same field names the Go browse used, so the UI consumes one shape:
 * id/name/model/version/host/port/isStale. A peer disappears when the platform
 * reports it lost; `isStale` is always false because the platform has no ageing
 * notion to report.
 */
class NsdBrowser internal constructor(
    private val client: NsdBrowseClient?,
) {

    constructor(context: Context) : this(
        (context.getSystemService(Context.NSD_SERVICE) as? NsdManager)?.let { PlatformNsdBrowseClient(it) }
    )

    companion object {
        const val SERVICE_TYPE = "_phonebridge._tcp"
    }

    private val lock = Any()
    private var generation = 0

    private val peers = ConcurrentHashMap<String, Map<String, Any?>>()

    /** Instance name -> peer key, so a lost service removes the right row. */
    private val peerKeyByServiceName = ConcurrentHashMap<String, String>()

    // NsdManager resolves one instance at a time (a second concurrent resolve
    // fails with FAILURE_ALREADY_ACTIVE), so resolutions are serialized.
    private val resolveQueue = ArrayDeque<String>()
    private var resolving = false

    @Volatile
    var isBrowsing = false
        private set

    /**
     * Invoked when the platform reports that discovery could not start. The
     * service uses this to schedule the same bounded LAN recovery it already
     * runs for a failed advertisement.
     */
    @Volatile
    var onBrowseFailed: ((Int) -> Unit)? = null

    /**
     * Starts browsing. Idempotent: a call while discovery is live is a no-op so
     * lifecycle signals can safely re-assert it.
     */
    fun start(): Boolean {
        val c = client
        if (c == null) {
            Log.e(TAG, "NsdManager unavailable on this device")
            return false
        }
        synchronized(lock) {
            if (isBrowsing) return true
            val gen = ++generation
            val started = c.startDiscovery(
                onServiceFound = { serviceName -> enqueueResolve(serviceName, gen) },
                onServiceLost = { serviceName ->
                    if (gen == generation) {
                        val key = peerKeyByServiceName.remove(serviceName)
                        if (key != null) {
                            peers.remove(key)
                            Log.i(TAG, "Peer lost: $serviceName")
                        }
                    }
                },
                onFailure = { errorCode ->
                    if (gen == generation) {
                        isBrowsing = false
                        onBrowseFailed?.invoke(errorCode)
                    }
                },
            )
            isBrowsing = started
            return started
        }
    }

    /** Restarts browsing: retires the current session and starts exactly one. */
    fun restart(): Boolean {
        stop()
        return start()
    }

    /** Stops browsing and forgets the peer snapshot. */
    fun stop() {
        synchronized(lock) {
            generation++
            isBrowsing = false
            client?.stopDiscovery()
        }
        synchronized(resolveQueue) {
            resolveQueue.clear()
            resolving = false
        }
        peers.clear()
        peerKeyByServiceName.clear()
    }

    /** Current peer snapshot, ordered by display name then id for a stable UI. */
    fun snapshot(): List<Map<String, Any?>> =
        peers.values.sortedWith(
            compareBy({ (it["name"] as? String) ?: "" }, { (it["id"] as? String) ?: "" })
        )

    private fun enqueueResolve(serviceName: String, gen: Int) {
        val next: String?
        synchronized(resolveQueue) {
            resolveQueue.addLast(serviceName)
            if (resolving) return
            resolving = true
            next = resolveQueue.removeFirstOrNull()
        }
        resolveNext(next, gen)
    }

    private fun resolveNext(serviceName: String?, gen: Int) {
        val c = client
        if (serviceName == null || c == null) {
            synchronized(resolveQueue) { resolving = false }
            return
        }
        if (gen != generation) {
            synchronized(resolveQueue) { resolving = false }
            return
        }

        c.resolve(
            serviceName,
            onResolved = { peer ->
                if (gen == generation) {
                    val id = peer.deviceId?.takeIf { it.isNotBlank() }
                    val host = peer.host?.takeIf { it.isNotBlank() }
                    if (id != null && host != null && peer.port > 0) {
                        peers[id] = mapOf(
                            "id" to id,
                            "name" to (peer.deviceName ?: id),
                            "model" to (peer.model ?: ""),
                            "version" to (peer.version ?: ""),
                            "host" to host,
                            "port" to peer.port,
                            "isStale" to false,
                        )
                        peerKeyByServiceName[serviceName] = id
                        Log.i(TAG, "Peer resolved: ${peer.deviceName ?: id} at $host:${peer.port}")
                    } else {
                        // An instance without an id, address, or port cannot be
                        // connected to, so it is not offered as a row.
                        Log.i(TAG, "Ignoring unresolvable instance: $serviceName")
                    }
                }
                advance(gen)
            },
            onFailed = { errorCode ->
                Log.w(TAG, "Resolve failed for $serviceName: $errorCode")
                advance(gen)
            },
        )
    }

    private fun advance(gen: Int) {
        val next: String?
        synchronized(resolveQueue) {
            next = resolveQueue.removeFirstOrNull()
            if (next == null) resolving = false
        }
        resolveNext(next, gen)
    }
}
