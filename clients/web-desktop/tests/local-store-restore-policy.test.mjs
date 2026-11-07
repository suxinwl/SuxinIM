import test from 'node:test';
import assert from 'node:assert/strict';
import {build} from 'esbuild';
import {createRequire} from 'node:module';
import {randomUUID} from 'node:crypto';
import {mkdtemp,mkdir,readFile,writeFile,readdir,rm,unlink} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import path from 'node:path';

const requireSDK=createRequire(path.resolve('../../sdks/wasm/package.json'));
const SQL=await requireSDK('@jlongster/sql.js')({wasmBinary:await readFile('../../sdks/wasm/assets/sql-wasm.wasm')});
async function load(file){const built=await build({entryPoints:[file],bundle:true,write:false,platform:'node',format:'esm',external:['electron']});return import('data:text/javascript;base64,'+Buffer.from(built.outputFiles[0].text).toString('base64')+'#'+randomUUID());}
const {LocalVault}=await load('electron/main/localStore/vault.ts');
const {restoreBackup}=await load('electron/main/localStore/backup.ts');
const {preserveCleanupPolicy,readExistingRestorationPolicy}=await load('electron/main/localStore/restorePolicy.ts');
const crypto=await load('electron/main/localStore/crypto.ts');
const account='test_restore_policy',key=Buffer.alloc(32,53),passphrase='Synthetic restoration review passphrase',check=async()=>{};

async function fixture(t){
  const root=await mkdtemp(path.join(tmpdir(),'puty-restore-policy-')),source=path.join(root,'source'),restored=path.join(root,'restored');
  await mkdir(source);await mkdir(restored);const stores=[];
  t.after(async()=>{for(const store of stores)store.lock();assert.equal(path.dirname(root),path.resolve(tmpdir()));assert.ok(path.basename(root).startsWith('puty-restore-policy-'));await rm(root,{recursive:true,force:true});});
  return {root,source,restored,open:async(directory=source,master=key)=>{const store=await LocalVault.open(directory,account,master,check);stores.push(store);return store;}};
}

function sqliteSnapshot(rows){
  const db=new SQL.Database();
  db.exec('CREATE TABLE puty_runtime(singleton INTEGER PRIMARY KEY,suppress_archive INTEGER);INSERT INTO puty_runtime VALUES(1,0);CREATE TABLE puty_archive_outbox(id INTEGER);INSERT INTO puty_archive_outbox VALUES(1);CREATE TABLE local_conversations(conversation_id TEXT PRIMARY KEY,latest_msg TEXT,unread_count INTEGER);');
  for(const conv of ['si_a','si_b']){
    db.exec(`CREATE TABLE chat_logs_${conv}(client_msg_id TEXT PRIMARY KEY,send_time INTEGER,status INTEGER,content TEXT,local_ex TEXT,attached_info TEXT,ex TEXT);`);
    for(const row of rows.filter(value=>value.conversationID===conv))db.run(`INSERT INTO chat_logs_${conv} VALUES(?,?,?,?,?,?,?)`,[row.client_msg_id,row.send_time,row.status,row.content,'PRIVATE_LOCAL_PATH','PRIVATE_ATTACHED','PRIVATE_EXTRA']);
    const latest=rows.find(value=>value.conversationID===conv&&value.client_msg_id===(conv==='si_a'?'a-old':'b-new'));
    db.run('INSERT INTO local_conversations VALUES(?,?,?)',[conv,JSON.stringify({sendTime:latest.send_time,text:latest.content}),7]);
  }
  return db;
}

