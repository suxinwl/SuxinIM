import test from "node:test";
import assert from "node:assert/strict";
import { createServer } from "node:http";
import { readFile, mkdtemp } from "node:fs/promises";
import { existsSync } from "node:fs";
import { resolve, join } from "node:path";
import { tmpdir } from "node:os";
import { spawn } from "node:child_process";

const root = resolve(import.meta.dirname, "..");
const base = "/plugins/biz-suite/admin/";
const api = "/plugins/biz-suite/api/admin/im/manager/openim";
const delay = ms => new Promise(resolve => setTimeout(resolve, ms));

class CDP {
  constructor(socket) {
    this.socket = socket;
    this.id = 0;
    this.pending = new Map();
    this.errors = [];
    socket.addEventListener("message", event => {
      const result = JSON.parse(event.data);
      if (result.id) {
        const pending = this.pending.get(result.id);
        this.pending.delete(result.id);
        if (result.error) pending.reject(new Error(result.error.message));
        else pending.resolve(result.result);
      }
      if (result.method === "Runtime.exceptionThrown") this.errors.push(result.params.exceptionDetails);
    });
  }
  send(method, params = {}) {
    return new Promise((resolve, reject) => {
      const id = ++this.id;
      this.pending.set(id, { resolve, reject });
      this.socket.send(JSON.stringify({ id, method, params }));
    });
  }
  async eval(expression) {
    const result = await this.send("Runtime.evaluate", { expression, awaitPromise: true, returnByValue: true });
    if (result.exceptionDetails) throw new Error(result.exceptionDetails.exception?.description || result.exceptionDetails.text);
    return result.result.value;
  }
  async wait(expression) {
    for (let i = 0; i < 100; i++) {
      if (await this.eval(expression)) return;
      await delay(25);
    }
    throw new Error("Timed out: " + expression);
  }
}

