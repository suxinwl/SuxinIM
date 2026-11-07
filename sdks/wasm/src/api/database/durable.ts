/* The desktop host owns encryption and durable storage. The worker never
 * receives a disk path or a master key and never mounts IndexedDB in this mode. */
import type { Database } from '@jlongster/sql.js';
import {converSqlExecResult} from '../../utils/value';
import {formatResponse} from '../../utils/response';
type Statement = { sql: string; bindings?: unknown[] };
type Tombstone = { conversationIDs: string[] | null; beforeMS: number; messageIDs?:string[] };
export type MessageSuppression={conversationID:string;clientMsgID:string;sendTime:number;mode:'delete'|'archive'};
type Operation = { id: string; statements: Statement[]; before?: string; started: boolean };
let enabled = false;
let database: Database | undefined;
let raw: Database | undefined;
let current: Operation | undefined;
let queue: Promise<unknown> = Promise.resolve();
let sequence = 0;
const pending = new Map<string, { resolve: (value: any) => void; reject: (error: Error) => void }>();
const quote = (value: string) => "'" + value.replace(/'/g, "''") + "'";
const identifier = (value: string) => '"' + value.replace(/"/g, '""') + '"';
function b64(bytes: Uint8Array) { let value = ''; for (let i = 0; i < bytes.length; i += 8192) value += String.fromCharCode(...bytes.subarray(i, i + 8192)); return btoa(value); }
function from64(value: string) { return Uint8Array.from(atob(value), c => c.charCodeAt(0)); }
function encode(value: any): any { return value instanceof Uint8Array ? { $bytes: b64(value) } : Array.isArray(value) ? value.map(encode) : value; }
function decode(value: any): any { return value?.$bytes !== undefined ? from64(value.$bytes) : Array.isArray(value) ? value.map(decode) : value; }
function send(operation: string, data?: unknown): Promise<any> {
  const id = crypto.randomUUID();
  return new Promise((resolve, reject) => { pending.set(id, { resolve, reject }); self.postMessage({ putyStore: 'request', id, operation, data }); });
}
self.addEventListener('message', (event: MessageEvent) => {
  if (event.data?.putyStore === 'configure') enabled = event.data.enabled === true;
  if (event.data?.putyStore !== 'response') return;
  const request = pending.get(event.data.id); if (!request) return; pending.delete(event.data.id);
  event.data.error ? request.reject(new Error(event.data.error)) : request.resolve(event.data.data);
});
export function durableEnabled() { return enabled; }
export function detachMemoryDatabase() { raw = undefined; database = undefined; triggered.clear(); triggerSchemaVersion = -1; }
export async function restoreMemoryDatabase(): Promise<{ bytes?: Uint8Array; entries: any[]; tombstones: Tombstone[]; suppression:MessageSuppression[] }> {
  if (!enabled) return { entries: [], tombstones: [],suppression:[] };
  const value = await send('sdk-restore'); sequence = value.sequence;
  return { bytes: value.snapshot ? from64(value.snapshot) : undefined, entries: value.entries, tombstones: value.tombstones || [],suppression:value.suppression || [] };
}
function rows(db: Database, sql: string): Record<string, any>[] {
  const result = db.exec(sql)[0]; return result ? result.values.map(values => result.columns.reduce<Record<string, any>>((row, key, index) => { row[key] = values[index]; return row; }, {})) : [];
}
function state(db: Database) { return JSON.stringify([db.exec('SELECT total_changes()')[0]?.values[0][0], db.exec('PRAGMA schema_version')[0]?.values[0][0], db.exec('PRAGMA user_version')[0]?.values[0][0]]); }
// Split only at SQL statement boundaries, preserving quoted payloads/comments.
export function splitSQL(sql: string): string[] {
  const result: string[] = []; let begin = 0, mode = '', comment = '';
  for (let i = 0; i < sql.length; i++) {
    const c = sql[i], next = sql[i + 1];
    if (comment === '--') { if (c === '\n') comment = ''; continue; }
    if (comment === '/*') { if (c === '*' && next === '/') { comment = ''; i++; } continue; }
    if (mode) { if (c === mode) { if (next === mode) i++; else mode = ''; } continue; }
    if (c === '-' && next === '-') { comment = '--'; i++; continue; }
    if (c === '/' && next === '*') { comment = '/*'; i++; continue; }
    if (c === "'" || c === '"' || c === '`') { mode = c; continue; }
    if (c === '[') { mode = ']'; continue; }
    if (c === ';') { if (sql.slice(begin, i).trim()) result.push(sql.slice(begin, i)); begin = i + 1; }
  }
  if (sql.slice(begin).trim()) result.push(sql.slice(begin)); return result;
}
function normalizeTransactions(sql: string) {
  return splitSQL(sql).map(part => /^\s*BEGIN(?:\s+(?:TRANSACTION|DEFERRED|IMMEDIATE|EXCLUSIVE))?\s*$/i.test(part) ? 'SAVEPOINT puty_inner' : /^\s*(?:COMMIT|END)(?:\s+TRANSACTION)?\s*$/i.test(part) ? 'RELEASE puty_inner' : /^\s*ROLLBACK(?:\s+TRANSACTION)?\s*$/i.test(part) ? 'ROLLBACK TO puty_inner; RELEASE puty_inner' : part).join(';');
}
function capture(sql: string, bindings?: unknown[]) { if (current) current.statements.push({ sql, ...(bindings ? { bindings: encode(bindings) } : {}) }); }
export function attachMemoryDatabase(db: Database, entries: any[], tombstones: Tombstone[],suppression:MessageSuppression[]=[]): Database {
  raw = db;
  triggered.clear(); triggerSchemaVersion = -1;
  db.exec('PRAGMA journal_mode=MEMORY; PRAGMA temp_store=MEMORY; PRAGMA secure_delete=ON;');
  db.exec(`CREATE TABLE IF NOT EXISTS puty_archive_outbox(id INTEGER PRIMARY KEY AUTOINCREMENT, conversation_id TEXT NOT NULL, kind TEXT NOT NULL, row_json TEXT NOT NULL);
    CREATE TABLE IF NOT EXISTS puty_cold(conversation_id TEXT NOT NULL,client_msg_id TEXT NOT NULL,PRIMARY KEY(conversation_id,client_msg_id));
    CREATE TABLE IF NOT EXISTS puty_volatile(conversation_id TEXT PRIMARY KEY);
    CREATE TABLE IF NOT EXISTS puty_local_tombstones(conversation_id TEXT NOT NULL,before_ms INTEGER NOT NULL,PRIMARY KEY(conversation_id,before_ms));
    CREATE TABLE IF NOT EXISTS puty_local_deleted(conversation_id TEXT NOT NULL,client_msg_id TEXT NOT NULL,PRIMARY KEY(conversation_id,client_msg_id));
    CREATE TABLE IF NOT EXISTS puty_runtime(singleton INTEGER PRIMARY KEY, suppress_archive INTEGER NOT NULL,hidden_count INTEGER NOT NULL DEFAULT 0); INSERT OR IGNORE INTO puty_runtime(singleton,suppress_archive) VALUES(1,0);`);
  if(!rows(db,'PRAGMA table_info(puty_runtime)').some(v=>v.name==='hidden_count'))db.exec('ALTER TABLE puty_runtime ADD COLUMN hidden_count INTEGER NOT NULL DEFAULT 0');
  for (const entry of entries) {
    db.exec('SAVEPOINT puty_replay');
    try { for (const command of entry.statements as Statement[]) { if (command.bindings) db.run(command.sql, decode(command.bindings)); else db.exec(command.sql); } db.exec('RELEASE puty_replay'); }
    catch (error) { db.exec('ROLLBACK TO puty_replay; RELEASE puty_replay'); throw error; }
  }
  // Exact IDs are never widened to a whole conversation/time range. Private
  // archive exclusions remain readable in the active hot DB until SDK expiry.
  for (const t of tombstones) for (const id of t.conversationIDs || ['*']) {
    if(t.messageIDs){if(id==='*')throw new Error('Exact deletion requires a conversation');for(const message of t.messageIDs)db.run('INSERT OR IGNORE INTO puty_local_deleted VALUES(?,?)',[id,message]);}
    else db.run('INSERT OR IGNORE INTO puty_local_tombstones VALUES(?,?)', [id, t.beforeMS]);
  }
  for(const item of suppression){
    if(!item || !['delete','archive'].includes(item.mode) || typeof item.conversationID!=='string' || !item.conversationID || typeof item.clientMsgID!=='string' || !item.clientMsgID || !Number.isSafeInteger(item.sendTime) || item.sendTime<0)throw new Error('Invalid message suppression');
    if(item.mode==='delete')db.run('INSERT OR IGNORE INTO puty_local_deleted VALUES(?,?)',[item.conversationID,item.clientMsgID]);
  }
  installArchiveTriggers();
  database = new Proxy(db, {
    get(target, name) {
      if (name === 'exec' || name === 'run') return (sql: string, bindings?: any) => {
        const actual = normalizeTransactions(sql), result = (target[name] as any).call(target, actual, bindings);
        capture(actual, bindings); installArchiveTriggers(); return result;
      };
      if (name === 'prepare') return (sql: string) => {
        const statement = target.prepare(sql); let bindings: unknown[] | undefined, recorded = false;
        return new Proxy(statement, { get(stmt, property) {
          if (property === 'bind') return (value: unknown[]) => { bindings = value; recorded = false; return stmt.bind(value as any); };
          if (property === 'step') return () => { const result = stmt.step(); if (!recorded) { capture(sql, bindings); recorded = true; } return result; };
          const value = (stmt as any)[property]; return typeof value === 'function' ? value.bind(stmt) : value;
        } });
      };
      const value = (target as any)[name]; return typeof value === 'function' ? value.bind(target) : value;
    },
  });
  if (current) start(); return database;
}
const triggered = new Set<string>();
let triggerSchemaVersion = -1;
function ephemeralCondition(table:string,alias='') {
  if(!raw)return '0';const fields=rows(raw,`PRAGMA table_info(${identifier(table)})`).map(v=>v.name),prefix=alias?alias+'.':'';
  const burn=fields.includes('attached_info')?`CASE WHEN json_valid(${prefix}attached_info) THEN COALESCE(json_extract(${prefix}attached_info,'$.isPrivateChat'),0) ELSE 0 END=1`:'0';
  const conversationFields=rows(raw,'PRAGMA table_info(local_conversations)').map(v=>v.name);
  const destruct=conversationFields.includes('is_msg_destruct')?`EXISTS(SELECT 1 FROM local_conversations WHERE conversation_id=${quote(table.slice('chat_logs_'.length))} AND is_msg_destruct=1)`:'0';
  return `(${burn} OR ${destruct})`;
}
function installArchiveTriggers() {
  if (!raw) return;
  const schemaVersion = Number(raw.exec('PRAGMA schema_version')[0]?.values[0][0]);
  if (schemaVersion === triggerSchemaVersion) return;
  for (const row of rows(raw, "SELECT name FROM sqlite_master WHERE type='table' AND name GLOB 'chat_logs_*'")) {
    const table = String(row.name); if (triggered.has(table)) continue;
    const fields = rows(raw, `PRAGMA table_info(${identifier(table)})`).map(v => String(v.name));
    if (!fields.includes('client_msg_id') || !fields.includes('send_time')) continue;
    const conversationID = table.slice('chat_logs_'.length), quoted = identifier(table), condition = `(SELECT suppress_archive FROM puty_runtime WHERE singleton=1)=0`;
    const hidden = `(EXISTS(SELECT 1 FROM puty_local_tombstones WHERE conversation_id IN('*',${quote(conversationID)}) AND NEW.send_time<before_ms) OR EXISTS(SELECT 1 FROM puty_local_deleted WHERE conversation_id=${quote(conversationID)} AND client_msg_id=NEW.client_msg_id))`;
    const ephemeral=ephemeralCondition(table,'NEW'),recalled=fields.includes('content_type')?'NEW.content_type=2101':'0';
    const deleted=`(${hidden} OR ${recalled}${fields.includes('status')?' OR NEW.status>=4':''})`;
    const json = `json_object(${fields.reduce<string[]>((items, name) => items.concat([quote(name), `NEW.${identifier(name)}`]), []).join(',')})`;
    const thinNew=`json_object('client_msg_id',NEW.client_msg_id,'send_time',NEW.send_time,'__ephemeral',1)`;
    const deletedNew=`json_object('client_msg_id',NEW.client_msg_id,'send_time',NEW.send_time,'status',4)`;
    const erase=fields.filter(field=>['content','local_ex','attached_info','ex','sender_nick_name','sender_face_url'].includes(field)).map(field=>identifier(field)+"=''");
    const stale=erase.map(value=>value.replace("=''","<>''")).concat(fields.includes('status')?['status<>4']:[]).join(' OR ')||'0';
    if(fields.includes('status'))erase.unshift('status=4');
    const sanitize=erase.length?`UPDATE ${quoted} SET ${erase.join(',')} WHERE client_msg_id=NEW.client_msg_id AND ${hidden} AND (${stale});`:'';
    const clearLatest=rows(raw,'PRAGMA table_info(local_conversations)').some(v=>v.name==='latest_msg')?`UPDATE local_conversations SET latest_msg='',unread_count=0 WHERE conversation_id=${quote(conversationID)} AND latest_msg<>'' AND json_valid(latest_msg) AND COALESCE(json_extract(latest_msg,'$.clientMsgID'),json_extract(latest_msg,'$.client_msg_id'))=NEW.client_msg_id AND EXISTS(SELECT 1 FROM puty_local_deleted WHERE conversation_id=${quote(conversationID)} AND client_msg_id=NEW.client_msg_id);`:'';
    const priorDeleted=[...(fields.includes('status')?['status>=4']:[]),...(fields.includes('content_type')?['content_type=2101']:[])].join(' OR ')||'0';
    // Upgrade explicit old SDK deletion metadata too. Queue thin IDs so this
    // first open durably transfers the new masks to the encrypted host index.
    const backfill=`INSERT INTO puty_archive_outbox(conversation_id,kind,row_json) SELECT ${quote(conversationID)},'upsert',json_object('client_msg_id',client_msg_id,'send_time',send_time,'status',4) FROM ${quoted} WHERE (${priorDeleted}) AND NOT EXISTS(SELECT 1 FROM puty_local_deleted d WHERE d.conversation_id=${quote(conversationID)} AND d.client_msg_id=${quoted}.client_msg_id);
      INSERT OR IGNORE INTO puty_local_deleted SELECT ${quote(conversationID)},client_msg_id FROM ${quoted} WHERE ${priorDeleted};`;
    raw.exec(backfill);capture(backfill);
    const delta=fields.filter(field=>!['client_msg_id','send_time'].includes(field)).map(field=>`SELECT ${quote(field)} AS k,NEW.${identifier(field)} AS v WHERE NEW.${identifier(field)} IS NOT OLD.${identifier(field)}`).join(' UNION ALL ');
    const patch=`(SELECT json_group_object(k,v) FROM (SELECT 'client_msg_id' AS k,NEW.client_msg_id AS v UNION ALL SELECT 'send_time',NEW.send_time UNION ALL SELECT '__patch',1 ${delta?'UNION ALL '+delta:''}))`;
    const thin = `json_object('client_msg_id',OLD.client_msg_id,'send_time',OLD.send_time,'status',4)`;
    const triggerSQL = `DROP TRIGGER IF EXISTS ${identifier('puty_insert_' + table)};DROP TRIGGER IF EXISTS ${identifier('puty_update_' + table)};DROP TRIGGER IF EXISTS ${identifier('puty_delete_' + table)};
      CREATE TRIGGER ${identifier('puty_insert_' + table)} AFTER INSERT ON ${quoted} WHEN ${condition} BEGIN
      INSERT OR IGNORE INTO puty_local_deleted SELECT ${quote(conversationID)},NEW.client_msg_id WHERE ${deleted};
      INSERT INTO puty_archive_outbox(conversation_id,kind,row_json) SELECT ${quote(conversationID)},'upsert',CASE WHEN ${deleted} THEN ${deletedNew} WHEN ${ephemeral} THEN ${thinNew} ELSE ${json} END;
      UPDATE puty_runtime SET hidden_count=hidden_count+1 WHERE singleton=1 AND ${hidden};
      ${sanitize}${clearLatest} END;
      CREATE TRIGGER ${identifier('puty_update_' + table)} AFTER UPDATE ON ${quoted} WHEN ${condition} BEGIN
      INSERT OR IGNORE INTO puty_local_deleted SELECT ${quote(conversationID)},NEW.client_msg_id WHERE ${deleted};
      INSERT INTO puty_archive_outbox(conversation_id,kind,row_json) SELECT ${quote(conversationID)},'upsert',CASE WHEN ${deleted} THEN ${deletedNew} WHEN ${ephemeral} THEN ${thinNew} ELSE ${patch} END;
      UPDATE puty_runtime SET hidden_count=hidden_count+1 WHERE singleton=1 AND ${hidden};${sanitize}${clearLatest} END;
      CREATE TRIGGER ${identifier('puty_delete_' + table)} AFTER DELETE ON ${quoted} WHEN ${condition} BEGIN
      INSERT OR IGNORE INTO puty_local_deleted VALUES(${quote(conversationID)},OLD.client_msg_id);
      INSERT INTO puty_archive_outbox(conversation_id,kind,row_json) VALUES(${quote(conversationID)},'delete',${thin});
      ${clearLatest.replace(/NEW\.client_msg_id/g,'OLD.client_msg_id')}
      DELETE FROM puty_cold WHERE conversation_id=${quote(conversationID)} AND client_msg_id=OLD.client_msg_id; END;`;
    raw.exec(triggerSQL); capture(triggerSQL); triggered.add(table);
    // Older snapshots may contain payload for IDs deleted after that snapshot.
    // Sanitize immediately, before any SDK read or latest-message hydration.
    const restoreHidden=`EXISTS(SELECT 1 FROM puty_local_deleted d WHERE d.conversation_id=${quote(conversationID)} AND d.client_msg_id=${quoted}.client_msg_id)`;
    if(erase.length)raw.exec(`UPDATE ${quoted} SET ${erase.join(',')} WHERE ${restoreHidden} AND (${stale})`);
  }
  if(rows(raw,'PRAGMA table_info(local_conversations)').some(v=>v.name==='latest_msg')){
    const blocked=`(EXISTS(SELECT 1 FROM puty_local_tombstones WHERE conversation_id IN('*',NEW.conversation_id) AND CAST(json_extract(NEW.latest_msg,'$.sendTime') AS INTEGER)<before_ms) OR EXISTS(SELECT 1 FROM puty_local_deleted WHERE conversation_id=NEW.conversation_id AND client_msg_id=COALESCE(json_extract(NEW.latest_msg,'$.clientMsgID'),json_extract(NEW.latest_msg,'$.client_msg_id'))))`;
    const hidden=`NEW.latest_msg<>'' AND json_valid(NEW.latest_msg) AND ${blocked}`;
    const sql=`DROP TRIGGER IF EXISTS puty_conversation_insert;DROP TRIGGER IF EXISTS puty_conversation_update;
    CREATE TRIGGER puty_conversation_insert AFTER INSERT ON local_conversations WHEN ${hidden} BEGIN UPDATE local_conversations SET latest_msg='',unread_count=0 WHERE conversation_id=NEW.conversation_id;END;
    CREATE TRIGGER puty_conversation_update AFTER UPDATE OF latest_msg ON local_conversations WHEN ${hidden} BEGIN UPDATE local_conversations SET latest_msg='',unread_count=0 WHERE conversation_id=NEW.conversation_id;END;`;
    raw.exec(sql);capture(sql);
    raw.exec("UPDATE local_conversations SET latest_msg='',unread_count=0 WHERE latest_msg<>'' AND json_valid(latest_msg) AND EXISTS(SELECT 1 FROM puty_local_deleted d WHERE d.conversation_id=local_conversations.conversation_id AND d.client_msg_id=COALESCE(json_extract(latest_msg,'$.clientMsgID'),json_extract(latest_msg,'$.client_msg_id')))");
  }
  triggerSchemaVersion = Number(raw.exec('PRAGMA schema_version')[0]?.values[0][0]);
}
const archiveReads=new Set(['getMessage','getMessageBySeq','getMultipleMessage','getMessagesByClientMsgIDs','getMessagesBySeqs','getMessageList','getMessageListNoTime','getLatestValidServerMessage','getLatestActiveMessage','searchMessageByContentType','searchMessageByKeyword','searchMessageByContentTypeAndKeyword','searchAllMessageByContentType']);
async function archiveRead(name:string,args:any[]) {
  if(!raw)throw new Error('本地加密档案尚未打开');
  const table='chat_logs_'+String(args[0]),fields=rows(raw,`PRAGMA table_info(${identifier(table)})`);
  const workingRows=fields.length?rows(raw,`SELECT * FROM ${identifier(table)} WHERE status<4 AND ${ephemeralCondition(table)}`):[];
  const found=await send('archive-query',{name,args,workingRows}) as Record<string,unknown>[];
  const columns=found.length?Object.keys(found[0]):[];
  const converted=converSqlExecResult({columns,values:found.map(row=>columns.map(key=>row[key]))} as any,'CamelCase',['isRead','isReact','isExternalExtensions']);
  if(['getMessage','getMessageBySeq','getLatestValidServerMessage'].includes(name))return converted.length?formatResponse(converted[0]):formatResponse('',10002,'no local message');
  return formatResponse(converted);
}
function pruneHot() {
  if(!raw)return false;let changed=false;
  const cutoff=Date.now()-90*24*60*60*1000;
  for(const row of rows(raw,"SELECT name FROM sqlite_master WHERE type='table' AND name GLOB 'chat_logs_*'")) {
    const table=String(row.name),fields=rows(raw,`PRAGMA table_info(${identifier(table)})`).map(v=>v.name);
    if(!['send_time','status','seq','is_read','content','client_msg_id'].every(v=>fields.includes(v)))continue;
    const self=fields.includes('send_id') && rows(raw,'PRAGMA table_info(local_users)').some(v=>v.name==='user_id')?' OR send_id=(SELECT user_id FROM local_users LIMIT 1)':'';
    const eligible=`send_time<${cutoff} AND seq>0 AND status IN(2,3) AND (is_read=1${self}) AND NOT ${ephemeralCondition(table)} AND (content<>'' OR local_ex<>'')`;
    if(!Number(rows(raw,`SELECT COUNT(*) AS count FROM ${identifier(table)} WHERE ${eligible}`)[0]?.count))continue;
    const erase=['content','local_ex','attached_info','ex'].filter(field=>fields.includes(field)).map(field=>identifier(field)+"=''").join(',');
    raw.exec('UPDATE puty_runtime SET suppress_archive=1 WHERE singleton=1');
    raw.exec(`INSERT OR IGNORE INTO puty_cold SELECT ${quote(table.slice('chat_logs_'.length))},client_msg_id FROM ${identifier(table)} WHERE ${eligible};UPDATE ${identifier(table)} SET ${erase} WHERE ${eligible};`);
    raw.exec('UPDATE puty_runtime SET suppress_archive=0 WHERE singleton=1');changed=true;
  }
  return changed;
}
async function reconcileVolatileConversations(){
  if(!raw || !rows(raw,'PRAGMA table_info(local_conversations)').some(v=>v.name==='is_msg_destruct'))return;
  const selected=rows(raw,'SELECT conversation_id FROM local_conversations WHERE is_msg_destruct=1 AND conversation_id NOT IN(SELECT conversation_id FROM puty_volatile)');
  for(const conversation of selected){
    const id=String(conversation.conversation_id);if(!/^[\w-]{1,256}$/.test(id))throw new Error('Invalid volatile conversation');
    const table='chat_logs_'+id,fields=rows(raw,`PRAGMA table_info(${identifier(table)})`).map(v=>String(v.name));if(!fields.length)continue;
    // Cold working rows retain sequence metadata only. Hydrate their payloads
    // into encrypted working storage before retiring the permanent month copy,
    // so the SDK's original destruction timers remain authoritative.
    const cold=rows(raw,`SELECT client_msg_id FROM puty_cold WHERE conversation_id=${quote(id)}`).map(v=>String(v.client_msg_id));
    raw.exec('UPDATE puty_runtime SET suppress_archive=1 WHERE singleton=1');capture('UPDATE puty_runtime SET suppress_archive=1 WHERE singleton=1');
    for(let offset=0;offset<cold.length;offset+=500){
      const found=await send('archive-query',{name:'getMessagesByClientMsgIDs',args:[id,cold.slice(offset,offset+500)],workingRows:[]}) as Record<string,any>[];
      for(const row of found){const payload=['content','local_ex','attached_info','ex'].filter(field=>fields.includes(field) && row[field]!==undefined);if(!payload.length)continue;
        const sql=`UPDATE ${identifier(table)} SET ${payload.map(field=>identifier(field)+'=?').join(',')} WHERE client_msg_id=? AND status<4`,bindings=[...payload.map(field=>row[field]),row.client_msg_id];raw.run(sql,bindings);capture(sql,bindings);
      }
    }
    const retire=`INSERT INTO puty_archive_outbox(conversation_id,kind,row_json) SELECT ${quote(id)},'upsert',json_object('client_msg_id',client_msg_id,'send_time',send_time,'__ephemeral',1) FROM ${identifier(table)};
      DELETE FROM puty_cold WHERE conversation_id=${quote(id)};INSERT OR IGNORE INTO puty_volatile VALUES(${quote(id)});UPDATE puty_runtime SET suppress_archive=0 WHERE singleton=1;`;
    raw.exec(retire);capture(retire);
  }
  if(rows(raw,'SELECT 1 FROM puty_volatile WHERE conversation_id NOT IN(SELECT conversation_id FROM local_conversations WHERE is_msg_destruct=1) LIMIT 1').length){const reset='DELETE FROM puty_volatile WHERE conversation_id NOT IN(SELECT conversation_id FROM local_conversations WHERE is_msg_destruct=1)';raw.exec(reset);capture(reset);}
}

function start() {
  if (!raw || !current || current.started) return;
  current.before = state(raw); raw.exec('SAVEPOINT puty_durable'); current.started = true; installArchiveTriggers();
  capture('CREATE TABLE IF NOT EXISTS puty_volatile(conversation_id TEXT PRIMARY KEY)');
}
function success(value: any) { if (value && typeof value === 'object') return !value.errCode; if (typeof value !== 'string') return true; try { const parsed = JSON.parse(value); return !parsed?.errCode; } catch { return true; } }
function snapshotBytes() { if(!raw)throw new Error('Local database closed');const result=raw.export();raw.exec('PRAGMA journal_mode=MEMORY;PRAGMA temp_store=MEMORY;PRAGMA secure_delete=ON;');return result; }
export function durableMethod(name: string, fn: (...args: any[]) => any) {
  if (['fileMapSet', 'fileMapClear', 'wasmOpen', 'wasmClose', 'wasmRead', 'setSqlWasmPath'].includes(name)) return fn;
  return (...args: any[]) => {
    if (!enabled) return fn(...args);
    const operation = async () => {
      if (name === 'putyLocalCleanup' || name==='putyLocalCleanupPreview') return fn(...args);
      if(name==='putyLocalCheckpoint'){if(!raw)throw new Error('加密工作库尚未打开');await send('sdk-checkpoint',{snapshot:b64(snapshotBytes())});return {checkpointed:true};}
      if(archiveReads.has(name)){try{return await archiveRead(name,args);}catch(error){return formatResponse(undefined,10001,error);}}
      if (name === 'closeDB') { if (raw) await send('sdk-checkpoint', { snapshot: b64(snapshotBytes()) }); const response = await fn(...args); raw = undefined; database = undefined; triggered.clear(); return response; }
      current = { id: crypto.randomUUID(), statements: [], started: false }; start();
      try {
        const result = await fn(...args);
        if (!raw || !current.started) return result;
        if (!success(result)) { raw.exec('ROLLBACK TO puty_durable; RELEASE puty_durable'); triggered.clear(); triggerSchemaVersion = -1; return result; }
        await reconcileVolatileConversations();
        const changed = current.before !== state(raw) || rows(raw,'SELECT 1 FROM puty_archive_outbox LIMIT 1').length>0;
        let released=false;
        if (changed) {
          const events = rows(raw, 'SELECT conversation_id,kind,row_json FROM puty_archive_outbox ORDER BY id').map(v => ({ conversationID: v.conversation_id, kind: v.kind, row: JSON.parse(v.row_json) }));
          if (events.length) { raw.exec('DELETE FROM puty_archive_outbox'); capture('DELETE FROM puty_archive_outbox'); }
          const hiddenCount=Number(rows(raw,'SELECT hidden_count FROM puty_runtime WHERE singleton=1')[0]?.hidden_count || 0);
          const compact=hiddenCount>0 || events.some(event=>event.kind==='delete' || event.row.__ephemeral || Number(event.row.status)>=4);
          if(hiddenCount)raw.exec('UPDATE puty_runtime SET hidden_count=0 WHERE singleton=1');
          let snapshot:string|undefined;
          if(compact){raw.exec('RELEASE puty_durable');current.started=false;released=true;snapshot=b64(snapshotBytes());}
          const commit = await send('sdk-commit', { opID: current.id, statements: compact?[]:current.statements, events, ...(snapshot?{snapshot}:{}) }); sequence = commit.sequence;
        }
        if(!released)raw.exec('RELEASE puty_durable');
        if(changed){const pruned=pruneHot();if(pruned || sequence%128===0)await send('sdk-checkpoint',{snapshot:b64(snapshotBytes())});}
        return result;
      } catch (error) {
        if (raw && current?.started) try { raw.exec('ROLLBACK TO puty_durable; RELEASE puty_durable'); triggered.clear(); triggerSchemaVersion = -1; } catch { /* The durable commit may already have been released. */ }
        return { data: '{}', errCode: 10001, errMsg: error instanceof Error ? error.message : '本地加密存储失败' };
      } finally { current = undefined; }
    };
    const next = queue.then(operation); queue = next.catch(() => {}); return next;
  };
}
type LocalCleanupScope={conversationIDs?:string[];beforeMS?:number;beforeDate?:string};
function cleanupScope(input:LocalCleanupScope){
  let beforeMS=input.beforeMS ?? Date.now()+1;
  if(input.beforeDate){if(!/^\d{4}-\d{2}-\d{2}$/.test(input.beforeDate))throw new Error('清理日期无效');beforeMS=Date.parse(input.beforeDate+'T00:00:00+08:00');if(new Date(beforeMS+8*3600000).toISOString().slice(0,10)!==input.beforeDate)throw new Error('清理日期无效');}
  if(!Number.isFinite(beforeMS) || beforeMS<0 || input.conversationIDs?.some(id=>!/^[\w-]{1,256}$/.test(id)))throw new Error('清理范围无效');return {conversationIDs:input.conversationIDs,beforeMS};
}
function cleanupInventory(scope:{conversationIDs?:string[];beforeMS:number}){
  if(!raw)throw new Error('加密档案尚未打开');const selected:{conversationID:string;clientMsgID:string;sendTime:number}[]=[];
  for(const table of rows(raw,"SELECT name FROM sqlite_master WHERE type='table' AND name GLOB 'chat_logs_*'")){
    const name=String(table.name),id=name.slice('chat_logs_'.length);if(scope.conversationIDs?.length && !scope.conversationIDs.includes(id))continue;
    for(const row of rows(raw,`SELECT client_msg_id,send_time FROM ${identifier(name)} WHERE status<4 AND send_time<${scope.beforeMS}`))selected.push({conversationID:id,clientMsgID:String(row.client_msg_id),sendTime:Number(row.send_time)});
  }return selected;
}
export async function localCleanupPreview(input:LocalCleanupScope){
  if(!enabled || !raw)throw new Error('加密档案尚未打开');const scope=cleanupScope(input);return send('cleanup-preview',{scope,workingMessages:cleanupInventory(scope)});
}
export async function localCleanup(input: LocalCleanupScope) {
  if (!enabled || !raw) throw new Error('加密档案尚未打开');
  const db = raw,scope=cleanupScope(input),maskBeforeMS=Math.min(scope.beforeMS,Date.now()+1),workingMessages=cleanupInventory(scope);
  try {
    db.exec('BEGIN');
    // The requested selection may include today's future boundary or clock-skewed
    // existing messages. Never make that future interval a persistent wildcard.
    for (const id of scope.conversationIDs?.length ? scope.conversationIDs : ['*']) db.run('INSERT OR IGNORE INTO puty_local_tombstones VALUES(?,?)', [id, maskBeforeMS]);
    for(const message of workingMessages)db.run('INSERT OR IGNORE INTO puty_local_deleted VALUES(?,?)',[message.conversationID,message.clientMsgID]);
    db.exec('UPDATE puty_runtime SET suppress_archive=1 WHERE singleton=1');
    for (const table of rows(db, "SELECT name FROM sqlite_master WHERE type='table' AND name GLOB 'chat_logs_*'")) {
      const name = String(table.name), id = name.slice('chat_logs_'.length);
      if (!scope.conversationIDs?.length || scope.conversationIDs.includes(id)) db.run(`UPDATE ${identifier(name)} SET status=4,content='',local_ex='' WHERE send_time<?`, [scope.beforeMS]);
    }
    if(rows(db,'PRAGMA table_info(local_conversations)').some(v=>v.name==='latest_msg')){
      const scopeSQL=scope.conversationIDs?.length?` AND conversation_id IN(${scope.conversationIDs.map(quote).join(',')})`:'';
      db.run(`UPDATE local_conversations SET latest_msg='',unread_count=0 WHERE json_valid(latest_msg) AND CAST(json_extract(latest_msg,'$.sendTime') AS INTEGER)<?${scopeSQL}`,[scope.beforeMS]);
    }
    db.exec('UPDATE puty_runtime SET suppress_archive=0 WHERE singleton=1; DELETE FROM puty_archive_outbox; COMMIT; VACUUM;');
    await send('cleanup-commit', { scope:{...scope,maskBeforeMS},workingMessages, snapshot: b64(snapshotBytes()) }); return { cleaned: true,maskBeforeMS };
  } catch (error) { try { db.exec('ROLLBACK'); } catch {} /* Host rejects further access until SDK reloads from its durable checkpoint. */ raw = undefined; database = undefined; throw error; }
}
