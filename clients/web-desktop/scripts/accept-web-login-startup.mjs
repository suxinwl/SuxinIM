import fs from 'node:fs/promises';
import path from 'node:path';
import crypto from 'node:crypto';
import assert from 'node:assert/strict';
import { chromium } from '@playwright/test';

// Run complete built Web renderers in an isolated synthetic HTTPS origin. Every
// request is fulfilled locally or blocked; there are no real tickets or logins.
const args = process.argv.slice(2);
const option = name => args[args.indexOf(name) + 1];
assert(args.includes('--dist') && args.includes('--out'), '--dist and --out are required');
const client = path.resolve(import.meta.dirname, '..');
const root = path.resolve(client, '../..');
const dist = path.resolve(option('--dist'));
const output = path.resolve(option('--out'));
assert(output.startsWith(path.join(root, '.private') + path.sep));
await fs.mkdir(output, { recursive: true });
const digest = data => crypto.createHash('sha256').update(data).digest('hex');
const origin = 'https://puty-web-startup.invalid';
const syntheticTicket = 'synthetic-invalid-ticket-not-a-real-sso-ticket';
const mime = { '.html': 'text/html', '.js': 'text/javascript', '.css': 'text/css', '.json': 'application/json', '.wasm': 'application/wasm', '.png': 'image/png', '.svg': 'image/svg+xml', '.ico': 'image/x-icon' };
const headers = { 'Cross-Origin-Opener-Policy': 'same-origin', 'Cross-Origin-Embedder-Policy': 'require-corp', 'Cross-Origin-Resource-Policy': 'same-origin' };
const browser = await chromium.launch({ executablePath: process.env.CHROME_PATH || 'C:/Program Files/Google/Chrome/Application/chrome.exe', headless: true });
const results = [];

async function scenario(directory, name, ticket = false, expectedBlank = false) {
  const context = await browser.newContext({ viewport: { width: 1200, height: 800 }, serviceWorkers: 'block' });
  const state = { name, pageerrors: [], blockedRequests: [], fixtureExchanges: 0, fixtureCaptchas: 0, realNetworkRequests: 0 };
  const page = await context.newPage();
  page.on('pageerror', error => state.pageerrors.push({ name: error.name, message: error.message }));
  await context.route('**/*', async route => {
    const request = route.request(), url = new URL(request.url());
    if (url.origin !== origin) { state.blockedRequests.push(url.origin + url.pathname); return route.abort(); }
    const json = body => route.fulfill({ status: 200, contentType: 'application/json', headers, body: JSON.stringify(body) });
    if (url.pathname === '/sso/exchange') {
      assert(ticket, 'Exchange without a synthetic ticket');
      const body = request.postDataJSON();
      assert.deepEqual(body, { ticket: syntheticTicket, platform: 'web' });
      state.fixtureExchanges++;
      return route.fulfill({ status: 401, contentType: 'application/json', headers, body: JSON.stringify({ code: 401, message: '合成票据已失效', data: null }) });
    }
    if (url.pathname === '/sso/captcha') { state.fixtureCaptchas++; return json({ code: 0, data: { enabled: false } }); }
    if (url.pathname === '/chat/client_config/get') return json({ errCode: 0, data: { config: { appName: 'PUTY内部通讯', webTitle: 'PUTY内部通讯' } } });
    const filename = path.resolve(directory, '.' + decodeURIComponent(url.pathname === '/' ? '/index.html' : url.pathname));
    if (!filename.startsWith(directory + path.sep)) return route.abort();
    try {
      const body = await fs.readFile(filename);
      return route.fulfill({ status: 200, headers, contentType: mime[path.extname(filename)] || 'application/octet-stream', body });
    } catch {
      state.blockedRequests.push(url.pathname);
      return route.fulfill({ status: 404, headers, body: 'Not present in isolated renderer' });
    }
  });
  try {
    await page.goto(origin + (ticket ? '/?ticket=' + syntheticTicket : '/') + '#/login', { waitUntil: 'networkidle' });
    if (expectedBlank) {
      assert(state.pageerrors.some(error => /config is not a function/.test(error.message)));
      assert.equal(await page.locator('#root').evaluate(element => element.childNodes.length), 0);
      state.expectedStartupFailureReproduced = true;
    } else {
      await page.getByRole('button', { name: '登录', exact: true }).waitFor({ state: 'visible' });
      assert(await page.getByLabel('统一账号 / 工号').isVisible());
      assert(await page.getByLabel('密码', { exact: true }).isVisible());
      assert.equal(await page.getByRole('radio').count(), 0);
      assert.equal(await page.getByRole('button', { name: '修复存储', exact: true }).count(), 0);
      assert.equal(await page.evaluate(() => 'electronAPI' in window), false);
      state.realLoginFormVisible = true;
      state.webHasNoNativeControls = true;
      state.localForageDatabaseConfigured = await page.evaluate(async () => (await indexedDB.databases()).some(db => db.name === 'OpenIM38-Config'));
      assert(state.localForageDatabaseConfigured);
      if (ticket) {
        await page.getByText('登录链接已失效，请重新从人事系统打开或使用统一账号登录', { exact: true }).waitFor({ state: 'visible' });
        assert.equal(new URL(page.url()).searchParams.has('ticket'), false);
        assert.equal(state.fixtureExchanges, 1);
        assert(state.fixtureCaptchas >= 1);
        state.invalidSyntheticTicketFallsBackToLogin = true;
        await page.reload({ waitUntil: 'networkidle' });
        await page.getByRole('button', { name: '登录', exact: true }).waitFor({ state: 'visible' });
        assert.equal(state.fixtureExchanges, 1, 'The removed ticket must never be replayed on reload');
        state.reloadDoesNotReplayTicket = true;
      } else assert.equal(state.fixtureExchanges, 0);
      assert.deepEqual(state.pageerrors, []);
    }
    assert.deepEqual(state.blockedRequests, []);
    state.screenshot = name + '.png';
    const screenshot = await page.screenshot({ path: path.join(output, state.screenshot) });
    state.screenshotSHA256 = digest(screenshot);
    state.passed = true;
    results.push(state);
  } finally { await context.close(); }
}

try {
  if (args.includes('--baseline')) await scenario(path.resolve(option('--baseline')), 'old-bundle-anonymous-blank', false, true);
  await scenario(dist, 'fixed-anonymous-login');
  await scenario(dist, 'fixed-synthetic-expired-ticket', true);
  const report = { format: 'puty-isolated-web-login-startup-v1', passed: true, productionAuthentication: false, realEmployeeDataRead: false, realNetworkRequests: 0, syntheticInvalidTicketOnly: true, realSDKLoginPerformed: false, rendererIndexSHA256: digest(await fs.readFile(path.join(dist, 'index.html'))), storageSourceSHA256: digest(await fs.readFile(path.join(client, 'src/utils/storage.ts'))), cases: results };
  const file = path.join(output, 'report.private.json');
  await fs.writeFile(file, JSON.stringify(report, null, 2) + '\n', { flag: 'wx', mode: 0o600 });
  console.log(JSON.stringify({ passed: true, report: file, reportSHA256: digest(await fs.readFile(file)), cases: results.length }));
} finally { await browser.close(); }
