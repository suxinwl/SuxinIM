import test from 'node:test';
import assert from 'node:assert/strict';
import {build} from 'esbuild';
import {promises as fs} from 'node:fs';
import path from 'node:path';
import {tmpdir} from 'node:os';
import {spawnSync} from 'node:child_process';
import {createRequire} from 'node:module';

test('real Windows Electron singleton lock remains held while exact legacy business files are sealed and cleaned', {skip:process.platform!=='win32',timeout:45000}, async t=>{
 const base=await fs.realpath(await fs.mkdtemp(path.join(tmpdir(),'puty-legacy-native-lock-')));
 try{
  await build({entryPoints:['electron/main/localStore/legacyCoordinator.ts'],outfile:path.join(base,'coordinator.cjs'),bundle:true,platform:'node',target:'node22',format:'cjs',external:['electron']});
  const entry=path.join(base,'entry.cjs');await fs.writeFile(entry,`
const {app}=require('electron'),fs=require('node:fs'),fsp=fs.promises,path=require('node:path');
const source=path.join(__dirname,'legacy'),profile=path.join(__dirname,'os-profile'),root=path.join(__dirname,'encrypted');
for(const p of [source,profile,root])fs.mkdirSync(p,{recursive:true});
app.setPath('userData',source);const locked=app.requestSingleInstanceLock();app.setPath('userData',profile);app.disableHardwareAcceleration();
app.whenReady().then(async()=>{
 const api=require('./coordinator.cjs'),options={root,source,currentUserData:profile,writersStopped:true,SQL:{},assertCurrent:async()=>{if(!app.hasSingleInstanceLock())throw Error('LOCK_NOT_HELD');}};
 let snapshotID;if(process.argv[2]==='create'){
  await fsp.writeFile(path.join(source,'message.synthetic'),'SYNTHETIC_OLD_BODY');
  const saved=await api.preserveLegacyData({...options,readout:{source,databases:[],rawDatabases:[],cacheFiles:[],unresolved:[],deletionAuthorized:false}});snapshotID=saved.snapshotID;
  let nativeBusy=false;try{await fsp.unlink(path.join(source,'lockfile'));}catch(e){nativeBusy=e.code==='EBUSY';}if(!nativeBusy)throw Error('NATIVE_BUSY_NOT_REPRODUCED');let refusedWithoutProof=false;try{await api.cleanupLegacyPlaintext({...options,snapshotID});}catch(e){refusedWithoutProof=e.code==='EBUSY';}if(!refusedWithoutProof||!fs.existsSync(path.join(source,'message.synthetic')))throw Error('MISSING_PROOF_DID_NOT_PRESERVE_SOURCE');
 }else snapshotID=(await api.listLegacySnapshots(root))[0].snapshotID;
 const result=await api.cleanupLegacyPlaintext({...options,snapshotID,nativeSingletonLockHeld:()=>app.hasSingleInstanceLock()});
 const names=await fsp.readdir(source),metadata=await fsp.lstat(path.join(source,'lockfile'));
 if(!locked||!app.hasSingleInstanceLock()||!result.plaintextCleanupComplete||names.length!==1||names[0]!=='lockfile'||metadata.size!==0||!metadata.isFile())throw Error('NATIVE_CLEANUP_FAILED');
 console.log('PUTY_LEGACY_LOCK_RESULT:'+JSON.stringify({passed:true,phase:process.argv[2],lockHeld:true,businessFilesRemaining:0,nativeMetadataRemaining:1,complete:true}));app.quit();
}).catch(()=>{console.log('PUTY_LEGACY_LOCK_RESULT:'+JSON.stringify({passed:false,code:'SYNTHETIC_NATIVE_LOCK_FAILED'}));app.exit(1);});`);
  const env={...process.env};delete env.ELECTRON_RUN_AS_NODE;
  for(const phase of ['create','reopen']){
   const child=spawnSync(createRequire(import.meta.url)('electron'),[entry,phase],{env,windowsHide:true,encoding:'utf8',timeout:18000,maxBuffer:1<<20});
   assert.equal(child.status,0,'Isolated native lock process failed');const rows=(child.stdout||'').split(/\r?\n/).filter(line=>line.startsWith('PUTY_LEGACY_LOCK_RESULT:'));assert.equal(rows.length,1);const result=JSON.parse(rows[0].slice('PUTY_LEGACY_LOCK_RESULT:'.length));assert.equal(result.passed,true);assert.equal(result.businessFilesRemaining,0);assert.equal(result.lockHeld,true);t.diagnostic(JSON.stringify(result));
  }
 }finally{assert.equal(path.dirname(base),await fs.realpath(tmpdir()));assert.ok(path.basename(base).startsWith('puty-legacy-native-lock-'));await fs.rm(base,{recursive:true,force:true});}
});
