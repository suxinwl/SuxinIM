// Exercises the real Dart renderer through its native MethodChannel boundary.
// Native sink/EGL behavior is covered separately by Java and AVD tests.
import 'dart:async';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_webrtc/flutter_webrtc.dart' as rtc;
import 'package:flutter_webrtc/src/native/utils.dart' as native;

class SyntheticStream extends Fake implements rtc.MediaStream {
  SyntheticStream(this.id);
  @override
  final String id;
  @override
  String get ownerTag => 'synthetic-owner';
}

Future<void> turn() => Future<void>.delayed(Duration.zero);

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  final messenger =
      TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
  const channel = MethodChannel('FlutterWebRTC.Method');
  const events = MethodChannel('FlutterWebRTC/Texture41');
  late Future<dynamic> Function(MethodCall) respond;
  late List<MethodCall> calls;
  late List<String> eventCalls;
  setUp(() {
    native.WebRTC.initialized = true;
    calls = [];
    eventCalls = [];
    respond = (call) async =>
        call.method == 'createVideoRenderer' ? {'textureId': 41} : null;
    messenger.setMockMethodCallHandler(channel, (call) async {
      calls.add(call);
      return respond(call);
    });
    messenger.setMockMethodCallHandler(events, (call) async {
      eventCalls.add(call.method);
      return null;
    });
  });
  tearDown(() {
    messenger.setMockMethodCallHandler(channel, null);
    messenger.setMockMethodCallHandler(events, null);
  });

  test(
      'initialize is shared and one real disposal cancels events and frees texture',
      () async {
    final renderer = rtc.RTCVideoRenderer();
    final first = renderer.initialize(), second = renderer.initialize();
    expect(identical(first, second), true);
    await first;
    await turn();
    expect(calls.where((c) => c.method == 'createVideoRenderer').length, 1);
    expect(eventCalls, ['listen']);
    final disposal = renderer.dispose();
    expect(identical(disposal, renderer.dispose()), true);
    await disposal;
    expect(eventCalls, ['listen', 'cancel']);
    expect(calls.where((c) => c.method == 'videoRendererDispose').length, 1);
    expect(renderer.textureId, null);
  });

  test(
      'dispose during pending create releases the late texture without subscribing',
      () async {
    final created = Completer<dynamic>();
    respond = (call) async =>
        call.method == 'createVideoRenderer' ? created.future : null;
    final renderer = rtc.RTCVideoRenderer();
    final initialization = renderer.initialize();
    await turn();
    final disposal = renderer.dispose();
    expect(identical(disposal, renderer.dispose()), true);
    await turn();
    expect(calls.where((c) => c.method == 'videoRendererDispose'), isEmpty);
    created.complete({'textureId': 41});
    await initialization;
    await disposal;
    expect(eventCalls, isEmpty);
    expect(calls.where((c) => c.method == 'videoRendererDispose').length, 1);
    expect(renderer.textureId, null);
  });

  test(
      'create failure completes all initialize waiters and disposal preserves that failure',
      () async {
    final created = Completer<dynamic>();
    respond = (call) async => created.future;
    final renderer = rtc.RTCVideoRenderer();
    final first = renderer.initialize(), second = renderer.initialize();
    final one = expectLater(
        first,
        throwsA(isA<PlatformException>()
            .having((e) => e.code, 'code', 'CREATE_FAILED')));
    final two = expectLater(
        second,
        throwsA(isA<PlatformException>()
            .having((e) => e.code, 'code', 'CREATE_FAILED')));
    created.completeError(PlatformException(code: 'CREATE_FAILED'));
    await one;
    await two;
    await expectLater(
        renderer.dispose(),
        throwsA(isA<PlatformException>()
            .having((e) => e.code, 'code', 'CREATE_FAILED')));
    expect(calls.where((c) => c.method == 'videoRendererDispose'), isEmpty);
  });

  test('source is confirmed only after native binding succeeds', () async {
    final renderer = rtc.RTCVideoRenderer();
    await renderer.initialize();
    final bound = Completer<dynamic>();
    respond = (call) async =>
        call.method == 'videoRendererSetSrcObject' ? bound.future : null;
    final stream = SyntheticStream('source-a');
    final binding = renderer.setSrcObject(stream: stream);
    await turn();
    expect(renderer.srcObject, null);
    expect(renderer.renderVideo, false);
    bound.complete();
    await binding;
    expect(renderer.srcObject, same(stream));
    expect(renderer.renderVideo, true);
    await renderer.dispose();
  });

  test('old source completion cannot overwrite a newer acknowledged binding',
      () async {
    final renderer = rtc.RTCVideoRenderer();
    await renderer.initialize();
    final a = Completer<dynamic>(), b = Completer<dynamic>();
    respond = (call) async => call.method != 'videoRendererSetSrcObject'
        ? null
        : call.arguments['streamId'] == 'a'
            ? a.future
            : b.future;
    final old = renderer.setSrcObject(stream: SyntheticStream('a'));
    final next = SyntheticStream('b');
    final recent = renderer.setSrcObject(stream: next);
    b.complete();
    await recent;
    a.complete();
    await old;
    expect(renderer.srcObject, same(next));
    await renderer.dispose();
  });

  test(
      'a failed native source change remains an error with no claimed active source',
      () async {
    final renderer = rtc.RTCVideoRenderer();
    await renderer.initialize();
    respond = (call) async {
      if (call.method == 'videoRendererSetSrcObject')
        throw PlatformException(code: 'RTC_MEDIA_TIMEOUT');
      return null;
    };
    await expectLater(renderer.setSrcObject(stream: SyntheticStream('timeout')),
        throwsA(isA<PlatformException>()));
    expect(renderer.srcObject, null);
    expect(renderer.renderVideo, false);
    expect(renderer.textureId, 41);
    await renderer.dispose();
  });

  test(
      'late source and frame events after disposal cannot update a disposed notifier',
      () async {
    final renderer = rtc.RTCVideoRenderer();
    await renderer.initialize();
    final bound = Completer<dynamic>();
    respond = (call) async =>
        call.method == 'videoRendererSetSrcObject' ? bound.future : null;
    final binding = renderer.setSrcObject(stream: SyntheticStream('late'));
    await turn();
    await renderer.dispose();
    renderer.eventListener(
        {'event': 'didTextureChangeVideoSize', 'width': 640, 'height': 480});
    bound.complete();
    await binding;
    expect(renderer.srcObject, null);
    expect(renderer.textureId, null);
  });

  test('failed native dispose retains texture and the same failing completion',
      () async {
    final renderer = rtc.RTCVideoRenderer();
    await renderer.initialize();
    respond = (call) async {
      if (call.method == 'videoRendererDispose')
        throw PlatformException(code: 'RTC_MEDIA_TIMEOUT');
      return null;
    };
    final first = renderer.dispose();
    final repeated = renderer.dispose();
    expect(identical(first, repeated), true);
    await expectLater(first, throwsA(isA<PlatformException>()));
    await expectLater(repeated, throwsA(isA<PlatformException>()));
    expect(renderer.textureId, 41);
    expect(calls.where((c) => c.method == 'videoRendererDispose').length, 1);
    expect(() => renderer.srcObject = SyntheticStream('forbidden'),
        throwsStateError);
  });

  test(
      'legacy setter observes asynchronous native failure without reporting a successful source',
      () async {
    final renderer = rtc.RTCVideoRenderer();
    await renderer.initialize();
    respond = (call) async {
      if (call.method == 'videoRendererSetSrcObject')
        throw PlatformException(code: 'RTC_RENDERER_UNAVAILABLE');
      return null;
    };
    renderer.srcObject = SyntheticStream('unavailable');
    await turn();
    await turn();
    expect(renderer.srcObject, null);
    expect(renderer.renderVideo, false);
    await renderer.dispose();
  });
}
