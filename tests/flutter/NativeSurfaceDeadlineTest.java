package com.cloudwebrtc.webrtc;

import android.view.Surface;
import io.flutter.view.TextureRegistry.SurfaceProducer;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicInteger;
import java.util.concurrent.atomic.AtomicReference;
import org.webrtc.VideoFrame;

/** Actual production SurfaceTextureRenderer; fake EGL/Surface/Flutter only. */
public final class NativeSurfaceDeadlineTest {
  static void check(boolean ok, String text) { if (!ok) throw new AssertionError(text); }
  static final class Producer implements SurfaceProducer {
    Callback callback; int gets;
    public void setCallback(Callback value) { callback=value; }
    public void setSize(int width,int height) {}
    public Surface getSurface() { gets++; return new Surface(); }
  }
  static SurfaceTextureRenderer surface(NativeMediaLifecycle.Owner owner, Producer producer, AtomicInteger failures) {
    SurfaceTextureRenderer surface = new SurfaceTextureRenderer("synthetic-test");
    surface.setCleanupOwner(owner); surface.setCleanupFailureHandler(failures::incrementAndGet);
    surface.init(null, null); surface.surfaceCreated(producer); surface.onFrame(new VideoFrame(320,240));
    check(surface.frames.get()==1 && surface.creates.get()==1, "initial frame was not passed to real Surface lifecycle");
    return surface;
  }
  static void actualTwelveSecondDeadlineThenLateCompletionRebuildsSurface() throws Exception {
    CountDownLatch entered=new CountDownLatch(1), timeout=new CountDownLatch(1);
    NativeMediaLifecycle.Owner owner=new NativeMediaLifecycle.Owner(Runnable::run, event -> { if(event.contains("phase=callerTimeout")) timeout.countDown(); });
    Producer producer=new Producer(); AtomicInteger failures=new AtomicInteger();
    SurfaceTextureRenderer surface=surface(owner,producer,failures); AtomicReference<Runnable> release=new AtomicReference<>();
    surface.releaseAction=completion -> { release.set(completion); entered.countDown(); };
    try {
      long start=System.nanoTime(); producer.callback.onSurfaceCleanup();
      check(TimeUnit.NANOSECONDS.toMillis(System.nanoTime()-start)<200, "Surface cleanup blocked main on EGL completion");
      check(entered.await(3,TimeUnit.SECONDS) && timeout.await(15,TimeUnit.SECONDS), "actual production12s caller deadline was not reached");
      check(owner.isQuarantined() && !owner.isPermanentlyQuarantined() && failures.get()==0, "caller deadline was promoted to hard Surface failure");
      surface.setCleanupFailureHandler(failures::incrementAndGet);
      check(failures.get()==0, "late failure-handler installation promoted soft barrier to hard failure");
      producer.callback.onSurfaceAvailable(); surface.onFrame(new VideoFrame(320,240));
      check(surface.frames.get()==1 && surface.creates.get()==1 && producer.gets==1, "pending EGL cleanup allowed premature surface recreation");
      release.get().run(); long until=System.nanoTime()+TimeUnit.SECONDS.toNanos(3);
      while(owner.isQuarantined() && System.nanoTime()<until) Thread.sleep(1);
      check(!owner.isQuarantined(), "late actual EGL completion did not remove exact soft deadline");
      surface.throwIfCleanupFailed(); surface.onFrame(new VideoFrame(320,240));
      check(surface.frames.get()==2 && surface.creates.get()==2 && producer.gets==2 && failures.get()==0,
          "actual Surface did not recreate and render after genuine late success");
    } finally { if(release.get()!=null)release.get().run(); }
  }
  static void actualEglFailureRemainsHardAndStopsFrames() throws Exception {
    NativeMediaLifecycle.Owner owner=new NativeMediaLifecycle.Owner(); Producer producer=new Producer(); AtomicInteger failures=new AtomicInteger();
    SurfaceTextureRenderer surface=surface(owner,producer,failures); RuntimeException marker=new IllegalStateException("synthetic EGL failure");
    surface.releaseAction=completion -> {throw marker;}; producer.callback.onSurfaceCleanup();
    long until=System.nanoTime()+TimeUnit.SECONDS.toNanos(3); while(failures.get()==0 && System.nanoTime()<until)Thread.sleep(1);
    check(owner.isPermanentlyQuarantined() && failures.get()==1, "actual EGL failure did not retain permanent owner/failure callback");
    try {surface.throwIfCleanupFailed();throw new AssertionError("true failure forgotten");} catch(RuntimeException error){check(error==marker,"first EGL error identity was replaced");}
    producer.callback.onSurfaceAvailable(); surface.onFrame(new VideoFrame(320,240));
    check(surface.frames.get()==1 && surface.creates.get()==1 && surface.surfaceReleases.get()==1, "failed EGL cleanup resumed or retried native release");
  }
  static void immediatePhysicalSuccessCannotLeaveDeadlineBarrier() {
    NativeMediaLifecycle.Owner owner=new NativeMediaLifecycle.Owner(Runnable::run,Runnable::run,8); Producer producer=new Producer(); AtomicInteger failures=new AtomicInteger();
    SurfaceTextureRenderer surface=surface(owner,producer,failures);
    for(int i=0;i<20;i++){producer.callback.onSurfaceCleanup(); producer.callback.onSurfaceAvailable(); surface.onFrame(new VideoFrame(320,240));}
    check(!owner.isQuarantined() && failures.get()==0 && surface.frames.get()==21 && surface.creates.get()==21 && surface.surfaceReleases.get()==20,
        "completed physical cleanup left barrier or lost real recreate/frame sequence");
  }
  public static void main(String[] args) throws Exception {
    actualTwelveSecondDeadlineThenLateCompletionRebuildsSurface(); System.out.println("PASS actual Surface12s deadline preserves pending EGL; late actual completion recreates surface and renders again without hard failure");
    actualEglFailureRemainsHardAndStopsFrames(); System.out.println("PASS actual Surface native EGL failure keeps original cause/permanent barrier and stops new frames");
    immediatePhysicalSuccessCannotLeaveDeadlineBarrier(); System.out.println("PASS actual Surface synchronous completion20cycles leaves no deadline barrier");
    System.out.println("PASS 3 actual production SurfaceTextureRenderer deadline/physical-outcome cases; fake EGL/platform boundary, not Android GPU acceptance");
  }
}
