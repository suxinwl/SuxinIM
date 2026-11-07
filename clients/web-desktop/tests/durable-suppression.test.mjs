import test from 'node:test';
import assert from 'node:assert/strict';
import {build} from 'esbuild';
import {createRequire} from 'node:module';
import {mkdtemp,readFile,rm,mkdir} from 'node:fs/promises';
import path from 'node:path';
import {tmpdir} from 'node:os';
const sdk=createRequire(path.resolve('../../sdks/wasm/package.json'));
const SQL=await sdk('@jlongster/sql.js')({wasmBinary:await readFile('../../sdks/wasm/assets/sql-wasm.wasm')});
async function load(file){const out=await build({entryPoints:[file],bundle:true,write:false,platform:'node',format:'esm'});return import('data:text/javascript;base64,'+Buffer.from(out.outputFiles[0].text).toString('base64')+'#'+crypto.randomUUID());}
const table='chat_logs_si_exact',time=1793740000000;
function schema(db){db.exec(`CREATE TABLE ${table}(client_msg_id TEXT PRIMARY KEY,send_time INTEGER,status INTEGER,content TEXT,local_ex TEXT,attached_info TEXT,ex TEXT,content_type INTEGER);CREATE TABLE local_conversations(conversation_id TEXT PRIMARY KEY,latest_msg TEXT,unread_count INTEGER,is_msg_destruct INTEGER DEFAULT 0)`);}
function insert(db,id,body='SYNTHETIC',attached='',conversation=table){db.run(`INSERT OR REPLACE INTO ${conversation} VALUES(?,?,?,?,?,?,?,?)`,[id,time,2,body,'local',attached,'extra',101]);}
function row(db,id){return db.exec(`SELECT content,status,local_ex,attached_info,ex FROM ${table} WHERE client_msg_id='${id}'`)[0]?.values[0];}
async function bridgeFor(post){const prior=globalThis.self,listeners=[];globalThis.self={addEventListener:(_,fn)=>listeners.push(fn),postMessage:request=>Promise.resolve().then(()=>post(request)).then(data=>listeners.forEach(fn=>fn({data:{putyStore:'response',id:request.id,data}})),error=>listeners.forEach(fn=>fn({data:{putyStore:'response',id:request.id,error:error.message}})))};const bridge=await load('../../sdks/wasm/src/api/database/durable.ts');listeners.forEach(fn=>fn({data:{putyStore:'configure',enabled:true}}));return {bridge,close(){bridge.detachMemoryDatabase();globalThis.self=prior;}};}

