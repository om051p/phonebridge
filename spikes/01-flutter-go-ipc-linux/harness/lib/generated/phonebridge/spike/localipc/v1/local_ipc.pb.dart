// This is a generated file - do not edit.
//
// Generated from phonebridge/spike/localipc/v1/local_ipc.proto.

// @dart = 3.3

// ignore_for_file: annotate_overrides, camel_case_types, comment_references
// ignore_for_file: constant_identifier_names
// ignore_for_file: curly_braces_in_flow_control_structures
// ignore_for_file: deprecated_member_use_from_same_package, library_prefixes
// ignore_for_file: non_constant_identifier_names, prefer_relative_imports

import 'dart:core' as $core;

import 'package:fixnum/fixnum.dart' as $fixnum;
import 'package:protobuf/protobuf.dart' as $pb;

export 'package:protobuf/protobuf.dart' show GeneratedMessageGenericExtensions;

/// PingRequest carries a caller nonce and an optional payload used to sweep
/// message sizes (chunking evidence for future file/clipboard transfer).
class PingRequest extends $pb.GeneratedMessage {
  factory PingRequest({
    $fixnum.Int64? nonce,
    $core.List<$core.int>? payload,
  }) {
    final result = PingRequest._();
    if (nonce != null) result.nonce = nonce;
    if (payload != null) result.payload = payload;
    return result;
  }

  PingRequest._();

  factory PingRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      PingRequest()..mergeFromBuffer(data, registry);
  factory PingRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      PingRequest()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'PingRequest',
      package: const $pb.PackageName(
          _omitMessageNames ? '' : 'phonebridge.spike.localipc.v1'),
      createEmptyInstance: PingRequest.$_createMessage)
    ..a<$fixnum.Int64>(1, _omitFieldNames ? '' : 'nonce', $pb.PbFieldType.OU6,
        defaultOrMaker: $fixnum.Int64.ZERO)
    ..a<$core.List<$core.int>>(
        2, _omitFieldNames ? '' : 'payload', $pb.PbFieldType.OY)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PingRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PingRequest copyWith(void Function(PingRequest) updates) =>
      super.copyWith((message) => updates(message as PingRequest))
          as PingRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated('Use PingRequest() / PingRequest.new instead')
  static PingRequest create() => PingRequest._();
  static $pb.GeneratedMessage $_createMessage() => PingRequest._();
  @$core.override
  PingRequest createEmptyInstance() => PingRequest._();
  @$core.pragma('dart2js:noInline')
  static PingRequest getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<PingRequest>(
          PingRequest.$_createMessage);
  static PingRequest? _defaultInstance;

  /// Caller-chosen nonce, echoed back to prove the reply belongs to the call.
  @$pb.TagNumber(1)
  $fixnum.Int64 get nonce => $_getI64(0);
  @$pb.TagNumber(1)
  set nonce($fixnum.Int64 value) => $_setInt64(0, value);
  @$pb.TagNumber(1)
  $core.bool hasNonce() => $_has(0);
  @$pb.TagNumber(1)
  void clearNonce() => $_clearField(1);

  /// Opaque payload; length drives the size sweep (0..64 KiB in the spike).
  @$pb.TagNumber(2)
  $core.List<$core.int> get payload => $_getN(1);
  @$pb.TagNumber(2)
  set payload($core.List<$core.int> value) => $_setBytes(1, value);
  @$pb.TagNumber(2)
  $core.bool hasPayload() => $_has(1);
  @$pb.TagNumber(2)
  void clearPayload() => $_clearField(2);
}

/// PingResponse is the daemon's reply to PingRequest.
class PingResponse extends $pb.GeneratedMessage {
  factory PingResponse({
    $fixnum.Int64? nonce,
    $core.String? serverVersion,
    $core.List<$core.int>? payload,
    $fixnum.Int64? serverRecvUnixNanos,
  }) {
    final result = PingResponse._();
    if (nonce != null) result.nonce = nonce;
    if (serverVersion != null) result.serverVersion = serverVersion;
    if (payload != null) result.payload = payload;
    if (serverRecvUnixNanos != null)
      result.serverRecvUnixNanos = serverRecvUnixNanos;
    return result;
  }

  PingResponse._();

