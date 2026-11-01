import test from "node:test";
import assert from "node:assert/strict";
import { build } from "esbuild";

// Execute ImFacade, its real send selector, payload adapter and read hydration.
// Replace only the external SDK, network, storage and native-window boundaries.
const mocks = {
  "@openim/wasm-client-sdk": `
    export const CbEvents = new Proxy({}, {get:(_,key)=>key});
    export const getSDK = () => ({
      on() {},
      sendMessage:(...args)=>globalThis.__quoteFacadeTest.sendMessage(...args),
      sendMessageNotOss:(...args)=>globalThis.__quoteFacadeTest.sendMessageNotOss(...args),
      getOneConversation:(...args)=>globalThis.__quoteFacadeTest.getOneConversation(...args),
    });`,
  "./enterprise": `export const enterpriseRequest=(...args)=>globalThis.__quoteFacadeTest.enterpriseRequest(...args);`,
  "@/utils/storage": `export const getIMUserID=async()=>"synthetic-quote-facade-user";`,
  "./database-close": `export const installDatabaseCloseBridge=()=>undefined;`,
};
const compiled = await build({
  entryPoints: ["src/im/ImFacade.ts"], write: false, bundle: true, format: "esm", platform: "node",
  define: { "import.meta.env.BASE_URL": '"/"' },
  plugins: [{ name: "quote-facade-boundaries", setup(builder) {
    builder.onResolve({ filter: /.*/ }, args => args.path in mocks ? {path: args.path, namespace: "quote-facade-test"} : undefined);
    builder.onLoad({ filter: /.*/, namespace: "quote-facade-test" }, args => ({contents: mocks[args.path], loader: "js"}));
  } }],
});
const previousWindow = globalThis.window;
globalThis.window = {location:{href:"https://synthetic-quote-facade.invalid/"},addEventListener(){}};
const { ImFacade } = await import(`data:text/javascript;base64,${Buffer.from(compiled.outputFiles[0].text).toString("base64")}`);
globalThis.window = previousWindow;

const quote = (changes={}) => ({
  clientMsgID:"synthetic-quote", contentType:114, seq:0, groupID:"",
  quoteElem:{text:"synthetic reply",quoteMessage:{clientMsgID:"synthetic-other-conversation-original",sendID:"synthetic-author",seq:23,contentType:101,textElem:{content:"synthetic original preview"}}},
  ...changes,
});
const quotePreview = element => ({text:element.text,original:{clientMsgID:element.quoteMessage.clientMsgID,sendID:element.quoteMessage.sendID,seq:element.quoteMessage.seq,contentType:element.quoteMessage.contentType,textElem:element.quoteMessage.textElem}});
function fixture(overrides={}) {
  const f = globalThis.__quoteFacadeTest = {
    calls:[],
    sendMessage:async(...args)=>{f.calls.push({method:"sendMessage",args});return {errCode:0,operationID:args[1],data:args[0].message};},
    sendMessageNotOss:async(...args)=>{f.calls.push({method:"sendMessageNotOss",args});return {errCode:0,operationID:args[1],data:args[0].message};},
    getOneConversation:async request=>{f.calls.push({method:"getOneConversation",request});return {data:{conversationID:"synthetic-group-conversation"}};},
    enterpriseRequest:async(path,body)=>{f.calls.push({method:"enterpriseRequest",path,body});return {errCode:0,data:{num:{"synthetic-quote":{readNum:2,unreadNum:3}}}};},
    ...overrides,
  };
  return f;
}

test("actual ImFacade routes Quote to the official safe method with unchanged arguments and operation ID",async()=>{
  const f=fixture();const params={recvID:"synthetic-new-recipient",groupID:"",isOnlineOnly:false,offlinePushInfo:{title:"synthetic push"},message:quote()};
  const before=JSON.stringify(params);const response=await ImFacade.sendMessage(params,"synthetic-quote-operation");
  assert.deepEqual(f.calls,[{method:"sendMessageNotOss",args:[params,"synthetic-quote-operation"]}]);
  assert.equal(JSON.stringify(params),before);assert.equal(response.operationID,"synthetic-quote-operation");
  assert.deepEqual(quotePreview(response.data.quoteElem),quotePreview(params.message.quoteElem));
  assert.equal(response.data.serverMsgID,""); // The real legacy payload adapter ran.
});

