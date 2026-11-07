import 'dart:async';
import 'dart:typed_data';
import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_screenutil/flutter_screenutil.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:get/get.dart';
import 'package:openim/core/controller/app_controller.dart';
import 'package:openim/core/controller/im_controller.dart';
import 'package:openim/core/controller/push_controller.dart';
import 'package:openim/pages/login/login_logic.dart';
import 'package:openim/pages/login/login_view.dart';
import 'package:openim_common/openim_common.dart';
import 'package:package_info_plus/package_info_plus.dart';
import 'package:shared_preferences/shared_preferences.dart';

// Only native encrypted-preference transport, the SDK reset boundary, and HTTP
// responses are synthetic. LoginLogic, Config, HrmSSO and the rendered page are real.
class _NativeState {
  String sealed = '{}';
  int writes = 0;
  final failWrites = <int>{};
  void install() =>
      TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
          .setMockMethodCallHandler(const MethodChannel('puty/local_store'),
              (call) async {
        if (call.method == 'preferences/read') return sealed;
        if (call.method == 'preferences/write') {
          if (failWrites.contains(++writes))
            throw PlatformException(code: 'SYNTHETIC_STORAGE_FAILURE');
          sealed = call.arguments as String;
          return true;
        }
        if (call.method == 'lock') return true;
        throw MissingPluginException('No native account operation permitted');
      });
}

class _HTTP implements HttpClientAdapter {
  final requests = <RequestOptions>[];
  Future<ResponseBody> Function(RequestOptions)? respond;
  @override
  Future<ResponseBody> fetch(
      RequestOptions o, Stream<Uint8List>? s, Future<void>? c) async {
    requests.add(o);
    return respond == null ? _body('false') : await respond!(o);
  }

  @override
  void close({bool force = false}) {}
}

ResponseBody _body(String show, {String id = '', String image = ''}) =>
    ResponseBody.fromString(
        '{"code":0,"data":{"show":$show,"captchaId":"$id","image":"$image"}}',
        200,
        headers: {
          Headers.contentTypeHeader: ['application/json']
        });

class _Inert extends Fake {
  final onStart = InternalFinalCallback<void>(callback: () {});
  final onDelete = InternalFinalCallback<void>(callback: () {});
}

class _IM extends _Inert implements IMController {
  final events = <String>[];
  Future<void> Function()? reset;
  Future<void> Function()? preparation;
  @override
  Future<void> prepareLogin() async {
    events.add('prepare');
    await preparation?.call();
  }

  @override
  Future<void> reconfigureServer() async {
    events.add('reset');
    expect(DataSp.getLoginCertificate(), isNull);
    expect(DataSp.getServerConfig()!.keys, ['baseUrl']);
    await reset?.call();
  }
}

class _Push extends _Inert implements PushController {}

