// This is a generated file - do not edit.
//
// Generated from phonebridge/localipc/v1/local_ipc.proto.

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

@$core.Deprecated('Use sessionStateDescriptor instead')
const SessionState$json = {
  '1': 'SessionState',
  '2': [
    {'1': 'SESSION_STATE_UNSPECIFIED', '2': 0},
    {'1': 'SESSION_STATE_DISCONNECTED', '2': 1},
    {'1': 'SESSION_STATE_DISCOVERING', '2': 2},
    {'1': 'SESSION_STATE_CONNECTING', '2': 3},
    {'1': 'SESSION_STATE_CONNECTED', '2': 4},
    {'1': 'SESSION_STATE_STREAMING', '2': 5},
    {'1': 'SESSION_STATE_RECONNECTING', '2': 6},
    {'1': 'SESSION_STATE_STOPPED', '2': 7},
    {'1': 'SESSION_STATE_FAILED', '2': 8},
  ],
};

/// Descriptor for `SessionState`. Decode as a `google.protobuf.EnumDescriptorProto`.
final $typed_data.Uint8List sessionStateDescriptor = $convert.base64Decode(
    'CgxTZXNzaW9uU3RhdGUSHQoZU0VTU0lPTl9TVEFURV9VTlNQRUNJRklFRBAAEh4KGlNFU1NJT0'
    '5fU1RBVEVfRElTQ09OTkVDVEVEEAESHQoZU0VTU0lPTl9TVEFURV9ESVNDT1ZFUklORxACEhwK'
    'GFNFU1NJT05fU1RBVEVfQ09OTkVDVElORxADEhsKF1NFU1NJT05fU1RBVEVfQ09OTkVDVEVEEA'
    'QSGwoXU0VTU0lPTl9TVEFURV9TVFJFQU1JTkcQBRIeChpTRVNTSU9OX1NUQVRFX1JFQ09OTkVD'
    'VElORxAGEhkKFVNFU1NJT05fU1RBVEVfU1RPUFBFRBAHEhgKFFNFU1NJT05fU1RBVEVfRkFJTE'
    'VEEAg=');

@$core.Deprecated('Use sessionReasonDescriptor instead')
const SessionReason$json = {
  '1': 'SessionReason',
  '2': [
    {'1': 'SESSION_REASON_UNSPECIFIED', '2': 0},
    {'1': 'SESSION_REASON_NONE', '2': 1},
    {'1': 'SESSION_REASON_PROTOCOL_VERSION_MISMATCH', '2': 2},
    {'1': 'SESSION_REASON_UNSUPPORTED_MEDIA_PARAMS', '2': 3},
    {'1': 'SESSION_REASON_DEVICE_NOT_TRUSTED', '2': 4},
    {'1': 'SESSION_REASON_DEVICE_NOT_FOUND', '2': 5},
    {'1': 'SESSION_REASON_SESSION_BUSY', '2': 6},
    {'1': 'SESSION_REASON_CONSENT_REVOKED', '2': 7},
    {'1': 'SESSION_REASON_CAPTURE_FAILED', '2': 8},
    {'1': 'SESSION_REASON_TRANSPORT_FAILED', '2': 9},
    {'1': 'SESSION_REASON_RECONNECT_TIMEOUT', '2': 10},
    {'1': 'SESSION_REASON_SIGNALING_FAILED', '2': 11},
    {'1': 'SESSION_REASON_USER_STOPPED', '2': 12},
  ],
};

/// Descriptor for `SessionReason`. Decode as a `google.protobuf.EnumDescriptorProto`.
final $typed_data.Uint8List sessionReasonDescriptor = $convert.base64Decode(
    'Cg1TZXNzaW9uUmVhc29uEh4KGlNFU1NJT05fUkVBU09OX1VOU1BFQ0lGSUVEEAASFwoTU0VTU0'
    'lPTl9SRUFTT05fTk9ORRABEiwKKFNFU1NJT05fUkVBU09OX1BST1RPQ09MX1ZFUlNJT05fTUlT'
    'TUFUQ0gQAhIrCidTRVNTSU9OX1JFQVNPTl9VTlNVUFBPUlRFRF9NRURJQV9QQVJBTVMQAxIlCi'
    'FTRVNTSU9OX1JFQVNPTl9ERVZJQ0VfTk9UX1RSVVNURUQQBBIjCh9TRVNTSU9OX1JFQVNPTl9E'
    'RVZJQ0VfTk9UX0ZPVU5EEAUSHwobU0VTU0lPTl9SRUFTT05fU0VTU0lPTl9CVVNZEAYSIgoeU0'
    'VTU0lPTl9SRUFTT05fQ09OU0VOVF9SRVZPS0VEEAcSIQodU0VTU0lPTl9SRUFTT05fQ0FQVFVS'
    'RV9GQUlMRUQQCBIjCh9TRVNTSU9OX1JFQVNPTl9UUkFOU1BPUlRfRkFJTEVEEAkSJAogU0VTU0'
    'lPTl9SRUFTT05fUkVDT05ORUNUX1RJTUVPVVQQChIjCh9TRVNTSU9OX1JFQVNPTl9TSUdOQUxJ'
    'TkdfRkFJTEVEEAsSHwobU0VTU0lPTl9SRUFTT05fVVNFUl9TVE9QUEVEEAw=');

@$core.Deprecated('Use sinkKindDescriptor instead')
const SinkKind$json = {
  '1': 'SinkKind',
  '2': [
    {'1': 'SINK_KIND_UNSPECIFIED', '2': 0},
    {'1': 'SINK_KIND_NULL', '2': 1},
    {'1': 'SINK_KIND_DISPLAY', '2': 2},
    {'1': 'SINK_KIND_PIPE', '2': 3},
    {'1': 'SINK_KIND_FILE', '2': 4},
  ],
};

/// Descriptor for `SinkKind`. Decode as a `google.protobuf.EnumDescriptorProto`.
final $typed_data.Uint8List sinkKindDescriptor = $convert.base64Decode(
    'CghTaW5rS2luZBIZChVTSU5LX0tJTkRfVU5TUEVDSUZJRUQQABISCg5TSU5LX0tJTkRfTlVMTB'
    'ABEhUKEVNJTktfS0lORF9ESVNQTEFZEAISEgoOU0lOS19LSU5EX1BJUEUQAxISCg5TSU5LX0tJ'
    'TkRfRklMRRAE');

@$core.Deprecated('Use transferDirectionDescriptor instead')
const TransferDirection$json = {
  '1': 'TransferDirection',
  '2': [
    {'1': 'TRANSFER_DIRECTION_UNSPECIFIED', '2': 0},
    {'1': 'TRANSFER_DIRECTION_OUTBOUND', '2': 1},
    {'1': 'TRANSFER_DIRECTION_INBOUND', '2': 2},
  ],
};

/// Descriptor for `TransferDirection`. Decode as a `google.protobuf.EnumDescriptorProto`.
final $typed_data.Uint8List transferDirectionDescriptor = $convert.base64Decode(
    'ChFUcmFuc2ZlckRpcmVjdGlvbhIiCh5UUkFOU0ZFUl9ESVJFQ1RJT05fVU5TUEVDSUZJRUQQAB'
    'IfChtUUkFOU0ZFUl9ESVJFQ1RJT05fT1VUQk9VTkQQARIeChpUUkFOU0ZFUl9ESVJFQ1RJT05f'
    'SU5CT1VORBAC');

@$core.Deprecated('Use transferStateDescriptor instead')
const TransferState$json = {
  '1': 'TransferState',
  '2': [
    {'1': 'TRANSFER_STATE_UNSPECIFIED', '2': 0},
    {'1': 'TRANSFER_STATE_PENDING', '2': 1},
    {'1': 'TRANSFER_STATE_ACTIVE', '2': 2},
    {'1': 'TRANSFER_STATE_VERIFYING', '2': 3},
    {'1': 'TRANSFER_STATE_COMPLETE', '2': 4},
    {'1': 'TRANSFER_STATE_CANCELLED', '2': 5},
    {'1': 'TRANSFER_STATE_FAILED', '2': 6},
  ],
};

