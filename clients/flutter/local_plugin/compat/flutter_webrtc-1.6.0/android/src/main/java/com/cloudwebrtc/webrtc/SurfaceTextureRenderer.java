package com.cloudwebrtc.webrtc;

import android.graphics.SurfaceTexture;
import android.view.Surface;

import org.webrtc.EglBase;
import org.webrtc.EglRenderer;
import org.webrtc.GlRectDrawer;
import org.webrtc.RendererCommon;
import org.webrtc.ThreadUtils;
import org.webrtc.VideoFrame;

import java.util.concurrent.CountDownLatch;

import io.flutter.view.TextureRegistry;

/**
 * Display the video stream on a Surface.
 * renderFrame() is asynchronous to avoid blocking the calling thread.
 * This class is thread safe and handles access from potentially three different threads:
 * Interaction from the main app in init, release and setMirror.
 * Interaction from C++ rtc::VideoSinkInterface in renderFrame.
 * Interaction from SurfaceHolder lifecycle in surfaceCreated, surfaceChanged, and surfaceDestroyed.
 */
public class SurfaceTextureRenderer extends EglRenderer {
  // Callback for reporting renderer events. Read-only after initilization so no lock required.
  private RendererCommon.RendererEvents rendererEvents;
  private final Object layoutLock = new Object();
  private boolean isRenderingPaused;
  private boolean isFirstFrameRendered;
  private int rotatedFrameWidth;
  private int rotatedFrameHeight;
  private int frameRotation;
  private volatile boolean finalRelease;
  private volatile boolean surfaceAvailable = true;
  private final java.util.concurrent.atomic.AtomicInteger pendingSurfaceCleanup = new java.util.concurrent.atomic.AtomicInteger();
  private NativeMediaLifecycle.Owner cleanupOwner;
  private volatile RuntimeException cleanupFailure;
  private volatile Runnable cleanupFailureHandler = () -> {};

  void setCleanupOwner(NativeMediaLifecycle.Owner owner) { cleanupOwner = owner; }
  void setCleanupFailureHandler(Runnable handler) {
    cleanupFailureHandler = handler;
    if (cleanupFailure != null || cleanupOwner.isPermanentlyQuarantined()) handler.run();
  }
  void throwIfCleanupFailed() { if (cleanupFailure != null) throw cleanupFailure; }
  void beginFinalRelease() { finalRelease = true; }
  void resetFrameState() {
    synchronized (layoutLock) {
      isFirstFrameRendered = false;
      rotatedFrameWidth = 0; rotatedFrameHeight = 0; frameRotation = -1;
    }
  }

  /**
   * In order to render something, you must first call init().
   */
  public SurfaceTextureRenderer(String name) {
    super(name);
  }

  public void init(final EglBase.Context sharedContext,
                   RendererCommon.RendererEvents rendererEvents) {
    init(sharedContext, rendererEvents, EglBase.CONFIG_PLAIN, new GlRectDrawer());
  }

  /**
   * Initialize this class, sharing resources with |sharedContext|. The custom |drawer| will be used
   * for drawing frames on the EGLSurface. This class is responsible for calling release() on
   * |drawer|. It is allowed to call init() to reinitialize the renderer after a previous
   * init()/release() cycle.
   */
  public void init(final EglBase.Context sharedContext,
                   RendererCommon.RendererEvents rendererEvents, final int[] configAttributes,
                   RendererCommon.GlDrawer drawer) {
    ThreadUtils.checkIsOnMainThread();
    this.rendererEvents = rendererEvents;
    synchronized (layoutLock) {
      isFirstFrameRendered = false;
      rotatedFrameWidth = 0;
      rotatedFrameHeight = 0;
      frameRotation = -1;
    }
    super.init(sharedContext, configAttributes, drawer);
  }
  @Override
  public void init(final EglBase.Context sharedContext, final int[] configAttributes,
                   RendererCommon.GlDrawer drawer) {
    init(sharedContext, null /* rendererEvents */, configAttributes, drawer);
  }
  /**
   * Limit render framerate.
   *
   * @param fps Limit render framerate to this value, or use Float.POSITIVE_INFINITY to disable fps
   *            reduction.
   */
  @Override
  public void setFpsReduction(float fps) {
    synchronized (layoutLock) {
      isRenderingPaused = fps == 0f;
    }
    super.setFpsReduction(fps);
  }
  @Override
  public void disableFpsReduction() {
    synchronized (layoutLock) {
      isRenderingPaused = false;
    }
    super.disableFpsReduction();
  }
  @Override
  public void pauseVideo() {
    synchronized (layoutLock) {
      isRenderingPaused = true;
    }
    super.pauseVideo();
  }
  // VideoSink interface.
  @Override
  public void onFrame(VideoFrame frame) {
    if (finalRelease || !surfaceAvailable || pendingSurfaceCleanup.get() != 0) return;
    synchronized (surfaceLock) {
      if(surface == null) {
        producer.setSize(frame.getRotatedWidth(),frame.getRotatedHeight());
        surface = producer.getSurface();
        createEglSurface(surface);
      } else if (frameSizeChanged(frame)) {
        // The producer's backing buffers are fixed-size: setSize() only takes
        // effect for a Surface obtained afterwards. Without recreating the EGL
        // surface here, a simulcast layer upgrade keeps rendering into the old
        // low-resolution buffer and the video stays blurry.
        releaseEglSurface(() -> {});
        // Clear the field before re-obtaining: if getSurface() throws, the
        // next frame takes the surface == null path and recreates cleanly
        // rather than rendering into the already-released surface.
        surface = null;
        producer.setSize(frame.getRotatedWidth(), frame.getRotatedHeight());
        surface = producer.getSurface();
        createEglSurface(surface);
      }
    }
    updateFrameDimensionsAndReportEvents(frame);
    super.onFrame(frame);
  }

