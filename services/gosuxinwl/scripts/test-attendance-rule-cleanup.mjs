// Read-only verification of reversible rule cleanup. The caller supplies and
// removes the temporary host account. This script never creates business data.
import assert from "node:assert/strict";
import { readFile, writeFile, mkdir, mkdtemp, rm } from "node:fs/promises";
import { createHash } from "node:crypto";
import { spawn } from "node:child_process";
import { resolve, join, basename, relative, isAbsolute } from "node:path";
import { tmpdir } from "node:os";

const root = resolve(import.meta.dirname, "..");
const help = `Read-only attendance rule cleanup verification
  node scripts/test-attendance-rule-cleanup.mjs --environment public|local
    --credentials <temporary-credentials.json> --package-info <build-report.json>
    [--base <origin>] [--manifest <plugin.json>]
    [--expected-visible <count>] [--expected-hidden <count>]

Public defaults: https://kf.szputy.com:9880, DNS pinned to 61.144.183.217, visible 52 / hidden 52.
Local defaults: http://127.0.0.1:19880, visible 47 / hidden 81.
For local HTTPS use --base https://kf.szputy.com:9880; DNS is pinned to 127.0.0.1.
Build report: {package:<manifest SHA256>,asset:<JS path>,stage:<package dir>}
or {public:{...},local:{...}}. An explicit manifest path or asset_sha256 is also supported.
Only host authentication may write; all plugin business writes are blocked.
Screenshots/reports are written under runtime/browser-tests. No business data is saved.`;
function parseArgs(args) {
  const allowed = new Set(["environment", "credentials", "package-info", "base", "manifest", "expected-visible", "expected-hidden"]), values = {};
  for (let i = 0; i < args.length; i++) {
    const key = args[i].replace(/^--/, "");
    assert.ok(args[i].startsWith("--") && allowed.has(key), "Unknown option: " + args[i]);
    assert.ok(args[i + 1] && !args[i + 1].startsWith("--"), "Missing value for " + args[i]);
    assert.ok(!(key in values), "Duplicate option: " + key); values[key] = args[++i];
  }
  return values;
}
if (process.argv.includes("--help")) { console.log(help); process.exit(0); }
const args = parseArgs(process.argv.slice(2)), environment = args.environment;
assert.ok(["public", "local"].includes(environment), "--environment must be public or local");
assert.ok(args.credentials && args["package-info"], "--credentials and --package-info are required");
const readJSON = async path => JSON.parse((await readFile(resolve(root, path), "utf8")).replace(/^\uFEFF/, ""));
const credentials = await readJSON(args.credentials), build = await readJSON(args["package-info"]), release = build[environment] || build;
assert.ok(typeof credentials.username === "string" && credentials.username && typeof credentials.password === "string" && credentials.password, "Temporary credentials must contain username and password");
assert.match(release.package || "", /^[a-f0-9]{64}$/i, "Build report must identify the exact manifest SHA256 as package");
assert.ok(typeof release.asset === "string" && release.asset.endsWith(".js"), "Build report must identify its JavaScript asset");
const expectedAsset = basename(release.asset.replaceAll("\\", "/"));
let expectedAssetHash = release.asset_sha256 || release.assetSha256 || "";
const manifestPath = args.manifest || release.manifest || (release.stage ? join(release.stage, "plugin.json") : "");
if (manifestPath) {
  const bytes = await readFile(resolve(root, manifestPath));
  assert.equal(createHash("sha256").update(bytes).digest("hex"), release.package.toLowerCase(), "Manifest bytes must match the build report package identity");
  const manifest = JSON.parse(bytes.toString("utf8").replace(/^\uFEFF/, ""));
  const matches = Object.entries(manifest.files || {}).filter(([path]) => path.replaceAll("\\", "/").endsWith("/" + expectedAsset));
  assert.equal(matches.length, 1, "Manifest must contain exactly one matching asset");
  if (expectedAssetHash) assert.equal(expectedAssetHash.toLowerCase(), matches[0][1].toLowerCase());
  expectedAssetHash = matches[0][1];
}
assert.match(expectedAssetHash, /^[a-f0-9]{64}$/i, "Provide stage/manifest or asset_sha256 so the served asset bytes can be verified");
const publicIP = "61.144.183.217", base = args.base || (environment === "public" ? "https://kf.szputy.com:9880" : "http://127.0.0.1:19880");
const origin = new URL(base);
assert.equal(origin.origin, base, "--base must be an origin without a path or trailing slash");
assert.ok(!origin.username && !origin.password, "Do not put credentials in the URL");
if (environment === "public") assert.equal(base, "https://kf.szputy.com:9880", "Public verification must use the authorized HTTPS hostname and port");
else assert.ok(["http://127.0.0.1:19880", "http://localhost:19880", "https://kf.szputy.com:9880"].includes(base), "Local verification must use port 19880 or local-pinned domain HTTPS on 9880");
const expectedVisible = Number(args["expected-visible"] ?? (environment === "public" ? 52 : 47));
const expectedHidden = Number(args["expected-hidden"] ?? (environment === "public" ? 52 : 81));
assert.ok(Number.isSafeInteger(expectedVisible) && expectedVisible > 0 && Number.isSafeInteger(expectedHidden) && expectedHidden > 0);
const output = join(root, "runtime/browser-tests"), run = `attendance-rule-cleanup-${environment}-${Date.now()}`, reportPath = join(output, run + "-report.json");
const admin = "/plugins/biz-suite/admin/", api = "/plugins/biz-suite/api/admin", currentSection = "[data-attendance-current-rules]", historySection = "[data-attendance-rule-history]";
const report = { run, environment, base, package: release.package, expected_asset: expectedAsset, expected_asset_sha256: expectedAssetHash.toLowerCase(), expected_visible: expectedVisible, expected_hidden: expectedHidden, started_at: new Date().toISOString(), business_writes_allowed: false, checks: [], screenshots: [], editor_checks: [] };
const pause = ms => new Promise(resolve => setTimeout(resolve, ms));
const rows = value => Array.isArray(value) ? value : value?.items || value?.rows || value?.list || [];
const stable = value => Array.isArray(value) ? value.map(stable) : value && typeof value === "object" ? Object.fromEntries(Object.keys(value).sort().map(key => [key, stable(value[key])])) : value;
const hash = value => createHash("sha256").update(JSON.stringify(stable([...value].sort((a, b) => String(a.id).localeCompare(String(b.id)))))).digest("hex");
function sourceOf(rule) { if (typeof rule.source !== "string") return rule.source || {}; try { return JSON.parse(rule.source); } catch { return {}; } }
const hidden = rule => sourceOf(rule).cleanup?.hidden === true;
const historical = rule => !!sourceOf(rule).snapshot_id || !["draft", "published"].includes(rule.status);
// Mirrors the shared dataTable search. A rule ID may also be a retained version's
// family_id, so searches must allow that legitimate match instead of treating it
// as the hidden row reappearing.
const matchesSearch = (rule, term) => Object.values(rule).some(value => String(value ?? "").toLowerCase().includes(term.toLowerCase()));
let chrome, profile, page, before;
await mkdir(output, { recursive: true });

