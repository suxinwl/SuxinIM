import test from 'node:test';
import assert from 'node:assert/strict';
import {build} from 'esbuild';
import {mkdtemp,mkdir,realpath,readFile,readdir,rename,rm,writeFile} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import path from 'node:path';
import {randomUUID} from 'node:crypto';

async function ownedDirectory(t){
  const parent=await realpath(tmpdir()),directory=await realpath(await mkdtemp(path.join(parent,'puty-backup-save-')));
  assert.ok(directory.startsWith(parent+path.sep));
  t.after(async()=>{assert.equal(await realpath(directory),directory);assert.ok(directory.startsWith(parent+path.sep));await rm(directory,{recursive:true,force:true});});
  return directory;
}
async function runtimeFixture(t){
  const directory=await ownedDirectory(t),destination=path.join(directory,'exports');await mkdir(destination);
  await mkdir(path.join(directory,'encrypted-data'));await writeFile(path.join(directory,'encrypted-data','.puty-local-root.json'),JSON.stringify({format:'PUTY-LOCAL-ROOT-1',id:'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa'}));
  const prior=process.env.OPENIM38_TEST_DATA_DIR;process.env.OPENIM38_TEST_DATA_DIR=directory;
  const handlers=new Map(),account='emp_'+'a'.repeat(32);let save=true;
  const owner={id:1,isDestroyed:()=>false,send:()=>{},reload:()=>{}},event={sender:owner,senderFrame:{url:'openim38://app/index.html'}};
  globalThis.__backupPolicyElectron={app:{getPath:()=>directory},BrowserWindow:{fromWebContents:()=>null,getFocusedWindow:()=>null},ipcMain:{handle:(name,fn)=>handlers.set(name,fn),on:()=>{}},shell:{},
    dialog:{showErrorBox:()=>{},showOpenDialog:async()=>{save=false;return {canceled:false,filePaths:[destination]};},showSaveDialog:async()=>{save=false;return {canceled:false,filePath:path.join(destination,'recovery.putykey')};}},
    net:{fetch:async()=>Response.json({errCode:0,data:{userID:account,sessionID:'same-owner',epoch:1,platform:3,logicalClient:'pc',expiresAt:Math.floor(Date.now()/1000)+3600,loginAllowed:true,saveAllowed:save,effectivePolicy:{zone:save?'lan':'internet',logicalClient:'pc',login:true,preview:true,save,securityVersion:1,policyRevision:0,networkRevision:0,currentIP:save?'192.168.10.2':'203.0.113.2',sources:{login:'synthetic',save:'synthetic'}}}})},
    safeStorage:{isAsyncEncryptionAvailable:async()=>true,encryptStringAsync:async value=>Buffer.from('synthetic-wrap:'+value),decryptStringAsync:async value=>({result:Buffer.from(value).toString().slice(15),shouldReEncrypt:false})}};
  const output=await build({entryPoints:['electron/main/localStore/runtime.ts'],bundle:true,write:false,platform:'node',format:'esm',plugins:[{name:'synthetic-backup-electron',setup(b){b.onResolve({filter:/^electron$/},()=>({path:'electron',namespace:'mock'}));b.onLoad({filter:/.*/,namespace:'mock'},()=>({contents:'export const {app,BrowserWindow,dialog,ipcMain,net,safeStorage,shell}=globalThis.__backupPolicyElectron;',loader:'js'}));}}]});
  const runtime=await import(`data:text/javascript;base64,${Buffer.from(output.outputFiles[0].text+'\n// '+randomUUID()).toString('base64')}`);runtime.registerLocalStore();
  t.after(()=>{runtime.lockLocalStore('test complete',false);if(prior===undefined)delete process.env.OPENIM38_TEST_DATA_DIR;else process.env.OPENIM38_TEST_DATA_DIR=prior;delete globalThis.__backupPolicyElectron;});
  const invoke=(operation,data)=>handlers.get('puty-store')(event,operation,data);
  await invoke('unlock',{origin:'https://im.szputy.com:9885',userID:account,imToken:'synthetic-im',chatToken:'synthetic-chat'});
  await invoke('sync-state',{state:'ready'});await invoke('sdk-checkpoint',{snapshot:Buffer.from('synthetic checkpoint').toString('base64')});
  return {destination,invoke};
}
for(const operation of ['export-backup','export-recovery'])test(`${operation} rechecks save after dialog when network zone changes without an epoch change`,async t=>{
  const {destination,invoke}=await runtimeFixture(t);
  await assert.rejects(invoke(operation,{passphrase:'Synthetic backup passphrase'}),/保存|下载/);
  assert.deepEqual(await readdir(destination),[]);
});

