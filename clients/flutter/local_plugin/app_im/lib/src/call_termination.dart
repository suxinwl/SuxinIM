import 'dart:async';
import 'dart:convert';

import 'package:dio/dio.dart';

enum CallTerminationStatus { pending, confirmed, failed, cancelled }

/// The result contains no credentials. Local media shutdown is independent of
/// this remotely confirmed result.
class CallTerminationResult {
  final String roomID;
  final String method;
  final CallTerminationStatus status;
  final int attempts;
  final String? errorType;
  const CallTerminationResult(
    this.roomID,
    this.method,
    this.status,
    this.attempts, [
    this.errorType,
  ]);
}

class CallTerminationSession {
  final String baseUrl, userID, imToken, chatToken;
  final int platformID, generation;
  const CallTerminationSession(
    this.baseUrl,
    this.userID,
    this.imToken,
    this.chatToken,
    this.platformID,
    this.generation,
  );
}

typedef CallTerminationRequest =
    Future<Map<String, dynamic>> Function(
      CallTerminationSession session,
      String path,
      Map<String, dynamic> body,
      CancelToken cancelToken,
    );

class _Termination {
  final CallTerminationSession session;
  final String roomID, method;
  final Map<String, dynamic> info;
  final cancel = CancelToken();
  final cancelled = Completer<void>();
  Future<CallTerminationResult>? flight;
  CallTerminationResult? result;
  bool retryable = true;
  _Termination(this.session, this.roomID, this.method, this.info);
  void invalidate() {
    if (!cancel.isCancelled) cancel.cancel('Call session ended');
    if (!cancelled.isCompleted) cancelled.complete();
  }
}

/// Owns a bounded set of idempotent remote termination tasks. Every retry uses
/// the captured pair; replacing a login cancels the old task rather than giving
/// an old room access to a new account's credentials.
class CallTerminationCoordinator {
  final CallTerminationRequest request;
  final bool Function(CallTerminationSession) current;
  final List<Duration> retryDelays;
  final _jobs = <String, _Termination>{};
  final changes = StreamController<CallTerminationResult>.broadcast();

  CallTerminationCoordinator({
    required this.request,
    required this.current,
    this.retryDelays = const [Duration(seconds: 2), Duration(seconds: 5)],
  });

  Future<CallTerminationResult> terminate(
    CallTerminationSession session,
    String method,
    Map<String, dynamic> info,
  ) {
    if (method != 'cancel' && method != 'hungUp') {
      throw ArgumentError.value(method, 'method');
    }
    final roomID = (info['invitation']?['roomID'] ?? info['roomID']) as String?;
    if (roomID == null || roomID.isEmpty) {
      return Future.value(
        CallTerminationResult(
          '',
          method,
          CallTerminationStatus.failed,
          0,
          'MissingRoomID',
        ),
      );
    }
    final key = '${session.generation}:${session.userID}:$roomID:$method';
    final existing = _jobs[key];
    if (existing != null) return _start(existing);
    _jobs.removeWhere(
      (_, job) =>
          job.result?.status == CallTerminationStatus.confirmed ||
          job.result?.status == CallTerminationStatus.cancelled,
    );
    if (_jobs.length >= 32) {
      return Future.value(
        CallTerminationResult(
          roomID,
          method,
          CallTerminationStatus.failed,
          0,
          'TerminationQueueFull',
        ),
      );
    }
    final immutableInfo = Map<String, dynamic>.from(
      jsonDecode(jsonEncode(info)),
    );
    immutableInfo['userID'] = session.userID;
    final job = _Termination(session, roomID, method, immutableInfo);
    _jobs[key] = job;
    return _start(job);
  }

  Future<CallTerminationResult> _start(_Termination job) {
    if (job.flight != null) return job.flight!;
    if (job.result?.status == CallTerminationStatus.confirmed ||
        job.result?.status == CallTerminationStatus.cancelled ||
        (!job.retryable && job.result != null)) {
      return Future.value(job.result!);
    }
    final completion = Completer<CallTerminationResult>();
    job.flight = completion.future;
    void finish(CallTerminationResult result) {
      job.result = result;
      job.flight = null;
      completion.complete(result);
      if (!changes.isClosed) changes.add(result);
    }

    // Observe guard/owner failures as well as HTTP errors: no public Future is
    // allowed to remain pending because its observation wrapper threw.
    unawaited(
      _run(job).then<void>(
        finish,
        onError: (Object error, StackTrace stack) => finish(
          CallTerminationResult(
            job.roomID,
            job.method,
            CallTerminationStatus.failed,
            0,
            error.runtimeType.toString(),
          ),
        ),
      ),
    );
    return completion.future;
  }

  bool _valid(_Termination job) =>
      !job.cancel.isCancelled && current(job.session);
  bool _unauthorized(Object error) =>
      error is DioException &&
      (error.response?.statusCode == 401 || error.response?.statusCode == 403);

  Future<CallTerminationResult> _run(_Termination job) async {
    var attempts = 0;
    Object? lastError;
    CallTerminationResult result(CallTerminationStatus status) =>
        CallTerminationResult(
          job.roomID,
          job.method,
          status,
          attempts,
          lastError?.runtimeType.toString(),
        );
    changes.add(result(CallTerminationStatus.pending));
    for (var attempt = 0; attempt <= retryDelays.length; attempt++) {
      if (!_valid(job)) return result(CallTerminationStatus.cancelled);
      attempts++;
      try {
        await request(job.session, '/signaling/signal_message_assemble', {
          'signalReq': {job.method: job.info},
        }, job.cancel);
        return result(
          _valid(job)
              ? CallTerminationStatus.confirmed
              : CallTerminationStatus.cancelled,
        );
      } catch (error) {
        lastError = error;
        if (!_valid(job)) return result(CallTerminationStatus.cancelled);
        if (_unauthorized(error)) {
          job.retryable = false;
          return result(CallTerminationStatus.failed);
        }
      }
      // A timed-out POST can already have committed. Query authoritative state
      // before retrying; a group participant leaving need not end the room.
      try {
        final state = await request(
          job.session,
          '/signaling/get_signal_invitation_info',
          {'roomID': job.roomID},
          job.cancel,
        );
        if (!_valid(job)) return result(CallTerminationStatus.cancelled);
        if (state['ended'] == true ||
            (state['participantState'] is Map &&
                state['participantState'][job.session.userID] == 'left')) {
          return result(CallTerminationStatus.confirmed);
        }
      } catch (error) {
        lastError = error;
        if (!_valid(job)) return result(CallTerminationStatus.cancelled);
        if (_unauthorized(error)) {
          job.retryable = false;
          return result(CallTerminationStatus.failed);
        }
      }
      if (attempt < retryDelays.length) {
        await Future.any<void>([
          Future<void>.delayed(retryDelays[attempt]),
          job.cancelled.future,
        ]);
      }
    }
    return result(CallTerminationStatus.failed);
  }

  /// Called on foreground recovery. Failed tasks retain their original pair.
  Future<List<CallTerminationResult>> retryPending() => Future.wait([
    for (final job in _jobs.values.toList())
      if (job.retryable &&
          job.result?.status != CallTerminationStatus.confirmed &&
          job.result?.status != CallTerminationStatus.cancelled)
        _start(job),
  ]);

  void invalidate() {
    for (final job in _jobs.values) {
      job.invalidate();
    }
    _jobs.clear();
  }
}
