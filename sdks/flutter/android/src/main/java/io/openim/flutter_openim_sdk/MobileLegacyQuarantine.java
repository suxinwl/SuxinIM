package io.openim.flutter_openim_sdk;

import android.content.Context;
import android.security.keystore.KeyGenParameterSpec;
import android.security.keystore.KeyProperties;
import android.system.Os;
import android.system.OsConstants;
import android.system.StructStat;
import java.io.*;
import java.nio.charset.StandardCharsets;
import java.security.*;
import java.util.*;
import javax.crypto.*;
import javax.crypto.spec.GCMParameterSpec;
import javax.crypto.spec.SecretKeySpec;

/** Native-only, device-wide quarantine. It never assigns unknown content to the
 * signed-in account and has no MethodChannel route. Call only after the SDK and
 * old preference/Hive producers are closed and their own migrations succeeded.
 * The encrypted .meta is the per-object restart receipt: no source is unlinked
 * until both objects are durable, fully authenticated and matched to its bytes.
 * Normal account backup must not include this separate no-backup directory. */
public final class MobileLegacyQuarantine implements AutoCloseable {
  static final String ACCOUNT="device-legacy", FORMAT="PUTY-LOCAL-STORE-1";
  static final int CHUNK=1048576, MAX_META=65536;
  private static final byte[] MAGIC=utf8("PUTYLS1\n");
  private final File directory;
  private final List<Root> roots;
  private final Fs fs;
  private final KeyProvider keys;
  private final Hook hook;
  private byte[] master;

  /** Only OS wrapping is injectable; production uses a distinct Keystore alias. */
  interface KeyProvider { byte[] wrap(byte[] value)throws Exception; byte[] unwrap(byte[] value)throws Exception; }
  interface Hook { void at(String phase)throws Exception; }
  static final class Entry {
    final long device,inode,size,modified; final boolean regular,directory; final long links;
    Entry(long d,long i,long s,long m,boolean r,boolean f,long l){device=d;inode=i;size=s;modified=m;regular=r;directory=f;links=l;}
    boolean same(Entry b){return b!=null&&device==b.device&&inode==b.inode&&size==b.size&&modified==b.modified&&regular==b.regular&&directory==b.directory&&links==b.links;}
  }
  interface Fs {
    Entry stat(File path)throws Exception;
    InputStream read(File path,Entry expected)throws Exception;
    FileOutputStream create(File path)throws Exception;
    void mkdir(File path)throws Exception;
    void moveNew(File from,File to)throws Exception;
    void replace(File from,File to)throws Exception;
    void sync(File directory)throws Exception;
    void syncFile(File file,Entry expected)throws Exception;
    void remove(File path,Entry expected)throws Exception;
  }
  static final class Root {
    final File declared,canonical;
    Root(File declared)throws IOException{
      this.declared=declared.getAbsoluteFile();canonical=declared.getCanonicalFile();
      // Context may have a platform alias in an ancestor (/data/user/0), but
      // the selected app-owned leaf itself must never redirect our inventory.
      File parent=this.declared.getParentFile();
      if(parent==null||!canonical.equals(new File(parent.getCanonicalFile(),this.declared.getName())))throw new IOException("Legacy root validation failed");
    }
  }
  public static final class Result {
    public final int sealedFiles,skippedDatabases;
    Result(int files,int skipped){sealedFiles=files;skippedDatabases=skipped;}
  }
  private static Exception rejected(){return new IOException("Legacy quarantine validation failed");}

  /** Fixed app-owned roots only. Never passes public Downloads or creates an old
   * app_flutter root through Context.getDir(). Platform aliases are resolved once
   * at this trusted boundary; every descendant must remain link-free. */
  public static synchronized MobileLegacyQuarantine create(Context value)throws Exception{
    try{
    if(android.os.Build.VERSION.SDK_INT<23)throw rejected(); // Keystore AES-GCM; no plaintext fallback.
    Context context=value.getApplicationContext();
    File data=new File(context.getApplicationInfo().dataDir);
    List<Root> roots=new ArrayList<>();
    for(File path:new File[]{new File(data,"app_flutter"),new File(data,"shared_prefs"),context.getCacheDir(),context.getExternalFilesDir(null),context.getExternalCacheDir()})
      if(path!=null)roots.add(new Root(path));
    File noBackup=new Root(context.getNoBackupFilesDir()).canonical;
    File target=new File(new File(noBackup,"puty-local"),"legacy-quarantine");
    return new MobileLegacyQuarantine(target,roots,new AndroidFs(),new AndroidKeys(),phase->{});
    }catch(Exception failure){throw rejected();}
  }

