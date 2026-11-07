// Capture the actual installed producer boundary. SQL.js, SQLiteFS and File
// are real; only File's persistence operations are synthetic synchronous RAM.
import test,{after} from 'node:test';
import assert from 'node:assert/strict';
import {build} from 'esbuild';
import {createRequire} from 'node:module';
import {createHash,randomBytes,randomUUID} from 'node:crypto';
import {promises as fs} from 'node:fs';
import path from 'node:path';

const client=path.resolve(import.meta.dirname,'..'),suite=path.resolve(client,'../..'),local=path.join(client,'electron/main/localStore'),require=createRequire(path.join(client,'package.json'));
const account='emp_'+'c'.repeat(32),other='emp_'+'d'.repeat(32),conversation='si_actual_idb_layout',sha=bytes=>createHash('sha256').update(bytes).digest('hex');
async function load(file,append='',resolveDir=path.dirname(file)){
  const result=await build({stdin:{contents:await fs.readFile(file,'utf8')+append,resolveDir,sourcefile:file,loader:file.endsWith('.ts')?'ts':'js'},write:false,bundle:true,platform:'node',format:'esm',external:['electron']});
  return import('data:text/javascript;base64,'+Buffer.from(result.outputFiles[0].text).toString('base64')+'#'+randomUUID());
}
const [legacy,api,quarantine,crypto,{LocalVault}]=await Promise.all([
  load(process.env.PUTY_LEGACY_LAYOUT_BASELINE||path.join(local,'legacy.ts'),'',local),
  load(process.env.PUTY_LEGACY_LAYOUT_COORDINATOR_BASELINE||path.join(local,'legacyCoordinator.ts'),'',local),
  load(path.join(local,'legacyQuarantine.ts')),load(path.join(local,'crypto.ts')),load(path.join(local,'vault.ts')),
]);
const wasm=await fs.readFile(path.join(client,'dist/sql-wasm.wasm'));
assert.equal(require('@jlongster/sql.js/package.json').version,'1.6.7');
async function freshSQL(){const file=require.resolve('@jlongster/sql.js');delete require.cache[file];return require(file)({wasmBinary:wasm});}
const memorySQL=await freshSQL(),schemas=[];
for(const file of ['localUsers.ts','localConversations.ts','localChatLogsConversationID.ts']){
  const source=await fs.readFile(path.join(suite,'sdks/wasm/src/sqls',file),'utf8'),match=/return db\.exec\(\s*`([\s\S]*?)`\s*\);/.exec(source);assert.ok(match,'Actual SDK schema missing');schemas.push(match[1].replaceAll('${conversationID}',conversation));
}
const fixtures=new Map();
async function produce(packageName,prefix='',identities=[account]){
  const key=JSON.stringify([packageName,prefix,identities]);
  if(fixtures.has(key))return fixtures.get(key);
  const base=path.join(suite,'sdks/wasm/node_modules',packageName),[{SQLiteFS},backend,SQL]=await Promise.all([load(path.join(base,'dist/index.js')),load(path.join(base,'dist/indexeddb-backend.js'),'\nexport {File};\n'),freshSQL()]);
  const files=new Map(),names=[];
  const sqliteFS=new SQLiteFS(SQL.FS,{createFile(filename){
    const realFile=new backend.default().createFile(filename);names.push({filename,storeName:realFile.ops.getStoreName()});
    const blocks=new Map();let meta;
    const operations={open(){},close(){},delete(){blocks.clear();meta=undefined;},lock(){return true;},unlock(){return true;},readMeta(){if(!meta)return null;const first=new Uint8Array(blocks.get(0));return{size:meta.size,blockSize:(first[16]<<8)|first[17]};},writeMeta(value){meta={...value};},readBlocks(positions,blockSize){return positions.map(pos=>({pos,data:blocks.get(Math.round(pos/blockSize))||new ArrayBuffer(blockSize)}));},writeBlocks(writes,blockSize){for(const write of writes)blocks.set(Math.round(write.pos/blockSize),write.data.slice(0));}};
    const file=new backend.File(filename,operations);files.set(filename,{blocks,getMeta:()=>meta});return file;
  }});
  SQL.register_for_idb(sqliteFS);SQL.FS.mkdir('/sql');SQL.FS.mount(sqliteFS,{},'/sql');
  const logical='/sql/./'+prefix+account+'.sqlite',db=new SQL.Database(logical,{filename:true});db.exec('PRAGMA page_size=8192;PRAGMA journal_mode=MEMORY;');for(const sql of schemas)db.exec(sql);
  for(const identity of identities)db.run('INSERT INTO local_users(user_id) VALUES(?)',[identity]);
  db.run('INSERT INTO chat_logs_'+conversation+'(client_msg_id,send_time,status,content_type,content,send_id) VALUES(?,?,?,?,?,?)',['actual-producer',1700000000000,2,101,JSON.stringify({content:'SYNTHETIC ACTUAL PRODUCER'}),account]);
  assert.equal(db.exec('PRAGMA integrity_check')[0].values[0][0],'ok');const bytes=db.export();db.close();
  const own=names.filter(row=>row.filename.endsWith(account+'.sqlite'));assert.equal(own.length,1);assert.equal(own[0].filename,prefix+account+'.sqlite');assert.equal(own[0].storeName,own[0].filename);
  const file=files.get(own[0].filename),raw={name:own[0].storeName,version:2,stores:['data'],pages:[{key:-1,value:file.getMeta()},...[...file.blocks].map(([key,value])=>({key,value}))]};
  const result={packageName,logical,names,raw,bytes,sha256:sha(bytes)};fixtures.set(key,result);return result;
}
const verify=(bytes,id)=>api.verifyLegacySQLite(memorySQL,bytes,id,true);

