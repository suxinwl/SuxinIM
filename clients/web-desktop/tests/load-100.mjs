import {mkdirSync} from "node:fs";
import {resolve as tempResolve} from "node:path";
const taskTemp=process.env.LOAD_TMP_DIR || tempResolve("../../.cache/temp-web");mkdirSync(taskTemp,{recursive:true});process.env.TEMP=taskTemp;process.env.TMP=taskTemp;
import {chromium} from "playwright";
import {readFile,writeFile,mkdir} from "node:fs/promises";
import {freemem,totalmem} from "node:os";
const accountFile=process.argv[2];if(!accountFile)throw new Error("Pass a private synthetic-account JSON file");
const input=JSON.parse(await readFile(accountFile,"utf8")),allAccounts=(input.users || input.accounts || input).slice(0,100),offsetAccount=Number(process.env.LOAD_OFFSET || 0),count=Number(process.env.LOAD_COUNT || 100),accounts=allAccounts.slice(offsetAccount,offsetAccount+count);
if(allAccounts.length!==100 || accounts.length!==count)throw new Error("100 synthetic accounts required; LOAD_OFFSET/COUNT must select a valid subset");
const origin=process.env.LOAD_API_ORIGIN || "https://192.168.10.11:29880",pageURL=process.env.LOAD_PAGE_URL || "http://127.0.0.1:7778/load.html";
const duration=Number(process.env.LOAD_SECONDS || 1800),run=new Date().toISOString().replaceAll(":","-");
const sharedRun=process.env.LOAD_RUN_ID || run;
const output=`test-results/load-${run}`;await mkdir(output,{recursive:true});
const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE || undefined});
const sharedContext=await browser.newContext();
const host=await sharedContext.newPage();await host.goto(pageURL.replace("load.html","load-host.html"));await host.waitForFunction(()=>Boolean(window.__loadCompiledWasm),undefined,{timeout:90000});
const clients=[],checkpoints=[],pageErrors=[];
host.on("pageerror",error=>pageErrors.push({error:error.message}));host.on("console",msg=>{if(msg.type()==="error" && /initialize WASM|Go program has already exited|out of memory/.test(msg.text()))pageErrors.push({error:msg.text().slice(0,500)});});let started=0,finished=false;
const save=async(final=false)=>{
 const samples=await Promise.all(clients.map(async({page,account})=>({userID:account.userID,...await page.evaluate(()=>window.loadStats()).catch(()=>({connected:false,errors:["Page closed"]}))})));
 const latencies=samples.flatMap(s=>s.latencies || []).sort((a,b)=>a-b),percentile=p=>latencies.length?latencies[Math.min(latencies.length-1,Math.floor(latencies.length*p))]:null;
 const report={run,sharedRun,startedAt:started?new Date(started).toISOString():null,finishedAt:final?new Date().toISOString():null,isolation:`${count} independent same-origin iframe SDK instances and userID-named SQLite databases; one shared compiled WebAssembly.Module`,phase:final?"complete":started?"running":"preparing",offset:offsetAccount,count,requiredSeconds:duration,elapsedSeconds:started?(Date.now()-started)/1000:0,clients:samples.length,connected:samples.filter(s=>s.connected).length,sent:samples.reduce((n,s)=>n+(s.sent || 0),0),received:samples.reduce((n,s)=>n+(s.received || 0),0),failed:samples.reduce((n,s)=>n+(s.failed || 0),0),disconnections:samples.reduce((n,s)=>n+(s.disconnections || 0),0),latencyMs:{p50:percentile(.5),p95:percentile(.95),p99:percentile(.99),max:latencies.at(-1) || null},memory:{free:freemem(),total:totalmem()},checkpoints,pageErrors};
 await writeFile(`${output}/${final?"summary":"progress"}.json`,JSON.stringify(report,null,2));
 if(final)console.log(JSON.stringify(report));return report;
};
try {
 const initialFree=freemem();
 for(let offset=0;offset<count;offset+=5){
  if(freemem()<3.7*1024**3)throw new Error("Less than 3.7 GiB before next batch; reserve 3 GiB workstation memory");
  for (const [index,account] of accounts.slice(offset,offset+5).entries()){
   const context=sharedContext,name=`sdk-${offset+index}`;
   await host.evaluate(({name,url})=>{const frame=document.createElement("iframe");frame.name=name;frame.src=url;frame.style.cssText="width:1px;height:1px;border:0";document.body.appendChild(frame);},{name,url:pageURL});
   let page;for(let attempt=0;attempt<120;attempt++){page=host.frame({name});if(page)break;await new Promise(r=>setTimeout(r,250));}
   if(!page)throw new Error("SDK frame was not created");
   clients.push({context,page,account});
   await page.waitForFunction(()=>window.loadSDKReady && typeof window.commonEventFunc === "function" && typeof window.login === "function",undefined,{timeout:120000});
   await page.evaluate(({account,origin,runID})=>window.loadLogin(account,origin,runID),{account,origin,runID:sharedRun});await page.waitForFunction(()=>window.loadStats().connected,undefined,{timeout:90000});
  }
  checkpoints.push({clients:clients.length,freeMemory:freemem(),secondsSinceEpoch:Date.now()/1000,estimatedUsed:initialFree-freemem()});await save();
  console.log(JSON.stringify({stage:"connected",clients:clients.length,freeMemory:freemem()}));
  if(freemem()<3*1024**3)throw new Error("Less than 3 GiB free memory; stop before exhausting workstation");
 }
 let startAt=Number(process.env.LOAD_START_AT || 0);console.log(JSON.stringify({stage:"prepared",clients:clients.length,startAt,now:Date.now()}));
 if(process.env.LOAD_START_FILE){let nextStatus=0;while(!startAt){try{startAt=Number(JSON.parse(await readFile(process.env.LOAD_START_FILE,"utf8")).startAt || 0);}catch{}if(Date.now()>nextStatus){await save();console.log(JSON.stringify({stage:"waiting-barrier",clients:clients.length,now:Date.now()}));nextStatus=Date.now()+15000;}if(!startAt)await new Promise(r=>setTimeout(r,500));}}
 if(!startAt)startAt=Date.now();
 if(Date.now()>startAt+30000)throw new Error("Prepared after common start deadline");
 while(Date.now()<startAt)await new Promise(r=>setTimeout(r,500));
 started=Date.now();let sequence=0,nextSend=0,nextSave=0;
 while(Date.now()-started<duration*1000){
  if(Date.now()>=nextSend){for(let offset=0;offset<count;offset+=10)await Promise.all(clients.slice(offset,offset+10).map(({page},index)=>page.evaluate(({recvID,sequence})=>window.loadSend(recvID,sequence),{recvID:allAccounts[(offsetAccount+offset+index+1)%100].userID,sequence:sequence++})));nextSend=Date.now()+30000;}
  if(Date.now()>=nextSave){const report=await save();console.log(JSON.stringify({stage:"holding",elapsed:Math.floor(report.elapsedSeconds),connected:report.connected,sent:report.sent,received:report.received,failed:report.failed}));nextSave=Date.now()+15000;}
  await new Promise(resolve=>setTimeout(resolve,1000));
 }
 await new Promise(resolve=>setTimeout(resolve,5000));const result=await save(true);finished=true;
 if(result.connected!==count || result.failed || result.pageErrors.length)throw new Error("SDK load acceptance failed; inspect summary metrics");
}finally{if(!finished)await save().catch(()=>{});await Promise.all(clients.map(({page})=>page.evaluate(()=>window.loadLogout()).catch(()=>{})));await browser.close();}
