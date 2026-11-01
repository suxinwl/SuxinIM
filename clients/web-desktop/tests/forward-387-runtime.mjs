import { chromium, _electron as electron } from "playwright";
import { readFile, writeFile, mkdir } from "node:fs/promises";
import { createHash } from "node:crypto";
import { resolve } from "node:path";

const root = resolve("../.."), label = process.env.OPENIM_FORWARD_PHASE ?? "baseline";
const vite = process.env.OPENIM_FORWARD_VITE ?? "http://127.0.0.1:7777";
const sourceType = process.env.OPENIM_FORWARD_SOURCE ?? "text";
const native = process.env.OPENIM_FORWARD_NATIVE === "1";
const out = resolve(root, "tests/delivery", `windows-387-forward-${label}-${Date.now()}`);
await mkdir(out, { recursive: true });
process.env.TEMP = process.env.TMP = resolve(root, ".cache/temp-web");
const fixture = JSON.parse(await readFile(resolve(root, ".private/test-accounts.json"), "utf8"));
const sender = fixture.users[90], recipients = fixture.users.slice(91, 91 + Math.min(19, Number(process.env.OPENIM_FORWARD_COUNT ?? 5)));
const secrets = new Set([fixture.password, sender.password].filter(Boolean));
for (const value of [...secrets]) secrets.add(createHash("md5").update(value).digest("hex"));
const redact = value => {
  let text = String(value);
  for (const secret of secrets) if (secret.length > 6) text = text.split(secret).join("[REDACTED]");
  return text.replace(/eyJ[A-Za-z0-9_-]{12,}\.[A-Za-z0-9_-]{12,}\.[A-Za-z0-9_-]{12,}/g, "[JWT_REDACTED]");
};
const results = [], errors = [], engineErrors = [], sessionEvents = [];
const base = "https://192.168.10.11:29880";
let browser, page;
if (native) {
  const env = { ...process.env, VITE_DEV_SERVER_URL: vite, OPENIM38_TEST_DATA_DIR: resolve(root, ".cache/electron-forward-387-" + Date.now()) };
  delete env.OPENIM38_SMOKE_TEST;
  browser = await electron.launch({ executablePath: resolve("node_modules/electron/dist/electron.exe"), args: ["."], cwd: process.cwd(), env, timeout: 90000 });
  const limit = Date.now() + 90000;
  while (!page && Date.now() < limit) { page = browser.windows().find(p => p.url().startsWith(vite)); if (!page) await new Promise(r => setTimeout(r, 200)); }
  if (!page) { await browser.close(); throw new Error("Synthetic Electron renderer not found"); }
} else {
  browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE });
  page = await browser.newPage({ viewport: { width: 1440, height: 1000 }, locale: "zh-CN" });
}
page.setDefaultTimeout(15000);
page.on("pageerror", e => errors.push(redact(e.stack ?? e)));
page.on("console", m => { if (/panic:|Go program has already exited|fatal error:|wasm with error|Value\.Get|Value\.Float/.test(m.text())) engineErrors.push(redact(m.text()).slice(0, 3000)); });
const deadline = setTimeout(() => void browser.close(), 6 * 60 * 1000);
async function step(name, fn) {
  const start = Date.now();
  try { const evidence = await fn(); results.push({ name, passed: true, elapsedMs: Date.now() - start, evidence }); console.log(name, "PASS"); return evidence; }
  catch (e) { results.push({ name, passed: false, elapsedMs: Date.now() - start, error: redact(e.stack ?? e) }); throw e; }
}
try {
  await step("Original email login; real WASM and normal TLS", async () => {
    await page.addInitScript(({ base }) => { for (const [key, value] of Object.entries({ apiUrl: base + "/api", chatUrl: base + "/chat", enterpriseUrl: base + "/enterprise/v1", wsUrl: base.replace("https:", "wss:") + "/ws" })) localStorage.setItem("upgrade38:" + key, value); }, { base });
    await page.goto(vite, { waitUntil: "domcontentloaded", timeout: 90000 });
    await page.waitForFunction(() => typeof window.commonEventFunc === "function", undefined, { timeout: 120000 });
    await page.getByRole("tab", { name: "邮箱", exact: true }).click();
    await page.locator("#email").fill(sender.email);
    await page.locator("#password").fill(sender.password ?? fixture.password);
    await page.getByRole("button", { name: "登录", exact: true }).click();
    await page.waitForURL("**/#/chat**", { timeout: 90000 });
    const pair = await page.evaluate(async () => { const s = await import("/src/utils/storage.ts"); return [await s.getIMToken(), await s.getChatToken()]; });
    pair.filter(Boolean).forEach(v => secrets.add(v));
    await page.evaluate(async () => {
      window.forwardTest = { im: (await import("/src/im/ImFacade.ts")).ImFacade, stores: await import("/src/store/index.ts"), emitter: (await import("/src/utils/events.ts")).default, events: [], source: null };
      for (const event of ["OnKickedOffline", "OnUserTokenExpired", "OnUserTokenInvalid", "OnConnectFailed"]) window.forwardTest.im.on(event, e => window.forwardTest.events.push({ event, errCode: e.errCode }));
    });
    await page.waitForFunction(async () => {
      try { return (await window.forwardTest.im.getLoginStatus()).data === 3 && !!window.forwardTest.stores.useUserStore.getState().selfInfo.userID; }
      catch { return false; }
    }, undefined, { timeout: 60000 });
    await page.waitForTimeout(1500);
    return { userID: sender.userID, platform: native ? "Native Windows Electron" : "Web real WASM; PC native validation separate", normalTLS: true };
  });
  await step("Seed only synthetic recent conversations and a sent source message", async () => {
    return page.evaluate(async ({ recipients, marker, sourceType }) => {
      const f = window.forwardTest, { im, stores } = f;
      const sent = [];
      const invoke = async (label, fn) => { try { return await fn(); } catch (e) { throw new Error(label + " code=" + e?.errCode + " message=" + (e?.errMsg ?? e?.message ?? "unknown")); } };
      for (const recipient of recipients) {
        const { data: message } = await invoke("create seed", () => im.createTextMessage(marker + " seed " + recipient.userID));
        const { data } = await invoke("send seed", () => im.sendMessage({ recvID: recipient.userID, groupID: "", message }));
        sent.push(data.clientMsgID);
      }
      let sourceID = recipients[0].userID, sessionType = 1, sourceClientMsgID = sent[0];
      if (sourceType === "group") {
        const { data: group } = await invoke("create synthetic group", () => im.createGroup({ groupInfo: { groupType: 2, groupName: marker + " group" }, memberUserIDs: recipients.slice(0, 2).map(r => r.userID), adminUserIDs: [] }));
        sourceID = group.groupID; sessionType = 3;
        const { data: message } = await im.createTextMessage(marker + " group source");
        const { data } = await invoke("send group source", () => im.sendMessage({ recvID: "", groupID: sourceID, message }));
        sourceClientMsgID = data.clientMsgID;
      }
      const { data: c } = await im.getOneConversation({ sourceID, sessionType });
      const { data: history } = await im.getAdvancedHistoryMessageList({ conversationID: c.conversationID, count: 30, startClientMsgID: "", viewType: 0 });
      f.source = history.messageList.find(m => m.clientMsgID === sourceClientMsgID);
      if (!f.source) throw new Error("Synthetic source did not synchronize");
      if (sourceType === "quote") {
        const { data: quote } = await im.createQuoteMessage({ text: marker + " quoted reply", message: JSON.stringify(f.source) });
        f.source = (await invoke("send source quoted reply", () => im.sendMessage({ recvID: recipients[0].userID, groupID: "", message: quote }))).data;
      }
      stores.useConversationStore.getState().updateCurrentConversation(c);
      await stores.useConversationStore.getState().getConversationListByReq();
      location.hash = "/chat/" + c.conversationID;
      return { sentCount: sent.length, sourceClientMsgID: f.source.clientMsgID, sourceSeq: f.source.seq, sourceConversationID: c.conversationID };
    }, { recipients: recipients.map(a => ({ userID: a.userID })), marker: "OpenIM387 forward " + Date.now(), sourceType });
  });
  await step("Original forward selector confirms multiple recent recipients", async () => {
    await page.evaluate(() => window.forwardTest.emitter.emit("OPEN_CHOOSE_MODAL", { type: "FORWARD_MESSAGE", extraData: window.forwardTest.source }));
    const modal = page.locator(".ant-modal:visible");
    await modal.getByText("最近聊天", { exact: true }).click();
    for (const account of recipients) {
      const input = modal.locator("input").first();
      await input.fill(account.nickname); await input.press("Enter");
      const target = modal.locator('[data-virtuoso-scroller="true"]').getByText(account.nickname, { exact: true });
      await target.first().click();
    }
    await modal.getByRole("button", { name: /^(确 认|确认)$/ }).click();
    await modal.waitFor({ state: "hidden", timeout: 40000 });
    await page.waitForTimeout(3000);
    return { selectedRecipients: recipients.map(a => a.userID), location: await page.evaluate(() => location.hash) };
  });
  await step("Session survives and original source appears once per recipient", async () => {
    return page.evaluate(async recipients => {
      const { im, source, events } = window.forwardTest;
      const self = (await im.getSelfUserInfo()).data;
      const found = [];
      for (const userID of recipients) {
        const { data: c } = await im.getOneConversation({ sourceID: userID, sessionType: 1 });
        const { data: history } = await im.getAdvancedHistoryMessageList({ conversationID: c.conversationID, count: 30, startClientMsgID: "", viewType: 0 });
        const forwards = history.messageList.filter(m => m.clientMsgID !== source.clientMsgID && m.contentType === source.contentType && (source.quoteElem ? m.quoteElem?.text === source.quoteElem.text : m.textElem?.content === source.textElem?.content));
        if (forwards.length !== 1) throw new Error("Forward count " + forwards.length + " for " + userID);
        found.push({ userID, count: forwards.length, clientMsgID: forwards[0].clientMsgID });
      }
      return { self: self.userID, found, sessionEvents: events };
    }, recipients.map(a => a.userID));
  });
} catch (error) { console.log("FORWARD_RUNTIME_FAIL", redact(error.message ?? error)); process.exitCode = 1; }
finally {
  try { sessionEvents.push(...await page.evaluate(() => window.forwardTest?.events ?? [])); } catch {}
  await writeFile(resolve(out, "report.json"), JSON.stringify({ phase: label, results, errors, engineErrors, sessionEvents, limits: ["Only synthetic accounts 90-95", "Real Web SDK and original selector; native Windows is a separate verification", "No raw credentials, headers or tokens saved"] }, null, 2));
  clearTimeout(deadline);
  if (native) {
    await browser.evaluate(({ BrowserWindow }) => { for (const w of BrowserWindow.getAllWindows()) w.destroy(); }).catch(() => {});
    await Promise.race([browser.close().catch(() => {}), new Promise(r => setTimeout(r, 5000)).then(() => { if (browser.process().exitCode === null) browser.process().kill(); })]);
  } else await browser.close();
  console.log("REPORT", resolve(out, "report.json"));
}
if (errors.length || engineErrors.length || sessionEvents.some(e => e.event !== "OnConnectFailed")) process.exitCode = 1;
