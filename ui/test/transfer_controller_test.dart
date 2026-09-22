// TransferController + TransferItem mapping tests (DEC-024, Phase 4).
//
// Pure Dart (no widgets): hydration, event merge, duplicate suppression,
// cancel-state bookkeeping and the typed enum -> UI text mapping.

import 'dart:async';

import 'package:fixnum/fixnum.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:phonebridge_ui/controllers/transfer_controller.dart';
import 'package:phonebridge_ui/generated/phonebridge/localipc/v1/local_ipc.pb.dart'
    as ipc;
import 'package:phonebridge_ui/models/transfer_item.dart';

import 'support/fake_transfer_backend.dart';

const int _startedAt = 1700000000000;

/// Builds one transfer snapshot with only the fields a test cares about.
TransferItem item(
  String id, {
  required ipc.TransferState state,
  int bytes = 0,
  int size = 0,
  bool finished = false,
  bool inbound = false,
  ipc.TransferReason reason = ipc.TransferReason.TRANSFER_REASON_UNSPECIFIED,
}) {
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
    startedAtMs: _startedAt,
    finishedAtMs: finished ? _startedAt + 1000 : 0,
    reasonCode: reason,
  );
}

void main() {
  group('TransferItem mapping', () {
    test('wraps a proto snapshot and formats bytes and progress', () {
      final proto = ipc.TransferInfo(
        transferId: 'tx-1',
        direction: ipc.TransferDirection.TRANSFER_DIRECTION_OUTBOUND,
        state: ipc.TransferState.TRANSFER_STATE_ACTIVE,
        peerDeviceId: 'abcdef0123456789',
        filename: 'report.pdf',
        sizeBytes: Int64(4 * 1024 * 1024),
        bytesTransferred: Int64(1024 * 1024),
        startedAtMs: Int64(_startedAt),
        reasonCode: ipc.TransferReason.TRANSFER_REASON_NONE,
      );

      final wrapped = TransferItem.fromProto(proto);
      expect(wrapped.transferId, 'tx-1');
      expect(wrapped.isOutbound, isTrue);
      expect(wrapped.isInbound, isFalse);
      expect(wrapped.directionLabel, 'Sent');
      expect(wrapped.stateLabel, 'Transferring');
      expect(wrapped.displayName, 'report.pdf');
      expect(wrapped.progress, closeTo(0.25, 1e-9));
      expect(wrapped.progressPercent, 25);
      expect(wrapped.progressLabel, '1.0 MB of 4.0 MB');
      expect(wrapped.sizeLabel, '4.0 MB');
      expect(wrapped.peerLabel, 'abcdef012345…');
      expect(wrapped.reasonText, isNull);
      expect(wrapped.timeAgo, isNot('—'));
    });

    test('an inbound offer without a size shows bytes only, never a bar', () {
      const offer = TransferItem(
        transferId: 'tx-offer',
        state: ipc.TransferState.TRANSFER_STATE_PENDING,
        bytesTransferred: 2048,
      );
      expect(offer.hasSize, isFalse);
      expect(offer.progress, isNull);
      expect(offer.progressPercent, 0);
      expect(offer.progressLabel, '2.0 KB');
      expect(offer.directionLabel, 'Transfer');
    });

    test('every state maps to UI text and cancellability follows DEC-024', () {
      String label(ipc.TransferState state) =>
          TransferItem(transferId: 't', state: state).stateLabel;
      bool cancellable(ipc.TransferState state) =>
          TransferItem(transferId: 't', state: state).isCancellable;

      expect(label(ipc.TransferState.TRANSFER_STATE_UNSPECIFIED), 'Unknown');
      expect(
        label(ipc.TransferState.TRANSFER_STATE_PENDING),
        'Waiting for peer',
      );
      expect(label(ipc.TransferState.TRANSFER_STATE_ACTIVE), 'Transferring');
      expect(label(ipc.TransferState.TRANSFER_STATE_VERIFYING), 'Verifying…');
      expect(label(ipc.TransferState.TRANSFER_STATE_COMPLETE), 'Complete');
      expect(label(ipc.TransferState.TRANSFER_STATE_CANCELLED), 'Cancelled');
      expect(label(ipc.TransferState.TRANSFER_STATE_FAILED), 'Failed');

      expect(cancellable(ipc.TransferState.TRANSFER_STATE_PENDING), isTrue);
      expect(cancellable(ipc.TransferState.TRANSFER_STATE_ACTIVE), isTrue);
      // Committed to staging on the receiver: a local cancel would race it.
      expect(cancellable(ipc.TransferState.TRANSFER_STATE_VERIFYING), isFalse);
      expect(cancellable(ipc.TransferState.TRANSFER_STATE_COMPLETE), isFalse);
      expect(cancellable(ipc.TransferState.TRANSFER_STATE_CANCELLED), isFalse);
      expect(cancellable(ipc.TransferState.TRANSFER_STATE_FAILED), isFalse);

      expect(const TransferItem(transferId: 't').isTerminal, isFalse);
      expect(
        const TransferItem(
          transferId: 't',
          state: ipc.TransferState.TRANSFER_STATE_FAILED,
        ).isTerminal,
        isTrue,
      );
    });

    test('maps every typed reason code to its UI text', () {
      final expected = <ipc.TransferReason, String>{
        ipc.TransferReason.TRANSFER_REASON_NO_SESSION: 'No active session',
        ipc.TransferReason.TRANSFER_REASON_UNSUPPORTED_PEER:
            'Peer does not support transfers',
        ipc.TransferReason.TRANSFER_REASON_BUSY:
            'Another transfer is in flight',
        ipc.TransferReason.TRANSFER_REASON_UNSAFE_FILENAME: 'Unsafe filename',
        ipc.TransferReason.TRANSFER_REASON_TOO_LARGE:
            'File exceeds the size limit',
        ipc.TransferReason.TRANSFER_REASON_CHECKSUM_MISMATCH:
            'Integrity check failed',
        ipc.TransferReason.TRANSFER_REASON_STORAGE_FAILED: 'Storage error',
        ipc.TransferReason.TRANSFER_REASON_INTERRUPTED:
            'Connection interrupted',
        ipc.TransferReason.TRANSFER_REASON_CANCELLED_BY_PEER:
            'Cancelled by peer',
        ipc.TransferReason.TRANSFER_REASON_CANCELLED_BY_USER: 'Cancelled',
        ipc.TransferReason.TRANSFER_REASON_PROTOCOL_ERROR: 'Protocol error',
        ipc.TransferReason.TRANSFER_REASON_INCOMPATIBLE_VERSION:
            'Incompatible device',
      };

      expected.forEach((reason, text) {
        expect(TransferItem.reasonTextFor(reason), text, reason: reason.name);
      });
      expect(
        TransferItem.reasonTextFor(ipc.TransferReason.TRANSFER_REASON_NONE),
        isNull,
      );
      expect(
        TransferItem.reasonTextFor(
          ipc.TransferReason.TRANSFER_REASON_UNSPECIFIED,
        ),
        isNull,
      );
    });

    test('a failed transfer with no classification still reports failure', () {
      final unclassified = item(
        'tx-fail',
        state: ipc.TransferState.TRANSFER_STATE_FAILED,
        finished: true,
      );
      expect(unclassified.reasonText, 'Transfer failed');

      final classified = item(
        'tx-fail-2',
        state: ipc.TransferState.TRANSFER_STATE_FAILED,
        reason: ipc.TransferReason.TRANSFER_REASON_CHECKSUM_MISMATCH,
        finished: true,
      );
      expect(classified.reasonText, 'Integrity check failed');
    });

    test('exposes the receiver-side saved name when it differs', () {
      const renamed = TransferItem(
        transferId: 'tx-rename',
        state: ipc.TransferState.TRANSFER_STATE_COMPLETE,
        filename: 'report.pdf',
        savedName: 'report (1).pdf',
      );
      expect(renamed.savedAsLabel, 'Saved as report (1).pdf');

      const unchanged = TransferItem(
        transferId: 'tx-same',
        state: ipc.TransferState.TRANSFER_STATE_COMPLETE,
        filename: 'report.pdf',
        savedName: 'report.pdf',
      );
      expect(unchanged.savedAsLabel, isNull);
    });

    test('parses channel maps with proto names or enum indices', () {
      final byName = TransferItem.fromMap(<dynamic, dynamic>{
        'transferId': 'tx-map',
        'direction': 'INBOUND',
        'state': 'COMPLETE',
        'reasonCode': 'CANCELLED_BY_PEER',
        'peerDeviceId': 'peer-1',
        'filename': 'photo.jpg',
        'sizeBytes': 3072,
        'bytesTransferred': 3072,
      });
      expect(byName.isInbound, isTrue);
      expect(byName.isComplete, isTrue);
      expect(byName.reasonText, 'Cancelled by peer');
      expect(byName.displayName, 'photo.jpg');
      expect(byName.sizeLabel, '3.0 KB');

      final byIndex = TransferItem.fromMap(<dynamic, dynamic>{
        'transferId': 'tx-index',
        'state': 2,
        'direction': 2,
      });
      expect(byIndex.state, ipc.TransferState.TRANSFER_STATE_ACTIVE);
      expect(
        byIndex.direction,
        ipc.TransferDirection.TRANSFER_DIRECTION_INBOUND,
      );
    });

    test('classifies send results from the typed reason', () {
      const accepted = TransferSendResult(transferId: 'tx-1');
      expect(accepted.isOk, isTrue);

      const busy = TransferSendResult(
        reasonCode: ipc.TransferReason.TRANSFER_REASON_BUSY,
      );
      expect(busy.isOk, isFalse);
      expect(busy.errorText, 'Another transfer is in flight');

      final transport = TransferSendResult.failure('connection refused');
      expect(transport.isOk, isFalse);
      expect(transport.errorText, 'connection refused');

      const empty = TransferSendResult();
      expect(empty.errorText, 'Transfer could not be started');
    });
  });

  group('TransferController', () {
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

    test('hydrate loads the history newest first', () async {
      backend.history = [
        item('tx-new',
            state: ipc.TransferState.TRANSFER_STATE_COMPLETE, finished: true),
        item('tx-old',
            state: ipc.TransferState.TRANSFER_STATE_COMPLETE, finished: true),
      ];

      await controller.hydrate();

      expect(
        controller.items.map((t) => t.transferId).toList(),
        ['tx-new', 'tx-old'],
      );
      expect(controller.isHydrated, isTrue);
      expect(controller.isLoading, isFalse);
      expect(controller.isBackendUnavailable, isFalse);
      expect(controller.hasTransfers, isTrue);
    });

    test('hydrating twice replaces nothing and duplicates nothing', () async {
      backend.history = [
        item('tx-1',
            state: ipc.TransferState.TRANSFER_STATE_COMPLETE, finished: true),
      ];

      await controller.hydrate();
      await controller.hydrate();

      expect(controller.items, hasLength(1));
      expect(backend.listCalls, 2);
    });

    test('hydrate records a typed error state and recovers on retry', () async {
      backend.listError = TimeoutException('token file missing');

      await controller.hydrate();

      expect(controller.isBackendUnavailable, isTrue);
      expect(controller.errorMessage, contains('token file missing'));
      expect(controller.items, isEmpty);
      expect(controller.isLoading, isFalse);

      backend.listError = null;
      backend.history = [
        item('tx-1', state: ipc.TransferState.TRANSFER_STATE_ACTIVE),
      ];
      await controller.hydrate();

      expect(controller.isBackendUnavailable, isFalse);
      expect(controller.items, hasLength(1));
    });

    test('pushed events insert new transfers at the front', () {
      controller.applyEvent(
        item('tx-1', state: ipc.TransferState.TRANSFER_STATE_PENDING),
      );
      controller.applyEvent(
        item('tx-2', state: ipc.TransferState.TRANSFER_STATE_PENDING),
      );

      expect(
        controller.items.map((t) => t.transferId).toList(),
        ['tx-2', 'tx-1'],
      );
    });

    test('a duplicate event is suppressed without notifying', () {
      var notifications = 0;
      controller.addListener(() => notifications++);
      final active = item(
        'tx-1',
        state: ipc.TransferState.TRANSFER_STATE_ACTIVE,
        bytes: 1024,
        size: 4096,
      );

      expect(controller.applyEvent(active), isTrue);
      expect(controller.applyEvent(active), isFalse);
      expect(notifications, 1);
      expect(controller.items, hasLength(1));
    });

    test('progress events update the row in place and keep the order', () {
      controller.applyEvent(item('tx-1',
          state: ipc.TransferState.TRANSFER_STATE_ACTIVE,
          bytes: 1024,
          size: 4096));
      controller.applyEvent(item('tx-2',
          state: ipc.TransferState.TRANSFER_STATE_ACTIVE,
          bytes: 0,
          size: 4096));
      controller.applyEvent(item('tx-1',
          state: ipc.TransferState.TRANSFER_STATE_ACTIVE,
          bytes: 3072,
          size: 4096));

      expect(
        controller.items.map((t) => t.transferId).toList(),
        ['tx-2', 'tx-1'],
      );
      expect(controller.items.last.bytesTransferred, 3072);
    });

    test('hydration never downgrades a finished transfer', () async {
      controller.applyEvent(item('tx-1',
          state: ipc.TransferState.TRANSFER_STATE_COMPLETE,
          bytes: 4096,
          size: 4096,
          finished: true));
      backend.history = [
        item('tx-1',
            state: ipc.TransferState.TRANSFER_STATE_ACTIVE,
            bytes: 1024,
            size: 4096),
      ];

      await controller.hydrate();

      expect(controller.items.single.isComplete, isTrue);
      expect(controller.items.single.bytesTransferred, 4096);
    });

    test('initialize subscribes so later events keep streaming in', () async {
      backend.history = [
        item('tx-1',
            state: ipc.TransferState.TRANSFER_STATE_PENDING, size: 4096),
      ];

      await controller.initialize();
      backend.streamController.add(item('tx-1',
          state: ipc.TransferState.TRANSFER_STATE_COMPLETE,
          bytes: 4096,
          size: 4096,
          finished: true));
      await Future<void>.delayed(Duration.zero);

      expect(controller.items.single.isComplete, isTrue);
      expect(controller.items.single.bytesTransferred, 4096);
    });

    test('counts in-flight transfers and exposes the latest one', () {
      controller.applyEvent(item('tx-a',
          state: ipc.TransferState.TRANSFER_STATE_ACTIVE, bytes: 1, size: 2));
      controller.applyEvent(item('tx-b',
          state: ipc.TransferState.TRANSFER_STATE_COMPLETE,
          bytes: 2,
          size: 2,
          finished: true));

      expect(controller.activeCount, 1);
      expect(controller.hasActiveTransfers, isTrue);
      expect(controller.latest!.transferId, 'tx-b');
    });

    test('cancel stays marked in flight until the daemon answers', () async {
      final gate = Completer<bool>();
      backend.cancelGate = gate;
      controller.applyEvent(item('tx-1',
          state: ipc.TransferState.TRANSFER_STATE_ACTIVE, bytes: 1, size: 10));

      final pending = controller.cancelTransfer('tx-1');
      expect(controller.isCancelling('tx-1'), isTrue);
      expect(controller.canCancel(controller.items.single), isFalse);

      // A second tap while the first is unanswered must not send another RPC.
      expect(await controller.cancelTransfer('tx-1'), isFalse);

      gate.complete(true);
      expect(await pending, isTrue);
      expect(backend.cancelCalls, ['tx-1']);
      // Still marked: the terminal CANCELLED event is what clears it.
      expect(controller.isCancelling('tx-1'), isTrue);

      controller.applyEvent(item('tx-1',
          state: ipc.TransferState.TRANSFER_STATE_CANCELLED,
          bytes: 1,
          size: 10,
          finished: true));
      expect(controller.isCancelling('tx-1'), isFalse);
    });

    test('a refused cancel releases the button for another attempt', () async {
      backend.cancelResult = false;
      controller.applyEvent(item('tx-1',
          state: ipc.TransferState.TRANSFER_STATE_ACTIVE, bytes: 1, size: 10));

      expect(await controller.cancelTransfer('tx-1'), isFalse);
      expect(controller.isCancelling('tx-1'), isFalse);
      expect(controller.canCancel(controller.items.single), isTrue);
      expect(backend.cancelCalls, ['tx-1']);
    });

    test('cancel is refused for verifying, terminal and unknown transfers',
        () async {
      controller.applyEvent(item('tx-verifying',
          state: ipc.TransferState.TRANSFER_STATE_VERIFYING, size: 10));
      controller.applyEvent(item('tx-done',
          state: ipc.TransferState.TRANSFER_STATE_COMPLETE,
          bytes: 10,
          size: 10,
          finished: true));

      expect(await controller.cancelTransfer('tx-verifying'), isFalse);
      expect(await controller.cancelTransfer('tx-done'), isFalse);
      expect(await controller.cancelTransfer('tx-unknown'), isFalse);
      expect(backend.cancelCalls, isEmpty);
    });

    test('sendFile surfaces a pending outbound row on success', () async {
      backend.sendResult = const TransferSendResult(
        transferId: 'tx-sent',
        state: ipc.TransferState.TRANSFER_STATE_PENDING,
        reasonCode: ipc.TransferReason.TRANSFER_REASON_NONE,
      );

      final result = await controller.sendFile(
          localPath: '/home/you/Documents/report.pdf');

      expect(result.isOk, isTrue);
      expect(backend.sendCalls.single['localPath'],
          '/home/you/Documents/report.pdf');
      expect(controller.lastSendError, isNull);
      expect(controller.isSending, isFalse);

      final row = controller.items.single;
      expect(row.transferId, 'tx-sent');
      expect(row.isOutbound, isTrue);
      expect(row.filename, 'report.pdf'); // basename fallback from the path
      expect(row.state, ipc.TransferState.TRANSFER_STATE_PENDING);
      expect(row.peerLabel, 'Unknown device');
    });

    test('a pushed PENDING event wins over the optimistic row', () async {
      controller.applyEvent(item('tx-sent',
          state: ipc.TransferState.TRANSFER_STATE_PENDING, size: 4096));
      backend.sendResult = const TransferSendResult(
          transferId: 'tx-sent',
          state: ipc.TransferState.TRANSFER_STATE_PENDING);

      await controller.sendFile(localPath: '/tmp/a.bin');

      expect(controller.items, hasLength(1));
      expect(controller.items.single.sizeBytes, 4096);
    });

    test('sendFile reports the typed reason and adds no row on failure',
        () async {
      backend.sendResult = const TransferSendResult(
        reasonCode: ipc.TransferReason.TRANSFER_REASON_NO_SESSION,
      );

      final result = await controller.sendFile(localPath: '/tmp/a.bin');

      expect(result.isOk, isFalse);
      expect(result.errorText, 'No active session');
      expect(controller.items, isEmpty);
      expect(controller.lastSendError, 'No active session');
    });

    test('sendFile validates an empty path before touching the backend',
        () async {
      final result = await controller.sendFile(localPath: '   ');

      expect(result.isOk, isFalse);
      expect(result.errorText, 'Choose a file to send');
      expect(backend.sendCalls, isEmpty);
    });

    test('sendFile reports a transport exception as a send error', () async {
      backend.sendError = StateError('grpc unavailable');

      final result = await controller.sendFile(localPath: '/tmp/a.bin');

      expect(result.isOk, isFalse);
      expect(result.errorText, contains('grpc unavailable'));
      expect(controller.lastSendError, contains('grpc unavailable'));
    });

    test('an unsupported backend never calls the daemon and stays quiet',
        () async {
      final unsupportedBackend =
          FakeTransferBackend(supportsFileTransfer: false);
      final plain = TransferController(backend: unsupportedBackend);
      addTearDown(() async {
        plain.dispose();
        await unsupportedBackend.close();
      });

      expect(plain.supportsFileTransfer, isFalse);

      await plain.initialize();
      expect(unsupportedBackend.listCalls, 0);
      expect(plain.isBackendUnavailable, isFalse);
      expect(plain.items, isEmpty);

      final result = await plain.sendFile(localPath: '/tmp/a.bin');
      expect(result.isOk, isFalse);
      expect(result.errorText, kTransferUnavailableMessage);
      expect(unsupportedBackend.sendCalls, isEmpty);

      // The Android events channel is silent by design; an error there must not
      // flip the section into its error state.
      unsupportedBackend.streamController.addError(StateError('no channel'));
      await Future<void>.delayed(Duration.zero);
      expect(plain.isBackendUnavailable, isFalse);
    });

    test('dispose cancels the subscription and tolerates late events',
        () async {
      final owned = TransferController(backend: backend);
      await owned.initialize();
      expect(owned.isHydrated, isTrue);

      owned.dispose();
      backend.streamController.add(
        item('tx-late', state: ipc.TransferState.TRANSFER_STATE_ACTIVE),
      );
      await Future<void>.delayed(Duration.zero);

      // Nothing was applied after dispose, and no "used after dispose" throw.
      expect(owned.items, isEmpty);
    });
  });
}