class _App extends _Inert implements AppController {
  @override
  final clientConfigMap = <String, dynamic>{}.obs;
  int brandRefreshes = 0;
  @override
  Future<void> queryPublicBrand() async {
    brandRefreshes++;
  }
}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  late _NativeState prefs;
  late _HTTP http;
  late _IM im;
  late _App app;
  setUp(() async {
    Get.testMode = true;
    prefs = _NativeState()..install();
    SharedPreferences.setMockInitialValues({});
    await DataSp.init();
    http = _HTTP();
    dio = Dio()..httpClientAdapter = http;
    im = _IM();
    app = _App();
    Get.put<IMController>(im, permanent: true);
    Get.put<PushController>(_Push(), permanent: true);
    Get.put<AppController>(app, permanent: true);
    PackageInfo.setMockInitialValues(
        appName: 'PUTY内部通讯',
        packageName: 'synthetic.login.only',
        version: '3.8.19',
        buildNumber: '119',
        buildSignature: '');
  });
  tearDown(() {
    dio.close(force: true);
    Get.reset();
  });

  test(
      'initial selection and same-value no-op preserve the existing certificate',
      () async {
    await DataSp.putServerConfig({'baseUrl': Config.wanOrigin});
    await DataSp.putLoginCertificate(LoginCertificate.fromJson(
        {'userID': 'synthetic', 'sessionToken': 'synthetic-session'}));
    final logic = LoginLogic();
    expect(logic.internalNetwork.value, isFalse);
    final writes = prefs.writes;
    await logic.switchNetwork(false);
    expect(prefs.writes, writes);
    expect(im.events, isEmpty);
    expect(http.requests, isEmpty);
    expect(DataSp.getLoginCertificate()!.sessionToken, 'synthetic-session');
    logic.onClose();
  });

  test(
      'switch removes old endpoint overrides, waits for SDK reset and refreshes captcha',
      () async {
    await DataSp.putServerConfig({
      'baseUrl': Config.lanOrigin,
      'apiAddr': 'https://obsolete.invalid',
      'wsAddr': 'wss://obsolete.invalid'
    });
    final logic = LoginLogic();
    logic.phoneCtrl.text = 'synthetic-account';
    logic.pwdCtrl.text = 'synthetic-password';
    logic.verificationCodeCtrl.text = 'old-captcha';
    logic.captchaId = 'old-id';
    final ack = Completer<void>();
    im.reset = () => ack.future;
    final operation = logic.switchNetwork(false);
    await Future<void>.delayed(Duration.zero);
    expect(im.events, ['prepare', 'reset']);
    expect(logic.switchingNetwork.value, isTrue);
    await logic.switchNetwork(true);
    await logic.login();
    expect(im.events, ['prepare', 'reset']);
    expect(Config.imApiUrl, '${Config.wanOrigin}/api');
    expect(Config.imWsUrl, 'wss://im.szputy.com:9885/ws');
    expect(Config.appAuthUrl, '${Config.wanOrigin}/chat');
    expect(http.requests, isEmpty);
    ack.complete();
    await operation;
    expect(
        http.requests.single.uri.toString(), '${Config.wanOrigin}/sso/captcha');
    expect(app.brandRefreshes, 1);
    expect(logic.phoneCtrl.text, 'synthetic-account');
    expect(logic.pwdCtrl.text, 'synthetic-password');
    expect(logic.verificationCodeCtrl.text, isEmpty);
    expect(logic.internalNetwork.value, isFalse);
    expect(logic.switchingNetwork.value, isFalse);
    await DataSp.init();
    expect(Config.serverBaseUrl, Config.wanOrigin);
    logic.onClose();
  });

  test(
      'SDK failure reflects saved selection, blocks login, and retries the same selection',
      () async {
    final logic = LoginLogic();
    im.reset = () async => throw StateError('synthetic reset failure');
    await logic.switchNetwork(false);
    expect(Config.serverBaseUrl, Config.wanOrigin);
    expect(logic.internalNetwork.value, isFalse);
    expect(logic.networkError.value, isNotEmpty);
    expect(logic.canLogin, isFalse);
    final events = im.events.length;
    await logic.login();
    expect(im.events.length, events);
    im.reset = null;
    await logic.switchNetwork(false);
    expect(im.events.where((e) => e == 'reset'), hasLength(2));
    expect(logic.networkError.value, isEmpty);
    expect(logic.switchingNetwork.value, isFalse);
    logic.onClose();
  });

  test(
      'failed protected commit restores the prior connection before selecting it',
      () async {
    await DataSp.putServerConfig({'baseUrl': Config.lanOrigin});
    final logic = LoginLogic();
    prefs.failWrites.add(prefs.writes + 2);
    await logic.switchNetwork(false);
    expect(im.events, ['prepare']);
    expect(logic.networkError.value, isNotEmpty);
    expect(logic.networkConfigurationKnown.value, isTrue);
    expect(logic.internalNetwork.value, isTrue);
    await DataSp.init();
    expect(Config.serverBaseUrl, Config.lanOrigin);
    logic.onClose();
  });

  test(
      'failed restore presents no claimed selection and a later selection can recover',
      () async {
    final logic = LoginLogic();
    prefs.failWrites.addAll([prefs.writes + 2, prefs.writes + 3]);
    await logic.switchNetwork(false);
    expect(logic.networkConfigurationKnown.value, isFalse);
    expect(logic.canLogin, isFalse);
    prefs.failWrites.clear();
    await logic.switchNetwork(true);
    expect(logic.networkConfigurationKnown.value, isTrue);
    expect(logic.internalNetwork.value, isTrue);
    expect(logic.networkError.value, isEmpty);
    expect(im.events.where((e) => e == 'reset'), hasLength(1));
    logic.onClose();
  });

  test('late captcha from the previous endpoint cannot replace the new captcha',
      () async {
    final old = Completer<ResponseBody>();
    http.respond = (o) async => o.uri.origin == Config.lanOrigin
        ? await old.future
        : _body('true', id: 'new-captcha');
    final logic = LoginLogic();
    final previous = logic.loadCaptcha();
    await Future<void>.delayed(Duration.zero);
    await logic.switchNetwork(false);
    expect(logic.captchaId, 'new-captcha');
    old.complete(_body('true', id: 'old-captcha'));
    await previous;
    expect(logic.captchaId, 'new-captcha');
    expect(logic.captchaRequired.value, isTrue);
    logic.onClose();
  });

  testWidgets(
      'inline radios hide endpoints, preserve inputs and disable during switching/login',
      (tester) async {
    tester.view.physicalSize = const Size(320, 700);
    tester.view.devicePixelRatio = 1;
    addTearDown(() {
      tester.view.resetPhysicalSize();
      tester.view.resetDevicePixelRatio();
    });
    final logic = Get.put<LoginLogic>(LoginLogic());
    await tester.pumpWidget(ScreenUtilInit(
        designSize: const Size(375, 812),
        builder: (_, __) => GetMaterialApp(home: LoginPage())));
    await tester.pumpAndSettle();
    expect(find.text('内部通讯'), findsOneWidget);
    expect(find.text('公网通讯'), findsOneWidget);
    expect(find.textContaining('192.168.'), findsNothing);
    expect(find.textContaining('szputy.com'), findsNothing);
    expect(find.text('内网 / 公网连接'), findsNothing);
    logic.phoneCtrl.text = 'synthetic-account';
    logic.pwdCtrl.text = 'synthetic-password';
    await tester.pump();
    final public = find.byKey(const Key('login-network-public'));
    await tester.ensureVisible(public);
    await tester.pumpAndSettle();
    final ack = Completer<void>();
    im.reset = () => ack.future;
    await tester.tap(public);
    await tester.pump();
    expect(tester.widget<RadioListTile<bool>>(public).onChanged, isNull);
    expect(tester.widget<FilledButton>(find.byType(FilledButton)).onPressed,
        isNull);
    expect(find.text('正在切换通讯连接…'), findsOneWidget);
    ack.complete();
    await tester.runAsync(() async {
      await Future<void>.delayed(const Duration(milliseconds: 50));
    });
    await tester.pumpAndSettle();
    expect(logic.internalNetwork.value, isFalse);
    expect(logic.phoneCtrl.text, 'synthetic-account');
    expect(logic.pwdCtrl.text, 'synthetic-password');
    logic.loggingIn.value = true;
    await tester.pump();
    expect(tester.widget<RadioListTile<bool>>(public).onChanged, isNull);
    logic.loggingIn.value = false;
    await tester.pump();
    expect(tester.widget<RadioListTile<bool>>(public).onChanged, isNotNull);
    expect(tester.takeException(), isNull);
    await tester.pumpWidget(const SizedBox.shrink());
    await tester.pumpAndSettle();
  });
}
