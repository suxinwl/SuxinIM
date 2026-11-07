// Read-only verification of the public Ubuntu R6 frontend, through its real IP.
// Credentials are read from a local temporary file and never logged or saved.
// No business record is written; the caller owns temporary account cleanup.
import assert from "node:assert/strict";
import { readFile, writeFile, mkdir, mkdtemp, rm } from "node:fs/promises";
import { createHash } from "node:crypto";
import { spawn } from "node:child_process";
import { resolve, join, relative, isAbsolute } from "node:path";
import { tmpdir } from "node:os";

const root = resolve(import.meta.dirname, ".."), output = join(root, "runtime/browser-tests");
const credentials = JSON.parse((await readFile(join(root, "runtime/ubuntu-attendance-qa-credentials.json"), "utf8")).replace(/^\uFEFF/, ""));
assert.equal(typeof credentials.username, "string"); assert.equal(typeof credentials.password, "string");
assert.ok(credentials.username && credentials.password, "Temporary test credentials must be present");
const base = "https://kf.szputy.com:9880", mappedIP = "61.144.183.217";
const adminPrefix = "/plugins/biz-suite/admin/", apiPrefix = "/plugins/biz-suite/api/admin";
const expectedAsset = "app-hrm-null-scope-238b0f2f25c8.js";
const run = "ubuntu-attendance-edit-" + Date.now(), reportPath = join(output, run + "-report.json");
const report = { run, base, mappedIP, expectedAsset, startedAt: new Date().toISOString(),
  transport: "Public domain with Chrome DNS pinned to actual public IP 61.144.183.217; existing temporary certificate accepted",
  business_writes_allowed: false, checks: [], editors: [], screenshots: [] };
const wait = ms => new Promise(resolve => setTimeout(resolve, ms));
const rows = value => Array.isArray(value) ? value : value?.items || value?.list || value?.rows || [];
const stable = value => Array.isArray(value) ? value.map(stable) : value && typeof value === "object" ? Object.fromEntries(Object.keys(value).sort().map(key => [key, stable(value[key])])) : value;
const hash = value => createHash("sha256").update(JSON.stringify(stable([...value].sort((a, b) => a.id.localeCompare(b.id))))).digest("hex");
const minuteKeys = ["in_before_minutes", "in_after_minutes", "out_before_minutes", "out_after_minutes"];
let chrome, profile, page, before, options;
await mkdir(output, { recursive: true });

