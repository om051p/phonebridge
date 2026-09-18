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

import 'package:fixnum/fixnum.dart' as $fixnum;
import 'package:protobuf/protobuf.dart' as $pb;

import '../../v1/phonebridge.pb.dart' as $1;
import 'local_ipc.pbenum.dart';

export 'package:protobuf/protobuf.dart' show GeneratedMessageGenericExtensions;

export 'local_ipc.pbenum.dart';

/// HandshakeRequest advertises the client's local-IPC protocol support.
class HandshakeRequest extends $pb.GeneratedMessage {
  factory HandshakeRequest({
    $core.int? clientVersion,
  }) {
    final result = HandshakeRequest._();
    if (clientVersion != null) result.clientVersion = clientVersion;
    return result;
  }

  HandshakeRequest._();

  factory HandshakeRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      HandshakeRequest()..mergeFromBuffer(data, registry);
  factory HandshakeRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      HandshakeRequest()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'HandshakeRequest',
      package: const $pb.PackageName(
          _omitMessageNames ? '' : 'phonebridge.localipc.v1'),
      createEmptyInstance: HandshakeRequest.$_createMessage)
    ..aI(1, _omitFieldNames ? '' : 'clientVersion',
        fieldType: $pb.PbFieldType.OU3)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  HandshakeRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  HandshakeRequest copyWith(void Function(HandshakeRequest) updates) =>
      super.copyWith((message) => updates(message as HandshakeRequest))
          as HandshakeRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated('Use HandshakeRequest() / HandshakeRequest.new instead')
  static HandshakeRequest create() => HandshakeRequest._();
  static $pb.GeneratedMessage $_createMessage() => HandshakeRequest._();
  @$core.override
  HandshakeRequest createEmptyInstance() => HandshakeRequest._();
  @$core.pragma('dart2js:noInline')
  static HandshakeRequest getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<HandshakeRequest>(
          HandshakeRequest.$_createMessage);
  static HandshakeRequest? _defaultInstance;

  /// Highest phonebridge.localipc protocol version the client implements.
  /// Currently always 1. The server negotiates down to its own maximum.
  @$pb.TagNumber(1)
  $core.int get clientVersion => $_getIZ(0);
  @$pb.TagNumber(1)
  set clientVersion($core.int value) => $_setUnsignedInt32(0, value);
  @$pb.TagNumber(1)
  $core.bool hasClientVersion() => $_has(0);
  @$pb.TagNumber(1)
  void clearClientVersion() => $_clearField(1);
}

/// HandshakeResponse answers Handshake.
class HandshakeResponse extends $pb.GeneratedMessage {
  factory HandshakeResponse({
    $core.int? negotiatedVersion,
    $core.String? serverVersion,
    $fixnum.Int64? daemonGeneration,
  }) {
    final result = HandshakeResponse._();
    if (negotiatedVersion != null) result.negotiatedVersion = negotiatedVersion;
    if (serverVersion != null) result.serverVersion = serverVersion;
    if (daemonGeneration != null) result.daemonGeneration = daemonGeneration;
    return result;
  }

  HandshakeResponse._();

  factory HandshakeResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      HandshakeResponse()..mergeFromBuffer(data, registry);
  factory HandshakeResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      HandshakeResponse()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'HandshakeResponse',
      package: const $pb.PackageName(
          _omitMessageNames ? '' : 'phonebridge.localipc.v1'),
      createEmptyInstance: HandshakeResponse.$_createMessage)
    ..aI(1, _omitFieldNames ? '' : 'negotiatedVersion',
        fieldType: $pb.PbFieldType.OU3)
    ..aOS(2, _omitFieldNames ? '' : 'serverVersion')
    ..a<$fixnum.Int64>(
        3, _omitFieldNames ? '' : 'daemonGeneration', $pb.PbFieldType.OU6,
        defaultOrMaker: $fixnum.Int64.ZERO)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  HandshakeResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  HandshakeResponse copyWith(void Function(HandshakeResponse) updates) =>
      super.copyWith((message) => updates(message as HandshakeResponse))
          as HandshakeResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated('Use HandshakeResponse() / HandshakeResponse.new instead')
  static HandshakeResponse create() => HandshakeResponse._();
  static $pb.GeneratedMessage $_createMessage() => HandshakeResponse._();
  @$core.override
  HandshakeResponse createEmptyInstance() => HandshakeResponse._();
  @$core.pragma('dart2js:noInline')
  static HandshakeResponse getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<HandshakeResponse>(
          HandshakeResponse.$_createMessage);
  static HandshakeResponse? _defaultInstance;

  /// Version selected for this connection: min(client_version, server max).
  /// Currently always 1.
  @$pb.TagNumber(1)
  $core.int get negotiatedVersion => $_getIZ(0);
  @$pb.TagNumber(1)
  set negotiatedVersion($core.int value) => $_setUnsignedInt32(0, value);
  @$pb.TagNumber(1)
  $core.bool hasNegotiatedVersion() => $_has(0);
  @$pb.TagNumber(1)
  void clearNegotiatedVersion() => $_clearField(1);

  /// Daemon build/version string (informational identity for logs/debug).
  @$pb.TagNumber(2)
  $core.String get serverVersion => $_getSZ(1);
  @$pb.TagNumber(2)
  set serverVersion($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasServerVersion() => $_has(1);
  @$pb.TagNumber(2)
  void clearServerVersion() => $_clearField(2);

  /// Identifier of the daemon process instance. Changes on every daemon
  /// (re)start; clients use it to detect daemon replacement across
  /// reconnects. Matches StreamEventsResponse.daemon_generation on later streams.
  @$pb.TagNumber(3)
  $fixnum.Int64 get daemonGeneration => $_getI64(2);
  @$pb.TagNumber(3)
  set daemonGeneration($fixnum.Int64 value) => $_setInt64(2, value);
  @$pb.TagNumber(3)
  $core.bool hasDaemonGeneration() => $_has(2);
  @$pb.TagNumber(3)
  void clearDaemonGeneration() => $_clearField(3);
}

/// PingRequest carries a caller nonce for reply correlation.
class PingRequest extends $pb.GeneratedMessage {
  factory PingRequest({
    $fixnum.Int64? nonce,
  }) {
    final result = PingRequest._();
    if (nonce != null) result.nonce = nonce;
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
          _omitMessageNames ? '' : 'phonebridge.localipc.v1'),
      createEmptyInstance: PingRequest.$_createMessage)
    ..a<$fixnum.Int64>(1, _omitFieldNames ? '' : 'nonce', $pb.PbFieldType.OU6,
        defaultOrMaker: $fixnum.Int64.ZERO)
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

  /// Caller-chosen nonce, echoed in PingResponse.
  @$pb.TagNumber(1)
  $fixnum.Int64 get nonce => $_getI64(0);
  @$pb.TagNumber(1)
  set nonce($fixnum.Int64 value) => $_setInt64(0, value);
  @$pb.TagNumber(1)
  $core.bool hasNonce() => $_has(0);
  @$pb.TagNumber(1)
  void clearNonce() => $_clearField(1);
}

/// PingResponse answers PingRequest.
class PingResponse extends $pb.GeneratedMessage {
  factory PingResponse({
    $fixnum.Int64? nonce,
    $core.String? serverVersion,
  }) {
    final result = PingResponse._();
    if (nonce != null) result.nonce = nonce;
    if (serverVersion != null) result.serverVersion = serverVersion;
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
          _omitMessageNames ? '' : 'phonebridge.localipc.v1'),
      createEmptyInstance: PingResponse.$_createMessage)
    ..a<$fixnum.Int64>(1, _omitFieldNames ? '' : 'nonce', $pb.PbFieldType.OU6,
        defaultOrMaker: $fixnum.Int64.ZERO)
    ..aOS(2, _omitFieldNames ? '' : 'serverVersion')
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

  /// Daemon build/version string (identity check after a reconnect).
  @$pb.TagNumber(2)
  $core.String get serverVersion => $_getSZ(1);
  @$pb.TagNumber(2)
  set serverVersion($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasServerVersion() => $_has(1);
  @$pb.TagNumber(2)
  void clearServerVersion() => $_clearField(2);
}

/// StreamEventsRequest subscribes to relayed device-protocol events.
/// Intentionally empty in v1: future filters (event kinds, device ids) are
/// added as optional fields; older servers ignore unknown fields.
class StreamEventsRequest extends $pb.GeneratedMessage {
  factory StreamEventsRequest() => StreamEventsRequest._();

  StreamEventsRequest._();

  factory StreamEventsRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      StreamEventsRequest()..mergeFromBuffer(data, registry);
  factory StreamEventsRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      StreamEventsRequest()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'StreamEventsRequest',
      package: const $pb.PackageName(
          _omitMessageNames ? '' : 'phonebridge.localipc.v1'),
      createEmptyInstance: StreamEventsRequest.$_createMessage)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  StreamEventsRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  StreamEventsRequest copyWith(void Function(StreamEventsRequest) updates) =>
      super.copyWith((message) => updates(message as StreamEventsRequest))
          as StreamEventsRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core
      .Deprecated('Use StreamEventsRequest() / StreamEventsRequest.new instead')
  static StreamEventsRequest create() => StreamEventsRequest._();
  static $pb.GeneratedMessage $_createMessage() => StreamEventsRequest._();
  @$core.override
  StreamEventsRequest createEmptyInstance() => StreamEventsRequest._();
  @$core.pragma('dart2js:noInline')
  static StreamEventsRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<StreamEventsRequest>(
          StreamEventsRequest.$_createMessage);
  static StreamEventsRequest? _defaultInstance;
}

/// StreamEventsResponse is one pushed message on the StreamEvents stream.
/// (the "local event" of DEC-018; the type is named to satisfy the repo's STANDARD lint)
class StreamEventsResponse extends $pb.GeneratedMessage {
  factory StreamEventsResponse({
    $fixnum.Int64? seq,
    $fixnum.Int64? daemonGeneration,
    $1.Envelope? envelope,
    SessionEvent? sessionEvent,
  }) {
    final result = StreamEventsResponse._();
    if (seq != null) result.seq = seq;
    if (daemonGeneration != null) result.daemonGeneration = daemonGeneration;
    if (envelope != null) result.envelope = envelope;
    if (sessionEvent != null) result.sessionEvent = sessionEvent;
    return result;
  }

