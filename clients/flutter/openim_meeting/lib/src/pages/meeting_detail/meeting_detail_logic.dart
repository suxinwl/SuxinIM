import 'dart:async';
import 'package:flutter/widgets.dart';
import 'package:common_utils/common_utils.dart';
import 'package:app_im/app_im.dart';
import 'package:get/get.dart';
import 'package:openim_common/openim_common.dart';

import '../../../openim_meeting.dart';

enum MeetingDetailPhase { loading, scheduled, active, ended, failed }

class MeetingDetailLogic extends GetxController with WidgetsBindingObserver {
  MeetingDetailLogic(
      {EnterpriseManager? enterprise,
      Future<bool> Function(String roomID, CallTerminationSession session,
              bool Function() current)?
          joinMeeting})
      : _enterprise = enterprise ?? AppIm.enterprise,
        _joinMeeting = joinMeeting ??
            ((roomID, session, current) async {
              return MeetingClient().join(Get.context!,
                  meetingID: roomID,
                  expectedSession: session,
                  isCurrent: current);
            });

  final EnterpriseManager _enterprise;
  final Future<bool> Function(String roomID, CallTerminationSession session,
      bool Function() current) _joinMeeting;
  final phase = MeetingDetailPhase.loading.obs;
  final loading = false.obs;
  final entering = false.obs;
  final errorType = RxnString();
  CallTerminationSession? _session;
  StreamSubscription<String?>? _hintSubscription;
  Future<void>? _pending;
  bool _dirty = false;
  bool _closed = false;
  bool _confirmedEnded = false;
  int _generation = 0;

  bool get _sessionCurrent =>
      _session != null && _enterprise.isMeetingStateSessionCurrent(_session!);
  bool get _joinable =>
      !_closed &&
      _sessionCurrent &&
      !loading.value &&
      errorType.value == null &&
      !_confirmedEnded &&
      const {MeetingDetailPhase.active, MeetingDetailPhase.scheduled}
          .contains(phase.value);
  bool get canEnter => !entering.value && _joinable;
  String get statusText => phase.value == MeetingDetailPhase.ended
      ? StrRes.meetingEnded
      : phase.value == MeetingDetailPhase.loading
          ? StrRes.meetingStateLoading
          : phase.value == MeetingDetailPhase.failed
              ? StrRes.meetingStateUnavailable
              : phase.value == MeetingDetailPhase.active
                  ? StrRes.started
                  : StrRes.didNotStart;

  late MeetingInfo meetingInfo;
  late String meetingCreator;
  late int startTime;
  late int endTime;

  SelectContactsBridge? get bridge => PackageBridge.selectContactsBridge;

  @override
  void onInit() {
    meetingInfo = Get.arguments['meetingInfo'];
    meetingCreator = Get.arguments['meetingCreator'];
    startTime = meetingInfo.startTime! * 1000;
    endTime = meetingInfo.endTime! * 1000;
    WidgetsBinding.instance.addObserver(this);
    try {
      _session = _enterprise.captureMeetingStateSession();
    } catch (error) {
      phase.value = MeetingDetailPhase.failed;
      errorType.value = error.runtimeType.toString();
    }
    _hintSubscription = _enterprise.meetingStateHints.stream.listen((roomID) {
      if (_closed) return;
      if (!_sessionCurrent) {
        _invalidateSession();
        return;
      }
      if (roomID == null || roomID == meetingNo)
        unawaited(refreshState(fromHint: true));
    });
    super.onInit();
  }

  String get meetingStartTime =>
      DateUtil.formatDateMs(startTime, format: 'HH:mm');

  String get meetingEndTime => DateUtil.formatDateMs(endTime, format: 'HH:mm');

  String get meetingStartDate =>
      DateUtil.formatDateMs(startTime, format: IMUtils.getTimeFormat1());

  String get meetingEndDate =>
      DateUtil.formatDateMs(endTime, format: IMUtils.getTimeFormat1());

  String get meetingNo => meetingInfo.roomID ?? '';

  String get meetingDuration {
    final elapsed = meetingInfo.endTime! - meetingInfo.startTime!;
    final offset = elapsed < 0 ? 0 : elapsed;
    return '${offset ~/ 60}${StrRes.minute}';
  }

  @override
  void onReady() {
    super.onReady();
    unawaited(refreshState());
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    if (state == AppLifecycleState.resumed)
      unawaited(refreshState(fromHint: true));
  }

  void _invalidateSession() {
    if (_closed) return;
    _generation++;
    _dirty = false;
    loading.value = false;
    entering.value = false;
    errorType.value = 'SessionChanged';
    phase.value = MeetingDetailPhase.failed;
  }

