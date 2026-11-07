// Actual packaged PC + normal production SSO/SDK. No network replacement,
// route interception, SDK initialization/state injection or business writes.
import fs from 'node:fs/promises';
import {createReadStream} from 'node:fs';
import path from 'node:path';
import {createHash,randomUUID} from 'node:crypto';
import {spawnSync} from 'node:child_process';
import {isIP} from 'node:net';
import {createInterface} from 'node:readline/promises';
import {fileURLToPath} from 'node:url';

export const NAMESPACE='security-20261104',INSTANCE='5ac7c644d9cd4923e2b701f66b52485d';
export const ORIGIN='https://192.168.10.11:9885';
const CLIENT=path.resolve(import.meta.dirname,'..'),SUITE=path.resolve(CLIENT,'../..');
const SECURITY_HOME=path.join(SUITE,'.private/production/acceptance',NAMESPACE);
const FIXTURE=path.join(SECURITY_HOME,'test-accounts.json'),STATE=path.join(SECURITY_HOME,'state.json');
const RESOURCE_HOME=path.join(SECURITY_HOME,'resource-acceptance');
const RESOURCE_REPORT=path.join(RESOURCE_HOME,'report.json'),RESOURCE_INPUT=path.join(RESOURCE_HOME,'attachment-inventory-input.private.json');
const EXE=path.join(CLIENT,'release/win-unpacked/PUTY_IM.exe'),ASAR=path.join(CLIENT,'release/win-unpacked/resources/app.asar');
const sha=value=>createHash('sha256').update(value).digest('hex');
const hex64=value=>typeof value==='string'&&/^[a-f0-9]{64}$/.test(value);
class Rejected extends Error{constructor(code){super(code);this.code=code;}}
const require=(value,code)=>{if(!value)throw new Rejected(code);};
const pause=ms=>new Promise(resolve=>setTimeout(resolve,ms));

export function argumentsFor(argv){
  const result={run:false,localFile:false,timeout:240000};
  const fields=new Map([['--fixture-sha256','fixtureSHA'],['--state-sha256','stateSHA'],['--resource-report-sha256','resourceSHA'],['--exe-sha256','exeSHA'],['--asar-sha256','asarSHA']]);
  const seen=new Set();
  for(let i=0;i<argv.length;i++){
    const arg=argv[i];require(!seen.has(arg),'DUPLICATE_OPTION');seen.add(arg);
    if(arg==='--run')result.run=true;
    else if(arg==='--dry-run')result.dry=true;
    else if(arg==='--local-file-check')result.localFile=true;
    else if(arg==='--timeout-ms'){const value=argv[++i];require(/^\d+$/.test(value||''),'INVALID_TIMEOUT');result.timeout=Number(value);require(result.timeout>=60000&&result.timeout<=900000,'INVALID_TIMEOUT');}
    else if(fields.has(arg)){const value=argv[++i];require(hex64(value),'EXPLICIT_SHA_REQUIRED');result[fields.get(arg)]=value;}
    else throw new Rejected('UNKNOWN_OPTION');
  }
  require(!(result.run&&result.dry),'CONFLICTING_MODE');
  if(result.run)for(const field of fields.values())require(hex64(result[field]),'RUN_HASH_GUARDS_REQUIRED');
  return result;
}

export function validateCaptchaAnswer(value){
  require(typeof value==='string'&&/^(?:[A-Za-z0-9]{1,12}|-[0-9]{1,11})$/.test(value),'MANUAL_CAPTCHA_ANSWER_REQUIRED');
  return value;
}

export function captchaObservation(status,body){
  const data=body?.data;
  return {httpStatus:Number.isSafeInteger(status)?status:-1,code:Number.isSafeInteger(body?.code)?body.code:null,
    enabled:typeof data?.enabled==='boolean'?data.enabled:null,show:typeof data?.show==='boolean'?data.show:null};
}
export function validateCaptchaPolicy(status,body){
  const value=captchaObservation(status,body);
  require(value.httpStatus===200&&value.code===0&&typeof value.enabled==='boolean'&&value.show===value.enabled
    &&body?.data&&typeof body.data==='object'&&!Array.isArray(body.data)
    &&!['token','imToken','chatToken','sessionToken','attachmentToken'].some(k=>Object.hasOwn(body,k)),'CAPTCHA_POLICY_RESPONSE_REJECTED');
  return value;
}

