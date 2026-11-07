import test from "node:test";
import assert from "node:assert/strict";
import {build} from "esbuild";
import {createRequire} from "node:module";
import {mkdtemp,mkdir,writeFile,readFile,rm,readdir} from "node:fs/promises";
import {tmpdir} from "node:os";
import path from "node:path";
const requireSDK=createRequire(path.resolve("../../sdks/wasm/package.json"));
const SQL=await requireSDK("@jlongster/sql.js")({wasmBinary:await readFile("../../sdks/wasm/assets/sql-wasm.wasm")});
const compiled=await build({entryPoints:["electron/main/localStore/legacy.ts"],bundle:true,write:false,format:"esm",platform:"node",external:["electron"]});
const api=await import("data:text/javascript;base64,"+Buffer.from(compiled.outputFiles[0].text).toString("base64"));
const account="emp_"+"a".repeat(32),other="emp_"+"b".repeat(32),name=`-sql-${account}.sqlite`;
function fixture(){const db=new SQL.Database();db.exec("PRAGMA page_size=8192;CREATE TABLE synthetic(id INTEGER PRIMARY KEY,content TEXT);INSERT INTO synthetic VALUES(1,'SYNTHETIC legacy import');");const bytes=db.export();db.close();const pages=[{key:-1,value:{size:bytes.length}}];for(let offset=0;offset<bytes.length;offset+=8192)pages.push({key:offset/8192,value:bytes.slice(offset,offset+8192)});return {bytes,pages};}

test("fixed Absurd-SQL v2 pages reconstruct exact real SQLite bytes without opening the SDK",()=>{
 const {bytes,pages}=fixture(),before=pages.map(row=>row.key);const result=api.reconstructLegacySQLite(name,pages);
 assert.equal(result.accountID,account);assert.deepEqual(result.bytes,bytes);assert.equal(result.pageSize,8192);assert.equal(result.requiresIntegrityCheck,true);assert.deepEqual(pages.map(row=>row.key),before);
 const db=new SQL.Database(result.bytes);assert.deepEqual(db.exec("PRAGMA integrity_check")[0].values,[["ok"]]);assert.equal(db.exec("SELECT COUNT(*) FROM synthetic")[0].values[0][0],1);db.close();
});
test("truncated logical files ignore stale trailing pages instead of resurrecting deleted content",()=>{
 const {bytes,pages}=fixture();pages.push({key:bytes.length/8192,value:new Uint8Array(8192).fill(77)});
 const result=api.reconstructLegacySQLite(name,pages);assert.deepEqual(result.bytes,bytes);assert.equal(result.ignoredTrailingPages,1);assert.equal(pages.length,bytes.length/8192+2);
});
test("missing duplicate or malformed blocks and unbound names fail without deleting the raw evidence",()=>{
 const {pages}=fixture();
 for(const [database,blocks] of [[name,pages.filter(p=>p.key!==1)],[name,[...pages,pages[0]]],[name,[{key:-1,value:{size:1e15}},...pages.slice(1)]],["-sql-unknown.sqlite",pages],[name,[pages[0],{key:0,value:new Uint8Array(8192)}]]])assert.throws(()=>api.reconstructLegacySQLite(database,blocks));
 const raw=[{name:"unknown-format",version:1,stores:["unknown"]},{name,version:2,stores:["data"],pages:pages.filter(p=>p.key!==1)}];const plan=api.planLegacyImport("synthetic",raw,[]);assert.equal(plan.deletionAuthorized,false);assert.equal(plan.rawDatabases,raw);assert.equal(plan.unresolved.length,2);assert.equal(plan.databases.length,0);
});
test("all accounts remain separate and duplicate account databases are not silently chosen",()=>{
 const {pages}=fixture();const raw=[{name,version:2,stores:["data"],pages},{name:`-sql-${other}.sqlite`,version:2,stores:["data"],pages}];
 const plan=api.planLegacyImport("synthetic",raw,[]);assert.deepEqual(plan.databases.map(d=>d.accountID),[account,other]);assert.equal(plan.deletionAuthorized,false);
 const duplicate=api.planLegacyImport("synthetic",[raw[0],{...raw[0],name:`-sql-dir-${account}.sqlite`}],[]);assert.equal(duplicate.databases.length,0);assert.equal(duplicate.unresolved.length,1);
});
test("cache inventory preserves all account paths and only reads metadata",async()=>{
 const directory=await mkdtemp(path.join(tmpdir(),"puty-legacy-plan-"));
 try{
  for(const who of [account,other,"unassigned-old-user"]){const cache=path.join(directory,"OpenIMData",who,"fileCache");await mkdir(cache,{recursive:true});await writeFile(path.join(cache,"原文件.txt"),"synthetic");}
  const before=await readdir(directory,{recursive:true});const files=await api.inventoryLegacyCaches(directory);assert.equal(files.length,3);assert.equal(files.filter(f=>f.accountID).length,2);assert.ok(files.every(f=>f.size===9));assert.deepEqual(await readdir(directory,{recursive:true}),before);assert.ok(files.every(f=>!Object.hasOwn(f,"bytes")));
 }finally{await rm(directory,{recursive:true,force:true});}
});
test("reader source uses only readonly IDB and an isolated network-blocked hidden window",async()=>{
 const source=await readFile("electron/main/localStore/legacy.ts","utf8");
 assert.match(source,/db\.transaction\(\["data"\], "readonly"\)/);assert.doesNotMatch(source,/transaction\([^\n]*"readwrite"/);assert.doesNotMatch(source,/indexedDB\.deleteDatabase\(/);
 assert.match(source,/nodeIntegration: false/);assert.match(source,/sandbox: true/);assert.match(source,/contextIsolation: true/);assert.match(source,/Network\.setBypassServiceWorker/);assert.match(source,/cancel: details\.url !== page/);assert.doesNotMatch(source,/preload:/);
 assert.equal(typeof api.readLegacyPartition,"function"); // Never invoke a real partition in offline tests.
});
