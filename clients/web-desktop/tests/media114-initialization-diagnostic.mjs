// One page at a time; no fixture, login, SDK method invocation or UI mutation.
import { chromium } from "playwright";
import { mkdir, writeFile } from "node:fs/promises";
import { resolve } from "node:path";
import { createHash } from "node:crypto";

const root = resolve("../.."), stamp = new Date().toISOString().replace(/[-:.]/g, "");
const out = resolve(root, "tests/delivery/media-resume-20261102/web-runtime", "initialization-diagnostic-" + stamp);
await mkdir(out, { recursive: true });
const temp = resolve(root, ".cache/temp-web/media114"); await mkdir(temp, { recursive: true }); process.env.TEMP = temp; process.env.TMP = temp;
const safeURL = raw => { try { const u = new URL(raw); return u.origin + u.pathname; } catch { return "InvalidURL"; } };
const clean = raw => String(raw).replace(/eyJ[A-Za-z0-9_-]{15,}\.[A-Za-z0-9_-]{20,}\.[A-Za-z0-9_-]{15,}/g, "[JWT_REDACTED]").replace(/((?:https?|wss?):\/\/[^\s?]+)\?[^\s]+/g, "$1?[QUERY_REDACTED]").slice(0, 500);
const timeout = async (promise, ms) => { let t; try { return await Promise.race([promise, new Promise(resolve => { t = setTimeout(() => resolve({ diagnosticTimedOut: true }), ms); })]); } finally { clearTimeout(t); } };
const results = []; let browser;
try {
  browser = await chromium.launch({ headless: true, executablePath: "C:/Program Files/Google/Chrome/Application/chrome.exe", args: ["--use-fake-device-for-media-stream", "--use-fake-ui-for-media-stream"] });
  const targets = process.argv.includes("--vite-only") ? [["source-vite-7777-after-owned-restart", "http://127.0.0.1:7777/"]] : [["deployed-static-387", "https://192.168.10.11:29880/"], ["source-vite-7777", "http://127.0.0.1:7777/"]];
  for (const [label, url] of targets) {
    const context = await browser.newContext({ locale: "zh-CN", viewport: { width: 1440, height: 1000 } }), page = await context.newPage();
    const r = { label, url, startedUtc: new Date().toISOString(), requests: [], errors: [], consoles: [] }; results.push(r);
    const map = new Map();
    page.on("request", request => { const value = { url: safeURL(request.url()), resourceType: request.resourceType(), startedMs: Date.now(), method: request.method() }; map.set(request, value); r.requests.push(value); });
    page.on("response", response => { const item = map.get(response.request()); if (item) Object.assign(item, { status: response.status(), contentType: response.headers()["content-type"] || null, contentLengthHeader: response.headers()["content-length"] || null }); });
    page.on("requestfinished", request => { const item = map.get(request); if (item) Object.assign(item, { finishedMs: Date.now(), durationMs: Date.now() - item.startedMs }); });
    page.on("requestfailed", request => { const item = map.get(request); if (item) Object.assign(item, { failed: true, reason: clean(request.failure()?.errorText || "RequestFailed"), durationMs: Date.now() - item.startedMs }); });
    page.on("pageerror", error => r.errors.push({ name: error.name, message: clean(error.message) }));
    page.on("console", async message => {
      const text = message.text();
      if (!text.startsWith("Failed to initialize WASM:") && !text.startsWith("Failed to put cache") && !["SharedArrayBuffer", "CORS", "Outdated Optimize Dep", "Failed to load module script"].some(x => text.includes(x))) return;
      const value = { category: text.startsWith("Failed to initialize WASM:") ? "FailedToInitializeWASM" : text.startsWith("Failed to put cache") ? "FailedToPutCache" : "ModuleOrIsolationError", type: message.type(), causes: [] };
      for (const arg of message.args()) { try { const cause = await arg.evaluate(v => v && typeof v === "object" ? { name: typeof v.name === "string" ? v.name : null, message: typeof v.message === "string" ? v.message : null } : null); if (cause?.name && ["CompileError", "LinkError", "RuntimeError", "TypeError", "RangeError", "Error", "SecurityError", "AbortError"].includes(cause.name)) value.causes.push({ name: cause.name, message: clean(cause.message || "") }); } catch {} }
      r.consoles.push(value);
    });
    const started = Date.now();
    try {
      await page.goto(url, { waitUntil: "domcontentloaded", timeout: 60000 }); r.domContentLoadedMs = Date.now() - started;
      try { await page.waitForFunction(() => typeof window.commonEventFunc === "function" && typeof window.login === "function", undefined, { timeout: 60000 }); r.originalWasmEntryReady = true; } catch (error) { r.originalWasmEntryReady = false; r.waitError = { name: error.name, message: clean(error.message) }; }
      r.readonlyPageState = await timeout(page.evaluate(() => ({ readyState: document.readyState, route: location.pathname + location.hash, commonEventFunc: typeof window.commonEventFunc, login: typeof window.login, Go: typeof window.Go, crossOriginIsolated: window.crossOriginIsolated, passwordInputExists: Boolean(document.getElementById("password")), loginButtonCount: [...document.querySelectorAll("button")].filter(b => b.textContent.trim() === "登录").length, resources: performance.getEntriesByType("resource").filter(e => /wasm|worker/i.test(e.name)).map(e => ({ url: new URL(e.name).origin + new URL(e.name).pathname, duration: e.duration, transferSize: e.transferSize, encodedBodySize: e.encodedBodySize, decodedBodySize: e.decodedBodySize })) })), 3000);
      r.cacheNames = await timeout(page.evaluate(async () => ({ names: await caches.keys() })), 3000);
      r.storageEstimate = await timeout(page.evaluate(async () => { const s = await navigator.storage.estimate(); return { usageBytes: s.usage, quotaBytes: s.quota }; }), 3000);
    } catch (error) { r.failure = { name: error.name, message: clean(error.message) }; }
    r.elapsedMs = Date.now() - started; r.finishedUtc = new Date().toISOString();
    await context.close();
    const bytes = JSON.stringify(r, null, 2) + "\n"; await writeFile(resolve(out, label + ".json"), bytes, { flag: "wx" });
    console.log(JSON.stringify({ label, originalWasmEntryReady: r.originalWasmEntryReady, elapsedMs: r.elapsedMs, consoles: r.consoles, wasmResources: r.readonlyPageState?.resources, cacheNames: r.cacheNames }));
  }
} finally {
  await browser?.close();
  const report = { createdUtc: new Date().toISOString(), results, scope: "Sequential one-page-only real Chrome diagnostic, normal TLS, no fixtures read/login/UI controls/token injection/SDK API calls. No product, SDK or asset modification.", observerOverridesInstalled: false };
  const bytes = JSON.stringify(report, null, 2) + "\n", p = resolve(out, "report.json"); await writeFile(p, bytes, { flag: "wx" });
  console.log(JSON.stringify({ report: p, sha256: createHash("sha256").update(bytes).digest("hex") }));
}
