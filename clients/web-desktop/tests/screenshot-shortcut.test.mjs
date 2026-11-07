import test from "node:test";
import assert from "node:assert/strict";
import { build } from "esbuild";
import { EventEmitter } from "node:events";
import { randomUUID } from "node:crypto";

const compiled = await build({
  stdin: { contents: "export * from './electron/main/screenshotShortcut'; export * from './src/utils/screenshot-shortcut';", resolveDir: process.cwd(), loader: "ts" },
  write: false, bundle: true, platform: "node", format: "esm",
});
const api = await import(`data:text/javascript;base64,${Buffer.from(compiled.outputFiles[0].text).toString("base64")}`);
const A = api.DEFAULT_SCREENSHOT_SHORTCUT, B = "Control+Alt+B", C = "Control+Shift+F8";
const config = (accelerator = A, enabled = true) => ({ enabled, accelerator });
const deferred = () => { let resolve, reject; const promise = new Promise((a, b) => { resolve = a; reject = b; }); return { promise, resolve, reject }; };
const tick = () => new Promise(resolve => setImmediate(resolve));

function fixture(options = {}) {
  const callbacks = new Map(), registered = new Map(), denied = new Set(), throws = new Set(), calls = [], saved = [];
  let persistent = true, captures = 0, save = async value => { saved.push({ ...value }); };
  const deps = {
    register: (key, callback) => {
      calls.push(["register", key]);
      if (throws.has(key)) throw Error("synthetic registrar failure");
      if (denied.has(key) || registered.has(key)) return false;
      callbacks.set(key, callback); registered.set(key, callback); return true;
    },
    unregister: key => { calls.push(["unregister", key]); registered.delete(key); },
    load: () => options.saved,
    save: value => save(value),
    persistent: () => persistent,
    capture: async () => { captures++; },
  };
  if (options.conflict) denied.add(options.conflict);
  const manager = api.createScreenshotShortcutManager(deps);
  return { manager, registered, callbacks, denied, throws, calls, saved, captures: () => captures,
    persistent: value => { persistent = value; }, save: value => { save = value; } };
}

test("shortcut parser canonicalizes supported aliases and keyboard codes", () => {
  for (const input of ["alt+ctrl+a", " Control + Alt + a ", "CommandOrControl+Alt+A", "CmdOrCtrl+Alt+A"])
    assert.equal(api.normalizeScreenshotShortcut(input), A);
  assert.equal(api.normalizeScreenshotShortcut("shift+alt+Control+f24"), "Control+Alt+Shift+F24");
  assert.equal(api.normalizeScreenshotShortcut("Alt+9"), "Alt+9");
  const event = { code: "KeyA", key: "å", ctrlKey: true, altKey: true, shiftKey: false, metaKey: false };
  assert.equal(api.shortcutFromKeyboard(event), A);
  assert.equal(api.shortcutFromKeyboard({ ...event, code: "Digit2", key: "@", shiftKey: true }), "Control+Alt+Shift+2");
  for (const change of [{ metaKey: true }, { isComposing: true }, { repeat: true }])
    assert.equal(api.shortcutFromKeyboard({ ...event, ...change }), undefined);
  assert.equal(api.formatScreenshotShortcut(A), "Ctrl + Alt + A");
});

test("invalid, plain, duplicate-modifier and reserved accelerators are rejected", () => {
  for (const input of [null, 42, {}, "", "A", "Shift+A", "Ctrl+Ctrl+A", "Ctrl+Control+A", "Ctrl++A", "Ctrl+A+B", "Ctrl+F25", "Meta+A", "Win+A", "Alt+Tab", "Control+Alt+Delete", "x".repeat(81),
    "Ctrl+A", "Ctrl+C", "Ctrl+V", "Ctrl+W", "Ctrl+F4", "Ctrl+F12", "Alt+F4"])
    assert.equal(api.normalizeScreenshotShortcut(input), undefined, String(input));
  assert.equal(api.validScreenshotShortcutPreference(config()), true);
  for (const input of [null, [], {}, { enabled: true }, { enabled: "yes", accelerator: A }, { ...config(), extra: true }, config("Ctrl+Alt+A")])
    assert.equal(api.validScreenshotShortcutPreference(input), false);
});

