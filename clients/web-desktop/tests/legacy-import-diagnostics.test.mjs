// Real SQL.js + production sealed legacy coordinator/vault, synthetic roots.
// No Electron/GUI, employee data, authorization service or network is used.
import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { build } from 'esbuild';
import { createRequire } from 'node:module';
import { createHash, randomBytes, randomUUID } from 'node:crypto';
import { mkdir, readFile, writeFile, readdir, lstat, realpath } from 'node:fs/promises';
import path from 'node:path';

const client=path.resolve(import.meta.dirname,'..'),suite=path.resolve(client,'../..'),nonce=randomUUID();
const work=path.resolve('D:/puty-build-temp','legacy-import-diagnostics-'+nonce);
const output=path.join(suite,'.private/production/acceptance/legacy-import-20261104','node-'+nonce);
await mkdir(work,{recursive:true});await mkdir(output,{recursive:true});
await writeFile(path.join(work,'.owned'),JSON.stringify({format:'PUTY-SYNTHETIC-LEGACY-DIAGNOSTICS-1',nonce}),{flag:'wx',mode:0o600});
assert.equal(await realpath(work),work);
const sha=bytes=>createHash('sha256').update(bytes).digest('hex');
const sourceFiles=['electron/main/localStore/legacyCoordinator.ts','electron/main/localStore/legacyFailure.ts','electron/main/localStore/vault.ts','electron/main/localStore/legacy.ts'];
const pins=await Promise.all(sourceFiles.map(async file=>({file,sha256:sha(await readFile(path.join(client,file)))})));
async function moduleAt(file){const compiled=await build({entryPoints:[path.join(client,file)],bundle:true,write:false,format:'esm',platform:'node',external:['electron']});return import('data:text/javascript;base64,'+Buffer.from(compiled.outputFiles[0].text).toString('base64'));}
const [api,legacy,vaultModule,crypto,diagnostics]=await Promise.all(['legacyCoordinator','legacy','vault','crypto','legacyFailure'].map(name=>moduleAt('electron/main/localStore/'+name+'.ts')));
const require=createRequire(path.join(client,'package.json'));
assert.equal(require('@jlongster/sql.js/package.json').version,'1.6.7');
const wasm=await readFile(path.join(client,'dist/sql-wasm.wasm'));
const SQL=await require('@jlongster/sql.js')({wasmBinary:wasm});
const account='emp_'+'c'.repeat(32),conversation='si_diagnostics_synthetic',osKey=randomBytes(32),vaultKey=randomBytes(32);
const check=async()=>{};
const protection={available:async()=>true,wrap:async value=>crypto.seal(osKey,{account:'synthetic-diagnostics',type:'os-wrap',objectID:'master'},Buffer.from(value)),unwrap:async value=>crypto.unseal(osKey,{account:'synthetic-diagnostics',type:'os-wrap',objectID:'master'},value).toString()};
const cases=[];
async function tree(directory){const result={};async function walk(current){for(const name of (await readdir(current)).sort()){const file=path.join(current,name),info=await lstat(file);assert.equal(info.isSymbolicLink(),false);if(info.isDirectory())await walk(file);else{assert.equal(info.isFile(),true);assert.equal(info.nlink,1);const bytes=await readFile(file);result[path.relative(directory,file).split(path.sep).join('/')]={bytes:bytes.length,sha256:sha(bytes)};}}}await walk(directory);return result;}
async function preserveEvidence(f){return {source:await tree(f.source),backup:await tree(path.join(f.root,'legacy'))};}
async function assertPreserved(f,before){assert.deepEqual(await tree(f.source),before.source,'Synthetic legacy source changed');assert.deepEqual(await tree(path.join(f.root,'legacy')),before.backup,'Sealed backup changed');}
function database(source,mode='valid'){
 const db=new SQL.Database();db.exec('PRAGMA page_size=8192;CREATE TABLE local_users(user_id TEXT);');db.run('INSERT INTO local_users VALUES(?)',[account]);
 const file=path.join(source,'OpenIMData',account,'fileCache','old-original.txt');
 if(mode==='missing-time'){db.exec('CREATE TABLE chat_logs_'+conversation+'(client_msg_id TEXT,status INTEGER,content_type INTEGER,content TEXT,local_ex TEXT);');db.run('INSERT INTO chat_logs_'+conversation+' VALUES(?,?,?,?,?)',['old-1',2,105,JSON.stringify({filePath:file,fileName:'旧源文件.txt'}),file]);}
 else{db.exec('CREATE TABLE chat_logs_'+conversation+'(client_msg_id TEXT,send_time INTEGER,status INTEGER,content_type INTEGER,content TEXT,local_ex TEXT);');db.run('INSERT INTO chat_logs_'+conversation+' VALUES(?,?,?,?,?,?)',[mode==='empty-id'?'':'old-1',1700000000000,2,105,JSON.stringify({filePath:file,fileName:'旧源文件.txt'}),file]);}
 assert.equal(db.exec('PRAGMA integrity_check')[0].values[0][0],'ok');const bytes=db.export();db.close();return bytes;
}
async function fixture(name,mode='valid'){
 const home=path.join(work,name),source=path.join(home,'old'),root=path.join(home,'encrypted');await mkdir(source,{recursive:true});await mkdir(root);
 const cache=path.join(source,'OpenIMData',account,'fileCache');await mkdir(cache,{recursive:true});await writeFile(path.join(cache,'old-original.txt'),'SYNTHETIC SEALED OLD CONTENT',{flag:'wx'});
 const bytes=database(source,mode),rawDir=path.join(source,'Partitions','part0','IndexedDB');await mkdir(rawDir,{recursive:true});await writeFile(path.join(rawDir,'synthetic-source.sqlite'),bytes,{flag:'wx'});
 const beforeSeal=await tree(source),pages=[{key:-1,value:{size:bytes.length}}];for(let n=0;n<bytes.length;n+=8192)pages.push({key:n/8192,value:bytes.slice(n,n+8192)});
 const readout=legacy.planLegacyImport(source,[{name:'-sql-'+account+'.sqlite',version:2,stores:['data'],pages}],await legacy.inventoryLegacyCaches(source));assert.equal(readout.databases.length,1);
 const receipt=await api.preserveLegacyData({root,source,readout,writersStopped:true,protection,SQL,assertCurrent:check});assert.equal(receipt.databases,1);assert.equal(receipt.verified,true);assert.deepEqual(await tree(source),beforeSeal);
 const vaultRoot=path.join(root,'accounts',account,'vault');return {home,source,root,vaultRoot,receipt,sourceSQLiteSHA256:sha(bytes),open:validator=>vaultModule.LocalVault.open(vaultRoot,account,vaultKey,validator||check)};
}
async function fail(f,vault,code,validator=check){let observed;try{await api.importLegacyAccount({root:f.root,snapshotID:f.receipt.snapshotID,accountID:account,vault,protection,SQL,assertCurrent:validator});}catch(error){observed=error;}assert.ok(observed,'Expected real import failure');const outer=diagnostics.legacyMigrationFailure('import',observed);assert.equal(outer.message,code);assert.match(outer.message,/^PUTY_LEGACY_import_[A-Z_]+$/);assert.equal(Object.hasOwn(outer,'cause'),false,'Outer IPC diagnostic must not retain raw cause');for(const privateValue of [f.source,f.root,account,'旧源文件.txt','SYNTHETIC'])assert.equal(outer.message.includes(privateValue),false);return {code:outer.message,step:observed.legacyImportStep,reason:observed.legacyImportReason};}
function suiteCase(name,run){test(name,async()=>{const proof={name,passed:false};try{await run(proof);proof.passed=true;}finally{cases.push(proof);await writeFile(path.join(output,name+'.json'),JSON.stringify(proof,null,2)+'\n',{flag:'wx',mode:0o600});}});}

