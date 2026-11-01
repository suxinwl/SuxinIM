package com.cloudwebrtc.webrtc;

import java.util.ArrayDeque;
import java.util.concurrent.ArrayBlockingQueue;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.Executor;
import java.util.concurrent.Executors;
import java.util.concurrent.RejectedExecutionException;
import java.util.concurrent.ScheduledExecutorService;
import java.util.concurrent.ThreadFactory;
import java.util.concurrent.ThreadPoolExecutor;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicInteger;
import java.util.function.Consumer;

/** Bounded, per-owner serial native work. Main never waits for native work. */
final class NativeMediaLifecycle {
  static final class Failure extends RuntimeException {
    final String code;
    Failure(String code, String message) { super(message); this.code = code; }
  }
  interface Response { void success(); void error(String code, String message); }
  interface Lease { CompletableFuture<Void> release(); Object key(); }
  private static ThreadFactory threads(String prefix) {
    AtomicInteger next = new AtomicInteger();
    return action -> { Thread thread = new Thread(action, prefix + next.incrementAndGet()); thread.setDaemon(true); return thread; };
  }
  private static final Executor WORKERS = new ThreadPoolExecutor(2, 2, 0, TimeUnit.MILLISECONDS,
      new ArrayBlockingQueue<>(32), threads("OpenIMRtcMedia-"), new ThreadPoolExecutor.AbortPolicy());
  private static final ScheduledExecutorService DEADLINES = Executors.newSingleThreadScheduledExecutor(threads("OpenIMRtcMediaDeadline-"));
  private static final AtomicInteger JOB_IDS = new AtomicInteger();

  /** Caller deadlines retain their exact physical barrier, never a replacement job. */
  static final class PhysicalDeadlines {
    private final java.util.IdentityHashMap<CompletableFuture<Void>, Integer> pending = new java.util.IdentityHashMap<>();
    private final Consumer<String> diagnostics;
    private final Runnable onPhysicalFailure;
    private final int limit;
    PhysicalDeadlines(int limit, Consumer<String> diagnostics, Runnable onPhysicalFailure) {
      this.limit = limit; this.diagnostics = diagnostics; this.onPhysicalFailure = onPhysicalFailure;
    }
    synchronized int pendingCount() { return pending.size(); }
    void timedOut(CompletableFuture<Void> physical, String operation) {
      final int id;
      final boolean overflow;
      synchronized (this) {
        if (pending.containsKey(physical)) return;
        overflow = pending.size() >= limit;
        id = JOB_IDS.incrementAndGet();
        if (!overflow) pending.put(physical, id);
      }
      final String label = safeLabel(operation);
      if (overflow) {
        onPhysicalFailure.run(); emit("deadline=" + id + " operation=" + label + " phase=deadlineOverflow"); return;
      }
      emit("deadline=" + id + " operation=" + label + " phase=callerTimeout physicalPending=" + !physical.isDone());
      physical.whenComplete((unused, failure) -> {
        // Publish permanent failure before removing the temporary barrier.
        // No callback/continuation runs while the identity-map lock is held.
        if (failure != null) onPhysicalFailure.run();
        synchronized (this) { pending.remove(physical); }
        emit("deadline=" + id + " operation=" + label + " phase=physicalOutcome outcome="
          + (failure == null ? "success" : "failure") + " deadlinePending=" + pendingCount()
          + (failure == null ? "" : " failure=" + unwrap(failure).getClass().getSimpleName()));
      });
    }
    private void emit(String event) { try { diagnostics.accept(event); } catch (Throwable ignored) {} }
  }

  private static String safeLabel(String label) {
    if (label == null) return "unspecified";
    final String clean = label.replaceAll("[^A-Za-z0-9_.-]", "_");
    return clean.substring(0, Math.min(64, clean.length()));
  }

