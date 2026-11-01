package com.cloudwebrtc.webrtc;

import java.util.ArrayList;
import java.util.List;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicInteger;
import java.util.concurrent.atomic.AtomicReference;

/** Independent production Owner/PeerLifecycle tests; native JNI is a fake Runnable. */
public final class NativePeerLifecycleIndependentTest {
  static void check(boolean value, String message) { if (!value) throw new AssertionError(message); }
  static void await(CountDownLatch latch) throws Exception {
    check(latch.await(3, TimeUnit.SECONDS), "test latch timed out");
  }
  static void complete(CompletableFuture<?> future) throws Exception { future.get(3, TimeUnit.SECONDS); }
  static Throwable failure(CompletableFuture<?> future) throws Exception {
    try { complete(future); throw new AssertionError("expected failure"); }
    catch (java.util.concurrent.ExecutionException error) { return NativeMediaLifecycle.unwrap(error); }
  }
  static void block(CountDownLatch entered, CountDownLatch release) {
    entered.countDown();
    try { check(release.await(3, TimeUnit.SECONDS), "test native operation never released"); }
    catch (InterruptedException error) { throw new RuntimeException(error); }
  }
  static ExecutorService worker() {
    return Executors.newSingleThreadExecutor(action -> {
      Thread thread = new Thread(action, "IndependentPeerNativeFake"); thread.setDaemon(true); return thread;
    });
  }
  static NativeMediaLifecycle.Owner owner(ExecutorService worker) {
    return new NativeMediaLifecycle.Owner(worker, Runnable::run, 8);
  }
  static final class Lease implements NativeMediaLifecycle.Lease {
    final CompletableFuture<Void> physical = new CompletableFuture<>();
    final AtomicInteger calls = new AtomicInteger();
    public CompletableFuture<Void> release() { calls.incrementAndGet(); return physical; }
    public Object key() { return this; }
  }
  static final class Response implements NativeMediaLifecycle.Response {
    final AtomicInteger calls = new AtomicInteger();
    final CountDownLatch answered = new CountDownLatch(1);
    volatile String error;
    public void success() { calls.incrementAndGet(); answered.countDown(); }
    public void error(String code, String message) { error = code; calls.incrementAndGet(); answered.countDown(); }
  }

  static void pendingCloseAndDisposeCoalescePhysicalOwnership() throws Exception {
    ExecutorService worker = worker();
    CountDownLatch entered = new CountDownLatch(1), release = new CountDownLatch(1);
    try {
      AtomicInteger closes = new AtomicInteger(), disposes = new AtomicInteger();
      NativePeerLifecycle peer = new NativePeerLifecycle(owner(worker), () -> {
        closes.incrementAndGet(); block(entered, release);
      }, disposes::incrementAndGet);
      long start = System.nanoTime();
      CompletableFuture<Void> close = peer.close();
      check(TimeUnit.NANOSECONDS.toMillis(System.nanoTime() - start) < 200, "caller waited for native close");
      await(entered);
      CompletableFuture<Void> dispose = peer.dispose();
      check(peer.close() == close && peer.dispose() == dispose, "pending physical Future not shared");
      check(!close.isDone() && !dispose.isDone() && disposes.get() == 0, "dispose bypassed native close");
      release.countDown(); complete(close); complete(dispose);
      check(closes.get() == 1 && disposes.get() == 1, "JNI was repeated");
      check(peer.close() == close && peer.dispose() == dispose, "completed operations lost identity");
    } finally { release.countDown(); worker.shutdownNow(); }
  }

  static void synchronousNativeReentrySharesPublishedFutures() throws Exception {
    ExecutorService worker = worker();
    try {
      AtomicReference<NativePeerLifecycle> ref = new AtomicReference<>();
      AtomicReference<CompletableFuture<Void>> nestedClose = new AtomicReference<>(), nestedDispose = new AtomicReference<>();
      AtomicInteger closes = new AtomicInteger(), disposes = new AtomicInteger();
      NativePeerLifecycle peer = new NativePeerLifecycle(owner(worker), () -> {
        closes.incrementAndGet(); nestedClose.set(ref.get().close()); nestedDispose.set(ref.get().dispose());
      }, disposes::incrementAndGet);
      ref.set(peer);
      CompletableFuture<Void> close = peer.close(); complete(close); complete(peer.dispose());
      check(nestedClose.get() == close, "reentrant close created another physical operation");
      check(nestedDispose.get() == peer.dispose(), "reentrant dispose lost its shared Future");
      check(closes.get() == 1 && disposes.get() == 1, "reentrant native calls repeated JNI");
    } finally { worker.shutdownNow(); }
  }

