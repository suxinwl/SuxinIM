package org.webrtc;
import java.util.concurrent.atomic.AtomicInteger;
/** Controlled refcounts only; no native buffer/driver implementation. */
public final class VideoFrame {
  public static final class Buffer {
    final int width, height; Buffer(int width, int height) { this.width=width; this.height=height; }
    public int getWidth() { return width; } public int getHeight() { return height; }
  }
  final Buffer buffer;
  public final AtomicInteger refs = new AtomicInteger(1), retains = new AtomicInteger(), releases = new AtomicInteger();
  public volatile Runnable beforeRetain = () -> {}, beforeRelease = () -> {};
  public VideoFrame(int width,int height) { buffer=new Buffer(width,height); }
  public int getRotatedWidth(){return buffer.width;}
  public int getRotatedHeight(){return buffer.height;}
  public int getRotation(){return 0;}
  public Buffer getBuffer(){return buffer;}
  public void retain(){beforeRetain.run();retains.incrementAndGet();refs.incrementAndGet();}
  public void release(){beforeRelease.run();releases.incrementAndGet();if(refs.decrementAndGet()<0)throw new AssertionError("double frame release");}
}
