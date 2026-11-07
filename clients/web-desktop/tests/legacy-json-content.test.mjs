import test from "node:test";
import assert from "node:assert/strict";
import { build } from "esbuild";
import { createRequire } from "node:module";
import { createHash, randomBytes, randomUUID } from "node:crypto";
import { promises as fs } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";

const requireClient = createRequire(path.resolve("package.json"));
const SQL = await requireClient("@jlongster/sql.js")({ wasmBinary: await fs.readFile("dist/sql-wasm.wasm") });
const localStore = path.resolve("electron/main/localStore");
async function moduleAt(name, beforeImage) {
  const result = await build({
    ...(beforeImage ? { stdin: { contents: await fs.readFile(beforeImage, "utf8"), loader: "ts", resolveDir: localStore } }
      : { entryPoints: [path.join(localStore, name)] }),
    write: false, bundle: true, platform: "node", format: "esm", external: ["electron"],
  });
  return import(`data:text/javascript;base64,${Buffer.from(result.outputFiles[0].text).toString("base64")}#${randomUUID()}`);
}
const api = await moduleAt("legacyCoordinator.ts", process.env.PUTY_LEGACY_JSON_BASELINE);
const retryBeforeImage = process.env.PUTY_LEGACY_JSON_RETRY_BASELINE;
const beforeAPI = retryBeforeImage ? await moduleAt("legacyCoordinator.ts", retryBeforeImage) : undefined;
const { LocalVault } = await moduleAt("vault.ts"), crypto = await moduleAt("crypto.ts");
const account = "emp_" + "a".repeat(32), other = "emp_" + "b".repeat(32), conversation = "si_synthetic_json_legacy";
const deepText = JSON.stringify(Array.from({ length: 40 }).reduce(value => ({ child: value }), { leaf: "ordinary user text" }));
const pathText = JSON.stringify({ filePath: "C:\\SYNTHETIC\\ordinary-user-text.txt", localEx: "ordinary user data" });
const sha = bytes => createHash("sha256").update(bytes).digest("hex");

// Use the actual SDK CREATE TABLE contracts rather than a permissive test schema.
const schemaFiles = ["localUsers.ts", "localConversations.ts", "localChatLogsConversationID.ts"];
const schemas = await Promise.all(schemaFiles.map(async name => {
  const source = await fs.readFile(path.resolve("../../sdks/wasm/src/sqls", name), "utf8");
  const literal = /return db\.exec\(\s*`([\s\S]*?)`\s*\);/.exec(source);
  assert.ok(literal, "Actual SDK CREATE TABLE contract missing: " + name);
  return literal[1].replaceAll("${conversationID}", conversation);
}));
function database(messages = [], latest) {
  const db = new SQL.Database();
  db.exec("PRAGMA page_size=8192;");
  for (const sql of schemas) db.exec(sql);
  db.run("INSERT INTO local_users(user_id) VALUES(?)", [account]);
  if (latest !== undefined) db.run("INSERT INTO local_conversations(conversation_id,latest_msg,is_msg_destruct) VALUES(?,?,0)", [conversation, JSON.stringify(latest)]);
  for (const [index, message] of messages.entries()) {
    db.run(`INSERT INTO chat_logs_${conversation}(client_msg_id,send_time,status,content_type,content,local_ex,attached_info) VALUES(?,?,?,?,?,?,?)`, [
      message.id || "synthetic-" + index, 1700000000000 + index, message.status ?? 2, message.type ?? 101,
      message.content ?? JSON.stringify({ content: message.text ?? "hello" }), message.localEx || "", message.attachedInfo || "{}",
    ]);
  }
  const bytes = db.export(); db.close(); return bytes;
}
function importedRows(prepared) {
  const db = new SQL.Database(prepared.snapshot);
  try { return db.exec(`SELECT client_msg_id,content,local_ex,attached_info FROM chat_logs_${conversation}`)[0]?.values || []; }
  finally { db.close(); }
}

