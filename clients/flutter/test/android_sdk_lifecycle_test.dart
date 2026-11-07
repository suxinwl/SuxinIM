import 'dart:async';

import 'package:app_im/app_im.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:get/get.dart';
import 'package:openim/core/controller/app_controller.dart';
import 'package:openim/core/controller/im_controller.dart';
import 'package:openim_common/openim_common.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'native_settings_mock.dart';

class _App extends Fake implements AppController {
  @override
  final onStart = InternalFinalCallback<void>(callback: () {});
  @override
  final onDelete = InternalFinalCallback<void>(callback: () {});
}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  const native = MethodChannel('flutter_openim_sdk');
  final messenger =
      TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;

  setUp(() async {
    Get.testMode = true;
    Get.put<AppController>(_App(), permanent: true);
    installNativeSettingsMock();
    SharedPreferences.setMockInitialValues({});
    await DataSp.init();
    Config.cachePath = '/synthetic-private-sdk-path/';
    AppIm.enterprise.configure(
        baseUrl: () => 'https://example.invalid/enterprise/v1',
        imToken: () => null,
        chatToken: () => null,
        platform: () => 2,
        ready: () => false,
        userID: () => null);
  });

  tearDown(() {
    messenger.setMockMethodCallHandler(native, null);
    AppIm.enterprise.dispose();
    Get.reset();
  });

  test('pinned Dart teardown waits for the native acknowledgement', () async {
    const channel = MethodChannel('synthetic/sdk-lifecycle');
    final manager = IMManager(channel);
    final acknowledgement = Completer<bool>();
    final methods = <String>[];
    messenger.setMockMethodCallHandler(channel, (call) async {
      methods.add(call.method);
      return acknowledgement.future;
    });
    addTearDown(() => messenger.setMockMethodCallHandler(channel, null));
    var completed = false;
    final teardown = manager.unInitSDKAndWait().then((_) => completed = true);
    await Future<void>.delayed(Duration.zero);
    expect(methods, ['unInitSDK']);
    expect(completed, isFalse);
    acknowledgement.complete(true);
    await teardown;
    expect(completed, isTrue);
  });

  test('native teardown error propagates through the real Dart SDK', () async {
    const channel = MethodChannel('synthetic/sdk-lifecycle');
    final manager = IMManager(channel);
    messenger.setMockMethodCallHandler(channel, (_) async {
      throw PlatformException(
          code: 'SDK_DEINITIALIZATION_FAILED', message: 'IllegalStateException');
    });
    addTearDown(() => messenger.setMockMethodCallHandler(channel, null));
    await expectLater(
        manager.unInitSDKAndWait(),
        throwsA(isA<PlatformException>().having(
            (e) => e.code, 'fixed code', 'SDK_DEINITIALIZATION_FAILED')));
  });

  test('actual controller serial queue admits login after server reset ACK',
      () async {
    final controller = IMController();
    final acknowledgement = Completer<bool>();
    final calls = <String>[];
    messenger.setMockMethodCallHandler(native, (call) async {
      calls.add(call.method);
      if (call.method == 'unInitSDK') return acknowledgement.future;
      if (call.method == 'initSDK') return true;
      if (call.method == 'getLoginStatus') {
        throw PlatformException(code: 'SYNTHETIC_STOP_BEFORE_AUTHORIZATION');
      }
      return null; // Listener wiring only; no account or server is contacted.
    });
    await DataSp.putLoginCertificate(LoginCertificate.fromJson({
      'userID': 'synthetic-account',
      'imToken': 'synthetic-im',
      'chatToken': 'synthetic-chat',
      'sessionToken': 'synthetic-session',
      'expiresAt': DateTime.now().millisecondsSinceEpoch ~/ 1000 + 60,
    }));
    final reset = controller.reconfigureServer();
    final login = controller.login('synthetic-account', 'synthetic-im');
    final rejected = expectLater(
        login,
        throwsA(isA<PlatformException>().having(
            (e) => e.code,
            'login reached its first native call',
            'SYNTHETIC_STOP_BEFORE_AUTHORIZATION')));
    await Future<void>.delayed(Duration.zero);
    expect(calls, ['unInitSDK']);
    acknowledgement.complete(true);
    await reset;
    await rejected;
    expect(calls.indexOf('unInitSDK'), lessThan(calls.indexOf('initSDK')));
    expect(calls.indexOf('initSDK'), lessThan(calls.indexOf('getLoginStatus')));
    expect(controller.session.active, isFalse);
    expect(NativeLocalVault.unlocked, isFalse);
  });

  test('actual controller queue recovers after a rejected teardown', () async {
    final controller = IMController();
    var attempts = 0;
    final calls = <String>[];
    messenger.setMockMethodCallHandler(native, (call) async {
      calls.add(call.method);
      if (call.method == 'unInitSDK' && ++attempts == 1) {
        throw PlatformException(
            code: 'SDK_DEINITIALIZATION_FAILED', message: 'IllegalStateException');
      }
      if (call.method == 'unInitSDK' || call.method == 'initSDK') return true;
      return null;
    });
    await expectLater(controller.reconfigureServer(),
        throwsA(isA<PlatformException>()));
    expect(calls, ['unInitSDK']);
    await controller.reconfigureServer();
    expect(calls.where((method) => method == 'unInitSDK').length, 2);
    expect(calls.where((method) => method == 'initSDK').length, 1);
    expect(controller.session.active, isFalse);
    expect(NativeLocalVault.unlocked, isFalse);
  });
}
