package org.webrtc;
import android.view.Surface;
import java.util.concurrent.atomic.AtomicInteger;
import java.util.function.Consumer;
/** Only EGL boundary/counters; no production Surface lifecycle is copied. */
public class EglRenderer {
  public final AtomicInteger creates = new AtomicInteger(), frames = new AtomicInteger(), surfaceReleases = new AtomicInteger();
  public volatile Consumer<Runnable> releaseAction = Runnable::run;
  public EglRenderer(String name) {}
  public void init(EglBase.Context context, int[] attrs, RendererCommon.GlDrawer drawer) {}
  public void setFpsReduction(float fps) {}
  public void disableFpsReduction() {}
  public void pauseVideo() {}
  public void onFrame(VideoFrame frame) { frames.incrementAndGet(); }
  public void createEglSurface(Surface surface) { creates.incrementAndGet(); }
  public void releaseEglSurface(Runnable completion) { surfaceReleases.incrementAndGet(); releaseAction.accept(completion); }
}
