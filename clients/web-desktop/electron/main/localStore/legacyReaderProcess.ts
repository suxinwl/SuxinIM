import { createHash, randomUUID } from "node:crypto";
import { promises as fs } from "node:fs";
import { createWriteStream, lstatSync, readFileSync, realpathSync } from "node:fs";
import type { Readable } from "node:stream";
import path from "node:path";
import { spawn } from "node:child_process";
import { serialize, deserialize } from "node:v8";
import { inventoryLegacyCaches, planLegacyImport, readLegacyPartition, type LegacyReadout, type LegacyRawDatabase, type LegacyCacheFile } from "./legacy";

const modeFlag = "--puty-legacy-reader";
const runtimeFlag = "--puty-legacy-reader-runtime=";
const MAGIC = Buffer.from("PUTYLR1\n", "ascii");
const HEADER = 48, MAX_PACKET_BYTES = 1024 * 1024 * 1024 + 64 * 1024 * 1024;
const same = (a: string, b: string) => process.platform === "win32" ? a.toLowerCase() === b.toLowerCase() : a === b;
const within = (a: string, b: string) => same(a, b) || (process.platform === "win32" ? a.toLowerCase().startsWith(b.toLowerCase() + path.sep) : a.startsWith(b + path.sep));
const diagnosticStages = new Set(['startup', 'runtime-profile', 'app-ready', 'source-ready', 'reader-created', 'debugger-ready', 'page-loaded', 'snapshot-read', 'packet-ready', 'pipe-flushed', 'deadline']);
const diagnosticCodes = new Set(['UNKNOWN', 'LOAD_FAILED', 'LEGACY_RENDERER_OOM', 'LEGACY_RENDERER_LAUNCH_FAILED', 'LEGACY_RENDERER_CRASHED', 'LEGACY_RENDERER_KILLED', 'LEGACY_RENDERER_INTEGRITY_FAILURE', 'LEGACY_RENDERER_MEMORY_EVICTION', 'LEGACY_RENDERER_ABNORMAL_EXIT']);
/** Parse only this protocol's fixed enums. Never return any raw child diagnostic. */
export function legacyReaderFailureCategory(input: string): string | undefined {
  for (const match of input.slice(-2048).matchAll(/(?:^|\n)PUTY_LEGACY_FAILED:([a-z-]+):([A-Z_]+)\r?\n/g))
    if (diagnosticStages.has(match[1]) && diagnosticCodes.has(match[2])) return match[1] + '/' + match[2];
  return undefined;
}

/** Call this in the entry-point flag branch before app.ready or normal imports.
 * The parent creates this exact private, disposable child profile. */
