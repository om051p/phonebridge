// Android control-channel pass-through for file transfers (DEC-024, Phase 4).
//
// GoBridge.transferSend/List exist in Kotlin but MainActivity does not forward
// them yet, so what matters on the Flutter side is graceful degradation: a
// method the native side does not answer must never throw into the UI, and the
// map shape a future handler returns must already be understood.

import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:phonebridge_ui/generated/phonebridge/localipc/v1/local_ipc.pb.dart'
    as ipc;
import 'package:phonebridge_ui/models/transfer_item.dart';
import 'package:phonebridge_ui/services/android_bridge_service.dart';
import 'package:phonebridge_ui/services/phonebridge_channel.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  const MethodChannel control = MethodChannel('dev.phonebridge/control');
  final messenger =
      TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;

  tearDown(() => messenger.setMockMethodCallHandler(control, null));

  test('an unwired channel degrades instead of throwing', () async {
    // No native handler answers at all — the state of the channel today.
    messenger.setMockMethodCallHandler(control, (call) async => null);

    final service = AndroidBridgeService(channel: PhoneBridgeChannel());

    expect(service.supportsFileTransfer, isFalse);
    expect(await service.listTransfers(), isEmpty);
    expect(await service.cancelTransfer('tx-1'), isFalse);

    final result = await service.sendFile(localPath: '/sdcard/a.bin');
    expect(result.isOk, isFalse);
    expect(result.errorText, kTransferUnavailableMessage);
  });

  test('a wired channel maps transfer maps into the UI model', () async {
    messenger.setMockMethodCallHandler(control, (call) async {
      switch (call.method) {
        case 'listTransfers':
          return <Map<String, Object?>>[
            <String, Object?>{
              'transferId': 'tx-1',
              'direction': 'INBOUND',
              'state': 'COMPLETE',
              'peerDeviceId': 'peer-1',
              'filename': 'photo.jpg',
              'sizeBytes': 2048,
              'bytesTransferred': 2048,
            },
          ];
        case 'sendFile':
          return <String, Object?>{
            'transferId': 'tx-2',
            'state': 'PENDING',
            'reasonCode': 'NONE',
          };
        case 'cancelTransfer':
          return true;
        default:
          return null;
      }
    });

    final service = AndroidBridgeService(channel: PhoneBridgeChannel());

    final history = await service.listTransfers();
    expect(history, hasLength(1));
    expect(history.single.transferId, 'tx-1');
    expect(history.single.isInbound, isTrue);
    expect(history.single.isComplete, isTrue);
    expect(history.single.sizeLabel, '2.0 KB');

    final sent = await service.sendFile(localPath: '/sdcard/a.bin');
    expect(sent.isOk, isTrue);
    expect(sent.transferId, 'tx-2');
    expect(sent.state, ipc.TransferState.TRANSFER_STATE_PENDING);

    expect(await service.cancelTransfer('tx-1'), isTrue);
  });

  test('a PlatformException becomes a typed send failure', () async {
    messenger.setMockMethodCallHandler(control, (call) async {
      throw PlatformException(
        code: 'unavailable',
        message: 'native transfer channel not wired',
      );
    });

    final service = AndroidBridgeService(channel: PhoneBridgeChannel());

    final result = await service.sendFile(localPath: '/sdcard/a.bin');
    expect(result.isOk, isFalse);
    expect(result.errorText, contains('not wired'));
    expect(await service.listTransfers(), isEmpty);
    expect(await service.cancelTransfer('tx-1'), isFalse);
  });
}
