// Real Chrome + local host/MySQL attendance-v2 acceptance. Nothing is mocked.
// Run only after deploying the tested build. Every mutation targets one new QA
// employee/account/rule; no WeCom endpoint or real monthly publication is used.
import assert from "node:assert/strict";
import { readFile, writeFile, mkdir, mkdtemp, rm } from "node:fs/promises";
import { existsSync } from "node:fs";
import { resolve, join, relative, isAbsolute } from "node:path";
import { tmpdir } from "node:os";
import { spawn, execFileSync } from "node:child_process";
import { createRequire } from "node:module";
import { inflateRawSync } from "node:zlib";
import { createHash } from "node:crypto";
import { createTestAdmin, cleanupRun } from "./cleanup-biz-suite-test.mjs";

const root = resolve(import.meta.dirname, "..");
const require = createRequire(join(root, "web/package.json"));
const { parse } = require("yaml");
const access = JSON.parse((await readFile(join(root, "runtime/local-access.json"), "utf8")).replace(/^\uFEFF/, ""));
const mappedHTTPS = process.env.ATTENDANCE_QA_HTTPS === "1";
const scheduleOnly = process.env.WECOM_QA_SCHEDULE_ONLY === "1";
// HTTPS tests keep the public SNI hostname, while Chrome resolves that hostname
// strictly to this machine. This does not change DNS or contact a remote host.
const base = mappedHTTPS ? "https://kf.szputy.com:9880" : new URL(access.url).origin;
assert.ok(mappedHTTPS || ["localhost", "127.0.0.1"].includes(new URL(base).hostname), "Only the local host may be tested");
assert.ok(["9880", "19880"].includes(new URL(base).port));
assert.equal(access.database, "kaoqin_dev");
const config = parse(await readFile(join(root, "manifest/config/config.yaml"), "utf8")).database.default;
assert.equal(config.name, "kaoqin_dev");
assert.ok(["localhost", "127.0.0.1"].includes(config.host));
assert.equal(config.prefix, "gf_");
const run = "QA" + Date.now(), employeeID = run + "-attendance", prefix = "/plugins/biz-suite/admin/";
const date = process.env.ATTENDANCE_QA_DATE || "2026-10-12";
const offsetDate = days => new Date(Date.parse(date + "T00:00:00Z") + days * 86400000).toISOString().slice(0,10);
const weekendDate = offsetDate(5), exceptionDate = offsetDate(7);
assert.equal(new Date(date + "T00:00:00Z").getUTCDay(), 1, "QA start date must be a Monday");
assert.ok(date >= "2026-10-01", "Local automatic policy starts in October 2026");
assert.ok(Date.parse(exceptionDate + "T23:59:59+08:00") < Date.now(), "The full eight-day QA range must be historical");
assert.match(date, /^\d{4}-\d{2}-\d{2}$/);
assert.ok(Date.parse(date + "T23:59:59+08:00") < Date.now(), "Use a completed historical QA day");
const out = join(root, "runtime/browser-tests");
await mkdir(out, { recursive: true });
const report = { run, base, transport: mappedHTTPS ? "HTTPS 9880 through Caddy; Chrome maps kf.szputy.com to 127.0.0.1; temporary certificate accepted" : "Direct local HTTP backend; public HTTPS is not covered by this run", date, startedAt: new Date().toISOString(), checks: [], screenshots: [],
  fixtures: { employee_id: employeeID, account_username: run.toLowerCase() + "a", rule_name_prefix: run } };
const wait = ms => new Promise(resolve => setTimeout(resolve, ms));
const asRows = v => Array.isArray(v) ? v : v?.items || v?.list || v?.rows || [];
const sqlString = value => { assert.match(String(value), /^[A-Za-z0-9:_-]+$/); return "'" + value + "'"; };
const employeeSQL = sqlString(employeeID);
let admin, chrome, profile, page, created = false;

