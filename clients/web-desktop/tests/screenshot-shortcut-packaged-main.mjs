// Actual packaged Main/preload/login renderer smoke; synthetic profile only.
// Does not log in, trigger capture, dispatch global OS keys or read clipboard.
import { _electron as electron } from '@playwright/test';
import assert from 'node:assert/strict';
import { createHash, randomUUID } from 'node:crypto';
import { createReadStream } from 'node:fs';
import { mkdir, readFile, writeFile } from 'node:fs/promises';
import { resolve, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';

const client = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const root = resolve(client, '../..');
const exe = resolve(client, 'release/win-unpacked/PUTY_IM.exe');
const asar = resolve(client, 'release/win-unpacked/resources/app.asar');
const nonce = randomUUID();
const profile = resolve('D:/puty-build-temp', `screenshot-shortcut-packaged-${nonce}`);
const dataRoot = resolve(profile, 'encrypted-data');
const output = resolve(root, '.private/production/acceptance/screenshot-shortcut', `packaged-main-${nonce}`);
await mkdir(dataRoot, { recursive: true });
await mkdir(output, { recursive: true });
await writeFile(resolve(dataRoot, '.puty-local-root.json'), JSON.stringify({ format: 'PUTY-LOCAL-ROOT-1', id: nonce }), { flag: 'wx' });
async function fileHash(file) { const hash = createHash('sha256');let bytes = 0;for await (const chunk of createReadStream(file)){hash.update(chunk);bytes += chunk.length;}return {sha256:hash.digest('hex'),bytes}; }
const packageBefore = {exe:await fileHash(exe),asar:await fileHash(asar)};
const logs = [], checks = [];
const startedAt = new Date().toISOString();
const wait = ms => new Promise(resolve => setTimeout(resolve, ms));
let app, page, original, report, round=0;
async function until(predicate, label, timeout=30000) { const end = Date.now()+timeout;while(Date.now()<end){const result=await predicate();if(result)return result;await wait(100);}throw Error(`Timed out: ${label}`); }
const invoke = (operation, input) => page.evaluate(({operation,input}) => window.electronAPI.ipcInvoke('puty-screenshot-shortcut',operation,input), {operation,input});
const registered = key => app.evaluate(({globalShortcut}, key) => globalShortcut.isRegistered(key),key);
function check(name,detail){checks.push({name,passed:true,detail});}
async function launch(){
 round++;
 const env={};for(const key of ['SystemRoot','WINDIR','PATH','TEMP','TMP','USERPROFILE','APPDATA','LOCALAPPDATA','PROGRAMDATA','ComSpec','PATHEXT'])if(process.env[key]!==undefined)env[key]=process.env[key];Object.assign(env,{TEMP:'D:/puty-build-temp',TMP:'D:/puty-build-temp',OPENIM38_TEST_DATA_DIR:profile,OPENIM38_SMOKE_TEST:'1'});
 app=await electron.launch({executablePath:exe,args:['--proxy-server=http://127.0.0.1:9','--proxy-bypass-list=<-loopback>'],cwd:client,env,timeout:60000});
 app.process().stdout?.on('data',data=>logs.push(data.toString()));app.process().stderr?.on('data',data=>logs.push(data.toString()));
 await app.context().route(/^https?:\/\//,route=>route.abort());
 page=await until(async()=>app.windows().find(page=>page.url().startsWith('openim38://app/')),'actual packaged renderer');
 await page.waitForURL(/#\/login(?:\?|$)/,{timeout:60000});
 await page.locator('input[autocomplete="username"]').waitFor({state:'visible',timeout:60000});
 await page.locator('input[autocomplete="current-password"]').waitFor({state:'visible'});
 assert.equal(await page.locator('input[autocomplete="username"]').inputValue(),'');assert.equal(await page.locator('input[autocomplete="current-password"]').inputValue(),'');
 await app.evaluate(({app,globalShortcut}, currentRound)=>{app.once('will-quit',()=>process.stdout.write('PUTY_SHORTCUT_QA_QUIT '+JSON.stringify({round:currentRound,defaultRegistered:globalShortcut.isRegistered('Control+Alt+A'),customRegistered:globalShortcut.isRegistered('Control+Alt+F9')})+'\n'));},round);
 return app.evaluate(({app})=>({version:app.getVersion(),packaged:app.isPackaged,electron:process.versions.electron,appPath:app.getAppPath(),userData:app.getPath('userData')}));
}
async function close(){await app.close();app=undefined;await wait(100);const markers=logs.join('').split(/\r?\n/).filter(line=>line.startsWith('PUTY_SHORTCUT_QA_QUIT ')).map(line=>JSON.parse(line.slice('PUTY_SHORTCUT_QA_QUIT '.length)));const proof=markers.find(value=>value.round===round);assert.ok(proof,'Actual will-quit observer missing');assert.equal(proof.defaultRegistered,false);assert.equal(proof.customRegistered,false);check('normal packaged quit releases screenshot registration',proof);}
try{
 const runtime=await launch();assert.equal(runtime.packaged,true);assert.equal(runtime.userData,profile);assert.ok(runtime.appPath.endsWith('resources\\app.asar')||runtime.appPath.endsWith('resources/app.asar'));check('unmodified packaged Main/preload shows normal empty login form',runtime);
 original=await invoke('get');assert.equal(original.persistent,true);assert.equal(typeof original.enabled,'boolean');assert.equal(typeof original.accelerator,'string');check('dedicated IPC reads persistent production manager state',original);
 await page.screenshot({path:resolve(output,'01-packaged-login.png')});
 const saved=await invoke('set',{enabled:true,accelerator:'Control+Alt+F9'});assert.equal(saved.ok,true);assert.equal(saved.state.persistent,true);assert.equal(saved.state.accelerator,'Control+Alt+F9');assert.equal(await registered('Control+Alt+F9'),true);check('real packaged set IPC commits and registers F9',saved);
 const cipher=await readFile(resolve(dataRoot,'shared/ui-preferences.puty'));assert.equal(cipher.includes(Buffer.from('Control+Alt+F9')),false);check('production encrypted preference file has no plaintext shortcut',{...await fileHash(resolve(dataRoot,'shared/ui-preferences.puty')),plaintextShortcutAbsent:true});
 await close();
 const restartedRuntime=await launch();const restarted=await invoke('get');assert.equal(restarted.persistent,true);assert.equal(restarted.enabled,true);assert.equal(restarted.accelerator,'Control+Alt+F9');assert.equal(await registered('Control+Alt+F9'),true);check('fresh packaged Main decrypts and registers committed F9',{runtime:restartedRuntime,state:restarted});
 const disabled=await invoke('set',{enabled:false,accelerator:'Control+Alt+F9'});assert.equal(disabled.ok,true);assert.equal(await registered('Control+Alt+F9'),false);check('real packaged disable IPC unregisters F9',disabled);
 const restored=await invoke('set',{enabled:original.enabled,accelerator:original.accelerator});
 if(restored.ok){assert.equal(restored.state.enabled,original.enabled);assert.equal(restored.state.accelerator,original.accelerator);check('original configuration restored in synthetic profile',restored);}
 else{assert.equal(restored.code,'conflict');assert.equal(original.accelerator,'Control+Alt+A');assert.equal(original.registered,false);assert.equal(restored.state.enabled,false);assert.equal(restored.state.accelerator,'Control+Alt+F9');assert.equal(await registered('Control+Alt+F9'),false);check('restoring originally occupied default correctly rejects and leaves synthetic F9 disabled',{restoredOriginal:false,reason:'Original default was already externally occupied; no external shortcut was removed',result:restored});}
 await page.screenshot({path:resolve(output,'02-packaged-login-after-shortcut-check.png')});
 await close();
 const packageAfter={exe:await fileHash(exe),asar:await fileHash(asar)};assert.deepEqual(packageAfter,packageBefore);
 report={result:'PASS',startedAt,finishedAt:new Date().toISOString(),checks,packageBefore,packageAfter,originalConfigurationRestored:restored.ok,syntheticProfile:profile,boundaries:['Actual packaged ASAR Main, original preload and normal login renderer; no source or dependency mocks','No account entered, authentication attempt, SDK login, message, upload or server fixture','Outbound HTTP/S blocked by dead proxy from process startup plus Playwright routing','No screenshot callback or global physical key triggered, no desktop or clipboard API called by this test','Only observational will-quit listener attached after production initialization','No NSIS installation, employee data access, APK or existing profile modified','Synthetic profile retained for evidence; original default may remain un-restorable because external OS shortcut occupancy is respected']};
}catch(error){report={result:'FAIL',startedAt,finishedAt:new Date().toISOString(),error:String(error.stack??error),checks,packageBefore,original,syntheticProfile:profile};process.exitCode=1;}
finally{if(app){try{if(page)await invoke('set',{enabled:false,accelerator:'Control+Alt+F9'});}catch{}await app.close().catch(()=>{});}await writeFile(resolve(output,'electron-process.log'),logs.join(''),{flag:'wx'});await writeFile(resolve(output,'result.json'),JSON.stringify(report,null,2)+'\n',{flag:'wx'});}
console.log(JSON.stringify({result:report.result,checks:checks.length,output,report:await fileHash(resolve(output,'result.json'))}));
