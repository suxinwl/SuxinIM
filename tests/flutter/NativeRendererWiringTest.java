package com.cloudwebrtc.webrtc;

import android.os.Handler;
import io.flutter.plugin.common.MethodChannel.Result;
import io.flutter.view.TextureRegistry.SurfaceProducer;
import org.webrtc.MediaStream;
import org.webrtc.VideoTrack;
import java.util.ArrayList;
import java.util.Collections;
import java.util.List;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicInteger;

/** Compiles the actual FlutterRTCVideoRenderer, Binding and Owner against fake
 * Android/Flutter/JNI/EGL boundaries. This is explicitly not an Android test. */
public final class NativeRendererWiringTest {
  private static final Thread MAIN = Thread.currentThread();
  static void check(boolean condition, String message) { if (!condition) throw new AssertionError(message); }
  static void await(CountDownLatch latch) throws Exception { check(latch.await(3, TimeUnit.SECONDS), "test latch timeout"); }
  static final class Reply implements Result {
    final AtomicInteger successes = new AtomicInteger(), errors = new AtomicInteger();
    volatile String error;
    public void success(Object value) { check(Thread.currentThread() == MAIN, "success was not marshalled to main"); successes.incrementAndGet(); }
    public void error(String code, String message, Object details) { check(Thread.currentThread() == MAIN, "error was not marshalled to main"); error = code; errors.incrementAndGet(); }
    public void notImplemented() { throw new AssertionError("unexpected notImplemented"); }
    int count() { return successes.get() + errors.get(); }
    void awaitResult() throws Exception { awaitResult(3000); }
    void awaitResult(long millis) throws Exception {
      long until = System.nanoTime() + TimeUnit.MILLISECONDS.toNanos(millis);
      while (count() == 0 && System.nanoTime() < until) { Handler.drainMainLoop(); Thread.sleep(1); }
      Handler.drainMainLoop(); check(count() == 1, "missing/duplicate MethodChannel response");
    }
  }
  static final class Producer implements SurfaceProducer {
    final AtomicInteger released = new AtomicInteger();
    Runnable onRelease = () -> {};
    public void release() { check(Thread.currentThread() == MAIN, "producer release did not run on main"); released.incrementAndGet(); onRelease.run(); }
  }
  static SurfaceTextureRenderer outputOfNewRenderer() {
    return SurfaceTextureRenderer.instances.get(SurfaceTextureRenderer.instances.size() - 1);
  }
  static void blockRemove(VideoTrack track, CountDownLatch entered, CountDownLatch release) {
    track.onRemove = () -> {
      check(Thread.currentThread() != MAIN, "real renderer called removeSink on main"); entered.countDown();
      try { check(release.await(3, TimeUnit.SECONDS), "blocked fake JNI never released"); }
      catch (InterruptedException error) { throw new RuntimeException(error); }
    };
  }
  static void attach(FlutterRTCVideoRenderer renderer, VideoTrack track) throws Exception {
    Reply reply = new Reply(); renderer.setTrack(track, "stream", "owner", reply); reply.awaitResult();
    check(reply.successes.get() == 1 && track.adds.get() == 1 && track.addThread != MAIN,
        "actual setTrack did not attach off main exactly once");
  }
  static void dispose(FlutterRTCVideoRenderer renderer) throws Exception {
    Reply reply = new Reply(); renderer.Dispose(reply, () -> {}); reply.awaitResult();
    check(reply.successes.get() == 1, "renderer disposal failed");
  }