test("startup registers persisted key once; invalid preference uses default and disabled preference reserves nothing", async () => {
  const f = fixture({ saved: config(B) });
  assert.deepEqual([...f.registered.keys()], [B]);
  assert.equal(f.manager.state().accelerator, B);
  f.callbacks.get(B)(); await tick(); assert.equal(f.captures(), 1);
  const invalid = fixture({ saved: { ...config(B), extra: true } });
  assert.deepEqual([...invalid.registered.keys()], [A]);
  const disabled = fixture({ saved: config(B, false) });
  assert.equal(disabled.manager.state().enabled, false);
  assert.equal(disabled.registered.size, 0);
  for (const item of [f, invalid, disabled]) item.manager.dispose();
});

test("startup conflict remains visible and can be retried without replacing the configured key", async () => {
  const f = fixture({ saved: config(B), conflict: B });
  assert.equal(f.manager.state().registered, false);
  assert.equal(f.manager.state().error, "conflict");
  assert.equal(f.manager.state().accelerator, B);
  f.denied.delete(B);
  assert.equal((await f.manager.set(config(B))).ok, true);
  assert.equal(f.manager.state().registered, true);
  assert.equal(f.manager.state().error, undefined);
  f.manager.dispose();
});

test("conflicting or throwing replacement keeps old registration and does not save", async () => {
  const f = fixture();
  for (const collection of [f.denied, f.throws]) {
    collection.add(B);
    const result = await f.manager.set(config(B));
    assert.equal(result.ok, false); assert.equal(result.code, "conflict");
    assert.deepEqual([...f.registered.keys()], [A]);
    assert.equal(f.manager.state().accelerator, A);
    assert.equal(f.saved.length, 0);
    collection.delete(B);
  }
  f.callbacks.get(A)(); await tick(); assert.equal(f.captures(), 1);
  f.manager.dispose();
});

test("replacement activates only after durable save and rolls back its reservation on save failure", async () => {
  const f = fixture(), gate = deferred();
  f.save(() => gate.promise);
  const result = f.manager.set(config(B)); await tick();
  assert.deepEqual([...f.registered.keys()], [A, B]);
  f.callbacks.get(A)(); f.callbacks.get(B)(); await tick();
  assert.equal(f.captures(), 1, "uncommitted key must not capture");
  gate.reject(Error("synthetic encryption failure"));
  assert.equal((await result).code, "storage");
  assert.deepEqual([...f.registered.keys()], [A]);
  assert.equal(f.manager.state().accelerator, A);
  f.callbacks.get(B)(); await tick(); assert.equal(f.captures(), 1);
  f.manager.dispose();
});

test("successful replacement releases old key, and same-key save does not re-register", async () => {
  const f = fixture();
  assert.equal((await f.manager.set(config("alt+ctrl+b"))).ok, true);
  assert.deepEqual([...f.registered.keys()], [B]);
  f.callbacks.get(A)(); f.callbacks.get(B)(); await tick(); assert.equal(f.captures(), 1);
  const calls = f.calls.length;
  assert.equal((await f.manager.set(config(B))).ok, true);
  assert.equal(f.calls.length, calls);
  assert.deepEqual(f.saved, [config(B), config(B)]);
  f.manager.dispose();
});

