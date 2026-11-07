import test from 'node:test';
import assert from 'node:assert/strict';
import {build} from 'esbuild';
import {createCipheriv,createDecipheriv,randomBytes,randomUUID,createHash} from 'node:crypto';
import {promises as fs} from 'node:fs';
import {tmpdir} from 'node:os';
import path from 'node:path';

const file=root=>path.join(root,'shared','ui-preferences.puty');
const wait=async check=>{for(let i=0;i<500;i++){if(check())return;await new Promise(resolve=>setTimeout(resolve,2));}throw Error('Synthetic preference gate did not open');};
async function fixture(t){
  const directory=await fs.realpath(await fs.mkdtemp(path.join(tmpdir(),'puty-ui-prefs-'))),master=randomBytes(32),key='__prefs_'+randomUUID().replaceAll('-','');
  let available=true,hold=false,held=false,release,quitCount=0,errorCount=0;const events=new Map();
  const encrypt=async text=>{if(hold){held=true;hold=false;await new Promise(resolve=>release=resolve);}const nonce=randomBytes(12),cipher=createCipheriv('aes-256-gcm',master,nonce);return Buffer.concat([nonce,cipher.update(text,'utf8'),cipher.final(),cipher.getAuthTag()]);};
  const decrypt=async bytes=>{const buffer=Buffer.from(bytes),decipher=createDecipheriv('aes-256-gcm',master,buffer.subarray(0,12));decipher.setAuthTag(buffer.subarray(-16));return {result:Buffer.concat([decipher.update(buffer.subarray(12,-16)),decipher.final()]).toString(),shouldReEncrypt:false};};
  globalThis[key]={app:{whenReady:async()=>{},on:(name,callback)=>events.set(name,callback),quit:()=>quitCount++},dialog:{showErrorBox:()=>errorCount++},shell:{},systemPreferences:{},safeStorage:{isAsyncEncryptionAvailable:async()=>available,encryptStringAsync:encrypt,decryptStringAsync:decrypt,getSelectedStorageBackend:()=> 'kwallet6'}};
  async function load(withAppManage=false){const output=await build({...(withAppManage?{stdin:{contents:"export * from './electron/main/storeManage';export {setAppListener} from './electron/main/appManage';",resolveDir:process.cwd(),loader:'ts'}}:{entryPoints:['electron/main/storeManage.ts']}),bundle:true,write:false,platform:'node',format:'esm',plugins:[{name:'preference-electron',setup(build){build.onResolve({filter:/^electron$/},()=>({path:'electron',namespace:'mock'}));build.onLoad({filter:/.*/,namespace:'mock'},()=>({contents:'export const {app,safeStorage,dialog,shell,systemPreferences}=globalThis['+JSON.stringify(key)+'];',loader:'js'}));if(withAppManage){build.onResolve({filter:/^\./},args=>args.importer.endsWith('appManage.ts')&&args.path!=='./storeManage'?{path:args.path,namespace:'app-stub'}:undefined);build.onLoad({filter:/.*/,namespace:'app-stub'},()=>({contents:'export const isMac=false,isProd=false,isWin=true,singleInstanceLock=true,legacyUserData="synthetic-only";export const isExistMainWindow=()=>true,showWindow=()=>{},checkClearLogs=()=>{},flicker=()=>{};',loader:'js'}));}}}]});return import('data:text/javascript;base64,'+Buffer.from(output.outputFiles[0].text).toString('base64')+'#'+randomUUID());}
  async function root(name){const value=path.join(directory,name);await fs.mkdir(value);await fs.writeFile(path.join(value,'.puty-local-root.json'),JSON.stringify({format:'PUTY-LOCAL-ROOT-1',id:randomUUID()}));return value;}
  t.after(async()=>{release?.();delete globalThis[key];master.fill(0);assert.equal(path.dirname(directory),await fs.realpath(tmpdir()));assert.ok(path.basename(directory).startsWith('puty-ui-prefs-'));await fs.rm(directory,{recursive:true,force:true});});
  return {directory,root,load,document:async root=>JSON.parse((await decrypt(await fs.readFile(file(root)))).result),available:value=>available=value,hold:()=>{hold=true;held=false;},held:()=>held,release:()=>release(),beforeQuit:()=>{let prevented=false;events.get('before-quit')({preventDefault:()=>prevented=true});return prevented;},quitCount:()=>quitCount,errorCount:()=>errorCount};
}

