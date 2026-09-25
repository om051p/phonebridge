// Production frame consumer for Phase 6 Slice 3A.
//
//	StreamFrames (<=64 KiB chunks)
//	→ reassembly (frame_id / chunk_index / last_chunk)
//	→ decode-on-arrival (decode immediately when idle, keep ONLY the newest
//	   complete JPEG while a decode is in flight)
//	→ one live dart:ui image for the mirror surface.
//
// Deliberately separate from the event stream: this never touches
// streamEvents, so frame load can never cost the app a control event.
// Memory is bounded by construction: one partial frame buffer, one pending
// JPEG, one decoded image (the previous one is disposed on replacement).

import 'dart:async';
import 'dart:typed_data';
import 'dart:ui' as ui;

import 'package:flutter/foundation.dart';

import '../generated/phonebridge/localipc/v1/local_ipc.pb.dart';

/// The per-message JPEG ceiling the daemon promises (DEC-018 bulk rule).
const int kMaxFrameChunkBytes = 64 * 1024;

/// Decodes one JPEG into a [ui.Image]; returns null when undecodable.
/// Injectable so tests can drive decode timing without a real codec.
typedef FrameDecoder = Future<ui.Image?> Function(Uint8List jpeg);

/// One painted frame's delivery facts (diagnostics/E2E measurement):
/// latency is daemon-frame-timestamp → painted, on one machine's clock.
@immutable
class FramePaintInfo {
  const FramePaintInfo({
    required this.latencyMicros,
    required this.jpegBytes,
    required this.width,
    required this.height,
  });

  final int latencyMicros;
  final int jpegBytes;
  final int width;
  final int height;
}

/// Counters describing the consumer's health (diagnostics/tests).
@immutable
class FrameStreamStats {
  const FrameStreamStats({
    this.chunksReceived = 0,
    this.framesComplete = 0,
    this.framesPainted = 0,
    this.framesIncompleteDropped = 0,
    this.framesCoalesced = 0,
    this.frameIdGaps = 0,
    this.decodeFailures = 0,
    this.oversizeChunks = 0,
    this.streamErrors = 0,
    this.lastPaintLatencyMicros,
    this.lastFrameAt,
  });

  final int chunksReceived;
  final int framesComplete;
  final int framesPainted;

  /// Frames abandoned because a newer frame started before the previous one
  /// finished reassembling (client-side latest-wins at the byte level).
  final int framesIncompleteDropped;

  /// Complete frames dropped because a decode was already running — the
  /// decode-on-arrival coalescing (this replaces the prototype's 33 ms tick).
  final int framesCoalesced;

  /// Frame-id jumps: frames the daemon-side latest-wins queue dropped before
  /// delivery (the client-visible drop signal).
  final int frameIdGaps;
  final int decodeFailures;
  final int oversizeChunks;
  final int streamErrors;

  /// Daemon-frame-timestamp → painted latency of the latest paint (null until
  /// the first paint of a run that carried a timestamp).
  final int? lastPaintLatencyMicros;
  final DateTime? lastFrameAt;

  FrameStreamStats copyWith({
    int? chunksReceived,
    int? framesComplete,
    int? framesPainted,
    int? framesIncompleteDropped,
    int? framesCoalesced,
    int? frameIdGaps,
    int? decodeFailures,
    int? oversizeChunks,
    int? streamErrors,
    int? lastPaintLatencyMicros,
    DateTime? lastFrameAt,
  }) {
    return FrameStreamStats(
      chunksReceived: chunksReceived ?? this.chunksReceived,
      framesComplete: framesComplete ?? this.framesComplete,
      framesPainted: framesPainted ?? this.framesPainted,
      framesIncompleteDropped:
          framesIncompleteDropped ?? this.framesIncompleteDropped,
      framesCoalesced: framesCoalesced ?? this.framesCoalesced,
      frameIdGaps: frameIdGaps ?? this.frameIdGaps,
      decodeFailures: decodeFailures ?? this.decodeFailures,
      oversizeChunks: oversizeChunks ?? this.oversizeChunks,
      streamErrors: streamErrors ?? this.streamErrors,
      lastPaintLatencyMicros:
          lastPaintLatencyMicros ?? this.lastPaintLatencyMicros,
      lastFrameAt: lastFrameAt ?? this.lastFrameAt,
    );
  }
}

