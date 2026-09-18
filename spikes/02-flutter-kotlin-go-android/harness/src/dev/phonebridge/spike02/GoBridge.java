package dev.phonebridge.spike02;

import java.io.File;

/**
 * GoBridge wraps the native JNI boundary to the embedded Go core.
 */
public class GoBridge {
    private static volatile boolean loaded = false;

    public static synchronized void load(String libraryPath) {
        if (!loaded) {
            if (libraryPath != null && new File(libraryPath).exists()) {
                System.load(new File(libraryPath).getAbsolutePath());
            } else {
                System.loadLibrary("phonebridge_spike02");
            }
            loaded = true;
        }
    }

    public static boolean isLoaded() {
        return loaded;
    }

    // Native JNI methods implemented in Go
    public static native boolean nativeStart(String storageDir);
    public static native byte[] nativeInvoke(String method, byte[] payload);
    public static native boolean nativeSubscribe(EventListener listener, int count, int intervalMs);
    public static native void nativeTrimMemory(int level);
    public static native boolean nativeShutdown();
    public static native String nativeGetStats();

    // High-level API
    public static void start(String storageDir) {
        if (!nativeStart(storageDir)) {
            throw new IllegalStateException("Failed to start Go engine");
        }
    }

    public static byte[] invoke(String method, byte[] payload) {
        return nativeInvoke(method, payload);
    }

    public static void subscribe(EventListener listener, int count, int intervalMs) {
        if (!nativeSubscribe(listener, count, intervalMs)) {
            throw new IllegalStateException("Failed to subscribe to Go events");
        }
    }

    public static void trimMemory(int level) {
        nativeTrimMemory(level);
    }

    public static void shutdown() {
        nativeShutdown();
    }

    public static String getStats() {
        return nativeGetStats();
    }
}
