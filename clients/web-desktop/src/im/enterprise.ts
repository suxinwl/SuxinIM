import { getEnterpriseUrl } from "@/config";
import { getChatToken, getIMToken } from "@/utils/storage";
export async function enterpriseRequest<T = unknown>(path: string, data?: unknown): Promise<{errCode:number;errMsg:string;data:T;operationID:string}> {
 const headers: Record<string,string> = {"Content-Type":"application/json",operationID:crypto.randomUUID(),token:String(await getIMToken() || ""),chatToken:String(await getChatToken() || ""),platformID:String(window.electronAPI?.getPlatform() || 5)};
 const response=await fetch(`${getEnterpriseUrl()}${path}`,{method:data===undefined?"GET":"POST",headers,body:data===undefined?undefined:JSON.stringify(data),signal:AbortSignal.timeout(25000)});
 const body=await response.json();
 if(!response.ok || body.errCode!==0) {
   if(response.status===401) window.dispatchEvent(new Event("enterprise-session-revoked"));
   throw Object.assign(new Error(body.errMsg || `Enterprise HTTP ${response.status}`),body);
 }
 return {...body,operationID:body.operationID || headers.operationID};
}
