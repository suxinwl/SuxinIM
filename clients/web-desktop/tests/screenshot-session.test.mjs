import test from "node:test";
import assert from "node:assert/strict";
import { build } from "esbuild";
import { EventEmitter } from "node:events";
import { readFileSync } from "node:fs";
import { createRequire } from "node:module";
import { resolve } from "node:path";
import vm from "node:vm";

const compiled = await build({ entryPoints: ["electron/main/screenshotSession.ts"], write: false, bundle: true, format: "esm", platform: "node" });
const { createScreenshotSession } = await import(`data:text/javascript;base64,${Buffer.from(compiled.outputFiles[0].text).toString("base64")}`);
const deferred = () => { let resolve, reject; const promise = new Promise((a, b) => { resolve = a; reject = b; }); return { promise, resolve, reject }; };
const tick = () => new Promise((resolve) => setImmediate(resolve));

function fixture(overrides = {}) {
  const events = [], results = [], errors = [];
  const session = createScreenshotSession({
    beforeCapture: (hidden) => events.push(["prepare", hidden]),
    startCapture: async () => { events.push(["start"]); },
    endCapture: async () => { events.push(["end"]); },
    writeClipboard: (bytes) => events.push(["clipboard", [...bytes]]),
    restoreWindow: () => events.push(["restore"]),
    respond: (bytes) => { events.push(["respond"]); results.push(bytes && [...bytes]); },
    reportError: (error) => errors.push(error.message),
    ...overrides,
  });
  return { session, events, results, errors };
}

test("confirmation waits for the overlay close before clipboard, window restore and one response", async () => {
  const close = deferred();
  const state = fixture({ endCapture: () => close.promise });
  assert.equal(await state.session.start(true), true);
  const bytes = Uint8Array.of(1, 2, 3);
  const finishing = state.session.confirm(bytes);
  bytes[0] = 9;
  assert.equal(await state.session.start(), false);
  const duplicate = state.session.confirm(Uint8Array.of(4));
  const cancel = state.session.cancel();
  assert.equal(duplicate, finishing);
  assert.equal(cancel, finishing);
  await tick();
  assert.deepEqual(state.results, []);
  assert.equal(state.events.some(([name]) => name === "restore"), false);
  close.resolve();
  await finishing;
  assert.deepEqual(state.results, [[1, 2, 3]]);
  assert.deepEqual(state.events.slice(-3).map(([name]) => name), ["clipboard", "restore", "respond"]);
});

test("cancel and Escape return one empty response; the next capture is usable", async () => {
  const state = fixture();
  await state.session.start(true);
  await Promise.all([state.session.cancel(), state.session.cancel()]);
  await state.session.confirm(Uint8Array.of(9));
  assert.deepEqual(state.results, [undefined]);
  assert.equal(await state.session.start(false), true);
  await state.session.confirm(Uint8Array.of(2));
  assert.deepEqual(state.results, [undefined, [2]]);
  assert.equal(state.events.filter(([name]) => name === "start").length, 2);
  assert.equal(state.events.filter(([name]) => name === "end").length, 2);
});

test("a clipboard exception cannot strand the capture surface or discard the result", async () => {
  const state = fixture({ writeClipboard: () => { throw new Error("clipboard busy"); } });
  await state.session.start();
  await state.session.confirm(Uint8Array.of(7));
  assert.deepEqual(state.results, [[7]]);
  assert.deepEqual(state.errors, ["clipboard busy"]);
  assert.equal(state.events.filter(([name]) => name === "end").length, 1);
  assert.equal(await state.session.start(), true);
  await state.session.cancel();
});

test("duplicate starts are rejected while asynchronous capture startup is in flight", async () => {
  const starting = deferred();
  let starts = 0, ends = 0;
  const state = fixture({ startCapture: () => { starts++; return starting.promise; }, endCapture: async () => { ends++; } });
  const first = state.session.start(true);
  assert.equal(await state.session.start(false), false);
  const canceled = state.session.cancel();
  await tick();
  assert.equal(starts, 1);
  assert.equal(ends, 0);
  starting.resolve();
  await first;
  await canceled;
  assert.equal(ends, 1);
  assert.deepEqual(state.results, [undefined]);
});

test("failed start and failed close restore the window and release the session", async () => {
  let first = true;
  const state = fixture({
    startCapture: async () => { if (first) { first = false; throw new Error("capture unavailable"); } },
    endCapture: async () => { throw new Error("view reset interrupted"); },
  });
  assert.equal(await state.session.start(true), false);
  assert.deepEqual(state.results, [undefined]);
  assert.equal(await state.session.start(), true);
  await state.session.confirm(Uint8Array.of(5));
  assert.deepEqual(state.results, [undefined, [5]]);
  assert.deepEqual(state.errors, ["capture unavailable", "view reset interrupted", "view reset interrupted"]);
});

