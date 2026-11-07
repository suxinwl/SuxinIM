// Read-only public verification. Never submit a rule or create a test account.
// The deployment caller supplies temporary credentials and cleans up that account.
import assert from "node:assert/strict";
import { readFile, writeFile, mkdir, mkdtemp, rm } from "node:fs/promises";
import { createHash } from "node:crypto";
import { spawn } from "node:child_process";
import { resolve, join, basename, relative, isAbsolute } from "node:path";
import { tmpdir } from "node:os";

const root = resolve(import.meta.dirname, ".."), output = join(root, "runtime/browser-tests");
const readJSON = async path => JSON.parse((await readFile(path, "utf8")).replace(/^\uFEFF/, ""));
const credentials = await readJSON(join(root, "runtime/ubuntu-attendance-qa-credentials.json"));
const release = await readJSON(join(root, "runtime/attendance-work-schedule-package.json"));
assert.ok(typeof credentials.username === "string" && credentials.username && typeof credentials.password === "string" && credentials.password, "Temporary QA credentials are required");
assert.ok(typeof release.asset === "string" && release.asset.endsWith(".js"), "The built package must provide its JavaScript asset");
const expectedAsset = basename(release.asset.replaceAll("\\", "/"));
const base = "https://kf.szputy.com:9880", publicIP = "61.144.183.217", admin = "/plugins/biz-suite/admin/", api = "/plugins/biz-suite/api/admin";
const run = "attendance-work-schedule-public-" + Date.now(), reportPath = join(output, run + "-report.json");
const report = { run, base, expectedAsset, started_at: new Date().toISOString(), business_writes_allowed: false,
  transport: "Chrome DNS pinned to the public IP; current temporary HTTPS certificate accepted", checks: [], screenshots: [], editors: [] };
const pause = ms => new Promise(resolve => setTimeout(resolve, ms));
const rows = value => Array.isArray(value) ? value : value?.items || value?.rows || value?.list || [];
const stable = value => Array.isArray(value) ? value.map(stable) : value && typeof value === "object" ? Object.fromEntries(Object.keys(value).sort().map(key => [key, stable(value[key])])) : value;
const hash = records => createHash("sha256").update(JSON.stringify(stable([...records].sort((a, b) => String(a.id).localeCompare(String(b.id)))))).digest("hex");
function sourceOf(rule) { if (typeof rule.source !== "string") return rule.source || {}; try { return JSON.parse(rule.source); } catch { return {}; } }
const reference = rule => !!sourceOf(rule).snapshot_id;
const historical = rule => reference(rule) || !["draft", "published"].includes(rule.status);
let chrome, profile, page, before;
await mkdir(output, { recursive: true });

