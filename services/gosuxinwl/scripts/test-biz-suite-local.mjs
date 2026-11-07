// Real Chrome + running host + MySQL acceptance. No mocked API responses.
// Local-only: reads ignored runtime/local-access.json; creates clearly named QA fixtures.
import assert from "node:assert/strict";
import { readFile, writeFile, mkdir, mkdtemp, rm } from "node:fs/promises";
import { existsSync } from "node:fs";
import { resolve, join, dirname, basename } from "node:path";
import { tmpdir } from "node:os";
import { spawn } from "node:child_process";
import {
  createHash,
  createCipheriv,
  createDecipheriv,
  randomBytes,
} from "node:crypto";
import { cleanupRun, createTestAdmin, createOrganizationScopeFixtures, createEnterpriseIMFixtures, inspectCredentialFixtureStorage } from "./cleanup-biz-suite-test.mjs";
const root = resolve(import.meta.dirname, "..");
const access = JSON.parse(
  (await readFile(join(root, "runtime/local-access.json"), "utf8")).replace(
    /^\uFEFF/,
    ""
  )
);
const base = new URL(access.url).origin;
assert.ok(
  ["127.0.0.1", "localhost"].includes(new URL(base).hostname),
  "Run only against local development"
);
assert.equal(access.database, "kaoqin_dev");
assert.ok(["9880", "19880"].includes(new URL(base).port), "Use the local development HTTP backend");
const prefix = "/plugins/biz-suite/admin/";
const output = join(root, "runtime/browser-tests");
await mkdir(output, { recursive: true });
const run = "QA" + Date.now();
const report = {
  run,
  base,
  startedAt: new Date().toISOString(),
  checks: [],
  fixtures: {},
  screenshots: [],
};
const wait = (ms) => new Promise((r) => setTimeout(r, ms));
const md5 = (value) => createHash("md5").update(value).digest("hex");
function encrypt(value) {
  const c = createCipheriv(
    "aes-128-cbc",
    "eI2lI3zL0qD0hY7m",
    "6289984834922396"
  );
  return Buffer.concat([c.update(value), c.final()]).toString("base64");
}
function decrypt(value) {
  const c = createDecipheriv(
    "aes-128-cbc",
    "eI2lI3zL0qD0hY7m",
    "6289984834922396"
  );
  return Buffer.concat([c.update(value, "base64"), c.final()]).toString();
}
async function host(path, body, token, method) {
  const ts = Math.floor(Date.now() / 1000),
    headers = {
      apiverify: Buffer.from(
        md5("suxin-public-example" + ts) + "#" + ts
      ).toString("base64"),
      "Content-Type": "application/json",
    };
  if (token) headers.Authorization = encrypt(token + "#" + ts);
  const res = await fetch(base + path, {
    method: method || (body === undefined ? "GET" : "POST"),
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
    signal: AbortSignal.timeout(20000),
  });
  const data = await res.json();
  assert.equal(data.code, 0, `${path}: ${data.message}`);
  return data.data;
}
class Page {
  constructor(socket) {
    this.socket = socket;
    this.seq = 0;
    this.pending = new Map();
    this.errors = [];
    this.wsFrames = [];
    this.requests = new Map();
    this.http = [];
    socket.addEventListener("message", ({ data }) => {
      const m = JSON.parse(data);
      if (m.method === "Page.frameNavigated" && !m.params.frame.parentId) {
        // Full-document navigation can cancel XHRs without a matching loading
        // event on the old renderer. Only the new document's requests matter.
        const loader = m.params.frame.loaderId;
        for (const [id, request] of this.requests)
          if (request.loaderId !== loader) this.requests.delete(id);
      }
      if (
        m.method === "Network.responseReceived" &&
        ["Fetch", "XHR"].includes(m.params.type)
      )
        this.http.push({
          path: new URL(m.params.response.url).pathname,
          status: m.params.response.status,
        });
      if (
        m.method === "Network.requestWillBeSent" &&
        ["Fetch", "XHR"].includes(m.params.type)
      )
        this.requests.set(m.params.requestId, { url: m.params.request.url, loaderId: m.params.loaderId });
      if (
        ["Network.loadingFinished", "Network.loadingFailed"].includes(m.method)
      )
        this.requests.delete(m.params.requestId);
    });
    socket.addEventListener("message", ({ data }) => {
      const m = JSON.parse(data);
      if (m.id) {
        const p = this.pending.get(m.id);
        if (p) {
          clearTimeout(p.timer);
          this.pending.delete(m.id);
          m.error ? p.reject(Error(m.error.message)) : p.resolve(m.result);
        }
      }
      if (m.method === "Runtime.exceptionThrown")
        this.errors.push(
          m.params.exceptionDetails.exception?.description ||
            m.params.exceptionDetails.text
        );
      if (m.method === "Network.webSocketFrameReceived")
        this.wsFrames.push(m.params.response.payloadData);
    });
    const disconnected = () => {
      for (const pending of this.pending.values()) {
        clearTimeout(pending.timer);
        pending.reject(Error("Test browser debugging connection closed"));
      }
      this.pending.clear();
    };
    socket.addEventListener("close", disconnected);
    socket.addEventListener("error", disconnected);
  }
  send(method, params = {}) {
    return new Promise((resolve, reject) => {
      const id = ++this.seq;
      const timer = setTimeout(() => {
        this.pending.delete(id);
        reject(Error(`Test browser timed out waiting for ${method}`));
      }, 15000);
      this.pending.set(id, { resolve, reject, timer });
      try { this.socket.send(JSON.stringify({ id, method, params })); }
      catch (error) { clearTimeout(timer); this.pending.delete(id); reject(error); }
    });
  }
  async eval(expression) {
    const r = await this.send("Runtime.evaluate", {
      expression,
      awaitPromise: true,
      returnByValue: true,
    });
    if (r.exceptionDetails)
      throw Error(
        r.exceptionDetails.exception?.description || r.exceptionDetails.text
      );
    return r.result.value;
  }
  async until(expression) {
    const deadline = Date.now() + 15000;
    while (Date.now() < deadline) {
      try {
        if (await this.eval(expression)) return;
      } catch {}
      await wait(80);
    }
    throw Error(
      "Timeout: " +
        expression +
        "\n" +
        JSON.stringify(await this.eval("({path:location.pathname,title:document.querySelector('#view h1')?.textContent,rows:document.querySelectorAll('tbody tr').length,dialogs:document.querySelectorAll('dialog').length,alerts:document.querySelectorAll('[role=alert]').length})"))
    );
  }
  async go(path) {
    await this.send("Page.navigate", { url: base + path });
  }
  async nav(path) {
    if (path === "overview") path = "hrm/overview";
    // Each workspace now owns its own sidebar. Cross workspace navigation uses
    // the same visible topbar link as a user, before selecting the destination.
    if (!(await this.eval(`!!document.querySelector('[data-route="${path}"]')`))) {
      const area = path.startsWith("im/") ? "im" : "hrm";
      await this.until(`!!document.querySelector('.topbar a[href="${prefix}${area}"]')`);
      await this.eval(`document.querySelector('.topbar a[href="${prefix}${area}"]').click()`);
      await this.until(`!!document.querySelector('[data-route="${path}"]')`);
    }
    const title = await this.eval(
      `document.querySelector('[data-route="${path}"]').textContent.replace(/^\\d+/, '').trim()`
    );
    await this.eval(`document.querySelector('[data-route="${path}"]').click()`);
    const expected =
      {
        "hrm/overview": "智慧HRM",
        "wecom/status": "企业微信同步",
        "im/manager": "通讯管理统计",
        "wecom/departments": "企业微信部门映射",
      }[path] || title;
    await this.until(
      `location.pathname.endsWith(${JSON.stringify(
        path
      )}) && document.querySelector('#view h1')?.textContent===${JSON.stringify(
        expected
      )} && !document.querySelector('#view .loading')`
    );
    await this.healthy();
  }
  async healthy() {
    for (let i = 0; i < 150 && this.requests.size; i++) await wait(60);
    assert.equal(
      this.requests.size,
      0,
      "Page API requests did not finish: " +
        [...this.requests.values()].map((request) => request.url).join(",")
    );
    assert.deepEqual(
      await this.eval(
        `[...document.querySelectorAll('#view [role=alert]')].map(x=>x.textContent)`
      ),
      []
    );
  }
  async click(text, scope = "document") {
    await this.until(
      `[...${scope}.querySelectorAll('button')].some(x=>x.textContent.trim()===${JSON.stringify(
        text
      )})`
    );
    await this.eval(
      `(()=>{const b=[...${scope}.querySelectorAll('button')].find(x=>x.textContent.trim()===${JSON.stringify(
        text
      )});b.click();})()`
    );
  }
  async fill(values, scope = "document") {
    await this.eval(
      `(()=>{for(const [k,v] of Object.entries(${JSON.stringify(
        values
      )})){const e=${scope}.querySelector('[name="'+k+'"]');if(!e)throw Error('Field not found: '+k);if(e.multiple){for(const o of e.options)o.selected=v.map(String).includes(o.value);}else e.value=v;e.dispatchEvent(new Event('input',{bubbles:true}));e.dispatchEvent(new Event('change',{bubbles:true}));}})()`
    );
  }
  async save() {
    await this.eval(`document.querySelector('dialog form').requestSubmit()`);
    await this.until(
      `!document.querySelector('dialog') || !!document.querySelector('dialog [role=alert]')`
    );
    assert.equal(
      await this.eval(
        `document.querySelector('dialog [role=alert]')?.textContent||''`
      ),
      ""
    );
    await this.until(`!document.querySelector('#view .loading')`);
    await this.healthy();
  }
  async create(button, values) {
    await this.click(button);
    await this.until(`!!document.querySelector('dialog form')`);
    await this.fill(values, `document.querySelector('dialog')`);
    await this.save();
  }
  async searchRecords(term, server = true) {
    await this.until("!!document.querySelector('#view input[aria-label=\"搜索记录\"],#view input[name=clock_search]')");
    await this.eval(`(()=>{const input=document.querySelector('#view input[name=clock_search],#view input[aria-label="搜索记录"]');input.value=${JSON.stringify(term)};input.dispatchEvent(new Event('input',{bubbles:true}));})()`);
    if (server) await this.click(await this.eval("!!document.querySelector('#view form.clock-filters')") ? "查询" : "搜索");
    await this.until("!document.querySelector('#view .loading')");
    await this.healthy();
  }
  async api(path, body, method) {
    return this.eval(
      `(async()=>{const {api}=await import('${prefix}assets/core.js');return api(${JSON.stringify(
        path
      )},${JSON.stringify(body)},${JSON.stringify(method)});})()`
    );
  }
  async raw(path, body, method) {
    return this.eval(`(async()=>{const r=await fetch('/plugins/biz-suite/api/admin'+${JSON.stringify(path)},{method:${JSON.stringify(method || (body === undefined ? "GET" : "POST"))},credentials:'include',headers:{'X-Requested-With':'XMLHttpRequest','Content-Type':'application/json'},${body === undefined ? "" : `body:JSON.stringify(${JSON.stringify(body)}),`}});return {status:r.status,...await r.json().catch(()=>({}))};})()`);
  }
  async shot(name) {
    // Headless Chrome may defer screenshot rendering for a background popup.
    await this.send("Page.bringToFront");
    const { data } = await this.send("Page.captureScreenshot", {
      format: "png",
      captureBeyondViewport: false,
    });
    await writeFile(join(output, name + ".png"), Buffer.from(data, "base64"));
    report.screenshots.push(name + ".png");
  }
  async openMenu(title) {
    const list = () => fetch(`http://127.0.0.1:${this.debugPort}/json/list`, { signal: AbortSignal.timeout(5000) }).then(r => r.json());
    const before = new Set((await list()).map(tab => tab.id));
    const selector = `[...document.querySelectorAll('.arco-menu-item')].find(x=>x.textContent.trim()===${JSON.stringify(title)})`;
    await this.until(`!!${selector}`);
    const clicked = await this.send("Runtime.evaluate", { expression: `${selector}.click()`, userGesture: true, returnByValue: true });
    assert.equal(clicked.exceptionDetails, undefined, `${title} menu click failed`);
    let tab;
    for (let attempt = 0; attempt < 100 && !tab; attempt++) {
      tab = (await list()).find(item => item.type === "page" && !before.has(item.id));
      if (!tab) await wait(100);
    }
    assert.ok(tab, `${title} did not open an independent page`);
    const socket = await connectDebugSocket(tab.webSocketDebuggerUrl);
    const opened = new Page(socket);
    opened.debugPort = this.debugPort;
    pages.push(opened);
    await opened.send("Runtime.enable");
    await opened.send("Page.enable");
    await opened.send("Network.enable");
    await opened.send("Emulation.setDeviceMetricsOverride", { width: 1440, height: 1000, deviceScaleFactor: 1, mobile: false });
    return opened;
  }
}
const processes = [],
  profiles = [],
  pages = [];
