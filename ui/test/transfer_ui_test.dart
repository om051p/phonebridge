// File-transfer UI tests (DEC-024, Phase 4).
//
// Widget level: the transfers section states, live progress rendering, the
// send-file flow, and the integration into Activity/Home navigation.

import 'dart:async';

import 'package:flutter/material.dart' hide ClipboardStatus;
import 'package:flutter_test/flutter_test.dart';
import 'package:phonebridge_ui/controllers/phonebridge_controller.dart';
import 'package:phonebridge_ui/controllers/transfer_controller.dart';
import 'package:phonebridge_ui/generated/phonebridge/localipc/v1/local_ipc.pb.dart'
    as ipc;
import 'package:phonebridge_ui/models/activity_event.dart';
import 'package:phonebridge_ui/models/transfer_item.dart';
import 'package:phonebridge_ui/screens/activity_screen.dart';
import 'package:phonebridge_ui/screens/app_scaffold.dart';
import 'package:phonebridge_ui/ui/transfer_views.dart';

import 'support/fake_transfer_backend.dart';

/// One transfer snapshot, timestamped "now" so the relative timestamps in the
/// UI are deterministic ('Just now').
TransferItem item(
  String id, {
  required ipc.TransferState state,
  int bytes = 0,
  int size = 0,
  bool finished = false,
  bool inbound = false,
  ipc.TransferReason reason = ipc.TransferReason.TRANSFER_REASON_UNSPECIFIED,
}) {
  final start = DateTime.now().millisecondsSinceEpoch;
  return TransferItem(
    transferId: id,
    direction: inbound
        ? ipc.TransferDirection.TRANSFER_DIRECTION_INBOUND
        : ipc.TransferDirection.TRANSFER_DIRECTION_OUTBOUND,
    state: state,
    peerDeviceId: 'abcdef0123456789',
    filename: '$id.bin',
    sizeBytes: size,
    bytesTransferred: bytes,
    startedAtMs: start,
    finishedAtMs: finished ? start + 500 : 0,
    reasonCode: reason,
  );
}

Widget app(Widget child) {
  return MaterialApp(
    theme: ThemeData(useMaterial3: true),
    home: child is Scaffold ? child : Scaffold(body: child),
  );
}

/// Wraps a non-scrolling section in the scroll view a screen would provide.
Widget inline(Widget child) {
  return app(
    SingleChildScrollView(
      padding: const EdgeInsets.symmetric(vertical: 16),
      child: child,
    ),
  );
}

