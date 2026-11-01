import 'dart:io';
import 'dart:async';

import 'package:flutter/material.dart';
import 'package:app_im/app_im.dart';
import 'package:flutter_screen_lock/flutter_screen_lock.dart';
import 'package:get/get.dart';
import 'package:local_auth/local_auth.dart';
import 'package:openim_common/openim_common.dart';
import 'package:permission_handler/permission_handler.dart';
import 'package:rxdart/rxdart.dart';

import '../../core/controller/app_controller.dart';
import '../../core/controller/im_controller.dart';
import '../../core/controller/push_controller.dart';
import '../../core/im_callback.dart';
import '../../routes/app_navigator.dart';
import '../../widgets/screen_lock_title.dart';

class HomeLogic extends SuperController {
  final pushLogic = Get.find<PushController>();
  final imLogic = Get.find<IMController>();
  final cacheLogic = Get.find<CacheController>();
  final initLogic = Get.find<AppController>();
  final index = 0.obs;
  final unreadMsgCount = 0.obs;
  final unhandledFriendApplicationCount = 0.obs;
  final unhandledGroupApplicationCount = 0.obs;
  final unhandledCount = 0.obs;
  String? _lockScreenPwd;
  bool _isShowScreenLock = false;
  late bool _isAutoLogin;
  final auth = LocalAuthentication();
  final _errorController = PublishSubject<String>();
  final _subscriptions = <StreamSubscription>[];
  StreamSubscription? _resumeConnectionSubscription;

  Function()? onScrollToUnreadMessage;

  switchTab(index) {
    this.index.value = index;
  }

  scrollToUnreadMessage(index) {
    onScrollToUnreadMessage?.call();
  }

  /// 获取消息未读数
  Future<void> _getUnreadMsgCount() async {
    final attempt = imLogic.session.attempt;
    if (attempt == null || !imLogic.session.established(attempt) || isClosed)
      return;
    try {
      final count =
          await AppIm.iMManager.conversationManager.getTotalUnreadMsgCount();
      if (isClosed || !imLogic.session.established(attempt)) return;
      unreadMsgCount.value = int.tryParse(count) ?? 0;
      initLogic.showBadge(unreadMsgCount.value);
    } catch (error) {
      Logger.print('Unread count refresh failed: ${error.runtimeType}');
    }
  }

  /// 获取好友申请未处理数
  /// 浏览过得不再计入红点
  void getUnhandledFriendApplicationCount() async {
    final attempt = imLogic.session.attempt;
    if (attempt == null || !imLogic.session.established(attempt) || isClosed)
      return;
    try {
      var i = 0;
      var list = await AppIm.iMManager.friendshipManager
          .getFriendApplicationListAsRecipient();
      if (isClosed || !imLogic.session.established(attempt)) return;
      var haveReadList = DataSp.getHaveReadUnHandleFriendApplication();
      haveReadList ??= <String>[];
      for (var info in list) {
        var id = IMUtils.buildFriendApplicationID(info);
        if (!haveReadList.contains(id)) {
          if (info.handleResult == 0) i++;
        }
      }
      unhandledFriendApplicationCount.value = i;
      unhandledCount.value = unhandledGroupApplicationCount.value + i;
    } catch (error) {
      Logger.print('Friend request refresh failed: ${error.runtimeType}');
    }
  }

  /// 获取群申请未处理数
  void getUnhandledGroupApplicationCount() async {
    final attempt = imLogic.session.attempt;
    if (attempt == null || !imLogic.session.established(attempt) || isClosed)
      return;
    try {
      var i = 0;
      var list = await AppIm.iMManager.groupManager
          .getGroupApplicationListAsRecipient();
      if (isClosed || !imLogic.session.established(attempt)) return;
      var haveReadList = DataSp.getHaveReadUnHandleGroupApplication();
      haveReadList ??= <String>[];
      for (var info in list) {
        var id = IMUtils.buildGroupApplicationID(info);
        if (!haveReadList.contains(id)) {
          if (info.handleResult == 0) i++;
        }
      }
      unhandledGroupApplicationCount.value = i;
      unhandledCount.value = unhandledFriendApplicationCount.value + i;
    } catch (error) {
      Logger.print('Group request refresh failed: ${error.runtimeType}');
    }
  }

