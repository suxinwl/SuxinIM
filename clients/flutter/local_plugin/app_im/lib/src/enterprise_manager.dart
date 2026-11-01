import 'dart:async';
import 'dart:developer' as developer;
import 'package:dio/dio.dart';
import '../app_im.dart';

/// Enterprise transport is separate from the unmodified public SDK.
class EnterpriseManager {
  final Dio _dio;
  EnterpriseManager({Dio? dio})
    : _dio =
          dio ??
          Dio(
            BaseOptions(
              connectTimeout: const Duration(seconds: 15),
              receiveTimeout: const Duration(seconds: 30),
            ),
          );
  String Function()? _baseUrl;
  String? Function()? _imToken;
  String? Function()? _chatToken;
  int Function()? _platform;
  bool Function()? _ready;
  String? Function()? _userID;
  OnSignalingListener? listener;
  final meetingInfoChanges = StreamController<MeetingInfo>.broadcast();
  void Function(GroupMessageReceipt)? onGroupReadReceipt;
  void Function(Map<String, dynamic>)? onMeetingInfoChanged;
  Timer? _timer;
  int _cursor = -1;
  String? _sessionToken;
  bool _pulling = false;
  bool _meetingsNeedRefresh = true;
  int _terminationGeneration = 0;
  int get callTerminationGeneration => _terminationGeneration;
  late final _terminations = CallTerminationCoordinator(
    request: _terminationRequest,
    current: (session) =>
        session.generation == _terminationGeneration &&
        (_ready?.call() ?? true) &&
        session.userID == _userID?.call() &&
        session.imToken == _imToken?.call() &&
        session.chatToken == _chatToken?.call() &&
        session.platformID == _platform?.call() &&
        session.baseUrl == _baseUrl?.call(),
  );

  void invalidateCallTerminations() {
    _terminationGeneration++;
    _terminations.invalidate();
  }

  Future<List<CallTerminationResult>> retryPendingCallTerminations() =>
      _terminations.retryPending();

  /// Capture all authentication and routing before the first await. Retry code
  /// never consults mutable _options belonging to a later login.
  Future<CallTerminationResult> terminateCall({
    required String method,
    required SignalingInfo info,
  }) {
    final base = _baseUrl?.call();
    final user = _userID?.call();
    final im = _imToken?.call();
    final chat = _chatToken?.call();
    final platform = _platform?.call();
    if (base == null ||
        user == null ||
        im == null ||
        chat == null ||
        platform == null ||
        !(_ready?.call() ?? true)) {
      return Future.value(
        CallTerminationResult(
          info.invitation?.roomID ?? '',
          method,
          CallTerminationStatus.cancelled,
          0,
          'SessionUnavailable',
        ),
      );
    }
    return _terminations.terminate(
      CallTerminationSession(
        base,
        user,
        im,
        chat,
        platform,
        _terminationGeneration,
      ),
      method,
      info.toJson(),
    );
  }

  Future<Map<String, dynamic>> _terminationRequest(
    CallTerminationSession session,
    String path,
    Map<String, dynamic> body,
    CancelToken cancellation,
  ) async {
    final watch = Stopwatch()..start();
    final phase = path.endsWith('signal_message_assemble')
        ? (body['signalReq'] as Map).keys.single.toString()
        : 'terminationReconcile';
    void diagnostic(String result) {
      if (const bool.fromEnvironment('OPENIM_DEVELOPMENT')) {
        developer.log(
          'CallSignal phase=$phase elapsedMs=${watch.elapsedMilliseconds} result=$result',
          name: 'OpenIM38',
        );
      }
    }

    try {
      final response = await _dio.post<Map<String, dynamic>>(
        '${session.baseUrl}$path',
        data: body,
        cancelToken: cancellation,
        options: Options(
          headers: {
            'token': session.imToken,
            'chatToken': session.chatToken,
            'platformID': session.platformID,
            'operationID': DateTime.now().microsecondsSinceEpoch.toString(),
          },
        ),
      );
      if (response.data?['errCode'] != 0) {
        diagnostic('applicationRejected');
        throw StateError('Call termination was not confirmed by server');
      }
      diagnostic('http${response.statusCode}');
      return Map<String, dynamic>.from(response.data?['data'] ?? {});
    } catch (error) {
      diagnostic(error.runtimeType.toString());
      rethrow;
    }
  }