for(const packageName of ['absurd-sql','absurd-sql-optimized'])for(const prefix of ['','openIM_v1.0_'])test(packageName+' actual producer '+(prefix||'bare account')+' name and page geometry reconstruct exactly',async()=>{
  const f=await produce(packageName,prefix),result=legacy.reconstructLegacySQLite(f.raw.name,f.raw.pages);assert.deepEqual(Buffer.from(result.bytes),Buffer.from(f.bytes));assert.equal(result.pageSize,8192);assert.equal(result.accountID,account);verify(result.bytes,account);
  const planned=legacy.planLegacyImport('SYNTHETIC', [f.raw], []);assert.equal(planned.databases.length,1);assert.equal(planned.unresolved.length,0);assert.equal(planned.databases[0].sha256,f.sha256);
});

test('legacy -sql- names and actual SDK names can deduplicate only the same complete bytes',async()=>{
  const f=await produce('absurd-sql-optimized','openIM_v1.0_'),raw={...f.raw,name:'-sql-previous-dir-'+account+'.sqlite'};
  const result=quarantine.resolveLegacyQuarantine([f.raw,raw],account,verify);assert.equal(result.sha256,f.sha256);assert.deepEqual(Buffer.from(result.bytes),Buffer.from(f.bytes));assert.equal(result.databaseNames.length,2);result.bytes.fill(0);
});

test('safe candidate names never bypass SQLite self-account checks',async()=>{
  for(const identities of [[other],[account,other],[]]){
    const f=await produce('absurd-sql-optimized','openIM_v1.0_',identities);
    assert.throws(()=>quarantine.resolveLegacyQuarantine([f.raw],account,verify),error=>error.legacyQuarantineReason==='QUARANTINE_IDENTITY');
  }
});

test('paths, control characters, trailing newline and inexact account suffixes remain invalid',async()=>{
  const f=await produce('absurd-sql');
  for(const name of ['../'+account+'.sqlite','/sql/'+account+'.sqlite','C:\\old\\'+account+'.sqlite','prefix\u0000'+account+'.sqlite','prefix\u007f'+account+'.sqlite',account+'.sqlite\n',account+'.sqlite.extra',account.toUpperCase()+'.sqlite'])assert.throws(()=>legacy.reconstructLegacySQLite(name,f.raw.pages),/identity is unrecognized/);
});

test('page content is not healed or rekeyed after accepting an actual basename',async()=>{
  const f=await produce('absurd-sql-optimized'),missing=f.raw.pages.filter(row=>row.key!==1),bytePositionKeys=f.raw.pages.map(row=>row.key<0?row:{...row,key:row.key*8192});
  assert.throws(()=>legacy.reconstructLegacySQLite(f.raw.name,missing),/page is missing/);assert.throws(()=>legacy.reconstructLegacySQLite(f.raw.name,bytePositionKeys),/page is missing/);
  assert.throws(()=>quarantine.resolveLegacyQuarantine([{...f.raw,pages:[{key:-1,value:{size:0}}]}],account,verify));
  const offset=f.raw.pages.map(row=>{if(row.key<0)return row;const value=new Uint8Array(row.value),buffer=new Uint8Array(value.length+17);buffer.set(value,7);return{key:row.key,value:buffer.subarray(7,7+value.length)};});assert.deepEqual(Buffer.from(legacy.reconstructLegacySQLite(f.raw.name,offset).bytes),Buffer.from(f.bytes));
});

