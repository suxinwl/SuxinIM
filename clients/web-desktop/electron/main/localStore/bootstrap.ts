import { app } from "electron";
import { readFileSync, existsSync, mkdirSync, promises as fs } from "node:fs";
import path from "node:path";
import {createHash,randomUUID} from 'node:crypto';
export const legacyUserData = process.env.OPENIM38_TEST_DATA_DIR ? path.join(process.env.OPENIM38_TEST_DATA_DIR,'legacy-source') : path.join(app.getPath("appData"), "openim-enterprise-upgrade");
export const bootstrapDirectory = path.join(app.getPath("appData"), "openim-enterprise-upgrade-bootstrap");
export const bootstrapPath = path.join(bootstrapDirectory,"storage.json");
export let startupStorageError:string|undefined;
export function clearStartupStorageError(){startupStorageError=undefined;}
export async function validateStorageDirectory(directory:string):Promise<string>{
  if(!path.isAbsolute(directory) || path.resolve(directory)!==directory)throw new Error('本地存储目录无效，请重新选择');
  let current=path.parse(directory).root;
  for(const component of path.relative(current,directory).split(path.sep).filter(Boolean)){
    current=path.join(current,component);const info=await fs.lstat(current);
    if(!info.isDirectory() || info.isSymbolicLink())throw new Error('本地存储目录不能包含链接');
  }
  return fs.realpath(directory);
}
export async function validateStorageRoot(directory:string):Promise<string>{
  const canonical=await validateStorageDirectory(directory),marker=path.join(canonical,'.puty-local-root.json'),info=await fs.lstat(marker);
  if(!info.isFile() || info.isSymbolicLink() || info.nlink!==1 || info.size>4096)throw new Error('存储目录标记无效，请保留原目录并重新选择');
  const value=JSON.parse(await fs.readFile(marker,'utf8'));
  if(value.format!=='PUTY-LOCAL-ROOT-1' || typeof value.id!=='string' || !/^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/i.test(value.id))throw new Error('存储目录标记无效，请保留原目录并重新选择');
  return canonical;
}
// This archives only the location document. No vault, key or old profile is read.
export async function preserveBootstrapConfiguration():Promise<void>{
  await validateStorageDirectory(bootstrapDirectory);
  const info=await fs.lstat(bootstrapPath).catch(error=>{if(error.code==='ENOENT')return undefined;throw error;});
  if(!info)return;
  if(!info.isFile() || info.isSymbolicLink() || info.nlink!==1 || info.size>65536)throw new Error('原存储配置不是可安全备份的普通文件，请保留并检查');
  const before=await fs.readFile(bootstrapPath),sha=createHash('sha256').update(before).digest('hex');
  const target=path.join(bootstrapDirectory,'storage.before-repair-'+Date.now()+'-'+randomUUID()+'.json'),handle=await fs.open(target,'wx',0o600);
  try{await handle.writeFile(before);await handle.sync();}finally{await handle.close();}
  if(createHash('sha256').update(await fs.readFile(target)).digest('hex')!==sha || !before.equals(await fs.readFile(bootstrapPath)))throw new Error('原存储配置备份校验失败，位置未更改');
  if(process.platform!=='win32'){const parent=await fs.open(bootstrapDirectory,'r');try{await parent.sync();}finally{await parent.close();}}
}
export function configuredRoot(): string | undefined {
  if(process.env.OPENIM38_TEST_DATA_DIR)return path.join(process.env.OPENIM38_TEST_DATA_DIR,"encrypted-data");
  try { const value=JSON.parse(readFileSync(bootstrapPath,"utf8"));if(value.format!==1 || typeof value.root!=="string" || path.resolve(value.root)!==value.root)throw new Error('Invalid local storage configuration');return value.root; } catch(error){if((error as NodeJS.ErrnoException).code!=='ENOENT')startupStorageError='本地存储配置无法读取，请检查配置权限或从备份恢复。';return;}
}
// Keep Chromium's OS-key protector identity stable across data-root moves.
// This profile hosts no message renderer: every business window uses an
// in-memory, cache-disabled partition and all business writes go to the vault.
export function runtimeUserData(){
  const root=configuredRoot();if(root && !existsSync(root))startupStorageError='已配置的本地存储目录不可用：'+root+'。请连接原磁盘并检查读写权限后重新启动。';
  const result=process.env.OPENIM38_TEST_DATA_DIR || path.join(bootstrapDirectory,'.os-profile');mkdirSync(result,{recursive:true});return result;
}
