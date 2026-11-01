import 'dart:io';
import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:app_im/app_im.dart';
import 'package:get/get.dart';
import 'package:openim_common/openim_common.dart';
import 'package:openim_live/openim_live.dart';
import 'package:permission_handler/permission_handler.dart';

import '../im_callback.dart';
import '../session_epoch.dart';
import '../../routes/app_navigator.dart';
import 'push_controller.dart';

class IMController extends GetxController with IMCallback, OpenIMLive {
  final userInfo = UserFullInfo().obs;
  late String atAllTag;
  final session = SessionEpoch();
  final _sdkReady = Completer<bool>();
  Future<void> _sdkOperation = Future<void>.value();
  Future<void> _invalidationCleanup = Future<void>.value();

  Future<T> _serialSdk<T>(Future<T> Function() action) async {
    final previous = _sdkOperation;
    final completion = Completer<void>();
    _sdkOperation = completion.future;
    await previous;
    try {
      return await action();
    } finally {
      completion.complete();
    }
  }

  Future<void> prepareLogin() {
    AppIm.enterprise.invalidateCallTerminations();
    return _invalidationCleanup;
  }

  @override
  void kickedOffline() {
    AppIm.enterprise.invalidateCallTerminations();
    final token = session.attempt?.imToken ?? DataSp.imToken;
    final epoch = session.invalidate();
    if (AppIm.iMManager.token == token) {
      AppIm.iMManager.isLogined = false;
      AppIm.iMManager.token = null;
    }
    // A precise logout can kick the socket before its HTTP response. Keep the
    // captured pair available for an idempotent retry if that response is lost.
    if (explicitLogoutPending) return;
    onKickedOfflineSubject.add('');
    _invalidationCleanup = _clearInvalidSession(epoch, token);
  }

  Future<void> _clearInvalidSession(int epoch, String? token) async {
    try {
      PackageBridge.meetingBridge?.dismiss();
      PackageBridge.rtcBridge?.dismiss();
      if (session.epoch != epoch || DataSp.imToken != token) return;
      await DataSp.removeLoginCertificate();
      if (session.epoch != epoch) return;
      if (Get.isRegistered<PushController>())
        Get.find<PushController>().logout();
      AppNavigator.startLogin();
    } catch (error) {
      Logger.print('Invalid session cleanup failed: ${error.runtimeType}');
    }
  }

  @override
  void onClose() {
    AppIm.enterprise.invalidateCallTerminations();
    session.invalidate();
    super.close();
    // AppIm.iMManager.unInitSDK();
    onCloseLive();
    super.onClose();
  }

  @override
  void onInit() async {
    super.onInit();
    onInitLive();
    // Initialize SDK
    WidgetsBinding.instance.addPostFrameCallback((_) async {
      try {
        await initOpenIM();
      } catch (error) {
        if (!_sdkReady.isCompleted) _sdkReady.complete(false);
        initializedSubject.add(false);
        Logger.print('SDK initialization failed: ${error.runtimeType}');
      }
    });
  }