  StreamEventsResponse._();

  factory StreamEventsResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      StreamEventsResponse()..mergeFromBuffer(data, registry);
  factory StreamEventsResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      StreamEventsResponse()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'StreamEventsResponse',
      package: const $pb.PackageName(
          _omitMessageNames ? '' : 'phonebridge.localipc.v1'),
      createEmptyInstance: StreamEventsResponse.$_createMessage)
    ..a<$fixnum.Int64>(1, _omitFieldNames ? '' : 'seq', $pb.PbFieldType.OU6,
        defaultOrMaker: $fixnum.Int64.ZERO)
    ..a<$fixnum.Int64>(
        2, _omitFieldNames ? '' : 'daemonGeneration', $pb.PbFieldType.OU6,
        defaultOrMaker: $fixnum.Int64.ZERO)
    ..aOM<$1.Envelope>(3, _omitFieldNames ? '' : 'envelope',
        subBuilder: $1.Envelope.$_createMessage)
    ..aOM<SessionEvent>(4, _omitFieldNames ? '' : 'sessionEvent',
        subBuilder: SessionEvent.$_createMessage)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  StreamEventsResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  StreamEventsResponse copyWith(void Function(StreamEventsResponse) updates) =>
      super.copyWith((message) => updates(message as StreamEventsResponse))
          as StreamEventsResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated(
      'Use StreamEventsResponse() / StreamEventsResponse.new instead')
  static StreamEventsResponse create() => StreamEventsResponse._();
  static $pb.GeneratedMessage $_createMessage() => StreamEventsResponse._();
  @$core.override
  StreamEventsResponse createEmptyInstance() => StreamEventsResponse._();
  @$core.pragma('dart2js:noInline')
  static StreamEventsResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<StreamEventsResponse>(
          StreamEventsResponse.$_createMessage);
  static StreamEventsResponse? _defaultInstance;

  /// Per-stream monotonic sequence starting at 1. The local ordering
  /// authority — clients detect gaps and must resubscribe/resync via Health.
  /// (Distinct from Envelope.sequence, which orders the DEVICE session.)
  @$pb.TagNumber(1)
  $fixnum.Int64 get seq => $_getI64(0);
  @$pb.TagNumber(1)
  set seq($fixnum.Int64 value) => $_setInt64(0, value);
  @$pb.TagNumber(1)
  $core.bool hasSeq() => $_has(0);
  @$pb.TagNumber(1)
  void clearSeq() => $_clearField(1);

  /// Daemon instance identifier; equals HandshakeResponse.daemon_generation for the
  /// serving daemon. A change means the daemon restarted under the client.
  @$pb.TagNumber(2)
  $fixnum.Int64 get daemonGeneration => $_getI64(1);
  @$pb.TagNumber(2)
  set daemonGeneration($fixnum.Int64 value) => $_setInt64(1, value);
  @$pb.TagNumber(2)
  $core.bool hasDaemonGeneration() => $_has(1);
  @$pb.TagNumber(2)
  void clearDaemonGeneration() => $_clearField(2);

  /// Opaque relayed device-protocol message (phonebridge.v1). The daemon
  /// forwards the Envelope verbatim; the UI decodes device payloads
  /// directly. Pass-through framing per DEC-018: this is payload relay,
  /// NOT a new transport for phonebridge.v1, and LocalEngine is never a
  /// device peer.
  @$pb.TagNumber(3)
  $1.Envelope get envelope => $_getN(2);
  @$pb.TagNumber(3)
  set envelope($1.Envelope value) => $_setField(3, value);
  @$pb.TagNumber(3)
  $core.bool hasEnvelope() => $_has(2);
  @$pb.TagNumber(3)
  void clearEnvelope() => $_clearField(3);
  @$pb.TagNumber(3)
  $1.Envelope ensureEnvelope() => $_ensure(2);

  /// Local session state transitions (connecting, streaming, reconnecting, etc.).
  @$pb.TagNumber(4)
  SessionEvent get sessionEvent => $_getN(3);
  @$pb.TagNumber(4)
  set sessionEvent(SessionEvent value) => $_setField(4, value);
  @$pb.TagNumber(4)
  $core.bool hasSessionEvent() => $_has(3);
  @$pb.TagNumber(4)
  void clearSessionEvent() => $_clearField(4);
  @$pb.TagNumber(4)
  SessionEvent ensureSessionEvent() => $_ensure(3);
}

/// LocalEvent represents a pushed local event containing the relayed device envelope.
/// On the wire, StreamEvents yields StreamEventsResponse (whose fields match LocalEvent)
/// to satisfy standard gRPC request/response naming conventions.
class LocalEvent extends $pb.GeneratedMessage {
  factory LocalEvent({
    $fixnum.Int64? seq,
    $fixnum.Int64? daemonGeneration,
    $1.Envelope? envelope,
    SessionEvent? sessionEvent,
  }) {
    final result = LocalEvent._();
    if (seq != null) result.seq = seq;
    if (daemonGeneration != null) result.daemonGeneration = daemonGeneration;
    if (envelope != null) result.envelope = envelope;
    if (sessionEvent != null) result.sessionEvent = sessionEvent;
    return result;
  }

  LocalEvent._();

  factory LocalEvent.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      LocalEvent()..mergeFromBuffer(data, registry);
  factory LocalEvent.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      LocalEvent()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'LocalEvent',
      package: const $pb.PackageName(
          _omitMessageNames ? '' : 'phonebridge.localipc.v1'),
      createEmptyInstance: LocalEvent.$_createMessage)
    ..a<$fixnum.Int64>(1, _omitFieldNames ? '' : 'seq', $pb.PbFieldType.OU6,
        defaultOrMaker: $fixnum.Int64.ZERO)
    ..a<$fixnum.Int64>(
        2, _omitFieldNames ? '' : 'daemonGeneration', $pb.PbFieldType.OU6,
        defaultOrMaker: $fixnum.Int64.ZERO)
    ..aOM<$1.Envelope>(3, _omitFieldNames ? '' : 'envelope',
        subBuilder: $1.Envelope.$_createMessage)
    ..aOM<SessionEvent>(4, _omitFieldNames ? '' : 'sessionEvent',
        subBuilder: SessionEvent.$_createMessage)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  LocalEvent clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  LocalEvent copyWith(void Function(LocalEvent) updates) =>
      super.copyWith((message) => updates(message as LocalEvent)) as LocalEvent;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated('Use LocalEvent() / LocalEvent.new instead')
  static LocalEvent create() => LocalEvent._();
  static $pb.GeneratedMessage $_createMessage() => LocalEvent._();
  @$core.override
  LocalEvent createEmptyInstance() => LocalEvent._();
  @$core.pragma('dart2js:noInline')
  static LocalEvent getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<LocalEvent>(LocalEvent.$_createMessage);
  static LocalEvent? _defaultInstance;

  /// Per-stream monotonic sequence starting at 1.
  @$pb.TagNumber(1)
  $fixnum.Int64 get seq => $_getI64(0);
  @$pb.TagNumber(1)
  set seq($fixnum.Int64 value) => $_setInt64(0, value);
  @$pb.TagNumber(1)
  $core.bool hasSeq() => $_has(0);
  @$pb.TagNumber(1)
  void clearSeq() => $_clearField(1);

  /// Daemon instance identifier; equals HandshakeResponse.daemon_generation.
  @$pb.TagNumber(2)
  $fixnum.Int64 get daemonGeneration => $_getI64(1);
  @$pb.TagNumber(2)
  set daemonGeneration($fixnum.Int64 value) => $_setInt64(1, value);
  @$pb.TagNumber(2)
  $core.bool hasDaemonGeneration() => $_has(1);
  @$pb.TagNumber(2)
  void clearDaemonGeneration() => $_clearField(2);

  /// Opaque relayed device-protocol message (phonebridge.v1).
  @$pb.TagNumber(3)
  $1.Envelope get envelope => $_getN(2);
  @$pb.TagNumber(3)
  set envelope($1.Envelope value) => $_setField(3, value);
  @$pb.TagNumber(3)
  $core.bool hasEnvelope() => $_has(2);
  @$pb.TagNumber(3)
  void clearEnvelope() => $_clearField(3);
  @$pb.TagNumber(3)
  $1.Envelope ensureEnvelope() => $_ensure(2);

  /// Local session state transitions.
  @$pb.TagNumber(4)
  SessionEvent get sessionEvent => $_getN(3);
  @$pb.TagNumber(4)
  set sessionEvent(SessionEvent value) => $_setField(4, value);
  @$pb.TagNumber(4)
  $core.bool hasSessionEvent() => $_has(3);
  @$pb.TagNumber(4)
  void clearSessionEvent() => $_clearField(4);
  @$pb.TagNumber(4)
  SessionEvent ensureSessionEvent() => $_ensure(3);
}

/// HealthRequest is empty.
class HealthRequest extends $pb.GeneratedMessage {
  factory HealthRequest() => HealthRequest._();

  HealthRequest._();

  factory HealthRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      HealthRequest()..mergeFromBuffer(data, registry);
  factory HealthRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      HealthRequest()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'HealthRequest',
      package: const $pb.PackageName(
          _omitMessageNames ? '' : 'phonebridge.localipc.v1'),
      createEmptyInstance: HealthRequest.$_createMessage)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  HealthRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  HealthRequest copyWith(void Function(HealthRequest) updates) =>
      super.copyWith((message) => updates(message as HealthRequest))
          as HealthRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated('Use HealthRequest() / HealthRequest.new instead')
  static HealthRequest create() => HealthRequest._();
  static $pb.GeneratedMessage $_createMessage() => HealthRequest._();
  @$core.override
  HealthRequest createEmptyInstance() => HealthRequest._();
  @$core.pragma('dart2js:noInline')
  static HealthRequest getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<HealthRequest>(
          HealthRequest.$_createMessage);
  static HealthRequest? _defaultInstance;
}

