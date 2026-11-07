import { createHash } from "node:crypto";
import { promises as fs } from "node:fs";
import path from "node:path";

// Only the import coordinator may consume this private in-memory result. This
// module never opens a normal SDK, writes a destination or deletes old data.
export type LegacyPage = { key: number; value: unknown };
export type LegacySQLite = { databaseName: string; accountID: string; bytes: Uint8Array; sha256: string; pageSize: number; pageCount: number; ignoredTrailingPages: number; requiresIntegrityCheck: true };
export type LegacyRawDatabase = { name: string; version: number; stores: string[]; pages?: LegacyPage[] };
export type LegacyCacheFile = { accountID?: string; relativePath: string; size: number; mtimeMs: number };
export type LegacyReadout = { source: string; sourceAliases?: string[]; databases: LegacySQLite[]; rawDatabases: LegacyRawDatabase[]; unresolved: { databaseName: string; reason: string }[]; cacheFiles: LegacyCacheFile[]; deletionAuthorized: false };
const maxDatabaseBytes = 256 * 1024 * 1024;
const maxTotalBytes = 1024 * 1024 * 1024;

function bytesOf(value: unknown): Uint8Array {
  if (value instanceof ArrayBuffer) return new Uint8Array(value);
  if (ArrayBuffer.isView(value)) return new Uint8Array(value.buffer, value.byteOffset, value.byteLength);
  throw new Error("Legacy page is not binary");
}

export function reconstructLegacySQLite(databaseName: string, pages: LegacyPage[]): LegacySQLite {
  // SQLiteFS passes the normalized basename to createFile. The SDK's dir may
  // contribute a safe basename prefix; older -sql- names remain compatible.
  // A name only selects a candidate; SQL self identity must still pass.
  const match = /^[^\\\x00-\x1f\x7f/]*?(emp_[a-f0-9]{32})\.sqlite$/.exec(databaseName);
  if (!match || match[0] !== databaseName || !Array.isArray(pages)) throw new Error("Legacy database identity is unrecognized");
  const blocks = new Map<number, unknown>();
  for (const page of pages) {
    if (!Number.isSafeInteger(page.key) || page.key < -1 || blocks.has(page.key)) throw new Error("Legacy page keys are ambiguous");
    blocks.set(page.key, page.value);
  }
  const meta = blocks.get(-1) as { size?: number } | undefined;
  const size = meta?.size;
  if (!Number.isSafeInteger(size) || size! < 512 || size! > maxDatabaseBytes) throw new Error("Legacy SQLite size is unsupported");
  const first = bytesOf(blocks.get(0));
  if (first.byteLength < 100 || Buffer.from(first.subarray(0, 16)).toString("binary") !== "SQLite format 3\0") throw new Error("Legacy SQLite header is invalid");
  const word = (first[16] << 8) | first[17], pageSize = word === 1 ? 65536 : word;
  if (pageSize < 512 || pageSize > 65536 || (pageSize & (pageSize - 1)) || size! % pageSize) throw new Error("Legacy SQLite page geometry is invalid");
  const pageCount = size! / pageSize, bytes = new Uint8Array(size!);
  for (let index = 0; index < pageCount; index++) {
    if (!blocks.has(index)) throw new Error("Legacy SQLite page is missing; preserve raw database");
    const block = bytesOf(blocks.get(index));
    if (block.byteLength !== pageSize) throw new Error("Legacy SQLite page length is invalid");
    bytes.set(block, index * pageSize);
  }
  // Absurd-SQL intentionally leaves blocks after a truncated logical EOF. Do
  // not resurrect them in the SQLite import; the sealed raw bundle retains them.
  const ignoredTrailingPages = [...blocks.keys()].filter(key => key >= pageCount).length;
  return { databaseName, accountID: match[1], bytes, sha256: createHash("sha256").update(bytes).digest("hex"), pageSize, pageCount, ignoredTrailingPages, requiresIntegrityCheck: true };
}

