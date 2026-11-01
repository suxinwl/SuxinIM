import type {MessageItem} from "./types";
export function adaptPayload(value:any):any {
 if(Array.isArray(value))return value.map(adaptPayload);
 if(!value || typeof value!=="object" || Object.getPrototypeOf(value)!==Object.prototype)return value;
 const result=Object.fromEntries(Object.entries(value).map(([key,v])=>[key,adaptPayload(v)]));
 if(typeof result.clientMsgID==="string" && typeof result.contentType==="number") {
  for(const key of ["serverMsgID","sendID","recvID","senderNickname","senderFaceUrl","groupID","content","attachedInfo","ex","localEx"])if(result[key]==null)result[key]="";
 }
 return result;
}

export function isEnterpriseHint(message:any):boolean {
 if(message?.contentType!==110 || !message.customElem?.data)return false;
 try {const data=JSON.parse(message.customElem.data);return data.enterpriseEvent===true || data.kind==="enterprise_event";}catch{return false;}
}
