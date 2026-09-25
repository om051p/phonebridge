// This is a generated file - do not edit.
//
// Generated from phonebridge/localipc/v1/local_ipc.proto.

// @dart = 3.3

// ignore_for_file: annotate_overrides, camel_case_types, comment_references
// ignore_for_file: constant_identifier_names
// ignore_for_file: curly_braces_in_flow_control_structures
// ignore_for_file: deprecated_member_use_from_same_package, library_prefixes
// ignore_for_file: non_constant_identifier_names, prefer_relative_imports

import 'dart:async' as $async;
import 'dart:core' as $core;

import 'package:grpc/service_api.dart' as $grpc;
import 'package:protobuf/protobuf.dart' as $pb;

import 'local_ipc.pb.dart' as $0;

export 'local_ipc.pb.dart';

/// ---------------------------------------------------------------------------
/// LocalEngineService — the UI-facing engine service on the local Unix socket.
/// ---------------------------------------------------------------------------
@$pb.GrpcServiceName('phonebridge.localipc.v1.LocalEngineService')
class LocalEngineServiceClient extends $grpc.Client {
  /// The hostname for this service.
  static const $core.String defaultHost = '';

  /// OAuth scopes needed for the client.
  static const $core.List<$core.String> oauthScopes = [
    '',
  ];

  LocalEngineServiceClient(super.channel, {super.options, super.interceptors});

  /// Handshake negotiates the local protocol version and returns daemon
  /// identity. Must be the first call on a new connection.
  $grpc.ResponseFuture<$0.HandshakeResponse> handshake(
    $0.HandshakeRequest request, {
    $grpc.CallOptions? options,
  }) {
    return $createUnaryCall(_$handshake, request, options: options);
  }

  /// Ping is a cheap liveness/reconnect probe.
  $grpc.ResponseFuture<$0.PingResponse> ping(
    $0.PingRequest request, {
    $grpc.CallOptions? options,
  }) {
    return $createUnaryCall(_$ping, request, options: options);
  }

  /// StreamEvents pushes relayed device-protocol messages to the UI.
  /// Server-streaming; the daemon ends the stream cleanly on shutdown, and
  /// the client re-subscribes with backoff after any stream end.
  $grpc.ResponseStream<$0.StreamEventsResponse> streamEvents(
    $0.StreamEventsRequest request, {
    $grpc.CallOptions? options,
  }) {
    return $createStreamingCall(
        _$streamEvents, $async.Stream.fromIterable([request]),
        options: options);
  }

  /// Health reports daemon readiness without opening an event stream.
  $grpc.ResponseFuture<$0.HealthResponse> health(
    $0.HealthRequest request, {
    $grpc.CallOptions? options,
  }) {
    return $createUnaryCall(_$health, request, options: options);
  }

  /// StartSession initiates a new connection session with a discovered phone.
  $grpc.ResponseFuture<$0.StartSessionResponse> startSession(
    $0.StartSessionRequest request, {
    $grpc.CallOptions? options,
  }) {
    return $createUnaryCall(_$startSession, request, options: options);
  }

  /// StopSession cleanly terminates an active session.
  $grpc.ResponseFuture<$0.StopSessionResponse> stopSession(
    $0.StopSessionRequest request, {
    $grpc.CallOptions? options,
  }) {
    return $createUnaryCall(_$stopSession, request, options: options);
  }

  /// GetSessionState queries the current phase, device, and metrics of a session.
  $grpc.ResponseFuture<$0.GetSessionStateResponse> getSessionState(
    $0.GetSessionStateRequest request, {
    $grpc.CallOptions? options,
  }) {
    return $createUnaryCall(_$getSessionState, request, options: options);
  }

  /// ListDevices returns all currently discovered LAN devices from mDNS.
  $grpc.ResponseFuture<$0.ListDevicesResponse> listDevices(
    $0.ListDevicesRequest request, {
    $grpc.CallOptions? options,
  }) {
    return $createUnaryCall(_$listDevices, request, options: options);
  }

