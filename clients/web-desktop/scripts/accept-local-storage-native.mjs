// Real Electron + pinned Go WASM/SQLite worker; all account/media responses are
// synthetic and intercepted in memory. No employee credentials or server writes.
import {_electron} from 'playwright';
import fs from 'node:fs/promises';
import path from 'node:path';
import os from 'node:os';
import {randomUUID,createHash} from 'node:crypto';
import assert from 'node:assert/strict';
const client=path.resolve(import.meta.dirname,'..'),temporary=await fs.mkdtemp(path.join(os.tmpdir(),'puty-native-store-'));
const profile=await fs.realpath(temporary),root=path.join(profile,'encrypted-data'),account='emp_'+'c'.repeat(32),origin='https://im.szputy.com:9885';
await fs.mkdir(root);await fs.writeFile(path.join(root,'.puty-local-root.json'),JSON.stringify({format:'PUTY-LOCAL-ROOT-1',id:randomUUID()}));
const evidence=path.join(client,'build/acceptance','local-storage-native-'+Date.now());await fs.mkdir(evidence,{recursive:true});
const report={syntheticOnly:true,normalMainSSO:false,goCoreAuthenticationUsed:false,productionCredentialsRead:0,productionBusinessRequests:0,realElectron:true,realSDKWorker:true,outboundNetworkBlocked:true,syntheticRequestRuns:[],cases:[]};
const packaged=process.env.PUTY_TEST_PACKAGED==='1';
const executable=path.join(client,packaged?'release/win-unpacked/PUTY_IM.exe':'node_modules/electron/dist/electron.exe');
report.packagedApplication=packaged;
if(packaged){const bytes=await fs.readFile(path.join(client,'release/win-unpacked/resources/app.asar'));report.packagedAsar={bytes:bytes.length,sha256:createHash('sha256').update(bytes).digest('hex')};}
report.runtimeFiles={};
for(const relative of ['dist-electron/main/index.js','dist/index.html','dist/openIM.wasm','dist/sql-wasm.wasm','dist/wasm_exec.js']){const bytes=await fs.readFile(path.join(client,relative));report.runtimeFiles[relative]={bytes:bytes.length,sha256:createHash('sha256').update(bytes).digest('hex')};}
let app,page,stage='launch';
const check=(name)=>report.cases.push({name,passed:true});
async function invoke(operation,data){return page.evaluate(([operation,data])=>window.electronAPI.ipcInvoke('puty-store',operation,data),[operation,data]);}
async function database(method,args=[]){const result=await page.evaluate(async([method,args])=>{const value=await window[method](...args);return typeof value==='string'?JSON.parse(value):value;},[method,args]);assert.equal(result.errCode || 0,0);if(!Object.hasOwn(result,'data'))return result;return typeof result.data==='string'?JSON.parse(result.data || 'null'):result.data;}
async function launchSynthetic(sid='synthetic-session-1'){
  // The real app/worker run normally, but Chromium cannot contact production,
  // even during startup before the test installs its in-memory net.fetch shim.
  app=await _electron.launch({executablePath:executable,args:[...(packaged?[]:['.']),'--proxy-server=http://127.0.0.1:9','--proxy-bypass-list=<-loopback>'],cwd:client,env:{...process.env,OPENIM38_TEST_DATA_DIR:profile,OPENIM38_SMOKE_TEST:'1'},timeout:60000});
  for(let n=0;n<200;n++){page=app.windows().find(value=>value.url().startsWith('openim38://app/'));if(page)break;await new Promise(resolve=>setTimeout(resolve,100));}
  assert.ok(page);await page.context().route(/^https?:\/\//,route=>route.abort());
  await page.waitForFunction(()=>typeof window.initDB==='function' && typeof window.putyLocalCheckpoint==='function',undefined,{timeout:60000});
  stage='login-ready';
  // The initial '/' mounts the anonymous account guard. Its clearIMProfile()
  // legitimately locks the native vault while navigation to Login completes.
  // Worker globals alone are too early to take ownership of that same vault.
  await page.waitForURL(/#\/login(?:\?|$)/,{timeout:30000});
  await page.locator('input[autocomplete="username"]').waitFor({state:'visible',timeout:30000});
  await page.locator('input[autocomplete="current-password"]').waitFor({state:'visible',timeout:30000});
  await page.evaluate(()=>new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve))));
  await invoke('lock');
  await app.evaluate(({net},{account,sid})=>{
    globalThis.__putyNativeStorePolicy={save:true,offline:false,sid,account,requests:{session:0,attachmentHead:0,attachmentRead:0,blocked:0}};
    net.fetch=async(url,options={})=>{
      const policy=globalThis.__putyNativeStorePolicy;
      if(String(url).endsWith('/enterprise/v1/session/state')){
        policy.requests.session++;
        if(policy.offline)throw new Error('synthetic offline');
        return Response.json({errCode:0,data:{userID:policy.account,sessionID:policy.sid,epoch:1,platform:3,logicalClient:'pc',expiresAt:Math.floor(Date.now()/1000)+3600,loginAllowed:true,saveAllowed:policy.save,effectivePolicy:{zone:'lan',logicalClient:'pc',login:true,preview:true,save:policy.save,securityVersion:1,policyRevision:0,networkRevision:0,currentIP:'192.168.10.2',sources:{login:'synthetic-personal',save:'synthetic-personal'}}}});
      }
      if(String(url).includes('/attachments/synthetic-local-store/')){
        policy.requests[options.method==='HEAD'?'attachmentHead':'attachmentRead']++;
        const bytes=Buffer.from('SYNTHETIC ATTACHMENT PRIVATE CONTENT');
        return new Response(options.method==='HEAD'?null:bytes,{headers:{'Content-Type':'application/octet-stream','Content-Length':String(bytes.length),ETag:'"synthetic-v1"'}});
      }
      policy.requests.blocked++;throw new Error('synthetic test blocked an unrecognized network request');
    };
  },{account,sid});
}
async function closeSyntheticRun(){
  report.syntheticRequestRuns.push(await app.evaluate(()=>({...globalThis.__putyNativeStorePolicy.requests})));
  const exiting=app,childProcess=exiting.process();
  const exited=childProcess.exitCode===null?new Promise(resolve=>{const timer=setTimeout(()=>resolve(null),10000);childProcess.once('exit',code=>{clearTimeout(timer);resolve(code);});}):Promise.resolve(childProcess.exitCode);
  await exiting.evaluate(({app})=>app.exit(0)).catch(()=>{});await exiting.close().catch(()=>{});
  assert.equal(await exited,0,'synthetic Electron must fully exit before restart');app=undefined;page=undefined;
}
try{
  await launchSynthetic();
  const credentials={origin,userID:account,imToken:'synthetic-im-token',chatToken:'synthetic-chat-token'};
  stage='unlock';await invoke('unlock',credentials);await assert.rejects(invoke('history',{conversationID:'si_fixture_a'}));await invoke('sync-state',{state:'ready'});
  stage='real-worker';await database('initDB',[account,'']);
  const when=Date.parse('2026-11-03T16:01:00Z'),old=Date.now()-120*86400000;
  const row=(id,time)=>({clientMsgID:id,sendTime:time,status:2,content:'SYNTHETIC NATIVE PRIVATE MESSAGE '+id,localEx:'',seq:3,isRead:true,contentType:101,sendID:account});
  await database('batchInsertMessageList',['si_fixture_a',JSON.stringify([row('native-old',old),row('native-a',when)])]);
  await database('batchInsertMessageList',['si_fixture_b',JSON.stringify([row('native-b',when)])]);
  const history=await database('getMessageList',['si_fixture_a',20,0,0,'',false]);assert.equal(history.length,2);assert.ok(history.every(item=>item.content.includes('SYNTHETIC NATIVE')));await database('putyLocalCheckpoint');check('real worker commits encrypted SQL and reads older monthly payload');
  stage='native-download';
  const download=async(taskID,msgID,conversationID)=>page.evaluate(async data=>{
    return new Promise((resolve,reject)=>{const timer=setTimeout(()=>{offSuccess();offFailure();reject(new Error('synthetic download timeout'));},15000);
      const offSuccess=window.electronAPI.subscribe('downloadSuccess',(url,file)=>{if(url!==data.url)return;clearTimeout(timer);offSuccess();offFailure();resolve(file);});
      const offFailure=window.electronAPI.subscribe('downloadFailed',(url)=>{if(url!==data.url)return;clearTimeout(timer);offSuccess();offFailure();reject(new Error('synthetic download failed'));});
      window.electronAPI.ipcInvoke('startDownload',data).catch(error=>{clearTimeout(timer);offSuccess();offFailure();reject(error);});
    });
  },{url:origin+'/attachments/synthetic-local-store/'+taskID,taskID,clientMsgID:msgID,conversationID,fileName:'职员名单.xlsx',resourceProof:'a'.repeat(64)});
  const first=await download('synthetic-native-a','native-a','si_fixture_a'),second=await download('synthetic-native-b','native-b','si_fixture_b');
  assert.equal(path.basename(first),'职员名单.xlsx');assert.equal(path.basename(second),'职员名单（1）.xlsx');
  assert.equal((await fs.readFile(first)).subarray(0,8).toString(),'PUTYLS1\n');assert.ok(!(await fs.readFile(first)).includes(Buffer.from('SYNTHETIC ATTACHMENT PRIVATE CONTENT')));
  const opened=await invoke('file-info',{path:first});assert.equal(Buffer.from(opened.bytes).toString(),'SYNTHETIC ATTACHMENT PRIVATE CONTENT');check('native download uses original full-width collision names and ciphertext');
  await database('batchInsertMessageList',['si_fixture_a',JSON.stringify([{...row('private-working-only',when),attachedInfo:JSON.stringify({isPrivateChat:true,burnDuration:30})}])]);
  const privateFile=await download('synthetic-native-private','private-working-only','si_fixture_a');assert.equal(path.basename(privateFile),'职员名单（2）.xlsx');assert.equal((await invoke('history',{conversationID:'si_fixture_a'})).length,2);
  stage='storage-settings-ui';
  await page.evaluate(()=>{location.hash='/third/personal-settings';});
  await page.getByText('本地存储 / 清理聊天记录',{exact:true}).click();
  await page.getByText(root,{exact:true}).waitFor({state:'visible'});
  await page.getByRole('button',{name:'计算清理范围',exact:true}).click();
  await page.getByText(/将清理 4 条消息、3 个附件/).waitFor({state:'visible'});
  await page.screenshot({path:path.join(evidence,'storage-settings.png')});
  await page.getByRole('button',{name:'关闭',exact:true}).click();check('actual settings UI displays selected data root and counts hot, cold and private references');
  stage='settings-confirmation-invalidation';
  await page.getByText('本地存储 / 清理聊天记录',{exact:true}).click();
  await page.evaluate(()=>{
    const actual=window.putyLocalCleanup;
    window.__putyNativeCleanupObservation={calls:0,actual};
    // Count the real SDK cleanup entry point without replacing its behavior.
    window.putyLocalCleanup=(...args)=>{window.__putyNativeCleanupObservation.calls++;return actual(...args);};
  });
  const confirmation=page.locator('.ant-modal-confirm').filter({hasText:'确认清理当前客户端？'});
  const calculateConfirmation=async()=>{
    await page.getByRole('button',{name:'计算清理范围',exact:true}).click();
    const preview=page.locator('.ant-alert').filter({hasText:/将清理 \d+ 条消息、\d+ 个附件/});
    await preview.getByRole('button',{name:'确认清理',exact:true}).click();
    await confirmation.waitFor({state:'visible'});
  };
  try{
    await calculateConfirmation();
    const date=page.getByPlaceholder('留空表示全部日期',{exact:true});
    // A real controlled DatePicker change invalidates the displayed confirmation.
    // Filling an input remains a DOM/user-input action; no React handler is called.
    await date.fill('2026-11-04');await date.press('Enter');
    await confirmation.waitFor({state:'hidden'});
    assert.equal(await page.evaluate(()=>window.__putyNativeCleanupObservation.calls),0);
    await page.locator('.ant-picker').hover();await page.locator('.ant-picker-clear').click();
    await calculateConfirmation();
    await database('putyLocalCheckpoint');await database('closeDB');await invoke('lock');
    // This script has no authenticated Main UI. Use the product's normal session
    // transition notification with a synthetic native SID change, explicitly recorded.
    await page.evaluate(()=>window.dispatchEvent(new Event('im-profile-changing')));
    await confirmation.waitFor({state:'hidden'});
    assert.equal(await page.evaluate(()=>window.__putyNativeCleanupObservation.calls),0);
    await app.evaluate(()=>{globalThis.__putyNativeStorePolicy.sid='synthetic-session-settings-2';});
    await invoke('unlock',credentials);await invoke('sync-state',{state:'ready'});await database('initDB',[account,'']);
    await page.evaluate(()=>window.dispatchEvent(new Event('im-profile-changed')));
    assert.equal((await invoke('history',{conversationID:'si_fixture_a'})).length,2);
    assert.equal((await invoke('history',{conversationID:'si_fixture_b'})).length,1);
    await fs.access(first);await fs.access(second);await fs.access(privateFile);
    report.settingsConfirmation={rangeChangeDestroyed:true,sessionChangeDestroyed:true,cleanCalls:0,syntheticSessionNotification:true};
  }finally{
    await page.evaluate(()=>{if(window.__putyNativeCleanupObservation){window.putyLocalCleanup=window.__putyNativeCleanupObservation.actual;delete window.__putyNativeCleanupObservation;}});
  }
  await page.getByRole('button',{name:'关闭',exact:true}).click();
  check('actual settings confirmations are destroyed by scope and synthetic session changes without any cleanup call');
  stage='exact-worker-delete';
  const deletedID='native-delete-exact',deletedWhen=when+3600000;
  await database('batchInsertMessageList',['si_fixture_a',JSON.stringify([row(deletedID,deletedWhen)])]);
  const deletedFile=await download('synthetic-native-delete',deletedID,'si_fixture_a');
  assert.equal(path.basename(deletedFile),'职员名单（3）.xlsx');
  assert.equal((await invoke('history',{conversationID:'si_fixture_a'})).length,3);
  // The real SDK database API used by Go calls the real SQL.js worker. This is
  // intentionally NOT a fabricated host sdk-commit or a claimed Go core login.
  await database('deleteConversationMsgs',['si_fixture_a',JSON.stringify([deletedID])]);
  await assert.rejects(invoke('file-info',{path:deletedFile}));await database('putyLocalCheckpoint');await assert.rejects(fs.access(deletedFile));
  assert.equal((await invoke('history',{conversationID:'si_fixture_a'})).length,2);
  assert.equal((await invoke('history',{conversationID:'si_fixture_b'})).length,1);await fs.access(first);await fs.access(second);
  const mask=await invoke('cleanup-policy');assert.ok(mask.some(rule=>rule.conversationIDs?.includes('si_fixture_a')&&rule.messageIDs?.includes(deletedID)));
  assert.ok(!mask.some(rule=>rule.conversationIDs?.includes('si_fixture_a')&&!rule.messageIDs));
  await database('closeDB');await invoke('lock');
  stage='exact-delete-process-restart';await closeSyntheticRun();
  if(packaged){const bytes=await fs.readFile(path.join(client,'release/win-unpacked/resources/app.asar'));assert.equal(createHash('sha256').update(bytes).digest('hex'),report.packagedAsar.sha256);}
  await launchSynthetic('synthetic-session-exact-restart');
  await invoke('unlock',credentials);await invoke('sync-state',{state:'ready'});await database('initDB',[account,'']);
  stage='exact-delete-late-sync';
  await database('batchInsertMessageList',['si_fixture_a',JSON.stringify([row(deletedID,deletedWhen)])]);
  const afterDelete=await database('getMessageList',['si_fixture_a',20,0,0,'',false]);
  assert.ok(!afterDelete.some(item=>(item.clientMsgID??item.client_msg_id)===deletedID));
  assert.ok(['native-old','native-a'].every(id=>afterDelete.some(item=>(item.clientMsgID??item.client_msg_id)===id)));
  const otherHistory=await database('getMessageList',['si_fixture_b',20,0,0,'',false]);
  assert.equal(otherHistory.length,1);assert.equal(otherHistory[0].clientMsgID??otherHistory[0].client_msg_id,'native-b');
  assert.equal((await invoke('history',{conversationID:'si_fixture_a'})).length,2);await assert.rejects(invoke('file-info',{path:deletedFile}));await fs.access(first);await fs.access(second);
  assert.equal(Buffer.from((await invoke('file-info',{path:second})).bytes).toString(),'SYNTHETIC ATTACHMENT PRIVATE CONTENT');
  const unchangedCounts=await database('putyLocalCleanupPreview',[{}]);assert.equal(unchangedCounts.messages,4);assert.equal(unchangedCounts.attachments,3);
  await database('putyLocalCheckpoint');
  report.exactDelete={entryPoint:'real SDK worker deleteConversationMsgs',normalGoCoreLogin:false,processRestarts:1,lateSameIDSuppressed:true,otherConversationPreserved:true,rangeTombstoneUsed:false,temporaryExtraMessages:1,temporaryExtraAttachments:1,remainingOriginalCounts:{messages:unchangedCounts.messages,attachments:unchangedCounts.attachments}};
  check('real SDK worker single-message delete removes its attachment and survives an Electron process restart and same-ID late sync without affecting fixture B');
  stage='permission';await app.evaluate(()=>{globalThis.__putyNativeStorePolicy.save=false;});await assert.rejects(page.evaluate(data=>window.electronAPI.ipcInvoke('startDownload',data),{url:origin+'/attachments/synthetic-local-store/blocked',resourceProof:'a'.repeat(64)}));
  const preview=await invoke('remote-preview',{url:origin+'/attachments/synthetic-local-store/preview',proof:'a'.repeat(64)});const previewed=await page.evaluate(async url=>(await fetch(url)).text(),preview.url);assert.equal(previewed,'SYNTHETIC ATTACHMENT PRIVATE CONTENT');check('save denial retains protected preview');
  stage='cleanup';const previewCount=await database('putyLocalCleanupPreview',[{conversationIDs:['si_fixture_a'],beforeMS:when+1}]);assert.equal(previewCount.messages,3);assert.equal(previewCount.attachments,2);assert.ok(previewCount.bytes>0);
  await database('putyLocalCleanup',[{conversationIDs:['si_fixture_a'],beforeMS:when+1}]);assert.equal((await invoke('history',{conversationID:'si_fixture_a'})).length,0);await assert.rejects(fs.access(first));await assert.rejects(fs.access(privateFile));await fs.access(second);
  await database('batchInsertMessageList',['si_fixture_a',JSON.stringify([row('native-late',when)])]);assert.equal((await invoke('history',{conversationID:'si_fixture_a'})).length,0);check('local cleanup removes only selected refs and blocks resync resurrection');
  stage='reopen';await database('closeDB');await invoke('lock');await assert.rejects(invoke('file-info',{path:second}));await app.evaluate(()=>{globalThis.__putyNativeStorePolicy.sid='synthetic-session-2';globalThis.__putyNativeStorePolicy.save=true;});await invoke('unlock',credentials);await invoke('sync-state',{state:'ready'});await database('initDB',[account,'']);assert.equal((await database('getMessageList',['si_fixture_a',20,0,0,'',false])).length,0);assert.equal((await database('getMessageList',['si_fixture_b',20,0,0,'',false])).length,1);check('new online SID reopens existing OS-wrapped key and durable clear markers');
  stage='protected-child';
  const openChild=()=>page.evaluate(()=>window.electronAPI.ipcInvoke('openChildWindow',{arg:'third/about',key:'synthetic-protected-preview',options:{width:200,height:150,opacity:0,focusable:false,skipTaskbar:true}}));
  await openChild();assert.equal(await app.evaluate(({BrowserWindow})=>BrowserWindow.getAllWindows().length),2);
  await invoke('sync-state',{state:'syncing'});assert.equal(await app.evaluate(({BrowserWindow})=>BrowserWindow.getAllWindows().length),1);
  await invoke('sync-state',{state:'ready'});await openChild();assert.equal(await app.evaluate(({BrowserWindow})=>BrowserWindow.getAllWindows().length),2);
  stage='offline';await app.evaluate(()=>{globalThis.__putyNativeStorePolicy.offline=true;});await assert.rejects(invoke('sync-state',{state:'ready'}));await assert.rejects(invoke('file-info',{path:second}));check('offline lease failure closes archive access');
  assert.equal(await app.evaluate(({BrowserWindow})=>BrowserWindow.getAllWindows().length),1);check('sync and revocation close protected child windows and allow a fresh preview after resync');
  report.syntheticRequests=await app.evaluate(()=>({...globalThis.__putyNativeStorePolicy.requests}));
  report.syntheticRequestRuns.push(report.syntheticRequests);
  report.electron=await app.evaluate(()=>process.versions.electron);
  report.passed=true;
}catch(error){report.passed=false;report.failureStage=stage;report.failureType=error.name;report.failureMessage=String(error.message).slice(0,350);process.exitCode=1;}
finally{
  if(app){await app.evaluate(({app})=>app.exit(0)).catch(()=>{});await app.close().catch(()=>{});}
  report.profileOutsideProduction=true;
  // Leave only ciphertext fixture data for a failed test to support diagnosis.
  if(report.passed){try{assert.equal(path.dirname(profile).toLowerCase(),(await fs.realpath(os.tmpdir())).toLowerCase());assert.ok(path.basename(profile).startsWith('puty-native-store-'));await fs.rm(profile,{recursive:true,force:true});report.fixtureCleanupVerified=!(await fs.stat(profile).then(()=>true,error=>{if(error.code==='ENOENT')return false;throw error;}));assert.equal(report.fixtureCleanupVerified,true);}catch{report.passed=false;report.failureStage='fixture-cleanup';report.failureType='Error';report.failureMessage='synthetic_fixture_cleanup_failed';process.exitCode=1;}}
  await fs.writeFile(path.join(evidence,'receipt.json'),JSON.stringify(report,null,2));
  console.log(JSON.stringify({passed:report.passed,evidence,failureStage:report.failureStage,cases:report.cases.length}));
}
