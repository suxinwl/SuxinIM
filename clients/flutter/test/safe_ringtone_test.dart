// Real SafeRingtone with controlled player Futures. No audio/device backend.
import 'dart:async';

import 'package:flutter_test/flutter_test.dart';
import 'package:just_audio/just_audio.dart';
import 'package:openim_common/src/utils/safe_ringtone.dart';

class _Player extends Fake implements AudioPlayer {
  Future<Duration?> Function(String)? loadOperation;
  Future<void> Function()? loop;
  Future<void> Function()? volumeOperation;
  Future<void> Function()? playOperation;
  Future<void> Function()? stopOperation;
  Future<void> Function()? disposeOperation;
  final assets = <String>[];
  final loopModes = <LoopMode>[];
  final volumes = <double>[];
  final _playing = <Completer<void>>[];
  int playCalls = 0;
  int stopCalls = 0;
  int disposeCalls = 0;

  @override
  Future<Duration?> setAsset(
    String assetPath, {
    String? package,
    bool preload = true,
    Duration? initialPosition,
    dynamic tag,
  }) {
    assets.add(assetPath);
    return loadOperation?.call(assetPath) ??
        Future.value(const Duration(seconds: 1));
  }

  @override
  Future<void> setLoopMode(LoopMode mode) {
    loopModes.add(mode);
    return loop?.call() ?? Future.value();
  }

  @override
  Future<void> setVolume(double value) {
    volumes.add(value);
    return volumeOperation?.call() ?? Future.value();
  }

  @override
  Future<void> play() {
    playCalls++;
    if (playOperation != null) return playOperation!();
    final completion = Completer<void>();
    _playing.add(completion);
    return completion.future;
  }

  void _endPlaying() {
    for (final completion in _playing) {
      if (!completion.isCompleted) completion.complete();
    }
  }

  @override
  Future<void> stop() {
    stopCalls++;
    _endPlaying();
    return stopOperation?.call() ?? Future.value();
  }

  @override
  Future<void> dispose() {
    disposeCalls++;
    _endPlaying();
    return disposeOperation?.call() ?? Future.value();
  }
}

Future<void> _drain() => Future<void>.delayed(Duration.zero);

