// Spike 07 / Phase 6 Slice 3 — P2 client bench.
//
// Measures the Flutter side of the proposed frame path through the REAL
// production client (LocalIpcClient → grpc-dart → UDS → bearer token):
//
//   flood  — transport ceiling: sustained msg/s and MiB/s including Dart
//            protobuf parse (no decode/paint).
//   decode — paced 30 fps real JPEG frames; decode-on-arrival consumer:
//            decode immediately when idle, latest-wins coalesce while busy
//            (no fixed tick); measures JPEG decode, raster (Picture.toImage
//            forces a real raster), end-to-end frame age, and RSS.
//   stall  — same as decode plus one 300 ms isolate stall (simulated UI jank)
//            to prove displayed-frame age stays bounded (drop-backpressure
//            design validation).
//
// Driven by run_p2.sh via P2_PHASE / P2_SOCKET / P2_TOKEN. Prints RESULT lines.

import 'dart:async';
import 'dart:io';
import 'dart:typed_data';
import 'dart:ui' as ui;

import 'package:flutter_test/flutter_test.dart';
import 'package:phonebridge_ui/services/local_ipc_client.dart';

int? _sentMicros(Uint8List payload) {
  if (payload.length < 8) return null;
  return ByteData.sublistView(payload).getUint64(0, Endian.big).toInt();
}

Uint8List _frameBytes(Uint8List payload) => payload.sublist(8);

double _p50(List<double> v) {
  if (v.isEmpty) return 0;
  final s = [...v]..sort();
  return s[s.length ~/ 2];
}

double _p95(List<double> v) {
  if (v.isEmpty) return 0;
  final s = [...v]..sort();
  final i = (0.95 * (s.length - 1)).round();
  return s[i];
}

double _ms(DateTime a, DateTime b) => a.difference(b).inMicroseconds / 1000.0;

class _Decoded {
  _Decoded(this.decodeMs, this.rasterMs, this.paintDone, this.sent);
  final double decodeMs;
  final double rasterMs;
  final DateTime paintDone;
  final DateTime? sent;
}

/// Decodes one JPEG and rasterizes it once. Returns null on failure.
Future<_Decoded?> _decodeAndPaint(Uint8List bytes, DateTime? sent) async {
  final sw = Stopwatch()..start();
  final ui.Codec codec;
  try {
    codec = await ui.instantiateImageCodec(bytes);
  } catch (e) {
    return null;
  }
  final ui.FrameInfo frame;
  try {
    frame = await codec.getNextFrame();
  } finally {
    codec.dispose();
  }
  sw.stop();
  final decodeMs = sw.elapsedMicroseconds / 1000.0;

  final sw2 = Stopwatch()..start();
  final recorder = ui.PictureRecorder();
  final canvas = ui.Canvas(recorder);
  canvas.drawImage(frame.image, ui.Offset.zero, ui.Paint());
  final picture = recorder.endRecording();
  final ui.Image rastered = await picture.toImage(720, 1600);
  sw2.stop();
  picture.dispose();
  rastered.dispose();
  frame.image.dispose();

  return _Decoded(decodeMs, sw2.elapsedMicroseconds / 1000.0,
      DateTime.now(), sent);
}

