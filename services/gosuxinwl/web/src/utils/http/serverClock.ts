// Anchor request timestamps to the server's HTTP Date, then advance with a
// monotonic clock. A remote device's wall clock must not invalidate a new login.
export function createServerClock(
  wallNow = () => Date.now(),
  monotonicNow = () => performance.now(),
) {
  let sample: { serverTime: number; receivedAt: number } | undefined;
  return {
    sync(dateHeader: unknown) {
      if (typeof dateHeader !== 'string') return;
      const serverTime = Date.parse(dateHeader);
      if (!Number.isFinite(serverTime)) return;
      // Do not add estimated latency: that could put a request in the future.
      sample = { serverTime, receivedAt: monotonicNow() };
    },
    timestamp() {
      const now = sample
        ? sample.serverTime + Math.max(0, monotonicNow() - sample.receivedAt)
        : wallNow();
      return Math.floor(now / 1000);
    },
  };
}

export const serverClock = createServerClock();
