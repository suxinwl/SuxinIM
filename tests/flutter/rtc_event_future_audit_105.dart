// Historical 105 defect probes: these assert that the current event callback
// failure reaches the zone. They are evidence, not the desired 106 contract.
// Uses the real EventsEmitter, Engine and Transport with inert native doubles.
import 'dart:async';

import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_webrtc/flutter_webrtc.dart' as rtc;
import 'package:livekit_client/src/core/engine.dart';
import 'package:livekit_client/src/core/signal_client.dart';
import 'package:livekit_client/src/core/transport.dart';
import 'package:livekit_client/src/internal/events.dart';
import 'package:livekit_client/src/managers/event.dart';
import 'package:livekit_client/src/options.dart';
import 'package:livekit_client/src/proto/livekit_models.pb.dart' as models;
import 'package:livekit_client/src/proto/livekit_rtc.pb.dart' as protocol;
import 'package:livekit_client/src/support/websocket.dart';

class _Peer extends Fake implements rtc.RTCPeerConnection {
  _Peer(this.failure);
  final Object failure;
  int closeCalls = 0;
  int disposeCalls = 0;
  @override
  Future<List<rtc.RTCRtpSender>> getSenders() async => [];
  @override
  Future<void> close() async {
    closeCalls++;
    throw failure;
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
          throw StateError('No network in event-future defect probes');
        });
}

Future<void> _drain() async {
  for (var turn = 0; turn < 8; turn++) {
    await Future<void>.delayed(Duration.zero);
  }
}

void main() {
  test('105 real synchronized event listener drops failing callback Future',
      () async {
    final done = Completer<void>();
    final failures = <Object>[];
    final failure = StateError('synthetic callback rejection');
    runZonedGuarded(() async {
      final events = EventsEmitter<int>();
      final listener = EventsListener<int>(events, synchronized: true);
      listener.on<int>((event) async {
        await Future<void>.delayed(Duration.zero);
        throw failure;
      });
      events.emit(1);
      await _drain();
      await listener.dispose();
      await events.dispose();
      done.complete();
    }, (Object error, StackTrace stack) {
      failures.add(error);
    });
    await done.future;
    expect(failures, [same(failure)]);
  });

  test('105 real SignalLeave cleanup rejection escapes as unobserved Future',
      () async {
    final done = Completer<void>();
    final failures = <Object>[];
    final failure = StateError('synthetic native close deadline');
    var disconnected = 0;
    late _Peer peer;
    runZonedGuarded(() async {
      final signal = _Signal();
      final engine = Engine(
        connectOptions: const ConnectOptions(),
        roomOptions: const RoomOptions(),
        signalClient: signal,
      );
      peer = _Peer(failure);
      engine.publisher = await Transport.create(
        (configuration, [constraints = const <String, dynamic>{}]) async => peer,
        connectOptions: const ConnectOptions(),
      );
      final listener = engine.createListener();
      listener.on<EngineDisconnectedEvent>((_) => disconnected++);
      signal.events.emit(SignalLeaveEvent(
        request: protocol.LeaveRequest(
          action: protocol.LeaveRequest_Action.DISCONNECT,
          reason: models.DisconnectReason.CLIENT_INITIATED,
        ),
      ));
      await _drain();
      await listener.dispose();
      // Observe the same stored physical error for final disposal as a caller;
      // the independent SignalLeave callback has already escaped to the zone.
      try {
        await engine.dispose();
      } catch (error) {
        expect(error, same(failure));
      }
      done.complete();
    }, (Object error, StackTrace stack) {
      failures.add(error);
    });
    await done.future;
    expect(failures, [same(failure)]);
    expect(disconnected, 0);
    expect(peer.closeCalls, 1);
    expect(peer.disposeCalls, 1);
  });
}
