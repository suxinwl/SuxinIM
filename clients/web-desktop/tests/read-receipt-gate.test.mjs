import test from "node:test";
import assert from "node:assert/strict";
import { build } from "esbuild";

const bundle = await build({
  entryPoints: ["src/im/read-receipt-gate.ts"],
  write: false,
  bundle: true,
  format: "esm",
  platform: "node",
});
const {
  createReadActivityStore,
  isReadWindowActive,
  scheduleConversationRead,
  sendVisibleReadReceipt,
  ReadReceiptCancelled,
} = await import(`data:text/javascript;base64,${Buffer.from(bundle.outputFiles[0].text).toString("base64")}`);

class TrackedTarget extends EventTarget {
  listeners = new Map();
  addEventListener(name, callback) {
    super.addEventListener(name, callback);
    this.listeners.set(name, (this.listeners.get(name) || 0) + 1);
  }
  removeEventListener(name, callback) {
    super.removeEventListener(name, callback);
    this.listeners.set(name, this.listeners.get(name) - 1);
  }
  dispatch(name) { this.dispatchEvent(new Event(name)); }
  get listenerCount() { return [...this.listeners.values()].reduce((a, b) => a + b, 0); }
}
function activityFixture() {
  const window = new TrackedTarget();
  const document = new TrackedTarget();
  document.visibilityState = "visible";
  document.focused = true;
  document.hasFocus = () => document.focused;
  return { window, document, activity: createReadActivityStore(window, document) };
}
function readFixture(t) {
  t.mock.timers.enable({ apis: ["setTimeout"] });
  const fixture = activityFixture();
  let current = { conversationID: "single-A-B", unreadCount: 1, resetGroupAt: false };
  const calls = [];
  return {
    ...fixture,
    calls,
    setConversation: value => { current = value; },
    schedule: (overrides = {}) => scheduleConversationRead({
      conversationID: "single-A-B",
      canRead: fixture.activity.getSnapshot,
      getConversation: () => current,
      markRead: async id => calls.push(["read", id]),
      resetGroupAt: async id => calls.push(["at", id]),
      ...overrides,
    }),
  };
}

test("foreground focus and visible page are both necessary", () => {
  const { document } = activityFixture();
  assert.equal(isReadWindowActive(document), true);
  document.focused = false;
  assert.equal(isReadWindowActive(document), false);
  document.focused = true;
  document.visibilityState = "hidden";
  assert.equal(isReadWindowActive(document), false);
});

test("shared activity subscription covers blur, visibility, pagehide and resume, and cleans up", () => {
  const { window, document, activity } = activityFixture();
  let changes = 0;
  const unsubscribe = activity.subscribe(() => changes++);
  const unsubscribeOther = activity.subscribe(() => changes++);
  assert.equal(window.listenerCount, 4);
  assert.equal(document.listenerCount, 1);
  document.focused = false;
  window.dispatch("blur");
  assert.equal(activity.getSnapshot(), false);
  document.focused = true;
  window.dispatch("focus");
  assert.equal(activity.getSnapshot(), true);
  document.visibilityState = "hidden";
  document.dispatch("visibilitychange");
  assert.equal(activity.getSnapshot(), false);
  document.visibilityState = "visible";
  window.dispatch("pagehide");
  assert.equal(activity.getSnapshot(), false);
  window.dispatch("pageshow");
  assert.equal(activity.getSnapshot(), true);
  assert.equal(changes, 10);
  unsubscribeOther();
  assert.equal(window.listenerCount, 4);
  unsubscribe();
  assert.equal(window.listenerCount, 0);
  assert.equal(document.listenerCount, 0);
  window.dispatch("blur");
  assert.equal(changes, 10);
});

test("a queued unread conversation reports once only after the active visible interval", t => {
  const f = readFixture(t);
  f.schedule();
  t.mock.timers.tick(149);
  assert.deepEqual(f.calls, []);
  t.mock.timers.tick(1);
  assert.deepEqual(f.calls, [["read", "single-A-B"]]);
});

test("closing to tray while a read is queued leaves the subsequent messages unread", t => {
  const f = readFixture(t);
  f.schedule();
  f.document.focused = false;
  f.document.visibilityState = "hidden";
  f.window.dispatch("blur");
  f.setConversation({ conversationID: "single-A-B", unreadCount: 3, resetGroupAt: true });
  t.mock.timers.tick(150);
  f.schedule();
  t.mock.timers.tick(150);
  assert.deepEqual(f.calls, []);
});

