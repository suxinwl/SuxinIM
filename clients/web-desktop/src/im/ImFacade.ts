import { getSDK, CbEvents, type SdkResponse, type GroupMemberItem, type PublicUserItem } from "@openim/wasm-client-sdk";
import { enterpriseRequest } from "./enterprise";
import { getIMUserID } from "@/utils/storage";
import type { FullUserItemWithCache, MessageItem } from "./types";
import { adaptPayload,isEnterpriseHint } from "./models";
import { installDatabaseCloseBridge } from "./database-close";
import { sendVisibleReadReceipt } from "./read-receipt-gate";
import { sendSdkMessage } from "./send-message";
const root = new URL(import.meta.env.BASE_URL, window.location.href);
const core = getSDK({debug:false,coreWasmPath:new URL("openIM.wasm",root).href,sqlWasmPath:new URL("sql-wasm.wasm",root).href});
const resetDatabaseClose=installDatabaseCloseBridge(window);
type Core = ReturnType<typeof getSDK>;
type LegacyResult<T> = T extends import("@openim/wasm-client-sdk").MessageItem ? MessageItem : T extends Array<infer U> ? LegacyResult<U>[] : T extends object ? {[K in keyof T]:LegacyResult<T[K]>} : T;
type Result<F> = F extends (...a:any[])=>Promise<SdkResponse<infer T>> ? LegacyResult<T> : unknown;
type CompatibleCore = { [K in Exclude<keyof Core,"on"|"off">]: Core[K] extends (...a:any[])=>Promise<any> ? <T=Result<Core[K]>>(...args:any[])=>Promise<SdkResponse<T>> : Core[K] };
type EnterpriseMethod = <T=any>(...args:any[])=>Promise<SdkResponse<T>>;
type Extra = Record<"pinConversation"|"setConversationPrivateChat"|"setConversationBurnDuration"|"setConversationIsMsgDestruct"|"setConversationMsgDestructTime"|"setConversationRecvMessageOpt"|"resetConversationGroupAtType"|"setGlobalRecvMessageOpt"|"setFriendRemark"|"typingStatusUpdate"|"signalingInvite"|"signalingInviteInGroup"|"signalingAccept"|"signalingReject"|"signalingCancel"|"signalingHungUp"|"signalingGetTokenByRoomID"|"signalingCreateMeeting"|"signalingJoinMeeting"|"signalingCloseRoom"|"signalingGetMeetings"|"signalingUpdateMeetingInfo"|"signalingGetRoomByGroupID"|"signalingOperateStream",EnterpriseMethod> & {
 getUsersInfoWithCache:<T=FullUserItemWithCache[]>(params:{userIDList:string[];groupID?:string})=>Promise<SdkResponse<T>>;
 getGroupMessageReaderList:<T=GroupMemberItem[]>(params:any)=>Promise<SdkResponse<T>>;
 sendGroupMessageReadReceipt:EnterpriseMethod;
 stopEnterpriseSession:()=>void;
 on:(event:string,cb:(event:any)=>void)=>void;
 off:(event:string,cb:(event:any)=>void)=>void;
};
const listeners = new Map<string,Set<(event:any)=>void>>();
const sdkListeners=new Map<string,Map<(event:any)=>void,(event:any)=>void>>();
let active=false, cursor=-1, generation=0, timer:ReturnType<typeof setTimeout>;
let sdkLogoutPromise: Promise<unknown> | undefined;
function logoutCoreOnce() {
 // Server revocation can emit OnKickedOffline before explicit logout returns.
 // Serialize both callers so the Go WASM engine sees only one logout operation.
 return sdkLogoutPromise ??= Promise.resolve().then(()=>core.logout()).catch(()=>({errCode:0,errMsg:"",operationID:crypto.randomUUID(),data:null}));
}
const invitationRooms=new Set<string>();
const polling=new Set<number>();
const dispatch=(name:string,data:unknown)=>listeners.get(name)?.forEach(cb=>cb({event:name,data:adaptPayload(data),errCode:0,errMsg:""}));
function stopEnterpriseSession(){active=false;generation++;invitationRooms.clear();clearTimeout(timer);window.dispatchEvent(new Event("enterprise-disconnect"));}
async function restoreInvitation(invitation:any) {
 if(!invitation || invitationRooms.has(invitation.roomID))return;
 const {data:users}=await core.getUsersInfo([invitation.inviterUserID]);
 if(!active || !users[0])return;
 invitationRooms.add(invitation.roomID);
 const participant:any={userInfo:users[0]};
 if(invitation.groupID){const [{data:members},{data:groups}]=await Promise.all([core.getSpecifiedGroupMembersInfo({groupID:invitation.groupID,userIDList:[invitation.inviterUserID]}),core.getSpecifiedGroupsInfo([invitation.groupID])]);participant.groupMemberInfo=members[0];participant.groupInfo=groups[0];}
 dispatch(CbEvents.OnReceiveNewInvitation,{invitation,participant});
}
async function processEvent(event:{id:number;event:string;data:any}){
 if(event.event==="OnReceiveNewInvitation"){
  const roomID=event.data?.invitation?.roomID;if(!roomID)return;
  const {data}=await enterpriseRequest<any>("/signaling/get_signal_invitation_info",{roomID});
  if(!data.ended && data.activeInvitationForActor!==false && ["invited","active"].includes(data.state))await restoreInvitation(event.data.invitation);
  return;
 }
 if(["OnInvitationCancelled","OnInvitationTimeout","OnHangUp","OnInviteeRejected"].includes(event.event))invitationRooms.delete(event.data?.invitation?.roomID);
 if(["OnInviteeAccepted","OnInviteeRejected"].includes(event.event) && event.data.userID===await getIMUserID() && event.data.opPlatformID && event.data.opPlatformID!==(window.electronAPI?.getPlatform() || 5)){
  dispatch(event.event==="OnInviteeAccepted"?CbEvents.OnInviteeAcceptedByOtherDevice:CbEvents.OnInviteeRejectedByOtherDevice,event.data);return;
 }
 const eventName=event.event==="OnGroupReadReceipt"?CbEvents.OnRecvGroupReadReceipt:event.event==="OnStreamChanged"?CbEvents.OnStreamChange:event.event;
 dispatch(eventName,event.data);
}
async function poll(){
 if(!active || polling.has(generation))return;const run=generation;polling.add(run);
 try {
  const bootstrap=cursor<0;
  const {data}=await enterpriseRequest<{events:{id:number;event:string;data:unknown}[];cursor?:number;resyncRequired?:boolean}>(`/rtc/events?after=${cursor}`);
  if(!active || run!==generation)return;
  if(bootstrap){cursor=data.cursor || 0;const pending=await enterpriseRequest<any>("/signaling/get_signal_invitation_info_start_app",{});if(!pending.data.ended)await restoreInvitation(pending.data.invitation);}
  else {for(const event of data.events || []) { if(event.id>cursor){await processEvent(event);cursor=event.id;} }if(data.cursor)cursor=Math.max(cursor,data.cursor);}
  if(data.resyncRequired){const pending=await enterpriseRequest<any>("/signaling/get_signal_invitation_info_start_app",{});if(!pending.data.ended)await restoreInvitation(pending.data.invitation);}
 } catch(error) { console.debug("Enterprise event sync deferred",error); }
 finally {polling.delete(run);if(active && run===generation)timer=setTimeout(poll,2000);}
}