  static void actualNullStreamAndDisposeWaitForPhysicalDetach() throws Exception {
    NativeMediaLifecycle.Owner owner = new NativeMediaLifecycle.Owner();
    Producer producer = new Producer();
    FlutterRTCVideoRenderer renderer = new FlutterRTCVideoRenderer(producer, (track, tag) -> owner);
    SurfaceTextureRenderer surface = outputOfNewRenderer();
    VideoTrack track = new VideoTrack("first"); attach(renderer, track);
    CountDownLatch entered = new CountDownLatch(1), release = new CountDownLatch(1);
    List<String> sequence = Collections.synchronizedList(new ArrayList<>());
    blockRemove(track, entered, release);
    Runnable block = track.onRemove;
    track.onRemove = () -> { block.run(); sequence.add("detached"); };
    surface.onRelease = () -> sequence.add("egl");
    producer.onRelease = () -> sequence.add("producer");
    try {
      Reply cleared = new Reply();
      long start = System.nanoTime(); renderer.setStream((MediaStream) null, "owner", cleared);
      check(TimeUnit.NANOSECONDS.toMillis(System.nanoTime() - start) < 200, "actual setStream(null) blocked caller");
      await(entered);
      Reply disposed = new Reply(); AtomicInteger onDisposed = new AtomicInteger();
      renderer.Dispose(disposed, onDisposed::incrementAndGet);
      AtomicInteger peerClosed = new AtomicInteger();
      CompletableFuture<Void> peer = owner.detachAll().thenCompose(v -> owner.submit(peerClosed::incrementAndGet, true));
      Handler.drainMainLoop();
      check(cleared.count() == 0 && disposed.count() == 0 && !peer.isDone(), "actual MethodChannel/peer acknowledged before removeSink");
      check(surface.releases.get() == 0 && producer.released.get() == 0 && track.sinks.size() == 1,
          "actual renderer released output or lost sink ownership before detach");
      release.countDown(); cleared.awaitResult(); disposed.awaitResult(); peer.get(3, TimeUnit.SECONDS);
      check(cleared.successes.get() == 1 && disposed.successes.get() == 1 && onDisposed.get() == 1,
          "actual completion callback missing or duplicated");
      check(surface.finalRelease && surface.releaseThread != MAIN && producer.released.get() == 1 && track.sinks.isEmpty(),
          "actual native/EGL/main release path skipped ownership stage");
      check(sequence.equals(java.util.Arrays.asList("detached", "egl", "producer")), "actual output release order wrong: " + sequence);
    } finally { release.countDown(); }
  }

  static void oldRendererCompletionCannotReleaseIndependentProducer() throws Exception {
    NativeMediaLifecycle.Owner oldOwner = new NativeMediaLifecycle.Owner(), newOwner = new NativeMediaLifecycle.Owner();
    Producer oldProducer = new Producer(), newProducer = new Producer();
    FlutterRTCVideoRenderer old = new FlutterRTCVideoRenderer(oldProducer, (track, tag) -> oldOwner);
    FlutterRTCVideoRenderer newer = new FlutterRTCVideoRenderer(newProducer, (track, tag) -> newOwner);
    VideoTrack first = new VideoTrack("old"), skipped = new VideoTrack("skipped"), next = new VideoTrack("next"), independent = new VideoTrack("independent");
    attach(old, first);
    CountDownLatch entered = new CountDownLatch(1), release = new CountDownLatch(1); blockRemove(first, entered, release);
    try {
      Reply skipResult = new Reply(); old.setTrack(skipped, "stream", "owner", skipResult); await(entered);
      Reply nextResult = new Reply(); old.setTrack(next, "stream", "owner", nextResult);
      Reply oldDisposed = new Reply(); old.Dispose(oldDisposed, () -> {});
      attach(newer, independent);
      release.countDown(); skipResult.awaitResult(); nextResult.awaitResult(); oldDisposed.awaitResult();
      check(skipped.adds.get() == 0 && next.adds.get() == 0, "old renderer attached after closing/new generation");
      check(oldProducer.released.get() == 1 && newProducer.released.get() == 0 && independent.sinks.size() == 1,
          "actual old renderer completion crossed producer ownership");
      dispose(newer);
    } finally { release.countDown(); }
  }

