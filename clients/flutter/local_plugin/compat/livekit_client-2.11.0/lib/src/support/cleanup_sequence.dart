import 'dart:async';

/// Run every owned-resource cleanup, then report the first failure to the caller.
/// No timeout here pretends a native operation has completed.
Future<void> runCleanupSteps(
  String owner,
  Map<String, FutureOr<void> Function()> steps,
) async {
  Object? firstError;
  StackTrace? firstStack;
  for (final step in steps.entries) {
    final clock = Stopwatch()..start();
    _trace('$owner/${step.key} start');
    try {
      await step.value();
      _trace('$owner/${step.key} complete ${clock.elapsedMilliseconds}ms');
    } catch (error, stack) {
      firstError ??= error;
      firstStack ??= stack;
      _trace('$owner/${step.key} failed ${clock.elapsedMilliseconds}ms ${error.runtimeType}');
    }
  }
  if (firstError != null) Error.throwWithStackTrace(firstError, firstStack!);
}

void _trace(String message) {
  if (const bool.fromEnvironment('OPENIM_DEVELOPMENT')) {
    // Stage names, elapsed time and error type only: never tokens or SDP.
    // ignore: avoid_print
    print('[OpenIMRtcCleanup] $message');
  }
}

/// Upstream Disposable logs and swallows callbacks' failures. RTC owners must
/// retain them so a subsequent dispose() cannot falsely claim native release.
mixin CleanupFailureReporting {
  Object? _cleanupFailure;
  StackTrace? _cleanupFailureStack;
  Future<bool>? _cleanupDisposal;

  Future<void> recordCleanup(Future<void> Function() operation) async {
    try {
      await operation();
    } catch (error, stack) {
      _cleanupFailure ??= error;
      _cleanupFailureStack ??= stack;
      rethrow;
    }
  }

  Future<bool> reportCleanupResult(Future<bool> Function() operation) {
    // The upstream owner sets isDisposed before awaiting native work. Repeated
    // callers must share actual completion, never get an immediate false result.
    final existing = _cleanupDisposal;
    if (existing != null) return existing;
    final completion = Completer<bool>();
    _cleanupDisposal = completion.future;
    Future<bool>.sync(operation).then((result) {
      final failure = _cleanupFailure;
      if (failure != null) {
        completion.completeError(failure, _cleanupFailureStack!);
      } else {
        completion.complete(result);
      }
    }, onError: (Object error, StackTrace stack) {
      completion.completeError(error, stack);
    });
    return completion.future;
  }
}

/// Share real in-progress cleanup. A failed task remains a failed barrier;
/// successful cleanup may run again after a subsequent reconnect.
class CleanupSingleFlight {
  Future<void>? _current;
  Future<void>? get current => _current;

  Future<void> run(Future<void> Function() operation) {
    final existing = _current;
    if (existing != null) return existing;
    final completion = Completer<void>();
    _current = completion.future;
    Future<void>.sync(operation).then((_) {
      if (identical(_current, completion.future)) _current = null;
      completion.complete();
    }, onError: (Object error, StackTrace stack) {
      completion.completeError(error, stack);
    });
    return completion.future;
  }
}
