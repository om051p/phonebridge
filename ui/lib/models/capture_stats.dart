class CaptureStats {
  final bool isCapturing;
  final int encodedFrames;
  final int keyframes;
  final int admittedFrames;
  final int droppedFrames;
  final String codec;
  final bool isHardwareCodec;
  final int durationUs;
  final String? lastError;
  final int timestampMs;
  final double? currentFps;

  const CaptureStats({
    required this.isCapturing,
    required this.encodedFrames,
    required this.keyframes,
    required this.admittedFrames,
    required this.droppedFrames,
    required this.codec,
    required this.isHardwareCodec,
    required this.durationUs,
    this.lastError,
    required this.timestampMs,
    this.currentFps,
  });

  factory CaptureStats.fromMap(
    Map<dynamic, dynamic> map, {
    CaptureStats? previous,
  }) {
    // Every event map rides this factory — including signal-only
    // {"trustChanged": true} / {"pairingChanged": true} and transfer-only
    // {"transfer": ...} maps. A key the event omits is "not reported", never
    // a reset: inherit the previous snapshot so a signal map cannot flip
    // isSharing, zero the counters, or fake a stop/start transition.
    // lastError keeps absent-means-clear: the native tick omits it once the
    // condition is gone, which is how errors clear.
    final isCapturing =
        map['isCapturing'] as bool? ?? previous?.isCapturing ?? false;
    final encoded =
        (map['encodedFrames'] as num?)?.toInt() ?? previous?.encodedFrames ?? 0;
    final keys =
        (map['keyframes'] as num?)?.toInt() ?? previous?.keyframes ?? 0;
    final admitted =
        (map['admittedFrames'] as num?)?.toInt() ?? previous?.admittedFrames ?? 0;
    final dropped =
        (map['droppedFrames'] as num?)?.toInt() ?? previous?.droppedFrames ?? 0;
    final codec = map['codec'] as String? ?? previous?.codec ?? 'None';
    final isHardware =
        map['isHardwareCodec'] as bool? ?? previous?.isHardwareCodec ?? false;
    final durationUs =
        (map['durationUs'] as num?)?.toInt() ?? previous?.durationUs ?? 0;
    final lastError = map['lastError'] as String?;
    final timestampMs =
        (map['timestampMs'] as num?)?.toInt() ?? previous?.timestampMs ?? 0;

    double? fps;
    if (previous != null && timestampMs > previous.timestampMs && previous.isCapturing && isCapturing) {
      final dtSec = (timestampMs - previous.timestampMs) / 1000.0;
      if (dtSec > 0.1) {
        final dFrames = encoded - previous.encodedFrames;
        fps = (dFrames / dtSec).clamp(0.0, 240.0);
      }
    }

    return CaptureStats(
      isCapturing: isCapturing,
      encodedFrames: encoded,
      keyframes: keys,
      admittedFrames: admitted,
      droppedFrames: dropped,
      codec: codec,
      isHardwareCodec: isHardware,
      durationUs: durationUs,
      lastError: lastError,
      timestampMs: timestampMs,
      // A signal map carries no new frame count: keep the last measured fps
      // instead of blanking it.
      currentFps: fps ?? previous?.currentFps,
    );
  }

  CaptureStats copyWith({
    bool? isCapturing,
    int? encodedFrames,
    int? keyframes,
    int? admittedFrames,
    int? droppedFrames,
    String? codec,
    bool? isHardwareCodec,
    int? durationUs,
    String? lastError,
    int? timestampMs,
    double? currentFps,
  }) {
    return CaptureStats(
      isCapturing: isCapturing ?? this.isCapturing,
      encodedFrames: encodedFrames ?? this.encodedFrames,
      keyframes: keyframes ?? this.keyframes,
      admittedFrames: admittedFrames ?? this.admittedFrames,
      droppedFrames: droppedFrames ?? this.droppedFrames,
      codec: codec ?? this.codec,
      isHardwareCodec: isHardwareCodec ?? this.isHardwareCodec,
      durationUs: durationUs ?? this.durationUs,
      lastError: lastError ?? this.lastError,
      timestampMs: timestampMs ?? this.timestampMs,
      currentFps: currentFps ?? this.currentFps,
    );
  }

  static const initial = CaptureStats(
    isCapturing: false,
    encodedFrames: 0,
    keyframes: 0,
    admittedFrames: 0,
    droppedFrames: 0,
    codec: 'None',
    isHardwareCodec: false,
    durationUs: 0,
    timestampMs: 0,
  );
}
