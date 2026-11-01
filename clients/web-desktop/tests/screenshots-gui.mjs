// Isolated Electron lifecycle test. Screen pixels are generated in memory;
// neither desktopCapturer nor node-screenshots is used to read the desktop.
import { _electron as electron } from "@playwright/test";
import { build } from "esbuild";
import { mkdir, readFile, writeFile } from "node:fs/promises";
import { createHash } from "node:crypto";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import assert from "node:assert/strict";

const clientRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const root = resolve(clientRoot, "../..");
const temp = resolve(root, ".cache/screenshots-gui-386");
const output = resolve(root, "tests/delivery/windows-386-screenshots-gui");
await mkdir(temp, { recursive: true });
await mkdir(output, { recursive: true });
process.env.TEMP = temp;
process.env.TMP = temp;
const boundary = resolve(temp, "window-boundary.cjs");
const backend = resolve(temp, "synthetic-backend.cjs");
await writeFile(boundary, `for(const name of ['hideWindow','sendEvent','showWindow','toggleWindowVisible'])exports[name]=(...args)=>global.__screenshotWindowBoundary[name](...args);`);
await writeFile(backend, `const Screenshots=require(${JSON.stringify(resolve(clientRoot, "node_modules/electron-screenshots"))});module.exports=class SyntheticScreenshots extends Screenshots{constructor(options){super(options);global.__screenshotBackend=this;this.capture=async(display)=>{global.__screenshotState.captureCalls++;return global.__syntheticPng(display).toDataURL();};this.$view.webContents.on('console-message',(event,level,message)=>global.__screenshotState.viewConsole.push(String(event.message??message)));this.$view.webContents.on('render-process-gone',(_event,details)=>global.__screenshotState.errors.push(details));}};`);
const managerBundle = resolve(temp, "manager.cjs");
await build({
  absWorkingDir: clientRoot,
  entryPoints: ["electron/main/screenshotsManage.ts"],
  outfile: managerBundle, bundle: true, format: "cjs", platform: "node", target: "node22", packages: "external",
  plugins: [{ name: "synthetic-screen-boundary", setup(builder) {
    builder.onResolve({ filter: /^\.\/windowManage$/ }, () => ({ path: boundary, external: true }));
    builder.onResolve({ filter: /^electron-screenshots$/ }, () => ({ path: backend, external: true }));
    builder.onResolve({ filter: /^i18next$/ }, () => ({ path: resolve(clientRoot, "node_modules/i18next/dist/cjs/i18next.js"), external: true }));
  } }],
});
const preload = resolve(temp, "preload.cjs");
await writeFile(preload, `const {ipcRenderer}=require('electron');ipcRenderer.on('captureResponse',(_event,bytes)=>ipcRenderer.send('fixture:observed',{image:typeof bytes==='string',bytes:bytes?.length??0}));`);
const fixtureMain = resolve(temp, "main.cjs");
await writeFile(fixtureMain, `
const{app,BrowserWindow,ipcMain,nativeImage,desktopCapturer}=require('electron');
const crypto=require('node:crypto');
app.setPath('userData',${JSON.stringify(resolve(temp, "user-data"))});
global.__screenshotState={captureCalls:0,responses:[],observed:[],restores:0,errors:[],viewConsole:[],desktopCaptureCalls:0};
global.__screenshotWriteImage=async(target,path)=>{const view=target==='overlay'?global.__screenshotBackend.$view.webContents:global.__screenshotMain.webContents;const image=await view.capturePage();require('node:fs').writeFileSync(path,image.toPNG());};
desktopCapturer.getSources=async()=>{global.__screenshotState.desktopCaptureCalls++;throw new Error('Real desktop capture is forbidden in this test');};
global.__syntheticPng=display=>{const width=Math.max(640,Math.min(1920,display.width));const height=Math.max(480,Math.min(1080,display.height));const bytes=Buffer.alloc(width*height*4);for(let y=0;y<height;y++){for(let x=0;x<width;x++){const at=(y*width+x)*4;const stripe=(Math.floor(x/80)+Math.floor(y/80))%2;bytes[at]=stripe?225:245;bytes[at+1]=stripe?145:210;bytes[at+2]=stripe?40:110;bytes[at+3]=255;}}return nativeImage.createFromBitmap(bytes,{width,height});};
ipcMain.on('fixture:observed',(_event,value)=>global.__screenshotState.observed.push(value));
app.whenReady().then(async()=>{
const main=new BrowserWindow({width:640,height:480,show:false,title:'OpenIM synthetic screenshot fixture',webPreferences:{preload:${JSON.stringify(preload)},contextIsolation:true,nodeIntegration:false}});
await main.loadURL('data:text/html;charset=utf-8,'+encodeURIComponent('<!doctype html><html><body style="background:#f5f9ff;font:22px sans-serif;padding:32px"><h1>OpenIM screenshot test</h1><p>Synthetic content only. No desktop pixels.</p><div style="background:#1677ff;color:white;padding:24px">Capture result fixture</div></body></html>'));
main.show();global.__screenshotMain=main;
global.__screenshotWindowBoundary={
hideWindow:()=>main.hide(),toggleWindowVisible:visible=>main.setOpacity(visible?1:0),showWindow:()=>{global.__screenshotState.restores++;if(main.isMinimized())main.restore();main.show();main.focus();},
sendEvent:(name,value)=>{const overlay=global.__screenshotBackend.$win;global.__screenshotState.responses.push({image:typeof value==='string',bytes:value?.length??0,sha256:typeof value==='string'?crypto.createHash('sha256').update(Buffer.from(value.split(',')[1],'base64')).digest('hex'):null,overlayVisible:overlay?.isVisible()??false});main.webContents.send(name,value);}
};
global.__screenshotManager=require(${JSON.stringify(managerBundle)}).default();
global.__screenshotManagerAgain=require(${JSON.stringify(managerBundle)}).default();
global.__screenshotReady=true;
});
`);