test('before selecting a root preferences stay in RAM and arbitrary business keys are rejected',async t=>{
  const f=await fixture(t),api=await f.load(),store=api.getStore();await api.initProtectedPreferences();store.set('language','zh-CN').set('closeAction','miniSize');await api.flushPreferences();
  assert.equal(store.get('language'),'zh-CN');assert.deepEqual(await fs.readdir(f.directory),[]);
  for(const key of ['token','chatToken','accountID','password','messages','__proto__','language.secret']){assert.throws(()=>store.set(key,'synthetic-private'),/仅允许/);assert.throws(()=>store.get(key),/仅允许/);}
  assert.throws(()=>store.set('language','synthetic-private'),/无效/);assert.throws(()=>store.set('closeAction',{token:'synthetic'}),/无效/);
});

test('only an OS-protected shared blob persists UI keys and a fresh process can read it',async t=>{
  const f=await fixture(t),root=await f.root('encrypted'),api=await f.load();await api.initProtectedPreferences(root);api.getStore().set('language','zh-CN').set('closeAction','quit').set('partitionRandom',3);await api.flushPreferences();
  const raw=await fs.readFile(file(root));assert.equal(raw.includes(Buffer.from('language')),false);assert.equal(raw.includes(Buffer.from('zh-CN')),false);assert.deepEqual(await fs.readdir(root),['.puty-local-root.json','shared']);
  const fresh=await f.load();await fresh.initProtectedPreferences(root);assert.equal(fresh.getStore().get('language'),'zh-CN');assert.equal(fresh.getStore().get('closeAction'),'quit');assert.equal(fresh.getStore().get('partitionRandom'),3);
  assert.deepEqual(await f.document(root),{format:'PUTY-UI-PREFERENCES-1',preferences:{language:'zh-CN',closeAction:'quit',partitionRandom:3}});
});

test('concurrent setters serialize the latest UI state without dropping another key',async t=>{
  const f=await fixture(t),root=await f.root('encrypted'),api=await f.load();await api.initProtectedPreferences(root);
  api.getStore().set('language','zh-CN');api.getStore().set('closeAction','quit');api.getStore().set('language','en-US');await api.flushPreferences();assert.deepEqual((await f.document(root)).preferences,{language:'en-US',closeAction:'quit'});
});

test('rapid root changes cannot promote an old in-flight UI snapshot to the wrong root',async t=>{
  const f=await fixture(t),a=await f.root('a'),b=await f.root('b'),c=await f.root('c'),api=await f.load();await api.initProtectedPreferences(a);api.getStore().set('language','en-US');await api.flushPreferences();const before=createHash('sha256').update(await fs.readFile(file(a))).digest('hex');
  f.hold();api.getStore().set('language','zh-CN');await wait(f.held);const stale=api.bindPreferencesRoot(b).then(()=>true,()=>false);api.getStore().set('language','en-US');const latest=api.bindPreferencesRoot(c);f.release();assert.equal(await stale,false);await latest;await api.flushPreferences();
  assert.equal(createHash('sha256').update(await fs.readFile(file(a))).digest('hex'),before);await assert.rejects(fs.stat(file(b)),{code:'ENOENT'});assert.equal((await f.document(c)).preferences.language,'en-US');
});

test('unavailable OS protection fails explicitly and never falls back to plaintext',async t=>{
  const f=await fixture(t),root=await f.root('encrypted'),api=await f.load();f.available(false);assert.deepEqual(await api.initProtectedPreferences(root),{mode:'memory',warning:'protection'});api.getStore().set('language','zh-CN');await api.flushPreferences();assert.equal(api.getStore().get('language'),'zh-CN');assert.deepEqual(await fs.readdir(root),['.puty-local-root.json']);
  f.available(true);await api.bindPreferencesRoot(root);const before=await fs.readFile(file(root));f.available(false);api.getStore().set('closeAction','quit');await assert.rejects(api.flushPreferences(),/未使用明文降级/);assert.deepEqual(await fs.readFile(file(root)),before);
});

