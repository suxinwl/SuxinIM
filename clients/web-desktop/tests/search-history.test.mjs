import test from "node:test";
import assert from "node:assert/strict";
import { build } from "esbuild";

const mocks = {
  "@openim/wasm-client-sdk": `export const MessageViewType={History:0,Search:1}; export const MessageType={PictureMessage:102,VideoMessage:104}; export const SessionType={Notification:4};`,
  "@/layout/MainContentWrap": `export const IMSDK=new Proxy({}, {get:(_,key)=>(...args)=>globalThis.__historyFixture.sdk[key](...args)});`,
  "@/utils/common": `export const feedbackToast=value=>globalThis.__historyFixture.toasts.push(value);`,
  "./conversation": `export const useConversationStore={getState:()=>({currentConversation:globalThis.__historyFixture.conversation})};`,
  "./user": `export const useUserStore={getState:()=>({imageCache:{}})};`,
  "i18next": `export const t=value=>value;`,
};
const built = await build({
  entryPoints: ["src/store/message.ts"], write: false, bundle: true, format: "esm", platform: "node",
  plugins: [{ name: "history-native-boundaries", setup(builder) {
    builder.onResolve({ filter: /.*/ }, args => args.path in mocks ? { path: args.path, namespace: "history-test" } : undefined);
    builder.onLoad({ filter: /.*/, namespace: "history-test" }, args => ({ contents: mocks[args.path], loader: "js" }));
  } }],
});
globalThis.__historyFixture = {};
const { useMessageStore: store } = await import(`data:text/javascript;base64,${Buffer.from(built.outputFiles[0].text).toString("base64")}`);
const msg = (id, seq) => ({ clientMsgID: id, seq, sendTime: seq * 1000, sessionType: 1 });
const page = (messages = [], isEnd = true) => ({ data: { messageList: messages, isEnd } });
const deferred = () => { let resolve, reject; const promise = new Promise((a, b) => { resolve = a; reject = b; }); return { resolve, reject, promise }; };
function fixture(sdk = {}) {
  const f = globalThis.__historyFixture = { conversation: { conversationID: "chat-A" }, toasts: [], calls: [], sdk: {
    getAdvancedHistoryMessageList: async request => { f.calls.push(["before", request]); return page([msg("before", 2)]); },
    getAdvancedHistoryMessageListReverse: async request => { f.calls.push(["after", request]); return page([msg("after", 4)], false); },
    ...sdk,
  } };
  store.getState().clearHistoryMessage();
  store.setState({ historyMessageList: [msg("existing", 1)], jumpClientMsgID: undefined, laterHasMore: false });
  return f;
}
const jump = (id = "anchor", seq = 3, cid = "chat-A") => store.getState().getTwoWayHistoryMessage({ conversationID: cid, message: msg(id, seq) });

test("actual store loads context using the pinned Search view and exact message ID", async () => {
  const f = fixture(); assert.equal(await jump(), true);
  assert.deepEqual(f.calls.map(([, request]) => request), Array(2).fill({ conversationID: "chat-A", startClientMsgID: "anchor", count: 20, viewType: 1 }));
  assert.deepEqual(store.getState().historyMessageList.map(m => m.clientMsgID), ["after", "anchor", "before"]);
  assert.equal(store.getState().jumpClientMsgID, "anchor"); assert.equal(store.getState().jumpLoading, false);
  assert.equal(store.getState().hasMore, false); assert.equal(store.getState().laterHasMore, true);
});

test("context includes the selected anchor once even when both SDK pages repeat it", async () => {
  fixture({ getAdvancedHistoryMessageList: async () => page([msg("anchor", 3), msg("older", 2)]), getAdvancedHistoryMessageListReverse: async () => page([msg("newer", 4), msg("anchor", 3)]) });
  assert.equal(await jump(), true);
  assert.deepEqual(store.getState().historyMessageList.map(m => m.clientMsgID), ["newer", "anchor", "older"]);
});

