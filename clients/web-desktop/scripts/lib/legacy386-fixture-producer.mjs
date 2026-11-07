// Synthetic fixture producer. Uses the exact shipped PC 3.8.6 SQLiteFS and
// File implementation. Only FileOps persistence is replaced with owned RAM;
// SQL writes still pass through the historical FS/File page writer.
import { promises as fs } from 'node:fs';
import path from 'node:path';
import { createHash } from 'node:crypto';
import { createRequire } from 'node:module';
import { fileURLToPath } from 'node:url';

const client=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'../..');
const require=createRequire(path.join(client,'package.json'));
const archive=path.resolve(client,'../../.private/windows387-install-20261102-073440/app-3.8.6.asar');
const sha=bytes=>createHash('sha256').update(bytes).digest('hex');
let runtime;
export async function legacy386Runtime(){
  if(runtime)return runtime;
  const asar=require('@electron/asar'),packageBytes=await fs.readFile(archive);
  if(sha(packageBytes)!=='172acd80854264698ab047f5068622af6e50e51e2adf4c02cd5482a3abaf99c4')throw Error('HISTORICAL_PACKAGE_PIN_MISMATCH');
  const worker=asar.extractFile(archive,'dist\\assets\\worker-6b1794ce.js'),source=worker.toString();
  if(sha(worker)!=='6b1794cec22faf905b358d1d7fed7f86cb861abfadfd96f97ac60766614f2e95')throw Error('HISTORICAL_WORKER_PIN_MISMATCH');
  const fsStart=source.indexOf('const en=class{'),fsEnd=source.indexOf(';let tn=',fsStart),fileStart=source.indexOf('let tn=',fsEnd),fileEnd=source.indexOf('let un,ln,cn,dn=',fileStart);
  if(fsStart<0||fsEnd<0||fileStart<0||fileEnd<0)throw Error('HISTORICAL_CLASS_BOUNDARY_MISSING');
  const fsSource=source.slice(fsStart+'const en='.length,fsEnd),fileSource=source.slice(fileStart,fileEnd);
  const SQLiteFS=Function('Xt','Zt','return ('+fsSource+')')(63,44),File=Function(fileSource+';return sn;')();
  const wasm=asar.extractFile(archive,'dist\\sql-wasm.wasm');
  if(sha(wasm)!=='806886f413da1a6acc1db373285adda1b6750c8d434c75a5a6042af0a65cbafd')throw Error('HISTORICAL_WASM_PIN_MISMATCH');
  runtime={SQLiteFS,File,wasm,pins:{archiveSHA256:sha(packageBytes),workerSHA256:sha(worker),sqliteFSSHA256:sha(fsSource),fileSHA256:sha(fileSource),sqlWasmSHA256:sha(wasm)}};
  return runtime;
}

export async function produceLegacy386({databasePath,schema,statements}){
  if(!/^\/sql\/(?:\.\/)?[A-Za-z0-9_.-]{1,512}\.sqlite$/.test(databasePath))throw Error('SYNTHETIC_PATH_REJECTED');
  // sql.js caches its first Module in its CommonJS closure. Compile a fresh
  // trusted module wrapper so each fixture has a separate FS mount/namespace.
  const historical=await legacy386Runtime(),sqlEntry=require.resolve('@jlongster/sql.js'),sqlModule={exports:{}};
  Function('require','module','exports','__filename','__dirname',await fs.readFile(sqlEntry,'utf8'))(require,sqlModule,sqlModule.exports,sqlEntry,path.dirname(sqlEntry));
  const SQL=await sqlModule.exports({wasmBinary:historical.wasm}),produced=[];
  const backend={createFile(filename){
    const blocks=new Map(),row={filename,blocks},ops={
      open(){},close(){},lock(){return true;},unlock(){return true;},delete(){throw Error('UNEXPECTED_SYNTHETIC_DELETE');},
      readMeta(){const meta=blocks.get(-1);if(!meta)return null;const first=blocks.get(0);return{size:meta.size,blockSize:first?new DataView(first).getUint16(16,false):undefined};},
      writeMeta(meta){blocks.set(-1,{size:meta.size});},
      readBlocks(positions,blockSize){return positions.map(pos=>({pos,data:blocks.get(Math.round(pos/blockSize))?.slice(0)||new ArrayBuffer(blockSize)}));},
      writeBlocks(writes,blockSize){for(const write of writes)blocks.set(Math.round(write.pos/blockSize),write.data.slice(0));}
    };
    produced.push(row);return new historical.File(filename,ops);
  }};
  const sqlFS=new historical.SQLiteFS(SQL.FS,backend);SQL.register_for_idb(sqlFS);SQL.FS.mkdir('/sql');SQL.FS.mount(sqlFS,{},'/sql');
  const db=new SQL.Database(databasePath,{filename:true});let exported;
  try{db.exec('PRAGMA page_size=8192;PRAGMA journal_mode=MEMORY;');for(const statement of schema)db.exec(statement);for(const {sql,values}of statements)db.run(sql,values);if(db.exec('PRAGMA integrity_check')[0]?.values[0]?.[0]!=='ok')throw Error('SYNTHETIC_SQLITE_INTEGRITY_FAILED');exported=db.export();}finally{db.close();}
  if(produced.length!==1)throw Error('SYNTHETIC_PRODUCER_FILE_COUNT_REJECTED');
  const {filename,blocks}=produced[0],pages=[...blocks.entries()].sort((a,b)=>a[0]-b[0]).map(([key,value])=>({key,value}));
  return{raw:{name:filename.replace(/\//g,'-'),version:2,stores:['data'],pages},sqliteBytes:exported,sqliteSHA256:sha(exported),databasePath,capturedBasename:filename,producerPins:historical.pins,persistenceBoundary:'owned RAM captures original FileOps writeMeta/writeBlocks; no handcrafted names/pages'};
}
