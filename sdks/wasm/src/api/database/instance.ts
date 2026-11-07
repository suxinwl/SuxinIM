/* eslint-disable @typescript-eslint/no-unsafe-member-access */
/* eslint-disable @typescript-eslint/no-unsafe-call */
import initSqlJs, { Database, SqlJsStatic } from '@jlongster/sql.js';
import { SQLiteFS } from 'absurd-sql-optimized';
import IndexedDBBackend from 'absurd-sql-optimized/dist/indexeddb-backend';
import { durableEnabled, restoreMemoryDatabase, attachMemoryDatabase, detachMemoryDatabase } from './durable';
(self as any).$RefreshReg$ = () => {};
(self as any).$RefreshSig$ = () => () => {};

let instance: Promise<Database> | undefined;
let SQL: SqlJsStatic | undefined;
let mounted = false;

async function InitializeDB(filePath: string, sqlWasmPath = '/sql-wasm.wasm') {
  if (!SQL) {
    SQL = await initSqlJs({ locateFile: () => sqlWasmPath });
  }
  if (durableEnabled()) {
    const restored = await restoreMemoryDatabase();
    return attachMemoryDatabase(new SQL.Database(restored.bytes), restored.entries, restored.tombstones,restored.suppression);
  }
  if (!mounted) {
    const sqlFS = new SQLiteFS(SQL.FS, new IndexedDBBackend());

    SQL.register_for_idb(sqlFS);
    SQL.FS.mkdir('/sql');
    SQL.FS.mount(sqlFS, {}, '/sql');
    mounted = true;
  }

  const path = `/sql/${filePath}`;

  const db = new SQL.Database(path, { filename: true });

  if (typeof SharedArrayBuffer === 'undefined') {
    // @ts-ignore
    const stream = SQL.FS.open(path, 'a+');
    await stream.node.contents.readIfFallback();
    // @ts-ignore
    SQL.FS.close(stream);
  }

  db.exec(`
    PRAGMA page_size=8192;
    PRAGMA journal_mode=MEMORY;
  `);

  return db;
}

export function getInstance(
  filePath?: string,
  sqlWasmPath?: string
): Promise<Database> {
  if (instance) {
    return instance;
  }

  if (!filePath) {
    throw new Error('must speciefic database file');
  }

  instance = new Promise<Database>((resolve, reject) => {
    const db = InitializeDB(filePath, sqlWasmPath);
    db.then(res => resolve(res)).catch(err => reject(err));
  });

  return instance;
}

export async function resetInstance() {
  if (!instance) {
    return;
  }

  const db = await instance;

  db.close();
  if (durableEnabled()) detachMemoryDatabase();
  instance = undefined;
}
