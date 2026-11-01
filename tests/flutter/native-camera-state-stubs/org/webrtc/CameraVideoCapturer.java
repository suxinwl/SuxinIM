package org.webrtc;

/** Only the fixed callback interface required by the actual product class. */
public interface CameraVideoCapturer {
  interface CameraEventsHandler {
    void onCameraError(String errorDescription);
    void onCameraDisconnected();
    void onCameraFreezed(String errorDescription);
    void onCameraOpening(String cameraName);
    void onFirstFrameAvailable();
    void onCameraClosed();
  }
}
