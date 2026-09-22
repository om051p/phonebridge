// This is a generated file - do not edit.
//
// Generated from phonebridge/v1/phonebridge.proto.

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

@$core.Deprecated('Use capabilityDescriptor instead')
const Capability$json = {
  '1': 'Capability',
  '2': [
    {'1': 'CAPABILITY_UNSPECIFIED', '2': 0},
    {'1': 'CAPABILITY_CLIPBOARD', '2': 1},
    {'1': 'CAPABILITY_FILES', '2': 2},
    {'1': 'CAPABILITY_NOTIFICATIONS', '2': 3},
    {'1': 'CAPABILITY_SCREEN', '2': 4},
    {'1': 'CAPABILITY_INPUT', '2': 5},
    {'1': 'CAPABILITY_DEVICE_STATUS', '2': 6},
  ],
};

/// Descriptor for `Capability`. Decode as a `google.protobuf.EnumDescriptorProto`.
final $typed_data.Uint8List capabilityDescriptor = $convert.base64Decode(
    'CgpDYXBhYmlsaXR5EhoKFkNBUEFCSUxJVFlfVU5TUEVDSUZJRUQQABIYChRDQVBBQklMSVRZX0'
    'NMSVBCT0FSRBABEhQKEENBUEFCSUxJVFlfRklMRVMQAhIcChhDQVBBQklMSVRZX05PVElGSUNB'
    'VElPTlMQAxIVChFDQVBBQklMSVRZX1NDUkVFThAEEhQKEENBUEFCSUxJVFlfSU5QVVQQBRIcCh'
    'hDQVBBQklMSVRZX0RFVklDRV9TVEFUVVMQBg==');

@$core.Deprecated('Use codeDescriptor instead')
const Code$json = {
  '1': 'Code',
  '2': [
    {'1': 'CODE_UNSPECIFIED', '2': 0},
    {'1': 'CODE_OK', '2': 1},
    {'1': 'CODE_INVALID_ARGUMENT', '2': 2},
    {'1': 'CODE_UNAUTHENTICATED', '2': 3},
    {'1': 'CODE_PERMISSION_DENIED', '2': 4},
    {'1': 'CODE_NOT_FOUND', '2': 5},
    {'1': 'CODE_ALREADY_EXISTS', '2': 6},
    {'1': 'CODE_INCOMPATIBLE_VERSION', '2': 7},
    {'1': 'CODE_RESOURCE_EXHAUSTED', '2': 8},
    {'1': 'CODE_INTERNAL', '2': 9},
    {'1': 'CODE_UNAVAILABLE', '2': 10},
    {'1': 'CODE_UNSUPPORTED_MEDIA_PARAMS', '2': 11},
    {'1': 'CODE_CONSENT_REVOKED', '2': 12},
    {'1': 'CODE_CAPTURE_FAILED', '2': 13},
    {'1': 'CODE_TRANSPORT_FAILED', '2': 14},
    {'1': 'CODE_RECONNECT_TIMEOUT', '2': 15},
    {'1': 'CODE_SESSION_BUSY', '2': 16},
    {'1': 'CODE_TRANSFER_INTERRUPTED', '2': 17},
    {'1': 'CODE_CHECKSUM_MISMATCH', '2': 18},
    {'1': 'CODE_STORAGE_FAILED', '2': 19},
    {'1': 'CODE_FILE_TOO_LARGE', '2': 20},
    {'1': 'CODE_UNSAFE_FILENAME', '2': 21},
    {'1': 'CODE_TRANSFER_BUSY', '2': 22},
    {'1': 'CODE_TRANSFER_CANCELLED', '2': 23},
  ],
};

/// Descriptor for `Code`. Decode as a `google.protobuf.EnumDescriptorProto`.
final $typed_data.Uint8List codeDescriptor = $convert.base64Decode(
    'CgRDb2RlEhQKEENPREVfVU5TUEVDSUZJRUQQABILCgdDT0RFX09LEAESGQoVQ09ERV9JTlZBTE'
    'lEX0FSR1VNRU5UEAISGAoUQ09ERV9VTkFVVEhFTlRJQ0FURUQQAxIaChZDT0RFX1BFUk1JU1NJ'
    'T05fREVOSUVEEAQSEgoOQ09ERV9OT1RfRk9VTkQQBRIXChNDT0RFX0FMUkVBRFlfRVhJU1RTEA'
    'YSHQoZQ09ERV9JTkNPTVBBVElCTEVfVkVSU0lPThAHEhsKF0NPREVfUkVTT1VSQ0VfRVhIQVVT'
    'VEVEEAgSEQoNQ09ERV9JTlRFUk5BTBAJEhQKEENPREVfVU5BVkFJTEFCTEUQChIhCh1DT0RFX1'
    'VOU1VQUE9SVEVEX01FRElBX1BBUkFNUxALEhgKFENPREVfQ09OU0VOVF9SRVZPS0VEEAwSFwoT'
    'Q09ERV9DQVBUVVJFX0ZBSUxFRBANEhkKFUNPREVfVFJBTlNQT1JUX0ZBSUxFRBAOEhoKFkNPRE'
    'VfUkVDT05ORUNUX1RJTUVPVVQQDxIVChFDT0RFX1NFU1NJT05fQlVTWRAQEh0KGUNPREVfVFJB'
    'TlNGRVJfSU5URVJSVVBURUQQERIaChZDT0RFX0NIRUNLU1VNX01JU01BVENIEBISFwoTQ09ERV'
    '9TVE9SQUdFX0ZBSUxFRBATEhcKE0NPREVfRklMRV9UT09fTEFSR0UQFBIYChRDT0RFX1VOU0FG'
    'RV9GSUxFTkFNRRAVEhYKEkNPREVfVFJBTlNGRVJfQlVTWRAWEhsKF0NPREVfVFJBTlNGRVJfQ0'
    'FOQ0VMTEVEEBc=');