  MobileLegacyQuarantine(File target,List<Root> roots,Fs fs,KeyProvider keys,Hook hook)throws Exception{
    this.directory=target.getAbsoluteFile();this.roots=new ArrayList<>(roots);this.fs=fs;this.keys=keys;this.hook=hook;
    // No untrusted caller chooses this path in production. Parent directories
    // may be absent on a first install; mkdir checks every existing component.
    ensureDirectory(directory);
    for(Root root:roots){
      if(within(directory,root.canonical)||within(root.canonical,directory))throw rejected();
      if(root.canonical.exists())checkDirectory(root.canonical);
    }
    try(Lock ignored=lock()){
      File envelope=new File(directory,"master.wrap");
      if(envelope.exists())master=keys.unwrap(readSmall(envelope,4096));
      else{
        // Existing sealed objects without their OS envelope must never acquire
        // a replacement key, even if an interrupted migration left no marker.
        File[] children=directory.listFiles();if(children==null)throw rejected();
        for(File child:children)if(!child.getName().equals("migration.lock"))throw rejected();
        master=new byte[32];new SecureRandom().nextBytes(master);
        byte[] wrapped=keys.wrap(master);try{writeBytes(envelope,wrapped,false);}finally{Arrays.fill(wrapped,(byte)0);}
      }
      if(master==null||master.length!=32)throw rejected();
    }catch(Exception failure){close();throw rejected();}
  }

  /** Native broker configures Go with a short-lived copy and clears it finally. */
  public synchronized byte[] masterCopy()throws Exception{checkOpen();return master.clone();}
  public File directory(){return directory;}
  @Override public synchronized void close(){if(master!=null){Arrays.fill(master,(byte)0);master=null;}}
  private void checkOpen()throws Exception{if(master==null||master.length!=32)throw rejected();}

