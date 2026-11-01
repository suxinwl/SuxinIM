package android.os;
import java.util.concurrent.ConcurrentLinkedQueue;
/** Deterministic fake platform dispatch boundary, not an Android Looper. */
public final class Handler {
  private static final ConcurrentLinkedQueue<Runnable> MAIN = new ConcurrentLinkedQueue<>();
  public static volatile boolean rejectPosts;
  public Handler(Looper looper) {}
  public boolean post(Runnable action) { if (rejectPosts) return false; MAIN.add(action); return true; }
  public static void drainMainLoop() { for (Runnable action; (action = MAIN.poll()) != null;) action.run(); }
  public static int pending() { return MAIN.size(); }
}
