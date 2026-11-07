// Read-only real HTTPS regression for every installed attendance rule editor.
// Only an isolated temporary administrator is created and cleaned up. Business
// writes are blocked at Chrome's request layer; no rule is saved or published.
import assert from "node:assert/strict";
import { readFile, writeFile, mkdir, mkdtemp, rm } from "node:fs/promises";
import { existsSync } from "node:fs";
import { resolve, join, relative, isAbsolute } from "node:path";
import { tmpdir } from "node:os";
import { spawn } from "node:child_process";
import { createHash } from "node:crypto";
import { createTestAdmin, cleanupRun } from "./cleanup-biz-suite-test.mjs";

const root = resolve(import.meta.dirname, ".."), out = join(root, "runtime/browser-tests");
const base = "https://kf.szputy.com:9880", prefix = "/plugins/biz-suite/admin/";
const run = "QA" + Date.now(), expectedError = process.argv.includes("--expect-null-error");
assert.ok(process.argv.slice(2).every(arg => arg === "--expect-null-error"), "Unknown argument");
const access = JSON.parse((await readFile(join(root, "runtime/local-access.json"), "utf8")).replace(/^\uFEFF/, ""));
assert.equal(access.database, "kaoqin_dev");
await mkdir(out, { recursive: true });
const reportFile = join(out, `attendance-rule-edit-${run}-report.json`);
const report = { run, base, mode: expectedError ? "reproduce-null-error" : "all-editors-readonly", startedAt: new Date().toISOString(),
  transport: "Real HTTPS 9880; Chrome maps kf.szputy.com to 127.0.0.1 and accepts the existing temporary certificate",
  checks: [], editors: [], screenshots: [], jsErrors: [], blockedBusinessWrites: [] };
const wait = ms => new Promise(resolve => setTimeout(resolve, ms));
const rows = value => Array.isArray(value) ? value : value?.items || value?.list || value?.rows || [];
const stable = value => Array.isArray(value) ? value.map(stable) : value && typeof value === "object" ? Object.fromEntries(Object.keys(value).sort().map(key => [key, stable(value[key])])) : value;
const hashRules = value => createHash("sha256").update(JSON.stringify(stable([...value].sort((a, b) => a.id.localeCompare(b.id))))).digest("hex");
const keys = ["in_start_second", "in_end_second", "out_start_second", "out_end_second"];
let admin, chrome, profile, page, before, selectable;

function windowValue(value, key) {
  if (!Number.isInteger(value)) return { day: "0", time: "" };
  const seconds = value - (key.endsWith("end_second") ? 1 : 0), day = Math.floor(seconds / 86400), within = seconds - day * 86400;
  return { day: String(day), time: [Math.floor(within / 3600), Math.floor(within % 3600 / 60), within % 60].map(n => String(n).padStart(2, "0")).join(":") };
}
function expectedWindows(rule) {
  const seen = new Set(), windows = [];
  function shift(saved = {}) {
    if (saved.id && seen.has(saved.id)) return;
    if (saved.id) seen.add(saved.id);
    const segments = saved.segments || [{}, {}, {}];
    for (const item of [...(saved.evidence_periods || []), ...segments]) windows.push(Object.fromEntries(keys.map(key => [key, windowValue((item.local_windows || item.pending_local_windows)?.[key], key)])));
    if (saved.overtime_variant?.shift) shift(saved.overtime_variant.shift);
  }
  for (const saved of rule.config?.shifts?.length ? rule.config.shifts : [{}]) shift(saved);
  return windows;
}
const sortedWindows = values => values.map(value => JSON.stringify(stable(value))).sort();

