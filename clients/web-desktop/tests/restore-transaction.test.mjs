import test from 'node:test';
import assert from 'node:assert/strict';
import {promises as fs} from 'node:fs';
import path from 'node:path';
import {randomUUID,createCipheriv,createDecipheriv,randomBytes,createHash} from 'node:crypto';
import {spawn} from 'node:child_process';
import {build} from 'esbuild';
import {pathToFileURL} from 'node:url';

const account='emp_'+'a'.repeat(32),other='emp_'+'b'.repeat(32),key=Buffer.alloc(32,71);
const hash=b=>createHash('sha256').update(b).digest('hex');
function cryptoOptions(overrides={}){return {
  protect:async value=>{const iv=randomBytes(12),c=createCipheriv('aes-256-gcm',key,iv);return Buffer.concat([iv,c.update(value),c.final(),c.getAuthTag()]);},
  unprotect:async value=>{const c=createDecipheriv('aes-256-gcm',key,value.subarray(0,12));c.setAuthTag(value.subarray(-16));return Buffer.concat([c.update(value.subarray(12,-16)),c.final()]);},
  check:async()=>{},freshSync:async()=>{},...overrides
};}
async function setup(t,{old=true}={}){
  const parent=path.resolve('D:/PUTY_AI/restore-transaction-tests');await fs.mkdir(parent,{recursive:true});
  const canonical=await fs.realpath(parent),home=await fs.mkdtemp(path.join(canonical,'puty-restore-'));
  t.after(async()=>{assert.equal(path.dirname(await fs.realpath(home)),canonical);assert.ok(path.basename(home).startsWith('puty-restore-'));await fs.rm(home,{recursive:true,force:true});});
  const root=path.join(home,'root'),accounts=path.join(root,'accounts');await fs.mkdir(accounts,{recursive:true});
  await fs.writeFile(path.join(root,'.puty-local-root.json'),JSON.stringify({format:'PUTY-LOCAL-ROOT-1',id:randomUUID()}));
  const active=path.join(accounts,account),stage=path.join(accounts,'.account-'+randomUUID());
  async function fill(base,prefix){await fs.mkdir(path.join(base,'vault','messages','202611'),{recursive:true});await fs.writeFile(path.join(base,'master.wrap'),prefix+' OS protected synthetic master');await fs.writeFile(path.join(base,'vault','checkpoint'),prefix+' synthetic encrypted checkpoint');await fs.writeFile(path.join(base,'vault','messages','202611','opaque.data'),prefix+' synthetic encrypted month');}
  if(old)await fill(active,'OLD');await fill(stage,'NEW');await fill(path.join(accounts,other),'OTHER');
  const output=await build({entryPoints:['electron/main/localStore/restoreTransaction.ts'],bundle:true,platform:'node',format:'esm',write:false});
  const bundle=path.join(home,'restoreTransaction.mjs');await fs.writeFile(bundle,output.outputFiles[0].contents);
  const load=()=>import(pathToFileURL(bundle).href+'?instance='+randomUUID());
  const module=await load();
  return {home,root,accounts,active,stage,bundle,load,module,options:cryptoOptions(),
    metadata:path.join(root,'.restore-transactions'),rollback:path.join(root,'.restore-rollback')};
}
async function journal(f){const file=path.join(f.metadata,account+'.journal');return JSON.parse((await f.options.unprotect(await fs.readFile(file))).toString());}
async function previous(f){const j=await journal(f);return path.join(f.root,...j.previous.split('/'));}
async function files(base){const out=[];async function walk(p){for(const name of await fs.readdir(p)){const next=path.join(p,name),s=await fs.lstat(next);if(s.isDirectory())await walk(next);else out.push([path.relative(base,next),hash(await fs.readFile(next))]);}}await walk(base);return out.sort();}
async function child(f,mode){
  const worker=path.join(f.home,'interrupt-'+randomUUID()+'.mjs');
  await fs.writeFile(worker,`import {promises as fs} from 'node:fs';import {createCipheriv,createDecipheriv,randomBytes} from 'node:crypto';import * as m from ${JSON.stringify(pathToFileURL(f.bundle).href)};
  const key=Buffer.alloc(32,71),root=${JSON.stringify(f.root)},account=${JSON.stringify(account)},stage=${JSON.stringify(f.stage)},mode=${JSON.stringify(mode)};
  const options={check:async()=>{},freshSync:async()=>{},protect:async value=>{const iv=randomBytes(12),c=createCipheriv('aes-256-gcm',key,iv);return Buffer.concat([iv,c.update(value),c.final(),c.getAuthTag()]);},unprotect:async value=>{const c=createDecipheriv('aes-256-gcm',key,value.subarray(0,12));c.setAuthTag(value.subarray(-16));return Buffer.concat([c.update(value.subarray(12,-16)),c.final()]);}};
  const rename=fs.rename;fs.rename=async(from,to)=>{await rename(from,to);if(mode==='old-moved'&&String(to).includes('.restore-rollback'))process.exit(83);if(mode==='new-promoted'&&String(from)===stage)process.exit(84);};
  if(mode==='gc-file'){const unlink=fs.unlink;fs.unlink=async file=>{await unlink(file);if(String(file).includes('.restore-rollback'))process.exit(85);};await m.collectRestorePrevious(root,account,options);}else await m.installRestoredAccount(root,account,stage,options);
  process.exit(99);`);
  return await new Promise((resolve,reject)=>{const p=spawn(process.execPath,[worker],{stdio:['ignore','pipe','pipe'],windowsHide:true});let out='';p.stdout.on('data',c=>out+=c);p.stderr.on('data',c=>out+=c);p.on('error',reject);p.on('close',code=>{assert.equal(out,'');resolve(code);});});
}

