import test from 'node:test';
import assert from 'node:assert/strict';
import {build} from 'esbuild';
import {mkdtemp,mkdir,rm,readFile,readdir,realpath,writeFile} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import path from 'node:path';

test('desktop broker binds online state, late responses, resource proof and archive readiness to the current session',async()=>{
  const directory=await realpath(await mkdtemp(path.join(tmpdir(),'puty-main-broker-')));await mkdir(path.join(directory,'encrypted-data'));await writeFile(path.join(directory,'encrypted-data','.puty-local-root.json'),JSON.stringify({format:'PUTY-LOCAL-ROOT-1',id:'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa'}));
  const priorEnv=process.env.OPENIM38_TEST_DATA_DIR;process.env.OPENIM38_TEST_DATA_DIR=directory;
  const handlers=new Map(),events=[],fetches=[],account='emp_'+ 'a'.repeat(32),origin='https://im.szputy.com:9885';let save=true,failure=false,delay,release;
  const owner={id:1,isDestroyed:()=>false,send:(...args)=>events.push(args),reload:()=>events.push(['reload'])},event={sender:owner,senderFrame:{url:'openim38://app/index.html'}};
  const mock={app:{getPath:()=>directory},BrowserWindow:{fromWebContents:()=>null,getFocusedWindow:()=>null},dialog:{showErrorBox:()=>{}},ipcMain:{handle:(name,fn)=>handlers.set(name,fn),on:()=>{}},net:{fetch:async(url,options)=>{
    fetches.push({url,options});
    if(url.endsWith('/session/state')){
      if(delay && options.headers.token==='old-token')await new Promise(resolve=>{release=resolve;});
      if(failure)return Response.json({errCode:403},{status:403});
      return Response.json({errCode:0,data:{userID:account,sessionID:options.headers.token==='old-token'?'old-session':'new-session',epoch:1,platform:3,logicalClient:'pc',expiresAt:Math.floor(Date.now()/1000)+3600,loginAllowed:true,saveAllowed:save,effectivePolicy:{zone:'lan',logicalClient:'pc',login:true,preview:true,save,securityVersion:1,policyRevision:0,networkRevision:0,currentIP:'192.168.10.2',sources:{login:'synthetic-personal',save:'synthetic-personal'}}}});
    }
    return new Response('SYNTHETIC PREVIEW',{headers:{'Content-Type':'image/png'}});
  }},safeStorage:{isAsyncEncryptionAvailable:async()=>true,encryptStringAsync:async value=>Buffer.from('synthetic-os-wrap:'+value),decryptStringAsync:async value=>({result:Buffer.from(value).toString().slice('synthetic-os-wrap:'.length),shouldReEncrypt:false})},shell:{}};
  globalThis.__putyElectronMock=mock;
  const output=await build({entryPoints:['electron/main/localStore/runtime.ts'],bundle:true,write:false,platform:'node',format:'esm',plugins:[{name:'synthetic-electron',setup(build){build.onResolve({filter:/^electron$/},()=>({path:'electron',namespace:'mock'}));build.onLoad({filter:/.*/,namespace:'mock'},()=>({contents:'export const {app,BrowserWindow,dialog,ipcMain,net,safeStorage,shell}=globalThis.__putyElectronMock;',loader:'js'}));}}]});
  const runtime=await import(`data:text/javascript;base64,${Buffer.from(output.outputFiles[0].text+'\n//# sourceURL=local-store-runtime-test.js').toString('base64')}`);runtime.registerLocalStore();
  const invoke=(operation,data)=>handlers.get('puty-store')(event,operation,data);
  try{
    const credentials={origin,userID:account,imToken:'old-token',chatToken:'synthetic-chat'};await invoke('unlock',credentials);
    await assert.rejects(runtime.readLocalRange('puty-local://pending/missing',0,1),/同步/);
    await invoke('sync-state',{state:'ready'});const pending=await invoke('stash-file',{name:'原名.txt',type:'text/plain',bytes:Buffer.from('SYNTHETIC PRIVATE CACHE')});
    assert.ok(pending.path.endsWith('/%E5%8E%9F%E5%90%8D.txt'));assert.equal((await invoke('file-info',{path:pending.path})).name,'原名.txt');
    const dataFile=(await readdir(path.join(directory,'encrypted-data','accounts',account,'vault','pending')))[0];assert.ok(!(await readFile(path.join(directory,'encrypted-data','accounts',account,'vault','pending',dataFile,'原名.txt'))).includes(Buffer.from('SYNTHETIC PRIVATE CACHE')));
    const proof='b'.repeat(64),preview=await invoke('remote-preview',{url:origin+'/enterprise/v1/attachments/read/synthetic',proof});const response=await runtime.remotePreviewResponse(new Request(preview.url));assert.equal(await response.text(),'SYNTHETIC PREVIEW');const previewFetch=fetches.at(-1);assert.equal(previewFetch.options.headers['X-Puty-Resource-Proof'],proof);assert.equal(previewFetch.options.headers.token,'old-token');assert.equal(previewFetch.options.redirect,'error');
    await assert.rejects(invoke('remote-preview',{url:'https://third.invalid/attachments/file',proof}),/Invalid resource/);
    delay=true;const stale=runtime.requireSavePermission();while(!release)await new Promise(resolve=>setTimeout(resolve,1));await invoke('lock');await invoke('unlock',{...credentials,imToken:'new-token'});await invoke('sync-state',{state:'ready'});release();await assert.rejects(stale,/会话已改变/);delay=false;
    assert.equal((await invoke('stats')).directory.includes(account),true);assert.ok(!events.some(args=>args[0]==='puty-store-locked' && String(args[1]).includes('授权不可用')));
    save=false;await assert.rejects(runtime.requireSavePermission(),/未开放/);const permitted=await invoke('remote-preview',{url:origin+'/enterprise/v1/attachments/read/synthetic',proof});assert.ok(permitted.url.startsWith('openim38://app/__remote/'));
    failure=true;await assert.rejects(invoke('sync-state',{state:'ready'}));await assert.rejects(invoke('stats'),/在线登录/);assert.ok(events.some(args=>args[0]==='puty-store-locked'));
  }finally{runtime.lockLocalStore('synthetic test finished',false);if(priorEnv===undefined)delete process.env.OPENIM38_TEST_DATA_DIR;else process.env.OPENIM38_TEST_DATA_DIR=priorEnv;delete globalThis.__putyElectronMock;await rm(directory,{recursive:true,force:true});}
});