/// Descriptor for `TransferState`. Decode as a `google.protobuf.EnumDescriptorProto`.
final $typed_data.Uint8List transferStateDescriptor = $convert.base64Decode(
    'Cg1UcmFuc2ZlclN0YXRlEh4KGlRSQU5TRkVSX1NUQVRFX1VOU1BFQ0lGSUVEEAASGgoWVFJBTl'
    'NGRVJfU1RBVEVfUEVORElORxABEhkKFVRSQU5TRkVSX1NUQVRFX0FDVElWRRACEhwKGFRSQU5T'
    'RkVSX1NUQVRFX1ZFUklGWUlORxADEhsKF1RSQU5TRkVSX1NUQVRFX0NPTVBMRVRFEAQSHAoYVF'
    'JBTlNGRVJfU1RBVEVfQ0FOQ0VMTEVEEAUSGQoVVFJBTlNGRVJfU1RBVEVfRkFJTEVEEAY=');

@$core.Deprecated('Use transferReasonDescriptor instead')
const TransferReason$json = {
  '1': 'TransferReason',
  '2': [
    {'1': 'TRANSFER_REASON_UNSPECIFIED', '2': 0},
    {'1': 'TRANSFER_REASON_NONE', '2': 1},
    {'1': 'TRANSFER_REASON_NO_SESSION', '2': 2},
    {'1': 'TRANSFER_REASON_UNSUPPORTED_PEER', '2': 3},
    {'1': 'TRANSFER_REASON_BUSY', '2': 4},
    {'1': 'TRANSFER_REASON_UNSAFE_FILENAME', '2': 5},
    {'1': 'TRANSFER_REASON_TOO_LARGE', '2': 6},
    {'1': 'TRANSFER_REASON_CHECKSUM_MISMATCH', '2': 7},
    {'1': 'TRANSFER_REASON_STORAGE_FAILED', '2': 8},
    {'1': 'TRANSFER_REASON_INTERRUPTED', '2': 9},
    {'1': 'TRANSFER_REASON_CANCELLED_BY_PEER', '2': 10},
    {'1': 'TRANSFER_REASON_CANCELLED_BY_USER', '2': 11},
    {'1': 'TRANSFER_REASON_PROTOCOL_ERROR', '2': 12},
    {'1': 'TRANSFER_REASON_INCOMPATIBLE_VERSION', '2': 13},
  ],
};

/// Descriptor for `TransferReason`. Decode as a `google.protobuf.EnumDescriptorProto`.
final $typed_data.Uint8List transferReasonDescriptor = $convert.base64Decode(
    'Cg5UcmFuc2ZlclJlYXNvbhIfChtUUkFOU0ZFUl9SRUFTT05fVU5TUEVDSUZJRUQQABIYChRUUk'
    'FOU0ZFUl9SRUFTT05fTk9ORRABEh4KGlRSQU5TRkVSX1JFQVNPTl9OT19TRVNTSU9OEAISJAog'
    'VFJBTlNGRVJfUkVBU09OX1VOU1VQUE9SVEVEX1BFRVIQAxIYChRUUkFOU0ZFUl9SRUFTT05fQl'
    'VTWRAEEiMKH1RSQU5TRkVSX1JFQVNPTl9VTlNBRkVfRklMRU5BTUUQBRIdChlUUkFOU0ZFUl9S'
    'RUFTT05fVE9PX0xBUkdFEAYSJQohVFJBTlNGRVJfUkVBU09OX0NIRUNLU1VNX01JU01BVENIEA'
    'cSIgoeVFJBTlNGRVJfUkVBU09OX1NUT1JBR0VfRkFJTEVEEAgSHwobVFJBTlNGRVJfUkVBU09O'
    'X0lOVEVSUlVQVEVEEAkSJQohVFJBTlNGRVJfUkVBU09OX0NBTkNFTExFRF9CWV9QRUVSEAoSJQ'
    'ohVFJBTlNGRVJfUkVBU09OX0NBTkNFTExFRF9CWV9VU0VSEAsSIgoeVFJBTlNGRVJfUkVBU09O'
    'X1BST1RPQ09MX0VSUk9SEAwSKAokVFJBTlNGRVJfUkVBU09OX0lOQ09NUEFUSUJMRV9WRVJTSU'
    '9OEA0=');

@$core.Deprecated('Use handshakeRequestDescriptor instead')
const HandshakeRequest$json = {
  '1': 'HandshakeRequest',
  '2': [
    {'1': 'client_version', '3': 1, '4': 1, '5': 13, '10': 'clientVersion'},
  ],
};

/// Descriptor for `HandshakeRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List handshakeRequestDescriptor = $convert.base64Decode(
    'ChBIYW5kc2hha2VSZXF1ZXN0EiUKDmNsaWVudF92ZXJzaW9uGAEgASgNUg1jbGllbnRWZXJzaW'
    '9u');

@$core.Deprecated('Use handshakeResponseDescriptor instead')
const HandshakeResponse$json = {
  '1': 'HandshakeResponse',
  '2': [
    {
      '1': 'negotiated_version',
      '3': 1,
      '4': 1,
      '5': 13,
      '10': 'negotiatedVersion'
    },
    {'1': 'server_version', '3': 2, '4': 1, '5': 9, '10': 'serverVersion'},
    {
      '1': 'daemon_generation',
      '3': 3,
      '4': 1,
      '5': 4,
      '10': 'daemonGeneration'
    },
  ],
};

/// Descriptor for `HandshakeResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List handshakeResponseDescriptor = $convert.base64Decode(
    'ChFIYW5kc2hha2VSZXNwb25zZRItChJuZWdvdGlhdGVkX3ZlcnNpb24YASABKA1SEW5lZ290aW'
    'F0ZWRWZXJzaW9uEiUKDnNlcnZlcl92ZXJzaW9uGAIgASgJUg1zZXJ2ZXJWZXJzaW9uEisKEWRh'
    'ZW1vbl9nZW5lcmF0aW9uGAMgASgEUhBkYWVtb25HZW5lcmF0aW9u');

@$core.Deprecated('Use pingRequestDescriptor instead')
const PingRequest$json = {
  '1': 'PingRequest',
  '2': [
    {'1': 'nonce', '3': 1, '4': 1, '5': 4, '10': 'nonce'},
  ],
};

/// Descriptor for `PingRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List pingRequestDescriptor =
    $convert.base64Decode('CgtQaW5nUmVxdWVzdBIUCgVub25jZRgBIAEoBFIFbm9uY2U=');

@$core.Deprecated('Use pingResponseDescriptor instead')
const PingResponse$json = {
  '1': 'PingResponse',
  '2': [
    {'1': 'nonce', '3': 1, '4': 1, '5': 4, '10': 'nonce'},
    {'1': 'server_version', '3': 2, '4': 1, '5': 9, '10': 'serverVersion'},
  ],
};

/// Descriptor for `PingResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List pingResponseDescriptor = $convert.base64Decode(
    'CgxQaW5nUmVzcG9uc2USFAoFbm9uY2UYASABKARSBW5vbmNlEiUKDnNlcnZlcl92ZXJzaW9uGA'
    'IgASgJUg1zZXJ2ZXJWZXJzaW9u');

@$core.Deprecated('Use streamEventsRequestDescriptor instead')
const StreamEventsRequest$json = {
  '1': 'StreamEventsRequest',
};

/// Descriptor for `StreamEventsRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List streamEventsRequestDescriptor =
    $convert.base64Decode('ChNTdHJlYW1FdmVudHNSZXF1ZXN0');

