import 'package:flutter/material.dart';
import 'controllers/phonebridge_controller.dart';
import 'screens/app_scaffold.dart';

export 'screens/app_scaffold.dart';
export 'controllers/phonebridge_controller.dart';

void main() {
  WidgetsFlutterBinding.ensureInitialized();
  runApp(const PhoneBridgeApp());
}

class PhoneBridgeApp extends StatelessWidget {
  const PhoneBridgeApp({super.key, this.home});

  final Widget? home;

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'PhoneBridge',
      theme: ThemeData(
        useMaterial3: true,
        colorScheme: ColorScheme.fromSeed(
          seedColor: const Color(0xFF1E88E5),
          brightness: Brightness.light,
        ),
      ),
      darkTheme: ThemeData(
        useMaterial3: true,
        colorScheme: ColorScheme.fromSeed(
          seedColor: const Color(0xFF1E88E5),
          brightness: Brightness.dark,
        ),
      ),
      themeMode: ThemeMode.system,
      home: home ?? _defaultHome(),
    );
  }

  Widget _defaultHome() {
    return AppScaffold(controller: PhoneBridgeController());
  }
}
