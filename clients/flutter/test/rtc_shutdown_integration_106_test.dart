// Independent real Room -> real Engine lifecycle tests. Only wire/network and
// native objects are doubles; the production connect, event dispatch,
// participant, publication, track, transport and shutdown layers remain real.
import 'dart:async';

import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_webrtc/flutter_webrtc.dart' as rtc;
import 'package:livekit_client/src/core/engine.dart';
import 'package:livekit_client/src/core/room.dart';
import 'package:livekit_client/src/core/signal_client.dart';
import 'package:livekit_client/src/events.dart';
import 'package:livekit_client/src/internal/events.dart';
import 'package:livekit_client/src/managers/event.dart';
import 'package:livekit_client/src/options.dart';
import 'package:livekit_client/src/proto/livekit_models.pb.dart' as models;
import 'package:livekit_client/src/proto/livekit_rtc.pb.dart' as protocol;
import 'package:livekit_client/src/publication/local.dart';
import 'package:livekit_client/src/support/websocket.dart';
import 'package:livekit_client/src/track/local/video.dart';
import 'package:livekit_client/src/track/options.dart';
import 'package:livekit_client/src/types/other.dart';

Future<void> _until(bool Function() condition) async {
  final until = DateTime.now().add(const Duration(seconds: 3));
  while (!condition()) {
    if (DateTime.now().isAfter(until)) fail('Fixture state never became ready');
    await Future<void>.delayed(Duration.zero);
  }
}

Future<void> _turns([int count = 5]) async {
  for (var i = 0; i < count; i++) {
    await Future<void>.delayed(Duration.zero);
  }
}

Future<void> _observeFailure(Future<dynamic> future) async {
  try {
    await future;
  } catch (_) {
    // Test teardown only; assertions below observe the actual operation errors.
  }
}

class _Channel extends Fake implements rtc.RTCDataChannel {
  final states = StreamController<rtc.RTCDataChannelState>.broadcast();
  @override
  Stream<rtc.RTCDataChannelState> get stateChangeStream => states.stream;
  @override
  int? bufferedAmountLowThreshold = 0;
  @override
  dynamic noSuchMethod(Invocation invocation) => null;
}

class _Peer extends Fake implements rtc.RTCPeerConnection {
  _Peer(this.name, this.trace);
  final String name;
  final List<String> trace;
  final channels = <_Channel>[];
  int closeCalls = 0;
  int disposeCalls = 0;
  Completer<void>? closeBarrier;
  Completer<void>? signalingBarrier;
  Completer<void>? answerBarrier;
  final answerEntered = Completer<void>();
  final remoteDescriptions = <rtc.RTCSessionDescription>[];
  final localDescriptions = <rtc.RTCSessionDescription>[];
  int answerCalls = 0;
  Object? closeFailure;
  Object? signalingFailure;
  int signalingReads = 0;
  @override
  void Function(rtc.RTCPeerConnectionState)? onConnectionState;
  @override
  Future<rtc.RTCSignalingState?> getSignalingState() async {
    signalingReads++;
    if (signalingBarrier != null) await signalingBarrier!.future;
    if (signalingFailure != null) throw signalingFailure!;
    return rtc.RTCSignalingState.RTCSignalingStateStable;
  }

  @override
  Future<void> setRemoteDescription(
      rtc.RTCSessionDescription description) async {
    remoteDescriptions.add(description);
  }

  @override
  Future<void> setLocalDescription(
      rtc.RTCSessionDescription description) async {
    localDescriptions.add(description);
  }

  @override
  Future<rtc.RTCSessionDescription> createAnswer(
      [Map<String, dynamic> constraints = const {}]) async {
    answerCalls++;
    if (!answerEntered.isCompleted) answerEntered.complete();
    if (answerBarrier != null) await answerBarrier!.future;
    return rtc.RTCSessionDescription('synthetic-answer-$name', 'answer');
  }

  @override
  Future<rtc.RTCDataChannel> createDataChannel(
      String label, rtc.RTCDataChannelInit init) async {
    final channel = _Channel();
    channels.add(channel);
    return channel;
  }

  @override
  Future<List<rtc.RTCRtpSender>> getSenders() async => [];
  @override
  Future<void> close() async {
    closeCalls++;
    trace.add('$name.close.begin');
    if (closeBarrier != null) await closeBarrier!.future;
    if (closeFailure != null) throw closeFailure!;
    trace.add('$name.close.end');
  }

