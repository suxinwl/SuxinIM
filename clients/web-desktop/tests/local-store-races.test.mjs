import test from 'node:test';
import assert from 'node:assert/strict';
import {build} from 'esbuild';
import {randomUUID} from 'node:crypto';
import {promises as fs} from 'node:fs';
import {mkdtemp,mkdir,writeFile,readdir,rm} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import path from 'node:path';

const load=async(entry,plugins=[])=>{const out=await build({entryPoints:[entry],bundle:true,write:false,platform:'node',format:'esm',plugins});return import('data:text/javascript;base64,'+Buffer.from(out.outputFiles[0].text).toString('base64')+'#'+randomUUID());};
const wait=async(check)=>{for(let i=0;i<500;i++){if(check())return;await new Promise(r=>setTimeout(r,2));}throw Error('Synthetic gate did not open');};
async function ownedTemporary(t,prefix){const dir=await fs.realpath(await mkdtemp(path.join(tmpdir(),prefix)));t.after(async()=>{assert.equal(path.dirname(dir),await fs.realpath(tmpdir()));assert.ok(path.basename(dir).startsWith(prefix));await rm(dir,{recursive:true,force:true});});return dir;}

test('post-checkpoint patch survives two reopens and a complete backup restored twice',async t=>{
  const {LocalVault}=await load('electron/main/localStore/vault.ts'),{restoreBackup}=await load('electron/main/localStore/backup.ts');
  const dir=await ownedTemporary(t,'puty-race-backup-'),source=path.join(dir,'source'),destination=path.join(dir,'restore');await mkdir(source);await mkdir(destination);
  const key=Buffer.alloc(32,29),account='test_independent_backup';let vault=await LocalVault.open(source,account,key,async()=>{}),restored;
  t.after(()=>{vault?.lock();restored?.lock();key.fill(0);});
  const when=Date.now();await vault.commit({opID:'base',statements:[],events:[{kind:'upsert',conversationID:'si_synthetic',row:{client_msg_id:'m1',send_time:when,content:'SYNTHETIC ONLY',status:2}}]});await vault.checkpoint(Buffer.from('synthetic checkpoint'));
  await vault.commit({opID:'patch',statements:[],events:[{kind:'upsert',conversationID:'si_synthetic',row:{client_msg_id:'m1',__patch:1,sender_nick_name:'synthetic change'}}]});
  for(let i=0;i<2;i++){vault.lock();vault=await LocalVault.open(source,account,key,async()=>{});const row=(await vault.history('si_synthetic'))[0];assert.equal(row.sender_nick_name,'synthetic change');assert.equal(row.send_time,when);}
  const saved=await vault.backup(dir,'Synthetic review recovery passphrase',async()=>{}),restoredKey=await restoreBackup(saved.directory,destination,account,'Synthetic review recovery passphrase',async()=>{});
  try{for(let i=0;i<2;i++){restored?.lock();restored=await LocalVault.open(destination,account,restoredKey,async()=>{});const row=(await restored.history('si_synthetic'))[0];assert.equal(row.content,'SYNTHETIC ONLY');assert.equal(row.sender_nick_name,'synthetic change');}}finally{restoredKey.fill(0);}
});