// This function is injected as plain trusted code into an empty hidden page.
// No SDK/SQLiteFS initialization or readwrite/deleteDatabase call is allowed.
export async function readLegacyIDBPages(): Promise<LegacyRawDatabase[]> {
  const descriptions = await indexedDB.databases();
  if (descriptions.length > 128) throw new Error("Legacy database count exceeds bound");
  const output: LegacyRawDatabase[] = [];
  let total = 0;
  for (const description of descriptions.sort((a, b) => String(a.name).localeCompare(String(b.name)))) {
    if (typeof description.name !== "string" || description.name.length > 1024) throw new Error("Legacy database description is invalid");
    const db = await new Promise<IDBDatabase>((resolve, reject) => {
      const request = indexedDB.open(description.name!);
      request.onupgradeneeded = () => { request.transaction?.abort(); reject(new Error("Legacy database changed during discovery")); };
      request.onerror = () => reject(new Error("Legacy database could not be opened read-only"));
      request.onblocked = () => reject(new Error("Legacy database is still active"));
      request.onsuccess = () => resolve(request.result);
    });
    try {
      const row: LegacyRawDatabase = { name: description.name, version: db.version, stores: Array.from(db.objectStoreNames) };
      if (db.version === 2 && row.stores.length === 1 && row.stores[0] === "data") {
        row.pages = await new Promise<LegacyPage[]>((resolve, reject) => {
          const transaction = db.transaction(["data"], "readonly"), pages: LegacyPage[] = [];
          let count = 0;
          const request = transaction.objectStore("data").openCursor();
          request.onsuccess = () => {
            const cursor = request.result;
            if (!cursor) return;
            const value: unknown = cursor.value;
            const length = value instanceof ArrayBuffer ? value.byteLength : ArrayBuffer.isView(value) ? value.byteLength : 0;
            total += length;
            if (!Number.isSafeInteger(cursor.key) || ++count > 524288 || total > 1024 * 1024 * 1024) { transaction.abort(); return; }
            pages.push({ key: cursor.key as number, value }); cursor.continue();
          };
          transaction.oncomplete = () => resolve(pages);
          transaction.onerror = transaction.onabort = () => reject(new Error("Legacy readonly snapshot failed or exceeded bound"));
        });
      }
      output.push(row);
    } finally { db.close(); }
  }
  return output;
}

export async function inventoryLegacyCaches(userData: string): Promise<LegacyCacheFile[]> {
  if (!path.isAbsolute(userData)) throw new Error("Legacy root must be absolute");
  const root = await fs.realpath(userData), base = path.join(root, "OpenIMData"), files: LegacyCacheFile[] = [];
  const meta = await fs.lstat(base).catch(error => { if (error.code === "ENOENT") return null; throw error; });
  if (!meta) return files;
  if (!meta.isDirectory() || meta.isSymbolicLink()) throw new Error("Legacy cache root is linked or invalid");
  const cacheKinds = new Set(["imageCache", "videoCache", "fileCache", "avatarCache", "sentFileCache"]);
  const accounts = await fs.readdir(base, { withFileTypes: true });
  for (const account of accounts) {
    if(account.isSymbolicLink())throw new Error("Legacy account directory is linked");
    if (!account.isDirectory()) continue;
    const directory = path.join(base, account.name);
    if ((await fs.lstat(directory)).isSymbolicLink()) throw new Error("Legacy account directory is linked");
    const accountID = /^emp_[a-f0-9]{32}$/.test(account.name) ? account.name : undefined;
    for (const cache of await fs.readdir(directory, { withFileTypes: true })) {
      if (!cacheKinds.has(cache.name)) continue;
      const walk = async (current: string): Promise<void> => {
        const info = await fs.lstat(current);
        if (info.isSymbolicLink() || !current.startsWith(root + path.sep)) throw new Error("Legacy cache contains a link or escaped path");
        if (info.isDirectory()) { for (const child of await fs.readdir(current)) await walk(path.join(current, child)); }
        else if (info.isFile()) {
          if (files.length >= 100000) throw new Error("Legacy cache file count exceeds bound");
          files.push({ accountID, relativePath: path.relative(root, current), size: info.size, mtimeMs: info.mtimeMs });
        } else throw new Error("Legacy cache contains a special file");
      };
      await walk(path.join(directory, cache.name));
    }
  }
  return files;
}

export function planLegacyImport(source: string, rawDatabases: LegacyRawDatabase[], cacheFiles: LegacyCacheFile[]): LegacyReadout {
  const result: LegacyReadout = { source, rawDatabases, cacheFiles, databases: [], unresolved: [], deletionAuthorized: false };
  const ambiguous = new Set<string>();
  let bytes = 0;
  for (const row of rawDatabases) {
    try {
      if (!row.pages || row.version !== 2 || row.stores.length !== 1 || row.stores[0] !== "data") throw new Error("Legacy schema is unrecognized; retain encrypted raw partition backup");
      const db = reconstructLegacySQLite(row.name, row.pages);
      bytes += db.bytes.length; if (bytes > maxTotalBytes) throw new Error("Legacy SQLite total exceeds bound");
      if (ambiguous.has(db.accountID) || result.databases.some(other => other.accountID === db.accountID)) { ambiguous.add(db.accountID);result.databases=result.databases.filter(other=>other.accountID!==db.accountID);throw new Error("Multiple legacy SQLite files claim one account"); }
      result.databases.push(db);
    } catch (error) { result.unresolved.push({ databaseName: row.name, reason: (error as Error).message }); }
  }
  return result;
}