for(const event of [CbEvents.OnKickedOffline,CbEvents.OnUserTokenExpired,CbEvents.OnUserTokenInvalid]) core.on(event,stopEnterpriseSession);
core.on(CbEvents.OnRecvOnlineOnlyMessage,(envelope:any)=>{
 const messages=Array.isArray(envelope.data)?envelope.data:[envelope.data];
 if(active && messages.some(isEnterpriseHint)){clearTimeout(timer);timer=setTimeout(poll,0);}
});
window.addEventListener("enterprise-session-revoked",()=>{stopEnterpriseSession();dispatch(CbEvents.OnKickedOffline,{});});
const wrapped:Record<string,any>={
 stopEnterpriseSession,
 sendMessage:async(params:any,operationID?:string)=>{
  await authorizeMeetingShare(params);
  const response=adaptPayload(await sendSdkMessage(core,params,operationID));
  await hydrateReads([response.data]);
  return response;
 },
 on:(event:string,cb:(event:any)=>void)=>{if(!listeners.has(event))listeners.set(event,new Set());listeners.get(event)!.add(cb);if(!sdkListeners.has(event))sdkListeners.set(event,new Map());const normalized=async(e:any)=>{let data=adaptPayload(e.data);if(event===CbEvents.OnRecvNewMessage || event===CbEvents.OnRecvNewMessages){if(Array.isArray(data)){data=data.filter(m=>!isEnterpriseHint(m));if(!data.length)return;}else if(isEnterpriseHint(data))return;await hydrateReads(Array.isArray(data)?data:[data]);}cb({...e,data});};sdkListeners.get(event)!.set(cb,normalized);core.on(event as CbEvents,normalized);},
 off:(event:string,cb:(event:any)=>void)=>{listeners.get(event)?.delete(cb);const normalized=sdkListeners.get(event)?.get(cb);if(normalized)core.off(event as CbEvents,normalized);sdkListeners.get(event)?.delete(cb);},
 login:async(params:any)=>{resetDatabaseClose?.();const result=await core.login({...params,tryParse:true});resetDatabaseClose?.();sdkLogoutPromise=undefined;active=true;generation++;cursor=-1;clearTimeout(timer);void poll();return result;},
 logout:async(options:{localOnly?:boolean}={})=>{
  if(!options.localOnly)await enterpriseRequest("/session/logout",{});
  stopEnterpriseSession();
  return logoutCoreOnce();
 },
 pinConversation:(p:any)=>core.setConversation(p),
 setConversationPrivateChat:({isPrivate,...p}:any)=>core.setConversation({...p,isPrivateChat:isPrivate}),
 setConversationBurnDuration:(p:any)=>core.setConversation(p),
 setConversationIsMsgDestruct:(p:any)=>core.setConversation(p),
 setConversationMsgDestructTime:(p:any)=>core.setConversation(p),
 setConversationRecvMessageOpt:({opt,...p}:any)=>core.setConversation({...p,recvMsgOpt:opt}),
 resetConversationGroupAtType:(conversationID:string)=>core.setConversation({conversationID,groupAtType:0}),
 setGlobalRecvMessageOpt:(globalRecvMsgOpt:number)=>core.setSelfInfo({globalRecvMsgOpt}),
 setFriendRemark:async({toUserID,remark}:any)=>{
  // The pinned cache RPC interpolates identifiers; do not pass SQL metacharacters.
  if(remark==="" && !/^[A-Za-z0-9_-]{1,128}$/.test(toUserID))throw new Error("Unsupported friend identifier for the pinned SDK cache");
  const response=await core.updateFriends({friendUserIDs:[toUserID],remark});
  // The fixed Go SDK omits an empty remark from its incremental SQL update.
  // Apply the server-confirmed value to the SDK's own cache RPC as well.
  if(remark===""){
   const cacheRPC=(window as unknown as {updateColumnsFriend:(ids:string,fields:string)=>Promise<string>}).updateColumnsFriend;
   if(typeof cacheRPC!=="function")throw new Error("Pinned SDK friend cache RPC is unavailable");
   const cache=JSON.parse(await cacheRPC(JSON.stringify([toUserID]),JSON.stringify({remark:""})));
   if(cache.errCode)throw new Error(cache.errMsg || "Friend cache update failed");
   const {data}=await core.getSpecifiedFriendsInfo({friendUserIDList:[toUserID],filterBlack:false});
   if(data[0])dispatch(CbEvents.OnFriendInfoChanged,data[0]);
  }
  return response;
 },
 getUsersInfoWithCache:async({userIDList,groupID}:any)=>{
 userIDList=[...new Set(userIDList)];
 const res=await core.getUsersInfo(userIDList);
 const friends=await core.getSpecifiedFriendsInfo({friendUserIDList:userIDList,filterBlack:false});
 const members=groupID?(await core.getSpecifiedGroupMembersInfo({groupID,userIDList})).data:[];
 return {...res,data:res.data.map(publicInfo=>({publicInfo,friendInfo:friends.data.find(f=>f.userID===publicInfo.userID),groupMemberInfo:members.find(m=>m.userID===publicInfo.userID)}))};
},
 typingStatusUpdate:async({recvID,msgTip}:any)=>{const {data}=await core.getOneConversation({sourceID:recvID,sessionType:1});return core.changeInputStates({conversationID:data.conversationID,focus:msgTip!==""});},
 getGroupMessageReaderList:async(p:any)=>{
 const response=await enterpriseRequest<{reads:{userID:string}[]}>("/msg/get_group_message_has_read",p);
 const ids=(response.data.reads || []).map(r=>r.userID);
 if(!ids.length)return {...response,data:[]};
 const {data:conversations}=await core.getMultipleConversation([p.conversationID]);
 const groupID=conversations[0]?.groupID;if(!groupID)throw new Error("Group conversation is unavailable");
 const {data:members}=await core.getSpecifiedGroupMembersInfo({groupID,userIDList:ids});
 return {...response,data:ids.map(id=>members.find(m=>m.userID===id) || {userID:id,nickname:id,faceURL:"",groupID})};
},
 sendGroupMessageReadReceipt:async(p:any)=>{
  const {shouldSendReadReceipt=()=>true,...request}=p;
  return sendVisibleReadReceipt(shouldSendReadReceipt,async()=>{
   const ids:string[]=request.clientMsgIDs || request.clientMsgIDList || Object.keys(request.clientMsgs || {});
   const {data}=await core.findMessageList([{conversationID:request.conversationID,clientMsgIDList:ids}]);
   const messages=(data.findResultItems || []).flatMap(item=>item.messageList || []);
   const clientMsgs=Object.fromEntries(ids.map(id=>{const message=messages.find(m=>m.clientMsgID===id);if(!message?.sendID || !message.seq)throw new Error("Group message has not synchronized yet");return [id,message.sendID];}));
   return {...request,clientMsgs,seqs:Object.fromEntries(messages.map(m=>[m.clientMsgID,m.seq]))};
  },payload=>enterpriseRequest("/msg/mark_group_message_read",payload));
 },
 createQuoteMessage:(p:any)=>{const quoted=typeof p.message==="string"?JSON.parse(p.message):p.message;if(!quoted?.sendID || !quoted?.clientMsgID)throw new Error("Only sent messages can be quoted");return core.createQuoteMessage({...p,message:JSON.stringify(quoted)});},
};
const routes:Record<string,string>={signalingInvite:"invite",signalingInviteInGroup:"inviteInGroup",signalingAccept:"accept",signalingReject:"reject",signalingCancel:"cancel",signalingHungUp:"hungUp",signalingGetTokenByRoomID:"getTokenByRoomID",signalingCreateMeeting:"signal_create_meeting",signalingJoinMeeting:"signal_join_meeting",signalingCloseRoom:"signal_close_room",signalingGetMeetings:"signal_get_meetings",signalingUpdateMeetingInfo:"signal_update_meeting_info",signalingGetRoomByGroupID:"signal_get_room_by_group_id",signalingOperateStream:"signal_operate_stream"};
const pendingInvites=new Map<string,Promise<any>>();
for(const [method,route] of Object.entries(routes)) wrapped[method]=async(p:any={})=>{
 const input=typeof p==="string"?(method==="signalingGetRoomByGroupID"?{groupID:p}:{roomID:p}):p;
 const isInvite=method==="signalingInvite" || method==="signalingInviteInGroup";
 const roomID=input.invitation?.roomID;
 if(method==="signalingCancel" && pendingInvites.has(roomID))await pendingInvites.get(roomID);
 const operation=enterpriseRequest<any>(`/signaling/${route}`,input);
 if(isInvite && roomID)pendingInvites.set(roomID,operation);
 try {
  const response=await operation;
  if(isInvite && input.invitation)Object.assign(input.invitation,response.data.invitation || {roomID:response.data.roomID});
  return response;
 }finally{if(isInvite && roomID)pendingInvites.delete(roomID);}
};
async function authorizeMeetingShare(params:any){
 if(params?.message?.contentType!==110 || !params.message.customElem?.data)return;
 const body=JSON.parse(params.message.customElem.data);if(body.customType!==905)return;
 const invitation=typeof body.data==="string"?JSON.parse(body.data):body.data;
 if(!invitation?.id)throw new Error("Meeting invitation has no room identity");
 await enterpriseRequest("/signaling/signal_invite_meeting",{roomID:invitation.id,userIDs:params.recvID?[params.recvID]:[],groupIDs:params.groupID?[params.groupID]:[]});
}

