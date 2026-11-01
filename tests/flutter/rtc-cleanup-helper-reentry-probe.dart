// Independent pure-Dart probe: imports only the actual cleanup helper.
// It does not create a Room, Flutter engine, device, or native resource.
import 'dart:async';
import 'dart:convert';

import '../../clients/flutter/local_plugin/compat/livekit_client-2.11.0/lib/src/support/cleanup_sequence.dart';

class ReentrantOwner with CleanupFailureReporting {
  bool started = false;
  final native = Completer<void>();
  Future<bool>? reentered;

  Future<bool> dispose() => reportCleanupResult(() async {
        // Same ordering as Disposable._dispose: set disposed before callbacks.
        if (started) return false;
        started = true;
        // A synchronous disposal callback re-enters before the first await.
        reentered = dispose();
        await native.future;
        return true;
      });
}

Future<void> main() async {
  final owner = ReentrantOwner();
  final outer = owner.dispose();
  var reenteredFinished = false;
  bool? reenteredValue;
  owner.reentered!.then((value) {
    reenteredFinished = true;
    reenteredValue = value;
  });
  await Future<void>.delayed(Duration.zero);
  final observation = {
    'scope': 'Pure-Dart helper probe; no RTC/native/runtime acceptance',
    'reenteredIsSameFuture': identical(outer, owner.reentered),
    'reenteredFinishedBeforeNativeCompleted': reenteredFinished,
    'reenteredValueBeforeNativeCompleted': reenteredValue,
    'nativeCompletedAtObservation': owner.native.isCompleted,
  };
  owner.native.complete();
  observation['outerResultAfterNativeCompletion'] = await outer;
  print(jsonEncode(observation));
}
