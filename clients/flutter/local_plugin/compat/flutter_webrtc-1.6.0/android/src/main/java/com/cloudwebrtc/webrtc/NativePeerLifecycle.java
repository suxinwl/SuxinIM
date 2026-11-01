package com.cloudwebrtc.webrtc;

import java.util.concurrent.CompletableFuture;

/** One real close and dispose operation. Caller deadlines never resolve these
 * physical barriers, and joining a pending operation never retries its JNI. */
final class NativePeerLifecycle {
  private final NativeMediaLifecycle.Owner owner;
  private final Runnable closeNative, disposeNative;
  private CompletableFuture<Void> closePhysical, disposePhysical;
  NativePeerLifecycle(NativeMediaLifecycle.Owner owner, Runnable closeNative, Runnable disposeNative) {
    this.owner = owner; this.closeNative = closeNative; this.disposeNative = disposeNative;
  }
  CompletableFuture<Void> close() {
    final CompletableFuture<Void> done;
    synchronized (this) {
      if (closePhysical != null) return closePhysical;
      done = new CompletableFuture<>(); closePhysical = done;
    }
    try {
      owner.detachAll().thenCompose(unused -> owner.submit("peer.close", closeNative, true))
        .whenComplete((unused, error) -> finish(done, error));
    } catch (Throwable error) { finish(done, error); }
    return done;
  }
  CompletableFuture<Void> dispose() {
    final CompletableFuture<Void> done;
    synchronized (this) {
      if (disposePhysical != null) return disposePhysical;
      done = new CompletableFuture<>(); disposePhysical = done;
    }
    // Real failure retains the peer/leases. A prior caller timeout is not a
    // failure here: the original physical close may still genuinely succeed.
    close().thenCompose(unused -> owner.submit("peer.dispose", disposeNative, true))
      .whenComplete((unused, error) -> finish(done, error));
    return done;
  }
  private static void finish(CompletableFuture<Void> done, Throwable error) {
    if (error == null) done.complete(null); else done.completeExceptionally(NativeMediaLifecycle.unwrap(error));
  }
}
