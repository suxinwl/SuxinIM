import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile, mkdtemp, writeFile, mkdir } from 'node:fs/promises';
import { existsSync } from 'node:fs';
import { resolve, join } from 'node:path';
import { tmpdir } from 'node:os';
import { spawn } from 'node:child_process';
import { createServer } from 'vite';
import vue from '@vitejs/plugin-vue';

const delay = ms => new Promise(resolve => setTimeout(resolve, ms));
const root = resolve(import.meta.dirname, '..');
class CDP {
  constructor(socket) {
    this.socket = socket; this.id = 0; this.pending = new Map(); this.errors = [];
    socket.addEventListener('message', event => {
      const result = JSON.parse(event.data);
      if (result.id) {
        const item = this.pending.get(result.id); this.pending.delete(result.id);
        if (result.error) item.reject(new Error(result.error.message)); else item.resolve(result.result);
      }
      if (result.method === 'Runtime.exceptionThrown') this.errors.push(result.params.exceptionDetails);
    });
  }
  send(method, params = {}) {
    return new Promise((resolve, reject) => {
      const id = ++this.id; this.pending.set(id, { resolve, reject });
      this.socket.send(JSON.stringify({ id, method, params }));
    });
  }
  async eval(expression) {
    const result = await this.send('Runtime.evaluate', { expression, awaitPromise: true, returnByValue: true });
    if (result.exceptionDetails) throw new Error(result.exceptionDetails.exception?.description || result.exceptionDetails.text);
    return result.result.value;
  }
  async wait(expression) {
    for (let i = 0; i < 600; i++) { if (await this.eval(expression)) return; await delay(25); }
    throw new Error('Timed out: ' + expression);
  }
}

