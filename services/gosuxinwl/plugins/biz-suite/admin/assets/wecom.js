import {
  el,
  api,
  asRows,
  can,
  page,
  surface,
  button,
  dataTable,
  edit,
  form,
  inspect,
  status,
  errorNode,
  toast,
  parseJSON,
  state,
  safe,
  details,
  dialog,
} from "./core.js";
import { organizationTools, departmentPath, mappingLabel } from "./organization.js";
export { dashboard } from "./wecom-console.js";
export function exceptionErrorText(value, row = {}) {
  if (value === "unsupported checkin_type; raw record retained") return String(row.checkin_type || "").trim()
    ? "暂不支持该打卡类型，原始记录已保留"
    : "企业微信未返回打卡类型，无法识别上下班或外出；原始记录已保留";
  const messages = {
    "normalized approval failed HRM validation or persistence": "审批起止、申请时长或业务字段校验未通过；原文已保留，请核对",
    "approval template is not enabled or mapped": "审批模板未配置或未启用；原文已保留，请检查模板映射及启用状态",
    "approval template requires an exact attendance_control_id": "审批模板缺少准确的时段控件 ID；请检查控件映射，原文已保留",
    "approval template type must be leave, out, trip or overtime": "审批模板业务类型无效；请选择请假、外出、出差或加班",
  };
  return Object.hasOwn(messages, value) ? messages[value] : value || "—";
}
export function exceptionCountText(data, kind) {
  const shown = asRows(data[kind]).length, total = Number(data.totals?.[kind]);
  if (!Number.isSafeInteger(total) || total < shown) return `当前显示 ${shown} 条`;
  return total > shown ? `仅显示前 ${shown} 条 / 共 ${total} 条；其余记录仍保留在系统中` : `显示 ${shown} 条 / 共 ${total} 条`;
}
export async function templates() {
  const body = page(
    "审批模板映射",
    "按企业微信模板与控件 ID 映射请假、外出、出差、加班时段；未映射数据进入异常列表。"
  );
  const panel = surface();
  body.append(panel);
  try {
    const rows = await api("/wecom/templates");
    const save = (row) => {
      const mapping = parseJSON(row?.mapping_json || row?.mapping, {});
      edit(
        row ? "编辑审批模板" : "新增审批模板",
        [
          { key: "template_id", label: "企业微信模板 ID", required: true },
          {
            key: "approval_type",
            label: "业务类型",
            options: [
              ["leave", "请假"],
              ["out", "外出"],
              ["trip", "出差"],
              ["overtime", "加班"],
            ],
          },
          {
            key: "attendance_control_id",
            label: "时段控件 ID",
            required: true,
            hint: "企业微信 Vacation / Attendance / DateRange 控件 ID；加班 Attendance 类型为 5",
          },
          { key: "reason_control_id", label: "原因控件 ID（可选）" },
          {
            key: "destination_control_id",
            label: "外出 / 出差地点控件 ID（可选）",
          },
          {
            key: "enabled",
            label: "启用状态",
            type: "number",
            options: [
              [1, "启用"],
              [0, "停用"],
            ],
          },
        ],
        { ...row, ...mapping },
        (data) =>
          api("/wecom/templates", {
            template_id: data.template_id,
            approval_type: data.approval_type,
            enabled: data.enabled,
            mapping: {
              attendance_control_id: data.attendance_control_id,
              reason_control_id: data.reason_control_id,
              destination_control_id: data.destination_control_id,
            },
          }),
        templates
      );
    };
    panel.replaceChildren(
      el(
        "div",
        { class: "subheading" },
        el("h2", {}, "模板列表"),
        can("wecom", "write")
          ? button("+ 新增模板", () => save(), "primary")
          : null
      ),
      el(
        "p",
        { class: "notice" },
        "使用模板的实际控件 ID，不使用展示名称猜测字段。停用或未配置的模板不会自动生成有效考勤。"
      ),
      dataTable(
        rows,
        [
          ["template_id", "模板 ID"],
          [
            "approval_type",
            "业务",
            (v) => ({ leave: "请假", out: "外出", trip: "出差", overtime: "加班" }[v] || v),
          ],
          [
            "mapping_json",
            "时段控件",
            (v) => parseJSON(v, {}).attendance_control_id || "未映射",
          ],
          ["enabled", "状态", (v) => (Number(v) ? "启用" : "停用")],
          ["updated_at", "更新时间"],
        ],
        [
          ...(can("wecom", "write") ? [{ label: "编辑", run: save }] : []),
          { label: "详情", run: inspect },
        ]
      )
    );
  } catch (error) {
    panel.replaceChildren(errorNode(error));
  }
}
export async function exceptions() {
  const epoch = state.epoch;
  const body = page(
    "同步异常",
    "检查未匹配员工、缺少模板或控件解析失败的原始记录；修正映射后重新同步相同时间窗口。"
  );
  const holder = surface();
  body.append(holder);
  try {
    const data = await api("/wecom/exceptions");
    if (epoch !== state.epoch || !holder.isConnected) return;
    const identityColumns = [
      ["employee_name", "员工姓名", (value, row) => {
        const name = String(value || "").trim();
        const internalIDs = [row.wecom_userid, row.employee_id, row.user_id, row.id]
          .filter((id) => id != null && id !== "").map(String);
        return name && !internalIDs.includes(name) ? name : "未匹配员工";
      }],
      ["employee_no", "工号", (value) => String(value || "").trim() || "未填写工号"],
    ];
    holder.replaceChildren(
      el("div", { class: "subheading" }, el("h2", {}, "异常记录"), button("刷新", exceptions)),
      el("section", { "data-exception-kind": "contacts" },
        el("h2", {}, "通讯录异常"),
        el("p", { class: "notice", "data-exception-count": "contacts" }, exceptionCountText(data, "contacts")),
        dataTable(asRows(data.contacts), [
          ...identityColumns,
          ["synced_at", "同步时间"],
          ["processing_error", "处理错误", exceptionErrorText],
        ], [{ label: "原始记录", run: inspect }])),
      el("section", { "data-exception-kind": "checkins" }, el("h2", {}, "打卡异常"),
      el("p", { class: "notice", "data-exception-count": "checkins" }, exceptionCountText(data, "checkins")),
      dataTable(
        asRows(data.checkins),
        [
          ...identityColumns,
          ["checkin_time", "打卡时间"],
          ["processing_error", "处理错误", exceptionErrorText],
        ],
        [{ label: "原始记录", run: inspect }]
      )),
      el("section", { "data-exception-kind": "approvals" }, el("h2", {}, "审批异常"),
      el("p", { class: "notice", "data-exception-count": "approvals" }, exceptionCountText(data, "approvals")),
      el("p", { class: "notice" }, "优先显示已配置模板的处理异常，再显示未配置模板的记录。" + (data.approval_counts ? `已配置模板异常 ${Number(data.approval_counts.configured) || 0} 条；未配置模板 ${Number(data.approval_counts.unmapped) || 0} 条。` : "")),
      dataTable(
        asRows(data.approvals),
        [
          ...identityColumns,
          ["sp_no", "审批单号"],
          ["template_id", "模板 ID"],
          ["status", "状态", status],
          ["processing_error", "处理错误", exceptionErrorText],
        ],
        [{ label: "原始记录", run: inspect }]
      ))
    );
  } catch (error) {
    if (epoch === state.epoch && holder.isConnected)
      holder.replaceChildren(errorNode(error), button("重试", exceptions));
  }
}
export async function departments() {
  const epoch = state.epoch;
  const body = page(
    "企业微信部门映射",
    "按企业微信组织架构同步本地部门，也可将部门关联到已有的本地部门。"
  );
  const holder = surface();
  body.append(holder);
  try {
    const [rows, options] = await Promise.all([
      api("/wecom/departments/mapping"),
      api("/wecom/departments/options"),
    ]);
    if (epoch !== state.epoch || !holder.isConnected) return;
    const hostPath = (row) => row.host_department_path || row.host_department_name ||
      departmentPath(asRows(options.departments).find((d) => Number(d.id) === Number(row.host_dept_id)), asRows(options.departments)) || "未关联部门";
    const wecomPath = (row) => row.path || departmentPath(row, asRows(rows));
    const change = (row) =>
      edit(
        "关联本地部门",
        [
          {
            key: "name",
            label: "企业微信部门",
            readonly: true,
          },
          {
            key: "host_dept_id",
            label: "本地部门",
            type: "number",
            options: [
              [0, "取消关联"],
              ...asRows(options.departments).map((d) => [
                d.id,
                departmentPath(d, asRows(options.departments)) || "未命名部门",
              ]),
            ],
          },
        ],
        row,
        (data) => api("/wecom/departments/mapping", { wecom_id: row.wecom_id, host_dept_id: data.host_dept_id }),
        departments
      );
    holder.replaceChildren(
      el("div", { class: "subheading" }, el("h2", {}, "部门关联"), button("刷新", departments)),
      organizationTools({ refresh: departments }),
      dataTable(
        rows,
        [
          ["name", "企业微信部门"],
          ["path", "组织层级", (_value, row) => wecomPath(row) || "待补充"],
          ["parent_name", "上级部门", (value, row) => value || asRows(rows).find((d) => Number(d.wecom_id) === Number(row.parent_id))?.name || "根部门"],
          ["host_department_path", "本地部门", (_value, row) => hostPath(row)],
          ["mapping_status", "映射状态", mappingLabel],
          ["mapping_message", "映射说明", (value) => el("span", { class: "organization-value" }, value || "—")],
          ["synced_at", "同步时间"],
        ],
        [
          ...(can("wecom", "manage")
            ? [{ label: "设置映射", run: change }]
            : []),
          { label: "详情", run: (row) => dialog("部门映射详情", details({
            "企业微信部门": row.name, "组织层级": wecomPath(row),
            "上级部门": row.parent_name || asRows(rows).find((d) => Number(d.wecom_id) === Number(row.parent_id))?.name || "根部门",
            "本地部门": hostPath(row), "映射状态": mappingLabel(row.mapping_status),
            "映射说明": row.mapping_message || "—", "同步时间": row.synced_at || "—",
          })) },
        ]
      )
    );
  } catch (error) {
    if (epoch === state.epoch && holder.isConnected)
      holder.replaceChildren(errorNode(error), button("重试", departments));
  }
}
