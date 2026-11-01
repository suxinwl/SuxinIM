package com.cloudwebrtc.webrtc;

import android.util.Log;
import android.graphics.SurfaceTexture;
import android.view.Surface;
import android.os.Handler;
import android.os.Looper;

import java.util.concurrent.CompletableFuture;
import java.util.concurrent.atomic.AtomicReference;
import java.util.function.BooleanSupplier;
import java.util.function.Supplier;
import com.cloudwebrtc.webrtc.utils.ConstraintsMap;
import com.cloudwebrtc.webrtc.utils.EglUtils;

import java.util.List;

import org.webrtc.EglBase;
import org.webrtc.MediaStream;
import org.webrtc.RendererCommon.RendererEvents;
import org.webrtc.VideoTrack;
import org.webrtc.ThreadUtils;

import io.flutter.plugin.common.EventChannel;
import io.flutter.view.TextureRegistry;
import io.flutter.plugin.common.MethodChannel.Result;

public class FlutterRTCVideoRenderer implements EventChannel.StreamHandler {

    private static final String TAG = FlutterWebRTCPlugin.TAG;
    private final TextureRegistry.SurfaceProducer producer;
    private int id = -1;
    private String mediaStreamId;

    private String ownerTag;
    interface OwnerResolver { NativeMediaLifecycle.Owner resolve(VideoTrack track, String ownerTag); }
    private final OwnerResolver owners;
    private final NativeRendererBinding binding;
    private final Handler platform = new Handler(Looper.getMainLooper());
    private final NativeMediaLifecycle.Owner outputOwner = new NativeMediaLifecycle.Owner(Runnable::run,
        event -> Log.d(TAG, "[OpenIMRtcNative] rendererOutput#" + Integer.toHexString(System.identityHashCode(this)) + " " + event));

    public void Dispose() {
        Dispose(null, () -> {});
    }

    void Dispose(Result result, Runnable onDisposed) {
        ThreadUtils.checkIsOnMainThread();
        disposing = true;
        sinkGeneration++;
        RendererEpoch epoch = desiredEpoch;
        if (epoch != null) epoch.stopFrameAdmission();
        report(binding.dispose(), result, onDisposed, "renderer.dispose");
    }

    private final java.util.Set<RendererEpoch> epochs = java.util.concurrent.ConcurrentHashMap.newKeySet();
    private volatile RendererEpoch desiredEpoch;
    private volatile boolean disposing;
    private volatile long sinkGeneration;
    private final AtomicReference<Throwable> sourceCallbackFailure = new AtomicReference<>();

    // No AnyThreadSink here: the epoch AND raw subscription guard run at the
    // final main delivery, not before a second adapter queues an unguarded task.
    private void postEvent(RendererEpoch epoch, Object event) {
        final EventChannel.EventSink raw = eventSink;
        final long subscription = sinkGeneration;
        if (raw == null) return;
        if (!platform.post(() -> {
            if (epoch.isCurrent() && raw == eventSink && subscription == sinkGeneration) {
                try { raw.success(event); }
                catch (Throwable error) { safeFailure("event.delivery", error); }
            }
        })) safeFailure("event.post", new java.util.concurrent.RejectedExecutionException("Renderer event rejected"));
    }

    private RendererEvents eventsFor(RendererEpoch epoch) {
        return new RendererEvents() {
            private int rotation = -1, width, height;
            public void onFirstFrameRendered() {
                ConstraintsMap event = new ConstraintsMap();
                event.putString("event", "didFirstFrameRendered"); event.putInt("id", id);
                postEvent(epoch, event.toMap());
            }
            public void onFrameResolutionChanged(int w, int h, int angle) {
                if (width != w || height != h) {
                    ConstraintsMap event = new ConstraintsMap();
                    event.putString("event", "didTextureChangeVideoSize"); event.putInt("id", id);
                    event.putDouble("width", (double) w); event.putDouble("height", (double) h);
                    width = w; height = h; postEvent(epoch, event.toMap());
                }
                if (rotation != angle) {
                    ConstraintsMap event = new ConstraintsMap();
                    event.putString("event", "didTextureChangeRotation"); event.putInt("id", id); event.putInt("rotation", angle);
                    rotation = angle; postEvent(epoch, event.toMap());
                }
            }
        };
    }