  factory PingResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      PingResponse()..mergeFromBuffer(data, registry);
  factory PingResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      PingResponse()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'PingResponse',
      package: const $pb.PackageName(
          _omitMessageNames ? '' : 'phonebridge.spike.localipc.v1'),
      createEmptyInstance: PingResponse.$_createMessage)
    ..a<$fixnum.Int64>(1, _omitFieldNames ? '' : 'nonce', $pb.PbFieldType.OU6,
        defaultOrMaker: $fixnum.Int64.ZERO)
    ..aOS(2, _omitFieldNames ? '' : 'serverVersion')
    ..a<$core.List<$core.int>>(
        3, _omitFieldNames ? '' : 'payload', $pb.PbFieldType.OY)
    ..a<$fixnum.Int64>(
        4, _omitFieldNames ? '' : 'serverRecvUnixNanos', $pb.PbFieldType.OU6,
        defaultOrMaker: $fixnum.Int64.ZERO)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PingResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PingResponse copyWith(void Function(PingResponse) updates) =>
      super.copyWith((message) => updates(message as PingResponse))
          as PingResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated('Use PingResponse() / PingResponse.new instead')
  static PingResponse create() => PingResponse._();
  static $pb.GeneratedMessage $_createMessage() => PingResponse._();
  @$core.override
  PingResponse createEmptyInstance() => PingResponse._();
  @$core.pragma('dart2js:noInline')
  static PingResponse getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<PingResponse>(
          PingResponse.$_createMessage);
  static PingResponse? _defaultInstance;

  /// Echo of PingRequest.nonce.
  @$pb.TagNumber(1)
  $fixnum.Int64 get nonce => $_getI64(0);
  @$pb.TagNumber(1)
  set nonce($fixnum.Int64 value) => $_setInt64(0, value);
  @$pb.TagNumber(1)
  $core.bool hasNonce() => $_has(0);
  @$pb.TagNumber(1)
  void clearNonce() => $_clearField(1);

  /// Daemon build/version string (identity + reconnect check).
  @$pb.TagNumber(2)
  $core.String get serverVersion => $_getSZ(1);
  @$pb.TagNumber(2)
  set serverVersion($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasServerVersion() => $_has(1);
  @$pb.TagNumber(2)
  void clearServerVersion() => $_clearField(2);

  /// Echo of PingRequest.payload (possibly truncated by the daemon).
  @$pb.TagNumber(3)
  $core.List<$core.int> get payload => $_getN(2);
  @$pb.TagNumber(3)
  set payload($core.List<$core.int> value) => $_setBytes(2, value);
  @$pb.TagNumber(3)
  $core.bool hasPayload() => $_has(2);
  @$pb.TagNumber(3)
  void clearPayload() => $_clearField(3);

  /// Daemon wall clock (CLOCK_REALTIME, nanoseconds) when the request was
  /// received; lets the client split transit from service time.
  @$pb.TagNumber(4)
  $fixnum.Int64 get serverRecvUnixNanos => $_getI64(3);
  @$pb.TagNumber(4)
  set serverRecvUnixNanos($fixnum.Int64 value) => $_setInt64(3, value);
  @$pb.TagNumber(4)
  $core.bool hasServerRecvUnixNanos() => $_has(3);
  @$pb.TagNumber(4)
  void clearServerRecvUnixNanos() => $_clearField(4);
}

/// SubscribeRequest configures the push test.
class SubscribeRequest extends $pb.GeneratedMessage {
  factory SubscribeRequest({
    $core.int? count,
    $core.int? intervalMs,
    $core.int? payloadSize,
  }) {
    final result = SubscribeRequest._();
    if (count != null) result.count = count;
    if (intervalMs != null) result.intervalMs = intervalMs;
    if (payloadSize != null) result.payloadSize = payloadSize;
    return result;
  }

  SubscribeRequest._();

