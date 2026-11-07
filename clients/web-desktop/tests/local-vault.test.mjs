import test from "node:test";
import assert from "node:assert/strict";
import { build } from "esbuild";
import { mkdtemp, readFile, readdir, writeFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
const compiled = await build({ entryPoints: ["electron/main/localStore/vault.ts", "electron/main/localStore/crypto.ts", "electron/main/localStore/names.ts","electron/main/localStore/backup.ts"], outdir: "unused", bundle: true, write: false, platform: "node", format: "esm" });
const loaded = await Promise.all(compiled.outputFiles.map(file => import(`data:text/javascript;base64,${Buffer.from(file.text + "\n//# sourceURL=" + path.basename(file.path)).toString("base64")}`)));
const vaultModule = loaded.find(v => v.LocalVault), crypto = loaded.find(v => v.seal), names = loaded.find(v => v.numberedName);
const { LocalVault } = vaultModule;
const backup=loaded.find(v=>v.restoreBackup);
const key = Buffer.alloc(32, 17), account = "test_synthetic_only";
async function fixture(t) {
  const directory = await mkdtemp(path.join(tmpdir(), "puty-vault-"));
  let allowed = true;
  const validate = async () => { if (!allowed) throw new Error("synthetic access revoked"); };
  const vault = await LocalVault.open(directory, account, key, validate);
  t.after(async () => { vault.lock(); await rm(directory, { recursive: true, force: true }); });
  return { directory, vault, validate, revoke: () => { allowed = false; } };
}
const message = (id, conv = "si_test", sendTime = Date.parse("2026-11-04T01:00:00Z")) => ({ kind: "upsert", conversationID: conv, row: { client_msg_id: id, content: "SYNTHETIC_PRIVATE_MESSAGE", send_time: sendTime, status: 2 } });
test("chunked encryption authenticates context, every frame, terminal marker and recovery password", () => {
  const ctx = { account, type: "attachment", objectID: "synthetic-1" }, body = Buffer.alloc(crypto.CHUNK_SIZE * 2 + 37, 65);
  const sealed = crypto.seal(key, ctx, body);
  assert.deepEqual(crypto.unseal(key, ctx, sealed), body);
  assert.ok(!sealed.includes(Buffer.alloc(32, 65)));
  for (const bad of [sealed.subarray(0, -1), Buffer.concat([sealed, Buffer.from([0])])]) assert.throws(() => crypto.unseal(key, ctx, bad));
  const modified = Buffer.from(sealed); modified[modified.length - 50] ^= 1;
  assert.throws(() => crypto.unseal(key, ctx, modified));
  assert.throws(() => crypto.unseal(key, { ...ctx, account: "test_other" }, sealed));
  const backup = crypto.exportRecovery(key, account, "Synthetic recovery passphrase");
  assert.deepEqual(crypto.importRecovery(backup, account, "Synthetic recovery passphrase"), key);
  assert.throws(() => crypto.importRecovery(backup, account, "Wrong recovery passphrase"));
});
test("real encrypted filenames use source names and atomic full-width numbering without .enc", async t => {
  const { directory, vault } = await fixture(t), refs = [{ conversationID: "si_test", clientMsgID: "m1" }];
  const allocated = await Promise.all(Array.from({ length: 6 }, (_, n) => vault.reserveAttachment({ taskID: `task-${n}`, name: "职员名单.xlsx", messageRefs: refs })));
  assert.deepEqual(allocated.map(v => v.name), ["职员名单.xlsx", ...Array.from({ length: 5 }, (_, n) => `职员名单（${n + 1}）.xlsx`)]);
  const retry = await vault.reserveAttachment({ taskID: "task-1", name: "other.xlsx", messageRefs: refs }); assert.equal(retry.name, "职员名单（1）.xlsx");
  for (const v of allocated) assert.equal((await readFile(path.join(directory, v.file))).length, 0);
  const otherDirectory = path.join(directory, "another-month");
  assert.equal((await names.reserveName(otherDirectory, "职员名单.xlsx")).name, "职员名单.xlsx");
  assert.equal(names.numberedName("NOEXT", 2), "NOEXT（2）");
  assert.equal(names.safeSourceName("../../folder\\list.xlsx"), "list.xlsx");
});
test("streamed encrypted attachment has authenticated byte ranges and never becomes ready before complete", async t => {
  const { directory, vault } = await fixture(t), payload = Buffer.alloc(crypto.CHUNK_SIZE + 211, 97);
  const entry = await vault.reserveAttachment({ taskID: "stream-1", name: "视频.mp4", messageRefs: [{ conversationID: "si_test", clientMsgID: "m1" }] });
  assert.equal(await vault.attachment("stream-1"), null);
  async function* source() { yield payload.subarray(0, 73); yield payload.subarray(73); }
  await vault.writeAttachment("stream-1", source(), payload.length);
  const stored = await readFile(path.join(directory, entry.file)); assert.ok(!stored.includes(Buffer.alloc(32, 97)));
  assert.deepEqual(await vault.readAttachment("stream-1", crypto.CHUNK_SIZE - 10, crypto.CHUNK_SIZE + 10), payload.subarray(crypto.CHUNK_SIZE - 10, crypto.CHUNK_SIZE + 11));
  stored[stored.length - 1] ^= 1; await writeFile(path.join(directory, entry.file), stored);
  await assert.rejects(vault.readAttachment("stream-1", 0, 10));
});
test("interrupted transfer retains its own reserved source name, retries never reuse another file", async t => {
  const { vault } = await fixture(t);
  await vault.reserveAttachment({ taskID: "retry-file", name: "same.bin", messageRefs: [{ conversationID: "si_test", clientMsgID: "m1" }] });
  async function* failed() { yield Buffer.from("synthetic part"); throw new Error("interrupted"); }
  await assert.rejects(vault.writeAttachment("retry-file", failed()), /interrupted/); assert.equal(await vault.attachment("retry-file"), null);
  async function* success() { yield Buffer.from("complete"); }
  const saved = await vault.writeAttachment("retry-file", success()); assert.equal(saved.name, "same.bin");
  const another = await vault.reserveAttachment({ taskID: "different-file", name: "same.bin", messageRefs: [{ conversationID: "si_test", clientMsgID: "m2" }] }); assert.equal(another.name, "same（1）.bin");
});
test("durable SDK SQL journal and month archive recover without a successful shutdown", async t => {
  const { directory, vault, validate } = await fixture(t);
  const commit = { opID: "synthetic-op-1", statements: [{ sql: "INSERT INTO synthetic VALUES (?)", bindings: ["SYNTHETIC_SQL_VALUE"] }], events: [message("m1")] };
  const first = await vault.commit(commit), duplicate = await vault.commit(commit); assert.equal(first.sequence, duplicate.sequence);
  vault.lock();
  const reopened = await LocalVault.open(directory, account, key, validate); t.after(() => reopened.lock());
  const restored = await reopened.restoreSDK(); assert.equal(restored.entries.length, 1); assert.deepEqual(restored.entries[0].statements, commit.statements);
  assert.equal((await reopened.history("si_test"))[0].client_msg_id, "m1");
  const journal = await readFile(path.join(directory, "sdk/journal/0000000000000001")); assert.ok(!journal.includes(Buffer.from("SYNTHETIC_SQL_VALUE")));
});
test("local conversation/date cleanup compacts payloads, respects shared attachment refs and survives replay", async t => {
  const { directory, vault, validate } = await fixture(t);
  await vault.commit({ opID: "initial", statements: [], events: [message("old", "si_a", Date.parse("2026-10-31T15:59:59Z")), message("keep", "si_a", Date.parse("2026-10-31T16:00:00Z")), message("other", "si_b")] });
  await vault.reserveAttachment({ taskID: "shared", name: "共享.xlsx", messageRefs: [{ conversationID: "si_a", clientMsgID: "old" }, { conversationID: "si_b", clientMsgID: "other" }] });
  async function* source() { yield Buffer.from("SYNTHETIC_ATTACHMENT"); }
  await vault.writeAttachment("shared", source());
  const preview = await vault.previewCleanup({ conversationIDs: ["si_a"], beforeDate: "2026-11-01" }); assert.equal(preview.messages, 1); assert.equal(preview.attachments, 0);
  await vault.clean({ conversationIDs: ["si_a"], beforeDate: "2026-11-01" }, Buffer.from("synthetic cleaned SDK checkpoint"));
  assert.equal((await vault.history("si_a")).length, 1); assert.ok(await vault.attachment("shared"));
  assert.deepEqual(await readdir(path.join(directory, "sdk/journal")), []);
  await vault.commit({ opID: "late-sync", statements: [], events: [message("old", "si_a", Date.parse("2026-10-31T15:59:59Z"))] });
  assert.equal((await vault.history("si_a")).length, 1);
  vault.lock(); const reopened = await LocalVault.open(directory, account, key, validate); t.after(() => reopened.lock());
  assert.equal((await reopened.history("si_a")).length, 1); assert.equal((await reopened.restoreSDK()).tombstones.length, 1);
});
test("revocation rejects reads and writes instead of offering offline access", async t => {
  const { vault, revoke } = await fixture(t); await vault.commit({ opID: "before-revoke", statements: [], events: [message("m1")] });
  revoke(); await assert.rejects(vault.history("si_test"), /revoked/); await assert.rejects(vault.commit({ opID: "after-revoke", statements: [] }), /revoked/);
});
test('authenticated Range resume reuses task name after restart and rotates nonces; source change restarts safely',async t=>{
  const {directory,vault,validate}=await fixture(t),body=Buffer.alloc(crypto.CHUNK_SIZE*2+79,71);
  const entry=await vault.reserveAttachment({taskID:'range',name:'原名.xlsx',messageRefs:[{conversationID:'si_test',clientMsgID:'m1'}]});
  await assert.rejects(vault.writeAttachment('range',async offset=>{assert.equal(offset,0);return {start:0,version:'"v1"',total:body.length,source:(async function*(){yield body.subarray(0,crypto.CHUNK_SIZE+7);throw new Error('synthetic network interrupted');})()};}),/interrupted/);
  const partial=path.join(directory,entry.file+'.part-'+entry.objectID),old=await readFile(partial);assert.ok(old.length>crypto.CHUNK_SIZE);assert.ok(!old.includes(Buffer.alloc(64,71)));
  vault.lock();const reopened=await LocalVault.open(directory,account,key,validate);t.after(()=>reopened.lock());
  const saved=await reopened.writeAttachment('range',async(offset,version)=>{assert.equal(offset,crypto.CHUNK_SIZE);assert.equal(version,'"v1"');return {start:offset,version,total:body.length,source:(async function*(){yield body.subarray(offset);})()};});
  assert.equal(saved.name,'原名.xlsx');const completed=await readFile(path.join(directory,saved.file));assert.notEqual(crypto.parseHeader(old).header.noncePrefix,crypto.parseHeader(completed).header.noncePrefix);assert.deepEqual(crypto.unseal(key,{account,type:'attachment',objectID:entry.objectID},completed),body);
  await reopened.reserveAttachment({taskID:'changed',name:'原名.xlsx',messageRefs:[{conversationID:'si_test',clientMsgID:'m2'}]});
  await assert.rejects(reopened.writeAttachment('changed',async()=>({start:0,version:'"old"',source:(async function*(){yield body.subarray(0,crypto.CHUNK_SIZE);throw new Error('pause');})()})),/pause/);
  const replacement=Buffer.from('different version');const replaced=await reopened.writeAttachment('changed',async(offset,version)=>{assert.equal(offset,crypto.CHUNK_SIZE);assert.equal(version,'"old"');return {start:0,version:'"new"',total:replacement.length,source:(async function*(){yield replacement;})()};});
  assert.equal(replaced.name,'原名（1）.xlsx');assert.deepEqual(await reopened.readAttachment('changed'),replacement);
});
test('complete encrypted backup restores message archive, SDK journal, original attachments and independent master key',async t=>{
  const {directory,vault,validate}=await fixture(t);await vault.commit({opID:'backup-data',statements:[],events:[message('m1')]});await vault.checkpoint(Buffer.from('SYNTHETIC SDK CHECKPOINT'));
  await vault.reserveAttachment({taskID:'backup-file',name:'职员名单.xlsx',messageRefs:[{conversationID:'si_test',clientMsgID:'m1'}]});await vault.writeAttachment('backup-file',(async function*(){yield Buffer.from('SYNTHETIC FILE');})());
  const result=await vault.backup(directory,'Synthetic independent recovery passphrase',async()=>{}),target=path.join(directory,'restored');await import('node:fs/promises').then(fs=>fs.mkdir(target));
  const restoredKey=await backup.restoreBackup(result.directory,target,account,'Synthetic independent recovery passphrase',validate);assert.deepEqual(restoredKey,key);
  const restored=await LocalVault.open(target,account,restoredKey,validate);restoredKey.fill(0);t.after(()=>restored.lock());assert.equal((await restored.history('si_test'))[0].client_msg_id,'m1');assert.equal((await restored.restoreSDK()).snapshot,Buffer.from('SYNTHETIC SDK CHECKPOINT').toString('base64'));assert.equal((await restored.readAttachment('backup-file')).toString(),'SYNTHETIC FILE');
  const manifest=await readFile(path.join(result.directory,'manifest.puty'));assert.ok(!manifest.includes(Buffer.from('SYNTHETIC')));manifest[manifest.length-1]^=1;await writeFile(path.join(result.directory,'manifest.puty'),manifest);const corrupt=path.join(directory,'corrupt');await import('node:fs/promises').then(fs=>fs.mkdir(corrupt));await assert.rejects(backup.restoreBackup(result.directory,corrupt,account,'Synthetic independent recovery passphrase',validate));
});
