package dev.phonebridge.discovery

import android.content.Context
import android.net.nsd.NsdManager
import android.net.nsd.NsdServiceInfo
import android.os.Build
import android.util.Log

/**
 * Minimal seam over [NsdManager] so the registration state machine is
 * unit-testable without the Android framework (the device-side behaviour is
 * still covered by NsdAdvertiserDeviceTest).
 */
internal interface NsdRegistrar {
    fun register(info: NsdServiceInfo, protocol: Int, listener: NsdManager.RegistrationListener)
    fun unregister(listener: NsdManager.RegistrationListener)
}

private class SystemNsdRegistrar(private val nsdManager: NsdManager) : NsdRegistrar {
    override fun register(info: NsdServiceInfo, protocol: Int, listener: NsdManager.RegistrationListener) {
        nsdManager.registerService(info, protocol, listener)
    }

    override fun unregister(listener: NsdManager.RegistrationListener) {
        nsdManager.unregisterService(listener)
    }
}

/**
 * NsdAdvertiser advertises the PhoneBridge device presence over local network
 * via Android Network Service Discovery (mDNS / DNS-SD).
 *
 * Conforms to DEC-007 and DEC-018:
 * - Service type: _phonebridge._tcp
 * - Domain: local.
 * - TXT records: id, name, model, v, caps, state
 *
 * Registration lifecycle: registrations are idempotent ([registerService] while
 * a registration is live or awaiting its callback is ignored) and re-registrable
 * ([reregisterService] performs exactly one clean unregister-then-register
 * cycle). Callbacks carry a generation token, so a late callback from a retired
 * listener can never clear the state of a newer registration — which is what
 * makes recovery from a torn-down advertisement safe to attempt repeatedly.
 */
