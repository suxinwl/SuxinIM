import { createHash, randomBytes, randomUUID } from "node:crypto";
import { promises as fs } from "node:fs";
import path from "node:path";
import { createRequire } from "node:module";
import { serialize, deserialize } from "node:v8";
import { CHUNK_SIZE, createHeader, decryptFrame, encryptFrame, objectKey, parseHeader, seal, unseal } from "./crypto";
import { atomicWrite, syncDirectory, type ArchiveEvent, type LocalVault } from "./vault";
import { type LegacyReadout } from "./legacy";
import { LEGACY_RESOLUTION_POLICY, legacyQuarantineFailure, resolveLegacyQuarantine } from "./legacyQuarantine";
import { legacyImportFailure, type LegacyImportStep } from './legacyFailure';

const FORMAT = "PUTY-LEGACY-1";
const accountPattern = /^emp_[a-f0-9]{32}$/;
const snapshotPattern = /^[a-f0-9-]{36}$/;
const digest = (bytes: Uint8Array | string) => createHash("sha256").update(bytes).digest("hex");
const context = (snapshotID: string, type: string, objectID: string) => ({ account: "legacy-" + snapshotID, type, objectID });
const maxFileCount = 200000;
const maxSQLiteBytes = 256 * 1024 * 1024;

export type LegacyProtection = {
  available(): Promise<boolean>;
  wrap(value: string): Promise<Uint8Array>;
  unwrap(value: Uint8Array): Promise<string>;
};
export type LegacySQLDatabase = {
  exec(sql: string): { columns: string[]; values: unknown[][] }[];
  run(sql: string, bindings?: unknown[]): unknown;
  export(): Uint8Array;
  close(): void;
};
export type LegacySQL = { Database: new (bytes?: Uint8Array) => LegacySQLDatabase };
type SourceFile = { relativePath: string; size: number; mtimeMs: number; sha256: string };
type ObjectRecord = { objectID: string; file: string; size: number; sha256: string; role: "source" | "sqlite" | "raw-idb"; relativePath?: string; mtimeMs?: number; accountID?: string; databaseName?: string };
type SQLiteRecord = { accountID: string; databaseName: string; objectID: string; sha256: string; integrity: "ok" };
type Manifest = { format: typeof FORMAT; snapshotID: string; parentSnapshotID?: string; source: string; sourceAliases: string[]; createdAt: number; directories: string[]; objects: ObjectRecord[]; databases: SQLiteRecord[]; unresolved: { databaseName: string; reason: string }[]; verified: true };
export type LegacyReceipt = { format: typeof FORMAT; snapshotID: string; parentSnapshotID?: string; files: number; bytes: number; databases: number; quarantined: number; verified: true; plaintextCleanupComplete: boolean };
type Options = { root: string; source: string; writersStopped: true; protection?: LegacyProtection; SQL?: LegacySQL; assertCurrent?: () => Promise<void>; nativeSingletonLockHeld?: () => boolean };