    private CompletableFuture<Void> onMain(Runnable action) {
        CompletableFuture<Void> done = new CompletableFuture<>();
        if (!platform.post(() -> {
            try { action.run(); done.complete(null); } catch (Throwable failure) { done.completeExceptionally(failure); }
        })) done.completeExceptionally(new java.util.concurrent.RejectedExecutionException("Renderer main operation rejected"));
        return done;
    }
    private CompletableFuture<Void> onMainFuture(Supplier<CompletableFuture<Void>> action) {
        CompletableFuture<Void> done = new CompletableFuture<>();
        if (!platform.post(() -> {
            try { action.get().whenComplete((unused, failure) -> {
                if (failure == null) done.complete(null); else done.completeExceptionally(NativeMediaLifecycle.unwrap(failure));
            }); } catch (Throwable failure) { done.completeExceptionally(failure); }
        })) done.completeExceptionally(new java.util.concurrent.RejectedExecutionException("Renderer main operation rejected"));
        return done;
    }
    private void safeFailure(String phase, Throwable failure) {
        try { Log.w(TAG, "[OpenIMRtcNative] phase=" + phase + " failure=" + failure.getClass().getSimpleName()); }
        catch (Throwable ignored) { /* Observations do not own physical cleanup. */ }
    }

    /** One immutable native source/proxy/mailbox/EGL lifetime. No renderer reuse after retirement. */
    private final class RendererEpoch implements NativeRendererBinding.Track {
        final VideoTrack track;
        final NativeMediaLifecycle.Owner owner;
        final NativeFrameHandoff handoff;
        final AtomicReference<Throwable> firstFailure = new AtomicReference<>();
        private final Object surfaceState = new Object();
        private volatile SurfaceTextureRenderer renderer;
        private volatile boolean initialized, retired;
        private boolean sinkRegistered;
        private boolean surfaceAvailable = true;
        private int surfacePending;
        private NativeFrameHandoff.Pause pause;
        private CompletableFuture<Void> surfaceTail = CompletableFuture.completedFuture(null);
        private CompletableFuture<Void> preparation, retirement;

        RendererEpoch(VideoTrack track, NativeMediaLifecycle.Owner owner) {
            this.track = track; this.owner = owner;
            handoff = new NativeFrameHandoff(frame -> {
                SurfaceTextureRenderer output = renderer;
                if (output == null) throw new NativeMediaLifecycle.Failure("RTC_RENDERER_UNAVAILABLE", "Renderer epoch has no initialized output");
                output.onFrame(frame);
            }, this::recordFrameFailure, action -> platform.post(action));
            pause = handoff.pause();
        }
        public Object key() { return track; }
        boolean isCurrent() { return desiredEpoch == this && !disposing && !retired && firstFailure.get() == null; }

