// Copyright 2024 LiveKit, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

import 'dart:async';
import 'dart:collection';

import 'package:flutter/foundation.dart' show kIsWeb;

import 'package:connectivity_plus/connectivity_plus.dart';
import 'package:fixnum/fixnum.dart';
import 'package:flutter_webrtc/flutter_webrtc.dart' as rtc;
import 'package:meta/meta.dart';

import '../events.dart';
import '../exceptions.dart';
import '../extensions.dart';
import '../internal/events.dart';
import '../logger.dart';
import '../managers/event.dart';
import '../options.dart';
import '../proto/livekit_models.pb.dart' as lk_models;
import '../proto/livekit_rtc.pb.dart' as lk_rtc;
import '../support/disposable.dart';
import '../support/cleanup_sequence.dart';
import '../support/http_client.dart';
import '../support/platform.dart';
import '../support/websocket.dart';
import '../types/other.dart';
import '../utils.dart' show Utils, UriExt;

class _SignalSocketEpoch {
  _SignalSocketEpoch(this.id);
  final int id;
  LiveKitWebSocket? socket;
  bool closing = false;
  bool disconnectedNotified = false;
  Object? failure;
}

class SignalClient extends Disposable with EventsEmittable<SignalEvent>, CleanupFailureReporting {
  @override
  Future<bool> dispose() => reportCleanupResult(super.dispose);

  final _resourceCleanup = CleanupSingleFlight();
  ConnectionState _connectionState = ConnectionState.disconnected;
  ConnectionState get connectionState => _connectionState;

  final WebSocketConnector _wsConnector;
  LiveKitWebSocket? _ws;
  int _socketGeneration = 0;
  _SignalSocketEpoch? _socketEpoch;
  // Pending, active and physically unknown sockets share one bounded owner
  // table. Unknown failures are retained and cannot generate fresh attempts.
  static const _maxOwnedSocketEpochs = 8;
  final _ownedSocketEpochs = <int, _SignalSocketEpoch>{};
  Object? _retiredSocketFailure;
  StackTrace? _retiredSocketFailureStack;

  bool _ownsSocket(_SignalSocketEpoch epoch) =>
      !isDisposed && !epoch.closing && epoch.id == _socketGeneration && identical(_socketEpoch, epoch);

  void _traceSocket(String phase, _SignalSocketEpoch epoch, [Object? error]) {
    if (const bool.fromEnvironment('OPENIM_DEVELOPMENT')) {
      // Fixed role, epoch, phase and type only; no token, URL or SDP.
      // ignore: avoid_print
      print('[OpenIMRtcSocket] role=signal epoch=${epoch.id} phase=$phase'
          '${error == null ? '' : ' type=${error.runtimeType}'}');
    }
  }

  final _queue = Queue<lk_rtc.SignalRequest>();
  Duration? _pingTimeoutDuration;
  Timer? _pingTimeoutTimer;

  Duration? _pingIntervalDuration;
  Timer? _pingIntervalTimer;

  int get pingCount => _pingCount;
  int _pingCount = 0;

  /// Signal round-trip time in milliseconds, calculated from pingReq/pongResp.
  int get rtt => _rtt;
  int _rtt = 0;
  String? participantSid;

  int _requestId = 0;

  @internal
  int getNextRequestId() {
    _requestId += 1;
    return _requestId;
  }

  List<ConnectivityResult> _connectivityResult = [];
  StreamSubscription<List<ConnectivityResult>>? _connectivitySubscription;

  Future<bool> networkIsAvailable() async {
    // Skip check for web or flutter test
    if (kIsWeb || lkPlatformIsTest()) {
      return true;
    }
    _connectivityResult = await Connectivity().checkConnectivity();
    return _connectivityResult.isNotEmpty && !_connectivityResult.contains(ConnectivityResult.none);
  }

  @internal
  SignalClient(WebSocketConnector wsConnector) : _wsConnector = wsConnector {
    events.listen((event) {
      logger.fine('[SignalEvent] $event');
    });

    onDispose(() => recordCleanup(() => runCleanupSteps('SignalClient.dispose', {
          'socketAndTimers': cleanUp,
          'cancelEvents': () async {
            await events.cancelAll();
          },
          'disposeEvents': () async {
            await events.dispose();
          },
          'connectivity': () async {
            if (!kIsWeb && !lkPlatformIsTest()) {
              await _connectivitySubscription?.cancel();
              _connectivitySubscription = null;
            }
          },
        })));
  }