  /** Rescans remaining roots on each invocation. A crash after a data rename,
   * metadata rename, or some exact unlinks is restartable without a plaintext
   * inventory. SQLite families belong to the transaction-aware Go migrator. */
  public synchronized Result sealRemaining()throws Exception{
    checkOpen();try(Lock ignored=lock()){
      int[] counts={0,0};List<Source> inventory=new ArrayList<>();Set<String> seen=new HashSet<>();
      for(Root root:roots)if(root.canonical.exists())scan(root,root.canonical,inventory,seen,counts);
      writeState("running",0);
      for(Source source:inventory){seal(source);counts[0]++;writeState("running",counts[0]);}
      List<Source> leftovers=new ArrayList<>();seen.clear();
      for(Root root:roots)if(root.canonical.exists())scan(root,root.canonical,leftovers,seen,new int[2]);
      if(!leftovers.isEmpty())throw rejected();
      writeState("complete",counts[0]);return new Result(counts[0],counts[1]);
    }catch(Exception failure){throw rejected();}
  }
  private static boolean databaseFamily(String name){return name.matches("OpenIM_v3_.+\\.db(?:-wal|-shm|-journal)?");}
  private static boolean excludedDirectory(String name){
    return name.equals("puty-local")||name.equals("no_backup")||name.equals("legacy-quarantine")||name.equals("puty-local-isolation");
  }
  private static final class Source {
    final File path;final String declared;final Entry entry;
    Source(File p,String d,Entry e){path=p;declared=d;entry=e;}
  }
  private void scan(Root root,File path,List<Source> output,Set<String> seen,int[] counts)throws Exception{
    if(!within(root.canonical,path)||!path.getCanonicalFile().equals(path.getAbsoluteFile()))throw rejected();
    Entry info=fs.stat(path);if(info.directory){
      if(!path.equals(root.canonical)&&excludedDirectory(path.getName()))return;
      File[] children=path.listFiles();if(children==null)throw rejected();Arrays.sort(children,(a,b)->a.getName().compareTo(b.getName()));
      for(File child:children)scan(root,child,output,seen,counts);
    }else if(info.regular&&info.links==1){
      if(databaseFamily(path.getName())){counts[1]++;return;}
      if(!seen.add(path.getAbsolutePath()))return;
      String relative=root.canonical.toURI().relativize(path.toURI()).getPath();
      String declared=new File(root.declared,relative).getAbsolutePath();
      if(utf8(declared).length>32768)throw rejected();output.add(new Source(path,declared,info));
    }else throw rejected(); // Includes symlinks, devices, sockets and hard links.
  }
  private void seal(Source source)throws Exception{
    String digest=hashSource(source),sourceID=hex(sha(utf8(source.declared))),id=sourceID+"-"+digest;
    File objects=new File(directory,"objects");ensureDirectory(objects);
    File versions=new File(objects,sourceID);ensureDirectory(versions);
    File data=new File(versions,digest+".data"),meta=new File(versions,digest+".meta");
    if(!data.exists())try(InputStream input=fs.read(source.path,source.entry)){writeSealed(data,input,id,"legacy-file",false);}
    hook.at("data-durable");
    verify(data,id,"legacy-file",source.entry.size,digest);
    String metadata="{\"source\":"+quote(source.declared)+",\"kind\":\"file\",\"account\":\"\",\"size\":"+source.entry.size+",\"sha256\":\""+digest+"\",\"createdAt\":"+System.currentTimeMillis()+"}";
    if(!meta.exists())writeSealed(meta,new ByteArrayInputStream(utf8(metadata)),id,"legacy-metadata",false);
    checkMeta(meta,id,source.declared,digest,source.entry.size);
    // Also required on a resumed receipt: its preceding process may have
    // renamed .meta successfully but failed its directory fsync. Existence and
    // authentication alone are not evidence that any directory entry is durable.
    fs.syncFile(data,fs.stat(data));fs.syncFile(meta,fs.stat(meta));
    fs.sync(versions);fs.sync(objects);fs.sync(directory);
    hook.at("metadata-durable");
    // No path-only unlink: stat identity plus a fresh full SHA protects changed
    // producers and replacement files. Source FD checks use O_NOFOLLOW/fstat.
    if(!digest.equals(hashSource(source)))throw rejected();
    hook.at("before-delete");
    if(!digest.equals(hashSource(source)))throw rejected();
    fs.remove(source.path,source.entry);fs.sync(source.path.getParentFile());hook.at("source-deleted");
  }
  private String hashSource(Source source)throws Exception{
    if(!source.entry.same(fs.stat(source.path)))throw rejected();
    try(InputStream input=fs.read(source.path,source.entry)){
      Hash body=hash(input);if(body.size!=source.entry.size||!source.entry.same(fs.stat(source.path)))throw rejected();return body.sha;
    }
  }
  private void checkMeta(File file,String id,String source,String digest,long size)throws Exception{
    byte[] bytes=decryptSmall(file,id,"legacy-metadata",MAX_META);
    try{
      Map<String,Object> meta=json(new String(bytes,StandardCharsets.UTF_8));
      if(meta.size()<5||meta.size()>6||!source.equals(meta.get("source"))||!"file".equals(meta.get("kind"))||!"".equals(meta.get("account"))||!digest.equals(meta.get("sha256"))||!Long.valueOf(size).equals(meta.get("size")))throw rejected();
      for(String key:meta.keySet())if(!Arrays.asList("source","kind","account","size","sha256","createdAt").contains(key))throw rejected();
    }finally{Arrays.fill(bytes,(byte)0);}
  }

