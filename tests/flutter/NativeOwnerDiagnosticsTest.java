package com.cloudwebrtc.webrtc;

import java.util.*;
import java.util.concurrent.*;

public final class NativeOwnerDiagnosticsTest {
  static void check(boolean ok,String text) {if(!ok)throw new AssertionError(text);}
  static void orderedPhysicalStages() {
    List<Runnable> queue=new ArrayList<>(); List<String> events=new ArrayList<>();
    NativeMediaLifecycle.Owner owner=new NativeMediaLifecycle.Owner(queue::add,Runnable::run,8,events::add);
    CompletableFuture<Void> physical=owner.submit("peer.close",()->events.add("FAKE_JNI"),true);
    check(!physical.isDone() && events.size()==1 && events.get(0).contains("phase=queued"),"queue diagnostic advanced physical work");
    queue.remove(0).run(); physical.join(); String all=String.join("\n",events);
    check(all.indexOf("phase=queued")<all.indexOf("phase=workerStart") && all.indexOf("phase=workerStart")<all.indexOf("phase=ownerAcquired") && all.indexOf("phase=ownerAcquired")<all.indexOf("FAKE_JNI") && all.indexOf("FAKE_JNI")<all.indexOf("phase=actionEnd"),"owner diagnostics did not preserve queue/acquire/physical order");
  }
  static void logFailureCannotChangeOwnership() {
    NativeMediaLifecycle.Owner owner=new NativeMediaLifecycle.Owner(Runnable::run,Runnable::run,8,value->{throw new IllegalStateException("synthetic logger rejection");});
    owner.submit("peer.dispose",()->{},true).join();
  }
  static void nativeErrorPreservedWithoutMessageLeak() {
    List<String> events=new ArrayList<>(); RuntimeException secret=new IllegalArgumentException("synthetic-private-message");
    NativeMediaLifecycle.Owner owner=new NativeMediaLifecycle.Owner(Runnable::run,Runnable::run,8,events::add);
    try {owner.submit("peer.close",()->{throw secret;},true).join();throw new AssertionError("native failure lost");} catch(CompletionException error){check(error.getCause()==secret,"diagnostics changed native failure");}
    check(owner.isQuarantined() && String.join("\n",events).contains("failure=IllegalArgumentException") && !String.join("\n",events).contains("synthetic-private-message"),"diagnostics leaked native message or lost quarantine");
  }
  public static void main(String[] args) {
    orderedPhysicalStages();logFailureCannotChangeOwnership();nativeErrorPreservedWithoutMessageLeak();
    System.out.println("PASS 3 production native-owner timing metadata cases; fake JNI/executor boundary only");
  }
}
