// Android LAN discovery + clipboard control channel (DEC-007 browse-on-Android).
//
// The phone is the browsing half of the pair: MainActivity answers
// `getDiscoveredDevices` from the Go core's mDNS browse, and the Devices tab can
// only offer a PC once those rows are dialable. These tests pin the contract
// that made the PC invisible before: a real browse is forwarded, undialable
// rows are dropped rather than rendered, and the clipboard master switch
// actually reaches the native adapter instead of only flipping a Dart flag.

import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:phonebridge_ui/controllers/phonebridge_controller.dart';
import 'package:phonebridge_ui/models/discovered_device.dart';
import 'package:phonebridge_ui/services/android_bridge_service.dart';
import 'package:phonebridge_ui/services/phonebridge_channel.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  const MethodChannel control = MethodChannel('dev.phonebridge/control');
  final messenger =
      TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;

  tearDown(() => messenger.setMockMethodCallHandler(control, null));

  test('an unwired channel lists no devices instead of throwing', () async {
    messenger.setMockMethodCallHandler(control, (call) async => null);

    final service = AndroidBridgeService(channel: PhoneBridgeChannel());

    expect(await service.listDiscoveredDevices(), isEmpty);
  });

  test('discovered peers become dialable devices with a signaling URL', () async {
    messenger.setMockMethodCallHandler(control, (call) async {
      switch (call.method) {
        case 'getDiscoveredDevices':
          return <Map<String, Object?>>[
            <String, Object?>{
              'id': 'aa67e88a629d12dd',
              'name': 'rp-x1-desktop',
              'model': 'Linux',
              'version': '1',
              'host': '192.168.0.236',
              'port': 7804,
              'is_stale': false,
            },
          ];
        default:
          return null;
      }
    });

    final service = AndroidBridgeService(channel: PhoneBridgeChannel());
    final devices = await service.listDiscoveredDevices();

    expect(devices, hasLength(1));
    expect(devices.single.name, 'rp-x1-desktop');
    expect(devices.single.signalingUrl, 'http://192.168.0.236:7804');
  });

  test('undialable browse rows are dropped, not rendered', () async {
    // A vanished record can report no host and port 0. Offering Connect on such
    // a row could only fail, so the service filters it out up front.
    messenger.setMockMethodCallHandler(control, (call) async {
      return <Map<String, Object?>>[
        <String, Object?>{'id': 'aged-out', 'name': 'Gone', 'host': '', 'port': 0},
        <String, Object?>{'id': '', 'name': 'Nameless', 'host': '10.0.0.5', 'port': 9000},
        <String, Object?>{
          'id': 'live',
          'name': 'Desktop',
          'host': '10.0.0.6',
          'port': 7804,
        },
      ];
    });

    final service = AndroidBridgeService(channel: PhoneBridgeChannel());
    final devices = await service.listDiscoveredDevices();

    expect(devices.map((d) => d.id), <String>['live']);
    expect(devices.single.signalingUrl, 'http://10.0.0.6:7804');
  });

  test('a nameless peer falls back to its id so the row is still labelled',
      () async {
    final device = DiscoveredDevice.fromMap(<String, Object?>{
      'id': 'abc123',
      'name': '',
      'host': '10.0.0.7',
      'port': 7804,
    });

    expect(device.name, 'abc123');
  });

  test('the clipboard master switch reaches the native adapter', () async {
    final sent = <Map<Object?, Object?>?>[];
    var nativeEnabled = true;
    messenger.setMockMethodCallHandler(control, (call) async {
      switch (call.method) {
        case 'setClipboardSyncEnabled':
          sent.add(call.arguments as Map<Object?, Object?>?);
          nativeEnabled = (call.arguments as Map<Object?, Object?>)['enabled'] as bool;
          return true;
        default:
          return null;
      }
    });

    final service = AndroidBridgeService(channel: PhoneBridgeChannel());
    final controller = PhoneBridgeController(service: service);

    await controller.setClipboardEnabled(false);

    expect(sent, hasLength(1));
    expect(sent.single, <Object?, Object?>{'enabled': false});
    expect(nativeEnabled, isFalse);
    expect(controller.clipboardStatus.isEnabled, isFalse);
  });

  test('a refused clipboard toggle leaves the UI flag alone', () async {
    messenger.setMockMethodCallHandler(control, (call) async {
      // The native adapter could not apply the change.
      return false;
    });

    final service = AndroidBridgeService(channel: PhoneBridgeChannel());
    final controller = PhoneBridgeController(service: service);

    await controller.setClipboardEnabled(false);

    expect(controller.clipboardStatus.isEnabled, isTrue);
    expect(controller.lastErrorMessage, isNotNull);
  });

  test('the companion IME affordances report whether they opened', () async {
    final opened = <String>[];
    messenger.setMockMethodCallHandler(control, (call) async {
      opened.add(call.method);
      return call.method == 'openInputMethodSettings';
    });

    final service = AndroidBridgeService(channel: PhoneBridgeChannel());

    expect(await service.openInputMethodSettings(), isTrue);
    expect(await service.showInputMethodPicker(), isFalse);
    expect(opened, <String>['openInputMethodSettings', 'showInputMethodPicker']);
  });
}