  static void actualNativeDetachFailureDoesNotReportSuccessOrDeleteProducer() throws Exception {
    NativeMediaLifecycle.Owner owner = new NativeMediaLifecycle.Owner(); Producer producer = new Producer();
    FlutterRTCVideoRenderer renderer = new FlutterRTCVideoRenderer(producer, (track, tag) -> owner);
    SurfaceTextureRenderer surface = outputOfNewRenderer(); VideoTrack track = new VideoTrack("throwing"); attach(renderer, track);
    track.onRemove = () -> { throw new IllegalStateException("synthetic JNI remove failure"); };
    Reply cleared = new Reply(); renderer.setStream((MediaStream) null, "owner", cleared); cleared.awaitResult();
    Reply disposed = new Reply(); AtomicInteger callback = new AtomicInteger(); renderer.Dispose(disposed, callback::incrementAndGet); disposed.awaitResult();
    check(cleared.successes.get() == 0 && disposed.successes.get() == 0
        && "RTC_MEDIA_FAILED".equals(cleared.error) && "RTC_MEDIA_FAILED".equals(disposed.error), "actual native error became success");
    check(callback.get() == 0 && surface.releases.get() == 0 && producer.released.get() == 0 && track.sinks.size() == 1,
        "actual failed detach lost output/native references");
    check(owner.detachMatching(track).isCompletedExceptionally(), "actual renderer did not register exact native track identity as lease key");
  }

  static void actualStreamTrackSelectionAndUnavailableOwner() throws Exception {
    NativeMediaLifecycle.Owner owner = new NativeMediaLifecycle.Owner(); Producer producer = new Producer();
    FlutterRTCVideoRenderer renderer = new FlutterRTCVideoRenderer(producer, (track, tag) -> "owner".equals(tag) ? owner : null);
    MediaStream stream = new MediaStream("selected-stream"); VideoTrack first = new VideoTrack("first"), selected = new VideoTrack("selected");
    stream.videoTracks.add(first); stream.videoTracks.add(selected);
    Reply result = new Reply(); renderer.setStream(stream, "selected", "owner", result); result.awaitResult();
    check(result.successes.get() == 1 && selected.adds.get() == 1 && first.adds.get() == 0, "actual overload did not select requested track");
    check(renderer.checkVideoTrack("selected", "owner") && renderer.checkMediaStream("selected-stream", "owner"), "actual ownership bookkeeping mismatch");
    Reply unavailable = new Reply(); renderer.setTrack(new VideoTrack("unavailable"), "unknown", "other-owner", unavailable);
    check(unavailable.count() == 1 && "RTC_MEDIA_UNAVAILABLE".equals(unavailable.error), "owner rejection did not fail closed");
    dispose(renderer);
  }

  static void actualEglFailurePreventsProducerSuccess() throws Exception {
    Producer producer = new Producer();
    FlutterRTCVideoRenderer renderer = new FlutterRTCVideoRenderer(producer, (track, tag) -> new NativeMediaLifecycle.Owner());
    SurfaceTextureRenderer surface = outputOfNewRenderer();
    surface.cleanupFailure = new IllegalStateException("synthetic observed EGL release failure");
    Reply reply = new Reply(); AtomicInteger callback = new AtomicInteger(); renderer.Dispose(reply, callback::incrementAndGet); reply.awaitResult();
    check(reply.successes.get() == 0 && "RTC_MEDIA_FAILED".equals(reply.error), "actual EGL failure was hidden");
    check(surface.releases.get() == 0 && producer.released.get() == 0 && callback.get() == 0, "actual renderer retried known failed EGL or released producer");
    Producer failingProducer = new Producer();
    FlutterRTCVideoRenderer fresh = new FlutterRTCVideoRenderer(failingProducer, (track, tag) -> new NativeMediaLifecycle.Owner());
    SurfaceTextureRenderer freshSurface = outputOfNewRenderer();
    freshSurface.onRelease = () -> { throw new IllegalStateException("synthetic first EGL release failure"); };
    Reply freshReply = new Reply(); fresh.Dispose(freshReply, () -> {}); freshReply.awaitResult();
    check(freshReply.successes.get() == 0 && "RTC_MEDIA_FAILED".equals(freshReply.error)
        && freshSurface.releases.get() == 1 && failingProducer.released.get() == 0,
        "actual new EGL failure was hidden or producer released despite failure");
  }

