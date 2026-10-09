import 'package:flutter/material.dart' hide ClipboardStatus;
import 'package:flutter/services.dart' show Clipboard, ClipboardData;
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
      padding: const EdgeInsets.fromLTRB(16, 16, 16, 32),
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
          _buildCompanionImeGuidanceCard(context, theme, clipboard.imeSelected),
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
      case ClipboardSyncState.needsSetup:
        statusColor = Colors.orange.shade800;
        break;
      case ClipboardSyncState.restricted:
        statusColor = theme.colorScheme.error;
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
    return Column(
      children: [
        SizedBox(
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
                          content: Text(
                            ok
                                ? 'Clipboard synchronized'
                                : 'Nothing forwarded — copy something on this device first, '
                                    'and make sure a paired device is connected',
                          ),
                          behavior: SnackBarBehavior.floating,
                          duration: const Duration(seconds: 3),
                        ),
                      );
                    }
                  },
            icon: const Icon(Icons.sync),
            label: const Text('SYNC CURRENT CLIPBOARD NOW'),
          ),
        ),
        const SizedBox(height: 10),
        SizedBox(
          width: double.infinity,
          height: 44,
          child: OutlinedButton.icon(
            onPressed: (!isEnabled || controller.isLoading)
                ? null
                : () async {
                    final now = DateTime.now();
                    final timeStr =
                        '${now.hour.toString().padLeft(2, '0')}:${now.minute.toString().padLeft(2, '0')}:${now.second.toString().padLeft(2, '0')}';
                    final testText = 'PhoneBridge Test Clip ($timeStr)';
                    await Clipboard.setData(ClipboardData(text: testText));
                    final ok = await controller.triggerClipboardPull();
                    if (context.mounted) {
                      ScaffoldMessenger.of(context).showSnackBar(
                        SnackBar(
                          content: Text(
                            ok
                                ? 'Test clip sent to paired device: "$testText"'
                                : 'Test clip copied locally. Connect a device to sync automatically.',
                          ),
                          behavior: SnackBarBehavior.floating,
                          duration: const Duration(seconds: 3),
                        ),
                      );
                    }
                  },
            icon: const Icon(Icons.send_to_mobile, size: 18),
            label: const Text('SEND TEST CLIP'),
          ),
        ),
      ],
    );
  }

  Widget _buildCompanionImeGuidanceCard(
    BuildContext context,
    ThemeData theme,
    bool imeSelected,
  ) {
    final accessibilityActive = controller.accessibilityEnabled;
    final isAutoActive = accessibilityActive || imeSelected;

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
                Icon(
                  accessibilityActive
                      ? Icons.accessibility_new_outlined
                      : Icons.keyboard_outlined,
                  color: theme.colorScheme.primary,
                  size: 20,
                ),
                const SizedBox(width: 8),
                Expanded(
                  child: Text(
                    'Automated Background Sync',
                    style: theme.textTheme.titleSmall?.copyWith(
                      fontWeight: FontWeight.bold,
                    ),
                    overflow: TextOverflow.ellipsis,
                    maxLines: 1,
                  ),
                ),
                const SizedBox(width: 8),
                Container(
                  padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 3),
                  decoration: BoxDecoration(
                    color: isAutoActive
                        ? Colors.green.withValues(alpha: 0.15)
                        : Colors.orange.withValues(alpha: 0.15),
                    borderRadius: BorderRadius.circular(8),
                  ),
                  child: Text(
                    accessibilityActive
                        ? 'Active (Accessibility)'
                        : (imeSelected ? 'Active (Keyboard)' : 'Action Required'),
                    style: TextStyle(
                      color: isAutoActive ? Colors.green : Colors.orange.shade800,
                      fontWeight: FontWeight.bold,
                      fontSize: 11,
                    ),
                  ),
                ),
              ],
            ),
            const SizedBox(height: 8),
            Text(
              accessibilityActive
                  ? 'Automated background sync is active via PhoneBridge Accessibility Service. '
                      'Any text you copy in Chrome, Notes, or other apps is forwarded instantly '
                      'to your PC while you keep your normal keyboard (Gboard).'
                  : (imeSelected
                      ? 'The PhoneBridge Companion Keyboard is active. Copies are detected '
                          'instantly and forwarded to your paired desktop without any manual pull.'
                      : 'On modern Android, background apps are restricted from reading the clipboard. '
                          'Enable the Accessibility Service (recommended, lets you keep Gboard) or activate '
                          'the Companion Keyboard to automatically sync text when copied in any app.'),
              style: theme.textTheme.bodySmall?.copyWith(
                color: theme.colorScheme.onSurfaceVariant,
                height: 1.35,
              ),
            ),
            if (!isAutoActive) ...[
              const SizedBox(height: 12),
              Wrap(
                spacing: 8,
                runSpacing: 8,
                children: [
                  FilledButton.tonalIcon(
                    onPressed: () => _openAccessibilitySettings(context),
                    icon: const Icon(Icons.accessibility_new_outlined, size: 18),
                    label: const Text('ENABLE ACCESSIBILITY'),
                  ),
                  OutlinedButton.icon(
                    onPressed: () => _openImeSettings(context),
                    icon: const Icon(Icons.settings_outlined, size: 18),
                    label: const Text('ENABLE KEYBOARD'),
                  ),
                  OutlinedButton.icon(
                    onPressed: () => _selectIme(context),
                    icon: const Icon(Icons.keyboard_alt_outlined, size: 18),
                    label: const Text('SELECT KEYBOARD'),
                  ),
                ],
              ),
            ],
          ],
        ),
      ),
    );
  }

  /// Opens the system Accessibility settings screen so the accessibility service can be enabled
  /// for background copy detection without replacing the typing keyboard.
  Future<void> _openAccessibilitySettings(BuildContext context) async {
    final ok = await controller.openAccessibilitySettings();
    if (!context.mounted) return;
    if (!ok) {
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(
          content: Text('Could not open accessibility settings on this device'),
          behavior: SnackBarBehavior.floating,
          duration: Duration(seconds: 3),
        ),
      );
    }
  }

  /// Opens the system "Languages & input" screen so the companion IME can be
  /// enabled (Android hides it from the keyboard picker until it is).
  Future<void> _openImeSettings(BuildContext context) async {
    final ok = await controller.openInputMethodSettings();
    if (!context.mounted) return;
    if (!ok) {
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(
          content: Text('Could not open keyboard settings on this device'),
          behavior: SnackBarBehavior.floating,
          duration: Duration(seconds: 3),
        ),
      );
    }
  }

  /// Shows the system keyboard picker so the companion IME can be selected as
  /// the active keyboard. Without that selection Android suppresses
  /// background clipboard reads (Spike 05 / DEC-023).
  Future<void> _selectIme(BuildContext context) async {
    final ok = await controller.showInputMethodPicker();
    if (!context.mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(
        content: Text(
          ok
              ? 'Choose "PhoneBridge" from the keyboard list'
              : 'Could not open the keyboard picker on this device',
        ),
        behavior: SnackBarBehavior.floating,
        duration: const Duration(seconds: 3),
      ),
    );
  }

  Widget _buildLinuxWaylandGuidanceCard(ThemeData theme, ClipboardStatus clipboard) {
    final needsSetup = clipboard.state == ClipboardSyncState.needsSetup ||
        clipboard.state == ClipboardSyncState.unavailable ||
        clipboard.state == ClipboardSyncState.restricted;
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
                Expanded(
                  child: Text(
                    'Wayland Data Control',
                    style: theme.textTheme.titleSmall?.copyWith(
                      fontWeight: FontWeight.bold,
                    ),
                    overflow: TextOverflow.ellipsis,
                    maxLines: 1,
                  ),
                ),
                const SizedBox(width: 8),
                Container(
                  padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 3),
                  decoration: BoxDecoration(
                    color: !needsSetup
                        ? Colors.green.withValues(alpha: 0.15)
                        : Colors.orange.withValues(alpha: 0.15),
                    borderRadius: BorderRadius.circular(8),
                  ),
                  child: Text(
                    !needsSetup ? 'Active' : 'Setup Required',
                    style: TextStyle(
                      color: !needsSetup ? Colors.green : Colors.orange.shade800,
                      fontWeight: FontWeight.bold,
                      fontSize: 11,
                    ),
                  ),
                ),
              ],
            ),
            const SizedBox(height: 8),
            Text(
              needsSetup
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
        children: [
          Expanded(
            child: Text(label,
                style: TextStyle(
                    color: theme.colorScheme.onSurfaceVariant, fontSize: 13)),
          ),
          const SizedBox(width: 8),
          Flexible(
            child: Text(value,
                textAlign: TextAlign.end,
                overflow: TextOverflow.ellipsis,
                maxLines: 2,
                style:
                    const TextStyle(fontWeight: FontWeight.bold, fontSize: 13)),
          ),
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
