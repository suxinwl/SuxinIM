package io.openim.flutter_openim_sdk;

import android.content.Context;
import android.os.Build;
import android.os.Handler;
import android.os.HandlerThread;
import android.os.ParcelFileDescriptor;
import android.os.ProxyFileDescriptorCallback;
import android.os.storage.StorageManager;
import android.system.ErrnoException;
import android.system.OsConstants;
import android.system.Os;
import java.io.File;
import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.io.FileOutputStream;
import java.io.ByteArrayOutputStream;
import java.util.UUID;
import java.util.ArrayList;
import android.net.Uri;
import java.util.Arrays;
import java.util.HashMap;
import java.util.Map;
import open_im_sdk.Open_im_sdk;

/** Bounded encrypted random-access scratch for native media producers. No
 * plaintext fallback: Android 24/25 media producers report unsupported until a
 * separately verified virtual descriptor implementation is available. */
public final class EncryptedScratchFiles {
  public interface Authority { String authorizedAccount(); long generation(); }
  private static volatile Context context;
  private static volatile Authority authority;
  private static final long MAX_SIZE=2L*1024*1024*1024;
  // Never give a second scratch the same procfs path in this process. A stale
  // asynchronous media result must not stage/discard a newer account's handle.
  private static int nextDescriptor=64;
  private static final Map<String,Handle> handles=new HashMap<>();
  private static final Map<String,Runnable> cancellations=new HashMap<>();
  private static final Map<String,PhotoCapture> photos=new HashMap<>();
  private static final HandlerThread worker=new HandlerThread("puty-encrypted-vfs");
  static {worker.start();}
  private EncryptedScratchFiles(){}
  public static void initialize(Context app,Authority value){ArrayList<Handle> closing;ArrayList<Runnable> cancel;ArrayList<PhotoCapture> closingPhotos;synchronized(EncryptedScratchFiles.class){context=app.getApplicationContext();authority=value;closing=new ArrayList<>(handles.values());cancel=new ArrayList<>(cancellations.values());closingPhotos=new ArrayList<>(photos.values());handles.clear();cancellations.clear();photos.clear();for(Handle handle:closing)handle.closed=true;for(PhotoCapture photo:closingPhotos)photo.closed=true;}finishClose(closing,cancel);for(PhotoCapture photo:closingPhotos)discardPhotoSource(photo,photo.source);}
  private static final class PhotoCapture {
    final String account,name;final long generation;final Authority owner;boolean staging;volatile boolean closed;String source;
    PhotoCapture(String account,String name,long generation,Authority owner){this.account=account;this.name=name;this.generation=generation;this.owner=owner;}
    boolean allowed(){Authority current=authority;return !closed&&current==owner&&current!=null&&generation==current.generation()&&account.equals(current.authorizedAccount())&&generation==current.generation()&&!closed&&current==authority;}
  }
  /** A photo already arrives in bounded memory. Bind it before capture without
   * allocating a SQLCipher scratch/FUSE file, then make one durable encrypted
   * outgoing write. The token never grants access to a different actor. */
  public static synchronized String beginPhotoCapture(String name)throws IOException {
    if(Build.VERSION.SDK_INT<26)throw new IOException("Encrypted media capture requires Android 8 or newer");
    Authority current=authority;long generation=current==null?-1:current.generation();String account=current==null?null:current.authorizedAccount();
    if(context==null||account==null||account.isEmpty()||photos.size()>=64)throw new IOException("Encrypted camera unavailable");
    if(!"CAP.jpg".equals(name))throw new IOException("Invalid encrypted photo name");
    PhotoCapture photo=new PhotoCapture(account,name,generation,current);if(!photo.allowed())throw new IOException("Encrypted camera identity changed");
    String token=UUID.randomUUID().toString();photos.put(token,photo);return token;
  }
  public static void stagePhotoCapture(String token,byte[] bytes)throws IOException {
    PhotoCapture photo=null;String source=null;boolean accepted=false,started=false;
    try{
      synchronized(EncryptedScratchFiles.class){photo=photos.get(token);if(photo==null||!photo.allowed()||photo.staging||bytes==null||bytes.length==0||bytes.length>(32<<20))throw new IOException("Encrypted photo rejected");photo.staging=true;started=true;}
      // No class monitor during native IO: revocation/cancellation can mark the
      // binding closed while fsync/SQL commit is in progress. Native code also
      // checks its live lease throughout the encrypted write.
      source=Open_im_sdk.stageNativeBytes(photo.account,photo.name,bytes);
      synchronized(EncryptedScratchFiles.class){if(source==null||source.isEmpty()||photos.get(token)!=photo||!photo.allowed())throw new IOException("Encrypted photo import rejected");photo.source=source;accepted=true;}
    }finally{
      if(bytes!=null)Arrays.fill(bytes,(byte)0);
      if(started&&!accepted){synchronized(EncryptedScratchFiles.class){if(photos.get(token)==photo){photos.remove(token);photo.closed=true;}}discardPhotoSource(photo,source);}
    }
  }
  /** Transfer a completed source only after a final current-actor check. */
  public static String finishPhotoCapture(String token)throws IOException {
    PhotoCapture photo;String source;boolean accepted;
    synchronized(EncryptedScratchFiles.class){photo=photos.get(token);if(photo==null)throw new IOException("Encrypted photo unavailable");accepted=photo.allowed()&&photo.source!=null&&!photo.source.isEmpty();photos.remove(token);photo.closed=true;source=photo.source;photo.source=null;}
    if(!accepted){discardPhotoSource(photo,source);throw new IOException("Encrypted photo identity changed");}return source;
  }
  public static void discardPhotoCapture(String token){PhotoCapture photo;String source;synchronized(EncryptedScratchFiles.class){photo=photos.remove(token);if(photo==null)return;photo.closed=true;source=photo.source;photo.source=null;}discardPhotoSource(photo,source);}
  private static void discardPhotoSource(PhotoCapture photo,String source){if(source!=null&&!source.isEmpty())try{Open_im_sdk.discardNativeSource(photo.account,source);}catch(RuntimeException|LinkageError ignored){/* An already revoked native store must not be reopened for cleanup. */}}
  private static final class Handle {
    final String account,id,name,previewID=UUID.randomUUID().toString();final long generation;final Authority owner;final ArrayList<ParcelFileDescriptor> consumers=new ArrayList<>();ParcelFileDescriptor descriptor;volatile boolean closed;
    Handle(String account,String id,String name,long generation,Authority owner){this.account=account;this.id=id;this.name=name;this.generation=generation;this.owner=owner;}
    boolean current(){Authority current=authority;return current==owner&&current!=null&&generation==current.generation()&&account.equals(current.authorizedAccount())&&generation==current.generation();}
    boolean allowed(){return !closed&&current();}
    void check()throws ErrnoException{if(!allowed())throw new ErrnoException("protected-scratch",OsConstants.EACCES);}
  }
  private static void range(long offset,int size,byte[] bytes)throws ErrnoException {
    if(bytes==null||offset<0||size<0||size>bytes.length||offset>MAX_SIZE||size>MAX_SIZE-offset)throw new ErrnoException("protected-range",OsConstants.EINVAL);
  }
  private static void closeDescriptor(Handle handle){for(ParcelFileDescriptor consumer:handle.consumers)try{consumer.close();}catch(IOException ignored){}handle.consumers.clear();if(handle.descriptor!=null)try{handle.descriptor.close();}catch(IOException ignored){}}
  private static ParcelFileDescriptor retain(Handle handle,ParcelFileDescriptor descriptor)throws IOException {
    handle.consumers.removeIf(value->{try{return !value.getFileDescriptor().valid();}catch(RuntimeException closed){return true;}});
    if(handle.consumers.size()>=64){descriptor.close();throw new IOException("Encrypted scratch consumer limit");}
    handle.consumers.add(descriptor);return descriptor;
  }
  private static void directIO(ParcelFileDescriptor descriptor)throws IOException {
    // AppFuse's default page cache can serve an old page after actor revocation
    // without another callback. Require uncached IO before exposing any FD.
    // O_DIRECT is absent from the public Java OsConstants API. Values come from
    // the pinned NDK's {aarch64,arm}-linux-android/asm/fcntl.h and asm-generic.
    String arch=System.getProperty("os.arch","");int direct;
    if(arch.equals("aarch64")||arch.equals("arm64")||arch.startsWith("armv")||arch.equals("arm"))direct=0200000;
    else if(arch.equals("x86_64")||arch.equals("amd64")||arch.matches("i[3-6]86")||arch.equals("x86"))direct=00040000;
    else throw new IOException("Encrypted descriptor architecture unsupported");
    try{int flags=Os.fcntlInt(descriptor.getFileDescriptor(),OsConstants.F_GETFL,0);Os.fcntlInt(descriptor.getFileDescriptor(),OsConstants.F_SETFL,flags|direct);if((Os.fcntlInt(descriptor.getFileDescriptor(),OsConstants.F_GETFL,0)&direct)==0)throw new IOException("Encrypted descriptor requires uncached IO");}catch(ErrnoException denied){throw new IOException("Encrypted descriptor requires uncached IO");}
  }
  public static synchronized File createTempFile(String prefix,String suffix)throws IOException {
    if(Build.VERSION.SDK_INT<26)throw new IOException("Encrypted media capture requires Android 8 or newer");
    Authority current=authority;String account=current==null?null:current.authorizedAccount();
    if(account==null||account.isEmpty()||context==null)throw new IOException("Encrypted media requires current online authorization");
    long generation=current.generation();
    String name=(prefix==null?"media":prefix)+(suffix==null?".bin":suffix);
    if(name.length()>255||name.isEmpty()||name.indexOf('/')>=0||name.indexOf('\\')>=0)throw new IOException("Invalid encrypted scratch name");
    for(int i=0;i<name.length();i++)if(Character.isISOControl(name.charAt(i)))throw new IOException("Invalid encrypted scratch name");
    if(nextDescriptor==Integer.MAX_VALUE)throw new IOException("Encrypted descriptor limit reached");
    String id=Open_im_sdk.beginNativeScratch(account,name);
    if(id==null||id.isEmpty())throw new IOException("Encrypted scratch allocation rejected");
    Handle handle=new Handle(account,id,name,generation,current);
    try {
      handle.check();handle.descriptor=Api26.open(context,ParcelFileDescriptor.MODE_READ_WRITE,handle);
      directIO(handle.descriptor);
      int fd=Os.fcntlInt(handle.descriptor.getFileDescriptor(),OsConstants.F_DUPFD_CLOEXEC,nextDescriptor);
      ParcelFileDescriptor unique=ParcelFileDescriptor.adoptFd(fd);
      ParcelFileDescriptor original=handle.descriptor;handle.descriptor=unique;
      nextDescriptor=fd==Integer.MAX_VALUE?Integer.MAX_VALUE:fd+1;
      original.close();handle.check();
      String path="/proc/self/fd/"+handle.descriptor.getFd();handles.put(path,handle);return new File(path);
    }catch(IOException|RuntimeException|ErrnoException failure){handle.closed=true;closeDescriptor(handle);Open_im_sdk.closeNativeScratch(account,id);throw new IOException("Encrypted scratch descriptor unavailable");}
  }
  // Keep API26-only types out of the outer class's method signatures so loading
  // the SDK on API24/25 can report unsupported media without a verifier failure.
  private static final class Api26 {
    static ParcelFileDescriptor open(Context context,int mode,Handle handle)throws IOException {
      StorageManager storage=(StorageManager)context.getSystemService(Context.STORAGE_SERVICE);if(storage==null)throw new IOException("Encrypted descriptor service unavailable");
      return storage.openProxyFileDescriptor(mode,callback(handle),new Handler(worker.getLooper()));
    }
    private static ProxyFileDescriptorCallback callback(Handle handle){final String account=handle.account,id=handle.id;return new ProxyFileDescriptorCallback(){
        @Override public long onGetSize()throws ErrnoException{handle.check();long size=Open_im_sdk.nativeScratchSize(account,id);handle.check();if(size<0||size>MAX_SIZE)throw new ErrnoException("protected-size",OsConstants.EIO);return size;}
        @Override public int onRead(long offset,int size,byte[] output)throws ErrnoException{
          handle.check();range(offset,size,output);long length=Open_im_sdk.nativeScratchSize(account,id);handle.check();if(length<0||length>MAX_SIZE)throw new ErrnoException("protected-read",OsConstants.EIO);if(size==0||offset>=length)return 0;
          // A short FUSE read is EOF to the kernel. Fill the complete requested
          // span using bounded SQLCipher pages, rather than returning one page.
          int required=(int)Math.min(size,length-offset),done=0;
          try{while(done<required){int request=Math.min(65536,required-done);handle.check();byte[] bytes=Open_im_sdk.readNativeScratch(account,id,offset+done,request);if(bytes==null)throw new ErrnoException("protected-read",OsConstants.EIO);try{handle.check();if(bytes.length!=request)throw new ErrnoException("protected-read",OsConstants.EIO);System.arraycopy(bytes,0,output,done,bytes.length);done+=bytes.length;}finally{Arrays.fill(bytes,(byte)0);}}handle.check();return done;}catch(ErrnoException denied){Arrays.fill(output,(byte)0);throw denied;}
        }
        @Override public int onWrite(long offset,int size,byte[] input)throws ErrnoException{handle.check();range(offset,size,input);int written=0;while(written<size){int count=Math.min(65536,size-written);byte[] bytes=Arrays.copyOfRange(input,written,written+count);try{handle.check();if(!Open_im_sdk.writeNativeScratch(account,id,offset+written,bytes))throw new ErrnoException("protected-write",OsConstants.EIO);handle.check();}finally{Arrays.fill(bytes,(byte)0);}written+=count;}return written;}
        @Override public void onFsync()throws ErrnoException{handle.check();}
        @Override public void onRelease(){/* Ownership ends only at stage/discard; a producer may close and reopen its descriptor. */}
      };
    }
  }
  public static synchronized boolean owns(String path){return handles.containsKey(path);}
  /** The returned one-use ID prevents a late completion unregistering a newer
   * camera/encoder. Register after creating the producer and stop it on failure. */
  public static String registerConsumer(Runnable cancel)throws IOException {
    if(cancel==null)throw new IOException("Missing encrypted media cancellation");String id=null;
    synchronized(EncryptedScratchFiles.class){Authority current=authority;long generation=current==null?-1:current.generation();String account=current==null?null:current.authorizedAccount();if(account!=null&&!account.isEmpty()&&current==authority&&generation==current.generation()&&account.equals(current.authorizedAccount())&&generation==current.generation()&&cancellations.size()<64){id=UUID.randomUUID().toString();cancellations.put(id,cancel);}}
    if(id==null){try{cancel.run();}catch(RuntimeException ignored){}throw new IOException("Encrypted media requires current online authorization");}return id;
  }
  public static synchronized void unregisterConsumer(String registration){cancellations.remove(registration);}
  private static Handle requireHandle(String path)throws IOException {Handle handle=handles.get(path);if(handle==null||!handle.allowed())throw new IOException("Encrypted scratch identity changed");return handle;}
  /** The procfs path is an opaque handle identity, not a reopenable filename.
   * Android SELinux rejects FileOutputStream(path); producers use these APIs. */
  public static synchronized ParcelFileDescriptor descriptor(String path)throws IOException {
    Handle handle=requireHandle(path);ParcelFileDescriptor duplicate=ParcelFileDescriptor.dup(handle.descriptor.getFileDescriptor());
    if(!handle.allowed()){duplicate.close();throw new IOException("Encrypted scratch identity changed");}return retain(handle,duplicate);
  }
  public static OutputStream outputStream(String path)throws IOException{return new ParcelFileDescriptor.AutoCloseOutputStream(descriptor(path));}
  public static synchronized String name(String path)throws IOException{return requireHandle(path).name;}
  public static synchronized long size(String path)throws IOException {Handle handle=requireHandle(path);long value=Open_im_sdk.nativeScratchSize(handle.account,handle.id);if(!handle.allowed()||value<0||value>MAX_SIZE)throw new IOException("Encrypted scratch size unavailable");return value;}
  public static synchronized InputStream inputStream(String path)throws IOException {
    final Handle handle=requireHandle(path);
    return new InputStream(){long offset;boolean closed;
      private void check()throws IOException{if(closed||!handle.allowed())throw new IOException("Encrypted scratch identity changed");}
      @Override public int read()throws IOException{byte[] one=new byte[1];int count=read(one,0,1);int result=count<0?-1:one[0]&255;Arrays.fill(one,(byte)0);return result;}
      @Override public int read(byte[] output,int start,int count)throws IOException{
        if(output==null)throw new NullPointerException();if(start<0||count<0||count>output.length-start)throw new IndexOutOfBoundsException();check();if(count==0)return 0;
        long length=Open_im_sdk.nativeScratchSize(handle.account,handle.id);check();if(length<0||length>MAX_SIZE)throw new IOException("Encrypted scratch size unavailable");if(offset>=length)return -1;
        int request=(int)Math.min(Math.min(count,65536),length-offset);byte[] bytes=Open_im_sdk.readNativeScratch(handle.account,handle.id,offset,request);
        if(bytes==null)throw new IOException("Encrypted scratch read rejected");try{check();if(bytes.length>request)throw new IOException("Encrypted scratch read invalid");if(bytes.length==0)return -1;System.arraycopy(bytes,0,output,start,bytes.length);offset+=bytes.length;return bytes.length;}finally{Arrays.fill(bytes,(byte)0);}
      }
      @Override public long skip(long count)throws IOException{check();long current=Open_im_sdk.nativeScratchSize(handle.account,handle.id);check();if(current<0)throw new IOException("Encrypted scratch size unavailable");long skipped=Math.min(Math.max(0,count),Math.max(0,current-offset));offset+=skipped;return skipped;}
      @Override public void close(){closed=true;}
    };
  }
  public static byte[] readBytes(String path,long maximum)throws IOException {
    if(maximum<0||maximum>32*1024*1024)throw new IOException("Encrypted scratch memory limit");long expected=size(path);if(expected>maximum)throw new IOException("Encrypted scratch memory limit");
    try(InputStream input=inputStream(path);ByteArrayOutputStream output=new ByteArrayOutputStream((int)expected)){byte[] buffer=new byte[65536];try{for(int count;(count=input.read(buffer))!=-1;){if(output.size()+count>maximum)throw new IOException("Encrypted scratch memory limit");output.write(buffer,0,count);}if(output.size()!=expected||size(path)!=expected)throw new IOException("Encrypted scratch changed during read");return output.toByteArray();}finally{Arrays.fill(buffer,(byte)0);}}
  }
  public static synchronized String previewUri(String path)throws IOException {Handle handle=requireHandle(path);return new Uri.Builder().scheme("content").authority(context.getPackageName()+".puty-encrypted-scratch").appendPath(handle.previewID).build().toString();}
  private static Handle previewHandle(Uri uri)throws IOException {
    if(context==null||uri==null||!"content".equals(uri.getScheme())||!(context.getPackageName()+".puty-encrypted-scratch").equals(uri.getAuthority())||uri.getQuery()!=null||uri.getFragment()!=null||uri.getPathSegments().size()!=1)throw new IOException("Encrypted scratch preview rejected");
    String identity=uri.getLastPathSegment();for(Handle handle:handles.values())if(handle.previewID.equals(identity)&&handle.allowed())return handle;throw new IOException("Encrypted scratch preview unavailable");
  }
  static synchronized String previewName(Uri uri)throws IOException{return previewHandle(uri).name;}
  static synchronized long previewSize(Uri uri)throws IOException{Handle handle=previewHandle(uri);long value=Open_im_sdk.nativeScratchSize(handle.account,handle.id);if(!handle.allowed()||value<0||value>MAX_SIZE)throw new IOException("Encrypted scratch preview unavailable");return value;}
  static synchronized ParcelFileDescriptor openPreview(Uri uri)throws IOException {
    if(Build.VERSION.SDK_INT<26)throw new IOException("Encrypted media capture requires Android 8 or newer");Handle handle=previewHandle(uri);
    ParcelFileDescriptor descriptor=Api26.open(context,ParcelFileDescriptor.MODE_READ_ONLY,handle);try{directIO(descriptor);if(!handle.allowed())throw new IOException("Encrypted scratch identity changed");return retain(handle,descriptor);}catch(IOException denied){descriptor.close();throw denied;}
  }
  public static String importUri(Context context,Uri uri,String sourceName)throws IOException {
    if(context==null||uri==null||!"content".equals(uri.getScheme())||sourceName==null||sourceName.isEmpty())throw new IOException("Invalid selected document");
    int dot=sourceName.lastIndexOf('.');String prefix=dot>0?sourceName.substring(0,dot):sourceName,suffix=dot>0?sourceName.substring(dot):".bin";
    File target=createTempFile(prefix,suffix);boolean complete=false;
    try(InputStream input=context.getContentResolver().openInputStream(uri);OutputStream output=outputStream(target.getAbsolutePath())){
      if(input==null)throw new IOException("Selected document unavailable");byte[] buffer=new byte[65536];long total=0;
      try{for(int n;(n=input.read(buffer))!=-1;){total+=n;if(total>1073741824L)throw new IOException("Selected document exceeds encrypted limit");output.write(buffer,0,n);}output.flush();}finally{Arrays.fill(buffer,(byte)0);}complete=true;return target.getAbsolutePath();
    }finally{if(!complete)discard(target.getAbsolutePath());}
  }
  public static synchronized String stage(String path)throws IOException {
    Handle handle=handles.get(path);if(handle==null||!handle.allowed())throw new IOException("Encrypted scratch identity changed");
    String result=Open_im_sdk.stageNativeScratch(handle.account,handle.id);if(result==null||result.isEmpty())throw new IOException("Encrypted scratch import failed");
    boolean current=handle.allowed();handles.remove(path);handle.closed=true;closeDescriptor(handle);
    if(!current||!handle.current())throw new IOException("Encrypted scratch identity changed");return result;
  }
  public static synchronized void discard(String path)throws IOException{Handle handle=handles.get(path);if(handle==null)return;if(!handle.allowed())throw new IOException("Encrypted scratch identity changed");handles.remove(path);handle.closed=true;closeDescriptor(handle);Open_im_sdk.closeNativeScratch(handle.account,handle.id);if(!handle.current())throw new IOException("Encrypted scratch identity changed");}
  private static void finishClose(ArrayList<Handle> closing,ArrayList<Runnable> cancel){for(Handle handle:closing)closeDescriptor(handle);for(Runnable action:cancel)try{action.run();}catch(RuntimeException ignored){}for(Handle handle:closing)Open_im_sdk.closeNativeScratch(handle.account,handle.id);}
  public static void closeAll(){ArrayList<Handle> closing;ArrayList<Runnable> cancel;ArrayList<PhotoCapture> closingPhotos;synchronized(EncryptedScratchFiles.class){closing=new ArrayList<>(handles.values());cancel=new ArrayList<>(cancellations.values());closingPhotos=new ArrayList<>(photos.values());handles.clear();cancellations.clear();photos.clear();for(Handle handle:closing)handle.closed=true;for(PhotoCapture photo:closingPhotos)photo.closed=true;}finishClose(closing,cancel);for(PhotoCapture photo:closingPhotos)discardPhotoSource(photo,photo.source);}
}