@$core.Deprecated('Use envelopeDescriptor instead')
const Envelope$json = {
  '1': 'Envelope',
  '2': [
    {'1': 'version', '3': 1, '4': 1, '5': 13, '10': 'version'},
    {'1': 'device_id', '3': 2, '4': 1, '5': 9, '10': 'deviceId'},
    {'1': 'session_id', '3': 3, '4': 1, '5': 9, '10': 'sessionId'},
    {'1': 'sequence', '3': 4, '4': 1, '5': 4, '10': 'sequence'},
    {'1': 'timestamp_ms', '3': 5, '4': 1, '5': 4, '10': 'timestampMs'},
    {'1': 'nonce', '3': 6, '4': 1, '5': 12, '10': 'nonce'},
    {
      '1': 'capabilities',
      '3': 7,
      '4': 1,
      '5': 11,
      '6': '.phonebridge.v1.CapabilitySet',
      '10': 'capabilities'
    },
    {
      '1': 'ping',
      '3': 10,
      '4': 1,
      '5': 11,
      '6': '.phonebridge.v1.Ping',
      '9': 0,
      '10': 'ping'
    },
    {
      '1': 'pong',
      '3': 11,
      '4': 1,
      '5': 11,
      '6': '.phonebridge.v1.Pong',
      '9': 0,
      '10': 'pong'
    },
    {
      '1': 'device_hello',
      '3': 12,
      '4': 1,
      '5': 11,
      '6': '.phonebridge.v1.DeviceHello',
      '9': 0,
      '10': 'deviceHello'
    },
    {
      '1': 'pair_request',
      '3': 13,
      '4': 1,
      '5': 11,
      '6': '.phonebridge.v1.PairRequest',
      '9': 0,
      '10': 'pairRequest'
    },
    {
      '1': 'pair_accept',
      '3': 14,
      '4': 1,
      '5': 11,
      '6': '.phonebridge.v1.PairAccept',
      '9': 0,
      '10': 'pairAccept'
    },
    {
      '1': 'pair_reject',
      '3': 15,
      '4': 1,
      '5': 11,
      '6': '.phonebridge.v1.PairReject',
      '9': 0,
      '10': 'pairReject'
    },
    {
      '1': 'error',
      '3': 16,
      '4': 1,
      '5': 11,
      '6': '.phonebridge.v1.Error',
      '9': 0,
      '10': 'error'
    },
    {
      '1': 'clipboard_update',
      '3': 20,
      '4': 1,
      '5': 11,
      '6': '.phonebridge.v1.ClipboardUpdate',
      '9': 0,
      '10': 'clipboardUpdate'
    },
    {
      '1': 'notification_event',
      '3': 21,
      '4': 1,
      '5': 11,
      '6': '.phonebridge.v1.NotificationEvent',
      '9': 0,
      '10': 'notificationEvent'
    },
    {
      '1': 'notification_action',
      '3': 22,
      '4': 1,
      '5': 11,
      '6': '.phonebridge.v1.NotificationAction',
      '9': 0,
      '10': 'notificationAction'
    },
    {
      '1': 'notification_dismissed',
      '3': 23,
      '4': 1,
      '5': 11,
      '6': '.phonebridge.v1.NotificationDismissed',
      '9': 0,
      '10': 'notificationDismissed'
    },
    {
      '1': 'file_offer',
      '3': 24,
      '4': 1,
      '5': 11,
      '6': '.phonebridge.v1.FileOffer',
      '9': 0,
      '10': 'fileOffer'
    },
    {
      '1': 'file_accept',
      '3': 25,
      '4': 1,
      '5': 11,
      '6': '.phonebridge.v1.FileAccept',
      '9': 0,
      '10': 'fileAccept'
    },
    {
      '1': 'file_chunk',
      '3': 26,
      '4': 1,
      '5': 11,
      '6': '.phonebridge.v1.FileChunk',
      '9': 0,
      '10': 'fileChunk'
    },
    {
      '1': 'file_complete',
      '3': 27,
      '4': 1,
      '5': 11,
      '6': '.phonebridge.v1.FileComplete',
      '9': 0,
      '10': 'fileComplete'
    },
    {
      '1': 'file_cancel',
      '3': 28,
      '4': 1,
      '5': 11,
      '6': '.phonebridge.v1.FileCancel',
      '9': 0,
      '10': 'fileCancel'
    },
    {
      '1': 'screen_start',
      '3': 29,
      '4': 1,
      '5': 11,
      '6': '.phonebridge.v1.ScreenStart',
      '9': 0,
      '10': 'screenStart'
    },
    {
      '1': 'screen_stop',
      '3': 30,
      '4': 1,
      '5': 11,
      '6': '.phonebridge.v1.ScreenStop',
      '9': 0,
      '10': 'screenStop'
    },
    {
      '1': 'input_keyboard',
      '3': 31,
      '4': 1,
      '5': 11,
      '6': '.phonebridge.v1.InputKeyboard',
      '9': 0,
      '10': 'inputKeyboard'
    },
    {
      '1': 'input_mouse',
      '3': 32,
      '4': 1,
      '5': 11,
      '6': '.phonebridge.v1.InputMouse',
      '9': 0,
      '10': 'inputMouse'
    },
    {
      '1': 'input_touch',
      '3': 33,
      '4': 1,
      '5': 11,
      '6': '.phonebridge.v1.InputTouch',
      '9': 0,
      '10': 'inputTouch'
    },
    {
      '1': 'device_status',
      '3': 34,
      '4': 1,
      '5': 11,
      '6': '.phonebridge.v1.DeviceStatus',
      '9': 0,
      '10': 'deviceStatus'
    },
  ],
  '8': [
    {'1': 'payload'},
  ],
  '9': [
    {'1': 8, '2': 9},
    {'1': 9, '2': 10},
  ],
};

