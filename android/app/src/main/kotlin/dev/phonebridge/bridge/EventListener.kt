package dev.phonebridge.bridge

/**
 * EventListener receives asynchronous events pushed from the Go core.
 */
interface EventListener {
    /**
     * Invoked when an asynchronous event is emitted by the Go runtime.
     *
     * @param seq Monotonic event sequence number.
     * @param kind Event category or type descriptor.
     * @param payload Raw event payload bytes.
     */
    fun onEvent(seq: Long, kind: String, payload: ByteArray)
}
