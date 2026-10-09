import 'dart:async';
import 'dart:collection';
import 'dart:io';

import 'package:flutter/foundation.dart';

import '../generated/phonebridge/localipc/v1/local_ipc.pb.dart' as ipc;
import '../models/transfer_item.dart';
import '../services/transfer_backend.dart';

/// Owns the transfer list the UI renders: history hydration from ListTransfers
/// merged with the live TransferEvent stream (no polling).
///
/// Merge rules, in one place so the widget tests can hold them:
///  * a transfer is keyed by transfer_id; new ids are inserted at the front
///    (newest first), known ids are updated in place so rows never jump,
///  * an identical event is dropped instead of notifying listeners,
///  * a stale snapshot (hydration) never downgrades a terminal transfer back to
///    in flight — the terminal state is the end of the state machine,
///  * a cancel request is recorded per transfer so the button disables until the
///    daemon answers or a terminal event arrives.
///
/// The controller never disposes the backend: the service is shared with the
/// rest of the app and is owned by [PhoneBridgeController].
class TransferController extends ChangeNotifier {
  TransferController({required TransferBackend backend}) : _backend = backend;

  final TransferBackend _backend;
  final LinkedHashMap<String, TransferItem> _items =
      LinkedHashMap<String, TransferItem>();
  final Set<String> _cancelRequested = <String>{};

  StreamSubscription<TransferItem>? _streamSub;
  bool _isLoading = false;
  bool _hydrated = false;
  bool _isSending = false;
  bool _disposed = false;
  String? _errorMessage;
  String? _lastSendError;

  /// Newest first.
  List<TransferItem> get items => List.unmodifiable(_items.values);

  bool get isLoading => _isLoading;

  /// False when even the history could not be read (daemon down, transfer engine
  /// disabled). Distinguishes the error state from the empty state.
  bool get isBackendUnavailable => _errorMessage != null;

  String? get errorMessage => _errorMessage;

  bool get supportsFileTransfer => _backend.supportsFileTransfer;

  bool get hasTransfers => _items.isNotEmpty;

  bool get hasActiveTransfers => _items.values.any((item) => item.isInFlight);

  bool get hasFinishedTransfers => _items.values.any((item) => item.isTerminal);

  int get activeCount => _items.values.where((item) => item.isInFlight).length;

  /// Most recent transfer, whatever its state — the Home summary uses it.
  TransferItem? get latest => _items.isEmpty ? null : _items.values.first;

  /// True while the daemon has not answered a cancel request for this transfer.
  bool isCancelling(String transferId) => _cancelRequested.contains(transferId);

  /// Cancel is offered only while it can still do something: PENDING/ACTIVE and
  /// no request already in flight.
  bool canCancel(TransferItem item) =>
      item.isCancellable && !_cancelRequested.contains(item.transferId);

  /// Subscribes to live events and loads the history once.
  Future<void> initialize() async {
    _subscribe();
    await hydrate();
  }

  void _subscribe() {
    _streamSub?.cancel();
    _streamSub = _backend.transferStream.listen(
      applyEvent,
      onError: (Object error) {
        // Without transfer support the stream is silent by design (the Android
        // events channel is not wired), so only real backends report errors.
        if (!_backend.supportsFileTransfer) return;
        _errorMessage = error.toString();
        _notify();
      },
    );
  }

  /// Reads in-flight transfers and the recent history from the daemon and merges
  /// them into the list. Safe to call again (refresh/retry).
  Future<void> hydrate() async {
    if (!_backend.supportsFileTransfer) {
      _hydrated = true;
      return;
    }
    _isLoading = true;
    _notify();
    try {
      final snapshot = await _backend.listTransfers();
      _applySnapshot(snapshot);
      _errorMessage = null;
      _hydrated = true;
    } catch (error) {
      _errorMessage = error.toString();
    } finally {
      _isLoading = false;
      _notify();
    }
  }