  @override
  void onInit() {
    _isAutoLogin = Get.arguments['isAutoLogin'];
    if (_isAutoLogin) {
      WidgetsBinding.instance.addPostFrameCallback((_) => _showLockScreenPwd());
    }
    _subscriptions.add(imLogic.unreadMsgCountEventSubject.listen((value) {
      unreadMsgCount.value = value;
    }));
    _subscriptions.add(imLogic.friendApplicationChangedSubject.listen((value) {
      getUnhandledFriendApplicationCount();
    }));
    _subscriptions.add(imLogic.groupApplicationChangedSubject.listen((value) {
      getUnhandledGroupApplicationCount();
    }));
    super.onInit();
  }

  @override
  void onReady() {
    if (!imLogic.session.active) return;
    _getUnreadMsgCount();
    getUnhandledFriendApplicationCount();
    getUnhandledGroupApplicationCount();
    cacheLogic.initCallRecords();
    cacheLogic.initFavoriteEmoji();
    super.onReady();
  }

  @override
  void onClose() {
    _resumeConnectionSubscription?.cancel();
    for (final subscription in _subscriptions) {
      subscription.cancel();
    }
    _errorController.close();
    super.onClose();
  }

  _localAuth() async {
    final didAuthenticate = await IMUtils.checkingBiometric(auth);
    if (didAuthenticate) {
      Get.back();
    }
  }

  _showLockScreenPwd() async {
    if (_isShowScreenLock) return;
    _lockScreenPwd = DataSp.getLockScreenPassword();
    if (null != _lockScreenPwd) {
      final isEnabledBiometric = DataSp.isEnabledBiometric() == true;
      bool enabled = false;
      if (isEnabledBiometric) {
        final isSupportedBiometrics = await auth.isDeviceSupported();
        final canCheckBiometrics = await auth.canCheckBiometrics;
        enabled = isSupportedBiometrics && canCheckBiometrics;
      }
      _isShowScreenLock = true;
      screenLock(
        context: Get.context!,
        correctString: _lockScreenPwd!,
        maxRetries: 3,
        title: ScreenLockTitle(stream: _errorController.stream),
        canCancel: false,
        customizedButtonChild: enabled ? const Icon(Icons.fingerprint) : null,
        customizedButtonTap: enabled ? () async => await _localAuth() : null,
        // onOpened: enabled ? () async => await _localAuth() : null,
        onUnlocked: () {
          _isShowScreenLock = false;
          Get.back();
        },
        onMaxRetries: (_) async {
          try {
            await LoadingView.singleton.wrap(asyncFunction: () async {
              await imLogic.logout();
              await DataSp.removeLoginCertificate();
              await DataSp.clearLockScreenPassword();
              await DataSp.closeBiometric();
              pushLogic.logout();
            });
            Get.back();
            AppNavigator.startLogin();
          } catch (_) {
            IMViews.showToast('退出失败，请检查网络后重试');
          }
        },
        onError: (retries) {
          _errorController.sink.add(
            retries.toString(),
          );
        },
      );
    }
  }

  @override
  void onDetached() {
    // TODO: implement onDetached
  }

  @override
  void onInactive() {
    // TODO: implement onInactive
  }

  @override
  void onPaused() {
    // TODO: implement onPaused
  }

  @override
  void onResumed() {
    _resumeConnectionSubscription?.cancel();
    _resumeConnectionSubscription = null;
    final attempt = imLogic.session.attempt;
    if (attempt == null || !imLogic.session.established(attempt) || isClosed)
      return;
    if (imLogic.imSdkStatusSubject.valueOrNull ==
        IMSdkStatus.connectionSucceeded) {
      _getRTCInvitationStart();
    } else {
      _resumeConnectionSubscription = imLogic.imSdkStatusSubject
          .where((value) => value == IMSdkStatus.connectionSucceeded)
          .take(1)
          .listen((value) {
        if (!isClosed && imLogic.session.established(attempt)) {
          _getRTCInvitationStart();
        }
      });
    }
  }

  void _getRTCInvitationStart() async {
    final attempt = imLogic.session.attempt;
    if (attempt == null || !imLogic.session.established(attempt) || isClosed)
      return;
    try {
      final signalingInfo = await AppIm.iMManager.signalingManager
          .getSignalingInvitationInfoStartApp();
      if (isClosed || !imLogic.session.established(attempt)) return;
      if (null != signalingInfo.invitation) {
        // 调用视频界面
        imLogic.receiveNewInvitation(signalingInfo);
      }
    } catch (error) {
      Logger.print('Active invitation refresh failed: ${error.runtimeType}');
    }
  }

  @override
  void onHidden() {
    // TODO: implement onHidden
  }
}