@$core.Deprecated('Use streamEventsResponseDescriptor instead')
const StreamEventsResponse$json = {
  '1': 'StreamEventsResponse',
  '2': [
    {'1': 'seq', '3': 1, '4': 1, '5': 4, '10': 'seq'},
    {
      '1': 'daemon_generation',
      '3': 2,
      '4': 1,
      '5': 4,
      '10': 'daemonGeneration'
    },
    {
      '1': 'envelope',
      '3': 3,
      '4': 1,
      '5': 11,
      '6': '.phonebridge.v1.Envelope',
      '10': 'envelope'
    },
    {
      '1': 'session_event',
      '3': 4,
      '4': 1,
      '5': 11,
      '6': '.phonebridge.localipc.v1.SessionEvent',
      '10': 'sessionEvent'
    },
    {
      '1': 'clipboard_event',
      '3': 5,
      '4': 1,
      '5': 11,
      '6': '.phonebridge.localipc.v1.ClipboardStatusEvent',
      '10': 'clipboardEvent'
    },
    {
      '1': 'transfer_event',
      '3': 6,
      '4': 1,
      '5': 11,
      '6': '.phonebridge.localipc.v1.TransferEvent',
      '10': 'transferEvent'
    },
  ],
};

/// Descriptor for `StreamEventsResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List streamEventsResponseDescriptor = $convert.base64Decode(
    'ChRTdHJlYW1FdmVudHNSZXNwb25zZRIQCgNzZXEYASABKARSA3NlcRIrChFkYWVtb25fZ2VuZX'
    'JhdGlvbhgCIAEoBFIQZGFlbW9uR2VuZXJhdGlvbhI0CghlbnZlbG9wZRgDIAEoCzIYLnBob25l'
    'YnJpZGdlLnYxLkVudmVsb3BlUghlbnZlbG9wZRJKCg1zZXNzaW9uX2V2ZW50GAQgASgLMiUucG'
    'hvbmVicmlkZ2UubG9jYWxpcGMudjEuU2Vzc2lvbkV2ZW50UgxzZXNzaW9uRXZlbnQSVgoPY2xp'
    'cGJvYXJkX2V2ZW50GAUgASgLMi0ucGhvbmVicmlkZ2UubG9jYWxpcGMudjEuQ2xpcGJvYXJkU3'
    'RhdHVzRXZlbnRSDmNsaXBib2FyZEV2ZW50Ek0KDnRyYW5zZmVyX2V2ZW50GAYgASgLMiYucGhv'
    'bmVicmlkZ2UubG9jYWxpcGMudjEuVHJhbnNmZXJFdmVudFINdHJhbnNmZXJFdmVudA==');

@$core.Deprecated('Use localEventDescriptor instead')
const LocalEvent$json = {
  '1': 'LocalEvent',
  '2': [
    {'1': 'seq', '3': 1, '4': 1, '5': 4, '10': 'seq'},
    {
      '1': 'daemon_generation',
      '3': 2,
      '4': 1,
      '5': 4,
      '10': 'daemonGeneration'
    },
    {
      '1': 'envelope',
      '3': 3,
      '4': 1,
      '5': 11,
      '6': '.phonebridge.v1.Envelope',
      '10': 'envelope'
    },
    {
      '1': 'session_event',
      '3': 4,
      '4': 1,
      '5': 11,
      '6': '.phonebridge.localipc.v1.SessionEvent',
      '10': 'sessionEvent'
    },
    {
      '1': 'clipboard_event',
      '3': 5,
      '4': 1,
      '5': 11,
      '6': '.phonebridge.localipc.v1.ClipboardStatusEvent',
      '10': 'clipboardEvent'
    },
    {
      '1': 'transfer_event',
      '3': 6,
      '4': 1,
      '5': 11,
      '6': '.phonebridge.localipc.v1.TransferEvent',
      '10': 'transferEvent'
    },
  ],
};

/// Descriptor for `LocalEvent`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List localEventDescriptor = $convert.base64Decode(
    'CgpMb2NhbEV2ZW50EhAKA3NlcRgBIAEoBFIDc2VxEisKEWRhZW1vbl9nZW5lcmF0aW9uGAIgAS'
    'gEUhBkYWVtb25HZW5lcmF0aW9uEjQKCGVudmVsb3BlGAMgASgLMhgucGhvbmVicmlkZ2UudjEu'
    'RW52ZWxvcGVSCGVudmVsb3BlEkoKDXNlc3Npb25fZXZlbnQYBCABKAsyJS5waG9uZWJyaWRnZS'
    '5sb2NhbGlwYy52MS5TZXNzaW9uRXZlbnRSDHNlc3Npb25FdmVudBJWCg9jbGlwYm9hcmRfZXZl'
    'bnQYBSABKAsyLS5waG9uZWJyaWRnZS5sb2NhbGlwYy52MS5DbGlwYm9hcmRTdGF0dXNFdmVudF'
    'IOY2xpcGJvYXJkRXZlbnQSTQoOdHJhbnNmZXJfZXZlbnQYBiABKAsyJi5waG9uZWJyaWRnZS5s'
    'b2NhbGlwYy52MS5UcmFuc2ZlckV2ZW50Ug10cmFuc2ZlckV2ZW50');

@$core.Deprecated('Use healthRequestDescriptor instead')
const HealthRequest$json = {
  '1': 'HealthRequest',
};

/// Descriptor for `HealthRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List healthRequestDescriptor =
    $convert.base64Decode('Cg1IZWFsdGhSZXF1ZXN0');

@$core.Deprecated('Use healthResponseDescriptor instead')
const HealthResponse$json = {
  '1': 'HealthResponse',
  '2': [
    {'1': 'ready', '3': 1, '4': 1, '5': 8, '10': 'ready'},
    {
      '1': 'daemon_generation',
      '3': 2,
      '4': 1,
      '5': 4,
      '10': 'daemonGeneration'
    },
    {'1': 'server_version', '3': 3, '4': 1, '5': 9, '10': 'serverVersion'},
    {'1': 'uptime_ms', '3': 4, '4': 1, '5': 4, '10': 'uptimeMs'},
  ],
};

/// Descriptor for `HealthResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List healthResponseDescriptor = $convert.base64Decode(
    'Cg5IZWFsdGhSZXNwb25zZRIUCgVyZWFkeRgBIAEoCFIFcmVhZHkSKwoRZGFlbW9uX2dlbmVyYX'
    'Rpb24YAiABKARSEGRhZW1vbkdlbmVyYXRpb24SJQoOc2VydmVyX3ZlcnNpb24YAyABKAlSDXNl'
    'cnZlclZlcnNpb24SGwoJdXB0aW1lX21zGAQgASgEUgh1cHRpbWVNcw==');

@$core.Deprecated('Use sessionEventDescriptor instead')
const SessionEvent$json = {
  '1': 'SessionEvent',
  '2': [
    {'1': 'session_id', '3': 1, '4': 1, '5': 9, '10': 'sessionId'},
    {
      '1': 'state',
      '3': 2,
      '4': 1,
      '5': 14,
      '6': '.phonebridge.localipc.v1.SessionState',
      '10': 'state'
    },
    {'1': 'reason', '3': 3, '4': 1, '5': 9, '10': 'reason'},
    {'1': 'error_message', '3': 4, '4': 1, '5': 9, '10': 'errorMessage'},
    {
      '1': 'reason_code',
      '3': 5,
      '4': 1,
      '5': 14,
      '6': '.phonebridge.localipc.v1.SessionReason',
      '10': 'reasonCode'
    },
  ],
};

/// Descriptor for `SessionEvent`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List sessionEventDescriptor = $convert.base64Decode(
    'CgxTZXNzaW9uRXZlbnQSHQoKc2Vzc2lvbl9pZBgBIAEoCVIJc2Vzc2lvbklkEjsKBXN0YXRlGA'
    'IgASgOMiUucGhvbmVicmlkZ2UubG9jYWxpcGMudjEuU2Vzc2lvblN0YXRlUgVzdGF0ZRIWCgZy'
    'ZWFzb24YAyABKAlSBnJlYXNvbhIjCg1lcnJvcl9tZXNzYWdlGAQgASgJUgxlcnJvck1lc3NhZ2'
    'USRwoLcmVhc29uX2NvZGUYBSABKA4yJi5waG9uZWJyaWRnZS5sb2NhbGlwYy52MS5TZXNzaW9u'
    'UmVhc29uUgpyZWFzb25Db2Rl');

