package android.util;
public final class Log {
  public static final java.util.List<String> messages = new java.util.concurrent.CopyOnWriteArrayList<>();
  public static int w(String tag, String message) { return 0; }
  public static int d(String tag, String message) { messages.add(message); return 0; }
}
