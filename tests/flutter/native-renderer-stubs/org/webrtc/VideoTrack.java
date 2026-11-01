package org.webrtc;
import java.util.Set;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.atomic.AtomicInteger;
/** Fake JNI calls only; production renderer still decides their scheduling. */
public final class VideoTrack {
  private final String id;
  public final AtomicInteger adds = new AtomicInteger(), removes = new AtomicInteger();
  public final AtomicInteger idCalls = new AtomicInteger();
  public final Set<Object> sinks = ConcurrentHashMap.newKeySet();
  public volatile Runnable onAdd = () -> {}, onRemove = () -> {};
  public volatile Thread addThread, removeThread;
  public VideoTrack(String id) { this.id = id; }
  public String id() { idCalls.incrementAndGet(); return id; }
  public void addSink(Object sink) { addThread = Thread.currentThread(); adds.incrementAndGet(); onAdd.run(); sinks.add(sink); }
  public void removeSink(Object sink) { removeThread = Thread.currentThread(); removes.incrementAndGet(); onRemove.run(); sinks.remove(sink); }
}
