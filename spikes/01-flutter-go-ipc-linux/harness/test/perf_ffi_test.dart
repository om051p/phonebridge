// Spike 01 — FFI performance + hazard measurement (EXPERIMENTAL, throwaway).
//
// Run alone: `flutter test test/perf_ffi_test.dart`.
//
// The three measurements that matter for the decision:
//   1. direct in-process call cost (the FFI branch's selling point),
//   2. the cost of *avoiding* the UI freeze (an isolate hop per call),
//   3. what "Go -> Flutter push" costs when there is no transport (polling).

import 'dart:async';
import 'dart:ffi';
import 'dart:isolate';

import 'package:ffi/ffi.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:phonebridge_spike01_harness/ffi_client.dart';
import 'package:phonebridge_spike01_harness/paths.dart';
import 'package:phonebridge_spike01_harness/stats.dart';

const int _warmup = 200;
const int _iterations = 2000;
const List<int> _payloadSizes = [0, 64, 1024, 16384, 65536];

void main() {
  late FfiSpikeClient ffi;
  final report = SpikeReport('ffi');

  setUpAll(() {
    ffi = FfiSpikeClient.open(SpikePaths.requireFfiLibrary().path);
  });

  tearDownAll(() async {
    await report.write();
  });

  test('library load (dlopen + symbol resolution)', () {
    report.record('lib_load_us', ffi.loadMicros);
    report.note('  dlopen + symbol resolution: ${ffi.loadMicros} us');
    expect(ffi.loadMicros, lessThan(500000));
  });

  test('direct in-process Ping call latency', () {
    for (var i = 0; i < _warmup; i++) {
      ffi.ping(i);
    }
    final samples = <int>[];
    final total = Stopwatch()..start();
    for (var i = 0; i < _iterations; i++) {
      final sw = Stopwatch()..start();
      final out = ffi.ping(i);
      sw.stop();
      if (out != i) fail('nonce mismatch at $i');
      samples.add(sw.elapsedMicroseconds);
    }
    total.stop();
    report.latency('ping_latency', LatencyStats.fromMicros(samples),
        context: {
          'iterations': _iterations,
          'warmup': _warmup,
          'sequential_rps': (_iterations / (total.elapsedMicroseconds / 1e6))
              .toStringAsFixed(0),
        });
  }, timeout: const Timeout(Duration(minutes: 3)));

  test('payload echo sweep (binary-safe, caller-owned buffers)', () {
    final sweep = <String, Object?>{};
    for (final size in _payloadSizes) {
      final src = calloc<Uint8>(size == 0 ? 1 : size);
      final dst = calloc<Uint8>(size == 0 ? 1 : size);
      try {
        if (size > 0) {
          for (var i = 0; i < size; i++) {
            src[i] = i % 251;
          }
        }
        final samples = <int>[];
        for (var i = 0; i < 300; i++) {
          final sw = Stopwatch()..start();
          final n = ffi.echoRaw(src, size, dst, size);
          sw.stop();
          if (n != size) fail('echo length mismatch at $size');
          samples.add(sw.elapsedMicroseconds);
        }
        final stats = LatencyStats.fromMicros(samples);
        sweep['$size'] = stats.toJson();
        report.note('  echo ${size}B: ${stats.summary}');
      } finally {
        calloc.free(src);
        calloc.free(dst);
      }
    }
    report.section('echo_sweep', {
      'iterations_per_size': 300,
      'sizes': sweep,
      'note': 'the core does no serialisation; cost is a memcpy + cgo boundary',
    });
  }, timeout: const Timeout(Duration(minutes: 3)));

  test('isolate hop cost (the price of not freezing the UI)', () async {
    final libPath = SpikePaths.requireFfiLibrary().path;
    const iterations = 200;
    final samples = await _isolateHopProbe(libPath, iterations);
    report.latency('isolate_hop_latency', LatencyStats.fromMicros(samples),
        context: {'iterations': iterations});
  }, timeout: const Timeout(Duration(minutes: 5)));

  test('a blocking FFI call stalls the calling isolate (UI-freeze hazard)',
      () async {
    const blockMicros = 50000; // 50 ms
    final sw = Stopwatch()..start();
    Duration? timerFiredAt;
    final fired = Completer<void>();
    Timer(const Duration(milliseconds: 10), () {
      timerFiredAt = sw.elapsed;
      fired.complete();
    });
    ffi.blockMicros(blockMicros);
    await fired.future;
    sw.stop();
    report.record('ui_thread_stall', {
      'block_us_requested': blockMicros,
      'timer_requested_ms': 10,
      'timer_fired_at_ms': timerFiredAt?.inMilliseconds,
      'observed_stall_ms': (timerFiredAt ?? Duration.zero).inMilliseconds - 10,
      'note': 'every synchronous FFI call from the UI isolate blocks frame '
          'production for its whole duration',
    });
    report.note('  a 10 ms timer fired at ${timerFiredAt?.inMilliseconds} ms '
        'while a 50 ms FFI call ran on the same isolate');
    expect(timerFiredAt!.inMilliseconds, greaterThanOrEqualTo(45),
        reason: 'a synchronous call must be assumed to block the UI thread');
  }, timeout: const Timeout(Duration(minutes: 1)));

  test('Go -> Flutter push over FFI is poll-based and coalesces events',
      () async {
    ffi.startTicker(2000); // the "core" produces an event every 2 ms
    const pollPeriodMs = 50;
    final sw = Stopwatch()..start();
    final observed = <int>[ffi.pollEvent()];
    Duration? firstChangeAt;
    while (sw.elapsedMilliseconds < 400) {
      await Future<void>.delayed(const Duration(milliseconds: pollPeriodMs));
      final seq = ffi.pollEvent();
      if (seq != observed.last) firstChangeAt ??= sw.elapsed;
      observed.add(seq);
    }
    sw.stop();
    final advanced = observed.last - observed.first;
    final polls = observed.length - 1;
    final visibleChanges = observed.where((s) => s != observed.first).toSet().length;

    report.record('poll_push', {
      'producer_interval_us': 2000,
      'poll_period_ms': pollPeriodMs,
      'observed_polls': polls,
      'producer_events_during_window': advanced,
      'distinct_poll_values': visibleChanges,
      'first_change_latency_ms': firstChangeAt?.inMilliseconds,
      'events_invisible_to_ui': advanced - visibleChanges,
      'note': 'a poller slower than the producer cannot see intermediate '
          'events: PBSpikePollEvent returns only the newest counter',
    });
    report.note('  $advanced Go-side events occurred; the UI observed '
        '$visibleChanges distinct values over $polls polls '
        '(${advanced - visibleChanges} invisible without a queue + callback)');

    expect(advanced, greaterThan(10));
    expect(visibleChanges, lessThan(advanced),
        reason: 'polling must be shown to coalesce/lose events');
  }, timeout: const Timeout(Duration(minutes: 1)));
}

/// Warmup + measured `Isolate.run` round trips, in a top-level function so the
/// closures sent to `Isolate.run` do not close over main()'s context — which
/// holds the main isolate's `DynamicLibrary`, an unsendable object.
Future<List<int>> _isolateHopProbe(String libPath, int iterations) async {
  final samples = <int>[];
  for (var i = 0; i < 10; i++) {
    await Isolate.run(() => FfiSpikeClient.open(libPath).ping(i));
  }
  for (var i = 0; i < iterations; i++) {
    final sw = Stopwatch()..start();
    // Each call spawns an isolate, dlopens the library inside it, calls, and
    // tears down. A real design would keep a long-lived worker isolate, which
    // still pays a port round trip per call but avoids the spawn.
    final out = await Isolate.run(() => FfiSpikeClient.open(libPath).ping(i));
    sw.stop();
    if (out != i) fail('isolate nonce mismatch at $i');
    samples.add(sw.elapsedMicroseconds);
  }
  return samples;
}