function loadSandboxPreload(source) {
  const sent = [];
  let api;
  const electron = {
    contextBridge: { exposeInMainWorld: (_name, value) => { api = value; } },
    ipcRenderer: { send: (...args) => sent.push(args), on() {}, off() {} },
  };
  // Electron 45's actual sandbox preload was observed to have no Buffer global.
  vm.runInNewContext(source, { exports: {}, require: (name) => { assert.equal(name, "electron"); return electron; }, console: { log() {} } });
  return { api, sent };
}

test("archived original screenshot preload fails before sending confirm in a Buffer-free sandbox", () => {
  const source = readFileSync("../../tests/delivery/windows-386-before/screenshots/clients/web-desktop/node_modules/electron-screenshots/lib/preload.js", "utf8");
  const state = loadSandboxPreload(source);
  assert.throws(() => state.api.ok(Uint8Array.of(1, 2, 3).buffer, {}), /Buffer is not defined/);
  assert.deepEqual(state.sent, []);
});

test("patched actual screenshot preload sends confirm and save bytes without a Buffer global", () => {
  const source = readFileSync("node_modules/electron-screenshots/lib/preload.js", "utf8");
  const state = loadSandboxPreload(source);
  const data = { bounds: { x: 2, y: 4, width: 10, height: 20 } };
  state.api.ok(Uint8Array.of(1, 2, 3).buffer, data);
  state.api.save(Uint8Array.of(4, 5).buffer, data);
  state.api.cancel();
  assert.equal(state.sent[0][0], "SCREENSHOTS:ok");
  assert.deepEqual(Array.from(state.sent[0][1]), [1, 2, 3]);
  assert.equal(state.sent[0][2], data);
  assert.equal(state.sent[1][0], "SCREENSHOTS:save");
  assert.deepEqual(Array.from(state.sent[1][1]), [4, 5]);
  assert.equal(state.sent[1][2], data);
  assert.deepEqual(state.sent[2], ["SCREENSHOTS:cancel"]);
});

// Use the INSTALLED dependency's real IPC listener, reset and endCapture code.
// Only Electron's OS/window boundary and the screen pixels are synthetic.
async function actualDependencyFixture({ original = false } = {}) {
  const ipcMain = new EventEmitter();
  const handlers = new Map(), events = [], responses = [], errors = [];
  ipcMain.handle = (name, handler) => { assert.equal(handlers.has(name), false); handlers.set(name, handler); };
  class WebContents extends EventEmitter {
    loadURL() { return Promise.resolve(); }
    send(channel) {
      events.push(channel);
      if (channel === "SCREENSHOTS:reset") queueMicrotask(() => ipcMain.emit("SCREENSHOTS:reset"));
    }
  }
  class BrowserView {
    webContents = new WebContents();
    setBounds() {}
  }
  class BrowserWindow extends EventEmitter {
    webContents = new WebContents();
    visible = false;
    isDestroyed() { return false; }
    isVisible() { return this.visible; }
    setKiosk() {}
    blur() {}
    blurWebView() {}
    unmaximize() {}
    removeBrowserView() { events.push("remove-view"); }
    setBrowserView() {}
    setBounds() {}
    setAlwaysOnTop() {}
    focus() {}
    show() { this.visible = true; this.emit("show"); }
    hide() { this.visible = false; events.push("overlay-hidden"); }
  }
  let clipboardThrows = false;
  const electron = {
    ipcMain, BrowserView, BrowserWindow,
    globalShortcut: { register() {}, unregister() {} },
    screen: { getCursorScreenPoint: () => ({ x: 0, y: 0 }), getDisplayNearestPoint: () => ({ id: 1, bounds: { x: 0, y: 0, width: 640, height: 480 }, scaleFactor: 1 }), screenToDipPoint: (point) => point },
    clipboard: { writeImage: () => { if (clipboardThrows) throw new Error("clipboard busy"); events.push("clipboard"); } },
    nativeImage: { createFromBuffer: (bytes) => bytes },
  };
  const dependencyRequire = createRequire(import.meta.resolve("electron-screenshots/lib/index.js"));
  const dependencyModule = { exports: {} };
  const dependencySource = readFileSync(dependencyRequire.resolve("./index.js"), "utf8");
  const syntheticRequire = (name) => {
    if (name === "electron") return electron;
    if (name === "./getDisplay") return { __esModule: true, default: () => ({ id: 1, x: 0, y: 0, width: 640, height: 480, scaleFactor: 1 }) };
    return dependencyRequire(name);
  };
  syntheticRequire.resolve = dependencyRequire.resolve;
  vm.runInNewContext(`(function(require,module,exports){${dependencySource}\n})`, { Buffer, setTimeout, clearTimeout, process })(syntheticRequire, dependencyModule, dependencyModule.exports);
  const RealScreenshots = dependencyModule.exports.default;
  let backend, instances = 0;
  class SyntheticScreenshots extends RealScreenshots {
    constructor(options) { super(options); instances++; backend = this; this.capture = async () => "data:image/png;base64,c3ludGhldGlj"; }
  }
  const windows = {
    hideWindow: () => events.push("main-hidden"),
    toggleWindowVisible: (visible) => events.push(visible ? "main-opaque" : "main-transparent"),
    showWindow: () => events.push("main-restored"),
    sendEvent: (_name, result) => { assert.equal(backend.$win?.isVisible() ?? false, false); responses.push(result); },
  };
  const entry = original
    ? { stdin: { contents: readFileSync("../../tests/delivery/windows-386-before/screenshots/clients/web-desktop/electron/main/screenshotsManage.ts", "utf8"), loader: "ts", resolveDir: resolve("electron/main") } }
    : { entryPoints: ["electron/main/screenshotsManage.ts"] };
  const bundle = await build({
    ...entry, write: false, bundle: true, format: "cjs", platform: "node",
    plugins: [{ name: "synthetic-electron-boundary", setup(builder) {
      builder.onResolve({ filter: /^(electron|electron-screenshots|i18next)$/ }, ({ path }) => ({ path, external: true }));
      builder.onResolve({ filter: /^\.\/windowManage$/ }, () => ({ path: "test-window-boundary", external: true }));
      builder.onResolve({ filter: /^\.\.\/utils$/ }, () => ({ path: "test-platform", external: true }));
    } }],
  });
  const module = { exports: {} };
  const bindings = { electron, "electron-screenshots": SyntheticScreenshots, i18next: { t: (name) => name }, "test-window-boundary": windows, "test-platform": { isWin: true } };
  vm.runInNewContext(`(function(require,module,exports){${bundle.outputFiles[0].text}\n})`, { Buffer, global: {}, console: { error: (...args) => errors.push(args) }, Promise, Uint8Array })((name) => { assert.ok(name in bindings, name); return bindings[name]; }, module, module.exports);
  const init = module.exports.default;
  const manager = init();
  if (!original) assert.equal(init(), manager);
  assert.equal(instances, 1);
  ipcMain.emit("SCREENSHOTS:ready");
  return { backend, ipcMain, events, responses, errors, manager, handlers, setClipboardFailure: (value) => { clipboardThrows = value; } };
}

