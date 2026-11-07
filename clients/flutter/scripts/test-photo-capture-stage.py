"""Exercise actual protected photo Java with synthetic CameraX, OS and Go IO.

No camera, Android device, account, filesystem source or network is accessed.
Native stage completion stands for the existing durable Go API, not real fsync.
"""
from pathlib import Path
import hashlib
import json
import os
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[3]
JAVA = Path('E:/AndroidDev/jdk/jdk-17.0.20.1+1/bin')
STUBS = {
 'android/content/Context.java': '''package android.content; public class Context {public static final String STORAGE_SERVICE="storage"; public Context getApplicationContext(){return this;}public Object getSystemService(String s){throw new AssertionError("No FUSE expected for photos");}public String getPackageName(){return "synthetic";}public ContentResolver getContentResolver(){return new ContentResolver();}}''',
 'android/content/ContentResolver.java': '''package android.content;public class ContentResolver {public java.io.InputStream openInputStream(android.net.Uri u)throws java.io.IOException{throw new AssertionError("No URI expected");}}''',
 'android/os/Build.java': 'package android.os;public class Build {public static class VERSION {public static int SDK_INT=36;}}',
 'android/os/HandlerThread.java': 'package android.os;public class HandlerThread {public HandlerThread(String s){}public void start(){}public Object getLooper(){return null;}}',
 'android/os/Handler.java': 'package android.os;public class Handler {public Handler(Object o){}}',
 'android/os/ProxyFileDescriptorCallback.java': '''package android.os;public class ProxyFileDescriptorCallback {public long onGetSize()throws android.system.ErrnoException{return 0;}public int onRead(long o,int n,byte[] b)throws android.system.ErrnoException{return 0;}public int onWrite(long o,int n,byte[] b)throws android.system.ErrnoException{return 0;}public void onFsync()throws android.system.ErrnoException{}public void onRelease(){}}''',
 'android/os/ParcelFileDescriptor.java': '''package android.os;public class ParcelFileDescriptor implements java.io.Closeable {public static final int MODE_READ_ONLY=1,MODE_READ_WRITE=2;public static ParcelFileDescriptor dup(java.io.FileDescriptor f)throws java.io.IOException{return new ParcelFileDescriptor();}public static ParcelFileDescriptor adoptFd(int f){return new ParcelFileDescriptor();}public java.io.FileDescriptor getFileDescriptor(){return new java.io.FileDescriptor();}public int getFd(){return 99;}public void close()throws java.io.IOException{}public static class AutoCloseOutputStream extends java.io.OutputStream {public AutoCloseOutputStream(ParcelFileDescriptor f){}public void write(int b){throw new AssertionError("No scratch write expected");}}}''',
 'android/os/storage/StorageManager.java': '''package android.os.storage;public class StorageManager {public android.os.ParcelFileDescriptor openProxyFileDescriptor(int m,android.os.ProxyFileDescriptorCallback c,android.os.Handler h)throws java.io.IOException{throw new AssertionError("No FUSE expected");}}''',
 'android/system/ErrnoException.java': 'package android.system;public class ErrnoException extends Exception {public ErrnoException(String s,int e){super(s);}}',
 'android/system/OsConstants.java': 'package android.system;public class OsConstants {public static final int EACCES=1,EINVAL=2,EIO=3,F_GETFL=4,F_SETFL=5,F_DUPFD_CLOEXEC=6;}',
 'android/system/Os.java': 'package android.system;public class Os {public static int fcntlInt(java.io.FileDescriptor f,int op,int v)throws ErrnoException{throw new AssertionError("No FUSE expected");}}',
 'android/net/Uri.java': '''package android.net;public class Uri {public String getScheme(){return "content";}public String getAuthority(){return "";}public String getQuery(){return null;}public String getFragment(){return null;}public java.util.List<String> getPathSegments(){return java.util.Collections.emptyList();}public String getLastPathSegment(){return "";}public static class Builder {public Builder scheme(String s){return this;}public Builder authority(String s){return this;}public Builder appendPath(String s){return this;}public Uri build(){return new Uri();}}}''',
 'android/graphics/ImageFormat.java': 'package android.graphics;public class ImageFormat {public static final int JPEG=256;}',
 'android/graphics/Matrix.java': 'package android.graphics;public class Matrix {public static int rotation;public void postRotate(float r){rotation=(int)r;}}',
 'android/graphics/Bitmap.java': '''package android.graphics;public class Bitmap {public enum CompressFormat{JPEG}public static int encoded,recycled,quality;public int getWidth(){return 320;}public int getHeight(){return 240;}public static Bitmap createBitmap(Bitmap b,int x,int y,int w,int h,Matrix m,boolean f){return new Bitmap();}public boolean compress(CompressFormat f,int q,java.io.OutputStream out){encoded++;quality=q;try{out.write(new byte[]{9,8,7});return true;}catch(java.io.IOException e){throw new RuntimeException(e);}}public void recycle(){recycled++;}}''',
 'android/graphics/BitmapFactory.java': '''package android.graphics;public class BitmapFactory {public static int width=320,height=240;public static class Options {public boolean inJustDecodeBounds;public int outWidth,outHeight;}public static Bitmap decodeByteArray(byte[] b,int o,int n,Options opts){opts.outWidth=width;opts.outHeight=height;return null;}public static Bitmap decodeByteArray(byte[] b,int o,int n){return new Bitmap();}}''',
 'androidx/annotation/NonNull.java': 'package androidx.annotation;public @interface NonNull {}',
 'androidx/annotation/Nullable.java': 'package androidx.annotation;public @interface Nullable {}',
 'androidx/camera/core/resolutionselector/ResolutionSelector.java': 'package androidx.camera.core.resolutionselector;public class ResolutionSelector {}',
 'androidx/camera/core/ImageCaptureException.java': 'package androidx.camera.core;public class ImageCaptureException extends Exception {}',
 'androidx/camera/core/ImageProxy.java': '''package androidx.camera.core;public interface ImageProxy {int getFormat();PlaneProxy[] getPlanes();ImageInfo getImageInfo();void close();interface PlaneProxy {java.nio.ByteBuffer getBuffer();}interface ImageInfo {int getRotationDegrees();}}''',
 'androidx/camera/core/ImageCapture.java': '''package androidx.camera.core;
public class ImageCapture {public static final int FLASH_MODE_AUTO=0,FLASH_MODE_OFF=1,FLASH_MODE_ON=2;public OnImageCapturedCallback pending;public boolean failStart;public void takePicture(java.util.concurrent.Executor e,OnImageCapturedCallback c){if(failStart)throw new IllegalStateException("synthetic camera start");pending=c;}public void setFlashMode(int m){}public void setTargetRotation(int r){}public androidx.camera.core.resolutionselector.ResolutionSelector getResolutionSelector(){return null;}
 public static class Builder {public Builder setTargetRotation(int r){return this;}public Builder setFlashMode(int m){return this;}public Builder setResolutionSelector(androidx.camera.core.resolutionselector.ResolutionSelector s){return this;}public ImageCapture build(){return new ImageCapture();}}
 public static class OnImageCapturedCallback {public void onCaptureSuccess(ImageProxy i){}public void onError(ImageCaptureException e){}}
 public interface OnImageSavedCallback {void onImageSaved(OutputFileResults r);void onError(ImageCaptureException e);}public static class OutputFileResults{}public static class OutputFileOptions {public static class Builder {public Builder(java.io.File f){}public OutputFileOptions build(){return new OutputFileOptions();}}}}
''',
 'kotlin/Unit.java': 'package kotlin;public class Unit {public static final Unit INSTANCE=new Unit();}',
 'kotlin/Result.java': 'package kotlin;public class Result<T> {public final T value;public final Throwable failure;public Result(T v,Throwable e){value=v;failure=e;}}',
 'kotlin/jvm/functions/Function1.java': 'package kotlin.jvm.functions;public interface Function1<P,R>{R invoke(P p);}',
 'io/flutter/plugins/camerax/CameraXFlashMode.java': 'package io.flutter.plugins.camerax;public enum CameraXFlashMode {AUTO,OFF,ON}',
 'io/flutter/plugins/camerax/ProxyApiRegistrar.java': 'package io.flutter.plugins.camerax;public class ProxyApiRegistrar {}',
 'io/flutter/plugins/camerax/PigeonApiImageCapture.java': '''package io.flutter.plugins.camerax;public abstract class PigeonApiImageCapture {private final ProxyApiRegistrar r;public PigeonApiImageCapture(ProxyApiRegistrar r){this.r=r;}public ProxyApiRegistrar getPigeonRegistrar(){return r;}public abstract androidx.camera.core.ImageCapture pigeon_defaultConstructor(androidx.camera.core.resolutionselector.ResolutionSelector s,Long r,CameraXFlashMode f);public abstract void setFlashMode(androidx.camera.core.ImageCapture i,CameraXFlashMode m);public abstract void takePicture(androidx.camera.core.ImageCapture i,kotlin.jvm.functions.Function1<? super kotlin.Result<String>,kotlin.Unit> c);public abstract void setTargetRotation(androidx.camera.core.ImageCapture i,long r);public abstract androidx.camera.core.resolutionselector.ResolutionSelector resolutionSelector(androidx.camera.core.ImageCapture i);}''',
 'io/flutter/plugins/camerax/ResultCompat.java': '''package io.flutter.plugins.camerax;public class ResultCompat {public static <T>void success(T v,kotlin.jvm.functions.Function1<? super kotlin.Result<T>,kotlin.Unit> c){c.invoke(new kotlin.Result<T>(v,null));}public static <T>void failure(Throwable e,kotlin.jvm.functions.Function1<? super kotlin.Result<T>,kotlin.Unit> c){c.invoke(new kotlin.Result<T>(null,e));}}''',
 'open_im_sdk/Open_im_sdk.java': '''package open_im_sdk;
public class Open_im_sdk {public static java.util.concurrent.CountDownLatch entered,release;public static volatile boolean fail,linkage;public static int staged,discarded;public static byte[] received,observed;public static String account,name;
public static String stageNativeBytes(String a,String n,byte[] b){staged++;account=a;name=n;received=b;observed=b.clone();if(entered!=null)entered.countDown();if(release!=null){boolean interrupted=false;for(;;)try{release.await();break;}catch(InterruptedException e){interrupted=true;}if(interrupted)Thread.currentThread().interrupt();}if(linkage)throw new UnsatisfiedLinkError("synthetic");if(fail)return "";return "/synthetic-encrypted/outgoing/source.jpg";}
public static boolean discardNativeSource(String a,String s){discarded++;if(!a.equals(account)||!s.startsWith("/synthetic-encrypted/"))throw new AssertionError("foreign cleanup");return true;}
public static String beginNativeScratch(String a,String n){throw new AssertionError("No SQL scratch for photos");}public static boolean closeNativeScratch(String a,String i){throw new AssertionError("No SQL scratch for photos");}public static long nativeScratchSize(String a,String i){throw new AssertionError("No SQL scratch for photos");}public static byte[] readNativeScratch(String a,String i,long o,int n){throw new AssertionError("No SQL scratch for photos");}public static boolean writeNativeScratch(String a,String i,long o,byte[] b){throw new AssertionError("No SQL scratch for photos");}public static String stageNativeScratch(String a,String i){throw new AssertionError("No SQL scratch for photos");}}
''',
}


