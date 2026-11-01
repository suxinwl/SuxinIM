import {getSDK,CbEvents,LogLevel} from "@openim/wasm-client-sdk";
const sdk=getSDK({coreWasmPath:"/openIM.wasm",sqlWasmPath:"/sql-wasm.wasm"});
const stats={connected:false,disconnections:0,sent:0,received:0,failed:0,latencies:[] as number[],errors:[] as string[]};
const seen=new Set<string>();let testRun="";
for(const event of [CbEvents.OnConnectSuccess])sdk.on(event,()=>{stats.connected=true;});
sdk.on(CbEvents.OnConnecting,()=>{if(stats.connected){stats.connected=false;stats.disconnections++;}});
for(const event of [CbEvents.OnConnectFailed,CbEvents.OnKickedOffline,CbEvents.OnUserTokenExpired,CbEvents.OnUserTokenInvalid])sdk.on(event,()=>{stats.connected=false;stats.disconnections++;});
for(const event of [CbEvents.OnRecvNewMessage,CbEvents.OnRecvNewMessages])sdk.on(event,(envelope:any)=>{
 for(const message of Array.isArray(envelope.data)?envelope.data:[envelope.data]) {
  if(seen.has(message.clientMsgID))continue;seen.add(message.clientMsgID);
  try{const body=JSON.parse(message.textElem?.content || "");if(body.test!=="openim38-load" || body.runID!==testRun)continue;stats.received++;stats.latencies.push(Date.now()-body.time);}catch{}
 }
});
Object.assign(window,{
 loadSDKReady:true,
 loadStats:()=>({...stats,errors:stats.errors.slice(-10)}),
 loadLogin:async(account:any,origin:string,runID:string)=>{testRun=runID;try{await sdk.login({userID:account.userID,token:account.imToken,platformID:5,apiAddr:origin+"/api",wsAddr:origin.replace(/^http/,"ws")+"/ws",logLevel:LogLevel.Error,isLogStandardOutput:false,tryParse:true});return true;}catch(error:any){throw new Error(JSON.stringify({errCode:error?.errCode,errMsg:error?.errMsg || error?.message || String(error)}));}},
 loadSend:async(recvID:string,sequence:number)=>{
  try{const {data:message}=await sdk.createTextMessage(JSON.stringify({test:"openim38-load",runID:testRun,sequence,time:Date.now()}));await sdk.sendMessage({recvID,groupID:"",message});stats.sent++;return true;}
  catch(error:any){stats.failed++;stats.errors.push(String(error?.errMsg || error?.message || error));return false;}
 },
 loadLogout:()=>sdk.logout(),
});