test('older complete backup keeps newer clear markers, clears real SQLite bodies/latest and survives reopens',async t=>{
  const f=await fixture(t),before=Date.parse('2026-11-04T01:00:00Z');
  const rows=[['si_a','a-old',before],['si_a','a-new',before+20000],['si_b','b-old',before],['si_b','b-new',before+20000]].map(([conversationID,client_msg_id,send_time])=>({conversationID,client_msg_id,send_time,status:2,content:'SYNTHETIC_BODY_'+client_msg_id}));
  const db=sqliteSnapshot(rows),oldSnapshot=db.export();let current=await f.open();
  await current.commit({opID:'older-state',statements:[],events:rows.map(({conversationID,...row})=>({kind:'upsert',conversationID,row}))});await current.checkpoint(oldSnapshot);
  const saved=await current.backup(f.root,passphrase,async()=>{});
  const rules=[{conversationIDs:['si_a'],beforeMS:before+1000},{conversationIDs:null,beforeMS:before+500}];
  for(const conv of ['si_a','si_b'])db.run(`UPDATE chat_logs_${conv} SET status=4,content='',local_ex='',attached_info='',ex='' WHERE send_time<?`,[before+500]);
  const cleaned=db.export();db.close();for(const rule of rules)await current.clean({conversationIDs:rule.conversationIDs||undefined,beforeMS:rule.beforeMS},cleaned);
  const newer=await current.restorationPolicy();assert.deepEqual(newer.ranges,rules);assert.equal(newer.suppressions.filter(v=>v.mode==='delete').length,2);
  const restoredKey=await restoreBackup(saved.directory,f.restored,account,passphrase,check);
  try{
    let restored=await f.open(f.restored,restoredKey);assert.equal((await restored.history('si_a')).length,2);
    await preserveCleanupPolicy(restored,newer.ranges,SQL,newer.suppressions);
    for(let attempt=0;attempt<3;attempt++){
      if(attempt){restored.lock();restored=await f.open(f.restored,restoredKey);}
      assert.deepEqual((await restored.history('si_a')).map(row=>row.client_msg_id),['a-new']);assert.deepEqual((await restored.history('si_b')).map(row=>row.client_msg_id),['b-new']);assert.deepEqual((await restored.restorationPolicy()).ranges,rules);assert.equal((await restored.restorationPolicy()).suppressions.filter(v=>v.mode==='delete').length,2);
      const replay=await restored.restoreSDK();assert.equal(replay.entries.length,0);const bytes=Buffer.from(replay.snapshot,'base64'),verified=new SQL.Database(bytes);
      try{
        for(const conv of ['si_a','si_b'])assert.deepEqual(verified.exec(`SELECT status,content,local_ex,attached_info,ex FROM chat_logs_${conv} WHERE client_msg_id LIKE '%-old'`)[0].values,[[4,'','','','']]);
        assert.deepEqual(verified.exec("SELECT latest_msg,unread_count FROM local_conversations WHERE conversation_id='si_a'")[0].values,[['',0]]);
        assert.equal(JSON.parse(verified.exec("SELECT latest_msg FROM local_conversations WHERE conversation_id='si_b'")[0].values[0][0]).text,'SYNTHETIC_BODY_b-new');
        assert.equal(verified.exec('SELECT COUNT(*) FROM puty_archive_outbox')[0].values[0][0],0);assert.equal(verified.exec('SELECT COUNT(*) FROM puty_local_tombstones')[0].values[0][0],2);assert.equal(verified.exec('SELECT suppress_archive FROM puty_runtime')[0].values[0][0],0);
      }finally{verified.close();}
      assert.equal(bytes.includes(Buffer.from('SYNTHETIC_BODY_a-old')),false);assert.equal(bytes.includes(Buffer.from('SYNTHETIC_BODY_b-old')),false);
    }
  }finally{restoredKey.fill(0);}
});

test('PC rejects authenticated native SQLCipher or missing-platform backups before copying any data',async t=>{
  const f=await fixture(t),store=await f.open();await store.checkpoint(Buffer.from('SYNTHETIC DESKTOP SNAPSHOT'));const saved=await store.backup(f.root,passphrase,async()=>{}),manifestPath=path.join(saved.directory,'manifest.puty'),context={account,type:'backup-manifest',objectID:'manifest'};
  const manifest=JSON.parse(crypto.unseal(key,context,await readFile(manifestPath)).toString());
  for(const platform of ['native-sqlcipher',undefined]){
    const changed={...manifest,platform};await writeFile(manifestPath,crypto.seal(key,context,Buffer.from(JSON.stringify(changed))));
    await assert.rejects(restoreBackup(saved.directory,f.restored,account,passphrase,check),/客户端存储类型|PC 完整备份/);assert.deepEqual(await readdir(f.restored),[]);
  }
});

