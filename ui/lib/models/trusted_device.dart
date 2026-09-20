class TrustedDevice {
  final String deviceId;
  final String displayName;
  final String platform;
  final int pairedAtMs;
  final int lastSeenMs;
  final bool revoked;

  const TrustedDevice({
    required this.deviceId,
    required this.displayName,
    required this.platform,
    required this.pairedAtMs,
    required this.lastSeenMs,
    required this.revoked,
  });

  factory TrustedDevice.fromMap(Map<dynamic, dynamic> map) {
    return TrustedDevice(
      deviceId: map['deviceId'] as String? ?? '',
      displayName: map['displayName'] as String? ?? 'Unnamed Device',
      platform: map['platform'] as String? ?? 'linux',
      pairedAtMs: (map['pairedAtMs'] as num?)?.toInt() ?? 0,
      lastSeenMs: (map['lastSeenMs'] as num?)?.toInt() ?? 0,
      revoked: map['revoked'] as bool? ?? false,
    );
  }

  bool get isLinux => platform.toLowerCase().contains('linux');
  bool get isCosmic => platform.toLowerCase().contains('cosmic');

  String get shortId {
    if (deviceId.length <= 12) return deviceId;
    return '${deviceId.substring(0, 6)}...${deviceId.substring(deviceId.length - 6)}';
  }
}
