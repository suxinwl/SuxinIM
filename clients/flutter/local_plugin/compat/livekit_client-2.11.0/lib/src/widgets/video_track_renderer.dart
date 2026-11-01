// Copyright 2024 LiveKit, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

import 'dart:async';
import 'dart:math';

import 'package:flutter/foundation.dart' show ValueListenable, kIsWeb;
import 'package:flutter/material.dart';
import 'package:flutter/services.dart' show PlatformException;

import 'package:flutter_webrtc/flutter_webrtc.dart' as rtc;

import '../events.dart';
import '../extensions.dart';
import '../internal/events.dart';
import '../logger.dart';
import '../managers/event.dart';
import '../support/platform.dart';
import '../support/cleanup_sequence.dart';
import '../track/local/local.dart';
import '../track/local/video.dart';
import '../track/options.dart';
import '../track/video_track_view_registration.dart';
import '../types/other.dart';

enum VideoViewMirrorMode {
  auto,
  off,
  mirror,
}

enum VideoRenderMode {
  /// Let the SDK choose the rendering backend. Currently resolves to
  /// [texture] on all platforms, but the resolution may change in a
  /// future release.
  auto,

  /// Render frames into a Flutter texture.
  texture,

  /// Render with a native platform view. Supported on iOS and macOS,
  /// other platforms fall back to [texture].
  platformView,
}

enum VideoViewFit {
  contain,
  cover,
}

extension VideoViewFitExt on VideoViewFit {
  rtc.RTCVideoViewObjectFit toRTCType() {
    if (this == VideoViewFit.cover) {
      return rtc.RTCVideoViewObjectFit.RTCVideoViewObjectFitCover;
    }
    return rtc.RTCVideoViewObjectFit.RTCVideoViewObjectFitContain;
  }
}

/// Widget that renders a [VideoTrack].
class VideoTrackRenderer extends StatefulWidget {
  final VideoTrack track;
  final VideoViewFit fit;
  final VideoViewMirrorMode mirrorMode;
  final VideoRenderMode renderMode;
  final rtc.RTCVideoRenderer? cachedRenderer;
  final bool autoDisposeRenderer;

  /// wrap the video view in a Center widget (if [fit] is [VideoViewFit.contain])
  final bool autoCenter;

  /// Controls how this view's logical size is converted to the physical-pixel
  /// dimensions requested from the server when adaptive stream is enabled.
  /// Defaults to [AdaptiveStreamPixelDensity.auto] (the view's own device pixel
  /// ratio), avoiding an under-sized layer on retina / high-density displays.
  final AdaptiveStreamPixelDensity adaptiveStreamPixelDensity;

  /// Placeholder builder to display while the track is loading.
  ///
  /// On iOS and macOS, this has no effect when [renderMode] is [VideoRenderMode.platformView].
  final WidgetBuilder? placeholderBuilder;

  const VideoTrackRenderer(
    this.track, {
    this.fit = VideoViewFit.contain,
    this.mirrorMode = VideoViewMirrorMode.auto,
    this.renderMode = VideoRenderMode.auto,
    this.autoDisposeRenderer = true,
    this.cachedRenderer,
    this.autoCenter = true,
    this.adaptiveStreamPixelDensity = AdaptiveStreamPixelDensity.auto,
    this.placeholderBuilder,
    Key? key,
  }) : super(key: key);

  @override
  State<StatefulWidget> createState() => _VideoTrackRendererState();
}

class _VideoTrackRendererState extends State<VideoTrackRenderer> {
  rtc.VideoRenderer? _renderer;
  // for flutter web only.
  bool _rendererReadyForWeb = false;
  double? _aspectRatio;
  EventsListener<TrackEvent>? _listener;
  int _attachmentGeneration = 0;
  Future<rtc.VideoRenderer?>? _initialization;
  VideoTrack? _listenedTrack;
  rtc.VideoRenderer? _requestedRenderer;
  VideoTrack? _requestedTrack;
  rtc.MediaStream? _requestedStream;
  String? _requestedTrackId;
  int _sourceGeneration = 0;
  // Used to compute visibility information
  late VideoTrackViewRegistration _viewRegistration;

  bool _usesPlatformView(VideoRenderMode renderMode) =>
      renderMode == VideoRenderMode.platformView && [PlatformType.iOS, PlatformType.macOS].contains(lkPlatform());

  bool get _shouldUsePlatformView => _usesPlatformView(widget.renderMode);

  double? get _rendererAspectRatio {
    final renderer = _renderer;
    if (renderer != null && renderer is ValueListenable<rtc.RTCVideoValue>) {
      return (renderer as ValueListenable<rtc.RTCVideoValue>).value.aspectRatio;
    }
    return null;
  }

