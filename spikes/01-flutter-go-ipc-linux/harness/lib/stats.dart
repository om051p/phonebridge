// Spike 01 harness — measurement plumbing (EXPERIMENTAL, throwaway).

import 'dart:convert';
import 'dart:io';

/// Latency samples in microseconds plus the percentiles the spike reports.
///
/// Percentiles use the nearest-rank method on the sorted sample set (no
/// interpolation), which is the conservative choice for tail reporting.
class LatencyStats {
  LatencyStats._(this._sorted);

  factory LatencyStats.fromMicros(Iterable<int> micros) {
    final sorted = micros.toList()..sort();
    return LatencyStats._(sorted);
  }

  final List<int> _sorted;

  int get count => _sorted.length;
  bool get isEmpty => _sorted.isEmpty;

  int get min => _sorted.first;
  int get max => _sorted.last;

  double get mean =>
      _sorted.fold<int>(0, (a, b) => a + b) / _sorted.length;

  /// Nearest-rank percentile in `[0, 100]`.
  double percentile(double p) {
    if (_sorted.isEmpty) return 0;
    final rank = (p / 100) * _sorted.length;
    var idx = rank.ceil() - 1;
    if (idx < 0) idx = 0;
    if (idx >= _sorted.length) idx = _sorted.length - 1;
    return _sorted[idx].toDouble();
  }

  double get p50 => percentile(50);
  double get p90 => percentile(90);
  double get p95 => percentile(95);
  double get p99 => percentile(99);

  Map<String, Object?> toJson() => {
        'count': count,
        'min_us': min,
        'mean_us': double.parse(mean.toStringAsFixed(2)),
        'p50_us': p50,
        'p90_us': p90,
        'p95_us': p95,
        'p99_us': p99,
        'max_us': max,
      };

  String get summary => isEmpty
      ? 'no samples'
      : 'n=$count p50=${p50.toStringAsFixed(1)}us '
          'p90=${p90.toStringAsFixed(1)}us p95=${p95.toStringAsFixed(1)}us '
          'p99=${p99.toStringAsFixed(1)}us max=${max}us mean=${mean.toStringAsFixed(1)}us';
}

/// Accumulates measurement sections and writes them as JSON evidence.
class SpikeReport {
  SpikeReport(this.name);

  final String name;
  final Map<String, Object?> _data = {};
  final StringBuffer _log = StringBuffer();

  Map<String, Object?> get data => Map.unmodifiable(_data);

  void record(String key, Object? value) => _data[key] = value;

  void section(String key, Map<String, Object?> value) => _data[key] = value;

  /// Records a latency block, optionally annotated with context (iterations,
  /// payload size, concurrency) so the numbers stay interpretable later.
  void latency(
    String key,
    LatencyStats stats, {
    Map<String, Object?> context = const {},
  }) {
    _data[key] = {...stats.toJson(), ...context};
    _log.writeln('  $key: ${stats.summary}');
    context.forEach((k, v) => _log.writeln('      $k: $v'));
  }

  void note(String line) => _log.writeln(line);

  /// Writes `results/<name>.json` and prints the human summary; returns the file.
  Future<File> write() async {
    final dir = Directory(SpikePathsResults.dir);
    if (!dir.existsSync()) dir.createSync(recursive: true);
    final file = File('${dir.path}/$name.json');
    _data['spike'] = '01-flutter-go-ipc-linux';
    _data['recorded_at_utc'] = DateTime.now().toUtc().toIso8601String();
    await file.writeAsString(
      const JsonEncoder.withIndent('  ').convert(_data),
    );
    stdout.writeln('── $name ──────────────────────────────');
    stdout.write(_log);
    stdout.writeln('  → ${file.path}');
    return file;
  }
}

/// Tiny indirection so this file does not import paths.dart (keeps the
/// measurement helpers usable from a plain `dart run` context too).
class SpikePathsResults {
  static String get dir {
    final override = Platform.environment['SPIKE_RESULTS_DIR'];
    if (override != null && override.isNotEmpty) return override;
    return '${Directory.current.path}/../results';
  }
}
