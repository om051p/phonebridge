// Session seam for the app-level connection model (Phase 5, first slice).
//
// Mirrors [TransferBackend]: the model and its controller depend on four
// members instead of the whole platform bridge, so a backend that has no
// session concept (Android — the phone side owns capture and has no local
// session state machine) simply does not implement it.
//
// A backend is implemented where the session state actually lives. On Linux
// that is [LinuxBridgeService], which already receives every session event on
// its single IPC subscription; exposing them here costs no extra IPC traffic.

import '../generated/phonebridge/localipc/v1/local_ipc.pb.dart' as ipc;
import '../generated/phonebridge/v1/phonebridge.pb.dart' as pb;

/// The codec requested when a caller does not pick one (DEC-020 makes H.264 the
/// primary, validated path).
const String kRequestedVideoCodec = 'h264';

/// Builds a requested media tuple, or null when the caller wants the device
/// default.
///
/// This lives next to the seam because every control path must express "device
/// default" the same way: on the wire an all-zero tuple means *unset* (DEC-022),
/// not "zero-sized video". Two hand-rolled builders is how that meaning drifts.
///
pb.MediaParams? mediaParamsRequest({
  required int width,
  required int height,
  required int fps,
  required int bitrateKbps,
  String codec = kRequestedVideoCodec,
}) {
  if (width <= 0 && height <= 0 && fps <= 0 && bitrateKbps <= 0) return null;
  return pb.MediaParams(
    width: width,
    height: height,
    fps: fps,
    bitrateKbps: bitrateKbps,
    codec: codec,
  );
}

abstract class SessionBackend {
  /// False when this platform has no local session state machine. The UI then
  /// shows the platform's own status surface instead of a dead connection card.
  bool get supportsSessions;

  /// The session the backend is currently tracking, or '' when there is none.
  /// Used to hydrate the model after a restart without inventing an id.
  String get activeSessionId;

  /// Pushed session lifecycle transitions (state, typed reason, reconnect
  /// attempt). Every change arrives here — the UI never polls for state.
  Stream<ipc.SessionEvent> get sessionEventStream;

  /// Authoritative snapshot for one session: state, typed reason, negotiated
  /// media, telemetry. Throws when the backend cannot answer.
  Future<ipc.GetSessionStateResponse> getSessionSnapshot(String sessionId);

  /// Asks a peer for a session. [requested] is the media tuple the caller wants
  /// — unset (or all-zero) fields mean "device default" — and the capture device
  /// answers with what it actually applies (DEC-022).
  ///
  /// This is the control half of the same seam: a screen must not need its own
  /// IPC client to start a session.
  Future<ipc.StartSessionResponse> startSession({
    required String target,
    pb.MediaParams? requested,
  });

  /// Terminates an active or in-progress session.
  Future<ipc.StopSessionResponse> stopSession({
    required String sessionId,
    String reason = 'user stopped',
  });
}

/// The backend used when a platform has no session support at all. Keeps the
/// controller honest: it reports "unsupported" rather than "disconnected",
/// because "no session capability" and "no session running" are different
/// facts and the UI words them differently.
class UnsupportedSessionBackend implements SessionBackend {
  const UnsupportedSessionBackend();

  @override
  bool get supportsSessions => false;

  @override
  String get activeSessionId => '';

  @override
  Stream<ipc.SessionEvent> get sessionEventStream => const Stream.empty();

  @override
  Future<ipc.GetSessionStateResponse> getSessionSnapshot(String sessionId) {
    throw UnsupportedError('this platform has no local session state machine');
  }

  @override
  Future<ipc.StartSessionResponse> startSession({
    required String target,
    pb.MediaParams? requested,
  }) {
    throw UnsupportedError('this platform has no local session state machine');
  }

  @override
  Future<ipc.StopSessionResponse> stopSession({
    required String sessionId,
    String reason = 'user stopped',
  }) {
    throw UnsupportedError('this platform has no local session state machine');
  }
}