  @internal
  Future<void> connect(
    String uriString,
    String token, {
    required ConnectOptions connectOptions,
    required RoomOptions roomOptions,
    bool reconnect = false,
    lk_models.ReconnectReason? reconnectReason,
  }) async {
    final retiredFailure = _retiredSocketFailure;
    if (retiredFailure != null) Error.throwWithStackTrace(retiredFailure, _retiredSocketFailureStack!);
    if (_ownedSocketEpochs.length >= _maxOwnedSocketEpochs) {
      throw StateError('Bounded pending signal socket capacity is occupied');
    }
    final epoch = _SignalSocketEpoch(++_socketGeneration);
    _ownedSocketEpochs[epoch.id] = epoch;
    var connectorStarted = false;
    try {
      void requireCurrent() {
        if (isDisposed || epoch.closing || epoch.id != _socketGeneration) {
          throw StateError('Signal connection was superseded or cleaned up');
        }
      }

      if (!kIsWeb && !lkPlatformIsTest()) {
        final connectivityResult = await Connectivity().checkConnectivity();
        requireCurrent();
        _connectivityResult = connectivityResult;
        await _connectivitySubscription?.cancel();
        requireCurrent();
        _connectivitySubscription = Connectivity().onConnectivityChanged.listen((List<ConnectivityResult> result) {
          if (epoch.id != _socketGeneration || isDisposed) return;
          if (_connectivityResult != result) {
            if (result.contains(ConnectivityResult.none)) {
              logger.warning('lost connectivity');
            } else {
              logger.info('Connectivity changed, ${_connectivityResult} => ${result}');
            }
            events.emit(SignalConnectivityChangedEvent(
              oldState: _connectivityResult,
              state: result,
            ));
            _connectivityResult = result;
          }
        });

        if (_connectivityResult.contains(ConnectivityResult.none)) {
          logger.warning('no internet connection');
          throw ConnectException('no internet connection',
              reason: ConnectionErrorReason.InternalError, statusCode: 503);
        }
      }

      final rtcUri = await Utils.buildUri(
        uriString,
        token: token,
        connectOptions: connectOptions,
        roomOptions: roomOptions,
        reconnect: reconnect,
        sid: reconnect ? participantSid : null,
        reconnectReason: reconnectReason,
      );
      requireCurrent();

      logger.fine(
          'SignalClient connecting ${Uri(scheme: rtcUri.scheme, host: rtcUri.host, port: rtcUri.hasPort ? rtcUri.port : null, path: rtcUri.path)} epoch=${epoch.id}');

      try {
        // Cleanup never grants an older in-flight connect ownership of a newer
        // socket. Public cleanup invalidates the generation synchronously.
        await _cleanUpResources();
        requireCurrent();
        _socketEpoch = epoch;
        if (reconnect == true) {
          _connectionState = ConnectionState.reconnecting;
          events.emit(const SignalReconnectingEvent());
        } else {
          _connectionState = ConnectionState.connecting;
          events.emit(const SignalConnectingEvent());
        }
        // Attempt to connect
        connectorStarted = true;
        final connected = Future<LiveKitWebSocket>.sync(() => _wsConnector(
              rtcUri,
              options: WebSocketEventHandlers(
                onData: (dynamic data) => _ownsSocket(epoch) ? _onSocketData(data) : null,
                onDispose: () => _onSocketDispose(epoch),
                onError: (dynamic error) => _onSocketError(epoch, error),
              ),
              headers: {
                'Authorization': 'Bearer $token',
              },
              networkOptions: roomOptions.networkOptions,
            )).then((socket) async {
          epoch.socket = socket;
          if (!_ownsSocket(epoch) || epoch.failure != null) {
            _traceSocket('lateConnectDisposeBegin', epoch);
            try {
              await socket.dispose();
              _ownedSocketEpochs.remove(epoch.id);
            } catch (error, stack) {
              // This old resource cannot disconnect a new active epoch, but its
              // unknown lifetime remains strongly owned and blocks future opens.
              _retiredSocketFailure ??= error;
              _retiredSocketFailureStack ??= stack;
              _traceSocket('lateConnectDisposeFailed', epoch, error);
              rethrow;
            }
            _traceSocket('lateConnectDisposed', epoch);
            throw epoch.failure ?? StateError('Signal connection was superseded or cleaned up');
          }
          return socket;
        }, onError: (Object error, StackTrace stack) {
          // No WebSocket was handed to this epoch, so it has no native resource
          // left to retain. Connector startup failures still reach the caller.
          _ownedSocketEpochs.remove(epoch.id);
          Error.throwWithStackTrace(error, stack);
        });
        final socket = await connected.timeout(connectOptions.timeouts.connection);
        requireCurrent();
        _ws = socket;
        // Successful connection
        _connectionState = ConnectionState.connected;
        events.emit(const SignalConnectedEvent());
      } catch (socketError) {
        // A failed old-resource barrier is not a network connect rejection.
        // Preserve its exact error/stack and never replace it via validation.
        if (!connectorStarted) rethrow;
        if (epoch.id != _socketGeneration || isDisposed) rethrow;
        // The connector may still return later after a timeout. It owns its
        // cleanup via the continuation above, and may never populate _ws.
        epoch.closing = true;
        epoch.failure ??= socketError;
        if (socketError is CertificatePinningException) {
          // In reconnect mode the engine owns state and event emission,
          // emitting here would race its reconnect handling.
          if (!reconnect) {
            _connectionState = ConnectionState.disconnected;
            _notifySocketDisconnected(epoch, DisconnectReason.signalingConnectionFailure);
          }
          rethrow;
        }

        // Skip validation if reconnect mode
        if (reconnect) rethrow;

        // Attempt Validation
        var finalError = socketError;
        try {
          // Re-build same uri for validate mode
          final validateUri = await Utils.buildUri(
            uriString,
            token: token,
            connectOptions: connectOptions,
            roomOptions: roomOptions,
            validate: true,
            forceSecure: rtcUri.isSecureScheme,
          );

          final validateResponse = await sdkHttpGet(
            validateUri,
            headers: {
              'Authorization': 'Bearer $token',
            },
            networkOptions: roomOptions.networkOptions,
          );
          if (validateResponse.statusCode != 200) {
            finalError = ConnectException(validateResponse.body,
                reason: validateResponse.statusCode >= 400
                    ? ConnectionErrorReason.NotAllowed
                    : ConnectionErrorReason.InternalError,
                statusCode: validateResponse.statusCode);
          }
        } catch (error) {
          if (socketError.runtimeType != error.runtimeType) {
            finalError = error;
          }
        } finally {
          if (epoch.id == _socketGeneration && !isDisposed) {
            _connectionState = ConnectionState.disconnected;
            _notifySocketDisconnected(epoch, DisconnectReason.signalingConnectionFailure);
          }
          throw finalError;
        }
      }
    } finally {
      if (!connectorStarted) _ownedSocketEpochs.remove(epoch.id);
    }
  }

