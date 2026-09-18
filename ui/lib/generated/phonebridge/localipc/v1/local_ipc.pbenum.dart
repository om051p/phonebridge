// This is a generated file - do not edit.
//
// Generated from phonebridge/localipc/v1/local_ipc.proto.

// @dart = 3.3

// ignore_for_file: annotate_overrides, camel_case_types, comment_references
// ignore_for_file: constant_identifier_names
// ignore_for_file: curly_braces_in_flow_control_structures
// ignore_for_file: deprecated_member_use_from_same_package, library_prefixes
// ignore_for_file: non_constant_identifier_names, prefer_relative_imports

import 'dart:core' as $core;

import 'package:protobuf/protobuf.dart' as $pb;

/// SessionState models the 8 operational phases of a device session.
class SessionState extends $pb.ProtobufEnum {
  static const SessionState SESSION_STATE_UNSPECIFIED =
      SessionState._(0, _omitEnumNames ? '' : 'SESSION_STATE_UNSPECIFIED');
  static const SessionState SESSION_STATE_DISCONNECTED =
      SessionState._(1, _omitEnumNames ? '' : 'SESSION_STATE_DISCONNECTED');
  static const SessionState SESSION_STATE_DISCOVERING =
      SessionState._(2, _omitEnumNames ? '' : 'SESSION_STATE_DISCOVERING');
  static const SessionState SESSION_STATE_CONNECTING =
      SessionState._(3, _omitEnumNames ? '' : 'SESSION_STATE_CONNECTING');
  static const SessionState SESSION_STATE_CONNECTED =
      SessionState._(4, _omitEnumNames ? '' : 'SESSION_STATE_CONNECTED');
  static const SessionState SESSION_STATE_STREAMING =
      SessionState._(5, _omitEnumNames ? '' : 'SESSION_STATE_STREAMING');
  static const SessionState SESSION_STATE_RECONNECTING =
      SessionState._(6, _omitEnumNames ? '' : 'SESSION_STATE_RECONNECTING');
  static const SessionState SESSION_STATE_STOPPED =
      SessionState._(7, _omitEnumNames ? '' : 'SESSION_STATE_STOPPED');
  static const SessionState SESSION_STATE_FAILED =
      SessionState._(8, _omitEnumNames ? '' : 'SESSION_STATE_FAILED');

  static const $core.List<SessionState> values = <SessionState>[
    SESSION_STATE_UNSPECIFIED,
    SESSION_STATE_DISCONNECTED,
    SESSION_STATE_DISCOVERING,
    SESSION_STATE_CONNECTING,
    SESSION_STATE_CONNECTED,
    SESSION_STATE_STREAMING,
    SESSION_STATE_RECONNECTING,
    SESSION_STATE_STOPPED,
    SESSION_STATE_FAILED,
  ];

  static final $core.List<SessionState?> _byValue =
      $pb.ProtobufEnum.$_initByValueList(values, 8);
  static SessionState? valueOf($core.int value) =>
      value < 0 || value >= _byValue.length ? null : _byValue[value];

  const SessionState._(super.value, super.name);
}

/// SessionReason is a typed classification of a session state change. It exists
/// so the UI never parses prose: `reason` and `error_message` stay human-readable
/// diagnostics, while `reason_code` is the machine-stable contract. The values
/// are intentionally local to this contract even where they mirror
/// phonebridge.v1.Code — the local session state machine has failure modes of
/// its own (discovery, trust, reconnect budget) that the device protocol does
/// not express.
class SessionReason extends $pb.ProtobufEnum {
  static const SessionReason SESSION_REASON_UNSPECIFIED =
      SessionReason._(0, _omitEnumNames ? '' : 'SESSION_REASON_UNSPECIFIED');

  /// Explicitly "no failure": a normal transition (connecting, streaming,
  /// user stop). Distinguished from UNSPECIFIED, which means the server did
  /// not classify the transition at all.
  static const SessionReason SESSION_REASON_NONE =
      SessionReason._(1, _omitEnumNames ? '' : 'SESSION_REASON_NONE');

