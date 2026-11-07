"""Whole production Broker JVM scheduler regression; no device/network/JNI.

Compile the complete Broker and native helpers against their actual Android,
Flutter and Go binding dependencies. Only temporary scheduling hooks are added
to Broker source. OS/network/JNI doubles are ahead of those jars at runtime.
This is not normal Main, real HTTP authorization, or a JNI/device acceptance.
"""
from pathlib import Path
import argparse, hashlib, json, os, subprocess, tempfile, zipfile

ROOT = Path(__file__).resolve().parents[3]
JAVA = Path('E:/AndroidDev/jdk/jdk-17.0.20.1+1/bin')
SDK = Path('E:/AndroidDev/sdk')
PACKAGE = 'io/openim/flutter_openim_sdk'


def run(command):
    result = subprocess.run([str(x) for x in command], capture_output=True, text=True)
    if result.returncode:
        print(result.stdout, end=''); print(result.stderr, end='')
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--source', type=Path)
    args = parser.parse_args()
    source = ROOT/'sdks/flutter/android/src/main/java'/PACKAGE
    broker_path = args.source or source/'NativeVaultBroker.java'
    raw = broker_path.read_bytes(); code = raw.decode('utf-8')
    gate = 'if(closed||unlocking.get()||credentials==null||!refreshing.compareAndSet(false,true))return;'
    catch = 'if(authorizationSerial.get()==serial)lockGeneration(current,true,lockReason(rejected));'
    handle_generation = 'requested=generation.get();'
    handle_catch = 'if((method.equals("unlock")&&authorizationSerial.get()==authorizationRequest)||!currentlyAuthorized())lockGeneration(method.equals("unlock")?generation.get():requested,true,lockReason(failure));'
    handle_finally = 'if(method.equals("unlock")&&authorizationSerial.get()==authorizationRequest)unlocking.set(false);'
    if any(code.count(value)!=1 for value in (gate,catch,handle_generation,handle_catch,handle_finally)):
        raise RuntimeError('Expected exact production renewal scheduling boundaries')
    instrumented = code.replace(gate, gate+'RenewSnapshotHooks.afterGate();').replace(
        catch, 'if(authorizationSerial.get()==serial){RenewSnapshotHooks.afterCatchCheck();lockGeneration(current,true,lockReason(rejected));}').replace(
        handle_generation,handle_generation+'RenewSnapshotHooks.afterHandleGeneration();').replace(
        handle_catch,'if((method.equals("unlock")&&authorizationSerial.get()==authorizationRequest)||!currentlyAuthorized()){RenewSnapshotHooks.afterHandleCatchCheck();lockGeneration(method.equals("unlock")?generation.get():requested,true,lockReason(failure));}').replace(
        handle_finally,'if(method.equals("unlock")&&authorizationSerial.get()==authorizationRequest){RenewSnapshotHooks.afterHandleFinallyCheck();unlocking.set(false);}')
    restored=instrumented.replace('RenewSnapshotHooks.afterGate();','').replace(
        'if(authorizationSerial.get()==serial){RenewSnapshotHooks.afterCatchCheck();lockGeneration(current,true,lockReason(rejected));}',catch).replace('RenewSnapshotHooks.afterHandleGeneration();','').replace(
        'if((method.equals("unlock")&&authorizationSerial.get()==authorizationRequest)||!currentlyAuthorized()){RenewSnapshotHooks.afterHandleCatchCheck();lockGeneration(method.equals("unlock")?generation.get():requested,true,lockReason(failure));}',handle_catch).replace(
        'if(method.equals("unlock")&&authorizationSerial.get()==authorizationRequest){RenewSnapshotHooks.afterHandleFinallyCheck();unlocking.set(false);}',handle_finally)
    assert restored==code
    work_parent = Path('D:/PUTY-local-build/mobile-temp'); work_parent.mkdir(parents=True,exist_ok=True)
    with tempfile.TemporaryDirectory(prefix='renew-snapshot-jvm-',dir=work_parent) as directory:
        work=Path(directory); prod=work/'production'; prod.mkdir(); doubles=work/'doubles'; doubles.mkdir()
        broker=work/'source'/PACKAGE/'NativeVaultBroker.java';broker.parent.mkdir(parents=True);broker.write_text(instrumented,encoding='utf-8')
        jar=work/'go-bindings.jar'
        with zipfile.ZipFile(ROOT/'sdks/flutter/android/libs/puty-core-sdk-3.8.3-patch15-enterprise.aar') as archive:
            jar.write_bytes(archive.read('classes.jar'))
        embedding=list((ROOT/'.cache/gradle/caches/modules-2/files-2.1/io.flutter/flutter_embedding_release').rglob('*.jar'))
        if len(embedding)!=1:raise RuntimeError('Expected one pinned Flutter embedding')
        android=SDK/'platforms/android-35/android.jar'
        if not android.is_file():android=SDK/'platforms/android-34/android.jar'
        cp=os.pathsep.join(map(str,[android,jar,*embedding]))
        generated=work/'test-source'; files=[]
        for name,body in STUBS.items():
            path=generated/name;path.parent.mkdir(parents=True,exist_ok=True);path.write_text(body,encoding='utf-8');files.append(path)
        # The hooks class is a no-authority scheduler only; compile it first.
        hook=generated/(PACKAGE+'/RenewSnapshotHooks.java')
        result=run([JAVA/'javac.exe','-encoding','UTF-8','-d',doubles,hook])
        if result.returncode:return result.returncode
        native=['NativeVaultRenewal','NativeVaultSessionState','NativeBackupPicker','EncryptedSelectedFiles','EncryptedScratchFiles','MobileLegacyQuarantine']
        result=run([JAVA/'javac.exe','-encoding','UTF-8','-classpath',os.pathsep.join([str(doubles),cp]),'-d',prod,broker,*[source/(x+'.java') for x in native]])
        if result.returncode:return result.returncode
        result=run([JAVA/'javac.exe','-encoding','UTF-8','-classpath',os.pathsep.join([str(prod),cp]),'-d',doubles,*files])
        if result.returncode:return result.returncode
        runtime=work/'runtime';runtime.mkdir()
        result=run([JAVA/'java.exe','-Djava.io.tmpdir='+str(runtime),'-classpath',os.pathsep.join([str(doubles),str(prod),cp]),'io.openim.flutter_openim_sdk.RenewSnapshotHarness'])
        print(json.dumps({'productionBrokerSHA256':hashlib.sha256(raw).hexdigest(),'wholeBrokerSourceCompiled':True,
                         'temporarySchedulerHooks':5,'realBrokerRenewHandleAndLockMethodsExecuted':True,
                         'androidNetworkAndJNITestDoubles':True,'normalMainOrDeviceAcceptance':False,
                         'productionRequests':0,'exitCode':result.returncode}))
        if result.returncode==0:print(result.stdout,end='')
        return result.returncode