export function legacyReaderRuntimeFromArgs(argv: string[] = process.argv): string {
  const flags = argv.filter(value => value === modeFlag), values = argv.filter(value => value.startsWith(runtimeFlag));
  if (flags.length !== 1 || values.length !== 1) throw new Error("Invalid legacy reader mode flags");
  const value = values[0].slice(runtimeFlag.length);
  if (!path.isAbsolute(value) || path.resolve(value) !== value || !/^\.reader-runtime-[a-f0-9-]{36}$/.test(path.basename(value)) || path.basename(path.dirname(value)) !== "legacy") throw new Error("Invalid isolated legacy reader profile");
  let cursor = path.parse(value).root;
  for (const part of path.relative(cursor, value).split(path.sep).filter(Boolean)) { cursor = path.join(cursor, part); const info = lstatSync(cursor); if (!info.isDirectory() || info.isSymbolicLink()) throw new Error("Legacy reader profile is linked"); }
  const marker = path.join(value, ".puty-reader-owned"), info = lstatSync(marker), id = path.basename(value).slice(".reader-runtime-".length);
  if (!info.isFile() || info.isSymbolicLink() || info.nlink !== 1 || readFileSync(marker, "utf8") !== id) throw new Error("Legacy reader profile ownership is invalid");
  return realpathSync.native(value);
}
async function directory(value: string): Promise<string> {
  if (!path.isAbsolute(value) || path.resolve(value) !== value) throw new Error("Legacy reader path is not absolute");
  let current = path.parse(value).root;
  for (const part of path.relative(current, value).split(path.sep).filter(Boolean)) { current = path.join(current, part); const info = await fs.lstat(current); if (!info.isDirectory() || info.isSymbolicLink()) throw new Error("Legacy reader path is linked"); }
  return fs.realpath(value);
}
function headerLength(header: Buffer): number {
  if (header.length !== HEADER || !header.subarray(0, 8).equals(MAGIC)) throw new Error("Legacy reader frame header is invalid");
  const length = header.readBigUInt64BE(8); if (length < 1n || length > BigInt(MAX_PACKET_BYTES)) throw new Error("Legacy reader frame exceeds bound"); return Number(length);
}
export function encodeLegacyReadoutPacket(readout: LegacyReadout): Buffer {
  // Reconstructed SQLite is deliberately not duplicated on the pipe: the parent
  // reconstructs it from these original binary pages after the child has exited.
  const body = serialize({ format: "PUTY-LEGACY-READOUT-1", source: readout.source, sourceAliases: readout.sourceAliases || [], rawDatabases: readout.rawDatabases, cacheFiles: readout.cacheFiles });
  if (body.length > MAX_PACKET_BYTES) throw new Error("Legacy reader result exceeds bound");
  const header = Buffer.alloc(HEADER); MAGIC.copy(header); header.writeBigUInt64BE(BigInt(body.length), 8); createHash("sha256").update(body).digest().copy(header, 16); return Buffer.concat([header, body]);
}
export function decodeLegacyReadoutPacket(packet: Uint8Array, expectedSource: string): LegacyReadout {
  const bytes = Buffer.from(packet), length = headerLength(bytes.subarray(0, HEADER));
  if (bytes.length !== HEADER + length || !createHash("sha256").update(bytes.subarray(HEADER)).digest().equals(bytes.subarray(16, HEADER))) throw new Error("Legacy reader frame length or checksum is invalid");
  const data = deserialize(bytes.subarray(HEADER)) as { format: string; source: string; sourceAliases: string[]; rawDatabases: LegacyRawDatabase[]; cacheFiles: LegacyCacheFile[] };
  if (data?.format !== "PUTY-LEGACY-READOUT-1" || typeof data.source !== "string" || !same(data.source, expectedSource) || !Array.isArray(data.rawDatabases) || data.rawDatabases.length > 128 || !Array.isArray(data.cacheFiles) || data.cacheFiles.length > 100000) throw new Error("Legacy reader result scope is invalid");
  if (!Array.isArray(data.sourceAliases) || data.sourceAliases.length > 4 || data.sourceAliases.some(alias => typeof alias !== "string" || !path.isAbsolute(alias) || !same(realpathSync.native(alias), expectedSource))) throw new Error("Legacy reader source alias is invalid");
  let binaryBytes = 0;
  for (const row of data.rawDatabases) {
    if (!row || typeof row.name !== "string" || row.name.length > 1024 || !Number.isSafeInteger(row.version) || row.version < 1 || !Array.isArray(row.stores) || row.stores.length > 128 || row.stores.some(name => typeof name !== "string" || name.length > 1024)) throw new Error("Legacy reader database descriptor is invalid");
    if (row.pages !== undefined) {
      if (!Array.isArray(row.pages) || row.pages.length > 524288) throw new Error("Legacy reader page count exceeds bound");
      for (const page of row.pages) { if (!Number.isSafeInteger(page.key) || page.key < -1) throw new Error("Legacy reader page key is invalid"); const value = page.value; binaryBytes += value instanceof ArrayBuffer ? value.byteLength : ArrayBuffer.isView(value) ? value.byteLength : 0; if (binaryBytes > 1024 * 1024 * 1024) throw new Error("Legacy reader binary pages exceed bound"); }
    }
  }
  for (const file of data.cacheFiles) if (!file || typeof file.relativePath !== "string" || !file.relativePath || path.isAbsolute(file.relativePath) || file.relativePath.split(/[\\/]/).some(part => !part || part === ".." || part === ".") || /[\x00-\x1f]/.test(file.relativePath) || !Number.isSafeInteger(file.size) || file.size < 0 || !Number.isFinite(file.mtimeMs) || file.accountID !== undefined && !/^emp_[a-f0-9]{32}$/.test(file.accountID)) throw new Error("Legacy reader cache descriptor is invalid");
  const result = planLegacyImport(expectedSource, data.rawDatabases, data.cacheFiles); result.sourceAliases = data.sourceAliases; return result;
}
async function removeReaderRuntime(value: string, id: string): Promise<void> {
  const root = await directory(value); if (!same(root, value) && process.platform !== "win32") throw new Error("Legacy reader profile changed");
  const marker = path.join(root, ".puty-reader-owned"), info = await fs.lstat(marker);
  if (!info.isFile() || info.isSymbolicLink() || (await fs.readFile(marker, "utf8")) !== id) throw new Error("Legacy reader profile ownership changed");
  const remove = async (current: string): Promise<void> => {
    const info = await fs.lstat(current); if (info.isSymbolicLink() || !within(current, root)) throw new Error("Legacy reader profile contains a link");
    if (info.isDirectory()) { for (const name of await fs.readdir(current)) await remove(path.join(current, name)); await fs.rmdir(current); }
    else if (info.isFile()) await fs.unlink(current); else throw new Error("Legacy reader profile contains a special file");
  };
  await remove(root);
}