  Future<void> initOpenIM() async {
    final initialized = await AppIm.iMManager.initSDK(
      platformID: IMUtils.getPlatform(),
      apiAddr: Config.imApiUrl,
      wsAddr: Config.imWsUrl,
      dataDir: Config.cachePath,
      logLevel: 6,
      logFilePath: Config.cachePath,
      listener: OnConnectListener(
        onConnecting: () {
          imSdkStatus(IMSdkStatus.connecting);
        },
        onConnectFailed: (code, error) {
          imSdkStatus(IMSdkStatus.connectionFailed);
        },
        onConnectSuccess: () {
          session.connected();
          imSdkStatus(IMSdkStatus.connectionSucceeded);
        },
        onKickedOffline: kickedOffline,
        onUserTokenExpired: kickedOffline,
        onUserTokenInvalid: kickedOffline,
      ),
    );
    AppIm.enterprise.onGroupReadReceipt = recvGroupMessageReadReceipt;
    // Set listener
    AppIm.iMManager
      ..setUploadLogsListener(
          OnUploadLogsListener(onUploadProgress: uploadLogsProgress))
      //
      ..userManager.setUserListener(OnUserListener(
          onSelfInfoUpdated: (u) {
            userInfo.update((val) {
              val?.nickname = u.nickname;
              val?.faceURL = u.faceURL;
              // val?.gender = u.gender;
              // val?.birth = u.birth;
              // val?.email = u.email;
              val?.remark = u.remark;
              val?.ex = u.ex;
              val?.globalRecvMsgOpt = u.globalRecvMsgOpt;
            });
            // _queryMyFullInfo();
          },
          onUserStatusChanged: userStausChanged))
      // Add message listener (remove when not in use)
      ..messageManager.setAdvancedMsgListener(OnAdvancedMsgListener(
          onRecvC2CReadReceipt: recvC2CMessageReadReceipt,
          onRecvNewMessage: recvNewMessage,
          onRecvOnlineOnlyMessage: recvNewMessage,
          onNewRecvMessageRevoked: recvMessageRevoked,
          onRecvOfflineNewMessage: recvOfflineMessage))

      // Set up message sending progress listener
      ..messageManager.setMsgSendProgressListener(OnMsgSendProgressListener(
        onProgress: progressCallback,
      ))
      ..messageManager.setCustomBusinessListener(OnCustomBusinessListener(
        onRecvCustomBusinessMessage: recvCustomBusinessMessage,
      ))
      // Set up friend relationship listener
      ..friendshipManager.setFriendshipListener(OnFriendshipListener(
        onBlackAdded: blacklistAdded,
        onBlackDeleted: blacklistDeleted,
        onFriendApplicationAccepted: friendApplicationAccepted,
        onFriendApplicationAdded: friendApplicationAdded,
        onFriendApplicationDeleted: friendApplicationDeleted,
        onFriendApplicationRejected: friendApplicationRejected,
        onFriendInfoChanged: friendInfoChanged,
        onFriendAdded: friendAdded,
        onFriendDeleted: friendDeleted,
      ))

      // Set up conversation listener
      ..conversationManager.setConversationListener(OnConversationListener(
        onInputStatusChanged: inputStatusChanged,
        onConversationChanged: conversationChanged,
        onNewConversation: newConversation,
        onTotalUnreadMessageCountChanged: totalUnreadMsgCountChanged,
        onSyncServerFailed: (_) {
          imSdkStatus(IMSdkStatus.syncFailed);
        },
        onSyncServerFinish: (_) {
          imSdkStatus(IMSdkStatus.syncEnded);
          if (Platform.isAndroid) {
            Permissions.request([Permission.systemAlertWindow]);
          }
        },
        onSyncServerStart: (_) {
          imSdkStatus(IMSdkStatus.syncStart);
        },
      ))

      // Set up group listener
      ..groupManager.setGroupListener(OnGroupListener(
        onGroupApplicationAccepted: groupApplicationAccepted,
        onGroupApplicationAdded: groupApplicationAdded,
        onGroupApplicationDeleted: groupApplicationDeleted,
        onGroupApplicationRejected: groupApplicationRejected,
        onGroupInfoChanged: groupInfoChanged,
        onGroupMemberAdded: groupMemberAdded,
        onGroupMemberDeleted: groupMemberDeleted,
        onGroupMemberInfoChanged: groupMemberInfoChanged,
        onJoinedGroupAdded: joinedGroupAdded,
        onJoinedGroupDeleted: joinedGroupDeleted,
      ))
      // Set up signaling listener
      ..signalingManager.setSignalingListener(OnSignalingListener(
        onInvitationCancelled: invitationCancelled,
        onInvitationTimeout: invitationTimeout,
        onInviteeAccepted: inviteeAccepted,
        onInviteeRejected: inviteeRejected,
        onReceiveNewInvitation: receiveNewInvitation,
        onInviteeAcceptedByOtherDevice: inviteeAcceptedByOtherDevice,
        onInviteeRejectedByOtherDevice: inviteeRejectedByOtherDevice,
        onHangup: beHangup,
        onRoomParticipantConnected: roomParticipantConnected,
        onRoomParticipantDisconnected: roomParticipantDisconnected,
        onMeetingStreamChanged: meetingSteamChanged,
      ));

    if (!_sdkReady.isCompleted) _sdkReady.complete(initialized == true);
    initializedSubject.sink.add(initialized == true);
  }