test('complete install protects metadata, retains old tree until fresh sync and only then exact GC',async t=>{
  const f=await setup(t),before=await files(path.join(f.accounts,other));
  const status=await f.module.installRestoredAccount(f.root,account,f.stage,f.options);
  assert.equal(status.state,'promoted');assert.equal(status.pendingPrevious,true);
  assert.match(await fs.readFile(path.join(f.active,'master.wrap'),'utf8'),/^NEW/);
  const old=await previous(f);assert.match(await fs.readFile(path.join(old,'master.wrap'),'utf8'),/^OLD/);
  for(const name of [account+'.journal',account+'.promoted']){const b=await fs.readFile(path.join(f.metadata,name));assert.equal(b.includes(Buffer.from(f.root)),false);assert.equal(b.includes(Buffer.from(account)),false);}
  const pending=await f.module.collectRestorePrevious(f.root,account,{...f.options,freshSync:undefined});
  assert.equal(pending.state,'pending');assert.equal(pending.reasonCode,'FRESH_SYNC_REQUIRED');
  assert.match(await fs.readFile(path.join(old,'master.wrap'),'utf8'),/^OLD/);
  // A real fresh SDK checkpoint is allowed to change the active tree.
  await fs.writeFile(path.join(f.active,'vault','checkpoint'),'FRESH SDK ENCRYPTED CHECKPOINT');
  await fs.writeFile(path.join(f.active,'vault','fresh.data'),'FRESH SDK DATA');
  const result=await f.module.collectRestorePrevious(f.root,account,f.options);
  assert.equal(result.state,'collected');assert.equal(result.pendingPrevious,false);
  assert.equal(await fs.stat(old).then(()=>true,()=>false),false);
  assert.deepEqual(await fs.readdir(f.metadata),[]);assert.deepEqual(await files(path.join(f.accounts,other)),before);
  assert.equal((await f.module.recoverRestoreTransaction(f.root,account,f.options)).state,'none');
});

for(const [mode,exit] of [['old-moved',83],['new-promoted',84]])test('new process recovers hard exit '+mode+' without opening an empty account',async t=>{
  const f=await setup(t);assert.equal(await child(f,mode),exit);
  const fresh=await f.load(),status=await fresh.recoverRestoreTransaction(f.root,account,f.options);
  assert.equal(status.state,'promoted');assert.match(await fs.readFile(path.join(f.active,'master.wrap'),'utf8'),/^NEW/);
  assert.match(await fs.readFile(path.join(await previous(f),'master.wrap'),'utf8'),/^OLD/);
  assert.equal((await fresh.collectRestorePrevious(f.root,account,f.options)).state,'collected');
});

test('new-account restore also journals promotion and fresh-sync metadata cleanup',async t=>{
  const f=await setup(t,{old:false});const installed=await f.module.installRestoredAccount(f.root,account,f.stage,f.options);
  assert.equal(installed.pendingPrevious,false);assert.equal(installed.state,'promoted');
  await fs.writeFile(path.join(f.active,'vault','new-sync'),'VALID NEW ACCOUNT SYNC');
  assert.equal((await f.module.collectRestorePrevious(f.root,account,f.options)).state,'collected');
  assert.deepEqual(await fs.readdir(f.metadata),[]);
});

