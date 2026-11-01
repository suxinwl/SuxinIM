// Production VideoTrackRenderer + real Dart RTCVideoRenderer. Only the native
// MethodChannel/media objects are controlled; no Android/JNI/camera involved.
import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_webrtc/flutter_webrtc.dart' as rtc;
import 'package:flutter_webrtc/src/native/utils.dart' as native;
import 'package:livekit_client/src/events.dart';
import 'package:livekit_client/src/managers/event.dart';
import 'package:livekit_client/src/track/remote/video.dart';
import 'package:livekit_client/src/types/other.dart';
import 'package:livekit_client/src/widgets/video_track_renderer.dart';

class _NativeTrack extends Fake implements rtc.MediaStreamTrack {
  @override
  final String id = 'synthetic-native-video';
  @override
  String get kind => 'video';
  @override
  bool enabled = true;
}

class _Stream extends Fake implements rtc.MediaStream {
  _Stream(this.id);
  @override
  final String id;
  @override
  String get ownerTag => 'synthetic-owner';
}

class _Track extends RemoteVideoTrack {
  _Track(String id) : super(TrackSource.camera, _Stream(id), _NativeTrack());
  final createdListeners = <EventsListener<TrackEvent>>[];
  @override
  EventsListener<TrackEvent> createListener(
      {bool synchronized = false, Object? Function()? callbackEpoch}) {
    final result = super.createListener(
        synchronized: synchronized, callbackEpoch: callbackEpoch);
    createdListeners.add(result);
    return result;
  }
}

Future<void> _pumpTurns(WidgetTester tester) async {
  // EventChannel subscription cancellation can finish in the real root zone.
  // Give that transport Future a turn rather than treating fake-clock pumping
  // as proof that asynchronous native teardown has completed.
  for (var i = 0; i < 3; i++) {
    await tester.pump();
    await tester.runAsync(() => Future<void>.delayed(Duration.zero));
  }
  await tester.pump();
}

Widget _view(_Track track, {rtc.RTCVideoRenderer? cachedRenderer}) =>
    MaterialApp(
      home: SizedBox(
        width: 320,
        height: 240,
        child: VideoTrackRenderer(track,
            key: const ValueKey('same-production-widget'),
            cachedRenderer: cachedRenderer),
      ),
    );

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  final messenger =
      TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
  const channel = MethodChannel('FlutterWebRTC.Method');
  late List<MethodCall> calls;
  late Future<dynamic> Function(MethodCall) respond;

  setUp(() {
    native.WebRTC.initialized = true;
    calls = [];
    respond = (call) async =>
        call.method == 'createVideoRenderer' ? {'textureId': 41} : null;
    messenger.setMockMethodCallHandler(channel, (call) async {
      calls.add(call);
      return respond(call);
    });
    for (final id in [41, 42]) {
      messenger.setMockMethodCallHandler(
          MethodChannel('FlutterWebRTC/Texture$id'), (_) async => null);
    }
  });
  tearDown(() {
    messenger.setMockMethodCallHandler(channel, null);
    for (final id in [41, 42]) {
      messenger.setMockMethodCallHandler(
          MethodChannel('FlutterWebRTC/Texture$id'), null);
    }
  });

  testWidgets(
      'actual widget unmount during create cannot attach or register a late listener',
      (tester) async {
    final created = Completer<dynamic>();
    respond = (call) async =>
        call.method == 'createVideoRenderer' ? created.future : null;
    final track = _Track('pending-unmount');
    addTearDown(track.dispose);
    await tester.pumpWidget(_view(track));
    await _pumpTurns(tester);
    expect(calls.where((c) => c.method == 'createVideoRenderer').length, 1);
    await tester.pumpWidget(const SizedBox.shrink());
    expect(track.viewRegistrations, isEmpty);
    expect(calls.where((c) => c.method == 'videoRendererDispose'), isEmpty);
    created.complete({'textureId': 41});
    await _pumpTurns(tester);
    expect(calls.where((c) => c.method == 'videoRendererDispose').length, 1);
    expect(
        calls.where((c) => c.method == 'videoRendererSetSrcObject'), isEmpty);
    expect(track.createdListeners, isEmpty);
    expect(tester.takeException(), isNull);
  });

  testWidgets(
      'actual widget track replacement while create is pending renders only the latest track',
      (tester) async {
    final created = Completer<dynamic>();
    respond = (call) async =>
        call.method == 'createVideoRenderer' ? created.future : null;
    final old = _Track('old-track'), latest = _Track('latest-track');
    addTearDown(old.dispose);
    addTearDown(latest.dispose);
    await tester.pumpWidget(_view(old));
    await _pumpTurns(tester);
    await tester.pumpWidget(_view(latest));
    await _pumpTurns(tester);
    expect(calls.where((c) => c.method == 'createVideoRenderer').length, 1);
    created.complete({'textureId': 41});
    await _pumpTurns(tester);
    final changes = calls.where((c) => c.method == 'videoRendererSetSrcObject');
    expect(changes, isNotEmpty);
    expect(changes.every((c) => c.arguments['streamId'] == 'latest-track'),
        isTrue);
    expect(old.createdListeners, isEmpty);
    expect(find.byType(rtc.RTCVideoView), findsOneWidget);
    expect(tester.takeException(), isNull);
    await tester.pumpWidget(const SizedBox.shrink());
    await _pumpTurns(tester);
    expect(latest.createdListeners.every((listener) => listener.isDisposed),
        isTrue);
  });

  testWidgets(
      'actual widget failed native disposal still cancels its event listeners',
      (tester) async {
    final track = _Track('dispose-failure');
    addTearDown(track.dispose);
    await tester.pumpWidget(_view(track));
    await _pumpTurns(tester);
    expect(track.createdListeners, isNotEmpty);
    respond = (call) async {
      if (call.method == 'videoRendererDispose')
        throw PlatformException(code: 'RTC_MEDIA_FAILED');
      return null;
    };
    await tester.pumpWidget(const SizedBox.shrink());
    await _pumpTurns(tester);
    expect(calls.where((c) => c.method == 'videoRendererDispose').length, 1);
    expect(track.createdListeners.every((listener) => listener.isDisposed),
        isTrue);
    expect(track.viewRegistrations, isEmpty);
    expect(tester.takeException(), isNull);
  });
}
