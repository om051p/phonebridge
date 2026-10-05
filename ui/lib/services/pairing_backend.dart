import '../models/inbound_pairing.dart';

/// Inbound-pairing seam (Phase 2 bidirectional pairing): pairing requests that
/// arrive at THIS device's signaling server and await the local user's
/// explicit approval.
///
/// Resolved structurally (like [SessionBackend]/[NotificationBackend]) so the
/// platform interface stays closed: a bridge that carries the inbound-pairing
/// state implements this; platforms without one report nothing rather than
/// faking an empty workflow.
abstract class PairingBackend {
  /// Snapshots the pairing requests awaiting local approval. Expired entries
  /// are swept by the native owner; an empty list is the truth after a
  /// timeout.
  Future<List<InboundPairing>> listInboundPairings();

  /// Records the local user's decision. Returns false when the token was
  /// unknown or already expired. Approval alone never commits trust: the
  /// requester's signed confirm must still verify natively.
  Future<bool> respondInboundPairing({
    required String pairingToken,
    required bool approved,
  });
}
