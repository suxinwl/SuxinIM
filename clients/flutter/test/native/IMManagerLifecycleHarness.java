package io.openim.flutter_openim_sdk.manager;

import io.flutter.plugin.common.MethodCall;
import io.flutter.plugin.common.MethodChannel;
import io.openim.flutter_openim_sdk.FlutterOpenimSdkPlugin;
import java.util.Collections;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicInteger;
import open_im_sdk.Open_im_sdk;

/** Runs the production IMManager with only its Go/Flutter platform boundary
 * replaced. No credentials, application state, devices or network are used. */
public final class IMManagerLifecycleHarness {
  private static final IMManager manager = new IMManager();
  private static int passed, failed;
  private static final MethodCall call = new MethodCall(Collections.singletonMap("operationID", "synthetic-lifecycle"));

  private static final class Reply implements MethodChannel.Result {
    final CountDownLatch done = new CountDownLatch(1);
    final AtomicInteger count = new AtomicInteger();
    Object value; String code, message;
    public void success(Object v) { value=v; count.incrementAndGet(); done.countDown(); }
    public void error(String c, String m, Object details) { code=c; message=m; count.incrementAndGet(); done.countDown(); }
    public void notImplemented() { error("NOT_IMPLEMENTED", "unexpected", null); }
    void await() throws Exception {
      require(done.await(500, TimeUnit.MILLISECONDS), "native operation never replied");
      require(count.get()==1, "native operation replied more than once");
    }
  }

  private static void require(boolean value, String message) { if(!value) throw new AssertionError(message); }
  private static void run(String name, Checked action) {
    try { action.run(); passed++; System.out.println("PASS " + name); }
    catch(Throwable failure) { failed++; System.out.println("FAIL " + name + ": " + failure.getMessage()); }
  }
  private interface Checked { void run() throws Exception; }
  private static void reset() {
    FlutterOpenimSdkPlugin.isInitialized=false;
    Open_im_sdk.failure=null; Open_im_sdk.initValue=true;
    Open_im_sdk.uninitEntered=null; Open_im_sdk.uninitRelease=null;
  }

  public static void main(String[] args) {
    run("initialization false replies without authorizing initialized state", () -> {
      reset(); Open_im_sdk.initValue=false; Reply r=new Reply(); manager.initSDK(call,r); r.await();
      require(Boolean.FALSE.equals(r.value) && !FlutterOpenimSdkPlugin.isInitialized,"false init changed state");
    });
    run("initialization succeeds and duplicate init is acknowledged once", () -> {
      reset(); int calls=Open_im_sdk.initCalls.get(); Reply a=new Reply(); manager.initSDK(call,a); a.await();
      Reply b=new Reply(); manager.initSDK(call,b); b.await();
      require(Boolean.TRUE.equals(a.value) && Boolean.TRUE.equals(b.value) && FlutterOpenimSdkPlugin.isInitialized,"init state mismatch");
      require(Open_im_sdk.initCalls.get()==calls+1,"duplicate native initialization");
    });
    run("deinitialization acknowledges after native teardown", () -> {
      reset(); FlutterOpenimSdkPlugin.isInitialized=true; Reply r=new Reply(); manager.unInitSDK(call,r); r.await();
      require(Boolean.TRUE.equals(r.value) && !FlutterOpenimSdkPlugin.isInitialized,"teardown did not finish before reply");
    });
    for(String mode:new String[]{"initException","initLinkage"}) run(mode + " returns a safe platform error", () -> {
      reset(); Open_im_sdk.failure=mode; Reply r=new Reply(); manager.initSDK(call,r); r.await();
      require("SDK_INITIALIZATION_FAILED".equals(r.code),"missing init error");
      require(!FlutterOpenimSdkPlugin.isInitialized && r.value==null,"failed init became successful");
      require(r.message!=null && !r.message.contains("synthetic-secret"),"native exception detail leaked");
    });
    for(String mode:new String[]{"uninitException","uninitLinkage"}) run(mode + " preserves prior state and permits a retry", () -> {
      reset(); FlutterOpenimSdkPlugin.isInitialized=true; Open_im_sdk.failure=mode;
      Reply r=new Reply(); manager.unInitSDK(call,r); r.await();
      require("SDK_DEINITIALIZATION_FAILED".equals(r.code),"missing teardown error");
      require(FlutterOpenimSdkPlugin.isInitialized && r.value==null,"failed teardown claimed success");
      require(r.message!=null && !r.message.contains("synthetic-secret"),"native exception detail leaked");
      Open_im_sdk.failure=null; Reply retry=new Reply(); manager.unInitSDK(call,retry); retry.await();
      require(Boolean.TRUE.equals(retry.value) && !FlutterOpenimSdkPlugin.isInitialized,"teardown retry was blocked");
    });
    run("queued init waits for teardown and both operations complete", () -> {
      reset(); FlutterOpenimSdkPlugin.isInitialized=true;
      Open_im_sdk.uninitEntered=new CountDownLatch(1); Open_im_sdk.uninitRelease=new CountDownLatch(1);
      Reply a=new Reply(); manager.unInitSDK(call,a);
      require(Open_im_sdk.uninitEntered.await(500,TimeUnit.MILLISECONDS),"teardown did not begin");
      Reply b=new Reply(); manager.initSDK(call,b);
      require(!b.done.await(30,TimeUnit.MILLISECONDS),"native init overtook teardown");
      Open_im_sdk.uninitRelease.countDown(); a.await(); b.await();
      require(Boolean.TRUE.equals(a.value) && Boolean.TRUE.equals(b.value) && FlutterOpenimSdkPlugin.isInitialized,"queued lifecycle did not finish");
    });
    System.out.println("{\"passed\":"+passed+",\"failed\":"+failed+",\"syntheticOnly\":true}");
    System.exit(failed==0 ? 0 : 1); // Production owns a process-wide executor.
  }
}