  /// PairDevice initiates pairing with a discovered LAN device.
  $grpc.ResponseFuture<$0.PairDeviceResponse> pairDevice(
    $0.PairDeviceRequest request, {
    $grpc.CallOptions? options,
  }) {
    return $createUnaryCall(_$pairDevice, request, options: options);
  }

  /// ConfirmPairing completes pairing by confirming the SAS.
  $grpc.ResponseFuture<$0.ConfirmPairingResponse> confirmPairing(
    $0.ConfirmPairingRequest request, {
    $grpc.CallOptions? options,
  }) {
    return $createUnaryCall(_$confirmPairing, request, options: options);
  }

  /// ListTrustedDevices returns all trusted/paired devices.
  $grpc.ResponseFuture<$0.ListTrustedDevicesResponse> listTrustedDevices(
    $0.ListTrustedDevicesRequest request, {
    $grpc.CallOptions? options,
  }) {
    return $createUnaryCall(_$listTrustedDevices, request, options: options);
  }

  /// RevokeDevice revokes trust for a previously paired device.
  $grpc.ResponseFuture<$0.RevokeDeviceResponse> revokeDevice(
    $0.RevokeDeviceRequest request, {
    $grpc.CallOptions? options,
  }) {
    return $createUnaryCall(_$revokeDevice, request, options: options);
  }

  /// GetClipboardStatus returns the current status of the host clipboard adapter and engine.
  $grpc.ResponseFuture<$0.GetClipboardStatusResponse> getClipboardStatus(
    $0.GetClipboardStatusRequest request, {
    $grpc.CallOptions? options,
  }) {
    return $createUnaryCall(_$getClipboardStatus, request, options: options);
  }

  /// TriggerClipboardPull reads the host clipboard and synchronizes it to the active peer.
  $grpc.ResponseFuture<$0.TriggerClipboardPullResponse> triggerClipboardPull(
    $0.TriggerClipboardPullRequest request, {
    $grpc.CallOptions? options,
  }) {
    return $createUnaryCall(_$triggerClipboardPull, request, options: options);
  }

  /// SendFile offers a local file to the active peer over the session's
  /// dedicated "transfer" DataChannel (DEC-024). The daemon reads local_path
  /// itself: file bytes never cross this local boundary, and no whole file is
  /// buffered anywhere (streaming chunked I/O).
  $grpc.ResponseFuture<$0.SendFileResponse> sendFile(
    $0.SendFileRequest request, {
    $grpc.CallOptions? options,
  }) {
    return $createUnaryCall(_$sendFile, request, options: options);
  }

  /// CancelTransfer aborts an in-flight transfer in either direction. The
  /// cancel frame goes to the peer and the partial file is deleted locally.
  $grpc.ResponseFuture<$0.CancelTransferResponse> cancelTransfer(
    $0.CancelTransferRequest request, {
    $grpc.CallOptions? options,
  }) {
    return $createUnaryCall(_$cancelTransfer, request, options: options);
  }

  /// ListTransfers returns in-flight transfers plus the recent in-memory
  /// history (Phase 4 does not persist history across daemon restarts).
  $grpc.ResponseFuture<$0.ListTransfersResponse> listTransfers(
    $0.ListTransfersRequest request, {
    $grpc.CallOptions? options,
  }) {
    return $createUnaryCall(_$listTransfers, request, options: options);
  }

