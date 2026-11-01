package com.cloudwebrtc.webrtc;

import java.util.ArrayDeque;
import java.util.ArrayList;
import java.util.List;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.Executor;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.RejectedExecutionException;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicInteger;
import java.util.concurrent.atomic.AtomicReference;

/** Actual production Binding/Owner with fake native track/output; JDK only. */
public final class NativeRendererBindingIndependentTest {
  static void check(boolean value, String message) {
    if (!value) throw new AssertionError(message);
  }
  static void await(CountDownLatch latch) throws Exception {
    check(latch.await(3, TimeUnit.SECONDS), "timed out waiting for test latch");
  }
  static void complete(CompletableFuture<?> future) throws Exception {
    future.get(3, TimeUnit.SECONDS);
  }
  static Throwable failure(CompletableFuture<?> future) throws Exception {
    try { complete(future); throw new AssertionError("expected failure"); }
    catch (java.util.concurrent.ExecutionException error) { return NativeMediaLifecycle.unwrap(error); }
  }
  static ExecutorService worker() {
    return Executors.newSingleThreadExecutor(action -> {
      Thread thread = new Thread(action, "IndependentNativeFake");
      thread.setDaemon(true); return thread;
    });
  }
  static final class Main implements Executor {
    private final ArrayDeque<Runnable> tasks = new ArrayDeque<>();
    public synchronized void execute(Runnable action) { tasks.add(action); }
    synchronized int size() { return tasks.size(); }
    void drain() {
      for (;;) {
        final Runnable action;
        synchronized (this) { action = tasks.poll(); }
        if (action == null) return;
        action.run();
      }
    }
  }
  static final class Track implements NativeRendererBinding.Track {
    final AtomicInteger attached = new AtomicInteger(), detached = new AtomicInteger();
    Runnable attaching = () -> {}, detaching = () -> {};
    public void attach() { attached.incrementAndGet(); attaching.run(); }
    public void detach() { detached.incrementAndGet(); detaching.run(); }
  }
  static final class Output implements NativeRendererBinding.Output {
    final AtomicInteger egl = new AtomicInteger(), producer = new AtomicInteger();
    Runnable releasingEgl = () -> {}, releasingProducer = () -> {};
    public void releaseEgl() { egl.incrementAndGet(); releasingEgl.run(); }
    public void releaseProducerOnMain() { producer.incrementAndGet(); releasingProducer.run(); }
  }
  static final class Response implements NativeMediaLifecycle.Response {
    final AtomicInteger calls = new AtomicInteger();
    final CountDownLatch answered = new CountDownLatch(1);
    volatile String error;
    public void success() { calls.incrementAndGet(); answered.countDown(); }
    public void error(String code, String message) { error = code; calls.incrementAndGet(); answered.countDown(); }
  }
  static void blockDetach(Track track, CountDownLatch entered, CountDownLatch release) {
    track.detaching = () -> {
      entered.countDown();
      try { check(release.await(3, TimeUnit.SECONDS), "test detach was never released"); }
      catch (InterruptedException error) { throw new RuntimeException(error); }
    };
  }
  static NativeMediaLifecycle.Owner owner(Executor worker) {
    return new NativeMediaLifecycle.Owner(worker, Runnable::run, 8);
  }
  static void drainUntil(Main main, CompletableFuture<?> future) throws Exception {
    long until = System.nanoTime() + TimeUnit.SECONDS.toNanos(3);
    while (!future.isDone() && System.nanoTime() < until) { main.drain(); Thread.sleep(1); }
    complete(future);
  }

  static void blockedDetachDoesNotBlockCallerOrClosePeerEarly() throws Exception {
    ExecutorService worker = worker(), outputWorker = worker();
    CountDownLatch entered = new CountDownLatch(1), release = new CountDownLatch(1);
    try {
      Main main = new Main(); Output output = new Output(); Track track = new Track();
      NativeMediaLifecycle.Owner owner = owner(worker);
      NativeRendererBinding binding = new NativeRendererBinding(main, output, owner(outputWorker));
      complete(binding.set(owner, track)); blockDetach(track, entered, release);
      long started = System.nanoTime();
      CompletableFuture<Void> detach = binding.set(null, null);
      check(TimeUnit.NANOSECONDS.toMillis(System.nanoTime() - started) < 200,
          "caller waited for blocking removeSink");
      await(entered);
      AtomicInteger peerClosed = new AtomicInteger();
      CompletableFuture<Void> peer = owner.detachAll().thenCompose(v -> owner.submit(peerClosed::incrementAndGet, true));
      CompletableFuture<Void> disposed = binding.dispose();
      check(!detach.isDone() && !peer.isDone() && !disposed.isDone(), "physical detach was reported as complete");
      check(peerClosed.get() == 0 && output.egl.get() == 0 && main.size() == 0,
          "peer/EGL/producer released before physical removeSink returned");
      release.countDown(); complete(detach); complete(peer); drainUntil(main, disposed);
      check(peerClosed.get() == 1 && output.egl.get() == 1 && output.producer.get() == 1,
          "physical release did not complete exactly once");
    } finally { release.countDown(); worker.shutdownNow(); outputWorker.shutdownNow(); }
  }

