import { el, api, API, BASE, asRows, can, state, page, surface, button, dataTable, field, form, dialog, details, errorNode, toast, safe } from "./core.js";
import { departmentPath } from "./organization.js";
import { windowEditor, windowTimeline, inspectWindows, windowClock, windowSecond } from "./attendance-windows.js";
import { scopeEditor } from "./attendance-scope.js";
import { workScheduleEditor, workScheduleCycle } from "./attendance-work-schedule.js";

const ROOT = "/hrm/attendance/v2";
const labels = { draft: "草稿", published: "已发布", locked: "已锁定", reviewed: "已复核", reviewing: "待复核", reopened: "已重开", calculated: "已计算", pending: "进行中", in_progress: "进行中", review: "需核对", rest: "休息", running: "计算中", queued: "等待计算", success: "完成", completed: "完成", failed: "失败", partial: "部分完成", confirmed: "已确认", candidate: "候选排班", workday: "工作日", restday: "休息日", holiday: "法定假日", work: "正班", break: "休息", overtime: "加班", manual: "人工登记", wecom: "企业微信", cycle: "制度周期", opening: "期初额度", convert: "加班转调休", use: "使用调休", reverse: "冲销", approved: "已通过", rejected: "已驳回", canceled: "已撤销", baseline: "月度基准", rules: "规则来源", matched: "已匹配", unmatched: "未匹配", missing: "证据缺失", equal: "一致", difference: "存在差异" };
Object.assign(labels, { configuration_pending:"待配置", archived:"已归档", superseded:"已替换", unmapped: "尚未关联", unverifiable: "原始证据不足", open: "待复核", imported: "已导入", confirmed: "已确认", pending_review: "待复核", uncomputed: "未计算", manual_adjustment: "人工认定", generated: "按制度生成", weekly: "每周排班" });
const label = (value) => labels[value] || (value ? "待核对" : "—");
const hours = (seconds) => (Number(seconds || 0) / 3600).toFixed(2);
const today = () => new Intl.DateTimeFormat("en-CA", { timeZone: "Asia/Shanghai", year: "numeric", month: "2-digit", day: "2-digit" }).format(new Date());
const personText = (e = {}) => `${e.name || e.employee_name || "未匹配员工"} · ${e.employee_no || "未填写工号"}${e.department ? " · " + e.department : ""}`;
const categories = [["workday", "工作日"], ["restday", "休息日"], ["holiday", "法定假日"]];
const weekdays = ["周一", "周二", "周三", "周四", "周五", "周六", "周日"];
const validDate = value => typeof value === "string" && /^\d{4}-\d{2}-\d{2}$/.test(value) && Number.isFinite(Date.parse(value + "T00:00:00Z")) && new Date(value + "T00:00:00Z").toISOString().slice(0, 10) === value;
// Convert only a complete anchored seven-day cycle. Longer cycles and unknown
// anchors must retain their original meaning until the administrator edits them.
function legacyWeekdays(cycle) {
  if (cycle.mode && cycle.mode !== "cycle" || !validDate(cycle.anchor) || !Array.isArray(cycle.days) || cycle.days.length !== 7 || !cycle.days.every(day => typeof day === "string")) return null;
  const first = (new Date(cycle.anchor + "T00:00:00Z").getUTCDay() + 6) % 7, result = [];
  cycle.days.forEach((shift_id, index) => { result[(first + index) % 7] = { shift_id, category: shift_id ? "workday" : "restday", scheduled_work: !!shift_id }; });
  return result;
}
const dateFields = [{ key: "start_date", label: "开始日期", type: "date", required: true }, { key: "end_date", label: "结束日期", type: "date", required: true }];
const queryString = (values) => new URLSearchParams(Object.entries(values).filter(([, v]) => v !== "" && v != null)).toString();
const request = (path, body, method) => api(ROOT + path, body, method);
const employeeOptions = (items, empty = "全部人员") => [["", empty], ...asRows(items).map((e) => [e.id, personText(e)])];
const batchOptions = (items) => [["", "请选择计算批次"], ...asRows(items).map((b) => [b.id, `${b.start_date} 至 ${b.end_date} · ${label(b.status)} · ${b.created_at || ""}`])];
// Exported so local seed/reconciliation tooling can consume the same confirmed
// production shift definition as the editor. It remains an unpublished draft.
export function productionFirstDepartmentPreset({ effective_from = today(), cycle_anchor = "2026-10-05", department_ids = [], employee_ids = [], cycle_days } = {}) {
  const morning = { id: "morning", name: "上午", kind: "work", start_minute: 480, end_minute: 700, punch_mode: "both" };
  const afternoon = { id: "afternoon", name: "下午", kind: "work", start_minute: 790, end_minute: 1050, punch_mode: "both" };
  const alternate = { id: "production-one-evening", name: "生产一部有晚间加班", version: "1", segments: [{ ...morning }, { ...afternoon, end_minute: 1035 }, { id: "evening-overtime", name: "晚间加班", kind: "overtime", start_minute: 1065, end_minute: 1800, open_ended: true, overtime_start_minute: 1080, punch_mode: "both" }] };
  const normal = { id: "production-one-normal", name: "生产一部无晚间加班", version: "1", segments: [morning, afternoon], overtime_variant: { trigger_minute: 1065, shift: alternate } };
  return { name: "生产一部有无晚间加班制度", status: "draft", effective_from, effective_to: "", conflicts: [], scope: { department_ids, employee_ids, include_children: true, is_default: false }, source: { source: "用户确认的生产一部案例", note: "实际晚间卡识别，缺卡待复核；适用范围、生效日期、周期和其他假勤制度须另行确认" }, config: {
    category: "员工", shifts: [normal, alternate], cycle: cycle_days ? { mode: "cycle", anchor: cycle_anchor, days: cycle_days } : { mode: "weekly", weekdays: weekdays.map((_, index) => ({ shift_id: normal.id, category: index < 5 ? "workday" : "restday", scheduled_work: index < 5 })) }, calendar: {},
    rule: { window: { in_before_minutes: 120, in_after_minutes: 120, out_before_minutes: 120, out_after_minutes: 120 }, punch_selection: "first_last", late_grace_minutes: 0, early_grace_minutes: 0, grace_mode: "normal", attendance_mode: "actual", standard_minutes: 0, late_deduction_mode: "none", early_deduction_mode: "none", credit_out: false, credit_trip: false, allow_abnormal_punches: false, overtime_date_mode: "workdate", flex_minutes: 0, late_arrival_tiers: [], leave_policies: {}, full_attendance: { allowed_leave_types: [], allow_out: false, allow_trip: false, allow_late: false, allow_early: false, allow_missing: false }, overtime: Object.fromEntries(categories.map(([key]) => [key, { enabled: true, require_approval: false, minimum_minutes: 30, rounding_minutes: 30, rounding_scope: "daily", maximum_minutes: 0, ...(key === "restday" ? { complete_workday_minutes: 480 } : {}) }])) },
  } };
}
const sourceLabels = { group: "考勤组", groupid: "来源考勤组编号", groupname: "考勤组名称", group_type: "考勤组类型", checkindate: "打卡日期规则", checkintime: "打卡时段", workdays: "工作日", work_sec: "上班时间（秒）", off_work_sec: "下班时间（秒）", schedule: "班次", schedule_id: "来源班次编号", schedule_name: "班次名称", allow_flex: "允许弹性", flex_on_duty_time: "上班弹性秒数", flex_off_duty_time: "下班弹性秒数", allow_rest: "允许休息", rest_begin_time: "休息开始秒数", rest_end_time: "休息结束秒数", no_need_checkon: "免上班卡", no_need_checkoff: "免下班卡", late_rule: "晚到规则", timerules: "时间档位", offwork_after_time: "前日加班达到秒数", onwork_flex_time: "次日晚到秒数", source: "来源", source_key: "来源单元格", source_cell: "来源单元格", import_id: "导入批次", snapshot_id: "规则快照", sheet: "工作表", row: "行号", fields: "源字段", cell: "单元格", value: "表格显示值", raw_value: "原始值", formula: "公式", comment: "批注", merged_from: "合并来源", department: "部门", names_text: "姓名范围", group_text: "组别范围", schedule_text: "原工作时间", checkin_text: "原打卡要求", remarks: "备注", note: "补充说明", overtime_unit_minutes: "加班取整分钟", conflicts: "待核对事项", issues: "来源问题", code: "问题类型", message: "问题说明", severity: "问题级别", endpoint: "企业微信来源接口", request: "查询条件", response: "响应来源", corp_id: "企业标识", fetched_at: "获取时间", snapshot_type: "快照类型" };
function sourceEvidence(value) {
  if (typeof value === "string") { try { value = JSON.parse(value); } catch { return el("p", {}, "来源内容无法解析，请联系管理员检查导入记录。"); } }
  const rows = [];
  function collect(item, path = []) { if (rows.length >= 1500) return; if (item && typeof item === "object") { for (const [key, child] of Object.entries(item)) collect(child, [...path, Array.isArray(item) ? `第${Number(key) + 1}项` : sourceLabels[key] || `来源字段（${key}）`]); } else rows.push({ field: path.join(" / "), value: item === true ? "是" : item === false ? "否" : item == null || item === "" ? "未填写" : String(item) }); }
  collect(value); return dataTable(rows, [["field", "来源字段"], ["value", "原始内容"]]);
}

async function view(title, description, render) {
  const epoch = state.epoch, body = page(title, description), panel = surface(el("p", {}, "正在加载…"));
  body.append(panel);
  const current = () => epoch === state.epoch && panel.isConnected;
  try { await render(panel, current); }
  catch (error) { if (current()) panel.replaceChildren(errorNode(error), button("重试", () => view(title, description, render))); }
}
function input(def, value) { const f = field(def, value); return { ...f, read: () => def.type === "number" ? Number(f.input.value) : f.input.value }; }
function minuteTimeInput(key,title,value) {
  const clock=windowClock(value?value*60:null), time=input({key,label:title,type:"time"},clock.time), day=input({key:key+"_day",label:"所属日期",options:[[0,"当日"],[1,"次日"],[2,"第3日"],[3,"第4日"]]},clock.day);
  return {wrapper:el("div",{class:"form-grid"},day.wrapper,time.wrapper),read:()=>{const second=windowSecond(day.read(),time.read());return second===null?0:second/60;}};
}
function check(text, checked = false) { const box = el("input", { type: "checkbox" }); box.checked = !!checked; return { node: el("label", { class: "check-label" }, box, text), read: () => box.checked, input: box }; }
function pickMany(title, rows, selected = [], key = "id", display = personText) {
  const search = el("input", { type: "search", placeholder: "输入姓名、工号或部门筛选", "aria-label": title + "搜索" });
  const chosen = new Set(asRows(selected).map(String)), list = el("div", { style: "max-height:200px;overflow:auto" });
  function render() {
    const term = search.value.trim().toLocaleLowerCase();
    list.replaceChildren(...asRows(rows).filter((r) => display(r).toLocaleLowerCase().includes(term)).map((r) => {
      const c = check(display(r), chosen.has(String(r[key])));
      c.input.addEventListener("change", () => c.read() ? chosen.add(String(r[key])) : chosen.delete(String(r[key])));
      return c.node;
    }));
  }
  search.addEventListener("input", render); render();
  return { node: el("section", {}, el("h3", {}, title), search, list), read: () => [...chosen] };
}
function issueTable(issues) {
  return dataTable(asRows(issues).map((i) => typeof i === "string" ? { message: i } : i), [["sheet", "工作表"], ["cell", "单元格"], ["name", "员工"], ["date", "日期"], ["message", "核对说明"]], [], { search: false });
}
async function options() { const o = await api("/hrm/options"); return { ...o, employees: asRows(o.employees), departments: asRows(o.departments) }; }
function executeDialog(title, fields, initial, save, refresh) {
  let popup, busy = false;
  const editor = form(fields, initial, async (data) => { if (busy) return; busy = true; try { await save(data); popup.close(); toast("操作已保存"); await refresh?.(); } finally { busy = false; } });
  popup = dialog(title, editor); return popup;
}

