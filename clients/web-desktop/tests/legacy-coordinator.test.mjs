import test from "node:test";
import assert from "node:assert/strict";
import { build } from "esbuild";
import { createRequire } from "node:module";
import { randomBytes } from "node:crypto";
import { mkdtemp, mkdir, writeFile, readFile, readdir, rm, stat, unlink } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
const requireSDK = createRequire(path.resolve("../../sdks/wasm/package.json"));
const SQL = await requireSDK("@jlongster/sql.js")({ wasmBinary: await readFile("../../sdks/wasm/assets/sql-wasm.wasm") });
async function moduleAt(file) { const compiled = await build({ entryPoints: [file], bundle: true, write: false, format: "esm", platform: "node", external: ["electron"] }); return import("data:text/javascript;base64," + Buffer.from(compiled.outputFiles[0].text).toString("base64")); }
const api = await moduleAt("electron/main/localStore/legacyCoordinator.ts"), legacy = await moduleAt("electron/main/localStore/legacy.ts"), vaultModule = await moduleAt("electron/main/localStore/vault.ts"), crypto = await moduleAt("electron/main/localStore/crypto.ts");
const account = "emp_" + "a".repeat(32), other = "emp_" + "b".repeat(32), check = async () => {};
const osKey = randomBytes(32), protection = { available: async () => true, wrap: async value => crypto.seal(osKey, { account: "synthetic", type: "os-wrap", objectID: "master" }, Buffer.from(value)), unwrap: async value => crypto.unseal(osKey, { account: "synthetic", type: "os-wrap", objectID: "master" }, value).toString() };
function database(source, who = account) {
  const db = new SQL.Database(); db.exec("PRAGMA page_size=8192;CREATE TABLE local_users(user_id TEXT);CREATE TABLE local_conversations(conversation_id TEXT,is_msg_destruct INTEGER); INSERT INTO local_conversations VALUES('si_synthetic',0); CREATE TABLE chat_logs_si_synthetic(client_msg_id TEXT PRIMARY KEY,send_time INTEGER,status INTEGER,seq INTEGER,is_read INTEGER,content_type INTEGER,content TEXT,local_ex TEXT,attached_info TEXT,ex TEXT,send_id TEXT);"); db.run("INSERT INTO local_users VALUES(?)", [who]);
  const file = path.join(source, "OpenIMData", who, "sentFileCache", "original.txt");
  db.run("INSERT INTO chat_logs_si_synthetic VALUES(?,?,?,?,?,?,?,?,?,?,?)", ["local-1", 1700000000000, 2, 1, 1, 105, JSON.stringify({ filePath: file, fileName: "员工原文件.txt", sourceUrl: "https://example.invalid/stable" }), file, "{}", "", who]);
  db.run("INSERT INTO chat_logs_si_synthetic VALUES(?,?,?,?,?,?,?,?,?,?,?)", ["removed", 1700000000001, 4, 2, 1, 105, "removed-body", file, "{}", "removed-ex", who]);
  const bytes = db.export(); db.close(); const pages = [{ key: -1, value: { size: bytes.length } }]; for (let offset = 0; offset < bytes.length; offset += 8192) pages.push({ key: offset / 8192, value: bytes.slice(offset, offset + 8192) }); return { name: `-sql-${who}.sqlite`, version: 2, stores: ["data"], pages };
}
async function fixture() {
  const directory = await mkdtemp(path.join(tmpdir(), "puty-legacy-coordinator-")), source = path.join(directory, "old"), root = path.join(directory, "encrypted"), currentUserData = path.join(directory, "current");
  await Promise.all([mkdir(source), mkdir(root), mkdir(currentUserData)]);
  for (const who of [account, other]) { const cache = path.join(source, "OpenIMData", who, "sentFileCache"); await mkdir(cache, { recursive: true }); await writeFile(path.join(cache, "original.txt"), "SYNTHETIC-LEGACY-CACHE-" + who); }
  await mkdir(path.join(source, "Partitions", "part0", "IndexedDB"), { recursive: true }); await writeFile(path.join(source, "Partitions", "part0", "IndexedDB", "raw.leveldb"), "SYNTHETIC-UNKNOWN-RAW-IDB");
  await mkdir(path.join(source, "OpenIMData", "logs")); await writeFile(path.join(source, "OpenIMData", "logs", "old.log"), "SYNTHETIC-PRIVATE-LOG");
  const raw = [database(source), database(source, other), { name: "unrecognized", version: 9, stores: ["opaque"] }], readout = legacy.planLegacyImport(source, raw, await legacy.inventoryLegacyCaches(source));
  const options = { source, root, currentUserData, writersStopped: true, protection, SQL, assertCurrent: check };
  return { directory, source, root, options, readout, close: () => rm(directory, { recursive: true, force: true }) };
}
async function vaultAt(root, who = account, validate = check) { return vaultModule.LocalVault.open(path.join(root, "accounts", who, "vault"), who, Buffer.alloc(32, 17), validate); }
test("all accounts, unknown IDB and logs are encrypted and verified without touching the source", async () => {
  const f = await fixture(); try {
    const before = await readdir(f.source, { recursive: true }), receipt = await api.preserveLegacyData({ ...f.options, readout: f.readout });
    assert.equal(receipt.verified, true); assert.equal(receipt.databases, 2); assert.equal(receipt.quarantined, 1); assert.equal(receipt.files, 4); assert.equal(receipt.plaintextCleanupComplete, false); assert.deepEqual(await readdir(f.source, { recursive: true }), before);
    const files = await readdir(path.join(f.root, "legacy"), { recursive: true });
    for (const file of files) { const target = path.join(f.root, "legacy", file); if ((await stat(target)).isFile()) { const bytes = await readFile(target); assert.equal(bytes.includes(Buffer.from("SYNTHETIC-")), false); assert.equal(bytes.includes(Buffer.from(account)), false); } }
    const listed = await api.listLegacySnapshots(f.root, protection); assert.equal(listed.length, 1); assert.equal(listed[0].snapshotID, receipt.snapshotID);
  } finally { await f.close(); }
});
test("current account imports real SQLite, monthly events and sent cache, later account stays isolated", async () => {
  const f = await fixture(); let vault, second; try {
    const receipt = await api.preserveLegacyData({ ...f.options, readout: f.readout }); vault = await vaultAt(f.root);
    const imported = await api.importLegacyAccount({ root: f.root, snapshotID: receipt.snapshotID, accountID: account, vault, protection, SQL, assertCurrent: check });
    assert.equal(imported.imported, true); assert.equal(imported.attachments, 1); assert.equal((await vault.stats()).messages, 1);
    const messages = await vault.history("si_synthetic"); assert.equal(messages.length, 1); assert.match(messages[0].local_ex, /^puty-local:\/\/attachment\/legacy_[a-f0-9]{64}$/); assert.equal(JSON.parse(messages[0].content).filePath, messages[0].local_ex);
    const entry = vault.attachmentForPath(messages[0].local_ex); assert.equal(entry.sourceName, "员工原文件.txt"); assert.equal((await vault.readAttachment(entry.taskID)).toString(), "SYNTHETIC-LEGACY-CACHE-" + account);
    const sdk = await vault.restoreSDK(), db = new SQL.Database(Buffer.from(sdk.snapshot, "base64")); assert.equal(db.exec("PRAGMA integrity_check")[0].values[0][0], "ok"); assert.deepEqual(db.exec("SELECT content,local_ex,ex FROM chat_logs_si_synthetic WHERE client_msg_id='removed'")[0].values[0], ["", "", ""]); db.close();
    const again = await api.importLegacyAccount({ root: f.root, snapshotID: receipt.snapshotID, accountID: account, vault, protection, SQL, assertCurrent: check }); assert.equal(again.alreadyImported, true); assert.equal((await vault.stats()).messages, 1);
    assert.equal((await readdir(path.join(f.root, "accounts"))).includes(other), false);
    second = await vaultAt(f.root, other); await api.importLegacyAccount({ root: f.root, snapshotID: receipt.snapshotID, accountID: other, vault: second, protection, SQL, assertCurrent: check }); assert.equal((await second.history("si_synthetic"))[0].send_id, other);
  } finally { vault?.lock(); second?.lock(); await f.close(); }
});
test("cleanup requires inactive distinct source and all backup hashes, then removes only exact old files", async () => {
  const f = await fixture(); try {
    const receipt = await api.preserveLegacyData({ ...f.options, readout: f.readout });
    await assert.rejects(api.cleanupLegacyPlaintext({ ...f.options, currentUserData: f.source, snapshotID: receipt.snapshotID }), /Active app/); assert.equal((await readdir(f.source)).length > 0, true);
    const complete = await api.cleanupLegacyPlaintext({ ...f.options, snapshotID: receipt.snapshotID }); assert.equal(complete.plaintextCleanupComplete, true); assert.deepEqual(await readdir(f.source), []); assert.equal((await api.listLegacySnapshots(f.root, protection)).length, 1);
  } finally { await f.close(); }
});
test("changed source or tampered encrypted object never authorizes cleanup", async () => {
  const f = await fixture(); try {
    const receipt = await api.preserveLegacyData({ ...f.options, readout: f.readout }); await writeFile(path.join(f.source, "new.log"), "SYNTHETIC-ACTIVE-WRITER");
    await assert.rejects(api.cleanupLegacyPlaintext({ ...f.options, snapshotID: receipt.snapshotID }), /inventory changed/); await unlink(path.join(f.source, "new.log"));
    const objects = path.join(f.root, "legacy", receipt.snapshotID, "objects"), names = await readdir(objects), target = path.join(objects, names[0]), bytes = await readFile(target); bytes[bytes.length - 1] ^= 1; await writeFile(target, bytes);
    await assert.rejects(api.cleanupLegacyPlaintext({ ...f.options, snapshotID: receipt.snapshotID })); assert.ok((await readdir(f.source)).length);
  } finally { await f.close(); }
});
test("OS-wrap unavailable, wrong account or lost key fail closed and leave source intact", async () => {
  const f = await fixture(); let vault; try {
    await assert.rejects(api.preserveLegacyData({ ...f.options, readout: f.readout, protection: { ...protection, available: async () => false } }), /unavailable/); assert.ok((await readdir(f.source)).length);
    const receipt = await api.preserveLegacyData({ ...f.options, readout: f.readout }); vault = await vaultAt(f.root);
    await assert.rejects(api.importLegacyAccount({ root: f.root, snapshotID: receipt.snapshotID, accountID: other, vault, protection, SQL, assertCurrent: check }), /active vault/);
    await unlink(path.join(f.root, "legacy", "master.oswrap")); await assert.rejects(api.listLegacySnapshots(f.root, protection), /without its OS-wrapped/); assert.ok((await readdir(f.source)).length);
  } finally { vault?.lock(); await f.close(); }
});
test("nonempty vault is never overwritten and a failed online lease prevents import", async () => {
  const f = await fixture(); let vault; try {
    const receipt = await api.preserveLegacyData({ ...f.options, readout: f.readout }); vault = await vaultAt(f.root); await vault.checkpoint(new Uint8Array([1, 2, 3]));
    await assert.rejects(api.importLegacyAccount({ root: f.root, snapshotID: receipt.snapshotID, accountID: account, vault, protection, SQL, assertCurrent: check }), /Existing vault/);
    await assert.rejects(api.importLegacyAccount({ root: f.root, snapshotID: receipt.snapshotID, accountID: account, vault, protection, SQL, assertCurrent: async () => { throw new Error("lease revoked"); } }), /lease revoked/); assert.equal((await vault.restoreSDK()).snapshot, Buffer.from([1, 2, 3]).toString("base64"));
  } finally { vault?.lock(); await f.close(); }
});
test("batched import resumes after partial materialization without duplicate archive entries", async () => {
  const f = await fixture(); let vault; try {
    vault = await vaultAt(f.root); const db = new SQL.Database(); db.exec("CREATE TABLE test(id)"); const snapshot = db.export(); db.close();
    const events = Array.from({ length: 1001 }, (_, index) => ({ kind: "upsert", conversationID: "si_synthetic", row: { client_msg_id: "message-" + index, send_time: 1700000000000, content: "synthetic", status: 2 } })), original = vault.apply.bind(vault); let fails = true;
    vault.apply = async journal => { if (fails && journal.commit.events?.[0]?.row.client_msg_id === "message-1000") { fails = false; throw new Error("synthetic materialization failure"); } return original(journal); };
    await assert.rejects(vault.importLegacy(snapshot, events, "c".repeat(64)), /synthetic materialization/); vault.lock();
    vault = await vaultAt(f.root); const result = await vault.importLegacy(snapshot, events, "c".repeat(64)); assert.equal(result.messages, 1001); assert.equal((await vault.stats()).messages, 1001); assert.equal((await vault.importLegacy(snapshot, events, "c".repeat(64))).alreadyImported, true); await assert.rejects(vault.importLegacy(snapshot, events, "d".repeat(64)), /durable source marker/);
  } finally { vault?.lock(); await f.close(); }
});
test("a complete pointer remains authoritative when the later convenience marker fails", async () => {
  const f = await fixture(); let vault; try {
    const receipt = await api.preserveLegacyData({ ...f.options, readout: f.readout }); vault = await vaultAt(f.root); const original = vault.preferences.bind(vault); let fails = true;
    vault.preferences = async value => { if (fails && value && Object.values(value).some(item => item?.state === "complete")) { fails = false; throw new Error("synthetic convenience marker failure"); } return original(value); };
    await assert.rejects(api.importLegacyAccount({ root: f.root, snapshotID: receipt.snapshotID, accountID: account, vault, protection, SQL, assertCurrent: check }), /convenience marker/); vault.lock();
    vault = await vaultAt(f.root); const retried = await api.importLegacyAccount({ root: f.root, snapshotID: receipt.snapshotID, accountID: account, vault, protection, SQL, assertCurrent: check }); assert.equal(retried.alreadyImported, true); assert.equal((await vault.stats()).messages, 1); assert.equal((await vault.stats()).attachments, 1);
  } finally { vault?.lock(); await f.close(); }
});
test("cleanup interruption resumes exact files and a large encrypted source streams completely", async () => {
  const f = await fixture(); try {
    await writeFile(path.join(f.source, "large.synthetic"), Buffer.alloc(2 * 1024 * 1024 + 7, 77)); const receipt = await api.preserveLegacyData({ ...f.options, readout: f.readout });
    let calls = 0; await assert.rejects(api.cleanupLegacyPlaintext({ ...f.options, snapshotID: receipt.snapshotID, assertCurrent: async () => { if (++calls === 24) throw new Error("synthetic cleanup interruption"); } }), /interruption/);
    const resumed = await api.cleanupLegacyPlaintext({ ...f.options, snapshotID: receipt.snapshotID }); assert.equal(resumed.plaintextCleanupComplete, true); assert.deepEqual(await readdir(f.source), []); assert.equal((await api.listLegacySnapshots(f.root, protection))[0].plaintextCleanupComplete, true);
  } finally { await f.close(); }
});
test("more than 100000 events are all batched, no silent truncation", async () => {
  const f = await fixture(); let vault; try {
    vault = await vaultAt(f.root); const db = new SQL.Database(); db.exec("CREATE TABLE test(id)"); const snapshot = db.export(); db.close();
    const events = Array.from({ length: 100001 }, (_, index) => ({ kind: "upsert", conversationID: "si_synthetic", row: { client_msg_id: "synthetic-" + index, send_time: 1700000000000 } })); let seen = 0, largest = 0;
    // Isolate batch/commit behavior from 100001 file fsyncs; the preceding test
    // exercises actual encrypted materialization and process reopen.
    vault.apply = async journal => { seen += journal.commit.events?.length || 0; largest = Math.max(largest, journal.commit.events?.length || 0); if (journal.preferences) vault.index.preferences = journal.preferences; vault.index.sequence = journal.sequence; };
    const result = await vault.importLegacy(snapshot, events, "e".repeat(64)); assert.equal(result.messages, 100001); assert.equal(seen, 100001); assert.equal(largest, 1000); assert.equal((await vault.restoreSDK()).snapshot, Buffer.from(snapshot).toString("base64"));
  } finally { vault?.lock(); await f.close(); }
});
test("duplicate third SQLite claim remains quarantined rather than being chosen", () => {
  const db = new SQL.Database(); db.exec("PRAGMA page_size=8192; CREATE TABLE test(id)"); const bytes = db.export(); db.close(); const pages = [{ key: -1, value: { size: bytes.length } }]; for (let n = 0; n < bytes.length; n += 8192) pages.push({ key: n / 8192, value: bytes.slice(n, n + 8192) });
  const raw = ["", "first-", "second-"].map(prefix => ({ name: `-sql-${prefix}${account}.sqlite`, version: 2, stores: ["data"], pages })), result = legacy.planLegacyImport("synthetic", raw, []); assert.equal(result.databases.length, 0); assert.equal(result.unresolved.length, 2);
});
test("declared A database containing B login identity is sealed as quarantine and never imported", async () => {
 const f = await fixture(); let vault; try {
  const wrong = database(f.source, other); wrong.name = `-sql-${account}.sqlite`; const readout = legacy.planLegacyImport(f.source, [wrong], []), receipt = await api.preserveLegacyData({ ...f.options, readout }); assert.equal(receipt.databases, 0); assert.equal(receipt.quarantined, 1);
  vault = await vaultAt(f.root); await assert.rejects(api.importLegacyAccount({ root: f.root, snapshotID: receipt.snapshotID, accountID: account, vault, SQL, protection, assertCurrent: check }), /quarantined/); assert.equal((await vault.stats()).messages, 0);
  const db = new SQL.Database(legacy.reconstructLegacySQLite(wrong.name, wrong.pages).bytes); db.exec("DELETE FROM local_users"); const noIdentity = db.export(); db.close(); assert.throws(() => api.verifyLegacySQLite(SQL, noIdentity, account), /no verifiable login identity/);
 } finally { vault?.lock(); await f.close(); }
});


