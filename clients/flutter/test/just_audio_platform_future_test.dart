// Actual vendored AudioPlayer activation; only the native platform boundary is
// controlled. No copied lifecycle algorithm, real audio, device or network.
import 'dart:async';

import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:just_audio/just_audio.dart';
// The production just_audio dependency supplies this exact native boundary.
// ignore: depend_on_referenced_packages
import 'package:just_audio_platform_interface/just_audio_platform_interface.dart'
    as native;

class _Backend extends native.JustAudioPlatform {
  final players = <_NativePlayer>[];
  Completer<void>? configureBarrier;
  Object? configureFailure;
  Object? initializationFailure;
  Completer<void>? loadBarrier;
  bool completePlayOnDispose = true;

  @override
  Future<native.AudioPlayerPlatform> init(native.InitRequest request) async {
    if (initializationFailure != null) throw initializationFailure!;
    final player = _NativePlayer(request.id)
      ..configureBarrier = configureBarrier
      ..configureFailure = configureFailure
      ..loadBarrier = loadBarrier;
    players.add(player);
    return player;
  }

  @override
  Future<native.DisposeAllPlayersResponse> disposeAllPlayers(
          native.DisposeAllPlayersRequest request) async =>
      native.DisposeAllPlayersResponse();

  @override
  Future<native.DisposePlayerResponse> disposePlayer(
      native.DisposePlayerRequest request) async {
    for (final player in players.where((p) => p.id == request.id)) {
      player.disposals++;
      if (completePlayOnDispose) player.finishPlay();
    }
    return native.DisposePlayerResponse();
  }
}

class _NativePlayer extends native.AudioPlayerPlatform {
  _NativePlayer(super.id);
  final events = StreamController<native.PlaybackEventMessage>.broadcast();
  final configureEntered = Completer<void>();
  Completer<void>? configureBarrier;
  Object? configureFailure;
  Completer<void>? loadBarrier;
  int disposals = 0;
  int loads = 0;
  int plays = 0;
  Completer<native.PlayResponse>? playing;

  @override
  Stream<native.PlaybackEventMessage> get playbackEventMessageStream =>
      events.stream;

  @override
  Future<native.SetVolumeResponse> setVolume(
      native.SetVolumeRequest request) async {
    if (!configureEntered.isCompleted) configureEntered.complete();
    await configureBarrier?.future;
    if (configureFailure != null) throw configureFailure!;
    return native.SetVolumeResponse();
  }

  @override
  Future<native.SetSpeedResponse> setSpeed(
          native.SetSpeedRequest request) async =>
      native.SetSpeedResponse();

  @override
  Future<native.SetPitchResponse> setPitch(
          native.SetPitchRequest request) async =>
      native.SetPitchResponse();

  @override
  Future<native.SetSkipSilenceResponse> setSkipSilence(
          native.SetSkipSilenceRequest request) async =>
      native.SetSkipSilenceResponse();

  @override
  Future<native.SetLoopModeResponse> setLoopMode(
          native.SetLoopModeRequest request) async =>
      native.SetLoopModeResponse();

  @override
  Future<native.SetShuffleModeResponse> setShuffleMode(
          native.SetShuffleModeRequest request) async =>
      native.SetShuffleModeResponse();

  @override
  Future<native.LoadResponse> load(native.LoadRequest request) async {
    loads++;
    events.add(native.PlaybackEventMessage(
      processingState: native.ProcessingStateMessage.ready,
      updateTime: DateTime.now(),
      updatePosition: Duration.zero,
      bufferedPosition: const Duration(seconds: 2),
      duration: const Duration(seconds: 2),
      icyMetadata: null,
      currentIndex: 0,
      androidAudioSessionId: null,
    ));
    await loadBarrier?.future;
    return native.LoadResponse(duration: const Duration(seconds: 2));
  }

  @override
  Future<native.PlayResponse> play(native.PlayRequest request) {
    plays++;
    return (playing ??= Completer<native.PlayResponse>()).future;
  }

  void finishPlay([Object? failure]) {
    final pending = playing;
    if (pending != null && !pending.isCompleted) {
      if (failure == null) {
        pending.complete(native.PlayResponse());
      } else {
        pending.completeError(failure);
      }
    }
  }
}

Future<List<Object>> _ownedErrors(Future<void> Function() operation) async {
  final result = Completer<void>();
  final errors = <Object>[];
  runZonedGuarded(() async {
    try {
      await operation();
      await Future<void>.delayed(const Duration(milliseconds: 20));
      result.complete();
    } catch (error, stack) {
      result.completeError(error, stack);
    }
  }, (Object error, StackTrace stack) => errors.add(error));
  await result.future.timeout(const Duration(seconds: 8));
  return errors;
}

Future<void> _until(bool Function() ready) async {
  final deadline = DateTime.now().add(const Duration(seconds: 2));
  while (!ready()) {
    if (DateTime.now().isAfter(deadline)) {
      fail('Native audio fixture not reached');
    }
    await Future<void>.delayed(Duration.zero);
  }
}