async function broker(t,{holdDecrypt=false}={}){
  const directory=await ownedTemporary(t,'puty-race-broker-'),base=path.join(directory,'encrypted-data');await mkdir(base);await writeFile(path.join(base,'.puty-local-root.json'),JSON.stringify({format:'PUTY-LOCAL-ROOT-1',id:randomUUID()}));
  const accountA='emp_'+'a'.repeat(32),accountB='emp_'+'b'.repeat(32),origin='https://im.szputy.com:9885';
  for(const id of [accountA,accountB]){const folder=path.join(base,'accounts',id);await mkdir(folder,{recursive:true});await writeFile(path.join(folder,'master.wrap'),Buffer.from(id+':'+Buffer.alloc(32,id===accountA?31:32).toString('base64')));}
  const prior=process.env.OPENIM38_TEST_DATA_DIR;process.env.OPENIM38_TEST_DATA_DIR=directory;
  const handlers=new Map(),globalKey='__putyRace_'+randomUUID().replaceAll('-',''),opened=[];let decryptRelease,decryptHeld=false,dialogRelease,dialogHeld=false;
  const owner={id:1,isDestroyed:()=>false,send:()=>{},reload:()=>{}},event={sender:owner,senderFrame:{url:'openim38://app/index.html'}};
  globalThis[globalKey]={app:{getPath:()=>directory},BrowserWindow:{fromWebContents:()=>null,getFocusedWindow:()=>null},dialog:{showErrorBox:()=>{},showOpenDialog:async()=>{dialogHeld=true;return new Promise(resolve=>dialogRelease=resolve);}},ipcMain:{handle:(name,fn)=>handlers.set(name,fn),on:()=>{}},net:{fetch:async(_url,options)=>Response.json({errCode:0,data:{userID:options.headers.token.startsWith('synthetic-a')?accountA:accountB,sessionID:options.headers.token,epoch:1,platform:3,logicalClient:'pc',expiresAt:Math.floor(Date.now()/1000)+3600,loginAllowed:true,saveAllowed:true,effectivePolicy:{zone:'lan',logicalClient:'pc',login:true,preview:true,save:true,securityVersion:1,policyRevision:0,networkRevision:0,currentIP:'192.168.10.2',sources:{login:'synthetic-personal',save:'synthetic-personal'}}}})},safeStorage:{isAsyncEncryptionAvailable:async()=>true,decryptStringAsync:async bytes=>{const value=bytes.toString();if(holdDecrypt&&value.startsWith(accountA+':')){decryptHeld=true;await new Promise(resolve=>decryptRelease=resolve);}return {result:value.slice(value.indexOf(':')+1),shouldReEncrypt:false};},encryptStringAsync:async value=>Buffer.from(value)},shell:{openPath:async value=>{opened.push(value);}}};
  const runtime=await load('electron/main/localStore/runtime.ts',[{name:'race-electron',setup(build){build.onResolve({filter:/^electron$/},()=>({path:'electron',namespace:'mock'}));build.onLoad({filter:/.*/,namespace:'mock'},()=>({contents:'export const {app,BrowserWindow,dialog,ipcMain,net,safeStorage,shell}=globalThis['+JSON.stringify(globalKey)+'];',loader:'js'}));}}]);runtime.registerLocalStore();
  t.after(()=>{decryptRelease?.();dialogRelease?.({canceled:true,filePaths:[]});runtime.lockLocalStore('synthetic race finished',false);if(prior===undefined)delete process.env.OPENIM38_TEST_DATA_DIR;else process.env.OPENIM38_TEST_DATA_DIR=prior;delete globalThis[globalKey];});
  return {directory,runtime,opened,accountB,invoke:(op,data)=>handlers.get('puty-store')(event,op,data),a:{origin,userID:accountA,imToken:'synthetic-a',chatToken:'synthetic'},b:{origin,userID:accountB,imToken:'synthetic-b',chatToken:'synthetic'},decryptHeld:()=>decryptHeld,releaseDecrypt:()=>decryptRelease(),dialogHeld:()=>dialogHeld,releaseDialog:value=>dialogRelease(value)};
}

test('late A OS-key decrypt cannot replace B account vault or lock B session',{timeout:15000},async t=>{
  const fixture=await broker(t,{holdDecrypt:true}),pending=fixture.invoke('unlock',fixture.a).then(()=>true,()=>false);await wait(fixture.decryptHeld);await fixture.invoke('unlock',fixture.b);fixture.releaseDecrypt();assert.equal(await pending,false);assert.ok((await fixture.invoke('stats')).directory.includes(fixture.accountB));
});

test('export chooser held across logout cannot write or open old-account plaintext',{timeout:15000},async t=>{
  const fixture=await broker(t);await fixture.invoke('unlock',fixture.a);await fixture.invoke('sync-state',{state:'ready'});const file=await fixture.invoke('stash-file',{name:'synthetic.txt',type:'text/plain',bytes:Buffer.from('SYNTHETIC PRIVATE EXPORT')});
  const destination=path.join(fixture.directory,'export');await mkdir(destination);const pending=fixture.invoke('export-file',{path:file.path}).then(()=>true,()=>false);await wait(fixture.dialogHeld);await fixture.invoke('lock');await fixture.invoke('unlock',fixture.b);fixture.releaseDialog({canceled:false,filePaths:[destination]});assert.equal(await pending,false);assert.deepEqual(await readdir(destination),[]);assert.deepEqual(fixture.opened,[]);assert.ok((await fixture.invoke('stats')).directory.includes(fixture.accountB));
});

test('restore transition rejects another unlock until the canceled chooser has settled',{timeout:15000},async t=>{
  const fixture=await broker(t);await fixture.invoke('unlock',fixture.a);const pending=fixture.invoke('restore-backup',{credentials:fixture.a,passphrase:'Synthetic only recovery passphrase'}).then(()=>true,()=>false);await wait(fixture.dialogHeld);await assert.rejects(fixture.invoke('unlock',fixture.b),/迁移|恢复/);fixture.releaseDialog({canceled:true,filePaths:[]});assert.equal(await pending,false);await fixture.invoke('unlock',fixture.b);assert.ok((await fixture.invoke('stats')).directory.includes(fixture.accountB));
});