test("a fully removed old source completes only after authenticated backup verification and still imports its sealed account", async () => {
 const f=await fixture();let vault;try{
  const saved=await api.preserveLegacyData({...f.options,readout:f.readout});
  const base=path.join(f.root,"legacy",saved.snapshotID),names=await readdir(base,{recursive:true}),before=new Map();
  for(const name of names){const p=path.join(base,name);if((await stat(p)).isFile())before.set(name,await readFile(p));}
  await rm(f.source,{recursive:true});
  assert.equal((await api.cleanupLegacyPlaintext({...f.options,snapshotID:saved.snapshotID})).plaintextCleanupComplete,true);
  for(const [name,bytes] of before)assert.deepEqual(await readFile(path.join(base,name)),bytes);
  await assert.rejects(stat(f.source),{code:"ENOENT"});
  vault=await vaultAt(f.root);const imported=await api.importLegacyAccount({root:f.root,snapshotID:saved.snapshotID,accountID:account,vault,protection,SQL,assertCurrent:check});assert.equal(imported.imported,true);assert.equal((await vault.stats()).messages,1);
 }finally{vault?.lock();await f.close();}
});
test("missing source never bypasses tampered encrypted backup and completed cleanup can reenter after root removal", async()=>{
 const f=await fixture();try{
  const saved=await api.preserveLegacyData({...f.options,readout:f.readout});await rm(f.source,{recursive:true});
  const objects=path.join(f.root,"legacy",saved.snapshotID,"objects"),target=path.join(objects,(await readdir(objects))[0]),bytes=await readFile(target),damaged=Buffer.from(bytes);damaged[damaged.length-1]^=1;await writeFile(target,damaged);
  await assert.rejects(api.cleanupLegacyPlaintext({...f.options,snapshotID:saved.snapshotID}));assert.equal((await api.listLegacySnapshots(f.root,protection))[0].plaintextCleanupComplete,false);
  await writeFile(target,bytes);await api.cleanupLegacyPlaintext({...f.options,snapshotID:saved.snapshotID});
  assert.equal((await api.cleanupLegacyPlaintext({...f.options,snapshotID:saved.snapshotID})).plaintextCleanupComplete,true);
 }finally{await f.close();}
});
test("a missing-source cleanup rejects a replacement linked root and never touches the target", async()=>{
 const f=await fixture();const {symlink}=await import('node:fs/promises');try{
  const saved=await api.preserveLegacyData({...f.options,readout:f.readout});await rm(f.source,{recursive:true});const outside=path.join(f.directory,'foreign');await mkdir(outside);await writeFile(path.join(outside,'synthetic.txt'),'SYNTHETIC_FOREIGN');
  await symlink(outside,f.source,process.platform==='win32'?'junction':'dir');await assert.rejects(api.cleanupLegacyPlaintext({...f.options,snapshotID:saved.snapshotID}),/linked/);assert.equal((await readFile(path.join(outside,'synthetic.txt'))).toString(),'SYNTHETIC_FOREIGN');
 }finally{await f.close();}
});
test("native lock exemption rejects nonempty metadata and completed cleanup rejects new business files", async()=>{
 const f=await fixture();try{
  await writeFile(path.join(f.source,'lockfile'),'SYNTHETIC_NOT_NATIVE');const saved=await api.preserveLegacyData({...f.options,readout:f.readout});
  if(process.platform==='win32')for(const options of [{nativeSingletonLockHeld:()=>true},{}]){await assert.rejects(api.cleanupLegacyPlaintext({...f.options,snapshotID:saved.snapshotID,...options}),/native lock metadata/);assert.equal((await readFile(path.join(f.source,'lockfile'))).toString(),'SYNTHETIC_NOT_NATIVE');}
 }finally{await f.close();}
 const fresh=await fixture();try{
  const saved=await api.preserveLegacyData({...fresh.options,readout:fresh.readout});await api.cleanupLegacyPlaintext({...fresh.options,snapshotID:saved.snapshotID});await writeFile(path.join(fresh.source,'new-business.txt'),'SYNTHETIC_NEW');
  await assert.rejects(api.cleanupLegacyPlaintext({...fresh.options,snapshotID:saved.snapshotID}),/inventory changed/);assert.equal((await readFile(path.join(fresh.source,'new-business.txt'))).toString(),'SYNTHETIC_NEW');
 }finally{await fresh.close();}
});
test("a removed old profile recreated as an empty directory does not invalidate its sealed snapshot",async()=>{
 const f=await fixture();try{const saved=await api.preserveLegacyData({...f.options,readout:f.readout});await rm(f.source,{recursive:true});await mkdir(f.source);assert.equal((await api.cleanupLegacyPlaintext({...f.options,snapshotID:saved.snapshotID})).plaintextCleanupComplete,true);assert.deepEqual(await readdir(f.source),[]);}finally{await f.close();}
});

test("root native lock links, hardlinks and missing or lost ownership preserve every business source",{skip:process.platform!=='win32'},async()=>{
 const f=await fixture();const {symlink,link}=await import('node:fs/promises');try{
  const saved=await api.preserveLegacyData({...f.options,readout:f.readout}),target=path.join(f.directory,'native-empty.synthetic'),lock=path.join(f.source,'lockfile');await writeFile(target,'');
  for(const kind of ['symlink','hardlink','unowned','lost-owner']){
   if(kind==='symlink')await symlink(target,lock,'file');else if(kind==='hardlink')await link(target,lock);else await writeFile(lock,'');
   let calls=0;const nativeSingletonLockHeld=()=>kind==='lost-owner'?++calls===1:false;
   await assert.rejects(api.cleanupLegacyPlaintext({...f.options,snapshotID:saved.snapshotID,nativeSingletonLockHeld}));
   assert.equal((await readFile(path.join(f.source,'OpenIMData',account,'sentFileCache','original.txt'))).toString(),'SYNTHETIC-LEGACY-CACHE-'+account);await unlink(lock);
  }
 }finally{await f.close();}
});
