// Spike 01 harness — UDS + gRPC branch (EXPERIMENTAL, throwaway).
//
// The whole Dart side of this branch is: one `ClientChannel` pointing at an
// `InternetAddress(type: unix)`. No plugin, no platform channel, no native code,
// no CMake change — which is itself the answer to the spike's
// "Flutter–UDS integration requires fragile native code" failure criterion.

import 'dart:io';
import 'dart:typed_data';

import 'package:fixnum/fixnum.dart';
import 'package:grpc/grpc.dart';

// local_ipc.pbgrpc.dart re-exports local_ipc.pb.dart, so one import is enough.
import 'generated/phonebridge/spike/localipc/v1/local_ipc.pbgrpc.dart';

/// Thin wrapper over the generated stub, with the auth metadata wired in.
class UdsSpikeClient {
  UdsSpikeClient({
    required this.socketPath,
    this.token,
    this.connectTimeout = const Duration(seconds: 3),
    this.idleTimeout = const Duration(seconds: 60),
    this.callTimeout = const Duration(seconds: 30),
  });

  final String socketPath;

  /// Sent as `authorization: Bearer <token>` when non-null.
  final String? token;

  final Duration connectTimeout;
  final Duration idleTimeout;
  final Duration callTimeout;

  ClientChannel? _channel;
  LocalIpcSpikeServiceClient? _stub;

  /// Wall time of the *first* RPC on a fresh channel: socket connect + HTTP/2
  /// handshake + first request. Reported as "cold RPC" latency.
  Duration? firstCallWallTime;

  /// Creates the channel object. The socket itself is opened lazily by gRPC on
  /// the first RPC, so this does not prove reachability.
  void connect() {
    if (_channel != null) return;
    _channel = ClientChannel(
      // The unix-socket form of the target: grpc-dart passes this straight to
      // Socket.connect and derives authority "localhost" for it.
      InternetAddress(socketPath, type: InternetAddressType.unix),
      port: 0,
      options: ChannelOptions(
        credentials: const ChannelCredentials.insecure(),
        connectTimeout: connectTimeout,
        idleTimeout: idleTimeout,
      ),
    );
    _stub = LocalIpcSpikeServiceClient(_channel!);
  }

  LocalIpcSpikeServiceClient get _service {
    final s = _stub;
    if (s == null) throw StateError('connect() was not called');
    return s;
  }

  CallOptions get _callOptions => CallOptions(
        metadata: token == null ? const {} : {'authorization': 'Bearer $token'},
        timeout: callTimeout,
      );

  /// Issues an RPC that includes connection establishment; records the wall time.
  Future<PingResponse> warmUp() async {
    final sw = Stopwatch()..start();
    final res = await ping(nonce: 0);
    sw.stop();
    firstCallWallTime = sw.elapsed;
    return res;
  }

  Future<PingResponse> ping({int nonce = 1, int payloadBytes = 0}) {
    return _service.ping(
      PingRequest(
        nonce: Int64(nonce),
        payload: deterministicPayload(payloadBytes),
      ),
      options: _callOptions,
    );
  }

  Stream<SubscribeResponse> subscribe({
    int count = 10,
    int intervalMs = 50,
    int payloadSize = 0,
  }) {
    return _service.subscribe(
      SubscribeRequest(
        count: count,
        intervalMs: intervalMs,
        payloadSize: payloadSize,
      ),
      options: _callOptions,
    );
  }

  Future<WhoAmIResponse> whoAmI() =>
      _service.whoAmI(WhoAmIRequest(), options: _callOptions);

  /// Closes the channel. Safe to call repeatedly; used by the leak test.
  Future<void> close() async {
    final ch = _channel;
    _channel = null;
    _stub = null;
    if (ch != null) await ch.shutdown();
  }
}

/// Deterministic filler so payload sweeps compare like with like (and so a
/// failing byte can be traced back to its offset).
Uint8List deterministicPayload(int size) {
  if (size <= 0) return Uint8List(0);
  final b = Uint8List(size);
  for (var i = 0; i < size; i++) {
    b[i] = i % 251;
  }
  return b;
}