package io.flutter.plugins.camerax;

import android.graphics.Bitmap;
import android.os.Build;
import androidx.camera.core.ImageCapture;
import androidx.camera.core.ImageProxy;
import io.openim.flutter_openim_sdk.EncryptedScratchFiles;
import java.io.IOException;
import java.nio.ByteBuffer;
import java.util.Arrays;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicReference;
import open_im_sdk.Open_im_sdk;

/** Real production bridge/proxy; only JNI, CameraX and Android OS are doubles. */
public final class PhotoCaptureStageHarness {
  static int passed;
  static final class Actor implements EncryptedScratchFiles.Authority {
    volatile String account="synthetic-A";volatile long generation=1;
    public String authorizedAccount(){return account;}public long generation(){return generation;}
  }
  static Actor actor;
  static void check(boolean value,String message){if(!value)throw new AssertionError(message);}
  static void clean(){EncryptedScratchFiles.closeAll();Open_im_sdk.entered=null;Open_im_sdk.release=null;Open_im_sdk.fail=false;Open_im_sdk.linkage=false;Open_im_sdk.staged=0;Open_im_sdk.discarded=0;Open_im_sdk.received=null;Open_im_sdk.observed=null;Build.VERSION.SDK_INT=36;Bitmap.encoded=0;Bitmap.recycled=0;android.graphics.BitmapFactory.width=320;android.graphics.BitmapFactory.height=240;actor=new Actor();EncryptedScratchFiles.initialize(new android.content.Context(),actor);}
  static boolean erased(byte[] bytes){if(bytes==null)return false;for(byte b:bytes)if(b!=0)return false;return true;}
  interface Work {void run()throws Exception;}
  static void test(String name,Work work)throws Exception{clean();try{work.run();passed++;System.out.println("PASS "+name);}finally{EncryptedScratchFiles.closeAll();}}
  static void denied(Work work)throws Exception{try{work.run();throw new AssertionError("expected denial");}catch(IOException expected){}}
  static String begin()throws Exception{return EncryptedScratchFiles.beginPhotoCapture("CAP.jpg");}
  static Thread blockedStage(String token,byte[] bytes,AtomicReference<Throwable> error)throws Exception{
    Open_im_sdk.entered=new CountDownLatch(1);Open_im_sdk.release=new CountDownLatch(1);
    Thread worker=new Thread(()->{try{EncryptedScratchFiles.stagePhotoCapture(token,bytes);}catch(Throwable failure){error.set(failure);}});
    worker.start();check(Open_im_sdk.entered.await(2,TimeUnit.SECONDS),"stage did not enter");return worker;
  }
  static void join(Thread thread)throws Exception{thread.join(2000);check(!thread.isAlive(),"owned stage thread leaked");}
  static final class Image implements ImageProxy {
    final byte[] bytes;final int rotation,format;boolean closed;
    Image(int rotation){this(rotation,256,new byte[]{1,2,3,4});}
    Image(int rotation,int format,byte[] bytes){this.rotation=rotation;this.format=format;this.bytes=bytes;}
    public int getFormat(){return format;}public PlaneProxy[] getPlanes(){return new PlaneProxy[]{()->ByteBuffer.wrap(bytes)};}
    public ImageInfo getImageInfo(){return ()->rotation;}public void close(){closed=true;}
  }
  static final class Capture {
    final ImageCapture camera=new ImageCapture();final AtomicReference<kotlin.Result<String>> result=new AtomicReference<>();int replies;
    Capture(){new ImageCaptureProxyApi(new ProxyApiRegistrar()).takePicture(camera,r->{replies++;result.set(r);return kotlin.Unit.INSTANCE;});}
    void image(Image image){camera.pending.onCaptureSuccess(image);}
    void success(){check(result.get()!=null&&result.get().failure==null&&result.get().value.startsWith("/synthetic-encrypted/"),"success source missing");check(replies==1,"duplicate callback");}
    void failure(){check(result.get()!=null&&result.get().failure instanceof IOException&&result.get().value==null,"fixed failure missing");check(replies==1,"duplicate callback");}
  }
  public static void main(String[] args)throws Exception{
    test("single durable API/no scratch; bytes cleared; one-use handoff",()->{String token=begin();byte[] bytes={1,2,3};EncryptedScratchFiles.stagePhotoCapture(token,bytes);check(erased(bytes),"bytes retained");check(Open_im_sdk.staged==1&&"CAP.jpg".equals(Open_im_sdk.name),"stage metadata");check(Arrays.equals(Open_im_sdk.observed,new byte[]{1,2,3}),"stage bytes");check(EncryptedScratchFiles.finishPhotoCapture(token).endsWith("source.jpg"),"source");denied(()->EncryptedScratchFiles.finishPhotoCapture(token));check(Open_im_sdk.discarded==0,"successful handoff discarded");});
    test("Android7 photo denied; name/path rejected; no JNI",()->{Build.VERSION.SDK_INT=25;denied(()->begin());Build.VERSION.SDK_INT=36;denied(()->EncryptedScratchFiles.beginPhotoCapture("../CAP.jpg"));check(Open_im_sdk.staged==0,"denied stage called");});
    test("empty and overlimit bytes denied and wiped",()->{String token=begin();denied(()->EncryptedScratchFiles.stagePhotoCapture(token,new byte[0]));byte[] big=new byte[(32<<20)+1];big[0]=9;denied(()->EncryptedScratchFiles.stagePhotoCapture(token,big));check(erased(big)&&Open_im_sdk.staged==0,"oversize leaked");});
    test("unknown token and changed account denied; bytes wiped",()->{byte[] bytes={1};denied(()->EncryptedScratchFiles.stagePhotoCapture("foreign",bytes));check(erased(bytes),"unknown bytes");String token=begin();actor.account="synthetic-B";byte[] input={2};denied(()->EncryptedScratchFiles.stagePhotoCapture(token,input));check(erased(input)&&Open_im_sdk.staged==0,"actor accepted");});
    test("native disk failure closed and wiped; fresh retry works",()->{String token=begin();byte[] bytes={4};Open_im_sdk.fail=true;denied(()->EncryptedScratchFiles.stagePhotoCapture(token,bytes));check(erased(bytes),"failure bytes");denied(()->EncryptedScratchFiles.finishPhotoCapture(token));Open_im_sdk.fail=false;String retry=begin();EncryptedScratchFiles.stagePhotoCapture(retry,new byte[]{3});EncryptedScratchFiles.finishPhotoCapture(retry);});
    test("slow stage account revoke denies and deletes owned result",()->{String token=begin();byte[] bytes={1};AtomicReference<Throwable> error=new AtomicReference<>();Thread thread=blockedStage(token,bytes,error);actor.account=null;actor.generation++;Open_im_sdk.release.countDown();join(thread);check(error.get() instanceof IOException&&erased(bytes)&&Open_im_sdk.discarded==1,"revoke result escaped");});
    test("same account new generation denies slow result",()->{String token=begin();AtomicReference<Throwable> error=new AtomicReference<>();Thread thread=blockedStage(token,new byte[]{1},error);actor.generation++;Open_im_sdk.release.countDown();join(thread);check(error.get() instanceof IOException&&Open_im_sdk.discarded==1,"generation accepted");});
    test("closeAll does not wait for slow stage; cancels then deletes",()->{String token=begin();AtomicReference<Throwable> error=new AtomicReference<>();Thread thread=blockedStage(token,new byte[]{1},error);EncryptedScratchFiles.closeAll();check(thread.isAlive(),"synthetic stage unexpectedly completed");Open_im_sdk.release.countDown();join(thread);check(error.get() instanceof IOException&&Open_im_sdk.discarded==1,"closeAll escaped");});
    test("concurrent duplicate cannot cancel rightful stage",()->{String token=begin();AtomicReference<Throwable> error=new AtomicReference<>();Thread thread=blockedStage(token,new byte[]{1},error);byte[] duplicate={9};denied(()->EncryptedScratchFiles.stagePhotoCapture(token,duplicate));check(erased(duplicate),"duplicate not wiped");Open_im_sdk.release.countDown();join(thread);check(error.get()==null,"duplicate invalidated owner");EncryptedScratchFiles.finishPhotoCapture(token);check(Open_im_sdk.staged==1,"duplicate stage");});
    test("discard between stage and handoff cleans without transfer",()->{String token=begin();EncryptedScratchFiles.stagePhotoCapture(token,new byte[]{1});EncryptedScratchFiles.discardPhotoCapture(token);denied(()->EncryptedScratchFiles.finishPhotoCapture(token));check(Open_im_sdk.discarded==1,"cancel source retained");});
    test("reinitialization rejects completed old actor and retains new actor",()->{String token=begin();EncryptedScratchFiles.stagePhotoCapture(token,new byte[]{1});Actor replacement=new Actor();EncryptedScratchFiles.initialize(new android.content.Context(),replacement);denied(()->EncryptedScratchFiles.finishPhotoCapture(token));check(Open_im_sdk.discarded==1,"initialize retained old result");check(begin()!=null,"new actor unavailable");});
    test("actual CameraX JPEG0 direct path and image close",()->{Capture capture=new Capture();Image image=new Image(0);capture.image(image);capture.success();check(image.closed&&Open_im_sdk.staged==1&&Bitmap.encoded==0&&erased(Open_im_sdk.received),"photo invariants");});
    test("actual CameraX rotation remains JPEG90 and recycled",()->{Capture capture=new Capture();Image image=new Image(90);capture.image(image);capture.success();check(image.closed&&Bitmap.encoded==1&&Bitmap.quality==90&&Bitmap.recycled==2&&erased(Open_im_sdk.received),"rotation changed");});
    test("actual CameraX rejects invalid format and dimensions",()->{Capture capture=new Capture();Image image=new Image(0,1,new byte[]{1});capture.image(image);capture.failure();check(image.closed&&Open_im_sdk.staged==0,"format accepted");capture=new Capture();image=new Image(90);android.graphics.BitmapFactory.width=20000;android.graphics.BitmapFactory.height=20000;capture.image(image);capture.failure();check(image.closed&&Open_im_sdk.staged==0,"dimensions accepted");});
    test("actual CameraX cancellation before capture rejects",()->{Capture capture=new Capture();EncryptedScratchFiles.closeAll();Image image=new Image(0);capture.image(image);capture.failure();check(image.closed&&Open_im_sdk.staged==0,"cancel accepted");});
    test("actual CameraX waits durable boundary and cancels in-flight",()->{Capture capture=new Capture();Open_im_sdk.entered=new CountDownLatch(1);Open_im_sdk.release=new CountDownLatch(1);Image image=new Image(0);Thread thread=new Thread(()->capture.image(image));thread.start();check(Open_im_sdk.entered.await(2,TimeUnit.SECONDS),"no stage");check(capture.result.get()==null,"returned before durable API");EncryptedScratchFiles.closeAll();Open_im_sdk.release.countDown();join(thread);capture.failure();check(image.closed&&erased(Open_im_sdk.received)&&Open_im_sdk.discarded==1,"in-flight cancel leaked");});
    test("actual CameraX native error completes once and clears",()->{Capture capture=new Capture();Open_im_sdk.fail=true;Image image=new Image(0);capture.image(image);capture.failure();check(image.closed&&erased(Open_im_sdk.received),"failed image bytes retained");});
    test("actual CameraX synchronous start error completes and releases token",()->{ImageCapture camera=new ImageCapture();camera.failStart=true;AtomicReference<kotlin.Result<String>> result=new AtomicReference<>();new ImageCaptureProxyApi(new ProxyApiRegistrar()).takePicture(camera,r->{result.set(r);return kotlin.Unit.INSTANCE;});check(result.get()!=null&&result.get().failure instanceof IOException,"start failure no callback");for(int i=0;i<64;i++)begin();denied(()->begin());});
    test("native linkage failure still clears original bytes",()->{String token=begin();byte[] bytes={7};Open_im_sdk.linkage=true;try{EncryptedScratchFiles.stagePhotoCapture(token,bytes);throw new AssertionError("linkage expected");}catch(UnsatisfiedLinkError expected){}check(erased(bytes),"linkage retained plaintext");denied(()->EncryptedScratchFiles.finishPhotoCapture(token));});
    System.out.println("PHOTO_STAGE_RESULT "+passed+"/19 passed; actual Java, synthetic JNI/CameraX/OS; no real camera timing");
  }
}
