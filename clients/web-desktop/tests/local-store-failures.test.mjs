import test from 'node:test';
import assert from 'node:assert/strict';
import {build} from 'esbuild';
import {randomUUID} from 'node:crypto';
import {promises as fs} from 'node:fs';
import {tmpdir} from 'node:os';
import path from 'node:path';

async function load(entry){const output=await build({entryPoints:[entry],bundle:true,write:false,platform:'node',format:'esm'});return import('data:text/javascript;base64,'+Buffer.from(output.outputFiles[0].text).toString('base64')+'#'+randomUUID());}
const {LocalVault,atomicWrite}=await load('electron/main/localStore/vault.ts'),crypto=await load('electron/main/localStore/crypto.ts');
const names=await load('electron/main/localStore/names.ts');
const account='test_enospc_review',key=Buffer.alloc(32,61),when=Date.parse('2026-11-04T01:00:00Z'),check=async()=>{};
const event=(id,conversationID='si_selected',content='SYNTHETIC PRIVATE BODY')=>({kind:'upsert',conversationID,row:{client_msg_id:id,send_time:when,status:2,content}});
async function fixture(t){const directory=await fs.realpath(await fs.mkdtemp(path.join(tmpdir(),'puty-disk-failure-'))),stores=[];t.after(async()=>{for(const store of stores)store.lock();assert.equal(path.dirname(directory),await fs.realpath(tmpdir()));assert.ok(path.basename(directory).startsWith('puty-disk-failure-'));await fs.rm(directory,{recursive:true,force:true});});return {directory,open:async()=>{const store=await LocalVault.open(directory,account,key,check);stores.push(store);return store;}};}
async function files(root){const result=[];for(const item of await fs.readdir(root,{withFileTypes:true}).catch(error=>{if(error.code==='ENOENT')return [];throw error;})){const name=path.join(root,item.name);if(item.isDirectory())result.push(...await files(name));else if(item.isFile())result.push(name);else throw Error('Synthetic directory contains link or special file');}return result;}
function inject(t,predicate,stage='writeFile',code='ENOSPC'){
  const original=fs.open;
  fs.open=async(value,...args)=>{const handle=await original(value,...args);if(!predicate(String(value)))return handle;return new Proxy(handle,{get(target,name){if(name===stage)return async bytes=>{if(stage==='writeFile')await target.writeFile(Buffer.from(bytes).subarray(0,19));throw Object.assign(Error('synthetic disk fault'),{code});};const result=Reflect.get(target,name,target);return typeof result==='function'?result.bind(target):result;}});};
  const release=()=>{fs.open=original;};t.after(release);return release;
}

test('atomic target survives partial ENOSPC and fsync EIO, each write rejects and removes its own temporary',async t=>{
  for(const stage of ['writeFile','sync'])await t.test(stage,async t=>{const f=await fixture(t),target=path.join(f.directory,'existing.puty'),old=crypto.seal(key,{account,type:'synthetic',objectID:'existing'},Buffer.from('ORIGINAL ENCRYPTED BODY'));await fs.writeFile(target,old);inject(t,value=>value.startsWith(target+'.next-'),stage,stage==='sync'?'EIO':'ENOSPC');await assert.rejects(atomicWrite(target,Buffer.alloc(4096,37)),{code:stage==='sync'?'EIO':'ENOSPC'});assert.deepEqual(await fs.readFile(target),old);assert.deepEqual(await fs.readdir(f.directory),['existing.puty']);});
});

test('failed filename fsync closes its handle and releases only the newly allocated empty name',async t=>{
  const f=await fixture(t),target=path.join(f.directory,'职员名单.xlsx');
  const release=inject(t,value=>value===target,'sync','EIO');
  await assert.rejects(names.reserveName(f.directory,'职员名单.xlsx'),{code:'EIO'});
  await assert.rejects(fs.access(target),{code:'ENOENT'});release();
  assert.equal((await names.reserveName(f.directory,'职员名单.xlsx')).name,'职员名单.xlsx');
});

