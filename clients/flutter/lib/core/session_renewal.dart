import 'package:openim_common/openim_common.dart' show LoginCertificate;

import 'session_epoch.dart';
import 'vault_reauthorization.dart';

/// The controller supplies its real account/native operations. This coordinator
/// owns their ordering without requiring SDK, navigation or plugin instances.
Future<void> renewCurrentSSOSession({
  required SessionEpoch session,
  required LoginCertificate? Function() readCertificate,
  required Future<LoginCertificate> Function() refresh,
  required Future<void> Function(LoginCertificate) unlock,
  required Future<void> Function(LoginCertificate) login,
  required Future<void> Function() lock,
  required void Function() kick,
  required bool Function() restoring,
  Future<void> Function()? suspend,
}) async {
  if (!session.active || restoring()) return;
  final attempt = session.attempt;
  if (attempt == null) return;
  // Refresh keeps the opaque SSO owner but may rotate both SDK credentials.
  // Capture strings: LoginCertificate itself is mutable.
  String? ownerID, ownerSession, originalIM, originalChat;
  bool ownerCaptured = false;
  String? refreshedIM, refreshedChat;
  bool boundToRefresh = false;

  bool ownsAttempt({bool unreadableIsCurrent = false}) {
    if (restoring() || !session.established(attempt)) return false;
    // An unreadable current certificate is an authorization failure, not proof
    // that another session owns it. The attempt gate still rejects stale work.
    if (!ownerCaptured) return true;
    LoginCertificate? stored;
    try {
      stored = readCertificate();
    } catch (_) {
      if (unreadableIsCurrent) return true;
      rethrow;
    }
    if (stored == null && ownerID != null) {
      if (unreadableIsCurrent) return true;
      throw StateError('Missing current login credentials');
    }
    if (stored?.userID != ownerID || stored?.sessionToken != ownerSession) {
      return false;
    }
    return !boundToRefresh ||
        (stored?.imToken == refreshedIM && stored?.chatToken == refreshedChat);
  }

  try {
    final original = readCertificate();
    ownerID = original?.userID;
    ownerSession = original?.sessionToken;
    originalIM = original?.imToken;
    originalChat = original?.chatToken;
    ownerCaptured = true;
    if (original == null) throw StateError('Missing current login credentials');
    final current = await refresh();
    if (!ownsAttempt()) return;
    if (current.userID != ownerID || current.sessionToken != ownerSession) {
      throw StateError('Renewal returned another session owner');
    }
    final stored = readCertificate()!;
    if (stored.imToken != current.imToken ||
        stored.chatToken != current.chatToken) {
      // Another refresh can publish its pair before SDK login starts. Never
      // unlock or tear it down using this older result. A result that was not
      // published at all is a failure of the still-current original owner.
      if (stored.imToken == originalIM && stored.chatToken == originalChat) {
        throw StateError('Renewal credentials were not committed');
      }
      return;
    }
    refreshedIM = current.imToken;
    refreshedChat = current.chatToken;
    boundToRefresh = true;
    await unlock(current);
    if (!ownsAttempt()) return;
    if (originalIM != current.imToken) await login(current);
  } catch (error) {
    if (!ownsAttempt(unreadableIsCurrent: true)) return;
    if (suspend != null && isTemporarySessionFailure(error)) {
      await suspend();
      return;
    }
    await lock();
    if (ownsAttempt(unreadableIsCurrent: true)) kick();
  }
}