function samplingSegments(shift) {
  return [...(shift.segments || []).filter(segment => !segment.evidence_id), ...(shift.evidence_periods || []).map(period => ({...period,name:period.name || "连续采证时段",kind:"work"}))];
}
function publicationIssues(doc) {
  const issues = [], cycle = doc.config?.cycle || {}, scope = doc.scope || {};
  if (!validDate(doc.effective_from)) issues.push("生效日期未填写或无效：请在制度编辑页选择生效日期。");
  if (doc.effective_to && !validDate(doc.effective_to)) issues.push("结束日期无效：请在制度编辑页重新选择，或留空。");
  else if (doc.effective_to && validDate(doc.effective_from) && doc.effective_to < doc.effective_from) issues.push("结束日期不能早于生效日期。");
  if (!scope.employee_ids?.length && !scope.department_ids?.length && !scope.is_default) issues.push("适用范围未设置：请选择适用人员、部门，或明确设为默认考勤制度。");
  if (!doc.config?.shifts?.length) issues.push("班次未设置：请配置班次及工作时段。");
  if (cycle.mode === "weekly") {
    if (!Array.isArray(cycle.weekdays) || cycle.weekdays.length !== 7) issues.push("每周排班未完整设置：请逐项核对周一至周日的日期类型和班次。");
    else cycle.weekdays.forEach((day, index) => {
      if (!day || !categories.some(([value]) => value === day.category)) issues.push(`${weekdays[index]}日期类型未设置：请选择工作日、休息日或法定假日。`);
      else if ((day.category === "workday" || day.scheduled_work) && !day.shift_id && !day.candidate_shift_ids?.length && doc.config?.shift_selection !== "auto") issues.push(`${weekdays[index]}需要出勤，请选择班次。`);
      if (day?.shift_id && !doc.config.shifts?.some(shift => shift.id === day.shift_id)) issues.push(`${weekdays[index]}的班次已不存在，请重新选择。`);
    });
  } else if (cycle.mode && cycle.mode !== "cycle") issues.push("排班模式无效：请选择按星期或高级轮班周期。");
  else {
    if (!validDate(cycle.anchor)) issues.push("轮班周期第一天未填写或无效：请在排班周期中选择日期。");
    if (!Array.isArray(cycle.days) || !cycle.days.length || cycle.days.length > 366) issues.push("排班周期未设置：请填写1至366天的周期，并核对每天的班次或休息日。");
  }
  if (doc.config?.rule?.evidence_mode === "local") for (const shift of doc.config.shifts || []) issues.push(...inspectWindows(samplingSegments(shift)).issues.map(issue => `${shift.name}：${issue}`));
  if (asRows(doc.conflicts).length) issues.push(...asRows(doc.conflicts).map(item => "来源待确认事项：" + item));
  return issues;
}

async function publishRule(doc, opt) {
  const issues = publicationIssues(doc); let popup;
  const editRule = () => { popup.close(); ruleEditor(doc, opt); };
  if (issues.length) return popup = dialog("生效前检查", el("section", { "data-attendance-publish-preflight": "" }, el("p", {class:"notice"}, `“${doc.name}”还需要完善以下配置。`), el("ul", {}, issues.map(issue => el("li", {}, issue))), button("去完善制度", editRule, "primary")));
  const start = input({...dateFields[0]}, [doc.effective_from, "2026-10-01"].sort().at(-1)), end = input({...dateFields[1]}, doc.effective_to && doc.effective_to < today() ? doc.effective_to : today()), reason = input({key:"reason",label:"本次调整说明",type:"textarea",required:true}, doc.status === "published" ? "依据最新部门作息修订历史考勤" : "根据已核对的本地作息生效"), output = el("div"), errors = el("div"), content = el("section", {"data-attendance-apply":""});
  let previewKey = "", busy = false;
  const range = () => ({start_date:start.read(),end_date:end.read()});
  const apply = button("确认生效并自动计算", async () => {
    if (busy || !previewKey || previewKey !== JSON.stringify(range())) return;
    if (!reason.read().trim()) { errors.replaceChildren(errorNode(Error("请填写本次调整说明"))); return; }
    busy = true; apply.disabled = true; errors.replaceChildren();
    try { const data = await request(`/rules/${encodeURIComponent(doc.id)}/apply`, {...range(),reason:reason.read().trim()}); popup.close(); toast("制度已生效，排班与计算任务已提交"); await rules(); showCalculationProgress(data.batch_ids || []); }
    catch(error) { errors.replaceChildren(errorNode(error)); }
    finally { busy = false; apply.disabled = !previewKey; }
  }, "primary"); apply.disabled = true;
  const preview = button("预览影响范围", async () => {
    if (busy) return; busy = true; preview.disabled = true; apply.disabled = true; errors.replaceChildren();
    try {
      if (!validDate(start.read()) || !validDate(end.read()) || start.read() > end.read()) throw Error("请选择有效的开始、结束日期");
      const values = range(), data = await request(`/rules/${encodeURIComponent(doc.id)}/preview`, values);
      previewKey = data.can_apply ? JSON.stringify(values) : "";
      output.replaceChildren(details({"适用员工":data.employee_count || 0,"影响天数":data.day_count || 0,"回算日期":`${data.start_date} 至 ${data.end_date}`,"已锁定月份":asRows(data.locked_months).join("、") || "无","人工排班":data.manual_roster_count || 0}), issueTable(data.issues));
    } catch(error) { previewKey = ""; errors.replaceChildren(errorNode(error)); }
    finally { busy = false; preview.disabled = false; apply.disabled = !previewKey; }
  });
  for (const f of [start,end]) f.input.addEventListener("input", () => { previewKey="";apply.disabled=true;output.replaceChildren(el("p",{},"日期已改变，请重新预览。")); });
  content.append(el("p",{class:"notice"},"生效后自动生成排班并重新计算。历史已发布制度产生新版本，原版本和原始证据保留。2026年10月起回算；锁定月份、人工排班与待配置项会在预览中明确列出。"), errors, el("div",{class:"form-grid"},start.wrapper,end.wrapper),reason.wrapper,preview,output,apply,button("返回编辑制度",editRule));
  popup=dialog("保存并生效",content); preview.click(); return popup;
}
function showCalculationProgress(ids) {
  if (!ids.length) return;
  const body=el("section",{},el("p",{},"正在自动排班并计算，可关闭此窗口，任务会在后台继续。")), rows=el("div"); body.append(rows);
  const popup=dialog("自动计算进度",body);
  const refresh=async()=>{if(!popup.modal.isConnected)return;try{const batches=await Promise.all(ids.map(id=>request("/batches/"+encodeURIComponent(id))));if(!popup.modal.isConnected)return;rows.replaceChildren(dataTable(batches,[["start_date","开始日期"],["end_date","结束日期"],["status","状态",label],["completed_count","已完成"],["total_count","总数"],["error_message","说明"]],[],{search:false}),el("a",{class:"button",href:BASE+"hrm/attendance-results?batch_id="+encodeURIComponent(ids[0])},"查看考勤明细"));if(batches.some(b=>["queued","running"].includes(b.status)))setTimeout(refresh,3000);}catch(error){if(popup.modal.isConnected)rows.replaceChildren(errorNode(error),button("刷新进度",refresh));}};refresh();
}
function batchWindowEditor(ruleDocs, opt) {
  ruleDocs = ruleDocs.filter(doc => ["draft", "published"].includes(doc.status) && !referenceOnlyRule(doc));
  const targets=[];
  for(const doc of ruleDocs) for(const shift of doc.config?.shifts || []) for(const [collection,segments] of [["segments",shift.segments||[]],["evidence_periods",shift.evidence_periods||[]]]) for(const segment of segments) if(segment.kind!=="break"&&segment.punch_mode!=="none"&&!segment.evidence_id) targets.push({id:[doc.id,shift.id,collection,segment.id].join("::"),doc,shift,segment,collection,name:`${doc.name} / ${shift.name} / ${segment.name || "连续采证"}`});
  const selected=pickMany("选择制度和班段",targets,[],"id",r=>r.name), preview=el("div"), window=windowEditor({},()=>preview.replaceChildren(el("p",{class:"hint"},"只覆盖已填写的边界，未填写的边界保持原值；保存后请预览生效。"))), feedback=el("div"), content=el("section",{"data-batch-windows":""},el("p",{class:"notice"},"批量为选中班段设置相同取卡时间。每个制度单独保存新草稿，不改写历史版本。请只选打卡时间相同的班段。"),selected.node,window.node,preview,feedback);
  const templates=targets.filter(target=>Object.values(target.segment.local_windows||{}).some(value=>Number.isInteger(value))), template=input({key:"window_template",label:"使用已有窗口作为模板",options:[["","请选择已有班段"],...templates.map(target=>[target.id,target.name])]},"");
  content.append(el("div",{class:"toolbar"},template.wrapper,button("载入窗口模板",()=>{const chosen=templates.find(target=>target.id===template.read());if(chosen){window.write(chosen.segment.local_windows);toast("已载入模板，请核对日期和边界后保存");}})));
  const completed=new Set();let busy=false;const save=button("保存选中班段的窗口",async()=>{if(busy)return;feedback.replaceChildren();const chosen=new Set(selected.read()),entries=targets.filter(r=>chosen.has(r.id)),values=Object.fromEntries(Object.entries(window.read()).filter(([,value])=>value!==null));if(!entries.length||!Object.keys(values).length){feedback.replaceChildren(errorNode(Error("请勾选至少一个班段并填写要覆盖的窗口时间")));return;}busy=true;save.disabled=true;for(const control of content.querySelectorAll("input,select"))control.disabled=true;const reports=[];
    try{for(const source of [...new Set(entries.map(row=>row.doc))]){if(completed.has(source.id)){reports.push({name:source.name,status:"此前已保存",id:source.id});continue;}const doc=structuredClone(source);doc.config.rule={...doc.config.rule,evidence_mode:"local"};for(const target of entries.filter(row=>row.doc.id===source.id)){const segment=doc.config.shifts.find(s=>s.id===target.shift.id)[target.collection].find(s=>s.id===target.segment.id);segment.local_windows={...segment.local_windows,...values};}for(const shift of doc.config.shifts)if(shift.overtime_variant){const alternate=doc.config.shifts.find(s=>s.id===shift.overtime_variant.shift.id);if(alternate)shift.overtime_variant.shift=structuredClone(alternate);}try{const saved=await request("/rules",doc);completed.add(source.id);reports.push({name:source.name,status:"草稿已保存",id:saved.id});}catch(error){reports.push({name:source.name,status:error.message});}}
    feedback.replaceChildren(dataTable(reports,[["name","制度"],["status","保存结果"]],[],{search:false}),el("p",{},"成功项保留为草稿，返回考勤设置后点击发布查看生效与回算范围。"));}finally{busy=false;save.disabled=reports.length>0&&reports.every(row=>row.id);}} ,"primary");content.append(save);dialog("批量设置取卡窗口",content);
}

