interface ScreenshotSessionDependencies {
  beforeCapture: (hidden: boolean) => void;
  startCapture: () => Promise<void>;
  endCapture: () => Promise<void>;
  restoreWindow: () => void;
  writeClipboard: (bytes: Uint8Array) => void | Promise<void>;
  respond: (bytes?: Uint8Array) => void;
  reportError: (error: unknown) => void;
}

interface CaptureAttempt {
  starting: Promise<void>;
  finishing?: Promise<void>;
}

/** One capture owns the overlay until it has closed and returned one result. */
export function createScreenshotSession(deps: ScreenshotSessionDependencies) {
  let active: CaptureAttempt | undefined;
  const reportError = (error: unknown) => {
    try { deps.reportError(error); } catch { /* Preserve cleanup. */ }
  };

  const finish = (bytes?: Uint8Array): Promise<void> => {
    const attempt = active;
    if (!attempt) return Promise.resolve();
    if (attempt.finishing) return attempt.finishing;
    // Claim synchronously before any await: double-click, Escape and duplicate
    // terminal IPC events must share the first outcome while reset is pending.
    attempt.finishing = Promise.resolve().then(async () => {
      await attempt.starting.catch(() => undefined);
      try { await deps.endCapture(); } catch (error) { reportError(error); }
      if (bytes) {
        try { await deps.writeClipboard(bytes); } catch (error) { reportError(error); }
      }
      try { deps.restoreWindow(); } catch (error) { reportError(error); }
      try { deps.respond(bytes); } catch (error) { reportError(error); }
      if (active === attempt) active = undefined;
    });
    return attempt.finishing;
  };

  return {
    start(hidden = false): Promise<boolean> {
      if (active) return Promise.resolve(false);
      const attempt: CaptureAttempt = { starting: Promise.resolve() };
      active = attempt;
      attempt.starting = Promise.resolve().then(async () => {
        deps.beforeCapture(hidden);
        await deps.startCapture();
      });
      return attempt.starting.then(
        () => true,
        async (error) => {
          reportError(error);
          await finish();
          return false;
        },
      );
    },
    confirm(bytes: Uint8Array) {
      return finish(Uint8Array.from(bytes));
    },
    cancel: () => finish(),
  };
}
