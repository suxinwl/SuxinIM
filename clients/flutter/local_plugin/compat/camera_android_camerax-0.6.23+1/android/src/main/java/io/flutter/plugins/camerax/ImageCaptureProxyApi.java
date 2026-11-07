// Copyright 2013 The Flutter Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package io.flutter.plugins.camerax;

import androidx.annotation.NonNull;
import androidx.annotation.Nullable;
import androidx.camera.core.ImageCapture;
import androidx.camera.core.ImageCaptureException;
import androidx.camera.core.ImageProxy;
import androidx.camera.core.resolutionselector.ResolutionSelector;
import java.io.File;
import java.io.IOException;
import java.nio.ByteBuffer;
import java.util.Arrays;
import java.util.concurrent.Executors;
import kotlin.Result;
import kotlin.Unit;
import kotlin.jvm.functions.Function1;

/**
 * ProxyApi implementation for {@link ImageCapture}. This class may handle instantiating native
 * object instances that are attached to a Dart instance or handle method calls on the associated
 * native class or an instance of that class.
 */
class ImageCaptureProxyApi extends PigeonApiImageCapture {
  static final String TEMPORARY_FILE_NAME = "CAP";
  static final String JPG_FILE_TYPE = ".jpg";

  ImageCaptureProxyApi(@NonNull ProxyApiRegistrar pigeonRegistrar) {
    super(pigeonRegistrar);
  }

  @NonNull
  @Override
  public ProxyApiRegistrar getPigeonRegistrar() {
    return (ProxyApiRegistrar) super.getPigeonRegistrar();
  }

  @NonNull
  @Override
  public ImageCapture pigeon_defaultConstructor(
      @Nullable ResolutionSelector resolutionSelector,
      @Nullable Long targetRotation,
      @Nullable CameraXFlashMode flashMode) {
    final ImageCapture.Builder builder = new ImageCapture.Builder();
    if (targetRotation != null) {
      builder.setTargetRotation(targetRotation.intValue());
    }
    if (flashMode != null) {
      // This sets the requested flash mode, but may fail silently.
      switch (flashMode) {
        case AUTO:
          builder.setFlashMode(ImageCapture.FLASH_MODE_AUTO);
          break;
        case OFF:
          builder.setFlashMode(ImageCapture.FLASH_MODE_OFF);
          break;
        case ON:
          builder.setFlashMode(ImageCapture.FLASH_MODE_ON);
          break;
      }
    }
    if (resolutionSelector != null) {
      builder.setResolutionSelector(resolutionSelector);
    }
    return builder.build();
  }

  @Override
  public void setFlashMode(
      @NonNull ImageCapture pigeonInstance, @NonNull CameraXFlashMode flashMode) {
    int nativeFlashMode = -1;
    switch (flashMode) {
      case AUTO:
        nativeFlashMode = ImageCapture.FLASH_MODE_AUTO;
        break;
      case OFF:
        nativeFlashMode = ImageCapture.FLASH_MODE_OFF;
        break;
      case ON:
        nativeFlashMode = ImageCapture.FLASH_MODE_ON;
    }
    pigeonInstance.setFlashMode(nativeFlashMode);
  }

