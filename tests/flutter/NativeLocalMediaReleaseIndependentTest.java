package com.cloudwebrtc.webrtc;

import java.util.ArrayList;
import java.util.Arrays;
import java.util.IdentityHashMap;
import java.util.List;
import java.util.Map;

/** Cross-API failure/ordering boundaries against the real production helper. */
public final class NativeLocalMediaReleaseIndependentTest {
  static void check(boolean condition, String message) { if (!condition) throw new AssertionError(message); }
  static RuntimeException failure(Runnable action) {
    try { action.run(); throw new AssertionError("expected native cleanup failure"); }
    catch (RuntimeException error) { return error; }
  }
  static final class Track {
    final String id;
    int stops, disposes;
    RuntimeException disposeFailure;
    Track(String id) { this.id = id; }
  }
  static final class Stream {
    final List<Track> owned = new ArrayList<>(), borrowed = new ArrayList<>();
    int disposed;
  }
  static final class Ops implements NativeLocalMediaRelease.Operations<Track, Stream> {
    final Map<Track, Integer> removes = new IdentityHashMap<>();
    Track removeFailure;
    final RuntimeException removeError = new IllegalStateException("synthetic native remove failure");
    public List<Track> ownedTracks(Stream stream) { return new ArrayList<>(stream.owned); }
    public List<Track> borrowedTracks(Stream stream) { return new ArrayList<>(stream.borrowed); }
    public void remove(Stream stream, Track track) {
      removes.put(track, removes.getOrDefault(track, 0) + 1);
      if (track == removeFailure) throw removeError;
      boolean member = stream.owned.remove(track); member = stream.borrowed.remove(track) || member;
      check(member, "helper attempted native removal of an absent wrapper");
    }
    public void stop(Track track) { track.stops++; }
    public void disposeTrack(Track track) { track.disposes++; if (track.disposeFailure != null) throw track.disposeFailure; }
    public void disposeStream(Stream stream) {
      check(stream.owned.isEmpty() && stream.borrowed.isEmpty(), "helper let native MediaStream implicitly dispose still-owned tracks");
      stream.disposed++;
    }
  }

  static void failedExplicitTrackThenStreamMustNotRepeatUnknownNativeRemove() {
    NativeLocalMediaRelease<Track, Stream> release = new NativeLocalMediaRelease<>();
    Ops ops = new Ops(); Track bad = new Track("bad"), good = new Track("good");
    Stream stream = new Stream(); stream.owned.addAll(Arrays.asList(bad, good)); ops.removeFailure = bad;
    List<Stream> streams = Arrays.asList(stream);
    check(failure(() -> release.releaseTrack(bad, streams, ops)) == ops.removeError, "initial native remove failure was replaced");
    check(release.isTrackFailed(bad) && !release.isTrackReleased(bad), "failed wrapper was not retained as quarantined");
    failure(() -> release.releaseStream(stream, streams, ops));
    check(ops.removes.get(bad) == 1, "streamDispose repeated unknown native remove after explicit trackDispose already failed");
    check(good.stops == 1 && good.disposes == 1 && release.isTrackReleased(good), "terminal failed track skipped independent sibling cleanup");
    check(bad.stops == 0 && bad.disposes == 0 && stream.disposed == 0, "terminal failure dropped unsafe native wrapper/stream");
  }

  static void disposedFailureDoesNotPoisonDifferentWrapperWithSameId() {
    NativeLocalMediaRelease<Track, Stream> release = new NativeLocalMediaRelease<>(); Ops ops = new Ops();
    Track failed = new Track("same-id"), replacement = new Track("same-id");
    failed.disposeFailure = new IllegalStateException("synthetic native dispose failure");
    Stream old = new Stream(), fresh = new Stream(); old.owned.add(failed); fresh.owned.add(replacement);
    List<Stream> streams = Arrays.asList(old, fresh);
    check(failure(() -> release.releaseTrack(failed, streams, ops)) == failed.disposeFailure, "native dispose error swallowed");
    failure(() -> release.releaseTrack(failed, streams, ops));
    check(failed.disposes == 1 && failed.stops == 1 && release.isTrackFailed(failed), "unknown JNI disposal was retried");
    release.releaseTrack(replacement, streams, ops);
    check(replacement.disposes == 1 && replacement.stops == 1 && release.isTrackReleased(replacement), "quarantine by ID poisoned a different native wrapper");
  }

