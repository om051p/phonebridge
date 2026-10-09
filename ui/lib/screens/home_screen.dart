import 'package:flutter/material.dart';
import '../controllers/phonebridge_controller.dart';
import '../models/feature_availability.dart';
import '../models/link_status.dart';
import '../ui/link_indicator.dart';
import '../models/activity_event.dart';
import '../ui/transfer_views.dart';
import '../ui/permission_onboarding_card.dart';

class HomeScreen extends StatelessWidget {
  const HomeScreen({
    super.key,
    required this.controller,
    required this.onNavigateToTab,
  });

  final PhoneBridgeController controller;
  final ValueChanged<int> onNavigateToTab;

  String _formatDuration(int us) {
    if (us <= 0) return '00:00';
    final totalSec = us ~/ 1000000;
    final min = (totalSec ~/ 60).toString().padLeft(2, '0');
    final sec = (totalSec % 60).toString().padLeft(2, '0');
    return '$min:$sec';
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    // One connection status for the app (Phase 5): the hero card no longer asks
    // "is capturing?" and infers a connection from it.
    final link = controller.linkStatus;
    final peer = controller.activePeer;
    final stats = controller.captureStats;
    final clipboard = controller.clipboardStatus;
    final recentEvents = controller.activityEvents.take(3).toList();

    return RefreshIndicator(
      onRefresh: controller.refreshAll,
      child: LayoutBuilder(
        builder: (context, constraints) {
          final isWide = constraints.maxWidth >= 720;
          if (isWide) {
            return ListView(
              padding: const EdgeInsets.fromLTRB(20, 20, 20, 32),
              children: [
                if (controller.service.isAndroid && controller.hasMissingPermissions) ...[
                  PermissionOnboardingCard(controller: controller),
                  const SizedBox(height: 16),
                ],
                Row(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Expanded(
                      flex: 5,
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          _buildConnectionCard(context, theme, peer, link),
                          const SizedBox(height: 16),
                          _buildQuickActionHub(context, theme, peer),
                          const SizedBox(height: 16),
                          _buildClipboardQuickCard(context, theme, clipboard),
                        ],
                      ),
                    ),
                    const SizedBox(width: 20),
                    Expanded(
                      flex: 5,
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          _buildScreenShareHeroCard(context, theme, link, stats),
                          const SizedBox(height: 16),
                          TransferSummaryCard(
                            controller: controller.transfers,
                            onOpen: () => onNavigateToTab(4), // Transfers live in Activity
                          ),
                          const SizedBox(height: 16),
                          _buildRecentActivitySection(context, theme, recentEvents),
                        ],
                      ),
                    ),
                  ],
                ),
              ],
            );
          }

          return ListView(
            padding: const EdgeInsets.fromLTRB(16, 16, 16, 32),
            children: [
              if (controller.service.isAndroid && controller.hasMissingPermissions) ...[
                PermissionOnboardingCard(controller: controller),
                const SizedBox(height: 16),
              ],
              _buildConnectionCard(context, theme, peer, link),
              const SizedBox(height: 16),
              _buildQuickActionHub(context, theme, peer),
              const SizedBox(height: 16),
              _buildScreenShareHeroCard(context, theme, link, stats),
              const SizedBox(height: 16),
              _buildClipboardQuickCard(context, theme, clipboard),
              const SizedBox(height: 16),
              TransferSummaryCard(
                controller: controller.transfers,
                onOpen: () => onNavigateToTab(4), // Transfers live in Activity
              ),
              const SizedBox(height: 16),
              _buildRecentActivitySection(context, theme, recentEvents),
            ],
          );
        },
      ),
    );
  }

  Widget _buildQuickActionHub(
    BuildContext context,
    ThemeData theme,
    dynamic peer,
  ) {
    return Card(
      elevation: 0,
      color: theme.colorScheme.surfaceContainerHighest.withValues(alpha: 0.35),
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(16),
        side: BorderSide(
          color: theme.colorScheme.outlineVariant.withValues(alpha: 0.4),
        ),
      ),
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 14.0, vertical: 12.0),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Icon(Icons.bolt, size: 18, color: theme.colorScheme.primary),
                const SizedBox(width: 6),
                Text(
                  'Quick Actions',
                  style: theme.textTheme.labelLarge?.copyWith(
                    fontWeight: FontWeight.bold,
                  ),
                ),
              ],
            ),
            const SizedBox(height: 10),
            Row(
              children: [
                Expanded(
                  child: _quickActionButton(
                    theme,
                    icon: Icons.screen_share_outlined,
                    label: 'Cast Screen',
                    onTap: () => onNavigateToTab(2),
                  ),
                ),
                const SizedBox(width: 8),
                Expanded(
                  child: _quickActionButton(
                    theme,
                    icon: Icons.content_paste_outlined,
                    label: 'Copy / Paste',
                    onTap: () => onNavigateToTab(3),
                  ),
                ),
                const SizedBox(width: 8),
                Expanded(
                  child: _quickActionButton(
                    theme,
                    icon: Icons.folder_shared_outlined,
                    label: 'Share File',
                    onTap: () => onNavigateToTab(4),
                  ),
                ),
                const SizedBox(width: 8),
                Expanded(
                  child: _quickActionButton(
                    theme,
                    icon: Icons.devices_outlined,
                    label: 'Pair Device',
                    onTap: () => onNavigateToTab(1),
                  ),
                ),
              ],
            ),
          ],
        ),
      ),
    );
  }

  Widget _quickActionButton(
    ThemeData theme, {
    required IconData icon,
    required String label,
    required VoidCallback onTap,
  }) {
    return InkWell(
      onTap: onTap,
      borderRadius: BorderRadius.circular(12),
      child: Container(
        padding: const EdgeInsets.symmetric(vertical: 8),
        decoration: BoxDecoration(
          color: theme.colorScheme.surfaceContainerHighest.withValues(alpha: 0.4),
          borderRadius: BorderRadius.circular(12),
        ),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(icon, size: 20, color: theme.colorScheme.primary),
            const SizedBox(height: 4),
            Text(
              label,
              style: theme.textTheme.labelSmall?.copyWith(
                fontWeight: FontWeight.w600,
              ),
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildConnectionCard(
    BuildContext context,
    ThemeData theme,
    dynamic peer,
    LinkStatus link,
  ) {
    // peer is the AUTHORITATIVE session peer (null when idle). Pairing state
    // is separate: trusted-but-idle is "paired, not connected", not "no
    // paired device".
    final hasPeer = peer != null;
    final hasTrusted = controller.trustedDevices.any((d) => !d.revoked);
    final isConnecting = controller.isSessionConnecting;

    return Card(
      elevation: 0,
      color: theme.colorScheme.surfaceContainerHighest.withValues(alpha: 0.5),
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(16),
        side: BorderSide(color: theme.colorScheme.outlineVariant.withValues(alpha: 0.5)),
      ),
      child: Padding(
        padding: const EdgeInsets.all(16.0),
        child: Row(
          children: [
            Container(
              width: 48,
              height: 48,
              decoration: BoxDecoration(
                color: theme.colorScheme.primaryContainer,
                borderRadius: BorderRadius.circular(12),
              ),
              child: Icon(
                hasPeer ? Icons.computer : (isConnecting ? Icons.sync : Icons.devices),
                color: theme.colorScheme.onPrimaryContainer,
                size: 26,
              ),
            ),
            const SizedBox(width: 14),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    hasPeer
                        ? peer.displayName
                        : (isConnecting
                            ? 'Connecting…'
                            : (hasTrusted
                                ? 'No active connection'
                                : 'No paired device')),
                    style: theme.textTheme.titleMedium?.copyWith(
                      fontWeight: FontWeight.bold,
                    ),
                    overflow: TextOverflow.ellipsis,
                    maxLines: 1,
                  ),
                  const SizedBox(height: 2),
                  Text(
                    hasPeer
                        // Trust (shield) is separate from connection (dot
                        // below): "Trusted" never means "Online". The peer
                        // shown here is the live session target.
                        ? '${peer.platform} · Connected · LAN · Direct'
                        : (isConnecting
                            ? 'Negotiating session connection…'
                            : (hasTrusted
                                ? 'Paired · select a device to connect'
                                : 'Pair with your other device to connect')),
                    style: theme.textTheme.bodySmall?.copyWith(
                      color: theme.colorScheme.onSurfaceVariant,
                    ),
                    overflow: TextOverflow.ellipsis,
                    maxLines: 1,
                  ),
                  const SizedBox(height: 6),
                  Tooltip(
                    message: [
                      link.description,
                      if (link.actionHint.isNotEmpty) link.actionHint,
                    ].join('\n'),
                    child: Row(
                      children: [
                        if (isConnecting) ...[
                          const SizedBox(
                            width: 8,
                            height: 8,
                            child: CircularProgressIndicator(
                              strokeWidth: 2,
                              color: Colors.amber,
                            ),
                          ),
                        ] else ...[
                          Container(
                            width: 8,
                            height: 8,
                            decoration: BoxDecoration(
                              shape: BoxShape.circle,
                              color: linkPhaseColor(theme, link.phase),
                            ),
                          ),
                        ],
                        const SizedBox(width: 6),
                        Expanded(
                          child: Text(
                            isConnecting ? 'CONNECTING…' : link.label,
                            maxLines: 1,
                            overflow: TextOverflow.ellipsis,
                            style: theme.textTheme.bodySmall?.copyWith(
                              fontWeight: FontWeight.bold,
                              color: isConnecting ? Colors.amber.shade800 : linkPhaseColor(theme, link.phase),
                            ),
                          ),
                        ),
                      ],
                    ),
                  ),
                  // Transfer activity is shown beside the connection, never as it.
                  if (link.transferLine.isNotEmpty)
                    Text(
                      link.transferLine,
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: theme.textTheme.labelSmall?.copyWith(
                        color: theme.colorScheme.onSurfaceVariant,
                      ),
                    ),
                ],
              ),
            ),
            OutlinedButton(
              onPressed: () => onNavigateToTab(1), // Go to Devices tab
              style: OutlinedButton.styleFrom(
                visualDensity: VisualDensity.compact,
                padding: const EdgeInsets.symmetric(horizontal: 12),
              ),
              child: Text(hasPeer ? 'Manage' : (hasTrusted ? 'Connect' : 'Pair')),
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildScreenShareHeroCard(
    BuildContext context,
    ThemeData theme,
    LinkStatus link,
    dynamic stats,
  ) {
    // Canonical with Screen tab: a live session (any active state) means the
    // control reads STOP; streaming is the narrower "video flowing" fact.
    final sessionActive = controller.session.status.isActive;
    final isStreaming = controller.isSharing;
    final showStop = sessionActive || isStreaming;
    final primaryColor = isStreaming ? Colors.green : theme.colorScheme.primary;

    return Card(
      elevation: 2,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(20),
        side: showStop
            ? const BorderSide(color: Colors.green, width: 2)
            : BorderSide.none,
      ),
      child: Padding(
        padding: const EdgeInsets.all(20.0),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Icon(
                  isStreaming ? Icons.screen_share : Icons.screen_share_outlined,
                  color: primaryColor,
                  size: 28,
                ),
                const SizedBox(width: 10),
                Expanded(
                  child: Text(
                    'Screen Sharing',
                    style: theme.textTheme.titleLarge?.copyWith(
                      fontWeight: FontWeight.bold,
                    ),
                  ),
                ),
                if (isStreaming)
                  Container(
                    padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 4),
                    decoration: BoxDecoration(
                      color: Colors.green.withValues(alpha: 0.15),
                      borderRadius: BorderRadius.circular(12),
                    ),
                    child: Text(
                      _formatDuration(stats.durationUs),
                      style: const TextStyle(
                        color: Colors.green,
                        fontWeight: FontWeight.bold,
                      ),
                    ),
                  )
                else if (sessionActive)
                  Container(
                    padding:
                        const EdgeInsets.symmetric(horizontal: 10, vertical: 4),
                    decoration: BoxDecoration(
                      color: theme.colorScheme.primary
                          .withValues(alpha: 0.12),
                      borderRadius: BorderRadius.circular(12),
                    ),
                    child: Text(
                      link.label.toUpperCase(),
                      style: TextStyle(
                        color: theme.colorScheme.primary,
                        fontWeight: FontWeight.bold,
                        fontSize: 12,
                      ),
                    ),
                  ),
              ],
            ),
            const SizedBox(height: 12),
            Text(
              // A failure or a recovery in progress is worth more than the
              // marketing line: it is the reason the button did not work.
              // When a session is active but not streaming, say so instead of
              // claiming "No session running" on one page and streaming on
              // another.
              link.actionHint.isNotEmpty
                  ? link.actionHint
                  : (isStreaming
                      ? 'Your screen is currently streaming to your connected device.'
                      : (sessionActive
                          ? 'Session ${link.label.toLowerCase()} — ${link.description}'
                          : 'Mirror your screen to the paired device in real-time. Uses the same session as the Screen tab.')),
              style: theme.textTheme.bodyMedium?.copyWith(
                color: theme.colorScheme.onSurfaceVariant,
              ),
            ),
            const SizedBox(height: 8),
            // Wrap, not Row+Spacer: narrow phones stack chips above the
            // Configure action instead of overflowing to the right.
            Wrap(
              spacing: 8,
              runSpacing: 8,
              crossAxisAlignment: WrapCrossAlignment.center,
              children: [
                Chip(
                  // A zero selection is the DEC-022 "device default"
                  // request — worded as the choice it is, never as "0x0".
                  label: Text(controller.selectedWidth == 0
                      ? 'Device default'
                      : '${controller.selectedWidth}x${controller.selectedHeight}'),
                  visualDensity: VisualDensity.compact,
                ),
                if (controller.selectedFps > 0)
                  Chip(
                    label: Text('${controller.selectedFps} fps'),
                    visualDensity: VisualDensity.compact,
                  ),
                TextButton(
                  onPressed: () => onNavigateToTab(2), // Go to Screen tab
                  child: const Text('Configure'),
                ),
              ],
            ),
            const SizedBox(height: 12),
            SizedBox(
              width: double.infinity,
              height: 52,
              child: FilledButton.icon(
                style: FilledButton.styleFrom(
                  backgroundColor: showStop ? theme.colorScheme.error : null,
                  shape: RoundedRectangleBorder(
                    borderRadius: BorderRadius.circular(14),
                  ),
                ),
                onPressed: controller.isLoading
                    ? null
                    : () {
                        if (showStop) {
                          controller.stopScreenSharing();
                        } else {
                          controller.startScreenSharing();
                        }
                      },
                icon: Icon(showStop ? Icons.stop : Icons.play_arrow),
                label: Text(
                  showStop ? 'STOP SHARING' : 'START SHARING',
                  style: const TextStyle(fontWeight: FontWeight.bold, fontSize: 16),
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildClipboardQuickCard(
    BuildContext context,
    ThemeData theme,
    dynamic clipboard,
  ) {
    // Feature-level availability (Active/Manual/Needs setup/…) rather than
    // the adapter's detailed state: one coherent feature reading across
    // Android and Linux. The Clipboard tab keeps the detailed states.
    final availability = clipboardAvailability(clipboard);
    final Color availabilityColor = switch (availability.state) {
      FeatureState.active => Colors.green,
      FeatureState.manual => Colors.amber.shade700,
      FeatureState.needsSetup || FeatureState.restricted => Colors.orange.shade800,
      _ => Colors.grey,
    };
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
          children: [
            Row(
              children: [
                Icon(Icons.content_paste, color: theme.colorScheme.primary),
                const SizedBox(width: 10),
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        'Clipboard Synchronization',
                        style: theme.textTheme.titleMedium?.copyWith(
                          fontWeight: FontWeight.bold,
                        ),
                      ),
                      const SizedBox(height: 2),
                      Text(
                        'Clipboard · ${availability.label}',
                        style: theme.textTheme.bodySmall?.copyWith(
                          color: availabilityColor,
                          fontWeight: FontWeight.w500,
                        ),
                      ),
                    ],
                  ),
                ),
                FilledButton.tonal(
                  onPressed: () => onNavigateToTab(3), // Go to Clipboard tab
                  style: FilledButton.styleFrom(
                    visualDensity: VisualDensity.compact,
                  ),
                  child: const Text('Open'),
                ),
              ],
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildRecentActivitySection(
    BuildContext context,
    ThemeData theme,
    List<ActivityEvent> events,
  ) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Row(
          mainAxisAlignment: MainAxisAlignment.spaceBetween,
          children: [
            Text(
              'Recent Activity',
              style: theme.textTheme.titleMedium?.copyWith(
                fontWeight: FontWeight.bold,
              ),
            ),
            TextButton(
              onPressed: () => onNavigateToTab(4), // Go to Activity tab
              child: const Text('View All'),
            ),
          ],
        ),
        if (events.isEmpty)
          Padding(
            padding: const EdgeInsets.symmetric(vertical: 24.0),
            child: Center(
              child: Text(
                'No recent activity',
                style: theme.textTheme.bodyMedium?.copyWith(
                  color: theme.colorScheme.onSurfaceVariant,
                ),
              ),
            ),
          )
        else
          ...events.map(
            (e) => ListTile(
              contentPadding: EdgeInsets.zero,
              leading: CircleAvatar(
                radius: 18,
                backgroundColor: theme.colorScheme.surfaceContainerHighest,
                child: Icon(
                  _categoryIcon(e.category),
                  size: 18,
                  color: _levelColor(e.level, theme),
                ),
              ),
              title: Text(
                e.title,
                style: const TextStyle(fontWeight: FontWeight.w600, fontSize: 14),
              ),
              subtitle: Text(
                e.detail,
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                style: TextStyle(
                  fontSize: 12,
                  color: theme.colorScheme.onSurfaceVariant,
                ),
              ),
              trailing: Text(
                e.timeAgo,
                style: TextStyle(
                  fontSize: 12,
                  color: theme.colorScheme.onSurfaceVariant,
                ),
              ),
            ),
          ),
      ],
    );
  }

  IconData _categoryIcon(ActivityCategory category) {
    switch (category) {
      case ActivityCategory.screen:
        return Icons.screen_share;
      case ActivityCategory.clipboard:
        return Icons.content_paste;
      case ActivityCategory.pairing:
        return Icons.link;
      case ActivityCategory.connection:
        return Icons.wifi;
      case ActivityCategory.system:
        return Icons.info_outline;
    }
  }

  Color _levelColor(ActivityLevel level, ThemeData theme) {
    switch (level) {
      case ActivityLevel.success:
        return Colors.green;
      case ActivityLevel.warning:
        return Colors.orange;
      case ActivityLevel.error:
        return theme.colorScheme.error;
      case ActivityLevel.info:
        return theme.colorScheme.primary;
    }
  }
}
