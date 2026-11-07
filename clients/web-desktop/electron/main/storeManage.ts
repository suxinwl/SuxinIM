import { app, safeStorage } from "electron";
import { promises as fs } from "node:fs";
import path from "node:path";
import { randomUUID } from "node:crypto";
import { ScreenshotShortcutPreference, validScreenshotShortcutPreference } from "../../src/utils/screenshot-shortcut";

type Preferences = { language?: "zh-CN" | "en-US"; closeAction?: "miniSize" | "quit"; partitionRandom?: number | string; screenshotShortcut?: ScreenshotShortcutPreference };
const keys = new Set(["language", "closeAction", "partitionRandom", "screenshotShortcut"]);
const format = "PUTY-UI-PREFERENCES-1";
export type PreferenceFailurePhase = "protection" | "root-path" | "root-marker" | "preferences-path" | "read" | "decrypt" | "format" | "encrypt" | "write";
export type PreferenceStartupResult = { mode: "memory" | "persistent"; warning?: PreferenceFailurePhase };
class PreferenceFailure extends Error {
  constructor(readonly phase: PreferenceFailurePhase) { super("系统密钥保护或加密偏好存储不可用，未使用明文降级"); }
}
const failure = (phase: PreferenceFailurePhase = "read") => new PreferenceFailure(phase);
let values: Preferences = {}, root: string | undefined, initialized = false, bindingGeneration = 0;
let queue: Promise<void> = Promise.resolve(), persistenceError: Error | undefined;

function valid(key: string, value: unknown): boolean {
  if (key === "screenshotShortcut") return validScreenshotShortcutPreference(value);
  if (key === "language") return value === "zh-CN" || value === "en-US";
  if (key === "closeAction") return value === "miniSize" || value === "quit";
  return key === "partitionRandom" && (Number.isInteger(value) && Number(value) >= 0 && Number(value) <= 5 || typeof value === "string" && /^puty-memory-part[1-5]$/.test(value));
}
function requireKey(key: unknown): asserts key is keyof Preferences {
  if (typeof key !== "string" || !keys.has(key)) throw new Error("仅允许保存界面偏好设置");
}
function enqueue(operation: () => Promise<void>): Promise<void> {
  const pending = queue.then(operation);
  queue = pending.catch(() => {});
  return pending;
}
function current(run: number) { if (run !== bindingGeneration) throw new Error("偏好存储目录已改变"); }
async function stage<T>(phase: PreferenceFailurePhase, operation: () => Promise<T>): Promise<T> {
  try { return await operation(); }
  catch (error) { if (error instanceof Error && error.message === "偏好存储目录已改变") throw error; throw failure(phase); }
}
async function protectedBackend() {
  if (!await safeStorage.isAsyncEncryptionAvailable() || process.platform === "linux" && safeStorage.getSelectedStorageBackend?.() === "basic_text") throw failure("protection");
}
async function checkedDirectory(value: string): Promise<void> {
  if (!path.isAbsolute(value) || path.resolve(value) !== value) throw failure();
  let cursor = path.parse(value).root;
  for (const part of path.relative(cursor, value).split(path.sep).filter(Boolean)) {
    cursor = path.join(cursor, part); const info = await fs.lstat(cursor);
    if (!info.isDirectory() || info.isSymbolicLink()) throw failure();
  }
  if (await fs.realpath(value) !== value) throw failure();
}
async function regularFile(value: string): Promise<Buffer | undefined> {
  try {
    const info = await fs.lstat(value);
    if (!info.isFile() || info.isSymbolicLink() || info.nlink !== 1 || info.size > 131072) throw failure();
    return await fs.readFile(value);
  } catch (error) { if ((error as NodeJS.ErrnoException).code === "ENOENT") return; throw error; }
}
async function location(value: string, createShared = true): Promise<string> {
  await stage("root-path", () => checkedDirectory(value));
  await stage("root-marker", async () => {
    const marker = await regularFile(path.join(value, ".puty-local-root.json"));
    if (!marker) throw failure();
    const owner = JSON.parse(marker.toString("utf8"));
    if (owner.format !== "PUTY-LOCAL-ROOT-1" || !/^[a-f0-9-]{36}$/.test(owner.id)) throw failure();
  });
  const shared = path.join(value, "shared");
  await stage("preferences-path", async () => {
    if (createShared) await fs.mkdir(shared).catch(error => { if (error.code !== "EEXIST") throw error; });
    try { await checkedDirectory(shared); }
    catch (error) { if (createShared || (error as NodeJS.ErrnoException).code !== "ENOENT") throw error; }
  });
  return path.join(shared, "ui-preferences.puty");
}
function decode(raw: string): Preferences {
  const document = JSON.parse(raw);
  if (document.format !== format || !document.preferences || typeof document.preferences !== "object" || Array.isArray(document.preferences)
      || Object.keys(document).some(key => !["format", "preferences"].includes(key))) throw failure();
  for (const [key, value] of Object.entries(document.preferences)) if (!keys.has(key) || !valid(key, value)) throw failure();
  return document.preferences;
}
async function persist(run: number, snapshot?: Preferences): Promise<void> {
  if (!root || !initialized) return;
  current(run); await stage("protection", protectedBackend); current(run);
  const destination = await location(root); current(run);
  await stage("read", () => regularFile(destination)); current(run);
  const plain = JSON.stringify({ format, preferences: { ...(snapshot ?? values) } });
  const encrypted = await stage("encrypt", () => safeStorage.encryptStringAsync(plain)); current(run);
  const readback = await stage("decrypt", () => safeStorage.decryptStringAsync(encrypted)); current(run);
  if (readback.result !== plain) throw failure("decrypt");
  const temporary = destination + ".next-" + randomUUID();
  await stage("write", async () => {
    const handle = await fs.open(temporary, "wx", 0o600);
    try {
      try { await handle.writeFile(encrypted); await handle.sync(); } finally { await handle.close(); }
      current(run); await fs.rename(temporary, destination);
      if (process.platform !== "win32") { const directory = await fs.open(path.dirname(destination), "r"); try { await directory.sync(); } finally { await directory.close(); } }
      current(run); persistenceError = undefined;
    } finally { await fs.unlink(temporary).catch(error => { if (error.code !== "ENOENT") throw error; }); }
  });
}

