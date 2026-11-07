import {
  el,
  api,
  asRows,
  can,
  page,
  surface,
  button,
  dataTable,
  resource,
  edit,
  form,
  field,
  dialog,
  inspect,
  status,
  errorNode,
  toast,
  parseJSON,
  state,
  safe,
  timestamp,
  details,
  BASE,
} from "./core.js";
import { organizationTools, departmentPath, mappingLabel, employeeScopeNotice } from "./organization.js";
const approvalOptions = [
  ["approved", "已通过"],
  ["pending", "待审批"],
  ["rejected", "已驳回"],
  ["canceled", "已撤销"],
];
const employeeOption = (rows) => [
  ["", "请选择员工"],
  ...asRows(rows).map((e) => [
    e.id,
    `${e.name} · ${e.employee_no || e.wecom_userid || e.id}`,
  ]),
];
const employeeField = (rows) => ({
  key: "employee_id",
  label: "员工",
  options: employeeOption(rows),
  required: true,
});
const dates = [
  {
    key: "start_time",
    label: "开始时间（上海）",
    type: "datetime-local",
    required: true,
  },
  {
    key: "end_time",
    label: "结束时间（上海）",
    type: "datetime-local",
    required: true,
  },
  {
    key: "duration",
    label: "时长（小时）",
    type: "number",
    min: 0,
    step: 0.01,
    required: true,
  },
  { key: "approval_status", label: "审批状态", options: approvalOptions },
  { key: "reason", label: "事由", type: "textarea", wide: true },
];
function approvedTransform(data) {
  if (data.end_time <= data.start_time)
    throw new Error("结束时间必须晚于开始时间");
  return { ...data, source: "manual" };
}
export function employees() {
  return resource({
    title: "员工与账号映射",
    itemName: "员工",
    area: "hrm",
    path: "/hrm/employees",
    description: "按企业微信组织架构关联员工与本地部门，显示名使用姓名，登录账号使用工号。",
    notice: employeeScopeNotice(),
    tools: organizationTools,
    load: () => api("/hrm/options"),
    columns: (options) => [
      ["name", "姓名"],
      ["employee_no", "工号", (value) => value || "待补充"],
      ["host_username", "登录账号", (value, row) => value || asRows(options.users).find((u) => Number(u.id) === Number(row.user_id))?.username || "未关联账号"],
      ["host_display_name", "显示名", (value, row) => value || asRows(options.users).find((u) => Number(u.id) === Number(row.user_id))?.name || "未关联账号"],
      ["host_department_path", "本地部门", (value, row) => value || row.host_department_name || departmentPath(asRows(options.departments).find((d) => Number(d.id) === Number(row.dept_id)), asRows(options.departments)) || "未关联部门"],
      ["department_names", "企业微信部门", (value) => asRows(value).join(" / ") || "未同步部门"],
      ["position", "职位"],
      ["status", "状态", status],
      ["mapping_status", "映射状态", mappingLabel],
      ["mapping_message", "映射说明", (value) => el("span", { class: "organization-value" }, value || "—")],
    ],
    detail: (row) => dialog("员工与账号详情", details({
      "姓名": row.name,
      "工号": row.employee_no || "待补充",
      "登录账号": row.host_username || "未关联账号",
      "显示名": row.host_display_name || "未关联账号",
      "本地部门": row.host_department_path || row.host_department_name || "未关联部门",
      "企业微信部门": asRows(row.department_names).join(" / ") || "未同步部门",
      "职位": row.position || "—", "手机号": row.mobile || "—", "邮箱": row.email || "—",
      "员工状态": ({ active: "在职", inactive: "停用", quit: "离职" })[row.status] || "待核对",
      "映射状态": mappingLabel(row.mapping_status), "映射说明": row.mapping_message || "—",
      "企业微信成员标识": row.wecom_userid || "未关联", "入职日期": row.entry_date || "—", "离职日期": row.quit_date || "—",
    })),
    fields: (options) => [
      { key: "name", label: "姓名", required: true },
      { key: "employee_no", label: "工号", hint: "同步账号时使用工号作为登录账号" },
      {
        key: "wecom_userid",
        label: "企业微信成员标识",
        hint: "对应企业微信成员的唯一标识",
      },
      {
        key: "user_id",
        label: "关联本地账号",
        type: "number",
        options: [
          [0, "暂不绑定"],
          ...asRows(options.users).map((u) => [
            u.id,
            `${u.name || u.nickname || "未填写姓名"} · ${u.username || "未填写账号"}${Number(u.status) === 1 ? " · 停用" : ""}`,
          ]),
        ],
      },
      {
        key: "dept_id",
        label: "本地部门",
        type: "number",
        options: [
          [0, "暂不绑定"],
          ...asRows(options.departments).map((d) => [
            d.id,
            departmentPath(d, asRows(options.departments)) || "未命名部门",
          ]),
        ],
      },
      { key: "position", label: "职位" },
      { key: "mobile", label: "手机号" },
      { key: "email", label: "邮箱", type: "email" },
      {
        key: "status",
        label: "员工状态",
        options: [
          ["active", "在职"],
          ["inactive", "停用"],
          ["quit", "离职"],
        ],
      },
      { key: "entry_date", label: "入职日期", type: "date" },
      { key: "quit_date", label: "离职日期", type: "date" },
    ],
  });
}
export function holidays() {
  return resource({
    title: "节假日与调休",
    itemName: "日期",
    area: "hrm",
    path: "/hrm/attendance/holidays",
    description: "节假日优先于每周班次规则；调休日期设为工作日。",
    columns: [
      ["holiday_date", "日期"],
      ["name", "名称"],
      ["is_workday", "类型", (v) => (Number(v) ? "调休工作日" : "休息日")],
      ["remark", "备注"],
    ],
    fields: [
      { key: "holiday_date", label: "日期", type: "date", required: true },
      { key: "name", label: "名称", required: true },
      {
        key: "is_workday",
        label: "日期类型",
        type: "number",
        options: [
          [0, "休息日"],
          [1, "调休工作日"],
        ],
      },
      { key: "remark", label: "备注" },
    ],
  });
}
const clockStatusOptions = [
  ["normal", "正常"],
  ["late", "迟到"],
  ["early", "早退"],
  ["missing", "缺卡"],
  ["abnormal", "异常"],
];
const localClockStatusOptions = [["uncomputed", "未计算"], ["in_progress", "进行中"], ["configuration_pending", "待配置"], ["pending_review", "待复核"]];
const clockStatusLabels = Object.fromEntries([...clockStatusOptions, ...localClockStatusOptions, ["pending", "进行中"]]);
const clockStatusText = (value) => clockStatusLabels[value] || (value ? "待核对" : "—");
const clockStatus = (value) => el("span", {
  class: `badge ${["missing", "abnormal"].includes(value) ? "error" : ["late", "early"].includes(value) ? "warn" : ""}`,
}, clockStatusText(value));
const clockTypeText = (value) => ({ in: "上班打卡", out: "下班打卡", outside: "外出打卡" })[value] || "待核对";
const clockSourceText = (value) => ({ manual: "手工", wecom: "企业微信", ruoyi: "历史导入" })[value] || "待核对";
const attendanceDayState = (value) => ({
  rest: "休息", ambiguous_schedule: "排班冲突，需核对", calculated: "已统计", pending: "待统计",
})[value] || (value ? "待核对" : "—");
function clockQuery({ filters = {} }) {
  const query = {
    employee_id: filters.employee_id,
    employee_search: filters.employee_search,
    department: filters.department,
    status: filters.status,
    basis: filters.basis || "local",
    include_subdepartments: filters.include_subdepartments !== false,
  };
  if (filters.mode === "day") query.date = filters.date;
  if (filters.mode === "month") query.month = filters.month;
  if (filters.mode === "range") {
    query.start_time = filters.start_time;
    query.end_time = filters.end_time;
  }
  return query;
}
function clockEmployeeLabel(employee) {
  return [
    employee.name || "未命名员工",
    ...(employee.department_names || []),
    employee.employee_no ? `工号 ${employee.employee_no}` : "",
  ].filter(Boolean).join(" · ");
}
function clockFilters({ extra = {}, total, filters, search, apply }) {
  const filterForm = el("form", { class: "clock-filters", "aria-label": "打卡记录筛选" });
  const errors = el("div");
  const grid = el("div", { class: "clock-filter-grid" });
  const mode = field({ key: "clock_date_mode", label: "日期范围", options: [
    ["all", "全部日期"], ["day", "按日"], ["month", "按月"], ["range", "自定义日期区间"],
  ] }, filters.mode || "all");
  const date = field({ key: "clock_date", label: "打卡日期（上海）", type: "date" }, filters.date);
  const month = field({ key: "clock_month", label: "打卡月份（上海）", type: "month" }, filters.month);
  const start = field({ key: "clock_start_time", label: "开始日期（上海）", type: "date" }, filters.start_time);
  const end = field({ key: "clock_end_time", label: "结束日期（含当天）", type: "date" }, filters.end_time);
  const department = field({ key: "clock_department", label: "部门", options: [
    ["", "全部部门"],
    ...asRows(extra.departments).map((d) => [d.key, `${d.path || d.name} · ${d.source === "host" ? "本地部门" : "企业微信"}`]),
  ] }, filters.department || "");
  const descendants = el("input", {
    type: "checkbox", name: "clock_include_subdepartments", checked: filters.include_subdepartments !== false,
  });
  const departmentGroup = el("div", {}, department.wrapper,
    el("label", { class: "clock-filter-check" }, descendants, "包含下级部门"));
  const employeeSearch = el("input", {
    type: "search", id: "clock_employee_search", name: "clock_employee_search", placeholder: "输入姓名或工号，点击查询",
    value: filters.employee_search || "",
    "aria-label": "查找员工",
  });
  const employeeSelect = el("select", { name: "clock_employee_id", "aria-label": "选择员工" });
  const employeeChoices = asRows(extra.employees);
  const departmentChoices = new Map(asRows(extra.departments).map((item) => [item.key, item]));
  let selectedEmployee = filters.employee_id || "";
  const employeeMatchesDepartment = (employee) => {
    const wanted = department.input.value;
    if (!wanted) return true;
    return (employee.department_keys || []).some((membership) => {
      const seen = new Set();
      for (let key = membership; key && !seen.has(key);) {
        if (key === wanted) return true;
        if (!descendants.checked) return false;
        seen.add(key);
        const current = departmentChoices.get(key);
        key = current?.parent_id > 0 ? `${current.source}:${current.parent_id}` : "";
      }
      return false;
    });
  };
  function renderEmployeeChoices() {
    const needle = employeeSearch.value.trim().toLocaleLowerCase();
    const matches = employeeChoices.filter((employee) => employeeMatchesDepartment(employee) &&
      [employee.name, employee.employee_no].some((value) => String(value || "").toLocaleLowerCase().includes(needle)));
    if (!matches.some((employee) => String(employee.id) === selectedEmployee)) selectedEmployee = "";
    employeeSelect.replaceChildren(el("option", { value: "" }, needle ? `全部匹配人员（${matches.length}人）` : `全部部门内人员（${matches.length}人）`));
    for (const employee of matches)
      employeeSelect.append(el("option", { value: employee.id }, clockEmployeeLabel(employee)));
    if (!matches.length)
      employeeSelect.append(el("option", { disabled: true }, "没有找到匹配员工"));
    employeeSelect.value = selectedEmployee;
  }
  employeeSearch.addEventListener("input", () => { selectedEmployee = ""; renderEmployeeChoices(); });
  employeeSelect.addEventListener("change", () => { selectedEmployee = employeeSelect.value; });
  department.input.addEventListener("change", renderEmployeeChoices);
  descendants.addEventListener("change", renderEmployeeChoices);
  renderEmployeeChoices();
  const employee = el("div", { class: "field" }, el("label", { for: "clock_employee_search" }, "人员"), employeeSearch, employeeSelect,
    el("small", { class: "hint" }, "输入姓名或工号即可查询；也可选择具体人员。同名员工按部门、工号区分。"));
  const textSearch = field({ key: "clock_search", label: "搜索记录", type: "search", placeholder: "姓名、工号或打卡地点" }, search);
  const attendanceStatus = field({ key: "clock_status", label: "考勤状态", options: [
    ["", "全部状态"], ...clockStatusOptions, ...localClockStatusOptions,
  ] }, filters.status || "");
  const basis = field({ key: "clock_basis", label: "状态依据", options: [["local", "本地考勤认定"], ["source", "来源状态（仅对照）"]] }, filters.basis || "local");
  function updateDateMode() {
    for (const [control, modes] of [[date, ["day"]], [month, ["month"]], [start, ["range"]], [end, ["range"]]]) {
      const visible = modes.includes(mode.input.value);
      control.wrapper.hidden = !visible;
      control.input.disabled = !visible;
    }
    errors.replaceChildren();
  }
  mode.input.addEventListener("change", updateDateMode);
  updateDateMode();
  grid.append(mode.wrapper, date.wrapper, month.wrapper, start.wrapper, end.wrapper, departmentGroup, employee, basis.wrapper, attendanceStatus.wrapper, textSearch.wrapper);
  filterForm.append(errors, grid,
    el("div", { class: "toolbar" },
      el("button", { type: "submit", class: "button primary" }, "查询"),
      button("清空筛选", () => apply({ filters: {}, search: "" })),
      el("span", { class: "count" }, `共 ${total} 条`)));
  filterForm.addEventListener("submit", (event) => {
    event.preventDefault();
    errors.replaceChildren();
    const next = {
      mode: mode.input.value,
      employee_id: employeeSelect.value,
      employee_search: employeeSearch.value.trim(),
      department: department.input.value,
      status: attendanceStatus.input.value,
      basis: basis.input.value,
      include_subdepartments: descendants.checked,
    };
    try {
      const validDate = (value) => /^\d{4}-\d{2}-\d{2}$/.test(value) &&
        !Number.isNaN(Date.parse(value + "T00:00:00Z")) &&
        new Date(value + "T00:00:00Z").toISOString().slice(0, 10) === value;
      if (next.mode === "day") {
        if (!validDate(date.input.value)) throw new Error("请选择有效的打卡日期");
        next.date = date.input.value;
      } else if (next.mode === "month") {
        if (!/^\d{4}-(0[1-9]|1[0-2])$/.test(month.input.value)) throw new Error("请选择有效的打卡月份");
        next.month = month.input.value;
      } else if (next.mode === "range") {
        if (!validDate(start.input.value) || !validDate(end.input.value))
          throw new Error("请选择有效的开始日期和结束日期");
        if (end.input.value < start.input.value) throw new Error("结束日期不能早于开始日期");
        next.start_time = start.input.value;
        next.end_time = end.input.value;
      }
      apply({ filters: next, search: textSearch.input.value.trim() });
    } catch (error) { errors.append(errorNode(error)); }
  });
  return filterForm;
}
export function clocks() {
  return resource({
    title: "打卡记录",
    itemName: "手工打卡",
    area: "hrm",
    path: "/hrm/attendance/clocks",
    description: "默认按本地考勤认定筛选，来源类型与状态仅作对照。制度生效后自动计算；待复核记录请在“考勤明细与异常”中处理。日期使用上海时区。",
    load: () => api("/hrm/attendance/clocks/options"),
    query: clockQuery,
    renderFilters: clockFilters,
    columns: [
      ["employee_name", "员工姓名", (name, row) => name && name !== row.employee_id && name !== row.wecom_userid ? name : "未匹配员工"],
      ["employee_no", "员工编号"],
      ["department_names", "部门", (names) => Array.isArray(names) && names.length ? names.join("、") : "—"],
      ["clock_time", "打卡时间"],
      ["clock_type", "来源类型（对照）", clockTypeText],
      ["local_status", "本地认定", (value) => clockStatus(value || "uncomputed")],
      ["source_status", "来源状态（对照）", (value, row) => clockStatus(value || row.status)],
      ["source", "来源", clockSourceText],
      ["address", "地点"],
    ],
    detail: (row) => dialog("打卡记录详情", details({
      "员工姓名": row.employee_name && row.employee_name !== row.employee_id && row.employee_name !== row.wecom_userid ? row.employee_name : "未匹配员工",
      "工号": row.employee_no || "—",
      "部门": asRows(row.department_names).join("、") || "—",
      "打卡时间（上海）": row.clock_time || "—",
      "来源打卡类型（对照）": clockTypeText(row.clock_type),
      "本地认定": clockStatusText(row.local_status || "uncomputed"),
      "来源状态（对照）": clockStatusText(row.source_status || row.status),
      "本地依据": row.local_basis ? ({ trial: "试算结果", published: "已发布月报" }[row.local_basis] || "待核对") : "尚无本地计算结果",
      "来源": clockSourceText(row.source),
      "打卡地点": row.address || "—",
    })),
    editable: (row) => row.source === "manual",
    fields: (options) => [
      { key: "employee_id", label: "员工", required: true, options: [
        ["", "请选择员工"], ...asRows(options?.employees).map((employee) => [employee.id, clockEmployeeLabel(employee)]),
      ] },
      {
        key: "clock_time",
        label: "打卡时间（上海）",
        type: "datetime-local",
        required: true,
      },
      {
        key: "clock_type",
        label: "类型",
        options: [
          ["in", "上班打卡"],
          ["out", "下班打卡"],
          ["outside", "外出打卡"],
        ],
      },
      {
        key: "status",
        label: "状态",
        options: clockStatusOptions,
      },
      { key: "address", label: "打卡地点", wide: true },
    ],
    transform: (data) => ({ ...data, source: "manual" }),
  });
}
const approvalKinds = { leaves: "请假", outs: "外出", trips: "出差" };
const approvalEmployeeName = (row) => row.employee_name && row.employee_name !== row.employee_id && row.employee_name !== row.wecom_userid ? row.employee_name : "未匹配员工";
const approvalEmployeeColumns = [
  ["employee_name", "员工姓名", (_, row) => approvalEmployeeName(row)],
  ["employee_no", "员工编号"],
  ["department_names", "部门", (names) => Array.isArray(names) && names.length ? names.join("、") : "—"],
];
function leaveTypeName(value, row) {
  if (typeof value === "string" && value.trim() && !/^\d+$/.test(value.trim())) return value;
  return typeof row.leave_type === "string" && /[\u3400-\u9fff]/.test(row.leave_type) ? row.leave_type : "未识别类型";
}
const reasonCell = (value) => el("div", { class: "approval-reason" }, typeof value === "string" && value ? value : "未填写");
const approvalText = (value, fallback = "未填写") => typeof value === "string" && value.trim() ? value :
  typeof value === "number" && Number.isFinite(value) ? String(value) : fallback;
