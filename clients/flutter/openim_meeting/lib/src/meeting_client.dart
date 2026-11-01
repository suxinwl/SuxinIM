import 'dart:async';

import 'package:flutter/material.dart';
import 'package:app_im/app_im.dart';
import 'package:livekit_client/livekit_client.dart';
import 'package:openim_common/openim_common.dart';
import 'package:openim_meeting/src/pages/meeting_room/room.dart';
import 'package:rxdart/rxdart.dart';
import 'package:wakelock_plus/wakelock_plus.dart';

typedef MeetingRoomBuilder = Widget Function(Room room,
    EventsListener<RoomEvent> listener, String roomID, VoidCallback onClose);

class _MeetingAttempt {
  _MeetingAttempt(
      this.generation, this.session, this.callerCurrent, this.roomID);
  final int generation;
  final CallTerminationSession session;
  final bool Function()? callerCurrent;
  String? roomID;
  final subject = PublishSubject<MeetingStreamEvent>();
  Room? room;
  EventsListener<RoomEvent>? listener;
  OverlayEntry? entry;
  bool inserted = false;
  bool closed = false;
  bool wantsWakelock = false;
  Future<void>? cleanup;
  Future<void>? closing;
}

class _MeetingAttemptCancelled implements Exception {}

/// Owns each connection attempt separately from the singleton's current view.
class MeetingClient implements MeetingBridge {
  MeetingClient._()
      : _enterprise = AppIm.enterprise,
        _roomFactory = Room.new,
        _wakelock = ((enabled) => WakelockPlus.toggle(enable: enabled)),
        _meetingBuilder = _defaultMeetingBuilder;

  /// Explicit dependencies keep this same connection/ownership path usable by
  /// embedders and its transport/widget boundary regressions.
  MeetingClient.withDependencies({
    required EnterpriseManager enterprise,
    required Room Function() roomFactory,
    Future<void> Function(bool enabled)? wakelock,
    MeetingRoomBuilder? meetingBuilder,
  })  : _enterprise = enterprise,
        _roomFactory = roomFactory,
        _wakelock =
            wakelock ?? ((enabled) => WakelockPlus.toggle(enable: enabled)),
        _meetingBuilder = meetingBuilder ?? _defaultMeetingBuilder;

  final EnterpriseManager _enterprise;
  final Room Function() _roomFactory;
  final Future<void> Function(bool enabled) _wakelock;
  final MeetingRoomBuilder _meetingBuilder;
  static final MeetingClient _singleton = MeetingClient._();

  factory MeetingClient.singleton() {
    PackageBridge.meetingBridge ??= _singleton;
    return _singleton;
  }
  factory MeetingClient() => MeetingClient.singleton();

  static Widget _defaultMeetingBuilder(
          Room room,
          EventsListener<RoomEvent> listener,
          String roomID,
          VoidCallback onClose) =>
      MeetingRoom(room, listener, roomID: roomID, onClose: onClose);

  _MeetingAttempt? _current;
  int _generation = 0;
  Future<void> _wakelockTail = Future.value();
  bool get isBusy => _current != null;
  PublishSubject<MeetingStreamEvent>? get subject => _current?.subject;

  @override
  bool get hasConnection => isBusy;

  @override
  void dismiss() => _observeClose(close());

  void _observeClose(Future<void> operation) {
    unawaited(operation.catchError((Object error) {
      Logger.print('Meeting close failed: ${error.runtimeType}');
    }));
  }

  bool _callerCurrent(_MeetingAttempt attempt) {
    try {
      return attempt.callerCurrent?.call() ?? true;
    } catch (error) {
      Logger.print('Meeting caller guard failed: ${error.runtimeType}');
      return false;
    }
  }

  bool _isCurrent(_MeetingAttempt attempt) =>
      identical(_current, attempt) &&
      !attempt.closed &&
      _generation == attempt.generation &&
      _enterprise.isMeetingStateSessionCurrent(attempt.session) &&
      _callerCurrent(attempt);

  void _ensureCurrent(_MeetingAttempt attempt) {
    if (!_isCurrent(attempt)) {
      throw _MeetingAttemptCancelled();
    }
  }

