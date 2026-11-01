package com.cloudwebrtc.webrtc.utils;
import io.flutter.plugin.common.EventChannel.EventSink;
public final class AnyThreadSink implements EventSink {
  private final EventSink sink;
  public AnyThreadSink(EventSink sink) { this.sink = sink; }
  public void success(Object value) { sink.success(value); }
  public void error(String code, String message, Object details) { sink.error(code,message,details); }
  public void endOfStream() { sink.endOfStream(); }
}
