import test from "node:test";
import assert from "node:assert/strict";
import { build } from "esbuild";
import { createRequire } from "node:module";
import { mkdtemp, readFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
const requireSDK = createRequire(path.resolve("../../sdks/wasm/package.json"));
const initialize = requireSDK("@jlongster/sql.js");
const SQL = await initialize({ wasmBinary: await readFile("../../sdks/wasm/assets/sql-wasm.wasm") });
async function load(entry) {
  const result = await build({ entryPoints: [entry], bundle: true, write: false, platform: "node", format: "esm" });
  return import(`data:text/javascript;base64,${Buffer.from(result.outputFiles[0].text + "\n//# sourceURL=" + path.basename(entry)).toString("base64")}#${crypto.randomUUID()}`);
}
const { LocalVault } = await load("electron/main/localStore/vault.ts");
test("real fixed SQLite worker waits for durable host ACK, rolls back errors and replays exact SQL", async t => {
  const directory = await mkdtemp(path.join(tmpdir(), "puty-sdk-durable-")), master = Buffer.alloc(32, 18);
  let vault = await LocalVault.open(directory, "test_sdk_bridge", master, async () => {});
  const listeners = [], sent = []; let failCommit = false;
  const prior = globalThis.self;
  globalThis.self = {
    addEventListener: (_event, callback) => listeners.push(callback),
    postMessage: request => {
      sent.push(request.operation);
      Promise.resolve().then(async () => {
        if (request.operation === "sdk-restore") return vault.restoreSDK();
        if (request.operation === "sdk-commit") { if (failCommit) throw new Error("synthetic disk failure"); return vault.commit(request.data); }
        if (request.operation === "sdk-checkpoint") return vault.checkpoint(Buffer.from(request.data.snapshot, "base64"));
        if(request.operation === "archive-query")return vault.sdkQuery(request.data.name,request.data.args,request.data.workingRows);
        if (request.operation === "cleanup-commit") return vault.clean(request.data.scope, Buffer.from(request.data.snapshot, "base64"),request.data.workingMessages);
        throw new Error("Unknown test bridge operation");
      }).then(data => listeners.forEach(listener => listener({ data: { putyStore: "response", id: request.id, data } })), error => listeners.forEach(listener => listener({ data: { putyStore: "response", id: request.id, error: error.message } })));
    },
  };
  const bridge = await load("../../sdks/wasm/src/api/database/durable.ts");
  listeners.forEach(listener => listener({ data: { putyStore: "configure", enabled: true } }));
  let db;
  const table = "chat_logs_si_fixture";
  const init = bridge.durableMethod("initDB", async () => {
    const restored = await bridge.restoreMemoryDatabase();
    db = bridge.attachMemoryDatabase(new SQL.Database(restored.bytes), restored.entries, restored.tombstones);
    db.exec(`CREATE TABLE IF NOT EXISTS '${table}' (client_msg_id TEXT PRIMARY KEY, send_time INTEGER, status INTEGER, content TEXT, local_ex TEXT,seq INTEGER)`);
    return { errCode: 0, data: "{}" };
  });
  const insert = bridge.durableMethod("insertMessage", async (id, time = Date.parse("2026-11-04T01:00:00Z")) => {
    const statement = db.prepare(`INSERT INTO '${table}' VALUES(?,?,?,?,?,?)`);
    try { statement.bind([id, time, 2, "SYNTHETIC BEFORE; BEGIN; COMMIT; AFTER", "", 7]); statement.step(); } finally { statement.free(); }
    return { errCode: 0, data: "{}" };
  });
  const count = () => db.exec(`SELECT COUNT(*) FROM '${table}'`)[0].values[0][0];
  try {
    assert.equal((await init()).errCode, 0);
    assert.equal((await insert("one")).errCode, 0); assert.equal(count(), 1);
    assert.equal((await vault.history("si_fixture"))[0].client_msg_id, "one");
    assert.ok(sent.includes("sdk-commit"));
    failCommit = true; const rejected = await insert("two"); assert.equal(rejected.errCode, 10001); assert.equal(count(), 1); failCommit = false;
    const partial = bridge.durableMethod("failureResponse", async () => { db.exec(`INSERT INTO '${table}' VALUES('partial',1,2,'must rollback','',8)`); return { errCode: 10001, data: "{}" }; });
    assert.equal((await partial()).errCode, 10001); assert.equal(count(), 1);
    const nested = bridge.durableMethod("nestedTransaction", async () => { db.exec("BEGIN"); db.exec(`UPDATE '${table}' SET content='BEGIN; COMMIT; still content' WHERE client_msg_id='one'`); db.exec("COMMIT"); return { errCode: 0 }; });
    assert.equal((await nested()).errCode, 0);
    assert.equal(db.exec(`SELECT content FROM '${table}'`)[0].values[0][0], "BEGIN; COMMIT; still content");
    db.close(); bridge.detachMemoryDatabase(); vault.lock(); vault = await LocalVault.open(directory, "test_sdk_bridge", master, async () => {});
    assert.equal((await init()).errCode, 0); assert.equal(count(), 1);
    assert.equal(db.exec(`SELECT content FROM '${table}'`)[0].values[0][0], "BEGIN; COMMIT; still content");
    const clean = bridge.durableMethod("putyLocalCleanup", bridge.localCleanup);
    await clean({ conversationIDs: ["si_fixture"], beforeMS: Date.parse("2026-11-05T00:00:00+08:00") });
    assert.equal((await vault.history("si_fixture")).length, 0);
    assert.equal(db.exec(`SELECT content,status,seq FROM '${table}'`)[0].values[0][0], "");
    assert.equal(db.exec(`SELECT content,status,seq FROM '${table}'`)[0].values[0][1], 4);
    assert.equal(db.exec(`SELECT content,status,seq FROM '${table}'`)[0].values[0][2], 7);
    assert.equal((await insert("late-old-message")).errCode, 0);
    assert.equal((await vault.history("si_fixture")).length, 0);
    assert.equal(db.exec(`SELECT content,status FROM '${table}' WHERE client_msg_id='late-old-message'`)[0].values[0][0], "");
  } finally { try { db?.close(); } catch {} bridge.detachMemoryDatabase(); vault.lock(); globalThis.self = prior; await rm(directory, { recursive: true, force: true }); }
});

test("90-day working payload pruning keeps unread/unsent and cold paged reads survive metadata updates and recall",async t=>{
  const directory=await mkdtemp(path.join(tmpdir(),"puty-sdk-cold-")),master=Buffer.alloc(32,19);const vault=await LocalVault.open(directory,"test_cold_bridge",master,async()=>{}),prior=globalThis.self,listeners=[];
  globalThis.self={addEventListener:(_,callback)=>listeners.push(callback),postMessage:request=>Promise.resolve().then(async()=>{
    if(request.operation==='sdk-restore')return vault.restoreSDK();
    if(request.operation==='sdk-commit')return vault.commit(request.data);
    if(request.operation==='sdk-checkpoint')return vault.checkpoint(Buffer.from(request.data.snapshot,'base64'));
    if(request.operation==='archive-query')return vault.sdkQuery(request.data.name,request.data.args,request.data.workingRows);
    if(request.operation==='cleanup-commit')return vault.clean(request.data.scope,Buffer.from(request.data.snapshot,'base64'),request.data.workingMessages);
    if(request.operation==='cleanup-preview')return vault.previewCleanup(request.data.scope,request.data.workingMessages);
    throw new Error('Unknown fixture bridge operation');
  }).then(data=>listeners.forEach(cb=>cb({data:{putyStore:'response',id:request.id,data}})),error=>listeners.forEach(cb=>cb({data:{putyStore:'response',id:request.id,error:error.message}})))};
  const bridge=await load('../../sdks/wasm/src/api/database/durable.ts'),schema=await load('../../sdks/wasm/src/sqls/localChatLogsConversationID.ts');listeners.forEach(cb=>cb({data:{putyStore:'configure',enabled:true}}));let db;
  const init=bridge.durableMethod('initDB',async()=>{const restored=await bridge.restoreMemoryDatabase();db=bridge.attachMemoryDatabase(new SQL.Database(restored.bytes),restored.entries,restored.tombstones);db.exec("CREATE TABLE IF NOT EXISTS local_conversations(conversation_id TEXT PRIMARY KEY,latest_msg TEXT,unread_count INTEGER,is_msg_destruct INTEGER DEFAULT 0,msg_destruct_time INTEGER DEFAULT 604800)");schema.localChatLogsConversationID(db,'si_cold');return {errCode:0};});
  const old=Date.now()-120*86400000;
  const insert=bridge.durableMethod('insertMessage',async(id,read=1,status=2,burn=false)=>{db.run("INSERT INTO chat_logs_si_cold(client_msg_id,send_time,status,content,local_ex,seq,is_read,attached_info,content_type)VALUES(?,?,?,?,?,?,?,?,?)",[id,old,status,'SYNTHETIC_COLD_PRIVATE '+id,'',status===1?0:9,read,burn?JSON.stringify({isPrivateChat:true,burnDuration:30}):'',101]);return {errCode:0};});
  const read=bridge.durableMethod('getMessageList',()=>{throw new Error('Cold reads must use monthly archive, not thin working rows');});
  try{
    assert.equal((await init()).errCode,0);assert.equal((await insert('read')).errCode,0);assert.equal(db.exec("SELECT content,seq,status FROM chat_logs_si_cold WHERE client_msg_id='read'")[0].values[0][0],'');
    assert.equal((await insert('unread',0)).errCode,0);assert.equal((await insert('unsent',0,1)).errCode,0);assert.equal((await insert('burn',0,2,true)).errCode,0);
    assert.equal(db.exec("SELECT content FROM chat_logs_si_cold WHERE client_msg_id='unread'")[0].values[0][0],'SYNTHETIC_COLD_PRIVATE unread');assert.equal((await vault.history('si_cold')).some(row=>row.client_msg_id==='burn'),false);
    const page=await read('si_cold',20,0,0,'',false),messages=JSON.parse(page.data);assert.equal(page.errCode,0);assert.equal(messages.length,4);assert.ok(messages.find(row=>row.clientMsgID==='read').content.includes('read'));assert.ok(messages.find(row=>row.clientMsgID==='burn').content.includes('burn'));
    const change=bridge.durableMethod('markConversationMessageAsRead',async()=>{db.run("UPDATE chat_logs_si_cold SET is_read=1 WHERE client_msg_id=?",['read']);db.run("UPDATE chat_logs_si_cold SET sender_nick_name=? WHERE client_msg_id=?",['new name','read']);return {errCode:0};});assert.equal((await change()).errCode,0);
    assert.equal((await vault.sdkQuery('getMessage',['si_cold','read']))[0].content,'SYNTHETIC_COLD_PRIVATE read');assert.equal((await vault.sdkQuery('getMessage',['si_cold','read']))[0].sender_nick_name,'new name');
    const recall=bridge.durableMethod('updateMessage',async()=>{db.run("UPDATE chat_logs_si_cold SET content_type=2101,content='recall' WHERE client_msg_id=?",['read']);return {errCode:0};});assert.equal((await recall()).errCode,0);assert.equal((await vault.sdkQuery('getMessage',['si_cold','read'])).length,0);
    assert.equal((await insert('cold-to-volatile')).errCode,0);assert.equal(db.exec("SELECT content FROM chat_logs_si_cold WHERE client_msg_id='cold-to-volatile'")[0].values[0][0],'');
    const destruct=bridge.durableMethod('updateConversation',async()=>{db.run("INSERT INTO local_conversations(conversation_id,is_msg_destruct)VALUES(?,1)",['si_cold']);return {errCode:0};});
    assert.equal((await destruct()).errCode,0);assert.equal((await vault.history('si_cold')).length,0);
    assert.equal(db.exec("SELECT content FROM chat_logs_si_cold WHERE client_msg_id='cold-to-volatile'")[0].values[0][0],'SYNTHETIC_COLD_PRIVATE cold-to-volatile');
    const volatilePage=JSON.parse((await read('si_cold',20,0,0,'',false)).data);assert.ok(volatilePage.some(row=>row.clientMsgID==='cold-to-volatile'));
    const privateFile=await vault.reserveAttachment({taskID:'private-working-only',name:'阅后附件.txt',messageRefs:[{conversationID:'si_cold',clientMsgID:'burn'}]});await vault.writeAttachment('private-working-only',(async function*(){yield Buffer.from('SYNTHETIC PRIVATE ATTACHMENT');})());
    const cleanupPreview=await bridge.durableMethod('putyLocalCleanupPreview',bridge.localCleanupPreview)({conversationIDs:['si_cold'],beforeMS:Date.now()+1});assert.equal(cleanupPreview.messages,4);assert.equal(db.exec("SELECT status FROM chat_logs_si_cold WHERE client_msg_id='read'")[0].values[0][0],4);assert.equal(cleanupPreview.attachments,1);assert.ok(cleanupPreview.bytes>0);
    const clean=bridge.durableMethod('putyLocalCleanup',bridge.localCleanup);await clean({conversationIDs:['si_cold'],beforeMS:Date.now()+1});
    await assert.rejects(readFile(path.join(directory,privateFile.file)));assert.equal(vault.attachmentForPath(path.join(directory,privateFile.file)),null);
    const lateConversation=bridge.durableMethod('updateConversation',async()=>{db.run("UPDATE local_conversations SET latest_msg=?,unread_count=1 WHERE conversation_id='si_cold'",[JSON.stringify({clientMsgID:'late',sendTime:old,content:'SYNTHETIC MUST HIDE'})]);return {errCode:0};});assert.equal((await lateConversation()).errCode,0);
    assert.deepEqual(db.exec("SELECT latest_msg,unread_count FROM local_conversations WHERE conversation_id='si_cold'")[0].values[0],['',0]);
  }finally{try{db?.close();}catch{}bridge.detachMemoryDatabase();vault.lock();globalThis.self=prior;await rm(directory,{recursive:true,force:true});}
});
