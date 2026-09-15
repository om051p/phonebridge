// Spike 01 — UDS+gRPC feasibility tests (EXPERIMENTAL, throwaway).
//
// Runs inside the real Flutter test runner (flutter tester), i.e. the same Dart
// VM and dart:io stack the Linux desktop app uses. What is deliberately NOT
// covered here: GTK window creation and the plugin registration path (both are
// exercised by the optional `flutter build linux` check in the runbook).

import 'dart:io';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:phonebridge_spike01_harness/daemon_harness.dart';
import 'package:phonebridge_spike01_harness/uds_client.dart';

void main() {
  late Directory dir;
  late String socketPath;
  late SpikeDaemon daemon;

  setUp(() async {
    dir = SpikeSockets.tempDir();
    socketPath = '${dir.path}/engine.sock';
    daemon = await SpikeDaemon.start(socketPath: socketPath);
  });

  tearDown(() async {
    if (daemon.isRunning) await daemon.terminateAndCollectStats();
    if (dir.existsSync()) dir.deleteSync(recursive: true);
  });

  test('Flutter process reaches the Go daemon over UDS+gRPC (unary)', () async {
    final client = UdsSpikeClient(socketPath: socketPath)..connect();
    try {
      // First call includes socket connect + HTTP/2 handshake.
      final cold = await client.warmUp();
      expect(cold.nonce.toInt(), 0);
      expect(cold.serverVersion, startsWith('0.0.0'),
          reason: 'daemon must report its version so the UI can detect a '
              'restart/upgrade after reconnect');
      expect(client.firstCallWallTime, isNotNull);

      final warm = await client.ping(nonce: 42, payloadBytes: 1024);
      expect(warm.nonce.toInt(), 42, reason: 'reply must belong to this call');
      expect(warm.payload.length, 1024);
      expect(warm.serverRecvUnixNanos.toInt(), greaterThan(0));
    } finally {
      await client.close();
    }

    // The daemon's own counters confirm the RPCs actually arrived.
    final stats = await daemon.terminateAndCollectStats();
    expect(int.parse(stats['pings']!), 2);
    expect(int.parse(stats['rejected']!), 0);
  });

  test('Go -> Flutter push works: server streaming delivers ordered events',
      () async {
    final client = UdsSpikeClient(socketPath: socketPath)..connect();
    try {
      final events = await client
          .subscribe(count: 20, intervalMs: 10, payloadSize: 128)
          .toList();
      expect(events.length, 20);
      expect(events.map((e) => e.seq.toInt()).toList(),
          List<int>.generate(20, (i) => i + 1),
          reason: 'sequence must be gap-free: this is the push channel the UI '
              'would use for clipboard/notification events');
      expect(events.every((e) => e.kind == 'tick'), isTrue);
      expect(events.every((e) => e.payload.length == 128), isTrue);
      expect(events.every((e) => e.emittedUnixNanos.toInt() > 0), isTrue);
    } finally {
      await client.close();
    }
  });

  test('peer identity is kernel-provided: WhoAmI reports this process uid',
      () async {
    final client = UdsSpikeClient(socketPath: socketPath)..connect();
    try {
      final who = await client.whoAmI();
      expect(who.peerUid, int.parse(daemon.readyLine['server_uid']!),
          reason: 'the peer uid comes from SO_PEERCRED, not from the caller');
      expect(who.serverUid, who.peerUid);
      expect(who.socketPath, socketPath);
      expect(who.tokenRequired, isFalse);
      // peer_pid must be *this* test process (the Dart VM), not a shell.
      expect(who.peerPid, pid);
    } finally {
      await client.close();
    }
  });

  test('a torn-down channel can be recreated against the same daemon',
      () async {
    for (var i = 0; i < 3; i++) {
      final client = UdsSpikeClient(socketPath: socketPath)..connect();
      final res = await client.ping(nonce: i);
      expect(res.nonce.toInt(), i);
      await client.close();
    }
    final stats = await daemon.terminateAndCollectStats();
    expect(int.parse(stats['pings']!), 3);
  });

  testWidgets('an RPC can be issued from Flutter widget code',
      (tester) async {
    final client = UdsSpikeClient(socketPath: socketPath)..connect();
    await tester.runAsync(() async {
      final future = client.ping(nonce: 7);
      await tester.pumpWidget(_PingProbe(future: future));
      await future;
      await tester.pump();
      await client.close();
    });
    expect(find.text('pong 7'), findsOneWidget);
  });
}

/// Minimal widget proving the RPC composes with the Flutter widget layer
/// without a plugin or a MethodChannel.
class _PingProbe extends StatelessWidget {
  const _PingProbe({required this.future});

  final Future<dynamic> future;

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      home: FutureBuilder<dynamic>(
        future: future,
        builder: (context, snap) => Text(
          snap.hasData ? 'pong ${snap.data.nonce}' : 'waiting',
        ),
      ),
    );
  }
}
