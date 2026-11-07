// Real Chromium verification of both exact patched packages using fixtures only.
// No login credentials, installed services, SSH, SQL, or production API is used.
import assert from "node:assert/strict";
import { readFile, writeFile, mkdir, mkdtemp, rm } from "node:fs/promises";
import { createServer } from "node:http";
import { createHash } from "node:crypto";
import { inflateRawSync } from "node:zlib";
import { spawn } from "node:child_process";
import { resolve, join, relative, isAbsolute } from "node:path";
import { tmpdir } from "node:os";

const root = resolve(import.meta.dirname, ".."), output = join(root, "runtime/browser-tests");
const sha = data => createHash("sha256").update(data).digest("hex"), pause = ms => new Promise(resolve => setTimeout(resolve, ms));
const run = "attendance-rule-cleanup-" + Date.now(), reportPath = join(output, run + "-report.json");
const report = { run, fixture_only: true, database_access: false, business_writes: false, packages: [], started_at: new Date().toISOString() };
await mkdir(output, { recursive: true });

function unpack(buffer) {
  let end = buffer.length - 22;
  while (end >= Math.max(0, buffer.length - 65557) && buffer.readUInt32LE(end) !== 0x06054b50) end--;
  assert.ok(end >= 0); const count = buffer.readUInt16LE(end + 10), files = new Map(); let at = buffer.readUInt32LE(end + 16);
  for (let i = 0; i < count; i++) {
    assert.equal(buffer.readUInt32LE(at), 0x02014b50);
    const method = buffer.readUInt16LE(at + 10), size = buffer.readUInt32LE(at + 20), plain = buffer.readUInt32LE(at + 24), n = buffer.readUInt16LE(at + 28), extra = buffer.readUInt16LE(at + 30), comment = buffer.readUInt16LE(at + 32), offset = buffer.readUInt32LE(at + 42), name = buffer.subarray(at + 46, at + 46 + n).toString();
    assert.equal(buffer.readUInt32LE(offset), 0x04034b50);
    const start = offset + 30 + buffer.readUInt16LE(offset + 26) + buffer.readUInt16LE(offset + 28), raw = buffer.subarray(start, start + size);
    assert.ok(method === 0 || method === 8); const bytes = method === 0 ? raw : inflateRawSync(raw); assert.equal(bytes.length, plain);
    assert.ok(!files.has(name)); files.set(name, bytes); at += 46 + n + extra + comment;
  }
  return files;
}

class Page {
  constructor(socket, origin) {
    this.socket = socket; this.origin = origin; this.id = 0; this.pending = new Map(); this.errors = []; this.external = [];
    socket.addEventListener("message", ({data}) => {
      const event = JSON.parse(data);
      if (event.id) { const item = this.pending.get(event.id); if (item) { clearTimeout(item.timer); this.pending.delete(event.id); event.error ? item.reject(Error(event.error.message)) : item.resolve(event.result); } }
      if (event.method === "Runtime.exceptionThrown") this.errors.push(event.params.exceptionDetails.exception?.description || event.params.exceptionDetails.text);
      if (event.method === "Fetch.requestPaused") {
        const {request, requestId} = event.params, denied = new URL(request.url).origin !== origin;
        if (denied) this.external.push(request.url);
        this.send(denied ? "Fetch.failRequest" : "Fetch.continueRequest", denied ? {requestId,errorReason:"BlockedByClient"} : {requestId}).catch(error=>this.errors.push(error.message));
      }
    });
  }
  send(method, params={}) { return new Promise((resolve,reject)=>{const id=++this.id,timer=setTimeout(()=>{this.pending.delete(id);reject(Error("CDP timeout: "+method));},15000);this.pending.set(id,{resolve,reject,timer});this.socket.send(JSON.stringify({id,method,params}));}); }
  async eval(expression) { const r=await this.send("Runtime.evaluate",{expression,awaitPromise:true,returnByValue:true});if(r.exceptionDetails)throw Error(r.exceptionDetails.exception?.description||r.exceptionDetails.text);return r.result.value; }
  async until(expression) { const until=Date.now()+15000;while(Date.now()<until){if(await this.eval(expression))return;await pause(60);}throw Error("Browser condition timed out: "+expression); }
  async reload() { await this.send("Page.navigate",{url:this.origin+"/plugins/biz-suite/admin/hrm/attendance-rules"});await this.until("!!document.querySelector('[data-attendance-current-rules]')"); }
}