function expectedMinuteWindows(rule) {
  const defaults = window => Object.fromEntries(minuteKeys.map(key => [key, String(window?.[key] ?? 120)]));
  const result = [defaults(rule.config?.rule?.window)], seen = new Set();
  function append(shift = {}) {
    if (shift.id && seen.has(shift.id)) return;
    if (shift.id) seen.add(shift.id);
    for (const segment of shift.segments || [{}, {}, {}]) result.push(defaults(segment.window));
    if (shift.overtime_variant?.shift) append(shift.overtime_variant.shift);
  }
  for (const shift of rule.config?.shifts?.length ? rule.config.shifts : [{}]) append(shift);
  return result;
}
class Page {
  constructor(socket) {
    this.socket = socket; this.sequence = 0; this.pending = new Map(); this.errors = []; this.blocked = []; this.interceptorErrors = []; this.serverAddresses = new Set();
    this.documentPath = "about:blank"; this.network = new Map(); this.responseTasks = new Set(); this.apiResponses = []; this.errorEvents = [];
    socket.addEventListener("message", ({ data }) => {
      const event = JSON.parse(data);
      if (event.id) { const item = this.pending.get(event.id); if (item) { clearTimeout(item.timer); this.pending.delete(event.id); event.error ? item.reject(Error(event.error.message)) : item.resolve(event.result); } }
      if (event.method === "Page.frameNavigated" && !event.params.frame.parentId) this.documentPath = new URL(event.params.frame.url).pathname;
      if (event.method === "Runtime.exceptionThrown") { const message = event.params.exceptionDetails.exception?.description || event.params.exceptionDetails.text; this.errors.push(message); this.errorEvents.push({ message, document_path: this.documentPath, recorded_at: new Date().toISOString() }); }
      if (event.method === "Network.requestWillBeSent" && ["Fetch", "XHR"].includes(event.params.type)) {
        const url = new URL(event.params.request.url);
        if (url.pathname === "/common/basetool/getCaptcha") {
          this.captchaVerify = Object.entries(event.params.request.headers || {}).find(([name]) => name.toLowerCase() === "apiverify")?.[1] || this.captchaVerify;
        }
        this.network.set(event.params.requestId, { method: event.params.request.method, path: url.pathname, document_path: this.documentPath, origin: url.origin,
          ...(url.pathname === "/common/basetool/getCaptcha" ? { captcha_types: url.searchParams.getAll("type"), has_timestamp: url.searchParams.has("_t") } : {}) });
      }
      if (event.method === "Network.responseReceived" && this.network.has(event.params.requestId)) Object.assign(this.network.get(event.params.requestId), { status: event.params.response.status, mime_type: event.params.response.mimeType });
      if (event.method === "Network.loadingFinished" && this.network.has(event.params.requestId)) {
        const info = this.network.get(event.params.requestId); this.network.delete(event.params.requestId);
        if (info.origin === base && (info.mime_type || "").includes("json")) {
          const task = this.send("Network.getResponseBody", { requestId: event.params.requestId }).then(result => {
            const body = JSON.parse(result.base64Encoded ? Buffer.from(result.body, "base64").toString("utf8") : result.body);
            // Only envelope status/shape is retained. Never record response data,
            // auth headers, cookies, token fields, query strings, or request bodies.
            const data = body.data, summary = { ...info, code: body.code ?? null, message: typeof (body.message ?? body.msg) === "string" ? String(body.message ?? body.msg).slice(0, 500) : null,
              has_data: Object.hasOwn(body, "data"), data_type: data === null ? "null" : Array.isArray(data) ? "array" : typeof data };
            this.apiResponses.push(summary);
          }).catch(error => this.apiResponses.push({ ...info, response_unavailable: error.message }));
          this.responseTasks.add(task); task.finally(() => this.responseTasks.delete(task));
        }
      }
      if (event.method === "Network.responseReceived" && new URL(event.params.response.url).hostname === "kf.szputy.com" && event.params.response.remoteIPAddress) this.serverAddresses.add(event.params.response.remoteIPAddress);
      if (event.method === "Fetch.requestPaused") {
        const { requestId, request } = event.params, denied = !["GET", "HEAD", "OPTIONS"].includes(request.method);
        if (denied) this.blocked.push({ method: request.method, path: new URL(request.url).pathname });
        this.send(denied ? "Fetch.failRequest" : "Fetch.continueRequest", denied ? { requestId, errorReason: "BlockedByClient" } : { requestId }).catch(error => this.interceptorErrors.push(error.message));
      }
    });
    socket.addEventListener("close", () => { for (const item of this.pending.values()) { clearTimeout(item.timer); item.reject(Error("QA Chrome closed")); } this.pending.clear(); });
  }
  send(method, params = {}) { return new Promise((resolve, reject) => { const id = ++this.sequence, timer = setTimeout(() => { this.pending.delete(id); reject(Error("Chrome timeout: " + method)); }, 30000); this.pending.set(id, { resolve, reject, timer }); this.socket.send(JSON.stringify({ id, method, params })); }); }
  async eval(expression) { const value = await this.send("Runtime.evaluate", { expression, awaitPromise: true, returnByValue: true }); if (value.exceptionDetails) throw Error(value.exceptionDetails.exception?.description || value.exceptionDetails.text); return value.result.value; }
  async until(expression, timeout = 30000) { const deadline = Date.now() + timeout; while (Date.now() < deadline) { if (await this.eval(expression)) return; await wait(100); } throw Error("Page condition timed out: " + expression); }
  async api(path) {
    assert.ok(path.startsWith("/hrm/"), "This helper only reads HRM state");
    return this.eval(`(async()=>{const response=await fetch(${JSON.stringify(apiPrefix + path)},{method:'GET',credentials:'include',cache:'no-store',headers:{'X-Requested-With':'XMLHttpRequest'}});const data=await response.json();if(!response.ok||(data.code!=null&&Number(data.code)!==0))throw Error(data.message||data.msg||'Read-only API failed');return data.data!==undefined?data.data:data;})()`);
  }
  async probeCaptcha() {
    assert.ok(this.captchaVerify, "The image captcha request verification header was not observed");
    return this.eval(`(async()=>{const results=[];for(const query of ['?type=image','']){const response=await fetch('/common/basetool/getCaptcha'+query,{method:'GET',credentials:'include',cache:'no-store',headers:{'apiverify':${JSON.stringify(this.captchaVerify)},'X-Requested-With':'XMLHttpRequest'}});const text=await response.text();let body;try{body=JSON.parse(text);}catch{results.push({query_variant:query?'single-image-type':'no-client-query',status:response.status,content_type:response.headers.get('content-type'),body_format:'non-json'});continue;}results.push({query_variant:query?'single-image-type':'no-client-query',status:response.status,code:body.code,message:body.message||body.msg||'',data_type:body.data===null?'null':typeof body.data,show:typeof body.data?.show==='boolean'?body.data.show:null,image_present:!!body.data?.img});}return results;})()`);
  }
  async shot(label) { const { data } = await this.send("Page.captureScreenshot", { format: "png", captureBeyondViewport: false }); const filename = `${run}-${label}.png`; await writeFile(join(output, filename), Buffer.from(data, "base64")); report.screenshots.push(filename); }
  async open(rule) {
    await this.eval(`(()=>{const toast=document.querySelector('#toast');if(toast){toast.textContent='';toast.classList.remove('show');}const search=document.querySelector('#view input[type=search]');if(!search)throw Error('Missing R6 rule table search');search.value=${JSON.stringify(rule.id)};search.dispatchEvent(new Event('input',{bubbles:true}));})()`);
    await this.until("document.querySelectorAll('#view tbody tr').length===1");
    await this.eval(`(()=>{const row=document.querySelector('#view tbody tr');const action=[...row.querySelectorAll('button')].find(button=>['编辑 / 新版本','查看'].includes(button.textContent.trim()));if(!action)throw Error('Missing rule editor action');action.click();})()`);
    await this.until("!!document.querySelector('[data-attendance-rule-editor]')||!!document.querySelector('#toast.show')?.textContent");
    const state = await this.eval("({opened:!!document.querySelector('[data-attendance-rule-editor]'),toast:document.querySelector('#toast.show')?.textContent||''})");
    assert.equal(state.toast, "", rule.name + " raised an editor error"); assert.equal(state.opened, true, rule.name + " failed to open");
    const actual = await this.eval(`(()=>{const form=document.querySelector('[data-attendance-rule-editor]');const count=title=>[...form.querySelectorAll('section')].find(section=>section.querySelector(':scope > h3')?.textContent===title)?.querySelectorAll('input[type=checkbox]:checked').length;const values=Object.fromEntries(${JSON.stringify(minuteKeys)}.map(key=>[key,[...form.querySelectorAll('[name="'+key+'"]')].map(input=>input.value)]));const windowCount=values.in_before_minutes.length;return {name:form.querySelector('[name=name]').value,employees:count('适用人员（优先于部门）'),departments:count('适用本地部门'),windows:Array.from({length:windowCount},(_,index)=>Object.fromEntries(${JSON.stringify(minuteKeys)}.map(key=>[key,values[key][index]])))};})()`);
    assert.equal(actual.name, rule.name);
    const employeeIDs = new Set(rows(rule.scope?.employee_ids).map(String)), departmentIDs = new Set(rows(rule.scope?.department_ids).map(String));
    assert.equal(actual.employees, rows(options.employees).filter(employee => employeeIDs.has(String(employee.id))).length);
    assert.equal(actual.departments, rows(options.departments).filter(department => departmentIDs.has(String(department.id))).length);
    assert.deepEqual(actual.windows, expectedMinuteWindows(rule), "The R6 minute window display must retain the original configuration");
    return { id: rule.id, name: rule.name, status: rule.status, opened: true,
      employee_scope_null: rule.scope?.employee_ids === null, department_scope_null: rule.scope?.department_ids === null,
      employees_selected: actual.employees, departments_selected: actual.departments, scope_preserved: true,
      minute_window_groups: actual.windows.length, minute_windows_preserved: true };
  }
  async close() { await this.eval("document.querySelector('dialog [aria-label=关闭]').click()"); await this.until("!document.querySelector('dialog')"); }
}