  factory SubscribeRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      SubscribeRequest()..mergeFromBuffer(data, registry);
  factory SubscribeRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      SubscribeRequest()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'SubscribeRequest',
      package: const $pb.PackageName(
          _omitMessageNames ? '' : 'phonebridge.spike.localipc.v1'),
      createEmptyInstance: SubscribeRequest.$_createMessage)
    ..aI(1, _omitFieldNames ? '' : 'count', fieldType: $pb.PbFieldType.OU3)
    ..aI(2, _omitFieldNames ? '' : 'intervalMs', fieldType: $pb.PbFieldType.OU3)
    ..aI(3, _omitFieldNames ? '' : 'payloadSize',
        fieldType: $pb.PbFieldType.OU3)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  SubscribeRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  SubscribeRequest copyWith(void Function(SubscribeRequest) updates) =>
      super.copyWith((message) => updates(message as SubscribeRequest))
          as SubscribeRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated('Use SubscribeRequest() / SubscribeRequest.new instead')
  static SubscribeRequest create() => SubscribeRequest._();
  static $pb.GeneratedMessage $_createMessage() => SubscribeRequest._();
  @$core.override
  SubscribeRequest createEmptyInstance() => SubscribeRequest._();
  @$core.pragma('dart2js:noInline')
  static SubscribeRequest getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<SubscribeRequest>(
          SubscribeRequest.$_createMessage);
  static SubscribeRequest? _defaultInstance;

  /// Number of events to emit before the stream ends.
  @$pb.TagNumber(1)
  $core.int get count => $_getIZ(0);
  @$pb.TagNumber(1)
  set count($core.int value) => $_setUnsignedInt32(0, value);
  @$pb.TagNumber(1)
  $core.bool hasCount() => $_has(0);
  @$pb.TagNumber(1)
  void clearCount() => $_clearField(1);

  /// Delay between events, in milliseconds.
  @$pb.TagNumber(2)
  $core.int get intervalMs => $_getIZ(1);
  @$pb.TagNumber(2)
  set intervalMs($core.int value) => $_setUnsignedInt32(1, value);
  @$pb.TagNumber(2)
  $core.bool hasIntervalMs() => $_has(1);
  @$pb.TagNumber(2)
  void clearIntervalMs() => $_clearField(2);

  /// Bytes of synthetic payload attached to each event.
  @$pb.TagNumber(3)
  $core.int get payloadSize => $_getIZ(2);
  @$pb.TagNumber(3)
  set payloadSize($core.int value) => $_setUnsignedInt32(2, value);
  @$pb.TagNumber(3)
  $core.bool hasPayloadSize() => $_has(2);
  @$pb.TagNumber(3)
  void clearPayloadSize() => $_clearField(3);
}

/// SubscribeResponse is one pushed event.
class SubscribeResponse extends $pb.GeneratedMessage {
  factory SubscribeResponse({
    $fixnum.Int64? seq,
    $core.String? kind,
    $fixnum.Int64? emittedUnixNanos,
    $core.List<$core.int>? payload,
  }) {
    final result = SubscribeResponse._();
    if (seq != null) result.seq = seq;
    if (kind != null) result.kind = kind;
    if (emittedUnixNanos != null) result.emittedUnixNanos = emittedUnixNanos;
    if (payload != null) result.payload = payload;
    return result;
  }

  SubscribeResponse._();

