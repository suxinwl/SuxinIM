import { getServerOrigin, NETWORKS } from "@/config";
import { getSSOSession } from "@/utils/storage";
import { enterpriseRequest } from "./enterprise";
import { isResourceSessionReady, subscribeResourceSession } from "./resource-session";
const trustedOrigins = new Set([...Object.values(NETWORKS),"https://192.168.10.11:9880","https://hr.szputy.com:9880"]);
export type ResourcePurpose = "preview" | "save";
type Grant = {url:string;expiresAt:number;purpose:ResourcePurpose;resourceProof:string;resourceProofExpiresAt:number};
type BoundGrant = Grant & {scope:string;source:string};
const cache = new Map<string,BoundGrant>();
const proofs = new Map<string,BoundGrant>();
const original = new Map<string,string>();
const flights = new Map<string,Promise<string>>();
const versions = new Map<string,number>();
let resourceGeneration=0;
const brandFileNames=["logo.png","favicon.ico","favicon-16x16.png","favicon-32x32.png","apple-touch-icon.png","android-chrome-192x192.png","android-chrome-512x512.png"];
const publicBrandPaths=new Set(["/resource/static/brand/default-avatar.png",...["","suxin/","hr/","im/"].flatMap(prefix=>brandFileNames.map(name=>"/resource/static/brand/"+prefix+name))]);
function publicBrandPath(value:string):string|undefined {
 try {
  const parsed=new URL(value);
  if(!trustedOrigins.has(parsed.origin) || parsed.username || parsed.password || !value.startsWith(parsed.origin))return;
  const path=value.slice(parsed.origin.length);
  const match=/^([^?#]+)(?:\?v=[A-Za-z0-9._-]{1,64})?$/.exec(path);
  if(match && match[0]===path && publicBrandPaths.has(match[1]))return path;
 }catch{}
}
export function resolvePublicBrandResource(value:string):string {
 const path=publicBrandPath(value);return path?getServerOrigin()+path:value;
}
export function requiresResourceGrant(url: string) {
  try { const parsed=new URL(url); return trustedOrigins.has(parsed.origin) && !parsed.username && !parsed.password && !publicBrandPath(url); } catch { return false; }
}
async function currentScope() {
  if (!isResourceSessionReady()) throw new Error("Resource session changed");
  const generation=resourceGeneration, session=await getSSOSession();
  if (!session?.userID || !session.sessionToken) throw new Error("Resource session unavailable");
  if(generation!==resourceGeneration || !isResourceSessionReady()) throw new Error("Resource session changed");
  return {generation,session,scope:JSON.stringify([generation,session.userID,session.sessionToken,getServerOrigin()])};
}
export async function authorizeResource(url: string, { force = false, purpose = "preview" }: { force?: boolean; purpose?:ResourcePurpose } = {}): Promise<string> {
  if(purpose!=="preview" && purpose!=="save")throw new Error("Invalid attachment purpose");
  if(publicBrandPath(url))return resolvePublicBrandResource(url);
  if (!requiresResourceGrant(url)) return url;
  const {generation,session,scope}=await currentScope();
  const source=original.get(url) || url;
  const key=JSON.stringify([scope,purpose,source]);
  if(generation!==resourceGeneration) throw new Error("Resource session changed");
  if(force){cache.delete(key);versions.set(key,(versions.get(key) || 0)+1);flights.delete(key);}
  const version=versions.get(key) || 0;
  const cached=cache.get(key);
  if (cached && cached.expiresAt > Date.now()/1000+30) return cached.url;
  if (flights.has(key)) return flights.get(key)!;
  const operation=enterpriseRequest<Grant>("/attachment/authorize",{url:source,purpose}).then(async({data})=>{
    const current=await getSSOSession();
    if(!isResourceSessionReady() || generation!==resourceGeneration || current?.userID!==session.userID || current?.sessionToken!==session.sessionToken || (versions.get(key) || 0)!==version) throw new Error("Resource session changed");
    const target=new URL(data.url);
    if(target.protocol!=="https:" || target.origin!==getServerOrigin() || target.username || target.password || target.hash || !target.pathname.startsWith("/attachments/") || data.purpose!==purpose || !/^[a-f0-9]{64}$/.test(data.resourceProof) || !Number.isFinite(data.expiresAt) || !Number.isFinite(data.resourceProofExpiresAt) || Math.min(data.expiresAt,data.resourceProofExpiresAt)<=Date.now()/1000) throw new Error("Invalid attachment grant");
    const bound={...data,expiresAt:Math.min(data.expiresAt,data.resourceProofExpiresAt),scope,source};
    cache.set(key,bound);proofs.set(data.url,bound);original.set(data.url,source);return data.url;
  }).finally(()=>{if(flights.get(key)===operation)flights.delete(key);});
  flights.set(key,operation);return operation;
}

// Proofs are never serialized into messages, query strings, DOM attributes,
// localStorage or React query keys. Only a current scope can retrieve one.
export async function getResourceProof(url:string):Promise<string> {
  if(!requiresResourceGrant(url))return "";
  const {scope}=await currentScope();
  const known=proofs.get(url);
  if(known?.scope===scope && known.expiresAt>Date.now()/1000+5)return known.resourceProof;
  const authorized=await authorizeResource(original.get(url) || url,{purpose:known?.purpose || "preview"});
  const current=proofs.get(authorized);
  if(!current || current.scope!==scope || (await currentScope()).scope!==scope)throw new Error("Resource session changed");
  return current.resourceProof;
}

export type ResourcePreview = {url:string;release:()=>void};
export async function openResourcePreview(source:string,{force=false,signal}:{force?:boolean;signal?:AbortSignal}={}):Promise<ResourcePreview> {
  const scope=await currentScope();
  const url=await authorizeResource(source,{force,purpose:"preview"});
  const proof=await getResourceProof(url);
  const stillCurrent=async()=>{if(signal?.aborted || (await currentScope()).scope!==scope.scope)throw new Error("Resource session changed");};
  await stillCurrent();
  if(window.electronAPI){
    const result=await window.electronAPI.ipcInvoke<{url:string}>("puty-store","remote-preview",{url,proof});
    const release=()=>{void window.electronAPI?.ipcInvoke("puty-store","remote-preview-release",{url:result.url}).catch(()=>{});};
    try{
      const parsed=new URL(result.url);
      if(parsed.protocol!=="openim38:" || parsed.hostname!=="app" || parsed.username || parsed.password || parsed.search || parsed.hash || !/^\/__remote\/[a-f0-9]{32,128}$/.test(parsed.pathname))throw new Error("Invalid local preview broker");
      await stillCurrent();
      return {url:result.url,release};
    }catch(error){release();throw error;}
  }
  // A normal same-origin Web tag sends the host-only HttpOnly proof cookie and
  // keeps native Range/streaming behavior. Never rely on cross-site cookies.
  if(new URL(url).origin===window.location.origin){await stillCurrent();return {url,release:()=>{}};}
  const response=await fetch(url,{headers:{"X-Puty-Resource-Proof":proof},credentials:"omit",redirect:"error",cache:"no-store",signal});
  if(!response.ok)throw new Error("Protected resource unavailable");
  const blob=await response.blob();await stillCurrent();
  const local=URL.createObjectURL(blob);return {url:local,release:()=>URL.revokeObjectURL(local)};
}
const resourceKeys=new Set(["url","faceurl","senderfaceurl","sourceurl","snapshoturl","videourl","soundurl","fileurl","imageurl","pictureurl","coverurl"]);
export async function authorizePayload(value: any): Promise<void> {
 if(!value || typeof value!=="object") return;
 await Promise.all(Object.entries(value).map(async([key,item])=>{
   if(typeof item==="string" && resourceKeys.has(key.toLowerCase())) {
     try { value[key]=await authorizeResource(item); } catch { /* Keep a retriable original URL. */ }
   } else if(item && typeof item==="object") await authorizePayload(item);
 }));
}
subscribeResourceSession(()=>{resourceGeneration++;cache.clear();proofs.clear();original.clear();flights.clear();versions.clear();});
