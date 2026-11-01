import { protocol,session,type Session } from "electron";
import { promises as fs,createReadStream } from "node:fs";
import { Readable } from "node:stream";
import { resolve, sep, extname } from "node:path";
protocol.registerSchemesAsPrivileged([{scheme:"openim38",privileges:{standard:true,secure:true,supportFetchAPI:true,corsEnabled:true,stream:true}}]);
const registeredSessions=new WeakSet<Session>();
export function registerLocalProtocol(target:Session=session.defaultSession){
 if(registeredSessions.has(target))return;registeredSessions.add(target);
 const root=resolve(global.pathConfig.distPath);
 const types:Record<string,string>={".html":"text/html; charset=utf-8",".js":"text/javascript",".css":"text/css",".json":"application/json",".wasm":"application/wasm",".svg":"image/svg+xml",".png":"image/png",".jpg":"image/jpeg",".ico":"image/x-icon",".woff2":"font/woff2",".mp3":"audio/mpeg",".mp4":"video/mp4",".webm":"video/webm",".jpeg":"image/jpeg",".gif":"image/gif",".webp":"image/webp"};
 target.protocol.handle("openim38",async(request)=>{
  try {
   const url=new URL(request.url);let path:string;let allowed:string[];
   if(url.host!=="app")return new Response("Forbidden",{status:403});
   if(url.pathname==="/__local") {
    path=resolve(url.searchParams.get("path") || "");
    allowed=[global.pathConfig.fileCachePath,global.pathConfig.sentFileCachePath,global.pathConfig.imageCachePath,global.pathConfig.videoCachePath,global.pathConfig.avatarCachePath].filter(Boolean);
   } else if(url.pathname.startsWith("/__extra/")) {
    path=resolve(global.pathConfig.extraResourcesPath,"."+decodeURIComponent(url.pathname.slice("/__extra".length)));allowed=[global.pathConfig.extraResourcesPath];
   } else {path=resolve(root,"."+decodeURIComponent(url.pathname));allowed=[root];}
   const real=await fs.realpath(path===root?resolve(root,"index.html"):path);
   let permitted=false;for(const base of allowed){const resolved=await fs.realpath(base).catch(()=>resolve(base));if(real===resolved || real.startsWith(resolved+sep)){permitted=true;break;}}
   if(!permitted)return new Response("Forbidden",{status:403});
   const info=await fs.stat(real);if(!info.isFile())return new Response("Not found",{status:404});
   const headers:Record<string,string>={"Content-Type":types[extname(real)] || "application/octet-stream","Cross-Origin-Opener-Policy":"same-origin","Cross-Origin-Embedder-Policy":"require-corp","Cross-Origin-Resource-Policy":"same-origin","Accept-Ranges":"bytes"};
   let start=0,end=info.size-1,status=200;const range=request.headers.get("Range");
   if(range){const m=/^bytes=(\d+)-(\d*)$/.exec(range);if(!m)return new Response(null,{status:416,headers:{"Content-Range":`bytes */${info.size}`}});start=Number(m[1]);end=m[2]?Math.min(Number(m[2]),end):end;if(start>end || start>=info.size)return new Response(null,{status:416});status=206;headers["Content-Range"]=`bytes ${start}-${end}/${info.size}`;}
   headers["Content-Length"]=String(end-start+1);
   return new Response(Readable.toWeb(createReadStream(real,{start,end})) as any,{status,headers});
  } catch{return new Response("Not found",{status:404});}
 });
}