  Future<SessionAttempt> login(String userID, String token) =>
      _serialSdk(() async {
        await prepareLogin();
        if (!await _sdkReady.future.timeout(const Duration(seconds: 30))) {
          throw StateError('SDK initialization failed; retry login');
        }
        final chatToken = DataSp.chatToken;
        if (chatToken == null || chatToken.isEmpty || DataSp.imToken != token) {
          throw StateError('Missing current login credentials');
        }
        session.invalidate();
        // Native callbacks have no attempt ID. Quiesce the old SDK session before
        // beginning another attempt; server verification remains authoritative.
        if (await AppIm.iMManager.getLoginStatus() != 1) {
          await AppIm.iMManager.logout();
        }
        final attempt =
            session.begin(userID, token, chatToken, IMUtils.getPlatform());
        try {
          var user = await AppIm.iMManager.login(
            userID: userID,
            token: token,
          );
          await session.waitForConnection(attempt);
          await AppIm.enterprise.verifySession(
              imToken: attempt.imToken,
              chatToken: attempt.chatToken,
              platformID: attempt.platformID);
          session.requireCurrent(attempt);
          if (DataSp.imToken != attempt.imToken ||
              DataSp.chatToken != attempt.chatToken) {
            throw StateError('Login credentials changed; retry login');
          }
          userInfo.value = UserFullInfo.fromJson(user.toJson());
          session.activate(attempt);
          explicitLogoutPending = false;
          _queryMyFullInfo();
          _queryAtAllTag();
          AppIm.enterprise.pullEvents();
          return attempt;
        } catch (e, s) {
          Logger.print('IM login failed: ${e.runtimeType}');
          await _handleLoginRepeatError(attempt);
          // rethrow;
          return Future.error(e, s);
        }
      });

  Future<void> logout({bool revokeSession = true}) => _serialSdk(() async {
        AppIm.enterprise.invalidateCallTerminations();
        if (revokeSession) {
          explicitLogoutPending = true;
          final result = await AppIm.enterprise.request('/session/logout');
          if (result['revoked'] != true) throw StateError('退出未确认，请重试');
        }
        session.invalidate();
        try {
          await AppIm.iMManager.logout();
        } catch (error) {
          // A confirmed remote revocation is final, even if local kicked/closed callbacks race it.
          if (!revokeSession && await AppIm.iMManager.getLoginStatus() != 1)
            rethrow;
          AppIm.iMManager.isLogined = false;
          AppIm.iMManager.token = null;
          if (revokeSession) {
            Logger.print(
                'Remote session revoked; resetting local SDK after ${error.runtimeType}');
            AppIm.iMManager.unInitSDK();
            try {
              await initOpenIM();
            } catch (_) {
              Logger.print('Local SDK reset deferred until app restart');
            }
          }
        }
        PackageBridge.meetingBridge?.dismiss();
        PackageBridge.rtcBridge?.dismiss();
      });

  /// @所有人ID
  void _queryAtAllTag() async {
    atAllTag = AppIm.iMManager.conversationManager.atAllTag;
    // atAllTag = await AppIm.iMManager.conversationManager.getAtAllTag();
  }

  void _queryMyFullInfo() async {
    final attempt = session.attempt;
    if (attempt == null || !session.established(attempt)) return;
    try {
      final data = await Apis.queryMyFullInfo();
      if (!session.established(attempt)) return;
      if (data is UserFullInfo) {
        userInfo.update((val) {
          val?.allowAddFriend = data.allowAddFriend;
          val?.allowBeep = data.allowBeep;
          val?.allowVibration = data.allowVibration;
          val?.nickname = data.nickname;
          val?.faceURL = data.faceURL;
          val?.phoneNumber = data.phoneNumber;
          val?.email = data.email;
          val?.birth = data.birth;
          val?.gender = data.gender;
        });
      }
    } catch (error) {
      Logger.print('Profile refresh failed: ${error.runtimeType}');
    }
  }

  Future<void> _handleLoginRepeatError(SessionAttempt attempt) async {
    // SDK login can set its local flag before fetching the initial self profile.
    // A failed first fetch (including TLS failure) must not become a successful UI login.
    if (!session.current(attempt)) return;
    final epoch = session.invalidate();
    try {
      await AppIm.iMManager.logout();
    } catch (_) {}
    AppIm.iMManager.isLogined = false;
    AppIm.iMManager.token = null;
    if (session.epoch == epoch &&
        DataSp.imToken == attempt.imToken &&
        DataSp.chatToken == attempt.chatToken) {
      await DataSp.removeLoginCertificate();
    }
  }
}
