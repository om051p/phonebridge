import 'package:flutter/material.dart';

import '../controllers/transfer_controller.dart';
import '../models/transfer_item.dart';

// Transfer UI for the desktop/mobile mission control, built from the same
// Material 3 tokens as the other screens: surfaceContainerHighest cards with
// outlineVariant borders, ListTile-style rows, colorScheme.error for failures.
// Nothing here talks to a backend directly — it renders a TransferController.

/// One transfer row: filename, direction, peer, state, progress, bytes, typed
/// reason text and timestamp.
class TransferTile extends StatelessWidget {
  const TransferTile({
    super.key,
    required this.item,
    this.controller,
    this.showCancel = true,
  });

  final TransferItem item;

  /// When provided (and [showCancel] is true) an in-flight transfer offers a
  /// cancel action that is disabled while the daemon has not answered yet.
  final TransferController? controller;

  final bool showCancel;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final color = transferStateColor(item, theme);
    final reason = item.reasonText;
    final savedAs = item.savedAsLabel;
    final progress = item.progress;
    final showProgressBar = progress != null && item.isInFlight;
    final showBytes = item.sizeBytes > 0 || item.bytesTransferred > 0;
    final cancellable = showCancel && controller != null && item.isCancellable;

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        ListTile(
          contentPadding:
              const EdgeInsets.symmetric(vertical: 4, horizontal: 16),
          leading: CircleAvatar(
            radius: 20,
            backgroundColor: color.withValues(alpha: 0.15),
            child: Icon(transferDirectionIcon(item), color: color, size: 20),
          ),
          title: Text(
            item.displayName,
            maxLines: 1,
            overflow: TextOverflow.ellipsis,
            style: const TextStyle(fontWeight: FontWeight.bold, fontSize: 14),
          ),
          subtitle: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              const SizedBox(height: 2),
              Text(
                '${item.directionLabel} · ${item.stateLabel} · ${item.peerLabel}',
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                style: TextStyle(
                  color: theme.colorScheme.onSurfaceVariant,
                  fontSize: 12,
                ),
              ),
              if (showBytes)
                Padding(
                  padding: const EdgeInsets.only(top: 2),
                  child: Text(
                    progress == null
                        ? item.progressLabel
                        : '${item.progressLabel} · ${item.progressPercent}%',
                    style: TextStyle(
                      color: theme.colorScheme.onSurfaceVariant,
                      fontSize: 11,
                    ),
                  ),
                ),
              if (reason != null)
                Padding(
                  padding: const EdgeInsets.only(top: 2),
                  child: Text(
                    reason,
                    style: TextStyle(
                      color: item.isFailed ? theme.colorScheme.error : color,
                      fontSize: 12,
                      fontWeight: FontWeight.w600,
                    ),
                  ),
                ),
              if (savedAs != null)
                Padding(
                  padding: const EdgeInsets.only(top: 2),
                  child: Text(
                    savedAs,
                    style: TextStyle(
                      color: theme.colorScheme.onSurfaceVariant,
                      fontSize: 11,
                    ),
                  ),
                ),
            ],
          ),
          trailing: Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              Text(
                item.timeAgo,
                style: TextStyle(
                  color: theme.colorScheme.onSurfaceVariant,
                  fontSize: 11,
                ),
              ),
              if (item.isComplete && item.isInbound && controller != null) ...[
                const SizedBox(width: 4),
                IconButton(
                  icon: const Icon(Icons.folder_open, size: 20),
                  tooltip: 'Show in file manager',
                  onPressed: () {
                    final target = item.savedName.isNotEmpty
                        ? item.savedName
                        : item.displayName;
                    controller!.openContainingFolder(target);
                  },
                ),
              ],
              if (cancellable) ...[
                const SizedBox(width: 4),
                IconButton(
                  icon: const Icon(Icons.close, size: 20),
                  tooltip: 'Cancel transfer',
                  onPressed: controller!.canCancel(item)
                      ? () => controller!.cancelTransfer(item.transferId)
                      : null,
                ),
              ],
            ],
          ),
        ),
        if (showProgressBar)
          Padding(
            padding: const EdgeInsets.fromLTRB(16, 0, 16, 8),
            child: ClipRRect(
              borderRadius: BorderRadius.circular(4),
              child: LinearProgressIndicator(
                value: progress,
                minHeight: 5,
                color: color,
                backgroundColor: color.withValues(alpha: 0.15),
              ),
            ),
          ),
      ],
    );
  }
}