void main() {
  test(
      'stop runs during pending load and an old load cannot restart a later ring',
      () async {
    final oldLoad = Completer<Duration?>();
    final player = _Player()
      ..loadOperation = (asset) =>
          asset == 'old' ? oldLoad.future : Future.value(Duration.zero);
    final errors = <List<Object>>[];
    final ring = SafeRingtone(player,
        onError: (phase, error) => errors.add([phase, error]));
    addTearDown(ring.dispose);

    final oldStart = ring.start('old', loopMode: LoopMode.one);
    await _drain();
    expect(player.assets, ['old']);
    final stopped = ring.stop();
    expect(player.stopCalls, 1); // not queued behind oldLoad
    await stopped;
    await ring.start('new', loopMode: LoopMode.one);
    expect(player.assets, ['old', 'new']);
    expect(player.playCalls, 1);
    expect(ring.active, isTrue);
    oldLoad.complete(Duration.zero);
    await oldStart;
    expect(player.playCalls, 1);
    expect(player.loopModes, [LoopMode.one]);
    expect(ring.active, isTrue);
    expect(errors, isEmpty);
  });

  test(
      'looping play completion never blocks stop and intended interruption is observed',
      () async {
    final play = Completer<void>();
    final player = _Player()..playOperation = () => play.future;
    final errors = <List<Object>>[];
    final ring = SafeRingtone(player,
        onError: (phase, error) => errors.add([phase, error]));
    addTearDown(ring.dispose);

    await ring.start('ring', loopMode: LoopMode.one);
    expect(player.playCalls, 1);
    expect(play.isCompleted, isFalse);
    await ring.stop();
    expect(player.stopCalls, 1);
    expect(ring.active, isFalse);
    play.completeError(
        PlayerInterruptedException('synthetic intentional stop'));
    await _drain();
    expect(errors, isEmpty);
  });

  test('stop during pending configuration prevents any subsequent play',
      () async {
    final configure = Completer<void>();
    final player = _Player()..loop = () => configure.future;
    final errors = <List<Object>>[];
    final ring = SafeRingtone(player,
        onError: (phase, error) => errors.add([phase, error]));
    addTearDown(ring.dispose);

    final start = ring.start('ring');
    await _drain();
    expect(player.loopModes.length, 1);
    await ring.stop();
    configure.complete();
    await start;
    expect(player.volumes, isEmpty);
    expect(player.playCalls, 0);
    expect(ring.active, isFalse);
    expect(errors, isEmpty);
  });

  test(
      'a new start awaits in-flight stop while repeated stops share completion',
      () async {
    final stopping = Completer<void>();
    final player = _Player()..stopOperation = () => stopping.future;
    final errors = <List<Object>>[];
    final ring = SafeRingtone(player,
        onError: (phase, error) => errors.add([phase, error]));
    addTearDown(ring.dispose);

    final firstStop = ring.stop();
    final repeatedStop = ring.stop();
    expect(identical(firstStop, repeatedStop), isTrue);
    expect(player.stopCalls, 1);
    final start = ring.start('new');
    await _drain();
    expect(player.assets, isEmpty);
    stopping.complete();
    await Future.wait([firstStop, repeatedStop, start]);
    expect(player.assets, ['new']);
    expect(player.playCalls, 1);
    expect(errors, isEmpty);
  });

  test(
      'dispose shares completion and prevents a held load or later start from playing',
      () async {
    final load = Completer<Duration?>();
    final disposing = Completer<void>();
    final player = _Player();
    player.loadOperation = (_) => load.future;
    player.disposeOperation = () => disposing.future;
    final errors = <List<Object>>[];
    final ring = SafeRingtone(player,
        onError: (phase, error) => errors.add([phase, error]));
    final start = ring.start('old');
    await _drain();
    final first = ring.dispose();
    final repeated = ring.dispose();
    expect(identical(first, repeated), isTrue);
    await _drain();
    expect(player.disposeCalls, 1);
    await ring.start('must-not-load');
    expect(player.assets, ['old']);
    load.completeError(PlayerInterruptedException('synthetic dispose'));
    await start;
    expect(player.playCalls, 0);
    disposing.complete();
    await Future.wait([first, repeated]);
    expect(ring.active, isFalse);
    expect(errors, isEmpty);
  });

  test('current load rejection is reported once and permits a later ring',
      () async {
    final error = PlayerException(42, 'synthetic load failure');
    final player = _Player()..loadOperation = (_) => Future.error(error);
    final errors = <List<Object>>[];
    final ring = SafeRingtone(player,
        onError: (phase, error) => errors.add([phase, error]));
    addTearDown(ring.dispose);

    await ring.start('first');
    expect(errors, [
      ['load', error]
    ]);
    expect(ring.active, isFalse);
    expect(player.playCalls, 0);
    player.loadOperation = null;
    await ring.start('second');
    expect(player.playCalls, 1);
    expect(ring.active, isTrue);
  });

  test(
      'current play rejection is reported and does not become an unhandled Future',
      () async {
    final play = Completer<void>();
    final error = PlayerException(43, 'synthetic play failure');
    final player = _Player()..playOperation = () => play.future;
    final errors = <List<Object>>[];
    final ring = SafeRingtone(player,
        onError: (phase, error) => errors.add([phase, error]));
    addTearDown(ring.dispose);

    await ring.start('ring');
    play.completeError(error);
    await _drain();
    expect(errors, [
      ['play', error]
    ]);
    expect(ring.active, isFalse);
  });

  test('unexpected late load failure is reported without changing a newer ring',
      () async {
    final oldLoad = Completer<Duration?>();
    final error = PlayerException(44, 'unexpected old decoder failure');
    final player = _Player()
      ..loadOperation = (asset) =>
          asset == 'old' ? oldLoad.future : Future.value(Duration.zero);
    final errors = <List<Object>>[];
    final ring = SafeRingtone(player,
        onError: (phase, error) => errors.add([phase, error]));
    addTearDown(ring.dispose);

    final old = ring.start('old');
    await _drain();
    await ring.stop();
    await ring.start('new');
    oldLoad.completeError(error);
    await old;
    expect(errors, [
      ['staleLoad', error]
    ]);
    expect(player.playCalls, 1);
    expect(ring.active, isTrue);
  });

  test('unexpected late play failure is reported without stopping a newer ring',
      () async {
    final oldPlay = Completer<void>();
    final error = PlayerException(45, 'unexpected old play failure');
    final player = _Player()..playOperation = () => oldPlay.future;
    final errors = <List<Object>>[];
    final ring = SafeRingtone(player,
        onError: (phase, error) => errors.add([phase, error]));
    addTearDown(ring.dispose);

    await ring.start('old');
    await ring.stop();
    player.playOperation = null;
    await ring.start('new');
    oldPlay.completeError(error);
    await _drain();
    expect(errors, [
      ['stalePlay', error]
    ]);
    expect(player.playCalls, 2);
    expect(ring.active, isTrue);
  });

  test(
      'stop and dispose rejections are observed and disposal is still attempted',
      () async {
    final stopError = StateError('synthetic stop failure');
    final disposeError = StateError('synthetic dispose failure');
    final player = _Player();
    player.stopOperation = () => Future.error(stopError);
    player.disposeOperation = () => Future.error(disposeError);
    final errors = <List<Object>>[];
    final ring = SafeRingtone(player,
        onError: (phase, error) => errors.add([phase, error]));

    await ring.start('ring');
    await ring.dispose();
    expect(player.stopCalls, 1);
    expect(player.disposeCalls, 1);
    expect(errors, [
      ['stop', stopError],
      ['dispose', disposeError]
    ]);
    expect(ring.active, isFalse);
  });
}