class Page {
  constructor(socket) {
    this.socket = socket; this.nextID = 0; this.pending = new Map(); this.errors = []; this.blocked = []; this.interceptionErrors = []; this.addresses = new Set();
    socket.addEventListener("message", ({ data }) => {
      const event = JSON.parse(data);
      if (event.id) { const item = this.pending.get(event.id); if (item) { clearTimeout(item.timer); this.pending.delete(event.id); event.error ? item.reject(Error(event.error.message)) : item.resolve(event.result); } }
      if (event.method === "Runtime.exceptionThrown") this.errors.push(event.params.exceptionDetails.exception?.description || event.params.exceptionDetails.text);
      if (event.method === "Network.responseReceived" && new URL(event.params.response.url).hostname === "kf.szputy.com" && event.params.response.remoteIPAddress) this.addresses.add(event.params.response.remoteIPAddress);
      if (event.method === "Fetch.requestPaused") {
        const { requestId, request } = event.params, denied = !["GET", "HEAD", "OPTIONS"].includes(request.method);
        if (denied) this.blocked.push({ method: request.method, path: new URL(request.url).pathname });
        this.send(denied ? "Fetch.failRequest" : "Fetch.continueRequest", denied ? { requestId, errorReason: "BlockedByClient" } : { requestId }).catch(error => this.interceptionErrors.push(error.message));
      }
    });
    socket.addEventListener("close", () => { for (const item of this.pending.values()) { clearTimeout(item.timer); item.reject(Error("QA browser closed")); } this.pending.clear(); });
  }
  send(method, params = {}) { return new Promise((resolve, reject) => { const id = ++this.nextID, timer = setTimeout(() => { this.pending.delete(id); reject(Error("CDP timeout: " + method)); }, 30000); this.pending.set(id, { resolve, reject, timer }); this.socket.send(JSON.stringify({ id, method, params })); }); }
  async eval(expression) { const result = await this.send("Runtime.evaluate", { expression, awaitPromise: true, returnByValue: true }); if (result.exceptionDetails) throw Error(result.exceptionDetails.exception?.description || result.exceptionDetails.text); return result.result.value; }
  async until(expression) { const deadline = Date.now() + 30000; while (Date.now() < deadline) { if (await this.eval(expression)) return; await pause(100); } throw Error("Browser condition timed out: " + expression); }
  async read(path) {
    assert.ok(path.startsWith("/hrm/"), "Only HRM read endpoints are allowed");
    return this.eval(`(async()=>{const response=await fetch(${JSON.stringify(api + path)},{method:'GET',credentials:'include',cache:'no-store',headers:{'X-Requested-With':'XMLHttpRequest'}});const body=await response.json();if(!response.ok||(body.code!=null&&Number(body.code)!==0))throw Error(body.message||body.msg||'HRM read failed');return body.data??body;})()`);
  }
  async screenshot(label) { const result = await this.send("Page.captureScreenshot", { format: "png", captureBeyondViewport: false }); const filename = run + "-" + label + ".png"; await writeFile(join(output, filename), Buffer.from(result.data, "base64")); report.screenshots.push(filename); }
  async open(rule) {
    const section = historical(rule) ? "[data-attendance-rule-history]" : "[data-attendance-current-rules]";
    await this.eval(`(()=>{const container=document.querySelector(${JSON.stringify(section)});if(!container)throw Error('Expected rule section is missing');if(container.tagName==='DETAILS')container.open=true;const search=container.querySelector('input[type=search]');search.value=${JSON.stringify(rule.id)};search.dispatchEvent(new Event('input',{bubbles:true}));})()`);
    await this.until(`document.querySelectorAll('${section} tbody tr').length===1`);
    const actions = await this.eval(`[...document.querySelectorAll('${section} tbody tr button')].map(button=>button.textContent.trim())`);
    if (historical(rule)) assert.deepEqual(actions, ["查看"], "Historical and WeCom reference rules must only expose View");
    await this.eval(`document.querySelector('${section} tbody tr button').click()`);
    await this.until("!!document.querySelector('[data-attendance-rule-editor]')");
    return this.eval(`(()=>{const f=document.querySelector('[data-attendance-rule-editor]');return {name:f.querySelector('[name=name]').value,category:f.querySelector('[name=category]').value,scope_mode:f.querySelector('[name=scope_mode]').value,people:[...f.querySelectorAll('[data-scope-selected=employees] [data-selected-id]')].map(e=>e.dataset.selectedId),departments:[...f.querySelectorAll('[data-scope-selected=departments] [data-selected-id]')].map(e=>e.dataset.selectedId),include_children:f.querySelector('[name=scope_include_children]').checked,legacy_default:f.querySelector('[name=scope_preserve_default_extras]').checked,fixed_visible:!f.querySelector('[data-work-schedule]').hidden,custom_hidden:f.querySelector('[data-custom-schedule]').hidden,schedule_control_hidden:f.querySelector('[name=schedule_mode]').closest('label').hidden,week:[...f.querySelectorAll('[data-work-schedule] .work-schedule-day')].map(e=>e.textContent),enabled_controls:f.querySelectorAll('input:not(:disabled),select:not(:disabled),button:not(:disabled)').length,scope_text:f.querySelector('[data-attendance-scope]').textContent};})()`);
  }
  async close() { await this.eval("document.querySelector('dialog [aria-label=关闭]').click()"); await this.until("!document.querySelector('dialog')"); }
}

