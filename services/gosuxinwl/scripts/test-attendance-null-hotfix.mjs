// Exercise the exact R6 Linux hotfix compiled frontend in real headless Chrome.
// All API responses and saves are isolated in-memory fixtures on 127.0.0.1.
// The installed services, credentials, accounts, and business database are unused.
import assert from "node:assert/strict";
import { readFile, writeFile, mkdir, mkdtemp, rm } from "node:fs/promises";
import { createServer } from "node:http";
import { createHash } from "node:crypto";
import { inflateRawSync } from "node:zlib";
import { spawn } from "node:child_process";
import { resolve, join, relative, isAbsolute } from "node:path";
import { tmpdir } from "node:os";

const root = resolve(import.meta.dirname, ".."), output = join(root, "runtime/browser-tests");
const build = JSON.parse(await readFile(join(root, "runtime/public-hrm-r6-null-hotfix-build.json"), "utf8"));
const bytes = await readFile(build.output), sha = data => createHash("sha256").update(data).digest("hex");
assert.equal(sha(bytes), build.output_zip_sha256);
const run = "null-hotfix-" + Date.now(), reportPath = join(output, run + "-report.json");
await mkdir(output, { recursive: true });
const report = { run, package: build.output, package_sha256: sha(bytes), startedAt: new Date().toISOString(),
  frontend: build.new_asset, fixture_only: true, database_access: false, checks: [], cases: [], screenshots: [] };
const wait = ms => new Promise(resolve => setTimeout(resolve, ms));

function unpack(buffer) {
  let end = buffer.length - 22;
  while (end >= Math.max(0, buffer.length - 65557) && buffer.readUInt32LE(end) !== 0x06054b50) end--;
  assert.ok(end >= 0, "ZIP end record missing");
  const count = buffer.readUInt16LE(end + 10), entries = new Map(); let at = buffer.readUInt32LE(end + 16);
  for (let index = 0; index < count; index++) {
    assert.equal(buffer.readUInt32LE(at), 0x02014b50);
    const method = buffer.readUInt16LE(at + 10), size = buffer.readUInt32LE(at + 20), plainSize = buffer.readUInt32LE(at + 24), nameLength = buffer.readUInt16LE(at + 28), extra = buffer.readUInt16LE(at + 30), comment = buffer.readUInt16LE(at + 32), offset = buffer.readUInt32LE(at + 42), name = buffer.subarray(at + 46, at + 46 + nameLength).toString();
    assert.equal(buffer.readUInt32LE(offset), 0x04034b50);
    const start = offset + 30 + buffer.readUInt16LE(offset + 26) + buffer.readUInt16LE(offset + 28), raw = buffer.subarray(start, start + size);
    assert.ok(method === 0 || method === 8); const content = method === 0 ? raw : inflateRawSync(raw); assert.equal(content.length, plainSize);
    assert.ok(!entries.has(name)); entries.set(name, content); at += 46 + nameLength + extra + comment;
  }
  return entries;
}
const files = unpack(bytes), manifest = JSON.parse(files.get("plugin.json"));
assert.equal(sha(files.get("plugin.json")), build.after_manifest_sha256);
for (const [name, expected] of Object.entries(manifest.files)) assert.equal(sha(files.get(name)), expected);
const bundle = files.get(build.new_asset).toString();
assert.ok(bundle.includes("i=new Set(b(r).map(String))"));
assert.ok(!bundle.includes("i=new Set(r.map(String))"));
assert.equal(bundle.includes("local_windows"), false, "Do not imply that this hotfix migrates the R6 editor to v0.3 second windows");
report.r6_window_contract = { inherited_segment_window: "null or omitted; save remains non-customized", configured_window: "minute offsets preserved", second_windows_supported: false,
  limitation: "R6 has no local_windows editor; this minimal hotfix does not introduce or migrate v0.3 second-based attendance rules." };

