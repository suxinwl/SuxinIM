import 'dart:async';

import 'package:flutter_test/flutter_test.dart';
import 'package:openim/core/session_epoch.dart';
import 'package:openim/core/session_renewal.dart';
import 'package:openim_common/openim_common.dart' show LoginCertificate;

// Exercise the production coordinator and SessionEpoch. Only account/native
// transport and navigation side effects are controlled; no copied state model.
LoginCertificate certificate(String suffix, {String owner = 'one'}) =>
    LoginCertificate.fromJson({
      'userID': 'synthetic-user',
      'sessionToken': 'synthetic-session-$owner',
      'imToken': 'synthetic-im-$suffix',
      'chatToken': 'synthetic-chat-$suffix',
      'expiresAt': 4102444800,
    });

class Harness {
  final session = SessionEpoch();
  LoginCertificate? value = certificate('one');
  late Future<LoginCertificate> Function() refresh;
  LoginCertificate? Function()? readOperation;
  Future<void> Function(LoginCertificate)? unlockOperation;
  Future<void> Function()? lockOperation;
  int locks = 0, kicks = 0, unlocks = 0, logins = 0;
  bool restoring = false;

  Harness() {
    activate(value!);
    refresh = () async => value!;
  }

  void activate(LoginCertificate next) {
    value = next;
    final attempt = session.begin(next.userID, next.imToken, next.chatToken, 2);
    session.activate(attempt);
  }

  Future<void> run() => renewCurrentSSOSession(
        session: session,
        readCertificate: () => readOperation == null ? value : readOperation!(),
        refresh: () => refresh(),
        unlock: (next) async {
          unlocks++;
          await unlockOperation?.call(next);
        },
        login: (next) async {
          logins++;
          activate(next);
        },
        lock: () async {
          locks++;
          await lockOperation?.call();
        },
        kick: () {
          kicks++;
          session.invalidate();
        },
        restoring: () => restoring,
      );
}