test('old snapshot triggers are upgraded and exact delete cannot widen to its neighbours or archive-only private hot rows',async()=>{
  const fixture=await bridgeFor(()=>{throw new Error('This SQLite trigger test needs no host mutation');});let db;
  try{
    const old=new SQL.Database();schema(old);old.exec('CREATE TABLE chat_logs_si_other AS SELECT * FROM chat_logs_si_exact WHERE 0');
    insert(old,'deleted');insert(old,'neighbour');insert(old,'private','PRIVATE LIVE',JSON.stringify({isPrivateChat:true}));insert(old,'deleted','OTHER CONVERSATION','', 'chat_logs_si_other');insert(old,'legacy-deleted');old.exec(`UPDATE ${table} SET status=4,content='' WHERE client_msg_id='legacy-deleted'`);
    old.run('INSERT INTO local_conversations VALUES(?,?,?,0)',['si_exact',JSON.stringify({clientMsgID:'deleted',sendTime:time,content:'OLD LATEST'}),1]);
    old.exec(`CREATE TRIGGER puty_insert_${table} AFTER INSERT ON ${table} BEGIN SELECT 1;END;CREATE TRIGGER puty_conversation_update AFTER UPDATE ON local_conversations BEGIN SELECT 1;END`);
    const bytes=old.export();old.close();
    db=fixture.bridge.attachMemoryDatabase(new SQL.Database(bytes),[],[{conversationIDs:['si_exact'],beforeMS:Number.MAX_SAFE_INTEGER,messageIDs:['deleted']}],[{conversationID:'si_exact',clientMsgID:'deleted',sendTime:time,mode:'delete'},{conversationID:'si_exact',clientMsgID:'private',sendTime:time,mode:'archive'}]);
    assert.deepEqual(row(db,'deleted'),['',4,'','','']);assert.equal(row(db,'neighbour')[0],'SYNTHETIC');assert.equal(row(db,'private')[0],'PRIVATE LIVE');
    assert.equal(db.exec('SELECT count(*) FROM puty_local_tombstones')[0].values[0][0],0);assert.equal(db.exec("SELECT count(*) FROM puty_local_deleted WHERE client_msg_id='private'")[0].values[0][0],0);
    assert.equal(db.exec("SELECT content FROM chat_logs_si_other WHERE client_msg_id='deleted'")[0].values[0][0],'OTHER CONVERSATION');assert.deepEqual(db.exec('SELECT latest_msg,unread_count FROM local_conversations')[0].values[0],['',0]);
    assert.match(db.exec(`SELECT sql FROM sqlite_master WHERE name='puty_insert_${table}'`)[0].values[0][0],/puty_local_deleted/);
    insert(db,'deleted','LATE RESYNC');assert.deepEqual(row(db,'deleted'),['',4,'','','']);insert(db,'new-neighbour','NEW LIVE');assert.equal(row(db,'new-neighbour')[0],'NEW LIVE');
    insert(db,'legacy-deleted','LEGACY DELETE RESYNC');assert.deepEqual(row(db,'legacy-deleted'),['',4,'','','']);
    db.run('UPDATE local_conversations SET latest_msg=?,unread_count=1',[JSON.stringify({clientMsgID:'deleted',sendTime:time,content:'LATE LATEST'})]);assert.deepEqual(db.exec('SELECT latest_msg,unread_count FROM local_conversations')[0].values[0],['',0]);
    db.exec(`DELETE FROM ${table} WHERE client_msg_id='new-neighbour'`);insert(db,'new-neighbour','PHYSICAL DELETE RESYNC');assert.equal(row(db,'new-neighbour')[1],4);
  }finally{db?.close();fixture.close();}
});