function referenceOnlyRule(doc) {
  let source = doc.source;
  if (typeof source === "string") { try { source = JSON.parse(source); } catch { return false; } }
  return !!source?.snapshot_id;
}
function cleanedUpRule(doc) {
  let source = doc.source;
  if (typeof source === "string") { try { source = JSON.parse(source); } catch { return false; } }
  return source?.cleanup?.hidden === true;
}
export async function rules() {
  return view("考勤设置", "配置作息和取卡范围，保存并生效后自动排班、计算。所有正常、迟到、早退和缺卡均由本系统判断。", async (panel, current) => {
    const [result, opt] = await Promise.all([request("/rules"), options()]); if (!current()) return;
    // Cleanup only hides the selected source records from this list. Their IDs,
    // versions and evidence remain available to historical results and audits.
    const visibleRules = asRows(result).filter(doc => !cleanedUpRule(doc));
    const writable = can("hrm", "write"), currentRules = visibleRules.filter(doc => ["draft", "published"].includes(doc.status) && !referenceOnlyRule(doc)), history = visibleRules.filter(doc => !["draft", "published"].includes(doc.status) || referenceOnlyRule(doc));
    const columns = [["name", "制度名称"], ["version", "版本"], ["status", "状态", label], ["effective_from", "生效日期"], ["effective_to", "结束日期"], ["conflicts", "待确认事项", (v) => asRows(v).length]];
    panel.replaceChildren(el("div", { class: "toolbar" }, button("刷新", rules), writable && button("新增制度", () => ruleEditor({}, opt), "primary"), writable && button("批量设置取卡窗口", () => batchWindowEditor(currentRules, opt))),
      el("section", {"data-attendance-current-rules":""}, dataTable(currentRules, columns,
        [{ label: writable ? "编辑 / 新版本" : "查看", run: (row) => ruleEditor(row, opt, !writable) }, ...(writable ? [{ label: "发布", when: (r) => r.status === "draft", run: (r) => publishRule(r, opt) }] : [])])),
      ...(history.length ? [el("details", {"data-attendance-rule-history":""}, el("summary", {}, `历史版本与旧来源（仅审计） · ${history.length}条`), el("p", {class:"hint"}, "历史制度保留原计算依据，仅供查看；不能修改、重新发布或批量设置窗口。"), dataTable(history, columns, [{label:"查看",run:row=>ruleEditor(row,opt,true)}]))] : []));
  });
}