  static void callerDeadlineDoesNotAdvancePhysicalBarrier() throws Exception {
    ExecutorService worker = worker(), outputWorker = worker();
    CountDownLatch entered = new CountDownLatch(1), release = new CountDownLatch(1);
    try {
      Main main = new Main(); Output output = new Output(); Track track = new Track();
      NativeMediaLifecycle.Owner owner = owner(worker);
      NativeRendererBinding binding = new NativeRendererBinding(main, output, owner(outputWorker));
      complete(binding.set(owner, track)); blockDetach(track, entered, release);
      CompletableFuture<Void> detach = binding.set(null, null); await(entered);
      Response response = new Response();
      NativeMediaLifecycle.report(detach, response, binding::quarantine, 20);
      await(response.answered);
      check("RTC_MEDIA_TIMEOUT".equals(response.error), "caller did not receive bounded deadline");
      AtomicInteger peerClosed = new AtomicInteger();
      CompletableFuture<Void> peer = owner.detachAll().thenCompose(v -> owner.submit(peerClosed::incrementAndGet, true));
      CompletableFuture<Void> disposed = binding.dispose();
      check(!detach.isDone() && !peer.isDone() && !disposed.isDone(), "deadline completed/cancelled a physical future");
      check(peerClosed.get() == 0 && output.producer.get() == 0, "deadline released native ownership");
      release.countDown(); complete(detach); complete(peer); drainUntil(main, disposed);
      check(response.calls.get() == 1, "late native completion replied after timeout");
      Throwable rejected = failure(binding.set(owner, new Track()));
      check(rejected instanceof NativeMediaLifecycle.Failure, "timed-out renderer accepted new work");
    } finally { release.countDown(); worker.shutdownNow(); outputWorker.shutdownNow(); }
  }

  static void detachErrorRetainsTrackAndOutputOwnership() throws Exception {
    NativeMediaLifecycle.Owner owner = owner(Runnable::run);
    Track track = new Track(); Output output = new Output();
    NativeRendererBinding binding = new NativeRendererBinding(Runnable::run, output, owner(Runnable::run));
    complete(binding.set(owner, track));
    RuntimeException marker = new RuntimeException("synthetic removeSink failure");
    track.detaching = () -> { throw marker; };
    check(failure(binding.set(null, null)) == marker, "detach error was replaced");
    check(failure(owner.detachMatching(track)) == marker, "failed lease was forgotten");
    AtomicInteger closed = new AtomicInteger();
    check(failure(owner.detachAll().thenCompose(v -> owner.submit(closed::incrementAndGet, true))) == marker,
        "peer close bypassed failed physical detach");
    check(failure(binding.dispose()) == marker, "renderer disposal swallowed earlier failure");
    check(track.detached.get() == 1 && output.egl.get() == 0 && output.producer.get() == 0 && closed.get() == 0,
        "failed detach dropped references or released output/peer");
  }

  static void ownerQueueFullRetainsAttachedTrack() throws Exception {
    Main work = new Main(); NativeMediaLifecycle.Owner owner = new NativeMediaLifecycle.Owner(work, Runnable::run, 1);
    Track track = new Track(); Output output = new Output();
    NativeRendererBinding binding = new NativeRendererBinding(Runnable::run, output, owner(Runnable::run));
    CompletableFuture<Void> attached = binding.set(owner, track); work.drain(); complete(attached);
    CompletableFuture<Void> occupying = owner.submit(() -> {}, false);
    Throwable error = failure(binding.set(null, null));
    check(error instanceof NativeMediaLifecycle.Failure && "RTC_MEDIA_BUSY".equals(((NativeMediaLifecycle.Failure) error).code),
        "full owner queue was not rejected");
    work.drain(); complete(occupying);
    check(failure(owner.detachMatching(track)) == error, "queue rejection removed live track lease");
    check(failure(binding.dispose()) == error, "queue rejection was hidden during dispose");
    check(track.attached.get() == 1 && track.detached.get() == 0 && output.producer.get() == 0,
        "full queue silently released an attached track or its producer");
    check(owner.isQuarantined(), "full owner was not quarantined");
  }

