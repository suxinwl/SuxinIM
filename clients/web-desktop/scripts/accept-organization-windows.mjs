// Real packaged Windows UI acceptance, restricted to the current C/D fixture.
// No registration/friend/group changes; credentials and tokens stay in memory.
import { _electron } from 'playwright';
import fs from 'node:fs/promises';
import path from 'node:path';
import os from 'node:os';
const client = path.resolve(import.meta.dirname, '..'), suite = path.resolve(client, '../..');
const fixture = JSON.parse((await fs.readFile(path.join(suite, '.private/production/acceptance/organization-test-accounts.json'), 'utf8')).replace(/^\uFEFF/, ''));
if (fixture.namespace !== 'organization-20261103' || fixture.accounts.length !== 2) throw new Error('FIXTURE_NAMESPACE_REJECTED');
const [actor, colleague] = fixture.accounts, origin = fixture.lanOrigin;
if (origin !== 'https://192.168.10.11:9885') throw new Error('ENTRY_REJECTED');
const stamp = Date.now(), evidence = path.join(suite, '.private/production/acceptance', 'organization-windows-' + stamp);
await fs.mkdir(evidence, { recursive: true });
const report = { namespace: fixture.namespace, platform: 'windows', expectedVersion: '3.8.9', expectedBuild: 380901, strictTLS: true, fixtureOnly: true, productionPublished: false, checks: [], pageErrors: [], websocketFrames: 0, logout: false };
let app, page, session, stage = 'launch';
const wait = ms => new Promise(r => setTimeout(r, ms));
const check = value => report.checks.push(value);
const shot = name => page.screenshot({ path: path.join(evidence, name + '.png'), mask: [page.locator('input[type=password]')] });
const directoryReply = () => page.waitForResponse(r => new URL(r.url()).pathname === '/sso/directory', { timeout: 60000 });
async function searchOrganization(value) {
  const input = page.getByLabel('按手机号、姓名或工号查找同事');
  await input.fill(String(value));
  const reply = directoryReply(); await input.press('Enter');
  const response = await reply, body = await response.json();
  if (response.status() !== 200 || body.code !== 0) throw new Error('DIRECTORY_REJECTED');
  const person = body.data?.items?.find(p => p.employeeNo === colleague.username);
  if (!person) throw new Error('COLLEAGUE_NOT_FOUND');
  await page.getByRole('button', { name: person.nickname, exact: true }).waitFor();
  return person;
}
try {
  app = await _electron.launch({ executablePath: process.env.PUTY_ORGANIZATION_EXE || path.join(client, 'release/win-unpacked/PUTY_IM.exe'), env: { ...process.env, OPENIM38_TEST_DATA_DIR: path.join(os.tmpdir(), 'puty-im-qa', 'org-' + stamp) }, timeout: 60000 });
  for (let i = 0; i < 120; i++) { page = app.windows().find(p => p.url().startsWith('openim38://app/')); if (page) break; await wait(250); }
  if (!page) throw new Error('WINDOW_UNAVAILABLE');
  page.setDefaultTimeout(45000);
  page.on('pageerror', () => report.pageErrors.push('renderer_exception'));
  page.on('websocket', ws => { if (new URL(ws.url()).pathname === '/ws') ws.on('framereceived', () => report.websocketFrames++); });
  stage = 'prelogin';
  await page.getByRole('heading', { name: 'PUTY内部通讯', exact: true }).waitFor();
  await page.waitForFunction(() => typeof window.login === 'function', undefined, { timeout: 60000 }); check('native_wasm_initialized');
  if (await app.evaluate(({app}) => app.getVersion()) !== '3.8.9') throw new Error('VERSION_REJECTED');
  await page.getByLabel('统一账号 / 工号').fill(actor.username); await page.getByLabel('密码', { exact: true }).fill(actor.password);
  const captchaInput = page.getByLabel('验证码', { exact: true });
  await page.getByAltText('点击更换验证码').waitFor();
  if (await captchaInput.isVisible()) {
    const challenge = path.join(evidence, 'captcha');
    await page.getByAltText('点击更换验证码').screenshot({ path: challenge + '.png' });
    console.log(JSON.stringify({ stage: 'captcha', image: challenge + '.png', answerFile: challenge + '.txt' }));
    for (let i = 0; i < 600; i++) { try { const answer = (await fs.readFile(challenge + '.txt', 'utf8')).trim(); if (answer) { await captchaInput.fill(answer); break; } } catch {} await wait(250); }
    if (!await captchaInput.inputValue()) throw new Error('CAPTCHA_ANSWER_UNAVAILABLE');
  }
  stage = 'login';
  const loginReply = page.waitForResponse(r => new URL(r.url()).pathname === '/sso/login', { timeout: 60000 });
  await page.getByRole('button', { name: '登录', exact: true }).click();
  const loginResponse = await loginReply, loginBody = await loginResponse.json();
  if (loginResponse.status() !== 200 || loginBody.code !== 0) throw new Error('SSO_REJECTED');
  session = loginBody.data;
  await page.getByText('组织架构', { exact: true }).first().waitFor({ timeout: 90000 }); check('normal_login_sdk_main');
  await page.getByText('组织架构', { exact: true }).first().click();
  await page.getByRole('heading').filter({ hasText: '组织架构' }).waitFor();
  stage = 'organization-search';
  let person;
  for (const [mode, value] of [['phone', colleague.mobile], ['name', colleague.name], ['employee_no', colleague.username]]) { person = await searchOrganization(value); check('organization_' + mode); }
  await shot('organization-search');
  stage = 'department';
  const deptTitle = page.locator('.ant-tree-title').getByText(person.department, { exact: true });
  const deptResponse = directoryReply(); await deptTitle.click();
  const deptBody = await (await deptResponse).json();
  if (!deptBody.data?.items?.some(p => p.userID === person.userID) || deptBody.data.items.some(p => String(p.departmentID) !== String(fixture.departmentID))) throw new Error('DEPARTMENT_FILTER_REJECTED');
  check('department_people_panel'); await shot('department');
  stage = 'profile';
  await page.getByRole('button', { name: person.nickname, exact: true }).click();
  await page.getByRole('button', { name: '发送消息', exact: true }).waitFor();
  await page.getByRole('button', { name: '添加好友', exact: true }).waitFor();
  await page.getByText(person.employeeNo, { exact: true }).last().waitFor(); check('nonfriend_profile_direct_chat'); await shot('profile');
  await page.getByRole('button', { name: '发送消息', exact: true }).click();
  stage = 'direct-chat';
  const editor = page.locator('#editable-div'); await editor.waitFor();
  const marker = 'puty-organization-windows-' + stamp;
  await editor.fill(marker); await page.getByRole('button', { name: '发送', exact: true }).click();
  await page.getByText(marker, { exact: true }).last().waitFor();
  await wait(2000);
  if (await page.locator('.anticon-exclamation-circle').isVisible().catch(() => false)) throw new Error('MESSAGE_SEND_REJECTED');
  report.sentStatus = await page.evaluate(async marker => {
    let result = await window.searchLocalMessages(crypto.randomUUID(), JSON.stringify({ conversationID:'', keywordList:[marker], messageTypeList:[], searchTimePosition:0, searchTimePeriod:0, pageIndex:1, count:20 }));
    if (typeof result === 'string') result = JSON.parse(result);
    if (result.errCode) return false;
    const payload = result.data ?? result;
    const data = typeof payload === 'string' ? JSON.parse(payload) : payload;
    return (data?.searchResultItems || []).flatMap(item => item.messageList || []).some(message => {
      // This calls the raw Go export, before SDK adaptPayload creates TextElem.
      let content = message.textElem?.content;
      if (content === undefined && typeof message.content === 'string') {
        try { content = JSON.parse(message.content).content; } catch {}
      }
      return message.status === 2 && content === marker;
    });
  }, marker);
  if (!report.sentStatus) throw new Error('SDK_SEND_ACK_MISSING');
  check('nonfriend_ui_send_text'); await shot('direct-chat');
  stage = 'add-friend-search';
  await page.locator('img.app-no-drag.ml-8').click(); await page.getByText('添加好友', { exact: true }).last().click();
  const addInput = page.getByPlaceholder('输入员工手机号、姓名或工号');
  for (const [mode, value] of [['phone', colleague.mobile], ['name', colleague.name], ['employee_no', colleague.username]]) {
    const reply = directoryReply(); await addInput.fill(String(value)); await addInput.press('Enter'); const body = await (await reply).json();
    if (!body.data?.items?.some(p => p.userID === person.userID)) throw new Error('ADD_SEARCH_REJECTED');
    await page.getByRole('button').filter({ hasText: person.nickname }).last().waitFor(); check('add_friend_' + mode);
  }
  await shot('add-friend-search'); await page.getByRole('button', { name: '取消', exact: true }).last().click();
  stage = 'global-search';
  await page.getByText('搜索', { exact: true }).first().click();
  const globalInput = page.getByPlaceholder('搜索手机号、姓名、工号或聊天内容');
  for (const [mode, value] of [['phone', colleague.mobile], ['name', colleague.name], ['employee_no', colleague.username]]) {
    const reply = directoryReply(); await globalInput.fill(String(value)); const body = await (await reply).json();
    if (!body.data?.items?.some(p => p.userID === person.userID)) throw new Error('GLOBAL_SEARCH_REJECTED');
    await page.getByText(person.nickname, { exact: true }).last().waitFor(); check('global_' + mode);
  }
  await shot('global-search');
  if (report.pageErrors.length || !report.websocketFrames) throw new Error('SDK_RUNTIME_REJECTED');
  report.passed = true;
} catch (error) {
  report.passed = false; report.failureStage = stage; report.failureType = error.name; report.failureCategory = /^[A-Z_]+$/.test(error.message) ? error.message : 'UI_CHECK_FAILED'; process.exitCode = 1;
  if (page) await shot('failure').catch(() => {});
} finally {
  if (session && page) try { report.logout = await page.evaluate(async ({origin, sessionToken}) => { const response = await fetch(origin + '/sso/logout', { method:'POST', headers:{'Content-Type':'application/json'}, body:JSON.stringify({sessionToken}) }); return response.status === 200 && (await response.json()).code === 0; }, {origin,sessionToken:session.sessionToken}); } catch {}
  if (app) { await Promise.race([app.evaluate(({app}) => app.exit(0)).catch(() => {}), wait(3000)]); await Promise.race([app.close().catch(() => {}), wait(3000)]); }
  await fs.writeFile(path.join(evidence, 'report.json'), JSON.stringify(report, null, 2));
  console.log(JSON.stringify({passed:report.passed,checks:report.checks.length,failureStage:report.failureStage,failureCategory:report.failureCategory,logout:report.logout,evidence}));
}
