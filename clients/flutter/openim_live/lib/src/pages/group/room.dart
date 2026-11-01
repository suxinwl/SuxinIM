import 'dart:async';
import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:livekit_client/livekit_client.dart';
import 'package:openim_common/openim_common.dart';

import '../../live_client.dart';
import '../../utils/call_capture.dart';
import 'widgets/call_state.dart';
import 'widgets/participant_info.dart';

class GroupRoomView extends SignalView {
  const GroupRoomView({
    super.key,
    required super.callType,
    required super.initState,
    required super.callEventSubject,
    required super.autoPickup,
    super.roomID,
    required super.groupID,
    required super.inviterUserID,
    required super.inviteeUserIDList,
    super.onClose,
    super.onBindRoomID,
    super.onDial,
    super.onJoin,
    super.onTapCancel,
    super.onTapHangup,
    super.onTapPickup,
    super.onTapReject,
    super.onError,
    super.onSyncGroupMemberInfo,
    super.onSyncGroupInfo,
    super.onRoomDisconnected,
  });

  @override
  State<GroupRoomView> createState() => _GroupRoomViewState();
}

class _GroupRoomViewState extends SignalState<GroupRoomView> {
  EventsListener<RoomEvent>? _listener;
  Room? _room;
  int _captureGeneration = 0;

  @override
  void dispose() {
    ++_captureGeneration;
    // always dispose listener
    (() async {
      try {
        _room?.removeListener(_onRoomDidUpdate);
        await _listener?.dispose();
      } catch (error) {
        Logger.print('RTC listener disposal failed: ${error.runtimeType}');
      }
      // if (null != _room) {
      //   for (var e in _room!.remoteParticipants.values) {
      //     await e.dispose();
      //   }
      // }
      // await _room?.remoteParticipants.values.firstOrNull?.dispose();
      try {
        await _room?.disconnect();
      } catch (error) {
        Logger.print('RTC disconnect failed: ${error.runtimeType}');
      } finally {
        try {
          await _room?.dispose();
        } catch (error) {
          Logger.print('RTC disposal failed: ${error.runtimeType}');
        }
      }
    })();
    super.dispose();
  }

  @override
  Future<void> connect() async {
    final generation = ++_captureGeneration;
    final url = certificate.liveURL!;
    final token = certificate.token!;
    final busyLineUsers = certificate.busyLineUserIDList ?? [];
    if (busyLineUsers.isNotEmpty) {
      IMViews.showToast(StrRes.busyVideoCallHint);
    }
    // Try to connect to a room
    // This will throw an Exception if it fails for any reason.
    try {
      //create new room
      final room = Room();
      _room = room;

      // Create a Listener before connecting
      _listener = _room?.createListener();

      // Try to connect to the room
      // This will throw an Exception if it fails for any reason.
      await room.connect(url, token,
          roomOptions: RoomOptions(
              dynacast: true,
              adaptiveStream: true,
              defaultCameraCaptureOptions: const CameraCaptureOptions(
                  params: VideoParametersPresets.h540_169),
              defaultVideoPublishOptions: VideoPublishOptions(
                  simulcast: true,
                  videoCodec: 'VP9',
                  videoEncoding: const VideoEncoding(
                    maxBitrate: 5 * 1000 * 1000,
                    maxFramerate: 15,
                  ))));
      if (!mounted ||
          generation != _captureGeneration ||
          !identical(_room, room)) return;
      _room?.addListener(_onRoomDidUpdate);
      if (null != _listener) _setUpListeners();
      if (null != _room) roomDidUpdateSubject.add(_room!);
      _sortParticipants();
      WidgetsBindingCompatible.instance?.addPostFrameCallback((_) {
        unawaited(_publish(room, generation));
      });
    } catch (error, stackTrace) {
      if (!mounted || generation != _captureGeneration) return;
      widget.onError?.call(error, stackTrace);
      Logger.print('Could not connect $error  $stackTrace');
    }
  }

  void _setUpListeners() => _listener!
    ..on<RoomCallbackErrorEvent>((event) {
      if (!mounted) return;
      widget.onError?.call(event.error, event.stackTrace);
    })
    ..on<RoomDisconnectedEvent>((event) async {
      Logger.print('Room disconnected: reason => ${event.reason}');
      WidgetsBindingCompatible.instance?.addPostFrameCallback((_) {
        widget.onRoomDisconnected?.call();
        widget.onClose?.call();
      });
    })
    ..on<RoomRecordingStatusChanged>((event) {})
    ..on<LocalTrackPublishedEvent>((_) => _sortParticipants())
    ..on<LocalTrackUnpublishedEvent>((_) => _sortParticipants())
    ..on<ParticipantConnectedEvent>((_) => {})
    ..on<ParticipantDisconnectedEvent>((_) => {})
    ..on<DataReceivedEvent>((event) {
      String decoded = 'Failed to decode';
      try {
        decoded = utf8.decode(event.data);
      } catch (_) {
        Logger.print('Failed to decode: $_');
      }
    });

  Future<bool> _publish(Room room, int generation) async {
    bool current() =>
        mounted &&
        generation == _captureGeneration &&
        identical(_room, room) &&
        !room.engine.isClosed;
    if (!current()) return false;
    final participant = room.localParticipant;
    if (participant == null) {
      return observeCallCapture(
        stage: 'publish.participant',
        operation: () async {
          throw StateError('Local participant unavailable');
        },
        isCurrent: current,
      );
    }
    final published = await publishCallMedia(
      participant: participant,
      cameraEnabled: isVideoCall,
      microphoneEnabled: enabledMicrophone,
      isCurrent: current,
    );
    if (current()) callStateSubject.add(CallState.calling);
    return published;
  }

  void _onRoomDidUpdate() {
    _sortParticipants();
    if (null != _room) roomDidUpdateSubject.add(_room!);
  }

  void _sortParticipants() {
    if (null == _room) return;
    List<ParticipantTrack> userMediaTracks = [];

    final localParticipant = _room!.localParticipant;
    if (null != localParticipant) {
      VideoTrack? videoTrack;
      for (var t in localParticipant.videoTrackPublications) {
        if (!t.isScreenShare) {
          videoTrack = t.track;
          break;
        }
      }
      userMediaTracks.add(ParticipantTrack(
        participant: localParticipant,
        videoTrack: videoTrack,
        isScreenShare: false,
      ));
    }

    for (var participant in _room!.remoteParticipants.values) {
      // 排除观察者
      if (roomID == participant.identity) {
        continue;
      }
      VideoTrack? videoTrack;
      for (var t in participant.videoTrackPublications) {
        if (!t.isScreenShare) {
          videoTrack = t.track;
          break;
        }
      }
      userMediaTracks.add(ParticipantTrack(
        participant: participant,
        videoTrack: videoTrack,
        isScreenShare: false,
      ));
    }

    setState(() {
      participantTracks = [...userMediaTracks];
    });
  }
}
