import 'dart:io';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import '../controllers/phonebridge_controller.dart';

class ReportIssueScreen extends StatefulWidget {
  const ReportIssueScreen({
    super.key,
    this.controller,
  });

  final PhoneBridgeController? controller;

  static const String developerEmail = 'raxatechnologies@gmail.com';
  static const String developerName = 'Raxa Technologies';

  @override
  State<ReportIssueScreen> createState() => _ReportIssueScreenState();
}

class _ReportIssueScreenState extends State<ReportIssueScreen> {
  final _titleController = TextEditingController();
  final _descriptionController = TextEditingController();

  final List<String> _categories = const [
    'Connection & Pairing',
    'Screen Mirroring',
    'Clipboard Sync',
    'File Transfer',
    'Notifications',
    'Performance / Crash',
    'General / Other',
  ];

  late String _selectedCategory;
  bool _includeDiagnostics = true;
  bool _isDiagnosticsExpanded = false;

  @override
  void initState() {
    super.initState();
    _selectedCategory = _categories.first;
  }

  @override
  void dispose() {
    _titleController.dispose();
    _descriptionController.dispose();
    super.dispose();
  }

  String _buildDiagnosticsSnapshot() {
    final buffer = StringBuffer();
    final ctrl = widget.controller;

    buffer.writeln('### Environment & System Diagnostics');
    buffer.writeln('- **App Version**: 0.3.0');
    buffer.writeln('- **Platform**: ${Platform.isAndroid ? "Android Mobile" : (Platform.isLinux ? "Linux Desktop" : Platform.operatingSystem)}');
    buffer.writeln('- **OS Version**: ${Platform.operatingSystemVersion}');

    if (ctrl != null) {
      final link = ctrl.linkStatus;
      buffer.writeln('- **Link Status**: ${link.phase.name}');
      buffer.writeln('- **Stream Resolution**: ${ctrl.selectedWidth == 0 ? "Device Default" : "${ctrl.selectedWidth}x${ctrl.selectedHeight}"}');
      buffer.writeln('- **Target FPS**: ${ctrl.selectedFps == 0 ? "Device Default" : "${ctrl.selectedFps} fps"}');
      buffer.writeln('- **Paired Devices**: ${ctrl.trustedDevices.length}');
      buffer.writeln('- **Clipboard Sync Enabled**: ${ctrl.clipboardStatus.isEnabled}');
      buffer.writeln('- **IME Selected**: ${ctrl.clipboardStatus.imeSelected}');
      buffer.writeln('- **Capturing**: ${ctrl.captureStats.isCapturing}');
      buffer.writeln('- **Session Active**: ${ctrl.deviceState.isSessionActive}');
      final err = ctrl.captureStats.lastError;
      if (err != null && err.isNotEmpty) {
        buffer.writeln('- **Last Engine Error**: $err');
      }
    } else {
      buffer.writeln('- **Engine Controller**: Not attached');
    }

    buffer.writeln('- **Privacy Notice**: Diagnostic snapshot contains zero personal identifiers, clipboard contents, notification payloads, or private keys.');
    return buffer.toString();
  }

  String _buildFullReportMarkdown() {
    final title = _titleController.text.trim().isEmpty
        ? 'Bug Report: [$_selectedCategory]'
        : _titleController.text.trim();
    final desc = _descriptionController.text.trim().isEmpty
        ? 'No additional description provided.'
        : _descriptionController.text.trim();

    final buffer = StringBuffer();
    buffer.writeln('# PhoneBridge Bug Report');
    buffer.writeln();
    buffer.writeln('**Developer Contact**: ${ReportIssueScreen.developerName} <${ReportIssueScreen.developerEmail}>');
    buffer.writeln('**Category**: $_selectedCategory');
    buffer.writeln('**Summary**: $title');
    buffer.writeln();
    buffer.writeln('## Description & Steps to Reproduce');
    buffer.writeln(desc);
    buffer.writeln();

    if (_includeDiagnostics) {
      buffer.writeln(_buildDiagnosticsSnapshot());
    }

    return buffer.toString();
  }

