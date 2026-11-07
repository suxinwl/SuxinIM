import test from 'node:test';
import assert from 'node:assert/strict';
import {build} from 'esbuild';
const compiled=await build({entryPoints:['electron/main/localStore/sessionState.ts'],bundle:true,write:false,platform:'node',format:'esm'});
const {assertPCSessionState}=await import('data:text/javascript;base64,'+Buffer.from(compiled.outputFiles[0].text).toString('base64'));
const account='emp_'+'d'.repeat(32),now=1793736000;
const valid=()=>({userID:account,sessionID:'synthetic-session',epoch:1,platform:3,logicalClient:'pc',expiresAt:now+3600,loginAllowed:true,saveAllowed:false,effectivePolicy:{zone:'lan',logicalClient:'pc',login:true,preview:true,save:false,securityVersion:1,policyRevision:0,networkRevision:1,currentIP:'192.168.10.2',sources:{login:'personal:allow',save:'personal:deny'}}});
test('complete authoritative PC policy accepts both real zones and preview-only permission',()=>{
  for(const zone of ['lan','internet']){const state=valid();state.effectivePolicy.zone=zone;assert.equal(assertPCSessionState(state,account,now),state);}
});
test('old partial responses and legacy zero epochs cannot unlock a local key',()=>{
  for(const patch of [{effectivePolicy:undefined},{epoch:0},{saveAllowed:undefined},{expiresAt:undefined},{expiresAt:now},{expiresAt:'9999999999'},{platform:undefined},{sessionID:{opaque:true}}])assert.throws(()=>assertPCSessionState({...valid(),...patch},account,now),/账号/);
});
test('policy must agree with this account, terminal, epoch and save decision',()=>{
  assert.throws(()=>assertPCSessionState(valid(),'emp_'+'e'.repeat(32),now),/账号/);
  for(const patch of [{logicalClient:'ios'},{securityVersion:2},{save:true},{login:false},{preview:false},{zone:'unknown'}]){const state=valid();Object.assign(state.effectivePolicy,patch);assert.throws(()=>assertPCSessionState(state,account,now),/账号/);}
});
test('source IP, permission provenance and revisions must be complete and valid',()=>{
  for(const patch of [{currentIP:''},{currentIP:'https://192.168.10.2'},{sources:undefined},{sources:{login:'personal:allow',save:''}},{policyRevision:-1},{networkRevision:NaN}]){const state=valid();Object.assign(state.effectivePolicy,patch);assert.throws(()=>assertPCSessionState(state,account,now),/账号/);}
});
