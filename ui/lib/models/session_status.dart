// Session presentation model (DEC-022).
//
// The daemon reports a session as a state plus a *typed* reason code, the
// requested media tuple, and the tuple the phone reported applying. Turning
// that into something a person can act on is presentation logic, so it lives
// here rather than inline in the view: the rules are unit-testable without a
// daemon, and the view stays about layout.
//
// The distinction the UI must never blur: a *requested* tuple is what we asked
// for, an *actual* tuple is what the phone said it applies. When the phone did
// not report one, the UI says so instead of echoing the request back as if it
// had been confirmed.

import '../generated/phonebridge/localipc/v1/local_ipc.pb.dart';
import '../generated/phonebridge/v1/phonebridge.pb.dart' as pb;

class SessionStatus {
  const SessionStatus({
    required this.state,
    this.sessionId,
    this.deviceId,
    this.reasonCode = SessionReason.SESSION_REASON_UNSPECIFIED,
    this.reasonDetail = '',
    this.errorMessage = '',
    this.durationMs = 0,
    this.requested,
    this.actual,
    this.capabilities,
    this.reconnectAttempts = 0,
    this.sinkKind = SinkKind.SINK_KIND_UNSPECIFIED,
    this.sinkActive = false,
  });

  final SessionState state;
  final String? sessionId;

  /// Authoritative session peer (daemon Session.targetDeviceId).
  /// Null/empty means no active target: the UI must render disconnected and
  /// must never fall back to trust ordering (Phase B).
  final String? deviceId;
  final SessionReason reasonCode;
  final String reasonDetail;
  final String errorMessage;
  final int durationMs;

  /// What this side asked the phone for. Null until a session has been started.
  final pb.MediaParams? requested;

  /// What the phone reported applying. Null means *not reported* — never
  /// assumed to equal [requested].
  final pb.MediaParams? actual;

  /// What the phone advertised it can do (DEC-022 offer exchange, retained by
  /// the Go session). Null means *not reported* — the UI renders "Checking
  /// device capabilities…" instead of guessing availability.
  final pb.MediaCapabilities? capabilities;

  final int reconnectAttempts;

  /// Where the daemon is writing decoded frames, as classified by the local
  /// engine. UNSPECIFIED until a snapshot classifies it, and cleared again
  /// when the session reaches an idle/terminal state.
  final SinkKind sinkKind;

  /// Whether that sink is currently attached to a live session.
  final bool sinkActive;

  /// True once the daemon has actually classified a sink — gates the display
  /// banner so "Active (ffplay)" is never rendered on assumption (a headless
  /// daemon runs a null sink and says so).
  bool get hasReportedSink => sinkKind != SinkKind.SINK_KIND_UNSPECIFIED;

  /// True once the peer has advertised its capture capabilities — gates the
  /// quality presets so "Available" is never rendered on assumption.
  bool get hasCapabilities => capabilities != null;

  static const SessionStatus idle = SessionStatus(
    state: SessionState.SESSION_STATE_DISCONNECTED,
  );

  /// A session state event (a transition, not a full snapshot). The negotiated
  /// media is not carried on the wire by events, so the previous values are
  /// preserved rather than cleared.
  factory SessionStatus.fromEvent(
    SessionEvent event, {
    SessionStatus? previous,
  }) {
    // Entering an idle or terminal state ends the session's sink, so the model
    // clears it right here: a stale classification must not outlive its
    // session even between snapshot polls.
    final eventClearsSink = event.state == SessionState.SESSION_STATE_STOPPED ||
        event.state == SessionState.SESSION_STATE_FAILED ||
        event.state == SessionState.SESSION_STATE_DISCONNECTED;
    return SessionStatus(
      state: event.state,
      sessionId:
          event.sessionId.isNotEmpty ? event.sessionId : previous?.sessionId,
      // Events carry no peer field: the target set at start (or from the
      // last snapshot) survives across transitions. A terminal event clears
      // it so no stale peer outlives its session.
      deviceId: eventClearsSink ? null : previous?.deviceId,
      reasonCode: event.reasonCode,
      reasonDetail: event.reason.isNotEmpty
          ? event.reason
          : (previous?.reasonDetail ?? ''),
      errorMessage: event.errorMessage.isNotEmpty
          ? event.errorMessage
          : (previous?.errorMessage ?? ''),
      durationMs: previous?.durationMs ?? 0,
      requested: previous?.requested,
      actual: previous?.actual,
      // Capabilities describe the device, not the session: an event carries
      // no advertisement, so the last reported one is preserved — including
      // across terminal transitions, where the snapshot keeps serving it.
      capabilities: previous?.capabilities,
      reconnectAttempts: previous?.reconnectAttempts ?? 0,
      // Events do not carry sink state: non-terminal transitions preserve
      // the previous classification.
      sinkKind: eventClearsSink
          ? SinkKind.SINK_KIND_UNSPECIFIED
          : (previous?.sinkKind ?? SinkKind.SINK_KIND_UNSPECIFIED),
      sinkActive: !eventClearsSink && (previous?.sinkActive ?? false),
    );
  }

