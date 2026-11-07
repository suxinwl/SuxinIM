package io.openim.flutter_openim_sdk;

import android.content.Context;
import android.security.keystore.KeyGenParameterSpec;
import android.security.keystore.KeyProperties;
import android.util.AtomicFile;
import android.util.Base64;
import org.json.JSONObject;
import java.io.*;
import java.net.URL;
import java.nio.charset.StandardCharsets;
import java.security.*;
import java.security.cert.*;
import java.util.*;
import java.util.concurrent.*;
import javax.crypto.*;
import javax.crypto.spec.GCMParameterSpec;
import javax.net.ssl.*;
import io.flutter.plugin.common.MethodChannel;
import open_im_sdk.Open_im_sdk;
import open_im_sdk.NativeRestore;

/** Native authority: Dart supplies session proof, never an encryption key. No
 * business plaintext is written to SharedPreferences, temporary files or logs. */
public final class NativeVaultBroker {
  private static final String LAN="https://192.168.10.11:9885",WAN="https://im.szputy.com:9885";
  private static final String ALIAS="puty.local.wrap.v1";
  private final Context context;
  private final ExecutorService executor=Executors.newSingleThreadExecutor();
  private final ExecutorService authorizationExecutor=Executors.newSingleThreadExecutor();
  private final java.util.concurrent.atomic.AtomicBoolean unlocking=new java.util.concurrent.atomic.AtomicBoolean();
  private volatile String activeAccount;
  private volatile Map<String,Object> activeCredentials;
  private final Object stateLock=new Object();
  private final java.util.concurrent.atomic.AtomicLong generation=new java.util.concurrent.atomic.AtomicLong();
  private final java.util.concurrent.atomic.AtomicLong authorizationSerial=new java.util.concurrent.atomic.AtomicLong();
  private volatile MobileLegacyQuarantine legacyQuarantine;
  private final ScheduledExecutorService leaseExecutor=Executors.newScheduledThreadPool(2);
  private final java.util.concurrent.atomic.AtomicBoolean refreshing=new java.util.concurrent.atomic.AtomicBoolean();
  private String sessionID;private long epoch;
  private volatile long leaseDeadline;
  private volatile boolean previewAllowed,saveAllowed,closed;
  private final Map<String,NativeRestore> restores=new HashMap<>();
  private MethodChannel vaultChannel;
  final NativeBackupPicker backupPicker;
  /** Only an explicit response from the configured online authority proves
   * account revocation. An expired local lease or unavailable transport does
   * not invalidate the stored login proof, but always closes the local store. */
  private static final class AccessDenied extends GeneralSecurityException { AccessDenied(){super("Online access denied");} }
  private static String lockReason(Exception failure){return failure instanceof AccessDenied?"ONLINE_AUTHORIZATION_REQUIRED":"ONLINE_REAUTHORIZATION_REQUIRED";}
  public NativeVaultBroker(Context context){this.context=context.getApplicationContext();EncryptedScratchFiles.Authority authority=new EncryptedScratchFiles.Authority(){public String authorizedAccount(){return currentlyAuthorized()?activeAccount:null;}public long generation(){return NativeVaultBroker.this.generation.get();}};EncryptedScratchFiles.initialize(this.context,authority);EncryptedSelectedFiles.initialize(authority,()->currentlyAuthorized()&&previewAllowed);backupPicker=new NativeBackupPicker(this.context,executor,authority,()->currentlyAuthorized()&&saveAllowed);leaseExecutor.scheduleAtFixedRate(this::renewLease,3,3,TimeUnit.SECONDS);}
  public void setChannel(MethodChannel channel){vaultChannel=channel;}
  private void lockGeneration(long expected){lockGeneration(expected,true);}
  private void lockGeneration(long expected,boolean notify){lockGeneration(expected,notify,"ONLINE_AUTHORIZATION_REQUIRED");}
  private void lockGeneration(long expected,boolean notify,String reason){final long lockedGeneration,lockedAuthorizationSerial;
    synchronized(stateLock){
      if(generation.get()!=expected)return;
      lockedGeneration=generation.incrementAndGet();lockedAuthorizationSerial=authorizationSerial.incrementAndGet();
      String account=activeAccount;activeAccount=null;activeCredentials=null;sessionID=null;epoch=0;previewAllowed=false;saveAllowed=false;leaseDeadline=0;
      // Configure, restore and native teardown are one transition. An old
      // GoLock must never run after a newer actor has reopened the same account.
      EncryptedScratchFiles.closeAll();backupPicker.close();abortRestores();
      if(account!=null)Open_im_sdk.lockNativeLocalStore(account);
    }
    if(notify&&vaultChannel!=null)new android.os.Handler(android.os.Looper.getMainLooper()).post(()->{
      synchronized(stateLock){if(!closed&&generation.get()==lockedGeneration&&authorizationSerial.get()==lockedAuthorizationSerial&&activeAccount==null&&vaultChannel!=null){vaultChannel.invokeMethod("onVaultLocked",reason);}}
    });
  }

