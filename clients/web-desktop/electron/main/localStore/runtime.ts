import { app, BrowserWindow, dialog, ipcMain, net, safeStorage, shell, type IpcMainInvokeEvent, type WebContents } from "electron";
import { createHash, randomBytes, randomUUID } from "node:crypto";
import { promises as fs } from "node:fs";
import path from "node:path";
import { atomicWrite, LocalVault, type MessageSuppression } from "./vault";
import { bootstrapDirectory, bootstrapPath, configuredRoot, legacyUserData,validateStorageDirectory,validateStorageRoot,preserveBootstrapConfiguration,clearStartupStorageError } from "./bootstrap";
import { readLegacyInChild } from './legacyReaderProcess';
import { listLegacySnapshots, preserveLegacyData, cleanupLegacyPlaintext, importLegacyAccount } from './legacyCoordinator';
import {legacyMigrationFailure,type LegacyMigrationPhase} from './legacyFailure';
import { beforeShanghaiDate, reserveName } from "./names";
import { importRecovery } from "./crypto";
import {restoreBackup,fileHash,writeRecoveryExport} from './backup';
import {preserveCleanupPolicy,readExistingRestorationPolicy} from './restorePolicy';
import {installRestoredAccount,recoverRestoreTransaction,collectRestorePrevious,type RestoreTransactionOptions} from './restoreTransaction';
import {bindPreferencesRoot,flushPreferences,initProtectedPreferences} from '../storeManage';
import {assertPCSessionState,type LiveState} from './sessionState';
import {EventEmitter} from 'node:events';
export const storeLifecycle=new EventEmitter();
type Credentials = { origin:string;userID:string;imToken:string;chatToken:string };
const origins=new Set(["https://192.168.10.11:9885","https://im.szputy.com:9885"]);
let vault:LocalVault|undefined, credentials:Credentials|undefined, live:LiveState|undefined, checkedAt=0, checking:Promise<LiveState>|undefined, owner:WebContents|undefined;
let root=configuredRoot(), timer:ReturnType<typeof setInterval>|undefined;
let generation=0;
let storageTransition=false;
let archiveReady=false,syncRevision=0;
let restoreCleanupPending=false;
function restoreOptions(check:()=>Promise<void>,freshSync?:()=>Promise<void>):RestoreTransactionOptions{return {
  check,freshSync,
  protect:async plain=>{if(!await safeStorage.isAsyncEncryptionAvailable())throw new Error('系统密钥保护不可用');await check();const wrapped=await safeStorage.encryptStringAsync(plain.toString('base64'));await check();return wrapped;},
  unprotect:async sealed=>{if(!await safeStorage.isAsyncEncryptionAvailable())throw new Error('系统密钥保护不可用');await check();const plain=await safeStorage.decryptStringAsync(sealed);await check();return Buffer.from(plain.result,'base64');}
};}
function requireArchiveReady(){if(!archiveReady)throw new Error("消息同步尚未完成，本地档案暂不可用");}
const previews=new Map<string,{url:string;proof:string;abort:AbortController;owner:number;userID:string;sessionID:string;epoch:number;generation:number}>();
function trusted(event:IpcMainInvokeEvent) { const url=event.senderFrame?.url || "";if(!url.startsWith("openim38://app/") && !(process.env.VITE_DEV_SERVER_URL && url.startsWith("http://127.0.0.1:7777/")))throw new Error("Untrusted local storage window"); }
export function lockLocalStore(reason="本地档案已锁定，请在线重新登录",reload=true) {
  storeLifecycle.emit('lock');
  generation++;archiveReady=false;syncRevision++;
  clearInterval(timer);timer=undefined;vault?.lock();vault=undefined;credentials=undefined;live=undefined;checkedAt=0;checking=undefined;
  for(const preview of previews.values())preview.abort.abort();previews.clear();
  if(owner && !owner.isDestroyed()){owner.send("puty-store-locked",reason);if(reload)setTimeout(()=>{if(owner && !owner.isDestroyed())owner.reload();},150);}
}
async function fetchSessionState(bound:Credentials):Promise<LiveState> {
  const response=await net.fetch(`${bound.origin}/enterprise/v1/session/state`,{method:"GET",cache:"no-store",redirect:"error",headers:{token:bound.imToken,chatToken:bound.chatToken,platformID:"3",operationID:randomUUID()},signal:AbortSignal.timeout(2500)});
  const result=await response.json() as any;
  if(!response.ok || result?.errCode!==0)throw new Error("账号、网络或终端权限已失效");
  return assertPCSessionState(result.data,bound.userID);
}
async function online(force=false):Promise<LiveState> {
  if(!credentials)throw new Error("请在线登录后打开本地档案");
  if(!force && live && Date.now()-checkedAt<8000 && live.expiresAt>Date.now()/1000)return live;
  if(checking)return checking;
  const bound=credentials,run=generation;
  const operation:Promise<LiveState>=(async()=>{
    const state=await fetchSessionState(bound);
    if(credentials!==bound || run!==generation)throw new Error("账号会话已改变");
    if(live && (live.sessionID!==state.sessionID || live.epoch!==state.epoch))throw new Error("账号安全版本已改变，请重新登录");
    live=state;checkedAt=Date.now();return state;
  })().catch(error=>{if(run===generation && credentials===bound)lockLocalStore("网络或账号授权不可用，本地档案已锁定");throw error;}).finally(()=>{if(checking===operation)checking=undefined;});
  checking=operation;
  return checking;
}
export async function activeVault() {const current=vault,run=generation;await online();if(!current || current!==vault || run!==generation)throw new Error("本地加密档案尚未打开或会话已改变");return current;}
export async function requireSavePermission(){requireArchiveReady();const current=vault,run=generation;const policy=await online(true);if(run!==generation || current!==vault || !current)throw new Error('账号会话已改变');if(!policy.saveAllowed)throw new Error('当前网络或终端未开放附件保存、下载权限');return policy;}
export async function authenticatedResourceHeaders(proof:string){const bound=credentials,run=generation;await online();if(bound!==credentials || run!==generation)throw new Error('账号会话已改变');if(!credentials || !/^[a-f0-9]{64}$/i.test(proof))throw new Error('Invalid resource proof');return {token:credentials.imToken,chatToken:credentials.chatToken,platformID:'3','X-Puty-Resource-Proof':proof};}
async function chooseRoot(parent?:BrowserWindow|null) {
  const selected=await dialog.showOpenDialog(parent || BrowserWindow.getFocusedWindow()!,{title:"选择本地存储目录（建议选择非 C 盘）",properties:["openDirectory","createDirectory"],buttonLabel:"使用此目录"});
  if(selected.canceled || !selected.filePaths[0])throw new Error("请选择本地存储目录后继续");
  const chosen=path.resolve(selected.filePaths[0]);
  const lower=(value:string)=>process.platform==='win32'?value.toLowerCase():value;
  const overlaps=(left:string,right:string)=>lower(left)===lower(right) || lower(left).startsWith(lower(right)+path.sep) || lower(right).startsWith(lower(left)+path.sep);
  if(overlaps(chosen,legacyUserData) || overlaps(chosen,bootstrapDirectory))throw new Error('请选择独立于旧版数据和系统密钥目录的存储位置');
  const selectedInfo=await fs.lstat(chosen).catch(error=>{if(error.code==='ENOENT')return undefined;throw error;});
  if(selectedInfo?.isSymbolicLink())throw new Error('存储目录不能是链接');
  await fs.mkdir(chosen,{recursive:true});const canonical=await validateStorageDirectory(chosen);
  const marker=path.join(canonical,".puty-local-root.json"),names=await fs.readdir(canonical);
  if(!names.includes(".puty-local-root.json") && names.length)throw new Error("请选择一个空目录，避免与其他文件混用");
  if(names.includes(".puty-local-root.json"))await validateStorageRoot(canonical);
  else await atomicWrite(marker,Buffer.from(JSON.stringify({format:"PUTY-LOCAL-ROOT-1",id:randomUUID()})));
  await validateStorageRoot(canonical);
  const probe=path.join(canonical,".write-check-"+randomUUID());let handle:Awaited<ReturnType<typeof fs.open>>|undefined;
  try{handle=await fs.open(probe,"wx",0o600);await handle.writeFile(randomBytes(64));await handle.sync();}finally{await handle?.close();if(handle)await fs.unlink(probe);}
  const space=await fs.statfs(canonical);if(Number(space.bavail)*Number(space.bsize)<128*1024*1024)throw new Error("存储目录可用空间不足 128 MiB");
  return canonical;
}
async function rememberRoot(value:string) { await fs.mkdir(bootstrapDirectory,{recursive:true});await preserveBootstrapConfiguration();await atomicWrite(bootstrapPath,Buffer.from(JSON.stringify({format:1,root:value})));root=value;clearStartupStorageError();await bindPreferencesRoot(value); }
async function repairRoot(event:IpcMainInvokeEvent){
  if(storageTransition)throw new Error('本地存储正在迁移或恢复，请稍后重试');
  storageTransition=true;const previous=vault;lockLocalStore('正在重新选择本地存储目录',false);
  try{
    await previous?.drain();
    const parent=BrowserWindow.fromWebContents(event.sender),selected=await chooseRoot(parent);
    const lower=(value:string)=>process.platform==='win32'?value.toLowerCase():value;
    const overlap=(left:string,right:string)=>lower(left)===lower(right) || lower(left).startsWith(lower(right)+path.sep) || lower(right).startsWith(lower(left)+path.sep);
    if(overlap(selected,legacyUserData) || overlap(selected,bootstrapDirectory))throw new Error('请选择独立于旧版数据和系统密钥目录的存储位置');
    if(root && lower(selected)!==lower(root) && overlap(selected,root))throw new Error('新旧存储目录不能相互包含，原目录仍保留');
    const confirmed=await dialog.showMessageBox(parent || BrowserWindow.getFocusedWindow()!,{type:'warning',title:'重新选择本地存储目录',message:'仅更改本机存储位置，不移动、删除或修复原聊天文件和密钥。',detail:'选择新空目录不会自动恢复旧记录。打开已有档案仍须在线登录并通过原密钥校验；需要恢复时请使用完整加密备份。',buttons:['取消','使用所选目录'],defaultId:0,cancelId:0});
    if(confirmed.response!==1)return {changed:false};
    await validateStorageRoot(selected);await fs.mkdir(bootstrapDirectory,{recursive:true});await preserveBootstrapConfiguration();
    await atomicWrite(bootstrapPath,Buffer.from(JSON.stringify({format:1,root:selected})));root=selected;clearStartupStorageError();
    const preferences=await initProtectedPreferences(selected);
    return {changed:true,directory:selected,preferencesPersistent:preferences.mode==='persistent'};
  }finally{storageTransition=false;}
}
async function migrateLegacy(candidate:LocalVault,assertCurrent:()=>Promise<void>,sender:WebContents){
  // Acceptance fixtures must never inspect or remove the machine's real old profile.
  if(process.env.OPENIM38_TEST_DATA_DIR)return;
  const progress=(message:string)=>{if(!sender.isDestroyed())sender.send('puty-store-migration',message);};
  let phase:LegacyMigrationPhase='inventory';
  try{
  let snapshots=await listLegacySnapshots(root!);await assertCurrent();
  const entries=await fs.readdir(legacyUserData).catch(error=>{if(error.code==='ENOENT')return [];throw error;});
  if(!snapshots.length && entries.length){
    phase='reader';
    progress('正在读取旧版记录，期间请勿关闭客户端…');
    const readout=await readLegacyInChild({root:root!,source:legacyUserData,writersStopped:true,assertCurrent});
    await assertCurrent();phase='seal';progress('正在加密备份并校验旧版数据…');
    const receipt=await preserveLegacyData({root:root!,source:legacyUserData,writersStopped:true,readout,assertCurrent});snapshots=[receipt];
  }
  if(!snapshots.length)return;
  const parents=new Set(snapshots.map(value=>value.parentSnapshotID).filter(Boolean)),leaves=snapshots.filter(value=>!parents.has(value.snapshotID));
  if(leaves.length!==1)throw new Error('存在多份独立旧版数据备份，已保留原件，请先核查迁移来源');
  const snapshot=leaves[0];
  phase='cleanup';
  if(!snapshot.plaintextCleanupComplete){
    progress('加密备份校验完成，正在清理旧版明文缓存…');
  }
  await cleanupLegacyPlaintext({root:root!,source:legacyUserData,currentUserData:app.getPath('userData'),snapshotID:snapshot.snapshotID,writersStopped:true,assertCurrent,nativeSingletonLockHeld:()=>app.hasSingleInstanceLock()});
  phase='import';
  progress('正在迁入当前账号的历史记录与附件…');
  await importLegacyAccount({root:root!,snapshotID:snapshot.snapshotID,accountID:candidate.account,vault:candidate,assertCurrent});
  progress(snapshot.quarantined?'迁移完成；无法确认归属的旧数据已加密保留':'旧版数据迁移完成');
  }catch(error){throw legacyMigrationFailure(phase,error);}
}
async function unlock(input:Credentials,event:IpcMainInvokeEvent) {
  if(storageTransition)throw new Error("本地存储正在迁移或恢复，请稍后重试");
  if(!input || !origins.has(input.origin) || !/^emp_[a-f0-9]{32}$/.test(input.userID) || !input.imToken || !input.chatToken)throw new Error("统一账号会话无效");
  if(vault && credentials?.userID===input.userID && credentials.origin===input.origin){
    const current=vault,run=generation,prior=credentials;if(checking)await checking.catch(()=>{});
    if(run!==generation || credentials!==prior || vault!==current)throw new Error('账号会话已改变');
    try{
      const state=await fetchSessionState(input);
      if(run!==generation || credentials!==prior || vault!==current)throw new Error('账号会话已改变');
      if(live?.sessionID===state.sessionID && live.epoch===state.epoch){credentials={...input};live=state;checkedAt=Date.now();return await current.stats();}
      // A new successful SSO login has a new SID. Reopen under its own lease;
      // existing vault validators remain bound to the previous SID and epoch.
    }catch(error){if(run===generation && credentials===prior)lockLocalStore('账号会话验证失败',false);throw error;}
  }
  const previousStore=vault;lockLocalStore("切换账号",false);owner=event.sender;const bound={...input},run=generation;credentials=bound;let candidate:LocalVault|undefined,key:Buffer|undefined;
  const assertCurrent=()=>{if(run!==generation || credentials!==bound)throw new Error('账号会话已改变');};
  try {
    await previousStore?.drain();assertCurrent();
    const state=await online(true);assertCurrent();const expected={userID:state.userID,sessionID:state.sessionID,epoch:state.epoch};
    const validateOwner=async()=>{
      if(run!==generation || credentials?.userID!==expected.userID)throw new Error('账号会话已改变');
      const current=await online();if(run!==generation || current.userID!==expected.userID || current.sessionID!==expected.sessionID || current.epoch!==expected.epoch)throw new Error('账号会话已改变');
    };
    if(!root){const selected=await chooseRoot(BrowserWindow.fromWebContents(event.sender));assertCurrent();await rememberRoot(selected);assertCurrent();}
    await validateStorageRoot(root!);assertCurrent();await fs.mkdir(path.join(root!,'accounts'),{recursive:true});await validateOwner();const recovered=await recoverRestoreTransaction(root!,input.userID,restoreOptions(validateOwner));restoreCleanupPending=recovered.pendingPrevious;assertCurrent();const accountRoot=path.join(root!,"accounts",input.userID);await fs.mkdir(accountRoot,{recursive:true});assertCurrent();
    const keyFile=path.join(accountRoot,"master.wrap");
    if(!await safeStorage.isAsyncEncryptionAvailable())throw new Error("系统密钥保护不可用，未使用明文密钥降级");assertCurrent();
    try {
      const sealed=await fs.readFile(keyFile);assertCurrent();const decrypted=await safeStorage.decryptStringAsync(sealed);assertCurrent();key=Buffer.from(decrypted.result,"base64");
      if(decrypted.shouldReEncrypt){const wrapped=await safeStorage.encryptStringAsync(decrypted.result);assertCurrent();await atomicWrite(keyFile,wrapped);assertCurrent();}
    }catch(error) {
      assertCurrent();if((error as NodeJS.ErrnoException).code!=="ENOENT")throw new Error("本机密钥无法解锁，请使用加密恢复包");
      const existing=await fs.readdir(path.join(accountRoot,"vault")).catch(error=>{if(error.code==='ENOENT')return [] as string[];throw error;});assertCurrent();if(existing.length)throw new Error("已有档案缺少密钥，请先恢复密钥");
      key=randomBytes(32);const wrapped=await safeStorage.encryptStringAsync(key.toString("base64"));assertCurrent();await atomicWrite(keyFile,wrapped);assertCurrent();
    }
    try {candidate=await LocalVault.open(path.join(accountRoot,"vault"),input.userID,key!,validateOwner);assertCurrent();}finally{key!.fill(0);}
    await migrateLegacy(candidate,validateOwner,event.sender);assertCurrent();
    const stats=await candidate.stats();assertCurrent();vault=candidate;candidate=undefined;
    timer=setInterval(()=>{void online(true).catch(()=>{});},5000);timer.unref();return stats;
  }catch(error){candidate?.lock();if(run===generation && credentials===bound)lockLocalStore("本地档案打开失败",false);throw error;}finally{key?.fill(0);}
}
async function restoreLocalBackup(input:{credentials:Credentials;passphrase:string},event:IpcMainInvokeEvent){
  const auth=input?.credentials;if(!auth || !origins.has(auth.origin) || !/^emp_[a-f0-9]{32}$/.test(auth.userID) || !auth.imToken || !auth.chatToken)throw new Error('统一账号会话无效');
  if(storageTransition)throw new Error('本地存储正在迁移或恢复，请稍后重试');storageTransition=true;
  let restored:Buffer|undefined;const bound={...auth},previousStore=vault,previousGeneration=generation;let cleanupRules:{conversationIDs:string[]|null;beforeMS:number}[]=[],suppressions:MessageSuppression[]=[];
  try{if(previousStore?.account===auth.userID){const saved=await previousStore.restorationPolicy();cleanupRules=saved.ranges;suppressions=saved.suppressions;}if(generation!==previousGeneration)throw new Error('账号会话已改变');}catch(error){storageTransition=false;throw error;}
  lockLocalStore('正在恢复本地数据',false);owner=event.sender;credentials=bound;const run=generation;
  const check=async()=>{if(run!==generation || credentials!==bound)throw new Error('恢复期间账号会话已改变');await online();if(run!==generation || credentials!==bound)throw new Error('恢复期间账号会话已改变');};
  try{
    await previousStore?.drain();await check();if(!root){const selected=await chooseRoot(BrowserWindow.fromWebContents(event.sender));await check();await rememberRoot(selected);}
    await validateStorageRoot(root!);await check();
    const selected=await dialog.showOpenDialog(BrowserWindow.fromWebContents(event.sender)!,{title:'选择包含 manifest.puty 与 recovery.putykey 的完整备份目录',properties:['openDirectory']});await check();
    if(selected.canceled || !selected.filePaths[0])throw new Error('已取消恢复');
    await fs.mkdir(path.join(root!,'accounts'),{recursive:true});await check();const recovered=await recoverRestoreTransaction(root!,auth.userID,restoreOptions(check));restoreCleanupPending=recovered.pendingPrevious;
    if(recovered.state==='promoted')throw new Error('上次恢复尚待完成在线同步，请先打开档案并完成同步后重试');
    const accountRoot=path.join(root!,'accounts',auth.userID),temporary=path.join(root!,'accounts','.restore-'+randomUUID());await fs.mkdir(temporary,{recursive:true});await check();
    restored=await restoreBackup(selected.filePaths[0],temporary,auth.userID,input.passphrase,check);
    const existing=await readExistingRestorationPolicy(accountRoot,auth.userID,restored,async sealed=>Buffer.from((await safeStorage.decryptStringAsync(sealed)).result,'base64'),check);cleanupRules.push(...existing.ranges);suppressions.push(...existing.suppressions);
    const verified=await LocalVault.open(temporary,auth.userID,restored,check);try{await verified.restoreSDK();await preserveCleanupPolicy(verified,cleanupRules,undefined,suppressions);}finally{verified.lock();}await check();
    if(!await safeStorage.isAsyncEncryptionAvailable())throw new Error('系统密钥保护不可用');await check();
    const wrapped=await safeStorage.encryptStringAsync(restored.toString('base64'));await check();
    const staged=path.join(root!,'accounts','.account-'+randomUUID());await fs.mkdir(staged);await fs.rename(temporary,path.join(staged,'vault'));await atomicWrite(path.join(staged,'master.wrap'),wrapped);await check();
    const installed=await installRestoredAccount(root!,auth.userID,staged,restoreOptions(check));restoreCleanupPending=installed.pendingPrevious;
    await check();
  }catch(error){if(run===generation && credentials===bound)lockLocalStore('本地恢复未完成',false);throw error;}
  finally{restored?.fill(0);storageTransition=false;}
  return unlock(auth,event);
}
export function localFileExists(value:string) {
  if(!vault || !archiveReady)return false;
  if(value.startsWith("puty-local://pending/"))return vault.hasPending(value.slice("puty-local://pending/".length).split("/")[0]);
  return Boolean(vault.attachmentForPath(value));
}
async function fileInfoOf(store:LocalVault,value:string){
  if(value.startsWith("puty-local://pending/")){const file=await store.readPending(value.slice("puty-local://pending/".length).split('/')[0]);return {name:file.name,type:file.type,size:file.size,bytes:file.bytes};}
  const file=store.attachmentForPath(value);if(!file)throw new Error('文件不属于当前账号的附件档案');return {name:file.sourceName,type:file.mime,size:file.size,taskID:file.taskID};
}
async function rangeOf(store:LocalVault,value:string,start:number,end:number){
  if(value.startsWith('puty-local://pending/'))return (await store.readPending(value.slice('puty-local://pending/'.length).split('/')[0])).bytes.subarray(start,end+1);
  const file=store.attachmentForPath(value);if(!file)throw new Error('本地附件不存在');return store.readAttachment(file.taskID,start,end);
}
export async function localFileInfo(value:string){requireArchiveReady();return fileInfoOf(await activeVault(),value);}
export async function readLocalRange(value:string,start:number,end:number){requireArchiveReady();return rangeOf(await activeVault(),value,start,end);}
export async function remotePreviewResponse(request:Request):Promise<Response> {
  requireArchiveReady();
  const token=new URL(request.url).pathname.slice("/__remote/".length),preview=previews.get(token);
  if(!preview)return new Response("Preview expired",{status:403});
  await activeVault();if(preview.generation!==generation || preview.userID!==live?.userID || preview.sessionID!==live?.sessionID || preview.epoch!==live?.epoch)return new Response('Preview session changed',{status:403});const range=request.headers.get("Range"),headers:Record<string,string>=await authenticatedResourceHeaders(preview.proof);
  if(range){if(!/^bytes=(?:\d+-\d*|-\d+)$/.test(range))return new Response(null,{status:416});headers.Range=range;}
  const response=await net.fetch(preview.url,{method:request.method==="HEAD"?"HEAD":"GET",headers,cache:"no-store",credentials:"omit",redirect:"error",signal:AbortSignal.any([request.signal,preview.abort.signal])});
  if(previews.get(token)!==preview || preview.generation!==generation){await response.body?.cancel();return new Response('Preview session changed',{status:403});}
  const output=new Headers({"Cache-Control":"no-store","Cross-Origin-Resource-Policy":"same-origin","Content-Type":response.headers.get("Content-Type") || "application/octet-stream"});
  for(const name of ["Content-Length","Content-Range","Accept-Ranges"])if(response.headers.has(name))output.set(name,response.headers.get(name)!);
  return new Response(response.body,{status:response.status,headers:output});
}
async function exportLocalFile(value:string) {
  const store=await activeVault(),run=generation;await requireSavePermission();
  const check=async()=>{if(run!==generation || vault!==store)throw new Error('账号会话已改变');const policy=await online();if(!policy.saveAllowed || run!==generation || vault!==store)throw new Error('附件保存权限或账号会话已改变');};
  const info=await fileInfoOf(store,value),selected=await dialog.showOpenDialog(BrowserWindow.getFocusedWindow()!,{title:"选择另存普通文件的目录",properties:["openDirectory","createDirectory"]});await check();
  if(selected.canceled || !selected.filePaths[0])return;
  const target=await reserveName(await fs.realpath(selected.filePaths[0]),info.name);let handle:Awaited<ReturnType<typeof fs.open>>|undefined,complete=false;
  try{await check();handle=await fs.open(target.path,"r+");if(!info.size)await rangeOf(store,value,0,-1);for(let position=0;position<info.size;position+=1024*1024){await check();const bytes=await rangeOf(store,value,position,Math.min(info.size-1,position+1024*1024-1));await check();await handle.writeFile(bytes);}await handle.sync();await check();complete=true;}
  finally{await handle?.close();if(!complete)await fs.unlink(target.path);}
  await shell.openPath(target.path);return target.path;
}
async function moveRoot(event:IpcMainInvokeEvent) {
  if(storageTransition)throw new Error('本地存储正在迁移或恢复');
  const current=await activeVault(),run=generation,source=await fs.realpath(root!);
  const requireRestoreComplete=async()=>{const entries=await fs.readdir(path.join(source,'.restore-transactions')).catch(error=>{if(error.code==='ENOENT')return [];throw error;});if(entries.length)throw new Error('恢复事务尚待完成，请先在线同步并释放恢复前副本后再迁移目录');};
  await requireRestoreComplete();const destination=await chooseRoot(BrowserWindow.fromWebContents(event.sender));
  if(run!==generation || vault!==current)throw new Error('账号会话已改变');if(source===destination)return {directory:source};
  if(destination.startsWith(source+path.sep) || source.startsWith(destination+path.sep))throw new Error('新旧存储目录不能相互包含');
  const existing=await fs.readdir(destination);if(existing.some(name=>name!=='.puty-local-root.json'))throw new Error('目标目录已包含档案，请选择空目录');
  const own=JSON.parse(await fs.readFile(path.join(source,'.puty-local-root.json'),'utf8'));if(own.format!=='PUTY-LOCAL-ROOT-1')throw new Error('旧目录缺少所有权标记');
  await requireRestoreComplete();if(run!==generation || vault!==current || storageTransition)throw new Error('账号会话或本地存储状态已改变');storageTransition=true;lockLocalStore('正在迁移存储目录',false);const transitionRun=generation;
  const copied:{relative:string;sha256:string}[]=[];
  const check=()=>{if(generation!==transitionRun)throw new Error('迁移已中止，原存储目录仍保留');};
  async function copy(directory:string,relative='') {
    for(const entry of await fs.readdir(directory,{withFileTypes:true})){
      check();if(entry.name==='.runtime' || entry.name==='.puty-local-root.json')continue;if(entry.isSymbolicLink())throw new Error('存储目录含链接，已停止迁移');
      const child=path.join(relative,entry.name),from=path.join(source,child),to=path.join(destination,child);
      if(entry.isDirectory()){await fs.mkdir(to,{recursive:true});await copy(from,child);}else if(entry.isFile()){await fs.copyFile(from,to);const handle=await fs.open(to,'r+');await handle.sync();await handle.close();const sha256=await fileHash(from);if(await fileHash(to)!==sha256)throw new Error('迁移文件校验失败');copied.push({relative:child,sha256});}
    }
  }
  try{
    await current.drain();await flushPreferences();check();await copy(source);check();await rememberRoot(destination);
    for(const file of copied){check();const from=path.resolve(source,file.relative);if(!from.startsWith(source+path.sep))throw new Error('Invalid migration source');if(await fileHash(from)===file.sha256)await fs.unlink(from);}
    app.relaunch();app.exit(0);return {directory:destination,restart:true};
  }finally{storageTransition=false;}
}
export function registerLocalStore() {
  ipcMain.on("puty-file-exists",(event,value)=>{event.returnValue=typeof value==="string" && localFileExists(value);});
  ipcMain.handle("puty-store",async(event,operation:string,data:any)=>{
    trusted(event);
    if(operation==="unlock")return unlock(data,event);
    if(operation==="repair-directory")return repairRoot(event);
    if(operation==="restore-backup")return restoreLocalBackup(data,event);
    if(operation==="sync-state"){
      if(data?.state==='ready'){const revision=syncRevision,run=generation,current=vault,bound=credentials;await online(true);if(run!==generation || revision!==syncRevision || !current || !bound || vault!==current || credentials!==bound)throw new Error('同步会话已改变');archiveReady=true;
        const check=async()=>{if(run!==generation || revision!==syncRevision || vault!==current || credentials!==bound)throw new Error('同步会话已改变');await online();if(run!==generation || revision!==syncRevision || vault!==current || credentials!==bound)throw new Error('同步会话已改变');};
        const result=await collectRestorePrevious(root!,current.account,restoreOptions(check,async()=>{await check();requireArchiveReady();}));await check();requireArchiveReady();restoreCleanupPending=result.pendingPrevious;return true;}
      archiveReady=false;syncRevision++;for(const preview of previews.values())preview.abort.abort();previews.clear();storeLifecycle.emit('sync-lock');
      if(data?.state==='failed')lockLocalStore('消息同步失败，本地档案已锁定，请重新登录');return false;
    }
    if(operation==="lock"){lockLocalStore("已退出账号",false);return true;}
    if(operation==="config")return {directory:root || "",configured:Boolean(root),encrypted:true};
    if(operation==="choose-directory")return moveRoot(event);
    if(operation==="remote-preview-release") { const prefix="openim38://app/__remote/";if(typeof data?.url==="string" && data.url.startsWith(prefix)){const id=data.url.slice(prefix.length),preview=previews.get(id);if(preview?.owner===event.sender.id){preview.abort.abort();previews.delete(id);}}return true; }
    const store=await activeVault(),run=generation;
    const checkExport=async(force=false)=>{requireArchiveReady();if(run!==generation||vault!==store)throw new Error('账号会话已改变');const policy=await online(force);if(run!==generation||vault!==store)throw new Error('账号会话已改变');if(!policy.saveAllowed)throw new Error('当前网络或终端未开放附件保存、下载权限');};
    try{
      const execute=async()=>{switch(operation){
        case "remote-preview":{requireArchiveReady();const url=new URL(data.url);if(!origins.has(url.origin) || url.origin!==credentials?.origin || !/^\/(?:enterprise\/v1\/)?attachments\//.test(url.pathname) || !/^[a-f0-9]{64}$/i.test(data.proof) || url.username || url.password || url.hash || !live)throw new Error("Invalid resource preview");const id=randomBytes(32).toString("hex");previews.set(id,{url:url.href,proof:data.proof,abort:new AbortController(),owner:event.sender.id,userID:live.userID,sessionID:live.sessionID,epoch:live.epoch,generation});return {url:`openim38://app/__remote/${id}`};}
        case "sdk-restore":return store.restoreSDK();
        case "sdk-commit":return store.commit(data);
        case "sdk-checkpoint":return store.checkpoint(Buffer.from(data.snapshot,"base64"));
        case "cleanup-policy":return store.cleanupPolicy();
        case "cleanup-preview":return store.previewCleanup(data.scope || data,data.workingMessages);
        case "cleanup-commit":{storeLifecycle.emit("sync-lock");for(const preview of previews.values())preview.abort.abort();previews.clear();return store.clean(data.scope,Buffer.from(data.snapshot,"base64"),data.workingMessages);}
        case "history":requireArchiveReady();return store.history(data.conversationID,data.beforeMS,data.count,data.startClientMsgID);
        case "archive-query":return store.sdkQuery(data.name,data.args,data.workingRows);
        case "stats":return {...await store.stats(),restoreCleanupPending};
        case "preferences":return store.preferences(data);
        case "stash-file":return store.stashFile(data.name,data.type,new Uint8Array(data.bytes));
        case "release-pending":return store.releasePending(data.ids,data.clientMsgID);
        case "file-info":{const info=await localFileInfo(data.path);if(info.bytes)return {name:info.name,type:info.type,bytes:info.bytes};const parts:Buffer[]=[];if(!info.size)await readLocalRange(data.path,0,-1);for(let offset=0;offset<info.size;offset+=1024*1024)parts.push(await readLocalRange(data.path,offset,Math.min(info.size-1,offset+1024*1024-1)));return {name:info.name,type:info.type,bytes:Buffer.concat(parts)};}
        case "export-file":return exportLocalFile(data.path);
        case "show-folder":{const policy=await online();if(!policy.saveAllowed)throw new Error("未开放附件保存权限");const info=store.attachmentForPath(data.path);if(info)shell.showItemInFolder(path.join(store.root,info.file));return;}
        case "export-recovery":{await checkExport(true);const selected=await dialog.showSaveDialog(BrowserWindow.getFocusedWindow()!,{title:"保存加密密钥恢复包",defaultPath:"PUTY_IM密钥恢复包.putykey"});if(selected.canceled || !selected.filePath)return;await checkExport(true);const bytes=await store.recovery(data.passphrase);const filePath=await writeRecoveryExport(selected.filePath,bytes,checkExport);return {saved:true,filePath};}
        case "export-backup":{await checkExport(true);const selected=await dialog.showOpenDialog(BrowserWindow.fromWebContents(event.sender)!,{title:'选择完整加密备份的保存位置',properties:['openDirectory','createDirectory']});if(selected.canceled || !selected.filePaths[0])return;await checkExport(true);return store.backup(selected.filePaths[0],data.passphrase,checkExport);}
        default:throw new Error("Unsupported local storage operation");
      }};
      const result=await execute();if(generation!==run || vault!==store)throw new Error('账号会话已改变，已拒绝过期本地数据');return result;
    }catch(error){if(["export-file","show-folder"].includes(operation))dialog.showErrorBox("附件操作失败",error instanceof Error?error.message:"附件操作失败");if(run===generation && vault===store && ["sdk-commit","sdk-checkpoint","cleanup-commit"].includes(operation))lockLocalStore("本地存储写入失败，档案已锁定以保护数据");throw error;}
  });
}
