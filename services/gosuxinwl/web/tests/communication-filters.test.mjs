import test from 'node:test';
import assert from 'node:assert/strict';
import { extractResponse } from '../src/views/communication/model.ts';
import { employeeFilterValues, filterRecords, employeeDepartments } from '../src/views/communication/filters.ts';

const all = { keyword: '', department: '', enabled: '', status: '' };
const row = (value, key = 'row') => ({ ...value, __key: key });
const filter = (rows, value, employees = true) => filterRecords(rows, { ...all, ...value }, employees);

test('employee values match flattened projections and visible nested profile fields', () => {
  const { rows } = extractResponse([{ status: 'ready', projection: { nickname: ' 员工甲 ', account: ' 0001 ', departmentName: ' 研发部 ', enabled: true, profile: { name: '备用姓名', account: '备用工号' } } }]);
  assert.deepEqual(employeeFilterValues(rows[0]), { name: '员工甲', account: '0001', department: '研发部', enabled: 'true', status: 'ready', reason: '' });
  assert.deepEqual(employeeFilterValues(row({ profile: { name: ' 员工乙 ', account: '0002', departmentName: '销售部' }, enabled: false, status: 'failed', reason: ' 权限限制 ', error: '上游同步失败' })), { name: '员工乙', account: '0002', department: '销售部', enabled: 'false', status: 'failed', reason: '权限限制；上游同步失败' });
});

test('preview derives enable and sync filters while explicit booleans and states win', () => {
  assert.deepEqual(employeeFilterValues(row({ name: '预览甲', employeeNo: 'P01', willEnable: true })), { name: '预览甲', account: 'P01', department: '', enabled: 'true', status: 'pending', reason: '' });
  const blocked = employeeFilterValues(row({ name: '预览乙', willEnable: false, reason: '需要补充工号', error: '需要补充工号' }));
  assert.equal(blocked.enabled, 'false');
  assert.equal(blocked.status, 'waiting');
  assert.equal(blocked.reason, '需要补充工号');
  for (const status of ['ready', 'pending', 'waiting', 'failed']) {
    const values = employeeFilterValues(row({ account: '0000', enabled: false, willEnable: true, status }));
    assert.equal(values.enabled, 'false');
    assert.equal(values.status, status);
  }
});

test('missing account is visible as missing, not coerced from an internal identifier', () => {
  const entry = row({ name: '待处理员工', employeeID: 'hidden-employee', hostUID: 1234, enabled: false, status: 'waiting' });
  assert.equal(employeeFilterValues(entry).account, '');
  assert.equal(employeeFilterValues(entry).reason, '缺少工号');
  assert.equal(filter([entry], { keyword: '缺少工号' }).length, 1);
  assert.equal(filter([entry], { keyword: '未填写' }).length, 1);
  assert.equal(filter([entry], { keyword: 'hidden-employee' }).length, 0);
  assert.equal(filter([entry], { keyword: '1234' }).length, 0);
});

test('employee keyword covers only visible identity, reasons and Chinese status text', () => {
  const entry = row({ nickname: 'Alice 张', account: 'HR-A12', departmentName: '研发部', enabled: false, status: 'failed', reason: '上游需要核对', authFingerprint: 'private-fingerprint', employeeID: 'private-employee', hostUID: 'private-host', userID: 'private-im', profile: { employeeID: 'private-profile' } });
  for (const keyword of [' alice ', 'hr-a12', '研发', '需要核对', '同步失败', '未启用']) assert.equal(filter([entry], { keyword }).length, 1, keyword);
  for (const keyword of ['private-fingerprint', 'private-employee', 'private-host', 'private-im', 'private-profile', entry.__key, 'failed']) assert.equal(filter([entry], { keyword }).length, 0, keyword);
});

test('preview labels and all canonical Chinese synchronization labels can be searched', () => {
  const preview = [row({ name: '甲', employeeNo: '1', willEnable: true }), row({ name: '乙', employeeNo: '2', willEnable: false })];
  assert.equal(filter(preview, { keyword: '可开通', enabled: 'true' }).length, 1);
  assert.equal(filter(preview, { keyword: '暂不可开通' }).length, 1);
  assert.deepEqual(filter(preview, { keyword: '待开通' }), [preview[0]]);
  assert.deepEqual(filter(preview, { keyword: '待处理' }), [preview[1]]);
  assert.deepEqual(filter(preview, { keyword: '待同步' }), []);
  const explicitPreview = row({ name: '显式预览状态', account: '3', willEnable: true, status: 'pending' });
  assert.deepEqual(filter([explicitPreview], { keyword: '待开通', status: 'pending' }), [explicitPreview]);
  assert.deepEqual(filter([explicitPreview], { keyword: '待同步' }), []);
  for (const [status, keyword] of [['ready', '已同步'], ['pending', '待同步'], ['waiting', '待开通'], ['failed', '同步失败']]) assert.equal(filter([row({ account: '1', status })], { keyword }).length, 1);
});