export async function readLegacyPartition({ writersStopped, onPhase }: { writersStopped: true; onPhase?: (phase: "reader-created" | "debugger-ready" | "page-loaded" | "snapshot-read") => void }): Promise<LegacyReadout> {
  if (writersStopped !== true) throw new Error("Stop legacy SDK writers before snapshot");
  const { app, BrowserWindow, session } = await import("electron");
  const source = path.join(app.getPath("appData"), "openim-enterprise-upgrade"), partition = path.join(source, "Partitions", "part0");
  for (const directory of [source, path.join(source, "Partitions"), partition]) {
    const info = await fs.lstat(directory); if (!info.isDirectory() || info.isSymbolicLink()) throw new Error("Legacy partition path is linked or invalid");
  }
  const target = session.fromPath(await fs.realpath(partition), { cache: false });
  if (await target.protocol.isProtocolHandled("openim38")) throw new Error("Legacy partition is already in use");
  const page = "openim38://app/__legacy-readonly__";
  let bootstrapping = true;
  target.webRequest.onBeforeRequest((details, callback) => callback({ cancel: details.url !== page && !(bootstrapping && details.url === "about:blank") }));
  target.setPermissionRequestHandler((_webContents, _permission, callback) => callback(false));
  target.setPermissionCheckHandler(() => false);
  target.protocol.handle("openim38", request => new Response(request.url === page ? "<!doctype html><meta charset=utf-8><title>Local import</title>" : "Blocked", { status: request.url === page ? 200 : 403, headers: { "Content-Type": "text/html", "Cache-Control": "no-store", "Content-Security-Policy": "default-src 'none'; connect-src 'none'; frame-src 'none'; worker-src 'none'; object-src 'none'; base-uri 'none'", "Cross-Origin-Opener-Policy": "same-origin", "Cross-Origin-Embedder-Policy": "require-corp" } }));
  const hidden = new BrowserWindow({ show: false, webPreferences: { session: target, sandbox: true, contextIsolation: true, nodeIntegration: false, webSecurity: true, devTools: true } });
  let rendererFailure = '';
  hidden.webContents.on('render-process-gone', (_event, details) => {
    if (['oom', 'launch-failed', 'crashed', 'killed', 'integrity-failure', 'memory-eviction', 'abnormal-exit'].includes(details.reason)) rendererFailure = details.reason;
  });
  onPhase?.("reader-created");
  hidden.webContents.setWindowOpenHandler(() => ({ action: "deny" }));
  hidden.webContents.on("will-navigate", (event, url) => { if (url !== page && !(bootstrapping && url === "about:blank")) event.preventDefault(); });
  try {
    const bounded = async <T>(promise: Promise<T>, ms: number): Promise<T> => { let timer: ReturnType<typeof setTimeout>; return Promise.race([promise, new Promise<never>((_, reject) => { timer = setTimeout(() => reject(new Error("Legacy reader deadline exceeded")), ms); })]).finally(() => clearTimeout(timer!)); };
    // Start a renderer in an empty, opaque page before CDP commands. Sending
    // Network.enable to an unstarted renderer can wait indefinitely. The old
    // storage origin is never navigated until service workers are bypassed.
    await bounded(hidden.loadURL("about:blank"), 10000);
    hidden.webContents.debugger.attach("1.3");
    await bounded(hidden.webContents.debugger.sendCommand("Network.enable"), 10000);
    await bounded(hidden.webContents.debugger.sendCommand("Network.setBypassServiceWorker", { bypass: true }), 10000);
    bootstrapping = false;
    onPhase?.("debugger-ready");
    await bounded(hidden.loadURL(page), 10000);
    onPhase?.("page-loaded");
    const snapshot = hidden.webContents.executeJavaScript(`(${readLegacyIDBPages.toString()})()`);
    let timer: ReturnType<typeof setTimeout>;
    const raw = await Promise.race([snapshot, new Promise<never>((_, reject) => { timer = setTimeout(() => reject(new Error("Legacy snapshot deadline exceeded")), 120000); })]).finally(() => clearTimeout(timer!)) as LegacyRawDatabase[];
    onPhase?.("snapshot-read");
    return planLegacyImport(source, raw, await inventoryLegacyCaches(source));
  } catch (error) {
    // A fixed reason distinguishes renderer/commit exhaustion from a rejected
    // navigation, without forwarding legacy URLs, database names or raw errors.
    if (rendererFailure) throw Object.assign(new Error('Legacy renderer failed'), {
      code: 'LEGACY_RENDERER_' + rendererFailure.replaceAll('-', '_').toUpperCase()
    });
    throw error;
  } finally {
    if (!hidden.isDestroyed()) { if (hidden.webContents.debugger.isAttached()) hidden.webContents.debugger.detach(); hidden.destroy(); }
    target.protocol.unhandle("openim38"); target.webRequest.onBeforeRequest(null);
  }
}
