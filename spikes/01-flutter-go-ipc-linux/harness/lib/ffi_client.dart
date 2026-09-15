// Spike 01 harness — FFI branch (EXPERIMENTAL, throwaway).
//
// Loads the c-shared Go library in-process. Two facts are worth noting up front
// because they are the whole comparison:
//   * there is no boundary to authenticate — every export is callable by any
//     code in the process, and there is no uid/token gate to apply;
//   * every export is synchronous *on the calling thread*, so calling one from
//     the UI isolate blocks Flutter (see blockMicros + the perf test).

import 'dart:ffi';
import 'dart:typed_data';

import 'package:ffi/ffi.dart';

typedef _VersionNative = Int32 Function(Pointer<Uint8> buf, Int32 bufLen);
typedef _VersionDart = int Function(Pointer<Uint8> buf, int bufLen);

typedef _PingNative = Uint64 Function(Uint64 nonce);
typedef _PingDart = int Function(int nonce);

typedef _EchoNative = Int32 Function(
    Pointer<Uint8> src, Int32 srcLen, Pointer<Uint8> dst, Int32 dstCap);
typedef _EchoDart = int Function(
    Pointer<Uint8> src, int srcLen, Pointer<Uint8> dst, int dstCap);

typedef _StartTickerNative = Uint64 Function(Int64 intervalMicros);
typedef _StartTickerDart = int Function(int intervalMicros);

typedef _PollEventNative = Uint64 Function();
typedef _PollEventDart = int Function();

typedef _BlockNative = Void Function(Int64 micros);
typedef _BlockDart = void Function(int micros);

typedef _GoroutinesNative = Int32 Function();
typedef _GoroutinesDart = int Function();

/// FFI view of the Go core loaded into this process.
class FfiSpikeClient {
  FfiSpikeClient._(this._lib);

  /// Loads the shared library and records how long dlopen + full symbol
  /// resolution took (the cold-start datapoint for the FFI branch).
  factory FfiSpikeClient.open(String path) {
    final sw = Stopwatch()..start();
    final client = FfiSpikeClient._(DynamicLibrary.open(path));
    // Resolve every export up front: this makes the measured cost real and
    // fails fast on a missing symbol instead of mid-benchmark.
    client._resolveAll();
    sw.stop();
    client._loadMicros = sw.elapsedMicroseconds;
    return client;
  }

  final DynamicLibrary _lib;

  int _loadMicros = 0;

  /// Microseconds spent in DynamicLibrary.open + symbol resolution.
  int get loadMicros => _loadMicros;

  late final _ping = _lib.lookupFunction<_PingNative, _PingDart>('PBSpikePing');
  late final _version =
      _lib.lookupFunction<_VersionNative, _VersionDart>('PBSpikeVersion');
  late final _echo = _lib.lookupFunction<_EchoNative, _EchoDart>('PBSpikeEcho');
  late final _startTicker = _lib
      .lookupFunction<_StartTickerNative, _StartTickerDart>('PBSpikeStartTicker');
  late final _pollEvent =
      _lib.lookupFunction<_PollEventNative, _PollEventDart>('PBSpikePollEvent');
  late final _block = _lib.lookupFunction<_BlockNative, _BlockDart>('PBSpikeBlock');
  late final _goroutines = _lib
      .lookupFunction<_GoroutinesNative, _GoroutinesDart>('PBSpikeGoroutines');

  void _resolveAll() {
    final resolved = <Object?>[
      _ping,
      _version,
      _echo,
      _startTicker,
      _pollEvent,
      _block,
      _goroutines,
    ];
    if (resolved.length != 7) {
      throw StateError('FFI export resolution returned an unexpected arity');
    }
  }

  /// In-process round trip: no serialisation, no syscalls, no syscall boundary.
  int ping(int nonce) => _ping(nonce);

  /// String marshalling through a caller-owned buffer.
  String version() {
    final buf = calloc<Uint8>(64);
    try {
      final n = _version(buf, 64);
      if (n < 0) throw StateError('version buffer too small');
      return String.fromCharCodes(buf.asTypedList(n));
    } finally {
      calloc.free(buf);
    }
  }

  /// Binary-safe echo into caller-owned buffers. Use this low-level form in
  /// benchmarks so allocation is not measured.
  int echoRaw(Pointer<Uint8> src, int srcLen, Pointer<Uint8> dst, int dstCap) =>
      _echo(src, srcLen, dst, dstCap);

  /// Convenience form: allocates both buffers (not for benchmarking).
  Uint8List echo(Uint8List data) {
    if (data.isEmpty) return Uint8List(0);
    final src = calloc<Uint8>(data.length);
    final dst = calloc<Uint8>(data.length);
    try {
      src.asTypedList(data.length).setAll(0, data);
      final n = _echo(src, data.length, dst, data.length);
      return Uint8List.fromList(dst.asTypedList(n));
    } finally {
      calloc.free(src);
      calloc.free(dst);
    }
  }

  /// Blocks the calling thread (and therefore the calling isolate) for [micros].
  void blockMicros(int micros) => _block(micros);

  /// Starts the Go-side producer; returns the current event sequence.
  int startTicker(int intervalMicros) => _startTicker(intervalMicros);

  /// The only Go → Dart channel this branch has: polling.
  int pollEvent() => _pollEvent();

  /// Non-zero proves the Go runtime/GC/scheduler live inside the Flutter process.
  int goroutines() => _goroutines();
}