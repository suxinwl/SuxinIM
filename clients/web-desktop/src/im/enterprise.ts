import { getEnterpriseUrl } from "@/config";
import { getChatToken, getIMToken } from "@/utils/storage";
export async function enterpriseRequest<T = unknown>(path: string, data?: unknown): Promise<{errCode:number;errMsg:string;data:T;operationID:string}> {
 const headers: Record<string,string> = {"Content-Type":"application/json",operationID:crypto.randomUUID(),token:String(await getIMToken() || ""),chatToken:String(await getChatToken() || ""),platformID:String(window.electronAPI?.getPlatform() || 5)};
 let response:Response,body:any;
 try{
  response=await fetch(`${getEnterpriseUrl()}${path}`,{method:data===undefined?"GET":"POST",headers,credentials:window.electronAPI?"omit":"include",redirect:"error",cache:"no-store",body:data===undefined?undefined:JSON.stringify(data),signal:AbortSignal.timeout(25000)});
  body=await response.json();
 }catch{
  window.dispatchEvent(new Event("enterprise-session-revoked"));
  throw new Error("Enterprise service unavailable");
 }
 if(!response.ok || body.errCode!==0) {
   // A Save-only denial must preserve an allowed preview/login session.
   if(response.status===401 || response.status===403 && (path==="/session/state" || body.errMsg==="HRM access denied") || response.status>=500) window.dispatchEvent(new Event("enterprise-session-revoked"));
   throw Object.assign(new Error(body.errMsg || `Enterprise HTTP ${response.status}`),body);
 }
 return {...body,operationID:body.operationID || headers.operationID};
}
