// This is a generated file - do not edit.
//
// Generated from phonebridge/spike/localipc/v1/local_ipc.proto.

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

/// LocalIpcSpikeService is the spike's stand-in for the future local IPC
/// service. Only transport-relevant shapes are modelled.
@$pb.GrpcServiceName('phonebridge.spike.localipc.v1.LocalIpcSpikeService')
class LocalIpcSpikeServiceClient extends $grpc.Client {
  /// The hostname for this service.
  static const $core.String defaultHost = '';

  /// OAuth scopes needed for the client.
  static const $core.List<$core.String> oauthScopes = [
    '',
  ];

  LocalIpcSpikeServiceClient(super.channel,
      {super.options, super.interceptors});

  /// Ping is the unary round-trip used for feasibility + latency measurement.
  $grpc.ResponseFuture<$0.PingResponse> ping(
    $0.PingRequest request, {
    $grpc.CallOptions? options,
  }) {
    return $createUnaryCall(_$ping, request, options: options);
  }

  /// Subscribe is a server-streaming RPC used to measure Go -> Flutter push
  /// (the direction FFI cannot provide without a Dart API DL callback shim).
  $grpc.ResponseStream<$0.SubscribeResponse> subscribe(
    $0.SubscribeRequest request, {
    $grpc.CallOptions? options,
  }) {
    return $createStreamingCall(
        _$subscribe, $async.Stream.fromIterable([request]),
        options: options);
  }

  /// WhoAmI returns the server's view of the calling process (SO_PEERCRED),
  /// which is the basis of the same-user trust boundary.
  $grpc.ResponseFuture<$0.WhoAmIResponse> whoAmI(
    $0.WhoAmIRequest request, {
    $grpc.CallOptions? options,
  }) {
    return $createUnaryCall(_$whoAmI, request, options: options);
  }

  // method descriptors

  static final _$ping = $grpc.ClientMethod<$0.PingRequest, $0.PingResponse>(
      '/phonebridge.spike.localipc.v1.LocalIpcSpikeService/Ping',
      ($0.PingRequest value) => value.writeToBuffer(),
      $0.PingResponse.fromBuffer);
  static final _$subscribe =
      $grpc.ClientMethod<$0.SubscribeRequest, $0.SubscribeResponse>(
          '/phonebridge.spike.localipc.v1.LocalIpcSpikeService/Subscribe',
          ($0.SubscribeRequest value) => value.writeToBuffer(),
          $0.SubscribeResponse.fromBuffer);
  static final _$whoAmI =
      $grpc.ClientMethod<$0.WhoAmIRequest, $0.WhoAmIResponse>(
          '/phonebridge.spike.localipc.v1.LocalIpcSpikeService/WhoAmI',
          ($0.WhoAmIRequest value) => value.writeToBuffer(),
          $0.WhoAmIResponse.fromBuffer);
}

@$pb.GrpcServiceName('phonebridge.spike.localipc.v1.LocalIpcSpikeService')
abstract class LocalIpcSpikeServiceBase extends $grpc.Service {
  $core.String get $name =>
      'phonebridge.spike.localipc.v1.LocalIpcSpikeService';

  LocalIpcSpikeServiceBase() {
    $addMethod($grpc.ServiceMethod<$0.PingRequest, $0.PingResponse>(
        'Ping',
        ping_Pre,
        false,
        false,
        ($core.List<$core.int> value) => $0.PingRequest.fromBuffer(value),
        ($0.PingResponse value) => value.writeToBuffer()));
    $addMethod($grpc.ServiceMethod<$0.SubscribeRequest, $0.SubscribeResponse>(
        'Subscribe',
        subscribe_Pre,
        false,
        true,
        ($core.List<$core.int> value) => $0.SubscribeRequest.fromBuffer(value),
        ($0.SubscribeResponse value) => value.writeToBuffer()));
    $addMethod($grpc.ServiceMethod<$0.WhoAmIRequest, $0.WhoAmIResponse>(
        'WhoAmI',
        whoAmI_Pre,
        false,
        false,
        ($core.List<$core.int> value) => $0.WhoAmIRequest.fromBuffer(value),
        ($0.WhoAmIResponse value) => value.writeToBuffer()));
  }

  $async.Future<$0.PingResponse> ping_Pre(
      $grpc.ServiceCall $call, $async.Future<$0.PingRequest> $request) async {
    return ping($call, await $request);
  }

  $async.Future<$0.PingResponse> ping(
      $grpc.ServiceCall call, $0.PingRequest request);

  $async.Stream<$0.SubscribeResponse> subscribe_Pre($grpc.ServiceCall $call,
      $async.Future<$0.SubscribeRequest> $request) async* {
    yield* subscribe($call, await $request);
  }

  $async.Stream<$0.SubscribeResponse> subscribe(
      $grpc.ServiceCall call, $0.SubscribeRequest request);

  $async.Future<$0.WhoAmIResponse> whoAmI_Pre(
      $grpc.ServiceCall $call, $async.Future<$0.WhoAmIRequest> $request) async {
    return whoAmI($call, await $request);
  }

  $async.Future<$0.WhoAmIResponse> whoAmI(
      $grpc.ServiceCall call, $0.WhoAmIRequest request);
}
