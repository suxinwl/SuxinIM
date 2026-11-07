// Restricted synthetic-account acceptance through the real chat file inputs.
// Credentials remain in memory; no HAR/trace/raw HTTP headers are written.
import { chromium, _electron } from 'playwright';
import fs from 'node:fs/promises';
import path from 'node:path';
import os from 'node:os';
import crypto from 'node:crypto';

const client = path.resolve(import.meta.dirname, '..'), suite = path.resolve(client, '../..');
const platform = process.argv.includes('--windows') ? 'windows' : 'web';
const candidateWeb = platform === 'web' && process.argv.includes('--candidate-web');
const invalidPreflight = process.argv.includes('--invalid-preflight');
const acceptance = path.join(suite, '.private/production/acceptance');
const fixturePath = path.resolve(process.env.PUTY_ATTACHMENT_FIXTURE || path.join(acceptance, 'attachment-20261103/test-accounts.json'));
if (!fixturePath.startsWith(acceptance + path.sep)) throw new Error('FIXTURE_PATH_REJECTED');
const fixture = JSON.parse((await fs.readFile(fixturePath, 'utf8')).replace(/^\uFEFF/, ''));
if (fixture.namespace !== 'attachment-20261103' || fixture.accounts?.length !== 2) throw new Error('FIXTURE_NAMESPACE_REJECTED');
const [actor, colleague] = fixture.accounts;
if (fixture.departmentID !== 53 || actor.username !== 'qa_261103_e' || colleague.username !== 'qa_261103_f') throw new Error('FIXTURE_IDENTITY_REJECTED');
for (const account of fixture.accounts) {
  if (!Number.isSafeInteger(account.uid) || account.uid <= 0 || typeof account.employeeID !== 'string' || !account.employeeID || typeof account.password !== 'string' || !account.password) throw new Error('FIXTURE_FIELDS_REJECTED');
  if (fixture.excludedPreviousUIDs?.includes(account.uid) || fixture.excludedPreviousEmployeeIDs?.includes(account.employeeID)) throw new Error('PREVIOUS_FIXTURE_REJECTED');
}
if (actor.uid === colleague.uid || actor.employeeID === colleague.employeeID || fixture.excludedPreviousUIDs?.length !== 4 || fixture.excludedPreviousEmployeeIDs?.length !== 4) throw new Error('FIXTURE_SEPARATION_REJECTED');
const imID = account => 'emp_' + crypto.createHash('sha256').update(account.employeeID, 'utf8').digest('hex').slice(0, 32);
const actorID = imID(actor), peerID = imID(colleague);
const origin = 'https://192.168.10.11:9885';
if (fixture.lanOrigin !== origin) throw new Error('ENTRY_REJECTED');
const fixturesDir = path.join(acceptance, 'attachment-20261103/fixtures');
const manifest = JSON.parse(await fs.readFile(path.join(fixturesDir, 'manifest.json'), 'utf8'));
if (!manifest.syntheticOnly || !Array.isArray(manifest.files)) throw new Error('INPUTS_REJECTED');
const allowedNames = new Set(['test-image.png', 'test-text.txt', 'test-document.pdf', 'test-archive.zip', 'test-video.mp4', 'unknown-mime.putytest', 'boundary-below.txt', 'boundary-above.txt', 'large-archive.zip', 'large-image.png', 'large-video.mp4']);
let inputs = [];
for (const entry of manifest.files) {
  if (entry.mustNotUpload) continue;
  if (!allowedNames.has(entry.name)) throw new Error('INPUT_NAME_REJECTED');
  const file = path.join(fixturesDir, entry.name), data = await fs.readFile(file);
  const size = entry.bytes ?? entry.size;
  if (data.length !== size || crypto.createHash('sha256').update(data).digest('hex') !== entry.sha256) throw new Error('INPUT_HASH_REJECTED');
  inputs.push({ ...entry, size, file });
}
if (inputs.length !== allowedNames.size || new Set(inputs.map(input => input.name)).size !== allowedNames.size || !inputs.some(input => input.name.endsWith('.mp4') && input.size > manifest.multipartBoundaryBytes)) throw new Error('INPUT_COVERAGE_REJECTED');
const baseline = process.argv.includes('--baseline');
const order = ['test-image.png', 'test-text.txt', 'test-document.pdf', 'test-archive.zip', 'unknown-mime.putytest', 'test-video.mp4', 'boundary-below.txt', 'boundary-above.txt', 'large-archive.zip', 'large-image.png', 'large-video.mp4'];
inputs.sort((a, b) => order.indexOf(a.name) - order.indexOf(b.name));
if (baseline) inputs = inputs.filter(input => ['test-text.txt', 'boundary-above.txt', 'large-video.mp4'].includes(input.name));
const stamp = Date.now(), evidence = path.join(acceptance, 'attachment-20261103', platform + '-ui-' + stamp);
await fs.mkdir(evidence, { recursive: true });
const report = { namespace: fixture.namespace, platform, baseline, candidateStaticContent: candidateWeb, strictTLS: true, origin, domainSNIMappedToLAN: false, internetNATAccepted: false, realFileInput: true, cases: [], directoryRequests: [], organizationVisits: [], errors: [], logout: false };
const privateCapture = { namespace: fixture.namespace, platform, uploads: [], sdkSends: [] };
let browser, app, page, session, stage = 'launch', activeCase;
const wait = ms => new Promise(resolve => setTimeout(resolve, ms));
function headerProof(headers) {
  const entries = Array.isArray(headers) ? headers.map(item => [item.key, item.values?.[0] || '']) : Object.entries(headers || {});
  const lower = Object.fromEntries(entries.map(([key, value]) => [key.toLowerCase(), String(value)]));
  const auth = lower.authorization || '';
  return {
    names: Object.keys(lower).sort(), contentType: lower['content-type'] || null,
    authorizationScheme: !auth ? null : auth.startsWith('AWS4-HMAC-SHA256 ') ? 'AWS4-HMAC-SHA256' : auth.startsWith('AWS ') ? 'AWS' : 'other',
    authorizationSignedHeaders: (auth.match(/SignedHeaders=([^,\s]+)/)?.[1]?.split(';') || []).filter(value => /^[a-z0-9-]{1,64}$/.test(value)),
    hasAmzDate: Boolean(lower['x-amz-date']), hasAmzContentSHA256: Boolean(lower['x-amz-content-sha256']),
  };
}
const shot = name => page.screenshot({ path: path.join(evidence, name + '.png'), mask: [page.locator('input[type=password]')] });
try {
  if (platform === 'windows') {
    app = await _electron.launch({ executablePath: path.join(client, 'release/win-unpacked/PUTY_IM.exe'), env: { ...process.env, OPENIM38_TEST_DATA_DIR: path.join(os.tmpdir(), 'puty-im-qa', 'attachment-' + stamp) }, timeout: 60000 });
    for (let i = 0; i < 160; i++) { page = app.windows().find(item => item.url().startsWith('openim38://app/')); if (page) break; await wait(250); }
  } else {
    browser = await chromium.launch({ headless: true, channel: 'chrome' });
    const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
    page = await context.newPage();
  }
  if (!page) throw new Error('PAGE_UNAVAILABLE');
  if (app) report.version = await app.evaluate(({ app }) => app.getVersion());
  if (candidateWeb) {
    // Override only frozen candidate app assets. API/WSS/attachments retain the
    // real HTTPS origin and server validation; this is not a server deployment.
    const staticRoot = path.join(client, 'dist-web');
    await page.route(origin + '/**', async route => {
      const request = route.request(), pathname = new URL(request.url()).pathname;
      if (request.method() !== 'GET' || !(pathname === '/' || pathname === '/index.html' || /^\/assets\/[A-Za-z0-9_.-]+$/.test(pathname) || /^\/(?:openIM\.wasm|sql-wasm\.wasm|wasm_exec\.js|favicon-32x32\.png)$/.test(pathname))) return route.continue();
      const file = path.join(staticRoot, pathname === '/' ? 'index.html' : pathname.slice(1));
      try { await route.fulfill({ path: file }); } catch { await route.abort('failed'); }
    });
    report.version = JSON.parse(await fs.readFile(path.join(client, 'package.json'), 'utf8')).version;
  }
  await page.route('**/api/object/initiate_multipart_upload', route => {
    if (activeCase?.name === 'invalid-video.mp4') { activeCase.blockedUpload = true; return route.abort('blockedbyclient'); }
    return route.continue();
  });
  page.setDefaultTimeout(45000);
  const counters = { directory: 0, authorize: 0, images: 0 };
  const requestCases = new WeakMap(), directoryTiming = new WeakMap();
  page.on('pageerror', () => report.errors.push({ type: 'renderer_exception', input: activeCase?.name }));
  page.on('request', request => {
    requestCases.set(request, activeCase);
    const pathname = new URL(request.url()).pathname;
    if (pathname === '/sso/directory') counters.directory++;
    if (pathname === '/enterprise/v1/attachment/authorize') counters.authorize++;
    if (request.resourceType() === 'image') counters.images++;
    if (new URL(request.url()).pathname === '/sso/directory') {
      let body = {}; try { body = request.postDataJSON(); } catch {}
      directoryTiming.set(request, { at: performance.now(), hasKeyword: Boolean(body.keyword), hasDepartment: Boolean(body.departmentID), page: body.page, stage });
    }
  });
  page.on('response', async response => {
    const request = response.request(), url = new URL(response.url()), current = requestCases.get(request);
    const timing = directoryTiming.get(request);
    if (timing) report.directoryRequests.push({ status: response.status(), durationMs: Math.round(performance.now() - timing.at), hasKeyword: timing.hasKeyword, hasDepartment: timing.hasDepartment, page: timing.page, stage: timing.stage });
    if (!current) return;
    if (request.method() === 'OPTIONS' && url.pathname.startsWith('/attachments/')) {
      current.preflights ??= [];
      current.preflights.push({ status: response.status(), allowHeaders: (response.headers()['access-control-allow-headers'] || '').toLowerCase().split(',').map(value => value.trim()).filter(value => /^[a-z0-9-]{1,64}$/.test(value)) });
    }
    if (request.method() === 'PUT' && url.pathname.startsWith('/attachments/')) {
      const proof = { status: response.status(), ...headerProof(await request.allHeaders()), querySignedHeaders: (url.searchParams.get('X-Amz-SignedHeaders') || '').split(';').filter(Boolean) };
      if (!response.ok()) {
        const body = await response.text().catch(() => '');
        const xmlCode = body.match(/<Code>([A-Za-z][A-Za-z0-9]{0,79})<\/Code>/)?.[1];
        if (xmlCode) proof.xmlCode = xmlCode;
        try { const data = JSON.parse(body); proof.errCode = data.errCode; } catch {}
      }
      current.puts.push(proof);
    }
    if (!['/api/object/initiate_multipart_upload', '/api/object/complete_multipart_upload', '/api/object/auth_sign'].includes(url.pathname)) return;
    try {
      const body = await response.json(), input = request.postDataJSON();
      current.http.push({ path: url.pathname, status: response.status(), errCode: body.errCode });
      const fields = Object.fromEntries(Object.entries(input || {}).filter(([key]) => ['name', 'size', 'hash', 'contentType', 'uploadID', 'group', 'key'].includes(key)));
      privateCapture.uploads.push({ input: current.name, path: url.pathname, fields });
      const sign = body.data?.upload?.sign || body.data;
      if (Array.isArray(sign?.header)) current.signs.push({ ...headerProof(sign.header), parts: (sign.parts || []).map(part => ({ ...headerProof(part.header), partNumber: part.partNumber })) });
    } catch {}
  });
  if (platform === 'web') await page.goto(origin, { waitUntil: 'domcontentloaded', timeout: 90000 });
  stage = 'login';
  await page.getByLabel('统一账号 / 工号').waitFor();
  await page.getByLabel('统一账号 / 工号').fill(actor.username);
  await page.getByLabel('密码', { exact: true }).fill(actor.password);
  const captchaInput = page.getByLabel('验证码', { exact: true });
  await page.getByAltText('点击更换验证码').waitFor();
  if (await captchaInput.isVisible()) {
    const challenge = path.join(evidence, 'captcha');
    await page.getByAltText('点击更换验证码').screenshot({ path: challenge + '.png' });
    console.log(JSON.stringify({ stage: 'captcha', image: challenge + '.png', answerFile: challenge + '.txt' }));
    for (let i = 0; i < 600; i++) { try { const answer = (await fs.readFile(challenge + '.txt', 'utf8')).trim(); if (answer) { await captchaInput.fill(answer); break; } } catch {} await wait(250); }
    if (!await captchaInput.inputValue()) throw new Error('CAPTCHA_ANSWER_UNAVAILABLE');
  }
  const loginReply = page.waitForResponse(response => new URL(response.url()).pathname === '/sso/login', { timeout: 60000 });
  await page.getByRole('button', { name: '登录', exact: true }).click();
  const loginResponse = await loginReply, loginBody = await loginResponse.json();
  if (loginResponse.status() !== 200 || loginBody.code !== 0) throw new Error('SSO_REJECTED');
  session = loginBody.data;
  if (session.userID !== actorID) throw new Error('LOGIN_IDENTITY_REJECTED');
  await page.getByText('组织架构', { exact: true }).first().waitFor({ timeout: 90000 });
  stage = 'peer';
  await page.getByText('组织架构', { exact: true }).first().click();
  const search = page.getByLabel('按手机号、姓名或工号查找同事');
  await search.fill(String(colleague.username));
  const directoryReply = page.waitForResponse(response => new URL(response.url()).pathname === '/sso/directory');
  await search.press('Enter');
  const directory = await (await directoryReply).json();
  const person = directory.data?.items?.find(item => item.employeeNo === colleague.username);
  if (!person || !person.canDirectChat || person.userID !== peerID || String(person.departmentID) !== '53') throw new Error('PEER_REJECTED');
  await page.getByRole('button', { name: person.nickname, exact: true }).click();
  await page.getByRole('button', { name: '发送消息', exact: true }).click();
  await page.locator('#editable-div').waitFor();
  // Observe the real Go export invoked by the SDK; never inject/create/send a message here.
  await page.evaluate(({ peerID }) => {
    window.__putyAttachmentUI = { active: null, results: [] };
    const send = window.sendMessage;
    window.sendMessage = (...args) => {
      const state = window.__putyAttachmentUI, input = state.active;
      if (!input) return send(...args);
      if (args[2] !== peerID || args[3] !== '') throw new Error('FIXTURE_RECIPIENT_REJECTED');
      const message = JSON.parse(args[1]);
      const result = { input, contentType: message.contentType, clientMsgID: message.clientMsgID, complete: false };
      state.results.push(result);
      return Promise.resolve(send(...args)).then(value => {
        result.complete = true;
        let body = value; try { if (typeof body === 'string') body = JSON.parse(body); } catch {}
        let data = body?.data ?? body; try { if (typeof data === 'string') data = JSON.parse(data); } catch {}
        result.success = !body?.errCode && data?.status === 2;
        result.errCode = body?.errCode || 0;
        const pick = (value, fields) => value && Object.fromEntries(fields.filter(field => field in value).map(field => [field, value[field]]));
        result.message = { clientMsgID: data?.clientMsgID, status: data?.status, contentType: data?.contentType,
          fileElem: pick(data?.fileElem, ['uuid', 'fileName', 'fileSize', 'fileType']),
          videoElem: pick(data?.videoElem, ['videoUUID', 'videoSize', 'videoType', 'duration', 'snapshotUUID', 'snapshotSize', 'snapshotWidth', 'snapshotHeight']),
          pictureElem: data?.pictureElem ? { sourcePicture: pick(data.pictureElem.sourcePicture, ['uuid', 'size', 'type', 'width', 'height']) } : undefined };
        return value;
      }, error => { result.complete = true; result.success = false; result.errCode = error?.errCode; result.errorType = error?.name || typeof error; throw error; });
    };
  }, { peerID });
  await shot('peer-conversation');
  for (const input of inputs) {
    stage = 'input-' + input.name;
    const current = activeCase = { name: input.name, size: input.size, sha256: input.sha256, puts: [], signs: [], http: [], inputEvents: 0 };
    report.cases.push(current);
    await page.evaluate(name => { window.__putyAttachmentUI.active = name; }, input.name);
    const accept = input.name.endsWith('.png') ? 'image/*' : input.name.endsWith('.mp4') ? '.mp4' : '*';
    const fileInput = page.locator(`footer input[type=file][accept="${accept}"]`);
    await fileInput.setInputFiles(input.file); current.inputEvents++;
    try { await page.waitForFunction(name => window.__putyAttachmentUI.results.some(result => result.input === name && result.complete), input.name, { timeout: 65000 }); } catch { current.timeout = true; }
    await wait(500);
    const results = await page.evaluate(name => window.__putyAttachmentUI.results.filter(result => result.input === name), input.name);
    privateCapture.sdkSends.push(...results);
    current.sdk = results.map(({ message, clientMsgID, ...result }) => result);
    current.passed = !current.timeout && results.length === 1 && results[0].success === true;
    await shot(input.name.replace(/[^a-z0-9-]/g, '-'));
    console.log(JSON.stringify({ stage: 'case', platform, input: current.name, passed: current.passed, putStatus: current.puts.map(put => put.status), sdkErrCode: current.sdk[0]?.errCode, timeout: Boolean(current.timeout) }));
    await fileInput.setInputFiles([]);
    if (current.timeout) break;
  }
  if (invalidPreflight) {
    stage = 'invalid-video-preflight';
    const invalid = manifest.files.find(input => input.name === 'invalid-video.mp4' && input.mustNotUpload);
    if (!invalid) throw new Error('NEGATIVE_FIXTURE_REJECTED');
    const file = path.join(fixturesDir, invalid.name), bytes = await fs.readFile(file);
    if (crypto.createHash('sha256').update(bytes).digest('hex') !== invalid.sha256) throw new Error('NEGATIVE_FIXTURE_HASH_REJECTED');
    activeCase = { name: invalid.name };
    await page.evaluate(() => { window.__putyAttachmentUI.active = 'invalid-video.mp4'; });
    await page.locator('footer input[type=file][accept=".mp4"]').setInputFiles(file);
    await page.getByText('无法读取图片或视频，请选择完整且受支持的文件。', { exact: true }).waitFor({ timeout: 20000 });
    const sdkReached = await page.evaluate(() => window.__putyAttachmentUI.results.some(item => item.input === 'invalid-video.mp4'));
    report.invalidVideo = { visibleError: true, sdkReached, blockedUpload: Boolean(activeCase.blockedUpload), passed: !sdkReached && !activeCase.blockedUpload };
    await shot('invalid-video-visible-error');
  }
  activeCase = undefined;
  if (!baseline) for (let visit = 0; visit < 3; visit++) {
    stage = 'organization-visit-' + visit;
    const before = { ...counters }, started = performance.now();
    const reply = page.waitForResponse(response => new URL(response.url()).pathname === '/sso/directory');
    await page.getByText('组织架构', { exact: true }).first().click();
    await reply;
    await page.getByRole('columnheader', { name: '工号', exact: true }).waitFor();
    await page.waitForFunction(() => !document.querySelector('section .ant-spin-spinning'));
    const spinnerMs = Math.round(performance.now() - started);
    await wait(300);
    report.organizationVisits.push({ visit, spinnerMs, directoryRequests: counters.directory - before.directory, authorizeRequests: counters.authorize - before.authorize, imageRequests: counters.images - before.images, globalSpinnerCount: await page.locator('.ant-spin-spinning').count() });
    await page.getByText('消息', { exact: true }).first().click();
  }
  report.passed = report.cases.length === inputs.length && report.cases.every(item => item.passed) && !report.errors.length;
  if (invalidPreflight && !report.invalidVideo?.passed) report.passed = false;
} catch (error) {
  report.passed = false; report.failureStage = stage; report.failureType = ['Error', 'TimeoutError', 'TypeError', 'SyntaxError'].includes(error.name) ? error.name : 'other';
  report.failureCategory = /^[A-Z_]{1,80}$/.test(error.message) ? error.message : 'UI_ACCEPTANCE_FAILED';
  if (page) await shot('failure').catch(() => {});
} finally {
  if (session && page) try { report.logout = await page.evaluate(async ({ origin, sessionToken }) => { const response = await fetch(origin + '/sso/logout', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ sessionToken }) }); return response.status === 200 && (await response.json()).code === 0; }, { origin, sessionToken: session.sessionToken }); } catch {}
  if (!report.logout) report.passed = false;
  if (app) { await app.evaluate(({ app }) => app.exit(0)).catch(() => {}); await Promise.race([app.close().catch(() => {}), wait(3000)]); }
  if (browser) await browser.close();
  await fs.writeFile(path.join(evidence, 'capture.private.json'), JSON.stringify(privateCapture, null, 2));
  await fs.writeFile(path.join(evidence, 'report.json'), JSON.stringify(report, null, 2));
  console.log(JSON.stringify({ platform, passed: report.passed, logout: report.logout, evidence, failureStage: report.failureStage }));
}
if (!report.passed) process.exitCode = 1;
