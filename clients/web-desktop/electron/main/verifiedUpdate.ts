import { app, net, shell } from "electron";
import { createHash } from "node:crypto";
import { createWriteStream, promises as fs } from "node:fs";
import { join } from "node:path";
import { once } from "node:events";
import { finished } from "node:stream/promises";
import { compareVersions } from "../../src/im/update-policy";
import buildVersion from "../../build-version.json";
type Metadata={url:string;size:number;sha256:string;version:string;build:number;metadataOrigin:string;platform?:string;applicationID?:string};
export async function downloadAndInstallUpdate(meta:Metadata) {
 if(process.platform!=="win32")throw new Error("Only Windows installer updates are enabled");
 const origin=new URL(process.env.OPENIM38_UPDATE_ORIGIN || "https://192.168.10.11:29880");
 const authoritative=await net.fetch(new URL("/enterprise/v1/application/latest",origin).href,{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({platform:"windows",version:app.getVersion(),build:buildVersion.build}),redirect:"error",signal:AbortSignal.timeout(25000)});
 if(!authoritative.ok)throw new Error("Release metadata is unavailable");
 const body=await authoritative.json() as any;const published=body.data;
 if(body.errCode!==0 || !published || published.available===false || ["url","sha256","size","version","build"].some(key=>published[key]!==meta[key]) || meta.build<=buildVersion.build || compareVersions(meta.version,app.getVersion())<0)throw new Error("Release metadata changed or package is not an upgrade");
 const url=new URL(meta.url);
 if(url.protocol!=="https:" || origin.protocol!=="https:" || url.origin!==origin.origin || !/^[a-f0-9]{64}$/i.test(meta.sha256) || !Number.isSafeInteger(meta.size) || meta.size<1024 || meta.size>2*1024**3 || !Number.isSafeInteger(meta.build))throw new Error("Invalid update metadata");
 if(meta.platform && meta.platform!=="windows")throw new Error("Package platform mismatch");
 if(meta.applicationID && meta.applicationID!=="io.openim.enterprise.upgrade38")throw new Error("Application identity mismatch");
 const cache=join(app.getPath("userData"),"verified-updates");await fs.mkdir(cache,{recursive:true});
 const target=join(cache,`${meta.sha256}.exe`), temporary=target+".partial";
 const response=await net.fetch(url.href,{redirect:"error",signal:AbortSignal.timeout(30*60*1000)});if(!response.ok || !response.body)throw new Error(`Download HTTP ${response.status}`);
 const writer=createWriteStream(temporary,{flags:"w"});const writeFinished=finished(writer);void writeFinished.catch(()=>{});const hash=createHash("sha256");let size=0;let header=Buffer.alloc(0);
 try {
  for await(const chunk of response.body as any){const data=Buffer.from(chunk);if(header.length<2)header=Buffer.concat([header,data]).subarray(0,2);size+=data.length;if(size>meta.size)throw new Error("Package size exceeds metadata");hash.update(data);if(!writer.write(data))await once(writer,"drain");}
  writer.end();await writeFinished;
  if(size!==meta.size || hash.digest("hex").toLowerCase()!==meta.sha256.toLowerCase() || header.toString("ascii")!=="MZ")throw new Error("Installer integrity verification failed");
  await fs.rename(temporary,target);
  const failure=await shell.openPath(target);if(failure)throw new Error(failure);
  setTimeout(()=>app.quit(),1000);return true;
 }catch(error){writer.destroy();await writeFinished.catch(()=>{});await fs.unlink(temporary).catch(()=>{});throw error;}
}