@$core.Deprecated('Use startSessionRequestDescriptor instead')
const StartSessionRequest$json = {
  '1': 'StartSessionRequest',
  '2': [
    {'1': 'device_id', '3': 1, '4': 1, '5': 9, '10': 'deviceId'},
    {
      '1': 'requested',
      '3': 2,
      '4': 1,
      '5': 11,
      '6': '.phonebridge.v1.MediaParams',
      '10': 'requested'
    },
  ],
};

/// Descriptor for `StartSessionRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List startSessionRequestDescriptor = $convert.base64Decode(
    'ChNTdGFydFNlc3Npb25SZXF1ZXN0EhsKCWRldmljZV9pZBgBIAEoCVIIZGV2aWNlSWQSOQoJcm'
    'VxdWVzdGVkGAIgASgLMhsucGhvbmVicmlkZ2UudjEuTWVkaWFQYXJhbXNSCXJlcXVlc3RlZA==');

@$core.Deprecated('Use startSessionResponseDescriptor instead')
const StartSessionResponse$json = {
  '1': 'StartSessionResponse',
  '2': [
    {'1': 'session_id', '3': 1, '4': 1, '5': 9, '10': 'sessionId'},
    {
      '1': 'state',
      '3': 2,
      '4': 1,
      '5': 14,
      '6': '.phonebridge.localipc.v1.SessionState',
      '10': 'state'
    },
  ],
};

/// Descriptor for `StartSessionResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List startSessionResponseDescriptor = $convert.base64Decode(
    'ChRTdGFydFNlc3Npb25SZXNwb25zZRIdCgpzZXNzaW9uX2lkGAEgASgJUglzZXNzaW9uSWQSOw'
    'oFc3RhdGUYAiABKA4yJS5waG9uZWJyaWRnZS5sb2NhbGlwYy52MS5TZXNzaW9uU3RhdGVSBXN0'
    'YXRl');

@$core.Deprecated('Use stopSessionRequestDescriptor instead')
const StopSessionRequest$json = {
  '1': 'StopSessionRequest',
  '2': [
    {'1': 'session_id', '3': 1, '4': 1, '5': 9, '10': 'sessionId'},
    {'1': 'reason', '3': 2, '4': 1, '5': 9, '10': 'reason'},
  ],
};

/// Descriptor for `StopSessionRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List stopSessionRequestDescriptor = $convert.base64Decode(
    'ChJTdG9wU2Vzc2lvblJlcXVlc3QSHQoKc2Vzc2lvbl9pZBgBIAEoCVIJc2Vzc2lvbklkEhYKBn'
    'JlYXNvbhgCIAEoCVIGcmVhc29u');

@$core.Deprecated('Use stopSessionResponseDescriptor instead')
const StopSessionResponse$json = {
  '1': 'StopSessionResponse',
  '2': [
    {'1': 'session_id', '3': 1, '4': 1, '5': 9, '10': 'sessionId'},
    {
      '1': 'state',
      '3': 2,
      '4': 1,
      '5': 14,
      '6': '.phonebridge.localipc.v1.SessionState',
      '10': 'state'
    },
  ],
};

/// Descriptor for `StopSessionResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List stopSessionResponseDescriptor = $convert.base64Decode(
    'ChNTdG9wU2Vzc2lvblJlc3BvbnNlEh0KCnNlc3Npb25faWQYASABKAlSCXNlc3Npb25JZBI7Cg'
    'VzdGF0ZRgCIAEoDjIlLnBob25lYnJpZGdlLmxvY2FsaXBjLnYxLlNlc3Npb25TdGF0ZVIFc3Rh'
    'dGU=');

@$core.Deprecated('Use getSessionStateRequestDescriptor instead')
const GetSessionStateRequest$json = {
  '1': 'GetSessionStateRequest',
  '2': [
    {'1': 'session_id', '3': 1, '4': 1, '5': 9, '10': 'sessionId'},
  ],
};

/// Descriptor for `GetSessionStateRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List getSessionStateRequestDescriptor =
    $convert.base64Decode(
        'ChZHZXRTZXNzaW9uU3RhdGVSZXF1ZXN0Eh0KCnNlc3Npb25faWQYASABKAlSCXNlc3Npb25JZA'
        '==');

@$core.Deprecated('Use streamStatsDescriptor instead')
const StreamStats$json = {
  '1': 'StreamStats',
  '2': [
    {'1': 'packets', '3': 1, '4': 1, '5': 4, '10': 'packets'},
    {'1': 'bytes_rtp', '3': 2, '4': 1, '5': 4, '10': 'bytesRtp'},
    {'1': 'bytes_h264', '3': 3, '4': 1, '5': 4, '10': 'bytesH264'},
    {'1': 'access_units', '3': 4, '4': 1, '5': 4, '10': 'accessUnits'},
    {'1': 'keyframes', '3': 5, '4': 1, '5': 4, '10': 'keyframes'},
    {'1': 'dropped_aus', '3': 6, '4': 1, '5': 3, '10': 'droppedAus'},
    {'1': 'seq_gaps', '3': 7, '4': 1, '5': 4, '10': 'seqGaps'},
    {'1': 'dup_seq', '3': 8, '4': 1, '5': 4, '10': 'dupSeq'},
    {'1': 'late_packets', '3': 9, '4': 1, '5': 4, '10': 'latePackets'},
    {'1': 'ts_backward', '3': 10, '4': 1, '5': 4, '10': 'tsBackward'},
  ],
};

/// Descriptor for `StreamStats`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List streamStatsDescriptor = $convert.base64Decode(
    'CgtTdHJlYW1TdGF0cxIYCgdwYWNrZXRzGAEgASgEUgdwYWNrZXRzEhsKCWJ5dGVzX3J0cBgCIA'
    'EoBFIIYnl0ZXNSdHASHQoKYnl0ZXNfaDI2NBgDIAEoBFIJYnl0ZXNIMjY0EiEKDGFjY2Vzc191'
    'bml0cxgEIAEoBFILYWNjZXNzVW5pdHMSHAoJa2V5ZnJhbWVzGAUgASgEUglrZXlmcmFtZXMSHw'
    'oLZHJvcHBlZF9hdXMYBiABKANSCmRyb3BwZWRBdXMSGQoIc2VxX2dhcHMYByABKARSB3NlcUdh'
    'cHMSFwoHZHVwX3NlcRgIIAEoBFIGZHVwU2VxEiEKDGxhdGVfcGFja2V0cxgJIAEoBFILbGF0ZV'
    'BhY2tldHMSHwoLdHNfYmFja3dhcmQYCiABKARSCnRzQmFja3dhcmQ=');

@$core.Deprecated('Use getSessionStateResponseDescriptor instead')
const GetSessionStateResponse$json = {
  '1': 'GetSessionStateResponse',
  '2': [
    {'1': 'session_id', '3': 1, '4': 1, '5': 9, '10': 'sessionId'},
    {
      '1': 'state',
      '3': 2,
      '4': 1,
      '5': 14,
      '6': '.phonebridge.localipc.v1.SessionState',
      '10': 'state'
    },
    {'1': 'device_id', '3': 3, '4': 1, '5': 9, '10': 'deviceId'},
    {
      '1': 'connected_duration_ms',
      '3': 4,
      '4': 1,
      '5': 4,
      '10': 'connectedDurationMs'
    },
    {'1': 'error_message', '3': 5, '4': 1, '5': 9, '10': 'errorMessage'},
    {
      '1': 'stats',
      '3': 6,
      '4': 1,
      '5': 11,
      '6': '.phonebridge.localipc.v1.StreamStats',
      '10': 'stats'
    },
    {
      '1': 'requested',
      '3': 7,
      '4': 1,
      '5': 11,
      '6': '.phonebridge.v1.MediaParams',
      '10': 'requested'
    },
    {
      '1': 'actual',
      '3': 8,
      '4': 1,
      '5': 11,
      '6': '.phonebridge.v1.MediaParams',
      '10': 'actual'
    },
    {
      '1': 'reason_code',
      '3': 9,
      '4': 1,
      '5': 14,
      '6': '.phonebridge.localipc.v1.SessionReason',
      '10': 'reasonCode'
    },
    {
      '1': 'reconnect_attempts',
      '3': 10,
      '4': 1,
      '5': 13,
      '10': 'reconnectAttempts'
    },
    {
      '1': 'sink_kind',
      '3': 11,
      '4': 1,
      '5': 14,
      '6': '.phonebridge.localipc.v1.SinkKind',
      '10': 'sinkKind'
    },
    {'1': 'sink_active', '3': 12, '4': 1, '5': 8, '10': 'sinkActive'},
    {'1': 'frames_reason', '3': 13, '4': 1, '5': 9, '10': 'framesReason'},
  ],
};

