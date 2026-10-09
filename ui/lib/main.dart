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
    const primarySeed = Color(0xFF2563EB);

    final lightBase = ThemeData(
      useMaterial3: true,
      colorScheme: ColorScheme.fromSeed(
        seedColor: primarySeed,
        brightness: Brightness.light,
      ),
    );

    final darkBase = ThemeData(
      useMaterial3: true,
      colorScheme: ColorScheme.fromSeed(
        seedColor: primarySeed,
        brightness: Brightness.dark,
      ),
    );

    ThemeData buildTheme(ThemeData base, Brightness brightness) {
      final isDark = brightness == Brightness.dark;
      return base.copyWith(
        cardTheme: CardThemeData(
          elevation: 0,
          shape: RoundedRectangleBorder(
            borderRadius: BorderRadius.circular(16),
            side: BorderSide(
              color: base.colorScheme.outlineVariant.withValues(alpha: isDark ? 0.35 : 0.45),
            ),
          ),
          clipBehavior: Clip.antiAlias,
        ),
        filledButtonTheme: FilledButtonThemeData(
          style: FilledButton.styleFrom(
            shape: RoundedRectangleBorder(
              borderRadius: BorderRadius.circular(12),
            ),
            padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
          ),
        ),
        outlinedButtonTheme: OutlinedButtonThemeData(
          style: OutlinedButton.styleFrom(
            shape: RoundedRectangleBorder(
              borderRadius: BorderRadius.circular(12),
            ),
            padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 10),
          ),
        ),
        chipTheme: base.chipTheme.copyWith(
          shape: RoundedRectangleBorder(
            borderRadius: BorderRadius.circular(8),
          ),
        ),
        dialogTheme: DialogThemeData(
          shape: RoundedRectangleBorder(
            borderRadius: BorderRadius.circular(20),
          ),
          elevation: 3,
        ),
        appBarTheme: const AppBarTheme(
          elevation: 0,
          scrolledUnderElevation: 1,
          centerTitle: false,
        ),
        navigationBarTheme: const NavigationBarThemeData(
          height: 68,
          elevation: 2,
          labelBehavior: NavigationDestinationLabelBehavior.alwaysShow,
        ),
        navigationRailTheme: const NavigationRailThemeData(
          elevation: 0,
          labelType: NavigationRailLabelType.all,
          groupAlignment: -0.85,
        ),
        snackBarTheme: SnackBarThemeData(
          behavior: SnackBarBehavior.floating,
          shape: RoundedRectangleBorder(
            borderRadius: BorderRadius.circular(12),
          ),
        ),
      );
    }

    return MaterialApp(
      title: 'PhoneBridge',
      debugShowCheckedModeBanner: false,
      theme: buildTheme(lightBase, Brightness.light),
      darkTheme: buildTheme(darkBase, Brightness.dark),
      themeMode: ThemeMode.system,
      home: home ?? _defaultHome(),
    );
  }

  Widget _defaultHome() {
    return AppScaffold(controller: PhoneBridgeController());
  }
}
