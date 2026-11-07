package io.openim.flutter_openim_sdk;

import java.net.SocketTimeoutException;
import java.security.GeneralSecurityException;

/** Bounded transport retry for a still-current, previously verified lease. */
public final class NativeVaultRenewal {
  private NativeVaultRenewal() {}
  public interface Clock { long now(); }
  public interface Current { boolean isCurrent(); }
  public interface Attempt<T> { T run(int timeoutMillis) throws Exception; }
  public static final int REQUEST_DEADLINE_MILLIS = 2500;

  public static void requireCurrent(boolean existingLease, long absoluteDeadline,
      Clock clock, Current current) throws GeneralSecurityException {
    if (!current.isCurrent() || (existingLease && clock.now() >= absoluteDeadline)) {
      throw new GeneralSecurityException("Vault generation or lease changed");
    }
  }

  public static <T> T execute(boolean existingLease, boolean retryAllowed,
      long absoluteDeadline, Clock clock, Current current, Attempt<T> attempt)
      throws Exception {
    for (int index = 0; index < 2; index++) {
      requireCurrent(existingLease, absoluteDeadline, clock, current);
      long started = clock.now();
      long remaining = existingLease ? absoluteDeadline - started : REQUEST_DEADLINE_MILLIS;
      if (remaining <= 0) throw new GeneralSecurityException("Vault lease expired");
      int timeout = (int) Math.min(REQUEST_DEADLINE_MILLIS, remaining);
      try {
        T response = attempt.run(timeout);
        long elapsed = clock.now() - started;
        if (elapsed < 0 || elapsed >= timeout) {
          throw new SocketTimeoutException("Vault request deadline exceeded");
        }
        requireCurrent(existingLease, absoluteDeadline, clock, current);
        return response;
      } catch (SocketTimeoutException timeoutFailure) {
        if (index != 0 || !existingLease || !retryAllowed) throw timeoutFailure;
        requireCurrent(true, absoluteDeadline, clock, current);
        // Never start an extra attempt without the original complete request
        // budget. No retry or failure changes the old monotonic deadline.
        if (absoluteDeadline - clock.now() < REQUEST_DEADLINE_MILLIS) throw timeoutFailure;
      }
    }
    throw new GeneralSecurityException("Vault renewal attempts exhausted");
  }
}