test('authorization changed between directory renames leaves both source trees for a new authorized recovery',async t=>{
  const f=await setup(t),rename=fs.rename;let allowed=true;
  fs.rename=async(from,to)=>{await rename(from,to);if(String(to).includes('.restore-rollback'))allowed=false;};
  try{await assert.rejects(f.module.installRestoredAccount(f.root,account,f.stage,cryptoOptions({check:async()=>{if(!allowed)throw Error('synthetic actor changed');}})),/AUTHORIZATION_REQUIRED/);}finally{fs.rename=rename;}
  assert.equal(await fs.stat(f.active).then(()=>true,()=>false),false);
  assert.match(await fs.readFile(path.join(f.stage,'master.wrap'),'utf8'),/^NEW/);
  assert.match(await fs.readFile(path.join(await previous(f),'master.wrap'),'utf8'),/^OLD/);
  const fresh=await f.load();assert.equal((await fresh.recoverRestoreTransaction(f.root,account,f.options)).state,'promoted');
});

test('missing stage rolls the verified original account back, but losing every tree never creates a blank one',async t=>{
  const f=await setup(t);assert.equal(await child(f,'old-moved'),83);const old=await previous(f);
  await fs.rm(f.stage,{recursive:true});
  assert.equal((await (await f.load()).recoverRestoreTransaction(f.root,account,f.options)).state,'rolled-back');
  assert.match(await fs.readFile(path.join(f.active,'master.wrap'),'utf8'),/^OLD/);
  assert.equal(await fs.stat(old).then(()=>true,()=>false),false);assert.deepEqual(await fs.readdir(f.metadata),[]);
  const second=await setup(t,{old:false});
  const rename=fs.rename;fs.rename=async(from,to)=>{if(String(from)===second.stage)throw Object.assign(Error('synthetic stop'),{code:'EIO'});return rename(from,to);};
  try{await assert.rejects(second.module.installRestoredAccount(second.root,account,second.stage,second.options));}finally{fs.rename=rename;}
  await fs.rm(second.stage,{recursive:true});
  await assert.rejects((await second.load()).recoverRestoreTransaction(second.root,account,second.options),/RESTORE_DATA_MISSING/);
  assert.equal(await fs.stat(second.active).then(()=>true,()=>false),false);
});

test('foreign active directory cannot be overwritten during recovery',async t=>{
  const f=await setup(t);assert.equal(await child(f,'old-moved'),83);
  await fs.mkdir(f.active);await fs.writeFile(path.join(f.active,'foreign'),'FOREIGN DO NOT TOUCH');
  const before=await files(f.active);await assert.rejects((await f.load()).recoverRestoreTransaction(f.root,account,f.options),/LAYOUT_CHANGED|TREE_CHANGED/);
  assert.deepEqual(await files(f.active),before);assert.match(await fs.readFile(path.join(f.stage,'master.wrap'),'utf8'),/^NEW/);
});

test('old tree mutation or unknown file rejects collection before deleting any old receipt entry',async t=>{
  const f=await setup(t);await f.module.installRestoredAccount(f.root,account,f.stage,f.options);
  const old=await previous(f),unknown=path.join(old,'unknown.data');await fs.writeFile(unknown,'UNKNOWN DO NOT TOUCH');
  const before=await files(old),result=await f.module.collectRestorePrevious(f.root,account,f.options);
  assert.equal(result.state,'pending');assert.equal(result.pendingPrevious,true);assert.deepEqual(await files(old),before);
  await fs.unlink(unknown);const file=path.join(old,'master.wrap'),original=await fs.readFile(file);await fs.unlink(file);await fs.writeFile(file,original);
  const replaced=await files(old);assert.equal((await f.module.collectRestorePrevious(f.root,account,f.options)).state,'pending');assert.deepEqual(await files(old),replaced);
});