  /** Recovery is deliberately package-private and additionally requires a
   * native attribution decision. No account/backup API may blindly call it.
   * The complete ciphertext is verified before the first byte is exposed. */
  interface RecoveryAuthority { boolean mayRecover(String declaredSource); }
  synchronized InputStream resolveStream(String declaredSource,String payloadSHA,RecoveryAuthority authority)throws Exception{
    checkOpen();if(authority==null||!authority.mayRecover(declaredSource)||!payloadSHA.matches("[0-9a-f]{64}"))throw rejected();
    File declared=new File(declaredSource);boolean owned=false;
    for(Root root:roots)if(within(root.declared,declared)&&declared.getAbsoluteFile().equals(declared)&&!declaredSource.contains("/../")&&!declaredSource.contains("/./"))owned=true;
    if(!owned||databaseFamily(declared.getName()))throw rejected();
    String sourceID=hex(sha(utf8(declaredSource))),id=sourceID+"-"+payloadSHA;
    File folder=new File(new File(directory,"objects"),sourceID),meta=new File(folder,payloadSHA+".meta"),data=new File(folder,payloadSHA+".data");
    byte[] raw=decryptSmall(meta,id,"legacy-metadata",MAX_META);long size;
    try{Map<String,Object> value=json(new String(raw,StandardCharsets.UTF_8));Object count=value.get("size");if(!(count instanceof Long)||(Long)count<0)throw rejected();size=(Long)count;}finally{Arrays.fill(raw,(byte)0);}
    checkMeta(meta,id,declaredSource,payloadSHA,size);verify(data,id,"legacy-file",size,payloadSHA);
    Entry entry=fs.stat(data);InputStream input=fs.read(data,entry);
    try{
      ObjectInput plain=new ObjectInput(input,master,id,"legacy-file");
      return new FilterInputStream(plain){
        private void authorized()throws IOException{
          synchronized(MobileLegacyQuarantine.this){if(MobileLegacyQuarantine.this.master==null||!authority.mayRecover(declaredSource)){try{close();}catch(IOException ignored){}throw new IOException("Legacy recovery authorization expired");}}
        }
        @Override public int read()throws IOException{byte[] one=new byte[1];return read(one,0,1)==-1?-1:one[0]&255;}
        @Override public int read(byte[] value,int at,int count)throws IOException{
          authorized();int n=in.read(value,at,count);
          try{authorized();return n;}catch(IOException denied){if(n>0)Arrays.fill(value,at,at+n,(byte)0);throw denied;}
        }
      };
    }catch(Exception failure){input.close();throw rejected();}
  }
  private void writeState(String state,int count)throws Exception{
    // Informational authenticated checkpoint. It never grants deletion rights.
    byte[] body=utf8("{\"format\":1,\"state\":\""+state+"\",\"sealedFiles\":"+count+"}");
    try{writeSealed(new File(directory,"migration.state"),new ByteArrayInputStream(body),"migration-state","legacy-state",true);}finally{Arrays.fill(body,(byte)0);}
  }
  private void verify(File file,String id,String type,long size,String digest)throws Exception{
    Entry entry=fs.stat(file);try(InputStream input=fs.read(file,entry);ObjectInput plain=new ObjectInput(input,master,id,type)){
      Hash body=hash(plain);if(body.size!=size||!body.sha.equals(digest)||!entry.same(fs.stat(file)))throw rejected();
    }
  }
  private byte[] decryptSmall(File file,String id,String type,int max)throws Exception{
    Entry entry=fs.stat(file);try(InputStream input=fs.read(file,entry);ObjectInput plain=new ObjectInput(input,master,id,type)){
      ByteArrayOutputStream output=new ByteArrayOutputStream();byte[] buffer=new byte[4096];
      try{for(int n;(n=plain.read(buffer))!=-1;){if(output.size()>max-n)throw rejected();output.write(buffer,0,n);}if(!entry.same(fs.stat(file)))throw rejected();return output.toByteArray();}
      finally{Arrays.fill(buffer,(byte)0);}
    }
  }
  private byte[] readSmall(File file,int max)throws Exception{
    Entry entry=fs.stat(file);if(!entry.regular||entry.links!=1||entry.size>max)throw rejected();
    try(InputStream input=fs.read(file,entry);ByteArrayOutputStream output=new ByteArrayOutputStream()){
      byte[] buffer=new byte[4096];for(int n;(n=input.read(buffer))!=-1;){if(output.size()>max-n)throw rejected();output.write(buffer,0,n);}return output.toByteArray();
    }
  }
  private void writeSealed(File file,InputStream plain,String id,String type,boolean replace)throws Exception{
    write(file,replace,output->encrypt(output,plain,master,id,type));
  }
  private void writeBytes(File file,byte[] bytes,boolean replace)throws Exception{write(file,replace,output->output.write(bytes));}
  interface Write { void run(OutputStream output)throws Exception; }
  private void write(File target,boolean replace,Write action)throws Exception{
    checkDirectory(target.getParentFile());if(target.exists()){Entry e=fs.stat(target);if(!replace||!e.regular||e.links!=1)throw rejected();}
    File temporary=new File(target.getParentFile(),".sealed-"+UUID.randomUUID());
    try{
      try(FileOutputStream output=fs.create(temporary)){action.run(output);output.flush();output.getFD().sync();}
      if(replace)fs.replace(temporary,target);else fs.moveNew(temporary,target);fs.sync(target.getParentFile());
    }finally{if(temporary.exists())fs.remove(temporary,fs.stat(temporary));}
  }
  private void ensureDirectory(File path)throws Exception{
    if(path.exists()){checkDirectory(path);return;}
    File parent=path.getParentFile();if(parent==null)throw rejected();ensureDirectory(parent);fs.mkdir(path);fs.sync(parent);checkDirectory(path);
  }
  private void checkDirectory(File path)throws Exception{
    if(!path.getCanonicalFile().equals(path.getAbsoluteFile())||!fs.stat(path).directory)throw rejected();
  }
  private static boolean within(File root,File child){String base=root.getAbsolutePath(),value=child.getAbsolutePath();return value.equals(base)||value.startsWith(base+File.separator);}
  private final class Lock implements AutoCloseable {
    final RandomAccessFile file;final java.nio.channels.FileLock held;
    Lock()throws Exception{
      File path=new File(directory,"migration.lock");
      if(!path.exists()){try(FileOutputStream ignored=fs.create(path)){}fs.sync(directory);}
      Entry entry=fs.stat(path);if(!entry.regular||entry.links!=1)throw rejected();
      file=new RandomAccessFile(path,"rw");held=file.getChannel().lock();
      if(!entry.same(fs.stat(path))){held.release();file.close();throw rejected();}
    }
    public void close()throws IOException{try{held.release();}finally{file.close();}}
  }
  private Lock lock()throws Exception{return new Lock();}

