import 'dart:async';

import 'package:flutter/foundation.dart';
import 'package:flutter/services.dart';

import 'package:webrtc_interface/webrtc_interface.dart';

import '../helper.dart';
import '../video_renderer_extension.dart' show AudioControl;
import 'utils.dart';

class RTCVideoRenderer extends ValueNotifier<RTCVideoValue>
    implements VideoRenderer, AudioControl {
  RTCVideoRenderer() : super(RTCVideoValue.empty);
  Completer<void>? _initializing;
  Future<void>? _disposing;
  bool _closing = false;
  bool _notifierDisposed = false;
  int _sourceGeneration = 0;
  int? _textureId;
  bool _disposed = false;
  MediaStream? _srcObject;
  StreamSubscription<dynamic>? _eventSubscription;

  @override
  Future<void> initialize() {
    if (_closing || _disposed)
      return Future.error(StateError('Renderer is closing'));
    final pending = _initializing;
    if (pending != null) return pending.future;
    final completion = Completer<void>();
    _initializing = completion;
    () async {
      try {
        final response = await WebRTC.invokeMethod('createVideoRenderer', {});
        _textureId = response['textureId'];
        // A late create response still belongs to the pending disposer. It must
        // release this texture instead of subscribing after the widget died.
        if (!_closing) {
          _eventSubscription = EventChannel('FlutterWebRTC/Texture$textureId')
              .receiveBroadcastStream()
              .listen(eventListener, onError: errorListener);
        }
        completion.complete();
      } catch (error, stack) {
        completion.completeError(error, stack);
      }
    }();
    return completion.future;
  }

  @override
  int get videoWidth => value.width.toInt();

  @override
  int get videoHeight => value.height.toInt();

  @override
  int? get textureId => _textureId;

  @override
  MediaStream? get srcObject => _srcObject;

  @override
  Function? onResize;

  @override
  Function? onFirstFrameRendered;

  @override
  set srcObject(MediaStream? stream) {
    if (_closing || _disposed) throw StateError('Renderer is closing');
    if (_textureId == null)
      throw StateError('Call initialize before setting the stream');
    unawaited(
        _setSource(stream: stream).catchError((Object error, StackTrace stack) {
      print('RTCVideoRenderer source change failed: ${error.runtimeType}');
    }));
  }

  Future<void> setSrcObject({MediaStream? stream, String? trackId}) =>
      _setSource(stream: stream, trackId: trackId);

  Future<void> _setSource({MediaStream? stream, String? trackId}) async {
    if (_closing || _disposed) throw StateError('Renderer is closing');
    if (_textureId == null)
      throw StateError('Call initialize before setting the stream');
    final generation = ++_sourceGeneration;
    final texture = _textureId;
    try {
      await WebRTC.invokeMethod('videoRendererSetSrcObject', <String, dynamic>{
        'textureId': texture,
        'streamId': stream?.id ?? '',
        'ownerTag': stream?.ownerTag ?? '',
        'trackId': trackId ?? '0'
      });
      if (_closing || _disposed || generation != _sourceGeneration) return;
      _srcObject = stream;
      value = (stream == null)
          ? RTCVideoValue.empty
          : value.copyWith(renderVideo: renderVideo);
    } catch (_) {
      if (!_closing && !_disposed && generation == _sourceGeneration) {
        // A failed native change has no confirmed active source.
        _srcObject = null;
        value = RTCVideoValue.empty;
      }
      rethrow;
    }
  }

  @override
  Future<void> dispose() {
    final pending = _disposing;
    if (pending != null) return pending;
    if (_disposed) return Future.value();
    _closing = true;
    ++_sourceGeneration;
    final completion = Completer<void>();
    _disposing = completion.future;
    () async {
      Object? firstError;
      StackTrace? firstStack;
      Future<void> attempt(Future<void> Function() action) async {
        try {
          await action();
        } catch (error, stack) {
          firstError ??= error;
          firstStack ??= stack;
        }
      }

      if (_initializing != null) await attempt(() => _initializing!.future);
      await attempt(() async {
        await _eventSubscription?.cancel();
        _eventSubscription = null;
      });
      var released = _textureId == null;
      if (_textureId != null) {
        await attempt(() async {
          await WebRTC.invokeMethod('videoRendererDispose', <String, dynamic>{
            'textureId': _textureId,
          });
          released = true;
        });
      }
      if (released) {
        _textureId = null;
        _srcObject = null;
        _disposed = true;
        if (!_notifierDisposed) {
          _notifierDisposed = true;
          super.dispose();
        }
      }
      if (firstError != null)
        completion.completeError(firstError!, firstStack!);
      else
        completion.complete();
    }();
    return completion.future;
  }

  void eventListener(dynamic event) {
    if (_closing || _disposed) return;
    final Map<dynamic, dynamic> map = event;
    switch (map['event']) {
      case 'didTextureChangeRotation':
        value =
            value.copyWith(rotation: map['rotation'], renderVideo: renderVideo);
        onResize?.call();
        break;
      case 'didTextureChangeVideoSize':
        value = value.copyWith(
            width: 0.0 + map['width'],
            height: 0.0 + map['height'],
            renderVideo: renderVideo);
        onResize?.call();
        break;
      case 'didFirstFrameRendered':
        value = value.copyWith(renderVideo: renderVideo);
        onFirstFrameRendered?.call();
        break;
    }
  }

  void errorListener(Object obj) {
    print('RTCVideoRenderer event stream failed: ${obj.runtimeType}');
  }

  @override
  bool get renderVideo => _textureId != null && _srcObject != null;

  @override
  bool get muted => _srcObject?.getAudioTracks()[0].muted ?? true;

  @override
  set muted(bool mute) {
    if (_disposed) {
      throw Exception('Can\'t be muted: The RTCVideoRenderer is disposed');
    }
    if (_srcObject == null) {
      throw Exception('Can\'t be muted: The MediaStream is null');
    }
    if (_srcObject!.ownerTag != 'local') {
      throw Exception(
          'You\'re trying to mute a remote track, this is not supported');
    }
    if (_srcObject!.getAudioTracks().isEmpty) {
      throw Exception('Can\'t be muted: The MediaStreamTrack(audio) is empty');
    }

    Helper.setMicrophoneMute(mute, _srcObject!.getAudioTracks()[0]);
  }

  @override
  Future<bool> audioOutput(String deviceId) async {
    try {
      await Helper.selectAudioOutput(deviceId);
    } catch (e) {
      print('Helper.selectAudioOutput ${e.toString()}');
      return false;
    }
    return true;
  }

  @override
  Future<void> setVolume(double value) async {
    try {
      if (_srcObject == null) {
        throw Exception('Can\'t set volume: The MediaStream is null');
      }
      for (MediaStreamTrack track in _srcObject!.getAudioTracks()) {
        await Helper.setVolume(value, track);
      }
    } catch (e) {
      print('Helper.setVolume ${e.toString()}');
    }
  }
}
