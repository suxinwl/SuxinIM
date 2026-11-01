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

import 'package:flutter/foundation.dart' show kIsWeb;
import 'package:flutter/services.dart' show PlatformException;

import 'package:flutter_webrtc/flutter_webrtc.dart' as rtc;
import 'package:meta/meta.dart';
import 'package:uuid/uuid.dart';

import '../events.dart';
import '../extensions.dart';
import '../internal/events.dart';
import '../logger.dart';
import '../managers/event.dart';
import '../stats/stats.dart';
import '../support/disposable.dart';
import '../support/cleanup_sequence.dart';
import '../types/other.dart';

/// Wrapper around a MediaStreamTrack with additional metadata.
/// Base for [AudioTrack] and [VideoTrack],
/// can not be instantiated directly.
abstract class Track extends DisposableChangeNotifier with EventsEmittable<TrackEvent>, CleanupFailureReporting {
  @override
  Future<bool> dispose() => reportCleanupResult(super.dispose);

  final _stopCleanup = CleanupSingleFlight();
  static const uuid = Uuid();
  final TrackType kind;
  final TrackSource source;

  // read only
  rtc.MediaStream get mediaStream => _mediaStream;
  rtc.MediaStream _mediaStream;

  // read only
  rtc.MediaStreamTrack get mediaStreamTrack => _mediaStreamTrack;
  rtc.MediaStreamTrack _mediaStreamTrack;

  rtc.MediaStreamTrack? _originalTrack;

  String? sid;
  rtc.RTCRtpTransceiver? transceiver;
  String? _cid;

  // started / stopped
  bool _active = false;
  bool get isActive => _active;

  bool _muted = false;
  bool get muted => _muted;

  rtc.RTCRtpSender? get sender => transceiver?.sender;

  rtc.RTCRtpReceiver? receiver;

  Track(this.kind, this.source, this._mediaStream, this._mediaStreamTrack, {this.receiver}) {
    // Any event emitted will trigger ChangeNotifier
    events.listen((event) {
      logger.finer('[TrackEvent] $event, will notifyListeners()');
      notifyListeners();
    });

    onDispose(() => recordCleanup(() => runCleanupSteps('Track.dispose', {
          'stop': () async {
            await stop();
          },
          'events': () async {
            await events.dispose();
          },
        })));
  }

  rtc.RTCRtpMediaType get mediaType {
    switch (kind) {
      case TrackType.AUDIO:
        return rtc.RTCRtpMediaType.RTCRtpMediaTypeAudio;
      case TrackType.VIDEO:
        return rtc.RTCRtpMediaType.RTCRtpMediaTypeVideo;
      // this should never happen
      default:
        return rtc.RTCRtpMediaType.RTCRtpMediaTypeAudio;
    }
  }

  String getCid() {
    var cid = _cid ?? mediaStreamTrack.id;

    if (cid == null) {
      cid = uuid.v4();
      _cid = cid;
    }
    return cid;
  }

  /// Start this [Track] if not started.
  /// Returns true if started, false if already started
  @mustCallSuper
  Future<bool> start() async {
    if (_active) {
      // already started
      return false;
    }

    logger.fine('$objectId.start()');

    await startCapture();

    _active = true;
    startMonitor();
    return true;
  }

  /// Stop this [Track] if not stopped.
  /// Returns true if stopped, false if already stopped
  @mustCallSuper
  Future<bool> stop() async {
    if (!_active) return false;
    await _stopCleanup.run(() => runCleanupSteps('Track.stop', {
          'monitor': stopMonitor,
          'capture': stopCapture,
          if (!kIsWeb)
            'nativeTrack': () async {
              await mediaStreamTrack.stop();
            },
          'originalTrack': () async {
            final original = _originalTrack;
            await original?.stop();
            if (identical(_originalTrack, original)) _originalTrack = null;
          },
        }));
    _active = false;
    return true;
  }

  Future<void> enable() async {
    logger.fine('$objectId.enable() enabling ${mediaStreamTrack.objectId}...');
    try {
      if (_active) {
        mediaStreamTrack.enabled = true;
      }
    } catch (e) {
      logger.warning('[$objectId] set rtc.mediaStreamTrack.enabled did throw $e');
    }
  }

  Future<void> disable() async {
    logger.fine('$objectId.disable() disabling ${mediaStreamTrack.objectId}...');
    try {
      if (_active) {
        mediaStreamTrack.enabled = false;
      }
    } catch (e) {
      logger.warning('[$objectId] set rtc.mediaStreamTrack.enabled did throw $e');
    }
  }

  Timer? _monitorTimer;
  bool _monitorInFlight = false;
  int _monitorGeneration = 0;

  @internal
  bool isStatsSamplePending(Object error) =>
      error is PlatformException && (error.code == 'RTC_STATS_BUSY' || error.code == 'RTC_STATS_SELECTOR_UNAVAILABLE');

  @internal
  Future<bool> monitorStats();

  /// Called by [start] before this track is marked active.
  @internal
  Future<void> startCapture() async {}

  /// Called by [stop] while this track is still active, before the media track
  /// is stopped.
  @internal
  Future<void> stopCapture() async {}

  @internal
  void startMonitor() {
    if (_monitorTimer != null) return;
    final generation = ++_monitorGeneration;
    _monitorTimer ??= Timer.periodic(const Duration(milliseconds: monitorFrequency), (_) async {
      if (_monitorInFlight) return;
      _monitorInFlight = true;
      try {
        if (!await monitorStats() && generation == _monitorGeneration) stopMonitor();
      } catch (error) {
        logger.warning('Track stats monitor failed: $error');
        if (generation == _monitorGeneration) stopMonitor();
      } finally {
        _monitorInFlight = false;
      }
    });
  }

  @internal
  void stopMonitor() {
    _monitorGeneration++;
    _monitorTimer?.cancel();
    _monitorTimer = null;
  }

  @internal
  void updateMuted(
    bool muted, {
    bool shouldNotify = true,
    bool shouldSendSignal = false,
  }) {
    if (_muted == muted) return;
    _muted = muted;
    if (shouldNotify) {
      events.emit(InternalTrackMuteUpdatedEvent(
        track: this,
        muted: muted,
        shouldSendSignal: shouldSendSignal,
      ));
    }
  }

  @internal
  void updateMediaStreamAndTrack(rtc.MediaStream stream, rtc.MediaStreamTrack track) {
    _mediaStream = stream;
    _mediaStreamTrack = track;
    events.emit(TrackStreamUpdatedEvent(
      track: this,
      stream: stream,
    ));
  }

  @internal
  void setProcessedTrack(rtc.MediaStreamTrack track) {
    _originalTrack = _mediaStreamTrack;
    _mediaStreamTrack = track;
  }
}
