package com.cloudwebrtc.webrtc;

import android.os.Handler;
import android.view.Surface;
import io.flutter.plugin.common.EventChannel;
import io.flutter.plugin.common.MethodChannel.Result;
import io.flutter.view.TextureRegistry.SurfaceProducer;
import org.webrtc.EglRenderer;
import org.webrtc.VideoFrame;
import org.webrtc.VideoTrack;
import java.util.List;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.CopyOnWriteArrayList;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicInteger;
import java.util.concurrent.atomic.AtomicReference;

/** Actual external Renderer/Surface/Binding/mailbox Java wiring; fake Android/JNI/EGL boundaries only. */
public final class NativeFrameIntegrationTest {
  static final Thread MAIN=Thread.currentThread();
  static void check(boolean value,String why){if(!value)throw new AssertionError(why);}
  static void await(CountDownLatch latch){try{check(latch.await(3,TimeUnit.SECONDS),"test latch timed out");}catch(InterruptedException e){throw new AssertionError(e);}}
  static void until(java.util.function.BooleanSupplier condition,boolean main)throws Exception{
    long end=System.nanoTime()+TimeUnit.SECONDS.toNanos(3);
    while(!condition.getAsBoolean()&&System.nanoTime()<end){if(main)Handler.drainMainLoop();Thread.sleep(1);}
    check(condition.getAsBoolean(),"condition did not settle");
  }
  static final class Reply implements Result{
    final AtomicInteger count=new AtomicInteger();volatile String code;
    public void success(Object value){check(Thread.currentThread()==MAIN,"result not on main");count.incrementAndGet();}
    public void error(String code,String message,Object details){check(Thread.currentThread()==MAIN,"error not on main");this.code=code;count.incrementAndGet();}
    public void notImplemented(){throw new AssertionError();}
    void done()throws Exception{until(()->count.get()>0,true);check(count.get()==1,"duplicate result");}
    void success()throws Exception{done();check(code==null,"unexpected error: "+code);}
  }
  static final class Producer implements SurfaceProducer{
    volatile Callback callback;final AtomicInteger releases=new AtomicInteger(),sizes=new AtomicInteger();volatile Thread sizeThread;
    public void setCallback(Callback value){check(Thread.currentThread()==MAIN,"callback not installed on main");callback=value;}
    public void setSize(int w,int h){sizes.incrementAndGet();sizeThread=Thread.currentThread();}
    public Surface getSurface(){return new Surface();}
    public void release(){check(Thread.currentThread()==MAIN,"producer release not on main");releases.incrementAndGet();}
  }
  static final class Sink implements EventChannel.EventSink{
    final List<Object> events=new CopyOnWriteArrayList<>();
    public void success(Object event){check(Thread.currentThread()==MAIN,"event not on main");events.add(event);}
    public void error(String code,String message,Object detail){throw new AssertionError("unexpected event error");}
    public void endOfStream(){}
  }
  static final class Fixture{
    final NativeMediaLifecycle.Owner owner=new NativeMediaLifecycle.Owner();
    final Producer producer=new Producer();
    final FlutterRTCVideoRenderer renderer=new FlutterRTCVideoRenderer(producer,(track,tag)->owner);
    Reply set(VideoTrack track){Reply result=new Reply();renderer.setTrack(track,"stream","local",result);return result;}
    EglRenderer attach(VideoTrack track)throws Exception{int before=EglRenderer.instances.size();set(track).success();check(EglRenderer.instances.size()==before+1,"not one fresh epoch");return EglRenderer.instances.get(before);}
    void dispose()throws Exception{Reply result=new Reply();renderer.Dispose(result,()->{});result.success();check(producer.releases.get()==1,"producer not released exactly once");}
  }

  static void sourceCallbackUsesProxyNotEglOrProducer()throws Exception{
    Fixture f=new Fixture();VideoTrack track=new VideoTrack("proxy");EglRenderer egl=f.attach(track);
    check(track.sinks.size()==1&&track.sinks.iterator().next() instanceof NativeFrameHandoff,"native sink is still EGL renderer");
    VideoFrame frame=new VideoFrame(640,360);track.emit(frame);
    until(()->egl.frames.get()==1,false);
    check(egl.frameThread!=MAIN&&f.producer.sizeThread!=MAIN,"source callback performed EGL/producer work");
    check(egl.frameThread.getName().startsWith("NativeFrameMailbox-"),"delivery not on bounded Frame2 pool");
    until(()->frame.refs.get()==1,false); // Only fake EGL's private pending-frame retain remains.
    f.dispose();check(frame.refs.get()==0&&frame.retains.get()==2&&frame.releases.get()==3,"mailbox/EGL/borrowed refs did not close");
  }

