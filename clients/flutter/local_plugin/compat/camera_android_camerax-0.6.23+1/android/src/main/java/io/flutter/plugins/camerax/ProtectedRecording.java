package io.flutter.plugins.camerax;

import android.os.ParcelFileDescriptor;
import androidx.camera.video.PendingRecording;
import androidx.camera.video.Recording;
import java.io.IOException;
import java.util.IdentityHashMap;
import java.util.Map;
import java.util.concurrent.atomic.AtomicBoolean;

/** Descriptor ownership follows CameraX through final muxer flush. No path is
 * reopened, and an online-vault lock cancels the actual active producer. */
final class ProtectedRecording {
 private static final Map<PendingRecording,ProtectedRecording> pending=new IdentityHashMap<>();
 final ParcelFileDescriptor descriptor;
 private final AtomicBoolean closed=new AtomicBoolean();
 private volatile Recording recording;
 private String registration;
 private ProtectedRecording(String path)throws Exception{
  if(android.os.Build.VERSION.SDK_INT<26)throw new IOException("Encrypted capture requires Android 8.0 or later");
  Class<?> bridge=Class.forName("io.openim.flutter_openim_sdk.EncryptedScratchFiles");
  descriptor=(ParcelFileDescriptor)bridge.getMethod("descriptor",String.class).invoke(null,path);
  try{registration=(String)bridge.getMethod("registerConsumer",Runnable.class).invoke(null,(Runnable)this::cancel);}catch(Exception rejected){cancel();throw rejected;}
 }
 static ProtectedRecording open(String path){try{return new ProtectedRecording(path);}catch(Exception rejected){throw new IllegalStateException("Encrypted recording unavailable",rejected);}}
 static synchronized void put(PendingRecording p,ProtectedRecording r){pending.put(p,r);}
 static synchronized void transfer(PendingRecording old,PendingRecording next){ProtectedRecording r=pending.remove(old);if(r!=null)pending.put(next,r);}
 static synchronized ProtectedRecording take(PendingRecording p){ProtectedRecording r=pending.remove(p);if(r==null||r.closed.get())throw new IllegalStateException("Encrypted recording lease ended");return r;}
 void bind(Recording value){recording=value;if(closed.get())value.stop();}
 void cancel(){synchronized(ProtectedRecording.class){pending.values().removeIf(value->value==this);}if(closed.compareAndSet(false,true)){Recording r=recording;if(r!=null)try{r.stop();}catch(RuntimeException ignored){}try{descriptor.close();}catch(IOException ignored){}}}
 void finish(){cancel();String token=registration;registration=null;if(token!=null)try{Class.forName("io.openim.flutter_openim_sdk.EncryptedScratchFiles").getMethod("unregisterConsumer",String.class).invoke(null,token);}catch(Exception ignored){}}
}