const logs = [];
let app, report;
const startedAt = new Date().toISOString();
const wait = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
async function until(predicate, label, timeout = 12000) {
  const end = Date.now() + timeout;
  while (Date.now() < end) { const value = await predicate(); if (value) return value; await wait(100); }
  throw new Error(`Timed out: ${label}`);
}
try {
  app = await electron.launch({ executablePath: resolve(clientRoot, "node_modules/electron/dist/electron.exe"), args: [fixtureMain], cwd: clientRoot, env: { ...process.env, TEMP: temp, TMP: temp }, timeout: 30000 });
  app.process().stdout?.on("data", (data) => logs.push(data.toString()));
  app.process().stderr?.on("data", (data) => logs.push(data.toString()));
  await until(() => app.evaluate(() => Boolean(global.__screenshotReady)), "fixture ready");
  assert.equal(await app.evaluate(() => global.__screenshotManager === global.__screenshotManagerAgain), true);
  assert.equal(await app.evaluate(() => global.__screenshotManager.start(true)), true);
  await until(() => app.evaluate(() => global.__screenshotBackend.$win?.isVisible()), "capture overlay visible");
  await until(() => app.evaluate(() => global.__screenshotBackend.$view.webContents.executeJavaScript(`Boolean(document.querySelector('.screenshots-background img')?.complete || document.querySelector('.screenshots-background'))`)), "synthetic image loaded");
  // Real Chromium mouse events select a region, then click the actual tool bar.
  await app.evaluate(() => {
    const view = global.__screenshotBackend.$view.webContents;
    view.sendInputEvent({ type: "mouseDown", x: 100, y: 100, button: "left", clickCount: 1 });
    view.sendInputEvent({ type: "mouseMove", x: 480, y: 300, button: "left" });
    view.sendInputEvent({ type: "mouseUp", x: 480, y: 300, button: "left", clickCount: 1 });
  });
  const button = await until(() => app.evaluate(() => global.__screenshotBackend.$view.webContents.executeJavaScript(`(()=>{const el=document.querySelector('[title="确定"]');if(!el)return null;const r=el.getBoundingClientRect();const x=Math.round(r.x+r.width/2),y=Math.round(r.y+r.height/2);return r.width>0&&getComputedStyle(el).visibility==='visible'&&el.contains(document.elementFromPoint(x,y))?{x,y}:null})()`)), "actual visible confirm tool bar");
  await app.evaluate((electron, path) => global.__screenshotWriteImage("overlay", path), resolve(output, "synthetic-overlay-before-confirm.png"));
  await app.evaluate(() => global.__screenshotBackend.$view.webContents.executeJavaScript(`document.querySelector('[title="确定"]').click()`));
  await until(() => app.evaluate(() => global.__screenshotState.observed.length === 1), "first response received by real preload IPC");
  const confirmed = await app.evaluate(() => ({ ...global.__screenshotState, overlayVisible: global.__screenshotBackend.$win.isVisible(), mainVisible: global.__screenshotMain.isVisible(), mainFocused: global.__screenshotMain.isFocused(), mainOpacity: global.__screenshotMain.getOpacity() }));
  assert.equal(confirmed.responses.length, 1);
  assert.equal(confirmed.responses[0].image, true);
  assert.equal(confirmed.responses[0].overlayVisible, false);
  assert.equal(confirmed.overlayVisible, false);
  assert.equal(confirmed.mainVisible, true);
  assert.equal(confirmed.mainOpacity, 1);
  // Replay stale terminal events through the REAL screenshot preload after
  // closure: no second result, reopening or screen capture is permitted.
  await app.evaluate(async () => global.__screenshotBackend.$view.webContents.executeJavaScript(`window.screenshots.ok(new Uint8Array([1,2,3]).buffer,{});window.screenshots.cancel();`));
  await wait(250);
  assert.equal(await app.evaluate(() => global.__screenshotState.responses.length), 1);
  assert.equal(await app.evaluate(() => global.__screenshotState.captureCalls), 1);
  assert.equal(await app.evaluate(() => global.__screenshotBackend.$win.isVisible()), false);
  assert.equal(await app.evaluate(() => global.__screenshotManager.start(true)), true);
  await until(() => app.evaluate(() => global.__screenshotBackend.$win.isVisible()), "second capture visible");
  await app.evaluate(() => {
    const view = global.__screenshotBackend.$view.webContents;
    view.sendInputEvent({ type: "keyDown", keyCode: "ESC" });
    view.sendInputEvent({ type: "keyUp", keyCode: "ESC" });
  });
  await until(() => app.evaluate(() => global.__screenshotState.observed.length === 2), "Escape cancellation response");
  const final = await app.evaluate(() => ({ ...global.__screenshotState, overlayVisible: global.__screenshotBackend.$win.isVisible(), mainVisible: global.__screenshotMain.isVisible(), mainFocused: global.__screenshotMain.isFocused(), mainOpacity: global.__screenshotMain.getOpacity(), electron: process.versions.electron, node: process.versions.node }));
  assert.equal(final.captureCalls, 2);
  assert.equal(final.responses.length, 2);
  assert.equal(final.responses[1].image, false);
  assert.equal(final.overlayVisible, false);
  assert.equal(final.mainVisible, true);
  assert.equal(final.mainOpacity, 1);
  assert.equal(final.desktopCaptureCalls, 0);
  await app.evaluate((electron, path) => global.__screenshotWriteImage("main", path), resolve(output, "synthetic-main-after-cancel.png"));
  report = { result: "PASS", startedAt, finishedAt: new Date().toISOString(), scope: "Actual Electron 45 screenshot BrowserWindow/BrowserView, actual select-region/confirm tool bar, dependency preload IPC, main-window preload response, cancel/reopen. All capture pixels are synthetic.", confirmed, final, skipped: ["No real desktop capture", "No business-account login", "No messages sent", "No full application package/installation"] };
} catch (error) {
  let state;
  if (app) state = await app.evaluate(async () => ({state:global.__screenshotState,overlayVisible:global.__screenshotBackend?.$win?.isVisible(),dom:await global.__screenshotBackend?.$view?.webContents.executeJavaScript(`(()=>{const el=document.querySelector('[title="确定"]');const r=el?.getBoundingClientRect();return {html:document.body.innerHTML.slice(0,4000),confirm:r?{x:r.x,y:r.y,width:r.width,height:r.height,visibility:getComputedStyle(el).visibility}:null}})()`)})).catch(()=>undefined);
  report = { result: "FAIL", startedAt, finishedAt: new Date().toISOString(), error: String(error.stack ?? error), state };
  throw error;
} finally {
  if (app) await app.close().catch(() => {});
  await writeFile(resolve(output, "electron-process.log"), logs.join(""));
  const files = [];
  for (const name of ["synthetic-overlay-before-confirm.png", "synthetic-main-after-cancel.png", "electron-process.log"]) {
    try { const bytes = await readFile(resolve(output, name)); files.push({ name, bytes: bytes.length, sha256: createHash("sha256").update(bytes).digest("hex") }); } catch {}
  }
  await writeFile(resolve(output, "result.json"), JSON.stringify({ ...report, files }, null, 2) + "\n");
}
console.log(JSON.stringify({ result: report.result, output, captures: report.final.captureCalls, responses: report.final.responses.length, desktopCaptureCalls: report.final.desktopCaptureCalls }));
