package com.cloudwebrtc.webrtc;

import java.util.concurrent.ArrayBlockingQueue;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.Semaphore;
import java.util.concurrent.atomic.AtomicBoolean;
import java.util.concurrent.atomic.AtomicInteger;
import java.util.function.Consumer;

/**
 * Bounded frame ownership mailbox independent of Android/WebRTC APIs.
 * A source callback only retains and publishes; worker delivery owns final release.
 */
public final class NativeFrameMailbox<F extends NativeFrameMailbox.Frame> {
  public interface Frame { void retain(); void release(); }
  public interface Delivery<F> { void deliver(F frame) throws Exception; }
  public enum Admission { ADMITTED, BUSY, CAPACITY, PAUSED, SEALED, FAILED }

  public interface Pool {
    /** Null means full. A permanently failed pool must throw, not silently drop forever. */
    Ticket tryReserve();
  }
  public interface Ticket {
    /** Only legal before retain; performs no frame operation. */
    void cancelBeforeRetain();
    /** Retain/publish failed with uncertain ownership; keep total-retained permit. */
    void keepUnknownRetained();
    /** Reserved publication: never CallerRuns and never blocking put. */
    void publish(Runnable physicalWork, Consumer<Throwable> unexpectedWorkerFailure);
    /** Called only after the one actual frame.release returned normally. */
    void frameReleased();
  }

  public static final class Pause {
    private final Object owner;
    public final long generation;
    public final CompletableFuture<Void> physicalDrain;
    private Pause(Object owner, long generation, CompletableFuture<Void> physicalDrain) {
      this.owner = owner; this.generation = generation; this.physicalDrain = physicalDrain;
    }
  }

  private final Object metadata = new Object();
  private final Pool pool;
  private final Delivery<F> delivery;
  // Must only set flags/post to the existing observed error path; never wait/do JNI here.
  private final Consumer<Throwable> faultObserver;
  private long generation;
  private boolean accepting;
  private boolean sealed;
  private Flight current;
  private Flight unknownRetained;
  private Throwable failure;
  private Throwable faultObserverFailure;
  private CompletableFuture<Void> lastPhysical = CompletableFuture.completedFuture(null);
  private CompletableFuture<Void> sealedDrain;

  private final class Flight {
    final F frame;
    final Ticket ticket;
    final long generation;
    final CompletableFuture<Void> physical = new CompletableFuture<>();
    Flight(F frame, Ticket ticket, long generation) {
      this.frame = frame; this.ticket = ticket; this.generation = generation;
    }
  }

  public NativeFrameMailbox(Pool pool, Delivery<F> delivery, Consumer<Throwable> faultObserver) {
    this.pool = pool; this.delivery = delivery; this.faultObserver = faultObserver;
    // Initially paused: only successful binding attach may call resume(pauseToken).
  }

  /** Short metadata transition. Does not wait, render, release or cancel native work. */
  public Pause pause() {
    synchronized (metadata) {
      accepting = false;
      ++generation;
      return new Pause(this, generation, lastPhysical);
    }
  }

  /** Caller composes this after native attach, and after token.physicalDrain. */
  public boolean resume(Pause token) {
    synchronized (metadata) {
      if (token.owner != this || token.generation != generation || sealed || failure != null
          || current != null || !token.physicalDrain.isDone()
          || token.physicalDrain.isCompletedExceptionally()) return false;
      accepting = true;
      return true;
    }
  }

  /** Canonical permanent seal. Deadline wrappers must never settle this physical Future. */
  public CompletableFuture<Void> seal() {
    synchronized (metadata) {
      if (sealedDrain != null) return sealedDrain;
      sealed = true; accepting = false; ++generation;
      sealedDrain = lastPhysical;
      return sealedDrain;
    }
  }