/// Descriptor for `Envelope`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List envelopeDescriptor = $convert.base64Decode(
    'CghFbnZlbG9wZRIYCgd2ZXJzaW9uGAEgASgNUgd2ZXJzaW9uEhsKCWRldmljZV9pZBgCIAEoCV'
    'IIZGV2aWNlSWQSHQoKc2Vzc2lvbl9pZBgDIAEoCVIJc2Vzc2lvbklkEhoKCHNlcXVlbmNlGAQg'
    'ASgEUghzZXF1ZW5jZRIhCgx0aW1lc3RhbXBfbXMYBSABKARSC3RpbWVzdGFtcE1zEhQKBW5vbm'
    'NlGAYgASgMUgVub25jZRJBCgxjYXBhYmlsaXRpZXMYByABKAsyHS5waG9uZWJyaWRnZS52MS5D'
    'YXBhYmlsaXR5U2V0UgxjYXBhYmlsaXRpZXMSKgoEcGluZxgKIAEoCzIULnBob25lYnJpZGdlLn'
    'YxLlBpbmdIAFIEcGluZxIqCgRwb25nGAsgASgLMhQucGhvbmVicmlkZ2UudjEuUG9uZ0gAUgRw'
    'b25nEkAKDGRldmljZV9oZWxsbxgMIAEoCzIbLnBob25lYnJpZGdlLnYxLkRldmljZUhlbGxvSA'
    'BSC2RldmljZUhlbGxvEkAKDHBhaXJfcmVxdWVzdBgNIAEoCzIbLnBob25lYnJpZGdlLnYxLlBh'
    'aXJSZXF1ZXN0SABSC3BhaXJSZXF1ZXN0Ej0KC3BhaXJfYWNjZXB0GA4gASgLMhoucGhvbmVicm'
    'lkZ2UudjEuUGFpckFjY2VwdEgAUgpwYWlyQWNjZXB0Ej0KC3BhaXJfcmVqZWN0GA8gASgLMhou'
    'cGhvbmVicmlkZ2UudjEuUGFpclJlamVjdEgAUgpwYWlyUmVqZWN0Ei0KBWVycm9yGBAgASgLMh'
    'UucGhvbmVicmlkZ2UudjEuRXJyb3JIAFIFZXJyb3ISTAoQY2xpcGJvYXJkX3VwZGF0ZRgUIAEo'
    'CzIfLnBob25lYnJpZGdlLnYxLkNsaXBib2FyZFVwZGF0ZUgAUg9jbGlwYm9hcmRVcGRhdGUSUg'
    'oSbm90aWZpY2F0aW9uX2V2ZW50GBUgASgLMiEucGhvbmVicmlkZ2UudjEuTm90aWZpY2F0aW9u'
    'RXZlbnRIAFIRbm90aWZpY2F0aW9uRXZlbnQSVQoTbm90aWZpY2F0aW9uX2FjdGlvbhgWIAEoCz'
    'IiLnBob25lYnJpZGdlLnYxLk5vdGlmaWNhdGlvbkFjdGlvbkgAUhJub3RpZmljYXRpb25BY3Rp'
    'b24SXgoWbm90aWZpY2F0aW9uX2Rpc21pc3NlZBgXIAEoCzIlLnBob25lYnJpZGdlLnYxLk5vdG'
    'lmaWNhdGlvbkRpc21pc3NlZEgAUhVub3RpZmljYXRpb25EaXNtaXNzZWQSOgoKZmlsZV9vZmZl'
    'chgYIAEoCzIZLnBob25lYnJpZGdlLnYxLkZpbGVPZmZlckgAUglmaWxlT2ZmZXISPQoLZmlsZV'
    '9hY2NlcHQYGSABKAsyGi5waG9uZWJyaWRnZS52MS5GaWxlQWNjZXB0SABSCmZpbGVBY2NlcHQS'
    'OgoKZmlsZV9jaHVuaxgaIAEoCzIZLnBob25lYnJpZGdlLnYxLkZpbGVDaHVua0gAUglmaWxlQ2'
    'h1bmsSQwoNZmlsZV9jb21wbGV0ZRgbIAEoCzIcLnBob25lYnJpZGdlLnYxLkZpbGVDb21wbGV0'
    'ZUgAUgxmaWxlQ29tcGxldGUSPQoLZmlsZV9jYW5jZWwYHCABKAsyGi5waG9uZWJyaWRnZS52MS'
    '5GaWxlQ2FuY2VsSABSCmZpbGVDYW5jZWwSQAoMc2NyZWVuX3N0YXJ0GB0gASgLMhsucGhvbmVi'
    'cmlkZ2UudjEuU2NyZWVuU3RhcnRIAFILc2NyZWVuU3RhcnQSPQoLc2NyZWVuX3N0b3AYHiABKA'
    'syGi5waG9uZWJyaWRnZS52MS5TY3JlZW5TdG9wSABSCnNjcmVlblN0b3ASRgoOaW5wdXRfa2V5'
    'Ym9hcmQYHyABKAsyHS5waG9uZWJyaWRnZS52MS5JbnB1dEtleWJvYXJkSABSDWlucHV0S2V5Ym'
    '9hcmQSPQoLaW5wdXRfbW91c2UYICABKAsyGi5waG9uZWJyaWRnZS52MS5JbnB1dE1vdXNlSABS'
    'CmlucHV0TW91c2USPQoLaW5wdXRfdG91Y2gYISABKAsyGi5waG9uZWJyaWRnZS52MS5JbnB1dF'
    'RvdWNoSABSCmlucHV0VG91Y2gSQwoNZGV2aWNlX3N0YXR1cxgiIAEoCzIcLnBob25lYnJpZGdl'
    'LnYxLkRldmljZVN0YXR1c0gAUgxkZXZpY2VTdGF0dXNCCQoHcGF5bG9hZEoECAgQCUoECAkQCg'
    '==');