function ruleEditor(doc, opt, readonly = false) {
  const config = structuredClone(doc.config || {}), rule = config.rule || {}, scope = doc.scope || {};
  // Audit views must show the saved historical schedule rather than infer a
  // new classification-based policy. Editable new versions still use the new UI.
  const historicalSchedule = readonly && !config.work_schedule;
  const fields = {}, grid = el("div", { class: "form-grid" }), content = el("form", { "data-attendance-rule-editor": "" }), feedback = el("div");
  const add = (key, title, value, type = "text", choices) => { const f = input({ key, label: title, type, required: key === "name" || key === "effective_from", ...(choices ? { options: choices } : {}), ...(type === "number" ? { min: 0, step: 1 } : {}) }, value); fields[key] = f; grid.append(f.wrapper); return f; };
  add("name", "制度名称", doc.name || ""); add("effective_from", "生效日期", doc.effective_from || (doc.id ? "" : today()), "date"); add("effective_to", "结束日期（可留空）", doc.effective_to || "", "date");
  add("category", "考勤人员类别", config.category || "员工", "text", [["员工", "生产员工"], ["生产职员", "生产职员"], ["职员", "普通职员"]]);
  add("attendance_mode", "出勤计算方式", rule.attendance_mode || "actual", "text", [["actual", "按有效出勤时间"], ["fixed", "按核定标准工时"], ["fixed_complete", "完整正班按标准工时，未满按实际"]]);
  add("complete_work_deduction_minutes", "完整正班固定扣除休息分钟", rule.complete_work_deduction_minutes || 0, "number");
  add("standard_minutes", "标准正班分钟（0为班次时段合计）", rule.standard_minutes || 0, "number");
  add("punch_selection", "窗口内打卡选择", rule.punch_selection || "first_last", "text", [["first_last", "最早上班 / 最晚下班"], ["earliest", "最早记录"], ["latest", "最晚记录"], ["nearest", "最接近计划时间"]]);
  add("late_grace_minutes", "迟到宽限分钟", rule.late_grace_minutes || 0, "number"); add("early_grace_minutes", "早退宽限分钟", rule.early_grace_minutes || 0, "number");
  add("grace_mode", "宽限认定", rule.grace_mode || "normal", "text", [["normal", "按正常认定"], ["grace", "保留宽限认定"]]);
  for (const [key, title] of [["late_deduction_mode", "迟到扣时"], ["early_deduction_mode", "早退扣时"]]) add(key, title, rule[key] || "none", "text", [["none", "不直接扣时"], ["excess", "仅扣超出宽限部分"], ["full", "按全部时长扣时"]]);
  add("flex_minutes", "弹性上下班分钟", rule.flex_minutes || 0, "number");
  add("overtime_date_mode", "跨日加班归属", rule.overtime_date_mode || "workdate", "text", [["workdate", "归属排班工作日"], ["calendar", "按自然日拆分"]]);
  const flags = Object.fromEntries([["credit_out", "批准外出计入出勤"], ["credit_trip", "批准出差计入出勤"], ["allow_abnormal_punches", "允许异常打卡作为计算证据"]].map(([k, text]) => [k, check(text, rule[k])]));
  
  const scopeForm = scopeEditor(scope, opt);
  let fixedSchedule;
  const shiftsNode = el("div"), shiftRows = [], shiftSelects = [];
  const newID = (prefix) => prefix + "-" + crypto.randomUUID();
  const refillSelects = () => { for (const select of shiftSelects) { const old = select.value; select.replaceChildren(el("option", { value: "" }, "休息 / 无班次"), ...shiftRows.map((r) => el("option", { value: r.id }, r.name.read() || "未命名班次"))); select.value = [...select.options].some((o) => o.value === old) ? old : ""; } fixedSchedule?.refresh(); };
  const shiftSelect = (value) => { const select = el("select"); shiftSelects.push(select); refillSelects(); select.value = value || ""; return select; };
  function addShift(saved = {}) {
    if (saved.id && shiftRows.some((r) => r.id === saved.id)) return;
    const entry = { original: saved, id: saved.id || newID("shift"), version: saved.version || "1", source_group_id: saved.source_group_id, source_schedule_id: saved.source_schedule_id, name: input({ key: "shift_name", label: "班次名称", required: true }, saved.name || "白班"), segments: [], evidenceRows: [] };
    const section = el("section", { class: "surface" }), segmentHolder = el("div"), timeline = el("div");
    const refreshTimeline = () => timeline.replaceChildren(windowTimeline(samplingSegments({segments:entry.segments.map(item=>item.read()),evidence_periods:entry.evidenceRows.map(item=>item.read())})));
    const evidenceHolder=el("div"), evidenceSelects=[];
    const updateEvidenceChoices=()=>{for(const select of evidenceSelects){const selected=select.value;select.replaceChildren(el("option",{value:""},"独立取卡"),...entry.evidenceRows.map(row=>el("option",{value:row.id},row.name.read()||"连续采证")));select.value=selected;}};
    function addEvidence(period={}) {
      const id=period.id||newID("evidence"), name=input({key:"evidence_name",label:"采证时段名称"},period.name||"连续采证"), start=windowClock(period.start_second), end=windowClock(period.end_second), controls={};
      const row=el("section",{class:"surface","data-evidence-period":""},name.wrapper), formGrid=el("div",{class:"form-grid"});
      for(const [key,title,item]of [["start","开始",start],["end","结束",end]]){controls[key]=input({key:"evidence_"+key,label:title+"时间",type:"time",step:1},item.time);controls[key+"_day"]=input({key:"evidence_"+key+"_day",label:title+"日期",options:[[-1,"前日"],[0,"当日"],[1,"次日"],[2,"第3日"],[3,"第4日"]]},item.day);formGrid.append(controls[key+"_day"].wrapper,controls[key].wrapper);}
      const mode=input({key:"evidence_punch_mode",label:"采证要求",options:[["both","首尾都打卡"],["in","仅开始卡"],["out","仅结束卡"],["none","免打卡"]]},period.punch_mode||"both"), windows=windowEditor(period.local_windows,refreshTimeline), item={id,name,read:()=>({...period,id,name:name.read(),start_second:windowSecond(controls.start_day.read(),controls.start.read()),end_second:windowSecond(controls.end_day.read(),controls.end.read()),punch_mode:mode.read(),local_windows:windows.read()})};
      row.append(formGrid,mode.wrapper,windows.node,button("移除采证时段",()=>{entry.evidenceRows.splice(entry.evidenceRows.indexOf(item),1);row.remove();updateEvidenceChoices();refreshTimeline();}));entry.evidenceRows.push(item);evidenceHolder.append(row);name.input.addEventListener("input",updateEvidenceChoices);mode.input.addEventListener("change",()=>windows.mode(mode.read(),"work"));windows.mode(mode.read(),"work");updateEvidenceChoices();
    }
    for(const period of saved.evidence_periods||[])addEvidence(period);
    const addSegment = (segment = {}) => {
      const row = el("div", { class: "surface" }), f = {}, d = (key, title, type, value, choices) => { f[key] = input({ key, label: title, type, ...(choices ? { options: choices } : {}), ...(type === "number" ? { min: 0, max: 3, step: 1 } : {}) }, value); return f[key].wrapper; };
      const minute = (n) => `${String(Math.floor((n || 0) % 1440 / 60)).padStart(2, "0")}:${String((n || 0) % 60).padStart(2, "0")}`;
      row.append(el("div", { class: "form-grid" }, d("name", "时段名称", "text", segment.name || "正班"), d("kind", "时段类型", "text", segment.kind || "work", [["work", "正班"], ["break", "休息"], ["overtime", "加班"]]), d("start", "开始时间", "time", minute(segment.start_minute ?? 510)), d("start_day", "开始日期", "number", Math.floor((segment.start_minute || 0) / 1440), [[0,"当日"],[1,"次日"],[2,"第3日"],[3,"第4日"]]), d("end", "结束时间", "time", minute(segment.end_minute ?? 720)), d("end_day", "结束日期", "number", Math.floor((segment.end_minute || 0) / 1440), [[0,"当日"],[1,"次日"],[2,"第3日"],[3,"第4日"]]), d("punch_mode", "打卡要求", "text", segment.punch_mode || "both", [["both", "上班及下班"], ["in", "仅上班"], ["out", "仅下班"], ["none", "免打卡"]])));
      const openEnded = check("开放结束：以实际下班卡为止（结束时间仅作最晚取卡边界）", segment.open_ended);
      const overtimeStart = minuteTimeInput("overtime_start_minute", "加班开始计时（留空沿用时段开始）", segment.overtime_start_minute || 0);
      row.append(openEnded.node, overtimeStart.wrapper);
      const evidenceSelect=el("select",{name:"evidence_id"});evidenceSelects.push(evidenceSelect);updateEvidenceChoices();evidenceSelect.value=segment.evidence_id||"";
      row.append(el("label",{class:"field"},"打卡依据（连续采证适用于只打首尾卡）",evidenceSelect));
      const readTime = (key) => { const [h, m] = f[key].read().split(":").map(Number); return h * 60 + m + f[key + "_day"].read() * 1440; };
      const local = windowEditor(segment.local_windows || segment.pending_local_windows, refreshTimeline, value => {
        let count = 0;
        for (const shift of shiftRows) for (const other of shift.segments) { const target = other.read(); if (target.start_minute === readTime("start") && target.end_minute === readTime("end") && target.punch_mode === f.punch_mode.read()) { other.writeWindows(value); count++; } }
        toast(`已复制到${count}个相同作息时段，请检查时间轴`);
      });
      const item = { writeWindows: value => local.write(value), read: () => ({ ...segment, id: segment.id || (segment.id = newID("segment")), name: f.name.read(), kind: f.kind.read(), start_minute: readTime("start"), end_minute: readTime("end"), punch_mode: f.punch_mode.read(), evidence_id:evidenceSelect.value, open_ended: openEnded.read(), overtime_start_minute: overtimeStart.read(), local_windows: local.read() }) };
      const update = () => { local.mode(f.punch_mode.read(), f.kind.read()); local.node.hidden=!!evidenceSelect.value; openEnded.node.hidden=f.kind.read()!=="overtime"; overtimeStart.wrapper.hidden=f.kind.read()!=="overtime"; refreshTimeline(); }; evidenceSelect.addEventListener("change",update);
      for (const control of Object.values(f)) control.input.addEventListener("input", update);
      row.append(local.node, button("移除此时段", () => { entry.segments.splice(entry.segments.indexOf(item), 1); row.remove(); refreshTimeline(); })); entry.segments.push(item); segmentHolder.append(row); update();
    };
    entry.name.input.addEventListener("input", refillSelects);
    entry.variant = check("有晚间打卡时使用另一班次", !!saved.overtime_variant);
    entry.trigger = minuteTimeInput("variant_trigger", "晚间实际卡识别起点", saved.overtime_variant?.trigger_minute || 1065);
    entry.variantShift = shiftSelect(saved.overtime_variant?.shift?.id);
    section.append(entry.name.wrapper, el("details",{},el("summary",{},"高级：连续采证 / 只打首尾卡"),el("p",{class:"hint"},"配置首尾采证时段，再在其覆盖的工作段选择共用此依据。休息和加班时段仍分别核算。"),evidenceHolder,button("增加连续采证时段",()=>addEvidence())), segmentHolder, timeline, button("增加时段", () => addSegment()), el("details",{open:!!saved.overtime_variant},el("summary",{},"高级：晚间实际打卡条件班次"),entry.variant.node,entry.trigger.wrapper,el("label",{class:"field"},"识别到晚间卡后的班次",entry.variantShift)), button("移除此班次", () => { shiftRows.splice(shiftRows.indexOf(entry), 1); section.remove(); refillSelects(); }));
    shiftRows.push(entry); shiftsNode.append(section);
    for (const segment of saved.segments || [{ name: "上午", kind: "work", start_minute: 510, end_minute: 720 }, { name: "午休", kind: "break", start_minute: 720, end_minute: 810, punch_mode: "none" }, { name: "下午", kind: "work", start_minute: 810, end_minute: 1080 }]) addSegment(segment);
    refillSelects();
    if (saved.overtime_variant?.shift && !shiftRows.some((r) => r.id === saved.overtime_variant.shift.id)) addShift(saved.overtime_variant.shift);
    entry.variantShift.value = saved.overtime_variant?.shift?.id || "";
  }
  for (const shift of config.shifts?.length ? config.shifts : [{}]) addShift(shift);
  const savedCycle = config.cycle || {}, convertedWeek = historicalSchedule ? null : legacyWeekdays(savedCycle);
  const autoShift = check("按实际打卡自动识别白班 / 夜班（证据不足或多个候选转人工复核）", config.shift_selection === "auto");
  const scheduleMode = input({ key: "schedule_mode", label: "排班方式", options: [["weekly", "按星期（周一至周日）"], ["cycle", "高级轮班周期（大小周 / AB班）"]] }, savedCycle.mode === "weekly" || convertedWeek || !doc.id ? "weekly" : "cycle");
  const anchor = input({ key: "anchor", label: "周期第一天", type: "date", required: true }, config.cycle?.anchor || (doc.id ? "" : today()));
  const length = input({ key: "cycle_length", label: "周期天数（7为每周、14为大小周）", type: "number", min: 1, max: 366 }, config.cycle?.days?.length || 7);
  const cycleNode = el("div", { class: "form-grid" }); let cycleInputs = [];
  function renderCycle(values = cycleInputs.map((s) => s.value)) {
    const n = historicalSchedule ? values.length : Math.min(366, Math.max(1, length.read() || 7)); length.input.value = n; cycleNode.replaceChildren(); cycleInputs = [];
    for (let i = 0; i < n; i++) { const date = new Date(anchor.read() + "T00:00:00Z"); date.setUTCDate(date.getUTCDate() + i); const week = ["日", "一", "二", "三", "四", "五", "六"][date.getUTCDay()] || "待定"; const select = shiftSelect(values[i] ?? ""); cycleInputs.push(select); cycleNode.append(el("label", { class: "field" }, `第${i + 1}天 · 周${week}`, select)); }
  }
  renderCycle(historicalSchedule ? asRows(savedCycle.days) : config.cycle?.days || Array.from({ length: 7 }, (_, i) => { const d = new Date(anchor.read() + "T00:00:00Z"); d.setUTCDate(d.getUTCDate() + i); return [0, 6].includes(d.getUTCDay()) ? "" : shiftRows[0]?.id || ""; }));
  anchor.input.addEventListener("change", () => renderCycle()); length.input.addEventListener("change", () => renderCycle());
  const advancedNode = el("section", { "data-attendance-cycle": "" }, el("p", { class: "notice" }, historicalSchedule ? (asRows(savedCycle.days).length ? "按原周期与起点显示；仅供历史审计，不转换为新的固定周休。" : "此历史制度未保存周期天数或班次配置，不推定为周一至周五出勤。") : "大小周或AB轮班使用连续天数及第一天定位。普通每周排班可切换为按星期，无需填写周期起点。"), anchor.wrapper, length.wrapper, cycleNode);
  const weeklyNode = el("section", { "data-attendance-weekly": "" }); let weeklyInputs = [];
  function renderWeekly(values) {
    weeklyNode.replaceChildren(el("p", { class: "notice" }, "从周一到周日配置，每周重复。休息日可保留班次用于识别实际加班；不勾选要求出勤时，无打卡不产生缺勤。")); weeklyInputs = [];
    for (let index = 0; index < 7; index++) {
      const saved = values[index] || {}, category = input({ key: `weekly_category_${index}`, label: weekdays[index] + "日期类型", required: true, options: [["", "请选择日期类型"], ...categories] }, saved.category || "");
      const shift = shiftSelect(saved.shift_id || ""); shift.name = `weekly_shift_${index}`;
      const candidateNote=el("div",{class:"hint"});
      function updateCandidates(){candidateNote.replaceChildren();if(saved.candidate_shift_ids?.length)candidateNote.append(el("p",{},"此日自动识别："+saved.candidate_shift_ids.map(id=>shiftRows.find(r=>r.id===id)?.name.read()||"待核对班次").join("、")),button("改用固定班次",()=>{delete saved.candidate_shift_ids;updateCandidates();refresh();}));}
      const scheduled = check("休息日 / 假日仍要求出勤", !!saved.scheduled_work); scheduled.input.name = `weekly_scheduled_${index}`;
      const refresh = () => { const work = category.read() === "workday"; scheduled.node.hidden = work; scheduled.node.style.display = work ? "none" : ""; scheduled.input.disabled = work || scheduleMode.read() !== "weekly"; if (work) scheduled.input.checked = true; shift.required = (work || scheduled.read()) && !autoShift.read() && !saved.candidate_shift_ids?.length; shift.disabled = scheduleMode.read() !== "weekly" || autoShift.read() || !!saved.candidate_shift_ids?.length; };
      category.input.addEventListener("change", () => { scheduled.input.checked = category.read() === "workday"; refresh(); }); scheduled.input.addEventListener("change", refresh);
      weeklyInputs.push({ refresh, read: () => ({ ...saved, shift_id: shift.value, category: category.read(), scheduled_work: category.read() === "workday" || scheduled.read() }) });
      weeklyNode.append(el("div", { class: "form-grid", "data-weekday": String(index) }, category.wrapper, el("label", { class: "field" }, weekdays[index] + "班次（休息日可选）", shift), candidateNote, scheduled.node)); updateCandidates(); refresh();
    }
  }
  renderWeekly(savedCycle.mode === "weekly" ? asRows(savedCycle.weekdays) : convertedWeek || (!doc.id ? weekdays.map((_, index) => ({ shift_id: index < 5 ? shiftRows[0]?.id || "" : "", category: index < 5 ? "workday" : "restday", scheduled_work: index < 5 })) : []));
  function updateScheduleMode() {
    const fixed = !!fixedSchedule?.active(), weekly = scheduleMode.read() === "weekly";
    scheduleMode.wrapper.hidden = fixed; autoShift.node.hidden = fixed;
    if (fixed) {
      weeklyNode.hidden = true; advancedNode.hidden = true;
      for (const control of [...weeklyNode.querySelectorAll("input,select"), ...advancedNode.querySelectorAll("input,select")]) control.disabled = true;
      return;
    }
    weeklyNode.hidden = !weekly; advancedNode.hidden = weekly;
    for (const control of weeklyNode.querySelectorAll("input,select")) control.disabled = !weekly;
    for (const control of advancedNode.querySelectorAll("input,select")) control.disabled = weekly;
    for (const row of weeklyInputs) row.refresh();
  }
  scheduleMode.input.addEventListener("change", updateScheduleMode); autoShift.input.addEventListener("change",()=>weeklyInputs.forEach(row=>row.refresh())); updateScheduleMode();
  const scheduleCategory = () => historicalSchedule ? "" : fields.category.read();
  fixedSchedule = workScheduleEditor({ ...config, category: scheduleCategory() }, scheduleCategory, () => shiftRows.map(row => ({ id: row.id, name: row.name.read() })), updateScheduleMode);
  const customSchedule = el("section", {"data-custom-schedule":""}, el("h3", {}, historicalSchedule ? "历史作息配置" : "普通职员：每周作息 / 特殊轮班"), autoShift.node, scheduleMode.wrapper, ...(convertedWeek ? [el("p", {class:"notice"}, "原7天周期已按真实星期展开，保存后采用按星期排班。")]:[]), weeklyNode, advancedNode);
  const updateCategory = () => { fixedSchedule.refresh(); customSchedule.hidden = fixedSchedule.active(); updateScheduleMode(); };
  fields.category.input.addEventListener("change", updateCategory); updateCategory();
  const calendarNode = el("div"), calendarRows = [];
  function addCalendar(date = "", value = {}) { const day = input({ key: "calendar_date", label: "覆盖日期", type: "date", required: true }, date), shift = shiftSelect(value.shift_id), category = input({ key: "calendar_category", label: "日期类型", options: categories }, value.category || "workday"), row = el("div", { class: "form-grid" }); const scheduled = check("休息日仍要求出勤", value.scheduled_work); const item = { read: () => [day.read(), { shift_id: shift.value, category: category.read(), scheduled_work: scheduled.read() }] }; row.append(day.wrapper, el("label", { class: "field" }, "班次", shift), category.wrapper, scheduled.node, button("移除此日期", () => { calendarRows.splice(calendarRows.indexOf(item), 1); row.remove(); })); calendarRows.push(item); calendarNode.append(row); }
  for (const [date, value] of Object.entries(config.calendar || {})) addCalendar(date, value);
  const otRows = categories.map(([key, text]) => {
    const p = rule.overtime?.[key] || {}, enabled = check("计算加班", p.enabled ?? true), approve = check("必须有已通过的加班凭据", p.require_approval ?? true), rounding = input({ key: "rounding_scope", label: "取整范围", options: [["daily", "当日合计取整"], ["segment", "每个时段取整"]] }, p.rounding_scope || "daily"), maximum = input({ key: "maximum_minutes", label: "每日加班上限分钟（0为未设）", type: "number", min: 0 }, p.maximum_minutes || 0);
    const complete = key === "restday" ? input({ key: "complete_workday_minutes", label: "休息日完整白班核定加班分钟（0为按实际）", type: "number", min: 0, max: 1440, step: 1 }, p.complete_workday_minutes || 0) : null;
    return { key, apply: value => { enabled.input.checked = !!value.enabled; approve.input.checked = !!value.require_approval; rounding.input.value = value.rounding_scope; maximum.input.value = value.maximum_minutes; if (complete) complete.input.value = value.complete_workday_minutes || 0; }, node: el("section", { "data-overtime-category": key }, el("h3", {}, text), enabled.node, approve.node, rounding.wrapper, maximum.wrapper, complete?.wrapper, complete && el("p", { class: "notice" }, "生产一部完整白班核定480分钟；部分白班按有效实际时间，缺卡须复核。晚间从18:00起每满30分钟计0.5小时，余数舍去。")), read: () => [key, { ...p, enabled: enabled.read(), require_approval: approve.read(), minimum_minutes: 30, rounding_minutes: 30, rounding_scope: rounding.read(), maximum_minutes: maximum.read(), ...(complete ? { complete_workday_minutes: complete.read() } : {}) }] };
  });
  const leaveNode = el("div"), leaveRows = [];
  function addLeave(name = "", policy = {}) { const type = input({ key: "leave_type", label: "请假类型", required: true }, name), credit = check("计入核定出勤", policy.credit_attendance), full = check("不影响全勤", policy.full_attendance_allowed), row = el("div", { class: "toolbar" }); const item = { read: () => [type.read().trim(), { credit_attendance: credit.read(), full_attendance_allowed: full.read() }] }; row.append(type.wrapper, credit.node, full.node, button("移除假别", () => { leaveRows.splice(leaveRows.indexOf(item), 1); row.remove(); })); leaveRows.push(item); leaveNode.append(row); }
  for (const [name, policy] of Object.entries(rule.leave_policies || {})) addLeave(name, policy);
  const full = rule.full_attendance || {}, fullFlags = Object.fromEntries([["allow_out", "外出不影响全勤"], ["allow_trip", "出差不影响全勤"], ["allow_late", "迟到不影响全勤"], ["allow_early", "早退不影响全勤"], ["allow_missing", "缺卡不影响全勤"]].map(([k, text]) => [k, check(text, full[k])]));
  const allowedLeave = input({ key: "allowed_leave_types", label: "允许全勤的假别（逗号分隔）" }, asRows(full.allowed_leave_types).join("，"));
  const tierNode = el("div"), tiers = [];
  const lateDeparture=input({key:"late_departure_after",label:"前日下班严格晚于此时间",type:"time",step:1},windowClock(rule.late_departure_after_second).time), lateDelay=input({key:"late_departure_delay_minutes",label:"次日允许晚到分钟",type:"number",min:0},rule.late_departure_delay_minutes||0);
  const departureNote=el("p",{class:"hint"},"按实际下班卡判定，不依赖已核定加班小时；例如21:30:00不触发，21:30:01才触发。留空不启用。" );
  function addTier(saved = {}) { const previous = input({ key: "previous_overtime_minutes", label: "前日加班达到分钟", type: "number", min: 0 }, saved.previous_overtime_minutes || 0), delay = input({ key: "delay_minutes", label: "次日晚到分钟", type: "number", min: 0 }, saved.delay_minutes || 0), row = el("div", { class: "toolbar" }); const item = { read: () => ({ previous_overtime_minutes: previous.read(), delay_minutes: delay.read() }) }; row.append(previous.wrapper, delay.wrapper, button("移除此档", () => { tiers.splice(tiers.indexOf(item), 1); row.remove(); })); tiers.push(item); tierNode.append(row); }
  for (const tier of rule.late_arrival_tiers || []) addTier(tier);
  const partialMode=input({key:"partial_work_mode",label:"不足整班的餐休处理",options:[["actual","按有效实际核定"],["review","未明确餐休部分待复核"]]},rule.partial_work_policy?.mode||"actual"), partialRows=[], partialHolder=el("div");
  function addPartialRange(range={}) {const start=windowClock(range.start_second),end=windowClock(range.end_second),startTime=input({key:"partial_start",label:"可按实际计算的开始时间",type:"time",step:1},start.time),endTime=input({key:"partial_end",label:"结束时间（不含该秒）",type:"time",step:1},end.time),startDay=input({key:"partial_start_day",label:"开始日期",options:[[-1,"前日"],[0,"当日"],[1,"次日"],[2,"第3日"]]},start.day),endDay=input({key:"partial_end_day",label:"结束日期",options:[[-1,"前日"],[0,"当日"],[1,"次日"],[2,"第3日"]]},end.day),row=el("div",{class:"form-grid"}),item={read:()=>({start_second:windowSecond(startDay.read(),startTime.read()),end_second:windowSecond(endDay.read(),endTime.read())})};row.append(startDay.wrapper,startTime.wrapper,endDay.wrapper,endTime.wrapper,button("移除此范围",()=>{partialRows.splice(partialRows.indexOf(item),1);row.remove();}));partialRows.push(item);partialHolder.append(row);}
  for(const range of rule.partial_work_policy?.actual_ranges||[])addPartialRange(range);
  const partialNode=el("details",{},el("summary",{},"高级：不足整班的核定范围"),partialMode.wrapper,el("p",{class:"hint"},"选择待复核时，只有下列范围内的部分出勤可直接按实际计算；其他部分需要核对餐休。这些范围不会新增休息扣除。"),partialHolder,button("增加按实际计算的范围",()=>addPartialRange()));
  const conflicts = asRows(doc.conflicts), resolved = check("已逐项核对并处理上述来源冲突", false);
  const advancedGeneral = el("details", {}, el("summary", {}, "高级：出勤核定、宽限与弹性"), el("div", { class: "form-grid" }, ...Object.entries(fields).filter(([key]) => !["name", "effective_from", "effective_to", "category"].includes(key)).map(([, f]) => f.wrapper)), ...Object.values(flags).filter(f => f !== flags.allow_abnormal_punches).map(f => f.node));
  content.append(feedback, el("p", {class:"notice"}, historicalSchedule ? "仅查看原制度的作息配置，不套用新的固定工作日；历史原始数据保持不变。" : "流程：选择适用范围 → 核对作息与取卡窗口 → 保存并生效。生产员工和生产职员自动按固定工作日核算，不必填写周期。"), grid, scopeForm.node,
    ...(!readonly && doc.id && rule.evidence_mode !== "local" ? [el("p",{class:"notice","data-local-conversion-notice":""},"这是一份历史制度。保存新草稿后将使用本地打卡窗口；请先补齐各时段窗口，未配置时不能生效。原已发布版本与原始记录保留。")]:[]), fixedSchedule.node, customSchedule, advancedGeneral, partialNode,
    el("h3", {}, "班次与时段（可选择次日结束）"), button("载入生产一部有无加班模板", () => {
      shiftRows.splice(0); shiftsNode.replaceChildren();
      const preset = productionFirstDepartmentPreset();
      for (const shift of preset.config.shifts) addShift(shift);
      for (const policy of otRows) policy.apply(preset.config.rule.overtime[policy.key]);
      fields.category.input.value = "员工"; fixedSchedule.replace(preset.config); scheduleMode.input.value = "weekly"; renderWeekly(preset.config.cycle.weekdays); updateCategory(); toast("已载入生产员工草稿：周一至周五正班，周末按实际出勤计加班；请核对范围和窗口后生效");
    }), shiftsNode, button("增加班次", () => addShift()),
    el("details",{},el("summary",{},"高级：节假日 / 单日覆盖"),calendarNode,button("增加覆盖日期",()=>addCalendar())), el("h3", {}, "加班认定"), el("p", { class: "notice" }, "晚间加班满30分钟计0.5小时，不足部分不计。休息日白班按下方规则单独认定。勾选“必须有已通过的加班凭据”时，无审批不计加班。正常与异常全部按本地制度计算。"), ...otRows.map((r) => r.node),
    el("details",{},el("summary",{},"高级：请假与全勤"),leaveNode,button("增加假别",()=>addLeave()),allowedLeave.wrapper,...Object.values(fullFlags).map(f=>f.node)),
    el("details",{},el("summary",{},"高级：前日晚走与次日晚到"),el("div",{class:"form-grid"},lateDeparture.wrapper,lateDelay.wrapper),departureNote,el("h4",{},"其他按加班时长设置的档位"),tierNode,button("增加档位",()=>addTier())));
  if (conflicts.length) content.append(el("h3", {}, "来源待确认事项"), el("ul", {}, conflicts.map((text) => el("li", {}, text))), resolved.node);
  if (doc.source) content.append(el("details", {}, el("summary", {}, "查看保留的来源规则 / 单元格"), sourceEvidence(doc.source)));
  const save = el("button", { type: "submit", class: "button" }, "保存草稿"), apply = el("button", { type: "submit", class: "button primary", "data-save-apply": "" }, "保存并生效");
  if (!readonly) content.append(el("footer", { class: "form-actions" }, apply, save)); else for (const control of content.querySelectorAll("input,select,button")) control.disabled = true;
  const popup = dialog(doc.name || "新增考勤制度", content); popup.modal.classList.add("attendance-rule-dialog"); let saving = false;
  content.addEventListener("submit", async (event) => {
    event.preventDefault(); if (readonly || saving || !content.reportValidity()) return; saving = true; save.disabled = true; apply.disabled = true; feedback.replaceChildren();
    try {
      if (!fields.name.read().trim()) throw Error("请填写制度名称");
      if (conflicts.length && !resolved.read()) throw Error("请先核对来源冲突");
      const configured = { ...rule, evidence_mode: "local", late_departure_after_second:windowSecond(0,lateDeparture.read()), late_departure_delay_minutes:lateDelay.read(), partial_work_policy:{mode:partialMode.read(),actual_ranges:partialRows.map(row=>row.read())}, overtime: Object.fromEntries(otRows.map((r) => r.read())), leave_policies: Object.fromEntries(leaveRows.map((r) => r.read())), late_arrival_tiers: tiers.map((r) => r.read()), full_attendance: { ...Object.fromEntries(Object.entries(fullFlags).map(([k, v]) => [k, v.read()])), allowed_leave_types: allowedLeave.read().split(/[,，;；\n]/).map((s) => s.trim()).filter(Boolean) }, ...Object.fromEntries(Object.entries(flags).map(([k, f]) => [k, f.read()])) };
      for (const [key, value] of Object.entries(fields)) if (!["name", "effective_from", "effective_to", "category"].includes(key)) configured[key] = value.read();
      const shifts = shiftRows.map((r) => ({ ...r.original, id: r.id, name: r.name.read(), version: r.version, source_group_id: r.source_group_id, source_schedule_id: r.source_schedule_id, segments: r.segments.map((s) => s.read()), evidence_periods:r.evidenceRows.map(row=>row.read()) }));
      for (const shift of shifts) delete shift.overtime_variant;
      for (const [index, r] of shiftRows.entries()) if (r.variant.read()) {
        const alternate = shifts.find((s) => s.id === r.variantShift.value);
        if (!alternate || alternate.id === r.id || shiftRows.find((s) => s.id === alternate.id)?.variant.read()) throw Error("条件班次须选择另一个未配置嵌套条件的班次");
        shifts[index].overtime_variant = { trigger_minute: r.trigger.read(), shift: structuredClone(alternate) };
      }
      if (shifts.some((s) => !s.segments.length || s.segments.some((p) => !Number.isFinite(p.start_minute) || p.end_minute <= p.start_minute))) throw Error("请检查时段，跨日结束需将结束日偏移设为1或更大");
      const workSchedule = fixedSchedule.read();
      const cycle = workSchedule ? workScheduleCycle(workSchedule) : scheduleMode.read() === "weekly" ? { mode: "weekly", weekdays: weeklyInputs.map(row => row.read()) } : { mode: "cycle", anchor: anchor.read(), days: cycleInputs.map((s) => s.value) };
      const saved = await request("/rules", { ...doc, name: fields.name.read().trim(), effective_from: fields.effective_from.read(), effective_to: fields.effective_to.read(), conflicts: [], config: { ...config, work_schedule:workSchedule, shift_selection:autoShift.read()?"auto":"", rule: configured, shifts, category: fields.category.read(), cycle, calendar: Object.fromEntries(calendarRows.map((r) => r.read())) }, scope: scopeForm.read() });
      popup.close(); toast("制度草稿已保存"); await rules(); if (event.submitter === apply) await publishRule(saved, opt);
    } catch (error) { feedback.replaceChildren(errorNode(error)); } finally { saving = false; save.disabled = false; apply.disabled = false; }
  });
}

