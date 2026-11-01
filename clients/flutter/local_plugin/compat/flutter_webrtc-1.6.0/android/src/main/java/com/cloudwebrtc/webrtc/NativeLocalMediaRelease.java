package com.cloudwebrtc.webrtc;

import java.util.ArrayList;
import java.util.Collection;
import java.util.List;
import java.util.Set;
import java.util.Collections;
import java.util.IdentityHashMap;
import java.util.HashMap;
import java.util.Map;
import java.lang.ref.WeakReference;
import java.lang.ref.ReferenceQueue;

/** Called only by the serial local-media owner. Wrapper identity, not track ID,
 * determines ownership. Native failures are terminal: never repeat unknown JNI. */
final class NativeLocalMediaRelease<T, S> {
  interface Operations<T, S> {
    List<T> ownedTracks(S stream);
    List<T> borrowedTracks(S stream);
    void remove(S stream, T track);
    void stop(T track);
    void disposeTrack(T track);
    void disposeStream(S stream);
  }
  private final WeakIdentitySet<T> releasedTracks = new WeakIdentitySet<>();
  private final Set<T> failedTracks = Collections.synchronizedSet(Collections.newSetFromMap(new IdentityHashMap<T,Boolean>()));
  private final WeakIdentitySet<S> releasedStreams = new WeakIdentitySet<>();
  private final Set<S> failedStreams = Collections.synchronizedSet(Collections.newSetFromMap(new IdentityHashMap<S,Boolean>()));
  boolean isTrackReleased(T track) { return releasedTracks.contains(track); }
  boolean isTrackFailed(T track) { return failedTracks.contains(track); }
  boolean isStreamReleased(S stream) { return releasedStreams.contains(stream); }

  void releaseUnpublishedEmpty(S stream, Operations<T, S> ops) {
    if (!ops.ownedTracks(stream).isEmpty() || !ops.borrowedTracks(stream).isEmpty()) {
      throw new IllegalStateException("Independent permission cleanup is restricted to an unpublished empty stream");
    }
    // The same terminal/released identity guards apply to this special path.
    releaseStream(stream, Collections.singletonList(stream), ops);
  }

  void releaseTrack(T track, Collection<S> streams, Operations<T, S> ops) {
    if (releasedTracks.contains(track)) return;
    if (failedTracks.contains(track)) throw new IllegalStateException("Local track cleanup previously failed; native wrapper remains quarantined");
    try {
      for (S stream : streams) {
        if (!releasedStreams.contains(stream)
            && (ops.ownedTracks(stream).contains(track) || ops.borrowedTracks(stream).contains(track))) ops.remove(stream, track);
      }
      ops.stop(track);
      ops.disposeTrack(track);
      releasedTracks.add(track);
    } catch (RuntimeException failure) { failedTracks.add(track); throw failure; }
  }

  void releaseStream(S stream, Collection<S> streams, Operations<T, S> ops) {
    if (releasedStreams.contains(stream)) return;
    if (failedStreams.contains(stream)) throw new IllegalStateException("Local stream cleanup previously failed; native wrapper remains quarantined");
    RuntimeException first = null;
    List<T> owned = new ArrayList<>(ops.ownedTracks(stream));
    List<T> borrowed = new ArrayList<>(ops.borrowedTracks(stream));
    // Empty the wrapper before MediaStream.dispose: the fixed AAR would
    // otherwise implicitly dispose tracks that another stream still owns.
    for (T track : concat(owned, borrowed)) {
      if (failedTracks.contains(track)) {
        if (first == null) first = new IllegalStateException("Local track cleanup previously failed; do not repeat unknown JNI");
        continue;
      }
      if (releasedTracks.contains(track)) continue;
      try { ops.remove(stream, track); }
      catch (RuntimeException failure) { failedTracks.add(track); if (first == null) first = failure; }
    }
    for (T track : owned) {
      boolean anotherOwner = false;
      for (S other : streams) {
        if (other != stream && !releasedStreams.contains(other) && ops.ownedTracks(other).contains(track)) { anotherOwner = true; break; }
      }
      if (!anotherOwner) {
        try { releaseTrack(track, streams, ops); }
        catch (RuntimeException failure) { if (first == null) first = failure; }
      }
    }
    if (first == null) {
      try { ops.disposeStream(stream); releasedStreams.add(stream); }
      catch (RuntimeException failure) { first = failure; }
    }
    if (first != null) { failedStreams.add(stream); throw first; }
  }
  private List<T> concat(List<T> first, List<T> second) {
    List<T> tracks = new ArrayList<>(first); tracks.addAll(second); return tracks;
  }
  /** Success must not pin every historical native wrapper for plugin life. */
  private static final class WeakIdentitySet<V> {
    private final ReferenceQueue<V> collected = new ReferenceQueue<>();
    private final Map<IdentityReference<V>, Boolean> values = new HashMap<>();
    private void drain() { java.lang.ref.Reference<? extends V> key; while ((key = collected.poll()) != null) values.remove(key); }
    synchronized boolean contains(V value) { drain(); return values.containsKey(new IdentityReference<>(value, null)); }
    synchronized void add(V value) { drain(); values.put(new IdentityReference<>(value, collected), Boolean.TRUE); }
  }
  private static final class IdentityReference<V> extends WeakReference<V> {
    private final int hash;
    IdentityReference(V value, ReferenceQueue<V> queue) { super(value, queue); hash = System.identityHashCode(value); }
    public int hashCode() { return hash; }
    public boolean equals(Object other) {
      if (this == other) return true;
      if (!(other instanceof IdentityReference)) return false;
      Object value = get(); return value != null && value == ((IdentityReference<?>) other).get();
    }
  }
}