test('unjournaled reservation ENOSPC leaves existing attachments intact and retry retains the first free original name',async t=>{
  const f=await fixture(t);let store=await f.open();
  const directory=path.join(f.directory,'attachments',names.shanghaiMonth());await fs.mkdir(directory,{recursive:true});
  const original=path.join(directory,'职员名单.xlsx');await fs.writeFile(original,Buffer.from('SYNTHETIC EXISTING FILE'));
  const release=inject(t,value=>value.includes(path.join('sdk','journal'))&&value.includes('.next-'));
  const input={taskID:'synthetic-name-retry',name:'职员名单.xlsx',messageRefs:[{conversationID:'si_selected',clientMsgID:'name-retry'}]};
  await assert.rejects(store.reserveAttachment(input),{code:'ENOSPC'});
  assert.deepEqual(await fs.readdir(directory),['职员名单.xlsx']);assert.equal((await fs.readFile(original)).toString(),'SYNTHETIC EXISTING FILE');
  release();store.lock();store=await f.open();const reserved=await store.reserveAttachment(input);assert.equal(reserved.name,'职员名单（1）.xlsx');
  store.lock();store=await f.open();assert.equal((await store.reserveAttachment(input)).name,'职员名单（1）.xlsx');
});

test('temporary unlink refusal never changes the existing target and leaves only bounded encrypted evidence',async t=>{
  const f=await fixture(t),target=path.join(f.directory,'existing.puty'),old=Buffer.from('SYNTHETIC OLD TARGET'),encrypted=crypto.seal(key,{account,type:'synthetic',objectID:'next'},Buffer.from('SYNTHETIC NEW PRIVATE BODY'));await fs.writeFile(target,old);const release=inject(t,value=>value.startsWith(target+'.next-')),unlink=fs.unlink;
  fs.unlink=async value=>{if(String(value).startsWith(target+'.next-'))throw Object.assign(Error('synthetic cleanup refusal'),{code:'EACCES'});return unlink(value);};t.after(()=>fs.unlink=unlink);
  await assert.rejects(atomicWrite(target,encrypted),{code:'EACCES'});assert.deepEqual(await fs.readFile(target),old);const remaining=(await files(f.directory)).filter(value=>value!==target);assert.equal(remaining.length,1);const partial=await fs.readFile(remaining[0]);assert.equal(partial.length,19);assert.equal(partial.subarray(0,8).toString(),'PUTYLS1\n');assert.equal(partial.includes(Buffer.from('SYNTHETIC NEW PRIVATE BODY')),false);release();
});

test('journal ENOSPC rejects before ACK, restart retains previous data and retry commits exactly once',async t=>{
  const f=await fixture(t);let store=await f.open();await store.commit({opID:'old',statements:[],events:[event('old')]});await store.checkpoint(Buffer.from('SYNTHETIC OLD SNAPSHOT'));
  const release=inject(t,value=>value.includes(path.join('sdk','journal'))&&value.includes('.next-'));
  await assert.rejects(store.commit({opID:'retry-stable',statements:[],events:[event('new')]}),{code:'ENOSPC'});assert.equal((await store.history('si_selected')).length,1);assert.equal((await files(f.directory)).some(value=>value.includes('.next-')),false);
  release();store.lock();store=await f.open();assert.deepEqual((await store.history('si_selected')).map(row=>row.client_msg_id),['old']);
  const request={opID:'retry-stable',statements:[],events:[event('new')]};assert.equal((await store.commit(request)).committed,true);assert.equal((await store.commit(request)).committed,true);store.lock();store=await f.open();assert.equal((await store.history('si_selected')).length,2);
});

test('materialization ENOSPC after durable journal is deferred, fails closed while full and replays after recovery',async t=>{
  const f=await fixture(t);let store=await f.open();await store.checkpoint(Buffer.from('SYNTHETIC EMPTY SNAPSHOT'));
  // Match platform-native separators without assuming the current month name.
  const releaseMessage=inject(t,value=>value.includes(path.sep+'messages'+path.sep)&&value.includes('.next-'));
  const result=await store.commit({opID:'durable-body',statements:[{sql:'SELECT 1'}],events:[event('recoverable')]});assert.equal(result.committed,true);assert.equal(result.archiveDeferred,true);
  store.lock();await assert.rejects(f.open(),{code:'ENOSPC'});releaseMessage();store=await f.open();assert.equal((await store.history('si_selected'))[0].client_msg_id,'recoverable');assert.equal((await store.restoreSDK()).entries.length,1);await store.checkpoint(Buffer.from('SYNTHETIC RECOVERED SNAPSHOT'));store.lock();store=await f.open();assert.equal((await store.history('si_selected')).length,1);
});

