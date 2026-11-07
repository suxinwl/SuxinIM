export type LocalCleanupRule={conversationIDs:string[]|null;beforeMS:number;messageIDs?:string[]};
export function sanitizeLocalPayload<T>(payload:T,rules:LocalCleanupRule[],hint?:string):T {
  function visit(value:any,conversationID?:string):any{
    if(Array.isArray(value))return value.map(item=>visit(item,conversationID)).filter(item=>item!==null);
    if(!value || typeof value!=='object')return value;
    const explicit=value.conversationID || value.conversation_id,session=Number(value.sessionType ?? value.session_type);
    let id=explicit || conversationID;
    const group=value.groupID || value.group_id,sender=value.sendID || value.send_id,receiver=value.recvID || value.recv_id;
    if(!id && group)id=(session===2?'g_':'sg_')+group;
    if(!id && sender && receiver)id=(session===4?'sn_':'si_')+[sender,receiver].sort().join('_');
    const time=Number(value.sendTime || value.send_time || value.createTime || value.create_time || 0),ms=time && time<100000000000?time*1000:time;
    const messageID=value.clientMsgID || value.client_msg_id;
    if(messageID && id && rules.some(rule=>(!rule.conversationIDs || rule.conversationIDs.includes(id)) && (rule.messageIDs?rule.messageIDs.includes(messageID):ms<rule.beforeMS)))return null;
    const result:any={};let latestCleared=false;
    for(const [key,item] of Object.entries(value)){
      if((key==='latestMsg' || key==='latest_msg') && typeof item==='string' && item){try{const latest=visit(JSON.parse(item),id);result[key]=latest?JSON.stringify(latest):'';if(!latest)latestCleared=true;}catch{result[key]=item;}}
      else result[key]=visit(item,id);
    }
    if(latestCleared){if('unreadCount' in result)result.unreadCount=0;if('unread_count' in result)result.unread_count=0;}return result;
  }
  return visit(payload,hint);
}
let generation=0;
export function invalidateLocalCleanup(){generation++;}
if(typeof window!=='undefined'){window.addEventListener('im-profile-changing',invalidateLocalCleanup);window.addEventListener('puty-local-cleanup',invalidateLocalCleanup);}
export async function filterNativePayload<T>(payload:T,hint?:string):Promise<T>{
  if(!window.electronAPI)return payload;
  // A durable single-message recall/delete can arrive between any two SDK
  // callbacks, without a range-clean event. Query current native metadata for
  // each payload; sharing/caching an older in-flight policy could resurrect it.
  const run=generation,rules=await window.electronAPI.ipcInvoke<LocalCleanupRule[]>('puty-store','cleanup-policy');
  if(run!==generation)throw new Error('本机清理规则已改变');
  if(!Array.isArray(rules))throw new Error('本机清理规则无效');
  return sanitizeLocalPayload(payload,rules,hint);
}