async function launch() {
  profile = await mkdtemp(join(tmpdir(), "attendance-ubuntu-edit-"));
  chrome = spawn(process.env.CHROME_PATH || "C:/Program Files/Google/Chrome/Application/chrome.exe", ["--headless=new", "--disable-gpu", "--no-first-run", "--no-default-browser-check", "--ignore-certificate-errors", "--remote-debugging-port=0", "--remote-allow-origins=*", "--host-resolver-rules=MAP kf.szputy.com " + mappedIP, "--no-proxy-server", "--user-data-dir=" + profile, "about:blank"], { stdio: "ignore", windowsHide: true });
  let port; for (let i = 0; i < 100 && !port; i++) { try { port = (await readFile(join(profile, "DevToolsActivePort"), "utf8")).split("\n")[0]; } catch { await wait(100); } }
  assert.ok(port, "Chrome startup failed");
  const targets = await (await fetch(`http://127.0.0.1:${port}/json/list`, { signal: AbortSignal.timeout(5000) })).json();
  const socket = new WebSocket(targets.find(target => target.type === "page").webSocketDebuggerUrl);
  await new Promise((resolve, reject) => { const timer = setTimeout(() => reject(Error("CDP connection failed")), 10000); socket.addEventListener("open", () => { clearTimeout(timer); resolve(); }, { once: true }); });
  page = new Page(socket); await page.send("Runtime.enable"); await page.send("Page.enable"); await page.send("Network.enable");
  // Host login/renewal is unaffected. Every plugin business write is blocked.
  await page.send("Fetch.enable", { patterns: [{ urlPattern: "*/plugins/biz-suite/api/admin/*", requestStage: "Request" }] });
  await page.send("Emulation.setDeviceMetricsOverride", { width: 1440, height: 1050, deviceScaleFactor: 1, mobile: false });
  await page.send("Page.navigate", { url: base + adminPrefix + "hrm/attendance-rules" });
  await page.until("!!document.querySelector('input[type=password]')"); await wait(500);
  await page.eval(`(()=>{const form=document.querySelector('input[type=password]').closest('form');for(const [selector,value]of [['input[type=text]',${JSON.stringify(credentials.username)}],['input[type=password]',${JSON.stringify(credentials.password)}]]){const input=form.querySelector(selector);input.value=value;input.dispatchEvent(new Event('input',{bubbles:true}));}const remember=form.querySelector('input[type=checkbox]');if(remember?.checked)remember.click();form.querySelector('button[type=submit]').click();})()`);
  await page.until("!!document.querySelector('#view tbody tr')");
}
async function step(name, action) {
  const started = Date.now();
  try { await action(); report.checks.push({ name, passed: true, ms: Date.now() - started }); console.log("PASS " + name); }
  catch (error) { report.checks.push({ name, passed: false, error: error.message }); throw error; }
  finally { await writeFile(reportPath, JSON.stringify(report, null, 2)); }
}

