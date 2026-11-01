// Synthetic media collaborators only. Startup logs in; it never starts a call.
import { chromium } from "playwright";
import { readFile, writeFile, mkdir } from "node:fs/promises";
import { resolve } from "node:path";
import { createServer } from "node:http";
import { createHash } from "node:crypto";
import { captureVideoState } from "./video-frame-state.mjs";
import { installRtcTransportObserver, captureAudioState, captureRtcTransportState } from "./rtc-media-state.mjs";

if (process.argv.includes("--help")) {
  console.log("node tests/media-resume-114-web.mjs --start-confirmed [--diagnostic-reuse-only]. One Chrome, synthetic users106/107; Android104. Loopback127.0.0.1:17794. Startup only original-form login. GET/state; explicit POST/group/setup, /group/reuse (fixed existing synthetic group only), /group/open, /android/open, /watch-incoming, /accept, /hangup, /sample, /screenshot, /close. No arbitrary evaluation or direct RTC-signaling endpoint. Diagnostic reuse mode forbids group creation. --check only validates script preparation without browser/login/network.");
  process.exit(0);
}
if (process.argv.includes("--check")) {
  console.log(JSON.stringify({ prepared: true, startupCalls: false, startsBrowser: false, endpointPort: 17794, users: ["upgrade38_u104", "upgrade38_u106", "upgrade38_u107"], transportObserver: "Native getStats with weak references; no transport parameter override", screenCaptureEnabled: false }));
  process.exit(0);
}
if (!process.argv.includes("--start-confirmed")) throw new Error("Wait for the Android114 owner and pass --start-confirmed");

const project = resolve("../.."), started = new Date().toISOString();
const out = resolve(project, "tests/delivery/media-resume-20261102/web-runtime", started.replace(/[-:.]/g, ""));
await mkdir(out, { recursive: true });
const temp = resolve(project, ".cache/temp-web/media114"); await mkdir(temp, { recursive: true }); process.env.TEMP = temp; process.env.TMP = temp;
const fixture = JSON.parse(await readFile(resolve(project, ".private/test-accounts.json"), "utf8"));
const accounts = { "106": fixture.users[105], "107": fixture.users[106] }, android = fixture.users[103];
if (accounts["106"].userID !== "upgrade38_u106" || accounts["107"].userID !== "upgrade38_u107" || android.userID !== "upgrade38_u104") throw new Error("Synthetic identity guard");
const base = "https://192.168.10.11:29880", vite = process.env.WEB_MEDIA_URL || "http://127.0.0.1:7777", port = 17794;
const secrets = new Set();
function remember(value, key = "") {
  if (Array.isArray(value)) return value.forEach(v => remember(v, key));
  if (value && typeof value === "object") return Object.entries(value).forEach(([k, v]) => remember(v, k));
  if (typeof value === "string" && value.length >= 6 && /password|token/i.test(key)) {
    secrets.add(value); if (/password/i.test(key)) { const hash = createHash("md5").update(value).digest("hex"); secrets.add(hash); secrets.add(hash.toUpperCase()); }
  }
}
remember(fixture);
const sanitize = value => {
  let text = String(value);
  for (const s of [...secrets].sort((a, b) => b.length - a.length)) text = text.split(s).join("[REDACTED]");
  return text.replace(/eyJ[A-Za-z0-9_-]{15,}\.[A-Za-z0-9_-]{20,}\.[A-Za-z0-9_-]{15,}/g, "[JWT_REDACTED]").replace(/((?:https?|wss?):\/\/[^\s?]+)\?[^\s]+/g, "$1?[QUERY_REDACTED]");
};
const pages = new Map(), groups = new Map(), operations = [], errors = [], requestFailures = [], httpErrors = [], consoleCategories = [];
const diagnosticReuseOnly = process.argv.includes("--diagnostic-reuse-only");
const fixedDiagnosticGroup = { groupID: "1739269662", groupName: "Android114 media synthetic 1793559206654", ownerUserID: "upgrade38_u106", members: ["upgrade38_u104", "upgrade38_u106", "upgrade38_u107"] };
let browser, server, watching = false, closing = false, startupPhase = "before-launch", startupTimer;
const sleep = ms => new Promise(r => setTimeout(r, ms));
const write = async (name, value) => { const target = resolve(out, name); const bytes = JSON.stringify(value, null, 2) + "\n"; await writeFile(target, bytes, { flag: "wx" }); return { path: target, bytes: Buffer.byteLength(bytes), sha256: createHash("sha256").update(bytes).digest("hex") }; };
const journal = async () => write("operations-" + Date.now() + ".json", { started, sampledAt: new Date().toISOString(), operations, errors, boundary: "One real Chrome with synthetic camera/microphone, original app login/accept/hangup UI. No screen capture, injected token, ignored TLS errors, real account or physical-device claim." });
const bounded = async (promise, label, milliseconds = 12000) => { let timer; try { return await Promise.race([promise, new Promise((_, reject) => { timer = setTimeout(() => reject(new Error(label + " exceeded its bounded deadline")), milliseconds); })]); } finally { clearTimeout(timer); } };
const phase = async value => { startupPhase = value; await write("phase-" + Date.now() + ".json", { at: new Date().toISOString(), phase: value }); console.log(JSON.stringify({ startupPhase: value })); };
const safeURL = value => { try { const u = new URL(value); return u.origin + u.pathname; } catch { return "InvalidURL"; } };
const getPage = key => { const p = pages.get(key || "106"); if (!p) throw new Error("Only synthetic peers106/107"); return p; };
const safeName = value => { if (!/^[A-Za-z0-9_-]{1,60}$/.test(value || "")) throw new Error("Invalid evidence label"); return value; };