async function defaultProtection(): Promise<LegacyProtection> {
  const { safeStorage } = await import("electron");
  return { available: () => safeStorage.isAsyncEncryptionAvailable(), wrap: value => safeStorage.encryptStringAsync(value), unwrap: async bytes => (await safeStorage.decryptStringAsync(Buffer.from(bytes))).result };
}
export async function loadLegacySQL(): Promise<LegacySQL> {
  const { app } = await import("electron");
  const require = createRequire(path.join(app.getAppPath(), "package.json"));
  if (require("@jlongster/sql.js/package.json").version !== "1.6.7") throw new Error("Legacy importer requires fixed SQL.js 1.6.7");
  // No SDK initialization, SQLiteFS, IndexedDB backend or disk database is used.
  const paths = (globalThis as typeof globalThis & { pathConfig?: { distPath: string } }).pathConfig;
  if (!paths || !path.isAbsolute(paths.distPath)) throw new Error("Legacy SQL.js resource path is unavailable");
  const wasmBinary = await fs.readFile(path.join(paths.distPath, "sql-wasm.wasm"));
  return require("@jlongster/sql.js")({ wasmBinary });
}
const rows = (db: LegacySQLDatabase, sql: string): Record<string, any>[] => {
  const result = db.exec(sql)[0];
  return result ? result.values.map(values => Object.fromEntries(result.columns.map((key, index) => [key, values[index]]))) : [];
};
const identifier = (value: string) => '"' + value.replace(/"/g, '""') + '"';
export function verifyLegacySQLite(SQL: LegacySQL, bytes: Uint8Array, accountID?: string, requireLoginIdentity = false): void {
  if (bytes.length > maxSQLiteBytes) throw new Error("Legacy SQLite exceeds RAM import bound");
  const db = new SQL.Database(bytes);
  try {
    const checks = db.exec("PRAGMA integrity_check");
    if (checks.length !== 1 || checks[0].values.length !== 1 || checks[0].values[0].length !== 1 || checks[0].values[0][0] !== "ok") throw new Error("Legacy SQLite integrity check failed; encrypted evidence retained");
    if (accountID !== undefined) {
      if (!accountPattern.test(accountID)) throw new Error("Legacy declared account is invalid");
      const hasUsers = rows(db, "SELECT name FROM sqlite_master WHERE type='table' AND name='local_users'").length !== 0;
      const hasUserID = hasUsers && rows(db, "PRAGMA table_info(local_users)").some(row => row.name === "user_id");
      const identities = hasUserID ? [...new Set(rows(db, "SELECT user_id FROM local_users").map(row => String(row.user_id || "")).filter(Boolean))] : [];
      if (identities.length > 1 || identities.length === 1 && identities[0] !== accountID) throw new Error("Legacy SQLite login identity differs from its declared account");
      if (!identities.length && requireLoginIdentity) throw new Error("Legacy history has no verifiable login identity; encrypted quarantine retained");
      if (!identities.length) for (const table of rows(db, "SELECT name FROM sqlite_master WHERE type='table' AND name GLOB 'chat_logs_*'")) {
        if (rows(db, `SELECT 1 FROM ${identifier(String(table.name))} LIMIT 1`).length) throw new Error("Legacy history has no verifiable login identity; encrypted quarantine retained");
      }
    }
  } finally { db.close(); }
}

function relative(value: string): string {
  if (!value || value.includes("\\") || value.startsWith("/") || value.split("/").some(part => !part || part === "." || part === "..") || /[\x00-\x1f]/.test(value)) throw new Error("Legacy relative path is unsafe");
  return value;
}
async function canonicalDirectory(value: string, create = false): Promise<string> {
  if (!path.isAbsolute(value) || path.resolve(value) !== value) throw new Error("Legacy path must be canonical and absolute");
  const parts = path.relative(path.parse(value).root, value).split(path.sep).filter(Boolean); let cursor = path.parse(value).root;
  for (const part of parts) {
    cursor = path.join(cursor, part);
    if (create) await fs.mkdir(cursor).catch(error => { if (error.code !== "EEXIST") throw error; });
    const info = await fs.lstat(cursor); if (!info.isDirectory() || info.isSymbolicLink()) throw new Error("Legacy directory is linked or not a directory");
  }
  return fs.realpath(value);
}
function samePath(left: string, right: string): boolean { return process.platform === "win32" ? left.toLowerCase() === right.toLowerCase() : left === right; }
function nested(child: string, parent: string): boolean { return samePath(child, parent) || (process.platform === "win32" ? child.toLowerCase().startsWith(parent.toLowerCase() + path.sep) : child.startsWith(parent + path.sep)); }
async function cleanupSource(value: string): Promise<string> {
  if (!path.isAbsolute(value) || path.resolve(value) !== value) throw new Error("Legacy path must be canonical and absolute");
  const parent = await canonicalDirectory(path.dirname(value)), target = path.join(parent, path.basename(value));
  const info = await fs.lstat(target).catch(error => { if (error.code === "ENOENT") return null; throw error; });
  if (!info) return target;
  if (!info.isDirectory() || info.isSymbolicLink()) throw new Error("Legacy source is linked or not a directory");
  return canonicalDirectory(target);
}
async function scope(options: Options, sourceMayBeMissing = false) {
  if (options.writersStopped !== true) throw new Error("Legacy writers must be stopped before migration");
  const source = sourceMayBeMissing ? await cleanupSource(options.source) : await canonicalDirectory(options.source), root = await canonicalDirectory(options.root);
  if (nested(root, source) || nested(source, root)) throw new Error("Legacy source and encrypted destination overlap");
  const base = await canonicalDirectory(path.join(root, "legacy"), true);
  return { source, root, base };
}
async function safeFile(base: string, name: string): Promise<string> {
  const value = relative(name), target = path.join(base, ...value.split("/"));
  await canonicalDirectory(path.dirname(target), true);
  const info = await fs.lstat(target).catch(error => { if (error.code === "ENOENT") return null; throw error; });
  if (info && (!info.isFile() || info.isSymbolicLink() || info.nlink !== 1)) throw new Error("Legacy file is linked or not regular");
  return target;
}
async function privateKey(base: string, protection?: LegacyProtection): Promise<Buffer> {
  const provider = protection || await defaultProtection();
  if (!await provider.available()) throw new Error("System key protection is unavailable; legacy plaintext was retained");
  const target = await safeFile(base, "master.oswrap");
  try {
    const value = await provider.unwrap(await fs.readFile(target));
    if (!/^[A-Za-z0-9+/]{43}=$/.test(value)) throw new Error("Invalid legacy master key");
    const key = Buffer.from(value, "base64"); if (key.length !== 32) throw new Error("Invalid legacy master key"); return key;
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code !== "ENOENT") throw new Error("Legacy backup key could not be unlocked; no plaintext was removed", { cause: error });
  }
  // Refuse replacing a lost key while any old snapshot may depend on it.
  if ((await fs.readdir(base)).length) throw new Error("Legacy backup exists without its OS-wrapped key");
  const key = randomBytes(32);
  try {
    const wrapped = await provider.wrap(key.toString("base64")), handle = await fs.open(target, "wx", 0o600);
    try { await handle.writeFile(wrapped); await handle.sync(); } finally { await handle.close(); }
    await syncDirectory(base);
    const verified = await provider.unwrap(await fs.readFile(target));
    if (!Buffer.from(verified, "base64").equals(key)) throw new Error("Legacy wrapped key readback failed");
    return key;
  } catch (error) { key.fill(0); throw error; }
}
async function inventory(source: string): Promise<{ files: Omit<SourceFile, "sha256">[]; directories: string[] }> {
  const files: Omit<SourceFile, "sha256">[] = [], directories: string[] = [];
  const walk = async (directory: string): Promise<void> => {
    for (const name of (await fs.readdir(directory)).sort()) {
      const target = path.join(directory, name), rel = path.relative(source, target).split(path.sep).join("/"); relative(rel);
      const info = await fs.lstat(target);
      if (info.isSymbolicLink()) throw new Error("Legacy tree contains a link; plaintext was retained");
      if (info.isDirectory()) { directories.push(rel); await walk(target); }
      else if (info.isFile() && info.nlink === 1) { if (files.length >= maxFileCount || !Number.isSafeInteger(info.size)) throw new Error("Legacy inventory exceeds bound"); files.push({ relativePath: rel, size: info.size, mtimeMs: info.mtimeMs }); }
      else throw new Error("Legacy tree contains a special or hard-linked file");
    }
  };
  await walk(source); return { files, directories };
}
async function * fileBytes(target: string, expected?: { size: number; mtimeMs: number }): AsyncGenerator<Buffer> {
  const before = await fs.lstat(target); if (!before.isFile() || before.isSymbolicLink() || before.nlink !== 1 || expected && (before.size !== expected.size || before.mtimeMs !== expected.mtimeMs)) throw new Error("Legacy source changed during backup");
  const handle = await fs.open(target, "r");
  try {
    const opened = await handle.stat(); if (opened.dev !== before.dev || opened.ino !== before.ino || opened.size !== before.size || opened.mtimeMs !== before.mtimeMs) throw new Error("Legacy source changed while opening");
    let offset = 0;
    while (offset < before.size) { const bytes = Buffer.alloc(Math.min(CHUNK_SIZE, before.size - offset)); const read = await handle.read(bytes, 0, bytes.length, offset); if (!read.bytesRead) throw new Error("Legacy source truncated"); offset += read.bytesRead; yield bytes.subarray(0, read.bytesRead); }
    const after = await handle.stat(), current = await fs.lstat(target);
    if (current.isSymbolicLink() || after.size !== before.size || after.mtimeMs !== before.mtimeMs || current.dev !== before.dev || current.ino !== before.ino || current.size !== before.size || current.mtimeMs !== before.mtimeMs) throw new Error("Legacy writer is still active; plaintext was retained");
  } finally { await handle.close(); }
}
async function encryptObject(base: string, master: Buffer, snapshotID: string, objectID: string, bytes: AsyncIterable<Uint8Array>, assertCurrent: () => Promise<void>): Promise<{ file: string; size: number; sha256: string }> {
  const file = "objects/" + objectID, target = await safeFile(base, file), handle = await fs.open(target, "wx", 0o600);
  const { header, bytes: headerBytes, prefix } = createHeader(context(snapshotID, "legacy-object", objectID)), key = objectKey(master, header), sha = createHash("sha256");
  let size = 0, index = 0;
  try {
    await handle.writeFile(prefix);
    for await (const input of bytes) {
      await assertCurrent();
      for (let offset = 0; offset < input.length; offset += CHUNK_SIZE) { const plain = input.subarray(offset, offset + CHUNK_SIZE); sha.update(plain); size += plain.length; await handle.writeFile(encryptFrame(key, header, headerBytes, index++, plain)); }
    }
    await handle.writeFile(encryptFrame(key, header, headerBytes, index, Buffer.alloc(0))); await handle.sync();
  } finally { key.fill(0); await handle.close(); }
  await syncDirectory(path.dirname(target)); return { file, size, sha256: sha.digest("hex") };
}
async function * decryptObject(base: string, master: Buffer, snapshotID: string, record: ObjectRecord, assertCurrent: () => Promise<void>): AsyncGenerator<Buffer> {
  if (!/^[a-f0-9]{64}$/.test(record.objectID) || record.file !== "objects/" + record.objectID) throw new Error("Legacy object descriptor is invalid");
  const handle = await fs.open(await safeFile(base, record.file), "r"); let key: Buffer | undefined;
  try {
    const readExact = async (length: number, position: number) => { const bytes = Buffer.alloc(length), result = await handle.read(bytes, 0, length, position); if (result.bytesRead !== length) throw new Error("Legacy encrypted backup is truncated"); return bytes; };
    const first = await readExact(12, 0), length = first.readUInt32BE(8); if (length < 2 || length > 65536) throw new Error("Invalid legacy encrypted header");
    const parsed = parseHeader(Buffer.concat([first, await readExact(length, 12)])), expected = context(snapshotID, "legacy-object", record.objectID), header = parsed.header;
    if (header.account !== expected.account || header.type !== expected.type || header.objectID !== expected.objectID) throw new Error("Legacy encrypted object context mismatch");
    key = objectKey(master, header); let position = parsed.offset, index = 0, size = 0; const sha = createHash("sha256");
    while (true) {
      await assertCurrent(); const count = (await readExact(4, position)).readUInt32BE(0); position += 4;
      if (count > CHUNK_SIZE) throw new Error("Legacy encrypted frame exceeds bound");
      const frame = await readExact(count + 16, position); position += count + 16;
      const plain = decryptFrame(key, header, parsed.bytes, index++, frame.subarray(0, count), frame.subarray(count));
      if (!count) { if ((await handle.stat()).size !== position || size !== record.size || sha.digest("hex") !== record.sha256) throw new Error("Legacy backup readback hash mismatch"); break; }
      size += plain.length; if (size > record.size) throw new Error("Legacy backup size mismatch"); sha.update(plain); yield plain;
    }
  } finally { key?.fill(0); await handle.close(); }
}
async function memoryObject(base: string, key: Buffer, id: string, record: ObjectRecord, check: () => Promise<void>): Promise<Buffer> {
  if (record.size > (record.role === "raw-idb" ? 1024 * 1024 * 1024 + 16 * 1024 * 1024 : maxSQLiteBytes)) throw new Error("Legacy in-memory object exceeds bound");
  const parts: Buffer[] = [];
  try { for await (const bytes of decryptObject(base, key, id, record, check)) parts.push(bytes); return Buffer.concat(parts); }
  finally { for (const bytes of parts) bytes.fill(0); }
}
async function assertInventory(source: string, files: SourceFile[], directories: string[], allowMissing = false, retainedNativeLock = false): Promise<void> {
  const current = await inventory(source).catch(async error => {
    if (allowMissing && error.code === "ENOENT" && await fs.lstat(source).then(() => false, missing => { if (missing.code === "ENOENT") return true; throw missing; })) return { files: [], directories: [] };
    throw error;
  });
  if (retainedNativeLock) current.files = current.files.filter(row => row.relativePath !== "lockfile");
  const expected = new Map(files.map(row => [row.relativePath, row]));
  if (!allowMissing && (current.files.length !== files.length || JSON.stringify(current.directories) !== JSON.stringify(directories))) throw new Error("Legacy source inventory changed; plaintext was retained");
  if (current.directories.some(name => !directories.includes(name))) throw new Error("Legacy source acquired new directories");
  for (const row of current.files) {
    const old = expected.get(row.relativePath); if (!old || old.size !== row.size || old.mtimeMs !== row.mtimeMs) throw new Error("Legacy source inventory changed; plaintext was retained");
    const sha = createHash("sha256"); for await (const part of fileBytes(path.join(source, ...row.relativePath.split("/")), old)) sha.update(part);
    if (sha.digest("hex") !== old.sha256) throw new Error("Legacy source hash changed; plaintext was retained");
  }
}
function receipt(manifest: Manifest, complete = false): LegacyReceipt { return { format: FORMAT, snapshotID: manifest.snapshotID, parentSnapshotID: manifest.parentSnapshotID, files: manifest.objects.filter(row => row.role === "source").length, bytes: manifest.objects.filter(row => row.role === "source").reduce((sum, row) => sum + row.size, 0), databases: manifest.databases.length, quarantined: manifest.unresolved.length, verified: true, plaintextCleanupComplete: complete }; }