async function launch() {
  profile = await mkdtemp(join(tmpdir(), "attendance-work-schedule-public-"));
  chrome = spawn(process.env.CHROME_PATH || "C:/Program Files/Google/Chrome/Application/chrome.exe", ["--headless=new", "--disable-gpu", "--no-first-run", "--no-default-browser-check", "--ignore-certificate-errors", "--remote-debugging-port=0", "--remote-allow-origins=*", "--host-resolver-rules=MAP kf.szputy.com " + publicIP, "--no-proxy-server", "--user-data-dir=" + profile, "about:blank"], { stdio: "ignore", windowsHide: true });
  let port; for (let i = 0; i < 100 && !port; i++) { try { port = (await readFile(join(profile, "DevToolsActivePort"), "utf8")).split("\n")[0]; } catch { await pause(100); } }
  assert.ok(port, "Chrome did not start");
  const targets = await (await fetch(`http://127.0.0.1:${port}/json/list`, { signal: AbortSignal.timeout(5000) })).json();
  const socket = new WebSocket(targets.find(target => target.type === "page").webSocketDebuggerUrl);
  await new Promise((resolve, reject) => { const timer = setTimeout(() => reject(Error("CDP connection failed")), 10000); socket.addEventListener("open", () => { clearTimeout(timer); resolve(); }, { once: true }); });
  page = new Page(socket); await page.send("Runtime.enable"); await page.send("Page.enable"); await page.send("Network.enable");
  // Only host authentication may write. All plugin business mutations are blocked.
  await page.send("Fetch.enable", { patterns: [{ urlPattern: "*/plugins/biz-suite/api/admin/*", requestStage: "Request" }] });
  await page.send("Emulation.setDeviceMetricsOverride", { width: 1440, height: 1050, deviceScaleFactor: 1, mobile: false });
  await page.send("Page.navigate", { url: base + admin + "hrm/attendance-rules" });
  await page.until("!!document.querySelector('input[type=password]')"); await pause(500);
  await page.eval(`(()=>{const form=document.querySelector('input[type=password]').closest('form');for(const [selector,value]of [['input[type=text]',${JSON.stringify(credentials.username)}],['input[type=password]',${JSON.stringify(credentials.password)}]]){const input=form.querySelector(selector);input.value=value;input.dispatchEvent(new Event('input',{bubbles:true}));}const remember=form.querySelector('input[type=checkbox]');if(remember?.checked)remember.click();form.querySelector('button[type=submit]').click();})()`);
  await page.until("!!document.querySelector('[data-attendance-current-rules]')");
}
async function step(name, action) {
  const started = Date.now();
  try { await action(); report.checks.push({ name, passed: true, ms: Date.now() - started }); console.log("PASS " + name); }
  catch (error) { report.checks.push({ name, passed: false, error: error.message }); throw error; }
  finally { await writeFile(reportPath, JSON.stringify(report, null, 2)); }
}

