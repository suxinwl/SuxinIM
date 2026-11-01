// Actual production VideoTrackRenderer + RTCVideoRenderer; only media/native
// MethodChannel boundaries are controlled. Not Android/JNI/device validation.
import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_webrtc/flutter_webrtc.dart' as rtc;
import 'package:flutter_webrtc/src/native/utils.dart' as native;
import 'package:livekit_client/src/track/remote/video.dart';
import 'package:livekit_client/src/types/other.dart';
import 'package:livekit_client/src/widgets/video_track_renderer.dart';

class _NativeTrack extends Fake implements rtc.MediaStreamTrack {
  _NativeTrack(this.id);
  @override
  final String id;
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
  String get ownerTag => 'synthetic-renderer-owner';
}

class _Track extends RemoteVideoTrack {
  _Track(String id)
      : super(TrackSource.camera, _Stream(id), _NativeTrack('native-$id'));
}

Future<void> _drain(WidgetTester tester, [int count = 5]) async {
  for (var i = 0; i < count; i++) {
    await tester.pump();
    await tester.runAsync(() => Future<void>.delayed(Duration.zero));
  }
  await tester.pump();
}

Widget _view(_Track track, rtc.RTCVideoRenderer renderer,
        {double width = 320, VideoViewFit fit = VideoViewFit.contain}) =>
    MaterialApp(
      home: Center(
        child: SizedBox(
          width: width,
          height: 240,
          child: VideoTrackRenderer(track,
              key: const ValueKey('same-live-widget'),
              cachedRenderer: renderer,
              fit: fit),
        ),
      ),
    );

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  final messenger =
      TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
  const methods = MethodChannel('FlutterWebRTC.Method');
  late List<MethodCall> calls;
  late Future<dynamic> Function(MethodCall) respond;
  List<MethodCall> sourceCalls() => calls
      .where((call) =>
          call.method == 'videoRendererSetSrcObject' &&
          call.arguments['streamId'] != '')
      .toList();

  setUp(() {
    native.WebRTC.initialized = true;
    calls = [];
    respond = (call) async =>
        call.method == 'createVideoRenderer' ? {'textureId': 81} : null;
    messenger.setMockMethodCallHandler(methods, (call) async {
      calls.add(call);
      return respond(call);
    });
    messenger.setMockMethodCallHandler(
        const MethodChannel('FlutterWebRTC/Texture81'), (_) async => null);
    messenger.setMockMethodCallHandler(
        const MethodChannel('FlutterWebRTC.Event'), (_) async => null);
  });
  tearDown(() {
    messenger.setMockMethodCallHandler(methods, null);
    messenger.setMockMethodCallHandler(
        const MethodChannel('FlutterWebRTC/Texture81'), null);
    messenger.setMockMethodCallHandler(
        const MethodChannel('FlutterWebRTC.Event'), null);
  });

  testWidgets(
      'parent rebuild does not rebind an unchanged renderer track stream',
      (tester) async {
    final renderer = rtc.RTCVideoRenderer();
    final track = _Track('stable-parent');
    addTearDown(track.dispose);
    await tester.pumpWidget(_view(track, renderer));
    await _drain(tester);
    expect(sourceCalls().length, 1);
    await tester.pumpWidget(_view(track, renderer, width: 340));
    await _drain(tester);
    await tester.pumpWidget(
        _view(track, renderer, width: 360, fit: VideoViewFit.cover));
    await _drain(tester);
    expect(sourceCalls().length, 1,
        reason: 'layout/fit rebuilding must not reset the same native source');
    await tester.pumpWidget(const SizedBox.shrink());
    await _drain(tester);
    expect(tester.takeException(), isNull);
  });

  testWidgets(
      'actual native resize callbacks never rebind the unchanged source',
      (tester) async {
    final renderer = rtc.RTCVideoRenderer();
    final track = _Track('stable-resize');
    addTearDown(track.dispose);
    await tester.pumpWidget(_view(track, renderer));
    await _drain(tester);
    expect(sourceCalls().length, 1);
    for (final size in [320, 640, 960]) {
      // The real RTCVideoRenderer native event entrypoint invokes the real
      // widget onResize callback; no copied setState/rebinding implementation.
      renderer.eventListener({
        'event': 'didTextureChangeVideoSize',
        'width': size,
        'height': size ~/ 2,
      });
      await _drain(tester);
    }
    expect(sourceCalls().length, 1,
        reason: 'resize -> setState must not send the same stream again');
    await tester.pumpWidget(const SizedBox.shrink());
    await _drain(tester);
    expect(tester.takeException(), isNull);
  });

  testWidgets(
      'real track and stream replacement each bind once and retire old events',
      (tester) async {
    final renderer = rtc.RTCVideoRenderer();
    final first = _Track('first-source'), next = _Track('next-source');
    addTearDown(first.dispose);
    addTearDown(next.dispose);
    await tester.pumpWidget(_view(first, renderer));
    await _drain(tester);
    await tester.pumpWidget(_view(next, renderer));
    await _drain(tester);
    final newest = _Stream('updated-same-track');
    next.updateMediaStreamAndTrack(newest, _NativeTrack('updated-native'));
    await _drain(tester);
    first.updateMediaStreamAndTrack(
        _Stream('retired-must-not-bind'), _NativeTrack('retired-native'));
    await _drain(tester);
    expect(sourceCalls().map((call) => call.arguments['streamId']).toList(),
        ['first-source', 'next-source', 'updated-same-track']);
    expect(renderer.srcObject, same(newest));
    await tester.pumpWidget(const SizedBox.shrink());
    await _drain(tester);
    expect(tester.takeException(), isNull);
  });

  testWidgets(
      'shared pending create and late old source completion cannot replace newest track',
      (tester) async {
    final created = Completer<dynamic>();
    final oldNativeSet = Completer<dynamic>();
    addTearDown(() {
      if (!created.isCompleted) created.complete({'textureId': 81});
      if (!oldNativeSet.isCompleted) oldNativeSet.complete(null);
    });
    respond = (call) async {
      if (call.method == 'createVideoRenderer') return created.future;
      if (call.method == 'videoRendererSetSrcObject' &&
          call.arguments['streamId'] == 'after-create') {
        return oldNativeSet.future;
      }
      return null;
    };
    final renderer = rtc.RTCVideoRenderer();
    final first = _Track('before-create'), next = _Track('after-create');
    final newest = _Track('newest-source');
    for (final track in [first, next, newest]) {
      addTearDown(track.dispose);
    }
    await tester.pumpWidget(_view(first, renderer));
    await _drain(tester);
    await tester.pumpWidget(_view(next, renderer));
    await _drain(tester);
    expect(
        calls.where((call) => call.method == 'createVideoRenderer').length, 1);
    created.complete({'textureId': 81});
    await _drain(tester);
    expect(sourceCalls().map((call) => call.arguments['streamId']).toList(),
        ['after-create']);
    await tester.pumpWidget(_view(newest, renderer));
    await _drain(tester);
    expect(sourceCalls().map((call) => call.arguments['streamId']).toList(),
        ['after-create', 'newest-source']);
    oldNativeSet.complete(null);
    await _drain(tester);
    expect(renderer.srcObject, same(newest.mediaStream));
    await tester.pumpWidget(const SizedBox.shrink());
    await _drain(tester);
    expect(tester.takeException(), isNull);
  });

  testWidgets(
      'native source failure stays false until a real stream update recovers the view',
      (tester) async {
    var sourceRequests = 0;
    respond = (call) async {
      if (call.method == 'createVideoRenderer') return {'textureId': 81};
      if (call.method == 'videoRendererSetSrcObject' &&
          call.arguments['streamId'] != '') {
        sourceRequests++;
        if (sourceRequests == 1) {
          throw PlatformException(code: 'RTC_RENDERER_BUSY');
        }
      }
      return null;
    };
    final renderer = rtc.RTCVideoRenderer();
    final track = _Track('initial-native-failure');
    addTearDown(track.dispose);
    await tester.pumpWidget(_view(track, renderer));
    await _drain(tester);
    expect(renderer.srcObject, isNull);
    expect(renderer.renderVideo, isFalse);
    expect(sourceRequests, 1);
    await tester.pumpWidget(_view(track, renderer, width: 350));
    await _drain(tester);
    expect(sourceRequests, 1,
        reason: 'a layout rebuild cannot blindly retry failed native attach');
    final recovered = _Stream('genuine-new-stream');
    track.updateMediaStreamAndTrack(
        recovered, _NativeTrack('genuine-new-native'));
    await _drain(tester);
    expect(sourceRequests, 2);
    expect(renderer.srcObject, same(recovered));
    expect(renderer.renderVideo, isTrue);
    expect(find.byType(rtc.RTCVideoView), findsOneWidget,
        reason:
            'successful source event cannot remain trapped by failed initialization cache');
    await tester.pumpWidget(const SizedBox.shrink());
    await _drain(tester);
    expect(tester.takeException(), isNull);
  });
}
