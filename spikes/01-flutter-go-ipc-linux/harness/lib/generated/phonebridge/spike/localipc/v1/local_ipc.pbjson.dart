// This is a generated file - do not edit.
//
// Generated from phonebridge/spike/localipc/v1/local_ipc.proto.

// @dart = 3.3

// ignore_for_file: annotate_overrides, camel_case_types, comment_references
// ignore_for_file: constant_identifier_names
// ignore_for_file: curly_braces_in_flow_control_structures
// ignore_for_file: deprecated_member_use_from_same_package, library_prefixes
// ignore_for_file: non_constant_identifier_names, prefer_relative_imports
// ignore_for_file: unused_import

import 'dart:convert' as $convert;
import 'dart:core' as $core;
import 'dart:typed_data' as $typed_data;

@$core.Deprecated('Use pingRequestDescriptor instead')
const PingRequest$json = {
  '1': 'PingRequest',
  '2': [
    {'1': 'nonce', '3': 1, '4': 1, '5': 4, '10': 'nonce'},
    {'1': 'payload', '3': 2, '4': 1, '5': 12, '10': 'payload'},
  ],
};

/// Descriptor for `PingRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List pingRequestDescriptor = $convert.base64Decode(
    'CgtQaW5nUmVxdWVzdBIUCgVub25jZRgBIAEoBFIFbm9uY2USGAoHcGF5bG9hZBgCIAEoDFIHcG'
    'F5bG9hZA==');

@$core.Deprecated('Use pingResponseDescriptor instead')
const PingResponse$json = {
  '1': 'PingResponse',
  '2': [
    {'1': 'nonce', '3': 1, '4': 1, '5': 4, '10': 'nonce'},
    {'1': 'server_version', '3': 2, '4': 1, '5': 9, '10': 'serverVersion'},
    {'1': 'payload', '3': 3, '4': 1, '5': 12, '10': 'payload'},
    {
      '1': 'server_recv_unix_nanos',
      '3': 4,
      '4': 1,
      '5': 4,
      '10': 'serverRecvUnixNanos'
    },
  ],
};

/// Descriptor for `PingResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List pingResponseDescriptor = $convert.base64Decode(
    'CgxQaW5nUmVzcG9uc2USFAoFbm9uY2UYASABKARSBW5vbmNlEiUKDnNlcnZlcl92ZXJzaW9uGA'
    'IgASgJUg1zZXJ2ZXJWZXJzaW9uEhgKB3BheWxvYWQYAyABKAxSB3BheWxvYWQSMwoWc2VydmVy'
    'X3JlY3ZfdW5peF9uYW5vcxgEIAEoBFITc2VydmVyUmVjdlVuaXhOYW5vcw==');

@$core.Deprecated('Use subscribeRequestDescriptor instead')
const SubscribeRequest$json = {
  '1': 'SubscribeRequest',
  '2': [
    {'1': 'count', '3': 1, '4': 1, '5': 13, '10': 'count'},
    {'1': 'interval_ms', '3': 2, '4': 1, '5': 13, '10': 'intervalMs'},
    {'1': 'payload_size', '3': 3, '4': 1, '5': 13, '10': 'payloadSize'},
  ],
};

/// Descriptor for `SubscribeRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List subscribeRequestDescriptor = $convert.base64Decode(
    'ChBTdWJzY3JpYmVSZXF1ZXN0EhQKBWNvdW50GAEgASgNUgVjb3VudBIfCgtpbnRlcnZhbF9tcx'
    'gCIAEoDVIKaW50ZXJ2YWxNcxIhCgxwYXlsb2FkX3NpemUYAyABKA1SC3BheWxvYWRTaXpl');

@$core.Deprecated('Use subscribeResponseDescriptor instead')
const SubscribeResponse$json = {
  '1': 'SubscribeResponse',
  '2': [
    {'1': 'seq', '3': 1, '4': 1, '5': 4, '10': 'seq'},
    {'1': 'kind', '3': 2, '4': 1, '5': 9, '10': 'kind'},
    {
      '1': 'emitted_unix_nanos',
      '3': 3,
      '4': 1,
      '5': 4,
      '10': 'emittedUnixNanos'
    },
    {'1': 'payload', '3': 4, '4': 1, '5': 12, '10': 'payload'},
  ],
};

/// Descriptor for `SubscribeResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List subscribeResponseDescriptor = $convert.base64Decode(
    'ChFTdWJzY3JpYmVSZXNwb25zZRIQCgNzZXEYASABKARSA3NlcRISCgRraW5kGAIgASgJUgRraW'
    '5kEiwKEmVtaXR0ZWRfdW5peF9uYW5vcxgDIAEoBFIQZW1pdHRlZFVuaXhOYW5vcxIYCgdwYXls'
    'b2FkGAQgASgMUgdwYXlsb2Fk');

@$core.Deprecated('Use whoAmIRequestDescriptor instead')
const WhoAmIRequest$json = {
  '1': 'WhoAmIRequest',
};

/// Descriptor for `WhoAmIRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List whoAmIRequestDescriptor =
    $convert.base64Decode('Cg1XaG9BbUlSZXF1ZXN0');

@$core.Deprecated('Use whoAmIResponseDescriptor instead')
const WhoAmIResponse$json = {
  '1': 'WhoAmIResponse',
  '2': [
    {'1': 'peer_uid', '3': 1, '4': 1, '5': 13, '10': 'peerUid'},
    {'1': 'peer_pid', '3': 2, '4': 1, '5': 5, '10': 'peerPid'},
    {'1': 'peer_gid', '3': 3, '4': 1, '5': 13, '10': 'peerGid'},
    {'1': 'socket_path', '3': 4, '4': 1, '5': 9, '10': 'socketPath'},
    {'1': 'server_uid', '3': 5, '4': 1, '5': 13, '10': 'serverUid'},
    {'1': 'token_required', '3': 6, '4': 1, '5': 8, '10': 'tokenRequired'},
  ],
};

/// Descriptor for `WhoAmIResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List whoAmIResponseDescriptor = $convert.base64Decode(
    'Cg5XaG9BbUlSZXNwb25zZRIZCghwZWVyX3VpZBgBIAEoDVIHcGVlclVpZBIZCghwZWVyX3BpZB'
    'gCIAEoBVIHcGVlclBpZBIZCghwZWVyX2dpZBgDIAEoDVIHcGVlckdpZBIfCgtzb2NrZXRfcGF0'
    'aBgEIAEoCVIKc29ja2V0UGF0aBIdCgpzZXJ2ZXJfdWlkGAUgASgNUglzZXJ2ZXJVaWQSJQoOdG'
    '9rZW5fcmVxdWlyZWQYBiABKAhSDXRva2VuUmVxdWlyZWQ=');
