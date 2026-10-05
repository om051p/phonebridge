import 'package:flutter/material.dart';
import '../controllers/phonebridge_controller.dart';
import '../models/inbound_pairing.dart';

/// The receiving device's explicit Pairing Request surface (Phase 2).
///
/// Shows who is asking and the SAS to compare, and offers exactly two exits:
/// Accept and Reject. Trust is committed natively only after the decision AND
/// the requester's signed confirm; this dialog never trusts anything by
/// itself. If the request expires or is withdrawn while open, the dialog
/// closes itself rather than offering a decision the native side can no
/// longer record.
class PairingRequestDialog extends StatefulWidget {
  const PairingRequestDialog({
    super.key,
    required this.controller,
    required this.request,
  });

  final PhoneBridgeController controller;
  final InboundPairing request;

  /// Convenience launcher: shows the dialog for [request] if it is still
  /// pending. Returns the dialog's future for tests.
  static Future<void> show(
    BuildContext context, {
    required PhoneBridgeController controller,
    required InboundPairing request,
  }) {
    return showDialog<void>(
      context: context,
      barrierDismissible: false,
      builder: (_) => PairingRequestDialog(controller: controller, request: request),
    );
  }

  @override
  State<PairingRequestDialog> createState() => _PairingRequestDialogState();
}

class _PairingRequestDialogState extends State<PairingRequestDialog> {
  bool _answered = false;

  @override
  void initState() {
    super.initState();
    // Linux pushes ARRIVED/WITHDRAWN transitions; Android's controller runs
    // the pending sweep. Listening to the controller covers both: when the
    // token leaves the snapshot the native side can no longer record a
    // decision, so the dialog must not linger.
    widget.controller.addListener(_onControllerChanged);
  }

  void _onControllerChanged() {
    if (!mounted || _answered) return;
    final still = widget.controller.pairingRequests
        .any((r) => r.token == widget.request.token);
    if (!still) {
      Navigator.of(context).pop();
    }
  }

  @override
  void dispose() {
    widget.controller.removeListener(_onControllerChanged);
    super.dispose();
  }

  Future<void> _respond(bool approved) async {
    _answered = true;
    final navigator = Navigator.of(context);
    final messenger = ScaffoldMessenger.maybeOf(context);
    final ok = await widget.controller.respondPairing(
      request: widget.request,
      approved: approved,
    );
    if (navigator.mounted) navigator.pop();
    if (ok && approved && messenger != null) {
      // Trust commits when the requester's confirm lands (usually within a
      // poll interval); refreshAll on the trustChanged event finishes the
      // Devices tab update. The snackbar says what was decided, not what has
      // necessarily committed yet.
      messenger.showSnackBar(
        SnackBar(
          content: Text('Pairing request from ${widget.request.remoteName} accepted.'),
          behavior: SnackBarBehavior.floating,
        ),
      );
    }
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return AlertDialog(
      icon: Icon(
        Icons.group_add_outlined,
        size: 36,
        color: theme.colorScheme.primary,
      ),
      title: const Text(
        'Pairing Request',
        textAlign: TextAlign.center,
        style: TextStyle(fontWeight: FontWeight.bold),
      ),
      content: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.center,
        children: [
          Text(
            '${widget.request.remoteName} (${widget.request.platformLabel}) wants to pair with this device.',
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
                    'Compare this 6-digit code with the one shown on the requesting device before accepting. Nothing is trusted until you accept.',
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
              widget.request.sas.isEmpty ? '——————' : widget.request.sas,
              style: TextStyle(
                fontFamily: 'monospace',
                fontSize: 32,
                fontWeight: FontWeight.bold,
                letterSpacing: 6,
                color: theme.colorScheme.onPrimaryContainer,
              ),
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
          onPressed: _answered ? null : () => _respond(false),
          child: const Text('REJECT'),
        ),
        FilledButton.icon(
          icon: const Icon(Icons.check, size: 18),
          onPressed: _answered ? null : () => _respond(true),
          label: const Text('ACCEPT'),
        ),
      ],
    );
  }
}
