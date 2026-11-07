import test from "node:test";
import assert from "node:assert/strict";
import { createServer } from "node:http";
import { createHash } from "node:crypto";
import { readFile, writeFile, mkdtemp, access } from "node:fs/promises";
import { existsSync } from "node:fs";
import { join, resolve, extname } from "node:path";
import { tmpdir } from "node:os";
import { spawn } from "node:child_process";
const root = resolve(import.meta.dirname, ".."),
  base = "/plugins/biz-suite/admin/",
  api = "/plugins/biz-suite/api/admin";
const delay = (ms) => new Promise((r) => setTimeout(r, ms));
class CDP {
  constructor(socket) {
    this.socket = socket;
    this.id = 0;
    this.pending = new Map();
    this.errors = [];
    socket.addEventListener("message", (e) => {
      const msg = JSON.parse(e.data);
      if (msg.id) {
        const p = this.pending.get(msg.id);
        if (p) {
          this.pending.delete(msg.id);
          msg.error
            ? p.reject(new Error(msg.error.message))
            : p.resolve(msg.result);
        }
      }
      if (msg.method === "Runtime.exceptionThrown")
        this.errors.push(msg.params.exceptionDetails);
    });
  }
  send(method, params = {}) {
    return new Promise((resolve, reject) => {
      const id = ++this.id;
      this.pending.set(id, { resolve, reject });
      this.socket.send(JSON.stringify({ id, method, params }));
    });
  }
  async eval(expression, userGesture = false) {
    const r = await this.send("Runtime.evaluate", {
      expression,
      userGesture,
      awaitPromise: true,
      returnByValue: true,
    });
    if (r.exceptionDetails)
      throw new Error(
        r.exceptionDetails.text +
          " " +
          r.exceptionDetails.exception?.description
      );
    return r.result.value;
  }
  async wait(expression) {
    for (let i = 0; i < 100; i++) {
      try {
        if (await this.eval(expression)) return;
      } catch {}
      await delay(60);
    }
    throw new Error(
      "Timeout: " +
        expression +
        "\n" +
        (await this.eval("document.body.innerText"))
    );
  }
  async go(url) {
    await this.send("Page.navigate", { url });
    await this.wait("!!document.querySelector('#view')");
  }
}

