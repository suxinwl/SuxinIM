// Formal111 production AppNavigator regression; four route timing cases.
// Uses the actual AppNavigator, LoginBinding, LoginPage and LoginLogic.
// Only SDK/push dependencies, SharedPreferences and package metadata are fake.
// Do not apply a navigation guard in this test: the production implementation
// must contain the approved guard before these assertions can pass.
import 'package:flutter/material.dart';
import 'package:flutter_screenutil/flutter_screenutil.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:get/get.dart';
import 'package:openim/core/controller/im_controller.dart';
import 'package:openim/core/controller/push_controller.dart';
import 'package:openim/pages/login/login_binding.dart';
import 'package:openim/pages/login/login_logic.dart';
import 'package:openim/pages/login/login_view.dart';
import 'package:openim/routes/app_navigator.dart';
import 'package:openim_common/openim_common.dart';
import 'package:package_info_plus/package_info_plus.dart';
import 'package:shared_preferences/shared_preferences.dart';

class _InertDependency extends Fake {
  final onStart = InternalFinalCallback<void>(callback: () {});
  final onDelete = InternalFinalCallback<void>(callback: () {});
}

class _IM extends _InertDependency implements IMController {}

class _Push extends _InertDependency implements PushController {}

class _RouteEvents extends NavigatorObserver {
  final pushed = <String?>[];

  @override
  void didPush(Route<dynamic> route, Route<dynamic>? previousRoute) {
    pushed.add(route.settings.name);
    super.didPush(route, previousRoute);
  }

  int get loginPushes => pushed.where((name) => name == '/login').length;
}

Future<_RouteEvents> _mount(WidgetTester tester) async {
  tester.view.physicalSize = const Size(375, 812);
  tester.view.devicePixelRatio = 1;
  addTearDown(() async {
    await tester.pumpWidget(const SizedBox.shrink());
    await tester.pumpAndSettle();
    Get.reset();
    tester.view.resetPhysicalSize();
    tester.view.resetDevicePixelRatio();
  });
  final events = _RouteEvents();
  await tester.pumpWidget(ScreenUtilInit(
    designSize: const Size(375, 812),
    builder: (_, __) => GetMaterialApp(
      initialRoute: '/navigation-test-start',
      translations: TranslationService(),
      locale: const Locale('zh', 'CN'),
      fallbackLocale: const Locale('zh', 'CN'),
      navigatorObservers: [events],
      getPages: [
        GetPage(name: '/navigation-test-start', page: () => const Scaffold()),
        GetPage(name: '/navigation-test-away', page: () => const Scaffold()),
        GetPage(
          name: '/login',
          page: () => LoginPage(),
          binding: LoginBinding(),
          transition: Transition.cupertino,
        ),
      ],
    ),
  ));
  await tester.pumpAndSettle();
  return events;
}

// Call only after route transitions have settled. This avoids sampling an
// outgoing Page from the overlay, the limitation identified in the 110 probe.
LoginLogic _currentLogic(WidgetTester tester) {
  expect(Get.currentRoute, '/login');
  final pageFinder = find.byType(LoginPage);
  expect(pageFinder, findsOneWidget);
  final page = tester.widget<LoginPage>(pageFinder);
  final fields = find.descendant(
    of: pageFinder,
    matching: find.byType(TextField),
  );
  expect(fields, findsNWidgets(2));
  expect(page.logic.isClosed, isFalse);
  expect(Get.isRegistered<LoginLogic>(), isTrue);
  expect(Get.find<LoginLogic>(), same(page.logic));
  expect(tester.widget<TextField>(fields.at(0)).controller,
      same(page.logic.phoneCtrl));
  expect(tester.widget<TextField>(fields.at(1)).controller,
      same(page.logic.pwdCtrl));
  return page.logic;
}

Future<LoginLogic> _assertEditableAndClear(WidgetTester tester) async {
  final logic = _currentLogic(tester);
  final fields = find.descendant(
    of: find.byType(LoginPage),
    matching: find.byType(TextField),
  );
  await tester.enterText(fields.at(0), 'navigation@example.invalid');
  await tester.enterText(fields.at(1), 'x' * 46);
  await tester.pump();
  expect(logic.enabled.value, isTrue);
  expect(tester.widget<Button>(find.byType(Button)).enabled, isTrue);
  final accountBox = find.ancestor(
    of: fields.at(0),
    matching: find.byType(InputBox),
  );
  expect(accountBox, findsOneWidget);
  // In email mode the account InputBox has one translucent GestureDetector:
  // its actual clear button (the optional country-code control is absent).
  final clearButton = find.descendant(
    of: accountBox,
    matching: find.byWidgetPredicate((widget) =>
        widget is GestureDetector &&
        widget.behavior == HitTestBehavior.translucent &&
        widget.onTap != null),
  );
  expect(clearButton, findsOneWidget);
  await tester.tap(clearButton);
  await tester.pump();
  expect(logic.phoneCtrl.text, isEmpty);
  expect(tester.widget<TextField>(fields.at(0)).controller!.text, isEmpty);
  expect(logic.enabled.value, isFalse);
  expect(tester.widget<Button>(find.byType(Button)).enabled, isFalse);
  expect(tester.takeException(), isNull);
  return logic;
}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  setUp(() async {
    Get.testMode = true;
    SharedPreferences.setMockInitialValues({'loginType': 1});
    await DataSp.init();
    PackageInfo.setMockInitialValues(
      appName: 'navigation-test',
      packageName: 'navigation.test.only',
      version: '0.0.0',
      buildNumber: '0',
      buildSignature: '',
    );
    Get.put<IMController>(_IM(), permanent: true);
    Get.put<PushController>(_Push(), permanent: true);
  });

  testWidgets('same-tick Login redirects admit one live page', (tester) async {
    final events = await _mount(tester);
    AppNavigator.startLogin();
    // Fixed Flutter/Get observers update the current route synchronously.
    expect(Get.currentRoute, '/login');
    AppNavigator.startLogin();
    await tester.pumpAndSettle();
    expect(events.loginPushes, 1);
    await _assertEditableAndClear(tester);
  });

  testWidgets('Login redirect during its transition preserves current inputs',
      (tester) async {
    final events = await _mount(tester);
    AppNavigator.startLogin();
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 80));
    AppNavigator.startLogin();
    await tester.pumpAndSettle();
    expect(events.loginPushes, 1);
    await _assertEditableAndClear(tester);
  });

  testWidgets('settled Login redirect preserves its controller and listeners',
      (tester) async {
    final events = await _mount(tester);
    AppNavigator.startLogin();
    await tester.pumpAndSettle();
    final before = await _assertEditableAndClear(tester);
    AppNavigator.startLogin();
    await tester.pumpAndSettle();
    final after = await _assertEditableAndClear(tester);
    expect(after, same(before));
    expect(events.loginPushes, 1);
  });

  testWidgets('leaving Login permits a subsequent real invalidation redirect',
      (tester) async {
    final events = await _mount(tester);
    AppNavigator.startLogin();
    await tester.pumpAndSettle();
    final first = await _assertEditableAndClear(tester);
    Get.offAllNamed('/navigation-test-away');
    await tester.pumpAndSettle();
    expect(Get.currentRoute, '/navigation-test-away');
    expect(first.isClosed, isTrue);
    AppNavigator.startLogin();
    await tester.pumpAndSettle();
    final next = await _assertEditableAndClear(tester);
    expect(next, isNot(same(first)));
    expect(events.loginPushes, 2);
  });
}