class BrowserPage {
  constructor(socket) {
    this.socket = socket; this.seq = 0; this.pending = new Map(); this.errors = []; this.businessWrites = []; this.interceptorErrors = [];
    socket.addEventListener("message", ({ data }) => {
      const event = JSON.parse(data);
      if (event.id) { const item = this.pending.get(event.id); if (item) { clearTimeout(item.timer); this.pending.delete(event.id); event.error ? item.reject(Error(event.error.message)) : item.resolve(event.result); } }
      if (event.method === "Runtime.exceptionThrown") this.errors.push(event.params.exceptionDetails.exception?.description || event.params.exceptionDetails.text);
      if (event.method === "Fetch.requestPaused") {
        const { request, requestId } = event.params, denied = !["GET", "HEAD", "OPTIONS"].includes(request.method);
        if (denied) this.businessWrites.push({ method: request.method, path: new URL(request.url).pathname });
        this.send(denied ? "Fetch.failRequest" : "Fetch.continueRequest", denied ? { requestId, errorReason: "BlockedByClient" } : { requestId }).catch(error => this.interceptorErrors.push(error.message));
      }
    });
    socket.addEventListener("close", () => { for (const item of this.pending.values()) { clearTimeout(item.timer); item.reject(Error("QA Chrome closed")); } this.pending.clear(); });
  }
  send(method, params = {}) { return new Promise((resolve, reject) => { const id = ++this.seq, timer = setTimeout(() => { this.pending.delete(id); reject(Error("Chrome timeout: " + method)); }, 20000); this.pending.set(id, { resolve, reject, timer }); this.socket.send(JSON.stringify({ id, method, params })); }); }
  async eval(expression) { const result = await this.send("Runtime.evaluate", { expression, awaitPromise: true, returnByValue: true }); if (result.exceptionDetails) throw Error(result.exceptionDetails.exception?.description || result.exceptionDetails.text); return result.result.value; }
  async until(expression, timeout = 20000) { const end = Date.now() + timeout; while (Date.now() < end) { if (await this.eval(expression)) return; await wait(80); } throw Error("Page condition timed out: " + expression); }
  async rules() { return rows(await this.eval(`(async()=>{const {api}=await import('${prefix}assets/core.js');return api('/hrm/attendance/v2/rules');})()`)); }
  async options() { return this.eval(`(async()=>{const {api}=await import('${prefix}assets/core.js');return api('/hrm/options');})()`); }
  async shot(label) { const { data } = await this.send("Page.captureScreenshot", { format: "png", captureBeyondViewport: false }); const name = `attendance-rule-edit-${run}-${label}.png`; await writeFile(join(out, name), Buffer.from(data, "base64")); report.screenshots.push(name); }
  async openRule(rule, history = false) {
    const selector = history ? "[data-attendance-rule-history]" : "[data-attendance-current-rules]";
    await this.eval(`(()=>{const host=document.querySelector(${JSON.stringify(selector)});if(!host)throw Error('Missing rule section');if(host.matches('details'))host.open=true;const toast=document.querySelector('#toast');if(toast){toast.textContent='';toast.classList.remove('show');}const input=host.querySelector('input[type=search]');input.value=${JSON.stringify(rule.id)};input.dispatchEvent(new Event('input',{bubbles:true}));})()`);
    await this.until(`document.querySelectorAll(${JSON.stringify(selector + " tbody tr")}).length===1`);
    await this.eval(`(()=>{const row=document.querySelector(${JSON.stringify(selector + " tbody tr")});const button=[...row.querySelectorAll('button')].find(item=>item.textContent.trim()===${JSON.stringify(history ? "查看" : "编辑 / 新版本")});if(!button)throw Error('Missing rule editor action');button.click();})()`);
    await this.until("!!document.querySelector('[data-attendance-rule-editor]')||!!document.querySelector('#toast.show')?.textContent");
    return this.eval("({opened:!!document.querySelector('[data-attendance-rule-editor]'),toast:document.querySelector('#toast.show')?.textContent||''})");
  }
  async inspect(rule, history) {
    const actual = await this.eval(`(()=>{const form=document.querySelector('[data-attendance-rule-editor]');const selected=title=>[...form.querySelectorAll('section')].find(section=>section.querySelector(':scope > h3')?.textContent===title)?.querySelectorAll('input[type=checkbox]:checked').length;return {name:form.querySelector('[name=name]').value,employee_selected:selected('适用人员（优先于部门）'),department_selected:selected('适用本地部门'),saveButtons:[...form.querySelectorAll('button[type=submit]')].map(n=>n.textContent),enabledControls:[...form.querySelectorAll('input,select,button')].filter(n=>!n.disabled).length,windows:[...form.querySelectorAll('[data-local-windows]')].map(row=>Object.fromEntries(${JSON.stringify(keys)}.map(key=>[key,{day:row.querySelector('[name="'+key+'_day"]').value,time:row.querySelector('[name="'+key+'"]').value}])))};})()`);
    assert.equal(actual.name, rule.name, "The editor must show the selected rule");
    assert.deepEqual(sortedWindows(actual.windows), sortedWindows(expectedWindows(rule)), "Opening the editor must preserve configured seconds and blank windows");
    const employeeIDs = new Set(rows(rule.scope?.employee_ids).map(String)), departmentIDs = new Set(rows(rule.scope?.department_ids).map(String));
    assert.equal(actual.employee_selected, rows(selectable.employees).filter(employee => employeeIDs.has(String(employee.id))).length, "Available scoped employees must remain selected");
    assert.equal(actual.department_selected, rows(selectable.departments).filter(department => departmentIDs.has(String(department.id))).length, "Available scoped departments must remain selected");
    if (history) { assert.equal(actual.enabledControls, 0, "Historical controls must be disabled"); assert.deepEqual(actual.saveButtons, []); }
    else assert.ok(actual.saveButtons.includes("保存草稿") && actual.saveButtons.includes("保存并生效"), "Current rule editor must retain its save actions");
    return { id: rule.id, name: rule.name, status: rule.status, history, opened: true, readonly: history,
      scope_employee_ids_null: rule.scope?.employee_ids === null, scope_department_ids_null: rule.scope?.department_ids === null,
      employee_selected_count: actual.employee_selected, department_selected_count: actual.department_selected, scope_selections_preserved: true,
      window_groups: actual.windows.length, unconfigured_boundaries: actual.windows.flatMap(window => Object.values(window)).filter(boundary => boundary.time === "").length, windows_preserved: true };
  }
  async closeEditor() { await this.eval("document.querySelector('dialog [aria-label=关闭]').click()"); await this.until("!document.querySelector('dialog')"); }
}