export async function rosters() {
  return view("人员排班", "先预览制度匹配与周期结果；无冲突后保存。已月结月份不能直接调整。", async (panel, current) => {
    const [opt, ruleList] = await Promise.all([options(), request("/rules")]); if (!current()) return;
    const result = el("div"), range = {}, controls = el("div", { class: "toolbar" });
    for (const def of dateFields) { range[def.key] = input(def, def.key === "start_date" ? today().slice(0, 8) + "01" : today()); controls.append(range[def.key].wrapper); }
    const people = pickMany("排班人员（不选则为权限范围内全部）", opt.employees);
    const values = () => ({ start_date: range.start_date.read(), end_date: range.end_date.read(), employee_ids: people.read() });
    const rosterTable = (rows) => dataTable(rows, [["employee", "员工", (v, r) => personText(v || opt.employees.find((e) => e.id === r.employee_id))], ["work_date", "工作日期"], ["rule_id", "制度", (id) => asRows(ruleList).find((r) => r.id === id)?.name || "待核对"], ["shift_id", "班次", (id, r) => asRows(ruleList).find((x) => x.id === r.rule_id)?.config?.shifts?.find((x) => x.id === id)?.name || "休息"], ["category", "日期类型", label], ["status", "状态", label]], can("hrm", "write") ? [{ label: "调整", run: (r) => rosterEdit(r, opt, asRows(ruleList), load) }] : []);
    const load = async () => { result.replaceChildren(el("p", {}, "正在加载…")); try { const rows = await request("/rosters?" + queryString({ ...values(), employee_ids: undefined, employee_id: people.read().length === 1 ? people.read()[0] : "" })); if (current()) result.replaceChildren(rosterTable(asRows(rows))); } catch (e) { if (current()) result.replaceChildren(errorNode(e)); } };
    let previewBusy = false;
    const generate = async () => {
      if (previewBusy) return; previewBusy = true;
      try { const planned = values(), p = await request("/rosters/generate", { ...planned, preview: true });
        const content = el("section", {}, el("p", {}, `候选排班 ${asRows(p.items).length} 条，待处理 ${asRows(p.issues).length} 项`), issueTable(p.issues), rosterTable(asRows(p.items))), feedback = el("div"), popup = dialog("排班预览", content);
        if (!asRows(p.issues).length) { let busy = false; const confirm = button("确认保存排班", async () => { if (busy) return; busy = true; confirm.disabled = true; feedback.replaceChildren(); try { const saved = await request("/rosters/generate", { ...planned, preview: false }); if (!saved.saved) throw Error("规则或人员已变化，请重新预览"); popup.close(); toast("排班已保存"); await load(); } catch (error) { feedback.replaceChildren(errorNode(error)); } finally { busy = false; confirm.disabled = false; } }, "primary"); content.append(feedback, confirm); }
      } finally { previewBusy = false; }
    };
    controls.append(button("查询排班", load), can("hrm", "write") && button("生成并预览", safe(generate), "primary"), can("hrm", "write") && button("按实际卡推荐班次", () => rosterCandidates(opt, asRows(ruleList), load))); panel.replaceChildren(controls, people.node, result); await load();
  });
}
function rosterCandidates(opt, rulesList, refresh) {
  const results = el("div"), content = el("section", {}, el("p", { class: "notice" }, "候选班次仅作建议，不会自动保存排班。请核对当日原始打卡并填写人工确认原因。"));
  const popup = dialog("候选班次预览", content);
  content.append(form([{ key: "employee_id", label: "员工", options: employeeOptions(opt.employees, "请选择员工"), required: true }, { key: "date", label: "工作日期", type: "date", required: true }], { date: today() }, async (values) => {
    const data = await request("/rosters/candidates", values);
    results.replaceChildren(dataTable(asRows(data), [["rule_name", "制度"], ["shift_name", "候选班次"], ["score", "匹配评分"], ["preview", "状态", (r) => label(r?.state)], ["preview", "核定小时", (r) => hours(r?.credited_seconds)], ["preview", "缺卡", (r) => r?.missing_count || 0], ["preview", "核对事项", (r) => asRows(r?.issues).join("；")]], [{ label: "选择并人工确认", run: (row) => rosterEdit({ employee_id: values.employee_id, employee: opt.employees.find((e) => e.id === values.employee_id), work_date: values.date, rule_id: row.rule_id, shift_id: row.shift_id, category: "workday", scheduled_work: true }, opt, rulesList, async () => { popup.close(); await refresh(); }) }]));
  }, "查询候选班次"), results);
}
function rosterEdit(row, opt, rulesList, refresh) {
  const published = rulesList.filter((r) => r.status === "published"), ruleField = input({ key: "rule_id", label: "制度", required: true, options: published.map((r) => [r.id, r.name]) }, row.rule_id), shift = input({ key: "shift_id", label: "班次", options: [["", "休息"]] }, row.shift_id);
  const update = () => { const value = shift.input.value; shift.input.replaceChildren(el("option", { value: "" }, "休息"), ...(published.find((r) => r.id === ruleField.read())?.config?.shifts || []).map((s) => el("option", { value: s.id }, s.name))); shift.input.value = value; }; update(); shift.input.value = row.shift_id || ""; ruleField.input.addEventListener("change", update);
  const scheduled = check("休息日仍要求出勤", row.scheduled_work);
  const reason = input({ key: "reason", label: "调整原因", required: true }, ""), category = input({ key: "category", label: "日期类型", options: categories }, row.category), body = el("form", {}, el("p", {}, `${personText(row.employee || opt.employees.find((e) => e.id === row.employee_id))} · ${row.work_date}`), ruleField.wrapper, shift.wrapper, category.wrapper, scheduled.node, reason.wrapper), feedback = el("div"), save = el("button", { type: "submit", class: "button primary" }, "保存调整"), popup = dialog("调整排班", body); body.append(feedback, save); let busy = false;
  body.addEventListener("submit", async (event) => { event.preventDefault(); if (busy || !body.reportValidity()) return; busy = true; save.disabled = true; try { await request("/rosters", { items: [{ ...row, rule_id: ruleField.read(), shift_id: shift.read(), category: category.read(), scheduled_work: scheduled.read(), source: "manual", status: "confirmed" }], reason: reason.read() }); popup.close(); await refresh(); } catch (e) { feedback.replaceChildren(errorNode(e)); } finally { busy = false; save.disabled = false; } });
}

