import 'device_presence.dart';
import 'discovered_device.dart';
import 'trusted_device.dart';

/// Canonical logical device (Phase F): exactly one row per deviceId,
/// merging the trust store, mDNS discovery, and the authoritative session
/// target. The underlying stores stay separate; this is the
/// controller/UI-boundary presentation model so the same ID is never shown
/// twice because it exists in both discovery and trust.
class DeviceRow {
  const DeviceRow({
    required this.deviceId,
    required this.displayName,
    required this.platform,
    this.model = '',
    this.trust = DeviceTrust.untrusted,
    this.discovered = false,
    this.isStale = false,
    this.isActiveTarget = false,
    this.signalingUrl = '',
    this.host = '',
    this.port = 0,
    this.pairedAtMs = 0,
    this.lastSeenMs = 0,
  });

  final String deviceId;
  final String displayName;
  final String platform;
  final String model;
  final DeviceTrust trust;
  final bool discovered;
  final bool isStale;
  final bool isActiveTarget;
  final String signalingUrl;
  final String host;
  final int port;
  final int pairedAtMs;
  final int lastSeenMs;

  bool get revoked => trust == DeviceTrust.revoked;
  bool get trusted => trust == DeviceTrust.trusted;

  /// Connect is only meaningful for trusted, discovered, non-active rows.
  bool get canConnect => trusted && discovered && !isStale && !isActiveTarget;

  /// Human grouping for the Devices screen sections.
  DeviceRowGroup get group {
    if (isActiveTarget) return DeviceRowGroup.connected;
    if (revoked) return DeviceRowGroup.revoked;
    if (!trusted) return DeviceRowGroup.untrusted;
    if (!discovered || isStale) return DeviceRowGroup.unavailable;
    return DeviceRowGroup.available;
  }

  String get shortId {
    if (deviceId.length <= 12) return deviceId;
    return '${deviceId.substring(0, 6)}...${deviceId.substring(deviceId.length - 6)}';
  }

  /// Disambiguation badge for when multiple devices share the same display name.
  String disambiguationTag(List<DeviceRow> allRows) {
    final sameNameCount = allRows
        .where((r) => r.displayName.toLowerCase().trim() == displayName.toLowerCase().trim())
        .length;
    if (sameNameCount <= 1) return '';
    if (isActiveTarget) return 'Connected';
    if (discovered && !isStale) return 'Active on LAN';
    if (revoked) return 'Revoked';
    if (group == DeviceRowGroup.unavailable) return 'Offline';
    if (group == DeviceRowGroup.untrusted) return 'Unpaired';
    return '';
  }
}

enum DeviceRowGroup { connected, available, untrusted, unavailable, revoked }

/// Merges trust + discovery + session target into one row per deviceId.
///
/// [activeDeviceId] is the authoritative session peer (or null when idle):
/// only the matching row reports [DeviceRow.isActiveTarget]. Revoked trust
/// records are preserved (security/audit history) but grouped separately —
/// never as connectable trusted rows.
List<DeviceRow> mergeDeviceRows({
  required List<TrustedDevice> trusted,
  required List<DiscoveredDevice> discovered,
  String? activeDeviceId,
}) {
  final byId = <String, _RowBuilder>{};

  for (final t in trusted) {
    if (t.deviceId.isEmpty) continue;
    final b = byId.putIfAbsent(t.deviceId, () => _RowBuilder(t.deviceId));
    b.displayName = t.displayName;
    b.platform = t.platform;
    b.trust = t.revoked ? DeviceTrust.revoked : DeviceTrust.trusted;
    b.pairedAtMs = t.pairedAtMs;
    b.lastSeenMs = t.lastSeenMs;
  }

  for (final d in discovered) {
    if (d.id.isEmpty) continue;
    final b = byId.putIfAbsent(d.id, () => _RowBuilder(d.id));
    // Discovery never overwrites the trusted display name: trust is the
    // pairing-time identity, discovery is the current advertisement.
    if (b.displayName.isEmpty) b.displayName = d.name;
    b.model = d.model;
    b.discovered = true;
    b.isStale = d.isStale;
    b.host = d.host;
    b.port = d.port;
    b.signalingUrl = d.signalingUrl;
  }

  final rows = <DeviceRow>[];
  for (final b in byId.values) {
    rows.add(DeviceRow(
      deviceId: b.id,
      displayName: b.displayName.isEmpty ? b.id : b.displayName,
      platform: b.platform,
      model: b.model,
      trust: b.trust,
      discovered: b.discovered,
      isStale: b.isStale,
      isActiveTarget: activeDeviceId != null &&
          activeDeviceId.isNotEmpty &&
          b.id == activeDeviceId,
      signalingUrl: b.signalingUrl,
      host: b.host,
      port: b.port,
      pairedAtMs: b.pairedAtMs,
      lastSeenMs: b.lastSeenMs,
    ));
  }

  int groupOrder(DeviceRow r) {
    switch (r.group) {
      case DeviceRowGroup.connected:
        return 0;
      case DeviceRowGroup.available:
        return 1;
      case DeviceRowGroup.untrusted:
        return 2;
      case DeviceRowGroup.unavailable:
        return 3;
      case DeviceRowGroup.revoked:
        return 4;
    }
  }

  rows.sort((a, b) {
    final g = groupOrder(a).compareTo(groupOrder(b));
    if (g != 0) return g;
    return a.displayName.toLowerCase().compareTo(b.displayName.toLowerCase());
  });
  return rows;
}

class _RowBuilder {
  _RowBuilder(this.id);

  final String id;
  String displayName = '';
  String platform = '';
  String model = '';
  DeviceTrust trust = DeviceTrust.untrusted;
  bool discovered = false;
  bool isStale = false;
  String signalingUrl = '';
  String host = '';
  int port = 0;
  int pairedAtMs = 0;
  int lastSeenMs = 0;
}
