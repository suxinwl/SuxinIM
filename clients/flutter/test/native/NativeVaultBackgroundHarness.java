package io.openim.flutter_openim_sdk;

import android.os.SystemClock;
import io.flutter.plugin.common.MethodChannel;
import java.io.*;
import java.lang.reflect.*;
import java.net.*;
import java.nio.file.*;
import java.security.*;
import java.security.cert.Certificate;
import java.util.*;
import java.util.concurrent.*;
import java.util.concurrent.atomic.*;
import javax.crypto.*;
import javax.crypto.spec.*;
import javax.net.ssl.*;
import open_im_sdk.Open_im_sdk;
import org.json.JSONObject;

/** Actual Broker methods; only OS, transport, JSON parsing and JNI are doubled. */
public final class NativeVaultBackgroundHarness {
  static final String A="emp_00000000000000000000000000000120";
  static final String B="emp_00000000000000000000000000000220";
  static final String WAN="https://im.szputy.com:9885";
  static final String SOFT="ONLINE_REAUTHORIZATION_REQUIRED";
  static final String HARD="ONLINE_AUTHORIZATION_REQUIRED";
  static final byte[] WRAPKEY=new byte[32];
  static final Deque<Response> responses=new ArrayDeque<>();
  static int requests,caseCount;

  public static final class TestKeyStore extends KeyStoreSpi {
    public Key engineGetKey(String a,char[] p){return new SecretKeySpec(WRAPKEY,"AES");}
    public Certificate[] engineGetCertificateChain(String a){return null;}
    public Certificate engineGetCertificate(String a){return null;}
    public Date engineGetCreationDate(String a){return new Date(0);}
    public void engineSetKeyEntry(String a,Key k,char[] p,Certificate[] c){throw new UnsupportedOperationException();}
    public void engineSetKeyEntry(String a,byte[] k,Certificate[] c){throw new UnsupportedOperationException();}
    public void engineSetCertificateEntry(String a,Certificate c){throw new UnsupportedOperationException();}
    public void engineDeleteEntry(String a){throw new UnsupportedOperationException();}
    public Enumeration<String> engineAliases(){return Collections.enumeration(Collections.singleton("puty.local.wrap.v1"));}
    public boolean engineContainsAlias(String a){return true;}
    public int engineSize(){return 1;}
    public boolean engineIsKeyEntry(String a){return true;}
    public boolean engineIsCertificateEntry(String a){return false;}
    public String engineGetCertificateAlias(Certificate c){return null;}
    public void engineStore(OutputStream s,char[] p){}
    public void engineLoad(InputStream s,char[] p){}
  }
  static final class Direct extends AbstractExecutorService {
    public void execute(Runnable operation){operation.run();}
    public void shutdown(){}
    public List<Runnable> shutdownNow(){return Collections.emptyList();}
    public boolean isShutdown(){return false;}
    public boolean isTerminated(){return false;}
    public boolean awaitTermination(long n,TimeUnit unit){return true;}
  }
  static final class Reply implements MethodChannel.Result {
    Object value;String error;int calls;
    public void success(Object result){value=result;calls++;}
    public void error(String code,String message,Object details){error=code;calls++;}
    public void notImplemented(){throw new AssertionError("UNEXPECTED_NOT_IMPLEMENTED");}
  }
  static final class Response {
    final int status;final JSONObject body;final String fault;
    final CountDownLatch entered=new CountDownLatch(1),release=new CountDownLatch(1);
    final boolean block;
    Response(int status,JSONObject body,String fault,boolean block){this.status=status;this.body=body;this.fault=fault;this.block=block;if(!block)release.countDown();}
  }
  static final class Fixture implements AutoCloseable {
    final NativeVaultBroker broker;
    final MethodChannel channel=new MethodChannel();
    Fixture()throws Exception{
      responses.clear();requests=0;SystemClock.now=10000;Open_im_sdk.reset();android.os.Handler.reset();
      android.content.Context.root=Files.createTempDirectory(Paths.get(System.getProperty("java.io.tmpdir")),"case-").toFile();
      Field unsafeField=sun.misc.Unsafe.class.getDeclaredField("theUnsafe");unsafeField.setAccessible(true);
      broker=(NativeVaultBroker)((sun.misc.Unsafe)unsafeField.get(null)).allocateInstance(NativeVaultBroker.class);
      set(broker,"context",new android.content.Context());set(broker,"stateLock",new Object());
      set(broker,"generation",new AtomicLong(1));set(broker,"authorizationSerial",new AtomicLong(1));
      set(broker,"unlocking",new AtomicBoolean());set(broker,"refreshing",new AtomicBoolean());
      set(broker,"executor",new Direct());set(broker,"authorizationExecutor",new Direct());
      set(broker,"leaseExecutor",Executors.newScheduledThreadPool(2));set(broker,"restores",new HashMap<>());
      set(broker,"backupPicker",new NativeBackupPicker());set(broker,"activeAccount",A);
      set(broker,"activeCredentials",credentials(A));set(broker,"sessionID","SID-A");set(broker,"epoch",1L);
      set(broker,"leaseDeadline",SystemClock.now+8000);set(broker,"previewAllowed",true);set(broker,"saveAllowed",true);
      broker.setChannel(channel);prepareWrappedMaster(A);prepareWrappedMaster(B);
    }
    Reply call(String method,Object argument){Reply reply=new Reply();broker.handle(method,argument,reply);check(reply.calls==1,"RESULT_EXACTLY_ONCE");return reply;}
    void reason(String reason)throws Exception{
      check(channel.reasons.equals(Collections.singletonList(reason)),"UNEXPECTED_LOCK_REASON_"+(channel.reasons.equals(Collections.singletonList(HARD))?"OBSERVED_HARD":channel.reasons.equals(Collections.singletonList(SOFT))?"OBSERVED_SOFT":"COUNT_OR_UNKNOWN"));
      check(field("activeAccount").get(broker)==null&&field("activeCredentials").get(broker)==null,"NATIVE_CREDENTIALS_NOT_CLEARED");
      check((Long)field("leaseDeadline").get(broker)==0L,"NATIVE_DEADLINE_RETAINED");
      check(!(Boolean)field("previewAllowed").get(broker)&&!(Boolean)field("saveAllowed").get(broker),"PERMISSIONS_RETAINED_AFTER_LOCK");
      check(Open_im_sdk.locks==1,"NATIVE_STORE_NOT_LOCKED_ONCE");
    }
    public void close()throws Exception{((ScheduledExecutorService)field("leaseExecutor").get(broker)).shutdownNow();}
  }
  static Field field(String name)throws Exception{Field field=NativeVaultBroker.class.getDeclaredField(name);field.setAccessible(true);return field;}
  static void set(NativeVaultBroker broker,String name,Object value)throws Exception{field(name).set(broker,value);}
  static void check(boolean value,String code){if(!value)throw new AssertionError(code);}
  static void await(CountDownLatch latch)throws Exception{check(latch.await(4,TimeUnit.SECONDS),"LATCH_TIMEOUT");}
  static void renew(NativeVaultBroker broker)throws Exception{Method method=NativeVaultBroker.class.getDeclaredMethod("renewLease");method.setAccessible(true);method.invoke(broker);}
  static Map<String,Object> credentials(String account){Map<String,Object> value=new HashMap<>();value.put("origin",WAN);value.put("userID",account);value.put("imToken","SYNTHETIC_IM_PROOF_"+account);value.put("chatToken","SYNTHETIC_CHAT_PROOF_"+account);return value;}
  static JSONObject envelope(String account,String sid,long epoch,int code){
    JSONObject policy=new JSONObject().put("zone","internet").put("logicalClient","android").put("login",true).put("preview",true).put("save",true).put("securityVersion",epoch).put("policyRevision",0L).put("networkRevision",0L).put("currentIP","192.0.2.120").put("sources",new JSONObject().put("login","synthetic").put("save","synthetic"));
    JSONObject data=new JSONObject().put("userID",account).put("sessionID",sid).put("epoch",epoch).put("platform",2).put("logicalClient","android").put("expiresAt",System.currentTimeMillis()/1000+3600).put("loginAllowed",true).put("saveAllowed",true).put("effectivePolicy",policy);
    return new JSONObject().put("errCode",code).put("data",data);
  }
  static Response add(int status,String account,String sid,long epoch,int code,String fault,boolean block){Response response=new Response(status,envelope(account,sid,epoch,code),fault,block);responses.add(response);return response;}
  static void prepareWrappedMaster(String account)throws Exception{
    byte[] digest=MessageDigest.getInstance("SHA-256").digest(account.getBytes("UTF-8"));StringBuilder name=new StringBuilder();for(byte value:digest)name.append(String.format("%02x",value));
    File directory=new File(android.content.Context.root,"puty-local/accounts/"+name);check(directory.isDirectory()||directory.mkdirs(),"MASTER_DIRECTORY");
    byte[] iv=new byte[12];Arrays.fill(iv,(byte)0x20);Cipher cipher=Cipher.getInstance("AES/GCM/NoPadding");cipher.init(Cipher.ENCRYPT_MODE,new SecretKeySpec(WRAPKEY,"AES"),new GCMParameterSpec(128,iv));cipher.updateAAD(("master\u0000"+account).getBytes("UTF-8"));
    ByteArrayOutputStream bytes=new ByteArrayOutputStream();bytes.write("PUTYOS1\n".getBytes("UTF-8"));bytes.write(iv);bytes.write(cipher.doFinal(new byte[32]));Files.write(new File(directory,"master.wrap").toPath(),bytes.toByteArray());
  }
  static void expiredCurrent(boolean repeat)throws Exception{
    try(Fixture f=new Fixture()){
      SystemClock.now+=9001;
      Reply first=f.call("authorization/current",null);check(Boolean.FALSE.equals(first.value)&&first.error==null,"EXPIRED_CURRENT_NOT_FALSE");f.reason(SOFT);
      Reply read=f.call("source/read",Collections.singletonMap("path","synthetic-source"));check("LOCAL_VAULT_REAUTHORIZATION_REQUIRED".equals(read.error)&&Open_im_sdk.reads==0,"LOCKED_SOURCE_READ_ESCAPED");
      // The failed source/read may issue another soft generation event, but
      // cannot re-open or re-lock a native store already closed.
      check(Open_im_sdk.configurations==0&&requests==0&&Open_im_sdk.locks==1,"OFFLINE_LEASE_REOPENED");
      if(repeat){int count=f.channel.reasons.size();Reply second=f.call("authorization/current",null);check(Boolean.FALSE.equals(second.value)&&f.channel.reasons.size()==count,"DUPLICATE_CURRENT_EVENT");}
    }
  }
  static void expiredRenewal()throws Exception{
    try(Fixture f=new Fixture()){SystemClock.now+=9001;renew(f.broker);f.reason(SOFT);check(requests==0&&Open_im_sdk.configurations==0,"EXPIRED_RENEW_ATTEMPTED_IO");}
  }
  static void failure(int status,int code,String fault,String reason)throws Exception{
    try(Fixture f=new Fixture()){
      add(status,A,"SID-A",1,code,fault,false);
      if("timeout".equals(fault))add(status,A,"SID-A",1,code,fault,false);
      renew(f.broker);f.reason(reason);
      check(requests==("timeout".equals(fault)?2:1)&&Open_im_sdk.configurations==0,"FAILURE_CONFIGURED_OR_RETRY_CHANGED");
    }
  }
  static void freshAfterExpiry()throws Exception{
    try(Fixture f=new Fixture()){
      SystemClock.now+=9001;f.call("authorization/current",null);f.reason(SOFT);
      Response fresh=add(200,A,"SID-A",1,0,"",true);
      AtomicReference<Throwable> error=new AtomicReference<>();AtomicReference<Reply> result=new AtomicReference<>();
      Thread worker=new Thread(()->{try{result.set(f.call("unlock",credentials(A)));}catch(Throwable t){error.set(t);}},"fresh-online-unlock");worker.start();await(fresh.entered);
      check(Open_im_sdk.configurations==0&&field("activeAccount").get(f.broker)==null,"REOPENED_BEFORE_ONLINE_SUCCESS");
      fresh.release.countDown();worker.join(4000);check(!worker.isAlive()&&error.get()==null,"FRESH_UNLOCK_DID_NOT_FINISH");
      check(result.get().error==null&&result.get().value instanceof Map,"FRESH_UNLOCK_FAILED");
      check(requests==1&&Open_im_sdk.configurations==1&&A.equals(Open_im_sdk.configuredAccount),"FRESH_UNLOCK_NOT_CONFIGURED");
      check(Boolean.TRUE.equals(f.call("authorization/current",null).value),"FRESH_UNLOCK_NOT_AUTHORIZED");
      check((Long)field("leaseDeadline").get(f.broker)<=SystemClock.now+8000,"LEASE_EXTENDED_BEYOND_8_SECONDS");
      Reply read=f.call("source/read",Collections.singletonMap("path","synthetic-source"));check(read.error==null&&Open_im_sdk.reads==1,"AUTHORIZED_SOURCE_UNREADABLE");
    }
  }
  static void expiredUnlockMustRetryOnline()throws Exception{
    try(Fixture f=new Fixture()){
      SystemClock.now+=9001;Reply denied=f.call("unlock",credentials(A));f.reason(SOFT);
      check("LOCAL_VAULT_REAUTHORIZATION_REQUIRED".equals(denied.error)&&requests==0,"EXPIRED_EXISTING_LEASE_BYPASSED");
      add(200,A,"SID-A",1,0,"",false);Reply retry=f.call("unlock",credentials(A));
      check(retry.error==null&&requests==1&&Open_im_sdk.configurations==1,"EXPLICIT_RETRY_DID_NOT_REVALIDATE");
    }
  }
  static void explicitDenial(int status,int code)throws Exception{
    try(Fixture f=new Fixture()){
      f.call("lock",null);check(f.channel.reasons.isEmpty(),"EXPLICIT_LOCK_NOT_SILENT");
      add(status,A,"SID-A",1,code,"",false);Reply denied=f.call("unlock",credentials(A));
      check("LOCAL_VAULT_ACCESS_DENIED".equals(denied.error),"EXPLICIT_REVOCATION_WRONG_ERROR");
      check(f.channel.reasons.equals(Collections.singletonList(HARD))&&Open_im_sdk.configurations==0,"EXPLICIT_REVOCATION_SOFT_OR_REOPENED");
    }
  }
  static void epochRejected()throws Exception{
    try(Fixture f=new Fixture()){add(200,A,"SID-A",2,0,"",false);renew(f.broker);f.reason(HARD);check(Open_im_sdk.configurations==0,"EPOCH_CHANGE_REOPENED");}
  }
  static void wrongIdentity()throws Exception{
    try(Fixture f=new Fixture()){add(200,B,"SID-A",1,0,"",false);renew(f.broker);f.reason(SOFT);check(Open_im_sdk.configurations==0,"WRONG_ACCOUNT_CONFIGURED");}
  }
  static void rejectedLocalStore()throws Exception{
    try(Fixture f=new Fixture()){Open_im_sdk.rejectConfiguration=true;add(200,A,"SID-A",1,0,"",false);renew(f.broker);f.reason(SOFT);check(Open_im_sdk.configurations==0,"REJECTED_STORE_PUBLISHED");}
  }
  static void staleResponse(int oldStatus)throws Exception{
    try(Fixture f=new Fixture()){
      Response old=add(oldStatus,A,"SID-A",1,0,"",true);AtomicReference<Throwable> error=new AtomicReference<>();
      Thread worker=new Thread(()->{try{renew(f.broker);}catch(Throwable t){error.set(t);}},"old-lease-response");worker.start();await(old.entered);
      f.call("lock",null);add(200,B,"SID-B",1,0,"",false);Reply replacement=f.call("unlock",credentials(B));
      check(replacement.error==null&&B.equals(field("activeAccount").get(f.broker)),"NEW_ACTOR_NOT_PUBLISHED");
      int locks=Open_im_sdk.locks,configs=Open_im_sdk.configurations;long serial=((AtomicLong)field("authorizationSerial").get(f.broker)).get();
      old.release.countDown();worker.join(4000);check(!worker.isAlive()&&error.get()==null,"OLD_RESPONSE_DID_NOT_FINISH");
      check(B.equals(field("activeAccount").get(f.broker))&&B.equals(Open_im_sdk.configuredAccount),"LATE_OLD_RESPONSE_REPLACED_ACTOR");
      check(Open_im_sdk.locks==locks&&Open_im_sdk.configurations==configs&&f.channel.reasons.isEmpty(),"LATE_OLD_RESPONSE_MUTATED_NATIVE");
      check(((AtomicLong)field("authorizationSerial").get(f.broker)).get()==serial,"LATE_OLD_RESPONSE_ADVANCED_SERIAL");
    }
  }
  static void invalidCurrentArguments()throws Exception{
    try(Fixture f=new Fixture()){Reply reply=f.call("authorization/current",Collections.emptyMap());check("LOCAL_VAULT_REAUTHORIZATION_REQUIRED".equals(reply.error)&&A.equals(field("activeAccount").get(f.broker))&&requests==0&&Open_im_sdk.locks==0,"CURRENT_ARGUMENT_ACCEPTED_OR_MUTATED_ACTOR");}
  }
  static void queuedHardNotificationCannotRejectNewUnlock()throws Exception{
    try(Fixture f=new Fixture()){
      android.os.Handler.immediate=false;
      add(401,A,"SID-A",1,0,"",false);renew(f.broker);
      check(f.channel.reasons.isEmpty()&&Open_im_sdk.locks==1&&field("activeAccount").get(f.broker)==null,"OLD_HARD_NOTIFICATION_NOT_QUEUED");
      Response fresh=add(200,A,"SID-A",1,0,"",true);Reply reply=new Reply();AtomicReference<Throwable> error=new AtomicReference<>();
      Thread newer=new Thread(()->{try{f.broker.handle("unlock",credentials(A),reply);}catch(Throwable t){error.set(t);}},"pending-new-unlock");newer.start();await(fresh.entered);
      android.os.Handler.drain();check(f.channel.reasons.isEmpty()&&reply.calls==0,"OLD_QUEUED_HARD_EVENT_ESCAPED_TO_NEW_LOGIN");
      fresh.release.countDown();newer.join(4000);check(!newer.isAlive()&&error.get()==null,"PENDING_UNLOCK_FAILED");
      android.os.Handler.drain();check(reply.calls==1&&reply.error==null&&A.equals(field("activeAccount").get(f.broker))&&Open_im_sdk.configurations==1,"NEW_UNLOCK_NOT_PUBLISHED_AFTER_OLD_EVENT");
      android.os.Handler.immediate=true;add(403,A,"SID-A",1,0,"",false);renew(f.broker);
      check(f.channel.reasons.equals(Collections.singletonList(HARD))&&Open_im_sdk.locks==2&&field("activeAccount").get(f.broker)==null,"NEW_CURRENT_REAL_DENIAL_NOT_HARD");
    }
  }
  interface Case {void run()throws Exception;}
  static void test(String name,Case operation)throws Exception{operation.run();caseCount++;System.out.println("PASS "+name);}
  public static void main(String[] args)throws Exception{
    Security.addProvider(new Provider("AndroidKeyStore","1.0","Synthetic JVM OS wrapping double"){{put("KeyStore.AndroidKeyStore",TestKeyStore.class.getName());}});
    URL.setURLStreamHandlerFactory(protocol->protocol.equals("https")?new URLStreamHandler(){protected URLConnection openConnection(URL url){
      check((WAN+"/enterprise/v1/session/state").equals(url.toString()),"UNEXPECTED_URL");Response response=responses.pollFirst();check(response!=null,"UNPLANNED_HTTP_REQUEST");requests++;
      return new HttpsURLConnection(url){
        public void disconnect(){}public boolean usingProxy(){return false;}public void connect(){}
        public String getCipherSuite(){return "SYNTHETIC";}public Certificate[] getLocalCertificates(){return null;}public Certificate[] getServerCertificates(){return null;}
        public int getResponseCode()throws IOException{response.entered.countDown();try{await(response.release);}catch(Exception failure){throw new IOException("SYNTHETIC_LATCH",failure);}if("transport".equals(response.fault))throw new IOException("SYNTHETIC_TRANSPORT");if("timeout".equals(response.fault))throw new SocketTimeoutException("SYNTHETIC_TIMEOUT");return response.status;}
        public InputStream getInputStream(){JSONObject.envelope.set(response.body);return new ByteArrayInputStream("{}".getBytes());}
      };}}:null);
    if(args.length==1&&args[0].equals("expired-renewal")){
      test("expired_renewal_soft_without_network",NativeVaultBackgroundHarness::expiredRenewal);
      System.out.println("COMPLETE 1/1 captured-source lease expiry probe");return;
    }
    check(args.length==0,"UNKNOWN_HARNESS_ARGUMENT");
    test("paused_over_8s_current_locks_and_denies_read",()->expiredCurrent(false));
    test("expired_current_repeat_does_not_reopen",()->expiredCurrent(true));
    test("expired_renewal_soft_without_network",NativeVaultBackgroundHarness::expiredRenewal);
    test("transport_soft",()->failure(200,0,"transport",SOFT));
    test("timeout_bounded_two_attempts_soft",()->failure(200,0,"timeout",SOFT));
    test("http500_soft",()->failure(500,0,"",SOFT));
    test("http401_hard",()->failure(401,0,"",HARD));
    test("http403_hard",()->failure(403,0,"",HARD));
    test("body1501_hard",()->failure(200,1501,"",HARD));
    test("body1002_hard",()->failure(200,1002,"",HARD));
    test("other_application_failure_soft",()->failure(200,5001,"",SOFT));
    test("fresh_online_success_before_reopen",NativeVaultBackgroundHarness::freshAfterExpiry);
    test("expired_direct_unlock_soft_then_fresh_retry",NativeVaultBackgroundHarness::expiredUnlockMustRetryOnline);
    test("explicit_http403_hard_result",()->explicitDenial(403,0));
    test("explicit_body1501_hard_result",()->explicitDenial(200,1501));
    test("security_epoch_change_hard",NativeVaultBackgroundHarness::epochRejected);
    test("wrong_account_never_configured",NativeVaultBackgroundHarness::wrongIdentity);
    test("local_store_failure_soft_no_publish",NativeVaultBackgroundHarness::rejectedLocalStore);
    test("late_old_success_cannot_reopen_or_lock_new_account",()->staleResponse(200));
    test("late_old_denial_cannot_lock_new_account",()->staleResponse(403));
    test("current_rejects_unexpected_arguments",NativeVaultBackgroundHarness::invalidCurrentArguments);
    test("old_queued_hard_event_cannot_reject_pending_new_unlock",NativeVaultBackgroundHarness::queuedHardNotificationCannotRejectNewUnlock);
    System.out.println("COMPLETE "+caseCount+"/"+caseCount+" unmodified production Broker background recovery cases");
  }
}
