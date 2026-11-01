package io.flutter.plugin.common;
public final class EventChannel {
  public interface EventSink { void success(Object value); void error(String code, String message, Object details); void endOfStream(); }
  public interface StreamHandler { void onListen(Object args, EventSink sink); void onCancel(Object args); }
  public volatile StreamHandler handler;
  public void setStreamHandler(StreamHandler handler) { this.handler = handler; }
}