  /// StreamFrames pushes encoded video frames of the active session to the UI
  /// renderer (Phase 6 Slice 3, additive). Deliberately a SEPARATE stream from
  /// StreamEvents: control events must never be starved or dropped by frame
  /// traffic, and frames are opt-in per subscriber (one renderer at a time,
  /// DEC-018's one-consumer rule). v1 carries JPEG frames produced by the
  /// daemon-side tap; the message shape is transport-generic (chunked opaque
  /// image bytes + geometry). The server ends the stream when the session it
  /// is bound to reaches a terminal state; clients resubscribe via their
  /// existing backoff pattern only while a session is live.
  $grpc.ResponseStream<$0.StreamFramesResponse> streamFrames(
    $0.StreamFramesRequest request, {
    $grpc.CallOptions? options,
  }) {
    return $createStreamingCall(
        _$streamFrames, $async.Stream.fromIterable([request]),
        options: options);
  }

  /// SendInput injects a remote input event to the active peer over the session's
  /// dedicated "input" DataChannel (Phase 7, DEC-027).
  $grpc.ResponseFuture<$0.SendInputResponse> sendInput(
    $0.SendInputRequest request, {
    $grpc.CallOptions? options,
  }) {
    return $createUnaryCall(_$sendInput, request, options: options);
  }

  /// ListNotifications returns the active in-memory mirrored notifications (Phase 8, DEC-028).
  $grpc.ResponseFuture<$0.ListNotificationsResponse> listNotifications(
    $0.ListNotificationsRequest request, {
    $grpc.CallOptions? options,
  }) {
    return $createUnaryCall(_$listNotifications, request, options: options);
  }

  // method descriptors