suiteCase('checkpoint-existing-history-is-preserved',async proof=>{
 const f=await fixture('checkpoint');let vault=await f.open();
 try{
  const db=new SQL.Database();db.exec("CREATE TABLE existing_live(id TEXT PRIMARY KEY,body TEXT);INSERT INTO existing_live VALUES('current-1','SYNTHETIC CURRENT HISTORY');");const snapshot=db.export();db.close();
  await vault.commit({opID:'existing-current-'+nonce,statements:[],events:[{kind:'upsert',conversationID:'si_existing',row:{client_msg_id:'current-1',send_time:1700000001000,content:'SYNTHETIC CURRENT HISTORY',status:2}}],snapshot:Buffer.from(snapshot).toString('base64')});
  await vault.reserveAttachment({taskID:'existing-attachment',name:'当前附件.txt',messageRefs:[{conversationID:'si_existing',clientMsgID:'current-1'}]});await vault.writeAttachment('existing-attachment',(async function*(){yield Buffer.from('SYNTHETIC CURRENT ATTACHMENT');})());
  const before=await preserveEvidence(f),sdk=await vault.restoreSDK(),history=await vault.history('si_existing'),attachment=await vault.attachment('existing-attachment');const currentFiles=await tree(f.vaultRoot);proof.attempts=[];
  for(let n=0;n<2;n++){proof.attempts.push(await fail(f,vault,'PUTY_LEGACY_import_COMMIT_HISTORY_VAULT_NOT_EMPTY'));await assertPreserved(f,before);assert.deepEqual(await vault.restoreSDK().then(v=>v.snapshot),sdk.snapshot);assert.deepEqual(await vault.history('si_existing'),history);assert.equal((await vault.stats()).messages,1);assert.equal((await vault.readAttachment('existing-attachment')).toString(),'SYNTHETIC CURRENT ATTACHMENT');const now=await tree(f.vaultRoot);for(const [file,meta]of Object.entries(currentFiles))assert.deepEqual(now[file],meta,'Existing target ciphertext changed');vault.lock();vault=await f.open();}
  // JSON-persisted metadata drops an optional own property whose value is
  // undefined. Compare its durable JSON value; all original ciphertext bytes
  // were independently compared above on both actual reopen attempts.
  assert.deepEqual(JSON.parse(JSON.stringify(await vault.attachment('existing-attachment'))),JSON.parse(JSON.stringify(attachment)));proof.existingSnapshotSHA256=sha(Buffer.from(sdk.snapshot,'base64'));proof.existingMessageCount=1;proof.existingAttachmentCount=1;proof.currentCipherFilesPreserved=Object.keys(currentFiles).length;proof.sourceAndBackupPreserved=true;proof.realReopenRetries=2;
 }finally{vault.lock();}
});

