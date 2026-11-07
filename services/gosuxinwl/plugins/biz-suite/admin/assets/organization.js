import { el, api, can, state, button, dataTable, errorNode, dialog } from "./core.js";

export const hasFullHRMRead = () => state.permissions.some((p) =>
  ["*", "biz-suite.*", "biz-suite.hrm.*", "biz-suite.hrm.read", "biz-suite.hrm.write", "biz-suite.hrm.manage"].includes(p));

export function employeeScopeNotice() {
  if (hasFullHRMRead()) return "人事管理员可查看全公司；部门负责人可查看主部门及下级部门；员工、职员仅可查看本人。";
  if (state.permissions.includes("biz-suite.hrm.department")) return "当前为部门负责人范围：可查看主部门及下级部门的员工与考勤，只读访问。";
  return "当前为员工、职员本人范围：仅可查看本人的档案与考勤，只读访问。";
}

export function departmentPath(row, rows = []) {
  if (row?.path) return row.path;
  const directory = new Map(rows.map((item) => [String(item.wecom_id ?? item.id), item]));
  const names = [], seen = new Set();
  for (let current = row; current;) {
    const id = String(current.wecom_id ?? current.id);
    if (seen.has(id)) break;
    seen.add(id);
    if (current.name) names.unshift(current.name);
    current = directory.get(String(current.pid ?? current.parent_id));
  }
  return names.join(" / ");
}

export function mappingLabel(value) {
  return ({ mapped: "已关联", linked: "已关联", synced: "已同步", ready: "已就绪", pending: "待同步", conflict: "存在冲突", unmapped: "未关联", disabled: "账号停用", missing_account: "未关联账号", missing_department: "未关联部门", missing_employee_no: "缺少工号", failed: "同步失败" })[value] || "待核对";
}

function pruneSummary(report, preview) {
  const counts = [
    ["employees_filtered", preview ? "将移出当前员工" : "已移出当前员工"],
    ["accounts_removed", preview ? "将移除自动导入账号" : "已移除自动导入账号"],
    ["departments_removed", preview ? "将移除本地部门" : "已移除本地部门"],
    ["already_filtered", "此前已过滤员工"],
  ];
  const node = el("div", { "data-prune-summary": preview ? "preview" : "result" },
    el("div", { class: "organization-counts" }, counts.map(([key, label]) =>
      el("span", {}, label + " ", el("strong", {}, Number(report?.[key] || 0))))),
    el("p", { class: "notice" }, "按已保存的过滤规则移出当前员工、通讯录和本地组织列表，保留历史考勤、审批及消息。仅移除系统自动导入的账号，人工账号和管理员账号会保留并提示核对。本操作只处理本地数据，不会修改企业微信源通讯录。"),
    el("p", { class: "hint" }, "解除过滤后，被移除的本地账号或部门需由管理员恢复；恢复账号保持停用，由管理员确认后再启用。"));
  const conflicts = Array.isArray(report?.conflicts) ? report.conflicts : [];
  if (conflicts.length) node.append(el("h3", {}, `需要人工核对（${conflicts.length}）`), dataTable(conflicts, [
    ["entity_type", "对象", (value) => ({ department: "部门", employee: "员工", account: "账号", user: "员工" })[value] || "组织成员"],
    ["name", "姓名 / 部门"], ["employee_no", "工号"], ["department_path", "所属部门"],
    ["reason", "处理说明"],
  ], [], { search: false, pageSize: 20 }));
  return node;
}

