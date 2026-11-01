import {mkdirSync} from "node:fs";
import {resolve as tempResolve} from "node:path";
const taskTemp=tempResolve("../../.cache/temp-web");mkdirSync(taskTemp,{recursive:true});process.env.TEMP=taskTemp;process.env.TMP=taskTemp;
import {chromium} from 'playwright';
import {spawn} from 'node:child_process';
import {resolve} from 'node:path';
import {createHash} from 'node:crypto';
import {readFile,writeFile,mkdir} from 'node:fs/promises';
const secrets=JSON.parse(await readFile('../../.private/test-accounts.json','utf8')), [a,b]=secrets.users.slice(100,102),password=secrets.password;
const base='https://192.168.10.11:29880', failures=[], calls=[];await mkdir('test-results',{recursive:true});
const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE});
console.log('browser ready'); const page=await browser.newPage();page.setDefaultTimeout(30000);console.log('page ready');page.on('pageerror',e=>failures.push({client:'web',error:e.message}));
page.on('requestfailed',r=>failures.push({client:'web-network',url:new URL(r.url()).origin+new URL(r.url()).pathname,error:r.failure()?.errorText}));
page.on('response',async response=>{if(response.url().startsWith(base) && !response.url().includes('/ws')){const body=await response.json().catch(()=>({}));calls.push({path:new URL(response.url()).pathname,status:response.status(),errCode:body.errCode,errMsg:body.errMsg});}});
await page.addInitScript(({base})=>{for(const [key,value]of Object.entries({apiUrl:base+'/api',chatUrl:base+'/chat',enterpriseUrl:base+'/enterprise/v1',wsUrl:base.replace('https:','wss:')+'/ws'}))localStorage.setItem('upgrade38:'+key,value);},{base});
const child=spawn(resolve('release/win-unpacked/OpenIM38 Enterprise.exe'),['--remote-debugging-port=17780'],{windowsHide:true,env:{...process.env,OPENIM38_SMOKE_TEST:'1',OPENIM38_TEST_DATA_DIR:resolve('../../.cache/electron-cross-client')},stdio:['ignore','pipe','pipe']});
let ws,id=0;const pending=new Map(),nativeLogs=[];child.stdout.on('data',c=>nativeLogs.push(c.toString()));child.stderr.on('data',c=>nativeLogs.push(c.toString()));
const sleep=ms=>new Promise(r=>setTimeout(r,ms));
let evaluate,command;
try{
 console.log('goto');await page.goto('http://127.0.0.1:7777',{waitUntil:'domcontentloaded',timeout:60000});console.log('goto ready',page.url());await page.waitForFunction(()=>typeof window.commonEventFunc==='function',undefined,{timeout:120000});
 console.log('wasm ready');await page.getByRole('tab',{name:'邮箱',exact:true}).click();await page.locator('#email').fill(a.email);await page.locator('#password').fill(password);await page.getByRole('button',{name:'登录',exact:true}).click();
 console.log('submitted');await page.waitForURL('**/#/chat**',{timeout:90000});console.log('web chat ready');await page.waitForTimeout(3000);
 let target;for(let i=0;i<240;i++){try{target=(await(await fetch('http://127.0.0.1:17780/json/list')).json()).find(t=>t.url.startsWith('openim38://app/'));if(target)break;}catch{}await sleep(250);}if(!target)throw new Error('Native target unavailable');
 ws=new WebSocket(target.webSocketDebuggerUrl);await new Promise((r,j)=>{ws.onopen=r;ws.onerror=j;});ws.onmessage=e=>{const m=JSON.parse(e.data);if(m.id){const p=pending.get(m.id);if(p){pending.delete(m.id);m.error?p.reject(new Error(JSON.stringify(m.error))):p.resolve(m.result);}}else if(m.method==='Runtime.exceptionThrown')failures.push({client:'windows',error:m.params.exceptionDetails.text});};
 command=(method,params={})=>new Promise((r,j)=>{const n=++id;pending.set(n,{resolve:r,reject:j});ws.send(JSON.stringify({id:n,method,params}));setTimeout(()=>{if(pending.delete(n))j(new Error('CDP timeout '+method));},120000).unref();});
 evaluate=async expression=>{const r=await command('Runtime.evaluate',{expression,awaitPromise:true,returnByValue:true});if(r.exceptionDetails)throw new Error(JSON.stringify(r.exceptionDetails));return r.result.value;};await command('Runtime.enable');
 for(let i=0;i<120;i++){if(await evaluate('typeof window.commonEventFunc==="function" && (location.hash.startsWith("#/chat") || Boolean(document.querySelector("#password")))'))break;await sleep(500);}
 console.log('native ready');if(!await evaluate('location.hash.startsWith("#/chat")')){
 await evaluate(`(()=>{[...document.querySelectorAll('[role=tab]')].find(e=>e.textContent==='邮箱').click();})()`);await sleep(500);
 await evaluate(`(()=>{const set=(id,value)=>{const el=document.getElementById(id);Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value').set.call(el,value);el.dispatchEvent(new Event('input',{bubbles:true}));};set('email',${JSON.stringify(b.email)});set('password',${JSON.stringify(password)});document.querySelector('button[type=submit]').click();})()`);
 }
 let logged=false;for(let i=0;i<120;i++){logged=await evaluate('location.hash.startsWith("#/chat")');if(logged)break;await sleep(500);}if(!logged)throw new Error('Native login failed '+await evaluate('document.body.innerText'));
 await sleep(5000);console.log('both logged in');
 console.log(JSON.stringify({webURL:page.url(),webBody:(await page.locator('body').innerText()).slice(0,1600),failures,calls}));await page.screenshot({path:'test-results/cross-web-before.png'});
 await page.getByText(b.nickname,{exact:true}).last().click();await page.locator('#editable-div').waitFor();
 const forwardText='OpenIM38 cross-client Web→Windows '+Date.now(),backText='OpenIM38 cross-client Windows→Web '+Date.now();
 await page.locator('#editable-div').fill(forwardText);await page.locator('#editable-div').press('Enter');
 await evaluate(`(()=>{const el=[...document.querySelectorAll('span')].find(e=>e.textContent===${JSON.stringify(a.nickname)});if(!el)throw new Error('Conversation not found');el.click();})()`);
 let received=false;for(let i=0;i<90;i++){received=await evaluate(`document.body.innerText.includes(${JSON.stringify(forwardText)})`);if(received)break;await sleep(500);}if(!received)throw new Error('Windows did not render Web message');
 await evaluate(`(()=>{const el=document.getElementById('editable-div');el.focus();el.innerHTML=${JSON.stringify(backText)};el.dispatchEvent(new InputEvent('input',{bubbles:true,inputType:'insertText',data:${JSON.stringify(backText)}}));})()`);await sleep(300);
 await evaluate(`(()=>{[...document.querySelectorAll('button')].find(e=>e.textContent==='发送').click();})()`);await page.locator('#scrollableDiv').getByText(backText,{exact:true}).waitFor({timeout:60000});
 console.log('cross messages received');
 const fileName='openim38-attachment-'+Date.now()+'.txt',fileContent='OpenIM38 official WASM SDK attachment integrity '+Date.now();
 await page.locator('input[type=file][accept="*"]').setInputFiles({name:fileName,mimeType:'text/plain',buffer:Buffer.from(fileContent)});
 let fileSeen=false;for(let i=0;i<120;i++){fileSeen=await evaluate(`document.body.innerText.includes(${JSON.stringify(fileName)})`);if(fileSeen)break;await sleep(500);}if(!fileSeen)throw new Error('Windows did not render attachment');
 const attachment=await page.evaluate(async({fileName,fileContent})=>{const {useMessageStore}=await import('/src/store/index.ts');for(let i=0;i<120;i++){const message=useMessageStore.getState().historyMessageList.find(m=>m.fileElem?.fileName===fileName && m.fileElem?.sourceUrl);if(message){const address=new URL(message.fileElem.sourceUrl);try{const response=await fetch(message.fileElem.sourceUrl);const data=await response.arrayBuffer();const sha256=[...new Uint8Array(await crypto.subtle.digest('SHA-256',data))].map(n=>n.toString(16).padStart(2,'0')).join('');return {source:address.origin+address.pathname,status:response.status,integrity:new TextDecoder().decode(data)===fileContent,sha256,clientMsgID:message.clientMsgID};}catch(error){return {source:address.origin+address.pathname,error:String(error),integrity:false};}}await new Promise(r=>setTimeout(r,500));}throw new Error('Attachment send did not finish');},{fileName,fileContent});if(!attachment.integrity)throw new Error('Attachment integrity mismatch '+JSON.stringify(attachment));console.log('attachment verified');
 await evaluate('window.testDownloaded=null;window.electronAPI.subscribe("downloadSuccess",(url,path)=>window.testDownloaded=path)');
 await evaluate(`(()=>{const el=[...document.querySelectorAll('[data-drag="app-drag"]')].find(e=>e.textContent===${JSON.stringify(fileName)});if(!el)throw new Error('File download control not found');el.click();})()`);
 let downloaded;for(let i=0;i<120;i++){downloaded=await evaluate('window.testDownloaded');if(downloaded)break;await sleep(500);}if(!downloaded)throw new Error('Windows managed-cache download failed');
 const nativeFile=await evaluate(`(async()=>{const f=await window.electronAPI.getFileByPath(${JSON.stringify(downloaded)});const data=await f.arrayBuffer();return {name:f.name,bytes:f.size,sha256:[...new Uint8Array(await crypto.subtle.digest('SHA-256',data))].map(n=>n.toString(16).padStart(2,'0')).join('')};})()`);if(nativeFile.sha256!==attachment.sha256)throw new Error('Windows cached file hash mismatch');
 const media=[];
 for(const fixture of [{name:'synthetic.png',type:102,selector:'input[type=file][accept="image/*"]',key:'pictureElem'},{name:'synthetic.mp4',type:104,selector:'input[type=file][accept=".mp4"]',key:'videoElem'}]){
  const before=await page.evaluate(async()=>{const{useMessageStore}=await import('/src/store/index.ts');return useMessageStore.getState().historyMessageList.map(m=>m.clientMsgID);});
  await page.locator(fixture.selector).setInputFiles(resolve('../../.cache/media-fixtures',fixture.name));
  let message;for(let attempt=0;attempt<180;attempt++){message=await page.evaluate(async({before,type})=>{const{useMessageStore}=await import('/src/store/index.ts');return useMessageStore.getState().historyMessageList.find(m=>m.contentType===type && !before.includes(m.clientMsgID) && m.status===2) || null;},{before,type:fixture.type});if(message?.[fixture.key])break;await sleep(500);}if(!message?.[fixture.key])throw new Error('Media upload did not finish '+fixture.name);
  const url=fixture.type===102?message.pictureElem.sourcePicture.url:message.videoElem.videoUrl;const expected=createHash('sha256').update(await readFile(resolve('../../.cache/media-fixtures',fixture.name))).digest('hex');
  const result=await evaluate(`(async()=>{const response=await fetch(${JSON.stringify(url)});const data=await response.arrayBuffer();const sha256=[...new Uint8Array(await crypto.subtle.digest('SHA-256',data))].map(n=>n.toString(16).padStart(2,'0')).join('');return {status:response.status,size:data.byteLength,sha256};})()`);if(result.sha256!==expected)throw new Error('Windows media hash mismatch '+fixture.name);
  let visual;for(let i=0;i<120;i++){visual=await evaluate(`Boolean(document.getElementById(${JSON.stringify('chat_'+message.clientMsgID)}))`);if(visual)break;await sleep(500);}if(!visual)throw new Error('Media message missing from native conversation');
  if(fixture.type===104)result.playback=await evaluate(`new Promise((resolve,reject)=>{const v=document.createElement('video');v.muted=true;v.src=${JSON.stringify(url)};document.body.append(v);v.oncanplay=async()=>{try{await v.play();resolve({width:v.videoWidth,height:v.videoHeight,duration:v.duration});v.pause();v.remove();}catch(e){reject(String(e));}};v.onerror=()=>reject('Video decode '+v.error?.code);setTimeout(()=>reject('Video timeout'),30000);})`);
  media.push({name:fixture.name,contentType:fixture.type,...result});
 }
 console.log('Windows image/video integrity and H264 AAC decode verified');


 const summary={phase:'cross-text',textMessages:{webToWindows:true,windowsToWeb:true},attachment,nativeFile,media,webURL:page.url(),windowsURL:await evaluate('location.href'),webText:(await page.locator('body').innerText()).slice(0,600),windowsText:(await evaluate('document.body.innerText')).slice(0,600),failures,calls};
 await page.screenshot({path:'test-results/cross-web.png'});await writeFile('test-results/cross-windows.png',Buffer.from((await command('Page.captureScreenshot',{format:'png'})).data,'base64'));
 await writeFile('test-results/cross-client.json',JSON.stringify(summary,null,2));console.log(JSON.stringify(summary));
}finally{await writeFile('test-results/cross-client-last-state.json',JSON.stringify({url:page.url(),failures,calls,body:(await page.locator('body').innerText().catch(()=>'' )).slice(0,2000)},null,2));if(!evaluate)await writeFile('test-results/cross-client-failure.json',JSON.stringify({failures,calls,body:(await page.locator('body').innerText().catch(()=>'' )).slice(0,1000)},null,2));ws?.close();child.kill();await browser.close();await writeFile('test-results/cross-native.log',nativeLogs.join(''));}