  static void realDeadlineStillRemovesTombstoneAfterLatePhysicalRelease() throws Exception {
    NativeMediaLifecycle.Owner owner = new NativeMediaLifecycle.Owner(); Producer producer = new Producer();
    FlutterRTCVideoRenderer renderer = new FlutterRTCVideoRenderer(producer, (track, tag) -> owner);
    SurfaceTextureRenderer surface = outputOfNewRenderer(); VideoTrack track = new VideoTrack("deadline"); attach(renderer, track);
    CountDownLatch entered = new CountDownLatch(1), release = new CountDownLatch(1);
    track.onRemove = () -> {
      entered.countDown();
      try { check(release.await(20, TimeUnit.SECONDS), "test did not release delayed native detach"); }
      catch (InterruptedException error) { throw new RuntimeException(error); }
    };
    try {
      Reply result = new Reply(); AtomicInteger removedFromRegistry = new AtomicInteger();
      renderer.Dispose(result, removedFromRegistry::incrementAndGet); await(entered);
      result.awaitResult(15000); // Exercise the actual production 12s deadline.
      check("RTC_MEDIA_TIMEOUT".equals(result.error) && result.successes.get() == 0,
          "physical stall did not return actual production deadline error");
      check(removedFromRegistry.get() == 0 && producer.released.get() == 0 && surface.releases.get() == 0,
          "caller timeout dropped renderer tombstone/native ownership");
      release.countDown();
      long until = System.nanoTime() + TimeUnit.SECONDS.toNanos(3);
      while (removedFromRegistry.get() == 0 && System.nanoTime() < until) { Handler.drainMainLoop(); Thread.sleep(1); }
      Handler.drainMainLoop();
      check(removedFromRegistry.get() == 1 && producer.released.get() == 1 && track.sinks.isEmpty(),
          "late physical success did not remove retained renderer registry tombstone");
      check(result.count() == 1 && result.errors.get() == 1, "late physical success answered MethodChannel twice");
    } finally { release.countDown(); }
  }

