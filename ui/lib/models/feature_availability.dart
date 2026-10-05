// Feature availability as pure presentation mappings (Phase B follow-up).
//
// No state lives here: every function reads the runtime models that already
// own the facts (session status, clipboard status, permission + liveness
// flags, capability advertisements) and answers what the user can do right
// now. Unknown runtime stays unknown — a missing liveness report or
// capability advertisement never renders as Active/Available.

import '../generated/phonebridge/localipc/v1/local_ipc.pb.dart' as ipc;
import '../generated/phonebridge/v1/phonebridge.pb.dart' as pb;
import 'clipboard_status.dart';
import 'session_status.dart';

/// User-facing availability of one feature.
enum FeatureState {
  active,
  available,
  granted,
  manual,
  starting,
  needsSetup,
  restricted,
  disabled,
  unavailable,
  unknown,
}

/// A feature's availability with the user-facing label and detail.
class FeatureStatus {
  const FeatureStatus({
    required this.state,
    required this.label,
    required this.detail,
  });

  final FeatureState state;
  final String label;
  final String detail;
}

/// Screen sharing, from the live session plus the capability advertisement.
///
/// The advertisement gates presets (see `availabilityOf` in
/// media_capabilities.dart); the session decides the lifecycle. A failed
/// session is unavailable with its typed reason; an idle session with no
/// advertisement is available-but-unverified rather than refused.
FeatureStatus screenAvailability({
  required SessionStatus session,
  required pb.MediaCapabilities? capabilities,
}) {
  if (session.state == ipc.SessionState.SESSION_STATE_FAILED) {
    final reason =
        session.failureSummary.isNotEmpty ? session.failureSummary : session.errorMessage;
    return FeatureStatus(
      state: FeatureState.unavailable,
      label: 'Unavailable',
      detail: reason.isNotEmpty ? reason : 'The last session failed.',
    );
  }
  if (session.isActive) {
    if (session.state == ipc.SessionState.SESSION_STATE_STREAMING) {
      return const FeatureStatus(
        state: FeatureState.active,
        label: 'Streaming',
        detail: 'The screen is streaming now.',
      );
    }
    final recovery = session.recoveringDetail;
    return FeatureStatus(
      state: FeatureState.starting,
      label: 'Starting',
      detail: recovery.isNotEmpty ? recovery : 'A session is being negotiated.',
    );
  }
  if (capabilities != null && !capabilities.supportsScreen) {
    return const FeatureStatus(
      state: FeatureState.unavailable,
      label: 'Unavailable',
      detail: 'This device cannot capture its screen.',
    );
  }
  return const FeatureStatus(
    state: FeatureState.available,
    label: 'Available',
    detail: 'Start sharing from the Screen tab.',
  );
}

/// Clipboard, from the canonical clipboard status (which already separates
/// the Android dormant reality from the Linux ambient one).
FeatureStatus clipboardAvailability(ClipboardStatus status) {
  if (!status.isEnabled) {
    return const FeatureStatus(
      state: FeatureState.disabled,
      label: 'Sync turned off',
      detail: 'Turn on the master switch to enable clipboard sync.',
    );
  }
  switch (status.state) {
    case ClipboardSyncState.ambientActive:
      return const FeatureStatus(
        state: FeatureState.active,
        label: 'Active',
        detail: 'Background monitoring is flowing.',
      );
    case ClipboardSyncState.writeOnlyDormant:
      return const FeatureStatus(
        state: FeatureState.manual,
        label: 'Manual',
        detail: 'Inbound copies apply automatically; push outbound manually.',
      );
    case ClipboardSyncState.needsSetup:
      return FeatureStatus(
        state: FeatureState.needsSetup,
        label: 'Needs setup',
        detail: status.state.description,
      );
    case ClipboardSyncState.restricted:
      return FeatureStatus(
        state: FeatureState.restricted,
        label: 'Restricted',
        detail: status.state.description,
      );
    case ClipboardSyncState.unavailable:
      return FeatureStatus(
        state: FeatureState.unavailable,
        label: 'Unavailable',
        detail: status.state.description,
      );
    case ClipboardSyncState.stopped:
      return const FeatureStatus(
        state: FeatureState.unavailable,
        label: 'Not running',
        detail: 'The background service is not running.',
      );
  }
}

/// Notification mirroring, from the granted flag plus the listener service's
/// runtime binding. Granted-but-stopped keeps the "Granted" label with a
/// warning detail — exactly the conflation this mapping exists to end.
FeatureStatus notificationAvailability({
  required bool granted,
  required bool? serviceActive,
}) {
  if (!granted) {
    return const FeatureStatus(
      state: FeatureState.needsSetup,
      label: 'Not granted',
      detail: 'Enable notification access in system settings.',
    );
  }
  if (serviceActive == true) {
    return const FeatureStatus(
      state: FeatureState.active,
      label: 'Active',
      detail: 'The listener service is bound and mirroring.',
    );
  }
  if (serviceActive == false) {
    return const FeatureStatus(
      state: FeatureState.needsSetup,
      label: 'Granted',
      detail: 'Access is granted but the listener service is not running.',
    );
  }
  return const FeatureStatus(
    state: FeatureState.granted,
    label: 'Granted',
    detail: 'Access is granted; runtime state not reported by this build.',
  );
}

/// Remote input, from the accessibility grant plus the service binding.
/// `restricted` covers platform-restricted access where reported.
FeatureStatus remoteInputAvailability({
  required bool granted,
  required bool? serviceActive,
  bool restricted = false,
}) {
  if (restricted) {
    return const FeatureStatus(
      state: FeatureState.restricted,
      label: 'Restricted',
      detail: 'The system is restricting accessibility access.',
    );
  }
  if (!granted) {
    return const FeatureStatus(
      state: FeatureState.needsSetup,
      label: 'Not configured',
      detail: 'Enable the accessibility service in system settings.',
    );
  }
  if (serviceActive == true) {
    return const FeatureStatus(
      state: FeatureState.active,
      label: 'Active',
      detail: 'Supported gestures run through the accessibility mechanism.',
    );
  }
  if (serviceActive == false) {
    return const FeatureStatus(
      state: FeatureState.needsSetup,
      label: 'Granted',
      detail: 'Access is granted but the service is not running.',
    );
  }
  return const FeatureStatus(
    state: FeatureState.granted,
    label: 'Granted',
    detail: 'Access is granted; runtime state not reported by this build.',
  );
}