  static void rejectedWorkerRetainsOutputAndFailedLease() throws Exception {
    Executor rejected = action -> { throw new RejectedExecutionException("synthetic bounded worker full"); };
    NativeMediaLifecycle.Owner owner = owner(rejected);
    Track track = new Track(); Output output = new Output();
    NativeRendererBinding binding = new NativeRendererBinding(Runnable::run, output, owner(Runnable::run));
    Throwable error = failure(binding.set(owner, track));
    check(error instanceof NativeMediaLifecycle.Failure, "worker rejection not surfaced");
    check(failure(owner.detachMatching(track)) == error, "failed attach lease not retained");
    check(failure(binding.dispose()) == error, "failed attach output released as success");
    check(track.attached.get() == 0 && output.egl.get() == 0 && output.producer.get() == 0,
        "rejected work invoked native track or released output");
  }

  static void supersededBindingCannotAttachAndOldProducerCannotReleaseNewOne() throws Exception {
    ExecutorService worker = worker(), otherWorker = worker(), outputWorker = worker();
    CountDownLatch entered = new CountDownLatch(1), release = new CountDownLatch(1);
    try {
      Main main = new Main(); Output oldOutput = new Output(), newOutput = new Output();
      Track first = new Track(), skipped = new Track(), latest = new Track(), independent = new Track();
      NativeMediaLifecycle.Owner firstOwner = owner(worker), otherOwner = owner(otherWorker);
      NativeRendererBinding oldBinding = new NativeRendererBinding(main, oldOutput, owner(outputWorker));
      NativeRendererBinding newBinding = new NativeRendererBinding(main, newOutput, owner(outputWorker));
      complete(oldBinding.set(firstOwner, first)); blockDetach(first, entered, release);
      CompletableFuture<Void> middle = oldBinding.set(otherOwner, skipped); await(entered);
      CompletableFuture<Void> last = oldBinding.set(otherOwner, latest);
      complete(newBinding.set(otherOwner, independent));
      check(skipped.attached.get() == 0 && latest.attached.get() == 0 && independent.attached.get() == 1,
          "queued generations attached early or unrelated renderer stalled");
      release.countDown(); complete(middle); complete(last);
      check(skipped.attached.get() == 0 && latest.attached.get() == 1, "old queued generation was revived");
      CompletableFuture<Void> disposed = oldBinding.dispose(); drainUntil(main, disposed);
      check(oldOutput.producer.get() == 1 && newOutput.producer.get() == 0 && independent.detached.get() == 0,
          "old renderer completion released the new renderer/producer");
      check(failure(oldBinding.set(otherOwner, new Track())) instanceof NativeMediaLifecycle.Failure,
          "closed renderer was revived");
      drainUntil(main, newBinding.dispose());
    } finally { release.countDown(); worker.shutdownNow(); otherWorker.shutdownNow(); outputWorker.shutdownNow(); }
  }

  static void synchronousCallbacksSharePublishedDisposal() throws Exception {
    AtomicReference<NativeRendererBinding> reference = new AtomicReference<>();
    List<CompletableFuture<Void>> reentered = new ArrayList<>();
    NativeMediaLifecycle.Owner owner = owner(Runnable::run);
    Track track = new Track(); Output output = new Output();
    NativeRendererBinding binding = new NativeRendererBinding(Runnable::run, output, owner(Runnable::run)); reference.set(binding);
    track.detaching = () -> reentered.add(reference.get().dispose());
    output.releasingEgl = () -> reentered.add(reference.get().dispose());
    output.releasingProducer = () -> reentered.add(reference.get().dispose());
    complete(binding.set(owner, track));
    CompletableFuture<Void> disposed = binding.dispose(); complete(disposed);
    check(reentered.size() == 3, "test did not reach actual detach/EGL/main callbacks");
    for (CompletableFuture<Void> reentrant : reentered) check(reentrant == disposed, "reentrant dispose got a different future");
    check(track.detached.get() == 1 && output.egl.get() == 1 && output.producer.get() == 1, "reentry duplicated cleanup");
  }

  static void concurrentDisposalsShareOnePhysicalRelease() throws Exception {
    ExecutorService worker = worker(), outputWorker = worker();
    CountDownLatch entered = new CountDownLatch(1), release = new CountDownLatch(1), start = new CountDownLatch(1);
    try {
      Main main = new Main(); Output output = new Output(); Track track = new Track();
      NativeRendererBinding binding = new NativeRendererBinding(main, output, owner(outputWorker));
      complete(binding.set(owner(worker), track)); blockDetach(track, entered, release);
      @SuppressWarnings("unchecked") CompletableFuture<Void>[] results = new CompletableFuture[12];
      List<Thread> callers = new ArrayList<>();
      for (int i = 0; i < results.length; i++) {
        final int index = i;
        Thread caller = new Thread(() -> {
          try { start.await(); results[index] = binding.dispose(); }
          catch (InterruptedException error) { throw new RuntimeException(error); }
        }); callers.add(caller); caller.start();
      }
      start.countDown(); for (Thread caller : callers) caller.join(1000); await(entered);
      for (CompletableFuture<Void> result : results) check(result != null && result == results[0], "concurrent caller got another disposal barrier");
      check(!results[0].isDone() && output.producer.get() == 0, "concurrent dispose completed before detach");
      release.countDown(); drainUntil(main, results[0]);
      check(track.detached.get() == 1 && output.egl.get() == 1 && output.producer.get() == 1, "concurrent dispose duplicated physical release");
    } finally { release.countDown(); worker.shutdownNow(); outputWorker.shutdownNow(); }
  }

