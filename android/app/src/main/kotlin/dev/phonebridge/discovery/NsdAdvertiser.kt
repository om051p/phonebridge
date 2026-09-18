package dev.phonebridge.discovery

import android.content.Context
import android.net.nsd.NsdManager
import android.net.nsd.NsdServiceInfo
import android.os.Build
import android.util.Log

/**
 * NsdAdvertiser advertises the PhoneBridge device presence over local network
 * via Android Network Service Discovery (mDNS / DNS-SD).
 *
 * Conforms to DEC-007 and DEC-018:
 * - Service type: _phonebridge._tcp
 * - Domain: local.
 * - TXT records: id, name, model, v, caps, state
 */
class NsdAdvertiser(context: Context) {

    companion object {
        private const val TAG = "NsdAdvertiser"
        const val SERVICE_TYPE = "_phonebridge._tcp"
    }

    private val nsdManager = context.getSystemService(Context.NSD_SERVICE) as? NsdManager
    private var registrationListener: NsdManager.RegistrationListener? = null

    @Volatile
    var isRegistered = false
        private set

    @Volatile
    var registeredServiceName: String? = null
        private set

    /**
     * Registers the mDNS advertisement for this device.
     */
    fun registerService(
        port: Int,
        deviceId: String,
        deviceName: String = Build.MODEL,
        capabilities: String = "screen,files,clipboard",
        state: String = "ready"
    ) {
        if (nsdManager == null) {
            Log.e(TAG, "NsdManager unavailable on this device")
            return
        }
        if (isRegistered) {
            Log.w(TAG, "NsdAdvertiser service is already registered")
            return
        }

        val serviceInfo = NsdServiceInfo().apply {
            serviceName = "PhoneBridge-$deviceId"
            serviceType = SERVICE_TYPE
            setPort(port)
            setAttribute("id", deviceId)
            setAttribute("name", deviceName)
            setAttribute("model", Build.MODEL)
            setAttribute("v", "1")
            setAttribute("caps", capabilities)
            setAttribute("state", state)
        }

        val listener = object : NsdManager.RegistrationListener {
            override fun onServiceRegistered(service: NsdServiceInfo) {
                isRegistered = true
                registeredServiceName = service.serviceName
                Log.i(TAG, "mDNS service registered successfully: ${service.serviceName} on port $port")
            }

            override fun onRegistrationFailed(service: NsdServiceInfo, errorCode: Int) {
                isRegistered = false
                registeredServiceName = null
                Log.e(TAG, "mDNS service registration failed with errorCode: $errorCode")
            }

            override fun onServiceUnregistered(service: NsdServiceInfo) {
                isRegistered = false
                registeredServiceName = null
                Log.i(TAG, "mDNS service unregistered: ${service.serviceName}")
            }

            override fun onUnregistrationFailed(service: NsdServiceInfo, errorCode: Int) {
                Log.e(TAG, "mDNS service unregistration failed with errorCode: $errorCode")
            }
        }
        registrationListener = listener

        try {
            nsdManager.registerService(serviceInfo, NsdManager.PROTOCOL_DNS_SD, listener)
            Log.i(TAG, "Initiated mDNS registration for _phonebridge._tcp (PhoneBridge-$deviceId, port $port)")
        } catch (t: Throwable) {
            Log.e(TAG, "Exception during nsdManager.registerService: ${t.message}", t)
        }
    }

    /**
     * Unregisters the mDNS advertisement.
     */
    fun unregisterService() {
        val listener = registrationListener ?: return
        try {
            nsdManager?.unregisterService(listener)
            Log.i(TAG, "Initiated mDNS unregistration")
        } catch (t: Throwable) {
            Log.w(TAG, "Failed to unregister mDNS service: ${t.message}")
        } finally {
            registrationListener = null
            isRegistered = false
            registeredServiceName = null
        }
    }
}
