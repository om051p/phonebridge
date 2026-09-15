// Spike 01 — process lifecycle tests (EXPERIMENTAL, throwaway).
//
// Covers the cases that decide whether the daemon model is viable for a Linux
// desktop app: start/stop, crash recovery, restart-and-reconnect, double-start,
// socket hygiene, and (rough) leak signals from /proc.

import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:grpc/grpc.dart' as grpc;
import 'package:phonebridge_spike01_harness/daemon_harness.dart';
import 'package:phonebridge_spike01_harness/uds_client.dart';

void main() {
  late Directory dir;
  late String socketPath;

  setUp(() {
    dir = SpikeSockets.tempDir();
    socketPath = '${dir.path}/engine.sock';
  });

  tearDown(() {
    if (dir.existsSync()) dir.deleteSync(recursive: true);
  });

  test('start -> READY is fast and the daemon survives until stopped',
      () async {
    final sw = Stopwatch()..start();
    final daemon = await SpikeDaemon.start(socketPath: socketPath);
    sw.stop();
    stdout.writeln('daemon READY in ${sw.elapsedMilliseconds} ms');
    expect(sw.elapsedMilliseconds, lessThan(5000));

    final stats = await daemon.terminateAndCollectStats();
    expect(stats.containsKey('pings'), isTrue);
  });

  test('SIGTERM is a graceful stop: socket removed, no orphan process',
      () async {
    final daemon = await SpikeDaemon.start(socketPath: socketPath);
    expect(daemon.socketExists, isTrue);

    final stats = await daemon.terminateAndCollectStats();
    expect(stats['pings'], '0');
    expect(daemon.socketExists, isFalse,
        reason: 'a graceful stop must not leave a socket file behind');
    expect(daemon.stdoutLines.last, 'STOPPED');
    expect(daemon.isRunning, isFalse);
  });

  test('SIGKILL leaves a stale socket; the next start cleans it up', () async {
    final first = await SpikeDaemon.start(socketPath: socketPath);
    await first.kill();
    expect(first.socketExists, isTrue,
        reason: 'the crash must leave the socket file behind for this test');

    // The product systemd unit uses Restart=on-failure, so this is the normal
    // recovery path rather than an edge case.
    final second = await SpikeDaemon.start(socketPath: socketPath);
    final client = UdsSpikeClient(socketPath: socketPath)..connect();
    expect((await client.ping(nonce: 3)).nonce.toInt(), 3);
    await client.close();
    await second.terminateAndCollectStats();
  });

  test('a second daemon refuses to clobber a live socket', () async {
    final live = await SpikeDaemon.start(socketPath: socketPath);
    await expectLater(
      SpikeDaemon.start(socketPath: socketPath),
      throwsA(isA<StateError>()),
      reason: 'two daemons on one socket would silently split the UI',
    );
    final client = UdsSpikeClient(socketPath: socketPath)..connect();
    expect((await client.ping(nonce: 9)).nonce.toInt(), 9,
        reason: 'the original daemon must still be serving');
    await client.close();
    await live.terminateAndCollectStats();
  });

  test('client reconnects after a daemon restart', () async {
    var daemon = await SpikeDaemon.start(socketPath: socketPath);
    final client = UdsSpikeClient(socketPath: socketPath)..connect();
    expect((await client.ping(nonce: 1)).nonce.toInt(), 1);

    await daemon.terminateAndCollectStats();

    // While the daemon is down the call must fail with a transport error the UI
    // can tell apart from an application error.
    await expectLater(client.ping(nonce: 2), throwsA(isA<grpc.GrpcError>()));

    daemon = await SpikeDaemon.start(socketPath: socketPath);
    final after = await _retry(() => client.ping(nonce: 2),
        within: const Duration(seconds: 10));
    expect(after.nonce.toInt(), 2,
        reason: 'gRPC must transparently reconnect to the restarted daemon');
    await client.close();
    await daemon.terminateAndCollectStats();
  });

  test('binding under XDG_RUNTIME_DIR works (the production path)', () async {
    final runtime = SpikeSockets.runtimeDir();
    final path = '${runtime.path}/engine.sock';
    if (File(path).existsSync()) File(path).deleteSync();

    final daemon = await SpikeDaemon.start(
      socketPath: path,
      xdgRuntimeDir: runtime.path,
    );
    expect(daemon.readyLine['socket'], path);
    expect(File(path).statSync().mode & 0x1FF, 0x180);
    final client = UdsSpikeClient(socketPath: path)..connect();
    expect((await client.ping(nonce: 11)).nonce.toInt(), 11);
    await client.close();
    await daemon.terminateAndCollectStats();
    expect(File(path).existsSync(), isFalse);
    runtime.deleteSync(recursive: true);
  });

  test('no fd/RSS leak across repeated connect+ping+close cycles', () async {
    final daemon = await SpikeDaemon.start(socketPath: socketPath);

    // Warm up so one-off allocations are not mistaken for a leak.
    for (var i = 0; i < 3; i++) {
      final c = UdsSpikeClient(socketPath: socketPath)..connect();
      await c.ping(nonce: i);
      await c.close();
    }
    final fdsBefore = daemon.openFdCount();
    final rssBefore = daemon.rssKb();

    for (var i = 0; i < 25; i++) {
      final client = UdsSpikeClient(socketPath: socketPath)..connect();
      expect((await client.ping(nonce: i)).nonce.toInt(), i);
      await client.close();
    }
    // Give the daemon a moment to reap the closed connections.
    await Future<void>.delayed(const Duration(milliseconds: 500));

    final fdsAfter = daemon.openFdCount();
    final rssAfter = daemon.rssKb();
    stdout.writeln('daemon fds $fdsBefore -> $fdsAfter, '
        'rss ${rssBefore}KiB -> ${rssAfter}KiB');

    expect(fdsAfter, lessThanOrEqualTo(fdsBefore + 2),
        reason: '25 connect/close cycles must not accumulate descriptors');
    expect(rssAfter, lessThan(rssBefore + 8192),
        reason: 'no unbounded growth across reconnects (8 MiB tolerance)');

    final stats = await daemon.terminateAndCollectStats();
    expect(int.parse(stats['pings']!), 28);
    expect(int.parse(stats['max_streams']!), 0,
        reason: 'no streaming RPC is opened in this test');
  });
}

/// Retries [attempt] until it succeeds or [within] elapses.
Future<T> _retry<T>(
  Future<T> Function() attempt, {
  required Duration within,
}) async {
  final deadline = DateTime.now().add(within);
  Object? lastError;
  while (DateTime.now().isBefore(deadline)) {
    try {
      return await attempt();
    } catch (e) {
      lastError = e;
      await Future<void>.delayed(const Duration(milliseconds: 100));
    }
  }
  throw StateError('retry window $within expired: $lastError');
}