@$core.Deprecated('Use capabilitySetDescriptor instead')
const CapabilitySet$json = {
  '1': 'CapabilitySet',
  '2': [
    {
      '1': 'capabilities',
      '3': 1,
      '4': 3,
      '5': 14,
      '6': '.phonebridge.v1.Capability',
      '10': 'capabilities'
    },
  ],
};

/// Descriptor for `CapabilitySet`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List capabilitySetDescriptor = $convert.base64Decode(
    'Cg1DYXBhYmlsaXR5U2V0Ej4KDGNhcGFiaWxpdGllcxgBIAMoDjIaLnBob25lYnJpZGdlLnYxLk'
    'NhcGFiaWxpdHlSDGNhcGFiaWxpdGllcw==');

@$core.Deprecated('Use versionNegotiationDescriptor instead')
const VersionNegotiation$json = {
  '1': 'VersionNegotiation',
  '2': [
    {'1': 'min_version', '3': 1, '4': 1, '5': 13, '10': 'minVersion'},
    {'1': 'max_version', '3': 2, '4': 1, '5': 13, '10': 'maxVersion'},
  ],
};

/// Descriptor for `VersionNegotiation`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List versionNegotiationDescriptor = $convert.base64Decode(
    'ChJWZXJzaW9uTmVnb3RpYXRpb24SHwoLbWluX3ZlcnNpb24YASABKA1SCm1pblZlcnNpb24SHw'
    'oLbWF4X3ZlcnNpb24YAiABKA1SCm1heFZlcnNpb24=');

@$core.Deprecated('Use mediaParamsDescriptor instead')
const MediaParams$json = {
  '1': 'MediaParams',
  '2': [
    {'1': 'width', '3': 1, '4': 1, '5': 13, '10': 'width'},
    {'1': 'height', '3': 2, '4': 1, '5': 13, '10': 'height'},
    {'1': 'fps', '3': 3, '4': 1, '5': 13, '10': 'fps'},
    {'1': 'bitrate_kbps', '3': 4, '4': 1, '5': 13, '10': 'bitrateKbps'},
    {'1': 'codec', '3': 5, '4': 1, '5': 9, '10': 'codec'},
  ],
};

/// Descriptor for `MediaParams`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List mediaParamsDescriptor = $convert.base64Decode(
    'CgtNZWRpYVBhcmFtcxIUCgV3aWR0aBgBIAEoDVIFd2lkdGgSFgoGaGVpZ2h0GAIgASgNUgZoZW'
    'lnaHQSEAoDZnBzGAMgASgNUgNmcHMSIQoMYml0cmF0ZV9rYnBzGAQgASgNUgtiaXRyYXRlS2Jw'
    'cxIUCgVjb2RlYxgFIAEoCVIFY29kZWM=');

@$core.Deprecated('Use mediaCapabilitiesDescriptor instead')
const MediaCapabilities$json = {
  '1': 'MediaCapabilities',
  '2': [
    {'1': 'codecs', '3': 1, '4': 3, '5': 9, '10': 'codecs'},
    {'1': 'max_width', '3': 2, '4': 1, '5': 13, '10': 'maxWidth'},
    {'1': 'max_height', '3': 3, '4': 1, '5': 13, '10': 'maxHeight'},
    {'1': 'max_fps', '3': 4, '4': 1, '5': 13, '10': 'maxFps'},
    {'1': 'supports_screen', '3': 5, '4': 1, '5': 8, '10': 'supportsScreen'},
  ],
};

/// Descriptor for `MediaCapabilities`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List mediaCapabilitiesDescriptor = $convert.base64Decode(
    'ChFNZWRpYUNhcGFiaWxpdGllcxIWCgZjb2RlY3MYASADKAlSBmNvZGVjcxIbCgltYXhfd2lkdG'
    'gYAiABKA1SCG1heFdpZHRoEh0KCm1heF9oZWlnaHQYAyABKA1SCW1heEhlaWdodBIXCgdtYXhf'
    'ZnBzGAQgASgNUgZtYXhGcHMSJwoPc3VwcG9ydHNfc2NyZWVuGAUgASgIUg5zdXBwb3J0c1Njcm'
    'Vlbg==');

@$core.Deprecated('Use deviceHelloDescriptor instead')
const DeviceHello$json = {
  '1': 'DeviceHello',
  '2': [
    {'1': 'display_name', '3': 1, '4': 1, '5': 9, '10': 'displayName'},
    {'1': 'platform', '3': 2, '4': 1, '5': 9, '10': 'platform'},
    {'1': 'identity_pubkey', '3': 3, '4': 1, '5': 12, '10': 'identityPubkey'},
    {
      '1': 'version_negotiation',
      '3': 4,
      '4': 1,
      '5': 11,
      '6': '.phonebridge.v1.VersionNegotiation',
      '10': 'versionNegotiation'
    },
    {
      '1': 'capabilities',
      '3': 5,
      '4': 1,
      '5': 11,
      '6': '.phonebridge.v1.CapabilitySet',
      '10': 'capabilities'
    },
    {
      '1': 'media',
      '3': 6,
      '4': 1,
      '5': 11,
      '6': '.phonebridge.v1.MediaCapabilities',
      '10': 'media'
    },
  ],
};