test("a failed context request clears loading, keeps existing chat intact and allows retry", async () => {
  const f = fixture({ getAdvancedHistoryMessageList: async () => { throw new Error("synthetic offline"); } });
  assert.equal(await jump(), false); assert.equal(store.getState().jumpLoading, false);
  assert.equal(store.getState().jumpClientMsgID, undefined); assert.deepEqual(store.getState().historyMessageList.map(m => m.clientMsgID), ["existing"]);
  assert.equal(f.toasts.length, 1);
  f.sdk.getAdvancedHistoryMessageList = async () => page([msg("older", 2)]);
  assert.equal(await jump(), true); assert.equal(store.getState().jumpClientMsgID, "anchor");
});

test("inner SDK history errors are failures rather than an empty successful jump", async () => {
  fixture({ getAdvancedHistoryMessageList: async () => ({ data: { errCode: 10006, errMsg: "synthetic missing context", messageList: [], isEnd: true } }) });
  assert.equal(await jump(), false); assert.equal(store.getState().jumpLoading, false);
  assert.deepEqual(store.getState().historyMessageList.map(m => m.clientMsgID), ["existing"]);
});

test("invalid or summary rows cannot start an undefined conversation/anchor query", async () => {
  const f = fixture(); assert.equal(await jump("", 0), false); assert.equal(await jump("anchor", 3, ""), false);
  assert.equal(f.calls.length, 0); assert.equal(store.getState().jumpLoading, false);
});

test("a slower old jump cannot replace a newer selected search result or clear its spinner", async () => {
  const old = deferred(), latest = deferred();
  fixture({ getAdvancedHistoryMessageList: request => request.startClientMsgID === "old" ? old.promise : latest.promise });
  const oldJump = jump("old"), latestJump = jump("latest", 8);
  old.resolve(page([])); assert.equal(await oldJump, false); assert.equal(store.getState().jumpLoading, true);
  latest.resolve(page([])); assert.equal(await latestJump, true); assert.equal(store.getState().jumpClientMsgID, "latest"); assert.equal(store.getState().jumpLoading, false);
});

test("changing chats while context loads does not replace the newly selected conversation's messages", async () => {
  const pending = deferred(); const f = fixture({ getAdvancedHistoryMessageList: () => pending.promise });
  const task = jump(); f.conversation = { conversationID: "chat-B" }; store.setState({ historyMessageList: [msg("B-message", 9)] });
  pending.resolve(page([])); assert.equal(await task, false);
  assert.deepEqual(store.getState().historyMessageList.map(m => m.clientMsgID), ["B-message"]); assert.equal(store.getState().jumpLoading, false);
});

test("a pending ordinary history fetch cannot overwrite a completed search context", async () => {
  const pending = deferred(); const f = fixture({ getAdvancedHistoryMessageList: request => request.viewType === 1 ? Promise.resolve(page([])) : pending.promise });
  const normal = store.getState().getHistoryMessageListByReq(); assert.equal(await jump(), true);
  pending.resolve(page([msg("stale-normal", 10)])); await normal;
  assert.equal(store.getState().jumpClientMsgID, "anchor"); assert.ok(store.getState().historyMessageList.some(m => m.clientMsgID === "anchor"));
  assert.ok(!store.getState().historyMessageList.some(m => m.clientMsgID === "stale-normal")); assert.equal(f.toasts.length, 0);
});

test("a failed stale ordinary history request cannot clear a completed search context", async () => {
  const pending = deferred(); const f = fixture({ getAdvancedHistoryMessageList: request => request.viewType === 1 ? Promise.resolve(page([])) : pending.promise });
  const normal = store.getState().getHistoryMessageListByReq(); assert.equal(await jump(), true);
  pending.reject(new Error("synthetic old connection failed")); await normal;
  assert.equal(store.getState().jumpClientMsgID, "anchor"); assert.ok(store.getState().historyMessageList.some(m => m.clientMsgID === "anchor"));
  assert.equal(f.toasts.length, 0);
});

test("scrolling both directions after a search jump retains the Search view's SDK cursors", async () => {
  const f = fixture(); assert.equal(await jump(), true); f.calls.length = 0;
  await store.getState().getHistoryMessageListByReq(true);
  await store.getState().getHistoryMessageListReverseByReq();
  assert.equal(f.calls.length, 2);
  assert.ok(f.calls.every(([, request]) => request.viewType === 1));
  store.getState().updateJumpClientMsgID(); f.calls.length = 0;
  await store.getState().getHistoryMessageListByReq();
  assert.equal(f.calls[0][1].viewType, 0);
});
