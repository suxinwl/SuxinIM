import test from 'node:test';
import assert from 'node:assert/strict';
import {build} from 'esbuild';
import {mkdtemp,rm} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import path from 'node:path';
const compiled=await build({entryPoints:['electron/main/localStore/vault.ts'],bundle:true,write:false,platform:'node',format:'esm'});
const {LocalVault}=await import('data:text/javascript;base64,'+Buffer.from(compiled.outputFiles[0].text+'\n//# sourceURL=archive-query-vault.js').toString('base64'));
const account='test_archive_query',key=Buffer.alloc(32,17),conversationID='si_query',T=Date.parse('2026-11-04T00:00:00Z');
const event=(id,time=T,seq=0)=>({kind:'upsert',conversationID,row:{client_msg_id:id,send_time:time,seq,content_type:101,content:'synthetic archive needle',status:2}});
async function fixture(t,events){const directory=await mkdtemp(path.join(tmpdir(),'puty-query-'));let vault=await LocalVault.open(directory,account,key,async()=>{});await vault.commit({opID:'query-fixture',statements:[],events});t.after(async()=>{vault.lock();await rm(directory,{recursive:true,force:true});});return {get vault(){return vault;},async reopen(){vault.lock();vault=await LocalVault.open(directory,account,key,async()=>{});}};}
const ids=rows=>rows.map(row=>row.client_msg_id);
test('same-millisecond seq-zero cursors traverse every row exactly once in both directions across reopen',async t=>{
  const state=await fixture(t,[event('b'),event('d'),event('a'),event('c'),event('server-2',T,2),event('server-1',T,1),event('older',T-1,1),event('newer',T+1,1)]);
  await state.reopen();
  for(const asc of [false,true]){
    let cursor=[conversationID,2,asc?1:Number.MAX_SAFE_INTEGER,0,'',asc],seen=[];
    for(let page=0;page<10;page++){
      const rows=await state.vault.sdkQuery('getMessageList',cursor);if(!rows.length)break;
      seen.push(...ids(rows));const last=rows.at(-1);cursor=[conversationID,2,last.send_time,last.seq,last.client_msg_id,asc];
    }
    const ordered=['older','a','b','c','d','server-1','server-2','newer'];if(!asc)ordered.reverse();
    assert.deepEqual(seen,ordered);assert.equal(new Set(seen).size,8);
  }
  assert.deepEqual(ids(await state.vault.history(conversationID,T,2,'c')),['b','a']);
});
test('SDK queries without an explicit page limit retain all 1025 encrypted archive rows',async t=>{
  const rows=Array.from({length:1025},(_,i)=>event('m'+String(i).padStart(4,'0'),T+i,i+1));
  const {vault}=await fixture(t,rows),allIDs=rows.map(v=>v.row.client_msg_id),allSeqs=rows.map(v=>v.row.seq);
  for(const [name,args] of [
    ['getLatestActiveMessage',[conversationID,false]],
    ['getMultipleMessage',[conversationID,allIDs]],
    ['getMessagesByClientMsgIDs',[conversationID,JSON.stringify(allIDs)]],
    ['getMessagesBySeqs',[conversationID,allSeqs]],
    ['searchAllMessageByContentType',[conversationID,101]],
    ['searchMessageByContentTypeAndKeyword',[conversationID,[101],['needle'],1,0,Number.MAX_SAFE_INTEGER]],
  ]){const result=await vault.sdkQuery(name,args);assert.equal(result.length,1025,name);assert.equal(new Set(ids(result)).size,1025,name);}
  assert.equal((await vault.sdkQuery('getMessageListNoTime',[conversationID,1002,false])).length,1002);
  const page=await vault.sdkQuery('searchMessageByContentType',[conversationID,[101],0,Number.MAX_SAFE_INTEGER,1000,25]);
  assert.equal(page.length,25);assert.equal(page[0].client_msg_id,'m0024');assert.equal(page.at(-1).client_msg_id,'m0000');
});
test('zero-size pages are empty and latest-server time cursor is strictly exclusive',async t=>{
  const {vault}=await fixture(t,[event('at',T,9),event('old',T-1,8),event('new',T+1,10),event('pending',T+2,0)]);
  assert.deepEqual(await vault.sdkQuery('getMessageListNoTime',[conversationID,0,false]),[]);
  assert.deepEqual(ids(await vault.sdkQuery('getLatestValidServerMessage',[conversationID,T,true])),['old']);
  assert.deepEqual(ids(await vault.sdkQuery('getLatestValidServerMessage',[conversationID,T,false])),['new']);
  for(const count of [-1,NaN,Infinity,1.5])await assert.rejects(vault.sdkQuery('getMessageListNoTime',[conversationID,count,false]),/range/);
});
test('working rows replace archive identities before stable cursor comparison',async t=>{
  const {vault}=await fixture(t,[event('a'),event('b'),event('c')]);
  const transient=[{client_msg_id:'b',send_time:T,seq:0,status:2,content_type:101,content:'synthetic updated'}];
  assert.deepEqual(ids(await vault.sdkQuery('getMessageList',[conversationID,10,T,0,'c',false],transient)),['b','a']);
  assert.deepEqual(ids(await vault.sdkQuery('getMessageList',[conversationID,10,T,0,'a',true],transient)),['b','c']);
});
