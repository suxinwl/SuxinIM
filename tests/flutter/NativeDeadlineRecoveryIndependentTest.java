package com.cloudwebrtc.webrtc;

import java.util.concurrent.CompletableFuture;
import java.util.ArrayDeque;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.ExecutionException;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executor;
import java.util.concurrent.Executors;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicInteger;
import java.util.function.BooleanSupplier;

/** Actual production Owner/sequence/reporter; JNI is a controlled Runnable only. */
public final class NativeDeadlineRecoveryIndependentTest {
  private static void check(boolean value, String message) {
    if (!value) throw new AssertionError(message);
  }
  private static ExecutorService worker() {
    return Executors.newSingleThreadExecutor(action -> {
      Thread thread = new Thread(action, "independent-native-deadline");
      thread.setDaemon(true);
      return thread;
    });
  }
  private static NativeMediaLifecycle.Owner owner(ExecutorService worker) {
    return new NativeMediaLifecycle.Owner(worker, Runnable::run, 8);
  }
  private static void await(CountDownLatch latch) throws Exception {
    check(latch.await(3, TimeUnit.SECONDS), "controlled native boundary not reached");
  }
  private static void held(CountDownLatch entered, CountDownLatch release) {
    entered.countDown();
    try {
      if (!release.await(3, TimeUnit.SECONDS)) throw new AssertionError("native release not delivered");
    } catch (InterruptedException interrupted) {
      Thread.currentThread().interrupt();
      throw new IllegalStateException("test worker interrupted", interrupted);
    }
  }
  private static void complete(CompletableFuture<Void> physical) throws Exception {
    physical.get(3, TimeUnit.SECONDS);
  }
  private static Throwable failure(CompletableFuture<Void> physical) throws Exception {
    try {
      complete(physical);
      throw new AssertionError("expected failed physical operation");
    } catch (ExecutionException error) {
      return NativeMediaLifecycle.unwrap(error);
    }
  }
  private static void eventually(BooleanSupplier condition) throws Exception {
    long deadline = System.nanoTime() + TimeUnit.SECONDS.toNanos(3);
    while (!condition.getAsBoolean()) {
      check(System.nanoTime() < deadline, "physical completion state did not settle");
      Thread.sleep(1);
    }
  }
  private static void unavailable(NativeMediaLifecycle.Owner owner, AtomicInteger attempted) throws Exception {
    Throwable error = failure(owner.submit(attempted::incrementAndGet, false));
    check(error instanceof NativeMediaLifecycle.Failure
        && "RTC_MEDIA_UNAVAILABLE".equals(((NativeMediaLifecycle.Failure) error).code),
        "owner accepted new ordinary work or changed rejection kind");
  }
  private static final class Reply implements NativeMediaLifecycle.Response {
    final AtomicInteger calls = new AtomicInteger();
    final CountDownLatch answered = new CountDownLatch(1);
    volatile String code;
    public void success() { calls.incrementAndGet(); answered.countDown(); }
    public void error(String code, String message) {
      this.code = code; calls.incrementAndGet(); answered.countDown();
    }
  }
  private static Reply timeOut(NativeMediaLifecycle.Owner owner,
      CompletableFuture<Void> physical, String label) throws Exception {
    Reply reply = new Reply();
    NativeMediaLifecycle.report(physical, reply,
        () -> owner.callerTimedOut(physical, label), 20);
    await(reply.answered);
    check("RTC_MEDIA_TIMEOUT".equals(reply.code) && reply.calls.get() == 1,
        "caller did not receive one genuine deadline response");
    return reply;
  }

