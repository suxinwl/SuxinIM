import 'dart:typed_data';
import 'package:dio/dio.dart';
// Real epoch, SplashLogic and HomeLogic. SDK transport and Get dependencies are
// inert doubles: this is not a native connection or end-to-end login test.
import 'dart:async';
import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:get/get.dart';
import 'package:openim/core/controller/app_controller.dart';
import 'package:openim/core/controller/im_controller.dart';
import 'package:openim/core/controller/push_controller.dart';
import 'package:openim/core/session_epoch.dart';
import 'package:openim/pages/home/home_logic.dart';
import 'package:openim/pages/splash/splash_logic.dart';
import 'package:openim_common/openim_common.dart';
import 'package:rxdart/rxdart.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'native_settings_mock.dart';

// Get.find starts its dependency lifecycle. These fakes deliberately avoid the
// unrelated audio, notifications, native SDK and push-service constructors.
class _SSOAdapter implements HttpClientAdapter {
 @override Future<ResponseBody> fetch(RequestOptions options,Stream<Uint8List>? stream,Future<void>? cancel)async =>
   ResponseBody.fromString(jsonEncode({'code':0,'data':{'userID':'synthetic','imToken':'im-one','chatToken':'chat-one','sessionToken':'sso-one','expiresAt':DateTime.now().millisecondsSinceEpoch~/1000+14400}}),200,headers:{Headers.contentTypeHeader:['application/json']});
 @override void close({bool force=false}){}
}

class _InertDependency extends Fake {
  final onStart = InternalFinalCallback<void>(callback: () {});
  final onDelete = InternalFinalCallback<void>(callback: () {});
}

class _IM extends _InertDependency implements IMController {
  @override
  final session = SessionEpoch();
  @override
  final initializedSubject = PublishSubject<bool>();
  final pendingLogin = Completer<SessionAttempt>();
  int loginCalls = 0;

  @override
  Future<void> prepareLogin() async {}

  @override
  Future<SessionAttempt> login(String userID, String token) {
    loginCalls++;
    return pendingLogin.future;
  }
}

class _Push extends _InertDependency implements PushController {
  final loggedIn = <String>[];
  @override
  void login(String userID) => loggedIn.add(userID);
}

class _App extends _InertDependency implements AppController {}

class _Cache extends _InertDependency implements CacheController {
  int calls = 0;
  @override
  void initCallRecords() {
    calls++;
  }

  @override
  void initFavoriteEmoji() {
    calls++;
  }
}

void _register(_IM im, _Push push, {_Cache? cache}) {
  Get.put<IMController>(im);
  Get.put<PushController>(push);
  Get.put<AppController>(_App());
  Get.put<CacheController>(cache ?? _Cache());
}

