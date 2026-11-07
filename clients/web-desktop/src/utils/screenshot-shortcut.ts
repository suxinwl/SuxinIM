export const DEFAULT_SCREENSHOT_SHORTCUT = "Control+Alt+A";

export type ScreenshotShortcutPreference = { enabled: boolean; accelerator: string };

const reserved = new Set([
  "Control+A", "Control+C", "Control+V", "Control+X", "Control+Z", "Control+Y",
  "Control+S", "Control+W", "Control+R", "Control+N", "Control+O", "Control+P",
  "Control+Q", "Control+F", "Control+F4", "Control+F12", "Alt+F4",
]);

/** Accept a small, predictable subset of Electron accelerators. */
export function normalizeScreenshotShortcut(value: unknown): string | undefined {
  if (typeof value !== "string" || value.length > 80) return;
  const parts = value.split("+").map(part => part.trim().toUpperCase());
  if (parts.length < 2 || parts.some(part => !part)) return;
  const modifiers = new Set<string>();
  let key: string | undefined;
  for (const part of parts) {
    const modifier = ["CTRL", "CONTROL", "CMDORCTRL", "COMMANDORCONTROL"].includes(part)
      ? "Control" : part === "ALT" ? "Alt" : part === "SHIFT" ? "Shift" : undefined;
    if (modifier) {
      if (modifiers.has(modifier)) return;
      modifiers.add(modifier);
    } else {
      if (key || !/^(?:[A-Z0-9]|F(?:[1-9]|1[0-9]|2[0-4]))$/.test(part)) return;
      key = part;
    }
  }
  if (!key || !modifiers.has("Control") && !modifiers.has("Alt")) return;
  const normalized = [...["Control", "Alt", "Shift"].filter(part => modifiers.has(part)), key].join("+");
  return reserved.has(normalized) ? undefined : normalized;
}

export function validScreenshotShortcutPreference(value: unknown): value is ScreenshotShortcutPreference {
  if (!value || typeof value !== "object" || Array.isArray(value)) return false;
  const config = value as ScreenshotShortcutPreference;
  return Object.keys(config).length === 2 && typeof config.enabled === "boolean"
    && normalizeScreenshotShortcut(config.accelerator) === config.accelerator;
}

export function shortcutFromKeyboard(event: {
  code?: string; key: string; ctrlKey: boolean; altKey: boolean; shiftKey: boolean;
  metaKey: boolean; isComposing?: boolean; repeat?: boolean;
}): string | undefined {
  if (event.metaKey || event.isComposing || event.repeat) return;
  const code = event.code ?? "";
  const key = /^(?:Key[A-Z]|Digit[0-9])$/.test(code) ? code.replace(/^(?:Key|Digit)/, "")
    : /^F(?:[1-9]|1[0-9]|2[0-4])$/.test(code) ? code : event.key.toUpperCase();
  return normalizeScreenshotShortcut([
    ...(event.ctrlKey ? ["Control"] : []), ...(event.altKey ? ["Alt"] : []),
    ...(event.shiftKey ? ["Shift"] : []), key,
  ].join("+"));
}

export function formatScreenshotShortcut(value: string): string {
  return value.split("+").map(part => part === "Control" ? "Ctrl" : part).join(" + ");
}
