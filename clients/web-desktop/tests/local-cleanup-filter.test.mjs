import test from 'node:test';
import assert from 'node:assert/strict';
import {build} from 'esbuild';
const output=await build({entryPoints:['src/im/local-cleanup-filter.ts'],bundle:true,write:false,platform:'node',format:'esm'});
const {sanitizeLocalPayload}=await import('data:text/javascript;base64,'+Buffer.from(output.outputFiles[0].text).toString('base64'));
test('original SDK live callbacks and conversation latest messages cannot resurrect local deleted content',()=>{
  const rules=[{conversationIDs:['si_emp_a_emp_b'],beforeMS:Date.parse('2026-11-01T00:00:00+08:00')}],old={clientMsgID:'old',sendID:'emp_b',recvID:'emp_a',sessionType:1,sendTime:Date.parse('2026-10-31T23:59:59+08:00'),content:'SYNTHETIC DELETED'},fresh={...old,clientMsgID:'fresh',sendTime:Date.parse('2026-11-01T00:00:00+08:00')};
  assert.deepEqual(sanitizeLocalPayload([old,fresh],rules),[fresh]);
  const conversation={conversationID:'si_emp_a_emp_b',latestMsg:JSON.stringify(old),unreadCount:1};assert.equal(sanitizeLocalPayload(conversation,rules).latestMsg,'');assert.equal(sanitizeLocalPayload(conversation,rules).unreadCount,0);
  assert.deepEqual(sanitizeLocalPayload({messageList:[old,fresh]},rules,'si_emp_a_emp_b').messageList,[fresh]);
  assert.equal(old.content,'SYNTHETIC DELETED');assert.equal(conversation.unreadCount,1);
});

test('exact message deletion leaves all other timestamps, conversations and active private messages intact',()=>{
  const rules=[{conversationIDs:['si_exact'],beforeMS:Number.MAX_SAFE_INTEGER,messageIDs:['deleted']}],deleted={conversationID:'si_exact',clientMsgID:'deleted',sendTime:Number.MAX_SAFE_INTEGER,content:'DELETED'},neighbour={...deleted,clientMsgID:'neighbour'},other={...deleted,conversationID:'si_other'},privateLive={...deleted,clientMsgID:'private',attachedInfo:{isPrivateChat:true}};
  assert.deepEqual(sanitizeLocalPayload([deleted,neighbour,other,privateLive],rules),[neighbour,other,privateLive]);
  const latest=sanitizeLocalPayload({conversationID:'si_exact',latest_msg:JSON.stringify(deleted),unread_count:1},rules);assert.equal(latest.latest_msg,'');assert.equal(latest.unread_count,0);
});

test('fresh native policy is read for each callback and old profile flights cannot publish',async()=>{
  const prior=globalThis.window,listeners=new Map(),requests=[];
  globalThis.window={electronAPI:{ipcInvoke:()=>new Promise(resolve=>requests.push(resolve))},addEventListener:(name,fn)=>listeners.set(name,fn)};
  try{
    const current=await import('data:text/javascript;base64,'+Buffer.from(output.outputFiles[0].text).toString('base64')+'#'+crypto.randomUUID()),message={conversationID:'si_exact',clientMsgID:'deleted',sendTime:1,content:'OLD CALLBACK'};
    const old=current.filterNativePayload([message]),fresh=current.filterNativePayload([message]);assert.equal(requests.length,2);
    requests[1]([{conversationIDs:['si_exact'],beforeMS:Number.MAX_SAFE_INTEGER,messageIDs:['deleted']}]);assert.deepEqual(await fresh,[]);requests[0]([]);assert.deepEqual(await old,[message]);
    const stale=current.filterNativePayload([message]);listeners.get('im-profile-changing')();requests[2]([]);await assert.rejects(stale,/规则已改变/);
  }finally{globalThis.window=prior;}
});
