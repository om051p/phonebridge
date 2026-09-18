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

import 'package:fixnum/fixnum.dart' as $fixnum;
import 'package:protobuf/protobuf.dart' as $pb;

import 'phonebridge.pbenum.dart';

export 'package:protobuf/protobuf.dart' show GeneratedMessageGenericExtensions;

export 'phonebridge.pbenum.dart';

enum Envelope_Payload {
  ping,
  pong,
  deviceHello,
  pairRequest,
  pairAccept,
  pairReject,
  error,
  clipboardUpdate,
  notificationEvent,
  notificationAction,
  notificationDismissed,
  fileOffer,
  fileAccept,
  fileChunk,
  fileComplete,
  fileCancel,
  screenStart,
  screenStop,
  inputKeyboard,
  inputMouse,
  inputTouch,
  deviceStatus,
  notSet
}

/// ---------------------------------------------------------------------------
/// Envelope — top-level framing for every message on the wire.
/// ---------------------------------------------------------------------------
class Envelope extends $pb.GeneratedMessage {
  factory Envelope({
    $core.int? version,
    $core.String? deviceId,
    $core.String? sessionId,
    $fixnum.Int64? sequence,
    $fixnum.Int64? timestampMs,
    $core.List<$core.int>? nonce,
    CapabilitySet? capabilities,
    Ping? ping,
    Pong? pong,
    DeviceHello? deviceHello,
    PairRequest? pairRequest,
    PairAccept? pairAccept,
    PairReject? pairReject,
    Error? error,
    ClipboardUpdate? clipboardUpdate,
    NotificationEvent? notificationEvent,
    NotificationAction? notificationAction,
    NotificationDismissed? notificationDismissed,
    FileOffer? fileOffer,
    FileAccept? fileAccept,
    FileChunk? fileChunk,
    FileComplete? fileComplete,
    FileCancel? fileCancel,
    ScreenStart? screenStart,
    ScreenStop? screenStop,
    InputKeyboard? inputKeyboard,
    InputMouse? inputMouse,
    InputTouch? inputTouch,
    DeviceStatus? deviceStatus,
  }) {
    final result = Envelope._();
    if (version != null) result.version = version;
    if (deviceId != null) result.deviceId = deviceId;
    if (sessionId != null) result.sessionId = sessionId;
    if (sequence != null) result.sequence = sequence;
    if (timestampMs != null) result.timestampMs = timestampMs;
    if (nonce != null) result.nonce = nonce;
    if (capabilities != null) result.capabilities = capabilities;
    if (ping != null) result.ping = ping;
    if (pong != null) result.pong = pong;
    if (deviceHello != null) result.deviceHello = deviceHello;
    if (pairRequest != null) result.pairRequest = pairRequest;
    if (pairAccept != null) result.pairAccept = pairAccept;
    if (pairReject != null) result.pairReject = pairReject;
    if (error != null) result.error = error;
    if (clipboardUpdate != null) result.clipboardUpdate = clipboardUpdate;
    if (notificationEvent != null) result.notificationEvent = notificationEvent;
    if (notificationAction != null)
      result.notificationAction = notificationAction;
    if (notificationDismissed != null)
      result.notificationDismissed = notificationDismissed;
    if (fileOffer != null) result.fileOffer = fileOffer;
    if (fileAccept != null) result.fileAccept = fileAccept;
    if (fileChunk != null) result.fileChunk = fileChunk;
    if (fileComplete != null) result.fileComplete = fileComplete;
    if (fileCancel != null) result.fileCancel = fileCancel;
    if (screenStart != null) result.screenStart = screenStart;
    if (screenStop != null) result.screenStop = screenStop;
    if (inputKeyboard != null) result.inputKeyboard = inputKeyboard;
    if (inputMouse != null) result.inputMouse = inputMouse;
    if (inputTouch != null) result.inputTouch = inputTouch;
    if (deviceStatus != null) result.deviceStatus = deviceStatus;
    return result;
  }

  Envelope._();

  factory Envelope.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      Envelope()..mergeFromBuffer(data, registry);
  factory Envelope.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      Envelope()..mergeFromJson(json, registry);