  private static void fullSequenceDeadlineWaitsForPhysicalThenRecovers() throws Exception {
    ExecutorService worker = worker();
    CountDownLatch entered = new CountDownLatch(1), release = new CountDownLatch(1);
    try {
      NativeMediaLifecycle.Owner owner = owner(worker);
      NativeCleanupSequence sequence = new NativeCleanupSequence(Runnable::run);
      AtomicInteger nativeCalls = new AtomicInteger(), rejected = new AtomicInteger();
      CompletableFuture<Void> physical = sequence.submit(() -> owner.submit(() -> {
        nativeCalls.incrementAndGet(); held(entered, release);
      }, true));
      await(entered);
      Reply reply = timeOut(owner, physical, "local.fullSequence");
      check(owner.isQuarantined() && sequence.isPending() && !physical.isDone()
          && !physical.isCancelled(), "deadline advanced or forgot physical owner");
      unavailable(owner, rejected);
      release.countDown(); complete(physical);
      eventually(() -> !owner.isQuarantined() && !sequence.isPending());
      complete(owner.submit(rejected::incrementAndGet, false));
      check(nativeCalls.get() == 1 && rejected.get() == 1 && reply.calls.get() == 1,
          "late completion repeated JNI, replied twice or retained a stale deadline");
    } finally { release.countDown(); worker.shutdownNow(); }
  }

  private static void eachPendingPhysicalOwnsItsOwnDeadline() throws Exception {
    ExecutorService worker = worker();
    CountDownLatch firstEntered = new CountDownLatch(1), firstRelease = new CountDownLatch(1);
    CountDownLatch secondEntered = new CountDownLatch(1), secondRelease = new CountDownLatch(1);
    try {
      NativeMediaLifecycle.Owner owner = owner(worker);
      AtomicInteger firstCalls = new AtomicInteger(), secondCalls = new AtomicInteger(), ordinary = new AtomicInteger();
      CompletableFuture<Void> first = owner.submit(() -> {
        firstCalls.incrementAndGet(); held(firstEntered, firstRelease);
      }, true);
      CompletableFuture<Void> second = owner.submit(() -> {
        secondCalls.incrementAndGet(); held(secondEntered, secondRelease);
      }, true);
      await(firstEntered);
      Reply a = timeOut(owner, first, "local.first");
      Reply b = timeOut(owner, second, "local.second");
      firstRelease.countDown(); complete(first); await(secondEntered);
      check(owner.isQuarantined() && !second.isDone(), "one success cleared another pending job");
      unavailable(owner, ordinary);
      secondRelease.countDown(); complete(second);
      eventually(() -> !owner.isQuarantined());
      complete(owner.submit(ordinary::incrementAndGet, false));
      check(firstCalls.get() == 1 && secondCalls.get() == 1 && ordinary.get() == 1
          && a.calls.get() == 1 && b.calls.get() == 1, "pending jobs or replies repeated");
    } finally { firstRelease.countDown(); secondRelease.countDown(); worker.shutdownNow(); }
  }

  private static void trueFailureCannotBeClearedByOtherSuccess() throws Exception {
    ExecutorService worker = worker();
    CountDownLatch entered = new CountDownLatch(1), release = new CountDownLatch(1);
    try {
      NativeMediaLifecycle.Owner owner = owner(worker);
      RuntimeException expected = new IllegalStateException("controlled native failure");
      AtomicInteger badCalls = new AtomicInteger(), goodCalls = new AtomicInteger(), ordinary = new AtomicInteger();
      CompletableFuture<Void> bad = owner.submit(() -> {
        badCalls.incrementAndGet(); held(entered, release); throw expected;
      }, true);
      CompletableFuture<Void> good = owner.submit(goodCalls::incrementAndGet, true);
      await(entered); Reply caller = timeOut(owner, bad, "local.bad");
      owner.callerTimedOut(good, "local.good");
      release.countDown();
      check(failure(bad) == expected, "physical error identity was replaced by deadline");
      complete(good);
      unavailable(owner, ordinary);
      owner.callerTimedOut(bad, "same.failed.physical");
      check(owner.isQuarantined() && badCalls.get() == 1 && goodCalls.get() == 1
          && ordinary.get() == 0 && caller.calls.get() == 1,
          "true failure recovered or a failed native operation was retried");
    } finally { release.countDown(); worker.shutdownNow(); }
  }

  private static void priorPermanentFailureSurvivesLaterDeadlineSuccess() throws Exception {
    ExecutorService worker = worker();
    try {
      NativeMediaLifecycle.Owner owner = owner(worker);
      AtomicInteger ordinary = new AtomicInteger();
      owner.quarantine();
      CompletableFuture<Void> later = new CompletableFuture<>();
      owner.callerTimedOut(later, "unrelated.later.cleanup");
      later.complete(null);
      check(owner.isQuarantined(), "late success erased unrelated permanent quarantine");
      unavailable(owner, ordinary);
      check(ordinary.get() == 0, "permanent failure allowed native reuse");
    } finally { worker.shutdownNow(); }
  }

