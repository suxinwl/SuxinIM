import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import {createHash,randomUUID} from 'node:crypto';
import {argumentsFor,validateCaptchaAnswer,captchaObservation,validateCaptchaPolicy,safePreloginDiagnostics,validateIdentity,validateResourceCompletion,validateSession,validateLive,cleanupPreview,pendingReceipt,cleanupScope,privateACL,main,NAMESPACE,INSTANCE} from '../scripts/accept-production-local-session.mjs';

const sha=value=>createHash('sha256').update(value).digest('hex');
const hex='a'.repeat(64);
function fixtures(){
  const runID='a'.repeat(32);
  const value={namespace:NAMESPACE,instanceID:INSTANCE,departmentID:53,runID,createdAt:Math.floor(Date.now()/1000),excludedUIDs:[10,11,12,13,14,15],excludedEmployeeIDs:['old-a','old-b','old-c','old-d','old-e','old-f'],accounts:[0,1].map(i=>{
    const employeeID='new-synthetic-'+i;
    return {uid:9001+i,employeeID,userID:'emp_'+sha(employeeID).slice(0,32),username:['qa_261104_sec_i','qa_261104_sec_j'][i],name:['Security acceptance I','Security acceptance J'][i],mobile:'000000000'+(i===0?'09':'10'),wecom_userid:'',remark:`${NAMESPACE} fixture ${runID} ${i}`,password:'synthetic-password-only'};
  })};
  const state=structuredClone(value);state.sessions={'resource-save-pc':{userID:state.accounts[0].userID,policyClient:'pc',_client:'pc',_platform:3}};
  return [value,state];
}
function completion(identity){
  return [{format:'puty-security-resource-result-v1',namespace:NAMESPACE,instanceID:INSTANCE,runID:identity.runID,passed:true,resourceChecksComplete:true,fixtureSourceSHA256:hex,pausedStateSHA256:hex,inventoryInputSHA256:hex,checks:[{passed:true}]},
    {namespace:NAMESPACE,instanceID:INSTANCE,runID:identity.runID,expectedObjectCount:1,expectedObjects:[{completed:true}],fixtureEmployeeIDs:[identity.actor.employeeID,identity.peer.employeeID],fixtureIMUserIDs:[identity.actor.userID,identity.peer.userID]}];
}
const hashes={fixtureSHA:hex,stateSHA:hex,inventorySHA:hex};

test('operator captcha input accepts signed arithmetic answers without solving or normalizing them',()=>{
  for(const value of ['0','123','synthetic','-48','-12345678901'])assert.equal(validateCaptchaAnswer(value),value);
  for(const value of ['',null,'-','--48','-synthetic','1-2','-123456789012','1\n2','<script>'])assert.throws(()=>validateCaptchaAnswer(value),/MANUAL_CAPTCHA_ANSWER_REQUIRED/);
});

