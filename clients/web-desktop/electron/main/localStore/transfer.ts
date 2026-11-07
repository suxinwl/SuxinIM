import {createHash,randomUUID} from 'node:crypto';
import {promises as fs} from 'node:fs';
import type {FileHandle} from 'node:fs/promises';
import {CHUNK_SIZE,createHeader,decryptFrame,encryptFrame,objectKey,parseHeader,type ObjectContext} from './crypto';

export type DownloadSource = {source:AsyncIterable<Uint8Array>;start:number;total?:number;version?:string};
export type DownloadFactory = (offset:number,version?:string)=>Promise<DownloadSource>;
export async function inspectEncryptedFile(master:Uint8Array,context:ObjectContext,file:string,check:()=>Promise<void>){
  const handle=await fs.open(file,'r'),key=objectKey(master,context),digest=createHash('sha256');
  try{
    const size=(await handle.stat()).size,first=Buffer.alloc(12);if((await handle.read(first,0,12,0)).bytesRead!==12)throw new Error('加密文件头不完整');
    const length=first.readUInt32BE(8);if(length<2 || length>65536)throw new Error('加密文件头无效');const rest=Buffer.alloc(length);if((await handle.read(rest,0,length,12)).bytesRead!==length)throw new Error('加密文件头不完整');
    const parsed=parseHeader(Buffer.concat([first,rest]));if(parsed.header.account!==context.account || parsed.header.objectID!==context.objectID || parsed.header.type!==context.type)throw new Error('加密文件上下文不匹配');
    let cursor=parsed.offset,index=0,plainSize=0,short=false;
    while(cursor+20<=size){
      await check();const prefix=Buffer.alloc(4);await handle.read(prefix,0,4,cursor);const length=prefix.readUInt32BE(0);if(length>CHUNK_SIZE || cursor+length+20>size || short && length)throw new Error('加密文件分块不完整');
      const body=Buffer.alloc(length+16);if((await handle.read(body,0,body.length,cursor+4)).bytesRead!==body.length)throw new Error('加密文件分块不完整');const plain=decryptFrame(key,parsed.header,parsed.bytes,index++,body.subarray(0,length),body.subarray(length));cursor+=length+20;
      if(!length){if(cursor!==size)throw new Error('加密文件尾部存在额外数据');await check();return {size:plainSize,sha256:digest.digest('hex')};}
      digest.update(plain);plainSize+=length;if(length<CHUNK_SIZE)short=true;
    }
    throw new Error('加密文件缺少完整结束标记');
  }finally{key.fill(0);await handle.close();}
}

// Only authenticated, complete frames survive interruption. Before appending we
// rotate the nonce prefix and re-encrypt the retained frames: even a torn write
// can never cause reuse of an AES-GCM nonce with different plaintext.
export async function encryptedTransfer(master:Uint8Array,context:ObjectContext,partial:string,source:AsyncIterable<Uint8Array>|DownloadFactory,options:{version?:string;expectedSize?:number;expectedSHA256?:string;check:()=>Promise<void>;saveVersion:(version?:string)=>Promise<void>}) {
  const key=objectKey(master,context),headerInfo=createHeader(context),digest=createHash('sha256');
  const next=partial+'.next-'+randomUUID();let handle:FileHandle|undefined,offset=0,index=0,closed=false;
  try {
    handle=await fs.open(next,'wx',0o600);await handle.writeFile(headerInfo.prefix);
    if(typeof source==='function' && options.version) {
      const old=await fs.open(partial,'r').catch(error=>{if(error.code==='ENOENT')return undefined;throw error;});
      if(old)try {
        const size=(await old.stat()).size,first=Buffer.alloc(12);
        if((await old.read(first,0,12,0)).bytesRead===12) {
          const length=first.readUInt32BE(8);if(length<2 || length>65536)throw new Error('续传密文头无效');
          const rest=Buffer.alloc(length);if((await old.read(rest,0,length,12)).bytesRead!==length)throw new Error('续传密文头不完整');
          const parsed=parseHeader(Buffer.concat([first,rest]));
          if(parsed.header.account!==context.account || parsed.header.objectID!==context.objectID || parsed.header.type!==context.type)throw new Error('续传文件不属于此下载任务');
          let cursor=parsed.offset,oldIndex=0;
          while(cursor+4<=size) {
            await options.check();
            const prefix=Buffer.alloc(4);await old.read(prefix,0,4,cursor);const length=prefix.readUInt32BE(0);
            if(length>CHUNK_SIZE)throw new Error('续传密文分块无效');
            if(cursor+length+20>size)break; // torn final frame, never acknowledged
            const frame=Buffer.alloc(length+16);await old.read(frame,0,frame.length,cursor+4);
            const plain=decryptFrame(key,parsed.header,parsed.bytes,oldIndex++,frame.subarray(0,length),frame.subarray(length));
            if(length!==CHUNK_SIZE)break; // re-fetch a short final frame
            await handle.writeFile(encryptFrame(key,headerInfo.header,headerInfo.bytes,index++,plain));digest.update(plain);offset+=plain.length;cursor+=length+20;
          }
        }
      }finally{await old.close();}
    }
    await handle.sync();await handle.close();handle=undefined;await fs.rename(next,partial);handle=await fs.open(partial,'a',0o600);
    let remote:DownloadSource=typeof source==='function'?await source(offset,options.version):{source,start:0};
    if(remote.start!==offset) {
      if(remote.start!==0)throw new Error('服务器续传范围不匹配');
      await handle.close();offset=0;index=0;
      // The same prefix has only been used with the identical retained prefix;
      // reset requires a fresh header because the remote version may differ.
      Object.assign(headerInfo,createHeader(context));handle=await fs.open(partial,'w',0o600);await handle.writeFile(headerInfo.prefix);
    }
    const hash=offset?digest:createHash('sha256');
    await options.saveVersion(remote.version);
    const expected=options.expectedSize ?? remote.total;let size=offset,pending=Buffer.alloc(0),checked=0;
    for await(const value of remote.source) {
      await options.check();checked=Date.now();
      const chunk=Buffer.from(value);hash.update(chunk);size+=chunk.length;pending=Buffer.concat([pending,chunk]);
      if(expected!==undefined && size>expected)throw new Error('附件超过声明大小');
      while(pending.length>=CHUNK_SIZE){await handle.writeFile(encryptFrame(key,headerInfo.header,headerInfo.bytes,index++,pending.subarray(0,CHUNK_SIZE)));pending=pending.subarray(CHUNK_SIZE);await handle.sync();}
    }
    if(expected!==undefined && size!==expected)throw new Error('附件大小不一致');
    const sha256=hash.digest('hex');if(options.expectedSHA256 && sha256!==options.expectedSHA256.toLowerCase())throw new Error('附件校验值不一致');
    if(pending.length)await handle.writeFile(encryptFrame(key,headerInfo.header,headerInfo.bytes,index++,pending));
    await handle.writeFile(encryptFrame(key,headerInfo.header,headerInfo.bytes,index,Buffer.alloc(0)));await options.check();await handle.sync();await handle.close();closed=true;
    return {size,sha256};
  }finally{key.fill(0);if(!closed)await handle?.sync().catch(()=>{});await handle?.close().catch(()=>{});await fs.unlink(next).catch(error=>{if(error.code!=='ENOENT')throw error;});}
}
