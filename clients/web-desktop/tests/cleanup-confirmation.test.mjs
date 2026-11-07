import test from 'node:test';
import assert from 'node:assert/strict';
import {build} from 'esbuild';
import {promises as fs} from 'node:fs';
import path from 'node:path';
import {tmpdir} from 'node:os';
const compiled=await build({entryPoints:['electron/main/localStore/vault.ts'],bundle:true,write:false,platform:'node',format:'esm'});
const {LocalVault}=await import('data:text/javascript;base64,'+Buffer.from(compiled.outputFiles[0].text).toString('base64'));
const key=Buffer.alloc(32,63),account='test_cleanup_confirmation';
const event=(id,time,conversationID='si_a')=>({kind:'upsert',conversationID,row:{client_msg_id:id,content:'SYNTHETIC_BODY',status:2,send_time:time}});
test('clear-all caps its range at confirmation, preserves exact skewed IDs, and permits later messages after restart',async t=>{
  const root=await fs.realpath(await fs.mkdtemp(path.join(tmpdir(),'puty-clear-confirm-'))),validate=async()=>{},vault=await LocalVault.open(root,account,key,validate),confirmed=Date.parse('2026-11-04T10:00:00Z');
  t.after(async()=>{vault.lock();assert.equal(path.dirname(root),await fs.realpath(tmpdir()));assert.ok(path.basename(root).startsWith('puty-clear-confirm-'));await fs.rm(root,{recursive:true,force:true});});
  await vault.commit({opID:'initial',statements:[],events:[event('old',confirmed-1),event('skewed-existing',confirmed+10000),event('other',confirmed-1,'si_b')]});
  const working=[{conversationID:'si_a',clientMsgID:'hot-only-future',sendTime:confirmed+5000}],now=Date.now;
  try{Date.now=()=>confirmed+100;await vault.clean({conversationIDs:['si_a'],beforeMS:Number.MAX_SAFE_INTEGER,maskBeforeMS:confirmed+1},Buffer.from('SYNTHETIC_SQL_SNAPSHOT'),working);}finally{Date.now=now;}
  const policy=await vault.restorationPolicy();assert.deepEqual(policy.ranges,[{conversationIDs:['si_a'],beforeMS:confirmed+1}]);assert.equal(policy.suppressions.filter(v=>v.mode==='delete').length,3);
  await vault.commit({opID:'late',statements:[],events:[event('old',confirmed+20000),event('skewed-existing',confirmed+20000),event('hot-only-future',confirmed+20000),event('new-after-clear',confirmed+1),event('previous-unseen-history',confirmed-5)]});
  assert.deepEqual((await vault.history('si_a')).map(v=>v.client_msg_id),['new-after-clear']);assert.equal((await vault.history('si_b')).length,1);
  vault.lock();const fresh=await LocalVault.open(root,account,key,validate);t.after(()=>fresh.lock());assert.deepEqual((await fresh.history('si_a')).map(v=>v.client_msg_id),['new-after-clear']);assert.deepEqual((await fresh.restorationPolicy()).ranges,policy.ranges);
  await fresh.commit({opID:'after-restart',statements:[],events:[event('old',confirmed+30000),event('another-new-id',confirmed+2)]});assert.equal((await fresh.history('si_a')).length,2);
});
test('an invalid SDK confirmation boundary cannot mutate cleanup policy or existing messages',async t=>{
  const root=await fs.realpath(await fs.mkdtemp(path.join(tmpdir(),'puty-clear-confirm-'))),vault=await LocalVault.open(root,account,key,async()=>{});
  t.after(async()=>{vault.lock();assert.equal(path.dirname(root),await fs.realpath(tmpdir()));assert.ok(path.basename(root).startsWith('puty-clear-confirm-'));await fs.rm(root,{recursive:true,force:true});});
  await vault.commit({opID:'initial',statements:[],events:[event('keep',Date.now()-1)]});
  for(const maskBeforeMS of [NaN,Infinity,-1,1001])await assert.rejects(vault.clean({beforeMS:1000,maskBeforeMS},Buffer.from('SYNTHETIC')),/confirmation/);
  await assert.rejects(vault.clean({beforeMS:Number.MAX_SAFE_INTEGER},Buffer.from('SYNTHETIC'),[{conversationID:'si_a',clientMsgID:'bad-time',sendTime:NaN}]),/working cleanup/);
  assert.equal((await vault.history('si_a')).length,1);assert.deepEqual((await vault.restorationPolicy()).ranges,[]);
});
test('a message queued while cleanup awaits authorization is not swallowed by a later main-process clock',async t=>{
  const root=await fs.realpath(await fs.mkdtemp(path.join(tmpdir(),'puty-clear-confirm-')));let hold=false,release,entered;
  const gate=new Promise(resolve=>entered=resolve),validate=async()=>{if(hold){hold=false;entered();await new Promise(resolve=>release=resolve);}},vault=await LocalVault.open(root,account,key,validate),confirmed=Date.parse('2026-11-04T12:00:00Z'),now=Date.now;let clock=confirmed;
  t.after(async()=>{release?.();Date.now=now;vault.lock();assert.equal(path.dirname(root),await fs.realpath(tmpdir()));assert.ok(path.basename(root).startsWith('puty-clear-confirm-'));await fs.rm(root,{recursive:true,force:true});});
  await vault.commit({opID:'seed',statements:[],events:[event('old',confirmed-1)]});
  try{Date.now=()=>clock;hold=true;const cleaning=vault.clean({beforeMS:Number.MAX_SAFE_INTEGER,maskBeforeMS:confirmed+1},Buffer.from('SYNTHETIC_SQL'));
    await gate;clock=confirmed+10000;const incoming=vault.commit({opID:'queued-after-confirmation',statements:[],events:[event('new-while-cleaning',confirmed+500)]});release();await cleaning;await incoming;
  }finally{Date.now=now;}
  assert.deepEqual((await vault.history('si_a')).map(v=>v.client_msg_id),['new-while-cleaning']);assert.equal((await vault.restorationPolicy()).ranges[0].beforeMS,confirmed+1);
});
test('verified historical restore retains its authenticated range instead of guessing a new cutoff',async t=>{
  const root=await fs.realpath(await fs.mkdtemp(path.join(tmpdir(),'puty-clear-confirm-'))),vault=await LocalVault.open(root,account,key,async()=>{});
  t.after(async()=>{vault.lock();assert.equal(path.dirname(root),await fs.realpath(tmpdir()));assert.ok(path.basename(root).startsWith('puty-clear-confirm-'));await fs.rm(root,{recursive:true,force:true});});
  await vault.preserveHistoricalCleanup({conversationIDs:['si_legacy'],beforeMS:Number.MAX_SAFE_INTEGER},Buffer.from('SYNTHETIC_VERIFIED_OLD_POLICY'));
  assert.deepEqual((await vault.restorationPolicy()).ranges,[{conversationIDs:['si_legacy'],beforeMS:Number.MAX_SAFE_INTEGER}]);
  const runtime=await fs.readFile('electron/main/localStore/runtime.ts','utf8');assert.equal(runtime.includes('store.preserveHistoricalCleanup('),false);
});