/** Parent only: keep the application's original single-instance lock while
 * awaiting this. Resolve only after child close, never merely after packet EOF. */
export async function readLegacyInChild(options: { root: string; source: string; writersStopped: true; assertCurrent: () => Promise<void> }): Promise<LegacyReadout> {
  if (options.writersStopped !== true) throw new Error("Legacy writer exclusion is required");
  const { app } = await import("electron"), root = await directory(options.root), source = await directory(options.source), expected = await directory(path.join(app.getPath("appData"), "openim-enterprise-upgrade"));
  if (!same(source, expected) || within(root, source) || within(source, root)) throw new Error("Legacy reader source is not the application's old storage");
  await options.assertCurrent(); const base = path.join(root, "legacy"); await fs.mkdir(base).catch(error => { if (error.code !== "EEXIST") throw error; }); await directory(base);
  const id = randomUUID(), runtime = path.join(base, ".reader-runtime-" + id); await fs.mkdir(runtime); await fs.writeFile(path.join(runtime, ".puty-reader-owned"), id, { flag: "wx", mode: 0o600 });
  const environment = { ...process.env }; delete environment.ELECTRON_RUN_AS_NODE;
  const args = [...(app.isPackaged ? [] : [app.getAppPath()]), modeFlag, runtimeFlag + runtime];
  // A dedicated binary pipe avoids Electron's native Windows stdout preamble;
  // never scan past arbitrary bytes looking for a magic marker.
  const child = spawn(process.execPath, args, { windowsHide: true, env: environment, stdio: ["ignore", "ignore", "pipe", "pipe"] });
  let header = Buffer.alloc(0), body: Buffer | undefined, received = 0, failed = false, polling = false;
  let childCategory = 'no-diagnostic', diagnosticTail = '';
  let failure: Error | undefined; const rejectChild = (error: unknown) => { if (failed) return; failed = true; failure = error instanceof Error ? error : new Error("Legacy reader failed"); child.kill(); };
  const timer = setTimeout(() => rejectChild(new Error("Legacy reader child deadline exceeded")), 180000), lease = setInterval(() => { if (polling || failed) return; polling = true; options.assertCurrent().catch(rejectChild).finally(() => { polling = false; }); }, 1000);
  try {
    (child.stdio[3] as Readable).on("data", (value: Buffer) => {
      if (failed) return;
      try {
        let cursor = 0;
        if (header.length < HEADER) { const count = Math.min(HEADER - header.length, value.length); header = Buffer.concat([header, value.subarray(0, count)]); cursor += count; if (header.length === HEADER) body = Buffer.allocUnsafe(headerLength(header)); }
        if (cursor < value.length) { if (!body || received + value.length - cursor > body.length) throw new Error("Legacy reader returned extra or oversized bytes"); value.copy(body, received, cursor); received += value.length - cursor; }
      } catch (error) { rejectChild(error); }
    });
    // Chromium diagnostics may contain old local names. Discard raw lines;
    // only this protocol's fixed stage/code enums can reach an error receipt.
    child.stderr!.on("data", (value: Buffer) => {
      diagnosticTail = (diagnosticTail + value.toString('ascii')).slice(-2048);
      childCategory = legacyReaderFailureCategory(diagnosticTail) || childCategory;
    });
    const exitCode = await new Promise<number | null>((resolve, reject) => { child.once("error", () => reject(new Error("Legacy reader child could not start"))); child.once("close", resolve); });
    if (failure) throw failure;
    if (exitCode !== 0 || !body || received !== body.length) throw new Error("Legacy reader child did not finish a complete snapshot [" + childCategory + "]");
    await options.assertCurrent();
    return decodeLegacyReadoutPacket(Buffer.concat([header, body]), source);
  } finally {
    clearTimeout(timer); clearInterval(lease);
    // The disposable profile has never hosted a business window or SDK. The
    // old source tree is not touched by this exact owned-profile cleanup.
    await removeReaderRuntime(runtime, id);
  }
}