  /// The two devices have no protocol version in common (DEC-022 handshake).
  static const SessionReason SESSION_REASON_PROTOCOL_VERSION_MISMATCH =
      SessionReason._(
          2, _omitEnumNames ? '' : 'SESSION_REASON_PROTOCOL_VERSION_MISMATCH');

  /// The capture device could not satisfy the requested media parameters.
  static const SessionReason SESSION_REASON_UNSUPPORTED_MEDIA_PARAMS =
      SessionReason._(
          3, _omitEnumNames ? '' : 'SESSION_REASON_UNSUPPORTED_MEDIA_PARAMS');

  /// The peer is not in the trust store; pairing is required.
  static const SessionReason SESSION_REASON_DEVICE_NOT_TRUSTED =
      SessionReason._(
          4, _omitEnumNames ? '' : 'SESSION_REASON_DEVICE_NOT_TRUSTED');

  /// The target device was not found within the discovery timeout.
  static const SessionReason SESSION_REASON_DEVICE_NOT_FOUND = SessionReason._(
      5, _omitEnumNames ? '' : 'SESSION_REASON_DEVICE_NOT_FOUND');

  /// The peer already has an active session.
  static const SessionReason SESSION_REASON_SESSION_BUSY =
      SessionReason._(6, _omitEnumNames ? '' : 'SESSION_REASON_SESSION_BUSY');

  /// The user withdrew MediaProjection consent; the link is healthy but there
  /// is no longer a screen to send (DEC-020).
  static const SessionReason SESSION_REASON_CONSENT_REVOKED = SessionReason._(
      7, _omitEnumNames ? '' : 'SESSION_REASON_CONSENT_REVOKED');

  /// Capture or encode failed on the sending device.
  static const SessionReason SESSION_REASON_CAPTURE_FAILED =
      SessionReason._(8, _omitEnumNames ? '' : 'SESSION_REASON_CAPTURE_FAILED');

  /// ICE/DTLS failed outside the reconnect budget.
  static const SessionReason SESSION_REASON_TRANSPORT_FAILED = SessionReason._(
      9, _omitEnumNames ? '' : 'SESSION_REASON_TRANSPORT_FAILED');

  /// The bounded reconnect window elapsed without recovery.
  static const SessionReason SESSION_REASON_RECONNECT_TIMEOUT = SessionReason._(
      10, _omitEnumNames ? '' : 'SESSION_REASON_RECONNECT_TIMEOUT');

  /// The LAN signaling exchange failed (HTTP/offer/answer).
  static const SessionReason SESSION_REASON_SIGNALING_FAILED = SessionReason._(
      11, _omitEnumNames ? '' : 'SESSION_REASON_SIGNALING_FAILED');

  /// The local user stopped the session.
  static const SessionReason SESSION_REASON_USER_STOPPED =
      SessionReason._(12, _omitEnumNames ? '' : 'SESSION_REASON_USER_STOPPED');

  static const $core.List<SessionReason> values = <SessionReason>[
    SESSION_REASON_UNSPECIFIED,
    SESSION_REASON_NONE,
    SESSION_REASON_PROTOCOL_VERSION_MISMATCH,
    SESSION_REASON_UNSUPPORTED_MEDIA_PARAMS,
    SESSION_REASON_DEVICE_NOT_TRUSTED,
    SESSION_REASON_DEVICE_NOT_FOUND,
    SESSION_REASON_SESSION_BUSY,
    SESSION_REASON_CONSENT_REVOKED,
    SESSION_REASON_CAPTURE_FAILED,
    SESSION_REASON_TRANSPORT_FAILED,
    SESSION_REASON_RECONNECT_TIMEOUT,
    SESSION_REASON_SIGNALING_FAILED,
    SESSION_REASON_USER_STOPPED,
  ];

  static final $core.List<SessionReason?> _byValue =
      $pb.ProtobufEnum.$_initByValueList(values, 12);
  static SessionReason? valueOf($core.int value) =>
      value < 0 || value >= _byValue.length ? null : _byValue[value];

  const SessionReason._(super.value, super.name);
}

const $core.bool _omitEnumNames =
    $core.bool.fromEnvironment('protobuf.omit_enum_names');
