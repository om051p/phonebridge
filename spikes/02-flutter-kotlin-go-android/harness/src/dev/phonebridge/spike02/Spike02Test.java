package dev.phonebridge.spike02;

import java.io.File;
import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.Collections;
import java.util.List;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.TimeUnit;

/**
 * Automated test suite proving the 6 required capabilities of the Android ↔ Go boundary:
 * 1. Start Go component from Android.
 * 2. Synchronous request/response.
 * 3. Asynchronous event delivery.
 * 4. Error handling & safe panic recovery.
 * 5. Clean shutdown.
 * 6. Android lifecycle state machine compliance.
 */
public class Spike02Test {

    private static int testsPassed = 0;
    private static int testsTotal = 0;

    public static void main(String[] args) {
        String libPath = args.length > 0 ? args[0] : "build/libphonebridge_spike02.so";
        System.out.println("=== Spike 02: Android (Kotlin/Java) ↔ Go Boundary Verification ===");
        System.out.println("Loading native library: " + libPath);

        try {
            GoBridge.load(libPath);
            System.out.println("Native library successfully loaded into JVM.");
        } catch (Throwable t) {
            System.err.println("FAILED to load native library: " + t.getMessage());
            t.printStackTrace();
            System.exit(1);
        }

        runTest("1. Start Go component from Android", Spike02Test::testStartGoComponent);
        runTest("2. Synchronous request/response", Spike02Test::testRequestResponse);
        runTest("3. Asynchronous event delivery", Spike02Test::testAsynchronousEvents);
        runTest("4. Error handling and safe panic recovery", Spike02Test::testErrorHandlingAndPanicRecovery);
        runTest("5. Clean shutdown & resource release", Spike02Test::testCleanShutdown);
        runTest("6. Android lifecycle state compliance", Spike02Test::testAndroidLifecycleCompliance);

        System.out.println("------------------------------------------------------------------");
        System.out.printf("Test Results: %d/%d passed (%.1f%%)\n", testsPassed, testsTotal, (testsPassed * 100.0) / testsTotal);

        if (testsPassed != testsTotal) {
            System.err.println("Spike 02 verification FAILED.");
            System.exit(1);
        }
        System.out.println("Spike 02 verification SUCCESSFUL: All 6 requirements proven.");
    }

    private static void runTest(String name, Runnable test) {
        testsTotal++;
        System.out.print("[TEST " + testsTotal + "] " + name + "... ");
        try {
            test.run();
            testsPassed++;
            System.out.println("PASSED");
        } catch (Throwable t) {
            System.out.println("FAILED");
            System.err.println("  Error: " + t.getMessage());
            t.printStackTrace(System.err);
        }
    }

    private static void assertTrue(boolean condition, String message) {
        if (!condition) {
            throw new AssertionError("Assertion failed: " + message);
        }
    }

    private static void assertEquals(Object expected, Object actual, String message) {
        if (expected == null && actual == null) return;
        if (expected != null && expected.equals(actual)) return;
        throw new AssertionError(String.format("Assertion failed: %s (expected: %s, got: %s)", message, expected, actual));
    }

    // 1. Start Go component
    private static void testStartGoComponent() {
        GoBridge.start("/data/user/0/dev.phonebridge/files");
        String stats = GoBridge.getStats();
        assertTrue(stats.contains("state=1"), "Engine state should be RUNNING (1)");
        assertTrue(stats.contains("goroutines="), "Stats should report Go goroutines");
    }

    // 2. Synchronous request/response
    private static void testRequestResponse() {
        // Test ping
        byte[] payload = "hello-from-android".getBytes(StandardCharsets.UTF_8);
        byte[] response = GoBridge.invoke("ping", payload);
        String respStr = new String(response, StandardCharsets.UTF_8);
        assertEquals("pong:hello-from-android", respStr, "Ping response mismatch");

        // Test binary echo
        byte[] binaryData = new byte[]{0x01, 0x02, (byte) 0xFF, 0x00, 0x7F};
        byte[] echoResp = GoBridge.invoke("echo", binaryData);
        assertEquals(binaryData.length, echoResp.length, "Echo response length mismatch");
        for (int i = 0; i < binaryData.length; i++) {
            assertEquals(binaryData[i], echoResp[i], "Byte mismatch at index " + i);
        }
    }

    // 3. Asynchronous event delivery
    private static void testAsynchronousEvents() {
        int eventCount = 5;
        CountDownLatch latch = new CountDownLatch(eventCount);
        List<Long> receivedSeqs = Collections.synchronizedList(new ArrayList<>());
        List<String> receivedKinds = Collections.synchronizedList(new ArrayList<>());

        EventListener listener = (seq, kind, data) -> {
            receivedSeqs.add(seq);
            receivedKinds.add(kind);
            latch.countDown();
        };

        GoBridge.subscribe(listener, eventCount, 10);

        try {
            boolean completed = latch.await(2, TimeUnit.SECONDS);
            assertTrue(completed, "Timed out waiting for asynchronous events from Go");
            assertEquals(eventCount, receivedSeqs.size(), "Should receive all 5 events");
            for (int i = 0; i < eventCount; i++) {
                assertEquals((long)(i + 1), receivedSeqs.get(i), "Sequence ordering mismatch");
                assertEquals("tick", receivedKinds.get(i), "Event kind mismatch");
            }
        } catch (InterruptedException e) {
            throw new RuntimeException("Interrupted", e);
        }
    }