  Future<void> sendLeave() async {
    _sendRequest(lk_rtc.SignalRequest(
        leave: lk_rtc.LeaveRequest(
      reason: lk_models.DisconnectReason.CLIENT_INITIATED,
      // server doesn't process this field, keeping it here to indicate the intent of a full disconnect
      action: lk_rtc.LeaveRequest_Action.DISCONNECT,
    )));
  }

  // resets internal state to a re-usable state
  @internal
  Future<void> cleanUp() {
    ++_socketGeneration;
    _socketEpoch?.closing = true;
    return _cleanUpResources();
  }

  Future<void> _cleanUpResources() => _resourceCleanup.run(() async {
        _connectionState = ConnectionState.disconnected;
        final epoch = _socketEpoch;
        if (epoch != null) epoch.closing = true;
        final socket = epoch?.socket ?? _ws;
        await runCleanupSteps('SignalClient.resources', {
          'socket': () async {
            await socket?.dispose();
            if (socket != null && epoch != null) _ownedSocketEpochs.remove(epoch.id);
            if (identical(_ws, socket)) _ws = null;
            if (identical(_socketEpoch, epoch)) _socketEpoch = null;
          },
          'queue': () {
            _queue.clear();
          },
          'pingTimers': _clearPingInterval,
          'retiredSocketFailure': () {
            final failure = _retiredSocketFailure;
            if (failure != null) Error.throwWithStackTrace(failure, _retiredSocketFailureStack!);
          },
        });
      });