/// Colour for a transfer's state: green complete, red failed, grey cancelled,
/// amber while verifying, primary while bytes flow.
Color transferStateColor(TransferItem item, ThemeData theme) {
  if (item.isComplete) return Colors.green;
  if (item.isFailed) return theme.colorScheme.error;
  if (item.isCancelled) return Colors.grey;
  if (item.isVerifying) return Colors.amber.shade700;
  return theme.colorScheme.primary;
}

IconData transferDirectionIcon(TransferItem item) {
  if (item.isInbound) return Icons.download_outlined;
  return Icons.upload_outlined;
}

/// Basename of a local path, for messages shown before the daemon answers.
String fileNameFromPath(String path) {
  final parts = path.replaceAll('\\', '/').split('/');
  final names = parts.where((part) => part.isNotEmpty);
  return names.isEmpty ? path : names.last;
}

/// The transfers surface: header with the Send-file action, the transfer rows and
/// explicit empty / unavailable / error states. Rendered inside the Activity
/// screen; pass [maxItems] to reuse it as a compact section elsewhere.
class TransfersSection extends StatelessWidget {
  const TransfersSection({
    super.key,
    required this.controller,
    this.maxItems,
  });

  final TransferController controller;
  final int? maxItems;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return ListenableBuilder(
      listenable: controller,
      builder: (context, _) {
        final all = controller.items;
        final shown = (maxItems == null || all.length <= maxItems!)
            ? all
            : all.sublist(0, maxItems!);

        return Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            _buildHeader(context, theme, all.length),
            const SizedBox(height: 8),
            if (!controller.supportsFileTransfer)
              _buildUnavailableCard(theme)
            else if (controller.isBackendUnavailable && all.isEmpty)
              _buildErrorCard(theme)
            else if (all.isEmpty)
              _buildEmptyCard(theme)
            else
              _buildListCard(theme, shown),
          ],
        );
      },
    );
  }

  Widget _buildHeader(BuildContext context, ThemeData theme, int count) {
    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: 16.0),
      // Wrap so the SEND FILE action drops below the title on narrow phones
      // instead of overflowing to the right.
      child: Wrap(
        spacing: 8,
        runSpacing: 8,
        crossAxisAlignment: WrapCrossAlignment.center,
        children: [
          Icon(
            Icons.insert_drive_file_outlined,
            size: 18,
            color: theme.colorScheme.primary,
          ),
          Text(
            'File Transfers',
            style: theme.textTheme.titleSmall?.copyWith(
              fontWeight: FontWeight.bold,
            ),
          ),
          if (count > 0)
            Container(
              padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
              decoration: BoxDecoration(
                color: theme.colorScheme.surfaceContainerHighest,
                borderRadius: BorderRadius.circular(10),
              ),
              child: Text(
                '$count',
                style: theme.textTheme.labelSmall?.copyWith(
                  fontWeight: FontWeight.bold,
                  color: theme.colorScheme.onSurfaceVariant,
                ),
              ),
            ),
          if (controller.supportsFileTransfer)
            FilledButton.tonalIcon(
              onPressed:
                  controller.isSending ? null : () => _promptAndSend(context),
              icon: const Icon(Icons.upload_file, size: 18),
              label: const Text('SEND FILE'),
              style: FilledButton.styleFrom(
                visualDensity: VisualDensity.compact,
              ),
            ),
          if (controller.hasFinishedTransfers)
            TextButton.icon(
              onPressed: controller.clearFinished,
              icon: const Icon(Icons.clear_all, size: 16),
              label: const Text('CLEAR FINISHED'),
              style: TextButton.styleFrom(
                visualDensity: VisualDensity.compact,
              ),
            ),
        ],
      ),
    );
  }

  Widget _buildListCard(ThemeData theme, List<TransferItem> items) {
    return Card(
      elevation: 0,
      margin: const EdgeInsets.symmetric(horizontal: 16.0),
      color: theme.colorScheme.surfaceContainerHighest.withValues(alpha: 0.35),
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(16),
        side: BorderSide(
          color: theme.colorScheme.outlineVariant.withValues(alpha: 0.4),
        ),
      ),
      child: Column(
        children: [
          for (var i = 0; i < items.length; i++) ...[
            if (i > 0) const Divider(height: 1),
            TransferTile(item: items[i], controller: controller),
          ],
        ],
      ),
    );
  }

  Widget _buildEmptyCard(ThemeData theme) {
    return _stateCard(
      theme: theme,
      icon: Icons.swap_horiz,
      title: 'No transfers yet',
      body:
          'Send a file to your paired device with SEND FILE. Files you receive '
          'appear here too, with live progress.',
    );
  }

  Widget _buildUnavailableCard(ThemeData theme) {
    return _stateCard(
      theme: theme,
      icon: Icons.upload_file_outlined,
      title: kTransferUnavailableMessage,
      body: 'This build does not expose the native transfer channel yet. '
          'Sending files from the paired Linux desktop works today.',
    );
  }

  Widget _buildErrorCard(ThemeData theme) {
    return Card(
      elevation: 0,
      margin: const EdgeInsets.symmetric(horizontal: 16.0),
      color: theme.colorScheme.error.withValues(alpha: 0.08),
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(16),
        side: BorderSide(
          color: theme.colorScheme.error.withValues(alpha: 0.3),
        ),
      ),
      child: Padding(
        padding: const EdgeInsets.fromLTRB(16, 12, 8, 12),
        child: Row(
          children: [
            Icon(Icons.error_outline, color: theme.colorScheme.error, size: 20),
            const SizedBox(width: 12),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    'Transfer history unavailable',
                    style: theme.textTheme.titleSmall?.copyWith(
                      fontWeight: FontWeight.bold,
                      color: theme.colorScheme.error,
                    ),
                  ),
                  const SizedBox(height: 2),
                  Text(
                    controller.errorMessage ?? 'The engine did not respond',
                    maxLines: 3,
                    overflow: TextOverflow.ellipsis,
                    style: theme.textTheme.bodySmall?.copyWith(
                      color: theme.colorScheme.onSurfaceVariant,
                    ),
                  ),
                ],
              ),
            ),
            TextButton(
              onPressed:
                  controller.isLoading ? null : () => controller.hydrate(),
              child: const Text('RETRY'),
            ),
          ],
        ),
      ),
    );
  }

  Widget _stateCard({
    required ThemeData theme,
    required IconData icon,
    required String title,
    required String body,
  }) {
    return Card(
      elevation: 0,
      margin: const EdgeInsets.symmetric(horizontal: 16.0),
      color: theme.colorScheme.surfaceContainerHighest.withValues(alpha: 0.25),
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(16),
        side: BorderSide(
          color: theme.colorScheme.outlineVariant.withValues(alpha: 0.4),
        ),
      ),
      child: Padding(
        padding: const EdgeInsets.all(16.0),
        child: Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Icon(icon, size: 20, color: theme.colorScheme.onSurfaceVariant),
            const SizedBox(width: 12),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    title,
                    style: theme.textTheme.titleSmall?.copyWith(
                      fontWeight: FontWeight.bold,
                    ),
                  ),
                  const SizedBox(height: 2),
                  Text(
                    body,
                    style: theme.textTheme.bodySmall?.copyWith(
                      color: theme.colorScheme.onSurfaceVariant,
                      height: 1.35,
                    ),
                  ),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }

  Future<void> _promptAndSend(BuildContext context) async {
    final messenger = ScaffoldMessenger.of(context);
    final path = await showSendFileDialog(context, controller: controller);
    if (path == null || path.trim().isEmpty) return;

    final result = await controller.sendFile(localPath: path);
    messenger.showSnackBar(
      SnackBar(
        content: Text(
          result.isOk
              ? 'Sending ${fileNameFromPath(path)}…'
              : 'Could not send file: ${result.errorText}',
        ),
        behavior: SnackBarBehavior.floating,
        duration: const Duration(seconds: 3),
      ),
    );
  }
}

