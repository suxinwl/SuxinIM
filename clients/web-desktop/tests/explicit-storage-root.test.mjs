import test from 'node:test';
import assert from 'node:assert/strict';
import {build} from 'esbuild';
import {promises as fs} from 'node:fs';
import {randomUUID} from 'node:crypto';
import {tmpdir} from 'node:os';
import path from 'node:path';

// Only the directory chooser, online response and bootstrap *input* are mocked.
// Root validation, rememberRoot's atomic file write, key wrapping and vault I/O
// use the actual runtime and actual files in this test-owned temporary tree.
async function fixture(t,{configured=false,cancel=false}={}) {
  const directory=await fs.realpath(await fs.mkdtemp(path.join(tmpdir(),'puty-explicit-root-')));
  const selected=path.join(directory,'explicit-selection');await fs.mkdir(selected);
  const bootstrapDirectory=path.join(directory,'synthetic-bootstrap'),bootstrapPath=path.join(bootstrapDirectory,'storage.json');
  const unavailable=path.join(directory,'unavailable-configured-root');
  const key='__putyExplicitRoot_'+randomUUID().replaceAll('-',''),handlers=new Map(),dialogCalls=[],remembered=[];
  const account='emp_'+'e'.repeat(32),origin='https://im.szputy.com:9885';
  const previous=process.env.OPENIM38_TEST_DATA_DIR;process.env.OPENIM38_TEST_DATA_DIR=directory;
  const owner={id:1,isDestroyed:()=>false,send:()=>{},reload:()=>{}},event={sender:owner,senderFrame:{url:'openim38://app/index.html'}};
  globalThis[key]={bootstrapDirectory,bootstrapPath,legacyUserData:path.join(directory,'synthetic-old-profile'),configured:configured?unavailable:undefined,remembered,
    app:{getPath:()=>directory},BrowserWindow:{fromWebContents:()=>null,getFocusedWindow:()=>null},
    dialog:{showErrorBox:()=>{},showOpenDialog:async(_parent,options)=>{dialogCalls.push(options);return {canceled:cancel,filePaths:cancel?[]:[selected]};}},
    ipcMain:{handle:(name,fn)=>handlers.set(name,fn),on:()=>{}},
    net:{fetch:async url=>{assert.equal(url,origin+'/enterprise/v1/session/state');return Response.json({errCode:0,data:{userID:account,sessionID:'synthetic-explicit-root',epoch:1,platform:3,logicalClient:'pc',expiresAt:Math.floor(Date.now()/1000)+3600,loginAllowed:true,saveAllowed:true,effectivePolicy:{zone:'lan',logicalClient:'pc',login:true,preview:true,save:true,securityVersion:1,policyRevision:0,networkRevision:0,currentIP:'192.168.10.2',sources:{login:'synthetic-personal',save:'synthetic-personal'}}}});}},
    safeStorage:{isAsyncEncryptionAvailable:async()=>true,encryptStringAsync:async value=>Buffer.from('SYNTHETIC-OS-WRAP:'+value),decryptStringAsync:async bytes=>({result:bytes.toString().slice('SYNTHETIC-OS-WRAP:'.length),shouldReEncrypt:false})},shell:{}};
  let runtime;
  t.after(async()=>{
    runtime?.lockLocalStore('synthetic explicit-root test finished',false);
    if(previous===undefined)delete process.env.OPENIM38_TEST_DATA_DIR;else process.env.OPENIM38_TEST_DATA_DIR=previous;
    delete globalThis[key];
    assert.equal(path.dirname(directory),await fs.realpath(tmpdir()));assert.ok(path.basename(directory).startsWith('puty-explicit-root-'));
    await fs.rm(directory,{recursive:true,force:true});
  });
  const output=await build({entryPoints:['electron/main/localStore/runtime.ts'],bundle:true,write:false,platform:'node',format:'esm',plugins:[{name:'isolated-explicit-root',setup(builder){
    builder.onResolve({filter:/^electron$/},()=>({path:'electron',namespace:'fixture'}));
    builder.onResolve({filter:/^\.\/bootstrap$/},()=>({path:'bootstrap',namespace:'fixture'}));
    builder.onResolve({filter:/^\.\.\/storeManage$/},()=>({path:'preferences',namespace:'fixture'}));
    builder.onLoad({filter:/.*/,namespace:'fixture'},({path:name})=>({loader:'js',contents:name==='electron'
      ?'export const {app,BrowserWindow,dialog,ipcMain,net,safeStorage,shell}=globalThis['+JSON.stringify(key)+'];'
      :name==='bootstrap'?'const f=globalThis['+JSON.stringify(key)+'];export const {bootstrapDirectory,bootstrapPath,legacyUserData}=f;export const configuredRoot=()=>f.configured;export const validateStorageRoot=async root=>{await import("node:fs/promises").then(fs=>fs.access(root));return root;};export const validateStorageDirectory=validateStorageRoot;export const preserveBootstrapConfiguration=async()=>{};export const clearStartupStorageError=()=>{};'
      :'const f=globalThis['+JSON.stringify(key)+'];export const bindPreferencesRoot=async root=>{f.remembered.push(root);};export const initProtectedPreferences=async()=>({mode:"memory"});export const flushPreferences=async()=>{};'}));
  }}]});
  runtime=await import('data:text/javascript;base64,'+Buffer.from(output.outputFiles[0].text).toString('base64')+'#'+randomUUID());runtime.registerLocalStore();
  return {directory,selected,unavailable,bootstrapPath,dialogCalls,remembered,account,runtime,credentials:{origin,userID:account,imToken:'synthetic-im-token',chatToken:'synthetic-chat-token'},invoke:(op,data)=>handlers.get('puty-store')(event,op,data)};
}