        public CompletableFuture<Void> prepare(BooleanSupplier current) {
            final CompletableFuture<Void> done;
            synchronized (this) {
                if (preparation != null) return preparation;
                done = new CompletableFuture<>(); preparation = done;
            }
            onMain(() -> {
                if (!current.getAsBoolean() || disposing || retired) return;
                SurfaceTextureRenderer next = new SurfaceTextureRenderer("");
                // Publish ownership before any init/callback can fail or re-enter.
                renderer = next;
                next.setCleanupOwner(outputOwner);
                next.setCleanupFailureHandler(() -> fail("surface.physical", new NativeMediaLifecycle.Failure("RTC_MEDIA_FAILED", "Surface cleanup failed")));
                next.setSurfaceLifecycle(new SurfaceTextureRenderer.SurfaceLifecycle() {
                    public void available() { surfaceAvailable(); }
                    public void cleanup() { surfaceCleanup(); }
                });
                Throwable prior = firstFailure.get();
                if (prior != null) throw new java.util.concurrent.CompletionException(prior);
                if (!current.getAsBoolean() || disposing || retired) return;
                next.init(EglUtils.getRootEglBaseContext(), eventsFor(this));
                // A reentrant or concurrent replacement can retire a fully initialized
                // but never-attached EGL epoch. Its lease still owns actual release.
                if (!current.getAsBoolean() || disposing || retired) return;
                next.surfaceCreated(producer);
                initialized = true;
            }).whenComplete((unused, error) -> {
                if (error == null) done.complete(null);
                else { fail("epoch.prepare", NativeMediaLifecycle.unwrap(error)); done.completeExceptionally(NativeMediaLifecycle.unwrap(error)); }
            });
            return done;
        }
        public void attach() {
            if (!isCurrent()) return;
            if (!initialized) throw new NativeMediaLifecycle.Failure("RTC_RENDERER_UNAVAILABLE", "Renderer output was not prepared");
            track.addSink(handoff);
            sinkRegistered = true;
            NativeFrameHandoff.Pause token;
            boolean available;
            synchronized (surfaceState) { token = pause; available = surfaceAvailable; }
            if (isCurrent() && available && !handoff.resume(token)) {
                throw new NativeMediaLifecycle.Failure("RTC_RENDERER_UNAVAILABLE", "Current frame mailbox could not start");
            }
        }
        public void detach() {
            if (!sinkRegistered) return;
            final long started = System.nanoTime(); boolean returned = false;
            traceRemoveSink("javaRemoveSinkBegin", track, this, owner, handoff, started, false);
            try { track.removeSink(handoff); sinkRegistered = false; returned = true; }
            finally { traceRemoveSink("javaRemoveSinkEnd", track, this, owner, handoff, started, returned); }
        }
        public void stopFrameAdmission() {
            retired = true;
            handoff.seal();
            SurfaceTextureRenderer output = renderer;
            if (output != null) output.beginFinalRelease();
        }
        public CompletableFuture<Void> retireAfterNativeDetach() {
            final CompletableFuture<Void> done, surface;
            synchronized (this) {
                if (retirement != null) return retirement;
                done = new CompletableFuture<>(); retirement = done;
            }
            stopFrameAdmission();
            synchronized (surfaceState) { surface = surfaceTail; }
            CompletableFuture.allOf(handoff.seal(), surface).thenCompose(unused -> {
                final SurfaceTextureRenderer output = renderer;
                if (output == null) return CompletableFuture.completedFuture(null);
                return outputOwner.submit("renderer.epoch.release", () -> {
                    output.throwIfCleanupFailed(); output.release();
                }, true);
            }).whenComplete((unused, error) -> {
                if (error == null) { epochs.remove(this); done.complete(null); }
                else { Throwable cause = NativeMediaLifecycle.unwrap(error); fail("epoch.retire", cause); done.completeExceptionally(cause); }
            });
            return done;
        }
        private void recordFrameFailure(Throwable error) {
            // Source callback path: only bounded metadata and post, never owner/log.
            if (!firstFailure.compareAndSet(null, error)) return;
            sourceCallbackFailure.compareAndSet(null, error);
            platform.post(() -> fail("frame.delivery", error));
        }
        private void fail(String phase, Throwable error) {
            firstFailure.compareAndSet(null, error);
            safeFailure(phase, error);
            owner.quarantine(); binding.quarantine();
        }
        private void surfaceCleanup() {
            if (!isCurrent()) return;
            final SurfaceTextureRenderer output = renderer;
            output.markSurfaceUnavailable();
            final NativeFrameHandoff.Pause token = handoff.pause();
            final CompletableFuture<Void> before, done = new CompletableFuture<>();
            boolean overflow;
            synchronized (surfaceState) {
                overflow = surfacePending >= 8;
                if (overflow) before = surfaceTail;
                else { surfacePending++; surfaceAvailable = false; pause = token; before = surfaceTail; surfaceTail = done; }
            }
            if (overflow) {
                fail("surface.capacity", new NativeMediaLifecycle.Failure("RTC_RENDERER_BUSY", "Bounded surface transition queue is occupied"));
                return;
            }
            // Normal manual lifecycle does not enter this path in the fixed embedding.
            // This composition is not an acknowledgement to Flutter's void callback.
            before.thenCompose(unused -> token.physicalDrain).thenCompose(unused -> onMainFuture(() -> {
                if (retired) return CompletableFuture.completedFuture(null);
                return output.releaseSurfacePhysical();
            })).whenComplete((unused, error) -> {
                synchronized (surfaceState) { surfacePending--; }
                if (error == null) done.complete(null);
                else { Throwable cause = NativeMediaLifecycle.unwrap(error); fail("surface.cleanup", cause); done.completeExceptionally(cause); }
            });
            NativeMediaLifecycle.report(done, new NativeMediaLifecycle.Response() {
                public void success() {}
                public void error(String code, String message) { Log.w(TAG, "[OpenIMRtcNative] operation=surface.cleanup phase=callerError code=" + code); }
            }, () -> outputOwner.callerTimedOut(done, "surface.cleanup"));
        }
        private void surfaceAvailable() {
            if (!isCurrent()) return;
            final CompletableFuture<Void> before;
            final NativeFrameHandoff.Pause token;
            synchronized (surfaceState) { surfaceAvailable = true; before = surfaceTail; token = pause; }
            before.whenComplete((unused, error) -> {
                if (error != null) return;
                onMain(() -> {
                    synchronized (surfaceState) {
                        if (!isCurrent() || !surfaceAvailable || token != pause) return;
                    }
                    renderer.markSurfaceAvailable();
                    if (sinkRegistered && !handoff.resume(token) && isCurrent()) {
                        fail("surface.resume", new NativeMediaLifecycle.Failure("RTC_RENDERER_UNAVAILABLE", "Mailbox resume failed"));
                    }
                }).whenComplete((ignored, mainError) -> { if (mainError != null) fail("surface.available", NativeMediaLifecycle.unwrap(mainError)); });
            });
        }
    }

