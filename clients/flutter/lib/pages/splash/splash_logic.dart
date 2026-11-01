import 'dart:async';

import 'package:get/get.dart';
import 'package:openim_common/openim_common.dart';

import '../../core/controller/im_controller.dart';
import '../../core/controller/push_controller.dart';
import '../../routes/app_navigator.dart';

class SplashLogic extends GetxController {
  final imLogic = Get.find<IMController>();
  final pushLogic = Get.find<PushController>();

  String? get userID => DataSp.userID;

  String? get token => DataSp.imToken;

  late StreamSubscription initializedSub;

  @override
  void onInit() {
    initializedSub = imLogic.initializedSubject.listen((value) {
      Logger.print('---------------------initialized---------------------');
      if (value && null != userID && null != token) {
        _login();
      } else {
        AppNavigator.startLogin();
      }
    });
    super.onInit();
  }

  _login() async {
    String? attemptedToken;
    try {
      await imLogic.prepareLogin();
      if (isClosed) return;
      final uid = userID;
      attemptedToken = token;
      if (uid == null || attemptedToken == null) {
        AppNavigator.startLogin();
        return;
      }
      Logger.print('---------login---------- userID: $userID');
      final attempt = await imLogic.login(uid, attemptedToken);
      if (isClosed || !imLogic.session.established(attempt)) return;
      Logger.print('---------im login success-------');
      pushLogic.login(uid);
      Logger.print('---------push login success----');
      AppNavigator.startSplashToMain(isAutoLogin: true);
    } catch (_) {
      if (isClosed) return;
      IMViews.showToast('登录未完成，请检查网络或重新登录');
      // The controller owns captured-pair cleanup. A late splash failure must
      // never erase credentials saved by a newer login attempt.
      if (isClosed || imLogic.session.active) return;
      AppNavigator.startLogin();
    }
  }

  @override
  void onClose() {
    initializedSub.cancel();
    super.onClose();
  }
}
