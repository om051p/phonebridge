// Canonical device/connection/service presentation semantics.
//
// These enums are PRESENTATION mappings over the existing owners — they do
// not replace them and introduce no second manager/model:
//
// - Trust: [TrustedDevice.revoked] + trust-store membership (Go
//   `crypto.TrustStore`, Kotlin `TrustStore`, daemon `ListTrustedDevices`).
// - Discovery: mDNS browse entries ([DiscoveredDevice], `isStale` from
//   Go `discovery.DeviceRegistry` / Android `NsdBrowser`).
// - Connection: Go `engine.SessionState` (8 phases) via [SessionStatus] and
//   composed for the whole app by [LinkStatus].
// - Feature/session: the same session state, viewed per-feature
//   (idle/starting/active/stopping/failed) for screen/clipboard.
// - Service status: Android `AdapterState` / Linux `AdapterStatus` via
//   [ClipboardSyncState], plus Android permission flags.
//
// The rule: Go owns networking/connection, Kotlin owns Android APIs, Flutter
// renders. This file only decides how existing states are NAMED and grouped
// so every surface agrees.

import '../generated/phonebridge/localipc/v1/local_ipc.pb.dart' as ipc;
import 'clipboard_status.dart';
import 'discovered_device.dart';
import 'link_status.dart';
import 'session_status.dart';
import 'trusted_device.dart';

/// Device trust — never a synonym for connection.
enum DeviceTrust { untrusted, trusted, revoked }

/// Discovery presence — whether the LAN currently sees the device.
enum DiscoveryPresence { notDiscovered, discovered }

/// Canonical connection phase for a single device/session view.
enum DeviceConnectionState {
  disconnected,
  connecting,
  connected,
  reconnecting,
  connectionError,
}

/// Per-feature session lifecycle (screen sharing, clipboard sync).
enum FeatureSessionState { idle, starting, active, stopping, failed }

/// Canonical service capability/status for clipboard, permissions, etc.
enum ServiceStatus {
  disabled,
  needsSetup,
  dormant,
  active,
  unavailable,
  restricted,
}

DeviceTrust trustOf({
  TrustedDevice? trusted,
  DiscoveredDevice? discovered,
}) {
  if (trusted != null) {
    return trusted.revoked ? DeviceTrust.revoked : DeviceTrust.trusted;
  }
  // A discovered row with no trust record is untrusted by definition —
  // pairing is required, never assumed.
  return DeviceTrust.untrusted;
}

DiscoveryPresence discoveryOf({
  List<DiscoveredDevice> discovered = const [],
  required String deviceId,
}) {
  final found = discovered.any((d) => d.id == deviceId && !d.isStale);
  return found ? DiscoveryPresence.discovered : DiscoveryPresence.notDiscovered;
}

DeviceConnectionState connectionStateOfSession(SessionStatus status) {
  switch (status.state) {
    case ipc.SessionState.SESSION_STATE_CONNECTING:
    case ipc.SessionState.SESSION_STATE_DISCOVERING:
      return DeviceConnectionState.connecting;
    case ipc.SessionState.SESSION_STATE_CONNECTED:
      return DeviceConnectionState.connected;
    case ipc.SessionState.SESSION_STATE_STREAMING:
      // Streaming implies connected transport with media flowing; the
      // per-device connection view reports it as connected.
      return DeviceConnectionState.connected;
    case ipc.SessionState.SESSION_STATE_RECONNECTING:
      return DeviceConnectionState.reconnecting;
    case ipc.SessionState.SESSION_STATE_FAILED:
      return DeviceConnectionState.connectionError;
    case ipc.SessionState.SESSION_STATE_STOPPED:
    case ipc.SessionState.SESSION_STATE_DISCONNECTED:
    default:
      return DeviceConnectionState.disconnected;
  }
}