Future<void> _drain() => Future<void>.delayed(Duration.zero);

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  setUp(() async {
    installNativeSettingsMock();
    Get.testMode = true;
    dio=Dio()..httpClientAdapter=_SSOAdapter();
    SharedPreferences.setMockInitialValues({});
    await DataSp.init();
  });
  tearDown(Get.reset);

  test('cached local completion is not an established business session',
      () async {
    final session = SessionEpoch();
    final attempt = session.begin('synthetic', 'im-one', 'chat-one', 2);
    expect(session.current(attempt), isTrue);
    expect(session.established(attempt), isFalse);
    final waiting = session.waitForConnection(attempt);
    session.connected();
    await waiting;
    // The production controller still has to verify the captured token pair.
    expect(session.established(attempt), isFalse);
    session.activate(attempt);
    expect(session.established(attempt), isTrue);
  });

  test(
      'kick before a waiter attaches is fail-closed without an error completer',
      () async {
    final session = SessionEpoch();
    final attempt = session.begin('synthetic', 'im-one', 'chat-one', 2);
    session.invalidate();
    await _drain();
    await expectLater(session.waitForConnection(attempt), throwsStateError);
    expect(() => session.activate(attempt), throwsStateError);
    expect(session.active, isFalse);
  });

  test('kick releases a connection waiter and stale success cannot activate it',
      () async {
    final session = SessionEpoch();
    final old = session.begin('synthetic', 'im-one', 'chat-one', 2);
    final rejected =
        expectLater(session.waitForConnection(old), throwsStateError);
    session.invalidate();
    await rejected;
    final current = session.begin('synthetic', 'im-two', 'chat-two', 2);
    session.connected();
    await session.waitForConnection(current);
    session.activate(current);
    expect(() => session.activate(old), throwsStateError);
    expect(session.established(current), isTrue);
    expect(session.established(old), isFalse);
  });

  test('an abandoned connection times out without establishing the attempt',
      () async {
    final session = SessionEpoch();
    final attempt = session.begin('synthetic', 'im-one', 'chat-one', 2);
    await expectLater(
      session.waitForConnection(attempt, timeout: Duration.zero),
      throwsA(isA<TimeoutException>()),
    );
    expect(session.active, isFalse);
    session.invalidate();
  });

  testWidgets(
      'production Splash does not navigate or register push for a late invalidated login',
      (tester) async {
    final im = _IM();
    final push = _Push();
    _register(im, push);
    await tester.runAsync(()async=>await DataSp.putLoginCertificate(LoginCertificate.fromJson({
      'userID': 'synthetic',
      'imToken': 'im-one',
      'chatToken': 'chat-one',
      'sessionToken':'sso-one',
      'expiresAt':DateTime.now().millisecondsSinceEpoch~/1000+14400,
    })));
    await tester.pumpWidget(GetMaterialApp(
      initialRoute: '/splash',
      getPages: [
        GetPage(
            name: '/splash', page: () => const Scaffold(body: Text('splash'))),
        GetPage(name: '/home', page: () => const Scaffold(body: Text('home'))),
        GetPage(
            name: '/login', page: () => const Scaffold(body: Text('login'))),
      ],
    ));
    final splash = SplashLogic();
    splash.onInit();
    addTearDown(() async {
      splash.onClose();
      await im.initializedSubject.close();
    });
    im.initializedSubject.add(true);
    // Alternate the widget clock and real platform-reply boundary. A native
    // encrypted-preference ACK must complete before Splash reaches SDK login.
    for(var i=0;i<50&&im.loginCalls==0;i++){
      await tester.pump(const Duration(milliseconds:1));
      await tester.runAsync(()=>Future<void>.delayed(const Duration(milliseconds:10)));
    }
    expect(im.loginCalls, 1);
    final attempt = im.session.begin('synthetic', 'im-one', 'chat-one', 2);
    im.session.connected();
    im.session.activate(attempt);
    im.session.invalidate(); // real global callback invalidates synchronously
    final newer = im.session.begin('synthetic', 'im-two', 'chat-two', 2);
    await tester.runAsync(()async=>await DataSp.putLoginCertificate(LoginCertificate.fromJson({
      'userID': 'synthetic',
      'imToken': 'im-two',
      'chatToken': 'chat-two',
    })));
    im.pendingLogin.complete(attempt); // an old native/API success arrives late
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 50));
    expect(push.loggedIn, isEmpty);
    expect(find.text('home'), findsNothing);
    expect(Get.currentRoute, '/splash');
    expect(DataSp.imToken, 'im-two');
    expect(DataSp.chatToken, 'chat-two');
    expect(im.session.current(newer), isTrue);
  });

  test('production Home does no SDK work before active or after a kick',
      () async {
    final im = _IM();
    final push = _Push();
    final cache = _Cache();
    _register(im, push, cache: cache);
    final home = HomeLogic();
    final methods = <String>[];
    const channel = MethodChannel('flutter_openim_sdk');
    final messenger =
        TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
    messenger.setMockMethodCallHandler(channel, (call) async {
      methods.add(call.method);
      return '[]';
    });
    addTearDown(() {
      messenger.setMockMethodCallHandler(channel, null);
      home.onClose();
      im.initializedSubject.close();
    });
    im.session.begin('synthetic', 'im-one', 'chat-one', 2);
    home.onReady();
    home.getUnhandledFriendApplicationCount();
    home.getUnhandledGroupApplicationCount();
    await _drain();
    expect(methods, isEmpty);
    expect(cache.calls, 0);
    im.session.invalidate();
    home.onReady();
    await _drain();
    expect(methods, isEmpty);
    expect(cache.calls, 0);
  });

  test(
      'production Home discards an old SDK result after another login is active',
      () async {
    final im = _IM();
    _register(im, _Push());
    final home = HomeLogic();
    final response = Completer<String>();
    var nativeCalls = 0;
    const channel = MethodChannel('flutter_openim_sdk');
    final messenger =
        TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
    messenger.setMockMethodCallHandler(channel, (call) {
      expect(call.method, 'getFriendApplicationListAsRecipient');
      nativeCalls++;
      return response.future;
    });
    addTearDown(() {
      messenger.setMockMethodCallHandler(channel, null);
      home.onClose();
      im.initializedSubject.close();
    });
    final old = im.session.begin('synthetic', 'im-one', 'chat-one', 2);
    im.session.activate(old);
    home.unhandledFriendApplicationCount.value = 7;
    home.getUnhandledFriendApplicationCount();
    await _drain();
    expect(nativeCalls, 1);
    final current = im.session.begin('synthetic', 'im-two', 'chat-two', 2);
    im.session.activate(current);
    response.complete(jsonEncode([
      {
        'fromUserID': 'sender',
        'toUserID': 'synthetic',
        'createTime': 1,
        'handleResult': 0
      }
    ]));
    await _drain();
    expect(home.unhandledFriendApplicationCount.value, 7);
    expect(im.session.established(current), isTrue);
  });

  test(
      'production Home observes SDK rejection instead of leaking an async-void error',
      () async {
    final im = _IM();
    _register(im, _Push());
    final home = HomeLogic();
    const channel = MethodChannel('flutter_openim_sdk');
    final messenger =
        TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
    messenger.setMockMethodCallHandler(channel, (call) async {
      throw PlatformException(
          code: '10005', message: 'synthetic resource not loaded');
    });
    addTearDown(() {
      messenger.setMockMethodCallHandler(channel, null);
      home.onClose();
      im.initializedSubject.close();
    });
    final attempt = im.session.begin('synthetic', 'im-one', 'chat-one', 2);
    im.session.activate(attempt);
    home.getUnhandledFriendApplicationCount();
    home.getUnhandledGroupApplicationCount();
    await _drain();
    await _drain();
    expect(home.unhandledCount.value, 0);
  });
}