function localSQL(sql) {
  try {
    return execFileSync(process.env.MYSQL_CLI || "C:/Program Files/MySQL/MySQL Server 8.0/bin/mysql.exe", [
      "--protocol=TCP", "--host=" + config.host, "--port=" + config.port, "--user=" + config.user,
      "--database=" + config.name, "--default-character-set=utf8mb4", "--batch", "--skip-column-names",
    ], { input: sql, encoding: "utf8", windowsHide: true, env: { ...process.env, MYSQL_PWD: String(config.pass) }, stdio: ["pipe", "pipe", "pipe"] });
  } catch { throw Error("Local QA database operation failed; credentials and SQL output are suppressed"); }
}
const batchPredicate = `JSON_VALID(request_json) AND JSON_LENGTH(JSON_EXTRACT(request_json,'$.employee_ids'))=1 AND JSON_UNQUOTE(JSON_EXTRACT(request_json,'$.employee_ids[0]'))=${employeeSQL}`;
const dirtyPredicate = `JSON_VALID(employee_ids) AND JSON_LENGTH(employee_ids)=1 AND JSON_UNQUOTE(JSON_EXTRACT(employee_ids,'$[0]'))=${employeeSQL}`;
async function cleanup() {
  if (!created) return;
  // Stop future eligibility for this QA employee, drain its already queued
  // calculations, then delete only rows linked to this exact fixture identity.
  localSQL(`UPDATE biz_hrm_employee SET status='filtered' WHERE id=${employeeSQL} AND employee_no='${run}'; DELETE FROM biz_att_dirty WHERE ${dirtyPredicate};`);
  const deadline = Date.now() + 45000;
  let idleSince = 0;
  // The worker may already have selected a dirty row when filtering happens.
  // Require sustained empty state across two 3-second worker ticks, rather
  // than trusting one zero count and racing a late queued QA calculation.
  while (true) {
    localSQL(`DELETE FROM biz_att_dirty WHERE ${dirtyPredicate};`);
    const active = Number(localSQL(`SELECT COUNT(*) FROM biz_att_batch WHERE (${batchPredicate}) AND status IN ('queued','running');`).trim());
    if (active) idleSince = 0;
    else if (!idleSince) idleSince = Date.now();
    else if (Date.now() - idleSince >= 7000) break;
    if (Date.now() >= deadline) {
      report.cleanup = { passed: false, preserved: true, identifiers: { ...report.fixtures }, active_batches: active,
        guidance: "Wait for this exact QA employee's queued/running batches to stop, then retry precise fixture cleanup; do not delete running batches or unrelated dirty jobs." };
      throw Error("QA jobs did not remain idle across worker cycles; exact fixture identifiers are preserved in this report");
    }
    await wait(1000);
  }
  localSQL(`START TRANSACTION;
DELETE s FROM biz_att_clock_state s JOIN biz_hrm_attendance_clock c ON c.id=s.clock_id WHERE c.employee_id=${employeeSQL};
DELETE a FROM biz_att_audit a JOIN biz_att_roster r ON a.entity='roster' AND a.entity_id=r.id WHERE r.employee_id=${employeeSQL};
DELETE a FROM biz_att_audit a JOIN biz_att_adjustment x ON a.entity='adjustment' AND a.entity_id=x.id WHERE x.employee_id=${employeeSQL};
DELETE a FROM biz_att_audit a JOIN biz_att_ledger l ON a.entity='ledger' AND a.entity_id=l.id WHERE l.employee_id=${employeeSQL};
DELETE a FROM biz_att_audit a JOIN biz_att_rule r ON a.entity='rule' AND a.entity_id=r.id WHERE r.created_by=${admin.uid} AND r.name LIKE '${run}%';
DELETE FROM biz_att_result WHERE employee_id=${employeeSQL};
DELETE FROM biz_att_batch WHERE ${batchPredicate};
DELETE FROM biz_att_dirty WHERE ${dirtyPredicate};
DELETE FROM biz_att_adjustment WHERE employee_id=${employeeSQL};
DELETE FROM biz_att_ledger WHERE employee_id=${employeeSQL};
DELETE FROM biz_att_roster WHERE employee_id=${employeeSQL};
DELETE FROM biz_att_rule WHERE created_by=${admin.uid} AND name LIKE '${run}%';
DELETE FROM biz_att_audit WHERE created_by=${admin.uid};
DELETE FROM biz_hrm_attendance_clock WHERE employee_id=${employeeSQL};
DELETE FROM biz_hrm_employee WHERE id=${employeeSQL} AND employee_no='${run}';
COMMIT;`);
  await cleanupRun(run);
  const remaining = Number(localSQL(`SELECT
    (SELECT COUNT(*) FROM biz_hrm_employee WHERE id=${employeeSQL})+
    (SELECT COUNT(*) FROM biz_hrm_attendance_clock WHERE employee_id=${employeeSQL})+
    (SELECT COUNT(*) FROM biz_att_result WHERE employee_id=${employeeSQL})+
    (SELECT COUNT(*) FROM biz_att_roster WHERE employee_id=${employeeSQL})+
    (SELECT COUNT(*) FROM biz_att_adjustment WHERE employee_id=${employeeSQL})+
    (SELECT COUNT(*) FROM biz_att_ledger WHERE employee_id=${employeeSQL})+
    (SELECT COUNT(*) FROM biz_att_batch WHERE ${batchPredicate})+
    (SELECT COUNT(*) FROM biz_att_dirty WHERE ${dirtyPredicate})+
    (SELECT COUNT(*) FROM biz_att_rule WHERE created_by=${admin.uid} AND name LIKE '${run}%')+
    (SELECT COUNT(*) FROM biz_att_audit WHERE created_by=${admin.uid})+
    (SELECT COUNT(*) FROM gf_admin WHERE username='${run.toLowerCase()}a');`).trim());
  assert.equal(remaining, 0, "Exact QA fixtures must be fully removed");
  report.cleanup = { passed: true, remaining };
}

