class DeviceState {
  final String model;
  final String manufacturer;
  final int sdkInt;
  final bool isCapturing;
  final bool goEngineLoaded;
  final String codec;
  final bool isHardwareCodec;

  const DeviceState({
    required this.model,
    required this.manufacturer,
    required this.sdkInt,
    required this.isCapturing,
    required this.goEngineLoaded,
    required this.codec,
    required this.isHardwareCodec,
  });

  factory DeviceState.fromMap(Map<dynamic, dynamic> map) {
    return DeviceState(
      model: map['model'] as String? ?? 'Unknown',
      manufacturer: map['manufacturer'] as String? ?? 'Unknown',
      sdkInt: map['sdkInt'] as int? ?? 0,
      isCapturing: map['isCapturing'] as bool? ?? false,
      goEngineLoaded: map['goEngineLoaded'] as bool? ?? false,
      codec: map['codec'] as String? ?? 'None',
      isHardwareCodec: map['isHardwareCodec'] as bool? ?? false,
    );
  }

  static const initial = DeviceState(
    model: 'Unknown',
    manufacturer: 'Unknown',
    sdkInt: 0,
    isCapturing: false,
    goEngineLoaded: false,
    codec: 'None',
    isHardwareCodec: false,
  );
}
