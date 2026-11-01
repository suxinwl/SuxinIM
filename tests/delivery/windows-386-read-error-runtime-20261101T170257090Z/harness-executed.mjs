import { chromium, _electron as electron } from "playwright";
import { createHash } from "node:crypto";
import { mkdir, readFile, writeFile } from "node:fs/promises";
import { resolve } from "node:path";

const project = resolve("../..");
const fixture = JSON.parse(await readFile(resolve(project, ".private/test-accounts.json"), "utf8"));
const senderAccount = fixture.users[101], receiverAccount = fixture.users[102];
if (!senderAccount?.email || !receiverAccount?.email) throw new Error("Synthetic account indexes are unavailable");
const secretValues = new Set();
const rememberSecrets = (value, key = "") => {
  if (Array.isArray(value)) return value.forEach(item => rememberSecrets(item, key));
  if (value && typeof value === "object") return Object.entries(value).forEach(([name, item]) => rememberSecrets(item, name));
  if (typeof value !== "string" || value.length < 8 || !/password|token/i.test(key) || /sha|hash/i.test(key)) return;
  secretValues.add(value);
  if (/password/i.test(key)) {
    const md5 = createHash("md5").update(value).digest("hex");
    secretValues.add(md5); secretValues.add(md5.toUpperCase());
  }
};
rememberSecrets(fixture);
const clean = value => {
  let text = String(value);
  for (const secret of [...secretValues].sort((a, b) => b.length - a.length)) text = text.split(secret).join("[REDACTED]");
  return text.replace(/eyJ[A-Za-z0-9_-]{15,}\.[A-Za-z0-9_-]{20,}\.[A-Za-z0-9_-]{15,}/g, "[REDACTED_JWT]");
};
const output = resolve(project, "tests/delivery/windows-386-read-error-runtime-" + new Date().toISOString().replace(/[-:.]/g, ""));
await mkdir(output, { recursive: true });
const progress = [], errors = [], stages = [], windowStates = [], screenshots = [], receiptPolls = [], exceptionDiagnostics = [];
const log = message => { const safe = clean(message); progress.push({ utc: new Date().toISOString(), message: safe }); console.log(safe); };
const base = "https://192.168.10.11:29880", vite = "http://127.0.0.1:7777";
const started = Date.now(), deadline = Math.min(started + 10 * 60 * 1000, process.env.READ_RECEIPT_DEADLINE_UTC ? Date.parse(process.env.READ_RECEIPT_DEADLINE_UTC) : Infinity);
const sleep = ms => new Promise(resolve => setTimeout(resolve, ms));
const checkDeadline = () => { if (Date.now() >= deadline) throw new Error("The 10 minute test deadline was reached"); };
const until = async (fn, label, timeout = 30000) => {
  const limit = Math.min(deadline, Date.now() + timeout);
  let last;
  while (Date.now() < limit) {
    checkDeadline();
    last = await fn();
    if (last) return last;
    await sleep(300);
  }
  throw new Error(label + " (condition not reached)");
};
const configScript = ({ base }) => {
  for (const [key, value] of Object.entries({ apiUrl: base + "/api", chatUrl: base + "/chat", enterpriseUrl: base + "/enterprise/v1", wsUrl: base.replace("https:", "wss:") + "/ws" })) localStorage.setItem("upgrade38:" + key, value);
};
function observe(page, client) {
  page.setDefaultTimeout(25000);
  page.on("pageerror", error => errors.push({ client, utc: new Date().toISOString(), error: clean(error.message) }));
  page.on("response", async response => {
    if (response.url().includes("/chat/account/login")) {
      try { rememberSecrets(await response.json()); } catch { /* no raw body is logged */ }
    }
  });
}
async function loginOriginal(page, account, client) {
  await page.waitForFunction(() => typeof window.commonEventFunc === "function", undefined, { timeout: 90000 });
  const emailTab = page.getByRole("tab", { name: "邮箱", exact: true });
  await emailTab.click();
  const guard = await page.evaluate(() => {
    const email = document.getElementById("email"), password = document.getElementById("password");
    return {
      emailCount: document.querySelectorAll("#email").length,
      passwordCount: document.querySelectorAll("#password").length,
      isInput: email instanceof HTMLInputElement && password instanceof HTMLInputElement,
      passwordType: password?.getAttribute("type"),
      sameForm: Boolean(email?.closest("form") && email.closest("form") === password?.closest("form")),
      loginRoute: location.hash.startsWith("#/login"),
    };
  });
  if (guard.emailCount !== 1 || guard.passwordCount !== 1 || !guard.isInput || guard.passwordType !== "password" || !guard.sameForm || !guard.loginRoute) throw new Error("Original login form identity guard failed for " + client);
  stages.push({ stage: "original-login-field-guard", client, result: guard });
  await page.locator("#email").fill(account.email);
  await page.locator("#password").fill(account.password ?? fixture.password);
  await page.getByRole("button", { name: "登录", exact: true }).click();
  await page.waitForURL("**/#/chat**", { timeout: 90000 });
  await page.waitForFunction(() => !document.querySelector("#password") && !document.querySelector(".ant-spin-spinning"), undefined, { timeout: 60000 });
  await page.evaluate(async () => {
    window.__readTestIm = (await import("/src/im/ImFacade.ts")).ImFacade;
    window.__readTestStores = await import("/src/store/index.ts");
  });
  await until(() => page.evaluate(() => Boolean(window.__readTestStores.useUserStore.getState().selfInfo.userID)), client + " self info not loaded");
  const actualUserID = await page.evaluate(() => window.__readTestStores.useUserStore.getState().selfInfo.userID);
  if (actualUserID !== account.userID) throw new Error("Synthetic login identity mismatch for " + client);
  stages.push({ stage: "original-login-complete", client, userID: actualUserID });
  log(client + " logged in through the original form");
}
let browser, electronApp, sender, receiver, mainWindow, foregroundControl;
const result = { status: "RUNNING", scope: "Synthetic 101 sender / 102 normal visible Electron receiver; actual pinned SDK transport and original UI", startedUtc: new Date().toISOString(), productionSourceChanged: false, offscreenSmokeMode: false, output };
try {
  browser = await chromium.launch({ headless: true, executablePath: "C:/Program Files/Google/Chrome/Application/chrome.exe" });
  sender = await browser.newPage(); observe(sender, "browser-sender");
  await sender.addInitScript(configScript, { base });
  await sender.goto(vite, { waitUntil: "domcontentloaded" });
  await loginOriginal(sender, senderAccount, "browser-sender");
  const env = { ...process.env, VITE_DEV_SERVER_URL: vite, OPENIM38_TEST_DATA_DIR: resolve(project, ".cache/electron-read-386-" + Date.now()) };
  delete env.OPENIM38_SMOKE_TEST;
  electronApp = await electron.launch({ executablePath: resolve("node_modules/electron/dist/electron.exe"), args: ["."], cwd: process.cwd(), env, timeout: 90000 });
  await until(async () => {
    receiver = electronApp.windows().find(page => page.url().startsWith(vite));
    return receiver;
  }, "Normal Electron application page not found", 90000);
  observe(receiver, "windows-receiver");
  await receiver.addInitScript(configScript, { base });
  await receiver.reload({ waitUntil: "domcontentloaded" });
  const receiverCDP = await receiver.context().newCDPSession(receiver);
  await receiverCDP.send("Emulation.setFocusEmulationEnabled", { enabled: false });
  result.receiverFocusEmulationExplicitlyDisabled = true;
  const safeURL = value => {
    try { const url = new URL(value); return { origin: url.origin, path: url.pathname }; }
    catch { return { origin: "unparseable", path: "" }; }
  };
  receiverCDP.on("Runtime.exceptionThrown", async event => {
    const detail = event.exceptionDetails;
    const record = {
      utc: new Date().toISOString(),
      text: clean(detail.text),
      location: { ...safeURL(detail.url || ""), line: detail.lineNumber, column: detail.columnNumber },
      stack: (detail.stackTrace?.callFrames || []).slice(0, 12).map(frame => ({ ...safeURL(frame.url), line: frame.lineNumber, column: frame.columnNumber })),
      fields: {},
      keys: [],
    };
    try {
      if (detail.exception?.objectId) {
        const properties = await receiverCDP.send("Runtime.getProperties", { objectId: detail.exception.objectId, ownProperties: true, accessorPropertiesOnly: false, generatePreview: false });
        record.keys = properties.result.slice(0, 50).map(property => clean(property.name));
        for (const property of properties.result) {
          if (!["errCode", "errMsg", "message", "name"].includes(property.name)) continue;
          const value = property.value?.value;
          if (["string", "number", "boolean"].includes(typeof value) || value === null) record.fields[property.name] = typeof value === "string" ? clean(value) : value;
        }
      }
    } catch {
      record.inspection = "White-listed exception properties were unavailable";
    }
    exceptionDiagnostics.push(record);
  });
  await receiverCDP.send("Runtime.enable");
  mainWindow = await electronApp.browserWindow(receiver);
  await mainWindow.evaluate(window => { window.show(); window.restore(); window.focus(); });
  await loginOriginal(receiver, receiverAccount, "windows-receiver");
  result.runtime = await receiver.evaluate(() => window.electronAPI.ipcInvoke("getRuntimeVersions"));
  if (result.runtime.app !== "3.8.6") throw new Error("Unexpected tested Electron app version");

  const selectConversation = async (page, peer, native) => {
    const conversation = await page.evaluate(async id => (await window.__readTestIm.getOneConversation({ sourceID: id, sessionType: 1 })).data, peer.userID);
    await page.evaluate(conversation => { window.__readTestStores.useConversationStore.getState().updateCurrentConversation(conversation); location.hash = "/chat/" + conversation.conversationID; }, conversation);
    stages.push({ stage: "open-original-chat-ui", client: native ? "windows-receiver" : "browser-sender", selection: "Actual SDK conversation selected via normal store and route; no receipt or message transport was injected", conversationID: conversation.conversationID });
    await until(() => page.evaluate(id => location.hash === "#/chat/" + id && Boolean(document.querySelector("#chat-main")) && window.__readTestStores.useConversationStore.getState().currentConversation?.conversationID === id, conversation.conversationID), "Selected conversation did not finish loading");
    return conversation.conversationID;
  };
  const senderCID = await selectConversation(sender, receiverAccount, false);
  const receiverCID = await selectConversation(receiver, senderAccount, true);
  result.conversations = { senderCID, receiverCID };
  const state = async label => {
    const native = await mainWindow.evaluate(window => ({ isVisible: window.isVisible(), isMinimized: window.isMinimized(), isFocused: window.isFocused(), destroyed: window.isDestroyed() }));
    const renderer = await receiver.evaluate(() => ({ visibilityState: document.visibilityState, hasFocus: document.hasFocus(), hash: location.hash, chatLoaded: Boolean(document.querySelector("#chat-main")) }));
    const record = { label, utc: new Date().toISOString(), native, renderer };
    windowStates.push(record); return record;
  };
  const ensureForeground = async label => {
    await mainWindow.evaluate(window => { window.show(); window.restore(); window.focus(); });
    return until(async () => { const s = await state(label); return s.native.isVisible && !s.native.isMinimized && s.native.isFocused && s.renderer.visibilityState === "visible" && s.renderer.hasFocus ? s : false; }, "Receiver is not actually in foreground");
  };
  const send = async label => {
    const marker = crypto.randomUUID();
    const text = "Read receipt regression 386 " + label + " " + marker;
    await sender.locator('#editable-div[contenteditable="true"]').fill(text);
    await sender.getByRole("button", { name: "发送", exact: true }).click();
    const sent = await until(async () => {
      const observed = await sender.evaluate(marker => {
      const message = window.__readTestStores.useMessageStore.getState().historyMessageList.find(item => item.textElem?.content?.includes(marker));
      return message ? { clientMsgID: message.clientMsgID, seq: message.seq, status: message.status } : { missing: true };
      }, marker);
      receiptPolls.push({ phase: "original-ui-send", label, utc: new Date().toISOString(), ...observed });
      return observed.status === 2 ? observed : false;
    }, "Original sender Send flow did not complete");
    return { ...sent, label };
  };
  const receipt = message => sender.evaluate(async ({ cid, id }) => {
    const { data } = await window.__readTestIm.getAdvancedHistoryMessageList({ conversationID: cid, startClientMsgID: "", count: 100, viewType: 0 });
    const found = data.messageList.find(item => item.clientMsgID === id);
    const row = document.getElementById("chat_" + id);
    const labels = row ? [...row.querySelectorAll("div")].filter(el => el.children.length === 0).map(el => el.textContent.trim()).filter(text => ["已读", "未读", "Read", "Unread"].includes(text)) : [];
    return { clientMsgID: id, sdkIsRead: found?.isRead, seq: found?.seq, uiReceipt: labels.at(-1) ?? null, rowPresent: Boolean(row) };
  }, { cid: senderCID, id: message.clientMsgID });
  const assertRead = async message => until(async () => {
    const value = await receipt(message);
    receiptPolls.push({ label: message.label, utc: new Date().toISOString(), ...value });
    return value.sdkIsRead === true && ["已读", "Read"].includes(value.uiReceipt) ? value : false;
  }, "Foreground message did not become read in SDK and original sender UI", 30000);
  const assertUnreadHeld = async (messages, label, duration = 3500) => {
    const observations = [];
    const end = Date.now() + duration;
    while (Date.now() < end) {
      for (const message of messages) {
        const value = await receipt(message);
        observations.push({ utc: new Date().toISOString(), ...value });
        if (value.sdkIsRead !== false || !["未读", "Unread"].includes(value.uiReceipt)) throw new Error(label + " unexpectedly read or missing SDK/UI state: " + JSON.stringify(value));
      }
      await sleep(450);
    }
    return observations;
  };
  const safeScreenshot = async (page, filename) => {
    const safe = await page.evaluate(() => !location.hash.startsWith("#/login") && !document.querySelector("#password") && !document.querySelector('input[type="password"]'));
    if (!safe) throw new Error("Post-login screenshot guard failed");
    const path = resolve(output, filename);
    await page.screenshot({ path }); screenshots.push(path);
  };
  await ensureForeground("initial-front");
  const first = await send("foreground");
  const firstReceipt = await assertRead(first);
  stages.push({ stage: "foreground-new-message-read", message: first, receipt: firstReceipt });
  await safeScreenshot(sender, "sender-foreground-read.png");
  log("Foreground actual SDK and original UI show read");

  await receiver.locator('img[alt="win_close"]').click();
  await until(async () => { const s = await state("close-to-tray"); return !s.native.isVisible && !s.native.isFocused && !s.renderer.hasFocus; }, "Close did not actually hide the native window");
  const hiddenOne = await send("tray-1"); await sleep(800); const hiddenTwo = await send("tray-2");
  const hiddenObservations = await assertUnreadHeld([hiddenOne, hiddenTwo], "Tray-hidden messages");
  stages.push({ stage: "two-consecutive-tray-hidden-messages-unread", messages: [hiddenOne, hiddenTwo], observations: hiddenObservations });
  await safeScreenshot(sender, "sender-tray-hidden-unread.png");
  log("Two later tray-hidden messages remain unread in actual SDK and sender UI");
  await ensureForeground("restore-after-tray");
  stages.push({ stage: "tray-restore-pending-messages-read", receipts: [await assertRead(hiddenOne), await assertRead(hiddenTwo)] });

  if (process.env.READ_RECEIPT_CORE_ONLY === "1") {
    await safeScreenshot(receiver, "receiver-restored-conversation.png");
    await safeScreenshot(sender, "sender-final-all-read.png");
    result.status = "PASS_CORE";
    result.deferredRuntimeChecks = ["Minimize and visible-unfocused native UI checks omitted to honor the original 10 minute window; behavior gate coverage remains separate."];
    log("Core tray-hidden and restored actual SDK/UI checks passed");
  } else {

  await receiver.locator('img[alt="win_min"]').click();
  await until(async () => { const s = await state("minimized"); return s.native.isMinimized && !s.native.isFocused && !s.renderer.hasFocus; }, "Native receiver did not minimize");
  const minimized = await send("minimized");
  stages.push({ stage: "minimized-message-unread", message: minimized, observations: await assertUnreadHeld([minimized], "Minimized message") });
  await ensureForeground("restore-after-minimize");
  stages.push({ stage: "minimized-restore-message-read", receipt: await assertRead(minimized) });
  log("Minimized receiver keeps new messages unread and reads after restore");

  foregroundControl = await electronApp.evaluateHandle(async ({ BrowserWindow }) => {
    const control = new BrowserWindow({ width: 360, height: 130, show: true, alwaysOnTop: true, title: "Synthetic read regression focus control", webPreferences: { sandbox: true, contextIsolation: true } });
    await control.loadURL("data:text/html,<body>Synthetic focus control - no credentials</body>");
    control.show(); control.focus(); return control;
  });
  await until(async () => { const s = await state("visible-unfocused"); return s.native.isVisible && !s.native.isMinimized && !s.native.isFocused && !s.renderer.hasFocus; }, "Focus control did not defocus the visible receiver");
  const unfocused = await send("visible-unfocused");
  stages.push({ stage: "visible-unfocused-message-unread", message: unfocused, observations: await assertUnreadHeld([unfocused], "Visible unfocused message") });
  await foregroundControl.evaluate(window => window.destroy()); foregroundControl = undefined;
  await ensureForeground("restore-after-unfocused");
  stages.push({ stage: "refocus-message-read", receipt: await assertRead(unfocused) });
  await safeScreenshot(receiver, "receiver-restored-conversation.png");
  await safeScreenshot(sender, "sender-final-all-read.png");
  log("Visible but unfocused receiver keeps message unread and reads after refocus");
  result.status = "PASS";
  }
} catch (error) {
  result.status = "FAIL";
  result.failure = clean(error?.stack ?? error);
  log("Runtime regression ended: " + result.failure);
} finally {
  if (foregroundControl) await foregroundControl.evaluate(window => window.destroy()).catch(() => {});
  if (mainWindow) await mainWindow.evaluate(window => { if (!window.isDestroyed()) window.destroy(); }).catch(() => {});
  if (electronApp) {
    await Promise.race([electronApp.close().catch(() => {}), sleep(5000).then(() => {
      if (electronApp.process().exitCode === null) {
        result.cleanupForceTerminatedOwnTestProcess = true;
        electronApp.process().kill();
      }
    })]);
  }
  if (browser) await browser.close().catch(() => {});
  result.finishedUtc = new Date().toISOString(); result.elapsedSeconds = (Date.now() - started) / 1000;
  result.stages = stages; result.windowStates = windowStates; result.receiptPolls = receiptPolls; result.errors = errors; result.screenshots = screenshots; result.exceptionDiagnostics = exceptionDiagnostics;
  result.diagnosticCollectionScope = "Runtime.exceptionThrown text; stack origin/path/line; own property names; only primitive errCode/errMsg/message/name values. No descriptions, Axios config, headers, body, token properties or complete exception object were serialized.";
  result.constraints = ["Original email/password forms were guarded and used; no tokens were injected.", "IM sends, cache history and receipts used the actual fixed WASM facade and test server.", "Only synthetic fixture users 101 and 102 were used.", "The auxiliary focus-control window belongs only to this test harness.", "No production source, server configuration, Android or media tests were changed."];
  await writeFile(resolve(output, "report.json"), clean(JSON.stringify(result, null, 2)), "utf8");
  await writeFile(resolve(output, "progress.json"), clean(JSON.stringify(progress, null, 2)), "utf8");
  console.log(clean(JSON.stringify({ status: result.status, elapsedSeconds: result.elapsedSeconds, report: resolve(output, "report.json"), stages: stages.map(value => value.stage), errorCount: errors.length })));
}
if (!["PASS", "PASS_CORE"].includes(result.status)) process.exitCode = 1;
