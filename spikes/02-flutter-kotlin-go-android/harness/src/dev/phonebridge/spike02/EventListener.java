package dev.phonebridge.spike02;

/**
 * EventListener receives asynchronous events pushed from the Go core.
 */
public interface EventListener {
    /**
     * Invoked when an asynchronous event is emitted by Go.
     *
     * @param seq Monotonic event sequence number.
     * @param kind Event category or type descriptor.
     * @param payload Raw event payload bytes.
     */
    void onEvent(long seq, String kind, byte[] payload);
}