test("normal SDK text JSON remains opaque even when its user text has more than 40 nested levels", () => {
  const content = JSON.stringify({ content: deepText }), bytes = database([{ content }]);
  api.verifyLegacySQLite(SQL, bytes, account);
  const prepared = api.prepareLegacyImport(SQL, bytes);
  assert.equal(prepared.events.length, 1);
  assert.equal(prepared.events[0].row.content, content);
  assert.equal(importedRows(prepared)[0][1], content);
});

test("ordinary JSON text containing media-looking property names is preserved exactly", () => {
  const content = JSON.stringify({ content: pathText }), prepared = api.prepareLegacyImport(SQL, database([{ content }]));
  assert.equal(prepared.events[0].row.content, content);
  assert.equal(importedRows(prepared)[0][1], content);
});

test("latest_msg recognizes a native message envelope but leaves TextElem content strings opaque", () => {
  const latest = { clientMsgID: "synthetic-latest", contentType: 101, content: JSON.stringify({ content: deepText }), textElem: { content: pathText }, status: 2 };
  const prepared = api.prepareLegacyImport(SQL, database([], latest)), db = new SQL.Database(prepared.snapshot);
  try { assert.deepEqual(JSON.parse(db.exec("SELECT latest_msg FROM local_conversations")[0].values[0][0]), latest); }
  finally { db.close(); }
  assert.equal(prepared.events.length, 0);
});

test("deleted rows are erased before any JSON traversal, including deeply structured old content", () => {
  const rawStructure = Array.from({ length: 40 }).reduce(value => ({ child: value }), { filePath: "C:\\SYNTHETIC\\deleted.txt" });
  const prepared = api.prepareLegacyImport(SQL, database([{ status: 4, content: JSON.stringify(rawStructure), localEx: "C:\\SYNTHETIC\\deleted.txt", attachedInfo: "{\"isPrivateChat\":true}" }]));
  assert.equal(prepared.events.length, 1); assert.equal(prepared.events[0].kind, "delete");
  const row = importedRows(prepared)[0];
  assert.equal(row[1], ""); assert.equal(row[2], ""); assert.equal(row[3], "");
  assert.equal(JSON.stringify(prepared.events).includes("deleted.txt"), false);
});

test("private-chat text stays out of permanent archive events while its SDK text is preserved", () => {
  const content = JSON.stringify({ content: deepText }), prepared = api.prepareLegacyImport(SQL, database([{ content, attachedInfo: "{\"isPrivateChat\":true}" }]));
  assert.deepEqual(prepared.events[0].row, { client_msg_id: "synthetic-0", send_time: 1700000000000, __ephemeral: 1 });
  assert.equal(JSON.stringify(prepared.events).includes("ordinary user text"), false);
  assert.equal(importedRows(prepared)[0][1], content);
});

test("quoted native attachment paths are remapped in both serialized content and explicit file elements", () => {
  const file = "C:\\SYNTHETIC\\source.pdf", destination = "puty-local://attachment/legacy_synthetic";
  const payload = { text: pathText, quoteMessage: { clientMsgID: "quoted-file", contentType: 105,
    content: JSON.stringify({ filePath: file, fileName: "original.pdf" }), fileElem: { filePath: file, fileName: "original.pdf" }, localEx: file } };
  const prepared = api.prepareLegacyImport(SQL, database([{ type: 114, content: JSON.stringify(payload) }]), new Map([[path.win32.normalize(file).toLowerCase(), destination]]));
  const result = JSON.parse(prepared.events[0].row.content);
  assert.equal(result.text, pathText);
  assert.equal(JSON.parse(result.quoteMessage.content).filePath, destination);
  assert.equal(result.quoteMessage.fileElem.filePath, destination);
  assert.equal(result.quoteMessage.localEx, destination);
  assert.equal(result.quoteMessage.fileElem.fileName, "original.pdf");
});