/// Descriptor for `GetSessionStateResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List getSessionStateResponseDescriptor = $convert.base64Decode(
    'ChdHZXRTZXNzaW9uU3RhdGVSZXNwb25zZRIdCgpzZXNzaW9uX2lkGAEgASgJUglzZXNzaW9uSW'
    'QSOwoFc3RhdGUYAiABKA4yJS5waG9uZWJyaWRnZS5sb2NhbGlwYy52MS5TZXNzaW9uU3RhdGVS'
    'BXN0YXRlEhsKCWRldmljZV9pZBgDIAEoCVIIZGV2aWNlSWQSMgoVY29ubmVjdGVkX2R1cmF0aW'
    '9uX21zGAQgASgEUhNjb25uZWN0ZWREdXJhdGlvbk1zEiMKDWVycm9yX21lc3NhZ2UYBSABKAlS'
    'DGVycm9yTWVzc2FnZRI6CgVzdGF0cxgGIAEoCzIkLnBob25lYnJpZGdlLmxvY2FsaXBjLnYxLl'
    'N0cmVhbVN0YXRzUgVzdGF0cxI5CglyZXF1ZXN0ZWQYByABKAsyGy5waG9uZWJyaWRnZS52MS5N'
    'ZWRpYVBhcmFtc1IJcmVxdWVzdGVkEjMKBmFjdHVhbBgIIAEoCzIbLnBob25lYnJpZGdlLnYxLk'
    '1lZGlhUGFyYW1zUgZhY3R1YWwSRwoLcmVhc29uX2NvZGUYCSABKA4yJi5waG9uZWJyaWRnZS5s'
    'b2NhbGlwYy52MS5TZXNzaW9uUmVhc29uUgpyZWFzb25Db2RlEi0KEnJlY29ubmVjdF9hdHRlbX'
    'B0cxgKIAEoDVIRcmVjb25uZWN0QXR0ZW1wdHMSPgoJc2lua19raW5kGAsgASgOMiEucGhvbmVi'
    'cmlkZ2UubG9jYWxpcGMudjEuU2lua0tpbmRSCHNpbmtLaW5kEh8KC3NpbmtfYWN0aXZlGAwgAS'
    'gIUgpzaW5rQWN0aXZlEiMKDWZyYW1lc19yZWFzb24YDSABKAlSDGZyYW1lc1JlYXNvbg==');

@$core.Deprecated('Use discoveredDeviceDescriptor instead')
const DiscoveredDevice$json = {
  '1': 'DiscoveredDevice',
  '2': [
    {'1': 'id', '3': 1, '4': 1, '5': 9, '10': 'id'},
    {'1': 'name', '3': 2, '4': 1, '5': 9, '10': 'name'},
    {'1': 'model', '3': 3, '4': 1, '5': 9, '10': 'model'},
    {'1': 'version', '3': 4, '4': 1, '5': 9, '10': 'version'},
    {'1': 'capabilities', '3': 5, '4': 3, '5': 9, '10': 'capabilities'},
    {'1': 'state', '3': 6, '4': 1, '5': 9, '10': 'state'},
    {'1': 'address', '3': 7, '4': 1, '5': 9, '10': 'address'},
    {'1': 'port', '3': 8, '4': 1, '5': 13, '10': 'port'},
    {'1': 'is_stale', '3': 9, '4': 1, '5': 8, '10': 'isStale'},
  ],
};

/// Descriptor for `DiscoveredDevice`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List discoveredDeviceDescriptor = $convert.base64Decode(
    'ChBEaXNjb3ZlcmVkRGV2aWNlEg4KAmlkGAEgASgJUgJpZBISCgRuYW1lGAIgASgJUgRuYW1lEh'
    'QKBW1vZGVsGAMgASgJUgVtb2RlbBIYCgd2ZXJzaW9uGAQgASgJUgd2ZXJzaW9uEiIKDGNhcGFi'
    'aWxpdGllcxgFIAMoCVIMY2FwYWJpbGl0aWVzEhQKBXN0YXRlGAYgASgJUgVzdGF0ZRIYCgdhZG'
    'RyZXNzGAcgASgJUgdhZGRyZXNzEhIKBHBvcnQYCCABKA1SBHBvcnQSGQoIaXNfc3RhbGUYCSAB'
    'KAhSB2lzU3RhbGU=');

@$core.Deprecated('Use listDevicesRequestDescriptor instead')
const ListDevicesRequest$json = {
  '1': 'ListDevicesRequest',
};

/// Descriptor for `ListDevicesRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List listDevicesRequestDescriptor =
    $convert.base64Decode('ChJMaXN0RGV2aWNlc1JlcXVlc3Q=');

@$core.Deprecated('Use listDevicesResponseDescriptor instead')
const ListDevicesResponse$json = {
  '1': 'ListDevicesResponse',
  '2': [
    {
      '1': 'devices',
      '3': 1,
      '4': 3,
      '5': 11,
      '6': '.phonebridge.localipc.v1.DiscoveredDevice',
      '10': 'devices'
    },
  ],
};

/// Descriptor for `ListDevicesResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List listDevicesResponseDescriptor = $convert.base64Decode(
    'ChNMaXN0RGV2aWNlc1Jlc3BvbnNlEkMKB2RldmljZXMYASADKAsyKS5waG9uZWJyaWRnZS5sb2'
    'NhbGlwYy52MS5EaXNjb3ZlcmVkRGV2aWNlUgdkZXZpY2Vz');

@$core.Deprecated('Use pairDeviceRequestDescriptor instead')
const PairDeviceRequest$json = {
  '1': 'PairDeviceRequest',
  '2': [
    {'1': 'device_id', '3': 1, '4': 1, '5': 9, '10': 'deviceId'},
  ],
};

/// Descriptor for `PairDeviceRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List pairDeviceRequestDescriptor = $convert.base64Decode(
    'ChFQYWlyRGV2aWNlUmVxdWVzdBIbCglkZXZpY2VfaWQYASABKAlSCGRldmljZUlk');

@$core.Deprecated('Use pairDeviceResponseDescriptor instead')
const PairDeviceResponse$json = {
  '1': 'PairDeviceResponse',
  '2': [
    {'1': 'device_id', '3': 1, '4': 1, '5': 9, '10': 'deviceId'},
    {'1': 'display_name', '3': 2, '4': 1, '5': 9, '10': 'displayName'},
    {'1': 'sas', '3': 3, '4': 1, '5': 9, '10': 'sas'},
    {
      '1': 'pending_confirmation',
      '3': 4,
      '4': 1,
      '5': 8,
      '10': 'pendingConfirmation'
    },
  ],
};

/// Descriptor for `PairDeviceResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List pairDeviceResponseDescriptor = $convert.base64Decode(
    'ChJQYWlyRGV2aWNlUmVzcG9uc2USGwoJZGV2aWNlX2lkGAEgASgJUghkZXZpY2VJZBIhCgxkaX'
    'NwbGF5X25hbWUYAiABKAlSC2Rpc3BsYXlOYW1lEhAKA3NhcxgDIAEoCVIDc2FzEjEKFHBlbmRp'
    'bmdfY29uZmlybWF0aW9uGAQgASgIUhNwZW5kaW5nQ29uZmlybWF0aW9u');