/// Descriptor for `DeviceHello`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List deviceHelloDescriptor = $convert.base64Decode(
    'CgtEZXZpY2VIZWxsbxIhCgxkaXNwbGF5X25hbWUYASABKAlSC2Rpc3BsYXlOYW1lEhoKCHBsYX'
    'Rmb3JtGAIgASgJUghwbGF0Zm9ybRInCg9pZGVudGl0eV9wdWJrZXkYAyABKAxSDmlkZW50aXR5'
    'UHVia2V5ElMKE3ZlcnNpb25fbmVnb3RpYXRpb24YBCABKAsyIi5waG9uZWJyaWRnZS52MS5WZX'
    'JzaW9uTmVnb3RpYXRpb25SEnZlcnNpb25OZWdvdGlhdGlvbhJBCgxjYXBhYmlsaXRpZXMYBSAB'
    'KAsyHS5waG9uZWJyaWRnZS52MS5DYXBhYmlsaXR5U2V0UgxjYXBhYmlsaXRpZXMSNwoFbWVkaW'
    'EYBiABKAsyIS5waG9uZWJyaWRnZS52MS5NZWRpYUNhcGFiaWxpdGllc1IFbWVkaWE=');

@$core.Deprecated('Use pairRequestDescriptor instead')
const PairRequest$json = {
  '1': 'PairRequest',
  '2': [
    {
      '1': 'hello',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.phonebridge.v1.DeviceHello',
      '10': 'hello'
    },
    {'1': 'pairing_token', '3': 2, '4': 1, '5': 9, '10': 'pairingToken'},
  ],
};

/// Descriptor for `PairRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List pairRequestDescriptor = $convert.base64Decode(
    'CgtQYWlyUmVxdWVzdBIxCgVoZWxsbxgBIAEoCzIbLnBob25lYnJpZGdlLnYxLkRldmljZUhlbG'
    'xvUgVoZWxsbxIjCg1wYWlyaW5nX3Rva2VuGAIgASgJUgxwYWlyaW5nVG9rZW4=');

@$core.Deprecated('Use pairAcceptDescriptor instead')
const PairAccept$json = {
  '1': 'PairAccept',
  '2': [
    {
      '1': 'hello',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.phonebridge.v1.DeviceHello',
      '10': 'hello'
    },
    {'1': 'sas', '3': 2, '4': 1, '5': 9, '10': 'sas'},
  ],
};

/// Descriptor for `PairAccept`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List pairAcceptDescriptor = $convert.base64Decode(
    'CgpQYWlyQWNjZXB0EjEKBWhlbGxvGAEgASgLMhsucGhvbmVicmlkZ2UudjEuRGV2aWNlSGVsbG'
    '9SBWhlbGxvEhAKA3NhcxgCIAEoCVIDc2Fz');

@$core.Deprecated('Use pairRejectDescriptor instead')
const PairReject$json = {
  '1': 'PairReject',
  '2': [
    {
      '1': 'error',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.phonebridge.v1.Error',
      '10': 'error'
    },
  ],
};

/// Descriptor for `PairReject`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List pairRejectDescriptor = $convert.base64Decode(
    'CgpQYWlyUmVqZWN0EisKBWVycm9yGAEgASgLMhUucGhvbmVicmlkZ2UudjEuRXJyb3JSBWVycm'
    '9y');

@$core.Deprecated('Use pingDescriptor instead')
const Ping$json = {
  '1': 'Ping',
  '2': [
    {'1': 'nonce', '3': 1, '4': 1, '5': 4, '10': 'nonce'},
  ],
};

/// Descriptor for `Ping`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List pingDescriptor =
    $convert.base64Decode('CgRQaW5nEhQKBW5vbmNlGAEgASgEUgVub25jZQ==');

@$core.Deprecated('Use pongDescriptor instead')
const Pong$json = {
  '1': 'Pong',
  '2': [
    {'1': 'nonce', '3': 1, '4': 1, '5': 4, '10': 'nonce'},
  ],
};

/// Descriptor for `Pong`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List pongDescriptor =
    $convert.base64Decode('CgRQb25nEhQKBW5vbmNlGAEgASgEUgVub25jZQ==');

@$core.Deprecated('Use errorDescriptor instead')
const Error$json = {
  '1': 'Error',
  '2': [
    {
      '1': 'code',
      '3': 1,
      '4': 1,
      '5': 14,
      '6': '.phonebridge.v1.Code',
      '10': 'code'
    },
    {'1': 'message', '3': 2, '4': 1, '5': 9, '10': 'message'},
    {
      '1': 'details',
      '3': 3,
      '4': 3,
      '5': 11,
      '6': '.phonebridge.v1.Error.DetailsEntry',
      '10': 'details'
    },
  ],
  '3': [Error_DetailsEntry$json],
};

@$core.Deprecated('Use errorDescriptor instead')
const Error_DetailsEntry$json = {
  '1': 'DetailsEntry',
  '2': [
    {'1': 'key', '3': 1, '4': 1, '5': 9, '10': 'key'},
    {'1': 'value', '3': 2, '4': 1, '5': 9, '10': 'value'},
  ],
  '7': {'7': true},
};