  static void actualLeaseWaitsEnteredDeliveryAndRealEglReturn()throws Exception{
    Fixture f=new Fixture();VideoTrack track=new VideoTrack("held");EglRenderer egl=f.attach(track);
    CountDownLatch entered=new CountDownLatch(1),allowFrame=new CountDownLatch(1),allowEgl=new CountDownLatch(1);
    egl.beforeOnFrame=()->{entered.countDown();await(allowFrame);};egl.beforeRelease=()->await(allowEgl);
    VideoFrame frame=new VideoFrame(640,360);track.emit(frame);await(entered);
    Reply cleared=f.set(null);until(()->track.removes.get()==1,false);
    AtomicInteger pcClosed=new AtomicInteger();CompletableFuture<Void> pc=f.owner.detachAll().thenCompose(v->f.owner.submit(pcClosed::incrementAndGet,true));
    f.owner.submit("host.marker",()->{},true).get(3,TimeUnit.SECONDS);
    check(!pc.isDone()&&cleared.count.get()==0&&egl.releaseStarted.get()==0,"lease ended at native detach only");
    allowFrame.countDown();until(()->egl.releaseStarted.get()==1,true);
    check(!pc.isDone()&&frame.refs.get()==1,"mailbox drain was treated as private EGL drain");
    allowEgl.countDown();cleared.success();pc.get(3,TimeUnit.SECONDS);
    check(pcClosed.get()==1&&frame.refs.get()==0&&track.removes.get()==1,"physical close order/identity wrong");f.dispose();
  }

  static void sourceReplacementUsesFreshEglAfterOldRetirement()throws Exception{
    Fixture f=new Fixture();VideoTrack first=new VideoTrack("old"),next=new VideoTrack("new");EglRenderer old=f.attach(first);
    CountDownLatch allow=new CountDownLatch(1);old.beforeRelease=()->await(allow);int count=EglRenderer.instances.size();
    Reply switched=f.set(next);until(()->old.releaseStarted.get()==1,true);
    check(next.adds.get()==0&&EglRenderer.instances.size()==count&&f.producer.releases.get()==0,"new EGL/source started before old actual release");
    allow.countDown();switched.success();EglRenderer fresh=EglRenderer.instances.get(count);
    check(fresh!=old&&fresh.initThread==MAIN&&old.releases.get()==1&&next.adds.get()==1,"source epoch not physically replaced");
    check(first.idCalls.get()==1&&next.idCalls.get()==1,"prototype introduced extra VideoTrack.id JNI");f.dispose();check(fresh.releases.get()==1,"fresh EGL not released");
  }

  static void queuedPrepareSupersededBeforeMainAllocatesNothing()throws Exception{
    Fixture f=new Fixture();int count=EglRenderer.instances.size();VideoTrack track=new VideoTrack("superseded");
    Reply first=f.set(track),cleared=f.set(null);first.success();cleared.success();
    check(EglRenderer.instances.size()==count&&track.adds.get()==0,"stale queued preparation allocated EGL/native sink");f.dispose();
  }

  static void preparedThenSupersededRetiresUnattachedEpoch()throws Exception{
    Fixture f=new Fixture();VideoTrack first=new VideoTrack("prepared"),next=new VideoTrack("current");AtomicReference<Reply> switched=new AtomicReference<>();AtomicInteger once=new AtomicInteger();int count=EglRenderer.instances.size();
    EglRenderer.onInit=egl->{if(once.getAndIncrement()==0)switched.set(f.set(next));};
    try{Reply initial=f.set(first);initial.success();until(()->switched.get()!=null,true);switched.get().success();}
    finally{EglRenderer.onInit=egl->{};}
    check(first.adds.get()==0&&next.adds.get()==1,"superseded prepared source attached");
    check(EglRenderer.instances.get(count).releases.get()==1&&EglRenderer.instances.size()==count+2,"unattached prepared EGL was lost");
    check(f.producer.releases.get()==0,"old prepared completion released current producer");f.dispose();
  }

