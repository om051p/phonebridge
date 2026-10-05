// Event fan-out regression tests (reliability hardening phase).
//
// The native events channel has exactly one EventSink. Before the fan-out
// fix, every stream getter called receiveBroadcastStream() again, so each
// subscription replaced the binary-message handler: the last subscriber won
// delivery and the first cancellation killed it for everyone. Measured on
// device: session/stats events stopped reaching the UI while a transfer
// subscription was live, leaving "READY — No session running" on a streaming
// session. These tests pin the corrected contract: ONE native subscription,
// every consumer gets its own copy, typed streams filter their own events,
// and disposal of one consumer never disturbs the others.
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:phonebridge_ui/models/capture_stats.dart';
import 'package:phonebridge_ui/services/phonebridge_channel.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  const events = EventChannel('dev.phonebridge/events');
  final messenger = TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;

  MockStreamHandlerEventSink? nativeSink;
  var nativeCancels = 0;

  setUp(() {
    nativeSink = null;
    nativeCancels = 0;
    messenger.setMockStreamHandler(
      events,
      MockStreamHandler.inline(
        onListen: (arguments, sink) => nativeSink = sink,
        onCancel: (arguments) {
          nativeCancels++;
          nativeSink = null;
        },
      ),
    );
  });

  tearDown(() {
    messenger.setMockStreamHandler(events, null);
  });

  void emit(Map<dynamic, dynamic> event) {
    final sink = nativeSink;
    if (sink == null) throw StateError('native subscription not open');
    sink.success(event);
  }

  Map<dynamic, dynamic> transferEvent(String id) => <dynamic, dynamic>{
        'transfer': <dynamic, dynamic>{
          'transferId': id,
          'state': 'COMPLETE',
          'filename': 'f.bin',
          'sizeBytes': 8,
          'bytesTransferred': 8,
        },
      };

  test('stats and raw session events reach the UI while a transfer subscription is live',
      () async {
    final channel = PhoneBridgeChannel();
    final raw = <Map<dynamic, dynamic>>[];
    final rawSub = channel.rawEventsStream.listen(raw.add);
    await pumpEventQueue();
    expect(nativeSink, isNotNull, reason: 'the native subscription must be open');

    final transfers = <Object>[];
    final transferSub = channel.transferStream.listen(transfers.add);
    await pumpEventQueue();

    emit(<dynamic, dynamic>{'isCapturing': true, 'encodedFrames': 5, 'timestampMs': 10});
    await pumpEventQueue();

    expect(transfers, isEmpty, reason: 'a stats event must not consume the transfer stream');
    expect(raw, isNotEmpty, reason: 'session/stats events must reach the UI consumer');
    expect(raw.last['isCapturing'], true);

    await transferSub.cancel();
    await rawSub.cancel();
  });

  test('transfer events reach the transfer consumer without displacing other listeners',
      () async {
    final channel = PhoneBridgeChannel();
    final raw = <Map<dynamic, dynamic>>[];
    final rawSub = channel.rawEventsStream.listen(raw.add);
    final transfers = <Object>[];
    final transferSub = channel.transferStream.listen(transfers.add);
    final stats = <CaptureStats>[];
    final statsSub = channel.statsStream.listen(stats.add);
    await pumpEventQueue();

    emit(transferEvent('t1'));
    await pumpEventQueue();

    expect(transfers.length, 1, reason: 'the transfer consumer must see its event');
    expect(raw.length, 1, reason: 'the raw consumer must still be receiving');
    // A transfer-only event carries no stats keys: the typed stats stream
    // re-merges its snapshot unchanged rather than inventing a state change.
    expect(stats.single.isCapturing, false);

    emit(transferEvent('t1'));
    await pumpEventQueue();
    expect(transfers.length, 2);

    await statsSub.cancel();
    await transferSub.cancel();
    await rawSub.cancel();
  });

  test('cancelling one consumer never starves the others', () async {
    final channel = PhoneBridgeChannel();
    final raw = <Map<dynamic, dynamic>>[];
    final rawSub = channel.rawEventsStream.listen(raw.add);
    final transfers = <Object>[];
    final transferSub = channel.transferStream.listen(transfers.add);
    await pumpEventQueue();

    // The controller that subscribed first is disposed (app surface closed).
    await rawSub.cancel();
    await pumpEventQueue();

    emit(transferEvent('t2'));
    await pumpEventQueue();

    expect(transfers.length, 1,
        reason: 'the surviving consumer must keep receiving after another cancels');
    expect(nativeCancels, 0,
        reason: 'the native subscription must stay open while any consumer listens');

    await transferSub.cancel();
    await pumpEventQueue();
    expect(nativeCancels, 1,
        reason: 'the native subscription closes only after the last consumer leaves');
  });

  test('every consumer receives each event exactly once', () async {
    final channel = PhoneBridgeChannel();
    final seen = <Map<dynamic, dynamic>>[];
    final a = channel.rawEventsStream.listen(seen.add);
    final b = channel.rawEventsStream.listen(seen.add);
    await pumpEventQueue();

    emit(<dynamic, dynamic>{'isCapturing': false});
    emit(<dynamic, dynamic>{'isCapturing': true});
    await pumpEventQueue();

    expect(seen.length, 4, reason: 'two consumers x two events, no duplicates');
    expect(seen.where((e) => e['isCapturing'] == true).length, 2);

    await a.cancel();
    await b.cancel();
  });

  test('subscription is re-openable: a late consumer still gets a live channel',
      () async {
    final channel = PhoneBridgeChannel();
    final first = channel.rawEventsStream.listen((_) {});
    await pumpEventQueue();
    await first.cancel();
    await pumpEventQueue();
    expect(nativeCancels, 1);

    final late = <Map<dynamic, dynamic>>[];
    final second = channel.rawEventsStream.listen(late.add);
    await pumpEventQueue();
    expect(nativeSink, isNotNull, reason: 'a later consumer must re-open the channel');

    emit(<dynamic, dynamic>{'pairingChanged': true});
    await pumpEventQueue();
    expect(late, isNotEmpty);

    await second.cancel();
  });
}