suiteCase('sdk-journal-is-preserved',async proof=>{
 const f=await fixture('journal');let vault=await f.open();
 try{
  const statement={sql:'CREATE TABLE sdk_current(id TEXT);'};await vault.commit({opID:'sdk-journal-'+nonce,statements:[statement],events:[]});
  const journalPath=path.join(f.vaultRoot,'sdk/journal/0000000000000001'),journalHash=sha(await readFile(journalPath)),before=await preserveEvidence(f);proof.attempts=[];
  for(let n=0;n<2;n++){proof.attempts.push(await fail(f,vault,'PUTY_LEGACY_import_COMMIT_HISTORY_VAULT_NOT_EMPTY'));assert.equal(sha(await readFile(journalPath)),journalHash);const restored=await vault.restoreSDK();assert.equal(restored.snapshot,'');assert.deepEqual(restored.entries.find(row=>row.sequence===1).statements,[statement]);assert.equal((await vault.stats()).messages,0);await assertPreserved(f,before);vault.lock();vault=await f.open();}
  proof.originalSDKJournalSHA256=journalHash;proof.sourceAndBackupPreserved=true;proof.realReopenRetries=2;
 }finally{vault.lock();}
});

for(const [name,mode]of [['schema-missing-send-time','missing-time'],['schema-empty-message-id','empty-id']])suiteCase(name,async proof=>{
 const f=await fixture(name,mode);let vault=await f.open();
 try{
  await vault.preferences({syntheticExistingPreference:'retained'});const before=await preserveEvidence(f);proof.attempts=[];let firstFiles;
  for(let n=0;n<2;n++){proof.attempts.push(await fail(f,vault,'PUTY_LEGACY_import_PREPARE_MESSAGES_SCHEMA_UNSUPPORTED'));await assertPreserved(f,before);assert.equal((await vault.preferences()).syntheticExistingPreference,'retained');assert.equal((await vault.stats()).messages,0);const attachments=await tree(path.join(f.vaultRoot,'attachments'));if(firstFiles)assert.deepEqual(attachments,firstFiles,'Retry duplicated or replaced existing imported ciphertext');else firstFiles=attachments;vault.lock();vault=await f.open();}
  proof.sqliteIntegrityPassed=true;proof.sourceSQLiteSHA256=f.sourceSQLiteSHA256;proof.sourceAndBackupPreserved=true;proof.retainedEncryptedAttachmentFiles=Object.keys(firstFiles).length;proof.realReopenRetries=2;
 }finally{vault.lock();}
});