  Future<void> _sendEmail() async {
    final title = _titleController.text.trim().isEmpty
        ? 'PhoneBridge Report: $_selectedCategory'
        : 'PhoneBridge Report: ${_titleController.text.trim()}';
    final body = _buildFullReportMarkdown();

    bool launched = false;
    final ctrl = widget.controller;

    if (Platform.isAndroid && ctrl != null) {
      try {
        const platform = MethodChannel('dev.phonebridge/control');
        final res = await platform.invokeMethod<bool>('openEmailClient', {
          'email': ReportIssueScreen.developerEmail,
          'subject': title,
          'body': body,
        });
        launched = res ?? false;
      } catch (_) {
        launched = false;
      }
    } else if (Platform.isLinux) {
      try {
        final uri = Uri(
          scheme: 'mailto',
          path: ReportIssueScreen.developerEmail,
          queryParameters: {
            'subject': title,
            'body': body,
          },
        );
        final res = await Process.run('xdg-open', [uri.toString()]);
        launched = res.exitCode == 0;
      } catch (_) {
        launched = false;
      }
    }

    if (!mounted) return;

    if (launched) {
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(
          content: Text('Email composer opened. Thank you for your feedback!'),
          behavior: SnackBarBehavior.floating,
        ),
      );
    } else {
      // Fallback: copy to clipboard so user never loses their report
      await Clipboard.setData(ClipboardData(text: body));
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(
          content: Text(
            'Could not launch mail client. Complete report copied to clipboard! Paste it into an email to raxatechnologies@gmail.com',
          ),
          duration: Duration(seconds: 5),
          behavior: SnackBarBehavior.floating,
        ),
      );
    }
  }

  Future<void> _copyReportToClipboard() async {
    final report = _buildFullReportMarkdown();
    await Clipboard.setData(ClipboardData(text: report));
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(
      const SnackBar(
        content: Text('Bug report copied to clipboard! Ready to send to raxatechnologies@gmail.com'),
        behavior: SnackBarBehavior.floating,
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);

    return Scaffold(
      appBar: AppBar(
        title: const Text('Report an Issue / Feedback'),
        actions: [
          IconButton(
            icon: const Icon(Icons.copy_rounded),
            tooltip: 'Copy Report to Clipboard',
            onPressed: _copyReportToClipboard,
          ),
        ],
      ),
      body: ListView(
        padding: const EdgeInsets.all(16.0),
        children: [
          // Developer Contact Header Card
          Card(
            color: theme.colorScheme.surfaceContainerHighest,
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
                      Icons.contact_support_rounded,
                      color: theme.colorScheme.onPrimaryContainer,
                      size: 28,
                    ),
                  ),
                  const SizedBox(width: 14),
                  Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text(
                          ReportIssueScreen.developerName,
                          style: theme.textTheme.titleMedium?.copyWith(
                            fontWeight: FontWeight.bold,
                          ),
                        ),
                        const SizedBox(height: 2),
                        SelectableText(
                          ReportIssueScreen.developerEmail,
                          style: theme.textTheme.bodyMedium?.copyWith(
                            color: theme.colorScheme.primary,
                            fontWeight: FontWeight.w600,
                          ),
                        ),
                      ],
                    ),
                  ),
                ],
              ),
            ),
          ),
          const SizedBox(height: 16),

          // Issue Category Chips
          Text(
            'Issue Category',
            style: theme.textTheme.titleSmall?.copyWith(
              fontWeight: FontWeight.bold,
            ),
          ),
          const SizedBox(height: 8),
          Wrap(
            spacing: 8,
            runSpacing: 8,
            children: _categories.map((cat) {
              final isSelected = _selectedCategory == cat;
              return ChoiceChip(
                label: Text(cat),
                selected: isSelected,
                onSelected: (selected) {
                  if (selected) {
                    setState(() {
                      _selectedCategory = cat;
                    });
                  }
                },
              );
            }).toList(),
          ),
          const SizedBox(height: 16),

          // Title / Summary Field
          TextField(
            controller: _titleController,
            decoration: InputDecoration(
              labelText: 'Summary / Title',
              hintText: 'e.g. Screen stream lag when waking up',
              border: OutlineInputBorder(
                borderRadius: BorderRadius.circular(12),
              ),
              prefixIcon: const Icon(Icons.short_text_rounded),
            ),
          ),
          const SizedBox(height: 16),

          // Details / Reproduction Field
          TextField(
            controller: _descriptionController,
            maxLines: 5,
            decoration: InputDecoration(
              labelText: 'Description & Steps to Reproduce',
              hintText: 'What happened? What did you expect to happen? Any specific error message displayed?',
              alignLabelWithHint: true,
              border: OutlineInputBorder(
                borderRadius: BorderRadius.circular(12),
              ),
            ),
          ),
          const SizedBox(height: 16),

          // Diagnostics Checkbox & Preview
          CheckboxListTile(
            contentPadding: EdgeInsets.zero,
            value: _includeDiagnostics,
            title: const Text('Include Privacy-Safe System Diagnostics'),
            subtitle: const Text('Helps diagnose engine, transport, and codec issues. Zero private data.'),
            onChanged: (val) {
              setState(() {
                _includeDiagnostics = val ?? true;
              });
            },
          ),

          if (_includeDiagnostics) ...[
            Card(
              child: ExpansionTile(
                initiallyExpanded: _isDiagnosticsExpanded,
                onExpansionChanged: (exp) => setState(() => _isDiagnosticsExpanded = exp),
                leading: const Icon(Icons.phonelink_setup_rounded),
                title: const Text('Diagnostic Snapshot Preview'),
                subtitle: const Text('Tap to review technical metadata'),
                children: [
                  Padding(
                    padding: const EdgeInsets.all(16.0),
                    child: SelectableText(
                      _buildDiagnosticsSnapshot(),
                      style: theme.textTheme.bodySmall?.copyWith(
                        fontFamily: 'monospace',
                        height: 1.5,
                      ),
                    ),
                  ),
                ],
              ),
            ),
            const SizedBox(height: 16),
          ],

          const SizedBox(height: 8),

          // Action Buttons
          Row(
            children: [
              Expanded(
                child: FilledButton.icon(
                  onPressed: _sendEmail,
                  icon: const Icon(Icons.email_outlined),
                  label: const Text('Send Email Report'),
                ),
              ),
              const SizedBox(width: 12),
              OutlinedButton.icon(
                onPressed: _copyReportToClipboard,
                icon: const Icon(Icons.copy_rounded),
                label: const Text('Copy Report'),
              ),
            ],
          ),
          const SizedBox(height: 24),
        ],
      ),
    );
  }
}
