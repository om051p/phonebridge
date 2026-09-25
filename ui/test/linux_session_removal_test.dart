// Phase 6 consolidation gate: the retired desktop session view must leave no
// trace in production code, and no screen or shared widget may own the stacks
// the view used to be able to build standalone (an IPC client, a session
// controller, or a service of its own).
//
// This is a source-level assertion on purpose: compile-time checks prove the
// deleted symbols no longer exist, but only a scan can prove that no
// *production* file reintroduces screen-level ownership — which is exactly the
// invariant the deletion exists to protect.

import 'dart:io';

import 'package:flutter_test/flutter_test.dart';

void main() {
  // `flutter test` runs from the package root; tolerate a repo-root invocation.
  final libRoot = Directory('lib').existsSync()
      ? 'lib'
      : (Directory('ui/lib').existsSync()
          ? 'ui/lib'
          : (throw StateError(
              'cannot locate ui/lib from ${Directory.current}')));

  List<File> dartFiles() => Directory(libRoot)
      .listSync(recursive: true)
      .whereType<File>()
      .where((f) => f.path.endsWith('.dart'))
      .toList();

  test('no production file references the deleted session view', () {
    // The view itself is gone...
    expect(
      File('$libRoot/ui/linux_session_view.dart').existsSync(),
      isFalse,
      reason: 'the retired session view must be deleted, not left as a shell',
    );

    // ...and nothing under lib/ names it — code, comments or exports alike.
    for (final file in dartFiles()) {
      final source = file.readAsStringSync();
      expect(
        source.contains('LinuxSessionView'),
        isFalse,
        reason: '${file.path} still references the deleted view',
      );
      expect(
        source.contains('linux_session_view'),
        isFalse,
        reason: '${file.path} still imports/exports the deleted view',
      );
    }
  });

  test('screens and shared widgets own no IPC client, service or session controller',
      () {
    final ownership = RegExp(
      r'(LocalIpcClient|LinuxBridgeService|SessionController)\s*\(',
    );

    for (final file in dartFiles()) {
      final normalized = file.path.replaceAll('\\', '/');
      final isSurface = normalized.contains('/screens/') ||
          normalized.contains('/ui/');
      if (!isSurface) continue;

      final source = file.readAsStringSync();
      final match = ownership.firstMatch(source);
      expect(
        match,
        isNull,
        reason: '${file.path} constructs an owned stack '
            '(${match?.group(0)}); surfaces are pure reads of the shared one',
      );
    }
  });

  test('the service exposes no view-only pairing/discovery seams', () {
    final service =
        File('$libRoot/services/linux_bridge_service.dart').readAsStringSync();
    // These proto-typed wrappers existed solely for the retired view; the
    // production surfaces ride the model-typed controller/service path.
    expect(service.contains('startPairing'), isFalse);
    expect(service.contains('completePairing'), isFalse);
    expect(
      RegExp(r'Future<ipc\.ListDevicesResponse> listDevices').hasMatch(service),
      isFalse,
    );
    expect(
      RegExp(r'Future<ipc\.ListTrustedDevicesResponse> listTrustedDevices')
          .hasMatch(service),
      isFalse,
    );
  });

  test('main.dart no longer exports anything from ui/', () {
    final main = File('$libRoot/main.dart').readAsStringSync();
    expect(main.contains("export 'ui/"), isFalse);
  });
}
