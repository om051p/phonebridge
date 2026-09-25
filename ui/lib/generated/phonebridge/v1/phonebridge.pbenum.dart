// This is a generated file - do not edit.
//
// Generated from phonebridge/v1/phonebridge.proto.

// @dart = 3.3

// ignore_for_file: annotate_overrides, camel_case_types, comment_references
// ignore_for_file: constant_identifier_names
// ignore_for_file: curly_braces_in_flow_control_structures
// ignore_for_file: deprecated_member_use_from_same_package, library_prefixes
// ignore_for_file: non_constant_identifier_names, prefer_relative_imports

import 'dart:core' as $core;

import 'package:protobuf/protobuf.dart' as $pb;

/// ---------------------------------------------------------------------------
/// Capabilities & version negotiation
/// ---------------------------------------------------------------------------
class Capability extends $pb.ProtobufEnum {
  static const Capability CAPABILITY_UNSPECIFIED =
      Capability._(0, _omitEnumNames ? '' : 'CAPABILITY_UNSPECIFIED');
  static const Capability CAPABILITY_CLIPBOARD =
      Capability._(1, _omitEnumNames ? '' : 'CAPABILITY_CLIPBOARD');
  static const Capability CAPABILITY_FILES =
      Capability._(2, _omitEnumNames ? '' : 'CAPABILITY_FILES');
  static const Capability CAPABILITY_NOTIFICATIONS =
      Capability._(3, _omitEnumNames ? '' : 'CAPABILITY_NOTIFICATIONS');
  static const Capability CAPABILITY_SCREEN =
      Capability._(4, _omitEnumNames ? '' : 'CAPABILITY_SCREEN');
  static const Capability CAPABILITY_INPUT =
      Capability._(5, _omitEnumNames ? '' : 'CAPABILITY_INPUT');
  static const Capability CAPABILITY_DEVICE_STATUS =
      Capability._(6, _omitEnumNames ? '' : 'CAPABILITY_DEVICE_STATUS');

  static const $core.List<Capability> values = <Capability>[
    CAPABILITY_UNSPECIFIED,
    CAPABILITY_CLIPBOARD,
    CAPABILITY_FILES,
    CAPABILITY_NOTIFICATIONS,
    CAPABILITY_SCREEN,
    CAPABILITY_INPUT,
    CAPABILITY_DEVICE_STATUS,
  ];

  static final $core.List<Capability?> _byValue =
      $pb.ProtobufEnum.$_initByValueList(values, 6);
  static Capability? valueOf($core.int value) =>
      value < 0 || value >= _byValue.length ? null : _byValue[value];

  const Capability._(super.value, super.name);
}

/// ---------------------------------------------------------------------------
/// Error model
/// ---------------------------------------------------------------------------
class Code extends $pb.ProtobufEnum {
  static const Code CODE_UNSPECIFIED =
      Code._(0, _omitEnumNames ? '' : 'CODE_UNSPECIFIED');
  static const Code CODE_OK = Code._(1, _omitEnumNames ? '' : 'CODE_OK');
  static const Code CODE_INVALID_ARGUMENT =
      Code._(2, _omitEnumNames ? '' : 'CODE_INVALID_ARGUMENT');
  static const Code CODE_UNAUTHENTICATED =
      Code._(3, _omitEnumNames ? '' : 'CODE_UNAUTHENTICATED');
  static const Code CODE_PERMISSION_DENIED =
      Code._(4, _omitEnumNames ? '' : 'CODE_PERMISSION_DENIED');
  static const Code CODE_NOT_FOUND =
      Code._(5, _omitEnumNames ? '' : 'CODE_NOT_FOUND');
  static const Code CODE_ALREADY_EXISTS =
      Code._(6, _omitEnumNames ? '' : 'CODE_ALREADY_EXISTS');
  static const Code CODE_INCOMPATIBLE_VERSION =
      Code._(7, _omitEnumNames ? '' : 'CODE_INCOMPATIBLE_VERSION');
  static const Code CODE_RESOURCE_EXHAUSTED =
      Code._(8, _omitEnumNames ? '' : 'CODE_RESOURCE_EXHAUSTED');
  static const Code CODE_INTERNAL =
      Code._(9, _omitEnumNames ? '' : 'CODE_INTERNAL');
  static const Code CODE_UNAVAILABLE =
      Code._(10, _omitEnumNames ? '' : 'CODE_UNAVAILABLE');

  /// Session negotiation failures (DEC-022). Appended, never renumbered:
  /// existing values are wire contracts.
  /// The requested MediaParams cannot be satisfied by the capture device.
  static const Code CODE_UNSUPPORTED_MEDIA_PARAMS =
      Code._(11, _omitEnumNames ? '' : 'CODE_UNSUPPORTED_MEDIA_PARAMS');