test('all four employee filters combine with AND and do not mutate source rows', () => {
  const rows = [row({ name: 'Alice', account: '01', departmentName: '研发部', enabled: true, status: 'ready' }, 'a'), row({ name: 'Alice', account: '02', departmentName: '销售部', enabled: true, status: 'ready' }, 'b'), row({ name: 'Alice', account: '03', departmentName: '研发部', enabled: false, status: 'ready' }, 'c'), row({ name: 'Alice', account: '04', departmentName: '研发部', enabled: true, status: 'pending' }, 'd')];
  const before = structuredClone(rows);
  assert.deepEqual(filter(rows, { keyword: ' ALICE ', department: '研发部', enabled: 'true', status: 'ready' }), [rows[0]]);
  assert.deepEqual(filter(rows, all), rows);
  assert.deepEqual(filter(rows, { department: '不存在的部门' }), []);
  assert.deepEqual(rows, before);
});

test('departments come from normalized visible values, are unique and sorted', () => {
  const rows = [row({ departmentName: ' 销售部 ' }), row({ profile: { departmentName: '研发部' } }), row({ departmentName: '销售部' }), row({ departmentName: '', profile: {} }), row({ projection: { departmentName: '财务部' } })];
  const expected = ['财务部', '销售部', '研发部'];
  assert.deepEqual(employeeDepartments(rows), expected);
  assert.deepEqual(employeeDepartments([]), []);
});

test('generic results search nested visible values and ignore employee-only controls', () => {
  const rows = [row({ groupName: 'Research Team', members: [{ nickname: 'ALIce' }], count: 42, enabled: false }), row({ title: '销售团队', nested: { enabled: true } }, 'b')];
  assert.deepEqual(filter(rows, { keyword: ' alice ', department: '不适用', enabled: 'true', status: 'failed' }, false), [rows[0]]);
  assert.deepEqual(filter(rows, { keyword: 'research' }, false), [rows[0]]);
  assert.deepEqual(filter(rows, { keyword: '42' }, false), [rows[0]]);
  assert.deepEqual(filter(rows, { keyword: '否' }, false), [rows[0]]);
  assert.deepEqual(filter(rows, all, false), rows);
});

test('generic recursion omits sensitive and synthetic keys at every depth', () => {
  const entry = row({ title: 'visible', token: 'secret-top-token', authFingerprint: 'secret-top-auth', password: 'secret-top-password', salt: 'secret-top-salt', nested: [{ __key: 'secret-inner-key', auth_fingerprint: 'secret-inner-auth', Password: 'secret-inner-password', Salt: 'secret-inner-salt', Token: 'secret-inner-token', refreshToken: 'secret-refresh-token', text: 'visible-inner' }] }, 'secret-row-key');
  for (const keyword of ['secret-top-token', 'secret-top-auth', 'secret-top-password', 'secret-top-salt', 'secret-inner-key', 'secret-inner-auth', 'secret-inner-password', 'secret-inner-salt', 'secret-inner-token', 'secret-refresh-token', 'secret-row-key']) assert.deepEqual(filter([entry], { keyword }, false), [], keyword);
  assert.deepEqual(filter([entry], { keyword: 'VISIBLE-INNER' }, false), [entry]);
  assert.deepEqual(filter([entry], { keyword: 'password' }, false), [], 'property names are not searchable values');
});

test('generic recursion handles empty, repeated and cyclic data without losing visible siblings', () => {
  const repeated = { text: 'shared visible' };
  const entry = row({ nested: [null, undefined, repeated, repeated], empty: {}, value: NaN });
  entry.loop = entry;
  assert.deepEqual(filter([entry], { keyword: 'shared visible' }, false), [entry]);
  assert.deepEqual(filter([entry], { keyword: 'not present' }, false), []);
  assert.deepEqual(filter([], all, false), []);
});

test('unknown visible status strings do not collide with JavaScript object properties', () => {
  const entry = row({ account: '01', status: 'constructor' });
  assert.deepEqual(filter([entry], { keyword: 'constructor' }), [entry]);
});
