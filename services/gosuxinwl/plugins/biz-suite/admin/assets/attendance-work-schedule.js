import { el, asRows } from "./core.js";

const week = ["周一", "周二", "周三", "周四", "周五", "周六", "周日"];
const ids = value => [...new Set(asRows(value).filter(value => typeof value === "string" && value).map(String))];
const dayIDs = day => ids(day?.candidate_shift_ids?.length ? day.candidate_shift_ids : day?.shift_id ? [day.shift_id] : []);

export function fixedScheduleMode(category, savedMode = "") {
  if (category === "员工") return "production_employee";
  if (category === "生产职员") return savedMode === "after_sales" ? "after_sales" : "production_staff";
  return "";
}

export function initialWorkSchedule(config = {}) {
  const saved = config.work_schedule || {}, mode = fixedScheduleMode(config.category, saved.mode);
  if (!mode) return null;
  if (saved.mode) return { ...saved, mode, weekday_shift_ids: ids(saved.weekday_shift_ids), saturday_shift_ids: ids(saved.saturday_shift_ids), restday_shift_ids: ids(saved.restday_shift_ids) };
  const shifts = asRows(config.shifts), variants = new Set(shifts.map(shift => shift.overtime_variant?.shift?.id).filter(Boolean));
  const normal = shifts.filter(shift => !variants.has(shift.id) && !/^saturday/.test(shift.id) && !/周六/.test(shift.name || ""));
  const days = config.cycle?.mode === "weekly" ? asRows(config.cycle.weekdays) : [];
  // Only retain explicit day choices or an unambiguous base shift. Never infer
  // Saturday hours from a weekday shift or from an employee's display name.
  const work = ids(days.slice(0, 5).flatMap(dayIDs));
  const weekday = work.length ? work : normal.length === 1 ? [normal[0].id] : [];
  const saturday = days[5]?.category === "workday" ? dayIDs(days[5]) : shifts.filter(shift => /^saturday/.test(shift.id) || /周六/.test(shift.name || "")).map(shift => shift.id);
  const rest = dayIDs(days[6]);
  return { mode, weekday_shift_ids: weekday, saturday_shift_ids: mode === "production_employee" ? [] : saturday, restday_shift_ids: rest.length ? rest : [...weekday] };
}

export function workScheduleCycle(schedule) {
  if (!schedule) return null;
  return { mode: "weekly", weekdays: week.map((_, index) => {
    const working = index < (schedule.mode === "production_employee" ? 5 : 6);
    const selected = ids(!working ? schedule.restday_shift_ids : index === 5 ? schedule.saturday_shift_ids : schedule.weekday_shift_ids);
    return { shift_id: selected[0] || "", candidate_shift_ids: selected, category: working ? "workday" : "restday", scheduled_work: working };
  }) };
}