/// HealthResponse answers Health.
class HealthResponse extends $pb.GeneratedMessage {
  factory HealthResponse({
    $core.bool? ready,
    $fixnum.Int64? daemonGeneration,
    $core.String? serverVersion,
    $fixnum.Int64? uptimeMs,
  }) {
    final result = HealthResponse._();
    if (ready != null) result.ready = ready;
    if (daemonGeneration != null) result.daemonGeneration = daemonGeneration;
    if (serverVersion != null) result.serverVersion = serverVersion;
    if (uptimeMs != null) result.uptimeMs = uptimeMs;
    return result;
  }

  HealthResponse._();

  factory HealthResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      HealthResponse()..mergeFromBuffer(data, registry);
  factory HealthResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      HealthResponse()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'HealthResponse',
      package: const $pb.PackageName(
          _omitMessageNames ? '' : 'phonebridge.localipc.v1'),
      createEmptyInstance: HealthResponse.$_createMessage)
    ..aOB(1, _omitFieldNames ? '' : 'ready')
    ..a<$fixnum.Int64>(
        2, _omitFieldNames ? '' : 'daemonGeneration', $pb.PbFieldType.OU6,
        defaultOrMaker: $fixnum.Int64.ZERO)
    ..aOS(3, _omitFieldNames ? '' : 'serverVersion')
    ..a<$fixnum.Int64>(
        4, _omitFieldNames ? '' : 'uptimeMs', $pb.PbFieldType.OU6,
        defaultOrMaker: $fixnum.Int64.ZERO)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  HealthResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  HealthResponse copyWith(void Function(HealthResponse) updates) =>
      super.copyWith((message) => updates(message as HealthResponse))
          as HealthResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated('Use HealthResponse() / HealthResponse.new instead')
  static HealthResponse create() => HealthResponse._();
  static $pb.GeneratedMessage $_createMessage() => HealthResponse._();
  @$core.override
  HealthResponse createEmptyInstance() => HealthResponse._();
  @$core.pragma('dart2js:noInline')
  static HealthResponse getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<HealthResponse>(
          HealthResponse.$_createMessage);
  static HealthResponse? _defaultInstance;

  /// True once the daemon is serving (its READY state).
  @$pb.TagNumber(1)
  $core.bool get ready => $_getBF(0);
  @$pb.TagNumber(1)
  set ready($core.bool value) => $_setBool(0, value);
  @$pb.TagNumber(1)
  $core.bool hasReady() => $_has(0);
  @$pb.TagNumber(1)
  void clearReady() => $_clearField(1);

  /// Daemon instance identifier (see HandshakeResponse.daemon_generation).
  @$pb.TagNumber(2)
  $fixnum.Int64 get daemonGeneration => $_getI64(1);
  @$pb.TagNumber(2)
  set daemonGeneration($fixnum.Int64 value) => $_setInt64(1, value);
  @$pb.TagNumber(2)
  $core.bool hasDaemonGeneration() => $_has(1);
  @$pb.TagNumber(2)
  void clearDaemonGeneration() => $_clearField(2);

  /// Daemon build/version string.
  @$pb.TagNumber(3)
  $core.String get serverVersion => $_getSZ(2);
  @$pb.TagNumber(3)
  set serverVersion($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasServerVersion() => $_has(2);
  @$pb.TagNumber(3)
  void clearServerVersion() => $_clearField(3);

  /// Milliseconds since daemon start (informational).
  @$pb.TagNumber(4)
  $fixnum.Int64 get uptimeMs => $_getI64(3);
  @$pb.TagNumber(4)
  set uptimeMs($fixnum.Int64 value) => $_setInt64(3, value);
  @$pb.TagNumber(4)
  $core.bool hasUptimeMs() => $_has(3);
  @$pb.TagNumber(4)
  void clearUptimeMs() => $_clearField(4);
}

/// SessionEvent represents a state transition pushed to StreamEvents subscribers.
class SessionEvent extends $pb.GeneratedMessage {
  factory SessionEvent({
    $core.String? sessionId,
    SessionState? state,
    $core.String? reason,
    $core.String? errorMessage,
    SessionReason? reasonCode,
  }) {
    final result = SessionEvent._();
    if (sessionId != null) result.sessionId = sessionId;
    if (state != null) result.state = state;
    if (reason != null) result.reason = reason;
    if (errorMessage != null) result.errorMessage = errorMessage;
    if (reasonCode != null) result.reasonCode = reasonCode;
    return result;
  }

  SessionEvent._();

