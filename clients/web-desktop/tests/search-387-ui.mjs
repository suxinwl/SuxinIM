import { chromium } from "playwright";
import { readFile, writeFile, mkdir } from "node:fs/promises";
import { resolve } from "node:path";
import { createHash } from "node:crypto";

const project = resolve("../.."), mode = process.env.SEARCH_PHASE || "after";
const out = resolve(project, "tests/delivery/windows-387-search-" + mode + "-" + new Date().toISOString().replace(/[-:.]/g, ""));
await mkdir(out, { recursive: true });
const fixture = JSON.parse(await readFile(resolve(project, ".private/test-accounts.json"), "utf8"));
const account = fixture.users[106], peer = fixture.users[107];
const password = account.password ?? fixture.password;
const secrets = [password, createHash("md5").update(password).digest("hex")];
const redact = value => { let text = String(value); for (const secret of secrets) if (secret) text = text.split(secret).join("[REDACTED]"); return text.replace(/eyJ[A-Za-z0-9_-]{15,}\.[A-Za-z0-9_-]{20,}\.[A-Za-z0-9_-]{15,}/g, "[JWT_REDACTED]"); };
const report = { phase: mode, normalTLS: true, injectedCredentials: false, originalSearchUI: true, steps: [], pageErrors: [], syntheticAccount: account.userID };
const base = "https://192.168.10.11:29880", marker = "Search387" + Date.now();
let browser, page;
const wait = ms => new Promise(resolve => setTimeout(resolve, ms));
async function until(fn, timeout = 15000) { const end = Date.now() + timeout; while (Date.now() < end) { const r = await fn(); if (r) return r; await wait(150); } throw new Error("Synthetic search UI did not converge"); }
const record = async (name, fn) => { try { const evidence = await fn(); report.steps.push({ name, pass: true, evidence }); console.log(name + " PASS"); } catch (error) { report.steps.push({ name, pass: false, error: redact(error.stack ?? error) }); console.log(name + " FAIL"); if (mode !== "before") throw error; } };
async function screenshot(name) { if (await page.locator('input[type="password"]').count()) throw new Error("Screenshot login guard"); await page.screenshot({ path: resolve(out, name), fullPage: false }); }
try {
  browser = await chromium.launch({ headless: true, executablePath: "C:/Program Files/Google/Chrome/Application/chrome.exe" });
  const context = await browser.newContext({ locale: "zh-CN", viewport: { width: 1280, height: 900 } }); page = await context.newPage(); page.setDefaultTimeout(15000);
  page.on("pageerror", error => report.pageErrors.push(redact(error.message)));
  await page.addInitScript(({ base }) => { for (const [key, value] of Object.entries({ apiUrl: base + "/api", chatUrl: base + "/chat", enterpriseUrl: base + "/enterprise/v1", wsUrl: base.replace("https:", "wss:") + "/ws" })) localStorage.setItem("upgrade38:" + key, value); }, { base });
  await page.goto(process.env.SEARCH_VITE || "http://127.0.0.1:7777", { waitUntil: "domcontentloaded" });
  await page.waitForFunction(() => typeof window.commonEventFunc === "function", undefined, { timeout: 90000 });
  await page.getByRole("tab", { name: "邮箱", exact: true }).click();
  await page.locator("#email").fill(account.email); await page.locator("#password").fill(password);
  await page.getByRole("button", { name: "登录", exact: true }).click();
  await page.waitForURL("**/#/chat**", { timeout: 90000 });
  await page.evaluate(async () => { window.__searchSDK = (await import("/src/im/ImFacade.ts")).ImFacade; window.__searchStores = await import("/src/store/index.ts"); });
  await until(() => page.evaluate(() => Boolean(window.__searchStores.useUserStore.getState().selfInfo.userID)));
  const cid = await page.evaluate(async peer => { const { data } = await window.__searchSDK.getOneConversation({ sourceID: peer, sessionType: 1 }); window.__searchStores.useConversationStore.getState().updateCurrentConversation(data); location.hash = "/chat/" + data.conversationID; return data.conversationID; }, peer.userID);
  await page.locator('#editable-div[contenteditable="true"]').waitFor({ state: "visible" });
  await page.locator('#editable-div[contenteditable="true"]').fill(marker + " original search regression");
  await page.getByRole("button", { name: "发送", exact: true }).click();
  const anchor = await until(() => page.evaluate(marker => { const m = window.__searchStores.useMessageStore.getState().historyMessageList.find(m => m.textElem?.content?.includes(marker)); return m?.status === 2 ? { clientMsgID: m.clientMsgID, seq: m.seq } : false; }, marker));
  // Pinned core's default search cutoff rounds time.Now down to whole seconds.
  await wait(4000);
  report.sdkDiagnostics = await page.evaluate(async ({ marker, cid, anchor }) => {
    const basic = { conversationID: "", keywordList: [marker], messageTypeList: [101] };
    const full = { ...basic, keywordListMatchType: 0, senderUserIDList: [], searchTimePosition: 0, searchTimePeriod: 0, pageIndex: 1, count: 20 };
    const evidence = [];
    for (const [name, request] of [["original-global", basic], ["explicit-global", full], ["explicit-conversation", { ...full, conversationID: cid }], ["future-cutoff", { ...full, conversationID: cid, searchTimePosition: Math.ceil(Date.now()/1000)+60 }], ["no-keyword", { ...full, conversationID: cid, keywordList: [] }]]) {
      try { const { data, errCode } = await window.__searchSDK.searchLocalMessages(request); evidence.push({ name, errCode, keys: Object.keys(data || {}), totalCount: data.totalCount, items: data.searchResultItems?.map(r => ({ conversationID: r.conversationID, messageCount: r.messageCount, listLength: r.messageList?.length })) }); }
      catch (e) { evidence.push({ name, errCode: e.errCode, errMsg: e.errMsg, message: e.message }); }
    }
    const { data } = await window.__searchSDK.findMessageList([{ conversationID: cid, clientMsgIDList: [anchor.clientMsgID] }]);
    evidence.push({ name: "find-anchor", now: Date.now(), totalCount: data.totalCount, keys: Object.keys(data || {}), found: (data.findResultItems || data.searchResultItems || []).flatMap(r => r.messageList || []).map(m => ({ clientMsgID: m.clientMsgID, seq: m.seq, contentType: m.contentType, sendTime: m.sendTime, hasTextElem: Boolean(m.textElem?.content), contentLength: m.content?.length })) });
    if (typeof window.searchMessageByKeyword === "function") {
      try { const raw = await window.searchMessageByKeyword(cid, "[101]", JSON.stringify([marker]), 0, 0, Date.now()+10000, 0, 20); const obj = typeof raw === "string" ? JSON.parse(raw) : raw; evidence.push({ name: "direct-sql-search", keys: Object.keys(obj || {}), errCode: obj.errCode, dataLength: obj.data?.length }); } catch(e) { evidence.push({ name: "direct-sql-search", error: e.message }); }
    }
    return evidence;
  }, { marker, cid, anchor });
  await record("Pinned SDK returns actual matching local messages", async () => {
    const data = await page.evaluate(async ({ marker, cid }) => { const { data } = await window.__searchSDK.searchLocalMessages({ conversationID: "", keywordList: [marker], messageTypeList: [101, 106, 108, 114, 105, 109, 107] }); return { totalCount: data.totalCount, resultCount: data.searchResultItems?.length, matchingConversation: data.searchResultItems?.some(x => x.conversationID === cid && x.messageList.some(m => m.textElem?.content?.includes(marker))) }; }, { marker, cid });
    if (!data.matchingConversation) throw new Error("SDK failed to return the synthetic search result"); return data;
  });
  const openSearch = async () => { await page.locator('img[src*="topSearchBar/search"]').locator("..").click(); const modal = page.locator(".ant-modal:visible"); await modal.getByRole("tab", { name: "综合", exact: true }).click(); await modal.locator("input").fill(marker); await until(() => modal.locator('[id^="dashboard-item-"]').count()); return modal; };
  let modal = await openSearch();
  const historyPane = () => modal.getByRole("tabpanel", { name: "聊天记录", exact: true });
  const detailText = () => historyPane().locator('[data-virtuoso-scroller="true"]').nth(1).getByText(marker + " original search regression", { exact: true });
  await record("Overview result selects the matching conversation and displays its message", async () => {
    await modal.locator('[id^="dashboard-item-"]').filter({ hasText: "original search regression" }).click();
    await until(async () => await modal.locator('[id^="conversation-item-"]').count());
    const detail = detailText();
    await detail.waitFor({ state: "visible", timeout: 4500 });
    await screenshot("overview-message-selected.png"); return { visibleResults: await detail.count() };
  });
  if (mode === "before") { await screenshot("before-overview-blank.png"); }
  await record("Chat-history tab has usable list height and selects the first result", async () => {
    await modal.getByRole("tab", { name: "聊天记录", exact: true }).click();
    const geo = await historyPane().locator('[data-virtuoso-scroller="true"]').evaluateAll(els => els.map(e => ({ width: e.getBoundingClientRect().width, height: e.getBoundingClientRect().height })));
    report.heightChain = await historyPane().locator('[data-virtuoso-scroller="true"]').first().evaluate(el => { const chain = []; for (let e = el; e && chain.length < 14; e = e.parentElement) { const s = getComputedStyle(e); chain.push({ tag: e.tagName, className: e.className, rectHeight: e.getBoundingClientRect().height, cssHeight: s.height, display: s.display, flex: s.flex, overflow: s.overflow, position: s.position }); } return chain; });
    if (!geo.length || geo.some(x => x.height < 100)) throw new Error("Chat-history virtual lists collapsed: " + JSON.stringify(geo));
    await detailText().waitFor({ state: "visible", timeout: 4500 }); return geo;
  });
  if (mode !== "before") {
    await record("Double-clicking the original result opens the corresponding chat and anchors its actual message", async () => {
      await detailText().dblclick();
      await until(() => page.evaluate(({ cid, anchor }) => location.hash === "#/chat/" + cid && !window.__searchStores.useMessageStore.getState().jumpLoading && window.__searchStores.useMessageStore.getState().jumpClientMsgID === anchor.clientMsgID && Boolean(document.getElementById("chat_" + anchor.clientMsgID)), { cid, anchor }));
      const value = await page.locator("#chat_" + anchor.clientMsgID).boundingBox(); if (!value || value.y < 0 || value.y > 900) throw new Error("Anchor message is outside visible chat viewport");
      await screenshot("search-anchor-visible.png"); return { conversationID: cid, clientMsgID: anchor.clientMsgID, visibleRect: value };
    });
    await record("Searching from contacts navigates back to the matching conversation and visible anchor", async () => {
      await page.locator("aside").getByText("通讯录", { exact: true }).click();
      await page.waitForURL("**/#/contact**");
      modal = await openSearch();
      await modal.locator('[id^="dashboard-item-"]').filter({ hasText: "original search regression" }).click();
      await detailText().waitFor({ state: "visible" });
      await detailText().dblclick();
      await until(() => page.evaluate(({ cid, anchor }) => location.hash === "#/chat/" + cid && !window.__searchStores.useMessageStore.getState().jumpLoading && window.__searchStores.useMessageStore.getState().jumpClientMsgID === anchor.clientMsgID && Boolean(document.getElementById("chat_" + anchor.clientMsgID)), { cid, anchor }));
      const rect = await page.locator("#chat_" + anchor.clientMsgID).boundingBox(); if (!rect || rect.y < 0 || rect.y > 900) throw new Error("Cross-route anchor is outside the visible chat");
      await screenshot("contacts-search-anchor-visible.png"); return { conversationID: cid, clientMsgID: anchor.clientMsgID, visibleRect: rect };
    });
  }
  report.status = report.steps.every(s => s.pass) ? "PASS" : "REPRODUCED_FAILURE";
} catch (error) { report.status = "FAIL"; report.error = redact(error.stack ?? error); process.exitCode = 1; }
finally { await browser?.close(); await writeFile(resolve(out, "report.json"), JSON.stringify(report, null, 2)); console.log(JSON.stringify({ report: resolve(out, "report.json"), status: report.status, tests: report.steps.map(s => ({ name: s.name, pass: s.pass })) })); }
