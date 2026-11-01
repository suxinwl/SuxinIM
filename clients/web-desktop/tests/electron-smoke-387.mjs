import {mkdirSync} from "node:fs";
import {resolve as tempResolve} from "node:path";
const taskTemp=tempResolve("../../.cache/temp-web");mkdirSync(taskTemp,{recursive:true});process.env.TEMP=taskTemp;process.env.TMP=taskTemp;
import {resolve} from "node:path";
import {spawn} from "node:child_process";
import {mkdir,writeFile} from "node:fs/promises";
await mkdir("test-results",{recursive:true});
const port=17779,errors=[],logs=[];
const child=spawn(process.env.ELECTRON_TEST_EXECUTABLE || resolve("node_modules/electron/dist/electron.exe"),[...(process.env.ELECTRON_TEST_EXECUTABLE?[]:["."]),`--remote-debugging-port=${port}`],{windowsHide:true,env:{...process.env,OPENIM38_SMOKE_TEST:"1",OPENIM38_TEST_DATA_DIR:resolve("../../.cache/electron-smoke-387")},stdio:["ignore","pipe","pipe"]});
child.stdout.on("data",chunk=>logs.push(chunk.toString()));child.stderr.on("data",chunk=>logs.push(chunk.toString()));
let ws;let id=0;const pending=new Map();
try{
 let endpoint;for(let i=0;i<240;i++){try{const targets=await(await fetch(`http://127.0.0.1:${port}/json/list`)).json();endpoint=targets.find(t=>t.url.startsWith("openim38://app/"));if(endpoint)break;}catch{}await new Promise(r=>setTimeout(r,250));}
 if(!endpoint)throw new Error("Electron application target did not start");
 ws=new WebSocket(endpoint.webSocketDebuggerUrl);await new Promise((res,rej)=>{ws.onopen=res;ws.onerror=rej;});
 ws.onmessage=event=>{const message=JSON.parse(event.data);if(message.id){const entry=pending.get(message.id);if(entry){pending.delete(message.id);message.error?entry.reject(new Error(JSON.stringify(message.error))):entry.resolve(message.result);}}else if(message.method==="Runtime.exceptionThrown")errors.push(message.params.exceptionDetails.text);};
 const command=(method,params={})=>new Promise((resolve,reject)=>{const key=++id;pending.set(key,{resolve,reject});ws.send(JSON.stringify({id:key,method,params}));setTimeout(()=>{if(pending.delete(key))reject(new Error(`CDP timeout ${method}`));},90000).unref();});
 const evaluate=async(expression)=>{const result=await command("Runtime.evaluate",{expression,awaitPromise:true,returnByValue:true});if(result.exceptionDetails)throw new Error(JSON.stringify(result.exceptionDetails));return result.result.value;};
 await command("Runtime.enable");
 let ready=false;for(let i=0;i<240;i++){ready=await evaluate('typeof window.login==="function" && document.querySelectorAll("input").length>0');if(ready)break;await new Promise(r=>setTimeout(r,500));}
 if(!ready)throw new Error("Native application login and WASM did not become ready");
 const summary=await evaluate('({url:location.href,isolated:crossOriginIsolated,platform:window.electronAPI?.getPlatform(),wasmLoginRegistered:typeof window.login==="function"})');
 summary.runtime=await evaluate('window.electronAPI.ipcInvoke("getRuntimeVersions")');
 summary.bridge=await evaluate('(async()=>{await window.electronAPI.ipcInvoke("setUserCachePath","synthetic-smoke");const path=await window.electronAPI.saveFileToDisk({file:new File(["OpenIM38 bridge"],"smoke.txt"),type:"sentFileCache",sync:true});const returned=await window.electronAPI.getFileByPath(path);const response=await fetch(`openim38://app/__local?path=${encodeURIComponent(path)}`);return {path,name:returned?.name,content:await returned?.text(),protocolContent:await response.text(),protocolStatus:response.status};})()');
 summary.cacheAccess=await evaluate('(async()=>{const path=window.electronAPI?await window.electronAPI.ipcInvoke("getRuntimeVersions"):null;const file=await window.electronAPI.saveFileToDisk({file:new File(["abcdefghij"],"range.txt"),type:"fileCache",sync:true});const url=`openim38://app/__local?path=${encodeURIComponent(file)}`;const range=await fetch(url,{headers:{Range:"bytes=2-5"}});return {status:range.status,content:await range.text(),contentRange:range.headers.get("Content-Range")};})()');
 summary.errors=errors;await writeFile("../../tests/delivery/windows-387-native-smoke.json",JSON.stringify(summary,null,2));
 if(summary.runtime.app!=="3.8.7" || errors.length || summary.platform!==3 || !summary.isolated || summary.bridge.content!=="OpenIM38 bridge" || summary.bridge.protocolContent!=="OpenIM38 bridge" || summary.cacheAccess.status!==206 || summary.cacheAccess.content!=="cdef")throw new Error(JSON.stringify(summary));
 console.log(JSON.stringify(summary));
}finally{ws?.close();child.kill();await writeFile("../../tests/delivery/windows-387-native-smoke-process.log",logs.join(""));}