  factory SessionEvent.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      SessionEvent()..mergeFromBuffer(data, registry);
  factory SessionEvent.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      SessionEvent()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'SessionEvent',
      package: const $pb.PackageName(
          _omitMessageNames ? '' : 'phonebridge.localipc.v1'),
      createEmptyInstance: SessionEvent.$_createMessage)
    ..aOS(1, _omitFieldNames ? '' : 'sessionId')
    ..aE<SessionState>(2, _omitFieldNames ? '' : 'state',
        enumValues: SessionState.values)
    ..aOS(3, _omitFieldNames ? '' : 'reason')
    ..aOS(4, _omitFieldNames ? '' : 'errorMessage')
    ..aE<SessionReason>(5, _omitFieldNames ? '' : 'reasonCode',
        enumValues: SessionReason.values)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  SessionEvent clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  SessionEvent copyWith(void Function(SessionEvent) updates) =>
      super.copyWith((message) => updates(message as SessionEvent))
          as SessionEvent;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated('Use SessionEvent() / SessionEvent.new instead')
  static SessionEvent create() => SessionEvent._();
  static $pb.GeneratedMessage $_createMessage() => SessionEvent._();
  @$core.override
  SessionEvent createEmptyInstance() => SessionEvent._();
  @$core.pragma('dart2js:noInline')
  static SessionEvent getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<SessionEvent>(
          SessionEvent.$_createMessage);
  static SessionEvent? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get sessionId => $_getSZ(0);
  @$pb.TagNumber(1)
  set sessionId($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasSessionId() => $_has(0);
  @$pb.TagNumber(1)
  void clearSessionId() => $_clearField(1);

  @$pb.TagNumber(2)
  SessionState get state => $_getN(1);
  @$pb.TagNumber(2)
  set state(SessionState value) => $_setField(2, value);
  @$pb.TagNumber(2)
  $core.bool hasState() => $_has(1);
  @$pb.TagNumber(2)
  void clearState() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get reason => $_getSZ(2);
  @$pb.TagNumber(3)
  set reason($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasReason() => $_has(2);
  @$pb.TagNumber(3)
  void clearReason() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.String get errorMessage => $_getSZ(3);
  @$pb.TagNumber(4)
  set errorMessage($core.String value) => $_setString(3, value);
  @$pb.TagNumber(4)
  $core.bool hasErrorMessage() => $_has(3);
  @$pb.TagNumber(4)
  void clearErrorMessage() => $_clearField(4);

  /// Typed classification of this transition (see SessionReason).
  @$pb.TagNumber(5)
  SessionReason get reasonCode => $_getN(4);
  @$pb.TagNumber(5)
  set reasonCode(SessionReason value) => $_setField(5, value);
  @$pb.TagNumber(5)
  $core.bool hasReasonCode() => $_has(4);
  @$pb.TagNumber(5)
  void clearReasonCode() => $_clearField(5);
}

/// StartSessionRequest initiates a session with the specified target device.
class StartSessionRequest extends $pb.GeneratedMessage {
  factory StartSessionRequest({
    $core.String? deviceId,
    $1.MediaParams? requested,
  }) {
    final result = StartSessionRequest._();
    if (deviceId != null) result.deviceId = deviceId;
    if (requested != null) result.requested = requested;
    return result;
  }

  StartSessionRequest._();

  factory StartSessionRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      StartSessionRequest()..mergeFromBuffer(data, registry);
  factory StartSessionRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      StartSessionRequest()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'StartSessionRequest',
      package: const $pb.PackageName(
          _omitMessageNames ? '' : 'phonebridge.localipc.v1'),
      createEmptyInstance: StartSessionRequest.$_createMessage)
    ..aOS(1, _omitFieldNames ? '' : 'deviceId')
    ..aOM<$1.MediaParams>(2, _omitFieldNames ? '' : 'requested',
        subBuilder: $1.MediaParams.$_createMessage)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  StartSessionRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  StartSessionRequest copyWith(void Function(StartSessionRequest) updates) =>
      super.copyWith((message) => updates(message as StartSessionRequest))
          as StartSessionRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core
      .Deprecated('Use StartSessionRequest() / StartSessionRequest.new instead')
  static StartSessionRequest create() => StartSessionRequest._();
  static $pb.GeneratedMessage $_createMessage() => StartSessionRequest._();
  @$core.override
  StartSessionRequest createEmptyInstance() => StartSessionRequest._();
  @$core.pragma('dart2js:noInline')
  static StartSessionRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<StartSessionRequest>(
          StartSessionRequest.$_createMessage);
  static StartSessionRequest? _defaultInstance;

  /// Unique target device ID (from mDNS discovery).
  @$pb.TagNumber(1)
  $core.String get deviceId => $_getSZ(0);
  @$pb.TagNumber(1)
  set deviceId($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasDeviceId() => $_has(0);
  @$pb.TagNumber(1)
  void clearDeviceId() => $_clearField(1);

  /// Screen-stream parameters the caller wants. Unset (or all-zero) fields
  /// mean "use the daemon default". The capture device answers with the values
  /// it actually applied; a request it cannot meet exactly is either reported
  /// as a downgrade or rejected with SESSION_REASON_UNSUPPORTED_MEDIA_PARAMS.
  @$pb.TagNumber(2)
  $1.MediaParams get requested => $_getN(1);
  @$pb.TagNumber(2)
  set requested($1.MediaParams value) => $_setField(2, value);
  @$pb.TagNumber(2)
  $core.bool hasRequested() => $_has(1);
  @$pb.TagNumber(2)
  void clearRequested() => $_clearField(2);
  @$pb.TagNumber(2)
  $1.MediaParams ensureRequested() => $_ensure(1);
}

/// StartSessionResponse returns the newly initiated session identity and state.
class StartSessionResponse extends $pb.GeneratedMessage {
  factory StartSessionResponse({
    $core.String? sessionId,
    SessionState? state,
  }) {
    final result = StartSessionResponse._();
    if (sessionId != null) result.sessionId = sessionId;
    if (state != null) result.state = state;
    return result;
  }

  StartSessionResponse._();

  factory StartSessionResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      StartSessionResponse()..mergeFromBuffer(data, registry);
  factory StartSessionResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      StartSessionResponse()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'StartSessionResponse',
      package: const $pb.PackageName(
          _omitMessageNames ? '' : 'phonebridge.localipc.v1'),
      createEmptyInstance: StartSessionResponse.$_createMessage)
    ..aOS(1, _omitFieldNames ? '' : 'sessionId')
    ..aE<SessionState>(2, _omitFieldNames ? '' : 'state',
        enumValues: SessionState.values)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  StartSessionResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  StartSessionResponse copyWith(void Function(StartSessionResponse) updates) =>
      super.copyWith((message) => updates(message as StartSessionResponse))
          as StartSessionResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated(
      'Use StartSessionResponse() / StartSessionResponse.new instead')
  static StartSessionResponse create() => StartSessionResponse._();
  static $pb.GeneratedMessage $_createMessage() => StartSessionResponse._();
  @$core.override
  StartSessionResponse createEmptyInstance() => StartSessionResponse._();
  @$core.pragma('dart2js:noInline')
  static StartSessionResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<StartSessionResponse>(
          StartSessionResponse.$_createMessage);
  static StartSessionResponse? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get sessionId => $_getSZ(0);
  @$pb.TagNumber(1)
  set sessionId($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasSessionId() => $_has(0);
  @$pb.TagNumber(1)
  void clearSessionId() => $_clearField(1);

  @$pb.TagNumber(2)
  SessionState get state => $_getN(1);
  @$pb.TagNumber(2)
  set state(SessionState value) => $_setField(2, value);
  @$pb.TagNumber(2)
  $core.bool hasState() => $_has(1);
  @$pb.TagNumber(2)
  void clearState() => $_clearField(2);
}

/// StopSessionRequest halts an active or in-progress session.
class StopSessionRequest extends $pb.GeneratedMessage {
  factory StopSessionRequest({
    $core.String? sessionId,
    $core.String? reason,
  }) {
    final result = StopSessionRequest._();
    if (sessionId != null) result.sessionId = sessionId;
    if (reason != null) result.reason = reason;
    return result;
  }

  StopSessionRequest._();

  factory StopSessionRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      StopSessionRequest()..mergeFromBuffer(data, registry);
  factory StopSessionRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      StopSessionRequest()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'StopSessionRequest',
      package: const $pb.PackageName(
          _omitMessageNames ? '' : 'phonebridge.localipc.v1'),
      createEmptyInstance: StopSessionRequest.$_createMessage)
    ..aOS(1, _omitFieldNames ? '' : 'sessionId')
    ..aOS(2, _omitFieldNames ? '' : 'reason')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  StopSessionRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  StopSessionRequest copyWith(void Function(StopSessionRequest) updates) =>
      super.copyWith((message) => updates(message as StopSessionRequest))
          as StopSessionRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated('Use StopSessionRequest() / StopSessionRequest.new instead')
  static StopSessionRequest create() => StopSessionRequest._();
  static $pb.GeneratedMessage $_createMessage() => StopSessionRequest._();
  @$core.override
  StopSessionRequest createEmptyInstance() => StopSessionRequest._();
  @$core.pragma('dart2js:noInline')
  static StopSessionRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<StopSessionRequest>(
          StopSessionRequest.$_createMessage);
  static StopSessionRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get sessionId => $_getSZ(0);
  @$pb.TagNumber(1)
  set sessionId($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasSessionId() => $_has(0);
  @$pb.TagNumber(1)
  void clearSessionId() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get reason => $_getSZ(1);
  @$pb.TagNumber(2)
  set reason($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasReason() => $_has(1);
  @$pb.TagNumber(2)
  void clearReason() => $_clearField(2);
}

/// StopSessionResponse confirms session termination.
class StopSessionResponse extends $pb.GeneratedMessage {
  factory StopSessionResponse({
    $core.String? sessionId,
    SessionState? state,
  }) {
    final result = StopSessionResponse._();
    if (sessionId != null) result.sessionId = sessionId;
    if (state != null) result.state = state;
    return result;
  }

  StopSessionResponse._();

  factory StopSessionResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      StopSessionResponse()..mergeFromBuffer(data, registry);
  factory StopSessionResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      StopSessionResponse()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'StopSessionResponse',
      package: const $pb.PackageName(
          _omitMessageNames ? '' : 'phonebridge.localipc.v1'),
      createEmptyInstance: StopSessionResponse.$_createMessage)
    ..aOS(1, _omitFieldNames ? '' : 'sessionId')
    ..aE<SessionState>(2, _omitFieldNames ? '' : 'state',
        enumValues: SessionState.values)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  StopSessionResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  StopSessionResponse copyWith(void Function(StopSessionResponse) updates) =>
      super.copyWith((message) => updates(message as StopSessionResponse))
          as StopSessionResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core
      .Deprecated('Use StopSessionResponse() / StopSessionResponse.new instead')
  static StopSessionResponse create() => StopSessionResponse._();
  static $pb.GeneratedMessage $_createMessage() => StopSessionResponse._();
  @$core.override
  StopSessionResponse createEmptyInstance() => StopSessionResponse._();
  @$core.pragma('dart2js:noInline')
  static StopSessionResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<StopSessionResponse>(
          StopSessionResponse.$_createMessage);
  static StopSessionResponse? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get sessionId => $_getSZ(0);
  @$pb.TagNumber(1)
  set sessionId($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasSessionId() => $_has(0);
  @$pb.TagNumber(1)
  void clearSessionId() => $_clearField(1);

  @$pb.TagNumber(2)
  SessionState get state => $_getN(1);
  @$pb.TagNumber(2)
  set state(SessionState value) => $_setField(2, value);
  @$pb.TagNumber(2)
  $core.bool hasState() => $_has(1);
  @$pb.TagNumber(2)
  void clearState() => $_clearField(2);
}

/// GetSessionStateRequest queries current session status.
class GetSessionStateRequest extends $pb.GeneratedMessage {
  factory GetSessionStateRequest({
    $core.String? sessionId,
  }) {
    final result = GetSessionStateRequest._();
    if (sessionId != null) result.sessionId = sessionId;
    return result;
  }

  GetSessionStateRequest._();

  factory GetSessionStateRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      GetSessionStateRequest()..mergeFromBuffer(data, registry);
  factory GetSessionStateRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      GetSessionStateRequest()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'GetSessionStateRequest',
      package: const $pb.PackageName(
          _omitMessageNames ? '' : 'phonebridge.localipc.v1'),
      createEmptyInstance: GetSessionStateRequest.$_createMessage)
    ..aOS(1, _omitFieldNames ? '' : 'sessionId')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  GetSessionStateRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  GetSessionStateRequest copyWith(
          void Function(GetSessionStateRequest) updates) =>
      super.copyWith((message) => updates(message as GetSessionStateRequest))
          as GetSessionStateRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated(
      'Use GetSessionStateRequest() / GetSessionStateRequest.new instead')
  static GetSessionStateRequest create() => GetSessionStateRequest._();
  static $pb.GeneratedMessage $_createMessage() => GetSessionStateRequest._();
  @$core.override
  GetSessionStateRequest createEmptyInstance() => GetSessionStateRequest._();
  @$core.pragma('dart2js:noInline')
  static GetSessionStateRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<GetSessionStateRequest>(
          GetSessionStateRequest.$_createMessage);
  static GetSessionStateRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get sessionId => $_getSZ(0);
  @$pb.TagNumber(1)
  set sessionId($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasSessionId() => $_has(0);
  @$pb.TagNumber(1)
  void clearSessionId() => $_clearField(1);
}

/// StreamStats reports point-in-time WebRTC media and network counters.
class StreamStats extends $pb.GeneratedMessage {
  factory StreamStats({
    $fixnum.Int64? packets,
    $fixnum.Int64? bytesRtp,
    $fixnum.Int64? bytesH264,
    $fixnum.Int64? accessUnits,
    $fixnum.Int64? keyframes,
    $fixnum.Int64? droppedAus,
  }) {
    final result = StreamStats._();
    if (packets != null) result.packets = packets;
    if (bytesRtp != null) result.bytesRtp = bytesRtp;
    if (bytesH264 != null) result.bytesH264 = bytesH264;
    if (accessUnits != null) result.accessUnits = accessUnits;
    if (keyframes != null) result.keyframes = keyframes;
    if (droppedAus != null) result.droppedAus = droppedAus;
    return result;
  }

  StreamStats._();

  factory StreamStats.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      StreamStats()..mergeFromBuffer(data, registry);
  factory StreamStats.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      StreamStats()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'StreamStats',
      package: const $pb.PackageName(
          _omitMessageNames ? '' : 'phonebridge.localipc.v1'),
      createEmptyInstance: StreamStats.$_createMessage)
    ..a<$fixnum.Int64>(1, _omitFieldNames ? '' : 'packets', $pb.PbFieldType.OU6,
        defaultOrMaker: $fixnum.Int64.ZERO)
    ..a<$fixnum.Int64>(
        2, _omitFieldNames ? '' : 'bytesRtp', $pb.PbFieldType.OU6,
        defaultOrMaker: $fixnum.Int64.ZERO)
    ..a<$fixnum.Int64>(
        3, _omitFieldNames ? '' : 'bytesH264', $pb.PbFieldType.OU6,
        defaultOrMaker: $fixnum.Int64.ZERO)
    ..a<$fixnum.Int64>(
        4, _omitFieldNames ? '' : 'accessUnits', $pb.PbFieldType.OU6,
        defaultOrMaker: $fixnum.Int64.ZERO)
    ..a<$fixnum.Int64>(
        5, _omitFieldNames ? '' : 'keyframes', $pb.PbFieldType.OU6,
        defaultOrMaker: $fixnum.Int64.ZERO)
    ..aInt64(6, _omitFieldNames ? '' : 'droppedAus')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  StreamStats clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  StreamStats copyWith(void Function(StreamStats) updates) =>
      super.copyWith((message) => updates(message as StreamStats))
          as StreamStats;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated('Use StreamStats() / StreamStats.new instead')
  static StreamStats create() => StreamStats._();
  static $pb.GeneratedMessage $_createMessage() => StreamStats._();
  @$core.override
  StreamStats createEmptyInstance() => StreamStats._();
  @$core.pragma('dart2js:noInline')
  static StreamStats getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<StreamStats>(
          StreamStats.$_createMessage);
  static StreamStats? _defaultInstance;

  @$pb.TagNumber(1)
  $fixnum.Int64 get packets => $_getI64(0);
  @$pb.TagNumber(1)
  set packets($fixnum.Int64 value) => $_setInt64(0, value);
  @$pb.TagNumber(1)
  $core.bool hasPackets() => $_has(0);
  @$pb.TagNumber(1)
  void clearPackets() => $_clearField(1);

  @$pb.TagNumber(2)
  $fixnum.Int64 get bytesRtp => $_getI64(1);
  @$pb.TagNumber(2)
  set bytesRtp($fixnum.Int64 value) => $_setInt64(1, value);
  @$pb.TagNumber(2)
  $core.bool hasBytesRtp() => $_has(1);
  @$pb.TagNumber(2)
  void clearBytesRtp() => $_clearField(2);

  @$pb.TagNumber(3)
  $fixnum.Int64 get bytesH264 => $_getI64(2);
  @$pb.TagNumber(3)
  set bytesH264($fixnum.Int64 value) => $_setInt64(2, value);
  @$pb.TagNumber(3)
  $core.bool hasBytesH264() => $_has(2);
  @$pb.TagNumber(3)
  void clearBytesH264() => $_clearField(3);

  @$pb.TagNumber(4)
  $fixnum.Int64 get accessUnits => $_getI64(3);
  @$pb.TagNumber(4)
  set accessUnits($fixnum.Int64 value) => $_setInt64(3, value);
  @$pb.TagNumber(4)
  $core.bool hasAccessUnits() => $_has(3);
  @$pb.TagNumber(4)
  void clearAccessUnits() => $_clearField(4);

  @$pb.TagNumber(5)
  $fixnum.Int64 get keyframes => $_getI64(4);
  @$pb.TagNumber(5)
  set keyframes($fixnum.Int64 value) => $_setInt64(4, value);
  @$pb.TagNumber(5)
  $core.bool hasKeyframes() => $_has(4);
  @$pb.TagNumber(5)
  void clearKeyframes() => $_clearField(5);

  @$pb.TagNumber(6)
  $fixnum.Int64 get droppedAus => $_getI64(5);
  @$pb.TagNumber(6)
  set droppedAus($fixnum.Int64 value) => $_setInt64(5, value);
  @$pb.TagNumber(6)
  $core.bool hasDroppedAus() => $_has(5);
  @$pb.TagNumber(6)
  void clearDroppedAus() => $_clearField(6);
}

/// GetSessionStateResponse contains a full point-in-time snapshot.
class GetSessionStateResponse extends $pb.GeneratedMessage {
  factory GetSessionStateResponse({
    $core.String? sessionId,
    SessionState? state,
    $core.String? deviceId,
    $fixnum.Int64? connectedDurationMs,
    $core.String? errorMessage,
    StreamStats? stats,
    $1.MediaParams? requested,
    $1.MediaParams? actual,
    SessionReason? reasonCode,
    $core.int? reconnectAttempts,
  }) {
    final result = GetSessionStateResponse._();
    if (sessionId != null) result.sessionId = sessionId;
    if (state != null) result.state = state;
    if (deviceId != null) result.deviceId = deviceId;
    if (connectedDurationMs != null)
      result.connectedDurationMs = connectedDurationMs;
    if (errorMessage != null) result.errorMessage = errorMessage;
    if (stats != null) result.stats = stats;
    if (requested != null) result.requested = requested;
    if (actual != null) result.actual = actual;
    if (reasonCode != null) result.reasonCode = reasonCode;
    if (reconnectAttempts != null) result.reconnectAttempts = reconnectAttempts;
    return result;
  }

  GetSessionStateResponse._();

  factory GetSessionStateResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      GetSessionStateResponse()..mergeFromBuffer(data, registry);
  factory GetSessionStateResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      GetSessionStateResponse()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'GetSessionStateResponse',
      package: const $pb.PackageName(
          _omitMessageNames ? '' : 'phonebridge.localipc.v1'),
      createEmptyInstance: GetSessionStateResponse.$_createMessage)
    ..aOS(1, _omitFieldNames ? '' : 'sessionId')
    ..aE<SessionState>(2, _omitFieldNames ? '' : 'state',
        enumValues: SessionState.values)
    ..aOS(3, _omitFieldNames ? '' : 'deviceId')
    ..a<$fixnum.Int64>(
        4, _omitFieldNames ? '' : 'connectedDurationMs', $pb.PbFieldType.OU6,
        defaultOrMaker: $fixnum.Int64.ZERO)
    ..aOS(5, _omitFieldNames ? '' : 'errorMessage')
    ..aOM<StreamStats>(6, _omitFieldNames ? '' : 'stats',
        subBuilder: StreamStats.$_createMessage)
    ..aOM<$1.MediaParams>(7, _omitFieldNames ? '' : 'requested',
        subBuilder: $1.MediaParams.$_createMessage)
    ..aOM<$1.MediaParams>(8, _omitFieldNames ? '' : 'actual',
        subBuilder: $1.MediaParams.$_createMessage)
    ..aE<SessionReason>(9, _omitFieldNames ? '' : 'reasonCode',
        enumValues: SessionReason.values)
    ..aI(10, _omitFieldNames ? '' : 'reconnectAttempts',
        fieldType: $pb.PbFieldType.OU3)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  GetSessionStateResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  GetSessionStateResponse copyWith(
          void Function(GetSessionStateResponse) updates) =>
      super.copyWith((message) => updates(message as GetSessionStateResponse))
          as GetSessionStateResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated(
      'Use GetSessionStateResponse() / GetSessionStateResponse.new instead')
  static GetSessionStateResponse create() => GetSessionStateResponse._();
  static $pb.GeneratedMessage $_createMessage() => GetSessionStateResponse._();
  @$core.override
  GetSessionStateResponse createEmptyInstance() => GetSessionStateResponse._();
  @$core.pragma('dart2js:noInline')
  static GetSessionStateResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<GetSessionStateResponse>(
          GetSessionStateResponse.$_createMessage);
  static GetSessionStateResponse? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get sessionId => $_getSZ(0);
  @$pb.TagNumber(1)
  set sessionId($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasSessionId() => $_has(0);
  @$pb.TagNumber(1)
  void clearSessionId() => $_clearField(1);

  @$pb.TagNumber(2)
  SessionState get state => $_getN(1);
  @$pb.TagNumber(2)
  set state(SessionState value) => $_setField(2, value);
  @$pb.TagNumber(2)
  $core.bool hasState() => $_has(1);
  @$pb.TagNumber(2)
  void clearState() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get deviceId => $_getSZ(2);
  @$pb.TagNumber(3)
  set deviceId($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasDeviceId() => $_has(2);
  @$pb.TagNumber(3)
  void clearDeviceId() => $_clearField(3);

  @$pb.TagNumber(4)
  $fixnum.Int64 get connectedDurationMs => $_getI64(3);
  @$pb.TagNumber(4)
  set connectedDurationMs($fixnum.Int64 value) => $_setInt64(3, value);
  @$pb.TagNumber(4)
  $core.bool hasConnectedDurationMs() => $_has(3);
  @$pb.TagNumber(4)
  void clearConnectedDurationMs() => $_clearField(4);

  @$pb.TagNumber(5)
  $core.String get errorMessage => $_getSZ(4);
  @$pb.TagNumber(5)
  set errorMessage($core.String value) => $_setString(4, value);
  @$pb.TagNumber(5)
  $core.bool hasErrorMessage() => $_has(4);
  @$pb.TagNumber(5)
  void clearErrorMessage() => $_clearField(5);

  @$pb.TagNumber(6)
  StreamStats get stats => $_getN(5);
  @$pb.TagNumber(6)
  set stats(StreamStats value) => $_setField(6, value);
  @$pb.TagNumber(6)
  $core.bool hasStats() => $_has(5);
  @$pb.TagNumber(6)
  void clearStats() => $_clearField(6);
  @$pb.TagNumber(6)
  StreamStats ensureStats() => $_ensure(5);

  /// What the caller asked for when the session was started.
  @$pb.TagNumber(7)
  $1.MediaParams get requested => $_getN(6);
  @$pb.TagNumber(7)
  set requested($1.MediaParams value) => $_setField(7, value);
  @$pb.TagNumber(7)
  $core.bool hasRequested() => $_has(6);
  @$pb.TagNumber(7)
  void clearRequested() => $_clearField(7);
  @$pb.TagNumber(7)
  $1.MediaParams ensureRequested() => $_ensure(6);

  /// What the capture device actually applied (authoritative, DEC-020). An
  /// `actual` that differs from `requested` is a reported downgrade, never a
  /// silent substitution. Both are unset until negotiation completes.
  @$pb.TagNumber(8)
  $1.MediaParams get actual => $_getN(7);
  @$pb.TagNumber(8)
  set actual($1.MediaParams value) => $_setField(8, value);
  @$pb.TagNumber(8)
  $core.bool hasActual() => $_has(7);
  @$pb.TagNumber(8)
  void clearActual() => $_clearField(8);
  @$pb.TagNumber(8)
  $1.MediaParams ensureActual() => $_ensure(7);

  /// Typed classification of the current state (see SessionReason).
  @$pb.TagNumber(9)
  SessionReason get reasonCode => $_getN(8);
  @$pb.TagNumber(9)
  set reasonCode(SessionReason value) => $_setField(9, value);
  @$pb.TagNumber(9)
  $core.bool hasReasonCode() => $_has(8);
  @$pb.TagNumber(9)
  void clearReasonCode() => $_clearField(9);

  /// Reconnect attempts made in the current recovery window (0 when healthy).
  /// Exposed so recovery is observable from the UI and from tests rather than
  /// inferred from a state label.
  @$pb.TagNumber(10)
  $core.int get reconnectAttempts => $_getIZ(9);
  @$pb.TagNumber(10)
  set reconnectAttempts($core.int value) => $_setUnsignedInt32(9, value);
  @$pb.TagNumber(10)
  $core.bool hasReconnectAttempts() => $_has(9);
  @$pb.TagNumber(10)
  void clearReconnectAttempts() => $_clearField(10);
}

/// DiscoveredDevice models a LAN device discovered via mDNS.
class DiscoveredDevice extends $pb.GeneratedMessage {
  factory DiscoveredDevice({
    $core.String? id,
    $core.String? name,
    $core.String? model,
    $core.String? version,
    $core.Iterable<$core.String>? capabilities,
    $core.String? state,
    $core.String? address,
    $core.int? port,
    $core.bool? isStale,
  }) {
    final result = DiscoveredDevice._();
    if (id != null) result.id = id;
    if (name != null) result.name = name;
    if (model != null) result.model = model;
    if (version != null) result.version = version;
    if (capabilities != null) result.capabilities.addAll(capabilities);
    if (state != null) result.state = state;
    if (address != null) result.address = address;
    if (port != null) result.port = port;
    if (isStale != null) result.isStale = isStale;
    return result;
  }

  DiscoveredDevice._();

  factory DiscoveredDevice.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      DiscoveredDevice()..mergeFromBuffer(data, registry);
  factory DiscoveredDevice.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      DiscoveredDevice()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'DiscoveredDevice',
      package: const $pb.PackageName(
          _omitMessageNames ? '' : 'phonebridge.localipc.v1'),
      createEmptyInstance: DiscoveredDevice.$_createMessage)
    ..aOS(1, _omitFieldNames ? '' : 'id')
    ..aOS(2, _omitFieldNames ? '' : 'name')
    ..aOS(3, _omitFieldNames ? '' : 'model')
    ..aOS(4, _omitFieldNames ? '' : 'version')
    ..pPS(5, _omitFieldNames ? '' : 'capabilities')
    ..aOS(6, _omitFieldNames ? '' : 'state')
    ..aOS(7, _omitFieldNames ? '' : 'address')
    ..aI(8, _omitFieldNames ? '' : 'port', fieldType: $pb.PbFieldType.OU3)
    ..aOB(9, _omitFieldNames ? '' : 'isStale')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  DiscoveredDevice clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  DiscoveredDevice copyWith(void Function(DiscoveredDevice) updates) =>
      super.copyWith((message) => updates(message as DiscoveredDevice))
          as DiscoveredDevice;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated('Use DiscoveredDevice() / DiscoveredDevice.new instead')
  static DiscoveredDevice create() => DiscoveredDevice._();
  static $pb.GeneratedMessage $_createMessage() => DiscoveredDevice._();
  @$core.override
  DiscoveredDevice createEmptyInstance() => DiscoveredDevice._();
  @$core.pragma('dart2js:noInline')
  static DiscoveredDevice getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<DiscoveredDevice>(
          DiscoveredDevice.$_createMessage);
  static DiscoveredDevice? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get id => $_getSZ(0);
  @$pb.TagNumber(1)
  set id($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasId() => $_has(0);
  @$pb.TagNumber(1)
  void clearId() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get name => $_getSZ(1);
  @$pb.TagNumber(2)
  set name($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasName() => $_has(1);
  @$pb.TagNumber(2)
  void clearName() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get model => $_getSZ(2);
  @$pb.TagNumber(3)
  set model($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasModel() => $_has(2);
  @$pb.TagNumber(3)
  void clearModel() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.String get version => $_getSZ(3);
  @$pb.TagNumber(4)
  set version($core.String value) => $_setString(3, value);
  @$pb.TagNumber(4)
  $core.bool hasVersion() => $_has(3);
  @$pb.TagNumber(4)
  void clearVersion() => $_clearField(4);

  @$pb.TagNumber(5)
  $pb.PbList<$core.String> get capabilities => $_getList(4);

  @$pb.TagNumber(6)
  $core.String get state => $_getSZ(5);
  @$pb.TagNumber(6)
  set state($core.String value) => $_setString(5, value);
  @$pb.TagNumber(6)
  $core.bool hasState() => $_has(5);
  @$pb.TagNumber(6)
  void clearState() => $_clearField(6);

  @$pb.TagNumber(7)
  $core.String get address => $_getSZ(6);
  @$pb.TagNumber(7)
  set address($core.String value) => $_setString(6, value);
  @$pb.TagNumber(7)
  $core.bool hasAddress() => $_has(6);
  @$pb.TagNumber(7)
  void clearAddress() => $_clearField(7);

  @$pb.TagNumber(8)
  $core.int get port => $_getIZ(7);
  @$pb.TagNumber(8)
  set port($core.int value) => $_setUnsignedInt32(7, value);
  @$pb.TagNumber(8)
  $core.bool hasPort() => $_has(7);
  @$pb.TagNumber(8)
  void clearPort() => $_clearField(8);

  @$pb.TagNumber(9)
  $core.bool get isStale => $_getBF(8);
  @$pb.TagNumber(9)
  set isStale($core.bool value) => $_setBool(8, value);
  @$pb.TagNumber(9)
  $core.bool hasIsStale() => $_has(8);
  @$pb.TagNumber(9)
  void clearIsStale() => $_clearField(9);
}

/// ListDevicesRequest is empty.
class ListDevicesRequest extends $pb.GeneratedMessage {
  factory ListDevicesRequest() => ListDevicesRequest._();

  ListDevicesRequest._();

  factory ListDevicesRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      ListDevicesRequest()..mergeFromBuffer(data, registry);
  factory ListDevicesRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      ListDevicesRequest()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ListDevicesRequest',
      package: const $pb.PackageName(
          _omitMessageNames ? '' : 'phonebridge.localipc.v1'),
      createEmptyInstance: ListDevicesRequest.$_createMessage)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ListDevicesRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ListDevicesRequest copyWith(void Function(ListDevicesRequest) updates) =>
      super.copyWith((message) => updates(message as ListDevicesRequest))
          as ListDevicesRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated('Use ListDevicesRequest() / ListDevicesRequest.new instead')
  static ListDevicesRequest create() => ListDevicesRequest._();
  static $pb.GeneratedMessage $_createMessage() => ListDevicesRequest._();
  @$core.override
  ListDevicesRequest createEmptyInstance() => ListDevicesRequest._();
  @$core.pragma('dart2js:noInline')
  static ListDevicesRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ListDevicesRequest>(
          ListDevicesRequest.$_createMessage);
  static ListDevicesRequest? _defaultInstance;
}

/// ListDevicesResponse returns currently known LAN devices.
class ListDevicesResponse extends $pb.GeneratedMessage {
  factory ListDevicesResponse({
    $core.Iterable<DiscoveredDevice>? devices,
  }) {
    final result = ListDevicesResponse._();
    if (devices != null) result.devices.addAll(devices);
    return result;
  }

  ListDevicesResponse._();

  factory ListDevicesResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      ListDevicesResponse()..mergeFromBuffer(data, registry);
  factory ListDevicesResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      ListDevicesResponse()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ListDevicesResponse',
      package: const $pb.PackageName(
          _omitMessageNames ? '' : 'phonebridge.localipc.v1'),
      createEmptyInstance: ListDevicesResponse.$_createMessage)
    ..pPM<DiscoveredDevice>(1, _omitFieldNames ? '' : 'devices',
        subBuilder: DiscoveredDevice.$_createMessage)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ListDevicesResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ListDevicesResponse copyWith(void Function(ListDevicesResponse) updates) =>
      super.copyWith((message) => updates(message as ListDevicesResponse))
          as ListDevicesResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core
      .Deprecated('Use ListDevicesResponse() / ListDevicesResponse.new instead')
  static ListDevicesResponse create() => ListDevicesResponse._();
  static $pb.GeneratedMessage $_createMessage() => ListDevicesResponse._();
  @$core.override
  ListDevicesResponse createEmptyInstance() => ListDevicesResponse._();
  @$core.pragma('dart2js:noInline')
  static ListDevicesResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ListDevicesResponse>(
          ListDevicesResponse.$_createMessage);
  static ListDevicesResponse? _defaultInstance;

  @$pb.TagNumber(1)
  $pb.PbList<DiscoveredDevice> get devices => $_getList(0);
}

/// PairDeviceRequest initiates pairing with a device.
class PairDeviceRequest extends $pb.GeneratedMessage {
  factory PairDeviceRequest({
    $core.String? deviceId,
  }) {
    final result = PairDeviceRequest._();
    if (deviceId != null) result.deviceId = deviceId;
    return result;
  }

  PairDeviceRequest._();

  factory PairDeviceRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      PairDeviceRequest()..mergeFromBuffer(data, registry);
  factory PairDeviceRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      PairDeviceRequest()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'PairDeviceRequest',
      package: const $pb.PackageName(
          _omitMessageNames ? '' : 'phonebridge.localipc.v1'),
      createEmptyInstance: PairDeviceRequest.$_createMessage)
    ..aOS(1, _omitFieldNames ? '' : 'deviceId')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PairDeviceRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PairDeviceRequest copyWith(void Function(PairDeviceRequest) updates) =>
      super.copyWith((message) => updates(message as PairDeviceRequest))
          as PairDeviceRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated('Use PairDeviceRequest() / PairDeviceRequest.new instead')
  static PairDeviceRequest create() => PairDeviceRequest._();
  static $pb.GeneratedMessage $_createMessage() => PairDeviceRequest._();
  @$core.override
  PairDeviceRequest createEmptyInstance() => PairDeviceRequest._();
  @$core.pragma('dart2js:noInline')
  static PairDeviceRequest getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<PairDeviceRequest>(
          PairDeviceRequest.$_createMessage);
  static PairDeviceRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get deviceId => $_getSZ(0);
  @$pb.TagNumber(1)
  set deviceId($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasDeviceId() => $_has(0);
  @$pb.TagNumber(1)
  void clearDeviceId() => $_clearField(1);
}

/// PairDeviceResponse returns the pairing status and SAS for user verification.
class PairDeviceResponse extends $pb.GeneratedMessage {
  factory PairDeviceResponse({
    $core.String? deviceId,
    $core.String? displayName,
    $core.String? sas,
    $core.bool? pendingConfirmation,
  }) {
    final result = PairDeviceResponse._();
    if (deviceId != null) result.deviceId = deviceId;
    if (displayName != null) result.displayName = displayName;
    if (sas != null) result.sas = sas;
    if (pendingConfirmation != null)
      result.pendingConfirmation = pendingConfirmation;
    return result;
  }

  PairDeviceResponse._();

  factory PairDeviceResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      PairDeviceResponse()..mergeFromBuffer(data, registry);
  factory PairDeviceResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      PairDeviceResponse()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'PairDeviceResponse',
      package: const $pb.PackageName(
          _omitMessageNames ? '' : 'phonebridge.localipc.v1'),
      createEmptyInstance: PairDeviceResponse.$_createMessage)
    ..aOS(1, _omitFieldNames ? '' : 'deviceId')
    ..aOS(2, _omitFieldNames ? '' : 'displayName')
    ..aOS(3, _omitFieldNames ? '' : 'sas')
    ..aOB(4, _omitFieldNames ? '' : 'pendingConfirmation')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PairDeviceResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PairDeviceResponse copyWith(void Function(PairDeviceResponse) updates) =>
      super.copyWith((message) => updates(message as PairDeviceResponse))
          as PairDeviceResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated('Use PairDeviceResponse() / PairDeviceResponse.new instead')
  static PairDeviceResponse create() => PairDeviceResponse._();
  static $pb.GeneratedMessage $_createMessage() => PairDeviceResponse._();
  @$core.override
  PairDeviceResponse createEmptyInstance() => PairDeviceResponse._();
  @$core.pragma('dart2js:noInline')
  static PairDeviceResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<PairDeviceResponse>(
          PairDeviceResponse.$_createMessage);
  static PairDeviceResponse? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get deviceId => $_getSZ(0);
  @$pb.TagNumber(1)
  set deviceId($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasDeviceId() => $_has(0);
  @$pb.TagNumber(1)
  void clearDeviceId() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get displayName => $_getSZ(1);
  @$pb.TagNumber(2)
  set displayName($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasDisplayName() => $_has(1);
  @$pb.TagNumber(2)
  void clearDisplayName() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get sas => $_getSZ(2);
  @$pb.TagNumber(3)
  set sas($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasSas() => $_has(2);
  @$pb.TagNumber(3)
  void clearSas() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.bool get pendingConfirmation => $_getBF(3);
  @$pb.TagNumber(4)
  set pendingConfirmation($core.bool value) => $_setBool(3, value);
  @$pb.TagNumber(4)
  $core.bool hasPendingConfirmation() => $_has(3);
  @$pb.TagNumber(4)
  void clearPendingConfirmation() => $_clearField(4);
}

/// ConfirmPairingRequest conveys user confirmation of the SAS.
class ConfirmPairingRequest extends $pb.GeneratedMessage {
  factory ConfirmPairingRequest({
    $core.String? deviceId,
    $core.bool? userConfirmed,
  }) {
    final result = ConfirmPairingRequest._();
    if (deviceId != null) result.deviceId = deviceId;
    if (userConfirmed != null) result.userConfirmed = userConfirmed;
    return result;
  }

  ConfirmPairingRequest._();

  factory ConfirmPairingRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      ConfirmPairingRequest()..mergeFromBuffer(data, registry);
  factory ConfirmPairingRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      ConfirmPairingRequest()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ConfirmPairingRequest',
      package: const $pb.PackageName(
          _omitMessageNames ? '' : 'phonebridge.localipc.v1'),
      createEmptyInstance: ConfirmPairingRequest.$_createMessage)
    ..aOS(1, _omitFieldNames ? '' : 'deviceId')
    ..aOB(2, _omitFieldNames ? '' : 'userConfirmed')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ConfirmPairingRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ConfirmPairingRequest copyWith(
          void Function(ConfirmPairingRequest) updates) =>
      super.copyWith((message) => updates(message as ConfirmPairingRequest))
          as ConfirmPairingRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated(
      'Use ConfirmPairingRequest() / ConfirmPairingRequest.new instead')
  static ConfirmPairingRequest create() => ConfirmPairingRequest._();
  static $pb.GeneratedMessage $_createMessage() => ConfirmPairingRequest._();
  @$core.override
  ConfirmPairingRequest createEmptyInstance() => ConfirmPairingRequest._();
  @$core.pragma('dart2js:noInline')
  static ConfirmPairingRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ConfirmPairingRequest>(
          ConfirmPairingRequest.$_createMessage);
  static ConfirmPairingRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get deviceId => $_getSZ(0);
  @$pb.TagNumber(1)
  set deviceId($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasDeviceId() => $_has(0);
  @$pb.TagNumber(1)
  void clearDeviceId() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.bool get userConfirmed => $_getBF(1);
  @$pb.TagNumber(2)
  set userConfirmed($core.bool value) => $_setBool(1, value);
  @$pb.TagNumber(2)
  $core.bool hasUserConfirmed() => $_has(1);
  @$pb.TagNumber(2)
  void clearUserConfirmed() => $_clearField(2);
}

/// ConfirmPairingResponse returns the result of the pairing confirmation.
class ConfirmPairingResponse extends $pb.GeneratedMessage {
  factory ConfirmPairingResponse({
    $core.String? deviceId,
    $core.bool? success,
    $core.String? errorMessage,
  }) {
    final result = ConfirmPairingResponse._();
    if (deviceId != null) result.deviceId = deviceId;
    if (success != null) result.success = success;
    if (errorMessage != null) result.errorMessage = errorMessage;
    return result;
  }

  ConfirmPairingResponse._();

  factory ConfirmPairingResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      ConfirmPairingResponse()..mergeFromBuffer(data, registry);
  factory ConfirmPairingResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      ConfirmPairingResponse()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ConfirmPairingResponse',
      package: const $pb.PackageName(
          _omitMessageNames ? '' : 'phonebridge.localipc.v1'),
      createEmptyInstance: ConfirmPairingResponse.$_createMessage)
    ..aOS(1, _omitFieldNames ? '' : 'deviceId')
    ..aOB(2, _omitFieldNames ? '' : 'success')
    ..aOS(3, _omitFieldNames ? '' : 'errorMessage')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ConfirmPairingResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ConfirmPairingResponse copyWith(
          void Function(ConfirmPairingResponse) updates) =>
      super.copyWith((message) => updates(message as ConfirmPairingResponse))
          as ConfirmPairingResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated(
      'Use ConfirmPairingResponse() / ConfirmPairingResponse.new instead')
  static ConfirmPairingResponse create() => ConfirmPairingResponse._();
  static $pb.GeneratedMessage $_createMessage() => ConfirmPairingResponse._();
  @$core.override
  ConfirmPairingResponse createEmptyInstance() => ConfirmPairingResponse._();
  @$core.pragma('dart2js:noInline')
  static ConfirmPairingResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ConfirmPairingResponse>(
          ConfirmPairingResponse.$_createMessage);
  static ConfirmPairingResponse? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get deviceId => $_getSZ(0);
  @$pb.TagNumber(1)
  set deviceId($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasDeviceId() => $_has(0);
  @$pb.TagNumber(1)
  void clearDeviceId() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.bool get success => $_getBF(1);
  @$pb.TagNumber(2)
  set success($core.bool value) => $_setBool(1, value);
  @$pb.TagNumber(2)
  $core.bool hasSuccess() => $_has(1);
  @$pb.TagNumber(2)
  void clearSuccess() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get errorMessage => $_getSZ(2);
  @$pb.TagNumber(3)
  set errorMessage($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasErrorMessage() => $_has(2);
  @$pb.TagNumber(3)
  void clearErrorMessage() => $_clearField(3);
}

/// TrustedDevice represents a paired device in the persistent trust store.
class TrustedDevice extends $pb.GeneratedMessage {
  factory TrustedDevice({
    $core.String? deviceId,
    $core.String? displayName,
    $core.String? platform,
    $core.List<$core.int>? publicKey,
    $fixnum.Int64? pairedAtMs,
    $fixnum.Int64? lastSeenMs,
    $core.bool? revoked,
  }) {
    final result = TrustedDevice._();
    if (deviceId != null) result.deviceId = deviceId;
    if (displayName != null) result.displayName = displayName;
    if (platform != null) result.platform = platform;
    if (publicKey != null) result.publicKey = publicKey;
    if (pairedAtMs != null) result.pairedAtMs = pairedAtMs;
    if (lastSeenMs != null) result.lastSeenMs = lastSeenMs;
    if (revoked != null) result.revoked = revoked;
    return result;
  }

  TrustedDevice._();

  factory TrustedDevice.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      TrustedDevice()..mergeFromBuffer(data, registry);
  factory TrustedDevice.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      TrustedDevice()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'TrustedDevice',
      package: const $pb.PackageName(
          _omitMessageNames ? '' : 'phonebridge.localipc.v1'),
      createEmptyInstance: TrustedDevice.$_createMessage)
    ..aOS(1, _omitFieldNames ? '' : 'deviceId')
    ..aOS(2, _omitFieldNames ? '' : 'displayName')
    ..aOS(3, _omitFieldNames ? '' : 'platform')
    ..a<$core.List<$core.int>>(
        4, _omitFieldNames ? '' : 'publicKey', $pb.PbFieldType.OY)
    ..aInt64(5, _omitFieldNames ? '' : 'pairedAtMs')
    ..aInt64(6, _omitFieldNames ? '' : 'lastSeenMs')
    ..aOB(7, _omitFieldNames ? '' : 'revoked')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  TrustedDevice clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  TrustedDevice copyWith(void Function(TrustedDevice) updates) =>
      super.copyWith((message) => updates(message as TrustedDevice))
          as TrustedDevice;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated('Use TrustedDevice() / TrustedDevice.new instead')
  static TrustedDevice create() => TrustedDevice._();
  static $pb.GeneratedMessage $_createMessage() => TrustedDevice._();
  @$core.override
  TrustedDevice createEmptyInstance() => TrustedDevice._();
  @$core.pragma('dart2js:noInline')
  static TrustedDevice getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<TrustedDevice>(
          TrustedDevice.$_createMessage);
  static TrustedDevice? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get deviceId => $_getSZ(0);
  @$pb.TagNumber(1)
  set deviceId($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasDeviceId() => $_has(0);
  @$pb.TagNumber(1)
  void clearDeviceId() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get displayName => $_getSZ(1);
  @$pb.TagNumber(2)
  set displayName($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasDisplayName() => $_has(1);
  @$pb.TagNumber(2)
  void clearDisplayName() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get platform => $_getSZ(2);
  @$pb.TagNumber(3)
  set platform($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasPlatform() => $_has(2);
  @$pb.TagNumber(3)
  void clearPlatform() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.List<$core.int> get publicKey => $_getN(3);
  @$pb.TagNumber(4)
  set publicKey($core.List<$core.int> value) => $_setBytes(3, value);
  @$pb.TagNumber(4)
  $core.bool hasPublicKey() => $_has(3);
  @$pb.TagNumber(4)
  void clearPublicKey() => $_clearField(4);

  @$pb.TagNumber(5)
  $fixnum.Int64 get pairedAtMs => $_getI64(4);
  @$pb.TagNumber(5)
  set pairedAtMs($fixnum.Int64 value) => $_setInt64(4, value);
  @$pb.TagNumber(5)
  $core.bool hasPairedAtMs() => $_has(4);
  @$pb.TagNumber(5)
  void clearPairedAtMs() => $_clearField(5);

  @$pb.TagNumber(6)
  $fixnum.Int64 get lastSeenMs => $_getI64(5);
  @$pb.TagNumber(6)
  set lastSeenMs($fixnum.Int64 value) => $_setInt64(5, value);
  @$pb.TagNumber(6)
  $core.bool hasLastSeenMs() => $_has(5);
  @$pb.TagNumber(6)
  void clearLastSeenMs() => $_clearField(6);

  @$pb.TagNumber(7)
  $core.bool get revoked => $_getBF(6);
  @$pb.TagNumber(7)
  set revoked($core.bool value) => $_setBool(6, value);
  @$pb.TagNumber(7)
  $core.bool hasRevoked() => $_has(6);
  @$pb.TagNumber(7)
  void clearRevoked() => $_clearField(7);
}

/// ListTrustedDevicesRequest is empty.
class ListTrustedDevicesRequest extends $pb.GeneratedMessage {
  factory ListTrustedDevicesRequest() => ListTrustedDevicesRequest._();

  ListTrustedDevicesRequest._();

  factory ListTrustedDevicesRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      ListTrustedDevicesRequest()..mergeFromBuffer(data, registry);
  factory ListTrustedDevicesRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      ListTrustedDevicesRequest()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ListTrustedDevicesRequest',
      package: const $pb.PackageName(
          _omitMessageNames ? '' : 'phonebridge.localipc.v1'),
      createEmptyInstance: ListTrustedDevicesRequest.$_createMessage)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ListTrustedDevicesRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ListTrustedDevicesRequest copyWith(
          void Function(ListTrustedDevicesRequest) updates) =>
      super.copyWith((message) => updates(message as ListTrustedDevicesRequest))
          as ListTrustedDevicesRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated(
      'Use ListTrustedDevicesRequest() / ListTrustedDevicesRequest.new instead')
  static ListTrustedDevicesRequest create() => ListTrustedDevicesRequest._();
  static $pb.GeneratedMessage $_createMessage() =>
      ListTrustedDevicesRequest._();
  @$core.override
  ListTrustedDevicesRequest createEmptyInstance() =>
      ListTrustedDevicesRequest._();
  @$core.pragma('dart2js:noInline')
  static ListTrustedDevicesRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ListTrustedDevicesRequest>(
          ListTrustedDevicesRequest.$_createMessage);
  static ListTrustedDevicesRequest? _defaultInstance;
}

/// ListTrustedDevicesResponse lists all paired devices.
class ListTrustedDevicesResponse extends $pb.GeneratedMessage {
  factory ListTrustedDevicesResponse({
    $core.Iterable<TrustedDevice>? devices,
  }) {
    final result = ListTrustedDevicesResponse._();
    if (devices != null) result.devices.addAll(devices);
    return result;
  }

  ListTrustedDevicesResponse._();

  factory ListTrustedDevicesResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      ListTrustedDevicesResponse()..mergeFromBuffer(data, registry);
  factory ListTrustedDevicesResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      ListTrustedDevicesResponse()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ListTrustedDevicesResponse',
      package: const $pb.PackageName(
          _omitMessageNames ? '' : 'phonebridge.localipc.v1'),
      createEmptyInstance: ListTrustedDevicesResponse.$_createMessage)
    ..pPM<TrustedDevice>(1, _omitFieldNames ? '' : 'devices',
        subBuilder: TrustedDevice.$_createMessage)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ListTrustedDevicesResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ListTrustedDevicesResponse copyWith(
          void Function(ListTrustedDevicesResponse) updates) =>
      super.copyWith(
              (message) => updates(message as ListTrustedDevicesResponse))
          as ListTrustedDevicesResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated(
      'Use ListTrustedDevicesResponse() / ListTrustedDevicesResponse.new instead')
  static ListTrustedDevicesResponse create() => ListTrustedDevicesResponse._();
  static $pb.GeneratedMessage $_createMessage() =>
      ListTrustedDevicesResponse._();
  @$core.override
  ListTrustedDevicesResponse createEmptyInstance() =>
      ListTrustedDevicesResponse._();
  @$core.pragma('dart2js:noInline')
  static ListTrustedDevicesResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ListTrustedDevicesResponse>(
          ListTrustedDevicesResponse.$_createMessage);
  static ListTrustedDevicesResponse? _defaultInstance;

  @$pb.TagNumber(1)
  $pb.PbList<TrustedDevice> get devices => $_getList(0);
}

/// RevokeDeviceRequest requests revocation of a paired device.
class RevokeDeviceRequest extends $pb.GeneratedMessage {
  factory RevokeDeviceRequest({
    $core.String? deviceId,
  }) {
    final result = RevokeDeviceRequest._();
    if (deviceId != null) result.deviceId = deviceId;
    return result;
  }

  RevokeDeviceRequest._();

  factory RevokeDeviceRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      RevokeDeviceRequest()..mergeFromBuffer(data, registry);
  factory RevokeDeviceRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      RevokeDeviceRequest()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'RevokeDeviceRequest',
      package: const $pb.PackageName(
          _omitMessageNames ? '' : 'phonebridge.localipc.v1'),
      createEmptyInstance: RevokeDeviceRequest.$_createMessage)
    ..aOS(1, _omitFieldNames ? '' : 'deviceId')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  RevokeDeviceRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  RevokeDeviceRequest copyWith(void Function(RevokeDeviceRequest) updates) =>
      super.copyWith((message) => updates(message as RevokeDeviceRequest))
          as RevokeDeviceRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core
      .Deprecated('Use RevokeDeviceRequest() / RevokeDeviceRequest.new instead')
  static RevokeDeviceRequest create() => RevokeDeviceRequest._();
  static $pb.GeneratedMessage $_createMessage() => RevokeDeviceRequest._();
  @$core.override
  RevokeDeviceRequest createEmptyInstance() => RevokeDeviceRequest._();
  @$core.pragma('dart2js:noInline')
  static RevokeDeviceRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<RevokeDeviceRequest>(
          RevokeDeviceRequest.$_createMessage);
  static RevokeDeviceRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get deviceId => $_getSZ(0);
  @$pb.TagNumber(1)
  set deviceId($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasDeviceId() => $_has(0);
  @$pb.TagNumber(1)
  void clearDeviceId() => $_clearField(1);
}

/// RevokeDeviceResponse confirms revocation.
class RevokeDeviceResponse extends $pb.GeneratedMessage {
  factory RevokeDeviceResponse({
    $core.String? deviceId,
    $core.bool? success,
  }) {
    final result = RevokeDeviceResponse._();
    if (deviceId != null) result.deviceId = deviceId;
    if (success != null) result.success = success;
    return result;
  }

  RevokeDeviceResponse._();

  factory RevokeDeviceResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      RevokeDeviceResponse()..mergeFromBuffer(data, registry);
  factory RevokeDeviceResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      RevokeDeviceResponse()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'RevokeDeviceResponse',
      package: const $pb.PackageName(
          _omitMessageNames ? '' : 'phonebridge.localipc.v1'),
      createEmptyInstance: RevokeDeviceResponse.$_createMessage)
    ..aOS(1, _omitFieldNames ? '' : 'deviceId')
    ..aOB(2, _omitFieldNames ? '' : 'success')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  RevokeDeviceResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  RevokeDeviceResponse copyWith(void Function(RevokeDeviceResponse) updates) =>
      super.copyWith((message) => updates(message as RevokeDeviceResponse))
          as RevokeDeviceResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated(
      'Use RevokeDeviceResponse() / RevokeDeviceResponse.new instead')
  static RevokeDeviceResponse create() => RevokeDeviceResponse._();
  static $pb.GeneratedMessage $_createMessage() => RevokeDeviceResponse._();
  @$core.override
  RevokeDeviceResponse createEmptyInstance() => RevokeDeviceResponse._();
  @$core.pragma('dart2js:noInline')
  static RevokeDeviceResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<RevokeDeviceResponse>(
          RevokeDeviceResponse.$_createMessage);
  static RevokeDeviceResponse? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get deviceId => $_getSZ(0);
  @$pb.TagNumber(1)
  set deviceId($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasDeviceId() => $_has(0);
  @$pb.TagNumber(1)
  void clearDeviceId() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.bool get success => $_getBF(1);
  @$pb.TagNumber(2)
  set success($core.bool value) => $_setBool(1, value);
  @$pb.TagNumber(2)
  $core.bool hasSuccess() => $_has(1);
  @$pb.TagNumber(2)
  void clearSuccess() => $_clearField(2);
}

const $core.bool _omitFieldNames =
    $core.bool.fromEnvironment('protobuf.omit_field_names');
const $core.bool _omitMessageNames =
    $core.bool.fromEnvironment('protobuf.omit_message_names');