export async function batches() {
  return view("考勤计算批次", "按已确认排班和本地制度计算，企业微信原始状态保留对照。", async (panel, current) => {
    const opt = await options(); if (!current()) return;
    const people = pickMany("计算人员（不选则计算权限范围全部人员）", opt.employees), results = el("div");
    const load = async () => { try { const r = await request("/batches"); if (current()) results.replaceChildren(dataTable(asRows(r), [["start_date", "起始日期"], ["end_date", "结束日期"], ["status", "状态", label], ["completed_count", "已计算"], ["total_count", "总条数"], ["created_at", "创建时间"], ["error_message", "说明"]], [{ label: "查看结果", run: (r) => { location.href = BASE + "hrm/attendance-results?batch_id=" + encodeURIComponent(r.id); } }, { label: "批次详情", run: safe(async (r) => { const d = await request("/batches/" + encodeURIComponent(r.id)); dialog("计算批次", details({ "状态": label(d.status), "开始日期": d.start_date, "结束日期": d.end_date, "已计算": d.completed_count, "总条数": d.total_count, "说明": d.error_message || "—" })); }) }])); } catch (e) { if (current()) results.replaceChildren(errorNode(e)); } };
    panel.replaceChildren(can("hrm", "write") && form(dateFields, { start_date: today().slice(0, 8) + "01", end_date: today() }, async (v) => { await request("/batches", { ...v, employee_ids: people.read() }); toast("计算任务已提交"); await load(); }, "开始计算"), can("hrm", "write") && people.node, button("刷新批次", load), results); await load();
    const timer = setInterval(() => { if (current()) void load(); }, 10000); state.cleanup = () => clearInterval(timer);
  });
}

