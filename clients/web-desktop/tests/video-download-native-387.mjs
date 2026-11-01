import { build } from "esbuild";
import { readFile, writeFile, mkdir } from "node:fs/promises";
import { resolve } from "node:path";
import { spawn } from "node:child_process";
const root = resolve("../.."),
  directory = resolve(root, ".cache/video-download-387");
await mkdir(directory, { recursive: true });
const original = resolve(
  root,
  "tests/delivery/windows-387-before/video-download/clients/web-desktop",
);
const output = resolve(root, "tests/delivery/windows-387-video-download");
await mkdir(output, { recursive: true });
const source = resolve("electron/main");
for (const variant of ["before", "fixed"]) {
  const isBefore = variant === "before";
  const download = await readFile(
    isBefore
      ? resolve(original, "electron/main/downloadManage.ts")
      : resolve(source, "downloadManage.ts"),
    "utf8",
  );
  await build({
    stdin: {
      contents: download,
      resolveDir: source,
      sourcefile: "downloadManage.ts",
      loader: "ts",
    },
    bundle: true,
    platform: "node",
    format: "cjs",
    external: ["electron"],
    outfile: resolve(directory, variant + ".cjs"),
    plugins: [
      {
        name: "no-window-side-effects",
        setup(b) {
          b.onResolve({ filter: /^\.\/windowManage$/ }, () => ({
            path: "window",
            namespace: "stub-window",
          }));
          b.onLoad({ filter: /.*/, namespace: "stub-window" }, () => ({
            contents: "export const setProgressBar=()=>{};",
          }));
        },
      },
    ],
  });
  const common = await readFile(
    isBefore
      ? resolve(original, "src/utils/common.ts")
      : resolve("src/utils/common.ts"),
    "utf8",
  );
  const stubs = {
    i18next: "export const t=key=>key;",
    uuid: 'export const v4=()=>"synthetic-prefix";',
    "./pinyin": "export default {};",
    "../AntdGlobalComp": "export const message={error:()=>{}};",
    "@/store":
      "export const useMessageStore={getState:()=>window.__testDownloadStore};",
  };
  await build({
    stdin: {
      contents: common,
      resolveDir: resolve("src/utils"),
      sourcefile: "common.ts",
      loader: "ts",
    },
    bundle: true,
    platform: "browser",
    format: "iife",
    globalName: "DownloadCommon",
    outfile: resolve(directory, variant + "-renderer.js"),
    plugins: [
      {
        name: "test-store-ui-boundaries",
        setup(b) {
          b.onResolve({ filter: /.*/ }, (a) =>
            a.path in stubs ? { path: a.path, namespace: "fixture" } : undefined,
          );
          b.onLoad({ filter: /.*/, namespace: "fixture" }, (a) => ({
            contents: stubs[a.path],
          }));
        },
      },
    ],
  });
}
await build({
  entryPoints: ["electron/main/localProtocol.ts"],
  bundle: true,
  platform: "node",
  format: "cjs",
  external: ["electron"],
  outfile: resolve(directory, "protocol.cjs"),
});
await writeFile(
  resolve(directory, "preload.cjs"),
  `const {contextBridge,ipcRenderer}=require('electron');contextBridge.exposeInMainWorld('electronAPI',{ipcInvoke:(...args)=>ipcRenderer.invoke(...args)});`,
);
await writeFile(
  resolve(directory, "native.cjs"),
  String.raw`
const {app,BrowserWindow}=require('electron');
const fs=require('node:fs'),path=require('node:path'),http=require('node:http'),crypto=require('node:crypto');
const variant=process.argv[2],root=process.argv[3],out=process.argv[4],base=__dirname;
const {registerLocalProtocol}=require('./protocol.cjs');
const {initDownloadManage}=require('./'+variant+'.cjs');
const directory=path.join(base,variant+'-native'),dist=path.join(directory,'dist');fs.mkdirSync(dist,{recursive:true});fs.writeFileSync(path.join(dist,'index.html'),'<body>synthetic download fixture</body>');
app.setPath('userData',path.join(directory,'userData'));
global.pathConfig={distPath:dist,extraResourcesPath:dist};for(const type of ['image','video','avatar','file','sentFile'])global.pathConfig[type+'CachePath']=path.join(directory,type);
const bytes=fs.readFileSync(path.join(root,'tests/flutter/media-rotation/asymmetric-source.mp4'));
const expected='/synthetic.mp4?part=a%20b~c&signature=synthetic-only';
const requests=[],nativeEvents=[];let window,server,clean;
const report={variant,startedUtc:new Date().toISOString(),syntheticOnly:true,tlsBypass:false,errors:[]};
const wait=ms=>new Promise(r=>setTimeout(r,ms));
function digest(value){return crypto.createHash('sha256').update(value).digest('hex');}
(async()=>{try{
 await app.whenReady();
 server=http.createServer((request,response)=>{const exact=request.url===expected;requests.push({exact,status:exact?200:403});response.writeHead(exact?200:403,{'Content-Type':exact?'video/mp4':'text/plain','Content-Disposition':'attachment; filename="synthetic-video.mp4"','Content-Length':exact?bytes.length:16});response.end(exact?bytes:'invalid signature');});await new Promise(r=>server.listen(0,'127.0.0.1',r));
 window=new BrowserWindow({show:false,webPreferences:{preload:path.join(base,'preload.cjs'),contextIsolation:true,sandbox:true}});
 registerLocalProtocol(window.webContents.session);
 const send=window.webContents.send.bind(window.webContents);window.webContents.send=(name,...args)=>{if(name.startsWith('download'))nativeEvents.push({name,url:args[0],path:args[1]});send(name,...args);};
 clean=initDownloadManage(window.webContents);
 await window.loadURL('openim38://app/index.html');
 await window.webContents.executeJavaScript('window.__testDownloadStore={downloadMap:{},addDownloadTask(url,task){this.downloadMap[url]=task},removeDownloadTask(url){delete this.downloadMap[url]}};void 0;');
 await window.webContents.executeJavaScript(fs.readFileSync(path.join(base,variant+'-renderer.js'),'utf8')+';void 0;');
 const url='http://127.0.0.1:'+server.address().port+expected;
 await window.webContents.executeJavaScript('DownloadCommon.downloadFile('+JSON.stringify(url)+',{saveType:"video",clientMsgID:"synthetic-video",showError:true})');
 let terminal;for(let i=0;i<60;i++){terminal=nativeEvents.find(e=>e.name==='downloadSuccess'||e.name==='downloadFailed');if(terminal)break;await wait(100);}
 report.requestCount=requests.length;report.exactSignedRequest=requests.some(r=>r.exact);report.responseStatuses=requests.map(r=>r.status);report.terminal=terminal?.name||'no terminal within 6 seconds';report.eventCorrelated=terminal?.url===url;
 if(terminal?.name==='downloadSuccess'){
  report.savedInVideoCache=path.dirname(terminal.path)===global.pathConfig.videoCachePath;
  report.bytes=fs.statSync(terminal.path).size;report.sha256=digest(fs.readFileSync(terminal.path));report.sourceSha256=digest(bytes);
  const local='openim38://app/__local?path='+encodeURIComponent(terminal.path);
  report.localRange=await window.webContents.executeJavaScript('(async()=>{const r=await fetch('+JSON.stringify(local)+',{headers:{Range:"bytes=0-15"}});return {status:r.status,bytes:(await r.arrayBuffer()).byteLength,contentRange:r.headers.get("Content-Range")};})()');
  report.videoMetadata=await window.webContents.executeJavaScript('(async()=>{const v=document.createElement("video");v.muted=true;v.src='+JSON.stringify(local)+';document.body.append(v);return new Promise(resolve=>{const timer=setTimeout(()=>resolve({timeout:true}),5000);v.onloadeddata=()=>{clearTimeout(timer);resolve({width:v.videoWidth,height:v.videoHeight,duration:v.duration,ready:v.readyState})};v.onerror=()=>{clearTimeout(timer);resolve({error:v.error?.code})};});})()');
 }
 if(variant==='fixed'){
  const previous=nativeEvents.length;
  const tlsUrl='https://192.168.10.11:29880/favicon.ico';
  await window.webContents.executeJavaScript('DownloadCommon.downloadFile('+JSON.stringify(tlsUrl)+',{saveType:"image",showError:true})');
  let tls;for(let i=0;i<100;i++){tls=nativeEvents.slice(previous).find(e=>e.name==='downloadSuccess'||e.name==='downloadFailed');if(tls)break;await wait(100);}
  report.normalTLS={terminal:tls?.name||'no terminal within 10 seconds',correlated:tls?.url===tlsUrl};if(tls?.name==='downloadSuccess'){report.normalTLS.bytes=fs.statSync(tls.path).size;report.normalTLS.sha256=digest(fs.readFileSync(tls.path));report.normalTLS.savedInImageCache=path.dirname(tls.path)===global.pathConfig.imageCachePath;}
 }
 report.pass=variant==='before'?requests.some(r=>r.status===403):report.terminal==='downloadSuccess'&&report.exactSignedRequest&&report.eventCorrelated&&report.savedInVideoCache&&report.sha256===report.sourceSha256&&report.localRange?.status===206&&report.videoMetadata?.width>0&&report.normalTLS?.terminal==='downloadSuccess';
}catch(error){report.errors.push({name:error.name,message:String(error.message).slice(0,300)});report.pass=false;}finally{report.finishedUtc=new Date().toISOString();fs.writeFileSync(path.join(out,variant+'.json'),JSON.stringify(report,null,2));try{clean?.();}catch{}window?.destroy();server?.close();console.log(JSON.stringify({variant,pass:report.pass,terminal:report.terminal,normalTLS:report.normalTLS?.terminal,errors:report.errors}));app.exit(report.pass?0:1);}})();
`,
);
for (const variant of ["before", "fixed"]) {
  const log = [];
  const child = spawn(
    resolve("node_modules/electron/dist/electron.exe"),
    [resolve(directory, "native.cjs"), variant, root, output],
    {
      windowsHide: true,
      stdio: ["ignore", "pipe", "pipe"],
      env: { ...process.env, TEMP: directory, TMP: directory },
    },
  );
  child.stdout.on("data", (chunk) => log.push(chunk.toString()));
  child.stderr.on("data", (chunk) => log.push(chunk.toString()));
  const timer = setTimeout(() => child.kill(), 45000);
  const code = await new Promise((r) => child.once("exit", r));
  clearTimeout(timer);
  await writeFile(resolve(output, variant + ".log"), log.join(""));
  console.log(
    JSON.stringify({
      variant,
      exit: code,
      report: JSON.parse(await readFile(resolve(output, variant + ".json"), "utf8")),
    }),
  );
  if (code !== 0) process.exitCode = 1;
}