  void _sendRequest(
    lk_rtc.SignalRequest req, {
    bool enqueueIfReconnecting = true,
  }) {
    if (isDisposed) {
      logger.warning('[$objectId] Could not send message, already disposed');
      return;
    }

    if (connectionState == ConnectionState.reconnecting && req._canQueue() && enqueueIfReconnecting) {
      _queue.add(req);
      return;
    }

    if (connectionState != ConnectionState.connected) {
      logger.warning('[$objectId] Could not send message, socket not connected');
      return;
    }

    _ws?.send(req.writeToBuffer());
  }

  Future<void> _onSocketData(dynamic message) async {
    if (message is! List<int>) return;
    final msg = lk_rtc.SignalResponse.fromBuffer(message);

    switch (msg.whichMessage()) {
      case lk_rtc.SignalResponse_Message.join:
        if (msg.join.pingTimeout > 0) {
          _pingTimeoutDuration = Duration(seconds: msg.join.pingTimeout);
          _pingIntervalDuration = Duration(seconds: msg.join.pingInterval);
          logger.info('ping config timeout: ${msg.join.pingTimeout}, interval: ${msg.join.pingInterval} ');
          _startPingInterval();
        }
        participantSid = msg.join.participant.sid;
        events.emit(SignalJoinResponseEvent(response: msg.join));
        break;
      case lk_rtc.SignalResponse_Message.answer:
        events.emit(SignalAnswerEvent(sd: msg.answer.toSDKType()));
        break;
      case lk_rtc.SignalResponse_Message.offer:
        events.emit(SignalOfferEvent(sd: msg.offer.toSDKType()));
        break;
      case lk_rtc.SignalResponse_Message.trickle:
        events.emit(SignalTrickleEvent(
          candidate: RTCIceCandidateExt.fromJson(msg.trickle.candidateInit),
          target: msg.trickle.target,
        ));
        break;
      case lk_rtc.SignalResponse_Message.update:
        events.emit(SignalParticipantUpdateEvent(participants: msg.update.participants));
        break;
      case lk_rtc.SignalResponse_Message.trackPublished:
        events.emit(SignalLocalTrackPublishedEvent(
          cid: msg.trackPublished.cid,
          track: msg.trackPublished.track,
        ));
        break;
      case lk_rtc.SignalResponse_Message.trackUnpublished:
        events.emit(SignalTrackUnpublishedEvent(
          trackSid: msg.trackUnpublished.trackSid,
        ));
        break;
      case lk_rtc.SignalResponse_Message.trackSubscribed:
        events.emit(SignalLocalTrackSubscribedEvent(
          trackSid: msg.trackSubscribed.trackSid,
        ));
        break;
      case lk_rtc.SignalResponse_Message.speakersChanged:
        events.emit(SignalSpeakersChangedEvent(speakers: msg.speakersChanged.speakers));
        break;
      case lk_rtc.SignalResponse_Message.roomUpdate:
        events.emit(SignalRoomUpdateEvent(room: msg.roomUpdate.room));
        break;
      case lk_rtc.SignalResponse_Message.connectionQuality:
        events.emit(SignalConnectionQualityUpdateEvent(
          updates: msg.connectionQuality.updates,
        ));
        break;
      case lk_rtc.SignalResponse_Message.leave:
        events.emit(SignalLeaveEvent(request: msg.leave));
        break;
      case lk_rtc.SignalResponse_Message.mute:
        events.emit(SignalRemoteMuteTrackEvent(
          sid: msg.mute.sid,
          muted: msg.mute.muted,
        ));
        break;
      case lk_rtc.SignalResponse_Message.streamStateUpdate:
        events.emit(SignalStreamStateUpdatedEvent(
          updates: msg.streamStateUpdate.streamStates,
        ));
        break;
      case lk_rtc.SignalResponse_Message.subscribedQualityUpdate:
        events.emit(SignalSubscribedQualityUpdatedEvent(
          trackSid: msg.subscribedQualityUpdate.trackSid,
          // ignore: deprecated_member_use_from_same_package
          subscribedQualities: msg.subscribedQualityUpdate.subscribedQualities,
          subscribedCodecs: msg.subscribedQualityUpdate.subscribedCodecs,
        ));
        break;
      case lk_rtc.SignalResponse_Message.subscriptionPermissionUpdate:
        events.emit(SignalSubscriptionPermissionUpdateEvent(
          participantSid: msg.subscriptionPermissionUpdate.participantSid,
          trackSid: msg.subscriptionPermissionUpdate.trackSid,
          allowed: msg.subscriptionPermissionUpdate.allowed,
        ));
        break;
      case lk_rtc.SignalResponse_Message.refreshToken:
        events.emit(SignalTokenUpdatedEvent(token: msg.refreshToken));
        break;
      case lk_rtc.SignalResponse_Message.notSet:
        logger.info('signal message not set');
        break;
      case lk_rtc.SignalResponse_Message.pong:
        _pingCount++;
        _resetPingTimeout();
        break;
      case lk_rtc.SignalResponse_Message.pongResp:
        _rtt = DateTime.timestamp().millisecondsSinceEpoch - msg.pongResp.lastPingTimestamp.toInt();
        _pingCount++;
        _resetPingTimeout();
        break;
      case lk_rtc.SignalResponse_Message.reconnect:
        events.emit(SignalReconnectResponseEvent(response: msg.reconnect));
        break;
      case lk_rtc.SignalResponse_Message.requestResponse:
        logger.fine('received request response: ${msg.requestResponse.reason}');
        events.emit(SignalRequestResponseEvent(response: msg.requestResponse));
        break;
      case lk_rtc.SignalResponse_Message.roomMoved:
        logger.fine('received room moved: ${msg.roomMoved.room.name}');
        if (msg.roomMoved.token.isNotEmpty) {
          events.emit(SignalTokenUpdatedEvent(token: msg.roomMoved.token));
        }
        events.emit(SignalRoomMovedEvent(response: msg.roomMoved));
        break;
      default:
        logger.warning('received unknown signal message');
    }
  }

