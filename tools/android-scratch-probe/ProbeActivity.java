package com.puty.test.encryptedscratch;

import android.app.Activity;
import android.os.Bundle;
import android.util.Log;
import io.openim.flutter_openim_sdk.EncryptedScratchFiles;
import java.io.*;
import java.util.Arrays;
import java.util.concurrent.atomic.AtomicLong;
import open_im_sdk.Open_im_sdk;

/** Standalone offline APK, never included in a production application. */
public final class ProbeActivity extends Activity {
  private final String account="scratch_probe_synthetic_only";
  private final AtomicLong generation=new AtomicLong(1);
  private volatile boolean allowed=true;
  private String phase="start";
  private void require(boolean value){if(!value)throw new AssertionError("synthetic assertion");}
  private void pass(String name){Log.i("PutyScratchProbe","PASS "+name);}
  @Override public void onCreate(Bundle state){super.onCreate(state);new Thread(()->{
    try{run();Log.i("PutyScratchProbe","COMPLETE");}
    catch(Throwable failure){String errno="";for(Throwable cause=failure;cause!=null;cause=cause.getCause())if(cause instanceof android.system.ErrnoException)errno=" errno="+((android.system.ErrnoException)cause).errno;Log.e("PutyScratchProbe","FAIL "+phase+" "+failure.getClass().getSimpleName()+errno);}
    finally{EncryptedScratchFiles.closeAll();Open_im_sdk.lockNativeLocalStore(account);finish();}
  },"synthetic-probe").start();}
  private void renew(){byte[] key=new byte[32];Arrays.fill(key,(byte)17);require(Open_im_sdk.configureNativeLocalStore(account,new File(getNoBackupFilesDir(),"synthetic").getAbsolutePath(),key,System.currentTimeMillis()+8000));Arrays.fill(key,(byte)0);}
  private void run()throws Exception {
    phase="configure";renew();require(Open_im_sdk.openConfiguredNativeWorkingStore(account));
    EncryptedScratchFiles.initialize(this,new EncryptedScratchFiles.Authority(){public String authorizedAccount(){return allowed?account:null;}public long generation(){return generation.get();}});pass(phase);
    phase="proc_write_read";File file=EncryptedScratchFiles.createTempFile("capture",".mp4");
    byte[] expected=new byte[131173];for(int i=0;i<expected.length;i++)expected[i]=(byte)(i%251);
    phase="proc_path_reopen_denied";try(FileOutputStream ignored=new FileOutputStream(file)){throw new AssertionError();}catch(FileNotFoundException expectedFailure){}pass(phase);
    phase="descriptor_output";try(OutputStream output=EncryptedScratchFiles.outputStream(file.getAbsolutePath())){output.write(expected);output.flush();}pass(phase);
    phase="descriptor_random_readwrite";try(android.os.ParcelFileDescriptor descriptor=EncryptedScratchFiles.descriptor(file.getAbsolutePath())){require(android.system.Os.fstat(descriptor.getFileDescriptor()).st_size==expected.length);byte[] patch={17,18,19,20,21};require(android.system.Os.pwrite(descriptor.getFileDescriptor(),patch,0,patch.length,65529)==patch.length);System.arraycopy(patch,0,expected,65529,patch.length);byte[] actual=new byte[expected.length];int done=0;while(done<actual.length){int n=android.system.Os.pread(descriptor.getFileDescriptor(),actual,done,actual.length-done,done);require(n>0);done+=n;}require(Arrays.equals(expected,actual));Arrays.fill(actual,(byte)0);}pass(phase);
    phase="input_stream";require(Arrays.equals(expected,EncryptedScratchFiles.readBytes(file.getAbsolutePath(),expected.length)));pass(phase);
    phase="private_content_preview";android.net.Uri preview=android.net.Uri.parse(EncryptedScratchFiles.previewUri(file.getAbsolutePath()));try(InputStream input=getContentResolver().openInputStream(preview)){byte[] actual=new byte[expected.length];int done=0;phase="private_content_read";while(done<actual.length){int n=input.read(actual,done,actual.length-done);require(n>0);done+=n;}phase="private_content_bytes";require(Arrays.equals(expected,actual));phase="private_content_eof";require(input.read()==-1);Arrays.fill(actual,(byte)0);}phase="private_content_write_reject";try{getContentResolver().openFileDescriptor(preview,"rw");throw new AssertionError();}catch(FileNotFoundException expectedFailure){}pass(phase);
    phase="stage";renew();String staged=EncryptedScratchFiles.stage(file.getAbsolutePath());require(!EncryptedScratchFiles.owns(file.getAbsolutePath()));byte[] recovered=Open_im_sdk.readNativeSource(account,staged,expected.length+1);require(Arrays.equals(recovered,expected));Arrays.fill(recovered,(byte)0);pass(phase);
    phase="descriptor_no_reuse";File newer=EncryptedScratchFiles.createTempFile("next",".bin");require(!file.equals(newer));try{EncryptedScratchFiles.stage(file.getAbsolutePath());throw new AssertionError();}catch(IOException expectedFailure){}require(EncryptedScratchFiles.owns(newer.getAbsolutePath()));pass(phase);
    phase="generation_prepare";android.net.Uri currentPreview=android.net.Uri.parse(EncryptedScratchFiles.previewUri(newer.getAbsolutePath()));try(OutputStream output=EncryptedScratchFiles.outputStream(newer.getAbsolutePath())){output.write(new byte[256]);try(android.os.ParcelFileDescriptor held=getContentResolver().openFileDescriptor(currentPreview,"r")){byte[] sample=new byte[128];phase="generation_first_read";require(android.system.Os.pread(held.getFileDescriptor(),sample,0,128,0)==128);generation.incrementAndGet();phase="generation_write_deny";try{output.write(1);throw new AssertionError();}catch(IOException expectedFailure){}phase="generation_cached_read_deny";try{android.system.Os.pread(held.getFileDescriptor(),sample,0,128,0);throw new AssertionError();}catch(android.system.ErrnoException expectedFailure){require(expectedFailure.errno==android.system.OsConstants.EACCES);}Arrays.fill(sample,(byte)0);}}phase="generation_stage_deny";try{EncryptedScratchFiles.stage(newer.getAbsolutePath());throw new AssertionError();}catch(IOException expectedFailure){}try{getContentResolver().openInputStream(preview);throw new AssertionError();}catch(FileNotFoundException expectedFailure){}pass(phase);
    EncryptedScratchFiles.closeAll();renew();
    phase="close_all_consumers";File closing=EncryptedScratchFiles.createTempFile("consumer",".bin");try(OutputStream output=EncryptedScratchFiles.outputStream(closing.getAbsolutePath())){output.write(new byte[]{1,2,3});}android.os.ParcelFileDescriptor held=EncryptedScratchFiles.descriptor(closing.getAbsolutePath());InputStream heldInput=EncryptedScratchFiles.inputStream(closing.getAbsolutePath());EncryptedScratchFiles.closeAll();require(!held.getFileDescriptor().valid());try{heldInput.read();throw new AssertionError();}catch(IOException expectedFailure){}held.close();heldInput.close();pass(phase);
    phase="producer_cancel_tokens";java.util.concurrent.atomic.AtomicInteger cancelled=new java.util.concurrent.atomic.AtomicInteger();String old=EncryptedScratchFiles.registerConsumer(()->cancelled.addAndGet(100));EncryptedScratchFiles.unregisterConsumer(old);String current=EncryptedScratchFiles.registerConsumer(()->{cancelled.incrementAndGet();EncryptedScratchFiles.closeAll();});require(!old.equals(current));EncryptedScratchFiles.unregisterConsumer(old);EncryptedScratchFiles.closeAll();require(cancelled.get()==1);EncryptedScratchFiles.closeAll();require(cancelled.get()==1);allowed=false;try{EncryptedScratchFiles.registerConsumer(()->cancelled.incrementAndGet());throw new AssertionError();}catch(IOException expectedFailure){}require(cancelled.get()==2);allowed=true;pass(phase);
    phase="lease_deny";File expiry=EncryptedScratchFiles.createTempFile("lease",".bin");try(OutputStream output=EncryptedScratchFiles.outputStream(expiry.getAbsolutePath())){Open_im_sdk.lockNativeLocalStore(account);try{output.write(1);throw new AssertionError();}catch(IOException expectedFailure){}}pass(phase);
    Arrays.fill(expected,(byte)0);
  }
}