const defaultWindow = { in_before_minutes: 90, in_after_minutes: 45, out_before_minutes: 40, out_after_minutes: 100 };
const customWindow = { in_before_minutes: 35, in_after_minutes: 20, out_before_minutes: 10, out_after_minutes: 70 };
const options = { employees: [{ id: "fixture-e1", name: "测试谢文冠", employee_no: "1001", department: "二部" }, { id: "fixture-e2", name: "测试同事", employee_no: "1002", department: "品质部" }], departments: [{ id: 10, name: "二部", pid: 0 }, { id: 20, name: "品质部", pid: 0 }] };
function fixture(id, employee_ids, department_ids, emptyWindow = null) {
  return { id, family_id: id, version: 1, name: id + "空范围专项", status: "draft", effective_from: "2026-10-01", effective_to: "", conflicts: null,
    scope: { employee_ids, department_ids, include_children: true, is_default: false },
    source: { note: "isolated compiled-bundle browser fixture" }, config: { category: "生产职员", calendar: null,
      shifts: [{ id: id + "-shift", name: "测试班次", version: "1", segments: [
        { id: id + "-inherited", name: "上午沿用制度窗口", kind: "work", start_minute: 480, end_minute: 720, punch_mode: "both", ...(emptyWindow === undefined ? {} : { window: emptyWindow }) },
        { id: id + "-custom", name: "下午自定义窗口", kind: "work", start_minute: 780, end_minute: 1020, punch_mode: "both", window: { ...customWindow } },
      ] }], cycle: { mode: "weekly", days: null, weekdays: Array.from({ length: 7 }, (_, index) => ({ shift_id: id + "-shift", category: index < 5 ? "workday" : "restday", scheduled_work: index < 5 })) },
      rule: { window: { ...defaultWindow }, punch_selection: "first_last", attendance_mode: "actual", standard_minutes: 0, late_arrival_tiers: null, leave_policies: null, full_attendance: { allowed_leave_types: null }, overtime: { workday: { enabled: true, require_approval: false, rounding_scope: "daily", minimum_minutes: 30, rounding_minutes: 30 }, restday: { enabled: true, require_approval: false, rounding_scope: "daily", minimum_minutes: 30, rounding_minutes: 30 } } },
    } };
}
const rules = [fixture("case-null-both", null, null), fixture("case-null-departments", ["fixture-e1"], null), fixture("case-null-employees", null, [10]), fixture("case-defined-scopes", ["fixture-e2"], [20])];
const saves = [], unexpected = [], requests = [];
const server = createServer(async (request, response) => {
  try {
    const url = new URL(request.url, "http://127.0.0.1"), path = url.pathname;
    requests.push({ method: request.method, path });
    const send = data => { response.setHeader("Content-Type", "application/json; charset=utf-8"); response.end(JSON.stringify({ code: 0, data })); };
    if (path.startsWith("/plugins/biz-suite/api/admin")) {
      const route = path.slice("/plugins/biz-suite/api/admin".length);
      if (request.method === "GET" && route === "/access") return send({ uid: 1, permissions: ["*"] });
      if (request.method === "GET" && route === "/im/openim/status") return send({ enabled: true, url: "https://im.invalid.example" });
      if (request.method === "GET" && route === "/hrm/options") return send(options);
      if (request.method === "GET" && route === "/hrm/attendance/v2/rules") return send({ items: rules });
      if (request.method === "POST" && route === "/hrm/attendance/v2/rules") {
        let body = ""; for await (const chunk of request) body += chunk;
        const value = JSON.parse(body), index = rules.findIndex(rule => rule.id === value.id);
        assert.ok(index >= 0, "Only exact fixture IDs may be saved"); saves.push(value); rules[index] = value; return send(value);
      }
      unexpected.push({ method: request.method, path }); response.statusCode = 400; return send({ error: "Unexpected fixture API" });
    }
    const adminPrefix = "/plugins/biz-suite/admin/";
    if (request.method === "GET" && path.startsWith(adminPrefix)) {
      const file = path.includes("/assets/") ? "admin/" + path.slice(adminPrefix.length) : "admin/index.html";
      const data = files.get(file); if (!data) { response.statusCode = 404; return response.end(); }
      response.setHeader("Content-Type", file.endsWith(".js") ? "text/javascript; charset=utf-8" : file.endsWith(".css") ? "text/css; charset=utf-8" : "text/html; charset=utf-8");
      response.setHeader("Cache-Control", "no-store"); return response.end(data);
    }
    response.statusCode = 404; response.end();
  } catch (error) { unexpected.push({ error: error.message }); response.statusCode = 500; response.end("Fixture error"); }
});
await new Promise(resolve => server.listen(0, "127.0.0.1", resolve));
const base = "http://127.0.0.1:" + server.address().port;
let chrome, profile, page;

