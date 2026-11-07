// Anonymous native entry check. Does not read or use any employee fixture.
import { _electron } from 'playwright';
import { readFile, mkdir, writeFile } from 'node:fs/promises';
import path from 'node:path';
import os from 'node:os';
const client = path.resolve(import.meta.dirname, '..');
const brand = JSON.parse(await readFile(path.join(client, '../brand.json'), 'utf8'));
const version = JSON.parse(await readFile(path.join(client, 'build-version.json'), 'utf8'));
const namespace = 'organization-20261103-prelogin-' + Date.now();
const evidence = path.join(client, 'build/acceptance', namespace);
// Chromium CacheStorage uses additional hashed paths. Keep isolated QA data
// outside the long source checkout, matching normal AppData path lengths.
const userData = path.join(os.tmpdir(), 'puty-im-qa', 'pre-' + Date.now());
await mkdir(evidence, { recursive: true });
const report = { scope: 'anonymous normal native LAN entry; no employee credentials', expectedVersion: version.version, expectedDisplayName: brand.displayName, responses: [], pageErrors: [] };
let app, page, stage = 'launch';
try {
  app = await _electron.launch({ executablePath: process.env.PUTY_ORGANIZATION_EXE || path.join(client, 'release/win-unpacked', brand.technicalName + '.exe'), env: { ...process.env, OPENIM38_TEST_DATA_DIR: userData }, timeout: 60000 });
  for (let i = 0; i < 160; i++) { page = app.windows().find(p => p.url().startsWith('openim38://app/')); if (page) break; await new Promise(r => setTimeout(r, 250)); }
  if (!page) throw new Error('native_window_unavailable');
  page.on('pageerror', () => report.pageErrors.push('renderer_exception'));
  page.on('response', response => { const url = new URL(response.url()); if (['/sso/captcha', '/chat/client_config/get'].includes(url.pathname)) report.responses.push({ path: url.pathname, status: response.status() }); });
  stage = 'heading';
  await page.getByRole('heading', { name: brand.displayName, exact: true }).waitFor({ timeout: 60000 });
  stage = 'wasm';
  await page.waitForFunction(() => typeof window.login === 'function', undefined, { timeout: 60000 });
  stage = 'captcha-image';
  await page.getByAltText('点击更换验证码').waitFor({ timeout: 45000 });
  report.runtime = await app.evaluate(({ app }) => ({ name: app.getName(), version: app.getVersion(), isolatedTestData: app.getPath('userData') }));
  if (report.runtime.name !== brand.displayName || report.runtime.version !== version.version || path.resolve(report.runtime.isolatedTestData) !== userData) throw new Error('native_brand_or_data_namespace_mismatch');
  report.runtime.isolatedTestData = true;
  report.documentTitle = await page.title();
  report.legacyDisplayNamePresent = (await page.locator('body').innerText()).includes('速信通讯');
  if (report.legacyDisplayNamePresent || report.pageErrors.length) throw new Error('native_entry_rejected');
  // Re-run only the existing anonymous display endpoint to prove its current
  // response independently of an early event listener attachment.
  report.publicBrand = await page.evaluate(async () => {
    const response = await fetch('https://192.168.10.11:9885/chat/client_config/get', { method: 'POST', headers: { 'Content-Type': 'application/json', operationID: crypto.randomUUID() }, body: '{}', credentials: 'omit' });
    const body = await response.json();
    return { status: response.status, errCode: body.errCode, appName: body.data?.config?.appName, webTitle: body.data?.config?.webTitle };
  });
  if (report.publicBrand.status !== 200 || report.publicBrand.errCode !== 0 || report.publicBrand.appName !== brand.displayName || report.documentTitle !== report.publicBrand.webTitle) throw new Error('public_brand_mismatch');
  await page.screenshot({ path: path.join(evidence, 'native-prelogin-final.png') });
  report.passed = true;
} catch (error) {
  report.passed = false; report.failureStage = stage; report.failureType = error.name; report.failureCategory = /^[a-z_]+$/.test(error.message) ? error.message : 'native_prelogin_check_failed'; process.exitCode = 1;
  if (page) { report.entryState = await page.evaluate(() => ({ title: document.title, wasmLogin: typeof window.login, goRuntime: typeof window.Go, sharedArrayBuffer: typeof SharedArrayBuffer, crossOriginIsolated, assets: performance.getEntriesByType('resource').filter(e => /wasm|worker/.test(e.name)).map(e => ({ path: new URL(e.name).pathname, bytes: e.decodedBodySize, duration: Math.round(e.duration) })), headings: [...document.querySelectorAll('h1,h2,h3')].map(e => e.textContent) })).catch(() => null); await page.screenshot({ path: path.join(evidence, 'prelogin-failure.png') }).catch(() => {}); }
} finally {
  if (app) { await Promise.race([app.evaluate(({ app }) => app.exit(0)).catch(() => {}), new Promise(r => setTimeout(r, 3000))]); await Promise.race([app.close().catch(() => {}), new Promise(r => setTimeout(r, 3000))]); }
  await writeFile(path.join(evidence, 'anonymous-entry.json'), JSON.stringify(report, null, 2));
  console.log(JSON.stringify({ passed: report.passed, evidence, failureCategory: report.failureCategory, employeeCredentialsRead: 0 }));
}
