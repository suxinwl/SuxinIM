import 'dart:async';
import 'dart:convert';
import 'dart:typed_data';
import 'dart:io';
import 'dart:ui' as ui;

import 'package:app_im/app_im.dart';
import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter/rendering.dart';
import 'package:flutter/services.dart';
import 'package:flutter_screenutil/flutter_screenutil.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:get/get.dart';
import 'package:openim/core/controller/app_controller.dart';
import 'package:openim/core/controller/im_controller.dart';
import 'package:openim/core/controller/push_controller.dart';
import 'package:openim/core/session_epoch.dart';
import 'package:openim/core/session_renewal.dart';
import 'package:openim/core/vault_reauthorization.dart';
import 'package:openim/pages/splash/splash_logic.dart';
import 'package:openim/pages/splash/splash_view.dart';
import 'package:openim_common/openim_common.dart';
import 'package:rxdart/rxdart.dart';
import 'package:shared_preferences/shared_preferences.dart';

class _App extends Fake implements AppController {
  @override
  final onStart = InternalFinalCallback<void>(callback: () {});
  @override
  final onDelete = InternalFinalCallback<void>(callback: () {});
}

class _Push extends Fake implements PushController {
  @override
  final onStart = InternalFinalCallback<void>(callback: () {});
  @override
  final onDelete = InternalFinalCallback<void>(callback: () {});
  int logins = 0;
  @override
  void login(String uid) {
    logins++;
  }
}

class _SplashIM extends Fake implements IMController {
  @override
  final onStart = InternalFinalCallback<void>(callback: () {});
  @override
  final onDelete = InternalFinalCallback<void>(callback: () {});
  @override
  bool vaultReauthorizationPending = true;
  @override
  final initializedSubject = PublishSubject<bool>();
  @override
  final session = SessionEpoch();
  int resumed = 0;
  @override
  Future<void> prepareLogin() async {}
  @override
  void prepareAutomaticVaultRecovery() {
    vaultReauthorizationPending = true;
  }

  @override
  Future<void> resumeAfterVaultBackground() async {
    resumed++;
  }

  @override
  Future<SessionAttempt> login(String userID, String token) async {
    final proof = DataSp.getLoginCertificate()!;
    await NativeLocalVault.unlock(proof);
    final attempt = session.begin(userID, token, proof.chatToken, 2);
    session.activate(attempt);
    vaultReauthorizationPending = false;
    return attempt;
  }
}

class _Adapter implements HttpClientAdapter {
  _Adapter(this.response);
  final Future<ResponseBody> Function(RequestOptions) response;
  @override
  Future<ResponseBody> fetch(RequestOptions options, Stream<Uint8List>? stream,
          Future<void>? cancel) =>
      response(options);
  @override
  void close({bool force = false}) {}
}