  private static byte[] utf8(String value){return value.getBytes(StandardCharsets.UTF_8);}
  private static byte[] sha(byte[] value)throws Exception{return MessageDigest.getInstance("SHA-256").digest(value);}
  private static String hex(byte[] bytes){char[] out=new char[bytes.length*2];char[] alphabet="0123456789abcdef".toCharArray();for(int i=0;i<bytes.length;i++){out[2*i]=alphabet[(bytes[i]&255)>>>4];out[2*i+1]=alphabet[bytes[i]&15];}return new String(out);}
  static final class Hash {final String sha;final long size;Hash(String s,long n){sha=s;size=n;}}
  private static Hash hash(InputStream input)throws Exception{
    MessageDigest digest=MessageDigest.getInstance("SHA-256");long size=0;byte[] buffer=new byte[65536];
    try{for(int n;(n=input.read(buffer))!=-1;){digest.update(buffer,0,n);if(size>Long.MAX_VALUE-n)throw rejected();size+=n;}return new Hash(hex(digest.digest()),size);}finally{Arrays.fill(buffer,(byte)0);}
  }
  private static byte[] key(byte[] master,String id,String type)throws Exception{
    if(master.length!=32||id.length()>512)throw rejected();
    Mac hmac=Mac.getInstance("HmacSHA256");hmac.init(new SecretKeySpec(utf8(FORMAT),"HmacSHA256"));byte[] prk=hmac.doFinal(master);
    try{hmac.init(new SecretKeySpec(prk,"HmacSHA256"));hmac.update(utf8(ACCOUNT+"\0"+type+"\0"+id));hmac.update((byte)1);return hmac.doFinal();}finally{Arrays.fill(prk,(byte)0);}
  }
  private static byte[] header(String id,String type,byte[] prefix){return utf8("{\"account\":\""+ACCOUNT+"\",\"chunkSize\":"+CHUNK+",\"format\":\""+FORMAT+"\",\"noncePrefix\":\""+base64(prefix)+"\",\"objectID\":"+quote(id)+",\"type\":"+quote(type)+"}");}
  private static byte[] crypt(int mode,byte[] key,byte[] header,byte[] prefix,long index,int length,byte[] body)throws Exception{
    if(index>0xffffffffL)throw rejected();byte[] nonce=new byte[12],aad=Arrays.copyOf(header,header.length+8);System.arraycopy(prefix,0,nonce,0,8);put(nonce,8,index);put(aad,header.length,index);put(aad,header.length+4,length);
    Cipher cipher=Cipher.getInstance("AES/GCM/NoPadding");cipher.init(mode,new SecretKeySpec(key,"AES"),new GCMParameterSpec(128,nonce));cipher.updateAAD(aad);return cipher.doFinal(body);
  }
  private static void put(byte[] bytes,int at,long n){for(int i=3;i>=0;i--){bytes[at+i]=(byte)n;n>>>=8;}}
  private static long uint(byte[] value){long n=0;for(byte b:value)n=(n<<8)|(b&255);return n;}
  private static void length(OutputStream output,int value)throws IOException{byte[] bytes=new byte[4];put(bytes,0,value);output.write(bytes);}
  static void encrypt(OutputStream output,InputStream input,byte[] master,String id,String type)throws Exception{
    byte[] key=key(master,id,type),prefix=new byte[8],buffer=new byte[CHUNK];new SecureRandom().nextBytes(prefix);byte[] header=header(id,type,prefix);
    try{output.write(MAGIC);length(output,header.length);output.write(header);long index=0;
      while(true){int count=0,n;while(count<buffer.length&&(n=input.read(buffer,count,buffer.length-count))!=-1){if(n==0)throw rejected();count+=n;}
        if(count!=0){length(output,count);byte[] plain=Arrays.copyOf(buffer,count);try{output.write(crypt(Cipher.ENCRYPT_MODE,key,header,prefix,index++,count,plain));}finally{Arrays.fill(plain,(byte)0);}}
        if(count<buffer.length){length(output,0);output.write(crypt(Cipher.ENCRYPT_MODE,key,header,prefix,index,0,new byte[0]));return;}
      }
    }finally{Arrays.fill(key,(byte)0);Arrays.fill(buffer,(byte)0);}
  }
  static final class ObjectInput extends InputStream {
    private final InputStream source;private final byte[] key,header,prefix;private byte[] plain=new byte[0];private int offset;private long index;private boolean eof,closed;
    ObjectInput(InputStream source,byte[] master,String id,String type)throws Exception{
      this.source=source;byte[] start=exact(source,12);if(!Arrays.equals(Arrays.copyOf(start,8),MAGIC))throw rejected();long count=uint(Arrays.copyOfRange(start,8,12));if(count<1||count>16384)throw rejected();
      header=exact(source,(int)count);Map<String,Object> value=json(new String(header,StandardCharsets.UTF_8));Object nonce=value.get("noncePrefix");if(!(nonce instanceof String))throw rejected();prefix=unbase64((String)nonce);
      if(prefix.length!=8||!Arrays.equals(header,header(id,type,prefix)))throw rejected();key=key(master,id,type);
    }
    public int read()throws IOException{byte[] one=new byte[1];return read(one,0,1)==-1?-1:one[0]&255;}
    public int read(byte[] buffer,int at,int count)throws IOException{
      if(at<0||count<0||at>buffer.length-count)throw new IndexOutOfBoundsException();if(closed)throw new IOException("Closed protected input");if(count==0)return 0;
      try{if(offset==plain.length){Arrays.fill(plain,(byte)0);plain=new byte[0];offset=0;if(eof)return -1;long length=uint(exact(source,4));if(length>CHUNK)throw rejected();byte[] sealed=exact(source,(int)length+16);plain=crypt(Cipher.DECRYPT_MODE,key,header,prefix,index++,(int)length,sealed);
          if(length==0){if(source.read()!=-1)throw rejected();eof=true;return -1;}}
        int n=Math.min(count,plain.length-offset);System.arraycopy(plain,offset,buffer,at,n);offset+=n;return n;
      }catch(Exception failure){throw new IOException("Invalid protected object");}
    }
    public void close()throws IOException{closed=true;Arrays.fill(key,(byte)0);Arrays.fill(plain,(byte)0);source.close();}
  }
  private static byte[] exact(InputStream input,int count)throws Exception{byte[] bytes=new byte[count];int position=0;while(position<count){int n=input.read(bytes,position,count-position);if(n<=0)throw rejected();position+=n;}return bytes;}
  private static String base64(byte[] bytes){String alphabet="ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";StringBuilder out=new StringBuilder();for(int i=0;i<bytes.length;i+=3){int n=(bytes[i]&255)<<16|(i+1<bytes.length?(bytes[i+1]&255)<<8:0)|(i+2<bytes.length?bytes[i+2]&255:0);out.append(alphabet.charAt(n>>>18)).append(alphabet.charAt(n>>>12&63)).append(i+1<bytes.length?alphabet.charAt(n>>>6&63):'=').append(i+2<bytes.length?alphabet.charAt(n&63):'=');}return out.toString();}
  private static byte[] unbase64(String text)throws Exception{if(!text.matches("[A-Za-z0-9+/]{11}="))throw rejected();String a="ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";byte[] out=new byte[8];int bits=0,buffer=0,at=0;for(char c:text.toCharArray()){if(c=='=')break;buffer=(buffer<<6)|a.indexOf(c);bits+=6;if(bits>=8){bits-=8;out[at++]=(byte)(buffer>>>bits);}}if(!base64(out).equals(text))throw rejected();return out;}
  private static String quote(String value){StringBuilder out=new StringBuilder("\"");for(char c:value.toCharArray()){switch(c){case '\"':out.append("\\\"");break;case '\\':out.append("\\\\");break;default:if(c<32||c=='<'||c=='>'||c=='&'||c=='\u2028'||c=='\u2029')out.append(String.format(Locale.ROOT,"\\u%04x",(int)c));else out.append(c);}}return out.append('"').toString();}
  /** Bounded metadata parser; duplicate keys, nested values and trailing data
   * are invalid. It does not depend on Android's stubbed JVM JSON classes. */
  private static Map<String,Object> json(String text)throws Exception{
    if(text.length()>MAX_META)throw rejected();Json input=new Json(text);Map<String,Object> out=new LinkedHashMap<>();input.token('{');input.space();if(input.peek()!='}')while(true){String name=input.string();input.token(':');input.space();Object value;if(input.peek()=='"')value=input.string();else{int start=input.at;if(input.peek()=='-')input.at++;while(input.at<text.length()&&Character.isDigit(text.charAt(input.at)))input.at++;try{value=Long.valueOf(text.substring(start,input.at));}catch(Exception failure){throw rejected();}}if(out.put(name,value)!=null)throw rejected();input.space();if(input.peek()!=',')break;input.at++;}input.token('}');input.space();if(input.at!=text.length())throw rejected();return out;
  }
  private static final class Json {
    final String text;int at;Json(String value){text=value;}void space(){while(at<text.length()&&" \r\n\t".indexOf(text.charAt(at))>=0)at++;}char peek()throws Exception{space();if(at>=text.length())throw rejected();return text.charAt(at);}void token(char c)throws Exception{if(peek()!=c)throw rejected();at++;}
    String string()throws Exception{token('"');StringBuilder out=new StringBuilder();while(at<text.length()){char c=text.charAt(at++);if(c=='"')return out.toString();if(c<32)throw rejected();if(c=='\\'){if(at>=text.length())throw rejected();c=text.charAt(at++);switch(c){case '"':case '\\':case '/':break;case 'b':c='\b';break;case 'f':c='\f';break;case 'n':c='\n';break;case 'r':c='\r';break;case 't':c='\t';break;case 'u':if(at+4>text.length())throw rejected();try{c=(char)Integer.parseInt(text.substring(at,at+4),16);}catch(Exception failure){throw rejected();}at+=4;break;default:throw rejected();}}out.append(c);}throw rejected();}
  }

