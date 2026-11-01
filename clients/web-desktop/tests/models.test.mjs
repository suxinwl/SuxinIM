import test from "node:test";
import assert from "node:assert/strict";
import {build} from "esbuild";
const result=await build({entryPoints:["src/im/models.ts"],write:false,bundle:true,format:"esm",platform:"node"});
const {adaptPayload,isEnterpriseHint}=await import(`data:text/javascript;base64,${Buffer.from(result.outputFiles[0].text).toString("base64")}`);
test("message conversion retains body and normalizes old UI scalar defaults",()=>{const source={clientMsgID:"m1",contentType:101,textElem:{content:"hello"},seq:12};const value=adaptPayload(source);assert.equal(value.textElem.content,"hello");assert.equal(value.senderNickname,"");assert.equal(value.groupID,"");assert.equal(value.seq,12);assert.equal("groupID" in source,false);assert.equal(value.pictureElem,undefined);});
test("quoted and merged messages are adapted recursively without replacing identities",()=>{const input={data:{messageList:[{clientMsgID:"outer",contentType:114,quoteElem:{quoteMessage:{clientMsgID:"inner",contentType:101,textElem:{content:"quoted"}}}}]}};const result=adaptPayload(input);const quote=result.data.messageList[0].quoteElem.quoteMessage;assert.equal(quote.clientMsgID,"inner");assert.equal(quote.recvID,"");assert.equal(quote.textElem.content,"quoted");});
test("unknown enterprise summaries stay unknown",()=>{const message=adaptPayload({clientMsgID:"m",contentType:101,groupID:"g"});assert.equal(message.attachedInfoElem,undefined);});

test("enterprise transport hints never swallow original call business messages",()=>{assert.equal(isEnterpriseHint({contentType:110,customElem:{data:JSON.stringify({customType:901,data:{status:1}})}}),false);assert.equal(isEnterpriseHint({contentType:110,customElem:{data:JSON.stringify({enterpriseEvent:true,customType:901,eventID:5})}}),true);});