class Page {
  constructor(socket) {
    this.socket = socket; this.nextID = 0; this.pending = new Map(); this.errors = []; this.blocked = []; this.interceptionErrors = []; this.addresses = new Set();
    socket.addEventListener("message", ({ data }) => {
      const event = JSON.parse(data);
      if (event.id) { const item = this.pending.get(event.id); if (item) { clearTimeout(item.timer); this.pending.delete(event.id); event.error ? item.reject(Error(event.error.message)) : item.resolve(event.result); } }
      if (event.method === "Runtime.exceptionThrown") this.errors.push(event.params.exceptionDetails.exception?.description || event.params.exceptionDetails.text);
      if (event.method === "Network.responseReceived" && new URL(event.params.response.url).hostname === origin.hostname && event.params.response.remoteIPAddress) this.addresses.add(event.params.response.remoteIPAddress);
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
  async readRules() {
    return this.eval(`(async()=>{const response=await fetch(${JSON.stringify(api + "/hrm/attendance/v2/rules")},{method:'GET',credentials:'include',cache:'no-store',headers:{'X-Requested-With':'XMLHttpRequest'}});const body=await response.json();if(!response.ok||(body.code!=null&&Number(body.code)!==0))throw Error(body.message||body.msg||'Rule read failed');return body.data??body;})()`);
  }
  async screenshot(label) { const result = await this.send("Page.captureScreenshot", { format: "png", captureBeyondViewport: false }); const filename = run + "-" + label + ".png"; await writeFile(join(output, filename), Buffer.from(result.data, "base64")); report.screenshots.push(filename); }
  async search(section, term) {
    return this.eval(`(()=>{const section=document.querySelector(${JSON.stringify(section)});if(!section)return {exists:false,count:0,rows:[]};if(section.tagName==='DETAILS')section.open=true;const input=section.querySelector('input[type=search]');if(!input)throw Error('Rule table search is missing');input.value=${JSON.stringify(term)};input.dispatchEvent(new Event('input',{bubbles:true}));return {exists:true,count:Number(section.querySelector('.count').textContent.match(/\\d+/)[0]),rows:[...section.querySelectorAll('tbody tr')].map(tr=>({name:tr.cells[0].textContent.trim(),version:tr.cells[1].textContent.trim(),actions:[...tr.querySelectorAll('button')].map(button=>button.textContent.trim())}))};})()`);
  }
  async open(rule) {
    const section = historical(rule) ? historySection : currentSection, result = await this.search(section, String(rule.id));
    const rowIndex = result.rows.findIndex(row => row.name === rule.name && row.version === String(rule.version));
    assert.ok(rowIndex >= 0, "Retained rule must be available to open: " + rule.id);
    await this.eval(`document.querySelectorAll('${section} tbody tr')[${rowIndex}].querySelector('button').click()`);
    await this.until("!!document.querySelector('[data-attendance-rule-editor]')");
    return this.eval("(()=>{const form=document.querySelector('[data-attendance-rule-editor]');return {name:form.querySelector('[name=name]').value,name_disabled:form.querySelector('[name=name]').disabled,enabled_controls:form.querySelectorAll('input:not(:disabled),select:not(:disabled),textarea:not(:disabled),button:not(:disabled)').length,save_buttons:[...form.querySelectorAll('button')].filter(button=>!button.disabled&&/保存/.test(button.textContent)).map(button=>button.textContent.trim())};})()");
  }
  async close() { await this.eval("document.querySelector('dialog [aria-label=关闭]').click()"); await this.until("!document.querySelector('dialog')"); }
}

async function launch() {
  profile = await mkdtemp(join(tmpdir(), "attendance-rule-cleanup-"));
  const pin = environment === "public" ? publicIP : "127.0.0.1";
  chrome = spawn(process.env.CHROME_PATH || "C:/Program Files/Google/Chrome/Application/chrome.exe", ["--headless=new", "--disable-gpu", "--no-first-run", "--no-default-browser-check", "--ignore-certificate-errors", "--remote-debugging-port=0", "--remote-allow-origins=*", "--host-resolver-rules=MAP kf.szputy.com " + pin, "--no-proxy-server", "--user-data-dir=" + profile, "about:blank"], { stdio: "ignore", windowsHide: true });
  let port; for (let i = 0; i < 100 && !port; i++) { try { port = (await readFile(join(profile, "DevToolsActivePort"), "utf8")).split("\n")[0]; } catch { await pause(100); } }
  assert.ok(port, "Chrome did not start");
  const targets = await (await fetch(`http://127.0.0.1:${port}/json/list`, { signal: AbortSignal.timeout(5000) })).json();
  const socket = new WebSocket(targets.find(target => target.type === "page").webSocketDebuggerUrl);
  await new Promise((resolve, reject) => { const timer = setTimeout(() => reject(Error("CDP connection failed")), 10000); socket.addEventListener("open", () => { clearTimeout(timer); resolve(); }, { once: true }); });
  page = new Page(socket); await page.send("Runtime.enable"); await page.send("Page.enable"); await page.send("Network.enable");
  await page.send("Fetch.enable", { patterns: [{ urlPattern: "*/plugins/biz-suite/api/*", requestStage: "Request" }] });
  await page.send("Emulation.setDeviceMetricsOverride", { width: 1440, height: 1050, deviceScaleFactor: 1, mobile: false });
  await page.send("Page.navigate", { url: base + admin + "hrm/attendance-rules" });
  await page.until("!!document.querySelector('input[type=password]')"); await pause(400);
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
  await step("login-and-exact-built-asset", async () => {
    await launch(); report.loaded_scripts = await page.eval("[...document.scripts].map(script=>script.src).filter(Boolean)");
    const assetURL = report.loaded_scripts.find(url => new URL(url).pathname.endsWith("/" + expectedAsset));
    assert.ok(assetURL, "Page must load the expected build's asset");
    report.served_asset_sha256 = await page.eval(`(async()=>{const r=await fetch(${JSON.stringify(assetURL)},{cache:'no-store'});if(!r.ok)throw Error('Asset fetch failed');const bytes=await r.arrayBuffer();return [...new Uint8Array(await crypto.subtle.digest('SHA-256',bytes))].map(b=>b.toString(16).padStart(2,'0')).join('');})()`);
    assert.equal(report.served_asset_sha256, expectedAssetHash.toLowerCase(), "Browser-served asset bytes must match the package manifest");
    if (environment === "public") assert.ok(page.addresses.has(publicIP), "Browser must connect to the public IP");
    else assert.ok([...page.addresses].some(ip => ["127.0.0.1", "::1", "[::1]"].includes(ip)), "Browser must connect to the local host");
  });
  await step("API-retains-original-rows-and-cleanup-metadata", async () => {
    before = rows(await page.readRules()); report.before_hash = hash(before);
    const hiddenRules = before.filter(hidden), visibleRules = before.filter(rule => !hidden(rule));
    assert.equal(before.length, expectedHidden + expectedVisible, "Rule rows must be retained, not deleted");
    assert.equal(hiddenRules.length, expectedHidden); assert.equal(visibleRules.length, expectedVisible);
    assert.ok(hiddenRules.every(rule => rule.status === "archived"), "Hidden rules must be archived");
    assert.equal(new Set(before.map(rule => rule.id)).size, before.length);
    report.total_rules = before.length; report.hidden_rules = hiddenRules.length; report.visible_rules = visibleRules.length;
    report.visible_current_rules = visibleRules.filter(rule => !historical(rule)).length; report.visible_history_rules = visibleRules.filter(historical).length;
  });
  await step("hidden-rules-absent-from-both-lists-and-all-retained-rules-present", async () => {
    const visibleRules = before.filter(rule => !hidden(rule));
    for (const section of [currentSection, historySection]) {
      const history = section === historySection, sectionRules = visibleRules.filter(rule => historical(rule) === history);
      const initial = await page.search(section, ""); assert.equal(initial.count, sectionRules.length);
      for (const rule of before) {
        const expected = sectionRules.filter(candidate => matchesSearch(candidate, String(rule.id)));
        const actual = await page.search(section, String(rule.id));
        assert.equal(actual.count, expected.length, "Unexpected search result count for " + rule.id + " in " + section);
        assert.deepEqual(actual.rows.map(row => [row.name, row.version]), expected.slice(0, 20).map(row => [row.name, String(row.version)]), "Hidden rows must not replace retained versions in search");
        for (const row of actual.rows) {
          if (history) assert.deepEqual(row.actions, ["查看"], "History must only permit View");
          else assert.equal(row.actions[0], "编辑 / 新版本", "Retained current rules must remain editable");
        }
      }
      await page.search(section, "");
    }
    report.hidden_ids_checked_in_both_lists = before.filter(hidden).map(rule => rule.id);
    report.retained_ids_checked = visibleRules.map(rule => rule.id);
    // A single list screenshot per environment, with the retained history section
    // expanded. No dialogs, employee directory details, or credentials are captured.
    await page.eval("window.scrollTo(0,0)"); await page.screenshot("retained-rule-list");
  });
  await step("retained-edit-and-history-read-only-behavior", async () => {
    const visible = before.filter(rule => !hidden(rule)), representatives = [];
    for (const status of ["draft", "published"]) { const rule = visible.find(rule => !historical(rule) && rule.status === status); if (rule) representatives.push(rule); }
    const history = visible.find(historical); if (history) representatives.push(history);
    assert.ok(representatives.some(rule => !historical(rule)), "At least one editable current rule must remain");
    if (environment === "local") assert.ok(history, "The useful local superseded rule must remain available");
    for (const rule of representatives) {
      const actual = await page.open(rule); assert.equal(actual.name, rule.name);
      if (historical(rule)) { assert.equal(actual.enabled_controls, 0); assert.equal(actual.name_disabled, true); assert.deepEqual(actual.save_buttons, []); }
      else { assert.equal(actual.name_disabled, false); assert.ok(actual.enabled_controls > 0); assert.ok(actual.save_buttons.includes("保存草稿")); }
      report.editor_checks.push({ id: rule.id, status: rule.status, read_only: historical(rule), passed: true }); await page.close();
    }
  });
  await step("zero-business-writes-no-JS-errors-and-unchanged-rule-data", async () => {
    report.after_hash = hash(rows(await page.readRules())); report.rules_unchanged = report.before_hash === report.after_hash;
    assert.equal(report.rules_unchanged, true); assert.deepEqual(page.errors, []); assert.deepEqual(page.blocked, []); assert.deepEqual(page.interceptionErrors, []);
  });
  report.passed = true;
} catch (error) {
  report.passed = false; report.error = error.message; process.exitCode = 1; console.error("FAIL rule cleanup: " + error.message);
  try { await page?.screenshot("failure"); } catch {}
} finally {
  if (page && before && !report.after_hash) { try { report.after_hash = hash(rows(await page.readRules())); report.rules_unchanged = report.before_hash === report.after_hash; if (!report.rules_unchanged) process.exitCode = 1; } catch { report.final_snapshot_failed = true; process.exitCode = 1; } }
  report.js_errors = page?.errors || []; report.blocked_business_writes = page?.blocked || []; report.interception_errors = page?.interceptionErrors || []; report.remote_addresses = [...(page?.addresses || [])];
  try { page?.socket.close(); chrome?.kill(); } catch {}
  if (profile) { const parent = resolve(tmpdir()), target = resolve(profile), child = relative(parent, target); assert.ok(child && !child.startsWith("..") && !isAbsolute(child) && target.includes("attendance-rule-cleanup-")); await pause(500); await rm(target, { recursive: true, force: true, maxRetries: 5, retryDelay: 300 }).catch(() => {}); }
  report.cleanup = { browser_stopped: true, business_records_unchanged: report.rules_unchanged === true, temporary_account: "Created and cleaned up by deployment caller" };
  if (process.exitCode) report.passed = false;
  report.finished_at = new Date().toISOString(); await writeFile(reportPath, JSON.stringify(report, null, 2)); console.log("Report: " + reportPath);
}