  @override
  Future<void> dispose() async {
    disposeCalls++;
    trace.add('$name.dispose');
    for (final channel in channels) {
      if (!channel.states.isClosed) await channel.states.close();
    }
  }

  @override
  dynamic noSuchMethod(Invocation invocation) => null;
}

class _Signal extends SignalClient {
  _Signal()
      : super((uri, {options, headers, networkOptions}) async =>
            throw StateError(
                'Network connector must not run in lifecycle tests'));
  ConnectionState visibleState = ConnectionState.disconnected;
  int connectionEpoch = 0;
  int leaveCalls = 0;
  int cleanupCalls = 0;
  final answers = <rtc.RTCSessionDescription>[];
  @override
  ConnectionState get connectionState => visibleState;
  @override
  Future<void> connect(String uriString, String token,
      {required ConnectOptions connectOptions,
      required RoomOptions roomOptions,
      bool reconnect = false,
      models.ReconnectReason? reconnectReason}) async {
    connectionEpoch++;
    visibleState = ConnectionState.connected;
    events.emit(SignalJoinResponseEvent(
        response: protocol.JoinResponse(
      room: models.Room(sid: 'RM_$connectionEpoch', name: 'synthetic'),
      participant: models.ParticipantInfo(
          sid: 'PA_$connectionEpoch', identity: 'synthetic-local'),
      subscriberPrimary: true,
    )));
  }

  @override
  Future<void> sendLeave() async {
    leaveCalls++;
    // Transport boundary acknowledgement, delivered through the real dispatcher.
    serverLeave();
  }

  @override
  void sendAnswer(rtc.RTCSessionDescription answer) => answers.add(answer);

  void serverLeave() => events.emit(SignalLeaveEvent(
      request: protocol.LeaveRequest(
          action: protocol.LeaveRequest_Action.DISCONNECT,
          reason: models.DisconnectReason.CLIENT_INITIATED)));

  @override
  Future<void> cleanUp() async {
    cleanupCalls++;
    visibleState = ConnectionState.disconnected;
    await super.cleanUp();
  }
}

class _NativeStream extends Fake implements rtc.MediaStream {
  _NativeStream(this.trace);
  final List<String> trace;
  @override
  String get id => 'synthetic-local-stream';
  @override
  Future<void> dispose() async => trace.add('stream.dispose');
  @override
  dynamic noSuchMethod(Invocation invocation) => null;
}

class _NativeTrack extends Fake implements rtc.MediaStreamTrack {
  _NativeTrack(this.id, this.trace);
  @override
  final String id;
  final List<String> trace;
  final stopEntered = Completer<void>();
  Completer<void>? stopBarrier;
  Object? stopFailure;
  int stopCalls = 0;
  @override
  bool enabled = true;
  @override
  String get kind => 'video';
  @override
  Future<void> stop() async {
    stopCalls++;
    trace.add('$id.stop.begin');
    if (!stopEntered.isCompleted) stopEntered.complete();
    if (stopBarrier != null) await stopBarrier!.future;
    if (stopFailure != null) throw stopFailure!;
    trace.add('$id.stop.end');
  }

  @override
  dynamic noSuchMethod(Invocation invocation) => null;
}

class _Video extends LocalVideoTrack {
  _Video(_NativeStream stream, _NativeTrack track)
      : super(TrackSource.camera, stream, track, const CameraCaptureOptions());
  // Native stats are separately tested. Avoid scheduling real stats JNI.
  @override
  void startMonitor() {}
  @override
  Future<void> stopCapture() async {}
}

class _Fixture {
  _Fixture() {
    engine = Engine(
        connectOptions: const ConnectOptions(),
        roomOptions: const RoomOptions(),
        signalClient: signal,
        peerConnectionCreate: (configuration,
            [constraints = const <String, dynamic>{}]) async {
          final peer = _Peer('pc${peers.length}', trace);
          peers.add(peer);
          if (creationBarrier != null) await creationBarrier!.future;
          return peer;
        });
    room = Room(engine: engine);
  }
  final signal = _Signal();
  final trace = <String>[];
  final peers = <_Peer>[];
  final tracks = <_NativeTrack>[];
  Completer<void>? creationBarrier;
  late final Engine engine;
  late final Room room;