  /** Source callback. Slow delivery and owned release never run here. */
  public Admission tryOffer(F frame) {
    final Ticket ticket;
    try {
      ticket = pool.tryReserve();
    } catch (Throwable error) {
      failWithoutRetain(error);
      return Admission.FAILED;
    }
    if (ticket == null) return Admission.CAPACITY;

    final Flight flight;
    final Admission rejected;
    synchronized (metadata) {
      rejected = failure != null ? Admission.FAILED : sealed ? Admission.SEALED
          : !accepting ? Admission.PAUSED : current != null ? Admission.BUSY : null;
      if (rejected == null) {
        flight = new Flight(frame, ticket, generation);
        current = flight;
        // Publish ownership before retain so a concurrent seal observes this reservation.
        lastPhysical = flight.physical;
      } else flight = null;
    }
    if (rejected != null) {
      ticket.cancelBeforeRetain();
      return rejected;
    }

    try {
      // No metadata/global queue lock held. Actual retain must precede publication.
      frame.retain();
      ticket.publish(() -> consume(flight), error -> failUnknown(flight, error));
      return Admission.ADMITTED;
    } catch (Throwable error) {
      // No caller-thread release: partial retain/publication ownership may be unknown.
      ticket.keepUnknownRetained();
      failUnknown(flight, error);
      return Admission.FAILED;
    }
  }

  private void consume(Flight flight) {
    Throwable first = null;
    boolean shouldRender;
    synchronized (metadata) {
      shouldRender = current == flight && accepting && !sealed && failure == null
          && generation == flight.generation;
    }
    try {
      // A pause AFTER this decision cannot cancel this body: its drain must wait.
      if (shouldRender) delivery.deliver(flight.frame);
    } catch (Throwable error) {
      first = error;
    }
    boolean released = false;
    try {
      flight.frame.release();
      released = true;
      flight.ticket.frameReleased();
    } catch (Throwable error) {
      if (first == null) first = error;
    }
    synchronized (metadata) {
      // A separate pool/admission failure during this flight must not be hidden by a
      // later successful frame.release. Physical failure remains explicit to drain.
      if (first == null && failure != null) first = failure;
      if (first != null && failure == null) failure = first;
      if (released) {
        if (current == flight) current = null;
      } else {
        unknownRetained = flight;
        accepting = false;
      }
    }
    // No native work/metadata lock while running Future continuations/error observers.
    if (first == null) flight.physical.complete(null);
    else {
      flight.physical.completeExceptionally(first);
      observeFault(first);
    }
  }

  private void failUnknown(Flight flight, Throwable error) {
    synchronized (metadata) {
      if (failure == null) failure = error;
      accepting = false;
      unknownRetained = flight;
    }
    flight.physical.completeExceptionally(error);
    observeFault(error);
  }

  private void failWithoutRetain(Throwable error) {
    final CompletableFuture<Void> failed = new CompletableFuture<>();
    synchronized (metadata) {
      if (failure == null) failure = error;
      accepting = false;
      // Existing owned work still needs its actual release; do not overwrite its Future.
      if (current == null) lastPhysical = failed;
    }
    failed.completeExceptionally(error);
    observeFault(error);
  }

  private void observeFault(Throwable error) {
    try { faultObserver.accept(error); }
    catch (Throwable observerError) {
      synchronized (metadata) {
        if (faultObserverFailure == null) faultObserverFailure = observerError;
      }
    }
  }

  public Throwable failure() { synchronized (metadata) { return failure; } }
  public Throwable faultObserverFailure() {
    synchronized (metadata) { return faultObserverFailure; }
  }

  /**
   * Process-lifetime fixed pool. Construction is a startup operation, never an onFrame action.
   * Queue slots are reserved separately from total retained slots. No shutdown/replacement API.
   */
  public static final class FixedPool implements Pool {
    private final Semaphore total = new Semaphore(10);
    private final Semaphore queueReservations = new Semaphore(8);
    private final ArrayBlockingQueue<Work> queue = new ArrayBlockingQueue<>(8);
    private volatile Throwable failed;
    private final AtomicInteger idleInterruptions = new AtomicInteger();
    private volatile InterruptedException lastIdleInterruption;
    private volatile Throwable interruptionObserverFailure;
    // Diagnostic callback must only record/post; it may not block or perform native work.
    private final Consumer<InterruptedException> interruptionObserver;

