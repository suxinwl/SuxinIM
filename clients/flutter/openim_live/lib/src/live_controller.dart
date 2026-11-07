import 'dart:async';
import 'dart:io';

import 'package:collection/collection.dart';
import 'package:flutter/services.dart';
import 'package:flutter_openim_live_alert/flutter_openim_live_alert.dart';
import 'package:app_im/app_im.dart';
import 'package:get/get.dart';
import 'package:just_audio/just_audio.dart';
import 'package:openim_common/openim_common.dart';
import 'package:permission_handler/permission_handler.dart';
import 'package:rxdart/rxdart.dart';
import 'package:sprintf/sprintf.dart';
import 'package:uuid/uuid.dart';

import '../openim_live.dart';

/// Safe to show in a toast. The original failure stays in memory for diagnosis;
/// it is never interpolated into user messages or logs.
class CallSignalingFailure implements Exception {
  CallSignalingFailure._(this.code, this.cause, this.originalStack);
  final String code;
  final Object cause;
  final StackTrace originalStack;
  @override
  String toString() => code;
}

/// 信令
mixin OpenIMLive {
  final signalingSubject = PublishSubject<CallEvent>();

  /// 被邀请者收到：邀请者取消音视频通话
  void invitationCancelled(SignalingInfo info) {
    signalingSubject.add(CallEvent(CallState.beCanceled, info));
  }

  /// 邀请者收到：被邀请者超时未接通
  void invitationTimeout(SignalingInfo info) {
    signalingSubject.add(CallEvent(CallState.timeout, info));
  }

  /// 邀请者收到：被邀请者同意音视频通话
  void inviteeAccepted(SignalingInfo info) {
    signalingSubject.add(CallEvent(CallState.beAccepted, info));
  }

  /// 邀请者收到：被邀请者拒绝音视频通话
  void inviteeRejected(SignalingInfo info) {
    signalingSubject.add(CallEvent(CallState.beRejected, info));
  }

  /// 被邀请者收到：音视频通话邀请
  void receiveNewInvitation(SignalingInfo info) {
    signalingSubject.add(CallEvent(CallState.beCalled, info));
  }

  /// 被邀请者（其他端）收到：比如被邀请者在手机拒接，在pc上会收到此回调
  void inviteeAcceptedByOtherDevice(SignalingInfo info) {
    signalingSubject.add(CallEvent(CallState.otherAccepted, info));
  }

  /// 被邀请者（其他端）收到：比如被邀请者在手机拒接，在pc上会收到此回调
  void inviteeRejectedByOtherDevice(SignalingInfo info) {
    signalingSubject.add(CallEvent(CallState.otherReject, info));
  }

  /// 被挂断
  void beHangup(SignalingInfo info) {
    signalingSubject.add(CallEvent(CallState.beHangup, info));
  }

  final backgroundSubject = PublishSubject<bool>();
  final insertSignalingMessageSubject = PublishSubject<CallEvent>();

  Function(SignalingMessageEvent)? onSignalingMessage;
  final roomParticipantDisconnectedSubject = PublishSubject<RoomCallingInfo>();
  final roomParticipantConnectedSubject = PublishSubject<RoomCallingInfo>();

  bool _isRunningBackground = false;

  /// 退到后台不会弹出拨号界面，切到前台后才会弹出界面。
  /// 如果存在值，表示收到了来电邀请，启动后需要恢复拨号界面
  CallEvent? _beCalledEvent;

  /// true:点击了系统桌面的接受按钮，恢复拨号界面后自动接听
  bool _autoPickup = false;

  final _ring = 'assets/audio/live_ring.wav';
  final _audioPlayer = AudioPlayer(
    // Handle audio_session events ourselves for the purpose of this demo.
    handleInterruptions: false,
    // androidApplyAudioAttributes: false,
    // handleAudioSessionActivation: false,
  );
  late final _ringtone = SafeRingtone(_audioPlayer,
      onError: (phase, error) =>
          Logger.print('Call ringtone $phase failed: ${error.runtimeType}'));

  bool get isBusy => OpenIMLiveClient().isBusy;

  onCloseLive() {
    signalingSubject.close();
    backgroundSubject.close();
    insertSignalingMessageSubject.close();
    roomParticipantDisconnectedSubject.close();
    roomParticipantConnectedSubject.close();
    _ringtone.dispose();
    FlutterOpenimLiveAlert.closeLiveAlert();
  }

  onInitLive() async {
    _signalingListener();
    _insertSignalingMessageListener();
    // 后台通话
    backgroundSubject.listen((background) {
      _isRunningBackground = background;
      // 从后台切换到前台，如果还在被call，则拉起通话页面
      if (!_isRunningBackground) {
        final generation = AppIm.enterprise.callTerminationGeneration;
        unawaited(
            AppIm.enterprise.retryPendingCallTerminations().then((results) {
          if (generation != AppIm.enterprise.callTerminationGeneration) return;
          for (final result in results) {
            _observeTerminationResult(result);
          }
        }, onError: (Object error, StackTrace stack) {
          Logger.print('Call termination retry failed: ${error.runtimeType}');
        }));
        // 恢复拨号界面
        if (_beCalledEvent != null) {
          signalingSubject.add(_beCalledEvent!);
        }
        // 关闭系统弹框
        FlutterOpenimLiveAlert.closeLiveAlert();
      }
    });

    // 桌面浮窗
    FlutterOpenimLiveAlert.buttonEvent(
      onAccept: () {
        // 自动接听
        _autoPickup = true;
      },
      onReject: () async {
        // 点击系统桌面浮窗的拒绝按钮
        await onTapReject(
            _beCalledEvent!.data..userID = AppIm.iMManager.userID);
        // 重置拨号状态
        _beCalledEvent = null;
      },
    );

    // 群通话状态监听
    roomParticipantDisconnectedSubject.listen((info) {
      if (null == info.participant || info.participant!.length == 1) {
        OpenIMLiveClient().closeByRoomID(info.invitation!.roomID!);
      }
    });
  }

  /// 拦截其他干扰信令
  Stream<CallEvent> get _stream => signalingSubject
      .stream /*.where((event) => LiveClient.dispatchSignaling(event))*/;

  _signalingListener() => _stream.listen(
        (event) async {
          _beCalledEvent = null;
          if (event.state == CallState.beCalled) {
            _playSound();
            final mediaType = event.data.invitation!.mediaType;
            final sessionType = event.data.invitation!.sessionType;
            final callType =
                mediaType == 'audio' ? CallType.audio : CallType.video;
            final callObj = sessionType == ConversationType.single
                ? CallObj.single
                : CallObj.group;

            if (Platform.isAndroid && _isRunningBackground) {
              // 记录拨号状态
              _beCalledEvent = event;
              if (await Permissions.checkSystemAlertWindow()) {
                // 如果当前处于后台，显示系统浮窗并拦截拨号界面
                var list = await AppIm.iMManager.userManager.getUsersInfo(
                  userIDList: [event.data.invitation!.inviterUserID!],
                );
                // 后台弹框
                try {
                  await FlutterOpenimLiveAlert.showLiveAlert(
                    title: sprintf(StrRes.inviteYouCall, [
                      list.firstOrNull?.nickname,
                      callType == CallType.audio
                          ? StrRes.callVoice
                          : StrRes.callVideo
                    ]),
                    rejectText: StrRes.rejectCall,
                    acceptText: StrRes.acceptCall,
                  );
                } catch (error) {
                  Logger.print(
                      'Background call alert unavailable: ${error.runtimeType}');
                }
                return;
              }
            }
            // 重置
            _beCalledEvent = null;
            OpenIMLiveClient().start(
              Get.overlayContext!,
              callEventSubject: signalingSubject,
              roomID: event.data.invitation!.roomID!,
              inviteeUserIDList: event.data.invitation!.inviteeUserIDList!,
              inviterUserID: event.data.invitation!.inviterUserID!,
              groupID: event.data.invitation!.groupID,
              callType: callType,
              callObj: callObj,
              initState: CallState.beCalled,
              onSyncUserInfo: onSyncUserInfo,
              onSyncGroupInfo: onSyncGroupInfo,
              onSyncGroupMemberInfo: onSyncGroupMemberInfo,
              autoPickup: _autoPickup,
              onTapPickup: () => onTapPickup(
                event.data..userID = AppIm.iMManager.userID,
              ),
              onTapReject: () => onTapReject(
                event.data..userID = AppIm.iMManager.userID,
              ),
              onTapHangup: (duration, isPositive) => onTapHangup(
                event.data..userID = AppIm.iMManager.userID,
                duration,
                isPositive,
              ),
              onError: onError,
              onRoomDisconnected: () => onRoomDisconnected(event.data),
            );
          } else if (event.state == CallState.beRejected) {
            // 被拒绝
            _stopSound();
            insertSignalingMessageSubject.add(event);
          } else if (event.state == CallState.beHangup) {
            // 被挂断
            _stopSound();
            // 通过挂断方法插入通话时长消息
            // insertSignalingMessageSubject.add(event);
          } else if (event.state == CallState.beCanceled) {
            // 超时被取消
            if (_isRunningBackground) {
              FlutterOpenimLiveAlert.closeLiveAlert();
            }
            _stopSound();
            insertSignalingMessageSubject.add(event);
          } else if (event.state == CallState.beAccepted) {
            // 被接听
            _stopSound();
          } else if (event.state == CallState.otherReject ||
              event.state == CallState.otherAccepted) {
            // 被其他设备接听
            _stopSound();
          } else if (event.state == CallState.timeout) {
            // 超时无响应
            _stopSound();
            insertSignalingMessageSubject.add(event);
            final sessionType = event.data.invitation!.sessionType;

            if (sessionType == 1) {
              onTimeoutCancelled(event.data);
            }
          }
        },
      );

  _insertSignalingMessageListener() {
    insertSignalingMessageSubject.listen((value) {
      _insertMessage(
        state: value.state,
        signalingInfo: value.data,
        duration: value.fields ?? 0,
      );
    });
  }

  call({
    required CallObj callObj,
    required CallType callType,
    CallState callState = CallState.call,
    String? roomID,
    String? inviterUserID,
    required List<String> inviteeUserIDList,
    String? groupID,
    SignalingCertificate? credentials,
  }) async {
    final mediaType = callType == CallType.audio ? 'audio' : 'video';
    final sessionType = callObj == CallObj.single ? 1 : 3;
    inviterUserID ??= AppIm.iMManager.userID;

    final signal = SignalingInfo(
      userID: inviterUserID,
      invitation: InvitationInfo(
        inviterUserID: inviterUserID,
        inviteeUserIDList: inviteeUserIDList,
        roomID: roomID ?? const Uuid().v4(),
        timeout: 30,
        mediaType: mediaType,
        sessionType: sessionType,
        platformID: IMUtils.getPlatform(),
        groupID: groupID,
      ),
    );

    OpenIMLiveClient().start(
      Get.overlayContext!,
      callEventSubject: signalingSubject,
      inviterUserID: inviterUserID,
      groupID: groupID,
      inviteeUserIDList: inviteeUserIDList,
      callObj: callObj,
      callType: callType,
      initState: callState,
      onDialSingle: () => onDialSingle(signal),
      onDialGroup: () => onDialGroup(signal),
      onJoinGroup: () => Future.value(credentials!),
      onTapCancel: () => onTapCancel(signal),
      onTapHangup: (duration, isPositive) => onTapHangup(
        signal,
        duration,
        isPositive,
      ),
      onSyncUserInfo: onSyncUserInfo,
      onSyncGroupInfo: onSyncGroupInfo,
      onSyncGroupMemberInfo: onSyncGroupMemberInfo,
      onWaitingAccept: () {
        if (callObj == CallObj.single) _playSound();
      },
      onBusyLine: onBusyLine,
      onStartCalling: () {
        _stopSound();
      },
      onError: onError,
      onRoomDisconnected: () => onRoomDisconnected(signal),
      onClose: _stopSound,
    );
  }

  void onError(error, stack) {
    final cause = error is CallSignalingFailure ? error.cause : error;
    final diagnostic = error is CallSignalingFailure ? error.code : 'CALL_CONNECTION_FAILED';
    Logger.print('Call failed: $diagnostic');
    OpenIMLiveClient().close();
    _stopSound();
    if (cause is PlatformException) {
      if (int.tryParse(cause.code) == SDKErrorCode.hasBeenBlocked) {
        IMViews.showToast(StrRes.callFail);
        return;
      }
    }
    IMViews.showToast(StrRes.networkError);
  }

  onRoomDisconnected(SignalingInfo signalingInfo) {
    insertSignalingMessageSubject.add(
      CallEvent(CallState.networkError, signalingInfo),
    );
  }

  Future<SignalingCertificate> _signalingCertificate(
    String code,
    Future<SignalingCertificate> Function() request,
  ) async {
    try {
      return await request();
    } catch (error, stack) {
      final failure = CallSignalingFailure._(code, error, stack);
      try {
        onError(failure, stack);
      } catch (_) {
        Logger.print('Call failure observer failed');
      }
      Error.throwWithStackTrace(failure, stack);
    }
  }

  /// 拨向单人
  Future<SignalingCertificate> onDialSingle(SignalingInfo signaling) =>
      _signalingCertificate('CALL_SIGNAL_INVITE_FAILED', () =>
          AppIm.iMManager.signalingManager.signalingInvite(
            info: signaling
              ..invitation?.timeout = 30
              ..offlinePushInfo =
                  (Config.offlinePushInfo..title = StrRes.offlineCallMessage),
          ));

  /// 拨向多人
  Future<SignalingCertificate> onDialGroup(SignalingInfo signaling) =>
      _signalingCertificate('CALL_SIGNAL_GROUP_INVITE_FAILED', () =>
          AppIm.iMManager.signalingManager.signalingInviteInGroup(
            info: signaling
              ..invitation?.timeout = 30
              ..offlinePushInfo =
                  (Config.offlinePushInfo..title = StrRes.offlineCallMessage),
          ));

  /// 接听
  Future<SignalingCertificate> onTapPickup(SignalingInfo signaling) {
    _beCalledEvent = null;
    _autoPickup = false;
    _stopSound();
    return _signalingCertificate('CALL_SIGNAL_ACCEPT_FAILED', () =>
        AppIm.iMManager.signalingManager.signalingAccept(info: signaling));
  }

  /// 拒绝
  onTapReject(SignalingInfo signaling) async {
    _stopSound();
    insertSignalingMessageSubject.add(CallEvent(CallState.reject, signaling));
    return AppIm.iMManager.signalingManager.signalingReject(info: signaling);
  }

  /// 取消
  onTapCancel(SignalingInfo signaling) async {
    _stopSound();
    insertSignalingMessageSubject.add(CallEvent(CallState.cancel, signaling));
    _terminateCall('cancel', signaling);
    return true;
  }

  /// 超时取消
  onTimeoutCancelled(SignalingInfo signaling) {
    _terminateCall('cancel', signaling);
  }

  /// 挂断
  /// [isPositive] 人为挂断行为
  onTapHangup(SignalingInfo signaling, int duration, bool isPositive) async {
    if (isPositive) {
      _terminateCall('hungUp', signaling);
    }
    _stopSound();
    insertSignalingMessageSubject.add(CallEvent(
      CallState.hangup,
      signaling,
      fields: duration,
    ));
  }

  void _terminateCall(String method, SignalingInfo signaling) {
    // The coordinator observes network completion independently. Local room
    // teardown must start immediately instead of waiting for a 30-second POST.
    final generation = AppIm.enterprise.callTerminationGeneration;
    unawaited(AppIm.enterprise
        .terminateCall(method: method, info: signaling)
        .then((result) {
      if (generation == AppIm.enterprise.callTerminationGeneration) {
        _observeTerminationResult(result);
      }
    }, onError: (Object error, StackTrace stack) {
      Logger.print('Call termination task failed: ${error.runtimeType}');
      if (generation != AppIm.enterprise.callTerminationGeneration) return;
      IMViews.showToast('本地通话已结束，服务端确认失败，请恢复网络后重试');
    }));
  }

  void _observeTerminationResult(CallTerminationResult result) {
    if (result.status == CallTerminationStatus.failed) {
      Logger.print('Call termination not confirmed: ${result.errorType}');
      IMViews.showToast('本地通话已结束，服务端尚未确认；恢复前台后将重试');
    }
  }

  /// 用户繁忙
  onBusyLine() {
    _stopSound();
    IMViews.showToast('用户正忙，请稍后再试！');
  }

  onJoin() {}

  /// 同步用户信息
  Future<UserInfo?> onSyncUserInfo(userID) async {
    var list = await AppIm.iMManager.userManager.getUsersInfo(
      userIDList: [userID],
    );

    return list.firstOrNull?.simpleUserInfo;
  }

  /// 同步组信息
  Future<GroupInfo?> onSyncGroupInfo(groupID) async {
    var list = await AppIm.iMManager.groupManager.getGroupsInfo(
      groupIDList: [groupID],
    );
    return list.firstOrNull;
  }

  /// 同步群成员信息
  Future<List<GroupMembersInfo>> onSyncGroupMemberInfo(
      groupID, userIDList) async {
    var list = await AppIm.iMManager.groupManager.getGroupMembersInfo(
      groupID: groupID,
      userIDList: userIDList,
    );
    return list;
  }

  /// 自定义通话消息
  void _insertMessage({
    required CallState state,
    required SignalingInfo signalingInfo,
    int duration = 0,
  }) async {
    (() async {
      var invitation = signalingInfo.invitation;
      var mediaType = invitation!.mediaType;
      var inviterUserID = invitation.inviterUserID;
      var inviteeUserID = invitation.inviteeUserIDList!.first;
      var groupID = invitation.groupID;
      Logger.print('----------state:${state.name}');
      Logger.print('----------mediaType:$mediaType');
      Logger.print('----------inviterUserID:$inviterUserID');
      Logger.print('----------inviteeUserIDList:$inviteeUserID');
      Logger.print('----------groupID:$groupID');
      Logger.print('----------duration:$duration');
      _recordCall(state: state, signaling: signalingInfo, duration: duration);
      var message = await AppIm.iMManager.messageManager.createCallMessage(
        state: state.name,
        type: mediaType!,
        duration: duration,
      );
      switch (invitation.sessionType) {
        case 1:
          {
            String? receiverID;
            if (inviterUserID != AppIm.iMManager.userID) {
              receiverID = inviterUserID;
            } else {
              receiverID = inviteeUserID;
            }

            var msg = await AppIm.iMManager.messageManager
                .insertSingleMessageToLocalStorage(
              receiverID: inviteeUserID,
              senderID: inviterUserID,
              // receiverID: receiverID,
              // senderID: AppIm.iMManager.uid,
              message: message
                ..status = 2
                ..isRead = true,
            );

            onSignalingMessage
                ?.call(SignalingMessageEvent(msg, 1, receiverID, null));
            // signalingMessageSubject.add(
            //   SignalingMessageEvent(msg, 1, receiverID, null),
            // );
          }
          break;
        case 2:
          {
            // signalingMessageSubject.add(
            //   SignalingMessageEvent(message, 2, null, groupID),
            // );
            // AppIm.iMManager.messageManager.insertGroupMessageToLocalStorage(
            //   groupID: groupID!,
            //   senderID: inviterUserID,
            //   message: message..status = 2,
            // );
          }
          break;
      }
    })();
  }

  /// 播放提示音
  Future<void> _playSound() =>
      _ringtone.start(_ring, package: 'openim_common', loopMode: LoopMode.one);

  /// 关闭提示音
  Future<void> _stopSound() => _ringtone.stop();

  void _recordCall({
    required CallState state,
    required SignalingInfo signaling,
    int duration = 0,
  }) async {
    var invitation = signaling.invitation;
    if (invitation!.sessionType != ConversationType.single) return;
    var mediaType = invitation.mediaType;
    var inviterUserID = invitation.inviterUserID;
    var inviteeUserID = invitation.inviteeUserIDList!.first;
    var groupID = invitation.groupID;
    var isMeCall = inviterUserID == AppIm.iMManager.userID;
    var userID = isMeCall ? inviteeUserID : inviterUserID!;
    var incomingCall = isMeCall ? false : true;
    var userInfo =
        (await AppIm.iMManager.userManager.getUsersInfo(userIDList: [userID]))
            .firstOrNull;
    if (null == userInfo) return;
    final cache = Get.find<CacheController>();
    cache.addCallRecords(CallRecords(
      userID: userID,
      nickname: userInfo.nickname ?? userID,
      faceURL: userInfo.faceURL,
      success: state == CallState.hangup || state == CallState.beHangup,
      date: DateTime.now().millisecondsSinceEpoch,
      type: mediaType!,
      incomingCall: incomingCall,
      duration: duration,
    ));
  }
}

class SignalingMessageEvent {
  Message message;
  String? userID;
  String? groupID;
  int sessionType;

  SignalingMessageEvent(
    this.message,
    this.sessionType,
    this.userID,
    this.groupID,
  );

  /// 单聊消息
  bool get isSingleChat => sessionType == ConversationType.single;

  /// 群聊消息
  bool get isGroupChat =>
      sessionType == ConversationType.group ||
      sessionType == ConversationType.superGroup;
}