  Future<void> connect() async {
    final count = peers.length;
    Object? connectError, readyError;
    StackTrace? connectStack;
    final ready = room.events
        .waitFor<RoomConnectedEvent>(duration: const Duration(seconds: 3))
        .then<RoomConnectedEvent?>((event) => event,
            onError: (Object error, StackTrace stack) {
      readyError = error;
      return null;
    });
    final operation = room
        .connect(
            'wss://synthetic.invalid/no-network', 'synthetic-not-real-token')
        .then<void>((_) {}, onError: (Object error, StackTrace stack) {
      connectError = error;
      connectStack = stack;
    });
    await _until(() =>
        connectError != null ||
        readyError != null ||
        (peers.length == count + 2 &&
            peers.last.onConnectionState != null &&
            room.localParticipant != null));
    if (connectError != null) {
      Error.throwWithStackTrace(connectError!, connectStack!);
    }
    if (readyError != null) throw readyError!;
    await _turns();
    // The real Engine callback produces its real connected event.
    peers.last.onConnectionState!(
        rtc.RTCPeerConnectionState.RTCPeerConnectionStateConnected);
    await Future.wait<dynamic>([operation, ready]);
    if (connectError != null) {
      Error.throwWithStackTrace(connectError!, connectStack!);
    }
    if (readyError != null) throw readyError!;
  }

  Future<_NativeTrack> addTrack(String id) async {
    final native = _NativeTrack(id, trace);
    tracks.add(native);
    final track = _Video(_NativeStream(trace), native);
    await track.start();
    final participant = room.localParticipant!;
    participant.trackPublications[id] = LocalTrackPublication<LocalVideoTrack>(
        participant: participant,
        info: models.TrackInfo(sid: id, type: models.TrackType.VIDEO),
        track: track);
    return native;
  }

  Future<void> dispose() async {
    if (creationBarrier != null && !creationBarrier!.isCompleted)
      creationBarrier!.complete();
    for (final track in tracks) {
      final barrier = track.stopBarrier;
      if (barrier != null && !barrier.isCompleted) barrier.complete();
    }
    for (final peer in peers) {
      for (final barrier in [
        peer.closeBarrier,
        peer.signalingBarrier,
        peer.answerBarrier
      ]) {
        if (barrier != null && !barrier.isCompleted) barrier.complete();
      }
    }
    await _observeFailure(room.dispose());
  }
}