/// Descriptor for `Error`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List errorDescriptor = $convert.base64Decode(
    'CgVFcnJvchIoCgRjb2RlGAEgASgOMhQucGhvbmVicmlkZ2UudjEuQ29kZVIEY29kZRIYCgdtZX'
    'NzYWdlGAIgASgJUgdtZXNzYWdlEjwKB2RldGFpbHMYAyADKAsyIi5waG9uZWJyaWRnZS52MS5F'
    'cnJvci5EZXRhaWxzRW50cnlSB2RldGFpbHMaOgoMRGV0YWlsc0VudHJ5EhAKA2tleRgBIAEoCV'
    'IDa2V5EhQKBXZhbHVlGAIgASgJUgV2YWx1ZToCOAE=');

@$core.Deprecated('Use screenStartDescriptor instead')
const ScreenStart$json = {
  '1': 'ScreenStart',
  '2': [
    {
      '1': 'requested',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.phonebridge.v1.MediaParams',
      '10': 'requested'
    },
    {'1': 'accepted', '3': 2, '4': 1, '5': 8, '10': 'accepted'},
    {'1': 'reject_reason', '3': 3, '4': 1, '5': 9, '10': 'rejectReason'},
    {
      '1': 'actual',
      '3': 4,
      '4': 1,
      '5': 11,
      '6': '.phonebridge.v1.MediaParams',
      '10': 'actual'
    },
  ],
};

/// Descriptor for `ScreenStart`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List screenStartDescriptor = $convert.base64Decode(
    'CgtTY3JlZW5TdGFydBI5CglyZXF1ZXN0ZWQYASABKAsyGy5waG9uZWJyaWRnZS52MS5NZWRpYV'
    'BhcmFtc1IJcmVxdWVzdGVkEhoKCGFjY2VwdGVkGAIgASgIUghhY2NlcHRlZBIjCg1yZWplY3Rf'
    'cmVhc29uGAMgASgJUgxyZWplY3RSZWFzb24SMwoGYWN0dWFsGAQgASgLMhsucGhvbmVicmlkZ2'
    'UudjEuTWVkaWFQYXJhbXNSBmFjdHVhbA==');

@$core.Deprecated('Use clipboardUpdateDescriptor instead')
const ClipboardUpdate$json = {
  '1': 'ClipboardUpdate',
  '2': [
    {'1': 'mime_type', '3': 1, '4': 1, '5': 9, '10': 'mimeType'},
    {'1': 'payload', '3': 2, '4': 1, '5': 12, '10': 'payload'},
    {'1': 'sha256_digest', '3': 3, '4': 1, '5': 12, '10': 'sha256Digest'},
    {'1': 'copied_at_ms', '3': 4, '4': 1, '5': 4, '10': 'copiedAtMs'},
  ],
};

/// Descriptor for `ClipboardUpdate`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clipboardUpdateDescriptor = $convert.base64Decode(
    'Cg9DbGlwYm9hcmRVcGRhdGUSGwoJbWltZV90eXBlGAEgASgJUghtaW1lVHlwZRIYCgdwYXlsb2'
    'FkGAIgASgMUgdwYXlsb2FkEiMKDXNoYTI1Nl9kaWdlc3QYAyABKAxSDHNoYTI1NkRpZ2VzdBIg'
    'Cgxjb3BpZWRfYXRfbXMYBCABKARSCmNvcGllZEF0TXM=');

@$core.Deprecated('Use transferFrameDescriptor instead')
const TransferFrame$json = {
  '1': 'TransferFrame',
  '2': [
    {'1': 'version', '3': 1, '4': 1, '5': 13, '10': 'version'},
    {
      '1': 'offer',
      '3': 10,
      '4': 1,
      '5': 11,
      '6': '.phonebridge.v1.FileOffer',
      '9': 0,
      '10': 'offer'
    },
    {
      '1': 'accept',
      '3': 11,
      '4': 1,
      '5': 11,
      '6': '.phonebridge.v1.FileAccept',
      '9': 0,
      '10': 'accept'
    },
    {
      '1': 'chunk',
      '3': 12,
      '4': 1,
      '5': 11,
      '6': '.phonebridge.v1.FileChunk',
      '9': 0,
      '10': 'chunk'
    },
    {
      '1': 'complete',
      '3': 13,
      '4': 1,
      '5': 11,
      '6': '.phonebridge.v1.FileComplete',
      '9': 0,
      '10': 'complete'
    },
    {
      '1': 'result',
      '3': 14,
      '4': 1,
      '5': 11,
      '6': '.phonebridge.v1.FileResult',
      '9': 0,
      '10': 'result'
    },
    {
      '1': 'cancel',
      '3': 15,
      '4': 1,
      '5': 11,
      '6': '.phonebridge.v1.FileCancel',
      '9': 0,
      '10': 'cancel'
    },
  ],
  '8': [
    {'1': 'body'},
  ],
};

/// Descriptor for `TransferFrame`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List transferFrameDescriptor = $convert.base64Decode(
    'Cg1UcmFuc2ZlckZyYW1lEhgKB3ZlcnNpb24YASABKA1SB3ZlcnNpb24SMQoFb2ZmZXIYCiABKA'
    'syGS5waG9uZWJyaWRnZS52MS5GaWxlT2ZmZXJIAFIFb2ZmZXISNAoGYWNjZXB0GAsgASgLMhou'
    'cGhvbmVicmlkZ2UudjEuRmlsZUFjY2VwdEgAUgZhY2NlcHQSMQoFY2h1bmsYDCABKAsyGS5waG'
    '9uZWJyaWRnZS52MS5GaWxlQ2h1bmtIAFIFY2h1bmsSOgoIY29tcGxldGUYDSABKAsyHC5waG9u'
    'ZWJyaWRnZS52MS5GaWxlQ29tcGxldGVIAFIIY29tcGxldGUSNAoGcmVzdWx0GA4gASgLMhoucG'
    'hvbmVicmlkZ2UudjEuRmlsZVJlc3VsdEgAUgZyZXN1bHQSNAoGY2FuY2VsGA8gASgLMhoucGhv'
    'bmVicmlkZ2UudjEuRmlsZUNhbmNlbEgAUgZjYW5jZWxCBgoEYm9keQ==');