  Future<rtc.VideoRenderer?> _initializeRenderer() {
    final existing = _initialization;
    if (existing != null) return existing;
    final completion = Completer<rtc.VideoRenderer?>();
    _initialization = completion.future;
    unawaited(_createAndAttachRenderer().then(completion.complete,
        onError: (Object error, StackTrace stack) => completion.completeError(error, stack)));
    return completion.future;
  }

  Future<rtc.VideoRenderer?> _createAndAttachRenderer() async {
    if (!mounted || _shouldUsePlatformView) {
      return null;
    }
    // A leftover platform view controller is owned by its RTCVideoPlatFormView
    // widget, which disposes it on unmount. Only drop our reference here.
    if (_renderer != null && _renderer is! rtc.RTCVideoRenderer) {
      _releaseRenderer(dispose: false);
    }
    var renderer = _renderer;
    if (renderer == null) {
      final cachedRenderer = widget.cachedRenderer;
      if (cachedRenderer != null) {
        renderer = cachedRenderer;
        _renderer = renderer;
      } else {
        renderer = rtc.RTCVideoRenderer();
        _renderer = renderer;
      }
    }
    // A rebuild may observe the renderer while its first create is pending.
    // Every caller shares that initialization before attempting to bind tracks.
    await renderer.initialize();
    if (!mounted || !identical(_renderer, renderer)) return null;
    // Renderer creation and source binding have different failure semantics.
    // A temporary rejected source must not poison the cached initialization
    // Future: a later real stream event can bind to this initialized renderer.
    _observeAttachment(_attach());
    return mounted && identical(_renderer, renderer) ? renderer : null;
  }

  Future<void> _attachWhenReady() async {
    final renderer = _renderer;
    if (!mounted || renderer == null) return;
    if (renderer is rtc.RTCVideoRenderer) await renderer.initialize();
    if (!mounted || !identical(_renderer, renderer)) return;
    await _attach();
  }

  void setZoom(double zoomLevel) async {
    final videoTrack = _renderer?.srcObject!.getVideoTracks().first;
    if (videoTrack == null) return;
    await rtc.Helper.setZoom(videoTrack, zoomLevel);
  }

  void onViewFinderTap(TapDownDetails details, BoxConstraints constraints) {
    final videoTrack = _renderer?.srcObject!.getVideoTracks().first;
    if (videoTrack == null) return;

    final point = Point<double>(
      details.localPosition.dx / constraints.maxWidth,
      details.localPosition.dy / constraints.maxHeight,
    );

    // Don't wait here as it will slow down the UI unnecessarily.
    unawaited(rtc.Helper.setFocusPoint(videoTrack, point));
    unawaited(rtc.Helper.setExposurePoint(videoTrack, point));
  }

  /// Detaches the current renderer and drops our reference to it.
  /// Pass [dispose] only for renderers this widget created and owns.
  void _releaseRenderer({required bool dispose}) {
    ++_attachmentGeneration;
    ++_sourceGeneration;
    final renderer = _renderer;
    final listener = _listener;
    _listener = null;
    _renderer = null;
    _initialization = null;
    _listenedTrack = null;
    _requestedRenderer = null;
    _requestedTrack = null;
    _requestedStream = null;
    _requestedTrackId = null;
    _rendererReadyForWeb = false;
    renderer?.onResize = null;
    unawaited(() async {
      try {
        await runCleanupSteps('VideoTrackRenderer', {
          // Issue detach before awaiting listener cancellation: a cached
          // renderer may already be handed to a newer view during that await.
          'renderer': () async {
            if (renderer == null) return;
            if (dispose) {
              await renderer.dispose();
            } else if (renderer is rtc.RTCVideoRenderer) {
              await renderer.setSrcObject(stream: null);
            } else {
              renderer.srcObject = null;
            }
          },
          'listener': () async {
            await listener?.dispose();
          },
        });
      } catch (error, stack) {
        // Widget.dispose cannot await. Native ownership barriers retain failed
        // resources; observing the error here must not report a fake release.
        logger.warning('Renderer cleanup failed', error, stack);
      }
    }());
  }

  @override
  void initState() {
    super.initState();
    if (!_shouldUsePlatformView && widget.cachedRenderer != null) {
      _renderer = widget.cachedRenderer;
    }
    _viewRegistration = widget.track.addViewRegistration(pixelDensity: widget.adaptiveStreamPixelDensity);
    if (kIsWeb) _startWebRenderer();
  }

  void _startWebRenderer() {
    _observeAttachment(() async {
      final renderer = await _initializeRenderer();
      if (!mounted || renderer == null || !identical(_renderer, renderer)) return;
      setState(() => _rendererReadyForWeb = true);
    }());
  }