@$core.Deprecated('Use confirmPairingRequestDescriptor instead')
const ConfirmPairingRequest$json = {
  '1': 'ConfirmPairingRequest',
  '2': [
    {'1': 'device_id', '3': 1, '4': 1, '5': 9, '10': 'deviceId'},
    {'1': 'user_confirmed', '3': 2, '4': 1, '5': 8, '10': 'userConfirmed'},
  ],
};

/// Descriptor for `ConfirmPairingRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List confirmPairingRequestDescriptor = $convert.base64Decode(
    'ChVDb25maXJtUGFpcmluZ1JlcXVlc3QSGwoJZGV2aWNlX2lkGAEgASgJUghkZXZpY2VJZBIlCg'
    '51c2VyX2NvbmZpcm1lZBgCIAEoCFINdXNlckNvbmZpcm1lZA==');

@$core.Deprecated('Use confirmPairingResponseDescriptor instead')
const ConfirmPairingResponse$json = {
  '1': 'ConfirmPairingResponse',
  '2': [
    {'1': 'device_id', '3': 1, '4': 1, '5': 9, '10': 'deviceId'},
    {'1': 'success', '3': 2, '4': 1, '5': 8, '10': 'success'},
    {'1': 'error_message', '3': 3, '4': 1, '5': 9, '10': 'errorMessage'},
  ],
};

/// Descriptor for `ConfirmPairingResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List confirmPairingResponseDescriptor = $convert.base64Decode(
    'ChZDb25maXJtUGFpcmluZ1Jlc3BvbnNlEhsKCWRldmljZV9pZBgBIAEoCVIIZGV2aWNlSWQSGA'
    'oHc3VjY2VzcxgCIAEoCFIHc3VjY2VzcxIjCg1lcnJvcl9tZXNzYWdlGAMgASgJUgxlcnJvck1l'
    'c3NhZ2U=');

@$core.Deprecated('Use trustedDeviceDescriptor instead')
const TrustedDevice$json = {
  '1': 'TrustedDevice',
  '2': [
    {'1': 'device_id', '3': 1, '4': 1, '5': 9, '10': 'deviceId'},
    {'1': 'display_name', '3': 2, '4': 1, '5': 9, '10': 'displayName'},
    {'1': 'platform', '3': 3, '4': 1, '5': 9, '10': 'platform'},
    {'1': 'public_key', '3': 4, '4': 1, '5': 12, '10': 'publicKey'},
    {'1': 'paired_at_ms', '3': 5, '4': 1, '5': 3, '10': 'pairedAtMs'},
    {'1': 'last_seen_ms', '3': 6, '4': 1, '5': 3, '10': 'lastSeenMs'},
    {'1': 'revoked', '3': 7, '4': 1, '5': 8, '10': 'revoked'},
  ],
};

/// Descriptor for `TrustedDevice`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List trustedDeviceDescriptor = $convert.base64Decode(
    'Cg1UcnVzdGVkRGV2aWNlEhsKCWRldmljZV9pZBgBIAEoCVIIZGV2aWNlSWQSIQoMZGlzcGxheV'
    '9uYW1lGAIgASgJUgtkaXNwbGF5TmFtZRIaCghwbGF0Zm9ybRgDIAEoCVIIcGxhdGZvcm0SHQoK'
    'cHVibGljX2tleRgEIAEoDFIJcHVibGljS2V5EiAKDHBhaXJlZF9hdF9tcxgFIAEoA1IKcGFpcm'
    'VkQXRNcxIgCgxsYXN0X3NlZW5fbXMYBiABKANSCmxhc3RTZWVuTXMSGAoHcmV2b2tlZBgHIAEo'
    'CFIHcmV2b2tlZA==');

@$core.Deprecated('Use listTrustedDevicesRequestDescriptor instead')
const ListTrustedDevicesRequest$json = {
  '1': 'ListTrustedDevicesRequest',
};

/// Descriptor for `ListTrustedDevicesRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List listTrustedDevicesRequestDescriptor =
    $convert.base64Decode('ChlMaXN0VHJ1c3RlZERldmljZXNSZXF1ZXN0');

@$core.Deprecated('Use listTrustedDevicesResponseDescriptor instead')
const ListTrustedDevicesResponse$json = {
  '1': 'ListTrustedDevicesResponse',
  '2': [
    {
      '1': 'devices',
      '3': 1,
      '4': 3,
      '5': 11,
      '6': '.phonebridge.localipc.v1.TrustedDevice',
      '10': 'devices'
    },
  ],
};

/// Descriptor for `ListTrustedDevicesResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List listTrustedDevicesResponseDescriptor =
    $convert.base64Decode(
        'ChpMaXN0VHJ1c3RlZERldmljZXNSZXNwb25zZRJACgdkZXZpY2VzGAEgAygLMiYucGhvbmVicm'
        'lkZ2UubG9jYWxpcGMudjEuVHJ1c3RlZERldmljZVIHZGV2aWNlcw==');

@$core.Deprecated('Use revokeDeviceRequestDescriptor instead')
const RevokeDeviceRequest$json = {
  '1': 'RevokeDeviceRequest',
  '2': [
    {'1': 'device_id', '3': 1, '4': 1, '5': 9, '10': 'deviceId'},
  ],
};

/// Descriptor for `RevokeDeviceRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List revokeDeviceRequestDescriptor =
    $convert.base64Decode(
        'ChNSZXZva2VEZXZpY2VSZXF1ZXN0EhsKCWRldmljZV9pZBgBIAEoCVIIZGV2aWNlSWQ=');

@$core.Deprecated('Use revokeDeviceResponseDescriptor instead')
const RevokeDeviceResponse$json = {
  '1': 'RevokeDeviceResponse',
  '2': [
    {'1': 'device_id', '3': 1, '4': 1, '5': 9, '10': 'deviceId'},
    {'1': 'success', '3': 2, '4': 1, '5': 8, '10': 'success'},
  ],
};

/// Descriptor for `RevokeDeviceResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List revokeDeviceResponseDescriptor = $convert.base64Decode(
    'ChRSZXZva2VEZXZpY2VSZXNwb25zZRIbCglkZXZpY2VfaWQYASABKAlSCGRldmljZUlkEhgKB3'
    'N1Y2Nlc3MYAiABKAhSB3N1Y2Nlc3M=');

@$core.Deprecated('Use getClipboardStatusRequestDescriptor instead')
const GetClipboardStatusRequest$json = {
  '1': 'GetClipboardStatusRequest',
};

/// Descriptor for `GetClipboardStatusRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List getClipboardStatusRequestDescriptor =
    $convert.base64Decode('ChlHZXRDbGlwYm9hcmRTdGF0dXNSZXF1ZXN0');

@$core.Deprecated('Use getClipboardStatusResponseDescriptor instead')
const GetClipboardStatusResponse$json = {
  '1': 'GetClipboardStatusResponse',
  '2': [
    {'1': 'state', '3': 1, '4': 1, '5': 9, '10': 'state'},
    {'1': 'is_connected', '3': 2, '4': 1, '5': 8, '10': 'isConnected'},
    {'1': 'max_payload_size', '3': 3, '4': 1, '5': 13, '10': 'maxPayloadSize'},
    {'1': 'remote_peer_id', '3': 4, '4': 1, '5': 9, '10': 'remotePeerId'},
    {'1': 'adapter_status', '3': 5, '4': 1, '5': 9, '10': 'adapterStatus'},
    {'1': 'last_sync_ms', '3': 6, '4': 1, '5': 4, '10': 'lastSyncMs'},
  ],
};

