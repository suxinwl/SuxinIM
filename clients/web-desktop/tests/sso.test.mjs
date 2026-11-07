import test from "node:test";
import assert from "node:assert/strict";
import {build} from "esbuild";
let saved=null, stores=0;
globalThis.window={location:{origin:"https://im.szputy.com:9885"}};
globalThis.localStorage={getItem:()=>null,removeItem:()=>{},setItem:()=>{}};
const result=await build({entryPoints:["src/api/sso.ts"],bundle:true,write:false,format:"esm",platform:"node",plugins:[{name:"storage",setup(b){b.onResolve({filter:/^@\/utils\/storage$/},()=>({path:"storage",namespace:"fake"}));b.onLoad({filter:/.*/,namespace:"fake"},()=>({contents:"export const getSSOSession=()=>globalThis.ssoSaved; export const setIMProfile=async(v)=>{globalThis.ssoSaved=v;globalThis.ssoStores++;};",loader:"js"}));}}]});
const api=await import("data:text/javascript;base64,"+Buffer.from(result.outputFiles[0].text).toString("base64"));
const session=()=>({userID:"emp_1",imToken:"im",chatToken:"chat",sessionToken:"sid",expiresAt:Math.floor(Date.now()/1000)+14400,profile:{}});
test("HRM password login uses current HTTPS origin and platform without legacy Chat login",async()=>{
 let request;globalThis.fetch=async(url,options)=>{request={url,...options};return {ok:true,status:200,json:async()=>({code:0,data:session()})};};
 await api.loginSSO("1001","md5","1234","challenge");
 assert.equal(request.url,"https://im.szputy.com:9885/sso/login");
 assert.deepEqual(JSON.parse(request.body),{account:"1001",password:"md5",captcha:"1234",captchaId:"challenge",platform:"web"});
 assert.equal(request.credentials,"omit");
});
test("refresh is single flight and preserves complete session",async()=>{
 globalThis.ssoSaved={...session(),expiresAt:1};globalThis.ssoStores=0;let calls=0;
 globalThis.fetch=async()=>{calls++;await new Promise(r=>setTimeout(r,20));return {ok:true,status:200,json:async()=>({code:0,data:session()})};};
 await Promise.all([api.refreshSSO(),api.refreshSSO()]);assert.equal(calls,1);assert.equal(globalThis.ssoStores,1);assert.equal(globalThis.ssoSaved.sessionToken,"sid");
});
test("invalid or revoked sessions fail closed",async()=>{
 assert.throws(()=>api.validateSession({...session(),sessionToken:""}));
 assert.throws(()=>api.validateSession({...session(),expiresAt:1}));
 globalThis.fetch=async()=>({ok:false,status:401,json:async()=>({code:401,msg:"revoked"})});
 await assert.rejects(api.refreshSSO(true),e=>e.status===401);
});
test("ticket exchange sends a one-time ticket to SSO",async()=>{
 let body;globalThis.fetch=async(_,options)=>{body=JSON.parse(options.body);return {ok:true,status:200,json:async()=>({code:0,data:session()})};};
 await api.exchangeSSOTicket("opaque");assert.deepEqual(body,{ticket:"opaque",platform:"web"});
});
test("unsupported account operation is never sent",async()=>{await assert.rejects(api.ssoRequest("register",{}));});