  @override
  void dispose() {
    widget.track.removeViewRegistration(_viewRegistration);
    if (widget.autoDisposeRenderer) {
      _releaseRenderer(dispose: true);
    } else {
      ++_attachmentGeneration;
      ++_sourceGeneration;
      final listener = _listener;
      _listener = null;
      _observeAttachment(() async {
        await listener?.dispose();
      }());
    }
    super.dispose();
  }

  Future<void> _attach() async {
    final renderer = _renderer;
    if (!mounted || renderer == null) return;
    final track = widget.track;
    // Only a real renderer/track replacement owns a new listener generation.
    // A layout/resize rebuild must never reset the native sink for the same
    // stream, and an in-flight request for that source is already sufficient.
    if (identical(_listenedTrack, track)) {
      await _bindStream(renderer, track, track.mediaStream);
      return;
    }
    final generation = ++_attachmentGeneration;
    _listenedTrack = track;
    bool current() =>
        mounted &&
        generation == _attachmentGeneration &&
        identical(_renderer, renderer) &&
        identical(widget.track, track);
    final previous = _listener;
    _listener = null;
    await previous?.dispose();
    if (!current()) return;
    _listener = track.createListener()
      ..on<TrackStreamUpdatedEvent>((event) {
        if (!current()) return;
        _observeAttachment(_bindStream(renderer, track, event.stream));
      })
      ..on<LocalTrackOptionsUpdatedEvent>((event) {
        if (!current()) return;
        // force recompute of mirror mode
        setState(() {});
      });
    renderer.onResize = () {
      if (current()) {
        setState(() {
          _aspectRatio = _rendererAspectRatio;
        });
      }
    };
    await _bindStream(renderer, track, track.mediaStream);
  }

  Future<void> _bindStream(rtc.VideoRenderer renderer, VideoTrack track, rtc.MediaStream stream) async {
    if (!mounted || !identical(_renderer, renderer) || !identical(widget.track, track)) return;
    final trackId = track.mediaStreamTrack.id;
    if (identical(_requestedRenderer, renderer) &&
        identical(_requestedTrack, track) &&
        identical(_requestedStream, stream) &&
        _requestedTrackId == trackId) return;
    // Record before awaiting: duplicate requests share the same desired source,
    // while a newer source may supersede a pending native request immediately.
    final generation = ++_sourceGeneration;
    _requestedRenderer = renderer;
    _requestedTrack = track;
    _requestedStream = stream;
    _requestedTrackId = trackId;
    try {
      if (renderer is rtc.RTCVideoRenderer) {
        await renderer.setSrcObject(stream: stream);
      } else {
        renderer.srcObject = stream;
      }
    } catch (_) {
      // Failed requests are not a successfully bound source. A later genuine
      // stream event can retry; no resize/rebuild loop is used as a retry timer.
      if (generation == _sourceGeneration && mounted && identical(_renderer, renderer)) {
        _requestedRenderer = null;
        _requestedTrack = null;
        _requestedStream = null;
        _requestedTrackId = null;
      }
      rethrow;
    }
  }

  void _observeAttachment(Future<void> operation) {
    unawaited(operation.catchError((Object error, StackTrace stack) {
      if (const bool.fromEnvironment('OPENIM_DEVELOPMENT')) {
        final code = error is PlatformException && RegExp(r'^[A-Z0-9_]{1,64}$').hasMatch(error.code)
            ? error.code
            : error.runtimeType.toString();
        // Fixed label and safe code only; never log native messages/URI/SDP.
        // ignore: avoid_print
        print('[OpenIMRendererAttachment] stage=source current=$mounted error=$code');
      }
      logger.warning('Renderer attachment failed', error, stack);
    }));
  }

  @override
  void didUpdateWidget(covariant VideoTrackRenderer oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (_usesPlatformView(oldWidget.renderMode) != _shouldUsePlatformView ||
        !identical(widget.cachedRenderer, oldWidget.cachedRenderer)) {
      // Only dispose texture renderers we created ourselves. Platform view
      // controllers belong to RTCVideoPlatFormView and a cachedRenderer
      // belongs to the caller, who may hand it back on a later switch.
      final ownsRenderer = _renderer is rtc.RTCVideoRenderer && !identical(_renderer, oldWidget.cachedRenderer);
      _aspectRatio = null;
      _releaseRenderer(dispose: ownsRenderer && oldWidget.autoDisposeRenderer);
      if (kIsWeb) _startWebRenderer();
    }

    if (widget.track != oldWidget.track) {
      oldWidget.track.removeViewRegistration(_viewRegistration);
      _viewRegistration = widget.track.addViewRegistration(pixelDensity: widget.adaptiveStreamPixelDensity);
      _observeAttachment(_attachWhenReady());
    } else if (widget.adaptiveStreamPixelDensity != oldWidget.adaptiveStreamPixelDensity) {
      _viewRegistration.pixelDensity = widget.adaptiveStreamPixelDensity;
    }

    if ([BrowserType.safari, BrowserType.firefox].contains(lkBrowser()) && oldWidget.key != widget.key) {
      _observeAttachment(_attachWhenReady());
    }
  }

