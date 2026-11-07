import test from "node:test";
import assert from "node:assert/strict";
import {build} from "esbuild";
const events=new Map();
globalThis.window={location:{origin:"https://im.szputy.com:9885",href:"https://im.szputy.com:9885/"},addEventListener:(name,cb)=>events.set(name,cb)};
const result=await build({entryPoints:["src/im/resources.ts"],bundle:true,write:false,format:"esm",platform:"node",plugins:[{name:"boundary",setup(b){
 b.onResolve({filter:/^@\/utils\/storage$/},()=>({path:"storage",namespace:"stub"}));
 b.onResolve({filter:/^\.\/enterprise$/},()=>({path:"enterprise",namespace:"stub"}));
 b.onLoad({filter:/.*/,namespace:"stub"},args=>({contents:args.path==="storage"?'export const getSSOSession=async()=>globalThis.resourceSession || {userID:"employee",sessionToken:"synthetic-session"};':'export const enterpriseRequest=(...args)=>globalThis.resourceRequest(...args);'}));
}}]});
const api=await import("data:text/javascript;base64,"+Buffer.from(result.outputFiles[0].text).toString("base64"));
const grant=(url="/attachments/opaque/openim38/file",purpose="preview")=>({data:{url:"https://im.szputy.com:9885"+url,expiresAt:Date.now()/1000+600,purpose,resourceProof:"a".repeat(64),resourceProofExpiresAt:Date.now()/1000+3600}});
test("third-party real avatars remain unchanged and never receive credentials",async()=>{let sent=false;globalThis.resourceRequest=()=>{sent=true;};assert.equal(await api.authorizeResource("https://avatar.example/person.png"),"https://avatar.example/person.png");assert.equal(sent,false);});
test("signed original URL is sent unchanged, concurrent render uses one grant",async()=>{
 events.get("enterprise-disconnect")();let calls=0,seen;
 const original="https://192.168.10.11:9885/openim38/a%20b?X-Amz-Signature=a%2Fb&part=1";
 globalThis.resourceRequest=async(path,body)=>{calls++;seen={path,body};await new Promise(r=>setTimeout(r,10));return grant();};
 const urls=await Promise.all([api.authorizeResource(original),api.authorizeResource(original)]);
 assert.equal(calls,1);assert.equal(seen.body.url,original);assert.equal(seen.path,"/attachment/authorize");assert.equal(urls[0],urls[1]);
});
test("grant response cannot redirect credentials or downloads to another origin",async()=>{
 events.get("enterprise-disconnect")();globalThis.resourceRequest=async()=>({data:{url:"https://evil.example/file",expiresAt:1e12}});
 await assert.rejects(api.authorizeResource("https://im.szputy.com:9885/openim38/blocked"));
});
test("nested attachment fields are authorized while message text stays original",async()=>{
 events.get("enterprise-disconnect")();globalThis.resourceRequest=async()=>grant("/attachments/new/openim38/a");
 const old="https://im.szputy.com:9885/api/object/a";const message={text:old,pictureElem:{sourcePicture:{url:old}}};
 await api.authorizePayload(message);assert.equal(message.text,old);assert.match(message.pictureElem.sourcePicture.url,/attachments\/new/);
});
test("persisted opaque URLs can be submitted for reauthorization",async()=>{
 events.get("enterprise-disconnect")();const old="https://192.168.10.11:9885/attachments/expired/openim38/a";let sent;
 globalThis.resourceRequest=async(_,body)=>{sent=body.url;return grant("/attachments/current/openim38/a");};
 await api.authorizeResource(old);assert.equal(sent,old);
});
test("logout invalidates an in-flight grant before the next account session",async()=>{
 events.get("enterprise-disconnect")();let complete;
 globalThis.resourceRequest=()=>new Promise(resolve=>{complete=resolve;});
 const pending=api.authorizeResource("https://im.szputy.com:9885/api/object/logout");
 await new Promise(r=>setTimeout(r,0));events.get("enterprise-disconnect")();
 complete(grant("/attachments/old/openim38/logout"));await assert.rejects(pending,/session changed/);
 let calls=0;globalThis.resourceRequest=async()=>{calls++;return grant("/attachments/new/openim38/logout");};
 assert.match(await api.authorizeResource("https://im.szputy.com:9885/api/object/logout"),/attachments\/new/);assert.equal(calls,1);
});

