import test from 'node:test';
import assert from 'node:assert/strict';
import {build} from 'esbuild';
import {randomUUID} from 'node:crypto';
import {promises as fs} from 'node:fs';
import path from 'node:path';
import {tmpdir} from 'node:os';

async function fixture(t,{configuration=Buffer.from('{BROKEN SYNTHETIC JSON'),confirmed=true,online=false}={}){
  const directory=await fs.realpath(await fs.mkdtemp(path.join(tmpdir(),'puty-root-repair-'))),key='__repair_'+randomUUID().replaceAll('-','');
  const bootstrap=path.join(directory,'openim-enterprise-upgrade-bootstrap'),config=path.join(bootstrap,'storage.json'),selected=path.join(directory,'chosen');
  if(configuration==='selected')configuration=Buffer.from(JSON.stringify({format:1,root:selected}));
  await fs.mkdir(bootstrap);await fs.mkdir(selected);await fs.writeFile(config,configuration);
  const oldEnv=process.env.OPENIM38_TEST_DATA_DIR;delete process.env.OPENIM38_TEST_DATA_DIR;
  const handlers=new Map();let network=0,crypto=0;const owner={id:7,isDestroyed:()=>false,send:()=>{}};
  globalThis[key]={app:{getPath:()=>directory,whenReady:async()=>{}},BrowserWindow:{fromWebContents:()=>null,getFocusedWindow:()=>null},
    dialog:{showOpenDialog:async()=>({canceled:false,filePaths:[selected]}),showMessageBox:async()=>({response:confirmed?1:0})},
    ipcMain:{handle:(name,callback)=>handlers.set(name,callback),on:()=>{}},net:{fetch:async()=>{network++;if(!online)throw Error('NO NETWORK ALLOWED');return Response.json({errCode:0,data:{userID:'emp_'+'a'.repeat(32),sessionID:'synthetic-session',epoch:1,platform:3,logicalClient:'pc',expiresAt:Math.floor(Date.now()/1000)+3600,loginAllowed:true,saveAllowed:true,effectivePolicy:{zone:'lan',logicalClient:'pc',login:true,preview:true,save:true,securityVersion:1,policyRevision:0,networkRevision:0,currentIP:'192.168.10.2',sources:{login:'synthetic',save:'synthetic'}}}});}},
    safeStorage:{isAsyncEncryptionAvailable:async()=>online,encryptStringAsync:async()=>{crypto++;throw Error('NO KEY ACCESS ALLOWED');},decryptStringAsync:async()=>{crypto++;throw Error('NO KEY ACCESS ALLOWED');}},shell:{}};
  const output=await build({entryPoints:['electron/main/localStore/runtime.ts'],bundle:true,write:false,platform:'node',format:'esm',plugins:[{name:'owned-repair-app',setup(builder){builder.onResolve({filter:/^electron$/},()=>({path:'electron',namespace:'fixture'}));builder.onLoad({filter:/.*/,namespace:'fixture'},()=>({contents:'export const {app,BrowserWindow,dialog,ipcMain,net,safeStorage,shell}=globalThis['+JSON.stringify(key)+'];',loader:'js'}));}}]});
  const runtime=await import('data:text/javascript;base64,'+Buffer.from(output.outputFiles[0].text).toString('base64')+'#'+randomUUID());runtime.registerLocalStore();
  t.after(async()=>{runtime.lockLocalStore('owned synthetic test ended',false);delete globalThis[key];if(oldEnv===undefined)delete process.env.OPENIM38_TEST_DATA_DIR;else process.env.OPENIM38_TEST_DATA_DIR=oldEnv;assert.equal(path.dirname(directory),await fs.realpath(tmpdir()));assert.ok(path.basename(directory).startsWith('puty-root-repair-'));await fs.rm(directory,{recursive:true,force:true});});
  const event={sender:owner,senderFrame:{url:'openim38://app/index.html'}};
  return {directory,bootstrap,config,configuration,selected,invoke:(operation,data,frame)=>handlers.get('puty-store')(frame?{...event,senderFrame:{url:frame}}:event,operation,data),network:()=>network,crypto:()=>crypto};
}

test('offline explicit repair preserves malformed original configuration before selecting a new empty root',async t=>{
  const f=await fixture(t),result=await f.invoke('repair-directory');assert.equal(result.changed,true);assert.equal(result.preferencesPersistent,false);
  assert.deepEqual(JSON.parse(await fs.readFile(f.config,'utf8')),{format:1,root:f.selected});
  const backups=(await fs.readdir(f.bootstrap)).filter(value=>value.startsWith('storage.before-repair-'));assert.equal(backups.length,1);assert.deepEqual(await fs.readFile(path.join(f.bootstrap,backups[0])),f.configuration);
  assert.deepEqual(await fs.readdir(f.selected),['.puty-local-root.json']);assert.equal(f.network(),0);assert.equal(f.crypto(),0);
  await assert.rejects(f.invoke('history',{conversationID:'synthetic'}),/在线登录/);
});

