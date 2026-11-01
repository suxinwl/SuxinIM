// Actual MediaDeviceNative, production call capture helpers and ControlsView
// with a real LocalParticipant. Only room connectivity/native channels are
// controlled. No camera, Android/JNI, server or account fixture is used.
import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_screenutil/flutter_screenutil.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_webrtc/src/native/mediadevices_impl.dart';
import 'package:flutter_webrtc/src/native/utils.dart' as native;
import 'package:livekit_client/src/core/engine.dart';
import 'package:livekit_client/src/core/room.dart';
import 'package:livekit_client/src/events.dart';
import 'package:livekit_client/src/managers/event.dart';
import 'package:livekit_client/src/options.dart';
import 'package:livekit_client/src/participant/local.dart';
import 'package:livekit_client/src/proto/livekit_models.pb.dart' as models;
import 'package:openim_live/src/live_client.dart';
import 'package:openim_live/src/pages/single/widgets/controls.dart';
import 'package:openim_live/src/utils/call_capture.dart';

class _Engine extends Fake implements Engine {
  @override
  bool get isClosed => false;
}

class _Room extends Fake implements Room {
  @override
  final Engine engine = _Engine();
  @override
  final events = EventsEmitter<RoomEvent>();
  @override
  RoomOptions get roomOptions => const RoomOptions();
  @override
  LocalParticipant? localParticipant;
}

Future<_Room> _room(String name) async {
  final room = _Room();
  room.localParticipant = await LocalParticipant.createFromInfo(
      room: room,
      info: models.ParticipantInfo(sid: 'PA_$name', identity: name));
  return room;
}

Future<void> _release(_Room room) async {
  await room.localParticipant!.dispose();
  await room.events.dispose();
}

Future<void> _drain(WidgetTester tester) async {
  for (var i = 0; i < 5; i++) {
    // Explicitly advance zero-duration fake timers created by an async UI
    // error observer; pump() alone only flushes this test's microtasks.
    await tester.pump(Duration.zero);
    await tester.runAsync(() => Future<void>.delayed(Duration.zero));
  }
  await tester.pump();
}

