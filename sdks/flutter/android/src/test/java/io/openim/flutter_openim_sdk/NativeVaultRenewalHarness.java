package io.openim.flutter_openim_sdk;

import java.io.IOException;
import java.net.SocketTimeoutException;
import java.security.GeneralSecurityException;
import java.security.cert.CertificateException;
import java.util.ArrayList;
import java.util.List;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicInteger;
import java.util.concurrent.atomic.AtomicLong;
import java.util.concurrent.atomic.AtomicReference;
import javax.net.ssl.SSLHandshakeException;
import javax.net.ssl.SSLPeerUnverifiedException;

/** Executes the production helper with synthetic monotonic authority, no Android/network. */
public final class NativeVaultRenewalHarness {
  private static int checks;
  private static int cases;
  private interface Checked { void run() throws Exception; }

  private static final class Authority implements NativeVaultRenewal.Clock {
    final AtomicLong now = new AtomicLong(1000);
    volatile long generation = 1;
    volatile String actor = "synthetic-a";
    volatile String sid = "synthetic-session-a";
    volatile long epoch = 3;
    volatile boolean alive = true;
    final long deadline = 9000; // The already-verified eight-second lease.
    @Override public long now() { return now.get(); }
    void advance(long value) { now.addAndGet(value); }
    NativeVaultRenewal.Current capture() {
      final long g = generation, e = epoch;
      final String a = actor, s = sid;
      return () -> alive && generation == g && actor.equals(a) && sid.equals(s) && epoch == e;
    }
  }

  private static void check(boolean value, String code) {
    if (!value) throw new AssertionError(code);
    checks++;
  }

  private static void reject(Class<? extends Throwable> type, Checked action, String code)
      throws Exception {
    try { action.run(); }
    catch (Throwable error) {
      check(type.isInstance(error), code + "_exception_type");
      return;
    }
    throw new AssertionError(code + "_accepted");
  }

  private static void oneTimeoutThenSuccess() throws Exception {
    Authority a = new Authority();
    AtomicInteger calls = new AtomicInteger();
    List<Integer> budgets = new ArrayList<>();
    NativeVaultRenewal.Current current = a.capture();
    String result = NativeVaultRenewal.execute(true, true, a.deadline, a, current, timeout -> {
      budgets.add(timeout);
      if (calls.incrementAndGet() == 1) {
        a.advance(1500);
        throw new SocketTimeoutException("synthetic");
      }
      a.advance(100);
      return "verified";
    });
    check(result.equals("verified") && calls.get() == 2, "transient_timeout_recovered");
    check(budgets.equals(List.of(2500, 2500)), "full_request_budgets");
    check(a.deadline == 9000 && a.now() == 2600, "original_deadline_unchanged");
    NativeVaultRenewal.requireCurrent(true, a.deadline, a, current);
    check(current.isCurrent(), "same_actor_preserved");
    cases++;
  }

  private static void repeatedFailureNeverExtendsLease() throws Exception {
    Authority a = new Authority();
    AtomicInteger calls = new AtomicInteger();
    NativeVaultRenewal.Current current = a.capture();
    reject(SocketTimeoutException.class, () -> NativeVaultRenewal.execute(true, true,
        a.deadline, a, current, timeout -> {
          calls.incrementAndGet(); a.advance(1500);
          throw new SocketTimeoutException("synthetic");
        }), "second_timeout_rejected");
    check(calls.get() == 2 && a.deadline == 9000, "two_attempts_no_lease_extension");
    a.now.set(a.deadline);
    reject(GeneralSecurityException.class,
        () -> NativeVaultRenewal.requireCurrent(true, a.deadline, a, current),
        "original_expiry_still_binding");
    cases++;
  }

  private static void insufficientBudgetAndClipping() throws Exception {
    Authority a = new Authority();
    AtomicInteger calls = new AtomicInteger();
    reject(SocketTimeoutException.class, () -> NativeVaultRenewal.execute(true, true,
        a.now() + 3000, a, a.capture(), timeout -> {
          calls.incrementAndGet(); a.advance(501);
          throw new SocketTimeoutException("synthetic");
        }), "insufficient_remaining_budget");
    check(calls.get() == 1, "no_partial_second_attempt");
    Authority clipped = new Authority();
    Integer observed = NativeVaultRenewal.execute(true, true, clipped.now() + 1200,
        clipped, clipped.capture(), timeout -> { clipped.advance(100); return timeout; });
    check(observed == 1200 && clipped.deadline == 9000, "first_attempt_clipped_positive");
    AtomicInteger none = new AtomicInteger();
    reject(GeneralSecurityException.class,
        () -> NativeVaultRenewal.execute(true, true, clipped.now(), clipped, clipped.capture(),
            timeout -> { none.incrementAndGet(); return "unexpected"; }), "already_expired");
    check(none.get() == 0, "expired_before_transport");
    cases++;
  }

  private static void authorizationAndCertificateFailuresNeverRetry() throws Exception {
    Exception[] failures = {
      new GeneralSecurityException("HTTP_401"), new GeneralSecurityException("HTTP_403"),
      new GeneralSecurityException("wrong_actor"), new GeneralSecurityException("wrong_epoch"),
      new GeneralSecurityException("permission_denied"), new SSLHandshakeException("synthetic"),
      new SSLPeerUnverifiedException("synthetic"), new CertificateException("synthetic"),
      new IOException("synthetic"), new IOException(new SocketTimeoutException("wrapped"))
    };
    for (Exception failure : failures) {
      Authority a = new Authority(); AtomicInteger calls = new AtomicInteger();
      reject(failure.getClass(), () -> NativeVaultRenewal.execute(true, true,
          a.deadline, a, a.capture(), timeout -> { calls.incrementAndGet(); throw failure; }),
          "non_transport_failure");
      check(calls.get() == 1, "non_timeout_one_attempt");
    }
    cases++;
  }