try {
  await step("public-HTTPS-login-and-exact-built-asset", async () => {
    await launch(); report.loaded_scripts = await page.eval("[...document.scripts].map(script=>script.src).filter(Boolean)");
    assert.ok(report.loaded_scripts.some(url => new URL(url).pathname.endsWith("/" + expectedAsset)), "The public page must serve this build's exact asset");
    assert.ok(page.addresses.has(publicIP), "The browser must reach the public server, not local DNS");
  });
  await step("read-current-and-historical-rule-inventory", async () => {
    before = rows(await page.read("/hrm/attendance/v2/rules")); assert.ok(before.length, "Rules must be present");
    report.before_hash = hash(before); report.total_rules = before.length; report.current_rules = before.filter(rule => !historical(rule)).length; report.reference_rules = before.filter(reference).length; report.historical_rules = before.filter(historical).length;
    assert.ok(report.current_rules && report.reference_rules, "Current local rules and retained WeCom references must both exist");
    assert.equal(await page.eval("document.querySelector('[data-attendance-rule-history]').open"), false);
  });
  await step("every-rule-opens-with-preserved-scope-and-correct-week-policy", async () => {
    let employeeCount = 0, staffCount = 0, screenshots = new Set();
    for (const rule of before) {
      const actual = await page.open(rule), people = rows(rule.scope?.employee_ids).map(String), departments = rows(rule.scope?.department_ids).map(String);
      assert.equal(actual.name, rule.name); assert.deepEqual(actual.people, people); assert.deepEqual(actual.departments, departments);
      const expectedMode = rule.scope?.is_default ? "default" : people.length && departments.length ? "union" : departments.length ? "departments" : "employees";
      assert.equal(actual.scope_mode, expectedMode); assert.equal(actual.include_children, rule.scope?.include_children ?? true);
      if (rule.scope?.is_default && (people.length || departments.length)) assert.equal(actual.legacy_default, true, "Default priority selections must not be discarded");
      if (historical(rule)) assert.equal(actual.enabled_controls, 0, "Historical reference editor must be completely read-only");
      if (historical(rule) && !rule.config?.work_schedule) {
        assert.equal(actual.fixed_visible, false); assert.equal(actual.custom_hidden, false); assert.equal(actual.schedule_control_hidden, false);
        const savedCycle=rule.config?.cycle||{};
        const schedule=await page.eval("(()=>{const form=document.querySelector('[data-attendance-rule-editor]');return {heading:form.querySelector('[data-custom-schedule] h3').textContent,mode:form.querySelector('[name=schedule_mode]').value,anchor:form.querySelector('[name=anchor]').value,days:[...form.querySelectorAll('[data-attendance-cycle] select')].map(e=>e.value),weekdays:Array.from({length:7},(_,i)=>({shift_id:form.querySelector('[name=weekly_shift_'+i+']').value,category:form.querySelector('[name=weekly_category_'+i+']').value,scheduled_work:form.querySelector('[name=weekly_scheduled_'+i+']').checked}))};})()");
        assert.equal(schedule.heading,'历史作息配置');assert.equal(schedule.mode,savedCycle.mode==='weekly'?'weekly':'cycle');
        if(savedCycle.mode==='weekly')assert.deepEqual(schedule.weekdays,Array.from({length:7},(_,i)=>({shift_id:rows(savedCycle.weekdays)[i]?.shift_id||'',category:rows(savedCycle.weekdays)[i]?.category||'',scheduled_work:rows(savedCycle.weekdays)[i]?.category==='workday'||!!rows(savedCycle.weekdays)[i]?.scheduled_work})));
        else{assert.equal(schedule.anchor,savedCycle.anchor||'');assert.deepEqual(schedule.days,rows(savedCycle.days));}
        if(!screenshots.has('historical-original')){await page.eval("document.querySelector('[data-custom-schedule]').scrollIntoView({block:'start'})");await page.screenshot('historical-original-schedule');screenshots.add('historical-original');}
      } else if (["员工", "生产职员"].includes(actual.category)) {
        assert.equal(actual.fixed_visible, true); assert.equal(actual.custom_hidden, true); assert.equal(actual.schedule_control_hidden, true);
        const regular = actual.category === "员工" ? 5 : 6; assert.equal(actual.week.length, 7);
        actual.week.forEach((label, index) => assert.ok(label.includes(index < regular ? "正常出勤" : "休息 · 出勤计加班"), "Week policy must match the personnel category"));
        actual.category === "员工" ? employeeCount++ : staffCount++;
        if (!historical(rule) && !screenshots.has(actual.category)) { await page.eval("document.querySelector('[data-work-schedule]').scrollIntoView({block:'start'})"); await page.screenshot(actual.category === "员工" ? "production-employee-fixed-week" : "production-staff-fixed-week"); screenshots.add(actual.category); }
      }
      report.editors.push({ id: rule.id, category: actual.category, reference: reference(rule), read_only: historical(rule), scope_mode: actual.scope_mode, employee_count: people.length, department_count: departments.length, preserved: true });
      await page.close();
    }
    assert.equal(report.editors.length, before.length); assert.ok(employeeCount && staffCount, "Both production personnel categories must be verified");
  });
  await step("scope-search-retains-hidden-selections-and-after-sales-controls-are-available", async () => {
    const scoped = before.find(rule => !historical(rule) && rows(rule.scope?.employee_ids).length) || before.find(rule => !historical(rule));
    const original = await page.open(scoped);
    await page.eval("document.querySelector('[data-attendance-scope]').scrollIntoView({block:'start'});var search=document.querySelector('[name=scope_employee_search]');search.value='不存在的验收人员_'+Date.now();search.dispatchEvent(new Event('input'))");
    assert.deepEqual(await page.eval("[...document.querySelectorAll('[data-scope-selected=employees] [data-selected-id]')].map(e=>e.dataset.selectedId)"), original.people);
    assert.equal(await page.eval("document.querySelectorAll('[data-scope-results=employees] [data-scope-id]').length"), 0);
    await page.screenshot("scope-hidden-selections-preserved"); await page.close();
    const staff = before.find(rule => !historical(rule) && rule.config?.work_schedule?.mode === "after_sales"); assert.ok(staff, "The configured after-sales draft must be present");
    assert.equal(staff.config.category,'生产职员');assert.equal(staff.config.work_schedule.duty_expected_members,3,'The verified after-sales group has three members');
    assert.equal(new Set(rows(staff.scope?.employee_ids).map(String)).size,3,'After-sales scope must explicitly contain the three confirmed employees');
    assert.equal(rows(staff.scope?.department_ids).length,0,'After-sales scope must not silently include an entire department');assert.equal(!!staff.scope?.is_default,false,'After-sales must not be a company-wide fallback');
    const directory=await page.read('/hrm/options'),staffIDs=new Set(rows(staff.scope.employee_ids).map(String));
    const members=rows(directory.employees).filter(employee=>staffIDs.has(String(employee.id))).map(employee=>({name:String(employee.name||'').trim(),employee_no:String(employee.employee_no||'').trim()})).sort((a,b)=>a.employee_no.localeCompare(b.employee_no));
    assert.deepEqual(members,[{name:'王转脱',employee_no:'2485'},{name:'普贴售后',employee_no:'3720'},{name:'安乐瑶',employee_no:'3958'}],'The scope must resolve to the user-confirmed after-sales employees');
    await page.open(staff);
    assert.equal(await page.eval("document.querySelector('[name=production_staff_schedule]').value"),'after_sales');
    assert.equal(await page.eval("document.querySelectorAll('[data-scope-selected=employees] [data-selected-id]').length"),3);
    await page.eval("document.querySelector('[data-work-schedule]').scrollIntoView({block:'start'})");
    assert.equal(await page.eval("document.querySelector('[name=duty_group]').closest('label').hidden"), false);
    const dutyCount = await page.eval("(()=>{const input=document.querySelector('[name=duty_expected_members]');return input?{visible:!input.closest('label').hidden,disabled:input.disabled,type:input.type,value:Number(input.value),min:input.min,max:input.max,step:input.step}:null})()");
    assert.deepEqual(dutyCount, {visible:true,disabled:false,type:'number',value:staff.config?.work_schedule?.duty_expected_members||3,min:'1',max:'1000',step:'1'}, 'After-sales group size must be visible, bounded and preserve its configured value');
    assert.ok(await page.eval("document.querySelectorAll('[name=duty_shift_ids]').length>0"));
    assert.match(await page.eval("document.querySelector('[data-aftersales-duty]').textContent"), /18:00.*20:00.*复核/s);
    await page.screenshot("after-sales-confirmed-three-member-draft"); await page.close();
    report.after_sales_group_size_verified = true;
  });
  await step("zero-business-writes-no-runtime-errors-and-unchanged-rule-hash", async () => {
    report.after_hash = hash(rows(await page.read("/hrm/attendance/v2/rules"))); report.rules_unchanged = report.before_hash === report.after_hash;
    assert.equal(report.rules_unchanged, true); assert.deepEqual(page.errors, []); assert.deepEqual(page.blocked, []); assert.deepEqual(page.interceptionErrors, []);
  });
  report.passed = true;
} catch (error) {
  report.passed = false; report.error = error.message; process.exitCode = 1; console.error("FAIL public work schedule: " + error.message);
  try { await page?.screenshot("failure"); } catch {}
} finally {
  if (page && before && !report.after_hash) { try { report.after_hash = hash(rows(await page.read("/hrm/attendance/v2/rules"))); report.rules_unchanged = report.before_hash === report.after_hash; if (!report.rules_unchanged) process.exitCode = 1; } catch { report.final_snapshot_failed = true; process.exitCode = 1; } }
  report.js_errors = page?.errors || []; report.blocked_business_writes = page?.blocked || []; report.remote_addresses = [...(page?.addresses || [])];
  try { page?.socket.close(); chrome?.kill(); } catch {}
  if (profile) { const parent = resolve(tmpdir()), target = resolve(profile), child = relative(parent, target); assert.ok(child && !child.startsWith("..") && !isAbsolute(child) && target.includes("attendance-work-schedule-public-")); await pause(500); await rm(target, { recursive: true, force: true, maxRetries: 5, retryDelay: 300 }).catch(() => {}); }
  report.cleanup = { browser_stopped: true, business_records_unchanged: report.rules_unchanged === true, temporary_account: "Created and cleaned up by deployment caller" };
  if (process.exitCode) report.passed = false;
  report.finished_at = new Date().toISOString(); await writeFile(reportPath, JSON.stringify(report, null, 2)); console.log("Report: " + reportPath);
}