/** Full encrypted preservation, independent of whichever account is logged in.
 * Nothing in the legacy directory is modified by this operation. */
export async function preserveLegacyData(options: Options & { readout: LegacyReadout }): Promise<LegacyReceipt> { return preserve(options); }
async function preserve(options: Options & { readout: LegacyReadout }, parent?: Manifest): Promise<LegacyReceipt> {
  const { source, base } = await scope(options), check = options.assertCurrent || (async () => {});
  if (!samePath(await canonicalDirectory(options.readout.source), source)) throw new Error("Legacy readout belongs to another source");
  const SQL = options.SQL || await loadLegacySQL(); await check();
  const key = await privateKey(base, options.protection), snapshotID = randomUUID(), temporary = path.join(base, ".partial-" + snapshotID), final = path.join(base, snapshotID);
  try {
    await fs.mkdir(temporary); const initial = await inventory(source), objects: ObjectRecord[] = [], databases: SQLiteRecord[] = [], unresolved = [...options.readout.unresolved];
    for (const row of initial.files) {
      const objectID = digest("source\0" + row.relativePath), saved = await encryptObject(temporary, key, snapshotID, objectID, fileBytes(path.join(source, ...row.relativePath.split("/")), row), check);
      if (saved.size !== row.size) throw new Error("Legacy source size changed"); objects.push({ ...saved, objectID, role: "source", relativePath: row.relativePath, mtimeMs: row.mtimeMs });
    }
    const rawID = digest("raw-idb"), rawBytes = serialize(options.readout.rawDatabases);
    objects.push({ ...await encryptObject(temporary, key, snapshotID, rawID, [rawBytes] as any, check), objectID: rawID, role: "raw-idb" });
    const seen = new Set<string>();
    for (const item of options.readout.databases) {
      if (!accountPattern.test(item.accountID) || seen.has(item.accountID) || digest(item.bytes) !== item.sha256) throw new Error("Legacy reconstructed account identity is ambiguous"); seen.add(item.accountID);
      try { verifyLegacySQLite(SQL, item.bytes, item.accountID); }
      catch { unresolved.push({ databaseName: item.databaseName, reason: "RAM SQLite integrity or login identity failed; full encrypted source retained" }); continue; }
      const objectID = digest("sqlite\0" + item.accountID), saved = await encryptObject(temporary, key, snapshotID, objectID, [item.bytes] as any, check);
      objects.push({ ...saved, objectID, role: "sqlite", accountID: item.accountID, databaseName: item.databaseName }); databases.push({ accountID: item.accountID, databaseName: item.databaseName, objectID, sha256: item.sha256, integrity: "ok" });
    }
    // Windows may have stored the same old root through an 8.3 spelling. Only
    // aliases whose realpath was verified above are retained, never basenames.
    const aliases = [...new Set([source, options.source, options.readout.source, ...(options.readout.sourceAliases || []), ...(parent?.sourceAliases || [])])];
    for (const alias of aliases) if (!samePath(await canonicalDirectory(alias), source)) throw new Error("Legacy source alias escaped its original root");
    const manifest: Manifest = { format: FORMAT, snapshotID, parentSnapshotID: parent?.snapshotID, source, sourceAliases: aliases, createdAt: Date.now(), directories: initial.directories, objects, databases, unresolved, verified: true };
    for (const item of objects) { for await (const _chunk of decryptObject(temporary, key, snapshotID, item, check)) { /* authenticated streaming readback */ } }
    const sources = objects.filter(row => row.role === "source").map(row => ({ relativePath: row.relativePath!, size: row.size, mtimeMs: row.mtimeMs!, sha256: row.sha256 }));
    await assertInventory(source, sources, initial.directories); await check();
    await atomicWrite(path.join(temporary, "manifest.puty"), seal(key, context(snapshotID, "legacy-manifest", "manifest"), Buffer.from(JSON.stringify(manifest))));
    const verified = JSON.parse(unseal(key, context(snapshotID, "legacy-manifest", "manifest"), await fs.readFile(path.join(temporary, "manifest.puty"))).toString("utf8"));
    if (JSON.stringify(verified) !== JSON.stringify(manifest)) throw new Error("Legacy manifest readback mismatch");
    await fs.rename(temporary, final); await syncDirectory(base); return receipt(manifest);
  } finally { key.fill(0); }
}