/** Synchronous UI-only facade. Every disk write uses the queued OS protector. */
export class Store {
  get(key: string, defaultValue?: unknown): unknown { requireKey(key); return values[key] ?? defaultValue; }
  set(key: string, value: unknown): this {
    requireKey(key); if (!valid(key, value)) throw new Error("界面偏好设置无效");
    if (key === "screenshotShortcut") throw new Error("请通过截图快捷键设置保存此项");
    values = { ...values, [key]: value };
    const run = bindingGeneration;
    if (initialized && root) void enqueue(async () => { if (run !== bindingGeneration) return; await persist(run); }).catch(() => { if (run === bindingGeneration) persistenceError = failure(); });
    return this;
  }
}
const store = new Store();
export const getStore = () => store;

export function hasPersistentPreferences(): boolean { return initialized && Boolean(root); }

/** Commit a shortcut only after the encrypted write succeeds; no RAM-only success. */
export function saveScreenshotShortcutPreference(value: ScreenshotShortcutPreference): Promise<void> {
  if (!validScreenshotShortcutPreference(value)) return Promise.reject(new Error("截图快捷键设置无效"));
  const config = { ...value }, run = bindingGeneration;
  return enqueue(async () => {
    current(run);
    if (!root || !initialized) throw failure("root-path");
    await persist(run, { ...values, screenshotShortcut: config });
    current(run);
    // Ordinary UI writes can arrive while encryption is pending. Keep them;
    // their already-queued persistence will include this committed shortcut.
    values = { ...values, screenshotShortcut: config };
  });
}

/** Call after app.ready, before i18n or renderer startup. No selected root means RAM only. */
export async function initProtectedPreferences(selectedRoot?: string): Promise<PreferenceStartupResult> {
  if (!selectedRoot) { await app.whenReady(); initialized = true; return { mode: "memory" }; }
  const run = bindingGeneration + 1;
  try { await bindRoot(selectedRoot, true); return { mode: "persistent" }; }
  catch (error) {
    // UI preferences alone may use defaults in RAM. Never reset an OS key,
    // alter the old encrypted blob, or weaken the separate vault unlock gate.
    current(run);
    if (!(error instanceof PreferenceFailure)) throw error;
    root = undefined; initialized = true; values = {}; persistenceError = undefined;
    return { mode: "memory", warning: error.phase };
  }
}

/** Root moves must flush before copying and bind after copying the encrypted shared tree. */
export function bindPreferencesRoot(selectedRoot: string): Promise<void> {
  return bindRoot(selectedRoot, false);
}
function bindRoot(selectedRoot: string, initial: boolean): Promise<void> {
  const run = ++bindingGeneration;
  return enqueue(async () => {
    await app.whenReady(); current(run); await stage("protection", protectedBackend); current(run);
    const destination = await location(selectedRoot, !initial); current(run);
    const encrypted = await stage("read", () => regularFile(destination)); current(run);
    let saved: Preferences = {};
    if (encrypted) { const decrypted = await stage("decrypt", () => safeStorage.decryptStringAsync(encrypted)); current(run); saved = await stage("format", async () => decode(decrypted.result)); }
    const changed = Object.entries(values).some(([key, value]) => saved[key as keyof Preferences] !== value);
    values = { ...saved, ...values }; root = selectedRoot; initialized = true; persistenceError = undefined;
    // Merely starting the app must not rewrite a readable existing blob.
    if (!initial || changed) await persist(run);
  }).catch(error => {
    if (run === bindingGeneration) { root = undefined; persistenceError = initial ? undefined : failure(); }
    if (error instanceof Error && error.message === "偏好存储目录已改变") throw error;
    throw error instanceof PreferenceFailure ? error : failure();
  });
}

export async function flushPreferences(): Promise<void> { await queue; if (persistenceError) throw persistenceError; }