export async function results() {
  return view("考勤明细与异常", "查看本地制度计算、原始证据、逐段认定与人工调整。", async (panel, current) => {
    const [opt, batchList] = await Promise.all([options(), request("/batches")]); if (!current()) return;
    const content = el("div"), initial = { batch_id: new URLSearchParams(location.search).get("batch_id") || "", start_date: today().slice(0, 8) + "01", end_date: today(), employee_id: "", q:"", dept_id:"", include_children:"true", status: "" };
    if (initial.batch_id) { const batch = await request("/batches/" + encodeURIComponent(initial.batch_id)); initial.start_date = batch.start_date; initial.end_date = batch.end_date; }
    const fields = [{ key: "batch_id", label: "计算批次", options: batchOptions(batchList) }, ...dateFields, {key:"q",label:"姓名 / 工号",type:"search",placeholder:"输入中文姓名或工号"}, {key:"dept_id",label:"部门",options:[["","全部部门"],...opt.departments.map(d=>[d.id,departmentPath(d,opt.departments)||d.name])]}, {key:"include_children",label:"下级部门",options:[["true","包含下级部门"],["false","仅当前部门"]]}, { key: "employee_id", label: "指定员工", options: employeeOptions(opt.employees) }, { key: "status", label: "计算状态", options: [["", "全部"], ["calculated", "已计算"], ["review", "需核对"], ["configuration_pending", "待配置"], ["pending", "进行中"], ["rest", "休息"]] }];
    const load = async (f) => { const data = await request("/results?" + queryString({ ...initial, ...f })); if (!current()) return; content.replaceChildren(dataTable(asRows(data), [["employee", "员工", personText], ["work_date", "日期", (v, r) => v || r.result?.date], ["result", "状态", (v) => label(v?.state)], ["result", "核定工时", (v) => hours(v?.credited_seconds)], ["result", "实际工时", (v) => hours(v?.actual_seconds)], ["result", "迟到分钟", (v) => (Number(v?.late_seconds || 0) / 60).toFixed(1)], ["result", "早退分钟", (v) => (Number(v?.early_seconds || 0) / 60).toFixed(1)], ["result", "缺卡", (v) => v?.missing_count || 0], ["result", "加班小时", (v) => hours(v?.overtime_seconds)]], [{ label: "计算详情", run: safe((r) => resultDetail(r, opt)) }]), (f.batch_id ?? initial.batch_id) ? el("a", { class: "button", href: API + ROOT + "/export?" + queryString({ batch_id: f.batch_id ?? initial.batch_id }) }, "导出当前批次") : el("p", { class: "notice" }, "选择计算批次后可导出该批次完整结果。")); };
    const filter = form(fields, initial, load, "查询结果");
    const batchControl=filter.querySelector('[name=batch_id]').closest('label');filter.append(el("details",{},el("summary",{},"高级：查看指定计算版本"),batchControl));
    filter.querySelector('[name=batch_id]').addEventListener("change", (event) => { const selected = asRows(batchList).find((b) => b.id === event.target.value); if (selected) { filter.querySelector('[name=start_date]').value = selected.start_date; filter.querySelector('[name=end_date]').value = selected.end_date; } });
    panel.replaceChildren(filter, content); await load(initial);
  });
}
async function resultDetail(row, opt) {
  const data = await request("/results/" + encodeURIComponent(row.id)), r = data.result || row.result || {};
  const hourColumns = [["expected_seconds", "应出勤小时", hours], ["actual_seconds", "实际工作小时", hours], ["credited_seconds", "核定出勤小时", hours], ["overtime_seconds", "加班小时", hours]];
  const overview = dataTable([r], hourColumns, [], { search: false, showPager: false });
  overview.setAttribute("data-attendance-hours", "total");
  const segments = dataTable(asRows(r.segments), [["kind", "时段", label], ...hourColumns, ["missing_count", "缺卡"], ["state", "状态", label], ["issues", "核对事项", (v) => asRows(v).join("；")]], [], { search: false, showPager: false, pageSize: Math.max(1, asRows(r.segments).length) });
  segments.setAttribute("data-attendance-hours", "segments");
  const restdayRows = asRows(r.overtime_buckets).filter(bucket => bucket.category === "restday" && (bucket.workday_actual_seconds || bucket.workday_credited_seconds || bucket.additional_seconds || bucket.complete_workday_applied));
  const restdayBreakdown = restdayRows.length ? el("section", { "data-attendance-restday-overtime": "" }, el("h3", {}, "休息日加班拆分"),
    el("p", { class: "notice" }, "白班核定加班与其他时段加班合计。完整白班可按制度核定480分钟，部分白班保留有效实际时间；差额用于说明核定，不改写原始打卡。"),
    dataTable(restdayRows, [["date", "日期"], ["workday_actual_seconds", "白班实际小时", hours], ["workday_credited_seconds", "白班核定加班小时", hours], ["difference", "白班核定差额小时", (_, bucket) => hours(Number(bucket.workday_credited_seconds || 0) - Number(bucket.workday_actual_seconds || 0))], ["additional_seconds", "其他时段加班小时", hours], ["seconds", "加班合计小时", hours], ["complete_workday_applied", "完整白班认定", value => value ? "已应用" : "未应用"]], [], { search: false, showPager: false })) : null;
  const content = el("section", {},
    details({ "员工": personText(data.employee || row.employee), "日期": r.date || row.work_date, "计算状态": label(r.state) }),
    el("h3", {}, "工时总览"), overview,
    el("p", { class: "notice" }, "实际工作、核定出勤和加班按各自规则计算，不能相加作为总工时。加班段的核定出勤为零时，请查看其实际工作和加班小时。"),
    details({ "请假小时": hours(r.leave_seconds), "外出小时": hours(r.out_seconds), "出差小时": hours(r.trip_seconds), "确认缺勤小时": hours(r.confirmed_absent_seconds), "全勤资格": r.full_attendance_eligible ? "符合" : "不符合 / 待核对", "全勤说明": asRows(r.full_attendance_reasons).join("；") || "—" }),
    ...(Object.hasOwn(r,"presence_seconds") ? [el("section",{"data-attendance-deductions":""},el("h3",{},"有效工时扣减"),el("p",{class:"notice"},"覆盖时长先扣除制度明确的固定休息，再得到有效工作时长。完整班次扣休一次；不足整班按制度指定范围或待复核处理。"),dataTable([r],[["presence_seconds","扣休前覆盖小时",hours],["deducted_seconds","固定扣休小时",hours],["actual_seconds","有效工作小时",hours]],[],{search:false,showPager:false}),dataTable(asRows(r.segments),[["name","时段",(v,row)=>v||label(row.kind)],["presence_seconds","扣休前覆盖小时",hours],["deducted_seconds","固定扣休小时",hours],["actual_seconds","有效工作小时",hours]],[],{search:false,showPager:false}))] : []),
    el("h3", {}, "分段计算"), segments,
    restdayBreakdown,
    el("h3", {}, "计算依据"), dataTable(asRows(r.trace), [["message", "说明"], ["seconds", "涉及小时", hours], ["evidence_ids", "证据编号", (v) => asRows(v).join("、")]]), issueTable(r.issues));
  if (can("hrm", "write")) content.append(button("人工认定 / 补签", () => adjustmentEditor(row, opt), "primary"));
  const popup = dialog("每日考勤计算详情", content);
  popup.modal.style.width = "min(1000px, calc(100vw - 32px))";
}
function adjustmentEditor(row, opt) {
  const definitions = [
    { key: "kind", label: "调整方式", options: [["punch", "补签上下班"], ["override", "人工核定工时"], ["credit", "补充有效出勤"], ["absence", "确认缺勤"], ["waive_late", "豁免迟到"], ["waive_early", "豁免早退"], ["waive_missing", "确认缺卡已核对"], ["exclude_punch", "排除错误打卡"]] },
    { key: "at", label: "补签时间（上海）", type: "datetime-local" }, { key: "punch_type", label: "补签类型", options: [["in", "上班"], ["out", "下班"]] },
    { key: "credited_minutes", label: "核定出勤分钟（留空不覆盖）", type: "number", min: 0, max: 2880 }, { key: "actual_minutes", label: "核定实际分钟（留空不覆盖）", type: "number", min: 0, max: 2880 },
    { key: "confirm_issues", label: "已核对问题", options: [["false", "保留待核对"], ["true", "已有依据并确认"]] },
    { key: "segment_id", label: "对应时段", options: [["", "全部适用时段"], ...asRows(row.result?.segments).map((s, index) => [s.id || s.segment_id, s.name || `第${index + 1}段 · ${label(s.kind)}`])] },
    { key: "punch_id", label: "排除的原始打卡编号" }, { key: "start_at", label: "认定开始（上海）", type: "datetime-local" }, { key: "end_at", label: "认定结束（上海）", type: "datetime-local" },
    { key: "reason", label: "认定原因", type: "textarea", required: true }, { key: "evidence", label: "核对依据、人员与结论", type: "textarea", required: true },
  ];
  const controls = {}, body = el("form"), grid = el("div", { class: "form-grid" }), feedback = el("div");
  for (const def of definitions) { controls[def.key] = input(def); grid.append(controls[def.key].wrapper); }
  const groups = { punch: ["at", "punch_type"], override: ["credited_minutes", "actual_minutes", "confirm_issues"], credit: ["start_at", "end_at", "segment_id"], absence: ["start_at", "end_at", "segment_id"], waive_late: ["segment_id"], waive_early: ["segment_id"], waive_missing: ["segment_id"], exclude_punch: ["punch_id"] };
  const update = () => { for (const def of definitions) if (!["kind", "reason", "evidence"].includes(def.key)) { const visible = groups[controls.kind.read()].includes(def.key); controls[def.key].wrapper.hidden = !visible; controls[def.key].input.disabled = !visible; } };
  controls.kind.input.addEventListener("change", update); update();
  const save = el("button", { type: "submit", class: "button primary" }, "保存认定并重新计算"); body.append(el("p", {}, "人工认定保留依据，不修改原始打卡。已锁定月份须先重开。"), feedback, grid, save); const popup = dialog("人工考勤认定", body); let busy = false;
  body.addEventListener("submit", async (event) => { event.preventDefault(); if (busy || !body.reportValidity()) return; busy = true; save.disabled = true; feedback.replaceChildren(); try {
    const data = { employee_id: row.employee_id || row.result?.employee_id, work_date: row.work_date || row.result?.date, kind: controls.kind.read(), reason: controls.reason.read(), evidence: controls.evidence.read() };
    for (const key of groups[data.kind]) { const raw = controls[key].input.value; if (!raw) continue; data[key] = ["at", "start_at", "end_at"].includes(key) ? Math.floor(Date.parse(raw + ":00+08:00") / 1000) : ["credited_minutes", "actual_minutes"].includes(key) ? Number(raw) : key === "confirm_issues" ? raw === "true" : raw; }
    await request("/adjustments", data); popup.close(); toast("认定已保存，重新计算任务已提交"); await results();
  } catch (error) { feedback.replaceChildren(errorNode(error)); } finally { busy = false; save.disabled = false; } });
}

let closingMonth = false;
function monthRange(month) {
  if (!/^\d{4}-\d{2}$/.test(month) || !validDate(month + "-01")) throw Error("请选择有效月份");
  const first = new Date(month + "-01T00:00:00Z");
  return { start_date: month + "-01", end_date: new Date(Date.UTC(first.getUTCFullYear(), first.getUTCMonth() + 1, 0)).toISOString().slice(0, 10) };
}
function closeMonthDialog(month, refresh) {
  if (closingMonth) { toast("月结正在处理中，请等待当前任务完成"); return; }
  let popup, busy = false, batchID = "", batchMonth = "";
  const progress = el("section", { "data-attendance-month-progress": "", "aria-live": "polite" });
  const editor = form([{key:"month",label:"月份",type:"month",required:true},{key:"reason",label:"核对说明",type:"textarea",required:true}], {month,reason:"当月考勤及异常已核对"}, async values => {
    if (busy || closingMonth) return;
    const range = monthRange(values.month), reason = values.reason.trim();
    if (values.month >= today().slice(0, 7)) throw Error("当前月份尚未结束，不能月结；请选择已结束的月份。");
    if (!reason) throw Error("请填写核对说明");
    busy = closingMonth = true;
    const controls = [...editor.querySelectorAll("input,textarea,button[type=submit]")];
    controls.forEach(control => control.disabled = true);
    try {
      if (!batchID || batchMonth !== values.month) {
        progress.replaceChildren(el("p", {}, "正在准备整月计算，自动生成排班并核算全部人员…"));
        const created = await request("/batches", {...range, employee_ids: []});
        if (!created.id) throw Error("计算任务未返回编号，请刷新后重试");
        batchID = created.id; batchMonth = values.month;
      }
      while (popup.modal.isConnected) {
        const batch = await request("/batches/" + encodeURIComponent(batchID));
        if (!popup.modal.isConnected) return;
        progress.replaceChildren(details({"月份":values.month,"计算状态":label(batch.status),"已计算":`${Number(batch.completed_count || 0)} / ${Number(batch.total_count || 0)}`,"说明":batch.error_message || "完成计算后自动检查完整性、复核状态并月结"}),el("a",{class:"button",href:BASE+"hrm/attendance-results?batch_id="+encodeURIComponent(batchID)},"查看当月明细与异常"));
        if (batch.status === "completed") {
          if (!Number(batch.total_count) || Number(batch.completed_count) !== Number(batch.total_count)) throw Error("整月计算不完整，不能月结。请查看当月明细与异常。");
          progress.prepend(el("p", {}, "整月计算完成，正在检查并月结…"));
          await request(`/months/${encodeURIComponent(values.month)}/close`, {batch_id:batchID,reason});
          popup.close(); toast("月结完成，正式结果已锁定"); await refresh?.(values.month); return;
        }
        if (!["queued", "running"].includes(batch.status)) {
          batchID = "";
          throw Error(batch.error_message || "整月计算未完成，请查看任务说明后重新月结");
        }
        await new Promise(resolve => setTimeout(resolve, 1500));
      }
    } catch (error) {
      // A rejected close must be retried against fresh inputs after HR resolves
      // exceptions. Never bypass the server's completeness or review checks.
      if (error.path?.includes("/months/") || error.message.includes("计算不完整")) batchID = "";
      throw error;
    } finally {
      busy = closingMonth = false;
      controls.forEach(control => control.disabled = false);
    }
  }, "确认计算并月结");
  popup = dialog("一键月结", el("section", {"data-attendance-month-close":""}, el("p", {class:"notice"}, "系统将自动核算整月全部人员，检查异常后完成复核、发布和锁定。未通过检查时显示原因。关闭此窗口会停止后续月结，已提交的计算仍在后台继续。"), editor, progress));
  return popup;
}
export async function months() {
  return view("月度考勤", "按月份查看本地每日结果汇总；处理异常后，一键自动核算整月并月结，保留正式版本。", async (panel, current) => {
    const selected = input({key:"month_summary",label:"考勤月份",type:"month",required:true}, new URLSearchParams(location.search).get("month") || today().slice(0,7));
    const summary = el("section", {"data-attendance-month-summary":""}), versions = el("section", {"data-attendance-month-versions":""});
    const decimal = value => Number(value || 0).toFixed(2);
    let sequence = 0;
    const load = async () => {
      const ticket = ++sequence, month = selected.read();
      summary.replaceChildren(el("p", {}, "正在汇总本地考勤…"));
      try {
        const range = monthRange(month);
        const [data, publications] = await Promise.all([api("/hrm/attendance/month?" + queryString({month})), request("/months")]);
        if (!current() || ticket !== sequence) return;
        const official = data.basis === "published", rows = asRows(data).map(row => ({...row, pending_days:asRows(row.days).filter(day=>["pending","review","configuration_pending"].includes(day.state)).length, uncomputed_days:asRows(row.days).filter(day=>day.state==="uncomputed").length}));
        summary.replaceChildren(el("h3", {}, `${month} · ${official ? "正式结果" : "本地试算汇总"}`), el("p", {class:"notice"}, official ? `正式结果版本：${Number(data.result_version || 0)}；${Number(data.change_pending) ? "存在待处理变更，请重开核对后重新月结。" : "按已发布版本展示。"}` : data.warning || "按每人每天最新本地计算汇总，尚未完成月结；未计算日期不可视为正常出勤。"), el("p",{class:"hint"},"实际工作、核定出勤与加班分别展示，不能相加作为实际总工时。存在待核对或未计算日期时，以上数字仅为试算，不能作为正式核定。"), dataTable(rows, [["name","员工"],["employee_no","工号"],["department","部门"],["expected_days","应出勤天"],["worked_days","出勤天"],["expected_hours","应出勤小时",decimal],["actual_hours","实际工作小时",decimal],["credited_hours","核定出勤小时",decimal],["overtime_hours","加班合计小时",decimal],["overtime_workday_hours","工作日加班小时",decimal],["overtime_restday_hours","休息日加班小时",decimal],["overtime_holiday_hours","法定假日加班小时",decimal],["late_count","迟到次数"],["early_count","早退次数"],["missing_count","缺卡次数"],["pending_days","待核对天"],["uncomputed_days","未计算天"],["leave_hours","请假小时",decimal],["out_hours","外出小时",decimal],["trip_hours","出差小时",decimal]], [{label:"每日明细",run:row=>{location.href=BASE+"hrm/attendance-results?"+queryString({...range,employee_id:row.employee_id,...(official&&data.batch_id?{batch_id:data.batch_id}:{})});}}]));
        versions.replaceChildren(el("h3", {}, "正式版本与月结记录"), dataTable(asRows(publications), [["month","月份"],["status","状态",label],["revision","版本"],["change_pending","待重算变更",v=>Number(v)?"有":"无"],["updated_at","更新时间"]], [{label:"查看明细",run:row=>{location.href=BASE+"hrm/attendance-results?"+queryString({batch_id:row.batch_id});}},{label:"导出",when:row=>!!row.batch_id,run:row=>{location.href=API+ROOT+"/export?"+queryString({batch_id:row.batch_id});}},...(can("hrm","write")?[{label:"月结",when:row=>row.status!=="locked",run:row=>close(row.month)},{label:"重开",when:row=>row.status==="locked",run:reopen}]:[])]));
      } catch(error) { if(current() && ticket===sequence) summary.replaceChildren(errorNode(error)); }
    };
    const refreshMonth = async month => {selected.input.value=month;await load();};
    const close = month => closeMonthDialog(month || selected.read(), refreshMonth);
    const reopen = row => executeDialog("重开月份",[{key:"reason",label:"重开原因",type:"textarea",required:true}],{},values=>request(`/months/${encodeURIComponent(row.month)}/action`,{action:"reopen",reason:values.reason}),()=>refreshMonth(row.month));
    selected.input.addEventListener("change", load);
    panel.replaceChildren(el("div", {class:"toolbar"}, selected.wrapper, button("刷新汇总", load), can("hrm","write") && button("一键月结",()=>close(),"primary")), summary, versions);
    await load();
  });
}