test('GC hard exit after one unlink is restartable and never revalidates changing active payload hashes',async t=>{
  const f=await setup(t);await f.module.installRestoredAccount(f.root,account,f.stage,f.options);
  assert.equal(await child(f,'gc-file'),85);const old=await previous(f);assert.ok((await files(old)).length<3);
  await fs.writeFile(path.join(f.active,'vault','checkpoint'),'NEW LIVE CHECKPOINT');
  const fresh=await f.load();assert.equal((await fresh.recoverRestoreTransaction(f.root,account,f.options)).state,'promoted');
  assert.equal((await fresh.collectRestorePrevious(f.root,account,f.options)).state,'collected');
  assert.equal(await fs.readFile(path.join(f.active,'vault','checkpoint'),'utf8'),'NEW LIVE CHECKPOINT');
});

test('post-GC metadata interruption resumes from collected receipt after the new SDK has changed its files',async t=>{
  const f=await setup(t);await f.module.installRestoredAccount(f.root,account,f.stage,f.options);
  const unlink=fs.unlink;let stop=true;fs.unlink=async file=>{if(stop&&String(file).endsWith('.promoted'))throw Object.assign(Error('synthetic metadata interruption'),{code:'EACCES'});return unlink(file);};
  try{assert.equal((await f.module.collectRestorePrevious(f.root,account,f.options)).state,'pending');}finally{stop=false;fs.unlink=unlink;}
  assert.ok((await fs.readdir(f.metadata)).includes(account+'.collected'));
  await fs.writeFile(path.join(f.active,'vault','checkpoint'),'LIVE SDK');
  const result=await (await f.load()).recoverRestoreTransaction(f.root,account,f.options);
  assert.equal(result.state,'collected');assert.deepEqual(await fs.readdir(f.metadata),[]);
});

test('symlink, hardlink and outside stage are rejected without moving either account',async t=>{
  const f=await setup(t),before=await files(f.active);
  const external=path.join(f.home,'.account-'+randomUUID());await fs.mkdir(external);
  await assert.rejects(f.module.installRestoredAccount(f.root,account,external,f.options),/OWNED_STAGE_REQUIRED/);
  const target=path.join(f.stage,'master.wrap'),link=path.join(f.stage,'duplicate');await fs.link(target,link);
  await assert.rejects(f.module.installRestoredAccount(f.root,account,f.stage,f.options),/SINGLE_LINK/);await fs.unlink(link);
  const junction=path.join(f.stage,'junction');await fs.symlink(path.join(f.accounts,other),junction,'junction');
  await assert.rejects(f.module.installRestoredAccount(f.root,account,f.stage,f.options));await fs.unlink(junction);
  assert.deepEqual(await files(f.active),before);
});

test('root marker changes and protected metadata tamper reject recovery with zero source writes',async t=>{
  const f=await setup(t);assert.equal(await child(f,'old-moved'),83);
  const marker=path.join(f.root,'.puty-local-root.json'),original=await fs.readFile(marker);await fs.writeFile(marker,JSON.stringify({format:'PUTY-LOCAL-ROOT-1',id:randomUUID()}));
  await assert.rejects((await f.load()).recoverRestoreTransaction(f.root,account,f.options),/ROOT_BINDING_CHANGED/);
  const old=await previous(f),before=await files(old);assert.match(await fs.readFile(path.join(f.stage,'master.wrap'),'utf8'),/^NEW/);
  // Tamper is separately checked on a clean fixture: exact root identity cannot
  // be restored merely by replacing its marker with the same JSON bytes.
  assert.notDeepEqual(await fs.readFile(marker),original);assert.deepEqual(await files(old),before);
  const second=await setup(t);assert.equal(await child(second,'old-moved'),83);const file=path.join(second.metadata,account+'.journal'),bytes=await fs.readFile(file);bytes[20]^=1;await fs.writeFile(file,bytes);
  await assert.rejects((await second.load()).recoverRestoreTransaction(second.root,account,second.options),/PROTECTED_METADATA_REJECTED/);
  assert.equal(await fs.stat(second.active).then(()=>true,()=>false),false);
});

test('metadata disk failure and concurrent installs cannot publish a partial replacement or delete old bytes',async t=>{
  const f=await setup(t),before=await files(f.active);
  await assert.rejects(f.module.installRestoredAccount(f.root,account,f.stage,{...f.options,protect:async()=>{throw Object.assign(Error('synthetic full disk'),{code:'ENOSPC'});}}));
  assert.deepEqual(await files(f.active),before);assert.match(await fs.readFile(path.join(f.stage,'master.wrap'),'utf8'),/^NEW/);
  const values=await Promise.allSettled([f.module.installRestoredAccount(f.root,account,f.stage,f.options),f.module.installRestoredAccount(f.root,account,f.stage,f.options)]);
  assert.equal(values.filter(v=>v.status==='fulfilled').length,1);assert.match(values.find(v=>v.status==='rejected').reason.message,/PENDING_RESTORE_TRANSACTION/);
});

