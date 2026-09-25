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

/// SinkKind classifies where a session's decoded frames are written, so the
/// UI reports the real display path instead of assuming ffplay is running (a
/// headless daemon silently uses a null sink).
class SinkKind extends $pb.ProtobufEnum {
  static const SinkKind SINK_KIND_UNSPECIFIED =
      SinkKind._(0, _omitEnumNames ? '' : 'SINK_KIND_UNSPECIFIED');
  static const SinkKind SINK_KIND_NULL =
      SinkKind._(1, _omitEnumNames ? '' : 'SINK_KIND_NULL');
  static const SinkKind SINK_KIND_DISPLAY =
      SinkKind._(2, _omitEnumNames ? '' : 'SINK_KIND_DISPLAY');
  static const SinkKind SINK_KIND_PIPE =
      SinkKind._(3, _omitEnumNames ? '' : 'SINK_KIND_PIPE');
  static const SinkKind SINK_KIND_FILE =
      SinkKind._(4, _omitEnumNames ? '' : 'SINK_KIND_FILE');

  static const $core.List<SinkKind> values = <SinkKind>[
    SINK_KIND_UNSPECIFIED,
    SINK_KIND_NULL,
    SINK_KIND_DISPLAY,
    SINK_KIND_PIPE,
    SINK_KIND_FILE,
  ];

  static final $core.List<SinkKind?> _byValue =
      $pb.ProtobufEnum.$_initByValueList(values, 4);
  static SinkKind? valueOf($core.int value) =>
      value < 0 || value >= _byValue.length ? null : _byValue[value];

  const SinkKind._(super.value, super.name);
}

/// TransferDirection states which way the bytes flow for this device.
class TransferDirection extends $pb.ProtobufEnum {
  static const TransferDirection TRANSFER_DIRECTION_UNSPECIFIED =
      TransferDirection._(
          0, _omitEnumNames ? '' : 'TRANSFER_DIRECTION_UNSPECIFIED');

  /// This device sends the file.
  static const TransferDirection TRANSFER_DIRECTION_OUTBOUND =
      TransferDirection._(
          1, _omitEnumNames ? '' : 'TRANSFER_DIRECTION_OUTBOUND');

  /// This device receives the file.
  static const TransferDirection TRANSFER_DIRECTION_INBOUND =
      TransferDirection._(
          2, _omitEnumNames ? '' : 'TRANSFER_DIRECTION_INBOUND');

  static const $core.List<TransferDirection> values = <TransferDirection>[
    TRANSFER_DIRECTION_UNSPECIFIED,
    TRANSFER_DIRECTION_OUTBOUND,
    TRANSFER_DIRECTION_INBOUND,
  ];

  static final $core.List<TransferDirection?> _byValue =
      $pb.ProtobufEnum.$_initByValueList(values, 2);
  static TransferDirection? valueOf($core.int value) =>
      value < 0 || value >= _byValue.length ? null : _byValue[value];

  const TransferDirection._(super.value, super.name);
}

/// TransferState is the lifecycle phase of one transfer.
class TransferState extends $pb.ProtobufEnum {
  static const TransferState TRANSFER_STATE_UNSPECIFIED =
      TransferState._(0, _omitEnumNames ? '' : 'TRANSFER_STATE_UNSPECIFIED');

  /// Offer sent (outbound) or received (inbound); awaiting FileAccept.
  static const TransferState TRANSFER_STATE_PENDING =
      TransferState._(1, _omitEnumNames ? '' : 'TRANSFER_STATE_PENDING');

  /// Chunks are flowing.
  static const TransferState TRANSFER_STATE_ACTIVE =
      TransferState._(2, _omitEnumNames ? '' : 'TRANSFER_STATE_ACTIVE');

  /// All chunks received; verifying size/digest and promoting the file.
  static const TransferState TRANSFER_STATE_VERIFYING =
      TransferState._(3, _omitEnumNames ? '' : 'TRANSFER_STATE_VERIFYING');

  /// Terminal success: the file was committed at the destination.
  static const TransferState TRANSFER_STATE_COMPLETE =
      TransferState._(4, _omitEnumNames ? '' : 'TRANSFER_STATE_COMPLETE');

  /// Terminal: cancelled locally or by the peer.
  static const TransferState TRANSFER_STATE_CANCELLED =
      TransferState._(5, _omitEnumNames ? '' : 'TRANSFER_STATE_CANCELLED');

  /// Terminal: failed (integrity, storage, interruption, protocol, policy).
  static const TransferState TRANSFER_STATE_FAILED =
      TransferState._(6, _omitEnumNames ? '' : 'TRANSFER_STATE_FAILED');

  static const $core.List<TransferState> values = <TransferState>[
    TRANSFER_STATE_UNSPECIFIED,
    TRANSFER_STATE_PENDING,
    TRANSFER_STATE_ACTIVE,
    TRANSFER_STATE_VERIFYING,
    TRANSFER_STATE_COMPLETE,
    TRANSFER_STATE_CANCELLED,
    TRANSFER_STATE_FAILED,
  ];

  static final $core.List<TransferState?> _byValue =
      $pb.ProtobufEnum.$_initByValueList(values, 6);
  static TransferState? valueOf($core.int value) =>
      value < 0 || value >= _byValue.length ? null : _byValue[value];