  private static final class AndroidKeys implements KeyProvider {
    private static final String ALIAS="puty.legacy.quarantine.v1";
    private SecretKey key()throws Exception{KeyStore store=KeyStore.getInstance("AndroidKeyStore");store.load(null);if(!store.containsAlias(ALIAS)){KeyGenerator generator=KeyGenerator.getInstance("AES","AndroidKeyStore");generator.init(new KeyGenParameterSpec.Builder(ALIAS,KeyProperties.PURPOSE_ENCRYPT|KeyProperties.PURPOSE_DECRYPT).setKeySize(256).setBlockModes(KeyProperties.BLOCK_MODE_GCM).setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE).setRandomizedEncryptionRequired(true).build());generator.generateKey();}return (SecretKey)store.getKey(ALIAS,null);}
    public byte[] wrap(byte[] value)throws Exception{Cipher cipher=Cipher.getInstance("AES/GCM/NoPadding");cipher.init(Cipher.ENCRYPT_MODE,key());cipher.updateAAD(utf8("master\0"+ACCOUNT));byte[] body=cipher.doFinal(value);ByteArrayOutputStream output=new ByteArrayOutputStream();output.write(utf8("PUTYOS1\n"));output.write(cipher.getIV());output.write(body);return output.toByteArray();}
    public byte[] unwrap(byte[] value)throws Exception{if(value.length!=68||!Arrays.equals(Arrays.copyOf(value,8),utf8("PUTYOS1\n")))throw rejected();Cipher cipher=Cipher.getInstance("AES/GCM/NoPadding");cipher.init(Cipher.DECRYPT_MODE,key(),new GCMParameterSpec(128,Arrays.copyOfRange(value,8,20)));cipher.updateAAD(utf8("master\0"+ACCOUNT));return cipher.doFinal(value,20,value.length-20);}
  }
  private static final class AndroidFs implements Fs {
    private void canonical(File path)throws Exception{if(!path.getCanonicalFile().equals(path.getAbsoluteFile()))throw rejected();}
    private Entry entry(StructStat s){return new Entry(s.st_dev,s.st_ino,s.st_size,s.st_mtime,OsConstants.S_ISREG(s.st_mode),OsConstants.S_ISDIR(s.st_mode),s.st_nlink);}
    public Entry stat(File file)throws Exception{canonical(file);return entry(Os.lstat(file.getAbsolutePath()));}
    public InputStream read(File path,Entry expected)throws Exception{canonical(path);FileDescriptor fd=Os.open(path.getAbsolutePath(),OsConstants.O_RDONLY|OsConstants.O_NOFOLLOW|OsConstants.O_CLOEXEC,0);try{if(!expected.regular||expected.links!=1||!expected.same(entry(Os.fstat(fd))))throw rejected();return new FileInputStream(fd);}catch(Exception failure){Os.close(fd);throw failure;}}
    public FileOutputStream create(File path)throws Exception{canonical(path);return new FileOutputStream(Os.open(path.getAbsolutePath(),OsConstants.O_WRONLY|OsConstants.O_CREAT|OsConstants.O_EXCL|OsConstants.O_NOFOLLOW|OsConstants.O_CLOEXEC,0600));}
    public void mkdir(File path)throws Exception{canonical(path);Os.mkdir(path.getAbsolutePath(),0700);}
    public void moveNew(File from,File to)throws Exception{canonical(from);canonical(to);if(to.exists())throw rejected();Os.rename(from.getAbsolutePath(),to.getAbsolutePath());}
    public void replace(File from,File to)throws Exception{canonical(from);canonical(to);if(to.exists()){Entry info=stat(to);if(!info.regular||info.links!=1)throw rejected();}Os.rename(from.getAbsolutePath(),to.getAbsolutePath());}
    public void sync(File path)throws Exception{canonical(path);FileDescriptor fd=Os.open(path.getAbsolutePath(),OsConstants.O_RDONLY|OsConstants.O_NOFOLLOW|OsConstants.O_CLOEXEC,0);try{if(!OsConstants.S_ISDIR(Os.fstat(fd).st_mode))throw rejected();Os.fsync(fd);}finally{Os.close(fd);}}
    public void syncFile(File path,Entry expected)throws Exception{canonical(path);FileDescriptor fd=Os.open(path.getAbsolutePath(),OsConstants.O_RDONLY|OsConstants.O_NOFOLLOW|OsConstants.O_CLOEXEC,0);try{if(!expected.regular||expected.links!=1||!expected.same(entry(Os.fstat(fd))))throw rejected();Os.fsync(fd);}finally{Os.close(fd);}}
    public void remove(File path,Entry expected)throws Exception{if(!expected.regular||expected.links!=1||!expected.same(stat(path)))throw rejected();Os.remove(path.getAbsolutePath());}
  }
}
