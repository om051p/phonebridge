enum ClipboardSyncState {
  stopped,
  writeOnlyDormant,
  ambientActive,
  unavailable;

  static ClipboardSyncState fromString(String? val) {
    switch (val?.toUpperCase()) {
      case 'AMBIENT_ACTIVE':
        return ClipboardSyncState.ambientActive;
      case 'WRITE_ONLY_DORMANT':
        return ClipboardSyncState.writeOnlyDormant;
      case 'UNAVAILABLE':
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
      case ClipboardSyncState.unavailable:
        return 'Permission Required';
      case ClipboardSyncState.stopped:
        return 'Service Inactive';
    }
  }

  String get description {
    switch (this) {
      case ClipboardSyncState.ambientActive:
        return 'PhoneBridge Companion IME is bound. Background clipboard copying and receiving are fully active.';
      case ClipboardSyncState.writeOnlyDormant:
        return 'Remote PC copies apply automatically. Use the Quick Settings Tile or the Sync Now button to push phone clips.';
      case ClipboardSyncState.unavailable:
        return 'Clipboard service cannot access system clipboard. Check Android system permissions.';
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
