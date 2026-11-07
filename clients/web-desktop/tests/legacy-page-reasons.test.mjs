import test from 'node:test';
import assert from 'node:assert/strict';
import {build} from 'esbuild';
import {createRequire} from 'node:module';
import {readFile} from 'node:fs/promises';
import path from 'node:path';
const client=path.resolve(import.meta.dirname,'..'),require=createRequire(path.join(client,'package.json'));
async function module(name){const r=await build({entryPoints:[path.join(client,'electron/main/localStore',name+'.ts')],bundle:true,write:false,platform:'node',format:'esm',external:['electron']});return import('data:text/javascript;base64,'+Buffer.from(r.outputFiles[0].text).toString('base64'));}
const [resolver,diagnostics]=await Promise.all([module('legacyQuarantine'),module('legacyFailure')]);
const SQL=await require('@jlongster/sql.js')({wasmBinary:await readFile(path.join(client,'dist/sql-wasm.wasm'))});
const account='emp_'+'c'.repeat(32),db=new SQL.Database();
db.exec('PRAGMA page_size=8192;CREATE TABLE local_users(user_id TEXT);');db.run('INSERT INTO local_users VALUES(?)',[account]);const bytes=db.export();db.close();
function raw(){const pages=[{key:-1,value:{size:bytes.length}}];for(let i=0;i<bytes.length;i+=8192)pages.push({key:i/8192,value:bytes.slice(i,i+8192)});return {name:account+'.sqlite',version:2,stores:['data'],pages};}
const cases=[
 ['QUARANTINE_NAME',r=>r.name='SYNTHETIC_PRIVATE_PATH/'+r.name],
 ['QUARANTINE_KEYS',r=>r.pages.push(r.pages[0])],
 ['QUARANTINE_SIZE',r=>r.pages[0].value.size=1e15],
 ['QUARANTINE_BINARY',r=>r.pages[1].value={body:'SYNTHETIC_PRIVATE_BODY'}],
 ['QUARANTINE_HEADER',r=>r.pages[1].value.fill(0)],
 ['QUARANTINE_GEOMETRY',r=>{r.pages[1].value[16]=0;r.pages[1].value[17]=3;}],
 ['QUARANTINE_MISSING_PAGE',r=>r.pages=r.pages.filter(p=>p.key!==1)],
 ['QUARANTINE_LENGTH',r=>r.pages[2].value=r.pages[2].value.slice(1)],
 ['QUARANTINE_PAGES',r=>r.pages[0]=null],
];
for(const [reason,mutate]of cases)test('real resolver exposes only fixed '+reason,()=>{
 const r=raw();mutate(r);let verified=0,observed;
 try{resolver.resolveLegacyQuarantine([r],account,()=>verified++);}catch(e){observed=e;}
 assert.ok(observed);assert.equal(verified,0);assert.equal(observed.legacyQuarantineReason,reason);
 const error=diagnostics.legacyMigrationFailure('import',diagnostics.legacyImportFailure('RESOLVE_QUARANTINE',observed));
 assert.equal(error.message,'PUTY_LEGACY_import_RESOLVE_QUARANTINE_'+reason);
 assert.deepEqual(Object.getOwnPropertyNames(error).sort(),['message','stack']);
 for(const value of [account,'SYNTHETIC_PRIVATE_PATH','SYNTHETIC_PRIVATE_BODY'])assert.equal(error.message.includes(value),false);
});
