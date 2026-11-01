// Actual SignalClient connection/cleanup epochs; connector/socket boundaries are
// inert doubles. No reflection, copied lifecycle algorithm, device or network.
import 'dart:async';
import 'dart:io' as io;

import 'package:flutter_test/flutter_test.dart';
import 'package:livekit_client/src/core/signal_client.dart';
import 'package:livekit_client/src/internal/events.dart';
import 'package:livekit_client/src/options.dart';
import 'package:livekit_client/src/proto/livekit_rtc.pb.dart' as protocol;
import 'package:livekit_client/src/support/websocket.dart';
import 'package:livekit_client/src/types/other.dart';

class _Socket extends LiveKitWebSocket {
  _Socket() {
    onDispose(() => recordCleanup(() async {
          physicalDisposals++;
          if (disposeBarrier != null) await disposeBarrier!.future;
          if (disposeFailure != null) throw disposeFailure!;
        }));
  }
  final sent = <List<int>>[];
  int physicalDisposals = 0;
  Completer<void>? disposeBarrier;
  Object? disposeFailure;
  @override
  void send(List<int> data) => sent.add(data);
}

class _Attempt {
  _Attempt(this.handlers);
  final WebSocketEventHandlers? handlers;
  final socket = Completer<LiveKitWebSocket>();
}

class _Fixture {
  _Fixture() {
    client = SignalClient((uri, {options, headers, networkOptions}) {
      final attempt = _Attempt(options);
      attempts.add(attempt);
      return attempt.socket.future;
    });
  }
  late final SignalClient client;
  final attempts = <_Attempt>[];
  final sockets = <_Socket>[];
  Object? expectedCleanupFailure;
  Future<void> connect(String generation) => client.connect(
        'wss://127.0.0.1:1/no-network',
        'synthetic-not-token-$generation',
        connectOptions: const ConnectOptions(),
        roomOptions: const RoomOptions(),
      );

  _Socket resolve(int index, {Object? disposeFailure}) {
    final socket = _Socket()..disposeFailure = disposeFailure;
    sockets.add(socket);
    attempts[index].socket.complete(socket);
    return socket;
  }

  Future<void> dispose() async {
    for (var index = 0; index < attempts.length; index++) {
      if (!attempts[index].socket.isCompleted) resolve(index);
    }
    for (final socket in sockets) {
      if (socket.disposeBarrier != null && !socket.disposeBarrier!.isCompleted)
        socket.disposeBarrier!.complete();
    }
    Object? firstError;
    try {
      await client.dispose();
    } catch (error) {
      firstError ??= error;
    }
    for (final socket in sockets) {
      try {
        await socket.dispose();
      } catch (error) {
        firstError ??= error;
      }
    }
    if (firstError != null && !identical(firstError, expectedCleanupFailure))
      throw firstError!;
  }
}

Future<void> _until(bool Function() condition) async {
  final deadline = DateTime.now().add(const Duration(seconds: 3));
  while (!condition()) {
    if (DateTime.now().isAfter(deadline))
      fail('Signal fixture did not advance');
    await Future<void>.delayed(Duration.zero);
  }
}

Future<void> _turns() async {
  for (var turn = 0; turn < 8; turn++) {
    await Future<void>.delayed(Duration.zero);
  }
}