    /**
     * The {@code VideoTrack}, if any, rendered by this {@code FlutterRTCVideoRenderer}.
     */
    private VideoTrack videoTrack;
    private String videoTrackId;

    EventChannel eventChannel;
    volatile EventChannel.EventSink eventSink;

    public FlutterRTCVideoRenderer(TextureRegistry.SurfaceProducer producer, OwnerResolver owners) {
        ThreadUtils.checkIsOnMainThread();
        this.owners = owners;
        this.eventSink = null;
        this.producer = producer;
        this.ownerTag = null;
        binding = new NativeRendererBinding(action -> {
            if (!platform.post(action)) throw new java.util.concurrent.RejectedExecutionException("Platform looper rejected renderer finalization");
        }, new NativeRendererBinding.Output() {
            public void releaseEgl() {
                // Each source already retired its own EGL exactly once through its lease.
                if (!epochs.isEmpty()) throw new NativeMediaLifecycle.Failure("RTC_RENDERER_UNAVAILABLE", "Renderer epochs remain physically owned");
            }
            public void releaseProducerOnMain() {
                if (eventChannel != null) eventChannel.setStreamHandler(null);
                sinkGeneration++; eventSink = null; desiredEpoch = null;
                producer.release();
            }
        }, outputOwner, event -> Log.d(TAG, "[OpenIMRtcNative] renderer#" + Integer.toHexString(System.identityHashCode(this)) + " " + event));
    }

    public void setEventChannel(EventChannel eventChannel) {
        ThreadUtils.checkIsOnMainThread();
        this.eventChannel = eventChannel;
    }

    public void setId(int id) {
        ThreadUtils.checkIsOnMainThread();
        this.id = id;
    }

    @Override
    public void onListen(Object o, EventChannel.EventSink sink) {
        ThreadUtils.checkIsOnMainThread();
        sinkGeneration++; eventSink = sink;
    }

