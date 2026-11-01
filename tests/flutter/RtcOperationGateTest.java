package com.cloudwebrtc.webrtc;

import java.util.concurrent.CountDownLatch;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicInteger;
import java.util.concurrent.atomic.AtomicReference;

/** Runs with JDK only: blocked JNI is simulated without Android or the network. */
public final class RtcOperationGateTest {
  static final class Response implements RtcOperationGate.Response {
    final CountDownLatch finished = new CountDownLatch(1);
    final AtomicInteger count = new AtomicInteger();
    volatile String error;
    public void success(Object value) { count.incrementAndGet(); finished.countDown(); }
    public void error(String code, String message) {
      error = code; count.incrementAndGet(); finished.countDown();
    }
    void await() throws Exception {
      if (!finished.await(2, TimeUnit.SECONDS)) throw new AssertionError("missing response");
    }
  }
  static void check(boolean condition, String message) {
    if (!condition) throw new AssertionError(message);
  }
  public static void main(String[] args) throws Exception {
    RuntimeException closeFailure = new RuntimeException("synthetic close failure");
    RuntimeException releaseFailure = new RuntimeException("synthetic release failure");
    AtomicInteger releaseAttempts = new AtomicInteger();
    try {
      RtcOperationGate.cleanupAll(() -> { throw closeFailure; }, () -> {
        releaseAttempts.incrementAndGet();
        throw releaseFailure;
      });
      throw new AssertionError("native close failure was swallowed");
    } catch (RuntimeException error) {
      check(error == closeFailure, "cleanup replaced first close failure");
    }
    check(releaseAttempts.get() == 1, "close failure skipped native disposal");
    RtcOperationGate peer = new RtcOperationGate(100, 200);
    CountDownLatch entered = new CountDownLatch(1);
    CountDownLatch release = new CountDownLatch(1);
    Response first = new Response();
    long start = System.nanoTime();
    peer.stats(response -> {
      entered.countDown();
      try { release.await(); } catch (InterruptedException error) {
        throw new RuntimeException(error);
      }
      response.success(null);
      response.success(null); // A late or repeated callback must not reply twice.
    }, first);
    check(TimeUnit.NANOSECONDS.toMillis(System.nanoTime() - start) < 80,
        "caller blocked behind native work");
    check(entered.await(1, TimeUnit.SECONDS), "worker not entered");
    Response second = new Response(); peer.stats(r -> r.success(null), second);
    second.await(); check("RTC_STATS_BUSY".equals(second.error), "overlap was queued");
    first.await(); check("RTC_STATS_TIMEOUT".equals(first.error), "missing deadline");
    Response third = new Response(); peer.stats(r -> r.success(null), third);
    third.await(); check("RTC_STATS_UNAVAILABLE".equals(third.error), "peer not quarantined");
    RtcOperationGate other = new RtcOperationGate(100, 200);
    Response independent = new Response(); other.stats(r -> r.success(null), independent);
    independent.await(); check(independent.error == null, "one peer starved another");
    AtomicInteger disposed = new AtomicInteger(); Response cleanup = new Response();
    start = System.nanoTime(); peer.cleanup(disposed::incrementAndGet, cleanup);
    check(TimeUnit.NANOSECONDS.toMillis(System.nanoTime() - start) < 80,
        "cleanup blocked caller behind native lock");
    cleanup.await(); check("RTC_CLEANUP_TIMEOUT".equals(cleanup.error), "cleanup lied about completion");
    check(disposed.get() == 0, "disposal raced running native code");
    release.countDown();
    for (int i = 0; i < 100 && disposed.get() == 0; i++) Thread.sleep(10);
    check(disposed.get() == 1, "cleanup never completed after native returned");
    check(first.count.get() == 1 && cleanup.count.get() == 1, "late result delivered twice");
    Response closed = new Response(); peer.stats(r -> r.success(null), closed);
    closed.await(); check("RTC_STATS_UNAVAILABLE".equals(closed.error), "closed peer accepted stats");
    RtcOperationGate generations = new RtcOperationGate(500, 500);
    AtomicReference<RtcOperationGate.Response> oldCallback = new AtomicReference<>();
    CountDownLatch oldReady = new CountDownLatch(1); Response oldResult = new Response();
    generations.stats(r -> { oldCallback.set(r); oldReady.countDown(); }, oldResult);
    check(oldReady.await(1, TimeUnit.SECONDS), "first callback not captured");
    oldCallback.get().success(null); oldResult.await();
    AtomicReference<RtcOperationGate.Response> newCallback = new AtomicReference<>();
    CountDownLatch newReady = new CountDownLatch(1); Response newResult = new Response();
    generations.stats(r -> { newCallback.set(r); newReady.countDown(); }, newResult);
    check(newReady.await(1, TimeUnit.SECONDS), "new callback not captured");
    oldCallback.get().success(null);
    Response overlap = new Response(); generations.stats(r -> r.success(null), overlap);
    overlap.await(); check("RTC_STATS_BUSY".equals(overlap.error), "old callback released new in-flight request");
    newCallback.get().success(null); newResult.await();
    check(oldResult.count.get() == 1 && newResult.count.get() == 1, "duplicate callback crossed generations");
    Thread.sleep(550); // Successful requests must not later become quarantined.
    Response afterDeadline = new Response(); generations.stats(r -> r.success(null), afterDeadline);
    afterDeadline.await(); check(afterDeadline.error == null, "successful request was quarantined later");
    long statsThreads = Thread.getAllStackTraces().keySet().stream()
        .filter(t -> t.getName().startsWith("OpenIMRtcStats-")).count();
    long cleanupThreads = Thread.getAllStackTraces().keySet().stream()
        .filter(t -> t.getName().startsWith("OpenIMRtcCleanup-")).count();
    check(statsThreads <= 2 && cleanupThreads <= 2, "unbounded native threads");
    System.out.println("PASS: native release attempted after close failure with first error preserved, nonblocking caller, single-flight, deadline/quarantine, independent peer, serialized cleanup, late response once, closed peer rejection, stale callback isolation, completed deadline cancellation, bounded workers");
  }
}