  /// Merges one pushed transfer transition. Returns true when the list changed.
  bool applyEvent(TransferItem item) {
    if (item.transferId.isEmpty) return false;
    final existing = _items[item.transferId];
    if (existing != null && !item.isUpdateOver(existing)) {
      return false;
    }
    if (existing == null) {
      _insertFront(item);
    } else {
      // In place: keeps the newest-first ordering stable while bytes flow.
      _items[item.transferId] = item;
    }
    if (item.isTerminal) {
      _cancelRequested.remove(item.transferId);
    }
    _notify();
    return true;
  }

  void _applySnapshot(List<TransferItem> snapshot) {
    // The daemon lists newest first. Walking the snapshot backwards means each
    // front-insert lands the newest transfer on top, so a hydrated list has the
    // same ordering as one built from pushed events.
    for (final item in snapshot.reversed) {
      if (item.transferId.isEmpty) continue;
      final existing = _items[item.transferId];
      if (existing == null) {
        _insertFront(item);
        continue;
      }
      // A snapshot is only a baseline: never resurrect a finished transfer.
      if (existing.isTerminal && !item.isTerminal) continue;
      if (!item.isUpdateOver(existing)) continue;
      _items[item.transferId] = item;
    }
  }

  void _insertFront(TransferItem item) {
    // A map literal is insertion-ordered (LinkedHashMap), which is exactly the
    // newest-first contract of _items.
    final reordered = <String, TransferItem>{item.transferId: item}
      ..addAll(_items);
    _items
      ..clear()
      ..addAll(reordered);
  }

  /// True once the first history load has completed (successfully or not).
  bool get isHydrated => _hydrated;

  /// True while a send request is in flight.
  bool get isSending => _isSending;

  /// Last send/cancel failure text, for an inline notice in the section.
  String? get lastSendError => _lastSendError;

  /// Offers a local file to the active peer. Returns the typed daemon result so
  /// the caller can report the classified reason (no session, peer unsupported,
  /// busy, too large, …) instead of a generic failure.
  Future<TransferSendResult> sendFile({
    required String localPath,
    String filename = '',
    String deviceId = '',
  }) async {
    final path = localPath.trim();
    if (path.isEmpty) {
      return TransferSendResult.failure('Choose a file to send');
    }
    if (!_backend.supportsFileTransfer) {
      return TransferSendResult.failure(kTransferUnavailableMessage);
    }

    _isSending = true;
    _lastSendError = null;
    _notify();
    try {
      final result = await _backend.sendFile(
        localPath: path,
        filename: filename.trim(),
        deviceId: deviceId.trim(),
      );
      if (result.isOk) {
        _insertPendingFor(result, path, filename);
      } else {
        _lastSendError = result.errorText;
      }
      return result;
    } catch (error) {
      _lastSendError = error.toString();
      return TransferSendResult.failure(error.toString());
    } finally {
      _isSending = false;
      _notify();
    }
  }

  /// Requests cancellation for one transfer. Returns false when there is nothing
  /// to cancel, or when the daemon refused — the pending mark is then released
  /// so the cancel button becomes available again.
  Future<bool> cancelTransfer(String transferId) async {
    final item = _items[transferId];
    if (item == null || !canCancel(item)) return false;

    _cancelRequested.add(transferId);
    _notify();
    try {
      final ok = await _backend.cancelTransfer(transferId);
      if (!ok) {
        _cancelRequested.remove(transferId);
        _notify();
      }
      return ok;
    } catch (error) {
      _cancelRequested.remove(transferId);
      _lastSendError = error.toString();
      _notify();
      return false;
    }
  }

