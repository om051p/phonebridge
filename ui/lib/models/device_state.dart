class DeviceState {
  final String model;
  final String manufacturer;
  final int sdkInt;
  final bool isCapturing;
  final bool goEngineLoaded;
  final String codec;
  final bool isHardwareCodec;
  final String deviceId;
  final String displayName;
  final String clipboardState;
  final bool imeSelected;

  const DeviceState({
    required this.model,
    required this.manufacturer,
    required this.sdkInt,
    required this.isCapturing,
    required this.goEngineLoaded,
    required this.codec,
    required this.isHardwareCodec,
    this.deviceId = 'unknown',
    this.displayName = 'PhoneBridge Device',
    this.clipboardState = 'STOPPED',
    this.imeSelected = false,
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
      deviceId: map['deviceId'] as String? ?? 'unknown',
      displayName: map['displayName'] as String? ?? 'PhoneBridge Device',
      clipboardState: map['clipboardState'] as String? ?? 'STOPPED',
      imeSelected: map['imeSelected'] as bool? ?? false,
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
    deviceId: 'unknown',
    displayName: 'PhoneBridge Device',
    clipboardState: 'STOPPED',
    imeSelected: false,
  );
}