  static final _$handshake =
      $grpc.ClientMethod<$0.HandshakeRequest, $0.HandshakeResponse>(
          '/phonebridge.localipc.v1.LocalEngineService/Handshake',
          ($0.HandshakeRequest value) => value.writeToBuffer(),
          $0.HandshakeResponse.fromBuffer);
  static final _$ping = $grpc.ClientMethod<$0.PingRequest, $0.PingResponse>(
      '/phonebridge.localipc.v1.LocalEngineService/Ping',
      ($0.PingRequest value) => value.writeToBuffer(),
      $0.PingResponse.fromBuffer);
  static final _$streamEvents =
      $grpc.ClientMethod<$0.StreamEventsRequest, $0.StreamEventsResponse>(
          '/phonebridge.localipc.v1.LocalEngineService/StreamEvents',
          ($0.StreamEventsRequest value) => value.writeToBuffer(),
          $0.StreamEventsResponse.fromBuffer);
  static final _$health =
      $grpc.ClientMethod<$0.HealthRequest, $0.HealthResponse>(
          '/phonebridge.localipc.v1.LocalEngineService/Health',
          ($0.HealthRequest value) => value.writeToBuffer(),
          $0.HealthResponse.fromBuffer);
  static final _$startSession =
      $grpc.ClientMethod<$0.StartSessionRequest, $0.StartSessionResponse>(
          '/phonebridge.localipc.v1.LocalEngineService/StartSession',
          ($0.StartSessionRequest value) => value.writeToBuffer(),
          $0.StartSessionResponse.fromBuffer);
  static final _$stopSession =
      $grpc.ClientMethod<$0.StopSessionRequest, $0.StopSessionResponse>(
          '/phonebridge.localipc.v1.LocalEngineService/StopSession',
          ($0.StopSessionRequest value) => value.writeToBuffer(),
          $0.StopSessionResponse.fromBuffer);
  static final _$getSessionState =
      $grpc.ClientMethod<$0.GetSessionStateRequest, $0.GetSessionStateResponse>(
          '/phonebridge.localipc.v1.LocalEngineService/GetSessionState',
          ($0.GetSessionStateRequest value) => value.writeToBuffer(),
          $0.GetSessionStateResponse.fromBuffer);
  static final _$listDevices =
      $grpc.ClientMethod<$0.ListDevicesRequest, $0.ListDevicesResponse>(
          '/phonebridge.localipc.v1.LocalEngineService/ListDevices',
          ($0.ListDevicesRequest value) => value.writeToBuffer(),
          $0.ListDevicesResponse.fromBuffer);
  static final _$pairDevice =
      $grpc.ClientMethod<$0.PairDeviceRequest, $0.PairDeviceResponse>(
          '/phonebridge.localipc.v1.LocalEngineService/PairDevice',
          ($0.PairDeviceRequest value) => value.writeToBuffer(),
          $0.PairDeviceResponse.fromBuffer);
  static final _$confirmPairing =
      $grpc.ClientMethod<$0.ConfirmPairingRequest, $0.ConfirmPairingResponse>(
          '/phonebridge.localipc.v1.LocalEngineService/ConfirmPairing',
          ($0.ConfirmPairingRequest value) => value.writeToBuffer(),
          $0.ConfirmPairingResponse.fromBuffer);
  static final _$listTrustedDevices = $grpc.ClientMethod<
          $0.ListTrustedDevicesRequest, $0.ListTrustedDevicesResponse>(
      '/phonebridge.localipc.v1.LocalEngineService/ListTrustedDevices',
      ($0.ListTrustedDevicesRequest value) => value.writeToBuffer(),
      $0.ListTrustedDevicesResponse.fromBuffer);
  static final _$revokeDevice =
      $grpc.ClientMethod<$0.RevokeDeviceRequest, $0.RevokeDeviceResponse>(
          '/phonebridge.localipc.v1.LocalEngineService/RevokeDevice',
          ($0.RevokeDeviceRequest value) => value.writeToBuffer(),
          $0.RevokeDeviceResponse.fromBuffer);
  static final _$getClipboardStatus = $grpc.ClientMethod<
          $0.GetClipboardStatusRequest, $0.GetClipboardStatusResponse>(
      '/phonebridge.localipc.v1.LocalEngineService/GetClipboardStatus',
      ($0.GetClipboardStatusRequest value) => value.writeToBuffer(),
      $0.GetClipboardStatusResponse.fromBuffer);
  static final _$triggerClipboardPull = $grpc.ClientMethod<
          $0.TriggerClipboardPullRequest, $0.TriggerClipboardPullResponse>(
      '/phonebridge.localipc.v1.LocalEngineService/TriggerClipboardPull',
      ($0.TriggerClipboardPullRequest value) => value.writeToBuffer(),
      $0.TriggerClipboardPullResponse.fromBuffer);
  static final _$sendFile =
      $grpc.ClientMethod<$0.SendFileRequest, $0.SendFileResponse>(
          '/phonebridge.localipc.v1.LocalEngineService/SendFile',
          ($0.SendFileRequest value) => value.writeToBuffer(),
          $0.SendFileResponse.fromBuffer);
  static final _$cancelTransfer =
      $grpc.ClientMethod<$0.CancelTransferRequest, $0.CancelTransferResponse>(
          '/phonebridge.localipc.v1.LocalEngineService/CancelTransfer',
          ($0.CancelTransferRequest value) => value.writeToBuffer(),
          $0.CancelTransferResponse.fromBuffer);
  static final _$listTransfers =
      $grpc.ClientMethod<$0.ListTransfersRequest, $0.ListTransfersResponse>(
          '/phonebridge.localipc.v1.LocalEngineService/ListTransfers',
          ($0.ListTransfersRequest value) => value.writeToBuffer(),
          $0.ListTransfersResponse.fromBuffer);
  static final _$streamFrames =
      $grpc.ClientMethod<$0.StreamFramesRequest, $0.StreamFramesResponse>(
          '/phonebridge.localipc.v1.LocalEngineService/StreamFrames',
          ($0.StreamFramesRequest value) => value.writeToBuffer(),
          $0.StreamFramesResponse.fromBuffer);
  static final _$sendInput =
      $grpc.ClientMethod<$0.SendInputRequest, $0.SendInputResponse>(
          '/phonebridge.localipc.v1.LocalEngineService/SendInput',
          ($0.SendInputRequest value) => value.writeToBuffer(),
          $0.SendInputResponse.fromBuffer);
  static final _$listNotifications = $grpc.ClientMethod<
          $0.ListNotificationsRequest, $0.ListNotificationsResponse>(
      '/phonebridge.localipc.v1.LocalEngineService/ListNotifications',
      ($0.ListNotificationsRequest value) => value.writeToBuffer(),
      $0.ListNotificationsResponse.fromBuffer);
}

