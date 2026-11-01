import test from "node:test";
import assert from "node:assert/strict";
import { build } from "esbuild";
const compiled = await build({ entryPoints: ["src/im/send-message.ts"], bundle: true, write: false, platform: "node", format: "esm" });
const { sendSdkMessage } = await import(`data:text/javascript;base64,${Buffer.from(compiled.outputFiles[0].text).toString("base64")}`);
function boundary() {
  const calls = [];
  const sdk = {
    sendMessage: async (...args) => { calls.push({ method: "normal", args }); return { errCode: 0, data: args[0].message }; },
    sendMessageNotOss: async (...args) => { calls.push({ method: "notOss", args }); return { errCode: 0, data: args[0].message }; },
  };
  return { sdk, calls };
}
test("cross-conversation quoted text uses the official safe route and preserves its full preview", async () => {
  const { sdk, calls } = boundary();
  const message = { contentType: 114, quoteElem: { text: "forwarded reply", quoteMessage: { clientMsgID: "other-conversation-original", sendID: "original-author", seq: 27, contentType: 101, textElem: { content: "quoted original" } } } };
  const params = { message, recvID: "new-recipient", groupID: "", isOnlineOnly: false };
  const before = JSON.stringify(params);
  const response = await sendSdkMessage(sdk, params, "synthetic-operation");
  assert.deepEqual(calls, [{ method: "notOss", args: [params, "synthetic-operation"] }]);
  assert.equal(JSON.stringify(params), before); assert.deepEqual(response.data, message);
});
test("all ordinary and attachment message types retain their normal upload/send route", async () => {
  for (const contentType of [101, 102, 103, 104, 105, 106, 107, 108, 110]) {
    const { sdk, calls } = boundary(); const params = { recvID: "target", groupID: "", message: { contentType } };
    await sendSdkMessage(sdk, params);
    assert.equal(calls[0].method, "normal", "type " + contentType);
  }
});
test("incomplete quoted messages fail before entering the Go sender", async () => {
  for (const quoteElem of [undefined, {}, { text: "reply" }, { text: "reply", quoteMessage: { clientMsgID: "id" } }]) {
    const { sdk, calls } = boundary();
    await assert.rejects(sendSdkMessage(sdk, { message: { contentType: 114, quoteElem } }), /incomplete/);
    assert.equal(calls.length, 0);
  }
});
test("server refusal of the safe quote route remains a rejected send", async () => {
  const error = { errCode: 1001, errMsg: "synthetic rejection" };
  const params = { message: { contentType: 114, quoteElem: { text: "reply", quoteMessage: { clientMsgID: "id", sendID: "author" } } } };
  const { sdk } = boundary(); sdk.sendMessageNotOss = async () => { throw error; };
  await assert.rejects(sendSdkMessage(sdk, params), e => e === error);
});
