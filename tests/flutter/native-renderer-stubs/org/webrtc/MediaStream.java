package org.webrtc;
import java.util.ArrayList;
import java.util.List;
public final class MediaStream {
  private final String id;
  public final List<VideoTrack> videoTracks = new ArrayList<>();
  public MediaStream(String id) { this.id = id; }
  public String getId() { return id; }
}