  /// The user (or the platform) withdrew MediaProjection consent mid-session.
  /// Distinct from a transport failure: the link is healthy and there is
  /// simply no longer a screen to send.
  static const Code CODE_CONSENT_REVOKED =
      Code._(12, _omitEnumNames ? '' : 'CODE_CONSENT_REVOKED');

  /// Capture or encode failed on the sending device.
  static const Code CODE_CAPTURE_FAILED =
      Code._(13, _omitEnumNames ? '' : 'CODE_CAPTURE_FAILED');

  /// The transport (ICE/DTLS/signaling) failed.
  static const Code CODE_TRANSPORT_FAILED =
      Code._(14, _omitEnumNames ? '' : 'CODE_TRANSPORT_FAILED');

  /// The bounded reconnect window elapsed without the session recovering.
  static const Code CODE_RECONNECT_TIMEOUT =
      Code._(15, _omitEnumNames ? '' : 'CODE_RECONNECT_TIMEOUT');

  /// The device already has an active session.
  static const Code CODE_SESSION_BUSY =
      Code._(16, _omitEnumNames ? '' : 'CODE_SESSION_BUSY');

  /// File-transfer failures (DEC-024). Appended, never renumbered.
  /// The transfer DataChannel or session went away mid-transfer. Restart from
  /// zero after reconnect: Phase 4 has no resume (see DEC-024).
  static const Code CODE_TRANSFER_INTERRUPTED =
      Code._(17, _omitEnumNames ? '' : 'CODE_TRANSFER_INTERRUPTED');

  /// The received bytes did not match the declared size or SHA-256 digest.
  static const Code CODE_CHECKSUM_MISMATCH =
      Code._(18, _omitEnumNames ? '' : 'CODE_CHECKSUM_MISMATCH');

  /// The receiver could not write or promote the file (no space, permissions,
  /// MediaStore/SAF failure).
  static const Code CODE_STORAGE_FAILED =
      Code._(19, _omitEnumNames ? '' : 'CODE_STORAGE_FAILED');

  /// The file exceeds the receiving side's file-size policy.
  static const Code CODE_FILE_TOO_LARGE =
      Code._(20, _omitEnumNames ? '' : 'CODE_FILE_TOO_LARGE');

  /// The proposed filename is not a plain basename (separator, NUL, "..",
  /// over-length). The receiver never accepts a sender-controlled path.
  static const Code CODE_UNSAFE_FILENAME =
      Code._(21, _omitEnumNames ? '' : 'CODE_UNSAFE_FILENAME');

  /// The receiver already has a transfer in flight (DEC-024 allows one inbound
  /// and one outbound transfer per session).
  static const Code CODE_TRANSFER_BUSY =
      Code._(22, _omitEnumNames ? '' : 'CODE_TRANSFER_BUSY');

  /// The local user cancelled the transfer.
  static const Code CODE_TRANSFER_CANCELLED =
      Code._(23, _omitEnumNames ? '' : 'CODE_TRANSFER_CANCELLED');

  static const $core.List<Code> values = <Code>[
    CODE_UNSPECIFIED,
    CODE_OK,
    CODE_INVALID_ARGUMENT,
    CODE_UNAUTHENTICATED,
    CODE_PERMISSION_DENIED,
    CODE_NOT_FOUND,
    CODE_ALREADY_EXISTS,
    CODE_INCOMPATIBLE_VERSION,
    CODE_RESOURCE_EXHAUSTED,
    CODE_INTERNAL,
    CODE_UNAVAILABLE,
    CODE_UNSUPPORTED_MEDIA_PARAMS,
    CODE_CONSENT_REVOKED,
    CODE_CAPTURE_FAILED,
    CODE_TRANSPORT_FAILED,
    CODE_RECONNECT_TIMEOUT,
    CODE_SESSION_BUSY,
    CODE_TRANSFER_INTERRUPTED,
    CODE_CHECKSUM_MISMATCH,
    CODE_STORAGE_FAILED,
    CODE_FILE_TOO_LARGE,
    CODE_UNSAFE_FILENAME,
    CODE_TRANSFER_BUSY,
    CODE_TRANSFER_CANCELLED,
  ];

  static final $core.List<Code?> _byValue =
      $pb.ProtobufEnum.$_initByValueList(values, 23);
  static Code? valueOf($core.int value) =>
      value < 0 || value >= _byValue.length ? null : _byValue[value];

  const Code._(super.value, super.name);
}