  private boolean frameSizeChanged(VideoFrame frame) {
    synchronized (layoutLock) {
      return !isRenderingPaused
          && (rotatedFrameWidth != frame.getRotatedWidth()
              || rotatedFrameHeight != frame.getRotatedHeight());
    }
  }

  // Guards surface lifecycle transitions: creation/recreation happens on the
  // frame delivery thread while destruction arrives on the main thread via
  // the producer's onSurfaceCleanup callback. Serializing the two prevents a
  // frame from re-creating the EGL surface against a Surface the producer is
  // concurrently invalidating. The bounded output worker holds this lock
  // during EGL release; the platform callback only enqueues the operation.
  private final Object surfaceLock = new Object();
  private Surface surface = null;

  private TextureRegistry.SurfaceProducer producer;

  public void surfaceCreated(final TextureRegistry.SurfaceProducer producer) {
    ThreadUtils.checkIsOnMainThread();
    this.producer = producer;
    this.producer.setCallback(
            new TextureRegistry.SurfaceProducer.Callback() {
              @Override
              public void onSurfaceAvailable() {
                surfaceAvailable = true;
              }

              @Override
              public void onSurfaceCleanup() {
                surfaceDestroyed();
              }
            }
    );
  }

  public void surfaceDestroyed() {
    ThreadUtils.checkIsOnMainThread();
    if (finalRelease) return;
    surfaceAvailable = false;
    pendingSurfaceCleanup.incrementAndGet();
    // The platform callback returns immediately. EGL waits happen only on the
    // same bounded output owner used by final renderer release.
    java.util.concurrent.CompletableFuture<Void> physical = cleanupOwner.submit("surface.cleanup", () -> {
      synchronized (surfaceLock) {
        final CountDownLatch completionLatch = new CountDownLatch(1);
        releaseEglSurface(completionLatch::countDown);
        ThreadUtils.awaitUninterruptibly(completionLatch);
        surface = null;
      }
      pendingSurfaceCleanup.decrementAndGet();
    }, true);
    physical.whenComplete((unused, error) -> {
      if (error != null) {
        Throwable cause = NativeMediaLifecycle.unwrap(error);
        cleanupFailure = cause instanceof RuntimeException ? (RuntimeException) cause : new RuntimeException(cause);
        cleanupOwner.quarantine(); cleanupFailureHandler.run();
      }
    });
    NativeMediaLifecycle.report(physical, new NativeMediaLifecycle.Response() {
      public void success() {}
      public void error(String code, String message) {
        // Only physical.whenComplete may report an actual native failure.
        // A caller deadline keeps this exact output barrier temporarily held.
        android.util.Log.w("FlutterWebRTCPlugin", "[OpenIMRtcNative] operation=surface.cleanup phase=callerError code=" + code);
      }
    }, () -> cleanupOwner.callerTimedOut(physical, "surface.cleanup"));
  }

  // Update frame dimensions and report any changes to |rendererEvents|.
  private void updateFrameDimensionsAndReportEvents(VideoFrame frame) {
    synchronized (layoutLock) {
      if (isRenderingPaused) {
        return;
      }
      if (!isFirstFrameRendered) {
        isFirstFrameRendered = true;
        if (rendererEvents != null) {
          rendererEvents.onFirstFrameRendered();
        }
      }
      if (rotatedFrameWidth != frame.getRotatedWidth()
              || rotatedFrameHeight != frame.getRotatedHeight()
              || frameRotation != frame.getRotation()) {
        if (rendererEvents != null) {
          rendererEvents.onFrameResolutionChanged(
                  frame.getBuffer().getWidth(), frame.getBuffer().getHeight(), frame.getRotation());
        }
        rotatedFrameWidth = frame.getRotatedWidth();
        rotatedFrameHeight = frame.getRotatedHeight();
        frameRotation = frame.getRotation();
      }
    }
  }
}