test('same account new SID drains and reopens its vault while same SID refresh preserves it',{timeout:15000},async t=>{
  const fixture=await broker(t);await fixture.invoke('unlock',fixture.a);const previous=await fixture.runtime.activeVault();
  await previous.commit({opID:'same-user-base',statements:[],events:[{kind:'upsert',conversationID:'si_synthetic',row:{client_msg_id:'same-user-message',send_time:Date.now(),content:'SYNTHETIC SAME ACCOUNT',status:2}}]});
  let release,held=false,calls=0;const drain=previous.drain.bind(previous);previous.drain=async()=>{calls++;held=true;await new Promise(resolve=>release=resolve);return drain();};t.after(()=>release?.());
  const next={...fixture.a,imToken:'synthetic-a2'},pending=fixture.invoke('unlock',next);await wait(()=>held);
  await assert.rejects(fixture.runtime.activeVault(),/尚未打开|会话/);release();await pending;
  const reopened=await fixture.runtime.activeVault();assert.notEqual(reopened,previous);assert.equal(calls,1);assert.equal((await reopened.history('si_synthetic'))[0].content,'SYNTHETIC SAME ACCOUNT');
  const headers=await fixture.runtime.authenticatedResourceHeaders('a'.repeat(64));assert.equal(headers.token,'synthetic-a2');
  await fixture.invoke('unlock',next);assert.equal(await fixture.runtime.activeVault(),reopened);assert.equal(calls,1);
});

test('staged attachment rename failure rejects backup until recovery can finalize its exact ciphertext',async t=>{
  const {LocalVault}=await load('electron/main/localStore/vault.ts'),{restoreBackup}=await load('electron/main/localStore/backup.ts');
  const dir=await ownedTemporary(t,'puty-race-staged-'),source=path.join(dir,'source'),destination=path.join(dir,'restore');await mkdir(source);await mkdir(destination);
  const key=Buffer.alloc(32,41),account='test_staged_review';let vault=await LocalVault.open(source,account,key,async()=>{}),restored;const rename=fs.rename;let denied=true;
  t.after(()=>{fs.rename=rename;vault?.lock();restored?.lock();key.fill(0);});
  await vault.checkpoint(Buffer.from('synthetic checkpoint'));await vault.reserveAttachment({taskID:'synthetic',name:'staged.bin',messageRefs:[{conversationID:'si_test',clientMsgID:'m1'}]});
  fs.rename=async(from,to)=>{if(denied&&path.basename(String(to))==='staged.bin'&&String(from).includes('.part-'))throw Object.assign(Error('synthetic target rename refusal'),{code:'EACCES'});return rename(from,to);};
  try{
    await assert.rejects(vault.writeAttachment('synthetic',(async function*(){yield Buffer.from('SYNTHETIC STAGED FILE');})()),{code:'EACCES'});
    await assert.rejects(vault.backup(dir,'Synthetic staged recovery passphrase',async()=>{}),{code:'EACCES'});
    denied=false;const saved=await vault.backup(dir,'Synthetic staged recovery passphrase',async()=>{}),restoredKey=await restoreBackup(saved.directory,destination,account,'Synthetic staged recovery passphrase',async()=>{});
    try{restored=await LocalVault.open(destination,account,restoredKey,async()=>{});assert.equal((await restored.readAttachment('synthetic')).toString(),'SYNTHETIC STAGED FILE');}finally{restoredKey.fill(0);}
  }finally{fs.rename=rename;}
});

test('old sync-ready cannot succeed after logout during actual restored-copy collection',{timeout:15000},async t=>{
  const fixture=await broker(t);await fixture.invoke('unlock',fixture.a);const first=await fixture.runtime.activeVault();await first.checkpoint(Buffer.from('SYNTHETIC SDK CHECKPOINT'));await fixture.invoke('lock');
  const base=path.join(fixture.directory,'encrypted-data'),accountRoot=path.join(base,'accounts',fixture.a.userID),stage=path.join(base,'accounts','.account-'+randomUUID());await fs.cp(accountRoot,stage,{recursive:true});
  const transaction=await load('electron/main/localStore/restoreTransaction.ts');
  await transaction.installRestoredAccount(base,fixture.a.userID,stage,{check:async()=>{},protect:async value=>Buffer.from(value.toString('base64')),unprotect:async value=>Buffer.from(value.toString(),'base64')});
  await fixture.invoke('unlock',fixture.a);let held=false,release;const unlink=fs.unlink;
  fs.unlink=async file=>{await unlink(file);if(!held&&String(file).includes('.restore-rollback')){held=true;await new Promise(resolve=>release=resolve);}};
  try{
    const pending=fixture.invoke('sync-state',{state:'ready'}).then(()=>true,()=>false);await wait(()=>held);await fixture.invoke('lock');await fixture.invoke('unlock',fixture.b);release();assert.equal(await pending,false);assert.ok((await fixture.invoke('stats')).directory.includes(fixture.accountB));assert.equal((await fixture.invoke('stats')).restoreCleanupPending,false);
  }finally{release?.();fs.unlink=unlink;}
});

test('root migration waits for every account restore transaction before opening a chooser',async t=>{
  const fixture=await broker(t);await fixture.invoke('unlock',fixture.a);const metadata=path.join(fixture.directory,'encrypted-data','.restore-transactions');await fs.writeFile(path.join(metadata,fixture.accountB+'.journal'),'SYNTHETIC PENDING OTHER ACCOUNT');
  await assert.rejects(fixture.invoke('choose-directory'),/恢复事务尚待完成/);assert.equal(fixture.dialogHeld(),false);assert.ok(await fixture.runtime.activeVault());
});