class BrowserPage {
  constructor(socket) {
    this.socket = socket; this.seq = 0; this.pending = new Map(); this.errors = []; this.requests = new Map(); this.publishRequests = 0; this.applyRequests = 0; this.wecomWriteRequests = 0;
    socket.addEventListener("message", ({ data }) => {
      const m = JSON.parse(data);
      if (m.id) { const p = this.pending.get(m.id); if (p) { clearTimeout(p.timer); this.pending.delete(m.id); m.error ? p.reject(Error(m.error.message)) : p.resolve(m.result); } }
      if (m.method === "Runtime.exceptionThrown") this.errors.push(m.params.exceptionDetails.exception?.description || m.params.exceptionDetails.text);
      if (m.method === "Page.frameNavigated" && !m.params.frame.parentId) for (const [id, loader] of this.requests) { if (loader !== m.params.frame.loaderId) this.requests.delete(id); }
      if (m.method === "Network.requestWillBeSent" && ["Fetch", "XHR"].includes(m.params.type)) this.requests.set(m.params.requestId, m.params.loaderId);
      if (m.method === "Network.requestWillBeSent" && m.params.request.method === "POST" && /\/api\/admin\/hrm\/attendance\/v2\/rules\/[^/]+\/publish$/.test(new URL(m.params.request.url).pathname)) this.publishRequests++;
      if (m.method === "Network.requestWillBeSent" && m.params.request.method === "POST" && /\/api\/admin\/hrm\/attendance\/v2\/rules\/[^/]+\/apply$/.test(new URL(m.params.request.url).pathname)) this.applyRequests++;
      if (m.method === "Network.requestWillBeSent" && m.params.request.method === "POST" && new URL(m.params.request.url).pathname.includes('/api/admin/wecom/')) this.wecomWriteRequests++;
      if (["Network.loadingFinished", "Network.loadingFailed"].includes(m.method)) this.requests.delete(m.params.requestId);
    });
    socket.addEventListener("close", () => { for (const p of this.pending.values()) { clearTimeout(p.timer); p.reject(Error("QA Chrome closed")); } this.pending.clear(); });
  }
  send(method, params = {}) { return new Promise((resolve, reject) => { const id = ++this.seq; const timer = setTimeout(() => { this.pending.delete(id); reject(Error("Chrome timeout: " + method)); }, 20000); this.pending.set(id, { resolve, reject, timer }); this.socket.send(JSON.stringify({ id, method, params })); }); }
  async eval(expression) { const r = await this.send("Runtime.evaluate", { expression, awaitPromise: true, returnByValue: true }); if (r.exceptionDetails) throw Error(r.exceptionDetails.exception?.description || r.exceptionDetails.text); return r.result.value; }
  async until(expression, timeout = 20000) { const end = Date.now() + timeout; while (Date.now() < end) { try { if (await this.eval(expression)) return; } catch {} await wait(100); } throw Error("Page condition timed out: " + expression); }
  async api(path, body, method) { return this.eval(`(async()=>{const {api}=await import('${prefix}assets/core.js');return api(${JSON.stringify(path)},${JSON.stringify(body)},${JSON.stringify(method)});})()`); }
  async go(path, title) { await this.send("Page.navigate", { url: base + prefix + path }); if (title) await this.until(`document.querySelector('#view h1')?.textContent===${JSON.stringify(title)}&&!document.querySelector('#view .loading')`); }
  async healthy() { const end = Date.now() + 20000; await wait(150); while (this.requests.size && Date.now() < end) await wait(100); assert.equal(this.requests.size, 0, "Page API requests did not finish"); assert.deepEqual(await this.eval("[...document.querySelectorAll('#view [role=alert]')].map(n=>n.textContent)"), [], "Page API failed"); assert.deepEqual(this.errors, [], "The actual page raised a JavaScript error"); }
  async fill(values, scope = "document") { await this.until(`${JSON.stringify(Object.keys(values))}.every(key=>${scope}?.querySelector('[name="'+key+'"]'))`); await this.eval(`(()=>{for(const [key,value] of Object.entries(${JSON.stringify(values)})){const node=${scope}.querySelector('[name="'+key+'"]');if(!node)throw Error('Missing input '+key);node.value=value;node.dispatchEvent(new Event('input',{bubbles:true}));node.dispatchEvent(new Event('change',{bubbles:true}));}})()`); }
  async click(text, scope = "document") { const expr = `[...${scope}.querySelectorAll('button')].find(b=>b.textContent.trim()===${JSON.stringify(text)})`; await this.until(`!!${expr}`); await this.eval(`${expr}.click()`); }
  async rowClick(name, text) { const row = `[...document.querySelectorAll('#view tbody tr')].find(r=>r.textContent.includes(${JSON.stringify(name)}))`; await this.until(`!!${row}`); await this.click(text, row); }
  async submit() { const before = await this.eval("document.querySelectorAll('dialog').length"); await this.eval("[...document.querySelectorAll('dialog')].at(-1).querySelector('form').requestSubmit()"); await this.until(`document.querySelectorAll('dialog').length<${before}||!![...document.querySelectorAll('dialog')].at(-1)?.querySelector('[role=alert]')`); assert.equal(await this.eval("[...document.querySelectorAll('dialog')].at(-1)?.querySelector('[role=alert]')?.textContent||''"), ""); }
  async shot(label) { await this.send("Page.bringToFront"); const { data } = await this.send("Page.captureScreenshot", { format: "png", captureBeyondViewport: false }); const file = `attendance-v2-${run}-${label}.png`; await writeFile(join(out, file), Buffer.from(data, "base64")); report.screenshots.push(file); }
}