class TouchEvent_Action extends $pb.ProtobufEnum {
  static const TouchEvent_Action ACTION_UNSPECIFIED =
      TouchEvent_Action._(0, _omitEnumNames ? '' : 'ACTION_UNSPECIFIED');
  static const TouchEvent_Action ACTION_DOWN =
      TouchEvent_Action._(1, _omitEnumNames ? '' : 'ACTION_DOWN');
  static const TouchEvent_Action ACTION_MOVE =
      TouchEvent_Action._(2, _omitEnumNames ? '' : 'ACTION_MOVE');
  static const TouchEvent_Action ACTION_UP =
      TouchEvent_Action._(3, _omitEnumNames ? '' : 'ACTION_UP');
  static const TouchEvent_Action ACTION_CANCEL =
      TouchEvent_Action._(4, _omitEnumNames ? '' : 'ACTION_CANCEL');

  static const $core.List<TouchEvent_Action> values = <TouchEvent_Action>[
    ACTION_UNSPECIFIED,
    ACTION_DOWN,
    ACTION_MOVE,
    ACTION_UP,
    ACTION_CANCEL,
  ];

  static final $core.List<TouchEvent_Action?> _byValue =
      $pb.ProtobufEnum.$_initByValueList(values, 4);
  static TouchEvent_Action? valueOf($core.int value) =>
      value < 0 || value >= _byValue.length ? null : _byValue[value];

  const TouchEvent_Action._(super.value, super.name);
}

class KeyEvent_Action extends $pb.ProtobufEnum {
  static const KeyEvent_Action ACTION_UNSPECIFIED =
      KeyEvent_Action._(0, _omitEnumNames ? '' : 'ACTION_UNSPECIFIED');
  static const KeyEvent_Action ACTION_DOWN =
      KeyEvent_Action._(1, _omitEnumNames ? '' : 'ACTION_DOWN');
  static const KeyEvent_Action ACTION_UP =
      KeyEvent_Action._(2, _omitEnumNames ? '' : 'ACTION_UP');

  static const $core.List<KeyEvent_Action> values = <KeyEvent_Action>[
    ACTION_UNSPECIFIED,
    ACTION_DOWN,
    ACTION_UP,
  ];

  static final $core.List<KeyEvent_Action?> _byValue =
      $pb.ProtobufEnum.$_initByValueList(values, 2);
  static KeyEvent_Action? valueOf($core.int value) =>
      value < 0 || value >= _byValue.length ? null : _byValue[value];

  const KeyEvent_Action._(super.value, super.name);
}

class GlobalActionEvent_Type extends $pb.ProtobufEnum {
  static const GlobalActionEvent_Type TYPE_UNSPECIFIED =
      GlobalActionEvent_Type._(0, _omitEnumNames ? '' : 'TYPE_UNSPECIFIED');
  static const GlobalActionEvent_Type TYPE_GLOBAL_ACTION_BACK =
      GlobalActionEvent_Type._(
          1, _omitEnumNames ? '' : 'TYPE_GLOBAL_ACTION_BACK');
  static const GlobalActionEvent_Type TYPE_GLOBAL_ACTION_HOME =
      GlobalActionEvent_Type._(
          2, _omitEnumNames ? '' : 'TYPE_GLOBAL_ACTION_HOME');
  static const GlobalActionEvent_Type TYPE_GLOBAL_ACTION_RECENTS =
      GlobalActionEvent_Type._(
          3, _omitEnumNames ? '' : 'TYPE_GLOBAL_ACTION_RECENTS');
  static const GlobalActionEvent_Type TYPE_GLOBAL_ACTION_NOTIFICATIONS =
      GlobalActionEvent_Type._(
          4, _omitEnumNames ? '' : 'TYPE_GLOBAL_ACTION_NOTIFICATIONS');
  static const GlobalActionEvent_Type TYPE_GLOBAL_ACTION_QUICK_SETTINGS =
      GlobalActionEvent_Type._(
          5, _omitEnumNames ? '' : 'TYPE_GLOBAL_ACTION_QUICK_SETTINGS');

  static const $core.List<GlobalActionEvent_Type> values =
      <GlobalActionEvent_Type>[
    TYPE_UNSPECIFIED,
    TYPE_GLOBAL_ACTION_BACK,
    TYPE_GLOBAL_ACTION_HOME,
    TYPE_GLOBAL_ACTION_RECENTS,
    TYPE_GLOBAL_ACTION_NOTIFICATIONS,
    TYPE_GLOBAL_ACTION_QUICK_SETTINGS,
  ];

  static final $core.List<GlobalActionEvent_Type?> _byValue =
      $pb.ProtobufEnum.$_initByValueList(values, 5);
  static GlobalActionEvent_Type? valueOf($core.int value) =>
      value < 0 || value >= _byValue.length ? null : _byValue[value];

  const GlobalActionEvent_Type._(super.value, super.name);
}

const $core.bool _omitEnumNames =
    $core.bool.fromEnvironment('protobuf.omit_enum_names');
