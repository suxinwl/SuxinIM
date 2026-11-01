import { chromium } from "playwright";
import { readFile, writeFile, mkdir } from "node:fs/promises";
import { resolve } from "node:path";
import { createHash } from "node:crypto";

const root = resolve("../.."), out = resolve(root, "tests/delivery/windows-386-moments-ui");
await mkdir(out, { recursive: true });
const temp = resolve(root, ".cache/temp-web"); await mkdir(temp, { recursive: true });
process.env.TEMP = temp; process.env.TMP = temp;
const fixture = JSON.parse(await readFile(resolve(root, ".private/test-accounts.json"), "utf8"));
const accounts = [fixture.users[104], fixture.users[105]], base = "https://192.168.10.11:29880";
const secrets = [];
const addSecret = (value) => { if (typeof value === "string" && value.length > 6) secrets.push(value); };
for (const a of accounts) { const password = a.password ?? fixture.password; addSecret(password); addSecret(createHash("md5").update(password).digest("hex")); }
const redact = (value) => {
  let text = String(value);
  for (const secret of [...secrets].sort((a, b) => b.length - a.length)) text = text.split(secret).join("[REDACTED]");
  return text.replace(/eyJ[A-Za-z0-9_-]{15,}\.[A-Za-z0-9_-]{20,}\.[A-Za-z0-9_-]{15,}/g, "[JWT_REDACTED]");
};
const results = [], errors = [], pages = [], marker = `OpenIM386 synthetic moments ${Date.now()}`;
let createdID, browser;
const step = async (name, fn) => {
  const started = Date.now();
  try { const evidence = await fn(); results.push({ name, passed: true, elapsedMs: Date.now() - started, evidence }); console.log(name, "PASS"); }
  catch (error) { results.push({ name, passed: false, elapsedMs: Date.now() - started, error: redact(error.stack ?? error) }); throw error; }
};
const deadline = setTimeout(() => { void browser?.close(); }, 12 * 60 * 1000);
const api = (p, path, body) => p.evaluate(async ({ base, path, body }) => {
  const s = await import("/src/utils/storage.ts");
  const response = await fetch(base + "/enterprise/v1" + path, { method: "POST", headers: { "Content-Type": "application/json", token: await s.getIMToken(), chatToken: await s.getChatToken(), platformID: "5", operationID: crypto.randomUUID() }, body: JSON.stringify(body) });
  const value = await response.json();
  if (!response.ok || value.errCode !== 0) throw new Error(`Synthetic moment API failure HTTP ${response.status} code ${value.errCode}`);
  return value.data;
}, { base, path, body });
try {
  browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE });
  await step("Two synthetic users log in using the original email/password form", async () => {
    for (const a of accounts) {
      const context = await browser.newContext({ locale: "zh-CN" }), p = await context.newPage(); pages.push(p);
      p.setDefaultTimeout(12000); p.on("pageerror", (e) => errors.push(redact(e.stack ?? e.message)));
      await p.addInitScript(({ base }) => { for (const [k, v] of Object.entries({ apiUrl: base + "/api", chatUrl: base + "/chat", enterpriseUrl: base + "/enterprise/v1", wsUrl: base.replace("https:", "wss:") + "/ws" })) localStorage.setItem("upgrade38:" + k, v); }, { base });
      await p.goto("http://127.0.0.1:7777", { waitUntil: "domcontentloaded", timeout: 90000 });
      if (errors.length) throw new Error("App startup script error: " + errors.at(-1));
      await Promise.race([
        p.waitForFunction(() => typeof window.commonEventFunc === "function", undefined, { timeout: 120000 }),
        p.waitForEvent("pageerror").then((error) => { throw new Error("App startup script error: " + redact(error.stack ?? error)); }),
      ]);
      await p.getByRole("tab", { name: "邮箱", exact: true }).click();
      await p.locator("#email").fill(a.email); await p.locator("#password").fill(a.password ?? fixture.password);
      await p.getByRole("button", { name: "登录", exact: true }).click();
      await p.waitForURL("**/#/chat**", { timeout: 90000 });
      const pair = await p.evaluate(async () => { const s = await import("/src/utils/storage.ts"); return [await s.getIMToken(), await s.getChatToken()]; });
      pair.forEach(addSecret);
    }
    return { syntheticUserIDs: accounts.map((a) => a.userID), normalTLS: true, credentialInjection: false };
  });
  const [A, B] = pages;
  const dialog = A.locator(".ant-modal:visible");
  const row = () => dialog.getByText(marker, { exact: true }).locator("..").locator("..");
  const likedNames = () => row().locator('img[src*="/liked.png"]').locator("..");
  const actions = async (name) => { await row().locator('img[src*="moment_actions"]').click(); await A.locator(".ant-popover:visible").getByText(name, { exact: true }).click(); };
  const waitFor = async (condition, timeout = 8000) => { const end = Date.now() + timeout; while (Date.now() < end) { if (await condition()) return; await new Promise((r) => setTimeout(r, 100)); } throw new Error("Synthetic moments UI did not converge before its deadline"); };
  await step("Original work-circle modal shows the new post with a compact header and no captured loading cover", async () => {
    const data = await api(A, "/office/work_moment/add", { content: { type: 0, text: marker, metas: [] }, permission: 0, atUserIDs: [], permissionUserIDs: [], permissionGroupIDs: [] }); createdID = data.workMoment.workMomentID;
    await A.locator("aside").getByText("工作圈", { exact: true }).click();
    await row().waitFor({ state: "visible", timeout: 20000 });
    const geometry = await row().evaluate((el) => { const modal = el.closest(".ant-modal-content"); const r = el.getBoundingClientRect(), m = modal.getBoundingClientRect(); const fakeCover = [...modal.querySelectorAll("*")].some((x) => getComputedStyle(x).backgroundImage.includes("/moments/background.png")); return { firstPostTopFromModal: r.top - m.top, fakeCover }; });
    if (geometry.fakeCover || geometry.firstPostTopFromModal > 150) throw new Error("Work circle still reserves the captured loading-cover area");
    await dialog.screenshot({ path: resolve(out, "compact-feed.png") });
    return { workMomentID: createdID, ...geometry };
  });
  await step("Like and unlike render immediately in the original modal", async () => {
    await actions("点赞"); await waitFor(async () => await likedNames().count() === 1 && (await likedNames().innerText()).includes(accounts[0].nickname), 2500);
    await actions("点赞"); await waitFor(async () => await likedNames().count() === 0, 2500);
    return { likeVisible: true, unlikeVisible: true, manualRefresh: false };
  });
  const comment = marker + " comment";
  await step("Comment send button submits once; network failure retains the draft; retry displays the comment immediately", async () => {
    await actions("评论"); const input = row().locator("textarea"); await input.fill(comment);
    await A.route("**/office/work_moment/comment/add", (route) => route.abort("internetdisconnected"));
    await row().getByRole("button", { name: "发送", exact: true }).click();
    await A.locator(".ant-message-error").waitFor({ state: "visible", timeout: 5000 });
    await waitFor(async () => await row().getByRole("button", { name: "发送", exact: true }).evaluate((el) => !el.classList.contains("ant-btn-loading")));
    if (await input.inputValue() !== comment) throw new Error("Failed comment send discarded the draft");
    await A.unroute("**/office/work_moment/comment/add");
    await row().getByRole("button", { name: "发送", exact: true }).click();
    await row().getByText(comment, { exact: true }).waitFor({ state: "visible", timeout: 2500 });
    const actual = await api(A, "/office/work_moment/get", { workMomentID: createdID });
    const count = actual.workMoment.comments.filter((c) => c.content === comment).length;
    if (count !== 1) throw new Error("Comment send was duplicated");
    return { failedDraftRetained: true, retryVisible: true, actualCommentCount: count, manualRefresh: false };
  });
  await step("Delete comment immediately removes its original row", async () => {
    await row().getByText(comment, { exact: true }).click({ button: "right" });
    await A.locator(".ant-popover:visible").getByText("删除", { exact: true }).click();
    await waitFor(async () => await row().getByText(comment, { exact: true }).count() === 0, 2500);
    return { removed: true, manualRefresh: false };
  });
  await step("Other user's like/comment and unlike/delete automatically reach the open modal", async () => {
    const t = Date.now();
    await api(B, "/office/work_moment/like", { workMomentID: createdID, like: true });
    await waitFor(async () => await likedNames().count() === 1 && (await likedNames().innerText()).includes(accounts[1].nickname));
    const likeMs = Date.now() - t, peerComment = marker + " peer";
    const peer = await api(B, "/office/work_moment/comment/add", { workMomentID: createdID, content: peerComment, replyUserID: "" });
    await row().getByText(peerComment, { exact: true }).waitFor({ state: "visible", timeout: 8000 });
    const clearStart = Date.now();
    await api(B, "/office/work_moment/like", { workMomentID: createdID, like: false });
    await api(B, "/office/work_moment/comment/del", { workMomentID: createdID, commentID: peer.commentID });
    await waitFor(async () => await likedNames().count() === 0 && await row().getByText(peerComment, { exact: true }).count() === 0);
    return { likeMs, unlikeAndDeleteMs: Date.now() - clearStart, peerCommentVisible: true, manualRefresh: false };
  });
  await dialog.screenshot({ path: resolve(out, "final-feed.png") });
} catch (error) {
  if (!results.some((r) => !r.passed)) results.push({ name: "Synthetic UI setup", passed: false, error: redact(error.stack ?? error) });
  console.log("Moments UI stopped:", redact(error.message ?? error));
} finally {
  clearTimeout(deadline);
  if (createdID && pages[0] && !pages[0].isClosed()) {
    try { await api(pages[0], "/office/work_moment/del", { workMomentID: createdID }); results.push({ name: "Only this test's created moment is deleted", passed: true, workMomentID: createdID }); }
    catch (error) { results.push({ name: "Own synthetic moment cleanup", passed: false, error: redact(error.message ?? error), workMomentID: createdID }); }
  }
  await browser?.close();
  const report = { schema: 1, utc: new Date().toISOString(), scope: "Original work-circle UI, two synthetic users, normal TLS, no media", results, errors, loginDOMOrScreenshotSaved: false, secretsRedacted: true };
  await writeFile(resolve(out, "report.json"), redact(JSON.stringify(report, null, 2)));
}
if (results.some((r) => !r.passed) || errors.length) process.exitCode = 1;
