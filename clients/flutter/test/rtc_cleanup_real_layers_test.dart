// Fault injection at actual LiveKit cleanup layers. Native objects are inert
// doubles; no device, network, camera or native method channel is used.
import 'dart:async';

import 'package:fake_async/fake_async.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_webrtc/flutter_webrtc.dart' as rtc;
import 'package:livekit_client/src/core/engine.dart';
import 'package:livekit_client/src/core/room.dart';
import 'package:livekit_client/src/core/signal_client.dart';
import 'package:livekit_client/src/core/transport.dart';
import 'package:livekit_client/src/events.dart';
import 'package:livekit_client/src/internal/events.dart';
import 'package:livekit_client/src/managers/event.dart';
import 'package:livekit_client/src/options.dart';
import 'package:livekit_client/src/participant/local.dart';
import 'package:livekit_client/src/participant/remote.dart';
import 'package:livekit_client/src/proto/livekit_models.pb.dart' as models;
import 'package:livekit_client/src/proto/livekit_rtc.pb.dart' as protocol;
import 'package:livekit_client/src/publication/local.dart';
import 'package:livekit_client/src/publication/remote.dart';
import 'package:livekit_client/src/support/cleanup_sequence.dart';
import 'package:livekit_client/src/support/disposable.dart';
import 'package:livekit_client/src/support/region_url_provider.dart';
import 'package:livekit_client/src/support/websocket.dart';
import 'package:livekit_client/src/track/local/video.dart';
import 'package:livekit_client/src/track/options.dart';
import 'package:livekit_client/src/track/remote/video.dart';
import 'package:livekit_client/src/types/other.dart';

class _NativeTrack extends Fake implements rtc.MediaStreamTrack {
  _NativeTrack(this.id, {this.stopFailure, this.failuresRemaining = 0});
  @override
  final String id;
  final Object? stopFailure;
  int failuresRemaining;
  int stopCalls = 0;
  @override
  bool enabled = true;
  @override
  String get kind => 'video';
  @override
  Future<void> stop() async {
    stopCalls++;
    if (failuresRemaining > 0) {
      failuresRemaining--;
      throw stopFailure!;
    }
  }

  @override
  dynamic noSuchMethod(Invocation invocation) => null;
}

class _NativeStream extends Fake implements rtc.MediaStream {
  int disposeCalls = 0;
  Object? disposeFailure;
  @override
  String get id => 'synthetic-stream';
  @override
  Future<void> dispose() async {
    disposeCalls++;
    if (disposeFailure != null) throw disposeFailure!;
  }

  @override
  dynamic noSuchMethod(Invocation invocation) => null;
}

class _RemoteVideo extends RemoteVideoTrack {
  _RemoteVideo(_NativeStream stream, _NativeTrack track)
      : super(TrackSource.camera, stream, track);
  // Stats scheduling is outside these ownership/cleanup tests.
  @override
  void startMonitor() {}
}

class _LocalVideo extends LocalVideoTrack {
  _LocalVideo(_NativeStream stream, _NativeTrack track, {this.captureFailure})
      : super(TrackSource.camera, stream, track, const CameraCaptureOptions());
  final Object? captureFailure;
  int stopCaptureCalls = 0;
  @override
  Future<void> stopCapture() async {
    stopCaptureCalls++;
    if (captureFailure != null) throw captureFailure!;
  }

  @override
  void startMonitor() {}
}

class _ClosedEngine extends Fake implements Engine {
  @override
  bool get isClosed => true;
}

class _ParticipantRoom extends Fake implements Room {
  @override
  final events = EventsEmitter<RoomEvent>();
  @override
  final Engine engine = _ClosedEngine();
  @override
  RoomOptions get roomOptions => const RoomOptions();
}