  static const $core.Map<$core.int, Envelope_Payload> _Envelope_PayloadByTag = {
    10: Envelope_Payload.ping,
    11: Envelope_Payload.pong,
    12: Envelope_Payload.deviceHello,
    13: Envelope_Payload.pairRequest,
    14: Envelope_Payload.pairAccept,
    15: Envelope_Payload.pairReject,
    16: Envelope_Payload.error,
    20: Envelope_Payload.clipboardUpdate,
    21: Envelope_Payload.notificationEvent,
    22: Envelope_Payload.notificationAction,
    23: Envelope_Payload.notificationDismissed,
    24: Envelope_Payload.fileOffer,
    25: Envelope_Payload.fileAccept,
    26: Envelope_Payload.fileChunk,
    27: Envelope_Payload.fileComplete,
    28: Envelope_Payload.fileCancel,
    29: Envelope_Payload.screenStart,
    30: Envelope_Payload.screenStop,
    31: Envelope_Payload.inputKeyboard,
    32: Envelope_Payload.inputMouse,
    33: Envelope_Payload.inputTouch,
    34: Envelope_Payload.deviceStatus,
    0: Envelope_Payload.notSet
  };
  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'Envelope',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'phonebridge.v1'),
      createEmptyInstance: Envelope.$_createMessage)
    ..oo(0, [
      10,
      11,
      12,
      13,
      14,
      15,
      16,
      20,
      21,
      22,
      23,
      24,
      25,
      26,
      27,
      28,
      29,
      30,
      31,
      32,
      33,
      34
    ])
    ..aI(1, _omitFieldNames ? '' : 'version', fieldType: $pb.PbFieldType.OU3)
    ..aOS(2, _omitFieldNames ? '' : 'deviceId')
    ..aOS(3, _omitFieldNames ? '' : 'sessionId')
    ..a<$fixnum.Int64>(
        4, _omitFieldNames ? '' : 'sequence', $pb.PbFieldType.OU6,
        defaultOrMaker: $fixnum.Int64.ZERO)
    ..a<$fixnum.Int64>(
        5, _omitFieldNames ? '' : 'timestampMs', $pb.PbFieldType.OU6,
        defaultOrMaker: $fixnum.Int64.ZERO)
    ..a<$core.List<$core.int>>(
        6, _omitFieldNames ? '' : 'nonce', $pb.PbFieldType.OY)
    ..aOM<CapabilitySet>(7, _omitFieldNames ? '' : 'capabilities',
        subBuilder: CapabilitySet.$_createMessage)
    ..aOM<Ping>(10, _omitFieldNames ? '' : 'ping',
        subBuilder: Ping.$_createMessage)
    ..aOM<Pong>(11, _omitFieldNames ? '' : 'pong',
        subBuilder: Pong.$_createMessage)
    ..aOM<DeviceHello>(12, _omitFieldNames ? '' : 'deviceHello',
        subBuilder: DeviceHello.$_createMessage)
    ..aOM<PairRequest>(13, _omitFieldNames ? '' : 'pairRequest',
        subBuilder: PairRequest.$_createMessage)
    ..aOM<PairAccept>(14, _omitFieldNames ? '' : 'pairAccept',
        subBuilder: PairAccept.$_createMessage)
    ..aOM<PairReject>(15, _omitFieldNames ? '' : 'pairReject',
        subBuilder: PairReject.$_createMessage)
    ..aOM<Error>(16, _omitFieldNames ? '' : 'error',
        subBuilder: Error.$_createMessage)
    ..aOM<ClipboardUpdate>(20, _omitFieldNames ? '' : 'clipboardUpdate',
        subBuilder: ClipboardUpdate.$_createMessage)
    ..aOM<NotificationEvent>(21, _omitFieldNames ? '' : 'notificationEvent',
        subBuilder: NotificationEvent.$_createMessage)
    ..aOM<NotificationAction>(22, _omitFieldNames ? '' : 'notificationAction',
        subBuilder: NotificationAction.$_createMessage)
    ..aOM<NotificationDismissed>(
        23, _omitFieldNames ? '' : 'notificationDismissed',
        subBuilder: NotificationDismissed.$_createMessage)
    ..aOM<FileOffer>(24, _omitFieldNames ? '' : 'fileOffer',
        subBuilder: FileOffer.$_createMessage)
    ..aOM<FileAccept>(25, _omitFieldNames ? '' : 'fileAccept',
        subBuilder: FileAccept.$_createMessage)
    ..aOM<FileChunk>(26, _omitFieldNames ? '' : 'fileChunk',
        subBuilder: FileChunk.$_createMessage)
    ..aOM<FileComplete>(27, _omitFieldNames ? '' : 'fileComplete',
        subBuilder: FileComplete.$_createMessage)
    ..aOM<FileCancel>(28, _omitFieldNames ? '' : 'fileCancel',
        subBuilder: FileCancel.$_createMessage)
    ..aOM<ScreenStart>(29, _omitFieldNames ? '' : 'screenStart',
        subBuilder: ScreenStart.$_createMessage)
    ..aOM<ScreenStop>(30, _omitFieldNames ? '' : 'screenStop',
        subBuilder: ScreenStop.$_createMessage)
    ..aOM<InputKeyboard>(31, _omitFieldNames ? '' : 'inputKeyboard',
        subBuilder: InputKeyboard.$_createMessage)
    ..aOM<InputMouse>(32, _omitFieldNames ? '' : 'inputMouse',
        subBuilder: InputMouse.$_createMessage)
    ..aOM<InputTouch>(33, _omitFieldNames ? '' : 'inputTouch',
        subBuilder: InputTouch.$_createMessage)
    ..aOM<DeviceStatus>(34, _omitFieldNames ? '' : 'deviceStatus',
        subBuilder: DeviceStatus.$_createMessage)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Envelope clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Envelope copyWith(void Function(Envelope) updates) =>
      super.copyWith((message) => updates(message as Envelope)) as Envelope;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated('Use Envelope() / Envelope.new instead')
  static Envelope create() => Envelope._();
  static $pb.GeneratedMessage $_createMessage() => Envelope._();
  @$core.override
  Envelope createEmptyInstance() => Envelope._();
  @$core.pragma('dart2js:noInline')
  static Envelope getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<Envelope>(Envelope.$_createMessage);
  static Envelope? _defaultInstance;

  @$pb.TagNumber(10)
  @$pb.TagNumber(11)
  @$pb.TagNumber(12)
  @$pb.TagNumber(13)
  @$pb.TagNumber(14)
  @$pb.TagNumber(15)
  @$pb.TagNumber(16)
  @$pb.TagNumber(20)
  @$pb.TagNumber(21)
  @$pb.TagNumber(22)
  @$pb.TagNumber(23)
  @$pb.TagNumber(24)
  @$pb.TagNumber(25)
  @$pb.TagNumber(26)
  @$pb.TagNumber(27)
  @$pb.TagNumber(28)
  @$pb.TagNumber(29)
  @$pb.TagNumber(30)
  @$pb.TagNumber(31)
  @$pb.TagNumber(32)
  @$pb.TagNumber(33)
  @$pb.TagNumber(34)
  Envelope_Payload whichPayload() => _Envelope_PayloadByTag[$_whichOneof(0)]!;
  @$pb.TagNumber(10)
  @$pb.TagNumber(11)
  @$pb.TagNumber(12)
  @$pb.TagNumber(13)
  @$pb.TagNumber(14)
  @$pb.TagNumber(15)
  @$pb.TagNumber(16)
  @$pb.TagNumber(20)
  @$pb.TagNumber(21)
  @$pb.TagNumber(22)
  @$pb.TagNumber(23)
  @$pb.TagNumber(24)
  @$pb.TagNumber(25)
  @$pb.TagNumber(26)
  @$pb.TagNumber(27)
  @$pb.TagNumber(28)
  @$pb.TagNumber(29)
  @$pb.TagNumber(30)
  @$pb.TagNumber(31)
  @$pb.TagNumber(32)
  @$pb.TagNumber(33)
  @$pb.TagNumber(34)
  void clearPayload() => $_clearField($_whichOneof(0));

  /// Protocol version (monotonic uint32). Negotiated via VersionNegotiation.
  @$pb.TagNumber(1)
  $core.int get version => $_getIZ(0);
  @$pb.TagNumber(1)
  set version($core.int value) => $_setUnsignedInt32(0, value);
  @$pb.TagNumber(1)
  $core.bool hasVersion() => $_has(0);
  @$pb.TagNumber(1)
  void clearVersion() => $_clearField(1);

  /// Stable device identity — fingerprint of Ed25519 public key (hex or base64url per spec).
  @$pb.TagNumber(2)
  $core.String get deviceId => $_getSZ(1);
  @$pb.TagNumber(2)
  set deviceId($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasDeviceId() => $_has(1);
  @$pb.TagNumber(2)
  void clearDeviceId() => $_clearField(2);

  /// Per-connection session identifier (random, e.g. UUID v4).
  @$pb.TagNumber(3)
  $core.String get sessionId => $_getSZ(2);
  @$pb.TagNumber(3)
  set sessionId($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasSessionId() => $_has(2);
  @$pb.TagNumber(3)
  void clearSessionId() => $_clearField(3);

  /// Per-session monotonic sequence. Authoritative ordering; receiver rejects
  /// duplicates/old values per replay window.
  @$pb.TagNumber(4)
  $fixnum.Int64 get sequence => $_getI64(3);
  @$pb.TagNumber(4)
  set sequence($fixnum.Int64 value) => $_setInt64(3, value);
  @$pb.TagNumber(4)
  $core.bool hasSequence() => $_has(3);
  @$pb.TagNumber(4)
  void clearSequence() => $_clearField(4);

  /// Sender wall-clock in milliseconds since Unix epoch (informational, not ordering).
  @$pb.TagNumber(5)
  $fixnum.Int64 get timestampMs => $_getI64(4);
  @$pb.TagNumber(5)
  set timestampMs($fixnum.Int64 value) => $_setInt64(4, value);
  @$pb.TagNumber(5)
  $core.bool hasTimestampMs() => $_has(4);
  @$pb.TagNumber(5)
  void clearTimestampMs() => $_clearField(5);

  /// Per-message nonce for replay-window uniqueness (e.g. 16 random bytes).
  @$pb.TagNumber(6)
  $core.List<$core.int> get nonce => $_getN(5);
  @$pb.TagNumber(6)
  set nonce($core.List<$core.int> value) => $_setBytes(5, value);
  @$pb.TagNumber(6)
  $core.bool hasNonce() => $_has(5);
  @$pb.TagNumber(6)
  void clearNonce() => $_clearField(6);

  /// Sender-advertised capabilities for this session.
  @$pb.TagNumber(7)
  CapabilitySet get capabilities => $_getN(6);
  @$pb.TagNumber(7)
  set capabilities(CapabilitySet value) => $_setField(7, value);
  @$pb.TagNumber(7)
  $core.bool hasCapabilities() => $_has(6);
  @$pb.TagNumber(7)
  void clearCapabilities() => $_clearField(7);
  @$pb.TagNumber(7)
  CapabilitySet ensureCapabilities() => $_ensure(6);

  @$pb.TagNumber(10)
  Ping get ping => $_getN(7);
  @$pb.TagNumber(10)
  set ping(Ping value) => $_setField(10, value);
  @$pb.TagNumber(10)
  $core.bool hasPing() => $_has(7);
  @$pb.TagNumber(10)
  void clearPing() => $_clearField(10);
  @$pb.TagNumber(10)
  Ping ensurePing() => $_ensure(7);

  @$pb.TagNumber(11)
  Pong get pong => $_getN(8);
  @$pb.TagNumber(11)
  set pong(Pong value) => $_setField(11, value);
  @$pb.TagNumber(11)
  $core.bool hasPong() => $_has(8);
  @$pb.TagNumber(11)
  void clearPong() => $_clearField(11);
  @$pb.TagNumber(11)
  Pong ensurePong() => $_ensure(8);

  @$pb.TagNumber(12)
  DeviceHello get deviceHello => $_getN(9);
  @$pb.TagNumber(12)
  set deviceHello(DeviceHello value) => $_setField(12, value);
  @$pb.TagNumber(12)
  $core.bool hasDeviceHello() => $_has(9);
  @$pb.TagNumber(12)
  void clearDeviceHello() => $_clearField(12);
  @$pb.TagNumber(12)
  DeviceHello ensureDeviceHello() => $_ensure(9);

  @$pb.TagNumber(13)
  PairRequest get pairRequest => $_getN(10);
  @$pb.TagNumber(13)
  set pairRequest(PairRequest value) => $_setField(13, value);
  @$pb.TagNumber(13)
  $core.bool hasPairRequest() => $_has(10);
  @$pb.TagNumber(13)
  void clearPairRequest() => $_clearField(13);
  @$pb.TagNumber(13)
  PairRequest ensurePairRequest() => $_ensure(10);

  @$pb.TagNumber(14)
  PairAccept get pairAccept => $_getN(11);
  @$pb.TagNumber(14)
  set pairAccept(PairAccept value) => $_setField(14, value);
  @$pb.TagNumber(14)
  $core.bool hasPairAccept() => $_has(11);
  @$pb.TagNumber(14)
  void clearPairAccept() => $_clearField(14);
  @$pb.TagNumber(14)
  PairAccept ensurePairAccept() => $_ensure(11);

  @$pb.TagNumber(15)
  PairReject get pairReject => $_getN(12);
  @$pb.TagNumber(15)
  set pairReject(PairReject value) => $_setField(15, value);
  @$pb.TagNumber(15)
  $core.bool hasPairReject() => $_has(12);
  @$pb.TagNumber(15)
  void clearPairReject() => $_clearField(15);
  @$pb.TagNumber(15)
  PairReject ensurePairReject() => $_ensure(12);

  @$pb.TagNumber(16)
  Error get error => $_getN(13);
  @$pb.TagNumber(16)
  set error(Error value) => $_setField(16, value);
  @$pb.TagNumber(16)
  $core.bool hasError() => $_has(13);
  @$pb.TagNumber(16)
  void clearError() => $_clearField(16);
  @$pb.TagNumber(16)
  Error ensureError() => $_ensure(13);

  /// Feature payloads — PLANNED stubs (filled in their respective phases).
  @$pb.TagNumber(20)
  ClipboardUpdate get clipboardUpdate => $_getN(14);
  @$pb.TagNumber(20)
  set clipboardUpdate(ClipboardUpdate value) => $_setField(20, value);
  @$pb.TagNumber(20)
  $core.bool hasClipboardUpdate() => $_has(14);
  @$pb.TagNumber(20)
  void clearClipboardUpdate() => $_clearField(20);
  @$pb.TagNumber(20)
  ClipboardUpdate ensureClipboardUpdate() => $_ensure(14);

  @$pb.TagNumber(21)
  NotificationEvent get notificationEvent => $_getN(15);
  @$pb.TagNumber(21)
  set notificationEvent(NotificationEvent value) => $_setField(21, value);
  @$pb.TagNumber(21)
  $core.bool hasNotificationEvent() => $_has(15);
  @$pb.TagNumber(21)
  void clearNotificationEvent() => $_clearField(21);
  @$pb.TagNumber(21)
  NotificationEvent ensureNotificationEvent() => $_ensure(15);

  @$pb.TagNumber(22)
  NotificationAction get notificationAction => $_getN(16);
  @$pb.TagNumber(22)
  set notificationAction(NotificationAction value) => $_setField(22, value);
  @$pb.TagNumber(22)
  $core.bool hasNotificationAction() => $_has(16);
  @$pb.TagNumber(22)
  void clearNotificationAction() => $_clearField(22);
  @$pb.TagNumber(22)
  NotificationAction ensureNotificationAction() => $_ensure(16);

  @$pb.TagNumber(23)
  NotificationDismissed get notificationDismissed => $_getN(17);
  @$pb.TagNumber(23)
  set notificationDismissed(NotificationDismissed value) =>
      $_setField(23, value);
  @$pb.TagNumber(23)
  $core.bool hasNotificationDismissed() => $_has(17);
  @$pb.TagNumber(23)
  void clearNotificationDismissed() => $_clearField(23);
  @$pb.TagNumber(23)
  NotificationDismissed ensureNotificationDismissed() => $_ensure(17);

  @$pb.TagNumber(24)
  FileOffer get fileOffer => $_getN(18);
  @$pb.TagNumber(24)
  set fileOffer(FileOffer value) => $_setField(24, value);
  @$pb.TagNumber(24)
  $core.bool hasFileOffer() => $_has(18);
  @$pb.TagNumber(24)
  void clearFileOffer() => $_clearField(24);
  @$pb.TagNumber(24)
  FileOffer ensureFileOffer() => $_ensure(18);

  @$pb.TagNumber(25)
  FileAccept get fileAccept => $_getN(19);
  @$pb.TagNumber(25)
  set fileAccept(FileAccept value) => $_setField(25, value);
  @$pb.TagNumber(25)
  $core.bool hasFileAccept() => $_has(19);
  @$pb.TagNumber(25)
  void clearFileAccept() => $_clearField(25);
  @$pb.TagNumber(25)
  FileAccept ensureFileAccept() => $_ensure(19);

  @$pb.TagNumber(26)
  FileChunk get fileChunk => $_getN(20);
  @$pb.TagNumber(26)
  set fileChunk(FileChunk value) => $_setField(26, value);
  @$pb.TagNumber(26)
  $core.bool hasFileChunk() => $_has(20);
  @$pb.TagNumber(26)
  void clearFileChunk() => $_clearField(26);
  @$pb.TagNumber(26)
  FileChunk ensureFileChunk() => $_ensure(20);

  @$pb.TagNumber(27)
  FileComplete get fileComplete => $_getN(21);
  @$pb.TagNumber(27)
  set fileComplete(FileComplete value) => $_setField(27, value);
  @$pb.TagNumber(27)
  $core.bool hasFileComplete() => $_has(21);
  @$pb.TagNumber(27)
  void clearFileComplete() => $_clearField(27);
  @$pb.TagNumber(27)
  FileComplete ensureFileComplete() => $_ensure(21);

  @$pb.TagNumber(28)
  FileCancel get fileCancel => $_getN(22);
  @$pb.TagNumber(28)
  set fileCancel(FileCancel value) => $_setField(28, value);
  @$pb.TagNumber(28)
  $core.bool hasFileCancel() => $_has(22);
  @$pb.TagNumber(28)
  void clearFileCancel() => $_clearField(28);
  @$pb.TagNumber(28)
  FileCancel ensureFileCancel() => $_ensure(22);

  /// Screen session negotiation — ratified by DEC-022 (Phase 2).
  @$pb.TagNumber(29)
  ScreenStart get screenStart => $_getN(23);
  @$pb.TagNumber(29)
  set screenStart(ScreenStart value) => $_setField(29, value);
  @$pb.TagNumber(29)
  $core.bool hasScreenStart() => $_has(23);
  @$pb.TagNumber(29)
  void clearScreenStart() => $_clearField(29);
  @$pb.TagNumber(29)
  ScreenStart ensureScreenStart() => $_ensure(23);

  @$pb.TagNumber(30)
  ScreenStop get screenStop => $_getN(24);
  @$pb.TagNumber(30)
  set screenStop(ScreenStop value) => $_setField(30, value);
  @$pb.TagNumber(30)
  $core.bool hasScreenStop() => $_has(24);
  @$pb.TagNumber(30)
  void clearScreenStop() => $_clearField(30);
  @$pb.TagNumber(30)
  ScreenStop ensureScreenStop() => $_ensure(24);

  @$pb.TagNumber(31)
  InputKeyboard get inputKeyboard => $_getN(25);
  @$pb.TagNumber(31)
  set inputKeyboard(InputKeyboard value) => $_setField(31, value);
  @$pb.TagNumber(31)
  $core.bool hasInputKeyboard() => $_has(25);
  @$pb.TagNumber(31)
  void clearInputKeyboard() => $_clearField(31);
  @$pb.TagNumber(31)
  InputKeyboard ensureInputKeyboard() => $_ensure(25);

  @$pb.TagNumber(32)
  InputMouse get inputMouse => $_getN(26);
  @$pb.TagNumber(32)
  set inputMouse(InputMouse value) => $_setField(32, value);
  @$pb.TagNumber(32)
  $core.bool hasInputMouse() => $_has(26);
  @$pb.TagNumber(32)
  void clearInputMouse() => $_clearField(32);
  @$pb.TagNumber(32)
  InputMouse ensureInputMouse() => $_ensure(26);

  @$pb.TagNumber(33)
  InputTouch get inputTouch => $_getN(27);
  @$pb.TagNumber(33)
  set inputTouch(InputTouch value) => $_setField(33, value);
  @$pb.TagNumber(33)
  $core.bool hasInputTouch() => $_has(27);
  @$pb.TagNumber(33)
  void clearInputTouch() => $_clearField(33);
  @$pb.TagNumber(33)
  InputTouch ensureInputTouch() => $_ensure(27);

  @$pb.TagNumber(34)
  DeviceStatus get deviceStatus => $_getN(28);
  @$pb.TagNumber(34)
  set deviceStatus(DeviceStatus value) => $_setField(34, value);
  @$pb.TagNumber(34)
  $core.bool hasDeviceStatus() => $_has(28);
  @$pb.TagNumber(34)
  void clearDeviceStatus() => $_clearField(34);
  @$pb.TagNumber(34)
  DeviceStatus ensureDeviceStatus() => $_ensure(28);
}

class CapabilitySet extends $pb.GeneratedMessage {
  factory CapabilitySet({
    $core.Iterable<Capability>? capabilities,
  }) {
    final result = CapabilitySet._();
    if (capabilities != null) result.capabilities.addAll(capabilities);
    return result;
  }

  CapabilitySet._();

  factory CapabilitySet.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      CapabilitySet()..mergeFromBuffer(data, registry);
  factory CapabilitySet.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      CapabilitySet()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'CapabilitySet',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'phonebridge.v1'),
      createEmptyInstance: CapabilitySet.$_createMessage)
    ..pc<Capability>(
        1, _omitFieldNames ? '' : 'capabilities', $pb.PbFieldType.KE,
        valueOf: Capability.valueOf,
        enumValues: Capability.values,
        defaultEnumValue: Capability.CAPABILITY_UNSPECIFIED)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  CapabilitySet clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  CapabilitySet copyWith(void Function(CapabilitySet) updates) =>
      super.copyWith((message) => updates(message as CapabilitySet))
          as CapabilitySet;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated('Use CapabilitySet() / CapabilitySet.new instead')
  static CapabilitySet create() => CapabilitySet._();
  static $pb.GeneratedMessage $_createMessage() => CapabilitySet._();
  @$core.override
  CapabilitySet createEmptyInstance() => CapabilitySet._();
  @$core.pragma('dart2js:noInline')
  static CapabilitySet getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<CapabilitySet>(
          CapabilitySet.$_createMessage);
  static CapabilitySet? _defaultInstance;

  @$pb.TagNumber(1)
  $pb.PbList<Capability> get capabilities => $_getList(0);
}

