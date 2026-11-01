// Test-only original-UI peers102/103. Never touches the independent Web101 helper.
// No call, message, group or meeting operation runs merely by starting this file.
import { chromium } from 'playwright';
import { readFile, writeFile, mkdir } from 'node:fs/promises';
import { mkdirSync } from 'node:fs';
import { resolve } from 'node:path';
import { createServer } from 'node:http';
import { captureVideoState } from './video-frame-state.mjs';
import { installRtcTransportObserver, captureAudioState, captureRtcTransportState } from './rtc-media-state.mjs';

if (process.argv.includes('--help')) {
  console.log('After the Android owner confirms the Web101 call window is finished: node tests/android-collaboration.mjs --start-confirmed. Control only localhost17788, peers102/103. GET/state, GET/messages; explicit POST/create-group, /announce-group, /open-group, /open-android, /composer, /accept, /hangup, /create-meeting, /invite-meeting-card, /join-meeting, /join-meeting-card, /meeting-control, /meeting-setting, /screenshot, /close. Composer is restricted to Web102 -> Android104 and real input/type/clear/blur; it never sends a message or invokes typing SDK methods. Startup only logs in synthetic Web102/103; no call/message/group/meeting is started. No production endpoints, TLS bypass or arbitrary evaluate endpoint.');
  process.exit(0);
}
if (!process.argv.includes('--start-confirmed')) throw new Error('Wait for the Android owner and pass --start-confirmed; startup logs in two independent synthetic Web peers.');
const temp = resolve('../../.cache/temp-web/android-collaboration');
mkdirSync(temp, { recursive: true }); process.env.TEMP = temp; process.env.TMP = temp;
const base = 'https://192.168.10.11:29880';
const fixtures = JSON.parse(await readFile('../../.private/test-accounts.json', 'utf8'));
const accounts = { '102': fixtures.users[101], '103': fixtures.users[102] };
const android = fixtures.users[103];
if (accounts['102'].userID !== 'upgrade38_u102' || accounts['103'].userID !== 'upgrade38_u103' || android.userID !== 'upgrade38_u104') throw new Error('Synthetic fixture identity mismatch');
const out = resolve('test-results/android-collaboration'); await mkdir(out, { recursive: true });
const sanitize = value => String(value).replace(/((?:https?|wss?):\/\/[^\s?]+)\?[^\s]+/g, '$1?[redactedQuery]').replace(/eyJ[A-Za-z0-9_.-]+/g, '[redactedJWT]');
const errors = [], operations = [], peers = new Map(), groups = new Map(), meetings = new Map();
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE,
  args: ['--use-fake-device-for-media-stream', '--use-fake-ui-for-media-stream', '--enable-usermedia-screen-capturing', '--auto-select-desktop-capture-source=Entire screen'] });

