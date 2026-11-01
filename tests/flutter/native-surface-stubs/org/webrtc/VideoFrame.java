package org.webrtc;
public final class VideoFrame {
  public static final class Buffer { private final int width, height; Buffer(int w, int h) { width=w; height=h; } public int getWidth(){return width;} public int getHeight(){return height;} }
  private final Buffer buffer;
  public VideoFrame(int width, int height) { buffer = new Buffer(width,height); }
  public int getRotatedWidth() { return buffer.getWidth(); }
  public int getRotatedHeight() { return buffer.getHeight(); }
  public int getRotation() { return 0; }
  public Buffer getBuffer() { return buffer; }
}