class VersionNegotiation extends $pb.GeneratedMessage {
  factory VersionNegotiation({
    $core.int? minVersion,
    $core.int? maxVersion,
  }) {
    final result = VersionNegotiation._();
    if (minVersion != null) result.minVersion = minVersion;
    if (maxVersion != null) result.maxVersion = maxVersion;
    return result;
  }

  VersionNegotiation._();

  factory VersionNegotiation.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      VersionNegotiation()..mergeFromBuffer(data, registry);
  factory VersionNegotiation.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      VersionNegotiation()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'VersionNegotiation',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'phonebridge.v1'),
      createEmptyInstance: VersionNegotiation.$_createMessage)
    ..aI(1, _omitFieldNames ? '' : 'minVersion', fieldType: $pb.PbFieldType.OU3)
    ..aI(2, _omitFieldNames ? '' : 'maxVersion', fieldType: $pb.PbFieldType.OU3)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  VersionNegotiation clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  VersionNegotiation copyWith(void Function(VersionNegotiation) updates) =>
      super.copyWith((message) => updates(message as VersionNegotiation))
          as VersionNegotiation;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated('Use VersionNegotiation() / VersionNegotiation.new instead')
  static VersionNegotiation create() => VersionNegotiation._();
  static $pb.GeneratedMessage $_createMessage() => VersionNegotiation._();
  @$core.override
  VersionNegotiation createEmptyInstance() => VersionNegotiation._();
  @$core.pragma('dart2js:noInline')
  static VersionNegotiation getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<VersionNegotiation>(
          VersionNegotiation.$_createMessage);
  static VersionNegotiation? _defaultInstance;

  @$pb.TagNumber(1)
  $core.int get minVersion => $_getIZ(0);
  @$pb.TagNumber(1)
  set minVersion($core.int value) => $_setUnsignedInt32(0, value);
  @$pb.TagNumber(1)
  $core.bool hasMinVersion() => $_has(0);
  @$pb.TagNumber(1)
  void clearMinVersion() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.int get maxVersion => $_getIZ(1);
  @$pb.TagNumber(2)
  set maxVersion($core.int value) => $_setUnsignedInt32(1, value);
  @$pb.TagNumber(2)
  $core.bool hasMaxVersion() => $_has(1);
  @$pb.TagNumber(2)
  void clearMaxVersion() => $_clearField(2);
}