    // 4. Error handling and panic recovery
    private static void testErrorHandlingAndPanicRecovery() {
        // Test unknown method error
        boolean caughtUnknown = false;
        try {
            GoBridge.invoke("unknown_nonexistent_method", new byte[0]);
        } catch (IllegalStateException e) {
            caughtUnknown = true;
            assertTrue(e.getMessage().contains("unknown method"), "Expected unknown method message");
        }
        assertTrue(caughtUnknown, "Go error should propagate to Java as IllegalStateException");

        // Test application error
        boolean caughtAppError = false;
        try {
            GoBridge.invoke("error", new byte[0]);
        } catch (IllegalStateException e) {
            caughtAppError = true;
            assertTrue(e.getMessage().contains("simulated application error"), "Expected app error message");
        }
        assertTrue(caughtAppError, "Explicit Go error should be caught cleanly");

        // CRITICAL: Test Go panic recovery — Must NOT crash the JVM!
        boolean caughtPanic = false;
        try {
            GoBridge.invoke("panic", new byte[0]);
        } catch (IllegalStateException e) {
            caughtPanic = true;
            assertTrue(e.getMessage().contains("Go panic recovered"), "Panic should be safely recovered and wrapped");
        }
        assertTrue(caughtPanic, "Go runtime panic must be recovered without aborting JVM");

        // Prove the Go engine and JVM remain functional after a panic!
        byte[] postPanicPing = GoBridge.invoke("ping", "post-panic".getBytes(StandardCharsets.UTF_8));
        assertEquals("pong:post-panic", new String(postPanicPing, StandardCharsets.UTF_8), "Engine must remain functional after panic");
    }

    // 5. Clean shutdown
    private static void testCleanShutdown() {
        GoBridge.shutdown();

        // Verify subsequent invoke fails because engine is stopped
        boolean caughtStopped = false;
        try {
            GoBridge.invoke("ping", "test".getBytes(StandardCharsets.UTF_8));
        } catch (IllegalStateException e) {
            caughtStopped = true;
            assertTrue(e.getMessage().contains("not running"), "Should report engine not running");
        }
        assertTrue(caughtStopped, "Calls after shutdown must be rejected");
    }

    // 6. Android lifecycle state compliance
    private static void testAndroidLifecycleCompliance() {
        AndroidLifecycleSimulator service = new AndroidLifecycleSimulator("/data/user/0/dev.phonebridge/cache");
        assertEquals(AndroidLifecycleSimulator.ServiceState.UNCREATED, service.getState(), "Initial state");

        // 1. Service.onCreate() -> Starts Go
        service.onCreate();
        assertEquals(AndroidLifecycleSimulator.ServiceState.CREATED, service.getState(), "State after onCreate");

        // 2. Service.onStartCommand() -> Active
        service.onStartCommand();
        assertEquals(AndroidLifecycleSimulator.ServiceState.RUNNING, service.getState(), "State after onStartCommand");

        // Functional check while running
        byte[] resp = GoBridge.invoke("ping", "lifecycle-test".getBytes(StandardCharsets.UTF_8));
        assertEquals("pong:lifecycle-test", new String(resp, StandardCharsets.UTF_8), "Ping during running state");

        // 3. ComponentCallbacks2.onTrimMemory() -> Low memory event
        service.onTrimMemory(AndroidLifecycleSimulator.TRIM_MEMORY_RUNNING_CRITICAL);
        assertEquals(AndroidLifecycleSimulator.ServiceState.TRIMMED, service.getState(), "State after onTrimMemory");
        String stats = GoBridge.getStats();
        assertTrue(stats.contains("state=2"), "Engine should report TRIMMED state");

        // Engine remains responsive during TRIMMED state
        byte[] respTrimmed = GoBridge.invoke("ping", "post-trim".getBytes(StandardCharsets.UTF_8));
        assertEquals("pong:post-trim", new String(respTrimmed, StandardCharsets.UTF_8), "Ping during trimmed state");

        // 4. Service.onDestroy() -> Clean shutdown
        service.onDestroy();
        assertEquals(AndroidLifecycleSimulator.ServiceState.DESTROYED, service.getState(), "State after onDestroy");

        // 5. Restart cycle (simulating Service recreate or ForegroundService relaunch)
        service.onCreate();
        service.onStartCommand();
        byte[] respRecreated = GoBridge.invoke("ping", "after-recreate".getBytes(StandardCharsets.UTF_8));
        assertEquals("pong:after-recreate", new String(respRecreated, StandardCharsets.UTF_8), "Ping after service recreation");

        // Final cleanup
        service.onDestroy();
    }
}
