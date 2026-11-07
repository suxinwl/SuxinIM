// Real React/Antd CacheImage and resource grants, with only transport/storage
// boundaries replaced. No production identities or network are used.
import assert from 'node:assert/strict';
import { build } from 'esbuild';
import { chromium } from 'playwright';
import fs from 'node:fs/promises';
const mocks = {
  '@/config': `export const NETWORKS={lan:'https://192.168.10.11:9885',wan:'https://im.szputy.com:9885'};export const getServerOrigin=()=>NETWORKS.lan;`,
  '@/utils/storage': `export const getSSOSession=async()=>window.fixtureSession;`,
  './enterprise': `export const enterpriseRequest=async(path,body)=>window.fixtureAuthorize(body.url);`,
  '@/store': `export const useUserStore={getState:()=>({imageCache:window.fixtureImageCache})};`,
  '@/utils/common': `export const downloadFile=(url)=>window.fixtureDownloads.push(url);`,
};
const bundle = await build({ stdin: { contents: `import React from 'react';import{createRoot}from'react-dom/client';import{flushSync}from'react-dom';import i18n from'i18next';import CacheImage from'./src/components/CacheImage/index';import ProtectedPreviewGroup from'./src/components/ProtectedPreviewGroup/index';i18n.init({lng:'en',resources:{en:{translation:{toast:{imageLoadFailed:'Image unavailable'},retry:'Retry'}}}});const root=createRoot(document.getElementById('root'));window.renderFixture=src=>flushSync(()=>root.render(<CacheImage src={src} width={80} height={60}/>));window.renderGroupFixture=src=>flushSync(()=>root.render(<ProtectedPreviewGroup items={[src]}><CacheImage src={src} width={80} height={60}/></ProtectedPreviewGroup>));window.renderQuoteFixture=(src,full)=>flushSync(()=>root.render(<CacheImage src={src} width={80} height={60} preview={{src:full}}/>));window.flushFixture=fn=>flushSync(fn);`, resolveDir: process.cwd(), loader: 'tsx' }, bundle: true, write: false, platform: 'browser', format: 'iife', define: { 'process.env.NODE_ENV': '"production"' }, plugins: [{ name: 'image-boundaries', setup(builder) {
  builder.onResolve({ filter: /.*/ }, args => args.path in mocks ? { path: args.path, namespace: 'mock' } : undefined);
  builder.onLoad({ filter: /.*/, namespace: 'mock' }, args => ({ contents: mocks[args.path] }));
} }] });
const png = await fs.readFile('../../.cache/media-fixtures/synthetic.png');
const browser = await chromium.launch({ headless: true, channel: 'chrome' });
const checks = [];
const blockedPaths = new Set(), requestedPaths = [];
try {
  const page = await browser.newPage();
  await page.route('https://192.168.10.11:9885/**', async route => {
    const pathname = new URL(route.request().url()).pathname;
    if(pathname==='/__synthetic_image_ui__'){await route.fulfill({status:200,contentType:'text/html',body:'<!doctype html><html><body><div id="root"></div></body></html>'});return;}
    requestedPaths.push(pathname);
    const failed = pathname.includes('/deny-') || blockedPaths.has(pathname);
    await route.fulfill({ status: failed ? 403 : 200, contentType: failed ? 'text/plain' : 'image/png', body: failed ? 'synthetic denied' : png });
  });
  await page.goto('https://192.168.10.11:9885/__synthetic_image_ui__');
  await page.evaluate(() => {
    window.fixtureSession = { userID: 'viewer-a', sessionToken: 'synthetic-sid-a' };
    window.fixtureImageCache = new Proxy({}, { get: () => 'C:\\stale-shared-private-image.png' });
    window.fixtureDownloads = []; window.fixtureCalls = []; window.fixtureDenyCount = 0; window.fixtureHold = false;
    window.fixtureAuthorize = async source => {
      const owner = window.fixtureSession.userID, index = window.fixtureCalls.length + 1;
      window.fixtureCalls.push({ source, owner });
      const target = `https://192.168.10.11:9885/attachments/${window.fixtureDenyCount-- > 0 ? 'deny-' : 'allow-'}${owner}-${index}/openim38/image.png`;
      if (window.fixtureHold) await new Promise(resolve => { window.completeOldFixture = resolve; });
      return { data: { url: target, expiresAt: Date.now() / 1000 + 600,purpose:'preview',resourceProof:'a'.repeat(64),resourceProofExpiresAt:Date.now()/1000+3600 } };
    };
  });
  await page.addScriptTag({ content: bundle.outputFiles[0].text });
  const source = 'https://im.szputy.com:9885/attachments/revoked-publisher/openim38/image.png?old=1';
  await page.evaluate(src => window.renderFixture(src), source);
  await page.waitForFunction(() => document.querySelector('img')?.naturalWidth > 0);
  assert.match(await page.locator('img').first().getAttribute('src'), /allow-viewer-a/);
  assert.equal(await page.evaluate(() => window.fixtureDownloads.length), 0);
  checks.push('revoked publisher URL reauthorized for viewer; private disk-cache hit cannot bypass access');

  await page.evaluate(src => { window.fixtureHold = true; window.renderFixture(src); }, source + '&second=1');
  await page.waitForFunction(() => Boolean(window.completeOldFixture));
  const immediate = await page.evaluate(() => {
    window.fixtureHold = false;
    window.fixtureSession = { userID: 'viewer-b', sessionToken: 'synthetic-sid-b' };
    window.flushFixture(() => window.dispatchEvent(new Event('im-profile-changed')));
    return document.querySelector('img')?.getAttribute('src') || '';
  });
  assert.doesNotMatch(immediate, /viewer-a/);
  await page.waitForFunction(() => document.querySelector('img')?.src.includes('allow-viewer-b') && document.querySelector('img').naturalWidth > 0);
  await page.evaluate(() => window.completeOldFixture());
  await page.waitForTimeout(100);
  assert.match(await page.locator('img').first().getAttribute('src'), /allow-viewer-b/);
  checks.push('src/session first render and late old response cannot reveal previous viewer image');

  let before = await page.evaluate(() => window.fixtureCalls.length);
  await page.evaluate(src => { window.fixtureDenyCount = 1; window.renderFixture(src); }, source + '&retry=one');
  await page.waitForFunction(() => document.querySelector('img')?.src.includes('/allow-') && document.querySelector('img').naturalWidth > 0);
  assert.equal(await page.evaluate(() => window.fixtureCalls.length) - before, 2);
  checks.push('one failed image load performs exactly one forced grant refresh');

  before = await page.evaluate(() => window.fixtureCalls.length);
  await page.evaluate(src => { window.fixtureDenyCount = 2; window.renderFixture(src); }, source + '&retry=bounded');
  await page.getByRole('button', { name: 'Retry', exact: true }).waitFor();
  await page.waitForTimeout(250);
  assert.equal(await page.evaluate(() => window.fixtureCalls.length) - before, 2);
  await page.getByRole('button', { name: 'Retry', exact: true }).click();
  await page.waitForFunction(() => document.querySelector('img')?.naturalWidth > 0);
  assert.equal(await page.evaluate(() => window.fixtureCalls.length) - before, 3);
  checks.push('second failure stops automatic retries and offers a bounded explicit retry');

  const stalePreview = await page.locator('img').first().getAttribute('src');
  await page.evaluate(src => window.renderGroupFixture(src), stalePreview);
  await page.waitForFunction(() => document.querySelector('.ant-image-img')?.naturalWidth > 0);
  blockedPaths.add(new URL(stalePreview).pathname);
  await page.locator('.ant-image-img').first().dispatchEvent('error');
  await page.waitForFunction(old => document.querySelector('.ant-image-img')?.src !== old && document.querySelector('.ant-image-img')?.naturalWidth > 0, stalePreview);
  const requestCount = requestedPaths.length;
  await page.locator('.ant-image-mask').first().click();
  await page.waitForFunction(() => document.querySelector('.ant-image-preview-img')?.naturalWidth > 0);
  const previewSrc = await page.locator('.ant-image-preview-img').getAttribute('src');
  assert.notEqual(previewSrc, stalePreview);
  assert.ok(!requestedPaths.slice(requestCount).includes(new URL(stalePreview).pathname));
  await page.locator('.ant-image-preview-close').click();
  checks.push('independent group items cannot keep an expired preview grant after thumbnail refresh');

  await page.evaluate(({ source, full }) => window.renderQuoteFixture(source, full), { source: source + '&quote-thumb=1', full: stalePreview });
  await page.waitForFunction(() => document.querySelector('.ant-image-img')?.naturalWidth > 0);
  await page.locator('.ant-image-mask').first().click();
  await page.waitForFunction(() => document.querySelector('.ant-image-preview-img')?.naturalWidth > 0);
  assert.notEqual(await page.locator('.ant-image-preview-img').getAttribute('src'), stalePreview);
  await page.locator('.ant-image-preview-close').click();
  checks.push('standalone quote preview.src is independently bound to the current viewer');

  console.log(JSON.stringify({ passed: true, checks, productionRequests: false }));
} finally { await browser.close(); }