/// MediaParams is the single definition of a screen-stream parameter tuple.
/// It is used in three places so that one shape flows end to end:
///   1. ScreenStart (device protocol) — canonical request/answer form;
///   2. the LAN signaling exchange (DEC-022) — the same fields as JSON;
///   3. the local IPC session snapshot, so the UI renders exactly what was
///      requested and what the capture device actually applied.
/// An unset field means "no preference / not reported" — proto3 has no explicit
/// null, so callers must treat zero values as "unspecified", not as a request
/// for a 0-pixel stream.
class MediaParams extends $pb.GeneratedMessage {
  factory MediaParams({
    $core.int? width,
    $core.int? height,
    $core.int? fps,
    $core.int? bitrateKbps,
    $core.String? codec,
  }) {
    final result = MediaParams._();
    if (width != null) result.width = width;
    if (height != null) result.height = height;
    if (fps != null) result.fps = fps;
    if (bitrateKbps != null) result.bitrateKbps = bitrateKbps;
    if (codec != null) result.codec = codec;
    return result;
  }

  MediaParams._();

  factory MediaParams.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      MediaParams()..mergeFromBuffer(data, registry);
  factory MediaParams.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      MediaParams()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'MediaParams',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'phonebridge.v1'),
      createEmptyInstance: MediaParams.$_createMessage)
    ..aI(1, _omitFieldNames ? '' : 'width', fieldType: $pb.PbFieldType.OU3)
    ..aI(2, _omitFieldNames ? '' : 'height', fieldType: $pb.PbFieldType.OU3)
    ..aI(3, _omitFieldNames ? '' : 'fps', fieldType: $pb.PbFieldType.OU3)
    ..aI(4, _omitFieldNames ? '' : 'bitrateKbps',
        fieldType: $pb.PbFieldType.OU3)
    ..aOS(5, _omitFieldNames ? '' : 'codec')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  MediaParams clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  MediaParams copyWith(void Function(MediaParams) updates) =>
      super.copyWith((message) => updates(message as MediaParams))
          as MediaParams;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated('Use MediaParams() / MediaParams.new instead')
  static MediaParams create() => MediaParams._();
  static $pb.GeneratedMessage $_createMessage() => MediaParams._();
  @$core.override
  MediaParams createEmptyInstance() => MediaParams._();
  @$core.pragma('dart2js:noInline')
  static MediaParams getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<MediaParams>(
          MediaParams.$_createMessage);
  static MediaParams? _defaultInstance;

  @$pb.TagNumber(1)
  $core.int get width => $_getIZ(0);
  @$pb.TagNumber(1)
  set width($core.int value) => $_setUnsignedInt32(0, value);
  @$pb.TagNumber(1)
  $core.bool hasWidth() => $_has(0);
  @$pb.TagNumber(1)
  void clearWidth() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.int get height => $_getIZ(1);
  @$pb.TagNumber(2)
  set height($core.int value) => $_setUnsignedInt32(1, value);
  @$pb.TagNumber(2)
  $core.bool hasHeight() => $_has(1);
  @$pb.TagNumber(2)
  void clearHeight() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.int get fps => $_getIZ(2);
  @$pb.TagNumber(3)
  set fps($core.int value) => $_setUnsignedInt32(2, value);
  @$pb.TagNumber(3)
  $core.bool hasFps() => $_has(2);
  @$pb.TagNumber(3)
  void clearFps() => $_clearField(3);

  /// Advisory on Android: DEC-020 records that the platform bitrate control is
  /// loose (a 6000 -> 2000 kbps request settled near 1.8x the target). It is
  /// carried so both sides can state intent, not as an enforceable limit.
  @$pb.TagNumber(4)
  $core.int get bitrateKbps => $_getIZ(3);
  @$pb.TagNumber(4)
  set bitrateKbps($core.int value) => $_setUnsignedInt32(3, value);
  @$pb.TagNumber(4)
  $core.bool hasBitrateKbps() => $_has(3);
  @$pb.TagNumber(4)
  void clearBitrateKbps() => $_clearField(4);

  /// Codec identifier, e.g. "h264". Empty means "device default".
  @$pb.TagNumber(5)
  $core.String get codec => $_getSZ(4);
  @$pb.TagNumber(5)
  set codec($core.String value) => $_setString(4, value);
  @$pb.TagNumber(5)
  $core.bool hasCodec() => $_has(4);
  @$pb.TagNumber(5)
  void clearCodec() => $_clearField(5);
}

/// MediaCapabilities advertises what a device can actually capture for screen
/// streaming, so the initiator can request something achievable instead of
/// discovering limits through failure. The capture device is authoritative for
/// the values it applies (DEC-020).
class MediaCapabilities extends $pb.GeneratedMessage {
  factory MediaCapabilities({
    $core.Iterable<$core.String>? codecs,
    $core.int? maxWidth,
    $core.int? maxHeight,
    $core.int? maxFps,
    $core.bool? supportsScreen,
  }) {
    final result = MediaCapabilities._();
    if (codecs != null) result.codecs.addAll(codecs);
    if (maxWidth != null) result.maxWidth = maxWidth;
    if (maxHeight != null) result.maxHeight = maxHeight;
    if (maxFps != null) result.maxFps = maxFps;
    if (supportsScreen != null) result.supportsScreen = supportsScreen;
    return result;
  }

  MediaCapabilities._();

  factory MediaCapabilities.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      MediaCapabilities()..mergeFromBuffer(data, registry);
  factory MediaCapabilities.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      MediaCapabilities()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'MediaCapabilities',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'phonebridge.v1'),
      createEmptyInstance: MediaCapabilities.$_createMessage)
    ..pPS(1, _omitFieldNames ? '' : 'codecs')
    ..aI(2, _omitFieldNames ? '' : 'maxWidth', fieldType: $pb.PbFieldType.OU3)
    ..aI(3, _omitFieldNames ? '' : 'maxHeight', fieldType: $pb.PbFieldType.OU3)
    ..aI(4, _omitFieldNames ? '' : 'maxFps', fieldType: $pb.PbFieldType.OU3)
    ..aOB(5, _omitFieldNames ? '' : 'supportsScreen')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  MediaCapabilities clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  MediaCapabilities copyWith(void Function(MediaCapabilities) updates) =>
      super.copyWith((message) => updates(message as MediaCapabilities))
          as MediaCapabilities;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated('Use MediaCapabilities() / MediaCapabilities.new instead')
  static MediaCapabilities create() => MediaCapabilities._();
  static $pb.GeneratedMessage $_createMessage() => MediaCapabilities._();
  @$core.override
  MediaCapabilities createEmptyInstance() => MediaCapabilities._();
  @$core.pragma('dart2js:noInline')
  static MediaCapabilities getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<MediaCapabilities>(
          MediaCapabilities.$_createMessage);
  static MediaCapabilities? _defaultInstance;

  /// Codec identifiers in preference order (e.g. "h264"). Empty means the
  /// device cannot capture a screen stream at all.
  @$pb.TagNumber(1)
  $pb.PbList<$core.String> get codecs => $_getList(0);

  @$pb.TagNumber(2)
  $core.int get maxWidth => $_getIZ(1);
  @$pb.TagNumber(2)
  set maxWidth($core.int value) => $_setUnsignedInt32(1, value);
  @$pb.TagNumber(2)
  $core.bool hasMaxWidth() => $_has(1);
  @$pb.TagNumber(2)
  void clearMaxWidth() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.int get maxHeight => $_getIZ(2);
  @$pb.TagNumber(3)
  set maxHeight($core.int value) => $_setUnsignedInt32(2, value);
  @$pb.TagNumber(3)
  $core.bool hasMaxHeight() => $_has(2);
  @$pb.TagNumber(3)
  void clearMaxHeight() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.int get maxFps => $_getIZ(3);
  @$pb.TagNumber(4)
  set maxFps($core.int value) => $_setUnsignedInt32(3, value);
  @$pb.TagNumber(4)
  $core.bool hasMaxFps() => $_has(3);
  @$pb.TagNumber(4)
  void clearMaxFps() => $_clearField(4);

  /// True when the platform can service a screen capture request (Android
  /// MediaProjection is consent-gated per session, so this advertises
  /// capability, never an existing grant).
  @$pb.TagNumber(5)
  $core.bool get supportsScreen => $_getBF(4);
  @$pb.TagNumber(5)
  set supportsScreen($core.bool value) => $_setBool(4, value);
  @$pb.TagNumber(5)
  $core.bool hasSupportsScreen() => $_has(4);
  @$pb.TagNumber(5)
  void clearSupportsScreen() => $_clearField(5);
}