void main() {
  test(
      'real event callback failures are typed data with captured listener epoch',
      () async {
    final emitter = EventsEmitter<int>();
    Object epoch = Object();
    final originalEpoch = epoch;
    final listener = EventsListener<int>(emitter,
        synchronized: true, callbackEpoch: () => epoch);
    final failures = <EventCallbackError<int>>[];
    final errors = emitter.callbackErrors.listen(failures.add);
    final entered = Completer<void>(), finish = Completer<void>();
    final expected = StateError('synthetic async event failure');
    var nextProcessed = false;
    listener.on<int>((event) async {
      if (event == 1) {
        entered.complete();
        await finish.future;
        throw expected;
      }
      nextProcessed = true;
    });
    emitter.emit(1);
    await entered.future;
    epoch = Object();
    finish.complete();
    emitter.emit(2);
    await _until(() => failures.isNotEmpty && nextProcessed);
    expect(failures.single.error, same(expected));
    expect(failures.single.event, 1);
    expect(failures.single.listener, same(listener));
    expect(failures.single.epoch, same(originalEpoch));
    expect(emitter.lastCallbackError, same(failures.single));
    await listener.dispose();
    await errors.cancel();
    await emitter.dispose();
  });

  test(
      'actual local, server and dispose entrances wait for participant then PC',
      () async {
    final fixture = _Fixture();
    addTearDown(fixture.dispose);
    await fixture.connect();
    final track = (await fixture.addTrack('held-local'))
      ..stopBarrier = Completer<void>();
    final first = fixture.room.disconnect();
    final repeated = fixture.room.disconnect();
    final viaEngine = fixture.engine.disconnect();
    fixture.signal.serverLeave();
    final disposal = fixture.room.dispose();
    await track.stopEntered.future.timeout(const Duration(seconds: 3));
    expect(identical(first, repeated), isTrue);
    expect(fixture.peers.every((peer) => peer.closeCalls == 0), isTrue);
    var completed = false;
    repeated.then((_) => completed = true);
    await _turns();
    expect(completed, isFalse);
    track.stopBarrier!.complete();
    await Future.wait<dynamic>([first, repeated, viaEngine, disposal]);
    await fixture.room.dispose();
    expect(fixture.peers.map((peer) => peer.closeCalls), everyElement(1));
    expect(fixture.peers.map((peer) => peer.disposeCalls), everyElement(1));
    expect(fixture.trace.indexOf('held-local.stop.end'),
        lessThan(fixture.trace.indexOf('pc0.close.begin')));
  });

  test(
      'actual Room connect/disconnect twice clears each healthy connection epoch',
      () async {
    final fixture = _Fixture();
    addTearDown(fixture.dispose);
    await fixture.connect();
    await fixture.addTrack('epoch1');
    await fixture.room.disconnect();
    await fixture.connect();
    await fixture.addTrack('epoch2');
    await fixture.room.disconnect();
    expect(fixture.signal.connectionEpoch, 2);
    expect(fixture.peers.length, 4);
    expect(fixture.peers.map((peer) => peer.closeCalls), everyElement(1));
    expect(fixture.peers.map((peer) => peer.disposeCalls), everyElement(1));
    expect(fixture.trace.indexOf('epoch2.stop.end'),
        lessThan(fixture.trace.indexOf('pc2.close.begin')));
  });

  test(
      'actual owner callback error is visible and triggers participant-first shutdown',
      () async {
    final fixture = _Fixture();
    addTearDown(fixture.dispose);
    await fixture.connect();
    final track = (await fixture.addTrack('callback-local'))
      ..stopBarrier = Completer<void>();
    final expected =
        StateError('synthetic real SignalOffer callback rejection');
    fixture.peers.last.signalingFailure = expected;
    final errors = <RoomCallbackErrorEvent>[];
    final listener = fixture.room.createListener()
      ..on<RoomCallbackErrorEvent>(errors.add);
    addTearDown(listener.dispose);
    fixture.signal.events.emit(SignalOfferEvent(
        sd: rtc.RTCSessionDescription('synthetic-sdp', 'offer')));
    await _until(() => errors.isNotEmpty);
    await track.stopEntered.future.timeout(const Duration(seconds: 3));
    expect(errors.first.error, same(expected));
    expect(fixture.peers.every((peer) => peer.closeCalls == 0), isTrue);
    final result = fixture.room.disconnect();
    final observed = expectLater(result, throwsA(same(expected)));
    track.stopBarrier!.complete();
    await observed;
    expect(fixture.peers.map((peer) => peer.disposeCalls), everyElement(1));
  });

  test('failed real shutdown retains barrier and cannot reopen Room connection',
      () async {
    final fixture = _Fixture();
    addTearDown(fixture.dispose);
    await fixture.connect();
    await fixture.addTrack('failure-local');
    final expected = StateError('synthetic native close rejection');
    fixture.peers.first.closeFailure = expected;
    final first = fixture.room.disconnect();
    await expectLater(first, throwsA(same(expected)));
    expect(identical(first, fixture.room.disconnect()), isTrue);
    await expectLater(
        fixture.room.connect(
            'wss://synthetic.invalid/no-network', 'next-synthetic-token'),
        throwsA(anything));
    expect(fixture.signal.connectionEpoch, 1);
    expect(fixture.peers.length, 2);
    expect(fixture.peers.first.closeCalls, 1);
  });

  test('late old signal callback failure cannot close a new actual Room epoch',
      () async {
    final fixture = _Fixture();
    addTearDown(fixture.dispose);
    await fixture.connect();
    final oldPeer = fixture.peers.last
      ..signalingBarrier = Completer<void>()
      ..signalingFailure = StateError('late callback from old connection');
    fixture.signal.events.emit(SignalOfferEvent(
        sd: rtc.RTCSessionDescription('old-synthetic-sdp', 'offer')));
    await _until(() => oldPeer.signalingReads == 1);
    await fixture.room.disconnect();
    final errors = <RoomCallbackErrorEvent>[];
    final listener = fixture.room.createListener()
      ..on<RoomCallbackErrorEvent>(errors.add);
    addTearDown(listener.dispose);
    final secondConnect = fixture.connect();
    await _until(() => fixture.signal.connectionEpoch == 2);
    oldPeer.signalingBarrier!.complete();
    await secondConnect;
    await _turns();
    expect(fixture.room.connectionState, ConnectionState.connected);
    expect(fixture.peers.skip(2).every((peer) => peer.closeCalls == 0), isTrue);
    expect(errors, isEmpty);
    await fixture.room.disconnect();
  });

  test('real participant failure cannot skip second track or physical peers',
      () async {
    final fixture = _Fixture();
    addTearDown(fixture.dispose);
    await fixture.connect();
    final first = await fixture.addTrack('first-failed');
    final second = await fixture.addTrack('second-good');
    final failure = StateError('synthetic first native stop failure');
    first.stopFailure = failure;
    await expectLater(fixture.room.disconnect(), throwsA(same(failure)));
    expect(second.stopCalls, greaterThan(0));
    expect(fixture.peers.map((peer) => peer.closeCalls), everyElement(1));
    expect(fixture.peers.map((peer) => peer.disposeCalls), everyElement(1));
  });

  for (final stage in ['read-state', 'create-answer']) {
    test(
        'old Offer $stage completion cannot write SD or answer after real restart',
        () async {
      final fixture = _Fixture();
      addTearDown(fixture.dispose);
      await fixture.connect();
      final oldPeer = fixture.peers.last;
      final release = Completer<void>();
      if (stage == 'read-state') {
        oldPeer.signalingBarrier = release;
      } else {
        oldPeer.answerBarrier = release;
      }
      fixture.signal.events.emit(SignalOfferEvent(
          sd: rtc.RTCSessionDescription('old-epoch-synthetic-offer', 'offer')));
      if (stage == 'read-state') {
        await _until(() => oldPeer.signalingReads == 1);
      } else {
        await oldPeer.answerEntered.future.timeout(const Duration(seconds: 3));
      }

      Object? restartError;
      final restart = fixture.engine.restartConnection().then<void>((_) {},
          onError: (Object error, StackTrace stack) {
        restartError = error;
      });
      await _until(() =>
          restartError != null ||
          (fixture.peers.length == 4 &&
              fixture.peers.last.onConnectionState != null));
      if (restartError != null) throw restartError!;
      await _turns();
      fixture.peers.last.onConnectionState!(
          rtc.RTCPeerConnectionState.RTCPeerConnectionStateConnected);
      await restart;
      if (restartError != null) throw restartError!;
      // restartConnection installs a new real signal listener. The old
      // in-flight handler can now complete while the new transports are live.
      release.complete();
      await _turns(10);
      expect(fixture.signal.connectionEpoch, 2);
      expect(fixture.room.connectionState, ConnectionState.connected);
      expect(fixture.signal.answers, isEmpty);
      expect(oldPeer.localDescriptions, isEmpty);
      for (final peer in fixture.peers.skip(2)) {
        expect(peer.remoteDescriptions, isEmpty);
        expect(peer.localDescriptions, isEmpty);
        expect(peer.answerCalls, 0);
        expect(peer.closeCalls, 0);
      }
      await fixture.room.disconnect();
    });
  }

  test(
      'real pending peer creation is physically released before shutdown completes',
      () async {
    final fixture = _Fixture()..creationBarrier = Completer<void>();
    addTearDown(fixture.dispose);
    Object? connectError;
    var connectSucceeded = false;
    var roomConnected = 0;
    final listener = fixture.room.createListener()
      ..on<RoomConnectedEvent>((_) => roomConnected++);
    addTearDown(listener.dispose);
    final connecting = fixture.room
        .connect(
            'wss://synthetic.invalid/no-network', 'synthetic-held-creation')
        .then<void>((_) => connectSucceeded = true,
            onError: (Object error, StackTrace stack) {
      connectError = error;
    });
    await _until(() => fixture.peers.isNotEmpty);
    var stopped = false;
    final shutdown =
        fixture.room.disconnect().then<void>((_) => stopped = true);
    await _turns();
    expect(stopped, isFalse);
    expect(fixture.engine.publisher, isNull);
    expect(fixture.engine.subscriber, isNull);
    fixture.creationBarrier!.complete();
    print('[HeldPeerCreation] native-create-released');
    await shutdown.timeout(const Duration(seconds: 3));
    print('[HeldPeerCreation] shutdown-complete');
    await connecting.timeout(const Duration(seconds: 3));
    print('[HeldPeerCreation] connecting-complete');
    expect(connectSucceeded, isFalse);
    expect(connectError, isNotNull);
    expect(roomConnected, 0);
    expect(fixture.room.localParticipant, isNull);
    expect(fixture.engine.publisher, isNull);
    expect(fixture.engine.subscriber, isNull);
    expect(fixture.peers.map((peer) => peer.closeCalls), everyElement(1));
    expect(fixture.peers.map((peer) => peer.disposeCalls), everyElement(1));
  });
}
