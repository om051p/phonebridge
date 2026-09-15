// Spike 01 — UDS+gRPC performance measurement (EXPERIMENTAL, throwaway).
//
// Run this file on its own (`flutter test test/perf_uds_grpc_test.dart`) so
// other test isolates do not perturb the numbers shown in the results doc.
//
// Method: wall-clock Stopwatch around each awaited RPC, after a warmup that
// absorbs connection setup and JIT. Caveat stated in the results doc: this is a
// debug/JIT Dart VM (flutter tester), so absolute numbers are pessimistic
// compared with an AOT release build; the UDS-vs-FFI ratio is the meaningful
// output.

import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:phonebridge_spike01_harness/daemon_harness.dart';
import 'package:phonebridge_spike01_harness/stats.dart';
import 'package:phonebridge_spike01_harness/uds_client.dart';

const int _warmup = 200;
const int _iterations = 2000;
const List<int> _payloadSizes = [0, 64, 1024, 16384, 65536];

void main() {
  late Directory dir;
  late String socketPath;
  late SpikeDaemon daemon;
  late UdsSpikeClient client;
  final report = SpikeReport('uds_grpc');

  setUpAll(() async {
    dir = SpikeSockets.tempDir();
    socketPath = '${dir.path}/engine.sock';
    daemon = await SpikeDaemon.start(socketPath: socketPath);
    client = UdsSpikeClient(socketPath: socketPath)..connect();
    await client.warmUp();
  });

  tearDownAll(() async {
    await client.close();
    final stats = await daemon.terminateAndCollectStats();
    report.record('daemon_stats_after_run', stats);
    report.record('channel_cold_first_call_us',
        client.firstCallWallTime?.inMicroseconds ?? 0);
    await report.write();
    if (dir.existsSync()) dir.deleteSync(recursive: true);
  });

  test('unary Ping round-trip latency (warm, sequential)', () async {
    for (var i = 0; i < _warmup; i++) {
      await client.ping(nonce: i);
    }
    final samples = <int>[];
    final total = Stopwatch()..start();
    for (var i = 0; i < _iterations; i++) {
      final sw = Stopwatch()..start();
      final res = await client.ping(nonce: i);
      sw.stop();
      if (res.nonce.toInt() != i) fail('nonce mismatch at $i');
      samples.add(sw.elapsedMicroseconds);
    }
    total.stop();
    report.latency(
      'ping_latency',
      LatencyStats.fromMicros(samples),
      context: {
        'iterations': _iterations,
        'warmup': _warmup,
        'payload_bytes': 0,
        'sequential_rps': (_iterations / (total.elapsedMicroseconds / 1e6))
            .toStringAsFixed(0),
      },
    );
    expect(LatencyStats.fromMicros(samples).p50, lessThan(10000),
        reason: 'spike success criterion is < 10 ms p50');
  }, timeout: const Timeout(Duration(minutes: 3)));

  test('payload-size sweep (evidence for future chunk sizing)', () async {
    final sweep = <String, Object?>{};
    for (final size in _payloadSizes) {
      for (var i = 0; i < 50; i++) {
        await client.ping(nonce: i, payloadBytes: size);
      }
      final samples = <int>[];
      for (var i = 0; i < 300; i++) {
        final sw = Stopwatch()..start();
        final res = await client.ping(nonce: i, payloadBytes: size);
        sw.stop();
        if (res.payload.length != size) fail('payload truncation at $size');
        samples.add(sw.elapsedMicroseconds);
      }
      final stats = LatencyStats.fromMicros(samples);
      sweep['$size'] = stats.toJson();
      report.note('  payload ${size}B: ${stats.summary}');
    }
    report.section('payload_sweep', {
      'iterations_per_size': 300,
      'sizes': sweep,
    });
  }, timeout: const Timeout(Duration(minutes: 3)));

  test('throughput with 64 in-flight unary calls', () async {
    const batches = 25;
    const concurrency = 64;
    final total = Stopwatch()..start();
    for (var b = 0; b < batches; b++) {
      await Future.wait(List<Future<dynamic>>.generate(
        concurrency,
        (i) => client.ping(nonce: b * concurrency + i),
      ));
    }
    total.stop();
    const calls = batches * concurrency;
    final rps = calls / (total.elapsedMicroseconds / 1e6);
    report.record('pipelined_throughput', {
      'calls': calls,
      'concurrency': concurrency,
      'elapsed_us': total.elapsedMicroseconds,
      'rps': double.parse(rps.toStringAsFixed(0)),
      'mean_latency_us': double.parse(
          (total.elapsedMicroseconds / calls).toStringAsFixed(1)),
    });
    report.note('  64 in-flight: ${rps.toStringAsFixed(0)} rps');
    expect(rps, greaterThan(1000));
  }, timeout: const Timeout(Duration(minutes: 3)));

  test('Go -> Flutter push latency over a server stream', () async {
    const eventCount = 20;
    const intervalMs = 20;
    final receiveTimes = <int>[];
    final deliveryLatency = <int>[];
    final sw = Stopwatch()..start();
    await for (final ev in client.subscribe(
        count: eventCount, intervalMs: intervalMs, payloadSize: 256)) {
      receiveTimes.add(sw.elapsedMicroseconds);
      // Same host clock on both sides (CLOCK_REALTIME), so daemon emission ->
      // Dart delivery is directly measurable.
      final emittedUs = ev.emittedUnixNanos.toInt() ~/ 1000;
      deliveryLatency.add(DateTime.now().microsecondsSinceEpoch - emittedUs);
    }
    expect(receiveTimes.length, eventCount);

    final interArrival = <int>[];
    for (var i = 1; i < receiveTimes.length; i++) {
      interArrival.add(receiveTimes[i] - receiveTimes[i - 1]);
    }
    report.latency('push_inter_arrival', LatencyStats.fromMicros(interArrival),
        context: {'requested_interval_ms': intervalMs});
    report.latency(
        'push_delivery_latency', LatencyStats.fromMicros(deliveryLatency),
        context: {'events': eventCount, 'payload_bytes': 256});
  }, timeout: const Timeout(Duration(minutes: 3)));
}