// Actual MeetingClient + EnterpriseManager/Dio/LoadingView/Overlay regressions.
// Only signaling HTTP replies, Room.connect, wakelock and the overlay's visual
// child are controlled. Real Room/Engine/Transport disposal still reaches an
// owned fake native peer. These are not device/media/full MeetingRoom UI tests.
import 'dart:async';
import 'dart:convert';
import 'dart:typed_data';

import 'package:app_im/app_im.dart';
import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_webrtc/flutter_webrtc.dart' as rtc;
import 'package:get/get.dart';
import 'package:livekit_client/livekit_client.dart';
import 'package:livekit_client/src/core/engine.dart';
import 'package:livekit_client/src/core/signal_client.dart';
import 'package:livekit_client/src/core/transport.dart';
import 'package:livekit_client/src/support/websocket.dart';
import 'package:openim_common/openim_common.dart';
import 'package:openim_meeting/src/meeting_client.dart';

class _Reply {
  _Reply(this.options);
  final RequestOptions options;
  final value = Completer<ResponseBody>();
  void complete(String roomID) {
    value.complete(ResponseBody.fromString(
        jsonEncode({
          'errCode': 0,
          'data': {
            'roomID': roomID,
            'token': 'synthetic-livekit-token',
            'liveURL': 'wss://unit.invalid/livekit',
          }
        }),
        200,
        headers: {
          Headers.contentTypeHeader: [Headers.jsonContentType]
        }));
  }
}

class _Http implements HttpClientAdapter {
  final replies = <_Reply>[];
  final fetchPaths = <String>[];
  @override
  Future<ResponseBody> fetch(RequestOptions options, Stream<Uint8List>? stream,
      Future<void>? cancelFuture) {
    fetchPaths.add(options.path);
    expectSync(
        options.path.endsWith('/signal_join_meeting') ||
            options.path.endsWith('/signal_create_meeting'),
        isTrue);
    final reply = _Reply(options);
    replies.add(reply);
    return reply.value.future;
  }

  @override
  void close({bool force = false}) {}
}

class _Signal extends SignalClient {
  _Signal()
      : super((Uri uri,
            {WebSocketEventHandlers? options,
            Map<String, String>? headers,
            NetworkOptions? networkOptions}) async {
          throw StateError('No socket connection is allowed in this test');
        });
}

class _Peer extends Fake implements rtc.RTCPeerConnection {
  Completer<void>? releaseClose;
  int closeCalls = 0, disposeCalls = 0;
  bool physicallyClosed = false;
  @override
  Future<List<rtc.RTCRtpSender>> getSenders() async => [];
  @override
  Future<void> close() async {
    closeCalls++;
    if (releaseClose != null) await releaseClose!.future;
    physicallyClosed = true;
  }

  @override
  Future<void> dispose() async {
    disposeCalls++;
  }

  @override
  dynamic noSuchMethod(Invocation invocation) => null;
}

class _ControlledRoom extends Room {
  _ControlledRoom(this.label, Engine engine, this.peer) : super(engine: engine);
  final String label;
  final _Peer peer;
  Completer<void>? releaseConnect;
  Object? connectFailure;
  int connectCalls = 0, disconnectCalls = 0;
  bool physicalDisposeFinished = false;
  @override
  Future<void> connect(String url, String token,
      {ConnectOptions? connectOptions,
      RoomOptions? roomOptions,
      FastConnectOptions? fastConnectOptions}) async {
    connectCalls++;
    if (releaseConnect != null) await releaseConnect!.future;
    if (connectFailure != null) throw connectFailure!;
  }

  @override
  Future<void> disconnect() {
    disconnectCalls++;
    return super.disconnect();
  }

  @override
  Future<bool> dispose() async {
    final result = await super.dispose();
    physicalDisposeFinished = true;
    return result;
  }
}

class _OverlayOwner extends StatefulWidget {
  const _OverlayOwner(
      this.fixture, this.room, this.listener, this.roomID, this.close,
      {super.key});
  final _Fixture fixture;
  final Room room;
  final EventsListener<RoomEvent> listener;
  final String roomID;
  final VoidCallback close;
  @override
  State<_OverlayOwner> createState() => _OverlayOwnerState();
}