test("actual ImFacade keeps ordinary text and attachments on their normal upload/send method",async()=>{
  for(const contentType of [101,102,103,104,105,106,107,108,110]) {
    const f=fixture();const params={recvID:"synthetic-recipient",groupID:"",message:{clientMsgID:"synthetic-media-"+contentType,contentType,seq:0}};
    const response=await ImFacade.sendMessage(params,"synthetic-media-operation");
    assert.deepEqual(f.calls,[{method:"sendMessage",args:[params,"synthetic-media-operation"]}],"contentType "+contentType);
    assert.equal(response.data.contentType,contentType);assert.equal(response.data.serverMsgID,"");
  }
});

test("actual group Quote response still receives legacy adaptation and enterprise read hydration",async()=>{
  const f=fixture();const params={recvID:"",groupID:"synthetic-group",message:quote({groupID:"synthetic-group",seq:34})};
  const response=await ImFacade.sendMessage(params,"synthetic-group-operation");
  assert.equal(f.calls[0].method,"sendMessageNotOss");
  assert.deepEqual(f.calls[1],{method:"getOneConversation",request:{sourceID:"synthetic-group",sessionType:3}});
  assert.deepEqual(f.calls[2],{method:"enterpriseRequest",path:"/msg/get_group_message_read_num",body:{conversationID:"synthetic-group-conversation",clientMsgIDs:["synthetic-quote"],clientMsgIDList:["synthetic-quote"],seqs:{"synthetic-quote":34}}});
  assert.equal(response.data.serverMsgID,"");
  assert.deepEqual(response.data.attachedInfoElem.groupHasReadInfo,{hasReadCount:2,unreadCount:3,hasReadUserIDList:[],groupMemberCount:6});
  assert.deepEqual(quotePreview(response.data.quoteElem),quotePreview(params.message.quoteElem));
  assert.equal(params.message.attachedInfoElem,undefined);
});

const meeting = () => ({clientMsgID:"synthetic-meeting-share",contentType:110,seq:0,customElem:{data:JSON.stringify({customType:905,data:JSON.stringify({id:"synthetic-meeting-room"})})}});
test("actual meeting 905 messages authorize before the normal SDK send",async()=>{
  const f=fixture();const params={recvID:"synthetic-recipient",groupID:"",message:meeting()};
  await ImFacade.sendMessage(params,"synthetic-meeting-operation");
  assert.deepEqual(f.calls,[
    {method:"enterpriseRequest",path:"/signaling/signal_invite_meeting",body:{roomID:"synthetic-meeting-room",userIDs:["synthetic-recipient"],groupIDs:[]}},
    {method:"sendMessage",args:[params,"synthetic-meeting-operation"]},
  ]);
});

test("rejected meeting 905 authorization cannot reach either SDK send method",async()=>{
  const denial={errCode:403,errMsg:"synthetic authorization denial"};
  const f=fixture({enterpriseRequest:async(path,body)=>{f.calls.push({method:"enterpriseRequest",path,body});throw denial;}});
  await assert.rejects(ImFacade.sendMessage({recvID:"",groupID:"synthetic-group",message:meeting()},"synthetic-denied-operation"),e=>e===denial);
  assert.deepEqual(f.calls,[{method:"enterpriseRequest",path:"/signaling/signal_invite_meeting",body:{roomID:"synthetic-meeting-room",userIDs:[],groupIDs:["synthetic-group"]}}]);
});

test("actual facade rejects malformed Quote before either SDK send route",async()=>{
  const f=fixture();await assert.rejects(ImFacade.sendMessage({recvID:"synthetic-recipient",groupID:"",message:{clientMsgID:"synthetic-incomplete-quote",contentType:114,quoteElem:{text:"synthetic reply"}}}),/incomplete/);
  assert.deepEqual(f.calls,[]);
});