/** Child only. The entry point must set the isolated userData synchronously,
 * skip ordinary application initialization, and call this instead of main(). */
export async function runLegacyReaderMode(): Promise<void> {
  const { app } = await import("electron"); const keepAlive = () => {}; app.on("window-all-closed", keepAlive);
  let stage = 'startup';
  const deadline = setTimeout(() => { process.stderr.write("\nPUTY_LEGACY_FAILED:deadline:UNKNOWN\n"); app.exit(1); }, 165000);
  try {
    const runtime = legacyReaderRuntimeFromArgs(); stage = 'runtime-profile'; await app.whenReady(); stage = 'app-ready';
    if (!same(await directory(app.getPath("userData")), await directory(runtime))) throw new Error("Legacy reader is not isolated");
    const source = await directory(path.join(app.getPath("appData"), "openim-enterprise-upgrade")), partition = path.join(source, "Partitions", "part0");
    const exists = await fs.lstat(partition).then(() => true, error => { if (error.code === "ENOENT") return false; throw error; });
    stage = 'source-ready';
    const result = exists ? await readLegacyPartition({ writersStopped: true, onPhase: value => {stage = value;} }) : planLegacyImport(source, [], await inventoryLegacyCaches(source));
    result.sourceAliases = [...new Set([result.source, source])]; result.source = source;
    const packet = encodeLegacyReadoutPacket(result); stage = 'packet-ready';
    await new Promise<void>((resolve, reject) => { const output = createWriteStream("", { fd: 3, autoClose: true }); output.once("error", reject); output.once("close", resolve); output.end(packet); });
    stage = 'pipe-flushed'; clearTimeout(deadline); app.quit();
  } catch (error) {
    let code = 'UNKNOWN';
    if (diagnosticCodes.has((error as NodeJS.ErrnoException).code || '')) code = (error as NodeJS.ErrnoException).code!;
    else if ((error as Error).message?.startsWith('ERR_FAILED (-2) loading ')) code = 'LOAD_FAILED';
    clearTimeout(deadline); process.stderr.write('\nPUTY_LEGACY_FAILED:' + stage + ':' + code + '\n'); app.exit(1);
  }
}