class _OverlayOwnerState extends State<_OverlayOwner> {
  @override
  void dispose() {
    // This thin visual boundary owns the actual Room handed to it by the actual
    // client. It does not fake Room disposal success or the underlying peer.
    final listener = widget.listener;
    final room = widget.room;
    final fixture = widget.fixture;
    final physical = (() async {
      try {
        await listener.dispose();
      } finally {
        await room.dispose();
      }
    })();
    fixture.widgetDisposals.add(physical);
    unawaited(physical.catchError((Object error) {
      fixture.widgetErrors.add(error);
    }));
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => Material(child: Text(widget.roomID));
}

class _Fixture {
  _Fixture() {
    final dio = Dio()..httpClientAdapter = http;
    dio.interceptors.add(InterceptorsWrapper(onRequest: (options, handler) {
      requestPhases.add('onRequest:${options.method}');
      handler.next(options);
    }, onError: (error, handler) {
      requestPhases.add(
          'onError:${error.type}:${error.error.runtimeType}:${error.message}');
      handler.next(error);
    }));
    manager = EnterpriseManager(dio: dio)
      ..configure(
          baseUrl: () => 'https://unit.invalid/enterprise/v1',
          imToken: () => im,
          chatToken: () => chat,
          platform: () => 2,
          userID: () => user,
          ready: () => true);
    client = MeetingClient.withDependencies(
        enterprise: manager,
        roomFactory: () => rooms[factoryCalls++],
        wakelock: (enabled) async {
          wakeRequests.add(enabled);
          if (enabled && holdFirstWake && !wakeHeld) {
            wakeHeld = true;
            await releaseWake.future;
          }
          wakeState = enabled;
        },
        meetingBuilder: (room, listener, id, close) {
          closeCallbacks[id] = close;
          return _OverlayOwner(this, room, listener, id, close,
              key: ValueKey('owned-$id'));
        });
  }
  final http = _Http();
  late final EnterpriseManager manager;
  late final MeetingClient client;
  late BuildContext context;
  bool contextReady = false;
  String user = 'synthetic-user-A', im = 'fake-im-A', chat = 'fake-chat-A';
  final rooms = <_ControlledRoom>[];
  final operations = <Future<bool>>[];
  final widgetDisposals = <Future<void>>[];
  final widgetErrors = <Object>[];
  final wakeRequests = <bool>[];
  final closeCallbacks = <String, VoidCallback>{};
  final requestPhases = <String>[];
  int factoryCalls = 0;
  bool holdFirstWake = false, wakeHeld = false, wakeState = false;
  final releaseWake = Completer<void>();

  CallTerminationSession get session => manager.captureMeetingStateSession();
  void replaceSession() {
    manager.invalidateCallTerminations();
    user = 'synthetic-user-B';
    im = 'fake-im-B';
    chat = 'fake-chat-B';
  }

  Future<_ControlledRoom> addRoom(String label) async {
    final peer = _Peer();
    final transport = await Transport.create(
        (configuration, [constraints = const <String, dynamic>{}]) async =>
            peer,
        connectOptions: const ConnectOptions());
    final engine = Engine(
        connectOptions: const ConnectOptions(),
        roomOptions: const RoomOptions(),
        signalClient: _Signal());
    engine.publisher = transport;
    final room = _ControlledRoom(label, engine, peer);
    rooms.add(room);
    return room;
  }

