package org.webrtc;
import android.view.Surface;
import java.util.List;
import java.util.concurrent.CopyOnWriteArrayList;
import java.util.concurrent.atomic.AtomicInteger;
import java.util.function.Consumer;
/** Fake EGL native/thread boundary. Models private pending-frame ownership, not the real EGL implementation. */
public class EglRenderer {
  public static final List<EglRenderer> instances=new CopyOnWriteArrayList<>();
  public static volatile Consumer<EglRenderer> onInit=renderer->{};
  public final AtomicInteger frames=new AtomicInteger(),releaseStarted=new AtomicInteger(),releases=new AtomicInteger(),surfaceReleases=new AtomicInteger();
  public volatile Runnable beforeOnFrame=()->{},beforeRelease=()->{};
  public volatile Consumer<Runnable> surfaceRelease=Runnable::run;
  public volatile Thread initThread,frameThread,releaseThread;
  private VideoFrame pending;
  public EglRenderer(String name){instances.add(this);}
  public void init(EglBase.Context context,int[]attrs,RendererCommon.GlDrawer drawer){initThread=Thread.currentThread();onInit.accept(this);}
  public void setFpsReduction(float fps){}
  public void disableFpsReduction(){}
  public void pauseVideo(){}
  public void createEglSurface(Surface surface){}
  public void releaseEglSurface(Runnable complete){surfaceReleases.incrementAndGet();surfaceRelease.accept(complete);}
  public void onFrame(VideoFrame frame){
    frameThread=Thread.currentThread();beforeOnFrame.run();
    synchronized(this){if(pending!=null)pending.release();frame.retain();pending=frame;}
    frames.incrementAndGet();
  }
  public void release(){
    releaseThread=Thread.currentThread();releaseStarted.incrementAndGet();beforeRelease.run();
    synchronized(this){if(pending!=null){pending.release();pending=null;}}
    releases.incrementAndGet();
  }
}