/// Supplies one frame stream per subscription (a fake supplies these in tests;
/// the Linux bridge wires the real daemon RPC).
typedef FrameStreamSource = Stream<StreamFramesResponse> Function();

/// Implemented by a platform service that can open the daemon's frame stream.
/// Resolved structurally (like SessionBackend) so Android — which has no
/// local-IPC frame stream — simply does not implement it and the UI keeps its
/// existing presentation.
abstract class ProvidesFrameStream {
  FrameStream createFrameStream();
}

class FrameStream extends ChangeNotifier {
  FrameStream({
    required FrameStreamSource source,
    FrameDecoder? decoder,
    this.onFramePainted,
  })  : _source = source,
        _decoder = decoder ?? _decodeJpeg;

  /// Observer hook for delivery diagnostics (E2E measurement / telemetry).
  /// Never called on the hot path for logic the consumer itself needs —
  /// stats carry that — this is purely for external measurement.
  final void Function(FramePaintInfo info)? onFramePainted;

  final FrameStreamSource _source;
  final FrameDecoder _decoder;

  StreamSubscription<StreamFramesResponse>? _sub;
  bool _started = false;
  bool _disposed = false;

  // Reassembly state for the frame currently on the wire.
  int? _frameId;
  int? _lastSeenId;
  int? _frameSent;
  BytesBuilder? _buffer;
  int _chunksGot = 0;
  int _expectedChunks = 0;

  // Decode-on-arrival state.
  bool _decoding = false;
  Uint8List? _pending;
  int _pendingSent = 0;

  ui.Image? _image;
  int _width = 0;
  int _height = 0;
  FrameStreamStats _stats = const FrameStreamStats();

  /// Latest decoded frame, or null until one is painted (and after stop()).
  ui.Image? get image => _image;

  /// Dimensions of the latest frame (0x0 before the first frame).
  int get width => _width;
  int get height => _height;

  /// 0 before the first frame; correct aspect ratio afterwards.
  double get aspectRatio => _height == 0 ? 0 : _width / _height;

  bool get hasFrame => _image != null;
  bool get isStarted => _started;
  FrameStreamStats get stats => _stats;

  /// Opens the frame subscription. Idempotent.
  void start() {
    if (_started || _disposed) return;
    _started = true;
    _sub = _source().listen(
      _onChunk,
      onError: (Object _) {
        if (_disposed) return;
        _stats = _stats.copyWith(streamErrors: _stats.streamErrors + 1);
        notifyListeners();
      },
      // The source (local IPC client) reconnects on its own; a done here
      // means the subscription is over — stop cleanly, never spin.
      onDone: stop,
      cancelOnError: false,
    );
  }

  /// Unsubscribes and releases the decoded frame. Idempotent. The next
  /// start() opens a fresh subscription (clean unsubscribe on stop/dispose).
  void stop() {
    if (!_started) return;
    _started = false;
    final sub = _sub;
    _sub = null;
    sub?.cancel();
    _resetReassembly();
    _pending = null;
    final img = _image;
    _image = null;
    img?.dispose();
    notifyListeners();
  }

  @override
  void dispose() {
    if (_disposed) return; // idempotent: ChangeNotifier.dispose asserts twice
    // stop() first (still legal to notify), then mark disposed: onDone can
    // fire after a cancel only via a race, and _disposed gates every path.
    stop();
    _disposed = true;
    super.dispose();
  }

  // ---- Reassembly -------------------------------------------------------

  void _onChunk(StreamFramesResponse msg) {
    if (_disposed) return;
    _stats = _stats.copyWith(chunksReceived: _stats.chunksReceived + 1);

    if (msg.jpeg.length > kMaxFrameChunkBytes) {
      // Contract breach: drop the whole frame rather than accept a message
      // the daemon promised never to send.
      _stats = _stats.copyWith(oversizeChunks: _stats.oversizeChunks + 1);
      _abandonFrame();
      return;
    }

    final id = msg.frameId.toInt();
    if (_frameId == null) {
      _beginFrame(msg);
    } else if (id != _frameId) {
      // A newer frame started: the previous one is unrecoverable.
      _stats = _stats.copyWith(
          framesIncompleteDropped: _stats.framesIncompleteDropped + 1);
      _beginFrame(msg);
    } else if (msg.chunkIndex != _chunksGot) {
      // Out-of-order/lost chunk within a frame: only the whole frame matters.
      _abandonFrame();
      return;
    }

    if (_frameId == null) return; // _beginFrame rejected it

    _buffer!.add(msg.jpeg);
    _chunksGot++;

    if (msg.lastChunk) {
      final expected = msg.chunkCount != 0 ? msg.chunkCount : _expectedChunks;
      if (expected != 0 && _chunksGot != expected) {
        _stats = _stats.copyWith(
            framesIncompleteDropped: _stats.framesIncompleteDropped + 1);
        _resetReassembly();
        return;
      }
      final jpeg = _buffer!.toBytes();
      final sent = _frameSent ?? 0;
      _resetReassembly();
      _stats = _stats.copyWith(
        framesComplete: _stats.framesComplete + 1,
        lastFrameAt: DateTime.now(),
      );
      _deliver(jpeg, sent);
    }
  }

