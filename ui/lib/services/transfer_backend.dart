import '../models/transfer_item.dart';

/// The narrow transfer surface the UI depends on.
///
/// [PlatformBridgeService] implements this interface, but keeping it separate
/// lets the transfer controller and its tests depend on four members instead of
/// the whole platform bridge. Backends never throw for ordinary failure paths
/// they can classify: `sendFile` returns a typed [TransferSendResult] and
/// `cancelTransfer` returns false. Only [listTransfers] reports failure by
/// throwing, because "empty history" and "backend unavailable" must stay
/// distinguishable in the UI.
abstract class TransferBackend {
  /// False when the platform cannot send or list files yet (native channel not
  /// wired). The UI then shows an explanatory state instead of a dead button.
  bool get supportsFileTransfer;

  /// Pushed transfer transitions (every state/progress change).
  Stream<TransferItem> get transferStream;

  /// In-flight transfers plus the recent in-memory history, newest first.
  Future<List<TransferItem>> listTransfers();

  /// Offers a local file to the active peer. An empty [deviceId] means "the
  /// active session's peer" (the daemon rejects it when no session is active).
  Future<TransferSendResult> sendFile({
    required String localPath,
    String filename = '',
    String deviceId = '',
  });

  /// Aborts an in-flight transfer; returns false when the daemon refused.
  Future<bool> cancelTransfer(String transferId);
}
