import { chromium } from 'playwright';
import { mkdir, writeFile } from 'node:fs/promises';
import { resolve } from 'node:path';
import { captureVideoState } from './video-frame-state.mjs';

const temp = resolve('../../.cache/temp-web');
await mkdir(temp, { recursive: true });
process.env.TEMP = temp;
process.env.TMP = temp;
const browser = await chromium.launch({
  headless: true,
  executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE,
});
try {
  const page = await browser.newPage();
  await page.setContent('<canvas width="160" height="120"></canvas><video autoplay muted playsinline></video>');
  await page.evaluate(async () => {
    const canvas = document.querySelector('canvas');
    const video = document.querySelector('video');
    const context = canvas.getContext('2d');
    let frame = 0;
    window.syntheticFrameTimer = setInterval(() => {
      context.fillStyle = `hsl(${++frame * 13 % 360} 80% 50%)`;
      context.fillRect(0, 0, canvas.width, canvas.height);
    }, 50);
    video.srcObject = canvas.captureStream(20);
    await video.play();
  });
  await page.waitForFunction(() => document.querySelector('video').videoWidth > 0);
  const before = (await page.evaluate(captureVideoState))[0];
  await page.waitForTimeout(1500);
  const after = (await page.evaluate(captureVideoState))[0];
  const passed = after.videoID === before.videoID && after.currentTime > before.currentTime
    && after.totalVideoFrames > before.totalVideoFrames
    && after.frameCallbackCount > before.frameCallbackCount;
  const report = {
    version: 1,
    endpoint: 'GET http://127.0.0.1:17787/state',
    test: 'Synthetic canvas MediaStream renders new frames in a real Chromium video element',
    sampledAt: new Date().toISOString(),
    before,
    after,
    passed,
    boundary: 'Tests the helper counters; does not claim Android or remote WebRTC media acceptance.',
  };
  await mkdir('../../tests/web-desktop', { recursive: true });
  await writeFile('../../tests/web-desktop/video-frame-state-smoke.json', JSON.stringify(report, null, 2) + '\n');
  console.log(JSON.stringify({ passed, before, after }));
  if (!passed) throw new Error('Video counters did not increase');
} finally {
  await browser.close();
}
