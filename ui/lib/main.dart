// PhoneBridge UI — PLANNED scaffold (Phase 0).
// No business logic here per architecture boundaries.

import 'package:flutter/material.dart';

void main() {
  runApp(const PhoneBridgeApp());
}

class PhoneBridgeApp extends StatelessWidget {
  const PhoneBridgeApp({super.key});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'PhoneBridge',
      theme: ThemeData(useMaterial3: true, colorSchemeSeed: Colors.indigo),
      home: const Scaffold(
        body: Center(
          child: Text('PhoneBridge — Phase 0 scaffold (PLANNED)'),
        ),
      ),
    );
  }
}