test('pending release preserves other sends and shared references until each exact message completes',async t=>{
  const f=await fixture(t);let store=await f.open();const first=await store.stashFile('shared.txt','text/plain',Buffer.from('SYNTHETIC SHARED')),other=await store.stashFile('unrelated.txt','text/plain',Buffer.from('SYNTHETIC OTHER'));
  const id=first.path.split('/')[3],otherID=other.path.split('/')[3],when=Date.now();
  await store.commit({opID:'two-shared-references',statements:[],events:['first-message','second-message'].map(client_msg_id=>({kind:'upsert',conversationID:'si_a',row:{client_msg_id,send_time:when,status:2,content:JSON.stringify({filePath:first.path})}}))});
  await store.releasePending([id],'first-message');assert.equal((await store.readPending(id)).bytes.toString(),'SYNTHETIC SHARED');assert.equal(store.hasPending(otherID),true);
  await store.releasePending([id],'unrelated-message');assert.equal(store.hasPending(id),true);
  await store.checkpoint(Buffer.from('SYNTHETIC PENDING SNAPSHOT'));store.lock();store=await f.open();assert.equal(store.hasPending(id),true);assert.equal(store.hasPending(otherID),true);
  await store.releasePending([id],'second-message');assert.equal(store.hasPending(id),false);assert.equal((await store.readPending(otherID)).bytes.toString(),'SYNTHETIC OTHER');
  await store.checkpoint(Buffer.from('SYNTHETIC FINAL PENDING SNAPSHOT'));store.lock();store=await f.open();assert.equal(store.hasPending(id),false);assert.equal(store.hasPending(otherID),true);
});

test('single-message deletion survives an older backup and late sync without deleting other IDs',async t=>{
  const f=await fixture(t),when=Date.now(),current=await f.open(),db=new SQL.Database();
  db.exec('CREATE TABLE chat_logs_si_a(client_msg_id TEXT PRIMARY KEY,send_time INTEGER,status INTEGER,content TEXT,local_ex TEXT);CREATE TABLE local_conversations(conversation_id TEXT PRIMARY KEY,latest_msg TEXT,unread_count INTEGER);');
  for(const id of ['gone','keep'])db.run('INSERT INTO chat_logs_si_a VALUES(?,?,2,?,?)',[id,when,'SYNTHETIC_'+id,'PRIVATE_PATH']);
  db.run('INSERT INTO local_conversations VALUES(?,?,1)',['si_a',JSON.stringify({clientMsgID:'gone',sendTime:when})]);
  await current.commit({opID:'before-exact-delete',statements:[],events:['gone','keep'].map(id=>({kind:'upsert',conversationID:'si_a',row:{client_msg_id:id,send_time:when,status:2,content:'SYNTHETIC_'+id}}))});await current.checkpoint(db.export());
  const saved=await current.backup(f.root,passphrase,async()=>{});await current.commit({opID:'delete-exact',statements:[],events:[{kind:'delete',conversationID:'si_a',row:{client_msg_id:'gone',send_time:when,status:4}}]});
  const policy=await current.restorationPolicy();assert.equal(policy.ranges.length,0);assert.equal(policy.suppressions[0].mode,'delete');
  const master=await restoreBackup(saved.directory,f.restored,account,passphrase,check),restored=await f.open(f.restored,master);
  try{
    await preserveCleanupPolicy(restored,policy.ranges,SQL,policy.suppressions);
    await restored.commit({opID:'late-exact-sync',statements:[],events:[{kind:'upsert',conversationID:'si_a',row:{client_msg_id:'gone',send_time:when,status:2,content:'RESURRECTED'}}]});
    assert.deepEqual((await restored.history('si_a')).map(row=>row.client_msg_id),['keep']);
    const copy=new SQL.Database(Buffer.from((await restored.restoreSDK()).snapshot,'base64'));
    try{assert.deepEqual(copy.exec("SELECT status,content,local_ex FROM chat_logs_si_a WHERE client_msg_id='gone'")[0].values,[[4,'','']]);assert.deepEqual(copy.exec('SELECT * FROM puty_local_deleted')[0].values,[['si_a','gone']]);assert.equal(copy.exec('SELECT latest_msg FROM local_conversations')[0].values[0][0],'');}finally{copy.close();}
    restored.lock();const reopened=await f.open(f.restored,master);assert.deepEqual((await reopened.history('si_a')).map(row=>row.client_msg_id),['keep']);assert.equal((await reopened.restoreSDK()).suppression[0].mode,'delete');
  }finally{db.close();master.fill(0);}
});

