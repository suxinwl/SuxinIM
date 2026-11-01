package com.cloudwebrtc.webrtc;
import java.util.List;
import java.util.concurrent.CopyOnWriteArrayList;
import java.util.concurrent.atomic.AtomicInteger;
import org.webrtc.EglBase;
import org.webrtc.RendererCommon.RendererEvents;
import io.flutter.view.TextureRegistry.SurfaceProducer;
/** Fake EGL/native output boundary. No production lifecycle logic is copied. */
public final class SurfaceTextureRenderer {
  public static final List<SurfaceTextureRenderer> instances = new CopyOnWriteArrayList<>();
  public final AtomicInteger releases = new AtomicInteger(), resets = new AtomicInteger();
  public volatile boolean finalRelease;
  public volatile Runnable onRelease = () -> {};
  public volatile RuntimeException cleanupFailure;
  public volatile Thread releaseThread;
  public NativeMediaLifecycle.Owner cleanupOwner;
  public Runnable cleanupFailureHandler;
  public SurfaceTextureRenderer(String name) { instances.add(this); }
  public void setCleanupOwner(NativeMediaLifecycle.Owner owner) { cleanupOwner = owner; }
  public void setCleanupFailureHandler(Runnable handler) { cleanupFailureHandler = handler; }
  public void init(EglBase.Context context, RendererEvents events) {}
  public void surfaceCreated(SurfaceProducer producer) {}
  public void resetFrameState() { resets.incrementAndGet(); }
  public void beginFinalRelease() { finalRelease = true; }
  public void release() { releaseThread = Thread.currentThread(); releases.incrementAndGet(); onRelease.run(); }
  public void throwIfCleanupFailed() { if (cleanupFailure != null) throw cleanupFailure; }
}
