// Signal-only event maps must never corrupt the capture snapshot.
//
// Found on hardware (POCO F1, Oct 2026): while streaming, the Activity feed
// showed repeated bogus "Screen Sharing Stopped / Session ended normally"
// entries although capture never stopped. Root cause: every raw event map is
// pushed through CaptureStats.fromMap, which defaulted a MISSING isCapturing
// key (and all counters) to zero/false instead of keeping the previous
// snapshot — so {"trustChanged": true}, {"pairingChanged": true} and
// transfer-only {"transfer": ...} maps flipped isSharing false, zeroed the
// counters and logged phantom stop/start transitions.
import 'package:flutter_test/flutter_test.dart';

import 'package:phonebridge_ui/controllers/phonebridge_controller.dart';
import 'package:phonebridge_ui/models/capture_stats.dart';

import 'support/fake_bridge_service.dart';

Map<dynamic, dynamic> fullStats() => <dynamic, dynamic>{
      'isCapturing': true,
      'encodedFrames': 100,
      'keyframes': 10,
      'admittedFrames': 95,
      'droppedFrames': 5,
      'codec': 'h264',
      'isHardwareCodec': true,
      'durationUs': 5000000,
      'timestampMs': 1000,
    };

void main() {
  test('fromMap keeps the previous snapshot for keys the event omits', () {
    const prev = CaptureStats(
      isCapturing: true,
      encodedFrames: 100,
      keyframes: 10,
      admittedFrames: 95,
      droppedFrames: 5,
      codec: 'h264',
      isHardwareCodec: true,
      durationUs: 5000000,
      timestampMs: 1000,
      currentFps: 12.5,
    );

    final next = CaptureStats.fromMap(
      const {'pairingChanged': true},
      previous: prev,
    );

    expect(next.isCapturing, isTrue, reason: 'capture flag must survive');
    expect(next.encodedFrames, 100);
    expect(next.codec, 'h264');
    expect(next.durationUs, 5000000);
    expect(next.timestampMs, 1000);
    expect(next.currentFps, 12.5, reason: 'fps must not blank on signal maps');
  });

  test('signal-only maps never flip sharing state or log phantom transitions',
      () async {
    final service = FakeBridgeService();
    final controller = PhoneBridgeController(service: service);
    addTearDown(controller.dispose);
    controller.initialize();
    // Let the initial refreshAll() settle: its getMediaStats answers with a
    // non-capturing snapshot, which must land BEFORE the live stats below.
    await pumpEventQueue();

    service.rawEvents.add(fullStats());
    await pumpEventQueue();
    expect(controller.isSharing, isTrue);
    final settledActivity = controller.activityEvents.length;
    expect(settledActivity, greaterThanOrEqualTo(1),
        reason: 'the real start transition is logged once');

    // Keyless maps ride _subscribeEvents with no accompanying refresh: the
    // snapshot must survive them untouched. (trustChanged is excluded here on
    // purpose — it triggers refreshAll(), whose live getMediaStats answers
    // with the real snapshot on device.)
    service.rawEvents.add(const {'pairingChanged': true});
    service.rawEvents.add(const {
      'transfer': {'transferId': 't1', 'state': 'ACTIVE'}
    });
    await pumpEventQueue();

    expect(controller.isSharing, isTrue,
        reason: 'signal maps must not end sharing');
    expect(controller.captureStats.encodedFrames, 100);
    expect(controller.captureStats.codec, 'h264');
    expect(
      controller.activityEvents.length,
      settledActivity,
      reason: 'no phantom stop/start transitions from signal maps',
    );
  });
}
