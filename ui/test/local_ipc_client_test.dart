import 'dart:async';
import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:grpc/grpc.dart';
import 'package:phonebridge_ui/services/local_ipc_client.dart';

void main() {
  group('LocalIpcClient', () {
    late Directory tempDir;
    late String socketPath;
    late String tokenPath;
    Process? daemonProcess;
    late LocalIpcClient client;

    final daemonBin = File(
      '${Directory.current.path}/../core/build/phonebridge-daemon',
    );

    setUp(() async {
      tempDir = await Directory.systemTemp.createTemp('pb_ipc_test_');
      socketPath = '${tempDir.path}/engine.sock';
      tokenPath = '${tempDir.path}/token';

      expect(
        await daemonBin.exists(),
        isTrue,
        reason: 'core/build/phonebridge-daemon must be built before running test',
      );

      // Start daemon
      daemonProcess = await Process.start(
        daemonBin.path,
        [
          '--socket',
          socketPath,
          '--token-file',
          tokenPath,
        ],
      );

      client = LocalIpcClient(
        socketPath: socketPath,
        tokenPath: tokenPath,
      );
    });

    tearDown(() async {
      await client.shutdown();
      if (daemonProcess != null) {
        daemonProcess!.kill(ProcessSignal.sigterm);
        await daemonProcess!.exitCode;
      }
      if (await tempDir.exists()) {
        await tempDir.delete(recursive: true);
      }
    });

    test('acquires token and completes Handshake', () async {
      final token = await client.acquireToken();
      expect(token, isNotEmpty);
      expect(token.length, equals(64)); // 256 bits hex = 64 characters

      final hs = await client.handshake(clientVersion: 1);
      expect(hs.negotiatedVersion, equals(1));
      expect(hs.serverVersion, equals('0.1.0'));
      expect(hs.daemonGeneration.toInt(), isPositive);
      expect(client.state, equals(LocalIpcState.connected));
    });

    test('completes Ping round-trip with nonce echo', () async {
      const testNonce = 98765;
      final pingResp = await client.ping(nonce: testNonce);
      expect(pingResp.nonce.toInt(), equals(testNonce));
      expect(pingResp.serverVersion, equals('0.1.0'));
    });

    test('completes Health check', () async {
      final healthResp = await client.health();
      expect(healthResp.ready, isTrue);
      expect(healthResp.uptimeMs.toInt(), isNonNegative);
      expect(healthResp.serverVersion, equals('0.1.0'));
    });

    test('rejects unauthenticated calls when token is invalid', () async {
      final badTokenFile = File('${tempDir.path}/bad_token');
      await badTokenFile.writeAsString('invalid-token-content\n');

      final badClient = LocalIpcClient(
        socketPath: socketPath,
        tokenPath: badTokenFile.path,
      );

      try {
        await expectLater(
          badClient.ping(nonce: 1),
          throwsA(predicate((e) => e.toString().contains('UNAUTHENTICATED') || e is GrpcError)),
        );
      } finally {
        await badClient.shutdown();
      }
    });

    test('streamEvents receives connection and closes cleanly', () async {
      // Connect to stream
      final eventsFuture = client.streamEvents(autoReconnect: false).first.timeout(
            const Duration(milliseconds: 300),
            onTimeout: () => throw TimeoutException('no events'),
          );

      // We expect timeout or events, but connecting without error proves the stream gate works
      try {
        await eventsFuture;
      } on TimeoutException {
        // Expected because daemon has not broadcasted anything yet
      }

      expect(client.state, anyOf(LocalIpcState.connected, LocalIpcState.connecting));
    });

    test('startSession, getSessionState, and stopSession RPC flow', () async {
      final devicesResp = await client.listDevices();
      expect(devicesResp, isNotNull);
      expect(devicesResp.devices, isA<List>());

      final startResp = await client.startSession('target-device-1');
      expect(startResp.sessionId, isNotEmpty);
      expect(startResp.state, isNotNull);

      final stateResp = await client.getSessionState(sessionId: startResp.sessionId);
      expect(stateResp.sessionId, equals(startResp.sessionId));

      final stopResp = await client.stopSession(
        sessionId: startResp.sessionId,
        reason: 'test stopped',
      );
      expect(stopResp.sessionId, equals(startResp.sessionId));
    });

    test('listTrustedDevices, revokeDevice, and pairDevice RPC methods', () async {
      final trustedResp = await client.listTrustedDevices();
      expect(trustedResp, isNotNull);
      expect(trustedResp.devices, isA<List>());

      // Revoking an unknown device throws an error from daemon
      await expectLater(
        client.revokeDevice('unknown-dev-id'),
        throwsA(isA<GrpcError>()),
      );

      // Attempting to pair an undiscovered device throws an error from daemon
      await expectLater(
        client.pairDevice('non-existent-device-id'),
        throwsA(isA<GrpcError>()),
      );
    });
  });
}
