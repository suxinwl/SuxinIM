import {
  DEFAULT_SCREENSHOT_SHORTCUT, normalizeScreenshotShortcut,
  ScreenshotShortcutPreference, validScreenshotShortcutPreference,
} from "../../src/utils/screenshot-shortcut";

export type ScreenshotShortcutState = ScreenshotShortcutPreference & {
  registered: boolean; persistent: boolean; recording: boolean;
  defaultAccelerator: string; error?: "conflict" | "storage";
};
type Result = { ok: boolean; code?: "invalid" | "conflict" | "storage" | "busy"; state: ScreenshotShortcutState };
type Dependencies = {
  register: (accelerator: string, callback: () => void) => boolean;
  unregister: (accelerator: string) => void;
  load: () => unknown; save: (config: ScreenshotShortcutPreference) => Promise<void>;
  persistent: () => boolean; capture: () => Promise<unknown>;
};

/** Own only screenshot accelerators. A failed replacement keeps the old one. */
export function createScreenshotShortcutManager(deps: Dependencies) {
  const saved = deps.load();
  let current: ScreenshotShortcutPreference = validScreenshotShortcutPreference(saved)
    ? { ...saved } : { enabled: true, accelerator: DEFAULT_SCREENSHOT_SHORTCUT };
  let suspended = false, stopped = false;
  let error: ScreenshotShortcutState["error"];
  const owned = new Set<string>();
  let queue: Promise<unknown> = Promise.resolve();
  const serialize = <T>(action: () => Promise<T> | T): Promise<T> => {
    const next = queue.then(action); queue = next.catch(() => undefined); return next;
  };
  const state = (): ScreenshotShortcutState => ({ ...current,
    registered: !stopped && !suspended && owned.has(current.accelerator),
    persistent: deps.persistent(), recording: suspended,
    defaultAccelerator: DEFAULT_SCREENSHOT_SHORTCUT, ...(error ? { error } : {}),
  });
  const release = (accelerator: string) => {
    if (!owned.has(accelerator)) return;
    deps.unregister(accelerator); owned.delete(accelerator);
  };
  const reserve = (accelerator: string): boolean => {
    if (owned.has(accelerator)) return true;
    if (stopped) return false;
    try {
      if (!deps.register(accelerator, () => {
        if (stopped || suspended || !current.enabled || current.accelerator !== accelerator) return;
        // The existing screenshot session owns duplicate-trigger protection.
        void deps.capture().catch(() => undefined);
      })) return false;
      owned.add(accelerator); return true;
    } catch { return false; }
  };
  if (current.enabled && !reserve(current.accelerator)) error = "conflict";

  return {
    state,
    set: (input: unknown): Promise<Result> => serialize(async () => {
      if (stopped || suspended) return { ok: false, code: "busy", state: state() };
      if (!input || typeof input !== "object" || Array.isArray(input)
          || Object.keys(input).some(key => !["enabled", "accelerator"].includes(key))) {
        return { ok: false, code: "invalid", state: state() };
      }
      const config = input as ScreenshotShortcutPreference;
      const accelerator = normalizeScreenshotShortcut(config.accelerator);
      if (typeof config.enabled !== "boolean" || !accelerator) return { ok: false, code: "invalid", state: state() };
      if (!deps.persistent()) return { ok: false, code: "storage", state: state() };
      const next = { enabled: config.enabled, accelerator };
      const prior = current;
      const alreadyOwned = owned.has(accelerator);
      if (next.enabled && !reserve(accelerator)) return { ok: false, code: "conflict", state: state() };
      try {
        await deps.save(next);
        if (stopped) return { ok: false, code: "busy", state: state() };
      } catch {
        if (!alreadyOwned) release(accelerator);
        return { ok: false, code: "storage", state: state() };
      }
      current = next; error = undefined;
      if (!next.enabled || prior.accelerator !== next.accelerator) release(prior.accelerator);
      return { ok: true, state: state() };
    }),
    suspend: (value: boolean) => serialize(() => {
      if (stopped) return state();
      suspended = value;
      if (value) for (const accelerator of [...owned]) release(accelerator);
      else if (current.enabled) error = reserve(current.accelerator) ? undefined : "conflict";
      return state();
    }),
    dispose: () => {
      stopped = true;
      for (const accelerator of [...owned]) release(accelerator);
    },
  };
}
