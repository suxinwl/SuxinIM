import test from 'node:test';
import assert from 'node:assert/strict';
import {build} from 'esbuild';
import {promises as fs} from 'node:fs';
import path from 'node:path';
import {tmpdir} from 'node:os';
import {createRequire} from 'node:module';
import {spawnSync} from 'node:child_process';
import {randomUUID} from 'node:crypto';

test('real Electron OS protection preserves damaged UI preferences and can reopen the original encrypted settings', {skip:process.platform!=='win32',timeout:60000}, async t=>{
  const base=await fs.realpath(await fs.mkdtemp(path.join(tmpdir(),'puty-prefs-electron-')));
  try{
    const profile=path.join(base,'os-profile'),root=path.join(base,'encrypted-data');await fs.mkdir(profile);await fs.mkdir(root);
    await fs.writeFile(path.join(root,'.puty-local-root.json'),JSON.stringify({format:'PUTY-LOCAL-ROOT-1',id:randomUUID()}));
    await build({entryPoints:['electron/main/storeManage.ts'],outfile:path.join(base,'preferences.cjs'),bundle:true,platform:'node',target:'node22',format:'cjs',external:['electron']});
    const entry=path.join(base,'main.cjs');
    await fs.writeFile(entry,`const {app,safeStorage}=require('electron');const fs=require('node:fs/promises');const path=require('node:path');
app.setPath('userData',path.join(__dirname,'os-profile'));app.disableHardwareAcceleration();
app.whenReady().then(async()=>{
 const api=require('./preferences.cjs'),root=path.join(__dirname,'encrypted-data'),file=path.join(root,'shared','ui-preferences.puty');
 const phase=process.argv[2],out={phase,electron:process.versions.electron,ready:app.isReady(),asyncAvailable:null,syncAvailable:null,asyncRoundtrip:false,syncRoundtrip:false};
 try{out.asyncAvailable=await safeStorage.isAsyncEncryptionAvailable();}catch{out.asyncAvailabilityError=true;}
 try{out.syncAvailable=safeStorage.isEncryptionAvailable();}catch{out.syncAvailabilityError=true;}
 const text='PUTY_SYNTHETIC_PREFERENCE_PROBE';
 if(out.asyncAvailable===true){const c=await safeStorage.encryptStringAsync(text);out.asyncRoundtrip=(await safeStorage.decryptStringAsync(c)).result===text;c.fill(0);}
 if(out.syncAvailable===true){const c=safeStorage.encryptString(text);out.syncRoundtrip=safeStorage.decryptString(c)===text;c.fill(0);}
 const before=await fs.readFile(file).catch(e=>{if(e.code!=='ENOENT')throw e;return undefined;});
 out.startup=await api.initProtectedPreferences(root);out.loadedLanguage=api.getStore().get('language')??null;
 api.getStore().set('language',phase==='create'?'zh-CN':'en-US');await api.flushPreferences();
 const after=await fs.readFile(file).catch(e=>{if(e.code!=='ENOENT')throw e;return undefined;});
 out.ramValue=api.getStore().get('language');out.blobExists=Boolean(after);out.originalBytesPreserved=Boolean(before&&after&&before.equals(after));
 out.encrypted=Boolean(after&&!after.includes(Buffer.from('language'))&&!after.includes(Buffer.from('zh-CN')));
 console.log('PUTY_PREFS_RESULT:'+JSON.stringify(out));app.quit();
}).catch(()=>{console.log('PUTY_PREFS_RESULT:'+JSON.stringify({passed:false,code:'NATIVE_PREFERENCE_TEST_FAILED'}));app.quit();});`);
    const electron=createRequire(import.meta.url)('electron'),env={};
    for(const key of ['SystemRoot','WINDIR','PATH','TEMP','TMP','USERPROFILE','APPDATA','LOCALAPPDATA','PROGRAMDATA','ComSpec','PATHEXT'])if(process.env[key]!==undefined)env[key]=process.env[key];
    const run=phase=>{const child=spawnSync(electron,[entry,phase],{env,encoding:'utf8',windowsHide:true,timeout:20000,maxBuffer:2<<20});
      assert.equal(child.status,0,'Synthetic Electron process did not exit cleanly');const lines=(child.stdout||'').split(/\r?\n/).filter(s=>s.startsWith('PUTY_PREFS_RESULT:'));assert.equal(lines.length,1,'Synthetic Electron receipt missing');
      const result=JSON.parse(lines[0].slice('PUTY_PREFS_RESULT:'.length));assert.notEqual(result.passed,false,'Synthetic Electron operation rejected');return result;};
    const first=run('create');assert.equal(first.ready,true);assert.equal(typeof first.asyncAvailable,'boolean');assert.equal(typeof first.syncAvailable,'boolean');
    t.diagnostic(JSON.stringify({electron:first.electron,ready:first.ready,asyncAvailable:first.asyncAvailable,syncAvailable:first.syncAvailable,asyncRoundtrip:first.asyncRoundtrip,syncRoundtrip:first.syncRoundtrip}));
    if(first.asyncAvailable!==true){assert.deepEqual(first.startup,{mode:'memory',warning:'protection'});assert.equal(first.blobExists,false);assert.equal(first.ramValue,'zh-CN');t.skip('Async OS provider unavailable: RAM preservation checked, encrypted reopen not exercised');return;}
    assert.equal(first.asyncRoundtrip,true);assert.deepEqual(first.startup,{mode:'persistent'});assert.equal(first.encrypted,true);
    if(first.syncAvailable)assert.equal(first.syncRoundtrip,true);
    const file=path.join(root,'shared','ui-preferences.puty'),original=await fs.readFile(file),damaged=Buffer.from(original);damaged[damaged.length-1]^=1;await fs.writeFile(file,damaged);
    const blocked=run('damaged');assert.deepEqual(blocked.startup,{mode:'memory',warning:'decrypt'});assert.equal(blocked.loadedLanguage,null);assert.equal(blocked.ramValue,'en-US');assert.equal(blocked.originalBytesPreserved,true);assert.deepEqual(await fs.readFile(file),damaged);
    await fs.writeFile(file,original);const fresh=run('recovered');assert.deepEqual(fresh.startup,{mode:'persistent'});assert.equal(fresh.loadedLanguage,'zh-CN');assert.equal(fresh.encrypted,true);
  }finally{
    assert.equal(path.dirname(base),await fs.realpath(tmpdir()));assert.ok(path.basename(base).startsWith('puty-prefs-electron-'));await fs.rm(base,{recursive:true,force:true});
  }
});
