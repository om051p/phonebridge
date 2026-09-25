// The unified status model is only worth having if the screens actually render
// it. These tests drive the real controller over a fake bridge and assert what a
// person sees in the app bar and on the session card.

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:phonebridge_ui/controllers/phonebridge_controller.dart';
import 'package:phonebridge_ui/generated/phonebridge/localipc/v1/local_ipc.pb.dart' as ipc;
import 'package:phonebridge_ui/models/transfer_item.dart';
import 'package:phonebridge_ui/screens/app_scaffold.dart';

import 'support/fake_transfer_backend.dart';

void main() {
  late FakeBridgeService service;
  late PhoneBridgeController controller;

  setUp(() {
    service = FakeBridgeService();
    controller = PhoneBridgeController(service: service);
  });

  // Disposal happens inside each test body rather than in tearDown: an active
  // session starts the 1 s telemetry poll, and flutter_test asserts that no
  // timer is pending before tearDown callbacks run.
  tearDown(() {
    controller.dispose();
  });

  Future<void> pumpApp(WidgetTester tester) async {
    await tester.pumpWidget(
      MaterialApp(home: AppScaffold(controller: controller)),
    );
    await tester.pump();
  }

  /// Pushes one session transition. Two pumps: the first delivers the stream
  /// event (it arrives in a microtask), the second rebuilds with the new status.
  Future<void> push(WidgetTester tester, ipc.SessionEvent event) async {
    service.sessions.events.add(event);
    await tester.pump();
    await tester.pump();
  }

  /// Opens the Screen tab. IndexedStack builds every tab, but non-selected ones
  /// are offstage and finders skip offstage widgets by default.
  Future<void> openScreenTab(WidgetTester tester) async {
    await tester.tap(find.text('Screen'));
    await tester.pump();
  }

  /// The app-bar indicator. Deliberately scoped: the same label also appears on
  /// the Home connection card, and both must agree — so a bare text finder would
  /// be asserting the wrong thing twice.
  Finder badge(String text) => find.descendant(
        of: find.byType(AppBar),
        matching: find.text(text),
      );

  testWidgets('the badge reports the composed connection phase', (tester) async {
    await pumpApp(tester);

    // No session and a reachable backend: ready, not sharing.
    expect(badge('Ready'), findsOneWidget);
    expect(badge('Sharing'), findsNothing);

    await push(
      tester,
      ipc.SessionEvent(
        sessionId: 'sess-1',
        state: ipc.SessionState.SESSION_STATE_STREAMING,
      ),
    );
    expect(badge('Sharing'), findsOneWidget);
    expect(badge('Ready'), findsNothing);

    await push(
      tester,
      ipc.SessionEvent(
        sessionId: 'sess-1',
        state: ipc.SessionState.SESSION_STATE_RECONNECTING,
        reason: 'transport lost',
      ),
    );
    expect(badge('Reconnecting'), findsOneWidget);

    await push(
      tester,
      ipc.SessionEvent(
        sessionId: 'sess-1',
        state: ipc.SessionState.SESSION_STATE_FAILED,
        reasonCode: ipc.SessionReason.SESSION_REASON_RECONNECT_TIMEOUT,
        errorMessage: 'reconnect budget 15s exhausted after 10 attempt(s)',
      ),
    );
    expect(badge('Session failed'), findsOneWidget);

    controller.dispose();
  });

  testWidgets('the session card explains a failure instead of hiding it',
      (tester) async {
    await pumpApp(tester);

    await push(
      tester,
      ipc.SessionEvent(
        sessionId: 'sess-1',
        state: ipc.SessionState.SESSION_STATE_FAILED,
        reasonCode: ipc.SessionReason.SESSION_REASON_RECONNECT_TIMEOUT,
      ),
    );
    await openScreenTab(tester);

    // Badge word, card heading and the actionable line all come from the model.
    expect(badge('Session failed'), findsOneWidget);
    expect(find.text('SESSION FAILED'), findsOneWidget);
    expect(
      find.textContaining('did not recover within the reconnect window'),
      findsWidgets,
    );

    controller.dispose();
  });

  testWidgets('the session card shows recovery as recovery, not as an error',
      (tester) async {
    await pumpApp(tester);

    await push(
      tester,
      ipc.SessionEvent(
        sessionId: 'sess-1',
        state: ipc.SessionState.SESSION_STATE_CONNECTED,
      ),
    );
    await push(
      tester,
      ipc.SessionEvent(
        sessionId: 'sess-1',
        state: ipc.SessionState.SESSION_STATE_RECONNECTING,
      ),
    );
    await openScreenTab(tester);

    expect(find.text('RECONNECTING'), findsOneWidget);
    expect(badge('Reconnecting'), findsOneWidget);
    // Recovery is not a failure: no actionable error line is shown.
    expect(find.text('SESSION FAILED'), findsNothing);

    controller.dispose();
  });

  testWidgets('an in-flight transfer is reported beside the connection, never '
      'as the connection', (tester) async {
    await pumpApp(tester);

    await push(
      tester,
      ipc.SessionEvent(
        sessionId: 'sess-1',
        state: ipc.SessionState.SESSION_STATE_STREAMING,
      ),
    );

    service.transfers.streamController.add(
      const TransferItem(
        transferId: 'tx-1',
        state: ipc.TransferState.TRANSFER_STATE_ACTIVE,
        filename: 'holiday.mp4',
      ),
    );
    await tester.pump();
    await tester.pump();

    // The badge still describes the session; the transfer gets its own line.
    expect(badge('Sharing'), findsOneWidget);
    expect(find.textContaining('1 transfer in progress'), findsWidgets);

    controller.dispose();
  });
}