async function connectDebugSocket(url) {
  const socket = new WebSocket(url);
  return new Promise((resolve, reject) => {
    const timer = setTimeout(() => { socket.close(); reject(Error("Test browser debugging connection timed out")); }, 10000);
    socket.addEventListener("open", () => { clearTimeout(timer); resolve(socket); }, { once: true });
    socket.addEventListener("error", () => { clearTimeout(timer); reject(Error("Test browser debugging connection failed")); }, { once: true });
  });
}
async function browser(credentials, { expectedLoginError } = {}) {
  const profile = await mkdtemp(join(tmpdir(), "biz-suite-browser-"));
  profiles.push(profile);
  const chrome =
    process.env.CHROME_PATH ||
    "C:/Program Files/Google/Chrome/Application/chrome.exe";
  assert.ok(existsSync(chrome), "Chrome unavailable");
  const proc = spawn(
    chrome,
    [
      "--headless=new",
      "--disable-gpu",
      "--no-first-run",
      "--no-default-browser-check",
      "--remote-debugging-port=0",
      "--remote-allow-origins=*",
      "--user-data-dir=" + profile,
      "about:blank",
    ],
    { stdio: "ignore", windowsHide: true }
  );
  processes.push(proc);
  let port;
  for (let i = 0; i < 100; i++) {
    try {
      port = (
        await readFile(join(profile, "DevToolsActivePort"), "utf8")
      ).split("\n")[0];
      break;
    } catch {}
    await wait(100);
  }
  assert.ok(port, "Chrome startup failed");
  const tabs = await (
    await fetch("http://127.0.0.1:" + port + "/json/list", { signal: AbortSignal.timeout(5000) })
  ).json();
  const socket = await connectDebugSocket(
    tabs.find((x) => x.type === "page").webSocketDebuggerUrl
  );
  const page = new Page(socket);
  page.debugPort = port;
  pages.push(page);
  await page.send("Runtime.enable");
  await page.send("Page.enable");
  await page.send("Network.enable");
  await page.send("Emulation.setDeviceMetricsOverride", {
    width: 1440,
    height: 1000,
    deviceScaleFactor: 1,
    mobile: false,
  });
  await page.go(prefix + "hrm/employees");
  await page.until(`!!document.querySelector('input[type=password]')`);
  // The host fetches its login challenge on mount, even when the image is
  // disabled. Wait for that request before submitting the form.
  await page.healthy();
  assert.ok(
    (await page.eval("location.pathname")).includes("login"),
    "Anonymous plugin page must redirect to host login"
  );
  await page.eval(
    `(()=>{const form=document.querySelector('input[type=password]').closest('form');for(const [selector,value]of [['input[type=text]',${JSON.stringify(
      credentials.username
    )}],['input[type=password]',${JSON.stringify(
      credentials.password
    )}]]){const e=form.querySelector(selector);e.value=value;e.dispatchEvent(new Event('input',{bubbles:true}));}const cb=form.querySelector('input[type=checkbox]');if(cb?.checked)cb.click();})()`
  );
  await page.eval(
    `document.querySelector('input[type=password]').closest('form').querySelector('button[type=submit]').click()`
  );
  if (expectedLoginError) {
    await page.until(`[...document.querySelectorAll('.arco-message-content,.arco-form-item-message')].some(item=>item.textContent.includes(${JSON.stringify(expectedLoginError)}))`);
    await page.healthy();
    assert.ok((await page.eval("location.pathname")).includes("login"));
    assert.equal(await page.eval("!!localStorage.getItem('biztoken')"), false, "failed login must not establish a host session");
    return page;
  }
  try {
    await page.until(`!!document.querySelector('#view .subheading')`);
  } catch (error) {
    report.loginDiagnostics = {
      errors: page.errors,
      http: page.http,
      form: await page.eval(
        `(()=>{const f=document.querySelector('input[type=password]')?.closest('form');return {valid:f?.checkValidity(),fields:[...(f?.querySelectorAll('input')||[])].map(x=>({type:x.type,length:x.value.length})),buttons:[...(f?.querySelectorAll('button')||[])].map(x=>({type:x.type,disabled:x.disabled,text:x.textContent})),messages:[...document.querySelectorAll('.arco-form-item-message,.arco-message-content')].map(x=>x.textContent)};})()`
      ),
    };
    throw error;
  }
  await page.healthy();
  return page;
}
async function step(name, fn) {
  const start = Date.now();
  report.activeCheck = { name, startedAt: new Date(start).toISOString() };
  await writeFile(join(output, "report.json"), JSON.stringify(report, null, 2));
  try {
    const result = await fn();
    report.checks.push({
      name,
      status: result?.skip ? "skipped" : "passed",
      ...(result?.skip ? { reason: result.skip } : {}),
      durationMs: Date.now() - start,
    });
    console.log((result?.skip ? "SKIP " : "PASS ") + name);
  } catch (error) {
    report.checks.push({ name, status: "failed", error: error.message });
    throw error;
  } finally {
    delete report.activeCheck;
    await writeFile(
      join(output, "report.json"),
      JSON.stringify(report, null, 2)
    );
  }
}
function assertNoCredentialValues(value, label) {
  if (Array.isArray(value)) {
    for (const item of value) assertNoCredentialValues(item, label);
  } else if (value && typeof value === "object") {
    for (const [key, item] of Object.entries(value)) {
      assert.ok(
        !/^(?:secret|contact_secret|app_secret|checkin_secret|approval_secret|access_token|token|ciphertext)$/i.test(key),
        `${label} must not return credential values`
      );
      assertNoCredentialValues(item, label);
    }
  }
}
let a, b, testAdmin;
try {
  testAdmin = await createTestAdmin(run);
  await step("宿主登录、插件深链接跳转与身份认证", async () => {
    a = await browser(testAdmin);
    const cookies = await a.send("Network.getCookies", { urls: [base + prefix] });
    const sessionCookie = cookies.cookies.find(cookie => cookie.name === "suxin_plugin_biz-suite_admin");
    assert.ok(sessionCookie?.httpOnly, "plugin session must remain HttpOnly");
    const remaining = sessionCookie.expires - Date.now() / 1000;
    assert.ok(remaining > 14100 && remaining <= 14460, "plugin cookie must last approximately four hours");
    report.sessionLifetime = { cookieHours: 4, httpOnly: true };
    assert.equal((await a.api("/access")).uid, testAdmin.uid);
    await a.searchRecords(run);
    await a.shot("01-employees-empty");
  });
  await step("全部 20 个业务页面加载（真实 MySQL）", async () => {
    for (const route of [
      "overview",
      "hrm/employees",
      "hrm/groups",
      "hrm/holidays",
      "hrm/clocks",
      "hrm/month",
      "hrm/leaves",
      "hrm/outs",
      "hrm/trips",
      "im/chat",
      "im/directory",
      "im/groups",
      "im/channels",
      "im/face-packs",
      "im/sensitive-words",
      "im/manager",
      "wecom/status",
      "wecom/templates",
      "wecom/departments",
      "wecom/exceptions",
    ])
      await a.nav(route);
    assert.ok(
      !(await a.eval('document.querySelector("nav").textContent')).includes(
        "OA"
      )
    );
  });
  let emp;
  // Explicit deployment operation, disabled in normal acceptance runs. It uses
  // the existing tenant snapshot and intentionally keeps the real mappings.
  if (process.env.BIZ_SUITE_APPLY_ORGANIZATION === "1") {
    await step("应用本地组织与停用账号映射并验证重复执行", async () => {
      await a.nav("hrm/employees");
      const synchronize = async () => {
        await a.click("同步组织与账号");
        await a.until("document.querySelector('[data-organization-sync]')?.textContent.includes('上次同步结果') && [...document.querySelectorAll('button')].some(b=>b.textContent==='同步组织与账号'&&!b.disabled)");
        await a.healthy();
        return a.api("/wecom/organization/sync");
      };
      const first = await synchronize();
      assert.ok(first.employees_linked > 0, "known employees should receive local account mappings");
      const second = await synchronize();
      assert.equal(second.accounts_created, 0, "repeated sync must not create extra accounts");
      assert.equal(second.departments_created, 0, "repeated sync must not create extra departments");
      report.organizationApplied = { first, repeat: second };
      await a.shot("02-organization-mapping");
    });
  }
  await step("员工新建、编辑、搜索与账号映射", async () => {
    await a.nav("hrm/employees");
    const departments = (await a.api("/hrm/options")).departments;
    const testDepartment = departments.find((item) => Number(item.pid) === 0) || departments[0];
    assert.ok(testDepartment?.id, "organization sync must supply a valid local department before acceptance");
    await a.create("+ 新增员工", {
      name: run + "员工",
      employee_no: run,
      wecom_userid: run,
      user_id: 0,
      dept_id: Number(testDepartment.id),
      position: "测试工程师",
      status: "active",
      entry_date: "2026-10-01",
    });
    emp = (await a.api("/hrm/employees?search=" + run))[0];
    assert.ok(emp?.id);
    report.fixtures.employee = emp.id;
    await a.searchRecords(run);
    await a.click(
      "编辑",
      `[...document.querySelectorAll('tbody tr')].find(x=>x.textContent.includes('${run}'))`
    );
    await a.fill({ position: "测试主管" }, `document.querySelector('dialog')`);
    await a.save();
    await a.searchRecords(run);
    await a.until(
      `document.querySelector('tbody')?.textContent.includes('测试主管')`
    );
    assert.equal((await a.api("/hrm/employees?search=" + run)).length, 1);
    await a.shot("02-employee");
  });
  await step("考勤组、标准班次和节假日创建", async () => {
    await a.nav("hrm/groups");
    await a.create("+ 新增考勤组", {
      name: run + "班组",
      employee_ids: [emp.id],
    });
    report.fixtures.group = (await a.api("/hrm/attendance/groups?search=" + run)).find(
      (x) => x.name === run + "班组"
    ).id;
    await a.nav("hrm/holidays");
    // A global holiday must not change the current attendance calendar. Use
    // an unoccupied far-future day and still check the exact date server-side.
    let holidayDate;
    for (let day = 1; day <= 31; day++) {
      const candidate = "2099-12-" + String(day).padStart(2, "0");
      if (!((await a.api("/hrm/attendance/holidays?search=" + candidate)) || []).length) {
        holidayDate = candidate;
        break;
      }
    }
    assert.ok(holidayDate, "No free isolated calendar date for QA");
    await a.create("+ 新增日期", {
      holiday_date: holidayDate,
      name: run + "调休日",
      is_workday: 1,
    });
    report.fixtures.holiday = (await a.api("/hrm/attendance/holidays?search=" + run)).find(
      (x) => x.name === run + "调休日"
    ).id;
  });
  await step("上下班打卡表单保存、编辑和数据库回读", async () => {
    await a.nav("hrm/clocks");
    for (const [clock_type, clock_time] of [
      ["in", "2026-10-26T09:05"],
      ["out", "2026-10-26T17:55"],
    ])
      await a.create("+ 新增手工打卡", {
        employee_id: emp.id,
        clock_type,
        clock_time,
        address: run + "办公室",
      });
    const rows = await a.api("/hrm/attendance/clocks?employee_id=" + emp.id);
    assert.equal(rows.length, 2);
    assert.deepEqual(rows.map((x) => x.clock_type).sort(), ["in", "out"]);
    await a.searchRecords(run + "员工");
    await a.click("编辑", `[...document.querySelectorAll('tbody tr')].find(x=>x.textContent.includes('${run}员工') && x.textContent.includes('${run}办公室'))`);
    assert.equal(await a.eval("document.querySelector('dialog [name=employee_id]').value"), emp.id);
    await a.fill(
      { address: run + "办公区" },
      `document.querySelector('dialog')`
    );
    await a.save();
    const updated = await a.api("/hrm/attendance/clocks?employee_id=" + emp.id);
    assert.equal(updated.filter((row) => row.address === run + "办公区").length, 1);
    await a.shot("03-clocks");
  });
  await step("打卡姓名与日期、月份、部门、人员组合筛选", async () => {
    await a.nav("hrm/clocks");
    const options = await a.api("/hrm/attendance/clocks/options");
    assert.ok(options.employees.some((row) => row.id === emp.id && row.name === run + "员工"));
    const departmentKey = "host:" + emp.dept_id;
    assert.ok(options.departments.some((row) => row.key === departmentKey));
    const submit = async () => {
      await a.click("查询", "document.querySelector('form.clock-filters')");
      await a.until("!document.querySelector('#view .loading')");
      await a.healthy();
    };
    await a.fill({clock_employee_search: run}, "document.querySelector('form.clock-filters')");
    await a.fill({clock_employee_id: emp.id, clock_department: departmentKey, clock_date_mode: "day", clock_date: "2026-10-26"}, "document.querySelector('form.clock-filters')");
    await submit();
    const checkRows = async (query, expected) => {
      const rows = (await a.api("/hrm/attendance/clocks?employee_id=" + emp.id + "&department=" + encodeURIComponent(departmentKey) + "&" + query)) || [];
      assert.equal(rows.length, expected);
      for (const row of rows) {
        assert.equal(row.employee_name, run + "员工");
        assert.ok(Array.isArray(row.department_names));
      }
    };
    await checkRows("date=2026-10-26", 2);
    assert.ok((await a.eval("document.querySelector('tbody').textContent")).includes(run + "员工"));
    assert.ok(!(await a.eval("document.querySelector('tbody').textContent")).includes(emp.id));
    await a.fill({clock_date_mode: "month", clock_month: "2026-10"}, "document.querySelector('form.clock-filters')");
    await submit();
    await checkRows("month=2026-10", 2);
    await a.fill({clock_month: "2026-11"}, "document.querySelector('form.clock-filters')");
    await submit();
    await checkRows("month=2026-11", 0);
    await a.fill({clock_date_mode: "range", clock_start_time: "2026-10-26", clock_end_time: "2026-10-25"}, "document.querySelector('form.clock-filters')");
    await a.click("查询", "document.querySelector('form.clock-filters')");
    assert.ok((await a.eval("document.querySelector('form.clock-filters [role=alert]')?.textContent || ''")).includes("结束日期不能早于开始日期"));
    await a.fill({clock_end_time: "2026-10-26"}, "document.querySelector('form.clock-filters')");
    await submit();
    await checkRows("start_time=2026-10-26&end_time=2026-10-26", 2);
    await a.click("刷新", "document.querySelector('#view .subheading')");
    await a.until("!document.querySelector('#view .loading')");
    await a.healthy();
    assert.equal(await a.eval("document.querySelector('[name=clock_employee_id]').value"), emp.id);
    assert.equal(await a.eval("document.querySelector('[name=clock_start_time]').value"), "2026-10-26");
    await a.shot("03-clocks-filters");
    await a.click("清空筛选", "document.querySelector('form.clock-filters')");
    await a.until("!document.querySelector('#view .loading')");
    await a.healthy();
    assert.equal(await a.eval("document.querySelector('[name=clock_date_mode]').value"), "all");
    assert.equal(await a.eval("document.querySelector('[name=clock_employee_id]').value"), "");
    assert.equal(await a.eval("document.querySelector('[name=clock_department]').value"), "");
  });
  await step("人员输入直接查询、真实工号与部门人员联动", async () => {
    await a.nav("hrm/clocks");
    const scope = "document.querySelector('form.clock-filters')";
    const submit = async () => {
      await a.click("查询", scope);
      await a.until("!document.querySelector('#view .loading')");
      await a.healthy();
    };
    const options = await a.api("/hrm/attendance/clocks/options");
    await a.fill({ clock_employee_search: run + "员工" }, scope);
    assert.equal(await a.eval("document.querySelector('[name=clock_employee_id]').value"), "");
    await submit();
    assert.equal(await a.eval("document.querySelectorAll('tbody tr').length"), 2);
    assert.ok((await a.eval("document.querySelector('tbody').textContent")).includes(run + "员工"));
    assert.equal(await a.eval("document.querySelector('[name=clock_employee_search]').value"), run + "员工");
    const employee = options.employees.find(item => item.name === "胡起伟");
    if (employee) {
      assert.equal(employee.employee_no, "1599");
      const wang = options.employees.find(item => item.name === "王芳");
      if (wang) assert.equal(wang.employee_no, "3240");
      for (const field of ["employee_search", "search"]) {
        const records = await a.api(`/hrm/attendance/clocks?${field}=1599&page_size=500`);
        assert.ok(records.length > 0);
        assert.ok(records.every(row => row.employee_id === employee.id), "1599 must identify the real employee number, not an internal ID");
      }
      await a.fill({ clock_employee_search: "1599" }, scope);
      await submit();
      assert.ok((await a.eval("document.querySelector('tbody').textContent")).includes("胡起伟"));
      assert.ok(!(await a.eval("document.querySelector('tbody').textContent")).includes("王芳"));
    }
    await a.fill({ clock_employee_search: "" }, scope);
    const departments = new Map(options.departments.map(item => [item.key, item]));
    const belongs = (person, wanted, children) => person.department_keys.some(membership => {
      const seen = new Set();
      for (let key = membership; key && !seen.has(key);) {
        if (key === wanted) return true;
        if (!children) return false;
        seen.add(key);
        const current = departments.get(key);
        key = current?.parent_id > 0 ? `${current.source}:${current.parent_id}` : "";
      }
      return false;
    });
    const department = options.departments.find(item => {
      const direct = options.employees.filter(person => belongs(person, item.key, false)).length;
      const subtree = options.employees.filter(person => belongs(person, item.key, true)).length;
      return subtree > direct && subtree < options.employees.length;
    });
    assert.ok(department, "need an existing department with children for acceptance");
    const subtreeIDs = options.employees.filter(person => belongs(person, department.key, true)).map(person => person.id).sort();
    const directIDs = options.employees.filter(person => belongs(person, department.key, false)).map(person => person.id).sort();
    const outside = options.employees.find(person => !belongs(person, department.key, true));
    await a.fill({ clock_employee_id: outside.id, clock_department: department.key }, scope);
    const choices = () => a.eval("[...document.querySelector('[name=clock_employee_id]').options].filter(o=>o.value&&!o.disabled).map(o=>o.value).sort()");
    assert.deepEqual(await choices(), subtreeIDs);
    assert.equal(await a.eval("document.querySelector('[name=clock_employee_id]').value"), "", "department switch must clear a person outside its scope");
    await a.eval("(()=>{const input=document.querySelector('[name=clock_include_subdepartments]');input.checked=false;input.dispatchEvent(new Event('change',{bubbles:true}));})()");
    assert.deepEqual(await choices(), directIDs);
    await a.eval("(()=>{const input=document.querySelector('[name=clock_include_subdepartments]');input.checked=true;input.dispatchEvent(new Event('change',{bubbles:true}));})()");
    assert.deepEqual(await choices(), subtreeIDs);
    report.clockPeople = { directTextQuery: true, realNumber1599: !!employee, subtreeMembers: subtreeIDs.length, directMembers: directIDs.length, clearsOutsideSelection: true };
    await a.click("清空筛选", scope);
    await a.until("!document.querySelector('#view .loading')");
    await a.healthy();
  });
  await step("请假、外出表单与月度考勤计算", async () => {
    await a.nav("hrm/leaves");
    await a.create("+ 新增请假", {
      employee_id: emp.id,
      leave_type: "事假",
      start_time: "2026-10-27T09:00",
      end_time: "2026-10-27T18:00",
      duration: 8,
      approval_status: "approved",
      reason: run,
    });
    await a.nav("hrm/outs");
    await a.create("+ 新增外出", {
      employee_id: emp.id,
      destination: run + "客户现场",
      start_time: "2026-10-28T09:00",
      end_time: "2026-10-28T12:00",
      duration: 3,
      approval_status: "approved",
      reason: run,
    });
    await a.nav("hrm/month");
    await a.eval("document.querySelector('input[type=month]').value='2026-10'");
    await a.click("查询");
    await a.until("!document.querySelector('#view .loading')");
    await a.healthy();
    await a.searchRecords(run, false);
    const result = await a.api("/hrm/attendance/month?month=2026-10&employee_id=" + emp.id);
    const row = result.items.find((x) => x.employee_id === emp.id);
    assert.ok(row);
    assert.equal(
      row.worked_days,
      2,
      "one clocked day plus approved outside-work day"
    );
    assert.equal(row.late_count, 1);
    assert.equal(row.early_count, 1);
    assert.equal(row.leave_hours, 8);
    assert.equal(row.out_hours, 3);
    report.attendance = {
      workedDays: row.worked_days,
      late: row.late_count,
      early: row.early_count,
      leaveHours: row.leave_hours,
      outHours: row.out_hours,
    };
    await a.click(
      "每日明细",
      `[...document.querySelectorAll('tbody tr')].find(x=>x.textContent.includes('${run}'))`
    );
    await a.until(
      `document.querySelector('dialog')?.textContent.includes('2026-10-01')`
    );
    await a.shot("04-attendance-details");
    await a.click("×", `document.querySelector('dialog')`);
    await a.nav("hrm/leaves");
    await a.searchRecords(run + "员工");
    await a.click(
      "编辑",
      `[...document.querySelectorAll('tbody tr')].find(x=>x.textContent.includes('${run}员工'))`
    );
    await a.fill(
      { approval_status: "canceled" },
      `document.querySelector('dialog')`
    );
    await a.save();
    assert.equal(
      (await a.api("/hrm/attendance/month?month=2026-10&employee_id=" + emp.id)).items.find(
        (x) => x.employee_id === emp.id
      ).leave_hours,
      0
    );
  });
  await step("出差独立记录、月度统计与撤销", async () => {
    await a.nav("hrm/trips");
    await a.create("+ 新增出差", {
      employee_id: emp.id,
      destination: run + "出差地",
      start_time: "2026-10-27T09:00",
      end_time: "2026-10-27T18:00",
      duration: 8,
      approval_status: "approved",
      reason: run,
    });
    const before = (
      await a.api("/hrm/attendance/month?month=2026-10&employee_id=" + emp.id)
    ).items.find((x) => x.employee_id === emp.id);
    assert.equal(before.trip_hours, 8);
    assert.equal(before.out_hours, 3);
    assert.equal(before.leave_hours, 0);
    await a.searchRecords(run + "员工");
    await a.click(
      "编辑",
      `[...document.querySelectorAll('tbody tr')].find(x=>x.textContent.includes('${run}员工'))`
    );
    await a.fill(
      { approval_status: "canceled" },
      `document.querySelector('dialog')`
    );
    await a.save();
    assert.equal(
      (await a.api("/hrm/attendance/month?month=2026-10&employee_id=" + emp.id)).items.find(
        (x) => x.employee_id === emp.id
      ).trip_hours,
      0
    );
    await a.shot("05-trip");
  });
  await step("请假外出出差姓名、事由与审批表单详情", async () => {
    report.absenceForms = {};
    for (const [kind, label] of [["leaves", "请假"], ["outs", "外出"], ["trips", "出差"]]) {
      await a.nav("hrm/" + kind);
      await a.searchRecords(run + "员工");
      const rows = await a.api(`/hrm/attendance/${kind}?search=${encodeURIComponent(run + "员工")}`);
      assert.equal(rows.length, 1, `${label} must search by employee name`);
      const row = rows[0];
      assert.equal(row.employee_name, run + "员工");
      assert.equal(row.reason, run);
      assert.equal(Object.hasOwn(row, "raw_json"), false);
      assert.equal(Object.hasOwn(row, "detail_json"), false);
      if (kind === "leaves") assert.equal(row.leave_type_name, "事假");
      const text = await a.eval("document.querySelector('tbody').textContent");
      assert.ok(text.includes(run + "员工") && text.includes(run));
      assert.ok(!text.includes(emp.id), "employee ID must not replace the name");
      await a.click("详情", "document.querySelector('tbody tr')");
      await a.until(`!!document.querySelector('dialog .approval-sheet[data-approval-kind="${kind}"]')`);
      assert.equal(await a.eval("document.querySelectorAll('dialog pre,dialog code').length"), 0);
      const sheet = await a.eval("document.querySelector('dialog .approval-sheet').textContent");
      assert.ok(sheet.includes(run + "员工") && sheet.includes(label + "事由") && sheet.includes(run));
      assert.ok(sheet.includes("手工登记") && !sheet.includes("raw_json") && !sheet.includes(emp.id));
      if (kind === "leaves") assert.ok(sheet.includes("事假") && sheet.includes("已撤销"));
      if (kind === "trips") assert.ok(sheet.includes(run + "出差地") && sheet.includes("已撤销"));
      await a.shot("05-approval-form-" + kind);
      await a.click("×", "document.querySelector('dialog')");
      const detail = await a.api(`/hrm/attendance/${kind}/${encodeURIComponent(row.id)}`);
      assert.equal(detail.employee_name, row.employee_name);
      assert.equal(detail.reason, row.reason);
      assert.equal(Object.hasOwn(detail, "raw_json"), false);
      assert.equal(Object.hasOwn(detail, "detail_json"), false);
      assert.ok(Array.isArray(detail.approval_form.fields) && Array.isArray(detail.approval_form.timeline));
      // Existing WeCom records are read only; never replay or modify approvals.
      const existing = (await a.api(`/hrm/attendance/${kind}?page_size=500`)).filter(item => item.source === "wecom");
      if (kind === "leaves") for (const item of existing) {
        assert.ok(/[\u3400-\u9fff]/.test(item.leave_type_name || ""), "historical leave type must have a readable label");
        assert.ok(!/未识别|待识别|未提供/.test(item.leave_type_name), "existing labeled WeCom vacation must resolve");
      }
      if (existing.length) {
        const saved = await a.api(`/hrm/attendance/${kind}/${encodeURIComponent(existing[0].id)}`);
        assert.ok(saved.employee_name && saved.employee_name !== saved.employee_id);
        assert.ok(saved.approval_form.fields.length > 0, `${label} raw controls must render as fields`);
        assert.ok(saved.approval_form.timeline.length > 0, `${label} approval steps must be available`);
        assert.equal(Object.hasOwn(saved, "raw_json"), false);
        report.absenceForms[kind] = { manualForm: true, existingWecomRecords: existing.length, rawFields: saved.approval_form.fields.length, approvalSteps: saved.approval_form.timeline.length };
      } else report.absenceForms[kind] = { manualForm: true, existingWecomRecords: 0 };
    }
  });
  await step("企业微信中文部门、人员过滤界面与配置回显", async () => {
    await a.nav("wecom/status");
    const original = await a.api("/wecom/settings");
    const connection = await a.api("/wecom/status");
    if (connection.corp_id || Object.values(connection.credentials || {}).some((c) => c.configured)) {
      const nameKeys = ["include_user_names", "exclude_user_names", "include_department_names", "exclude_department_names"];
      for (const key of nameKeys) {
        assert.equal(await a.eval(`document.querySelector('[name=${key}]')?.value`), (original[key] || []).join("\n"));
      }
      await a.fill({exclude_user_names: "段详政", exclude_department_names: "分子公司\n新入职员工"}, "document.querySelector('#wecom-settings')");
      assert.equal(await a.eval("[...document.querySelectorAll('[data-sync-kind],[data-diagnostic-kind]')].every(b=>b.disabled)"), true);
      await a.eval("document.querySelector('[name=exclude_department_names]').scrollIntoView({block:'center'})");
      await a.shot("05-wecom-name-filters-unsaved");
      // Keep real credentials and live sync rules untouched; mock-browser tests
      // exercise saves. Reload discards only these unsaved DOM edits.
      await a.go(prefix + "wecom/status");
      await a.until("!!document.querySelector('[name=exclude_user_names]')");
      await a.healthy();
      for (const key of nameKeys) {
        assert.equal(await a.eval(`document.querySelector('[name=${key}]').value`), (original[key] || []).join("\n"));
      }
      assert.deepEqual(await a.api("/wecom/settings"), original);
      report.wecomNameFilters = {mode: "read_only_existing_configuration", nameFields: true, unsavedGuard: true, configurationUnchanged: true};
      return;
    }
    try {
      await a.fill(
        {
          corp_id: original.corp_id || connection.corp_id || "wwLocalTest",
          agent_id: "1000002",
          include_department_ids: "2, 4, 2",
          exclude_department_ids: "3",
          include_userids: "alice,bob",
          exclude_userids: "bob",
          include_user_names: "测试员工，段详政，段详政",
          exclude_user_names: "段详政",
          include_department_names: "",
          exclude_department_names: "分子公司\n新入职员工",
        },
        `document.querySelector('#wecom-settings')`
      );
      assert.ok(
        await a.eval(
          "[...document.querySelectorAll('[data-sync-kind]')].every(b=>b.disabled)"
        )
      );
      assert.equal(
        await a.eval(
          "document.querySelectorAll('[data-diagnostic-kind]').length===4 && [...document.querySelectorAll('[data-diagnostic-kind]')].every(b=>b.disabled)"
        ),
        true,
        "Unsaved configuration must also block diagnostics"
      );
      await a.click("保存配置");
      await a.until(
        "document.querySelector('#wecom-settings').textContent.includes('配置已保存')"
      );
      const saved = await a.api("/wecom/settings");
      assert.deepEqual(saved.include_department_ids, [2, 4]);
      assert.deepEqual(saved.exclude_department_ids, [3]);
      assert.deepEqual(saved.include_userids, ["alice", "bob"]);
      assert.deepEqual(saved.exclude_userids, ["bob"]);
      assert.deepEqual(saved.include_user_names, ["段详政", "测试员工"]);
      assert.deepEqual(saved.exclude_user_names, ["段详政"]);
      assert.deepEqual(saved.exclude_department_names, ["分子公司", "新入职员工"]);
      assert.equal(saved.agent_id, "1000002");
      assertNoCredentialValues(saved, "Saved settings");
      await a.go(prefix + "wecom/status");
      await a.healthy();
      await a.until(
        "document.querySelector('#wecom-settings [name=agent_id]')?.value==='1000002'"
      );
      assert.equal(
        await a.eval("document.querySelector('[name=exclude_userids]').value"),
        "bob"
      );
      assert.equal(
        await a.eval("document.querySelectorAll('[data-sync-kind]').length"),
        5
      );
      assert.equal(await a.eval("document.querySelector('[name=exclude_user_names]').value"), "段详政");
      assert.equal(await a.eval("document.querySelector('[name=exclude_department_names]').value"), "分子公司\n新入职员工");
      await a.eval("document.querySelector('[name=exclude_department_names]').scrollIntoView({block:'center'})");
      await a.shot("05-wecom-name-filters");
      await a.shot("05-wecom-settings");
      await a.eval(
        "document.querySelector('.sync-actions').scrollIntoView({block:'center'})"
      );
      await a.shot("05-wecom-sync-buttons");
      report.wecomNameFilters = {mode: "empty_enterprise_save_and_reload", saveReloadAndRestore: true};
    } finally {
      await a.api("/wecom/settings", original);
    }
  });
  await step("两套凭据不回显、缺配置诊断与业务数据隔离", async () => {
    await a.nav("wecom/status");
    const connection = await a.api("/wecom/status");
    const settings = await a.api("/wecom/settings");
    assertNoCredentialValues(connection, "Connection status");
    assertNoCredentialValues(settings, "Settings");
    for (const kind of ["contact", "app"]) {
      const credential = connection.credentials?.[kind];
      assert.ok(credential, `Missing ${kind} credential metadata`);
      assert.equal(typeof credential.configured, "boolean");
      assert.equal(typeof credential.source, "string");
      assert.ok(
        credential.environment_variable == null || typeof credential.environment_variable === "string"
      );
      assert.equal(
        await a.eval(
          `(()=>{const e=document.querySelector('[name=${kind}_secret]');return !!e && e.type==='password' && e.value==='';})()`
        ),
        true,
        "Saved secrets must never populate form inputs"
      );
    }
    const snapshotPaths = [
      "/wecom/sync/runs",
      "/wecom/filter-options",
      "/wecom/exceptions",
      "/hrm/employees",
      "/hrm/attendance/clocks",
      "/hrm/attendance/leaves",
      "/hrm/attendance/outs",
      "/hrm/attendance/trips",
    ];
    const before = await Promise.all(snapshotPaths.map((path) => a.api(path)));
    const checked = [], skipped = [];
    for (const kind of ["contact", "app", "checkin", "approval"]) {
      const credential = connection.credentials[kind === "contact" ? "contact" : "app"];
      if (credential.configured) {
        // Do not make live third-party requests with the user's credentials in
        // a local UI regression; mock-upstream tests cover remote responses.
        skipped.push(kind);
        continue;
      }
      await a.eval(`document.querySelector('[data-diagnostic-kind=${kind}]').click()`);
      await a.until(
        `document.querySelector('[data-diagnostic-result=${kind}]')?.textContent.includes('缺少配置') && !document.querySelector('[data-diagnostic-kind=${kind}]').disabled`
      );
      const result = await a.api(`/wecom/diagnostics/${kind}`, {});
      assertNoCredentialValues(result, "Diagnostic result");
      assert.equal(result.kind, kind);
      assert.equal(result.status, "missing");
      assert.equal(result.credential_configured, false);
      assert.equal(typeof result.checked_at, "string");
      assert.ok(Array.isArray(result.checks));
      checked.push(kind);
    }
    const after = await Promise.all(snapshotPaths.map((path) => a.api(path)));
    // Log only the affected endpoint, never business records or credentials.
    for (let i = 0; i < snapshotPaths.length; i++)
      assert.ok(JSON.stringify(after[i]) === JSON.stringify(before[i]), `Diagnostics changed ${snapshotPaths[i]}`);
    report.wecomDiagnostics = { checkedWithoutCredentials: checked, skippedLiveCredentials: skipped, businessDataUnchanged: true };
    await a.shot("05-wecom-diagnostics");
  });
  await step("空企业本地网页保存、保留与清除加密 Secret", async () => {
    await a.nav("wecom/status");
    const original = await a.api("/wecom/settings");
    const initial = await a.api("/wecom/status");
    if (initial.corp_id || !["contact", "app"].every((kind) => initial.credentials?.[kind]?.configured === false && initial.credentials[kind].has_saved === false)) {
      const reason = "Existing corporation or credentials: preserve user configuration";
      report.wecomCredentialLifecycle = { status: "skipped", reason };
      return { skip: reason };
    }
    let mayHaveWrittenFixture = false;
    const passwordsEmpty = () => a.eval(
      "['contact','app'].every(k=>document.querySelector('[name='+k+'_secret]')?.value==='')"
    );
    const saveForm = async () => {
      await a.eval("document.querySelector('#wecom-settings').requestSubmit()");
      await a.until("!document.querySelector('#wecom-settings').hasAttribute('aria-busy') && document.querySelector('#wecom-settings').textContent.includes('配置已保存')");
      await a.healthy();
      assert.equal(await passwordsEmpty(), true, "Secret inputs must clear after saving");
    };
    const assertSaved = async () => {
      const status = await a.api("/wecom/status");
      assert.equal(status.corp_id, "", "Fixture must never enable an enterprise connection");
      assertNoCredentialValues(status, "Saved credential status");
      assertNoCredentialValues(await a.api("/wecom/settings"), "Saved credential settings");
      for (const kind of ["contact", "app"]) {
        assert.equal(status.credentials[kind].configured, true);
        assert.equal(status.credentials[kind].source, "saved");
        assert.equal(status.credentials[kind].has_saved, true);
      }
      assert.equal(await a.eval("[...document.querySelectorAll('[data-sync-kind]')].every(b=>b.disabled)"), true);
    };
    try {
      await a.fill({ corp_id: "", contact_secret: run + "-contact-secret", app_secret: run + "-app-secret" }, "document.querySelector('#wecom-settings')");
      mayHaveWrittenFixture = true;
      await saveForm();
      await assertSaved();
      await a.go(prefix + "wecom/status");
      await a.until("!!document.querySelector('#wecom-settings [name=contact_secret]')");
      await a.healthy();
      assert.equal(await passwordsEmpty(), true, "Refresh must not populate saved secrets");
      await assertSaved();
      await saveForm();
      await assertSaved();
      const storage = await inspectCredentialFixtureStorage(run);
      assert.equal(storage.encryptedOnly, true, "MySQL credential rows must contain ciphertext only");
      assert.equal(storage.settingsNoPlaintext, true, "Settings JSON must not contain fixture secrets");
      assert.equal(storage.auditNoPlaintext, true, "Host audit must not contain fixture secrets");
      await a.eval("for(const k of ['contact','app']){const c=document.querySelector('[name=clear_'+k+'_secret]');c.checked=true;c.dispatchEvent(new Event('change',{bubbles:true}));c.dispatchEvent(new Event('input',{bubbles:true}));}");
      await saveForm();
      const cleared = await a.api("/wecom/status");
      assert.equal(cleared.corp_id, "");
      for (const kind of ["contact", "app"]) {
        assert.equal(cleared.credentials[kind].configured, false);
        assert.equal(cleared.credentials[kind].has_saved, false);
        assert.equal(cleared.credentials[kind].source, "missing");
      }
      report.wecomCredentialLifecycle = { status: "passed", saveReloadBlankPreserveAndClear: true, ...storage, externalRequestsEnabled: false };
      await a.shot("05-wecom-secrets-cleared");
    } finally {
      if (mayHaveWrittenFixture)
        await a.api("/wecom/settings", { ...original, clear_contact_secret: true, clear_app_secret: true });
    }
  });
  await step("出差审批模板保存与缺失凭据禁用同步", async () => {
    await a.nav("wecom/templates");
    await a.create("+ 新增模板", {
      template_id: run,
      approval_type: "trip",
      attendance_control_id: "qa_period",
      reason_control_id: "qa_reason",
      enabled: 0,
    });
    assert.ok(
      (await a.api("/wecom/templates")).some(
        (x) => x.template_id === run && x.approval_type === "trip"
      )
    );
    await a.nav("wecom/status");
    const connection = await a.api("/wecom/status");
    assert.ok(!("secret" in connection));
    if (!connection.corp_id || !connection.scopes.contact)
      assert.equal(
        await a.eval(
          "document.querySelector('[data-sync-kind=contact]').disabled"
        ),
        true
      );
    report.wecom =
      "Settings/filter mutation tests run only without enterprise configuration; existing credentials are preserved and their live checks skipped. No intentional live upstream call from this regression.";
  });
  let peerId, groupId, channelId, enterpriseFixtures, c;
  await step("企业通讯录替代好友申请、跨部门与同名工号查询", async () => {
    const token = await a.eval(`localStorage.getItem('biztoken')`),
      peer = {
        username: run.toLowerCase(),
        password: randomBytes(15).toString("base64url"),
      };
    await host(
      "/admin/system/account/save",
      {
        ...peer,
        password: md5(peer.password),
        name: run + "通讯测试",
        roleid: [1],
        dept_id: 1,
        tel: "",
        mobile: "",
        email: "",
        remark: "Local browser acceptance fixture",
      },
      token
    );
    enterpriseFixtures = await createEnterpriseIMFixtures(run);
    b = await browser(peer);
    peerId = (await b.api("/access")).uid;
    report.fixtures.peerUser = peerId;
    await a.go(prefix + "im/friends");
    await a.until("location.pathname.endsWith('/im/directory') && !!document.querySelector('[name=directory_search]')");
    await a.healthy();
    assert.equal((await a.api("/im/friends") || []).length, 0, "new QA users must not acquire fabricated friends");
    const lookup = async query => a.api("/im/directory/users?" + new URLSearchParams(query));
    const same = await lookup({q:run + "同名同事"});
    assert.deepEqual(same.items.map(row=>Number(row.user_id)).sort((x,y)=>x-y), [peerId,enterpriseFixtures.ids.third].sort((x,y)=>x-y));
    const exact = await lookup({q:peer.username});
    assert.ok(exact.items.some(row=>Number(row.user_id)===peerId && row.employee_no===enterpriseFixtures.entries.find(entry=>entry.key==='peer').username));
    assert.ok(!(await lookup({q:String(peerId)})).items.some(row=>Number(row.user_id)===peerId), "a hidden local ID must not match the employee name/number search");
    const tree = await lookup({q:run,department_id:enterpriseFixtures.ids.main,include_subdepartments:true});
    assert.ok(tree.items.some(row=>Number(row.user_id)===peerId));
    assert.ok(!tree.items.some(row=>Number(row.user_id)===enterpriseFixtures.ids.third));
    const direct = await lookup({q:run,department_id:enterpriseFixtures.ids.main,include_subdepartments:false});
    assert.ok(!direct.items.some(row=>Number(row.user_id)===peerId));
    for (const key of ["disabled","noim","unbound"]) {
      const entry=enterpriseFixtures.entries.find(item=>item.key===key);
      const found=(await lookup({q:entry.username})).items.find(row=>row.employee_no===entry.username);
      assert.ok(found && !found.can_chat && found.unavailable_reason, key + " must remain visible but not callable");
      assert.ok(found.account_status, key + " must have an explicit account status");
      assert.ok(!Object.keys(found).some(field=>/mobile|email|raw_json|secret/i.test(field)));
    }
    const removed = await a.raw("/im/friend-requests", {receiver_id:peerId});
    assert.equal(removed.status,410);
    await a.fill({directory_search:run + "同名同事"});
    await a.click("查询");
    await a.until(`document.querySelectorAll('.directory-person').length===2`);
    await a.eval(`document.querySelector('button[name=directory_department][value="${enterpriseFixtures.ids.main}"]').click()`);
    await a.until(`document.querySelectorAll('.directory-person').length===1 && !!document.querySelector('.directory-person[data-user-id="${peerId}"]')`);
    await a.eval("document.querySelector('button[name=directory_department][value=\"\"]').click()");
    await a.until(`document.querySelectorAll('.directory-person').length===2`);
    await a.shot("13-enterprise-directory");
    await a.click("发起聊天", `document.querySelector('.directory-person[data-user-id="${peerId}"]')`);
    await a.until(`document.querySelector('.chat-head>strong')?.textContent.includes('${run}同名同事')`);
  });
  async function openChat(page, kind, id) {
    await page.eval(
      `(async()=>{const {navigate}=await import('${prefix}assets/core.js');navigate('im/chat?kind=${kind}&id=${id}');})()`
    );
    await page.until(
      `!!document.querySelector('.messages') && document.querySelector('.chat-head')?.textContent.includes('实时连接')`
    );
  }
  async function sendMessage(page, content) {
    await page.fill({ message: content });
    await page.eval(`document.querySelector('.composer').requestSubmit()`);
    await page.until(
      `document.querySelector('.messages')?.textContent.includes(${JSON.stringify(
        content
      )})`
    );
  }
  await step("双会话私聊、WebSocket 推送、已读回执与撤回", async () => {
    await openChat(a, "private", peerId);
    await b.nav("im/chat");
    await b.until("document.querySelector('.conversation-heading')?.textContent.includes('实时连接')");
    await b.click("发起群聊");
    await b.until("!!document.querySelector('dialog [name=group_name]')");
    await b.click("×", "document.querySelector('dialog')");
    await b.click("发起聊天");
    await b.until("location.pathname.endsWith('/im/directory') && !!document.querySelector('[name=directory_search]')");
    await b.nav("im/chat");
    await b.until("document.querySelector('.conversation-heading')?.textContent.includes('实时连接')");
    await sendMessage(a, run + "实时消息");
    await b.until(`document.querySelector('.conversations')?.textContent.includes('${run}实时消息')`);
    assert.ok(b.wsFrames.some(x=>x.includes(run+"实时消息")), "an empty chat page must receive the first private message");
    await openChat(b, "private", testAdmin.uid);
    await b.until(
      `document.querySelector('.messages')?.textContent.includes('${run}实时消息')`
    );
    assert.ok(
      b.wsFrames.some((x) => x.includes(run + "实时消息")),
      "message must arrive over WebSocket"
    );
    const msg = (
      await a.api("/im/private/messages?receiver_id=" + peerId)
    ).find((x) => x.content === run + "实时消息");
    report.fixtures.privateMessage = msg.id;
    await a.until(
      `(async()=>{const {api}=await import('${prefix}assets/core.js');return (await api('/im/messages/${msg.id}/receipts')).some(x=>Number(x.user_id)===${peerId});})()`
    );
    await a.click("回执");
    await a.until(
      `document.querySelector('dialog')?.textContent.includes('阅读时间') && document.querySelector('dialog')?.textContent.includes('${run}同名同事')`
    );
    await a.click("×", `document.querySelector('dialog')`);
    await a.click("撤回");
    await b.until(
      `document.querySelector('.messages')?.textContent.includes('撤回')`
    );
    await a.shot("06-im-private");
  });
  await step("聊天文件上传、图片发送和宿主附件删除", async () => {
    const filename = join(output, run + "-upload.png");
    await writeFile(filename, await readFile(join(output, "02-employee.png")));
    const { root: dom } = await a.send("DOM.getDocument");
    const { nodeId } = await a.send("DOM.querySelector", {
      nodeId: dom.nodeId,
      selector: ".composer input[type=file]",
    });
    await a.send("DOM.setFileInputFiles", { nodeId, files: [filename] });
    await a.until(
      `document.querySelector('[name=message]').value.includes('/resource/uploads/')`
    );
    const url = await a.eval(`document.querySelector('[name=message]').value`);
    assert.equal(new URL(url, base).origin, base);
    await a.eval(`document.querySelector('.composer').requestSubmit()`);
    await b.until(`!!document.querySelector('.messages img')?.naturalWidth`);
    const token = await a.eval(`localStorage.getItem('biztoken')`);
    const items = (
      await host(
        "/admin/datacenter/attachment/getMyFiles?filetype=image&searchword=" +
          run,
        undefined,
        token
      )
    ).items;
    const file = items.find((x) => x.url === url || base + x.url === url);
    assert.ok(file);
    report.fixtures.attachment = file.id;
    await host(
      "/admin/datacenter/attachment/del",
      { ids: [Number(file.id)] },
      token,
      "DELETE"
    );
    assert.equal((await fetch(new URL(url, base))).status, 404);
    // Exercise the host's existing PDF allowlist without changing upload policy.
    const documentFile = join(output, run + "-attachment.pdf");
    const stream = `BT /F1 12 Tf 30 80 Td (Enterprise IM file acceptance ${run}) Tj ET\n`;
    const objects = [
      '<< /Type /Catalog /Pages 2 0 R >>',
      '<< /Type /Pages /Kids [3 0 R] /Count 1 >>',
      '<< /Type /Page /Parent 2 0 R /MediaBox [0 0 600 120] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>',
      `<< /Length ${Buffer.byteLength(stream)} >>\nstream\n${stream}endstream`,
      '<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>',
    ];
    let pdf = '%PDF-1.4\n'; const offsets = [0];
    for (const [index, object] of objects.entries()) { offsets.push(Buffer.byteLength(pdf)); pdf += `${index + 1} 0 obj\n${object}\nendobj\n`; }
    const xref = Buffer.byteLength(pdf);
    pdf += `xref\n0 6\n0000000000 65535 f \n${offsets.slice(1).map(offset=>String(offset).padStart(10,'0')+' 00000 n \n').join('')}trailer\n<< /Size 6 /Root 1 0 R >>\nstartxref\n${xref}\n%%EOF\n`;
    await writeFile(documentFile, pdf);
    await a.send("DOM.setFileInputFiles", { nodeId, files: [documentFile] });
    await a.until(`document.querySelector('[name=message]').value.includes('/resource/uploads/')`);
    const fileURL = await a.eval("document.querySelector('[name=message]').value");
    assert.notEqual(fileURL, url);
    await a.eval("document.querySelector('.composer').requestSubmit()");
    await b.until(`[...document.querySelectorAll('.messages a')].some(link=>link.href===${JSON.stringify(new URL(fileURL, base).href)})`);
    const documents = (await host("/admin/datacenter/attachment/getMyFiles?filetype=file&searchword=" + run, undefined, token)).items;
    const document = documents.find(item => item.url === fileURL || base + item.url === fileURL);
    assert.ok(document, "uploaded file must use host storage");
    try {
      assert.equal((await fetch(new URL(fileURL, base))).status, 200);
      const delivered = (await b.api("/im/private/messages?receiver_id=" + testAdmin.uid)).find(item=>item.content===fileURL);
      assert.equal(delivered?.message_type, "file");
      assert.ok(b.wsFrames.some(frame=>frame.includes(fileURL)), "file message must arrive through WebSocket");
    } finally {
      await host("/admin/datacenter/attachment/del", {ids:[Number(document.id)]}, token, "DELETE");
    }
  });
  await step("群组创建、加入、群聊与敏感词过滤", async () => {
    await a.nav("im/sensitive-words");
    await a.create("+ 新增敏感词", {
      word: run + "敏感",
      replacement: "[过滤]",
    });
    await a.nav("im/groups");
    // Keep the legacy open-group API contract covered; the new colleague
    // picker is exercised separately below with its default approval policy.
    await a.api("/im/groups", {
      name: run + "测试群",
      notice: "测试完成后清理",
      join_approval: 0,
    });
    groupId = (await a.api("/im/groups")).find(
      (x) => x.name === run + "测试群"
    ).id;
    report.fixtures.imGroup = groupId;
    await b.nav("im/groups");
    await b.searchRecords(run, false);
    await b.click(
      "加入",
      `[...document.querySelectorAll('tbody tr')].find(x=>x.textContent.includes('${run}'))`
    );
    await b.until(
      `(async()=>{const {api}=await import('${prefix}assets/core.js');return (await api('/im/groups/${groupId}/members')).some(x=>Number(x.user_id)===${peerId});})()`
    );
    await openChat(a, "group", groupId);
    await openChat(b, "group", groupId);
    await a.fill({ message: run + "敏感 消息" });
    await a.eval(`document.querySelector('.composer').requestSubmit()`);
    await b.until(
      `document.querySelector('.messages')?.textContent.includes('[过滤] 消息')`
    );
    assert.ok(b.wsFrames.some((x) => x.includes("[过滤] 消息")));
    await b.shot("07-im-group");
  });
  await step("勾选同事原子建群、空会话实时通知与越权拒绝", async () => {
    c = await browser(enterpriseFixtures.third);
    await c.nav("im/chat");
    await c.until("document.querySelector('.conversation-heading')?.textContent.includes('实时连接')");
    assert.equal((await c.raw(`/im/groups/${groupId}/messages`)).status,403);
    assert.equal((await c.raw(`/im/messages/${report.fixtures.privateMessage}/receipts`)).status,403);
    assert.ok(!(await c.api('/im/private/messages?receiver_id='+peerId) || []).some(row=>row.id===report.fixtures.privateMessage));
    const invalid = await a.raw("/im/groups", {name:run+"无效群",join_approval:1,member_ids:[peerId,enterpriseFixtures.ids.disabled]});
    assert.ok(invalid.status>=400);
    assert.equal((await a.api('/im/groups')).some(row=>row.name===run+"无效群"),false);
    await a.nav("im/directory");
    await a.fill({directory_search:run+"同名同事"});
    await a.click("查询");
    for (const id of [peerId,enterpriseFixtures.ids.third]) {
      await a.until(`!!document.querySelector('input[data-member-id="${id}"]')`);
      await a.eval(`document.querySelector('input[data-member-id="${id}"]').click()`);
    }
    await a.eval("[...document.querySelectorAll('#view button')].find(b=>b.textContent.startsWith('发起群聊')).click()");
    await a.until("!!document.querySelector('dialog [name=group_name]')");
    await a.fill({group_name:run+"企业群",group_notice:"临时企业群验收"},"document.querySelector('dialog')");
    await a.save();
    const group=(await a.api('/im/groups')).find(row=>row.name===run+"企业群");
    assert.ok(group?.id);
    assert.equal(Number(group.join_approval),1);
    report.fixtures.enterpriseGroup=group.id;
    assert.deepEqual((await a.api(`/im/groups/${group.id}/members`)).map(row=>Number(row.user_id)).sort((x,y)=>x-y),[testAdmin.uid,peerId,enterpriseFixtures.ids.third].sort((x,y)=>x-y));
    await c.until(`document.querySelector('.conversations')?.textContent.includes('${run}企业群')`);
    assert.ok(c.wsFrames.some(frame=>frame.includes(group.id)),"new group must be discovered through a realtime event without any message");
    const conversation=(await c.api('/im/conversations')).find(row=>row.conversation_id===group.id);
    assert.ok(Number(conversation.change_seq)>0 && Number(conversation.unread_count)===0);
    await openChat(a,"group",group.id);
    await openChat(c,"group",group.id);
    await a.until(`document.querySelector('.chat-head>strong')?.textContent==='${run}企业群'`);
    await sendMessage(a,run+"企业群首条消息");
    await c.until(`document.querySelector('.messages')?.textContent.includes('${run}企业群首条消息')`);
    await a.shot("14-enterprise-group");
  });
  await step("无好友屏蔽与恢复、停用后本人历史保留", async () => {
    await openChat(a,"private",peerId);
    await sendMessage(a,run+"保留历史");
    await a.click("屏蔽");
    await a.until("!!document.querySelector('.chat-restriction')?.textContent && document.querySelector('.composer [name=message]')?.disabled===true");
    assert.ok((await a.api('/im/private/messages?receiver_id='+peerId)).some(row=>row.content===run+"保留历史"));
    assert.equal((await b.raw('/im/private/messages',{receiver_id:testAdmin.uid,client_message_id:run+'-blocked',content:'must not send'})).status,403);
    assert.equal((await b.raw(`/im/friends/${testAdmin.uid}/action`,{action:'delete'})).status,410);
    assert.equal((await b.api('/im/private/peer?receiver_id='+testAdmin.uid)).can_chat,false);
    await a.shot('15-blocked-history');
    await a.click('恢复联系');
    await a.until("document.querySelector('.composer [name=message]')?.disabled===false");
    await sendMessage(a,run+"恢复后发送");
    const historical = await a.api('/im/private/messages',{receiver_id:enterpriseFixtures.ids.third,client_message_id:run+'-inactive-history',content:run+'停用后可读'});
    // Stop only this run's third browser page before disabling its QA account.
    await c.send('Page.navigate',{url:'about:blank'});
    const token=await a.eval("localStorage.getItem('biztoken')");
    try {
      await host('/admin/system/account/upStatus',{id:enterpriseFixtures.ids.third,status:1},token);
      const peer=await a.api('/im/private/peer?receiver_id='+enterpriseFixtures.ids.third);
      assert.equal(peer.can_chat,false);
      assert.equal(peer.can_read,true);
      assert.ok((await a.api('/im/private/messages?receiver_id='+enterpriseFixtures.ids.third)).some(row=>row.id===historical.id));
      assert.equal((await a.raw('/im/private/messages',{receiver_id:enterpriseFixtures.ids.third,client_message_id:run+'-inactive-reject',content:'must not send'})).status,403);
    } finally {
      await host('/admin/system/account/upStatus',{id:enterpriseFixtures.ids.third,status:0},token);
    }
    report.enterpriseIM={noFriendRequired:true,emptyChatRealtime:true,departmentTree:true,selectedGroup:true,blockAndHistory:true,disabledHistory:true};
  });
  await step("频道发布、成员订阅与发言权限", async () => {
    await a.nav("im/channels");
    await a.create("+ 创建频道", {
      name: run + "频道",
      description: "验收频道",
    });
    channelId = (await a.api("/im/channels")).find(
      (x) => x.name === run + "频道"
    ).id;
    report.fixtures.channel = channelId;
    await b.nav("im/channels");
    await b.searchRecords(run, false);
    await b.click(
      "订阅",
      `[...document.querySelectorAll('tbody tr')].find(x=>x.textContent.includes('${run}'))`
    );
    await openChat(a, "channel", channelId);
    await openChat(b, "channel", channelId);
    await sendMessage(a, run + "频道公告");
    await b.until(
      `document.querySelector('.messages')?.textContent.includes('${run}频道公告')`
    );
    assert.equal(await b.eval(`!!document.querySelector('.composer')`), false);
    assert.ok(b.wsFrames.some((x) => x.includes(run + "频道公告")));
  });
  await step("表情包、管理统计与 RTC 未配置提示", async () => {
    await a.nav("im/face-packs");
    await a.create("+ 新增表情包", {
      name: run + "表情",
      cover_url: "/favicon.ico",
      item_name: "图标",
      item_url: "/favicon.ico",
    });
    assert.ok(
      (await a.api("/im/face-packs")).some((x) => x.name === run + "表情")
    );
    await a.nav("im/manager");
    await a.healthy();
    await a.shot("08-im-statistics");
    await openChat(a, "private", peerId);
    await a.click("音视频通话");
    await a.until(
      `document.querySelector('dialog')?.textContent.includes('尚未配置音视频服务')`
    );
    await a.click("×", `document.querySelector('dialog')`);
    report.rtc =
      "LiveKit absent: configuration prompt tested; no media call made";
  });
  await step("匿名 API 和伪造身份拒绝、Cookie 写入 CSRF 防护", async () => {
    const res = await fetch(
      base + "/plugins/biz-suite/api/admin/hrm/employees",
      { headers: { "X-Suxin-Plugin-User": '{"uid":1}' } }
    );
    assert.equal(res.status, 401);
    const status = await a.eval(
      `(async()=>{const r=await fetch('/plugins/biz-suite/api/admin/hrm/employees',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({name:'must-not-save'})});await r.text();return r.status;})()`
    );
    assert.equal(status, 403);
    await a.healthy();
  });
  await step("已读回执没有重复广播或循环请求", async () => {
    await a.healthy();
    await b.healthy();
    const before = a.wsFrames.length + b.wsFrames.length;
    await wait(1000);
    assert.equal(
      a.wsFrames.length + b.wsFrames.length,
      before,
      "idle chats must not keep broadcasting receipts"
    );
    for (const page of [a, b]) {
      const receipts = page.wsFrames
        .map((x) => JSON.parse(x))
        .filter((x) => x.type === "receipt")
        .map((x) =>
          JSON.stringify([
            x.data.user_id,
            x.data.stream_id,
            x.data.last_read_seq,
          ])
        );
      assert.equal(
        new Set(receipts).size,
        receipts.length,
        "same read cursor was broadcast repeatedly"
      );
    }
  });
  await step("智慧HRM与企业即时通讯独立入口、单侧栏及旧入口兼容", async () => {
    const token = await a.eval(`localStorage.getItem('biztoken')`);
    const menu = await host("/admin/user/getMenu", undefined, token);
    const allMenus = [];
    const collect = (items) => { for (const item of items || []) { allMenus.push(item); collect(item.children); } };
    collect(menu);
    for (const [area, title] of [["hrm", "智慧HRM"], ["im", "企业即时通讯"]]) {
      const item = allMenus.find(item => item.path === `/runtime-plugins/biz-suite/${area}`);
      assert.equal(item?.meta.title, title);
      assert.equal(item?.meta.onlypage, true);
    }
    await a.go("/suxinweb/");
    await a.until("[...document.querySelectorAll('.arco-menu-item')].some(x=>x.textContent.trim()==='智慧HRM')");
    for (const [area, title, path, heading] of [["hrm", "智慧HRM", "hrm/overview", "智慧HRM"], ["im", "企业即时通讯", "im/chat", "消息中心"]]) {
      const opened = await a.openMenu(title);
      await opened.until(`location.pathname===${JSON.stringify(prefix + path)} && document.querySelector('#view h1')?.textContent===${JSON.stringify(heading)}`);
      await opened.healthy();
      assert.equal(await opened.eval("document.querySelectorAll('iframe').length"), 0);
      assert.equal(await opened.eval("document.querySelectorAll('.sidebar').length"), 1);
      assert.equal(await opened.eval("document.querySelector('.brand strong')?.textContent"), title);
      const paths = await opened.eval("[...document.querySelectorAll('.sidebar [data-route]')].map(x=>x.dataset.route)");
      assert.ok(paths.length > 0);
      assert.ok(paths.every(path => area === "im" ? path.startsWith("im/") : /^(hrm|wecom)\//.test(path)));
      await opened.shot("09-independent-" + area);
    }
    // Existing bookmarks keep their authenticated entry and canonical landing.
    await a.go("/suxinweb/runtime-plugins/biz-suite");
    await a.until(
      `location.pathname==='${prefix}hrm/overview' && document.querySelector('#view h1')?.textContent==='智慧HRM'`
    );
    assert.equal(await a.eval("document.querySelectorAll('iframe').length"), 0);
    await a.healthy();
    await a.shot("09-host-plugin-entry");
  });
  await step("即时通讯页面会话保活与离开停止", async () => {
    const before = a.http.filter(item => item.path.endsWith("/im/session") && item.status === 200).length;
    await a.nav("im/chat");
    for (let attempt = 0; attempt < 100 && a.http.filter(item => item.path.endsWith("/im/session") && item.status === 200).length <= before; attempt++) await wait(100);
    assert.ok(a.http.filter(item => item.path.endsWith("/im/session") && item.status === 200).length > before, "opening IM must verify and maintain its session");
    const session = await a.api("/im/session");
    assert.equal(Number(session.uid), Number(testAdmin.uid));
    assert.ok(Array.isArray(session.permissions));
    assert.ok(!Object.keys(session).some(key => /token|secret|password/i.test(key)));
    await a.nav("hrm/overview");
    const leaving = a.http.filter(item => item.path.endsWith("/im/session")).length;
    await a.eval("window.dispatchEvent(new Event('focus')); window.dispatchEvent(new Event('online'))");
    await wait(300);
    assert.equal(a.http.filter(item => item.path.endsWith("/im/session")).length, leaving, "HRM must not keep IM session heartbeats running");
    report.sessionLifetime.imHeartbeat = true;
    report.sessionLifetime.stopsOutsideIM = true;
  });
  await step("宿主角色网页新增、编辑、删除及表单状态重置", async () => {
    const name = run + "网页角色", editedName = name + "已编辑";
    const token = await a.eval("localStorage.getItem('biztoken')");
    const readRoles = async () => {
      const response = await host("/admin/system/role/getList?name=" + run, undefined, token);
      const flatten = (rows) => (rows || []).flatMap((row) => [row, ...flatten(row.children)]);
      return flatten(response.list);
    };
    // Role tree cells prepend a branch glyph before the actual role name.
    const row = (value) => `[...document.querySelectorAll('.arco-table-tr')].find(r=>[...r.querySelectorAll('td')].some(c=>c.textContent.trim().endsWith(${JSON.stringify(value)})))`;
    // The host keeps its settings drawer mounted. Anchor all operations to the
    // visible role field instead of the first (possibly hidden) drawer.
    const drawer = `([...document.querySelectorAll('.arco-drawer input[placeholder="请填写角色名称"]')].find(input=>input.getClientRects().length&&getComputedStyle(input).visibility!=='hidden')?.closest('.arco-drawer'))`;
    const fillDrawer = async (values) => {
      await a.eval(`(()=>{const drawer=${drawer};if(!drawer)throw Error('Visible role form missing');for(const [placeholder,value] of Object.entries(${JSON.stringify(values)})){const input=drawer.querySelector('[placeholder="'+placeholder+'"]');if(!input)throw Error('Role field missing');input.value=value;input.dispatchEvent(new Event('input',{bubbles:true}));}})()`);
    };
    const saveDrawer = async () => {
      await a.until(`!![...(${drawer}?.querySelectorAll('.arco-drawer-footer button')||[])].find(b=>/^(确定|确认)$/.test(b.textContent.trim())&&!b.disabled)`);
      await a.eval(`[...${drawer}.querySelectorAll('.arco-drawer-footer button')].find(b=>/^(确定|确认)$/.test(b.textContent.trim())).click()`);
      await a.until(`!${drawer}`);
      await a.healthy();
    };
    await a.go("/suxinweb/system/role");
    await a.until("!!document.querySelector('input[placeholder=\"搜索角色名称\"]')");
    await a.healthy();
    await a.eval("[...document.querySelectorAll('.table-toolbar button')].find(b=>/^(新建|新增)$/.test(b.textContent.trim())).click()");
    await a.until(`!!${drawer}`);
    await a.healthy();
    assert.ok((await a.eval(`${drawer}.textContent`)).includes("上级角色"));
    await fillDrawer({ "请填写角色名称": name, "请填写备注": run + "角色创建验证" });
    await saveDrawer();
    const created = (await readRoles()).find((item) => item.name === name);
    assert.ok(Number(created?.id) > 1, "new role must be persisted with a distinct ID");
    report.fixtures.browserRole = Number(created.id);
    await a.until(`!!${row(name)}?.querySelector('.iconbtn')`);
    await a.eval(`${row(name)}.querySelectorAll('.iconbtn')[0].dispatchEvent(new MouseEvent('click',{bubbles:true}))`);
    await a.until(`!!${drawer}`);
    await a.healthy();
    assert.equal(await a.eval(`${drawer}.querySelector('input[placeholder="请填写角色名称"]').value`), name);
    await fillDrawer({ "请填写角色名称": editedName, "请填写备注": run + "角色编辑验证" });
    await saveDrawer();
    const updated = (await readRoles()).find((item) => Number(item.id) === Number(created.id));
    assert.equal(updated?.name, editedName);
    assert.equal(updated?.remark, run + "角色编辑验证");
    await a.eval("[...document.querySelectorAll('.table-toolbar button')].find(b=>/^(新建|新增)$/.test(b.textContent.trim())).click()");
    await a.until(`!!${drawer}`);
    await a.healthy();
    assert.equal(await a.eval(`${drawer}.querySelector('input[placeholder="请填写角色名称"]').value`), "", "new form must not reuse the edited role");
    await a.eval(`[...${drawer}.querySelectorAll('.arco-drawer-footer button')].find(b=>b.textContent.trim()==='取消').click()`);
    await a.until(`!${drawer}`);
    await a.until(`!!${row(editedName)}?.querySelectorAll('.iconbtn')[1]`);
    await a.eval(`${row(editedName)}.querySelectorAll('.iconbtn')[1].dispatchEvent(new MouseEvent('click',{bubbles:true}))`);
    await a.until("!![...document.querySelectorAll('.arco-popconfirm-footer button')].find(b=>/^(确定|确认)$/.test(b.textContent.trim()))");
    await a.eval("[...document.querySelectorAll('.arco-popconfirm-footer button')].find(b=>/^(确定|确认)$/.test(b.textContent.trim())).click()");
    await a.until(`!${row(editedName)}`);
    await a.healthy();
    assert.equal((await readRoles()).some((item) => Number(item.id) === Number(created.id)), false);
    await a.go(prefix + "hrm/employees");
    await a.until("!!document.querySelector('#view .subheading')");
    await a.healthy();
  });
  let organizationFixtures;
  await step("员工映射显示停用账号、工号登录名和本地部门完整路径", async () => {
    organizationFixtures = await createOrganizationScopeFixtures(run);
    report.fixtures.scopeEmployees = organizationFixtures.employees.map((employee) => employee.employeeId);
    report.fixtures.scopeDepartments = organizationFixtures.departments;
    const disabled = organizationFixtures.employees.find((item) => item.key === "disabled");
    const child = organizationFixtures.employees.find((item) => item.key === "child");
    await a.nav("hrm/employees");
    await a.searchRecords(disabled.username);
    const record = (await a.api("/hrm/employees?search=" + disabled.username))[0];
    assert.equal(record?.id, disabled.employeeId);
    assert.equal(record.host_username, disabled.username);
    assert.equal(record.host_display_name, disabled.name);
    assert.equal(record.mapping_status, "disabled");
    const text = await a.eval("document.querySelector('table').textContent");
    for (const value of [disabled.name, disabled.username, run + "权限主部门", "账号停用"])
      assert.ok(text.includes(value), "disabled employee identity must be readable");
    assert.doesNotMatch(text, /宿主账号 ID|部门 ID|企业微信 userid/);
    await a.click("编辑", "document.querySelector('tbody tr')");
    await a.until("!!document.querySelector('dialog [name=user_id]')");
    assert.ok((await a.eval("document.querySelector('dialog [name=user_id]').selectedOptions[0].textContent")).includes(disabled.username));
    assert.ok((await a.eval("document.querySelector('dialog [name=user_id]').selectedOptions[0].textContent")).includes("停用"));
    await a.click("×", "document.querySelector('dialog')");
    await a.searchRecords(child.username);
    const path = run + "权限主部门 / " + run + "权限下级部门";
    assert.ok((await a.eval("document.querySelector('tbody').textContent")).includes(path));
    await a.shot("10-readable-organization-mapping");
    // Deliberately do not click the real organization sync action. These
    // fixtures exist only to verify the projection and authorization boundary.
  });
  await step("本人和部门角色真实登录、数据范围与只读权限隔离", async () => {
    assert.ok(organizationFixtures);
    const own = organizationFixtures.employees.find((item) => item.key === "self");
    const outside = organizationFixtures.employees.find((item) => item.key === "outside");
    const raw = async (page, path, body) => page.eval(`(async()=>{
      const response=await fetch('/plugins/biz-suite/api/admin'+${JSON.stringify(path)},{credentials:'include',headers:{'X-Requested-With':'XMLHttpRequest','Content-Type':'application/json'},method:${JSON.stringify(body === undefined ? "GET" : "POST")},${body === undefined ? "" : `body:JSON.stringify(${JSON.stringify(body)}),`}});
      const value=await response.json().catch(()=>({}));return {status:response.status,code:value.code,data:value.data};
    })()`);
    for (const [key, credentials, expected] of [
      ["self", organizationFixtures.self, [own.employeeId]],
      ["department", organizationFixtures.manager, organizationFixtures.employees.filter((item) => item.department !== "outside").map((item) => item.employeeId)],
    ]) {
      const scopePage = await browser(credentials);
      const access = await scopePage.api("/access");
      assert.deepEqual(access.permissions, ["biz-suite.hrm." + key]);
      const list = await scopePage.api("/hrm/employees?page_size=100");
      assert.deepEqual(list.map((item) => item.id).sort(), [...expected].sort(), "scope must be enforced on the server, not only by a browser filter");
      await scopePage.searchRecords(run);
      const text = await scopePage.eval("document.querySelector('tbody').textContent");
      assert.ok(text.includes(own.name));
      assert.equal(text.includes(outside.name), false);
      assert.equal(await scopePage.eval("[...document.querySelectorAll('#view button')].some(b=>/新增|编辑|同步组织与账号/.test(b.textContent))"), false);
      const routes = await scopePage.eval("[...document.querySelectorAll('.sidebar [data-route]')].map(link=>link.dataset.route)");
      assert.equal(routes.some((path) => path.startsWith("wecom/") || path.startsWith("im/") || /hrm\/(groups|holidays)$/.test(path)), false);
      assert.deepEqual((await scopePage.api("/hrm/options")).employees.map((item) => item.id).sort(), [...expected].sort());
      const clocks = await scopePage.api("/hrm/attendance/clocks?month=2026-10&page_size=100");
      assert.deepEqual(clocks.map((item) => item.employee_id).sort(), [...expected].sort());
      const leaves = await scopePage.api("/hrm/attendance/leaves?month=2026-10&page_size=100");
      assert.deepEqual(leaves.map((item) => item.employee_id).sort(), [...expected].sort());
      const clockOptions = await scopePage.api("/hrm/attendance/clocks/options");
      assert.deepEqual(clockOptions.employees.map((item) => item.id).sort(), [...expected].sort());
      assert.equal(clockOptions.departments.some((item) => item.key === "host:" + organizationFixtures.departments.outside), false);
      assert.deepEqual(await scopePage.api("/hrm/employees?search=" + outside.username), []);
      const detail = await raw(scopePage, "/hrm/attendance/leaves/" + outside.employeeId + "-leave");
      assert.ok(detail.status >= 400 || Number(detail.code) !== 0, "another department's approval detail must be denied");
      assert.ok(!detail.data, "denied details must not return another employee's fields");
      const deniedWrite = await raw(scopePage, "/hrm/employees", { id: own.employeeId, name: own.name, employee_no: own.username });
      assert.equal(deniedWrite.status, 403, "scoped read permission must not grant employee edits");
      for (const path of ["/wecom/settings", "/wecom/organization/sync"])
        assert.equal((await raw(scopePage, path)).status, 403);
      await scopePage.go(prefix + "hrm/groups");
      await scopePage.until("document.querySelector('#view h1')?.textContent==='无访问权限'");
      await scopePage.go(prefix + "wecom/status");
      await scopePage.until("document.querySelector('#view h1')?.textContent==='无访问权限'");
      await scopePage.go(prefix + "hrm/employees");
      await scopePage.until("!!document.querySelector('#view .subheading')");
      await scopePage.healthy();
      await scopePage.shot("11-scope-" + key);
    }
  });
  await step("停用账号网页设置密码、启用及新旧密码真实登录验证", async () => {
    assert.ok(organizationFixtures);
    const disabled = organizationFixtures.employees.find((item) => item.key === "disabled");
    assert.equal(disabled.username, run.toLowerCase() + "sd", "account edits must target only this run's isolated QA account");
    const nextPassword = randomBytes(24).toString("base64url");
    const token = await a.eval("localStorage.getItem('biztoken')");
    const readAccount = async () => {
      const result = await host("/admin/system/account/getList?page=1&pageSize=100&name=" + encodeURIComponent(disabled.name), undefined, token);
      return result.items.find((item) => item.username === disabled.username);
    };
    const before = await readAccount();
    assert.ok(Number(before?.id) > 1);
    assert.equal(before.remark, run + " isolated scope fixture");
    assert.equal(Number(before.status), 1);
    assert.equal(before.name, disabled.name);
    assert.equal(Number(before.dept_id), Number(organizationFixtures.departments.main));
    assert.deepEqual(before.rolename, [run + "scope-self"]);
    const row = `([...document.querySelectorAll('.arco-table-tr')].find(r=>[...r.querySelectorAll('td')].some(c=>c.textContent.trim()===${JSON.stringify(disabled.username)})))`;
    const modal = `([...document.querySelectorAll('.arco-modal input[placeholder="请填登录账号"]')].find(input=>input.getClientRects().length&&getComputedStyle(input).visibility!=='hidden')?.closest('.arco-modal'))`;
    await a.go("/suxinweb/system/account");
    await a.until("!!document.querySelector('input[placeholder=\"搜索用户名\"]')");
    await a.healthy();
    await a.eval(`(()=>{const input=document.querySelector('input[placeholder="搜索用户名"]');input.value=${JSON.stringify(disabled.name)};input.dispatchEvent(new Event('input',{bubbles:true}));})()`);
    await a.eval("[...document.querySelectorAll('button')].find(button=>/^(查询|搜索)$/.test(button.textContent.trim())&&button.getClientRects().length).click()");
    await a.until(`!!${row}?.querySelector('.iconbtn')`);
    await a.healthy();
    await a.eval(`${row}.querySelector('.iconbtn').dispatchEvent(new MouseEvent('click',{bubbles:true}))`);
    await a.until(`!!${modal}`);
    await a.healthy();
    assert.equal(await a.eval(`${modal}.querySelector('input[placeholder="请填用户名"]').value`), disabled.name);
    assert.equal(await a.eval(`${modal}.querySelector('input[placeholder="请填登录账号"]').value`), disabled.username);
    const formText = await a.eval(`${modal}.textContent`);
    for (const value of ["用户名", "登录账号", "选择角色", "选择部门", run + "scope-self", run + "权限主部门", "编辑时留空保留原密码"])
      assert.ok(formText.includes(value), "account editor must show readable identity, role, department and password guidance");
    assert.equal(await a.eval(`${modal}.querySelector('input[placeholder="留空保留原密码"]')?.type`), "password");
    assert.equal(await a.eval(`${modal}.querySelector('input[placeholder="留空保留原密码"]').value.length`), 0, "existing password must never be exposed by the form");
    await a.eval(`(()=>{const input=${modal}.querySelector('input[placeholder="留空保留原密码"]');input.value=${JSON.stringify(nextPassword)};input.dispatchEvent(new Event('input',{bubbles:true}));input.dispatchEvent(new Event('change',{bubbles:true}));})()`);
    await a.until(`!![...(${modal}?.querySelectorAll('.arco-modal-footer button')||[])].find(button=>/^(确认|确定)$/.test(button.textContent.trim())&&!button.disabled)`);
    await a.eval(`[...${modal}.querySelectorAll('.arco-modal-footer button')].find(button=>/^(确认|确定)$/.test(button.textContent.trim())).click()`);
    await a.until(`!${modal}`);
    await a.healthy();
    const saved = await readAccount();
    assert.equal(Number(saved?.id), Number(before.id));
    assert.equal(Number(saved.status), 1, "setting a password must leave a disabled account disabled");
    assert.equal(saved.name, disabled.name);
    assert.equal(Number(saved.dept_id), Number(before.dept_id));
    assert.deepEqual(saved.roleid, before.roleid);
    await a.until(`!!${row}?.querySelector('.arco-switch')`);
    assert.equal(await a.eval(`${row}.querySelector('.arco-switch').getAttribute('aria-checked')`), "false");
    await a.eval(`${row}.querySelector('.arco-switch').click()`);
    await a.until("[...document.querySelectorAll('.arco-message-content')].some(item=>item.textContent.includes('更新状态成功'))");
    await a.healthy();
    assert.equal(Number((await readAccount()).status), 0, "the explicit table switch must enable only the selected QA account");
    await browser(organizationFixtures.disabled, { expectedLoginError: "密码不正确" });
    const enabledPage = await browser({ username: disabled.username, password: nextPassword });
    assert.equal(Number((await enabledPage.api("/access")).uid), Number(before.id));
    assert.deepEqual((await enabledPage.api("/hrm/employees")).map((item) => item.id), [disabled.employeeId]);
    await enabledPage.shot("12-enabled-qa-account");
    report.accountLifecycle = { passwordFormMasked: true, saveKeptDisabled: true, explicitActivation: true, oldPasswordRejected: true, newPasswordLogin: true };
    await a.go(prefix + "hrm/employees");
    await a.until("!!document.querySelector('#view .subheading')");
    await a.healthy();
  });
  await step("退出宿主后插件会话立即失效", async () => {
    await a.nav("im/directory");
    await wait(1100);
    const token = await a.eval(`localStorage.getItem('biztoken')`);
    await host("/admin/user/logout", {}, token);
    await a.eval("window.dispatchEvent(new Event('focus'))");
    await a.until("!!document.querySelector('[data-session-state=expired]')");
    assert.ok(await a.eval("!!document.querySelector('[data-session-state=expired] a[href^=\"/suxinweb/login\"]')"));
    assert.equal(
      await a.eval(
        `(async()=>{const r=await fetch('/plugins/biz-suite/api/admin/access');await r.text();return r.status;})()`
      ),
      401
    );
    assert.equal(await a.eval("(async()=>{const r=await fetch('/plugins/biz-suite/api/admin/im/session');await r.text();return r.status;})()"), 401);
    report.sessionLifetime.logoutStopsIM = true;
    await a.go(prefix + "hrm/employees");
    await a.until(`!!document.querySelector('input[type=password]')`);
  });
  await step("浏览器无未捕获 JavaScript 异常", async () => {
    for (const p of pages) assert.deepEqual(p.errors, []);
  });
} catch (error) {
  if (pages[0]) await pages[0].shot("failure").catch(() => {});
  console.error(error.message);
  process.exitCode = 1;
} finally {
  // Stop test browsers before deleting their fixtures so background polls cannot
  // recreate audit rows. Temporary profiles contain only this run's QA sessions.
  for (const page of pages) page.socket.close();
  for (const proc of processes) proc.kill();
  if (processes.length) await wait(300);
  try {
    await cleanupRun(run);
    report.fixturesCleaned = true;
  } catch (error) {
    report.cleanupError = error.message;
    process.exitCode = 1;
  }
  report.finishedAt = new Date().toISOString();
  await writeFile(join(output, "report.json"), JSON.stringify(report, null, 2));
  await writeFile(join(output, run + ".json"), JSON.stringify(report, null, 2));
  for (const profile of profiles) {
    assert.equal(dirname(resolve(profile)), resolve(tmpdir()));
    assert.ok(basename(profile).startsWith("biz-suite-browser-"));
    await rm(profile, { recursive: true, force: true, maxRetries: 6, retryDelay: 200 }).catch(() => {
      report.profileCleanupFailed = true;
      process.exitCode = 1;
    });
  }
  if (report.profileCleanupFailed) {
    await writeFile(join(output, "report.json"), JSON.stringify(report, null, 2));
    await writeFile(join(output, run + ".json"), JSON.stringify(report, null, 2));
  }
  console.log("Report: " + join(output, "report.json"));
}