void main() {
  group('TransfersSection states', () {
    late FakeTransferBackend backend;
    late TransferController controller;

    setUp(() {
      backend = FakeTransferBackend();
      controller = TransferController(backend: backend);
    });

    tearDown(() async {
      controller.dispose();
      await backend.close();
    });

    testWidgets('empty history shows the empty state and the send action',
        (tester) async {
      await controller.hydrate();

      await tester.pumpWidget(inline(TransfersSection(controller: controller)));

      expect(find.text('File Transfers'), findsOneWidget);
      expect(find.text('No transfers yet'), findsOneWidget);
      expect(find.text('SEND FILE'), findsOneWidget);
      expect(find.byType(TransferTile), findsNothing);
    });

    testWidgets('an unsupported platform explains itself and offers no action',
        (tester) async {
      final unsupported = FakeTransferBackend(supportsFileTransfer: false);
      final plain = TransferController(backend: unsupported);
      addTearDown(() async {
        plain.dispose();
        await unsupported.close();
      });

      await plain.hydrate();
      await tester.pumpWidget(inline(TransfersSection(controller: plain)));

      expect(find.text(kTransferUnavailableMessage), findsOneWidget);
      expect(find.text('SEND FILE'), findsNothing);
      expect(find.text('No transfers yet'), findsNothing);
    });

    testWidgets('a failed history load shows the error state and retries',
        (tester) async {
      backend.listError =
          'dial unix /run/user/1000/phonebridge/engine.sock: connect: no such file';
      await controller.hydrate();

      await tester.pumpWidget(inline(TransfersSection(controller: controller)));

      expect(find.text('Transfer history unavailable'), findsOneWidget);
      expect(find.textContaining('no such file'), findsOneWidget);
      expect(backend.listCalls, 1);

      backend.listError = null;
      backend.history = [
        item('tx-1',
            state: ipc.TransferState.TRANSFER_STATE_ACTIVE, size: 8192),
      ];
      await tester.tap(find.text('RETRY'));
      await tester.pumpAndSettle();

      expect(controller.isBackendUnavailable, isFalse);
      expect(find.text('tx-1.bin'), findsOneWidget);
      expect(find.text('Transfer history unavailable'), findsNothing);
      expect(backend.listCalls, 2);
    });

    testWidgets('renders filename, direction, peer, state, bytes and progress',
        (tester) async {
      backend.history = [
        item(
          'tx-in',
          state: ipc.TransferState.TRANSFER_STATE_COMPLETE,
          bytes: 5 * 1024 * 1024,
          size: 5 * 1024 * 1024,
          finished: true,
          inbound: true,
        ),
        item(
          'tx-out',
          state: ipc.TransferState.TRANSFER_STATE_ACTIVE,
          bytes: 2048,
          size: 8192,
        ),
      ];
      await controller.hydrate();

      await tester.pumpWidget(inline(TransfersSection(controller: controller)));

      expect(find.text('tx-in.bin'), findsOneWidget);
      expect(find.text('tx-out.bin'), findsOneWidget);

      // Inbound completion: direction, state, peer and the final byte count.
      expect(find.textContaining('Received · Complete'), findsOneWidget);
      expect(find.textContaining('5.0 MB of 5.0 MB · 100%'), findsOneWidget);

      // Outbound in-flight: live byte progress and a bar at the right value.
      expect(find.textContaining('Sent · Transferring'), findsOneWidget);
      expect(find.textContaining('2.0 KB of 8.0 KB · 25%'), findsOneWidget);
      final bar = tester.widget<LinearProgressIndicator>(
        find.byType(LinearProgressIndicator),
      );
      expect(bar.value, closeTo(0.25, 1e-9));

      // Only the in-flight transfer is cancellable; both stamp "Just now".
      expect(find.byTooltip('Cancel transfer'), findsOneWidget);
      expect(find.text('Just now'), findsNWidgets(2));

      // Newest first, as the daemon orders its history: tx-in was listed first.
      final tiles =
          tester.widgetList<TransferTile>(find.byType(TransferTile)).toList();
      expect(
        tiles.map((tile) => tile.item.transferId).toList(),
        ['tx-in', 'tx-out'],
      );
    });

    testWidgets('a failed transfer shows its typed reason text',
        (tester) async {
      backend.history = [
        item(
          'tx-fail',
          state: ipc.TransferState.TRANSFER_STATE_FAILED,
          bytes: 10,
          size: 20,
          finished: true,
          reason: ipc.TransferReason.TRANSFER_REASON_CHECKSUM_MISMATCH,
        ),
      ];
      await controller.hydrate();

      await tester.pumpWidget(inline(TransfersSection(controller: controller)));

      expect(find.text('Integrity check failed'), findsOneWidget);
      expect(find.textContaining('Sent · Failed'), findsOneWidget);
      expect(find.byTooltip('Cancel transfer'), findsNothing);
    });

    testWidgets('cancelling disables the action until the daemon answers',
        (tester) async {
      final gate = Completer<bool>();
      backend.history = [
        item(
          'tx-1',
          state: ipc.TransferState.TRANSFER_STATE_ACTIVE,
          bytes: 1,
          size: 10,
        ),
      ];
      await controller.initialize();
      await tester.pumpWidget(inline(TransfersSection(controller: controller)));
      backend.cancelGate = gate;

      await tester.tap(find.byTooltip('Cancel transfer'));
      await tester.pump();

      expect(backend.cancelCalls, ['tx-1']);
      final button = tester.widget<IconButton>(
        find.widgetWithIcon(IconButton, Icons.close),
      );
      expect(button.onPressed, isNull, reason: 'cancel is in flight');

      // The terminal event ends it: the row reports Cancelled, no action left.
      gate.complete(true);
      backend.streamController.add(item(
        'tx-1',
        state: ipc.TransferState.TRANSFER_STATE_CANCELLED,
        bytes: 1,
        size: 10,
        finished: true,
        reason: ipc.TransferReason.TRANSFER_REASON_CANCELLED_BY_USER,
      ));
      await tester.pumpAndSettle();

      expect(find.byTooltip('Cancel transfer'), findsNothing);
      expect(find.text('Cancelled'), findsOneWidget);
    });

    testWidgets('a pushed progress event updates the row without polling',
        (tester) async {
      backend.history = [
        item(
          'tx-1',
          state: ipc.TransferState.TRANSFER_STATE_ACTIVE,
          bytes: 1024,
          size: 4096,
        ),
      ];
      await controller.initialize();
      await tester.pumpWidget(inline(TransfersSection(controller: controller)));
      expect(find.textContaining('1.0 KB of 4.0 KB'), findsOneWidget);

      backend.streamController.add(item(
        'tx-1',
        state: ipc.TransferState.TRANSFER_STATE_ACTIVE,
        bytes: 3072,
        size: 4096,
      ));
      await tester.pumpAndSettle();

      expect(find.textContaining('3.0 KB of 4.0 KB'), findsOneWidget);
      expect(find.textContaining('1.0 KB of 4.0 KB'), findsNothing);
      expect(find.byType(TransferTile), findsOneWidget);
      // One list RPC for the whole test: progress comes from the stream.
      expect(backend.listCalls, 1);
    });

    testWidgets('SEND FILE validates the path and sends the chosen file',
        (tester) async {
      await controller.hydrate();
      await tester.pumpWidget(inline(TransfersSection(controller: controller)));

      await tester.tap(find.text('SEND FILE'));
      await tester.pumpAndSettle();
      expect(find.text('Send File'), findsOneWidget);

      // Empty path: refused locally, nothing reaches the backend.
      await tester.tap(find.text('SEND'));
      await tester.pumpAndSettle();
      expect(find.text('Enter the file path to send'), findsOneWidget);
      expect(backend.sendCalls, isEmpty);

      // Relative path: refused locally too (the daemon needs an absolute path).
      await tester.enterText(find.byType(TextField), 'report.pdf');
      await tester.tap(find.text('SEND'));
      await tester.pumpAndSettle();
      expect(find.textContaining('Use an absolute path'), findsOneWidget);
      expect(backend.sendCalls, isEmpty);

      // Absolute path: sent, the row appears, the outcome is messaged.
      await tester.enterText(find.byType(TextField), '/home/you/report.pdf');
      await tester.tap(find.text('SEND'));
      await tester.pumpAndSettle();

      expect(backend.sendCalls.single['localPath'], '/home/you/report.pdf');
      expect(controller.items.single.filename, 'report.pdf');
      expect(find.text('report.pdf'), findsOneWidget);
      expect(find.textContaining('Sending report.pdf'), findsOneWidget);

      // Let the snackbar's dismissal timer elapse before the test ends.
      await tester.pump(const Duration(seconds: 4));
    });

    testWidgets('a refused send reports the typed reason in the snackbar',
        (tester) async {
      backend.sendResult = const TransferSendResult(
        reasonCode: ipc.TransferReason.TRANSFER_REASON_UNSUPPORTED_PEER,
      );
      await controller.hydrate();
      await tester.pumpWidget(inline(TransfersSection(controller: controller)));

      await tester.tap(find.text('SEND FILE'));
      await tester.pumpAndSettle();
      await tester.enterText(find.byType(TextField), '/home/you/report.pdf');
      await tester.tap(find.text('SEND'));
      await tester.pumpAndSettle();

      expect(
        find.textContaining('Peer does not support transfers'),
        findsOneWidget,
      );
      expect(controller.items, isEmpty);
      await tester.pump(const Duration(seconds: 4));
    });
  });

  group('Activity & Home integration', () {
    testWidgets('Activity hosts the transfers section above the event log',
        (tester) async {
      tester.view.physicalSize = const Size(1080, 2400);
      tester.view.devicePixelRatio = 1.0;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);

      final service = FakeBridgeService();
      service.transfers.history = [
        item(
          'tx-1',
          state: ipc.TransferState.TRANSFER_STATE_COMPLETE,
          bytes: 4096,
          size: 4096,
          finished: true,
        ),
      ];
      final controller = PhoneBridgeController(service: service);
      controller.initialize();
      controller.addActivityEvent(
        ActivityCategory.screen,
        'Screen Sharing Started',
        '720p @ 30fps',
        ActivityLevel.success,
      );

      await tester.pumpWidget(app(ActivityScreen(controller: controller)));
      await tester.pumpAndSettle();

      final section = find.byType(TransfersSection);
      expect(section, findsOneWidget);
      expect(
        find.descendant(of: section, matching: find.text('tx-1.bin')),
        findsOneWidget,
      );
      expect(
        find.descendant(of: section, matching: find.text('SEND FILE')),
        findsOneWidget,
      );

      // The activity log keeps working below the transfers section.
      expect(find.text('Screen Sharing Started'), findsOneWidget);
      expect(find.text('All'), findsOneWidget);
      expect(find.text('No Activity Recorded'), findsNothing);
    });

    testWidgets('navigating away and back keeps one row and one subscription',
        (tester) async {
      tester.view.physicalSize = const Size(600, 1000);
      tester.view.devicePixelRatio = 1.0;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);

      final service = FakeBridgeService();
      service.transfers.history = [
        item(
          'tx-1',
          state: ipc.TransferState.TRANSFER_STATE_ACTIVE,
          bytes: 1024,
          size: 4096,
        ),
      ];
      final controller = PhoneBridgeController(service: service);
      // AppScaffold.initialize() (from initState) is the single owner of the
      // transfer subscription, exactly as in the running app.
      await tester.pumpWidget(app(AppScaffold(controller: controller)));
      await tester.pumpAndSettle();
      expect(find.byType(NavigationBar), findsOneWidget);
      expect(service.transfers.listCalls, 1);

      await tester.tap(find.text('Activity'));
      await tester.pumpAndSettle();

      final section = find.byType(TransfersSection);
      expect(
        find.descendant(of: section, matching: find.byType(TransferTile)),
        findsOneWidget,
      );

      // Away and back twice: the IndexedStack keeps the screen alive, so this
      // must not re-subscribe or duplicate rows.
      for (var i = 0; i < 2; i++) {
        await tester.tap(find.text('Home'));
        await tester.pumpAndSettle();
        await tester.tap(find.text('Activity'));
        await tester.pumpAndSettle();
      }

      service.transfers.streamController.add(item(
        'tx-1',
        state: ipc.TransferState.TRANSFER_STATE_COMPLETE,
        bytes: 4096,
        size: 4096,
        finished: true,
      ));
      await tester.pumpAndSettle();

      expect(
        find.descendant(of: section, matching: find.byType(TransferTile)),
        findsOneWidget,
      );
      expect(
        find.descendant(of: section, matching: find.textContaining('Complete')),
        findsWidgets,
      );
      expect(controller.transfers.items, hasLength(1));
      // Exactly one history load for the whole lifetime: no re-subscription.
      expect(service.transfers.listCalls, 1);
    });

    testWidgets('wide layout reaches transfers from the navigation rail',
        (tester) async {
      tester.view.physicalSize = const Size(1280, 800);
      tester.view.devicePixelRatio = 1.0;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);

      final service = FakeBridgeService();
      service.transfers.history = [
        item(
          'tx-1',
          state: ipc.TransferState.TRANSFER_STATE_FAILED,
          finished: true,
          reason: ipc.TransferReason.TRANSFER_REASON_TOO_LARGE,
        ),
      ];
      final controller = PhoneBridgeController(service: service);

      await tester.pumpWidget(app(AppScaffold(controller: controller)));
      await tester.pumpAndSettle();
      expect(find.byType(NavigationRail), findsOneWidget);
      expect(find.byType(NavigationBar), findsNothing);

      await tester.tap(find.text('Activity'));
      await tester.pumpAndSettle();

      expect(
        find.descendant(
          of: find.byType(TransfersSection),
          matching: find.text('tx-1.bin'),
        ),
        findsOneWidget,
      );
      expect(find.text('File exceeds the size limit'), findsOneWidget);
    });

    testWidgets('Home summarises transfers and opens the Activity tab',
        (tester) async {
      tester.view.physicalSize = const Size(600, 1000);
      tester.view.devicePixelRatio = 1.0;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);

      final service = FakeBridgeService();
      final controller = PhoneBridgeController(service: service);

      await tester.pumpWidget(app(AppScaffold(controller: controller)));
      await tester.pumpAndSettle();

      final card = find.byType(TransferSummaryCard);
      expect(card, findsOneWidget);
      expect(
        find.descendant(of: card, matching: find.text('No transfers yet')),
        findsOneWidget,
      );

      // A pushed event surfaces on Home, progress bar included.
      service.transfers.streamController.add(item(
        'tx-1',
        state: ipc.TransferState.TRANSFER_STATE_ACTIVE,
        bytes: 2048,
        size: 8192,
      ));
      await tester.pumpAndSettle();

      expect(
        find.descendant(of: card, matching: find.textContaining('25%')),
        findsOneWidget,
      );
      expect(
        find.descendant(
          of: card,
          matching: find.byType(LinearProgressIndicator),
        ),
        findsOneWidget,
      );

      await tester.tap(find.descendant(of: card, matching: find.text('Open')));
      await tester.pumpAndSettle();

      expect(
        tester.widget<NavigationBar>(find.byType(NavigationBar)).selectedIndex,
        4,
      );
      expect(
        find.descendant(
          of: find.byType(TransfersSection),
          matching: find.text('tx-1.bin'),
        ),
        findsOneWidget,
      );
    });
  });
}
