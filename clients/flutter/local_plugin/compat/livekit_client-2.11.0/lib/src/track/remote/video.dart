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

import 'package:collection/collection.dart';
import 'package:flutter/services.dart' show PlatformException;
import 'package:flutter_webrtc/flutter_webrtc.dart' as rtc;
import 'package:meta/meta.dart';

import '../../events.dart';
import '../../logger.dart';
import '../../stats/stats.dart';
import '../../types/other.dart';
import '../local/local.dart';
import 'remote.dart';

class RemoteVideoTrack extends RemoteTrack with VideoTrack {
  RemoteVideoTrack(TrackSource source, rtc.MediaStream stream, rtc.MediaStreamTrack track,
      {rtc.RTCRtpReceiver? receiver})
      : super(
          TrackType.VIDEO,
          source,
          stream,
          track,
          receiver: receiver,
        );

  VideoReceiverStats? prevStats;
  num? _currentBitrate;
  num? get currentBitrate => _currentBitrate;

  @internal
  String? getDecoderImplementation() {
    return prevStats?.decoderImplementation;
  }

  String? _developmentReceiverState;

  void _diagnoseReceiverState(String state) {
    if (const bool.fromEnvironment('OPENIM_DEVELOPMENT')) {
      if (_developmentReceiverState == state) return;
      _developmentReceiverState = state;
      // Local enum/code and opaque track ID only; no exception text or SDP.
      // ignore: avoid_print
      print('[OpenIMRtcReceiveState] track=${mediaStreamTrack.id} state=$state');
    }
  }

  String _safeStatsErrorCode(Object error) {
    if (error is PlatformException && RegExp(r'^[A-Z0-9_]{1,64}$').hasMatch(error.code)) {
      return error.code;
    }
    return error.runtimeType.toString();
  }

  @override
  Future<bool> monitorStats() async {
    if (receiver == null || events.isDisposed || !isActive) {
      _diagnoseReceiverState(receiver == null
          ? 'STOP_NO_RECEIVER'
          : events.isDisposed
              ? 'STOP_DISPOSED'
              : 'STOP_INACTIVE');
      _currentBitrate = 0;
      return false;
    }
    try {
      final stats = await getReceiverStats();
      _diagnoseReceiverState(stats == null ? 'NO_INBOUND_RTP' : 'SAMPLE');

      if (const bool.fromEnvironment('OPENIM_DEVELOPMENT')) {
        if (stats != null && isActive && !events.isDisposed) {
          // Counts from the already-completed receiver sample, not another JNI
          // request. No tokens, addresses, SDP or user profile data are logged.
          // ignore: avoid_print
          print('[OpenIMRtcReceive] track=${mediaStreamTrack.id} '
              'framesDecoded=${stats.framesDecoded} bytesReceived=${stats.bytesReceived}');
        }
      }

      if (stats != null && prevStats != null && receiver != null) {
        final bitrate = computeBitrateForReceiverStats(stats, prevStats);
        _currentBitrate = bitrate;
        events.emit(VideoReceiverStatsEvent(stats: stats, currentBitrate: bitrate));
      }

      prevStats = stats;
    } catch (e) {
      if (isStatsSamplePending(e)) {
        _diagnoseReceiverState('PENDING_${_safeStatsErrorCode(e)}');
        return true;
      }
      _diagnoseReceiverState('STOP_${_safeStatsErrorCode(e)}');
      logger.warning('Failed to monitor stats: $e');
      return false;
    }

    return true;
  }

  Future<VideoReceiverStats?> getReceiverStats() async {
    if (receiver == null) {
      return null;
    }

    late List<rtc.StatsReport> stats;
    try {
      stats = await receiver!.getStats();
    } catch (e) {
      rethrow;
    }

    VideoReceiverStats? receiverStats;
    for (var v in stats) {
      if (v.type == 'inbound-rtp') {
        receiverStats ??= VideoReceiverStats(v.id, v.timestamp);
        receiverStats.jitter = getNumValFromReport(v.values, 'jitter');
        receiverStats.jitterBufferDelay = getNumValFromReport(v.values, 'jitterBufferDelay');
        receiverStats.bytesReceived = getNumValFromReport(v.values, 'bytesReceived');
        receiverStats.packetsLost = getNumValFromReport(v.values, 'packetsLost');
        receiverStats.framesDecoded = getNumValFromReport(v.values, 'framesDecoded');
        receiverStats.framesDropped = getNumValFromReport(v.values, 'framesDropped');
        receiverStats.framesReceived = getNumValFromReport(v.values, 'framesReceived');
        receiverStats.packetsReceived = getNumValFromReport(v.values, 'packetsReceived');
        receiverStats.framesPerSecond = getNumValFromReport(v.values, 'framesPerSecond');
        receiverStats.frameWidth = getNumValFromReport(v.values, 'frameWidth');
        receiverStats.frameHeight = getNumValFromReport(v.values, 'frameHeight');
        receiverStats.pliCount = getNumValFromReport(v.values, 'pliCount');
        receiverStats.firCount = getNumValFromReport(v.values, 'firCount');
        receiverStats.nackCount = getNumValFromReport(v.values, 'nackCount');
        receiverStats.decoderImplementation = getStringValFromReport(v.values, 'decoderImplementation');

        final c = stats.firstWhereOrNull((element) => element.type == 'codec');
        if (c != null) {
          receiverStats.mimeType = getStringValFromReport(c.values, 'mimeType');
          receiverStats.payloadType = getNumValFromReport(c.values, 'payloadType');
          receiverStats.channels = getNumValFromReport(c.values, 'channels');
          receiverStats.clockRate = getNumValFromReport(c.values, 'clockRate');
        }
        break;
      }
    }

    return receiverStats;
  }
}