/// Descriptor for `GetClipboardStatusResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List getClipboardStatusResponseDescriptor = $convert.base64Decode(
    'ChpHZXRDbGlwYm9hcmRTdGF0dXNSZXNwb25zZRIUCgVzdGF0ZRgBIAEoCVIFc3RhdGUSIQoMaX'
    'NfY29ubmVjdGVkGAIgASgIUgtpc0Nvbm5lY3RlZBIoChBtYXhfcGF5bG9hZF9zaXplGAMgASgN'
    'Ug5tYXhQYXlsb2FkU2l6ZRIkCg5yZW1vdGVfcGVlcl9pZBgEIAEoCVIMcmVtb3RlUGVlcklkEi'
    'UKDmFkYXB0ZXJfc3RhdHVzGAUgASgJUg1hZGFwdGVyU3RhdHVzEiAKDGxhc3Rfc3luY19tcxgG'
    'IAEoBFIKbGFzdFN5bmNNcw==');

@$core.Deprecated('Use triggerClipboardPullRequestDescriptor instead')
const TriggerClipboardPullRequest$json = {
  '1': 'TriggerClipboardPullRequest',
};

/// Descriptor for `TriggerClipboardPullRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List triggerClipboardPullRequestDescriptor =
    $convert.base64Decode('ChtUcmlnZ2VyQ2xpcGJvYXJkUHVsbFJlcXVlc3Q=');

@$core.Deprecated('Use triggerClipboardPullResponseDescriptor instead')
const TriggerClipboardPullResponse$json = {
  '1': 'TriggerClipboardPullResponse',
  '2': [
    {'1': 'success', '3': 1, '4': 1, '5': 8, '10': 'success'},
    {'1': 'error_message', '3': 2, '4': 1, '5': 9, '10': 'errorMessage'},
  ],
};

/// Descriptor for `TriggerClipboardPullResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List triggerClipboardPullResponseDescriptor =
    $convert.base64Decode(
        'ChxUcmlnZ2VyQ2xpcGJvYXJkUHVsbFJlc3BvbnNlEhgKB3N1Y2Nlc3MYASABKAhSB3N1Y2Nlc3'
        'MSIwoNZXJyb3JfbWVzc2FnZRgCIAEoCVIMZXJyb3JNZXNzYWdl');

@$core.Deprecated('Use clipboardStatusEventDescriptor instead')
const ClipboardStatusEvent$json = {
  '1': 'ClipboardStatusEvent',
  '2': [
    {'1': 'state', '3': 1, '4': 1, '5': 9, '10': 'state'},
    {'1': 'is_connected', '3': 2, '4': 1, '5': 8, '10': 'isConnected'},
    {'1': 'adapter_status', '3': 3, '4': 1, '5': 9, '10': 'adapterStatus'},
    {'1': 'remote_peer_id', '3': 4, '4': 1, '5': 9, '10': 'remotePeerId'},
    {'1': 'max_payload_size', '3': 5, '4': 1, '5': 13, '10': 'maxPayloadSize'},
  ],
};

/// Descriptor for `ClipboardStatusEvent`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clipboardStatusEventDescriptor = $convert.base64Decode(
    'ChRDbGlwYm9hcmRTdGF0dXNFdmVudBIUCgVzdGF0ZRgBIAEoCVIFc3RhdGUSIQoMaXNfY29ubm'
    'VjdGVkGAIgASgIUgtpc0Nvbm5lY3RlZBIlCg5hZGFwdGVyX3N0YXR1cxgDIAEoCVINYWRhcHRl'
    'clN0YXR1cxIkCg5yZW1vdGVfcGVlcl9pZBgEIAEoCVIMcmVtb3RlUGVlcklkEigKEG1heF9wYX'
    'lsb2FkX3NpemUYBSABKA1SDm1heFBheWxvYWRTaXpl');

@$core.Deprecated('Use transferInfoDescriptor instead')
const TransferInfo$json = {
  '1': 'TransferInfo',
  '2': [
    {'1': 'transfer_id', '3': 1, '4': 1, '5': 9, '10': 'transferId'},
    {
      '1': 'direction',
      '3': 2,
      '4': 1,
      '5': 14,
      '6': '.phonebridge.localipc.v1.TransferDirection',
      '10': 'direction'
    },
    {
      '1': 'state',
      '3': 3,
      '4': 1,
      '5': 14,
      '6': '.phonebridge.localipc.v1.TransferState',
      '10': 'state'
    },
    {'1': 'peer_device_id', '3': 4, '4': 1, '5': 9, '10': 'peerDeviceId'},
    {'1': 'filename', '3': 5, '4': 1, '5': 9, '10': 'filename'},
    {'1': 'mime_type', '3': 6, '4': 1, '5': 9, '10': 'mimeType'},
    {'1': 'size_bytes', '3': 7, '4': 1, '5': 4, '10': 'sizeBytes'},
    {
      '1': 'bytes_transferred',
      '3': 8,
      '4': 1,
      '5': 4,
      '10': 'bytesTransferred'
    },
    {'1': 'started_at_ms', '3': 9, '4': 1, '5': 4, '10': 'startedAtMs'},
    {'1': 'finished_at_ms', '3': 10, '4': 1, '5': 4, '10': 'finishedAtMs'},
    {
      '1': 'reason_code',
      '3': 11,
      '4': 1,
      '5': 14,
      '6': '.phonebridge.localipc.v1.TransferReason',
      '10': 'reasonCode'
    },
    {'1': 'error_message', '3': 12, '4': 1, '5': 9, '10': 'errorMessage'},
    {'1': 'saved_name', '3': 13, '4': 1, '5': 9, '10': 'savedName'},
  ],
};

/// Descriptor for `TransferInfo`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List transferInfoDescriptor = $convert.base64Decode(
    'CgxUcmFuc2ZlckluZm8SHwoLdHJhbnNmZXJfaWQYASABKAlSCnRyYW5zZmVySWQSSAoJZGlyZW'
    'N0aW9uGAIgASgOMioucGhvbmVicmlkZ2UubG9jYWxpcGMudjEuVHJhbnNmZXJEaXJlY3Rpb25S'
    'CWRpcmVjdGlvbhI8CgVzdGF0ZRgDIAEoDjImLnBob25lYnJpZGdlLmxvY2FsaXBjLnYxLlRyYW'
    '5zZmVyU3RhdGVSBXN0YXRlEiQKDnBlZXJfZGV2aWNlX2lkGAQgASgJUgxwZWVyRGV2aWNlSWQS'
    'GgoIZmlsZW5hbWUYBSABKAlSCGZpbGVuYW1lEhsKCW1pbWVfdHlwZRgGIAEoCVIIbWltZVR5cG'
    'USHQoKc2l6ZV9ieXRlcxgHIAEoBFIJc2l6ZUJ5dGVzEisKEWJ5dGVzX3RyYW5zZmVycmVkGAgg'
    'ASgEUhBieXRlc1RyYW5zZmVycmVkEiIKDXN0YXJ0ZWRfYXRfbXMYCSABKARSC3N0YXJ0ZWRBdE'
    '1zEiQKDmZpbmlzaGVkX2F0X21zGAogASgEUgxmaW5pc2hlZEF0TXMSSAoLcmVhc29uX2NvZGUY'
    'CyABKA4yJy5waG9uZWJyaWRnZS5sb2NhbGlwYy52MS5UcmFuc2ZlclJlYXNvblIKcmVhc29uQ2'
    '9kZRIjCg1lcnJvcl9tZXNzYWdlGAwgASgJUgxlcnJvck1lc3NhZ2USHQoKc2F2ZWRfbmFtZRgN'
    'IAEoCVIJc2F2ZWROYW1l');

@$core.Deprecated('Use sendFileRequestDescriptor instead')
const SendFileRequest$json = {
  '1': 'SendFileRequest',
  '2': [
    {'1': 'device_id', '3': 1, '4': 1, '5': 9, '10': 'deviceId'},
    {'1': 'local_path', '3': 2, '4': 1, '5': 9, '10': 'localPath'},
    {'1': 'filename', '3': 3, '4': 1, '5': 9, '10': 'filename'},
  ],
};