    @Override
    public void onCancel(Object o) {
        ThreadUtils.checkIsOnMainThread();
        sinkGeneration++; eventSink = null;
    }

    /**
     * Stops rendering {@link #videoTrack} and releases the associated acquired
     * resources (if rendering is in progress).
     */

    /**
     * Sets the {@code MediaStream} to be rendered by this {@code FlutterRTCVideoRenderer}.
     * The implementation renders the first {@link VideoTrack}, if any, of the
     * specified {@code mediaStream}.
     *
     * @param mediaStream The {@code MediaStream} to be rendered by this
     *                    {@code FlutterRTCVideoRenderer} or {@code null}.
     */
    public void setStream(MediaStream mediaStream, String ownerTag) {
        setStream(mediaStream, ownerTag, (Result) null);
    }
    void setStream(MediaStream mediaStream, String ownerTag, Result result) {
        VideoTrack videoTrack;
        if (mediaStream == null) {
            videoTrack = null;
        } else {
            List<VideoTrack> videoTracks = mediaStream.videoTracks;

            videoTrack = videoTracks.isEmpty() ? null : videoTracks.get(0);
        }

        setVideoTrack(videoTrack, mediaStream == null ? null : mediaStream.getId(), ownerTag, result);
    }
   /**
     * Sets the {@code MediaStream} to be rendered by this {@code FlutterRTCVideoRenderer}.
     * The implementation renders the first {@link VideoTrack}, if any, of the
     * specified trackId
     *
     * @param mediaStream The {@code MediaStream} to be rendered by this
     *                    {@code FlutterRTCVideoRenderer} or {@code null}.
     * @param trackId The {@code trackId} to be rendered by this
     *                    {@code FlutterRTCVideoRenderer} or {@code null}.
     */
    public void setStream(MediaStream mediaStream,String trackId, String ownerTag) {
        setStream(mediaStream, trackId, ownerTag, null);
    }
    void setStream(MediaStream mediaStream,String trackId, String ownerTag, Result result) {
        VideoTrack videoTrack;
        if (mediaStream == null) {
            videoTrack = null;
        } else {
            List<VideoTrack> videoTracks = mediaStream.videoTracks;

            videoTrack = videoTracks.isEmpty() ? null : videoTracks.get(0);

            for (VideoTrack track : videoTracks){
                if (track.id().equals(trackId)){
                    videoTrack = track;
                }
            }
        }

        setVideoTrack(videoTrack, mediaStream == null ? null : mediaStream.getId(), ownerTag, result);
    }

    /**
     * Sets a video track already resolved from its owning PeerConnection.
     *
     * The stream id is retained only for renderer lifecycle bookkeeping. The
     * MediaStream wrapper itself may be replaced or disposed by a Unified Plan
     * renegotiation while the logical stream and track ids remain unchanged.
     */
    public void setTrack(
            VideoTrack videoTrack, String mediaStreamId, String ownerTag) {
        setTrack(videoTrack, mediaStreamId, ownerTag, null);
    }
    void setTrack(VideoTrack videoTrack, String mediaStreamId, String ownerTag, Result result) {
        setVideoTrack(videoTrack, mediaStreamId, ownerTag, result);
    }

    /**
     * Sets the {@code VideoTrack} to be rendered by this {@code FlutterRTCVideoRenderer}.
     *
     * @param videoTrack The {@code VideoTrack} to be rendered by this
     *                   {@code FlutterRTCVideoRenderer} or {@code null}.
     */
    public void setVideoTrack(VideoTrack videoTrack) {
        setVideoTrack(videoTrack, null);
    }

