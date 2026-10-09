// Regression: the Android connection status must reflect the phone's live
// session, not the always-idle "unsupported" session model.
//
// Found on hardware (Pixel 4 XL / POCO F1, Oct 2026): Android has no local
// session state machine (AndroidBridgeService does not implement SessionBackend),
// so composing LinkStatus from the session controller made the app-bar badge and
// the Screen tab read "Ready"/"No session running"/"Disconnected" even while the
// phone was capturing and streaming. Android must compose from capture telemetry.
import 'package:flutter_test/flutter_test.dart';

import 'package:phonebridge_ui/controllers/phonebridge_controller.dart';
import 'package:phonebridge_ui/models/link_status.dart';

import 'support/fake_bridge_service.dart';

Map<dynamic, dynamic> capturingStats({bool capturing = true}) =>
    <dynamic, dynamic>{
      'isCapturing': capturing,
      'encodedFrames': 42,
      'keyframes': 4,
      'timestampMs': 2000,
    };

void main() {
  test('Android link status follows capture telemetry (not the idle session)',
      () async {
    final service = FakeBridgeService();
    final controller = PhoneBridgeController(service: service);
    addTearDown(controller.dispose);
    controller.initialize();
    await pumpEventQueue();

    // Not capturing: idle/ready.
    expect(controller.linkStatus.phase, LinkPhase.disconnected);
    expect(controller.linkStatus.hasSession, isFalse);

    // Capturing: the phone has a live session and the badge must say so.
    service.rawEvents.add(capturingStats());
    await pumpEventQueue();

    expect(controller.session.supportsSessions, isFalse,
        reason: 'Android still has no local session state machine');
    expect(controller.linkStatus.phase, LinkPhase.streaming);
    expect(controller.linkStatus.hasSession, isTrue);
    expect(controller.linkStatus.label, 'Sharing');

    // Capture stops: back to ready.
    service.rawEvents.add(capturingStats(capturing: false));
    await pumpEventQueue();
    expect(controller.linkStatus.phase, LinkPhase.disconnected);
    expect(controller.linkStatus.hasSession, isFalse);
  });

  // The Devices screen picks "SWITCH TO THIS DEVICE" (and routes through
  // switchToDevice) exactly when linkStatus.hasSession is true. Reading the raw
  // session here would always be false on Android, so a live session would be
  // started cold and refused as SESSION_BUSY instead of switched.
  test('Devices-screen switch predicate (linkStatus.hasSession) tracks the Android session',
      () async {
    final service = FakeBridgeService();
    final controller = PhoneBridgeController(service: service);
    addTearDown(controller.dispose);
    controller.initialize();
    await pumpEventQueue();

    // Idle: the screen offers CONNECT / START SESSION.
    expect(controller.linkStatus.hasSession, isFalse);

    // Live capture: the screen must offer SWITCH instead.
    service.rawEvents.add(capturingStats());
    await pumpEventQueue();
    expect(controller.linkStatus.hasSession, isTrue);
  });

  test('Android link status follows inbound connected session from PC',
      () async {
    final service = FakeBridgeService();
    final controller = PhoneBridgeController(service: service);
    addTearDown(controller.dispose);
    controller.initialize();
    await pumpEventQueue();

    expect(controller.linkStatus.phase, LinkPhase.disconnected);
    expect(controller.linkStatus.hasSession, isFalse);
    expect(controller.activeDeviceId, isNull);

    // PC connects to phone (not capturing, but session established):
    service.rawEvents.add(<dynamic, dynamic>{
      'isSessionActive': true,
      'activeSessionPeerId': 'pc-device-42',
      'isCapturing': false,
      'timestampMs': 3000,
    });
    await pumpEventQueue();

    expect(controller.linkStatus.phase, LinkPhase.connected);
    expect(controller.linkStatus.hasSession, isTrue);
    expect(controller.linkStatus.label, 'Connected');
    expect(controller.activeDeviceId, 'pc-device-42');

    // PC disconnects:
    service.rawEvents.add(<dynamic, dynamic>{
      'isSessionActive': false,
      'activeSessionPeerId': '',
      'isCapturing': false,
      'timestampMs': 4000,
    });
    await pumpEventQueue();

    expect(controller.linkStatus.phase, LinkPhase.disconnected);
    expect(controller.linkStatus.hasSession, isFalse);
    expect(controller.activeDeviceId, isNull);
  });
}
