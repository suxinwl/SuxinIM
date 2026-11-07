// Local decoding only. A bad/unsupported file must reject before any SDK upload.
export class MediaPreparationError extends Error {
  constructor(public readonly reason: "decode" | "timeout") {
    super(reason === "timeout" ? "MEDIA_PREPARATION_TIMEOUT" : "MEDIA_DECODE_FAILED");
    this.name = "MediaPreparationError";
  }
}

export function fileMessageErrorKey(error: unknown) {
  return error instanceof MediaPreparationError
    ? error.reason === "timeout" ? "toast.mediaPreparationTimeout" : "toast.mediaDecodeFailed"
    : "toast.uploadFailed";
}

const PREPARATION_TIMEOUT_MS = 15000;

export function readLocalImage(file: File, timeoutMs = PREPARATION_TIMEOUT_MS): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const url = URL.createObjectURL(file), image = new Image();
    const finish = (error?: MediaPreparationError) => {
      clearTimeout(timer);
      image.onload = image.onerror = null;
      URL.revokeObjectURL(url);
      if (error) reject(error); else resolve(image);
    };
    const timer = setTimeout(() => finish(new MediaPreparationError("timeout")), timeoutMs);
    image.onerror = () => finish(new MediaPreparationError("decode"));
    image.onload = () => finish(image.width > 0 && image.height > 0 ? undefined : new MediaPreparationError("decode"));
    image.src = url;
  });
}

export function readLocalVideo(file: File, timeoutMs = PREPARATION_TIMEOUT_MS): Promise<{ snapshot: File; duration: number }> {
  return new Promise((resolve, reject) => {
    const url = URL.createObjectURL(file), video = document.createElement("video");
    let finished = false;
    const finish = (error?: MediaPreparationError, result?: { snapshot: File; duration: number }) => {
      if (finished) return;
      finished = true;
      clearTimeout(timer);
      video.onloadeddata = video.onerror = null;
      video.pause();
      video.removeAttribute("src");
      video.load();
      URL.revokeObjectURL(url);
      if (error) reject(error); else resolve(result!);
    };
    const timer = setTimeout(() => finish(new MediaPreparationError("timeout")), timeoutMs);
    video.onerror = () => finish(new MediaPreparationError("decode"));
    video.onloadeddata = () => {
      if (finished) return;
      video.onloadeddata = null;
      if (!Number.isFinite(video.duration) || video.duration <= 0 || !video.videoWidth || !video.videoHeight) {
        finish(new MediaPreparationError("decode")); return;
      }
      try {
        const duration = Math.round(video.duration), canvas = document.createElement("canvas");
        canvas.width = video.videoWidth;
        canvas.height = video.videoHeight;
        const context = canvas.getContext("2d");
        if (!context) { finish(new MediaPreparationError("decode")); return; }
        context.drawImage(video, 0, 0, canvas.width, canvas.height);
        canvas.toBlob(blob => {
          if (!blob?.size) { finish(new MediaPreparationError("decode")); return; }
          finish(undefined, { duration, snapshot: new File([blob], `snapshot-${Date.now()}.png`, { type: "image/png" }) });
        }, "image/png");
      } catch { finish(new MediaPreparationError("decode")); }
    };
    video.preload = "auto";
    video.muted = true;
    video.playsInline = true;
    // Do not claim an MP4 video is audio/mp4, or depend on autoplay succeeding.
    video.src = url;
    video.load();
  });
}
