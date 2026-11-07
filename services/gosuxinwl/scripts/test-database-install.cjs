// Real HTTP smoke test in an isolated runtime directory. Build bin/gosuxin first.
const fs = require('node:fs');
const path = require('node:path');
const net = require('node:net');
const crypto = require('node:crypto');
const { spawn, execFileSync } = require('node:child_process');
const { once } = require('node:events');
const assert = require('node:assert/strict');
const root = path.resolve(__dirname, '..');
const binary = path.join(root, 'bin', process.platform === 'win32' ? 'gosuxin.exe' : 'gosuxin');
const databaseKind = process.env.SUXIN_SMOKE_DATABASE || 'sqlite';
const parent = path.join(root, 'runtime', 'db-compat');
fs.mkdirSync(parent, { recursive: true });
const dir = fs.mkdtempSync(path.join(parent, databaseKind + '-http-'));
let server, logfile, base, token;
const md5 = value => crypto.createHash('md5').update(value).digest('hex');
const wait = ms => new Promise(resolve => setTimeout(resolve, ms));
function copy(source, target = source) {
  const dest = path.join(dir, target);
  fs.mkdirSync(path.dirname(dest), { recursive: true });
  fs.copyFileSync(path.join(root, source), dest);
}
function encrypt(value) {
  const cipher = crypto.createCipheriv('aes-128-cbc', 'eI2lI3zL0qD0hY7m', '6289984834922396');
  return Buffer.concat([cipher.update(value), cipher.final()]).toString('base64');
}
function decrypt(value) {
  const decipher = crypto.createDecipheriv('aes-128-cbc', 'eI2lI3zL0qD0hY7m', '6289984834922396');
  return Buffer.concat([decipher.update(value, 'base64'), decipher.final()]).toString();
}
async function request(url, method = 'GET', body, authenticated = true) {
  const timestamp = Math.floor(Date.now() / 1000);
  const headers = { 'apiverify': Buffer.from(md5('suxin-public-example' + timestamp) + '#' + timestamp).toString('base64') };
  if (authenticated && token) headers.Authorization = encrypt(token + '#' + timestamp);
  const multipart = body instanceof FormData;
  if (body !== undefined && !multipart) headers['Content-Type'] = 'application/json';
  const response = await fetch(base + url, { method, headers, body: body === undefined ? undefined : multipart ? body : JSON.stringify(body), signal: AbortSignal.timeout(120000) });
  const result = await response.json();
  assert.equal(result.code, 0, `${method} ${url}: ${result.message}`);
  return result.data;
}
async function stop() {
  if (server && server.exitCode === null) {
    const exited = once(server, 'exit');
    server.kill();
    await exited;
  }
  server = null;
  if (logfile !== undefined) { fs.closeSync(logfile); logfile = undefined; }
}
async function start() {
  logfile = fs.openSync(path.join(dir, 'server.log'), 'a');
  server = spawn(binary, [], { cwd: dir, stdio: ['ignore', logfile, logfile], windowsHide: true });
  server.on('error', error => console.error(error.message));
  for (let i = 0; i < 100; i++) {
    if (server.exitCode !== null) throw new Error('Server exited; inspect ' + path.join(dir, 'server.log'));
    try { await request('/admin/install/getConfig', 'GET', undefined, false); return; } catch {}
    await wait(100);
  }
  throw new Error('Server did not become ready; inspect ' + path.join(dir, 'server.log'));
}
async function testWorker(port) {
  const workerBinary = path.join(dir, process.platform === 'win32' ? 'db-probe.exe' : 'db-probe');
  execFileSync('go', ['build', '-mod=readonly', '-o', workerBinary, './internal/pluginworker/testdata/dbprobe'], { cwd: root, windowsHide: true, stdio: 'pipe' });
  const secret = crypto.randomBytes(24).toString('hex');
  const workerLog = fs.openSync(path.join(dir, 'worker.log'), 'a');
  const worker = spawn(workerBinary, [], { cwd: dir, windowsHide: true, stdio: ['pipe', workerLog, workerLog], env: { ...process.env, SUXIN_PLUGIN_NAME: 'db-probe', SUXIN_PLUGIN_ADDR: `127.0.0.1:${port}`, SUXIN_PLUGIN_SECRET: secret, SUXIN_PLUGIN_PACKAGE: dir, SUXIN_PLUGIN_DATA: path.join(dir, 'storage/probe'), SUXIN_PLUGIN_BASE: '/plugins/db-probe' } });
  const headers = { 'X-Suxin-Plugin-Secret': secret, 'X-Suxin-Plugin-User': JSON.stringify({ uid: 1 }) };
  try {
    let ready = false;
    for (let i = 0; i < 100; i++) {
      if (worker.exitCode !== null) throw new Error('Test worker exited');
      try { ready = (await fetch(`http://127.0.0.1:${port}/__plugin_health`, { headers })).ok; } catch {}
      if (ready) break;
      await wait(100);
    }
    assert.ok(ready, 'worker did not start');
    const response = await fetch(`http://127.0.0.1:${port}/api/admin/probe`, { method: 'POST', headers, signal: AbortSignal.timeout(15000) });
    const result = await response.json();
    assert.equal(result.code, 0, 'worker database access: ' + result.message);
    const rows = await request('/admin/system/dept/getList');
    assert.ok(JSON.stringify(rows).includes('插件测试车间'), 'host cannot see worker write');
    console.log('PASS independent plugin worker shares the configured database');
  } finally {
    if (worker.exitCode === null) {
      const exited = once(worker, 'exit');
      worker.stdin.end();
      const timer = setTimeout(() => worker.kill(), 5000);
      await exited; clearTimeout(timer);
    }
    fs.closeSync(workerLog);
  }
}
async function main() {
  const listener = net.createServer();
  listener.listen(0, '127.0.0.1'); await once(listener, 'listening');
  const port = listener.address().port;
  await new Promise(resolve => listener.close(resolve));
  base = `http://127.0.0.1:${port}`;
  for (const name of ['config', 'app', 'upload']) {
    copy(`manifest/config/${name}.example.yaml`, `manifest/config/${name}.yaml`);
    const file = path.join(dir, `manifest/config/${name}.yaml`);
    let content = fs.readFileSync(file, 'utf8').replace('address: :8603', `address: 127.0.0.1:${port}`);
    content = content.replaceAll('CHANGE_ME_WITH_INIT_LOCAL_CONFIG', crypto.randomBytes(16).toString('hex'));
    fs.writeFileSync(file, content);
  }
  copy('hack/config.example.yaml', 'hack/config.yaml');
  for (const name of ['sqlite.sql', 'gosuxin.sql', 'webcode.zip', 'index.html', 'vue.js', 'axios.min.js']) copy(`devsource/developer/install/${name}`);
  fs.mkdirSync(path.join(dir, 'resource/uploads'), { recursive: true });
  fs.mkdirSync(path.join(dir, 'resource/static/brand'), { recursive: true });
  fs.mkdirSync(path.join(dir, 'web'), { recursive: true });
  fs.writeFileSync(path.join(dir, 'web/keep.txt'), 'existing frontend');
  await start();
  const installer = await (await fetch(base + '/install/')).text();
  for (const value of ['mysql8', 'mysql57', 'sqlite']) assert.ok(installer.includes(`value="${value}"`));
  await request('/admin/install/save', 'POST', { formDB: databaseKind === 'sqlite' ? { type: 'sqlite', path: 'storage/database/system.db', prefix: 'gf_' } : { type: databaseKind, host: process.env.SUXIN_TEST_MYSQL_HOST, port: process.env.SUXIN_TEST_MYSQL_PORT, user: process.env.SUXIN_TEST_MYSQL_USER, pass: process.env.SUXIN_TEST_MYSQL_PASSWORD || '', name: 'suxin_smoke_' + Date.now(), prefix: 'gf_' }, formApp: { username: 'factory_admin', password: 'smoke-test-password', vueobjroot: 'web' } }, false);
  assert.ok(fs.existsSync(path.join(dir, 'devsource/developer/install/install.lock')));
  assert.equal(fs.readFileSync(path.join(dir, 'web/keep.txt'), 'utf8'), 'existing frontend');
  assert.ok(fs.existsSync(path.join(dir, 'web/gosuxin-web/package.json')));
  console.log('PASS ' + databaseKind + ' HTTP installation and selector');
  await stop(); await start();
  token = decrypt(await request('/admin/user/login', 'POST', { username: 'factory_admin', password: md5('smoke-test-password') }, false));
  const identity = await request('/admin/user/getUserinfo');
  assert.equal(identity.name, 'Suxin管理员');
  assert.equal(identity.mobile || '', '');
  assert.equal(identity.email || '', '');
  const menu = await request('/admin/user/getMenu');
  assert.ok(menu);
  console.log('PASS restart, login, identity and dynamic menus');
  await request('/admin/system/dept/save', 'POST', { name: '测试车间', pid: 1, remark: '', status: 0 });
  const departments = await request('/admin/system/dept/getList');
  const flatten = list => list.flatMap(row => [row, ...flatten(row.children || [])]);
  const dept = flatten(departments).find(row => row.name === '测试车间');
  assert.ok(dept && Number(dept.id) > 1, 'department must receive an auto-generated ID');
  await request('/common/basetool/table/weigh', 'POST', { tableanme: 'auth_dept', pid: '1', weighList: [{ id: Number(dept.id), weigh: 30 }] });
  assert.equal(Number(flatten(await request('/admin/system/dept/getList')).find(row => Number(row.id) === Number(dept.id)).weigh), 30);
  await request('/admin/system/dept/save', 'POST', { id: Number(dept.id), name: '测试车间已更新', pid: 1, remark: '', status: 0 });
  const tables = await request('/common/basetool/table/getTables');
  assert.ok(tables.some(table => table.name === 'gf_admin'));
  await request('/admin/system/dept/del', 'DELETE', { ids: [Number(dept.id)] });
  const remaining = flatten(await request('/admin/system/dept/getList'));
  assert.ok(!remaining.some(row => Number(row.id) === Number(dept.id)), 'department deletion failed');
  console.log('PASS department CRUD, sorting and database metadata');
  await request('/admin/datacenter/tabledata/save', 'POST', { title: '测试班次', db_way: 'sys', remark: '' });
  const groups = await request('/admin/datacenter/tabledata/getList');
  const group = groups.find(row => row.title === '测试班次');
  assert.ok(group && Number(group.id) > 0);
  await request('/admin/datacenter/dictionary/save', 'POST', { tablename: 'dictionary_data', group_id: Number(group.id), keyname: '早班', keyvalue: 'day' });
  const dictionary = await request('/admin/datacenter/dictionary/getList?tablename=dictionary_data&group_id=' + group.id);
  const item = dictionary.items.find(row => row.keyvalue === 'day');
  assert.ok(item && Number(item.id) > 0);
  await request('/common/basetool/table/weigh', 'POST', { tableanme: 'dictionary_data', weighList: [{ id: Number(item.id), weigh: 12 }] });
  console.log('PASS dictionary creation and sorting tables without a parent column');
  await request('/admin/system/account/save', 'POST', { username: 'test_operator', name: '测试人员', password: md5('operator-password'), roleid: [1], dept_id: 1, tel: '', mobile: '', email: '', remark: '' });
  const adminToken = token;
  token = decrypt(await request('/admin/user/login', 'POST', { username: 'test_operator', password: md5('operator-password') }, false));
  await request('/admin/system/dept/getList');
  console.log('PASS account creation, role assignment and authorized access');
  token = adminToken;
  await request('/admin/developer/ai/interfaces');
  console.log('PASS built-in AI administration');
  const workerListener = net.createServer();
  workerListener.listen(0, '127.0.0.1'); await once(workerListener, 'listening');
  const workerPort = workerListener.address().port;
  await new Promise(resolve => workerListener.close(resolve));
  await testWorker(workerPort);
  await testPluginUpload();
  await request('/admin/user/logout', 'POST', {});
  console.log('PASS logout');
  console.log('Isolated test files: ' + dir);
}
async function testPluginUpload() {
  const catalogPath = '/admin/developer/packinstall/getInstallPack?catalog=true';
  assert.deepEqual(await request(catalogPath), [], 'fresh host must not ship preinstalled business plugins');
  const filename = process.platform === 'win32' ? 'db-probe.exe' : 'db-probe';
  const html = '<h1>Suxin plugin fixture</h1>';
  const binary = path.join(dir, filename);
  const archive = path.join(dir, 'db-probe.zip');
  const manifest = { format: 'suxin-runtime-v1', name: 'db-probe', title: 'Suxin 示例插件', version: '1.0.0', protocol: 1,
    os: process.platform === 'win32' ? 'windows' : process.platform,
    arch: process.arch === 'x64' ? 'amd64' : process.arch,
    files: {
      ['bin/' + filename]: crypto.createHash('sha256').update(fs.readFileSync(binary)).digest('hex'),
      'admin/index.html': crypto.createHash('sha256').update(html).digest('hex'),
    },
  };
  execFileSync(process.env.PYTHON || (process.platform === 'win32' ? 'python' : 'python3'), ['-c',
    'import sys,zipfile\nwith zipfile.ZipFile(sys.argv[1], "w", zipfile.ZIP_DEFLATED) as z:\n z.write(sys.argv[2], sys.argv[3])\n z.writestr("plugin.json", sys.argv[4])\n z.writestr("admin/index.html", sys.argv[5])',
    archive, binary, 'bin/' + filename, JSON.stringify(manifest), html], { windowsHide: true, stdio: 'pipe' });
  const form = new FormData();
  form.set('file', new Blob([fs.readFileSync(archive)], { type: 'application/zip' }), 'db-probe.zip');
  const upload = await request('/admin/developer/packinstall/installLocalCode', 'POST', form);
  await request('/admin/developer/packinstall/installCode', 'POST', { name: upload });
  try {
    assert.ok((await request(catalogPath)).some(row => row.name === 'db-probe' && row.installed));
    assert.ok(JSON.stringify(await request('/admin/user/getMenu')).includes('runtime_plugin_db-probe'));
    await request('/plugins/db-probe/api/admin/probe', 'POST', {});
    const retained = path.join(dir, 'storage/plugins/db-probe/data/retained.txt');
    fs.mkdirSync(path.dirname(retained), { recursive: true });
    fs.writeFileSync(retained, 'plugin data');
    await request('/admin/developer/packinstall/uninstallCode', 'POST', { name: 'db-probe' });
    assert.equal(fs.readFileSync(retained, 'utf8'), 'plugin data');
    assert.ok((await request(catalogPath)).some(row => row.name === 'db-probe' && !row.installed));
    assert.ok(!JSON.stringify(await request('/admin/user/getMenu')).includes('runtime_plugin_db-probe'));
    await request('/admin/developer/packinstall/installCode', 'POST', { name: 'db-probe' });
    await request('/plugins/db-probe/api/admin/probe', 'POST', {});
    console.log('PASS empty plugin catalog, ZIP upload/install, menu, authenticated API, uninstall and reinstall with retained data');
  } finally {
    await request('/admin/developer/packinstall/uninstallCode', 'POST', { name: 'db-probe' });
  }
}
main().catch(error => { console.error(error.message); process.exitCode = 1; }).finally(stop);