class Page {
  constructor(socket) {
    this.socket = socket; this.id = 0; this.pending = new Map(); this.errors = []; this.external = [];
    socket.addEventListener("message", ({ data }) => {
      const message = JSON.parse(data);
      if (message.id) { const item = this.pending.get(message.id); if (item) { clearTimeout(item.timer); this.pending.delete(message.id); message.error ? item.reject(Error(message.error.message)) : item.resolve(message.result); } }
      if (message.method === "Runtime.exceptionThrown") this.errors.push(message.params.exceptionDetails.exception?.description || message.params.exceptionDetails.text);
      if (message.method === "Fetch.requestPaused") {
        const { request, requestId } = message.params, external = new URL(request.url).origin !== base;
        if (external) this.external.push(new URL(request.url).origin);
        this.send(external ? "Fetch.failRequest" : "Fetch.continueRequest", external ? { requestId, errorReason: "BlockedByClient" } : { requestId }).catch(error => this.errors.push(error.message));
      }
    });
  }
  send(method, params = {}) { return new Promise((resolve, reject) => { const id = ++this.id, timer = setTimeout(() => { this.pending.delete(id); reject(Error("Chrome timeout: " + method)); }, 15000); this.pending.set(id, { resolve, reject, timer }); this.socket.send(JSON.stringify({ id, method, params })); }); }
  async eval(expression) { const value = await this.send("Runtime.evaluate", { expression, awaitPromise: true, returnByValue: true }); if (value.exceptionDetails) throw Error(value.exceptionDetails.exception?.description || value.exceptionDetails.text); return value.result.value; }
  async until(expression) { const deadline = Date.now() + 15000; while (Date.now() < deadline) { if (await this.eval(expression)) return; await wait(75); } throw Error("Page condition timed out: " + expression); }
  async shot(name) { const { data } = await this.send("Page.captureScreenshot", { format: "png" }); const file = `${run}-${name}.png`; await writeFile(join(output, file), Buffer.from(data, "base64")); report.screenshots.push(file); }
}
async function launch() {
  profile = await mkdtemp(join(tmpdir(), "attendance-null-hotfix-"));
  chrome = spawn(process.env.CHROME_PATH || "C:/Program Files/Google/Chrome/Application/chrome.exe", ["--headless=new", "--disable-gpu", "--no-first-run", "--no-default-browser-check", "--remote-debugging-port=0", "--remote-allow-origins=*", "--no-proxy-server", "--user-data-dir=" + profile, "about:blank"], { stdio: "ignore", windowsHide: true });
  let port; for (let i = 0; i < 100 && !port; i++) { try { port = (await readFile(join(profile, "DevToolsActivePort"), "utf8")).split("\n")[0]; } catch { await wait(100); } }
  assert.ok(port); const targets = await (await fetch(`http://127.0.0.1:${port}/json/list`)).json();
  const socket = new WebSocket(targets.find(target => target.type === "page").webSocketDebuggerUrl);
  await new Promise((resolve, reject) => { const timer = setTimeout(() => reject(Error("CDP connection failed")), 10000); socket.addEventListener("open", () => { clearTimeout(timer); resolve(); }, { once: true }); });
  page = new Page(socket); await page.send("Runtime.enable"); await page.send("Page.enable"); await page.send("Fetch.enable", { patterns: [{ urlPattern: "*", requestStage: "Request" }] });
  await page.send("Emulation.setDeviceMetricsOverride", { width: 1440, height: 1050, deviceScaleFactor: 1, mobile: false });
  await page.send("Page.navigate", { url: base + "/plugins/biz-suite/admin/hrm/attendance-rules" });
  await page.until("!!document.querySelector('#view tbody tr')");
}

