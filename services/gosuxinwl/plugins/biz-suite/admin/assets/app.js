import {
  el,
  api,
  can,
  state,
  page,
  surface,
  button,
  errorNode,
  navigate,
  BASE,
} from "./core.js";
import * as hrm from "./hrm.js";
import * as attendance from "./attendance-v2.js";
import * as wecom from "./wecom.js";
import * as im from "./im.js";
import * as openim from "./openim.js";
import { createIMSession } from "./session.js";
import { hasFullHRMRead } from "./organization.js";
const routes = [
  ["hrm/overview", "工作总览", "智慧HRM", null, overview],
  ["hrm/employees", "员工与账号映射", "人事与考勤", "hrm", hrm.employees],
  ["hrm/groups", "考勤组与班次", "人事与考勤", "hrm", hrm.groups],
  ["hrm/holidays", "节假日与调休", "人事与考勤", "hrm", hrm.holidays],
  ["hrm/clocks", "打卡记录", "人事与考勤", "hrm", hrm.clocks],
  ["hrm/month", "月度考勤", "人事与考勤", "hrm", hrm.month],
  ["hrm/attendance-rules", "考勤设置", "智慧考勤", "hrm", attendance.rules],
  ["hrm/attendance-rosters", "人员排班", "本地考勤核算", "hrm", attendance.rosters],
  ["hrm/attendance-batches", "考勤计算批次", "本地考勤核算", "hrm", attendance.batches],
  ["hrm/attendance-results", "考勤明细与异常", "智慧考勤", "hrm", attendance.results],
  ["hrm/attendance-months", "月度考勤", "智慧考勤", "hrm", attendance.months],
  ["hrm/attendance-imports", "工作簿导入与核对", "本地考勤核算", "hrm", attendance.imports],
  ["hrm/attendance-ledger", "调休台账", "本地考勤核算", "hrm", attendance.ledger],
  ["hrm/attendance-overtime", "加班凭据", "本地考勤核算", "hrm", attendance.overtime],
  ["hrm/attendance-audit", "考勤操作审计", "本地考勤核算", "hrm", attendance.audit],
  ["hrm/leaves", "请假记录", "人事与考勤", "hrm", hrm.leaves],
  ["hrm/outs", "外出记录", "人事与考勤", "hrm", hrm.outs],
  ["hrm/trips", "出差记录", "人事与考勤", "hrm", hrm.trips],
  ["im/chat", "消息中心", "企业即时通讯", "im", () => state.openimEnabled ? openim.launch() : im.chat()],
  ["im/directory", "企业通讯录", "企业即时通讯", "im", im.directory],
  ["im/groups", "群组", "企业即时通讯", "im", im.groups],
  ["im/channels", "频道", "企业即时通讯", "im", im.channels],
  ["im/face-packs", "表情包", "企业即时通讯", "im", im.facePacks],
  [
    "im/sensitive-words",
    "敏感词",
    "企业即时通讯",
    "im",
    im.sensitiveWords,
    "manage",
  ],
  ["im/manager", "通讯管理", "企业即时通讯", "im", () => state.openimEnabled ? openim.manager() : im.manager(), "manage"],
  ["wecom/status", "同步控制台", "企业微信", "wecom", wecom.dashboard],
  ["wecom/templates", "审批模板映射", "企业微信", "wecom", wecom.templates],
  ["wecom/departments", "部门映射", "企业微信", "wecom", wecom.departments],
  ["wecom/exceptions", "同步异常", "企业微信", "wecom", wecom.exceptions],
];
const workspaces = {
  hrm: { title: "智慧HRM", mark: "/resource/static/brand/hr/logo.png?v=puty3", home: "hrm/overview", note: "人事 · 考勤 · 企业微信" },
  im: { title: "企业即时通讯", mark: "/resource/static/brand/im/logo.png?v=puty3", home: "im/chat", note: "团队沟通与消息" },
};
const hasWorkspace = (key) => key === "im" ? !state.imSessionDenied && can("im") : can("hrm") || can("wecom");
const defaultWorkspace = () => hasWorkspace("hrm") || !hasWorkspace("im") ? "hrm" : "im";
const routeWorkspace = (path) => path === "im" || path.startsWith("im/") ? "im" :
  /^(hrm|wecom)(\/|$)/.test(path) ? "hrm" : state.workspace || defaultWorkspace();