  static void detachOneLocalStreamPreservesOtherRendererOnSameTrack() throws Exception {
    android.util.Log.messages.clear();
    NativeMediaLifecycle.Owner owner = new NativeMediaLifecycle.Owner();
    Producer aProducer = new Producer(), bProducer = new Producer();
    FlutterRTCVideoRenderer a = new FlutterRTCVideoRenderer(aProducer, (track, tag) -> owner);
    SurfaceTextureRenderer aSurface = outputOfNewRenderer();
    FlutterRTCVideoRenderer b = new FlutterRTCVideoRenderer(bProducer, (track, tag) -> owner);
    SurfaceTextureRenderer bSurface = outputOfNewRenderer();
    VideoTrack shared = new VideoTrack("same-native-wrapper");
    Reply aSet = new Reply(), bSet = new Reply();
    a.setTrack(shared, "stream-A", "local", aSet); aSet.awaitResult();
    b.setTrack(shared, "stream-B", "local", bSet); bSet.awaitResult();
    check(shared.sinks.size() == 2, "two real renderer bindings did not attach to shared native track");
    a.detachStream("unrelated-stream").get(3, TimeUnit.SECONDS);
    check(shared.sinks.size() == 2, "unrelated stream cleanup detached live renderer");
    int beforeDetachIdCalls = shared.idCalls.get();
    a.detachStream("stream-A").get(3, TimeUnit.SECONDS);
    check(shared.idCalls.get() == beforeDetachIdCalls, "diagnostics introduced a track.id JNI query");
    String trackHash = "trackJava#" + Integer.toHexString(System.identityHashCode(shared));
    String ownerHash = "ownerJava#" + Integer.toHexString(System.identityHashCode(owner));
    String aHash = "rendererJava#" + Integer.toHexString(System.identityHashCode(a));
    String aSinkHash = "sinkJava#" + Integer.toHexString(System.identityHashCode(aSurface));
    check(android.util.Log.messages.stream().anyMatch(line -> line.contains("phase=javaRemoveSinkBegin")
        && line.contains(trackHash) && line.contains(ownerHash) && line.contains(aHash) && line.contains(aSinkHash)),
        "actual renderer begin diagnostic missing known Java-object correlation hints");
    check(android.util.Log.messages.stream().anyMatch(line -> line.contains("phase=javaRemoveSinkEnd")
        && line.contains(trackHash) && line.contains(aHash) && line.contains("returned=true")),
        "actual renderer completion diagnostic missing returned outcome");
    check(android.util.Log.messages.stream().anyMatch(line -> line.contains("phase=leaseAssociation")
        && line.contains(trackHash) && line.contains(ownerHash) && line.contains("leaseJava#")),
        "first physical lease release association not logged");
    check(shared.sinks.size() == 1 && shared.removes.get() == 1 && b.checkVideoTrack(shared.id(), "local"),
        "target stream detach removed another stream renderer from shared track");
    check(aProducer.released.get() == 0 && bProducer.released.get() == 0, "stream detach disposed a renderer producer");
    dispose(a);
    check(shared.sinks.size() == 1 && bProducer.released.get() == 0, "old stream renderer disposal released shared live renderer");
    int beforeFinalIdCalls = shared.idCalls.get();
    dispose(b);
    check(shared.idCalls.get() == beforeFinalIdCalls, "final diagnostic release introduced track.id JNI");
    String bHash = "rendererJava#" + Integer.toHexString(System.identityHashCode(b));
    String bSinkHash = "sinkJava#" + Integer.toHexString(System.identityHashCode(bSurface));
    check(android.util.Log.messages.stream().anyMatch(line -> line.contains("phase=javaRemoveSinkBegin")
        && line.contains(trackHash) && line.contains(bHash) && line.contains(bSinkHash)),
        "second actual sink diagnostic missing known fixture objects");
    check(shared.sinks.isEmpty() && shared.removes.get() == 2 && aProducer.released.get() == 1 && bProducer.released.get() == 1,
        "shared track sinks/producers did not each release exactly once");
  }

  public static void main(String[] args) throws Exception {
    actualNullStreamAndDisposeWaitForPhysicalDetach(); System.out.println("PASS actual setTrack -> setStream(null) -> Dispose waits for physical detach/EGL/main producer and Result");
    oldRendererCompletionCannotReleaseIndependentProducer(); System.out.println("PASS actual old renderer/generation cannot attach late or release independent producer");
    actualNativeDetachFailureDoesNotReportSuccessOrDeleteProducer(); System.out.println("PASS actual native detach failure retains lease/producer, no successful Result/onDisposed");
    actualStreamTrackSelectionAndUnavailableOwner(); System.out.println("PASS actual setStream track selection and missing-owner rejection");
    actualEglFailurePreventsProducerSuccess(); System.out.println("PASS actual EGL failure propagates and retains producer ownership");
    realDeadlineStillRemovesTombstoneAfterLatePhysicalRelease(); System.out.println("PASS actual 12s timeout retains tombstone until late physical success; Result remains exactly once");
    detachOneLocalStreamPreservesOtherRendererOnSameTrack(); System.out.println("PASS actual local stream detach preserves another renderer/sink on shared native track");
    System.out.println("PASS 7 actual production FlutterRTCVideoRenderer wiring cases; fake Android/Flutter/JNI/EGL boundaries, not device acceptance");
  }
}
