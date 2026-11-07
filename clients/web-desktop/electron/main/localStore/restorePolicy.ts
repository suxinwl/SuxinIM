import {LocalVault,type MessageSuppression} from './vault';
import {promises as fs} from 'node:fs';
import path from 'node:path';
import {loadLegacySQL,type LegacySQL} from './legacyCoordinator';
type Rule={conversationIDs:string[]|null;beforeMS:number};
const identifier=(value:string)=>'"'+value.replace(/"/g,'""')+'"';
const quote=(value:string)=>"'"+value.replace(/'/g,"''")+"'";
function decode(value:any):any{return value?.$bytes!==undefined?new Uint8Array(Buffer.from(value.$bytes,'base64')):Array.isArray(value)?value.map(decode):value;}
// Restoring an older backup must not roll back this installation's local clear
// markers. Rebuild only in RAM, then persist the same tombstones and compacted
// payload into the new vault before it is promoted or exposed to the SDK.
export async function preserveCleanupPolicy(store:LocalVault,rules:Rule[],providedSQL?:LegacySQL,suppressions:MessageSuppression[]=[]){
  rules=[...new Map(rules.map(rule=>[JSON.stringify([rule.conversationIDs&&[...rule.conversationIDs].sort(),rule.beforeMS]),rule])).values()];
  const saved=await store.restoreSDK(),merged=new Map<string,MessageSuppression>();
  for(const value of [...saved.suppression,...suppressions]){const id=JSON.stringify([value.conversationID,value.clientMsgID]),old=merged.get(id);merged.set(id,{...value,mode:old?.mode==='delete'?'delete':value.mode});}
  if(!rules.length&&!merged.size)return;const SQL=providedSQL || await loadLegacySQL(),db=new SQL.Database(saved.snapshot?new Uint8Array(Buffer.from(saved.snapshot,'base64')):undefined);
  try{
    for(const entry of saved.entries)for(const statement of entry.statements)statement.bindings?db.run(statement.sql,decode(statement.bindings)):db.exec(statement.sql);
    db.exec("PRAGMA secure_delete=ON;CREATE TABLE IF NOT EXISTS puty_local_tombstones(conversation_id TEXT NOT NULL,before_ms INTEGER NOT NULL,PRIMARY KEY(conversation_id,before_ms));CREATE TABLE IF NOT EXISTS puty_local_deleted(conversation_id TEXT NOT NULL,client_msg_id TEXT NOT NULL,PRIMARY KEY(conversation_id,client_msg_id));");
    const hasRuntime=Boolean(db.exec("SELECT name FROM sqlite_master WHERE name='puty_runtime'").length);if(hasRuntime)db.exec('UPDATE puty_runtime SET suppress_archive=1 WHERE singleton=1');
    const tables=db.exec("SELECT name FROM sqlite_master WHERE type='table' AND name GLOB 'chat_logs_*'")[0]?.values.map(row=>String(row[0])) || [];
    for(const rule of rules){
      if(!Number.isFinite(rule.beforeMS))throw new Error('本机清理规则无效');for(const id of rule.conversationIDs || ['*'])db.run('INSERT OR IGNORE INTO puty_local_tombstones VALUES(?,?)',[id,rule.beforeMS]);
      for(const table of tables){if(rule.conversationIDs && !rule.conversationIDs.includes(table.slice('chat_logs_'.length)))continue;const columns=db.exec(`PRAGMA table_info(${identifier(table)})`)[0].values.map(row=>String(row[1]));const values=["status=4",...['content','local_ex','attached_info','ex'].filter(name=>columns.includes(name)).map(name=>identifier(name)+"=''")];db.run(`UPDATE ${identifier(table)} SET ${values.join(',')} WHERE send_time<?`,[rule.beforeMS]);}
      if(db.exec("PRAGMA table_info(local_conversations)")[0]?.values.some(row=>row[1]==='latest_msg'))db.run(`UPDATE local_conversations SET latest_msg='',unread_count=0 WHERE json_valid(latest_msg) AND CAST(json_extract(latest_msg,'$.sendTime') AS INTEGER)<?${rule.conversationIDs?` AND conversation_id IN(${rule.conversationIDs.map(quote).join(',')})`:''}`,[rule.beforeMS]);
    }
    for(const value of merged.values())if(value.mode==='delete'){
      if(!/^[\w-]{1,256}$/.test(value.conversationID) || !value.clientMsgID || value.clientMsgID.length>512)throw new Error('本机消息保留规则无效');
      db.run('INSERT OR IGNORE INTO puty_local_deleted VALUES(?,?)',[value.conversationID,value.clientMsgID]);
      const table='chat_logs_'+value.conversationID;if(tables.includes(table)){const columns=db.exec(`PRAGMA table_info(${identifier(table)})`)[0].values.map(row=>String(row[1]));const values=["status=4",...['content','local_ex','attached_info','ex'].filter(name=>columns.includes(name)).map(name=>identifier(name)+"=''")];db.run(`UPDATE ${identifier(table)} SET ${values.join(',')} WHERE client_msg_id=?`,[value.clientMsgID]);}
      if(db.exec('PRAGMA table_info(local_conversations)')[0]?.values.some(row=>row[1]==='latest_msg'))db.run("UPDATE local_conversations SET latest_msg='',unread_count=0 WHERE conversation_id=? AND json_valid(latest_msg) AND json_extract(latest_msg,'$.clientMsgID')=?",[value.conversationID,value.clientMsgID]);
    }
    if(hasRuntime)db.exec('UPDATE puty_runtime SET suppress_archive=0 WHERE singleton=1');if(db.exec("SELECT name FROM sqlite_master WHERE name='puty_archive_outbox'").length)db.exec('DELETE FROM puty_archive_outbox');db.exec('VACUUM');
    const snapshot=db.export();for(const rule of rules)await store.preserveHistoricalCleanup({conversationIDs:rule.conversationIDs || undefined,beforeMS:rule.beforeMS},snapshot);if(merged.size)await store.preserveSuppressions([...merged.values()],snapshot);
  }finally{db.close();}
}

// A failed OS unwrap leaves no active LocalVault. Read the on-disk policy using
// its OS-protected key, or the independently verified backup key, before replacing
// anything. Unreadable existing data must never be interpreted as an empty policy.
export async function readExistingRestorationPolicy(accountRoot:string,account:string,recoveryKey:Uint8Array,unwrap:(sealed:Buffer)=>Promise<Uint8Array>,check:()=>Promise<void>){
  await check();const meta=await fs.lstat(accountRoot).catch(error=>{if(error.code==='ENOENT')return undefined;throw error;});
  const empty={ranges:[] as Rule[],suppressions:[] as MessageSuppression[]};if(!meta)return empty;
  if(!meta.isDirectory() || meta.isSymbolicLink())throw new Error('现有账号目录无效');
  const vaultRoot=path.join(accountRoot,'vault'),data=await fs.lstat(vaultRoot).catch(error=>{if(error.code==='ENOENT')return undefined;throw error;});
  if(!data){if((await fs.readdir(accountRoot)).some(name=>name!=='master.wrap'))throw new Error('现有账号资料无法校验');return empty;}
  if(!data.isDirectory() || data.isSymbolicLink())throw new Error('现有档案目录无效');
  if(!(await fs.readdir(vaultRoot)).length)return empty;
  const pointer=await fs.lstat(path.join(vaultRoot,'state/current')).catch(error=>{if(error.code==='ENOENT')return undefined;throw error;});
  const journals=await fs.readdir(path.join(vaultRoot,'sdk/journal')).catch(error=>{if(error.code==='ENOENT')return [] as string[];throw error;});
  if(!pointer&&!journals.some(name=>/^\d{16}$/.test(name)&&Number(name)>0)){
    const pending=[vaultRoot];while(pending.length){const folder=pending.pop()!;for(const name of await fs.readdir(folder)){const entry=path.join(folder,name),stat=await fs.lstat(entry);if(stat.isSymbolicLink() || !stat.isDirectory())throw new Error('现有档案缺少有效提交记录，已保留原资料');pending.push(entry);}}
    return empty;
  }
  const keys:Buffer[]=[];
  try{
    try{keys.push(Buffer.from(await unwrap(await fs.readFile(path.join(accountRoot,'master.wrap')))));}catch{/* A recovered key may unlock the same existing archive after an OS profile change. */}
    keys.push(Buffer.from(recoveryKey));
    for(const key of keys){await check();if(key.length!==32)continue;let current:LocalVault|undefined;
      try{current=await LocalVault.open(vaultRoot,account,key,check);const result=await current.restorationPolicy();await check();return result;}catch{await check();}finally{current?.lock();}
    }
    throw new Error('现有档案无法校验，已保留原资料；恢复前必须能读取本机清理记录');
  }finally{for(const key of keys)key.fill(0);}
}
