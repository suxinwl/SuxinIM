package org.webrtc;
public final class RendererCommon {
  public interface RendererEvents { void onFirstFrameRendered(); void onFrameResolutionChanged(int width, int height, int rotation); }
}