// Observe native-created local tracks without keeping strong track ownership or
// replacing transport options, track.stop, Room.disconnect or product controls.
function installLocalTrackObserver() {
  if (window.media114Tracks || !navigator.mediaDevices || typeof WeakRef !== "function") return;
  const state = window.media114Tracks = { next: 0, tracks: [] };
  const native = navigator.mediaDevices.getUserMedia;
  navigator.mediaDevices.getUserMedia = new Proxy(native, { apply(target, thisArg, args) {
    return Reflect.apply(target, thisArg, args).then(stream => {
      for (const track of stream.getTracks()) state.tracks.push({ id: ++state.next, weak: new WeakRef(track) });
      if (state.tracks.length > 64) state.tracks.splice(0, state.tracks.length - 64);
      return stream;
    });
  } });
}

async function health(page, expectedUserID) {
  return bounded(page.evaluate(async id => {
    try {
      const [login, self, conversations] = await Promise.all([window.media114Im.getLoginStatus(), window.media114Im.getSelfUserInfo(), window.media114Im.getConversationListSplit({ offset: 0, count: 1 })]);
      return { ready: Number(login.data) === 3 && self.data?.userID === id && Array.isArray(conversations.data), userID: self.data?.userID, loginStatus: Number(login.data), sqliteReadOK: conversations.errCode === 0 && Array.isArray(conversations.data) };
    } catch (e) { return { ready: false, errorType: e?.name || "SDKError", errCode: Number.isFinite(e?.errCode) ? e.errCode : null }; }
  }, expectedUserID), "SDK-health-" + expectedUserID);
}
async function incoming(page) {
  const modal = page.locator(".rtc-single-modal:visible");
  const visible = await modal.count() > 0;
  if (!visible) return { visible: false, accept: false, hangup: false, expectedCaller: false };
  const text = await modal.innerText();
  return { visible: true, accept: await modal.getByText("接听", { exact: true }).count() === 1, hangup: await modal.getByText("挂断", { exact: true }).count() === 1, expectedCaller: text.includes(android.nickname) && text.includes("邀请你进行"), groupInvitation: /\d+人正在通话/.test(text) };
}
async function state(key, page) {
  const [video, audio, transport, appHealth, ui, localTracks] = await Promise.all([
    page.evaluate(captureVideoState), page.evaluate(captureAudioState), page.evaluate(captureRtcTransportState), health(page, accounts[key].userID), incoming(page),
    page.evaluate(() => ({ page: location.origin + location.pathname + location.hash, meetingVisible: Boolean(document.querySelector(".meeting-modal")), videoSources: [...document.querySelectorAll("video")].map(v => ({ source: v.dataset.lkSource || null, localParticipant: v.dataset.lkLocalParticipant || null })), localTracks: (window.media114Tracks?.tracks || []).map(r => { const t = r.weak.deref(); return t ? { id: r.id, trackID: t.id, kind: t.kind, enabled: t.enabled, muted: t.muted, readyState: t.readyState } : { id: r.id, collected: true }; }) })),
  ]);
  return { peer: key, sampledAt: new Date().toISOString(), appHealth, ui, ...localTracks, videos: video.map((v, i) => ({ ...v, ...localTracks.videoSources[i] })), audios: audio, transport };
}
async function allState() { return { sampledAt: new Date().toISOString(), peers: await Promise.all([...pages].map(([k, p]) => state(k, p))), groups: [...groups.values()], errors }; }
async function open(page, id, sessionType) {
  await page.evaluate(async ({ id, sessionType }) => { const { data: c } = await window.media114Im.getOneConversation({ sourceID: id, sessionType }); window.media114Stores.useConversationStore.getState().updateCurrentConversation(c); location.hash = "/chat/" + c.conversationID; }, { id, sessionType });
  await page.locator("#chat-main").waitFor({ state: "visible" });
}
async function accept(key, expectedGroupID) {
  const page = getPage(key), flags = await incoming(page);
  if (!flags.visible || !flags.accept || !flags.expectedCaller) throw new Error("No original incoming call from synthetic Android104");
  if (expectedGroupID && (!groups.has(expectedGroupID) || !flags.groupInvitation || !page.url().endsWith("/#/chat/sg_" + expectedGroupID))) throw new Error("Incoming group call does not match the expected synthetic group route");
  await page.locator(".rtc-single-modal:visible").getByText("接听", { exact: true }).click();
  return { peer: key, acceptedThroughOriginalUI: true, at: new Date().toISOString() };
}
async function safeScreenshot(key, label) {
  const page = getPage(key);
  if (await page.locator('input[type="password"]').count() || !await page.locator("aside").count()) throw new Error("Post-login screenshot guard");
  const target = resolve(out, safeName(label) + "-" + key + "-" + Date.now() + ".png"); await page.screenshot({ path: target });
  return { peer: key, path: target };
}
async function close() {
  if (closing) return; closing = true; clearTimeout(startupTimer); await journal(); server?.close(); await browser?.close();
}

