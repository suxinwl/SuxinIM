package com.cloudwebrtc.webrtc;

import android.util.Log;
import android.graphics.SurfaceTexture;
import android.view.Surface;
import android.os.Handler;
import android.os.Looper;

import com.cloudwebrtc.webrtc.utils.AnyThreadSink;
import com.cloudwebrtc.webrtc.utils.ConstraintsMap;
import com.cloudwebrtc.webrtc.utils.EglUtils;

import java.util.List;

import org.webrtc.EglBase;
import org.webrtc.MediaStream;
import org.webrtc.RendererCommon.RendererEvents;
import org.webrtc.VideoTrack;

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
        surfaceTextureRenderer.beginFinalRelease();
        report(binding.dispose(), result, onDisposed, "renderer.dispose");
    }

    /**
     * The {@code RendererEvents} which listens to rendering events reported by
     * {@link #surfaceTextureRenderer}.
     */
    private RendererEvents rendererEvents;

    private void listenRendererEvents() {
        rendererEvents = new RendererEvents() {
            private int _rotation = -1;
            private int _width = 0, _height = 0;

            @Override
            public void onFirstFrameRendered() {
                ConstraintsMap params = new ConstraintsMap();
                params.putString("event", "didFirstFrameRendered");
                params.putInt("id", id);
                if (eventSink != null) {
                    eventSink.success(params.toMap());
                }
            }

            @Override
            public void onFrameResolutionChanged(
                    int videoWidth, int videoHeight,
                    int rotation) {

                if (eventSink != null) {
                    if (_width != videoWidth || _height != videoHeight) {
                        ConstraintsMap params = new ConstraintsMap();
                        params.putString("event", "didTextureChangeVideoSize");
                        params.putInt("id", id);
                        params.putDouble("width", (double) videoWidth);
                        params.putDouble("height", (double) videoHeight);
                        _width = videoWidth;
                        _height = videoHeight;
                        eventSink.success(params.toMap());
                    }

                    if (_rotation != rotation) {
                        ConstraintsMap params2 = new ConstraintsMap();
                        params2.putString("event", "didTextureChangeRotation");
                        params2.putInt("id", id);
                        params2.putInt("rotation", rotation);
                        _rotation = rotation;
                        eventSink.success(params2.toMap());
                    }
                }
            }
        };
    }

    private final SurfaceTextureRenderer surfaceTextureRenderer;

    /**
     * The {@code VideoTrack}, if any, rendered by this {@code FlutterRTCVideoRenderer}.
     */
    private VideoTrack videoTrack;
    private String videoTrackId;

    EventChannel eventChannel;
    volatile EventChannel.EventSink eventSink;

    public FlutterRTCVideoRenderer(TextureRegistry.SurfaceProducer producer, OwnerResolver owners) {
        this.owners = owners;
        this.surfaceTextureRenderer = new SurfaceTextureRenderer("");
        surfaceTextureRenderer.setCleanupOwner(outputOwner);
        listenRendererEvents();
        surfaceTextureRenderer.init(EglUtils.getRootEglBaseContext(), rendererEvents);
        surfaceTextureRenderer.surfaceCreated(producer);

        this.eventSink = null;
        this.producer = producer;
        this.ownerTag = null;
        binding = new NativeRendererBinding(action -> {
            if (!platform.post(action)) throw new java.util.concurrent.RejectedExecutionException("Platform looper rejected renderer finalization");
        }, new NativeRendererBinding.Output() {
            public void releaseEgl() { surfaceTextureRenderer.throwIfCleanupFailed(); surfaceTextureRenderer.release(); }
            public void releaseProducerOnMain() {
                if (eventChannel != null) eventChannel.setStreamHandler(null);
                eventSink = null;
                producer.release();
            }
        }, outputOwner, event -> Log.d(TAG, "[OpenIMRtcNative] renderer#" + Integer.toHexString(System.identityHashCode(this)) + " " + event));
        surfaceTextureRenderer.setCleanupFailureHandler(binding::quarantine);
    }

    public void setEventChannel(EventChannel eventChannel) {
        this.eventChannel = eventChannel;
    }

    public void setId(int id) {
        this.id = id;
    }

    @Override
    public void onListen(Object o, EventChannel.EventSink sink) {
        eventSink = new AnyThreadSink(sink);
    }

    @Override
    public void onCancel(Object o) {
        eventSink = null;
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
        NativeMediaLifecycle.Owner owner = next == null ? null : owners.resolve(next, requestedOwner);
        if (next != null && owner == null) {
            if (result != null) result.error("RTC_MEDIA_UNAVAILABLE", "Track owner is closing or unavailable", null);
            return;
        }
        NativeRendererBinding.Track track = next == null ? null : new NativeRendererBinding.Track() {
            public Object key() { return next; }
            public void attach() {
                surfaceTextureRenderer.resetFrameState();
                next.addSink(surfaceTextureRenderer);
            }
            public void detach() {
                final long started = System.nanoTime();
                boolean returned = false;
                traceRemoveSink("javaRemoveSinkBegin", next, this, owner, started, false);
                try { next.removeSink(surfaceTextureRenderer); returned = true; }
                finally { traceRemoveSink("javaRemoveSinkEnd", next, this, owner, started, returned); }
            }
        };
        NativeRendererBinding.SetRequest request = binding.requestSet(owner, track);
        if (request.accepted) {
            this.mediaStreamId = requestedStream; this.ownerTag = requestedOwner;
            this.videoTrack = next; this.videoTrackId = next == null ? null : next.id();
        }
        report(request.physical, result, () -> {}, "renderer.set");
    }

    // Identity hashes are correlation hints, not unique IDs or native pointers.
    private void traceRemoveSink(String phase, VideoTrack track, Object bindingTrack,
            NativeMediaLifecycle.Owner owner, long started, boolean returned) {
        try {
            Log.d(TAG, "[OpenIMRtcNative] operation=renderer.detach phase=" + phase
                + " rendererJava#" + Integer.toHexString(System.identityHashCode(this))
                + " bindingTrackJava#" + Integer.toHexString(System.identityHashCode(bindingTrack))
                + " trackJava#" + Integer.toHexString(System.identityHashCode(track))
                + " sinkJava#" + Integer.toHexString(System.identityHashCode(surfaceTextureRenderer))
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