suiteCase('authorization-change-before-history-commit-is-retryable',async proof=>{
 const f=await fixture('authorization');let authorized=true;const validate=async()=>{if(!authorized)throw new Error('账号安全版本已改变，请重新登录');};let vault=await f.open(validate);
 try{
  await vault.preferences({syntheticExistingPreference:'retained'});const before=await preserveEvidence(f),commit=vault.importLegacy.bind(vault);
  // Authorization boundary fault only: production importLegacy still runs and
  // rejects from its real owner validator before its durable history write.
  vault.importLegacy=(...args)=>{authorized=false;return commit(...args);};proof.failure=await fail(f,vault,'PUTY_LEGACY_import_COMMIT_HISTORY_AUTHORIZATION_CHANGED',validate);vault.lock();authorized=true;vault=await f.open(validate);
  await assertPreserved(f,before);assert.equal((await vault.stats()).messages,0);assert.equal((await vault.preferences()).syntheticExistingPreference,'retained');const retainedAttachments=await tree(path.join(f.vaultRoot,'attachments'));
  const imported=await api.importLegacyAccount({root:f.root,snapshotID:f.receipt.snapshotID,accountID:account,vault,protection,SQL,assertCurrent:validate});assert.equal(imported.imported,true);assert.equal(imported.messages,1);assert.equal(imported.attachments,1);assert.equal((await vault.stats()).messages,1);assert.equal((await vault.stats()).attachments,1);assert.equal((await vault.preferences()).syntheticExistingPreference,'retained');assert.deepEqual(await tree(path.join(f.vaultRoot,'attachments')),retainedAttachments);await assertPreserved(f,before);
  vault.lock();vault=await f.open(validate);const again=await api.importLegacyAccount({root:f.root,snapshotID:f.receipt.snapshotID,accountID:account,vault,protection,SQL,assertCurrent:validate});assert.equal(again.alreadyImported,true);assert.equal((await vault.stats()).messages,1);assert.equal((await vault.stats()).attachments,1);await assertPreserved(f,before);
  proof.retry={imported:true,messages:1,attachments:1,secondReopenAlreadyImported:true,duplicateAttachmentFiles:0};proof.sourceAndBackupPreserved=true;proof.boundary='Synthetic authorization flag changes at COMMIT_HISTORY; no production account/network or actual SDK login.';
 }finally{authorized=true;vault.lock();}
});

after(async()=>{
 const unchanged=await Promise.all(pins.map(async pin=>({...pin,unchanged:sha(await readFile(path.join(client,pin.file)))===pin.sha256})));
 const report={format:'puty-legacy-import-diagnostics-node-1',passed:cases.length===5&&cases.every(row=>row.passed)&&unchanged.every(row=>row.unchanged),syntheticOnly:true,cases,sourcePins:unchanged,sqlVersion:'1.6.7',sqlWasmSHA256:sha(wasm),fixtures:work,retained:true,boundaries:['Real production legacy seal/readback/coordinator/vault encryption and real SQLite; synthetic RAM protector only','No GUI/Electron, SDK login, network, production authorization or employee files','Checkpoint/journal refusal and schema/lease failure are reproducible classes; actual employee-machine cause is not proven','Metadata preparing journals and encrypted attachment reservations may be added before refusal; preexisting contents and all sealed evidence remain unchanged','All fixture/evidence directories retained; no recursive deletion']};
 await writeFile(path.join(output,'result.json'),JSON.stringify(report,null,2)+'\n',{flag:'wx',mode:0o600});osKey.fill(0);vaultKey.fill(0);console.log(JSON.stringify({report:path.join(output,'result.json'),sha256:sha(await readFile(path.join(output,'result.json'))),passed:report.passed,cases:cases.length}));
});