Future<Object?> _observe(Future<Duration?> future,
        [List<StackTrace>? stacks]) =>
    future.then<Object?>((duration) => duration,
        onError: (Object error, StackTrace stack) {
      stacks?.add(stack);
      return error;
    });

AudioPlayer _player() => AudioPlayer(
    handleInterruptions: false,
    androidApplyAudioAttributes: false,
    handleAudioSessionActivation: false);

Future<void> _release(AudioPlayer player, _Backend backend) async {
  await player.dispose();
  for (final nativePlayer in backend.players) {
    if (!nativePlayer.events.isClosed) await nativePlayer.events.close();
  }
}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  setUp(() {
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(
            const MethodChannel('com.ryanheise.audio_session'),
            (_) async => null);
  });

  test('actual activation interrupted after configure observes both futures',
      () async {
    final errors = await _ownedErrors(() async {
      final barrier = Completer<void>();
      final backend = _Backend()..configureBarrier = barrier;
      native.JustAudioPlatform.instance = backend;
      final player = _player();
      try {
        await Future<void>.delayed(Duration.zero);
        final loading = _observe(player
            .setAudioSource(AudioSource.uri(Uri.file('/synthetic-ring.wav'))));
        await _until(() => backend.players.isNotEmpty);
        await backend.players.single.configureEntered.future;
        await player.stop();
        barrier.complete();
        final failure = await loading.timeout(const Duration(seconds: 2));
        expect(failure,
            isA<PlatformException>().having((e) => e.code, 'code', 'abort'));
        expect(backend.players.single.loads, 0);
        expect(backend.players.single.plays, 0);
        // A genuine stop/abort is still an interruption, while a later
        // activation of this same player can load normally again.
        backend.configureBarrier = null;
        expect(await player.load(), const Duration(seconds: 2));
        expect(backend.players.length, 2);
        expect(backend.players.last.loads, 1);
        await player.stop();
      } finally {
        if (!barrier.isCompleted) barrier.complete();
        await _release(player, backend);
      }
    });
    expect(errors, isEmpty,
        reason:
            'observed public load must not leave a cached platform sibling error');
  });

  test('actual pre-load configuration failure reaches caller without hanging',
      () async {
    final expected = StateError('synthetic volume configuration failure');
    final errors = await _ownedErrors(() async {
      final backend = _Backend()..configureFailure = expected;
      native.JustAudioPlatform.instance = backend;
      final player = _player();
      try {
        await Future<void>.delayed(Duration.zero);
        final stacks = <StackTrace>[];
        final failure = await _observe(
                player.setAudioSource(
                    AudioSource.uri(Uri.file('/synthetic-ring.wav'))),
                stacks)
            .timeout(const Duration(seconds: 2));
        expect(failure, same(expected));
        expect(stacks.single.toString(), contains('_NativePlayer.setVolume'));
        expect(backend.players.single.loads, 0);
      } finally {
        await _release(player, backend);
      }
    });
    expect(errors, isEmpty);
  });

  test('actual initialization failure retains original error identity',
      () async {
    final expected = StateError('synthetic native initialization failure');
    final errors = await _ownedErrors(() async {
      final backend = _Backend()..initializationFailure = expected;
      native.JustAudioPlatform.instance = backend;
      final player = _player();
      try {
        await Future<void>.delayed(Duration.zero);
        final failure = await _observe(player.setAudioSource(
                AudioSource.uri(Uri.file('/synthetic-ring.wav'))))
            .timeout(const Duration(seconds: 2));
        expect(failure, same(expected));
      } finally {
        // dispose legitimately awaits the same failed native initialization.
        try {
          await _release(player, backend);
        } catch (error) {
          expect(error, same(expected));
        }
      }
    });
    expect(errors, isEmpty);
  });

  test('actual deferred-play activation failure reaches its public future',
      () async {
    final expected = StateError('synthetic deferred play activation failure');
    final errors = await _ownedErrors(() async {
      final backend = _Backend()..configureFailure = expected;
      native.JustAudioPlatform.instance = backend;
      final player = _player();
      try {
        await Future<void>.delayed(Duration.zero);
        await player.setAudioSource(
            AudioSource.uri(Uri.file('/synthetic-ring.wav')),
            preload: false);
        final failure = await player
            .play()
            .then<Object?>((_) => null,
                onError: (Object error, StackTrace stack) => error)
            .timeout(const Duration(seconds: 2));
        expect(failure, same(expected));
        expect(backend.players.single.plays, 0);
      } finally {
        await _release(player, backend);
      }
    });
    expect(errors, isEmpty);
  });

  test('cached platform awaits still fail with original configuration error',
      () async {
    final expected = StateError('synthetic shared platform activation failure');
    final errors = await _ownedErrors(() async {
      final barrier = Completer<void>();
      final backend = _Backend()
        ..configureBarrier = barrier
        ..configureFailure = expected;
      native.JustAudioPlatform.instance = backend;
      final player = _player();
      try {
        await Future<void>.delayed(Duration.zero);
        final loading = _observe(player
            .setAudioSource(AudioSource.uri(Uri.file('/synthetic-ring.wav'))));
        await _until(() => backend.players.isNotEmpty);
        await backend.players.single.configureEntered.future;
        final setting = player.setSpeed(1.2).then<Object?>((_) => null,
            onError: (Object error, StackTrace stack) => error);
        barrier.complete();
        expect(
            await setting.timeout(const Duration(seconds: 2)), same(expected));
        expect(
            await loading.timeout(const Duration(seconds: 2)), same(expected));
      } finally {
        if (!barrier.isCompleted) barrier.complete();
        await _release(player, backend);
      }
    });
    expect(errors, isEmpty);
  });

  for (final lateNativeFails in [false, true]) {
    test(
        'actual interrupted play ignores late native ${lateNativeFails ? 'failure' : 'success'} completion once',
        () async {
      final errors = await _ownedErrors(() async {
        final loadBarrier = Completer<void>();
        final backend = _Backend()
          ..loadBarrier = loadBarrier
          ..completePlayOnDispose = false;
        native.JustAudioPlatform.instance = backend;
        final player = _player();
        try {
          await Future<void>.delayed(Duration.zero);
          await player.setAudioSource(
              AudioSource.uri(Uri.file('/synthetic-ring.wav')),
              preload: false);
          final playing = player.play().then<Object?>((_) => null,
              onError: (Object error, StackTrace stack) => error);
          await _until(() =>
              backend.players.isNotEmpty &&
              backend.players.single.loads == 1 &&
              backend.players.single.plays == 1);
          final old = backend.players.single;
          await player.stop();
          loadBarrier.complete();
          final interruption =
              await playing.timeout(const Duration(seconds: 2));
          expect(interruption, isA<PlayerInterruptedException>());
          old.finishPlay(lateNativeFails
              ? StateError('synthetic late native play failure')
              : null);
          await Future<void>.delayed(Duration.zero);
          expect(await playing, same(interruption));
          expect(player.playing, isFalse);
        } finally {
          if (!loadBarrier.isCompleted) loadBarrier.complete();
          for (final old in backend.players) {
            old.finishPlay();
          }
          await _release(player, backend);
        }
      });
      expect(errors, isEmpty,
          reason: 'late native reply must not double-complete play completer');
    });
  }

  test('late old native load cannot deactivate a newer same-player activation',
      () async {
    final errors = await _ownedErrors(() async {
      final oldLoad = Completer<void>();
      final backend = _Backend()..loadBarrier = oldLoad;
      native.JustAudioPlatform.instance = backend;
      final player = _player();
      Future<Object?>? newPlaying;
      var newerPlayCompleted = false;
      try {
        await Future<void>.delayed(Duration.zero);
        final old = _observe(player.setAudioSource(
            AudioSource.uri(Uri.file('/synthetic-old-ring.wav'))));
        await _until(() =>
            backend.players.isNotEmpty && backend.players.first.loads == 1);
        await player.stop();
        backend.loadBarrier = null;
        expect(
            await player.setAudioSource(
                AudioSource.uri(Uri.file('/synthetic-new-ring.wav'))),
            const Duration(seconds: 2));
        final current = backend.players.last;
        newPlaying = player.play().then<Object?>((_) {
          newerPlayCompleted = true;
          return null;
        }, onError: (Object error, StackTrace stack) {
          newerPlayCompleted = true;
          return error;
        });
        await _until(() => current.plays == 1);
        oldLoad.complete();
        expect(await old.timeout(const Duration(seconds: 2)),
            isA<PlayerInterruptedException>());
        await Future<void>.delayed(Duration.zero);
        expect(current.disposals, 0,
            reason: 'an older load error does not own the current activation');
        expect(newerPlayCompleted, isFalse);
        expect(player.playing, isTrue);
      } finally {
        if (!oldLoad.isCompleted) oldLoad.complete();
        await player.stop();
        await newPlaying;
        await _release(player, backend);
      }
    });
    expect(errors, isEmpty);
  });

  test('healthy actual load play stop and a later reload remain usable',
      () async {
    final errors = await _ownedErrors(() async {
      final backend = _Backend();
      native.JustAudioPlatform.instance = backend;
      final player = _player();
      try {
        await Future<void>.delayed(Duration.zero);
        expect(
            await player.setAudioSource(
                AudioSource.uri(Uri.file('/synthetic-ring.wav'))),
            const Duration(seconds: 2));
        final played = player.play();
        await _until(() => backend.players.single.plays == 1);
        await player.stop();
        await played;
        expect(await player.load(), const Duration(seconds: 2));
        expect(backend.players.length, 2);
        expect(backend.players.last.loads, 1);
        await player.stop();
      } finally {
        await _release(player, backend);
      }
    });
    expect(errors, isEmpty);
  });
}