test("public brand images map all configured origins to current IM without private grants",async()=>{
 let calls=0;globalThis.resourceRequest=async()=>{calls++;throw new Error("must not authorize public brand");};
 for(const origin of ["https://192.168.10.11:9885","https://im.szputy.com:9885","https://192.168.10.11:9880","https://hr.szputy.com:9880"]){
  const source=origin+"/resource/static/brand/default-avatar.png?v=puty2";
  assert.equal(api.requiresResourceGrant(source),false);assert.equal(await api.authorizeResource(source),"https://im.szputy.com:9885/resource/static/brand/default-avatar.png?v=puty2");
  const profile={faceURL:source};await api.authorizePayload(profile);assert.equal(profile.faceURL,"https://im.szputy.com:9885/resource/static/brand/default-avatar.png?v=puty2");
 }
 for(const prefix of ['', 'suxin/', 'hr/', 'im/'])for(const name of ['logo.png','favicon.ico','favicon-16x16.png','favicon-32x32.png','apple-touch-icon.png','android-chrome-192x192.png','android-chrome-512x512.png']){
  const suffix='/resource/static/brand/'+prefix+name+'?v=puty2',source='https://hr.szputy.com:9880'+suffix;assert.equal(api.requiresResourceGrant(source),false);assert.equal(await api.authorizeResource(source),'https://im.szputy.com:9885'+suffix);
 }assert.equal(calls,0);
});
test("brand exception excludes private paths, traversal, userinfo and unsafe query",()=>{
 for(const suffix of ["/resource/uploads/a.png","/attachments/opaque/openim38/a.png","/resource/static/brand/nested/a.png","/resource/static/brand/../uploads/a.png","/resource/static/brand/a%2fb.png","/resource/static/brand/a%252fb.png","/resource/static/brand/a.png?redirect=../../uploads/a","/resource/static/brand/a.png?v=1&url=x","/resource/static/brand/a.png#fragment"]){
  const url="https://im.szputy.com:9885"+suffix;assert.equal(api.resolvePublicBrandResource(url),url);assert.equal(api.requiresResourceGrant(url),true);
 }
 for(const url of ["https://external.example/resource/static/brand/a.png","http://im.szputy.com:9885/resource/static/brand/a.png","https://u:p@im.szputy.com:9885/resource/static/brand/a.png"]){assert.equal(api.resolvePublicBrandResource(url),url);}
});

test("same viewer with a new SID cannot reuse the previous SID's cached grant",async()=>{
 events.get("enterprise-disconnect")();let calls=0;
 globalThis.resourceSession={userID:"same-viewer",sessionToken:"sid-one"};
 globalThis.resourceRequest=async()=>grant("/attachments/session-"+(++calls)+"/openim38/same-object");
 const source="https://im.szputy.com:9885/attachments/revoked-publisher/openim38/same-object";
 const first=await api.authorizeResource(source);
 globalThis.resourceSession={userID:"same-viewer",sessionToken:"sid-two"};
 const second=await api.authorizeResource(source);
 assert.notEqual(first,second);assert.equal(calls,2);
 delete globalThis.resourceSession;
});

test("force refresh bypasses a still-unexpired failed image grant exactly when requested",async()=>{
 events.get("enterprise-disconnect")();let calls=0;
 globalThis.resourceRequest=async()=>grant("/attachments/refresh-"+(++calls)+"/openim38/image");
 const source="https://im.szputy.com:9885/api/object/refresh-image";
 const first=await api.authorizeResource(source);assert.equal(await api.authorizeResource(source),first);assert.equal(calls,1);
 const refreshed=await api.authorizeResource(first,{force:true});assert.notEqual(refreshed,first);assert.equal(calls,2);
 assert.equal(await api.authorizeResource(source),refreshed);assert.equal(calls,2);
});