async function startBrowser(credentials) {
  profile = await mkdtemp(join(tmpdir(), "attendance-v2-qa-"));
  const path = process.env.CHROME_PATH || "C:/Program Files/Google/Chrome/Application/chrome.exe";
  assert.ok(existsSync(path), "Chrome is unavailable");
  chrome = spawn(path, ["--headless=new", "--disable-gpu", "--no-first-run", "--no-default-browser-check", "--ignore-certificate-errors", "--remote-debugging-port=0", "--remote-allow-origins=*", ...(mappedHTTPS ? ["--host-resolver-rules=MAP kf.szputy.com 127.0.0.1", "--no-proxy-server"] : []), "--user-data-dir=" + profile, "about:blank"], { stdio: "ignore", windowsHide: true });
  let port;
  for (let i = 0; i < 100 && !port; i++) { try { port = (await readFile(join(profile, "DevToolsActivePort"), "utf8")).split("\n")[0]; } catch { await wait(100); } }
  assert.ok(port, "Chrome startup failed");
  const tabs = await (await fetch(`http://127.0.0.1:${port}/json/list`, { signal: AbortSignal.timeout(5000) })).json();
  const socket = new WebSocket(tabs.find(t => t.type === "page").webSocketDebuggerUrl);
  await new Promise((resolve, reject) => { const timer = setTimeout(() => reject(Error("Chrome connection timed out")), 10000); socket.addEventListener("open", () => { clearTimeout(timer); resolve(); }, { once: true }); socket.addEventListener("error", () => { clearTimeout(timer); reject(Error("Chrome connection failed")); }, { once: true }); });
  page = new BrowserPage(socket);
  await page.send("Runtime.enable"); await page.send("Page.enable"); await page.send("Network.enable");
  await page.send("Emulation.setDeviceMetricsOverride", { width: 1440, height: 1050, deviceScaleFactor: 1, mobile: false });
  await page.go("hrm/employees"); await page.until("!!document.querySelector('input[type=password]')");
  await wait(500);
  await page.eval(`(()=>{const f=document.querySelector('input[type=password]').closest('form');for(const [selector,value]of [['input[type=text]',${JSON.stringify(credentials.username)}],['input[type=password]',${JSON.stringify(credentials.password)}]]){const n=f.querySelector(selector);n.value=value;n.dispatchEvent(new Event('input',{bubbles:true}));}const cb=f.querySelector('input[type=checkbox]');if(cb?.checked)cb.click();f.querySelector('button[type=submit]').click();})()`);
  await page.until("!!document.querySelector('#view .subheading')"); await page.healthy();
}
async function step(name, work) { const start = Date.now(); try { await work(); report.checks.push({ name, passed: true, ms: Date.now() - start }); console.log("PASS " + name); } catch (e) { report.checks.push({ name, passed: false, error: e.message }); throw e; } finally { await writeFile(join(out, `attendance-v2-${run}-report.json`), JSON.stringify(report, null, 2)); } }
async function waitBatch(id) { const end = Date.now() + 90000; while (Date.now() < end) { const b = await page.api("/hrm/attendance/v2/batches/" + id); if (b.status === "failed") throw Error("QA calculation failed: " + b.error_message); if (b.status === "completed") return b; await wait(800); } throw Error("QA batch did not complete"); }
async function calculate() { const { id } = await page.api("/hrm/attendance/v2/batches", { start_date: date, end_date: date, employee_ids: [employeeID] }); await waitBatch(id); const rows = asRows(await page.api("/hrm/attendance/v2/results?batch_id=" + id)); assert.equal(rows.length, 1); assert.equal(rows[0].employee_id, employeeID); return { batch: id, row: rows[0] }; }
async function assertDetailHours(credited) {
  const tables = await page.eval(`(()=>{
    function rows(kind){const table=document.querySelector('dialog [data-attendance-hours="'+kind+'"] table');if(!table)throw Error('Missing labelled '+kind+' hours table');const headers=[...table.querySelectorAll('thead th')].map(n=>n.textContent.trim());return [...table.querySelectorAll('tbody tr')].map(row=>Object.fromEntries([...row.querySelectorAll('td')].map((cell,i)=>[headers[i],cell.textContent.trim()])));}
    return {summary:rows('total'),segments:rows('segments')};
  })()`);
  assert.equal(tables.summary.length, 1, "The hour overview must contain exactly one total row");
  const summary = tables.summary[0];
  assert.equal(summary["应出勤小时"], "7.75");
  assert.equal(summary["实际工作小时"], "11.00", "Detail must display full daytime plus evening actual work");
  assert.equal(summary["核定出勤小时"], credited, "Detail must distinguish credited attendance from actual work");
  assert.equal(summary["加班小时"], "3.00", "Detail must show overtime separately from credited attendance");
  const evening = tables.segments.find(row=>row["时段"] === "加班");
  assert.ok(evening, "The overtime segment must remain visible");
  assert.equal(evening["实际工作小时"], "3.25", "The overtime segment contains 195 actual minutes");
  assert.equal(evening["核定出勤小时"], "0.00");
  assert.equal(evening["加班小时"], "3.00", "Only 180 minutes count as overtime after the 18:00 threshold");
}
function unzipWorkbook(buffer) {
  let end = buffer.length - 22; while (end >= Math.max(0, buffer.length - 65557) && buffer.readUInt32LE(end) !== 0x06054b50) end--;
  assert.ok(end >= 0, "Export is not a ZIP workbook"); const total = buffer.readUInt16LE(end + 10), entries = new Map(); let at = buffer.readUInt32LE(end + 16);
  for (let i = 0; i < total; i++) { assert.equal(buffer.readUInt32LE(at), 0x02014b50); const method = buffer.readUInt16LE(at + 10), size = buffer.readUInt32LE(at + 20), plainSize = buffer.readUInt32LE(at + 24), nameLen = buffer.readUInt16LE(at + 28), extra = buffer.readUInt16LE(at + 30), comment = buffer.readUInt16LE(at + 32), local = buffer.readUInt32LE(at + 42), name = buffer.subarray(at + 46, at + 46 + nameLen).toString(); assert.equal(buffer.readUInt32LE(local), 0x04034b50); const start = local + 30 + buffer.readUInt16LE(local + 26) + buffer.readUInt16LE(local + 28), compressed = buffer.subarray(start, start + size); assert.ok(method === 0 || method === 8); const content = method === 0 ? compressed : inflateRawSync(compressed); assert.equal(content.length, plainSize); entries.set(name, content.toString()); at += 46 + nameLen + extra + comment; }
  for (const name of ["[Content_Types].xml", "xl/workbook.xml", "xl/worksheets/sheet1.xml"]) assert.ok(entries.has(name), "Missing XLSX part: " + name);
  return entries;
}