const compiled=await build({entryPoints:['electron/main/localStore/backup.ts','electron/main/localStore/vault.ts'],outdir:'unused',bundle:true,write:false,platform:'node',format:'esm'});
const modules=await Promise.all(compiled.outputFiles.map(f=>import(`data:text/javascript;base64,${Buffer.from(f.text).toString('base64')}`)));
const {createBackup,restoreBackup,writeRecoveryExport}=modules.find(v=>v.createBackup),{LocalVault}=modules.find(v=>v.LocalVault);
test('backup checks authorization inside a large file and leaves no partial export after rejection',async t=>{
  const root=await ownedDirectory(t),source=path.join(root,'source'),destination=path.join(root,'exports');await mkdir(source);await mkdir(destination);await writeFile(path.join(source,'large.bin'),Buffer.alloc(8*1024*1024,0x62));let checks=0;
  await assert.rejects(createBackup(source,'test_backup',Buffer.alloc(32,7),destination,['large.bin'],'Synthetic backup passphrase',async()=>{if(++checks===5)throw new Error('synthetic save denied');}),/save denied/);
  assert.deepEqual(await readdir(destination),[]);
});
test('LocalVault backup invokes its separate save gate even while login authorization remains valid',async t=>{
  const root=await ownedDirectory(t),source=path.join(root,'vault'),destination=path.join(root,'exports');await mkdir(destination);const vault=await LocalVault.open(source,'test_backup',Buffer.alloc(32,7),async()=>{});t.after(()=>vault.lock());await vault.checkpoint(Buffer.from('synthetic checkpoint'));let checks=0;
  await assert.rejects(vault.backup(destination,'Synthetic backup passphrase',async()=>{checks++;throw new Error('synthetic save denied');}),/save denied/);
  assert.equal(checks,1);assert.deepEqual(await readdir(destination),[]);
});
test('authorized chunked backup still restores complete bytes with the independent recovery passphrase',async t=>{
  const root=await ownedDirectory(t),source=path.join(root,'source'),destination=path.join(root,'exports'),restored=path.join(root,'restored');await mkdir(source);await mkdir(destination);await mkdir(restored);
  const body=Buffer.alloc(512*1024+17,0x81),key=Buffer.alloc(32,7);await writeFile(path.join(source,'synthetic-cipher.bin'),body);let checks=0;
  const result=await createBackup(source,'test_backup',key,destination,['synthetic-cipher.bin'],'Synthetic backup passphrase',async()=>{checks++;});
  assert.ok(checks>8);const restoredKey=await restoreBackup(result.directory,restored,'test_backup','Synthetic backup passphrase',async()=>{});
  try{assert.deepEqual(restoredKey,key);assert.deepEqual(await readFile(path.join(restored,'synthetic-cipher.bin')),body);}finally{restoredKey.fill(0);}
});
test('rejected backup cleanup preserves a foreign directory substituted at its allocated name',async t=>{
  const root=await ownedDirectory(t),source=path.join(root,'source'),destination=path.join(root,'exports');await mkdir(source);await mkdir(destination);await writeFile(path.join(source,'cipher.bin'),Buffer.from('synthetic ciphertext'));let checks=0,foreign;
  await assert.rejects(createBackup(source,'test_backup',Buffer.alloc(32,7),destination,['cipher.bin'],'Synthetic backup passphrase',async()=>{
    if(++checks!==2)return;foreign=path.join(destination,(await readdir(destination))[0]);const moved=path.join(destination,'owned-moved');assert.ok(foreign.startsWith(destination+path.sep));assert.ok(moved.startsWith(destination+path.sep));await rename(foreign,moved);await mkdir(foreign);await writeFile(path.join(foreign,'KEEP.txt'),'foreign file');throw new Error('synthetic save denied');
  }),/save denied/);
  assert.equal(await readFile(path.join(foreign,'KEEP.txt'),'utf8'),'foreign file');
});

test('recovery export removes only its new file when permission is revoked after durable writing',async t=>{
  const root=await ownedDirectory(t),selected=path.join(root,'recovery.putykey');await writeFile(selected,'existing user export');let checks=0;
  await assert.rejects(writeRecoveryExport(selected,Buffer.alloc(512,7),async()=>{if(++checks===4)throw new Error('synthetic save denied');}),/save denied/);
  assert.deepEqual(await readdir(root),['recovery.putykey']);assert.equal(await readFile(selected,'utf8'),'existing user export');
});

test('authorized recovery export preserves existing files and uses the full width duplicate suffix',async t=>{
  const root=await ownedDirectory(t),selected=path.join(root,'recovery.putykey');await writeFile(selected,'existing user export');
  const result=await writeRecoveryExport(selected,Buffer.from('synthetic encrypted recovery'),async()=>{});
  assert.equal(path.basename(result),'recovery（1）.putykey');assert.equal(await readFile(result,'utf8'),'synthetic encrypted recovery');assert.equal(await readFile(selected,'utf8'),'existing user export');
});
