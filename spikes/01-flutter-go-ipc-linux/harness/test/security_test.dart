// Spike 01 — trust-boundary tests (EXPERIMENTAL, throwaway).
//
// What is verified: filesystem permissions on the socket, the bearer-token gate,
// the SO_PEERCRED uid gate, and the fact that the FFI branch has no gate at all.
//
// What is NOT verified here (environment has no second user / no sudo): an
// actual connect attempt from a different uid. The filesystem check plus the
// uid-mismatch test are the closest available proxies; see the results doc.

import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:grpc/grpc.dart' as grpc;
import 'package:phonebridge_spike01_harness/daemon_harness.dart';
import 'package:phonebridge_spike01_harness/ffi_client.dart';
import 'package:phonebridge_spike01_harness/paths.dart';
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

  test('socket dir is 0700 and the socket is 0600 (no world access)', () {
    // Numeric permission bits: unambiguous, and independent of how
    // FileStat.modeString() formats the type character.
    expect(dir.statSync().mode & 0x1FF, 0x1C0, // 0700
        reason: 'the runtime dir must not be traversable by other users');
    expect(File(socketPath).statSync().mode & 0x1FF, 0x180, // 0600
        reason: 'a world-accessible control socket is the whole vulnerability');
    expect(daemon.socketMode, '0600',
        reason: 'the daemon must verify/report the mode it created');
  });

  test('bearer token gate: missing and wrong tokens are UNAUTHENTICATED',
      () async {
    await daemon.terminateAndCollectStats();
    daemon = await SpikeDaemon.start(socketPath: socketPath, token: 's3cret');

    final noToken = UdsSpikeClient(socketPath: socketPath)..connect();
    await expectLater(
      noToken.ping(),
      throwsA(isA<grpc.GrpcError>()
          .having((e) => e.code, 'code', grpc.StatusCode.unauthenticated)),
    );
    await noToken.close();

    final wrongToken = UdsSpikeClient(socketPath: socketPath, token: 'wrong')
      ..connect();
    await expectLater(
      wrongToken.ping(),
      throwsA(isA<grpc.GrpcError>()
          .having((e) => e.code, 'code', grpc.StatusCode.unauthenticated)),
    );
    await wrongToken.close();

    // The correct token works — the gate is not simply rejecting everything.
    final good = UdsSpikeClient(socketPath: socketPath, token: 's3cret')
      ..connect();
    final ok = await good.ping(nonce: 1);
    expect(ok.nonce.toInt(), 1);
    expect((await good.whoAmI()).tokenRequired, isTrue);
    await good.close();

    final stats = await daemon.terminateAndCollectStats();
    expect(int.parse(stats['rejected']!), 2,
        reason: 'both rejected calls must be counted by the daemon');
  });

  test('SO_PEERCRED gate: a uid mismatch is PERMISSION_DENIED even with the '
      'right token', () async {
    final serverUid = int.parse(daemon.readyLine['server_uid']!);
    await daemon.terminateAndCollectStats();

    // Same user on the filesystem, but the daemon expects a different uid: this
    // is how the peer-cred enforcement is proven without a second account.
    daemon = await SpikeDaemon.start(
      socketPath: socketPath,
      token: 's3cret',
      expectUid: serverUid + 1,
    );

    final client =
        UdsSpikeClient(socketPath: socketPath, token: 's3cret')..connect();
    await expectLater(
      client.ping(),
      throwsA(isA<grpc.GrpcError>()
          .having((e) => e.code, 'code', grpc.StatusCode.permissionDenied)),
    );
    await client.close();

    final stats = await daemon.terminateAndCollectStats();
    expect(int.parse(stats['rejected']!), 1);
  });

  test('a 0000 socket file cannot be connected to (connect needs write perm)',
      () async {
    await Process.run('chmod', ['0000', socketPath]);
    expect(File(socketPath).statSync().mode & 0x1FF, 0, // 0000
        reason: 'test must actually remove connect permissions');

    final client = UdsSpikeClient(
      socketPath: socketPath,
      callTimeout: const Duration(milliseconds: 800),
      connectTimeout: const Duration(milliseconds: 300),
    )..connect();
    await expectLater(client.ping(), throwsA(isA<grpc.GrpcError>()));
    await client.close();

    await Process.run('chmod', ['0600', socketPath]);
  });

  test('FFI has no boundary to authenticate (documented tradeoff)', () {
    final ffi = FfiSpikeClient.open(SpikePaths.requireFfiLibrary().path);
    // No token, no uid check, no peer credentials: the core is simply in the
    // process. Any code that can load the library can call every export, and
    // the process boundary that made SO_PEERCRED meaningful no longer exists.
    expect(ffi.ping(5), 5);
    expect(ffi.version(), startsWith('0.0.0'));
    expect(ffi.goroutines(), greaterThanOrEqualTo(1),
        reason: 'the Go runtime is live inside the Flutter process');
  });
}