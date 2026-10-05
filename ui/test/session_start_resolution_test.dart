// BUG-1 regression: bare START must resolve the peer's signaling endpoint
// from canonical discovery state — a deviceId must never reach the Android
// channel as receiverUrl (Kotlin normaliseEndpoint rejects it, so the
// consent UI appears yet no session can ever establish).
//
// Contract under test (Android):
//   explicit receiverUrl  -> passed through verbatim (Kotlin validates);
//   device id (arg or active peer) -> resolved via discoveredDevices;
//   unresolvable -> clean abort: no capture, truthful error.

import 'package:fixnum/fixnum.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:phonebridge_ui/controllers/phonebridge_controller.dart';
import 'package:phonebridge_ui/generated/phonebridge/localipc/v1/local_ipc.pb.dart'
    as ipc;
import 'package:phonebridge_ui/models/discovered_device.dart';
import 'package:phonebridge_ui/models/trusted_device.dart';
import 'package:phonebridge_ui/services/linux_bridge_service.dart';

import 'support/fake_bridge_service.dart';
import 'support/fake_ipc_client.dart';

const _peerId = 'aa67e88a629d12dd914f64da225b74946cd66ce7fd6970ebc0423a00679f2b8a';
const _peerUrl = 'http://192.168.0.150:7804';

class _RecordingBridge extends FakeBridgeService {
  List<DiscoveredDevice> discovered = const [];
  final List<String?> captureUrls = [];

  @override
  Future<List<DiscoveredDevice>> listDiscoveredDevices() async =>
      List<DiscoveredDevice>.of(discovered);

  @override
  Future<bool> startCapture({
    String? receiverUrl,
    int width = 720,
    int height = 1600,
    int fps = 30,
    int bitrateKbps = 2500,
  }) async {
    captureUrls.add(receiverUrl);
    return true;
  }
}

const _trustedPeer = TrustedDevice(
  deviceId: _peerId,
  displayName: 'POCOPHONE F1',
  platform: 'android',
  pairedAtMs: 1,
  lastSeenMs: 2,
  revoked: false,
);

const _discoveredPeer = DiscoveredDevice(
  id: _peerId,
  name: 'POCOPHONE F1',
  model: 'POCOPHONE F1',
  version: '1',
  host: '192.168.0.150',
  port: 7804,
);

PhoneBridgeController _controller(_RecordingBridge bridge) {
  final controller = PhoneBridgeController(service: bridge);
  addTearDown(controller.dispose);
  return controller;
}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  test('explicit target resolves the discovered signaling endpoint',
      () async {
    final bridge = _RecordingBridge()
      ..trustedDevices = [_trustedPeer]
      ..discovered = [_discoveredPeer];
    final controller = _controller(bridge);
    await controller.refreshAll();
    await controller.refreshDiscoveredDevices();

    expect(
        await controller.startScreenSharing(targetDeviceId: _peerId), isTrue);
    expect(bridge.captureUrls, hasLength(1));
    expect(bridge.captureUrls.single, _peerUrl);
  });

  test('bare START without a target fails instead of guessing (Phase C)',
      () async {
    final bridge = _RecordingBridge()
      ..trustedDevices = [_trustedPeer]
      ..discovered = [_discoveredPeer];
    final controller = _controller(bridge);
    await controller.refreshAll();
    await controller.refreshDiscoveredDevices();

    expect(await controller.startScreenSharing(), isFalse);
    expect(bridge.captureUrls, isEmpty);
    expect(controller.lastErrorMessage, contains('Select a device'));
  });

  test('deviceId arg is resolved through discovery, never passed as URL',
      () async {
    final bridge = _RecordingBridge()
      ..trustedDevices = [_trustedPeer]
      ..discovered = [_discoveredPeer];
    final controller = _controller(bridge);
    await controller.refreshAll();
    await controller.refreshDiscoveredDevices();

    expect(
        await controller.startScreenSharing(targetDeviceId: _peerId), isTrue);
    expect(bridge.captureUrls.single, _peerUrl);
  });

  test('trusted but undiscovered peer aborts cleanly without capture',
      () async {
    final bridge = _RecordingBridge()
      ..trustedDevices = [_trustedPeer]
      ..discovered = const [];
    final controller = _controller(bridge);
    await controller.refreshAll();
    await controller.refreshDiscoveredDevices();

    expect(await controller.startScreenSharing(), isFalse);
    expect(bridge.captureUrls, isEmpty);
    expect(controller.lastErrorMessage, contains('reachable'));
  });

  test('stale discovery entry aborts cleanly without capture', () async {
    const stale = DiscoveredDevice(
      id: _peerId,
      name: 'POCOPHONE F1',
      model: 'x',
      version: '1',
      host: '192.168.0.150',
      port: 7804,
      isStale: true,
    );
    final bridge = _RecordingBridge()
      ..trustedDevices = [_trustedPeer]
      ..discovered = [stale];
    final controller = _controller(bridge);
    await controller.refreshAll();
    await controller.refreshDiscoveredDevices();

    expect(await controller.startScreenSharing(), isFalse);
    expect(bridge.captureUrls, isEmpty);
  });

  test('explicit receiver URL passes through verbatim', () async {
    final bridge = _RecordingBridge()
      ..trustedDevices = [_trustedPeer]
      ..discovered = [_discoveredPeer];
    final controller = _controller(bridge);
    await controller.refreshAll();
    await controller.refreshDiscoveredDevices();

    expect(
        await controller.startScreenSharing(receiverUrl: _peerUrl), isTrue);
    expect(bridge.captureUrls.single, _peerUrl);
  });

  test('no known peer aborts without capture', () async {
    final bridge = _RecordingBridge();
    final controller = _controller(bridge);
    await controller.refreshAll();
    await controller.refreshDiscoveredDevices();

    expect(await controller.startScreenSharing(), isFalse);
    expect(bridge.captureUrls, isEmpty);
    expect(controller.lastErrorMessage, isNotNull);
  });

  test('Linux START requires an explicit device id (Phase C)', () async {
    final client = FakeIpcClient()
      ..trustedDevices = [
        ipc.TrustedDevice(
          deviceId: _peerId,
          displayName: 'POCOPHONE F1',
          platform: 'android',
          pairedAtMs: Int64(1),
          lastSeenMs: Int64(2),
        ),
      ];
    final controller = PhoneBridgeController(
      service: LinuxBridgeService(client: client),
    );
    addTearDown(controller.dispose);
    addTearDown(client.events.close);
    controller.initialize();
    await controller.refreshAll();

    // No target, no manual override: fails before any RPC (no peer-auto).
    expect(await controller.startScreenSharing(), isFalse);
    expect(client.startCalls, isEmpty);

    // Explicit target keeps the device-id contract (daemon resolves).
    expect(
        await controller.startScreenSharing(targetDeviceId: _peerId), isTrue);
    expect(client.startCalls.single.target, _peerId);
  });
}
