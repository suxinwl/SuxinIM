import 'dart:io';
import 'dart:async';

import 'package:flutter/material.dart';
import 'package:app_im/app_im.dart';
import 'package:get/get.dart';
import 'package:openim_common/openim_common.dart';
import 'package:openim_live/openim_live.dart';
import 'package:permission_handler/permission_handler.dart';

import '../im_callback.dart';
import '../session_epoch.dart';
import '../session_renewal.dart';
import '../vault_reauthorization.dart';
import '../../routes/app_navigator.dart';
import 'push_controller.dart';

class IMController extends GetxController with IMCallback, OpenIMLive {
  final userInfo = UserFullInfo().obs;
  late String atAllTag;
  final session = SessionEpoch();
  Timer? _ssoTimer;
  StreamSubscription<void>? _vaultLock,_backgroundVaultLock;
  bool _needsBackgroundSDKReset=false;
  bool vaultReauthorizationPending=false;
  bool _hardInvalidationPending=false;
  void prepareAutomaticVaultRecovery(){
    if(!_hardInvalidationPending&&!session.active&&DataSp.getLoginCertificate()!=null){
      vaultReauthorizationPending=true;
      if(_sdkReady.isCompleted)_needsBackgroundSDKReset=true;
    }
  }
  Future<void>? _resumingVault;
  void requireVaultReauthorization(){
    if(_hardInvalidationPending||DataSp.getLoginCertificate()==null)return;
    _needsBackgroundSDKReset=true;
    if(vaultReauthorizationPending)return;
    vaultReauthorizationPending=true;session.invalidate();AppIm.enterprise.invalidateCallTerminations();
    AppNavigator.startVaultReauthorization();
    if(WidgetsBinding.instance.lifecycleState==AppLifecycleState.resumed)unawaited(resumeAfterVaultBackground());
  }
  Future<void> resumeAfterVaultBackground()=>_resumingVault??=
      _resumeVault().whenComplete(()=>_resumingVault=null);
  Future<void> _resumeVault()async{
    if(WidgetsBinding.instance.lifecycleState!=AppLifecycleState.resumed)return;
    if(Platform.isAndroid&&session.active&&!vaultReauthorizationPending){
      await NativeLocalVault.ensureCurrentAuthorization();
      await Future<void>.delayed(Duration.zero);
    }
    if(!vaultReauthorizationPending||_hardInvalidationPending)return;
    if(!_needsBackgroundSDKReset){initializedSubject.add(true);return;}
    _needsBackgroundSDKReset=false;
    final generation=NativeLocalVault.generation;
    try{await _serialSdk(()async{
      if(generation!=NativeLocalVault.generation||_hardInvalidationPending)return;
      await AppIm.iMManager.unInitSDKAndWait();await initOpenIM();
    });}
    catch(_){_needsBackgroundSDKReset=true;initializedSubject.add(false);}
  }
  bool _renewingSSO=false;
  Future<void> reconfigureServer() => _serialSdk(() async {
    await AppIm.iMManager.unInitSDKAndWait();
    await initOpenIM();
  });
  Future<void> _renewSSO() async {
    if(_renewingSSO || !session.active || NativeLocalVault.restoring) return;
    _renewingSSO=true;
    try {
      await renewCurrentSSOSession(
        session: session,
        readCertificate: DataSp.getLoginCertificate,
        refresh: () => HrmSSO.refresh(),
        unlock: NativeLocalVault.unlock,
        login: (current) async { await login(current.userID, current.imToken); },
        lock: NativeLocalVault.lock,
        kick: kickedOffline,
        restoring: () => NativeLocalVault.restoring,
        suspend: NativeLocalVault.suspend,
      );
    } finally { _renewingSSO=false; }
  }
  Completer<bool> _sdkReady = Completer<bool>();
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

  Future<void> prepareLogin() async {
    AppIm.enterprise.invalidateCallTerminations();
    await _invalidationCleanup;
    _hardInvalidationPending=false;
  }

