enum ClipboardSyncState {
  stopped,
  writeOnlyDormant,
  ambientActive,
  needsSetup,
  restricted,
  unavailable;

  static ClipboardSyncState fromString(String? val) {
    switch (val?.toUpperCase()) {
      case 'AMBIENT_ACTIVE':
      case 'READY':
        // Linux Wayland helper bound and passively monitoring (Go
        // AdapterStatusReady) and Android ambient IME-bound state share one
        // canonical meaning: background sync is actually flowing.
        return ClipboardSyncState.ambientActive;
      case 'WRITE_ONLY_DORMANT':
        return ClipboardSyncState.writeOnlyDormant;
      case 'COSMIC_FLAG_REQUIRED':
      case 'NO_DATA_CONTROL':
      case 'NO_BACKEND':
      case 'WAYLAND_UNAVAILABLE':
      case 'COMPOSITOR_DISCONNECTED':
      case 'STARTING':
        // The daemon is telling us setup is missing, not that sync is
        // generically inactive. Collapsing these into STOPPED hid the fix.
        return ClipboardSyncState.needsSetup;
      case 'RESTRICTED':
      case 'PERMISSION_REQUIRED':
        return ClipboardSyncState.restricted;
      case 'UNAVAILABLE':
      case 'CRASHED':
        return ClipboardSyncState.unavailable;
      case 'STOPPED':
      default:
        return ClipboardSyncState.stopped;
    }
  }

  String get name {
    switch (this) {
      case ClipboardSyncState.ambientActive:
        return 'AMBIENT_ACTIVE';
      case ClipboardSyncState.writeOnlyDormant:
        return 'WRITE_ONLY_DORMANT';
      case ClipboardSyncState.needsSetup:
        return 'NEEDS_SETUP';
      case ClipboardSyncState.restricted:
        return 'RESTRICTED';
      case ClipboardSyncState.unavailable:
        return 'UNAVAILABLE';
      case ClipboardSyncState.stopped:
        return 'STOPPED';
    }
  }

  String get label {
    switch (this) {
      case ClipboardSyncState.ambientActive:
        return 'Ambient Sync Active';
      case ClipboardSyncState.writeOnlyDormant:
        return 'Dormant (Manual/Tile Ready)';
      case ClipboardSyncState.needsSetup:
        return 'Setup Required';
      case ClipboardSyncState.restricted:
        return 'Restricted by System';
      case ClipboardSyncState.unavailable:
        return 'Permission Required';
      case ClipboardSyncState.stopped:
        return 'Service Inactive';
    }
  }

  String get description {
    switch (this) {
      case ClipboardSyncState.ambientActive:
        return 'Background clipboard monitoring is active. Copies are detected automatically and forwarded to the paired device.';
      case ClipboardSyncState.writeOnlyDormant:
        return 'Remote PC copies apply automatically. Use the Quick Settings Tile or the Sync Now button to push phone clips.';
      case ClipboardSyncState.needsSetup:
        return 'Clipboard monitoring needs setup on this device (Wayland data-control or companion keyboard). Inbound copies still apply; outbound needs the setup step below.';
      case ClipboardSyncState.restricted:
        return 'The system is restricting clipboard access. Open the setup step below to grant access.';
      case ClipboardSyncState.unavailable:
        return 'Clipboard service cannot access system clipboard. Check system permissions.';
      case ClipboardSyncState.stopped:
        return 'PhoneBridge background service is not running.';
    }
  }
}

class ClipboardStatus {
  final ClipboardSyncState state;
  final bool imeSelected;
  final int maxPayloadSize;
  final bool isEnabled;
  final int? lastSyncTimestampMs;
  final int syncCount;
  final int lastSyncBytes;
  final String? lastDirection; // "inbound" (PC->Phone) or "outbound" (Phone->PC)

  const ClipboardStatus({
    required this.state,
    required this.imeSelected,
    required this.maxPayloadSize,
    this.isEnabled = true,
    this.lastSyncTimestampMs,
    this.syncCount = 0,
    this.lastSyncBytes = 0,
    this.lastDirection,
  });

  factory ClipboardStatus.fromMap(Map<dynamic, dynamic> map, {ClipboardStatus? previous}) {
    final rawState = map['state'] as String? ?? map['clipboardState'] as String?;
    final st = ClipboardSyncState.fromString(rawState);
    final ime = map['imeSelected'] as bool? ?? false;
    final maxPayload = (map['maxPayloadSize'] as num?)?.toInt() ?? 786432;
    // The native adapter owns the enable flag; `previous` is only a fallback for
    // a payload that does not carry it (e.g. the Linux daemon's status map).
    final enabled = map['enabled'] as bool? ?? previous?.isEnabled ?? true;

    return ClipboardStatus(
      state: st,
      imeSelected: ime,
      maxPayloadSize: maxPayload,
      isEnabled: enabled,
      lastSyncTimestampMs: previous?.lastSyncTimestampMs,
      syncCount: previous?.syncCount ?? 0,
      lastSyncBytes: previous?.lastSyncBytes ?? 0,
      lastDirection: previous?.lastDirection,
    );
  }

  ClipboardStatus copyWith({
    ClipboardSyncState? state,
    bool? imeSelected,
    int? maxPayloadSize,
    bool? isEnabled,
    int? lastSyncTimestampMs,
    int? syncCount,
    int? lastSyncBytes,
    String? lastDirection,
  }) {
    return ClipboardStatus(
      state: state ?? this.state,
      imeSelected: imeSelected ?? this.imeSelected,
      maxPayloadSize: maxPayloadSize ?? this.maxPayloadSize,
      isEnabled: isEnabled ?? this.isEnabled,
      lastSyncTimestampMs: lastSyncTimestampMs ?? this.lastSyncTimestampMs,
      syncCount: syncCount ?? this.syncCount,
      lastSyncBytes: lastSyncBytes ?? this.lastSyncBytes,
      lastDirection: lastDirection ?? this.lastDirection,
    );
  }

  static const initial = ClipboardStatus(
    state: ClipboardSyncState.stopped,
    imeSelected: false,
    maxPayloadSize: 786432,
    isEnabled: true,
  );
}
