import test from "node:test";
import assert from "node:assert/strict";
import { build } from "esbuild";
import { EventEmitter } from "node:events";
import { mkdtempSync, existsSync, writeFileSync, readFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";

async function load(entry, plugins = []) {
  const result = await build({
    entryPoints: [entry],
    bundle: true,
    write: false,
    format: "esm",
    platform: "node",
    plugins,
  });
  return import(
    `data:text/javascript;base64,${Buffer.from(result.outputFiles[0].text).toString(
      "base64",
    )}`
  );
}
const { startAttachmentDownload } = await load("src/im/attachment-download.ts");
const { normalizeDownloadRequest, attachmentSavePath } = await load(
  "electron/main/downloadTarget.ts",
);
const syntheticUrl =
  "https://files.example.invalid/video%20one.mp4?X-Amz-Credential=SYNTHETIC%2Fscope&X-Amz-Signature=synthetic&part=a%20b~c";
function ports() {
  const tasks = new Map(),
    sent = [];
  return {
    tasks,
    sent,
    getTask: (key) => tasks.get(key),
    addTask: (key, task) => tasks.set(key, task),
    removeTask: (key) => tasks.delete(key),
    start: async (request) => {
      sent.push(request);
      assert.equal(tasks.get(request.url)?.downloadState, "downloading");
    },
    randomPrefix: () => "synthetic-unique",
  };
}
test("attachment URL signature bytes are untouched and cache metadata travels separately", async () => {
  const p = ports();
  await startAttachmentDownload(
    syntheticUrl,
    { saveType: "video", clientMsgID: "synthetic-video", randomName: true },
    p,
  );
  assert.deepEqual(p.sent, [
    {url:syntheticUrl,saveType:"video",fileName:undefined,fileSize:undefined,taskID:"synthetic-video-video",conversationID:undefined,clientMsgID:"synthetic-video"},
  ]);
  assert.equal(p.tasks.get(syntheticUrl).downloadUrl, syntheticUrl);
  assert.equal(p.tasks.get(syntheticUrl).clientMsgID, "synthetic-video");
});
test("a rejected native start removes the task and a later user retry can start again", async () => {
  const p = ports();
  const start = p.start;
  p.start = async () => {
    throw new Error("synthetic IPC failure");
  };
  await assert.rejects(
    startAttachmentDownload(syntheticUrl, { saveType: "video" }, p),
    /IPC failure/,
  );
  assert.equal(p.tasks.size, 0);
  p.start = start;
  await startAttachmentDownload(syntheticUrl, { saveType: "video" }, p);
  assert.equal(p.sent.length, 1);
});
test("duplicate active clicks do not start another transfer", async () => {
  const p = ports();
  await Promise.all([
    startAttachmentDownload(syntheticUrl, { saveType: "video" }, p),
    startAttachmentDownload(syntheticUrl, { saveType: "video" }, p),
  ]);
  assert.equal(p.sent.length, 1);
});

test('work-circle retry retains task identity across renewed URLs while different media stay independent',async()=>{
  const p=ports(),data={saveType:'image',workMomentID:'synthetic-moment-0'};
  await startAttachmentDownload(syntheticUrl,data,p);p.tasks.clear();
  await startAttachmentDownload(syntheticUrl+'&renewed=1',data,p);p.tasks.clear();
  await startAttachmentDownload(syntheticUrl+'&renewed=2',{...data,workMomentID:'synthetic-moment-1'},p);
  assert.equal(p.sent[0].taskID,'moment_synthetic-moment-0-image');
  assert.equal(p.sent[1].taskID,p.sent[0].taskID);assert.notEqual(p.sent[2].taskID,p.sent[0].taskID);
});
test("non-network attachment URLs and credentials are refused before task creation", async () => {
  const p = ports();
  for (const url of [
    "file:///C:/secret",
    "javascript:alert(1)",
    "https://user:secret@example.invalid/a.mp4",
  ])
    await assert.rejects(startAttachmentDownload(url, {}, p));
  assert.equal(p.tasks.size, 0);
});
test("native normalization preserves signed URLs and refuses cache traversal prefixes", () => {
  assert.deepEqual(normalizeDownloadRequest({ url: syntheticUrl, saveType: "video" }), {
    url: syntheticUrl,
    saveType: "video",
  });
  assert.deepEqual(normalizeDownloadRequest(syntheticUrl + "&save-type=video"), {
    url: syntheticUrl + "&save-type=video",
    saveType: "file",
  });
  assert.throws(() =>
    normalizeDownloadRequest({
      url: syntheticUrl,
      saveType: "video",
      randomPrefix: "../../",
    }),
  );
  assert.throws(() => normalizeDownloadRequest({ url: "file:///C:/secret" }));
});
test("remote filenames cannot choose directories outside the managed cache", () => {
  const directory = path.resolve("synthetic-cache");
  for (const filename of [
    "../../escape.mp4",
    String.raw`C:\outside\escape.mp4`,
    "../",
    "",
  ]) {
    const output = attachmentSavePath(directory, filename);
    assert.equal(path.dirname(output), directory);
  }
  assert.throws(() => attachmentSavePath("", "video.mp4"));
});


test('native manager sends current proof and atomic encrypted original names; a failed stream resumes with Range',async t=>{
  const {mkdtemp,rm,readFile}=await import('node:fs/promises'),directory=await mkdtemp(path.join(tmpdir(),'puty-download-manager-'));
  const {LocalVault}=await load('electron/main/localStore/vault.ts'),store=await LocalVault.open(directory,'test_download',Buffer.alloc(32,21),async()=>{}),handlers=new Map(),events=new EventEmitter(),requests=[],lifecycle=new EventEmitter();
  const size=1024*1024+89,payload=Buffer.alloc(size,53),url='https://im.szputy.com:9885/enterprise/v1/attachments/read/synthetic?opaque=keep%20these~bytes',proof='a'.repeat(64);let fail=true;
  const contents={isDestroyed:()=>false,send:(...args)=>events.emit(args[0],args)};
  globalThis.__attachmentIPC={handle:(name,fn)=>handlers.set(name,fn),removeHandler:name=>handlers.delete(name)};
  globalThis.__attachmentRuntime={activeVault:async()=>store,requireSavePermission:async()=>({saveAllowed:true}),authenticatedResourceHeaders:async resourceProof=>({token:'synthetic-current-token','X-Puty-Resource-Proof':resourceProof}),storeLifecycle:lifecycle};
  globalThis.__attachmentNet={fetch:async(request,options)=>{
    requests.push({request,options});assert.equal(request,url);assert.equal(options.headers['X-Puty-Resource-Proof'],proof);assert.equal(options.redirect,'error');
    const headers={'Content-Length':String(size),'ETag':'"same-object"','Content-Disposition':"attachment; filename*=UTF-8''%E8%81%8C%E5%91%98%E5%90%8D%E5%8D%95.xlsx"};
    if(options.method==='HEAD')return new Response(null,{headers});
    if(fail){fail=false;let n=0;return new Response(new ReadableStream({pull(controller){if(!n++){controller.enqueue(payload.subarray(0,1024*1024));return;}controller.error(new Error('synthetic network interruption'));}}),{headers});}
    const offset=Number(options.headers.Range?.match(/bytes=(\d+)-/)?.[1] || 0);if(offset){assert.equal(offset,1024*1024);assert.equal(options.headers['If-Range'],'"same-object"');headers['Content-Range']=`bytes ${offset}-${size-1}/${size}`;headers['Content-Length']=String(size-offset);}
    return new Response(payload.subarray(offset),{status:offset?206:200,headers});
  }};
  const {initDownloadManage}=await load('electron/main/downloadManage.ts',[{name:'controlled-download-host',setup(builder){builder.onResolve({filter:/^electron$/},()=>({path:'electron',namespace:'mock'}));builder.onResolve({filter:/localStore\/runtime$/},()=>({path:'runtime',namespace:'mock'}));builder.onLoad({filter:/.*/,namespace:'mock'},args=>({contents:args.path==='electron'?'export const ipcMain=globalThis.__attachmentIPC;export const net=globalThis.__attachmentNet;':'export const {activeVault,requireSavePermission,authenticatedResourceHeaders,storeLifecycle}=globalThis.__attachmentRuntime;'}));}}]);
  const clean=initDownloadManage(contents),invoke=(name,arg,sender=contents)=>handlers.get(name)({sender},arg),wait=name=>new Promise((resolve,reject)=>{const timeout=setTimeout(()=>reject(new Error('Timed out '+name)),3000);events.once(name,args=>{clearTimeout(timeout);resolve(args);});});
  try{
    await assert.rejects(invoke('startDownload',{url,resourceProof:proof},{}),/window/);await assert.rejects(invoke('startDownload',{url,resourceProof:''}),/授权/);
    const failed=wait('downloadFailed');await invoke('startDownload',{url,resourceProof:proof,taskID:'same-task',clientMsgID:'m1',conversationID:'si_test',fileSize:size});await failed;
    const completed=wait('downloadSuccess');await invoke('startDownload',{url,resourceProof:proof,taskID:'same-task',clientMsgID:'m1',conversationID:'si_test',fileSize:size});const success=await completed;
    assert.equal(path.basename(success[2]),'职员名单.xlsx');const disk=await readFile(success[2]);assert.equal(disk.subarray(0,8).toString(),'PUTYLS1\n');assert.ok(!disk.includes(Buffer.alloc(32,53)));assert.deepEqual(await store.readAttachment('same-task'),payload);assert.ok(requests.some(item=>item.options.headers.Range==='bytes=1048576-'));
    const second=wait('downloadSuccess');await invoke('startDownload',{url,resourceProof:proof,taskID:'different-content',clientMsgID:'m2',conversationID:'si_test',fileSize:size,fileName:'职员名单.xlsx'});assert.equal(path.basename((await second)[2]),'职员名单（1）.xlsx');
  }finally{clean();store.lock();delete globalThis.__attachmentIPC;delete globalThis.__attachmentNet;delete globalThis.__attachmentRuntime;await rm(directory,{recursive:true,force:true});}
});