try {
  await step("real-public-IP-login-and-correct-hotfix-bundle", async () => {
    await launch(); report.loaded_scripts = await page.eval("[...document.scripts].map(script=>script.src).filter(Boolean)");
    assert.ok(report.loaded_scripts.some(url => url.endsWith("/" + expectedAsset)), "The actual public site must serve the exact R6 hotfix asset");
    report.remote_addresses = [...page.serverAddresses]; assert.ok(report.remote_addresses.includes(mappedIP), "The browser must reach the actual public IP");
  });
  if (process.env.ATTENDANCE_QA_CAPTCHA_PROBE === "1") await step("image-captcha-query-diagnostics-with-in-memory-verification", async () => { report.captcha_probes = await page.probeCaptcha(); });
  await step("snapshot-real-Ubuntu-rules-and-open-specific-reported-rule", async () => {
    before = rows(await page.api("/hrm/attendance/v2/rules")); options = await page.api("/hrm/options"); assert.ok(before.length > 0);
    report.before_hash = hash(before); report.total_rules = before.length; report.rule_ids = before.map(rule => rule.id);
    report.null_employee_scopes = before.filter(rule => rule.scope?.employee_ids === null).length;
    report.null_department_scopes = before.filter(rule => rule.scope?.department_ids === null).length;
    const target = before.find(rule => rule.name === "二部  谢文冠") || before.find(rule => rule.name.replace(/\s+/g, " ").trim() === "二部 谢文冠");
    assert.ok(target, "The exact reported rule must be present");
    report.reported_rule = await page.open(target); report.editors.push(report.reported_rule); await page.shot("reported-erbu-xiewenguan-editor"); await page.close();
  });
  await step("every-public-rule-opens-without-any-business-save", async () => {
    for (const rule of before) {
      if (rule.id === report.reported_rule.id) continue;
      report.editors.push(await page.open(rule)); await page.close();
      if (report.editors.length % 20 === 0) { console.log(`Verified ${report.editors.length}/${before.length} public editors`); await writeFile(reportPath, JSON.stringify(report, null, 2)); }
    }
    assert.equal(report.editors.length, before.length);
  });
  await step("all-rules-unchanged-and-no-JavaScript-or-write-errors", async () => {
    report.after_hash = hash(rows(await page.api("/hrm/attendance/v2/rules"))); report.rules_unchanged = report.after_hash === report.before_hash;
    await Promise.all([...page.responseTasks]);
    const loginCaptcha = page.apiResponses.find(response => response.path === "/common/basetool/getCaptcha" && response.document_path === "/suxinweb/login");
    assert.ok(loginCaptcha, "The normal login image-captcha response must be observed");
    assert.equal(loginCaptcha.status, 200); assert.equal(Number(loginCaptcha.code), 0, "Normal image captcha must return success");
    assert.deepEqual(loginCaptcha.captcha_types, ["image"]); assert.equal(loginCaptcha.data_type, "object");
    report.login_captcha = loginCaptcha;
    assert.equal(report.after_hash, report.before_hash); assert.deepEqual(page.errors, []); assert.deepEqual(page.blocked, []); assert.deepEqual(page.interceptorErrors, []);
  });
  report.passed = true;
} catch (error) {
  report.passed = false; report.error = error.message; process.exitCode = 1; console.error("FAIL public attendance edit: " + error.message);
  try { await page?.shot("failure"); } catch {}
} finally {
  if (page && before && !report.after_hash) {
    try { report.after_hash = hash(rows(await page.api("/hrm/attendance/v2/rules"))); report.rules_unchanged = report.after_hash === report.before_hash; if (!report.rules_unchanged) process.exitCode = 1; }
    catch (error) { report.final_snapshot_error = error.message; process.exitCode = 1; }
  }
  if (page) await Promise.all([...page.responseTasks]);
  report.js_errors = page?.errors || []; report.js_error_events = page?.errorEvents || []; report.api_response_summaries = page?.apiResponses || [];
  report.blocked_business_writes = page?.blocked || []; report.remote_addresses = [...(page?.serverAddresses || [])];
  try { page?.socket.close(); chrome?.kill(); } catch {}
  if (profile) {
    const parent = resolve(tmpdir()), target = resolve(profile), child = relative(parent, target);
    assert.ok(child && !child.startsWith("..") && !isAbsolute(child) && target.includes("attendance-ubuntu-edit-"));
    await wait(500); await rm(target, { recursive: true, force: true, maxRetries: 5, retryDelay: 300 }).catch(() => {});
  }
  report.cleanup = { browser_stopped: true, business_records_unchanged: report.rules_unchanged === true, temporary_account: "Managed and cleaned up by the deployment caller" };
  if (process.exitCode) report.passed = false;
  report.finishedAt = new Date().toISOString(); await writeFile(reportPath, JSON.stringify(report, null, 2)); console.log("Report: " + reportPath);
}