class NsdAdvertiser internal constructor(
    private val registrar: NsdRegistrar?,
) {

    constructor(context: Context) : this(
        (context.getSystemService(Context.NSD_SERVICE) as? NsdManager)?.let { SystemNsdRegistrar(it) }
    )

    companion object {
        private const val TAG = "NsdAdvertiser"
        const val SERVICE_TYPE = "_phonebridge._tcp"
    }

    private class RegistrationParams(
        val port: Int,
        val deviceId: String,
        val deviceName: String,
        val capabilities: String,
        val state: String,
    )

    private val lock = Any()
    private var registrationListener: NsdManager.RegistrationListener? = null
    private var generation = 0
    private var lastParams: RegistrationParams? = null

    @Volatile
    var isRegistered = false
        private set

    /** True between [registerService] and the platform's registration callback. */
    @Volatile
    var isRegistrationPending = false
        private set

    @Volatile
    var registeredServiceName: String? = null
        private set

    /**
     * Invoked when the platform reports a registration failure. The service uses
     * this to schedule a bounded retry: without it a failed registration left
     * discovery dead until the process restarted.
     */
    @Volatile
    var onRegistrationFailed: ((Int) -> Unit)? = null

    /**
     * Registers the mDNS advertisement for this device.
     *
     * Idempotent: a call while a registration is live or pending is ignored
     * (and reported as success) so lifecycle signals can safely re-assert it.
     * The parameters are remembered so [ensureRegistered] and
     * [reregisterService] can replay them.
     *
     * @return true if a registration is live or awaiting its callback after the
     *   call; false if no registration could be started (missing NSD support, a
     *   throwing platform call, or an immediate failure callback).
     */
    fun registerService(
        port: Int,
        deviceId: String,
        deviceName: String = Build.MODEL,
        capabilities: String = "screen,files,clipboard",
        state: String = "ready"
    ): Boolean {
        val params = RegistrationParams(port, deviceId, deviceName, capabilities, state)
        if (registrar == null) {
            Log.e(TAG, "NsdManager unavailable on this device")
            return false
        }
        synchronized(lock) {
            lastParams = params
            if (isRegistered || isRegistrationPending) {
                Log.w(TAG, "mDNS registration already ${if (isRegistered) "active" else "pending"}; ignoring duplicate request")
                return true
            }
            return registerLocked(params)
        }
    }

    /**
     * Re-registers with the remembered parameters only if no registration is
     * live or pending. Safe to call on every lifecycle signal.
     */
    fun ensureRegistered(): Boolean {
        if (registrar == null) return false
        synchronized(lock) {
            if (isRegistered || isRegistrationPending) return true
            val params = lastParams ?: return false
            return registerLocked(params)
        }
    }

    /**
     * Forces exactly one clean registration with the remembered parameters:
     * best-effort unregister of the current listener, then a fresh registration.
     *
     * This is the recovery primitive for an advertisement the platform has torn
     * down while the app still believes it is registered — Android exposes no
     * "registration lost" callback and no liveness query, so a caller that has a
     * reason to doubt the advertisement (network transition, explicit start)
     * must re-register to repair it.
     */
    fun reregisterService(): Boolean {
        if (registrar == null) return false
        synchronized(lock) {
            val params = lastParams ?: return false
            tearDownLocked()
            return registerLocked(params)
        }
    }

    /**
     * Unregisters the mDNS advertisement and forgets the registration params.
     */
    fun unregisterService() {
        synchronized(lock) {
            if (registrationListener == null && !isRegistrationPending) return
            tearDownLocked()
            lastParams = null
        }
        Log.i(TAG, "Initiated mDNS unregistration")
    }

    private fun registerLocked(params: RegistrationParams): Boolean {
        val reg = registrar ?: return false
        val serviceInfo = NsdServiceInfo().apply {
            serviceName = "PhoneBridge-${params.deviceId}"
            serviceType = SERVICE_TYPE
            setPort(params.port)
            setAttribute("id", params.deviceId)
            setAttribute("name", params.deviceName)
            setAttribute("model", Build.MODEL)
            setAttribute("v", "1")
            setAttribute("caps", params.capabilities)
            setAttribute("state", params.state)
        }

        val gen = ++generation
        val listener = object : NsdManager.RegistrationListener {
            override fun onServiceRegistered(service: NsdServiceInfo) {
                synchronized(lock) {
                    if (gen != generation) {
                        Log.i(TAG, "Ignoring late onServiceRegistered from a retired registration (gen=$gen)")
                        return
                    }
                    isRegistered = true
                    isRegistrationPending = false
                    registeredServiceName = service.serviceName
                }
                Log.i(TAG, "mDNS service registered successfully: ${service.serviceName} on port ${params.port}")
            }

            override fun onRegistrationFailed(service: NsdServiceInfo, errorCode: Int) {
                synchronized(lock) {
                    if (gen != generation) return
                    isRegistered = false
                    isRegistrationPending = false
                    registeredServiceName = null
                }
                Log.e(TAG, "mDNS service registration failed with errorCode: $errorCode")
                onRegistrationFailed?.invoke(errorCode)
            }

            override fun onServiceUnregistered(service: NsdServiceInfo) {
                synchronized(lock) {
                    if (gen != generation) {
                        // A retired listener's unregistration must not clear the
                        // state of the registration that replaced it.
                        Log.i(TAG, "Ignoring late onServiceUnregistered from a retired registration (gen=$gen)")
                        return
                    }
                    isRegistered = false
                    registeredServiceName = null
                }
                Log.i(TAG, "mDNS service unregistered: ${service.serviceName}")
            }

            override fun onUnregistrationFailed(service: NsdServiceInfo, errorCode: Int) {
                Log.e(TAG, "mDNS service unregistration failed with errorCode: $errorCode")
            }
        }

        registrationListener = listener
        isRegistrationPending = true
        return try {
            reg.register(serviceInfo, NsdManager.PROTOCOL_DNS_SD, listener)
            Log.i(
                TAG,
                "Initiated mDNS registration for $SERVICE_TYPE (PhoneBridge-${params.deviceId}, port ${params.port})"
            )
            // A synchronously reported failure clears both flags, so report the
            // truth: no registration is live or pending in that case.
            isRegistrationPending || isRegistered
        } catch (t: Throwable) {
            registrationListener = null
            isRegistrationPending = false
            Log.e(TAG, "Exception during nsdManager.registerService: ${t.message}", t)
            false
        }
    }

    /**
     * Retires the current listener and clears state. Callers must hold [lock].
     * The generation bump happens *before* the platform is told to unregister so
     * that any callback still in flight for the retired listener is ignored.
     */
    private fun tearDownLocked() {
        val listener = registrationListener
        registrationListener = null
        generation++
        isRegistered = false
        isRegistrationPending = false
        registeredServiceName = null
        if (listener != null) {
            try {
                registrar?.unregister(listener)
            } catch (t: Throwable) {
                Log.w(TAG, "Failed to unregister mDNS service: ${t.message}")
            }
        }
    }
}