void main() {
  test('late refresh failure cannot lock a newer established attempt',
      () async {
    final h = Harness();
    final held = Completer<LoginCertificate>();
    h.refresh = () => held.future;
    final running = h.run();
    h.activate(certificate('two', owner: 'two'));
    held.completeError(StateError('Synthetic refresh rejected'));
    await running;
    expect(h.locks, 0);
    expect(h.kicks, 0);
    expect(h.session.active, isTrue);
    expect(h.value!.sessionToken, 'synthetic-session-two');
  });

  test('late unlock failure cannot lock a newer established attempt', () async {
    final h = Harness();
    final started = Completer<void>(), held = Completer<void>();
    h.unlockOperation = (_) {
      started.complete();
      return held.future;
    };
    final running = h.run();
    await started.future;
    h.activate(certificate('two', owner: 'two'));
    held.completeError(StateError('Synthetic native denial'));
    await running;
    expect(h.locks, 0);
    expect(h.kicks, 0);
    expect(h.session.active, isTrue);
  });

  test('a newer certificate written before SDK activation owns the failure',
      () async {
    final h = Harness();
    final held = Completer<LoginCertificate>();
    h.refresh = () => held.future;
    final running = h.run();
    // Login stores its new SID before the SDK begins the next SessionAttempt.
    h.value = certificate('two', owner: 'two');
    held.completeError(StateError('Synthetic old refresh rejected'));
    await running;
    expect(h.locks, 0);
    expect(h.kicks, 0);
    expect(h.value!.sessionToken, 'synthetic-session-two');
  });

  test('late unlock success cannot start SDK login for the old owner',
      () async {
    final h = Harness();
    final refreshed = certificate('renewed');
    h.refresh = () async => h.value = refreshed;
    final started = Completer<void>(), held = Completer<void>();
    h.unlockOperation = (_) {
      started.complete();
      return held.future;
    };
    final running = h.run();
    await started.future;
    h.activate(certificate('two', owner: 'two'));
    held.complete();
    await running;
    expect(h.logins, 0);
    expect(h.locks, 0);
    expect(h.value!.sessionToken, 'synthetic-session-two');
  });

  test('a session established while native lock drains is not kicked',
      () async {
    final h = Harness();
    h.refresh = () async => throw StateError('Synthetic current failure');
    final started = Completer<void>(), held = Completer<void>();
    h.lockOperation = () {
      started.complete();
      return held.future;
    };
    final running = h.run();
    await started.future;
    h.activate(certificate('two', owner: 'two'));
    held.complete();
    await running;
    expect(h.locks, 1); // The old owner was current when teardown began.
    expect(h.kicks, 0);
    expect(h.session.active, isTrue);
  });

  test('current refresh network and permission failures remain fail closed',
      () async {
    for (final failure in [
      TimeoutException('Synthetic timeout'),
      StateError('Synthetic 403')
    ]) {
      final h = Harness();
      h.refresh = () async => throw failure;
      await h.run();
      expect(h.locks, 1);
      expect(h.kicks, 1);
      expect(h.unlocks, 0);
      expect(h.session.active, isFalse);
    }
  });

  test('current native authorization failure remains fail closed', () async {
    final h = Harness();
    h.unlockOperation =
        (_) async => throw StateError('Synthetic native denial');
    await h.run();
    expect(h.locks, 1);
    expect(h.kicks, 1);
    expect(h.session.active, isFalse);
  });

  test('legitimate same SID JWT rotation unlocks and logs SDK in once',
      () async {
    final h = Harness();
    final next = certificate('rotated');
    h.refresh = () async => h.value = next;
    await h.run();
    expect(h.unlocks, 1);
    expect(h.logins, 1);
    expect(h.locks, 0);
    expect(h.kicks, 0);
    expect(h.session.attempt!.imToken, next.imToken);
    expect(h.session.active, isTrue);
  });

  test('same owner persistence failure still locks after RAM pair changed',
      () async {
    final h = Harness();
    h.refresh = () async {
      h.value = certificate('rotated');
      throw StateError('Synthetic protected persistence failure');
    };
    await h.run();
    expect(h.locks, 1);
    expect(h.kicks, 1);
    expect(h.session.active, isFalse);
  });

  test('restore owns a preexisting renewal failure after it starts', () async {
    final h = Harness();
    final held = Completer<LoginCertificate>();
    h.refresh = () => held.future;
    final running = h.run();
    h.restoring = true;
    held.completeError(StateError('Synthetic old refresh failure'));
    await running;
    expect(h.locks, 0);
    expect(h.kicks, 0);
    expect(h.session.active, isTrue);
  });

  test('a newer same SID pair during unlock is not locked by the old failure',
      () async {
    final h = Harness();
    final started = Completer<void>(), held = Completer<void>();
    h.unlockOperation = (_) {
      started.complete();
      return held.future;
    };
    final running = h.run();
    await started.future;
    // The owner did not change, and SDK activation has not begun yet.
    h.value = certificate('newer');
    held.completeError(StateError('Synthetic older native denial'));
    await running;
    expect(h.locks, 0);
    expect(h.kicks, 0);
    expect(h.value!.imToken, 'synthetic-im-newer');
    expect(h.session.active, isTrue);
  });

  test('a superseded same SID refresh result cannot unlock the newer pair',
      () async {
    final h = Harness();
    final held = Completer<LoginCertificate>();
    h.refresh = () => held.future;
    final running = h.run();
    h.value = certificate('newest');
    held.complete(certificate('older-result'));
    await running;
    expect(h.unlocks, 0);
    expect(h.logins, 0);
    expect(h.locks, 0);
    expect(h.kicks, 0);
    expect(h.session.active, isTrue);
  });

  test('an unpublished refresh result still fails closed for the current owner',
      () async {
    final h = Harness();
    h.refresh = () async => certificate('not-persisted');
    await h.run();
    expect(h.unlocks, 0);
    expect(h.locks, 1);
    expect(h.kicks, 1);
    expect(h.session.active, isFalse);
  });

  test('an active session with missing current credentials fails closed',
      () async {
    final h = Harness();
    h.value = null;
    await h.run();
    expect(h.unlocks, 0);
    expect(h.locks, 1);
    expect(h.kicks, 1);
    expect(h.session.active, isFalse);
  });

  test('current credentials cannot accept a refresh result from another SID',
      () async {
    final h = Harness();
    h.refresh = () async => certificate('wrong', owner: 'another');
    await h.run();
    expect(h.unlocks, 0);
    expect(h.locks, 1);
    expect(h.kicks, 1);
  });

  test('unchanged current pair renews without restarting the SDK', () async {
    final h = Harness();
    await h.run();
    expect(h.unlocks, 1);
    expect(h.logins, 0);
    expect(h.locks, 0);
    expect(h.kicks, 0);
    expect(h.session.active, isTrue);
  });

  test('initial current certificate read failure remains fail closed',
      () async {
    final h = Harness();
    h.readOperation =
        () => throw const FormatException('Synthetic invalid JSON');
    await h.run();
    expect(h.unlocks, 0);
    expect(h.locks, 1);
    expect(h.kicks, 1);
    expect(h.session.active, isFalse);
  });

  test('current certificate read failure after refresh remains fail closed',
      () async {
    final h = Harness();
    h.refresh = () async {
      h.readOperation =
          () => throw const FormatException('Synthetic invalid JSON');
      throw StateError('Synthetic current refresh failure');
    };
    await h.run();
    expect(h.unlocks, 0);
    expect(h.locks, 1);
    expect(h.kicks, 1);
    expect(h.session.active, isFalse);
  });

  test('unreadable newer credentials cannot revive stale attempt teardown',
      () async {
    final h = Harness();
    final held = Completer<LoginCertificate>();
    h.refresh = () => held.future;
    final running = h.run();
    h.activate(certificate('new', owner: 'new'));
    h.readOperation =
        () => throw const FormatException('Synthetic invalid JSON');
    held.completeError(StateError('Synthetic older failure'));
    await running;
    expect(h.locks, 0);
    expect(h.kicks, 0);
    expect(h.session.active, isTrue);
  });

  test('post unlock unreadable unchanged pair is a current failure', () async {
    final h = Harness();
    h.unlockOperation = (_) async {
      h.readOperation =
          () => throw const FormatException('Synthetic invalid JSON');
    };
    await h.run();
    expect(h.unlocks, 1);
    expect(h.logins, 0);
    expect(h.locks, 1);
    expect(h.kicks, 1);
    expect(h.session.active, isFalse);
  });

  test(
      'post unlock missing credentials cannot leave the current session active',
      () async {
    final h = Harness();
    h.unlockOperation = (_) async {
      h.value = null;
    };
    await h.run();
    expect(h.unlocks, 1);
    expect(h.locks, 1);
    expect(h.kicks, 1);
    expect(h.session.active, isFalse);
  });
}