test("custom string fields and an envelope-shaped unknown object without message identity stay opaque", () => {
  const value = { data: deepText, content: pathText, extension: pathText, contentType: 101, opaque: { plugin: "synthetic", values: [1, 2, 3] } };
  const content = JSON.stringify(value), prepared = api.prepareLegacyImport(SQL, database([{ type: 110, content }]));
  assert.equal(prepared.events[0].row.content, content);
  assert.deepEqual(JSON.parse(importedRows(prepared)[0][1]), value);
});

test("native merged-forward containers preserve text and remap serialized attachment payloads through nested quotes", () => {
  const source = "C:\\SYNTHETIC\\merge-source.pdf", destination = "puty-local://attachment/legacy_merge_synthetic";
  const file = { clientMsgID: "merge-file", contentType: 105, content: JSON.stringify({ filePath: source, fileName: "original.pdf" }), fileElem: { filePath: source, fileName: "original.pdf" } };
  const text = { clientMsgID: "merge-text", contentType: 101, content: JSON.stringify({ content: deepText }), textElem: { content: pathText } };
  const quote = { clientMsgID: "merge-quote", contentType: 114, content: JSON.stringify({ text: pathText, quoteMessage: file }), quoteElem: { text: deepText, quoteMessage: file } };
  const merge = { title: "synthetic forwarded messages", abstractList: [pathText], multiMessage: [file, text, quote], messageEntityList: [] };
  const message = { clientMsgID: "quoted-merge", contentType: 107, content: JSON.stringify(merge), mergeElem: merge };
  const payload = { text: pathText, quoteMessage: message };
  const prepared = api.prepareLegacyImport(SQL, database([{ type: 114, content: JSON.stringify(payload) }]), new Map([[path.win32.normalize(source).toLowerCase(), destination]]));
  const result = JSON.parse(prepared.events[0].row.content);
  assert.equal(result.text, pathText);
  for (const forwarded of [JSON.parse(result.quoteMessage.content), result.quoteMessage.mergeElem]) {
    const [fileResult, textResult, quoteResult] = forwarded.multiMessage;
    assert.equal(JSON.parse(fileResult.content).filePath, destination);
    assert.equal(fileResult.fileElem.filePath, destination);
    assert.equal(textResult.content, text.content); assert.equal(textResult.textElem.content, pathText);
    assert.equal(forwarded.abstractList[0], pathText);
    const quoted = JSON.parse(quoteResult.content);
    assert.equal(quoted.text, pathText);
    assert.equal(JSON.parse(quoted.quoteMessage.content).filePath, destination);
    assert.equal(JSON.parse(quoteResult.quoteElem.quoteMessage.content).filePath, destination);
    assert.equal(quoteResult.quoteElem.text, deepText);
  }
});

test("custom containers named multiMessage, messageList or quoteMessage are not inferred to be native envelopes", () => {
  const text = { clientMsgID: "user-data", contentType: 101, content: pathText };
  const payload = { title: "opaque custom data", abstractList: [], multiMessage: [text], messageList: [text], quoteMessage: text };
  const content = JSON.stringify(payload), prepared = api.prepareLegacyImport(SQL, database([{ type: 110, content }]));
  assert.equal(prepared.events[0].row.content, content);
});