test("disabling is transactional: failed save keeps old key, successful save releases it", async () => {
  const f = fixture();
  f.save(async () => { throw Error("synthetic disk full"); });
  assert.equal((await f.manager.set(config(A, false))).code, "storage");
  assert.equal(f.manager.state().enabled, true);
  assert.deepEqual([...f.registered.keys()], [A]);
  f.save(async () => {});
  assert.equal((await f.manager.set(config(A, false))).ok, true);
  assert.equal(f.manager.state().enabled, false); assert.equal(f.registered.size, 0);
  f.callbacks.get(A)(); await tick(); assert.equal(f.captures(), 0);
  f.manager.dispose();
});

test("RAM-only preferences refuse a persistent-success claim and invalid requests have no side effects", async () => {
  const f = fixture(); f.persistent(false);
  assert.equal((await f.manager.set(config(B))).code, "storage");
  f.persistent(true);
  for (const value of [null, [], {}, config("Control+F12"), { ...config(B), unexpected: true }, { enabled: 1, accelerator: B }])
    assert.equal((await f.manager.set(value)).code, "invalid");
  assert.deepEqual(f.calls, [["register", A]]); assert.equal(f.saved.length, 0);
  f.manager.dispose();
});

test("recording pauses capture, rejects edits, and resume conflict is visible and retriable", async () => {
  const f = fixture();
  const state = await f.manager.suspend(true);
  assert.equal(state.recording, true); assert.equal(state.registered, false); assert.equal(f.registered.size, 0);
  f.callbacks.get(A)(); await tick(); assert.equal(f.captures(), 0);
  assert.equal((await f.manager.set(config(B))).code, "busy");
  f.denied.add(A);
  assert.equal((await f.manager.suspend(false)).error, "conflict");
  assert.equal(f.manager.state().accelerator, A); assert.equal(f.manager.state().registered, false);
  f.denied.delete(A); await f.manager.suspend(false);
  assert.equal(f.manager.state().registered, true);
  f.callbacks.get(A)(); await tick(); assert.equal(f.captures(), 1);
  f.manager.dispose();
});

test("concurrent replacements serialize their saves and retain a valid old key until each commit", async () => {
  const f = fixture(), first = deferred(), second = deferred(), saves = [];
  f.save(value => { saves.push({ ...value }); return saves.length === 1 ? first.promise : second.promise; });
  const one = f.manager.set(config(B)), two = f.manager.set(config(C));
  await tick(); assert.deepEqual(saves, [config(B)]); assert.deepEqual([...f.registered.keys()], [A, B]);
  first.resolve(); assert.equal((await one).ok, true); await tick();
  assert.deepEqual(saves, [config(B), config(C)]); assert.deepEqual([...f.registered.keys()], [B, C]);
  second.resolve(); assert.equal((await two).ok, true);
  assert.deepEqual([...f.registered.keys()], [C]); assert.equal(f.manager.state().accelerator, C);
  f.manager.dispose();
});

test("quit during pending save releases every key and queued work cannot register again", async () => {
  const f = fixture(), gate = deferred(); f.save(() => gate.promise);
  const one = f.manager.set(config(B)), two = f.manager.set(config(C));
  await tick(); f.manager.dispose(); const calls = f.calls.length;
  assert.equal(f.registered.size, 0);
  gate.resolve(); assert.equal((await one).code, "busy"); assert.equal((await two).code, "busy");
  await f.manager.suspend(false); assert.equal(f.calls.length, calls);
  f.callbacks.get(A)(); f.callbacks.get(B)(); await tick(); assert.equal(f.captures(), 0);
  assert.equal(f.manager.state().registered, false);
});

