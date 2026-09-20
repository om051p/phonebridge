class DiscoveredDevice {
  final String id;
  final String name;
  final String model;
  final String version;
  final String host;
  final int port;

  const DiscoveredDevice({
    required this.id,
    required this.name,
    required this.model,
    required this.version,
    required this.host,
    required this.port,
  });

  String get shortId {
    if (id.length <= 12) return id;
    return '${id.substring(0, 6)}...${id.substring(id.length - 6)}';
  }
}

class PairingResult {
  final String deviceName;
  final String sasCode;

  const PairingResult({
    required this.deviceName,
    required this.sasCode,
  });
}