/// Descriptor for `SendFileRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List sendFileRequestDescriptor = $convert.base64Decode(
    'Cg9TZW5kRmlsZVJlcXVlc3QSGwoJZGV2aWNlX2lkGAEgASgJUghkZXZpY2VJZBIdCgpsb2NhbF'
    '9wYXRoGAIgASgJUglsb2NhbFBhdGgSGgoIZmlsZW5hbWUYAyABKAlSCGZpbGVuYW1l');

@$core.Deprecated('Use sendFileResponseDescriptor instead')
const SendFileResponse$json = {
  '1': 'SendFileResponse',
  '2': [
    {'1': 'transfer_id', '3': 1, '4': 1, '5': 9, '10': 'transferId'},
    {
      '1': 'state',
      '3': 2,
      '4': 1,
      '5': 14,
      '6': '.phonebridge.localipc.v1.TransferState',
      '10': 'state'
    },
    {
      '1': 'reason_code',
      '3': 3,
      '4': 1,
      '5': 14,
      '6': '.phonebridge.localipc.v1.TransferReason',
      '10': 'reasonCode'
    },
    {'1': 'error_message', '3': 4, '4': 1, '5': 9, '10': 'errorMessage'},
  ],
};

/// Descriptor for `SendFileResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List sendFileResponseDescriptor = $convert.base64Decode(
    'ChBTZW5kRmlsZVJlc3BvbnNlEh8KC3RyYW5zZmVyX2lkGAEgASgJUgp0cmFuc2ZlcklkEjwKBX'
    'N0YXRlGAIgASgOMiYucGhvbmVicmlkZ2UubG9jYWxpcGMudjEuVHJhbnNmZXJTdGF0ZVIFc3Rh'
    'dGUSSAoLcmVhc29uX2NvZGUYAyABKA4yJy5waG9uZWJyaWRnZS5sb2NhbGlwYy52MS5UcmFuc2'
    'ZlclJlYXNvblIKcmVhc29uQ29kZRIjCg1lcnJvcl9tZXNzYWdlGAQgASgJUgxlcnJvck1lc3Nh'
    'Z2U=');

@$core.Deprecated('Use cancelTransferRequestDescriptor instead')
const CancelTransferRequest$json = {
  '1': 'CancelTransferRequest',
  '2': [
    {'1': 'transfer_id', '3': 1, '4': 1, '5': 9, '10': 'transferId'},
  ],
};

/// Descriptor for `CancelTransferRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List cancelTransferRequestDescriptor = $convert.base64Decode(
    'ChVDYW5jZWxUcmFuc2ZlclJlcXVlc3QSHwoLdHJhbnNmZXJfaWQYASABKAlSCnRyYW5zZmVySW'
    'Q=');

@$core.Deprecated('Use cancelTransferResponseDescriptor instead')
const CancelTransferResponse$json = {
  '1': 'CancelTransferResponse',
  '2': [
    {'1': 'transfer_id', '3': 1, '4': 1, '5': 9, '10': 'transferId'},
    {'1': 'cancelled', '3': 2, '4': 1, '5': 8, '10': 'cancelled'},
    {'1': 'error_message', '3': 3, '4': 1, '5': 9, '10': 'errorMessage'},
  ],
};

/// Descriptor for `CancelTransferResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List cancelTransferResponseDescriptor = $convert.base64Decode(
    'ChZDYW5jZWxUcmFuc2ZlclJlc3BvbnNlEh8KC3RyYW5zZmVyX2lkGAEgASgJUgp0cmFuc2Zlck'
    'lkEhwKCWNhbmNlbGxlZBgCIAEoCFIJY2FuY2VsbGVkEiMKDWVycm9yX21lc3NhZ2UYAyABKAlS'
    'DGVycm9yTWVzc2FnZQ==');

@$core.Deprecated('Use listTransfersRequestDescriptor instead')
const ListTransfersRequest$json = {
  '1': 'ListTransfersRequest',
};

/// Descriptor for `ListTransfersRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List listTransfersRequestDescriptor =
    $convert.base64Decode('ChRMaXN0VHJhbnNmZXJzUmVxdWVzdA==');

@$core.Deprecated('Use listTransfersResponseDescriptor instead')
const ListTransfersResponse$json = {
  '1': 'ListTransfersResponse',
  '2': [
    {
      '1': 'transfers',
      '3': 1,
      '4': 3,
      '5': 11,
      '6': '.phonebridge.localipc.v1.TransferInfo',
      '10': 'transfers'
    },
  ],
};

/// Descriptor for `ListTransfersResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List listTransfersResponseDescriptor = $convert.base64Decode(
    'ChVMaXN0VHJhbnNmZXJzUmVzcG9uc2USQwoJdHJhbnNmZXJzGAEgAygLMiUucGhvbmVicmlkZ2'
    'UubG9jYWxpcGMudjEuVHJhbnNmZXJJbmZvUgl0cmFuc2ZlcnM=');

@$core.Deprecated('Use transferEventDescriptor instead')
const TransferEvent$json = {
  '1': 'TransferEvent',
  '2': [
    {
      '1': 'transfer',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.phonebridge.localipc.v1.TransferInfo',
      '10': 'transfer'
    },
  ],
};

/// Descriptor for `TransferEvent`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List transferEventDescriptor = $convert.base64Decode(
    'Cg1UcmFuc2ZlckV2ZW50EkEKCHRyYW5zZmVyGAEgASgLMiUucGhvbmVicmlkZ2UubG9jYWxpcG'
    'MudjEuVHJhbnNmZXJJbmZvUgh0cmFuc2Zlcg==');

@$core.Deprecated('Use streamFramesRequestDescriptor instead')
const StreamFramesRequest$json = {
  '1': 'StreamFramesRequest',
};

/// Descriptor for `StreamFramesRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List streamFramesRequestDescriptor =
    $convert.base64Decode('ChNTdHJlYW1GcmFtZXNSZXF1ZXN0');

@$core.Deprecated('Use streamFramesResponseDescriptor instead')
const StreamFramesResponse$json = {
  '1': 'StreamFramesResponse',
  '2': [
    {'1': 'frame_id', '3': 1, '4': 1, '5': 4, '10': 'frameId'},
    {'1': 'chunk_index', '3': 2, '4': 1, '5': 13, '10': 'chunkIndex'},
    {'1': 'chunk_count', '3': 3, '4': 1, '5': 13, '10': 'chunkCount'},
    {'1': 'last_chunk', '3': 4, '4': 1, '5': 8, '10': 'lastChunk'},
    {'1': 'width', '3': 5, '4': 1, '5': 13, '10': 'width'},
    {'1': 'height', '3': 6, '4': 1, '5': 13, '10': 'height'},
    {'1': 'jpeg', '3': 7, '4': 1, '5': 12, '10': 'jpeg'},
    {'1': 'sent_unix_micros', '3': 8, '4': 1, '5': 4, '10': 'sentUnixMicros'},
  ],
};

/// Descriptor for `StreamFramesResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List streamFramesResponseDescriptor = $convert.base64Decode(
    'ChRTdHJlYW1GcmFtZXNSZXNwb25zZRIZCghmcmFtZV9pZBgBIAEoBFIHZnJhbWVJZBIfCgtjaH'
    'Vua19pbmRleBgCIAEoDVIKY2h1bmtJbmRleBIfCgtjaHVua19jb3VudBgDIAEoDVIKY2h1bmtD'
    'b3VudBIdCgpsYXN0X2NodW5rGAQgASgIUglsYXN0Q2h1bmsSFAoFd2lkdGgYBSABKA1SBXdpZH'
    'RoEhYKBmhlaWdodBgGIAEoDVIGaGVpZ2h0EhIKBGpwZWcYByABKAxSBGpwZWcSKAoQc2VudF91'
    'bml4X21pY3JvcxgIIAEoBFIOc2VudFVuaXhNaWNyb3M=');
