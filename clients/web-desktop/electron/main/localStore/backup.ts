import {createHash,randomUUID} from 'node:crypto';
import {createReadStream,promises as fs} from 'node:fs';
import path from 'node:path';
import {FORMAT,exportRecovery,importRecovery,seal,unseal} from './crypto';
import {numberedName} from './names';

export async function writeRecoveryExport(selected:string,bytes:Uint8Array,check:()=>Promise<void>){
  await check();const parent=await fs.realpath(path.dirname(selected));
  for(let number=0;number<100000;number++){
    const output=path.join(parent,numberedName(path.basename(selected),number));
    let handle:Awaited<ReturnType<typeof fs.open>>;
    try{handle=await fs.open(output,'wx',0o600);}catch(error){if((error as NodeJS.ErrnoException).code==='EEXIST')continue;throw error;}
    let owned:{dev:number;ino:number}|undefined;
    try{
      try{owned=await handle.stat();for(let offset=0;offset<bytes.length;){await check();const end=Math.min(bytes.length,offset+65536),written=await handle.write(bytes,offset,end-offset);if(written.bytesWritten<=0)throw new Error('恢复包写入未完成');offset+=written.bytesWritten;}await check();await handle.sync();}finally{await handle.close();}
      await check();return output;
    }catch(error){
      try{const current=await fs.lstat(output);if(owned&&current.isFile()&&!current.isSymbolicLink()&&current.nlink===1&&current.dev===owned.dev&&current.ino===owned.ino&&await fs.realpath(parent)===parent&&await fs.realpath(output)===output)await fs.unlink(output);}catch{}
      throw error;
    }
  }
  throw new Error('同名恢复包过多，无法分配文件名');
}

export async function fileHash(file:string,check?:()=>Promise<void>){const hash=createHash('sha256');for await(const bytes of createReadStream(file,{highWaterMark:65536})){await check?.();hash.update(bytes);}return hash.digest('hex');}
async function checkedFile(root:string,relative:string){
  if(!relative || relative.includes('\\') || relative.includes(':') || relative.split('/').some(v=>!v || v==='.' || v==='..'))throw new Error('备份路径无效');
  const file=path.resolve(root,relative);if(!file.startsWith(root+path.sep))throw new Error('备份路径超出目录');
  let cursor=root;for(const part of relative.split('/')){cursor=path.join(cursor,part);const stat=await fs.lstat(cursor);if(stat.isSymbolicLink())throw new Error('备份不能包含链接');}
  if(!(await fs.stat(file)).isFile())throw new Error('备份对象不是文件');return file;
}
export async function createBackup(root:string,account:string,master:Uint8Array,destination:string,files:string[],passphrase:string,check:()=>Promise<void>){
  await check();const parent=await fs.realpath(destination),output=path.join(parent,'PUTY_IM-'+new Date().toISOString().replace(/[:.]/g,'-')+'-'+randomUUID().slice(0,8));await fs.mkdir(output);const owned=await fs.lstat(output);
  const entries:{relative:string;size:number;sha256:string}[]=[];
  try{
  for(const relative of [...new Set(files)]){
    await check();const from=await checkedFile(root,relative),to=path.join(output,'data',relative);await fs.mkdir(path.dirname(to),{recursive:true});
    const handle=await fs.open(to,'wx',0o600);try{for await(const bytes of createReadStream(from,{highWaterMark:65536})){await check();let offset=0;while(offset<bytes.length){await check();const written=await handle.write(bytes,offset,bytes.length-offset);if(written.bytesWritten<=0)throw new Error('备份写入未完成');offset+=written.bytesWritten;}}await check();await handle.sync();}finally{await handle.close();}
    const source=await fileHash(from,check),copied=await fileHash(to,check);if(source!==copied)throw new Error('备份复制校验失败');entries.push({relative,size:(await fs.stat(to)).size,sha256:copied});
  }
  await check();
  const manifest=seal(master,{account,type:'backup-manifest',objectID:'manifest'},Buffer.from(JSON.stringify({format:FORMAT,platform:'desktop-sqljs',account,createdAt:new Date().toISOString(),files:entries})));
  for(const [name,bytes] of [['manifest.puty',manifest],['recovery.putykey',exportRecovery(master,account,passphrase)]] as const){await check();const handle=await fs.open(path.join(output,name),'wx',0o600);try{await handle.writeFile(bytes);await handle.sync();}finally{await handle.close();}}
  await check();
  return {directory:output,files:entries.length,bytes:entries.reduce((n,v)=>n+v.size,0)};
  }catch(error){
    // Remove only this job's newly allocated child. A replaced directory or
    // redirected parent is retained rather than traversed during cleanup.
    try{const current=await fs.lstat(output);if(!current.isSymbolicLink()&&current.isDirectory()&&current.dev===owned.dev&&current.ino===owned.ino&&await fs.realpath(output)===output&&await fs.realpath(parent)===parent&&output.startsWith(parent+path.sep))await fs.rm(output,{recursive:true});}catch{}
    throw error;
  }
}
export async function restoreBackup(directory:string,destination:string,account:string,passphrase:string,check:()=>Promise<void>){
  const root=await fs.realpath(directory);if((await fs.lstat(root)).isSymbolicLink())throw new Error('备份目录不能使用链接');
  const recovery=await checkedFile(root,'recovery.putykey');if((await fs.stat(recovery)).size>65536)throw new Error('恢复包过大');
  const master=importRecovery(await fs.readFile(recovery),account,passphrase);
  try {
    const manifestPath=await checkedFile(root,'manifest.puty');if((await fs.stat(manifestPath)).size>16*1024*1024)throw new Error('备份清单过大');
    const manifest=JSON.parse(unseal(master,{account,type:'backup-manifest',objectID:'manifest'},await fs.readFile(manifestPath)).toString());
    if(manifest.format!==FORMAT || manifest.platform!=='desktop-sqljs' || manifest.account!==account || !Array.isArray(manifest.files) || manifest.files.length>1000000)throw new Error('备份格式或客户端存储类型不匹配，请使用 PC 完整备份');
    if((await fs.readdir(destination)).length)throw new Error('恢复目标必须为空目录');
    const seen=new Set<string>();
    for(const entry of manifest.files){
      await check();if(seen.has(entry.relative))throw new Error('备份包含重复路径');seen.add(entry.relative);
      const from=await checkedFile(path.join(root,'data'),entry.relative),to=path.resolve(destination,entry.relative);
      if(!to.startsWith(destination+path.sep) || !Number.isSafeInteger(entry.size) || entry.size<0 || !/^[a-f0-9]{64}$/.test(entry.sha256))throw new Error('备份对象无效');
      if((await fs.stat(from)).size!==entry.size || await fileHash(from)!==entry.sha256)throw new Error('备份文件校验失败');
      await fs.mkdir(path.dirname(to),{recursive:true});await fs.copyFile(from,to);const handle=await fs.open(to,'r+');await handle.sync();await handle.close();if(await fileHash(to)!==entry.sha256)throw new Error('恢复文件校验失败');
    }
    await check();return master;
  }catch(error){master.fill(0);throw error;}
}