test('actual worker commits recall as delete while private classification stays archive-only across reopen/resync',async()=>{
  const home=await mkdtemp(path.join(tmpdir(),'puty-exact-suppression-')),directory=path.join(home,'current'),master=Buffer.alloc(32,29),{LocalVault}=await load('electron/main/localStore/vault.ts');let vault=await LocalVault.open(directory,'test_exact_suppression',master,async()=>{}),db,restoredVault;
  const priorDB=new SQL.Database();schema(priorDB);insert(priorDB,'legacy-deleted','OLD BACKUP BODY');const priorVault=await LocalVault.open(path.join(home,'prior'),'test_exact_suppression',master,async()=>{});await priorVault.commit({opID:'old-backup-row',statements:[],events:[{kind:'upsert',conversationID:'si_exact',row:{client_msg_id:'legacy-deleted',send_time:time,status:2,content:'OLD BACKUP BODY'}}]});await priorVault.checkpoint(priorDB.export());priorDB.close();await mkdir(path.join(home,'export'));const oldBackup=await priorVault.backup(path.join(home,'export'),'Synthetic exact suppression backup passphrase',async()=>{});priorVault.lock();
  const legacy=new SQL.Database();schema(legacy);insert(legacy,'legacy-deleted');legacy.exec(`UPDATE ${table} SET status=4,content='' WHERE client_msg_id='legacy-deleted'`);await vault.checkpoint(legacy.export());legacy.close();
  const fixture=await bridgeFor(request=>{
    if(request.operation==='sdk-restore')return vault.restoreSDK();
    if(request.operation==='sdk-commit')return vault.commit(request.data);
    if(request.operation==='sdk-checkpoint')return vault.checkpoint(Buffer.from(request.data.snapshot,'base64'));
    if(request.operation==='archive-query')return vault.sdkQuery(request.data.name,request.data.args,request.data.workingRows);
    throw new Error('Unexpected bridge operation');
  });const {bridge}=fixture;
  const init=bridge.durableMethod('initDB',async()=>{const r=await bridge.restoreMemoryDatabase();db=bridge.attachMemoryDatabase(new SQL.Database(r.bytes),r.entries,r.tombstones,r.suppression);if(!db.exec(`SELECT name FROM sqlite_master WHERE name='${table}'`).length)schema(db);return {errCode:0};});
  const write=bridge.durableMethod('insertMessage',async(id,privateChat=false)=>{insert(db,id,'SYNTHETIC '+id,privateChat?JSON.stringify({isPrivateChat:true}):'');return {errCode:0};});
  try{
    assert.equal((await init()).errCode,0);assert.equal((await vault.restoreSDK()).suppression.find(item=>item.clientMsgID==='legacy-deleted')?.mode,'delete');assert.equal((await write('legacy-deleted')).errCode,0);assert.equal(row(db,'legacy-deleted')[1],4);assert.equal((await write('recall')).errCode,0);assert.equal((await write('keep')).errCode,0);assert.equal((await write('private',true)).errCode,0);
    const {restoreBackup}=await load('electron/main/localStore/backup.ts'),{preserveCleanupPolicy}=await load('electron/main/localStore/restorePolicy.ts'),target=path.join(home,'restored');await mkdir(target);const restoredKey=await restoreBackup(oldBackup.directory,target,'test_exact_suppression','Synthetic exact suppression backup passphrase',async()=>{});restoredVault=await LocalVault.open(target,'test_exact_suppression',restoredKey,async()=>{});restoredKey.fill(0);const policy=await vault.restorationPolicy();await preserveCleanupPolicy(restoredVault,policy.ranges,SQL,policy.suppressions);assert.equal((await restoredVault.history('si_exact')).length,0);const restoredSQL=new SQL.Database(Buffer.from((await restoredVault.restoreSDK()).snapshot,'base64'));assert.equal(row(restoredSQL,'legacy-deleted')[1],4);restoredSQL.close();restoredVault.lock();
    db.run('INSERT INTO local_conversations VALUES(?,?,?,0)',['si_exact',JSON.stringify({clientMsgID:'recall',sendTime:time,content:'OLD LATEST'}),1]);
    const recall=bridge.durableMethod('updateMessage',async()=>{db.run(`UPDATE ${table} SET content_type=2101,content='RECALLED' WHERE client_msg_id=?`,['recall']);return {errCode:0};});assert.equal((await recall()).errCode,0);assert.deepEqual(db.exec('SELECT latest_msg,unread_count FROM local_conversations')[0].values[0],['',0]);
    const saved=await vault.restoreSDK(),deleted=saved.suppression.find(item=>item.clientMsgID==='recall'),privateRule=saved.suppression.find(item=>item.clientMsgID==='private');assert.equal(deleted?.mode,'delete');assert.equal(privateRule?.mode,'archive');assert.equal(row(db,'private')[0],'SYNTHETIC private');
    await vault.checkpoint(db.export());db.close();bridge.detachMemoryDatabase();vault.lock();vault=await LocalVault.open(directory,'test_exact_suppression',master,async()=>{});
    assert.equal((await init()).errCode,0);assert.equal((await write('recall')).errCode,0);assert.deepEqual(row(db,'recall'),['',4,'','','']);assert.equal((await vault.history('si_exact')).some(item=>item.client_msg_id==='recall'),false);assert.equal(row(db,'keep')[0],'SYNTHETIC keep');assert.equal(row(db,'private')[0],'SYNTHETIC private');
    assert.equal(db.exec("SELECT count(*) FROM puty_local_deleted WHERE client_msg_id='private'")[0].values[0][0],0);
  }finally{try{db?.close();}catch{}fixture.close();restoredVault?.lock();vault.lock();await rm(home,{recursive:true,force:true});}
});