/// ---------------------------------------------------------------------------
/// Control / handshake.
/// DeviceHello's version/capability/media advertising is CONFIRMED (Phase 2,
/// DEC-022) and exchanged before any media. The PairRequest/PairAccept payloads
/// remain EXPERIMENTAL: the validated pairing flow runs through the
/// authenticated LAN exchange (DEC-022) and does not yet carry these payloads.
/// ---------------------------------------------------------------------------
class DeviceHello extends $pb.GeneratedMessage {
  factory DeviceHello({
    $core.String? displayName,
    $core.String? platform,
    $core.List<$core.int>? identityPubkey,
    VersionNegotiation? versionNegotiation,
    CapabilitySet? capabilities,
    MediaCapabilities? media,
  }) {
    final result = DeviceHello._();
    if (displayName != null) result.displayName = displayName;
    if (platform != null) result.platform = platform;
    if (identityPubkey != null) result.identityPubkey = identityPubkey;
    if (versionNegotiation != null)
      result.versionNegotiation = versionNegotiation;
    if (capabilities != null) result.capabilities = capabilities;
    if (media != null) result.media = media;
    return result;
  }

  DeviceHello._();

  factory DeviceHello.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      DeviceHello()..mergeFromBuffer(data, registry);
  factory DeviceHello.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      DeviceHello()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'DeviceHello',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'phonebridge.v1'),
      createEmptyInstance: DeviceHello.$_createMessage)
    ..aOS(1, _omitFieldNames ? '' : 'displayName')
    ..aOS(2, _omitFieldNames ? '' : 'platform')
    ..a<$core.List<$core.int>>(
        3, _omitFieldNames ? '' : 'identityPubkey', $pb.PbFieldType.OY)
    ..aOM<VersionNegotiation>(4, _omitFieldNames ? '' : 'versionNegotiation',
        subBuilder: VersionNegotiation.$_createMessage)
    ..aOM<CapabilitySet>(5, _omitFieldNames ? '' : 'capabilities',
        subBuilder: CapabilitySet.$_createMessage)
    ..aOM<MediaCapabilities>(6, _omitFieldNames ? '' : 'media',
        subBuilder: MediaCapabilities.$_createMessage)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  DeviceHello clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  DeviceHello copyWith(void Function(DeviceHello) updates) =>
      super.copyWith((message) => updates(message as DeviceHello))
          as DeviceHello;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated('Use DeviceHello() / DeviceHello.new instead')
  static DeviceHello create() => DeviceHello._();
  static $pb.GeneratedMessage $_createMessage() => DeviceHello._();
  @$core.override
  DeviceHello createEmptyInstance() => DeviceHello._();
  @$core.pragma('dart2js:noInline')
  static DeviceHello getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<DeviceHello>(
          DeviceHello.$_createMessage);
  static DeviceHello? _defaultInstance;

  /// Human-readable device name.
  @$pb.TagNumber(1)
  $core.String get displayName => $_getSZ(0);
  @$pb.TagNumber(1)
  set displayName($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasDisplayName() => $_has(0);
  @$pb.TagNumber(1)
  void clearDisplayName() => $_clearField(1);

  /// Platform identifier (e.g. "android", "linux").
  @$pb.TagNumber(2)
  $core.String get platform => $_getSZ(1);
  @$pb.TagNumber(2)
  set platform($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasPlatform() => $_has(1);
  @$pb.TagNumber(2)
  void clearPlatform() => $_clearField(2);

  /// Ed25519 public key (raw 32 bytes) or fingerprint — exact encoding TBD (EXPERIMENTAL).
  @$pb.TagNumber(3)
  $core.List<$core.int> get identityPubkey => $_getN(2);
  @$pb.TagNumber(3)
  set identityPubkey($core.List<$core.int> value) => $_setBytes(2, value);
  @$pb.TagNumber(3)
  $core.bool hasIdentityPubkey() => $_has(2);
  @$pb.TagNumber(3)
  void clearIdentityPubkey() => $_clearField(3);

  @$pb.TagNumber(4)
  VersionNegotiation get versionNegotiation => $_getN(3);
  @$pb.TagNumber(4)
  set versionNegotiation(VersionNegotiation value) => $_setField(4, value);
  @$pb.TagNumber(4)
  $core.bool hasVersionNegotiation() => $_has(3);
  @$pb.TagNumber(4)
  void clearVersionNegotiation() => $_clearField(4);
  @$pb.TagNumber(4)
  VersionNegotiation ensureVersionNegotiation() => $_ensure(3);

  @$pb.TagNumber(5)
  CapabilitySet get capabilities => $_getN(4);
  @$pb.TagNumber(5)
  set capabilities(CapabilitySet value) => $_setField(5, value);
  @$pb.TagNumber(5)
  $core.bool hasCapabilities() => $_has(4);
  @$pb.TagNumber(5)
  void clearCapabilities() => $_clearField(5);
  @$pb.TagNumber(5)
  CapabilitySet ensureCapabilities() => $_ensure(4);

  /// Screen-streaming limits, present when CAPABILITY_SCREEN is advertised.
  @$pb.TagNumber(6)
  MediaCapabilities get media => $_getN(5);
  @$pb.TagNumber(6)
  set media(MediaCapabilities value) => $_setField(6, value);
  @$pb.TagNumber(6)
  $core.bool hasMedia() => $_has(5);
  @$pb.TagNumber(6)
  void clearMedia() => $_clearField(6);
  @$pb.TagNumber(6)
  MediaCapabilities ensureMedia() => $_ensure(5);
}

class PairRequest extends $pb.GeneratedMessage {
  factory PairRequest({
    DeviceHello? hello,
    $core.String? pairingToken,
  }) {
    final result = PairRequest._();
    if (hello != null) result.hello = hello;
    if (pairingToken != null) result.pairingToken = pairingToken;
    return result;
  }

  PairRequest._();

  factory PairRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      PairRequest()..mergeFromBuffer(data, registry);
  factory PairRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      PairRequest()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'PairRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'phonebridge.v1'),
      createEmptyInstance: PairRequest.$_createMessage)
    ..aOM<DeviceHello>(1, _omitFieldNames ? '' : 'hello',
        subBuilder: DeviceHello.$_createMessage)
    ..aOS(2, _omitFieldNames ? '' : 'pairingToken')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PairRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PairRequest copyWith(void Function(PairRequest) updates) =>
      super.copyWith((message) => updates(message as PairRequest))
          as PairRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated('Use PairRequest() / PairRequest.new instead')
  static PairRequest create() => PairRequest._();
  static $pb.GeneratedMessage $_createMessage() => PairRequest._();
  @$core.override
  PairRequest createEmptyInstance() => PairRequest._();
  @$core.pragma('dart2js:noInline')
  static PairRequest getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<PairRequest>(
          PairRequest.$_createMessage);
  static PairRequest? _defaultInstance;

  @$pb.TagNumber(1)
  DeviceHello get hello => $_getN(0);
  @$pb.TagNumber(1)
  set hello(DeviceHello value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasHello() => $_has(0);
  @$pb.TagNumber(1)
  void clearHello() => $_clearField(1);
  @$pb.TagNumber(1)
  DeviceHello ensureHello() => $_ensure(0);

  /// QR / rendezvous correlation id.
  @$pb.TagNumber(2)
  $core.String get pairingToken => $_getSZ(1);
  @$pb.TagNumber(2)
  set pairingToken($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasPairingToken() => $_has(1);
  @$pb.TagNumber(2)
  void clearPairingToken() => $_clearField(2);
}

class PairAccept extends $pb.GeneratedMessage {
  factory PairAccept({
    DeviceHello? hello,
    $core.String? sas,
  }) {
    final result = PairAccept._();
    if (hello != null) result.hello = hello;
    if (sas != null) result.sas = sas;
    return result;
  }

  PairAccept._();

  factory PairAccept.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      PairAccept()..mergeFromBuffer(data, registry);
  factory PairAccept.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      PairAccept()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'PairAccept',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'phonebridge.v1'),
      createEmptyInstance: PairAccept.$_createMessage)
    ..aOM<DeviceHello>(1, _omitFieldNames ? '' : 'hello',
        subBuilder: DeviceHello.$_createMessage)
    ..aOS(2, _omitFieldNames ? '' : 'sas')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PairAccept clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PairAccept copyWith(void Function(PairAccept) updates) =>
      super.copyWith((message) => updates(message as PairAccept)) as PairAccept;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated('Use PairAccept() / PairAccept.new instead')
  static PairAccept create() => PairAccept._();
  static $pb.GeneratedMessage $_createMessage() => PairAccept._();
  @$core.override
  PairAccept createEmptyInstance() => PairAccept._();
  @$core.pragma('dart2js:noInline')
  static PairAccept getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<PairAccept>(PairAccept.$_createMessage);
  static PairAccept? _defaultInstance;

  @$pb.TagNumber(1)
  DeviceHello get hello => $_getN(0);
  @$pb.TagNumber(1)
  set hello(DeviceHello value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasHello() => $_has(0);
  @$pb.TagNumber(1)
  void clearHello() => $_clearField(1);
  @$pb.TagNumber(1)
  DeviceHello ensureHello() => $_ensure(0);

  /// Short Authentication String (human-verified).
  @$pb.TagNumber(2)
  $core.String get sas => $_getSZ(1);
  @$pb.TagNumber(2)
  set sas($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasSas() => $_has(1);
  @$pb.TagNumber(2)
  void clearSas() => $_clearField(2);
}

class PairReject extends $pb.GeneratedMessage {
  factory PairReject({
    Error? error,
  }) {
    final result = PairReject._();
    if (error != null) result.error = error;
    return result;
  }

  PairReject._();

  factory PairReject.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      PairReject()..mergeFromBuffer(data, registry);
  factory PairReject.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      PairReject()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'PairReject',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'phonebridge.v1'),
      createEmptyInstance: PairReject.$_createMessage)
    ..aOM<Error>(1, _omitFieldNames ? '' : 'error',
        subBuilder: Error.$_createMessage)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PairReject clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PairReject copyWith(void Function(PairReject) updates) =>
      super.copyWith((message) => updates(message as PairReject)) as PairReject;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated('Use PairReject() / PairReject.new instead')
  static PairReject create() => PairReject._();
  static $pb.GeneratedMessage $_createMessage() => PairReject._();
  @$core.override
  PairReject createEmptyInstance() => PairReject._();
  @$core.pragma('dart2js:noInline')
  static PairReject getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<PairReject>(PairReject.$_createMessage);
  static PairReject? _defaultInstance;

  @$pb.TagNumber(1)
  Error get error => $_getN(0);
  @$pb.TagNumber(1)
  set error(Error value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasError() => $_has(0);
  @$pb.TagNumber(1)
  void clearError() => $_clearField(1);
  @$pb.TagNumber(1)
  Error ensureError() => $_ensure(0);
}

class Ping extends $pb.GeneratedMessage {
  factory Ping({
    $fixnum.Int64? nonce,
  }) {
    final result = Ping._();
    if (nonce != null) result.nonce = nonce;
    return result;
  }

  Ping._();

  factory Ping.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      Ping()..mergeFromBuffer(data, registry);
  factory Ping.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      Ping()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'Ping',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'phonebridge.v1'),
      createEmptyInstance: Ping.$_createMessage)
    ..a<$fixnum.Int64>(1, _omitFieldNames ? '' : 'nonce', $pb.PbFieldType.OU6,
        defaultOrMaker: $fixnum.Int64.ZERO)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Ping clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Ping copyWith(void Function(Ping) updates) =>
      super.copyWith((message) => updates(message as Ping)) as Ping;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated('Use Ping() / Ping.new instead')
  static Ping create() => Ping._();
  static $pb.GeneratedMessage $_createMessage() => Ping._();
  @$core.override
  Ping createEmptyInstance() => Ping._();
  @$core.pragma('dart2js:noInline')
  static Ping getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<Ping>(Ping.$_createMessage);
  static Ping? _defaultInstance;

  @$pb.TagNumber(1)
  $fixnum.Int64 get nonce => $_getI64(0);
  @$pb.TagNumber(1)
  set nonce($fixnum.Int64 value) => $_setInt64(0, value);
  @$pb.TagNumber(1)
  $core.bool hasNonce() => $_has(0);
  @$pb.TagNumber(1)
  void clearNonce() => $_clearField(1);
}