async function startBrowser() {
  profile = await mkdtemp(join(tmpdir(), "attendance-rule-edit-qa-"));
  const executable = process.env.CHROME_PATH || "C:/Program Files/Google/Chrome/Application/chrome.exe";
  assert.ok(existsSync(executable), "Chrome is unavailable");
  chrome = spawn(executable, ["--headless=new", "--disable-gpu", "--no-first-run", "--no-default-browser-check", "--ignore-certificate-errors", "--remote-debugging-port=0", "--remote-allow-origins=*", "--host-resolver-rules=MAP kf.szputy.com 127.0.0.1", "--no-proxy-server", "--user-data-dir=" + profile, "about:blank"], { stdio: "ignore", windowsHide: true });
  let port;
  for (let i = 0; i < 100 && !port; i++) { try { port = (await readFile(join(profile, "DevToolsActivePort"), "utf8")).split("\n")[0]; } catch { await wait(100); } }
  assert.ok(port, "Chrome startup failed");
  const tabs = await (await fetch(`http://127.0.0.1:${port}/json/list`, { signal: AbortSignal.timeout(5000) })).json();
  const socket = new WebSocket(tabs.find(tab => tab.type === "page").webSocketDebuggerUrl);
  await new Promise((resolve, reject) => { const timer = setTimeout(() => reject(Error("Chrome connection timed out")), 10000); socket.addEventListener("open", () => { clearTimeout(timer); resolve(); }, { once: true }); socket.addEventListener("error", () => { clearTimeout(timer); reject(Error("Chrome connection failed")); }, { once: true }); });
  page = new BrowserPage(socket);
  await page.send("Runtime.enable"); await page.send("Page.enable");
  await page.send("Fetch.enable", { patterns: [{ urlPattern: "*/plugins/biz-suite/api/admin/*", requestStage: "Request" }] });
  await page.send("Emulation.setDeviceMetricsOverride", { width: 1440, height: 1050, deviceScaleFactor: 1, mobile: false });
  await page.send("Page.navigate", { url: base + prefix + "hrm/attendance-rules" });
  await page.until("!!document.querySelector('input[type=password]')"); await wait(500);
  await page.eval(`(()=>{const form=document.querySelector('input[type=password]').closest('form');for(const [selector,value]of [['input[type=text]',${JSON.stringify(admin.username)}],['input[type=password]',${JSON.stringify(admin.password)}]]){const input=form.querySelector(selector);input.value=value;input.dispatchEvent(new Event('input',{bubbles:true}));}const checkbox=form.querySelector('input[type=checkbox]');if(checkbox?.checked)checkbox.click();form.querySelector('button[type=submit]').click();})()`);
  await page.until("!!document.querySelector('[data-attendance-current-rules] input[type=search]')");
}
async function step(name, action) {
  const started = Date.now();
  try { await action(); report.checks.push({ name, passed: true, ms: Date.now() - started }); console.log("PASS " + name); }
  catch (error) { report.checks.push({ name, passed: false, error: error.message }); throw error; }
  finally { await writeFile(reportFile, JSON.stringify(report, null, 2)); }
}