  private static void closingOwnerNeverReopensOnLateSuccess() throws Exception {
    ExecutorService worker = worker();
    CountDownLatch entered = new CountDownLatch(1), release = new CountDownLatch(1);
    try {
      NativeMediaLifecycle.Owner owner = owner(worker);
      AtomicInteger calls = new AtomicInteger(), ordinary = new AtomicInteger();
      CompletableFuture<Void> physical = owner.submit(() -> {
        calls.incrementAndGet(); held(entered, release);
      }, true);
      await(entered); Reply reply = timeOut(owner, physical, "closing.cleanup");
      owner.closeForNewWork();
      release.countDown(); complete(physical);
      check(owner.isClosing(), "late success reversed terminal closing state");
      unavailable(owner, ordinary);
      complete(owner.submit(calls::incrementAndGet, true));
      check(calls.get() == 2 && ordinary.get() == 0 && reply.calls.get() == 1,
          "terminal owner repeated JNI or rejected required cleanup");
    } finally { release.countDown(); worker.shutdownNow(); }
  }

  private static void repeatedCallersCannotDuplicatePhysicalWorkOrLeaveMarkers() throws Exception {
    ExecutorService worker = worker();
    CountDownLatch entered = new CountDownLatch(1), release = new CountDownLatch(1);
    try {
      NativeMediaLifecycle.Owner owner = owner(worker);
      AtomicInteger calls = new AtomicInteger();
      CompletableFuture<Void> physical = owner.submit(() -> {
        calls.incrementAndGet(); held(entered, release);
      }, true);
      await(entered);
      Reply one = timeOut(owner, physical, "observer.one");
      Reply two = timeOut(owner, physical, "observer.two");
      release.countDown(); complete(physical);
      eventually(() -> !owner.isQuarantined());
      owner.callerTimedOut(physical, "late.observer.already.complete");
      check(!owner.isQuarantined() && calls.get() == 1 && one.calls.get() == 1 && two.calls.get() == 1,
          "repeated deadline observers duplicated work or retained completed marker");
    } finally { release.countDown(); worker.shutdownNow(); }
  }

  private static final class EqualFuture extends CompletableFuture<Void> {
    @Override public boolean equals(Object other) { return other instanceof EqualFuture; }
    @Override public int hashCode() { return 1; }
  }
  private static void deadlineTrackingUsesPhysicalIdentity() throws Exception {
    ExecutorService worker = worker();
    try {
      NativeMediaLifecycle.Owner owner = owner(worker);
      EqualFuture first = new EqualFuture(), second = new EqualFuture();
      owner.callerTimedOut(first, "equal.identity.first");
      owner.callerTimedOut(second, "equal.identity.second");
      first.complete(null);
      check(owner.isQuarantined(), "equal-but-distinct physical barriers were merged");
      second.complete(null);
      eventually(() -> !owner.isQuarantined());
    } finally { worker.shutdownNow(); }
  }

  private static void failedSequenceTailCannotBeRecoveredOrReentered() throws Exception {
    ExecutorService worker = worker();
    try {
      NativeMediaLifecycle.Owner owner = owner(worker);
      NativeCleanupSequence sequence = new NativeCleanupSequence(Runnable::run);
      CompletableFuture<Void> nativeBoundary = new CompletableFuture<>();
      AtomicInteger firstCalls = new AtomicInteger(), laterCalls = new AtomicInteger();
      CompletableFuture<Void> first = sequence.submit(() -> {
        firstCalls.incrementAndGet(); return nativeBoundary;
      });
      CompletableFuture<Void> second = sequence.submit(() -> {
        laterCalls.incrementAndGet(); return CompletableFuture.completedFuture(null);
      });
      Reply a = timeOut(owner, first, "sequence.first");
      Reply b = timeOut(owner, second, "sequence.second");
      RuntimeException expected = new IllegalArgumentException("controlled detach failure");
      nativeBoundary.completeExceptionally(expected);
      check(failure(first) == expected && failure(second) == expected, "sequence lost original physical failure");
      CompletableFuture<Void> retry = sequence.submit(() -> {
        laterCalls.incrementAndGet(); return CompletableFuture.completedFuture(null);
      });
      check(failure(retry) == expected && sequence.isPending() && owner.isQuarantined()
          && firstCalls.get() == 1 && laterCalls.get() == 0
          && a.calls.get() == 1 && b.calls.get() == 1,
          "failed tail was reset, retried or allowed queued native cleanup");
    } finally { worker.shutdownNow(); }
  }