  static void rejectedMainDispatchCompletesAsFailureAndKeepsProducer() throws Exception {
    Executor rejectedMain = action -> { throw new RejectedExecutionException("synthetic main dispatch rejected"); };
    Output output = new Output();
    NativeRendererBinding binding = new NativeRendererBinding(rejectedMain, output, owner(Runnable::run));
    CompletableFuture<Void> disposal = binding.dispose();
    check(disposal.isCompletedExceptionally(), "rejected main dispatch left physical disposal permanently pending");
    check(failure(disposal) instanceof RejectedExecutionException, "main dispatch failure was replaced");
    check(output.egl.get() == 1 && output.producer.get() == 0, "rejected main task released producer as success");
    check(binding.dispose() == disposal, "retry replaced failed disposal and lost original producer ownership");
  }

  static void fullRendererQueueDoesNotReleaseBlockedTrack() throws Exception {
    ExecutorService worker = worker(), outputWorker = worker();
    CountDownLatch entered = new CountDownLatch(1), release = new CountDownLatch(1);
    try {
      Main main = new Main(); Output output = new Output(); Track track = new Track();
      NativeMediaLifecycle.Owner owner = owner(worker);
      NativeRendererBinding binding = new NativeRendererBinding(main, output, owner(outputWorker));
      complete(binding.set(owner, track)); blockDetach(track, entered, release);
      List<CompletableFuture<Void>> accepted = new ArrayList<>();
      List<Track> queued = new ArrayList<>();
      for (int i = 0; i < 8; i++) {
        Track next = new Track(); queued.add(next); accepted.add(binding.set(owner, next));
        if (i == 0) await(entered);
      }
      Track rejected = new Track();
      Throwable error = failure(binding.set(owner, rejected));
      check(error instanceof NativeMediaLifecycle.Failure && "RTC_RENDERER_BUSY".equals(((NativeMediaLifecycle.Failure) error).code),
          "renderer accepted an unbounded ninth pending set");
      CompletableFuture<Void> disposed = binding.dispose();
      check(!disposed.isDone() && track.detached.get() == 1 && output.producer.get() == 0,
          "full renderer queue forgot the physically blocked old track");
      release.countDown();
      for (CompletableFuture<Void> future : accepted) complete(future);
      drainUntil(main, disposed);
      for (Track next : queued) check(next.attached.get() == 0, "quarantined renderer attached a queued generation");
      check(rejected.attached.get() == 0 && output.producer.get() == 1,
          "queue rejection invoked rejected track or cleanup failed after physical release");
    } finally { release.countDown(); worker.shutdownNow(); outputWorker.shutdownNow(); }
  }

  public static void main(String[] args) throws Exception {
    blockedDetachDoesNotBlockCallerOrClosePeerEarly(); System.out.println("PASS blocked detach: caller returns; peer/output wait physical barrier");
    callerDeadlineDoesNotAdvancePhysicalBarrier(); System.out.println("PASS caller deadline cannot advance/cancel physical detach; late result once");
    detachErrorRetainsTrackAndOutputOwnership(); System.out.println("PASS detach error retains lease/output and prevents peer close");
    ownerQueueFullRetainsAttachedTrack(); System.out.println("PASS bounded owner queue rejection retains attached track/output");
    rejectedWorkerRetainsOutputAndFailedLease(); System.out.println("PASS bounded executor rejection retains failed lease/output");
    supersededBindingCannotAttachAndOldProducerCannotReleaseNewOne(); System.out.println("PASS generations skip stale attach; old producer cannot release new renderer");
    synchronousCallbacksSharePublishedDisposal(); System.out.println("PASS synchronous detach/EGL/main reentry shares physical disposal");
    concurrentDisposalsShareOnePhysicalRelease(); System.out.println("PASS twelve concurrent dispose calls share one physical release");
    rejectedMainDispatchCompletesAsFailureAndKeepsProducer(); System.out.println("PASS rejected main dispatch fails physical future and retains producer");
    fullRendererQueueDoesNotReleaseBlockedTrack(); System.out.println("PASS renderer queue bounded to eight; overflow cannot forget a blocked lease");
    System.out.println("PASS 10 independent real Binding/Owner cases; fake JNI/Output only, no Android/JNI device claim");
  }
}