  static final class Owner {
    private final Executor workers;
    private final Consumer<Runnable> exclusive;
    private final int limit;
    private final Consumer<String> diagnostics;
    private final ArrayDeque<Job> jobs = new ArrayDeque<>();
    private boolean running;
    private volatile boolean closing;
    private volatile boolean quarantined;
    private volatile String quarantineReason = "none";
    private final PhysicalDeadlines deadlines;
    private final java.util.Set<Lease> leases = java.util.concurrent.ConcurrentHashMap.newKeySet();
    Owner() { this(WORKERS, Runnable::run, 8, unused -> {}); }
    Owner(Consumer<Runnable> exclusive) { this(WORKERS, exclusive, 8, unused -> {}); }
    Owner(Consumer<Runnable> exclusive, Consumer<String> diagnostics) { this(WORKERS, exclusive, 8, diagnostics); }
    Owner(Executor workers, Consumer<Runnable> exclusive, int limit) {
      this(workers, exclusive, limit, unused -> {});
    }
    Owner(Executor workers, Consumer<Runnable> exclusive, int limit, Consumer<String> diagnostics) {
      this.workers = workers; this.exclusive = exclusive; this.limit = limit; this.diagnostics = diagnostics;
      this.deadlines = new PhysicalDeadlines(32, this::emitState, () -> quarantine("deadline_physical_failure"));
    }
    boolean isClosing() { return closing; }
    // Failure is published before its pending entry is removed. Read in the
    // matching order so false(old hard) + zero(new pending) cannot admit work.
    boolean isQuarantined() { return deadlines.pendingCount() != 0 || quarantined; }
    boolean isPermanentlyQuarantined() { return quarantined; }
    void closeForNewWork() { closing = true; emitState("phase=closing"); }
    void quarantine() { quarantine("explicit_failure"); }
    private void quarantine(String reason) { quarantineReason = reason; quarantined = true; emitState("phase=quarantine reason=" + reason); }
    void callerTimedOut(CompletableFuture<Void> physical, String operation) { deadlines.timedOut(physical, operation); }
    String stateSummary() {
      final int nativePending;
      synchronized (this) { nativePending = jobs.size() + (running ? 1 : 0); }
      return "closing=" + closing + " permanentFailure=" + quarantined + " reason=" + quarantineReason
        + " deadlinePending=" + deadlines.pendingCount() + " nativePending=" + nativePending;
    }
    void register(Lease lease) { leases.add(lease); }
    void unregister(Lease lease) { leases.remove(lease); }
    CompletableFuture<Void> detachAll() {
      closeForNewWork();
      return CompletableFuture.allOf(leases.stream().map(Lease::release).toArray(CompletableFuture[]::new));
    }
    CompletableFuture<Void> detachMatching(Object key) {
      return CompletableFuture.allOf(leases.stream().filter(lease -> lease.key() == key).map(Lease::release).toArray(CompletableFuture[]::new));
    }

