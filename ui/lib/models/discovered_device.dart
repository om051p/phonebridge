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
    this.isStale = false,
  });

  /// Builds a device from the Android host's map shape.
  ///
  /// The Android backend answers `getDiscoveredDevices` from the Go core's mDNS
  /// browse, so the fields arrive already resolved to a dialable `host` and
  /// `port` — the phone has no daemon to ask for an endpoint later, and it
  /// composes the signaling URL itself.
  factory DiscoveredDevice.fromMap(Map<dynamic, dynamic> map) {
    final id = (map['id'] as String?) ?? '';
    return DiscoveredDevice(
      id: id,
      name: ((map['name'] as String?)?.isNotEmpty ?? false) ? map['name'] as String : id,
      model: (map['model'] as String?) ?? '',
      version: (map['version'] as String?) ?? '',
      host: (map['host'] as String?) ?? '',
      port: (map['port'] as num?)?.toInt() ?? 0,
      isStale: (map['isStale'] as bool?) ?? false,
    );
  }

  /// The signaling base URL for this peer, or empty when it is not dialable.
  ///
  /// A device with no address is a browse entry that has aged out; the UI keeps
  /// it visible (so the user can see it was there) but must not offer a connect
  /// action that could only fail.
  String get signalingUrl {
    if (host.isEmpty || port <= 0) return '';
    return 'http://$host:$port';
  }

  /// The daemon reports a browse entry as stale when its mDNS record has
  /// aged out. Surfaced so the Devices tab can refuse to offer pairing on a
  /// result that may no longer exist on the LAN.
  final bool isStale;

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