  Future<bool> join(String id, {bool Function()? isCurrent}) {
    _phase('join/$id/before', this);
    final operation = client.join(context,
        meetingID: id, expectedSession: session, isCurrent: isCurrent);
    _phase('join/$id/after', this);
    operations.add(operation);
    return operation;
  }
}

Future<void> _flush(WidgetTester tester, {int count = 6}) async {
  for (var i = 0; i < count; i++) {
    await tester.pump(const Duration(milliseconds: 5));
    // Dio/Stream cleanup may finish on a real event-loop task outside the
    // widget's fake-clock zone. Yield it explicitly, within this bounded loop.
    await tester
        .runAsync(() => Future<void>.delayed(const Duration(milliseconds: 1)));
  }
}

void _phase(String label, [_Fixture? fixture]) {
  // Only controlled counters/booleans; never credentials, URLs or server data.
  print(jsonEncode({
    'phase': label,
    if (fixture != null) ...{
      'busy': fixture.client.isBusy,
      'http': fixture.http.replies.length,
      'fetchPaths': fixture.http.fetchPaths,
      'requestPhases': fixture.requestPhases,
      'sessionCurrent':
          fixture.manager.isMeetingStateSessionCurrent(fixture.session),
      'contextMounted':
          fixture.contextReady ? fixture.context.mounted : null,
      'factory': fixture.factoryCalls,
      'wakeRequests': fixture.wakeRequests,
      'widgetDisposals': fixture.widgetDisposals.length,
    }
  }));
}

Future<T> _settle<T>(WidgetTester tester, Future<T> operation, String label,
    {_Fixture? fixture}) async {
  var done = false;
  late T value;
  Object? failure;
  StackTrace? failureStack;
  operation.then<void>((result) {
    value = result;
    done = true;
  }, onError: (Object error, StackTrace stack) {
    failure = error;
    failureStack = stack;
    done = true;
  });
  _phase('$label/begin', fixture);
  // Observe completion while advancing the widget fake clock. No direct await
  // of a pending Dio/Stream/Overlay Future can suspend the test driver itself.
  for (var i = 0; !done && i < 80; i++) {
    await _flush(tester, count: 1);
  }
  _phase('$label/end:done=$done', fixture);
  expect(done, isTrue, reason: 'Bounded fake-clock completion: $label');
  if (failure != null) Error.throwWithStackTrace(failure!, failureStack!);
  return value;
}

Future<_Fixture> _mount(WidgetTester tester) async {
  _phase('mount/before-fixture');
  final f = _Fixture();
  _phase('mount/fixture-created', f);
  Get.testMode = true;
  await tester.pumpWidget(
      GetMaterialApp(home: Scaffold(body: Builder(builder: (context) {
    f.context = context;
    f.contextReady = true;
    return const Text('base-route');
  }))));
  _phase('mount/after-pumpWidget', f);
  await _flush(tester);
  expect(find.text('base-route'), findsOneWidget);
  _phase('mount/settled', f);
  _cleanup = () async {
    _phase('teardown/begin', f);
    for (final room in f.rooms) {
      if (room.releaseConnect != null && !room.releaseConnect!.isCompleted) {
        room.releaseConnect!.complete();
      }
      if (room.peer.releaseClose != null &&
          !room.peer.releaseClose!.isCompleted) {
        room.peer.releaseClose!.complete();
      }
    }
    if (!f.releaseWake.isCompleted) f.releaseWake.complete();
    for (final reply in f.http.replies) {
      if (!reply.value.isCompleted) {
        reply.complete((reply.options.data as Map)['roomID']?.toString() ??
            'synthetic-created');
      }
    }
    final closed = f.client.close();
    await _flush(tester);
    await tester.pumpWidget(const SizedBox.shrink());
    await _flush(tester);
    await _settle(tester, closed, 'teardown/closed', fixture: f);
    await _settle(tester, Future.wait(f.operations), 'teardown/operations',
        fixture: f);
    await _settle(tester, Future.wait(f.widgetDisposals), 'teardown/widgets',
        fixture: f);
    for (final room in f.rooms) {
      await _settle(tester, room.dispose(), 'teardown/room:${room.label}',
          fixture: f);
    }
    LoadingView.singleton.dismiss();
    Get.reset();
    _phase('teardown/finished', f);
  };
  return f;
}

Future<void> Function()? _cleanup;

// Room creates a real TTLMap timer even when its native peer is controlled.
// Dispose owned rooms before Flutter verifies fake-clock timer invariants;
// package:test addTearDown runs after that verification and is too late.
void _meetingTest(String name, Future<void> Function(WidgetTester) body) {
  testWidgets(name, (tester) async {
    _cleanup = null;
    try {
      await body(tester);
    } finally {
      final cleanup = _cleanup;
      _cleanup = null;
      if (cleanup != null) await cleanup();
    }
  });
}

void main() {
  _meetingTest('pair changes during real loading delay: no join HTTP or Room',
      (tester) async {
    final f = await _mount(tester);
    final attempt = f.join('room-A');
    f.replaceSession(); // No pump: actual LoadingView1ms has not elapsed.
    await _flush(tester);
    expect(await _settle(tester, attempt, 'body/attempt', fixture: f), isFalse);
    expect(f.http.replies, isEmpty);
    expect(f.factoryCalls, 0);
    expect(f.client.isBusy, isFalse);
    expect(tester.takeException(), isNull);
  });

  _meetingTest('caller page retires during loading delay without changing pair',
      (tester) async {
    final f = await _mount(tester);
    var current = true;
    final attempt = f.join('room-A', isCurrent: () => current);
    current = false;
    await _flush(tester);
    expect(await _settle(tester, attempt, 'body/attempt', fixture: f), isFalse);
    expect(f.http.replies, isEmpty);
    expect(f.factoryCalls, 0);
    expect(tester.takeException(), isNull);
  });

  _meetingTest('late old HTTP pair cannot allocate Room or replace new overlay',
      (tester) async {
    final f = await _mount(tester);
    final newRoom = await _settle(
        tester, f.addRoom('new-only'), 'setup/room:new-only',
        fixture: f);
    final old = f.join('room-A');
    await _flush(tester);
    expect(f.http.replies, hasLength(1));
    final closing = f.client.close();
    f.replaceSession();
    final fresh = f.join('room-B');
    await _flush(tester);
    expect(f.http.replies, hasLength(2));
    f.http.replies[1].complete('room-B');
    await _flush(tester);
    expect(await _settle(tester, fresh, 'body/fresh', fixture: f), isTrue);
    expect(find.text('room-B'), findsOneWidget);
    f.http.replies[0].complete('room-A');
    await _flush(tester);
    expect(await _settle(tester, old, 'body/old', fixture: f), isFalse);
    await _settle(tester, closing, 'body/closing', fixture: f);
    expect(f.factoryCalls, 1);
    expect(newRoom.connectCalls, 1);
    expect(newRoom.peer.closeCalls, 0);
    expect(find.text('room-B'), findsOneWidget);
    expect(f.http.replies[0].options.headers['token'], 'fake-im-A');
    expect(f.http.replies[0].options.headers['chatToken'], 'fake-chat-A');
    expect(f.http.replies[1].options.headers['token'], 'fake-im-B');
    expect(f.http.replies[1].options.headers['chatToken'], 'fake-chat-B');
    expect(tester.takeException(), isNull);
  });

  for (final lateFailure in [false, true]) {
    _meetingTest(
        'late Room.connect ${lateFailure ? 'failure' : 'success'} cleans only old real Room',
        (tester) async {
      final f = await _mount(tester);
      final oldRoom =
          await _settle(tester, f.addRoom('old'), 'setup/room:old', fixture: f);
      final newRoom =
          await _settle(tester, f.addRoom('new'), 'setup/room:new', fixture: f);
      oldRoom.releaseConnect = Completer<void>();
      oldRoom.peer.releaseClose = Completer<void>();
      if (lateFailure) oldRoom.connectFailure = StateError('late old connect');
      final old = f.join('room-A');
      await _flush(tester);
      f.http.replies[0].complete('room-A');
      await _flush(tester);
      expect(oldRoom.connectCalls, 1);
      final closing = f.client.close();
      final fresh = f.join('room-B');
      await _flush(tester);
      f.http.replies[1].complete('room-B');
      await _flush(tester);
      expect(await _settle(tester, fresh, 'body/fresh', fixture: f), isTrue);
      expect(find.text('room-B'), findsOneWidget);
      expect(oldRoom.peer.closeCalls, 1);
      expect(oldRoom.physicalDisposeFinished, isFalse);
      oldRoom.releaseConnect!.complete();
      await _flush(tester);
      expect(find.text('room-A'), findsNothing);
      expect(find.text('room-B'), findsOneWidget);
      expect(newRoom.peer.closeCalls, 0);
      oldRoom.peer.releaseClose!.complete();
      await _flush(tester);
      await _settle(tester, closing, 'body/closing', fixture: f);
      expect(await _settle(tester, old, 'body/old', fixture: f), isFalse);
      expect(oldRoom.physicalDisposeFinished, isTrue);
      expect(oldRoom.peer.closeCalls, 1);
      expect(oldRoom.peer.disposeCalls, 1);
      expect(newRoom.peer.closeCalls, 0);
      expect(f.client.isBusy, isTrue);
      expect(find.text('room-B'), findsOneWidget);
      expect(tester.takeException(), isNull);
    });
  }

  _meetingTest('late wakelock enable cannot install old or disable new holder',
      (tester) async {
    final f = await _mount(tester);
    final oldRoom =
        await _settle(tester, f.addRoom('old'), 'setup/room:old', fixture: f);
    final newRoom =
        await _settle(tester, f.addRoom('new'), 'setup/room:new', fixture: f);
    oldRoom.peer.releaseClose = Completer<void>();
    f.holdFirstWake = true;
    final old = f.join('room-A');
    await _flush(tester);
    f.http.replies[0].complete('room-A');
    await _flush(tester);
    expect(f.wakeHeld, isTrue);
    expect(find.text('room-A'), findsNothing);
    final closing = f.client.close();
    final fresh = f.join('room-B');
    await _flush(tester);
    f.http.replies[1].complete('room-B');
    await _flush(tester);
    expect(newRoom.connectCalls, 1);
    f.releaseWake.complete();
    await _flush(tester);
    expect(await _settle(tester, fresh, 'body/fresh', fixture: f), isTrue);
    expect(find.text('room-B'), findsOneWidget);
    expect(find.text('room-A'), findsNothing);
    oldRoom.peer.releaseClose!.complete();
    await _flush(tester);
    await _settle(tester, closing, 'body/closing', fixture: f);
    expect(await _settle(tester, old, 'body/old', fixture: f), isFalse);
    expect(oldRoom.peer.closeCalls, 1);
    expect(oldRoom.peer.disposeCalls, 1);
    expect(newRoom.peer.closeCalls, 0);
    expect(f.wakeRequests.last, isTrue);
    expect(f.wakeState, isTrue);
    expect(find.text('room-B'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });

  _meetingTest(
      'ordinary UI close does not claim peer disposal or close new holder',
      (tester) async {
    final f = await _mount(tester);
    final oldRoom =
        await _settle(tester, f.addRoom('old'), 'setup/room:old', fixture: f);
    final newRoom =
        await _settle(tester, f.addRoom('new'), 'setup/room:new', fixture: f);
    oldRoom.peer.releaseClose = Completer<void>();
    final first = f.join('room-A');
    await _flush(tester);
    f.http.replies[0].complete('room-A');
    await _flush(tester);
    expect(await _settle(tester, first, 'body/first', fixture: f), isTrue);
    final oldCallback = f.closeCallbacks['room-A']!;
    final closing = f.client.close();
    await _flush(tester);
    await _settle(tester, closing, 'body/closing',
        fixture: f); // Documented UI ownership completion, not native release.
    expect(find.text('room-A'), findsNothing);
    expect(oldRoom.peer.closeCalls, 1);
    expect(oldRoom.peer.physicallyClosed, isFalse);
    expect(oldRoom.physicalDisposeFinished, isFalse);
    expect(f.widgetDisposals, hasLength(1));
    final fresh = f.join('room-B');
    await _flush(tester);
    f.http.replies[1].complete('room-B');
    await _flush(tester);
    expect(await _settle(tester, fresh, 'body/fresh', fixture: f), isTrue);
    oldCallback(); // Actual old entry callback, after a new entry is installed.
    await _flush(tester);
    expect(find.text('room-B'), findsOneWidget);
    expect(newRoom.peer.closeCalls, 0);
    oldRoom.peer.releaseClose!.complete();
    await _flush(tester);
    await _settle(tester, f.widgetDisposals.single, 'body/widget-disposal',
        fixture: f);
    expect(oldRoom.physicalDisposeFinished, isTrue);
    expect(oldRoom.peer.closeCalls, 1);
    expect(oldRoom.peer.disposeCalls, 1);
    expect(f.widgetErrors, isEmpty);
    expect(f.client.isBusy, isTrue);
    expect(tester.takeException(), isNull);
  });

  _meetingTest('create uses captured HTTP credentials and original payload',
      (tester) async {
    final f = await _mount(tester);
    final room = await _settle(
        tester, f.addRoom('created'), 'setup/room:created',
        fixture: f);
    final created = f.client.create(f.context,
        meetingName: 'Synthetic meeting',
        startTime: 100,
        duration: 3600,
        expectedSession: f.session);
    f.operations.add(created);
    await _flush(tester);
    expect(f.http.replies, hasLength(1));
    final request = f.http.replies.single.options;
    expect(request.path, endsWith('/signal_create_meeting'));
    expect(request.headers['token'], 'fake-im-A');
    expect(request.headers['chatToken'], 'fake-chat-A');
    expect(request.headers['platformID'], 2);
    expect((request.data as Map)['meetingName'], 'Synthetic meeting');
    expect((request.data as Map)['startTime'], 100);
    expect((request.data as Map)['meetingDuration'], 3600);
    f.http.replies.single.complete('created-room');
    await _flush(tester);
    expect(await _settle(tester, created, 'body/created', fixture: f), isTrue);
    expect(room.connectCalls, 1);
    expect(find.text('created-room'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });
}