class Pong extends $pb.GeneratedMessage {
  factory Pong({
    $fixnum.Int64? nonce,
  }) {
    final result = Pong._();
    if (nonce != null) result.nonce = nonce;
    return result;
  }

  Pong._();

  factory Pong.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      Pong()..mergeFromBuffer(data, registry);
  factory Pong.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      Pong()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'Pong',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'phonebridge.v1'),
      createEmptyInstance: Pong.$_createMessage)
    ..a<$fixnum.Int64>(1, _omitFieldNames ? '' : 'nonce', $pb.PbFieldType.OU6,
        defaultOrMaker: $fixnum.Int64.ZERO)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Pong clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Pong copyWith(void Function(Pong) updates) =>
      super.copyWith((message) => updates(message as Pong)) as Pong;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated('Use Pong() / Pong.new instead')
  static Pong create() => Pong._();
  static $pb.GeneratedMessage $_createMessage() => Pong._();
  @$core.override
  Pong createEmptyInstance() => Pong._();
  @$core.pragma('dart2js:noInline')
  static Pong getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<Pong>(Pong.$_createMessage);
  static Pong? _defaultInstance;

  @$pb.TagNumber(1)
  $fixnum.Int64 get nonce => $_getI64(0);
  @$pb.TagNumber(1)
  set nonce($fixnum.Int64 value) => $_setInt64(0, value);
  @$pb.TagNumber(1)
  $core.bool hasNonce() => $_has(0);
  @$pb.TagNumber(1)
  void clearNonce() => $_clearField(1);
}

class Error extends $pb.GeneratedMessage {
  factory Error({
    Code? code,
    $core.String? message,
    $core.Iterable<$core.MapEntry<$core.String, $core.String>>? details,
  }) {
    final result = Error._();
    if (code != null) result.code = code;
    if (message != null) result.message = message;
    if (details != null) result.details.addEntries(details);
    return result;
  }

  Error._();