  void configure({
    required String Function() baseUrl,
    required String? Function() imToken,
    required String? Function() chatToken,
    required int Function() platform,
    bool Function()? ready,
    String? Function()? userID,
  }) {
    _baseUrl = baseUrl;
    _imToken = imToken;
    _chatToken = chatToken;
    _platform = platform;
    _ready = ready;
    _userID = userID;
  }

  Options get _options => Options(
    headers: {
      'token': _imToken?.call(),
      'chatToken': _chatToken?.call(),
      'platformID': _platform?.call(),
      'operationID': DateTime.now().microsecondsSinceEpoch.toString(),
    },
  );
  Future<Map<String, dynamic>> request(
    String path, [
    Map<String, dynamic> data = const {},
  ]) async {
    if (_baseUrl == null)
      throw StateError('Enterprise transport is not configured');
    final response = await _dio.post<Map<String, dynamic>>(
      '${_baseUrl!.call()}$path',
      data: data,
      options: _options,
    );
    final body = response.data!;
    if (body['errCode'] != 0)
      throw StateError('${body['errMsg']}: ${body['errDlt'] ?? ''}');
    final result = body['data'];
    return result is Map
        ? Map<String, dynamic>.from(result)
        : <String, dynamic>{};
  }

  /// Validate the immutable pair from a particular login, never mutable headers.
  Future<void> verifySession({
    required String imToken,
    required String chatToken,
    required int platformID,
  }) async {
    if (_baseUrl == null)
      throw StateError('Enterprise transport is not configured');
    final response = await _dio.post<Map<String, dynamic>>(
      '${_baseUrl!.call()}/signaling/get_signal_invitation_info_start_app',
      data: <String, dynamic>{},
      options: Options(
        headers: {
          'token': imToken,
          'chatToken': chatToken,
          'platformID': platformID,
          'operationID': DateTime.now().microsecondsSinceEpoch.toString(),
        },
      ),
    );
    if (response.data?['errCode'] != 0) {
      throw StateError('IM session verification failed; retry login');
    }
  }

  Future<void> setSignalingListener(OnSignalingListener value) async {
    listener = value;
    _timer?.cancel();
    _timer = Timer.periodic(const Duration(seconds: 3), (_) => pullEvents());
    await pullEvents();
  }

  void dispose() {
    invalidateCallTerminations();
    _timer?.cancel();
    _cursor = -1;
    _sessionToken = null;
    _meetingsNeedRefresh = true;
  }

  Future<void> pullEvents() async {
    final token = _imToken?.call();
    if ((_ready != null && !_ready!()) ||
        _pulling ||
        token == null ||
        token.isEmpty ||
        _baseUrl == null)
      return;
    if (_sessionToken != token) {
      _cursor = -1;
      _sessionToken = token;
      _meetingsNeedRefresh = true;
    }
    _pulling = true;
    try {
      final response = await _dio.get<Map<String, dynamic>>(
        '${_baseUrl!.call()}/rtc/events',
        queryParameters: {'after': _cursor},
        options: _options,
      );
      final body = response.data!;
      if (body['errCode'] != 0) return;
      if (_imToken?.call() != token) return;
      final data = Map<String, dynamic>.from(body['data'] ?? {});
      if (_cursor < 0 || data['resyncRequired'] == true) {
        _cursor = (data['cursor'] as num?)?.toInt() ?? 0;
        final active = await getSignalingInvitationInfoStartApp();
        if (active.invitation != null) listener?.receiveNewInvitation(active);
        _meetingsNeedRefresh = true;
        await _refreshMeetings();
        return;
      }
      for (final raw in (data['events'] as List? ?? [])) {
        final event = Map<String, dynamic>.from(raw);
        final sequence = (event['id'] ?? event['sequence'] ?? 0) as int;
        if (sequence <= _cursor) continue;
        if (event['event'] == 'OnReceiveNewInvitation') {
          final raw = Map<String, dynamic>.from(event['data'] ?? {});
          final roomID = raw['invitation']?['roomID'];
          final current = await request(
            '/signaling/get_signal_invitation_info',
            {'roomID': roomID},
          );
          if (current['ended'] == true ||
              current['activeInvitationForActor'] == false ||
              !['invited', 'active'].contains(current['state'])) {
            _cursor = sequence;
            continue;
          }
        }
        await _dispatch(event);
        _cursor = sequence;
      }
      await _refreshMeetings();
    } catch (_) {
      /* Network/auth errors retain the cursor for reconnect. */
    } finally {
      _pulling = false;
    }
  }

