import {mkdirSync} from 'node:fs';
import {resolve} from 'node:path';
import {mkdir,readFile,writeFile} from 'node:fs/promises';
import {chromium} from 'playwright';
const temp=resolve('../../.cache/temp-web');mkdirSync(temp,{recursive:true});process.env.TEMP=temp;process.env.TMP=temp;
const secrets=JSON.parse(await readFile('../../.private/test-accounts.json','utf8')),user=secrets.users[105],base='https://192.168.10.11:29880';
const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE});
const context=await browser.newContext();const page=await context.newPage(),errors=[];
page.on('pageerror',e=>errors.push(e.message));
await page.addInitScript(({base})=>{const Native=window.WebSocket;window.__testSockets=[];window.WebSocket=class extends Native{constructor(...args){super(...args);window.__testSockets.push(this)}};for(const[k,v]of Object.entries({apiUrl:base+'/api',chatUrl:base+'/chat',enterpriseUrl:base+'/enterprise/v1',wsUrl:base.replace('https:','wss:')+'/ws'}))localStorage.setItem('upgrade38:'+k,v);},{base});
const login=async()=>{await page.getByRole('tab',{name:'邮箱',exact:true}).click();await page.locator('#email').fill(user.email);await page.locator('#password').fill(secrets.password);await page.getByRole('button',{name:'登录',exact:true}).click();await page.waitForURL('**/#/chat**',{timeout:90000});await page.waitForTimeout(3500);};
const pair=()=>page.evaluate(async()=>{const s=await import('/src/utils/storage.ts');return{imToken:await s.getIMToken(),chatToken:await s.getChatToken()};});
const check=async(pair)=>page.evaluate(async({base,pair,userID})=>{const result={};for(const[name,path,body,token]of[['im','/api/user/get_users_info',{userIDs:[userID]},pair.imToken],['chat','/chat/user/find/full',{userIDs:[userID]},pair.chatToken],['enterprise','/enterprise/v1/rtc/events?after=-1',undefined,pair.imToken]]){const response=await fetch(base+path,{method:body?'POST':'GET',headers:{'Content-Type':'application/json',operationID:crypto.randomUUID(),token,chatToken:pair.chatToken,platformID:'5'},body:body?JSON.stringify(body):undefined});const data=await response.json();result[name]={status:response.status,errCode:data.errCode,errMsg:data.errMsg};}return result;},{base,pair,userID:user.userID});
try{
 await page.goto('http://127.0.0.1:7777',{waitUntil:'domcontentloaded'});await page.waitForFunction(()=>typeof window.commonEventFunc==='function',undefined,{timeout:120000});await login();console.log('logout fixture ready');
 const previous=await pair();await page.route('**/enterprise/v1/session/logout',route=>route.abort('internetdisconnected'));
 const failedLogout=await page.evaluate(async()=>{try{await(await import('/src/store/user.ts')).useUserStore.getState().userLogout();return false;}catch{return true;}});
 const retained=await page.evaluate(async()=>{const s=await import('/src/utils/storage.ts');return Boolean(await s.getIMToken())&&location.hash.startsWith('#/chat');});await page.unroute('**/enterprise/v1/session/logout');if(!failedLogout||!retained)throw new Error('Failed logout discarded local session');
 const socketCount=await page.evaluate(()=>window.__testSockets.filter(s=>s.url.includes(':29880/ws')&&s.readyState===1).length);if(!socketCount)throw new Error('No actual live WebSocket before logout');
 await page.locator('aside .cursor-pointer').first().click();await page.getByText('退出登录',{exact:true}).last().click();await page.getByRole('button',{name:'确定',exact:true}).click();await page.waitForURL('**/#/login',{timeout:45000});
 await page.waitForFunction(()=>window.__testSockets.filter(s=>s.url.includes(':29880/ws')).every(s=>s.readyState!==1));
 const socketsClosed=await page.evaluate(()=>window.__testSockets.filter(s=>s.url.includes(':29880/ws')).every(s=>s.readyState!==1));if(!socketsClosed)throw new Error('Old WebSocket still open');
 const old=await check(previous);if(Object.values(old).some(v=>v.status<400&&v.errCode===0))throw new Error('Old logged-out pair accepted '+JSON.stringify(old));console.log('old pair revoked; socket closed');
 await login();const current=await pair();const replay=await page.evaluate(async({base,pair})=>{const response=await fetch(base+'/enterprise/v1/session/logout',{method:'POST',headers:{'Content-Type':'application/json',token:pair.imToken,chatToken:pair.chatToken,platformID:'5'},body:'{}'});const data=await response.json();return{status:response.status,errCode:data.errCode,revoked:data.data?.revoked};},{base,pair:previous});
 const fresh=await check(current);if(Object.values(fresh).some(v=>v.status>=400||v.errCode!==0))throw new Error('Fresh same-platform pair check failed '+JSON.stringify({fresh,replay}));if(replay.status!==200||replay.errCode!==0||!replay.revoked)throw new Error('Repeated logout not idempotent');
 const summary={failedLogoutRetainsSession:failedLogout&&retained,webSocketCount:socketCount,oldSocketsClosed:socketsClosed,loggedOutPair:old,repeatedOldPairLogout:replay,freshSamePlatformPair:fresh,errors};await mkdir('test-results',{recursive:true});await writeFile('test-results/session-logout.json',JSON.stringify(summary,null,2));console.log(JSON.stringify(summary));
}finally{await browser.close();}
