const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const Module = require('node:module');
const { parse, compileScript } = require('@vue/compiler-sfc');
const { build } = require('esbuild');

// Execute the real SFC setup and API wrapper; only modal/HTTP/platform boundaries
// are synthetic. No authenticated backend or employee data is used.
async function loadForm() {
  const filename = path.resolve(__dirname, '../src/views/system/dept/AddForm.vue');
  const { descriptor } = parse(fs.readFileSync(filename, 'utf8'), { filename });
  const source = compileScript(descriptor, { id: 'department-leader-loading' }).content;
  const fixtures = {
    vue: `export const defineComponent=v=>v;export const ref=value=>({value});export const computed=get=>({get value(){return get()}});export const unref=v=>v?.value;export const onBeforeUnmount=fn=>globalThis.__deptLeaderTest.unmount=fn;`,
    '/@/components/Modal': `export const BasicModal={};export const useModalInner=fn=>{globalThis.__deptLeaderTest.open=fn;return[()=>{},{setModalProps(){},closeModal(){}}]};`,
    '@/hooks/loading': `export default()=>({loading:{value:false},setLoading(){}});`,
    'vue-i18n': `export const useI18n=()=>({t:v=>v});`,
    'lodash-es': `export const cloneDeep=v=>structuredClone(v);`,
    '@/components/Icon': `export const IconPicker={};export const Icon={};`,
    '@arco-design/web-vue': `export const Message={warning(){},loading(){},success(){}};`,
    '/@/utils': `export const FilterParam=v=>v;`,
    '@/utils/http': `export const defHttp={get:(config,options)=>globalThis.__deptLeaderTest.get(config,options),post(){throw Error('Unexpected write')},delete(){throw Error('Unexpected write')}};`,
  };
  const result = await build({
    stdin: { contents: source, sourcefile: filename, resolveDir: path.dirname(filename), loader: 'ts' },
    tsconfig: path.resolve(__dirname, '../tsconfig.json'), bundle: true, write: false, platform: 'node', format: 'cjs',
    plugins: [{ name: 'department-boundaries', setup(builder) {
      builder.onResolve({ filter: /./ }, args => Object.hasOwn(fixtures, args.path) ? { path: args.path, namespace: 'boundary' } : undefined);
      builder.onLoad({ filter: /./, namespace: 'boundary' }, args => ({ contents: fixtures[args.path], loader: 'ts' }));
    } }],
  });
  const module = new Module(filename);
  module.paths = require.main.paths;
  module._compile(result.outputFiles[0].text, filename);
  return module.exports.default;
}

const flush = () => new Promise(resolve => setImmediate(resolve));
const record = { id: 53, pid: 15, name: '合成部门', leader_uid: 7, leader_name: '原负责人' };
const page = (id, name) => ({ items: [{ id, name, employee_no: 'TEST-' + id, department: '合成部门' }], total: 1, page: 1, pageSize: 100 });

test('department leader requests settle, retry and preserve the selected leader', async t => {
  const component = await loadForm();
  const state = { requests: [], ignoreAbort: false, get(config, options) {
    if (config.url.endsWith('/getParent')) return Promise.resolve([]);
    assert.ok(config.url.endsWith('/getLeaderOptions'));
    return new Promise((resolve, reject) => {
      state.requests.push({ config, options, resolve, reject });
      if (!state.ignoreAbort) config.signal.addEventListener('abort', () => reject(new Error('cancelled')), { once: true });
    });
  } };
  globalThis.__deptLeaderTest = state;
  t.after(() => { delete globalThis.__deptLeaderTest; });
  const form = component.setup({}, { emit() {} });
  form.formRef.value = { resetFields() {} };
  await state.open({ isUpdate: true, record });

  await t.test('loads only when opened, retaining the current name before any response', async () => {
    assert.equal(state.requests.length, 0);
    assert.deepEqual(form.leaderOptions.value, [{ value: 7, label: '原负责人' }]);
    form.handleLeaderPopup(true);
    assert.equal(form.leaderLoading.value, true);
    const request = state.requests.at(-1);
    assert.deepEqual(request.config.params, { keyword: '', dept_id: 53, parent_id: 15, page: 1, pageSize: 100 });
    assert.equal(request.config.timeout, 15000);
    assert.equal(request.options.retryRequest.isOpenRetry, false);
    assert.equal(request.options.errorMessageMode, 'none');
    request.reject(new Error('timeout; secret/path must not become UI text'));
    await flush();
    assert.equal(form.leaderLoading.value, false);
    assert.equal(form.leaderError.value, '部门负责人加载失败，请重试');
    assert.deepEqual(form.leaderOptions.value, [{ value: 7, label: '原负责人' }]);
  });

  await t.test('retry recovers, accepts the actual unwrapped items shape and does not reload on every open', async () => {
    const retry = form.retryLeaders();
    state.requests.at(-1).resolve(page(8, '新负责人'));
    await retry;
    assert.equal(form.leaderLoading.value, false);
    assert.equal(form.leaderError.value, '');
    assert.deepEqual(form.leaderOptions.value.map(item => item.value), [7, 8]);
    const before = state.requests.length;
    form.handleLeaderPopup(true);
    assert.equal(state.requests.length, before);
  });

  await t.test('a newer search cancels the old request and late completion cannot replace the result', async () => {
    state.ignoreAbort = true;
    const slow = form.searchLeaders('慢');
    const old = state.requests.at(-1);
    const fast = form.searchLeaders('快');
    assert.equal(old.config.signal.aborted, true);
    state.requests.at(-1).resolve(page(9, '最新结果'));
    await fast;
    old.resolve(page(10, '过期结果'));
    await slow;
    assert.deepEqual(form.leaderOptions.value.map(item => item.value), [7, 9]);
    assert.equal(form.leaderLoading.value, false);
    state.ignoreAbort = false;
  });

  await t.test('malformed responses show a retryable error without discarding the current leader', async () => {
    const search = form.searchLeaders('格式');
    state.requests.at(-1).resolve({ items: {} });
    await search;
    assert.equal(form.leaderLoading.value, false);
    assert.equal(form.leaderError.value, '部门负责人加载失败，请重试');
    assert.deepEqual(form.leaderOptions.value.map(item => item.value), [7, 9]);
  });

  await t.test('closing aborts in-flight work and ignores delayed search events; reopening resets the error', async () => {
    const search = form.searchLeaders('关闭');
    const request = state.requests.at(-1);
    form.handleModalVisibility(false);
    await search;
    assert.equal(request.config.signal.aborted, true);
    assert.equal(form.leaderLoading.value, false);
    const before = state.requests.length;
    await form.searchLeaders('关闭后的延迟事件');
    assert.equal(state.requests.length, before);
    await state.open({ isUpdate: true, record: { ...record, id: 54, leader_uid: 11, leader_name: '另一部门负责人' } });
    assert.equal(form.leaderError.value, '');
    assert.deepEqual(form.leaderOptions.value, [{ value: 11, label: '另一部门负责人' }]);
    form.handleLeaderPopup(true);
    const next = state.requests.at(-1);
    assert.equal(next.config.params.dept_id, 54);
    state.unmount();
    await flush();
    assert.equal(next.config.signal.aborted, true);
    assert.equal(form.leaderLoading.value, false);
  });
});