function fixture(id, status, source) {
  return {id,name:id,family_id:id,version:1,status,source,conflicts:[],effective_from:"2026-10-01",effective_to:"",
    scope:{employee_ids:["fixture-e1"],department_ids:[],include_children:true,is_default:false},
    config:{category:"职员",cycle:{mode:"weekly",weekdays:Array.from({length:7},(_,i)=>({shift_id:"day",category:i<5?"workday":"restday",scheduled_work:i<5}))},
      shifts:[{id:"day",name:"日班",segments:[{id:"daytime",name:"日间正班",kind:"work",start_minute:480,end_minute:1020,punch_mode:"both"}]}],
      rule:{evidence_mode:"local",window:{},overtime:{},full_attendance:{},leave_policies:{}}}};
}

async function testPackage(target) {
  const build = JSON.parse(await readFile(join(root, `runtime/attendance-rule-cleanup-${target}-build.json`), "utf8")), bytes = await readFile(build.output);
  assert.equal(sha(bytes), build.output_zip_sha256); const files = unpack(bytes), manifest = JSON.parse(files.get("plugin.json"));
  assert.equal(sha(files.get("plugin.json")), build.after_manifest_sha256);
  for (const [name, expected] of Object.entries(manifest.files)) assert.equal(sha(files.get(name)), expected);
  for (const [name, expected] of Object.entries(build.before_backend_sha256)) assert.equal(sha(files.get(name)), expected);
  const bundle = files.get(build.new_asset).toString(); assert.ok(bundle.includes(build.bundle_anchor+build.inserted_fragment));
  const result = {target,package:build.output,manifest:build.after_manifest_sha256,asset:build.new_asset,checks:[]}; report.packages.push(result);
  const rules = [
    fixture("hidden-draft","draft",{cleanup:{hidden:true}}), fixture("hidden-published","published",JSON.stringify({cleanup:{hidden:true}})),
    fixture("hidden-archived","archived",{cleanup:{hidden:true}}), fixture("hidden-reference","draft",{snapshot_id:"old",cleanup:{hidden:true}}),
    fixture("hidden-superseded","superseded",JSON.stringify({cleanup:{hidden:true}})),
    fixture("visible-draft","draft",{}), fixture("visible-history","archived",{cleanup:{hidden:false}}),
    fixture("visible-string","draft",{cleanup:{hidden:"true"}}), fixture("visible-null","published",null),
    fixture("visible-invalid-json","draft","{invalid"), fixture("visible-reference","draft",{snapshot_id:"reference"}),
  ];
  let currentRules=rules, permissions=["*"]; const requests=[], unexpected=[];
  const options={employees:[{id:"fixture-e1",name:"测试人员",employee_no:"0001",department:"测试部门"}],departments:[{id:1,name:"测试部门",pid:0}]};
  const server=createServer(async(req,res)=>{
    try {
      const path=new URL(req.url,"http://127.0.0.1").pathname;
      if(path.startsWith("/plugins/biz-suite/api/admin")) {
        const route=path.slice("/plugins/biz-suite/api/admin".length);requests.push({method:req.method,path:route});
        const send=data=>{res.setHeader("Content-Type","application/json; charset=utf-8");res.end(JSON.stringify({code:0,data}));};
        if(req.method==="GET"&&route==="/access")return send({uid:1,permissions});
        if(req.method==="GET"&&route==="/im/openim/status")return send({enabled:true,url:"https://im.invalid.example"});
        if(req.method==="GET"&&route==="/hrm/options")return send(options);
        if(req.method==="GET"&&route==="/hrm/attendance/v2/rules")return send({items:currentRules});
        unexpected.push({method:req.method,path:route});res.statusCode=400;return send({message:"Unexpected API"});
      }
      const prefix="/plugins/biz-suite/admin/";
      if(req.method==="GET"&&path.startsWith(prefix)) {
        const name=path.includes("/assets/")?"admin/"+path.slice(prefix.length):"admin/index.html", data=files.get(name);
        if(!data){res.statusCode=404;return res.end();}
        res.setHeader("Content-Type",name.endsWith(".js")?"text/javascript; charset=utf-8":name.endsWith(".css")?"text/css; charset=utf-8":"text/html; charset=utf-8");res.setHeader("Cache-Control","no-store");return res.end(data);
      }
      res.statusCode=404;res.end();
    } catch(error){unexpected.push({error:error.message});res.statusCode=500;res.end("Fixture failure");}
  });
  await new Promise(resolve=>server.listen(0,"127.0.0.1",resolve));const origin="http://127.0.0.1:"+server.address().port;
  let chrome,profile,page;
  const check=(name, fn)=>Promise.resolve().then(fn).then(()=>{result.checks.push({name,passed:true});console.log("PASS "+target+" "+name);});
  try {
    profile=await mkdtemp(join(tmpdir(),"attendance-rule-cleanup-"));
    chrome=spawn(process.env.CHROME_PATH||"C:/Program Files/Google/Chrome/Application/chrome.exe",["--headless=new","--disable-gpu","--no-first-run","--no-default-browser-check","--remote-debugging-port=0","--remote-allow-origins=*","--no-proxy-server","--user-data-dir="+profile,"about:blank"],{stdio:"ignore",windowsHide:true});
    let port;for(let i=0;i<100&&!port;i++){try{port=(await readFile(join(profile,"DevToolsActivePort"),"utf8")).split("\n")[0];}catch{await pause(100);}}
    assert.ok(port);const tabs=await(await fetch(`http://127.0.0.1:${port}/json/list`)).json(),socket=new WebSocket(tabs.find(t=>t.type==="page").webSocketDebuggerUrl);
    await new Promise((resolve,reject)=>{socket.addEventListener("open",resolve,{once:true});socket.addEventListener("error",reject,{once:true});});
    page=new Page(socket,origin);await page.send("Runtime.enable");await page.send("Page.enable");await page.send("Fetch.enable",{patterns:[{urlPattern:"*",requestStage:"Request"}]});
    await page.send("Emulation.setDeviceMetricsOverride",{width:1440,height:1000,deviceScaleFactor:1,mobile:false});await page.reload();
    await check("exact-patched-asset-and-unchanged-worker",async()=>{assert.equal(await page.eval("[...document.scripts].map(s=>s.src).find(s=>s.includes('/assets/app-'))"),origin+"/plugins/biz-suite/"+build.new_asset);assert.deepEqual(build.before_backend_sha256,build.after_backend_sha256);});
    await check("both-rule-lists-exclude-explicit-markers-only",async()=>{
      const current=await page.eval("document.querySelector('[data-attendance-current-rules]').textContent"),history=await page.eval("document.querySelector('[data-attendance-rule-history]').textContent");
      assert.doesNotMatch(current+history,/hidden-(draft|published|archived|reference|superseded)/);
      for(const name of ["visible-draft","visible-string","visible-null","visible-invalid-json"])assert.ok(current.includes(name));
      assert.ok(history.includes("visible-history"));assert.equal(history.includes("visible-reference"),target==="public","Keep each base version's existing snapshot policy");
      assert.equal(current.includes("visible-reference"),target==="local");
      assert.equal(await page.eval("document.querySelector('[data-attendance-rule-history]').open"),false);
    });
    await check("search-cannot-reintroduce-cleaned-up-records",async()=>{
      await page.eval("document.querySelector('[data-attendance-rule-history]').open=true;for(const search of document.querySelectorAll('#view input[type=search]')){search.value='hidden-';search.dispatchEvent(new Event('input'));}");
      assert.equal(await page.eval("document.querySelectorAll('#view tbody tr').length"),0);await page.reload();
    });
    await check("batch-window-picker-excludes-hidden-current-and-history",async()=>{
      await page.eval("[...document.querySelectorAll('#view button')].find(b=>b.textContent==='批量设置取卡窗口').click()");await page.until("!!document.querySelector('[data-batch-windows]')");
      const text=await page.eval("document.querySelector('[data-batch-windows]').textContent");assert.doesNotMatch(text,/hidden-(draft|published|archived|reference|superseded)|visible-history/);assert.ok(text.includes("visible-draft"));
      await page.eval("document.querySelector('dialog [aria-label=关闭]').click()");
    });
    await check("read-only-role-keeps-filtering-and-historical-access",async()=>{
      permissions=["biz-suite.hrm.read"];await page.reload();
      assert.doesNotMatch(await page.eval("document.querySelector('#view').textContent"),/hidden-(draft|published|archived|reference|superseded)/);
      assert.equal(await page.eval("[...document.querySelectorAll('#view button')].some(b=>['新增制度','批量设置取卡窗口','发布','编辑 / 新版本'].includes(b.textContent))"),false);
    });
    await check("all-hidden-response-renders-empty-without-a-history-count",async()=>{
      permissions=["*"];currentRules=rules.filter(r=>r.id.startsWith("hidden-"));await page.reload();
      assert.equal(await page.eval("!!document.querySelector('[data-attendance-rule-history]')"),false);assert.equal(await page.eval("document.querySelectorAll('[data-attendance-current-rules] tbody tr').length"),0);
    });
    await check("clearing-marker-restores-history-without-a-new-record",async()=>{
      currentRules=rules;rules.find(r=>r.id==="hidden-archived").source.cleanup.hidden=false;await page.reload();
      assert.ok((await page.eval("document.querySelector('[data-attendance-rule-history]').textContent")).includes("hidden-archived"));assert.equal(rules.length,11);
      const {data}=await page.send("Page.captureScreenshot",{format:"png"});const screenshot=`${run}-${target}.png`;await writeFile(join(output,screenshot),Buffer.from(data,"base64"));result.screenshot=screenshot;
    });
    await check("no-business-writes-external-requests-or-js-errors",()=>{assert.deepEqual(unexpected,[]);assert.deepEqual(page.errors,[]);assert.deepEqual(page.external,[]);assert.ok(requests.every(r=>r.method==="GET"));});
    result.passed=true;result.requests=requests;
  } catch(error){result.passed=false;result.error=error.message;result.js_errors=page?.errors;result.unexpected=unexpected;throw error;}
  finally {
    page?.socket.close();chrome?.kill();await new Promise(resolve=>server.close(resolve));
    if(profile){const child=relative(resolve(tmpdir()),resolve(profile));assert.ok(child&&!child.startsWith("..")&&!isAbsolute(child)&&child.startsWith("attendance-rule-cleanup-"));await pause(300);await rm(profile,{recursive:true,force:true,maxRetries:5,retryDelay:300}).catch(()=>{});}
    await writeFile(reportPath,JSON.stringify(report,null,2));
  }
}

try { for(const target of ["local","public"]) await testPackage(target);report.passed=true; }
catch(error){report.passed=false;report.error=error.message;process.exitCode=1;console.error(error);}
finally {report.finished_at=new Date().toISOString();await writeFile(reportPath,JSON.stringify(report,null,2));console.log("Report: "+reportPath);}
