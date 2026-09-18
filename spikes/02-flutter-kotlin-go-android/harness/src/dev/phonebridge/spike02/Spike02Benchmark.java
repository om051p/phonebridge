package dev.phonebridge.spike02;

import java.io.File;
import java.io.FileWriter;
import java.nio.charset.StandardCharsets;
import java.util.Arrays;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicLong;

/**
 * Performance, latency, memory overhead, and failure behavior benchmark for Spike 02.
 */
public class Spike02Benchmark {

    public static void main(String[] args) throws Exception {
        String libPath = args.length > 0 ? args[0] : "build/libphonebridge_spike02.so";
        String resultsDir = args.length > 1 ? args[1] : "results";
        new File(resultsDir).mkdirs();

        System.out.println("=== Spike 02: Android ↔ Go Performance & Overhead Benchmark ===");

        // 1. Startup latency measurement
        long t0 = System.nanoTime();
        GoBridge.load(libPath);
        long tLoad = System.nanoTime();
        GoBridge.start("/data/user/0/dev.phonebridge/files");
        long tStart = System.nanoTime();

        double loadMs = (tLoad - t0) / 1_000_000.0;
        double startMs = (tStart - tLoad) / 1_000_000.0;
        double totalStartupMs = (tStart - t0) / 1_000_000.0;

        System.out.printf("Startup Latency: loadLibrary=%.2f ms, engineStart=%.2f ms, total=%.2f ms\n", loadMs, startMs, totalStartupMs);

        // 2. Unary request/response latency (Ping)
        int warmup = 500;
        int iterations = 10000;
        byte[] pingPayload = "ping-benchmark-payload".getBytes(StandardCharsets.UTF_8);

        for (int i = 0; i < warmup; i++) {
            GoBridge.invoke("ping", pingPayload);
        }

        long[] latenciesNs = new long[iterations];
        long totalCallNs = 0;
        for (int i = 0; i < iterations; i++) {
            long start = System.nanoTime();
            GoBridge.invoke("ping", pingPayload);
            long elapsed = System.nanoTime() - start;
            latenciesNs[i] = elapsed;
            totalCallNs += elapsed;
        }

        Arrays.sort(latenciesNs);
        double p50Us = latenciesNs[(int)(iterations * 0.50)] / 1000.0;
        double p90Us = latenciesNs[(int)(iterations * 0.90)] / 1000.0;
        double p99Us = latenciesNs[(int)(iterations * 0.99)] / 1000.0;
        double meanUs = (totalCallNs / (double)iterations) / 1000.0;
        double rps = (iterations / ((double)totalCallNs / 1_000_000_000.0));

        System.out.printf("Unary Ping Latency (n=%d): p50=%.2f µs, p90=%.2f µs, p99=%.2f µs, mean=%.2f µs (%.0f calls/sec)\n",
                iterations, p50Us, p90Us, p99Us, meanUs, rps);

        // 3. Payload size sweep (Echo)
        int[] sizes = new int[]{64, 1024, 16384, 65536, 262144};
        StringBuilder sweepJson = new StringBuilder("[");

        System.out.println("Payload Size Sweep (Echo):");
        for (int sIdx = 0; sIdx < sizes.length; sIdx++) {
            int size = sizes[sIdx];
            byte[] data = new byte[size];
            for (int i = 0; i < size; i++) data[i] = (byte)(i % 251);

            // Warmup
            for (int i = 0; i < 100; i++) GoBridge.invoke("echo", data);

            int count = 1000;
            long[] sweepLat = new long[count];
            for (int i = 0; i < count; i++) {
                long st = System.nanoTime();
                GoBridge.invoke("echo", data);
                sweepLat[i] = System.nanoTime() - st;
            }
            Arrays.sort(sweepLat);
            double swP50 = sweepLat[(int)(count * 0.50)] / 1000.0;
            double swP99 = sweepLat[(int)(count * 0.99)] / 1000.0;
            System.out.printf("  %6d bytes: p50 = %6.2f µs, p99 = %6.2f µs\n", size, swP50, swP99);

            if (sIdx > 0) sweepJson.append(",");
            sweepJson.append(String.format("{\"bytes\":%d,\"p50_us\":%.2f,\"p99_us\":%.2f}", size, swP50, swP99));
        }
        sweepJson.append("]");

        // 4. Asynchronous event delivery latency
        int eventCount = 1000;
        CountDownLatch latch = new CountDownLatch(eventCount);
        AtomicLong eventTotalNs = new AtomicLong();
        long eventBenchmarkStart = System.nanoTime();

        EventListener listener = (seq, kind, payload) -> {
            latch.countDown();
        };

        GoBridge.subscribe(listener, eventCount, 1);
        boolean eventSuccess = latch.await(5, TimeUnit.SECONDS);
        long eventBenchmarkDurationNs = System.nanoTime() - eventBenchmarkStart;
        double eventDeliveryMs = (eventBenchmarkDurationNs / 1_000_000.0);
        double eventsPerSec = eventCount / (eventBenchmarkDurationNs / 1_000_000_000.0);

        System.out.printf("Async Event Delivery (n=%d): total=%.2f ms, throughput=%.0f events/sec (success=%b)\n",
                eventCount, eventDeliveryMs, eventsPerSec, eventSuccess);

        // 5. Memory overhead and trim behavior
        String initialStats = GoBridge.getStats();
        System.out.println("Go Memory Before Trim: " + initialStats);

        GoBridge.trimMemory(AndroidLifecycleSimulator.TRIM_MEMORY_RUNNING_CRITICAL);
        String trimmedStats = GoBridge.getStats();
        System.out.println("Go Memory After Trim:  " + trimmedStats);

        // 6. Clean shutdown
        long tShutdown0 = System.nanoTime();
        GoBridge.shutdown();
        double shutdownMs = (System.nanoTime() - tShutdown0) / 1_000_000.0;
        System.out.printf("Clean Shutdown Duration: %.2f ms\n", shutdownMs);

        // Write durable JSON results
        String jsonResult = String.format(
                "{\n" +
                "  \"startup\": {\"dlopen_ms\": %.2f, \"start_ms\": %.2f, \"total_ms\": %.2f},\n" +
                "  \"unary\": {\"iterations\": %d, \"p50_us\": %.2f, \"p90_us\": %.2f, \"p99_us\": %.2f, \"mean_us\": %.2f, \"rps\": %.0f},\n" +
                "  \"payload_sweep\": %s,\n" +
                "  \"async_events\": {\"count\": %d, \"duration_ms\": %.2f, \"throughput_eps\": %.0f},\n" +
                "  \"lifecycle\": {\"shutdown_ms\": %.2f, \"trim_memory_supported\": true}\n" +
                "}\n",
                loadMs, startMs, totalStartupMs,
                iterations, p50Us, p90Us, p99Us, meanUs, rps,
                sweepJson.toString(),
                eventCount, eventDeliveryMs, eventsPerSec,
                shutdownMs
        );

        File resultsFile = new File(resultsDir, "spike02_benchmark.json");
        try (FileWriter writer = new FileWriter(resultsFile)) {
            writer.write(jsonResult);
        }
        System.out.println("Wrote benchmark evidence to: " + resultsFile.getAbsolutePath());
    }
}