  factory Error.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      Error()..mergeFromBuffer(data, registry);
  factory Error.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      Error()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'Error',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'phonebridge.v1'),
      createEmptyInstance: Error.$_createMessage)
    ..aE<Code>(1, _omitFieldNames ? '' : 'code', enumValues: Code.values)
    ..aOS(2, _omitFieldNames ? '' : 'message')
    ..m<$core.String, $core.String>(3, _omitFieldNames ? '' : 'details',
        entryClassName: 'Error.DetailsEntry',
        keyFieldType: $pb.PbFieldType.OS,
        valueFieldType: $pb.PbFieldType.OS,
        packageName: const $pb.PackageName('phonebridge.v1'))
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Error clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Error copyWith(void Function(Error) updates) =>
      super.copyWith((message) => updates(message as Error)) as Error;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated('Use Error() / Error.new instead')
  static Error create() => Error._();
  static $pb.GeneratedMessage $_createMessage() => Error._();
  @$core.override
  Error createEmptyInstance() => Error._();
  @$core.pragma('dart2js:noInline')
  static Error getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<Error>(Error.$_createMessage);
  static Error? _defaultInstance;

  @$pb.TagNumber(1)
  Code get code => $_getN(0);
  @$pb.TagNumber(1)
  set code(Code value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasCode() => $_has(0);
  @$pb.TagNumber(1)
  void clearCode() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get message => $_getSZ(1);
  @$pb.TagNumber(2)
  set message($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasMessage() => $_has(1);
  @$pb.TagNumber(2)
  void clearMessage() => $_clearField(2);

  @$pb.TagNumber(3)
  $pb.PbMap<$core.String, $core.String> get details => $_getMap(2);
}

/// ---------------------------------------------------------------------------
/// Screen session negotiation (DEC-022, Phase 2)
/// ---------------------------------------------------------------------------
/// ScreenStart carries the capture request and the capture device's answer.
/// The parameter tuple is MediaParams (below), so the request, the LAN
/// signaling exchange (DEC-022) and the local IPC session snapshot all speak
/// one shape.
///
/// Contract, in order:
///   1. The initiator sends ScreenStart with `requested` populated and no
///      answer fields. Android requires a MediaProjection consent before any
///      capture exists (DEC-020), so parameters must be settled BEFORE the
///      offer is composed and never mid-stream.
///   2. The capture device replies with `accepted` + `actual` — always. A
///      reply that differs from `requested` is a reported downgrade, never a
///      silent substitution: the initiator can compare the two tuples and
///      tell the user what is really being sent.
///   3. If nothing within the device's MediaCapabilities can satisfy the
///      request, `accepted` is false and `reject_reason` says why.
/// Resolution changes are never negotiated in-session: DEC-020 records that a
/// new geometry needs a new consent and a new session.
class ScreenStart extends $pb.GeneratedMessage {
  factory ScreenStart({
    MediaParams? requested,
    $core.bool? accepted,
    $core.String? rejectReason,
    MediaParams? actual,
  }) {
    final result = ScreenStart._();
    if (requested != null) result.requested = requested;
    if (accepted != null) result.accepted = accepted;
    if (rejectReason != null) result.rejectReason = rejectReason;
    if (actual != null) result.actual = actual;
    return result;
  }

  ScreenStart._();

  factory ScreenStart.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      ScreenStart()..mergeFromBuffer(data, registry);
  factory ScreenStart.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      ScreenStart()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ScreenStart',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'phonebridge.v1'),
      createEmptyInstance: ScreenStart.$_createMessage)
    ..aOM<MediaParams>(1, _omitFieldNames ? '' : 'requested',
        subBuilder: MediaParams.$_createMessage)
    ..aOB(2, _omitFieldNames ? '' : 'accepted')
    ..aOS(3, _omitFieldNames ? '' : 'rejectReason')
    ..aOM<MediaParams>(4, _omitFieldNames ? '' : 'actual',
        subBuilder: MediaParams.$_createMessage)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ScreenStart clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ScreenStart copyWith(void Function(ScreenStart) updates) =>
      super.copyWith((message) => updates(message as ScreenStart))
          as ScreenStart;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated('Use ScreenStart() / ScreenStart.new instead')
  static ScreenStart create() => ScreenStart._();
  static $pb.GeneratedMessage $_createMessage() => ScreenStart._();
  @$core.override
  ScreenStart createEmptyInstance() => ScreenStart._();
  @$core.pragma('dart2js:noInline')
  static ScreenStart getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<ScreenStart>(
          ScreenStart.$_createMessage);
  static ScreenStart? _defaultInstance;

  /// What the initiator asked for. Zero fields mean "no preference".
  @$pb.TagNumber(1)
  MediaParams get requested => $_getN(0);
  @$pb.TagNumber(1)
  set requested(MediaParams value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasRequested() => $_has(0);
  @$pb.TagNumber(1)
  void clearRequested() => $_clearField(1);
  @$pb.TagNumber(1)
  MediaParams ensureRequested() => $_ensure(0);

  /// True when `actual` equals `requested` exactly.
  @$pb.TagNumber(2)
  $core.bool get accepted => $_getBF(1);
  @$pb.TagNumber(2)
  set accepted($core.bool value) => $_setBool(1, value);
  @$pb.TagNumber(2)
  $core.bool hasAccepted() => $_has(1);
  @$pb.TagNumber(2)
  void clearAccepted() => $_clearField(2);

  /// Set when `accepted` is false: why the request could not be met.
  @$pb.TagNumber(3)
  $core.String get rejectReason => $_getSZ(2);
  @$pb.TagNumber(3)
  set rejectReason($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasRejectReason() => $_has(2);
  @$pb.TagNumber(3)
  void clearRejectReason() => $_clearField(3);

  /// What the capture device actually applied. Authoritative (DEC-020); the
  /// initiator must treat this as the truth for display and diagnostics.
  @$pb.TagNumber(4)
  MediaParams get actual => $_getN(3);
  @$pb.TagNumber(4)
  set actual(MediaParams value) => $_setField(4, value);
  @$pb.TagNumber(4)
  $core.bool hasActual() => $_has(3);
  @$pb.TagNumber(4)
  void clearActual() => $_clearField(4);
  @$pb.TagNumber(4)
  MediaParams ensureActual() => $_ensure(3);
}

/// ---------------------------------------------------------------------------
/// Feature payloads — PLANNED stubs. No fields populated in Phase 0.
/// Each will be expanded in its feature phase with reserved ranges preserved.
/// ---------------------------------------------------------------------------
class ClipboardUpdate extends $pb.GeneratedMessage {
  factory ClipboardUpdate() => ClipboardUpdate._();

  ClipboardUpdate._();

  factory ClipboardUpdate.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      ClipboardUpdate()..mergeFromBuffer(data, registry);
  factory ClipboardUpdate.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      ClipboardUpdate()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClipboardUpdate',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'phonebridge.v1'),
      createEmptyInstance: ClipboardUpdate.$_createMessage)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClipboardUpdate clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClipboardUpdate copyWith(void Function(ClipboardUpdate) updates) =>
      super.copyWith((message) => updates(message as ClipboardUpdate))
          as ClipboardUpdate;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated('Use ClipboardUpdate() / ClipboardUpdate.new instead')
  static ClipboardUpdate create() => ClipboardUpdate._();
  static $pb.GeneratedMessage $_createMessage() => ClipboardUpdate._();
  @$core.override
  ClipboardUpdate createEmptyInstance() => ClipboardUpdate._();
  @$core.pragma('dart2js:noInline')
  static ClipboardUpdate getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<ClipboardUpdate>(
          ClipboardUpdate.$_createMessage);
  static ClipboardUpdate? _defaultInstance;
}

class NotificationEvent extends $pb.GeneratedMessage {
  factory NotificationEvent() => NotificationEvent._();

  NotificationEvent._();

  factory NotificationEvent.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      NotificationEvent()..mergeFromBuffer(data, registry);
  factory NotificationEvent.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      NotificationEvent()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'NotificationEvent',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'phonebridge.v1'),
      createEmptyInstance: NotificationEvent.$_createMessage)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  NotificationEvent clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  NotificationEvent copyWith(void Function(NotificationEvent) updates) =>
      super.copyWith((message) => updates(message as NotificationEvent))
          as NotificationEvent;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated('Use NotificationEvent() / NotificationEvent.new instead')
  static NotificationEvent create() => NotificationEvent._();
  static $pb.GeneratedMessage $_createMessage() => NotificationEvent._();
  @$core.override
  NotificationEvent createEmptyInstance() => NotificationEvent._();
  @$core.pragma('dart2js:noInline')
  static NotificationEvent getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<NotificationEvent>(
          NotificationEvent.$_createMessage);
  static NotificationEvent? _defaultInstance;
}

class NotificationAction extends $pb.GeneratedMessage {
  factory NotificationAction() => NotificationAction._();

  NotificationAction._();

  factory NotificationAction.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      NotificationAction()..mergeFromBuffer(data, registry);
  factory NotificationAction.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      NotificationAction()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'NotificationAction',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'phonebridge.v1'),
      createEmptyInstance: NotificationAction.$_createMessage)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  NotificationAction clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  NotificationAction copyWith(void Function(NotificationAction) updates) =>
      super.copyWith((message) => updates(message as NotificationAction))
          as NotificationAction;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated('Use NotificationAction() / NotificationAction.new instead')
  static NotificationAction create() => NotificationAction._();
  static $pb.GeneratedMessage $_createMessage() => NotificationAction._();
  @$core.override
  NotificationAction createEmptyInstance() => NotificationAction._();
  @$core.pragma('dart2js:noInline')
  static NotificationAction getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<NotificationAction>(
          NotificationAction.$_createMessage);
  static NotificationAction? _defaultInstance;
}

class NotificationDismissed extends $pb.GeneratedMessage {
  factory NotificationDismissed() => NotificationDismissed._();

  NotificationDismissed._();

  factory NotificationDismissed.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      NotificationDismissed()..mergeFromBuffer(data, registry);
  factory NotificationDismissed.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      NotificationDismissed()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'NotificationDismissed',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'phonebridge.v1'),
      createEmptyInstance: NotificationDismissed.$_createMessage)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  NotificationDismissed clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  NotificationDismissed copyWith(
          void Function(NotificationDismissed) updates) =>
      super.copyWith((message) => updates(message as NotificationDismissed))
          as NotificationDismissed;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated(
      'Use NotificationDismissed() / NotificationDismissed.new instead')
  static NotificationDismissed create() => NotificationDismissed._();
  static $pb.GeneratedMessage $_createMessage() => NotificationDismissed._();
  @$core.override
  NotificationDismissed createEmptyInstance() => NotificationDismissed._();
  @$core.pragma('dart2js:noInline')
  static NotificationDismissed getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<NotificationDismissed>(
          NotificationDismissed.$_createMessage);
  static NotificationDismissed? _defaultInstance;
}

class FileOffer extends $pb.GeneratedMessage {
  factory FileOffer() => FileOffer._();

  FileOffer._();

  factory FileOffer.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      FileOffer()..mergeFromBuffer(data, registry);
  factory FileOffer.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      FileOffer()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FileOffer',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'phonebridge.v1'),
      createEmptyInstance: FileOffer.$_createMessage)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FileOffer clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FileOffer copyWith(void Function(FileOffer) updates) =>
      super.copyWith((message) => updates(message as FileOffer)) as FileOffer;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated('Use FileOffer() / FileOffer.new instead')
  static FileOffer create() => FileOffer._();
  static $pb.GeneratedMessage $_createMessage() => FileOffer._();
  @$core.override
  FileOffer createEmptyInstance() => FileOffer._();
  @$core.pragma('dart2js:noInline')
  static FileOffer getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<FileOffer>(FileOffer.$_createMessage);
  static FileOffer? _defaultInstance;
}

class FileAccept extends $pb.GeneratedMessage {
  factory FileAccept() => FileAccept._();

  FileAccept._();

  factory FileAccept.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      FileAccept()..mergeFromBuffer(data, registry);
  factory FileAccept.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      FileAccept()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FileAccept',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'phonebridge.v1'),
      createEmptyInstance: FileAccept.$_createMessage)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FileAccept clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FileAccept copyWith(void Function(FileAccept) updates) =>
      super.copyWith((message) => updates(message as FileAccept)) as FileAccept;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated('Use FileAccept() / FileAccept.new instead')
  static FileAccept create() => FileAccept._();
  static $pb.GeneratedMessage $_createMessage() => FileAccept._();
  @$core.override
  FileAccept createEmptyInstance() => FileAccept._();
  @$core.pragma('dart2js:noInline')
  static FileAccept getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<FileAccept>(FileAccept.$_createMessage);
  static FileAccept? _defaultInstance;
}