@$core.Deprecated('Use fileOfferDescriptor instead')
const FileOffer$json = {
  '1': 'FileOffer',
  '2': [
    {'1': 'transfer_id', '3': 1, '4': 1, '5': 9, '10': 'transferId'},
    {'1': 'filename', '3': 2, '4': 1, '5': 9, '10': 'filename'},
    {'1': 'mime_type', '3': 3, '4': 1, '5': 9, '10': 'mimeType'},
    {'1': 'size_bytes', '3': 4, '4': 1, '5': 4, '10': 'sizeBytes'},
    {'1': 'sha256_digest', '3': 5, '4': 1, '5': 12, '10': 'sha256Digest'},
    {'1': 'chunk_size', '3': 6, '4': 1, '5': 13, '10': 'chunkSize'},
    {'1': 'created_at_ms', '3': 7, '4': 1, '5': 4, '10': 'createdAtMs'},
  ],
};

/// Descriptor for `FileOffer`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List fileOfferDescriptor = $convert.base64Decode(
    'CglGaWxlT2ZmZXISHwoLdHJhbnNmZXJfaWQYASABKAlSCnRyYW5zZmVySWQSGgoIZmlsZW5hbW'
    'UYAiABKAlSCGZpbGVuYW1lEhsKCW1pbWVfdHlwZRgDIAEoCVIIbWltZVR5cGUSHQoKc2l6ZV9i'
    'eXRlcxgEIAEoBFIJc2l6ZUJ5dGVzEiMKDXNoYTI1Nl9kaWdlc3QYBSABKAxSDHNoYTI1NkRpZ2'
    'VzdBIdCgpjaHVua19zaXplGAYgASgNUgljaHVua1NpemUSIgoNY3JlYXRlZF9hdF9tcxgHIAEo'
    'BFILY3JlYXRlZEF0TXM=');

@$core.Deprecated('Use fileAcceptDescriptor instead')
const FileAccept$json = {
  '1': 'FileAccept',
  '2': [
    {'1': 'transfer_id', '3': 1, '4': 1, '5': 9, '10': 'transferId'},
    {'1': 'accept', '3': 2, '4': 1, '5': 8, '10': 'accept'},
    {
      '1': 'code',
      '3': 3,
      '4': 1,
      '5': 14,
      '6': '.phonebridge.v1.Code',
      '10': 'code'
    },
    {'1': 'reason', '3': 4, '4': 1, '5': 9, '10': 'reason'},
  ],
};

/// Descriptor for `FileAccept`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List fileAcceptDescriptor = $convert.base64Decode(
    'CgpGaWxlQWNjZXB0Eh8KC3RyYW5zZmVyX2lkGAEgASgJUgp0cmFuc2ZlcklkEhYKBmFjY2VwdB'
    'gCIAEoCFIGYWNjZXB0EigKBGNvZGUYAyABKA4yFC5waG9uZWJyaWRnZS52MS5Db2RlUgRjb2Rl'
    'EhYKBnJlYXNvbhgEIAEoCVIGcmVhc29u');

@$core.Deprecated('Use fileChunkDescriptor instead')
const FileChunk$json = {
  '1': 'FileChunk',
  '2': [
    {'1': 'transfer_id', '3': 1, '4': 1, '5': 9, '10': 'transferId'},
    {'1': 'chunk_index', '3': 2, '4': 1, '5': 4, '10': 'chunkIndex'},
    {'1': 'offset', '3': 3, '4': 1, '5': 4, '10': 'offset'},
    {'1': 'data', '3': 4, '4': 1, '5': 12, '10': 'data'},
  ],
};

/// Descriptor for `FileChunk`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List fileChunkDescriptor = $convert.base64Decode(
    'CglGaWxlQ2h1bmsSHwoLdHJhbnNmZXJfaWQYASABKAlSCnRyYW5zZmVySWQSHwoLY2h1bmtfaW'
    '5kZXgYAiABKARSCmNodW5rSW5kZXgSFgoGb2Zmc2V0GAMgASgEUgZvZmZzZXQSEgoEZGF0YRgE'
    'IAEoDFIEZGF0YQ==');

@$core.Deprecated('Use fileCompleteDescriptor instead')
const FileComplete$json = {
  '1': 'FileComplete',
  '2': [
    {'1': 'transfer_id', '3': 1, '4': 1, '5': 9, '10': 'transferId'},
    {'1': 'size_bytes', '3': 2, '4': 1, '5': 4, '10': 'sizeBytes'},
    {'1': 'sha256_digest', '3': 3, '4': 1, '5': 12, '10': 'sha256Digest'},
  ],
};

/// Descriptor for `FileComplete`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List fileCompleteDescriptor = $convert.base64Decode(
    'CgxGaWxlQ29tcGxldGUSHwoLdHJhbnNmZXJfaWQYASABKAlSCnRyYW5zZmVySWQSHQoKc2l6ZV'
    '9ieXRlcxgCIAEoBFIJc2l6ZUJ5dGVzEiMKDXNoYTI1Nl9kaWdlc3QYAyABKAxSDHNoYTI1NkRp'
    'Z2VzdA==');