const osKey=randomBytes(32),protection={available:async()=>true,wrap:async text=>crypto.seal(osKey,{account:'synthetic-layout',type:'os',objectID:'master'},Buffer.from(text)),unwrap:async bytes=>crypto.unseal(osKey,{account:'synthetic-layout',type:'os',objectID:'master'},bytes).toString()},check=async()=>{};
async function sealedFixture(){
  const home=await fs.realpath(await fs.mkdtemp('D:/puty-build-temp/actual-layout-')),root=path.join(home,'encrypted'),source=path.join(home,'old'),current=path.join(home,'current');await fs.writeFile(path.join(home,'.owned'),randomUUID(),{flag:'wx'});for(const folder of [root,source,current])await fs.mkdir(folder);await fs.writeFile(path.join(source,'synthetic-physical.bin'),'SYNTHETIC ORIGINAL PROFILE',{flag:'wx'});
  const f=await produce('absurd-sql-optimized','openIM_v1.0_'),readout={source,databases:[],rawDatabases:[f.raw],unresolved:[{databaseName:f.raw.name,reason:'Legacy database identity is unrecognized'}],cacheFiles:[],deletionAuthorized:false};
  const receipt=await api.preserveLegacyData({root,source,writersStopped:true,readout,SQL:memorySQL,protection,assertCurrent:check});assert.equal(receipt.databases,0);const cleaned=await api.cleanupLegacyPlaintext({root,source,currentUserData:current,writersStopped:true,snapshotID:receipt.snapshotID,SQL:memorySQL,protection,assertCurrent:check});assert.equal(cleaned.plaintextCleanupComplete,true);assert.deepEqual(await fs.readdir(source),[]);
  const snapshot=path.join(root,'legacy',receipt.snapshotID),original=new Map();for(const name of ['manifest.puty',...(await fs.readdir(path.join(snapshot,'objects'))).map(name=>'objects/'+name)])original.set(name,await fs.readFile(path.join(snapshot,name)));
  const key=randomBytes(32),vaultRoot=path.join(root,'accounts',account,'vault');return{root,source,snapshot,receipt,key,open:()=>LocalVault.open(vaultRoot,account,key,check),args:vault=>({root,snapshotID:receipt.snapshotID,accountID:account,vault,SQL:memorySQL,protection,assertCurrent:check}),async unchanged(){for(const[name,bytes]of original)assert.deepEqual(await fs.readFile(path.join(snapshot,name)),bytes);assert.deepEqual(await fs.readdir(source),[]);assert.equal((await api.listLegacySnapshots(root,protection)).length,1);}};
}

test('an already sealed safe-prefix quarantine is recovered, never silently ignored, and reopens idempotently',async()=>{
  const f=await sealedFixture();let vault=await f.open();try{
    const first=await api.importLegacyAccount(f.args(vault));assert.equal(first.imported,true);assert.equal(first.messages,1);assert.equal(first.attachments,0);assert.equal((await vault.history(conversation))[0].content,JSON.stringify({content:'SYNTHETIC ACTUAL PRODUCER'}));await f.unchanged();
    const proof=await fs.readFile(path.join(f.snapshot,'resolved',account,'proof.puty'));vault.lock();vault=await f.open();const reopened=await api.importLegacyAccount(f.args(vault));assert.equal(reopened.alreadyImported,true);assert.equal((await vault.stats()).messages,1);assert.deepEqual(await fs.readFile(path.join(f.snapshot,'resolved',account,'proof.puty')),proof);await f.unchanged();
  }finally{vault.lock();f.key.fill(0);}
});

test('an already used nonempty target is never overwritten or merged by safe-prefix recovery',async()=>{
  const f=await sealedFixture(),vault=await f.open();try{
    const db=new memorySQL.Database();db.exec('CREATE TABLE kept(id TEXT,body TEXT);');db.run('INSERT INTO kept VALUES(?,?)',['kept','SYNTHETIC CURRENT HISTORY']);const bytes=db.export();db.close();await vault.commit({opID:'kept-'+randomUUID(),statements:[],snapshot:Buffer.from(bytes).toString('base64'),events:[{kind:'upsert',conversationID:'si_current',row:{client_msg_id:'kept',send_time:1700000000000,status:2,content:'SYNTHETIC CURRENT HISTORY'}}]});
    const before=await vault.restoreSDK();await assert.rejects(api.importLegacyAccount(f.args(vault)),error=>error.legacyImportReason==='VAULT_NOT_EMPTY');assert.equal((await vault.restoreSDK()).snapshot,before.snapshot);assert.equal((await vault.history('si_current'))[0].content,'SYNTHETIC CURRENT HISTORY');assert.equal((await vault.stats()).messages,1);await f.unchanged();bytes.fill(0);
  }finally{vault.lock();f.key.fill(0);}
});

after(()=>{osKey.fill(0);for(const f of fixtures.values())f.bytes.fill(0);});