test('tampered encrypted preferences and redirected shared paths are never replaced with empty preferences',async t=>{
  const f=await fixture(t),root=await f.root('encrypted'),api=await f.load();await api.initProtectedPreferences(root);api.getStore().set('language','zh-CN');await api.flushPreferences();const damaged=await fs.readFile(file(root));damaged[15]^=1;await fs.writeFile(file(root),damaged);
  const fresh=await f.load();assert.deepEqual(await fresh.initProtectedPreferences(root),{mode:'memory',warning:'decrypt'});fresh.getStore().set('language','en-US');await fresh.flushPreferences();assert.deepEqual(await fs.readFile(file(root)),damaged);await assert.rejects(fresh.bindPreferencesRoot(root),/未使用明文降级/);assert.deepEqual(await fs.readFile(file(root)),damaged);
  const redirected=await f.root('redirected'),target=await f.root('target');await fs.symlink(target,path.join(redirected,'shared'),process.platform==='win32'?'junction':'dir');const other=await f.load();assert.deepEqual(await other.initProtectedPreferences(redirected),{mode:'memory',warning:'preferences-path'});await other.flushPreferences();assert.deepEqual(await fs.readdir(target),['.puty-local-root.json']);
});

test('normal quit waits for queued encrypted UI writes and reports failures without plaintext fallback',async t=>{
  const f=await fixture(t),root=await f.root('encrypted'),api=await f.load(true);await api.initProtectedPreferences(root);api.setAppListener(()=>{});
  f.hold();api.getStore().set('language','zh-CN');await wait(f.held);assert.equal(f.beforeQuit(),true);assert.equal(f.quitCount(),0);f.release();await wait(()=>f.quitCount()===1);assert.equal((await f.document(root)).preferences.language,'zh-CN');assert.equal(f.beforeQuit(),false);
  const next=await f.load(true);await next.initProtectedPreferences(root);next.setAppListener(()=>{});f.available(false);next.getStore().set('closeAction','quit');assert.equal(f.beforeQuit(),true);await wait(()=>f.quitCount()===2);assert.equal(f.errorCount(),1);assert.equal((await f.document(root)).preferences.closeAction,undefined);
});

test('failed encrypted temporary write preserves the previous blob and removes its exact partial file',async t=>{
  const f=await fixture(t),root=await f.root('encrypted'),api=await f.load();await api.initProtectedPreferences(root);api.getStore().set('language','en-US');await api.flushPreferences();const before=await fs.readFile(file(root)),original=fs.open;
  fs.open=async(value,...args)=>{const handle=await original(value,...args);if(String(value).includes('ui-preferences.puty.next-'))return {writeFile:async()=>{throw Object.assign(Error('synthetic disk full'),{code:'ENOSPC'});},sync:async()=>{},close:()=>handle.close()};return handle;};
  try{api.getStore().set('language','zh-CN');await assert.rejects(api.flushPreferences(),/未使用明文降级/);assert.deepEqual(await fs.readFile(file(root)),before);assert.deepEqual(await fs.readdir(path.dirname(file(root))),['ui-preferences.puty']);}finally{fs.open=original;}
});

test('startup only reads a healthy encrypted blob and disk-full still fails a later real setting write',async t=>{
  const f=await fixture(t),root=await f.root('encrypted'),first=await f.load();await first.initProtectedPreferences(root);first.getStore().set('language','zh-CN');await first.flushPreferences();
  const before=await fs.readFile(file(root)),original=fs.open;let attempts=0;
  fs.open=async(value,...args)=>{if(String(value).includes('ui-preferences.puty.next-')){attempts++;throw Object.assign(Error('synthetic disk full'),{code:'ENOSPC'});}return original(value,...args);};
  try{const fresh=await f.load();assert.deepEqual(await fresh.initProtectedPreferences(root),{mode:'persistent'});assert.equal(fresh.getStore().get('language'),'zh-CN');assert.equal(attempts,0);await fresh.flushPreferences();
    fresh.getStore().set('language','en-US');await assert.rejects(fresh.flushPreferences(),/未使用明文降级/);assert.equal(attempts,1);assert.deepEqual(await fs.readFile(file(root)),before);
  }finally{fs.open=original;}
});