try {
  startupTimer = setTimeout(() => { void write("startup-hard-deadline.json", { at: new Date().toISOString(), phase: startupPhase, deadlineMilliseconds: 300000 }).finally(() => { process.exitCode = 1; void close(); }); }, 300000);
  await phase("launching-owned-Chrome");
  browser = await chromium.launch({ headless: true, executablePath: "C:/Program Files/Google/Chrome/Application/chrome.exe", args: ["--use-fake-device-for-media-stream", "--use-fake-ui-for-media-stream"] });
  for (const [key, account] of Object.entries(accounts)) {
    const context = await browser.newContext({ permissions: ["camera", "microphone"], viewport: { width: 1440, height: 1000 }, locale: "zh-CN" });
    const page = await context.newPage(); pages.set(key, page); page.setDefaultTimeout(15000);
    page.on("pageerror", e => errors.push({ peer: key, at: new Date().toISOString(), errorType: e.name, message: sanitize(e.message) }));
    page.on("requestfailed", r => requestFailures.push({ peer: key, url: safeURL(r.url()), resourceType: r.resourceType(), reason: sanitize(r.failure()?.errorText || "RequestFailed") }));
    page.on("response", r => { if (r.status() >= 400) httpErrors.push({ peer: key, url: safeURL(r.url()), status: r.status() }); });
    page.on("console", message => { if (!["error", "warning"].includes(message.type())) return; const text = message.text(); const matched = ["SharedArrayBuffer", "CORS", "Failed to load module script", "WebAssembly", "net::ERR", "Outdated Optimize Dep", "SyntaxError", "ReferenceError"].filter(s => text.includes(s)); if (matched.length) consoleCategories.push({ peer: key, type: message.type(), matched }); });
    page.on("response", async r => { if (r.url().includes("/chat/account/login")) { try { remember(await r.json()); } catch {} } });
    await page.addInitScript(installRtcTransportObserver); await page.addInitScript(installLocalTrackObserver);
    await page.addInitScript(({ base }) => { for (const [k, v] of Object.entries({ apiUrl: base + "/api", chatUrl: base + "/chat", enterpriseUrl: base + "/enterprise/v1", wsUrl: base.replace("https:", "wss:") + "/ws" })) localStorage.setItem("upgrade38:" + k, v); }, { base });
    await phase("peer" + key + "-navigating-before-login");
    await page.goto(vite, { waitUntil: "domcontentloaded", timeout: 90000 });
    await phase("peer" + key + "-waiting-original-WASM-entry");
    await page.waitForFunction(() => typeof window.commonEventFunc === "function", undefined, { timeout: 120000 });
    await phase("peer" + key + "-submitting-original-login-form");
    await page.getByRole("tab", { name: "邮箱", exact: true }).click();
    await page.locator("#email").fill(account.email); await page.locator("#password").fill(account.password ?? fixture.password);
    await page.getByRole("button", { name: "登录", exact: true }).click(); await page.waitForURL("**/#/chat**", { timeout: 90000 });
    await phase("peer" + key + "-checking-real-SDK-SQLite-health");
    await page.evaluate(async () => { window.media114Im = (await import("/src/im/ImFacade.ts")).ImFacade; window.media114Stores = await import("/src/store/index.ts"); });
    const until = Date.now() + 90000; let checked;
    while (Date.now() < until) { checked = await health(page, account.userID); if (checked.ready) break; await sleep(500); }
    if (!checked.ready) throw new Error("Original login did not reach healthy SDK/database for peer" + key);
    operations.push({ operation: "original-login", peer: key, normalTLS: true, ...checked });
    await phase("peer" + key + "-healthy");
  }
  server = createServer(async (req, res) => {
    try {
      if (req.headers.origin || req.headers.host !== "127.0.0.1:" + port) throw new Error("Only local non-browser test commands");
      const route = new URL(req.url, "http://127.0.0.1").pathname; let body = {};
      if (req.method === "POST") { let raw = ""; for await (const chunk of req) { raw += chunk; if (raw.length > 2048) throw new Error("Test request too large"); } body = raw ? JSON.parse(raw) : {}; }
      else if (req.method !== "GET") throw new Error("Unsupported method");
      let result;
      if (route === "/state" && req.method === "GET") result = await allState();
      else if (req.method !== "POST") throw new Error("Unknown read operation");
      else if (route === "/close") { await close(); result = { closedOwnedBrowser: true }; }
      else if (route === "/group/setup") {
        if (diagnosticReuseOnly) throw new Error("Diagnostic reuse mode forbids group creation");
        if (groups.size) throw new Error("This run already owns its one synthetic group");
        const group = await getPage("106").evaluate(async members => (await window.media114Im.createGroup({ groupInfo: { groupType: 2, groupName: "Android114 media synthetic " + Date.now() }, memberUserIDs: members, adminUserIDs: [] })).data, [accounts["107"].userID, android.userID]);
        const g = { groupID: group.groupID, groupName: group.groupName, members: [accounts["106"].userID, accounts["107"].userID, android.userID] }; groups.set(g.groupID, g); result = g;
      } else if (route === "/group/reuse") {
        if (!diagnosticReuseOnly || groups.size || body.groupID !== fixedDiagnosticGroup.groupID || Object.keys(body).some(k => k !== "groupID")) throw new Error("Only the one fixed existing diagnostic group may be reused");
        const checks = [];
        for (const peer of ["106", "107"]) {
          const checked = await bounded(getPage(peer).evaluate(async expected => {
            const [groupResult, memberResult] = await Promise.all([
              window.media114Im.getSpecifiedGroupsInfo([expected.groupID]),
              window.media114Im.getGroupMemberList({ groupID: expected.groupID, filter: 0, offset: 0, count: 4 }),
            ]);
            const group = groupResult.data?.[0], memberIDs = (memberResult.data || []).map(v => v.userID).sort();
            const exact = groupResult.errCode === 0 && memberResult.errCode === 0 && groupResult.data.length === 1 && group?.groupID === expected.groupID && group.groupName === expected.groupName && group.ownerUserID === expected.ownerUserID && group.memberCount === 3 && memberIDs.length === 3 && new Set(memberIDs).size === 3 && memberIDs.every((id, i) => id === [...expected.members].sort()[i]);
            return { exact, groupID: expected.groupID, expectedNameMatches: group?.groupName === expected.groupName, expectedOwnerMatches: group?.ownerUserID === expected.ownerUserID, groupMemberCount: group?.memberCount, memberIDsMatchExactly: memberIDs.length === 3 && memberIDs.every((id, i) => id === [...expected.members].sort()[i]) };
          }, fixedDiagnosticGroup), "fixed-group-reuse-" + peer);
          if (!checked.exact) throw new Error("Existing fixed group metadata or exact member guard failed for peer" + peer);
          checks.push({ peer, ...checked });
        }
        const g = { ...fixedDiagnosticGroup, reused: true, checks }; groups.set(g.groupID, g); result = g;
      } else if (route === "/group/open") {
        if (!groups.has(body.groupID)) throw new Error("Only this run's synthetic group");
        await open(getPage(body.peer), body.groupID, 3); result = { opened: true, groupID: body.groupID };
      } else if (route === "/android/open") { await open(getPage(body.peer), android.userID, 1); result = { opened: true }; }
      else if (route === "/accept") result = await accept(body.peer || "106", body.groupID);
      else if (route === "/hangup") {
        const page = getPage(body.peer), flags = await incoming(page); if (!flags.hangup) throw new Error("No original hangup control");
        await page.locator(".rtc-single-modal:visible").getByText("挂断", { exact: true }).click(); result = { hungUpThroughOriginalUI: true, peer: body.peer || "106" };
      } else if (route === "/watch-incoming") {
        if (watching) throw new Error("An incoming watcher is already running");
        const wanted = body.peers || ["106", "107"]; if (!Array.isArray(wanted) || !wanted.length || new Set(wanted).size !== wanted.length || wanted.some(k => !pages.has(k))) throw new Error("Only peers106/107");
        if (body.groupID && !groups.has(body.groupID)) throw new Error("Only this run's synthetic group");
        watching = true;
        const watchID = Date.now(), deadline = watchID + Math.max(1000, Math.min(60000, Number(body.timeoutMs) || 60000)); const accepted = [], samples = [];
        let watchError;
        await write("watch-running-" + watchID + ".json", { startedAt: new Date(watchID).toISOString(), deadline: new Date(deadline).toISOString(), peers: wanted, groupID: body.groupID || null, directSDKSignaling: false });
        try {
          while (Date.now() < deadline && accepted.length < wanted.length) {
            const next = wanted[accepted.length], flags = await incoming(getPage(next));
            const expectedRoute = !body.groupID || (flags.groupInvitation && getPage(next).url().endsWith("/#/chat/sg_" + body.groupID));
            samples.push({ peer: next, at: new Date().toISOString(), flags, expectedRoute });
            if (flags.accept && flags.expectedCaller && expectedRoute) accepted.push(await accept(next, body.groupID)); else await sleep(250);
          }
          result = { accepted, complete: accepted.length === wanted.length, samples, directSDKSignaling: false };
        } catch (e) { watchError = { errorType: e.name || "Error", message: sanitize(e.message) }; throw e;
        } finally {
          watching = false;
          const evidence = await write("watch-result-" + watchID + ".json", { finishedAt: new Date().toISOString(), accepted, complete: accepted.length === wanted.length, samples, directSDKSignaling: false, ...(watchError ? { error: watchError } : {}) });
          if (result) result.evidence = evidence;
        }
      } else if (route === "/sample") { const value = await allState(); result = { ...value, evidence: await write(safeName(body.label) + "-" + Date.now() + ".json", value) }; }
      else if (route === "/screenshot") result = await safeScreenshot(body.peer || "106", body.label || "original-ui");
      else throw new Error("Unknown test operation");
      operations.push({ operation: route, peer: body.peer || null, at: new Date().toISOString(), success: true });
      res.writeHead(200, { "Content-Type": "application/json" }); res.end(JSON.stringify(result));
    } catch (e) { res.writeHead(400, { "Content-Type": "application/json" }); res.end(JSON.stringify({ errorType: e.name || "Error", message: sanitize(e.message) })); }
  });
  await new Promise((r, reject) => { server.once("error", reject); server.listen(port, "127.0.0.1", r); });
  clearTimeout(startupTimer);
  await write("ready.json", { started, output: out, control: "127.0.0.1:" + port, accounts: Object.values(accounts).map(a => a.userID), android: android.userID, automaticCallOrMeeting: false, tokenInjected: false, tlsValidationDisabled: false, browserInstances: 1, contexts: 2, screenCaptureEnabled: false, errors });
  console.log(JSON.stringify({ ready: true, control: "127.0.0.1:" + port, output: out }));
} catch (e) {
  const pageDiagnostics = [];
  for (const [peer, page] of pages) {
    try { pageDiagnostics.push({ peer, ...await bounded(page.evaluate(() => ({ readyState: document.readyState, route: location.pathname + location.hash, commonEventFunc: typeof window.commonEventFunc, loginFunction: typeof window.login, Go: typeof window.Go, crossOriginIsolated: window.crossOriginIsolated, emailInputExists: Boolean(document.getElementById("email")), passwordInputExists: Boolean(document.getElementById("password")), loginButtonCount: [...document.querySelectorAll("button")].filter(b => b.textContent.trim() === "登录").length })), "startup-readonly-diagnostic", 3000) }); } catch { pageDiagnostics.push({ peer, diagnosticUnavailable: true }); }
  }
  await write("startup-failed.json", { at: new Date().toISOString(), phase: startupPhase, errorType: e.name || "Error", message: sanitize(e.stack || e), pageDiagnostics, requestFailures, httpErrors, consoleCategories, errors }); await close(); process.exitCode = 1;
}
process.on("SIGINT", () => { void close(); });
process.on("SIGTERM", () => { void close(); });
