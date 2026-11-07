// A real Electron/IndexedDB test using a disposable appData root only.
import test from "node:test";
import assert from "node:assert/strict";
import { build } from "esbuild";
import { createRequire } from "node:module";
import { mkdtemp, readFile, writeFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import { spawn } from "node:child_process";
const requireSDK = createRequire(path.resolve("../../sdks/wasm/package.json"));
const SQL = await requireSDK("@jlongster/sql.js")({ wasmBinary: await readFile("../../sdks/wasm/assets/sql-wasm.wasm") });
async function runElectron(executable, runner, environment) {
 return new Promise((resolve, reject) => { const child = spawn(executable, [runner], { env: environment, windowsHide: true, stdio: ["ignore", "pipe", "pipe"] }); let output = "", errors = ""; const timeout = setTimeout(() => { child.kill(); }, 50000); child.stdout.on("data", value => output += value); child.stderr.on("data", value => errors += value); child.on("error", error => { clearTimeout(timeout); reject(error); }); child.on("exit", code => { clearTimeout(timeout); resolve({ code, output, errors }); }); });
}
test("real hidden Electron reader preserves all synthetic IDB pages and isolates unknown schemas", async (t) => {
 const temporary = await mkdtemp(path.join(tmpdir(), "puty-legacy-electron-"));
 try {
  const compiled = path.join(temporary, "reader.cjs"), coordinator = path.join(temporary, "coordinator.cjs"); await build({ entryPoints: ["electron/main/localStore/legacy.ts"], bundle: true, outfile: compiled, format: "cjs", platform: "node", external: ["electron"] }); await build({ entryPoints: ["electron/main/localStore/legacyCoordinator.ts"], bundle: true, outfile: coordinator, format: "cjs", platform: "node", external: ["electron"] });
  const db = new SQL.Database(); db.exec("PRAGMA page_size=8192;CREATE TABLE synthetic(id INTEGER PRIMARY KEY,content TEXT);INSERT INTO synthetic VALUES(1,'SYNTHETIC ONLY');"); const bytes = db.export(); db.close();
  const names = ["-sql-emp_" + "a".repeat(32) + ".sqlite", "-sql-emp_" + "b".repeat(32) + ".sqlite"], pageArrays = []; for (let n = 0; n < bytes.length; n += 8192) pageArrays.push(Array.from(bytes.slice(n, n + 8192)));
  const runner = path.join(temporary, "runner.cjs"), fixtureRoot = path.join(temporary, "appdata");
  const code = `
const {app,BrowserWindow,session,protocol}=require('electron');
process.on('uncaughtException',()=>{console.error('Synthetic legacy Electron setup failed');app.exit(3);});
const fs=require('node:fs/promises'),path=require('node:path');
const {readLegacyPartition}=require(${JSON.stringify(compiled)});
const {preserveLegacyData}=require(${JSON.stringify(coordinator)});
const directory=${JSON.stringify(fixtureRoot)},names=${JSON.stringify(names)},pages=${JSON.stringify(pageArrays)},size=${bytes.length};let stage='setup';
protocol.registerSchemesAsPrivileged([{scheme:'openim38',privileges:{standard:true,secure:true,supportFetchAPI:true,corsEnabled:true}}]);
app.disableHardwareAcceleration(); app.setName('PUTY Synthetic Legacy Test');
app.on('window-all-closed',()=>{});
const current=path.join(directory,'current');require('node:fs').mkdirSync(current,{recursive:true});app.setPath('appData',directory);app.setPath('userData',current);
(async()=>{
 await app.whenReady();stage='ready'; const source=path.join(directory,'openim-enterprise-upgrade'),partition=path.join(source,'Partitions','part0');await fs.mkdir(partition,{recursive:true});
 const target=session.fromPath(await fs.realpath(partition),{cache:false}),url='openim38://app/__fixture__';target.protocol.handle('openim38',()=>new Response('<!doctype html><title>synthetic</title>',{headers:{'Content-Type':'text/html'}}));
 const hidden=new BrowserWindow({show:false,webPreferences:{session:target,sandbox:true,contextIsolation:true,nodeIntegration:false}});hidden.webContents.setWindowOpenHandler(()=>({action:'deny'}));await hidden.loadURL(url);
 const populate=async(names,pages,size)=>{for(const name of names){const db=await new Promise((resolve,reject)=>{const q=indexedDB.open(name,2);q.onupgradeneeded=()=>q.result.createObjectStore('data');q.onsuccess=()=>resolve(q.result);q.onerror=()=>reject(q.error);});await new Promise((resolve,reject)=>{const tx=db.transaction('data','readwrite'),store=tx.objectStore('data');store.put({size},-1);pages.forEach((page,i)=>store.put(new Uint8Array(page).buffer,i));tx.oncomplete=resolve;tx.onerror=()=>reject(tx.error);});db.close();}const db=await new Promise((resolve,reject)=>{const q=indexedDB.open('unknown-synthetic-schema',9);q.onupgradeneeded=()=>q.result.createObjectStore('opaque');q.onsuccess=()=>resolve(q.result);q.onerror=()=>reject(q.error);});db.close();return true;};
 stage='populate';await hidden.webContents.executeJavaScript('('+populate.toString()+')('+JSON.stringify(names)+','+JSON.stringify(pages)+','+size+')');hidden.destroy();target.protocol.unhandle('openim38');
 stage='read1';const first=await readLegacyPartition({writersStopped:true,onPhase:value=>{stage='read1-'+value;}});stage='read2';const second=await readLegacyPartition({writersStopped:true,onPhase:value=>{stage='read2-'+value;}});
 if(first.databases.length!==2||first.unresolved.length!==1||first.rawDatabases.length!==3||second.databases.length!==2||first.databases.some((row,i)=>row.sha256!==second.databases[i].sha256||!(row.bytes instanceof Uint8Array)||row.bytes.length!==size))throw new Error('synthetic readonly mismatch');
 stage='backup';const SQL=await require(${JSON.stringify(path.resolve("../../sdks/wasm/node_modules/@jlongster/sql.js"))})({wasmBinary:await fs.readFile(${JSON.stringify(path.resolve("../../sdks/wasm/assets/sql-wasm.wasm"))})});const root=path.join(directory,'encrypted');await fs.mkdir(root);const options={root,source:first.source,readout:first,writersStopped:true,SQL,assertCurrent:async()=>{}};const receipt=await preserveLegacyData(options);
 stage='lock-check';let sameProcessCleanup=false;const lock=(await fs.readdir(partition,{recursive:true})).find(name=>path.basename(name)==='LOCK');if(!lock)throw new Error('synthetic LevelDB lock missing');try{await fs.unlink(path.join(partition,lock));sameProcessCleanup=true;}catch(error){if(!['EPERM','EBUSY','EACCES'].includes(error.code))throw error;}
 console.log(JSON.stringify({passed:true,snapshotID:receipt.snapshotID,databases:first.databases.length,quarantined:first.unresolved.length,typedPages:true,unchanged:true,backupVerified:receipt.verified,sameProcessCleanup}));app.quit();
})().catch((error)=>{console.error('Synthetic legacy Electron check failed stage='+stage+' class='+error.name+' message='+error.message);app.exit(1);});
setTimeout(()=>{console.error('Synthetic legacy Electron timeout stage='+stage);app.exit(2);},45000).unref();
`;
  await writeFile(runner, code); const environment = { ...process.env }; delete environment.ELECTRON_RUN_AS_NODE;
  const executable = path.resolve("node_modules/electron/dist", process.platform === "win32" ? "electron.exe" : "electron");
  const result = await runElectron(executable, runner, environment);
  assert.equal(result.code, 0, result.errors.slice(-2000)); const summary = result.output.split(/\r?\n/).map(line => { try { return JSON.parse(line); } catch { return null; } }).find(row => row?.passed); assert.equal(summary?.unchanged, true); assert.equal(summary?.typedPages, true); assert.equal(summary?.databases, 2); assert.equal(summary?.quarantined, 1); assert.equal(summary?.backupVerified, true); t.diagnostic(JSON.stringify(summary));
  const resumedRunner = path.join(temporary, "cleanup.cjs");
  await writeFile(resumedRunner, `const {app}=require('electron');const fs=require('node:fs/promises'),path=require('node:path');const {finalizeLegacySnapshot,cleanupLegacyPlaintext,listLegacySnapshots}=require(${JSON.stringify(coordinator)});process.on('uncaughtException',()=>app.exit(3));app.setName('PUTY Synthetic Legacy Test');app.setPath('appData',${JSON.stringify(fixtureRoot)});app.setPath('userData',path.join(${JSON.stringify(fixtureRoot)},'current'));app.on('window-all-closed',()=>{});app.whenReady().then(async()=>{const root=path.join(${JSON.stringify(fixtureRoot)},'encrypted'),source=path.join(${JSON.stringify(fixtureRoot)},'openim-enterprise-upgrade');const SQL=await require(${JSON.stringify(path.resolve("../../sdks/wasm/node_modules/@jlongster/sql.js"))})({wasmBinary:await fs.readFile(${JSON.stringify(path.resolve("../../sdks/wasm/assets/sql-wasm.wasm"))})});const options={root,source,currentUserData:app.getPath('userData'),writersStopped:true,assertCurrent:async()=>{},SQL};const finalized=await finalizeLegacySnapshot({...options,snapshotID:${JSON.stringify(summary.snapshotID)},readerProcessExited:true});if(finalized.parentSnapshotID!==${JSON.stringify(summary.snapshotID)})throw new Error('lineage mismatch');const result=await cleanupLegacyPlaintext({...options,snapshotID:finalized.snapshotID});const snapshots=await listLegacySnapshots(root);if(!result.plaintextCleanupComplete||(await fs.readdir(source)).length!==0||!snapshots.find(row=>row.snapshotID===finalized.snapshotID)?.plaintextCleanupComplete||snapshots.length!==2)throw new Error('resumed cleanup mismatch');console.log(JSON.stringify({passed:true,secondProcessCleanup:true,closedTreeResealed:true,parentRetained:true,sourceFilesRemaining:0}));app.exit(0);}).catch(error=>{console.error('Synthetic resumed cleanup failed '+error.message);app.exit(1);});setTimeout(()=>app.exit(2),45000).unref();`);
  const resumed = await runElectron(executable, resumedRunner, environment); assert.equal(resumed.code, 0, resumed.errors.slice(-2000)); const cleanupSummary = resumed.output.split(/\r?\n/).map(line => { try { return JSON.parse(line); } catch { return null; } }).find(row => row?.secondProcessCleanup); assert.equal(cleanupSummary?.sourceFilesRemaining, 0); t.diagnostic(JSON.stringify(cleanupSummary));
 } finally { await rm(temporary, { recursive: true, force: true }); }
});
