// The browser never handles credentials here. Renewal only checks the current
// authenticated cookie; an expired or revoked session is never recreated.
export function createIMSession({ renew, renewed, failed, denied }) {
  let active = false, generation = 0, timer, pending = null, lastAttempt = 0;
  let retryDelay = 15000;
  const clearTimer = () => { clearTimeout(timer); timer = undefined; };
  function stop() {
    active = false;
    generation++;
    clearTimer();
    pending?.abort();
    pending = null;
    lastAttempt = 0;
  }
  function schedule(delay) {
    clearTimer();
    if (active) timer = setTimeout(() => check(), delay);
  }
  async function check() {
    if (!active || pending) return;
    clearTimer();
    const current = generation;
    const controller = new AbortController();
    pending = controller;
    lastAttempt = Date.now();
    const timeout = setTimeout(() => controller.abort(), 20000);
    let nextDelay = 60000;
    try {
      const access = await renew(controller.signal);
      if (!active || current !== generation) return;
      retryDelay = 15000;
      renewed(access);
    } catch (error) {
      if (!active || current !== generation) return;
      if (error.status === 401 || error.status === 403) {
        stop();
        await denied(error);
        return;
      }
      failed(error);
      nextDelay = retryDelay;
      retryDelay = Math.min(retryDelay * 2, 60000);
    } finally {
      clearTimeout(timeout);
      if (current === generation) {
        pending = null;
        schedule(nextDelay);
      }
    }
  }
  function start() {
    if (active) return;
    active = true;
    generation++;
    retryDelay = 15000;
    void check();
  }
  function resume() {
    // A focus + visibility pair is one restoration, not two renewals. Timer
    // heartbeats continue while hidden, including the empty IM inbox page.
    if (active && Date.now() - lastAttempt >= 1000) void check();
  }
  return { start, stop, resume };
}