test("a visible but unfocused PC window does not report read", t => {
  const f = readFixture(t);
  f.document.focused = false;
  f.schedule();
  t.mock.timers.tick(150);
  assert.deepEqual(f.calls, []);
});

test("restoring and focusing the same conversation can read its pending messages", t => {
  const f = readFixture(t);
  f.document.focused = false;
  f.schedule();
  t.mock.timers.tick(150);
  assert.deepEqual(f.calls, []);
  f.document.focused = true;
  f.window.dispatch("focus");
  f.schedule();
  t.mock.timers.tick(150);
  assert.deepEqual(f.calls, [["read", "single-A-B"]]);
});

test("effect cleanup cancels a queued receipt without marking anything", t => {
  const f = readFixture(t);
  const cleanup = f.schedule();
  cleanup();
  cleanup();
  t.mock.timers.tick(1000);
  assert.deepEqual(f.calls, []);
});

test("switching to another conversation cannot read the former or new chat through an old task", t => {
  const f = readFixture(t);
  f.schedule();
  f.setConversation({ conversationID: "single-A-C", unreadCount: 7, resetGroupAt: true });
  t.mock.timers.tick(150);
  assert.deepEqual(f.calls, []);
});

test("live unread state is checked when queued work runs", t => {
  const f = readFixture(t);
  f.schedule();
  f.setConversation({ conversationID: "single-A-B", unreadCount: 0, resetGroupAt: false });
  t.mock.timers.tick(150);
  assert.deepEqual(f.calls, []);
});

test("a foreground group mention resets only in that active conversation", t => {
  const f = readFixture(t);
  f.setConversation({ conversationID: "single-A-B", unreadCount: 0, resetGroupAt: true });
  f.schedule();
  t.mock.timers.tick(150);
  assert.deepEqual(f.calls, [["at", "single-A-B"]]);
});

test("failed SDK receipt remains retryable without an unhandled rejection", async t => {
  const f = readFixture(t);
  let attempts = 0;
  f.schedule({ markRead: async () => { attempts++; throw new Error("offline"); } });
  t.mock.timers.tick(150);
  await Promise.resolve();
  f.schedule();
  t.mock.timers.tick(150);
  assert.equal(attempts, 1);
  assert.deepEqual(f.calls, [["read", "single-A-B"]]);
});

test("group receipt is sent with the prepared payload while the message stays visible", async () => {
  const payload = { conversationID: "group-A", clientMsgIDList: ["message-1"] };
  const sent = [];
  const result = await sendVisibleReadReceipt(() => true, async () => payload, async body => {
    sent.push(body);
    return { ok: true };
  });
  assert.deepEqual(sent, [payload]);
  assert.deepEqual(result, { ok: true });
});

test("background group messages never even start the asynchronous cache lookup", async () => {
  let lookups = 0;
  let sends = 0;
  await assert.rejects(sendVisibleReadReceipt(() => false, async () => { lookups++; }, async () => { sends++; }), ReadReceiptCancelled);
  assert.equal(lookups, 0);
  assert.equal(sends, 0);
});

test("closing the window during WASM cache lookup cancels the group receipt", async () => {
  const { document, activity } = activityFixture();
  let finishLookup;
  const sent = [];
  const lookup = new Promise(resolve => { finishLookup = resolve; });
  const receipt = sendVisibleReadReceipt(activity.getSnapshot, () => lookup, async body => sent.push(body));
  document.focused = false;
  document.visibilityState = "hidden";
  finishLookup({ conversationID: "group-A" });
  await assert.rejects(receipt, ReadReceiptCancelled);
  assert.deepEqual(sent, []);
});

test("switching chats or unmounting during group lookup cancels the pending operation", async () => {
  for (const change of ["switch", "unmount"]) {
    let activeConversation = "group-A";
    let mounted = true;
    let finishLookup;
    const sent = [];
    const receipt = sendVisibleReadReceipt(
      () => mounted && activeConversation === "group-A",
      () => new Promise(resolve => { finishLookup = resolve; }),
      async body => sent.push(body),
    );
    if (change === "switch") activeConversation = "group-B";
    else mounted = false;
    finishLookup({ conversationID: "group-A" });
    await assert.rejects(receipt, ReadReceiptCancelled);
    assert.deepEqual(sent, []);
  }
});