Widget _controls(Stream<Room> rooms, CallCaptureErrorHandler onError) =>
    ScreenUtilInit(
      designSize: const Size(390, 844),
      builder: (context, child) => MaterialApp(
        home: Scaffold(
          body: ControlsView(
            callType: CallType.video,
            initState: CallState.call,
            callStateStream: const Stream<CallState>.empty(),
            roomDidUpdateStream: rooms,
            onCaptureError: onError,
          ),
        ),
      ),
    );

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  final messenger =
      TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
  const methods = MethodChannel('FlutterWebRTC.Method');
  late List<MethodCall> captures;
  late Future<dynamic> Function(MethodCall) captureReply;
  setUp(() {
    native.WebRTC.initialized = true;
    captures = [];
    captureReply = (_) async => throw PlatformException(
        code: 'RTC_MEDIA_UNAVAILABLE',
        message: 'synthetic owned capture rejected',
        details: {'stage': 'native-admission'});
    messenger.setMockMethodCallHandler(methods, (call) async {
      if (call.method == 'getSources') return {'sources': []};
      if (call.method == 'getUserMedia') {
        captures.add(call);
        return captureReply(call);
      }
      return null;
    });
    messenger.setMockMethodCallHandler(
        const MethodChannel('FlutterWebRTC.Event'), (_) async => null);
  });
  tearDown(() {
    messenger.setMockMethodCallHandler(methods, null);
    messenger.setMockMethodCallHandler(
        const MethodChannel('FlutterWebRTC.Event'), null);
  });

  test(
      'actual media device rejection keeps PlatformException through production observer',
      () async {
    Object? decoded, reported;
    StackTrace? reportedStack;
    final result = await observeCallCapture(
      stage: 'test.actualMediaDevice',
      operation: () async {
        try {
          await MediaDeviceNative.instance
              .getUserMedia({'audio': false, 'video': true});
        } catch (error) {
          decoded = error;
          rethrow;
        }
      },
      isCurrent: () => true,
      onError: (error, stack) async {
        reported = error;
        reportedStack = stack;
      },
    );
    expect(result, isFalse);
    expect(reported, same(decoded));
    expect(
        reported,
        isA<PlatformException>()
            .having((e) => e.code, 'code', 'RTC_MEDIA_UNAVAILABLE'));
    expect(
        (reported as PlatformException).details, {'stage': 'native-admission'});
    expect(
        reportedStack.toString(), contains('MediaDeviceNative.getUserMedia'));
    expect(captures.length, 1);
  });

  test(
      'actual participant camera rejection remains false and still observes audio',
      () async {
    final room = await _room('camera-and-audio');
    addTearDown(() => _release(room));
    final errors = <Object>[];
    final result = await publishCallMedia(
      participant: room.localParticipant!,
      cameraEnabled: true,
      microphoneEnabled: true,
      isCurrent: () => true,
      onError: (error, stack) => errors.add(error),
    );
    expect(result, isFalse);
    expect(captures.length, 2);
    expect(captures[0].arguments['constraints']['video'], isNot(false));
    expect(captures[0].arguments['constraints']['audio'], isFalse);
    expect(captures[1].arguments['constraints']['video'], isFalse);
    expect(captures[1].arguments['constraints']['audio'], isNot(false));
    expect(errors.length, 2);
    expect(errors.every((e) => e is PlatformException), isTrue);
    expect(room.localParticipant!.trackPublications, isEmpty);
    expect(room.localParticipant!.isCameraEnabled(), isFalse);
  });

  test(
      'superseded actual participant publication cannot start the next media step',
      () async {
    final room = await _room('retired-publication');
    addTearDown(() => _release(room));
    var current = true;
    final result = await publishCallMedia(
      participant: room.localParticipant!,
      cameraEnabled: true,
      microphoneEnabled: true,
      isCurrent: () => current,
      onError: (error, stack) {
        current = false;
      },
    );
    expect(result, isFalse);
    expect(captures.length, 1,
        reason: 'a closed call cannot begin microphone capture after camera');
  });

  testWidgets(
      'actual controls coalesce camera taps and expose real participant rejection',
      (tester) async {
    final room = await _room('controls-rejection');
    final rooms = StreamController<Room>.broadcast();
    addTearDown(() async {
      await rooms.close();
      await _release(room);
    });
    final held = Completer<dynamic>();
    addTearDown(() {
      if (!held.isCompleted)
        held.completeError(PlatformException(code: 'TEST_END'));
    });
    captureReply = (_) => held.future;
    final errors = <Object>[];
    await tester.pumpWidget(_controls(rooms.stream, (error, stack) async {
      await Future<void>.delayed(Duration.zero);
      errors.add(error);
    }));
    rooms.add(room);
    await _drain(tester);
    final toggle = find.byKey(const ValueKey('call-camera-toggle'));
    expect(toggle, findsOneWidget);
    await tester.tap(toggle);
    await tester.tap(toggle);
    await _drain(tester);
    expect(captures.length, 1);
    held.completeError(PlatformException(code: 'RTC_MEDIA_UNAVAILABLE'));
    await _drain(tester);
    expect(errors.length, 1);
    expect(
        errors.single,
        isA<PlatformException>()
            .having((e) => e.code, 'code', 'RTC_MEDIA_UNAVAILABLE'));
    expect(room.localParticipant!.isCameraEnabled(), isFalse);
    captureReply = (_) async => throw PlatformException(code: 'RTC_MEDIA_BUSY');
    await tester.tap(toggle);
    await _drain(tester);
    expect(captures.length, 2, reason: 'failure must release the UI tap latch');
    expect(errors.length, 2);
    await tester.pumpWidget(const SizedBox.shrink());
    await _drain(tester);
    expect(tester.takeException(), isNull);
  });

  testWidgets(
      'late old-room rejection cannot clear a newer camera operation or report its error',
      (tester) async {
    final old = await _room('old-controls'), next = await _room('new-controls');
    final rooms = StreamController<Room>.broadcast();
    final first = Completer<dynamic>(), second = Completer<dynamic>();
    addTearDown(() async {
      if (!first.isCompleted)
        first.completeError(PlatformException(code: 'TEST_END'));
      if (!second.isCompleted)
        second.completeError(PlatformException(code: 'TEST_END'));
      await rooms.close();
      await _release(old);
      await _release(next);
    });
    captureReply = (_) => captures.length == 1 ? first.future : second.future;
    final errors = <Object>[];
    await tester.pumpWidget(
        _controls(rooms.stream, (error, stack) => errors.add(error)));
    rooms.add(old);
    await _drain(tester);
    final toggle = find.byKey(const ValueKey('call-camera-toggle'));
    await tester.tap(toggle);
    await _drain(tester);
    expect(captures.length, 1);
    rooms.add(next);
    await _drain(tester);
    await tester.tap(toggle);
    await _drain(tester);
    expect(captures.length, 2);
    first.completeError(PlatformException(code: 'OLD_ROOM_FAILURE'));
    await _drain(tester);
    expect(errors, isEmpty,
        reason: 'retired error cannot belong to the new call');
    await tester.tap(toggle);
    await _drain(tester);
    expect(captures.length, 2,
        reason: 'old completion cannot release the newer in-flight latch');
    second.completeError(PlatformException(code: 'NEW_ROOM_FAILURE'));
    await _drain(tester);
    expect(errors.length, 1);
    expect(
        errors.single,
        isA<PlatformException>()
            .having((e) => e.code, 'code', 'NEW_ROOM_FAILURE'));
    expect(next.localParticipant!.trackPublications, isEmpty);
    await tester.pumpWidget(const SizedBox.shrink());
    await _drain(tester);
    expect(tester.takeException(), isNull);
  });
}
