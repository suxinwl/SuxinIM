package io.flutter.view;
import android.view.Surface;
public interface TextureRegistry {
  interface SurfaceProducer {
    interface Callback{void onSurfaceAvailable();void onSurfaceCleanup();}
    void setCallback(Callback callback);void setSize(int width,int height);Surface getSurface();void release();
  }
}
