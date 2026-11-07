import test from "node:test";
import assert from "node:assert/strict";
import { build } from "esbuild";

// Execute the real public facade method; replace only network/native boundaries.
const mocks = {
  "@openim/wasm-client-sdk": `
    export const CbEvents = new Proxy({}, { get: (_, name) => name });
    export const getSDK = () => ({
      on() {},
      findMessageList: (...args) => globalThis.__receiptFacadeTest.findMessageList(...args),
    });`,
  "./enterprise": `export const enterpriseRequest = (...args) => globalThis.__receiptFacadeTest.enterpriseRequest(...args);`,
  "@/utils/storage": `export const getIMUserID = async () => "synthetic-receipt-test"; export const getSSOSession=async()=>({userID:"synthetic-receipt-test",sessionToken:"synthetic-receipt-session"});`,
  "./models": `export const adaptPayload = value => value; export const isEnterpriseHint = () => false;`,
  "./database-close": `export const installDatabaseCloseBridge = () => undefined;`,
};
const bundle = await build({
  entryPoints: ["src/im/ImFacade.ts"],
  write: false,
  bundle: true,
  format: "esm",
  platform: "node",
  define: { "import.meta.env.BASE_URL": '"/"' },
  plugins: [{
    name: "receipt-native-boundaries",
    setup(builder) {
      builder.onResolve({ filter: /.*/ }, args => args.path in mocks
        ? { path: args.path, namespace: "receipt-test" }
        : undefined);
      builder.onLoad({ filter: /.*/, namespace: "receipt-test" }, args => ({ contents: mocks[args.path], loader: "js" }));
    },
  }],
});
const previousWindow = globalThis.window;
globalThis.window = { location: { href: "https://receipt-test.invalid/" }, addEventListener() {} };
const { ImFacade } = await import(`data:text/javascript;base64,${Buffer.from(bundle.outputFiles[0].text).toString("base64")}`);
globalThis.window = previousWindow;
const cachedMessage = { clientMsgID: "msg-1", sendID: "synthetic-sender", seq: 34 };
function boundaries(findMessageList = async () => ({ data: { findResultItems: [{ messageList: [cachedMessage] }] } })) {
  const sent = [];
  const found = [];
  globalThis.__receiptFacadeTest = {
    findMessageList: (...args) => { found.push(args); return findMessageList(...args); },
    enterpriseRequest: async (path, body) => { sent.push({ path, body }); return { errCode: 0, data: {} }; },
  };
  return { sent, found };
}

test("actual facade reports verified cache identities and strips the renderer-only visibility function", async () => {
  const { sent, found } = boundaries();
  await ImFacade.sendGroupMessageReadReceipt({
    conversationID: "group-test",
    clientMsgIDList: ["msg-1"],
    seqs: { "msg-1": 9999 },
    shouldSendReadReceipt: () => true,
  });
  assert.deepEqual(found, [[[{ conversationID: "group-test", clientMsgIDList: ["msg-1"] }]]]);
  assert.deepEqual(sent, [{
    path: "/msg/mark_group_message_read",
    body: { conversationID: "group-test", clientMsgIDList: ["msg-1"], clientMsgs: { "msg-1": "synthetic-sender" }, seqs: { "msg-1": 34 } },
  }]);
  assert.equal("shouldSendReadReceipt" in sent[0].body, false);
});

test("actual facade cancels after a window hide while the SDK cache lookup is pending", async () => {
  let finish;
  let active = true;
  const { sent } = boundaries(() => new Promise(resolve => { finish = resolve; }));
  const pending = ImFacade.sendGroupMessageReadReceipt({
    conversationID: "group-test", clientMsgIDList: ["msg-1"], shouldSendReadReceipt: () => active,
  });
  active = false;
  finish({ data: { findResultItems: [{ messageList: [cachedMessage] }] } });
  await assert.rejects(pending, /no longer visible/);
  assert.deepEqual(sent, []);
});

test("actual facade retains the legacy call shape for deliberate callers without a visibility callback", async () => {
  const { sent } = boundaries();
  await ImFacade.sendGroupMessageReadReceipt({ conversationID: "group-test", clientMsgIDs: ["msg-1"] });
  assert.equal(sent.length, 1);
  assert.deepEqual(sent[0].body.clientMsgs, { "msg-1": "synthetic-sender" });
});

test("actual facade refuses unsynchronized messages instead of inventing read identities", async () => {
  const { sent } = boundaries(async () => ({ data: { findResultItems: [] } }));
  await assert.rejects(ImFacade.sendGroupMessageReadReceipt({ conversationID: "group-test", clientMsgIDs: ["msg-1"] }), /has not synchronized/);
  assert.deepEqual(sent, []);
});
