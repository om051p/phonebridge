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
    final isCapturing = map['isCapturing'] as bool? ?? false;
    final encoded = (map['encodedFrames'] as num?)?.toInt() ?? 0;
    final keys = (map['keyframes'] as num?)?.toInt() ?? 0;
    final admitted = (map['admittedFrames'] as num?)?.toInt() ?? 0;
    final dropped = (map['droppedFrames'] as num?)?.toInt() ?? 0;
    final codec = map['codec'] as String? ?? 'None';
    final isHardware = map['isHardwareCodec'] as bool? ?? false;
    final durationUs = (map['durationUs'] as num?)?.toInt() ?? 0;
    final lastError = map['lastError'] as String?;
    final timestampMs = (map['timestampMs'] as num?)?.toInt() ?? 0;

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
      currentFps: fps,
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