  /// A telemetry poll. Carries the authoritative view of the negotiation, so it
  /// replaces the previous snapshots wholesale (including clearing a stale
  /// error when the session is healthy again).
  factory SessionStatus.fromSnapshot(
    GetSessionStateResponse snapshot, {
    SessionStatus? previous,
  }) {
    final hasRequested = snapshot.hasRequested();
    final hasActual = snapshot.hasActual();
    return SessionStatus(
      state: snapshot.state,
      sessionId: snapshot.sessionId.isNotEmpty
          ? snapshot.sessionId
          : previous?.sessionId,
      // The snapshot is authoritative for the peer: a non-empty device_id
      // adopts it, an empty one preserves the previous target (early
      // DISCOVERING snapshots), and a terminal snapshot clears it.
      deviceId: snapshot.deviceId.isNotEmpty
          ? snapshot.deviceId
          : (snapshot.state == SessionState.SESSION_STATE_STOPPED ||
                  snapshot.state == SessionState.SESSION_STATE_FAILED ||
                  snapshot.state == SessionState.SESSION_STATE_DISCONNECTED
              ? null
              : previous?.deviceId),
      reasonCode: snapshot.reasonCode,
      reasonDetail: previous?.reasonDetail ?? '',
      errorMessage: snapshot.errorMessage,
      durationMs: snapshot.connectedDurationMs.toInt(),
      requested: hasRequested ? snapshot.requested : previous?.requested,
      // An unreported tuple must stay unreported; a stale one would be a lie.
      actual: hasActual ? snapshot.actual : null,
      // An unreported advertisement must stay unreported: the UI renders
      // "Checking device capabilities…" instead of guessing availability.
      capabilities: snapshot.hasCapabilities() ? snapshot.capabilities : null,
      reconnectAttempts: snapshot.reconnectAttempts,
      // The snapshot is authoritative for sink state — the daemon clears both
      // fields itself on terminal transitions.
      sinkKind: snapshot.sinkKind,
      sinkActive: snapshot.sinkActive,
    );
  }

