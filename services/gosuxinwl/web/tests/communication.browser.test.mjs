import test from "node:test";
import assert from "node:assert/strict";
import { createServer } from "node:http";
import { readFile, mkdtemp, writeFile, mkdir } from "node:fs/promises";
import { existsSync } from "node:fs";
import { resolve, join } from "node:path";
import { tmpdir } from "node:os";
import { spawn } from "node:child_process";

const root = resolve(import.meta.dirname, "..");
const base = "/plugins/biz-suite/admin/";
const api = "/plugins/biz-suite/api/admin/im/manager/openim";
const delay = ms => new Promise(resolve => setTimeout(resolve, ms));

class CDP {
  constructor(socket) {
    this.socket = socket;
    this.id = 0;
    this.pending = new Map();
    this.errors = [];
    socket.addEventListener("message", event => {
      const result = JSON.parse(event.data);
      if (result.id) {
        const pending = this.pending.get(result.id);
        this.pending.delete(result.id);
        if (result.error) pending.reject(new Error(result.error.message));
        else pending.resolve(result.result);
      }
      if (result.method === "Runtime.exceptionThrown") this.errors.push(result.params.exceptionDetails);
    });
  }
  send(method, params = {}) {
    return new Promise((resolve, reject) => {
      const id = ++this.id;
      this.pending.set(id, { resolve, reject });
      this.socket.send(JSON.stringify({ id, method, params }));
    });
  }
  async eval(expression) {
    const result = await this.send("Runtime.evaluate", { expression, awaitPromise: true, returnByValue: true });
    if (result.exceptionDetails) throw new Error(result.exceptionDetails.exception?.description || result.exceptionDetails.text);
    return result.result.value;
  }
  async wait(expression) {
    for (let i = 0; i < 800; i++) {
      if (await this.eval(expression)) return;
      await delay(25);
    }
    throw new Error("Timed out: " + expression);
  }
}


import { createServer as createViteServer } from 'vite';
import vue from '@vitejs/plugin-vue';