def main():
    sources = [ROOT/'sdks/flutter/android/src/main/java/io/openim/flutter_openim_sdk/EncryptedScratchFiles.java',
               ROOT/'clients/flutter/local_plugin/compat/camera_android_camerax-0.6.23+1/android/src/main/java/io/flutter/plugins/camerax/ImageCaptureProxyApi.java',
               ROOT/'clients/flutter/test/native/PhotoCaptureStageHarness.java']
    temp = Path(os.environ.get('TEMP', 'D:/puty-build-temp')).resolve()
    temp.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix='puty-photo-stage-', dir=temp) as generated:
        directory = Path(generated)
        for name, content in STUBS.items():
            path=directory/'doubles'/name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(content, encoding='utf-8')
        classes=directory/'classes'
        classes.mkdir()
        result=subprocess.run([str(JAVA/'javac.exe'), '--release', '11','-encoding','UTF-8','-d',str(classes),
                               *map(str,sources),*map(str,sorted((directory/'doubles').rglob('*.java')))],
                              capture_output=True,text=True,timeout=30)
        print(result.stdout,end='');print(result.stderr,end='')
        if result.returncode:return result.returncode
        print(json.dumps({'actualJavaSources':{str(p.relative_to(ROOT)):hashlib.sha256(p.read_bytes()).hexdigest() for p in sources},
                          'cameraOSAndGoAreSynthetic':True,'realDeviceTimingMeasured':False}),flush=True)
        result=subprocess.run([str(JAVA/'java.exe'),'-cp',str(classes),'io.flutter.plugins.camerax.PhotoCaptureStageHarness'],
                              capture_output=True,text=True,timeout=25)
        print(result.stdout,end='');print(result.stderr,end='')
        return result.returncode


if __name__=='__main__':raise SystemExit(main())
