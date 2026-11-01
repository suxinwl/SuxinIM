import 'dart:async';
import 'package:flutter_test/flutter_test.dart';
import 'package:livekit_client/src/support/cleanup_sequence.dart';
import 'package:livekit_client/src/support/disposable.dart';

class TestRtcOwner extends Disposable with CleanupFailureReporting {
  TestRtcOwner(Future<void> Function() cleanup) {
    onDispose(() => recordCleanup(cleanup));
  }

  @override
  Future<bool> dispose() => reportCleanupResult(super.dispose);
}

void main() {
  test('synchronous native callback reentry cannot observe premature disposal',
      () async {
    final native = Completer<void>();
    late TestRtcOwner owner;
    Future<bool>? reentered;
    var calls = 0;
    owner = TestRtcOwner(() async {
      calls++;
      reentered = owner.dispose();
      await native.future;
    });
    final first = owner.dispose();
    expect(identical(first, reentered), isTrue);
    expect(calls, 1);
    native.complete();
    expect(await first, isTrue);
    expect(await reentered, isTrue);
  });

  test('concurrent disposal shares real native completion', () async {
    final native = Completer<void>();
    var calls = 0;
    var completed = false;
    final owner = TestRtcOwner(() async {
      calls++;
      await native.future;
    });
    final first = owner.dispose();
    final repeated = owner.dispose();
    repeated.then((_) => completed = true);
    await Future<void>.delayed(Duration.zero);
    expect(identical(first, repeated), isTrue);
    expect(completed, isFalse);
    expect(calls, 1);
    native.complete();
    expect(await repeated, isTrue);
    expect(await first, isTrue);
  });
  test('a track failure cannot skip camera, either peer, or signal cleanup',
      () async {
    final order = <String>[];
    final first = StateError('synthetic track stop failure');
    final later = StateError('synthetic publisher close failure');
    final owner = TestRtcOwner(() => runCleanupSteps('room', {
          'remoteTrack': () async {
            order.add('remoteTrack');
            throw first;
          },
          'camera': () async {
            order.add('camera');
          },
          'publisher': () async {
            order.add('publisher');
            throw later;
          },
          'subscriber': () async {
            order.add('subscriber');
          },
          'signal': () async {
            order.add('signal');
          },
        }));
    await expectLater(owner.dispose(), throwsA(same(first)));
    expect(
        order, ['remoteTrack', 'camera', 'publisher', 'subscriber', 'signal']);
    // Upstream marks an owner disposed after a callback fails. Retain failure
    // across that state so a second dispose is never interpreted as release.
    await expectLater(owner.dispose(), throwsA(same(first)));
    expect(order.length, 5);
  });

  test('native close rejection still attempts native dispose and reports close',
      () async {
    final failure = StateError('native close failed');
    var nativeDisposeAttempted = false;
    await expectLater(
        runCleanupSteps('transport', {
          'close': () async {
            throw failure;
          },
          'dispose': () async {
            nativeDisposeAttempted = true;
          },
        }),
        throwsA(same(failure)));
    expect(nativeDisposeAttempted, isTrue);
  });
}