  factory SubscribeResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      SubscribeResponse()..mergeFromBuffer(data, registry);
  factory SubscribeResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      SubscribeResponse()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'SubscribeResponse',
      package: const $pb.PackageName(
          _omitMessageNames ? '' : 'phonebridge.spike.localipc.v1'),
      createEmptyInstance: SubscribeResponse.$_createMessage)
    ..a<$fixnum.Int64>(1, _omitFieldNames ? '' : 'seq', $pb.PbFieldType.OU6,
        defaultOrMaker: $fixnum.Int64.ZERO)
    ..aOS(2, _omitFieldNames ? '' : 'kind')
    ..a<$fixnum.Int64>(
        3, _omitFieldNames ? '' : 'emittedUnixNanos', $pb.PbFieldType.OU6,
        defaultOrMaker: $fixnum.Int64.ZERO)
    ..a<$core.List<$core.int>>(
        4, _omitFieldNames ? '' : 'payload', $pb.PbFieldType.OY)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  SubscribeResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  SubscribeResponse copyWith(void Function(SubscribeResponse) updates) =>
      super.copyWith((message) => updates(message as SubscribeResponse))
          as SubscribeResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated('Use SubscribeResponse() / SubscribeResponse.new instead')
  static SubscribeResponse create() => SubscribeResponse._();
  static $pb.GeneratedMessage $_createMessage() => SubscribeResponse._();
  @$core.override
  SubscribeResponse createEmptyInstance() => SubscribeResponse._();
  @$core.pragma('dart2js:noInline')
  static SubscribeResponse getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<SubscribeResponse>(
          SubscribeResponse.$_createMessage);
  static SubscribeResponse? _defaultInstance;

  /// Monotonic per-stream sequence number, starting at 1.
  @$pb.TagNumber(1)
  $fixnum.Int64 get seq => $_getI64(0);
  @$pb.TagNumber(1)
  set seq($fixnum.Int64 value) => $_setInt64(0, value);
  @$pb.TagNumber(1)
  $core.bool hasSeq() => $_has(0);
  @$pb.TagNumber(1)
  void clearSeq() => $_clearField(1);

  /// Event kind label (spike-internal, e.g. "tick").
  @$pb.TagNumber(2)
  $core.String get kind => $_getSZ(1);
  @$pb.TagNumber(2)
  set kind($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasKind() => $_has(1);
  @$pb.TagNumber(2)
  void clearKind() => $_clearField(2);

  /// Daemon wall clock (CLOCK_REALTIME, nanoseconds) at emission time.
  @$pb.TagNumber(3)
  $fixnum.Int64 get emittedUnixNanos => $_getI64(2);
  @$pb.TagNumber(3)
  set emittedUnixNanos($fixnum.Int64 value) => $_setInt64(2, value);
  @$pb.TagNumber(3)
  $core.bool hasEmittedUnixNanos() => $_has(2);
  @$pb.TagNumber(3)
  void clearEmittedUnixNanos() => $_clearField(3);

  /// Synthetic payload (payload_size bytes).
  @$pb.TagNumber(4)
  $core.List<$core.int> get payload => $_getN(3);
  @$pb.TagNumber(4)
  set payload($core.List<$core.int> value) => $_setBytes(3, value);
  @$pb.TagNumber(4)
  $core.bool hasPayload() => $_has(3);
  @$pb.TagNumber(4)
  void clearPayload() => $_clearField(4);
}

/// WhoAmIRequest is empty; the caller identity comes from the socket itself.
class WhoAmIRequest extends $pb.GeneratedMessage {
  factory WhoAmIRequest() => WhoAmIRequest._();

  WhoAmIRequest._();

  factory WhoAmIRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      WhoAmIRequest()..mergeFromBuffer(data, registry);
  factory WhoAmIRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      WhoAmIRequest()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'WhoAmIRequest',
      package: const $pb.PackageName(
          _omitMessageNames ? '' : 'phonebridge.spike.localipc.v1'),
      createEmptyInstance: WhoAmIRequest.$_createMessage)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WhoAmIRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WhoAmIRequest copyWith(void Function(WhoAmIRequest) updates) =>
      super.copyWith((message) => updates(message as WhoAmIRequest))
          as WhoAmIRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated('Use WhoAmIRequest() / WhoAmIRequest.new instead')
  static WhoAmIRequest create() => WhoAmIRequest._();
  static $pb.GeneratedMessage $_createMessage() => WhoAmIRequest._();
  @$core.override
  WhoAmIRequest createEmptyInstance() => WhoAmIRequest._();
  @$core.pragma('dart2js:noInline')
  static WhoAmIRequest getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<WhoAmIRequest>(
          WhoAmIRequest.$_createMessage);
  static WhoAmIRequest? _defaultInstance;
}

/// WhoAmIResponse is the server's view of the peer process.
class WhoAmIResponse extends $pb.GeneratedMessage {
  factory WhoAmIResponse({
    $core.int? peerUid,
    $core.int? peerPid,
    $core.int? peerGid,
    $core.String? socketPath,
    $core.int? serverUid,
    $core.bool? tokenRequired,
  }) {
    final result = WhoAmIResponse._();
    if (peerUid != null) result.peerUid = peerUid;
    if (peerPid != null) result.peerPid = peerPid;
    if (peerGid != null) result.peerGid = peerGid;
    if (socketPath != null) result.socketPath = socketPath;
    if (serverUid != null) result.serverUid = serverUid;
    if (tokenRequired != null) result.tokenRequired = tokenRequired;
    return result;
  }

  WhoAmIResponse._();

