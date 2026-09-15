// Spike 01 harness — daemon lifecycle driver (EXPERIMENTAL, throwaway).
//
// Spawns/kills the Go daemon the way the Flutter app (and systemd) would, and
// exposes /proc introspection so the tests can make leak/lifecycle claims with
// evidence instead of assertions about intent.

import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'paths.dart';

/// A running `spiked` process.
class SpikeDaemon {
  SpikeDaemon._(this.process, this.socketPath, this._stdout, this._stderr,
      this.readyLine);

  final Process process;
  final String socketPath;
  final List<String> _stdout;
  final List<String> _stderr;

  /// The parsed `READY ...` line the daemon prints once it is listening.
  final Map<String, String> readyLine;

  int get pid => process.pid;

  bool get isRunning => _running;
  bool _running = true;

  List<String> get stdoutLines => List.unmodifiable(_stdout);
  List<String> get stderrLines => List.unmodifiable(_stderr);

  /// Socket mode as reported by the daemon itself, e.g. "0600".
  String? get socketMode => readyLine['mode'];

  /// "required" or "none".
  String? get tokenState => readyLine['token'];

  int? get expectUid => int.tryParse(readyLine['expect_uid'] ?? '');

  bool get socketExists => File(socketPath).existsSync();

  /// Starts the daemon and waits for its READY line.
  ///
  /// [xdgRuntimeDir] overrides XDG_RUNTIME_DIR so a test can prove the
  /// production socket path works without touching the real daemon's socket.
  static Future<SpikeDaemon> start({
    required String socketPath,
    String? token,
    int? expectUid,
    File? binary,
    String? xdgRuntimeDir,
    Duration timeout = const Duration(seconds: 15),
  }) async {
    final bin = binary ?? SpikePaths.requireDaemonBinary();
    final args = <String>['--socket', socketPath];
    if (token != null) args..add('--token')..add(token);
    if (expectUid != null) args..add('--expect-uid')..add('$expectUid');

    final process = await Process.start(
      bin.path,
      args,
      environment: xdgRuntimeDir == null
          ? null
          : <String, String>{'XDG_RUNTIME_DIR': xdgRuntimeDir},
      includeParentEnvironment: true,
    );

    final stdoutLines = <String>[];
    final stderrLines = <String>[];
    final readyCompleter = Completer<Map<String, String>>();
    final daemon =
        SpikeDaemon._(process, socketPath, stdoutLines, stderrLines, const {});

    process.stdout
        .transform(utf8.decoder)
        .transform(const LineSplitter())
        .listen((line) {
      stdoutLines.add(line);
      if (!readyCompleter.isCompleted && line.startsWith('READY ')) {
        readyCompleter.complete(_parseReady(line));
      }
    });
    process.stderr
        .transform(utf8.decoder)
        .transform(const LineSplitter())
        .listen(stderrLines.add);
    unawaited(process.exitCode.then((_) => daemon._running = false));

    final ready = await readyCompleter.future.timeout(
      timeout,
      onTimeout: () => throw StateError(
        'daemon did not become READY within $timeout\n'
        'stdout: ${stdoutLines.join('\n')}\nstderr: ${stderrLines.join('\n')}',
      ),
    );

    return SpikeDaemon._(process, socketPath, stdoutLines, stderrLines, ready);
  }

  /// Parses both `READY k=v k=v` and `STATS k=v k=v` lines.
  static Map<String, String> _parseReady(String line) {
    final out = <String, String>{};
    for (final token in line.split(' ')) {
      final idx = token.indexOf('=');
      if (idx > 0) out[token.substring(0, idx)] = token.substring(idx + 1);
    }
    return out;
  }

  /// Waits for the `STATS ...` line the daemon prints on shutdown.
  Future<Map<String, String>> waitForStats({
    Duration timeout = const Duration(seconds: 10),
  }) async {
    final deadline = DateTime.now().add(timeout);
    while (DateTime.now().isBefore(deadline)) {
      for (final line in _stdout) {
        if (line.startsWith('STATS ')) return _parseReady(line);
      }
      await Future<void>.delayed(const Duration(milliseconds: 20));
    }
    throw StateError('no STATS line within $timeout; stdout: $_stdout');
  }

  /// Graceful stop (SIGTERM) — how systemd stops a Type=simple user service.
  /// Returns the daemon's own STATS counters, which also proves the shutdown
  /// path completed rather than the process being reaped.
  Future<Map<String, String>> terminateAndCollectStats({
    Duration timeout = const Duration(seconds: 10),
  }) async {
    final statsFuture = waitForStats(timeout: timeout);
    process.kill(ProcessSignal.sigterm);
    final exit = await process.exitCode.timeout(
      timeout,
      onTimeout: () => throw StateError('daemon did not exit after SIGTERM'),
    );
    // Set the flag explicitly: the exitCode listener below is unawaited and may
    // not have run its microtask yet when the caller inspects isRunning.
    _running = false;
    if (exit != 0) {
      throw StateError('daemon exited $exit after SIGTERM; stderr: $_stderr');
    }
    return statsFuture;
  }

  /// Crash simulation (SIGKILL): leaves a stale socket file behind on purpose.
  Future<int> kill() async {
    process.kill(ProcessSignal.sigkill);
    final exit = await process.exitCode.timeout(const Duration(seconds: 5));
    _running = false;
    return exit;
  }

  /// Open file descriptors of the daemon, read from /proc.
  /// Returns -1 once the process is gone.
  int openFdCount() {
    if (!Directory('/proc/$pid/fd').existsSync()) return -1;
    return Directory('/proc/$pid/fd').listSync().length;
  }

  /// Resident set size in KiB from /proc/<pid>/status (rough leak signal).
  int rssKb() {
    final status = File('/proc/$pid/status');
    if (!status.existsSync()) return -1;
    for (final line in status.readAsLinesSync()) {
      if (line.startsWith('VmRSS:')) {
        final parts = line.split(RegExp(r'\s+'));
        return int.tryParse(parts[1]) ?? -1;
      }
    }
    return -1;
  }
}

/// Socket locations used by the lifecycle tests.
class SpikeSockets {
  const SpikeSockets._();

  /// A private temp directory (isolated; safe to abandon on failure).
  static Directory tempDir() =>
      Directory.systemTemp.createTempSync('pb-spike01-');

  /// The production location, under a spike-specific sub-directory so a real
  /// `phonebridge/engine.sock` is never touched.
  static Directory runtimeDir() {
    final base =
        Platform.environment['XDG_RUNTIME_DIR'] ?? Directory.systemTemp.path;
    return Directory('$base/phonebridge-spike01-test')
      ..createSync(recursive: true);
  }
}