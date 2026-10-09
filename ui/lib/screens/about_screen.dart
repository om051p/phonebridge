import 'package:flutter/material.dart';
import '../controllers/phonebridge_controller.dart';
import 'report_issue_screen.dart';

class AboutScreen extends StatelessWidget {
  const AboutScreen({
    super.key,
    this.controller,
  });

  final PhoneBridgeController? controller;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);

    return Scaffold(
      appBar: AppBar(
        title: const Text('About PhoneBridge'),
      ),
      body: ListView(
        padding: const EdgeInsets.all(24.0),
        children: [
          Center(
            child: Container(
              width: 100,
              height: 100,
              decoration: BoxDecoration(
                borderRadius: BorderRadius.circular(24),
                boxShadow: [
                  BoxShadow(
                    color: Colors.black.withValues(alpha: 0.35),
                    blurRadius: 18,
                    offset: const Offset(0, 8),
                  ),
                ],
              ),
              clipBehavior: Clip.antiAlias,
              child: Image.asset(
                'assets/branding/logo.png',
                fit: BoxFit.cover,
                errorBuilder: (context, error, stackTrace) => Container(
                  color: theme.colorScheme.primaryContainer,
                  child: Icon(
                    Icons.phonelink,
                    size: 48,
                    color: theme.colorScheme.onPrimaryContainer,
                  ),
                ),
              ),
            ),
          ),
          const SizedBox(height: 16),
          Center(
            child: Text(
              'PhoneBridge',
              style: theme.textTheme.headlineMedium?.copyWith(
                fontWeight: FontWeight.bold,
                letterSpacing: -0.5,
              ),
            ),
          ),
          Center(
            child: Text(
              'Android ↔ Linux Device Integration',
              style: theme.textTheme.bodyMedium?.copyWith(
                color: theme.colorScheme.onSurfaceVariant,
              ),
            ),
          ),
          Center(
            child: Text(
              'Version 0.3.0 · Phase 4 Production Build',
              style: theme.textTheme.labelSmall?.copyWith(
                color: theme.colorScheme.outline,
              ),
            ),
          ),
          const SizedBox(height: 24),
          const Divider(),
          const SizedBox(height: 16),
          Text(
            'Developer & Support',
            style: theme.textTheme.titleSmall?.copyWith(
              fontWeight: FontWeight.bold,
            ),
          ),
          const SizedBox(height: 8),
          Card(
            child: ListTile(
              leading: const Icon(Icons.business_rounded),
              title: const Text(ReportIssueScreen.developerName),
              subtitle: const Text(ReportIssueScreen.developerEmail),
              trailing: const Icon(Icons.bug_report_outlined),
              onTap: () {
                Navigator.of(context).push(
                  MaterialPageRoute(
                    builder: (_) => ReportIssueScreen(controller: controller),
                  ),
                );
              },
            ),
          ),
          const SizedBox(height: 16),
          const Divider(),
          const SizedBox(height: 16),
          Text(
            'Architecture Highlights',
            style: theme.textTheme.titleSmall?.copyWith(
              fontWeight: FontWeight.bold,
            ),
          ),
          const SizedBox(height: 8),
          _featureRow(Icons.check_circle_outline, 'Embedded Go Core engine via direct JNI gateway'),
          _featureRow(Icons.check_circle_outline, 'Hardware H.264 video encoding & WebRTC media transport'),
          _featureRow(Icons.check_circle_outline, 'Mutual Ed25519 authentication & SAS pairing protocol'),
          _featureRow(Icons.check_circle_outline, 'Real-time clipboard sync with SHA-256 echo suppression'),
          _featureRow(Icons.check_circle_outline, 'COSMIC desktop & Wayland Linux receiver support'),
          const SizedBox(height: 24),
          const Divider(),
          const SizedBox(height: 16),
          Text(
            'Open Source License',
            style: theme.textTheme.titleSmall?.copyWith(
              fontWeight: FontWeight.bold,
            ),
          ),
          const SizedBox(height: 8),
          Text(
            'Licensed under the Apache License, Version 2.0. You may obtain a copy of the License at:\nhttp://www.apache.org/licenses/LICENSE-2.0',
            style: theme.textTheme.bodySmall?.copyWith(
              color: theme.colorScheme.onSurfaceVariant,
              height: 1.4,
            ),
          ),
        ],
      ),
    );
  }

  Widget _featureRow(IconData icon, String text) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 4.0),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Icon(icon, size: 18, color: Colors.green),
          const SizedBox(width: 8),
          Expanded(
            child: Text(text, style: const TextStyle(fontSize: 13)),
          ),
        ],
      ),
    );
  }
}