export function safePreloginDiagnostics(rawURL,dom){
  let appOrigin=false,pathname='other',hashPathname='other',hashPathnameSHA256;
  try{const u=new URL(rawURL);appOrigin=u.protocol==='openim38:'&&u.host==='app';
    if(appOrigin){pathname=['/','/index.html'].includes(u.pathname)?u.pathname:'other';
      const route=u.hash.slice(1).split(/[?#]/,1)[0];
      hashPathname=['','/','/login','/conversation','/chat','/home'].includes(route)?route:'other';
      hashPathnameSHA256=sha(Buffer.from(route.slice(0,2048)));}}
  catch{}
  const result={captured:true,appOrigin,pathname,hashPathname,hashPathnameSHA256,
    readyState:['loading','interactive','complete'].includes(dom?.readyState)?dom.readyState:'unknown'};
  for(const key of ['hasAppRoot','hasRenderedApp','usernamePresent','passwordPresent','captchaPresent','usernameVisible','passwordVisible','captchaVisible','updateChecking','forceUpgrade','downloadAndInstall','loginText'])result[key]=dom?.[key]===true;
  return result;
}

export function validateIdentity(fixture,state,now=Date.now()){
  const immutable=['uid','employeeID','userID','username','name','mobile','wecom_userid','remark'];
  for(const source of [fixture,state]){
    require(source&&source.namespace===NAMESPACE&&source.instanceID===INSTANCE&&source.departmentID===53&&!source.creationFailed&&/^[a-f0-9]{32}$/.test(source.runID||''),'FIXTURE_OWNER_REJECTED');
    require(Number.isSafeInteger(source.createdAt)&&now/1000-source.createdAt>=0&&now/1000-source.createdAt<=86400,'FIXTURE_FRESHNESS_REJECTED');
    require(Array.isArray(source.accounts)&&source.accounts.length===2,'TWO_NEW_IDENTITIES_REQUIRED');
    for(const [i,a]of source.accounts.entries()){
      require(a&&Number.isSafeInteger(a.uid)&&a.uid>1&&a.username===['qa_261104_sec_i','qa_261104_sec_j'][i]&&a.name===['Security acceptance I','Security acceptance J'][i]&&a.mobile==='000000000'+(i===0?'09':'10')&&a.wecom_userid===''&&a.remark===`${NAMESPACE} fixture ${source.runID} ${i}`,'EXACT_QA_IDENTITY_REJECTED');
      require(typeof a.employeeID==='string'&&/^[A-Za-z0-9_-]{1,64}$/.test(a.employeeID)&&a.userID==='emp_'+sha(Buffer.from(a.employeeID)).slice(0,32),'QA_IM_ID_REJECTED');
    }
    const uids=source.excludedUIDs,employees=source.excludedEmployeeIDs;
    require(Array.isArray(uids)&&uids.length===6&&new Set(uids).size===6&&uids.every(v=>Number.isSafeInteger(v)&&v>1)&&Array.isArray(employees)&&employees.length===6&&new Set(employees).size===6&&employees.every(v=>typeof v==='string'&&v.length>0),'SIX_OLD_IDENTITIES_REQUIRED');
    require(source.accounts[0].uid!==source.accounts[1].uid&&source.accounts[0].employeeID!==source.accounts[1].employeeID&&source.accounts.every(a=>!uids.includes(a.uid)&&!employees.includes(a.employeeID)),'OLD_IDENTITY_REUSE_REJECTED');
  }
  require(fixture.runID===state.runID&&fixture.accounts.every((a,i)=>immutable.every(k=>a[k]===state.accounts[i][k]))&&JSON.stringify(fixture.excludedUIDs)===JSON.stringify(state.excludedUIDs)&&JSON.stringify(fixture.excludedEmployeeIDs)===JSON.stringify(state.excludedEmployeeIDs),'FIXTURE_STATE_DRIFT');
  const actor=state.accounts[0],peer=state.accounts[1];
  require(typeof actor.password==='string'&&actor.password.length>=6&&actor.password.length<=512&&!actor.pendingPassword,'LATEST_COMMITTED_PASSWORD_REQUIRED');
  const slot=state.sessions?.['resource-save-pc'];
  require(slot&&slot.userID===actor.userID&&slot.policyClient==='pc'&&slot._client==='pc'&&slot._platform===3,'VERIFIED_PAUSE_PC_SLOT_REQUIRED');
  return {actor,peer,runID:state.runID};
}

export function validateResourceCompletion(report,input,identity,hashes){
  require(report&&report.format==='puty-security-resource-result-v1'&&report.namespace===NAMESPACE&&report.instanceID===INSTANCE&&report.runID===identity.runID&&report.passed===true&&report.resourceChecksComplete===true,'RESOURCE_CHECKS_NOT_COMPLETE');
  require(report.fixtureSourceSHA256===hashes.fixtureSHA&&report.pausedStateSHA256===hashes.stateSHA&&report.inventoryInputSHA256===hashes.inventorySHA,'RESOURCE_SOURCE_BINDING_REJECTED');
  require(Array.isArray(report.checks)&&report.checks.length>0&&report.checks.every(v=>v&&v.passed===true),'RESOURCE_CHECKS_NOT_COMPLETE');
  require(input&&input.namespace===NAMESPACE&&input.instanceID===INSTANCE&&input.runID===identity.runID&&input.expectedObjectCount===1&&Array.isArray(input.expectedObjects)&&input.expectedObjects.length===1&&input.expectedObjects[0].completed===true,'RESOURCE_INVENTORY_REJECTED');
  require(JSON.stringify(input.fixtureEmployeeIDs)===JSON.stringify([identity.actor.employeeID,identity.peer.employeeID])&&JSON.stringify(input.fixtureIMUserIDs)===JSON.stringify([identity.actor.userID,identity.peer.userID]),'RESOURCE_INVENTORY_IDENTITY_REJECTED');
}

export function validateSession(value,actor,now=Date.now()){
  require(value&&value.userID===actor.userID&&value.policyClient==='pc'&&['imToken','chatToken','sessionToken'].every(k=>typeof value[k]==='string'&&value[k].length>0&&value[k].length<=32768),'NORMAL_PC_SSO_REJECTED');
  require(Number.isSafeInteger(value.expiresAt)&&value.expiresAt>now/1000,'SSO_EXPIRY_REJECTED');
  return value;
}
export function validateLive(value,actor,now=Date.now()){
  const p=value?.effectivePolicy;
  require(value&&value.userID===actor.userID&&value.platform===3&&value.logicalClient==='pc'&&typeof value.sessionID==='string'&&value.sessionID.length>0&&value.sessionID.length<=512&&!/[\x00-\x20\x7f]/.test(value.sessionID)&&Number.isSafeInteger(value.epoch)&&value.epoch>=1&&value.loginAllowed===true&&value.saveAllowed===true,'LIVE_PC_LEASE_REJECTED');
  require(Number.isSafeInteger(value.expiresAt)&&value.expiresAt>0&&Number.isSafeInteger(now)&&now>0,'LIVE_PC_EXPIRY_REJECTED');
  const expiry=value.expiresAt<100000000000?value.expiresAt*1000:value.expiresAt;require(Number.isSafeInteger(expiry)&&expiry>now,'LIVE_PC_EXPIRY_REJECTED');
  require(p&&p.zone==='lan'&&p.logicalClient==='pc'&&p.login===true&&p.preview===true&&p.save===true&&p.securityVersion===value.epoch&&Number.isSafeInteger(p.policyRevision)&&p.policyRevision>=0&&Number.isSafeInteger(p.networkRevision)&&p.networkRevision>=0,'LIVE_EFFECTIVE_POLICY_REJECTED');
  require(typeof p.currentIP==='string'&&!p.currentIP.includes('%')&&isIP(p.currentIP)>0&&p.sources&&['login','save'].every(key=>typeof p.sources[key]==='string'&&p.sources[key].length>0&&p.sources[key].length<=64&&p.sources[key].trim()===p.sources[key]),'LIVE_EFFECTIVE_POLICY_REJECTED');
  return {epoch:value.epoch};
}
export function cleanupPreview(value){
  require(value&&![value.messages,value.attachments,value.bytes,value.beforeMS].some(n=>!Number.isSafeInteger(n)||n<0)&&value.conversationIDs===null&&!value.errCode,'CLEANUP_PREVIEW_REJECTED');
  return {messages:value.messages,attachments:value.attachments,bytes:value.bytes};
}
export function pendingReceipt(value){
  require(value&&typeof value.path==='string'&&/^puty-local:\/\/pending\/[a-f0-9-]{36}\/SECURITY_ACCEPTANCE_LOCAL_ONLY\.txt$/.test(value.path)&&value.name==='SECURITY_ACCEPTANCE_LOCAL_ONLY.txt'&&value.type==='text/plain','OWNED_PENDING_RECEIPT_REJECTED');
  const id=value.path.split('/')[3];require(/^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/.test(id),'OWNED_PENDING_RECEIPT_REJECTED');return id;
}
export function cleanupScope(base,profile,marker,expectedNonce){
  require(path.dirname(profile)===base&&path.basename(profile)===`puty-production-local-${expectedNonce}`&&/^[a-f0-9-]{36}$/.test(expectedNonce)&&marker?.format==='PUTY-PRODUCTION-LOCAL-SESSION-1'&&marker.namespace===NAMESPACE&&marker.instanceID===INSTANCE&&marker.nonce===expectedNonce,'PROFILE_CLEANUP_SCOPE_REJECTED');
}

async function regular(file,max=Infinity){
  const stat=await fs.lstat(file);require(stat.isFile()&&!stat.isSymbolicLink()&&stat.nlink===1&&stat.size<=max&&await fs.realpath(file)===file,'CANONICAL_REGULAR_FILE_REQUIRED');return stat;
}
export async function privateACL(file,directory,protect=false){
  // Fixed command; only a local path is sent on stdin. No password/token/DSN
  // enters a shell, argv, environment, child output or saved script.
  const command=`$ErrorActionPreference='Stop';$v=[Console]::In.ReadToEnd()|ConvertFrom-Json;$sid=[Security.Principal.WindowsIdentity]::GetCurrent().User;$allowed=@($sid.Value,'S-1-5-18','S-1-5-32-544');if($v.directory){$a=[IO.Directory]::GetAccessControl($v.path)}else{$a=[IO.File]::GetAccessControl($v.path)};if($allowed -notcontains $a.GetOwner([Security.Principal.SecurityIdentifier]).Value){exit 2};if($v.protect){$a=New-Object Security.AccessControl.DirectorySecurity;$a.SetOwner($sid);$a.SetAccessRuleProtection($true,$false);foreach($s in $allowed){$id=New-Object Security.Principal.SecurityIdentifier($s);$r=New-Object Security.AccessControl.FileSystemAccessRule($id,'FullControl','ContainerInherit,ObjectInherit','None','Allow');$a.AddAccessRule($r)};[IO.Directory]::SetAccessControl($v.path,$a)};$rules=$a.GetAccessRules($true,$true,[Security.Principal.SecurityIdentifier]);foreach($r in $rules){if($r.AccessControlType -eq 'Allow' -and $allowed -notcontains $r.IdentityReference.Value -and $r.IdentityReference.Value -ne 'S-1-3-4'){exit 3}};[Console]::Out.Write('private')`;
  const result=spawnSync('C:\\Windows\\System32\\WindowsPowerShell\\v1.0\\powershell.exe',['-NoLogo','-NoProfile','-NonInteractive','-Command',command],{input:JSON.stringify({path:path.toNamespacedPath(file),directory,protect}),encoding:'utf8',windowsHide:true,timeout:30000,maxBuffer:4096});
  require(result.status===0&&result.stdout==='private','PRIVATE_ACL_REQUIRED');
}
async function inputFile(file){await regular(file,4<<20);await privateACL(file,false);const raw=await fs.readFile(file);return {raw,value:JSON.parse(raw.toString('utf8').replace(/^\uFEFF/,'')),sha:sha(raw)};}
async function hashFile(file){const before=await regular(file);const digest=createHash('sha256');for await(const chunk of createReadStream(file))digest.update(chunk);const after=await regular(file);require(before.size===after.size&&before.ino===after.ino&&before.mtimeMs===after.mtimeMs,'PACKAGE_CHANGED_DURING_HASH');return {sha256:digest.digest('hex'),bytes:after.size};}
async function writePrivate(file,value){const handle=await fs.open(file,'wx',0o600);try{await handle.writeFile(JSON.stringify(value,null,2)+'\n');await handle.sync();}finally{await handle.close();}await privateACL(file,false);}
async function checkpoint(page){const value=await page.evaluate(async()=>{if(typeof window.putyLocalCheckpoint!=='function')return null;return await window.putyLocalCheckpoint();});require(value&&value.checkpointed===true&&!value.errCode,'REAL_SDK_WORKING_DB_NOT_READY');}
async function invoke(page,operation,value){return page.evaluate(([op,data])=>window.electronAPI.ipcInvoke('puty-store',op,data),[operation,value]);}
async function waitUntil(check,timeout){const until=Date.now()+timeout;while(Date.now()<until){if(await check())return;await pause(300);}throw new Rejected('REAL_SYNC_WAIT_TIMEOUT');}

export async function execute(options){
  require(process.platform==='win32','WINDOWS_PACKAGED_TEST_REQUIRED');
  // Silence optional protocol debugging before loading Playwright; only this
  // short-lived process is affected. Never alter operator proxy configuration.
  delete process.env.DEBUG;delete process.env.PWDEBUG;
  const [fixture,state,resources,inventory]=await Promise.all([inputFile(FIXTURE),inputFile(STATE),inputFile(RESOURCE_REPORT),inputFile(RESOURCE_INPUT)]);
  require(fixture.sha===options.fixtureSHA&&state.sha===options.stateSHA&&resources.sha===options.resourceSHA,'PRIVATE_INPUT_SHA_MISMATCH');
  const identity=validateIdentity(fixture.value,state.value);
  validateResourceCompletion(resources.value,inventory.value,identity,{...options,inventorySHA:inventory.sha});
  const [exe,asar]=await Promise.all([hashFile(EXE),hashFile(ASAR)]);require(exe.sha256===options.exeSHA&&asar.sha256===options.asarSHA,'PACKAGED_CANDIDATE_SHA_MISMATCH');
  const parentStat=await fs.lstat(SECURITY_HOME);require(parentStat.isDirectory()&&!parentStat.isSymbolicLink()&&await fs.realpath(SECURITY_HOME)===SECURITY_HOME,'PRIVATE_HOME_REDIRECTED');await privateACL(SECURITY_HOME,true);
  const nonce=randomUUID(),home=path.join(SECURITY_HOME,`pc-local-session-${Date.now()}-${nonce}`);
  await fs.mkdir(home);await privateACL(home,true,true);
  const profile=path.join(home,`puty-production-local-${nonce}`);await fs.mkdir(profile);await privateACL(profile,true,true);
  const owner={format:'PUTY-PRODUCTION-LOCAL-SESSION-1',namespace:NAMESPACE,instanceID:INSTANCE,nonce};
  await writePrivate(path.join(profile,'.puty-production-local-owner.json'),owner);
  const dataRoot=path.join(profile,'encrypted-data');await fs.mkdir(dataRoot);await writePrivate(path.join(dataRoot,'.puty-local-root.json'),{format:'PUTY-LOCAL-ROOT-1',id:randomUUID()});
  const report={format:'puty-production-local-session-v1',namespace:NAMESPACE,instanceID:INSTANCE,strictTLS:true,packaged:true,exe,asar,resourceReportSHA256:resources.sha,fixtureSourceSHA256:fixture.sha,pausedStateSHA256:state.sha,networkInterception:false,sdkStateInjection:false,serverBusinessObjectsCreatedByScript:0,localFileCheck:options.localFile,cases:[],counts:{ssoLogin:0,ssoRefresh:0,websockets:0,wsFrames:0,businessMutationRequests:0},logout:false,locked:false,profileCleanupVerified:false,passed:false};
  let app,page,session,phase='launch',ownedPending,loginResponseError,closing=false;
  const captures=new Set();
  const mark=name=>report.cases.push({code:name,passed:true});
  try{
    const {_electron}=await import('playwright');
    const environment={};for(const key of ['SystemRoot','WINDIR','PATH','TEMP','TMP','USERPROFILE','APPDATA','LOCALAPPDATA','PROGRAMDATA','PROGRAMFILES','ProgramFiles(x86)','ComSpec','PATHEXT','NUMBER_OF_PROCESSORS','PROCESSOR_ARCHITECTURE','LANG','LC_ALL','TZ'])if(process.env[key]!==undefined)environment[key]=process.env[key];
    Object.assign(environment,{OPENIM38_TEST_DATA_DIR:profile,OPENIM38_SMOKE_TEST:'1'});
    app=await _electron.launch({executablePath:EXE,args:[],cwd:CLIENT,env:environment,timeout:60000});
    const version=await app.evaluate(({app})=>app.getVersion());require(typeof version==='string'&&/^[0-9A-Za-z.+_-]{1,64}$/.test(version),'PACKAGED_VERSION_REJECTED');report.appVersion=version;
    await waitUntil(async()=>{page=app.windows().find(v=>v.url().startsWith('openim38://app/'));return Boolean(page);},60000);
    page.setDefaultTimeout(30000);
    page.on('websocket',socket=>{try{const url=new URL(socket.url());if(url.protocol==='wss:'&&url.host===new URL(ORIGIN).host&&/^\/ws(?:\/|$)/.test(url.pathname)){report.counts.websockets++;socket.on('framereceived',()=>report.counts.wsFrames++);}}catch{}});
    page.on('request',request=>{try{const url=new URL(request.url());if(url.origin===ORIGIN&&request.method()==='POST'&&/^\/(?:api\/msg\/send_msg|api\/group\/create_group|api\/object\/(?:initiate|complete)|chat\/account\/register)/.test(url.pathname))report.counts.businessMutationRequests++;}catch{}});
    page.on('response',response=>{
      let url;try{url=new URL(response.url());}catch{return;}
      if(url.origin!==ORIGIN||!['/sso/login','/sso/refresh'].includes(url.pathname))return;
      const capture=(async()=>{
        try{const body=await response.json();if(response.status()!==200||body?.code!==0){loginResponseError='NORMAL_LOGIN_REJECTED';return;}
          const value=validateSession(body.data,identity.actor);if(!closing)session=value;
          report.counts[url.pathname==='/sso/login'?'ssoLogin':'ssoRefresh']++;
        }catch{loginResponseError='SSO_RESPONSE_REJECTED';}
      })();captures.add(capture);void capture.finally(()=>captures.delete(capture));
    });
    phase='normal-login-ready';
    // Observe a fresh normal page request, before entering any credentials.
    // A missing/error response can never stand in for an explicitly disabled policy.
    const [captchaPolicy]=await Promise.all([
      page.waitForResponse(response=>{try{const u=new URL(response.url());return u.origin===ORIGIN&&u.pathname==='/sso/captcha'&&response.request().method()==='GET';}catch{return false;}},{timeout:60000}).then(async response=>{
        let body;try{body=await response.json();}catch{report.captchaObservation=captchaObservation(response.status(),null);throw new Rejected('CAPTCHA_POLICY_RESPONSE_REJECTED');}
        report.captchaObservation=captchaObservation(response.status(),body);return validateCaptchaPolicy(response.status(),body);
      }),page.reload({waitUntil:'domcontentloaded',timeout:60000})]);
    await page.waitForURL(/#\/login(?:\?|$)/,{timeout:60000});
    const username=page.locator('input[autocomplete="username"]'),password=page.locator('input[autocomplete="current-password"]'),captcha=page.locator('input#captcha');
    await username.waitFor({state:'visible'});await password.waitFor({state:'visible'});
    await page.evaluate(()=>{
      // Reload has finished. Only observe the product's real callback; never
      // dispatch it or inject SDK state, and install before normal login.
      window.__putyProductionSyncObservations={ready:0};
      window.addEventListener('puty-archive-ready',()=>{window.__putyProductionSyncObservations.ready++;});
    });
    const image=page.getByAltText('点击更换验证码',{exact:true});let answer;
    if(captchaPolicy.enabled){
      await image.waitFor({state:'visible'});await captcha.waitFor({state:'visible'});
      const captchaOrigin=await page.evaluate(()=>{
      const values=performance.getEntriesByType('resource').flatMap(entry=>{try{const url=new URL(entry.name);return url.pathname==='/sso/captcha'?[url.origin]:[];}catch{return [];}});return values.at(-1);
      });require(captchaOrigin===ORIGIN,'NORMAL_LAN_CAPTCHA_ORIGIN_REQUIRED');
      const picture=path.join(home,'captcha.png');await image.screenshot({path:picture});await privateACL(picture,false);
      // Only the captcha element is captured, before entering either credential.
      console.log(JSON.stringify({phase:'captcha',image:picture,manualAnswerRequired:true}));
      const terminal=createInterface({input:process.stdin,output:process.stdout,terminal:false});
      try{answer=(await terminal.question('Captcha answer (normal UI): ')).trim();}finally{terminal.close();}
      validateCaptchaAnswer(answer);
    }else require(!(await captcha.isVisible())&&!(await image.isVisible()),'CAPTCHA_POLICY_DOM_MISMATCH');
    require(sha(await fs.readFile(STATE))===state.sha&&sha(await fs.readFile(FIXTURE))===fixture.sha,'PAUSED_IDENTITY_CHANGED_BEFORE_LOGIN');
    await username.fill(identity.actor.username);await password.fill(identity.actor.password);if(captchaPolicy.enabled)await captcha.fill(answer);answer=undefined;
    const reply=page.waitForResponse(r=>{try{return new URL(r.url()).origin===ORIGIN&&new URL(r.url()).pathname==='/sso/login'&&r.request().method()==='POST';}catch{return false;}},{timeout:60000});
    await page.getByRole('button',{name:'登录',exact:true}).click();await reply;await Promise.allSettled([...captures]);
    require(!loginResponseError&&session,'NORMAL_PC_LOGIN_FAILED');
    await page.waitForURL(value=>!value.hash.startsWith('#/login'),{timeout:60000});mark(captchaPolicy.enabled?'NORMAL_UI_AND_CAPTCHA_LOGIN':'NORMAL_UI_SERVER_CAPTCHA_DISABLED_LOGIN');
    phase='real-sdk-sync';
    const conversationID='si_'+[identity.actor.userID,identity.peer.userID].sort().join('_');let history;
    await waitUntil(async()=>{require(!loginResponseError,'SSO_REFRESH_REJECTED');try{history=await invoke(page,'history',{conversationID,count:20});return Array.isArray(history);}catch{return false;}},options.timeout);
    require(report.counts.websockets>0&&report.counts.wsFrames>0,'REAL_WSS_NOT_OBSERVED');
    report.realSyncFinishEvents=await page.evaluate(()=>window.__putyProductionSyncObservations?.ready||0);require(report.realSyncFinishEvents>0,'REAL_SDK_SYNC_FINISH_NOT_OBSERVED');
    report.historyRows=history.length;history=undefined;await checkpoint(page);mark('REAL_WSS_SYNC_ARCHIVE_AND_SDK_WORKING_DB');
    const configuration=await invoke(page,'config');require(configuration?.directory===dataRoot&&configuration.encrypted===true&&configuration.configured===true,'ISOLATED_NATIVE_ROOT_REJECTED');
    phase='real-online-lease';
    const start=Date.now();await pause(11000);
    await invoke(page,'stats');
    const live=await page.evaluate(async({origin,imToken,chatToken})=>{
      const r=await fetch(origin+'/enterprise/v1/session/state',{method:'GET',cache:'no-store',redirect:'error',headers:{token:imToken,chatToken,platformID:'3'}});
      const body=await r.json();return {status:r.status,code:body?.errCode,data:body?.data};
    },{origin:ORIGIN,imToken:session.imToken,chatToken:session.chatToken});
    require(live.status===200&&live.code===0,'ONLINE_SESSION_STATE_FAILED');validateLive(live.data,identity.actor);report.leaseRecheckAfterMS=Date.now()-start;mark('REAL_BACKEND_LEASE_AFTER_INITIAL_CACHE_WINDOW');
    phase='actual-settings-preview';
    // Open the normal profile menu/overlay without unmounting the live SDK host.
    await page.locator('.ant-layout-sider .ant-avatar').first().click();
    await page.locator('.profile-popover').getByText(/^(?:账号设置|Account Settings)$/).click();
    await page.getByText('本地存储 / 清理聊天记录',{exact:true}).click();
    await page.getByText('本地存储与聊天记录清理',{exact:true}).waitFor({state:'visible'});
    await page.getByText(dataRoot,{exact:true}).waitFor({state:'visible'});
    await page.getByRole('button',{name:'计算清理范围',exact:true}).click();
    await page.getByText(/^将清理 \d+ 条消息、\d+ 个附件，预计释放 /).waitFor({state:'visible'});
    const preview=await page.evaluate(async()=>await window.putyLocalCleanupPreview({}));report.cleanupPreview=cleanupPreview(preview);mark('ACTUAL_STORAGE_SETTINGS_PREVIEW_ONLY');
    await page.getByRole('button',{name:'关闭',exact:true}).click();
    if(options.localFile){
      phase='owned-local-file';const bytes=Buffer.from('PUTY synthetic PC local-session bytes '+nonce),expectedSHA=sha(bytes);
      const stored=await invoke(page,'stash-file',{name:'SECURITY_ACCEPTANCE_LOCAL_ONLY.txt',type:'text/plain',bytes:[...bytes]});const id=pendingReceipt(stored);ownedPending={id,path:stored.path};
      const cipher=path.join(dataRoot,'accounts',identity.actor.userID,'vault','pending',id,'SECURITY_ACCEPTANCE_LOCAL_ONLY.txt');await regular(cipher,1<<20);const sealed=await fs.readFile(cipher);require(sealed.subarray(0,8).toString()==='PUTYLS1\n'&&!sealed.includes(bytes),'LOCAL_STASH_NOT_ENCRYPTED');
      const read=await page.evaluate(async value=>{const item=await window.electronAPI.ipcInvoke('puty-store','file-info',{path:value});return {name:item.name,bytes:Array.from(item.bytes)};},stored.path);
      require(read.name==='SECURITY_ACCEPTANCE_LOCAL_ONLY.txt'&&sha(Buffer.from(read.bytes))===expectedSHA,'LOCAL_STASH_READ_MISMATCH');
      const released=await invoke(page,'release-pending',{ids:[id],clientMsgID:'acceptance-local-'+nonce});require(released?.released===true,'LOCAL_PENDING_RELEASE_NOT_ACKNOWLEDGED');ownedPending=undefined;
      await checkpoint(page);require(!(await fs.stat(cipher).then(()=>true,e=>{if(e.code==='ENOENT')return false;throw e;})),'LOCAL_PENDING_CIPHER_NOT_REMOVED');mark('OWNED_LOCAL_STASH_ENCRYPT_READ_RELEASE');
    }
    require(report.counts.businessMutationRequests===0,'UNEXPECTED_BUSINESS_MUTATION_OBSERVED');
    report.passed=true;
  }catch(error){report.passed=false;report.failurePhase=phase;report.failureCode=error instanceof Rejected?error.code:error?.name==='TimeoutError'?'PHASE_TIMEOUT':'LOCAL_SESSION_ACCEPTANCE_FAILED';
    if(phase==='normal-login-ready'&&page&&!page.isClosed()){
      try{const dom=await Promise.race([page.evaluate(()=>{const text=document.body?.innerText||'',root=document.getElementById('root');const visible=selector=>{const el=document.querySelector(selector);if(!el)return false;const style=getComputedStyle(el);return style.visibility!=='hidden'&&style.display!=='none'&&el.getClientRects().length>0;};return {
        readyState:document.readyState,hasAppRoot:Boolean(root),hasRenderedApp:Boolean(root?.childElementCount),
        usernamePresent:Boolean(document.querySelector('input[autocomplete="username"]')),passwordPresent:Boolean(document.querySelector('input[autocomplete="current-password"]')),captchaPresent:Boolean(document.querySelector('input#captcha')),
        usernameVisible:visible('input[autocomplete="username"]'),passwordVisible:visible('input[autocomplete="current-password"]'),captchaVisible:visible('input#captcha'),
        updateChecking:text.includes('检查应用版本'),forceUpgrade:text.includes('需要更新到'),downloadAndInstall:text.includes('下载并安装'),loginText:text.includes('登录')};}),pause(2500).then(()=>{throw new Rejected('DIAGNOSTIC_UNAVAILABLE');})]);
        report.preloginDiagnostics=safePreloginDiagnostics(page.url(),dom);
      }catch{report.preloginDiagnostics={captured:false};}
    }
  }
  finally{
    closing=true;await Promise.allSettled([...captures]);
    if(page&&ownedPending)await invoke(page,'release-pending',{ids:[ownedPending.id],clientMsgID:'acceptance-local-'+nonce}).catch(()=>{});
    if(session&&app){
      try{
        const result=page&&!page.isClosed()?await page.evaluate(async({origin,token})=>{const response=await fetch(origin+'/sso/logout',{method:'POST',cache:'no-store',redirect:'error',headers:{'Content-Type':'application/json'},body:JSON.stringify({sessionToken:token})});const value=await response.json();return response.status===200&&(value.code??value.errCode)===0;},{origin:ORIGIN,token:session.sessionToken}):await app.evaluate(async({net},input)=>{const response=await net.fetch(input.origin+'/sso/logout',{method:'POST',cache:'no-store',redirect:'error',headers:{'Content-Type':'application/json'},body:JSON.stringify({sessionToken:input.token})});const value=await response.json();return response.status===200&&(value.code??value.errCode)===0;},{origin:ORIGIN,token:session.sessionToken});
        report.logout=result===true;
      }catch{}
    }
    session=undefined;identity.actor.password=undefined;
    if(page&&!page.isClosed()){try{report.locked=await invoke(page,'lock')===true;}catch{}}
    if(app){await app.evaluate(({app})=>app.exit(0)).catch(()=>{});await app.close().catch(()=>{});}
    try{
      require(await fs.realpath(home)===home&&await fs.realpath(profile)===profile,'PROFILE_CLEANUP_REDIRECTED');const marker=JSON.parse(await fs.readFile(path.join(profile,'.puty-production-local-owner.json'),'utf8'));cleanupScope(home,profile,marker,nonce);
      // One exact app-owned profile, never an account path or user-selected root.
      for(let attempt=0;attempt<4;attempt++){try{await fs.rm(profile,{recursive:true,force:false});break;}catch{if(attempt===3)throw new Rejected('PROFILE_CLEANUP_FAILED');await pause(500);}}
      report.profileCleanupVerified=!(await fs.stat(profile).then(()=>true,e=>{if(e.code==='ENOENT')return false;throw e;}));
    }catch{report.failurePhase='profile-cleanup';report.failureCode='PROFILE_CLEANUP_FAILED';}
    if(!report.logout||!report.locked||!report.profileCleanupVerified)report.passed=false;
    await writePrivate(path.join(home,'report.json'),report);
    console.log(JSON.stringify({passed:report.passed,report:path.join(home,'report.json'),phase:report.failurePhase,cases:report.cases.length,logout:report.logout,profileCleanupVerified:report.profileCleanupVerified}));
  }
  return report.passed?0:1;
}

export async function main(argv=process.argv.slice(2)){
  const options=argumentsFor(argv);
  if(!options.run){console.log(JSON.stringify({dryRun:true,privateFilesRead:0,productionRequests:0,namespace:NAMESPACE,requiresCompletedResourceReceipt:true,manualCaptcha:true,packagedOnly:true,networkInterception:false,sdkStateInjection:false,serverBusinessObjectsCreatedByScript:0}));return 0;}
  return execute(options);
}
if(process.argv[1]&&path.resolve(process.argv[1])===fileURLToPath(import.meta.url)){
  try{process.exitCode=await main();}catch(error){console.error(JSON.stringify({passed:false,phase:'preflight',code:error instanceof Rejected?error.code:'LOCAL_SESSION_PREFLIGHT_REJECTED',credentialsPrinted:false}));process.exitCode=1;}
}