test(
  "business SPA works in real Chromium with HRM, WeCom, IM and permission contracts",
  { timeout: 120000 },
  async (t) => {
    const recorded = [],
      employees = [
        {
          id: "e1",
          name: '<img src=x onerror="window.xss=1">',
          employee_no: "001",
          wecom_userid: "wx1",
          user_id: 1,
          dept_id: 1,
          host_username: "001",
          host_display_name: "管理员姓名",
          host_department_name: "技术部",
          host_department_path: "本地总公司 / 技术部",
          department_names: ["研发中心"],
          mapping_status: "mapped",
          mapping_message: "账号、部门已关联",
          position: "工程师",
          status: "active",
        },
      ];
    let permissions = [
      "biz-suite.hrm.manage",
      "biz-suite.im.manage",
      "biz-suite.wecom.manage",
    ];
    let authFail = false;
    let clockFailure = false;
    let summaryDelay = 0;
    let hrmOptionsDelay = 0;
    let approvalDetailFailure = false;
    let imSessionStatus = 200;
    let imSessionCode = 0;
    let imSessionUid = 1;
    let organizationReport = null;
    let organizationFailure = false;
    let organizationReadFailure = false;
    let pruneReadFailure = false;
    let pruneWriteFailure = false;
    const pruneReport = { employees_filtered: 35, accounts_removed: 3, departments_removed: 10, already_filtered: 2,
      conflicts: [{ entity_type: "account", name: "人工账号", employee_no: "H001", department_path: "总公司 / 分子公司", reason: "保留人工绑定账号 <img src=x onerror=window.pruneXss=1>" }] };
    const organizationDepartments = [{ wecom_id: 1, name: "研发中心", path: "总公司 / 研发中心", parent_name: "总公司", host_dept_id: 1,
      host_department_name: "技术部", host_department_path: "本地总公司 / 技术部", mapping_status: "mapped", mapping_message: "本地部门已关联", synced_at: "2026-10-29 10:00:00" }];
    const approvalRecords = Object.fromEntries(["leaves", "outs", "trips"].map((kind) => [kind, [
      { id: "wecom-" + kind, employee_id: "private-employee-hash", employee_name: "段详政", employee_no: "HR001", department_names: ["研发部"],
        leave_type: "private-option-id", leave_type_name: "年假", destination: "广州客户现场", start_time: "2026-10-29 09:00:00", end_time: "2026-10-29 18:00:00", duration: 8,
        reason: "处理个人事务\n第二行说明 <img src=x onerror=window.approvalXss=1>", approval_status: "approved", source: "wecom", external_ref: "202610290001" },
      { id: "manual-" + kind, employee_id: "e1", employee_name: "手工登记员工", employee_no: "001", department_names: ["技术部"],
        leave_type: "事假", leave_type_name: "事假", destination: "本地业务点", start_time: "2026-10-30 09:00:00", end_time: "2026-10-30 10:00:00", duration: 1,
        reason: "手工登记事由", approval_status: "pending", source: "manual", external_ref: "" },
    ]]));
    approvalRecords.leaves.push({ id: "unmatched-leave", employee_id: "unmatched-employee-secret", employee_name: "", leave_type: "87654321", leave_type_name: "", reason: "待补充资料", source: "wecom", approval_status: "pending" });
    const clockEmployees = [
      { id: "employee-a", name: "段详政", employee_no: "A001", department_names: ["研发部"], department_keys: ["wecom:2"] },
      { id: "employee-b", name: "段详政", employee_no: "B002", department_names: ["业务部"], department_keys: ["wecom:4"] },
    ];
    const clockDepartments = [
      { key: "wecom:1", name: "总公司", path: "总公司", source: "wecom", id: 1, parent_id: 0 },
      { key: "wecom:2", name: "研发部", path: "总公司 / 研发部", source: "wecom", id: 2, parent_id: 1 },
      { key: "wecom:3", name: "分子公司", path: "总公司 / 分子公司", source: "wecom", id: 3, parent_id: 1 },
      { key: "wecom:4", name: "业务部", path: "总公司 / 分子公司 / 业务部", source: "wecom", id: 4, parent_id: 3 },
      { key: "host:2", name: "技术部", path: "本地总公司 / 技术部", source: "host", id: 2, parent_id: 1 },
    ];
    const clockRecords = Array.from({ length: 55 }, (_, i) => ({
      id: `clock-${i}`, employee_id: "employee-b", employee_name: "段详政", employee_no: "B002",
      department_names: ["业务部"], clock_time: `2026-10-29 ${i % 2 ? "18:00:00" : "09:00:00"}`,
      clock_type: i % 2 ? "out" : "in", status: ["normal", "late", "early", "missing", "abnormal"][i] || "normal",
      source: i === 0 || i >= 50 ? "manual" : "wecom", address: "深圳园区",
    }));
    clockRecords.push({ id: "unmatched-clock", employee_id: "unmatched-internal-hash", employee_name: "",
      department_names: [], clock_time: "2026-09-30 09:00:00", clock_type: "in", status: "normal", source: "wecom" });
    const savedCredentials = { contact: false, app: false };
    let wecomSettings = {
      corp_id: "demo",
      agent_id: "100",
      include_department_ids: [],
      exclude_department_ids: [],
      include_department_names: [],
      exclude_department_names: [],
      include_userids: [],
      exclude_userids: [],
      include_user_names: [],
      exclude_user_names: [],
      include_subdepartments: true,
    };
    let filterChoices = {
      departments: [
        { wecom_id: 1, name: "总公司", parent_id: 0 },
        { wecom_id: 2, name: "研发部", parent_id: 1 },
        { wecom_id: 3, name: "分子公司", parent_id: 1 },
        { wecom_id: 4, name: "业务部", parent_id: 3 },
        { wecom_id: 5, name: "新入职员工", parent_id: 1 },
      ],
      users: [
        { wecom_userid: "alice", name: "Alice", department_ids: "[2]", department_names: ["研发部"] },
        { wecom_userid: "duan1", name: "段详政", department_ids: "[2]", department_names: ["研发部"] },
        { wecom_userid: "duan2", name: "段详政", department_ids: "[4]", department_names: ["业务部"] },
      ],
    };
    const messages = [
      {
        id: "m1",
        seq: 1,
        change_seq: 1,
        sender_id: 2,
        sender_name: "李四",
        sender_employee_no: "002",
        target_id: 1,
        content: "你好",
        message_type: "text",
        send_time: "2026-10-28 09:00:00",
        status: "sent",
      },
    ];
    const directoryDepartments = [
      {id:1,parent_id:0,name:'总公司',path:'总公司'}, {id:2,parent_id:1,name:'研发部',path:'总公司 / 研发部'},
      {id:3,parent_id:2,name:'研发一部',path:'总公司 / 研发部 / 研发一部'}, {id:4,parent_id:1,name:'业务部',path:'总公司 / 业务部'}
    ];
    const directoryUsers = [
      {id:2,employee_id:'im-2',name:'李四',employee_no:'002',department_id:2,department_path:'总公司 / 研发部',can_chat:true},
      {id:3,employee_id:'im-3',name:'李四',employee_no:'003',department_id:3,department_path:'总公司 / 研发部 / 研发一部',can_chat:true},
      {id:4,employee_id:'im-4',name:'停用同事',employee_no:'004',department_id:4,can_chat:false,unavailable_reason:'对方账号已停用'},
      {id:5,employee_id:'im-5',name:'无权限同事',employee_no:'005',department_id:4,can_chat:false,unavailable_reason:'对方未获企业即时通讯权限'},
      {id:null,employee_id:'im-unbound',name:'未关联员工',employee_no:'006',department_id:4,can_chat:false,unavailable_reason:'尚未关联本地账号'},
      {id:7,employee_id:'im-unassigned',name:'未分部门同事',employee_no:'007',department_id:0,can_chat:true},
      ...Array.from({length:21},(_,i)=>({id:i+10,employee_id:'im-'+(i+10),name:'企业同事'+i,employee_no:'E'+i,department_id:4,can_chat:true}))
    ];
    const contactSettings = new Map();
    const directoryPerson = row => ({...row,account_status:row.id===null?'unlinked':row.id===4?'disabled':row.id===5?'no_permission':'active',department_path:row.department_path||directoryDepartments.find(dep=>dep.id===row.department_id)?.path||'',...contactSettings.get(row.id),can_chat:row.can_chat&&!contactSettings.get(row.id)?.blocked_by_me,
      unavailable_reason:contactSettings.get(row.id)?.blocked_by_me?'你已屏蔽该成员':row.unavailable_reason||''});
    let conversations = [{conversation_type:'private',conversation_id:'2',name:'李四',employee_no:'002',last_content:'你好',last_seq:1,unread_count:1}];
    let directoryDelay = 0, directoryFailure = false;
    let rejectGroupCreation = false, createdGroupCount = 0;
    let nextSeq = 2;
    let templates = [];
    let automaticSchedule = {timezone:'Asia/Shanghai',times:['08:30','10:00','14:30','23:59'],next_run_at:'2026-10-29 23:59:00',startup_sync:false,catch_up:false};
    const attendanceRules = [], attendanceMonths = [], attendanceLedger = [], attendanceMappings = {};
    let attendanceSaveFailure = false;
    let monthlyBatchStates = [], monthlyCloseError = "";
    const attendanceBatch = { id: "batch-oct", start_date: "2026-10-01", end_date: "2026-10-31", status: "completed", completed_count: 31, total_count: 31, created_at: "2026-10-29 12:00:00" };
    const attendanceResult = { id: "result-day", batch_id: "batch-oct", employee_id: "e1", employee: { id: "e1", name: "胡起伟", employee_no: "1599", department: "市场部" }, work_date: "2026-10-01", status: "calculated", result: { employee_id: "e1", date: "2026-10-01", state: "calculated", expected_seconds: 28800, credited_seconds: 28800, actual_seconds: 33300, overtime_seconds: 7200, leave_seconds: 3600, segments: [{id:"segment-1",kind:"work",state:"calculated",expected_seconds:28800,actual_seconds:25200,credited_seconds:28800,overtime_seconds:0,leave_seconds:3600},{id:"segment-2",kind:"overtime",state:"calculated",expected_seconds:0,actual_seconds:8100,credited_seconds:0,overtime_seconds:7200}],trace:[{message:"按已确认制度核定",evidence_ids:["clock-1"]}],issues:[] } };
    const attendancePreview = { kind: "baseline", filename: "9月考勤数据.xlsx", month: "2026-09", issues: [{sheet:"9月职员",cell:"C6",message:"工号缺失，需人工关联"}], employees: [{name:"胡起伟",employee_no:"1599",department:"市场部",source_key:"9月职员!B43",days:[{date:"2026-09-01",cells:[{cell:"E43",value:"√",comment:"人工核定依据 <img src=x onerror=window.attXss=1>",formula:"SUM(A1:A2)"}]}]}],rules:[] };
    const peers = new Set();
    function push(packet) {
      const payload = Buffer.from(JSON.stringify(packet));
      if (payload.length < 126) {
        const frame = Buffer.concat([
          Buffer.from([129, payload.length]),
          payload,
        ]);
        for (const peer of peers) peer.write(frame);
      }
    }
    const server = createServer(async (req, res) => {
      try {
        const url = new URL(req.url, "http://localhost");
        if (url.pathname.startsWith(api)) {
          const path = url.pathname.slice(api.length);
          if (authFail) {
            res.writeHead(401).end();
            return;
          }
          let raw = "";
          for await (const chunk of req) raw += chunk;
          let body = {};
          if (raw && req.headers["content-type"]?.includes("json"))
            body = JSON.parse(raw);
          recorded.push({
            method: req.method,
            path,
            body,
            query: url.search,
            headers: req.headers,
          });
          let data = [];
          if (path === "/access") data = { uid: 1, permissions, menus: [] };
          else if (path.startsWith("/hrm/attendance/v2/")) {
            const local = path.slice("/hrm/attendance/v2".length);
            if (local === "/rules") {
              if (req.method === "POST") { await delay(80); if (attendanceSaveFailure) { res.writeHead(503, {"Content-Type":"application/json"}).end(JSON.stringify({message:"保存暂时失败，请重试"}));return; } const value={...body,id:body.id||"rule-"+(attendanceRules.length+1),version:1,status:"draft"}; const index=attendanceRules.findIndex(r=>r.id===value.id); if(index<0)attendanceRules.push(value);else attendanceRules[index]=value;data=value; }
              else data={items:attendanceRules};
            } else if (/^\/rules\/.+\/preview$/.test(local)) { data={rule_id:local.split('/')[2],...body,employee_count:3,day_count:31,locked_months:[],manual_roster_count:0,issues:[],can_apply:true}; }
            else if (/^\/rules\/.+\/apply$/.test(local)) {const row=attendanceRules.find(r=>r.id===local.split('/')[2]);row.status="published";data={rule:row,batch_ids:[],status:"queued"};}
            else if (/^\/rules\/.+\/publish$/.test(local)) { const row=attendanceRules.find(r=>r.id===local.split('/')[2]);row.status="published";data=row; }
            else if (local==="/rosters/generate") { await delay(80);data={items:[{id:"roster-1",employee_id:"e1",work_date:body.start_date,rule_id:attendanceRules[0]?.id,shift_id:attendanceRules[0]?.config.shifts[0]?.id,category:"workday",status:"confirmed"}],issues:[],saved:!body.preview}; }
            else if (local==="/rosters") data={items:[]};
            else if (local==="/rosters/candidates") data={items:[{rule_id:attendanceRules[0]?.id,rule_name:attendanceRules[0]?.name,shift_id:attendanceRules[0]?.config.shifts[0]?.id,shift_name:"候选白班",score:100,preview:attendanceResult.result}]};
            else if (local==="/batches") { if(req.method==="POST") await delay(80); data=req.method==="POST"?{id:"new-batch",status:"queued"}:{items:[attendanceBatch]}; }
            else if (local.startsWith("/batches/")) { const override=local==="/batches/new-batch"?(monthlyBatchStates.length>1?monthlyBatchStates.shift():monthlyBatchStates[0]):null;data={...attendanceBatch,id:local.split('/').at(-1),...override}; }
            else if (local==="/results") data={items:[attendanceResult]};
            else if (local.startsWith("/results/")) data={...attendanceResult,evidence:{punches:[]}};
            else if (local==="/adjustments") data=body;
            else if (local==="/months") data={items:attendanceMonths};
            else if (/^\/months\/.+\/close$/.test(local)) {if(monthlyCloseError){res.writeHead(409,{"Content-Type":"application/json"}).end(JSON.stringify({message:monthlyCloseError}));return;} const month=local.split('/')[2],entry={month,batch_id:body.batch_id,status:"locked",revision:1};const index=attendanceMonths.findIndex(r=>r.month===month);if(index<0)attendanceMonths.push(entry);else attendanceMonths[index]=entry;data=entry;}
            else if (/^\/months\/.+\/action$/.test(local)) { const month=local.split('/')[2],entry={month,batch_id:body.batch_id,status:{review:"reviewed",publish:"published",lock:"locked",reopen:"draft"}[body.action],revision:1};const index=attendanceMonths.findIndex(r=>r.month===month);if(index<0)attendanceMonths.push(entry);else attendanceMonths[index]=entry;data=entry; }
            else if (local==="/ledger") { if(req.method==="POST")attendanceLedger.push({...body,id:"entry-1"});data={items:attendanceLedger,balance_minutes:attendanceLedger.reduce((n,x)=>n+Number(x.minutes),0)}; }
            else if(local==="/imports")data=req.method==="POST"?{id:"import-1",preview:attendancePreview,mappings:attendanceMappings}:{items:[{id:"import-1",filename:attendancePreview.filename,kind:"baseline",month:"2026-09"}]};
            else if(local==="/imports/import-1")data={id:"import-1",preview:attendancePreview,mappings:attendanceMappings};
            else if(local==="/imports/import-1/mappings"){Object.assign(attendanceMappings,body.mappings);data={id:"import-1",mappings:attendanceMappings};}
            else if(local==="/imports/import-1/compare")data={items:[{name:"胡起伟",employee_no:"1599",status:"unverifiable",expected_hours:192,actual_hours:8,issues:["没有完整月份的计算结果"],source_key:"9月职员!B43"}]};
            else if(local==="/imports/wecom")data={count:22,ids:["draft-snapshot"]};
            else data={items:[]};
          }
          else if (path === "/im/session") {
            if (imSessionStatus !== 200 || imSessionCode) {
              res.writeHead(imSessionStatus, { "Content-Type": "application/json" }).end(JSON.stringify({ code: imSessionCode || imSessionStatus, message: "会话校验暂不可用" }));
              return;
            }
            data = { uid: imSessionUid, permissions };
          }
          else if (path === "/hrm/summary") {
            if (summaryDelay) await delay(summaryDelay);
            data = { employees: 1, clocks: 2, leaves: 0, outs: 0 };
          }
          else if (
            path === "/hrm/options" ||
            path === "/wecom/departments/options"
          ) {
            if (path === "/hrm/options" && hrmOptionsDelay) await delay(hrmOptionsDelay);
            data = {
              users: [{ id: 1, name: "管理员姓名", username: "001", status: 0 }, { id: 2, name: "停用员工", username: "002", status: 1 }],
              departments: [{ id: 10, name: "本地总公司", pid: 0 }, { id: 1, name: "技术部", pid: 10 }],
              employees,
            };
          } else if (path === "/hrm/employees") {
            if (req.method === "POST")
              employees.push({ id: "e" + employees.length, ...body });
            const filtered = employees.filter((employee) =>
              employee.name.includes(url.searchParams.get("search") || "")
            );
            const page = Math.max(1, Number(url.searchParams.get("page") || 1));
            const pageSize = Math.max(
              1,
              Number(url.searchParams.get("page_size") || 50)
            );
            res.setHeader("X-Total-Count", filtered.length);
            data = filtered.slice((page - 1) * pageSize, page * pageSize);
          } else if (path === "/hrm/attendance/clocks/options") {
            data = {
              employees: clockEmployees,
              departments: clockDepartments,
            };
          } else if (path === "/hrm/attendance/clocks") {
            if (clockFailure) { res.writeHead(503, { "Content-Type": "application/json" }).end(JSON.stringify({ message: "打卡查询暂时失败" })); return; }
            if (req.method === "POST") {
              const index = clockRecords.findIndex((row) => row.id === body.id);
              if (index >= 0) clockRecords[index] = { ...clockRecords[index], ...body };
              else clockRecords.push({ id: "new-clock", ...body });
            }
            const q = url.searchParams;
            const filtered = clockRecords.filter((row) =>
              (!q.get("employee_id") || row.employee_id === q.get("employee_id")) &&
              (!q.get("status") || row.status === q.get("status")) &&
              (!q.get("employee_search") || [row.employee_name, row.employee_no].some((value) => value?.includes(q.get("employee_search")))) &&
              (!q.get("department") || (row.employee_id === "employee-b" &&
                (q.get("department") === "wecom:4" ||
                  (q.get("include_subdepartments") !== "false" && ["wecom:1", "wecom:3"].includes(q.get("department")))))) &&
              (!q.get("date") || row.clock_time.startsWith(q.get("date"))) &&
              (!q.get("month") || row.clock_time.startsWith(q.get("month"))) &&
              (!q.get("start_time") || row.clock_time.slice(0, 10) >= q.get("start_time")) &&
              (!q.get("end_time") || row.clock_time.slice(0, 10) <= q.get("end_time")) &&
              (!q.get("search") || [row.employee_name, row.employee_no, row.address].some((value) => value?.includes(q.get("search"))))
            );
            const page = Math.max(1, Number(q.get("page") || 1));
            const pageSize = Math.max(1, Number(q.get("page_size") || 50));
            res.setHeader("X-Total-Count", filtered.length);
            data = filtered.slice((page - 1) * pageSize, page * pageSize).map(row => ({ ...row, source_status: row.status, local_status: row.status, local_basis: "本地制度计算" }));
          } else if (/^\/hrm\/attendance\/(leaves|outs|trips)(\/[^/]+)?$/.test(path)) {
            const parts = path.split("/");
            const kind = parts[3];
            const id = parts[4];
            if (id) {
              if (approvalDetailFailure) { res.writeHead(503, { "Content-Type": "application/json" }).end(JSON.stringify({ message: "审批详情读取失败，请稍后重试" })); return; }
              const row = approvalRecords[kind].find((item) => item.id === id);
              const hasOriginal = row.source === "wecom" && id !== "unmatched-leave";
              data = { ...row, raw_json: "NEVER_RENDER_PRIVATE_RAW_JSON", approval_form: hasOriginal ? {
                title: { leaves: "员工请假申请", outs: "员工外出申请", trips: "员工出差申请" }[kind],
                applicant: { name: "段详政", employee_no: "HR001", department_names: ["研发部"] },
                apply_time: "2026-10-28 15:30:00",
                fields: [{ label: "事由说明", value: row.reason }, { label: "同行人员", value: "李四、王五" }, { label: "交通方式", value: "高铁" }],
                timeline: [{ name: "段详政", status: "submitted", status_name: "已提交", time: "2026-10-28 15:30:00", comment: "" },
                  { name: "王主管", status: "approved", status_name: "已同意", time: "2026-10-28 16:00:00", comment: "同意，请注意出行安全\n已与部门协调" }],
                notice: "",
              } : { title: "", applicant: {}, fields: [], timeline: [], notice: "" } };
            } else {
              if (req.method === "POST") {
                const row = approvalRecords[kind].find((item) => item.id === body.id);
                if (row) Object.assign(row, body);
              }
              const filtered = approvalRecords[kind].filter((row) => !url.searchParams.get("search") || [row.employee_name, row.reason].some((value) => value?.includes(url.searchParams.get("search"))));
              res.setHeader("X-Total-Count", filtered.length);
              data = filtered;
            }
          } else if (path === "/hrm/attendance/month")
            data = {
              month: url.searchParams.get("month"),
              basis: "local_trial", status: "trial", warning: "本地试算，未计算日期不可视为正常出勤", result_version: 0,
              items: [
                {
                  employee_id: "e1",
                  name: "张三", employee_no:"0033", department:"品质部", expected_hours:176, actual_hours:179.25, credited_hours:160, overtime_hours:12.5, overtime_workday_hours:4, overtime_restday_hours:8.5, overtime_holiday_hours:0,
                  expected_days: 22,
                  worked_days: 20,
                  late_count: 1,
                  early_count: 0,
                  missing_count: 2,
                  leave_hours: 8,
                  out_hours: 0,
                  days: [
                    {date:"2026-10-02",state:"uncomputed"},{date:"2026-10-03",state:"pending"},
                    {
                      date: "2026-10-01",
                      expected_hours: 8,
                      worked: true,
                      late_count: 1,
                    },
                  ],
                },
              ],
            };
          else if(path==="/wecom/checkin-rules/snapshots")data=req.method==="POST"?{id:"snapshot-1",snapshot_type:"corp"}:[{id:"snapshot-1",snapshot_type:"corp",fetched_at:"2026-10-29 12:00:00"}];
          else if(path==="/wecom/checkin-rules/snapshots/snapshot-1")data={id:"snapshot-1",raw_json:JSON.stringify({source:"wecom",response:{group:[{groupid:20,groupname:"生产一部",checkindate:[{checkintime:[{work_sec:28800,off_work_sec:63000}],allow_flex:false}]}]}})};
          else if (path === "/wecom/status")
            data = {
              configured: true,
              corp_id: wecomSettings.corp_id,
              agent_id: wecomSettings.agent_id,
              secret_configured: true,
              scopes: { contact: true, checkin: true, approval: true },
              credentials: Object.fromEntries(["contact", "app"].map((kind) => [kind, {
                configured: true,
                source: savedCredentials[kind] ? "saved" : "environment",
                has_saved: savedCredentials[kind],
                environment_variable: kind === "contact" ? "WECOM_CONTACT_SECRET" : "WECOM_APP_SECRET",
              }])),
              running: false,
              automatic_schedule: automaticSchedule,
            };
          else if (path === "/wecom/settings") {
            if (req.method === "POST") {
              const { contact_secret, app_secret, clear_contact_secret, clear_app_secret, ...publicSettings } = body;
              if (clear_contact_secret) savedCredentials.contact = false;
              else if (contact_secret) savedCredentials.contact = true;
              if (clear_app_secret) savedCredentials.app = false;
              else if (app_secret) savedCredentials.app = true;
              wecomSettings = publicSettings;
            }
            data = wecomSettings;
          } else if (path.startsWith("/wecom/diagnostics/") && req.method === "POST") {
            const kind = path.split("/").at(-1);
            data = {
              kind,
              status: { contact: "limited", app: "success", checkin: "failed", approval: "missing" }[kind],
              credential_configured: true,
              checked_at: "2026-10-28 18:00:00",
              checks: {
                contact: [
                  { name: "部门 ID", endpoint: "department/simplelist", status: "success", message: "可读取部门 ID", details: { department_count: 2, name_available: false, secret: "DO_NOT_RENDER" } },
                  { name: "部门详情", endpoint: "department/list", status: "failed", code: 48009, message: "当前凭据没有详细资料权限 <img src=x onerror=window.xss=1>" },
                ],
                app: [{ name: "应用身份", status: "success", message: "身份验证通过", details: { agent_id: "123", app_name: "考勤应用", visible_department_ids: [2, 4] } }],
                checkin: [{ name: "打卡权限", status: "failed", code: 48002, message: "请在打卡 API 中添加此应用" }],
                approval: [{ name: "审批权限", status: "skipped", message: "缺少本次检查所需配置" }],
              }[kind],
            };
          } else if (path === "/wecom/filter-options")
            data = filterChoices;
          else if (path === "/wecom/templates") {
            if (req.method === "POST")
              templates.push({
                ...body,
                mapping_json: JSON.stringify(body.mapping),
              });
            data = templates;
          } else if (path === "/wecom/exceptions")
            data = {
              totals: { contacts: 2, checkins: 2, approvals: 655 },
              limit: 100,
              approval_counts: { configured: 23, unmapped: 632 },
              contacts: [
                { wecom_userid: "hidden-contact-user", employee_name: "王芳", employee_no: "3240", synced_at: "2026-10-29 09:00:00", processing_error: "待关联部门" },
                { wecom_userid: "hidden-unmatched-contact", employee_name: "hidden-unmatched-contact", employee_no: "", synced_at: "2026-10-29 09:00:00", processing_error: "未匹配员工" },
              ],
              checkins: [
                { wecom_userid: "hidden-checkin-user", employee_name: "徐小会", employee_no: "3299", checkin_time: "2026-10-29 08:00:00", checkin_type: "", processing_error: "unsupported checkin_type; raw record retained" },
                { wecom_userid: "hidden-unmatched-checkin", employee_name: "", employee_no: "", checkin_time: "2026-10-29 08:00:00", checkin_type: "其他打卡", processing_error: "unsupported checkin_type; raw record retained" },
              ],
              approvals: [
                {
                  sp_no: "sp1",
                  wecom_userid: "hidden-approval-user",
                  employee_name: "胡起伟",
                  employee_no: "1599",
                  processing_error: "未匹配模板 <script>window.xss=1</script>",
                },
                { sp_no: "sp2", wecom_userid: "hidden-unmatched-approval", employee_id: "hidden-employee-id", employee_name: "hidden-employee-id", employee_no: "", processing_error: "normalized approval failed HRM validation or persistence" },
                { sp_no: "sp3", employee_name: "模板异常员工", employee_no: "0088", processing_error: "approval template is not enabled or mapped" },
              ],
            };
          else if (path.startsWith("/wecom/sync/") && req.method === "POST")
            data = { run_id: "run1", status: "running" };
          else if (path === "/wecom/departments/mapping") {
            if (req.method === "POST") organizationDepartments[0].host_dept_id = body.host_dept_id;
            data = organizationDepartments;
          }
          else if (path === "/wecom/organization/sync") {
            if ((req.method === "POST" && organizationFailure) || (req.method === "GET" && organizationReadFailure)) {
              res.writeHead(503, { "Content-Type": "application/json" }).end(JSON.stringify({ message: "组织同步暂时不可用" }));
              return;
            }
            if (req.method === "POST") {
              await delay(120);
              organizationReport = { departments_created: 2, departments_updated: 1, accounts_created: 3, accounts_updated: 1, employees_linked: 4, skipped: 2, conflict_count: 2,
                conflicts: [
                  { entity_type: "employee", name: "缺工号员工", employee_no: "", department_path: "总公司 / 业务部", reason: "未填写工号，待补充" },
                  { entity_type: "account", name: "重复工号员工", employee_no: "1599", department_path: "总公司 / 研发部", reason: "工号重复 <img src=x onerror=window.organizationXss=1>" },
                ] };
            }
            data = organizationReport;
          }
          else if (path === "/wecom/organization/prune") {
            await delay(120);
            if ((req.method === "GET" && pruneReadFailure) || (req.method === "POST" && pruneWriteFailure)) {
              res.writeHead(503, { "Content-Type": "application/json" }).end(JSON.stringify({ message: "过滤清理暂时不可用" }));
              return;
            }
            data = pruneReport;
          }
          else if (path === "/im/summary" || path === "/im/manager/summary")
            data = { private_messages: 2, groups: 1 };
          else if (path === "/im/directory/departments") data = directoryDepartments;
          else if (path === "/im/directory/users") {
            const query = url.searchParams.get('q') || '', dep = Number(url.searchParams.get('department_id') || 0), page = Number(url.searchParams.get('page') || 1), size=Number(url.searchParams.get('page_size') || 20);
            const ids = new Set([dep]); if(url.searchParams.get('include_subdepartments') !== 'false') for(let i=0;i<directoryDepartments.length;i++) for(const row of directoryDepartments) if(ids.has(row.parent_id))ids.add(row.id);
            const rows=directoryUsers.filter(row=>(url.searchParams.get('unassigned')==='true'?!row.department_id:!dep||ids.has(row.department_id))&&(!query||row.name.includes(query)||row.employee_no.includes(query))).map(directoryPerson);
            if(directoryDelay)await delay(directoryDelay);
            if(directoryFailure){res.writeHead(503,{'Content-Type':'application/json'}).end(JSON.stringify({code:503,message:'通讯录暂不可用'}));return;}
            data={items:rows.slice((page-1)*size,page*size),total:rows.length,page,page_size:size};
          }
          else if (path === '/im/private/peer') data=directoryPerson(directoryUsers.find(row=>row.id===Number(url.searchParams.get('user_id'))));
          else if (/^\/im\/contacts\/\d+\/action$/.test(path)) {
            const id=Number(path.split('/')[3]), old=contactSettings.get(id)||{};
            contactSettings.set(id,{...old,...(body.action==='update'?{display_name:body.display_name}:{blocked_by_me:body.action==='block'})});data=true;
          }
          else if (path === '/im/friends' || path === '/im/friend-requests') {res.writeHead(410,{'Content-Type':'application/json'}).end(JSON.stringify({code:410,message:'请使用企业通讯录'}));return;}
          else if (path === '/im/conversations') data=conversations.map(row=>row.conversation_type==='private'?{...row,...contactSettings.get(Number(row.conversation_id))}:row);
          else if (path === "/im/private/messages") {
            if (req.method === "POST") {
              data = {
                id: "m" + nextSeq,
                seq: nextSeq,
                change_seq: nextSeq,
                sender_id: 1,
                sender_name: "管理员",
                target_id: 2,
                ...body,
              };
              messages.push(data);
              nextSeq++;
              push({ type: "message" });
            } else
              data = messages.filter(
                (m) =>
                  m.change_seq > Number(url.searchParams.get("after_seq") || 0)
              );
          } else if (path === "/im/groups") {
            if(req.method==='POST')await delay(100);
            if(req.method==='POST' && rejectGroupCreation){res.writeHead(403,{'Content-Type':'application/json'}).end(JSON.stringify({code:403,message:'not a participant or recipient is unavailable'}));return;}
            if(req.method==='POST')createdGroupCount++;
            data = req.method === "POST" ? {id:"created-group",...body} : [{ id: "g1", name: "研发群", owner_id: 1, owner_name: "管理员", status: "active" }];
          }
          else if (path === "/im/channels")
            data = [
              { id: "c1", name: "公司公告", owner_id: 1, status: "active" },
            ];
          else if (path === "/im/groups/g1/members")
            data = [{ user_id: 1, name: "管理员", employee_no: "001", role: "owner", status: "active" },{user_id:2,name:"李四",employee_no:"002",role:"member",status:"active"}];
          else if (path === "/im/media/upload")
            data = {
              id: "file1",
              name: "hello.txt",
              url: "/uploads/hello.txt",
            };
          else if (path === "/im/rtc/config") data = { configured: false };
          else if (path === "/im/conversations/read") data = {};
          else if (req.method === "POST") data = { id: "new" };
          res
            .writeHead(200, { "Content-Type": "application/json" })
            .end(JSON.stringify({ code: 0, data }));
          return;
        }
        const relative = url.pathname.startsWith(base)
          ? url.pathname.slice(base.length)
          : "";
        const filename = relative.startsWith("assets/")
          ? join(root, relative)
          : join(root, "index.html");
        const data = await readFile(filename);
        res
          .writeHead(200, {
            "Content-Type":
              extname(filename) === ".js"
                ? "application/javascript"
                : extname(filename) === ".css"
                ? "text/css"
                : "text/html",
          })
          .end(data);
      } catch (error) {
        res.writeHead(500).end(error.message);
      }
    });
    server.on("upgrade", (req, socket) => {
      const key = createHash("sha1")
        .update(
          req.headers["sec-websocket-key"] +
            "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
        )
        .digest("base64");
      socket.write(
        "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: " +
          key +
          "\r\n\r\n"
      );
      peers.add(socket);
      socket.on("data", () => {});
      socket.on("close", () => peers.delete(socket));
      socket.on("error", () => peers.delete(socket));
    });
    await new Promise((r) => server.listen(0, "127.0.0.1", r));
    const origin = "http://127.0.0.1:" + server.address().port;
    const browserPath =
      process.env.CHROME_PATH ||
      [
        "C:/Program Files/Google/Chrome/Application/chrome.exe",
        "C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe",
        "/usr/bin/chromium",
        "/usr/bin/google-chrome",
      ].find(existsSync);
    // Explicit CHROME_PATH supports Linux CI; Windows defaults to the installed Chrome.
    assert.ok(
      browserPath,
      "Set CHROME_PATH to an installed Chromium / Chrome / Edge executable"
    );
    await access(browserPath);
    const userDir = await mkdtemp(join(tmpdir(), "biz-suite-browser-"));
    const child = spawn(
      browserPath,
      [
        "--headless=new",
        "--disable-gpu",
        "--no-first-run",
        "--no-default-browser-check",
        "--remote-debugging-port=0",
        "--remote-allow-origins=*",
        "--user-data-dir=" + userDir,
        "about:blank",
      ],
      { windowsHide: true, stdio: "ignore" }
    );
    let socket;
    try {
      let port;
      for (let i = 0; i < 100; i++) {
        try {
          port = (
            await readFile(join(userDir, "DevToolsActivePort"), "utf8")
          ).split("\n")[0];
          break;
        } catch {
          await delay(70);
        }
      }
      assert.ok(port, "Chrome started");
      const targets = await (
        await fetch(`http://127.0.0.1:${port}/json/list`)
      ).json();
      socket = new WebSocket(
        targets.find((t) => t.type === "page").webSocketDebuggerUrl
      );
      await new Promise((resolve, reject) => {
        socket.addEventListener("open", resolve);
        socket.addEventListener("error", reject);
      });
      const cdp = new CDP(socket);
      await cdp.send("Runtime.enable");
      await cdp.send("Page.enable");
      await cdp.send("Emulation.setDeviceMetricsOverride", {
        width: 1440,
        height: 1000,
        deviceScaleFactor: 1,
        mobile: false,
      });
      await t.test(
        "deep route refresh uses absolute assets and safely renders employee values",
        async () => {
          await cdp.go(origin + base + "hrm/employees");
          await cdp.wait("document.querySelector('table')");
          assert.equal(await cdp.eval("window.xss"), undefined);
          assert.match(await cdp.eval("document.body.innerText"), /<img src=x/);
          assert.equal(
            await cdp.eval('document.querySelectorAll("td img").length'),
            0
          );
          assert.equal(
            await cdp.eval('document.querySelector("h1").textContent'),
            "员工与账号映射"
          );
          if (process.env.BIZ_SCREENSHOT_PATH) {
            const shot = await cdp.send("Page.captureScreenshot", {
              format: "png",
            });
            await writeFile(
              process.env.BIZ_SCREENSHOT_PATH,
              Buffer.from(shot.data, "base64")
            );
          }
        }
      );
      await t.test(
        "employee form persists numeric host mapping and position",
        async () => {
          await cdp.eval(
            "[...document.querySelectorAll('button')].find(b=>b.textContent==='+ 新增员工').click()"
          );
          await cdp.wait("document.querySelector('dialog form')");
          await cdp.eval(
            "document.querySelector('[name=name]').value='赵六';document.querySelector('[name=employee_no]').value='002';document.querySelector('[name=user_id]').value='1';document.querySelector('[name=dept_id]').value='1';document.querySelector('[name=position]').value='测试';document.querySelector('dialog form').requestSubmit()"
          );
          await cdp.wait("!document.querySelector('dialog')");
          const post = recorded.find(
            (r) => r.method === "POST" && r.path === "/hrm/employees"
          );
          assert.equal(post.body.user_id, 1);
          assert.equal(post.body.position, "测试");
          assert.equal(post.headers["x-requested-with"], "XMLHttpRequest");
        }
      );
      await t.test(
        "employee mappings show readable identities and organization sync reports conflicts without duplicate submissions",
        async () => {
          await cdp.go(origin + base + "hrm/employees");
          await cdp.wait("document.querySelector('[data-organization-sync]')?.textContent.includes('尚未同步')");
          const table = await cdp.eval("document.querySelector('table').textContent");
          for (const label of ["姓名", "工号", "登录账号", "显示名", "本地部门", "管理员姓名", "本地总公司 / 技术部", "已关联"])
            assert.ok(table.includes(label), label);
          assert.doesNotMatch(table, /宿主账号 ID|部门 ID|企业微信 userid|wx1/);
          const calls = () => recorded.filter((r) => r.path === "/wecom/organization/sync" && r.method === "POST");
          assert.equal(calls().length, 0, "loading the previous result is read-only");
          await cdp.eval("[...document.querySelectorAll('button')].find(b=>b.textContent==='同步组织与账号').click()");
          assert.equal(await cdp.eval("[...document.querySelectorAll('button')].find(b=>b.textContent==='正在同步…').disabled"), true);
          await cdp.wait("document.querySelector('.organization-result')?.textContent.includes('查看冲突（2）')");
          assert.equal(calls().length, 1);
          assert.deepEqual(calls()[0].body, {});
          const result = await cdp.eval("document.querySelector('[data-organization-sync]').textContent");
          for (const text of ["新增部门 2", "新增停用账号 3", "关联员工 4", "冲突 2", "管理员设置密码", "分配合适角色"])
            assert.ok(result.includes(text), text);
          await cdp.eval("[...document.querySelectorAll('button')].find(b=>b.textContent==='查看冲突（2）').click()");
          await cdp.wait("document.querySelector('dialog')?.textContent.includes('重复工号员工')");
          assert.match(await cdp.eval("document.querySelector('dialog').textContent"), /未填写工号.*工号重复/s);
          assert.equal(await cdp.eval("window.organizationXss"), undefined);
          assert.equal(await cdp.eval("document.querySelectorAll('dialog img, dialog pre').length"), 0);
          await cdp.eval("document.querySelector('dialog .close').click()");
          await cdp.eval("[...document.querySelectorAll('tbody tr button')].find(b=>b.textContent==='详情').click()");
          await cdp.wait("document.querySelector('dialog')");
          assert.match(await cdp.eval("document.querySelector('dialog').textContent"), /登录账号.*001.*显示名.*管理员姓名.*企业微信成员标识.*wx1/s);
          assert.equal(await cdp.eval("document.querySelector('dialog pre')"), null);
          await cdp.eval("document.querySelector('dialog .close').click()");
          await cdp.eval("[...document.querySelectorAll('tbody tr button')].find(b=>b.textContent==='编辑').click()");
          await cdp.wait("document.querySelector('dialog [name=user_id]')");
          assert.match(await cdp.eval("document.querySelector('dialog [name=user_id]').textContent"), /管理员姓名 · 001.*停用员工 · 002 · 停用/);
          assert.match(await cdp.eval("document.querySelector('dialog [name=dept_id]').textContent"), /本地总公司 \/ 技术部/);
          await cdp.eval("document.querySelector('dialog .close').click()");
        }
      );
      await t.test(
        "department mapping uses named hierarchy and sync errors remain recoverable without changing the mapping",
        async () => {
          await cdp.go(origin + base + "wecom/departments");
          await cdp.wait("document.querySelector('.organization-result')?.textContent.includes('上次同步结果')");
          const tables = await cdp.eval("[...document.querySelectorAll('table')].map(table=>table.textContent).join(' ')");
          assert.match(tables, /总公司 \/ 研发中心.*本地总公司 \/ 技术部/s);
          assert.doesNotMatch(tables, /企业微信部门 ID|上级 ID|宿主部门 ID/);
          await cdp.eval("[...document.querySelectorAll('button')].find(b=>b.textContent==='设置映射').click()");
          await cdp.wait("document.querySelector('dialog [name=host_dept_id]')");
          assert.equal(await cdp.eval("document.querySelector('dialog [name=name]').value"), "研发中心");
          assert.match(await cdp.eval("document.querySelector('dialog [name=host_dept_id]').textContent"), /本地总公司 \/ 技术部/);
          await cdp.eval("document.querySelector('dialog form').requestSubmit()");
          await cdp.wait("!document.querySelector('dialog') && document.querySelector('.organization-result')?.textContent.includes('上次同步结果')");
          assert.deepEqual(recorded.filter((r) => r.path === "/wecom/departments/mapping" && r.method === "POST").at(-1).body, { wecom_id: 1, host_dept_id: 1 });
          organizationFailure = true;
          await cdp.eval("[...document.querySelectorAll('button')].find(b=>b.textContent==='同步组织与账号').click()");
          await cdp.wait("document.querySelector('.organization-result [role=alert]')?.textContent.includes('组织同步暂时不可用')");
          assert.equal(await cdp.eval("[...document.querySelectorAll('button')].find(b=>b.textContent==='同步组织与账号').disabled"), false);
          assert.match(await cdp.eval("document.querySelector('#view').textContent"), /本地总公司 \/ 技术部/);
          organizationFailure = false;
          await cdp.eval("[...document.querySelectorAll('button')].find(b=>b.textContent==='同步组织与账号').click()");
          await cdp.wait("document.querySelector('.organization-result')?.textContent.includes('上次同步结果')");
          organizationReadFailure = true;
          await cdp.go(origin + base + "wecom/departments");
          await cdp.wait("document.querySelector('.organization-result [role=alert]')");
          const posts = recorded.filter((r) => r.path === "/wecom/organization/sync" && r.method === "POST").length;
          organizationReadFailure = false;
          await cdp.eval("[...document.querySelectorAll('button')].find(b=>b.textContent==='重试读取结果').click()");
          await cdp.wait("document.querySelector('.organization-result')?.textContent.includes('上次同步结果')");
          assert.equal(recorded.filter((r) => r.path === "/wecom/organization/sync" && r.method === "POST").length, posts);
        }
      );
      await t.test(
        "filtered organization cleanup previews without writes and is shared by employee and department pages",
        async () => {
          await cdp.go(origin + base + "hrm/employees");
          await cdp.wait("document.querySelector('[data-prune-open]')");
          const calls = (method) => recorded.filter((r) => r.path === "/wecom/organization/prune" && r.method === method);
          const gets = calls("GET").length, posts = calls("POST").length;
          await cdp.eval("document.querySelector('[data-prune-open]').click();document.querySelector('[data-prune-open]').dispatchEvent(new MouseEvent('click'))");
          await cdp.wait("document.querySelector('[data-prune-summary=preview]')");
          assert.equal(calls("GET").length, gets + 1);
          assert.equal(calls("POST").length, posts);
          const preview = await cdp.eval("document.querySelector('dialog').textContent");
          for (const text of ["将移出当前员工 35", "将移除自动导入账号 3", "将移除本地部门 10", "此前已过滤员工 2", "尚未修改数据", "保留历史考勤", "不会修改企业微信源通讯录", "人工账号", "账号保持停用"])
            assert.ok(preview.includes(text), text);
          assert.equal(await cdp.eval("document.querySelectorAll('dialog img,dialog pre').length"), 0);
          assert.equal(await cdp.eval("window.pruneXss"), undefined);
          await cdp.eval("[...document.querySelectorAll('dialog button')].find(b=>b.textContent==='取消').click()");
          await cdp.wait("!document.querySelector('dialog') && !document.querySelector('[data-prune-open]').disabled");
          assert.equal(calls("POST").length, posts);
          assert.equal(await cdp.eval("document.querySelector('[data-prune-open]').disabled"), false);
          await cdp.go(origin + base + "wecom/departments");
          await cdp.wait("document.querySelector('[data-prune-open]')");
          assert.equal(calls("POST").length, posts);
        }
      );
      await t.test(
        "filtered organization cleanup retries previews and writes, prevents duplicate confirmation and refreshes the page",
        async () => {
          await cdp.go(origin + base + "wecom/departments");
          await cdp.wait("document.querySelector('[data-prune-open]')");
          const calls = (method) => recorded.filter((r) => r.path === "/wecom/organization/prune" && r.method === method);
          const posts = calls("POST").length;
          pruneReadFailure = true;
          await cdp.eval("document.querySelector('[data-prune-open]').click()");
          await cdp.wait("document.querySelector('dialog [role=alert]')?.textContent.includes('过滤清理暂时不可用')");
          assert.equal(calls("POST").length, posts);
          pruneReadFailure = false;
          await cdp.eval("[...document.querySelectorAll('dialog button')].find(b=>b.textContent==='重试预览').click()");
          await cdp.wait("document.querySelector('[data-prune-summary=preview]')");
          pruneWriteFailure = true;
          await cdp.eval("{const b=[...document.querySelectorAll('dialog button')].find(b=>b.textContent==='确认清理');b.click();b.dispatchEvent(new MouseEvent('click'));}");
          assert.equal(await cdp.eval("document.querySelector('dialog .close').disabled"), true);
          await cdp.wait("document.querySelector('[data-prune-feedback] [role=alert]')?.textContent.includes('过滤清理暂时不可用')");
          assert.equal(calls("POST").length, posts + 1);
          assert.equal(await cdp.eval("[...document.querySelectorAll('dialog button')].find(b=>b.textContent==='确认清理').disabled"), false);
          pruneWriteFailure = false;
          const refreshes = recorded.filter((r) => r.path === "/wecom/departments/mapping" && r.method === "GET").length;
          await cdp.eval("{const b=[...document.querySelectorAll('dialog button')].find(b=>b.textContent==='确认清理');b.click();b.dispatchEvent(new MouseEvent('click'));}");
          await cdp.wait("document.querySelector('[data-prune-summary=result]') && document.querySelector('dialog')?.textContent.includes('清理完成')");
          await cdp.wait("document.querySelector('[data-prune-open]') && !document.querySelector('dialog .close').disabled");
          assert.equal(calls("POST").length, posts + 2);
          assert.deepEqual(calls("POST").at(-1).body, {});
          assert.ok(recorded.filter((r) => r.path === "/wecom/departments/mapping" && r.method === "GET").length > refreshes);
          assert.match(await cdp.eval("document.querySelector('dialog').textContent"), /已移出当前员工 35.*已移除自动导入账号 3.*已移除本地部门 10/s);
          await cdp.eval("[...document.querySelectorAll('dialog button')].find(b=>b.textContent==='完成').click()");
        }
      );
      await t.test(
        "filtered organization cleanup is unavailable without WeCom management permission",
        async () => {
          const previousPermissions = permissions;
          const calls = recorded.filter((r) => r.path === "/wecom/organization/prune").length;
          permissions = ["biz-suite.hrm.read", "biz-suite.wecom.read", "biz-suite.wecom.write"];
          try {
            for (const route of ["hrm/employees", "wecom/departments"]) {
              await cdp.go(origin + base + route);
              await cdp.wait("document.querySelector('table')");
              assert.equal(await cdp.eval("document.querySelector('[data-prune-open]')"), null);
            }
            assert.equal(recorded.filter((r) => r.path === "/wecom/organization/prune").length, calls);
          } finally { permissions = previousPermissions; }
        }
      );
      await t.test(
        "HRM page controls fetch the next server page and search all records",
        async () => {
          for (let i = 0; i < 60; i++)
            employees.push({
              id: "extra-" + i,
              name: "分页员工" + i,
              status: "active",
            });
          await cdp.go(origin + base + "hrm/employees");
          await cdp.wait(
            "document.querySelector('.count')?.textContent.includes('62')"
          );
          await cdp.eval(
            "[...document.querySelectorAll('button')].find(b=>b.textContent==='下一页').click()"
          );
          await cdp.wait(
            "document.querySelector('.pager')?.textContent.includes('2 / 2')"
          );
          assert.ok(
            recorded.some(
              (r) => r.path === "/hrm/employees" && r.query.includes("page=2")
            )
          );
          await cdp.eval(
            "document.querySelector('input[type=search]').value='分页员工59';[...document.querySelectorAll('button')].find(b=>b.textContent==='搜索').click()"
          );
          await cdp.wait(
            "document.querySelector('.count')?.textContent==='共 1 条'"
          );
          assert.match(
            await cdp.eval("document.querySelector('tbody').textContent"),
            /分页员工59/
          );
        }
      );
      await t.test(
        "all attendance states are Chinese in the list, details and manual form",
        async () => {
          await cdp.go(origin + base + "hrm/clocks");
          await cdp.wait("document.querySelector('.clock-filters') && document.querySelector('tbody')");
          const labels = ["正常", "迟到", "早退", "缺卡", "异常"];
          assert.deepEqual(await cdp.eval("[...document.querySelectorAll('tbody tr')].slice(0,5).map(row=>row.cells[5].textContent)"), labels);
          assert.deepEqual(await cdp.eval("[...document.querySelector('[name=clock_status]').options].map(option=>option.textContent)"), ["全部状态", ...labels, "未计算", "进行中", "待配置", "待复核"]);
          assert.doesNotMatch(await cdp.eval("document.querySelector('table').textContent"), /\b(normal|late|early|missing|abnormal)\b/);
          for (let index = 0; index < labels.length; index++) {
            await cdp.eval(`[...document.querySelectorAll('tbody tr')][${index}].querySelectorAll('button').forEach(button=>{if(button.textContent==='详情')button.click()})`);
            await cdp.wait("document.querySelector('dialog .details')");
            assert.equal(await cdp.eval("[...document.querySelectorAll('dialog dt')].find(item=>item.textContent==='本地认定').nextElementSibling.textContent"), labels[index]);
            assert.doesNotMatch(await cdp.eval("document.querySelector('dialog').textContent"), /\b(normal|late|early|missing|abnormal)\b/);
            await cdp.eval("document.querySelector('dialog .close').click()");
          }
          await cdp.eval("[...document.querySelectorAll('tbody tr:first-child button')].find(button=>button.textContent==='编辑').click()");
          await cdp.wait("document.querySelector('dialog form')");
          assert.deepEqual(await cdp.eval("[...document.querySelector('dialog [name=status]').options].map(option=>option.textContent)"), labels);
          await cdp.eval("document.querySelector('dialog .close').click()");
        }
      );
      await t.test(
        "clock records show names and server filters persist through paging, refresh and manual edits",
        async () => {
          await cdp.go(origin + base + "hrm/clocks");
          await cdp.wait("document.querySelector('.clock-filters') && document.querySelector('tbody')");
          assert.match(await cdp.eval("document.querySelector('tbody').textContent"), /段详政.*B002.*业务部/s);
          assert.doesNotMatch(await cdp.eval("document.querySelector('tbody').textContent"), /employee-b/);
          assert.equal(await cdp.eval("document.querySelector('thead').textContent.includes('员工 ID')"), false);
          assert.equal(await cdp.eval("[...document.querySelectorAll('tbody tr')][1].textContent.includes('编辑')"), false, "upstream records cannot be edited");
          assert.match(await cdp.eval("document.querySelector('[name=clock_department]').textContent"), /总公司 \/ 分子公司 \/ 业务部 · 企业微信/);
          await cdp.eval(`
            const employeeSearch=document.querySelector('[name=clock_employee_search]');
            employeeSearch.value='段详政';employeeSearch.dispatchEvent(new Event('input'));
          `);
          assert.deepEqual(await cdp.eval("[...document.querySelector('[name=clock_employee_id]').options].slice(1).map(o=>o.textContent)"), ["段详政 · 研发部 · 工号 A001", "段详政 · 业务部 · 工号 B002"]);
          await cdp.eval(`
            document.querySelector('[name=clock_employee_id]').value='employee-b';
            var clockMode=document.querySelector('[name=clock_date_mode]');clockMode.value='day';clockMode.dispatchEvent(new Event('change'));
            document.querySelector('[name=clock_date]').value='2026-10-29';
            document.querySelector('[name=clock_department]').value='wecom:4';
            document.querySelector('[name=clock_include_subdepartments]').checked=false;
            document.querySelector('[name=clock_search]').value='段详政';
            document.querySelector('.clock-filters').requestSubmit();
          `);
          await cdp.wait("document.querySelector('.count')?.textContent==='共 55 条'");
          const expected = { page: "1", page_size: "50", search: "段详政", employee_id: "employee-b", employee_search: "段详政", department: "wecom:4", basis: "local", include_subdepartments: "false", date: "2026-10-29" };
          const latestQuery = () => Object.fromEntries(new URLSearchParams(recorded.filter((r) => r.method === "GET" && r.path === "/hrm/attendance/clocks").at(-1).query));
          assert.deepEqual(latestQuery(), expected);
          await cdp.eval("[...document.querySelectorAll('button')].find(b=>b.textContent==='下一页').click()");
          await cdp.wait("document.querySelector('.pager')?.textContent.includes('2 / 2')");
          assert.deepEqual(latestQuery(), { ...expected, page: "2" });
          assert.equal(await cdp.eval("document.querySelector('[name=clock_employee_id]').value"), "employee-b");
          assert.equal(await cdp.eval("document.querySelector('[name=clock_employee_search]').value"), "段详政");
          assert.equal(await cdp.eval("document.querySelector('[name=clock_date]').value"), "2026-10-29");
          await cdp.eval("[...document.querySelectorAll('button')].find(b=>b.textContent==='刷新').click()");
          await cdp.wait("document.querySelector('.pager')?.textContent.includes('2 / 2')");
          assert.deepEqual(latestQuery(), { ...expected, page: "2" });
          await cdp.eval("[...document.querySelectorAll('tbody tr:first-child button')].find(b=>b.textContent==='编辑').click()");
          await cdp.wait("document.querySelector('dialog form')");
          assert.match(await cdp.eval("document.querySelector('dialog [name=employee_id]').selectedOptions[0].textContent"), /段详政.*业务部.*B002/);
          await cdp.eval("document.querySelector('dialog [name=address]').value='深圳园区更新';document.querySelector('dialog form').requestSubmit()");
          await cdp.wait("!document.querySelector('dialog') && document.querySelector('.pager')?.textContent.includes('2 / 2')");
          assert.deepEqual(latestQuery(), { ...expected, page: "2" });
          assert.match(await cdp.eval("document.querySelector('tbody').textContent"), /深圳园区更新/);
          await cdp.eval("document.querySelector('.clock-filters').requestSubmit()");
          await cdp.wait("document.querySelector('.pager')?.textContent.includes('1 / 2')");
          assert.deepEqual(latestQuery(), expected, "query resets the server page to one");
          assert.equal(await cdp.eval("getComputedStyle(document.querySelector('[name=clock_month]').closest('.field')).display"), "none");
          assert.equal(await cdp.eval("getComputedStyle(document.querySelector('[name=clock_start_time]').closest('.field')).display"), "none");
          if (process.env.BIZ_CLOCK_SCREENSHOT_PATH) {
            const shot = await cdp.send("Page.captureScreenshot", { format: "png" });
            await writeFile(process.env.BIZ_CLOCK_SCREENSHOT_PATH, Buffer.from(shot.data, "base64"));
          }
          await cdp.send("Emulation.setDeviceMetricsOverride", { width: 390, height: 844, deviceScaleFactor: 1, mobile: true });
          assert.equal(await cdp.eval("document.documentElement.scrollWidth <= innerWidth"), true, "mobile page has no horizontal overflow");
          if (process.env.BIZ_CLOCK_MOBILE_SCREENSHOT_PATH) {
            const shot = await cdp.send("Page.captureScreenshot", { format: "png" });
            await writeFile(process.env.BIZ_CLOCK_MOBILE_SCREENSHOT_PATH, Buffer.from(shot.data, "base64"));
          }
          await cdp.send("Emulation.setDeviceMetricsOverride", { width: 1440, height: 1000, deviceScaleFactor: 1, mobile: false });
        }
      );
      await t.test(
        "clock personnel text submits without a selection and department changes cascade and clear stale choices",
        async () => {
          await cdp.go(origin + base + "hrm/clocks");
          await cdp.wait("document.querySelector('.clock-filters') && document.querySelector('tbody')");
          const choices = () => cdp.eval("[...document.querySelector('[name=clock_employee_id]').options].filter(o=>o.value&&!o.disabled).map(o=>o.value)");
          const query = () => Object.fromEntries(new URLSearchParams(recorded.filter((r) => r.method === "GET" && r.path === "/hrm/attendance/clocks").at(-1).query));
          await cdp.eval(`
            var personnel=document.querySelector('[name=clock_employee_search]');
            personnel.value='B002';personnel.dispatchEvent(new Event('input'));
            document.querySelector('.clock-filters').requestSubmit();
          `);
          await cdp.wait("document.querySelector('.count')?.textContent==='共 55 条'");
          assert.deepEqual(query(), { page: "1", page_size: "50", search: "", employee_search: "B002", basis: "local", include_subdepartments: "true" });
          assert.equal(await cdp.eval("document.querySelector('[name=clock_employee_id]').value"), "");
          assert.equal(await cdp.eval("document.querySelector('[name=clock_employee_id]').selectedOptions[0].textContent"), "全部匹配人员（1人）");
          assert.deepEqual(await choices(), ["employee-b"]);
          await cdp.eval(`
            var personnel=document.querySelector('[name=clock_employee_search]');personnel.value='';personnel.dispatchEvent(new Event('input'));
            var department=document.querySelector('[name=clock_department]');department.value='wecom:1';department.dispatchEvent(new Event('change'));
          `);
          assert.deepEqual(await choices(), ["employee-a", "employee-b"], "parent includes staff in both direct and deeper departments");
          await cdp.eval("var department=document.querySelector('[name=clock_department]');department.value='wecom:3';department.dispatchEvent(new Event('change'))");
          assert.deepEqual(await choices(), ["employee-b"]);
          await cdp.eval(`
            var employee=document.querySelector('[name=clock_employee_id]');employee.value='employee-b';employee.dispatchEvent(new Event('change'));
            var children=document.querySelector('[name=clock_include_subdepartments]');children.checked=false;children.dispatchEvent(new Event('change'));
          `);
          assert.deepEqual(await choices(), []);
          assert.equal(await cdp.eval("document.querySelector('[name=clock_employee_id]').value"), "", "excluding descendants clears a selected descendant");
          await cdp.eval(`
            var children=document.querySelector('[name=clock_include_subdepartments]');children.checked=true;children.dispatchEvent(new Event('change'));
            var employee=document.querySelector('[name=clock_employee_id]');employee.value='employee-b';employee.dispatchEvent(new Event('change'));
            var department=document.querySelector('[name=clock_department]');department.value='wecom:2';department.dispatchEvent(new Event('change'));
          `);
          assert.deepEqual(await choices(), ["employee-a"]);
          assert.equal(await cdp.eval("document.querySelector('[name=clock_employee_id]').value"), "", "changing department clears an unrelated selection");
          await cdp.eval(`
            var department=document.querySelector('[name=clock_department]');department.value='wecom:3';department.dispatchEvent(new Event('change'));
            document.querySelector('.clock-filters').requestSubmit();
          `);
          await cdp.wait("document.querySelector('[name=clock_department]')?.value==='wecom:3' && document.querySelector('.count')?.textContent==='共 55 条'");
          assert.deepEqual(query(), { page: "1", page_size: "50", search: "", department: "wecom:3", basis: "local", include_subdepartments: "true" });
          assert.deepEqual(await choices(), ["employee-b"]);
          await cdp.eval(`
            var employee=document.querySelector('[name=clock_employee_id]');employee.value='employee-b';employee.dispatchEvent(new Event('change'));
            var personnel=document.querySelector('[name=clock_employee_search]');personnel.value='A001';personnel.dispatchEvent(new Event('input'));
            document.querySelector('.clock-filters').requestSubmit();
          `);
          await cdp.wait("document.querySelector('.count')?.textContent==='共 0 条'");
          assert.deepEqual(query(), { page: "1", page_size: "50", search: "", employee_search: "A001", department: "wecom:3", basis: "local", include_subdepartments: "true" });
          assert.equal(await cdp.eval("document.querySelector('[name=clock_employee_id]').value"), "", "new text cannot retain a previous employee ID");
          await cdp.eval("[...document.querySelectorAll('button')].find(b=>b.textContent==='清空筛选').click()");
          await cdp.wait("document.querySelector('.count')?.textContent==='共 56 条'");
          assert.equal(await cdp.eval("document.querySelector('[name=clock_employee_search]').value"), "");
          assert.deepEqual(await choices(), ["employee-a", "employee-b"]);
        }
      );
      await t.test(
        "clock month and date-range modes are exclusive and unmatched records do not expose IDs",
        async () => {
          await cdp.go(origin + base + "hrm/clocks");
          await cdp.wait("document.querySelector('.clock-filters')");
          await cdp.eval(`
            var clockMode=document.querySelector('[name=clock_date_mode]');clockMode.value='month';clockMode.dispatchEvent(new Event('change'));
            document.querySelector('[name=clock_month]').value='2026-09';document.querySelector('.clock-filters').requestSubmit();
          `);
          await cdp.wait("document.querySelector('.count')?.textContent==='共 1 条'");
          const latestQuery = () => Object.fromEntries(new URLSearchParams(recorded.filter((r) => r.method === "GET" && r.path === "/hrm/attendance/clocks").at(-1).query));
          assert.deepEqual(latestQuery(), { page: "1", page_size: "50", search: "", basis: "local", include_subdepartments: "true", month: "2026-09" });
          assert.match(await cdp.eval("document.querySelector('tbody').textContent"), /未匹配员工/);
          assert.doesNotMatch(await cdp.eval("document.querySelector('tbody').textContent"), /unmatched-internal-hash/);
          await cdp.eval(`
            var clockMode=document.querySelector('[name=clock_date_mode]');clockMode.value='range';clockMode.dispatchEvent(new Event('change'));
            document.querySelector('[name=clock_start_time]').value='2026-10-01';
            document.querySelector('[name=clock_end_time]').value='2026-10-29';
            document.querySelector('.clock-filters').requestSubmit();
          `);
          await cdp.wait("document.querySelector('.count')?.textContent==='共 55 条'");
          assert.deepEqual(latestQuery(), { page: "1", page_size: "50", search: "", basis: "local", include_subdepartments: "true", start_time: "2026-10-01", end_time: "2026-10-29" });
          assert.equal(await cdp.eval("document.querySelector('[name=clock_month]').disabled"), true);
          assert.equal(await cdp.eval("document.querySelector('[name=clock_date]').disabled"), true);
        }
      );
      await t.test(
        "invalid clock date inputs are explained in Chinese without a request and clearing restores all records",
        async () => {
          await cdp.go(origin + base + "hrm/clocks");
          await cdp.wait("document.querySelector('.clock-filters')");
          await cdp.eval("var clockMode=document.querySelector('[name=clock_date_mode]');clockMode.value='range';clockMode.dispatchEvent(new Event('change'))");
          const count = () => recorded.filter((r) => r.path === "/hrm/attendance/clocks").length;
          const before = count();
          await cdp.eval("document.querySelector('[name=clock_start_time]').value='2026-10-30';document.querySelector('[name=clock_end_time]').value='2026-10-29';document.querySelector('.clock-filters').requestSubmit()");
          await cdp.wait("document.querySelector('.clock-filters [role=alert]')?.textContent.includes('结束日期不能早于开始日期')");
          assert.equal(count(), before);
          await cdp.eval("document.querySelector('[name=clock_start_time]').value='';document.querySelector('.clock-filters').requestSubmit()");
          assert.match(await cdp.eval("document.querySelector('.clock-filters [role=alert]').textContent"), /有效的开始日期和结束日期/);
          assert.equal(count(), before);
          await cdp.eval("var clockMode=document.querySelector('[name=clock_date_mode]');clockMode.value='day';clockMode.dispatchEvent(new Event('change'));document.querySelector('[name=clock_date]').value='2026-02-30';document.querySelector('.clock-filters').requestSubmit()");
          assert.match(await cdp.eval("document.querySelector('.clock-filters [role=alert]').textContent"), /有效的打卡日期/);
          assert.equal(count(), before);
          await cdp.eval("[...document.querySelectorAll('button')].find(b=>b.textContent==='清空筛选').click()");
          await cdp.wait("document.querySelector('.count')?.textContent==='共 56 条'");
          const query = new URLSearchParams(recorded.filter((r) => r.path === "/hrm/attendance/clocks").at(-1).query);
          assert.deepEqual(Object.fromEntries(query), { page: "1", page_size: "50", search: "", basis: "local", include_subdepartments: "true" });
          assert.equal(await cdp.eval("document.querySelector('[name=clock_date_mode]').value"), "all");
          assert.equal(await cdp.eval("document.querySelector('[name=clock_employee_id]').value"), "");
          assert.equal(await cdp.eval("document.querySelector('[name=clock_department]').value"), "");
        }
      );
      await t.test(
        "clock request errors retry the same filters and empty filtered results remain queryable",
        async () => {
          clockFailure = true;
          await cdp.eval("document.querySelector('[name=clock_department]').value='host:2';document.querySelector('[name=clock_status]').value='missing';document.querySelector('.clock-filters').requestSubmit()");
          await cdp.wait("document.querySelector('[role=alert]')?.textContent.includes('打卡查询暂时失败')");
          const failed = recorded.filter((r) => r.path === "/hrm/attendance/clocks").at(-1).query;
          clockFailure = false;
          await cdp.eval("[...document.querySelectorAll('button')].find(b=>b.textContent==='重试').click()");
          await cdp.wait("document.querySelector('.clock-filters') && document.querySelector('.empty')");
          assert.equal(recorded.filter((r) => r.path === "/hrm/attendance/clocks").at(-1).query, failed);
          assert.equal(await cdp.eval("document.querySelector('[name=clock_department]').value"), "host:2");
          assert.equal(await cdp.eval("document.querySelector('[name=clock_status]').value"), "missing");
          assert.equal(new URLSearchParams(failed).get("status"), "missing");
          assert.match(await cdp.eval("document.querySelector('.empty').textContent"), /暂无记录/);
          await cdp.eval("[...document.querySelectorAll('button')].find(b=>b.textContent==='清空筛选').click()");
          await cdp.wait("document.querySelector('.count')?.textContent==='共 56 条'");
          assert.equal(await cdp.eval("document.querySelector('[name=clock_status]').value"), "");
        }
      );
      await t.test(
        "status filtering combines with personnel, descendant departments and dates and survives pagination, refresh and editing",
        async () => {
          await cdp.go(origin + base + "hrm/clocks");
          await cdp.wait("document.querySelector('.clock-filters') && document.querySelector('tbody')");
          const query = () => Object.fromEntries(new URLSearchParams(recorded.filter((r) => r.method === "GET" && r.path === "/hrm/attendance/clocks").at(-1).query));
          await cdp.eval(`
            var employeeSearch=document.querySelector('[name=clock_employee_search]');
            employeeSearch.value='B002';employeeSearch.dispatchEvent(new Event('input'));
            var department=document.querySelector('[name=clock_department]');department.value='wecom:3';department.dispatchEvent(new Event('change'));
            document.querySelector('[name=clock_employee_id]').value='employee-b';
            var mode=document.querySelector('[name=clock_date_mode]');mode.value='month';mode.dispatchEvent(new Event('change'));
            document.querySelector('[name=clock_month]').value='2026-10';
            document.querySelector('[name=clock_status]').value='normal';
            document.querySelector('[name=clock_search]').value='深圳';
            document.querySelector('.clock-filters').requestSubmit();
          `);
          await cdp.wait("document.querySelector('.count')?.textContent==='共 51 条'");
          const expected = { page: "1", page_size: "50", search: "深圳", employee_id: "employee-b", employee_search: "B002", department: "wecom:3", status: "normal", basis: "local", include_subdepartments: "true", month: "2026-10" };
          assert.deepEqual(query(), expected);
          assert.equal(await cdp.eval("[...document.querySelectorAll('tbody tr')].every(row=>row.cells[5].textContent==='正常')"), true);
          await cdp.eval("[...document.querySelectorAll('button')].find(button=>button.textContent==='下一页').click()");
          await cdp.wait("document.querySelector('.pager')?.textContent.includes('2 / 2')");
          assert.deepEqual(query(), { ...expected, page: "2" });
          assert.equal(await cdp.eval("document.querySelectorAll('tbody tr').length"), 1, "server count and page contain only the selected status");
          assert.equal(await cdp.eval("document.querySelector('[name=clock_status]').value"), "normal");
          await cdp.eval("[...document.querySelectorAll('button')].find(button=>button.textContent==='刷新').click()");
          await cdp.wait("document.querySelector('.pager')?.textContent.includes('2 / 2')");
          assert.deepEqual(query(), { ...expected, page: "2" });
          await cdp.eval("[...document.querySelectorAll('tbody tr:first-child button')].find(button=>button.textContent==='编辑').click()");
          await cdp.wait("document.querySelector('dialog form')");
          await cdp.eval("document.querySelector('dialog [name=address]').value='深圳状态筛选回归';document.querySelector('dialog form').requestSubmit()");
          await cdp.wait("!document.querySelector('dialog') && document.querySelector('tbody')?.textContent.includes('深圳状态筛选回归')");
          assert.deepEqual(query(), { ...expected, page: "2" });
          assert.equal(await cdp.eval("document.querySelector('[name=clock_status]').value"), "normal");
          await cdp.eval(`
            var mode=document.querySelector('[name=clock_date_mode]');mode.value='range';mode.dispatchEvent(new Event('change'));
            document.querySelector('[name=clock_start_time]').value='2026-10-01';
            document.querySelector('[name=clock_end_time]').value='2026-10-29';
            document.querySelector('[name=clock_status]').value='late';document.querySelector('.clock-filters').requestSubmit();
          `);
          await cdp.wait("document.querySelector('.count')?.textContent==='共 1 条'");
          const { month, ...rangeExpected } = expected;
          assert.deepEqual(query(), { ...rangeExpected, status: "late", start_time: "2026-10-01", end_time: "2026-10-29" });
          assert.equal(await cdp.eval("document.querySelector('tbody tr').cells[5].textContent"), "迟到");
          assert.match(await cdp.eval("document.querySelector('.pager').textContent"), /1 \/ 1/, "changing status resets page to one");
          for (const [value, label] of [["early", "早退"], ["missing", "缺卡"], ["abnormal", "异常"]]) {
            await cdp.eval(`document.querySelector('[name=clock_status]').value='${value}';document.querySelector('.clock-filters').requestSubmit()`);
            await cdp.wait(`document.querySelector('tbody tr')?.cells[5]?.textContent==='${label}'`);
            assert.equal(query().status, value);
            assert.equal(await cdp.eval("document.querySelector('.count').textContent"), "共 1 条");
          }
          await cdp.eval("[...document.querySelectorAll('button')].find(button=>button.textContent==='清空筛选').click()");
          await cdp.wait("document.querySelector('.count')?.textContent==='共 56 条'");
          assert.deepEqual(query(), { page: "1", page_size: "50", search: "", basis: "local", include_subdepartments: "true" });
          assert.equal(await cdp.eval("document.querySelector('[name=clock_status]').value"), "");
        }
      );
      await t.test(
        "leave, out and trip records show people and reasons with structured Chinese approval details",
        async () => {
          for (const [kind, label] of [["leaves", "请假"], ["outs", "外出"], ["trips", "出差"]]) {
            await cdp.go(origin + base + "hrm/" + kind);
            await cdp.wait("document.querySelector('tbody')");
            const table = await cdp.eval("document.querySelector('table').textContent");
            assert.match(table, /员工姓名.*员工编号.*部门/s);
            assert.ok(table.includes(label + "事由"));
            assert.match(table, /段详政.*HR001.*研发部/s);
            assert.match(table, /处理个人事务\n第二行说明/);
            assert.doesNotMatch(table, /private-employee-hash|private-option-id|员工 ID/);
            if (kind === "leaves") assert.match(table, /年假/);
            assert.equal(await cdp.eval("[...document.querySelectorAll('tbody tr:first-child button')].some(b=>b.textContent==='编辑')"), false);
            await cdp.eval("[...document.querySelectorAll('tbody tr:first-child button')].find(b=>b.textContent==='详情').click()");
            await cdp.wait(`document.querySelector('[data-approval-kind=${kind}]')`);
            const detail = await cdp.eval("document.querySelector('dialog').textContent");
            assert.ok(detail.includes("员工" + label + "申请"));
            for (const text of ["申请人", "所属部门", "段详政", "研发部", "申请内容", label + "事由", "企业微信表单", "同行人员", "李四、王五", "审批流程", "王主管", "已同意", "审批编号", "202610290001", "申请时间（上海）"])
              assert.ok(detail.includes(text), `${kind}: ${text}`);
            assert.doesNotMatch(detail, /private-employee-hash|private-option-id|NEVER_RENDER_PRIVATE_RAW_JSON|employee_id|approval_form/);
            assert.equal(await cdp.eval("document.querySelectorAll('dialog pre,dialog img').length"), 0);
            assert.equal(await cdp.eval("window.approvalXss"), undefined);
            if (kind === "leaves" && process.env.BIZ_APPROVAL_SCREENSHOT_PATH) {
              const shot = await cdp.send("Page.captureScreenshot", { format: "png" });
              await writeFile(process.env.BIZ_APPROVAL_SCREENSHOT_PATH, Buffer.from(shot.data, "base64"));
            }
            await cdp.send("Emulation.setDeviceMetricsOverride", { width: 390, height: 844, deviceScaleFactor: 1, mobile: true });
            assert.equal(await cdp.eval("document.querySelector('.dialog-body').scrollWidth <= document.querySelector('.dialog-body').clientWidth"), true, `${kind}: mobile detail has no horizontal overflow`);
            await cdp.send("Emulation.setDeviceMetricsOverride", { width: 1440, height: 1000, deviceScaleFactor: 1, mobile: false });
            await cdp.eval("document.querySelector('dialog .close').click()");
          }
        }
      );
      await t.test(
        "manual and unmatched approval details have clear fallbacks and preserve editing permissions",
        async () => {
          await cdp.go(origin + base + "hrm/leaves");
          await cdp.wait("document.querySelector('tbody')");
          await cdp.eval("[...document.querySelectorAll('tbody tr:nth-child(2) button')].find(b=>b.textContent==='详情').click()");
          await cdp.wait("document.querySelector('.approval-notice')");
          assert.match(await cdp.eval("document.querySelector('dialog').textContent"), /手工登记.*未关联企业微信原始审批表单/s);
          assert.equal(await cdp.eval("document.querySelectorAll('dialog pre').length"), 0);
          await cdp.eval("document.querySelector('dialog .close').click()");
          await cdp.eval("[...document.querySelectorAll('tbody tr:nth-child(2) button')].find(b=>b.textContent==='编辑').click()");
          await cdp.wait("document.querySelector('dialog form')");
          await cdp.eval("document.querySelector('dialog [name=reason]').value='调整手工事由';document.querySelector('dialog form').requestSubmit()");
          await cdp.wait("!document.querySelector('dialog') && document.querySelector('tbody')?.textContent.includes('调整手工事由')");
          const post = recorded.filter((r) => r.method === "POST" && r.path === "/hrm/attendance/leaves").at(-1);
          assert.equal(post.body.source, "manual");
          assert.equal(post.body.id, "manual-leaves");
          await cdp.eval("[...document.querySelectorAll('tbody tr:nth-child(3) button')].find(b=>b.textContent==='详情').click()");
          await cdp.wait("document.querySelector('.approval-notice')");
          const missing = await cdp.eval("document.querySelector('dialog').textContent");
          assert.match(missing, /未匹配员工/);
          assert.match(missing, /未识别类型/);
          assert.match(missing, /暂未找到企业微信原始审批表单/);
          assert.doesNotMatch(missing, /unmatched-employee-secret|87654321/);
          await cdp.eval("document.querySelector('dialog .close').click()");
          const originalPermissions = permissions;
          permissions = ["biz-suite.hrm.read"];
          try {
            await cdp.go(origin + base + "hrm/outs");
            await cdp.wait("document.querySelector('tbody')");
            assert.equal(await cdp.eval("[...document.querySelectorAll('button')].some(b=>b.textContent.includes('新增')||b.textContent==='编辑')"), false);
            await cdp.eval("[...document.querySelectorAll('tbody tr:first-child button')].find(b=>b.textContent==='详情').click()");
            await cdp.wait("document.querySelector('[data-approval-kind=outs]')");
            await cdp.eval("document.querySelector('dialog .close').click()");
          } finally { permissions = originalPermissions; }
        }
      );
      await t.test(
        "approval detail errors stay in the dialog and retry the same record without falling back to raw JSON",
        async () => {
          await cdp.go(origin + base + "hrm/trips");
          await cdp.wait("document.querySelector('tbody')");
          approvalDetailFailure = true;
          try {
            await cdp.eval("[...document.querySelectorAll('tbody tr:first-child button')].find(b=>b.textContent==='详情').click()");
            await cdp.wait("document.querySelector('dialog [role=alert]')?.textContent.includes('审批详情读取失败')");
            assert.equal(await cdp.eval("document.querySelectorAll('dialog pre').length"), 0);
            approvalDetailFailure = false;
            await cdp.eval("[...document.querySelectorAll('dialog button')].find(b=>b.textContent==='重新读取详情').click()");
            await cdp.wait("document.querySelector('[data-approval-kind=trips]')");
            assert.ok(recorded.filter((r) => r.path === "/hrm/attendance/trips/wecom-trips").length >= 2);
            assert.equal(await cdp.eval("document.querySelector('#view h1').textContent"), "出差记录");
            await cdp.eval("document.querySelector('dialog .close').click()");
          } finally { approvalDetailFailure = false; }
        }
      );
      await t.test(
        "group editor generates typed shift JSON without a raw JSON form",
        async () => {
          await cdp.go(origin + base + "hrm/groups");
          await cdp.wait(
            "[...document.querySelectorAll('button')].some(b=>b.textContent==='+ 新增考勤组')"
          );
          await cdp.eval(
            "[...document.querySelectorAll('button')].find(b=>b.textContent==='+ 新增考勤组').click();document.querySelector('dialog [name=name]').value='研发组';document.querySelector('dialog form').requestSubmit()"
          );
          await cdp.wait("!document.querySelector('dialog')");
          const post = recorded.find(
            (r) => r.method === "POST" && r.path === "/hrm/attendance/groups"
          );
          assert.deepEqual(
            JSON.parse(post.body.shifts)[0].weekdays,
            [1, 2, 3, 4, 5]
          );
          assert.equal(JSON.parse(post.body.shifts)[0].start, "09:00");
        }
      );
      await t.test(
        "sync exceptions show reliable employee names and numbers and readable checkin errors without ID fallbacks",
        async () => {
          await cdp.go(origin + base + "wecom/exceptions");
          await cdp.wait("document.querySelectorAll('[data-exception-kind] table').length === 3");
          for (const [kind, name, number] of [["contacts", "王芳", "3240"], ["checkins", "徐小会", "3299"], ["approvals", "胡起伟", "1599"]]) {
            const text = await cdp.eval(`document.querySelector('[data-exception-kind=${kind}]').textContent`);
            for (const expected of ["员工姓名", "工号", name, number, "未匹配员工", "未填写工号"])
              assert.ok(text.includes(expected), `${kind}: ${expected}`);
            assert.doesNotMatch(text, /企业微信用户|hidden-/);
          }
          const checkins = await cdp.eval("document.querySelector('[data-exception-kind=checkins]').textContent");
          assert.ok(checkins.includes("企业微信未返回打卡类型，无法识别上下班或外出；原始记录已保留"));
          assert.ok(checkins.includes("暂不支持该打卡类型，原始记录已保留"));
          assert.doesNotMatch(checkins, /unsupported checkin_type/);
          const approvals = await cdp.eval("document.querySelector('[data-exception-kind=approvals]').textContent");
          assert.match(approvals, /仅显示前 3 条 \/ 共 655 条/);
          assert.match(approvals, /已配置模板异常 23 条；未配置模板 632 条/);
          assert.match(approvals, /审批起止、申请时长或业务字段校验未通过；原文已保留，请核对/);
          assert.match(approvals, /审批模板未配置或未启用/);
          assert.doesNotMatch(approvals, /normalized approval failed|approval template is not enabled/);
          assert.equal(await cdp.eval("document.querySelectorAll('[data-exception-kind] img,[data-exception-kind] script').length"), 0);
          assert.equal(await cdp.eval("window.xss"), undefined);
          await cdp.eval("document.querySelector('[data-exception-kind=contacts] tbody button').click()");
          await cdp.wait("document.querySelector('dialog')");
          assert.match(await cdp.eval("document.querySelector('dialog').textContent"), /hidden-contact-user/);
          await cdp.eval("document.querySelector('dialog .close').click()");
        }
      );
      await t.test("approval template sends official control IDs", async () => {
        await cdp.go(origin + base + "wecom/templates");
        await cdp.wait(
          "[...document.querySelectorAll('button')].some(b=>b.textContent==='+ 新增模板')"
        );
        await cdp.eval(
          "[...document.querySelectorAll('button')].find(b=>b.textContent==='+ 新增模板').click();document.querySelector('[name=template_id]').value='tpl1';document.querySelector('[name=attendance_control_id]').value='Vacation-123';document.querySelector('dialog form').requestSubmit()"
        );
        await cdp.wait("!document.querySelector('dialog')");
        const post = recorded.find(
          (r) => r.method === "POST" && r.path === "/wecom/templates"
        );
        assert.equal(post.body.mapping.attendance_control_id, "Vacation-123");
        assert.equal(post.body.enabled, 1);
      });
      await t.test("production employee rules show fixed weekdays and apply the complete overtime preset", async () => {
        await cdp.go(origin + base + "hrm/attendance-rules");
        await cdp.wait("[...document.querySelectorAll('button')].some(b=>b.textContent==='新增制度')");
        await cdp.eval("[...document.querySelectorAll('button')].find(b=>b.textContent==='新增制度').click()");
        await cdp.wait("document.querySelector('[data-attendance-rule-editor]')");
        assert.equal(await cdp.eval("document.querySelectorAll('dialog textarea').length"),0);
        assert.equal(await cdp.eval("getComputedStyle(document.querySelector('dialog [data-work-schedule]')).display!=='none'"),true);
        await cdp.eval(`document.querySelector('dialog [name=name]').value='生产一部核定规则';document.querySelector('dialog [name=effective_from]').value='2026-10-01';document.querySelector('dialog [data-overtime-category=restday] input[type=checkbox]').checked=false;document.querySelector('dialog [data-overtime-category=restday] [name=maximum_minutes]').value=60;document.querySelector('dialog [data-overtime-category=restday] [name=rounding_scope]').value='segment';[...document.querySelectorAll('dialog button')].find(b=>b.textContent==='载入生产一部有无加班模板').click()`);
        assert.equal(await cdp.eval("document.querySelector('dialog [name=anchor]').disabled"),true);
        assert.equal(await cdp.eval("document.querySelector('dialog [data-attendance-cycle]').hidden"),true);
        assert.equal(await cdp.eval("document.querySelector('dialog [data-attendance-weekly]').hidden"),true);
        assert.match(await cdp.eval("document.querySelector('dialog [data-work-schedule]').textContent"), /周一至周五为应出勤日.*周六、周日无卡为休息/s);
        assert.deepEqual(await cdp.eval("[...document.querySelectorAll('dialog [name=weekday_shift_ids]:checked')].map(e=>e.value)"),['production-one-normal']);
        assert.deepEqual(await cdp.eval("[...document.querySelectorAll('dialog [name=restday_shift_ids]:checked')].map(e=>e.value)"),['production-one-normal']);
        assert.equal(await cdp.eval("document.querySelector('dialog').textContent.includes('null')"),false,"A missing legacy conversion notice must not become literal null text");
        assert.equal(await cdp.eval("document.querySelector('dialog [name=schedule_mode]').closest('label').hidden"),true,"Fixed production schedules must not offer editable cycle modes");
        if (process.env.BIZ_ATTENDANCE_RULE_SCREENSHOT_PATH) {
          await cdp.eval("document.querySelector('dialog [data-work-schedule]').scrollIntoView({block:'start'})");
          const shot = await cdp.send("Page.captureScreenshot", { format: "png" });
          await writeFile(process.env.BIZ_ATTENDANCE_RULE_SCREENSHOT_PATH, Buffer.from(shot.data, "base64"));
        }
        await cdp.eval("var mode=document.querySelector('dialog [name=scope_mode]');mode.value='default';mode.dispatchEvent(new Event('change'))");
        assert.equal(await cdp.eval("document.querySelector('dialog [name=in_start_second]').value"),'',"Unknown windows must not silently receive two hours");
        assert.match(await cdp.eval("document.querySelector('dialog [data-window-preview]').textContent"),/待配置/);
        await cdp.eval(`for(const editor of document.querySelectorAll('dialog [data-local-windows]')){const row=editor.parentElement;const start=row.querySelector('[name=start]').value;const times=start==='08:00'?['07:00:00','09:59:59','10:00:00','12:39:59']:start==='13:10'?['12:40:00','15:29:59','15:30:00','17:44:59']:['17:45:00','18:00:00','18:00:01','05:59:59'];['in_start_second','in_end_second','out_start_second','out_end_second'].forEach((key,i)=>{editor.querySelector('[name='+key+']').value=times[i];editor.querySelector('[name='+key+'_day]').value=start==='17:45'&&i===3?'1':'0';editor.querySelector('[name='+key+']').dispatchEvent(new Event('input'));});}`);
        attendanceSaveFailure=true;
        await cdp.eval("document.querySelector('dialog form').requestSubmit()");
        await cdp.wait("document.querySelector('dialog [role=alert]')?.textContent.includes('保存暂时失败')");
        assert.equal(attendanceRules.length,0);
        attendanceSaveFailure=false;
        const before=recorded.filter(r=>r.path==='/hrm/attendance/v2/rules'&&r.method==='POST').length;
        await cdp.eval("document.querySelector('dialog form').requestSubmit();document.querySelector('dialog form').requestSubmit()");
        await cdp.wait("!document.querySelector('dialog')&&document.querySelector('tbody')?.textContent.includes('生产一部核定规则')");
        assert.equal(recorded.filter(r=>r.path==='/hrm/attendance/v2/rules'&&r.method==='POST').length,before+1);
        const rule=attendanceRules[0];
        assert.equal(rule.config.cycle.mode,'weekly');assert.equal(rule.config.cycle.anchor,undefined);assert.equal(rule.config.cycle.days,undefined);
        assert.deepEqual(rule.config.cycle.weekdays,Array.from({length:7},(_,i)=>({shift_id:'production-one-normal',candidate_shift_ids:['production-one-normal'],category:i<5?'workday':'restday',scheduled_work:i<5})));
        assert.deepEqual(rule.config.work_schedule,{mode:'production_employee',weekday_shift_ids:['production-one-normal'],saturday_shift_ids:[],restday_shift_ids:['production-one-normal']});
        assert.equal(rule.config.shifts[0].overtime_variant.trigger_minute,1065);
        const overtime=rule.config.shifts[0].overtime_variant.shift.segments[2];
        assert.equal(overtime.start_minute,1065);assert.equal(overtime.overtime_start_minute,1080);assert.equal(overtime.open_ended,true);
        assert.equal(rule.config.rule.overtime.workday.rounding_minutes,30);
        assert.equal(rule.config.rule.overtime.workday.require_approval,false);
        assert.equal(rule.config.rule.overtime.restday.require_approval,false);
        assert.equal(rule.config.rule.overtime.restday.enabled,true);
        assert.equal(rule.config.rule.overtime.restday.maximum_minutes,0);
        assert.equal(rule.config.rule.overtime.restday.rounding_scope,'daily');
        assert.equal(rule.config.rule.overtime.restday.complete_workday_minutes,480);
        assert.equal(rule.config.rule.overtime.holiday.require_approval,false);
        assert.equal(rule.status,'draft');
        await cdp.eval("[...document.querySelectorAll('tbody button')].find(b=>b.textContent==='发布').click()");
        await cdp.wait("document.querySelector('[data-attendance-apply]')?.textContent.includes('适用员工')");
        assert.equal(await cdp.eval("!!document.querySelector('dialog [name=confirmation]')"),false);
        await cdp.eval("[...document.querySelectorAll('dialog button')].find(b=>b.textContent==='确认生效并自动计算').click()");
        await cdp.wait("!document.querySelector('dialog') && document.querySelector('tbody')?.textContent.includes('已发布')");
      });
      await t.test("production staff use Saturday regular attendance and preserve independent Saturday shifts", async () => {
        const rule=structuredClone(attendanceRules[0]);Object.assign(rule,{id:'staff-fixed-week',name:'生产职员固定周休测试',status:'draft'});
        rule.config.category='生产职员';const saturday={...structuredClone(rule.config.shifts[0]),id:'saturday-normal',name:'周六正班'};delete saturday.overtime_variant;rule.config.shifts.push(saturday);
        rule.config.work_schedule={mode:'production_staff',weekday_shift_ids:['production-one-normal'],saturday_shift_ids:['saturday-normal'],restday_shift_ids:['production-one-normal']};attendanceRules.push(rule);
        try{
          await cdp.go(origin+base+'hrm/attendance-rules');await cdp.wait("document.querySelector('tbody')?.textContent.includes('生产职员固定周休测试')");
          await cdp.eval("[...document.querySelectorAll('tbody tr')].find(r=>r.textContent.includes('生产职员固定周休测试')).querySelector('button').click()");
          assert.match(await cdp.eval("document.querySelector('dialog [data-work-schedule]').textContent"),/周一至周六为应出勤日.*周日无卡为休息/s);
          assert.equal(await cdp.eval("document.querySelector('dialog [name=schedule_mode]').closest('label').hidden"),true);
          assert.deepEqual(await cdp.eval("[...document.querySelectorAll('dialog [name=saturday_shift_ids]:checked')].map(e=>e.value)"),['saturday-normal']);
          assert.equal(await cdp.eval("document.querySelector('dialog [data-work-schedule]').textContent.includes('null')"),false);
          await cdp.eval("document.querySelector('dialog form').requestSubmit()");await cdp.wait("!document.querySelector('dialog')");
          const saved=attendanceRules.find(r=>r.id===rule.id);assert.deepEqual(saved.config.work_schedule,rule.config.work_schedule);
          assert.deepEqual(saved.config.cycle.weekdays[5],{shift_id:'saturday-normal',candidate_shift_ids:['saturday-normal'],category:'workday',scheduled_work:true});
          assert.equal(saved.config.cycle.weekdays[6].category,'restday');assert.equal(saved.config.cycle.weekdays[6].scheduled_work,false);
        }finally{const index=attendanceRules.findIndex(r=>r.id===rule.id);if(index>=0)attendanceRules.splice(index,1);}
      });
      await t.test("scope candidates filter by name or employee number and local descendants without losing hidden selections", async () => {
        await cdp.go(origin+base+'hrm/attendance-rules');await cdp.wait("document.querySelector('#view tbody')");
        const choices={departments:[{id:1,pid:0,name:'总公司'},{id:2,pid:1,name:'生产部'},{id:3,pid:2,name:'组装部'},{id:4,pid:1,name:'业务部'}],employees:[{id:'p1',name:'王芳',employee_no:'3240',dept_id:2},{id:'p2',name:'王芳',employee_no:'1599',dept_id:3},{id:'1599',name:'刘云',employee_no:'999',dept_id:4}]};
        await cdp.eval(`(async()=>{const {scopeEditor}=await import('${base}assets/attendance-scope.js');const {dialog}=await import('${base}assets/core.js');window.scopeFixture=scopeEditor({employee_ids:['p1','missing-person'],department_ids:[2,999],include_children:true,is_default:false},${JSON.stringify(choices)});dialog('范围验证',window.scopeFixture.node);})()`);
        assert.equal(await cdp.eval("document.querySelector('[name=scope_mode]').value"),'union');
        assert.match(await cdp.eval("document.querySelector('[data-scope-summary]').textContent"),/并集/);
        assert.match(await cdp.eval("document.querySelector('[data-scope-selected=employees]').textContent"),/历史选择仍保留/);
        assert.doesNotMatch(await cdp.eval("document.querySelector('[data-attendance-scope]').textContent"),/missing-person|历史记录：999/);
        if(process.env.BIZ_ATTENDANCE_SCOPE_SCREENSHOT_PATH){const shot=await cdp.send('Page.captureScreenshot',{format:'png'});await writeFile(process.env.BIZ_ATTENDANCE_SCOPE_SCREENSHOT_PATH,Buffer.from(shot.data,'base64'));}
        await cdp.eval("var q=document.querySelector('[name=scope_employee_search]');q.value='1599';q.dispatchEvent(new Event('input'))");
        assert.deepEqual(await cdp.eval("[...document.querySelectorAll('[data-scope-results=employees] [data-scope-id]')].map(e=>e.dataset.scopeId)"),['p2']);
        await cdp.eval("document.querySelector('[data-scope-select-visible=employees]').click();var q=document.querySelector('[name=scope_employee_search]');q.value='王芳';q.dispatchEvent(new Event('input'));var d=document.querySelector('[name=scope_employee_department]');d.value='2';d.dispatchEvent(new Event('change'));document.querySelector('[name=scope_employee_include_children]').click()");
        assert.deepEqual(await cdp.eval("[...document.querySelectorAll('[data-scope-results=employees] [data-scope-id]')].map(e=>e.dataset.scopeId)"),['p1']);
        await cdp.eval("document.querySelector('[data-scope-clear-visible=employees]').click()");
        assert.deepEqual(await cdp.eval("window.scopeFixture.read().employee_ids"),['missing-person','p2']);
        await cdp.eval("document.querySelector('[name=scope_employee_include_children]').click()");
        assert.deepEqual(await cdp.eval("[...document.querySelectorAll('[data-scope-results=employees] [data-scope-id]')].map(e=>e.dataset.scopeId)"),['p1','p2']);
        assert.match(await cdp.eval("document.querySelector('[data-scope-count=employees]').textContent"),/已选 2 项.*1 项不在当前筛选/);
        await cdp.eval("var m=document.querySelector('[name=scope_mode]');m.value='departments';m.dispatchEvent(new Event('change'));document.querySelector('[name=scope_include_children]').click()");
        assert.deepEqual(await cdp.eval("window.scopeFixture.read()"),{employee_ids:[],department_ids:[2,999],include_children:false,is_default:false});
        await cdp.eval("var m=document.querySelector('[name=scope_mode]');m.value='default';m.dispatchEvent(new Event('change'))");
        assert.deepEqual(await cdp.eval("window.scopeFixture.read()"),{employee_ids:[],department_ids:[],include_children:false,is_default:true});
        await cdp.eval("var m=document.querySelector('[name=scope_mode]');m.value='union';m.dispatchEvent(new Event('change'))");
        assert.deepEqual(await cdp.eval("window.scopeFixture.read()"),{employee_ids:['missing-person','p2'],department_ids:[2,999],include_children:false,is_default:false});
        await cdp.eval("document.querySelector('[data-scope-selected=employees] [data-selected-id=missing-person] button').click()");
        assert.deepEqual(await cdp.eval("window.scopeFixture.read().employee_ids"),['p2']);
        assert.match(await cdp.eval("document.querySelector('[data-scope-results=departments]').textContent"),/总公司 \/ 生产部 \/ 组装部/);
        await cdp.eval("document.querySelector('dialog .close').click();delete window.scopeFixture");
      });
      await t.test("legacy default scope retains explicit priority selections unless deliberately removed", async () => {
        await cdp.eval(`(async()=>{const {scopeEditor}=await import('${base}assets/attendance-scope.js');const {dialog}=await import('${base}assets/core.js');window.scopeFixture=scopeEditor({employee_ids:['old-person'],department_ids:[98765],include_children:false,is_default:true},{employees:[],departments:[]});dialog('旧默认范围',window.scopeFixture.node);})()`);
        assert.deepEqual(await cdp.eval("window.scopeFixture.read()"),{employee_ids:['old-person'],department_ids:[98765],include_children:false,is_default:true});
        assert.match(await cdp.eval("document.querySelector('[data-scope-summary]').textContent"),/同时保留原有 1 名人员、1 个部门/);
        await cdp.eval("document.querySelector('[name=scope_preserve_default_extras]').click()");
        assert.deepEqual(await cdp.eval("window.scopeFixture.read()"),{employee_ids:[],department_ids:[],include_children:false,is_default:true});
        await cdp.eval("document.querySelector('[name=scope_preserve_default_extras]').click()");
        assert.deepEqual(await cdp.eval("window.scopeFixture.read().employee_ids"),['old-person']);
        await cdp.eval("document.querySelector('dialog .close').click();delete window.scopeFixture");
      });
      await t.test("imported rules with null scope arrays can be edited and saved without filling unknown windows or losing selected IDs", async () => {
        const fixtures = [
          { employee_ids: ['e1', 'unavailable-employee'], department_ids: null },
          { employee_ids: null, department_ids: [1, 98765] },
          { employee_ids: null, department_ids: null },
        ].map((scope, index) => {
          const row = structuredClone(attendanceRules[0]);
          Object.assign(row, { id: 'null-scope-' + index, name: '导入空范围制度' + index, status: 'draft', conflicts: null, scope: { ...scope, include_children: true, is_default: false } });
          Object.assign(row.config.rule, { late_arrival_tiers: null, leave_policies: null });
          row.config.rule.full_attendance.allowed_leave_types = null;
          row.config.calendar = null;
          row.config.cycle.days = null;
          function clearWindows(shift) {
            shift.evidence_periods = null;
            for (const segment of shift.segments) segment.local_windows = { in_start_second: null, in_end_second: null, out_start_second: null, out_end_second: null };
            if (shift.overtime_variant) clearWindows(shift.overtime_variant.shift);
          }
          row.config.shifts.forEach(clearWindows);
          return row;
        });
        attendanceRules.push(...fixtures);
        const mutationCount = () => recorded.filter(r => /\/hrm\/attendance\/v2\/rules\/[^/]+\/(apply|publish)$/.test(r.path)).length;
        const before = mutationCount();
        try {
          for (const fixture of fixtures) {
            await cdp.go(origin + base + 'hrm/attendance-rules');
            await cdp.wait(`document.querySelector('[data-attendance-current-rules] tbody')?.textContent.includes(${JSON.stringify(fixture.name)})`);
            await cdp.eval(`[...document.querySelectorAll('[data-attendance-current-rules] tbody tr')].find(r => r.textContent.includes(${JSON.stringify(fixture.name)})).querySelector('button').click()`);
            await cdp.wait("document.querySelector('[data-attendance-rule-editor]')");
            const selected = await cdp.eval(`['employees','departments'].map(kind => document.querySelector('dialog [data-scope-selected="'+kind+'"]').querySelectorAll('[data-selected-id]').length)`);
            assert.deepEqual(selected, [fixture.scope.employee_ids?.length || 0, fixture.scope.department_ids?.length || 0]);
            assert.equal(await cdp.eval("document.querySelector('dialog [name=in_start_second]').value"), '');
            await cdp.eval("document.querySelector('dialog form').requestSubmit()");
            await cdp.wait("!document.querySelector('dialog')");
            const saved = attendanceRules.find(r => r.id === fixture.id);
            assert.deepEqual(saved.scope.employee_ids, fixture.scope.employee_ids || []);
            assert.deepEqual(saved.scope.department_ids, fixture.scope.department_ids || []);
            assert.equal(saved.status, 'draft');
            for (const shift of saved.config.shifts) for (const segment of shift.segments) {
              assert.deepEqual(segment.local_windows, { in_start_second: null, in_end_second: null, out_start_second: null, out_end_second: null });
            }
          }
          assert.equal(mutationCount(), before, 'Saving an imported draft must not publish it');
        } finally {
          for (const fixture of fixtures) { const index = attendanceRules.findIndex(r => r.id === fixture.id); if (index >= 0) attendanceRules.splice(index, 1); }
        }
      });
      await t.test("historical attendance rules are folded into read-only audit and excluded from batch window editing", async () => {
        const history = ['archived','superseded'].map((status,i)=>({...structuredClone(attendanceRules[0]),id:'history-rule-'+i,name:'旧来源审计制度'+i,status}));
        history.push({...structuredClone(attendanceRules[0]),id:'wecom-reference-draft',name:'企微原始参考草稿',status:'draft',source:JSON.stringify({snapshot_id:'reference-only-snapshot'})});
        attendanceRules.push(...history);
        try {
          await cdp.go(origin+base+'hrm/attendance-rules');
          await cdp.wait("document.querySelector('[data-attendance-rule-history]')");
          assert.equal(await cdp.eval("document.querySelector('[data-attendance-rule-history]').open"),false);
          assert.doesNotMatch(await cdp.eval("document.querySelector('[data-attendance-current-rules]').textContent"),/旧来源审计制度/);
          assert.doesNotMatch(await cdp.eval("document.querySelector('[data-attendance-current-rules]').textContent"),/企微原始参考草稿/);
          await cdp.eval("document.querySelector('[data-attendance-rule-history] summary').click()");
          assert.deepEqual(await cdp.eval("[...document.querySelectorAll('[data-attendance-rule-history] tbody button')].map(b=>b.textContent)"),['查看','查看','查看']);
          await cdp.eval("document.querySelector('[data-attendance-rule-history] tbody button').click()");
          await cdp.wait("document.querySelector('[data-attendance-rule-editor]')");
          assert.equal(await cdp.eval("document.querySelectorAll('[data-attendance-rule-editor] input:not(:disabled),[data-attendance-rule-editor] select:not(:disabled),[data-attendance-rule-editor] button:not(:disabled)').length"),0);
          assert.equal(await cdp.eval("!!document.querySelector('[data-attendance-rule-editor] .form-actions')"),false);
          await cdp.eval("document.querySelector('dialog .close').click();[...document.querySelectorAll('#view button')].find(b=>b.textContent==='批量设置取卡窗口').click()");
          await cdp.wait("document.querySelector('[data-batch-windows]')");
          assert.doesNotMatch(await cdp.eval("document.querySelector('[data-batch-windows]').textContent"),/旧来源审计制度/);
          assert.doesNotMatch(await cdp.eval("document.querySelector('[data-batch-windows]').textContent"),/企微原始参考草稿/);
          await cdp.eval("document.querySelector('dialog .close').click()");
        } finally { for(const row of history){const i=attendanceRules.findIndex(r=>r.id===row.id);if(i>=0)attendanceRules.splice(i,1);} }
      });
      await t.test("recoverable cleanup hides only explicit true markers in current, history and batch windows without deleting source records", async () => {
        const examples = [
          ['hidden-draft', 'draft', {cleanup:{hidden:true}}],
          ['hidden-published', 'published', JSON.stringify({cleanup:{hidden:true}})],
          ['hidden-archived', 'archived', {cleanup:{hidden:true}}],
          ['hidden-reference', 'draft', {snapshot_id:'old-reference',cleanup:{hidden:true}}],
          ['visible-archived', 'archived', {cleanup:{hidden:false}}],
          ['visible-string', 'draft', {cleanup:{hidden:'true'}}],
          ['visible-null', 'draft', null],
          ['visible-invalid-json', 'draft', '{invalid'],
        ];
        const fixtures = examples.map(([id,status,source]) => ({...structuredClone(attendanceRules[0]),id,name:id,status,source}));
        const before = recorded.filter(r=>r.method!=='GET'&&r.path.startsWith('/hrm/attendance/v2/')).length;
        attendanceRules.push(...fixtures);
        try {
          await cdp.go(origin+base+'hrm/attendance-rules');
          await cdp.wait("document.querySelector('[data-attendance-rule-history]')");
          const current = await cdp.eval("document.querySelector('[data-attendance-current-rules]').textContent"), history = await cdp.eval("document.querySelector('[data-attendance-rule-history]').textContent");
          assert.doesNotMatch(current+history,/hidden-(draft|published|archived|reference)/);
          assert.match(history,/visible-archived/);
          for (const name of ['visible-string','visible-null','visible-invalid-json']) assert.ok(current.includes(name));
          await cdp.eval("[...document.querySelectorAll('#view button')].find(b=>b.textContent==='批量设置取卡窗口').click()");
          await cdp.wait("document.querySelector('[data-batch-windows]')");
          assert.doesNotMatch(await cdp.eval("document.querySelector('[data-batch-windows]').textContent"),/hidden-(draft|published|archived|reference)|visible-archived/);
          await cdp.eval("document.querySelector('dialog .close').click()");
          // Clearing the marker is enough to restore the historical row to view;
          // list rendering never writes, deletes, or changes the rule itself.
          const restored = attendanceRules.find(r=>r.id==='hidden-archived');
          restored.source.cleanup.hidden=false;
          await cdp.go(origin+base+'hrm/attendance-rules');
          await cdp.wait("document.querySelector('[data-attendance-rule-history]')?.textContent.includes('hidden-archived')");
          assert.equal(recorded.filter(r=>r.method!=='GET'&&r.path.startsWith('/hrm/attendance/v2/')).length,before);
          assert.ok(fixtures.every(row=>attendanceRules.includes(row)));
        } finally { for (const row of fixtures) { const i=attendanceRules.findIndex(r=>r.id===row.id); if(i>=0) attendanceRules.splice(i,1); } }
      });
      await t.test("read-only legacy production schedules retain the original cycle and do not infer new fixed weekdays", async () => {
        const normal='production-one-normal',evening='production-one-evening';
        const examples=[
          {id:'legacy-production-cycle',category:'员工',cycle:{anchor:'2026-10-01',days:[normal,evening,'',normal,'',evening,normal]}},
          {id:'legacy-staff-weekly',category:'生产职员',cycle:{mode:'weekly',weekdays:Array.from({length:7},(_,i)=>({shift_id:normal,category:i===0?'restday':'workday',scheduled_work:i!==0}))}},
          {id:'legacy-production-unknown',category:'员工',cycle:{anchor:'',days:null}},
        ];
        const fixtures=examples.map(example=>{const rule=structuredClone(attendanceRules[0]);Object.assign(rule,{id:example.id,name:example.id,status:'archived'});rule.config.category=example.category;rule.config.cycle=example.cycle;delete rule.config.work_schedule;return rule;});
        const before=recorded.filter(r=>r.method!=='GET'&&r.path.startsWith('/hrm/attendance/v2/rules')).length;
        attendanceRules.push(...fixtures);
        try{
          for(const rule of fixtures){
            await cdp.go(origin+base+'hrm/attendance-rules');await cdp.wait("document.querySelector('[data-attendance-rule-history]')");
            await cdp.eval(`var section=document.querySelector('[data-attendance-rule-history]');section.open=true;var search=section.querySelector('input[type=search]');search.value=${JSON.stringify(rule.id)};search.dispatchEvent(new Event('input'));section.querySelector('tbody button').click()`);
            await cdp.wait("document.querySelector('[data-attendance-rule-editor]')");
            assert.equal(await cdp.eval("document.querySelector('dialog [data-work-schedule]').hidden"),true);
            assert.equal(await cdp.eval("document.querySelector('dialog [data-custom-schedule]').hidden"),false);
            assert.equal(await cdp.eval("document.querySelector('dialog [data-custom-schedule] h3').textContent"),'历史作息配置');
            assert.equal(await cdp.eval("document.querySelector('dialog [name=schedule_mode]').value"),rule.config.cycle.mode||'cycle');
            assert.equal(await cdp.eval("document.querySelectorAll('[data-attendance-rule-editor] input:not(:disabled),[data-attendance-rule-editor] select:not(:disabled),[data-attendance-rule-editor] button:not(:disabled)').length"),0);
            if(rule.config.cycle.mode==='weekly'){
              assert.deepEqual(await cdp.eval("Array.from({length:7},(_,i)=>({shift_id:document.querySelector('[name=weekly_shift_'+i+']').value,category:document.querySelector('[name=weekly_category_'+i+']').value,scheduled_work:document.querySelector('[name=weekly_scheduled_'+i+']').checked}))"),rule.config.cycle.weekdays);
            }else{
              assert.equal(await cdp.eval("document.querySelector('dialog [name=anchor]').value"),rule.config.cycle.anchor);
              assert.deepEqual(await cdp.eval("[...document.querySelectorAll('dialog [data-attendance-cycle] select')].map(e=>e.value)"),rule.config.cycle.days||[]);
            }
            assert.equal(await cdp.eval("!!document.querySelector('dialog [data-local-conversion-notice]')"),false);
            await cdp.eval("document.querySelector('dialog .close').click()");
          }
          assert.equal(recorded.filter(r=>r.method!=='GET'&&r.path.startsWith('/hrm/attendance/v2/rules')).length,before);
        }finally{for(const rule of fixtures){const index=attendanceRules.findIndex(r=>r.id===rule.id);if(index>=0)attendanceRules.splice(index,1);}}
      });
      await t.test("local attendance has three primary entries, unconfigured windows block activation and batch edits affect selected segments only", async () => {
        const make=(id,name)=>{const rule=structuredClone(attendanceRules[0]);Object.assign(rule,{id,name,status:'draft'});rule.config.shifts=rule.config.shifts.slice(0,1);delete rule.config.shifts[0].overtime_variant;return rule;};
        const a=make('batch-window-a','批量窗口甲'),b=make('batch-window-b','批量窗口乙');
        const beforeMorning=structuredClone(a.config.shifts[0].segments[0].local_windows);
        a.config.shifts[0].segments[1].local_windows.in_start_second=null;
        attendanceRules.push(a,b);
        try {
          await cdp.go(origin+base+'hrm/attendance-rules');await cdp.wait("document.querySelector('tbody')?.textContent.includes('批量窗口甲')");
          assert.deepEqual(await cdp.eval("[...document.querySelectorAll('.sidebar nav > a')].filter(a=>/attendance-(rules|results|months)$/.test(a.dataset.route)).map(a=>a.textContent.replace(/^\\d+/,''))"),['考勤设置','考勤明细与异常','月度考勤']);
          assert.equal(await cdp.eval("document.querySelector('[data-attendance-advanced]').open"),false);
          const applyBefore=recorded.filter(r=>r.path==='/hrm/attendance/v2/rules/batch-window-a/apply').length;
          await cdp.eval("[...document.querySelectorAll('tbody tr')].find(r=>r.textContent.includes('批量窗口甲')).querySelectorAll('button')[1].click()");
          assert.match(await cdp.eval("document.querySelector('dialog').textContent"),/窗口待配置/);
          assert.equal(recorded.filter(r=>r.path==='/hrm/attendance/v2/rules/batch-window-a/apply').length,applyBefore);
          await cdp.eval("document.querySelector('dialog .close').click();[...document.querySelectorAll('#view button')].find(b=>b.textContent==='批量设置取卡窗口').click()");
          await cdp.wait("document.querySelector('[data-batch-windows]')");
          await cdp.eval("for(const label of document.querySelectorAll('[data-batch-windows] label'))if(/批量窗口[甲乙]/.test(label.textContent)&&label.textContent.includes('下午'))label.querySelector('input[type=checkbox]')?.click();document.querySelector('[data-batch-windows] [name=in_start_second]').value='12:20:00';[...document.querySelectorAll('dialog button')].find(b=>b.textContent==='保存选中班段的窗口').click()");
          await cdp.wait("document.querySelector('[data-batch-windows]')?.textContent.includes('成功项保留为草稿')");
          for(const id of [a.id,b.id]){const rule=attendanceRules.find(r=>r.id===id);assert.equal(rule.config.rule.evidence_mode,'local');assert.equal(rule.config.shifts[0].segments[1].local_windows.in_start_second,44400);assert.equal(rule.config.shifts[0].segments[1].local_windows.in_end_second,b.config.shifts[0].segments[1].local_windows.in_end_second);assert.deepEqual(rule.config.shifts[0].segments[0].local_windows,beforeMorning);}
          assert.equal(recorded.filter(r=>r.path==='/hrm/attendance/v2/rules/batch-window-a/apply').length,applyBefore);
        } finally {for(const id of [a.id,b.id]){const index=attendanceRules.findIndex(r=>r.id===id);if(index>=0)attendanceRules.splice(index,1);}}
      });
      await t.test("continuous evidence edits retain shared sampling, exact overnight boundaries and per-day shift candidates",async()=>{
        const rule=structuredClone(attendanceRules[0]);Object.assign(rule,{id:'continuous-local',name:'两卡跨日采证测试',status:'draft'});rule.config.shifts=rule.config.shifts.slice(0,1);const shift=rule.config.shifts[0];delete shift.overtime_variant;
        rule.config.category='职员';delete rule.config.work_schedule;
        shift.evidence_periods=[{id:'continuous',name:'首末卡采证',start_second:28800,end_second:108000,punch_mode:'both',local_windows:null}];for(const segment of shift.segments){segment.evidence_id='continuous';delete segment.local_windows;}
        rule.config.cycle.weekdays[0]={...rule.config.cycle.weekdays[0],shift_id:'',candidate_shift_ids:[shift.id]};attendanceRules.push(rule);
        try{
          await cdp.go(origin+base+'hrm/attendance-rules');await cdp.wait("document.querySelector('tbody')?.textContent.includes('两卡跨日采证测试')");
          await cdp.eval("[...document.querySelectorAll('tbody tr')].find(r=>r.textContent.includes('两卡跨日采证测试')).querySelector('button').click()");
          assert.equal(await cdp.eval("document.querySelector('dialog [name=weekly_shift_0]').disabled"),true);
          assert.equal(await cdp.eval("[...document.querySelectorAll('dialog [data-local-windows]')].filter(e=>e.hidden).length"),2);
          await cdp.eval("var d=document.querySelector('[data-evidence-period]');for(const [key,value] of Object.entries({in_start_second:'07:00:00',in_end_second:'08:00:00',out_start_second:'05:00:00',out_end_second:'06:00:00'})){d.querySelector('[name='+key+']').value=value;d.querySelector('[name='+key+'_day]').value=key.startsWith('out')?'1':'0';}document.querySelector('dialog form').requestSubmit()");await cdp.wait("!document.querySelector('dialog')");
          const saved=attendanceRules.find(r=>r.id===rule.id);assert.deepEqual(saved.config.cycle.weekdays[0].candidate_shift_ids,[shift.id]);assert.deepEqual(saved.config.shifts[0].evidence_periods[0].local_windows,{in_start_second:25200,in_end_second:28801,out_start_second:104400,out_end_second:108001});assert.equal(saved.config.shifts[0].segments[0].evidence_id,'continuous');
        }finally{const index=attendanceRules.findIndex(r=>r.id===rule.id);if(index>=0)attendanceRules.splice(index,1);}
      });
      await t.test("full-shift credit, fixed meal deductions and late departure thresholds remain explicit through the Chinese editor",async()=>{
        const rule=structuredClone(attendanceRules[0]);Object.assign(rule,{id:'complete-credit-local',name:'注塑完整正班扣休',status:'draft'});Object.assign(rule.config.rule,{attendance_mode:'fixed_complete',standard_minutes:480,complete_work_deduction_minutes:30,late_departure_after_second:77400,late_departure_delay_minutes:60,partial_work_policy:{mode:'review',actual_ranges:[{start_second:28800,end_second:43200}]}});attendanceRules.push(rule);
        const original=structuredClone(attendanceResult.result);
        try{
          await cdp.go(origin+base+'hrm/attendance-rules');await cdp.wait("document.querySelector('tbody')?.textContent.includes('注塑完整正班扣休')");
          await cdp.eval("[...document.querySelectorAll('tbody tr')].find(r=>r.textContent.includes('注塑完整正班扣休')).querySelector('button').click()");
          assert.equal(await cdp.eval("document.querySelector('dialog [name=attendance_mode]').value"),'fixed_complete');
          assert.equal(await cdp.eval("document.querySelector('dialog [name=complete_work_deduction_minutes]').value"),'30');
          assert.equal(await cdp.eval("document.querySelector('dialog [name=late_departure_after]').value"),'21:30:00');
          assert.equal(await cdp.eval("document.querySelector('dialog [name=partial_work_mode]').value"),'review');
          await cdp.eval("document.querySelector('dialog form').requestSubmit()");await cdp.wait("!document.querySelector('dialog')");
          const saved=attendanceRules.find(r=>r.id===rule.id).config.rule;for(const key of ['attendance_mode','complete_work_deduction_minutes','late_departure_after_second','late_departure_delay_minutes','partial_work_policy'])assert.deepEqual(saved[key],rule.config.rule[key]);
          attendanceResult.result={...original,presence_seconds:43200,deducted_seconds:1800,actual_seconds:41400};
          await cdp.go(origin+base+'hrm/attendance-results');await cdp.wait("document.querySelector('[name=q]')");
          await cdp.eval("document.querySelector('[name=q]').value='胡起伟';document.querySelector('[name=include_children]').value='false';document.querySelector('#view form').requestSubmit()");await delay(100);
          const query=new URLSearchParams(recorded.filter(r=>r.path==='/hrm/attendance/v2/results').at(-1).query);assert.equal(query.get('q'),'胡起伟');assert.equal(query.get('include_children'),'false');
          await cdp.eval("[...document.querySelectorAll('tbody button')].find(b=>b.textContent==='计算详情').click()");await cdp.wait("document.querySelector('[data-attendance-deductions]')");
          assert.deepEqual(await cdp.eval("[...document.querySelector('[data-attendance-deductions] table').querySelectorAll('tbody td')].map(e=>e.textContent)"),['12.00','0.50','11.50']);
        }finally{attendanceResult.result=original;const index=attendanceRules.findIndex(r=>r.id===rule.id);if(index>=0)attendanceRules.splice(index,1);}
      });
      await t.test("anchored seven-day rules rotate by weekday while AB cycles and unknown anchors are preserved", async () => {
        const normal='production-one-normal',evening='production-one-evening';
        const scenarios=[
          {id:'rule-legacy-seven',name:'原周四起七天制度',cycle:{anchor:'2026-10-01',days:[normal,evening,'','',normal,normal,normal]},mode:'weekly'},
          {id:'rule-legacy-ab',name:'原大小周制度',cycle:{anchor:'2026-10-01',days:[normal,normal,normal,normal,normal,'','',evening,evening,evening,evening,evening,evening,'']},mode:'cycle'},
          {id:'rule-legacy-unknown',name:'未知起点待确认制度',cycle:{anchor:'',days:[normal,normal,normal,normal,normal,'','']},mode:'cycle'},
        ];
        for(const scenario of scenarios){const rule=structuredClone(attendanceRules[0]);Object.assign(rule,{id:scenario.id,name:scenario.name,status:'draft'});rule.config.category='职员';delete rule.config.work_schedule;rule.config.cycle=scenario.cycle;rule.config.calendar={'2026-10-02':{shift_id:normal,category:'holiday',scheduled_work:false}};attendanceRules.push(rule);}
        try{
          for(const scenario of scenarios){
            await cdp.go(origin+base+'hrm/attendance-rules');await cdp.wait(`document.querySelector('tbody')?.textContent.includes(${JSON.stringify(scenario.name)})`);
            await cdp.eval(`[...document.querySelectorAll('tbody tr')].find(r=>r.textContent.includes(${JSON.stringify(scenario.name)})).querySelector('button').click()`);
            assert.equal(await cdp.eval("document.querySelector('dialog [name=schedule_mode]').value"),scenario.mode);
            if(scenario.id==='rule-legacy-unknown'){
              assert.equal(await cdp.eval("document.querySelector('dialog [name=anchor]').value"),'');
              await cdp.eval("document.querySelector('dialog .close').click()");
              assert.deepEqual(attendanceRules.find(r=>r.id===scenario.id).config.cycle,scenario.cycle);continue;
            }
            if(scenario.mode==='cycle'){
              assert.equal(await cdp.eval("document.querySelector('dialog [name=cycle_length]').value"),'14');
              await cdp.eval("var mode=document.querySelector('dialog [name=schedule_mode]');mode.value='weekly';mode.dispatchEvent(new Event('change'));mode.value='cycle';mode.dispatchEvent(new Event('change'))");
            }else{
              assert.equal(await cdp.eval("document.querySelector('dialog [name=weekly_shift_4]').value"),evening);
              assert.equal(await cdp.eval("document.querySelector('dialog [name=weekly_category_5]').value"),'restday');
            }
            await cdp.eval("document.querySelector('dialog form').requestSubmit()");await cdp.wait("!document.querySelector('dialog')");
            const saved=attendanceRules.find(r=>r.id===scenario.id);
            assert.deepEqual(saved.config.calendar,{'2026-10-02':{shift_id:normal,category:'holiday',scheduled_work:false}});
            if(scenario.mode==='cycle')assert.deepEqual(saved.config.cycle,{mode:'cycle',...scenario.cycle});
            else assert.deepEqual(saved.config.cycle,{mode:'weekly',weekdays:[normal,normal,normal,normal,evening,'',''].map(shift_id=>({shift_id,category:shift_id?'workday':'restday',scheduled_work:!!shift_id}))});
          }
        }finally{for(const scenario of scenarios){const index=attendanceRules.findIndex(r=>r.id===scenario.id);if(index>=0)attendanceRules.splice(index,1);}}
      });
      await t.test("weekly publication rejects a missing workday shift and preserves optional restday work", async () => {
        const rule=structuredClone(attendanceRules[0]);Object.assign(rule,{id:'rule-weekly-incomplete',name:'每周排班待完善',status:'draft'});rule.config.cycle.weekdays[1].shift_id='';attendanceRules.push(rule);
        rule.config.category='职员';delete rule.config.work_schedule;for(const day of rule.config.cycle.weekdays)delete day.candidate_shift_ids;
        const count=()=>recorded.filter(r=>r.path==='/hrm/attendance/v2/rules/rule-weekly-incomplete/apply').length;
        try{
          await cdp.go(origin+base+'hrm/attendance-rules');await cdp.wait("document.querySelector('tbody')?.textContent.includes('每周排班待完善')");
          await cdp.eval("[...document.querySelectorAll('tbody tr')].find(r=>r.textContent.includes('每周排班待完善')).querySelectorAll('button')[1].click()");
          const text=await cdp.eval("document.querySelector('dialog').textContent");assert.match(text,/周二需要出勤，请选择班次/);assert.doesNotMatch(text,/轮班周期第一天/);assert.equal(count(),0);
          await cdp.eval("[...document.querySelectorAll('dialog button')].find(b=>b.textContent==='去完善制度').click();document.querySelector('dialog [name=weekly_shift_1]').value='production-one-normal';var category=document.querySelector('dialog [name=weekly_category_4]');category.value='restday';category.dispatchEvent(new Event('change'))");
          assert.equal(await cdp.eval("document.querySelector('dialog [name=weekly_scheduled_4]').checked"),false);
          assert.equal(await cdp.eval("getComputedStyle(document.querySelector('dialog [name=weekly_scheduled_4]').closest('label')).display"),'flex',"Changing a workday to a restday must reveal the optional attendance control");
          assert.equal(await cdp.eval("document.querySelector('dialog [name=weekly_shift_4]').required"),false);
          await cdp.eval("document.querySelector('dialog [name=complete_workday_minutes]').value=0;document.querySelector('dialog form').requestSubmit()");await cdp.wait("!document.querySelector('dialog')");
          const saved=attendanceRules.find(r=>r.id===rule.id);assert.deepEqual(saved.config.cycle.weekdays[4],{shift_id:'production-one-normal',category:'restday',scheduled_work:false});assert.equal(saved.config.rule.overtime.restday.complete_workday_minutes,0);
          await cdp.eval("[...document.querySelectorAll('tbody tr')].find(r=>r.textContent.includes('每周排班待完善')).querySelectorAll('button')[1].click()");await cdp.wait("document.querySelector('[data-attendance-apply]')?.textContent.includes('适用员工')");await cdp.eval("[...document.querySelectorAll('dialog button')].find(b=>b.textContent==='确认生效并自动计算').click()");await cdp.wait("!document.querySelector('dialog')");assert.equal(count(),1);
        }finally{const index=attendanceRules.findIndex(r=>r.id===rule.id);if(index>=0)attendanceRules.splice(index,1);}
      });
      await t.test("incomplete attendance drafts show field checks before confirmation and can be completed then published", async () => {
        const incomplete = structuredClone(attendanceRules[0]);
        Object.assign(incomplete, {id:'rule-incomplete',name:'生产一部待完善草稿',status:'draft',effective_from:'',effective_to:'',scope:{employee_ids:[],department_ids:[],is_default:false},conflicts:['请确认白天核定工时']});
        incomplete.config.category='职员';delete incomplete.config.work_schedule;
        incomplete.config.cycle={anchor:'',days:[]};attendanceRules.push(incomplete);
        const count=()=>recorded.filter(r=>r.path==='/hrm/attendance/v2/rules/rule-incomplete/apply').length;
        try {
          await cdp.go(origin+base+'hrm/attendance-rules');await cdp.wait("document.querySelector('tbody')?.textContent.includes('生产一部待完善草稿')");
          await cdp.eval("[...document.querySelectorAll('tbody tr')].find(r=>r.textContent.includes('生产一部待完善草稿')).querySelectorAll('button')[1].click()");
          const text=await cdp.eval("document.querySelector('dialog').textContent");
          for(const fragment of ['生效前检查','生效日期','适用范围','周期第一天','排班周期','来源待确认事项']) assert.ok(text.includes(fragment),fragment);
          assert.equal(await cdp.eval("!!document.querySelector('dialog [name=confirmation]')"),false);assert.equal(count(),0);
          await cdp.eval("[...document.querySelectorAll('dialog button')].find(b=>b.textContent==='去完善制度').click()");
          assert.equal(await cdp.eval("document.querySelector('dialog [name=effective_from]').value"),'');
          assert.equal(await cdp.eval("document.querySelector('dialog [name=anchor]').value"),'');
          await cdp.eval(`document.querySelector('dialog [name=effective_from]').value='2026-10-01';document.querySelector('dialog [name=anchor]').value='2026-10-05';document.querySelector('dialog [name=cycle_length]').value='1';document.querySelector('dialog [name=cycle_length]').dispatchEvent(new Event('change'));[...document.querySelectorAll('dialog label')].find(n=>n.textContent.startsWith('第1天')).querySelector('select').value=${JSON.stringify(incomplete.config.shifts[0].id)};var mode=document.querySelector('dialog [name=scope_mode]');mode.value='default';mode.dispatchEvent(new Event('change'));[...document.querySelectorAll('dialog label')].find(n=>n.textContent.includes('已逐项核对并处理上述来源冲突')).querySelector('input').click();document.querySelector('dialog form').requestSubmit()`);
          await cdp.wait("!document.querySelector('dialog')");
          assert.equal(attendanceRules.find(r=>r.id==='rule-incomplete').conflicts.length,0);
          await cdp.eval("[...document.querySelectorAll('tbody tr')].find(r=>r.textContent.includes('生产一部待完善草稿')).querySelectorAll('button')[1].click()");
          await cdp.wait("document.querySelector('[data-attendance-apply]')?.textContent.includes('适用员工')");
          assert.equal(await cdp.eval("!!document.querySelector('dialog [name=confirmation]')"),false);
          await cdp.eval("document.querySelector('dialog [name=reason]').value='';[...document.querySelectorAll('dialog button')].find(b=>b.textContent==='确认生效并自动计算').click()");
          await cdp.wait("document.querySelector('dialog [role=alert]')?.textContent.includes('调整说明')");assert.equal(count(),0);
          await cdp.eval("document.querySelector('dialog [name=reason]').value='核对作息及历史范围';[...document.querySelectorAll('dialog button')].find(b=>b.textContent==='确认生效并自动计算').click()");
          await cdp.wait("!document.querySelector('dialog')");assert.equal(count(),1);assert.equal(attendanceRules.find(r=>r.id==='rule-incomplete').status,'published');
        } finally {const index=attendanceRules.findIndex(r=>r.id==='rule-incomplete');if(index>=0)attendanceRules.splice(index,1);}
      });
      await t.test("roster generation previews before writing and confirmation is submitted once", async () => {
        await cdp.go(origin+base+'hrm/attendance-rosters');await cdp.wait("[...document.querySelectorAll('button')].some(b=>b.textContent==='生成并预览')");
        const before=recorded.filter(r=>r.path==='/hrm/attendance/v2/rosters/generate').length;
        await cdp.eval("[...document.querySelectorAll('button')].find(b=>b.textContent==='生成并预览').click()");
        await cdp.wait("document.querySelector('dialog')?.textContent.includes('候选排班 1 条')");
        assert.equal(recorded.filter(r=>r.path==='/hrm/attendance/v2/rosters/generate').length,before+1);
        assert.equal(recorded.filter(r=>r.path==='/hrm/attendance/v2/rosters/generate').at(-1).body.preview,true);
        await cdp.eval("var confirm=[...document.querySelectorAll('dialog button')].find(b=>b.textContent==='确认保存排班');confirm.click();confirm.click()");
        await cdp.wait("!document.querySelector('dialog')");
        assert.equal(recorded.filter(r=>r.path==='/hrm/attendance/v2/rosters/generate').length,before+2);
        assert.equal(recorded.filter(r=>r.path==='/hrm/attendance/v2/rosters/generate').at(-1).body.preview,false);
      });
      await t.test("candidate shifts require confirmation and local rules do not expose WeCom snapshot actions", async () => {
        await cdp.go(origin+base+'hrm/attendance-rosters');await cdp.wait("[...document.querySelectorAll('button')].some(b=>b.textContent==='按实际卡推荐班次')");
        const before=recorded.filter(r=>r.path==='/hrm/attendance/v2/rosters'&&r.method==='POST').length;
        await cdp.eval("[...document.querySelectorAll('button')].find(b=>b.textContent==='按实际卡推荐班次').click();document.querySelector('dialog [name=employee_id]').value='e1';document.querySelector('dialog form').requestSubmit()");
        await cdp.wait("document.querySelector('dialog')?.textContent.includes('候选白班')");assert.equal(recorded.filter(r=>r.path==='/hrm/attendance/v2/rosters'&&r.method==='POST').length,before);
        await cdp.eval("[...document.querySelectorAll('dialog button')].find(b=>b.textContent==='选择并人工确认').click()");await cdp.wait("document.querySelector('dialog:last-of-type [name=reason]')");
        await cdp.eval("document.querySelector('dialog:last-of-type [name=reason]').value='核对当日打卡后选定';document.querySelector('dialog:last-of-type form').requestSubmit()");await cdp.wait("document.querySelectorAll('dialog').length===0");
        assert.equal(recorded.filter(r=>r.path==='/hrm/attendance/v2/rosters'&&r.method==='POST').length,before+1);
        const beforeSnapshots=recorded.filter(r=>r.path.includes('/checkin-rules/snapshots')||r.path.endsWith('/imports/wecom')).length;
        await cdp.go(origin+base+'hrm/attendance-rules');await cdp.wait("document.querySelector('h1')?.textContent==='考勤设置'");
        assert.equal(await cdp.eval("[...document.querySelectorAll('button')].some(b=>b.textContent.includes('企业微信规则')||b.textContent.includes('保存快照'))"),false);
        assert.equal(recorded.filter(r=>r.path.includes('/checkin-rules/snapshots')||r.path.endsWith('/imports/wecom')).length,beforeSnapshots);
      });
      await t.test("attendance evidence, signed corrections, monthly transitions and ledger entries use structured Chinese forms", async () => {
        await cdp.go(origin+base+'hrm/attendance-results?batch_id=batch-oct');await cdp.wait("document.querySelector('tbody')?.textContent.includes('胡起伟')");
        assert.equal(await cdp.eval("document.querySelector('[name=end_date]').value"),'2026-10-31');
        if (process.env.BIZ_ATTENDANCE_RESULT_SCREENSHOT_PATH) {
          const shot = await cdp.send("Page.captureScreenshot", { format: "png" });
          await writeFile(process.env.BIZ_ATTENDANCE_RESULT_SCREENSHOT_PATH, Buffer.from(shot.data, "base64"));
        }
        await cdp.eval("[...document.querySelectorAll('tbody button')].find(b=>b.textContent==='计算详情').click()");await cdp.wait("document.querySelector('dialog')?.textContent.includes('按已确认制度核定')");
        assert.deepEqual(await cdp.eval("[...document.querySelectorAll('dialog [data-attendance-hours=total] th')].map(e=>e.textContent)"), ['应出勤小时','实际工作小时','核定出勤小时','加班小时']);
        assert.deepEqual(await cdp.eval("[...document.querySelectorAll('dialog [data-attendance-hours=total] tbody td')].map(e=>e.textContent)"), ['8.00','9.25','8.00','2.00']);
        assert.deepEqual(await cdp.eval("[...document.querySelectorAll('dialog [data-attendance-hours=segments] tbody tr')].map(r=>[...r.cells].slice(0,5).map(e=>e.textContent))"), [['正班','8.00','7.00','8.00','0.00'],['加班','0.00','2.25','0.00','2.00']]);
        assert.equal(await cdp.eval("[...document.querySelectorAll('dialog dt')].find(e=>e.textContent==='请假小时')?.nextElementSibling.textContent"),'1.00');
        assert.equal(await cdp.eval("!!document.querySelector('dialog [data-attendance-restday-overtime]')"),false,"Old results must not invent an empty restday breakdown");
        assert.match(await cdp.eval("document.querySelector('dialog').textContent"), /不能相加作为总工时/);
        assert.equal(await cdp.eval("document.querySelector('dialog').getBoundingClientRect().width<=innerWidth&&!!document.querySelector('dialog [data-attendance-hours=segments] .table-wrap')"),true);
        if (process.env.BIZ_ATTENDANCE_DETAIL_SCREENSHOT_PATH) {
          const shot = await cdp.send("Page.captureScreenshot", { format: "png" });
          await writeFile(process.env.BIZ_ATTENDANCE_DETAIL_SCREENSHOT_PATH, Buffer.from(shot.data, "base64"));
        }
        await cdp.eval("[...document.querySelectorAll('dialog button')].find(b=>b.textContent==='人工认定 / 补签').click()");
        await cdp.wait("document.querySelector('dialog:last-of-type [name=at]')");
        await cdp.eval("var d=document.querySelector('dialog:last-of-type');d.querySelector('[name=at]').value='2026-10-01T08:00';d.querySelector('[name=reason]').value='原始打卡遗漏';d.querySelector('[name=evidence]').value='部门负责人核对纸质签字记录';d.querySelector('form').requestSubmit()");
        await cdp.wait("!!document.querySelector('#view tbody')");await delay(100);
        const correction=recorded.find(r=>r.path==='/hrm/attendance/v2/adjustments'&&r.method==='POST').body;
        assert.equal(correction.kind,'punch');assert.equal(correction.at,Date.parse('2026-10-01T08:00:00+08:00')/1000);assert.equal(correction.employee_id,'e1');assert.match(correction.evidence,/纸质签字/);
        await cdp.go(origin+base+'hrm/attendance-months');await cdp.wait("[...document.querySelectorAll('button')].some(b=>b.textContent==='一键月结')");
        await cdp.eval("[...document.querySelectorAll('button')].find(b=>b.textContent==='一键月结').click();document.querySelector('dialog [name=month]').value='2026-10';document.querySelector('dialog [name=reason]').value='全员核对完成'");
        assert.equal(await cdp.eval("!!document.querySelector('dialog [name=batch_id]')"),false);
        await cdp.eval("document.querySelector('dialog form').requestSubmit()");
        await cdp.wait("!document.querySelector('dialog')&&document.querySelector('[data-attendance-month-versions]')?.textContent.includes('已锁定')");
        assert.deepEqual(recorded.filter(r=>r.path==='/hrm/attendance/v2/months/2026-10/close').at(-1).body,{batch_id:'new-batch',reason:'全员核对完成'});
        assert.deepEqual(recorded.filter(r=>r.path==='/hrm/attendance/v2/batches'&&r.method==='POST').at(-1).body,{start_date:'2026-10-01',end_date:'2026-10-31',employee_ids:[]});
        await cdp.eval("[...document.querySelectorAll('tbody button')].find(b=>b.textContent==='重开').click();document.querySelector('dialog [name=reason]').value='重查补签';document.querySelector('dialog form').requestSubmit()");await cdp.wait("!document.querySelector('dialog')&&document.querySelector('[data-attendance-month-versions]')?.textContent.includes('草稿')");
        await cdp.go(origin+base+'hrm/attendance-ledger');await cdp.wait("document.querySelector('[name=employee_id]')");
        await cdp.eval("document.querySelector('[name=employee_id]').value='e1';[...document.querySelectorAll('button')].find(b=>b.textContent==='新增流水').click()");await cdp.wait("document.querySelector('dialog [name=evidence]')");
        await cdp.eval("document.querySelector('dialog [name=minutes]').value='120';document.querySelector('dialog [name=evidence]').value='9月经签字确认余额';document.querySelector('dialog [name=source_ref]').value='期初202610e1';document.querySelector('dialog [name=reason]').value='期初登记';document.querySelector('dialog form').requestSubmit()");await cdp.wait("!document.querySelector('dialog')&&document.querySelector('#view').textContent.includes('可用调休：120 分钟')");
      });
      await t.test("monthly summary is read-only, shows daily pending counts and respects selected month", async () => {
        const originalPermissions=permissions, writes=recorded.filter(r=>r.method==='POST'&&r.path.startsWith('/hrm/attendance')).length;
        try {
          permissions=['biz-suite.hrm.read'];
          await cdp.go(origin+base+'hrm/attendance-months?month=2026-09');
          await cdp.wait("document.querySelector('[data-attendance-month-summary]')?.textContent.includes('张三')");
          assert.match(await cdp.eval("document.querySelector('[data-attendance-month-summary]').textContent"),/2026-09 · 本地试算汇总/);
          assert.equal(await cdp.eval("[...document.querySelectorAll('button')].some(b=>b.textContent==='一键月结'||b.textContent==='重开')"),false);
          assert.deepEqual(await cdp.eval("[...document.querySelectorAll('[data-attendance-month-summary] tbody tr:first-child td')].slice(15,17).map(e=>e.textContent)"),['1','1']);
          assert.deepEqual(await cdp.eval("[...document.querySelectorAll('[data-attendance-month-summary] tbody tr:first-child td')].slice(1,3).map(e=>e.textContent)"),['0033','品质部']);
          assert.deepEqual(await cdp.eval("[...document.querySelectorAll('[data-attendance-month-summary] tbody tr:first-child td')].slice(5,12).map(e=>e.textContent)"),['176.00','179.25','160.00','12.50','4.00','8.50','0.00']);
          assert.match(await cdp.eval("document.querySelector('[data-attendance-month-summary]').textContent"),/以上数字仅为试算/);
          await cdp.eval("var m=document.querySelector('[name=month_summary]');m.value='2026-08';m.dispatchEvent(new Event('change'))");
          await cdp.wait("document.querySelector('[data-attendance-month-summary] h3')?.textContent.includes('2026-08')");
          assert.equal(new URLSearchParams(recorded.filter(r=>r.path==='/hrm/attendance/month').at(-1).query).get('month'),'2026-08');
          assert.equal(recorded.filter(r=>r.method==='POST'&&r.path.startsWith('/hrm/attendance')).length,writes);
          assert.match(await cdp.eval("document.querySelector('[data-attendance-month-versions]').textContent"),/正式版本与月结记录/);
        } finally {permissions=originalPermissions;}
      });
      await t.test("one-click month close calculates the entire leap month, shows progress and prevents duplicate submission", async () => {
        monthlyBatchStates=[{status:'running',completed_count:1,total_count:29},{status:'completed',completed_count:29,total_count:29}];
        const before=recorded.filter(r=>r.path==='/hrm/attendance/v2/batches'&&r.method==='POST').length;
        try {
          await cdp.go(origin+base+'hrm/attendance-months?month=2024-02');await cdp.wait("document.querySelector('[data-attendance-month-summary] h3')?.textContent.includes('2024-02')");
          await cdp.eval("[...document.querySelectorAll('button')].find(b=>b.textContent==='一键月结').click();document.querySelector('dialog [name=reason]').value='闰月核对';var f=document.querySelector('dialog form');f.requestSubmit();f.requestSubmit()");
          await cdp.wait("document.querySelector('[data-attendance-month-progress]')?.textContent.includes('1 / 29')");
          assert.equal(recorded.filter(r=>r.path==='/hrm/attendance/v2/batches'&&r.method==='POST').length,before+1);
          assert.equal(await cdp.eval("!!document.querySelector('dialog [name=batch_id]')"),false);
          await cdp.wait("!document.querySelector('dialog')&&document.querySelector('[data-attendance-month-versions]')?.textContent.includes('2024-02')");
          assert.deepEqual(recorded.filter(r=>r.path==='/hrm/attendance/v2/batches'&&r.method==='POST').at(-1).body,{start_date:'2024-02-01',end_date:'2024-02-29',employee_ids:[]});
          assert.deepEqual(recorded.filter(r=>r.path==='/hrm/attendance/v2/months/2024-02/close').at(-1).body,{batch_id:'new-batch',reason:'闰月核对'});
        } finally {monthlyBatchStates=[];}
      });
      await t.test("month close refuses current and future months before creating tasks", async () => {
        const before=recorded.filter(r=>r.path==='/hrm/attendance/v2/batches'&&r.method==='POST').length;
        await cdp.go(origin+base+'hrm/attendance-months');await cdp.wait("!!document.querySelector('[name=month_summary]')");
        await cdp.eval("[...document.querySelectorAll('button')].find(b=>b.textContent==='一键月结').click();document.querySelector('dialog form').requestSubmit()");
        await cdp.wait("document.querySelector('dialog')?.textContent.includes('当前月份尚未结束')");
        await cdp.eval("document.querySelector('dialog [name=month]').value='2099-12';document.querySelector('dialog form').requestSubmit()");
        assert.equal(recorded.filter(r=>r.path==='/hrm/attendance/v2/batches'&&r.method==='POST').length,before);
        await cdp.eval("document.querySelector('dialog [aria-label=关闭]').click()");
      });
      await t.test("failed or incomplete month calculations never request close", async () => {
        const before=recorded.filter(r=>/\/months\/.+\/close$/.test(r.path)).length;
        try {
          for(const result of [{status:'failed',error_message:'缺少可用排班证据',completed_count:0,total_count:31},{status:'completed',completed_count:30,total_count:31}]) {
            monthlyBatchStates=[result];
            await cdp.go(origin+base+'hrm/attendance-months?month=2025-01');await cdp.wait("document.querySelector('[data-attendance-month-summary] h3')?.textContent.includes('2025-01')");
            await cdp.eval("[...document.querySelectorAll('button')].find(b=>b.textContent==='一键月结').click();document.querySelector('dialog form').requestSubmit()");
            await cdp.wait("!!document.querySelector('dialog [role=alert]')");
            assert.match(await cdp.eval("document.querySelector('dialog [role=alert]').textContent"),result.status==='failed'?/缺少可用排班证据/:/计算不完整/);
            await cdp.eval("document.querySelector('dialog [aria-label=关闭]').click()");
          }
          assert.equal(recorded.filter(r=>/\/months\/.+\/close$/.test(r.path)).length,before);
        } finally {monthlyBatchStates=[];}
      });
      await t.test("closing a pending monthly calculation dialog does not later lock the month", async () => {
        monthlyBatchStates=[{status:'running',completed_count:1,total_count:31},{status:'completed',completed_count:31,total_count:31}];
        const before=recorded.filter(r=>r.path==='/hrm/attendance/v2/months/2025-05/close').length;
        try {
          await cdp.go(origin+base+'hrm/attendance-months?month=2025-05');await cdp.wait("document.querySelector('[data-attendance-month-summary] h3')?.textContent.includes('2025-05')");
          await cdp.eval("[...document.querySelectorAll('button')].find(b=>b.textContent==='一键月结').click();document.querySelector('dialog form').requestSubmit()");
          await cdp.wait("document.querySelector('[data-attendance-month-progress]')?.textContent.includes('1 / 31')");
          await cdp.eval("document.querySelector('dialog [aria-label=关闭]').click()");
          await delay(1650);
          assert.equal(recorded.filter(r=>r.path==='/hrm/attendance/v2/months/2025-05/close').length,before);
          assert.equal(attendanceMonths.some(row=>row.month==='2025-05'),false);
        } finally {monthlyBatchStates=[];}
      });
      await t.test("month review rejection remains visible and retry recalculates fresh inputs", async () => {
        monthlyCloseError='存在未复核异常，不能月结';
        const before=recorded.filter(r=>r.path==='/hrm/attendance/v2/batches'&&r.method==='POST').length;
        try {
          await cdp.go(origin+base+'hrm/attendance-months?month=2025-03');await cdp.wait("document.querySelector('[data-attendance-month-summary] h3')?.textContent.includes('2025-03')");
          await cdp.eval("[...document.querySelectorAll('button')].find(b=>b.textContent==='一键月结').click();document.querySelector('dialog form').requestSubmit()");
          await cdp.wait("document.querySelector('dialog [role=alert]')?.textContent.includes('存在未复核异常')");
          assert.equal(attendanceMonths.some(row=>row.month==='2025-03'),false);
          await cdp.eval("document.querySelector('dialog form').requestSubmit()");
          await cdp.wait("document.querySelector('dialog [role=alert]')?.textContent.includes('存在未复核异常')");
          assert.equal(recorded.filter(r=>r.path==='/hrm/attendance/v2/batches'&&r.method==='POST').length,before+2);
          await cdp.eval("document.querySelector('dialog [aria-label=关闭]').click()");
        } finally {monthlyCloseError='';}
      });
      await t.test("restday overtime details separate actual white-shift work, confirmed eight hours and additional overtime", async () => {
        const original=structuredClone(attendanceResult.result);
        try{
          attendanceResult.result={...original,expected_seconds:0,credited_seconds:0,actual_seconds:39600,overtime_seconds:39600,overtime_buckets:[{date:'2026-10-03',category:'restday',raw_seconds:38700,seconds:39600,workday_actual_seconds:27900,workday_credited_seconds:28800,additional_seconds:10800,complete_workday_applied:true}]};
          await cdp.go(origin+base+'hrm/attendance-results');await cdp.wait("[...document.querySelectorAll('tbody button')].some(b=>b.textContent==='计算详情')");
          await cdp.eval("[...document.querySelectorAll('tbody button')].find(b=>b.textContent==='计算详情').click()");await cdp.wait("document.querySelector('dialog [data-attendance-restday-overtime]')");
          assert.deepEqual(await cdp.eval("[...document.querySelectorAll('dialog [data-attendance-restday-overtime] thead th')].map(e=>e.textContent)"),['日期','白班实际小时','白班核定加班小时','白班核定差额小时','其他时段加班小时','加班合计小时','完整白班认定']);
          assert.deepEqual(await cdp.eval("[...document.querySelectorAll('dialog [data-attendance-restday-overtime] tbody td')].map(e=>e.textContent)"),['2026-10-03','7.75','8.00','0.25','3.00','11.00','已应用']);
          assert.match(await cdp.eval("document.querySelector('dialog [data-attendance-restday-overtime]').textContent"),/部分白班保留有效实际时间/);
        }finally{attendanceResult.result=original;}
      });
      await t.test("baseline import preserves source evidence, maps explicitly and reports unverifiable comparisons", async () => {
        await cdp.go(origin+base+'hrm/attendance-imports');await cdp.wait("document.querySelector('tbody')?.textContent.includes('9月考勤数据.xlsx')");
        await cdp.eval("[...document.querySelectorAll('tbody button')].find(b=>b.textContent==='预览 / 关联').click()");await cdp.wait("document.querySelector('dialog')?.textContent.includes('工号缺失')");
        await cdp.eval("[...document.querySelectorAll('dialog button')].find(b=>b.textContent==='查看日格').click()");await cdp.wait("document.querySelector('dialog:last-of-type')?.textContent.includes('原公式')");
        assert.match(await cdp.eval("document.querySelector('dialog:last-of-type').textContent"),/E43.*人工核定依据.*SUM/s);assert.equal(await cdp.eval("window.attXss"),undefined);
        await cdp.eval("document.querySelector('dialog:last-of-type .close').click();[...document.querySelectorAll('dialog button')].find(b=>b.textContent==='关联员工').click()");await cdp.wait("document.querySelector('dialog:last-of-type [name=employee_id]')");
        await cdp.eval("document.querySelector('dialog:last-of-type [name=employee_id]').value='e1';document.querySelector('dialog:last-of-type form').requestSubmit()");await cdp.wait("document.querySelectorAll('dialog').length===1");assert.equal(attendanceMappings['9月职员!B43'],'e1');
        await cdp.eval("document.querySelector('dialog .close').click();[...document.querySelectorAll('tbody button')].find(b=>b.textContent==='对账').click();document.querySelector('dialog [name=batch_id]').value='batch-oct';document.querySelector('dialog form').requestSubmit()");await cdp.wait("document.querySelector('dialog')?.textContent.includes('原始证据不足')");
        assert.match(await cdp.eval("document.querySelector('dialog').textContent"),/192.*8.*没有完整月份/s);
      });
      await t.test("attendance v2 read-only users see source evidence without mutation controls and scoped users cannot open global rules", async () => {
        const previous=permissions;
        try{permissions=['biz-suite.hrm.read'];await cdp.go(origin+base+'hrm/attendance-rules');await cdp.wait("document.querySelector('tbody')");assert.equal(await cdp.eval("[...document.querySelectorAll('#view button')].some(b=>['新增制度','发布','编辑 / 新版本'].includes(b.textContent))"),false);
        await cdp.go(origin+base+'hrm/attendance-imports');await cdp.wait("document.querySelector('tbody')");assert.equal(await cdp.eval("!!document.querySelector('input[type=file]')"),false);
        permissions=['biz-suite.hrm.read.self'];const before=recorded.filter(r=>r.path==='/hrm/attendance/v2/rules').length;await cdp.go(origin+base+'hrm/attendance-rules');await cdp.wait("document.querySelector('#view')?.textContent.includes('无访问权限')");assert.equal(recorded.filter(r=>r.path==='/hrm/attendance/v2/rules').length,before);
        }finally{permissions=previous;}
      });
      await t.test("automatic WeCom schedule displays the server's four Beijing times without settings or background sync writes", async () => {
        const writes=()=>recorded.filter(r=>r.method==='POST'&&(r.path==='/wecom/settings'||r.path.startsWith('/wecom/sync/'))).length;
        const before=writes();
        await cdp.go(origin+base+'wecom/status');await cdp.wait("document.querySelector('[data-schedule-next]')?.textContent==='2026-10-29 23:59:00'");
        assert.equal(await cdp.eval("document.querySelector('[data-schedule-times]').textContent"),'08:30、10:00、14:30、23:59');
        assert.equal(await cdp.eval("document.querySelector('[data-schedule-timezone]').textContent"),'北京时间（Asia/Shanghai，UTC+8）');
        const text=await cdp.eval("document.querySelector('[data-automatic-schedule]').textContent");
        for(const fragment of ['无需另行保存设置','服务启动不触发自动同步','错过的同步时点不补跑','同步批次可能持续一段时间','手动同步'])assert.ok(text.includes(fragment),fragment);
        assert.equal(await cdp.eval("document.querySelectorAll('[data-automatic-schedule] input,[data-automatic-schedule] select,[data-automatic-schedule] button').length"),0);
        assert.equal(await cdp.eval("!!document.querySelector('#wecom-settings [data-automatic-schedule]')"),false);
        assert.equal(writes(),before);
        try{
          automaticSchedule={...automaticSchedule,next_run_at:'2026-10-30 08:30:00'};
          await cdp.eval("[...document.querySelectorAll('#view button')].find(b=>b.textContent==='刷新').click()");await cdp.wait("document.querySelector('[data-schedule-next]')?.textContent==='2026-10-30 08:30:00'");
          assert.equal(writes(),before,"Refreshing the plan must not save settings or start a sync");
          automaticSchedule=undefined;
          await cdp.eval("[...document.querySelectorAll('#view button')].find(b=>b.textContent==='刷新').click()");await cdp.wait("document.querySelector('[data-automatic-schedule]')?.textContent.includes('暂未取得服务器自动同步计划')");
          assert.equal(await cdp.eval("!!document.querySelector('[data-schedule-next]')"),false,"Unavailable schedule must not show a stale next run");
          assert.equal(await cdp.eval("document.querySelectorAll('[data-sync-kind]').length"),6,"Manual sync actions remain available");
        }finally{automaticSchedule={timezone:'Asia/Shanghai',times:['08:30','10:00','14:30','23:59'],next_run_at:'2026-10-29 23:59:00',startup_sync:false,catch_up:false};}
      });
      await t.test(
        "manual sync submits explicit Shanghai time window",
        async () => {
          await cdp.go(origin + base + "wecom/status");
          await cdp.wait(
            "document.querySelector('[data-sync-kind=checkin]') && !document.querySelector('[data-sync-kind=checkin]').disabled"
          );
          await cdp.eval(
            "document.querySelector('[name=start_time]').value='2026-10-01T08:00';document.querySelector('[name=end_time]').value='2026-10-02T18:00';document.querySelector('[data-sync-kind=checkin]').click()"
          );
          await cdp.wait(
            "document.getElementById('toast').textContent.includes('run1')"
          );
          assert.equal(
            recorded.find((r) => r.path === "/wecom/sync/checkin").body
              .start_time,
            "2026-10-01 08:00:00"
          );
        }
      );
      await t.test(
        "WeCom separates configured credentials from four read-only diagnostic results",
        async () => {
          await cdp.go(origin + base + "wecom/status");
          await cdp.wait("document.querySelector('[data-credential=app]')?.textContent.includes('WECOM_APP_SECRET')");
          assert.equal(await cdp.eval("document.querySelectorAll('[data-diagnostic-kind]').length"), 4);
          assert.equal(await cdp.eval("[...document.querySelectorAll('[data-diagnostic-result]')].every(n=>n.textContent.includes('未验证'))"), true);
          assert.equal(await cdp.eval("document.querySelectorAll('#wecom-settings input[type=password][autocomplete=new-password]').length"), 2);
          for (const [kind, expected] of [["contact", "部分可用"], ["app", "成功"], ["checkin", "失败"], ["approval", "未验证 · 缺少配置"]]) {
            await cdp.eval(`document.querySelector('[data-diagnostic-kind=${kind}]').click()`);
            await cdp.wait(`document.querySelector('[data-diagnostic-result=${kind}]').textContent.includes('2026-10-28 18:00:00')`);
            assert.match(await cdp.eval(`document.querySelector('[data-diagnostic-result=${kind}]').textContent`), new RegExp(expected));
            const request = recorded.find((r) => r.method === "POST" && r.path === "/wecom/diagnostics/" + kind);
            assert.deepEqual(request.body, {}, "diagnostics use saved server credentials");
          }
          assert.match(await cdp.eval("document.querySelector('[data-diagnostic-result=contact]').textContent"), /48009/);
          assert.match(await cdp.eval("document.querySelector('[data-diagnostic-result=app]').textContent"), /可见部门 ID2、4/);
          assert.equal(await cdp.eval("document.body.textContent.includes('DO_NOT_RENDER')"), false);
          assert.equal(await cdp.eval("document.querySelectorAll('.diagnostic-output img').length"), 0);
          assert.equal(await cdp.eval("window.xss"), undefined);
          const requestsBefore = recorded.filter((r) => r.path.startsWith("/wecom/diagnostics/")).length;
          await cdp.eval("const input=document.querySelector('[name=agent_id]');input.value='124';input.dispatchEvent(new Event('input',{bubbles:true}));document.querySelector('[data-diagnostic-kind=app]').click()");
          assert.equal(await cdp.eval("[...document.querySelectorAll('[data-diagnostic-kind],[data-sync-kind]')].every(b=>b.disabled)"), true);
          assert.equal(await cdp.eval("[...document.querySelectorAll('[data-diagnostic-result]')].every(n=>n.textContent.includes('未验证'))"), true);
          assert.equal(recorded.filter((r) => r.path.startsWith("/wecom/diagnostics/")).length, requestsBefore);
        }
      );
      await t.test(
        "WeCom secrets are write-only, blank preserves and explicit clearing falls back to environment",
        async () => {
          await cdp.go(origin + base + "wecom/status");
          await cdp.wait("document.querySelector('[name=app_secret]')");
          await cdp.eval(`document.querySelector('[name=contact_secret]').value='fixture-contact-secret';const app=document.querySelector('[name=app_secret]');app.value='fixture-app-secret';app.dispatchEvent(new Event('input',{bubbles:true}));document.querySelector('#wecom-settings').requestSubmit()`);
          await cdp.wait("document.querySelector('[data-credential=app]')?.textContent.includes('加密保存')");
          assert.equal(await cdp.eval("document.querySelector('[name=contact_secret]').value"), "");
          assert.equal(await cdp.eval("document.querySelector('[name=app_secret]').value"), "");
          assert.equal("contact_secret" in wecomSettings, false);
          assert.equal("app_secret" in wecomSettings, false);
          assert.equal(savedCredentials.contact, true);
          assert.equal(savedCredentials.app, true);
          await cdp.eval("document.querySelector('#wecom-settings').requestSubmit()");
          await cdp.wait("!document.querySelector('#wecom-settings button[type=submit]').disabled");
          const blank = recorded.filter((r) => r.method === "POST" && r.path === "/wecom/settings").at(-1);
          assert.equal("contact_secret" in blank.body, false);
          assert.equal("app_secret" in blank.body, false);
          assert.equal(savedCredentials.contact, true);
          assert.equal(savedCredentials.app, true);
          await cdp.eval("document.querySelector('[name=clear_contact_secret]').click();document.querySelector('[name=clear_app_secret]').click();document.querySelector('#wecom-settings').requestSubmit()");
          await cdp.wait("!document.querySelector('#wecom-settings button[type=submit]').disabled && document.querySelector('[data-credential=app]').textContent.includes('服务器环境')");
          assert.equal(savedCredentials.contact, false);
          assert.equal(savedCredentials.app, false);
          const cleared = recorded.filter((r) => r.method === "POST" && r.path === "/wecom/settings").at(-1);
          assert.equal(cleared.body.clear_contact_secret, true);
          assert.equal(cleared.body.clear_app_secret, true);
          assert.equal(await cdp.eval("document.querySelector('[name=clear_app_secret]').checked"), false);
          await cdp.go(origin + base + "wecom/status");
          await cdp.wait("document.querySelector('[name=app_secret]')");
          assert.equal(await cdp.eval("document.querySelector('[name=app_secret]').value"), "");
        }
      );

  await t.test(
    "Chinese department and employee name rules save before the first directory sync and survive refresh",
    async () => {
      const previousSettings = structuredClone(wecomSettings);
      const previousChoices = filterChoices;
      filterChoices = { departments: [], users: [] };
      wecomSettings = {
        ...wecomSettings,
        include_department_ids: [],
        exclude_department_ids: [],
        include_userids: [],
        exclude_userids: [],
        include_department_names: [],
        exclude_department_names: [],
        include_user_names: [],
        exclude_user_names: [],
      };
      try {
        await cdp.go(origin + base + "wecom/status");
        await cdp.wait("document.querySelector('[name=exclude_department_names]')");
        assert.match(await cdp.eval("document.querySelector('[data-name-filter=user]').textContent"), /也可以先填写中文姓名/);
        await cdp.eval(`
          document.querySelector('[name=exclude_department_names]').value='分子公司\\n新入职员工';
          const names=document.querySelector('[name=exclude_user_names]');
          names.value=' 段详政，段详政\\nMary Ann';
          names.dispatchEvent(new Event('input',{bubbles:true}));
          document.querySelector('[name=include_subdepartments]').checked=true;
          document.querySelector('#wecom-settings').requestSubmit();
        `);
        await cdp.wait("!document.querySelector('#wecom-settings button[type=submit]').disabled && document.querySelector('#wecom-settings').textContent.includes('配置已保存')");
        assert.deepEqual(wecomSettings.exclude_department_names, ["分子公司", "新入职员工"]);
        assert.deepEqual(wecomSettings.exclude_user_names, ["段详政", "Mary Ann"]);
        assert.deepEqual(wecomSettings.exclude_userids, []);
        assert.deepEqual(wecomSettings.exclude_department_ids, []);
        assert.equal(wecomSettings.include_subdepartments, true);
        await cdp.go(origin + base + "wecom/status");
        await cdp.wait("document.querySelector('[name=exclude_user_names]')?.value.includes('段详政')");
        assert.equal(await cdp.eval("document.querySelector('[name=exclude_department_names]').value"), "分子公司\n新入职员工");
        assert.equal(await cdp.eval("document.querySelector('[name=exclude_user_names]').value"), "段详政\nMary Ann");
        assert.equal(await cdp.eval("document.querySelector('[name=include_subdepartments]').checked"), true);
        await cdp.eval("document.querySelector('[name=include_subdepartments]').click();document.querySelector('#wecom-settings').requestSubmit()");
        await cdp.wait("!document.querySelector('#wecom-settings button[type=submit]').disabled && document.querySelector('#wecom-settings').textContent.includes('配置已保存')");
        assert.equal(wecomSettings.include_subdepartments, false);
        await cdp.go(origin + base + "wecom/status");
        await cdp.wait("document.querySelector('[name=exclude_user_names]')?.value.includes('段详政')");
        assert.equal(await cdp.eval("document.querySelector('[name=include_subdepartments]').checked"), false);
      } finally {
        wecomSettings = previousSettings;
        filterChoices = previousChoices;
      }
    }
  );
  await t.test(
    "Duplicate names select a specific employee by department or a specific department by hierarchy",
    async () => {
      const previousSettings = structuredClone(wecomSettings);
      const previousChoices = filterChoices;
      filterChoices = {
        ...filterChoices,
        departments: [...filterChoices.departments, { wecom_id: 6, name: "分子公司", parent_id: 2 }],
      };
      try {
        wecomSettings = {
          ...wecomSettings,
          exclude_userids: [], exclude_user_names: [],
          exclude_department_ids: [], exclude_department_names: [],
        };
        await cdp.go(origin + base + "wecom/status");
        await cdp.wait("document.querySelector('[data-name-filter=user] select')");
        assert.match(await cdp.eval("document.querySelector('[data-name-filter=user]').textContent"), /同名人员都会按名称规则处理/);
        assert.equal(await cdp.eval("[...document.querySelectorAll('[data-id-filter]')].every(n=>!n.open)"), true);
        await cdp.eval(`
          document.querySelector('[data-name-filter=user] .filter-picker').open=true;
          const search=document.querySelector('[aria-label="搜索可选人员"]');
          search.value='段详政';search.dispatchEvent(new Event('input',{bubbles:true}));
        `);
        assert.equal(await cdp.eval("document.querySelector('[aria-label=\"可选人员\"]').options.length"), 2);
        const labels = await cdp.eval("[...document.querySelector('[aria-label=\"可选人员\"]').options].map(o=>o.textContent)");
        assert.ok(labels.some((text) => text.startsWith("段详政 · 研发部")));
        assert.ok(labels.some((text) => text.startsWith("段详政 · 业务部")));
        const departmentLabels = await cdp.eval("[...document.querySelector('[aria-label=\"可选部门\"]').options].filter(o=>o.textContent.startsWith('分子公司')).map(o=>o.textContent)");
        assert.deepEqual(departmentLabels, [
          "分子公司 · 总公司 · 部门：3",
          "分子公司 · 总公司 / 研发部 · 部门：6",
        ]);
        await cdp.eval(`
          const select=document.querySelector('[aria-label="可选人员"]');
          for(const option of select.options)option.selected=option.value==='duan2';
          document.querySelector('[data-filter-action=user-exclude]').click();
          document.querySelector('[data-name-filter=department] .filter-picker').open=true;
          const departments=document.querySelector('[aria-label="可选部门"]');
          for(const option of departments.options)option.selected=option.value==='3';
          document.querySelector('[data-filter-action=department-exclude]').click();
          document.querySelector('#wecom-settings').requestSubmit();
        `);
        await cdp.wait("!document.querySelector('#wecom-settings button[type=submit]').disabled && document.querySelector('#wecom-settings').textContent.includes('配置已保存')");
        assert.deepEqual(wecomSettings.exclude_userids, ["duan2"]);
        assert.deepEqual(wecomSettings.exclude_user_names, []);
        assert.deepEqual(wecomSettings.exclude_department_ids, [3]);
        assert.deepEqual(wecomSettings.exclude_department_names, []);
        await cdp.go(origin + base + "wecom/status");
        await cdp.wait("document.querySelector('[data-bound-id=duan2]')");
        assert.match(await cdp.eval("document.querySelector('[data-bound-id=duan2]').textContent"), /段详政业务部/);
        assert.equal(await cdp.eval("!!document.querySelector('[data-bound-id=duan1]')"), false);
        assert.match(await cdp.eval("document.querySelector('[data-name-filter=department] [data-bound-id=\"3\"]').textContent"), /分子公司/);
        assert.equal(await cdp.eval("!!document.querySelector('[data-name-filter=department] [data-bound-id=\"6\"]')"), false);
      } finally {
        wecomSettings = previousSettings;
        filterChoices = previousChoices;
      }
    }
  );
  await t.test(
    "Legacy person and department IDs remain intact when names cannot be resolved",
    async () => {
      const previousSettings = structuredClone(wecomSettings);
      try {
        wecomSettings = {
          ...wecomSettings,
          include_userids: ["alice", "legacy-unresolved"],
          exclude_userids: [],
          include_department_ids: [2, 999],
          exclude_department_ids: [],
          include_user_names: [],
          exclude_user_names: ["段详政"],
          include_department_names: [],
          exclude_department_names: ["分子公司", "新入职员工"],
        };
        await cdp.go(origin + base + "wecom/status");
        await cdp.wait("document.querySelector('[data-bound-id=legacy-unresolved]')");
        assert.match(await cdp.eval("document.querySelector('[data-bound-id=alice]').textContent"), /Alice研发部/);
        assert.match(await cdp.eval("document.querySelector('[data-bound-id=legacy-unresolved]').textContent"), /姓名待解析/);
        assert.match(await cdp.eval("document.querySelector('[data-name-filter=department] [data-bound-id=\"999\"]').textContent"), /部门名称待解析/);
        assert.equal(await cdp.eval("[...document.querySelectorAll('[data-id-filter]')].every(n=>!n.open)"), true);
        await cdp.eval("document.querySelector('#wecom-settings').requestSubmit()");
        await cdp.wait("!document.querySelector('#wecom-settings button[type=submit]').disabled && document.querySelector('#wecom-settings').textContent.includes('配置已保存')");
        assert.deepEqual(wecomSettings.include_userids, ["alice", "legacy-unresolved"]);
        assert.deepEqual(wecomSettings.include_department_ids, [2, 999]);
        assert.deepEqual(wecomSettings.exclude_user_names, ["段详政"]);
        assert.deepEqual(wecomSettings.exclude_department_names, ["分子公司", "新入职员工"]);
      } finally {
        wecomSettings = previousSettings;
      }
    }
  );

      await t.test(
        "WeCom configuration persists and six buttons target distinct endpoints",
        async () => {
          await cdp.go(origin + base + "wecom/status");
          await cdp.wait(
            "document.querySelector('#wecom-settings [name=corp_id]')"
          );
          await cdp.eval(
            `document.querySelector('[name=corp_id]').value='wwTest';document.querySelector('[name=agent_id]').value='123';document.querySelector('[name=include_department_ids]').value='2,4';document.querySelector('[name=exclude_department_ids]').value='3';document.querySelector('[name=include_userids]').value='alice,bob';document.querySelector('[name=exclude_userids]').value='bob';document.querySelector('[name=exclude_userids]').dispatchEvent(new Event('input',{bubbles:true}));`
          );
          assert.equal(
            await cdp.eval(
              "[...document.querySelectorAll('[data-sync-kind]')].every(b=>b.disabled)"
            ),
            true
          );
          await cdp.eval(
            "document.querySelector('#wecom-settings').requestSubmit()"
          );
          await cdp.wait(
            "document.querySelector('#wecom-settings').textContent.includes('配置已保存')"
          );
          assert.deepEqual(wecomSettings.include_department_ids, [2, 4]);
          assert.deepEqual(wecomSettings.exclude_userids, ["bob"]);
          assert.equal(wecomSettings.corp_id, "wwTest");
          assert.equal("secret" in wecomSettings, false);
          for (const kind of ["out", "trip", "overtime"])
            templates.push({
              template_id: kind + "-tpl",
              approval_type: kind,
              enabled: 1,
            });
          await cdp.go(origin + base + "wecom/status");
          await cdp.wait(
            "document.querySelector('[data-sync-kind=trip]') && !document.querySelector('[data-sync-kind=trip]').disabled"
          );
          assert.equal(
            await cdp.eval("document.querySelector('[name=corp_id]').value"),
            "wwTest"
          );
          await cdp.eval(
            `const search=document.querySelector('[aria-label="搜索可选部门"]');search.value='研发';search.dispatchEvent(new Event('input',{bubbles:true}));`
          );
          assert.equal(
            await cdp.eval(
              "document.querySelector('[data-sync-kind=contact]').disabled"
            ),
            false,
            "searching options must not mark settings dirty"
          );
          for (const kind of ["contact", "checkin", "leave", "out", "trip", "overtime"]) {
            const previous = recorded.filter(
              (r) => r.path === "/wecom/sync/" + kind && r.method === "POST"
            ).length;
            await cdp.eval(
              `document.querySelector('[data-sync-kind=${kind}]').click()`
            );
            await cdp.wait(
              `!document.querySelector('[data-sync-kind=${kind}]').disabled`
            );
            assert.equal(
              recorded.filter(
                (r) => r.path === "/wecom/sync/" + kind && r.method === "POST"
              ).length,
              previous + 1
            );
          }
          assert.equal(
            await cdp.eval(
              "document.querySelectorAll('[data-sync-kind]').length"
            ),
            6
          );
        }
      );
      await t.test(
        "WeCom read/write users can sync attendance and approvals but cannot trigger contact or organization provisioning",
        async () => {
          const previousPermissions = permissions;
          permissions = ["biz-suite.wecom.read", "biz-suite.wecom.write"];
          const contactBefore = recorded.filter((r) => r.method === "POST" && r.path === "/wecom/sync/contact").length;
          const organizationBefore = recorded.filter((r) => r.path === "/wecom/organization/sync").length;
          try {
            await cdp.go(origin + base + "wecom/status");
            await cdp.wait("document.querySelector('[data-sync-kind=checkin]') && !document.querySelector('[data-sync-kind=checkin]').disabled");
            assert.deepEqual(await cdp.eval("[...document.querySelectorAll('[data-sync-kind]')].map(b=>b.dataset.syncKind)"), ["checkin", "leave", "out", "trip", "overtime"]);
            assert.equal(await cdp.eval("document.querySelector('[data-sync-kind=contact]')"), null);
            assert.equal(await cdp.eval("document.querySelectorAll('[data-diagnostic-kind]').length"), 0);
            assert.equal(await cdp.eval("[...document.querySelectorAll('button')].some(b=>b.textContent==='保存配置')"), false);
            await cdp.eval("document.querySelector('[name=start_time]').value='2026-10-01T08:00';document.querySelector('[name=end_time]').value='2026-10-02T18:00'");
            for (const kind of ["checkin", "leave", "out", "trip", "overtime"]) {
              const count = () => recorded.filter((r) => r.method === "POST" && r.path === "/wecom/sync/" + kind).length;
              const before = count();
              await cdp.eval(`document.querySelector('[data-sync-kind=${kind}]').click()`);
              await cdp.wait(`!document.querySelector('[data-sync-kind=${kind}]').disabled`);
              assert.equal(count(), before + 1, kind + " synchronization remains available with write permission");
              const request = recorded.filter((r) => r.method === "POST" && r.path === "/wecom/sync/" + kind).at(-1);
              assert.equal(request.body.start_time, "2026-10-01 08:00:00");
              assert.equal(request.body.end_time, "2026-10-02 18:00:00");
            }
            assert.equal(recorded.filter((r) => r.method === "POST" && r.path === "/wecom/sync/contact").length, contactBefore);
            await cdp.go(origin + base + "wecom/departments");
            await cdp.wait("document.querySelector('table')");
            assert.equal(await cdp.eval("[...document.querySelectorAll('#view button')].some(b=>['设置映射','同步组织与账号'].includes(b.textContent))"), false);
            assert.equal(recorded.filter((r) => r.path === "/wecom/organization/sync").length, organizationBefore);
          } finally {
            permissions = previousPermissions;
          }
        }
      );
      await t.test(
        "all navigation routes render without runtime errors",
        async () => {
          for (const path of [
            "overview",
            "hrm/holidays",
            "hrm/clocks",
            "hrm/month",
            "hrm/leaves",
            "hrm/outs",
            "hrm/trips",
            "wecom/departments",
            "wecom/exceptions",
            "im/directory",
            "im/groups",
            "im/channels",
            "im/face-packs",
            "im/sensitive-words",
            "im/manager",
          ]) {
            await cdp.go(origin + base + path);
            await delay(100);
            assert.equal(
              await cdp.eval("!!document.querySelector('.error-text')"),
              false,
              path
            );
            const workspace = path.startsWith("im/") ? "im" : "hrm";
            assert.equal(await cdp.eval("document.querySelector('.layout').dataset.workspace"), workspace, path);
            assert.equal(await cdp.eval("document.querySelectorAll('.sidebar').length"), 1, path);
            assert.equal(await cdp.eval("document.querySelectorAll('iframe').length"), 0, path);
            const navPaths = await cdp.eval("[...document.querySelectorAll('.sidebar .nav-link')].map(a=>a.dataset.route)");
            assert.ok(navPaths.every((route) => workspace === "im" ? route.startsWith("im/") : /^(hrm|wecom)\//.test(route)), path);
          }
        }
      );
      await t.test(
        "HRM and IM have independent branding and navigation that follow switching and browser history",
        async () => {
          await cdp.go(origin + base + "hrm");
          await cdp.wait("document.querySelector('h1')?.textContent==='智慧HRM'");
          assert.equal(await cdp.eval("location.pathname"), base + "hrm/overview");
          assert.equal(await cdp.eval("document.querySelector('.brand strong').textContent"), "智慧HRM");
          assert.match(await cdp.eval("document.title"), /智慧HRM/);
          assert.equal(await cdp.eval("document.querySelector('.workspace-return').getAttribute('href')"), "/suxinweb/");
          assert.equal(await cdp.eval("document.querySelector('[data-workspace-link=im]').getAttribute('href')"), base + "im");
          assert.equal(await cdp.eval("document.querySelector('.sidebar').textContent.includes('企业即时通讯')"), false);
          assert.equal(await cdp.eval("document.querySelector('#view').textContent.includes('打开消息中心')"), false);
          await cdp.eval("document.querySelector('[data-workspace-link=im]').click()", true);
          await cdp.wait("document.querySelector('h1')?.textContent==='消息中心'");
          assert.equal(await cdp.eval("location.pathname"), base + "im/chat");
          assert.equal(await cdp.eval("document.querySelector('.brand strong').textContent"), "企业即时通讯");
          assert.equal(await cdp.eval("document.querySelector('#workspace-name').textContent"), "企业即时通讯");
          assert.equal(await cdp.eval("document.title"), "消息中心 · 企业即时通讯");
          assert.equal(await cdp.eval("document.querySelector('.sidebar').textContent.includes('打卡记录')"), false);
          assert.equal(await cdp.eval("document.querySelector('.sidebar').textContent.includes('企业微信')"), false);
          assert.equal(await cdp.eval("document.querySelector('[data-workspace-link=hrm]').getAttribute('href')"), base + "hrm");
          await cdp.eval("document.querySelector('[data-workspace-link=hrm]').click()", true);
          await cdp.wait("document.querySelector('h1')?.textContent==='智慧HRM'");
          await cdp.eval("document.querySelector('[data-route=\"wecom/status\"]').click()", true);
          await cdp.wait("document.querySelector('#wecom-settings')");
          assert.equal(await cdp.eval("document.querySelector('.brand strong').textContent"), "智慧HRM");
          assert.equal(await cdp.eval("document.querySelector('.nav-link.active').dataset.route"), "wecom/status");
          await cdp.eval("history.back()");
          await cdp.wait("location.pathname.endsWith('/hrm/overview') && document.querySelector('h1')?.textContent==='智慧HRM'");
          await cdp.eval("history.back()");
          await cdp.wait("location.pathname.endsWith('/im/chat') && document.querySelector('.brand strong')?.textContent==='企业即时通讯'");
          await cdp.eval("history.forward()");
          await cdp.wait("location.pathname.endsWith('/hrm/overview') && document.querySelector('.brand strong')?.textContent==='智慧HRM'");
          assert.equal(await cdp.eval("document.querySelectorAll('.sidebar').length"), 1);
        }
      );
      await t.test(
        "unknown routes recover inside their workspace and slow HRM data cannot leak into IM",
        async () => {
          for (const [unknown, brand, heading] of [["im/missing", "企业即时通讯", "消息中心"], ["wecom/missing", "智慧HRM", "智慧HRM"]]) {
            await cdp.go(origin + base + unknown);
            await cdp.wait("document.querySelector('h1')?.textContent==='页面不存在'");
            assert.equal(await cdp.eval("document.querySelector('.brand strong').textContent"), brand);
            await cdp.eval(`[...document.querySelectorAll('button')].find(b=>b.textContent===${JSON.stringify("返回" + brand)}).click()`);
            await cdp.wait(`document.querySelector('h1')?.textContent===${JSON.stringify(heading)}`);
          }
          summaryDelay = 350;
          try {
            await cdp.go(origin + base + "overview");
            await cdp.wait("document.querySelector('[data-workspace-link=im]')");
            await cdp.eval("document.querySelector('[data-workspace-link=im]').click()");
            await cdp.wait("document.querySelector('.chat-layout')");
            await delay(450);
            assert.equal(await cdp.eval("document.querySelector('h1').textContent"), "消息中心");
            assert.equal(await cdp.eval("document.querySelector('#view').textContent.includes('工作入口')"), false);
            assert.equal(await cdp.eval("document.querySelectorAll('#view .cards').length"), 0);
          } finally { summaryDelay = 0; }
          hrmOptionsDelay = 350;
          try {
            await cdp.go(origin + base + "hrm/groups");
            await cdp.wait("document.querySelector('[data-workspace-link=im]')");
            await cdp.eval("document.querySelector('[data-workspace-link=im]').click()");
            await cdp.wait("document.querySelector('.chat-layout')");
            await delay(450);
            assert.equal(await cdp.eval("document.querySelector('h1').textContent"), "消息中心");
            assert.equal(await cdp.eval("document.querySelector('.brand strong').textContent"), "企业即时通讯");
            assert.equal(await cdp.eval("document.querySelector('#view').textContent.includes('新增考勤组')"), false);
          } finally { hrmOptionsDelay = 0; }
          if (process.env.BIZ_IM_WORKSPACE_SCREENSHOT_PATH) {
            const shot = await cdp.send("Page.captureScreenshot", { format: "png" });
            await writeFile(process.env.BIZ_IM_WORKSPACE_SCREENSHOT_PATH, Buffer.from(shot.data, "base64"));
          }
          await cdp.send("Emulation.setDeviceMetricsOverride", { width: 390, height: 844, deviceScaleFactor: 1, mobile: true });
          assert.equal(await cdp.eval("document.documentElement.scrollWidth <= innerWidth"), true, "IM mobile shell has no horizontal overflow");
          await cdp.eval("[...document.querySelectorAll('button')].find(b=>b.textContent==='菜单').click()");
          assert.equal(await cdp.eval("getComputedStyle(document.querySelector('.sidebar nav')).display!=='none'"), true);
          await cdp.eval("document.querySelector('[data-route=\"im/directory\"]').click()");
          await cdp.wait("location.pathname.endsWith('/im/directory')");
          assert.equal(await cdp.eval("document.querySelector('.sidebar').classList.contains('open')"), false);
          await cdp.send("Emulation.setDeviceMetricsOverride", { width: 1440, height: 1000, deviceScaleFactor: 1, mobile: false });
        }
      );
      await t.test(
        "enterprise directory replaces friends and searches departments, descendants and paged names on the server",
        async () => {
          await cdp.go(origin + base + 'im/friends');
          await cdp.wait("location.pathname.endsWith('/im/directory') && document.querySelector('.directory-person')");
          assert.equal(await cdp.eval("document.querySelector('h1').textContent"), '企业通讯录');
          assert.equal(await cdp.eval("document.querySelector('.directory-person .directory-avatar').tagName"),'SPAN', 'empty avatar must show the name placeholder rather than a broken image');
          assert.equal(await cdp.eval("document.querySelector('.brand strong').textContent"), '企业即时通讯');
          assert.equal(await cdp.eval("document.querySelector('[data-route=\"im/friends\"]')"), null);
          await cdp.eval("document.querySelector('[data-department=\"2\"]').click()");
          await cdp.wait("document.querySelectorAll('.directory-person').length===2");
          assert.match(await cdp.eval("document.querySelector('.directory-results').textContent"), /002[\s\S]*003/);
          await cdp.eval("document.querySelector('[name=include_subdepartments]').click()");
          await cdp.wait("document.querySelectorAll('.directory-person').length===1");
          assert.equal(await cdp.eval("document.querySelector('.directory-person').dataset.userId"), '2');
          await cdp.eval("document.querySelector('[name=include_subdepartments]').click();document.querySelector('[name=directory_search]').value='003';document.querySelector('.directory-search').requestSubmit()");
          await cdp.wait("document.querySelector('.directory-person')?.dataset.userId==='3'");
          const filtered=recorded.filter(row=>row.path==='/im/directory/users').at(-1);
          const params=new URLSearchParams(filtered.query);
          assert.equal(params.get('q'),'003'); assert.equal(params.get('department_id'),'2'); assert.equal(params.get('include_subdepartments'),'true'); assert.equal(params.get('page'),'1');
          await cdp.eval("[...document.querySelectorAll('.directory-search button')].find(button=>button.textContent==='清空').click()");
          await cdp.wait("document.querySelectorAll('.directory-person').length===20");
          await cdp.eval("[...document.querySelectorAll('.directory-results .pager button')].find(button=>button.textContent==='下一页').click()");
          await cdp.wait("document.querySelector('.directory-count').textContent.includes('第 2 页')");
          assert.equal(new URLSearchParams(recorded.filter(row=>row.path==='/im/directory/users').at(-1).query).get('page'),'2');
          await cdp.eval("document.querySelector('[data-department=unassigned]').click()");
          await cdp.wait("document.querySelectorAll('.directory-person').length===1 && document.querySelector('.directory-person').dataset.userId==='7'");
          const unassignedParams=new URLSearchParams(recorded.filter(row=>row.path==='/im/directory/users').at(-1).query);
          assert.equal(unassignedParams.get('unassigned'),'true');assert.equal(unassignedParams.has('department_id'),false);assert.equal(unassignedParams.get('page'),'1');
          assert.equal(recorded.some(row=>/\/im\/(friends|friend-requests)/.test(row.path)),false);
        }
      );
      await t.test(
        "directory keeps unavailable staff visible and can select distinct coworkers for an approval-protected group",
        async () => {
          await cdp.go(origin + base + 'im/directory');
          await cdp.wait("document.querySelector('[data-user-id=\"4\"]')");
          for (const selector of ['[data-user-id="4"]','[data-user-id="5"]','[data-employee="im-unbound"]']) {
            assert.equal(await cdp.eval(`document.querySelector(${JSON.stringify(selector)}).querySelector('input').disabled`),true);
            assert.equal(await cdp.eval(`document.querySelector(${JSON.stringify(selector)}).querySelector('button').disabled`),true);
          }
          const text=await cdp.eval("document.querySelector('.directory-results').textContent");
          for(const reason of ['对方账号已停用','对方未获企业即时通讯权限','尚未关联本地账号'])assert.ok(text.includes(reason));
          assert.equal(await cdp.eval("document.querySelector('[data-user-id=\"2\"] .directory-account-status').textContent"),'账号已启用');
          await cdp.eval("document.querySelector('[data-member-id=\"2\"]').click();document.querySelector('[data-member-id=\"3\"]').click()");
          await cdp.eval("[...document.querySelectorAll('#view button')].find(button=>button.textContent.startsWith('发起群聊')).click()");
          await cdp.wait("document.querySelector('dialog [name=group_name]')");
          assert.equal(await cdp.eval("document.querySelectorAll('dialog .selection-chip').length"),2);
          assert.match(await cdp.eval("document.querySelector('dialog .directory-selection').textContent"),/李四 · 002[\s\S]*李四 · 003/);
          const groupPosts=recorded.filter(row=>row.path==='/im/groups'&&row.method==='POST').length;
          await cdp.eval("document.querySelector('dialog [name=group_name]').value='同名同事工作群';document.querySelector('dialog form').requestSubmit();document.querySelector('dialog form').requestSubmit()");
          await cdp.wait("location.search.includes('created-group')");
          const post=recorded.filter(row=>row.path==='/im/groups'&&row.method==='POST').at(-1);
          assert.deepEqual(post.body.member_ids,[2,3]); assert.equal(post.body.join_approval,1); assert.equal(post.body.name,'同名同事工作群');
          assert.equal(recorded.filter(row=>row.path==='/im/groups'&&row.method==='POST').length,groupPosts+1,'repeated Enter must not create duplicate groups');
        }
      );
      await t.test(
        "a member becoming unavailable during group creation keeps the dialog open with Chinese recovery guidance and permits reselection",
        async () => {
          const invalid=directoryUsers.find(row=>row.id===2), oldCanChat=invalid.can_chat, before=createdGroupCount;
          rejectGroupCreation=true;
          try {
            await cdp.go(origin + base + 'im/directory');
            await cdp.wait("document.querySelector('[data-member-id=\"2\"]')");
            await cdp.eval("document.querySelector('[data-member-id=\"2\"]').click();[...document.querySelectorAll('#view button')].find(button=>button.textContent.startsWith('发起群聊')).click()");
            await cdp.wait("document.querySelector('dialog [name=group_name]')");
            await cdp.eval("document.querySelector('dialog [name=group_name]').value='成员失效重试群';document.querySelector('dialog form').requestSubmit()");
            await cdp.wait("document.querySelector('dialog [role=alert]')?.textContent.includes('创建失败，请刷新通讯录并重新选择成员后重试。')");
            assert.ok((await cdp.eval("document.querySelector('dialog [role=alert]').textContent")).includes('not a participant or recipient is unavailable'));
            assert.equal(await cdp.eval("document.querySelector('dialog [name=group_name]').value"),'成员失效重试群');
            assert.equal(createdGroupCount,before);
            assert.equal(await cdp.eval("location.pathname"),base+'im/directory');
            invalid.can_chat=false;
            await cdp.eval("document.querySelector('dialog .directory-search').requestSubmit()");
            await cdp.wait("document.querySelector('dialog [data-member-id=\"2\"]')?.disabled===true && document.querySelectorAll('dialog .selection-chip').length===0");
            assert.equal(await cdp.eval("document.querySelector('dialog form button[type=submit]').disabled"),true);
            await cdp.eval("document.querySelector('dialog [data-member-id=\"3\"]').click()");
            rejectGroupCreation=false;
            await cdp.eval("document.querySelector('dialog form').requestSubmit()");
            await cdp.wait("!document.querySelector('dialog') && location.search.includes('created-group')");
            assert.equal(createdGroupCount,before+1);
            assert.deepEqual(recorded.filter(row=>row.path==='/im/groups'&&row.method==='POST').at(-1).body.member_ids,[3]);
          } finally { rejectGroupCreation=false;invalid.can_chat=oldCanChat; }
        }
      );
      await t.test(
        "empty chat receives new private and group conversations and closes its realtime connection on departure",
        async () => {
          const saved=conversations; conversations=[];
          try {
            await cdp.go(origin + base + 'im/chat');
            await cdp.wait("document.querySelector('.conversation-heading').textContent.includes('实时连接')");
            assert.equal(await cdp.eval("document.querySelectorAll('.conversation').length"),0);
            conversations=[{conversation_type:'private',conversation_id:'3',name:'李四',employee_no:'003',last_content:'首次企业消息',unread_count:1}];
            push({type:'message'});
            await cdp.wait("document.querySelector('.conversations').textContent.includes('首次企业消息')");
            conversations.push({conversation_type:'group',conversation_id:'new-group',name:'新项目群',last_content:'',unread_count:0});
            push({type:'membership',group_id:'new-group',action:'created'});
            await cdp.wait("document.querySelector('.conversations').textContent.includes('新项目群')");
            await cdp.eval("document.querySelector('[data-route=\"im/directory\"]').click()");
            await cdp.wait("document.querySelector('.directory-person')");
            await delay(100);
            const calls=recorded.filter(row=>row.path==='/im/conversations').length;
            push({type:'message'});await delay(120);
            assert.equal(recorded.filter(row=>row.path==='/im/conversations').length,calls);
          } finally {conversations=saved;}
        }
      );
      await t.test(
        "non-friend direct chat uses names and contact blocks preserve history while disabling sends and calls",
        async () => {
          await cdp.go(origin + base + 'im/directory');
          await cdp.wait("document.querySelector('[data-user-id=\"2\"] button')");
          await cdp.eval("document.querySelector('[data-user-id=\"2\"] button').click()");
          await cdp.wait("document.querySelector('.message') && !document.querySelector('[name=message]').disabled");
          assert.equal(await cdp.eval("document.querySelector('.chat-head>strong').textContent"),'李四');
          assert.match(await cdp.eval("document.querySelector('.message>small').textContent"),/^李四（002）/);
          await cdp.eval("[...document.querySelectorAll('.chat-head button')].find(button=>button.textContent==='备注').click()");
          await cdp.wait("document.querySelector('dialog [name=display_name]')");
          await cdp.eval("document.querySelector('dialog [name=display_name]').value='研发同事';document.querySelector('dialog form').requestSubmit()");
          await cdp.wait("document.querySelector('.chat-head>strong').textContent==='研发同事'");
          await cdp.eval("[...document.querySelectorAll('.chat-head button')].find(button=>button.textContent==='屏蔽').click()");
          await cdp.wait("document.querySelector('.chat-restriction').textContent.includes('你已屏蔽') && document.querySelector('[name=message]').disabled");
          assert.equal(await cdp.eval("[...document.querySelectorAll('.chat-head button')].find(button=>button.textContent==='音视频通话').disabled"),true);
          assert.ok(await cdp.eval("document.querySelector('.messages').textContent.includes('你好')"));
          const sends=recorded.filter(row=>row.path==='/im/private/messages'&&row.method==='POST').length;
          await cdp.eval("document.querySelector('[name=message]').value='禁止发送';document.querySelector('.composer').requestSubmit()");
          await delay(80);assert.equal(recorded.filter(row=>row.path==='/im/private/messages'&&row.method==='POST').length,sends);
          await cdp.eval("[...document.querySelectorAll('.chat-head button')].find(button=>button.textContent==='恢复联系').click()");
          await cdp.wait("!document.querySelector('[name=message]').disabled");
          await cdp.go(origin + base + 'im/chat?kind=private&id=4');
          await cdp.wait("document.querySelector('.chat-restriction').textContent.includes('对方账号已停用') && document.querySelector('.message')");
          assert.equal(await cdp.eval("document.querySelector('[name=message]').disabled"),true);
          contactSettings.clear();
        }
      );
      await t.test(
        "directory failed queries retry in place and late responses cannot overwrite a new workspace",
        async () => {
          directoryFailure=true;
          await cdp.go(origin + base + 'im/directory');
          await cdp.wait("document.querySelector('.directory-results [role=alert]')");
          directoryFailure=false;
          await cdp.eval("[...document.querySelectorAll('.directory-results button')].find(button=>button.textContent==='重试').click()");
          await cdp.wait("document.querySelector('.directory-person')");
          directoryDelay=350;
          await cdp.eval("document.querySelector('[name=directory_search]').value='李四';document.querySelector('.directory-search').requestSubmit();document.querySelector('[data-route=\"im/chat\"]').click()");
          await cdp.wait("document.querySelector('.chat-layout')"); await delay(450);
          assert.equal(await cdp.eval("document.querySelector('#view h1').textContent"),'消息中心');
          assert.equal(await cdp.eval("document.querySelector('.directory-results')"),null);
          directoryDelay=0;
        }
      );
      await t.test(
        "explicit peer capabilities restrict history and calls and group management uses names instead of raw IDs",
        async () => {
          const row=directoryUsers.find(row=>row.id===3);
          Object.assign(row,{can_send:false,can_call:false,can_read:false,readonly_reason:'当前成员不可通信'});
          try {
            const before=recorded.filter(row=>row.path==='/im/private/messages'&&new URLSearchParams(row.query).get('receiver_id')==='3').length;
            await cdp.go(origin + base + 'im/chat?kind=private&id=3');
            await cdp.wait("document.querySelector('.chat-restriction').textContent==='当前成员不可通信'");
            assert.equal(await cdp.eval("document.querySelector('[name=message]').disabled"),true);
            assert.equal(await cdp.eval("document.querySelectorAll('.message').length"),0);
            assert.equal(recorded.filter(row=>row.path==='/im/private/messages'&&new URLSearchParams(row.query).get('receiver_id')==='3').length,before);
          } finally { for(const key of ['can_send','can_call','can_read','readonly_reason'])delete row[key]; }
          await cdp.go(origin + base + 'im/groups');
          await cdp.wait("document.querySelector('tbody')");
          assert.ok((await cdp.eval("document.querySelector('tbody').textContent")).includes('管理员'));
          await cdp.eval("[...document.querySelectorAll('tbody button')].find(button=>button.textContent==='成员 / 管理').click()");
          await cdp.wait("document.querySelector('dialog [name=user_id]')");
          const text=await cdp.eval("document.querySelector('dialog').textContent");
          assert.ok(text.includes('李四'));assert.ok(text.includes('002'));assert.ok(!text.includes('用户 ID'));
          assert.match(await cdp.eval("document.querySelector('dialog [name=user_id]').textContent"),/李四 · 002/);
          await cdp.eval("document.querySelector('dialog .close').click()");
        }
      );
      await t.test(
        "directory group picker closes on navigation and narrow layouts stay within the viewport",
        async () => {
          await cdp.go(origin + base + 'im/directory');await cdp.wait("document.querySelector('[data-member-id=\"2\"]')");
          if(process.env.BIZ_IM_DIRECTORY_SCREENSHOT_PATH){const shot=await cdp.send('Page.captureScreenshot',{format:'png'});await writeFile(process.env.BIZ_IM_DIRECTORY_SCREENSHOT_PATH,Buffer.from(shot.data,'base64'));}
          await cdp.send('Emulation.setDeviceMetricsOverride',{width:390,height:844,deviceScaleFactor:1,mobile:true});
          assert.equal(await cdp.eval('document.documentElement.scrollWidth<=innerWidth'),true);
          await cdp.eval("document.querySelector('[data-member-id=\"2\"]').click();[...document.querySelectorAll('#view button')].find(button=>button.textContent.startsWith('发起群聊')).click()");
          await cdp.wait("document.querySelector('dialog [name=group_name]')");
          assert.equal(await cdp.eval('document.documentElement.scrollWidth<=innerWidth'),true);
          await cdp.eval("history.pushState({},'', '/plugins/biz-suite/admin/im/chat');window.dispatchEvent(new PopStateEvent('popstate'))");
          await cdp.wait("!document.querySelector('dialog') && document.querySelector('.chat-layout')");
          await cdp.send('Emulation.setDeviceMetricsOverride',{width:1440,height:1000,deviceScaleFactor:1,mobile:false});
        }
      );
      await t.test(
        "message center provides permitted chat and group entry actions without requiring an existing conversation",
        async () => {
          const previousPermissions=permissions;
          try {
            await cdp.go(origin + base + 'im/chat');
            await cdp.wait("document.querySelector('.chat-start-actions')");
            assert.deepEqual(await cdp.eval("[...document.querySelectorAll('.chat-start-actions button')].map(button=>button.textContent)"),['发起聊天','发起群聊']);
            await cdp.eval("[...document.querySelectorAll('.chat-start-actions button')].find(button=>button.textContent==='发起聊天').click()");
            await cdp.wait("location.pathname.endsWith('/im/directory') && document.querySelector('.directory-person')");
            await cdp.eval("document.querySelector('[data-route=\"im/chat\"]').click()");
            await cdp.wait("document.querySelector('.chat-start-actions')");
            await cdp.eval("[...document.querySelectorAll('.chat-start-actions button')].find(button=>button.textContent==='发起群聊').click()");
            await cdp.wait("document.querySelector('dialog [name=group_name]') && document.querySelector('dialog .directory-person')");
            assert.equal(await cdp.eval("document.querySelectorAll('dialog .selection-chip').length"),0);
            assert.equal(await cdp.eval("document.querySelector('dialog button[type=submit]').disabled"),true);
            await cdp.eval("document.querySelector('dialog .close').click()");
            permissions=['biz-suite.im.read'];
            await cdp.go(origin + base + 'im/chat');
            await cdp.wait("document.querySelector('.chat-layout')");
            assert.equal(await cdp.eval("document.querySelector('.chat-start-actions')"),null);
          } finally { permissions=previousPermissions; }
        }
      );
      await t.test(
        "group and channel deep links resolve their real conversation names without a name query",
        async () => {
          const saved=conversations;
          conversations=[...saved,{conversation_type:'group',conversation_id:'g1',name:'研发项目群'}, {conversation_type:'channel',conversation_id:'c1',name:'公司通知频道'}];
          try {
            for(const [kind,id,name] of [['group','g1','研发项目群'],['channel','c1','公司通知频道']]) {
              await cdp.go(origin + base + `im/chat?kind=${kind}&id=${id}`);
              await cdp.wait(`document.querySelector('.chat-head>strong')?.textContent===${JSON.stringify(name)}`);
              assert.equal(await cdp.eval("new URLSearchParams(location.search).has('name')"),false);
            }
          } finally {conversations=saved;}
        }
      );
      await t.test(
        "authenticated chat sends client ID, tracks read sequence and handles RTC disabled",
        async () => {
          await cdp.go(origin + base + "im/chat?kind=private&id=2&name=李四");
          await cdp.wait("document.querySelector('.message')");
          await cdp.eval(
            "document.querySelector('[name=message]').value='测试消息 <script>xss</script>';document.querySelector('.composer').requestSubmit()"
          );
          await cdp.wait(
            "document.querySelector('.messages').textContent.includes('测试消息')"
          );
          const post = recorded.find(
            (r) => r.method === "POST" && r.path === "/im/private/messages"
          );
          assert.equal(post.body.receiver_id, 2);
          assert.ok(post.body.client_message_id);
          assert.ok(
            recorded.some(
              (r) => r.path === "/im/conversations/read" && r.body.last_seq >= 1
            )
          );
          await cdp.eval(
            "[...document.querySelectorAll('button')].find(b=>b.textContent==='音视频通话').click()"
          );
          await cdp.wait(
            "document.querySelector('dialog').textContent.includes('尚未配置')"
          );
          await cdp.eval("document.querySelector('dialog .close').click()");
        }
      );
      await t.test(
        "local LiveKit SDK is loadable and host media upload is usable",
        async () => {
          assert.equal(
            await cdp.eval(
              "import('/plugins/biz-suite/admin/assets/livekit.js').then(m=>typeof m.Room)"
            ),
            "function"
          );
          await cdp.eval(
            "const dt=new DataTransfer();dt.items.add(new File(['hello'],'hello.txt',{type:'text/plain'}));const input=document.querySelector('input[type=file]');input.files=dt.files;input.dispatchEvent(new Event('change'));"
          );
          await cdp.wait(
            "document.querySelector('[name=message]').value==='/uploads/hello.txt'"
          );
          assert.ok(
            recorded.some(
              (r) =>
                r.path === "/im/media/upload" &&
                r.headers["content-type"].startsWith("multipart/form-data")
            )
          );
        }
      );
      await t.test(
        "read-only permission hides mutation controls and protected routes",
        async () => {
          permissions = ["biz-suite.wecom.read"];
          await cdp.go(origin + base + "wecom/status");
          await cdp.wait(
            "document.querySelector('#wecom-settings [name=corp_id]')"
          );
          assert.equal(
            await cdp.eval("document.querySelector('[name=corp_id]').disabled"),
            true
          );
          assert.equal(
            await cdp.eval(
              "document.querySelectorAll('[data-sync-kind]').length"
            ),
            0
          );
          assert.equal(await cdp.eval("document.querySelectorAll('[data-diagnostic-kind]').length"), 0);
          assert.equal(await cdp.eval("[...document.querySelectorAll('[data-name-filter] textarea,[data-name-filter] select')].every(n=>n.disabled)"), true);
          assert.equal(await cdp.eval("document.querySelectorAll('.filter-selection button').length"), 0);
          assert.equal(await cdp.eval("[...document.querySelectorAll('#wecom-settings input[type=password]')].every(n=>n.disabled && n.value==='')"), true);
          assert.equal(
            await cdp.eval(
              "[...document.querySelectorAll('button')].some(b=>b.textContent==='保存配置')"
            ),
            false
          );
          permissions = ["biz-suite.hrm.read"];
          await cdp.go(origin + base + "hrm/employees");
          await cdp.wait("document.querySelector('table')");
          assert.equal(
            await cdp.eval(
              "[...document.querySelectorAll('button')].some(b=>b.textContent.includes('新增')||b.textContent==='编辑')"
            ),
            false
          );
          assert.equal(
            await cdp.eval(
              "!!document.querySelector('[data-route=\"im/chat\"]')"
            ),
            false
          );
          await cdp.go(origin + base + "wecom/templates");
          await cdp.wait(
            "document.querySelector('h1').textContent==='无访问权限'"
          );
        }
      );
      await t.test(
        "self and department HRM grants expose only scoped read pages and never request organization administration",
        async () => {
          for (const [grant, hint] of [["self", "仅可查看本人的档案与考勤"], ["department", "可查看主部门及下级部门"]]) {
            permissions = ["biz-suite.hrm." + grant];
            const organizationCalls = recorded.filter((r) => r.path === "/wecom/organization/sync").length;
            await cdp.go(origin + base);
            await cdp.wait("document.querySelector('h1')?.textContent==='智慧HRM'");
            const nav = await cdp.eval("[...document.querySelectorAll('.sidebar [data-route]')].map(n=>n.dataset.route)");
            assert.ok(nav.includes("hrm/employees") && nav.includes("hrm/clocks") && nav.includes("hrm/leaves"));
            assert.equal(nav.some((path) => path.startsWith("wecom/") || path.startsWith("im/") || ["hrm/groups", "hrm/holidays"].includes(path)), false);
            await cdp.eval("document.querySelector('[data-route=\"hrm/employees\"]').click()");
            await cdp.wait("document.querySelector('table')");
            assert.ok((await cdp.eval("document.querySelector('#view').textContent")).includes(hint));
            assert.equal(await cdp.eval("[...document.querySelectorAll('#view button')].some(b=>/新增|编辑|同步组织与账号/.test(b.textContent))"), false);
            assert.equal(recorded.filter((r) => r.path === "/wecom/organization/sync").length, organizationCalls);
            const configCalls = recorded.filter((r) => r.path === "/hrm/attendance/groups").length;
            await cdp.go(origin + base + "hrm/groups");
            await cdp.wait("document.querySelector('h1')?.textContent==='无访问权限'");
            assert.equal(recorded.filter((r) => r.path === "/hrm/attendance/groups").length, configCalls);
            await cdp.go(origin + base + "wecom/departments");
            await cdp.wait("document.querySelector('h1')?.textContent==='无访问权限'");
            assert.equal(recorded.filter((r) => r.path === "/wecom/organization/sync").length, organizationCalls);
          }
          permissions = ["biz-suite.hrm.write"];
          await cdp.go(origin + base + "hrm/employees");
          await cdp.wait("document.querySelector('table')");
          assert.ok(await cdp.eval("document.querySelector('[data-route=\"hrm/groups\"]')!==null"));
          assert.ok(await cdp.eval("[...document.querySelectorAll('#view button')].some(b=>b.textContent==='+ 新增员工')"));
          assert.equal(await cdp.eval("[...document.querySelectorAll('#view button')].some(b=>b.textContent==='同步组织与账号')"), false);
          permissions = ["biz-suite.wecom.read"];
          const organizationCalls = recorded.filter((r) => r.path === "/wecom/organization/sync").length;
          await cdp.go(origin + base + "wecom/departments");
          await cdp.wait("document.querySelector('table')");
          assert.equal(await cdp.eval("[...document.querySelectorAll('#view button')].some(b=>b.textContent==='同步组织与账号'||b.textContent==='设置映射')"), false);
          assert.equal(recorded.filter((r) => r.path === "/wecom/organization/sync").length, organizationCalls);
        }
      );
      await t.test(
        "single-workspace and empty permissions keep menus, entry routes and switches consistent",
        async () => {
          permissions = ["biz-suite.im.read"];
          await cdp.go(origin + base);
          await cdp.wait("document.querySelector('h1')?.textContent==='消息中心'");
          assert.equal(await cdp.eval("location.pathname"), base + "im/chat");
          assert.equal(await cdp.eval("document.querySelector('[data-workspace-link=hrm]')"), null);
          assert.equal(await cdp.eval("document.querySelector('[data-route=\"im/manager\"]')"), null);
          assert.equal(await cdp.eval("document.querySelector('[data-route=\"im/sensitive-words\"]')"), null);
          await cdp.go(origin + base + "hrm/clocks");
          await cdp.wait("document.querySelector('h1')?.textContent==='无访问权限'");
          assert.equal(await cdp.eval("document.querySelectorAll('.sidebar .nav-link').length"), 0);
          assert.equal(await cdp.eval("document.querySelector('.brand strong').textContent"), "智慧HRM");
          await cdp.eval("document.querySelector('[data-workspace-link=im]').click()");
          await cdp.wait("document.querySelector('h1')?.textContent==='消息中心'");
          permissions = ["biz-suite.wecom.read"];
          await cdp.go(origin + base + "overview");
          await cdp.wait("document.querySelector('h1')?.textContent==='智慧HRM'");
          assert.equal(await cdp.eval("location.pathname"), base + "hrm/overview");
          const nav = await cdp.eval("[...document.querySelectorAll('.sidebar .nav-link')].map(a=>a.dataset.route)");
          assert.ok(nav.includes("hrm/overview"));
          assert.ok(nav.includes("wecom/status"));
          assert.equal(nav.some((path) => path.startsWith("im/") || path === "hrm/clocks"), false);
          assert.equal(await cdp.eval("document.querySelectorAll('[data-workspace-link]').length"), 0);
          permissions = [];
          const protectedBefore = recorded.filter((r) => r.path.startsWith("/hrm/") || r.path.startsWith("/im/") || r.path.startsWith("/wecom/")).length;
          await cdp.go(origin + base);
          await cdp.wait("document.querySelector('h1')?.textContent==='无访问权限'");
          assert.equal(await cdp.eval("document.querySelectorAll('.sidebar .nav-link,[data-workspace-link]').length"), 0);
          assert.equal(await cdp.eval("document.querySelector('.workspace-return').getAttribute('href')"), "/suxinweb/");
          await cdp.go(origin + base + "im/chat");
          await cdp.wait("document.querySelector('h1')?.textContent==='无访问权限'");
          assert.equal(await cdp.eval("document.querySelector('.brand strong').textContent"), "企业即时通讯");
          assert.equal(recorded.filter((r) => r.path.startsWith("/hrm/") || r.path.startsWith("/im/") || r.path.startsWith("/wecom/")).length, protectedBefore, "ungranted workspaces issue no protected data requests");
        }
      );
      const timerOverride = await cdp.send("Page.addScriptToEvaluateOnNewDocument", { source: `
        window.__fastIMTimer = true;
        const originalSetTimeout = window.setTimeout;
        window.setTimeout = function(callback, milliseconds, ...args) {
          const fast = window.__fastIMTimer && [60000, 30000, 15000].includes(milliseconds);
          return originalSetTimeout(callback, fast ? 80 : milliseconds, ...args);
        };
      ` });
      const allPermissions = ["biz-suite.hrm.manage", "biz-suite.im.manage", "biz-suite.wecom.manage"];
      await t.test(
        "IM workspace renews while open, recovers after timer throttling and stops when leaving or pagehide",
        async () => {
          permissions = [...allPermissions];
          const calls = () => recorded.filter((r) => r.path === "/im/session");
          const before = calls().length;
          await cdp.go(origin + base + "im/directory");
          await cdp.wait("document.querySelector('#view h1')?.textContent==='企业通讯录'");
          await delay(260);
          assert.ok(calls().length >= before + 2, "even a non-chat IM page keeps the session alive");
          assert.ok(calls().slice(before).every((r) => r.method === "GET" && !r.headers.authorization));
          await cdp.eval("window.__fastIMTimer=false");
          await delay(1250);
          const quiet = calls().length;
          await cdp.eval("window.dispatchEvent(new Event('focus'));window.dispatchEvent(new Event('online'));document.dispatchEvent(new Event('visibilitychange'))");
          await delay(180);
          assert.equal(calls().length, quiet + 1, "focus/online/visibility restoration is deduplicated");
          await cdp.eval("document.querySelector('[data-workspace-link=hrm]').click()");
          await cdp.wait("document.querySelector('#view h1')?.textContent==='智慧HRM'");
          const hrm = calls().length;
          await cdp.eval("window.__fastIMTimer=true;window.dispatchEvent(new Event('focus'));window.dispatchEvent(new Event('online'))");
          await delay(220);
          assert.equal(calls().length, hrm, "HRM does not renew the IM session");
          await cdp.eval("document.querySelector('[data-workspace-link=im]').click()");
          await cdp.wait("document.querySelector('.chat-layout')");
          await delay(150);
          await cdp.eval("window.dispatchEvent(new PageTransitionEvent('pagehide'))");
          const hidden = calls().length;
          await delay(220);
          assert.equal(calls().length, hidden, "pagehide cancels renewal");
          await cdp.eval("window.dispatchEvent(new PageTransitionEvent('pageshow',{persisted:true}))");
          await delay(180);
          assert.ok(calls().length > hidden, "back-forward cache restoration restarts validation");
        }
      );
      await t.test(
        "temporary IM renewal errors retry and recover without asking for credentials",
        async () => {
          imSessionStatus = 503;
          try {
            await cdp.go(origin + base + "im/groups");
            await cdp.wait("document.querySelector('#session-notice')?.hidden===false");
            assert.match(await cdp.eval("document.querySelector('#session-notice').textContent"), /正在重试/);
            assert.equal(await cdp.eval("document.querySelector('[data-session-state=expired]')"), null);
            imSessionStatus = 200;
            await cdp.wait("document.querySelector('#session-notice')?.hidden===true");
            assert.equal(await cdp.eval("document.querySelector('#view h1').textContent"), "群组");
          } finally { imSessionStatus = 200; }
        }
      );
      await t.test(
        "HTTP and envelope session failures stop renewal; revoked IM grants keep HRM available",
        async () => {
          for (const [httpStatus, bodyCode] of [[403, 403], [200, 403]]) {
            permissions = [...allPermissions];
            await cdp.go(origin + base + "im/directory");
            await cdp.wait("document.querySelector('#view h1')?.textContent==='企业通讯录'");
            permissions = ["biz-suite.hrm.read"];
            imSessionStatus = httpStatus;
            imSessionCode = bodyCode;
            await cdp.wait("document.querySelector('[data-session-state=forbidden]')");
            const stopped = recorded.filter((r) => r.path === "/im/session").length;
            await delay(250);
            assert.equal(recorded.filter((r) => r.path === "/im/session").length, stopped);
            assert.equal(await cdp.eval("document.querySelectorAll('.sidebar [data-route^=\"im/\"]').length"), 0);
            assert.ok(await cdp.eval("document.querySelector('[data-workspace-link=hrm]')!==null"));
            await cdp.eval("document.querySelector('[data-workspace-link=hrm]').click()");
            await cdp.wait("document.querySelector('#view h1')?.textContent==='智慧HRM'");
            assert.equal(await cdp.eval("document.querySelector('[data-workspace-link=im]')"), null);
            imSessionStatus = 200; imSessionCode = 0;
          }
          for (const [httpStatus, bodyCode] of [[401, 401], [200, 401]]) {
            permissions = [...allPermissions];
            imSessionStatus = httpStatus; imSessionCode = bodyCode;
            await cdp.go(origin + base + "im/chat?kind=private&id=2&name=李四");
            await cdp.wait("document.querySelector('[data-session-state=expired]')");
            assert.equal(await cdp.eval("document.querySelectorAll('.chat-layout').length"), 0);
            const stopped = recorded.filter((r) => r.path.startsWith("/im/")).length;
            await delay(250);
            assert.equal(recorded.filter((r) => r.path.startsWith("/im/")).length, stopped, "authentication failure stops heartbeat, polling and reconnect requests");
            assert.match(await cdp.eval("document.querySelector('.session-recovery a').getAttribute('href')"), /^\/suxinweb\/login\?redirect=/);
            imSessionStatus = 200; imSessionCode = 0;
          }
        }
      );
      await t.test(
        "host logout and account changes clear the previous IM identity without token bootstrap",
        async () => {
          permissions = [...allPermissions];
          await cdp.go(origin + base + "im/chat?kind=private&id=2&name=李四");
          await cdp.wait("document.querySelector('.message')");
          await cdp.eval("window.dispatchEvent(new StorageEvent('storage',{key:'biztoken',newValue:null}))");
          await cdp.wait("document.querySelector('[data-session-state=expired]')");
          assert.equal(await cdp.eval("document.querySelectorAll('.message').length"), 0);
          const stopped = recorded.filter((r) => r.path.startsWith("/im/")).length;
          await delay(250);
          assert.equal(recorded.filter((r) => r.path.startsWith("/im/")).length, stopped);
          await cdp.go(origin + base + "im/chat?kind=private&id=2&name=李四");
          await cdp.wait("document.querySelector('.message')");
          imSessionUid = 2;
          await cdp.wait("document.querySelector('[data-session-state=expired]')");
          assert.equal(await cdp.eval("document.querySelectorAll('.message,.chat-layout').length"), 0);
          assert.equal(recorded.some((r) => r.path.includes("__host_auth/bootstrap")), false);
          imSessionUid = 1;
        }
      );
      await cdp.send("Page.removeScriptToEvaluateOnNewDocument", { identifier: timerOverride.identifier });
      await t.test("expired session gives actionable feedback", async () => {
        authFail = true;
        await cdp.send("Page.navigate", {
          url: origin + base + "hrm/employees",
        });
        await cdp.wait("document.body.textContent.includes('登录已失效')");
      });
      assert.deepEqual(cdp.errors, [], "No uncaught browser errors");
    } finally {
      socket?.close();
      child.kill();
      for (const peer of peers) peer.destroy();
      await new Promise((r) => server.close(r));
    }
  }
);