class _RemotePublication extends RemoteTrackPublication<RemoteVideoTrack> {
  _RemotePublication(
      RemoteParticipant participant, String sid, RemoteVideoTrack track,
      {this.disposeFailure})
      : super(
            participant: participant,
            info: models.TrackInfo(sid: sid, type: models.TrackType.VIDEO),
            track: track);
  final Object? disposeFailure;
  int disposeCalls = 0;
  @override
  Future<bool> dispose() {
    disposeCalls++;
    if (disposeFailure != null) return Future<bool>.error(disposeFailure!);
    return super.dispose();
  }
}

class _LocalPublication extends LocalTrackPublication<LocalVideoTrack> {
  _LocalPublication(
      LocalParticipant participant, String sid, LocalVideoTrack track,
      {this.disposeFailure})
      : super(
            participant: participant,
            info: models.TrackInfo(sid: sid, type: models.TrackType.VIDEO),
            track: track);
  final Object? disposeFailure;
  int disposeCalls = 0;
  @override
  Future<bool> dispose() {
    disposeCalls++;
    if (disposeFailure != null) return Future<bool>.error(disposeFailure!);
    return super.dispose();
  }
}

class _NativePeer extends Fake implements rtc.RTCPeerConnection {
  Completer<void>? closeBarrier;
  Object? closeFailure;
  int closeCalls = 0;
  int disposeCalls = 0;
  @override
  Future<List<rtc.RTCRtpSender>> getSenders() async => [];
  @override
  Future<void> close() async {
    closeCalls++;
    if (closeBarrier != null) await closeBarrier!.future;
    if (closeFailure != null) throw closeFailure!;
  }

  @override
  Future<void> dispose() async {
    disposeCalls++;
  }

  @override
  dynamic noSuchMethod(Invocation invocation) => null;
}

class _Signal extends SignalClient {
  _Signal()
      : super((Uri uri,
            {WebSocketEventHandlers? options,
            Map<String, String>? headers,
            NetworkOptions? networkOptions}) async {
          throw StateError(
              'network connector must never run in cleanup unit tests');
        });
  int cleanUpCalls = 0;
  Object? cleanUpFailure;
  bool reportConnected = false;
  @override
  ConnectionState get connectionState =>
      reportConnected ? ConnectionState.connected : super.connectionState;
  @override
  Future<void> cleanUp() async {
    cleanUpCalls++;
    await super.cleanUp();
    if (cleanUpFailure != null) throw cleanUpFailure!;
  }
}

class _AlreadyClosedEngine extends Engine {
  _AlreadyClosedEngine(_Signal signal)
      : super(
            connectOptions: const ConnectOptions(),
            roomOptions: const RoomOptions(),
            signalClient: signal);
  @override
  bool get isClosed => true;
}

class _RestartSignal extends _Signal {
  int connections = 0;
  @override
  Future<void> connect(String uriString, String token,
      {required ConnectOptions connectOptions,
      required RoomOptions roomOptions,
      bool reconnect = false,
      models.ReconnectReason? reconnectReason}) async {
    connections++;
    reportConnected = true;
    // Only the wire reply is synthetic. The Engine's real join dispatcher,
    // peer creation, connection waits and connection epoch remain exercised.
    events.emit(SignalJoinResponseEvent(
        response: protocol.JoinResponse(subscriberPrimary: true)));
  }

  @override
  Future<void> cleanUp() async {
    await super.cleanUp();
    reportConnected = false;
  }
}

class _RestartChannel extends Fake implements rtc.RTCDataChannel {
  final states = StreamController<rtc.RTCDataChannelState>.broadcast();
  @override
  Stream<rtc.RTCDataChannelState> get stateChangeStream => states.stream;
  @override
  int? bufferedAmountLowThreshold = 0;
  @override
  dynamic noSuchMethod(Invocation invocation) => null;
}

class _RestartPeer extends _NativePeer {
  final channels = <_RestartChannel>[];
  @override
  void Function(rtc.RTCPeerConnectionState)? onConnectionState;
  @override
  Future<rtc.RTCDataChannel> createDataChannel(
      String label, rtc.RTCDataChannelInit init) async {
    final channel = _RestartChannel();
    channels.add(channel);
    return channel;
  }