export function workScheduleEditor(config, getCategory, getShifts, changed = () => {}) {
  let value = initialWorkSchedule(config), previousCategory = getCategory();
  const node = el("section", { class: "work-schedule-editor", "data-work-schedule": "" });
  const heading = el("h3", {}, "工作日与周休"), description = el("p", { class: "hint" });
  const calendar = el("div", { class: "work-schedule-week", "aria-label": "每周出勤规则" });
  const choices = el("div", { class: "work-schedule-choices" });
  const special = el("select", { "aria-label": "生产职员工作制度", name: "production_staff_schedule" }, el("option", { value: "production_staff" }, "生产职员：周一至周六正班"), el("option", { value: "after_sales" }, "售后：每日一人留守，其余18:00下班"));
  const specialField = el("label", { class: "field" }, "生产职员工作制度", special);
  const duty = el("div", { class: "notice", "data-aftersales-duty": "" });
  const dutyGroup = el("input", { type:"text", name:"duty_group", maxlength:128, placeholder:"例如：售后留守组" });
  dutyGroup.value = value?.duty_group || "";
  dutyGroup.addEventListener("input", () => { value.duty_group = dutyGroup.value.trim(); });
  const dutyGroupField = el("label", {class:"field"}, "留守组名称（同组人员填写相同名称）", dutyGroup);
  const dutyCount = el("input", {type:"number",name:"duty_expected_members",min:1,step:1,max:1000});
  dutyCount.value = value?.duty_expected_members || 3;
  const dutyCountField = el("label", {class:"field"}, "本组应有人员数（发布前须全部明确）", dutyCount);
  const legacyNotice = config.work_schedule ? null : el("p", { class: "notice" }, "按人员类别整理固定工作日。保存前请核对下方班次；历史已发布制度会保留原版本。未选班次可暂存草稿，不能生效。");
  node.append(heading, description, specialField, calendar, choices, dutyGroupField, dutyCountField, duty, ...(legacyNotice ? [legacyNotice] : []));

  function picker(key, title, hint) {
    const selected = new Set(ids(value[key])), rows = asRows(getShifts()), known = new Set(rows.map(row => row.id));
    const list = el("div", { class: "work-schedule-shift-list" });
    for (const row of rows) {
      const input = el("input", { type: "checkbox", name: key, value: row.id }); input.checked = selected.has(row.id);
      input.addEventListener("change", () => { input.checked ? selected.add(row.id) : selected.delete(row.id); value[key] = [...selected]; changed(); });
      list.append(el("label", { class: "check-label" }, input, row.name || "未命名班次"));
    }
    for (const id of selected) if (!known.has(id)) {
      const input = el("input", { type: "checkbox", checked: true, "aria-label": "移除不可用班次 " + id });
      input.addEventListener("change", () => { if (!input.checked) { selected.delete(id); value[key] = [...selected]; changed(); } });
      list.append(el("label", { class: "check-label error-text" }, input, "原班次已不可用，请重新选择"));
    }
    return el("section", { class: "work-schedule-choice" }, el("h4", {}, title), el("p", { class: "hint" }, hint), list);
  }

  function refresh() {
    const category = getCategory(), mode = fixedScheduleMode(category, value?.mode);
    node.hidden = !mode;
    if (!mode) { previousCategory = category; return; }
    if (!value || category !== previousCategory) value = initialWorkSchedule({ ...config, category, work_schedule: value ? { ...value, mode } : undefined });
    previousCategory = category; value.mode = mode;
    special.value = mode; specialField.hidden = category !== "生产职员";
    description.textContent = mode === "production_employee" ? "生产员工：周一至周五为应出勤日；周六、周日无卡为休息，有效出勤计加班。工作日晚间可加班，也可不加班。" : "生产职员：周一至周六为应出勤日；周日无卡为休息，有效出勤计加班。周六使用表格中的独立作息。";
    calendar.replaceChildren(...week.map((name, index) => el("div", { class: "work-schedule-day" + (index >= (mode === "production_employee" ? 5 : 6) ? " is-rest" : "") }, el("strong", {}, name), el("span", {}, index >= (mode === "production_employee" ? 5 : 6) ? "休息 · 出勤计加班" : "正常出勤"))));
    choices.replaceChildren(picker("weekday_shift_ids", "周一至周五作息", "选择正常班；有无晚间加班由班次内的条件规则识别。确有白夜班时可选多个候选。"));
    if (mode !== "production_employee") choices.append(picker("saturday_shift_ids", "周六正班作息", "按源表选择周六班次，不沿用周一至周五的晚间加班要求。"));
    choices.append(picker("restday_shift_ids", mode === "production_employee" ? "周六、周日实际出勤识别" : "周日实际出勤识别", "保留可识别的班次；不要求休息日必须打卡。完整日间工时与晚间加班按核定口径分别计算。"));
    if (mode === "after_sales") choices.append(picker("duty_shift_ids", "20:00留守班次", "从已配置的班次中明确标记留守班。普通班和留守班都应加入对应工作日的候选列表。"));
    dutyGroupField.hidden = mode !== "after_sales";
    dutyCountField.hidden = mode !== "after_sales"; dutyCount.disabled = mode !== "after_sales";
    duty.hidden = mode !== "after_sales";
    duty.textContent = "按实际打卡识别：普通班18:00下班，留守班20:00下班。同组每天核对留守人数；缺卡或多人符合时转人工复核，不把其他人员18:00下班判为早退。";
  }
  special.addEventListener("change", () => { value.mode = special.value; refresh(); changed(); });
  refresh();
  return { node, refresh, active: () => !!fixedScheduleMode(getCategory(), value?.mode), read: () => fixedScheduleMode(getCategory(), value?.mode) ? { ...value, mode: fixedScheduleMode(getCategory(), value?.mode), ...(getCategory() === "员工" ? { saturday_shift_ids:[] } : {}), ...(value.mode === "after_sales" ? { duty_group:dutyGroup.value.trim(), duty_shift_ids:ids(value.duty_shift_ids), duty_expected_members:Number(dutyCount.value) } : {}) } : null,
    replace: next => { value = initialWorkSchedule({ ...next, category: getCategory() }); refresh(); changed(); } };
}