/** After the extractor process has exited, Chromium may have flushed its final
 * LevelDB/Quota metadata. Preserve that closed physical tree in a new immutable
 * snapshot; retain the original extracted SQLite and its complete parent backup.
 * This never bypasses source equality in cleanup and never reopens IndexedDB.
 * The caller must still hold the application's legacy-writer exclusion. */
export async function finalizeLegacySnapshot(options: Options & { snapshotID: string; readerProcessExited: true }): Promise<LegacyReceipt> {
  if (options.readerProcessExited !== true) throw new Error("Legacy extractor must exit before finalizing physical backup");
  const check = options.assertCurrent || (async () => {}); await check();
  const { source } = await scope(options), opened = await openSnapshot(options.root, options.snapshotID, options.protection), { manifest, directory, key } = opened;
  const plaintext: Buffer[] = [];
  try {
    if (!samePath(manifest.source, source) || manifest.parentSnapshotID) throw new Error("Legacy physical finalization requires its original extraction snapshot");
    const progress = await safeFile(directory, "cleanup.puty");
    if (await fs.access(progress).then(() => true, error => { if (error.code === "ENOENT") return false; throw error; })) throw new Error("Legacy snapshot cleanup already started; do not recapture a partial source");
    const children = (await listLegacySnapshots(options.root, options.protection)).filter(row => row.parentSnapshotID === manifest.snapshotID);
    if (children.length > 1) throw new Error("Legacy snapshot has ambiguous finalized children");
    if (children.length === 1) return children[0];
    const raw = manifest.objects.find(row => row.role === "raw-idb"); if (!raw) throw new Error("Legacy extraction metadata is absent");
    const rawBytes = await memoryObject(directory, key, manifest.snapshotID, raw, check); plaintext.push(rawBytes);
    const rawDatabases = deserialize(rawBytes); if (!Array.isArray(rawDatabases) || rawDatabases.length > 128) throw new Error("Legacy extraction metadata is invalid");
    const databases: LegacyReadout["databases"] = [];
    for (const item of manifest.databases) {
      const object = manifest.objects.find(row => row.role === "sqlite" && row.objectID === item.objectID); if (!object) throw new Error("Legacy extracted SQLite is missing");
      const bytes = await memoryObject(directory, key, manifest.snapshotID, object, check); plaintext.push(bytes);
      const word = (bytes[16] << 8) | bytes[17], pageSize = word === 1 ? 65536 : word;
      databases.push({ accountID: item.accountID, databaseName: item.databaseName, bytes, sha256: item.sha256, pageSize, pageCount: bytes.length / pageSize, ignoredTrailingPages: 0, requiresIntegrityCheck: true });
    }
    return await preserve({ ...options, readout: { source, databases, rawDatabases, cacheFiles: [], unresolved: manifest.unresolved, deletionAuthorized: false } }, manifest);
  } finally { for (const bytes of plaintext) bytes.fill(0); key.fill(0); }
}

async function openSnapshot(root: string, snapshotID: string, protection?: LegacyProtection) {
  if (!snapshotPattern.test(snapshotID)) throw new Error("Invalid legacy snapshot identifier");
  const base = await canonicalDirectory(path.join(await canonicalDirectory(root), "legacy")), directory = await canonicalDirectory(path.join(base, snapshotID)), key = await privateKey(base, protection);
  try {
    const bytes = await fs.readFile(await safeFile(directory, "manifest.puty")), manifest = JSON.parse(unseal(key, context(snapshotID, "legacy-manifest", "manifest"), bytes).toString("utf8")) as Manifest;
    if (manifest.format !== FORMAT || manifest.snapshotID !== snapshotID || manifest.verified !== true || !Array.isArray(manifest.objects) || manifest.objects.length > maxFileCount + 129) throw new Error("Legacy snapshot manifest is invalid");
    return { directory, key, manifest, manifestSHA256: digest(bytes) };
  } catch (error) { key.fill(0); throw error; }
}

type ResolutionBinding = { policy: typeof LEGACY_RESOLUTION_POLICY; snapshotID: string; manifestSHA256: string; accountID: string; rawObjects: { objectID: string; size: number; sha256: string }[] };
type ResolutionProof = { binding: ResolutionBinding; databaseNames: string[]; record: SQLiteRecord; object: ObjectRecord };
type ResolvedAccount = { record: SQLiteRecord; object: ObjectRecord; directory: string; bytes: Buffer };

/** Account-scoped derived evidence. The sealed extraction and its manifest are
 * immutable; this never reopens the old profile or selects a new global leaf. */