void main() {
  test(
      'late old connector result is physically disposed and never replaces current socket',
      () async {
    final fixture = _Fixture();
    addTearDown(fixture.dispose);
    Object? oldError;
    var oldSucceeded = false;
    final old = fixture.connect('old').then<void>((_) {
      oldSucceeded = true;
    }, onError: (Object error, StackTrace stack) {
      oldError = error;
    });
    await _until(() => fixture.attempts.length == 1);
    await fixture.client.cleanUp();
    final current = fixture.connect('new');
    await _until(() => fixture.attempts.length == 2);
    final activeSocket = fixture.resolve(1);
    await current;
    final staleSocket = fixture.resolve(0);
    await old;
    expect(oldSucceeded, isFalse);
    expect(oldError, isNotNull);
    expect(staleSocket.physicalDisposals, 1);
    expect(activeSocket.physicalDisposals, 0);
    expect(fixture.client.connectionState, ConnectionState.connected);
    fixture.client.sendUpdateLocalMetadata(
        protocol.UpdateParticipantMetadata(name: 'current-only'));
    expect(activeSocket.sent.length, 1);
    expect(staleSocket.sent, isEmpty);
  });

  test(
      'late callbacks from old socket cannot disconnect or deliver into current epoch',
      () async {
    final fixture = _Fixture();
    addTearDown(fixture.dispose);
    final first = fixture.connect('first');
    await _until(() => fixture.attempts.length == 1);
    final oldSocket = fixture.resolve(0);
    await first;
    await fixture.client.cleanUp();
    final second = fixture.connect('second');
    await _until(() => fixture.attempts.length == 2);
    final currentSocket = fixture.resolve(1);
    await second;
    var disconnected = 0;
    var joins = 0;
    final listener = fixture.client.createListener()
      ..on<SignalDisconnectedEvent>((_) => disconnected++)
      ..on<SignalJoinResponseEvent>((_) => joins++);
    addTearDown(listener.dispose);
    final oldHandlers = fixture.attempts.first.handlers!;
    oldHandlers.onDispose?.call();
    oldHandlers.onError?.call(StateError('stale previous socket failure'));
    oldHandlers.onData?.call(
        protocol.SignalResponse(join: protocol.JoinResponse()).writeToBuffer());
    await _turns();
    expect(disconnected, 0);
    expect(joins, 0);
    expect(fixture.client.connectionState, ConnectionState.connected);
    expect(oldSocket.physicalDisposals, 1);
    expect(currentSocket.physicalDisposals, 0);
  });

  test(
      'unexpected current error disconnects only its epoch once and observes cleanup',
      () async {
    final fixture = _Fixture();
    addTearDown(fixture.dispose);
    final operation = fixture.connect('unexpected-error');
    await _until(() => fixture.attempts.length == 1);
    final socket = fixture.resolve(0);
    await operation;
    final disconnected = <SignalDisconnectedEvent>[];
    final listener = fixture.client.createListener()
      ..on<SignalDisconnectedEvent>(disconnected.add);
    addTearDown(listener.dispose);
    final handlers = fixture.attempts.single.handlers!;
    handlers.onError?.call(StateError('synthetic current socket failure'));
    handlers.onError?.call(StateError('repeated same socket error'));
    handlers.onDispose?.call();
    await _until(() => disconnected.isNotEmpty);
    await _turns();
    expect(disconnected.length, 1);
    expect(fixture.client.connectionState, ConnectionState.disconnected);
    expect(socket.physicalDisposals, 1);
  });

  test(
      'failed late old release preserves current socket but bars another connect',
      () async {
    final fixture = _Fixture();
    final failure = StateError('synthetic retired socket physical failure');
    fixture.expectedCleanupFailure = failure;
    addTearDown(fixture.dispose);
    Object? oldError;
    final old = fixture.connect('old-failed-release').then<void>((_) {
      fail('stale connector cannot succeed');
    }, onError: (Object error, StackTrace stack) {
      oldError = error;
    });
    await _until(() => fixture.attempts.length == 1);
    await fixture.client.cleanUp();
    final current = fixture.connect('current-after-old');
    await _until(() => fixture.attempts.length == 2);
    final active = fixture.resolve(1);
    await current;
    final retired = fixture.resolve(0, disposeFailure: failure);
    await old;
    expect(oldError, same(failure));
    expect(retired.physicalDisposals, 1);
    expect(active.physicalDisposals, 0);
    expect(fixture.client.connectionState, ConnectionState.connected);
    fixture.client.sendUpdateLocalMetadata(
        protocol.UpdateParticipantMetadata(name: 'still-current'));
    expect(active.sent.length, 1);
    await expectLater(
        fixture.connect('must-not-reopen'), throwsA(same(failure)));
    expect(fixture.attempts.length, 2);
    await expectLater(fixture.client.cleanUp(), throwsA(same(failure)));
    expect(active.physicalDisposals, 1);
    expect(retired.physicalDisposals, 1);
  });

  test(
      'eight pending owned epochs bound ninth before mutation and recover after release',
      () async {
    final fixture = _Fixture();
    addTearDown(fixture.dispose);
    final futures = <Future<void>>[];
    final succeeded = List<bool>.filled(8, false);
    final failures = <int, Object>{};
    for (var index = 0; index < 8; index++) {
      final captured = index;
      futures.add(fixture.connect('pending-$index').then<void>((_) {
        succeeded[captured] = true;
      }, onError: (Object error, StackTrace stack) {
        failures[captured] = error;
      }));
      await _until(() => fixture.attempts.length == index + 1);
    }
    await expectLater(fixture.connect('ninth-rejected'), throwsA(anything));
    expect(fixture.attempts.length, 8);
    // Resolve the newest first: rejected capacity must not have invalidated it.
    final active = fixture.resolve(7);
    await futures[7];
    expect(succeeded[7], isTrue);
    for (var index = 0; index < 7; index++) {
      fixture.resolve(index);
    }
    await Future.wait(futures);
    expect(failures.length, 7);
    expect(fixture.client.connectionState, ConnectionState.connected);
    expect(active.physicalDisposals, 0);
    await fixture.client.cleanUp();
    expect(fixture.sockets.map((socket) => socket.physicalDisposals),
        everyElement(1));
    final healthy = fixture.connect('capacity-recovered');
    await _until(() => fixture.attempts.length == 9);
    final latest = fixture.resolve(8);
    await healthy;
    fixture.client.sendUpdateLocalMetadata(
        protocol.UpdateParticipantMetadata(name: 'new-healthy-owner'));
    expect(latest.sent.length, 1);
  });

  test(
      'active socket cleanup failure before connector never validates or replaces first error',
      () async {
    final fixture = _Fixture();
    final failure = StateError('synthetic active physical cleanup failure');
    fixture.expectedCleanupFailure = failure;
    addTearDown(fixture.dispose);
    final connected = fixture.connect('active-before-failure');
    await _until(() => fixture.attempts.length == 1);
    final socket = fixture.resolve(0, disposeFailure: failure);
    await connected;
    var validationClients = 0;
    await io.HttpOverrides.runZoned(() async {
      await expectLater(fixture.connect('cleanup-must-block-connector'),
          throwsA(same(failure)));
    }, createHttpClient: (context) {
      validationClients++;
      throw StateError(
          'HTTP validation must not follow physical cleanup error');
    });
    expect(validationClients, 0);
    expect(fixture.attempts.length, 1);
    expect(socket.physicalDisposals, 1);
    await expectLater(fixture.client.cleanUp(), throwsA(same(failure)));
    expect(socket.physicalDisposals, 1);
  });
}