  private void renewLease(){
    final Map<String,Object> credentials;final long current,serial;
    synchronized(stateLock){
      credentials=activeCredentials;
      if(closed||unlocking.get()||credentials==null||!refreshing.compareAndSet(false,true))return;
      current=generation.get();serial=authorizationSerial.get();
    }
    try{unlock(credentials,current,true,serial);}
    catch(Exception rejected){synchronized(stateLock){if(authorizationSerial.get()==serial)lockGeneration(current,true,lockReason(rejected));}}
    finally{refreshing.set(false);}
  }

  private boolean currentlyAuthorized(){return !closed&&activeAccount!=null&&android.os.SystemClock.elapsedRealtime()<leaseDeadline;}
  private void abortRestores(){synchronized(restores){for(NativeRestore restore:restores.values())restore.abort();restores.clear();}}
  public void close(){EncryptedScratchFiles.closeAll();closed=true;long current=generation.get();lockGeneration(current);leaseExecutor.shutdownNow();authorizationExecutor.shutdownNow();executor.shutdownNow();synchronized(restores){for(NativeRestore restore:restores.values())restore.abort();restores.clear();}}
  private static byte[] utf8(String value){return value.getBytes(StandardCharsets.UTF_8);}
  private static String digest(String value)throws Exception{StringBuilder result=new StringBuilder();for(byte b:MessageDigest.getInstance("SHA-256").digest(utf8(value)))result.append(String.format(Locale.ROOT,"%02x",b));return result.toString();}
  private SecretKey wrappingKey()throws Exception{
    KeyStore store=KeyStore.getInstance("AndroidKeyStore");store.load(null);
    if(!store.containsAlias(ALIAS)){KeyGenerator generator=KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES,"AndroidKeyStore");generator.init(new KeyGenParameterSpec.Builder(ALIAS,KeyProperties.PURPOSE_ENCRYPT|KeyProperties.PURPOSE_DECRYPT).setKeySize(256).setBlockModes(KeyProperties.BLOCK_MODE_GCM).setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE).setRandomizedEncryptionRequired(true).build());generator.generateKey();}
    return (SecretKey)store.getKey(ALIAS,null);
  }
  private byte[] seal(byte[] plain,String identity)throws Exception{Cipher cipher=Cipher.getInstance("AES/GCM/NoPadding");cipher.init(Cipher.ENCRYPT_MODE,wrappingKey());cipher.updateAAD(utf8(identity));byte[] body=cipher.doFinal(plain);ByteArrayOutputStream output=new ByteArrayOutputStream();output.write(utf8("PUTYOS1\n"));output.write(cipher.getIV());output.write(body);return output.toByteArray();}
  private byte[] unseal(byte[] bytes,String identity)throws Exception{if(bytes.length<36||!Arrays.equals(Arrays.copyOf(bytes,8),utf8("PUTYOS1\n")))throw new GeneralSecurityException("Invalid protected envelope");Cipher cipher=Cipher.getInstance("AES/GCM/NoPadding");cipher.init(Cipher.DECRYPT_MODE,wrappingKey(),new GCMParameterSpec(128,Arrays.copyOfRange(bytes,8,20)));cipher.updateAAD(utf8(identity));return cipher.doFinal(bytes,20,bytes.length-20);}
  private File root()throws IOException{File directory=new File(context.getNoBackupFilesDir(),"puty-local");if(!directory.isDirectory()&&!directory.mkdirs())throw new IOException("Cannot create protected store");return directory;}
  private static void atomicWrite(File file,byte[] bytes)throws IOException{AtomicFile atomic=new AtomicFile(file);FileOutputStream stream=null;try{stream=atomic.startWrite();stream.write(bytes);stream.getFD().sync();atomic.finishWrite(stream);}catch(IOException error){if(stream!=null)atomic.failWrite(stream);throw error;}}
  private static byte[] read(File file,int max)throws IOException{try(InputStream input=new FileInputStream(file);ByteArrayOutputStream output=new ByteArrayOutputStream()){byte[] buffer=new byte[8192];for(int count;(count=input.read(buffer))!=-1;){if(output.size()+count>max)throw new IOException("Protected object exceeds limit");output.write(buffer,0,count);}return output.toByteArray();}}
  private byte[] master(File directory,String account)throws Exception{File envelope=new File(directory,"master.wrap");if(envelope.exists()){byte[] key=unseal(read(envelope,4096),"master\u0000"+account);if(key.length!=32)throw new GeneralSecurityException("Invalid native master");return key;}byte[] key=new byte[32];new SecureRandom().nextBytes(key);atomicWrite(envelope,seal(key,"master\u0000"+account));return key;}

  private HttpsURLConnection connection(String origin)throws Exception{
    if(!LAN.equals(origin)&&!WAN.equals(origin))throw new GeneralSecurityException("Unconfigured vault origin");
    HttpsURLConnection connection=(HttpsURLConnection)new URL(origin+"/enterprise/v1/session/state").openConnection();connection.setInstanceFollowRedirects(false);connection.setConnectTimeout(1500);connection.setReadTimeout(1500);connection.setUseCaches(false);
    if(LAN.equals(origin)){
      // Reuse the already-reviewed exact-IP PKIX authority. It returns no
      // caller-controlled trust manager and does not change global defaults.
      Class<?> authority=Class.forName("cn.rentsoft.flutter.openim.consumer.EnterpriseLanTls");
      connection.setSSLSocketFactory((SSLSocketFactory)authority.getMethod("verifiedSocketFactory",Context.class).invoke(null,context));
    }
    return connection;
  }
  private Map<String,Object> unlock(Map<String,Object> args,long requested,boolean renewal,long authorizationRequest)throws Exception{
    String origin=(String)args.get("origin"),account=(String)args.get("userID"),token=(String)args.get("imToken"),chat=(String)args.get("chatToken");
    if(account==null||!account.matches("emp_[0-9a-f]{32}")||token==null||token.isEmpty()||chat==null||chat.isEmpty())throw new GeneralSecurityException("Missing current identity proof");
    final boolean existingLease,retryAllowed;final long originalDeadline,originalEpoch;final String originalSID;
    synchronized(stateLock){
      if(closed||generation.get()!=requested||authorizationSerial.get()!=authorizationRequest)throw new GeneralSecurityException("Vault generation changed");
      existingLease=account.equals(activeAccount)&&activeCredentials!=null&&origin.equals(activeCredentials.get("origin"))&&token.equals(activeCredentials.get("imToken"))&&chat.equals(activeCredentials.get("chatToken"));
      if(existingLease&&!currentlyAuthorized())throw new GeneralSecurityException("Vault lease expired");
      originalDeadline=leaseDeadline;originalSID=sessionID;originalEpoch=epoch;
      retryAllowed=existingLease;
    }
    NativeVaultRenewal.Clock clock=android.os.SystemClock::elapsedRealtime;
    NativeVaultRenewal.Current current=()->{synchronized(stateLock){return !closed&&generation.get()==requested&&authorizationSerial.get()==authorizationRequest&&(!existingLease||(account.equals(activeAccount)&&originalDeadline==leaseDeadline&&originalEpoch==epoch&&Objects.equals(originalSID,sessionID)&&activeCredentials!=null&&origin.equals(activeCredentials.get("origin"))));}};
    JSONObject state=NativeVaultRenewal.execute(existingLease,retryAllowed,originalDeadline,clock,current,timeout->requestSessionState(origin,token,chat,timeout));
    long now=System.currentTimeMillis();NativeVaultSessionState.Result verified=NativeVaultSessionState.validate(state,account,now);
    long lease=Math.min(verified.expiresAt,now+8000);String sid=verified.sessionID;long currentEpoch=verified.epoch;
    if(existingLease&&(!Objects.equals(originalSID,sid)||originalEpoch!=currentEpoch))throw new AccessDenied();
    File directory=new File(new File(root(),"accounts"),digest(account));
    byte[] key=null;boolean configured=false,published=false;long actorGeneration;
    try{synchronized(stateLock){try{
      if(closed||generation.get()!=requested||authorizationSerial.get()!=authorizationRequest)throw new GeneralSecurityException("Vault generation changed");
      NativeVaultRenewal.requireCurrent(existingLease,originalDeadline,clock,current);
      if(!Open_im_sdk.recoverNativeStoreRoot(directory.getAbsolutePath()))throw new IOException("Protected restore recovery rejected");if(!directory.isDirectory()&&!directory.mkdirs())throw new IOException("Cannot create account vault");key=master(directory,account);
      boolean changed=activeAccount!=null&&(!activeAccount.equals(account)||!sid.equals(sessionID)||epoch!=currentEpoch||activeCredentials==null||!origin.equals(activeCredentials.get("origin")));
      if(renewal&&changed)throw new GeneralSecurityException("Vault identity changed");
      if(changed){String previous=activeAccount;activeAccount=null;activeCredentials=null;generation.incrementAndGet();EncryptedScratchFiles.closeAll();backupPicker.close();abortRestores();Open_im_sdk.lockNativeLocalStore(previous);}
      if(existingLease)NativeVaultRenewal.requireCurrent(true,originalDeadline,clock,current);
      if(!Open_im_sdk.configureNativeLocalStore(account,directory.getCanonicalPath(),key,lease))throw new GeneralSecurityException("Native vault rejected authorization");configured=true;
      // Renewal must never wait behind a large quarantine seal's monitor.
      // Its device key/owned roots are already bound to this Go Store.
      if(!renewal&&activeAccount==null){
      if(legacyQuarantine==null)legacyQuarantine=MobileLegacyQuarantine.create(context);
      byte[] migrationKey=legacyQuarantine.masterCopy();try{if(!Open_im_sdk.configureNativeLegacyQuarantine(account,legacyQuarantine.directory().getCanonicalPath(),migrationKey))throw new GeneralSecurityException("Native migration authority rejected");}finally{Arrays.fill(migrationKey,(byte)0);}
      org.json.JSONArray roots=new org.json.JSONArray();for(File owned:new File[]{new File(context.getApplicationInfo().dataDir,"app_flutter"),context.getCacheDir(),context.getExternalFilesDir(null),context.getExternalCacheDir()})if(owned!=null&&owned.isDirectory())roots.put(owned.getAbsolutePath());if(!Open_im_sdk.setNativeLegacyRoots(account,roots.toString()))throw new GeneralSecurityException("Native legacy ownership rejected");
      }
      if(existingLease)NativeVaultRenewal.requireCurrent(true,originalDeadline,clock,current);activeAccount=account;activeCredentials=new HashMap<>(args);sessionID=sid;epoch=currentEpoch;leaseDeadline=android.os.SystemClock.elapsedRealtime()+Math.max(0,lease-System.currentTimeMillis());saveAllowed=verified.saveAllowed;previewAllowed=verified.previewAllowed;actorGeneration=generation.get();published=true;
    }finally{if(configured&&!published)Open_im_sdk.lockNativeLocalStore(account);}}}finally{if(key!=null)Arrays.fill(key,(byte)0);}
    Map<String,Object> result=new HashMap<>();result.put("userID",account);result.put("directory",directory.getCanonicalPath());result.put("authorizedUntil",lease);result.put("actorGeneration",actorGeneration);result.put("saveAllowed",verified.saveAllowed);return result;
  }

  private JSONObject requestSessionState(String origin,String token,String chat,int timeoutMillis)throws Exception{
    HttpsURLConnection conn=connection(origin);
    conn.setConnectTimeout(Math.min(1500,timeoutMillis));conn.setReadTimeout(Math.min(1500,timeoutMillis));
    ScheduledFuture<?> deadline=leaseExecutor.schedule(conn::disconnect,timeoutMillis,TimeUnit.MILLISECONDS);
    try{conn.setRequestProperty("token",token);conn.setRequestProperty("chatToken",chat);conn.setRequestProperty("platformID","2");conn.setRequestProperty("Cache-Control","no-store");int status=conn.getResponseCode();if(status==401||status==403)throw new AccessDenied();if(status!=200)throw new IOException("Online vault authority unavailable");try(InputStream input=conn.getInputStream();ByteArrayOutputStream output=new ByteArrayOutputStream()){byte[] buffer=new byte[4096];for(int n;(n=input.read(buffer))!=-1;){if(output.size()+n>65536)throw new IOException("Authorization response limit");output.write(buffer,0,n);}JSONObject body=new JSONObject(output.toString("UTF-8"));int code=body.optInt("errCode",body.optInt("code",-1));if(code==1501||code==1002)throw new AccessDenied();if(code!=0)throw new IOException("Online vault authority unavailable");return body.getJSONObject("data");}}
    finally{deadline.cancel(false);conn.disconnect();}
  }

  private HttpsURLConnection resourceConnection(String origin,String path)throws Exception{
    if(!LAN.equals(origin)&&!WAN.equals(origin))throw new GeneralSecurityException("Unconfigured resource origin");
    URL url=new URL(origin+path);HttpsURLConnection conn=(HttpsURLConnection)url.openConnection();conn.setInstanceFollowRedirects(false);conn.setConnectTimeout(8000);conn.setReadTimeout(15000);conn.setUseCaches(false);
    if(LAN.equals(origin)){Class<?> authority=Class.forName("cn.rentsoft.flutter.openim.consumer.EnterpriseLanTls");conn.setSSLSocketFactory((SSLSocketFactory)authority.getMethod("verifiedSocketFactory",Context.class).invoke(null,context));}return conn;
  }
  private JSONObject response(HttpsURLConnection conn)throws Exception{
    if(conn.getResponseCode()!=200)throw new GeneralSecurityException("Protected resource rejected");
    try(InputStream input=conn.getInputStream();ByteArrayOutputStream output=new ByteArrayOutputStream()){byte[] buffer=new byte[4096];for(int n;(n=input.read(buffer))!=-1;){if(output.size()+n>65536)throw new IOException("Protected response limit");output.write(buffer,0,n);}JSONObject envelope=new JSONObject(output.toString("UTF-8"));if(envelope.optInt("errCode",envelope.optInt("code",-1))!=0)throw new GeneralSecurityException("Protected resource rejected");return envelope.getJSONObject("data");}
  }
  private Map<String,Object> saveAttachment(Map<String,Object> args)throws Exception{
    Map<String,Object> credentials=activeCredentials;String account=activeAccount;
    if(account==null||credentials==null)throw new GeneralSecurityException("Vault locked");
    String origin=(String)credentials.get("origin"),source=(String)args.get("url"),sourceName=(String)args.get("sourceName");
    if(source==null||sourceName==null||sourceName.isEmpty())throw new GeneralSecurityException("Missing attachment source");
    HttpsURLConnection authorize=resourceConnection(origin,"/enterprise/v1/attachment/authorize");JSONObject grant;
    try{authorize.setRequestMethod("POST");authorize.setDoOutput(true);authorize.setRequestProperty("Content-Type","application/json");authorize.setRequestProperty("token",(String)credentials.get("imToken"));authorize.setRequestProperty("chatToken",(String)credentials.get("chatToken"));authorize.setRequestProperty("platformID","2");byte[] body=utf8(new JSONObject().put("url",source).put("purpose","save").toString());authorize.setFixedLengthStreamingMode(body.length);try(OutputStream output=authorize.getOutputStream()){output.write(body);}grant=response(authorize);}finally{authorize.disconnect();}
    String url=grant.getString("url"),proof=grant.getString("resourceProof"),object=grant.getString("objectID");URL parsed=new URL(url);
    if(!proof.matches("[0-9a-f]{64}")||!url.startsWith(origin+"/attachments/")||parsed.getUserInfo()!=null||parsed.getRef()!=null||!origin.equals(parsed.getProtocol()+"://"+parsed.getAuthority()))throw new GeneralSecurityException("Invalid protected read binding");
    String task=digest(object+"\u0000"+sourceName),conversation=args.get("conversationID") instanceof String?(String)args.get("conversationID"):"",message=args.get("messageID") instanceof String?(String)args.get("messageID"):"";
    String path=Open_im_sdk.beginNativeAttachment(account,task,object,sourceName,conversation,message);if(path==null||path.isEmpty())throw new IOException("Protected name allocation failed");
    if(!Open_im_sdk.nativeAttachmentComplete(account,task)){
      long offset=Open_im_sdk.nativeAttachmentOffset(account,task);String priorTag=Open_im_sdk.nativeAttachmentETag(account,task);
      if(offset<0)throw new IOException("Protected pending state missing");
      if(offset>0&&(priorTag==null||priorTag.isEmpty())){if(!Open_im_sdk.resetNativeAttachment(account,task))throw new IOException("Protected resume reset failed");path=Open_im_sdk.beginNativeAttachment(account,task,object,sourceName,conversation,message);offset=0;}
      boolean completed=false;
      try{
        for(int attempt=0;attempt<2;attempt++){
          HttpsURLConnection download=resourceConnection(origin,url.substring(origin.length()));
          try{
            download.setRequestProperty("X-Puty-Resource-Proof",proof);
            if(offset>0){download.setRequestProperty("Range","bytes="+offset+"-");download.setRequestProperty("If-Range",priorTag);}
            int status=download.getResponseCode();String tag=download.getHeaderField("ETag");
            if(offset>0&&status==200&&attempt==0){
              if(!Open_im_sdk.resetNativeAttachment(account,task))throw new IOException("Protected object changed");path=Open_im_sdk.beginNativeAttachment(account,task,object,sourceName,conversation,message);offset=0;priorTag="";continue;
            }
            long expected=args.get("size") instanceof Number?((Number)args.get("size")).longValue():-1;
            if(offset>0){
              String contentRange=download.getHeaderField("Content-Range");java.util.regex.Matcher match=java.util.regex.Pattern.compile("bytes ([0-9]+)-([0-9]+)/([0-9]+)").matcher(contentRange==null?"":contentRange);
              if(status!=206||!match.matches()||Long.parseLong(match.group(1))!=offset||Long.parseLong(match.group(2))<offset||!priorTag.equals(tag))throw new GeneralSecurityException("Protected range mismatch");
              long total=Long.parseLong(match.group(3));if(expected>=0&&expected!=total)throw new GeneralSecurityException("Protected range size mismatch");expected=total;
            }else if(status!=200)throw new GeneralSecurityException("Protected download rejected");
            if(tag!=null&&!tag.isEmpty()&&!Open_im_sdk.setNativeAttachmentETag(account,task,tag))throw new IOException("Protected object validator rejected");
            long actual=offset;
            try(InputStream input=download.getInputStream()){
              byte[] buffer=new byte[65536];
              try{for(int n;(n=input.read(buffer))!=-1;){actual+=n;if(actual>1073741824L)throw new IOException("Protected attachment exceeds limit");byte[] chunk=Arrays.copyOf(buffer,n);try{if(!Open_im_sdk.writeNativeAttachment(account,task,chunk))throw new IOException("Protected write rejected");}finally{Arrays.fill(chunk,(byte)0);}}}finally{Arrays.fill(buffer,(byte)0);}
            }
            if(expected<0)expected=actual;
            if(!Open_im_sdk.finishNativeAttachment(account,task,expected))throw new IOException("Protected attachment commit failed");completed=true;break;
          }finally{download.disconnect();}
        }
        if(!completed)throw new IOException("Protected range retry exhausted");
      }finally{if(!completed)Open_im_sdk.abortNativeAttachment(account,task);}
    }

    Map<String,Object> result=new HashMap<>();result.put("taskID",task);result.put("name",new File(path).getName());result.put("path",path);result.put("encrypted",true);return result;
  }
  private static long exactMillis(Object value)throws GeneralSecurityException{if(!(value instanceof Integer)&&!(value instanceof Long))throw new GeneralSecurityException("Invalid time scope");long n=((Number)value).longValue();if(n<0||n>9007199254740991L)throw new GeneralSecurityException("Invalid time scope");return n;}
  public void handle(String method,Object arguments,MethodChannel.Result result){
    if(method.equals("backup/pick")){backupPicker.launch(false,null,result);return;}
    if(method.equals("backup/save")){if(!currentlyAuthorized()||!saveAllowed){result.error("LOCAL_BACKUP_UNAVAILABLE","BackupSaveNotAllowed",null);return;}if(!(arguments instanceof Map)){result.error("LOCAL_BACKUP_UNAVAILABLE","InvalidBackupRequest",null);return;}backupPicker.launch(true,(String)((Map)arguments).get("path"),result);return;}
    // Lock must not queue behind a large network transfer. The Go driver also
    // denies in-flight writes after this account is removed from its registry.
    if(method.equals("lock")){synchronized(stateLock){lockGeneration(generation.get(),false);}result.success(true);return;}
    if(method.equals("authorization/current")){if(arguments!=null){result.error("LOCAL_VAULT_REAUTHORIZATION_REQUIRED","InvalidAuthorizationCheck",null);return;}synchronized(stateLock){boolean current=currentlyAuthorized();if(!current&&activeAccount!=null)lockGeneration(generation.get(),true,"ONLINE_REAUTHORIZATION_REQUIRED");result.success(current);}return;}
    final long requested,authorizationRequest;
    synchronized(stateLock){requested=generation.get();if(method.equals("unlock")){unlocking.set(true);authorizationRequest=authorizationSerial.incrementAndGet();}else authorizationRequest=authorizationSerial.get();}
    Runnable operation=()->{
    try{
      Object response;
      if(method.equals("unlock")){if(!(arguments instanceof Map))throw new GeneralSecurityException("Missing identity proof");response=unlock((Map<String,Object>)arguments,requested,false,authorizationRequest);}
      else if(method.equals("preferences/read")){File file=new File(root(),"settings.wrap");response=file.exists()?new String(unseal(read(file,2<<20),"settings"),StandardCharsets.UTF_8):"{}";}
      else if(method.equals("preferences/write")){if(!(arguments instanceof String)||utf8((String)arguments).length>(1<<20))throw new IOException("Invalid settings size");new JSONObject((String)arguments);atomicWrite(new File(root(),"settings.wrap"),seal(utf8((String)arguments),"settings"));response=true;}
      else if(method.equals("legacy/finalize")){if(!currentlyAuthorized()||legacyQuarantine==null)throw new GeneralSecurityException("Vault locked");legacyQuarantine.sealRemaining();response=true;}
      else if(method.equals("archive/previewClear")){Map<String,Object>a=(Map<String,Object>)arguments;if(!currentlyAuthorized())throw new GeneralSecurityException("Vault locked");response=Open_im_sdk.previewNativeClearRange(activeAccount,(String)a.get("conversationID"),((Number)a.get("from")).longValue(),((Number)a.get("to")).longValue());if(response==null||((String)response).isEmpty())throw new IOException("Invalid local clear scope");}
      else if(method.equals("archive/previewClearScopes")||method.equals("archive/clearScopes")){Map<String,Object>a=(Map<String,Object>)arguments;if(!currentlyAuthorized()||!(a.get("conversationIDs") instanceof java.util.List))throw new GeneralSecurityException("Vault locked or invalid scope");String ids=new org.json.JSONArray((java.util.List<?>)a.get("conversationIDs")).toString();long from=exactMillis(a.get("from")),to=exactMillis(a.get("toExclusive"));if(method.equals("archive/clearScopes"))EncryptedScratchFiles.closeAll();response=method.equals("archive/previewClearScopes")?Open_im_sdk.previewNativeClearScopes(activeAccount,ids,from,to):Open_im_sdk.clearNativeLocalScopes(activeAccount,ids,from,to);if(response==null||(response instanceof String&&((String)response).isEmpty()))throw new IOException("Invalid local clear scope");}
      else if(method.equals("attachment/import")){Map<String,Object>a=(Map<String,Object>)arguments;if(!currentlyAuthorized()||!saveAllowed)throw new GeneralSecurityException("Protected save rejected");response=Open_im_sdk.importNativeLocalAttachment(activeAccount,(String)a.get("path"),(String)a.get("conversationID"));if(response==null||((String)response).isEmpty())throw new IOException("Protected local import rejected");}
      else if(method.equals("archive/page")){Map<String,Object> a=(Map<String,Object>)arguments;if(activeAccount==null)throw new GeneralSecurityException("Vault locked");response=Open_im_sdk.getNativeLocalArchivePage(activeAccount,(String)a.get("conversationID"),((Number)a.get("before")).longValue(),a.get("beforeID") instanceof String?(String)a.get("beforeID"):"",((Number)a.get("limit")).intValue());}
      else if(method.equals("archive/clear")){Map<String,Object> a=(Map<String,Object>)arguments;if(activeAccount==null)throw new GeneralSecurityException("Vault locked");response=Open_im_sdk.clearNativeLocalRange(activeAccount,(String)a.get("conversationID"),((Number)a.get("from")).longValue(),((Number)a.get("to")).longValue());}
      else if(method.equals("attachment/save")){if(!(arguments instanceof Map))throw new GeneralSecurityException("Invalid attachment request");response=saveAttachment((Map<String,Object>)arguments);}
      else if(method.equals("attachment/list")){Map<String,Object>a=(Map<String,Object>)arguments;if(!currentlyAuthorized())throw new GeneralSecurityException("Vault locked");response=Open_im_sdk.listNativeAttachments(activeAccount,(String)a.get("conversationID"),(String)a.get("cursor"),((Number)a.get("limit")).intValue());if(response==null||((String)response).isEmpty())throw new IOException("Attachment inventory unavailable");}
      else if(method.equals("attachment/previewUri")){Map<String,Object>a=(Map<String,Object>)arguments;response=EncryptedSelectedFiles.attachmentUri((String)a.get("taskID"));}
      else if(method.equals("attachment/read")){Map<String,Object> a=(Map<String,Object>)arguments;if(!currentlyAuthorized()||!previewAllowed)throw new GeneralSecurityException("Protected preview rejected");response=Open_im_sdk.readNativeAttachment(activeAccount,(String)a.get("taskID"),32L<<20);if(response==null)throw new GeneralSecurityException("Protected preview unavailable");}
      else if(method.equals("source/captureSupported")){response=android.os.Build.VERSION.SDK_INT>=26;}
      else if(method.equals("source/previewUri")){Map<String,Object>a=(Map<String,Object>)arguments;String path=(String)a.get("path");response=EncryptedScratchFiles.owns(path)?EncryptedScratchFiles.previewUri(path):EncryptedSelectedFiles.resourceUri(path);}
      else if(method.equals("source/import")){Map<String,Object>a=(Map<String,Object>)arguments;if(!currentlyAuthorized())throw new GeneralSecurityException("Vault locked");String source=(String)a.get("path");boolean scratch=EncryptedScratchFiles.owns(source);if(!scratch&&source.startsWith("/proc/"))throw new GeneralSecurityException("Unknown protected descriptor");String path=scratch?EncryptedScratchFiles.stage(source):Open_im_sdk.stageNativeSource(activeAccount,source);if(path==null||path.isEmpty())throw new IOException("Encrypted source import failed");response=path;}
      else if(method.equals("source/scratch")){Map<String,Object>a=(Map<String,Object>)arguments;response=EncryptedScratchFiles.createTempFile((String)a.get("prefix"),(String)a.get("suffix")).getAbsolutePath();}
      else if(method.equals("source/discard")){Map<String,Object>a=(Map<String,Object>)arguments;String path=(String)a.get("path");if(EncryptedScratchFiles.owns(path)){EncryptedScratchFiles.discard(path);response=true;}else{response=currentlyAuthorized()&&Open_im_sdk.discardNativeSource(activeAccount,path);}}
      else if(method.equals("source/read")){Map<String,Object>a=(Map<String,Object>)arguments;if(!currentlyAuthorized())throw new GeneralSecurityException("Vault locked");String path=(String)a.get("path");response=EncryptedScratchFiles.owns(path)?EncryptedScratchFiles.readBytes(path,32<<20):Open_im_sdk.readNativeSource(activeAccount,path,32L<<20);if(response==null)throw new GeneralSecurityException("Encrypted source unavailable");}
      else if(method.equals("source/fromBytes")){Map<String,Object>a=(Map<String,Object>)arguments;if(!currentlyAuthorized()||!(a.get("bytes") instanceof byte[])||((byte[])a.get("bytes")).length>(32<<20))throw new GeneralSecurityException("Vault source rejected");byte[] bytes=(byte[])a.get("bytes");try{response=Open_im_sdk.stageNativeBytes(activeAccount,(String)a.get("name"),bytes);}finally{Arrays.fill(bytes,(byte)0);}if(response==null||((String)response).isEmpty())throw new IOException("Encrypted source rejected");}
      else if(method.equals("source/info")){Map<String,Object>a=(Map<String,Object>)arguments;if(!currentlyAuthorized())throw new GeneralSecurityException("Vault locked");String path=(String)a.get("path");if(EncryptedScratchFiles.owns(path)){Map<String,Object> info=new HashMap<>();info.put("name",EncryptedScratchFiles.name(path));info.put("size",EncryptedScratchFiles.size(path));response=info;}else{JSONObject info=new JSONObject(Open_im_sdk.nativeSourceInfo(activeAccount,path));Map<String,Object> metadata=new HashMap<>();metadata.put("name",info.getString("name"));metadata.put("size",info.getLong("size"));response=metadata;}}
      else if(method.equals("backup/export")){Map<String,Object>a=(Map<String,Object>)arguments;final String backupAccount;synchronized(stateLock){if(generation.get()!=requested||!currentlyAuthorized()||!saveAllowed)throw new GeneralSecurityException("Backup save not allowed");backupAccount=activeAccount;}String path=Open_im_sdk.exportNativeBackup(backupAccount,(String)a.get("passphrase"));if(path==null||path.isEmpty())throw new IOException("Encrypted backup failed");boolean retained=false;try{synchronized(stateLock){if(generation.get()!=requested||!backupAccount.equals(activeAccount)||!currentlyAuthorized()||!saveAllowed)throw new GeneralSecurityException("Backup save permission changed");backupPicker.retainExport(path,requested,backupAccount);retained=true;}response=path;}finally{if(!retained)backupPicker.discardUnretainedExport(path,backupAccount);}}
      else if(method.equals("backup/prepare")){Map<String,Object>a=(Map<String,Object>)arguments;if(!currentlyAuthorized())throw new GeneralSecurityException("Vault locked");String source=(String)a.get("path");NativeRestore restore;try{restore=source.startsWith("puty-backup:")?Open_im_sdk.prepareNativeRestoreDescriptor(activeAccount,backupPicker.inputDescriptor(source),(String)a.get("passphrase")):Open_im_sdk.prepareNativeRestore(activeAccount,source,(String)a.get("passphrase"));}finally{backupPicker.releaseInput(source);}backupPicker.releaseInput((String)a.get("path"));if(restore==null)throw new GeneralSecurityException("Encrypted backup validation rejected");String id=UUID.randomUUID().toString();synchronized(stateLock){if(generation.get()!=requested||!currentlyAuthorized()){restore.abort();throw new GeneralSecurityException("Restore generation changed");}synchronized(restores){restores.put(id,restore);}}response=id;}
      else if(method.equals("backup/cancel")){Map<String,Object>a=(Map<String,Object>)arguments;NativeRestore restore;synchronized(restores){restore=restores.remove((String)a.get("restoreID"));}if(restore!=null)restore.abort();response=true;}
      else if(method.equals("backup/commit")){Map<String,Object>a=(Map<String,Object>)arguments;NativeRestore restore;synchronized(restores){restore=restores.remove((String)a.get("restoreID"));}if(restore==null||!currentlyAuthorized())throw new GeneralSecurityException("Restore handle unavailable");byte[] master=restore.master();if(master==null||master.length!=32)throw new GeneralSecurityException("Restore key unavailable");try{String restoreAccount=activeAccount;EncryptedScratchFiles.closeAll();byte[] envelope=seal(master,"master\u0000"+restoreAccount);try{if(!restore.prepareCommit(envelope))throw new IOException("Encrypted restore final verification failed");synchronized(stateLock){if(generation.get()!=requested||!restoreAccount.equals(activeAccount)||!currentlyAuthorized())throw new GeneralSecurityException("Restore generation changed");activeCredentials=null;if(!restore.commit(envelope))throw new IOException("Encrypted restore promotion failed");}}finally{Arrays.fill(envelope,(byte)0);}}finally{Arrays.fill(master,(byte)0);restore.abort();}response=true;}
      else{new android.os.Handler(android.os.Looper.getMainLooper()).post(result::notImplemented);return;}
      new android.os.Handler(android.os.Looper.getMainLooper()).post(()->{if(method.equals("backup/export")){synchronized(stateLock){if(generation.get()!=requested||!currentlyAuthorized()||!saveAllowed){backupPicker.discardExport((String)response);result.error("LOCAL_VAULT_REJECTED","BackupSaveNotAllowed",null);}else result.success(response);}return;}if(!method.startsWith("preferences/")&&((method.equals("unlock")?authorizationSerial.get()!=authorizationRequest:generation.get()!=requested)||!currentlyAuthorized())){if(response instanceof byte[])Arrays.fill((byte[])response,(byte)0);result.error("LOCAL_VAULT_REAUTHORIZATION_REQUIRED","GenerationOrLeaseChanged",null);}else result.success(response);});
    }catch(Exception failure){synchronized(stateLock){if((method.equals("unlock")&&authorizationSerial.get()==authorizationRequest)||!currentlyAuthorized())lockGeneration(method.equals("unlock")?generation.get():requested,true,lockReason(failure));}new android.os.Handler(android.os.Looper.getMainLooper()).post(()->result.error(failure instanceof AccessDenied?"LOCAL_VAULT_ACCESS_DENIED":"LOCAL_VAULT_REAUTHORIZATION_REQUIRED",failure.getClass().getSimpleName(),null));}finally{synchronized(stateLock){if(method.equals("unlock")&&authorizationSerial.get()==authorizationRequest)unlocking.set(false);}}
  };if(method.equals("unlock"))authorizationExecutor.execute(operation);else executor.execute(operation);}
}
