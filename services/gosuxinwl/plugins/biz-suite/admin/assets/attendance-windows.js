import { el, field, button } from "./core.js";

export const windowKeys = ["in_start_second", "in_end_second", "out_start_second", "out_end_second"];
const labels = ["上班卡最早时间", "上班卡最晚时间", "下班卡最早时间", "下班卡最晚时间"];
const dayOptions = [[-1, "前日"], [0, "当日"], [1, "次日"], [2, "第3日"], [3, "第4日"]];
export function windowClock(value, end = false) {
  if (!Number.isInteger(value)) return { day: 0, time: "" };
  const second = value - (end ? 1 : 0), day = Math.floor(second / 86400), within = second - day * 86400;
  return { day, time: [Math.floor(within / 3600), Math.floor(within % 3600 / 60), within % 60].map(n => String(n).padStart(2, "0")).join(":") };
}
export function windowSecond(day, clock, end = false) {
  if (!/^(?:[01]\d|2[0-3]):[0-5]\d(?::[0-5]\d)?$/.test(clock)) return null;
  const [h, m, s = 0] = clock.split(":").map(Number);
  return Number(day) * 86400 + h * 3600 + m * 60 + s + (end ? 1 : 0);
}
export function windowText(second, end = false) {
  const clock = windowClock(second, end);
  if (!clock.time) return "待配置";
  return (dayOptions.find(([n]) => n === clock.day)?.[1] || `第${clock.day + 1}日`) + " " + clock.time;
}
export function inspectWindows(segments) {
  const rows = [], issues = [];
  for (const segment of segments) {
    if (segment.kind === "break" || segment.punch_mode === "none") continue;
    const mode = segment.punch_mode || "both", value = segment.local_windows || {};
    for (const direction of mode === "both" ? ["in", "out"] : [mode]) {
      const start = value[direction + "_start_second"], end = value[direction + "_end_second"], name = `${segment.name || "未命名时段"}·${direction === "in" ? "上班卡" : "下班卡"}`;
      if (!Number.isInteger(start) || !Number.isInteger(end)) { issues.push(`${name}窗口待配置`); continue; }
      if (start >= end) { issues.push(`${name}最晚时间不能早于最早时间`); continue; }
      rows.push({ name, start, end });
    }
  }
  rows.sort((a, b) => a.start - b.start || a.end - b.end);
  for (let i = 0; i < rows.length; i++) for (let j = i + 1; j < rows.length && rows[j].start < rows[i].end; j++) {
    if (rows[j].end > rows[i].start) issues.push(`${rows[i].name}与${rows[j].name}窗口重叠，重叠打卡需复核；请调整边界`);
  }
  return { rows, issues };
}
export function windowEditor(value, onChange, copy) {
  const controls = {}, node = el("section", { class: "attendance-window-editor", "data-local-windows": "" }, el("h4", {}, "取卡时间窗口"), el("p", { class: "hint" }, "选择前日、当日或次日，并精确到秒。最晚时间包含该秒；相邻窗口不要重复。未填写会保留为待配置，不套用默认范围。")), grid = el("div", { class: "form-grid" });
  for (const [index, key] of windowKeys.entries()) {
    const saved = windowClock(value?.[key], key.endsWith("end_second")), day = field({ key: key + "_day", label: labels[index] + "日期", options: dayOptions }, saved.day), time = field({ key, label: labels[index], type: "time", step: 1 }, saved.time);
    for (const control of [day.input, time.input]) control.addEventListener("input", () => onChange?.());
    controls[key] = { day, time }; grid.append(el("div", { class: "form-grid" }, day.wrapper, time.wrapper));
  }
  node.append(grid);
  if (copy) node.append(button("复制到相同作息的时段", () => copy(read())));
  function read() { return Object.fromEntries(windowKeys.map(key => [key, windowSecond(controls[key].day.input.value, controls[key].time.input.value, key.endsWith("end_second"))])); }
  return { node, read, write(next) { for (const key of windowKeys) { const item = windowClock(next?.[key], key.endsWith("end_second")); controls[key].day.input.value = item.day; controls[key].time.input.value = item.time; } onChange?.(); }, mode(mode, kind) { for (const key of windowKeys) { const disabled = kind === "break" || mode === "none" || (mode === "in" && key.startsWith("out")) || (mode === "out" && key.startsWith("in")); for (const control of [controls[key].day, controls[key].time]) { control.input.disabled = disabled; control.wrapper.hidden = disabled; } } } };
}
export function windowTimeline(segments) {
  const { rows, issues } = inspectWindows(segments), node = el("section", { "data-window-preview": "" }, el("h4", {}, "打卡归属时间轴"));
  if (issues.length) node.append(el("ul", { class: "notice" }, issues.map(message => el("li", {}, message))));
  if (!rows.length) { node.append(el("p", {}, "填写取卡窗口后显示时间轴。")); return node; }
  const start = Math.min(...rows.map(row => row.start)), span = Math.max(1, Math.max(...rows.map(row => row.end)) - start);
  for (const row of rows) node.append(el("div", { class: "attendance-window-row" }, el("span", {}, `${row.name}：${windowText(row.start)} — ${windowText(row.end, true)}`), el("div", { class: "attendance-window-track", style: "height:8px;background:#e5e7eb;border-radius:4px;margin:5px 0 12px;overflow:hidden" }, el("div", { style: `height:100%;background:#2563eb;margin-left:${(row.start - start) / span * 100}%;width:${(row.end - row.start) / span * 100}%` }))));
  return node;
}
