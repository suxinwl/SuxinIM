import test from 'node:test';
import assert from 'node:assert/strict';
import {build} from 'esbuild';
import {createRequire} from 'node:module';
import {createCipheriv,createDecipheriv,randomBytes} from 'node:crypto';
import {readFile} from 'node:fs/promises';
import path from 'node:path';
const output=await build({entryPoints:['electron/main/localStore/legacyFailure.ts'],bundle:true,write:false,format:'esm',platform:'node'});
const {legacyImportFailure,legacyMigrationFailure}=await import('data:text/javascript;base64,'+Buffer.from(output.outputFiles[0].text).toString('base64'));
const wire=(step,error)=>legacyMigrationFailure('import',legacyImportFailure(step,error));
const safe=error=>{assert.doesNotMatch(error.message,/SYNTHETIC_PRIVATE|sensitive_payload|sqlite_secret|E:\\|D:\\|file:/);assert.equal(error.cause,undefined);};

test('real OS ENOENT keeps actionable import location without exposing its private path',async()=>{
  let caught;try{await readFile(path.resolve('D:/puty-build-temp/SYNTHETIC_PRIVATE_NONEXISTENT_legacy_file.sqlite'));}catch(error){caught=error;}
  assert.equal(caught?.code,'ENOENT');assert.match(caught.message,/SYNTHETIC_PRIVATE/);
  const result=wire('READ_DATABASE',caught);assert.equal(result.message,'PUTY_LEGACY_import_READ_DATABASE_ENOENT');safe(result);
});
test('real SQL.js schema failure becomes a diagnostic without the SQL identifier',async()=>{
  const requireSDK=createRequire(path.resolve('../../sdks/wasm/package.json'));
  const SQL=await requireSDK('@jlongster/sql.js')({wasmBinary:await readFile('../../sdks/wasm/assets/sql-wasm.wasm')});
  const db=new SQL.Database();let caught;try{db.exec('CREATE TABLE current_table(id TEXT);SELECT sqlite_secret_column FROM current_table');}catch(error){caught=error;}finally{db.close();}
  assert.match(caught.message,/sqlite_secret_column/);const result=wire('PREPARE_MESSAGES',caught);
  assert.equal(result.message,'PUTY_LEGACY_import_PREPARE_MESSAGES_SCHEMA_UNSUPPORTED');safe(result);
});
test('real AES-GCM authentication failure retains a fixed backup-integrity category',()=>{
  const key=randomBytes(32),iv=randomBytes(12),cipher=createCipheriv('aes-256-gcm',key,iv),sealed=Buffer.concat([cipher.update('sensitive_payload'),cipher.final()]),tag=cipher.getAuthTag();tag[0]^=1;
  const decipher=createDecipheriv('aes-256-gcm',key,iv);decipher.setAuthTag(tag);decipher.update(sealed);let caught;try{decipher.final();}catch(error){caught=error;}finally{key.fill(0);}
  assert.ok(caught);const result=wire('READ_DATABASE',caught);assert.equal(result.message,'PUTY_LEGACY_import_READ_DATABASE_BACKUP_INTEGRITY');safe(result);
});
test('unknown and forged diagnostics never serialize a raw exception or caller-defined code',()=>{
  const original=Object.assign(new Error('sensitive_payload E:\\SYNTHETIC_PRIVATE\\old.sqlite'),{code:'sqlite_secret',legacyImportStep:'SYNTHETIC_PRIVATE',legacyImportReason:'sensitive_payload'});
  const result=wire('COMMIT_HISTORY',original);assert.equal(result.message,'PUTY_LEGACY_import_COMMIT_HISTORY_UNKNOWN');safe(result);
  const forged=legacyMigrationFailure('import',Object.assign(new Error('sensitive_payload'),{legacyImportStep:'COMMIT_HISTORY',legacyImportReason:'SYNTHETIC_PRIVATE'}));assert.equal(forged.message,'PUTY_LEGACY_import_UNKNOWN');safe(forged);
});
