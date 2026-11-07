import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile, mkdtemp, writeFile, mkdir } from 'node:fs/promises';
import { existsSync } from 'node:fs';
import { resolve, join } from 'node:path';
import { tmpdir } from 'node:os';
import { spawn } from 'node:child_process';
import { createServer } from 'vite';
import vue from '@vitejs/plugin-vue';
const root=resolve(import.meta.dirname,'..'),delay=ms=>new Promise(r=>setTimeout(r,ms));
class CDP{
 constructor(socket){this.socket=socket;this.id=0;this.pending=new Map();this.errors=[];socket.addEventListener('message',event=>{const r=JSON.parse(event.data);if(r.id){const p=this.pending.get(r.id);this.pending.delete(r.id);r.error?p.reject(Error(r.error.message)):p.resolve(r.result);}if(r.method==='Runtime.exceptionThrown')this.errors.push(r.params.exceptionDetails);});}
 send(method,params={}){return new Promise((resolve,reject)=>{const id=++this.id;this.pending.set(id,{resolve,reject});this.socket.send(JSON.stringify({id,method,params}));});}
 async eval(expression){const r=await this.send('Runtime.evaluate',{expression,awaitPromise:true,returnByValue:true});if(r.exceptionDetails)throw Error(r.exceptionDetails.exception?.description||r.exceptionDetails.text);return r.result.value;}
 async wait(e){for(let i=0;i<600;i++){if(await this.eval(e))return;await delay(25);}throw Error('Timed out: '+e);}
}
const clients=['web_desktop','web_mobile','pc','android','ios'];
function matrix(value){return Object.fromEntries(['lan','internet'].map(z=>[z,Object.fromEntries(clients.map(c=>[c,{login:value,save:value}]))]));}

