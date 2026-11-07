import {isIP} from 'node:net';

export type LiveState={userID:string;sessionID:string;epoch:number;platform:number;logicalClient:string;expiresAt:number;loginAllowed:boolean;saveAllowed:boolean;effectivePolicy:{zone:'lan'|'internet';logicalClient:string;login:boolean;preview:boolean;save:boolean;securityVersion:number;policyRevision:number;networkRevision:number;currentIP:string;sources:{login:string;save:string}}};
const revision=(value:unknown)=>Number.isSafeInteger(value)&&Number(value)>=0;
const source=(value:unknown)=>typeof value==='string'&&value.length>0&&value.length<=256;

// A partial or old successful response must not unlock the OS-wrapped key.
// The authoritative policy is evaluated by HRM from the actual source IP.
export function assertPCSessionState(value:unknown,userID:string,now=Date.now()/1000):LiveState{
  const state=value as LiveState|undefined,p=state?.effectivePolicy;
  if(!state || state.userID!==userID || typeof state.sessionID!=='string' || !/^[\w-]{1,256}$/.test(state.sessionID)
      || !Number.isSafeInteger(state.epoch) || state.epoch<1 || state.platform!==3 || state.logicalClient!=='pc'
      || !Number.isSafeInteger(state.expiresAt) || state.expiresAt<=now || state.loginAllowed!==true || typeof state.saveAllowed!=='boolean'
      || !p || !['lan','internet'].includes(p.zone) || p.logicalClient!=='pc' || p.login!==true || p.preview!==true
      || p.save!==state.saveAllowed || p.securityVersion!==state.epoch || !revision(p.policyRevision) || !revision(p.networkRevision)
      || typeof p.currentIP!=='string' || !isIP(p.currentIP) || !source(p.sources?.login) || !source(p.sources?.save)){
    throw new Error('账号、网络或终端权限已失效');
  }
  return state;
}