const routeAllowed = (route) => route[0] === "hrm/overview" ? hasWorkspace("hrm") :
  !(state.openimEnabled && route[3] === "im" && !["im/chat", "im/manager"].includes(route[0])) &&
  !(route[3] === "im" && state.imSessionDenied) &&
  (!["hrm/groups", "hrm/holidays", "hrm/attendance-rules", "hrm/attendance-rosters", "hrm/attendance-imports", "hrm/attendance-months", "hrm/attendance-audit"].includes(route[0]) || hasFullHRMRead()) && can(route[3], route[5] || "read");
function loginHref() {
  return "/suxinweb/login?redirect=" + encodeURIComponent(location.pathname + location.search + location.hash);
}
function sessionNotice(message = "") {
  const notice = document.getElementById("session-notice");
  if (!notice) return;
  notice.textContent = message;
  notice.hidden = !message;
}
function showSessionIssue(status) {
  imSession.stop();
  state.cleanup?.();
  state.cleanup = null;
  state.epoch++;
  if (status === 401) {
    state.sessionExpired = true;
    state.permissions = [];
  } else state.imSessionDenied = true;
  for (const modal of document.querySelectorAll("dialog")) modal.close();
  if (!document.querySelector(".layout")) return;
  renderWorkspace();
  sessionNotice();
  const expired = status === 401;
  const body = page(expired ? "登录已失效" : "企业即时通讯权限已失效", expired ?
    "登录会话已结束，请重新登录后继续。" : "当前账号已无企业即时通讯访问权限，请联系管理员。");
  body.append(el("section", { class: "surface session-recovery", "data-session-state": expired ? "expired" : "forbidden" },
    el("a", { class: "button primary", href: expired ? loginHref() : "/suxinweb/" }, expired ? "重新登录" : "返回管理后台")));
}
async function sessionDenied(error) {
  if (error.status === 401 || state.sessionExpired) {
    showSessionIssue(401);
    return;
  }
  state.imSessionDenied = true;
  state.cleanup?.();
  state.cleanup = null;
  try {
    const access = await api("/access");
    if (Array.isArray(access.permissions)) state.permissions = access.permissions;
  } catch { /* A revoked plugin grant may also deny the access snapshot. */ }
  if (!state.sessionExpired) {
    if (state.workspace === "im") showSessionIssue(403);
    else renderWorkspace();
  }
}
const imSession = createIMSession({
  renew: (signal) => api("/im/session", undefined, "GET", false, { signal }),
  renewed: (access) => {
    if (!Array.isArray(access?.permissions) || !access.uid) throw new Error("会话校验返回无效内容");
    if (String(access.uid) !== String(state.uid)) {
      showSessionIssue(401);
      return;
    }
    const changed = JSON.stringify(state.permissions) !== JSON.stringify(access.permissions);
    state.permissions = access.permissions;
    sessionNotice();
    if (!can("im")) { showSessionIssue(403); return; }
    if (changed) {
      renderWorkspace();
      const route = routes.find((item) => item[0] === state.route);
      if (route && !routeAllowed(route)) void render();
    }
  },
  failed: () => sessionNotice("暂时无法校验登录状态，正在重试；网络恢复后会自动重新连接。"),
  denied: sessionDenied,
});
window.addEventListener("biz-suite:session-expired", () => showSessionIssue(401));
window.addEventListener("storage", (event) => {
  // React only to the host's logout signal. Never read or reuse its token.
  if ((event.key === "biztoken" || event.key === null) && event.newValue === null)
    showSessionIssue(401);
});
window.addEventListener("focus", () => imSession.resume());
window.addEventListener("online", () => imSession.resume());
document.addEventListener("visibilitychange", () => {
  if (document.visibilityState === "visible") imSession.resume();
});
window.addEventListener("pagehide", () => {
  imSession.stop();
  state.cleanup?.();
  state.cleanup = null;
});
window.addEventListener("pageshow", (event) => {
  if (event.persisted && !state.sessionExpired) void render();
});
function routeLink(label, path, attributes = {}) {
  return el("a", {
    href: BASE + path,
    ...attributes,
    onclick: (event) => {
      if (event.ctrlKey || event.metaKey || event.shiftKey || event.altKey || event.button !== 0) return;
      event.preventDefault();
      navigate(path);
    },
  }, label);
}
const attendanceAdvanced = new Set(["hrm/groups","hrm/holidays","hrm/clocks","hrm/month","hrm/attendance-rosters","hrm/attendance-batches","hrm/attendance-imports","hrm/attendance-ledger","hrm/attendance-overtime","hrm/attendance-audit"]);
function renderWorkspace() {
  const workspace = workspaces[state.workspace];
  state.workspaceTitle = workspace.title;
  document.querySelector(".layout").dataset.workspace = state.workspace;
  const nav = el("nav", { "aria-label": workspace.title + "导航" });
  const advanced = el("details", { class: "attendance-advanced-nav", "data-attendance-advanced": "" }, el("summary", {}, "考勤高级工具")), advancedLinks = el("div"); advanced.append(advancedLinks); advanced.open = attendanceAdvanced.has(state.route);
  let group = "", index = 0;
  for (const route of routes) {
    const [path, title, section] = route;
    if (routeWorkspace(path) !== state.workspace || !routeAllowed(route)) continue;
    const target = attendanceAdvanced.has(path) ? advancedLinks : nav;
    if (target === nav && group !== section) {
      nav.append(el("p", { class: "nav-group" }, section));
      group = section;
    }
    index++;
    target.append(routeLink([
      el("span", { class: "nav-index" }, String(index).padStart(2, "0")), title,
    ], path, {
      class: "nav-link" + (path === state.route ? " active" : ""),
      "data-route": path,
      "aria-current": path === state.route ? "page" : null,
    }));
  }
  if (advancedLinks.children.length) nav.append(advanced);
  if (!index) nav.append(el("p", { class: "workspace-empty-nav" }, "当前账号暂无此工作空间权限"));
  const sidebar = document.querySelector(".sidebar");
  sidebar.classList.remove("open");
  sidebar.replaceChildren(
    routeLink([
      el("img", { class: "brand-mark", src: workspace.mark, alt: workspace.title, width: 40, height: 40 }),
      el("div", {}, el("strong", {}, workspace.title), el("small", {}, "GO SUXINWL")),
    ], workspace.home, { class: "brand", "aria-label": workspace.title + "首页" }),
    nav,
    el("div", { class: "sidebar-note" }, workspace.title, el("br"), workspace.note),
  );
  document.getElementById("workspace-name").textContent = workspace.title;
  const switches = document.getElementById("workspace-switches");
  switches.replaceChildren();
  for (const [key, other] of Object.entries(workspaces))
    if (key !== state.workspace && hasWorkspace(key))
      switches.append(routeLink(other.title, key, { class: "workspace-switch", "data-workspace-link": key }));
}
async function overview() {
  const epoch = state.epoch;
  const body = page(
    "智慧HRM",
    "统一查看员工档案、考勤记录与企业微信同步状态。"
  );
  const cards = el("div", { class: "cards" });
  body.append(cards);
  if (can("hrm")) {
    try {
      const summary = await api("/hrm/summary");
      if (epoch !== state.epoch) return;
      for (const [key, label, path] of [
        ["employees", "员工档案", "hrm/employees"],
        ["clocks", "打卡记录", "hrm/clocks"],
        ["leaves", "请假记录", "hrm/leaves"],
        ["outs", "外出记录", "hrm/outs"],
        ["trips", "出差记录", "hrm/trips"],
      ])
        cards.append(
          el(
            "article",
            { class: "card" },
            el("span", {}, label),
            el("strong", {}, summary[key] ?? 0),
            el("small", {}, "当前业务记录"),
            button("查看详情 →", () => navigate(path), "small")
          )
        );
    } catch (error) {
      if (epoch !== state.epoch) return;
      cards.append(errorNode(error));
    }
  }
  const entry = surface(
    el("h2", {}, "工作入口"),
    el(
      "p",
      { class: "hint" },
      "选择人事、考勤或企业微信功能开始工作。"
    )
  );
  const actions = el("div", { class: "actions" });
  for (const [area, label, path] of [
    ["hrm", "考勤设置", "hrm/attendance-rules"],
    ["hrm", "考勤明细与异常", "hrm/attendance-results"],
    ["hrm", "查看月度考勤", "hrm/attendance-months"],
    ["wecom", "企业微信同步", "wecom/status"],
  ])
    if (can(area)) actions.append(button(label, () => navigate(path)));
  entry.append(el("br"), actions);
  if (!actions.children.length)
    entry.append(
      el(
        "p",
        { class: "notice" },
        "当前账号未获业务权限，请联系管理员授权。"
      )
    );
  body.append(entry);
}
async function render() {
  state.cleanup?.();
  state.cleanup = null;
  state.epoch++;
  const epoch = state.epoch;
  state.route = location.pathname.startsWith(BASE)
    ? location.pathname.slice(BASE.length)
    : "";
  state.route = state.route.replace(/\/+$/, "");
  const aliases = {
    "": workspaces[defaultWorkspace()].home,
    overview: workspaces[defaultWorkspace()].home,
    hrm: "hrm/overview",
    im: "im/chat",
    "im/friends": "im/directory",
    wecom: "wecom/status",
  };
  if (aliases[state.route]) {
    history.replaceState({}, "", BASE + aliases[state.route] + location.search + location.hash);
    state.route = aliases[state.route];
  }
  state.workspace = routeWorkspace(state.route);
  if (state.openimEnabled && state.workspace === "im") {
    location.replace("/suxinweb/communication");
    return;
  }
  const favicon = document.querySelector('link[rel="icon"]');
  favicon?.setAttribute("href", `/resource/static/brand/${state.workspace === "im" ? "im" : "hr"}/favicon-32x32.png?v=puty3`);
  favicon?.setAttribute("type", "image/png");
  if (state.workspace !== "im") {
    imSession.stop();
    sessionNotice();
  }
  renderWorkspace();
  if (state.sessionExpired || (state.workspace === "im" && state.imSessionDenied)) {
    showSessionIssue(state.sessionExpired ? 401 : 403);
    return;
  }
  const route = routes.find((r) => r[0] === state.route);
  document.getElementById("crumb").textContent = route?.[1] || "页面不存在";
  if (!route) {
    imSession.stop();
    page(
      "页面不存在",
      "该地址没有对应页面。",
      button("返回" + state.workspaceTitle, () => navigate(workspaces[state.workspace].home))
    );
    return;
  }
  if (!routeAllowed(route)) {
    if (state.workspace === "im") imSession.stop();
    page("无访问权限", "当前账号没有此业务模块的访问权限，请联系管理员授权。");
    return;
  }
  if (state.workspace === "im") imSession.start();
  try {
    await route[4]();
  } catch (error) {
    if (epoch === state.epoch) document.getElementById("view").append(errorNode(error));
  }
}
async function boot() {
  const root = document.getElementById("app");
  try {
    const access = await api("/access");
    state.permissions = Array.isArray(access.permissions)
      ? access.permissions
      : [];
    state.uid = access.uid || access.user_id || 0;
    if (can("im")) state.openimEnabled = !!(await api("/im/openim/status")).enabled;
    const sidebar = el("aside", { class: "sidebar" });
    root.replaceChildren(
      el(
        "div",
        { class: "layout" },
        sidebar,
        el(
          "main",
          { class: "workspace" },
          el(
            "header",
            { class: "topbar" },
            el(
              "div",
              { class: "breadcrumb" },
              el("span", { id: "workspace-name" }), " / ",
              el("strong", { id: "crumb" }, "工作总览")
            ),
            el(
              "div",
              { class: "top-actions" },
              el("div", { id: "workspace-switches", "aria-label": "切换工作空间" }),
              el("a", { href: "/suxinweb/", class: "workspace-return" }, "管理后台"),
              el("span", { class: "badge" }, `宿主账号 #${state.uid}`),
              button(
                "菜单",
                () => sidebar.classList.toggle("open"),
                "mobile-nav"
              ),
              button("刷新页面", render)
            )
          ),
          el("div", { id: "session-notice", class: "session-notice", role: "status", "aria-live": "polite", hidden: true }),
          el("div", { class: "content", id: "view" })
        )
      )
    );
    root.removeAttribute("aria-busy");
    window.removeEventListener("popstate", render);
    window.addEventListener("popstate", render);
    await render();
  } catch (error) {
    root.replaceChildren(
      el(
        "main",
        { class: "content" },
        surface(
          el("h1", {}, "无法加载工作空间"),
          errorNode(error),
          state.sessionExpired ? el("a", { class: "button primary", href: loginHref(), "data-session-state": "expired" }, "重新登录") : button("重试", boot, "primary"),
          el(
            "p",
            { class: "hint" },
            "请从 GoSuxinwl 宿主后台打开插件，使用有效登录会话。"
          )
        )
      )
    );
    root.removeAttribute("aria-busy");
  }
}
boot();
