// Real loopback TLS/WebSocket transport tests. No device, production endpoint,
// private fixture, TLS bypass, or embedded private key is involved.
import 'dart:async';
import 'dart:io' as io;

import 'package:flutter_test/flutter_test.dart';
import 'package:livekit_client/src/options.dart';
import 'package:livekit_client/src/support/websocket.dart';

late io.Directory _material;
late NetworkOptions _trusted;

class _Loopback {
  _Loopback._(this.server);
  final io.HttpServer server;
  final accepted = <io.WebSocket>[];
  final serverErrors = <Object>[];
  final _nextPeer = Completer<io.WebSocket>();
  late StreamSubscription<io.HttpRequest> subscription;

  static Future<_Loopback> start() async {
    final context = io.SecurityContext()
      ..useCertificateChain('${_material.path}/loopback-certificate.pem')
      ..usePrivateKey('${_material.path}/loopback-key.pem');
    final result = _Loopback._(await io.HttpServer.bindSecure(
        io.InternetAddress.loopbackIPv4, 0, context));
    result.subscription = result.server.listen((request) async {
      try {
        final socket = await io.WebSocketTransformer.upgrade(request);
        result.accepted.add(socket);
        if (!result._nextPeer.isCompleted) result._nextPeer.complete(socket);
        socket.listen((data) {
          if (socket.readyState == io.WebSocket.open) socket.add(data);
        }, onError: (Object error, StackTrace stack) {
          result.serverErrors.add(error);
        });
      } catch (error) {
        result.serverErrors.add(error);
      }
    }, onError: (Object error, StackTrace stack) {
      // A rejected, untrusted TLS client is expected to fail the server handshake.
      result.serverErrors.add(error);
    });
    return result;
  }

  Uri get uri => Uri.parse('wss://127.0.0.1:${server.port}/socket');
  Future<io.WebSocket> get peer => _nextPeer.future;

  Future<void> dispose() async {
    Object? closeError;
    for (final peer in accepted) {
      try {
        await peer.close().timeout(const Duration(seconds: 3));
      } catch (error) {
        closeError ??= error;
      }
    }
    try {
      await server.close(force: true);
    } finally {
      await subscription.cancel();
    }
    if (closeError != null) throw closeError!;
  }
}

// Bridge across the deliberately isolated error zone so a callback failure
// cannot make this test hang. All caught zone errors are asserted, never ignored.
Future<List<Object>> _zoneErrors(Future<void> Function() operation) async {
  final complete = Completer<void>();
  final failures = <Object>[];
  runZonedGuarded(() async {
    try {
      await operation();
      // Allow queued close/read callbacks to run after physical socket closure.
      await Future<void>.delayed(const Duration(milliseconds: 40));
      complete.complete();
    } catch (error, stack) {
      complete.completeError(error, stack);
    }
  }, (Object error, StackTrace stack) {
    failures.add(error);
  });
  await complete.future.timeout(const Duration(seconds: 15));
  return failures;
}