test('collection authorization loss remains pending and retries exact remaining files without recursive rm',async t=>{
  const f=await setup(t);await f.module.installRestoredAccount(f.root,account,f.stage,f.options);
  const unlink=fs.unlink,remove=fs.rm;let allowed=true;
  fs.unlink=async file=>{await unlink(file);if(String(file).includes('.restore-rollback'))allowed=false;};fs.rm=async()=>{throw Error('Recursive removal forbidden in product helper');};
  try{const result=await f.module.collectRestorePrevious(f.root,account,cryptoOptions({check:async()=>{if(!allowed)throw Error('synthetic expired actor');}}));assert.equal(result.state,'pending');assert.equal(result.reasonCode,'AUTHORIZATION_REQUIRED');}
  finally{fs.unlink=unlink;fs.rm=remove;}
  assert.equal((await (await f.load()).collectRestorePrevious(f.root,account,f.options)).state,'collected');
});

test('complete journal left by post-rename fsync failure must be flushed on every retry before moving account trees',async t=>{
  const f=await setup(t),open=fs.open,rename=fs.rename;let denied=true,moves=0;
  fs.open=async(file,...args)=>{const h=await open(file,...args),sync=h.sync.bind(h);h.sync=async()=>{if(denied&&String(file)===path.join(f.metadata,account+'.journal'))throw Object.assign(Error('synthetic journal flush failure'),{code:'EIO'});return sync();};return h;};
  fs.rename=async(from,to)=>{if(String(from)===f.stage||String(to).startsWith(f.rollback+path.sep))moves++;return rename(from,to);};
  try{
    await assert.rejects(f.module.installRestoredAccount(f.root,account,f.stage,f.options),{code:'EIO'});
    assert.ok((await fs.readdir(f.metadata)).includes(account+'.journal'));assert.equal(moves,0);
    await assert.rejects((await f.load()).recoverRestoreTransaction(f.root,account,f.options),{code:'EIO'});
    assert.equal(moves,0);assert.match(await fs.readFile(path.join(f.active,'master.wrap'),'utf8'),/^OLD/);
    denied=false;assert.equal((await (await f.load()).recoverRestoreTransaction(f.root,account,f.options)).state,'promoted');assert.equal(moves,2);
  }finally{fs.open=open;fs.rename=rename;}
});

test('GC intent post-rename fsync failure is not mistaken for permission to delete on a later retry',async t=>{
  const f=await setup(t);await f.module.installRestoredAccount(f.root,account,f.stage,f.options);
  const old=await previous(f),before=await files(old),open=fs.open,unlink=fs.unlink;let denied=true,deletes=0;
  fs.open=async(file,...args)=>{const h=await open(file,...args),sync=h.sync.bind(h);h.sync=async()=>{if(denied&&String(file)===path.join(f.metadata,account+'.gc'))throw Object.assign(Error('synthetic GC flush failure'),{code:'EIO'});return sync();};return h;};
  fs.unlink=async file=>{if(String(file).startsWith(f.rollback+path.sep))deletes++;return unlink(file);};
  try{
    assert.equal((await f.module.collectRestorePrevious(f.root,account,f.options)).state,'pending');
    assert.equal((await (await f.load()).collectRestorePrevious(f.root,account,f.options)).state,'pending');
    assert.equal(deletes,0);assert.deepEqual(await files(old),before);
    denied=false;assert.equal((await (await f.load()).collectRestorePrevious(f.root,account,f.options)).state,'collected');assert.equal(deletes,3);
  }finally{fs.open=open;fs.unlink=unlink;}
});

test('ordinary recovery without a journal still refuses a redirected UID directory',async t=>{
  const f=await setup(t,{old:false});await fs.symlink(path.join(f.accounts,other),f.active,'junction');
  await assert.rejects(f.module.recoverRestoreTransaction(f.root,account,f.options),/DIRECTORY_OWNERSHIP_REQUIRED/);
  assert.match(await fs.readFile(path.join(f.accounts,other,'master.wrap'),'utf8'),/^OTHER/);
});
