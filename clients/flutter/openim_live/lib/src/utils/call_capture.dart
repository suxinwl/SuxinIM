import 'dart:async';

import 'package:flutter/services.dart' show PlatformException;
import 'package:livekit_client/livekit_client.dart';
import 'package:openim_common/openim_common.dart';

typedef CallCaptureErrorHandler = FutureOr<void> Function(
    Object error, StackTrace stack);

void _diagnoseCaptureFailure(String stage, Object error,
    {required bool current}) {
  if (!const bool.fromEnvironment('OPENIM_DEVELOPMENT')) return;
  final code = error is PlatformException &&
          RegExp(r'^[A-Z0-9_]{1,64}$').hasMatch(error.code)
      ? error.code
      : error.runtimeType.toString();
  // Fixed operation labels and safe codes only. No native message, SDP or URI.
  // ignore: avoid_print
  print('[OpenIMCallCapture] stage=$stage current=$current error=$code');
}

/// Observe a real capture operation without allowing late work to change a
/// different call. False means failure or a superseded owner, never success.
Future<bool> observeCallCapture({
  required String stage,
  required Future<void> Function() operation,
  required bool Function() isCurrent,
  CallCaptureErrorHandler? onError,
}) async {
  if (!isCurrent()) return false;
  try {
    await operation();
    return isCurrent();
  } catch (error, stack) {
    final current = isCurrent();
    _diagnoseCaptureFailure(stage, error, current: current);
    if (current) {
      try {
        if (onError != null) {
          await onError(error, stack);
        } else {
          // Capture failure is local to this attempt. The existing general
          // call onError closes the whole call, so do not route a camera
          // toggle failure through that fatal path.
          await IMViews.showToast(StrRes.mediaCaptureFailed);
        }
      } catch (handlerError) {
        // The failed operation remains false even if its UI error handler also
        // fails. That handler's Future is observed rather than leaking a zone error.
        _diagnoseCaptureFailure('$stage.errorHandler', handlerError,
            current: isCurrent());
      }
    }
    return false;
  }
}

/// Both operations use this exact participant. A camera failure is reported
/// before attempting audio; a closed/replaced call cannot start the next step.
Future<bool> publishCallMedia({
  required LocalParticipant participant,
  required bool cameraEnabled,
  required bool microphoneEnabled,
  required bool Function() isCurrent,
  CallCaptureErrorHandler? onError,
}) async {
  final camera = await observeCallCapture(
    stage: 'publish.camera',
    operation: () async {
      await participant.setCameraEnabled(cameraEnabled);
    },
    isCurrent: isCurrent,
    onError: onError,
  );
  if (!isCurrent()) return false;
  final microphone = await observeCallCapture(
    stage: 'publish.microphone',
    operation: () async {
      await participant.setMicrophoneEnabled(microphoneEnabled);
    },
    isCurrent: isCurrent,
    onError: onError,
  );
  return camera && microphone && isCurrent();
}
