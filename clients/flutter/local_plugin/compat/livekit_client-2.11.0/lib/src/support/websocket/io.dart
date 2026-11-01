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
import 'dart:io' as io;

import '../../exceptions.dart';
import '../../logger.dart';
import '../../options.dart';
import '../http_client/io.dart';
import '../websocket.dart';
import '../cleanup_sequence.dart';

Future<LiveKitWebSocketIO> lkWebSocketConnect(
  Uri uri, {
  WebSocketEventHandlers? options,
  Map<String, String>? headers,
  NetworkOptions? networkOptions = const NetworkOptions(),
}) =>
    LiveKitWebSocketIO.connect(uri, options: options, headers: headers, networkOptions: networkOptions);

class LiveKitWebSocketIO extends LiveKitWebSocket {
  final io.WebSocket _ws;
  final io.HttpClient _client;
  final WebSocketEventHandlers? options;
  final Zone _ioZone = Zone.current;
  final Zone _callerZone;
  final Completer<bool> _disposal;
  late final StreamSubscription _subscription;
  final int _epoch;
  static int _nextEpoch = 0;
  final Stopwatch _clock = Stopwatch()..start();
  bool _closing = false;
  bool _physicallyClosed = false;
  bool _notifiedDone = false;
  bool _disposalStarted = false;
  bool _firstErrorDelivered = false;
  Object? _failure;
  StackTrace? _failureStack;

  LiveKitWebSocketIO._(
    this._ws,
    this._client,
    this._epoch,
    this._callerZone, [
    this.options,
  ]) : _disposal = _callerZone.run(() => Completer<bool>()) {
    // Keep the public failed future observed even when server-close initiates
    // cleanup and no external caller has reached dispose yet. Awaiters still
    // receive the same original error from this canonical future.
    _callerZone.run(() {
      unawaited(_disposal.future.then<void>((_) {}, onError: (Object error, StackTrace stack) {
        _trace('publicCloseFailed', error);
      }));
    });
    _subscription = _ws.listen(
      (dynamic data) {
        if (_closing || isDisposed) return;
        _observeCallback('onData', () => options?.onData?.call(data));
      },
      onError: (Object error, StackTrace stack) {
        _error(error, stack, 'stream');
      },
      onDone: () {
        // Do not make stream completion wait for a user callback which may
        // itself await dispose(). The physical socket has its own barrier.
        _observeShutdown();
      },
    );

    onDispose(() => recordCleanup(() async {
          _closing = true;
          _trace('closeBegin');
          await runCleanupSteps('LiveKitWebSocketIO', {
            'socketClose': () async {
              // Even when remote close changed readyState first, close() must
              // complete the outgoing consumer. readyState is not its barrier.
              await _ws.close();
            },
            'socketDone': () async {
              try {
                await _ws.done;
              } finally {
                // A completed done future plus the closed state establish the
                // lifetime boundary even when done reports a transport error.
                // An unfinished/unknown socket keeps its client ownership.
                if (_ws.readyState == io.WebSocket.closed) {
                  _physicallyClosed = true;
                  _trace('socketClosed');
                }
              }
            },
            'subscription': () async {
              await _subscription.cancel();
            },
            'httpClient': () {
              // Never close the TLS client immediately after the upgrade. Its
              // ownership lasts until the actual WebSocket close has finished.
              if (_physicallyClosed) _client.close();
            },
          });
          final failure = _failure;
          if (failure != null) Error.throwWithStackTrace(failure, _failureStack!);
        }));
  }

  @override
  // super.dispose is invoked by _startDisposal in the owning IO zone, then
  // explicitly bridged. Calling/awaiting it in the caller zone is unsafe.
  // ignore: must_call_super
  Future<bool> dispose() {
    _startDisposal();
    return _disposal.future;
  }

  void _startDisposal() {
    if (_disposalStarted) return;
    _disposalStarted = true;
    // All awaits on WebSocket-owned futures remain in the IO error zone.
    // Only the result crosses to the caller through an explicit completer.
    _ioZone.run(() {
      unawaited(super.dispose().then<void>((value) {
        _disposal.complete(value);
        _notifyDone();
      }, onError: (Object error, StackTrace stack) {
        _disposal.completeError(error, stack);
        _notifyDone();
      }));
    });
  }

  void _trace(String phase, [Object? error]) {
    if (const bool.fromEnvironment('OPENIM_DEVELOPMENT')) {
      // No URI, message contents, credentials or exception messages.
      // ignore: avoid_print
      print('[OpenIMRtcSocket] role=signalIO epoch=$_epoch phase=$phase elapsedMs=${_clock.elapsedMilliseconds}'
          '${error == null ? '' : ' type=${error.runtimeType}'}');
    }
  }

