package com.cloudwebrtc.webrtc;

import java.util.concurrent.CompletableFuture;
import java.util.concurrent.atomic.AtomicReference;
import java.util.concurrent.atomic.AtomicLongArray;
import java.util.function.Consumer;
import org.webrtc.VideoFrame;
import org.webrtc.VideoSink;

/** Android source adapter: one bounded process pool and one mailbox per source epoch. */
final class NativeFrameHandoff implements VideoSink {
  interface Dispatcher { boolean post(Runnable action); }
  // Exactly one bounded two-worker pool per class loader; never one pool per renderer.
  private static final NativeFrameMailbox.FixedPool POOL = new NativeFrameMailbox.FixedPool();
  private static final class OwnedFrame implements NativeFrameMailbox.Frame {
    final VideoFrame frame;
    OwnedFrame(VideoFrame frame) { this.frame = frame; }
    public void retain() { frame.retain(); }
    public void release() { frame.release(); }
  }
  private final NativeFrameMailbox<OwnedFrame> mailbox;
  private final Dispatcher main;
  private final Consumer<Throwable> recordFailure;
  private final AtomicReference<Throwable> firstFailure = new AtomicReference<>();
  private final AtomicLongArray admissions = new AtomicLongArray(NativeFrameMailbox.Admission.values().length);
  private CompletableFuture<Void> sealed;
  static final class Pause {
    final NativeFrameMailbox.Pause raw;
    final CompletableFuture<Void> physicalDrain;
    Pause(NativeFrameMailbox.Pause raw, CompletableFuture<Void> physicalDrain) { this.raw=raw;this.physicalDrain=physicalDrain; }
  }
  NativeFrameHandoff(Consumer<VideoFrame> delivery, Consumer<Throwable> recordFailure, Dispatcher main) {
    this.main=main;this.recordFailure=recordFailure;
    mailbox = new NativeFrameMailbox<>(POOL, frame -> delivery.accept(frame.frame), this::recordFirst);
  }
  public void onFrame(VideoFrame frame) {
    // Native callback must return without producer/EGL/main/Media2 waits.
    // Rejected borrowed frames are not retained/released by this adapter.
    NativeFrameMailbox.Admission outcome = mailbox.tryOffer(new OwnedFrame(frame));
    admissions.incrementAndGet(outcome.ordinal());
  }
  private void recordFirst(Throwable failure) {
    // This observer is deliberately restricted to the renderer's CAS+post path.
    // No log, owner diagnostics, JNI or user callback may execute here.
    if(firstFailure.compareAndSet(null,failure)) recordFailure.accept(failure);
  }
  private void bridge(CompletableFuture<Void> raw, CompletableFuture<Void> publicFuture) {
    raw.whenComplete((unused,error)->{
      // raw may complete on the borrowed native callback during retain failure.
      // Do not run arbitrary CompletableFuture continuations on that callback.
      if(!main.post(()->{
        if(error==null)publicFuture.complete(null);
        else publicFuture.completeExceptionally(NativeMediaLifecycle.unwrap(error));
      })) {
        // Main cannot finalize producer ownership either. Keep a real tombstone
        // and pending public barrier; caller deadline never becomes fake success.
        recordFirst(error==null?new java.util.concurrent.RejectedExecutionException("Frame completion main post rejected"):NativeMediaLifecycle.unwrap(error));
      }
    });
  }
  Pause pause() {
    NativeFrameMailbox.Pause raw=mailbox.pause();
    CompletableFuture<Void> physical=new CompletableFuture<>();bridge(raw.physicalDrain,physical);
    return new Pause(raw,physical);
  }
  boolean resume(Pause token) { return mailbox.resume(token.raw); }
  CompletableFuture<Void> seal() {
    final CompletableFuture<Void> result;
    synchronized(this){if(sealed!=null)return sealed;result=new CompletableFuture<>();sealed=result;}
    bridge(mailbox.seal(),result);return result;
  }
  Throwable failure() { Throwable value=firstFailure.get();return value==null?mailbox.failure():value; }
  long admissionCount(NativeFrameMailbox.Admission value) { return admissions.get(value.ordinal()); }
}