test('missing root marker uses UI defaults in RAM without recreating any deleted file or emitting a false quit write error',async t=>{
  const f=await fixture(t),root=await f.root('partial-delete'),api=await f.load(true);await fs.unlink(path.join(root,'.puty-local-root.json'));await fs.writeFile(path.join(root,'KEEP.enc'),Buffer.from('SYNTHETIC_ENCRYPTED_VAULT_PLACEHOLDER'));
  const before=await fs.readFile(path.join(root,'KEEP.enc'));assert.deepEqual(await api.initProtectedPreferences(root),{mode:'memory',warning:'root-marker'});api.getStore().set('closeAction','quit');await api.flushPreferences();api.setAppListener(()=>{});assert.equal(f.beforeQuit(),true);await wait(()=>f.quitCount()===1);assert.equal(f.errorCount(),0);assert.deepEqual(await fs.readdir(root),['KEEP.enc']);assert.deepEqual(await fs.readFile(path.join(root,'KEEP.enc')),before);
});

test('failed startup leaves an existing OS-protected blob untouched and a later fresh process can recover it',async t=>{
  const f=await fixture(t),root=await f.root('existing'),first=await f.load();await first.initProtectedPreferences(root);first.getStore().set('language','zh-CN').set('partitionRandom',4);await first.flushPreferences();const before=await fs.readFile(file(root));
  f.available(false);const blocked=await f.load();assert.deepEqual(await blocked.initProtectedPreferences(root),{mode:'memory',warning:'protection'});assert.equal(blocked.getStore().get('language'),undefined);blocked.getStore().set('language','en-US');await blocked.flushPreferences();assert.deepEqual(await fs.readFile(file(root)),before);
  f.available(true);const recovered=await f.load();assert.deepEqual(await recovered.initProtectedPreferences(root),{mode:'persistent'});assert.equal(recovered.getStore().get('language'),'zh-CN');assert.equal(recovered.getStore().get('partitionRandom'),4);assert.deepEqual(await fs.readFile(file(root)),before);
});