export function organizationTools({ refresh }) {
  const holder = el("section", { class: "organization-tools", "data-organization-sync": "" });
  if (!can("wecom", "manage")) return holder;
  const epoch = state.epoch;
  const current = () => epoch === state.epoch && holder.isConnected;
  let busy = false;
  const setBusy = (value) => {
    busy = value;
    sync.disabled = value;
    prune.disabled = value;
  };
  const result = el("div", { class: "organization-result", "aria-live": "polite" });
  const sync = button("同步组织与账号", async () => {
    if (busy) return;
    setBusy(true);
    sync.textContent = "正在同步…";
    result.replaceChildren(el("p", { class: "hint" }, "正在按通讯录建立本地部门并关联员工账号…"));
    try {
      await api("/wecom/organization/sync", {}, "POST");
      if (current()) await refresh();
    } catch (error) {
      if (current()) result.replaceChildren(errorNode(error));
    } finally {
      if (current()) { setBusy(false); sync.textContent = "同步组织与账号"; }
    }
  }, "primary");
  const prune = button("清理已过滤部门和人员", () => {
    if (busy) return;
    setBusy(true);
    const content = el("section", { "data-organization-prune": "", "aria-live": "polite" });
    const popup = dialog("清理已过滤部门和人员", content);
    let loading = false, applying = false;
    const present = () => current() && popup.modal.isConnected;
    const cancel = button("取消", popup.close);
    popup.modal.addEventListener("close", () => { if (!applying) setBusy(false); });
    popup.modal.addEventListener("cancel", (event) => { if (applying) event.preventDefault(); });
    const loadPreview = async () => {
      if (loading || applying) return;
      loading = true;
      content.replaceChildren(el("p", { class: "hint" }, "正在预览已保存过滤规则的影响，尚未修改数据…"), cancel);
      try {
        const report = await api("/wecom/organization/prune");
        if (!present()) return;
        const feedback = el("div", { "data-prune-feedback": "" });
        const confirm = button("确认清理", async () => {
          if (applying || !present()) return;
          applying = true;
          confirm.disabled = cancel.disabled = true;
          popup.modal.querySelector(".close").disabled = true;
          confirm.textContent = "正在清理…";
          feedback.replaceChildren();
          try {
            const applied = await api("/wecom/organization/prune", {}, "POST");
            if (!present()) return;
            content.replaceChildren(el("h3", {}, "清理完成"), pruneSummary(applied, false), button("完成", popup.close, "primary"));
            await refresh();
          } catch (error) {
            if (present()) feedback.replaceChildren(errorNode(error), el("p", { class: "hint" }, "本次清理未完成，请稍后点击“确认清理”重试。"));
          } finally {
            applying = false;
            confirm.disabled = cancel.disabled = false;
            confirm.textContent = "确认清理";
            const close = popup.modal.querySelector(".close");
            if (close) close.disabled = false;
            if (!popup.modal.isConnected) setBusy(false);
          }
        }, "primary");
        content.replaceChildren(pruneSummary(report, true),
          el("p", { class: "hint" }, "这是清理预览，尚未修改数据。确认时会重新校验最新规则与账号归属。"),
          feedback, el("footer", { class: "form-actions" }, cancel, confirm));
      } catch (error) {
        if (present()) content.replaceChildren(errorNode(error), button("重试预览", loadPreview), cancel);
      } finally { loading = false; }
    };
    void loadPreview();
  }, "", { "data-prune-open": "" });
  const load = async () => {
    result.replaceChildren(el("p", { class: "hint" }, "正在读取上次同步结果…"));
    try {
      const report = await api("/wecom/organization/sync");
      if (!current()) return;
      if (!report || !Object.keys(report).length) {
        result.replaceChildren(el("p", { class: "hint" }, "尚未同步组织与账号。请先在同步控制台同步通讯录。"));
        return;
      }
      const counts = [
        ["departments_created", "新增部门"], ["departments_updated", "更新部门"],
        ["accounts_created", "新增停用账号"], ["accounts_updated", "更新账号"],
        ["employees_linked", "关联员工"], ["skipped", "跳过"], ["conflict_count", "冲突"],
      ];
      result.replaceChildren(el("h3", {}, "上次同步结果"),
        el("div", { class: "organization-counts" }, counts.map(([key, label]) =>
          el("span", {}, label + " ", el("strong", {}, Number(report[key] || 0))))));
      const conflicts = Array.isArray(report.conflicts) ? report.conflicts : [];
      if (conflicts.length) result.append(button(`查看冲突（${conflicts.length}）`, () => dialog("组织与账号同步冲突", dataTable(conflicts, [
        ["entity_type", "对象", (value) => ({ department: "部门", employee: "员工", account: "账号", user: "员工" })[value] || "组织成员"],
        ["name", "姓名 / 部门"], ["employee_no", "工号"], ["department_path", "所属部门"],
        ["reason", "处理说明", (value) => el("span", { class: "organization-value" }, value || "待核对")],
      ], [], { search: false, pageSize: 20 })), "small"));
    } catch (error) {
      if (current()) result.replaceChildren(errorNode(error), button("重试读取结果", load, "small"));
    }
  };
  holder.append(el("div", { class: "toolbar" }, sync, prune),
    el("p", { class: "hint" }, "使用已同步的企业微信通讯录建立本地部门，显示名使用姓名，登录账号使用工号。缺少或重复工号会进入冲突列表。"),
    el("p", { class: "notice" }, "新建账号默认停用。管理员设置密码并分配合适角色后，再启用账号。"), result);
  // The resource attaches this section immediately after constructing it.
  queueMicrotask(() => { if (current()) void load(); });
  return holder;
}
