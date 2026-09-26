import 'package:flutter/material.dart';
import '../controllers/phonebridge_controller.dart';

/// Card guiding Android users through granting required permissions and
/// navigating Android 13+ Restricted Settings (M-BETA-1).
class PermissionOnboardingCard extends StatelessWidget {
  const PermissionOnboardingCard({
    super.key,
    required this.controller,
    this.initiallyExpanded = false,
  });

  final PhoneBridgeController controller;
  final bool initiallyExpanded;

  @override
  Widget build(BuildContext context) {
    if (!controller.service.isAndroid) {
      return const SizedBox.shrink();
    }

    final theme = Theme.of(context);
    final hasMissing = controller.hasMissingPermissions;
    final postNotifications = controller.postNotificationsGranted;
    final notifListener = controller.notificationListenerEnabled;
    final accessibility = controller.accessibilityEnabled;

    final grantedCount = (postNotifications ? 1 : 0) +
        (notifListener ? 1 : 0) +
        (accessibility ? 1 : 0);

    return Card(
      elevation: 0,
      color: hasMissing
          ? theme.colorScheme.errorContainer.withValues(alpha: 0.25)
          : theme.colorScheme.surfaceContainerHighest.withValues(alpha: 0.5),
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(16),
        side: BorderSide(
          color: hasMissing
              ? theme.colorScheme.error.withValues(alpha: 0.3)
              : theme.colorScheme.outlineVariant.withValues(alpha: 0.5),
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
                  width: 40,
                  height: 40,
                  decoration: BoxDecoration(
                    color: hasMissing
                        ? theme.colorScheme.error.withValues(alpha: 0.15)
                        : theme.colorScheme.primaryContainer,
                    borderRadius: BorderRadius.circular(10),
                  ),
                  child: Icon(
                    hasMissing ? Icons.security_update_warning : Icons.verified_user,
                    color: hasMissing
                        ? theme.colorScheme.error
                        : theme.colorScheme.onPrimaryContainer,
                    size: 22,
                  ),
                ),
                const SizedBox(width: 12),
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        hasMissing
                            ? 'Android Permissions Setup ($grantedCount/3)'
                            : 'Permissions Configured',
                        style: theme.textTheme.titleMedium?.copyWith(
                          fontWeight: FontWeight.bold,
                        ),
                      ),
                      const SizedBox(height: 2),
                      Text(
                        hasMissing
                            ? 'Required for background service, notifications, and remote control'
                            : 'All system services and notification access are active',
                        style: theme.textTheme.bodySmall?.copyWith(
                          color: theme.colorScheme.onSurfaceVariant,
                        ),
                      ),
                    ],
                  ),
                ),
                IconButton(
                  tooltip: 'Refresh permissions',
                  icon: const Icon(Icons.refresh, size: 20),
                  onPressed: controller.refreshPermissions,
                ),
              ],
            ),
            const SizedBox(height: 14),
            _buildPermissionItem(
              context: context,
              theme: theme,
              title: 'Notifications Permission',
              subtitle: 'Show foreground service alerts & system status',
              icon: Icons.notifications_active_outlined,
              isGranted: postNotifications,
              actionLabel: 'Allow',
              onAction: () => controller.requestNotificationPermission(),
            ),
            const Divider(height: 16),
            _buildPermissionItem(
              context: context,
              theme: theme,
              title: 'Notification Mirroring',
              subtitle: 'Forward incoming phone alerts to your Linux desktop',
              icon: Icons.sync_outlined,
              isGranted: notifListener,
              actionLabel: 'Open Settings',
              onAction: () => controller.openNotificationListenerSettings(),
            ),
            const Divider(height: 16),
            _buildPermissionItem(
              context: context,
              theme: theme,
              title: 'Remote Input & Accessibility',
              subtitle: 'Inject remote touch and navigation gestures from Linux',
              icon: Icons.accessibility_new_outlined,
              isGranted: accessibility,
              actionLabel: 'Open Settings',
              onAction: () => controller.openAccessibilitySettings(),
            ),
            if (hasMissing) ...[
              const SizedBox(height: 14),
              _buildRestrictedSettingsBox(context, theme),
            ],
          ],
        ),
      ),
    );
  }

  Widget _buildPermissionItem({
    required BuildContext context,
    required ThemeData theme,
    required String title,
    required String subtitle,
    required IconData icon,
    required bool isGranted,
    required String actionLabel,
    required VoidCallback onAction,
  }) {
    return Row(
      children: [
        Icon(
          icon,
          size: 22,
          color: isGranted
              ? theme.colorScheme.primary
              : theme.colorScheme.onSurfaceVariant,
        ),
        const SizedBox(width: 12),
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                title,
                style: theme.textTheme.bodyMedium?.copyWith(
                  fontWeight: FontWeight.w600,
                ),
              ),
              const SizedBox(height: 1),
              Text(
                subtitle,
                style: theme.textTheme.bodySmall?.copyWith(
                  color: theme.colorScheme.onSurfaceVariant,
                ),
              ),
            ],
          ),
        ),
        const SizedBox(width: 8),
        if (isGranted)
          Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              Icon(
                Icons.check_circle,
                color: theme.colorScheme.primary,
                size: 18,
              ),
              const SizedBox(width: 4),
              Text(
                'Active',
                style: theme.textTheme.labelMedium?.copyWith(
                  color: theme.colorScheme.primary,
                  fontWeight: FontWeight.bold,
                ),
              ),
            ],
          )
        else
          FilledButton.tonal(
            style: FilledButton.styleFrom(
              padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
              visualDensity: VisualDensity.compact,
            ),
            onPressed: onAction,
            child: Text(actionLabel),
          ),
      ],
    );
  }

  Widget _buildRestrictedSettingsBox(BuildContext context, ThemeData theme) {
    return Container(
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        color: theme.colorScheme.surfaceContainerHighest.withValues(alpha: 0.6),
        borderRadius: BorderRadius.circular(10),
        border: Border.all(
          color: theme.colorScheme.outlineVariant.withValues(alpha: 0.5),
        ),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Icon(
                Icons.info_outline,
                size: 16,
                color: theme.colorScheme.primary,
              ),
              const SizedBox(width: 6),
              Text(
                'Android 13+ Restricted Settings Guidance',
                style: theme.textTheme.labelMedium?.copyWith(
                  fontWeight: FontWeight.bold,
                  color: theme.colorScheme.primary,
                ),
              ),
            ],
          ),
          const SizedBox(height: 6),
          Text(
            'If Accessibility or Notification Access is grayed out ("Restricted setting"), '
            'open App Info, tap ⋮ (More options) at the top right, and select "Allow restricted settings".',
            style: theme.textTheme.bodySmall?.copyWith(
              color: theme.colorScheme.onSurfaceVariant,
            ),
          ),
          const SizedBox(height: 8),
          OutlinedButton.icon(
            style: OutlinedButton.styleFrom(
              padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 4),
              visualDensity: VisualDensity.compact,
            ),
            icon: const Icon(Icons.settings_applications_outlined, size: 16),
            label: const Text('Open App Info'),
            onPressed: () => controller.openAppDetailsSettings(),
          ),
        ],
      ),
    );
  }
}