async function hydrateReads(messages:any[],conversationID?:string){
 const groupMessages=messages.filter(m=>m?.groupID && m.seq>0);if(!groupMessages.length)return;
 try {
  const id=conversationID || (await core.getOneConversation({sourceID:groupMessages[0].groupID,sessionType:3})).data.conversationID;
  const {data}=await enterpriseRequest<any>("/msg/get_group_message_read_num",{conversationID:id,clientMsgIDs:groupMessages.map(m=>m.clientMsgID),clientMsgIDList:groupMessages.map(m=>m.clientMsgID),seqs:Object.fromEntries(groupMessages.map(m=>[m.clientMsgID,m.seq]))});
  for(const message of groupMessages){const summary=data.num?.[message.clientMsgID];if(summary)message.attachedInfoElem={...(message.attachedInfoElem || {}),groupHasReadInfo:{hasReadCount:summary.readNum,unreadCount:summary.unreadNum,hasReadUserIDList:[],groupMemberCount:summary.readNum+summary.unreadNum+1}};}
 }catch(error){console.debug("Read summary unavailable",error);}
}
export const ImFacade = new Proxy(core,{get(target,key){if(typeof key==="string" && key in wrapped)return wrapped[key];const value=Reflect.get(target,key);return typeof value==="function"?async(...args:any[])=>{if(key==="sendMessage")await authorizeMeetingShare(args[0]);const response=adaptPayload(await value.apply(target,args));if(key==="sendMessage")await hydrateReads([response.data]);else if(String(key).startsWith("getAdvancedHistoryMessageList"))await hydrateReads(response.data?.messageList || [],args[0]?.conversationID);return response;}:value;}}) as unknown as CompatibleCore & Extra;
export default ImFacade;