@$pb.GrpcServiceName('phonebridge.localipc.v1.LocalEngineService')
abstract class LocalEngineServiceBase extends $grpc.Service {
  $core.String get $name => 'phonebridge.localipc.v1.LocalEngineService';

  LocalEngineServiceBase() {
    $addMethod($grpc.ServiceMethod<$0.HandshakeRequest, $0.HandshakeResponse>(
        'Handshake',
        handshake_Pre,
        false,
        false,
        ($core.List<$core.int> value) => $0.HandshakeRequest.fromBuffer(value),
        ($0.HandshakeResponse value) => value.writeToBuffer()));
    $addMethod($grpc.ServiceMethod<$0.PingRequest, $0.PingResponse>(
        'Ping',
        ping_Pre,
        false,
        false,
        ($core.List<$core.int> value) => $0.PingRequest.fromBuffer(value),
        ($0.PingResponse value) => value.writeToBuffer()));
    $addMethod(
        $grpc.ServiceMethod<$0.StreamEventsRequest, $0.StreamEventsResponse>(
            'StreamEvents',
            streamEvents_Pre,
            false,
            true,
            ($core.List<$core.int> value) =>
                $0.StreamEventsRequest.fromBuffer(value),
            ($0.StreamEventsResponse value) => value.writeToBuffer()));
    $addMethod($grpc.ServiceMethod<$0.HealthRequest, $0.HealthResponse>(
        'Health',
        health_Pre,
        false,
        false,
        ($core.List<$core.int> value) => $0.HealthRequest.fromBuffer(value),
        ($0.HealthResponse value) => value.writeToBuffer()));
    $addMethod(
        $grpc.ServiceMethod<$0.StartSessionRequest, $0.StartSessionResponse>(
            'StartSession',
            startSession_Pre,
            false,
            false,
            ($core.List<$core.int> value) =>
                $0.StartSessionRequest.fromBuffer(value),
            ($0.StartSessionResponse value) => value.writeToBuffer()));
    $addMethod(
        $grpc.ServiceMethod<$0.StopSessionRequest, $0.StopSessionResponse>(
            'StopSession',
            stopSession_Pre,
            false,
            false,
            ($core.List<$core.int> value) =>
                $0.StopSessionRequest.fromBuffer(value),
            ($0.StopSessionResponse value) => value.writeToBuffer()));
    $addMethod($grpc.ServiceMethod<$0.GetSessionStateRequest,
            $0.GetSessionStateResponse>(
        'GetSessionState',
        getSessionState_Pre,
        false,
        false,
        ($core.List<$core.int> value) =>
            $0.GetSessionStateRequest.fromBuffer(value),
        ($0.GetSessionStateResponse value) => value.writeToBuffer()));
    $addMethod(
        $grpc.ServiceMethod<$0.ListDevicesRequest, $0.ListDevicesResponse>(
            'ListDevices',
            listDevices_Pre,
            false,
            false,
            ($core.List<$core.int> value) =>
                $0.ListDevicesRequest.fromBuffer(value),
            ($0.ListDevicesResponse value) => value.writeToBuffer()));
    $addMethod($grpc.ServiceMethod<$0.PairDeviceRequest, $0.PairDeviceResponse>(
        'PairDevice',
        pairDevice_Pre,
        false,
        false,
        ($core.List<$core.int> value) => $0.PairDeviceRequest.fromBuffer(value),
        ($0.PairDeviceResponse value) => value.writeToBuffer()));
    $addMethod($grpc.ServiceMethod<$0.ConfirmPairingRequest,
            $0.ConfirmPairingResponse>(
        'ConfirmPairing',
        confirmPairing_Pre,
        false,
        false,
        ($core.List<$core.int> value) =>
            $0.ConfirmPairingRequest.fromBuffer(value),
        ($0.ConfirmPairingResponse value) => value.writeToBuffer()));
    $addMethod($grpc.ServiceMethod<$0.ListTrustedDevicesRequest,
            $0.ListTrustedDevicesResponse>(
        'ListTrustedDevices',
        listTrustedDevices_Pre,
        false,
        false,
        ($core.List<$core.int> value) =>
            $0.ListTrustedDevicesRequest.fromBuffer(value),
        ($0.ListTrustedDevicesResponse value) => value.writeToBuffer()));
    $addMethod(
        $grpc.ServiceMethod<$0.RevokeDeviceRequest, $0.RevokeDeviceResponse>(
            'RevokeDevice',
            revokeDevice_Pre,
            false,
            false,
            ($core.List<$core.int> value) =>
                $0.RevokeDeviceRequest.fromBuffer(value),
            ($0.RevokeDeviceResponse value) => value.writeToBuffer()));
    $addMethod($grpc.ServiceMethod<$0.GetClipboardStatusRequest,
            $0.GetClipboardStatusResponse>(
        'GetClipboardStatus',
        getClipboardStatus_Pre,
        false,
        false,
        ($core.List<$core.int> value) =>
            $0.GetClipboardStatusRequest.fromBuffer(value),
        ($0.GetClipboardStatusResponse value) => value.writeToBuffer()));
    $addMethod($grpc.ServiceMethod<$0.TriggerClipboardPullRequest,
            $0.TriggerClipboardPullResponse>(
        'TriggerClipboardPull',
        triggerClipboardPull_Pre,
        false,
        false,
        ($core.List<$core.int> value) =>
            $0.TriggerClipboardPullRequest.fromBuffer(value),
        ($0.TriggerClipboardPullResponse value) => value.writeToBuffer()));
    $addMethod($grpc.ServiceMethod<$0.SendFileRequest, $0.SendFileResponse>(
        'SendFile',
        sendFile_Pre,
        false,
        false,
        ($core.List<$core.int> value) => $0.SendFileRequest.fromBuffer(value),
        ($0.SendFileResponse value) => value.writeToBuffer()));
    $addMethod($grpc.ServiceMethod<$0.CancelTransferRequest,
            $0.CancelTransferResponse>(
        'CancelTransfer',
        cancelTransfer_Pre,
        false,
        false,
        ($core.List<$core.int> value) =>
            $0.CancelTransferRequest.fromBuffer(value),
        ($0.CancelTransferResponse value) => value.writeToBuffer()));
    $addMethod(
        $grpc.ServiceMethod<$0.ListTransfersRequest, $0.ListTransfersResponse>(
            'ListTransfers',
            listTransfers_Pre,
            false,
            false,
            ($core.List<$core.int> value) =>
                $0.ListTransfersRequest.fromBuffer(value),
            ($0.ListTransfersResponse value) => value.writeToBuffer()));
    $addMethod(
        $grpc.ServiceMethod<$0.StreamFramesRequest, $0.StreamFramesResponse>(
            'StreamFrames',
            streamFrames_Pre,
            false,
            true,
            ($core.List<$core.int> value) =>
                $0.StreamFramesRequest.fromBuffer(value),
            ($0.StreamFramesResponse value) => value.writeToBuffer()));
    $addMethod($grpc.ServiceMethod<$0.SendInputRequest, $0.SendInputResponse>(
        'SendInput',
        sendInput_Pre,
        false,
        false,
        ($core.List<$core.int> value) => $0.SendInputRequest.fromBuffer(value),
        ($0.SendInputResponse value) => value.writeToBuffer()));
    $addMethod($grpc.ServiceMethod<$0.ListNotificationsRequest,
            $0.ListNotificationsResponse>(
        'ListNotifications',
        listNotifications_Pre,
        false,
        false,
        ($core.List<$core.int> value) =>
            $0.ListNotificationsRequest.fromBuffer(value),
        ($0.ListNotificationsResponse value) => value.writeToBuffer()));
  }

