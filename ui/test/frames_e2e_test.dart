// Phase 6 Slice 3A — real-device E2E (opt-in).
//
//	POCO F5 → MediaProjection → H.264 → Pion/Go receiver → PSI guard →
//	FrameTap → ffmpeg MJPEG q3 → StreamFrames (UDS/gRPC) → Flutter
//	FrameStream (reassembly + decode-on-arrival) → dart:ui paint
//
// Run (only with a real phone ready and a daemon built from this tree):
//
//	PHONEBRIDGE_E2E=1 flutter test test/frames_e2e_test.dart
//
// Measures, through the PRODUCTION path end to end:
//   * send→paint latency (daemon frame timestamp → painted), p50/p95/max
//   * decode failures / coalesced frames / daemon-side frame-id gaps
//   * the <=64 KiB chunk contract (oversize violations must be 0)
//   * control-event losslessness while frames flow (StreamEvents seq)
//   * bounded RSS across the streaming window
//   * no frame after session termination (stale-frame gate)
//
// Skipped entirely unless PHONEBRIDGE_E2E=1, so CI and normal runs are
// untouched.

// E2E harness: stdout IS the artifact (measurements are the point).
// ignore_for_file: avoid_print

import 'dart:io';

import 'package:flutter_test/flutter_test.dart';

import 'package:phonebridge_ui/generated/phonebridge/localipc/v1/local_ipc.pb.dart'
    as ipc;
import 'package:phonebridge_ui/services/frame_stream.dart';
import 'package:phonebridge_ui/services/local_ipc_client.dart';
import 'package:phonebridge_ui/services/session_backend.dart';

int _percentile(List<int> sorted, double p) {
  if (sorted.isEmpty) return 0;
  final idx = ((sorted.length - 1) * p).round().clamp(0, sorted.length - 1);
  return sorted[idx];
}

int _rssKiB() {
  final status = File('/proc/self/status').readAsStringSync();
  final match =
      RegExp(r'^VmRSS:\s+(\d+) kB', multiLine: true).firstMatch(status);
  return match == null ? 0 : int.parse(match.group(1)!);
}

