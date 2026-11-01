import 'dart:async';
import 'dart:io';

import 'package:flutter/material.dart';
import 'package:flutter_background/flutter_background.dart';
import 'package:app_im/app_im.dart';
import 'package:livekit_client/livekit_client.dart';
import 'package:openim_common/openim_common.dart';
import 'package:rxdart/rxdart.dart';
import 'package:synchronized/synchronized.dart';

import 'appbar.dart';
import 'meeting_close_sheet.dart';
import 'meeting_detail_sheet.dart';
import 'meeting_members.dart';
import 'meeting_settings_sheet.dart';
import 'overlay_widget.dart';
import 'participant_info.dart';
import 'toolsbar.dart';

class ControlsView extends StatefulWidget {
  const ControlsView(this.room, this.participant,
      {Key? key,
      required this.child,
      required this.meetingInfoChangedSubject,
      required this.participantsSubject,
      this.onClose,
      this.onMinimize,
      this.onInviteMembers,
      this.startTimerCompleter,
      this.enableFullScreen = false})
      : super(key: key);
  final Room room;
  final LocalParticipant participant;
  final Widget child;
  final BehaviorSubject<MeetingInfo> meetingInfoChangedSubject;
  final BehaviorSubject<List<ParticipantTrack>> participantsSubject;
  final Function()? onClose;
  final Function()? onMinimize;
  final Function()? onInviteMembers;
  final Completer<bool>? startTimerCompleter;
  final bool enableFullScreen;

  @override
  State<ControlsView> createState() => _ControlsViewState();
}

class _ControlsViewState extends State<ControlsView> {
  LocalParticipant get _participant => widget.participant;
  MeetingInfo? _meetingInfo;
  Timer? _callingTimer;
  int _duration = 0;
  bool _openSpeakerphone = true;

  // Use this object to prevent concurrent access to data
  final _lockVideo = Lock();
  final _lockAudio = Lock();
  final _lockCamera = Lock();
  final _lockScreenShare = Lock();

  late StreamSubscription _meetingInfoChangedSub;

  bool get _isHost => _meetingInfo?.hostUserID == _participant.identity;

  bool get _disabledMicrophone =>
      !_participant.isMicrophoneEnabled() &&
      _meetingInfo?.participantCanUnmuteSelf == false &&
      !_isHost;

  bool get _disabledCamera =>
      !_participant.isCameraEnabled() &&
      _meetingInfo?.participantCanEnableVideo == false &&
      !_isHost;

  bool get _disabledScreenShare =>
      _meetingInfo?.onlyHostShareScreen == true && !_isHost;

  int get membersCount => widget.room.remoteParticipants.length + 1;

  @override
  void initState() {
    _participant.addListener(_onChange);
    _meetingInfoChangedSub =
        widget.meetingInfoChangedSubject.listen(_onChangedMeetingInfo);
    widget.startTimerCompleter?.future.then((value) => _startCallingTimer());
    _enableSpeakerphone(true);
    super.initState();
  }

  @override
  void dispose() {
    _stopBackgroundService();
    _participant.removeListener(_onChange);
    _meetingInfoChangedSub.cancel();
    _callingTimer?.cancel();
    super.dispose();
  }

  void _startCallingTimer() {
    _callingTimer ??= Timer.periodic(
      const Duration(seconds: 1),
      (timer) {
        if (!mounted) return;
        setState(() {
          ++_duration;
        });
      },
    );
  }

  void _onChangedMeetingInfo(meetingInfo) {
    if (!mounted) return;
    setState(() {
      _meetingInfo = meetingInfo;
    });
  }

  void _onChange() {
    // trigger refresh
    // setState(() {});
  }

  _toggleAudio() async {
    await _lockAudio.synchronized(() async {
      if (_participant.isMicrophoneEnabled()) {
        await _participant.setMicrophoneEnabled(false);
      } else {
        await _participant.setMicrophoneEnabled(true);
      }
    });
  }

  _toggleVideo({bool forceDisable = false}) async {
    await _lockVideo.synchronized(() async {
      if (forceDisable) {
        await _participant.setCameraEnabled(false);
      } else {
        if (_participant.isCameraEnabled()) {
          await _participant.setCameraEnabled(false);
        } else {
          await _participant.setCameraEnabled(true);
        }
      }
    });
  }

  _toggleScreenShare() async {
    try {
      await _lockScreenShare.synchronized(() async {
        if (_participant.isScreenShareEnabled()) {
          await _disableScreenShare();
        } else {
          await _enableScreenShare();
        }
      });
    } catch (_) {
      if (!_participant.isScreenShareEnabled()) await _stopBackgroundService();
      IMViews.showToast('屏幕共享未启动，请检查权限后重试');
    }
  }

  Future<void> _stopBackgroundService() async {
    try {
      if (FlutterBackground.isBackgroundExecutionEnabled)
        await FlutterBackground.disableBackgroundExecution();
    } catch (error) {
      Logger.print('Screen sharing service cleanup: ${error.runtimeType}');
    }
  }