  SessionStatus copyWith({
    SessionState? state,
    String? sessionId,
    String? deviceId,
    SessionReason? reasonCode,
    String? reasonDetail,
    String? errorMessage,
    int? durationMs,
    pb.MediaParams? requested,
    pb.MediaParams? actual,
    bool clearActual = false,
    pb.MediaCapabilities? capabilities,
    bool clearError = false,
    int? reconnectAttempts,
    SinkKind? sinkKind,
    bool? sinkActive,
    bool clearDeviceId = false,
  }) {
    final nextState = state ?? this.state;
    // An idle/terminal state owns no sink: copyWith must never be a path for
    // a stale classification to survive a stop or failure (the controller's
    // stop goes through here).
    final sinkGone = nextState == SessionState.SESSION_STATE_STOPPED ||
        nextState == SessionState.SESSION_STATE_FAILED ||
        nextState == SessionState.SESSION_STATE_DISCONNECTED;
    return SessionStatus(
      state: nextState,
      sessionId: sessionId ?? this.sessionId,
      // A terminal state owns no peer either: clearing here keeps a stale
      // target from outliving its session even between snapshot polls.
      deviceId: clearDeviceId || sinkGone ? null : (deviceId ?? this.deviceId),
      reasonCode: reasonCode ?? this.reasonCode,
      reasonDetail: reasonDetail ?? this.reasonDetail,
      errorMessage: clearError ? '' : (errorMessage ?? this.errorMessage),
      durationMs: durationMs ?? this.durationMs,
      requested: requested ?? this.requested,
      actual: clearActual ? null : (actual ?? this.actual),
      // Capabilities describe the device, not the session, so they survive
      // terminal transitions (the snapshot keeps serving the last report).
      capabilities: capabilities ?? this.capabilities,
      reconnectAttempts: reconnectAttempts ?? this.reconnectAttempts,
      sinkKind: sinkKind ??
          (sinkGone ? SinkKind.SINK_KIND_UNSPECIFIED : this.sinkKind),
      sinkActive: sinkActive ?? (sinkGone ? false : this.sinkActive),
    );
  }

  // ------------------------------------------------------------ state classes

  bool get isIdle => state == SessionState.SESSION_STATE_DISCONNECTED;

  bool get isTerminal =>
      state == SessionState.SESSION_STATE_STOPPED ||
      state == SessionState.SESSION_STATE_FAILED;

  bool get isActive => !isIdle && !isTerminal;

  /// Recovery is automatic and bounded, so the UI presents it as "healing",
  /// not as an error the user must resolve.
  bool get isRecovering => state == SessionState.SESSION_STATE_RECONNECTING;

  /// In-progress connection negotiation (discovering peer or negotiating params).
  bool get isConnecting =>
      state == SessionState.SESSION_STATE_CONNECTING ||
      state == SessionState.SESSION_STATE_DISCOVERING;

  /// Connected WebRTC peer or actively streaming frames.
  bool get isConnectedOrStreaming =>
      state == SessionState.SESSION_STATE_CONNECTED ||
      state == SessionState.SESSION_STATE_STREAMING;

  /// Telemetry keeps flowing across a reconnect: the session is the same one
  /// (the daemon preserves its ID), it is only the transport that is replaced.
  bool get shouldKeepTelemetry => isActive;

  bool get canStop => isActive;

  // ------------------------------------------------------------------ labels

  String get label {
    switch (state) {
      case SessionState.SESSION_STATE_STREAMING:
        return 'STREAMING (VIDEO ACTIVE)';
      case SessionState.SESSION_STATE_CONNECTED:
        return 'CONNECTED (WEBRTC PEER)';
      case SessionState.SESSION_STATE_CONNECTING:
        return 'CONNECTING (NEGOTIATING PARAMETERS)';
      case SessionState.SESSION_STATE_DISCOVERING:
        return 'DISCOVERING TARGET DEVICE';
      case SessionState.SESSION_STATE_RECONNECTING:
        return 'RECONNECTING (AUTOMATIC RECOVERY)';
      case SessionState.SESSION_STATE_FAILED:
        return 'SESSION FAILED';
      case SessionState.SESSION_STATE_STOPPED:
        return 'SESSION STOPPED';
      case SessionState.SESSION_STATE_DISCONNECTED:
        return 'SESSION IDLE (READY)';
      default:
        return 'UNKNOWN SESSION STATE';
    }
  }

  /// Detail line for a recovery in progress, including how many attempts the
  /// daemon has made so far.
  String get recoveringDetail {
    if (!isRecovering) return '';
    if (reconnectAttempts <= 0) {
      return 'Transport lost; re-negotiating with the phone.';
    }
    return 'Transport lost; attempt $reconnectAttempts of the bounded reconnect window.';
  }

  static String _describe(pb.MediaParams? params) {
    if (params == null) return 'not reported';
    final parts = <String>[];
    if (params.width > 0 && params.height > 0) {
      parts.add('${params.width}x${params.height}');
    }
    if (params.fps > 0) parts.add('@${params.fps} fps');
    if (params.bitrateKbps > 0) parts.add('${params.bitrateKbps} kbps');
    if (params.codec.isNotEmpty) parts.add(params.codec);
    return parts.isEmpty ? 'device default' : parts.join(' · ');
  }

