import test from "node:test";
import assert from "node:assert/strict";
import {build} from "esbuild";
const events=[];
globalThis.window={dispatchEvent:event=>events.push(event.type)};
const result=await build({entryPoints:["src/im/enterprise.ts"],bundle:true,write:false,format:"esm",platform:"node",plugins:[{name:"transport-boundaries",setup(b){
 b.onResolve({filter:/^@\/(config|utils\/storage)$/},args=>({path:args.path,namespace:"mock"}));
 b.onLoad({filter:/.*/,namespace:"mock"},args=>({contents:args.path==="@/config"?'export const getEnterpriseUrl=()=>"https://im.szputy.com:9885/enterprise/v1";':'export const getIMToken=async()=>"unusable-im-fixture";export const getChatToken=async()=>"unusable-chat-fixture";'}));
}}]});
const {enterpriseRequest}=await import("data:text/javascript;base64,"+Buffer.from(result.outputFiles[0].text).toString("base64"));

test("Web uses same-origin HttpOnly cookies and PC uses explicit dual headers without credentialed CORS",async()=>{
 const requests=[];globalThis.fetch=async(url,options)=>{requests.push({url,options});return {ok:true,status:200,json:async()=>({errCode:0,data:{}})};};
 await enterpriseRequest("/attachment/authorize",{url:"stable-fixture",purpose:"preview"});
 assert.equal(requests[0].options.credentials,"include");assert.equal(requests[0].options.redirect,"error");assert.equal(requests[0].options.cache,"no-store");
 window.electronAPI={getPlatform:()=>3};
 try{await enterpriseRequest("/session/state");assert.equal(requests[1].options.credentials,"omit");assert.equal(requests[1].options.method,"GET");assert.equal(requests[1].options.headers.platformID,"3");assert.equal(requests[1].options.headers.token,"unusable-im-fixture");assert.equal(requests[1].options.headers.chatToken,"unusable-chat-fixture");}finally{delete window.electronAPI;}
});

test("Save-only and room permission denials preserve allowed login; current session denials revoke",async()=>{
 events.length=0;
 let message="attachment access denied";globalThis.fetch=async()=>({ok:false,status:403,json:async()=>({errCode:1002,errMsg:message})});
 await assert.rejects(enterpriseRequest("/attachment/authorize",{purpose:"save"}));assert.equal(events.length,0);
 message="room permission denied";await assert.rejects(enterpriseRequest("/rtc/room",{}));assert.equal(events.length,0);
 await assert.rejects(enterpriseRequest("/session/state"));assert.deepEqual(events,["enterprise-session-revoked"]);
 events.length=0;message="HRM access denied";await assert.rejects(enterpriseRequest("/attachment/authorize",{}));assert.deepEqual(events,["enterprise-session-revoked"]);
});

test("network, malformed dependency response and service outage fail closed without exposing details",async()=>{
 for(const response of [()=>{throw new Error("private upstream details");},()=>({ok:true,status:200,json:async()=>{throw new Error("private body");}}),()=>({ok:false,status:503,json:async()=>({errCode:1006,errMsg:"service unavailable"})})]){
  events.length=0;globalThis.fetch=async()=>response();await assert.rejects(enterpriseRequest("/session/state"));assert.deepEqual(events,["enterprise-session-revoked"]);
 }
});