class FileChunk extends $pb.GeneratedMessage {
  factory FileChunk() => FileChunk._();

  FileChunk._();

  factory FileChunk.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      FileChunk()..mergeFromBuffer(data, registry);
  factory FileChunk.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      FileChunk()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FileChunk',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'phonebridge.v1'),
      createEmptyInstance: FileChunk.$_createMessage)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FileChunk clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FileChunk copyWith(void Function(FileChunk) updates) =>
      super.copyWith((message) => updates(message as FileChunk)) as FileChunk;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated('Use FileChunk() / FileChunk.new instead')
  static FileChunk create() => FileChunk._();
  static $pb.GeneratedMessage $_createMessage() => FileChunk._();
  @$core.override
  FileChunk createEmptyInstance() => FileChunk._();
  @$core.pragma('dart2js:noInline')
  static FileChunk getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<FileChunk>(FileChunk.$_createMessage);
  static FileChunk? _defaultInstance;
}

class FileComplete extends $pb.GeneratedMessage {
  factory FileComplete() => FileComplete._();

  FileComplete._();

  factory FileComplete.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      FileComplete()..mergeFromBuffer(data, registry);
  factory FileComplete.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      FileComplete()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FileComplete',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'phonebridge.v1'),
      createEmptyInstance: FileComplete.$_createMessage)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FileComplete clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FileComplete copyWith(void Function(FileComplete) updates) =>
      super.copyWith((message) => updates(message as FileComplete))
          as FileComplete;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated('Use FileComplete() / FileComplete.new instead')
  static FileComplete create() => FileComplete._();
  static $pb.GeneratedMessage $_createMessage() => FileComplete._();
  @$core.override
  FileComplete createEmptyInstance() => FileComplete._();
  @$core.pragma('dart2js:noInline')
  static FileComplete getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<FileComplete>(
          FileComplete.$_createMessage);
  static FileComplete? _defaultInstance;
}

class FileCancel extends $pb.GeneratedMessage {
  factory FileCancel() => FileCancel._();

  FileCancel._();

  factory FileCancel.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      FileCancel()..mergeFromBuffer(data, registry);
  factory FileCancel.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      FileCancel()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FileCancel',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'phonebridge.v1'),
      createEmptyInstance: FileCancel.$_createMessage)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FileCancel clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FileCancel copyWith(void Function(FileCancel) updates) =>
      super.copyWith((message) => updates(message as FileCancel)) as FileCancel;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated('Use FileCancel() / FileCancel.new instead')
  static FileCancel create() => FileCancel._();
  static $pb.GeneratedMessage $_createMessage() => FileCancel._();
  @$core.override
  FileCancel createEmptyInstance() => FileCancel._();
  @$core.pragma('dart2js:noInline')
  static FileCancel getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<FileCancel>(FileCancel.$_createMessage);
  static FileCancel? _defaultInstance;
}

class ScreenStop extends $pb.GeneratedMessage {
  factory ScreenStop({
    Code? reasonCode,
    $core.String? reason,
  }) {
    final result = ScreenStop._();
    if (reasonCode != null) result.reasonCode = reasonCode;
    if (reason != null) result.reason = reason;
    return result;
  }

  ScreenStop._();

  factory ScreenStop.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      ScreenStop()..mergeFromBuffer(data, registry);
  factory ScreenStop.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      ScreenStop()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ScreenStop',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'phonebridge.v1'),
      createEmptyInstance: ScreenStop.$_createMessage)
    ..aE<Code>(1, _omitFieldNames ? '' : 'reasonCode', enumValues: Code.values)
    ..aOS(2, _omitFieldNames ? '' : 'reason')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ScreenStop clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ScreenStop copyWith(void Function(ScreenStop) updates) =>
      super.copyWith((message) => updates(message as ScreenStop)) as ScreenStop;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated('Use ScreenStop() / ScreenStop.new instead')
  static ScreenStop create() => ScreenStop._();
  static $pb.GeneratedMessage $_createMessage() => ScreenStop._();
  @$core.override
  ScreenStop createEmptyInstance() => ScreenStop._();
  @$core.pragma('dart2js:noInline')
  static ScreenStop getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ScreenStop>(ScreenStop.$_createMessage);
  static ScreenStop? _defaultInstance;

  /// Why the stream stopped (typed where the sender knows the cause).
  @$pb.TagNumber(1)
  Code get reasonCode => $_getN(0);
  @$pb.TagNumber(1)
  set reasonCode(Code value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasReasonCode() => $_has(0);
  @$pb.TagNumber(1)
  void clearReasonCode() => $_clearField(1);

  /// Human-readable detail for logs and diagnostics; never parsed by code.
  @$pb.TagNumber(2)
  $core.String get reason => $_getSZ(1);
  @$pb.TagNumber(2)
  set reason($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasReason() => $_has(1);
  @$pb.TagNumber(2)
  void clearReason() => $_clearField(2);
}

class InputKeyboard extends $pb.GeneratedMessage {
  factory InputKeyboard() => InputKeyboard._();

  InputKeyboard._();

  factory InputKeyboard.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      InputKeyboard()..mergeFromBuffer(data, registry);
  factory InputKeyboard.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      InputKeyboard()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'InputKeyboard',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'phonebridge.v1'),
      createEmptyInstance: InputKeyboard.$_createMessage)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  InputKeyboard clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  InputKeyboard copyWith(void Function(InputKeyboard) updates) =>
      super.copyWith((message) => updates(message as InputKeyboard))
          as InputKeyboard;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated('Use InputKeyboard() / InputKeyboard.new instead')
  static InputKeyboard create() => InputKeyboard._();
  static $pb.GeneratedMessage $_createMessage() => InputKeyboard._();
  @$core.override
  InputKeyboard createEmptyInstance() => InputKeyboard._();
  @$core.pragma('dart2js:noInline')
  static InputKeyboard getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<InputKeyboard>(
          InputKeyboard.$_createMessage);
  static InputKeyboard? _defaultInstance;
}

class InputMouse extends $pb.GeneratedMessage {
  factory InputMouse() => InputMouse._();

  InputMouse._();

  factory InputMouse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      InputMouse()..mergeFromBuffer(data, registry);
  factory InputMouse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      InputMouse()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'InputMouse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'phonebridge.v1'),
      createEmptyInstance: InputMouse.$_createMessage)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  InputMouse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  InputMouse copyWith(void Function(InputMouse) updates) =>
      super.copyWith((message) => updates(message as InputMouse)) as InputMouse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated('Use InputMouse() / InputMouse.new instead')
  static InputMouse create() => InputMouse._();
  static $pb.GeneratedMessage $_createMessage() => InputMouse._();
  @$core.override
  InputMouse createEmptyInstance() => InputMouse._();
  @$core.pragma('dart2js:noInline')
  static InputMouse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<InputMouse>(InputMouse.$_createMessage);
  static InputMouse? _defaultInstance;
}

class InputTouch extends $pb.GeneratedMessage {
  factory InputTouch() => InputTouch._();

  InputTouch._();

  factory InputTouch.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      InputTouch()..mergeFromBuffer(data, registry);
  factory InputTouch.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      InputTouch()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'InputTouch',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'phonebridge.v1'),
      createEmptyInstance: InputTouch.$_createMessage)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  InputTouch clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  InputTouch copyWith(void Function(InputTouch) updates) =>
      super.copyWith((message) => updates(message as InputTouch)) as InputTouch;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated('Use InputTouch() / InputTouch.new instead')
  static InputTouch create() => InputTouch._();
  static $pb.GeneratedMessage $_createMessage() => InputTouch._();
  @$core.override
  InputTouch createEmptyInstance() => InputTouch._();
  @$core.pragma('dart2js:noInline')
  static InputTouch getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<InputTouch>(InputTouch.$_createMessage);
  static InputTouch? _defaultInstance;
}

class DeviceStatus extends $pb.GeneratedMessage {
  factory DeviceStatus() => DeviceStatus._();

  DeviceStatus._();

  factory DeviceStatus.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      DeviceStatus()..mergeFromBuffer(data, registry);
  factory DeviceStatus.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      DeviceStatus()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'DeviceStatus',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'phonebridge.v1'),
      createEmptyInstance: DeviceStatus.$_createMessage)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  DeviceStatus clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  DeviceStatus copyWith(void Function(DeviceStatus) updates) =>
      super.copyWith((message) => updates(message as DeviceStatus))
          as DeviceStatus;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated('Use DeviceStatus() / DeviceStatus.new instead')
  static DeviceStatus create() => DeviceStatus._();
  static $pb.GeneratedMessage $_createMessage() => DeviceStatus._();
  @$core.override
  DeviceStatus createEmptyInstance() => DeviceStatus._();
  @$core.pragma('dart2js:noInline')
  static DeviceStatus getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<DeviceStatus>(
          DeviceStatus.$_createMessage);
  static DeviceStatus? _defaultInstance;
}

const $core.bool _omitFieldNames =
    $core.bool.fromEnvironment('protobuf.omit_field_names');
const $core.bool _omitMessageNames =
    $core.bool.fromEnvironment('protobuf.omit_message_names');
