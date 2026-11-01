package com.cloudwebrtc.webrtc;

import java.util.concurrent.ArrayBlockingQueue;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.RejectedExecutionException;
import java.util.concurrent.ScheduledExecutorService;
import java.util.concurrent.ThreadFactory;
import java.util.concurrent.ThreadPoolExecutor;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicBoolean;
import java.util.concurrent.atomic.AtomicInteger;

/** Keeps synchronous JNI stats/teardown off Android's platform thread. */
final class RtcOperationGate {
  interface Response {
    void success(Object value);
    void error(String code, String message);
  }
  interface Operation { void run(Response response); }

  private static ThreadFactory threads(String name) {
    AtomicInteger ids = new AtomicInteger();
    return runnable -> {
      Thread thread = new Thread(runnable, name + ids.incrementAndGet());
      thread.setDaemon(true);
      return thread;
    };
  }
  // A timed-out JNI call cannot be interrupted safely. Never replace that worker
  // with a new thread or queue another stats call for the quarantined peer.
  private static final ExecutorService STATS = new ThreadPoolExecutor(2, 2, 0,
      TimeUnit.MILLISECONDS, new ArrayBlockingQueue<>(8), threads("OpenIMRtcStats-"),
      new ThreadPoolExecutor.AbortPolicy());
  private static final ExecutorService CLEANUP = new ThreadPoolExecutor(2, 2, 0,
      TimeUnit.MILLISECONDS, new ArrayBlockingQueue<>(16), threads("OpenIMRtcCleanup-"),
      new ThreadPoolExecutor.AbortPolicy());
  private static final ScheduledExecutorService DEADLINES =
      Executors.newSingleThreadScheduledExecutor(threads("OpenIMRtcDeadline-"));
  private final Object nativeCall = new Object();
  private final Object completionState = new Object();
  private final AtomicBoolean inFlight = new AtomicBoolean();
  private final AtomicBoolean quarantined = new AtomicBoolean();
  private final AtomicBoolean closing = new AtomicBoolean();
  private final long statsTimeoutMs;
  private final long cleanupTimeoutMs;
  private volatile Once pending;

  RtcOperationGate() { this(3000, 12000); }
  RtcOperationGate(long statsTimeoutMs, long cleanupTimeoutMs) {
    this.statsTimeoutMs = statsTimeoutMs;
    this.cleanupTimeoutMs = cleanupTimeoutMs;
  }
  boolean isClosing() { return closing.get(); }
  void beginClosing() {
    closing.set(true);
    // A late successful collector callback deliberately suppresses stats once
    // closing. Settle its caller now; retain the physical JNI/callback lifetime.
    Once stats = pending;
    if (stats != null) stats.error("RTC_STATS_CLOSED", "Peer is closing");
  }
  // Used only by bounded media workers; Android main never enters this lock.
  void exclusiveNative(Runnable operation) { synchronized (nativeCall) { operation.run(); } }

  /** Disposal must still attempt native release if closing the peer rejected. */
  static void cleanupAll(Runnable... operations) {
    RuntimeException first = null;
    for (Runnable operation : operations) {
      try { operation.run(); }
      catch (RuntimeException error) { if (first == null) first = error; }
    }
    if (first != null) throw first;
  }

  private static final class Once implements Response {
    final AtomicBoolean completed = new AtomicBoolean();
    final Response target;
    Once(Response target) { this.target = target; }
    public void success(Object value) {
      if (completed.compareAndSet(false, true)) target.success(value);
    }
    public void error(String code, String message) {
      if (completed.compareAndSet(false, true)) target.error(code, message);
    }
  }

  void stats(Operation operation, Response response) {
    if (closing.get() || quarantined.get()) {
      response.error("RTC_STATS_UNAVAILABLE", "Peer is closing or stats are quarantined");
      return;
    }
    if (!inFlight.compareAndSet(false, true)) {
      response.error("RTC_STATS_BUSY", "A stats request is already in flight for this peer");
      return;
    }
    Once once = new Once(response);
    pending = once;
    AtomicBoolean nativeCompleted = new AtomicBoolean();
    java.util.concurrent.ScheduledFuture<?> deadline = DEADLINES.schedule(() -> {
      synchronized (completionState) {
        if (once.completed.compareAndSet(false, true)) {
          quarantined.set(true);
          once.target.error("RTC_STATS_TIMEOUT", "Native stats timed out; peer stats quarantined");
        }
      }
    }, statsTimeoutMs, TimeUnit.MILLISECONDS);
    Response completion = new Response() {
      public void success(Object value) {
        if (!nativeCompleted.compareAndSet(false, true)) return;
        synchronized (completionState) {
          deadline.cancel(false);
          inFlight.set(false);
          if (!closing.get()) once.success(value);
        }
      }
      public void error(String code, String message) {
        if (!nativeCompleted.compareAndSet(false, true)) return;
        synchronized (completionState) {
          deadline.cancel(false);
          inFlight.set(false);
          once.error(code, message);
        }
      }
    };
    try {
      STATS.execute(() -> {
        synchronized (nativeCall) {
          if (closing.get() || quarantined.get()) {
            completion.error("RTC_STATS_UNAVAILABLE", "Peer no longer accepts stats");
            return;
          }
          try { operation.run(completion); }
          catch (RuntimeException error) {
            completion.error("RTC_STATS_FAILED", error.getClass().getSimpleName());
          }
        }
      });
    } catch (RejectedExecutionException full) {
      completion.error("RTC_STATS_BUSY", "Bounded native stats executor is occupied");
    }
  }

  void cleanup(Runnable operation, Response response) {
    beginClosing();
    Once once = new Once(response);
    java.util.concurrent.ScheduledFuture<?> deadline = DEADLINES.schedule(() ->
        once.error("RTC_CLEANUP_TIMEOUT", "Native cleanup has not completed"),
        cleanupTimeoutMs, TimeUnit.MILLISECONDS);
    try {
      CLEANUP.execute(() -> {
        try {
          // Disposal cannot race a still-executing native invocation. Waiting
          // happens only on the bounded cleanup pool, never on Android main.
          synchronized (nativeCall) { operation.run(); }
          once.success(null);
        } catch (RuntimeException error) {
          once.error("RTC_CLEANUP_FAILED", error.getClass().getSimpleName());
        } finally { deadline.cancel(false); }
      });
    } catch (RejectedExecutionException full) {
      deadline.cancel(false);
      once.error("RTC_CLEANUP_BUSY", "Bounded native cleanup executor is occupied");
    }
  }
}