test('failed OS unlock still preserves existing cleanup using recovered master; different unreadable data fails closed',async t=>{
  const f=await fixture(t),accountRoot=path.join(f.root,'account'),vaultRoot=path.join(accountRoot,'vault');await mkdir(accountRoot);await mkdir(vaultRoot);
  const current=await f.open(vaultRoot);await current.commit({opID:'existing-delete',statements:[],events:[{kind:'delete',conversationID:'si_a',row:{client_msg_id:'removed',send_time:Date.now()}}]});await current.checkpoint(Buffer.from('SYNTHETIC SDK'));current.lock();
  const failedUnwrap=async()=>{throw new Error('synthetic OS key changed');};await writeFile(path.join(accountRoot,'master.wrap'),'SYNTHETIC SEALED');
  const policy=await readExistingRestorationPolicy(accountRoot,account,key,failedUnwrap,check);assert.equal(policy.suppressions[0].clientMsgID,'removed');
  await assert.rejects(readExistingRestorationPolicy(accountRoot,account,Buffer.alloc(32,99),failedUnwrap,check),/现有档案无法校验/);
  const fromOS=await readExistingRestorationPolicy(accountRoot,account,Buffer.alloc(32,99),async()=>key,check);assert.equal(fromOS.suppressions[0].mode,'delete');assert.equal(key[0],53);
});

test('private archive suppression keeps active hot messages visible and never becomes a deletion marker',async t=>{
  const f=await fixture(t),current=await f.open(),when=Date.now(),row={client_msg_id:'private',send_time:when,status:2,content:'SYNTHETIC_PRIVATE'};
  await current.commit({opID:'private-classification',statements:[],events:[{kind:'upsert',conversationID:'si_a',row:{...row,__ephemeral:1}}]});
  assert.equal((await current.history('si_a')).length,0);assert.equal((await current.cleanupPolicy()).length,0);
  assert.equal((await current.sdkQuery('getMessage',['si_a','private'],[row]))[0].content,'SYNTHETIC_PRIVATE');
  await current.commit({opID:'private-final-delete',statements:[],events:[{kind:'delete',conversationID:'si_a',row}]});
  assert.equal((await current.sdkQuery('getMessage',['si_a','private'],[row])).length,0);
});

test('nonempty existing archive without a committed pointer or positive journal is never treated as empty',async t=>{
  const f=await fixture(t),base=path.join(f.root,'uncommitted'),vault=path.join(base,'vault');await mkdir(vault,{recursive:true});await writeFile(path.join(vault,'unexpected-ciphertext'),'SYNTHETIC PRESERVE ME');
  await assert.rejects(readExistingRestorationPolicy(base,account,key,async()=>key,check),/缺少有效提交记录/);assert.equal(await readFile(path.join(vault,'unexpected-ciphertext'),'utf8'),'SYNTHETIC PRESERVE ME');
});

test('lost committed pointer refuses normal open and preserves every encrypted month and checkpoint',async t=>{
  const f=await fixture(t),current=await f.open();await current.commit({opID:'durable-pointer-source',statements:[],events:[{kind:'upsert',conversationID:'si_a',row:{client_msg_id:'kept',send_time:Date.now(),status:2,content:'SYNTHETIC_KEEP_AFTER_POINTER_LOSS'}}]});await current.checkpoint(Buffer.from('SYNTHETIC SDK CHECKPOINT'));current.lock();
  await unlink(path.join(f.source,'state/current'));const files=[];async function walk(folder){for(const name of await readdir(folder)){const target=path.join(folder,name);try{files.push([target,await readFile(target)]);}catch(error){if(error.code==='EISDIR' || error.code==='EPERM')await walk(target);else throw error;}}}await walk(f.source);
  await assert.rejects(f.open(),/提交记录缺失/);for(const [file,bytes]of files)assert.deepEqual(await readFile(file),bytes);assert.ok(files.some(([file])=>file.includes('messages')));
});
