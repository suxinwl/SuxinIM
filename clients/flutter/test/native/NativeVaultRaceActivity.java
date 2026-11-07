package com.puty.test.vaulttransitions;

import android.app.Activity;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
import android.os.SystemClock;
import android.util.Log;
import java.io.File;
import java.lang.reflect.Field;
import java.lang.reflect.Method;
import java.nio.ByteBuffer;
import java.util.Arrays;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicInteger;
import java.util.concurrent.atomic.AtomicLong;
import java.util.concurrent.atomic.AtomicReference;
import io.flutter.plugin.common.BinaryMessenger;
import io.flutter.plugin.common.MethodChannel;
import io.flutter.plugin.common.StandardMethodCodec;
import io.openim.flutter_openim_sdk.NativeVaultBroker;
import io.openim.flutter_openim_sdk.EncryptedScratchFiles;
import open_im_sdk.Open_im_sdk;

/** Separate no-INTERNET package; no synthetic authority enters the production
 * application. Exercise the actual Broker teardown, JNI Go Store and callbacks. */
public final class NativeVaultRaceActivity extends Activity {
 static final String TAG="PutyVaultRace";
 static void check(boolean value,String code){if(!value)throw new AssertionError(code);}
 static void await(CountDownLatch latch)throws Exception{check(latch.await(5,TimeUnit.SECONDS),"LATCH_TIMEOUT");}
 static Field field(String name)throws Exception{Field f=NativeVaultBroker.class.getDeclaredField(name);f.setAccessible(true);return f;}
 static AtomicLong generation(NativeVaultBroker broker)throws Exception{return (AtomicLong)field("generation").get(broker);}
 static void callLock(NativeVaultBroker broker,long expected)throws Exception{Method m=NativeVaultBroker.class.getDeclaredMethod("lockGeneration",long.class);m.setAccessible(true);m.invoke(broker,expected);}
 @Override public void onCreate(Bundle state){super.onCreate(state);new Thread(()->{try{race(false,false);race(true,false);race(false,true);Log.i(TAG,"COMPLETE 3/3 actual Broker/JNI transition races");}catch(Throwable failed){Log.i(TAG,"FAIL "+failed.getClass().getSimpleName());}},"offline-vault-races").start();}
 void publish(NativeVaultBroker broker,Object mutex,String account,File directory,byte[] master)throws Exception{
  synchronized(mutex){generation(broker).incrementAndGet();check(Open_im_sdk.configureNativeLocalStore(account,directory.getAbsolutePath(),master,System.currentTimeMillis()+8000),"CONFIGURE_REJECTED");field("activeAccount").set(broker,account);field("leaseDeadline").setLong(broker,SystemClock.elapsedRealtime()+8000);}
 }
 void race(boolean manual,boolean notification)throws Exception{
  NativeVaultBroker broker=new NativeVaultBroker(this);String account="synthetic-race-"+manual+"-"+notification;
  File directory=new File(getFilesDir(),account);check(directory.isDirectory()||directory.mkdirs(),"DIRECTORY_FAILED");byte[] master=new byte[32];Arrays.fill(master,(byte)0x17);
  Object mutex=field("stateLock").get(broker);CountingMessenger messenger=new CountingMessenger();broker.setChannel(new MethodChannel(messenger,"puty/local_store"));
  CountDownLatch entered=new CountDownLatch(1),release=new CountDownLatch(1),configured=new CountDownLatch(1),mainHeld=new CountDownLatch(notification?1:0),mainRelease=new CountDownLatch(notification?1:0);
  AtomicReference<Throwable> failure=new AtomicReference<>();AtomicInteger replies=new AtomicInteger();
  try{
   publish(broker,mutex,account,directory,master);check(Open_im_sdk.openConfiguredNativeWorkingStore(account),"OPEN_REJECTED");
   long oldGeneration=generation(broker).get();
   EncryptedScratchFiles.registerConsumer(()->{entered.countDown();try{await(release);}catch(Exception e){failure.compareAndSet(null,e);}});
   if(notification){new Handler(Looper.getMainLooper()).post(()->{mainHeld.countDown();try{await(mainRelease);}catch(Exception e){failure.compareAndSet(null,e);}});await(mainHeld);}
   Thread old=new Thread(()->{try{if(manual)broker.handle("lock",null,new MethodChannel.Result(){public void success(Object value){replies.incrementAndGet();}public void error(String code,String message,Object details){failure.compareAndSet(null,new AssertionError("MANUAL_LOCK_FAILED"));}public void notImplemented(){failure.compareAndSet(null,new AssertionError("MANUAL_LOCK_MISSING"));}});else callLock(broker,oldGeneration);}catch(Throwable error){failure.compareAndSet(null,error);}},"old-native-lock");
   old.start();await(entered);
   Thread newer=new Thread(()->{try{publish(broker,mutex,account,directory,master);configured.countDown();}catch(Throwable error){failure.compareAndSet(null,error);}},"new-native-actor");newer.start();
   check(!configured.await(150,TimeUnit.MILLISECONDS),"NEW_ACTOR_BYPASSED_TEARDOWN");
   release.countDown();old.join(5000);newer.join(5000);check(!old.isAlive()&&!newer.isAlive(),"TRANSITION_DID_NOT_FINISH");check(failure.get()==null,"TRANSITION_FAILED");
   check(Open_im_sdk.nativeLocalStoreStatus(account).contains("\"unlocked\":true"),"OLD_LOCK_CLOSED_NEW_STORE");check(account.equals(field("activeAccount").get(broker)),"NEW_ACTOR_LOST");
   if(manual)check(replies.get()==1,"LOCK_REPLY_COUNT");
   // Even invoking the old rejection again cannot touch a newly published key.
   callLock(broker,oldGeneration);check(Open_im_sdk.nativeLocalStoreStatus(account).contains("\"unlocked\":true"),"STALE_LOCK_CLOSED_NEW_STORE");
   if(notification){mainRelease.countDown();CountDownLatch drained=new CountDownLatch(1);new Handler(Looper.getMainLooper()).post(drained::countDown);await(drained);check(messenger.notifications.get()==0,"OLD_NOTIFICATION_REACHED_NEW_ACTOR");}
   Log.i(TAG,"PASS "+(notification?"STALE_MAIN_NOTIFICATION":manual?"EXPLICIT_LOCK_SERIALIZED":"REJECTION_LOCK_SERIALIZED"));
  }finally{release.countDown();mainRelease.countDown();broker.close();Arrays.fill(master,(byte)0);}
 }
 static final class CountingMessenger implements BinaryMessenger{
  final AtomicInteger notifications=new AtomicInteger();
  @Override public void send(String channel,ByteBuffer message){send(channel,message,null);}
  @Override public void send(String channel,ByteBuffer message,BinaryReply callback){if(message!=null){ByteBuffer readable=message.duplicate();readable.flip();if("onVaultLocked".equals(StandardMethodCodec.INSTANCE.decodeMethodCall(readable).method))notifications.incrementAndGet();}if(callback!=null)callback.reply(StandardMethodCodec.INSTANCE.encodeSuccessEnvelope(null));}
  @Override public void setMessageHandler(String channel,BinaryMessageHandler handler){}
 }
}