test("archived original handler reproduces a stuck overlay when synchronous clipboard access fails", async () => {
  const state = await actualDependencyFixture({ original: true });
  state.setClipboardFailure(true);
  state.handlers.get("capture")({}, false);
  await tick();
  assert.equal(state.backend.$win.isVisible(), true);
  assert.throws(() => state.ipcMain.emit("SCREENSHOTS:ok", {}, Buffer.of(3), {}), /clipboard busy/);
  assert.equal(state.backend.$win.isVisible(), true);
  assert.equal(state.events.filter((name) => name === "remove-view").length, 0);
  assert.deepEqual(state.responses, []);
});

test("actual dependency IPC: confirm, duplicate results and canceled recapture close once without restarting", async () => {
  const state = await actualDependencyFixture();
  assert.equal(await state.manager.start(true), true, state.errors.map((entry) => String(entry[1])).join("\n"));
  assert.equal(state.backend.$win.isVisible(), true);
  state.ipcMain.emit("SCREENSHOTS:ok", {}, Buffer.of(1, 2), {});
  state.ipcMain.emit("SCREENSHOTS:ok", {}, Buffer.of(9), {});
  state.ipcMain.emit("SCREENSHOTS:cancel", {});
  await state.manager.cancel();
  assert.equal(state.backend.$win.isVisible(), false);
  assert.deepEqual(state.responses, ["data:image/png;base64,AQI="]);
  assert.equal(state.events.filter((name) => name === "remove-view").length, 1);
  assert.equal(await state.manager.start(), true);
  const input = { prevented: false, preventDefault() { this.prevented = true; } };
  state.backend.$view.webContents.emit("before-input-event", input, { type: "keyDown", key: "Escape" });
  await state.manager.cancel();
  assert.equal(input.prevented, true);
  assert.deepEqual(state.responses, ["data:image/png;base64,AQI=", undefined]);
  assert.equal(state.events.filter((name) => name === "remove-view").length, 2);
});

test("actual dependency IPC: clipboard busy still closes capture and returns one image", async () => {
  const state = await actualDependencyFixture();
  state.setClipboardFailure(true);
  assert.equal(await state.manager.start(), true, state.errors.map((entry) => String(entry[1])).join("\n"));
  assert.doesNotThrow(() => state.ipcMain.emit("SCREENSHOTS:ok", {}, Buffer.of(3), {}));
  await state.manager.cancel();
  assert.equal(state.backend.$win.isVisible(), false);
  assert.deepEqual(state.responses, ["data:image/png;base64,Aw=="]);
  assert.equal(state.errors.length, 1);
});

test("actual dependency Save completion shares its default close and restores once", async () => {
  const state = await actualDependencyFixture();
  assert.equal(await state.manager.start(true), true, state.errors.map((entry) => String(entry[1])).join("\n"));
  // Exact synchronous sequence in the installed library's successful Save path.
  state.backend.emit("afterSave", {}, Buffer.of(4), {}, true);
  const defaultClose = state.backend.endCapture();
  await Promise.all([defaultClose, state.manager.cancel()]);
  assert.equal(state.backend.$win.isVisible(), false);
  assert.deepEqual(state.responses, [undefined]);
  assert.equal(state.events.filter((name) => name === "remove-view").length, 1);
});