  const TransferState._(super.value, super.name);
}

/// TransferReason is the typed classification of a non-success outcome, so the
/// UI never parses prose. SESSION-style distinction applies: NONE is an
/// explicitly normal outcome, UNSPECIFIED means the daemon did not classify.
class TransferReason extends $pb.ProtobufEnum {
  static const TransferReason TRANSFER_REASON_UNSPECIFIED =
      TransferReason._(0, _omitEnumNames ? '' : 'TRANSFER_REASON_UNSPECIFIED');

  /// No failure (normal progress or completion).
  static const TransferReason TRANSFER_REASON_NONE =
      TransferReason._(1, _omitEnumNames ? '' : 'TRANSFER_REASON_NONE');

  /// No active session with a peer that supports file transfer.
  static const TransferReason TRANSFER_REASON_NO_SESSION =
      TransferReason._(2, _omitEnumNames ? '' : 'TRANSFER_REASON_NO_SESSION');

  /// The peer does not advertise file transfer (no "transfer" DataChannel).
  static const TransferReason TRANSFER_REASON_UNSUPPORTED_PEER =
      TransferReason._(
          3, _omitEnumNames ? '' : 'TRANSFER_REASON_UNSUPPORTED_PEER');

  /// A transfer is already in flight in that direction (DEC-024 limit).
  static const TransferReason TRANSFER_REASON_BUSY =
      TransferReason._(4, _omitEnumNames ? '' : 'TRANSFER_REASON_BUSY');

  /// The proposed filename is not a plain basename.
  static const TransferReason TRANSFER_REASON_UNSAFE_FILENAME =
      TransferReason._(
          5, _omitEnumNames ? '' : 'TRANSFER_REASON_UNSAFE_FILENAME');

  /// The file exceeds the configured size policy.
  static const TransferReason TRANSFER_REASON_TOO_LARGE =
      TransferReason._(6, _omitEnumNames ? '' : 'TRANSFER_REASON_TOO_LARGE');

  /// Size or SHA-256 did not match; nothing was committed.
  static const TransferReason TRANSFER_REASON_CHECKSUM_MISMATCH =
      TransferReason._(
          7, _omitEnumNames ? '' : 'TRANSFER_REASON_CHECKSUM_MISMATCH');

  /// Destination write/promotion failed (space, permission, MediaStore/SAF).
  static const TransferReason TRANSFER_REASON_STORAGE_FAILED = TransferReason._(
      8, _omitEnumNames ? '' : 'TRANSFER_REASON_STORAGE_FAILED');

  /// The transport/session went away mid-transfer; retry restarts from zero.
  static const TransferReason TRANSFER_REASON_INTERRUPTED =
      TransferReason._(9, _omitEnumNames ? '' : 'TRANSFER_REASON_INTERRUPTED');

  /// The peer cancelled.
  static const TransferReason TRANSFER_REASON_CANCELLED_BY_PEER =
      TransferReason._(
          10, _omitEnumNames ? '' : 'TRANSFER_REASON_CANCELLED_BY_PEER');

  /// The local user cancelled.
  static const TransferReason TRANSFER_REASON_CANCELLED_BY_USER =
      TransferReason._(
          11, _omitEnumNames ? '' : 'TRANSFER_REASON_CANCELLED_BY_USER');

  /// A wire frame violated the protocol (wrong index/offset/state).
  static const TransferReason TRANSFER_REASON_PROTOCOL_ERROR = TransferReason._(
      12, _omitEnumNames ? '' : 'TRANSFER_REASON_PROTOCOL_ERROR');

  /// The peer speaks a transfer frame version this build does not implement.
  static const TransferReason TRANSFER_REASON_INCOMPATIBLE_VERSION =
      TransferReason._(
          13, _omitEnumNames ? '' : 'TRANSFER_REASON_INCOMPATIBLE_VERSION');

  static const $core.List<TransferReason> values = <TransferReason>[
    TRANSFER_REASON_UNSPECIFIED,
    TRANSFER_REASON_NONE,
    TRANSFER_REASON_NO_SESSION,
    TRANSFER_REASON_UNSUPPORTED_PEER,
    TRANSFER_REASON_BUSY,
    TRANSFER_REASON_UNSAFE_FILENAME,
    TRANSFER_REASON_TOO_LARGE,
    TRANSFER_REASON_CHECKSUM_MISMATCH,
    TRANSFER_REASON_STORAGE_FAILED,
    TRANSFER_REASON_INTERRUPTED,
    TRANSFER_REASON_CANCELLED_BY_PEER,
    TRANSFER_REASON_CANCELLED_BY_USER,
    TRANSFER_REASON_PROTOCOL_ERROR,
    TRANSFER_REASON_INCOMPATIBLE_VERSION,
  ];

  static final $core.List<TransferReason?> _byValue =
      $pb.ProtobufEnum.$_initByValueList(values, 13);
  static TransferReason? valueOf($core.int value) =>
      value < 0 || value >= _byValue.length ? null : _byValue[value];

  const TransferReason._(super.value, super.name);
}

const $core.bool _omitEnumNames =
    $core.bool.fromEnvironment('protobuf.omit_enum_names');