async function bridgeFixture(t) {
  const key = `__screenshotBridge_${randomUUID().replaceAll("-", "")}`;
  const app = new EventEmitter(), window = new EventEmitter(), sender = new EventEmitter(), handlers = new Map(), registered = new Map(), sent = [];
  let captures = 0, devtools = 0, persistent = true;
  Object.assign(sender, { id: 17, isDestroyed: () => false, send: (...args) => sent.push(args) });
  globalThis[key] = {
    app, BrowserWindow: { fromWebContents: () => window },
    globalShortcut: { register: (name, callback) => { if (registered.has(name)) return false; registered.set(name, callback); return true; }, unregister: name => registered.delete(name) },
    ipcMain: { handle: (name, callback) => { assert.equal(handlers.has(name), false); handlers.set(name, callback); } },
    toggleDevTools: () => { devtools++; }, getStore: () => ({ get: () => undefined }),
    hasPersistentPreferences: () => persistent, saveScreenshotShortcutPreference: async () => {},
  };
  const compiled = await build({ entryPoints: ["electron/main/shortcutManage.ts"], bundle: true, write: false, platform: "node", format: "esm", plugins: [{ name: "actual-shortcut-bridge-boundaries", setup(builder) {
    builder.onResolve({ filter: /^electron$|^\.\/windowManage$|^\.\/storeManage$/ }, args => ({ path: args.path, namespace: "shortcut-boundary" }));
    builder.onLoad({ filter: /.*/, namespace: "shortcut-boundary" }, () => ({ contents: `export const {app,BrowserWindow,globalShortcut,ipcMain,toggleDevTools,getStore,hasPersistentPreferences,saveScreenshotShortcutPreference}=globalThis[${JSON.stringify(key)}];`, loader: "js" }));
  } }] });
  const bridge = await import(`data:text/javascript;base64,${Buffer.from(compiled.outputFiles[0].text).toString("base64")}#${randomUUID()}`);
  const manager = bridge.installScreenshotShortcut(async () => { captures++; });
  const invoke = (operation, value, url = "openim38://app/index.html") => handlers.get("puty-screenshot-shortcut")({ sender, senderFrame: { url } }, operation, value);
  t.after(() => { app.emit("will-quit"); delete globalThis[key]; });
  return { app, window, sender, registered, sent, manager, bridge, invoke, captures: () => captures, devtools: () => devtools };
}

test("actual Main bridge keeps screenshot registration when DevTools focus shortcut is removed", async t => {
  const f = await bridgeFixture(t);
  assert.equal(f.bridge.installScreenshotShortcut(async () => assert.fail("duplicate capture callback")), f.manager);
  f.bridge.registerShortcuts(); assert.equal(f.registered.has("CmdOrCtrl+F12"), true);
  f.bridge.unregisterShortcuts();
  assert.equal(f.registered.has("CmdOrCtrl+F12"), false); assert.equal(f.registered.has(A), true);
  f.registered.get(A)(); await tick(); assert.equal(f.captures(), 1);
  await assert.rejects(f.invoke("set", config(B), "https://untrusted.example/"), /Untrusted/);
  assert.equal(f.manager.state().accelerator, A);
  f.app.emit("will-quit"); assert.equal(f.registered.size, 0);
});

test("actual Main bridge recorder uses tokens; blur restores shortcut and stale token cannot end newer recorder", async t => {
  const f = await bridgeFixture(t);
  const one = await f.invoke("begin-record"); assert.equal(one.ok, true); assert.equal(f.registered.size, 0);
  await f.invoke("end-record", { token: "foreign" }); assert.equal(f.manager.state().recording, true);
  f.window.emit("blur"); await tick(); assert.equal(f.registered.has(A), true);
  assert.deepEqual(f.sent.at(-1), ["puty-screenshot-shortcut-recording-ended", { token: one.token }]);
  const two = await f.invoke("begin-record"); assert.notEqual(one.token, two.token);
  await f.invoke("end-record", { token: one.token }); assert.equal(f.manager.state().recording, true);
  f.sender.emit("render-process-gone"); await tick(); assert.equal(f.registered.has(A), true);
  assert.equal(f.manager.state().recording, false);
  const three = await f.invoke("begin-record"); assert.equal(three.ok, true);
  f.app.emit("will-quit"); await tick(); assert.equal(f.registered.size, 0);
  assert.equal((await f.invoke("end-record", { token: three.token })).state.registered, false);
});