  Future<void> _syncWakelock() {
    // Serialize the global side effect, but evaluate the current owner's intent
    // when this task actually runs. An old pending enable cannot disable or
    // overwrite a later meeting's final wakelock state.
    final next = _wakelockTail.then<void>((_) async {
      try {
        final owner = _current;
        final enabled =
            owner != null && _isCurrent(owner) && owner.wantsWakelock;
        await _wakelock(enabled);
      } catch (error) {
        Logger.print('Meeting wakelock failed: ${error.runtimeType}');
      }
    });
    _wakelockTail = next;
    return next;
  }

  Future<void> _disposePending(_MeetingAttempt attempt) {
    final existing = attempt.cleanup;
    if (existing != null) return existing;
    final done = Completer<void>();
    attempt.cleanup = done.future;
    Future<void>.sync(() async {
      Object? first;
      StackTrace? firstStack;
      Future<void> step(FutureOr<void> Function() operation) async {
        try {
          await operation();
        } catch (error, stack) {
          first ??= error;
          firstStack ??= stack;
        }
      }

      final listener = attempt.listener;
      final room = attempt.room;
      if (listener != null) await step(listener.dispose);
      if (room != null) {
        await step(room.disconnect);
        await step(room.dispose);
      }
      if (first != null) Error.throwWithStackTrace(first!, firstStack!);
    }).then<void>((_) => done.complete(),
        onError: (Object error, StackTrace stack) =>
            done.completeError(error, stack));
    return done.future;
  }

  Future<void> _closeAttempt(_MeetingAttempt attempt) {
    final existing = attempt.closing;
    if (existing != null) return existing;
    final done = Completer<void>();
    attempt.closing = done.future;
    attempt.closed = true;
    attempt.wantsWakelock = false;
    if (identical(_current, attempt)) {
      _current = null;
      _generation++;
    }
    Future<void>.sync(() async {
      final roomID = attempt.roomID;
      if (roomID != null) _enterprise.notifyMeetingRoomChange(roomID);
      final entry = attempt.entry;
      attempt.entry = null;
      if (entry != null) {
        if (attempt.inserted) entry.remove();
        entry.dispose();
      }
      await attempt.subject.close();
      // Once inserted, the existing MeetingRoom widget owns listener/Room physical
      // shutdown from dispose. Removing that UI is not a physical-release promise.
      try {
        if (!attempt.inserted) await _disposePending(attempt);
      } finally {
        await _syncWakelock();
      }
    }).then<void>((_) => done.complete(),
        onError: (Object error, StackTrace stack) =>
            done.completeError(error, stack));
    return done.future;
  }

  /// Removes the current UI and invalidates pending attempts. For an inserted
  /// MeetingRoom this Future does not represent its asynchronous media disposal.
  Future<void> close() {
    final owner = _current;
    return owner == null ? _syncWakelock() : _closeAttempt(owner);
  }

  Future<bool> create(
    BuildContext ctx, {
    required String meetingName,
    required int startTime,
    required int duration,
    CallTerminationSession? expectedSession,
    bool Function()? isCurrent,
  }) =>
      _connect(ctx,
          isCreate: true,
          meetingName: meetingName,
          startTime: startTime,
          duration: duration,
          expectedSession: expectedSession,
          callerCurrent: isCurrent);

  Future<bool> join(
    BuildContext ctx, {
    required String meetingID,
    String? meetingName,
    String? participantNickname,
    CallTerminationSession? expectedSession,
    bool Function()? isCurrent,
  }) =>
      _connect(ctx,
          isCreate: false,
          meetingID: meetingID,
          meetingName: meetingName,
          participantNickname: participantNickname,
          expectedSession: expectedSession,
          callerCurrent: isCurrent);