function approvalBadge(value, label) {
  const labels = { approved: "已通过", pending: "待审批", rejected: "已驳回", canceled: "已撤销", submitted: "已提交", copied: "已抄送" };
  return el("span", { class: `badge ${value === "rejected" ? "error" : value === "pending" ? "warn" : ""}` }, label || labels[value] || "状态待确认");
}
function approvalFields(entries) {
  const fields = el("dl", { class: "approval-fields" });
  for (const [label, value] of entries)
    fields.append(el("dt", {}, label), el("dd", { class: "approval-value" }, approvalText(value)));
  return fields;
}
function approvalSheet(kind, record) {
  const label = approvalKinds[kind];
  const original = record.approval_form || {};
  const applicant = original.applicant || {};
  const employee = approvalEmployeeName({ ...record, employee_name: applicant.name || record.employee_name });
  const departments = Array.isArray(applicant.department_names) && applicant.department_names.length ? applicant.department_names : record.department_names;
  const departmentNames = Array.isArray(departments) && departments.length ? departments.join("、") : "暂无部门信息";
  const source = record.source === "wecom" ? "企业微信审批" : record.source === "manual" ? "手工登记" : "来源待确认";
  const sheet = el("article", { class: "approval-sheet", "data-approval-kind": kind });
  sheet.append(
    el("header", { class: "approval-sheet-heading" },
      el("div", {}, el("p", { class: "hint" }, source), el("h3", {}, approvalText(original.title, label + "申请单"))),
      approvalBadge(record.approval_status)),
    el("section", { class: "approval-applicant", "aria-label": "申请人信息" },
      el("span", { class: "approval-avatar", "aria-hidden": "true" }, employee === "未匹配员工" ? "人" : Array.from(employee)[0]),
      el("div", {}, el("strong", {}, employee), el("p", {}, departmentNames),
        record.employee_no || applicant.employee_no ? el("small", {}, "工号 " + approvalText(applicant.employee_no || record.employee_no)) : null)),
  );
  const basic = [
    ["申请人", employee],
    ["所属部门", departmentNames],
    [label + "类型", kind === "leaves" ? leaveTypeName(record.leave_type_name, record) : label],
    ["开始时间（上海）", record.start_time],
    ["结束时间（上海）", record.end_time],
    [label + "时长", approvalText(record.duration, "") ? `${approvalText(record.duration)} 小时` : "未填写"],
  ];
  if (kind !== "leaves") basic.push(["目的地", record.destination]);
  basic.push([label + "事由", record.reason]);
  sheet.append(el("section", { class: "approval-section" }, el("h4", {}, "申请内容"), approvalFields(basic)));
  const fields = Array.isArray(original.fields) ? original.fields : [];
  const timeline = Array.isArray(original.timeline) ? original.timeline : [];
  const notice = approvalText(original.notice, !fields.length && !timeline.length ? record.source === "manual" ?
    "该记录由手工登记，未关联企业微信原始审批表单。" : "暂未找到企业微信原始审批表单，当前显示已保存的考勤记录。" : "");
  if (notice) sheet.append(el("p", { class: "notice approval-notice" }, notice));
  if (fields.length)
    sheet.append(el("section", { class: "approval-section" }, el("h4", {}, "企业微信表单"),
      approvalFields(fields.map((item) => [approvalText(item.label, "表单字段"), approvalText(item.value, "暂无法显示此字段")]))));
  if (timeline.length) {
    const steps = el("ol", { class: "approval-timeline" });
    for (const item of timeline)
      steps.append(el("li", {},
        el("div", { class: "approval-step-heading" },
          el("strong", {}, approvalText(item.name, "未匹配审批人")),
          approvalBadge(item.status, approvalText(item.status_name, ""))),
        item.time ? el("p", { class: "hint" }, approvalText(item.time)) : null,
        item.comment ? el("p", { class: "approval-value" }, approvalText(item.comment)) : null));
    sheet.append(el("section", { class: "approval-section" }, el("h4", {}, "审批流程"), steps));
  }
  const reference = [["记录来源", source], ["审批编号", approvalText(record.external_ref, record.source === "manual" ? "未关联企业微信审批" : "暂无审批编号")]];
  if (original.apply_time) reference.push(["申请时间（上海）", original.apply_time]);
  sheet.append(el("section", { class: "approval-section approval-reference" }, el("h4", {}, "申请信息"), approvalFields(reference)));
  return sheet;
}
async function approvalDetail(kind, row) {
  const content = el("div");
  const view = dialog(approvalKinds[kind] + "详情", content);
  view.modal.classList.add("approval-dialog");
  const epoch = state.epoch;
  async function load() {
    content.replaceChildren(el("div", { class: "loading", role: "status" }, "正在读取审批详情…"));
    try {
      const record = await api(`/hrm/attendance/${kind}/${encodeURIComponent(row.id)}`);
      if (!view.modal.isConnected || epoch !== state.epoch) return;
      content.replaceChildren(approvalSheet(kind, record));
    } catch (error) {
      if (!view.modal.isConnected || epoch !== state.epoch) return;
      content.replaceChildren(errorNode(error), button("重新读取详情", load));
    }
  }
  await load();
}
export function leaves() {
  return resource({
    title: "请假记录",
    itemName: "请假",
    area: "hrm",
    path: "/hrm/attendance/leaves",
    description: "只有已通过的记录参与考勤计算；企业微信审批记录由同步维护。",
    load: async () => {
      const options = await api("/hrm/options");
      return options.employees || [];
    },
    columns: [
      ...approvalEmployeeColumns,
      ["leave_type_name", "请假类型", leaveTypeName],
      ["start_time", "开始时间"],
      ["end_time", "结束时间"],
      ["duration", "小时"],
      ["reason", "请假事由", reasonCell],
      ["approval_status", "审批状态", status],
      ["source", "来源", status],
    ],
    detail: (row) => approvalDetail("leaves", row),
    editable: (r) => r.source === "manual",
    fields: (rows) => [
      employeeField(rows),
      {
        key: "leave_type",
        label: "请假类型",
        options: ["事假", "病假", "年假", "调休", "婚假", "产假", "其他"],
      },
      ...dates,
    ],
    transform: approvedTransform,
  });
}
export function trips() {
  return outs("trips");
}
export function outs(kind = "outs") {
  const trip = kind === "trips";
  return resource({
    title: trip ? "出差记录" : "外出记录",
    itemName: trip ? "出差" : "外出",
    area: "hrm",
    path: `/hrm/attendance/${kind}`,
    description:
      "请假、外出、出差独立归入 HRM 考勤，企业微信审批撤销后自动取消有效时段。",
    load: async () => {
      const options = await api("/hrm/options");
      return options.employees || [];
    },
    columns: [
      ...approvalEmployeeColumns,
      ["destination", "目的地"],
      ["start_time", "开始时间"],
      ["end_time", "结束时间"],
      ["duration", "小时"],
      ["reason", trip ? "出差事由" : "外出事由", reasonCell],
      ["approval_status", "审批状态", status],
      ["source", "来源", status],
    ],
    detail: (row) => approvalDetail(kind, row),
    editable: (r) => r.source === "manual",
    fields: (rows) => [
      employeeField(rows),
      { key: "destination", label: "目的地", required: true },
      ...dates,
    ],
    transform: approvedTransform,
  });
}
export async function groups() {
  const epoch = state.epoch;
  try {
    const options = await api("/hrm/options");
    if (epoch !== state.epoch) return;
    return await resource({
      title: "考勤组与班次",
      itemName: "考勤组",
      description:
        "按员工或部门分配班次，支持跨日班次、每周工作日与迟到早退宽限。",
      area: "hrm",
      path: "/hrm/attendance/groups",
      columns: [
        ["name", "名称"],
        ["dept_ids", "部门范围"],
        ["employee_ids", "员工范围"],
        [
          "shifts",
          "班次",
          (v) =>
            parseJSON(v, [])
              .map((s) => `${s.name} ${s.start}–${s.end}`)
              .join(" / "),
        ],
        ["status", "状态", (v) => (Number(v) ? "启用" : "停用")],
      ],
      create: () => groupEditor({}, options.employees || [], options),
      actions: can("hrm", "write")
        ? [
            {
              label: "编辑",
              run: (row) => groupEditor(row, options.employees || [], options),
            },
          ]
        : [],
    });
  } catch (error) {
    if (epoch !== state.epoch) return;
    const body = page("考勤组与班次", "读取考勤组配置失败");
    body.append(surface(errorNode(error)));
  }
}
function groupEditor(row, employees, options) {
  const shifts = parseJSON(row.shifts, []);
  const rules = parseJSON(row.rule_json, {});
  let view;
  const groupForm = el("form"),
    errors = el("div"),
    basics = el("div", { class: "form-grid" }),
    shiftBox = el("div");
  const inputMap = {};
  for (const def of [
    { key: "name", label: "考勤组名称", required: true },
    {
      key: "status",
      label: "状态",
      type: "number",
      options: [
        [1, "启用"],
        [0, "停用"],
      ],
    },
    {
      key: "late_grace_minutes",
      label: "迟到宽限（分钟）",
      type: "number",
      min: 0,
      value: rules.late_grace_minutes || 0,
    },
    {
      key: "early_grace_minutes",
      label: "早退宽限（分钟）",
      type: "number",
      min: 0,
      value: rules.early_grace_minutes || 0,
    },
  ]) {
    const f = field(def, row[def.key]);
    inputMap[def.key] = f.input;
    basics.append(f.wrapper);
  }
  const selectors = {};
  for (const [key, label, records, id] of [
    ["dept_ids", "适用部门", options.departments, "id"],
    ["employee_ids", "适用员工", employees, "id"],
  ]) {
    const selected = parseJSON(row[key], []).map(String),
      select = el("select", { multiple: true, name: key, size: 4 });
    for (const item of asRows(records)) {
      const opt = el(
        "option",
        { value: item[id] },
        `${item.name} (${item[id]})`
      );
      opt.selected = selected.includes(String(item[id]));
      select.append(opt);
    }
    selectors[key] = select;
    basics.append(
      el(
        "label",
        { class: "field" },
        el("span", {}, label),
        select,
        el(
          "small",
          { class: "hint" },
          "按住 Ctrl / Command 多选。部门与员工同时生效。"
        )
      )
    );
  }
  function addShift(
    shift = {
      name: "标准班",
      start: "09:00",
      end: "18:00",
      break_start: "12:00",
      break_end: "13:00",
      weekdays: [1, 2, 3, 4, 5],
    }
  ) {
    const box = el("div", { class: "surface shift-row" }),
      grid = el("div", { class: "form-grid" });
    for (const def of [
      { key: "name", label: "班次名称", required: true },
      { key: "start", label: "上班时间", type: "time", required: true },
      { key: "end", label: "下班时间", type: "time", required: true },
      { key: "break_start", label: "休息开始", type: "time" },
      { key: "break_end", label: "休息结束", type: "time" },
    ])
      grid.append(field(def, shift[def.key]).wrapper);
    const weekdays = el("div", { class: "actions" });
    for (let day = 1; day <= 7; day++) {
      const actual = day === 7 ? 0 : day;
      weekdays.append(
        el(
          "label",
          {},
          el("input", {
            type: "checkbox",
            name: "weekday",
            value: actual,
            checked: shift.weekdays?.includes(actual),
          }),
          ` 周${["日", "一", "二", "三", "四", "五", "六"][actual]}`
        )
      );
    }
    box.append(
      el(
        "div",
        { class: "subheading" },
        el("h3", {}, "班次时段"),
        button("移除此班次", () => box.remove(), "small danger")
      ),
      grid,
      el("p", { class: "hint" }, "工作日（跨夜时下班时间可早于上班时间）"),
      weekdays
    );
    shiftBox.append(box);
  }
  shifts.forEach(addShift);
  if (!shifts.length) addShift();
  const save = el(
    "button",
    { type: "submit", class: "button primary" },
    "保存考勤组"
  );
  groupForm.append(
    errors,
    basics,
    el("hr"),
    shiftBox,
    button("+ 添加班次", () => addShift()),
    el("footer", { class: "form-actions" }, save)
  );
  groupForm.addEventListener("submit", async (event) => {
    event.preventDefault();
    errors.replaceChildren();
    if (!groupForm.reportValidity()) return;
    save.disabled = true;
    try {
      const values = [...shiftBox.children].map((box) => {
        const get = (name) => box.querySelector(`[name="${name}"]`).value;
        const days = [...box.querySelectorAll('[name="weekday"]:checked')].map(
          (n) => Number(n.value)
        );
        if (!days.length) throw new Error("每个班次至少选择一个工作日");
        return {
          name: get("name"),
          start: get("start"),
          end: get("end"),
          break_start: get("break_start"),
          break_end: get("break_end"),
          weekdays: days,
        };
      });
      if (!values.length) throw new Error("至少需要一个班次");
      await api("/hrm/attendance/groups", {
        id: row.id,
        name: inputMap.name.value,
        status: Number(inputMap.status.value),
        dept_ids: JSON.stringify(
          [...selectors.dept_ids.selectedOptions].map((o) => Number(o.value))
        ),
        employee_ids: JSON.stringify(
          [...selectors.employee_ids.selectedOptions].map((o) => o.value)
        ),
        shifts: JSON.stringify(values),
        rule_json: JSON.stringify({
          late_grace_minutes: Number(inputMap.late_grace_minutes.value),
          early_grace_minutes: Number(inputMap.early_grace_minutes.value),
        }),
      });
      view.close();
      toast("考勤组已保存");
      await groups();
    } catch (error) {
      errors.append(errorNode(error));
    } finally {
      save.disabled = false;
    }
  });
  view = dialog(row.id ? "编辑考勤组" : "新增考勤组", groupForm);
}
export async function month() {
  const body = page(
    "月度考勤",
    "此页为旧版汇总参考，不作为正式月报。本地制度试算、复核发布和锁定请使用下方正式考勤入口。"
  );
  const input = el("input", {
    type: "month",
    value: new Date(Date.now() + 8 * 3600000).toISOString().slice(0, 7),
    "aria-label": "考勤月份",
  });
  const content = el("div");
  body.append(
    surface(el("div", { class: "toolbar" }, el("a", { class: "button primary", href: BASE + "hrm/attendance-batches" }, "本地考勤试算"), el("a", { class: "button", href: BASE + "hrm/attendance-results" }, "每日计算结果"), el("a", { class: "button", href: BASE + "hrm/attendance-months" }, "正式月报与月结"))),
    surface(
      el(
        "div",
        { class: "toolbar" },
        el("label", {}, "选择月份"),
        input,
        button("查询", safe(load), "primary")
      ),
      content
    )
  );
  async function load() {
    content.replaceChildren(el("div", { class: "loading" }, "正在计算…"));
    try {
      const result = await api(
        "/hrm/attendance/month?month=" + encodeURIComponent(input.value)
      );
      content.replaceChildren(
        dataTable(
          asRows(result),
          [
            ["name", "员工"],
            ["expected_days", "应出勤天"],
            ["worked_days", "出勤天"],
            ["late_count", "迟到次数"],
            ["early_count", "早退次数"],
            ["missing_count", "缺卡次数"],
            ["absent_days", "缺勤天"],
            ["leave_hours", "请假小时"],
            ["out_hours", "外出小时"],
            ["trip_hours", "出差小时"],
            ["warning", "提示"],
          ],
          [
            {
              label: "每日明细",
              run: (row) =>
                dialog(
                  `${row.name || row.employee_id} · ${result.month}`,
                  dataTable(
                    row.days || [],
                    [
                      ["date", "日期"],
                      ["expected_hours", "应出勤小时"],
                      ["worked", "已出勤", (v) => (v ? "是" : "否")],
                      ["late_count", "迟到"],
                      ["early_count", "早退"],
                      ["missing_count", "缺卡"],
                      ["state", "状态", attendanceDayState],
                      ["leave_hours", "请假小时"],
                      ["out_hours", "外出小时"],
                      ["trip_hours", "出差小时"],
                    ],
                    [{ label: "详情", run: (day) => dialog("每日考勤详情", details({
                      "日期": day.date,
                      "状态": attendanceDayState(day.state),
                      "应出勤小时": day.expected_hours,
                      "已出勤": day.worked ? "是" : "否",
                      "缺勤": day.absent ? "是" : "否",
                      "迟到次数": day.late_count,
                      "早退次数": day.early_count,
                      "缺卡次数": day.missing_count,
                      "请假小时": day.leave_hours,
                      "外出小时": day.out_hours,
                      "出差小时": day.trip_hours,
                    })) }]
                  )
                ),
            },
          ]
        )
      );
    } catch (error) {
      content.replaceChildren(errorNode(error));
    }
  }
  await load();
}