void main() {
  final phase = Platform.environment['P2_PHASE'] ?? 'flood';
  final socket = Platform.environment['P2_SOCKET'] ?? '';
  final token = Platform.environment['P2_TOKEN'] ?? '';

  test('p2 bench phase=$phase', () async {
    expect(socket, isNotEmpty,
        reason: 'run via run_p2.sh (P2_SOCKET/P2_TOKEN unset)');
    final client = LocalIpcClient(
      socketPath: socket,
      tokenPath: token,
      connectTimeout: const Duration(seconds: 5),
      callTimeout: const Duration(seconds: 60),
    );

    if (phase.startsWith('flood')) {
      int msgs = 0;
      int bytes = 0;
      int payloadBytes = 0;
      final firstMsg = Completer<void>();
      final sub = client.streamEvents().listen((r) {
        if (!r.envelope.hasClipboardUpdate()) return;
        final p = r.envelope.clipboardUpdate.payload;
        msgs++;
        bytes += p.length;
        payloadBytes = p.length;
        if (!firstMsg.isCompleted) firstMsg.complete();
      });
      await firstMsg.future
          .timeout(const Duration(seconds: 90), onTimeout: () {});
      // Warm-up grace: skip whatever accrued while settling.
      await Future<void>.delayed(const Duration(seconds: 1));
      final m0 = msgs, b0 = bytes;
      const window = Duration(seconds: 5);
      final t0 = DateTime.now();
      await Future<void>.delayed(window);
      final t1 = DateTime.now();
      final secs = _ms(t1, t0) / 1000.0;
      await sub.cancel();
      await client.shutdown();
      print('RESULT p2_flood payload_bytes=$payloadBytes '
          'msgs=${msgs - m0} msg_per_s=${((msgs - m0) / secs).toStringAsFixed(1)} '
          'MiB_per_s=${(((bytes - b0) / (1 << 20)) / secs).toStringAsFixed(1)} '
          'window_s=${secs.toStringAsFixed(2)}');
      return;
    }

    // decode / stall: paced JPEG frames + latest-wins coalescing consumer.
    int received = 0;
    int painted = 0;
    int coalesced = 0;
    int decodeFail = 0;
    Uint8List? pending;
    DateTime? pendingSent;
    DateTime? lastSent;
    final transportAge = <double>[];
    final decodeMs = <double>[];
    final rasterMs = <double>[];
    final paintAge = <double>[];
    final postStallPaintAge = <double>[];

    final firstMsg = Completer<void>();
    DateTime? stallDone;

    // Memory (RSS) sampling: baseline at window start, peak after each paint.
    int rssStart = ProcessInfo.currentRss;
    int rssPeak = rssStart;

    // Decode-on-arrival: decode immediately when idle; while a decode is in
    // flight keep only the newest frame (latest-wins) and pick it up as soon
    // as the current decode completes — no fixed consumer tick.
    bool busy = false;
    void pump() {
      if (busy || pending == null) return;
      busy = true;
      final bytes = pending!;
      final sent = pendingSent;
      pending = null;
      pendingSent = null;
      _decodeAndPaint(bytes, sent).then((res) {
        busy = false;
        if (res == null) {
          decodeFail++;
        } else {
          painted++;
          decodeMs.add(res.decodeMs);
          rasterMs.add(res.rasterMs);
          final age = _ms(res.paintDone, res.sent ?? res.paintDone);
          paintAge.add(age);
          final sd = stallDone;
          if (sd != null && res.paintDone.isAfter(sd)) {
            postStallPaintAge.add(age);
          }
          final rss = ProcessInfo.currentRss;
          if (rss > rssPeak) rssPeak = rss;
        }
        pump(); // a newer frame may have arrived while decoding
      });
    }

    final sub = client.streamEvents().listen((r) {
      if (!r.envelope.hasClipboardUpdate()) return;
      final raw = r.envelope.clipboardUpdate.payload;
      final Uint8List payload =
          raw is Uint8List ? raw : Uint8List.fromList(raw);
      final sentMicros = _sentMicros(payload);
      received++;
      lastSent =
          sentMicros == null ? null : DateTime.fromMicrosecondsSinceEpoch(sentMicros);
      if (pending != null) coalesced++;
      pending = _frameBytes(payload);
      pendingSent = lastSent;
      if (lastSent != null) {
        transportAge.add(_ms(DateTime.now(), lastSent!));
      }
      if (!firstMsg.isCompleted) firstMsg.complete();
      pump();
    });

    await firstMsg.future.timeout(const Duration(seconds: 90));
    // settle 1s so the window starts mid-stream
    await Future<void>.delayed(const Duration(seconds: 1));

    // Re-baseline RSS at the measurement window start.
    rssStart = ProcessInfo.currentRss;
    rssPeak = rssStart;

    if (phase == 'stall') {
      await Future<void>.delayed(const Duration(seconds: 2));
      final stallStart = DateTime.now();
      final end = stallStart.add(const Duration(milliseconds: 300));
      while (DateTime.now().isBefore(end)) {
        // Simulated UI jank: the isolate (delivery + timers + paint) halts.
      }
      stallDone = DateTime.now();
      print('INFO stall applied: ${_ms(stallDone, stallStart).toStringAsFixed(1)} ms '
          'at received=$received');
      await Future<void>.delayed(const Duration(seconds: 6));
    } else {
      await Future<void>.delayed(const Duration(seconds: 8));
    }

    await sub.cancel();
    await client.shutdown();

    int rssEnd = ProcessInfo.currentRss;
    if (rssEnd > rssPeak) rssPeak = rssEnd;

    print('RESULT p2_$phase received=$received painted=$painted '
        'coalesced_dropped=$coalesced decode_fail=$decodeFail '
        'consumer=doa');
    print('RESULT p2_$phase rss_kb start=${rssStart ~/ 1024} '
        'end=${rssEnd ~/ 1024} peak=${rssPeak ~/ 1024}');
    print('RESULT p2_$phase transport_age_ms p50=${_p50(transportAge).toStringAsFixed(1)} '
        'p95=${_p95(transportAge).toStringAsFixed(1)} '
        'max=${(transportAge.isEmpty ? 0.0 : transportAge.reduce((a, b) => a > b ? a : b)).toStringAsFixed(1)}');
    print('RESULT p2_$phase decode_ms p50=${_p50(decodeMs).toStringAsFixed(1)} '
        'p95=${_p95(decodeMs).toStringAsFixed(1)} '
        'max=${(decodeMs.isEmpty ? 0.0 : decodeMs.reduce((a, b) => a > b ? a : b)).toStringAsFixed(1)}');
    print('RESULT p2_$phase raster_ms p50=${_p50(rasterMs).toStringAsFixed(1)} '
        'p95=${_p95(rasterMs).toStringAsFixed(1)} '
        'max=${(rasterMs.isEmpty ? 0.0 : rasterMs.reduce((a, b) => a > b ? a : b)).toStringAsFixed(1)}');
    print('RESULT p2_$phase paint_age_ms p50=${_p50(paintAge).toStringAsFixed(1)} '
        'p95=${_p95(paintAge).toStringAsFixed(1)} '
        'max=${(paintAge.isEmpty ? 0.0 : paintAge.reduce((a, b) => a > b ? a : b)).toStringAsFixed(1)}');
    if (phase == 'stall') {
      print('RESULT p2_stall post_stall_paint_age_ms '
          'p50=${_p50(postStallPaintAge).toStringAsFixed(1)} '
          'p95=${_p95(postStallPaintAge).toStringAsFixed(1)} '
          'max=${(postStallPaintAge.isEmpty ? 0.0 : postStallPaintAge.reduce((a, b) => a > b ? a : b)).toStringAsFixed(1)}');
    }
    expect(received, greaterThan(0), reason: 'no frames arrived — server window missed');
  }, timeout: const Timeout(Duration(minutes: 4)));
}