  static void finalRawEventDeliveryChecksSourceAndSubscription()throws Exception{
    Fixture f=new Fixture();VideoTrack first=new VideoTrack("event-old"),next=new VideoTrack("event-new");EglRenderer egl=f.attach(first);Sink old=new Sink(),fresh=new Sink();f.renderer.onListen(null,old);
    VideoFrame frame=new VideoFrame(640,360);first.emit(frame);until(()->egl.frames.get()==1,false);
    f.renderer.onCancel(null);f.renderer.onListen(null,fresh);Handler.drainMainLoop();
    check(old.events.isEmpty()&&fresh.events.isEmpty(),"old queued event crossed subscription identity");
    VideoFrame resize=new VideoFrame(1280,720);first.emit(resize);until(()->egl.frames.get()==2,false);
    Reply switched=f.set(next);Handler.drainMainLoop();switched.success();
    check(fresh.events.isEmpty(),"old source event delivered after new source request");
    VideoFrame current=new VideoFrame(640,360);next.emit(current);until(()->!fresh.events.isEmpty(),true);
    f.dispose();check(frame.refs.get()==0&&resize.refs.get()==0&&current.refs.get()==0,"event path leaked accepted frames");
  }

  static void disposeAndOwnerCloseRetireQueuedOrPreparedEpoch()throws Exception{
    Fixture queued=new Fixture();VideoTrack q=new VideoTrack("queued-dispose");int count=EglRenderer.instances.size();
    Reply qset=queued.set(q),qdispose=new Reply();queued.renderer.Dispose(qdispose,()->{});qset.success();qdispose.success();
    check(q.adds.get()==0&&EglRenderer.instances.size()==count&&queued.producer.releases.get()==1,"dispose lost queued preparation");
    Fixture prepared=new Fixture();VideoTrack p=new VideoTrack("prepared-dispose");AtomicReference<Reply> pdispose=new AtomicReference<>();AtomicInteger once=new AtomicInteger();count=EglRenderer.instances.size();
    EglRenderer.onInit=egl->{if(once.getAndIncrement()==0){Reply r=new Reply();pdispose.set(r);prepared.renderer.Dispose(r,()->{});}};
    try{Reply pset=prepared.set(p);pset.success();pdispose.get().success();}finally{EglRenderer.onInit=egl->{};}
    check(p.adds.get()==0&&EglRenderer.instances.get(count).releases.get()==1&&prepared.producer.releases.get()==1,"late prepared EGL was abandoned on dispose");
    Fixture closing=new Fixture();VideoTrack c=new VideoTrack("owner-closing");count=EglRenderer.instances.size();Reply cset=closing.set(c);closing.owner.closeForNewWork();
    cset.success();CompletableFuture<Void> ownerClosed=closing.owner.detachAll();until(ownerClosed::isDone,true);ownerClosed.get();
    check(c.adds.get()==0&&EglRenderer.instances.size()==count,"owner close allowed queued init/attach");closing.dispose();
  }

  static void sharedNativeTrackKeepsBothProxyLeasesThroughEgl()throws Exception{
    Fixture first=new Fixture();Producer secondProducer=new Producer();FlutterRTCVideoRenderer second=new FlutterRTCVideoRenderer(secondProducer,(track,tag)->first.owner);
    VideoTrack track=new VideoTrack("shared");EglRenderer left=first.attach(track);int count=EglRenderer.instances.size();Reply attached=new Reply();second.setTrack(track,"stream","local",attached);attached.success();EglRenderer right=EglRenderer.instances.get(count);
    VideoFrame frame=new VideoFrame(640,360);track.emit(frame);until(()->left.frames.get()==1&&right.frames.get()==1,false);
    CompletableFuture<Void> released=first.owner.detachMatching(track);until(released::isDone,true);released.get();
    check(track.removes.get()==2&&track.adds.get()==2&&left.releases.get()==1&&right.releases.get()==1&&frame.refs.get()==0,"shared wrapper key or distinct proxy ownership lost");
    check(first.producer.releases.get()==0&&secondProducer.releases.get()==0,"track release disposed producer");first.dispose();Reply dispose=new Reply();second.Dispose(dispose,()->{});dispose.success();check(secondProducer.releases.get()==1,"second producer not released");
  }

  static void actualEglFailureIsRetainedWithoutSecondRelease()throws Exception{
    Fixture f=new Fixture();VideoTrack track=new VideoTrack("failed-egl");EglRenderer egl=f.attach(track);RuntimeException original=new RuntimeException("fake EGL release failure");egl.beforeRelease=()->{throw original;};
    Reply cleared=f.set(null);cleared.done();check(cleared.code!=null,"actual EGL failure became success");
    CompletableFuture<Void> lease=f.owner.detachMatching(track);try{lease.get(3,TimeUnit.SECONDS);throw new AssertionError("failed lease lost");}catch(java.util.concurrent.ExecutionException error){check(NativeMediaLifecycle.unwrap(error)==original,"EGL first error identity changed");}
    Reply dispose=new Reply();f.renderer.Dispose(dispose,()->{});dispose.done();
    check(dispose.code!=null&&egl.releaseStarted.get()==1&&egl.releases.get()==0&&track.removes.get()==1&&f.producer.releases.get()==0,"actual failed EGL retried or released producer");
  }