  _enableScreenShare() async {
    if (lkPlatformIs(PlatformType.android)) {
      // Android 14+ requires fresh capture consent before starting the projection FGS.
      if (!await Hardware.instance.requestCapturePermission()) return;
      final androidConfig = FlutterBackgroundAndroidConfig(
        notificationTitle: StrRes.screenShare,
        notificationText: StrRes.screenShareHint,
        notificationImportance: AndroidNotificationImportance.normal,
        notificationIcon:
            const AndroidResource(name: 'ic_launcher', defType: 'mipmap'),
      );
      if (!await FlutterBackground.initialize(androidConfig: androidConfig))
        throw StateError('Screen capture service permission denied');
      if (!FlutterBackground.isBackgroundExecutionEnabled &&
          !await FlutterBackground.enableBackgroundExecution())
        throw StateError('Screen capture service did not start');
    }
    await _toggleVideo(forceDisable: true);
    if (lkPlatformIs(PlatformType.iOS)) {
      var track = await LocalVideoTrack.createScreenShareTrack(
        const ScreenShareCaptureOptions(
            useiOSBroadcastExtension: true,
            maxFrameRate: 15.0,
            params: VideoParametersPresets.screenShareH720FPS15),
      );
      await _participant.publishVideoTrack(track);

      return;
    }
    await _participant.setScreenShareEnabled(true, captureScreenAudio: true);
  }

  _disableScreenShare() async {
    await _participant.setScreenShareEnabled(false);
    if (Platform.isAndroid) {
      await _stopBackgroundService();
    }
  }

  void _enableSpeakerphone(bool enabled) {
    // LocalTrack? track;
    // track = widget.room.localParticipant?.videoTrackPublications.firstOrNull?.track;
    // track?.mediaStreamTrack.enableSpeakerphone(enabled);
    Hardware.instance.setSpeakerphoneOn(enabled);
  }

  _onTapSpeaker() async {
    _openSpeakerphone = !_openSpeakerphone;
    _enableSpeakerphone(_openSpeakerphone);
    setState(() {});
  }

  @override
  Widget build(BuildContext context) {
    return SafeArea(
      child: Column(
        children: [
          Visibility(
              visible: !widget.enableFullScreen,
              child: MeetingAppBar(
                title: _meetingInfo?.meetingName,
                time: IMUtils.seconds2HMS(_duration),
                openSpeakerphone: _openSpeakerphone,
                onMinimize: widget.onMinimize,
                onEndMeeting: _openCloseMeetingSheet,
                onTapSpeakerphone: _onTapSpeaker,
                onViewMeetingDetail: _viewMeetingDetail,
              )),
          Expanded(child: widget.child),
          Visibility(
              visible: !widget.enableFullScreen,
              child: ToolsBar(
                openedCamera: _participant.isCameraEnabled(),
                openedMicrophone: _participant.isMicrophoneEnabled(),
                openedScreenShare: _participant.isScreenShareEnabled(),
                enabledMicrophone: !_disabledMicrophone,
                enabledCamera: !_disabledCamera,
                enabledScreenShare: !_disabledScreenShare,
                onTapCamera: _toggleVideo,
                onTapMicrophone: _toggleAudio,
                onTapScreenShare: _toggleScreenShare,
                onTapSettings: _openSettingsSheet,
                onTapMemberList: _openMembersSheet,
                membersCount: membersCount,
                isHost: _isHost,
              )),
        ],
      ),
    );
  }

  _openMembersSheet() {
    OverlayWidget().showBottomSheet(
      context: context,
      child: (AnimationController? controller) => MeetingMembersSheetView(
        controller: controller,
        participantsSubject: widget.participantsSubject,
        meetingInfoChangedSubject: widget.meetingInfoChangedSubject,
        onInvite: widget.onInviteMembers,
        isHost: _isHost,
      ),
    );
  }

  _openSettingsSheet() {
    if (null == _meetingInfo) return;
    OverlayWidget().showBottomSheet(
      context: context,
      child: (AnimationController? controller) => MeetingSettingsSheetView(
        controller: controller,
        allowParticipantUnmute: _meetingInfo!.participantCanUnmuteSelf!,
        allowParticipantVideo: _meetingInfo!.participantCanEnableVideo!,
        onlyHostCanInvite: _meetingInfo!.onlyHostInviteUser!,
        onlyHostCanShareScreen: _meetingInfo!.onlyHostShareScreen!,
        joinMeetingDefaultMute: _meetingInfo!.joinDisableMicrophone!,
        onConfirm: _confirmSettings,
      ),
    );
  }

  _openCloseMeetingSheet() {
    OverlayWidget().showBottomSheet(
      context: context,
      child: (AnimationController? controller) => MeetingCloseSheetView(
        controller: controller,
        isHost: _isHost,
        onDismiss: () async {
          await LoadingView.singleton.wrap(
              asyncFunction: () =>
                  AppIm.iMManager.signalingManager.signalingCloseRoom(
                    roomID: _meetingInfo!.roomID!,
                  ));
          widget.onClose?.call();
        },
        onLeave: () {
          widget.onClose?.call();
        },
      ),
    );
  }

  _confirmSettings(Map map) {
    LoadingView.singleton.wrap(asyncFunction: () async {
      await AppIm.iMManager.signalingManager.signalingUpdateMeetingInfo(
        info: map..putIfAbsent('roomID', () => _meetingInfo!.roomID!),
      );
      _meetingInfo!.participantCanUnmuteSelf = map['participantCanUnmuteSelf'];
      _meetingInfo!.participantCanEnableVideo =
          map['participantCanEnableVideo'];
      _meetingInfo!.onlyHostInviteUser = map['onlyHostInviteUser'];
      _meetingInfo!.onlyHostShareScreen = map['onlyHostShareScreen'];
      _meetingInfo!.joinDisableMicrophone = map['joinDisableMicrophone'];
    });
  }

  _viewMeetingDetail() {
    OverlayWidget().showBottomSheet(
      context: context,
      child: (AnimationController? controller) => MeetingDetailSheetView(
        info: _meetingInfo!,
        hostNickname: '',
      ),
    );
  }
}
