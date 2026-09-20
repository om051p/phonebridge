import 'package:flutter/material.dart' hide ClipboardStatus;
import '../controllers/phonebridge_controller.dart';
import '../models/clipboard_status.dart';

class ClipboardScreen extends StatelessWidget {
  const ClipboardScreen({
    super.key,
    required this.controller,
  });

  final PhoneBridgeController controller;

  String _formatLastSync(int? ms) {
    if (ms == null || ms <= 0) return 'Never';
    final dt = DateTime.fromMillisecondsSinceEpoch(ms);
    return '${dt.hour.toString().padLeft(2, '0')}:${dt.minute.toString().padLeft(2, '0')}:${dt.second.toString().padLeft(2, '0')}';
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final clipboard = controller.clipboardStatus;
    final isEnabled = clipboard.isEnabled;
    final state = clipboard.state;

    final isLinux = controller.service.isLinux;

    return ListView(
      padding: const EdgeInsets.all(16.0),
      children: [
        _buildMasterToggleCard(theme, isEnabled),
        const SizedBox(height: 16),
        _buildStatusCard(theme, state, isEnabled),
        const SizedBox(height: 16),
        _buildSyncNowCard(context, theme, isEnabled),
        const SizedBox(height: 16),
        if (isLinux)
          _buildLinuxWaylandGuidanceCard(theme, clipboard)
        else
          _buildCompanionImeGuidanceCard(theme, clipboard.imeSelected),
        const SizedBox(height: 16),
        _buildSyncStatsCard(theme, clipboard),
        const SizedBox(height: 16),
        _buildPrivacyGuaranteeCard(theme),
      ],
    );
  }