STUBS = {
'android/os/SystemClock.java':'''package android.os; public final class SystemClock { public static long elapsedRealtime(){return System.nanoTime()/1000000;} }''',
'android/os/Looper.java':'''package android.os; public final class Looper { public static Looper getMainLooper(){return new Looper();} }''',
'android/os/Handler.java':'''package android.os; public final class Handler { public Handler(Looper value){} public boolean post(Runnable value){value.run();return true;} }''',
'android/content/pm/ApplicationInfo.java':'''package android.content.pm; public class ApplicationInfo { public String dataDir; }''',
'android/content/Context.java':'''package android.content; import java.io.File; public class Context { public static File root; public Context getApplicationContext(){return this;} public File getNoBackupFilesDir(){return root;} public File getCacheDir(){return root;} public File getExternalFilesDir(String type){return root;} public File getExternalCacheDir(){return root;} public android.content.pm.ApplicationInfo getApplicationInfo(){return new android.content.pm.ApplicationInfo();} }''',
'org/json/JSONObject.java':'''package org.json; import java.util.*; public final class JSONObject { public static JSONObject envelope; private final Map<String,Object> values=new HashMap<>(); public JSONObject(){} public JSONObject(String raw){values.putAll(envelope.values);} public JSONObject put(String key,Object value){values.put(key,value);return this;} public Object opt(String key){return values.get(key);} public int optInt(String key,int fallback){Object value=values.get(key);return value instanceof Number?((Number)value).intValue():fallback;} public JSONObject getJSONObject(String key){return (JSONObject)values.get(key);} }''',
'io/openim/flutter_openim_sdk/EncryptedScratchFiles.java':'''package io.openim.flutter_openim_sdk; public final class EncryptedScratchFiles { public interface Authority {String authorizedAccount();long generation();} public static void closeAll(){} }''',
'io/openim/flutter_openim_sdk/NativeBackupPicker.java':'''package io.openim.flutter_openim_sdk; public final class NativeBackupPicker { public void close(){} }''',
'open_im_sdk/Open_im_sdk.java':'''package open_im_sdk; public final class Open_im_sdk { public static int locks; public static void lockNativeLocalStore(String account){locks++;} public static boolean recoverNativeStoreRoot(String root){return true;} public static boolean configureNativeLocalStore(String account,String root,byte[] key,long deadline){return true;} }''',
'io/openim/flutter_openim_sdk/RenewSnapshotHooks.java':r'''package io.openim.flutter_openim_sdk;
import java.util.concurrent.*;
public final class RenewSnapshotHooks {
  public static String mode="none"; public static CountDownLatch entered,release;
  public static void reset(String value){mode=value;entered=new CountDownLatch(1);release=new CountDownLatch(1);}
  public static void afterGate(){if(mode.equals("snapshot"))hold();}
  public static void afterCatchCheck(){if(mode.equals("catch"))hold();}
  public static void afterHandleGeneration(){if(mode.equals("handle"))hold();}
  public static void afterHandleCatchCheck(){if(mode.equals("handle-catch"))hold();}
  public static void afterHandleFinallyCheck(){if(mode.equals("handle-finally"))hold();}
  private static void hold(){entered.countDown();try{if(!release.await(4,TimeUnit.SECONDS))throw new AssertionError("HOOK_TIMEOUT");}catch(InterruptedException e){throw new AssertionError("INTERRUPTED");}}
}''',
'io/openim/flutter_openim_sdk/RenewSnapshotHarness.java':r'''package io.openim.flutter_openim_sdk;
import java.io.*;import java.net.*;import java.security.*;import java.security.cert.Certificate;import java.lang.reflect.*;
import java.util.*;import java.util.concurrent.*;import java.util.concurrent.atomic.*;import javax.net.ssl.*;import javax.crypto.*;import javax.crypto.spec.*;import java.nio.file.*;import org.json.JSONObject;
public final class RenewSnapshotHarness {
  static final String ACCOUNT="emp_00000000000000000000000000000017";
  static CountDownLatch networkEntered,networkRelease;static boolean rejected;static int rejectionStatus=403;
  static final byte[] WRAPKEY=new byte[32];
  public static final class TestKeyStore extends KeyStoreSpi {
    public Key engineGetKey(String a,char[] p){return new SecretKeySpec(WRAPKEY,"AES");}public Certificate[] engineGetCertificateChain(String a){return null;}public Certificate engineGetCertificate(String a){return null;}public Date engineGetCreationDate(String a){return new Date(0);}public void engineSetKeyEntry(String a,Key k,char[] p,Certificate[] c){throw new UnsupportedOperationException();}public void engineSetKeyEntry(String a,byte[] k,Certificate[] c){throw new UnsupportedOperationException();}public void engineSetCertificateEntry(String a,Certificate c){throw new UnsupportedOperationException();}public void engineDeleteEntry(String a){throw new UnsupportedOperationException();}public Enumeration<String> engineAliases(){return Collections.enumeration(Collections.singleton("puty.local.wrap.v1"));}public boolean engineContainsAlias(String a){return true;}public int engineSize(){return 1;}public boolean engineIsKeyEntry(String a){return true;}public boolean engineIsCertificateEntry(String a){return false;}public String engineGetCertificateAlias(Certificate c){return null;}public void engineStore(OutputStream s,char[] p){}public void engineLoad(InputStream s,char[] p){}
  }
  static void check(boolean value,String code){if(!value)throw new AssertionError(code);}
  static void await(CountDownLatch value)throws Exception{check(value.await(4,TimeUnit.SECONDS),"LATCH_TIMEOUT");}
  static Field field(String name)throws Exception{Field value=NativeVaultBroker.class.getDeclaredField(name);value.setAccessible(true);return value;}
  static void set(NativeVaultBroker target,String name,Object value)throws Exception{field(name).set(target,value);}
  static AtomicLong number(NativeVaultBroker target,String name)throws Exception{return (AtomicLong)field(name).get(target);}
  static NativeVaultBroker fixture()throws Exception{
    Field f=sun.misc.Unsafe.class.getDeclaredField("theUnsafe");f.setAccessible(true);NativeVaultBroker target=(NativeVaultBroker)((sun.misc.Unsafe)f.get(null)).allocateInstance(NativeVaultBroker.class);
    set(target,"context",new android.content.Context());set(target,"stateLock",new Object());set(target,"generation",new AtomicLong(1));set(target,"authorizationSerial",new AtomicLong(1));
    set(target,"unlocking",new AtomicBoolean());set(target,"refreshing",new AtomicBoolean());set(target,"leaseExecutor",Executors.newScheduledThreadPool(2));set(target,"restores",new HashMap<>());set(target,"backupPicker",new NativeBackupPicker());
    set(target,"activeAccount",ACCOUNT);Map<String,Object> credentials=new HashMap<>();credentials.put("origin","https://im.szputy.com:9885");credentials.put("userID",ACCOUNT);credentials.put("imToken","SYNTHETIC_IM_PROOF");credentials.put("chatToken","SYNTHETIC_CHAT_PROOF");set(target,"activeCredentials",credentials);set(target,"sessionID","SYNTHETIC_SID");set(target,"epoch",1L);set(target,"leaseDeadline",android.os.SystemClock.elapsedRealtime()+8000);
    JSONObject policy=new JSONObject().put("zone","internet").put("logicalClient","android").put("login",true).put("preview",true).put("save",true).put("securityVersion",1L).put("policyRevision",0L).put("networkRevision",0L).put("currentIP","192.0.2.1").put("sources",new JSONObject().put("login","synthetic").put("save","synthetic"));
    JSONObject data=new JSONObject().put("userID",ACCOUNT).put("sessionID","SYNTHETIC_SID").put("epoch",1L).put("platform",2).put("logicalClient","android").put("expiresAt",System.currentTimeMillis()/1000+3600).put("loginAllowed",true).put("saveAllowed",true).put("effectivePolicy",policy);
    JSONObject.envelope=new JSONObject().put("errCode",0).put("data",data);networkEntered=new CountDownLatch(1);networkRelease=new CountDownLatch(1);rejected=false;return target;
  }
  static void prepareWrappedMaster()throws Exception{
    byte[] digest=MessageDigest.getInstance("SHA-256").digest(ACCOUNT.getBytes("UTF-8"));StringBuilder name=new StringBuilder();for(byte value:digest)name.append(String.format("%02x",value));
    File directory=new File(android.content.Context.root,"puty-local/accounts/"+name);check(directory.isDirectory()||directory.mkdirs(),"MASTER_DIRECTORY");byte[] iv=new byte[12];Arrays.fill(iv,(byte)0x17);Cipher cipher=Cipher.getInstance("AES/GCM/NoPadding");cipher.init(Cipher.ENCRYPT_MODE,new SecretKeySpec(WRAPKEY,"AES"),new GCMParameterSpec(128,iv));cipher.updateAAD(("master\u0000"+ACCOUNT).getBytes("UTF-8"));ByteArrayOutputStream bytes=new ByteArrayOutputStream();bytes.write("PUTYOS1\n".getBytes("UTF-8"));bytes.write(iv);bytes.write(cipher.doFinal(new byte[32]));Files.write(new File(directory,"master.wrap").toPath(),bytes.toByteArray());
  }
  static void renew(NativeVaultBroker target){try{Method m=NativeVaultBroker.class.getDeclaredMethod("renewLease");m.setAccessible(true);m.invoke(target);}catch(Exception e){throw new AssertionError("ACTUAL_BROKER_INVOCATION_FAILED",e);}}
  static void shutdown(NativeVaultBroker target)throws Exception{((ScheduledExecutorService)field("leaseExecutor").get(target)).shutdownNow();}
  static void race(String kind)throws Exception{
    NativeVaultBroker target=fixture();RenewSnapshotHooks.reset(kind);rejected=kind.equals("catch");if(rejected)networkRelease.countDown();
    CountDownLatch explicitEntered=new CountDownLatch(1),explicitDone=new CountDownLatch(1),publish=new CountDownLatch(1);AtomicReference<Throwable> error=new AtomicReference<>();Object mutex=field("stateLock").get(target);
    Thread old=new Thread(()->{try{renew(target);}catch(Throwable t){error.set(t);}},"old-renew");old.start();await(RenewSnapshotHooks.entered);
    Thread newer=new Thread(()->{try{synchronized(mutex){((AtomicBoolean)field("unlocking").get(target)).set(true);number(target,"authorizationSerial").incrementAndGet();}explicitEntered.countDown();await(publish);synchronized(mutex){set(target,"activeAccount",ACCOUNT);set(target,"leaseDeadline",android.os.SystemClock.elapsedRealtime()+8000);}}catch(Throwable t){error.compareAndSet(null,t);}finally{explicitDone.countDown();}},"new-explicit-unlock");newer.start();
    long until=System.nanoTime()+TimeUnit.SECONDS.toNanos(3);while(explicitEntered.getCount()!=0&&newer.getState()!=Thread.State.BLOCKED&&System.nanoTime()<until)Thread.yield();
    check(explicitEntered.getCount()==0||newer.getState()==Thread.State.BLOCKED,"EXPLICIT_DID_NOT_REACH_TRANSITION");boolean blocked=newer.getState()==Thread.State.BLOCKED;
    if(kind.equals("catch")&&!blocked){publish.countDown();await(explicitDone);}
    RenewSnapshotHooks.release.countDown();await(explicitEntered);
    if(kind.equals("snapshot")){if(!blocked)await(networkEntered);publish.countDown();await(explicitDone);networkRelease.countDown();}else{publish.countDown();await(explicitDone);}
    old.join(4000);newer.join(4000);check(!old.isAlive()&&!newer.isAlive(),"THREAD_DID_NOT_FINISH");shutdown(target);check(error.get()==null,"HARNESS_THREAD_FAILED");
    check(ACCOUNT.equals(field("activeAccount").get(target)),kind.equals("snapshot")?"SNAPSHOT_OLD_RENEW_LOCKED_NEW_ACTOR":"CATCH_OLD_RENEW_LOCKED_NEW_ACTOR");
    System.out.println("PASS "+kind+"_new_actor_survives");
  }
  static void trueRejection(int status)throws Exception{NativeVaultBroker target=fixture();RenewSnapshotHooks.reset("none");rejected=true;rejectionStatus=status;networkRelease.countDown();renew(target);shutdown(target);check(field("activeAccount").get(target)==null,"CURRENT_REAL_REJECTION_DID_NOT_LOCK");System.out.println("PASS current_"+status+"_still_locks");}
  static final class Queued extends AbstractExecutorService {
    Runnable operation;public void execute(Runnable value){operation=value;}public void shutdown(){}public java.util.List<Runnable> shutdownNow(){return java.util.Collections.emptyList();}public boolean isShutdown(){return false;}public boolean isTerminated(){return false;}public boolean awaitTermination(long timeout,TimeUnit unit){return true;}
  }
  static void handleSnapshot()throws Exception{
    NativeVaultBroker target=fixture();RenewSnapshotHooks.reset("handle");Queued queue=new Queued();set(target,"authorizationExecutor",queue);Object mutex=field("stateLock").get(target);AtomicReference<Throwable> error=new AtomicReference<>();CountDownLatch published=new CountDownLatch(1);
    Thread old=new Thread(()->{try{target.handle("unlock",field("activeCredentials").get(target),new io.flutter.plugin.common.MethodChannel.Result(){public void success(Object value){}public void error(String code,String message,Object details){}public void notImplemented(){}});}catch(Throwable t){error.set(t);}},"old-handle-unlock");old.start();await(RenewSnapshotHooks.entered);
    Thread newer=new Thread(()->{try{synchronized(mutex){number(target,"generation").incrementAndGet();number(target,"authorizationSerial").incrementAndGet();set(target,"activeAccount",ACCOUNT);set(target,"leaseDeadline",android.os.SystemClock.elapsedRealtime()+8000);}}catch(Throwable t){error.compareAndSet(null,t);}finally{published.countDown();}},"new-handle-actor");newer.start();
    long until=System.nanoTime()+TimeUnit.SECONDS.toNanos(3);while(published.getCount()!=0&&newer.getState()!=Thread.State.BLOCKED&&System.nanoTime()<until)Thread.yield();check(published.getCount()==0||newer.getState()==Thread.State.BLOCKED,"HANDLE_TRANSITION_NOT_REACHED");
    RenewSnapshotHooks.release.countDown();old.join(4000);await(published);check(error.get()==null&&queue.operation!=null,"HANDLE_FIXTURE_FAILED");queue.operation.run();shutdown(target);check(ACCOUNT.equals(field("activeAccount").get(target)),"HANDLE_OLD_GENERATION_NEW_SERIAL_LOCKED_NEW_ACTOR");System.out.println("PASS handle_generation_serial_captured_atomically");
  }
  static void handleTail(String kind)throws Exception{
    NativeVaultBroker target=fixture();RenewSnapshotHooks.reset(kind);Queued queue=new Queued();set(target,"authorizationExecutor",queue);Object mutex=field("stateLock").get(target);AtomicReference<Throwable> error=new AtomicReference<>();CountDownLatch published=new CountDownLatch(1);rejected=kind.equals("handle-catch");networkRelease.countDown();prepareWrappedMaster();
    target.handle("unlock",field("activeCredentials").get(target),new io.flutter.plugin.common.MethodChannel.Result(){public void success(Object value){}public void error(String code,String message,Object details){}public void notImplemented(){}});
    Thread old=new Thread(()->{try{queue.operation.run();}catch(Throwable t){error.set(t);}},"old-unlock-result");old.start();await(RenewSnapshotHooks.entered);
    Thread newer=new Thread(()->{try{synchronized(mutex){number(target,"authorizationSerial").incrementAndGet();((AtomicBoolean)field("unlocking").get(target)).set(true);set(target,"activeAccount",ACCOUNT);set(target,"leaseDeadline",android.os.SystemClock.elapsedRealtime()+8000);}}catch(Throwable t){error.compareAndSet(null,t);}finally{published.countDown();}},"new-unlock-started");newer.start();
    long until=System.nanoTime()+TimeUnit.SECONDS.toNanos(3);while(published.getCount()!=0&&newer.getState()!=Thread.State.BLOCKED&&System.nanoTime()<until)Thread.yield();check(published.getCount()==0||newer.getState()==Thread.State.BLOCKED,"TAIL_TRANSITION_NOT_REACHED");
    RenewSnapshotHooks.release.countDown();old.join(4000);await(published);shutdown(target);check(error.get()==null&&!old.isAlive(),"TAIL_FIXTURE_FAILED");
    if(kind.equals("handle-catch"))check(ACCOUNT.equals(field("activeAccount").get(target)),"OLD_UNLOCK_CATCH_LOCKED_NEW_ACTOR");else check(((AtomicBoolean)field("unlocking").get(target)).get(),"OLD_UNLOCK_FINALLY_CLEARED_NEW_UNLOCKING");System.out.println("PASS "+kind+"_new_unlock_survives");
  }
  public static void main(String[] args)throws Exception{
    android.content.Context.root=new File(System.getProperty("java.io.tmpdir"));
    Security.addProvider(new Provider("AndroidKeyStore","1.0","Synthetic JVM OS wrapping double"){{put("KeyStore.AndroidKeyStore",TestKeyStore.class.getName());}});
    URL.setURLStreamHandlerFactory(protocol->protocol.equals("https")?new URLStreamHandler(){protected URLConnection openConnection(URL url){return new HttpsURLConnection(url){public void disconnect(){}public boolean usingProxy(){return false;}public void connect(){}public String getCipherSuite(){return "SYNTHETIC";}public Certificate[] getLocalCertificates(){return null;}public Certificate[] getServerCertificates(){return null;}public int getResponseCode()throws IOException{networkEntered.countDown();try{await(networkRelease);}catch(Exception e){throw new IOException("SYNTHETIC_LATCH",e);}return rejected?rejectionStatus:200;}public InputStream getInputStream(){return new ByteArrayInputStream("{}".getBytes());}};}}:null);
    int failures=0;for(String kind:new String[]{"snapshot","catch","handle","handle-catch","handle-finally","401","403"})try{if(kind.equals("401")||kind.equals("403"))trueRejection(Integer.parseInt(kind));else if(kind.equals("handle"))handleSnapshot();else if(kind.startsWith("handle-"))handleTail(kind);else race(kind);}catch(AssertionError failure){failures++;System.out.println("FAIL "+failure.getMessage());}
    if(failures!=0)System.exit(1);System.out.println("COMPLETE 7/7 whole production Broker scheduler cases");
  }
}''',
}

if __name__=='__main__':raise SystemExit(main())
