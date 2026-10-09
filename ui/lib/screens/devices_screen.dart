import 'package:flutter/material.dart';
import '../controllers/phonebridge_controller.dart';
import '../generated/phonebridge/localipc/v1/local_ipc.pb.dart' as ipc;
import '../models/device_presence.dart';
import '../models/device_row.dart';
import '../ui/pairing_request_dialog.dart';

/// Devices screen (Phases F+G): renders the canonical [DeviceRow] list —
/// exactly one row per deviceId — instead of two overlapping raw lists.
///
/// Sections:
/// - CONNECTED DEVICE hero (authoritative session target) with Disconnect.
/// - Other trusted & available rows with Connect (becomes Switch while live).
/// - Discovered-but-unpaired rows with Pair.
/// - Unavailable (trusted, not on LAN) as info.
/// - Revoked / past devices muted with Re-pair + Forget.
///
/// CONNECTED is derived ONLY from the session target, never trust ordering.
class DevicesScreen extends StatelessWidget {
  const DevicesScreen({
    super.key,
    required this.controller,
  });

  final PhoneBridgeController controller;

  void _startPairingFlow(
    BuildContext context, {
    required String deviceId,
    required String deviceName,
    required String shortId,
  }) async {
    final result = await controller.pairDevice(deviceId);
    if (!context.mounted) return;
    if (result == null) {
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          content: Row(
            children: [
              const Icon(Icons.error_outline, color: Colors.white, size: 18),
              const SizedBox(width: 8),
              Expanded(
                child: Text(
                    controller.lastErrorMessage ?? 'Pairing initiation failed'),
              ),
            ],
          ),
          backgroundColor: Theme.of(context).colorScheme.error,
        ),
      );
      return;
    }
    if (result.alreadyTrusted) {
      // The peer answered 409 already-trusted: nothing to confirm, and no
      // approval dialog may open for a peer both sides already trust.
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          content: Text(
              'Already paired with $deviceName. Trust is already established.'),
          behavior: SnackBarBehavior.floating,
        ),
      );
      return;
    }

    showDialog(
      context: context,
      barrierDismissible: false,
      builder: (ctx) {
        final theme = Theme.of(ctx);
        return AlertDialog(
          icon: Icon(
            Icons.shield_outlined,
            size: 36,
            color: theme.colorScheme.primary,
          ),
          title: const Text(
            'Confirm Pairing Code',
            textAlign: TextAlign.center,
            style: TextStyle(fontWeight: FontWeight.bold),
          ),
          content: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.center,
            children: [
              Text(
                'Pairing with ${result.deviceName} ($shortId)',
                textAlign: TextAlign.center,
                style: theme.textTheme.bodyMedium?.copyWith(
                  color: theme.colorScheme.onSurfaceVariant,
                ),
              ),
              const SizedBox(height: 16),
              Container(
                padding: const EdgeInsets.all(12),
                decoration: BoxDecoration(
                  color: theme.colorScheme.surfaceContainerHighest
                      .withValues(alpha: 0.5),
                  borderRadius: BorderRadius.circular(12),
                ),
                child: Row(
                  children: [
                    Icon(Icons.info_outline,
                        size: 16, color: theme.colorScheme.primary),
                    const SizedBox(width: 8),
                    const Expanded(
                      child: Text(
                        'Verify this 6-digit Short Authentication String (SAS). No code entry is needed on mobile — just ensure the numbers match to confirm zero-trust identity:',
                        style: TextStyle(fontSize: 12),
                      ),
                    ),
                  ],
                ),
              ),
              const SizedBox(height: 16),
              Container(
                padding:
                    const EdgeInsets.symmetric(horizontal: 24, vertical: 12),
                decoration: BoxDecoration(
                  color:
                      theme.colorScheme.primaryContainer.withValues(alpha: 0.7),
                  borderRadius: BorderRadius.circular(16),
                  border: Border.all(
                    color: theme.colorScheme.primary.withValues(alpha: 0.3),
                    width: 1.5,
                  ),
                ),
                child: Text(
                  result.sasCode,
                  style: TextStyle(
                    fontFamily: 'monospace',
                    fontSize: 32,
                    fontWeight: FontWeight.bold,
                    letterSpacing: 6,
                    color: theme.colorScheme.onPrimaryContainer,
                  ),
                ),
              ),
              const SizedBox(height: 10),
              Text(
                'Once confirmed, your devices establish mutual Ed25519 cryptographic trust.',
                textAlign: TextAlign.center,
                style: theme.textTheme.bodySmall?.copyWith(
                  color: theme.colorScheme.onSurfaceVariant,
                ),
              ),
            ],
          ),
          actionsAlignment: MainAxisAlignment.spaceBetween,
          actions: [
            TextButton(
              style: TextButton.styleFrom(
                foregroundColor: theme.colorScheme.error,
              ),
              onPressed: () {
                Navigator.pop(ctx);
                controller.confirmPairing(deviceId: deviceId, confirmed: false);
              },
              child: const Text('REJECT'),
            ),
            FilledButton.icon(
              icon: const Icon(Icons.check, size: 18),
              onPressed: () async {
                Navigator.pop(ctx);
                final ok = await controller.confirmPairing(
                  deviceId: deviceId,
                  confirmed: true,
                );
                if (ok && context.mounted) {
                  ScaffoldMessenger.of(context).showSnackBar(
                    SnackBar(
                      content:
                          Text('Paired with $deviceName. Ready to connect!'),
                      backgroundColor: Colors.green.shade700,
                      behavior: SnackBarBehavior.floating,
                      action: SnackBarAction(
                        label: 'CONNECT NOW',
                        textColor: Colors.white,
                        onPressed: () =>
                            _connectDevice(context, targetDeviceId: deviceId),
                      ),
                    ),
                  );
                } else if (!ok && context.mounted) {
                  ScaffoldMessenger.of(context).showSnackBar(
                    SnackBar(
                      content: Text(
                        controller.lastErrorMessage ??
                            'Pairing was rejected or could not be confirmed',
                      ),
                      backgroundColor: Theme.of(context).colorScheme.error,
                    ),
                  );
                }
              },
              label: const Text('CONFIRM MATCH'),
            ),
          ],
        );
      },
    );
  }

  Future<void> _connectDevice(
    BuildContext context, {
    String? targetDeviceId,
    String? receiverUrl,
  }) async {
    // Composed truth: on Android the live session is the phone's own capture
    // pipeline (no SessionBackend), so controller.session is always idle and a
    // live session would be started as a cold start (SESSION_BUSY) instead of
    // being switched.
    final sessionWasActive = controller.linkStatus.hasSession;
    final ok = sessionWasActive
        ? await controller.switchToDevice(
            targetDeviceId: targetDeviceId,
            receiverUrl: receiverUrl,
          )
        : await controller.startScreenSharing(
            targetDeviceId: targetDeviceId,
            receiverUrl: receiverUrl,
          );
    if (!context.mounted) return;
    if (ok) {
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          content:
              Text(sessionWasActive ? 'Switching connection…' : 'Connecting…'),
          backgroundColor: Theme.of(context).colorScheme.primary,
          behavior: SnackBarBehavior.floating,
        ),
      );
    } else {
      final err = controller.lastErrorMessage ??
          controller.session.lastError ??
          'Failed to connect';
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          content: Text(err),
          backgroundColor: Theme.of(context).colorScheme.error,
          behavior: SnackBarBehavior.floating,
        ),
      );
    }
  }

  Future<void> _confirmSwitch(
      BuildContext context, DeviceRow current, DeviceRow next) async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (dialogCtx) => AlertDialog(
        title: const Text('Switch device?'),
        content: Text(
          'Disconnect from ${current.displayName} and connect to '
          '${next.displayName} instead?',
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(dialogCtx, false),
            child: const Text('CANCEL'),
          ),
          FilledButton(
            onPressed: () => Navigator.pop(dialogCtx, true),
            child: const Text('SWITCH'),
          ),
        ],
      ),
    );
    if (confirmed != true || !context.mounted) return;
    await _connectDevice(
      context,
      targetDeviceId: next.deviceId,
      receiverUrl: next.signalingUrl.isNotEmpty ? next.signalingUrl : null,
    );
  }

  Future<void> _disconnect(BuildContext context) async {
    final ok = await controller.disconnectActiveDevice();
    if (!context.mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(
        content: Text(ok
            ? 'Disconnected.'
            : (controller.lastErrorMessage ?? 'Disconnect failed')),
        backgroundColor:
            ok ? Colors.green.shade700 : Theme.of(context).colorScheme.error,
        behavior: SnackBarBehavior.floating,
      ),
    );
  }

  Future<void> _confirmRemoveDevice(BuildContext context, DeviceRow row) async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (dialogCtx) => AlertDialog(
        icon: const Icon(Icons.delete_outline, size: 36, color: Colors.red),
        title: const Text('Forget Device?'),
        content: Text(
          'Permanently remove "${row.displayName}" (${row.shortId}) from trust store? '
          'You will need to pair again to reconnect.',
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(dialogCtx, false),
            child: const Text('CANCEL'),
          ),
          FilledButton(
            style: FilledButton.styleFrom(
              backgroundColor: Theme.of(context).colorScheme.error,
              foregroundColor: Theme.of(context).colorScheme.onError,
            ),
            onPressed: () => Navigator.pop(dialogCtx, true),
            child: const Text('FORGET'),
          ),
        ],
      ),
    );
    if (confirmed != true || !context.mounted) return;
    final ok = await controller.removeDevice(row.deviceId);
    if (!context.mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(
        content: Text(ok
            ? 'Removed "${row.displayName}" from trust store.'
            : (controller.lastErrorMessage ?? 'Failed to remove device.')),
        behavior: SnackBarBehavior.floating,
      ),
    );
  }

  Future<void> _confirmClearAllRevoked(
      BuildContext context, int count) async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (dialogCtx) => AlertDialog(
        icon: const Icon(Icons.delete_sweep_outlined, size: 36, color: Colors.red),
        title: const Text('Clear All Revoked Devices?'),
        content: Text(
          'This will permanently remove $count past revoked pairing records from your trust store. '
          'Active and trusted devices will not be affected.',
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(dialogCtx, false),
            child: const Text('CANCEL'),
          ),
          FilledButton(
            style: FilledButton.styleFrom(
              backgroundColor: Theme.of(context).colorScheme.error,
              foregroundColor: Theme.of(context).colorScheme.onError,
            ),
            onPressed: () => Navigator.pop(dialogCtx, true),
            child: const Text('CLEAR ALL'),
          ),
        ],
      ),
    );
    if (confirmed != true || !context.mounted) return;
    final removed = await controller.clearAllRevokedDevices();
    if (!context.mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(
        content: Text('Cleared $removed revoked device records.'),
        behavior: SnackBarBehavior.floating,
      ),
    );
  }

  Future<void> _confirmClearDuplicateStale(BuildContext context) async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (dialogCtx) => AlertDialog(
        icon: const Icon(Icons.cleaning_services_outlined, size: 36, color: Colors.amber),
        title: const Text('Clean Up Duplicate Offline Devices?'),
        content: const Text(
          'This will remove stale offline pairings that share their name with an active or available device. '
          'This typically occurs when the PhoneBridge app was reinstalled.',
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(dialogCtx, false),
            child: const Text('CANCEL'),
          ),
          FilledButton(
            onPressed: () => Navigator.pop(dialogCtx, true),
            child: const Text('CLEAN UP'),
          ),
        ],
      ),
    );
    if (confirmed != true || !context.mounted) return;
    final removed = await controller.clearDuplicateStaleDevices();
    if (!context.mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(
        content: Text(removed > 0
            ? 'Cleaned $removed duplicate device record(s).'
            : 'No duplicate records found to remove.'),
        behavior: SnackBarBehavior.floating,
      ),
    );
  }

  bool _hasDuplicateUnavailable(List<DeviceRow> rows) {
    final nameCounts = <String, int>{};
    for (final r in rows) {
      final key = r.displayName.toLowerCase().trim();
      nameCounts[key] = (nameCounts[key] ?? 0) + 1;
    }
    return rows.any((r) =>
        r.group == DeviceRowGroup.unavailable &&
        (nameCounts[r.displayName.toLowerCase().trim()] ?? 0) > 1);
  }

  String _formatDate(int ms) {
    if (ms <= 0) return 'Unknown';
    final dt = DateTime.fromMillisecondsSinceEpoch(ms);
    return '${dt.year}-${dt.month.toString().padLeft(2, '0')}-${dt.day.toString().padLeft(2, '0')} ${dt.hour.toString().padLeft(2, '0')}:${dt.minute.toString().padLeft(2, '0')}';
  }

  void _showDeviceDetails(BuildContext context, DeviceRow row) {
    showModalBottomSheet(
      context: context,
      shape: const RoundedRectangleBorder(
        borderRadius: BorderRadius.vertical(top: Radius.circular(24)),
      ),
      builder: (ctx) {
        final theme = Theme.of(ctx);
        final isLinuxPeer = row.platform.toLowerCase().contains('linux');
        return Padding(
          padding: const EdgeInsets.all(24.0),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Row(
                children: [
                  Container(
                    width: 48,
                    height: 48,
                    decoration: BoxDecoration(
                      color: theme.colorScheme.primaryContainer,
                      borderRadius: BorderRadius.circular(14),
                    ),
                    child: Icon(
                      isLinuxPeer ? Icons.computer : Icons.phone_android,
                      color: theme.colorScheme.onPrimaryContainer,
                    ),
                  ),
                  const SizedBox(width: 14),
                  Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text(
                          row.displayName,
                          style: theme.textTheme.titleMedium?.copyWith(
                            fontWeight: FontWeight.bold,
                          ),
                          overflow: TextOverflow.ellipsis,
                          maxLines: 2,
                        ),
                        Text(
                          row.platform.toUpperCase(),
                          style: theme.textTheme.bodySmall?.copyWith(
                            color: theme.colorScheme.onSurfaceVariant,
                          ),
                        ),
                      ],
                    ),
                  ),
                  _statusBadge(
                    row.revoked
                        ? 'REVOKED'
                        : describeTrust(row.trust).toUpperCase(),
                    row.revoked ? theme.colorScheme.error : Colors.green,
                    icon: row.revoked ? Icons.block : Icons.verified_outlined,
                  ),
                ],
              ),
              const Divider(height: 32),
              _detailRow(theme, 'Device Fingerprint', row.shortId),
              _detailRow(theme, 'Paired On', _formatDate(row.pairedAtMs)),
              _detailRow(theme, 'Last Seen', _formatDate(row.lastSeenMs)),
              const SizedBox(height: 16),
              if (!row.discovered && !row.revoked) ...[
                Container(
                  width: double.infinity,
                  padding: const EdgeInsets.all(12),
                  decoration: BoxDecoration(
                    color: Colors.orange.withValues(alpha: 0.1),
                    borderRadius: BorderRadius.circular(8),
                    border:
                        Border.all(color: Colors.orange.withValues(alpha: 0.3)),
                  ),
                  child: Row(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Icon(Icons.radar_outlined,
                          size: 16, color: Colors.orange.shade800),
                      const SizedBox(width: 8),
                      Expanded(
                        child: Text(
                          'Not currently discovered. Pairing and connect unlock when mDNS sees it again. Ensure PhoneBridge is open on the device on the same Wi-Fi.',
                          style: TextStyle(
                            fontSize: 12,
                            color: Colors.orange.shade800,
                          ),
                        ),
                      ),
                    ],
                  ),
                ),
                const SizedBox(height: 16),
              ],
              if (row.isActiveTarget) ...[
                SizedBox(
                  width: double.infinity,
                  height: 48,
                  child: OutlinedButton.icon(
                    style: OutlinedButton.styleFrom(
                      foregroundColor: theme.colorScheme.error,
                      side: BorderSide(color: theme.colorScheme.error),
                    ),
                    onPressed: controller.isLoading
                        ? null
                        : () async {
                            Navigator.pop(ctx);
                            if (!context.mounted) return;
                            await _disconnect(context);
                          },
                    icon: const Icon(Icons.link_off),
                    label: const Text('DISCONNECT'),
                  ),
                ),
                const SizedBox(height: 12),
              ] else if (row.canConnect) ...[
                SizedBox(
                  width: double.infinity,
                  height: 48,
                  child: FilledButton.icon(
                    onPressed: controller.isLoading
                        ? null
                        : () async {
                            Navigator.pop(ctx);
                            if (!context.mounted) return;
                            final connected = controller.deviceRows
                                .where((r) => r.isActiveTarget)
                                .cast<DeviceRow?>()
                                .fold<DeviceRow?>(null, (p, e) => e);
                            if (connected != null) {
                              await _confirmSwitch(context, connected, row);
                            } else {
                              await _connectDevice(
                                context,
                                targetDeviceId: row.deviceId,
                                receiverUrl: row.signalingUrl.isNotEmpty
                                    ? row.signalingUrl
                                    : null,
                              );
                            }
                          },
                    icon: const Icon(Icons.link),
                    label: Text(controller.linkStatus.hasSession
                        ? 'SWITCH TO THIS DEVICE'
                        : 'CONNECT / START SESSION'),
                  ),
                ),
                const SizedBox(height: 12),
              ],
              if (!row.revoked &&
                  row.trusted &&
                  row.discovered &&
                  !row.isStale &&
                  !row.isActiveTarget) ...[
                SizedBox(
                  width: double.infinity,
                  child: OutlinedButton(
                    onPressed: controller.isLoading
                        ? null
                        : () {
                            Navigator.pop(ctx);
                            _startPairingFlow(
                              context,
                              deviceId: row.deviceId,
                              deviceName: row.displayName,
                              shortId: row.shortId,
                            );
                          },
                    child: const Text('RE-PAIR'),
                  ),
                ),
                const SizedBox(height: 12),
              ],
              Row(
                children: [
                  if (!row.revoked)
                    Expanded(
                      child: OutlinedButton.icon(
                        style: OutlinedButton.styleFrom(
                          foregroundColor: theme.colorScheme.error,
                          side: BorderSide(color: theme.colorScheme.error),
                        ),
                        onPressed: () async {
                          Navigator.pop(ctx);
                          if (!context.mounted) return;
                          final confirmed = await showDialog<bool>(
                            context: context,
                            builder: (dialogCtx) => AlertDialog(
                              title: const Text('Revoke Trust'),
                              content: Text(
                                'Are you sure you want to revoke trust for device '
                                '${row.deviceId}? You will need to pair again to connect.',
                              ),
                              actions: [
                                TextButton(
                                  onPressed: () =>
                                      Navigator.pop(dialogCtx, false),
                                  child: const Text('CANCEL'),
                                ),
                                FilledButton(
                                  style: FilledButton.styleFrom(
                                    backgroundColor:
                                        Theme.of(dialogCtx).colorScheme.error,
                                  ),
                                  onPressed: () =>
                                      Navigator.pop(dialogCtx, true),
                                  child: const Text('REVOKE'),
                                ),
                              ],
                            ),
                          );
                          if (confirmed != true || !context.mounted) return;
                          await controller.revokeDevice(row.deviceId);
                        },
                        icon: const Icon(Icons.block, size: 18),
                        label: const Text('Revoke Trust'),
                      ),
                    ),
                  if (!row.revoked) const SizedBox(width: 12),
                  Expanded(
                    child: FilledButton.icon(
                      style: FilledButton.styleFrom(
                        backgroundColor: theme.colorScheme.errorContainer,
                        foregroundColor: theme.colorScheme.onErrorContainer,
                      ),
                      onPressed: () async {
                        Navigator.pop(ctx);
                        await controller.removeDevice(row.deviceId);
                      },
                      icon: const Icon(Icons.delete_outline, size: 18),
                      label: const Text('Forget Device'),
                    ),
                  ),
                ],
              ),
              const SizedBox(height: 12),
            ],
          ),
        );
      },
    );
  }

  Widget _detailRow(ThemeData theme, String label, String value) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 4.0),
      child: Row(
        mainAxisAlignment: MainAxisAlignment.spaceBetween,
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(label,
              style: TextStyle(color: theme.colorScheme.onSurfaceVariant)),
          const SizedBox(width: 12),
          Flexible(
            child: Text(value,
                style: const TextStyle(
                    fontWeight: FontWeight.bold, fontFamily: 'monospace'),
                textAlign: TextAlign.right,
                overflow: TextOverflow.ellipsis,
                maxLines: 2),
          ),
        ],
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final deviceState = controller.deviceState;
    final isLinux = controller.service.isLinux;
    final rows = controller.deviceRows;

    final connected =
        rows.where((r) => r.group == DeviceRowGroup.connected).toList();
    final available =
        rows.where((r) => r.group == DeviceRowGroup.available).toList();
    final unpaired =
        rows.where((r) => r.group == DeviceRowGroup.untrusted).toList();
    final unavailable =
        rows.where((r) => r.group == DeviceRowGroup.unavailable).toList();
    final revoked =
        rows.where((r) => r.group == DeviceRowGroup.revoked).toList();

    final connectingId = controller.connectingDeviceId;
    final connectingRow = (connectingId != null && connectingId.isNotEmpty)
        ? rows.where((r) => r.deviceId == connectingId).firstOrNull
        : null;

    return RefreshIndicator(
      onRefresh: () async {
        await controller.refreshAll();
        await controller.refreshDiscoveredDevices();
      },
      child: ListView(
        padding: const EdgeInsets.fromLTRB(16, 16, 16, 32),
        children: [
          _buildThisDeviceCard(theme, deviceState, isLinux),
          const SizedBox(height: 20),
          if (controller.pairingRequests.isNotEmpty) ...[
            ...controller.pairingRequests.map(
              (r) => _buildInboundPairingCard(context, theme, r),
            ),
            const SizedBox(height: 20),
          ],
          _sectionHeader(theme, 'Connected device',
              'The one live session — disconnect or switch from here'),
          const SizedBox(height: 10),
          if (connected.isEmpty)
            if (connectingRow != null || controller.isSessionConnecting)
              _buildConnectingCard(
                context,
                theme,
                connectingRow ??
                    DeviceRow(
                      deviceId: controller.connectingDeviceId ?? '',
                      displayName: 'Device',
                      platform: 'Remote',
                    ),
              )
            else
              _buildEmptyConnectedCard(theme)
          else
            ...connected.map((r) => _buildConnectedCard(context, theme, r)),
          const SizedBox(height: 20),
          _sectionHeader(theme, 'Your devices (${available.length})',
              'Trusted · connect to make active'),
          const SizedBox(height: 10),
          if (available.isEmpty)
            _buildEmptyAvailableCard(theme, connected.isNotEmpty)
          else
            ...available.map((r) => _buildAvailableTile(context, theme, r,
                sessionLive: connected.isNotEmpty)),
          const SizedBox(height: 20),
          if (unpaired.isNotEmpty) ...[
            _sectionHeader(theme, 'Available to pair (${unpaired.length})',
                'Discovered on the LAN · not yet trusted'),
            const SizedBox(height: 10),
            ...unpaired.map((r) => _buildUnpairedTile(context, theme, r)),
            const SizedBox(height: 20),
          ],
          if (unavailable.isNotEmpty) ...[
            _sectionHeader(
              theme,
              'Unavailable (${unavailable.length})',
              'Trusted · not currently on the LAN',
              action: _hasDuplicateUnavailable(rows)
                  ? TextButton.icon(
                      style: TextButton.styleFrom(
                        foregroundColor: Colors.amber.shade900,
                        visualDensity: VisualDensity.compact,
                      ),
                      onPressed: () => _confirmClearDuplicateStale(context),
                      icon: const Icon(Icons.cleaning_services_outlined, size: 16),
                      label: const Text('CLEAN DUPLICATES'),
                    )
                  : null,
            ),
            const SizedBox(height: 10),
            ...unavailable.map((r) => _buildUnavailableTile(context, theme, r)),
            const SizedBox(height: 20),
          ],
          if (revoked.isNotEmpty) ...[
            Theme(
              data: theme.copyWith(dividerColor: Colors.transparent),
              child: ExpansionTile(
                initiallyExpanded: true,
                tilePadding: EdgeInsets.zero,
                childrenPadding: EdgeInsets.zero,
                title: _sectionHeader(
                  theme,
                  'Revoked / past devices (${revoked.length})',
                  'Trust revoked · kept for audit, never auto-connected',
                  action: TextButton.icon(
                    style: TextButton.styleFrom(
                      foregroundColor: theme.colorScheme.error,
                      visualDensity: VisualDensity.compact,
                    ),
                    onPressed: () =>
                        _confirmClearAllRevoked(context, revoked.length),
                    icon: const Icon(Icons.delete_sweep_outlined, size: 16),
                    label: const Text('CLEAR ALL'),
                  ),
                ),
                children: [
                  const SizedBox(height: 10),
                  ...revoked.map((r) => _buildRevokedTile(context, theme, r)),
                ],
              ),
            ),
            const SizedBox(height: 20),
          ],
          _buildPairingGuideCard(theme),
        ],
      ),
    );
  }

  Widget _sectionHeader(ThemeData theme, String title, String subtitle,
      {Widget? action}) {
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                title,
                style: theme.textTheme.titleMedium?.copyWith(
                  fontWeight: FontWeight.bold,
                ),
              ),
              Text(
                subtitle,
                style: theme.textTheme.bodySmall?.copyWith(
                  color: theme.colorScheme.onSurfaceVariant,
                ),
              ),
            ],
          ),
        ),
        if (action != null) action,
      ],
    );
  }

  Widget _buildInboundPairingCard(
      BuildContext context, ThemeData theme, dynamic request) {
    return Card(
      margin: const EdgeInsets.only(bottom: 12),
      elevation: 0,
      color: theme.colorScheme.tertiaryContainer.withValues(alpha: 0.35),
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(16),
        side: BorderSide(
          color: theme.colorScheme.tertiary.withValues(alpha: 0.5),
        ),
      ),
      child: Padding(
        padding: const EdgeInsets.all(12.0),
        child: Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Container(
              width: 44,
              height: 44,
              decoration: BoxDecoration(
                color: theme.colorScheme.tertiaryContainer,
                borderRadius: BorderRadius.circular(12),
              ),
              child: Icon(
                Icons.group_add_outlined,
                color: theme.colorScheme.onTertiaryContainer,
              ),
            ),
            const SizedBox(width: 12),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    'Pairing Request · ${request.remoteName}',
                    style: const TextStyle(fontWeight: FontWeight.bold),
                    overflow: TextOverflow.ellipsis,
                    maxLines: 1,
                  ),
                  const SizedBox(height: 4),
                  Text(
                    '${request.platformLabel} device wants to pair with this one. Review the SAS before accepting.',
                    style: TextStyle(
                      fontSize: 12,
                      color: theme.colorScheme.onSurfaceVariant,
                    ),
                  ),
                ],
              ),
            ),
            const SizedBox(width: 8),
            FilledButton.tonalIcon(
              style: FilledButton.styleFrom(
                visualDensity: VisualDensity.compact,
              ),
              onPressed: () => PairingRequestDialog.show(
                context,
                controller: controller,
                request: request,
              ),
              icon: const Icon(Icons.reviews_outlined, size: 16),
              label: const Text('REVIEW'),
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildConnectedCard(
      BuildContext context, ThemeData theme, DeviceRow row) {
    if (controller.isSessionConnecting ||
        controller.session.status.isConnecting) {
      return _buildConnectingCard(context, theme, row);
    }
    // Composed link status, not the raw session state: on Android the phone's
    // own capture pipeline is the session (it does not implement
    // SessionBackend), so reading controller.session here showed
    // SESSION_STATE_DISCONNECTED on a card the same screen labels CONNECTED.
    final linkStatus = controller.linkStatus;
    return Card(
      elevation: 2,
      color: Colors.green.withValues(alpha: 0.08),
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(16),
        side:
            BorderSide(color: Colors.green.withValues(alpha: 0.5), width: 1.5),
      ),
      child: Padding(
        padding: const EdgeInsets.all(16.0),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Container(
                  width: 48,
                  height: 48,
                  decoration: BoxDecoration(
                    color: Colors.green.withValues(alpha: 0.15),
                    borderRadius: BorderRadius.circular(14),
                  ),
                  child: const Icon(Icons.phone_android, color: Colors.green),
                ),
                const SizedBox(width: 14),
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        row.displayName,
                        style: const TextStyle(
                            fontWeight: FontWeight.bold, fontSize: 16),
                        overflow: TextOverflow.ellipsis,
                        maxLines: 1,
                      ),
                      const SizedBox(height: 2),
                      Text(
                        '${row.platform.toUpperCase()} · ${row.shortId}',
                        style: TextStyle(
                          fontSize: 12,
                          color: theme.colorScheme.onSurfaceVariant,
                          fontFamily: 'monospace',
                        ),
                      ),
                    ],
                  ),
                ),
                _connectionDot(theme, isConnected: true),
              ],
            ),
            const SizedBox(height: 8),
            Text(
              'Session state: ${linkStatus.label} · ${linkStatus.phase.name}',
              style: TextStyle(
                fontSize: 12,
                color: theme.colorScheme.onSurfaceVariant,
              ),
            ),
            const SizedBox(height: 12),
            Row(
              children: [
                Expanded(
                  child: OutlinedButton.icon(
                    style: OutlinedButton.styleFrom(
                      foregroundColor: theme.colorScheme.error,
                      side: BorderSide(color: theme.colorScheme.error),
                    ),
                    onPressed: controller.isLoading
                        ? null
                        : () => _disconnect(context),
                    icon: const Icon(Icons.link_off, size: 18),
                    label: const Text('DISCONNECT'),
                  ),
                ),
                const SizedBox(width: 12),
                Expanded(
                  child: OutlinedButton.icon(
                    onPressed: () => _showDeviceDetails(context, row),
                    icon: const Icon(Icons.info_outline, size: 18),
                    label: const Text('DETAILS'),
                  ),
                ),
              ],
            ),
            const SizedBox(height: 8),
            Text(
              'Disconnect stops the session but keeps trust. '
              'To use another phone, tap Connect on it below.',
              style: TextStyle(
                fontSize: 11,
                color: theme.colorScheme.onSurfaceVariant,
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildConnectingCard(
      BuildContext context, ThemeData theme, DeviceRow row) {
    final sessionState = controller.session.status.state;
    final String stateDescription;
    if (sessionState == ipc.SessionState.SESSION_STATE_DISCOVERING) {
      stateDescription = 'Discovering target device on LAN…';
    } else if (sessionState == ipc.SessionState.SESSION_STATE_CONNECTING) {
      stateDescription = 'Negotiating WebRTC session…';
    } else {
      stateDescription = 'Establishing connection…';
    }

    return Card(
      elevation: 2,
      color: Colors.amber.withValues(alpha: 0.08),
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(16),
        side: BorderSide(
          color: Colors.amber.withValues(alpha: 0.6),
          width: 1.5,
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
                  width: 48,
                  height: 48,
                  decoration: BoxDecoration(
                    color: Colors.amber.withValues(alpha: 0.15),
                    borderRadius: BorderRadius.circular(14),
                  ),
                  child: const Center(
                    child: SizedBox(
                      width: 22,
                      height: 22,
                      child: CircularProgressIndicator(
                        strokeWidth: 2.5,
                        color: Colors.amber,
                      ),
                    ),
                  ),
                ),
                const SizedBox(width: 14),
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        'Connecting to ${row.displayName}…',
                        style: const TextStyle(
                          fontWeight: FontWeight.bold,
                          fontSize: 16,
                        ),
                        overflow: TextOverflow.ellipsis,
                        maxLines: 1,
                      ),
                      const SizedBox(height: 2),
                      Text(
                        stateDescription,
                        style: TextStyle(
                          fontSize: 12,
                          color: theme.colorScheme.onSurfaceVariant,
                        ),
                      ),
                    ],
                  ),
                ),
                _statusBadge('CONNECTING…', Colors.amber, icon: Icons.sync),
              ],
            ),
            const SizedBox(height: 8),
            Text(
              'Session state: ${sessionState.name} · ${row.platform.toUpperCase()} (${row.shortId})',
              style: TextStyle(
                fontSize: 11,
                fontFamily: 'monospace',
                color: theme.colorScheme.onSurfaceVariant,
              ),
            ),
            const SizedBox(height: 12),
            Row(
              children: [
                Expanded(
                  child: OutlinedButton.icon(
                    style: OutlinedButton.styleFrom(
                      foregroundColor: theme.colorScheme.error,
                      side: BorderSide(color: theme.colorScheme.error),
                    ),
                    onPressed: () => _disconnect(context),
                    icon: const Icon(Icons.close, size: 18),
                    label: const Text('CANCEL'),
                  ),
                ),
                const SizedBox(width: 12),
                Expanded(
                  child: OutlinedButton.icon(
                    onPressed: () => _showDeviceDetails(context, row),
                    icon: const Icon(Icons.info_outline, size: 18),
                    label: const Text('DETAILS'),
                  ),
                ),
              ],
            ),
            const SizedBox(height: 8),
            Text(
              'Negotiating direct peer connection. Tap Cancel to abort.',
              style: TextStyle(
                fontSize: 11,
                color: theme.colorScheme.onSurfaceVariant,
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildEmptyConnectedCard(ThemeData theme) {
    return Card(
      elevation: 0,
      color: theme.colorScheme.surfaceContainerHighest.withValues(alpha: 0.25),
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(16)),
      child: const Padding(
        padding: EdgeInsets.symmetric(vertical: 20.0, horizontal: 16.0),
        child: Center(
          child: Text(
            'No active connection.\nPick a trusted device below to connect.',
            textAlign: TextAlign.center,
            style: TextStyle(fontSize: 12),
          ),
        ),
      ),
    );
  }

  Widget _buildEmptyAvailableCard(ThemeData theme, bool sessionLive) {
    return Card(
      elevation: 0,
      color: theme.colorScheme.surfaceContainerHighest.withValues(alpha: 0.25),
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(16)),
      child: Padding(
        padding: const EdgeInsets.symmetric(vertical: 20.0, horizontal: 16.0),
        child: Center(
          child: Text(
            sessionLive
                ? 'No other trusted devices available right now.'
                : 'No trusted devices available.\nPair a discovered device first.',
            textAlign: TextAlign.center,
            style: const TextStyle(fontSize: 12),
          ),
        ),
      ),
    );
  }

  Widget _buildAvailableTile(
      BuildContext context, ThemeData theme, DeviceRow row,
      {required bool sessionLive}) {
    final isThisConnecting = controller.isConnectingTo(row.deviceId);
    final isAnyConnecting = controller.isSessionConnecting;

    return Card(
      margin: const EdgeInsets.only(bottom: 10),
      elevation: 0,
      color: theme.colorScheme.surfaceContainerHighest.withValues(alpha: 0.35),
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(16),
        side: BorderSide(
          color: theme.colorScheme.outlineVariant.withValues(alpha: 0.4),
        ),
      ),
      child: InkWell(
        borderRadius: BorderRadius.circular(16),
        onTap: () => _showDeviceDetails(context, row),
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
          child: Row(
            children: [
              Container(
                width: 40,
                height: 40,
                decoration: BoxDecoration(
                  color: isThisConnecting
                      ? Colors.amber.withValues(alpha: 0.15)
                      : theme.colorScheme.primaryContainer,
                  borderRadius: BorderRadius.circular(12),
                ),
                child: Icon(
                  row.platform.toLowerCase().contains('linux')
                      ? Icons.computer
                      : Icons.phone_android,
                  color: isThisConnecting
                      ? Colors.amber
                      : theme.colorScheme.onPrimaryContainer,
                ),
              ),
              const SizedBox(width: 12),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Row(
                      children: [
                        Flexible(
                          child: Text(
                            row.displayName,
                            style: const TextStyle(fontWeight: FontWeight.bold),
                            overflow: TextOverflow.ellipsis,
                            maxLines: 1,
                          ),
                        ),
                        if (row.disambiguationTag(controller.deviceRows).isNotEmpty) ...[
                          const SizedBox(width: 6),
                          Container(
                            padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
                            decoration: BoxDecoration(
                              color: theme.colorScheme.surfaceContainerHighest,
                              borderRadius: BorderRadius.circular(6),
                            ),
                            child: Text(
                              row.disambiguationTag(controller.deviceRows),
                              style: theme.textTheme.labelSmall?.copyWith(
                                fontSize: 10,
                                color: theme.colorScheme.onSurfaceVariant,
                              ),
                            ),
                          ),
                        ],
                      ],
                    ),
                    const SizedBox(height: 2),
                    Text(
                      '${row.platform.toUpperCase()} · ${connectionPathLabel(isStale: row.isStale)}',
                      style: TextStyle(
                        fontSize: 12,
                        color: theme.colorScheme.onSurfaceVariant,
                      ),
                    ),
                    const SizedBox(height: 6),
                    Wrap(
                      spacing: 6,
                      runSpacing: 6,
                      children: [
                        if (isThisConnecting)
                          _statusBadge('CONNECTING…', Colors.amber,
                              icon: Icons.sync)
                        else ...[
                          _statusBadge('TRUSTED', Colors.green,
                              icon: Icons.verified_outlined),
                          _connectionDot(theme, isConnected: false),
                        ],
                      ],
                    ),
                  ],
                ),
              ),
              const SizedBox(width: 8),
              if (isThisConnecting)
                FilledButton(
                  style: FilledButton.styleFrom(
                    visualDensity: VisualDensity.compact,
                    backgroundColor: Colors.amber.shade700,
                  ),
                  onPressed: null,
                  child: const Row(
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      SizedBox(
                        width: 12,
                        height: 12,
                        child: CircularProgressIndicator(
                          strokeWidth: 2,
                          color: Colors.white,
                        ),
                      ),
                      SizedBox(width: 6),
                      Text('CONNECTING…'),
                    ],
                  ),
                )
              else
                FilledButton(
                  style: FilledButton.styleFrom(
                      visualDensity: VisualDensity.compact),
                  onPressed: (controller.isLoading || isAnyConnecting)
                      ? null
                      : () async {
                          if (sessionLive) {
                            final connected = controller.deviceRows
                                .where((r) => r.isActiveTarget)
                                .cast<DeviceRow?>()
                                .fold<DeviceRow?>(null, (p, e) => e);
                            if (connected != null) {
                              await _confirmSwitch(context, connected, row);
                              return;
                            }
                          }
                          await _connectDevice(
                            context,
                            targetDeviceId: row.deviceId,
                            receiverUrl: row.signalingUrl.isNotEmpty
                                ? row.signalingUrl
                                : null,
                          );
                        },
                  child: Text(sessionLive ? 'SWITCH' : 'CONNECT'),
                ),
            ],
          ),
        ),
      ),
    );
  }

  Widget _buildUnpairedTile(
      BuildContext context, ThemeData theme, DeviceRow row) {
    final isPairing = controller.outboundPairingDeviceId == row.deviceId;
    return Card(
      margin: const EdgeInsets.only(bottom: 10),
      elevation: 0,
      color: theme.colorScheme.surfaceContainerHighest.withValues(alpha: 0.35),
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(16),
        side: BorderSide(
          color: theme.colorScheme.outlineVariant.withValues(alpha: 0.4),
        ),
      ),
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
        child: Row(
          children: [
            Container(
              width: 40,
              height: 40,
              decoration: BoxDecoration(
                color: theme.colorScheme.primaryContainer,
                borderRadius: BorderRadius.circular(12),
              ),
              child: Icon(
                Icons.phone_android,
                color: theme.colorScheme.onPrimaryContainer,
              ),
            ),
            const SizedBox(width: 12),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    row.displayName,
                    style: const TextStyle(fontWeight: FontWeight.bold),
                    overflow: TextOverflow.ellipsis,
                    maxLines: 1,
                  ),
                  const SizedBox(height: 2),
                  Text(
                    '${row.model.isNotEmpty ? '${row.model} · ' : ''}${connectionPathLabel(isStale: row.isStale)}',
                    style: TextStyle(
                      fontSize: 12,
                      color: theme.colorScheme.onSurfaceVariant,
                    ),
                  ),
                  const SizedBox(height: 6),
                  Wrap(
                    spacing: 6,
                    runSpacing: 6,
                    children: [
                      _statusBadge('UNPAIRED', Colors.amber.shade700,
                          icon: Icons.link_off),
                      if (isPairing)
                        _statusBadge('PAIRING', Colors.amber.shade800,
                            icon: Icons.sync_outlined),
                      if (row.isStale)
                        _statusBadge('STALE', Colors.orange,
                            icon: Icons.access_time),
                    ],
                  ),
                ],
              ),
            ),
            const SizedBox(width: 8),
            if (!row.isStale)
              FilledButton.tonal(
                style: FilledButton.styleFrom(
                    visualDensity: VisualDensity.compact),
                onPressed: controller.isLoading
                    ? null
                    : () => _startPairingFlow(
                          context,
                          deviceId: row.deviceId,
                          deviceName: row.displayName,
                          shortId: row.shortId,
                        ),
                child: const Text('PAIR'),
              ),
          ],
        ),
      ),
    );
  }

  Widget _buildUnavailableTile(
      BuildContext context, ThemeData theme, DeviceRow row) {
    return Card(
      margin: const EdgeInsets.only(bottom: 10),
      elevation: 0,
      color: theme.colorScheme.surfaceContainerHighest.withValues(alpha: 0.25),
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(16),
        side: BorderSide(
          color: theme.colorScheme.outlineVariant.withValues(alpha: 0.3),
        ),
      ),
      child: InkWell(
        borderRadius: BorderRadius.circular(16),
        onTap: () => _showDeviceDetails(context, row),
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
          child: Row(
            children: [
              Container(
                width: 40,
                height: 40,
                decoration: BoxDecoration(
                  color: theme.colorScheme.surfaceContainerHighest,
                  borderRadius: BorderRadius.circular(12),
                ),
                child: Icon(
                  Icons.phone_android,
                  color: theme.colorScheme.onSurfaceVariant,
                ),
              ),
              const SizedBox(width: 12),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Row(
                      children: [
                        Flexible(
                          child: Text(
                            row.displayName,
                            style: const TextStyle(fontWeight: FontWeight.bold),
                            overflow: TextOverflow.ellipsis,
                            maxLines: 1,
                          ),
                        ),
                        if (row.disambiguationTag(controller.deviceRows).isNotEmpty) ...[
                          const SizedBox(width: 6),
                          Container(
                            padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
                            decoration: BoxDecoration(
                              color: theme.colorScheme.surfaceContainerHighest,
                              borderRadius: BorderRadius.circular(6),
                            ),
                            child: Text(
                              row.disambiguationTag(controller.deviceRows),
                              style: theme.textTheme.labelSmall?.copyWith(
                                fontSize: 10,
                                color: theme.colorScheme.onSurfaceVariant,
                              ),
                            ),
                          ),
                        ],
                      ],
                    ),
                    const SizedBox(height: 6),
                    Wrap(
                      spacing: 6,
                      runSpacing: 6,
                      children: [
                        _statusBadge('TRUSTED', Colors.green,
                            icon: Icons.verified_outlined),
                        _connectionDot(theme, isConnected: false),
                        _statusBadge('NOT DISCOVERED', Colors.orange,
                            icon: Icons.radar_outlined),
                      ],
                    ),
                    const SizedBox(height: 6),
                    Text(
                      'Not currently discovered · actions unlock when the LAN sees it again.',
                      style: TextStyle(
                        fontSize: 11,
                        color: Colors.orange.shade800,
                        fontWeight: FontWeight.w500,
                      ),
                    ),
                  ],
                ),
              ),
              const SizedBox(width: 8),
              const Icon(Icons.chevron_right),
            ],
          ),
        ),
      ),
    );
  }

  Widget _buildRevokedTile(
      BuildContext context, ThemeData theme, DeviceRow row) {
    return Card(
      margin: const EdgeInsets.only(bottom: 10),
      elevation: 0,
      color: theme.colorScheme.surfaceContainerHighest.withValues(alpha: 0.25),
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(16),
        side: BorderSide(
          color: theme.colorScheme.error.withValues(alpha: 0.3),
        ),
      ),
      child: InkWell(
        borderRadius: BorderRadius.circular(16),
        onTap: () => _showDeviceDetails(context, row),
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
          child: Row(
            children: [
              Container(
                width: 40,
                height: 40,
                decoration: BoxDecoration(
                  color:
                      theme.colorScheme.errorContainer.withValues(alpha: 0.4),
                  borderRadius: BorderRadius.circular(12),
                ),
                child: Icon(
                  Icons.phone_android,
                  color: theme.colorScheme.error,
                ),
              ),
              const SizedBox(width: 12),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      row.displayName,
                      style: const TextStyle(
                        fontWeight: FontWeight.bold,
                        decoration: TextDecoration.lineThrough,
                      ),
                      overflow: TextOverflow.ellipsis,
                      maxLines: 1,
                    ),
                    const SizedBox(height: 6),
                    Wrap(
                      spacing: 6,
                      runSpacing: 6,
                      children: [
                        _statusBadge('REVOKED', theme.colorScheme.error,
                            icon: Icons.block),
                        if (!row.discovered)
                          _statusBadge('NOT DISCOVERED', Colors.orange,
                              icon: Icons.radar_outlined),
                      ],
                    ),
                    if (!row.discovered) ...[
                      const SizedBox(height: 6),
                      Text(
                        'Not currently discovered · re-pair unlocks when the LAN sees it again.',
                        style: TextStyle(
                          fontSize: 11,
                          color: Colors.orange.shade800,
                          fontWeight: FontWeight.w500,
                        ),
                      ),
                    ],
                  ],
                ),
              ),
              Row(
                mainAxisSize: MainAxisSize.min,
                children: [
                  if (row.discovered && !row.isStale) ...[
                    FilledButton.tonal(
                      style: FilledButton.styleFrom(
                          visualDensity: VisualDensity.compact),
                      onPressed: controller.isLoading
                          ? null
                          : () => _startPairingFlow(
                                context,
                                deviceId: row.deviceId,
                                deviceName: row.displayName,
                                shortId: row.shortId,
                              ),
                      child: const Text('RE-PAIR'),
                    ),
                    const SizedBox(width: 4),
                  ],
                  IconButton(
                    icon: const Icon(Icons.delete_outline, size: 20),
                    tooltip: 'Forget device',
                    color: theme.colorScheme.error,
                    onPressed: controller.isLoading
                        ? null
                        : () => _confirmRemoveDevice(context, row),
                  ),
                ],
              ),
            ],
          ),
        ),
      ),
    );
  }

  Widget _statusBadge(String label, Color color, {IconData? icon}) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 7, vertical: 3),
      decoration: BoxDecoration(
        color: color.withValues(alpha: 0.15),
        borderRadius: BorderRadius.circular(8),
        border: Border.all(color: color.withValues(alpha: 0.3), width: 1),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          if (icon != null) ...[
            Icon(icon, size: 10, color: color),
            const SizedBox(width: 4),
          ],
          Text(
            label,
            style: TextStyle(
              fontSize: 10,
              fontWeight: FontWeight.bold,
              letterSpacing: 0.4,
              color: color,
            ),
          ),
        ],
      ),
    );
  }

  /// Connection dot — the visual counterpart to the trust shield. Trust says
  /// who the device is; this says whether we are talking to it now.
  Widget _connectionDot(ThemeData theme, {required bool isConnected}) {
    final color = isConnected ? Colors.green : Colors.grey;
    final label = isConnected ? 'CONNECTED' : 'OFFLINE';
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 7, vertical: 3),
      decoration: BoxDecoration(
        color: color.withValues(alpha: 0.12),
        borderRadius: BorderRadius.circular(8),
        border: Border.all(color: color.withValues(alpha: 0.3), width: 1),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Container(
            width: 7,
            height: 7,
            decoration: BoxDecoration(shape: BoxShape.circle, color: color),
          ),
          const SizedBox(width: 5),
          Text(
            label,
            style: TextStyle(
              fontSize: 10,
              fontWeight: FontWeight.bold,
              letterSpacing: 0.4,
              color: color,
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildThisDeviceCard(
      ThemeData theme, dynamic deviceState, bool isLinux) {
    return Card(
      elevation: 0,
      color: theme.colorScheme.surfaceContainerHighest.withValues(alpha: 0.3),
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(16),
        side: BorderSide(
            color: theme.colorScheme.outlineVariant.withValues(alpha: 0.4)),
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
                borderRadius: BorderRadius.circular(14),
              ),
              child: Icon(
                isLinux
                    ? Icons.desktop_windows_outlined
                    : Icons.phone_android_outlined,
                color: theme.colorScheme.onPrimaryContainer,
                size: 24,
              ),
            ),
            const SizedBox(width: 14),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    'This Device (${deviceState.displayName})',
                    style: theme.textTheme.titleSmall?.copyWith(
                      fontWeight: FontWeight.bold,
                    ),
                  ),
                  const SizedBox(height: 2),
                  Text(
                    isLinux
                        ? deviceState.manufacturer
                        : '${deviceState.manufacturer} ${deviceState.model} · Android API ${deviceState.sdkInt}',
                    style: theme.textTheme.bodySmall?.copyWith(
                      color: theme.colorScheme.onSurfaceVariant,
                    ),
                  ),
                ],
              ),
            ),
            Container(
              padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 4),
              decoration: BoxDecoration(
                color: Colors.green.withValues(alpha: 0.15),
                borderRadius: BorderRadius.circular(8),
                border: Border.all(color: Colors.green.withValues(alpha: 0.3)),
              ),
              child: const Row(
                mainAxisSize: MainAxisSize.min,
                children: [
                  Icon(Icons.wifi_tethering, size: 12, color: Colors.green),
                  SizedBox(width: 4),
                  Text(
                    'Discoverable',
                    style: TextStyle(
                      color: Colors.green,
                      fontSize: 11,
                      fontWeight: FontWeight.bold,
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

  Widget _buildPairingGuideCard(ThemeData theme) {
    return Card(
      elevation: 0,
      color: theme.colorScheme.surfaceContainerHighest.withValues(alpha: 0.2),
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(16),
        side: BorderSide(
            color: theme.colorScheme.outlineVariant.withValues(alpha: 0.3)),
      ),
      child: Padding(
        padding: const EdgeInsets.all(16.0),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Icon(Icons.security,
                    size: 18, color: theme.colorScheme.primary),
                const SizedBox(width: 8),
                Expanded(
                  child: Text(
                    'Zero-Trust Pairing Guide',
                    style: theme.textTheme.titleSmall
                        ?.copyWith(fontWeight: FontWeight.bold),
                    overflow: TextOverflow.ellipsis,
                    maxLines: 1,
                  ),
                ),
              ],
            ),
            const SizedBox(height: 12),
            _guideStep(theme, '1',
                'Connect both devices to the same LAN or Wi-Fi network.'),
            _guideStep(theme, '2',
                'Tap "Pair" or "Re-Pair" on either device — both sides can start the pairing.'),
            _guideStep(theme, '3',
                'Verify that the 6-digit SAS code matches, then the receiving device accepts the request.'),
          ],
        ),
      ),
    );
  }

  Widget _guideStep(ThemeData theme, String num, String text) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 8.0),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Container(
            width: 20,
            height: 20,
            alignment: Alignment.center,
            decoration: BoxDecoration(
              color: theme.colorScheme.primary.withValues(alpha: 0.15),
              shape: BoxShape.circle,
            ),
            child: Text(
              num,
              style: TextStyle(
                fontSize: 11,
                fontWeight: FontWeight.bold,
                color: theme.colorScheme.primary,
              ),
            ),
          ),
          const SizedBox(width: 10),
          Expanded(
            child: Text(
              text,
              style: TextStyle(
                fontSize: 12,
                color: theme.colorScheme.onSurfaceVariant,
              ),
            ),
          ),
        ],
      ),
    );
  }
}