  Widget _videoViewForWeb() => !_rendererReadyForWeb
      ? (widget.placeholderBuilder?.call(context) ?? const SizedBox.shrink())
      : Builder(
          key: _viewRegistration.key,
          builder: (ctx) {
            // let it render before notifying build
            WidgetsBindingCompatible.instance?.addPostFrameCallback((timeStamp) {
              widget.track.onVideoViewBuild?.call();
            });
            return rtc.RTCVideoView(
              _renderer! as rtc.RTCVideoRenderer,
              mirror: _shouldMirror(),
              filterQuality: FilterQuality.medium,
              objectFit: widget.fit.toRTCType(),
              placeholderBuilder: widget.placeholderBuilder,
            );
          },
        );

  Widget _videoRendererView() {
    if (_shouldUsePlatformView) {
      return rtc.RTCVideoPlatFormView(
        mirror: _shouldMirror(),
        objectFit: widget.fit.toRTCType(),
        onViewReady: (controller) async {
          _renderer = controller;
          _observeAttachment(_attach());
        },
      );
    }
    return rtc.RTCVideoView(
      _renderer! as rtc.RTCVideoRenderer,
      mirror: _shouldMirror(),
      filterQuality: FilterQuality.medium,
      objectFit: widget.fit.toRTCType(),
      placeholderBuilder: widget.placeholderBuilder,
    );
  }

  Widget _videoViewForNative() => FutureBuilder(
      future: _initializeRenderer(),
      builder: (context, snapshot) {
        if ((snapshot.hasData && _renderer != null) || _shouldUsePlatformView) {
          return Builder(
            key: _viewRegistration.key,
            builder: (ctx) {
              // let it render before notifying build
              WidgetsBindingCompatible.instance?.addPostFrameCallback((timeStamp) {
                widget.track.onVideoViewBuild?.call();
              });

              if (!lkPlatformIsMobile() || widget.track is! LocalVideoTrack) {
                return _videoRendererView();
              }
              return LayoutBuilder(
                builder: (BuildContext context, BoxConstraints constraints) {
                  return GestureDetector(
                    onScaleStart: (details) {},
                    onScaleUpdate: (details) {
                      if (details.scale != 1.0) {
                        setZoom(details.scale);
                      }
                    },
                    onTapDown: (TapDownDetails details) => onViewFinderTap(details, constraints),
                    child: _videoRendererView(),
                  );
                },
              );
            },
          );
        }
        return widget.placeholderBuilder?.call(context) ?? const SizedBox.shrink();
      });

  // FutureBuilder will cause flickering for flutter web. so using
  // different rendering methods for web and native.
  @override
  Widget build(BuildContext context) {
    final child = kIsWeb ? _videoViewForWeb() : _videoViewForNative();

    if (widget.fit == VideoViewFit.cover) {
      return child;
    }

    final videoView = LayoutBuilder(
      builder: (BuildContext context, BoxConstraints constraints) {
        if (!constraints.hasBoundedWidth && !constraints.hasBoundedHeight) {
          return child;
        }
        if (_aspectRatio == null) {
          return child;
        }

        bool fixHeight;
        if (!constraints.hasBoundedWidth) {
          fixHeight = true;
        } else if (!constraints.hasBoundedHeight) {
          fixHeight = false;
        } else {
          // both width and height are bound, figure out which to fix based on aspect ratios
          final constraintsAspectRatio = constraints.maxWidth / constraints.maxHeight;
          fixHeight = constraintsAspectRatio > _aspectRatio!;
        }
        final double width;
        final double height;
        if (fixHeight) {
          height = constraints.maxHeight;
          width = height * _aspectRatio!;
        } else {
          width = constraints.maxWidth;
          height = width / _aspectRatio!;
        }
        return SizedBox(width: width, height: height, child: child);
      },
    );

    if (widget.autoCenter) {
      return Center(child: videoView);
    } else {
      return videoView;
    }
  }

  bool _shouldMirror() {
    // off for screen share
    if (widget.track.source == TrackSource.screenShareVideo) return false;
    // on
    if (widget.mirrorMode == VideoViewMirrorMode.mirror) return true;
    // auto
    if (widget.mirrorMode == VideoViewMirrorMode.auto) {
      final track = widget.track;
      if (track is LocalVideoTrack) {
        final settings = track.mediaStreamTrack.getSettings();
        final facingMode = settings['facingMode'];
        if (facingMode != null) {
          return facingMode == 'user';
        }
        final options = track.currentOptions;
        if (options is CameraCaptureOptions) {
          // mirror if front camera
          return options.cameraPosition == CameraPosition.front;
        }
      }
    }
    // default to false
    return false;
  }
}
