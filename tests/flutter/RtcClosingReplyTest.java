package com.cloudwebrtc.webrtc;

import java.util.concurrent.*;
import java.util.concurrent.atomic.*;

/** Production gate reply tests, fake JNI callback boundaries only. */
public final class RtcClosingReplyTest {
  static void check(boolean ok, String text) { if (!ok) throw new AssertionError(text); }
  static final class Reply implements RtcOperationGate.Response {
    final AtomicInteger count = new AtomicInteger(); volatile String code;
    public void success(Object value) { code = "SUCCESS"; count.incrementAndGet(); }
    public void error(String value, String message) { code = value; count.incrementAndGet(); }
  }
  static RtcOperationGate.Response start(RtcOperationGate gate, Reply reply) throws Exception {
    AtomicReference<RtcOperationGate.Response> callback = new AtomicReference<>(); CountDownLatch entered = new CountDownLatch(1);
    gate.stats(value -> { callback.set(value); entered.countDown(); }, reply);
    check(entered.await(2, TimeUnit.SECONDS), "stats fixture did not enter"); return callback.get();
  }
  static void closesPendingReplyAndIgnoresLateSuccess() throws Exception {
    RtcOperationGate gate = new RtcOperationGate(120,500); Reply reply = new Reply(); RtcOperationGate.Response callback = start(gate,reply);
    gate.beginClosing(); check(reply.count.get()==1 && "RTC_STATS_CLOSED".equals(reply.code), "closing did not settle pending MethodChannel reply");
    callback.success(null); Thread.sleep(220); check(reply.count.get()==1, "late success/deadline replied twice");
  }
  static void closesPendingReplyAndIgnoresLateError() throws Exception {
    RtcOperationGate gate = new RtcOperationGate(120,500); Reply reply = new Reply(); RtcOperationGate.Response callback = start(gate,reply);
    gate.beginClosing(); callback.error("SYNTHETIC_NATIVE_ERROR", "not public"); Thread.sleep(220);
    check(reply.count.get()==1 && "RTC_STATS_CLOSED".equals(reply.code), "late native error replaced closing reply");
  }
  static void doesNotWaitForExecutingNativeInvocation() throws Exception {
    RtcOperationGate gate = new RtcOperationGate(1000,500); Reply reply = new Reply(); CountDownLatch inside = new CountDownLatch(1), release = new CountDownLatch(1);
    gate.stats(callback -> { inside.countDown(); try { release.await(); } catch (InterruptedException error) { throw new RuntimeException(error); } callback.success(null); },reply);
    check(inside.await(2,TimeUnit.SECONDS),"native fixture did not start"); long started=System.nanoTime(); gate.beginClosing();
    check(TimeUnit.NANOSECONDS.toMillis(System.nanoTime()-started)<200 && reply.count.get()==1,"beginClosing waited for nativeCall lock");
    CountDownLatch cleanup = new CountDownLatch(1); NativeMediaLifecycle.Owner owner = new NativeMediaLifecycle.Owner(gate::exclusiveNative);
    CompletableFuture<Void> physical = owner.submit(cleanup::countDown,true); check(!cleanup.await(80,TimeUnit.MILLISECONDS),"cleanup raced executing native invocation");
    release.countDown(); physical.get(2,TimeUnit.SECONDS); check(reply.count.get()==1,"native completion after physical release replied twice");
  }
  static void timeoutIsNotReplacedByClose() throws Exception {
    RtcOperationGate gate = new RtcOperationGate(60,500); Reply reply = new Reply(); RtcOperationGate.Response callback=start(gate,reply); Thread.sleep(160);
    check("RTC_STATS_TIMEOUT".equals(reply.code),"timeout fixture missing"); gate.beginClosing(); callback.success(null); check(reply.count.get()==1 && "RTC_STATS_TIMEOUT".equals(reply.code),"closing overwrote actual timeout");
  }
  static void closingRejectsNewCollection() {
    RtcOperationGate gate = new RtcOperationGate(); gate.beginClosing(); Reply reply=new Reply(); gate.stats(value -> {throw new AssertionError("new JNI after closing");},reply);
    check(reply.count.get()==1 && "RTC_STATS_UNAVAILABLE".equals(reply.code),"closing admitted new stats");
  }
  public static void main(String[] args) throws Exception {
    closesPendingReplyAndIgnoresLateSuccess(); closesPendingReplyAndIgnoresLateError(); doesNotWaitForExecutingNativeInvocation(); timeoutIsNotReplacedByClose(); closingRejectsNewCollection();
    System.out.println("PASS 5 production beginClosing reply/physical-lifetime cases; fake JNI callbacks, not Android runtime acceptance");
  }
}