test("a late normal grant cannot overwrite the newer forced result",async()=>{
 events.get("enterprise-disconnect")();let firstComplete,calls=0;
 globalThis.resourceRequest=()=>++calls===1?new Promise(resolve=>{firstComplete=resolve;}):Promise.resolve(grant("/attachments/new-force/openim38/race"));
 const source="https://im.szputy.com:9885/api/object/force-race";
 const old=api.authorizeResource(source);await new Promise(resolve=>setImmediate(resolve));
 const fresh=await api.authorizeResource(source,{force:true});
 firstComplete(grant("/attachments/stale/openim38/race"));await assert.rejects(old,/session changed/);
 assert.equal(await api.authorizeResource(source),fresh);assert.equal(calls,2);
});

test("viewer switches reject an in-flight response even before a lifecycle notification",async()=>{
 events.get("enterprise-disconnect")();let complete;
 globalThis.resourceSession={userID:"viewer-a",sessionToken:"sid-a"};
 globalThis.resourceRequest=()=>new Promise(resolve=>{complete=resolve;});
 const pending=api.authorizeResource("https://im.szputy.com:9885/api/object/viewer-race");
 await new Promise(resolve=>setImmediate(resolve));
 globalThis.resourceSession={userID:"viewer-b",sessionToken:"sid-b"};
 complete(grant("/attachments/viewer-a/openim38/race"));await assert.rejects(pending,/session changed/);
 delete globalThis.resourceSession;
});

test("credential writes pause protected reads and completion starts a fresh generation",async()=>{
 let calls=0;globalThis.resourceRequest=async()=>{calls++;return grant("/attachments/after-write/openim38/a");};
 events.get("im-profile-changing")();
 await assert.rejects(api.authorizeResource("https://im.szputy.com:9885/api/object/pending-write"),/session changed/);assert.equal(calls,0);
 events.get("im-profile-changed")();
 assert.match(await api.authorizeResource("https://im.szputy.com:9885/api/object/pending-write"),/after-write/);assert.equal(calls,1);
});

test("preview and save have separate grants and proofs; no raw proof enters a URL",async()=>{
 events.get("enterprise-disconnect")();const calls=[];
 globalThis.resourceRequest=async(_,body)=>{calls.push(body);const response=grant(`/attachments/${body.purpose}/openim38/separate`,body.purpose);response.data.resourceProof=(body.purpose==="save"?"b":"a").repeat(64);return response;};
 const source="https://im.szputy.com:9885/api/object/separate";
 const preview=await api.authorizeResource(source),save=await api.authorizeResource(source,{purpose:"save"});
 assert.notEqual(preview,save);assert.equal(calls.length,2);assert.equal(await api.getResourceProof(preview),"a".repeat(64));assert.equal(await api.getResourceProof(save),"b".repeat(64));
 assert.equal(await api.authorizeResource(source),preview);assert.equal(await api.authorizeResource(source,{purpose:"save"}),save);
 assert.deepEqual(calls.map(c=>c.purpose),["preview","save"]);assert.ok(!preview.includes("a".repeat(64)));assert.ok(!save.includes("b".repeat(64)));
});

test("proof lookup cannot reuse a previous SID and rejects missing proof contracts",async()=>{
 events.get("enterprise-disconnect")();let calls=0;
 globalThis.resourceSession={userID:"viewer",sessionToken:"first"};
 globalThis.resourceRequest=async()=>{const response=grant(`/attachments/${++calls}/openim38/proof`);response.data.resourceProof=(calls===1?"a":"b").repeat(64);return response;};
 const old=await api.authorizeResource("https://im.szputy.com:9885/api/object/proof");
 globalThis.resourceSession={userID:"viewer",sessionToken:"second"};
 assert.equal(await api.getResourceProof(old),"b".repeat(64));assert.equal(calls,2);
 events.get("enterprise-disconnect")();globalThis.resourceRequest=async()=>({data:{url:old,expiresAt:1e12,purpose:"preview"}});
 await assert.rejects(api.authorizeResource(old),/Invalid attachment grant/);delete globalThis.resourceSession;
});