export async function ledger() {
  return view("调休台账", "按分钟记录期初额度、已核定加班转换、使用和冲销，保留每笔依据。", async (panel, current) => {
    const opt = await options(); if (!current()) return;
    const results = el("div"), selected = input({ key: "employee_id", label: "员工", required: true, options: employeeOptions(opt.employees, "请选择员工") }, "");
    let entries = [];
    const load = async () => { if (!selected.read()) { results.replaceChildren(el("p", {}, "请选择员工查看余额与流水。")); return; } const data = await request("/ledger?" + queryString({ employee_id: selected.read() })); entries = asRows(data); if (current()) results.replaceChildren(el("p", { class: "notice" }, `可用调休：${Number(data.balance_minutes || 0)} 分钟`), dataTable(entries, [["entry_type", "类型", label], ["minutes", "分钟"], ["effective_date", "生效日期"], ["reason", "依据 / 原因"], ["source_ref", "来源凭据"], ["created_at", "登记时间"]])); };
    const create = async () => {
      if (!selected.read()) { toast("请先选择员工"); return; }
      const employee = selected.read(), [calculated, journal] = await Promise.all([request("/results?" + queryString({ employee_id: employee })), request("/ledger?" + queryString({ employee_id: employee }))]);
      const fields = [
        { key: "entry_type", label: "流水类型", options: [["opening", "期初额度"], ["convert", "核定加班转调休"], ["use", "使用调休"], ["reverse", "冲销原流水"]] },
        { key: "minutes", label: "分钟数（冲销可留空，按原流水全额反向）", type: "number", min: 1 },
        { key: "effective_date", label: "生效日期", type: "date", required: true },
        { key: "result_id", label: "核定日结果（转换时选择，须已发布或锁定）", options: [["", "请选择"], ...asRows(calculated).map((r) => [r.id, `${r.work_date} · 加班 ${hours(r.result?.overtime_seconds)} 小时`])] },
        { key: "reverse_entry_id", label: "被冲销流水（冲销必选）", options: [["", "请选择"], ...asRows(journal).filter((e) => e.entry_type !== "reverse").map((e) => [e.id, `${e.effective_date} · ${label(e.entry_type)} ${e.minutes} 分钟 · ${e.reason}`])] },
        { key: "source_ref", label: "来源凭据编号", required: true }, { key: "evidence", label: "期初依据（期初额度必填）", type: "textarea" }, { key: "reason", label: "原因", required: true },
      ];
      executeDialog("登记调休流水", fields, { effective_date: today() }, (v) => request("/ledger", { ...v, employee_id: employee }), load);
    };
    panel.replaceChildren(selected.wrapper, button("查询台账", safe(load)), can("hrm", "write") && button("新增流水", safe(create), "primary"), results); await load();
  });
}

export async function imports() {
  return view("工作簿导入与核对", "保留核定月表、批注、公式缓存和来源坐标；精确关联员工，规则草稿确认后发布。", async (panel, current) => {
    const [list, opt, batchList] = await Promise.all([request("/imports"), options(), request("/batches")]); if (!current()) return;
    const upload = el("form"), file = el("input", { type: "file", accept: ".xlsx", required: true, "aria-label": "考勤工作簿" }), submit = el("button", { type: "submit", class: "button primary" }, "上传并预览"), error = el("div");
    upload.append(file, submit, error); let busy = false;
    upload.addEventListener("submit", async (event) => { event.preventDefault(); if (busy || !file.files[0]) return; busy = true; submit.disabled = true; error.replaceChildren(); try { const body = new FormData(); body.append("file", file.files[0]); const response = await fetch(API + ROOT + "/imports", { method: "POST", credentials: "include", headers: { "X-Requested-With": "XMLHttpRequest" }, body }); const json = await response.json(); if (!response.ok || (json.code != null && Number(json.code) !== 0)) throw Error(json.message || json.msg || "导入失败"); toast("工作簿已解析，尚未自动启用规则或创建员工"); await imports(); } catch (e) { error.replaceChildren(errorNode(e)); } finally { busy = false; submit.disabled = false; } });
    panel.replaceChildren(can("hrm", "write") && upload, el("p", { class: "notice" }, "对应月份原始打卡未补齐时，只能核对已核定结果，不能宣称原始计算通过。工号重复、缺失或同名须人工确认。"), dataTable(asRows(list), [["filename", "文件"], ["kind", "类型", label], ["month", "月份"], ["status", "状态", label], ["created_at", "导入时间"]], [{ label: "预览 / 关联", run: safe((r) => importDetail(r, opt)) }, { label: "对账", run: (r) => executeDialog("选择对账批次", [{ key: "batch_id", label: "计算批次", options: batchOptions(batchList), required: true }], {}, async (v) => { const data = await request(`/imports/${encodeURIComponent(r.id)}/compare?` + queryString(v)); dialog("基准对账结果", dataTable(asRows(data), [["name", "姓名"], ["employee_no", "工号"], ["status", "对账状态", label], ["expected_hours", "核定基准小时"], ["actual_hours", "计算小时"], ["difference_hours", "差异小时"], ["issues", "差异说明", (v) => asRows(v).join("；")], ["source_key", "来源位置"]], [{ label: "分项差异", run: (row) => dialog("核定出勤与加班分项对账", el("div", {}, el("p", {class: "notice"}, "空白或非数值的原表单元格标记为无法验证，不视为零。"), dataTable(asRows(row.metrics), [["name", "指标"], ["expected_hours", "原表小时"], ["actual_hours", "计算小时"], ["difference_hours", "差异小时"], ["status", "验证状态", label], ["source_cell", "来源", (v) => `${v?.sheet || ""}!${v?.cell || ""}`]]))) }])); }) }]));
  });
}
async function importDetail(record, opt) {
  const data = await request("/imports/" + encodeURIComponent(record.id)), preview = data.preview || data, mappings = data.mappings || {}, content = el("section", {}, details({ "文件": preview.filename || record.filename, "类型": label(preview.kind || record.kind), "月份": preview.month || "规则来源", "人员记录": asRows(preview.employees).length, "规则草稿": asRows(preview.rules).length }), issueTable(preview.issues));
  if (asRows(preview.employees).length) {
    content.append(dataTable(preview.employees, [["name", "姓名"], ["employee_no", "源工号"], ["department", "源部门"], ["source_key", "来源位置"], ["source_key", "本地关联", (k) => opt.employees.find((e) => e.id === mappings[k]) ? personText(opt.employees.find((e) => e.id === mappings[k])) : "未关联"]], [{ label: "查看日格", run: (e) => dialog(`${e.name} · 基准日格`, dataTable(asRows(e.days).flatMap((d) => asRows(d.cells).map((c) => ({ ...c, date: d.date }))), [["date", "日期"], ["cell", "单元格"], ["value", "核定内容"], ["comment", "批注"], ["formula", "原公式"], ["merged_from", "合并来源"]])) }, ...(can("hrm", "write") ? [{ label: "关联员工", run: (e) => executeDialog(`关联 ${e.name} · ${e.employee_no || "缺工号"}`, [{ key: "employee_id", label: "精确选择本地员工", options: employeeOptions(opt.employees, "请选择"), required: true }], { employee_id: mappings[e.source_key] || "" }, async (v) => { const next = { ...mappings, [e.source_key]: v.employee_id }; await request(`/imports/${encodeURIComponent(record.id)}/mappings`, { mappings: next }); Object.assign(mappings, next); toast("员工关联已保存"); }, async () => { previewModal.close(); await importDetail(record, opt); }) }] : [])]));
  }
  if (asRows(preview.rules).length) content.append(dataTable(preview.rules, [["department", "部门"], ["names_text", "姓名"], ["group", "组别 / 人员"], ["schedule_text", "源工作时段"], ["remarks", "备注"], ["source_key", "来源位置"]]), can("hrm", "write") && button("生成待核对制度草稿", safe(async () => { await request(`/imports/${encodeURIComponent(record.id)}/rules`, {}); toast("草稿已生成，请到考勤制度逐项核对"); }), "primary"));
  const previewModal = dialog("工作簿证据预览", content);
}

export async function audit() {
  return view("考勤操作审计", "查看制度发布、排班调整、月结和人工认定记录。", async (panel, current) => { const data = await request("/audit"); if (current()) panel.replaceChildren(button("刷新", audit), dataTable(asRows(data), [["created_at", "操作时间"], ["entity", "对象", (v) => ({ rule: "制度", roster: "排班", month: "月报", adjustment: "人工认定", ledger: "调休", import: "工作簿" })[v] || "考勤记录"], ["action", "操作", (v) => ({ save: "保存草稿", publish: "发布", assign: "调整排班", review: "复核", lock: "月结锁定", reopen: "重开", create: "创建", approve: "核准" })[v] || label(v)], ["reason", "原因"], ["created_by", "操作人编号"]])); });
}
export async function overtime() {
  return view("加班凭据", "已批准的加班凭据用于本地考勤认定，企业微信来源记录不可在此改写。", async (panel, current) => { const [data, opt] = await Promise.all([request("/overtime"), options()]); if (!current()) return;
    const editor = (row = {}) => executeDialog("人工加班凭据", [{ key: "employee_id", label: "员工", options: employeeOptions(opt.employees, "请选择员工"), required: true }, { key: "start_time", label: "开始时间", type: "datetime-local", required: true }, { key: "end_time", label: "结束时间", type: "datetime-local", required: true }, { key: "duration", label: "批准小时", type: "number", min: 0, step: 0.5, required: true }, { key: "approval_status", label: "审批状态", options: [["pending", "待审批"], ["approved", "已通过"], ["rejected", "已驳回"], ["canceled", "已撤销"]] }, { key: "reason", label: "加班事由", required: true }], row, (v) => request("/overtime", { ...row, ...v, source: "manual" }), overtime);
    panel.replaceChildren(can("hrm", "write") && button("新增人工凭据", () => editor(), "primary"), dataTable(asRows(data), [["employee_id", "员工", (v, r) => r.employee_name || personText(opt.employees.find((e) => e.id === v))], ["start_time", "开始"], ["end_time", "结束"], ["duration", "批准小时"], ["approval_status", "状态", label], ["reason", "事由"], ["source", "来源", label]], can("hrm", "write") ? [{ label: "编辑", when: (r) => r.source === "manual", run: editor }] : []));
  });
}
