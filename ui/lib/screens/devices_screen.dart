import 'package:flutter/material.dart';
import '../controllers/phonebridge_controller.dart';
import '../models/trusted_device.dart';
import '../models/discovered_device.dart';

class DevicesScreen extends StatelessWidget {
  const DevicesScreen({
    super.key,
    required this.controller,
  });

  final PhoneBridgeController controller;

  void _startPairingFlow(BuildContext context, DiscoveredDevice dev) async {
    final result = await controller.pairDevice(dev.id);
    if (!context.mounted) return;
    if (result == null) {
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          content: Row(
            children: [
              const Icon(Icons.error_outline, color: Colors.white, size: 18),
              const SizedBox(width: 8),
              Expanded(
                child: Text(controller.lastErrorMessage ?? 'Pairing initiation failed'),
              ),
            ],
          ),
          backgroundColor: Theme.of(context).colorScheme.error,
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
                'Pairing with ${result.deviceName} (${dev.shortId})',
                textAlign: TextAlign.center,
                style: theme.textTheme.bodyMedium?.copyWith(
                  color: theme.colorScheme.onSurfaceVariant,
                ),
              ),
              const SizedBox(height: 16),
              Container(
                padding: const EdgeInsets.all(12),
                decoration: BoxDecoration(
                  color: theme.colorScheme.surfaceContainerHighest.withValues(alpha: 0.5),
                  borderRadius: BorderRadius.circular(12),
                ),
                child: Row(
                  children: [
                    Icon(Icons.info_outline, size: 16, color: theme.colorScheme.primary),
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
                padding: const EdgeInsets.symmetric(horizontal: 24, vertical: 12),
                decoration: BoxDecoration(
                  color: theme.colorScheme.primaryContainer.withValues(alpha: 0.7),
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
                controller.confirmPairing(deviceId: dev.id, confirmed: false);
              },
              child: const Text('REJECT'),
            ),
            FilledButton.icon(
              icon: const Icon(Icons.check, size: 18),
              onPressed: () async {
                Navigator.pop(ctx);
                final ok = await controller.confirmPairing(
                  deviceId: dev.id,
                  confirmed: true,
                );
                if (ok && context.mounted) {
                  ScaffoldMessenger.of(context).showSnackBar(
                    SnackBar(
                      content: Text('Paired with ${dev.name}. Ready to connect!'),
                      backgroundColor: Colors.green.shade700,
                      behavior: SnackBarBehavior.floating,
                      action: SnackBarAction(
                        label: 'CONNECT NOW',
                        textColor: Colors.white,
                        onPressed: () => _connectDevice(context, dev),
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

  Future<void> _connectDevice(BuildContext context, DiscoveredDevice dev) async {
    // Linux hands the daemon a device id and the daemon resolves the endpoint
    // from its own discovery registry. The phone owns its session, so it must
    // dial the peer's signaling server itself from the discovered endpoint.
    final isLinux = controller.service.isLinux;
    final ok = await controller.startScreenSharing(
      targetDeviceId: isLinux ? dev.id : null,
      receiverUrl: isLinux ? null : dev.signalingUrl,
    );
    if (!context.mounted) return;
    if (ok) {
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          content: Text('Connecting to ${dev.name}...'),
          backgroundColor: Theme.of(context).colorScheme.primary,
          behavior: SnackBarBehavior.floating,
        ),
      );
    } else {
      final err = controller.lastErrorMessage ?? controller.session.lastError ?? 'Failed to connect';
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          content: Text(err),
          backgroundColor: Theme.of(context).colorScheme.error,
          behavior: SnackBarBehavior.floating,
        ),
      );
    }
  }

  String _formatDate(int ms) {
    if (ms <= 0) return 'Unknown';
    final dt = DateTime.fromMillisecondsSinceEpoch(ms);
    return '${dt.year}-${dt.month.toString().padLeft(2, '0')}-${dt.day.toString().padLeft(2, '0')} ${dt.hour.toString().padLeft(2, '0')}:${dt.minute.toString().padLeft(2, '0')}';
  }

  void _showDeviceDetails(BuildContext context, TrustedDevice device) {
    showModalBottomSheet(
      context: context,
      shape: const RoundedRectangleBorder(
        borderRadius: BorderRadius.vertical(top: Radius.circular(24)),
      ),
      builder: (ctx) {
        final theme = Theme.of(ctx);
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
                      device.isLinux ? Icons.computer : Icons.phone_android,
                      color: theme.colorScheme.onPrimaryContainer,
                    ),
                  ),
                  const SizedBox(width: 14),
                  Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text(
                          device.displayName,
                          style: theme.textTheme.titleMedium?.copyWith(
                            fontWeight: FontWeight.bold,
                          ),
                        ),
                        Text(
                          device.platform.toUpperCase(),
                          style: theme.textTheme.bodySmall?.copyWith(
                            color: theme.colorScheme.onSurfaceVariant,
                          ),
                        ),
                      ],
                    ),
                  ),
                  _statusBadge(
                    device.revoked ? 'REVOKED' : 'TRUSTED',
                    device.revoked ? theme.colorScheme.error : Colors.green,
                    icon: device.revoked ? Icons.block : Icons.verified_outlined,
                  ),
                ],
              ),
              const Divider(height: 32),
              _detailRow(theme, 'Device Fingerprint', device.shortId),
              _detailRow(theme, 'Paired On', _formatDate(device.pairedAtMs)),
              _detailRow(theme, 'Last Seen', _formatDate(device.lastSeenMs)),
              const SizedBox(height: 16),
              if (!controller.discoveredDevices.any((d) => d.id == device.deviceId && !d.isStale)) ...[
                Container(
                  width: double.infinity,
                  padding: const EdgeInsets.all(12),
                  decoration: BoxDecoration(
                    color: Colors.orange.withValues(alpha: 0.1),
                    borderRadius: BorderRadius.circular(8),
                    border: Border.all(color: Colors.orange.withValues(alpha: 0.3)),
                  ),
                  child: Row(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Icon(Icons.radar_outlined, size: 16, color: Colors.orange.shade800),
                      const SizedBox(width: 8),
                      Expanded(
                        child: Text(
                          'Waiting for device / Not currently discovered. Pairing cannot start until mDNS discovery succeeds. Ensure PhoneBridge is open on the device on the same Wi-Fi.',
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
              if (!device.revoked && controller.discoveredDevices.any((d) => d.id == device.deviceId && !d.isStale)) ...[
                SizedBox(
                  width: double.infinity,
                  height: 48,
                  child: FilledButton.icon(
                    onPressed: controller.isLoading
                        ? null
                        : () async {
                            Navigator.pop(ctx);
                            await controller.startScreenSharing(targetDeviceId: device.deviceId);
                          },
                    icon: const Icon(Icons.link),
                    label: const Text('CONNECT / START SESSION'),
                  ),
                ),
                const SizedBox(height: 12),
              ],
              Row(
                children: [
                  if (!device.revoked)
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
                                '${device.deviceId}? You will need to pair again to connect.',
                              ),
                              actions: [
                                TextButton(
                                  onPressed: () => Navigator.pop(dialogCtx, false),
                                  child: const Text('CANCEL'),
                                ),
                                FilledButton(
                                  style: FilledButton.styleFrom(
                                    backgroundColor:
                                        Theme.of(dialogCtx).colorScheme.error,
                                  ),
                                  onPressed: () => Navigator.pop(dialogCtx, true),
                                  child: const Text('REVOKE'),
                                ),
                              ],
                            ),
                          );
                          if (confirmed != true || !context.mounted) return;
                          await controller.revokeDevice(device.deviceId);
                        },
                        icon: const Icon(Icons.block, size: 18),
                        label: const Text('Revoke Trust'),
                      ),
                    ),
                  if (!device.revoked) const SizedBox(width: 12),
                  Expanded(
                    child: FilledButton.icon(
                      style: FilledButton.styleFrom(
                        backgroundColor: theme.colorScheme.errorContainer,
                        foregroundColor: theme.colorScheme.onErrorContainer,
                      ),
                      onPressed: () async {
                        Navigator.pop(ctx);
                        await controller.removeDevice(device.deviceId);
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
        children: [
          Text(label, style: TextStyle(color: theme.colorScheme.onSurfaceVariant)),
          Text(value, style: const TextStyle(fontWeight: FontWeight.bold, fontFamily: 'monospace')),
        ],
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final trusted = controller.trustedDevices;
    final deviceState = controller.deviceState;
    final isLinux = controller.service.isLinux;

    return RefreshIndicator(
      onRefresh: () async {
        await controller.refreshAll();
        await controller.refreshDiscoveredDevices();
      },
      child: ListView(
        padding: const EdgeInsets.all(16.0),
        children: [
          _buildThisDeviceCard(theme, deviceState, isLinux),
          const SizedBox(height: 20),
          // Discovery results are rendered on every platform. On Linux the
          // daemon browses the LAN for phones; on Android the phone browses
          // through the Go core, which is the only way the desktop can appear
          // here at all. Suppressing the section behind `if (isLinux)` made the
          // phone's own browse invisible even once it existed.
          ...[
            Row(
              mainAxisAlignment: MainAxisAlignment.spaceBetween,
              children: [
                Text(
                  'Discovered Devices (${controller.discoveredDevices.length})',
                  style: theme.textTheme.titleMedium?.copyWith(
                    fontWeight: FontWeight.bold,
                  ),
                ),
                TextButton.icon(
                  onPressed: controller.isLoading
                      ? null
                      : () => controller.refreshDiscoveredDevices(),
                  icon: const Icon(Icons.search, size: 16),
                  label: const Text('Scan'),
                ),
              ],
            ),
            const SizedBox(height: 10),
            if (controller.discoveredDevices.isEmpty)
              Card(
                elevation: 0,
                color: theme.colorScheme.surfaceContainerHighest.withValues(alpha: 0.25),
                shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(16)),
                child: Padding(
                  padding: const EdgeInsets.symmetric(vertical: 24.0, horizontal: 16.0),
                  child: Center(
                    child: Column(
                      mainAxisSize: MainAxisSize.min,
                      children: [
                        Icon(
                          Icons.radar_outlined,
                          size: 36,
                          color: theme.colorScheme.onSurfaceVariant.withValues(alpha: 0.6),
                        ),
                        const SizedBox(height: 10),
                        Text(
                          isLinux
                              ? 'No new PhoneBridge devices discovered yet.\nEnsure Android PhoneBridge is running on the same LAN.'
                              : 'No PhoneBridge computer discovered yet.\nEnsure PhoneBridge is running on the PC and both devices are on the same Wi-Fi.',
                          textAlign: TextAlign.center,
                          style: const TextStyle(fontSize: 12),
                        ),
                      ],
                    ),
                  ),
                ),
              )
            else
              ...controller.discoveredDevices
                  .map((d) => _buildDiscoveredDeviceTile(context, theme, d)),
            const SizedBox(height: 20),
          ],
          Row(
            mainAxisAlignment: MainAxisAlignment.spaceBetween,
            children: [
              Text(
                'Trusted Devices (${trusted.length})',
                style: theme.textTheme.titleMedium?.copyWith(
                  fontWeight: FontWeight.bold,
                ),
              ),
            ],
          ),
          const SizedBox(height: 10),
          if (trusted.isEmpty)
            _buildEmptyTrustedCard(theme)
          else
            ...trusted.map((dev) => _buildDeviceTile(context, theme, dev)),
          const SizedBox(height: 20),
          _buildPairingGuideCard(theme),
        ],
      ),
    );
  }

  Widget _buildDiscoveredDeviceTile(
      BuildContext context, ThemeData theme, DiscoveredDevice dev) {
    // Distinct state resolution:
    // 1. isKnown: Device present in local trust store (active or revoked).
    // 2. isTrusted: Local trust store holds an active, non-revoked record.
    // 3. isRevoked: Local trust was revoked.
    // 4. isConnected: Session is actively established with this device.
    // 5. isStale: mDNS record has aged out.
    final isKnown = controller.trustedDevices
        .any((t) => t.deviceId == dev.id);
    final isRevoked = controller.trustedDevices
        .any((t) => t.deviceId == dev.id && t.revoked);
    final isTrusted = controller.trustedDevices
        .any((t) => t.deviceId == dev.id && !t.revoked);
    final isStale = dev.isStale;
    final isConnected = controller.session.status.isActive &&
        controller.activePeer?.deviceId == dev.id;

    final Color statusColor;
    final String statusLabel;
    final IconData statusIcon;

    if (isConnected) {
      statusColor = Colors.green;
      statusLabel = 'CONNECTED';
      statusIcon = Icons.link;
    } else if (isRevoked) {
      statusColor = theme.colorScheme.error;
      statusLabel = 'REVOKED';
      statusIcon = Icons.block;
    } else if (isTrusted) {
      statusColor = theme.colorScheme.primary;
      statusLabel = 'TRUSTED';
      statusIcon = Icons.verified_user_outlined;
    } else {
      statusColor = Colors.amber.shade700;
      statusLabel = 'UNPAIRED';
      statusIcon = Icons.link_off;
    }

    Widget? actionButton;
    if (isStale) {
      actionButton = null;
    } else if (isKnown) {
      if (isConnected) {
        actionButton = Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            FilledButton.tonalIcon(
              style: FilledButton.styleFrom(
                backgroundColor: Colors.green.withValues(alpha: 0.15),
                foregroundColor: Colors.green.shade700,
              ),
              onPressed: null,
              icon: const Icon(Icons.check_circle_outline, size: 16),
              label: const Text('CONNECTED'),
            ),
            const SizedBox(width: 8),
            OutlinedButton(
              onPressed: controller.isLoading
                  ? null
                  : () => _startPairingFlow(context, dev),
              child: const Text('RE-PAIR'),
            ),
          ],
        );
      } else {
        actionButton = Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            FilledButton.icon(
              onPressed: controller.isLoading
                  ? null
                  : () => _connectDevice(context, dev),
              icon: const Icon(Icons.link, size: 16),
              label: const Text('CONNECT'),
            ),
            const SizedBox(width: 8),
            OutlinedButton(
              onPressed: controller.isLoading
                  ? null
                  : () => _startPairingFlow(context, dev),
              child: const Text('RE-PAIR'),
            ),
          ],
        );
      }
    } else {
      actionButton = FilledButton.tonal(
        onPressed: controller.isLoading
            ? null
            : () => _startPairingFlow(context, dev),
        child: const Text('PAIR'),
      );
    }

