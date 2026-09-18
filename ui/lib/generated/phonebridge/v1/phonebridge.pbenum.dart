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
  ];

  static final $core.List<Code?> _byValue =
      $pb.ProtobufEnum.$_initByValueList(values, 10);
  static Code? valueOf($core.int value) =>
      value < 0 || value >= _byValue.length ? null : _byValue[value];

  const Code._(super.value, super.name);
}

const $core.bool _omitEnumNames =
    $core.bool.fromEnvironment('protobuf.omit_enum_names');