  /// Surfaces the new outbound transfer immediately, so the row appears even if
  /// the pushed PENDING event races the RPC response (the event wins if it
  /// arrives first: a known id is never re-inserted).
  void _insertPendingFor(
    TransferSendResult result,
    String localPath,
    String filename,
  ) {
    if (_items.containsKey(result.transferId)) return;
    final state = result.state == ipc.TransferState.TRANSFER_STATE_UNSPECIFIED
        ? ipc.TransferState.TRANSFER_STATE_PENDING
        : result.state;
    _insertFront(TransferItem(
      transferId: result.transferId,
      direction: ipc.TransferDirection.TRANSFER_DIRECTION_OUTBOUND,
      state: state,
      filename: filename.isNotEmpty ? filename : _basename(localPath),
      startedAtMs: DateTime.now().millisecondsSinceEpoch,
      reasonCode: ipc.TransferReason.TRANSFER_REASON_NONE,
    ));
  }

  static String _basename(String path) {
    final normalized = path.replaceAll('\\', '/');
    final parts = normalized.split('/').where((part) => part.isNotEmpty);
    return parts.isEmpty ? path : parts.last;
  }

  /// Clears a recorded backend error (the retry affordance).
  void clearError() {
    if (_errorMessage == null) return;
    _errorMessage = null;
    _notify();
  }

  /// Removes all completed, cancelled, or failed transfers from the in-memory list.
  void clearFinished() {
    final toRemove = _items.entries
        .where((e) => e.value.isTerminal)
        .map((e) => e.key)
        .toList();
    if (toRemove.isEmpty) return;
    for (final id in toRemove) {
      _items.remove(id);
    }
    _notify();
  }

  /// Opens the containing directory of a received file in the desktop file manager.
  Future<bool> openContainingFolder(String filename) async {
    try {
      if (kIsWeb) return false;
      final home = Platform.environment['HOME'] ?? '';
      final xdgDownload = Platform.environment['XDG_DOWNLOAD_DIR'];
      final downloadDir = (xdgDownload != null && xdgDownload.isNotEmpty)
          ? xdgDownload
          : '$home/Downloads';
      final file = File('$downloadDir/$filename');
      final target = await file.exists() ? file.path : downloadDir;
      final result = await Process.run('xdg-open', [target]);
      return result.exitCode == 0;
    } catch (_) {
      return false;
    }
  }

  /// Interactive native desktop file picker using zenity or kdialog.
  Future<String?> pickDesktopFilePath({String title = 'Select file to send'}) async {
    return pickNativeFile(title: title);
  }

  /// Static helper for picking files on Linux via native dialogs.
  static Future<String?> pickNativeFile({String title = 'Select file to send'}) async {
    try {
      if (kIsWeb) return null;
      if (!Platform.isLinux) return null;

      // 1. Try zenity (standard on GNOME / Ubuntu / Debian)
      final zenityRes = await Process.run('zenity', [
        '--file-selection',
        '--title=$title',
      ]);
      if (zenityRes.exitCode == 0) {
        final path = zenityRes.stdout.toString().trim();
        if (path.isNotEmpty) return path;
      }
    } catch (_) {}

    try {
      if (kIsWeb) return null;
      if (!Platform.isLinux) return null;

      // 2. Try kdialog fallback (KDE / Qt)
      final kdialogRes = await Process.run('kdialog', [
        '--getopenfilename',
        '.',
        '*',
        '--title',
        title,
      ]);
      if (kdialogRes.exitCode == 0) {
        final path = kdialogRes.stdout.toString().trim();
        if (path.isNotEmpty) return path;
      }
    } catch (_) {}

    return null;
  }

  @override
  void dispose() {
    // Pending RPC futures may still resolve after the widget tree is gone; the
    // guard below keeps those late notifications from touching a disposed
    // ChangeNotifier. The shared backend is deliberately NOT disposed here.
    //
    // Idempotent: an owner teardown plus an explicit call must not throw.
    if (_disposed) return;
    _disposed = true;
    _streamSub?.cancel();
    _streamSub = null;
    super.dispose();
  }

  /// [ChangeNotifier.notifyListeners] that ignores calls arriving after
  /// [dispose] (late hydrate/send/cancel completions).
  void _notify() {
    if (_disposed) return;
    notifyListeners();
  }
}