DeviceConnectionState connectionStateOfLink(LinkStatus link) {
  switch (link.phase) {
    case LinkPhase.connecting:
      return DeviceConnectionState.connecting;
    case LinkPhase.connected:
    case LinkPhase.streaming:
      return DeviceConnectionState.connected;
    case LinkPhase.reconnecting:
      return DeviceConnectionState.reconnecting;
    case LinkPhase.failed:
      return DeviceConnectionState.connectionError;
    case LinkPhase.offline:
      return DeviceConnectionState.connectionError;
    case LinkPhase.disconnected:
      return DeviceConnectionState.disconnected;
  }
}

FeatureSessionState featureSessionOf(SessionStatus status) {
  switch (status.state) {
    case ipc.SessionState.SESSION_STATE_DISCOVERING:
    case ipc.SessionState.SESSION_STATE_CONNECTING:
      return FeatureSessionState.starting;
    case ipc.SessionState.SESSION_STATE_CONNECTED:
    case ipc.SessionState.SESSION_STATE_STREAMING:
    case ipc.SessionState.SESSION_STATE_RECONNECTING:
      return FeatureSessionState.active;
    case ipc.SessionState.SESSION_STATE_STOPPED:
      // STOPPED is terminal in the wire model; the stopping transition is
      // observed as the active session ending. Presentation reports idle once
      // stopped so Home and Screen agree ("Not running" / "Start Sharing").
      return FeatureSessionState.idle;
    case ipc.SessionState.SESSION_STATE_FAILED:
      return FeatureSessionState.failed;
    case ipc.SessionState.SESSION_STATE_DISCONNECTED:
    default:
      return FeatureSessionState.idle;
  }
}

/// Maps the clipboard subsystem to the canonical service vocabulary.
///
/// Android and Linux report DIFFERENT underlying realities through the same
/// field, so the mapping is explicit:
/// - Android `WRITE_ONLY_DORMANT`: service running, inbound writes work,
///   ambient reads need the companion IME or a manual pull (dormant).
/// - Linux `READY`/`AMBIENT_ACTIVE`: Wayland data-control helper is bound
///   and passively monitoring (active).
/// Daemon setup states (`COSMIC_FLAG_REQUIRED`, `NO_DATA_CONTROL`,
/// `NO_BACKEND`, `WAYLAND_UNAVAILABLE`) are needs-setup/unavailable — never
/// collapsed into a generic inactive.
ServiceStatus clipboardServiceStatusOf(ClipboardStatus status) {
  if (!status.isEnabled) return ServiceStatus.disabled;
  switch (status.state) {
    case ClipboardSyncState.ambientActive:
      return ServiceStatus.active;
    case ClipboardSyncState.writeOnlyDormant:
      return ServiceStatus.dormant;
    case ClipboardSyncState.needsSetup:
      return ServiceStatus.needsSetup;
    case ClipboardSyncState.restricted:
      return ServiceStatus.restricted;
    case ClipboardSyncState.unavailable:
      return ServiceStatus.unavailable;
    case ClipboardSyncState.stopped:
      return ServiceStatus.disabled;
  }
}

String describeTrust(DeviceTrust trust) {
  switch (trust) {
    case DeviceTrust.trusted:
      return 'Trusted';
    case DeviceTrust.revoked:
      return 'Revoked';
    case DeviceTrust.untrusted:
      return 'Unpaired';
  }
}

String describeConnection(DeviceConnectionState state) {
  switch (state) {
    case DeviceConnectionState.disconnected:
      return 'Disconnected';
    case DeviceConnectionState.connecting:
      return 'Connecting';
    case DeviceConnectionState.connected:
      return 'Connected';
    case DeviceConnectionState.reconnecting:
      return 'Reconnecting';
    case DeviceConnectionState.connectionError:
      return 'Connection error';
  }
}

/// Connection path for the current P2P-first product: LAN direct via mDNS +
/// WebRTC. Rendered as data, not hard-coded per device.
String connectionPathLabel({bool isStale = false}) {
  if (isStale) return 'LAN · Stale';
  return 'LAN · Direct';
}

/// Human-readable device identity line: name + platform, never an internal
/// id like "Target PC x1".
String deviceIdentityLine({required String displayName, required String platform}) {
  final plat = platform.trim().isEmpty ? 'Device' : platform.trim();
  return '$displayName · $plat';
}