  private static final class ManualWorker implements Executor {
    final ArrayDeque<Runnable> jobs = new ArrayDeque<>();
    public void execute(Runnable job) { jobs.add(job); }
    void runAll() {
      while (!jobs.isEmpty()) jobs.remove().run();
    }
  }
  private static final class BoundTrack implements NativeRendererBinding.Track {
    int attaches, detaches;
    public void attach() { attaches++; }
    public void detach() { detaches++; }
  }
  private static final class BoundOutput implements NativeRendererBinding.Output {
    int egl, producer;
    public void releaseEgl() { egl++; }
    public void releaseProducerOnMain() { producer++; }
  }
  private static void acceptedCurrentAttachCannotSilentlySkipAfterSoftDeadline() throws Exception {
    ManualWorker worker = new ManualWorker();
    NativeMediaLifecycle.Owner owner = new NativeMediaLifecycle.Owner(worker, Runnable::run, 8);
    NativeMediaLifecycle.Owner outputOwner = new NativeMediaLifecycle.Owner(Runnable::run, Runnable::run, 8);
    BoundTrack track = new BoundTrack(); BoundOutput output = new BoundOutput();
    NativeRendererBinding binding = new NativeRendererBinding(Runnable::run, output, outputOwner);
    // TrackLease.attach has been admitted by the real owner, but its actual
    // native action has not run. A later soft deadline gates only new work.
    CompletableFuture<Void> attaching = binding.set(owner, track);
    check(!attaching.isDone() && track.attaches == 0 && !worker.jobs.isEmpty(),
        "fixture did not hold the already-admitted production attach");
    CompletableFuture<Void> unrelatedPhysical = new CompletableFuture<>();
    owner.callerTimedOut(unrelatedPhysical, "older.physical.cleanup");
    AtomicInteger rejected = new AtomicInteger();
    unavailable(owner, rejected);
    worker.runAll(); complete(attaching);
    check(track.attaches == 1 && owner.isQuarantined(),
        "current accepted attach was reported successful without attaching, or cleared another deadline");
    unrelatedPhysical.complete(null);
    eventually(() -> !owner.isQuarantined());
    CompletableFuture<Void> disposing = binding.dispose(); worker.runAll(); complete(disposing);
    check(track.attaches == 1 && track.detaches == 1 && output.egl == 1 && output.producer == 1,
        "late-safe accepted attach repeated or lost physical renderer release");
  }

  private static void currentHardOwnerRejectsInsteadOfFakingAttachSuccess() throws Exception {
    ManualWorker worker = new ManualWorker();
    NativeMediaLifecycle.Owner owner = new NativeMediaLifecycle.Owner(worker, Runnable::run, 8);
    NativeMediaLifecycle.Owner outputOwner = new NativeMediaLifecycle.Owner(Runnable::run, Runnable::run, 8);
    BoundTrack track = new BoundTrack(); BoundOutput output = new BoundOutput();
    NativeRendererBinding binding = new NativeRendererBinding(Runnable::run, output, outputOwner);
    CompletableFuture<Void> attaching = binding.set(owner, track);
    check(!attaching.isDone(), "fixture did not hold the admitted current attach");
    owner.quarantine();
    worker.runAll();
    Throwable rejected = failure(attaching);
    check(rejected instanceof NativeMediaLifecycle.Failure
        && "RTC_MEDIA_UNAVAILABLE".equals(((NativeMediaLifecycle.Failure) rejected).code),
        "hard-invalid current source did not expose an explicit admission error");
    owner.callerTimedOut(CompletableFuture.completedFuture(null), "later.success");
    check(track.attaches == 0 && track.detaches == 0 && owner.isPermanentlyQuarantined()
        && output.egl == 0 && output.producer == 0,
        "hard owner attached JNI, reset failure, or claimed output release");
    AtomicInteger ordinary = new AtomicInteger();
    unavailable(owner, ordinary);
    check(ordinary.get() == 0, "hard-invalid owner reopened");
  }

