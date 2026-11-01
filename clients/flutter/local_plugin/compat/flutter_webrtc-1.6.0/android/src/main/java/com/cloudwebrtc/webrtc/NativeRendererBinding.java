package com.cloudwebrtc.webrtc;

import java.util.concurrent.CompletableFuture;
import java.util.concurrent.Executor;

/** Track sink ownership and renderer output lifetime; independent of Android. */
final class NativeRendererBinding {
  interface Track {
    void attach(); void detach(); default Object key() { return this; }
    default CompletableFuture<Void> prepare(java.util.function.BooleanSupplier current) { return CompletableFuture.completedFuture(null); }
    default void stopFrameAdmission() {}
    default CompletableFuture<Void> retireAfterNativeDetach() { return CompletableFuture.completedFuture(null); }
  }
  interface Output { void releaseEgl(); void releaseProducerOnMain(); }
  static final class SetRequest {
    final boolean accepted;
    final CompletableFuture<Void> physical;
    SetRequest(boolean accepted, CompletableFuture<Void> physical) { this.accepted = accepted; this.physical = physical; }
  }
  private final Executor main;
  private final Output output;
  private final NativeMediaLifecycle.Owner outputOwner;
  private final java.util.function.Consumer<String> diagnostics;
  private final NativeMediaLifecycle.PhysicalDeadlines deadlines;
  private final java.util.Set<TrackLease> leases = java.util.concurrent.ConcurrentHashMap.newKeySet();
  private CompletableFuture<Void> tail = CompletableFuture.completedFuture(null);
  private CompletableFuture<Void> disposal;
  private TrackLease current;
  private final java.util.concurrent.atomic.AtomicLong generation = new java.util.concurrent.atomic.AtomicLong();
  private volatile boolean closing;
  private volatile boolean failed;
  private int pending;
  NativeRendererBinding(Executor main, Output output) { this(main, output, new NativeMediaLifecycle.Owner()); }
  NativeRendererBinding(Executor main, Output output, NativeMediaLifecycle.Owner outputOwner) {
    this(main, output, outputOwner, unused -> {});
  }
  NativeRendererBinding(Executor main, Output output, NativeMediaLifecycle.Owner outputOwner, java.util.function.Consumer<String> diagnostics) {
    this.main = main; this.output = output; this.outputOwner = outputOwner;
    this.diagnostics = diagnostics;
    this.deadlines = new NativeMediaLifecycle.PhysicalDeadlines(32, this::emit, this::quarantine);
  }