  factory WhoAmIResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      WhoAmIResponse()..mergeFromBuffer(data, registry);
  factory WhoAmIResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      WhoAmIResponse()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'WhoAmIResponse',
      package: const $pb.PackageName(
          _omitMessageNames ? '' : 'phonebridge.spike.localipc.v1'),
      createEmptyInstance: WhoAmIResponse.$_createMessage)
    ..aI(1, _omitFieldNames ? '' : 'peerUid', fieldType: $pb.PbFieldType.OU3)
    ..aI(2, _omitFieldNames ? '' : 'peerPid')
    ..aI(3, _omitFieldNames ? '' : 'peerGid', fieldType: $pb.PbFieldType.OU3)
    ..aOS(4, _omitFieldNames ? '' : 'socketPath')
    ..aI(5, _omitFieldNames ? '' : 'serverUid', fieldType: $pb.PbFieldType.OU3)
    ..aOB(6, _omitFieldNames ? '' : 'tokenRequired')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WhoAmIResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WhoAmIResponse copyWith(void Function(WhoAmIResponse) updates) =>
      super.copyWith((message) => updates(message as WhoAmIResponse))
          as WhoAmIResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated('Use WhoAmIResponse() / WhoAmIResponse.new instead')
  static WhoAmIResponse create() => WhoAmIResponse._();
  static $pb.GeneratedMessage $_createMessage() => WhoAmIResponse._();
  @$core.override
  WhoAmIResponse createEmptyInstance() => WhoAmIResponse._();
  @$core.pragma('dart2js:noInline')
  static WhoAmIResponse getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<WhoAmIResponse>(
          WhoAmIResponse.$_createMessage);
  static WhoAmIResponse? _defaultInstance;

  /// Peer UID from SO_PEERCRED on the accepted Unix socket.
  @$pb.TagNumber(1)
  $core.int get peerUid => $_getIZ(0);
  @$pb.TagNumber(1)
  set peerUid($core.int value) => $_setUnsignedInt32(0, value);
  @$pb.TagNumber(1)
  $core.bool hasPeerUid() => $_has(0);
  @$pb.TagNumber(1)
  void clearPeerUid() => $_clearField(1);

  /// Peer PID from SO_PEERCRED (Linux). Informational.
  @$pb.TagNumber(2)
  $core.int get peerPid => $_getIZ(1);
  @$pb.TagNumber(2)
  set peerPid($core.int value) => $_setSignedInt32(1, value);
  @$pb.TagNumber(2)
  $core.bool hasPeerPid() => $_has(1);
  @$pb.TagNumber(2)
  void clearPeerPid() => $_clearField(2);

  /// Peer primary GID from SO_PEERCRED. Informational.
  @$pb.TagNumber(3)
  $core.int get peerGid => $_getIZ(2);
  @$pb.TagNumber(3)
  set peerGid($core.int value) => $_setUnsignedInt32(2, value);
  @$pb.TagNumber(3)
  $core.bool hasPeerGid() => $_has(2);
  @$pb.TagNumber(3)
  void clearPeerGid() => $_clearField(3);

  /// Absolute path of the socket the request arrived on.
  @$pb.TagNumber(4)
  $core.String get socketPath => $_getSZ(3);
  @$pb.TagNumber(4)
  set socketPath($core.String value) => $_setString(3, value);
  @$pb.TagNumber(4)
  $core.bool hasSocketPath() => $_has(3);
  @$pb.TagNumber(4)
  void clearSocketPath() => $_clearField(4);

  /// Daemon's own effective UID (the expected peer UID).
  @$pb.TagNumber(5)
  $core.int get serverUid => $_getIZ(4);
  @$pb.TagNumber(5)
  set serverUid($core.int value) => $_setUnsignedInt32(4, value);
  @$pb.TagNumber(5)
  $core.bool hasServerUid() => $_has(4);
  @$pb.TagNumber(5)
  void clearServerUid() => $_clearField(5);

  /// Whether the daemon enforced a bearer-token check for this call.
  @$pb.TagNumber(6)
  $core.bool get tokenRequired => $_getBF(5);
  @$pb.TagNumber(6)
  set tokenRequired($core.bool value) => $_setBool(5, value);
  @$pb.TagNumber(6)
  $core.bool hasTokenRequired() => $_has(5);
  @$pb.TagNumber(6)
  void clearTokenRequired() => $_clearField(6);
}

const $core.bool _omitFieldNames =
    $core.bool.fromEnvironment('protobuf.omit_field_names');
const $core.bool _omitMessageNames =
    $core.bool.fromEnvironment('protobuf.omit_message_names');