test("OpenIM manager keeps one set of sections across overlapping refreshes and route changes", { timeout: 60000 }, async t => {
  let revision = 1;
  const holdNext = new Set();
  const held = [];
  const requests = [];
  const fixture = `<!doctype html><meta charset="utf-8"><div id="view"></div><div id="toast"></div>
    <script type="module">
      import { state } from '${base}assets/core.js';
      import { manager } from '${base}assets/openim.js';
      state.workspaceTitle = '企业即时通讯';
      state.route = 'im/manager';
      window.startManager = manager;
      window.leaveManager = () => { state.epoch++; document.getElementById('view').replaceChildren('其他页面'); };
      await manager(); window.ready = true;
    </script>`;
  const server = createServer(async (request, response) => {
    try {
      const url = new URL(request.url, "http://localhost");
      if (url.pathname.startsWith(api + "/")) {
        const path = url.pathname.slice(api.length);
        let input = "";
        for await (const chunk of request) input += chunk;
        requests.push({ path, method: request.method, headers: request.headers, body: input ? JSON.parse(input) : undefined });
        const version = revision;
        const person = { employeeID: "e1", account: "1599", name: "测试员工", departmentName: "测试部门", eligible: true,
          allowLAN: true, allowPC: true, allowMobile: false, allowInternet: false, allowInternetDownload: false, revision: version };
        const values = {
          "/employees": [{ projection: { account: "1599", nickname: "同步版本" + version, departmentName: "测试部门", enabled: true }, status: "applied" }],
          "/departments": [], "/groups": [], "/access/employees": [person],
          "/access/networks": { cidrs: ["192.168.10.0/24"], currentIP: "192.168.10.10", currentZone: "lan", revision: version },
          "/certificate": { domain: "kf.szputy.com", status: "applied", available: true, revision: version },
        };
        const send = () => {
          response.writeHead(200, { "Content-Type": "application/json" });
          response.end(JSON.stringify({ code: 0, data: request.method === "POST" ? {} : values[path] }));
        };
        const key = request.method + " " + path;
        if (holdNext.delete(key)) held.push({ key, send });
        else send();
        return;
      }
      if (url.pathname.startsWith(base + "assets/")) {
        const name = url.pathname.slice((base + "assets/").length);
        if (!/^[a-z-]+\.js$/.test(name)) throw new Error("Invalid fixture asset");
        response.writeHead(200, { "Content-Type": "text/javascript" });
        response.end(await readFile(join(root, "assets", name)));
        return;
      }
      response.writeHead(200, { "Content-Type": "text/html" });
      response.end(fixture);
    } catch (error) {
      response.writeHead(500);
      response.end(error.message);
    }
  });
  await new Promise(resolve => server.listen(0, "127.0.0.1", resolve));
  const browser = process.env.CHROME_PATH || [
    "C:/Program Files/Google/Chrome/Application/chrome.exe",
    "C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe",
    "/usr/bin/chromium", "/usr/bin/google-chrome",
  ].find(existsSync);
  assert.ok(browser, "Set CHROME_PATH to a Chromium browser");
  const userDir = await mkdtemp(join(tmpdir(), "biz-suite-openim-manager-"));
  const child = spawn(browser, ["--headless=new", "--disable-gpu", "--no-first-run", "--no-default-browser-check",
    "--remote-debugging-port=0", "--remote-allow-origins=*", "--user-data-dir=" + userDir, "about:blank"],
  { windowsHide: true, stdio: "ignore" });
  let socket;
  try {
    let port;
    for (let i = 0; i < 100; i++) {
      try { port = (await readFile(join(userDir, "DevToolsActivePort"), "utf8")).split("\n")[0]; break; }
      catch { await delay(50); }
    }
    assert.ok(port, "Chrome started");
    const targets = await (await fetch(`http://127.0.0.1:${port}/json/list`)).json();
    socket = new WebSocket(targets.find(target => target.type === "page").webSocketDebuggerUrl);
    await new Promise((resolve, reject) => { socket.addEventListener("open", resolve); socket.addEventListener("error", reject); });
    const cdp = new CDP(socket);
    await cdp.send("Runtime.enable");
    await cdp.send("Page.enable");
    await cdp.send("Page.navigate", { url: "http://127.0.0.1:" + server.address().port + base + "im/manager" });
    await cdp.wait("window.ready === true");
    const sectionTitles = ["访问网段", "员工访问权限", "SSL证书", "员工开通与同步", "部门群", "群组管理"];
    const assertSingleSections = async () => {
      const counts = await cdp.eval("[...document.querySelectorAll('#view h2')].map(node => node.textContent)");
      for (const title of sectionTitles) assert.equal(counts.filter(value => value === title).length, 1, title);
      assert.equal(await cdp.eval("document.querySelectorAll('[data-im-manager]').length"), 1);
    };
    const waitHeld = async () => {
      for (let i = 0; i < 100 && !held.length; i++) await delay(25);
      assert.equal(held.length, 1, "The selected response is held");
    };
    await assertSingleSections();

    for (const path of ["/employees", "/access/networks"]) {
      await t.test("older response cannot append duplicate sections after refreshing " + path, async () => {
        const oldVersion = ++revision;
        holdNext.add("GET " + path);
        await cdp.eval("void window.startManager(); true");
        await waitHeld();
        const newVersion = ++revision;
        await cdp.eval("window.startManager()");
        await assertSingleSections();
        held.shift().send();
        await delay(120);
        await assertSingleSections();
        assert.match(await cdp.eval("document.getElementById('view').textContent"), new RegExp("同步版本" + newVersion));
        assert.doesNotMatch(await cdp.eval("document.getElementById('view').textContent"), new RegExp("同步版本" + oldVersion));
      });
    }

    await t.test("saving employee permissions refreshes once and preserves request protection", async () => {
      const previous = requests.filter(request => request.path === "/employees").length;
      await cdp.eval("[...document.querySelectorAll('button')].find(button => button.textContent === '配置权限').click()");
      await cdp.wait("!!document.querySelector('dialog')");
      assert.match(await cdp.eval("document.querySelector('dialog').textContent"), /手机网页与Android\/iOS应用归手机端/);
      await cdp.eval("[...document.querySelectorAll('dialog button')].find(button => button.textContent === '保存权限').click()");
      await cdp.wait("!document.querySelector('dialog') && !!document.querySelector('[data-im-manager] h2')");
      await delay(100);
      await assertSingleSections();
      assert.equal(requests.filter(request => request.path === "/access/employee" && request.method === "POST").length, 1);
      const saved = requests.find(request => request.path === "/access/employee" && request.method === "POST");
      assert.equal(saved.headers["x-requested-with"], "XMLHttpRequest");
      assert.equal(saved.headers["content-type"], "application/json");
      assert.equal(saved.body.employeeID, "e1");
      assert.equal(saved.body.expectedRevision, revision);
      assert.equal(requests.filter(request => request.path === "/employees").length, previous + 1);
    });

    await t.test("saving after navigating away cannot reopen the manager", async () => {
      holdNext.add("POST /access/networks");
      await cdp.eval("[...document.querySelectorAll('button')].find(button => button.textContent === '保存网段').click()");
      await waitHeld();
      const previous = requests.length;
      await cdp.eval("window.leaveManager(); window.dispatchEvent(new CustomEvent('gosuxin:im-access-changed')); true");
      held.shift().send();
      await delay(120);
      assert.equal(await cdp.eval("document.getElementById('view').textContent"), "其他页面");
      assert.equal(requests.length, previous, "A completed save must not start background manager requests on another route");
    });
    assert.deepEqual(cdp.errors, [], "No uncaught browser errors");
  } finally {
    for (const pending of held) pending.send();
    socket?.close();
    child.kill();
    server.closeAllConnections();
    await new Promise(resolve => server.close(resolve));
  }
});
