package android.util;
public final class Log {
  public static final java.util.List<String> messages = new java.util.concurrent.CopyOnWriteArrayList<>();
  public static final java.util.List<Thread> warningThreads = new java.util.concurrent.CopyOnWriteArrayList<>();
  public static int w(String tag, String message) { messages.add(message); warningThreads.add(Thread.currentThread()); return 0; }
  public static int d(String tag, String message) { messages.add(message); return 0; }
}
