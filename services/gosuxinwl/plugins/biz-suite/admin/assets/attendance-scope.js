import { el, asRows, field, button } from "./core.js";
import { departmentPath } from "./organization.js";

const text = value => String(value ?? "").normalize("NFKC").trim().toLocaleLowerCase();

export function scopeEditor(scope = {}, options = {}) {
  const employees = asRows(options.employees), departments = asRows(options.departments);
  const people = new Map(employees.map(row => [String(row.id), row]));
  const depts = new Map(departments.map(row => [String(row.id), row]));
  const chosenPeople = new Set(asRows(scope.employee_ids).map(String));
  const chosenDepts = new Set(asRows(scope.department_ids).map(String));
  const initialMode = scope.is_default ? "default" : chosenPeople.size && chosenDepts.size ? "union" : chosenDepts.size ? "departments" : "employees";
  const mode = field({ key: "scope_mode", label: "制度适用范围", options: [["employees", "按人员"], ["departments", "按部门"], ["union", "人员 + 部门（并集）"], ["default", "兜底默认"]] }, initialMode);
  const hadDefaultExtras = !!scope.is_default && !!(chosenPeople.size || chosenDepts.size);
  const preserveExtras = el("input", { type: "checkbox", name: "scope_preserve_default_extras", checked: hadDefaultExtras });
  const legacyDefault = el("label", { class: "scope-check" }, preserveExtras, "保留默认制度原有的指定优先范围");
  const descendants = el("input", { type: "checkbox", name: "scope_include_children", checked: scope.include_children ?? true });
  const modeHint = el("p", { class: "notice", "data-scope-summary": "", "aria-live": "polite" });
  const staged = el("p", { class: "hint", "data-scope-staged": "" });
  const root = el("section", { "data-attendance-scope": "" }, mode.wrapper, legacyDefault, modeHint, staged);
  const path = row => departmentPath(row, departments) || row.name || "部门名称待解析";
  const employeeLabel = row => [row.name || "姓名待解析", row.employee_no || "未填写工号", row.department || (depts.has(String(row.dept_id)) ? path(depts.get(String(row.dept_id))) : "未归属本地部门")].join(" · ");
  function inDepartment(row, id, includeChildren) {
    if (!id) return true;
    let current = String(row.dept_id || 0);
    if (id === "unassigned") return current === "0";
    const seen = new Set();
    while (current && current !== "0" && !seen.has(current)) {
      if (current === id) return true;
      if (!includeChildren) return false;
      seen.add(current); const node = depts.get(current);
      current = String(node?.pid ?? node?.parent_id ?? 0);
    }
    return false;
  }
  const employeeSearch = field({ key: "scope_employee_search", label: "姓名 / 工号", type: "search", placeholder: "输入中文姓名或工号" });
  const departmentFilter = field({ key: "scope_employee_department", label: "筛选候选人员的部门", options: [["", "全部本地部门"], ["unassigned", "未归属本地部门"], ...departments.map(row => [row.id, path(row)])] });
  const filterChildren = el("input", { type: "checkbox", name: "scope_employee_include_children", checked: true });
  const departmentSearch = field({ key: "scope_department_search", label: "查找适用部门", type: "search", placeholder: "输入部门名称或组织路径" });
  const employeeFilter = () => {
    const terms = text(employeeSearch.input.value).split(/\s+/).filter(Boolean);
    return employees.filter(row => terms.every(term => text(row.name).includes(term) || text(row.employee_no).includes(term)) && inDepartment(row, departmentFilter.input.value, filterChildren.checked));
  };
  const departmentResults = () => departments.filter(row => text(path(row)).includes(text(departmentSearch.input.value)));
  const panels = [];
  function picker(kind, title, selected, lookup, filter, label, controls) {
    const results = el("div", { class: "scope-options", "data-scope-results": kind });
    const selectedList = el("div", { class: "scope-selected", "data-scope-selected": kind });
    const counts = el("p", { class: "hint", "data-scope-count": kind, "aria-live": "polite" });
    const pane = el("section", { class: "scope-pane", "data-scope-pane": kind }, el("h3", {}, title), ...controls,
      el("div", { class: "scope-actions" }, button("全选当前结果", () => { filter().forEach(row => selected.add(String(row.id))); refresh(); }, "small", { "data-scope-select-visible": kind }), button("清除当前结果", () => { filter().forEach(row => selected.delete(String(row.id))); refresh(); }, "small", { "data-scope-clear-visible": kind })), counts,
      el("div", { class: "scope-columns" }, el("section", {}, el("h4", {}, "候选结果"), results), el("section", {}, el("h4", {}, "已选清单（不受搜索条件影响）"), selectedList)));
    function render() {
      const visible = filter();
      const visibleIDs = new Set(visible.map(row => String(row.id)));
      const hidden = [...selected].filter(id => !visibleIDs.has(id)).length;
      counts.textContent = `当前结果 ${visible.length} 项 · 已选 ${selected.size} 项${hidden ? `（其中 ${hidden} 项不在当前筛选结果中）` : ""}`;
      results.replaceChildren(...(visible.length ? visible.map(row => {
        const id = String(row.id), check = el("input", { type: "checkbox", checked: selected.has(id), "data-scope-id": id });
        check.addEventListener("change", () => { check.checked ? selected.add(id) : selected.delete(id); refresh(); });
        return el("label", { class: "scope-check" }, check, el("span", {}, label(row)));
      }) : [el("p", { class: "hint" }, "没有符合筛选条件的结果，已选项目仍保留。")]));
      selectedList.replaceChildren(...(selected.size ? [...selected].map((id, index) => {
        const row = lookup.get(id);
        return el("div", { class: "scope-selection", "data-selected-id": id }, el("div", {}, row ? label(row) : `${kind === "employees" ? "员工" : "部门"}资料暂不可用（历史选择仍保留）`, !row && el("small", { class: "hint" }, `历史选择 #${index + 1} · 请核对后再移除`)), button("移除", () => { selected.delete(id); refresh(); }, "small", { "aria-label": `移除${row ? label(row) : "历史选择 #" + (index + 1)}` }));
      }) : [el("p", { class: "hint" }, "尚未选择")]));
    }
    panels.push({ pane, render }); return pane;
  }
  const employeePanel = picker("employees", "指定人员", chosenPeople, people, employeeFilter, employeeLabel, [
    el("div", { class: "scope-filter-grid" }, employeeSearch.wrapper, departmentFilter.wrapper), el("label", { class: "scope-check" }, filterChildren, "候选人员包含下级部门"),
    el("p", { class: "hint" }, "这里的部门筛选只缩小候选列表，不会自动把整个部门加入制度；同名员工按工号与本地主部门区分。")
  ]);
  const departmentPanel = picker("departments", "适用本地部门", chosenDepts, depts, departmentResults, path, [departmentSearch.wrapper, el("label", { class: "scope-check" }, descendants, "制度范围包含所选部门的下级部门")]);
  root.append(employeePanel, departmentPanel);
  function active() {
    const value = mode.input.value, legacy = value === "default" && hadDefaultExtras && preserveExtras.checked;
    return { employees: value === "employees" || value === "union" || legacy, departments: value === "departments" || value === "union" || legacy, default: value === "default", legacy };
  }
  function refresh() {
    const selected = active();
    employeePanel.hidden = !selected.employees; departmentPanel.hidden = !selected.departments;
    legacyDefault.hidden = !(selected.default && hadDefaultExtras); legacyDefault.style.display = legacyDefault.hidden ? "none" : "";
    modeHint.textContent = selected.default ? `兜底默认：适用于没有匹配其他更具体制度的人员。${selected.legacy ? `同时保留原有 ${chosenPeople.size} 名人员、${chosenDepts.size} 个部门的优先匹配，保存不会丢失历史范围。` : "本模式不提交暂存的人员或部门名单。"}` : mode.input.value === "union" ? `适用于所选 ${chosenPeople.size} 名人员，或所选 ${chosenDepts.size} 个部门的人员（并集）；不是只限这些部门里的已选人员。` : mode.input.value === "departments" ? `适用于所选 ${chosenDepts.size} 个本地部门${descendants.checked ? "及下级部门" : "本身"}的人员；人员名单仅暂存，不参与本次范围。` : `仅适用于已选 ${chosenPeople.size} 名人员；部门名单仅暂存，不参与本次范围。`;
    staged.textContent = `匹配同一天的多条制度时：指定人员优先，其次本部门及最近上级部门，最后默认制度；同级冲突须核对。${!selected.employees && chosenPeople.size ? ` 暂存人员 ${chosenPeople.size} 名，切回后可继续编辑。` : ""}${!selected.departments && chosenDepts.size ? ` 暂存部门 ${chosenDepts.size} 个，切回后可继续编辑。` : ""}`;
    panels.forEach(panel => panel.render());
  }
  for (const control of [mode.input, preserveExtras, descendants, employeeSearch.input, departmentFilter.input, filterChildren, departmentSearch.input]) control.addEventListener(control.type === "search" ? "input" : "change", refresh);
  refresh();
  return { node: root, read: () => {
    const selected = active();
    const ids = selected.departments ? [...chosenDepts].map(id => { const n = Number(id); if (!Number.isSafeInteger(n) || n <= 0) throw Error("所选部门含无法识别的历史记录，请先核对部门清单。"); return n; }) : [];
    return { employee_ids: selected.employees ? [...chosenPeople] : [], department_ids: ids, include_children: descendants.checked, is_default: selected.default };
  } };
}