for (const [key, account] of Object.entries(accounts)) {
  const context = await browser.newContext({ permissions: ['camera', 'microphone'], viewport: { width: 1440, height: 1000 } });
  const page = await context.newPage(); page.setDefaultTimeout(30000);
  page.on('pageerror', error => errors.push({ peer: key, message: sanitize(error.message) }));
  await page.addInitScript(installRtcTransportObserver);
  await page.addInitScript(({ base }) => {
    for (const [k, v] of Object.entries({ apiUrl: base + '/api', chatUrl: base + '/chat', enterpriseUrl: base + '/enterprise/v1', wsUrl: base.replace('https:', 'wss:') + '/ws' })) localStorage.setItem('upgrade38:' + k, v);
  }, { base });
  await page.goto('http://127.0.0.1:7777', { waitUntil: 'domcontentloaded', timeout: 120000 });
  await page.waitForFunction(() => typeof window.commonEventFunc === 'function', undefined, { timeout: 120000 });
  await page.getByRole('tab', { name: '邮箱', exact: true }).click();
  await page.locator('#email').fill(account.email); await page.locator('#password').fill(fixtures.password);
  await page.getByRole('button', { name: '登录', exact: true }).click();
  await page.waitForURL('**/#/chat**', { timeout: 90000 });
  await page.evaluate(async () => {
    window.qaIm = (await import('/src/im/ImFacade.ts')).ImFacade;
    window.qaStore = await import('/src/store/index.ts'); window.qaMessages = [];
    for (const event of ['OnRecvNewMessage', 'OnRecvNewMessages']) window.qaIm.on(event, e => window.qaMessages.push(...(Array.isArray(e.data) ? e.data : [e.data])));
  });
  // Chat route navigation precedes the product's asynchronous IM/resource
  // initialization. Only read real SDK state; never force a second SDK login.
  await page.waitForFunction(async expectedUserID => {
    try {
      const login = await window.qaIm.getLoginStatus();
      if (login.errCode !== 0 || Number(login.data) !== 3) return false;
      const [self, conversations] = await Promise.all([window.qaIm.getSelfUserInfo(), window.qaIm.getConversationListSplit({ offset: 0, count: 1 })]);
      if (self.errCode !== 0 || self.data.userID !== expectedUserID || conversations.errCode !== 0 || !Array.isArray(conversations.data)) return false;
      window.qaHealth = { checkedAt: new Date().toISOString(), userID: self.data.userID, loginStatus: Number(login.data), wasmCallbackReady: typeof window.commonEventFunc === 'function', sqliteConversationRead: true, sqliteResultCount: conversations.data.length, observerInstalled: Boolean(window.collabRtcObserver) };
      return true;
    } catch { return false; }
  }, account.userID, { timeout: 90000, polling: 500 });
  peers.set(key, page);
}

const getPeer = body => { const page = peers.get(body.peer || '102'); if (!page) throw new Error('Only independent synthetic peers102/103 are available'); return page; };
const fixedGroup = id => { if (!groups.has(id)) throw new Error('Only a group created by this helper is allowed'); return id; };
const fixedMeeting = id => { if (!/^[a-f0-9]{32}$/.test(id || '')) throw new Error('Invalid synthetic meeting identity'); return id; };
async function openGroup(page, id) {
  fixedGroup(id);
  await page.evaluate(async groupID => { const { data: c } = await window.qaIm.getOneConversation({ sourceID: groupID, sessionType: 3 }); window.qaStore.useConversationStore.getState().updateCurrentConversation(c); location.hash = '/chat/' + c.conversationID; }, id);
}
async function openAndroid(page) {
  await page.evaluate(async userID => { const { data: c } = await window.qaIm.getOneConversation({ sourceID: userID, sessionType: 1 }); window.qaStore.useConversationStore.getState().updateCurrentConversation(c); location.hash = '/chat/' + c.conversationID; }, android.userID);
}
async function menu(page) {
  await page.locator('img[src*="show_more"]').click();
  await page.locator('.ant-popover:visible').getByText('视频会议', { exact: true }).click();
  await page.locator('.meeting-manage-modal').waitFor({ state: 'visible' });
}
async function state(page, peer) {
  const videos = await page.evaluate(captureVideoState);
  const audios = await page.evaluate(captureAudioState);
  const transport = await page.evaluate(captureRtcTransportState);
  const summary = await page.evaluate(() => ({ health: window.qaHealth, pageURL: location.origin + location.pathname + location.hash, body: document.body.innerText.slice(-7000),
    sources: [...document.querySelectorAll('video')].map(v => ({ source: v.dataset.lkSource || null, localParticipant: v.dataset.lkLocalParticipant || null })),
    meetingVisible: Boolean(document.querySelector('.meeting-modal')), received: window.qaMessages.length,
    meetingControls: [...document.querySelectorAll('.meeting-modal button, .meeting-modal [role="switch"]')].map(element => ({ text: (element.textContent || '').trim().slice(0, 80), disabled: element.disabled ?? null, ariaDisabled: element.getAttribute('aria-disabled'), checked: element.getAttribute('aria-checked') })) }));
  for (const connection of transport.peers) for (const rtp of connection.rtp || []) {
    rtp.audioElementIDs = audios.filter(audio => audio.audioTracks.some(track => track.trackID === rtp.trackIdentifier)).map(audio => audio.audioID);
    rtp.videoElementIDs = videos.filter(video => video.videoTracks.some(track => track.trackID === rtp.trackIdentifier)).map(video => video.videoID);
  }
  return { peer, sampledAt: new Date().toISOString(), ...summary, videos: videos.map((v, i) => ({ ...v, ...summary.sources[i] })), audios, transport };
}
async function writeJournal() { await writeFile(resolve(out, 'operations.json'), JSON.stringify({ operations, errors, boundary: 'Synthetic Web102/103 original UI. Test-only group setup and invitation messages use the real fixed SDK; camera/microphone/display sources are synthetic. No Android physical-device or production acceptance claim.' }, null, 2)); }

