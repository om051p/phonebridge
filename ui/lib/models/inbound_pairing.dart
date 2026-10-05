/// One pairing request awaiting THIS device's approval (Phase 2 bidirectional
/// pairing). This model is a presentation mapping over two transports:
///
/// - Linux: daemon `ListInboundPairings` / `PairingEvent` (local IPC proto).
/// - Android: `LanSignalingServer.pendingPairings` surfaced over the platform
///   event channel.
///
/// The owning state lives native (Go signaling server / Kotlin signaling
/// server); Flutter only renders it and records the user's decision. Trust is
/// committed natively, never here.
class InboundPairing {
  /// Opaque token identifying the request; passed back when responding.
  final String token;

  final String remoteName;
  final String remotePlatform;

  /// Short Authentication String the local user compares with the
  /// requester's screen before approving.
  final String sas;

  final int createdAtMs;

  const InboundPairing({
    required this.token,
    required this.remoteName,
    required this.remotePlatform,
    required this.sas,
    required this.createdAtMs,
  });

  /// Builds from the Android host's map shape (platform event channel).
  factory InboundPairing.fromMap(Map<dynamic, dynamic> map) {
    return InboundPairing(
      token: (map['token'] as String?) ?? '',
      remoteName: (map['remoteName'] as String?) ?? '',
      remotePlatform: (map['remotePlatform'] as String?) ?? '',
      sas: (map['sas'] as String?) ?? '',
      createdAtMs: (map['createdAtMs'] as num?)?.toInt() ?? 0,
    );
  }

  String get platformLabel =>
      remotePlatform.isEmpty ? 'UNKNOWN' : remotePlatform.toUpperCase();
}
