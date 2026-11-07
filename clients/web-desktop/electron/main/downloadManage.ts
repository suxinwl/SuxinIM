import { createHash,randomUUID } from "node:crypto";
import path from "node:path";
import { ipcMain,net } from "electron";
import type {AttachmentDownloadRequest} from "../../src/im/attachment-download";
import {activeVault,authenticatedResourceHeaders,requireSavePermission,storeLifecycle} from "./localStore/runtime";
import {safeSourceName} from "./localStore/names";
type Transfer={request:AttachmentDownloadRequest;controller:AbortController;state:"running"|"paused"|"cancelled";flight?:Promise<void>};
const origins=new Set(["https://192.168.10.11:9885","https://im.szputy.com:9885"]);
function sourceName(header:string|null,fallback:string) {
  if(header){const utf=/filename\*=UTF-8''([^;]+)/i.exec(header);if(utf)try{return safeSourceName(decodeURIComponent(utf[1]));}catch{}
    const quoted=/filename="([^"]+)"/i.exec(header),plain=/filename=([^;]+)/i.exec(header);if(quoted || plain)return safeSourceName((quoted || plain)![1].trim());}
  return safeSourceName(fallback);
}
export const initDownloadManage=(webContents:Electron.WebContents)=>{
  const transfers=new Map<string,Transfer>();
  const send=(name:string,...args:any[])=>{if(!webContents.isDestroyed())webContents.send(name,...args);};
  const execute=async(transfer:Transfer)=>{
    const request=transfer.request;
    try{
      await requireSavePermission();const store=await activeVault();
      const response=await net.fetch(request.url,{method:"HEAD",cache:"no-store",credentials:"omit",redirect:"error",headers:await authenticatedResourceHeaders(request.resourceProof!),signal:transfer.controller.signal});
      if(!response.ok)throw new Error("附件下载授权失效或网络不可用");
      const url=new URL(request.url),name=request.fileName || sourceName(response.headers.get("Content-Disposition"),decodeURIComponent(url.pathname.split('/').pop() || 'attachment'));
      const taskID=request.taskID || createHash("sha256").update(request.url).digest("hex"),conversationID=request.conversationID || "local_downloads",clientMsgID=request.clientMsgID || taskID;
      request.taskID=taskID;
      const reserved=await store.reserveAttachment({taskID,name,messageRefs:[{conversationID,clientMsgID}],mime:response.headers.get("Content-Type") || undefined});
      if(reserved.state==='ready'){send('downloadSuccess',request.url,path.join(store.root,reserved.file));return;}
      const total=request.fileSize ?? (response.headers.has('Content-Length')?Number(response.headers.get('Content-Length')):undefined);
      const saved=await store.writeAttachment(taskID,async(offset,version)=>{
        const headers:Record<string,string>=await authenticatedResourceHeaders(request.resourceProof!);
        if(offset && version){headers.Range=`bytes=${offset}-`;headers['If-Range']=version;}
        const remote=await net.fetch(request.url,{method:'GET',cache:'no-store',credentials:'omit',redirect:'error',headers,signal:transfer.controller.signal});
        if(!remote.ok || !remote.body)throw new Error('附件下载授权失效或网络不可用');
        const validator=remote.headers.get('ETag') || remote.headers.get('Last-Modified') || undefined;
        let start=0;
        if(remote.status===206){const range=/^bytes (\d+)-(\d+)\/(\d+)$/.exec(remote.headers.get('Content-Range') || '');if(!range || Number(range[1])!==offset || validator!==version || total!==undefined && Number(range[3])!==total)throw new Error('服务器续传范围或文件版本不匹配');start=offset;}
        else if(remote.status!==200)throw new Error('服务器未返回有效的附件内容');
        let received=start,lastProgress=0;
        async function* chunks(){try{for await(const chunk of remote.body as any){if(transfer.state!=="running")throw new Error("下载已暂停或取消");const bytes=new Uint8Array(chunk);received+=bytes.length;if(Date.now()-lastProgress>150){send('downloadProgress',request.url,total?Math.min(99,Math.floor(received*100/total)):0);lastProgress=Date.now();}yield bytes;}}finally{await remote.body?.cancel().catch(()=>{});}}
        return {source:chunks(),start,total,version:validator};
      },total);
      send('downloadProgress',request.url,100);send('downloadSuccess',request.url,path.join(store.root,saved.file));
    }catch(error){if(transfer.state==='running')send('downloadFailed',request.url,error instanceof Error?error.message:'附件下载失败');}
    finally{if(transfer.state!=='paused' && transfers.get(request.url)===transfer)transfers.delete(request.url);}
  };
  const start=async(event:Electron.IpcMainInvokeEvent,input:AttachmentDownloadRequest)=>{
    if(event.sender!==webContents)throw new Error("Invalid attachment window");
    const url=new URL(input.url);
    if(!origins.has(url.origin) || url.username || url.password || !/^\/(?:enterprise\/v1\/)?attachments\//.test(url.pathname) || !/^[a-f0-9]{64}$/i.test(input.resourceProof || ''))throw new Error("附件必须通过当前会话的保存授权");
    if(transfers.has(input.url))return;
    await requireSavePermission();
    const transfer:Transfer={request:{...input,taskID:input.taskID || randomUUID()},controller:new AbortController(),state:'running'};
    transfers.set(input.url,transfer);transfer.flight=execute(transfer);
  };
  ipcMain.handle('startDownload',start);
  ipcMain.handle('pauseDownload',(event,url:string)=>{if(event.sender!==webContents)return;const transfer=transfers.get(url);if(transfer){transfer.state='paused';transfer.controller.abort();send('downloadPaused',url);}});
  ipcMain.handle('resumeDownload',async(event,url:string)=>{if(event.sender!==webContents)return;const transfer=transfers.get(url);if(transfer?.state==='paused'){await transfer.flight;if(transfers.get(url)!==transfer || transfer.state!=='paused')return;transfer.state='running';transfer.controller=new AbortController();transfer.flight=execute(transfer);}});
  ipcMain.handle('cancelDownload',(event,url:string)=>{if(event.sender!==webContents)return;const transfer=transfers.get(url);if(transfer){transfer.state='cancelled';transfer.controller.abort();transfers.delete(url);}send('downloadCancel',url);});
  const abort=()=>{for(const transfer of transfers.values()){transfer.state='cancelled';transfer.controller.abort();}transfers.clear();};
  storeLifecycle.on('lock',abort);storeLifecycle.on('sync-lock',abort);
  return()=>{abort();storeLifecycle.off('lock',abort);storeLifecycle.off('sync-lock',abort);for(const name of ['startDownload','pauseDownload','resumeDownload','cancelDownload'])ipcMain.removeHandler(name);};
};