  Future<bool> _connect(
    BuildContext ctx, {
    required bool isCreate,
    String? meetingID,
    String? meetingName,
    int? startTime,
    int? duration,
    String? participantNickname,
    CallTerminationSession? expectedSession,
    bool Function()? callerCurrent,
  }) async {
    if (isBusy) return false;
    _MeetingAttempt? attempt;
    try {
      final session =
          expectedSession ?? _enterprise.captureMeetingStateSession();
      final owner =
          _MeetingAttempt(++_generation, session, callerCurrent, meetingID);
      attempt = owner;
      _current = owner;
      if (!ctx.mounted) throw _MeetingAttemptCancelled();
      _ensureCurrent(owner);
      FocusScope.of(ctx).unfocus();
      await LoadingView.singleton.wrap(asyncFunction: () async {
        // LoadingView has its own asynchronous delay. Recheck before the HTTP
        // method materializes any authentication, including default callers.
        if (!ctx.mounted) throw _MeetingAttemptCancelled();
        _ensureCurrent(owner);
        final sc = isCreate
            ? await _enterprise.signalingCreateMeeting(
                meetingName: meetingName!,
                startTime: startTime!,
                meetingDuration: duration!,
                expectedSession: owner.session)
            : await _enterprise.signalingJoinMeeting(
                roomID: meetingID!,
                meetingName: meetingName,
                participantNickname: participantNickname,
                expectedSession: owner.session);
        if (!ctx.mounted) throw _MeetingAttemptCancelled();
        _ensureCurrent(owner);
        if (sc.roomID == null ||
            sc.roomID!.isEmpty ||
            (!isCreate && sc.roomID != meetingID) ||
            sc.liveURL == null ||
            sc.token == null) {
          throw const FormatException(
              'Meeting connection certificate is invalid');
        }
        owner.roomID = sc.roomID;
        final room = _roomFactory();
        owner.room = room;
        final listener = room.createListener();
        owner.listener = listener;
        await room.connect(sc.liveURL!, sc.token!,
            roomOptions: const RoomOptions(
              dynacast: true,
              adaptiveStream: true,
              defaultCameraCaptureOptions:
                  CameraCaptureOptions(params: VideoParametersPresets.h720_169),
              defaultVideoPublishOptions: VideoPublishOptions(
                  simulcast: true,
                  videoCodec: 'VP8',
                  videoEncoding: VideoEncoding(
                      maxBitrate: 5 * 1000 * 1000, maxFramerate: 15)),
              defaultScreenShareCaptureOptions: ScreenShareCaptureOptions(
                  useiOSBroadcastExtension: true, maxFrameRate: 15.0),
            ));
        if (!ctx.mounted) throw _MeetingAttemptCancelled();
        _ensureCurrent(owner);
        owner.wantsWakelock = true;
        await _syncWakelock();
        if (!ctx.mounted) throw _MeetingAttemptCancelled();
        _ensureCurrent(owner);
        final entry = OverlayEntry(
            builder: (context) => _meetingBuilder(room, listener, sc.roomID!,
                () => _observeClose(_closeAttempt(owner))));
        owner.entry = entry;
        if (!ctx.mounted) throw _MeetingAttemptCancelled();
        _ensureCurrent(owner);
        Overlay.of(ctx).insert(entry);
        owner.inserted = true;
      });
      if (!ctx.mounted) throw _MeetingAttemptCancelled();
      _ensureCurrent(owner);
      return true;
    } catch (error) {
      final owner = attempt;
      final notify = owner != null &&
          _isCurrent(owner) &&
          error is! _MeetingAttemptCancelled;
      if (owner != null) {
        try {
          await _closeAttempt(owner);
        } catch (cleanupError) {
          Logger.print(
              'Meeting owned cleanup failed: ${cleanupError.runtimeType}');
        }
      }
      Logger.print('Meeting connection stopped: ${error.runtimeType}');
      if (notify &&
          _current == null &&
          _generation == owner.generation + 1 &&
          _enterprise.isMeetingStateSessionCurrent(owner.session) &&
          _callerCurrent(owner) &&
          ctx.mounted) {
        try {
          await IMViews.showToast(error.toString().contains('NotExist')
              ? StrRes.meetingIsOver
              : StrRes.networkError);
        } catch (toastError) {
          Logger.print(
              'Meeting error display failed: ${toastError.runtimeType}');
        }
      }
      return false;
    }
  }

  invite({
    required String meetingID,
    required String meetingName,
    required int startTime,
    required int duration,
    String? userID,
    String? groupID,
  }) async =>
      AppIm.iMManager.messageManager.sendMessage(
        userID: userID,
        groupID: groupID,
        message: await AppIm.iMManager.messageManager.createMeetingMessage(
          inviterUserID: AppIm.iMManager.userInfo.userID!,
          inviterNickname: AppIm.iMManager.userInfo.nickname ?? '',
          inviterFaceURL: AppIm.iMManager.userInfo.faceURL,
          subject: meetingName,
          id: meetingID,
          start: startTime,
          duration: duration,
        ),
        offlinePushInfo: Config.offlinePushInfo
          ..title = StrRes.offlineMeetingMessage,
      );
}