  static void earlyAvailableCannotCrossPendingSurfaceCleanup()throws Exception{
    Fixture f=new Fixture();VideoTrack track=new VideoTrack("surface");EglRenderer egl=f.attach(track);
    CountDownLatch entered=new CountDownLatch(1),allow=new CountDownLatch(1);egl.surfaceRelease=complete->{entered.countDown();await(allow);complete.run();};
    f.producer.callback.onSurfaceCleanup();f.producer.callback.onSurfaceAvailable();
    until(()->entered.getCount()==0,true);VideoFrame paused=new VideoFrame(640,360);track.emit(paused);
    check(paused.retains.get()==0&&egl.frames.get()==0,"availability rearmed before surface physical cleanup");
    allow.countDown();until(()->egl.surfaceReleases.get()==1,true);
    // Completion continuations post to main; exercise normal scheduler, not arbitrary sleeps.
    f.owner.submit("host.marker",()->{},true).get(3,TimeUnit.SECONDS);
    for(int i=0;i<30;i++){Handler.drainMainLoop();Thread.sleep(1);}
    VideoFrame resumed=new VideoFrame(640,360);track.emit(resumed);until(()->egl.frames.get()==1,true);
    f.dispose();check(paused.refs.get()==0&&resumed.refs.get()==0,"surface transition lost frame ownership");
  }

  static void nativeDetachFailureRetainsEpochAndNeverRetries()throws Exception{
    Fixture f=new Fixture();VideoTrack track=new VideoTrack("failed-native");EglRenderer egl=f.attach(track);track.onRemove=()->{throw new IllegalStateException("fake native remove failure");};
    Reply clear=f.set(null);clear.done();check(clear.code!=null,"native failure became success");
    Reply dispose=new Reply();f.renderer.Dispose(dispose,()->{});dispose.done();
    check(dispose.code!=null&&egl.releases.get()==0&&f.producer.releases.get()==0,"failed unknown sink released output");
    check(f.owner.detachMatching(track).isCompletedExceptionally()&&track.removes.get()==1,"failed canonical detach retried JNI or lost key");
  }

  static void rawMailboxFailureCompletesPublicBarrierOffCallback()throws Exception{
    Handler main=new Handler(android.os.Looper.getMainLooper());AtomicReference<Throwable> observed=new AtomicReference<>();
    NativeFrameHandoff handoff=new NativeFrameHandoff(frame->{throw new AssertionError("must not deliver failed retain");},error->observed.compareAndSet(null,error),main::post);
    check(handoff.resume(handoff.pause()),"initial handoff not active");Handler.drainMainLoop();
    CountDownLatch entered=new CountDownLatch(1),allow=new CountDownLatch(1);RuntimeException original=new RuntimeException("fake retain failure");
    VideoFrame frame=new VideoFrame(640,360);frame.beforeRetain=()->{entered.countDown();await(allow);throw original;};
    AtomicReference<Throwable> escaped=new AtomicReference<>();Thread source=new Thread(()->{try{handoff.onFrame(frame);}catch(Throwable e){escaped.set(e);}finally{frame.release();}},"FakeNativeSinkCallback");source.setDaemon(true);source.start();await(entered);
    CompletableFuture<Void> drain=handoff.seal();AtomicReference<Thread> continuation=new AtomicReference<>();
    drain.whenComplete((unused,error)->continuation.set(Thread.currentThread()));
    allow.countDown();source.join(3000);check(!source.isAlive()&&escaped.get()==null,"borrowed callback escaped error or stalled");
    check(observed.get()==original&&!drain.isDone()&&continuation.get()==null,"source ran public physical continuations");
    Handler.drainMainLoop();check(drain.isCompletedExceptionally()&&continuation.get()==MAIN,"bridged failure not observed on main");
    try{drain.get();throw new AssertionError("failed retain became success");}catch(java.util.concurrent.ExecutionException e){check(NativeMediaLifecycle.unwrap(e)==original,"error identity lost");}
  }

