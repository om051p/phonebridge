// File-transfer UI model (DEC-024, Phase 4).
//
// A plain, Flutter-free wrapper around the generated
// phonebridge.localipc.v1.TransferInfo snapshot. Screens, widgets and the
// transfer controller therefore never touch generated protobuf types for
// formatting decisions, and the typed state/reason enums become readable UI
// text in exactly one place. This is a *mapping*, never a contract change: the
// proto files, engine and backends stay untouched.

import '../generated/phonebridge/localipc/v1/local_ipc.pb.dart' as ipc;

/// Text shown when the platform cannot send files yet (native channel not
/// wired). Kept here so services and UI agree on one wording.
const String kTransferUnavailableMessage =
    'File transfers are not available on this device yet';

/// One immutable transfer snapshot (in flight or from the recent history).
class TransferItem {
  const TransferItem({
    required this.transferId,
    this.direction = ipc.TransferDirection.TRANSFER_DIRECTION_UNSPECIFIED,
    this.state = ipc.TransferState.TRANSFER_STATE_UNSPECIFIED,
    this.peerDeviceId = '',
    this.filename = '',
    this.mimeType = '',
    this.sizeBytes = 0,
    this.bytesTransferred = 0,
    this.startedAtMs = 0,
    this.finishedAtMs = 0,
    this.reasonCode = ipc.TransferReason.TRANSFER_REASON_UNSPECIFIED,
    this.errorMessage = '',
    this.savedName = '',
  });

  /// Wraps the local-IPC snapshot returned by SendFile/ListTransfers and pushed
  /// on TransferEvent.
  factory TransferItem.fromProto(ipc.TransferInfo info) {
    return TransferItem(
      transferId: info.transferId,
      direction: info.direction,
      state: info.state,
      peerDeviceId: info.peerDeviceId,
      filename: info.filename,
      mimeType: info.mimeType,
      sizeBytes: info.sizeBytes.toInt(),
      bytesTransferred: info.bytesTransferred.toInt(),
      startedAtMs: info.startedAtMs.toInt(),
      finishedAtMs: info.finishedAtMs.toInt(),
      reasonCode: info.reasonCode,
      errorMessage: info.errorMessage,
      savedName: info.savedName,
    );
  }

  /// Builds an item from a platform-channel map (Android pass-through shape),
  /// where enums arrive either as ints or as their proto names.
  factory TransferItem.fromMap(Map<dynamic, dynamic> map) {
    return TransferItem(
      transferId: map['transferId']?.toString() ?? '',
      direction: _directionFrom(map['direction']),
      state: _stateFrom(map['state']),
      peerDeviceId: map['peerDeviceId']?.toString() ?? '',
      filename: map['filename']?.toString() ?? '',
      mimeType: map['mimeType']?.toString() ?? '',
      sizeBytes: (map['sizeBytes'] as num?)?.toInt() ?? 0,
      bytesTransferred: (map['bytesTransferred'] as num?)?.toInt() ?? 0,
      startedAtMs: (map['startedAtMs'] as num?)?.toInt() ?? 0,
      finishedAtMs: (map['finishedAtMs'] as num?)?.toInt() ?? 0,
      reasonCode: _reasonFrom(map['reasonCode']),
      errorMessage: map['errorMessage']?.toString() ?? '',
      savedName: map['savedName']?.toString() ?? '',
    );
  }

  final String transferId;
  final ipc.TransferDirection direction;
  final ipc.TransferState state;
  final String peerDeviceId;
  final String filename;
  final String mimeType;
  final int sizeBytes;
  final int bytesTransferred;
  final int startedAtMs;
  final int finishedAtMs;
  final ipc.TransferReason reasonCode;
  final String errorMessage;
  final String savedName;

  bool get isOutbound =>
      direction == ipc.TransferDirection.TRANSFER_DIRECTION_OUTBOUND;

  bool get isInbound =>
      direction == ipc.TransferDirection.TRANSFER_DIRECTION_INBOUND;

  /// In flight (PENDING/ACTIVE/VERIFYING).
  bool get isInFlight =>
      state == ipc.TransferState.TRANSFER_STATE_PENDING ||
      state == ipc.TransferState.TRANSFER_STATE_ACTIVE ||
      state == ipc.TransferState.TRANSFER_STATE_VERIFYING;

  /// Finished: COMPLETE, CANCELLED or FAILED.
  bool get isTerminal =>
      state == ipc.TransferState.TRANSFER_STATE_COMPLETE ||
      state == ipc.TransferState.TRANSFER_STATE_CANCELLED ||
      state == ipc.TransferState.TRANSFER_STATE_FAILED;

  bool get isComplete => state == ipc.TransferState.TRANSFER_STATE_COMPLETE;
  bool get isCancelled => state == ipc.TransferState.TRANSFER_STATE_CANCELLED;
  bool get isFailed => state == ipc.TransferState.TRANSFER_STATE_FAILED;
  bool get isVerifying => state == ipc.TransferState.TRANSFER_STATE_VERIFYING;