test('startup warning contains only a fixed phase and the application continues after one non-fatal notice',async()=>{
  const source=await fs.readFile('electron/main/index.ts','utf8');
  assert.match(source,/const preferences = await initProtectedPreferences\(configuredRoot\(\)\)/);assert.match(source,/if \(startupStorageError \|\| preferences\.warning\) await dialog\.showMessageBox\(/);
  assert.match(source,/诊断阶段：\$\{startupStorageError \? 'storage-location' : preferences\.warning\}/);assert.match(source,/原有加密设置、系统密钥和聊天档案均未更改/);
  assert.equal(source.includes("if(startupStorageError){"),false);assert.ok(source.indexOf('if (startupStorageError || preferences.warning)')<source.indexOf('isLinux ? setTimeout(init, 300) : init()'));
});

test('screenshot shortcut strict save requires a protected root and generic setters cannot bypass it',async t=>{
  const f=await fixture(t),api=await f.load(),config={enabled:true,accelerator:'Control+Alt+A'};
  await api.initProtectedPreferences();assert.equal(api.hasPersistentPreferences(),false);
  await assert.rejects(api.saveScreenshotShortcutPreference(config),/未使用明文降级/);
  assert.equal(api.getStore().get('screenshotShortcut'),undefined);
  assert.throws(()=>api.getStore().set('screenshotShortcut',config),/截图快捷键设置/);
  for(const invalid of [{enabled:true,accelerator:'Ctrl+Alt+A'},{enabled:true,accelerator:'Control+F12'},{...config,extra:true}])
    await assert.rejects(api.saveScreenshotShortcutPreference(invalid),/截图快捷键设置无效/);
  assert.deepEqual(await fs.readdir(f.directory),[]);
});

test('screenshot shortcut is ciphertext on disk and a fresh process reads enabled and disabled values',async t=>{
  const f=await fixture(t),root=await f.root('shortcut-encrypted'),api=await f.load();await api.initProtectedPreferences(root);
  assert.equal(api.hasPersistentPreferences(),true);
  const first={enabled:true,accelerator:'Control+Alt+B'};await api.saveScreenshotShortcutPreference(first);
  const bytes=await fs.readFile(file(root));
  for(const plaintext of ['screenshotShortcut','Control+Alt+B','enabled'])assert.equal(bytes.includes(Buffer.from(plaintext)),false);
  assert.deepEqual((await f.document(root)).preferences.screenshotShortcut,first);
  const fresh=await f.load();await fresh.initProtectedPreferences(root);assert.deepEqual(fresh.getStore().get('screenshotShortcut'),first);
  const disabled={enabled:false,accelerator:'Control+Shift+F8'};await fresh.saveScreenshotShortcutPreference(disabled);
  const next=await f.load();await next.initProtectedPreferences(root);assert.deepEqual(next.getStore().get('screenshotShortcut'),disabled);
});

test('failed screenshot shortcut write preserves both committed RAM value and exact old ciphertext',async t=>{
  const f=await fixture(t),root=await f.root('shortcut-failure'),api=await f.load();await api.initProtectedPreferences(root);
  const prior={enabled:true,accelerator:'Control+Alt+A'};await api.saveScreenshotShortcutPreference(prior);
  const before=await fs.readFile(file(root)),original=fs.open;
  fs.open=async(value,...args)=>{const handle=await original(value,...args);if(String(value).includes('ui-preferences.puty.next-'))return {writeFile:async()=>{throw Object.assign(Error('synthetic shortcut disk full'),{code:'ENOSPC'});},sync:async()=>{},close:()=>handle.close()};return handle;};
  try{
    await assert.rejects(api.saveScreenshotShortcutPreference({enabled:false,accelerator:'Control+Alt+B'}),/未使用明文降级/);
    assert.deepEqual(api.getStore().get('screenshotShortcut'),prior);
    assert.deepEqual(await fs.readFile(file(root)),before);
    assert.deepEqual(await fs.readdir(path.dirname(file(root))),['ui-preferences.puty']);
  }finally{fs.open=original;}
  api.getStore().set('language','en-US');await api.flushPreferences();
  assert.deepEqual((await f.document(root)).preferences,{screenshotShortcut:prior,language:'en-US'});
});

test('ordinary UI writes arriving during shortcut encryption keep both changes after commit and restart',async t=>{
  const f=await fixture(t),root=await f.root('shortcut-concurrent'),api=await f.load();await api.initProtectedPreferences(root);
  api.getStore().set('language','zh-CN');await api.flushPreferences();
  const value={enabled:true,accelerator:'Control+Alt+B'};
  f.hold();const pending=api.saveScreenshotShortcutPreference(value);await wait(f.held);
  assert.equal(api.getStore().get('screenshotShortcut'),undefined,'uncommitted shortcut must stay out of RAM');
  api.getStore().set('language','en-US').set('closeAction','quit');
  f.release();await pending;await api.flushPreferences();
  assert.deepEqual((await f.document(root)).preferences,{language:'en-US',closeAction:'quit',screenshotShortcut:value});
  const fresh=await f.load();await fresh.initProtectedPreferences(root);
  assert.equal(fresh.getStore().get('language'),'en-US');assert.equal(fresh.getStore().get('closeAction'),'quit');
  assert.deepEqual(fresh.getStore().get('screenshotShortcut'),value);
});

test('root rebinding cancels an in-flight shortcut snapshot without committing it to either root',async t=>{
  const f=await fixture(t),a=await f.root('shortcut-root-a'),b=await f.root('shortcut-root-b'),api=await f.load();await api.initProtectedPreferences(a);
  const prior={enabled:true,accelerator:'Control+Alt+A'};await api.saveScreenshotShortcutPreference(prior);
  const before=await fs.readFile(file(a));
  f.hold();const pending=api.saveScreenshotShortcutPreference({enabled:true,accelerator:'Control+Alt+B'}).then(()=>true,()=>false);await wait(f.held);
  const moved=api.bindPreferencesRoot(b);f.release();assert.equal(await pending,false);await moved;await api.flushPreferences();
  assert.deepEqual(await fs.readFile(file(a)),before);
  assert.deepEqual(api.getStore().get('screenshotShortcut'),prior);
  assert.deepEqual((await f.document(b)).preferences.screenshotShortcut,prior);
});
