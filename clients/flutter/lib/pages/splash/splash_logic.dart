import 'dart:async';

import 'package:get/get.dart';
import 'package:flutter/widgets.dart';
import 'package:openim_common/openim_common.dart';

import '../../core/controller/im_controller.dart';
import '../../core/controller/push_controller.dart';
import '../../routes/app_navigator.dart';
import '../../core/vault_reauthorization.dart';

class SplashLogic extends GetxController {
  final imLogic = Get.find<IMController>();
  final pushLogic = Get.find<PushController>();

  String? get userID => DataSp.userID;

  String? get token => DataSp.imToken;

  late StreamSubscription initializedSub;
  final recovering = false.obs;
  final busy = false.obs;
  final recoveryError = ''.obs;

  @override
  void onInit() {
    imLogic.prepareAutomaticVaultRecovery();
    recovering.value = imLogic.vaultReauthorizationPending;
    initializedSub = imLogic.initializedSubject.listen((value) {
      recovering.value = imLogic.vaultReauthorizationPending;
      Logger.print('---------------------initialized---------------------');
      if (value && null != userID && null != token) {
        _login();
      } else if (recovering.value) {
        recoveryError.value = '本地档案已锁定，请联网后重试恢复会话';
      } else {
        AppNavigator.startLogin();
      }
    });
    super.onInit();
  }

  Future<void> retry() => _login();

  @override
  void onReady() {
    super.onReady();
    // PublishSubject does not replay an SDK-ready event emitted before this
    // route mounted. Recovery must also have an explicit single-flight entry.
    if (imLogic.vaultReauthorizationPending) unawaited(retry());
  }

  Future<void> chooseAnotherAccount() async {
    if (busy.value) return;
    imLogic.kickedOffline();
    await imLogic.prepareLogin();
    if (!isClosed) AppNavigator.startLogin();
  }

  Future<void> _login() async {
    if (busy.value || isClosed) return;
    if(imLogic.vaultReauthorizationPending &&
        WidgetsBinding.instance.lifecycleState!=AppLifecycleState.resumed)return;
    busy.value = true;
    recoveryError.value = '';
    final wasRecovering = imLogic.vaultReauthorizationPending;
    final owner = DataSp.getLoginCertificate()?.sessionToken;
    String? attemptedToken;
    try {
      await imLogic.prepareLogin();
      if (isClosed) return;
      if (wasRecovering) await imLogic.resumeAfterVaultBackground();
      if (isClosed) return;
      await HrmSSO.refresh(force: true);
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
    } catch (error) {
      if (isClosed) return;
      if (DataSp.getLoginCertificate()?.sessionToken != owner ||
          imLogic.session.active) return;
      if (wasRecovering && !isDefiniteSessionDenial(error)) {
        recovering.value = true;
        recoveryError.value = '会话恢复未完成，本地档案仍已锁定。请检查网络后重试';
        return;
      }
      if (wasRecovering && isDefiniteSessionDenial(error))
        imLogic.kickedOffline();
      IMViews.showToast('登录未完成，请检查网络或重新登录');
      // The controller owns captured-pair cleanup. A late splash failure must
      // never erase credentials saved by a newer login attempt.
      if (isClosed || imLogic.session.active) return;
      AppNavigator.startLogin();
    } finally {
      if (!isClosed) busy.value = false;
    }
  }

  @override
  void onClose() {
    initializedSub.cancel();
    super.onClose();
  }
}