  /// Only PENDING and ACTIVE can be aborted locally: during VERIFYING the bytes
  /// are already staged on the receiver and a local cancel would race the
  /// promotion step (DEC-024).
  bool get isCancellable =>
      state == ipc.TransferState.TRANSFER_STATE_PENDING ||
      state == ipc.TransferState.TRANSFER_STATE_ACTIVE;

  /// Total size is unknown while an inbound offer has not been parsed; the UI
  /// must not show a progress bar in that case.
  bool get hasSize => sizeBytes > 0;

  /// bytesTransferred/sizeBytes, or null when the size is unknown.
  double? get progress {
    if (!hasSize) return null;
    final ratio = bytesTransferred / sizeBytes;
    if (ratio.isNaN) return null;
    return ratio.clamp(0.0, 1.0);
  }

  int get progressPercent {
    final value = progress;
    return value == null ? 0 : (value * 100).round();
  }

  String get displayName {
    if (filename.isNotEmpty) return filename;
    if (savedName.isNotEmpty) return savedName;
    return 'Unnamed file';
  }

  /// 'Saved as <name>' when the receiver renamed the file on collision.
  String? get savedAsLabel {
    if (savedName.isEmpty || savedName == displayName) return null;
    return 'Saved as $savedName';
  }

  String get directionLabel {
    if (isOutbound) return 'Sent';
    if (isInbound) return 'Received';
    return 'Transfer';
  }

  String get stateLabel {
    switch (state) {
      case ipc.TransferState.TRANSFER_STATE_PENDING:
        return 'Waiting for peer';
      case ipc.TransferState.TRANSFER_STATE_ACTIVE:
        return 'Transferring';
      case ipc.TransferState.TRANSFER_STATE_VERIFYING:
        return 'Verifying…';
      case ipc.TransferState.TRANSFER_STATE_COMPLETE:
        return 'Complete';
      case ipc.TransferState.TRANSFER_STATE_CANCELLED:
        return 'Cancelled';
      case ipc.TransferState.TRANSFER_STATE_FAILED:
        return 'Failed';
      default:
        return 'Unknown';
    }
  }

  /// Classified outcome as text, or null when the daemon reported a normal
  /// outcome (NONE / UNSPECIFIED). A FAILED transfer with no classification
  /// still reports that it failed rather than showing nothing.
  String? get reasonText {
    final typed = reasonTextFor(reasonCode);
    if (typed != null) return typed;
    if (isFailed) return 'Transfer failed';
    return null;
  }

  /// Human-readable text for one typed reason code, or null when there is
  /// nothing to report.
  static String? reasonTextFor(ipc.TransferReason reason) {
    switch (reason) {
      case ipc.TransferReason.TRANSFER_REASON_NO_SESSION:
        return 'No active session';
      case ipc.TransferReason.TRANSFER_REASON_UNSUPPORTED_PEER:
        return 'Peer does not support transfers';
      case ipc.TransferReason.TRANSFER_REASON_BUSY:
        return 'Another transfer is in flight';
      case ipc.TransferReason.TRANSFER_REASON_UNSAFE_FILENAME:
        return 'Unsafe filename';
      case ipc.TransferReason.TRANSFER_REASON_TOO_LARGE:
        return 'File exceeds the size limit';
      case ipc.TransferReason.TRANSFER_REASON_CHECKSUM_MISMATCH:
        return 'Integrity check failed';
      case ipc.TransferReason.TRANSFER_REASON_STORAGE_FAILED:
        return 'Storage error';
      case ipc.TransferReason.TRANSFER_REASON_INTERRUPTED:
        return 'Connection interrupted';
      case ipc.TransferReason.TRANSFER_REASON_CANCELLED_BY_PEER:
        return 'Cancelled by peer';
      case ipc.TransferReason.TRANSFER_REASON_CANCELLED_BY_USER:
        return 'Cancelled';
      case ipc.TransferReason.TRANSFER_REASON_PROTOCOL_ERROR:
        return 'Protocol error';
      case ipc.TransferReason.TRANSFER_REASON_INCOMPATIBLE_VERSION:
        return 'Incompatible device';
      default:
        // NONE and UNSPECIFIED are explicitly "not a failure".
        return null;
    }
  }

  /// Progress as bytes ('3.4 MB of 12.0 MB'), or just the byte count while the
  /// total is unknown. No speed/ETA: the backend does not provide either.
  String get progressLabel {
    if (!hasSize) return _formatBytes(bytesTransferred);
    return '${_formatBytes(bytesTransferred)} of ${_formatBytes(sizeBytes)}';
  }

  String get sizeLabel => _formatBytes(sizeBytes);

  /// Short peer reference for tiles: device ids are long hex digests.
  String get peerLabel {
    if (peerDeviceId.isEmpty) return 'Unknown device';
    if (peerDeviceId.length <= 12) return peerDeviceId;
    return '${peerDeviceId.substring(0, 12)}…';
  }

  /// When the transfer finished, else when it started. Null when the daemon
  /// reported neither timestamp.
  DateTime? get timestamp {
    if (finishedAtMs > 0) {
      return DateTime.fromMillisecondsSinceEpoch(finishedAtMs);
    }
    if (startedAtMs > 0) {
      return DateTime.fromMillisecondsSinceEpoch(startedAtMs);
    }
    return null;
  }