test("deleted latest native messages retain a tombstone without parsing or preserving obsolete payloads", () => {
  const deeplyStructured = Array.from({ length: 40 }).reduce(item => ({ child: item }), { filePath: "C:\\SYNTHETIC\\deleted-latest.pdf" });
  const latest = { clientMsgID: "deleted-latest", contentType: 105, status: 4, sendTime: 1700000000000, seq: 77,
    content: JSON.stringify(deeplyStructured), fileElem: deeplyStructured, localEx: "C:\\SYNTHETIC\\deleted-latest.pdf", attachedInfo: "deleted private payload", ex: "deleted extra payload" };
  const prepared = api.prepareLegacyImport(SQL, database([], latest)), db = new SQL.Database(prepared.snapshot);
  try {
    const result = JSON.parse(db.exec("SELECT latest_msg FROM local_conversations")[0].values[0][0]);
    assert.equal(result.clientMsgID, latest.clientMsgID); assert.equal(result.status, 4);
    assert.equal(result.seq, 77); assert.equal(result.sendTime, latest.sendTime); assert.equal(result.content, "");
    assert.equal(JSON.stringify(result).includes("deleted-latest.pdf"), false);
    assert.equal(JSON.stringify(result).includes("deleted private payload"), false);
    assert.equal(JSON.stringify(result).includes("deleted extra payload"), false);
    assert.equal("fileElem" in result, false);
  } finally { db.close(); }
});

test("nested deleted quoted messages are sanitized before their old deep content and media elements are traversed", () => {
  const deep = Array.from({ length: 40 }).reduce(item => ({ child: item }), { filePath: "C:\\SYNTHETIC\\deleted-quote.pdf" });
  const quoted = { clientMsgID: "deleted-quote", contentType: 105, status: 4, content: JSON.stringify(deep), fileElem: deep, localEx: "C:\\SYNTHETIC\\deleted-quote.pdf" };
  const content = JSON.stringify({ text: pathText, quoteMessage: quoted });
  const prepared = api.prepareLegacyImport(SQL, database([{ type: 114, content }]));
  const result = JSON.parse(prepared.events[0].row.content);
  assert.equal(result.text, pathText); assert.equal(result.quoteMessage.clientMsgID, quoted.clientMsgID);
  assert.equal(result.quoteMessage.status, 4); assert.equal(result.quoteMessage.content, "");
  assert.equal("fileElem" in result.quoteMessage, false);
  assert.equal(JSON.stringify(result).includes("deleted-quote.pdf"), false);
});

test("actual native JSON structure still has a finite depth limit and is not silently omitted", () => {
  const value = Array.from({ length: 40 }).reduce(item => ({ child: item }), { filePath: "C:\\SYNTHETIC\\actual-native-data.pdf" });
  assert.throws(() => api.prepareLegacyImport(SQL, database([{ type: 105, content: JSON.stringify(value) }])), /Legacy message JSON exceeds depth bound/);
});

test("SQLite account and integrity checks still reject wrong ownership and damaged SQLite", () => {
  const bytes = database([{ text: deepText }]);
  assert.throws(() => api.verifyLegacySQLite(SQL, bytes, other), /identity differs/);
  const damaged = bytes.slice(); damaged.fill(0, 0, 16);
  assert.throws(() => api.verifyLegacySQLite(SQL, damaged, account));
  assert.throws(() => api.prepareLegacyImport(SQL, damaged));
});