  Future<void> _refreshMeetings() async {
    if (!_meetingsNeedRefresh) return;
    final meetings = await signalingGetMeetings();
    for (final info in meetings.meetingInfoList ?? <MeetingInfo>[]) {
      meetingInfoChanges.add(info);
    }
    _meetingsNeedRefresh = false;
  }

  Future<void> _dispatch(Map<String, dynamic> event) async {
    final type = event['type'] ?? event['event'];
    final payload = Map<String, dynamic>.from(event['data'] ?? {});
    final info = SignalingInfo.fromJson(payload);
    switch (type) {
      case 'OnGroupReadReceipt':
        onGroupReadReceipt?.call(GroupMessageReceipt.fromJson(payload));
        break;
      case 'OnMeetingInfoChanged':
        onMeetingInfoChanged?.call(payload);
        if (payload['meetingInfo'] is Map)
          meetingInfoChanges.add(
            MeetingInfo.fromJson(
              Map<String, dynamic>.from(payload['meetingInfo']),
            ),
          );
        break;
      case 'invite':
      case 'inviteInGroup':
      case 'onReceiveNewInvitation':
      case 'OnReceiveNewInvitation':
        listener?.receiveNewInvitation(info);
        break;
      case 'accept':
      case 'onInviteeAccepted':
      case 'OnInviteeAccepted':
        if (payload['opUserID'] == _userID?.call() &&
            payload['opPlatformID'] != null &&
            payload['opPlatformID'] != _platform?.call()) {
          listener?.inviteeAcceptedByOtherDevice(info);
        } else {
          listener?.inviteeAccepted(info);
        }
        break;
      case 'reject':
      case 'onInviteeRejected':
      case 'OnInviteeRejected':
        if (payload['opUserID'] == _userID?.call() &&
            payload['opPlatformID'] != null &&
            payload['opPlatformID'] != _platform?.call()) {
          listener?.inviteeRejectedByOtherDevice(info);
        } else {
          listener?.inviteeRejected(info);
        }
        break;
      case 'cancel':
      case 'onInvitationCancelled':
      case 'OnInvitationCancelled':
        listener?.invitationCancelled(info);
        break;
      case 'timeout':
      case 'onInvitationTimeout':
      case 'OnInvitationTimeout':
        listener?.invitationTimeout(info);
        break;
      case 'hungUp':
      case 'hangup':
      case 'onHangup':
      case 'OnHangUp':
        listener?.hangup(info);
        break;
      case 'onInviteeAcceptedByOtherDevice':
      case 'OnInviteeAcceptedByOtherDevice':
        listener?.inviteeAcceptedByOtherDevice(info);
        break;
      case 'onInviteeRejectedByOtherDevice':
      case 'OnInviteeRejectedByOtherDevice':
        listener?.inviteeRejectedByOtherDevice(info);
        break;
      case 'onRoomParticipantConnected':
      case 'OnRoomParticipantConnected':
        listener?.roomParticipantConnected(await _roomInfo(payload));
        break;
      case 'onRoomParticipantDisconnected':
      case 'OnRoomParticipantDisconnected':
        listener?.roomParticipantDisconnected(await _roomInfo(payload));
        break;
      case 'onMeetingStreamChanged':
      case 'OnMeetingStreamChanged':
      case 'OnStreamChanged':
        if (payload['userID'] == null || payload['userID'] == _userID?.call()) {
          if (payload['muteAll'] == true) {
            for (final kind in ['audio', 'video']) {
              listener?.streamChangedEvent(
                MeetingStreamEvent.fromJson({...payload, 'streamType': kind}),
              );
            }
          } else {
            listener?.streamChangedEvent(MeetingStreamEvent.fromJson(payload));
          }
        }
        break;
      case 'onReceiveCustomSignal':
      case 'OnReceiveCustomSignal':
        listener?.receiveCustomSignal(CustomSignaling.fromJson(payload));
        break;
    }
  }

  Future<RoomCallingInfo> _roomInfo(Map<String, dynamic> payload) async {
    final info = RoomCallingInfo.fromJson(payload);
    final participants = info.participant;
    if (participants == null || participants.isEmpty) return info;
    final ids = participants
        .map((p) => p.userInfo?.userID)
        .whereType<String>()
        .toList();
    if (ids.isEmpty) return info;
    final profiles = await AppIm.iMManager.userManager.getUsersInfo(
      userIDList: ids,
    );
    for (final person in participants) {
      final profile = profiles
          .where((u) => u.userID == person.userInfo?.userID)
          .firstOrNull;
      if (profile != null)
        person.userInfo = UserInfo.fromJson(profile.toJson());
    }
    return info;
  }