    private final class Work {
      final Slot slot; final Runnable action; final Consumer<Throwable> unexpected;
      Work(Slot slot, Runnable action, Consumer<Throwable> unexpected) {
        this.slot = slot; this.action = action; this.unexpected = unexpected;
      }
    }

    public FixedPool() {
      this(interrupted -> {});
    }

    public FixedPool(Consumer<InterruptedException> interruptionObserver) {
      if (interruptionObserver == null) throw new NullPointerException("interruptionObserver");
      this.interruptionObserver = interruptionObserver;
      // Prestart exactly two daemon workers. No worker depends on Media2/output-owner tasks.
      for (int i = 0; i < 2; ++i) {
        Thread worker = new Thread(this::runWorker, "NativeFrameMailbox-" + i);
        worker.setDaemon(true);
        worker.start();
      }
    }

    public int idleInterruptionCount() { return idleInterruptions.get(); }
    public InterruptedException lastIdleInterruption() { return lastIdleInterruption; }
    public Throwable interruptionObserverFailure() { return interruptionObserverFailure; }

    @Override public Ticket tryReserve() {
      if (failed != null) throw new IllegalStateException("Frame pool failed", failed);
      if (!total.tryAcquire()) return null;
      if (!queueReservations.tryAcquire()) { total.release(); return null; }
      if (failed != null) {
        queueReservations.release(); total.release();
        throw new IllegalStateException("Frame pool failed", failed);
      }
      return new Slot();
    }

    private final class Slot implements Ticket {
      final AtomicBoolean queueReleased = new AtomicBoolean();
      final AtomicBoolean totalReleased = new AtomicBoolean();
      final AtomicBoolean published = new AtomicBoolean();
      private void freeQueue() {
        if (queueReleased.compareAndSet(false, true)) queueReservations.release();
      }
      @Override public void cancelBeforeRetain() {
        if (published.get()) throw new IllegalStateException("Already published");
        freeQueue(); frameReleased();
      }
      @Override public void keepUnknownRetained() { freeQueue(); }
      @Override public void frameReleased() {
        if (totalReleased.compareAndSet(false, true)) total.release();
      }
      @Override public void publish(Runnable action, Consumer<Throwable> unexpected) {
        if (!published.compareAndSet(false, true)) throw new IllegalStateException("Double publish");
        // All producers hold a queue reservation; there is no ordinary full-queue rejection.
        if (!queue.offer(new Work(this, action, unexpected))) {
          IllegalStateException invariant = new IllegalStateException("Reserved queue slot lost");
          failed = invariant;
          freeQueue();
          throw invariant;
        }
      }
    }

    private void runWorker() {
      for (;;) {
        final Work work;
        try { work = queue.take(); }
        catch (InterruptedException interrupted) {
          // queue.take has not transferred any owned Work. The exception clears this
          // thread's interrupt flag. Keep this same bounded worker responsible for
          // queued releases; no replacement, caller-runs, or retry of an owned action.
          lastIdleInterruption = interrupted;
          idleInterruptions.incrementAndGet();
          try { interruptionObserver.accept(interrupted); }
          catch (Throwable observerError) { interruptionObserverFailure = observerError; }
          continue;
        }
        work.slot.freeQueue();
        try { work.action.run(); }
        catch (Throwable unexpected) {
          failed = unexpected;
          // Conservatively retain total permit; the frame's release state is unknown.
          try { work.unexpected.accept(unexpected); } catch (Throwable ignored) {}
        }
      }
    }
  }
}
