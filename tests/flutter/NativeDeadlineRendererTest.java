package com.cloudwebrtc.webrtc;

import android.os.Handler;
import java.util.ArrayDeque;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.Executor;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicInteger;
import org.webrtc.VideoTrack;

/** Production binding/renderer with controlled native/platform boundaries. */
public final class NativeDeadlineRendererTest {
  static void check(boolean ok, String text) { if (!ok) throw new AssertionError(text); }
  static final class Manual implements Executor {
    final ArrayDeque<Runnable> queue = new ArrayDeque<>();
    public void execute(Runnable operation) { queue.add(operation); }
    void drain() { while (!queue.isEmpty()) queue.remove().run(); }
  }
  static final class Track implements NativeRendererBinding.Track {
    int adds, removes;
    Runnable onRemove = () -> {};
    public void attach() { adds++; }
    public void detach() { removes++; onRemove.run(); }
  }
  static NativeRendererBinding binding() {
    return new NativeRendererBinding(Runnable::run, new NativeRendererBinding.Output() {
      public void releaseEgl() {}
      public void releaseProducerOnMain() {}
    });
  }
  static void rejects(CompletableFuture<Void> future, String code) {
    try { future.join(); throw new AssertionError("expected explicit rejection"); }
    catch (java.util.concurrent.CompletionException error) {
      Throwable cause = NativeMediaLifecycle.unwrap(error);
      check(cause instanceof NativeMediaLifecycle.Failure && code.equals(((NativeMediaLifecycle.Failure) cause).code), "wrong error identity/code");
    }
  }
  static void deadlineDoesNotRetireCurrentGeneration() {
    Manual worker = new Manual(); NativeMediaLifecycle.Owner owner = new NativeMediaLifecycle.Owner(worker, Runnable::run, 8);
    NativeRendererBinding binding = binding(); Track old = new Track(), next = new Track(), rejected = new Track(), after = new Track();
    CompletableFuture<Void> first = binding.set(owner, old); worker.drain(); first.join();
    CompletableFuture<Void> change = binding.set(owner, next);
    binding.callerTimedOut(change, "renderer.set");
    NativeRendererBinding.SetRequest denied = binding.requestSet(owner, rejected);
    check(!denied.accepted, "new attach admitted while physical deadline barrier pending"); rejects(denied.physical, "RTC_RENDERER_BUSY");
    check(!change.isDone() && next.adds == 0 && rejected.adds == 0, "deadline advanced physical work");
    worker.drain(); change.join();
    check(old.removes == 1 && next.adds == 1 && rejected.adds == 0, "timeout retired accepted current generation or attached rejected track");
    CompletableFuture<Void> recovered = binding.set(owner, after); worker.drain(); recovered.join();
    check(next.removes == 1 && after.adds == 1, "genuine late success did not permit a later new admission");
    CompletableFuture<Void> disposed = binding.dispose(); worker.drain(); disposed.join();
  }
  static void cleanupMayRetireTimedOutSetWithoutRevivingIt() {
    Manual worker = new Manual(); NativeMediaLifecycle.Owner owner = new NativeMediaLifecycle.Owner(worker, Runnable::run, 8);
    NativeRendererBinding binding = binding(); Track old = new Track(), retired = new Track(), after = new Track();
    CompletableFuture<Void> first = binding.set(owner, old); worker.drain(); first.join();
    CompletableFuture<Void> change = binding.set(owner, retired); binding.callerTimedOut(change, "renderer.set");
    NativeRendererBinding.SetRequest cleanup = binding.requestSet(null, null);
    check(cleanup.accepted, "soft deadline blocked null-stream physical cleanup");
    worker.drain(); change.join(); cleanup.physical.join();
    check(retired.adds == 0 && old.removes == 1, "an explicitly retired generation was revived by late success");
    CompletableFuture<Void> recovered = binding.set(owner, after); worker.drain(); recovered.join();
    check(after.adds == 1, "retired physical cleanup did not complete normally");
    CompletableFuture<Void> disposed = binding.dispose(); worker.drain(); disposed.join();
  }
  static void physicalFailureNeverClearsRendererLatchOrRetriesJNI() {
    Manual worker = new Manual(); NativeMediaLifecycle.Owner owner = new NativeMediaLifecycle.Owner(worker, Runnable::run, 8);
    NativeRendererBinding binding = binding(); Track old = new Track(), next = new Track();
    CompletableFuture<Void> first = binding.set(owner, old); worker.drain(); first.join();
    RuntimeException marker = new IllegalStateException("synthetic native detach failure"); old.onRemove = () -> { throw marker; };
    CompletableFuture<Void> change = binding.set(owner, next); binding.callerTimedOut(change, "renderer.set"); worker.drain();
    try { change.join(); throw new AssertionError("failure was swallowed"); }
    catch (java.util.concurrent.CompletionException error) { check(NativeMediaLifecycle.unwrap(error) == marker, "native failure identity changed"); }
    rejects(binding.set(owner, new Track()), "RTC_RENDERER_UNAVAILABLE");
    check(binding.dispose().isCompletedExceptionally() && old.removes == 1 && next.adds == 0 && owner.isPermanentlyQuarantined(), "failed detach was retried or true failure latch cleared");
  }
  static void closeCannotBeUndoneByLateSuccess() {
    Manual worker = new Manual(); NativeMediaLifecycle.Owner owner = new NativeMediaLifecycle.Owner(worker, Runnable::run, 8);
    NativeRendererBinding binding = binding(); Track old = new Track(), next = new Track();
    CompletableFuture<Void> first = binding.set(owner, old); worker.drain(); first.join();
    CompletableFuture<Void> change = binding.set(owner, next); binding.callerTimedOut(change, "renderer.set");
    CompletableFuture<Void> disposed = binding.dispose(); worker.drain(); disposed.join(); change.join();
    rejects(binding.set(owner, new Track()), "RTC_RENDERER_UNAVAILABLE");
    check(next.adds == 0 && old.removes == 1, "late success reopened closing renderer");
  }
  static void actualRendererDeadlineRecoversWithoutMetadataPollution() throws Exception {
    NativeMediaLifecycle.Owner owner = new NativeMediaLifecycle.Owner();
    NativeRendererWiringTest.Producer producer = new NativeRendererWiringTest.Producer();
    FlutterRTCVideoRenderer renderer = new FlutterRTCVideoRenderer(producer, (track, tag) -> owner);
    VideoTrack old = new VideoTrack("old"), next = new VideoTrack("accepted-next"), rejected = new VideoTrack("rejected"), after = new VideoTrack("after");
    NativeRendererWiringTest.Reply first = new NativeRendererWiringTest.Reply(); renderer.setTrack(old, "old-stream", "owner", first); first.awaitResult();
    CountDownLatch entered = new CountDownLatch(1), release = new CountDownLatch(1);
    old.onRemove = () -> {
      entered.countDown();
      try { check(release.await(20, TimeUnit.SECONDS), "controlled native detach was not released"); }
      catch (InterruptedException error) { throw new RuntimeException(error); }
    };
    try {
      NativeRendererWiringTest.Reply timedOut = new NativeRendererWiringTest.Reply();
      renderer.setTrack(next, "next-stream", "owner", timedOut);
      check(entered.await(3, TimeUnit.SECONDS), "actual removeSink not reached"); timedOut.awaitResult(15000);
      check("RTC_MEDIA_TIMEOUT".equals(timedOut.error) && old.sinks.size() == 1 && next.adds.get() == 0, "actual 12s deadline changed native physical ownership");
      NativeRendererWiringTest.Reply denied = new NativeRendererWiringTest.Reply();
      renderer.setTrack(rejected, "rejected-stream", "other-owner", denied); denied.awaitResult();
      check("RTC_RENDERER_BUSY".equals(denied.error) && rejected.adds.get() == 0, "actual new attach accepted despite soft physical barrier");
      check(renderer.checkVideoTrack("accepted-next", "owner") && renderer.checkMediaStream("next-stream", "owner"), "rejected request overwrote accepted binding metadata");
      release.countDown(); long until = System.nanoTime() + TimeUnit.SECONDS.toNanos(3);
      while (next.adds.get() == 0 && System.nanoTime() < until) { Handler.drainMainLoop(); Thread.sleep(1); }
      check(next.adds.get() == 1 && old.sinks.isEmpty() && timedOut.count() == 1 && timedOut.successes.get() == 0, "late real attach did not finish once or answered expired caller twice");
      NativeRendererWiringTest.Reply recovered = new NativeRendererWiringTest.Reply(); renderer.setTrack(after, "after-stream", "owner", recovered); recovered.awaitResult();
      check(recovered.successes.get() == 1 && after.adds.get() == 1 && next.sinks.isEmpty(), "actual renderer did not recover after genuine late success");
      NativeRendererWiringTest.dispose(renderer);
      check(producer.released.get() == 1 && after.sinks.isEmpty(), "actual recovered renderer failed final physical disposal");
    } finally { release.countDown(); }
  }
  public static void main(String[] args) throws Exception {
    deadlineDoesNotRetireCurrentGeneration(); System.out.println("PASS deadline preserves accepted current generation; rejects only new attach until late actual success");
    cleanupMayRetireTimedOutSetWithoutRevivingIt(); System.out.println("PASS null cleanup may retire timed-out generation; late completion cannot revive it");
    physicalFailureNeverClearsRendererLatchOrRetriesJNI(); System.out.println("PASS real native failure retains latch/lease and never retries JNI");
    closeCannotBeUndoneByLateSuccess(); System.out.println("PASS closing renderer cannot reopen on late success");
    actualRendererDeadlineRecoversWithoutMetadataPollution(); System.out.println("PASS actual renderer12s deadline -> late physical attach -> new admission; rejected metadata unchanged; caller answered once");
    System.out.println("PASS 5 production binding/actual-renderer deadline cases; fake native/platform boundaries, not Android JNI acceptance");
  }
}