test('reselecting an existing owned root preserves every ciphertext and wrapped key without opening them',async t=>{
  const f=await fixture(t),account=path.join(f.selected,'accounts','emp_'+'a'.repeat(32));
  await fs.writeFile(path.join(f.selected,'.puty-local-root.json'),JSON.stringify({format:'PUTY-LOCAL-ROOT-1',id:randomUUID()}));await fs.mkdir(path.join(account,'vault'),{recursive:true});
  const key=Buffer.from('SYNTHETIC UNREADABLE WRAPPED KEY'),data=Buffer.from('SYNTHETIC EXISTING CIPHERTEXT');await fs.writeFile(path.join(account,'master.wrap'),key);await fs.writeFile(path.join(account,'vault','synthetic.puty'),data);
  assert.equal((await f.invoke('repair-directory')).changed,true);assert.deepEqual(await fs.readFile(path.join(account,'master.wrap')),key);assert.deepEqual(await fs.readFile(path.join(account,'vault','synthetic.puty')),data);assert.equal(f.crypto(),0);assert.equal(f.network(),0);
});

test('a nonempty directory missing its ownership marker is rejected without claiming or overwriting existing data',async t=>{
  const f=await fixture(t);await fs.writeFile(path.join(f.selected,'preserve.cipher'),Buffer.from('SYNTHETIC CIPHER'));
  await assert.rejects(f.invoke('repair-directory'),/空目录/);assert.deepEqual(await fs.readdir(f.selected),['preserve.cipher']);assert.deepEqual(await fs.readFile(f.config),f.configuration);assert.equal((await fs.readdir(f.bootstrap)).length,1);assert.equal(f.network(),0);
});

test('cancel keeps the previous configuration, and an untrusted window cannot start directory repair',async t=>{
  const f=await fixture(t,{confirmed:false});assert.deepEqual(await f.invoke('repair-directory'),{changed:false});assert.deepEqual(await fs.readFile(f.config),f.configuration);assert.equal((await fs.readdir(f.bootstrap)).length,1);assert.equal(f.crypto(),0);assert.equal(f.network(),0);
  await assert.rejects(f.invoke('repair-directory',undefined,'https://untrusted.invalid/'),/Untrusted/);
});

test('a failed actual directory write probe keeps original configuration and removes its exact partial probe',async t=>{
  const f=await fixture(t),original=fs.open;
  fs.open=async(file,...args)=>{const handle=await original(file,...args);if(String(file).includes('.write-check-'))return {writeFile:async()=>{throw Object.assign(Error('synthetic full device'),{code:'ENOSPC'});},close:()=>handle.close()};return handle;};
  try{await assert.rejects(f.invoke('repair-directory'),{code:'ENOSPC'});assert.deepEqual(await fs.readFile(f.config),f.configuration);assert.deepEqual(await fs.readdir(f.selected),['.puty-local-root.json']);assert.equal((await fs.readdir(f.bootstrap)).length,1);}finally{fs.open=original;}
});

test('a linked selected root or invalid ownership marker cannot redirect storage repair',async t=>{
  const f=await fixture(t),target=path.join(f.directory,'target');await fs.mkdir(target);await fs.rmdir(f.selected);await fs.symlink(target,f.selected,process.platform==='win32'?'junction':'dir');
  await assert.rejects(f.invoke('repair-directory'),/链接/);assert.deepEqual(await fs.readdir(target),[]);await fs.unlink(f.selected);await fs.mkdir(f.selected);await fs.writeFile(path.join(f.selected,'.puty-local-root.json'),JSON.stringify({format:'PUTY-LOCAL-ROOT-1',id:'-'.repeat(36)}));
  await assert.rejects(f.invoke('repair-directory'),/标记无效/);assert.deepEqual(await fs.readFile(f.config),f.configuration);assert.equal(f.crypto(),0);assert.equal(f.network(),0);
});

test('an unreadable existing vault never becomes an empty account or receives a replacement master key',async t=>{
  const f=await fixture(t,{configuration:'selected',online:true}),userID='emp_'+'a'.repeat(32),account=path.join(f.selected,'accounts',userID),vault=path.join(account,'vault');
  await fs.writeFile(path.join(f.selected,'.puty-local-root.json'),JSON.stringify({format:'PUTY-LOCAL-ROOT-1',id:randomUUID()}));await fs.mkdir(vault,{recursive:true});const payload=Buffer.from('SYNTHETIC CIPHER MUST SURVIVE');await fs.writeFile(path.join(vault,'message.puty'),payload);
  const original=fs.readdir;fs.readdir=async(directory,...args)=>{if(directory===vault)throw Object.assign(Error('synthetic access denied'),{code:'EACCES'});return original(directory,...args);};
  try{await assert.rejects(f.invoke('unlock',{origin:'https://im.szputy.com:9885',userID,imToken:'synthetic-only',chatToken:'synthetic-only'}),{code:'EACCES'});}finally{fs.readdir=original;}
  await assert.rejects(fs.access(path.join(account,'master.wrap')),{code:'ENOENT'});assert.deepEqual(await fs.readFile(path.join(vault,'message.puty')),payload);assert.equal(f.crypto(),0);assert.equal(f.network(),1);
});