  void _observeCallback(String phase, dynamic Function() callback) {
    // Business callbacks and their returned futures belong to the caller's
    // error zone, never to the raw-socket guarded zone.
    _callerZone.run(() {
      unawaited(Future<dynamic>.sync(callback).then<void>((_) {}, onError: (Object error, StackTrace stack) {
        _error(error, stack, phase);
      }));
    });
  }

  void _notifyDone() {
    if (_notifiedDone) return;
    _notifiedDone = true;
    _observeCallback('onDispose', () => options?.onDispose?.call());
  }

  void _observeShutdown() {
    _startDisposal();
  }

  bool _isExpectedClosedRead(Object error, StackTrace stack) {
    if (!_closing ||
        !_physicallyClosed ||
        error is! io.SocketException ||
        error.message != 'Reading from a closed socket') {
      return false;
    }
    final frames = stack.toString().split('\n');
    // Exact SDK-owned leading frames only. A business SocketException has no
    // such provenance and remains an unexpected error, including after close.
    return frames.length >= 2 &&
        frames[0].contains('_RawSecureSocket.read (dart:io/secure_socket.dart:') &&
        frames[1].contains('_Socket._onData (dart:io-patch/socket_patch.dart:');
  }

  void _zoneError(Object error, StackTrace stack) {
    if (_isExpectedClosedRead(error, stack)) {
      _trace('expectedSDKReadAfterPhysicalClose', error);
      return;
    }
    _error(error, stack, 'ownedZone');
  }

  void _error(Object error, StackTrace stack, String origin) {
    final alreadyDelivered = _firstErrorDelivered && identical(_failure, error);
    _failure ??= error;
    _failureStack ??= stack;
    _trace('error.$origin', error);
    logger.warning('[WebSocketIO] epoch=$_epoch origin=$origin errorType=${error.runtimeType}');
    // Observe both synchronous and Future failures. Never recursively send a
    // failed error handler back to itself, or replace the original error.
    if (!alreadyDelivered) {
      if (identical(_failure, error)) _firstErrorDelivered = true;
      _callerZone.run(() {
        unawaited(Future<dynamic>.sync(() => options?.onError?.call(error)).then<void>((_) {},
            onError: (Object callbackError, StackTrace callbackStack) {
          _trace('onErrorFailed', callbackError);
          logger.warning('[WebSocketIO] epoch=$_epoch onErrorFailed type=${callbackError.runtimeType}');
        }));
      });
    }
    _observeShutdown();
  }

  @override
  void send(List<int> data) {
    if (_ws.readyState != io.WebSocket.open) {
      _trace('sendNotOpen');
      return;
    }

    try {
      _ws.add(data);
    } catch (error, stack) {
      _error(error, stack, 'send');
    }
  }

  static Future<LiveKitWebSocketIO> connect(
    Uri uri, {
    WebSocketEventHandlers? options,
    Map<String, String>? headers,
    NetworkOptions? networkOptions = const NetworkOptions(),
  }) {
    final callerZone = Zone.current;
    final epoch = ++_nextEpoch;
    logger.fine(
        '[WebSocketIO] Connecting ${Uri(scheme: uri.scheme, host: uri.host, port: uri.hasPort ? uri.port : null, path: uri.path)} epoch=$epoch');
    final resolvedNetworkOptions = networkOptions ?? const NetworkOptions();
    // This future is deliberately created in the caller's error zone. Every
    // connect failure is bridged explicitly, so crossing the per-connection
    // guarded zone cannot leave the caller waiting for an unreachable error.
    final completion = Completer<LiveKitWebSocketIO>();
    LiveKitWebSocketIO? owner;
    io.HttpClient? client;
    Future<void> open() async {
      try {
        client = createSdkIoHttpClient(resolvedNetworkOptions);
        final ws = await io.WebSocket.connect(uri.toString(), headers: headers, customClient: client);
        owner = LiveKitWebSocketIO._(ws, client!, epoch, callerZone, options);
        logger.fine('[WebSocketIO] Connected epoch=$epoch');
        if (completion.isCompleted) {
          // An IO-zone startup error may have already rejected the caller.
          // A later successful upgrade still owns a socket and must release it.
          owner!._startDisposal();
        } else {
          completion.complete(owner!);
        }
      } catch (error, stack) {
        client?.close();
        if (!completion.isCompleted) {
          completion.completeError(
              error is CertificatePinningException ? error : WebSocketException('Failed to connect', error), stack);
        }
      }
    }

    runZonedGuarded<void>(() {
      unawaited(open());
    }, (Object error, StackTrace stack) {
      final socket = owner;
      if (socket != null) {
        socket._zoneError(error, stack);
      } else if (!completion.isCompleted) {
        client?.close();
        completion.completeError(
            error is CertificatePinningException ? error : WebSocketException('Failed to connect', error), stack);
      } else {
        logger.warning('[WebSocketIO] epoch=$epoch unowned startup error type=${error.runtimeType}');
      }
    });
    return completion.future;
  }
}