  static void actualEpochSourceFaultDefersOwnerAndLogButBlocksAdmission()throws Exception{
    Fixture f=new Fixture();VideoTrack track=new VideoTrack("retain-fault");EglRenderer egl=f.attach(track);
    android.util.Log.warningThreads.clear();RuntimeException original=new RuntimeException("fake retain failure");
    VideoFrame frame=new VideoFrame(640,360);frame.beforeRetain=()->{throw original;};track.emit(frame);
    check(android.util.Log.warningThreads.isEmpty(),"source callback synchronously logged failure");
    VideoTrack forbidden=new VideoTrack("new-after-fault");Reply next=f.set(forbidden);next.done();
    check(next.code!=null&&forbidden.adds.get()==0,"unprocessed source fault admitted new generation");
    Handler.drainMainLoop();check(!android.util.Log.warningThreads.isEmpty(),"observed source failure stayed silent");
    Reply disposed=new Reply();f.renderer.Dispose(disposed,()->{});disposed.done();
    check(disposed.code!=null&&track.removes.get()==1&&egl.releases.get()==0&&f.producer.releases.get()==0,"unknown retain failure released ownership or retried JNI");
  }

  static void workerEntryRejectedBeforeEpochMutation()throws Exception{
    Fixture f=new Fixture();VideoTrack track=new VideoTrack("main-only");
    int before=EglRenderer.instances.size();AtomicReference<Throwable> rejected=new AtomicReference<>();
    Thread wrong=new Thread(()->{try{f.renderer.setTrack(track,"stream","local",new Reply());}catch(Throwable error){rejected.set(error);}},"WrongRendererCaller");
    wrong.start();wrong.join(3000);check(!wrong.isAlive()&&rejected.get()!=null,"worker source mutation was admitted");
    Handler.drainMainLoop();check(track.adds.get()==0&&EglRenderer.instances.size()==before,"off-main source allocated/attached epoch");
    f.attach(track);rejected.set(null);
    Thread dispose=new Thread(()->{try{f.renderer.Dispose(null,()->{});}catch(Throwable error){rejected.set(error);}},"WrongRendererDispose");
    dispose.start();dispose.join(3000);check(!dispose.isAlive()&&rejected.get()!=null,"worker dispose mutation was admitted");
    check(f.producer.releases.get()==0,"off-main dispose crossed ownership");f.dispose();
  }

  public static void main(String[] args)throws Exception{
    workerEntryRejectedBeforeEpochMutation();System.out.println("PASS workerEntryRejectedBeforeEpochMutation");
    sourceCallbackUsesProxyNotEglOrProducer();System.out.println("PASS sourceCallbackUsesProxyNotEglOrProducer");
    actualLeaseWaitsEnteredDeliveryAndRealEglReturn();System.out.println("PASS actualLeaseWaitsEnteredDeliveryAndRealEglReturn");
    sourceReplacementUsesFreshEglAfterOldRetirement();System.out.println("PASS sourceReplacementUsesFreshEglAfterOldRetirement");
    queuedPrepareSupersededBeforeMainAllocatesNothing();System.out.println("PASS queuedPrepareSupersededBeforeMainAllocatesNothing");
    preparedThenSupersededRetiresUnattachedEpoch();System.out.println("PASS preparedThenSupersededRetiresUnattachedEpoch");
    finalRawEventDeliveryChecksSourceAndSubscription();System.out.println("PASS finalRawEventDeliveryChecksSourceAndSubscription");
    disposeAndOwnerCloseRetireQueuedOrPreparedEpoch();System.out.println("PASS disposeAndOwnerCloseRetireQueuedOrPreparedEpoch");
    sharedNativeTrackKeepsBothProxyLeasesThroughEgl();System.out.println("PASS sharedNativeTrackKeepsBothProxyLeasesThroughEgl");
    earlyAvailableCannotCrossPendingSurfaceCleanup();System.out.println("PASS earlyAvailableCannotCrossPendingSurfaceCleanup");
    nativeDetachFailureRetainsEpochAndNeverRetries();System.out.println("PASS nativeDetachFailureRetainsEpochAndNeverRetries");
    actualEglFailureIsRetainedWithoutSecondRelease();System.out.println("PASS actualEglFailureIsRetainedWithoutSecondRelease");
    rawMailboxFailureCompletesPublicBarrierOffCallback();System.out.println("PASS rawMailboxFailureCompletesPublicBarrierOffCallback");
    actualEpochSourceFaultDefersOwnerAndLogButBlocksAdmission();System.out.println("PASS actualEpochSourceFaultDefersOwnerAndLogButBlocksAdmission");
  }
}