  String get requestedSummary => _describe(requested);

  String get actualSummary => _describe(actual);

  bool get hasNegotiatedMedia => requested != null || actual != null;

  /// True when the phone is applying something other than what was asked for
  /// (a reported downgrade). A missing report is not a mismatch — it is
  /// unknown, and the UI says so separately.
  bool get isDowngraded {
    final req = requested;
    final act = actual;
    if (req == null || act == null) return false;
    return req.width != act.width ||
        req.height != act.height ||
        req.fps != act.fps;
  }

  /// The one-line negotiation summary shown under the state banner.
  String negotiationSummary() {
    if (!hasNegotiatedMedia) return '';
    final requestedLine = 'Requested: $requestedSummary';
    if (actual == null) {
      return '$requestedLine\nApplied: not reported by the phone yet';
    }
    final appliedLine = 'Applied: $actualSummary';
    if (isDowngraded) {
      return '$requestedLine\n$appliedLine  (device applied different parameters)';
    }
    return '$requestedLine\n$appliedLine';
  }

  /// Turns the typed reason code into something actionable. The reason string
  /// stays available as technical detail; the code is what drives the message,
  /// so a failure class is never guessed from prose.
  String get failureSummary {
    switch (reasonCode) {
      case SessionReason.SESSION_REASON_CONSENT_REVOKED:
        return 'Screen sharing was stopped on the phone (capture consent withdrawn). '
            'Start sharing again on the phone, then start a new session.';
      case SessionReason.SESSION_REASON_CAPTURE_FAILED:
        return "The phone's capture pipeline failed. Check the phone, then start a new session.";
      case SessionReason.SESSION_REASON_UNSUPPORTED_MEDIA_PARAMS:
        return 'The phone could not apply the requested media parameters. '
            'Ask for a supported resolution or frame rate.';
      case SessionReason.SESSION_REASON_PROTOCOL_VERSION_MISMATCH:
        return 'The phone speaks a different session protocol version. '
            'Update PhoneBridge on both devices.';
      case SessionReason.SESSION_REASON_DEVICE_NOT_TRUSTED:
        return 'This device is not trusted. Pair it before starting a session.';
      case SessionReason.SESSION_REASON_SESSION_BUSY:
        return 'The phone already has an active session. Stop it there first.';
      case SessionReason.SESSION_REASON_DEVICE_NOT_FOUND:
        return 'The phone was not found on the LAN. Check that it is discoverable '
            'and on the same network.';
      case SessionReason.SESSION_REASON_TRANSPORT_FAILED:
        return 'The media transport failed.';
      case SessionReason.SESSION_REASON_RECONNECT_TIMEOUT:
        return 'The connection did not recover within the reconnect window. '
            'Check the network, then start a new session.';
      case SessionReason.SESSION_REASON_SIGNALING_FAILED:
        return 'Signalling to the phone failed.';
      case SessionReason.SESSION_REASON_USER_STOPPED:
      case SessionReason.SESSION_REASON_NONE:
      case SessionReason.SESSION_REASON_UNSPECIFIED:
        return '';
      default:
        return '';
    }
  }

  /// Short badge text for the typed reason (empty when there is nothing typed).
  String get reasonBadge {
    switch (reasonCode) {
      case SessionReason.SESSION_REASON_NONE:
      case SessionReason.SESSION_REASON_UNSPECIFIED:
      case SessionReason.SESSION_REASON_USER_STOPPED:
        return '';
      default:
        return _enumName(reasonCode);
    }
  }

  static String _enumName(SessionReason reason) {
    // ProtobufEnum#toString() is the declared name (SESSION_REASON_<NAME>).
    final raw = reason.toString();
    const prefix = 'SESSION_REASON_';
    return raw.startsWith(prefix) ? raw.substring(prefix.length) : raw;
  }
}
