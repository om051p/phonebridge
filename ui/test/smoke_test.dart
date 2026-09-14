import 'package:flutter_test/flutter_test.dart';
import 'package:phonebridge_ui/main.dart';

void main() {
  testWidgets('scaffold renders title', (tester) async {
    await tester.pumpWidget(const PhoneBridgeApp());
    expect(find.textContaining('PhoneBridge'), findsOneWidget);
  });
}
