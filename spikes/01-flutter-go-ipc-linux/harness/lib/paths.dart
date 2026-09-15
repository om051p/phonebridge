// Spike 01 harness — artifact resolution (EXPERIMENTAL, throwaway).
//
// `flutter test` runs with CWD = package root (this harness/ directory), so the
// Go build outputs are reachable relatively. Environment overrides exist for CI
// or for running the same harness from a different working directory.

import 'dart:io';

/// Paths to the Go artifacts the harness drives.
class SpikePaths {
  const SpikePaths._();

  static String _override(String name, String fallback) {
    final v = Platform.environment[name];
    if (v == null || v.isEmpty) return fallback;
    return v;
  }

  /// Package root of this harness (CWD under `flutter test`).
  static String get packageDir => Directory.current.path;

  /// The UDS+gRPC daemon built by `make spike01-build`.
  static String get daemonBinary => _override(
        'SPIKE_DAEMON_BIN',
        '$packageDir/../go/bin/phonebridge-spike-daemon',
      );

  /// The c-shared library for the FFI branch.
  static String get ffiLibrary => _override(
        'SPIKE_FFI_LIB',
        '$packageDir/../go/bin/libphonebridge_spike.so',
      );

  /// Where measured results are written (JSON evidence).
  static String get resultsDir =>
      _override('SPIKE_RESULTS_DIR', '$packageDir/../results');

  /// Fails with an actionable message instead of a confusing socket error.
  static File require(String path, String label) {
    final f = File(path);
    if (!f.existsSync()) {
      throw StateError(
        '$label not found at $path\n'
        'Build the Go side first:  make -C <repo>/spikes/01-flutter-go-ipc-linux build\n'
        '(or set SPIKE_DAEMON_BIN / SPIKE_FFI_LIB to the built artifacts)',
      );
    }
    return f;
  }

  static File requireDaemonBinary() =>
      require(daemonBinary, 'spike daemon binary');

  static File requireFfiLibrary() => require(ffiLibrary, 'FFI shared library');
}