test('pointer ENOSPC never acknowledges a new snapshot and restart selects the original committed generation',async t=>{
  const f=await fixture(t);let store=await f.open();await store.commit({opID:'original',statements:[],events:[event('original')]});await store.checkpoint(Buffer.from('ORIGINAL SNAPSHOT'));const pointer=await fs.readFile(path.join(f.directory,'state','current'));
  const release=inject(t,value=>value.startsWith(path.join(f.directory,'state','current')+'.next-'));
  await assert.rejects(store.commit({opID:'not-committed',statements:[],snapshot:Buffer.from('NEW SNAPSHOT').toString('base64'),events:[event('not-committed')]}),{code:'ENOSPC'});assert.deepEqual(await fs.readFile(path.join(f.directory,'state','current')),pointer);release();store.lock();store=await f.open();assert.deepEqual((await store.history('si_selected')).map(row=>row.client_msg_id),['original']);assert.equal(Buffer.from((await store.restoreSDK()).snapshot,'base64').toString(),'ORIGINAL SNAPSHOT');assert.equal((await files(f.directory)).some(value=>value.includes('.next-')),false);
});

test('committed cleanup reports deferred payload unlink and retries after restart without reviving tombstones',async t=>{
  const f=await fixture(t);let store=await f.open();await store.commit({opID:'both',statements:[],events:[event('selected'),event('kept','si_other')]});await store.checkpoint(Buffer.from('SYNTHETIC BEFORE CLEAN'));
  const selected=[];for(const file of await files(path.join(f.directory,'messages'))){const bytes=await fs.readFile(file),header=crypto.parseHeader(bytes).header;if(JSON.parse(crypto.unseal(key,header,bytes)).client_msg_id==='selected')selected.push(file);}assert.equal(selected.length,1);
  const unlink=fs.unlink;fs.unlink=async value=>{if(selected.includes(String(value)))throw Object.assign(Error('synthetic file busy'),{code:'EACCES'});return unlink(value);};t.after(()=>fs.unlink=unlink);
  const result=await store.clean({conversationIDs:['si_selected'],beforeMS:when+1},Buffer.from('SYNTHETIC CLEAN SNAPSHOT'));assert.equal(result.cleaned,true);assert.equal(result.payloadCleanupPending,true);assert.equal((await store.history('si_selected')).length,0);assert.equal((await store.history('si_other')).length,1);await fs.access(selected[0]);
  fs.unlink=unlink;store.lock();store=await f.open();await assert.rejects(fs.access(selected[0]),{code:'ENOENT'});assert.equal((await store.history('si_selected')).length,0);assert.equal((await store.cleanupPolicy()).length,1);assert.equal((await store.stats()).payloadCleanupPending,false);
});

test('cleanup preview counts actual encrypted message, ready/partial attachment and referenced pending bytes only',async t=>{
  const f=await fixture(t),store=await f.open(),pending=await store.stashFile('pending.bin','application/octet-stream',Buffer.alloc(83,52)),other=await store.stashFile('other.bin','application/octet-stream',Buffer.alloc(113,55));
  await store.commit({opID:'references',statements:[],events:[event('selected','si_selected',JSON.stringify({filePath:pending.path})),event('kept','si_other',JSON.stringify({filePath:other.path}))]});
  const refs=[{conversationID:'si_selected',clientMsgID:'selected'}],ready=await store.reserveAttachment({taskID:'ready',name:'ready.bin',messageRefs:refs});await store.writeAttachment('ready',(async function*(){yield Buffer.alloc(71,56);})());
  const partial=await store.reserveAttachment({taskID:'partial',name:'partial.bin',messageRefs:refs});await assert.rejects(store.writeAttachment('partial',(async function*(){yield Buffer.alloc(crypto.CHUNK_SIZE,57);throw Error('synthetic interrupted stream');})()),/synthetic interrupted/);
  const shared=await store.reserveAttachment({taskID:'shared',name:'shared.bin',messageRefs:[...refs,{conversationID:'si_other',clientMsgID:'kept'}]});await store.writeAttachment('shared',(async function*(){yield Buffer.alloc(95,58);})());
  await store.checkpoint(Buffer.from('SYNTHETIC PREVIEW SNAPSHOT'));
  const selectedFiles=[path.join(f.directory,ready.file),path.join(f.directory,partial.file),path.join(f.directory,partial.file+'.part-'+partial.objectID),path.join(f.directory,'pending',pending.path.split('/')[3],pending.name)];
  for(const file of await files(path.join(f.directory,'messages'))){const bytes=await fs.readFile(file),header=crypto.parseHeader(bytes).header;if(JSON.parse(crypto.unseal(key,header,bytes)).client_msg_id==='selected')selectedFiles.push(file);}
  const expected=(await Promise.all(selectedFiles.map(async file=>(await fs.stat(file)).size))).reduce((sum,size)=>sum+size,0),preview=await store.previewCleanup({conversationIDs:['si_selected'],beforeMS:when+1});assert.equal(preview.messages,1);assert.equal(preview.attachments,3);assert.equal(preview.bytes,expected);assert.ok(preview.bytes>crypto.CHUNK_SIZE);
  await store.clean({conversationIDs:['si_selected'],beforeMS:when+1},Buffer.from('SYNTHETIC AFTER PREVIEW'));for(const file of selectedFiles)await assert.rejects(fs.access(file),{code:'ENOENT'});assert.equal((await store.readAttachment('shared')).length,95);assert.equal((await store.readPending(other.path.split('/')[3])).bytes.length,113);
});

