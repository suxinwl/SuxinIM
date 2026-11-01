import {chromium} from '../clients/web-desktop/node_modules/playwright/index.mjs';
import {readFile,writeFile,mkdir} from 'node:fs/promises';
import {createHash,randomUUID} from 'node:crypto';

const root=new URL('../',import.meta.url);
const state=JSON.parse(await readFile(new URL('.private/test-accounts.json',root),'utf8'));
const base='https://192.168.10.11:29880',results=[],errors=[];
const transport=process.env.TURN_TRANSPORT||'udp';
if(!['udp','tls','tcp'].includes(transport))throw new Error('TURN_TRANSPORT must be udp, tls or tcp');
async function call(path,body,session){
 const response=await fetch(base+path,{method:'POST',headers:{'Content-Type':'application/json',operationID:randomUUID(),token:session?.imToken||'',chatToken:session?.chatToken||'',platformID:'5'},body:JSON.stringify(body)});
 const value=await response.json();
 if(response.status!==200||value.errCode!==0)throw new Error(`${path}: ${response.status}/${value.errCode}`);
 return value.data||{};
}
const login=async user=>({...user,...await call('/chat/account/login',{email:user.email,password:createHash('md5').update(state.password).digest('hex'),platform:5,deviceID:'relay-media-'+randomUUID()})});
const [alice,bob]=await Promise.all([login(state.users[107]),login(state.users[108])]);
const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE,args:['--use-fake-device-for-media-stream','--use-fake-ui-for-media-stream','--autoplay-policy=no-user-gesture-required']});
let rid;
try{
 const invitation={inviterUserID:alice.userID,inviteeUserIDList:[bob.userID],sessionType:1,mediaType:'video',timeout:60,roomID:randomUUID().replaceAll('-','')};
 // Retain the known synthetic ID even if CreateRoom fails during deployment.
 rid=invitation.roomID;
 const first=await call('/enterprise/v1/signaling/invite',{invitation},alice);
 const second=await call('/enterprise/v1/signaling/accept',{roomID:rid},bob),pages=[];
 for(const grant of [first,second]){
  const context=await browser.newContext({permissions:['camera','microphone','local-network-access']});
  const page=await context.newPage();pages.push(page);page.on('pageerror',e=>errors.push(e.message));
  page.on('requestfailed',request=>{const url=new URL(request.url());errors.push({request:url.origin+url.pathname,failure:request.failure()?.errorText});});
  page.on('console',message=>{if(message.type()==='error'&&/CORS|access control|network access/i.test(message.text()))errors.push(message.text().replace(/\?[^'"\s]*/g,'?[redacted]'));});
  // This transport regression exercises real LiveKit/WebRTC, not the IM UI.
  // Avoid initializing the unrelated React/WASM/SQLite app in both contexts.
  await page.route('http://127.0.0.1:7777/',route=>route.fulfill({status:200,contentType:'text/html',body:'<!doctype html><html><head><title>Isolated TURN media regression</title></head><body></body></html>'}));
  await page.addInitScript(({transport})=>{
   const Native=window.RTCPeerConnection;window.relayPCs=[];
   function filter(configuration){
    if(!configuration?.iceServers?.length)return configuration;
    const iceServers=configuration.iceServers.flatMap(server=>{
     let urls=(Array.isArray(server.urls)?server.urls:[server.urls]).filter(url=>transport==='tls'?url.startsWith('turns:'):url.startsWith('turn:')&&url.includes('transport=udp'));
     if(transport==='tcp')urls=urls.map(url=>url.replace('transport=udp','transport=tcp'));
     return urls.length?[{...server,urls}]:[];
    });
    if(!iceServers.length)throw new Error('Server did not advertise the requested TURN transport');
    return {...configuration,iceServers};
   }
   window.RTCPeerConnection=class extends Native{
    constructor(configuration,...rest){super(filter(configuration),...rest);window.relayPCs.push(this);}
    setConfiguration(configuration){return super.setConfiguration(filter(configuration));}
   };
  },{transport});
  await page.goto('http://127.0.0.1:7777/');
  await page.evaluate(async grant=>{
   const {Room,RoomEvent}=await import('/node_modules/livekit-client/dist/livekit-client.esm.mjs');
   const room=window.relayRoom=new Room();
   room.on(RoomEvent.TrackSubscribed,track=>{if(track.kind==='video'){const video=track.attach();video.muted=true;video.autoplay=true;document.body.appendChild(video);}});
   await room.connect(grant.liveURL,grant.token,{rtcConfig:{iceTransportPolicy:'relay'}});
   await room.localParticipant.setCameraEnabled(true);await room.localParticipant.setMicrophoneEnabled(true);
  },grant);
 }
 for(const page of pages)await page.waitForFunction(()=>[...document.querySelectorAll('video')].some(v=>v.readyState>=2&&v.videoWidth>0),undefined,{timeout:60000});
 const peers=await Promise.all(pages.map(page=>page.evaluate(async()=>{
  const pairs=[];
  for(const pc of window.relayPCs){
   const stats=await pc.getStats(),configuration=pc.getConfiguration();
   for(const entry of stats.values())if(entry.type==='transport'&&entry.selectedCandidatePairId){
    const pair=stats.get(entry.selectedCandidatePairId),local=stats.get(pair.localCandidateId),remote=stats.get(pair.remoteCandidateId);
    // A peer-reflexive mapping must point to an allocated TURN relay and retain
    // its exact TURN URL/protocol. It is never accepted merely because prflx.
    const allocated=[...stats.values()].find(candidate=>candidate.type==='local-candidate'&&candidate.candidateType==='relay'&&candidate.address===local.relatedAddress&&candidate.port===local.relatedPort&&candidate.url===local.url&&candidate.relayProtocol===local.relayProtocol);
    const viaRelay=local.candidateType==='relay'||local.candidateType==='prflx'&&!!allocated;
    const onlyRequestedTURN=configuration.iceServers.length>0&&typeof local.url==='string'&&configuration.iceServers.every(server=>(Array.isArray(server.urls)?server.urls:[server.urls]).every(url=>url===local.url));
    pairs.push({state:pair.state,policy:configuration.iceTransportPolicy,localType:local.candidateType,localProtocol:local.protocol,relayProtocol:local.relayProtocol,url:local.url?.replace(/\?.*$/,''),remoteType:remote.candidateType,derivedFromAllocatedRelay:local.candidateType==='prflx'&&!!allocated,viaRelay,onlyRequestedTURN,bytesSent:pair.bytesSent,bytesReceived:pair.bytesReceived});
   }
  }
  return {decodedVideos:[...document.querySelectorAll('video')].filter(v=>v.readyState>=2&&v.videoWidth>0).length,pairs};
 })));
 if(peers.some(peer=>!peer.decodedVideos||peer.pairs.length!==2||peer.pairs.some(pair=>pair.state!=='succeeded'||pair.policy!=='relay'||!pair.viaRelay||!pair.onlyRequestedTURN||pair.bytesSent<=0||pair.bytesReceived<=0)))throw new Error('Selected publisher/subscriber pairs did not prove bidirectional forced TURN media');
 results.push({case:`Two real LiveKit browsers decode remote video using forced TURN ${transport}`,passed:true,peers});
}catch(error){results.push({case:`Forced TURN ${transport} media regression`,passed:false,error:String(error)});process.exitCode=1;}
finally{
 if(rid)await call('/enterprise/v1/signaling/hungUp',{roomID:rid},alice).catch(()=>{});
 await browser.close();await mkdir(new URL('tests/media/',root),{recursive:true});
 const report={transport,results,pageErrors:errors};
 await writeFile(new URL(`tests/media/relay-${transport}.json`,root),JSON.stringify(report,null,2));
 console.log(JSON.stringify(report));
}
