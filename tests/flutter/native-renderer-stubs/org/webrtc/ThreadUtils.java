package org.webrtc;
import java.util.concurrent.CountDownLatch;
public final class ThreadUtils {
  private static final Thread MAIN = Thread.currentThread();
  public static void checkIsOnMainThread() { if (Thread.currentThread() != MAIN) throw new AssertionError("platform operation not on main"); }
  public static void awaitUninterruptibly(CountDownLatch latch) {
    boolean interrupted = false;
    for (;;) { try { latch.await(); break; } catch (InterruptedException error) { interrupted = true; } }
    if (interrupted) Thread.currentThread().interrupt();
  }
}