  $async.Future<$0.HandshakeResponse> handshake_Pre($grpc.ServiceCall $call,
      $async.Future<$0.HandshakeRequest> $request) async {
    return handshake($call, await $request);
  }

  $async.Future<$0.HandshakeResponse> handshake(
      $grpc.ServiceCall call, $0.HandshakeRequest request);

  $async.Future<$0.PingResponse> ping_Pre(
      $grpc.ServiceCall $call, $async.Future<$0.PingRequest> $request) async {
    return ping($call, await $request);
  }

  $async.Future<$0.PingResponse> ping(
      $grpc.ServiceCall call, $0.PingRequest request);

  $async.Stream<$0.StreamEventsResponse> streamEvents_Pre(
      $grpc.ServiceCall $call,
      $async.Future<$0.StreamEventsRequest> $request) async* {
    yield* streamEvents($call, await $request);
  }

  $async.Stream<$0.StreamEventsResponse> streamEvents(
      $grpc.ServiceCall call, $0.StreamEventsRequest request);

  $async.Future<$0.HealthResponse> health_Pre(
      $grpc.ServiceCall $call, $async.Future<$0.HealthRequest> $request) async {
    return health($call, await $request);
  }

  $async.Future<$0.HealthResponse> health(
      $grpc.ServiceCall call, $0.HealthRequest request);

