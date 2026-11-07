// Real production diagnostic classifier and renderer mapper; synthetic errors only.
// No employee files, GUI, network, authorization service or recovery writes.
import test from 'node:test';
import assert from 'node:assert/strict';
import {serialize,deserialize} from 'node:v8';
import {build} from 'esbuild';

async function load(entry){
  const compiled=await build({entryPoints:[entry],bundle:true,write:false,format:'esm',platform:'node'});
  return import('data:text/javascript;base64,'+Buffer.from(compiled.outputFiles[0].text).toString('base64'));
}
const [main,ui]=await Promise.all([
  load('electron/main/localStore/legacyFailure.ts'),
  load('src/pages/login/local-storage-error.ts'),
]);
const expectedReasons=[
  'QUARANTINE_NO_CANDIDATE','QUARANTINE_SCHEMA','QUARANTINE_PAGES','QUARANTINE_CONFLICT',
  'QUARANTINE_IDENTITY','QUARANTINE_SQLITE','QUARANTINE_CAPSULE','QUARANTINE_LIMIT',
  'QUARANTINE_NAME','QUARANTINE_KEYS','QUARANTINE_SIZE','QUARANTINE_BINARY',
  'QUARANTINE_HEADER','QUARANTINE_GEOMETRY','QUARANTINE_MISSING_PAGE','QUARANTINE_LENGTH',
];
const privateMessage='SYNTHETIC_PRIVATE_PAYLOAD D:\\SYNTHETIC_PRIVATE\\old-account.sqlite TOKEN_SYNTHETIC_ONLY';
const privateCause='SYNTHETIC_PRIVATE_CAUSE sealed-source-key-synthetic-only';
const privatePath='D:\\SYNTHETIC_PRIVATE\\capsule-source.sqlite';
const remote=message=>new Error("Error invoking remote method 'puty-store': Error: "+message);
const wire=(error,step='RESOLVE_QUARANTINE')=>main.legacyMigrationFailure('import',main.legacyImportFailure(step,error));

function sensitiveFailure(reason){
  return Object.assign(new Error(privateMessage,{cause:new Error(privateCause)}),{
    path:privatePath,code:'UNTRUSTED_SYNTHETIC_CODE',legacyQuarantineReason:reason,
    secret:'TOKEN_SYNTHETIC_ONLY',
  });
}

function assertPrivateBoundary(error,expected){
  assert.equal(error.message,expected);
  assert.deepEqual(Object.keys(error),[],'the outward Error must not carry classified/raw fields');
  assert.equal(Object.hasOwn(error,'cause'),false,'raw nested exceptions must end before IPC');
  assert.equal(Object.hasOwn(error,'path'),false);
  assert.equal(Object.hasOwn(error,'code'),false);
  assert.equal(Object.hasOwn(error,'legacyImportReason'),false);
  assert.equal(Object.hasOwn(error,'legacyQuarantineReason'),false);
  const cloned=deserialize(serialize(error));
  assert.equal(cloned.message,expected);
  assert.equal(cloned.cause,undefined);
  for(const value of [error.message,error.stack,cloned.message,cloned.stack]){
    assert.doesNotMatch(String(value),/SYNTHETIC_PRIVATE|TOKEN_SYNTHETIC_ONLY|sealed-source-key-synthetic-only|UNTRUSTED_SYNTHETIC_CODE/);
    assert.equal(String(value).includes(privatePath),false);
  }
}

test('production quarantine diagnostic vocabulary contains exactly the sixteen approved reasons',()=>{
  assert.deepEqual([...main.legacyQuarantineReasons],expectedReasons);
  assert.ok(main.legacyImportSteps.includes('RESOLVE_QUARANTINE'));
  for(const reason of expectedReasons)assert.ok(main.legacyImportReasons.includes(reason));
});

test('all approved production quarantine classifiers remove private cause and path at the outward error boundary',()=>{
  for(const reason of expectedReasons){
    const source=sensitiveFailure(reason);
    const internal=main.legacyImportFailure('RESOLVE_QUARANTINE',source);
    assert.equal(internal.legacyImportStep,'RESOLVE_QUARANTINE');
    assert.equal(internal.legacyImportReason,reason);
    assert.equal(internal.cause,source,'raw context remains available internally, before the IPC boundary');
    const result=main.legacyMigrationFailure('import',internal);
    const code=`PUTY_LEGACY_import_RESOLVE_QUARANTINE_${reason}`;
    assertPrivateBoundary(result,code);
    const displayed=ui.localStorageIPCMessage(remote(result.message),true);
    assert.match(displayed,/重检旧版隔离档案/);
    assert.ok(displayed.includes(code));
    assert.doesNotMatch(displayed,/SYNTHETIC_PRIVATE|TOKEN_SYNTHETIC_ONLY|sealed-source-key-synthetic-only|删除|跳过|空目录|改文件名/);
  }
});

test('unknown and secret-bearing quarantine reasons never become an outward diagnostic reason',()=>{
  const coercionTrap={toString(){throw new Error('quarantine reason must never be coerced');}};
  for(const reason of [
    'QUARANTINE_UNKNOWN','QUARANTINE_CONFLICT TOKEN_SYNTHETIC_ONLY',
    'QUARANTINE_CONFLICT\nSYNTHETIC_PRIVATE','QUARANTINE_CONFLICT\0TOKEN_SYNTHETIC_ONLY',
    'quarantine_conflict',privatePath,coercionTrap,42,null,undefined,
  ]){
    const result=wire(sensitiveFailure(reason));
    assertPrivateBoundary(result,'PUTY_LEGACY_import_RESOLVE_QUARANTINE_UNKNOWN');
  }
});

test('forged tagged errors and unknown steps cannot carry private raw text or nested causes through the boundary',()=>{
  for(const reason of expectedReasons){
    const source=Object.assign(sensitiveFailure('QUARANTINE_UNKNOWN'),{
      legacyImportStep:'RESOLVE_QUARANTINE',legacyImportReason:reason,
    });
    // Existing approved tags are intentionally idempotent inside Main. The
    // final outward wrapper must still create a new, context-free Error.
    assert.equal(main.legacyImportFailure('READ_DATABASE',source),source);
    assertPrivateBoundary(main.legacyMigrationFailure('import',source),`PUTY_LEGACY_import_RESOLVE_QUARANTINE_${reason}`);
  }
  const unknown=Object.assign(sensitiveFailure('QUARANTINE_UNKNOWN'),{
    legacyImportStep:'RESOLVE_QUARANTINE',legacyImportReason:'QUARANTINE_CONFLICT TOKEN_SYNTHETIC_ONLY',
  });
  assertPrivateBoundary(main.legacyMigrationFailure('import',unknown),'PUTY_LEGACY_import_UNKNOWN');
  assertPrivateBoundary(wire(unknown),'PUTY_LEGACY_import_RESOLVE_QUARANTINE_UNKNOWN');
  assertPrivateBoundary(wire(sensitiveFailure('QUARANTINE_CONFLICT'),'SYNTHETIC_PRIVATE_STEP'),'PUTY_LEGACY_import_UNKNOWN');
});

test('unclassified quarantine objects and non-import phases keep the legacy fixed-code boundary',()=>{
  for(const reason of expectedReasons){
    const source=sensitiveFailure(reason);
    assertPrivateBoundary(main.legacyMigrationFailure('import',source),'PUTY_LEGACY_import_UNKNOWN');
    for(const phase of ['inventory','reader','seal','cleanup']){
      assertPrivateBoundary(main.legacyMigrationFailure(phase,source),`PUTY_LEGACY_${phase}_UNKNOWN`);
    }
  }
});
