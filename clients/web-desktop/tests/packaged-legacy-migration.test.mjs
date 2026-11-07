import test from 'node:test';
import assert from 'node:assert/strict';
import {spawnSync} from 'node:child_process';
import {optionsFor,safeFailure,validateResult} from '../scripts/accept-packaged-legacy-migration.mjs';
test('packaged legacy operator is dry by default and requires both exact explicit package hashes to run',()=>{
  assert.equal(optionsFor([]).run,false);assert.equal(optionsFor(['--source-removed']).sourceRemoved,true);assert.equal(optionsFor(['--cold']).cold,true);assert.throws(()=>optionsFor(['--cold','--source-removed']));
  for(const args of [['--run'],['--run','--exe-sha256','a'.repeat(64)],['--run','--asar-sha256','b'.repeat(64)],['--unknown'],['--run','--run']])assert.throws(()=>optionsFor(args));
  assert.deepEqual(optionsFor(['--run','--exe-sha256','a'.repeat(64),'--asar-sha256','b'.repeat(64)]),{run:true,expectBlocked:false,sourceRemoved:false,cold:false,exeSHA:'a'.repeat(64),asarSHA:'b'.repeat(64)});
});
test('operator only publishes a fixed failure category from native exceptions',()=>{
  assert.equal(safeFailure(Error('Error invoking remote method puty-store: EBUSY private-file')),'cleanup_EBUSY');
  assert.equal(safeFailure(Error('Error invoking remote method: PUTY_LEGACY_cleanup_EBUSY')),'cleanup_EBUSY');
  assert.equal(safeFailure(Error('untrusted-private-file')),'IPC_FAILED');
  assert.equal(safeFailure(Error('PUTY_LEGACY_SECRET_TOKEN')),'IPC_FAILED');
});
test('successful actual Main requires the exact imported current-account message and attachment counts',()=>{
  validateResult({ok:true,messages:1,attachments:1});
  for(const value of [{ok:true,messages:0,attachments:1},{ok:true,messages:2,attachments:1},{ok:true,messages:1,attachments:0},{ok:false,code:'cleanup_EBUSY'},{ok:1,messages:1,attachments:1}])assert.throws(()=>validateResult(value));
  validateResult({ok:false,code:'cleanup_EBUSY'},{blocked:true});
  assert.throws(()=>validateResult({ok:false,code:'IPC_FAILED'},{blocked:true}));
});
test('subprocess dry run does not inspect packages, credentials, profiles or network',()=>{
  const result=spawnSync(process.execPath,['scripts/accept-packaged-legacy-migration.mjs'],{encoding:'utf8'});assert.equal(result.status,0);
  assert.deepEqual(JSON.parse(result.stdout),{dryRun:true,privateInputsRead:0,packageRead:0,productionRequests:0});
});