  @override
  void kickedOffline() {
    _hardInvalidationPending=true;
    vaultReauthorizationPending=false;
    unawaited(NativeLocalVault.lock());
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
    _vaultLock?.cancel();_backgroundVaultLock?.cancel();
    _ssoTimer?.cancel();
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
    NativeLocalVault.quiesceSDKForRestore=()=>_serialSdk(()async{if(await AppIm.iMManager.getLoginStatus()!=1)await AppIm.iMManager.logout();});
    _vaultLock=NativeLocalVault.locked.listen((_){if(session.active||vaultReauthorizationPending)kickedOffline();});
    _backgroundVaultLock=NativeLocalVault.backgroundLocked.listen((_)=>requireVaultReauthorization());
    onInitLive();
    _ssoTimer=Timer.periodic(const Duration(minutes:1),(_)=>_renewSSO());
    // Initialize SDK
    WidgetsBinding.instance.addPostFrameCallback((_) async {
      try {
        await initOpenIM();
      } catch (error) {
        if(vaultReauthorizationPending)_needsBackgroundSDKReset=true;
        if (!_sdkReady.isCompleted) _sdkReady.complete(false);
        initializedSubject.add(false);
        Logger.print('SDK initialization failed: ${error.runtimeType}');
      }
    });
  }

  Future<void> initOpenIM() async {
    if(_sdkReady.isCompleted)_sdkReady=Completer<bool>();
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
    if(initialized!=true){if(vaultReauthorizationPending)_needsBackgroundSDKReset=true;throw StateError('SDK initialization failed; retry initialization');}
  }

  Future<SessionAttempt> login(String userID, String token) =>
      _serialSdk(() async {
        final recovering=vaultReauthorizationPending;
        if(NativeLocalVault.restoring)throw StateError('本机档案正在恢复，请完成后重新登录');
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
        final certificate = DataSp.getLoginCertificate();
        if(certificate == null || certificate.userID != userID) throw StateError('Missing local vault identity');
        await NativeLocalVault.unlock(certificate);
        final attempt =
            session.begin(userID, token, chatToken, IMUtils.getPlatform());
        try {
          var user = await AppIm.iMManager.login(
            userID: userID,
            token: token,
          );
          await session.waitForConnection(attempt);
          await NativeLocalVault.finalizeLegacy();
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
          vaultReauthorizationPending=false;
          _needsBackgroundSDKReset=false;
          explicitLogoutPending = false;
          _queryMyFullInfo();
          _queryAtAllTag();
          AppIm.enterprise.pullEvents();
          return attempt;
        } catch (e, s) {
          final preserve=recovering&&!isDefiniteSessionDenial(e);
          await NativeLocalVault.lock(reauthorize:preserve);
          Logger.print('IM login failed: ${e.runtimeType}');
          await _handleLoginRepeatError(attempt,preserveCredentials:preserve);
          // rethrow;
          return Future.error(e, s);
        }
      });

  Future<void> logout({bool revokeSession = true}) => _serialSdk(() async {
        _hardInvalidationPending=true;
        vaultReauthorizationPending=false;
        AppIm.enterprise.invalidateCallTerminations();
        if (revokeSession) {
          explicitLogoutPending = true;
          await HrmSSO.logout();
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
        await NativeLocalVault.lock();
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

  Future<void> _handleLoginRepeatError(SessionAttempt attempt,{bool preserveCredentials=false}) async {
    // SDK login can set its local flag before fetching the initial self profile.
    // A failed first fetch (including TLS failure) must not become a successful UI login.
    if (!session.current(attempt)) return;
    final epoch = session.invalidate();
    try {
      await AppIm.iMManager.logout();
    } catch (_) {}
    AppIm.iMManager.isLogined = false;
    AppIm.iMManager.token = null;
    if (!preserveCredentials && session.epoch == epoch &&
        DataSp.imToken == attempt.imToken &&
        DataSp.chatToken == attempt.chatToken) {
      await DataSp.removeLoginCertificate();
    }
  }
}