test('failed compact pointer leaves managed message ciphertext that the next stable reopen reclaims without losing prior history',async t=>{
  const f=await fixture(t);let store=await f.open();await store.commit({opID:'baseline',statements:[],events:[event('baseline')]});await store.checkpoint(Buffer.from('BASELINE SNAPSHOT'));
  const baselineFiles=await files(path.join(f.directory,'messages')),release=inject(t,value=>value.startsWith(path.join(f.directory,'state','current')+'.next-'));
  await assert.rejects(store.commit({opID:'compact-orphan',statements:[],snapshot:Buffer.from('UNCOMMITTED SNAPSHOT').toString('base64'),events:[event('orphan-private-body')]}),{code:'ENOSPC'});release();
  const materialized=await files(path.join(f.directory,'messages')),orphans=materialized.filter(value=>!baselineFiles.includes(value));assert.equal(orphans.length,1);
  const unmanaged=path.join(path.dirname(orphans[0]),'do-not-own.bin');await fs.writeFile(unmanaged,Buffer.from('SYNTHETIC UNMANAGED'));
  store.lock();store=await f.open();await assert.rejects(fs.access(orphans[0]),{code:'ENOENT'});await fs.access(baselineFiles[0]);assert.equal((await fs.readFile(unmanaged)).toString(),'SYNTHETIC UNMANAGED');assert.deepEqual((await store.history('si_selected')).map(row=>row.client_msg_id),['baseline']);assert.equal((await store.stats()).payloadCleanupPending,false);
});

test('uncheckpointed patch replay postpones the managed orphan sweep across two reopens until a new durable checkpoint',async t=>{
  const f=await fixture(t);let store=await f.open();await store.commit({opID:'baseline',statements:[],events:[event('original')]});await store.checkpoint(Buffer.from('BASELINE SNAPSHOT'));
  const original=(await files(path.join(f.directory,'messages')))[0],orphan=path.join(path.dirname(original),'a'.repeat(64)+'-0000000000000099');await fs.writeFile(orphan,crypto.seal(key,{account,type:'message',objectID:'synthetic-orphan'},Buffer.from('UNREFERENCED SYNTHETIC')));
  await store.commit({opID:'patch',statements:[],events:[{kind:'upsert',conversationID:'si_selected',row:{client_msg_id:'original',send_time:when,__patch:1,status:3}}]});
  for(let n=0;n<2;n++){store.lock();store=await f.open();await fs.access(orphan);await fs.access(original);const row=(await store.history('si_selected'))[0];assert.equal(row.content,'SYNTHETIC PRIVATE BODY');assert.equal(row.status,3);}
  await store.checkpoint(Buffer.from('PATCH CHECKPOINT'));await assert.rejects(fs.access(orphan),{code:'ENOENT'});assert.equal((await store.history('si_selected'))[0].status,3);
});

test('managed message directory link is refused and an external synthetic target is never traversed or removed',async t=>{
  const f=await fixture(t),other=await fixture(t);let store=await f.open();await store.commit({opID:'baseline',statements:[],events:[event('original')]});await store.checkpoint(Buffer.from('BASELINE SNAPSHOT'));store.lock();
  const marker=path.join(other.directory,'b'.repeat(64)+'-0000000000000001'),month=path.join(f.directory,'messages','202612'),link=path.join(month,'a'.repeat(64));await fs.mkdir(month,{recursive:true});await fs.writeFile(marker,Buffer.from('SYNTHETIC OUTSIDE TARGET'));
  try{await fs.symlink(other.directory,link,process.platform==='win32'?'junction':'dir');}catch(error){if(error.code==='EPERM'){t.skip('This operator cannot create the synthetic directory link');return;}throw error;}
  t.after(async()=>{if((await fs.lstat(link).catch(()=>null))?.isSymbolicLink())await fs.unlink(link);});
  store=await f.open();assert.equal((await store.stats()).payloadCleanupPending,true);assert.equal((await fs.readFile(marker)).toString(),'SYNTHETIC OUTSIDE TARGET');assert.equal((await fs.lstat(link)).isSymbolicLink(),true);assert.equal((await store.history('si_selected')).length,1);
});
