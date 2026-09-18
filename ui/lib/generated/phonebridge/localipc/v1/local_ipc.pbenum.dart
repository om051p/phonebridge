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

const $core.bool _omitEnumNames =
    $core.bool.fromEnvironment('protobuf.omit_enum_names');