test('first unlock accepts an explicitly selected real C directory and remembers only that canonical root',{skip:process.platform!=='win32'},async t=>{
  const f=await fixture(t);assert.match(f.directory,/^c:\\/i,'this regression must exercise a real owned C drive directory');
  const stats=await f.invoke('unlock',f.credentials),canonical=await fs.realpath(f.selected);
  assert.equal(f.dialogCalls.length,1);assert.deepEqual(f.dialogCalls[0].properties,['openDirectory','createDirectory']);
  assert.equal(f.dialogCalls[0].buttonLabel,'使用此目录');assert.match(f.dialogCalls[0].title,/建议选择非 C 盘/);
  assert.deepEqual(JSON.parse(await fs.readFile(f.bootstrapPath,'utf8')),{format:1,root:canonical});assert.deepEqual(f.remembered,[canonical]);
  assert.equal(stats.directory,path.join(canonical,'accounts',f.account,'vault'));
  const marker=JSON.parse(await fs.readFile(path.join(canonical,'.puty-local-root.json'),'utf8'));assert.equal(marker.format,'PUTY-LOCAL-ROOT-1');assert.match(marker.id,/^[a-f0-9-]{36}$/);
  const sealed=await fs.readFile(path.join(canonical,'accounts',f.account,'master.wrap'));assert.ok(sealed.toString().startsWith('SYNTHETIC-OS-WRAP:'));
  assert.equal(await fs.stat(path.join(f.directory,'encrypted-data')).then(()=>true,e=>{if(e.code==='ENOENT')return false;throw e;}),false);
});

test('an unavailable configured root fails without asking for or creating a C replacement',async t=>{
  const f=await fixture(t,{configured:true});await assert.rejects(f.invoke('unlock',f.credentials),{code:'ENOENT'});
  assert.equal(f.dialogCalls.length,0);assert.deepEqual(f.remembered,[]);await assert.rejects(f.runtime.activeVault(),/在线登录/);
  for(const missing of [f.unavailable,f.bootstrapPath,path.join(f.directory,'encrypted-data')])await assert.rejects(fs.access(missing),{code:'ENOENT'});
  assert.deepEqual(await fs.readdir(f.selected),[]);
});

test('cancelling first root selection never chooses a silent default',async t=>{
  const f=await fixture(t,{cancel:true});await assert.rejects(f.invoke('unlock',f.credentials),/请选择本地存储目录/);
  assert.equal(f.dialogCalls.length,1);assert.deepEqual(f.remembered,[]);assert.deepEqual(await fs.readdir(f.selected),[]);
  for(const missing of [f.bootstrapPath,path.join(f.directory,'encrypted-data')])await assert.rejects(fs.access(missing),{code:'ENOENT'});
});