  @Override
  public void takePicture(
      @NonNull ImageCapture pigeonInstance,
      @NonNull Function1<? super Result<String>, Unit> callback) {
    final Class<?> bridge;
    final String captureToken;
    try {
      bridge=Class.forName("io.openim.flutter_openim_sdk.EncryptedScratchFiles");
      captureToken=(String)bridge.getMethod("beginPhotoCapture",String.class).invoke(null,TEMPORARY_FILE_NAME+JPG_FILE_TYPE);
    } catch (ReflectiveOperationException | SecurityException e) {
      ResultCompat.failure(e, callback);
      return;
    }

    // In-memory camera capture avoids ImageSaver's plaintext temporary JPEG.
    final java.util.concurrent.ExecutorService captureExecutor=Executors.newSingleThreadExecutor();
    final java.util.concurrent.atomic.AtomicBoolean canceled=new java.util.concurrent.atomic.AtomicBoolean();
    final java.util.concurrent.atomic.AtomicReference<String> registration=new java.util.concurrent.atomic.AtomicReference<>();
    try{
      registration.set((String)bridge.getMethod("registerConsumer",Runnable.class).invoke(null,(Runnable)()->{canceled.set(true);try{bridge.getMethod("discardPhotoCapture",String.class).invoke(null,captureToken);}catch(Exception ignored){}captureExecutor.shutdownNow();}));
    }catch(Exception rejected){try{bridge.getMethod("discardPhotoCapture",String.class).invoke(null,captureToken);}catch(Exception ignored){}captureExecutor.shutdownNow();ResultCompat.failure(new IOException("Encrypted camera unavailable"),callback);return;}
    try{pigeonInstance.takePicture(captureExecutor,new ImageCapture.OnImageCapturedCallback(){
      private void finish(){
        String token=registration.getAndSet(null);
        try{if(token!=null)bridge.getMethod("unregisterConsumer",String.class).invoke(null,token);bridge.getMethod("discardPhotoCapture",String.class).invoke(null,captureToken);}catch(Exception ignored){}
        captureExecutor.shutdown();
      }
      @Override public void onCaptureSuccess(@NonNull ImageProxy image){byte[] bytes=null;android.graphics.Bitmap decoded=null,rotated=null;
        try{
          if(canceled.get()||image.getFormat()!=android.graphics.ImageFormat.JPEG||image.getPlanes().length!=1)throw new IOException("Protected camera unavailable");
          ByteBuffer buffer=image.getPlanes()[0].getBuffer();if(buffer.remaining()>(32<<20))throw new IOException("Protected image exceeds limit");bytes=new byte[buffer.remaining()];buffer.get(bytes);
          int rotation=image.getImageInfo().getRotationDegrees();
          if(rotation!=0){
            android.graphics.BitmapFactory.Options bounds=new android.graphics.BitmapFactory.Options();bounds.inJustDecodeBounds=true;android.graphics.BitmapFactory.decodeByteArray(bytes,0,bytes.length,bounds);
            if(bounds.outWidth<=0||bounds.outHeight<=0||(long)bounds.outWidth*bounds.outHeight>16000000)throw new IOException("Protected image dimensions rejected");
            decoded=android.graphics.BitmapFactory.decodeByteArray(bytes,0,bytes.length);if(decoded==null)throw new IOException("Protected image unavailable");
            android.graphics.Matrix matrix=new android.graphics.Matrix();matrix.postRotate(rotation);rotated=android.graphics.Bitmap.createBitmap(decoded,0,0,decoded.getWidth(),decoded.getHeight(),matrix,true);
            class ZeroingBytes extends java.io.ByteArrayOutputStream{void destroy(){Arrays.fill(buf,(byte)0);reset();}}
            ZeroingBytes output=new ZeroingBytes();try{if(!rotated.compress(android.graphics.Bitmap.CompressFormat.JPEG,90,output))throw new IOException("Protected image unavailable");Arrays.fill(bytes,(byte)0);bytes=output.toByteArray();}finally{output.destroy();}
          }
          if(canceled.get())throw new IOException("Protected capture canceled");
          bridge.getMethod("stagePhotoCapture",String.class,byte[].class).invoke(null,captureToken,bytes);
          if(canceled.get())throw new IOException("Protected capture canceled");
          String source=(String)bridge.getMethod("finishPhotoCapture",String.class).invoke(null,captureToken);
          ResultCompat.success(source,callback);
        }catch(Exception rejected){ResultCompat.failure(new IOException("Encrypted camera capture rejected"),callback);}
        finally{if(bytes!=null)Arrays.fill(bytes,(byte)0);if(rotated!=null&&rotated!=decoded)rotated.recycle();if(decoded!=null)decoded.recycle();image.close();finish();}
      }
      @Override public void onError(@NonNull ImageCaptureException error){try{ResultCompat.failure(new IOException("Encrypted camera capture unavailable"),callback);}finally{finish();}}
    });}catch(RuntimeException|LinkageError unavailable){
      String token=registration.getAndSet(null);
      try{if(token!=null)bridge.getMethod("unregisterConsumer",String.class).invoke(null,token);bridge.getMethod("discardPhotoCapture",String.class).invoke(null,captureToken);}catch(Exception ignored){}
      captureExecutor.shutdownNow();ResultCompat.failure(new IOException("Encrypted camera capture unavailable"),callback);
    }
  }

  @Override
  public void setTargetRotation(ImageCapture pigeonInstance, long rotation) {
    pigeonInstance.setTargetRotation((int) rotation);
  }

  @Nullable
  @Override
  public ResolutionSelector resolutionSelector(@NonNull ImageCapture pigeonInstance) {
    return pigeonInstance.getResolutionSelector();
  }

  ImageCapture.OutputFileOptions createImageCaptureOutputFileOptions(@NonNull File file) {
    return new ImageCapture.OutputFileOptions.Builder(file).build();
  }

  @NonNull
  ImageCapture.OnImageSavedCallback createOnImageSavedCallback(
      @NonNull File file, @NonNull Function1<? super Result<String>, Unit> callback) {
    return new ImageCapture.OnImageSavedCallback() {
      @Override
      public void onImageSaved(@NonNull ImageCapture.OutputFileResults outputFileResults) {
        ResultCompat.success(file.getAbsolutePath(), callback);
      }

      @Override
      public void onError(@NonNull ImageCaptureException exception) {
        ResultCompat.failure(exception, callback);
      }
    };
  }
}