  void _beginFrame(StreamFramesResponse msg) {
    if (msg.chunkIndex != 0) {
      // Stream joined mid-frame (e.g. right after a resubscribe): wait for
      // the next chunk-0 instead of assembling a partial frame.
      _resetReassembly();
      return;
    }
    final id = msg.frameId.toInt();
    final last = _lastSeenId;
    if (last != null && id > last + 1) {
      _stats =
          _stats.copyWith(frameIdGaps: _stats.frameIdGaps + (id - last - 1));
    }
    _lastSeenId = id;
    _frameId = id;
    _frameSent = msg.sentUnixMicros.toInt();
    _buffer = BytesBuilder(copy: false);
    _chunksGot = 0;
    _expectedChunks = msg.chunkCount;
  }

  void _abandonFrame() {
    final last = _lastSeenId;
    if (last != null && _frameId != null && _frameId! > last) {
      _lastSeenId = _frameId;
    }
    _resetReassembly();
  }

  void _resetReassembly() {
    _frameId = null;
    _frameSent = null;
    _buffer = null;
    _chunksGot = 0;
    _expectedChunks = 0;
  }

  // ---- Decode-on-arrival ------------------------------------------------

  void _deliver(Uint8List jpeg, int sentMicros) {
    if (_decoding) {
      // Busy: retain ONLY the newest complete frame (newest-wins, bounded).
      if (_pending != null) {
        _stats = _stats.copyWith(framesCoalesced: _stats.framesCoalesced + 1);
      }
      _pending = jpeg;
      _pendingSent = sentMicros;
      return;
    }
    _runDecode(jpeg, sentMicros);
  }

  Future<void> _runDecode(Uint8List jpeg, int sentMicros) async {
    _decoding = true;
    ui.Image? decoded;
    try {
      decoded = await _decoder(jpeg);
    } catch (_) {
      decoded = null;
    }
    _decoding = false;
    if (_disposed) {
      decoded?.dispose();
      return;
    }
    if (decoded == null) {
      _stats = _stats.copyWith(decodeFailures: _stats.decodeFailures + 1);
    } else {
      final old = _image;
      _image = decoded;
      _width = decoded.width;
      _height = decoded.height;
      old?.dispose();
      var painted = _stats.copyWith(framesPainted: _stats.framesPainted + 1);
      if (sentMicros > 0) {
        final latency = DateTime.now().microsecondsSinceEpoch - sentMicros;
        painted = painted.copyWith(lastPaintLatencyMicros: latency);
        onFramePainted?.call(FramePaintInfo(
          latencyMicros: latency,
          jpegBytes: jpeg.length,
          width: decoded.width,
          height: decoded.height,
        ));
      }
      _stats = painted;
    }
    notifyListeners();

    final next = _pending;
    final nextSent = _pendingSent;
    _pending = null;
    _pendingSent = 0;
    if (next != null && !_disposed && _started) {
      // Decode the newest retained frame immediately: no tick, no timer.
      unawaited(_runDecode(next, nextSent));
    }
  }

  static Future<ui.Image?> _decodeJpeg(Uint8List jpeg) => decodeJpegFrame(jpeg);
}

/// The production decoder: JPEG bytes → [ui.Image] via dart:ui (no FFI, no
/// native plugin — the validated P2 path). Public so tests can gate or wrap
/// the real decode when they need to control decode timing.
Future<ui.Image?> decodeJpegFrame(Uint8List jpeg) async {
  final codec = await ui.instantiateImageCodec(jpeg);
  try {
    final frame = await codec.getNextFrame();
    return frame.image;
  } finally {
    codec.dispose();
  }
}