try {
  await launch();
  report.loaded_bundle = await page.eval("[...document.scripts].map(script=>script.src).find(url=>url.includes('/assets/app-'))");
  assert.equal(report.loaded_bundle, base + "/plugins/biz-suite/" + build.new_asset);
  for (const original of structuredClone(rules)) {
    await page.eval(`(()=>{const toast=document.querySelector('#toast');if(toast){toast.textContent='';toast.classList.remove('show');}const search=document.querySelector('#view input[type=search]');search.value=${JSON.stringify(original.id)};search.dispatchEvent(new Event('input',{bubbles:true}));const row=[...document.querySelectorAll('#view tbody tr')].find(row=>row.textContent.includes(${JSON.stringify(original.name)}));const button=[...row.querySelectorAll('button')].find(button=>button.textContent.trim()==='编辑 / 新版本');button.click();})()`);
    await page.until("!!document.querySelector('[data-attendance-rule-editor]')||!!document.querySelector('#toast.show')");
    assert.equal(await page.eval("!!document.querySelector('[data-attendance-rule-editor]')"), true, "Null scopes must open the editor");
    const selection = await page.eval("(()=>{const form=document.querySelector('[data-attendance-rule-editor]');const count=title=>[...form.querySelectorAll('section')].find(section=>section.querySelector(':scope > h3')?.textContent===title).querySelectorAll('input:checked').length;return {employees:count('适用人员（优先于部门）'),departments:count('适用本地部门'),valid:form.checkValidity()};})()");
    assert.equal(selection.employees, original.scope.employee_ids?.length || 0); assert.equal(selection.departments, original.scope.department_ids?.length || 0);
    assert.equal(selection.valid, true, "Fixture must be a complete form under the original R6 rules");
    if (report.cases.length === 0) await page.shot("null-scopes-editor");
    const countBefore = saves.length;
    await page.eval("document.querySelector('[data-attendance-rule-editor]').requestSubmit()");
    await page.until("!document.querySelector('[data-attendance-rule-editor]')||!!document.querySelector('[data-attendance-rule-editor] [role=alert]')");
    assert.equal(await page.eval("document.querySelector('[data-attendance-rule-editor] [role=alert]')?.textContent||''"), "");
    assert.equal(saves.length, countBefore + 1); const saved = saves.at(-1);
    assert.deepEqual(saved.scope.employee_ids, original.scope.employee_ids || []); assert.deepEqual(saved.scope.department_ids, original.scope.department_ids || []);
    assert.deepEqual(saved.config.rule.window, defaultWindow);
    assert.equal(Object.hasOwn(saved.config.shifts[0].segments[0], "window"), false, "An inherited null window must not become a custom window");
    assert.deepEqual(saved.config.shifts[0].segments[1].window, customWindow, "Explicit custom minute boundaries must be retained");
    report.cases.push({ id: original.id, employee_scope_null: original.scope.employee_ids === null, department_scope_null: original.scope.department_ids === null,
      opened: true, saved: true, scope_values_preserved: true, inherited_window_remained_unset: true, explicit_windows_preserved: true });
    console.log("PASS " + original.id);
  }
  assert.deepEqual(page.errors, []); assert.deepEqual(page.external, []); assert.deepEqual(unexpected, []);
  report.api_requests = requests; report.js_errors = page.errors; report.external_requests = page.external;
  assert.equal(requests.filter(request => request.method !== "GET").length, rules.length, "Only fixture draft saves may mutate in-memory state");
  report.passed = true;
} catch (error) {
  report.passed = false; report.error = error.message; report.js_errors = page?.errors || []; report.unexpected_requests = unexpected; process.exitCode = 1;
  console.error("FAIL " + error.message); try { await page?.shot("failure"); } catch {}
} finally {
  try { page?.socket.close(); chrome?.kill(); } catch {}
  await new Promise(resolve => server.close(resolve));
  if (profile) {
    const parent = resolve(tmpdir()), target = resolve(profile), child = relative(parent, target);
    assert.ok(child && !child.startsWith("..") && !isAbsolute(child) && target.includes("attendance-null-hotfix-"));
    await wait(500); await rm(target, { recursive: true, force: true, maxRetries: 5, retryDelay: 300 }).catch(() => {});
  }
  report.cleanup = { fixture_server_stopped: true, business_database_untouched: true };
  report.finishedAt = new Date().toISOString(); await writeFile(reportPath, JSON.stringify(report, null, 2)); console.log("Report: " + reportPath);
}