  Future<void> _onSocketError(_SignalSocketEpoch epoch, dynamic error) async {
    epoch.failure ??= error;
    _traceSocket('error', epoch, error);
    logger.warning('received websocket error epoch=${epoch.id} type=${error.runtimeType}');
    if (!_ownsSocket(epoch)) return;
    _notifySocketDisconnected(epoch, DisconnectReason.signalingConnectionFailure);
    // Mark before awaiting so onDone and other errors cannot emit twice.
    epoch.closing = true;
    await _cleanUpResources();
  }

  void _notifySocketDisconnected(_SignalSocketEpoch epoch, DisconnectReason reason) {
    if (epoch.disconnectedNotified || epoch.id != _socketGeneration || isDisposed) return;
    epoch.disconnectedNotified = true;
    _connectionState = ConnectionState.disconnected;
    events.emit(SignalDisconnectedEvent(reason: reason));
  }

  Future<void> _onSocketDispose(_SignalSocketEpoch epoch) async {
    if (!_ownsSocket(epoch)) return;
    _traceSocket('physicalDone', epoch);
    _notifySocketDisconnected(epoch, DisconnectReason.disconnected);
    epoch.closing = true;
    await _cleanUpResources();
  }

  void _sendPing() {
    final now = DateTime.timestamp().millisecondsSinceEpoch;
    // Send both ping and pingReq for compatibility with old and new servers
    _sendRequest(lk_rtc.SignalRequest()..ping = Int64(now));
    _sendRequest(lk_rtc.SignalRequest()
      ..pingReq = lk_rtc.Ping(
        timestamp: Int64(now),
        rtt: Int64(_rtt),
      ));
  }

  void _startPingInterval() {
    _clearPingInterval();
    _resetPingTimeout();

    if (_pingIntervalDuration == null) {
      logger.warning('ping timeout duration not set');
      return;
    }

    _pingIntervalTimer ??= Timer.periodic(_pingIntervalDuration!, (_) => _sendPing());
  }

  void _clearPingInterval() {
    _clearPingTimeout();
    _pingIntervalTimer?.cancel();
    _pingIntervalTimer = null;
  }

  void _resetPingTimeout() {
    _clearPingTimeout();
    if (_pingTimeoutDuration == null) {
      logger.warning('ping timeout duration not set');
      return;
    }
    _pingTimeoutTimer ??= Timer(_pingTimeoutDuration!, () {
      logger.warning('ping timeout');
      final epoch = _socketEpoch;
      if (epoch == null) return;
      unawaited(_onSocketError(epoch, StateError('Signal ping timed out')).then<void>((_) {},
          onError: (Object error, StackTrace stack) => _traceSocket('pingCleanupFailed', epoch, error)));
    });
  }

  void _clearPingTimeout() {
    _pingTimeoutTimer?.cancel();
    _pingTimeoutTimer = null;
  }
}

extension SignalClientRequests on SignalClient {
  @internal
  void sendOffer(rtc.RTCSessionDescription offer) => _sendRequest(lk_rtc.SignalRequest(
        offer: offer.toPBType(),
      ));