  Widget _buildMasterToggleCard(ThemeData theme, bool isEnabled) {
    return Card(
      elevation: 0,
      color: theme.colorScheme.surfaceContainerHighest.withValues(alpha: 0.35),
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(16),
        side: BorderSide(color: theme.colorScheme.outlineVariant.withValues(alpha: 0.4)),
      ),
      child: SwitchListTile(
        title: const Text(
          'Clipboard Synchronization',
          style: TextStyle(fontWeight: FontWeight.bold),
        ),
        subtitle: Text(
          isEnabled
              ? 'Automatically synchronizes text clips between devices'
              : 'Clipboard sync is paused',
          style: TextStyle(
            fontSize: 12,
            color: theme.colorScheme.onSurfaceVariant,
          ),
        ),
        value: isEnabled,
        onChanged: (val) => controller.setClipboardEnabled(val),
      ),
    );
  }

  Widget _buildStatusCard(ThemeData theme, ClipboardSyncState state, bool isEnabled) {
    Color statusColor;
    switch (state) {
      case ClipboardSyncState.ambientActive:
        statusColor = Colors.green;
        break;
      case ClipboardSyncState.writeOnlyDormant:
        statusColor = Colors.amber.shade700;
        break;
      case ClipboardSyncState.unavailable:
        statusColor = theme.colorScheme.error;
        break;
      case ClipboardSyncState.stopped:
        statusColor = Colors.grey;
        break;
    }

    if (!isEnabled) {
      statusColor = Colors.grey;
    }

    return Card(
      elevation: 0,
      color: statusColor.withValues(alpha: 0.1),
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(16),
        side: BorderSide(color: statusColor.withValues(alpha: 0.3)),
      ),
      child: Padding(
        padding: const EdgeInsets.all(16.0),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Container(
                  width: 10,
                  height: 10,
                  decoration: BoxDecoration(
                    shape: BoxShape.circle,
                    color: statusColor,
                  ),
                ),
                const SizedBox(width: 8),
                Text(
                  isEnabled ? state.label : 'Sync Disabled',
                  style: theme.textTheme.titleSmall?.copyWith(
                    fontWeight: FontWeight.bold,
                    color: statusColor,
                  ),
                ),
              ],
            ),
            const SizedBox(height: 8),
            Text(
              isEnabled ? state.description : 'Turn on the master switch above to enable clipboard sync.',
              style: theme.textTheme.bodySmall?.copyWith(
                color: theme.colorScheme.onSurfaceVariant,
                height: 1.3,
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildSyncNowCard(BuildContext context, ThemeData theme, bool isEnabled) {
    return SizedBox(
      width: double.infinity,
      height: 48,
      child: FilledButton.tonalIcon(
        onPressed: (!isEnabled || controller.isLoading)
            ? null
            : () async {
                final ok = await controller.triggerClipboardPull();
                if (context.mounted) {
                  ScaffoldMessenger.of(context).showSnackBar(
                    SnackBar(
                      content: Text(ok ? 'Clipboard synchronized' : 'No clip available to sync'),
                      behavior: SnackBarBehavior.floating,
                      duration: const Duration(seconds: 2),
                    ),
                  );
                }
              },
        icon: const Icon(Icons.sync),
        label: const Text('SYNC CURRENT CLIPBOARD NOW'),
      ),
    );
  }

  Widget _buildCompanionImeGuidanceCard(ThemeData theme, bool imeSelected) {
    return Card(
      elevation: 0,
      color: theme.colorScheme.surfaceContainerHighest.withValues(alpha: 0.25),
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(16),
        side: BorderSide(color: theme.colorScheme.outlineVariant.withValues(alpha: 0.4)),
      ),
      child: Padding(
        padding: const EdgeInsets.all(16.0),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Icon(Icons.keyboard_outlined, color: theme.colorScheme.primary, size: 20),
                const SizedBox(width: 8),
                Text(
                  'Companion Input Method',
                  style: theme.textTheme.titleSmall?.copyWith(
                    fontWeight: FontWeight.bold,
                  ),
                ),
                const Spacer(),
                Container(
                  padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 3),
                  decoration: BoxDecoration(
                    color: imeSelected
                        ? Colors.green.withValues(alpha: 0.15)
                        : Colors.orange.withValues(alpha: 0.15),
                    borderRadius: BorderRadius.circular(8),
                  ),
                  child: Text(
                    imeSelected ? 'Active' : 'Optional',
                    style: TextStyle(
                      color: imeSelected ? Colors.green : Colors.orange.shade800,
                      fontWeight: FontWeight.bold,
                      fontSize: 11,
                    ),
                  ),
                ),
              ],
            ),
            const SizedBox(height: 8),
            Text(
              'On modern Android, background apps are restricted from reading your clipboard. '
              'The PhoneBridge Companion Keyboard runs passively alongside your regular keyboard '
              'to detect copy events instantly without requiring manual pulls.',
              style: theme.textTheme.bodySmall?.copyWith(
                color: theme.colorScheme.onSurfaceVariant,
                height: 1.35,
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildLinuxWaylandGuidanceCard(ThemeData theme, ClipboardStatus clipboard) {
    final isUnavailable = clipboard.state == ClipboardSyncState.unavailable;
    return Card(
      elevation: 0,
      color: theme.colorScheme.surfaceContainerHighest.withValues(alpha: 0.25),
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(16),
        side: BorderSide(color: theme.colorScheme.outlineVariant.withValues(alpha: 0.4)),
      ),
      child: Padding(
        padding: const EdgeInsets.all(16.0),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Icon(Icons.desktop_windows_outlined, color: theme.colorScheme.primary, size: 20),
                const SizedBox(width: 8),
                Text(
                  'Wayland Data Control',
                  style: theme.textTheme.titleSmall?.copyWith(
                    fontWeight: FontWeight.bold,
                  ),
                ),
                const Spacer(),
                Container(
                  padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 3),
                  decoration: BoxDecoration(
                    color: !isUnavailable
                        ? Colors.green.withValues(alpha: 0.15)
                        : Colors.orange.withValues(alpha: 0.15),
                    borderRadius: BorderRadius.circular(8),
                  ),
                  child: Text(
                    !isUnavailable ? 'Active' : 'Setup Required',
                    style: TextStyle(
                      color: !isUnavailable ? Colors.green : Colors.orange.shade800,
                      fontWeight: FontWeight.bold,
                      fontSize: 11,
                    ),
                  ),
                ),
              ],
            ),
            const SizedBox(height: 8),
            Text(
              isUnavailable
                  ? 'Linux Wayland clipboard synchronization requires compositor support for '
                      'wlr-data-control-unstable-v1.\n\n'
                      '• COSMIC Desktop: Set COSMIC_DATA_CONTROL_ENABLED=1 in /etc/environment or your session profile.\n'
                      '• wlroots / Sway / Hyprland: Supported natively.\n'
                      '• Note: GNOME Wayland lacks data-control protocol and is unsupported.'
                  : 'Wayland data-control helper is active. Clipboard changes are monitored passively '
                      'via wl-data-control and synchronized automatically with paired devices.',
              style: theme.textTheme.bodySmall?.copyWith(
                color: theme.colorScheme.onSurfaceVariant,
                height: 1.35,
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildSyncStatsCard(ThemeData theme, ClipboardStatus clipboard) {
    return Card(
      elevation: 0,
      color: theme.colorScheme.surfaceContainerHighest.withValues(alpha: 0.35),
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(16),
        side: BorderSide(color: theme.colorScheme.outlineVariant.withValues(alpha: 0.4)),
      ),
      child: Padding(
        padding: const EdgeInsets.all(16.0),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(
              'Synchronization Details',
              style: theme.textTheme.titleSmall?.copyWith(
                fontWeight: FontWeight.bold,
              ),
            ),
            const SizedBox(height: 10),
            _statRow(theme, 'Last Synchronized', _formatLastSync(clipboard.lastSyncTimestampMs)),
            _statRow(theme, 'Syncs Completed', '${clipboard.syncCount} items'),
            _statRow(theme, 'Payload Limit', '768 KiB (Application Ceiling)'),
          ],
        ),
      ),
    );
  }

  Widget _statRow(ThemeData theme, String label, String value) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 4.0),
      child: Row(
        mainAxisAlignment: MainAxisAlignment.spaceBetween,
        children: [
          Text(label, style: TextStyle(color: theme.colorScheme.onSurfaceVariant, fontSize: 13)),
          Text(value, style: const TextStyle(fontWeight: FontWeight.bold, fontSize: 13)),
        ],
      ),
    );
  }

  Widget _buildPrivacyGuaranteeCard(ThemeData theme) {
    return Card(
      elevation: 0,
      color: theme.colorScheme.surfaceContainerHighest.withValues(alpha: 0.15),
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(16),
        side: BorderSide(color: theme.colorScheme.outlineVariant.withValues(alpha: 0.2)),
      ),
      child: Padding(
        padding: const EdgeInsets.all(14.0),
        child: Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Icon(Icons.lock_outline, size: 18, color: theme.colorScheme.primary),
            const SizedBox(width: 10),
            Expanded(
              child: Text(
                'Zero-Logging Policy: PhoneBridge never logs or displays clipboard text. '
                'Deduplication and echo-suppression are computed using SHA-256 hashes on your device.',
                style: theme.textTheme.bodySmall?.copyWith(
                  color: theme.colorScheme.onSurfaceVariant,
                  fontSize: 11,
                  height: 1.3,
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}