    CompletableFuture<Void> submit(Runnable action, boolean cleanup) {
      return submit("media", action, cleanup);
    }
    CompletableFuture<Void> submit(String label, Runnable action, boolean cleanup) {
      Job job = new Job(safeLabel(label), action);
      boolean dispatch = false;
      Failure rejected = null;
      boolean queueFull = false;
      synchronized (this) {
        if (!cleanup && (closing || isQuarantined())) rejected = new Failure("RTC_MEDIA_UNAVAILABLE", "Native media owner is closing or quarantined");
        else if (jobs.size() + (running ? 1 : 0) >= limit) {
          quarantined = true; quarantineReason = "owner_queue_full"; queueFull = true;
          rejected = new Failure("RTC_MEDIA_BUSY", "Bounded native media owner queue is occupied");
        } else { jobs.add(job); dispatch = !running; if (dispatch) running = true; }
      }
      if (rejected != null) {
        emitState("job=" + job.id + " operation=" + job.label + " phase=rejected reason=" + (queueFull ? "owner_queue_full" : "owner_unavailable"));
        return failed(rejected.code, rejected.getMessage());
      }
      emit(job, "queued", 0, null);
      if (dispatch) dispatch();
      return job.physical;
    }
    private void dispatch() {
      final Job job;
      synchronized (this) { job = jobs.poll(); if (job == null) { running = false; return; } }
      try {
        workers.execute(() -> {
          Throwable failure = null;
          emit(job, "workerStart", elapsed(job.queuedAt), null);
          final long waiting = System.nanoTime();
          try { exclusive.accept(() -> {
            emit(job, "ownerAcquired", elapsed(waiting), null);
            final long started = System.nanoTime();
            Throwable nativeFailure = null;
            try { job.action.run(); }
            catch (Throwable error) { nativeFailure = error; throw error; }
            finally { emit(job, "actionEnd", elapsed(started), nativeFailure); }
          }); } catch (Throwable error) { failure = error; quarantine("native_action_failure"); }
          // All native locks are released before completion invokes continuations.
          if (failure == null) job.physical.complete(null); else job.physical.completeExceptionally(failure);
          emit(job, "physicalComplete", elapsed(job.queuedAt), failure);
          dispatch();
        });
      } catch (RejectedExecutionException full) {
        quarantine("worker_rejected"); job.physical.completeExceptionally(new Failure("RTC_MEDIA_BUSY", "Bounded native media worker queue is occupied"));
        // Do not retry jobs against a full executor or grow a replacement pool.
        final java.util.List<Job> rejected;
        synchronized (this) {
          rejected = new java.util.ArrayList<>(jobs);
          jobs.clear(); running = false;
        }
        for (Job queued : rejected) queued.physical.completeExceptionally(new Failure("RTC_MEDIA_BUSY", "Native media dispatch was rejected"));
      }
    }
    private void emitState(String event) {
      try { diagnostics.accept(event + " " + stateSummary()); }
      catch (Throwable ignored) { /* Diagnostics cannot change native lifetime. */ }
    }
    private void emit(Job job, String phase, long millis, Throwable failure) {
      // Diagnostics are deliberately no-token metadata and never participate
      // in ownership/error control or invoke callbacks under the owner lock.
      try { diagnostics.accept("job=" + job.id + " operation=" + job.label + " phase=" + phase + " elapsedMs=" + millis
        + " thread=" + Thread.currentThread().getName() + (failure == null ? "" : " failure=" + failure.getClass().getSimpleName())); }
      catch (Throwable ignored) { /* A log sink cannot change native lifetime. */ }
    }
  }
  private static long elapsed(long start) { return TimeUnit.NANOSECONDS.toMillis(System.nanoTime() - start); }
  private static final class Job {
    final int id = JOB_IDS.incrementAndGet();
    final long queuedAt = System.nanoTime();
    final String label;
    final Runnable action; final CompletableFuture<Void> physical = new CompletableFuture<>();
    Job(String label, Runnable action) { this.label = label; this.action = action; }
  }
  static CompletableFuture<Void> failed(String code, String message) {
    CompletableFuture<Void> result = new CompletableFuture<>(); result.completeExceptionally(new Failure(code, message)); return result;
  }
  static Throwable unwrap(Throwable error) {
    while ((error instanceof java.util.concurrent.CompletionException || error instanceof java.util.concurrent.ExecutionException) && error.getCause() != null) error = error.getCause();
    return error;
  }
  /** A caller deadline never completes or cancels the physical native barrier. */
  static void report(CompletableFuture<Void> physical, Response response, Runnable onTimeout) {
    report(physical, response, onTimeout, 12000);
  }
  static void report(CompletableFuture<Void> physical, Response response, Runnable onTimeout, long millis) {
    java.util.concurrent.atomic.AtomicBoolean answered = new java.util.concurrent.atomic.AtomicBoolean();
    java.util.concurrent.ScheduledFuture<?> timer = DEADLINES.schedule(() -> {
      if (answered.compareAndSet(false, true)) { onTimeout.run(); response.error("RTC_MEDIA_TIMEOUT", "Native media operation is still in progress"); }
    }, millis, TimeUnit.MILLISECONDS);
    physical.whenComplete((unused, error) -> {
      if (answered.compareAndSet(false, true)) {
        timer.cancel(false);
        if (error == null) response.success();
        else { Throwable cause = unwrap(error); response.error(cause instanceof Failure ? ((Failure) cause).code : "RTC_MEDIA_FAILED", cause.getClass().getSimpleName()); }
      }
    });
  }
}