  String get timeAgo {
    final at = timestamp;
    if (at == null) return '—';
    final diff = DateTime.now().difference(at);
    if (diff.inSeconds < 45) return 'Just now';
    if (diff.inMinutes < 60) return '${diff.inMinutes}m ago';
    if (diff.inHours < 24) return '${diff.inHours}h ago';
    return '${diff.inDays}d ago';
  }

  /// True when the incoming snapshot carries something new. Used to suppress
  /// duplicate pushed events instead of rebuilding the list for no reason.
  bool isUpdateOver(TransferItem other) {
    return state != other.state ||
        bytesTransferred != other.bytesTransferred ||
        sizeBytes != other.sizeBytes ||
        reasonCode != other.reasonCode ||
        errorMessage != other.errorMessage ||
        finishedAtMs != other.finishedAtMs;
  }

  @override
  String toString() => 'TransferItem($transferId, ${direction.name}, '
      '${state.name}, $bytesTransferred/$sizeBytes)';
}

/// Outcome of a SendFile request. The daemon answers with a transfer id on
/// success, and with a typed reason (plus prose) when the offer could not start.
class TransferSendResult {
  const TransferSendResult({
    this.transferId = '',
    this.state = ipc.TransferState.TRANSFER_STATE_UNSPECIFIED,
    this.reasonCode = ipc.TransferReason.TRANSFER_REASON_UNSPECIFIED,
    this.errorMessage = '',
  });

  /// Sentinel for failures that never reached the daemon (validation,
  /// transport, unsupported platform).
  factory TransferSendResult.failure(String message) {
    return TransferSendResult(errorMessage: message);
  }

  /// Builds a result from a platform-channel map (Android pass-through shape).
  factory TransferSendResult.fromMap(Map<dynamic, dynamic> map) {
    return TransferSendResult(
      transferId: map['transferId']?.toString() ?? '',
      state: _stateFrom(map['state']),
      reasonCode: _reasonFrom(map['reasonCode']),
      errorMessage: map['errorMessage']?.toString() ?? '',
    );
  }

  final String transferId;
  final ipc.TransferState state;
  final ipc.TransferReason reasonCode;
  final String errorMessage;

  /// The daemon only returns a transfer id when the offer was accepted.
  bool get isOk => transferId.isNotEmpty;

  /// Text to show the user: typed reason first, daemon prose second.
  ///
  /// Both are kept: the typed reason classifies the failure, but the daemon's
  /// prose carries the specifics (e.g. which path was unreadable and why), and
  /// dropping it turned permission errors into an unactionable bare
  /// "Storage error" on device.
  String get errorText {
    final typed = TransferItem.reasonTextFor(reasonCode);
    if (typed != null && errorMessage.isNotEmpty) {
      return '$typed: $errorMessage';
    }
    if (typed != null) return typed;
    if (errorMessage.isNotEmpty) return errorMessage;
    return 'Transfer could not be started';
  }

  @override
  String toString() => 'TransferSendResult($transferId, ${state.name}, '
      '${reasonCode.name}, $errorMessage)';
}

// ---------------------------------------------------------------------------
// Platform-channel enum parsing (ints or proto names, case-insensitive).
// ---------------------------------------------------------------------------

ipc.TransferDirection _directionFrom(Object? value) {
  for (final candidate in ipc.TransferDirection.values) {
    if (_stateNameMatches(candidate.name, candidate.value, value)) {
      return candidate;
    }
  }
  return ipc.TransferDirection.TRANSFER_DIRECTION_UNSPECIFIED;
}

ipc.TransferState _stateFrom(Object? value) {
  for (final candidate in ipc.TransferState.values) {
    if (_stateNameMatches(candidate.name, candidate.value, value)) {
      return candidate;
    }
  }
  return ipc.TransferState.TRANSFER_STATE_UNSPECIFIED;
}

ipc.TransferReason _reasonFrom(Object? value) {
  for (final candidate in ipc.TransferReason.values) {
    if (_stateNameMatches(candidate.name, candidate.value, value)) {
      return candidate;
    }
  }
  return ipc.TransferReason.TRANSFER_REASON_UNSPECIFIED;
}

/// Matches one generated enum constant against an int value or a name. Both the
/// full proto name ('TRANSFER_STATE_ACTIVE') and the short form ('ACTIVE') are
/// accepted, since the two platform bridges spell enums differently.
bool _stateNameMatches(String protoName, int protoValue, Object? value) {
  if (value is int) return protoValue == value;
  if (value is! String || value.isEmpty) return false;
  final wanted = value.toUpperCase();
  if (protoName == wanted) return true;
  final parts = protoName.split('_');
  if (parts.length > 1 && parts.last == wanted) return true;
  return parts.length > 2 && parts.sublist(2).join('_') == wanted;
}

String _formatBytes(int bytes) {
  if (bytes <= 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  var value = bytes.toDouble();
  var unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit++;
  }
  if (unit == 0) return '$bytes B';
  return '${value.toStringAsFixed(1)} ${units[unit]}';
}