  Future<Map<String, dynamic>> _signal(
    String method,
    Map<String, dynamic> info,
  ) async {
    final data = await request('/signaling/signal_message_assemble', {
      'signalReq': {method: info},
    });
    final response = Map<String, dynamic>.from(data['signalResp'] ?? data);
    return Map<String, dynamic>.from(response[method] ?? response);
  }

  Future<SignalingCertificate> signalingInvite({
    required SignalingInfo info,
    String? operationID,
  }) async =>
      SignalingCertificate.fromJson(await _signal('invite', info.toJson()));
  Future<SignalingCertificate> signalingInviteInGroup({
    required SignalingInfo info,
    String? operationID,
  }) async => SignalingCertificate.fromJson(
    await _signal('inviteInGroup', info.toJson()),
  );
  Future<SignalingCertificate> signalingAccept({
    required SignalingInfo info,
    String? operationID,
  }) async =>
      SignalingCertificate.fromJson(await _signal('accept', info.toJson()));
  Future<dynamic> signalingReject({
    required SignalingInfo info,
    String? operationID,
  }) => _signal('reject', info.toJson());
  Future<dynamic> signalingCancel({
    required SignalingInfo info,
    String? operationID,
  }) => _signal('cancel', info.toJson());
  Future<dynamic> signalingHungUp({
    required SignalingInfo info,
    String? operationID,
  }) => _signal('hungUp', info.toJson());
  Future<RoomCallingInfo> signalingGetRoomByGroupID({
    required String groupID,
    String? operationID,
  }) async => _roomInfo(
    await request('/signaling/signal_get_room_by_group_id', {
      'groupID': groupID,
    }),
  );
  Future<SignalingCertificate> signalingGetTokenByRoomID({
    required String roomID,
    String? operationID,
  }) async => SignalingCertificate.fromJson(
    await _signal('getTokenByRoomID', {'roomID': roomID}),
  );
  Future<dynamic> signalingUpdateMeetingInfo({
    required Map info,
    String? operationID,
  }) => request(
    '/signaling/signal_update_meeting_info',
    Map<String, dynamic>.from(info)
      ..['roomID'] = info['roomID'] ?? info['meetingID'],
  );
  Future<SignalingCertificate> signalingCreateMeeting({
    required String meetingName,
    String? meetingHostUserID,
    int? startTime,
    int? meetingDuration,
    List<String> inviteeUserIDList = const [],
    String? ex,
    String? operationID,
  }) async => SignalingCertificate.fromJson(
    await request('/signaling/signal_create_meeting', {
      'meetingName': meetingName,
      'meetingHostUserID': meetingHostUserID,
      'startTime': startTime,
      'meetingDuration': meetingDuration,
      'inviteeUserIDList': inviteeUserIDList,
      'ex': ex,
    }),
  );
  Future<SignalingCertificate> signalingJoinMeeting({
    required String roomID,
    String? meetingName,
    String? participantNickname,
    String? operationID,
  }) async => SignalingCertificate.fromJson(
    await request('/signaling/signal_join_meeting', {
      'roomID': roomID,
      'meetingName': meetingName,
      'participantNickname': participantNickname,
    }),
  );
  Future<dynamic> signalingOperateStream({
    required String roomID,
    String? streamType,
    required String userID,
    bool mute = false,
    bool muteAll = false,
    String? operationID,
  }) => request('/signaling/signal_operate_stream', {
    'roomID': roomID,
    'streamType': streamType,
    'userID': userID,
    'mute': mute,
    'muteAll': muteAll,
  });
  Future<MeetingInfoList> signalingGetMeetings({String? operationID}) async =>
      MeetingInfoList.fromJson(await request('/signaling/signal_get_meetings'));
  Future<dynamic> signalingCloseRoom({
    required String roomID,
    String? operationID,
  }) => request('/signaling/signal_close_room', {'roomID': roomID});
  Future<dynamic> signalingSendCustomSignal({
    required String roomID,
    required String customInfo,
    String? operationID,
  }) => request('/signaling/signal_send_custom_signal', {
    'roomID': roomID,
    'customInfo': customInfo,
  });
  Future<SignalingInfo> getSignalingInvitationInfoStartApp({
    String? operationID,
  }) async => SignalingInfo.fromJson(
    await request('/signaling/get_signal_invitation_info_start_app'),
  );
}