test('real Arco account security and transport policy forms', {timeout:120000}, async t=>{
 const requests=[];let locked=true,mustChange=false,rejectSave=false,slow=false;
 const fixture={name:'security-fixture',enforce:'pre',resolveId(id){if(id==='/fixture.ts')return '\0fixture';if(id==='@/utils/http'||id.replaceAll('\\','/').endsWith('/src/utils/http'))return '\0http';},
 load(id){if(id==='\0http')return `async function call(method,{url,params={}}){const q=method==='GET'?'?'+new URLSearchParams(params):'';const r=await fetch(url+q,{method,headers:{'Content-Type':'application/json'},body:method==='GET'?undefined:JSON.stringify(params)});const b=await r.json();if(b.code)throw Error('Synthetic rejection');return b.data}export const defHttp={get:a=>call('GET',a),post:a=>call('POST',a)};`;
 if(id==='\0fixture')return `import {createApp,h,ref} from 'vue';import Arco from '@arco-design/web-vue';import '@arco-design/web-vue/dist/arco.css';import Policy from '/src/views/system/account/AccessPolicyModal.vue';import Security from '/src/views/system/account/AccountSecurityModal.vue';import Lock from '/src/views/system/account/LockPolicyModal.vue';import Required from '/src/views/login/components/RequiredPasswordModal.vue';const app=createApp({setup(){const p=ref(),s=ref(),l=ref(),r=ref();window.openPolicy=(id=7)=>p.value.open(id,'synthetic '+id,'person');window.openSecurity=()=>s.value.open(7,'synthetic');window.openLock=()=>l.value.open();window.openRequired=()=>r.value.open({changeTicket:'a'.repeat(64),expiresAt:Math.floor(Date.now()/1000)+300,reasonCode:'PASSWORD_CHANGE_REQUIRED',requiresPasswordChange:true});window.changed=0;return()=>h('main',[h(Policy,{ref:p}),h(Security,{ref:s}),h(Lock,{ref:l}),h(Required,{ref:r,onSuccess:()=>window.changed++})])}});app.directive('permission',()=>{});app.use(Arco).mount('#app');`;
 },configureServer(server){server.middlewares.use(async(req,res,next)=>{
  const u=new URL(req.url,'http://localhost');if(u.pathname==='/fixture'){res.setHeader('Content-Type','text/html');res.end('<!doctype html><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><div id="app"></div><script type="module" src="/fixture.ts"></script>');return;}
  if(!u.pathname.startsWith('/system/')&&!u.pathname.startsWith('/user/effectivePolicy')&&!u.pathname.startsWith('/common/im-sso/password/'))return next();
  let data='';for await(const part of req)data+=part;const input=req.method==='GET'?Object.fromEntries(u.searchParams):JSON.parse(data||'{}');requests.push({method:req.method,path:u.pathname,input});res.setHeader('Content-Type','application/json');let output={};let code=0;
  if(u.pathname.endsWith('/accessPolicy')){if(req.method==='GET'){if(slow&&input.id==='7')await delay(300);const effective=Object.fromEntries(['lan','internet'].map(z=>[z,Object.fromEntries(clients.map(c=>[c,{zone:z,logicalClient:c,login:false,preview:false,save:false,sources:{login:'roles',save:'roles'}}]))]));output={subject:'person',subjectID:Number(input.id),revision:3,matrix:matrix(input.id==='8'?'allow':'inherit'),effective};}else if(rejectSave)code=409;}
  else if(u.pathname==='/user/effectivePolicy')output={zone:'lan',logicalClient:'web_desktop',login:true,preview:true,save:true,currentIP:'192.168.10.42',policyRevision:3,networkRevision:1,securityVersion:1,sources:{login:'default',save:'default'}};
  else if(u.pathname.endsWith('/security'))output={uid:7,locked,lockedUntil:Math.floor(Date.now()/1000)+1800,mustChangePassword:mustChange,recentFailures:locked?5:0,securityVersion:1};
  else if(u.pathname.endsWith('/unlock'))locked=false;
  else if(u.pathname.endsWith('/resetPassword'))mustChange=true;
  else if(u.pathname.endsWith('/lockPolicy'))output={windowSeconds:600,failures:5,lockSeconds:1800,revision:2};
  else if(u.pathname.endsWith('/password/change'))output={changed:true,requiresLogin:true};
  res.end(JSON.stringify({code,data:output}));
 });}};
 const server=await createServer({root,configFile:false,plugins:[fixture,vue()],resolve:{alias:{'@':join(root,'src'),'/@':join(root,'src')}},optimizeDeps:{noDiscovery:true,include:['vue','@arco-design/web-vue','md5']},server:{host:'127.0.0.1',port:0},logLevel:'error'});await server.listen();t.after(()=>server.close());
 const origin=`http://127.0.0.1:${server.httpServer.address().port}`;const browser=['C:/Program Files/Google/Chrome/Application/chrome.exe','C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe'].find(existsSync);assert.ok(browser);
 const dir=await mkdtemp(join(tmpdir(),'account-security-'));const child=spawn(browser,['--headless=new','--disable-gpu','--no-first-run','--remote-debugging-port=0',`--user-data-dir=${dir}`,'about:blank'],{stdio:'ignore',windowsHide:true});t.after(()=>child.kill());let port;
 for(let i=0;i<200;i++){try{port=Number((await readFile(join(dir,'DevToolsActivePort'),'utf8')).split('\n')[0]);break;}catch{await delay(50);}}
 const target=await(await fetch(`http://127.0.0.1:${port}/json/new?${encodeURIComponent(origin+'/fixture')}`,{method:'PUT'})).json();const socket=new WebSocket(target.webSocketDebuggerUrl);await new Promise(r=>socket.addEventListener('open',r,{once:true}));t.after(()=>socket.close());const cdp=new CDP(socket);await cdp.send('Runtime.enable');await cdp.wait('typeof window.openPolicy==="function"');await cdp.eval("window.shown=selector=>[...document.querySelectorAll(selector)].filter(e=>e.getClientRects().length>0)");
 const clickText=text=>cdp.eval(`window.shown('button').find(e=>e.textContent.trim()===${JSON.stringify(text)}).click()`);
 const password=(index,value)=>cdp.eval(`(()=>{const e=window.shown('.arco-modal input[type=password]')[${index}];Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value').set.call(e,${JSON.stringify(value)});e.dispatchEvent(new Event('input',{bubbles:true}));})()`);
 await t.test('matrix presents all five clients and saves only the editable matrix and revision',async()=>{
  await cdp.eval('window.openPolicy(7)');await cdp.wait("window.shown('.policy-table select,.policy-table .arco-select-view').length>=10");
  const text=await cdp.eval('document.body.innerText');for(const name of ['电脑网页','手机网页','PC 客户端','Android','iOS','192.168.10.42'])assert.ok(text.includes(name));
  await cdp.eval("window.shown('.policy-table .arco-select-view')[0].click()");await cdp.wait("window.shown('.arco-select-option').length>=3");await cdp.eval("window.shown('.arco-select-option').find(e=>e.textContent.trim()==='允许').click()");await clickText('确定');await cdp.wait("window.shown('.arco-modal').length===0");
  const r=requests.find(r=>r.method==='POST'&&r.path.endsWith('/accessPolicy'));assert.deepEqual(Object.keys(r.input).sort(),['id','matrix','revision']);assert.equal(r.input.revision,3);assert.equal(r.input.matrix.lan.web_desktop.login,'allow');
 });
 await t.test('rejected saves keep the dialog and stale load cannot replace the next person',async()=>{
  rejectSave=true;await cdp.eval('window.openPolicy(7)');await cdp.wait("window.shown('.policy-table').length>0");await clickText('确定');await delay(100);assert.equal(await cdp.eval("window.shown('.arco-modal').length>0"),true);await clickText('取消');rejectSave=false;
  slow=true;await cdp.eval('window.openPolicy(7);window.openPolicy(8)');await delay(450);assert.equal(await cdp.eval("window.shown('.arco-modal-title')[0].textContent.includes('synthetic 8')"),true);assert.equal(await cdp.eval("window.shown('.policy-table .arco-select-view')[0].textContent.includes('允许')"),true);await clickText('取消');await cdp.wait("window.shown('.arco-modal').length===0");slow=false;
 });
 await t.test('reset requires confirmation and sends a digest without changing other account fields',async()=>{
  await cdp.eval('window.openSecurity()');await cdp.wait("document.body.innerText.includes('临时密码')");await password(0,'Synthetic-only-new-password');await password(1,'mismatch');await clickText('重置并要求首次修改');assert.equal(requests.filter(r=>r.path.endsWith('/resetPassword')).length,0);
  await password(1,'Synthetic-only-new-password');await clickText('重置并要求首次修改');await cdp.wait("document.body.innerText.includes('必须修改')");const r=requests.find(r=>r.path.endsWith('/resetPassword'));assert.deepEqual(Object.keys(r.input).sort(),['id','password']);assert.match(r.input.password,/^[a-f0-9]{32}$/);assert.equal(await cdp.eval("[...document.querySelectorAll('.arco-modal input[type=password]')].every(e=>!e.value)"),true);
  await cdp.eval("window.shown('.arco-modal-close-btn')[0].click()");await cdp.wait("window.shown('.arco-modal').length===0");
 });
 await t.test('default lock policy is ten minutes five failures thirty minutes',async()=>{
  await cdp.eval('window.openLock()');await cdp.wait("document.body.innerText.includes('统计窗口')");assert.deepEqual(await cdp.eval("[...window.shown('.arco-modal input')].map(e=>e.value)"),['10','5','30']);await clickText('确定');await cdp.wait("window.shown('.arco-modal').length===0");const r=requests.find(r=>r.method==='POST'&&r.path.endsWith('/lockPolicy'));assert.deepEqual(r.input,{windowSeconds:600,failures:5,lockSeconds:1800,revision:2});
 });
 await t.test('forced change is ticket-only and ends with a fresh login requirement',async()=>{
  await cdp.eval('window.openRequired()');await cdp.wait("document.body.innerText.includes('尚未登录')");await password(0,'Synthetic-personal-new-password');await password(1,'Synthetic-personal-new-password');await clickText('确定');await cdp.wait('window.changed===1');const r=requests.find(r=>r.path.endsWith('/password/change'));assert.deepEqual(Object.keys(r.input).sort(),['changeTicket','newPassword']);assert.equal(r.input.changeTicket,'a'.repeat(64));assert.match(r.input.newPassword,/^[a-f0-9]{32}$/);assert.equal(await cdp.eval("Object.values(localStorage).some(x=>x.includes('a'.repeat(64)))"),false);
 });
 await cdp.send('Emulation.setDeviceMetricsOverride',{width:375,height:850,deviceScaleFactor:1,mobile:true});await cdp.eval('window.openPolicy(7)');await cdp.wait("window.shown('.policy-table').length>0");
 await cdp.wait("window.shown('.arco-message').length===0");assert.equal(await cdp.eval('document.documentElement.scrollWidth<=375'),true);
 await mkdir(resolve(root,'../dist/qa'),{recursive:true});const shot=await cdp.send('Page.captureScreenshot',{format:'png'});await writeFile(resolve(root,'../dist/qa/account-security-375.png'),Buffer.from(shot.data,'base64'));
 assert.deepEqual(cdp.errors,[]);
});