  private static void actorChangeOrLockRejectsLateFlight() throws Exception {
    for (int mutation = 0; mutation < 4; mutation++) {
      for (boolean timeoutFirst : new boolean[]{false, true}) {
        Authority a = new Authority();
        CountDownLatch entered = new CountDownLatch(1), release = new CountDownLatch(1);
        AtomicInteger calls = new AtomicInteger();
        AtomicReference<Throwable> failure = new AtomicReference<>();
        AtomicReference<String> value = new AtomicReference<>();
        NativeVaultRenewal.Current current = a.capture();
        Thread flight = new Thread(() -> {
          try {
            value.set(NativeVaultRenewal.execute(true, true, a.deadline, a, current, budget -> {
              calls.incrementAndGet(); entered.countDown();
              if (!release.await(2, TimeUnit.SECONDS)) throw new IOException("test_latch_timeout");
              if (timeoutFirst) throw new SocketTimeoutException("synthetic");
              return "late";
            }));
          } catch (Throwable error) { failure.set(error); }
        });
        flight.start();
        check(entered.await(2, TimeUnit.SECONDS), "flight_entered");
        if (mutation == 0) { a.actor = "synthetic-b"; a.generation++; }
        else if (mutation == 1) { a.alive = false; a.generation++; }
        else if (mutation == 2) a.sid = "synthetic-new-session";
        else a.epoch++;
        release.countDown(); flight.join(2500);
        check(!flight.isAlive(), "flight_finished");
        check(failure.get() instanceof GeneralSecurityException && value.get() == null,
            "stale_flight_not_published");
        check(calls.get() == 1 && a.deadline == 9000, "stale_flight_not_retried");
      }
    }
    cases++;
  }

  private static void publicationRequiresSecondCurrentGate() throws Exception {
    Authority a = new Authority(); NativeVaultRenewal.Current current = a.capture();
    String value = NativeVaultRenewal.execute(true, true, a.deadline, a, current, timeout -> "verified");
    check(value.equals("verified"), "verified_response_before_transition");
    a.generation++;
    reject(GeneralSecurityException.class,
        () -> NativeVaultRenewal.requireCurrent(true, a.deadline, a, current),
        "transition_before_publication");
    cases++;
  }

  private static void initialUnlockAndChangedProofAreSingleAttempt() throws Exception {
    for (boolean existing : new boolean[]{false, true}) {
      Authority a = new Authority(); AtomicInteger calls = new AtomicInteger();
      reject(SocketTimeoutException.class,
          () -> NativeVaultRenewal.execute(existing, false, existing ? a.deadline : 0,
              a, a.capture(), timeout -> {
                calls.incrementAndGet(); a.advance(1500);
                throw new SocketTimeoutException("synthetic");
              }), "fresh_or_ineligible_no_retry");
      check(calls.get() == 1, "fresh_transport_once");
    }
    Authority a = new Authority(); AtomicInteger calls = new AtomicInteger();
    reject(SocketTimeoutException.class,
        () -> NativeVaultRenewal.execute(false, true, 0, a, a.capture(), timeout -> {
          calls.incrementAndGet(); a.advance(1500); throw new SocketTimeoutException("synthetic");
        }), "initial_cannot_opt_into_retry");
    check(calls.get() == 1, "initial_no_previous_lease");
    cases++;
  }

  private static void lateSuccessIsAnAbsoluteRequestTimeout() throws Exception {
    Authority a = new Authority(); AtomicInteger calls = new AtomicInteger();
    String accepted = NativeVaultRenewal.execute(true, true, a.deadline, a, a.capture(), timeout -> {
      if (calls.incrementAndGet() == 1) { a.advance(timeout); return "late-first-200"; }
      a.advance(timeout - 1); return "timely-second";
    });
    check(accepted.equals("timely-second") && calls.get() == 2, "late_200_not_accepted");
    Authority fresh = new Authority(); AtomicInteger freshCalls = new AtomicInteger();
    reject(SocketTimeoutException.class,
        () -> NativeVaultRenewal.execute(false, true, 0, fresh, fresh.capture(), timeout -> {
          freshCalls.incrementAndGet(); fresh.advance(timeout); return "late-fresh-200";
        }), "fresh_deadline_also_enforced");
    check(freshCalls.get() == 1, "fresh_late_success_not_retried");
    Authority expired = new Authority(); AtomicInteger expiryCalls = new AtomicInteger();
    reject(GeneralSecurityException.class,
        () -> NativeVaultRenewal.execute(true, true, expired.deadline, expired,
            expired.capture(), timeout -> {
              expiryCalls.incrementAndGet(); expired.advance(8000); return "expired";
            }), "late_result_past_original_lease");
    check(expiryCalls.get() == 1, "expired_result_no_retry");
    cases++;
  }

  public static void main(String[] args) throws Exception {
    oneTimeoutThenSuccess(); repeatedFailureNeverExtendsLease(); insufficientBudgetAndClipping();
    authorizationAndCertificateFailuresNeverRetry(); actorChangeOrLockRejectsLateFlight();
    publicationRequiresSecondCurrentGate(); initialUnlockAndChangedProofAreSingleAttempt();
    lateSuccessIsAnAbsoluteRequestTimeout();
    System.out.println("{\"passed\":true,\"cases\":" + cases + ",\"checks\":" + checks
        + ",\"productionRenewalHelper\":true,\"controlledClockAndLatch\":true,"
        + "\"brokerInstantiated\":false,\"networkOrDeviceUsed\":false}");
  }
}