  @override
  Future<void> dispose() async {
    await super.dispose();
    for (final channel in channels) {
      await channel.states.close();
    }
  }
}

class _SuccessfulRestartEngine extends Engine {
  _SuccessfulRestartEngine(_Signal signal, PeerConnectionCreate createPeer)
      : super(
            connectOptions: const ConnectOptions(),
            roomOptions: const RoomOptions(),
            signalClient: signal,
            peerConnectionCreate: createPeer);
  final calls = <String>[];

  // Record entry but retain the actual connection epoch/join/peer waits.
  @override
  Future<void> connect(
    String url,
    String token, {
    ConnectOptions? connectOptions,
    RoomOptions? roomOptions,
    FastConnectOptions? fastConnectOptions,
    RegionUrlProvider? regionUrlProvider,
  }) async {
    calls.add('connect');
    await super.connect(url, token,
        connectOptions: connectOptions,
        roomOptions: roomOptions,
        fastConnectOptions: fastConnectOptions,
        regionUrlProvider: regionUrlProvider);
  }

  @override
  Future<void> ensurePublisherConnected() async {
    calls.add('ensurePublisherConnected');
  }
}

class _FaultSocket extends LiveKitWebSocket {
  final sent = <List<int>>[];
  Object? disposeFailure;
  int disposeCalls = 0;
  @override
  void send(List<int> data) => sent.add(List<int>.of(data));
  @override
  Future<bool> dispose() {
    disposeCalls++;
    if (disposeFailure != null) return Future<bool>.error(disposeFailure!);
    return super.dispose();
  }
}

class _SignalWithVisibleState extends SignalClient {
  _SignalWithVisibleState(WebSocketConnector connector) : super(connector);
  // Control only the public state view to enqueue real requests; the queue,
  // protobuf processing, timer creation and cleanup methods are production code.
  ConnectionState? visibleState;
  @override
  ConnectionState get connectionState => visibleState ?? super.connectionState;
}

Future<Transport> _transport(_NativePeer peer) => Transport.create(
    (configuration, [constraints = const <String, dynamic>{}]) async => peer,
    connectOptions: const ConnectOptions());

Future<void> _ignoreFailure(Future<dynamic> operation) async {
  try {
    await operation;
  } catch (_) {}
}