  $async.Future<$0.StartSessionResponse> startSession_Pre(
      $grpc.ServiceCall $call,
      $async.Future<$0.StartSessionRequest> $request) async {
    return startSession($call, await $request);
  }

  $async.Future<$0.StartSessionResponse> startSession(
      $grpc.ServiceCall call, $0.StartSessionRequest request);

  $async.Future<$0.StopSessionResponse> stopSession_Pre($grpc.ServiceCall $call,
      $async.Future<$0.StopSessionRequest> $request) async {
    return stopSession($call, await $request);
  }

  $async.Future<$0.StopSessionResponse> stopSession(
      $grpc.ServiceCall call, $0.StopSessionRequest request);

  $async.Future<$0.GetSessionStateResponse> getSessionState_Pre(
      $grpc.ServiceCall $call,
      $async.Future<$0.GetSessionStateRequest> $request) async {
    return getSessionState($call, await $request);
  }

  $async.Future<$0.GetSessionStateResponse> getSessionState(
      $grpc.ServiceCall call, $0.GetSessionStateRequest request);

  $async.Future<$0.ListDevicesResponse> listDevices_Pre($grpc.ServiceCall $call,
      $async.Future<$0.ListDevicesRequest> $request) async {
    return listDevices($call, await $request);
  }

  $async.Future<$0.ListDevicesResponse> listDevices(
      $grpc.ServiceCall call, $0.ListDevicesRequest request);

  $async.Future<$0.PairDeviceResponse> pairDevice_Pre($grpc.ServiceCall $call,
      $async.Future<$0.PairDeviceRequest> $request) async {
    return pairDevice($call, await $request);
  }

  $async.Future<$0.PairDeviceResponse> pairDevice(
      $grpc.ServiceCall call, $0.PairDeviceRequest request);

  $async.Future<$0.ConfirmPairingResponse> confirmPairing_Pre(
      $grpc.ServiceCall $call,
      $async.Future<$0.ConfirmPairingRequest> $request) async {
    return confirmPairing($call, await $request);
  }

  $async.Future<$0.ConfirmPairingResponse> confirmPairing(
      $grpc.ServiceCall call, $0.ConfirmPairingRequest request);

