package com.cloudwebrtc.webrtc;

import java.util.*;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.atomic.AtomicInteger;

/** Executes production ownership/sequence code with fake JNI boundaries. */
public final class NativeLocalMediaReleaseTest {
  static void check(boolean value, String text) { if (!value) throw new AssertionError(text); }
  static final class Track { int stopped, disposed; boolean fail; int source; Track(int source) { this.source = source; } }
  static final class Stream { List<Track> owned = new ArrayList<>(), borrowed = new ArrayList<>(); int disposed; }
  static class Ops implements NativeLocalMediaRelease.Operations<Track, Stream> {
    int removals, sourceReleases; final Map<Integer, Integer> capturers = new HashMap<>();
    public List<Track> ownedTracks(Stream stream) { return new ArrayList<>(stream.owned); }
    public List<Track> borrowedTracks(Stream stream) { return new ArrayList<>(stream.borrowed); }
    public void remove(Stream stream, Track track) {
      removals++; if (track.fail) throw new IllegalStateException("injected native remove failure");
      stream.owned.remove(track); stream.borrowed.remove(track);
    }
    public void stop(Track track) { track.stopped++; int rest = capturers.getOrDefault(track.source, 1) - 1; capturers.put(track.source, rest); if (rest == 0) sourceReleases++; }
    public void disposeTrack(Track track) { check(track.disposed++ == 0, "track double dispose"); }
    public void disposeStream(Stream stream) { check(stream.owned.isEmpty() && stream.borrowed.isEmpty(), "implicit AAR track dispose risk"); check(stream.disposed++ == 0, "stream double dispose"); }
  }
  static Stream stream(Track track) { Stream stream = new Stream(); stream.owned.add(track); return stream; }
  static void sharedStreams() {
    NativeLocalMediaRelease<Track,Stream> release = new NativeLocalMediaRelease<>(); Ops ops = new Ops(); Track track = new Track(1); Stream a = stream(track), b = stream(track); List<Stream> all = Arrays.asList(a,b);
    release.releaseStream(a, all, ops); check(track.disposed == 0 && track.stopped == 0 && b.owned.contains(track), "first shared stream stopped active track");
    release.releaseStream(b, all, ops); release.releaseStream(a, all, ops); check(track.disposed == 1 && track.stopped == 1 && a.disposed == 1 && b.disposed == 1, "last owner did not dispose exactly once");
  }
  static void explicitTrackThenStreams() {
    NativeLocalMediaRelease<Track,Stream> release = new NativeLocalMediaRelease<>(); Ops ops = new Ops(); Track track = new Track(1); Stream a = stream(track), b = stream(track); List<Stream> all = Arrays.asList(a,b);
    release.releaseTrack(track, all, ops); release.releaseTrack(track, all, ops); release.releaseStream(a, all, ops); release.releaseStream(b, all, ops);
    check(track.disposed == 1 && track.stopped == 1 && ops.removals == 2, "explicit track cleanup left dangling/double-owned wrapper");
  }
  static void borrowedIsNotOwner() {
    NativeLocalMediaRelease<Track,Stream> release = new NativeLocalMediaRelease<>(); Ops ops = new Ops(); Track track = new Track(1); Stream a = new Stream(), b = stream(track); a.borrowed.add(track); List<Stream> all = Arrays.asList(a,b);
    release.releaseStream(a, all, ops); check(track.disposed == 0 && track.stopped == 0, "borrowed stream disposed owning track"); release.releaseStream(b, all, ops); check(track.disposed == 1, "borrowed stream prevented last release");
  }
  static void failureRetainsAndNeverRetries() {
    NativeLocalMediaRelease<Track,Stream> release = new NativeLocalMediaRelease<>(); Ops ops = new Ops(); Track bad = new Track(1), good = new Track(2); bad.fail = true; Stream a = stream(bad); a.owned.add(good); List<Stream> all = Collections.singletonList(a);
    try { release.releaseStream(a, all, ops); throw new AssertionError("failure lost"); } catch (IllegalStateException expected) {}
    int removals = ops.removals; check(a.disposed == 0 && bad.disposed == 0 && good.disposed == 1 && release.isTrackFailed(bad), "failed cleanup lost ownership or skipped other track");
    try { release.releaseStream(a, all, ops); throw new AssertionError("failed stream retried"); } catch (IllegalStateException expected) {}
    try { release.releaseTrack(bad, all, ops); throw new AssertionError("failed track retried"); } catch (IllegalStateException expected) {}
    check(ops.removals == removals, "unknown native failure was retried");
  }
  static void sharedCapturerPromotion() {
    NativeLocalMediaRelease<Track,Stream> release = new NativeLocalMediaRelease<>(); Ops ops = new Ops(); Track primary = new Track(7), clone = new Track(7); ops.capturers.put(7,2); Stream a = stream(primary), b = stream(clone); List<Stream> all = Arrays.asList(a,b);
    release.releaseStream(a, all, ops); check(primary.disposed == 1 && clone.disposed == 0 && ops.sourceReleases == 0, "primary exit released shared source");
    release.releaseStream(b, all, ops); check(clone.disposed == 1 && ops.sourceReleases == 1, "final source not released exactly once");
  }
  static void boundedPhysicalSequence() {
    NativeCleanupSequence sequence = new NativeCleanupSequence(Runnable::run); CompletableFuture<Void> blocked = new CompletableFuture<>(); AtomicInteger runs = new AtomicInteger();
    CompletableFuture<Void> first = sequence.submit(() -> { runs.incrementAndGet(); return blocked; }); List<CompletableFuture<Void>> accepted = new ArrayList<>(); accepted.add(first);
    for (int i=0;i<7;i++) accepted.add(sequence.submit(() -> { runs.incrementAndGet(); return CompletableFuture.completedFuture(null); }));
    CompletableFuture<Void> rejected = sequence.submit(() -> { throw new AssertionError("overflow ran"); });
    check(rejected.isCompletedExceptionally() && runs.get() == 1 && sequence.isPending(), "bounded queue/physical barrier broken"); blocked.complete(null); for (CompletableFuture<Void> item:accepted) item.join(); check(runs.get() == 8 && !sequence.isPending(), "sequence did not drain");
  }
  static void concurrentLastOwnerDecision() {
    NativeCleanupSequence sequence = new NativeCleanupSequence(Runnable::run); CompletableFuture<Void> blocked = new CompletableFuture<>(); sequence.submit(() -> blocked);
    NativeLocalMediaRelease<Track,Stream> release = new NativeLocalMediaRelease<>(); Ops ops = new Ops(); Track track = new Track(1); Stream a = stream(track), b = stream(track); List<Stream> all = Arrays.asList(a,b);
    CompletableFuture<Void> first = sequence.submit(() -> { release.releaseStream(a, all, ops); return CompletableFuture.completedFuture(null); });
    CompletableFuture<Void> second = sequence.submit(() -> { release.releaseStream(b, all, ops); return CompletableFuture.completedFuture(null); });
    check(track.disposed == 0, "physical barrier skipped"); blocked.complete(null); first.join(); second.join(); check(track.disposed == 1 && ops.sourceReleases == 1, "concurrent last owner leaked/double disposed");
  }
  static void failedSequenceDoesNotStartNextJNI() {
    NativeCleanupSequence sequence = new NativeCleanupSequence(Runnable::run); CompletableFuture<Void> blocked = new CompletableFuture<>(); sequence.submit(() -> blocked); AtomicInteger runs = new AtomicInteger(); CompletableFuture<Void> next=sequence.submit(() -> { runs.incrementAndGet(); return CompletableFuture.completedFuture(null); }); blocked.completeExceptionally(new IllegalStateException("unknown JNI")); check(next.isCompletedExceptionally() && runs.get()==0 && sequence.isPending(), "failure barrier advanced into later JNI");
  }
  static void shutdownWaitsForAcceptedWorkAndUsesOneReservedSlot() {
    NativeCleanupSequence sequence = new NativeCleanupSequence(Runnable::run); CompletableFuture<Void> blocked = new CompletableFuture<>(); AtomicInteger ordinary = new AtomicInteger(), finals = new AtomicInteger();
    sequence.submit(() -> { ordinary.incrementAndGet(); return blocked; });
    for (int i=0;i<7;i++) sequence.submit(() -> { ordinary.incrementAndGet(); return CompletableFuture.completedFuture(null); });
    CompletableFuture<Void> shutdown = sequence.shutdown(() -> { check(ordinary.get()==8, "shutdown inspected native owners before prior work completed"); finals.incrementAndGet(); return CompletableFuture.completedFuture(null); });
    check(shutdown == sequence.shutdown(() -> { throw new AssertionError("duplicate shutdown operation"); }), "shutdown did not share physical barrier");
    check(!shutdown.isDone() && finals.get()==0, "shutdown advanced past blocked physical cleanup");
    check(sequence.submit(() -> { throw new AssertionError("work accepted after shutdown"); }).isCompletedExceptionally(), "shutdown accepted a new native operation");
    blocked.complete(null); shutdown.join(); check(finals.get()==1, "shutdown did not execute exactly once after bounded prior work");
    NativeCleanupSequence failed = new NativeCleanupSequence(Runnable::run); CompletableFuture<Void> unknown = new CompletableFuture<>(); failed.submit(() -> unknown);
    CompletableFuture<Void> noRetry = failed.shutdown(() -> { throw new AssertionError("shutdown advanced past unknown failed native barrier"); });
    unknown.completeExceptionally(new IllegalStateException("unknown native result")); check(noRetry.isCompletedExceptionally(), "failed shutdown barrier was hidden");
  }
  static void latePermissionEmptyStreamCanFinishAfterShutdown() {
    NativeCleanupSequence sequence = new NativeCleanupSequence(Runnable::run); sequence.shutdown(() -> CompletableFuture.completedFuture(null)).join();
    check(sequence.submit(() -> { throw new AssertionError("closed ordinary path executed"); }).isCompletedExceptionally(), "shutdown was reopened");
    NativeMediaLifecycle.Owner owner = new NativeMediaLifecycle.Owner(Runnable::run, Runnable::run, 8); owner.closeForNewWork();
    NativeLocalMediaRelease<Track,Stream> release = new NativeLocalMediaRelease<>(); Ops ops = new Ops(); Stream late = new Stream();
    owner.submit(() -> release.releaseUnpublishedEmpty(late, ops), true).join();
    // The plugin's prior snapshot may also contain this exact wrapper.
    owner.submit(() -> release.releaseStream(late, Collections.singletonList(late), ops), true).join();
    check(late.disposed==1, "late/snapshot empty stream was leaked or disposed twice");
    Stream snapshotFirst = new Stream();
    owner.submit(() -> release.releaseStream(snapshotFirst, Collections.singletonList(snapshotFirst), ops), true).join();
    owner.submit(() -> release.releaseUnpublishedEmpty(snapshotFirst, ops), true).join();
    check(snapshotFirst.disposed==1, "snapshot-first order disposed the late object twice");
    Stream notEmpty = stream(new Track(9));
    try { release.releaseUnpublishedEmpty(notEmpty, ops); throw new AssertionError("nonempty ownership bypass allowed"); } catch (IllegalStateException expected) {}
    check(notEmpty.disposed==0 && notEmpty.owned.get(0).disposed==0, "independent path touched a shared track");
  }
  static void lateEmptyPathDoesNotRetryPreviouslyFailedDispose() {
    NativeLocalMediaRelease<Track,Stream> release = new NativeLocalMediaRelease<>(); Stream stream = new Stream(); AtomicInteger nativeCalls = new AtomicInteger();
    Ops ops = new Ops() { public void disposeStream(Stream value) { nativeCalls.incrementAndGet(); throw new IllegalStateException("unknown native stream release"); } };
    try { release.releaseUnpublishedEmpty(stream, ops); throw new AssertionError("failure lost"); } catch (IllegalStateException expected) {}
    try { release.releaseUnpublishedEmpty(stream, ops); throw new AssertionError("failed object retried via late path"); } catch (IllegalStateException expected) {}
    check(nativeCalls.get()==1, "special late path repeated unknown native JNI");
  }
  public static void main(String[] args) {
    sharedStreams(); explicitTrackThenStreams(); borrowedIsNotOwner(); failureRetainsAndNeverRetries(); sharedCapturerPromotion(); boundedPhysicalSequence(); concurrentLastOwnerDecision(); failedSequenceDoesNotStartNextJNI(); shutdownWaitsForAcceptedWorkAndUsesOneReservedSlot(); latePermissionEmptyStreamCanFinishAfterShutdown(); lateEmptyPathDoesNotRetryPreviouslyFailedDispose();
    System.out.println("PASS 11 production local ownership/physical-sequence tests (fake JNI/source boundaries)");
  }
}
