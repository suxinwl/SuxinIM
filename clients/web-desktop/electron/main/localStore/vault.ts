import { createHash, randomBytes, randomUUID } from "node:crypto";
import { promises as fs } from "node:fs";
import path from "node:path";
import { createHeader, decryptFrame, encryptFrame, exportRecovery, objectKey, parseHeader, seal, unseal } from "./crypto";
import {encryptedTransfer,inspectEncryptedFile,type DownloadFactory} from "./transfer";
import {createBackup} from './backup';
import { beforeShanghaiDate, reserveName, safeSourceName, shanghaiMonth } from "./names";

export type SQLStatement = { sql: string; bindings?: unknown[] };
export type ArchiveEvent = { kind: "upsert" | "delete"; conversationID: string; row: Record<string, any> };
export type SDKCommit = { opID: string; statements: SQLStatement[]; events?: ArchiveEvent[]; snapshot?: string };
export type CleanupScope = { conversationIDs?: string[]; beforeDate?: string; beforeMS?: number; maskBeforeMS?:number };
export type CleanupMessage = {conversationID:string;clientMsgID:string;sendTime:number};
export type MessageSuppression = {conversationID:string;clientMsgID:string;sendTime:number;mode:'delete'|'archive'};
type MessageRef = { conversationID: string; clientMsgID: string; sendTime: number; month: string; file: string; objectID: string; bytes: number; deleted?: boolean; seq?:number; status?:number; contentType?:number; sendID?:string };
type Attachment = { taskID: string; objectID: string; name: string; sourceName: string; file: string; month: string; state: "reserved" | "staged" | "ready"; size: number; sha256: string; refs: string[]; mime: string; remoteVersion?:string };
type Index = { version: 1; sequence: number; messages: Record<string, MessageRef>; attachments: Record<string, Attachment>; tombstones: { conversationIDs: string[] | null; beforeMS: number }[]; suppressions:Record<string,MessageSuppression>; preferences: Record<string, unknown>; pending: Record<string, {name:string;type:string;size:number;file:string;refs?:string[]}>; gc: string[] };
type Journal = { sequence: number; commit: SDKCommit; attachments?: Record<string, Attachment>; tombstones?: Index["tombstones"]; suppressions?:Index['suppressions']; preferences?: Record<string, unknown>; pending?:Index['pending'] };
const emptyIndex = (): Index => ({ version: 1, sequence: 0, messages: {}, attachments: {}, tombstones: [], suppressions:{}, preferences: {}, pending: {}, gc: [] });
const hash = (value: string) => createHash("sha256").update(value).digest("hex");
const messageID = (conversationID: string, clientMsgID: string) => hash(conversationID + "\0" + clientMsgID);
export async function syncDirectory(directory: string) {
  if (process.platform === "win32") return; // Windows does not expose directory fsync through Node.
  const handle = await fs.open(directory, "r"); try { await handle.sync(); } finally { await handle.close(); }
}
export async function atomicWrite(target: string, bytes: Uint8Array) {
  await fs.mkdir(path.dirname(target), { recursive: true });
  const temporary = target + ".next-" + randomUUID(), handle = await fs.open(temporary, "wx", 0o600);
  try { try { await handle.writeFile(bytes); await handle.sync(); } finally { await handle.close(); }
    await fs.rename(temporary, target); await syncDirectory(path.dirname(target)); }
  finally { await fs.unlink(temporary).catch(error => { if (error.code !== "ENOENT") throw error; }); }
}
export class LocalVault {
  private index = emptyIndex();
  private key: Buffer;
  private generation = "";
  private checkpointSequence = 0;
  private knownOps = new Map<string, number>();
  private queue: Promise<unknown> = Promise.resolve();
  private closed = false;
  private needsRecovery = false;
  private gcPending = false;
  private orphanSweepPending = true;
  private transfers = new Map<string,Promise<Attachment>>();
  private constructor(readonly root: string, readonly account: string, key: Uint8Array, private validate: () => Promise<void>) { this.key = Buffer.from(key); }
  static async open(root: string, account: string, key: Uint8Array, validate: () => Promise<void>) {
    if (!/^emp_[a-f0-9]{32}$/.test(account) && !/^test_[a-z0-9_-]+$/i.test(account)) throw new Error("Invalid archive account");
    if (key.length !== 32 || path.resolve(root) !== root) throw new Error("Invalid archive root or key");
    await fs.mkdir(root, { recursive: true });
    const meta = await fs.lstat(root); if (meta.isSymbolicLink() || !meta.isDirectory()) throw new Error("存储目录不能使用链接或重定向路径");
    const result = new LocalVault(await fs.realpath(root), account, key, validate);try{await result.check();await result.restoreIndex();return result;}catch(error){result.lock();throw error;}
  }
  lock() { this.closed = true; this.key.fill(0); this.index = emptyIndex(); this.knownOps.clear(); }
  async drain(){await Promise.allSettled([...this.transfers.values()]);await this.queue;}
  private async check() { if (this.closed) throw new Error("本地档案已锁定，请在线重新登录"); await this.validate(); if (this.closed) throw new Error("本地档案已锁定"); if (this.needsRecovery) { await this.restoreIndex(); this.needsRecovery = false; } }
  private serial<T>(operation: () => Promise<T>): Promise<T> { const next = this.queue.then(operation); this.queue = next.catch(() => {}); return next; }
  private async location(relative: string) {
    const target = path.resolve(this.root, relative);
    if (target === this.root || !target.startsWith(this.root + path.sep)) throw new Error("Archive path outside storage root");
    let cursor = this.root;
    for (const part of path.relative(this.root, target).split(path.sep).slice(0, -1)) {
      cursor = path.join(cursor, part); await fs.mkdir(cursor, { recursive: true });
      if ((await fs.lstat(cursor)).isSymbolicLink()) throw new Error("Archive path is a symbolic link");
    }
    const meta = await fs.lstat(target).catch(error => { if (error.code === "ENOENT") return null; throw error; });
    if (meta && (!meta.isFile() || meta.isSymbolicLink())) throw new Error("Archive object is not a regular file");
    return target;
  }
  private async write(relative: string, type: string, objectID: string, data: Uint8Array) { const target=await this.location(relative);if(this.closed)throw new Error('本地档案已锁定');const encrypted=seal(this.key,{account:this.account,type,objectID},data);await atomicWrite(target,encrypted); }
  private async read(relative: string, type: string, objectID: string) { const bytes=await fs.readFile(await this.location(relative));if(this.closed)throw new Error('本地档案已锁定');return unseal(this.key, { account: this.account, type, objectID },bytes); }
  private async readJSON<T>(relative: string, type: string, objectID: string): Promise<T> { return JSON.parse((await this.read(relative, type, objectID)).toString("utf8")); }
  private async writeJSON(relative: string, type: string, objectID: string, data: unknown) { await this.write(relative, type, objectID, Buffer.from(JSON.stringify(data))); }
  private async restoreIndex() {
    this.index = emptyIndex(); this.generation = ""; this.checkpointSequence = 0; this.knownOps.clear();this.orphanSweepPending=true;
    let pointer: { generation: string; sequence: number } | undefined;
    try { pointer = await this.readJSON("state/current", "pointer", "current"); }
    catch (error) { if ((error as NodeJS.ErrnoException).code !== "ENOENT") throw new Error("本地档案校验失败，未自动创建空档案", { cause: error }); }
    if (pointer) {
      if (!/^[a-f0-9-]{36}$/.test(pointer.generation)) throw new Error("Invalid archive generation");
      this.generation = pointer.generation; this.checkpointSequence = pointer.sequence;
      this.index = await this.readJSON(`state/${this.generation}.index`, "index", this.generation);
      if (this.index.version !== 1 || this.index.sequence !== pointer.sequence) throw new Error("Archive checkpoint sequence mismatch");
      this.index.suppressions ||= {};
    }
    const directory = path.join(this.root, "sdk/journal"); await fs.mkdir(directory, { recursive: true });
    const names = (await fs.readdir(directory)).filter(name => /^\d{16}$/.test(name)).sort();
    if(!pointer&&!names.some(name=>Number(name)>0)){
      // A missing pointer beside existing ciphertext is corruption, not a new
      // account. Never let the empty index's orphan sweep erase that evidence.
      const folders=[path.join(this.root,'state'),path.join(this.root,'messages')];while(folders.length){const folder=folders.pop()!,meta=await fs.lstat(folder).catch(error=>{if(error.code==='ENOENT')return undefined;throw error;});if(!meta)continue;if(meta.isSymbolicLink() || !meta.isDirectory())throw new Error('本地档案提交记录缺失，已保留原资料，请从完整备份恢复');for(const name of await fs.readdir(folder)){const target=path.join(folder,name),entry=await fs.lstat(target);if(entry.isSymbolicLink() || !entry.isDirectory())throw new Error('本地档案提交记录缺失，已保留原资料，请从完整备份恢复');folders.push(target);}}
    }
    for (const name of names) {
      const sequence = Number(name); if (sequence <= this.checkpointSequence) continue;
      if (sequence !== this.index.sequence + 1) throw new Error("本地档案日志缺失，已停止恢复");
      const journal = await this.readJSON<Journal>(`sdk/journal/${name}`, "sdk-journal", name);
      if (journal.sequence !== sequence) throw new Error("Archive journal sequence mismatch");
      await this.apply(journal); this.knownOps.set(journal.commit.opID, sequence);
    }
    let recovered=false;
    for(const entry of Object.values(this.index.attachments))if(entry.state==='staged'){
      const target=await this.location(entry.file),partial=await this.location(entry.file+'.part-'+entry.objectID),ready=(await fs.stat(target)).size>0,from=ready?target:partial;
      const checked=await inspectEncryptedFile(this.key,{account:this.account,type:'attachment',objectID:entry.objectID},from,()=>this.validate());
      if(checked.size!==entry.size || checked.sha256!==entry.sha256)throw new Error('附件恢复完整性校验失败');
      if(!ready){await fs.rename(from,target);await syncDirectory(path.dirname(target));}entry.state='ready';recovered=true;
    }
    if(recovered)await this.metaCommit();
    // Uncheckpointed patch journals may still need a checkpoint's old message
    // object during the next replay. Retire those objects only after a pointer
    // commit makes the new index and SDK snapshot authoritative together.
    if(this.index.sequence===this.checkpointSequence)try{await this.collectGarbage();this.gcPending=false;}catch{this.gcPending=true;}
  }
  async restoreSDK() { return this.serial(async () => {
    await this.check();
    const snapshot = this.generation ? (await this.read(`state/${this.generation}.sdk`, "sdk-checkpoint", this.generation)).toString("base64") : "";
    const entries: { sequence: number; statements: SQLStatement[] }[] = [];
    for (let sequence = this.checkpointSequence + 1; sequence <= this.index.sequence; sequence++) {
      const name = String(sequence).padStart(16, "0"), value = await this.readJSON<Journal>(`sdk/journal/${name}`, "sdk-journal", name);
      entries.push({ sequence, statements: value.commit.statements });
    }
    return { snapshot, entries, sequence: this.index.sequence, tombstones: this.index.tombstones, suppression:Object.values(this.index.suppressions) };
  }); }
  private hidden(conversationID: string, sendTime: number) { return this.index.tombstones.some(t => (!t.conversationIDs || t.conversationIDs.includes(conversationID)) && sendTime < t.beforeMS); }
  private async apply(journal: Journal) {
    if (journal.tombstones) this.index.tombstones = journal.tombstones;
    if (journal.attachments) this.index.attachments = journal.attachments;
    if (journal.preferences) this.index.preferences = journal.preferences;
    if (journal.pending) this.index.pending = journal.pending;
    if (journal.suppressions) this.index.suppressions = journal.suppressions;
    for (const event of journal.commit.events || []) {
      const clientMsgID = String(event.row.client_msg_id || event.row.clientMsgID || ""), conversationID = event.conversationID;
      if (!clientMsgID || !conversationID) throw new Error("Archive event has no stable message identifier");
      const id = messageID(conversationID, clientMsgID), prior = this.index.messages[id];
      const rawTime = Number(event.row.send_time || event.row.sendTime || event.row.create_time || event.row.createTime || 0);
      const sendTime = rawTime ? (rawTime < 100000000000 ? rawTime * 1000 : rawTime) : prior?.sendTime || this.index.suppressions[id]?.sendTime || 0;
      const deleted=event.kind==='delete' || Number(event.row.content_type ?? event.row.contentType)===2101 || Number(event.row.status)>=4;
      if(deleted || event.row.__ephemeral){const old=this.index.suppressions[id];this.index.suppressions[id]={conversationID,clientMsgID,sendTime,mode:deleted || old?.mode==='delete'?'delete':'archive'};}
      if (this.index.suppressions[id] || this.hidden(conversationID, sendTime)) {
        if (prior) this.index.gc.push(prior.file); delete this.index.messages[id];
        for (const [taskID, file] of Object.entries(this.index.attachments)) if (file.refs.includes(id)) { file.refs = file.refs.filter(ref => ref !== id); if (!file.refs.length) { this.index.gc.push(file.file,file.file+".part-"+file.objectID); delete this.index.attachments[taskID]; } }
        for(const [pendingID,file] of Object.entries(this.index.pending))if(file.refs?.includes(id)){file.refs=file.refs.filter(ref=>ref!==id);if(!file.refs.length){this.index.gc.push(file.file);delete this.index.pending[pendingID];}}
        continue;
      }
      let record=event.row;
      if(record.__patch && prior){const old=await this.readJSON<Record<string,any>>(prior.file,"message",id);record={...old,...record};}
      const {__patch,__ephemeral,...bodyRecord}=record;
      const month = shanghaiMonth(sendTime), file = `messages/${month}/${hash(conversationID)}/${id}-${String(journal.sequence).padStart(16,"0")}`;
      const body = Buffer.from(JSON.stringify(bodyRecord));
      for(const match of body.toString().matchAll(/puty-local:\/\/pending\/([a-f0-9-]{36})/g)){const pending=this.index.pending[match[1]];if(pending)pending.refs=[...new Set([...(pending.refs || []),id])];}
      await this.write(file, "message", id, body);
      if (prior && prior.file !== file) this.index.gc.push(prior.file);
      this.index.messages[id] = { conversationID, clientMsgID, sendTime, month, file, objectID: id, bytes: body.length,seq:Number(bodyRecord.seq || 0),status:Number(bodyRecord.status || 0),contentType:Number(bodyRecord.content_type || 0),sendID:String(bodyRecord.send_id || "") };
    }
    this.index.sequence = journal.sequence;
  }
  async commit(commit: SDKCommit) { return this.serial(async () => {
    await this.check();
    if (!commit.opID || !Array.isArray(commit.statements) || (commit.events || []).length > 100000) throw new Error("Invalid durable operation");
    const previous = this.knownOps.get(commit.opID); if (previous) return { sequence: previous, committed: true };
    const sequence = this.index.sequence + 1, name = String(sequence).padStart(16, "0"), journal = { sequence, commit };
    if (commit.snapshot !== undefined) {
      const previousIndex = structuredClone(this.index);
      try { await this.apply(journal); await this.saveCheckpoint(Buffer.from(commit.snapshot,"base64")); this.knownOps.set(commit.opID,sequence); return { sequence, committed:true, archiveDeferred:false }; }
      catch (error) { this.index = previousIndex; this.needsRecovery = true; throw error; }
    }
    const target = await this.location(`sdk/journal/${name}`);
    try { await fs.access(target); throw new Error("本地日志存在未恢复的提交，请重新打开档案"); } catch (error) { if ((error as NodeJS.ErrnoException).code !== "ENOENT") throw error; }
    await this.writeJSON(`sdk/journal/${name}`, "sdk-journal", name, journal);
    // The durable encrypted journal is the commit point. Materialized message
    // files are recoverable from it; an I/O failure must not report rollback of
    // an already durable transaction.
    this.knownOps.set(commit.opID, sequence);
    try { await this.apply(journal); } catch { this.index.sequence = sequence; this.needsRecovery = true; return { sequence, committed: true, archiveDeferred: true }; }
    return { sequence, committed: true, archiveDeferred: false };
  }); }
  async checkpoint(snapshot: Uint8Array) { return this.serial(async () => { await this.check(); await this.saveCheckpoint(snapshot); }); }
  // Called only by the main-process legacy coordinator before SDK startup.
  // Each <=1000-event journal is a durable, replayable batch. The final pointer
  // atomically commits both the complete archive index and the SQLite snapshot.
  async importLegacy(snapshot: Uint8Array, events: ArchiveEvent[], sourceID: string): Promise<{ imported: boolean; alreadyImported: boolean; messages: number; sequence: number }> { return this.serial(async () => {
    await this.check();
    if (!/^[a-f0-9]{64}$/.test(sourceID) || !snapshot.length || !Array.isArray(events)) throw new Error("Invalid sealed legacy import identity");
    const snapshotHash = createHash("sha256").update(snapshot).digest("hex"), eventHash = createHash("sha256");
    for (const event of events) eventHash.update(JSON.stringify(event) + "\n");
    const eventsHash = eventHash.digest("hex"), name = "legacy-import", existing = this.index.preferences[name] as any, batches = Math.ceil(events.length / 1000);
    if (existing) {
      if (existing.sourceID !== sourceID || existing.snapshotHash !== snapshotHash || existing.eventsHash !== eventsHash || existing.messages !== events.length || existing.batches !== batches) throw new Error("Legacy import differs from its durable source marker");
      if (existing.phase === "complete") return { imported: false, alreadyImported: true, messages: events.length, sequence: this.index.sequence };
      if (existing.phase !== "preparing" || !Number.isSafeInteger(existing.nextBatch) || existing.nextBatch < 0 || existing.nextBatch > batches || this.generation) throw new Error("Legacy preparation marker is invalid");
    } else {
      if (this.generation || Object.keys(this.index.messages).length || this.index.tombstones.length) throw new Error("Existing vault cannot be overwritten by a legacy import");
      for (let sequence = 1; sequence <= this.index.sequence; sequence++) {
        const number = String(sequence).padStart(16, "0"), journal = await this.readJSON<Journal>(`sdk/journal/${number}`, "sdk-journal", number);
        if (journal.commit.statements.length || journal.commit.events?.length) throw new Error("Legacy import found an existing SDK journal");
      }
      this.index.preferences[name] = { sourceID, snapshotHash, eventsHash, messages: events.length, batches, phase: "preparing", nextBatch: 0 };
      try { await this.metaCommit(); } catch (error) { this.needsRecovery = true; throw error; }
    }
    try {
      const marker = this.index.preferences[name] as any;
      for (let batch = marker.nextBatch; batch < batches; batch++) {
        await this.check();
        const sequence = this.index.sequence + 1, number = String(sequence).padStart(16, "0"), opID = `legacy-${sourceID}-${batch}`;
        const journal: Journal = { sequence, commit: { opID, statements: [], events: events.slice(batch * 1000, (batch + 1) * 1000) }, preferences: { ...this.index.preferences, [name]: { ...this.index.preferences[name] as any, nextBatch: batch + 1 } } };
        await this.writeJSON(`sdk/journal/${number}`, "sdk-journal", number, journal);
        this.knownOps.set(opID, sequence); await this.apply(journal);
      }
      this.index.preferences[name] = { ...this.index.preferences[name] as any, phase: "complete" };
      await this.check(); await this.saveCheckpoint(snapshot);
      return { imported: true, alreadyImported: false, messages: events.length, sequence: this.index.sequence };
    } catch (error) { this.needsRecovery = true; throw error; }
  }); }
  private async saveCheckpoint(snapshot: Uint8Array) {
    const generation = randomUUID();
    try{await this.write(`state/${generation}.sdk`, "sdk-checkpoint", generation, snapshot);
      await this.writeJSON(`state/${generation}.index`, "index", generation, this.index);
      await this.writeJSON("state/current", "pointer", "current", { generation, sequence: this.index.sequence });
    }catch(error){this.orphanSweepPending=true;throw error;}
    this.generation = generation; this.checkpointSequence = this.index.sequence;
    // state/current is the commit point. Cleanup failure must never turn a
    // committed checkpoint into a reported rollback.
    try{await this.collectGarbage();this.gcPending=false;}catch{this.gcPending=true;}
  }
  private async collectGarbage() {
    while(this.index.gc.length){const relative=this.index.gc[0];await fs.unlink(await this.location(relative)).catch(error=>{if(error.code!=="ENOENT")throw error;});this.index.gc.shift();}
    for (const name of await fs.readdir(path.join(this.root, "state")).catch(() => [] as string[])) {
      if (/^[a-f0-9-]{36}\.(sdk|index)$/.test(name) && !name.startsWith(this.generation + ".")) await fs.unlink(await this.location(`state/${name}`));
    }
    for (const name of await fs.readdir(path.join(this.root, "sdk/journal")).catch(() => [] as string[])) {
      if (/^\d{16}$/.test(name) && Number(name) <= this.checkpointSequence) await fs.unlink(await this.location(`sdk/journal/${name}`));
    }
    // A compact snapshot can materialize monthly rows before its pointer is
    // committed. If that pointer fails there is no journal referencing those
    // new rows. Reclaim only our exact message-file namespace after a complete
    // checkpoint; outstanding sparse patches may still need older row bodies.
    if(this.index.sequence!==this.checkpointSequence || !this.orphanSweepPending)return;
    const referenced=new Set(Object.values(this.index.messages).map(value=>value.file)),base=path.join(this.root,'messages');
    const readDirectory=async(value:string)=>{const info=await fs.lstat(value).catch(error=>{if(error.code==='ENOENT')return;throw error;});if(!info)return [];if(!info.isDirectory() || info.isSymbolicLink())throw new Error('月档目录包含链接，未清理文件');return fs.readdir(value,{withFileTypes:true});};
    for(const month of await readDirectory(base)){
      if(!/^\d{4}(?:0[1-9]|1[0-2])$/.test(month.name))continue;const monthDir=path.join(base,month.name);
      for(const conversation of await readDirectory(monthDir)){
        if(!/^[a-f0-9]{64}$/.test(conversation.name))continue;const conversationDir=path.join(monthDir,conversation.name);
        for(const file of await readDirectory(conversationDir)){
          if(!/^[a-f0-9]{64}-\d{16}$/.test(file.name))continue;const relative=`messages/${month.name}/${conversation.name}/${file.name}`;
          if(!referenced.has(relative))await fs.unlink(await this.location(relative));
        }
        await fs.rmdir(conversationDir).catch(error=>{if(error.code!=='ENOTEMPTY' && error.code!=='EEXIST')throw error;});
      }
      await fs.rmdir(monthDir).catch(error=>{if(error.code!=='ENOTEMPTY' && error.code!=='EEXIST')throw error;});
    }
    this.orphanSweepPending=false;
  }
  async history(conversationID: string, beforeMS = Number.MAX_SAFE_INTEGER, count = 20, startClientMsgID?: string) {
    const anchor=startClientMsgID?this.index.messages[messageID(conversationID,startClientMsgID)]:undefined;
    return this.sdkQuery("getMessageList",[conversationID,count,anchor?.sendTime ?? beforeMS,anchor?.seq ?? 0,startClientMsgID || "",false]);
  }
  async sdkQuery(name:string,args:any[],workingRows:Record<string,any>[]=[]):Promise<Record<string,any>[]> { return this.serial(async()=>{
    await this.check();const conversationID=String(args[0] || "");if(!/^[\w-]{1,256}$/.test(conversationID))throw new Error("Invalid archive conversation");
    let count=20,unlimited=false,offset=0,asc=false,ids:string[]|undefined,seqs:number[]|undefined,types:number[]|undefined,keywords:string[]=[],allKeywords=false,start=0,end=Number.MAX_SAFE_INTEGER,boundary:number|undefined,anchorSeq=0,anchorID="",seqRequired=false,strictTimeBoundary=false;
    const list=(value:any):any[]=>Array.isArray(value)?value:JSON.parse(value || "[]");
    switch(name){
      case "getMessage":ids=[String(args[1])];count=1;break;
      case "getMessageBySeq":seqs=[Number(args[1])];count=1;break;
      case "getMultipleMessage":case "getMessagesByClientMsgIDs":ids=list(args[1]).map(String);unlimited=true;break;
      case "getMessagesBySeqs":seqs=list(args[1]).map(Number);unlimited=true;break;
      case "getMessageList":count=Number(args[1]);boundary=Number(args[2]) || undefined;anchorSeq=Number(args[3]);anchorID=String(args[4] || "");asc=Boolean(args[5]);break;
      case "getMessageListNoTime":count=Number(args[1]);asc=Boolean(args[2]);break;
      case "getLatestValidServerMessage":boundary=Number(args[1]);asc=!args[2];count=1;seqRequired=true;strictTimeBoundary=true;break;
      case "getLatestActiveMessage":asc=Boolean(args[1]);unlimited=true;break;
      case "searchAllMessageByContentType":types=[Number(typeof args[1]==="string"?JSON.parse(args[1]):args[1])];unlimited=true;break;
      case "searchMessageByContentType":types=list(args[1]).map(Number);start=Number(args[2]);end=Number(args[3]);offset=Number(args[4]);count=Number(args[5]);break;
      case "searchMessageByKeyword":case "searchMessageByContentTypeAndKeyword":types=list(args[1]).map(Number);keywords=list(args[2]).map(String);allKeywords=Number(args[3])!==0;start=Number(args[4]);end=Number(args[5]);offset=name==="searchMessageByKeyword"?Number(args[6]):0;if(name==="searchMessageByKeyword")count=Number(args[7]);else unlimited=true;break;
      default:throw new Error("Unsupported archive query");
    }
    if(!Number.isSafeInteger(count) || !Number.isSafeInteger(offset) || count<0 || offset<0 || offset>1000000 || !Number.isFinite(start) || !Number.isFinite(end) || boundary!==undefined && !Number.isFinite(boundary) || !Number.isSafeInteger(anchorSeq))throw new Error("Invalid archive query range");
    if(!unlimited&&count===0)return [];
    const wantedIDs=ids?new Set(ids):undefined,wantedSeqs=seqs?new Set(seqs):undefined;
    const transient=new Map<string,Record<string,any>>();for(const row of workingRows){const id=String(row.client_msg_id || "");if(id && Number(row.status)<4)transient.set(id,row);}
    const refs=Object.values(this.index.messages).filter(v=>v.conversationID===conversationID && !transient.has(v.clientMsgID));
    const candidates=refs.map(ref=>({ref,time:ref.sendTime,seq:ref.seq || 0,id:ref.clientMsgID,type:ref.contentType || 0}));
    for(const [id,row] of transient)candidates.push({ref:undefined as any,time:Number(row.send_time),seq:Number(row.seq || 0),id,type:Number(row.content_type || 0)});
    // The clientMsgID tie breaker is also used by the cursor. Locale-dependent
    // sorting and the former `seq=0 && id!=anchor` rule could repeat every other
    // pending message forever when several rows shared a millisecond.
    const compareID=(a:string,b:string)=>a===b?0:a<b?-1:1;
    candidates.sort((a,b)=>(asc?1:-1)*(a.time-b.time || a.seq-b.seq || compareID(a.id,b.id)));
    const result:Record<string,any>[]=[];let skipped=0;
    for(const item of candidates){
      if(this.index.suppressions[messageID(conversationID,item.id)]?.mode==='delete' || this.hidden(conversationID,item.time) || item.time<start || item.time>end || wantedIDs && !wantedIDs.has(item.id) || wantedSeqs && !wantedSeqs.has(item.seq) || types && !types.includes(item.type) || seqRequired && !item.seq)continue;
      if(boundary!==undefined){
        const order=item.time-boundary || (strictTimeBoundary?0:item.seq-anchorSeq || (anchorID?compareID(item.id,anchorID):0));
        // Without an ID the old SDK exposes only a time/seq cursor. Preserve its
        // initial seq-zero inclusion; ID cursors always use the strict tuple.
        const noIDPending=!strictTimeBoundary&&!anchorID&&item.time===boundary&&item.seq===0;
        if(!(asc?order>0:order<0)&&!noIDPending)continue;
      }
      const row=transient.get(item.id) || await this.readJSON<Record<string,any>>(item.ref.file,"message",item.ref.objectID);
      if(keywords.length){const text=String(row.content || "").toLocaleLowerCase();const matches=keywords.map(word=>text.includes(word.toLocaleLowerCase()));if(allKeywords?!matches.every(Boolean):!matches.some(Boolean))continue;}
      if(skipped++<offset)continue;result.push(row);if(!unlimited&&result.length>=count)break;
    }
    return result;
  }); }
  private scope(input: CleanupScope) {
    const beforeMS = input.beforeDate ? beforeShanghaiDate(input.beforeDate) : input.beforeMS ?? Date.now() + 1;
    const conversationIDs = input.conversationIDs?.length ? [...new Set(input.conversationIDs)] : null;
    if (!Number.isFinite(beforeMS) || beforeMS < 0 || conversationIDs?.some(id => !/^[\w-]{1,256}$/.test(id))) throw new Error("Invalid cleanup scope");
    if(input.maskBeforeMS!==undefined && (!Number.isSafeInteger(input.maskBeforeMS) || input.maskBeforeMS<0 || input.maskBeforeMS>beforeMS))throw new Error('Invalid cleanup confirmation boundary');
    return { beforeMS, conversationIDs };
  }
  private cleanupIDs(scope:{conversationIDs:string[]|null;beforeMS:number},working:CleanupMessage[]=[]){
    if(!Array.isArray(working) || working.length>1000000)throw new Error('Invalid working cleanup inventory');
    const removed=new Set(Object.values(this.index.messages).filter(v=>(!scope.conversationIDs || scope.conversationIDs.includes(v.conversationID)) && v.sendTime<scope.beforeMS).map(v=>messageID(v.conversationID,v.clientMsgID)));
    for(const row of working){if(!row || !/^[\w-]{1,256}$/.test(row.conversationID) || typeof row.clientMsgID!=='string' || !row.clientMsgID || row.clientMsgID.length>512 || !Number.isFinite(row.sendTime))throw new Error('Invalid working cleanup message');if((!scope.conversationIDs || scope.conversationIDs.includes(row.conversationID)) && row.sendTime<scope.beforeMS)removed.add(messageID(row.conversationID,row.clientMsgID));}
    return removed;
  }
  async previewCleanup(input: CleanupScope,working:CleanupMessage[]=[]) { return this.serial(async () => {
    await this.check(); const scope = this.scope(input), rows = Object.values(this.index.messages).filter(v => (!scope.conversationIDs || scope.conversationIDs.includes(v.conversationID)) && v.sendTime < scope.beforeMS);
    const removed = this.cleanupIDs(scope,working);
    const files = Object.values(this.index.attachments).filter(v => v.refs.length > 0 && v.refs.every(ref => removed.has(ref)));
    const pending=Object.values(this.index.pending).filter(v=>v.refs?.length && v.refs.every(ref=>removed.has(ref)));
    const paths=new Set([...rows.map(v=>v.file),...files.flatMap(v=>[v.file,v.file+'.part-'+v.objectID]),...pending.map(v=>v.file)]);
    let bytes=0;for(const file of paths){try{bytes+=(await fs.stat(await this.location(file))).size;}catch(error){if((error as NodeJS.ErrnoException).code!=='ENOENT')throw error;}}
    return { messages: removed.size, attachments: files.length+pending.length, bytes, beforeMS: scope.beforeMS, conversationIDs: scope.conversationIDs };
  }); }
  async clean(input: CleanupScope, cleanedSDKSnapshot: Uint8Array,working:CleanupMessage[]=[]) { return this.cleanScope(input,cleanedSDKSnapshot,working,false); }
  // Only the verified restore-policy path uses this method. It is deliberately
  // absent from IPC: existing authenticated history cannot be guessed narrower.
  async preserveHistoricalCleanup(input: CleanupScope, cleanedSDKSnapshot: Uint8Array) { return this.cleanScope(input,cleanedSDKSnapshot,[],true); }
  private async cleanScope(input: CleanupScope, cleanedSDKSnapshot: Uint8Array,working:CleanupMessage[],historical:boolean) { const confirmedBeforeMS=Date.now()+1;return this.serial(async () => {
    await this.check(); const scope = this.scope(input), removed = this.cleanupIDs(scope,working), previousIndex = structuredClone(this.index); this.index.tombstones.push({...scope,beforeMS:historical?scope.beforeMS:Math.min(scope.beforeMS,input.maskBeforeMS??confirmedBeforeMS,confirmedBeforeMS)});
    // Exact IDs keep already selected clock-skewed/future-dated rows deleted,
    // while a broad calendar selection cannot mask later incoming messages.
    for(const message of [...Object.values(this.index.messages),...working]){
      const id=messageID(message.conversationID,message.clientMsgID);
      if(removed.has(id))this.index.suppressions[id]={conversationID:message.conversationID,clientMsgID:message.clientMsgID,sendTime:message.sendTime,mode:'delete'};
    }
    for (const [id, message] of Object.entries(this.index.messages)) if ((!scope.conversationIDs || scope.conversationIDs.includes(message.conversationID)) && message.sendTime < scope.beforeMS) { removed.add(id); this.index.gc.push(message.file); delete this.index.messages[id]; }
    for (const [id, file] of Object.entries(this.index.attachments)) { file.refs = file.refs.filter(ref => !removed.has(ref)); if (file.refs.length === 0) { this.index.gc.push(file.file,file.file+".part-"+file.objectID); delete this.index.attachments[id]; } }
    for(const [id,file] of Object.entries(this.index.pending))if(file.refs?.length){file.refs=file.refs.filter(ref=>!removed.has(ref));if(!file.refs.length){this.index.gc.push(file.file);delete this.index.pending[id];}}
    // The new checkpoint contains the tombstone before obsolete payloads are
    // unlinked, so neither journal replay nor a late history response revives it.
    try { await this.saveCheckpoint(cleanedSDKSnapshot); return { messages: removed.size, cleaned: true,payloadCleanupPending:this.gcPending }; }
    catch(error) { this.index=previousIndex;this.needsRecovery=true;throw error; }
  }); }
  async reserveAttachment(input: { taskID: string; name: string; messageRefs: { conversationID: string; clientMsgID: string }[]; mime?: string; downloadedAt?:number }) { return this.serial(async () => {
    await this.check(); const existing = this.index.attachments[input.taskID]; if (existing) {const refs=[...new Set([...existing.refs,...input.messageRefs.map(v=>messageID(v.conversationID,v.clientMsgID))])];if(refs.length!==existing.refs.length){existing.refs=refs;await this.metaCommit();}return { ...existing };}
    if (!/^[\w-]{1,160}$/.test(input.taskID) || !input.messageRefs.length) throw new Error("Invalid attachment identity");
    const month = shanghaiMonth(input.downloadedAt ?? Date.now()), relativeDir = `attachments/${month}`, directory = path.dirname(await this.location(`${relativeDir}/.probe`));
    const reserved = await reserveName(directory, input.name), entry: Attachment = { taskID: input.taskID, objectID: randomUUID(), name: reserved.name, sourceName: safeSourceName(input.name), file: `${relativeDir}/${reserved.name}`, month, state: "reserved", size: 0, sha256: "", refs: input.messageRefs.map(v => messageID(v.conversationID, v.clientMsgID)), mime: input.mime || "application/octet-stream" };
    const owned = await fs.stat(reserved.path);
    this.index.attachments[input.taskID] = entry;
    // Reservation metadata must survive a process restart while the transfer is
    // incomplete; use a durable metadata journal with the same sequence.
    const journalName = String(this.index.sequence + 1).padStart(16, '0');
    try { await this.metaCommit(); }
    catch (error) {
      // A failed directory fsync may still leave a readable committed journal.
      // Retain its name; only remove our exact empty, unjournaled reservation.
      let absent = false;
      try { await fs.access(path.join(this.root, 'sdk/journal', journalName)); }
      catch (readError) { absent = (readError as NodeJS.ErrnoException).code === 'ENOENT'; }
      if (absent) {
        const current = await fs.lstat(reserved.path).catch(() => null);
        if (current?.isFile() && !current.isSymbolicLink() && current.size === 0 && current.dev === owned.dev && current.ino === owned.ino) await fs.unlink(reserved.path);
      }
      throw error;
    }
    return { ...entry };
  }); }
  private async metaCommit() {
    const sequence = this.index.sequence + 1, name = String(sequence).padStart(16, "0");
    const journal: Journal = { sequence, commit: { opID: randomUUID(), statements: [] }, attachments: this.index.attachments, preferences: this.index.preferences, tombstones: this.index.tombstones, suppressions:this.index.suppressions, pending:this.index.pending };
    try{await this.writeJSON(`sdk/journal/${name}`, "sdk-journal", name, journal);this.index.sequence=sequence;}catch(error){this.needsRecovery=true;throw error;}
  }
  async writeAttachment(taskID: string, source: AsyncIterable<Uint8Array>|DownloadFactory, expectedSize?: number, expectedSHA256?: string):Promise<Attachment> {
    const existing=this.transfers.get(taskID);if(existing)return existing;
    const operation=(async()=>{
      await this.check();const entry=this.index.attachments[taskID];if(!entry)throw new Error("附件尚未分配名称");if(entry.state==="ready")return {...entry};
      const target=await this.location(entry.file),temporary=target+".part-"+entry.objectID;
      const check=async()=>{await this.check();if(this.index.attachments[taskID]!==entry)throw new Error("附件已被清理");};
      if(this.closed)throw new Error('本地档案已锁定');
      const result=await encryptedTransfer(this.key,{account:this.account,objectID:entry.objectID,type:"attachment"},temporary,source,{version:entry.remoteVersion,expectedSize,expectedSHA256,check,saveVersion:async(version)=>{await this.serial(async()=>{await check();entry.remoteVersion=version;await this.metaCommit();});}});
      return this.serial(async()=>{
        await check();entry.state="staged";entry.size=result.size;entry.sha256=result.sha256;await this.metaCommit();if((await fs.stat(target)).size!==0)throw new Error("附件存档目标已被其他文件占用");
        await fs.rename(temporary,target);await syncDirectory(path.dirname(target));
        entry.state="ready";await this.metaCommit();return {...entry};
      });
    })();this.transfers.set(taskID,operation);try{return await operation;}catch(error){if(this.index.attachments[taskID]?.state==='staged')this.needsRecovery=true;throw error;}finally{if(this.transfers.get(taskID)===operation)this.transfers.delete(taskID);}
  }
  async attachment(taskID: string) { await this.check(); const value = this.index.attachments[taskID]; return value?.state === "ready" ? { ...value } : null; }
  async readAttachment(taskID: string, start = 0, end?: number): Promise<Buffer> {
    await this.check(); const entry = this.index.attachments[taskID]; if (!entry || entry.state !== "ready") throw new Error("附件尚未下载完成");
    const last = Math.min(end ?? entry.size - 1, entry.size - 1); if (!Number.isInteger(start) || start < 0 || (entry.size>0 && start > last) || last - start > 16 * 1024 * 1024) throw new Error("Invalid attachment read range");
    const handle = await fs.open(await this.location(entry.file), "r");
    try {
      const first = Buffer.alloc(12); await handle.read(first, 0, 12, 0); const length = first.readUInt32BE(8); if (length > 65536) throw new Error("Invalid encrypted header");
      const rest = Buffer.alloc(length); await handle.read(rest, 0, length, 12);
      const parsed = parseHeader(Buffer.concat([first, rest])), { header, bytes, offset } = parsed;
      if (header.account !== this.account || header.objectID !== entry.objectID || header.type !== "attachment") throw new Error("Attachment context mismatch");
      if(this.closed)throw new Error('本地档案已锁定');
      const key = objectKey(this.key, header), result: Buffer[] = [], firstIndex = Math.floor(start / header.chunkSize), lastIndex = Math.floor(last / header.chunkSize);
      try {
        const chunkCount = Math.ceil(entry.size / header.chunkSize), endOffset = offset + entry.size + chunkCount * 20;
        if ((await handle.stat()).size !== endOffset + 20) throw new Error("Truncated attachment");
        const terminal = Buffer.alloc(20); await handle.read(terminal, 0, 20, endOffset); if (terminal.readUInt32BE(0) !== 0) throw new Error("Attachment end marker invalid");
        decryptFrame(key, header, bytes, chunkCount, Buffer.alloc(0), terminal.subarray(4));
        for (let index = firstIndex; index <= lastIndex; index++) {
          const plainSize = Math.min(header.chunkSize, entry.size - index * header.chunkSize), frame = Buffer.alloc(plainSize + 20);
          const read = await handle.read(frame, 0, frame.length, offset + index * (header.chunkSize + 20));
          if (read.bytesRead !== frame.length || frame.readUInt32BE(0) !== plainSize) throw new Error("Attachment frame missing");
          result.push(decryptFrame(key, header, bytes, index, frame.subarray(4, 4 + plainSize), frame.subarray(4 + plainSize)));
        }
        if(this.closed)throw new Error('本地档案已锁定');return Buffer.concat(result).subarray(start - firstIndex * header.chunkSize, last - firstIndex * header.chunkSize + 1);
      } finally { key.fill(0); }
    } finally { await handle.close(); }
  }
  async preferences(value?: Record<string, unknown>) { return this.serial(async () => { await this.check(); if (value) { this.index.preferences = { ...this.index.preferences, ...value }; await this.metaCommit(); } return { ...this.index.preferences }; }); }
  attachmentForPath(value: string) { if(this.closed)return null;return Object.values(this.index.attachments).find(file=>file.state==='ready' && (value===path.join(this.root,file.file) || value===`puty-local://attachment/${file.taskID}`)) || null; }
  hasPending(id:string) { return !this.closed && Boolean(this.index.pending[id]); }
  async releasePending(ids:string[],clientMsgID:string){return this.serial(async()=>{
    await this.check();if(!Array.isArray(ids) || ids.length>64 || !clientMsgID)throw new Error('无效的发送缓存任务');
    for(const id of new Set(ids)){if(!/^[a-f0-9-]{36}$/.test(id))throw new Error('无效的发送缓存任务');const file=this.index.pending[id];if(!file)continue;
      const refs=(file.refs || []).filter(ref=>this.index.messages[ref]?.clientMsgID!==clientMsgID);if(refs.length){file.refs=refs;continue;}
      this.index.gc.push(file.file);delete this.index.pending[id];
    }
    await this.metaCommit();return {released:true};
  });}
  async stashFile(name:string,type:string,bytes:Uint8Array) { return this.serial(async()=>{await this.check();const id=randomUUID(),file=`pending/${id}/${safeSourceName(name)}`;await this.write(file,'pending',id,bytes);this.index.pending[id]={name:safeSourceName(name),type,size:bytes.length,file};await this.metaCommit();return {path:`puty-local://pending/${id}/${encodeURIComponent(safeSourceName(name))}`,name:safeSourceName(name),type,size:bytes.length};}); }
  async readPending(id:string) { await this.check();const info=this.index.pending[id];if(!info)throw new Error('待发送文件不存在');return {...info,bytes:await this.read(info.file,'pending',id)}; }
  async cleanupPolicy(){await this.check();return structuredClone([...this.index.tombstones,...Object.values(this.index.suppressions).filter(v=>v.mode==='delete').map(v=>({conversationIDs:[v.conversationID],beforeMS:Number.MAX_SAFE_INTEGER,messageIDs:[v.clientMsgID]}))]);}
  async restorationPolicy(){return this.serial(async()=>{await this.check();return structuredClone({ranges:this.index.tombstones,suppressions:Object.values(this.index.suppressions)});});}
  async preserveSuppressions(input:MessageSuppression[],snapshot:Uint8Array){return this.serial(async()=>{
    await this.check();const previous=structuredClone(this.index);
    try{
      for(const value of input){
        if(!/^[\w-]{1,256}$/.test(value.conversationID) || !value.clientMsgID || value.clientMsgID.length>512 || !Number.isFinite(value.sendTime) || !['delete','archive'].includes(value.mode))throw new Error('本机消息保留规则无效');
        const id=messageID(value.conversationID,value.clientMsgID),old=this.index.suppressions[id];this.index.suppressions[id]={...value,mode:old?.mode==='delete'?'delete':value.mode};
        const message=this.index.messages[id];if(message){this.index.gc.push(message.file);delete this.index.messages[id];}
        for(const [task,file] of Object.entries(this.index.attachments))if(file.refs.includes(id)){file.refs=file.refs.filter(ref=>ref!==id);if(!file.refs.length){this.index.gc.push(file.file,file.file+'.part-'+file.objectID);delete this.index.attachments[task];}}
        if(this.index.suppressions[id].mode==='delete')for(const [task,file] of Object.entries(this.index.pending))if(file.refs?.includes(id)){file.refs=file.refs.filter(ref=>ref!==id);if(!file.refs.length){this.index.gc.push(file.file);delete this.index.pending[task];}}
      }
      await this.saveCheckpoint(snapshot);
    }catch(error){this.index=previous;this.needsRecovery=true;throw error;}
  });}
  async recovery(passphrase:string) { await this.check();return exportRecovery(this.key,this.account,passphrase); }
  async backup(destination:string,passphrase:string,authorizeSave:()=>Promise<void>){return this.serial(async()=>{
    const check=async()=>{await this.check();await authorizeSave();await this.check();};
    await check();if(!this.generation)throw new Error('请先完成聊天库检查点再备份');
    const checkpoint=await this.readJSON<Index>(`state/${this.generation}.index`,'index',this.generation);
    const files=[`state/current`,`state/${this.generation}.sdk`,`state/${this.generation}.index`,...Object.values(this.index.messages).map(v=>v.file),...Object.values(this.index.attachments).map(v=>v.file),...Object.values(this.index.pending).map(v=>v.file),...Object.values(checkpoint.messages).map(v=>v.file),...Object.values(checkpoint.attachments).map(v=>v.file),...Object.values(checkpoint.pending).map(v=>v.file)];
    for(const entry of Object.values(this.index.attachments))if(entry.state==='staged')files.push(entry.file+'.part-'+entry.objectID);
    for(const relative of this.index.gc)if(await fs.stat(await this.location(relative)).then(()=>true,error=>{if(error.code==='ENOENT')return false;throw error;}))files.push(relative);
    for(let n=this.checkpointSequence+1;n<=this.index.sequence;n++)files.push('sdk/journal/'+String(n).padStart(16,'0'));
    return createBackup(this.root,this.account,this.key,destination,files,passphrase,check);
  });}
  async stats() { await this.check(); return { messages: Object.keys(this.index.messages).length, attachments: Object.values(this.index.attachments).filter(v => v.state === "ready").length, bytes: Object.values(this.index.attachments).reduce((n, v) => n + v.size, 0), directory: this.root, sequence: this.index.sequence,payloadCleanupPending:this.gcPending }; }
}