  $async.Future<$0.ListTrustedDevicesResponse> listTrustedDevices_Pre(
      $grpc.ServiceCall $call,
      $async.Future<$0.ListTrustedDevicesRequest> $request) async {
    return listTrustedDevices($call, await $request);
  }

  $async.Future<$0.ListTrustedDevicesResponse> listTrustedDevices(
      $grpc.ServiceCall call, $0.ListTrustedDevicesRequest request);

  $async.Future<$0.RevokeDeviceResponse> revokeDevice_Pre(
      $grpc.ServiceCall $call,
      $async.Future<$0.RevokeDeviceRequest> $request) async {
    return revokeDevice($call, await $request);
  }

  $async.Future<$0.RevokeDeviceResponse> revokeDevice(
      $grpc.ServiceCall call, $0.RevokeDeviceRequest request);

  $async.Future<$0.GetClipboardStatusResponse> getClipboardStatus_Pre(
      $grpc.ServiceCall $call,
      $async.Future<$0.GetClipboardStatusRequest> $request) async {
    return getClipboardStatus($call, await $request);
  }

  $async.Future<$0.GetClipboardStatusResponse> getClipboardStatus(
      $grpc.ServiceCall call, $0.GetClipboardStatusRequest request);

  $async.Future<$0.TriggerClipboardPullResponse> triggerClipboardPull_Pre(
      $grpc.ServiceCall $call,
      $async.Future<$0.TriggerClipboardPullRequest> $request) async {
    return triggerClipboardPull($call, await $request);
  }

  $async.Future<$0.TriggerClipboardPullResponse> triggerClipboardPull(
      $grpc.ServiceCall call, $0.TriggerClipboardPullRequest request);

  $async.Future<$0.SendFileResponse> sendFile_Pre($grpc.ServiceCall $call,
      $async.Future<$0.SendFileRequest> $request) async {
    return sendFile($call, await $request);
  }

  $async.Future<$0.SendFileResponse> sendFile(
      $grpc.ServiceCall call, $0.SendFileRequest request);

  $async.Future<$0.CancelTransferResponse> cancelTransfer_Pre(
      $grpc.ServiceCall $call,
      $async.Future<$0.CancelTransferRequest> $request) async {
    return cancelTransfer($call, await $request);
  }

  $async.Future<$0.CancelTransferResponse> cancelTransfer(
      $grpc.ServiceCall call, $0.CancelTransferRequest request);

  $async.Future<$0.ListTransfersResponse> listTransfers_Pre(
      $grpc.ServiceCall $call,
      $async.Future<$0.ListTransfersRequest> $request) async {
    return listTransfers($call, await $request);
  }

  $async.Future<$0.ListTransfersResponse> listTransfers(
      $grpc.ServiceCall call, $0.ListTransfersRequest request);

  $async.Stream<$0.StreamFramesResponse> streamFrames_Pre(
      $grpc.ServiceCall $call,
      $async.Future<$0.StreamFramesRequest> $request) async* {
    yield* streamFrames($call, await $request);
  }

  $async.Stream<$0.StreamFramesResponse> streamFrames(
      $grpc.ServiceCall call, $0.StreamFramesRequest request);

  $async.Future<$0.SendInputResponse> sendInput_Pre($grpc.ServiceCall $call,
      $async.Future<$0.SendInputRequest> $request) async {
    return sendInput($call, await $request);
  }

  $async.Future<$0.SendInputResponse> sendInput(
      $grpc.ServiceCall call, $0.SendInputRequest request);

  $async.Future<$0.ListNotificationsResponse> listNotifications_Pre(
      $grpc.ServiceCall $call,
      $async.Future<$0.ListNotificationsRequest> $request) async {
    return listNotifications($call, await $request);
  }

  $async.Future<$0.ListNotificationsResponse> listNotifications(
      $grpc.ServiceCall call, $0.ListNotificationsRequest request);
}