test("PC preview is a proof-bound range broker lease, never a renderer blob or raw grant",async()=>{
 events.get("enterprise-disconnect")();const calls=[];
 globalThis.localStorage={getItem:()=>"wan"};
 window.electronAPI={ipcInvoke:async(...args)=>{calls.push(args);return {url:"openim38://app/__remote/"+"1".repeat(64)};}};
 globalThis.resourceRequest=async()=>grant("/attachments/broker/openim38/video");
 try{
  const lease=await api.openResourcePreview("https://im.szputy.com:9885/api/object/video");
  assert.match(lease.url,/^openim38:\/\/app\/__remote\/1+$/);assert.equal(calls[0][0],"puty-store");assert.equal(calls[0][1],"remote-preview");assert.equal(calls[0][2].proof,"a".repeat(64));assert.match(calls[0][2].url,/attachments\/broker/);
  lease.release();await new Promise(resolve=>setImmediate(resolve));assert.equal(calls[1][1],"remote-preview-release");assert.ok(!calls[1][2].proof);
 }finally{delete window.electronAPI;delete globalThis.localStorage;}
});

test("same-origin Web preview uses HttpOnly cookie transport while cross-origin fallback is header-only",async()=>{
 events.get("enterprise-disconnect")();globalThis.resourceRequest=async()=>grant("/attachments/web/openim38/image");
 const direct=await api.openResourcePreview("https://im.szputy.com:9885/api/object/web");assert.match(direct.url,/^https:\/\/im.szputy.com/);
 const originalFetch=globalThis.fetch,originalOrigin=window.location.origin;let sent;
 window.location.origin=""; // Config falls back to the trusted URL; no same-origin cookie context.
 globalThis.fetch=async(url,options)=>{sent={url,options};return {ok:true,blob:async()=>new Blob(["synthetic"])};};
 try{
  const blob=await api.openResourcePreview("https://im.szputy.com:9885/api/object/web");assert.match(blob.url,/^blob:/);assert.equal(sent.options.headers["X-Puty-Resource-Proof"],"a".repeat(64));assert.equal(sent.options.credentials,"omit");assert.equal(sent.options.redirect,"error");assert.equal(sent.options.cache,"no-store");blob.release();
 }finally{globalThis.fetch=originalFetch;window.location.origin=originalOrigin;}
});

test("late PC broker responses are released when the SID changes",async()=>{
 events.get("enterprise-disconnect")();let complete;const calls=[];
 globalThis.localStorage={getItem:()=>"wan"};window.electronAPI={ipcInvoke:async(_,op,data)=>{calls.push({op,data});if(op==="remote-preview")return new Promise(resolve=>complete=resolve);}};
 globalThis.resourceRequest=async()=>grant("/attachments/late-broker/openim38/image");
 try{
  const pending=api.openResourcePreview("https://im.szputy.com:9885/api/object/late");await new Promise(resolve=>setImmediate(resolve));events.get("enterprise-disconnect")();
  complete({url:"openim38://app/__remote/"+"2".repeat(64)});await assert.rejects(pending,/session changed/);assert.equal(calls.at(-1).op,"remote-preview-release");
 }finally{delete window.electronAPI;delete globalThis.localStorage;}
});

test("offline clears preview access and online alone cannot restore it",async()=>{
 events.get("offline")();let calls=0;globalThis.resourceRequest=async()=>{calls++;return grant();};
 await assert.rejects(api.authorizeResource("https://im.szputy.com:9885/api/object/offline"),/session changed/);assert.equal(calls,0);assert.equal(events.has("online"),false);
 events.get("im-profile-changed")();assert.ok(await api.authorizeResource("https://im.szputy.com:9885/api/object/offline"));assert.equal(calls,1);
});
