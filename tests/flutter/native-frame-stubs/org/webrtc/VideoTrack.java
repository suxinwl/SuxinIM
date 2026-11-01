package org.webrtc;
import java.util.Set;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.atomic.AtomicInteger;
/** Fake JNI/borrowed-frame transfer boundary; actual candidate selects the sink proxy. */
public final class VideoTrack {
  private final String id;
  public final AtomicInteger adds=new AtomicInteger(),removes=new AtomicInteger(),idCalls=new AtomicInteger();
  public final Set<VideoSink> sinks=ConcurrentHashMap.newKeySet();
  public volatile Runnable onAdd=()->{},onRemove=()->{};
  public VideoTrack(String id){this.id=id;}
  public String id(){idCalls.incrementAndGet();return id;}
  public void addSink(VideoSink sink){adds.incrementAndGet();onAdd.run();sinks.add(sink);}
  public void removeSink(VideoSink sink){removes.incrementAndGet();onRemove.run();sinks.remove(sink);}
  public void emit(VideoFrame frame){try{for(VideoSink sink:sinks)sink.onFrame(frame);}finally{frame.release();}}
}