test('native communication: employee preview, config preservation, complete version metadata and discovery changes', {timeout:120000}, async t => {
  const requests=[];
  let grants=['biz-suite.im.manage'];
  let capacityVerified=false;
  let config={webTitle:'原标题',appName:'速信',allowAddFriend:'true',discoverEnabled:'true'};
  let discovery=[{id:'d1',title:'入口一',url:'https://example.com/one',enabled:true,sort:1},{id:'d2',title:'入口二',url:'https://example.com/two',enabled:true,sort:2}];
  const operations=['groups/search','config/get','config/update','versions/search','versions/update','discovery/list','discovery/save','storage/capacity'];
  const fixture={name:'fixture',enforce:'pre',resolveId(id){if(id==='/fixture.ts')return '\0fixture';if(id==='@/utils/http'||id.replaceAll('\\','/').endsWith('/src/utils/http'))return '\0http'},load(id){if(id==='\0fixture')return `import {createApp} from 'vue';import Arco from '@arco-design/web-vue';import '@arco-design/web-vue/dist/arco.css';import App from '/src/views/communication/index.vue';createApp(App).use(Arco).mount('#app');`;if(id==='\0http')return `export const defHttp={get:async({url})=>(await fetch(url)).json(),post:async({url,params})=>(await fetch(url,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(params||{})})).json()};`},configureServer(server){server.middlewares.use(async(req,res,next)=>{
    if(req.url==='/fixture'){res.setHeader('Content-Type','text/html');res.end('<!doctype html><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><div id="app"></div><script type="module" src="/fixture.ts"></script>');return}
    const path=new URL(req.url,'http://localhost').pathname;if(!path.startsWith('/plugins/'))return next();
    let raw='';for await(const chunk of req)raw+=chunk;const input=raw?JSON.parse(raw):{};requests.push({path,input});let data={};
    if(path.endsWith('/api/admin/access'))data={permissions:grants};
    else if(path.endsWith('/capabilities'))data={operations,version:'3.8'};
    else if(path.endsWith('/storage/capacity'))data={dataCapacity:capacityVerified,status:capacityVerified?'critical':'unknown',measuredAt:1700000000,filesystems:[{kind:'mongo',measured:capacityVerified,dataCapacity:capacityVerified,status:capacityVerified?'critical':'unknown',usageRatio:capacityVerified?.9:null,usage_bytes:capacityVerified?900:null,total_bytes:capacityVerified?1000:null,available_bytes:capacityVerified?100:null},{kind:'minio',measured:capacityVerified,dataCapacity:capacityVerified,status:capacityVerified?'warning':'unknown',usageRatio:capacityVerified?.7:null,usage_bytes:capacityVerified?700:null,total_bytes:capacityVerified?1000:null,available_bytes:capacityVerified?300:null},{kind:'service_root',measured:true,dataCapacity:false,status:'ok',usageRatio:.1,usage_bytes:100,total_bytes:1000,available_bytes:900}]};
    else if(path.endsWith('/preview'))data={initialProvisioned:false,items:[{employeeNo:'0001',name:'员工甲',willEnable:true,reason:''},{employeeNo:'',name:'待处理员工',willEnable:false,reason:'缺少明确工号，请补充企业微信通讯录的工号字段'}]};
    else if(path.endsWith('/employees'))data=[{id:'internal-record-id',status:'waiting',error:'',appliedVersion:0,desiredVersion:1,projection:{employeeID:'internal-employee-id',hostUID:42,userID:'internal-im-id',nickname:'待处理员工',account:'',departmentName:'测试部门',enabled:false,reason:'缺少明确工号，请补充企业微信通讯录的工号字段',authFingerprint:'never-render-this',profile:{name:'待处理员工',account:''}}},{id:'employee-two',status:'ready',error:'',appliedVersion:1,desiredVersion:1,projection:{nickname:'陈工程',account:'0012',departmentName:'研发部',enabled:true}},{id:'employee-three',status:'failed',error:'网络异常',appliedVersion:1,desiredVersion:2,projection:{nickname:'李会计',account:'AC-03',departmentName:'财务部',enabled:false}}];
    else if(path.endsWith('/provision'))data={accepted:true};
    else if(path.endsWith('/config/get'))data={config};
    else if(path.endsWith('/config/update')){config={...config,...input};data={}}
    else if(path.endsWith('/versions/search'))data={total:1,versions:[{id:'v1',platform:'windows',version:'1.2.0',url:'https://im.szputy.com:9885/client.exe',text:'原说明',force:true,build:120,size:123456,sha256:'a'.repeat(64)}]};
    else if(path.endsWith('/groups/search'))data={total:22,groups:Array.from({length:22},(_,i)=>({groupID:'group-'+i,groupName:i===0?'唯一结果':'普通群'+(i+1)})).slice(((input.pageNumber||1)-1)*20,(input.pageNumber||1)*20)};
    else if(path.endsWith('/discovery/list'))data={items:discovery};
    else if(path.endsWith('/discovery/save')){discovery=input.items;data={items:discovery}}
    else if(path.endsWith('/openim/ticket'))data={url:'https://evil.invalid/?ticket=untrusted'};
    res.setHeader('Content-Type','application/json');res.end(JSON.stringify({code:0,data}));
  })}};
  const server=await createViteServer({root,configFile:false,plugins:[fixture,vue()],resolve:{alias:{'@':join(root,'src')}},optimizeDeps:{noDiscovery:true,include:['vue','@arco-design/web-vue']},server:{host:'127.0.0.1',port:0},logLevel:'error'});await server.listen();t.after(()=>server.close());
  const origin=`http://127.0.0.1:${server.httpServer.address().port}`;
  const browser=process.env.CHROME_PATH||['C:/Program Files/Google/Chrome/Application/chrome.exe','C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe','/usr/bin/chromium'].find(existsSync);assert.ok(browser);
  const userDir=await mkdtemp(join(tmpdir(),'native-communication-'));
  const child=spawn(browser,['--headless=new','--disable-gpu','--no-first-run','--remote-debugging-port=0',`--user-data-dir=${userDir}`,'about:blank'],{stdio:'ignore',windowsHide:true});t.after(()=>child.kill());
  let debugPort;
  for(let i=0;i<200;i++){try{debugPort=Number((await readFile(join(userDir,'DevToolsActivePort'),'utf8')).split('\n')[0]);break}catch{await delay(50)}}
  const target=await(await fetch(`http://127.0.0.1:${debugPort}/json/new?${encodeURIComponent(origin+'/fixture')}`,{method:'PUT'})).json();
  const socket=new WebSocket(target.webSocketDebuggerUrl);await new Promise(r=>socket.addEventListener('open',r,{once:true}));t.after(()=>socket.close());
  const cdp=new CDP(socket);await cdp.send('Runtime.enable');await cdp.send('Emulation.setDeviceMetricsOverride',{width:1280,height:900,deviceScaleFactor:1,mobile:false});await cdp.wait("document.querySelector('[data-testid=operation-select]')");
  async function clickText(selector,text){await cdp.eval(`(()=>{const e=[...document.querySelectorAll(${JSON.stringify(selector)})].find(e=>e.getClientRects().length&&e.textContent.trim()===${JSON.stringify(text)});if(!e)throw Error('Missing '+${JSON.stringify(text)});e.click()})()`)}
  async function choose(section,title){if(!(await cdp.eval(`[...document.querySelectorAll('.arco-tabs-tab-active')].some(e=>e.textContent.trim()===${JSON.stringify(section)})`)))await clickText('.arco-tabs-tab',section);await cdp.eval("document.querySelector('[data-testid=operation-select]').click()");await cdp.wait(`[...document.querySelectorAll('.arco-select-option')].some(e=>e.textContent.trim()===${JSON.stringify(title)})`);await clickText('.arco-select-option',title);await delay(100)}
  async function field(key,value){await cdp.eval(`(()=>{const e=document.querySelector('[data-field=${key}]').querySelector('input,textarea');Object.getOwnPropertyDescriptor(e.tagName==='TEXTAREA'?HTMLTextAreaElement.prototype:HTMLInputElement.prototype,'value').set.call(e,${JSON.stringify(String(value))});e.dispatchEvent(new Event('input',{bubbles:true}));e.dispatchEvent(new Event('change',{bubbles:true}));e.blur()})()`)}
  async function submit(confirm=false){await cdp.eval("document.querySelector('[data-testid=submit-operation]').click()");if(confirm){await cdp.wait("document.querySelector('.arco-modal-footer .arco-btn-primary')");await cdp.eval("document.querySelector('.arco-modal-footer .arco-btn-primary').click()");await cdp.wait("document.querySelectorAll('.arco-modal').length===0")}await delay(100)}
  async function resultKeyword(value){await cdp.eval(`(()=>{const e=document.querySelector('[data-testid=result-keyword] input')||document.querySelector('input[data-testid=result-keyword]');Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value').set.call(e,${JSON.stringify(value)});e.dispatchEvent(new Event('input',{bubbles:true}))})()`);await delay(50)}
  async function filterSelect(id,label){await cdp.eval(`document.querySelector('[data-testid=${id}]').click()`);await cdp.wait(`[...document.querySelectorAll('.arco-select-option')].some(e=>e.getClientRects().length&&e.textContent.trim()===${JSON.stringify(label)})`);await clickText('.arco-select-option',label);await delay(50)}
  await t.test('capacity stays unknown until verified data mounts and never treats service root as data capacity',async()=>{
    await cdp.wait("document.body.innerText.includes('数据卷挂载尚未核验') && document.body.innerText.includes('不代表数据卷余量')");
    assert.ok((await cdp.eval('document.body.innerText')).includes('不代表数据卷余量'));
    assert.equal(await cdp.eval("document.querySelectorAll('.capacity-grid .arco-progress').length"),0);
    capacityVerified=true;await clickText('button','刷新容量');await cdp.wait("document.body.innerText.includes('数据存储空间严重不足')");
    assert.equal(await cdp.eval("document.querySelectorAll('.capacity-grid .arco-progress').length"),2);
    assert.ok((await cdp.eval('document.body.innerText')).includes('不代表数据卷余量'));
    assert.deepEqual(requests.find(r=>r.path.endsWith('/storage/capacity')).input,{});
  });
  await t.test('employee provisioning requires an actual preview',async()=>{
    await choose('员工同步','确认首次通讯开通');await cdp.eval("document.querySelector('[data-field=confirm] .arco-switch').click()");await submit();assert.equal(requests.filter(r=>r.path.endsWith('/provision')).length,0);
    await choose('员工同步','预览首次通讯开通');await submit();await cdp.wait("document.body.innerText.includes('员工甲')");
    await choose('员工同步','确认首次通讯开通');await cdp.eval("document.querySelector('[data-field=confirm] .arco-switch').click()");await submit(true);assert.equal(requests.filter(r=>r.path.endsWith('/provision')).length,1);
  });
  await t.test('employee columns prioritize identity and pending reasons without internal projection fields',async()=>{
    await choose('员工同步','查询员工同步状态');await submit();await cdp.wait("document.body.innerText.includes('待处理员工')");
    const labels=await cdp.eval("[...document.querySelectorAll('.results th')].map(e=>e.textContent.trim())");assert.deepEqual(labels.slice(0,6),['姓名','工号','部门','启用状态','同步状态','待处理原因']);
    const text=await cdp.eval("document.querySelector('.results').innerText");assert.ok(text.includes('缺少明确工号'));assert.ok(text.includes('未填写'));assert.ok(text.includes('未启用'));assert.ok(text.includes('待开通'));assert.ok(text.includes('测试部门'));
    for(const hidden of ['profile','authFingerprint','never-render-this','internal-record-id','internal-employee-id','internal-im-id','hostUID'])assert.ok(!text.includes(hidden),hidden);
    await choose('员工同步','预览首次通讯开通');await submit();await cdp.wait("document.body.innerText.includes('暂不可开通')");assert.equal(await cdp.eval("document.querySelector('.results').innerText.includes('缺少明确工号')"),true);
  });
  await t.test('employee filters combine visible identity, department, enabled and sync state without server mutations',async()=>{
    await choose('员工同步','查询员工同步状态');await submit();await cdp.wait("document.querySelector('[data-testid=filter-summary]')?.textContent.includes('匹配 3 / 3')");
    assert.equal(await cdp.eval("document.querySelector('.results').innerText.includes('填入操作表单')"),false,'employee sync has no per-record operation');
    const count=requests.length;
    await resultKeyword('ac-03');await filterSelect('filter-department','财务部');await filterSelect('filter-enabled','未启用');await filterSelect('filter-status','同步失败');
    await cdp.wait("document.querySelector('[data-testid=filter-summary]').textContent.includes('匹配 1 / 3')");
    assert.equal(await cdp.eval("document.querySelector('.results table').innerText.includes('李会计')"),true);
    await resultKeyword('0012');await cdp.wait("document.body.innerText.includes('没有符合筛选条件的记录')");
    await clickText('button','清空筛选');await cdp.wait("document.querySelector('[data-testid=filter-summary]').textContent.includes('匹配 3 / 3')");
    await resultKeyword('缺少明确工号');await cdp.wait("document.querySelector('[data-testid=filter-summary]').textContent.includes('匹配 1 / 3')");
    await resultKeyword('internal-record-id');await cdp.wait("document.querySelector('[data-testid=filter-summary]').textContent.includes('匹配 0 / 3')");
    assert.equal(requests.length,count,'filters stay local and never send a write');
    await clickText('button','清空筛选');await mkdir(resolve(root,'../dist/qa'),{recursive:true});await cdp.wait("document.querySelectorAll('.arco-message').length===0");
    for(const width of [1280,390]){await cdp.send('Emulation.setDeviceMetricsOverride',{width,height:900,deviceScaleFactor:1,mobile:width<600});await delay(100);const shot=await cdp.send('Page.captureScreenshot',{format:'png'});await writeFile(resolve(root,`../dist/qa/communication-filters-${width}.png`),Buffer.from(shot.data,'base64'));}
    await cdp.send('Emulation.setDeviceMetricsOverride',{width:1280,height:900,deviceScaleFactor:1,mobile:false});
  });
  await t.test('current-page filtering preserves server pagination and a new query starts at page one',async()=>{
    await choose('群与成员','查询群');await submit();await cdp.wait("document.querySelector('[data-testid=filter-summary]')?.textContent.includes('匹配 20 / 20')");
    await resultKeyword('唯一结果');await cdp.wait("document.querySelector('[data-testid=filter-summary]').textContent.includes('匹配 1 / 20')");
    assert.equal(await cdp.eval("[...document.querySelectorAll('.pager button')].find(e=>e.textContent.trim()==='下一页').disabled"),false);
    await clickText('.pager button','下一页');await cdp.wait("document.querySelector('.pager').innerText.includes('第 2 页') && document.querySelector('[data-testid=filter-summary]').textContent.includes('匹配 0 / 2')");
    await submit();await cdp.wait("document.querySelector('.pager').innerText.includes('第 1 页')");
    const search=requests.filter(r=>r.path.endsWith('/groups/search'));assert.deepEqual(search.slice(-3).map(r=>r.input.pageNumber),[1,2,1]);assert.ok(search.every(r=>!r.input.keyword));
  });
  await t.test('editing title preserves current config boolean values',async()=>{
    await choose('客户端配置','修改客户端配置');await cdp.wait("document.querySelector('[data-field=webTitle] input')?.value==='原标题'");await field('webTitle','新标题');await submit(true);assert.equal(config.webTitle,'新标题',JSON.stringify(requests.filter(r=>r.path.includes('/config/'))));assert.equal(config.allowAddFriend,'true');assert.equal(config.discoverEnabled,'true');assert.deepEqual(requests.find(r=>r.path.endsWith('/config/update')).input,{webTitle:'新标题'});
  });
  await t.test('3.8 version aliases populate the complete package form',async()=>{
    await choose('客户端版本','查询客户端版本');await submit();await cdp.wait("document.body.innerText.includes('1.2.0')");await clickText('button','填入操作表单');await cdp.wait("document.querySelector('[data-testid=record-selection]')?.innerText.includes('提交并确认后才会修改数据')");assert.equal(requests.filter(r=>r.path.endsWith('/versions/update')).length,0);await choose('客户端版本','修改客户端版本');await field('description','新说明');await submit(true);
    const input=requests.find(r=>r.path.endsWith('/versions/update'))?.input;assert.ok(input,await cdp.eval('document.body.innerText'));assert.equal(input.downloadURL,'https://im.szputy.com:9885/client.exe');assert.equal(input.build,120);assert.equal(input.size,123456);assert.equal(input.sha256,'a'.repeat(64));assert.equal(input.description,'新说明');
  });
  await t.test('editing and deleting discovery keeps unrelated entries',async()=>{
    await choose('发现页','查询发现页');await submit();await cdp.wait("document.body.innerText.includes('入口一')");await clickText('button','填入操作表单');await choose('发现页','保存发现页入口');await field('title','更新入口一');await submit(true);assert.equal(discovery.length,2);assert.equal(discovery[0].title,'更新入口一');assert.equal(discovery[1].id,'d2');
    await clickText('button','删除入口');await cdp.wait("document.querySelector('.arco-modal-footer .arco-btn-primary')");await cdp.eval("document.querySelector('.arco-modal-footer .arco-btn-primary').click()");for(let i=0;i<100&&discovery.length!==1;i++)await delay(50);assert.equal(discovery.length,1);assert.equal(discovery[0].id,'d2');
  });
  await cdp.wait("document.querySelectorAll('.arco-message').length===0");await mkdir(resolve(root,'../dist/qa'),{recursive:true});const screenshot=await cdp.send('Page.captureScreenshot',{format:'png'});await writeFile(resolve(root,'../dist/qa/native-communication.png'),Buffer.from(screenshot.data,'base64'));
  await t.test('ordinary staff do not request manager capabilities and reject untrusted ticket redirects',async()=>{
    grants=['biz-suite.im.read','biz-suite.im.write'];const count=requests.filter(r=>r.path.endsWith('/capabilities')).length;await clickText('button','刷新权限');await cdp.wait("document.body.innerText.includes('使用“进入通讯”打开聊天')");assert.equal(await cdp.eval("document.querySelector('[data-testid=operation-select]')===null"),true);assert.equal(requests.filter(r=>r.path.endsWith('/capabilities')).length,count);await clickText('button','进入通讯');await cdp.wait("document.body.innerText.includes('通讯入口地址无效')");assert.equal(new URL(await cdp.eval('location.href')).origin,origin);
  });
  assert.deepEqual(cdp.errors,[]);
});
