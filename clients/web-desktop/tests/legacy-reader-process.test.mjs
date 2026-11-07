import test from "node:test";
import assert from "node:assert/strict";
import { build } from "esbuild";
import { createRequire } from "node:module";
import { randomUUID } from "node:crypto";
import { mkdtemp, mkdir, writeFile, readFile, rm, realpath } from "node:fs/promises";
import path from "node:path";
import { tmpdir } from "node:os";
import { spawn } from "node:child_process";
const compile = await build({ entryPoints: ["electron/main/localStore/legacyReaderProcess.ts"], bundle: true, write: false, format: "esm", platform: "node", external: ["electron"] }), api = await import("data:text/javascript;base64," + Buffer.from(compile.outputFiles[0].text).toString("base64"));
const empty = source => ({ source, rawDatabases: [{ name: "unknown", version: 1, stores: ["opaque"] }], cacheFiles: [], databases: [], unresolved: [], deletionAuthorized: false });
test('child diagnostics expose fixed stage and reason enums and never raw local names',()=>{
 assert.equal(api.legacyReaderFailureCategory('untrusted raw diagnostic\nPUTY_LEGACY_FAILED:reader-created:LOAD_FAILED\n'),'reader-created/LOAD_FAILED');
 assert.equal(api.legacyReaderFailureCategory('\nPUTY_LEGACY_FAILED:page-loaded:LEGACY_RENDERER_OOM\n'),'page-loaded/LEGACY_RENDERER_OOM');
 for(const value of ['raw old account/path', '\nPUTY_LEGACY_FAILED:private-account:UNKNOWN\n','\nPUTY_LEGACY_FAILED:reader-created:RAW_CREDENTIAL_VALUE\n'])assert.equal(api.legacyReaderFailureCategory(value),undefined);
});
test("binary framing rejects corruption, trailing bytes, wrong source and oversized length", () => {
 const source = path.resolve("synthetic-legacy"), packet = api.encodeLegacyReadoutPacket(empty(source)); assert.equal(api.decodeLegacyReadoutPacket(packet, source).unresolved.length, 1);
 const corrupt = Buffer.from(packet); corrupt[corrupt.length - 1] ^= 1; assert.throws(() => api.decodeLegacyReadoutPacket(corrupt, source)); assert.throws(() => api.decodeLegacyReadoutPacket(Buffer.concat([packet, Buffer.from([0])]), source)); assert.throws(() => api.decodeLegacyReadoutPacket(packet, source + "-wrong"));
 const large = Buffer.from(packet); large.writeBigUInt64BE(2n ** 40n, 8); assert.throws(() => api.decodeLegacyReadoutPacket(large, source), /bound/);
 const unsafe = empty(source); unsafe.cacheFiles = [{ relativePath: "../escape", size: 1, mtimeMs: 1 }]; assert.throws(() => api.decodeLegacyReadoutPacket(api.encodeLegacyReadoutPacket(unsafe), source), /descriptor/);
});
test("child runtime flag is bound to an existing exact owned isolated profile before ready", async () => {
 const temporary = await mkdtemp(path.join(tmpdir(), "puty-reader-flags-")); try { const id = randomUUID(), runtime = path.join(temporary, "legacy", ".reader-runtime-" + id); await mkdir(runtime, { recursive: true }); await writeFile(path.join(runtime, ".puty-reader-owned"), id); const argv = ["exe", "--puty-legacy-reader", "--puty-legacy-reader-runtime=" + runtime]; assert.equal(api.legacyReaderRuntimeFromArgs(argv), await realpath(runtime)); assert.throws(() => api.legacyReaderRuntimeFromArgs([...argv, "--puty-legacy-reader"])); await writeFile(path.join(runtime, ".puty-reader-owned"), "wrong"); assert.throws(() => api.legacyReaderRuntimeFromArgs(argv), /ownership/); } finally { await rm(temporary, { recursive: true, force: true }); }
});
async function electronRun(executable, args) {
 const env = { ...process.env }; delete env.ELECTRON_RUN_AS_NODE;
 return new Promise((resolve, reject) => { const child = spawn(executable, args, { env, windowsHide: true, stdio: ["ignore", "pipe", "pipe"] }); let output = "", errors = ""; const timer = setTimeout(() => child.kill(), 50000); child.stdout.on("data", bytes => output += bytes); child.stderr.on("data", bytes => errors += bytes); child.on("error", error => { clearTimeout(timer); reject(error); }); child.on("close", code => { clearTimeout(timer); resolve({ code, output, errors }); }); });
}
test("real same-exe reader exits before parent seals and removes the complete synthetic old tree", async t => {
 const temporary = await mkdtemp(path.join(tmpdir(), "puty-reader-pipe-"));
 try {
  const reader = path.join(temporary, "reader.cjs"), coordinator = path.join(temporary, "coordinator.cjs");
  await build({ entryPoints: ["electron/main/localStore/legacyReaderProcess.ts"], outfile: reader, bundle: true, format: "cjs", platform: "node", external: ["electron"] }); await build({ entryPoints: ["electron/main/localStore/legacyCoordinator.ts"], outfile: coordinator, bundle: true, format: "cjs", platform: "node", external: ["electron"] });
  const requireSDK = createRequire(path.resolve("../../sdks/wasm/package.json")), wasmPath = path.resolve("../../sdks/wasm/assets/sql-wasm.wasm"), sqlModule = path.resolve("../../sdks/wasm/node_modules/@jlongster/sql.js"), SQL = await requireSDK("@jlongster/sql.js")({ wasmBinary: await readFile(wasmPath) });
  const db = new SQL.Database(); db.exec("PRAGMA page_size=8192;CREATE TABLE synthetic(id INTEGER PRIMARY KEY,content TEXT);INSERT INTO synthetic VALUES(1,'SYNTHETIC PIPE ONLY');"); const bytes = db.export(); db.close(); const pages = []; for (let n = 0; n < bytes.length; n += 8192) pages.push(Array.from(bytes.slice(n, n + 8192)));
  const fixture = path.join(temporary, "appdata"), entry = path.join(temporary, "index.cjs"); await writeFile(path.join(temporary, "package.json"), JSON.stringify({ name: "puty-synthetic-reader", version: "1.0.0", main: "index.cjs" }));
  await writeFile(entry, `
const {app,protocol,session,BrowserWindow}=require('electron'),fs=require('node:fs'),fsp=fs.promises,path=require('node:path');const reader=require(${JSON.stringify(reader)});const fixture=${JSON.stringify(fixture)};fs.mkdirSync(fixture,{recursive:true});app.setName('PUTY Synthetic Pipe');app.setPath('appData',fixture);app.on('window-all-closed',()=>{});process.on('uncaughtException',()=>{process.stderr.write('SYNTHETIC_FAILURE\\n');app.exit(3);});protocol.registerSchemesAsPrivileged([{scheme:'openim38',privileges:{standard:true,secure:true,supportFetchAPI:true,corsEnabled:true}}]);
if(process.argv.includes('--puty-legacy-reader')){app.setPath('userData',reader.legacyReaderRuntimeFromArgs());void reader.runLegacyReaderMode();}
else{const profile=path.join(fixture,'stable-os-profile');fs.mkdirSync(profile,{recursive:true});app.setPath('userData',profile);app.whenReady().then(async()=>{
 const source=path.join(fixture,'openim-enterprise-upgrade'),partition=path.join(source,'Partitions','part0');
 if(process.argv.includes('--fixture-populate')){
  await fsp.mkdir(partition,{recursive:true});const s=session.fromPath(await fsp.realpath(partition),{cache:false});s.protocol.handle('openim38',()=>new Response('<!doctype html><title>synthetic</title>',{headers:{'Content-Type':'text/html'}}));const w=new BrowserWindow({show:false,webPreferences:{session:s,sandbox:true,nodeIntegration:false,contextIsolation:true}});await w.loadURL('openim38://app/fixture');
  const populate=async(pages,size)=>{for(const account of ['a','b']){const name='-sql-emp_'+account.repeat(32)+'.sqlite';const db=await new Promise((resolve,reject)=>{const q=indexedDB.open(name,2);q.onupgradeneeded=()=>q.result.createObjectStore('data');q.onsuccess=()=>resolve(q.result);q.onerror=()=>reject(q.error);});await new Promise((resolve,reject)=>{const tx=db.transaction('data','readwrite'),store=tx.objectStore('data');store.put({size},-1);pages.forEach((p,i)=>store.put(new Uint8Array(p).buffer,i));tx.oncomplete=resolve;tx.onerror=()=>reject(tx.error);});db.close();}return true;};await w.webContents.executeJavaScript('('+populate.toString()+')('+${JSON.stringify(JSON.stringify(pages))}+','+${bytes.length}+')');w.destroy();s.protocol.unhandle('openim38');app.quit();return;
 }
 const root=path.join(fixture,'encrypted');await fsp.mkdir(root);const sourceCanonical=await fsp.realpath(source);const result=await reader.readLegacyInChild({root,source:sourceCanonical,writersStopped:true,assertCurrent:async()=>{}});if(result.databases.length!==2||!(result.databases[0].bytes instanceof Uint8Array))throw new Error('binary readout mismatch');
 const remainingProfiles=(await fsp.readdir(path.join(root,'legacy'))).filter(name=>name.startsWith('.reader-runtime-'));if(remainingProfiles.length)throw new Error('child profile leak');
 const {preserveLegacyData,cleanupLegacyPlaintext}=require(${JSON.stringify(coordinator)}),SQL=await require(${JSON.stringify(sqlModule)})({wasmBinary:await fsp.readFile(${JSON.stringify(wasmPath)})});const options={root,source:sourceCanonical,writersStopped:true,assertCurrent:async()=>{},SQL};const saved=await preserveLegacyData({...options,readout:result});const cleaned=await cleanupLegacyPlaintext({...options,snapshotID:saved.snapshotID,currentUserData:app.getPath('userData')});if(!cleaned.plaintextCleanupComplete||(await fsp.readdir(source)).length)throw new Error('closed tree cleanup mismatch');console.log(JSON.stringify({passed:true,binaryDatabases:2,readerExitedBeforeBackup:true,childProfileRemoved:true,sourceFilesRemaining:0}));app.quit();
}).catch(error=>{process.stderr.write('SYNTHETIC_PARENT_FAILED '+error.message+'\\n');app.exit(1);});}setTimeout(()=>app.exit(2),45000).unref();
`);
  const executable = path.resolve("node_modules/electron/dist", process.platform === "win32" ? "electron.exe" : "electron"); const populate = await electronRun(executable, [temporary, "--fixture-populate"]); assert.equal(populate.code, 0, populate.errors.slice(-1000));
  const result = await electronRun(executable, [temporary, "--fixture-parent"]); assert.equal(result.code, 0, result.errors.slice(-1000)); const summary = result.output.split(/\r?\n/).map(line => { try { return JSON.parse(line); } catch { return null; } }).find(value => value?.passed); assert.equal(summary?.sourceFilesRemaining, 0); assert.equal(summary?.readerExitedBeforeBackup, true); t.diagnostic(JSON.stringify(summary));
 } finally { await rm(temporary, { recursive: true, force: true }); }
});