    return Card(
      margin: const EdgeInsets.only(bottom: 12),
      elevation: 0,
      color: theme.colorScheme.surfaceContainerHighest.withValues(alpha: 0.35),
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(16),
        side: BorderSide(
          color: isConnected
              ? Colors.green.withValues(alpha: 0.5)
              : theme.colorScheme.outlineVariant.withValues(alpha: 0.4),
        ),
      ),
      child: Padding(
        padding: const EdgeInsets.all(12.0),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            ListTile(
              contentPadding: EdgeInsets.zero,
              leading: Container(
                width: 44,
                height: 44,
                decoration: BoxDecoration(
                  color: isConnected
                      ? Colors.green.withValues(alpha: 0.15)
                      : theme.colorScheme.primaryContainer,
                  borderRadius: BorderRadius.circular(12),
                ),
                child: Icon(
                  Icons.phone_android,
                  color: isConnected
                      ? Colors.green
                      : theme.colorScheme.onPrimaryContainer,
                ),
              ),
              title: Row(
                children: [
                  Flexible(
                    child: Text(
                      dev.name,
                      style: const TextStyle(fontWeight: FontWeight.bold),
                      overflow: TextOverflow.ellipsis,
                    ),
                  ),
                  const SizedBox(width: 8),
                  _statusBadge(statusLabel, statusColor, icon: statusIcon),
                  if (isStale) ...[
                    const SizedBox(width: 6),
                    _statusBadge('STALE', Colors.orange, icon: Icons.access_time),
                  ],
                ],
              ),
              subtitle: Padding(
                padding: const EdgeInsets.only(top: 4.0),
                child: Text(
                  '${dev.model} · ${dev.host}:${dev.port}',
                  style: TextStyle(
                    fontSize: 12,
                    color: theme.colorScheme.onSurfaceVariant,
                  ),
                ),
              ),
              trailing: actionButton,
            ),
            if (isTrusted && !isConnected) ...[
              const SizedBox(height: 4),
              Container(
                width: double.infinity,
                padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 6),
                decoration: BoxDecoration(
                  color: theme.colorScheme.surfaceContainerHigh.withValues(alpha: 0.5),
                  borderRadius: BorderRadius.circular(8),
                ),
                child: Row(
                  children: [
                    Icon(
                      Icons.info_outline,
                      size: 14,
                      color: theme.colorScheme.onSurfaceVariant,
                    ),
                    const SizedBox(width: 6),
                    Expanded(
                      child: Text(
                        'Stored in local trust. If remote phone was reset or cleared, tap Re-Pair.',
                        style: TextStyle(
                          fontSize: 11,
                          color: theme.colorScheme.onSurfaceVariant,
                        ),
                      ),
                    ),
                  ],
                ),
              ),
            ],
          ],
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

  Widget _buildThisDeviceCard(ThemeData theme, dynamic deviceState, bool isLinux) {
    return Card(
      elevation: 0,
      color: theme.colorScheme.surfaceContainerHighest.withValues(alpha: 0.3),
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(16),
        side: BorderSide(color: theme.colorScheme.outlineVariant.withValues(alpha: 0.4)),
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
                isLinux ? Icons.desktop_windows_outlined : Icons.phone_android_outlined,
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

  Widget _buildEmptyTrustedCard(ThemeData theme) {
    return Card(
      elevation: 0,
      color: theme.colorScheme.surfaceContainerHighest.withValues(alpha: 0.25),
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(16),
        side: BorderSide(color: theme.colorScheme.outlineVariant.withValues(alpha: 0.3)),
      ),
      child: Padding(
        padding: const EdgeInsets.symmetric(vertical: 32.0, horizontal: 20.0),
        child: Column(
          children: [
            Container(
              width: 56,
              height: 56,
              decoration: BoxDecoration(
                color: theme.colorScheme.surfaceContainerHigh,
                shape: BoxShape.circle,
              ),
              child: Icon(
                Icons.devices_other_outlined,
                size: 28,
                color: theme.colorScheme.onSurfaceVariant,
              ),
            ),
            const SizedBox(height: 12),
            Text(
              'No Trusted Devices Yet',
              style: theme.textTheme.titleMedium?.copyWith(
                fontWeight: FontWeight.bold,
              ),
            ),
            const SizedBox(height: 6),
            Text(
              'Initiate pairing from your Linux desktop to connect securely using SAS verification.',
              textAlign: TextAlign.center,
              style: theme.textTheme.bodySmall?.copyWith(
                color: theme.colorScheme.onSurfaceVariant,
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildDeviceTile(BuildContext context, ThemeData theme, TrustedDevice dev) {
    final isDiscovered = controller.discoveredDevices
        .any((d) => d.id == dev.deviceId && !d.isStale);

    return Card(
      margin: const EdgeInsets.only(bottom: 10),
      elevation: 0,
      color: theme.colorScheme.surfaceContainerHighest.withValues(alpha: 0.35),
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(16),
        side: BorderSide(
          color: dev.revoked
              ? theme.colorScheme.error.withValues(alpha: 0.3)
              : theme.colorScheme.outlineVariant.withValues(alpha: 0.4),
        ),
      ),
      child: ListTile(
        contentPadding: const EdgeInsets.symmetric(horizontal: 16, vertical: 4),
        onTap: () => _showDeviceDetails(context, dev),
        leading: Container(
          width: 40,
          height: 40,
          decoration: BoxDecoration(
            color: dev.revoked
                ? theme.colorScheme.errorContainer.withValues(alpha: 0.4)
                : theme.colorScheme.surfaceContainerHighest,
            borderRadius: BorderRadius.circular(12),
          ),
          child: Icon(
            dev.isLinux ? Icons.computer : Icons.phone_android,
            color: dev.revoked ? theme.colorScheme.error : theme.colorScheme.primary,
          ),
        ),
        title: Row(
          children: [
            Flexible(
              child: Text(
                dev.displayName,
                style: TextStyle(
                  fontWeight: FontWeight.bold,
                  decoration: dev.revoked ? TextDecoration.lineThrough : null,
                ),
                overflow: TextOverflow.ellipsis,
              ),
            ),
            if (!isDiscovered) ...[
              const SizedBox(width: 8),
              _statusBadge('NOT DISCOVERED', Colors.orange, icon: Icons.radar_outlined),
            ],
          ],
        ),
        subtitle: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            const SizedBox(height: 2),
            Text(
              '${dev.platform.toUpperCase()} · ${dev.revoked ? "Trust revoked" : "Paired"}',
              style: TextStyle(
                fontSize: 12,
                color: dev.revoked ? theme.colorScheme.error : theme.colorScheme.onSurfaceVariant,
              ),
            ),
            if (!isDiscovered) ...[
              const SizedBox(height: 4),
              Container(
                width: double.infinity,
                padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
                decoration: BoxDecoration(
                  color: Colors.orange.withValues(alpha: 0.1),
                  borderRadius: BorderRadius.circular(6),
                  border: Border.all(
                    color: Colors.orange.withValues(alpha: 0.3),
                    width: 1,
                  ),
                ),
                child: Row(
                  children: [
                    Icon(Icons.info_outline, size: 12, color: Colors.orange.shade800),
                    const SizedBox(width: 6),
                    Expanded(
                      child: Text(
                        'Waiting for device / Not currently discovered · Pairing cannot start until discovery succeeds.',
                        style: TextStyle(
                          fontSize: 11,
                          color: Colors.orange.shade800,
                          fontWeight: FontWeight.w500,
                        ),
                      ),
                    ),
                  ],
                ),
              ),
            ],
          ],
        ),
        trailing: const Icon(Icons.chevron_right),
      ),
    );
  }

  Widget _buildPairingGuideCard(ThemeData theme) {
    return Card(
      elevation: 0,
      color: theme.colorScheme.surfaceContainerHighest.withValues(alpha: 0.2),
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(16),
        side: BorderSide(color: theme.colorScheme.outlineVariant.withValues(alpha: 0.3)),
      ),
      child: Padding(
        padding: const EdgeInsets.all(16.0),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Icon(Icons.security, size: 18, color: theme.colorScheme.primary),
                const SizedBox(width: 8),
                Text(
                  'Zero-Trust Pairing Guide',
                  style: theme.textTheme.titleSmall?.copyWith(fontWeight: FontWeight.bold),
                ),
              ],
            ),
            const SizedBox(height: 12),
            _guideStep(theme, '1', 'Connect both devices to the same LAN or Wi-Fi network.'),
            _guideStep(theme, '2', 'Initiate pairing from Linux: tap "Pair" or "Re-Pair" on the discovered device.'),
            _guideStep(theme, '3', 'Verify that the 6-digit SAS code matches on both screens to establish mutual Ed25519 trust.'),
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
