import {mkdirSync} from "node:fs";
import {resolve as tempResolve} from "node:path";
const taskTemp=tempResolve("../../.cache/temp-web");mkdirSync(taskTemp,{recursive:true});process.env.TEMP=taskTemp;process.env.TMP=taskTemp;
import {chromium} from 'playwright';
import {spawn} from 'node:child_process';
import {resolve} from 'node:path';
import {createHash} from 'node:crypto';
import {readFile,writeFile,mkdir} from 'node:fs/promises';
const secrets=JSON.parse(await readFile('../../.private/test-accounts.json','utf8')), [a,b]=secrets.users.slice(101,103),password=secrets.password;
const base='https://192.168.10.11:29880', failures=[], calls=[];await mkdir('test-results',{recursive:true});
const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE});
console.log('browser ready'); const page=await browser.newPage();page.setDefaultTimeout(30000);console.log('page ready');page.on('pageerror',e=>failures.push({client:'web',error:e.message}));
page.on('requestfailed',r=>failures.push({client:'web-network',url:new URL(r.url()).origin+new URL(r.url()).pathname,error:r.failure()?.errorText}));
page.on('response',async response=>{if(response.url().startsWith(base) && !response.url().includes('/ws')){const body=await response.json().catch(()=>({}));calls.push({path:new URL(response.url()).pathname,status:response.status(),errCode:body.errCode,errMsg:body.errMsg});}});
await page.addInitScript(({base})=>{for(const [key,value]of Object.entries({apiUrl:base+'/api',chatUrl:base+'/chat',enterpriseUrl:base+'/enterprise/v1',wsUrl:base.replace('https:','wss:')+'/ws'}))localStorage.setItem('upgrade38:'+key,value);},{base});
const child=spawn(resolve('release/win-unpacked/OpenIM38 Enterprise.exe'),['--disable-background-timer-throttling','--disable-renderer-backgrounding','--disable-backgrounding-occluded-windows','--remote-debugging-port=17782'],{windowsHide:true,env:{...process.env,OPENIM38_SMOKE_TEST:'1',OPENIM38_TEST_DATA_DIR:resolve('../../.cache/electron-policy-client-'+Date.now())},stdio:['ignore','pipe','pipe']});
let ws,id=0;const pending=new Map(),nativeLogs=[];child.stdout.on('data',c=>nativeLogs.push(c.toString()));child.stderr.on('data',c=>nativeLogs.push(c.toString()));
const sleep=ms=>new Promise(r=>setTimeout(r,ms));
let evaluate,command;
try{
 console.log('goto');await page.goto('http://127.0.0.1:7777',{waitUntil:'domcontentloaded',timeout:60000});console.log('goto ready',page.url());await page.waitForFunction(()=>typeof window.commonEventFunc==='function',undefined,{timeout:120000});
 console.log('wasm ready');await page.getByRole('tab',{name:'邮箱',exact:true}).click();await page.locator('#email').fill(a.email);await page.locator('#password').fill(password);await page.getByRole('button',{name:'登录',exact:true}).click();
 console.log('submitted');await page.waitForURL('**/#/chat**',{timeout:90000});console.log('web chat ready');await page.waitForTimeout(3000);
 let target;for(let i=0;i<240;i++){try{target=(await(await fetch('http://127.0.0.1:17782/json/list')).json()).find(t=>t.url.startsWith('openim38://app/'));if(target)break;}catch{}await sleep(250);}if(!target)throw new Error('Native target unavailable');
 ws=new WebSocket(target.webSocketDebuggerUrl);await new Promise((r,j)=>{ws.onopen=r;ws.onerror=j;});ws.onmessage=e=>{const m=JSON.parse(e.data);if(m.id){const p=pending.get(m.id);if(p){pending.delete(m.id);m.error?p.reject(new Error(JSON.stringify(m.error))):p.resolve(m.result);}}else if(m.method==='Runtime.exceptionThrown')failures.push({client:'windows',error:m.params.exceptionDetails.text});};
 command=(method,params={})=>new Promise((r,j)=>{const n=++id;pending.set(n,{resolve:r,reject:j});ws.send(JSON.stringify({id:n,method,params}));setTimeout(()=>{if(pending.delete(n))j(new Error('CDP timeout '+method));},120000).unref();});
 evaluate=async expression=>{const r=await command('Runtime.evaluate',{expression,awaitPromise:true,returnByValue:true});if(r.exceptionDetails)throw new Error(JSON.stringify(r.exceptionDetails));return r.result.value;};await command('Runtime.enable');
 for(let i=0;i<120;i++){if(await evaluate('typeof window.commonEventFunc==="function" && (location.hash.startsWith("#/chat") || Boolean(document.querySelector("#password")))'))break;await sleep(500);}
 console.log('native ready');if(!await evaluate('location.hash.startsWith("#/chat")')){
 await evaluate(`(()=>{[...document.querySelectorAll('[role=tab]')].find(e=>e.textContent==='邮箱').click();})()`);await sleep(500);
 await evaluate(`(()=>{const set=(id,value)=>{const el=document.getElementById(id);Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value').set.call(el,value);el.dispatchEvent(new Event('input',{bubbles:true}));};set('email',${JSON.stringify(a.email)});set('password',${JSON.stringify(password)});document.querySelector('button[type=submit]').click();})()`);
 }
 let logged=false;for(let i=0;i<120;i++){logged=await evaluate('location.hash.startsWith("#/chat")');if(logged)break;await sleep(500);}if(!logged)throw new Error('Native login failed '+await evaluate('document.body.innerText'));

 await sleep(4000);console.log('native-core-summary',await evaluate(`(async()=>{const parse=v=>typeof v==='string'?JSON.parse(v):v;const status=parse(await window.getLoginStatus(crypto.randomUUID()));const conv=parse(await window.getConversationListSplit(crypto.randomUUID(),0,100));return {status,conversationError:conv.errCode,conversationCount:conv.data?.length}})()`));const coexist={web:page.url(),windows:await evaluate('location.href')};if(!coexist.web.includes('/chat')||!coexist.windows.includes('/chat'))throw new Error('Windows/Web coexistence failed');
 const previous=await page.evaluate(async()=>{const s=await import('/src/utils/storage.ts');return{imToken:await s.getIMToken(),chatToken:await s.getChatToken()};});
 const second=await browser.newPage();second.setDefaultTimeout(30000);await second.addInitScript(({base})=>{for(const[k,v]of Object.entries({apiUrl:base+'/api',chatUrl:base+'/chat',enterpriseUrl:base+'/enterprise/v1',wsUrl:base.replace('https:','wss:')+'/ws'}))localStorage.setItem('upgrade38:'+k,v);},{base});await second.goto('http://127.0.0.1:7777',{waitUntil:'domcontentloaded'});await second.waitForFunction(()=>typeof window.commonEventFunc==='function',undefined,{timeout:90000});await second.getByRole('tab',{name:'邮箱',exact:true}).click();await second.locator('#email').fill(a.email);await second.locator('#password').fill(password);await second.getByRole('button',{name:'登录',exact:true}).click();await second.waitForURL('**/#/chat**',{timeout:90000});await page.waitForURL('**/#/login',{timeout:45000});await sleep(5000);
 const oldEnterprise=await second.evaluate(async({base,previous})=>{const r=await fetch(base+'/enterprise/v1/rtc/events?after=-1',{headers:{token:previous.imToken,chatToken:previous.chatToken,platformID:'5'}});return r.status;},{base,previous});if(oldEnterprise!==401)throw new Error('Kicked pair still authorized');
 const other=secrets.users[102],marker='Policy4 cross-platform sender sync '+Date.now();
 await evaluate(`(()=>{[...document.querySelectorAll('div[class*="conversation-item"]')].find(el=>el.textContent.includes(${JSON.stringify(other.nickname)}))?.click();})()`);await sleep(2000);
 await second.getByText(other.nickname,{exact:true}).last().click();await second.locator('#editable-div').fill(marker);await second.locator('#editable-div').press('Enter');let received=false;for(let i=0;i<90;i++){received=await evaluate(`document.body.innerText.includes(${JSON.stringify(marker)})`);if(received)break;await sleep(500);}if(!received){await writeFile('test-results/policy-native-failure.txt',await evaluate('document.body.innerText'));throw new Error('Windows lost sender sync after Web replaced Web');}
 const summary={windowsAndWebCoexist:true,oldWebKickedToLogin:page.url().endsWith('/login'),oldEnterpriseStatus:oldEnterprise,windowsStillLoggedIn:(await evaluate('location.href')).includes('/chat'),windowsReceivedSenderSync:received,failures};await writeFile('test-results/policy-client.json',JSON.stringify(summary,null,2));console.log(JSON.stringify(summary));
}finally{ws?.close();child.kill();await browser.close();}
