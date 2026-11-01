package com.cloudwebrtc.webrtc;

import java.util.concurrent.CompletableFuture;
import java.util.concurrent.Executor;
import java.util.function.Supplier;

/** Bounded async physical sequence. No worker waits for work on its own queue. */
final class NativeCleanupSequence {
  private final Executor main;
  private CompletableFuture<Void> tail = CompletableFuture.completedFuture(null);
  private int pending;
  private boolean closed;
  private CompletableFuture<Void> shutdown;
  NativeCleanupSequence(Executor main) { this.main = main; }
  synchronized boolean isPending() { return pending != 0 || tail.isCompletedExceptionally(); }
  synchronized String stateSummary() { return "sequencePending=" + pending + " sequenceFailed=" + tail.isCompletedExceptionally() + " sequenceClosed=" + closed; }
  CompletableFuture<Void> submit(Supplier<CompletableFuture<Void>> operation) {
    return enqueue(operation, false);
  }
  CompletableFuture<Void> shutdown(Supplier<CompletableFuture<Void>> operation) {
    return enqueue(operation, true);
  }
  private CompletableFuture<Void> enqueue(Supplier<CompletableFuture<Void>> operation, boolean finalCleanup) {
    final CompletableFuture<Void> previous, done = new CompletableFuture<>();
    synchronized (this) {
      if (finalCleanup && shutdown != null) return shutdown;
      if (!finalCleanup && closed) return NativeMediaLifecycle.failed("RTC_MEDIA_UNAVAILABLE", "Local cleanup sequence is closed");
      if (pending >= (finalCleanup ? 9 : 8)) return NativeMediaLifecycle.failed("RTC_MEDIA_BUSY", "Bounded local cleanup queue is occupied");
      if (finalCleanup) { closed = true; shutdown = done; }
      previous = tail; tail = done; pending++;
    }
    done.whenComplete((unused, error) -> { synchronized (this) { pending--; } });
    previous.whenComplete((unused, error) -> {
      if (error != null) { done.completeExceptionally(NativeMediaLifecycle.unwrap(error)); return; }
      try {
        main.execute(() -> {
          try {
            operation.get().whenComplete((value, failure) -> {
              if (failure == null) done.complete(null); else done.completeExceptionally(NativeMediaLifecycle.unwrap(failure));
            });
          } catch (Throwable failure) { done.completeExceptionally(failure); }
        });
      } catch (Throwable rejected) { done.completeExceptionally(rejected); }
    });
    return done;
  }
}
