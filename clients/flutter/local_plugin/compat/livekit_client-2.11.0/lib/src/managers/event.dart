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
import 'dart:collection';

import 'package:logging/logging.dart';
import 'package:meta/meta.dart';
import 'package:synchronized/synchronized.dart' as sync;

import '../events.dart';
import '../exceptions.dart';
import '../extensions.dart';
import '../logger.dart';
import '../support/disposable.dart';
import '../types/other.dart';

mixin EventsEmittable<T> {
  final events = EventsEmitter<T>();
  EventsListener<T> createListener({bool synchronized = false, Object? Function()? callbackEpoch}) =>
      EventsListener<T>(events, synchronized: synchronized, callbackEpoch: callbackEpoch);
}

// Type-safe, multi-listenable, dispose safe event handling
// TODO: Move to a separate package

/// An observed asynchronous event-handler failure. This is data, not a stream
/// error: an absent diagnostic subscriber must not create another zone error.
class EventCallbackError<T> {
  final T event;
  final Object error;
  final StackTrace stackTrace;
  final Object listener;
  final Object? epoch;

  const EventCallbackError(this.event, this.error, this.stackTrace, this.listener, this.epoch);
}

class EventsEmitter<T> extends EventsListenable<T> {
  // suppport for multiple event listeners
  final streamCtrl = StreamController<T>.broadcast(sync: false);
  final _callbackErrorCtrl = StreamController<EventCallbackError<T>>.broadcast();
  EventCallbackError<T>? _lastCallbackError;

  Stream<EventCallbackError<T>> get callbackErrors => _callbackErrorCtrl.stream;
  EventCallbackError<T>? get lastCallbackError => _lastCallbackError;

  void _recordCallbackError(T event, Object error, StackTrace stack, Object listener, Object? epoch) {
    final failure = EventCallbackError<T>(event, error, stack, listener, epoch);
    _lastCallbackError = failure;
    if (!_callbackErrorCtrl.isClosed) _callbackErrorCtrl.add(failure);
    // Do not log event payloads, exception messages, SDP, URLs or credentials.
    logger.warning('Event callback failed: ${event.runtimeType}/${error.runtimeType}');
  }

  bool _queueMode = false;
  final _queue = Queue<T>();

  EventsEmitter({
    bool listenSynchronized = false,
  }) : super(synchronized: listenSynchronized) {
    // clean up
    onDispose(() async {
      await streamCtrl.close();
      await _callbackErrorCtrl.close();
    });
  }

  @override
  EventsEmitter<T> get emitter => this;

  @internal
  void emit(T event) {
    // check if already disposed
    if (isDisposed) {
      logger.warning('failed to emit event ${event} on a disposed emitter');
      return;
    }

    if (logger.isLoggable(Level.FINEST)) {
      final scope = event is InternalEvent ? 'internal' : 'public';
      logger.finest('[${objectId}] emit ($scope) $event');
    }

    // queue mode
    if (_queueMode) {
      _queue.add(event);
      return;
    }
    // emit the event
    streamCtrl.add(event);
  }

  @internal
  void updateQueueMode(bool newValue, {bool shouldEmitQueued = true}) {
    // check if already disposed
    if (isDisposed) {
      logger.warning('failed to update queueMode on a disposed emitter');
      return;
    }
    if (_queueMode == newValue) return;
    _queueMode = newValue;
    if (!_queueMode && shouldEmitQueued) emitQueued();
  }

  @internal
  void emitQueued() {
    while (_queue.isNotEmpty) {
      final event = _queue.removeFirst();
      // emit the event
      streamCtrl.add(event);
    }
  }
}

// for listening only
class EventsListener<T> extends EventsListenable<T> {
  @override
  final EventsEmitter<T> emitter;

  EventsListener(
    this.emitter, {
    bool synchronized = false,
    Object? Function()? callbackEpoch,
  }) : super(
          synchronized: synchronized,
          callbackEpoch: callbackEpoch ?? emitter.callbackEpoch,
        );
}