@$core.Deprecated('Use fileResultDescriptor instead')
const FileResult$json = {
  '1': 'FileResult',
  '2': [
    {'1': 'transfer_id', '3': 1, '4': 1, '5': 9, '10': 'transferId'},
    {'1': 'committed', '3': 2, '4': 1, '5': 8, '10': 'committed'},
    {
      '1': 'code',
      '3': 3,
      '4': 1,
      '5': 14,
      '6': '.phonebridge.v1.Code',
      '10': 'code'
    },
    {'1': 'reason', '3': 4, '4': 1, '5': 9, '10': 'reason'},
    {'1': 'saved_name', '3': 5, '4': 1, '5': 9, '10': 'savedName'},
  ],
};

/// Descriptor for `FileResult`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List fileResultDescriptor = $convert.base64Decode(
    'CgpGaWxlUmVzdWx0Eh8KC3RyYW5zZmVyX2lkGAEgASgJUgp0cmFuc2ZlcklkEhwKCWNvbW1pdH'
    'RlZBgCIAEoCFIJY29tbWl0dGVkEigKBGNvZGUYAyABKA4yFC5waG9uZWJyaWRnZS52MS5Db2Rl'
    'UgRjb2RlEhYKBnJlYXNvbhgEIAEoCVIGcmVhc29uEh0KCnNhdmVkX25hbWUYBSABKAlSCXNhdm'
    'VkTmFtZQ==');

@$core.Deprecated('Use fileCancelDescriptor instead')
const FileCancel$json = {
  '1': 'FileCancel',
  '2': [
    {'1': 'transfer_id', '3': 1, '4': 1, '5': 9, '10': 'transferId'},
    {
      '1': 'code',
      '3': 2,
      '4': 1,
      '5': 14,
      '6': '.phonebridge.v1.Code',
      '10': 'code'
    },
    {'1': 'reason', '3': 3, '4': 1, '5': 9, '10': 'reason'},
  ],
};

/// Descriptor for `FileCancel`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List fileCancelDescriptor = $convert.base64Decode(
    'CgpGaWxlQ2FuY2VsEh8KC3RyYW5zZmVyX2lkGAEgASgJUgp0cmFuc2ZlcklkEigKBGNvZGUYAi'
    'ABKA4yFC5waG9uZWJyaWRnZS52MS5Db2RlUgRjb2RlEhYKBnJlYXNvbhgDIAEoCVIGcmVhc29u');

@$core.Deprecated('Use notificationEventDescriptor instead')
const NotificationEvent$json = {
  '1': 'NotificationEvent',
};

/// Descriptor for `NotificationEvent`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List notificationEventDescriptor =
    $convert.base64Decode('ChFOb3RpZmljYXRpb25FdmVudA==');

@$core.Deprecated('Use notificationActionDescriptor instead')
const NotificationAction$json = {
  '1': 'NotificationAction',
};

/// Descriptor for `NotificationAction`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List notificationActionDescriptor =
    $convert.base64Decode('ChJOb3RpZmljYXRpb25BY3Rpb24=');

@$core.Deprecated('Use notificationDismissedDescriptor instead')
const NotificationDismissed$json = {
  '1': 'NotificationDismissed',
};

/// Descriptor for `NotificationDismissed`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List notificationDismissedDescriptor =
    $convert.base64Decode('ChVOb3RpZmljYXRpb25EaXNtaXNzZWQ=');

@$core.Deprecated('Use screenStopDescriptor instead')
const ScreenStop$json = {
  '1': 'ScreenStop',
  '2': [
    {
      '1': 'reason_code',
      '3': 1,
      '4': 1,
      '5': 14,
      '6': '.phonebridge.v1.Code',
      '10': 'reasonCode'
    },
    {'1': 'reason', '3': 2, '4': 1, '5': 9, '10': 'reason'},
  ],
};

/// Descriptor for `ScreenStop`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List screenStopDescriptor = $convert.base64Decode(
    'CgpTY3JlZW5TdG9wEjUKC3JlYXNvbl9jb2RlGAEgASgOMhQucGhvbmVicmlkZ2UudjEuQ29kZV'
    'IKcmVhc29uQ29kZRIWCgZyZWFzb24YAiABKAlSBnJlYXNvbg==');

@$core.Deprecated('Use inputKeyboardDescriptor instead')
const InputKeyboard$json = {
  '1': 'InputKeyboard',
};

/// Descriptor for `InputKeyboard`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List inputKeyboardDescriptor =
    $convert.base64Decode('Cg1JbnB1dEtleWJvYXJk');

@$core.Deprecated('Use inputMouseDescriptor instead')
const InputMouse$json = {
  '1': 'InputMouse',
};

/// Descriptor for `InputMouse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List inputMouseDescriptor =
    $convert.base64Decode('CgpJbnB1dE1vdXNl');

@$core.Deprecated('Use inputTouchDescriptor instead')
const InputTouch$json = {
  '1': 'InputTouch',
};

/// Descriptor for `InputTouch`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List inputTouchDescriptor =
    $convert.base64Decode('CgpJbnB1dFRvdWNo');

@$core.Deprecated('Use deviceStatusDescriptor instead')
const DeviceStatus$json = {
  '1': 'DeviceStatus',
};

/// Descriptor for `DeviceStatus`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List deviceStatusDescriptor =
    $convert.base64Decode('CgxEZXZpY2VTdGF0dXM=');