/// Asks for the absolute path of the local file to send and returns it, or null
/// when the user cancelled. Offers a native file picker on Linux desktop.
Future<String?> showSendFileDialog(BuildContext context, {TransferController? controller}) {
  return showDialog<String>(
    context: context,
    builder: (_) => _SendFileDialog(controller: controller),
  );
}

class _SendFileDialog extends StatefulWidget {
  const _SendFileDialog({this.controller});

  final TransferController? controller;

  @override
  State<_SendFileDialog> createState() => _SendFileDialogState();
}

class _SendFileDialogState extends State<_SendFileDialog> {
  final TextEditingController _pathController = TextEditingController();
  String? _error;

  @override
  void dispose() {
    _pathController.dispose();
    super.dispose();
  }

  Future<void> _browse() async {
    final picked = await (widget.controller?.pickDesktopFilePath() ??
        TransferController.pickNativeFile());
    if (picked != null && picked.isNotEmpty && mounted) {
      _pathController.text = picked;
      setState(() => _error = null);
    }
  }

  String? _validate(String raw) {
    final value = raw.trim();
    if (value.isEmpty) return 'Enter the file path to send';
    final isAbsolute =
        value.startsWith('/') || RegExp(r'^[a-zA-Z]:[\\/]').hasMatch(value);
    if (!isAbsolute) {
      return 'Use an absolute path, e.g. /home/you/report.pdf';
    }
    return null;
  }