  CompletableFuture<Void> set(NativeMediaLifecycle.Owner owner, Track track) {
    return requestSet(owner, track).physical;
  }
  SetRequest requestSet(NativeMediaLifecycle.Owner owner, Track track) {
    final CompletableFuture<Void> before, physical = new CompletableFuture<>();
    final TrackLease previous, next;
    final long requested;
    synchronized (this) {
      final boolean pendingDeadline = deadlines.pendingCount() != 0;
      if (closing || failed) return new SetRequest(false, NativeMediaLifecycle.failed("RTC_RENDERER_UNAVAILABLE", "Renderer is closing or quarantined"));
      if (track != null && pendingDeadline) return new SetRequest(false, NativeMediaLifecycle.failed("RTC_RENDERER_BUSY", "Renderer physical operation is still pending after caller deadline"));
      if (pending >= 8) { failed = true; return new SetRequest(false, NativeMediaLifecycle.failed("RTC_RENDERER_BUSY", "Bounded renderer request queue is occupied")); }
      requested = generation.incrementAndGet(); previous = current;
      next = track == null ? null : new TrackLease(owner, track, requested);
      current = next; before = tail; tail = physical; pending++;
    }
    before.whenComplete((unused, earlierError) -> {
      if (earlierError != null) {
        if (next != null) next.skipAttach();
        physical.completeExceptionally(NativeMediaLifecycle.unwrap(earlierError)); return;
      }
      CompletableFuture<Void> detached = previous == null ? CompletableFuture.completedFuture(null) : previous.release();
      detached.whenComplete((v, detachError) -> {
        if (detachError != null) {
          if (next != null) next.skipAttach();
          physical.completeExceptionally(NativeMediaLifecycle.unwrap(detachError)); return;
        }
        if (next == null) physical.complete(null);
        else {
          if (requested != generation.get() || closing || failed) next.skipAttach(); else next.attach();
          next.attachedBarrier.whenComplete((ignored, attachError) -> {
            if (attachError == null) physical.complete(null); else physical.completeExceptionally(NativeMediaLifecycle.unwrap(attachError));
          });
        }
      });
    });
    physical.whenComplete((v, error) -> {
      synchronized (this) { pending--; if (error != null) failed = true; }
      emit("operation=renderer.set phase=physicalOutcome outcome=" + (error == null ? "success" : "failure")
        + (error == null ? "" : " failure=" + NativeMediaLifecycle.unwrap(error).getClass().getSimpleName()));
    });
    return new SetRequest(true, physical);
  }
  void quarantine() { failed = true; generation.incrementAndGet(); emit("phase=quarantine reason=physical_or_surface_failure"); }
  void callerTimedOut(CompletableFuture<Void> physical, String operation) { deadlines.timedOut(physical, operation); }
  private void emit(String event) {
    final int pendingCount;
    synchronized (this) { pendingCount = pending; }
    try { diagnostics.accept(event + " generation=" + generation.get() + " closing=" + closing + " failed=" + failed
      + " bindingPending=" + pendingCount + " deadlinePending=" + deadlines.pendingCount()); }
    catch (Throwable ignored) { /* A diagnostic callback cannot own media. */ }
  }
  boolean isClosing() { return closing; }
  NativeMediaLifecycle.Owner outputOwner() { return outputOwner; }
  CompletableFuture<Void> dispose() {
    final CompletableFuture<Void> previous, done;
    synchronized (this) {
      if (disposal != null) return disposal;
      closing = true; generation.incrementAndGet(); previous = tail;
      done = new CompletableFuture<>(); disposal = done;
    }
    previous.whenComplete((v, earlierError) -> {
      if (earlierError != null) { done.completeExceptionally(NativeMediaLifecycle.unwrap(earlierError)); return; }
      CompletableFuture<Void> detached = CompletableFuture.allOf(leases.stream().map(TrackLease::release).toArray(CompletableFuture[]::new));
      detached.thenCompose(unused -> outputOwner.submit("renderer.output.release", output::releaseEgl, true)).whenComplete((unused, error) -> {
        if (error != null) { done.completeExceptionally(NativeMediaLifecycle.unwrap(error)); return; }
        try {
          main.execute(() -> {
            try { output.releaseProducerOnMain(); done.complete(null); }
            catch (Throwable failure) { done.completeExceptionally(failure); }
          });
        } catch (Throwable rejectedMain) { done.completeExceptionally(rejectedMain); }
      });
    });
    return done;
  }
  private final class TrackLease implements NativeMediaLifecycle.Lease {
    final NativeMediaLifecycle.Owner owner; final Track track; final long requested;
    final CompletableFuture<Void> attachedBarrier = new CompletableFuture<>();
    private CompletableFuture<Void> detachedBarrier;
    private volatile boolean attached;
    TrackLease(NativeMediaLifecycle.Owner owner, Track track, long requested) {
      this.owner = owner; this.track = track; this.requested = requested;
      leases.add(this); owner.register(this);
    }
    void skipAttach() { attachedBarrier.complete(null); }
    boolean stillCurrent() { return !closing && !failed && requested == generation.get(); }
    void attach() {
      final CompletableFuture<Void> prepared;
      try { prepared = track.prepare(() -> stillCurrent() && !owner.isClosing() && !owner.isPermanentlyQuarantined()); }
      catch (Throwable error) { attachedBarrier.completeExceptionally(error); return; }
      prepared.whenComplete((unused, preparationError) -> {
        if (preparationError != null) { attachedBarrier.completeExceptionally(NativeMediaLifecycle.unwrap(preparationError)); return; }
        if (!stillCurrent()) { attachedBarrier.complete(null); return; }
        if (owner.isPermanentlyQuarantined()) {
          attachedBarrier.completeExceptionally(new NativeMediaLifecycle.Failure("RTC_MEDIA_UNAVAILABLE", "Track owner has a permanent physical failure")); return;
        }
        if (owner.isClosing()) { attachedBarrier.complete(null); return; }
        owner.submit("renderer.attach", () -> {
          if (!stillCurrent()) return;
          if (owner.isPermanentlyQuarantined()) {
            emit("operation=renderer.attach phase=attachRejected reason=owner_hard_failure");
            throw new NativeMediaLifecycle.Failure("RTC_MEDIA_UNAVAILABLE", "Track owner has a permanent physical failure");
          }
          if (owner.isClosing()) { emit("operation=renderer.attach phase=attachRetired reason=owner_closing"); return; }
          track.attach(); attached = true;
        }, false).whenComplete((v, error) -> {
          if (error == null) attachedBarrier.complete(null);
          else attachedBarrier.completeExceptionally(NativeMediaLifecycle.unwrap(error));
        });
      });
    }
    public Object key() { return track.key(); }
    public CompletableFuture<Void> release() {
      // Publish before any synchronous completion can re-enter this lease.
      final CompletableFuture<Void> done;
      synchronized (this) {
        if (detachedBarrier != null) return detachedBarrier;
        done = new CompletableFuture<>(); detachedBarrier = done;
      }
      // Outside the lease monitor; these hashes do not uniquely identify native tracks.
      try {
        emit("operation=renderer.detach phase=leaseAssociation"
          + " leaseJava#" + Integer.toHexString(System.identityHashCode(this))
          + " bindingTrackJava#" + Integer.toHexString(System.identityHashCode(track))
          + " ownerJava#" + Integer.toHexString(System.identityHashCode(owner))
          + " trackJava#" + Integer.toHexString(System.identityHashCode(track.key())));
      } catch (Throwable ignored) { /* Current key() is only return-next/default-this. */ }
      try { track.stopFrameAdmission(); }
      catch (Throwable error) { done.completeExceptionally(error); return done; }
      attachedBarrier.whenComplete((v, attachError) -> {
        // An unknown addSink/init failure keeps this lease and its epoch quarantined.
        if (attachError != null) { done.completeExceptionally(NativeMediaLifecycle.unwrap(attachError)); return; }
        owner.submit("renderer.detach", () -> {
          if (attached) { track.detach(); attached = false; }
        }, true).thenCompose(unused -> track.retireAfterNativeDetach()).whenComplete((unused, error) -> {
          if (error == null) { owner.unregister(this); leases.remove(this); done.complete(null); }
          else done.completeExceptionally(NativeMediaLifecycle.unwrap(error));
        });
      });
      return done;
    }
  }
}