test("a pre-fix preparation failure can resume its exact sealed account and ready attachment without renaming", async t => {
  const directory = await fs.realpath(await fs.mkdtemp(path.join(tmpdir(), "puty-legacy-json-")));
  const source = path.join(directory, "old"), root = path.join(directory, "encrypted"), osKey = randomBytes(32), accountKey = randomBytes(32);
  const vaults = new Set();
  t.after(async () => {
    for (const vault of vaults) vault.lock();
    osKey.fill(0); accountKey.fill(0);
    assert.equal(path.dirname(directory), await fs.realpath(tmpdir()));
    assert.ok(path.basename(directory).startsWith("puty-legacy-json-"));
    assert.equal(await fs.realpath(directory), directory);
    await fs.rm(directory, { recursive: true });
  });
  await fs.mkdir(source); await fs.mkdir(root);
  const mediaDir = path.join(source, "OpenIMData", account, "sentFileCache"); await fs.mkdir(mediaDir, { recursive: true });
  const media = path.join(mediaDir, "old-cache.bin"), mediaBytes = Buffer.from("SYNTHETIC-LEGACY-JSON-ATTACHMENT");
  await fs.writeFile(media, mediaBytes);
  const bytes = database([{ text: deepText }, { id: "synthetic-file", type: 105, content: JSON.stringify({ filePath: media, fileName: "员工原文件.txt" }), localEx: media }]);
  const protection = { available: async () => true,
    wrap: async value => crypto.seal(osKey, { account: "synthetic", type: "os-wrap", objectID: "master" }, Buffer.from(value)),
    unwrap: async value => crypto.unseal(osKey, { account: "synthetic", type: "os-wrap", objectID: "master" }, value).toString() };
  const readout = { source, sourceAliases: [source], databases: [{ accountID: account, databaseName: `-sql-${account}.sqlite`, bytes, sha256: sha(bytes), pageSize: 8192, pageCount: bytes.length / 8192, ignoredTrailingPages: 0, requiresIntegrityCheck: true }], rawDatabases: [], unresolved: [], cacheFiles: [], deletionAuthorized: false };
  const receipt = await api.preserveLegacyData({ source, root, readout, writersStopped: true, SQL, protection, assertCurrent: async () => {} });
  const open = async () => { const value = await LocalVault.open(path.join(root, "accounts", account, "vault"), account, accountKey, async () => {}); vaults.add(value); return value; };
  let vault = await open();
  const options = () => ({ root, snapshotID: receipt.snapshotID, accountID: account, vault, SQL, protection, assertCurrent: async () => {} });
  // The acceptance run explicitly supplies the captured old production source.
  // A clean checkout uses a fault at the same real SQL preparation boundary.
  let opens = 0;
  const failAtPreparation = { Database: class { constructor(input) { if (++opens === 3) throw Error("Legacy message JSON exceeds depth bound"); return new SQL.Database(input); } } };
  await assert.rejects((beforeAPI || api).importLegacyAccount({ ...options(), ...(!beforeAPI ? { SQL: failAtPreparation } : {}) }), error => {
    assert.equal(error.legacyImportStep, "PREPARE_MESSAGES");
    assert.match(error.message, /Legacy message JSON exceeds depth bound/); return true;
  });
  assert.equal((await vault.preferences())["legacyImport:" + receipt.snapshotID].state, "preparing");
  const attachmentKeys = Object.keys(vault.index.attachments); assert.equal(attachmentKeys.length, 1);
  const taskID = attachmentKeys[0], prior = await vault.attachment(taskID);
  assert.equal(prior.state, "ready"); assert.equal(prior.name, "员工原文件.txt");
  const filename = path.join(vault.root, prior.file), statBefore = await fs.stat(filename), cipherBefore = await fs.readFile(filename);
  vault.lock(); vault = await open();
  const result = await api.importLegacyAccount(options());
  assert.equal(result.imported, true); assert.equal(result.messages, 2); assert.equal(result.attachments, 1);
  const after = await vault.attachment(taskID);
  assert.equal(after.name, prior.name); assert.equal(after.file, prior.file); assert.equal(after.objectID, prior.objectID);
  assert.deepEqual(await fs.readFile(filename), cipherBefore);
  const statAfter = await fs.stat(filename); assert.equal(statAfter.dev, statBefore.dev); assert.equal(statAfter.ino, statBefore.ino);
  assert.deepEqual(await fs.readdir(path.dirname(filename)), ["员工原文件.txt"]);
  assert.deepEqual(await vault.readAttachment(taskID), mediaBytes);
  assert.equal((await vault.preferences())["legacyImport:" + receipt.snapshotID].state, "complete");
  assert.equal((await vault.stats()).messages, 2);
  assert.equal((await api.importLegacyAccount(options())).alreadyImported, true);
  assert.equal((await vault.stats()).attachments, 1);
  assert.equal(await fs.readFile(media, "utf8"), mediaBytes.toString());
});