void main() {
  setUpAll(() async {
    _material = await io.Directory.systemTemp.createTemp('lk-io-106-');
    final helper =
        io.File('../../tests/flutter/create_ephemeral_loopback_tls.py')
            .absolute
            .path;
    final generated = await io.Process.run(
        io.Platform.environment['OPENIM_TEST_PYTHON'] ?? 'python',
        [helper, _material.path]);
    if (generated.exitCode != 0) {
      await _material.delete(recursive: true);
      throw StateError('Ephemeral loopback TLS generation failed: '
          '${generated.exitCode}; ${generated.stderr}');
    }
    final certificate =
        await io.File('${_material.path}/loopback-certificate.pem')
            .readAsBytes();
    _trusted = NetworkOptions(
        certificatePinning: CertificatePinningOptions(rules: [
      CertificatePinningRule(
          hosts: const ['127.0.0.1'],
          trustedCertificates: [CertificateBytes.pem(certificate)],
          pinnedLeafCertificates: [CertificateBytes.pem(certificate)])
    ]));
  });

  tearDownAll(() async {
    // Only the Directory.createTemp return value is recursively removed.
    if (_material.existsSync()) await _material.delete(recursive: true);
  });

  test('real IO websocket verifies loopback TLS and exchanges binary payload',
      () async {
    final errors = await _zoneErrors(() async {
      final server = await _Loopback.start();
      LiveKitWebSocket? client;
      final received = Completer<dynamic>();
      final streamErrors = <dynamic>[];
      try {
        client = await LiveKitWebSocket.connect(server.uri,
            networkOptions: _trusted,
            options: WebSocketEventHandlers(
                onData: (data) {
                  if (!received.isCompleted) received.complete(data);
                },
                onError: streamErrors.add));
        client.send([7, 10, 6]);
        expect(await received.future.timeout(const Duration(seconds: 3)),
            orderedEquals([7, 10, 6]));
        expect(streamErrors, isEmpty);
        expect(server.serverErrors, isEmpty);
      } finally {
        await client?.dispose();
        await server.dispose();
      }
    });
    expect(errors, isEmpty);
  });

  test('untrusted real TLS rejection settles caller Future across IO zone',
      () async {
    final errors = await _zoneErrors(() async {
      final server = await _Loopback.start();
      try {
        await expectLater(
            LiveKitWebSocket.connect(server.uri)
                .timeout(const Duration(seconds: 3)),
            throwsA(isA<WebSocketException>()));
        expect(server.accepted, isEmpty);
      } finally {
        await server.dispose();
      }
    });
    expect(errors, isEmpty);
  });

  test('refused real loopback connection settles without unobserved Future',
      () async {
    final errors = await _zoneErrors(() async {
      final reserve =
          await io.ServerSocket.bind(io.InternetAddress.loopbackIPv4, 0);
      final port = reserve.port;
      await reserve.close();
      await expectLater(
          LiveKitWebSocket.connect(Uri.parse('wss://127.0.0.1:$port/socket'),
                  networkOptions: _trusted)
              .timeout(const Duration(seconds: 3)),
          throwsA(isA<WebSocketException>()));
    });
    expect(errors, isEmpty);
  });

  test('real TLS simultaneous server/client close retains shared completion',
      () async {
    final errors = await _zoneErrors(() async {
      for (var attempt = 0; attempt < 8; attempt++) {
        final server = await _Loopback.start();
        LiveKitWebSocket? client;
        final streamErrors = <dynamic>[];
        final data = Completer<dynamic>();
        try {
          client = await LiveKitWebSocket.connect(server.uri,
              networkOptions: _trusted,
              options: WebSocketEventHandlers(
                  onData: (value) {
                    if (!data.isCompleted) data.complete(value);
                  },
                  onError: streamErrors.add));
          client.send([attempt]);
          await data.future.timeout(const Duration(seconds: 3));
          final peer = await server.peer;
          // Queue a final read while both peers close the verified TLS socket.
          peer.add([106, attempt]);
          final first = client.dispose();
          final repeated = client.dispose();
          expect(identical(first, repeated), isTrue);
          await Future.wait<dynamic>([first, repeated, peer.close()]);
          expect(streamErrors, isEmpty);
        } finally {
          await client?.dispose();
          await server.dispose();
        }
      }
    });
    expect(errors, isEmpty);
  });

  test('closing one real TLS websocket does not disconnect another connection',
      () async {
    final errors = await _zoneErrors(() async {
      final firstServer = await _Loopback.start();
      final secondServer = await _Loopback.start();
      LiveKitWebSocket? first, second;
      final secondData = Completer<dynamic>();
      final streamErrors = <dynamic>[];
      var secondClosed = 0;
      try {
        first = await LiveKitWebSocket.connect(firstServer.uri,
            networkOptions: _trusted,
            options: WebSocketEventHandlers(onError: streamErrors.add));
        second = await LiveKitWebSocket.connect(secondServer.uri,
            networkOptions: _trusted,
            options: WebSocketEventHandlers(
                onData: secondData.complete,
                onError: streamErrors.add,
                onDispose: () => secondClosed++));
        final firstPeer = await firstServer.peer;
        await Future.wait<dynamic>([first.dispose(), firstPeer.close()]);
        await _turns();
        second.send([106, 2]);
        expect(await secondData.future.timeout(const Duration(seconds: 3)),
            orderedEquals([106, 2]));
        expect(streamErrors, isEmpty);
        expect(secondClosed, 0);
      } finally {
        await first?.dispose();
        await second?.dispose();
        await firstServer.dispose();
        await secondServer.dispose();
      }
    });
    expect(errors, isEmpty);
  });

  test(
      'late async business callback error is observed even during socket close',
      () async {
    final errors = await _zoneErrors(() async {
      for (final failure in <Object>[
        StateError('synthetic business callback rejection'),
        const io.SocketException('synthetic business-owned socket rejection')
      ]) {
        final server = await _Loopback.start();
        LiveKitWebSocket? client;
        final entered = Completer<void>(), release = Completer<void>();
        final observed = Completer<dynamic>();
        final deliveredErrors = <dynamic>[];
        Future<bool>? closing;
        Object? closeFailure;
        try {
          client = await LiveKitWebSocket.connect(server.uri,
              networkOptions: _trusted,
              options: WebSocketEventHandlers(onData: (data) async {
                entered.complete();
                await release.future;
                throw failure;
              }, onError: (dynamic error) {
                deliveredErrors.add(error);
                if (!observed.isCompleted) observed.complete(error);
              }));
          client.send([106, 3]);
          await entered.future.timeout(const Duration(seconds: 3));
          closing = client.dispose().then<bool>((value) => value,
              onError: (Object error, StackTrace stack) {
            closeFailure = error;
            return false;
          });
          // The error belongs to application onData, even if physical close
          // finishes first. Its stack cannot be classified as SDK raw read.
          await _turns();
          release.complete();
          expect(await observed.future.timeout(const Duration(seconds: 3)),
              same(failure));
          expect(deliveredErrors, [same(failure)]);
          await closing;
          if (closeFailure != null) expect(closeFailure, same(failure));
        } finally {
          if (!release.isCompleted) release.complete();
          try {
            await (closing ?? client?.dispose());
          } catch (error) {
            expect(error, same(failure));
          }
          await server.dispose();
        }
      }
    });
    expect(errors, isEmpty);
  });
  test('actual IO onDispose can reenter physical dispose without self-await',
      () async {
    final errors = await _zoneErrors(() async {
      for (final initiatedBy in ['client', 'server']) {
        print('[IOReentry] $initiatedBy start');
        final server = await _Loopback.start();
        LiveKitWebSocket? client;
        final callbackDone = Completer<void>();
        Future<bool>? reentered;
        var calls = 0;
        try {
          client = await LiveKitWebSocket.connect(server.uri,
              networkOptions: _trusted,
              options: WebSocketEventHandlers(onDispose: () async {
            print('[IOReentry] $initiatedBy callback-enter');
            calls++;
            reentered = client!.dispose();
            print('[IOReentry] $initiatedBy reentered');
            await reentered;
            print('[IOReentry] $initiatedBy callback-complete');
            if (!callbackDone.isCompleted) callbackDone.complete();
          }));
          final peer = await server.peer;
          final Future<dynamic> closing =
              initiatedBy == 'client' ? client.dispose() : peer.close();
          print('[IOReentry] $initiatedBy trigger-returned');
          await closing.timeout(const Duration(seconds: 3));
          print('[IOReentry] $initiatedBy outer-close-complete');
          await callbackDone.future.timeout(const Duration(seconds: 3));
          print('[IOReentry] $initiatedBy callbackDone-complete');
          expect(calls, 1);
          expect(reentered, isNotNull);
          expect(identical(client.dispose(), reentered), isTrue);
          await client.dispose().timeout(const Duration(seconds: 3));
        } finally {
          print('[IOReentry] $initiatedBy finally-enter');
          try {
            await client?.dispose().timeout(const Duration(seconds: 3));
          } finally {
            await server.dispose();
          }
        }
      }
    });
    expect(errors, isEmpty);
  });

  test(
      'owned onError async failure is observed once without recursive callback',
      () async {
    final errors = await _zoneErrors(() async {
      final server = await _Loopback.start();
      LiveKitWebSocket? client;
      final unexpected = StateError('synthetic IO callback transport error');
      final observerFailure = StateError('synthetic onError observer failure');
      final received = Completer<void>();
      final callbackErrors = <dynamic>[];
      try {
        client = await LiveKitWebSocket.connect(server.uri,
            networkOptions: _trusted,
            options: WebSocketEventHandlers(
                onData: (_) => throw unexpected,
                onError: (dynamic error) async {
                  callbackErrors.add(error);
                  received.complete();
                  await Future<void>.delayed(Duration.zero);
                  throw observerFailure;
                }));
        client.send([106, 4]);
        await received.future.timeout(const Duration(seconds: 3));
        await _turns();
        expect(callbackErrors, [same(unexpected)]);
        try {
          await client.dispose().timeout(const Duration(seconds: 3));
        } catch (error) {
          expect(error, anyOf(same(unexpected), same(observerFailure)));
        }
        await _turns();
        expect(callbackErrors.length, 1);
      } finally {
        try {
          await client?.dispose().timeout(const Duration(seconds: 3));
        } catch (error) {
          expect(error, anyOf(same(unexpected), same(observerFailure)));
        }
        await server.dispose();
      }
    });
    expect(errors, isEmpty);
  });
}

Future<void> _turns() async {
  for (var turn = 0; turn < 5; turn++) {
    await Future<void>.delayed(Duration.zero);
  }
}