  private static void normalClosingRetiresUnattachedLeaseWithoutPoisoningPeerCleanup() throws Exception {
    ManualWorker worker = new ManualWorker();
    NativeMediaLifecycle.Owner owner = new NativeMediaLifecycle.Owner(worker, Runnable::run, 8);
    NativeMediaLifecycle.Owner outputOwner = new NativeMediaLifecycle.Owner(Runnable::run, Runnable::run, 8);
    BoundTrack track = new BoundTrack(); BoundOutput output = new BoundOutput();
    NativeRendererBinding binding = new NativeRendererBinding(Runnable::run, output, outputOwner);
    CompletableFuture<Void> attaching = binding.set(owner, track);
    owner.closeForNewWork();
    worker.runAll(); complete(attaching);
    check(track.attaches == 0 && !owner.isPermanentlyQuarantined(),
        "normal lifecycle retirement invoked attach or created a hard failure");
    CompletableFuture<Void> detached = owner.detachAll();
    worker.runAll(); complete(detached);
    AtomicInteger closes = new AtomicInteger(), disposes = new AtomicInteger();
    NativePeerLifecycle peer = new NativePeerLifecycle(owner, closes::incrementAndGet, disposes::incrementAndGet);
    CompletableFuture<Void> close = peer.close(), dispose = peer.dispose();
    worker.runAll(); complete(close); complete(dispose);
    check(peer.close() == close && peer.dispose() == dispose
        && closes.get() == 1 && disposes.get() == 1 && track.detaches == 0,
        "retired unattached lease poisoned detachAll or repeated peer JNI");
    CompletableFuture<Void> outputRelease = binding.dispose();
    worker.runAll(); complete(outputRelease);
    check(output.egl == 1 && output.producer == 1 && owner.isClosing(),
        "normal retirement failed to finish output cleanup or reopened terminal owner");
  }

  public static void main(String[] args) throws Exception {
    fullSequenceDeadlineWaitsForPhysicalThenRecovers();
    System.out.println("PASS full physical sequence remains blocking after caller deadline, then true success recovers");
    eachPendingPhysicalOwnsItsOwnDeadline();
    System.out.println("PASS one physical success cannot clear a second pending deadline");
    trueFailureCannotBeClearedByOtherSuccess();
    System.out.println("PASS true native failure retains original cause and cannot be cleared by unrelated success");
    priorPermanentFailureSurvivesLaterDeadlineSuccess();
    System.out.println("PASS preexisting permanent quarantine survives later successful deadline barrier");
    closingOwnerNeverReopensOnLateSuccess();
    System.out.println("PASS closing remains terminal while required cleanup is still admitted");
    repeatedCallersCannotDuplicatePhysicalWorkOrLeaveMarkers();
    System.out.println("PASS repeated caller deadlines share physical work and each response settles once");
    deadlineTrackingUsesPhysicalIdentity();
    System.out.println("PASS equal-but-distinct physical Futures retain separate deadline ownership");
    failedSequenceTailCannotBeRecoveredOrReentered();
    System.out.println("PASS failed sequence tail preserves first error and never runs queued or retried JNI");
    acceptedCurrentAttachCannotSilentlySkipAfterSoftDeadline();
    System.out.println("PASS already-admitted current Binding attach actually executes despite later soft deadline");
    currentHardOwnerRejectsInsteadOfFakingAttachSuccess();
    System.out.println("PASS current hard-invalid owner rejects explicitly without attach or false recovery");
    normalClosingRetiresUnattachedLeaseWithoutPoisoningPeerCleanup();
    System.out.println("PASS normal owner closing retires never-attached lease and still permits exactly-once peer/output cleanup");
    System.out.println("PASS 11 independent production Owner/sequence/Binding deadline recovery cases; fake JNI boundary, no Android/runtime claim");
  }
}