void main() {
  final enabled = Platform.environment['PHONEBRIDGE_E2E'] == '1';

  test('real POCO → daemon → Flutter paint E2E', () async {
    if (!enabled) {
      return; // opt-in only: no phone/daemon assumptions in normal runs
    }

    final client = LocalIpcClient();
    final events = <ipc.StreamEventsResponse>[];
    final eventSub = client.streamEvents().listen(events.add);

    // -- Handshake + discovery ------------------------------------------
    final hs = await client.handshake();
    print('E2E handshake: server=${hs.serverVersion} '
        'generation=${hs.daemonGeneration}');

    List<ipc.DiscoveredDevice> devices = const [];
    final discoverDeadline = DateTime.now().add(const Duration(seconds: 45));
    while (DateTime.now().isBefore(discoverDeadline)) {
      devices = (await client.listDevices()).devices;
      if (devices.isNotEmpty) break;
      await Future<void>.delayed(const Duration(seconds: 2));
    }
    if (devices.isEmpty) {
      fail('discovery found no devices after 45s');
    }
    final target = devices.firstWhere(
      (d) => d.name.toUpperCase().contains('POCO'),
      orElse: () => devices.first,
    );
    print('E2E target: ${target.name} (${target.id})');

    // -- Start session (720p30 q3 target tuple) -------------------------
    // A leftover session from an earlier run (STREAMING when we arrive) is
    // stopped first: the daemon enforces one active session.
    final pre = await client.getSessionState();
    if (pre.sessionId.isNotEmpty &&
        pre.state == ipc.SessionState.SESSION_STATE_STREAMING) {
      print('E2E: stopping leftover session ${pre.sessionId}');
      await client.stopSession(
          sessionId: pre.sessionId, reason: 'e2e: clearing stale session');
      await Future<void>.delayed(const Duration(seconds: 3));
    }

    final started = await client.startSession(
      target.id,
      requested: mediaParamsRequest(
          width: 720, height: 1600, fps: 30, bitrateKbps: 2500),
    );
    final sessionId = started.sessionId;
    print('E2E session: $sessionId -> ${started.state}');

    // Poll the pushed ladder without busy-spinning.
    Future<ipc.SessionState> untilState(
        ipc.SessionState want, Duration t) async {
      final deadline = DateTime.now().add(t);
      ipc.SessionState? last;
      while (DateTime.now().isBefore(deadline)) {
        for (final e in events) {
          if (e.hasSessionEvent() && e.sessionEvent.sessionId == sessionId) {
            last = e.sessionEvent.state;
            if (e.sessionEvent.state == want) return want;
            if (e.sessionEvent.state == ipc.SessionState.SESSION_STATE_FAILED) {
              fail('session failed: ${e.sessionEvent.reason}');
            }
          }
        }
        await Future<void>.delayed(const Duration(milliseconds: 250));
      }
      fail('timed out waiting for $want (last=$last)');
    }

    await untilState(
        ipc.SessionState.SESSION_STATE_STREAMING, const Duration(seconds: 90));
    print('E2E: STREAMING (device consent + negotiation OK)');

    // -- Subscribe to frames: the dedicated second stream ---------------
    final latencies = <int>[];
    var jpegBytesTotal = 0;
    var paintsBeforeStop = 0;
    String? dims;
    final rssStart = _rssKiB();
    final frames = FrameStream(
      source: () => client.streamFrames(),
      onFramePainted: (info) {
        latencies.add(info.latencyMicros);
        jpegBytesTotal += info.jpegBytes;
        dims ??= '${info.width}x${info.height}';
      },
    );
    frames.start();

    // Let the pipeline run: measure a 20 s window.
    await Future<void>.delayed(const Duration(seconds: 20));
    final s = frames.stats;
    print('E2E frames (20s window): painted=${s.framesPainted} '
        'complete=${s.framesComplete} coalesced=${s.framesCoalesced} '
        'gaps=${s.frameIdGaps} decodeFailures=${s.decodeFailures} '
        'oversizeChunks=${s.oversizeChunks} incompleteDropped='
        '${s.framesIncompleteDropped} streamErrors=${s.streamErrors}');
    final rssMid = _rssKiB();
    final avgJpegFirst =
        s.framesPainted == 0 ? 0 : jpegBytesTotal ~/ s.framesPainted;
    print('E2E geometry/size: ${dims ?? "unknown"} avgJpeg=${avgJpegFirst}B '
        'rssStart=${rssStart}KiB rssMid=${rssMid}KiB');

    // -- Gates -----------------------------------------------------------
    expect(s.framesPainted, greaterThan(0), reason: 'no frame was painted');
    expect(s.decodeFailures, 0, reason: 'decode failure on real frames');
    expect(s.oversizeChunks, 0, reason: 'a message exceeded the 64 KiB rule');
    expect(s.framesComplete, greaterThan(0));
    expect(latencies, isNotEmpty);

    final sorted = List<int>.from(latencies)..sort();
    final avgBytes = jpegBytesTotal ~/ latencies.length;
    print('E2E latency (send→paint): n=${sorted.length} '
        'p50=${_percentile(sorted, 0.50)}µs '
        'p95=${_percentile(sorted, 0.95)}µs '
        'max=${sorted.last}µs avgJpeg=${avgBytes}B');
    expect(_percentile(sorted, 0.95), lessThanOrEqualTo(60 * 1000),
        reason: 'p95 send→paint exceeds the 60 ms gate');

    // Control events must have stayed gapless while frames flowed.
    final seqs = events.map((e) => e.seq).toList();
    for (var i = 1; i < seqs.length; i++) {
      expect(seqs[i], seqs[i - 1] + 1,
          reason: 'StreamEvents seq gap at #$i: ${seqs[i - 1]} -> ${seqs[i]}');
    }
    print('E2E control events: ${seqs.length} received, seq gapless');

    // -- Stop: no stale frame, control stream survives -------------------
    await client.stopSession(sessionId: sessionId, reason: 'e2e done');
    await untilState(
        ipc.SessionState.SESSION_STATE_STOPPED, const Duration(seconds: 20));
    // The hub closes subscriptions at the terminal transition; drain whatever
    // was already on the wire, then freeze the counter.
    await Future<void>.delayed(const Duration(seconds: 1));
    paintsBeforeStop = frames.stats.framesPainted;
    await Future<void>.delayed(const Duration(seconds: 3));

    final after = frames.stats;
    expect(after.framesPainted, paintsBeforeStop,
        reason: 'a frame was painted after session termination');
    // The event stream is still alive on the same client.
    final pong = await client.ping(nonce: 7);
    expect(pong.nonce.toInt(), equals(7));
    print('E2E post-stop: painted=${after.framesPainted} (frozen), '
        'event stream alive, rssStart=${rssStart}KiB rssEnd=${_rssKiB()}KiB');

    frames.dispose();
    await eventSub.cancel();
    await client.shutdown();
  }, timeout: const Timeout(Duration(minutes: 4)));
}