try {
  await step("preflight-isolated-unpublished-day", async () => {
    assert.equal(Number(localSQL("SELECT COUNT(*) FROM biz_schema_version WHERE version=5;").trim()), 1, "Deploy migration v5 first");
    const nearby = [-1, 0, 1].map(n => new Date(Date.parse(date + "T00:00:00Z") + n * 86400000).toISOString().slice(0, 7));
    assert.equal(Number(localSQL(`SELECT COUNT(*) FROM biz_att_month WHERE month IN (${[...new Set(nearby)].map(sqlString).join(",")});`).trim()), 0, "QA must not touch an existing real month lifecycle");
    admin = await createTestAdmin(run); created = true; report.fixtures.account_uid = admin.uid;
  });
  await step("host-login-and-qa-employee", async () => {
    await startBrowser(admin);
    if (!scheduleOnly) await page.api("/hrm/employees", { id: employeeID, user_id: admin.uid, employee_no: run, name: run + "考勤验收", dept_id: 0, status: "active", position: "临时自动化验收" });
  });
  await step("wecom-four-daily-times-readonly-console", async () => {
    const current = await page.api('/wecom/status');
    assert.deepEqual(current.automatic_schedule.times, ['08:30','10:00','14:30','23:59']);
    assert.equal(current.automatic_schedule.timezone,'Asia/Shanghai');
    assert.equal(current.automatic_schedule.startup_sync,false);
    assert.equal(current.automatic_schedule.catch_up,false);
    assert.match(current.automatic_schedule.next_run_at,/^\d{4}-\d{2}-\d{2} (08:30|10:00|14:30|23:59):00$/);
    assert.ok(Date.parse(current.automatic_schedule.next_run_at.replace(' ','T')+'+08:00') > Date.now());
    const beforeWrites = page.wecomWriteRequests;
    await page.go('wecom/status');
    await page.until("!!document.querySelector('[data-automatic-schedule] [data-schedule-times]')");
    await page.healthy();
    const panel = await page.eval("document.querySelector('[data-automatic-schedule]').textContent");
    for (const time of current.automatic_schedule.times) assert.ok(panel.includes(time));
    assert.ok(panel.includes(current.automatic_schedule.next_run_at));
    assert.equal(await page.eval("document.querySelector('[data-automatic-schedule]').querySelectorAll('input,select,button').length"),0);
    assert.deepEqual(await page.eval("[...document.querySelectorAll('[data-sync-kind]')].map(n=>n.getAttribute('data-sync-kind')).sort()"),['checkin','contact','leave','out','overtime','trip']);
    assert.equal(page.wecomWriteRequests,beforeWrites,'Opening the schedule must not write credentials or start synchronization');
    await page.eval("document.querySelector('[data-automatic-schedule]').scrollIntoView({block:'center'})");
    await page.shot('wecom-four-times');
    report.automatic_schedule = current.automatic_schedule;
    report.schedule_only = scheduleOnly;
  });
  if (!scheduleOnly) {
  let ruleID, result, firstPublished;
  const modal = "[...document.querySelectorAll('dialog')].at(-1)";
  const getResults = async day => asRows(await page.api("/hrm/attendance/v2/results?" + new URLSearchParams({employee_id:employeeID,start_date:day,end_date:day})));
  const latestResult = async (day, predicate = () => true) => {
    const end = Date.now()+90000;
    while(Date.now()<end){const rows=await getResults(day);if(rows[0]&&predicate(rows[0]))return {batch:rows[0].batch_id,row:rows[0]};await wait(800);}
    throw Error("Automatic QA recalculation did not finish for "+day);
  };
  const applyUI = async name => {
    const before = page.applyRequests;
    await page.rowClick(name,"发布");
    await page.until("!!document.querySelector('[data-attendance-apply]')");
    await page.until("[...document.querySelectorAll('[data-attendance-apply] button')].some(b=>b.textContent==='确认生效并自动计算'&&!b.disabled)");
    const preview = await page.api(`/hrm/attendance/v2/rules/${ruleID}/preview`,{start_date:date,end_date:exceptionDate});
    assert.equal(preview.can_apply,true);assert.equal(preview.employee_count,1);assert.equal(preview.day_count,8);assert.deepEqual(preview.locked_months||[],[]);
    await page.fill({start_date:date,end_date:exceptionDate,reason:run+"仅验收独立测试人员的本地制度"},"document.querySelector('[data-attendance-apply]')");
    await page.click("预览影响范围","document.querySelector('[data-attendance-apply]')");
    await page.until("[...document.querySelectorAll('[data-attendance-apply] button')].some(b=>b.textContent==='确认生效并自动计算'&&!b.disabled)");
    await page.shot("apply-preview");
    await page.click("确认生效并自动计算","document.querySelector('[data-attendance-apply]')");
    await page.until("!document.querySelector('[data-attendance-apply]')");
    assert.equal(page.applyRequests,before+1,"Only one explicit apply request should publish the configured rule");
    const published=asRows(await page.api("/hrm/attendance/v2/rules")).find(r=>r.id===ruleID);
    assert.equal(published.status,"published");assert.deepEqual(published.scope.employee_ids,[employeeID]);assert.equal(published.scope.is_default,false);
    // Poll only this fixture's auto-queued eight-day task. No manually created
    // rosters or manual calculation batch may make this acceptance pass.
    let ids=[];const deadline=Date.now()+20000;
    while(!ids.length&&Date.now()<deadline){ids=localSQL(`SELECT id FROM biz_att_batch WHERE (${batchPredicate}) AND start_date='${date}' AND end_date='${exceptionDate}' ORDER BY created_at DESC;`).trim().split(/\s+/).filter(Boolean);if(!ids.length)await wait(300);}
    assert.ok(ids.length,"Applying a local rule must enqueue the affected historical range");
    for(const id of ids)await waitBatch(id);
    return published;
  };
  await step("isolated-local-policy-and-raw-clock-fixtures",async()=>{
    const preset=await page.eval(`(async()=>{const {productionFirstDepartmentPreset}=await import('${prefix}assets/attendance-v2.js');return productionFirstDepartmentPreset({effective_from:${JSON.stringify(date)},employee_ids:[${JSON.stringify(employeeID)}]});})()`);
    preset.name=run+"本地考勤制度";preset.effective_to=exceptionDate;
    Object.assign(preset.config.rule,{evidence_mode:"local",attendance_mode:"fixed_complete",standard_minutes:480});
    // Explicit, test-only confirmed windows; never seed these as real business
    // defaults. Half-open noon boundary is 12:40:00 exactly.
    const windows={morning:{in_start_second:25200,in_end_second:36000,out_start_second:39600,out_end_second:45600},afternoon:{in_start_second:45600,in_end_second:57600,out_start_second:61200,out_end_second:63600},"evening-overtime":{in_start_second:63600,in_end_second:66600,out_start_second:66600,out_end_second:108000}};
    function assign(shift){for(const segment of shift.segments)segment.local_windows={...windows[segment.id]};if(shift.overtime_variant)assign(shift.overtime_variant.shift);}
    for(const shift of preset.config.shifts)assign(shift);
    preset.config.shifts[0].segments[0].local_windows.in_start_second=null;
    ruleID=(await page.api("/hrm/attendance/v2/rules",preset)).id;
    for(const [day,times]of [[date,["08:00:00","11:40:00","13:10:00","17:15:00","17:45:00","21:00:00"]],[weekendDate,["08:00:00","11:40:00","13:10:00","17:15:00","17:45:00","21:00:00"]],[exceptionDate,["08:00:00","11:40:00","13:10:00","17:15:00","21:00:00"]]])for(const[i,time]of times.entries())await page.api("/hrm/attendance/clocks",{id:run+"-clock-"+day+"-"+i,employee_id:employeeID,clock_time:day+" "+time,clock_type:i%2?"in":"out",status:i%2?"early":"late",address:run+"验收原始卡，不采用上游状态"});
    assert.equal(asRows(await page.api(`/hrm/attendance/v2/rosters?employee_id=${employeeID}&start_date=${date}&end_date=${exceptionDate}`)).length,0,"Draft rules may not generate rosters");
  });
  await step("configure-missing-window-and-apply-through-ui",async()=>{
    await page.go("hrm/attendance-rules","考勤设置");
    await page.rowClick(run+"本地考勤制度","发布");
    await page.until("!!document.querySelector('[data-attendance-publish-preflight]')");
    assert.ok((await page.eval("document.querySelector('[data-attendance-publish-preflight]').textContent")).includes("窗口"));
    assert.equal(page.applyRequests,0,"A missing window must not publish");
    await page.shot("window-required");
    await page.click("去完善制度","document.querySelector('dialog')");
    await page.until("!!document.querySelector('[data-attendance-rule-editor]')");
    await page.fill({name:run+"已确认本地制度",attendance_mode:"fixed_complete",standard_minutes:"480"},"document.querySelector('[data-attendance-rule-editor]')");
    await page.fill({in_start_second:"07:00:00"},"document.querySelector('[data-attendance-rule-editor] [data-local-windows]')");
    assert.equal(await page.eval("document.querySelector('[name=schedule_mode]').value"),"weekly");
    const week=await page.eval("[...document.querySelectorAll('[data-attendance-weekly] [data-weekday]')].map(row=>({text:row.textContent,category:row.querySelector('[name^=weekly_category_]').value}))");
    assert.equal(week.length,7);for(const[i,item]of week.entries()){assert.ok(item.text.includes(['周一','周二','周三','周四','周五','周六','周日'][i]));assert.equal(item.category,i<5?'workday':'restday');}
    await page.click("保存草稿","document.querySelector('[data-attendance-rule-editor]')");
    await page.until("!document.querySelector('[data-attendance-rule-editor]')");
    firstPublished=await applyUI(run+"已确认本地制度");
    assert.equal(firstPublished.config.rule.evidence_mode,"local");
    assert.equal(firstPublished.config.shifts[0].segments[0].local_windows.in_start_second,25200);
    result=await latestResult(date,row=>row.rule_id===ruleID&&row.result.state==='calculated');
    assert.equal(result.row.result.actual_seconds,660*60);assert.equal(result.row.result.credited_seconds,480*60);assert.equal(result.row.result.overtime_seconds,180*60);
    assert.equal(result.row.result.late_count,0);assert.equal(result.row.result.early_count,0,"Upstream late/early and reversed card types must not decide local attendance");
    const rosters=asRows(await page.api(`/hrm/attendance/v2/rosters?employee_id=${employeeID}&start_date=${date}&end_date=${exceptionDate}`));
    assert.equal(rosters.length,8);assert.ok(rosters.every(r=>r.source==='automatic'),"Apply must derive rosters without manual scheduling");
    const weekend=await latestResult(weekendDate,row=>row.rule_id===ruleID&&row.result.state==='calculated');
    assert.equal(weekend.row.result.overtime_seconds,660*60);assert.equal(weekend.row.result.expected_seconds,0);
    const missing=await latestResult(exceptionDate,row=>row.rule_id===ruleID);assert.equal(missing.row.result.state,'review');
    report.localResults={normal:{credited_hours:8,actual_hours:11,overtime_hours:3},weekend:{overtime_hours:11},missing_return_punch:missing.row.result.state};
    await page.go('hrm/attendance-results?batch_id='+result.batch,'考勤明细与异常');await page.rowClick(date,'计算详情');await page.until("!!document.querySelector('[data-attendance-hours]')");await assertDetailHours('8.00');await page.shot('automatic-local-calculation');
  });
  await step("missing-return-punch-review-and-automatic-recalculation",async()=>{
    const before=await latestResult(exceptionDate,row=>row.result.state==='review');
    await page.go('hrm/attendance-results?batch_id='+before.batch,'考勤明细与异常');await page.rowClick(exceptionDate,'计算详情');await page.until("document.querySelector('dialog')?.textContent.includes('计算依据')");
    await page.click('人工认定 / 补签',"document.querySelector('dialog')");
    await page.fill({kind:'punch',at:exceptionDate+'T17:45',punch_type:'in'},modal);
    assert.equal(await page.eval(`${modal}.querySelector('form').checkValidity()`),false,'A supplement requires a reason and evidence');
    await page.fill({reason:run+'核对晚间返岗漏卡',evidence:run+'签到签字与班组长核对，补17:45返岗'},modal);await page.submit();
    const adjustments=asRows(await page.api('/hrm/attendance/v2/adjustments?employee_id='+employeeID));assert.equal(adjustments.length,1);assert.equal(JSON.parse(adjustments[0].adjustment_json).kind,'punch');
    const repaired=await latestResult(exceptionDate,row=>row.id!==before.row.id&&row.result.state==='calculated');
    assert.equal(repaired.row.result.credited_seconds,480*60);assert.equal(repaired.row.result.overtime_seconds,180*60);
    assert.equal(asRows(await page.api('/hrm/attendance/clocks?employee_id='+employeeID)).length,17,'Supplement must not overwrite enterprise raw cards');
    await page.go('hrm/attendance-results?batch_id='+repaired.batch,'考勤明细与异常');await page.rowClick(exceptionDate,'计算详情');await page.until("!!document.querySelector('[data-attendance-hours]')");await assertDetailHours('8.00');await page.shot('review-supplement');
  });
  await step("historical-rule-new-version-auto-recalculates",async()=>{
    await page.go('hrm/attendance-rules','考勤设置');await page.rowClick(run+'已确认本地制度','编辑 / 新版本');await page.until("!!document.querySelector('[data-attendance-rule-editor]')");
    await page.fill({name:run+'历史修订版本',late_grace_minutes:'3'},"document.querySelector('[data-attendance-rule-editor]')");await page.click('保存草稿',"document.querySelector('[data-attendance-rule-editor]')");await page.until("!document.querySelector('[data-attendance-rule-editor]')");
    const rules=asRows(await page.api('/hrm/attendance/v2/rules')),draft=rules.find(r=>r.name===run+'历史修订版本');assert.ok(draft&&draft.id!==ruleID&&draft.version>firstPublished.version);assert.equal(draft.family_id,firstPublished.family_id);
    assert.deepEqual(rules.find(r=>r.id===firstPublished.id),firstPublished,'Published evidence and rule version must remain immutable');
    ruleID=draft.id;await applyUI(draft.name);result=await latestResult(date,row=>row.rule_id===ruleID&&row.result.state==='calculated');
    const rosters=asRows(await page.api(`/hrm/attendance/v2/rosters?employee_id=${employeeID}&start_date=${date}&end_date=${exceptionDate}`));assert.ok(rosters.every(r=>r.rule_id===ruleID));
    assert.equal(asRows(await page.api('/hrm/attendance/v2/adjustments?employee_id='+employeeID)).length,1,'Historical recalculation must retain manual evidence');
  });
  await step("latest-result-filters-and-local-clock-ui",async()=>{
    const rows=asRows(await page.api('/hrm/attendance/v2/results?'+new URLSearchParams({q:run,start_date:date,end_date:exceptionDate})));assert.equal(rows.length,8);assert.ok(rows.every(row=>row.employee_id===employeeID&&row.rule_id===ruleID));
    assert.equal(asRows(await page.api('/hrm/attendance/v2/results?'+new URLSearchParams({q:employeeID,start_date:date,end_date:exceptionDate}))).length,0,'Hidden internal IDs are not name/employee-number search terms');
    await page.go('hrm/attendance-results?batch_id='+result.batch,'考勤明细与异常');await page.fill({q:run});await page.click('查询结果');await page.until(`document.querySelector('#view tbody')?.textContent.includes(${JSON.stringify(run)})`);await page.healthy();
    await page.go('hrm/clocks','打卡记录');await page.fill({clock_date_mode:'day',clock_date:date,clock_employee_search:run});await page.until(`!!document.querySelector('[name=clock_employee_id] option[value="${employeeID}"]')`);await page.fill({clock_employee_id:employeeID});await page.click('查询');await page.until(`document.querySelector('#view tbody')?.textContent.includes(${JSON.stringify(run)})`);await page.healthy();await page.shot('local-clock-basis');
  });
  await step("monthly-summary-keeps-actual-credited-and-overtime-separate",async()=>{
    const month=date.slice(0,7), summary=await page.api('/hrm/attendance/month?'+new URLSearchParams({month,employee_id:employeeID}));
    assert.equal(summary.basis,'local_trial');
    const person=asRows(summary).find(row=>row.employee_id===employeeID);
    assert.ok(person);assert.equal(person.employee_no,run);
    assert.equal(person.actual_hours,33);assert.equal(person.overtime_hours,17);
    assert.equal(person.overtime_workday_hours,6);assert.equal(person.overtime_restday_hours,11);assert.equal(person.overtime_holiday_hours,0);
    await page.go('hrm/attendance-months?month='+month,'月度考勤');await page.healthy();
    await page.until("!!document.querySelector('[data-attendance-month-summary] input[type=search]')");
    await page.eval(`(()=>{const search=document.querySelector('[data-attendance-month-summary] input[type=search]');search.value=${JSON.stringify(run)};search.dispatchEvent(new Event('input',{bubbles:true}));})()`);
    await page.until(`!![...document.querySelectorAll('[data-attendance-month-summary] tbody tr')].find(row=>row.textContent.includes(${JSON.stringify(run)}))`);
    const row=await page.eval(`[...document.querySelectorAll('[data-attendance-month-summary] tbody tr')].find(row=>row.textContent.includes(${JSON.stringify(run)})).textContent`);
    assert.ok(row.includes('33.00')&&row.includes('17.00')&&row.includes('11.00'));
    report.monthlySummary={actual_hours:person.actual_hours,credited_hours:person.credited_hours,overtime_hours:person.overtime_hours,workday_overtime_hours:person.overtime_workday_hours,restday_overtime_hours:person.overtime_restday_hours};
    await page.shot('monthly-local-summary');
  });
  await step("incomplete-month-close-is-rejected-without-business-mutation",async()=>{
    const month=date.slice(0,7),before=localSQL(`SELECT COUNT(*) FROM biz_att_month WHERE month='${month}';`).trim();
    await page.go('hrm/attendance-months','月度考勤');await page.until("[...document.querySelectorAll('#view button')].some(b=>b.textContent==='一键月结')");
    // Explicit isolated batch prevents auto-selection of any real all-company
    // monthly batch. Never submit the production UI's unrestricted month close.
    const response=await page.eval(`(async()=>{try{const{api}=await import('${prefix}assets/core.js');await api('/hrm/attendance/v2/months/${month}/close',${JSON.stringify({batch_id:result.batch,reason:run+'隔离人员不完整月结应拒绝'})});return {accepted:true};}catch(error){return {accepted:false,message:error.message};}})()`);
    assert.equal(response.accepted,false,'A one-employee eight-day fixture must never lock a real company month');assert.match(response.message,/完整|批次|月报/);assert.equal(localSQL(`SELECT COUNT(*) FROM biz_att_month WHERE month='${month}';`).trim(),before);
    report.monthClose={rejectedPartialBatch:true,message:response.message,successfulClose:'Covered by isolated SQLite integration tests; never locks production months'};await page.shot('monthly-close-safe-boundary');await page.healthy();
  });
  await step("export-valid-xlsx-from-authenticated-host",async()=>{
    const file=await page.eval(`(async()=>{const response=await fetch('/plugins/biz-suite/api/admin/hrm/attendance/v2/export?batch_id=${result.batch}',{credentials:'include',headers:{'X-Requested-With':'XMLHttpRequest'}});if(!response.ok)throw Error('Export failed');const bytes=new Uint8Array(await response.arrayBuffer());let raw='';for(let i=0;i<bytes.length;i+=16384)raw+=String.fromCharCode(...bytes.subarray(i,i+16384));return {type:response.headers.get('content-type'),data:btoa(raw)};})()`);
    assert.ok(file.type.includes('spreadsheetml'));const bytes=Buffer.from(file.data,'base64'),entries=unzipWorkbook(bytes);assert.ok([...entries.values()].some(s=>s.includes(run)));const rows=asRows(await page.api('/hrm/attendance/v2/results?batch_id='+result.batch));assert.ok(rows.every(row=>row.employee_id===employeeID),'Export batch must contain only the test employee');
    const name=`attendance-v2-${run}.xlsx`;await writeFile(join(out,name),bytes);report.workbook={file:name,bytes:bytes.length,sha256:createHash('sha256').update(bytes).digest('hex')};
  });
  await step("three-primary-pages-and-advanced-pages-remain-available",async()=>{
    for(const[route,title]of [['rules','考勤设置'],['results','考勤明细与异常'],['months','月度考勤'],['rosters','人员排班'],['batches','考勤计算批次'],['imports','工作簿导入与核对'],['ledger','调休台账'],['overtime','加班凭据'],['audit','考勤操作审计']]){await page.go('hrm/attendance-'+route,title);await page.healthy();}
    assert.equal(page.wecomWriteRequests,0,'Local attendance QA must not call WeCom synchronization');
  });
  }
  report.passed = true;
} catch (error) {
  report.passed = false; report.error = error.message; process.exitCode = 1;
  console.error("FAIL attendance-v2: " + error.message);
  try { await page?.shot("failure"); } catch {}
} finally {
  try { page?.socket.close(); chrome?.kill(); } catch {}
  try { await cleanup(); } catch (error) { report.cleanup = { ...report.cleanup, passed: false, identifiers: { ...report.fixtures }, error: error.message }; process.exitCode = 1; console.error("QA cleanup incomplete for " + run + ": " + error.message); }
  if (profile) {
    const parent = resolve(tmpdir()), target = resolve(profile), child = relative(parent, target);
    assert.ok(child && !child.startsWith("..") && !isAbsolute(child) && target.includes("attendance-v2-qa-"), "Unsafe browser-profile cleanup path");
    await wait(500); await rm(target, { recursive: true, force: true, maxRetries: 5, retryDelay: 300 }).catch(() => {});
  }
  report.finishedAt = new Date().toISOString(); await writeFile(join(out, `attendance-v2-${run}-report.json`), JSON.stringify(report, null, 2));
  console.log("Report: runtime/browser-tests/attendance-v2-" + run + "-report.json");
}
