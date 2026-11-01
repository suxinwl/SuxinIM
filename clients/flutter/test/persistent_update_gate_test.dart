import 'dart:async';
import 'package:app_installer/app_installer.dart';
import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:openim/widgets/persistent_update_gate.dart';

void main() {
  testWidgets(
      'fast mandatory update survives slow auto-login and route replacement',
      (tester) async {
    final gate = ValueNotifier<Widget?>(null);
    final login = Completer<void>();
    final nav = GlobalKey<NavigatorState>();
    var businessActions = 0;
    await tester.pumpWidget(MaterialApp(
      navigatorKey: nav,
      builder: (context, child) =>
          PersistentUpdateGate(gate: gate, child: child!),
      routes: {
        '/': (_) => const Scaffold(body: Text('splash')),
        '/home': (_) => Scaffold(
            body: TextButton(
                onPressed: () => businessActions++,
                child: const Text('business action'))),
        '/login': (_) => const Scaffold(body: Text('login')),
      },
    ));
    login.future.then((_) => nav.currentState!.popAndPushNamed('/home'));
    // The version API finishes before cached-token login and its navigation.
    gate.value = const Text('mandatory update', key: Key('mandatory'));
    await tester.pump();
    login.complete();
    await tester.pumpAndSettle();
    expect(find.byKey(const Key('mandatory')), findsOneWidget);
    await tester.tap(find.text('business action'), warnIfMissed: false);
    expect(businessActions, 0);
    nav.currentState!.pushNamedAndRemoveUntil('/login', (_) => false);
    await tester.pumpAndSettle();
    expect(find.text('login'), findsOneWidget);
    expect(find.byKey(const Key('mandatory')), findsOneWidget);
    await tester.binding.handlePopRoute();
    await tester.pumpAndSettle();
    expect(find.byKey(const Key('mandatory')), findsOneWidget);
    await tester.pumpWidget(const SizedBox.shrink());
    gate.dispose();
  });

  test('native APK installer rejection propagates for retry UI', () async {
    TestWidgetsFlutterBinding.ensureInitialized();
    debugDefaultTargetPlatformOverride = TargetPlatform.android;
    addTearDown(() => debugDefaultTargetPlatformOverride = null);
    const channel = MethodChannel('app_installer');
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(channel, (call) async {
      expect(call.method, 'installApk');
      throw PlatformException(
          code: 'PERMISSION_DENIED', message: 'synthetic denial');
    });
    addTearDown(() => TestDefaultBinaryMessengerBinding
        .instance.defaultBinaryMessenger
        .setMockMethodCallHandler(channel, null));
    await expectLater(AppInstaller.installApk('/synthetic/package.apk'),
        throwsA(isA<PlatformException>()));
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(channel, (call) async => false);
    await expectLater(AppInstaller.installApk('/synthetic/missing.apk'),
        throwsA(isA<PlatformException>().having((e) => e.code, 'code', 'INSTALLER_REJECTED')));
  });
}
