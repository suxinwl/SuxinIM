import test from 'node:test';
import assert from 'node:assert/strict';
import {build} from 'esbuild';
import {createRequire} from 'node:module';
import {mkdtemp,readFile,rm} from 'node:fs/promises';
import path from 'node:path';
import {tmpdir} from 'node:os';
const sdk=createRequire(path.resolve('../../sdks/wasm/package.json'));
const SQL=await sdk('@jlongster/sql.js')({wasmBinary:await readFile('../../sdks/wasm/assets/sql-wasm.wasm')});
async function load(file){const out=await build({entryPoints:[file],bundle:true,write:false,platform:'node',format:'esm'});return import('data:text/javascript;base64,'+Buffer.from(out.outputFiles[0].text).toString('base64')+'#'+crypto.randomUUID());}
test('actual SQLite worker keeps requested clock-skew selection while persisting only confirmation cutoff and exact deleted IDs',async()=>{
  const home=await mkdtemp(path.join(tmpdir(),'puty-worker-cutoff-')),master=Buffer.alloc(32,37),account='test_clock_cutoff',now=Date.now(),future=now+3600000,requested=now+86400000;
  const {LocalVault}=await load('electron/main/localStore/vault.ts');let vault=await LocalVault.open(home,account,master,async()=>{}),db;
  const seed=new SQL.Database();seed.exec("CREATE TABLE chat_logs_si_cut(client_msg_id TEXT PRIMARY KEY,send_time INTEGER,status INTEGER,content TEXT,local_ex TEXT,attached_info TEXT,ex TEXT,content_type INTEGER);CREATE TABLE local_conversations(conversation_id TEXT PRIMARY KEY,latest_msg TEXT,unread_count INTEGER,is_msg_destruct INTEGER DEFAULT 0)");
  seed.run('INSERT INTO chat_logs_si_cut VALUES(?,?,2,?,?,?,?,101)',['existing-future',future,'SYNTHETIC FUTURE','','','']);
  await vault.commit({opID:'archive-only-future',statements:[],events:[{kind:'upsert',conversationID:'si_cut',row:{client_msg_id:'archive-only-future',send_time:future,status:2,content:'SYNTHETIC ARCHIVE ONLY'}}]});
  await vault.checkpoint(seed.export());seed.close();
  const prior=globalThis.self,listeners=[];let committed;
  globalThis.self={addEventListener:(_,fn)=>listeners.push(fn),postMessage:request=>Promise.resolve().then(()=>{
    if(request.operation==='sdk-restore')return vault.restoreSDK();if(request.operation==='sdk-commit')return vault.commit(request.data);if(request.operation==='sdk-checkpoint')return vault.checkpoint(Buffer.from(request.data.snapshot,'base64'));
    if(request.operation==='cleanup-commit'){committed=request.data;return vault.clean(request.data.scope,Buffer.from(request.data.snapshot,'base64'),request.data.workingMessages);}throw Error('Unexpected synthetic bridge request');
  }).then(data=>listeners.forEach(fn=>fn({data:{putyStore:'response',id:request.id,data}})),error=>listeners.forEach(fn=>fn({data:{putyStore:'response',id:request.id,error:error.message}})))};
  const bridge=await load('../../sdks/wasm/src/api/database/durable.ts');listeners.forEach(fn=>fn({data:{putyStore:'configure',enabled:true}}));
  const init=bridge.durableMethod('initDB',async()=>{const saved=await bridge.restoreMemoryDatabase();db=bridge.attachMemoryDatabase(new SQL.Database(saved.bytes),saved.entries,saved.tombstones,saved.suppression);return {errCode:0};});
  const insert=bridge.durableMethod('insertMessage',async(id)=>{db.run('INSERT OR REPLACE INTO chat_logs_si_cut VALUES(?,?,2,?,?,?,?,101)',[id,future,'SYNTHETIC LIVE','','','']);return {errCode:0};});
  try{
    assert.equal((await init()).errCode,0);const began=Date.now(),result=await bridge.durableMethod('putyLocalCleanup',bridge.localCleanup)({conversationIDs:['si_cut'],beforeMS:requested});
    assert.equal(result.cleaned,true);assert.ok(result.maskBeforeMS>=began && result.maskBeforeMS<=Date.now()+1);assert.equal(committed.scope.beforeMS,requested);assert.equal(committed.scope.maskBeforeMS,result.maskBeforeMS);
    assert.equal(committed.workingMessages.find(row=>row.clientMsgID==='existing-future').sendTime,future);
    assert.equal(db.exec('SELECT before_ms FROM puty_local_tombstones')[0].values[0][0],result.maskBeforeMS);
    const saved=await vault.restoreSDK();assert.equal(saved.tombstones[0].beforeMS,result.maskBeforeMS);assert.equal((await vault.history('si_cut')).length,0);
    for(const id of ['existing-future','archive-only-future'])assert.equal(saved.suppression.find(row=>row.clientMsgID===id)?.mode,'delete');
    db.close();bridge.detachMemoryDatabase();vault.lock();vault=await LocalVault.open(home,account,master,async()=>{});assert.equal((await init()).errCode,0);
    for(const id of ['existing-future','archive-only-future']){assert.equal((await insert(id)).errCode,0);assert.equal(db.exec("SELECT status FROM chat_logs_si_cut WHERE client_msg_id='"+id+"'")[0].values[0][0],4);}
    assert.equal((await insert('new-after-confirmation')).errCode,0);assert.equal(db.exec("SELECT content,status FROM chat_logs_si_cut WHERE client_msg_id='new-after-confirmation'")[0].values[0][0],'SYNTHETIC LIVE');
    assert.equal((await vault.history('si_cut')).some(row=>row.client_msg_id==='new-after-confirmation'),true);
  }finally{try{db?.close();}catch{}bridge.detachMemoryDatabase();globalThis.self=prior;vault.lock();master.fill(0);await rm(home,{recursive:true,force:true});}
});