  static void callerDeadlineDoesNotReleasePhysicalCloseOrReinvokeJni() throws Exception {
    ExecutorService worker = worker();
    try {
      NativeMediaLifecycle.Owner owner = owner(worker); Lease lease = new Lease(); owner.register(lease);
      AtomicInteger closes = new AtomicInteger(), disposes = new AtomicInteger();
      NativePeerLifecycle peer = new NativePeerLifecycle(owner, closes::incrementAndGet, disposes::incrementAndGet);
      CompletableFuture<Void> close = peer.close(), dispose = peer.dispose();
      Response firstCaller = new Response();
      NativeMediaLifecycle.report(close, firstCaller, owner::quarantine, 20);
      await(firstCaller.answered);
      check("RTC_MEDIA_TIMEOUT".equals(firstCaller.error), "expected caller deadline");
      check(!close.isDone() && !dispose.isDone(), "deadline completed physical barrier");
      check(closes.get() == 0 && disposes.get() == 0, "peer closed before physical sink detach");
      check(peer.close() == close && peer.dispose() == dispose, "deadline caused a new JNI request");
      Response nextCaller = new Response(); NativeMediaLifecycle.report(dispose, nextCaller, owner::quarantine, 1000);
      lease.physical.complete(null); complete(close); complete(dispose); await(nextCaller.answered);
      Thread.sleep(35);
      check(firstCaller.calls.get() == 1 && nextCaller.calls.get() == 1 && nextCaller.error == null,
          "late physical success replied twice or failed a joined observer");
      check(closes.get() == 1 && disposes.get() == 1 && lease.calls.get() == 1, "physical ownership repeated");
    } finally { worker.shutdownNow(); }
  }

  static void trueNativeCloseFailureNeverRetriesOrDisposes() throws Exception {
    ExecutorService worker = worker();
    try {
      RuntimeException expected = new RuntimeException("synthetic JNI close failure");
      AtomicInteger closes = new AtomicInteger(), disposes = new AtomicInteger();
      NativePeerLifecycle peer = new NativePeerLifecycle(owner(worker), () -> { closes.incrementAndGet(); throw expected; }, disposes::incrementAndGet);
      CompletableFuture<Void> close = peer.close(), dispose = peer.dispose();
      check(failure(close) == expected && failure(dispose) == expected, "first physical failure not preserved");
      check(peer.close() == close && peer.dispose() == dispose, "failed JNI operation was reopened");
      check(closes.get() == 1 && disposes.get() == 0, "unsafe JNI retry or disposal after failed close");
    } finally { worker.shutdownNow(); }
  }

  static void failedDetachNeverEntersJni() throws Exception {
    ExecutorService worker = worker();
    try {
      NativeMediaLifecycle.Owner owner = owner(worker); Lease lease = new Lease(); owner.register(lease);
      RuntimeException expected = new RuntimeException("synthetic removeSink failure");
      AtomicInteger closes = new AtomicInteger(), disposes = new AtomicInteger();
      NativePeerLifecycle peer = new NativePeerLifecycle(owner, closes::incrementAndGet, disposes::incrementAndGet);
      CompletableFuture<Void> close = peer.close(), dispose = peer.dispose(); lease.physical.completeExceptionally(expected);
      check(failure(close) == expected && failure(dispose) == expected, "detach error not preserved");
      check(peer.close() == close && peer.dispose() == dispose, "failed detach barrier reopened");
      check(closes.get() == 0 && disposes.get() == 0 && lease.calls.get() == 1, "JNI or detach repeated after failure");
    } finally { worker.shutdownNow(); }
  }

