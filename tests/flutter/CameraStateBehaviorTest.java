package com.cloudwebrtc.webrtc;

import java.util.concurrent.CompletableFuture;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.TimeUnit;

/** Actual class and wait methods; no copied state machine or reflective flag. */
public final class CameraStateBehaviorTest {
  private static int passed;

  private static void expectWaitReturns(String label, Runnable wait, Runnable callback,
      boolean requireSleeping) throws Exception {
    CountDownLatch entered = new CountDownLatch(1);
    CompletableFuture<Void> result = new CompletableFuture<>();
    Thread reader = new Thread(() -> {
      entered.countDown();
      try {
        wait.run();
        result.complete(null);
      } catch (Throwable failure) {
        result.completeExceptionally(failure);
      }
    }, "proposal-camera-wait");
    // A test failure must not hang the host JVM. Never interrupt the product wait.
    reader.setDaemon(true);
    reader.start();
    if (!entered.await(1, TimeUnit.SECONDS)) throw new AssertionError("Reader not scheduled");
    if (requireSleeping) {
      long limit = System.nanoTime() + TimeUnit.SECONDS.toNanos(1);
      while (reader.getState() != Thread.State.TIMED_WAITING && !result.isDone()
          && System.nanoTime() < limit) Thread.yield();
      if (result.isDone()) throw new AssertionError("Actual wait returned before callback: " + label);
      if (reader.getState() != Thread.State.TIMED_WAITING)
        throw new AssertionError("Actual wait did not reach Thread.sleep: " + label);
    }
    if (callback != null) callback.run(); // Main is a different thread from reader.
    result.get(1, TimeUnit.SECONDS);
    reader.join(1000);
    if (reader.isAlive()) throw new AssertionError("Actual wait thread did not end: " + label);
    passed++;
    System.out.println("PASS " + label);
  }

  public static void main(String[] args) throws Exception {
    CameraEventsHandler opened = new CameraEventsHandler();
    opened.onFirstFrameAvailable();
    expectWaitReturns("already OPENED", opened::waitForCameraOpen, null, false);

    CameraEventsHandler closed = new CameraEventsHandler();
    closed.onCameraClosed();
    expectWaitReturns("already CLOSED", closed::waitForCameraClosed, null, false);

    CameraEventsHandler opening = new CameraEventsHandler();
    opening.onCameraOpening("synthetic-camera");
    expectWaitReturns("cross-thread first frame releases actual open wait",
        opening::waitForCameraOpen, opening::onFirstFrameAvailable, true);

    CameraEventsHandler closing = new CameraEventsHandler();
    closing.onFirstFrameAvailable();
    expectWaitReturns("cross-thread close releases actual close wait",
        closing::waitForCameraClosed, closing::onCameraClosed, true);

    CameraEventsHandler errorOpen = new CameraEventsHandler();
    expectWaitReturns("ERROR releases actual open wait", errorOpen::waitForCameraOpen,
        () -> errorOpen.onCameraError("synthetic-error"), true);

    CameraEventsHandler errorClose = new CameraEventsHandler();
    errorClose.onFirstFrameAvailable();
    expectWaitReturns("ERROR releases actual close wait", errorClose::waitForCameraClosed,
        () -> errorClose.onCameraError("synthetic-error"), true);

    if (passed != 6) throw new AssertionError("Unexpected case count");
    System.out.println("PASS 6 actual-production CameraEventsHandler behavior cases; "
        + "fake Log/interface, not old-JIT failure reproduction or Android/native timing proof");
  }
}