// ensures all listeners will close on dispose
abstract class EventsListenable<T> extends Disposable {
  // the emitter to listen to
  EventsEmitter<T> get emitter;

  final bool synchronized;
  Object? Function()? callbackEpoch;
  // keep track of listeners to cancel later
  final _listeners = <StreamSubscription<T>>[];
  final _syncLock = sync.Lock();

  List<StreamSubscription<T>> get listeners => _listeners;

  EventsListenable({
    required this.synchronized,
    this.callbackEpoch,
  }) {
    onDispose(() async {
      await cancelAll();
    });
  }

  Future<void> cancelAll() async {
    if (_listeners.isNotEmpty) {
      // Stop listening to all events
      logger.finer('${objectId} cancelling ${_listeners.length} listeners(s)');
      final listenersCopy = List.of(_listeners);
      _listeners.clear();
      for (final listener in listenersCopy) {
        await listener.cancel();
      }
    }
  }

  // listens to all events, guaranteed to be cancelled on dispose
  CancelListenFunc listen(FutureOr<void> Function(T) onEvent) {
    final listener = emitter.streamCtrl.stream.listen((event) {
      Object? epoch;
      try {
        epoch = callbackEpoch?.call();
      } catch (error, stack) {
        emitter._recordCallbackError(event, error, stack, this, null);
        return;
      }
      Future<void> dispatch() async {
        // A synchronized callback may have waited behind an old handler while
        // a new connection was established. Do not run its destructive body in
        // the new epoch; checking only the eventual error would be too late.
        if (callbackEpoch != null && callbackEpoch!.call() != epoch) return;
        await onEvent(event);
      }
      // Stream.listen does not await an async onData callback. Observe the
      // actual returned Future, including the synchronized listener's lock.
      unawaited(Future<void>.sync(() => synchronized ? _syncLock.synchronized(dispatch) : dispatch()).then<void>(
        (_) {},
        onError: (Object error, StackTrace stack) {
          emitter._recordCallbackError(event, error, stack, this, epoch);
        },
      ));
    });
    _listeners.add(listener);

    // make a cancel func to cancel listening and remove from list in 1 call
    cancelFunc() async {
      await listener.cancel();
      _listeners.remove(listener);
      logger.fine('${objectId} event was cancelled by func');
    }

    return cancelFunc;
  }

  // convenience method to listen & filter a specific event type
  CancelListenFunc on<E>(
    FutureOr<void> Function(E) then, {
    bool Function(E)? filter,
  }) =>
      listen((event) async {
        // event must be E
        if (event is! E) return;
        // filter must be true (if filter is used)
        if (filter != null && !filter(event)) return;
        // cast to E
        await then(event);
      });

  /// convenience method to listen & filter a specific event type, just once.
  CancelListenFunc? once<E>(
    FutureOr<void> Function(E) then, {
    bool Function(E)? filter,
  }) {
    CancelListenFunc? cancelFunc;
    cancelFunc = listen((event) async {
      // event must be E
      if (event is! E) return;
      // filter must be true (if filter is used)
      if (filter != null && !filter(event)) return;
      // cast to E
      await then(event);
      // cancel after 1 event
      await cancelFunc?.call();
    });
    return cancelFunc;
  }

  // waits for a specific event type
  Future<E> waitFor<E>({
    required Duration duration,
    bool Function(E)? filter,
    FutureOr<E> Function()? onTimeout,
  }) async {
    final completer = Completer<E>();

    final cancelFunc = on<E>(
      (event) {
        if (!completer.isCompleted) {
          completer.complete(event);
        }
      },
      filter: filter,
    );

    try {
      // wait to complete with timeout
      return await completer.future.timeout(
        duration,
        onTimeout: onTimeout ?? () => throw TimeoutException(),
      );
      // do not catch exceptions and pass it up
    } finally {
      // always clean-up listener
      await cancelFunc.call();
    }
  }
}
