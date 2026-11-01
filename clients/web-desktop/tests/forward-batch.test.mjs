import test from "node:test";
import assert from "node:assert/strict";
import { build } from "esbuild";

const compiled = await build({ entryPoints: ["src/im/forward-batch.ts"], bundle: true, write: false, platform: "node", format: "esm" });
const { forwardBatch } = await import(`data:text/javascript;base64,${Buffer.from(compiled.outputFiles[0].text).toString("base64")}`);
const target = userID => ({ userID });
function setup(overrides = {}) {
  let next = 0;
  return {
    targets: [target("a"), target("b"), target("c")], additional: "", progress: new Map(),
    createMessage: async () => ({ id: ++next, kind: "forward" }),
    createAdditional: async text => ({ id: ++next, kind: text }),
    isJoinGroup: async () => true, send: async () => undefined, ...overrides,
  };
}
test("factories, primary sends and notes are awaited in recipient order", async () => {
  const events = []; let running = 0, peak = 0;
  const opts = setup({ additional: "note", send: async (message, recipient) => {
    peak = Math.max(peak, ++running); events.push(recipient.userID + ":" + message.kind);
    await new Promise(resolve => setTimeout(resolve, 5)); running--;
  } });
  const result = await forwardBatch(opts);
  assert.equal(peak, 1); assert.equal(result.successful.length, 3); assert.equal(result.failures.length, 0);
  assert.deepEqual(events, ["a:forward", "a:note", "b:forward", "b:note", "c:forward", "c:note"]);
});
test("one send failure remains failed while later recipients can finish", async () => {
  const result = await forwardBatch(setup({ send: async (_, recipient) => { if (recipient.userID === "b") throw new Error("synthetic send failure"); } }));
  assert.deepEqual(result.successful.map(r => r.userID), ["a", "c"]);
  assert.deepEqual(result.failures.map(r => r.target.userID), ["b"]);
});
test("retrying a failed note does not resend its confirmed primary", async () => {
  const sent = []; let fail = true;
  const opts = setup({ targets: [target("a")], additional: "note", send: async message => {
    sent.push(message.kind); if (message.kind === "note" && fail) { fail = false; throw new Error("synthetic note failure"); }
  } });
  assert.equal((await forwardBatch(opts)).failures.length, 1);
  assert.equal((await forwardBatch(opts)).successful.length, 1);
  assert.deepEqual(sent, ["forward", "note", "note"]);
});
test("retries preserve completed recipients and only repeat the failed target", async () => {
  const sent = []; let fail = true;
  const opts = setup({ send: async (_, recipient) => {
    sent.push(recipient.userID); if (recipient.userID === "b" && fail) { fail = false; throw new Error("synthetic transient"); }
  } });
  await forwardBatch(opts); await forwardBatch(opts);
  assert.deepEqual(sent, ["a", "b", "c", "b"]);
});
test("group membership false rejects that group without creating a message", async () => {
  const checked = [], sent = [];
  const result = await forwardBatch(setup({ targets: [{ groupID: "left" }, target("a"), { groupID: "joined" }],
    isJoinGroup: async groupID => { checked.push(groupID); return groupID === "joined"; },
    send: async (_, recipient) => sent.push(recipient.groupID || recipient.userID),
  }));
  assert.deepEqual(checked, ["left", "joined"]); assert.deepEqual(sent, ["a", "joined"]);
  assert.equal(result.failures[0].target.groupID, "left");
});
test("duplicate recipients produce one forward and empty targets fail visibly", async () => {
  let sent = 0;
  const result = await forwardBatch(setup({ targets: [target("a"), target("a"), {}], send: async () => sent++ }));
  assert.equal(sent, 1); assert.equal(result.successful.length, 1); assert.equal(result.failures.length, 1);
});
test("a failed factory is not mistaken for successful delivery", async () => {
  let sent = 0;
  const result = await forwardBatch(setup({ createMessage: async () => { throw new Error("factory failed"); }, send: async () => sent++ }));
  assert.equal(sent, 0); assert.equal(result.failures.length, 3); assert.equal(result.successful.length, 0);
});
test("the batch promise remains pending until the final send resolves", async () => {
  let release, done = false;
  const opts = setup({ targets: [target("a")], send: () => new Promise(resolve => { release = resolve; }) });
  const pending = forwardBatch(opts).then(result => { done = true; return result; });
  await new Promise(resolve => setImmediate(resolve)); assert.equal(done, false);
  release(); assert.equal((await pending).successful.length, 1); assert.equal(done, true);
});