test('department IM controls use scoped leaders and partial group updates', { timeout: 120000 }, async t => {
  const requests = [];
  const departments = [
    { id: 1, pid: 0, name: '研发部', status: 0, weigh: 1, createtime: '2026-11-01', remark: '', im_group_enabled: 0, leader_uid: 7, leader_name: '工程甲' },
    { id: 2, pid: 0, name: '财务部', status: 0, weigh: 2, createtime: '2026-11-01', remark: '', im_group_enabled: 0, leader_uid: 0, leader_name: '' },
  ];
  let rejectGroup = true;
  const modules = {
    '\0fixture': `import {createApp,h} from 'vue';import {createI18n} from 'vue-i18n';import Arco from '@arco-design/web-vue';import '@arco-design/web-vue/dist/arco.css';import App from '/src/views/system/dept/index.vue';const app=createApp(App);app.component('page-card',{setup:(_,ctx)=>()=>h('main',ctx.slots.default?.())});app.use(createI18n({legacy:false,locale:'zh-CN',messages:{'zh-CN':{searchTable:{form:{search:'搜索',reset:'重置'}}}}})).use(Arco).mount('#app');`,
    '\0http': `async function call(method,{url,params={}}){const query=method==='GET'?'?'+new URLSearchParams(params):'';const response=await fetch(url+query,{method,headers:{'Content-Type':'application/json'},body:method==='GET'?undefined:JSON.stringify(params)});const body=await response.json();if(body.code)throw Error('Request rejected');return body.data}export const defHttp={get:args=>call('GET',args),post:args=>call('POST',args),delete:args=>call('DELETE',args)};`,
    '\0modal': `import {ref,h} from 'vue';const visible=ref(false);export const BasicModal={emits:['ok','register'],setup(_,ctx){return()=>visible.value?h('section',{'data-modal':true},[ctx.slots.default?.(),h('button',{'data-submit':true,onClick:()=>ctx.emit('ok')},'保存')]):null}};export function useModal(){return[()=>{},{openModal:(_,data)=>window.__openDeptModal(data)}]}export function useModalInner(callback){window.__openDeptModal=async(data)=>{visible.value=true;await callback(data)};return[()=>{},{setModalProps:()=>{},closeModal:()=>{visible.value=false}}]}`,
    '\0icons': `import {h} from 'vue';export const Icon={props:['icon'],setup:(p,c)=>()=>h('button',{'data-icon':p.icon,onClick:c.attrs.onClick},p.icon)};export const IconPicker=Icon;`,
    '\0tools': `import {h} from 'vue';export const statusOptions=[{label:'开启',value:0},{label:'关闭',value:1}];export const tabletool={emits:['create'],setup:(_,c)=>()=>h('button',{onClick:()=>c.emit('create')},'新增部门')};`,
    '\0tree': `export const Vue3TreeOrg={render(){return null}};`,
  };
  const fixture = {
    name: 'department-fixture', enforce: 'pre',
    resolveId(id) {
      const normalized = id.replaceAll('\\', '/');
      if (id === '/fixture.ts') return '\0fixture';
      if (id === '@/utils/http' || normalized.endsWith('/src/utils/http')) return '\0http';
      if (id === '/@/components/Modal' || normalized.endsWith('/src/components/Modal')) return '\0modal';
      if (id === '@/components/Icon' || normalized.endsWith('/src/components/Icon')) return '\0icons';
      if (id === '/@/components/tabletool' || normalized.endsWith('/src/components/tabletool')) return '\0tools';
      if (id === 'vue3-tree-org') return '\0tree';
    },
    load(id) { return modules[id]; },
    configureServer(server) {
      server.middlewares.use(async (req, res, next) => {
        const url = new URL(req.url, 'http://localhost');
        if (url.pathname === '/fixture') {
          res.setHeader('Content-Type', 'text/html');
          res.end('<!doctype html><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><div id="app"></div><script type="module" src="/fixture.ts"></script>'); return;
        }
        if (!url.pathname.startsWith('/system/dept/')) return next();
        let raw = ''; for await (const chunk of req) raw += chunk;
        const input = raw ? JSON.parse(raw) : Object.fromEntries(url.searchParams);
        requests.push({ path: url.pathname, method: req.method, input });
        let data = {}, code = 0;
        if (url.pathname.endsWith('/getList')) data = departments;
        else if (url.pathname.endsWith('/getParent')) data = departments.map(({ id, name, pid }) => ({ id, name, pid }));
        else if (url.pathname.endsWith('/getLeaderOptions')) {
          if (input.keyword === '慢') await delay(700);
          data = { items: [{ id: 7, name: input.keyword === '慢' ? '过期结果' : '工程甲', employee_no: 'RD-0007', dept_id: '1', department: '研发部' }], total: 1, page: 1, pageSize: 100 };
        } else if (url.pathname.endsWith('/upIMGroup')) {
          if (rejectGroup) code = 1; else departments.find(row => row.id === input.id).im_group_enabled = input.im_group_enabled;
        } else if (url.pathname.endsWith('/save')) {
          Object.assign(departments.find(row => row.id === input.id), input, { leader_name: input.leader_uid ? '工程甲' : '' });
        }
        res.setHeader('Content-Type', 'application/json'); res.end(JSON.stringify({ code, data }));
      });
    },
  };
  const server = await createServer({ root, configFile: false, plugins: [fixture, vue()],
    resolve: { alias: { '@': join(root, 'src'), '/@': join(root, 'src') } },
    optimizeDeps: { noDiscovery: true, include: ['vue', 'vue-i18n', '@arco-design/web-vue', 'lodash', 'lodash/cloneDeep', 'lodash-es', 'dayjs'] },
    server: { host: '127.0.0.1', port: 0 }, logLevel: 'error' });
  await server.listen(); t.after(() => server.close());
  const origin = `http://127.0.0.1:${server.httpServer.address().port}`;
  const browser = process.env.CHROME_PATH || ['C:/Program Files/Google/Chrome/Application/chrome.exe', 'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe'].find(existsSync);
  assert.ok(browser);
  const userDir = await mkdtemp(join(tmpdir(), 'department-im-'));
  const child = spawn(browser, ['--headless=new', '--disable-gpu', '--no-first-run', '--remote-debugging-port=0', `--user-data-dir=${userDir}`, 'about:blank'], { stdio: 'ignore', windowsHide: true });
  t.after(() => child.kill());
  let port;
  for (let i = 0; i < 200; i++) { try { port = Number((await readFile(join(userDir, 'DevToolsActivePort'), 'utf8')).split('\n')[0]); break; } catch { await delay(50); } }
  const target = await (await fetch(`http://127.0.0.1:${port}/json/new?${encodeURIComponent(origin + '/fixture')}`, { method: 'PUT' })).json();
  const socket = new WebSocket(target.webSocketDebuggerUrl);
  await new Promise(resolve => socket.addEventListener('open', resolve, { once: true })); t.after(() => socket.close());
  const cdp = new CDP(socket); await cdp.send('Runtime.enable');
  await cdp.send('Emulation.setDeviceMetricsOverride', { width: 1440, height: 950, deviceScaleFactor: 1, mobile: false });
  try { await cdp.wait("document.querySelector('[data-dept-group=\"1\"]')"); }
  catch (error) { throw new Error(JSON.stringify({ text: await cdp.eval('document.body.innerText'), errors: cdp.errors, requests }), { cause: error }); }
  const groupChecked = id => cdp.eval(`document.querySelector('[data-dept-group="${id}"]').getAttribute('aria-checked')==='true'`);
  await t.test('group column follows status and a rejected update keeps its original value', async () => {
    const labels = await cdp.eval("[...document.querySelectorAll('th')].map(e=>e.textContent.trim())");
    assert.equal(labels[labels.indexOf('状态') + 1], '部门群');
    await cdp.eval("document.querySelector('[data-dept-group=\"1\"]').click()");
    await cdp.wait("!document.querySelector('[data-dept-group=\"1\"]').disabled");
    assert.equal(await groupChecked(1), false);
    assert.deepEqual(requests.find(r => r.path.endsWith('/upIMGroup')).input, { id: 1, im_group_enabled: 1 });
    rejectGroup = false;
    await cdp.eval("document.querySelector('[data-dept-group=\"1\"]').click()");
    await cdp.wait("document.querySelector('[data-dept-group=\"1\"]').getAttribute('aria-checked')==='true'");
  });
  await t.test('a missing leader opens editing without enabling a group', async () => {
    const before = requests.filter(r => r.path.endsWith('/upIMGroup')).length;
    await cdp.eval("document.querySelector('[data-dept-group=\"2\"]').click()");
    await cdp.wait("document.querySelector('[data-testid=dept-leader]')");
    assert.equal(requests.filter(r => r.path.endsWith('/upIMGroup')).length, before);
    await cdp.eval("document.querySelector('[data-testid=dept-group]').click();document.querySelector('[data-submit]').click()");
    await cdp.wait("document.body.innerText.includes('开启部门群前')");
    assert.equal(requests.filter(r => r.path.endsWith('/save')).length, 0);
  });
  await t.test('remote leader search ignores stale responses and saves only editable fields', async () => {
    await cdp.eval("document.querySelector('[data-testid=dept-leader]').click()");
    async function query(value) {
      await cdp.eval(`(()=>{const input=document.querySelector('[data-testid=dept-leader] input');Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value').set.call(input,${JSON.stringify(value)});input.dispatchEvent(new Event('input',{bubbles:true}))})()`);
    }
    await query('慢');
    for (let i = 0; i < 100 && !requests.some(r => r.input.keyword === '慢'); i++) await delay(10);
    assert.ok(requests.some(r => r.input.keyword === '慢'));
    await query('快'); await delay(800);
    assert.equal(await cdp.eval("document.body.innerText.includes('过期结果')"), false);
    await cdp.wait("[...document.querySelectorAll('.arco-select-option')].some(e=>e.textContent.includes('RD-0007'))");
    await cdp.eval("[...document.querySelectorAll('.arco-select-option')].find(e=>e.textContent.includes('RD-0007')).click()");
    await cdp.eval("document.querySelector('[data-submit]').click()");
    await cdp.wait("!document.querySelector('[data-modal]')");
    const saved = requests.find(r => r.path.endsWith('/save')).input;
    assert.deepEqual(saved, { id: 2, name: '财务部', pid: 0, weigh: 2, remark: '', im_group_enabled: 1, leader_uid: 7 });
    const searches = requests.filter(r => r.path.endsWith('/getLeaderOptions'));
    assert.ok(searches.some(r => r.input.keyword === '快' && r.input.dept_id === '2'), JSON.stringify(searches));
  });
  await cdp.wait("document.querySelectorAll('.arco-message').length===0");
  await mkdir(resolve(root, '../dist/qa'), { recursive: true });
  const screenshot = await cdp.send('Page.captureScreenshot', { format: 'png' });
  await writeFile(resolve(root, '../dist/qa/department-im-1440.png'), Buffer.from(screenshot.data, 'base64'));
  assert.deepEqual(cdp.errors, []);
});