const server = createServer(async (req, res) => {
  try {
    if (req.headers.origin) throw new Error('Browser-origin commands are not accepted');
    const url = new URL(req.url, 'http://127.0.0.1'); let body = {};
    if (req.method === 'POST') { let raw = ''; for await (const chunk of req) { raw += chunk; if (raw.length > 2048) throw new Error('Request too large'); } body = raw ? JSON.parse(raw) : {}; }
    else if (req.method !== 'GET') throw new Error('Unsupported method');
    let result;
    if (url.pathname === '/state' && req.method === 'GET') {
      result = { peers: await Promise.all([...peers].map(([key, page]) => state(page, key))), groups: [...groups.values()], createdMeetingRoomIDs: [...meetings.keys()], androidUserID: android.userID, errors };
    } else if (url.pathname === '/messages' && req.method === 'GET') {
      result = { peers: await Promise.all([...peers].map(async ([key, page]) => ({ peer: key, messages: await page.evaluate(() => window.qaMessages.map(m => {
        let custom; try { custom = m.customElem?.data ? JSON.parse(m.customElem.data) : null; } catch { custom = null; }
        return { clientMsgID: m.clientMsgID, contentType: m.contentType, sendID: m.sendID, groupID: m.groupID, text: m.textElem?.content, customType: custom?.customType ?? null,
          meetingRoomID: custom?.customType === 905 && /^[a-f0-9]{32}$/.test(custom?.data?.id || '') ? custom.data.id : null };
      })) }))) };
    } else if (req.method !== 'POST') throw new Error('Unknown read operation');
    else if (url.pathname === '/close') { await writeJournal(); res.writeHead(200, { 'Content-Type': 'application/json' }); res.end('{"closed":true}'); server.close(); await browser.close(); return; }
    else if (url.pathname === '/create-group') {
      if (body.peer && body.peer !== '102') throw new Error('Only102 creates this fixture group');
      const group = await peers.get('102').evaluate(async members => (await window.qaIm.createGroup({ groupInfo: { groupType: 2, groupName: 'Android104 RTC collaboration ' + Date.now() }, memberUserIDs: members, adminUserIDs: [] })).data, [accounts['103'].userID, android.userID]);
      const fixture = { groupID: group.groupID, groupName: group.groupName, members: [accounts['102'].userID, accounts['103'].userID, android.userID] }; groups.set(group.groupID, fixture); result = fixture;
    } else if (url.pathname === '/announce-group') {
      const groupID = fixedGroup(body.groupID);
      result = await peers.get('102').evaluate(async groupID => { const { data: draft } = await window.qaIm.createTextMessage('Android104 RTC fixture ready: use this original group chat for the next test.'); const { data: m } = await window.qaIm.sendMessage({ recvID: '', groupID, message: draft }); return { groupID, clientMsgID: m.clientMsgID, seq: m.seq }; }, groupID);
    } else if (url.pathname === '/open-group') { await openGroup(getPeer(body), body.groupID); result = { opened: true, groupID: body.groupID }; }
    else if (url.pathname === '/open-android') { await openAndroid(getPeer(body)); result = { opened: true, userID: android.userID }; }
    else if (url.pathname === '/composer') {
      if (body.peer && body.peer !== '102') throw new Error('Composer controls only the independent Web102 peer');
      if (!['type', 'clear', 'blur'].includes(body.action)) throw new Error('Composer action must be type, clear or blur');
      const text = body.text ?? 'typing-probe Web102';
      if (body.action === 'type' && (typeof text !== 'string' || !/^typing-probe[ A-Za-z0-9._-]{0,80}$/.test(text))) throw new Error('Only a short synthetic typing-probe draft is allowed');
      const page = peers.get('102');
      const recipient = await page.evaluate(() => window.qaStore.useConversationStore.getState().currentConversation?.userID);
      if (recipient !== android.userID) throw new Error('Open the Android104 single conversation with /open-android first');
      if (await page.locator('.ant-modal:visible').count()) throw new Error('Finish or close the active dialog/call before using the composer');
      const input = page.locator('#editable-div');
      await input.waitFor({ state: 'visible' });
      await input.click();
      if (body.action === 'type') await input.pressSequentially(text, { delay: 90 });
      else if (body.action === 'clear') { await input.press('ControlOrMeta+A'); await input.press('Backspace'); }
      else await input.press('Tab');
      result = await input.evaluate((element, action) => ({
        action, originalComposerOperated: true, draftLength: element.textContent?.length ?? 0,
        focused: document.activeElement === element, sentMessage: false,
        stopBoundary: 'The frozen original Web UI does not send focus=false on clear/blur. Observe the actual SDK/UI typing expiry; do not count clearing as an immediate stop event.'
      }), body.action);
    }
    else if (url.pathname === '/accept') { await getPeer(body).getByText('接听', { exact: true }).click(); result = { accepted: true }; }
    else if (url.pathname === '/hangup') { await getPeer(body).getByText('挂断', { exact: true }).click(); result = { hungUp: true }; }
    else if (url.pathname === '/create-meeting') {
      if (body.peer && body.peer !== '102') throw new Error('Only102 creates this fixture meeting');
      const page = peers.get('102'); await menu(page);
      const response = page.waitForResponse(r => r.url().includes('signal_create_meeting') && r.request().method() === 'POST');
      await page.getByText('快速会议', { exact: true }).click();
      const payload = await (await response).json(); if (payload.errCode !== 0) throw new Error('Meeting creation failed');
      const roomID = fixedMeeting(payload.data.roomID); meetings.set(roomID, true);
      await page.locator('.meeting-modal').waitFor({ state: 'visible' }); result = { roomID, createdThroughOriginalUI: true };
    } else if (url.pathname === '/invite-meeting-card') {
      const roomID = fixedMeeting(body.roomID); if (!meetings.has(roomID)) throw new Error('Only102 owned fixture meetings can be shared');
      result = await peers.get('102').evaluate(async ({ roomID, receivers }) => {
        const { enterpriseRequest } = await import('/src/im/enterprise.ts');
        const info = (await enterpriseRequest('/signaling/get_signal_invitation_info', { roomID })).data.meetingInfo;
        const self = window.qaStore.useUserStore.getState().selfInfo; const sent = [];
        for (const recvID of receivers) {
          const { data: message } = await window.qaIm.createCustomMessage({ data: JSON.stringify({ customType: 905, data: { id: roomID, duration: info.endTime - info.startTime, inviterNickname: self.nickname, inviterUserID: self.userID, inviterFaceURL: self.faceURL || '', subject: info.meetingName, start: info.startTime } }), extension: '', description: '' });
          const { data } = await window.qaIm.sendMessage({ recvID, groupID: '', message });
          sent.push({ recvID, clientMsgID: data.clientMsgID, contentType: data.contentType });
        }
        return { roomID, sent, setup: 'Real SDK905 messages; ImFacade authorizes each recipient before send. Android joins by tapping its original UI card.' };
      }, { roomID, receivers: [accounts['103'].userID, android.userID] });
    } else if (url.pathname === '/join-meeting') {
      const page = getPeer(body), roomID = fixedMeeting(body.roomID); await menu(page);
      await page.locator('.meeting-manage-modal').getByText('进入会议', { exact: true }).first().click();
      await page.locator('#meetingNo').fill(roomID); await page.locator('.meeting-manage-modal').getByRole('button', { name: '进入会议', exact: true }).click();
      await page.locator('.meeting-modal').waitFor({ state: 'visible' }); result = { roomID, joinedThroughOriginalUI: true };
    } else if (url.pathname === '/join-meeting-card') {
      const page = getPeer(body);
      if (!/^[a-zA-Z0-9_-]{8,80}$/.test(body.clientMsgID || '')) throw new Error('Invalid synthetic message identity');
      const roomID = await page.evaluate(({ clientMsgID, androidUserID }) => {
        const message = window.qaMessages.find(m => m.clientMsgID === clientMsgID && m.sendID === androidUserID && m.contentType === 110);
        if (!message) throw new Error('Only a received Android104 fixture card can be opened');
        const custom = JSON.parse(message.customElem.data);
        if (custom.customType !== 905 || !/^[a-f0-9]{32}$/.test(custom.data?.id || '')) throw new Error('Message is not a meeting card');
        return custom.data.id;
      }, { clientMsgID: body.clientMsgID, androidUserID: android.userID });
      await openAndroid(page);
      await page.locator('[id="chat_' + body.clientMsgID + '"]').getByText('进入会议', { exact: true }).click();
      await page.locator('.meeting-modal').waitFor({ state: 'visible' }); result = { roomID, clientMsgID: body.clientMsgID, joinedThroughOriginalCard: true };
    } else if (url.pathname === '/meeting-control') {
      const labels = { cameraOn: '开启摄像头', cameraOff: '关闭摄像头', micOn: '开启麦克风', micOff: '关闭麦克风', share: '共享屏幕', stopShare: '结束共享', members: '成员' };
      const page = getPeer(body);
      if (labels[body.action]) await page.locator('.meeting-modal').getByText(labels[body.action], { exact: true }).click();
      else if (body.action === 'leave' || body.action === 'finish') {
        await page.locator('.meeting-modal').getByRole('button', { name: '结束会议', exact: true }).click();
        const popup = page.locator('.ant-popover:visible');
        if (await popup.isVisible()) await popup.getByRole('button', { name: body.action === 'finish' ? '结束会议' : '离开会议', exact: true }).click();
      } else throw new Error('Unsupported original meeting control');
      result = { clickedOriginalUI: true, action: body.action };
    } else if (url.pathname === '/meeting-setting') {
      const page = getPeer(body);
      const labels = { participantCanUnmuteSelf: '允许成员自我解除静音', participantCanEnableVideo: '允许成员开启视频', onlyHostShareScreen: '仅主持人可以共享屏幕' };
      if (!labels[body.setting] || typeof body.value !== 'boolean') throw new Error('Unsupported setting');
      await page.locator('.meeting-modal').getByText('设置', { exact: true }).click();
      const toggle = page.getByText(labels[body.setting], { exact: true }).locator('..').getByRole('switch');
      if ((await toggle.getAttribute('aria-checked')) !== String(body.value)) await toggle.click();
      await page.getByText('会议设置', { exact: true }).locator('..').locator('.anticon-close').click();
      result = { clickedOriginalUI: true, setting: body.setting, value: body.value };
    } else if (url.pathname === '/screenshot') {
      const peer = body.peer || '102'; await getPeer(body).screenshot({ path: resolve(out, 'peer' + peer + '.png') }); result = { path: 'test-results/android-collaboration/peer' + peer + '.png' };
    } else throw new Error('Unknown test command');
    if (req.method === 'POST') operations.push({ utc: new Date().toISOString(), action: url.pathname, peer: body.peer || '102', evidence: result });
    await writeJournal(); res.writeHead(200, { 'Content-Type': 'application/json' }); res.end(JSON.stringify({ ...result, errors }));
  } catch (error) { res.writeHead(400, { 'Content-Type': 'application/json' }); res.end(JSON.stringify({ error: sanitize(error.message), errors })); }
});
await new Promise(resolve => server.listen(17788, '127.0.0.1', resolve));
console.log('Synthetic Web102/103 ready; independent control127.0.0.1:17788. No group/message/call/meeting action executed. Web101 is untouched.');
process.on('SIGINT', async () => { server.close(); await writeJournal(); await browser.close(); process.exit(); });
