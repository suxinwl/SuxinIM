// Executed inside the test page. No application or media transport overrides.
export function captureVideoState() {
  const registry = window.coopVideoFrameMetrics ??= {
    nextID: 0,
    byElement: new WeakMap(),
  };
  return [...document.querySelectorAll('video')].map((video) => {
    let metrics = registry.byElement.get(video);
    if (!metrics) {
      metrics = { videoID: `video_${++registry.nextID}`, callbacks: 0, pending: false };
      registry.byElement.set(video, metrics);
    }
    if (typeof video.requestVideoFrameCallback === 'function' && !metrics.pending) {
      const nextFrame = (_now, metadata) => {
        metrics.pending = false;
        metrics.callbacks += 1;
        metrics.presentedFrames = metadata.presentedFrames;
        metrics.mediaTime = metadata.mediaTime;
        metrics.frameWidth = metadata.width;
        metrics.frameHeight = metadata.height;
        if (video.isConnected) {
          metrics.pending = true;
          video.requestVideoFrameCallback(nextFrame);
        }
      };
      metrics.pending = true;
      video.requestVideoFrameCallback(nextFrame);
    }
    const quality = typeof video.getVideoPlaybackQuality === 'function'
      ? video.getVideoPlaybackQuality()
      : null;
    const tracks = video.srcObject && typeof video.srcObject.getVideoTracks === 'function'
      ? video.srcObject.getVideoTracks()
      : [];
    return {
      videoID: metrics.videoID,
      width: video.videoWidth,
      height: video.videoHeight,
      ready: video.readyState,
      duration: Number.isFinite(video.duration) ? video.duration : null,
      currentTime: Number.isFinite(video.currentTime) ? video.currentTime : null,
      paused: video.paused,
      ended: video.ended,
      totalVideoFrames: quality?.totalVideoFrames ?? null,
      droppedVideoFrames: quality?.droppedVideoFrames ?? null,
      decodedVideoFrames: video.webkitDecodedFrameCount ?? null,
      frameCallbackSupported: typeof video.requestVideoFrameCallback === 'function',
      frameCallbackCount: metrics.callbacks,
      presentedFrames: metrics.presentedFrames ?? null,
      mediaTime: metrics.mediaTime ?? null,
      frameWidth: metrics.frameWidth ?? null,
      frameHeight: metrics.frameHeight ?? null,
      videoTracks: tracks.map((track) => ({
        trackID: track.id,
        enabled: track.enabled,
        muted: track.muted,
        readyState: track.readyState,
      })),
    };
  });
}