void main() {
  final binding = TestWidgetsFlutterBinding.ensureInitialized();
  final messenger =
      TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
  setUpAll(()async{
    if(Platform.environment.containsKey('PUTY_RECOVERY_FIXTURE_PNG')){
      for(final font in {
        'Roboto':'C:/Windows/Fonts/msyh.ttc',
        'MaterialIcons':'O:/tools/flutter-3.32.8/bin/cache/artifacts/material_fonts/MaterialIcons-Regular.otf',
      }.entries){
        final loader=FontLoader(font.key);
        loader.addFont(File(font.value).readAsBytes().then(ByteData.sublistView));
        await loader.load();
      }
    }
  });
  const sdk = MethodChannel('flutter_openim_sdk');
  late String settings;
  late Future<Object?> Function(MethodCall) vaultAction;
  LoginCertificate certificate([String owner = 'A']) =>
      LoginCertificate.fromJson({
        'userID': 'synthetic-$owner',
        'imToken': 'synthetic-im-$owner',
        'chatToken': 'synthetic-chat-$owner',
        'sessionToken': 'synthetic-sso-$owner',
        'expiresAt': 4102444800,
      });
  Future<void> nativeEvent(String reason) async {
    final done = Completer<void>();
    ServicesBinding.instance.channelBuffers.push(
        NativeLocalVault.channel.name,
        const StandardMethodCodec()
            .encodeMethodCall(MethodCall('onVaultLocked', reason)),
        (_) => done.complete());
    await done.future;
    await Future<void>.delayed(Duration.zero);
  }

  setUp(() async {
    Get.testMode = true;
    settings = '{}';
    SharedPreferences.setMockInitialValues({});
    binding.handleAppLifecycleStateChanged(AppLifecycleState.resumed);
    vaultAction = (call) async {
      if (call.method == 'unlock')
        return {
          'userID': (call.arguments as Map)['userID'],
          'actorGeneration': 1
        };
      if (call.method == 'authorization/current') return true;
      if (call.method == 'lock' || call.method == 'legacy/finalize')
        return true;
      throw StateError('Unexpected native method ${call.method}');
    };
    messenger.setMockMethodCallHandler(NativeLocalVault.channel, (call) async {
      if (call.method == 'preferences/read') return settings;
      if (call.method == 'preferences/write') {
        settings = call.arguments as String;
        return true;
      }
      return vaultAction(call);
    });
    NativeLocalVault.install();
    await DataSp.init();
    await DataSp.putLoginCertificate(certificate());
    Config.cachePath = '/synthetic-private-sdk/';
    AppIm.enterprise.configure(
        baseUrl: () => 'https://example.invalid/enterprise/v1',
        imToken: () => null,
        chatToken: () => null,
        platform: () => 2,
        ready: () => false,
        userID: () => null);
    Get.put<AppController>(_App(), permanent: true);
  });
  tearDown(() async {
    await NativeLocalVault.lock();
    messenger.setMockMethodCallHandler(sdk, null);
    AppIm.enterprise.dispose();
    Get.reset();
  });
  test('temporary failures are distinct from explicit authority denials', () {
    for (final error in [
      HrmSessionError(503, 'synthetic'),
      HrmSessionError(404, 'synthetic'),
      HrmSessionError(409, 'synthetic'),
      HrmSessionError(429, 'synthetic'),
      TimeoutException('synthetic'),
      PlatformException(code: 'LOCAL_VAULT_REAUTHORIZATION_REQUIRED')
    ]) {
      expect(isTemporarySessionFailure(error), isTrue);
      expect(isDefiniteSessionDenial(error), isFalse);
    }
    for (final error in [
      HrmSessionError(401, 'synthetic'),
      HrmSessionError(403, 'synthetic'),
      PlatformException(code: 'LOCAL_VAULT_ACCESS_DENIED')
    ]) {
      expect(isDefiniteSessionDenial(error), isTrue);
      expect(isTemporarySessionFailure(error), isFalse);
    }
  });
  test('native expired lease closes views but retains login proof', () async {
    await NativeLocalVault.unlock(certificate());
    final before = NativeLocalVault.generation;
    var hard = 0, soft = 0;
    final a = NativeLocalVault.locked.listen((_) => hard++),
        b = NativeLocalVault.backgroundLocked.listen((_) => soft++);
    await nativeEvent('ONLINE_REAUTHORIZATION_REQUIRED');
    expect(NativeLocalVault.unlocked, isFalse);
    expect(NativeLocalVault.generation, greaterThan(before));
    expect(DataSp.imToken, certificate().imToken);
    expect(hard, 0);
    expect(soft, 1);
    await expectLater(
        NativeLocalVault.page('synthetic', before: 1), throwsStateError);
    await a.cancel();
    await b.cancel();
  });
  test('resume checks actual native current lease and cannot unlock offline',
      () async {
    await NativeLocalVault.unlock(certificate());
    var calls = 0;
    vaultAction = (call) async {
      if (call.method == 'authorization/current') {
        calls++;
        return false;
      }
      if (call.method == 'lock') return true;
      throw StateError('No offline unlock permitted');
    };
    expect(await NativeLocalVault.ensureCurrentAuthorization(), isFalse);
    expect(calls, 1);
    expect(NativeLocalVault.unlocked, isFalse);
    expect(DataSp.userID, certificate().userID);
  });
  test(
      'fresh online unlock waits its response; late old response cannot unlock new owner',
      () async {
    final held = Completer<Map<String, Object>>();
    var calls = 0;
    vaultAction = (call) async {
      if (call.method == 'unlock' && ++calls == 1) return held.future;
      if (call.method == 'unlock')
        return {
          'userID': (call.arguments as Map)['userID'],
          'actorGeneration': 2
        };
      return true;
    };
    final old = NativeLocalVault.unlock(certificate());
    final rejected = expectLater(old, throwsStateError);
    await Future<void>.delayed(Duration.zero);
    expect(NativeLocalVault.unlocked, isFalse);
    await NativeLocalVault.suspend();
    await DataSp.putLoginCertificate(certificate('B'));
    await NativeLocalVault.unlock(certificate('B'));
    final generation = NativeLocalVault.generation;
    held.complete({'userID': certificate().userID, 'actorGeneration': 1});
    await rejected;
    expect(NativeLocalVault.unlocked, isTrue);
    expect(NativeLocalVault.generation, generation);
    expect(DataSp.userID, certificate('B').userID);
  });
  test('logout while online unlock is pending rejects its late response',
      () async {
    final held = Completer<Map<String, Object>>();
    vaultAction = (call) async => call.method == 'unlock' ? held.future : true;
    final old = NativeLocalVault.unlock(certificate());
    final rejected = expectLater(old, throwsStateError);
    await Future<void>.delayed(Duration.zero);
    await NativeLocalVault.lock();
    await DataSp.removeLoginCertificate();
    held.complete({'userID': certificate().userID, 'actorGeneration': 1});
    await rejected;
    expect(NativeLocalVault.unlocked, isFalse);
    expect(DataSp.getLoginCertificate(), isNull);
  });
  test('late resume lease check cannot cancel a newer unlock in progress',
      () async {
    await NativeLocalVault.unlock(certificate());
    final checkLease = Completer<bool>(),
        newUnlock = Completer<Map<String, Object>>();
    var locks = 0;
    vaultAction = (call) async {
      if (call.method == 'authorization/current') return checkLease.future;
      if (call.method == 'unlock') return newUnlock.future;
      if (call.method == 'lock') {
        locks++;
        return true;
      }
      throw StateError('unexpected');
    };
    final oldCheck = NativeLocalVault.ensureCurrentAuthorization();
    await Future<void>.delayed(Duration.zero);
    await DataSp.putLoginCertificate(certificate('B'));
    final fresh = NativeLocalVault.unlock(certificate('B'));
    checkLease.complete(false);
    expect(await oldCheck, isFalse);
    expect(locks, 0);
    newUnlock
        .complete({'userID': certificate('B').userID, 'actorGeneration': 2});
    await fresh;
    expect(NativeLocalVault.unlocked, isTrue);
    expect(DataSp.userID, certificate('B').userID);
  });
  Future<void> renewal(Object error, {required bool late}) async {
    final session = SessionEpoch();
    var cert = certificate();
    final attempt = session.begin(cert.userID, cert.imToken, cert.chatToken, 2);
    session.activate(attempt);
    var soft = 0, hard = 0;
    final held = Completer<LoginCertificate>();
    final run = renewCurrentSSOSession(
        session: session,
        readCertificate: () => cert,
        refresh: () => held.future,
        unlock: NativeLocalVault.unlock,
        login: (_) async {},
        lock: () async {
          hard++;
        },
        kick: () {
          hard++;
          session.invalidate();
        },
        restoring: () => false,
        suspend: () async {
          soft++;
          await NativeLocalVault.suspend();
          session.invalidate();
        });
    if (late) {
      cert = certificate('B');
      final next = session.begin(cert.userID, cert.imToken, cert.chatToken, 2);
      session.activate(next);
    }
    held.completeError(error);
    await run;
    expect(soft, late || isDefiniteSessionDenial(error) ? 0 : 1);
    expect(hard, !late && isDefiniteSessionDenial(error) ? 2 : 0);
    expect(DataSp.getLoginCertificate(), isNotNull);
  }

  test('503 renewal suspends without kicking or clearing certificate',
      () => renewal(HrmSessionError(503, 'synthetic'), late: false));
  test('401 and 403 renewal still invoke hard lock and kick', () async {
    await renewal(HrmSessionError(401, 'synthetic'), late: false);
    await renewal(HrmSessionError(403, 'synthetic'), late: false);
  });
  test('late transport failure cannot suspend replacement account',
      () => renewal(HrmSessionError(503, 'synthetic'), late: true));
  test(
      'actual controller init false keeps reset pending then retry reinitializes',
      () async {
    final controller = IMController();
    final calls = <String>[];
    var initialized = true;
    messenger.setMockMethodCallHandler(sdk, (call) async {
      calls.add(call.method);
      if (call.method == 'initSDK') return initialized;
      if (call.method == 'unInitSDK') return true;
      return null;
    });
    await controller.reconfigureServer();
    controller.vaultReauthorizationPending = true;
    controller.requireVaultReauthorization();
    initialized = false;
    await controller.resumeAfterVaultBackground();
    initialized = true;
    await controller.resumeAfterVaultBackground();
    expect(calls.where((x) => x == 'initSDK').length, 3);
    expect(calls.where((x) => x == 'unInitSDK').length, 3);
    expect(controller.vaultReauthorizationPending, isTrue);
    expect(NativeLocalVault.unlocked, isFalse);
    expect(DataSp.imToken, certificate().imToken);
  });
  test(
      'actual recovering controller SDK network failure retains proof and locks archive',
      () async {
    final controller = IMController();
    messenger.setMockMethodCallHandler(sdk, (call) async {
      if (call.method == 'initSDK' || call.method == 'unInitSDK') return true;
      if (call.method == 'getLoginStatus') return 1;
      if (call.method == 'login')
        throw PlatformException(code: 'SYNTHETIC_NETWORK_UNAVAILABLE');
      return null;
    });
    await controller.reconfigureServer();
    controller.vaultReauthorizationPending = true;
    await expectLater(
        controller.login(certificate().userID, certificate().imToken),
        throwsA(isA<PlatformException>()));
    expect(DataSp.imToken, certificate().imToken);
    expect(NativeLocalVault.unlocked, isFalse);
    expect(controller.session.active, isFalse);
  });
  test('first SDK initialization false can recover and admit a later login',
      () async {
    final controller = IMController();
    var initialized = false;
    messenger.setMockMethodCallHandler(sdk, (call) async {
      if (call.method == 'initSDK') return initialized;
      if (call.method == 'unInitSDK') return true;
      if (call.method == 'getLoginStatus')
        throw PlatformException(code: 'SYNTHETIC_REACHED_AFTER_READY');
      return null;
    });
    await expectLater(controller.reconfigureServer(), throwsStateError);
    controller.prepareAutomaticVaultRecovery();
    initialized = true;
    await controller.resumeAfterVaultBackground();
    await expectLater(
        controller.login(certificate().userID, certificate().imToken),
        throwsA(isA<PlatformException>().having((e) => e.code,
            'ready gate reopened', 'SYNTHETIC_REACHED_AFTER_READY')));
    expect(DataSp.imToken, certificate().imToken);
    expect(NativeLocalVault.unlocked, isFalse);
  });
  test('actual hard invalidation during recovery removes the stored proof',
      () async {
    final controller = IMController()..vaultReauthorizationPending = true;
    controller.kickedOffline();
    await controller.prepareLogin();
    expect(DataSp.getLoginCertificate(), isNull);
    expect(controller.vaultReauthorizationPending, isFalse);
    expect(NativeLocalVault.unlocked, isFalse);
  });
  testWidgets(
      'ready event before Splash mount still recovers; retry is singleflight and offline UI hides chat',
      (tester) async {
    tester.view.physicalSize=const Size(393,873);tester.view.devicePixelRatio=1;
    addTearDown(tester.view.resetPhysicalSize);addTearDown(tester.view.resetDevicePixelRatio);
    final im = _SplashIM();
    Get.put<IMController>(im, permanent: true);
    Get.put<PushController>(_Push(), permanent: true);
    final previous = dio.httpClientAdapter;
    final held = Completer<ResponseBody>();
    var requests = 0;
    dio.httpClientAdapter = _Adapter((request) {
      expect(request.uri.path, '/sso/refresh');
      requests++;
      return held.future;
    });
    addTearDown(() => dio.httpClientAdapter = previous);
    late SplashLogic logic;
    await tester.pumpWidget(ScreenUtilInit(
        designSize: const Size(375, 812),
        builder: (_, __) => GetMaterialApp(initialRoute: '/home', getPages: [
              GetPage(
                  name: '/home',
                  page: () => const Scaffold(body: Text('SYNTHETIC_OLD_CHAT'))),
              GetPage(
                  name: '/splash',
                  page: () => RepaintBoundary(key:const ValueKey('protected-recovery'),child:SplashPage()),
                  binding: BindingsBuilder(() {
                    logic = Get.put(SplashLogic());
                  })),
            ])));
    expect(find.text('SYNTHETIC_OLD_CHAT'), findsOneWidget);
    im.initializedSubject
        .add(true); // Deliberately lost before route subscribes.
    await tester.pump();
    Get.offAllNamed('/splash');
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 500));
    expect(im.resumed, 1);
    expect(requests, 1);
    expect(logic.busy.value, isTrue);
    await logic.retry();
    await logic.retry();
    expect(requests, 1);
    held.complete(
        ResponseBody.fromString(jsonEncode({'code': 503}), 503, headers: {
      Headers.contentTypeHeader: ['application/json']
    }));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 30));
    expect(find.text('重试恢复'), findsOneWidget);
    expect(find.text('使用其他账号'), findsOneWidget);
    expect(find.text('SYNTHETIC_OLD_CHAT'), findsNothing);
    final style = DefaultTextStyle.of(
            tester.element(find.text('会话恢复未完成，本地档案仍已锁定。请检查网络后重试')))
        .style;
    expect(style.fontSize, lessThan(30));
    expect(
        style.decoration?.contains(TextDecoration.underline) ?? false, isFalse);
    expect(tester.widget<Scaffold>(find.byType(Scaffold).first).backgroundColor,
        Colors.white);
    expect(DataSp.imToken, certificate().imToken);
    expect(logic.busy.value, isFalse);
    expect(NativeLocalVault.unlocked, isFalse);
    binding.handleAppLifecycleStateChanged(AppLifecycleState.paused);
    await logic.retry();expect(requests,1);
    binding.handleAppLifecycleStateChanged(AppLifecycleState.resumed);
    final picturePath=Platform.environment['PUTY_RECOVERY_FIXTURE_PNG'];
    if(picturePath!=null){
      final boundary=tester.renderObject<RenderRepaintBoundary>(find.byKey(const ValueKey('protected-recovery')));
      await tester.runAsync(()async{
        final image=await boundary.toImage(pixelRatio:1);
        try{final bytes=await image.toByteData(format:ui.ImageByteFormat.png);expect(bytes,isNotNull);
          final target=File(picturePath);expect(target.existsSync(),isFalse);await target.writeAsBytes(bytes!.buffer.asUint8List(),flush:true);
        }finally{image.dispose();}
      });
    }
    await im.initializedSubject.close();
  });
  testWidgets(
      'cold stored certificate plus503 stays protected then fresh online retry succeeds',
      (tester) async {
    final im = _SplashIM()..vaultReauthorizationPending = false;
    final push = _Push();
    Get.put<IMController>(im, permanent: true);
    Get.put<PushController>(push, permanent: true);
    final previous = dio.httpClientAdapter;
    var online = false, requests = 0;
    dio.httpClientAdapter = _Adapter((request) async {
      expect(request.uri.path, '/sso/refresh');
      requests++;
      return ResponseBody.fromString(
          jsonEncode({
            'code': online ? 0 : 503,
            if (online) 'data': certificate().toJson()
          }),
          online ? 200 : 503,
          headers: {
            Headers.contentTypeHeader: ['application/json']
          });
    });
    addTearDown(() => dio.httpClientAdapter = previous);
    late SplashLogic logic;
    await tester.pumpWidget(ScreenUtilInit(
        designSize: const Size(375, 812),
        builder: (_, __) => GetMaterialApp(initialRoute: '/splash', getPages: [
              GetPage(
                  name: '/splash',
                  page: () => SplashPage(),
                  binding: BindingsBuilder(() {
                    logic = Get.put(SplashLogic());
                  })),
              GetPage(
                  name: '/home',
                  page: () =>
                      const Scaffold(body: Text('SYNTHETIC_VERIFIED_HOME'))),
            ])));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 40));
    expect(find.text('重试恢复'), findsOneWidget);
    expect(find.text('SYNTHETIC_VERIFIED_HOME'), findsNothing);
    expect(DataSp.imToken, certificate().imToken);
    expect(NativeLocalVault.unlocked, isFalse);
    expect(push.logins, 0);
    online = true;
    await tester.runAsync(logic.retry);
    await tester.pumpAndSettle();
    expect(requests, 2);
    expect(find.text('SYNTHETIC_VERIFIED_HOME'), findsOneWidget);
    expect(NativeLocalVault.unlocked, isTrue);
    expect(push.logins, 1);
    expect(im.session.active, isTrue);
    await im.initializedSubject.close();
  });
}