  static void borrowingStreamReleasedBeforeOwnerDoesNotStopNativeTrack() {
    NativeLocalMediaRelease<Track, Stream> release = new NativeLocalMediaRelease<>(); Ops ops = new Ops();
    Track track = new Track("borrowed-first"); Stream borrower = new Stream(), owner = new Stream();
    borrower.borrowed.add(track); owner.owned.add(track); List<Stream> streams = Arrays.asList(borrower, owner);
    release.releaseStream(borrower, streams, ops);
    check(track.stops == 0 && track.disposes == 0 && owner.owned.contains(track) && borrower.disposed == 1,
        "borrowed-first cleanup released live owner's native track");
    release.releaseStream(owner, streams, ops);
    check(track.stops == 1 && track.disposes == 1 && owner.disposed == 1, "last owning stream did not release native track once");
  }

  static Map<?, ?> weakSuccessRecords(NativeLocalMediaRelease<Track, Stream> release, String name) throws Exception {
    java.lang.reflect.Field field = NativeLocalMediaRelease.class.getDeclaredField(name); field.setAccessible(true);
    Object records = field.get(release);
    java.lang.reflect.Field values = records.getClass().getDeclaredField("values"); values.setAccessible(true);
    return (Map<?, ?>) values.get(records);
  }

  static void successHistoryUsesWeakKeysAndDrainsEnqueuedRecords() throws Exception {
    NativeLocalMediaRelease<Track, Stream> release = new NativeLocalMediaRelease<>(); Ops ops = new Ops();
    for (int i = 0; i < 32; i++) {
      Stream stream = new Stream(); stream.owned.add(new Track("historical-" + i));
      release.releaseStream(stream, Arrays.asList(stream), ops);
    }
    Map<?, ?> tracks = weakSuccessRecords(release, "releasedTracks"), streams = weakSuccessRecords(release, "releasedStreams");
    check(tracks.size() == 32 && streams.size() == 32, "test did not create repeated successful cleanup records");
    // Verify actual success-map representation rather than hoping System.gc()
    // runs. Explicitly enqueue cleared weak refs to test the production drain.
    // This is not a claim that a particular GC ran or all JNI memory is free.
    for (Map<?, ?> records : Arrays.asList(tracks, streams)) {
      for (Object key : new ArrayList<>(records.keySet())) {
        check(key instanceof java.lang.ref.WeakReference, "success tombstone strongly retained wrapper key");
        check(records.get(key) instanceof Boolean, "success tombstone value retained historical wrapper");
        java.lang.ref.WeakReference<?> weak = (java.lang.ref.WeakReference<?>) key;
        weak.clear(); weak.enqueue();
      }
    }
    release.isTrackReleased(new Track("drain-only")); release.isStreamReleased(new Stream());
    check(tracks.isEmpty() && streams.isEmpty(), "production weak identity sets did not drain collected-success records");
  }

  public static void main(String[] args) throws Exception {
    failedExplicitTrackThenStreamMustNotRepeatUnknownNativeRemove(); System.out.println("PASS failed explicit track -> stream does not retry unknown native remove, siblings still cleaned");
    disposedFailureDoesNotPoisonDifferentWrapperWithSameId(); System.out.println("PASS terminal native dispose failure retained; same-ID new wrapper remains independently releasable");
    borrowingStreamReleasedBeforeOwnerDoesNotStopNativeTrack(); System.out.println("PASS borrowed stream first preserves owning stream until final owner release");
    successHistoryUsesWeakKeysAndDrainsEnqueuedRecords(); System.out.println("PASS repeated success history uses weak identity keys and drains enqueued records; no GC/JNI claim");
    System.out.println("PASS 4 independent real local-release cross-API/reference boundaries; fake native wrappers, not Android/JNI acceptance");
  }
}