  @internal
  void sendAnswer(rtc.RTCSessionDescription answer) => _sendRequest(lk_rtc.SignalRequest(
        answer: answer.toPBType(),
      ));

  @internal
  void sendIceCandidate(rtc.RTCIceCandidate candidate, lk_rtc.SignalTarget target) => _sendRequest(
        lk_rtc.SignalRequest(
          trickle: lk_rtc.TrickleRequest(
            candidateInit: candidate.toJson(),
            target: target,
          ),
        ),
      );

  @internal
  void sendMuteTrack(String trackSid, bool muted) => _sendRequest(lk_rtc.SignalRequest(
        mute: lk_rtc.MuteTrackRequest(
          sid: trackSid,
          muted: muted,
        ),
      ));

  @internal
  void sendAddTrack(lk_rtc.AddTrackRequest req) => _sendRequest(lk_rtc.SignalRequest(
        addTrack: req,
      ));

  @internal
  int sendUpdateLocalMetadata(lk_rtc.UpdateParticipantMetadata metadata) {
    final requestId = getNextRequestId();
    metadata.requestId = requestId;
    _sendRequest(lk_rtc.SignalRequest(updateMetadata: metadata));
    return requestId;
  }

  @internal
  void sendUpdateTrackSettings(lk_rtc.UpdateTrackSettings settings) => _sendRequest(lk_rtc.SignalRequest(
        trackSetting: settings,
      ));

  @internal
  void sendUpdateSubscription(lk_rtc.UpdateSubscription subscription) => _sendRequest(lk_rtc.SignalRequest(
        subscription: subscription,
      ));

  @internal
  void sendUpdateSubscriptionPermissions({
    required bool allParticipants,
    required Iterable<lk_rtc.TrackPermission> trackPermissions,
  }) =>
      _sendRequest(lk_rtc.SignalRequest(
        subscriptionPermission: lk_rtc.SubscriptionPermission(
          allParticipants: allParticipants,
          trackPermissions: trackPermissions,
        ),
      ));

  @internal
  void sendSyncState({
    required lk_rtc.SessionDescription? answer,
    required lk_rtc.SessionDescription? offer,
    required lk_rtc.UpdateSubscription subscription,
    required Iterable<lk_rtc.TrackPublishedResponse>? publishTracks,
    required Iterable<lk_rtc.DataChannelInfo>? dataChannelInfo,
    required List<String> trackSidsDisabled,
    List<lk_rtc.DataChannelReceiveState>? dataChannelReceiveStates,
  }) =>
      _sendRequest(lk_rtc.SignalRequest(
        syncState: lk_rtc.SyncState(
          answer: answer,
          offer: offer,
          subscription: subscription,
          publishTracks: publishTracks,
          dataChannels: dataChannelInfo,
          trackSidsDisabled: trackSidsDisabled,
          datachannelReceiveStates: dataChannelReceiveStates,
        ),
      ));

  @internal
  void sendSimulateScenario({
    int? speakerUpdate,
    bool? nodeFailure,
    bool? migration,
    bool? serverLeave,
    bool? switchCandidate,
  }) =>
      _sendRequest(lk_rtc.SignalRequest(
        simulate: lk_rtc.SimulateScenario(
          speakerUpdate: speakerUpdate,
          nodeFailure: nodeFailure,
          migration: migration,
          serverLeave: serverLeave,
          switchCandidateProtocol: (switchCandidate != null && switchCandidate) ? lk_rtc.CandidateProtocol.TCP : null,
        ),
      ));
}

// private methods
extension on lk_rtc.SignalRequest {
  // returns if this request can be queued
  bool _canQueue() => ![
        // list of types that cannot be queued
        lk_rtc.SignalRequest_Message.syncState,
        lk_rtc.SignalRequest_Message.trickle,
        lk_rtc.SignalRequest_Message.answer,
        lk_rtc.SignalRequest_Message.simulate
      ].contains(whichMessage());
}

// internal methods
extension SignalClientInternalMethods on SignalClient {
  @internal
  void sendQueuedRequests() {
    // queue is empty
    if (_queue.isEmpty) return;
    // send requests
    final queueCopy = List.of(_queue);
    _queue.clear();
    for (final request in queueCopy) {
      _sendRequest(request, enqueueIfReconnecting: false);
    }
  }

  @internal
  void clearQueue() => _queue.clear();
}