void main() {
  test('synchronous dispose callback reentry shares actual completion',
      () async {
    final owner = _ReentrantDisposable();
    final first = owner.dispose();
    final reentered = owner.reentered!;
    var completed = false;
    reentered.then((_) => completed = true);
    await Future<void>.delayed(Duration.zero);
    expect(identical(first, reentered), isTrue);
    expect(completed, isFalse);
    owner.native.complete();
    expect(await first, isTrue);
    expect(await reentered, isTrue);
    expect(owner.cleanupCalls, 1);
  });

  test(
      'real remote batch attempts publication disposal and second track after first stop fails',
      () async {
    final room = _ParticipantRoom();
    final created = await RemoteParticipant.createFromInfo(
        room: room,
        info: models.ParticipantInfo(sid: 'PA_remote', identity: 'remote'));
    final participant = created.participant;
    final firstError =
        StateError('synthetic native first-track stop rejection');
    final firstNative =
        _NativeTrack('remote-1', stopFailure: firstError, failuresRemaining: 1);
    final secondNative = _NativeTrack('remote-2');
    final first = _RemoteVideo(_NativeStream(), firstNative);
    final second = _RemoteVideo(_NativeStream(), secondNative);
    await first.start();
    await second.start();
    final pub1 = _RemotePublication(participant, 'TR_remote_1', first);
    final pub2 = _RemotePublication(participant, 'TR_remote_2', second);
    participant.trackPublications.addAll({pub1.sid: pub1, pub2.sid: pub2});
    addTearDown(() async {
      await _ignoreFailure(first.dispose());
      await _ignoreFailure(second.dispose());
      await _ignoreFailure(participant.dispose());
      await room.events.dispose();
    });

    await expectLater(participant.removeAllPublishedTracks(notify: false),
        throwsA(same(firstError)));
    expect(pub1.disposeCalls, greaterThanOrEqualTo(1));
    expect(pub2.disposeCalls, greaterThanOrEqualTo(1));
    expect(secondNative.stopCalls, greaterThanOrEqualTo(1));
    expect(second.isDisposed, isTrue);
  });

  test('real remote ownership retains a publication whose disposal rejects',
      () async {
    final room = _ParticipantRoom();
    final participant = (await RemoteParticipant.createFromInfo(
            room: room,
            info: models.ParticipantInfo(sid: 'PA_retain', identity: 'retain')))
        .participant;
    final track = _RemoteVideo(_NativeStream(), _NativeTrack('remote-retain'));
    final failure = StateError('synthetic publication dispose rejection');
    final publication = _RemotePublication(participant, 'TR_retain', track,
        disposeFailure: failure);
    participant.trackPublications[publication.sid] = publication;
    addTearDown(() async {
      await _ignoreFailure(track.dispose());
      await _ignoreFailure(participant.dispose());
      await room.events.dispose();
    });

    await expectLater(
        participant.removePublishedTrack(publication.sid, notify: false),
        throwsA(same(failure)));
    expect(participant.trackPublications[publication.sid], same(publication));
  });

  test('real local batch keeps cleaning after a native stop failure', () async {
    final room = _ParticipantRoom();
    final participant = await LocalParticipant.createFromInfo(
        room: room,
        info: models.ParticipantInfo(sid: 'PA_local', identity: 'local'));
    final failure = StateError('synthetic local native stop rejection');
    final native1 =
        _NativeTrack('local-1', stopFailure: failure, failuresRemaining: 1);
    final native2 = _NativeTrack('local-2');
    final first = _LocalVideo(_NativeStream(), native1);
    final second = _LocalVideo(_NativeStream(), native2);
    await first.start();
    await second.start();
    final pub1 = _LocalPublication(participant, 'TR_local_1', first);
    final pub2 = _LocalPublication(participant, 'TR_local_2', second);
    participant.trackPublications.addAll({pub1.sid: pub1, pub2.sid: pub2});
    addTearDown(() async {
      await _ignoreFailure(first.dispose());
      await _ignoreFailure(second.dispose());
      await _ignoreFailure(participant.dispose());
      await room.events.dispose();
    });

    await expectLater(
        participant.unpublishAllTracks(notify: false), throwsA(same(failure)));
    expect(pub1.disposeCalls, greaterThanOrEqualTo(1));
    expect(pub2.disposeCalls, greaterThanOrEqualTo(1));
    expect(native2.stopCalls, greaterThanOrEqualTo(1));
    expect(second.isDisposed, isTrue);
  });

  test(
      'real LocalTrack capture failure cannot skip native track or stream disposal',
      () async {
    final failure = StateError('synthetic stopCapture rejection');
    final stream = _NativeStream()
      ..disposeFailure = StateError('later stream rejection');
    final native = _NativeTrack('local-capture');
    final track = _LocalVideo(stream, native, captureFailure: failure);
    await track.start();
    addTearDown(() => _ignoreFailure(track.dispose()));

    await expectLater(track.stop(), throwsA(same(failure)));
    expect(track.stopCaptureCalls, greaterThanOrEqualTo(1));
    expect(native.stopCalls, greaterThanOrEqualTo(1));
    expect(stream.disposeCalls, greaterThanOrEqualTo(1));
  });

  test(
      'real Engine concurrent cleanup awaits publisher native completion and retains ownership',
      () async {
    final signal = _Signal();
    final engine = Engine(
        connectOptions: const ConnectOptions(),
        roomOptions: const RoomOptions(),
        signalClient: signal);
    final publisherPeer = _NativePeer()..closeBarrier = Completer<void>();
    final subscriberPeer = _NativePeer();
    final publisher = await _transport(publisherPeer);
    final subscriber = await _transport(subscriberPeer);
    engine.publisher = publisher;
    engine.subscriber = subscriber;
    addTearDown(() async {
      if (!publisherPeer.closeBarrier!.isCompleted)
        publisherPeer.closeBarrier!.complete();
      await _ignoreFailure(engine.dispose());
    });

    final first = engine.cleanUp();
    final repeated = engine.cleanUp();
    var repeatedCompleted = false;
    repeated.then((_) => repeatedCompleted = true);
    await Future<void>.delayed(Duration.zero);
    expect(identical(first, repeated), isTrue);
    expect(repeatedCompleted, isFalse);
    expect(engine.publisher, same(publisher));
    expect(engine.subscriber, same(subscriber));
    publisherPeer.closeBarrier!.complete();
    await Future.wait([first, repeated]);
    expect(publisherPeer.closeCalls, 1);
    expect(publisherPeer.disposeCalls, 1);
    expect(subscriberPeer.closeCalls, 1);
    expect(signal.cleanUpCalls, 1);
    expect(engine.publisher, isNull);
    expect(engine.subscriber, isNull);
  });

  test(
      'real Engine publisher failure still disposes subscriber and signal and reports first failure',
      () async {
    final firstError = StateError('synthetic publisher native close failure');
    final signal = _Signal()
      ..cleanUpFailure = StateError('later signal failure');
    final engine = Engine(
        connectOptions: const ConnectOptions(),
        roomOptions: const RoomOptions(),
        signalClient: signal);
    final publisherPeer = _NativePeer()..closeFailure = firstError;
    final subscriberPeer = _NativePeer();
    final publisher = await _transport(publisherPeer);
    engine.publisher = publisher;
    engine.subscriber = await _transport(subscriberPeer);
    addTearDown(() => _ignoreFailure(engine.dispose()));

    await expectLater(engine.cleanUp(), throwsA(same(firstError)));
    expect(publisherPeer.disposeCalls, 1);
    expect(subscriberPeer.closeCalls, 1);
    expect(subscriberPeer.disposeCalls, 1);
    expect(signal.cleanUpCalls, 1);
    expect(engine.publisher, same(publisher));
    expect(engine.subscriber, isNull);
  });

  test(
      'real Room repeated disconnect on a closed engine still awaits native cleanup',
      () async {
    final signal = _Signal();
    final engine = _AlreadyClosedEngine(signal);
    final publisherPeer = _NativePeer()..closeBarrier = Completer<void>();
    final publisher = await _transport(publisherPeer);
    engine.publisher = publisher;
    final room = Room(engine: engine);
    addTearDown(() async {
      if (!publisherPeer.closeBarrier!.isCompleted)
        publisherPeer.closeBarrier!.complete();
      await _ignoreFailure(room.dispose());
    });

    final first = room.disconnect();
    final repeated = room.disconnect();
    var completed = false;
    repeated.then((_) => completed = true);
    await Future<void>.delayed(Duration.zero);
    expect(completed, isFalse);
    expect(engine.publisher, same(publisher));
    publisherPeer.closeBarrier!.complete();
    await Future.wait([first, repeated]);
    expect(publisherPeer.closeCalls, 1);
    expect(publisherPeer.disposeCalls, 1);
    expect(engine.publisher, isNull);
  });

  test(
      'real Engine restart failure still attempts both peers and signal cleanup',
      () async {
    final failure = StateError('synthetic restart publisher close rejection');
    final signal = _Signal()..reportConnected = true;
    final engine = Engine(
        connectOptions: const ConnectOptions(),
        roomOptions: const RoomOptions(),
        signalClient: signal);
    final publisherPeer = _NativePeer()..closeFailure = failure;
    final subscriberPeer = _NativePeer();
    final publisher = await _transport(publisherPeer);
    engine.publisher = publisher;
    engine.subscriber = await _transport(subscriberPeer);
    addTearDown(() => _ignoreFailure(engine.dispose()));

    await expectLater(engine.restartConnection(), throwsA(same(failure)));
    expect(signal.cleanUpCalls, 1);
    expect(publisherPeer.disposeCalls, 1);
    expect(subscriberPeer.closeCalls, 1);
    expect(subscriberPeer.disposeCalls, 1);
    expect(engine.publisher, same(publisher));
    expect(engine.subscriber, isNull);
  });

  test(
      'successful real Engine restart preserves prior publishing and ensures publisher connection',
      () async {
    final signal = _RestartSignal();
    final peers = <_RestartPeer>[];
    final engine = _SuccessfulRestartEngine(signal, (configuration,
        [constraints = const <String, dynamic>{}]) async {
      final peer = _RestartPeer();
      peers.add(peer);
      return peer;
    });
    addTearDown(() => _ignoreFailure(engine.dispose()));

    Future<void> completeConnection(Future<void> connection, int count) async {
      Object? error;
      StackTrace? stack;
      final observed = connection.then<void>((_) {},
          onError: (Object failure, StackTrace trace) {
        error = failure;
        stack = trace;
      });
      final deadline = DateTime.now().add(const Duration(seconds: 3));
      while (error == null &&
          (peers.length != count || peers.last.onConnectionState == null)) {
        if (DateTime.now().isAfter(deadline))
          fail('Peer creation did not finish');
        await Future<void>.delayed(Duration.zero);
      }
      if (error != null) Error.throwWithStackTrace(error!, stack!);
      // Let the real join event install the connected wait before delivering
      // the native peer's public connection callback.
      for (var turn = 0; turn < 3; turn++) {
        await Future<void>.delayed(Duration.zero);
      }
      peers.last.onConnectionState!(
          rtc.RTCPeerConnectionState.RTCPeerConnectionStateConnected);
      await observed.timeout(const Duration(seconds: 3));
      if (error != null) Error.throwWithStackTrace(error!, stack!);
    }

    await completeConnection(
        engine.connect('wss://synthetic.invalid/no-network',
            'synthetic-not-a-real-credential'),
        2);
    final publisherPeer = peers[0], subscriberPeer = peers[1];
    engine.calls.clear();

    // Exercise the real public operation that sets _hasPublished. Its queued
    // negotiation debounce is cancelled by the real Transport disposal below.
    await engine.negotiate();
    await completeConnection(engine.restartConnection(), 4);
    expect(publisherPeer.closeCalls, 1);
    expect(publisherPeer.disposeCalls, 1);
    expect(subscriberPeer.closeCalls, 1);
    expect(subscriberPeer.disposeCalls, 1);
    expect(signal.connections, 2);
    expect(engine.calls, ['connect', 'ensurePublisherConnected']);
  });

  test(
      'real SignalClient socket rejection still clears queued requests and live ping timers',
      () async {
    late FakeAsync simulation;
    late _SignalWithVisibleState finalSignal;
    late _FaultSocket finalSocket;
    Object? disposalError;
    var disposalCompleted = false;
    fakeAsync((clock) {
      simulation = clock;
      final socket = _FaultSocket();
      finalSocket = socket;
      WebSocketEventHandlers? handlers;
      final signal = _SignalWithVisibleState((uri,
          {options, headers, networkOptions}) async {
        handlers = options;
        return socket;
      });
      finalSignal = signal;
      var connected = false;
      Object? connectError;
      signal
          .connect('wss://synthetic.invalid', 'synthetic-not-a-real-credential',
              connectOptions: const ConnectOptions(),
              roomOptions: const RoomOptions())
          .then<void>((_) {
        connected = true;
      }, onError: (Object error) {
        connectError = error;
      });
      clock.flushMicrotasks();
      expect(connectError, isNull);
      expect(connected, isTrue);
      expect(handlers, isNotNull);

      // Verify the real enqueue/flush route once, then leave a second request
      // pending so post-failure queue clearing has an observable consequence.
      signal.visibleState = ConnectionState.reconnecting;
      signal.sendUpdateLocalMetadata(
          protocol.UpdateParticipantMetadata(name: 'baseline'));
      expect(socket.sent, isEmpty);
      signal.visibleState = ConnectionState.connected;
      signal.sendQueuedRequests();
      expect(socket.sent.length, 1);
      signal.visibleState = ConnectionState.reconnecting;
      signal.sendUpdateLocalMetadata(
          protocol.UpdateParticipantMetadata(name: 'must-be-cleared'));
      signal.visibleState = null;

      handlers!.onData!(protocol.SignalResponse(
              join: protocol.JoinResponse(
                  participant:
                      models.ParticipantInfo(sid: 'PA_ping', identity: 'ping'),
                  pingInterval: 5,
                  pingTimeout: 20))
          .writeToBuffer());
      clock.flushMicrotasks();
      expect(clock.periodicTimerCount, 1);
      expect(clock.nonPeriodicTimerCount, 1);
      clock.elapse(const Duration(seconds: 5));
      expect(socket.sent.length, 3); // baseline plus real ping and pingReq

      final failure = StateError('synthetic websocket dispose rejection');
      socket.disposeFailure = failure;
      Object? cleanupError;
      final first = signal.cleanUp();
      final repeated = signal.cleanUp();
      first.then<void>((_) {
        fail('cleanup must report native rejection');
      }, onError: (Object error) {
        cleanupError = error;
      });
      expect(identical(first, repeated), isTrue);
      clock.flushMicrotasks();
      expect(cleanupError, same(failure));
      expect(socket.disposeCalls, 1);
      expect(clock.periodicTimerCount, 0);
      expect(clock.nonPeriodicTimerCount, 0);

      // The failed socket remains owned. If the queue were left behind this
      // forced-connected flush would deliver the pending request to that socket.
      signal.visibleState = ConnectionState.connected;
      signal.sendQueuedRequests();
      expect(socket.sent.length, 3);
      clock.elapse(const Duration(minutes: 1));
      expect(socket.sent.length, 3);

      signal.dispose().then<void>((_) {
        fail('owner must retain cleanup rejection');
      }, onError: (Object error) {
        disposalError = error;
        disposalCompleted = true;
      });
      clock.flushMicrotasks();
      clock.elapse(Duration.zero);
      clock.flushMicrotasks();
      expect(clock.periodicTimerCount, 0);
      expect(clock.nonPeriodicTimerCount, 0);
    });
    // StreamSubscription.cancel may return a shared completed Dart future
    // created outside the fake-clock zone. Drain actual event turns as well as
    // the clock's microtasks; no native operation is timed out or marked done.
    for (var turn = 0; turn < 4 && !disposalCompleted; turn++) {
      await Future<void>.delayed(Duration.zero);
      simulation.flushMicrotasks();
      simulation.elapse(Duration.zero);
    }
    expect(disposalCompleted, isTrue);
    expect(disposalError, same(finalSocket.disposeFailure));
    expect(finalSignal.events.isDisposed, isTrue);
    expect(finalSocket.disposeCalls, 1);
    expect(simulation.periodicTimerCount, 0);
    expect(simulation.nonPeriodicTimerCount, 0);
  });
}

class _ReentrantDisposable extends Disposable with CleanupFailureReporting {
  _ReentrantDisposable() {
    onDispose(() => recordCleanup(() async {
          cleanupCalls++;
          reentered = dispose();
          await native.future;
        }));
  }
  final native = Completer<void>();
  Future<bool>? reentered;
  int cleanupCalls = 0;
  @override
  Future<bool> dispose() => reportCleanupResult(super.dispose);
}
