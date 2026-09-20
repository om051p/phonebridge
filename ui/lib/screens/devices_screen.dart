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
          content: Text(controller.lastErrorMessage ?? 'Pairing initiation failed'),
          backgroundColor: Theme.of(context).colorScheme.error,
        ),
      );
      return;
    }

    showDialog(
      context: context,
      barrierDismissible: false,
      builder: (ctx) => AlertDialog(
        title: const Row(
          children: [
            Icon(Icons.security, color: Colors.blue),
            SizedBox(width: 8),
            Text('Confirm Pairing Code'),
          ],
        ),
        content: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text('Pairing with ${result.deviceName} (${dev.shortId})'),
            const SizedBox(height: 16),
            const Text(
              'Compare the 6-digit Short Authentication String (SAS) code with the code shown on the other device:',
              style: TextStyle(fontSize: 13),
            ),
            const SizedBox(height: 16),
            Center(
              child: Container(
                padding: const EdgeInsets.symmetric(horizontal: 20, vertical: 10),
                decoration: BoxDecoration(
                  color: Theme.of(context).colorScheme.primaryContainer,
                  borderRadius: BorderRadius.circular(12),
                ),
                child: Text(
                  result.sasCode,
                  style: TextStyle(
                    fontSize: 28,
                    fontWeight: FontWeight.bold,
                    letterSpacing: 4,
                    color: Theme.of(context).colorScheme.onPrimaryContainer,
                  ),
                ),
              ),
            ),
          ],
        ),
        actions: [
          TextButton(
            onPressed: () {
              Navigator.pop(ctx);
              controller.confirmPairing(deviceId: dev.id, confirmed: false);
            },
            child: const Text('REJECT'),
          ),
          FilledButton(
            onPressed: () {
              Navigator.pop(ctx);
              controller.confirmPairing(deviceId: dev.id, confirmed: true);
            },
            child: const Text('CONFIRM MATCH'),
          ),
        ],
      ),
    );
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
                    width: 44,
                    height: 44,
                    decoration: BoxDecoration(
                      color: theme.colorScheme.primaryContainer,
                      borderRadius: BorderRadius.circular(12),
                    ),
                    child: Icon(
                      Icons.computer,
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
                  Chip(
                    label: Text(device.revoked ? 'Revoked' : 'Trusted'),
                    backgroundColor: device.revoked
                        ? theme.colorScheme.errorContainer
                        : Colors.green.withValues(alpha: 0.15),
                    labelStyle: TextStyle(
                      color: device.revoked ? theme.colorScheme.error : Colors.green,
                      fontWeight: FontWeight.bold,
                    ),
                  ),
                ],
              ),
              const Divider(height: 32),
              _detailRow(theme, 'Device Fingerprint', device.shortId),
              _detailRow(theme, 'Paired On', _formatDate(device.pairedAtMs)),
              _detailRow(theme, 'Last Seen', _formatDate(device.lastSeenMs)),
              const SizedBox(height: 24),
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
          Text(value, style: const TextStyle(fontWeight: FontWeight.bold)),
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
        if (isLinux) {
          await controller.refreshDiscoveredDevices();
        }
      },
      child: ListView(
        padding: const EdgeInsets.all(16.0),
        children: [
          _buildThisDeviceCard(theme, deviceState, isLinux),
          const SizedBox(height: 20),
          if (isLinux) ...[
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
                shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(14)),
                child: const Padding(
                  padding: EdgeInsets.all(16.0),
                  child: Center(
                    child: Text(
                      'No new PhoneBridge devices discovered yet.\nTap "Scan" to search the local network via mDNS.',
                      textAlign: TextAlign.center,
                      style: TextStyle(fontSize: 12),
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
    return Card(
      margin: const EdgeInsets.only(bottom: 10),
      elevation: 0,
      color: theme.colorScheme.surfaceContainerHighest.withValues(alpha: 0.35),
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(14),
        side: BorderSide(color: theme.colorScheme.outlineVariant.withValues(alpha: 0.4)),
      ),
      child: ListTile(
        leading: CircleAvatar(
          backgroundColor: theme.colorScheme.primaryContainer,
          child: Icon(
            Icons.phone_android,
            color: theme.colorScheme.primary,
          ),
        ),
        title: Text(
          dev.name,
          style: const TextStyle(fontWeight: FontWeight.bold),
        ),
        subtitle: Text(
          '${dev.model} · ${dev.host}:${dev.port}',
          style: TextStyle(
            fontSize: 12,
            color: theme.colorScheme.onSurfaceVariant,
          ),
        ),
        trailing: FilledButton.tonal(
          onPressed: controller.isLoading ? null : () => _startPairingFlow(context, dev),
          child: const Text('PAIR'),
        ),
      ),
    );
  }

  Widget _buildThisDeviceCard(ThemeData theme, dynamic deviceState, bool isLinux) {
    return Card(
      elevation: 0,
      color: theme.colorScheme.primaryContainer.withValues(alpha: 0.3),
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(16),
        side: BorderSide(color: theme.colorScheme.primary.withValues(alpha: 0.2)),
      ),
      child: Padding(
        padding: const EdgeInsets.all(16.0),
        child: Row(
          children: [
            Container(
              width: 44,
              height: 44,
              decoration: BoxDecoration(
                color: theme.colorScheme.primary,
                borderRadius: BorderRadius.circular(12),
              ),
              child: Icon(
                isLinux ? Icons.desktop_windows : Icons.phone_android,
                color: Colors.white,
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
              padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
              decoration: BoxDecoration(
                color: Colors.green.withValues(alpha: 0.15),
                borderRadius: BorderRadius.circular(8),
              ),
              child: const Text(
                'Discoverable',
                style: TextStyle(color: Colors.green, fontSize: 11, fontWeight: FontWeight.bold),
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
      color: theme.colorScheme.surfaceContainerHighest.withValues(alpha: 0.3),
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(16),
      ),
      child: Padding(
        padding: const EdgeInsets.symmetric(vertical: 32.0, horizontal: 20.0),
        child: Column(
          children: [
            Icon(
              Icons.devices_other,
              size: 44,
              color: theme.colorScheme.onSurfaceVariant.withValues(alpha: 0.6),
            ),
            const SizedBox(height: 12),
            Text(
              'No Trusted Devices Yet',
              style: theme.textTheme.titleMedium?.copyWith(
                fontWeight: FontWeight.bold,
              ),
            ),
            const SizedBox(height: 4),
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
    return Card(
      margin: const EdgeInsets.only(bottom: 10),
      elevation: 0,
      color: theme.colorScheme.surfaceContainerHighest.withValues(alpha: 0.35),
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(14),
        side: BorderSide(color: theme.colorScheme.outlineVariant.withValues(alpha: 0.4)),
      ),
      child: ListTile(
        onTap: () => _showDeviceDetails(context, dev),
        leading: CircleAvatar(
          backgroundColor: theme.colorScheme.surfaceContainerHighest,
          child: Icon(
            Icons.computer,
            color: dev.revoked ? Colors.grey : theme.colorScheme.primary,
          ),
        ),
        title: Text(
          dev.displayName,
          style: TextStyle(
            fontWeight: FontWeight.bold,
            decoration: dev.revoked ? TextDecoration.lineThrough : null,
          ),
        ),
        subtitle: Text(
          '${dev.platform.toUpperCase()} · ${dev.revoked ? "Trust revoked" : "Paired"}',
          style: TextStyle(
            fontSize: 12,
            color: dev.revoked ? theme.colorScheme.error : theme.colorScheme.onSurfaceVariant,
          ),
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
        side: BorderSide(color: theme.colorScheme.outlineVariant.withValues(alpha: 0.4)),
      ),
      child: Padding(
        padding: const EdgeInsets.all(16.0),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Icon(Icons.shield_outlined, color: theme.colorScheme.primary, size: 20),
                const SizedBox(width: 8),
                Text(
                  'Zero-Configuration Pairing',
                  style: theme.textTheme.titleSmall?.copyWith(
                    fontWeight: FontWeight.bold,
                  ),
                ),
              ],
            ),
            const SizedBox(height: 8),
            Text(
              '1. Ensure both devices are on the same Wi-Fi network.\n'
              '2. Launch PhoneBridge on your Linux PC.\n'
              '3. Select this phone from the discovered list.\n'
              '4. Verify the 6-digit SAS code on both screens to establish an Ed25519 authenticated link.',
              style: theme.textTheme.bodySmall?.copyWith(
                color: theme.colorScheme.onSurfaceVariant,
                height: 1.4,
              ),
            ),
          ],
        ),
      ),
    );
  }
}