  static void fullOwnerQueueRetainsFailedPeerBarrier() throws Exception {
    ExecutorService worker = worker(); CountDownLatch entered = new CountDownLatch(1), release = new CountDownLatch(1);
    try {
      NativeMediaLifecycle.Owner owner = new NativeMediaLifecycle.Owner(worker, Runnable::run, 1);
      CompletableFuture<Void> occupied = owner.submit(() -> block(entered, release), true); await(entered);
      AtomicInteger closes = new AtomicInteger(), disposes = new AtomicInteger();
      NativePeerLifecycle peer = new NativePeerLifecycle(owner, closes::incrementAndGet, disposes::incrementAndGet);
      CompletableFuture<Void> close = peer.close(), dispose = peer.dispose();
      Throwable expected = failure(close); check(expected instanceof NativeMediaLifecycle.Failure, "missing bounded queue error");
      check(failure(dispose) == expected, "queue rejection not preserved across dispose");
      release.countDown(); complete(occupied);
      check(peer.close() == close && peer.dispose() == dispose, "queue error was retried after capacity returned");
      check(closes.get() == 0 && disposes.get() == 0, "rejected cleanup still entered JNI");
    } finally { release.countDown(); worker.shutdownNow(); }
  }

  static void parallelCallersGetOneFuturePerOperation() throws Exception {
    ExecutorService worker = worker(), callers = Executors.newFixedThreadPool(8);
    CountDownLatch entered = new CountDownLatch(1), release = new CountDownLatch(1);
    try {
      AtomicInteger closes = new AtomicInteger(), disposes = new AtomicInteger();
      NativePeerLifecycle peer = new NativePeerLifecycle(owner(worker), () -> { closes.incrementAndGet(); block(entered, release); }, disposes::incrementAndGet);
      List<java.util.concurrent.Future<CompletableFuture<Void>>> closeCalls = new ArrayList<>(), disposeCalls = new ArrayList<>();
      for (int i = 0; i < 16; i++) { closeCalls.add(callers.submit(peer::close)); disposeCalls.add(callers.submit(peer::dispose)); }
      await(entered);
      CompletableFuture<Void> close = closeCalls.get(0).get(3, TimeUnit.SECONDS), dispose = disposeCalls.get(0).get(3, TimeUnit.SECONDS);
      for (java.util.concurrent.Future<CompletableFuture<Void>> result : closeCalls) check(result.get(3, TimeUnit.SECONDS) == close, "parallel close Future differs");
      for (java.util.concurrent.Future<CompletableFuture<Void>> result : disposeCalls) check(result.get(3, TimeUnit.SECONDS) == dispose, "parallel dispose Future differs");
      release.countDown(); complete(close); complete(dispose);
      check(closes.get() == 1 && disposes.get() == 1, "parallel callers repeated JNI");
    } finally { release.countDown(); callers.shutdownNow(); worker.shutdownNow(); }
  }

  static void disposeFailureIsLatchedAndDoesNotRepeatClose() throws Exception {
    ExecutorService worker = worker();
    try {
      AtomicInteger closes = new AtomicInteger(), disposes = new AtomicInteger(); RuntimeException expected = new RuntimeException("synthetic JNI dispose failure");
      NativePeerLifecycle peer = new NativePeerLifecycle(owner(worker), closes::incrementAndGet, () -> { disposes.incrementAndGet(); throw expected; });
      CompletableFuture<Void> dispose = peer.dispose(); check(failure(dispose) == expected, "dispose failure not propagated");
      complete(peer.close()); check(peer.dispose() == dispose, "failed dispose reopened");
      check(closes.get() == 1 && disposes.get() == 1, "dispose failure repeated JNI");
    } finally { worker.shutdownNow(); }
  }

  public static void main(String[] args) throws Exception {
    pendingCloseAndDisposeCoalescePhysicalOwnership(); System.out.println("PASS pending close/dispose physical coalescing");
    synchronousNativeReentrySharesPublishedFutures(); System.out.println("PASS synchronous native reentry");
    callerDeadlineDoesNotReleasePhysicalCloseOrReinvokeJni(); System.out.println("PASS caller deadline / late physical success");
    trueNativeCloseFailureNeverRetriesOrDisposes(); System.out.println("PASS native close failure latch");
    failedDetachNeverEntersJni(); System.out.println("PASS failed detach ownership retained");
    fullOwnerQueueRetainsFailedPeerBarrier(); System.out.println("PASS bounded owner rejection latch");
    parallelCallersGetOneFuturePerOperation(); System.out.println("PASS parallel callers share physical futures");
    disposeFailureIsLatchedAndDoesNotRepeatClose(); System.out.println("PASS failed disposal latch");
  }
}