  void _submit() {
    final error = _validate(_pathController.text);
    if (error != null) {
      setState(() => _error = error);
      return;
    }
    Navigator.of(context).pop(_pathController.text.trim());
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return AlertDialog(
      title: const Text('Send File'),
      content: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            'The file is streamed straight from disk to the paired device over '
            'the session channel. Nothing is uploaded anywhere else.',
            style: theme.textTheme.bodySmall?.copyWith(
              color: theme.colorScheme.onSurfaceVariant,
              height: 1.35,
            ),
          ),
          const SizedBox(height: 12),
          Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Expanded(
                child: TextField(
                  controller: _pathController,
                  autofocus: true,
                  decoration: InputDecoration(
                    labelText: 'Absolute file path',
                    hintText: '/home/you/Documents/report.pdf',
                    errorText: _error,
                    border: const OutlineInputBorder(),
                    prefixIcon: const Icon(Icons.insert_drive_file_outlined),
                  ),
                  onSubmitted: (_) => _submit(),
                ),
              ),
              const SizedBox(width: 8),
              FilledButton.tonalIcon(
                style: FilledButton.styleFrom(
                  padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 16),
                ),
                onPressed: _browse,
                icon: const Icon(Icons.folder_open, size: 18),
                label: const Text('BROWSE…'),
              ),
            ],
          ),
        ],
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.of(context).pop(),
          child: const Text('Cancel'),
        ),
        FilledButton(
          onPressed: _submit,
          child: const Text('SEND'),
        ),
      ],
    );
  }
}

/// Compact Card summarising transfers for the Home screen, with a way into the
/// Activity tab (the full history lives there).
class TransferSummaryCard extends StatelessWidget {
  const TransferSummaryCard({
    super.key,
    required this.controller,
    required this.onOpen,
  });

  final TransferController controller;
  final VoidCallback onOpen;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return ListenableBuilder(
      listenable: controller,
      builder: (context, _) {
        final latest = controller.latest;
        final active = controller.activeCount;
        final progress = latest?.progress;

        return Card(
          elevation: 0,
          color:
              theme.colorScheme.surfaceContainerHighest.withValues(alpha: 0.5),
          shape: RoundedRectangleBorder(
            borderRadius: BorderRadius.circular(16),
            side: BorderSide(
              color: theme.colorScheme.outlineVariant.withValues(alpha: 0.5),
            ),
          ),
          child: Padding(
            padding: const EdgeInsets.all(16.0),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Row(
                  children: [
                    Container(
                      width: 44,
                      height: 44,
                      decoration: BoxDecoration(
                        color: theme.colorScheme.primaryContainer,
                        borderRadius: BorderRadius.circular(12),
                      ),
                      child: Icon(
                        Icons.insert_drive_file_outlined,
                        color: theme.colorScheme.onPrimaryContainer,
                        size: 22,
                      ),
                    ),
                    const SizedBox(width: 14),
                    Expanded(
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          Text(
                            'File Transfers',
                            style: theme.textTheme.titleMedium?.copyWith(
                              fontWeight: FontWeight.bold,
                            ),
                          ),
                          const SizedBox(height: 2),
                          Text(
                            _summaryText(active, latest),
                            maxLines: 2,
                            overflow: TextOverflow.ellipsis,
                            style: theme.textTheme.bodySmall?.copyWith(
                              color: theme.colorScheme.onSurfaceVariant,
                            ),
                          ),
                        ],
                      ),
                    ),
                    FilledButton.tonal(
                      onPressed: onOpen,
                      style: FilledButton.styleFrom(
                        visualDensity: VisualDensity.compact,
                      ),
                      child: const Text('Open'),
                    ),
                  ],
                ),
                if (active > 0 && progress != null) ...[
                  const SizedBox(height: 12),
                  ClipRRect(
                    borderRadius: BorderRadius.circular(4),
                    child: LinearProgressIndicator(
                      value: progress,
                      minHeight: 5,
                      color: transferStateColor(latest!, theme),
                      backgroundColor:
                          theme.colorScheme.surfaceContainerHighest,
                    ),
                  ),
                ],
              ],
            ),
          ),
        );
      },
    );
  }

  String _summaryText(int active, TransferItem? latest) {
    if (active > 0) {
      final name = latest?.displayName ?? 'Transfer';
      return active == 1
          ? '$name · ${latest?.progressPercent ?? 0}%'
          : '$active transfers in flight';
    }
    if (latest != null) return '${latest.displayName} · ${latest.stateLabel}';
    if (!controller.supportsFileTransfer) return kTransferUnavailableMessage;
    return 'No transfers yet';
  }
}