async function resolveSealedQuarantine(options: { directory: string; manifest: Manifest; manifestSHA256: string; key: Buffer; accountID: string; SQL: LegacySQL; assertCurrent: () => Promise<void>; completedSHA?: unknown }): Promise<ResolvedAccount> {
  const { directory, manifest, key, accountID, SQL, assertCurrent: check } = options;
  const originalIDs = new Set<string>();
  for (const object of manifest.objects) {
    if (!object || !/^[a-f0-9]{64}$/.test(object.objectID) || originalIDs.has(object.objectID) || object.file !== "objects/" + object.objectID || !Number.isSafeInteger(object.size) || object.size < 0 || !/^[a-f0-9]{64}$/.test(object.sha256) || !["source", "sqlite", "raw-idb"].includes(object.role)) throw legacyQuarantineFailure("QUARANTINE_CAPSULE");
    originalIDs.add(object.objectID);
  }
  const rawObjects = manifest.objects.filter(row => row.role === "raw-idb");
  if (rawObjects.length !== 1) throw legacyQuarantineFailure("QUARANTINE_SCHEMA");
  const binding: ResolutionBinding = { policy: LEGACY_RESOLUTION_POLICY, snapshotID: manifest.snapshotID, manifestSHA256: options.manifestSHA256, accountID, rawObjects: rawObjects.map(({ objectID, size, sha256 }) => ({ objectID, size, sha256 })).sort((a, b) => a.objectID.localeCompare(b.objectID)) };
  const bindingText = JSON.stringify(binding), bindingID = digest(bindingText);
  const resolvedBase = await canonicalDirectory(path.join(directory, "resolved"), true), final = path.join(resolvedBase, accountID);
  const assertParent = async () => {
    await check();
    if (digest(await fs.readFile(await safeFile(directory, "manifest.puty"))) !== options.manifestSHA256) throw legacyQuarantineFailure("QUARANTINE_CAPSULE");
  };
  const readCapsule = async (where: string): Promise<ResolvedAccount> => {
    let bytes: Buffer | undefined;
    try {
      await check(); await canonicalDirectory(where);
      const proofFile = await safeFile(where, "proof.puty"), stat = await fs.lstat(proofFile);
      if (stat.size > 1024 * 1024) throw new Error("Oversized resolution proof");
      const proof = JSON.parse(unseal(key, context(manifest.snapshotID, "legacy-resolution", bindingID), await fs.readFile(proofFile)).toString("utf8")) as ResolutionProof;
      if (JSON.stringify(proof.binding) !== bindingText || !Array.isArray(proof.databaseNames) || !proof.databaseNames.length || proof.databaseNames.length > 128 || new Set(proof.databaseNames).size !== proof.databaseNames.length || proof.databaseNames.some(name => typeof name !== "string" || name.length > 1024 || !name.endsWith(accountID + ".sqlite"))) throw new Error("Invalid resolution binding");
      const { record, object } = proof;
      if (!record || record.accountID !== accountID || record.integrity !== "ok" || record.databaseName !== proof.databaseNames[0] || record.objectID !== digest("resolved-sqlite\0" + bindingID + "\0" + record.sha256) || !/^[a-f0-9]{64}$/.test(record.sha256) || !object || object.objectID !== record.objectID || object.file !== "objects/" + record.objectID || object.sha256 !== record.sha256 || object.role !== "sqlite" || object.accountID !== accountID || object.databaseName !== record.databaseName || !Number.isSafeInteger(object.size) || object.size < 512 || object.size > maxSQLiteBytes) throw new Error("Invalid resolution database descriptor");
      bytes = await memoryObject(where, key, manifest.snapshotID, object, check);
      verifyLegacySQLite(SQL, bytes, accountID, true);
      await assertParent();
      const result = { record, object, directory: where, bytes }; bytes = undefined; return result;
    } catch (error) {
      // System/authorization errors retain their established classifications.
      if ((error as NodeJS.ErrnoException).code && ["EACCES", "EPERM", "EBUSY", "EIO", "ENOSPC"].includes((error as NodeJS.ErrnoException).code!)) throw error;
      if (error instanceof Error && ["账号会话已改变", "账号安全版本已改变，请重新登录", "账号、网络或终端权限已失效", "本地档案已锁定，请在线重新登录", "本地档案已锁定"].includes(error.message)) throw error;
      throw legacyQuarantineFailure("QUARANTINE_CAPSULE", error);
    } finally { bytes?.fill(0); }
  };
  let existing: ResolvedAccount | undefined, rawBytes: Buffer | undefined, parsedRaw: unknown, selected: Uint8Array | undefined;
  try {
    const finalInfo = await fs.lstat(final).catch(error => { if (error.code === "ENOENT") return undefined; throw error; });
    if (finalInfo) existing = await readCapsule(final);
    // Like ordinary completed imports, this fast path does not reread every old
    // payload. Parent and capsule are authenticated on every use. Before a
    // durable completion marker, all original objects must still authenticate.
    if (existing && options.completedSHA === existing.record.sha256) { const result = existing; existing = undefined; return result; }
    for (const object of manifest.objects) {
      if (!existing && object.role === "raw-idb") rawBytes = await memoryObject(directory, key, manifest.snapshotID, object, check);
      else for await (const chunk of decryptObject(directory, key, manifest.snapshotID, object, check)) chunk.fill(0);
    }
    await assertParent();
    if (existing) { const result = existing; existing = undefined; return result; }
    try { parsedRaw = deserialize(rawBytes!); } catch (error) { throw legacyQuarantineFailure("QUARANTINE_SCHEMA", error); }
    const resolution = resolveLegacyQuarantine(parsedRaw, accountID, (bytes, id) => verifyLegacySQLite(SQL, bytes, id, true)); selected = resolution.bytes;
    const temporary = path.join(resolvedBase, accountID + ".pending-" + randomUUID());
    await fs.mkdir(temporary, { mode: 0o700 }); await canonicalDirectory(temporary); await check();
    const objectID = digest("resolved-sqlite\0" + bindingID + "\0" + resolution.sha256);
    const encrypted = await encryptObject(temporary, key, manifest.snapshotID, objectID, (async function* () { yield resolution.bytes; })(), check);
    const record: SQLiteRecord = { accountID, databaseName: resolution.databaseNames[0], objectID, sha256: resolution.sha256, integrity: "ok" };
    const object: ObjectRecord = { objectID, ...encrypted, role: "sqlite", accountID, databaseName: record.databaseName };
    const proof: ResolutionProof = { binding, databaseNames: resolution.databaseNames, record, object };
    const handle = await fs.open(await safeFile(temporary, "proof.puty"), "wx", 0o600);
    try { await handle.writeFile(seal(key, context(manifest.snapshotID, "legacy-resolution", bindingID), Buffer.from(JSON.stringify(proof)))); await handle.sync(); } finally { await handle.close(); }
    await syncDirectory(temporary);
    const verified = await readCapsule(temporary); verified.bytes.fill(0);
    await assertParent();
    try { await fs.rename(temporary, final); await syncDirectory(resolvedBase); }
    catch (error) {
      // A competing valid publisher wins. Never replace a published nonempty
      // directory; retain our encrypted partial for explicit maintenance.
      if (!["EEXIST", "ENOTEMPTY", "EPERM", "EACCES"].includes((error as NodeJS.ErrnoException).code || "")) throw error;
      if (!await fs.lstat(final).then(() => true, missing => { if (missing.code === "ENOENT") return false; throw missing; })) throw error;
    }
    return await readCapsule(final);
  } finally {
    existing?.bytes.fill(0); rawBytes?.fill(0); selected?.fill(0);
    if (Array.isArray(parsedRaw)) for (const row of parsedRaw) if (row && Array.isArray(row.pages)) for (const page of row.pages) {
      const value = page?.value;
      if (value instanceof ArrayBuffer) new Uint8Array(value).fill(0);
      else if (ArrayBuffer.isView(value)) new Uint8Array(value.buffer, value.byteOffset, value.byteLength).fill(0);
    }
  }
}
export async function listLegacySnapshots(root: string, protection?: LegacyProtection): Promise<LegacyReceipt[]> {
  const base = path.join(await canonicalDirectory(root), "legacy"), info = await fs.lstat(base).catch(error => { if (error.code === "ENOENT") return null; throw error; });
  if (!info) return []; await canonicalDirectory(base); const values: LegacyReceipt[] = [];
  for (const name of (await fs.readdir(base)).filter(name => snapshotPattern.test(name)).sort()) {
    const opened = await openSnapshot(root, name, protection);
    try {
      let complete = false;
      try { complete = JSON.parse(unseal(opened.key, context(name, "legacy-cleanup", "progress"), await fs.readFile(await safeFile(opened.directory, "cleanup.puty"))).toString("utf8")).complete === true; }
      catch (error) { if ((error as NodeJS.ErrnoException).code !== "ENOENT") throw error; }
      values.push(receipt(opened.manifest, complete));
    } finally { opened.key.fill(0); }
  }
  return values;
}