    /**
     * Starts rendering {@link #videoTrack} if rendering is not in progress and
     * all preconditions for the start of rendering are met.
     */
    private void setVideoTrack(VideoTrack next, Result result) {
        setVideoTrack(next, mediaStreamId, ownerTag, result);
    }
    private void setVideoTrack(VideoTrack next, String requestedStream, String requestedOwner, Result result) {
        ThreadUtils.checkIsOnMainThread();
        if (next != null && sourceCallbackFailure.get() != null) {
            if (result != null) result.error("RTC_MEDIA_UNAVAILABLE", "Renderer source callback has a retained failure", null);
            return;
        }
        NativeMediaLifecycle.Owner owner = next == null ? null : owners.resolve(next, requestedOwner);
        if (next != null && owner == null) {
            if (result != null) result.error("RTC_MEDIA_UNAVAILABLE", "Track owner is closing or unavailable", null);
            return;
        }
        RendererEpoch epoch = next == null ? null : new RendererEpoch(next, owner);
        if (epoch != null) epochs.add(epoch);
        NativeRendererBinding.SetRequest request = binding.requestSet(owner, epoch);
        if (request.accepted) {
            desiredEpoch = epoch;
            this.mediaStreamId = requestedStream; this.ownerTag = requestedOwner;
            this.videoTrack = next; this.videoTrackId = next == null ? null : next.id();
        } else if (epoch != null) epochs.remove(epoch); // rejected before allocation/admission
        report(request.physical, result, () -> {}, "renderer.set");
    }

    // Identity hashes are correlation hints, not unique IDs or native pointers.
    private void traceRemoveSink(String phase, VideoTrack track, Object bindingTrack,
            NativeMediaLifecycle.Owner owner, Object sink, long started, boolean returned) {
        try {
            Log.d(TAG, "[OpenIMRtcNative] operation=renderer.detach phase=" + phase
                + " rendererJava#" + Integer.toHexString(System.identityHashCode(this))
                + " bindingTrackJava#" + Integer.toHexString(System.identityHashCode(bindingTrack))
                + " trackJava#" + Integer.toHexString(System.identityHashCode(track))
                + " sinkJava#" + Integer.toHexString(System.identityHashCode(sink))
                + " ownerJava#" + Integer.toHexString(System.identityHashCode(owner))
                + " returned=" + returned + " elapsedMs="
                + java.util.concurrent.TimeUnit.NANOSECONDS.toMillis(System.nanoTime() - started));
        } catch (Throwable ignored) { /* Diagnostics never own media. */ }
    }

    private void report(java.util.concurrent.CompletableFuture<Void> physical, Result result, Runnable completed, String operation) {
        // Physical ownership is independent of the caller's deadline. A late,
        // genuine completion must still remove the exact renderer tombstone.
        physical.whenComplete((unused, failure) -> {
            if (failure == null && !platform.post(completed)) {
                Log.w(TAG, "Platform looper rejected completed renderer bookkeeping");
            }
        });
        NativeMediaLifecycle.report(physical, new NativeMediaLifecycle.Response() {
            public void success() { platform.post(() -> { if (result != null) result.success(null); }); }
            public void error(String code, String message) { platform.post(() -> {
                Log.w(TAG, "[OpenIMRtcNative] operation=" + operation + " phase=callerError code=" + code);
                if (result != null) result.error(code, message, null);
            }); }
        }, () -> binding.callerTimedOut(physical, operation));
    }

    public boolean checkMediaStream(String id, String ownerTag) {
        if (null == id || null == mediaStreamId || ownerTag == null || !ownerTag.equals(this.ownerTag)) {
            return false;
        }
        return id.equals(mediaStreamId);
    }

    java.util.concurrent.CompletableFuture<Void> detachStream(String streamId) {
        if (!checkMediaStream(streamId, "local")) return java.util.concurrent.CompletableFuture.completedFuture(null);
        desiredEpoch = null;
        mediaStreamId = null; videoTrack = null; videoTrackId = null; ownerTag = null;
        return binding.set(null, null);
    }

    public boolean checkVideoTrack(String id, String ownerTag) {
        if (null == id || null == videoTrackId || ownerTag == null || !ownerTag.equals(this.ownerTag)) {
            return false;
        }
        return id.equals(videoTrackId);
    }
}
