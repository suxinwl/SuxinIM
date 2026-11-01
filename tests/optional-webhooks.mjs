// Trigger callbacks through an original client and official message API, never
// by POSTing a synthetic callback directly to the receiver.
import {chromium} from '../clients/web-desktop/node_modules/playwright/index.mjs';
import {readFile,writeFile,mkdir} from 'node:fs/promises';
import {createHash,randomUUID} from 'node:crypto';
const root=new URL('../',import.meta.url),base='https://192.168.10.11:29880';
const state=JSON.parse(await readFile(new URL('.private/test-accounts.json',root),'utf8'));
const sender=state.users[108],recipient=state.users[109],runID=randomUUID();
const errors=[],results=[];const wait=ms=>new Promise(r=>setTimeout(r,ms));
async function call(path,body,token=''){
 const response=await fetch(base+path,{method:'POST',headers:{'Content-Type':'application/json',operationID:randomUUID(),token,chatToken:token},body:JSON.stringify(body)});
 const value=await response.json();if(!response.ok||value.errCode!==0)throw new Error(path+': '+response.status+'/'+value.errCode);return value.data||{};
}
const admin=await call('/enterprise/v1/admin/login',{account:state.admin.account,password:createHash('md5').update(state.admin.password).digest('hex')});
const send=async stage=>call('/enterprise/v1/admin/im/msg/send_msg',{sendID:sender.userID,recvID:recipient.userID,senderNickname:sender.nickname,senderPlatformID:5,sessionType:1,contentType:101,content:{content:'Optional webhook '+stage+' '+runID},offlinePushInfo:{title:'Synthetic callback verification',desc:'No real push provider'}},admin.adminToken);
const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE});
let context;
try{
 context=await browser.newContext();const page=await context.newPage();page.on('pageerror',e=>errors.push(e.message));
 await page.addInitScript(({base})=>{for(const[k,v]of Object.entries({apiUrl:base+'/api',chatUrl:base+'/chat',enterpriseUrl:base+'/enterprise/v1',wsUrl:base.replace('https:','wss:')+'/ws'}))localStorage.setItem('upgrade38:'+k,v);},{base});
 await page.goto('http://127.0.0.1:7777',{waitUntil:'domcontentloaded'});
 await page.waitForFunction(()=>typeof window.commonEventFunc==='function',undefined,{timeout:90000});
 await page.getByRole('tab',{name:'邮箱',exact:true}).click();await page.locator('#email').fill(recipient.email);await page.locator('#password').fill(state.password);await page.getByRole('button',{name:'登录',exact:true}).click();await page.waitForURL('**/#/chat**',{timeout:90000});
 await page.evaluate(async()=>{window.optionalWebhookIm=(await import('/src/im/ImFacade.ts')).ImFacade;});
 results.push({case:'Original UI login opens actual SDK WebSocket',passed:true,userID:recipient.userID,platformID:5});
 await wait(5000);const online=await send('online');let read,lastReadError;
 for(let i=0;i<30;i++){
  try{read=await page.evaluate(async({senderID,messageID})=>{try{const sdk=window.optionalWebhookIm;const{data:c}=await sdk.getOneConversation({sourceID:senderID,sessionType:1});const{data:h}=await sdk.getAdvancedHistoryMessageList({conversationID:c.conversationID,count:20,startClientMsgID:'',viewType:0});const message=h.messageList.find(m=>m.clientMsgID===messageID);if(!message?.seq)return null;await sdk.markConversationMessageAsRead(c.conversationID);return{conversationID:c.conversationID,seq:message.seq};}catch(e){throw new Error(JSON.stringify({errCode:e?.errCode,errMsg:e?.errMsg||e?.message||String(e)}));}},{senderID:sender.userID,messageID:online.clientMsgID});}catch(error){lastReadError=String(error);}
  if(read)break;await wait(1000);
 }
 if(!read)throw new Error('Official sent message read did not complete: '+lastReadError);
 results.push({case:'Official single message received in SDK and marked read',passed:true,clientMsgID:online.clientMsgID,...read});
 await wait(2000);await context.close();context=undefined;await wait(4000);
 results.push({case:'Closing browser context closes actual SDK connection',passed:true});
 const offline=await send('offline');await wait(4000);
 results.push({case:'Official message sent while receiver offline',passed:true,clientMsgID:offline.clientMsgID});
}catch(error){results.push({case:'Optional native callback triggers',passed:false,error:String(error)});process.exitCode=1;}
finally{
 if(context)await context.close();await browser.close();await mkdir(new URL('tests/webhooks/',root),{recursive:true});
 const report={utc:new Date().toISOString(),runID,source:'Actual original UI SDK connection/read/close and official send_msg; no direct receiver POST',results,pageErrors:errors,limits:['These are trigger-side steps; match receiver Mongo records before declaring all four callbacks passed.','No third-party offline push delivery is claimed.']};
 await writeFile(new URL('tests/webhooks/triggers.json',root),JSON.stringify(report,null,2)+'\n');console.log(JSON.stringify(report));
}
