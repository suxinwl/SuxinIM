import 'dart:async';

import 'package:just_audio/just_audio.dart';

/// Observes every player Future without putting stop behind a long-running play.
/// A load from an earlier invitation must never start ringing a later session.
class SafeRingtone {
  SafeRingtone(this.player, {required this.onError});

  final AudioPlayer player;
  final void Function(String phase, Object error) onError;
  int _generation = 0;
  bool _active = false;
  bool _disposed = false;
  Future<void>? _stopping;
  Future<void>? _disposing;

  bool get active => _active;

  Future<void> start(String asset,
      {String? package, LoopMode loopMode = LoopMode.off}) async {
    if (_disposed || _active) return;
    final generation = ++_generation;
    _active = true;
    bool current() => !_disposed && generation == _generation && _active;
    try {
      await _stopping;
      if (!current()) return;
      await player.setAsset(asset, package: package);
      if (!current()) return;
      await player.setLoopMode(loopMode);
      if (!current()) return;
      await player.setVolume(1.0);
      if (!current()) return;
      // Looping play completes only when stopped; do not await it before stop.
      unawaited(player.play().then<void>((_) {
        if (generation == _generation) _active = false;
      }, onError: (Object error, StackTrace stack) {
        if (current()) {
          _active = false;
          onError('play', error);
        } else if (error is! PlayerInterruptedException) {
          onError('stalePlay', error);
        }
      }));
    } catch (error) {
      // stop/dispose can intentionally interrupt the older pending load.
      if (current()) {
        _active = false;
        onError('load', error);
      } else if (error is! PlayerInterruptedException) {
        onError('staleLoad', error);
      }
    }
  }

  Future<void> stop() {
    ++_generation;
    _active = false;
    final pending = _stopping;
    if (pending != null) return pending;
    final completion = Completer<void>();
    _stopping = completion.future;
    Future<void>.sync(player.stop).then<void>((_) {
      _stopping = null;
      completion.complete();
    }, onError: (Object error, StackTrace stack) {
      _stopping = null;
      onError('stop', error);
      completion.complete();
    });
    return completion.future;
  }

  Future<void> dispose() {
    final pending = _disposing;
    if (pending != null) return pending;
    _disposed = true;
    final completion = Completer<void>();
    _disposing = completion.future;
    () async {
      await stop();
      try {
        await player.dispose();
      } catch (error) {
        onError('dispose', error);
      }
      completion.complete();
    }();
    return completion.future;
  }
}