try {
  await step("isolated-admin-and-real-https-login", async () => { admin = await createTestAdmin(run); report.temporary_admin_uid = admin.uid; await startBrowser(); });
  await step("snapshot-all-real-rules", async () => {
    before = await page.rules(); selectable = await page.options(); assert.ok(before.length > 0);
    report.before_hash = hashRules(before); report.rule_ids = before.map(rule => rule.id);
    report.total_rules = before.length; report.current_rules = before.filter(rule => ["draft", "published"].includes(rule.status)).length;
    report.historical_rules = before.length - report.current_rules;
    report.null_employee_scopes = before.filter(rule => rule.scope?.employee_ids === null).length;
    report.null_department_scopes = before.filter(rule => rule.scope?.department_ids === null).length;
  });
  if (expectedError) {
    await step("reproduce-null-map-error-with-real-imported-draft", async () => {
      const rule = before.find(rule => rule.status === "draft" && (rule.scope?.employee_ids === null || rule.scope?.department_ids === null));
      assert.ok(rule, "Expected an imported draft with a null scope array");
      const state = await page.openRule(rule);
      assert.equal(state.opened, false, "The deployed editor was expected to reproduce the bug");
      assert.match(state.toast, /Cannot read propert(?:y|ies) of null.*map/i);
      report.reproduced = { id: rule.id, name: rule.name, message: state.toast };
      await page.shot("null-map-reproduced");
    });
  } else {
    await step("open-every-current-and-historical-rule-without-saving", async () => {
      for (const rule of before) {
        const history = !["draft", "published"].includes(rule.status), state = await page.openRule(rule, history);
        assert.equal(state.toast, "", `Rule ${rule.name} unexpectedly displayed an error`);
        assert.equal(state.opened, true, `Rule ${rule.name} failed to open`);
        const verified = await page.inspect(rule, history); report.editors.push(verified);
        if ((!history && report.editors.filter(item => !item.history).length === 1) || (history && report.editors.filter(item => item.history).length === 1)) await page.shot(history ? "historical-readonly" : "current-rule-editor");
        await page.closeEditor();
        if (report.editors.length % 20 === 0) { console.log(`Verified ${report.editors.length}/${before.length} editors`); await writeFile(reportFile, JSON.stringify(report, null, 2)); }
      }
      assert.equal(report.editors.length, before.length);
    });
  }
  await step("no-business-writes-no-rule-change-no-unhandled-js-errors", async () => {
    const after = await page.rules(); report.after_hash = hashRules(after); report.rules_unchanged = report.after_hash === report.before_hash;
    assert.equal(report.after_hash, report.before_hash, "Rule records must be identical before and after UI inspection");
    assert.deepEqual(page.businessWrites, [], "No business write should even be attempted");
    assert.deepEqual(page.errors, [], "The real page must have no unhandled JavaScript errors");
    assert.deepEqual(page.interceptorErrors, [], "Read-only request interception must remain healthy");
  });
  report.passed = true;
} catch (error) {
  report.passed = false; report.error = error.message; process.exitCode = 1;
  console.error("FAIL attendance-rule-edit: " + error.message);
  try { await page?.shot("failure"); } catch {}
} finally {
  if (before && page && !report.after_hash) {
    try { report.after_hash = hashRules(await page.rules()); report.rules_unchanged = report.after_hash === report.before_hash; if (!report.rules_unchanged) process.exitCode = 1; }
    catch (error) { report.final_snapshot_error = error.message; process.exitCode = 1; }
  }
  report.jsErrors = page?.errors || []; report.blockedBusinessWrites = page?.businessWrites || [];
  try { page?.socket.close(); chrome?.kill(); } catch {}
  if (admin) { try { await cleanupRun(run); report.cleanup = { passed: true, remaining: 0 }; } catch (error) { report.cleanup = { passed: false, error: error.message }; process.exitCode = 1; } }
  if (profile) {
    const parent = resolve(tmpdir()), target = resolve(profile), child = relative(parent, target);
    assert.ok(child && !child.startsWith("..") && !isAbsolute(child) && target.includes("attendance-rule-edit-qa-"), "Unsafe browser-profile cleanup path");
    await wait(500); await rm(target, { recursive: true, force: true, maxRetries: 5, retryDelay: 300 }).catch(() => {});
  }
  if (process.exitCode) report.passed = false;
  report.finishedAt = new Date().toISOString(); await writeFile(reportFile, JSON.stringify(report, null, 2));
  console.log("Report: " + reportFile);
}
