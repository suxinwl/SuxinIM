import 'dart:async';

/// An immutable login attempt; never render/log the credentials it contains.
class SessionAttempt {
  SessionAttempt(
      this.epoch, this.userID, this.imToken, this.chatToken, this.platformID);
  final int epoch;
  final String userID;
  final String imToken;
  final String chatToken;
  final int platformID;
}

/// Local SDK login can finish from cache before its WebSocket is authenticated.
/// Business navigation additionally requires the captured pair's server check.
class SessionEpoch {
  int _epoch = 0;
  SessionAttempt? _attempt;
  Completer<bool>? _connection;
  bool _active = false;

  int get epoch => _epoch;
  bool get active => _active;
  SessionAttempt? get attempt => _attempt;
  bool current(SessionAttempt value) =>
      identical(value, _attempt) && value.epoch == _epoch;
  bool established(SessionAttempt value) => current(value) && _active;

  SessionAttempt begin(
      String userID, String imToken, String chatToken, int platformID) {
    invalidate();
    _connection = Completer<bool>();
    return _attempt =
        SessionAttempt(_epoch, userID, imToken, chatToken, platformID);
  }

  void connected() {
    final pending = _connection;
    if (pending != null && !pending.isCompleted) pending.complete(true);
  }

  Future<void> waitForConnection(SessionAttempt value,
      {Duration timeout = const Duration(seconds: 20)}) async {
    requireCurrent(value);
    final accepted = await _connection!.future.timeout(timeout);
    requireCurrent(value);
    if (!accepted) throw StateError('IM session was invalidated; retry login');
  }

  void activate(SessionAttempt value) {
    requireCurrent(value);
    _active = true;
  }

  void requireCurrent(SessionAttempt value) {
    if (!current(value))
      throw StateError('IM login attempt is no longer current');
  }

  int invalidate() {
    ++_epoch;
    _active = false;
    _attempt = null;
    final pending = _connection;
    if (pending != null && !pending.isCompleted) pending.complete(false);
    return _epoch;
  }
}
