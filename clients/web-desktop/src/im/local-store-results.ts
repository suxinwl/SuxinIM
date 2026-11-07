export type CleanupPreview={messages:number;attachments:number;bytes:number;beforeMS:number;conversationIDs:string[]|null};
export function requireStoreAck(value:unknown,flag:'checkpointed'|'cleaned'){
  if(!value || typeof value!=='object' || (value as any).errCode || (value as any)[flag]!==true)throw new Error('本地加密存储操作未确认完成，请重新登录后重试');
}
export function requireCleanupPreview(value:unknown):CleanupPreview{
  if(!value || typeof value!=='object' || (value as any).errCode)throw new Error('清理范围计算未完成，请重新计算');const result=value as CleanupPreview;
  if([result.messages,result.attachments,result.bytes,result.beforeMS].some(number=>!Number.isSafeInteger(number) || number<0) || result.conversationIDs!==null && (!Array.isArray(result.conversationIDs) || result.conversationIDs.some(id=>typeof id!=='string' || !/^[\w-]{1,256}$/.test(id))))throw new Error('清理范围计算结果无效，请重新计算');
  return result;
}