test('captcha policy accepts only successful explicit matching booleans and never exposes images or IDs',()=>{
  for(const enabled of [true,false]){
    const value=validateCaptchaPolicy(200,{code:0,data:{enabled,show:enabled,id:'SYNTHETIC_PRIVATE',image:'SYNTHETIC_PRIVATE'}});
    assert.deepEqual(value,{httpStatus:200,code:0,enabled,show:enabled});assert.equal(JSON.stringify(value).includes('SYNTHETIC_PRIVATE'),false);
  }
  for(const [status,body] of [[503,{code:0,data:{enabled:false,show:false}}],[200,{code:1,data:{enabled:false,show:false}}],[200,{code:false,data:{enabled:false,show:false}}],[200,{code:0,data:{}}],[200,{code:0,data:{enabled:false}}],[200,{code:0,data:{enabled:0,show:false}}],[200,{code:0,data:{enabled:'false',show:false}}],[200,{code:0,data:{enabled:false,show:true}}],[200,{code:0,data:[]}],[200,{code:0,data:{enabled:false,show:false},token:'SYNTHETIC_PRIVATE'}],[200,null]])assert.throws(()=>validateCaptchaPolicy(status,body),/CAPTCHA_POLICY_RESPONSE_REJECTED/);
  assert.deepEqual(captchaObservation(503,null),{httpStatus:503,code:null,enabled:null,show:null});
});
test('normal page reload precedes read-only sync observer and login, without captcha state injection',async()=>{
  const source=await fs.readFile(new URL('../scripts/accept-production-local-session.mjs',import.meta.url),'utf8');
  assert.equal((source.match(/page\.reload\(/g)||[]).length,1);
  assert.ok(source.indexOf('page.reload(')<source.indexOf('window.__putyProductionSyncObservations='));
  assert.ok(source.indexOf('window.__putyProductionSyncObservations=')<source.indexOf('await username.fill('));
  assert.match(source,/validateCaptchaPolicy\(response.status\(\),body\)/);
  assert.match(source,/CAPTCHA_POLICY_DOM_MISMATCH/);assert.equal(source.includes('setCaptcha('),false);
});

test('prelogin diagnostics retain only allowlisted route and fixed DOM booleans without URL queries or text',()=>{
  const result=safePreloginDiagnostics('openim38://app/index.html#/login?sessionToken=SYNTHETIC_PRIVATE',{readyState:'complete',hasAppRoot:true,hasRenderedApp:true,usernamePresent:true,passwordPresent:true,captchaPresent:false,usernameVisible:true,passwordVisible:false,captchaVisible:false,updateChecking:false,forceUpgrade:true,downloadAndInstall:true,loginText:true,rawText:'SYNTHETIC_PRIVATE',inputValue:'SYNTHETIC_PRIVATE'});
  assert.equal(result.pathname,'/index.html');assert.equal(result.hashPathname,'/login');assert.equal(result.hashPathnameSHA256,sha(Buffer.from('/login')));
  assert.equal(result.readyState,'complete');assert.equal(result.forceUpgrade,true);assert.equal(result.captchaPresent,false);assert.equal(result.usernameVisible,true);assert.equal(result.passwordVisible,false);
  assert.equal(JSON.stringify(result).includes('SYNTHETIC_PRIVATE'),false);assert.equal('rawText'in result,false);assert.equal('inputValue'in result,false);
});
test('prelogin diagnostics never persist unknown paths, external URLs or arbitrary DOM values',()=>{
  const result=safePreloginDiagnostics('openim38://app/private/SYNTHETIC_PRIVATE#/chat/SYNTHETIC_PRIVATE?token=PRIVATE',{readyState:'SYNTHETIC_PRIVATE',forceUpgrade:'SYNTHETIC_PRIVATE',hasAppRoot:1});
  assert.equal(result.pathname,'other');assert.equal(result.hashPathname,'other');assert.equal(result.readyState,'unknown');assert.equal(result.forceUpgrade,false);assert.equal(result.hasAppRoot,false);assert.equal(JSON.stringify(result).includes('SYNTHETIC_PRIVATE'),false);
  for(const url of ['https://example.invalid/SYNTHETIC_PRIVATE','not a URL']){const v=safePreloginDiagnostics(url,{});assert.equal(v.appOrigin,false);assert.equal(v.pathname,'other');assert.equal(v.hashPathname,'other');assert.equal(v.hashPathnameSHA256,undefined);}
});

test('default dry-run reads no private files, package or network and creates no profile',async()=>{
  const oldLog=console.log,oldRead=fs.readFile,oldOpen=fs.open,oldMkdir=fs.mkdir;let read=0,write=0;const reports=[];
  console.log=value=>reports.push(JSON.parse(value));fs.readFile=async()=>{read++;throw new Error('private read prohibited');};fs.open=async()=>{write++;throw new Error('write prohibited');};fs.mkdir=async()=>{write++;throw new Error('mkdir prohibited');};
  try{assert.equal(await main([]),0);}finally{console.log=oldLog;fs.readFile=oldRead;fs.open=oldOpen;fs.mkdir=oldMkdir;}
  assert.equal(read,0);assert.equal(write,0);assert.equal(reports[0].productionRequests,0);assert.equal(reports[0].requiresCompletedResourceReceipt,true);
});
test('explicit run requires all five SHA guards and rejects unknown options and conflicting modes',()=>{
  assert.throws(()=>argumentsFor(['--run']),/RUN_HASH_GUARDS_REQUIRED/);
  assert.throws(()=>argumentsFor(['--url','https://untrusted']),/UNKNOWN_OPTION/);
  assert.throws(()=>argumentsFor(['--run','--dry-run']),/CONFLICTING_MODE/);
  assert.throws(()=>argumentsFor(['--dry-run','--dry-run']),/DUPLICATE_OPTION/);
  assert.throws(()=>argumentsFor(['--state-sha256','not-a-hash']),/EXPLICIT_SHA_REQUIRED/);
  assert.throws(()=>argumentsFor(['--timeout-ms','1']),/INVALID_TIMEOUT/);
  const args=['--run','--local-file-check',...['fixture','state','resource-report','exe','asar'].flatMap(k=>[`--${k}-sha256`,hex])];
  assert.equal(argumentsFor(args).run,true);assert.equal(argumentsFor(args).localFile,true);
});
test('fresh exact I/J binding uses only committed latest I password',()=>{
  const [fixture,state]=fixtures();fixture.accounts[0].password='obsolete-synthetic-password';
  assert.equal(validateIdentity(fixture,state).actor.password,'synthetic-password-only');
  const bad=structuredClone(state);bad.accounts[0].pendingPassword='pending';assert.throws(()=>validateIdentity(fixture,bad),/LATEST_COMMITTED_PASSWORD_REQUIRED/);
});
test('real account names, old identities, wrong scope and mismatched IM IDs cannot run',()=>{
  for(const mutate of [v=>v.accounts[0].username='real-account',v=>v.departmentID=1,v=>v.accounts[0].uid=10,v=>v.accounts[0].userID='emp_'+'0'.repeat(32),v=>v.accounts[0].wecom_userid='nonempty',v=>v.accounts[1].employeeID=v.accounts[0].employeeID,v=>v.excludedUIDs.pop(),v=>v.createdAt-=86401]){
    const [fixture,state]=fixtures();mutate(state);assert.throws(()=>validateIdentity(fixture,state));
  }
});
test('immutable fixture state mismatch and missing pc pause slot are rejected',()=>{
  const [fixture,state]=fixtures();state.accounts[0].uid+=100;
  assert.throws(()=>validateIdentity(fixture,state),/FIXTURE_STATE_DRIFT/);
  const [f,s]=fixtures();delete s.sessions['resource-save-pc'];assert.throws(()=>validateIdentity(f,s),/VERIFIED_PAUSE_PC_SLOT_REQUIRED/);
});
test('partial resource receipt never authorizes replacement of resource-save-pc',()=>{
  const identity=validateIdentity(...fixtures());const [report,input]=completion(identity);
  for(const mutate of [r=>r.passed=false,r=>r.resourceChecksComplete=false,r=>delete r.resourceChecksComplete,r=>r.checks=[],r=>r.checks[0].passed=false]){const bad=structuredClone(report);mutate(bad);assert.throws(()=>validateResourceCompletion(bad,input,identity,hashes));}
  validateResourceCompletion(report,input,identity,hashes);
});
test('completed resource receipt binds exact run and all three source and inventory hashes',()=>{
  const identity=validateIdentity(...fixtures());const [report,input]=completion(identity);
  for(const field of ['fixtureSourceSHA256','pausedStateSHA256','inventoryInputSHA256','runID']){const bad=structuredClone(report);bad[field]='b'.repeat(field==='runID'?32:64);assert.throws(()=>validateResourceCompletion(bad,input,identity,hashes));}
  const other=structuredClone(input);other.fixtureIMUserIDs[0]='emp_'+'0'.repeat(32);assert.throws(()=>validateResourceCompletion(report,other,identity,hashes),/RESOURCE_INVENTORY_IDENTITY_REJECTED/);
  other.fixtureIMUserIDs=input.fixtureIMUserIDs;other.expectedObjects[0].completed=false;assert.throws(()=>validateResourceCompletion(report,other,identity,hashes),/RESOURCE_INVENTORY_REJECTED/);
});
test('normal SSO accepts only exact I pc, formal pair and future expiry',()=>{
  const {actor}=validateIdentity(...fixtures());const value={userID:actor.userID,policyClient:'pc',imToken:'synthetic-im',chatToken:'synthetic-chat',sessionToken:'synthetic-sso',expiresAt:Math.floor(Date.now()/1000)+3600};
  validateSession(value,actor);
  for(const [key,bad]of [['userID','emp_other'],['policyClient','web_desktop'],['sessionToken',''],['expiresAt',0]])assert.throws(()=>validateSession({...value,[key]:bad},actor));
});
test('real lease requires complete matching pc epoch and save policy',()=>{
  const {actor}=validateIdentity(...fixtures());const value={userID:actor.userID,platform:3,logicalClient:'pc',sessionID:'synthetic-sid',epoch:3,expiresAt:Math.floor(Date.now()/1000)+3600,loginAllowed:true,saveAllowed:true,effectivePolicy:{zone:'lan',logicalClient:'pc',login:true,preview:true,save:true,securityVersion:3,policyRevision:0,networkRevision:0,currentIP:'192.168.10.10',sources:{login:'roles',save:'person'}}};
  validateLive(value,actor);
  validateLive({...value,expiresAt:Date.now()+3600000},actor);
  for(const mutate of [v=>v.platform=5,v=>v.epoch=0,v=>v.saveAllowed=false,v=>v.effectivePolicy.securityVersion=2,v=>v.effectivePolicy.zone='internet',v=>delete v.effectivePolicy.networkRevision,v=>delete v.expiresAt,v=>v.expiresAt=1,v=>v.expiresAt=Number.MAX_SAFE_INTEGER+1,v=>v.effectivePolicy.currentIP='host.example',v=>v.effectivePolicy.currentIP='[::1]',v=>v.effectivePolicy.currentIP='fe80::1%eth0',v=>delete v.effectivePolicy.sources.save,v=>v.effectivePolicy.sources.login=true]){const bad=structuredClone(value);mutate(bad);assert.throws(()=>validateLive(bad,actor));}
});
test('cleanup preview is typed, all-scope and cannot become a cleanup mutation',()=>{
  const value={messages:0,attachments:0,bytes:0,beforeMS:0,conversationIDs:null};assert.deepEqual(cleanupPreview(value),{messages:0,attachments:0,bytes:0});
  for(const extra of [{messages:-1},{bytes:1.5},{beforeMS:'0'},{conversationIDs:[]},{errCode:1}])assert.throws(()=>cleanupPreview({...value,...extra}));
});
test('pending release guard only accepts the one owned local synthetic path',()=>{
  const id=randomUUID(),value={path:`puty-local://pending/${id}/SECURITY_ACCEPTANCE_LOCAL_ONLY.txt`,name:'SECURITY_ACCEPTANCE_LOCAL_ONLY.txt',type:'text/plain'};assert.equal(pendingReceipt(value),id);
  for(const extra of [{path:'file:///outside'},{name:'other.txt'},{type:'image/png'},{path:value.path+'?token=x'},{path:value.path.replace(id,'../../outside')}])assert.throws(()=>pendingReceipt({...value,...extra}));
});
test('profile cleanup requires exact canonical parent, prefix, marker and nonce',()=>{
  const nonce=randomUUID(),base=path.resolve('synthetic-private-home'),profile=path.join(base,`puty-production-local-${nonce}`),owner={format:'PUTY-PRODUCTION-LOCAL-SESSION-1',namespace:NAMESPACE,instanceID:INSTANCE,nonce};
  cleanupScope(base,profile,owner,nonce);
  assert.throws(()=>cleanupScope(base,path.dirname(base),owner,nonce));assert.throws(()=>cleanupScope(base,profile,{...owner,namespace:'old-fixture'},nonce));assert.throws(()=>cleanupScope(base,profile,owner,randomUUID()));
});
test('script source has no request shim, route interception, fake SDK state or business mutations',async()=>{
  const source=await fs.readFile(new URL('../scripts/accept-production-local-session.mjs',import.meta.url),'utf8');
  for(const pattern of [/net\.fetch\s*=/,/\.route\s*\(/,/\.fulfill\s*\(/,/\.abort\s*\(/,/invoke\(page\s*,\s*['"]sync-state['"]/,/window\.initDB\s*\(/,/setIMProfile\s*\(/,/putyLocalCleanup\s*\(/,/ignoreHTTPSErrors\s*:/,/certificate-error/])assert.equal(pattern.test(source),false,String(pattern));
  assert.match(source,/release\/win-unpacked\/PUTY_IM\.exe/);assert.match(source,/manualAnswerRequired:true/);assert.match(source,/image\.screenshot/);assert.equal(/page\.screenshot/.test(source),false);
});
test('Windows profile ACL is actually restricted before any synthetic file is written',{skip:process.platform!=='win32'},async()=>{
  const parent=path.resolve('D:/PUTY_AI/pc-session-guard-tests');await fs.mkdir(parent,{recursive:true});
  const canonical=await fs.realpath(parent),leaf=await fs.mkdtemp(path.join(canonical,'pc-session-acl-'));
  try{await privateACL(leaf,true,true);const file=path.join(leaf,'synthetic.txt');await fs.writeFile(file,'synthetic only');await privateACL(file,false);await privateACL(leaf,true);}
  finally{assert.equal(path.dirname(await fs.realpath(leaf)),canonical);assert.ok(path.basename(leaf).startsWith('pc-session-acl-'));await fs.rm(leaf,{recursive:true,force:false});}
});

test('Windows long private marker paths retain the same real ACL guard',{skip:process.platform!=='win32'},async()=>{
  const parent=path.resolve('D:/PUTY_AI/pc-session-guard-tests');await fs.mkdir(parent,{recursive:true});
  const canonical=await fs.realpath(parent),leaf=await fs.mkdtemp(path.join(canonical,'pc-session-long-acl-'));
  try{
    await privateACL(leaf,true,true);
    const nested=path.join(leaf,...Array.from({length:4},(_,i)=>'synthetic-'+i+'-'+'x'.repeat(48)));
    await fs.mkdir(nested,{recursive:true});
    const file=path.join(nested,'.puty-production-local-owner.json');assert.ok(file.length>300);
    const handle=await fs.open(file,'wx',0o600);try{await handle.writeFile('synthetic only');await handle.sync();}finally{await handle.close();}
    await privateACL(file,false);await privateACL(nested,true);
  }finally{
    assert.equal(path.dirname(await fs.realpath(leaf)),canonical);assert.ok(path.basename(leaf).startsWith('pc-session-long-acl-'));
    await fs.rm(leaf,{recursive:true,force:false});
  }
});