function pathKey(value: string): string { return path.win32.isAbsolute(value) ? path.win32.normalize(value).toLowerCase() : path.normalize(value); }
const sourceBasename = (value: string): string => path.win32.basename(path.posix.basename(value));
const localFields = new Set(["filePath", "file_path", "videoPath", "video_path", "snapshotPath", "snapshot_path", "soundPath", "sound_path", "sourcePath", "source_path", "localEx", "local_ex"]);
function messageEnvelope(value: unknown): boolean {
  if (!value || typeof value !== "object" || Array.isArray(value)) return false;
  const message = value as Record<string, unknown>, id = message.clientMsgID ?? message.client_msg_id, type = message.contentType ?? message.content_type;
  return typeof id === "string" && id.length > 0 && id.length <= 512
    && typeof type === "number" && Number.isSafeInteger(type) && type > 0;
}
const tombstoneFields = new Set(["clientMsgID", "client_msg_id", "serverMsgID", "server_msg_id", "sendID", "send_id", "recvID", "recv_id", "groupID", "group_id", "createTime", "create_time", "sendTime", "send_time", "seq", "sessionType", "session_type", "contentType", "content_type", "senderPlatformID", "sender_platform_id", "msgFrom", "msg_from", "isRead", "is_read", "status"]);
function rewriteJSON(value: unknown, paths: Map<string, string>, depth = 0, nativeMessage = false, payloadType?: number): unknown {
  if (depth > 32) throw new Error("Legacy message JSON exceeds depth bound");
  if (Array.isArray(value)) return value.map(item => rewriteJSON(item, paths, depth + 1, nativeMessage && messageEnvelope(item)));
  const envelope = nativeMessage && messageEnvelope(value);
  if (envelope && Number((value as Record<string, unknown>).status) >= 4) {
    // Deleted native messages may also occur in latest_msg or inside a quote or
    // forwarded message. Retain only their identity/order/status, never bodies.
    return { ...Object.fromEntries(Object.entries(value as Record<string, unknown>).filter(([key, item]) => tombstoneFields.has(key) && ["string", "number", "boolean"].includes(typeof item))), content: "" };
  }
  const type = envelope ? Number((value as Record<string, unknown>).contentType ?? (value as Record<string, unknown>).content_type) : undefined;
  if (value && typeof value === "object") return Object.fromEntries(Object.entries(value).map(([key, item]) => {
    if (localFields.has(key) && typeof item === "string") return [key, paths.get(pathKey(item)) || ""];
    // Only a native MessageItem serializes its payload in `content`. A
    // TextElem's identically named field is ordinary user text, even valid JSON.
    if (envelope && key === "content" && typeof item === "string" && /^\s*[\[{]/.test(item)) { try { return [key, JSON.stringify(rewriteJSON(JSON.parse(item), paths, depth + 1, false, type))]; } catch (error) { if (!(error instanceof SyntaxError)) throw error; } }
    const quoted = (payloadType === 106 || payloadType === 114) && (key === "quoteMessage" || key === "quote_message") && messageEnvelope(item);
    const merged = payloadType === 107 && (key === "multiMessage" || key === "multi_message") && Array.isArray(item);
    const elementType = type === 106 && ["atTextElem", "at_text_elem"].includes(key) ? 106
      : type === 114 && ["quoteElem", "quote_elem"].includes(key) ? 114
      : type === 107 && ["mergeElem", "merge_elem"].includes(key) ? 107 : undefined;
    return [key, rewriteJSON(item, paths, depth + 1, quoted || merged, elementType)];
  }));
  return value;
}
export function prepareLegacyImport(SQL: LegacySQL, bytes: Uint8Array, paths = new Map<string, string>()): { snapshot: Uint8Array; events: ArchiveEvent[]; remapped: number; unresolvedLocalPaths: number } {
  verifyLegacySQLite(SQL, bytes); const db = new SQL.Database(bytes), events: ArchiveEvent[] = []; let remapped = 0, unresolvedLocalPaths = 0;
  try {
    db.exec("PRAGMA journal_mode=MEMORY; PRAGMA temp_store=MEMORY; PRAGMA secure_delete=ON;");
    const conversations = new Map<string, boolean>();
    if (rows(db, "SELECT name FROM sqlite_master WHERE type='table' AND name='local_conversations'").length) {
      const fields = rows(db, "PRAGMA table_info(local_conversations)").map(row => row.name);
      if (fields.includes("conversation_id") && fields.includes("is_msg_destruct")) for (const row of rows(db, "SELECT conversation_id,is_msg_destruct FROM local_conversations")) conversations.set(String(row.conversation_id), Number(row.is_msg_destruct) === 1);
      if (fields.includes("conversation_id") && fields.includes("latest_msg")) for (const row of rows(db, "SELECT conversation_id,latest_msg FROM local_conversations")) {
        try { const rewritten = JSON.stringify(rewriteJSON(JSON.parse(String(row.latest_msg)), paths, 0, true)); if (rewritten !== row.latest_msg) db.run("UPDATE local_conversations SET latest_msg=? WHERE conversation_id=?", [rewritten, row.conversation_id]); }
        catch (error) { if (!(error instanceof SyntaxError)) throw error; }
      }
    }
    for (const item of rows(db, "SELECT name FROM sqlite_master WHERE type='table' AND name GLOB 'chat_logs_*'")) {
      const table = String(item.name), conversationID = table.slice("chat_logs_".length), fields = rows(db, `PRAGMA table_info(${identifier(table)})`).map(row => String(row.name));
      if (!/^[\w-]{1,256}$/.test(conversationID) || !fields.includes("client_msg_id") || !fields.includes("send_time")) throw new Error("Legacy message schema cannot be safely assigned");
      for (const row of rows(db, `SELECT * FROM ${identifier(table)}`)) {
        if (!row.client_msg_id) throw new Error("Legacy message has no stable identity");
        const removed = Number(row.status) >= 4;
        // Deleted payloads are not eligible for remap or parsing. Preserve their
        // exact deletion event without letting an obsolete body block import.
        if (removed) for (const field of ["content", "local_ex", "attached_info", "ex"].filter(name => fields.includes(name))) { row[field] = ""; db.run(`UPDATE ${identifier(table)} SET ${identifier(field)}='' WHERE client_msg_id=?`, [row.client_msg_id]); }
        if (fields.includes("local_ex") && row.local_ex) { const mapped = paths.get(pathKey(String(row.local_ex))); mapped ? remapped++ : unresolvedLocalPaths++; row.local_ex = mapped || ""; db.run(`UPDATE ${identifier(table)} SET local_ex=? WHERE client_msg_id=?`, [row.local_ex, row.client_msg_id]); }
        if (typeof row.content === "string" && /^[\s]*[\[{]/.test(row.content)) {
          try { const parsed = JSON.parse(row.content), rewritten = JSON.stringify(rewriteJSON(parsed, paths, 0, false, Number(row.content_type))); if (rewritten !== row.content) { row.content = rewritten; db.run(`UPDATE ${identifier(table)} SET content=? WHERE client_msg_id=?`, [rewritten, row.client_msg_id]); } } catch (error) { if (error instanceof SyntaxError) { /* text/custom payload remains opaque */ } else throw error; }
        }
        let ephemeral = conversations.get(conversationID) === true; try { ephemeral ||= JSON.parse(String(row.attached_info || "{}"))?.isPrivateChat === true; } catch { /* older opaque metadata */ }
        events.push({ kind: removed ? "delete" : "upsert", conversationID, row: ephemeral ? { client_msg_id: row.client_msg_id, send_time: row.send_time, __ephemeral: 1 } : row });
      }
    }
    const snapshot = db.export(); verifyLegacySQLite(SQL, snapshot); return { snapshot, events, remapped, unresolvedLocalPaths };
  } finally { db.close(); }
}
function fileMime(name: string): string { return ({ ".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".webp": "image/webp", ".gif": "image/gif", ".mp4": "video/mp4", ".mp3": "audio/mpeg", ".wav": "audio/wav", ".txt": "text/plain", ".pdf": "application/pdf" } as Record<string, string>)[path.extname(name).toLowerCase()] || "application/octet-stream"; }

/** Must be called only for the currently online-authorized account, before SDK
 * open. Other accounts stay encrypted and isolated until their own login. */
export async function importLegacyAccount(options: { root: string; snapshotID: string; accountID: string; vault: LocalVault; protection?: LegacyProtection; SQL?: LegacySQL; assertCurrent: () => Promise<void> }): Promise<{ imported: boolean; alreadyImported: boolean; messages: number; attachments: number; unresolvedLocalPaths: number }> {
  let step:LegacyImportStep='MATCH_ACCOUNT',key:Buffer|undefined,decryptedSQLite:Buffer|undefined;
  try {
    if (!accountPattern.test(options.accountID) || options.vault.account !== options.accountID) throw new Error("Legacy account is not the active vault account");
    step='OPEN_SNAPSHOT';await options.assertCurrent();
    const opened=await openSnapshot(options.root,options.snapshotID,options.protection),{manifest,directory}=opened;key=opened.key;
    step='MATCH_ACCOUNT';
    let record = manifest.databases.find(row => row.accountID === options.accountID), resolvedObject: ObjectRecord | undefined, resolvedSQL: LegacySQL | undefined, sqliteDirectory = directory;
    if (!record) {
      if (!manifest.unresolved.some(row => typeof row.databaseName === "string" && row.databaseName.endsWith(options.accountID + ".sqlite"))) return { imported: false, alreadyImported: false, messages: 0, attachments: 0, unresolvedLocalPaths: 0 };
      step='RESOLVE_QUARANTINE';
      const prior = (await options.vault.preferences())["legacyImport:" + options.snapshotID] as any;
      resolvedSQL = options.SQL || await loadLegacySQL();
      const resolved = await resolveSealedQuarantine({ directory, manifest, manifestSHA256: opened.manifestSHA256, key, accountID: options.accountID, SQL: resolvedSQL, assertCurrent: options.assertCurrent, completedSHA: prior?.state === "complete" ? prior.sha256 : undefined });
      record = resolved.record; resolvedObject = resolved.object; sqliteDirectory = resolved.directory; decryptedSQLite = resolved.bytes;
    }
    step='CHECK_DESTINATION';
    const markerName = "legacyImport:" + options.snapshotID, preferences = await options.vault.preferences(), prior = preferences[markerName] as any;
    if (prior?.state === "complete" && prior.sha256 === record.sha256) return { imported: false, alreadyImported: true, messages: prior.messages, attachments: prior.attachments, unresolvedLocalPaths: prior.unresolvedLocalPaths };
    if (prior && (prior.state !== "preparing" || prior.sha256 !== record.sha256)) throw new Error("Legacy import marker does not match the sealed source");
    await options.vault.preferences({ [markerName]: { state: "preparing", sha256: record.sha256 } });
    const object = resolvedObject || manifest.objects.find(row => row.role === "sqlite" && row.objectID === record!.objectID); if (!object) throw new Error("Sealed legacy SQLite is absent");
    step='LOAD_SQL';const SQL = resolvedSQL || options.SQL || await loadLegacySQL();
    step='READ_DATABASE';const bytes = decryptedSQLite ||= await memoryObject(sqliteDirectory, key, manifest.snapshotID, object, options.assertCurrent), paths = new Map<string, string>();
    step='VERIFY_DATABASE';
    verifyLegacySQLite(SQL, bytes, options.accountID); const refs = new Map<string, { conversationID: string; clientMsgID: string }[]>(), displayNames = new Map<string, { name: string; priority: number }>();
    // Only exact local paths belonging to this account are eligible for remap.
    const prefix = "OpenIMData/" + options.accountID + "/", sources = manifest.objects.filter(row => row.role === "source" && row.relativePath?.startsWith(prefix));
    const sourcePaths = new Map<string, ObjectRecord>();
    for (const source of sources) for (const alias of manifest.sourceAliases) sourcePaths.set(pathKey(path.join(alias, ...source.relativePath!.split("/"))), source);
    step='SCAN_MESSAGES';const rawDB = new SQL.Database(bytes);
    try {
      for (const table of rows(rawDB, "SELECT name FROM sqlite_master WHERE type='table' AND name GLOB 'chat_logs_*'")) {
        const name = String(table.name), conversationID = name.slice("chat_logs_".length); if (!/^[\w-]{1,256}$/.test(conversationID)) throw new Error("Invalid legacy conversation identity");
        for (const row of rows(rawDB, `SELECT * FROM ${identifier(name)}`)) {
          if (Number(row.status) >= 4) continue; const values: string[] = [];
          const prefer = (localPath: unknown, name: unknown, priority: number): void => { if (typeof localPath !== "string" || typeof name !== "string" || !name.trim()) return; const source = sourcePaths.get(pathKey(localPath)); if (source && (displayNames.get(source.objectID)?.priority ?? -1) < priority) displayNames.set(source.objectID, { name: sourceBasename(name), priority }); };
          const collect = (value: unknown, depth = 0): void => {
            if (depth > 32) return;
            if (typeof value === "string") values.push(value);
            else if (Array.isArray(value)) value.forEach(item => collect(item, depth + 1));
            else if (value && typeof value === "object") {
              const object = value as Record<string, unknown>;
              prefer(object.filePath ?? object.file_path, object.fileName ?? object.file_name, 10);
              for (const field of ["videoPath", "video_path", "snapshotPath", "snapshot_path", "sourcePath", "source_path", "soundPath", "sound_path"]) prefer(object[field], object[field], 5);
              for (const item of Object.values(object)) collect(item, depth + 1);
            }
          };
          if (typeof row.local_ex === "string") values.push(row.local_ex);
          try { const content = JSON.parse(String(row.content)); collect(content); const media = content.fileElem || content.videoElem || content; prefer(row.local_ex, media.fileName || media.file_name, 10); prefer(row.local_ex, media.videoPath || media.video_path || media.sourcePath || media.source_path, 5); } catch { /* opaque non-media text */ }
          for (const value of values) { const candidate = sourcePaths.get(pathKey(value)); if (!candidate) continue; const entries = refs.get(candidate.objectID) || []; if (!entries.some(ref => ref.conversationID === conversationID && ref.clientMsgID === String(row.client_msg_id))) entries.push({ conversationID, clientMsgID: String(row.client_msg_id) }); refs.set(candidate.objectID, entries); }
        }
      }
    } finally { rawDB.close(); }
    step='IMPORT_ATTACHMENT';let attachments = 0;
    for (const source of sources) {
      const messageRefs = refs.get(source.objectID); if (!messageRefs?.length) continue;
      await options.assertCurrent(); const taskID = "legacy_" + digest(manifest.snapshotID + "\0" + options.accountID + "\0" + source.objectID), name = displayNames.get(source.objectID)?.name || path.posix.basename(source.relativePath!);
      await options.vault.reserveAttachment({ taskID, name, messageRefs, mime: fileMime(name), downloadedAt:source.mtimeMs });
      await options.vault.writeAttachment(taskID, decryptObject(directory, key, manifest.snapshotID, source, options.assertCurrent), source.size, source.sha256);
      for (const alias of manifest.sourceAliases) paths.set(pathKey(path.join(alias, ...source.relativePath!.split("/"))), "puty-local://attachment/" + taskID); attachments++;
    }
    step='PREPARE_MESSAGES';const prepared = prepareLegacyImport(SQL, bytes, paths); bytes.fill(0); await options.assertCurrent();
    step='COMMIT_HISTORY';
    const imported = await options.vault.importLegacy(prepared.snapshot, prepared.events, digest(manifest.snapshotID + "\0" + options.accountID + "\0" + record.sha256));
    step='COMMIT_MARKER';
    await options.vault.preferences({ [markerName]: { state: "complete", sha256: record.sha256, messages: prepared.events.length, attachments, unresolvedLocalPaths: prepared.unresolvedLocalPaths } });
    await options.assertCurrent(); return { imported: imported.imported, alreadyImported: imported.alreadyImported, messages: prepared.events.length, attachments, unresolvedLocalPaths: prepared.unresolvedLocalPaths };
  } catch(error) { throw legacyImportFailure(step,error); }
  finally { decryptedSQLite?.fill(0); key?.fill(0); }
}

/** Explicit cleanup only after full readback and source equality. A locked or
 * changed source is preserved, never forcibly removed. No recursive rm is used. */
export async function cleanupLegacyPlaintext(options: Options & { snapshotID: string; currentUserData: string; assertCurrent: () => Promise<void> }): Promise<LegacyReceipt> {
  const { source } = await scope(options, true), current = await canonicalDirectory(options.currentUserData);
  if (nested(current, source) || nested(source, current)) throw new Error("Active app userData cannot be cleaned as legacy storage");
  const { manifest, key, directory } = await openSnapshot(options.root, options.snapshotID, options.protection);
  try {
    if (!samePath(manifest.source, source)) throw new Error("Legacy snapshot source does not match cleanup scope");
    await options.assertCurrent();
    const progressFile = await safeFile(directory, "cleanup.puty"); let progress: { started: boolean; deleted: string[]; complete?: boolean; retainedNativeLockFile?: boolean } = { started: false, deleted: [] };
    try { progress = JSON.parse(unseal(key, context(manifest.snapshotID, "legacy-cleanup", "progress"), await fs.readFile(progressFile)).toString("utf8")); }
    catch (error) { if ((error as NodeJS.ErrnoException).code !== "ENOENT") throw error; }
    const files = manifest.objects.filter(row => row.role === "source").map(row => ({ relativePath: relative(row.relativePath!), size: row.size, mtimeMs: row.mtimeMs!, sha256: row.sha256 }));
    const approved = new Set(files.map(row => row.relativePath)); if (typeof progress.started !== "boolean" || !Array.isArray(progress.deleted) || progress.deleted.some(name => !approved.has(name)) || progress.complete !== undefined && typeof progress.complete !== "boolean") throw new Error("Legacy cleanup progress is invalid");
    // Windows Electron's original single-instance lock is deliberately held by
    // the parent Main to stop an older client writing during migration. Its
    // fixed zero-byte lockfile is native coordination metadata, not a cache.
    const nativeLock = async (): Promise<boolean> => {
      if (process.platform !== "win32") return false;
      await options.assertCurrent(); await cleanupSource(source);
      const target = path.join(source, "lockfile"), info = await fs.lstat(target).catch(error => { if (error.code === "ENOENT") return null; throw error; });
      if (!info) return false;
      if (!info.isFile() || info.isSymbolicLink() || info.nlink !== 1 || info.size !== 0) throw new Error("Legacy native lock metadata is invalid");
      const original = files.find(row => row.relativePath === "lockfile");
      if (original && (original.size !== 0 || original.sha256 !== digest(Buffer.alloc(0)))) throw new Error("Legacy native lock metadata is not empty");
      if (options.nativeSingletonLockHeld?.() !== true) throw Object.assign(new Error("Legacy native lock requires its current parent owner"), { code: "EBUSY" });
      if ((await fs.readFile(target)).length || options.nativeSingletonLockHeld?.() !== true) throw new Error("Legacy native lock ownership changed");
      return true;
    };
    const retained = await nativeLock(), managed = files.filter(row => !retained || row.relativePath !== "lockfile");
    if (progress.complete === true) {
      await assertInventory(source, [], [], true, retained); await options.assertCurrent();
      if (retained !== await nativeLock()) throw new Error("Legacy native lock ownership changed");
      return receipt(manifest, true);
    }
    for (const item of manifest.objects) for await (const _chunk of decryptObject(directory, key, manifest.snapshotID, item, options.assertCurrent)) { /* every backup is verified before removal */ }
    const remaining = await fs.readdir(source).catch(error => { if (error.code === "ENOENT") return [] as string[]; throw error; });
    // A user may have removed the old profile and Main recreated only its lock.
    // There is then no business plaintext to remove, but sealed data was still
    // fully authenticated above before acknowledging this completed state.
    const noBusinessSource = remaining.every(name => retained && name === "lockfile");
    await assertInventory(source, managed.filter(row => !progress.deleted.includes(row.relativePath)), manifest.directories, progress.started || noBusinessSource, retained);
    const save = async () => atomicWrite(progressFile, seal(key, context(manifest.snapshotID, "legacy-cleanup", "progress"), Buffer.from(JSON.stringify(progress))));
    if (!progress.started) { progress.started = true; await save(); }
    for (const row of managed) {
      if (progress.deleted.includes(row.relativePath)) continue; await options.assertCurrent(); await cleanupSource(source);
      const target = path.join(source, ...row.relativePath.split("/")); let exists = true; const sha = createHash("sha256");
      try { for await (const bytes of fileBytes(target, row)) sha.update(bytes); }
      catch (error) { if ((error as NodeJS.ErrnoException).code === "ENOENT" && progress.started) exists = false; else throw error; }
      if (exists) { if (sha.digest("hex") !== row.sha256) throw new Error("Legacy file changed before deletion; remaining plaintext retained"); await fs.unlink(target); await syncDirectory(path.dirname(target)); }
      progress.deleted.push(row.relativePath); await save();
    }
    for (const rel of [...manifest.directories].sort((a, b) => b.split("/").length - a.split("/").length)) await fs.rmdir(path.join(source, ...relative(rel).split("/"))).catch(error => { if (error.code !== "ENOENT") throw error; });
    // Keep the original root and its actively held native lock. Any other new
    // file or directory leaves cleanup incomplete; no recursive rm is used.
    await assertInventory(source, [], [], true, retained); await options.assertCurrent();
    if (retained !== await nativeLock()) throw new Error("Legacy native lock ownership changed");
    progress.retainedNativeLockFile = retained; progress.complete = true; await save(); return receipt(manifest, true);
  } finally { key.fill(0); }
}
