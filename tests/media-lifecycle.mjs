import {chromium} from '../clients/web-desktop/node_modules/playwright/index.mjs';
import {readFile,writeFile,mkdir} from 'node:fs/promises';
import {createHash,randomUUID} from 'node:crypto';
const root=new URL('../',import.meta.url),state=JSON.parse(await readFile(new URL('.private/test-accounts.json',root),'utf8'));
const base='https://192.168.10.11:29880',results=[],errors=[];
const wait=ms=>new Promise(r=>setTimeout(r,ms));
async function call(path,body,session,expected=200){const response=await fetch(base+path,{method:body===undefined?'GET':'POST',headers:{'Content-Type':'application/json',operationID:randomUUID(),token:session?.imToken||'',chatToken:session?.chatToken||'',platformID:'5'},body:body===undefined?undefined:JSON.stringify(body)});const data=await response.json();if(response.status!==expected||expected===200&&data.errCode!==0)throw new Error(`${path}: ${response.status}, ${data.errCode}`);return data.data||{};}
const login=async user=>({...user,...await call('/chat/account/login',{email:user.email,password:createHash('md5').update(state.password).digest('hex'),platform:5,deviceID:'media-lifecycle-'+randomUUID()})});
const [alice,bob]=await Promise.all([login(state.users[107]),login(state.users[108])]);
const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE,args:['--use-fake-device-for-media-stream','--use-fake-ui-for-media-stream']});
let rid;const contexts=[];
try{
 const invitation={inviterUserID:alice.userID,inviteeUserIDList:[bob.userID],sessionType:1,mediaType:'video',timeout:60,roomID:randomUUID().replaceAll('-','')};
 const first=await call('/enterprise/v1/signaling/invite',{invitation},alice);rid=first.roomID;
 const second=await call('/enterprise/v1/signaling/accept',{roomID:rid},bob);
 const pages=[];
 for(const grant of [first,second]){const context=await browser.newContext({permissions:['camera','microphone']});contexts.push(context);const page=await context.newPage();pages.push(page);page.on('pageerror',e=>errors.push(e.message));await page.goto('http://127.0.0.1:7777/');await page.evaluate(async grant=>{const {Room}=await import('/node_modules/livekit-client/dist/livekit-client.esm.mjs');window.lifecycleRoom=new Room();await window.lifecycleRoom.connect(grant.liveURL,grant.token);await window.lifecycleRoom.localParticipant.setCameraEnabled(true);await window.lifecycleRoom.localParticipant.setMicrophoneEnabled(true);},grant);}
 for(const page of pages)await page.waitForFunction(()=>window.lifecycleRoom.state==='connected'&&window.lifecycleRoom.remoteParticipants.size===1,undefined,{timeout:30000});
 results.push({case:'Real LiveKit two participants with audio and video',passed:true});
 await contexts[1].setOffline(true);await wait(5000);await contexts[1].setOffline(false);
 await pages[1].waitForFunction(()=>window.lifecycleRoom.state==='connected',undefined,{timeout:20000});
 let info=await call('/enterprise/v1/signaling/get_signal_invitation_info',{roomID:rid},alice);
 if(info.ended||!info.participant.includes(bob.userID))throw new Error('Transient disconnect removed valid membership');
 results.push({case:'Transient signaling loss reconnects within grace without ending call',passed:true});
 await contexts[1].close();const disconnectedAt=Date.now();let ended=false;
 for(let i=0;i<45;i++){await wait(2000);info=await call('/enterprise/v1/signaling/get_signal_invitation_info',{roomID:rid},alice);if(info.ended){ended=true;break;}}
 if(!ended)throw new Error('Abrupt browser close left one-to-one call active after 90 seconds');
 const oldStatus=await fetch(base+'/livekit/rtc/validate?access_token='+encodeURIComponent(second.token)).then(r=>r.status);
 if(![401,403].includes(oldStatus))throw new Error('Disconnected call still accepts old room token');
 results.push({case:'Abrupt close reconciles persisted call and rejects old media grant',passed:true,elapsedSeconds:(Date.now()-disconnectedAt)/1000,oldStatus});
 const again=await call('/enterprise/v1/signaling/invite',{invitation:{...invitation,roomID:randomUUID().replaceAll('-','')}},alice);rid=again.roomID;
 await call('/enterprise/v1/signaling/cancel',{roomID:rid},alice);rid=undefined;
 results.push({case:'Disconnected users are released from busy state and can call again',passed:true});
}catch(error){results.push({case:'Media lifecycle regression',passed:false,error:String(error)});process.exitCode=1;}
finally{if(rid)await call('/enterprise/v1/signaling/hungUp',{roomID:rid},alice).catch(()=>{});await browser.close();await mkdir(new URL('tests/media/',root),{recursive:true});await writeFile(new URL('tests/media/lifecycle.json',root),JSON.stringify({results,pageErrors:errors},null,2));console.log(JSON.stringify({results,pageErrors:errors}));}