  Future<void> _loadOne(int generation) async {
    bool current() => !_closed && generation == _generation;
    if (!current()) return;
    if (!_sessionCurrent) {
      _invalidateSession();
      return;
    }
    loading.value = true;
    errorType.value = null;
    if (!_confirmedEnded) phase.value = MeetingDetailPhase.loading;
    try {
      final snapshot =
          await _enterprise.queryMeetingRoomState(meetingNo, _session!);
      if (!current()) return;
      if (!_sessionCurrent) {
        _invalidateSession();
        return;
      }
      if (_confirmedEnded && !snapshot.ended) {
        errorType.value = 'NonMonotonicMeetingState';
        phase.value = MeetingDetailPhase.ended;
        return;
      }
      meetingInfo = snapshot.info;
      startTime = meetingInfo.startTime! * 1000;
      endTime = meetingInfo.endTime! * 1000;
      _confirmedEnded = _confirmedEnded || snapshot.ended;
      phase.value = _confirmedEnded
          ? MeetingDetailPhase.ended
          : snapshot.state == 'active'
              ? MeetingDetailPhase.active
              : MeetingDetailPhase.scheduled;
    } catch (error) {
      if (!current()) return;
      if (!_sessionCurrent) {
        _invalidateSession();
        return;
      }
      errorType.value = error.runtimeType.toString();
      phase.value = _confirmedEnded
          ? MeetingDetailPhase.ended
          : MeetingDetailPhase.failed;
    } finally {
      if (current()) loading.value = false;
    }
  }

  Future<void> refreshState({bool fromHint = false}) {
    if (_closed) return Future.value();
    if (!_sessionCurrent) {
      _invalidateSession();
      return Future.value();
    }
    final existing = _pending;
    if (existing != null) {
      if (fromHint) _dirty = true;
      return existing;
    }
    final done = Completer<void>();
    _pending = done.future; // Publish before observable state changes/reentry.
    final generation = _generation;
    late void Function() run;
    void finishOrRerun() {
      if (!_closed && generation == _generation && _sessionCurrent && _dirty) {
        run();
        return;
      }
      _pending = null;
      done.complete();
    }

    run = () {
      _dirty = false;
      Future<void>.sync(() => _loadOne(generation)).then<void>(
          (_) => finishOrRerun(), onError: (Object error, StackTrace stack) {
        if (!_closed && generation == _generation) {
          errorType.value = error.runtimeType.toString();
          phase.value = MeetingDetailPhase.failed;
          loading.value = false;
        }
        finishOrRerun();
      });
    };
    run();
    return done.future;
  }

  void retry() => unawaited(refreshState());

  @override
  void onClose() {
    _closed = true;
    _generation++;
    _dirty = false;
    WidgetsBinding.instance.removeObserver(this);
    final subscription = _hintSubscription;
    _hintSubscription = null;
    if (subscription != null)
      unawaited(subscription.cancel().catchError((Object error) {
        Logger.print(
            'Meeting state subscription cleanup failed: ${error.runtimeType}');
      }));
    super.onClose();
  }

  void copy() {
    IMUtils.copy(text: meetingInfo.roomID!);
  }

  Future<void> enterMeeting() async {
    if (_closed || entering.value || _confirmedEnded || !_sessionCurrent)
      return;
    entering.value = true;
    final generation = _generation;
    try {
      await refreshState();
      if (_closed || generation != _generation || !_joinable) return;
      bool current() =>
          !_closed && generation == _generation && _sessionCurrent;
      final joined = await _joinMeeting(meetingNo, _session!, current);
      if (current() && !joined) {
        errorType.value = 'MeetingJoinFailed';
        phase.value = MeetingDetailPhase.failed;
      }
    } catch (error) {
      if (!_closed && generation == _generation && _sessionCurrent) {
        errorType.value = error.runtimeType.toString();
        phase.value = MeetingDetailPhase.failed;
      }
    } finally {
      if (!_closed && generation == _generation) entering.value = false;
    }
  }

  shareMeeting() async {
    final meetingID = meetingInfo.roomID!;
    final meetingName = meetingInfo.meetingName!;
    final startTime = meetingInfo.startTime!;
    final duration = meetingInfo.endTime! - meetingInfo.startTime!;
    Logger.print('metaData meetingID : $meetingID');
    Logger.print('metaData meetingName : $meetingName');
    Logger.print('metaData startTime : $startTime');
    Logger.print('metaData duration : $duration');
    final result = await bridge?.selectContacts(2);
    if (result is Map) {
      final list = IMUtils.convertCheckedListToShare(result.values);
      await LoadingView.singleton.wrap(
          asyncFunction: () => Future.forEach(
                list,
                (map) => MeetingClient().invite(
                  userID: map['userID'],
                  groupID: map['groupID'],
                  meetingID: meetingID,
                  meetingName: meetingName,
                  startTime: startTime,
                  duration: duration,
                ),
              ));
      IMViews.showToast('分享成功');
    }
  }

  editMeeting() {
    Get.bottomSheet(
      BottomSheetView(
        items: [
          SheetItem(label: StrRes.updateMeetingInfo, onTap: _modifyMeetingInfo),
          SheetItem(
            label: StrRes.cancelMeeting,
            textStyle: Styles.ts_FF381F_17sp,
            onTap: _cancelMeeting,
          ),
        ],
      ),
    );
  }

  _cancelMeeting() async {
    try {
      await LoadingView.singleton.wrap(
          asyncFunction: () =>
              AppIm.iMManager.signalingManager.signalingCloseRoom(
                roomID: meetingInfo.roomID!,
              ));
      Get.back();
    } catch (e, s) {
      Logger.print('error: $e,  stack: $s');
      if (e.toString().contains('NotExist')) {
        IMViews.showToast("会议已经结束！");
      } else {
        IMViews.showToast("网络异常请稍后再试！");
      }
    }
  }

  _modifyMeetingInfo() => MNavigator.startBookMeeting(
        meetingInfo: meetingInfo,
        offAndToNamed: true,
      );
}
