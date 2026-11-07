export const BASE = "/plugins/biz-suite/admin/";
export const API = "/plugins/biz-suite/api/admin";
export const state = {
  permissions: [],
  uid: 0,
  route: "",
  workspace: "",
  workspaceTitle: "业务工作空间",
  sessionExpired: false,
  imSessionDenied: false,
  epoch: 0,
  cleanup: null,
};
export const el = (tag, attrs = {}, ...children) => {
  const node = document.createElement(tag);
  for (const [key, value] of Object.entries(attrs)) {
    if (key.startsWith("on") && typeof value === "function")
      node.addEventListener(key.slice(2).toLowerCase(), value);
    else if (key === "class") node.className = value;
    else if (key === "text") node.textContent = value ?? "";
    else if (value != null && value !== false)
      node.setAttribute(key, value === true ? "" : value);
  }
  for (const child of children.flat(Infinity))
    if (child != null && child !== false)
      node.append(
        child instanceof Node ? child : document.createTextNode(String(child))
      );
  return node;
};
export const can = (area, action = "read") => {
  const requested = `biz-suite.${area}.${action}`;
  return state.permissions.some(
    (p) =>
      p === "*" ||
      p === "biz-suite.*" ||
      p === `biz-suite.${area}.*` ||
      p === requested ||
      (area === "hrm" && action === "read" && ["biz-suite.hrm.self", "biz-suite.hrm.department", "biz-suite.hrm.write"].includes(p)) ||
      p === `biz-suite.${area}.manage`
  );
};
function requestError(path, status, message, code) {
  // Some host middleware returns an auth error inside an HTTP 200 envelope.
  if (status >= 200 && status < 300 && [401, 403].includes(Number(code))) status = Number(code);
  const error = Object.assign(new Error(message), { status, code, path });
  if (status === 401)
    window.dispatchEvent(new CustomEvent("biz-suite:session-expired", { detail: error }));
  return error;
}
export async function api(path, body, method, withMeta = false, options = {}) {
  const response = await fetch(API + path, {
    method: method || (body === undefined ? "GET" : "POST"),
    credentials: "include",
    cache: "no-store",
    signal: options.signal,
    headers: {
      "X-Requested-With": "XMLHttpRequest",
      "Content-Type": "application/json",
    },
    ...(body !== undefined ? { body: JSON.stringify(body) } : {}),
  });
  let data;
  try {
    data = await response.json();
  } catch {
    throw requestError(path, response.status,
      response.status === 401
        ? "登录已失效，请从宿主后台重新打开插件。"
        : response.status === 403
        ? "当前账号没有此操作权限，请联系管理员授权。"
        : `接口返回无效内容 (${response.status})`
    );
  }
  if (!response.ok || (data.code != null && Number(data.code) !== 0))
    throw requestError(path, response.status,
      data.message || data.msg || `请求失败 (${response.status})`, data.code);
  const result = data.data !== undefined ? data.data : data;
  return withMeta
    ? {
        data: result,
        total: response.headers.has("X-Total-Count")
          ? Number(response.headers.get("X-Total-Count"))
          : asRows(result).length,
      }
    : result;
}
export const apiPage = (path) => api(path, undefined, undefined, true);
export function toast(text) {
  const node = document.getElementById("toast");
  node.textContent = text;
  node.classList.add("show");
  clearTimeout(toast.timer);
  toast.timer = setTimeout(() => node.classList.remove("show"), 4000);
}
export function errorNode(error) {
  return el(
    "div",
    { class: "error-text", role: "alert" },
    error.message || String(error)
  );
}
export const button = (text, fn, style = "", attrs = {}) =>
  el(
    "button",
    { type: "button", class: `button ${style}`, onclick: fn, ...attrs },
    text
  );
export const safe =
  (fn) =>
  async (...args) => {
    try {
      await fn(...args);
    } catch (error) {
      toast(error.message || String(error));
    }
  };
export const asRows = (value) =>
  Array.isArray(value)
    ? value
    : value?.items || value?.list || value?.rows || [];
export const parseJSON = (value, fallback = {}) => {
  try {
    return typeof value === "string" ? JSON.parse(value) : value || fallback;
  } catch {
    return fallback;
  }
};
export function safeURL(value) {
  try {
    const url = new URL(value, location.origin);
    return ["http:", "https:"].includes(url.protocol) ? url.href : null;
  } catch {
    return null;
  }
}
export function timestamp(value) {
  return (value || "").replace("T", " ").slice(0, 19);
}
export function localDateTime(date = new Date()) {
  const sh = new Date(date.getTime() + 8 * 3600000);
  return sh.toISOString().slice(0, 16);
}
export const statusLabels = {
  active: "在职 / 启用",
  inactive: "停用",
  quit: "离职",
  pending: "待审批",
  approved: "已通过",
  rejected: "已驳回",
  canceled: "已撤销",
  normal: "正常",
  success: "成功",
  failed: "失败",
  running: "运行中",
  partial: "部分失败",
  manual: "手工",
  wecom: "企业微信",
  sent: "已发送",
  read: "已读",
};
export const status = (value) =>
  el(
    "span",
    {
      class: `badge ${
        ["failed", "rejected"].includes(value)
          ? "error"
          : ["pending", "running", "partial"].includes(value)
          ? "warn"
          : ""
      }`,
    },
    statusLabels[value] || value || "—"
  );
export function page(title, description, actions) {
  const body = document.getElementById("view");
  body.replaceChildren();
  body.append(
    el(
      "header",
      { class: "page-head" },
      el(
        "div",
        {},
        el("p", { class: "eyebrow" }, state.workspaceTitle),
        el("h1", {}, title),
        el("p", { class: "description" }, description)
      ),
      actions
    )
  );
  document.title = `${title} · ${state.workspaceTitle}`;
  return body;
}
export const surface = (...children) =>
  el("section", { class: "surface" }, ...children);
export function details(value) {
  const dl = el("dl", { class: "details" });
  for (const [k, v] of Object.entries(value))
    dl.append(
      el("dt", {}, k),
      el("dd", {}, typeof v === "object" ? JSON.stringify(v) : String(v ?? "—"))
    );
  return dl;
}
export function dialog(title, content) {
  const modal = el("dialog");
  const close = () => {
    modal.close();
    modal.remove();
  };
  modal.append(
    el(
      "div",
      { class: "dialog-body" },
      el(
        "header",
        { class: "dialog-head" },
        el("h2", {}, title),
        button("×", close, "close", { "aria-label": "关闭" })
      ),
      content
    )
  );
  modal.addEventListener("close", () => modal.remove());
  document.body.append(modal);
  modal.showModal();
  return { modal, close };
}
export function inspect(row, title = "记录详情") {
  return dialog(
    title,
    el("pre", { class: "json" }, JSON.stringify(row, null, 2))
  );
}
export function field(def, value) {
  const input = def.options
    ? el(
        "select",
        { name: def.key, required: def.required },
        def.options.map((option) => {
          const [id, label] = Array.isArray(option) ? option : [option, option];
          return el("option", { value: id }, label);
        })
      )
    : el(def.type === "textarea" ? "textarea" : "input", {
        name: def.key,
        type: def.type || "text",
        required: def.required,
        placeholder: def.placeholder,
        min: def.min,
        max: def.max,
        step: def.step,
        readonly: def.readonly,
      });
  input.value =
    value ??
    def.value ??
    (def.options?.[0]
      ? Array.isArray(def.options[0])
        ? def.options[0][0]
        : def.options[0]
      : "");
  if (def.type === "datetime-local" && input.value === "")
    input.value = (value || "").replace(" ", "T").slice(0, 16);
  const wrapper = el(
    "label",
    { class: `field ${def.wide ? "wide" : ""}` },
    el("span", {}, def.label, def.required ? el("em", {}, " *") : null),
    input,
    def.hint ? el("small", { class: "hint" }, def.hint) : null
  );
  return { wrapper, input };
}
export function form(fields, values, submit, label = "保存") {
  const formNode = el("form"),
    grid = el("div", { class: "form-grid" }),
    errors = el("div");
  const inputs = {};
  for (const def of fields) {
    const f = field(def, values?.[def.key]);
    inputs[def.key] = f.input;
    grid.append(f.wrapper);
  }
  const save = el("button", { type: "submit", class: "button primary" }, label);
  formNode.append(errors, grid, el("footer", { class: "form-actions" }, save));
  formNode.addEventListener("submit", async (event) => {
    event.preventDefault();
    if (!formNode.reportValidity()) return;
    errors.replaceChildren();
    save.disabled = true;
    try {
      const data = {};
      for (const def of fields) {
        const raw = inputs[def.key].value;
        data[def.key] =
          def.type === "number"
            ? Number(raw)
            : def.type === "datetime-local"
            ? raw
              ? timestamp(raw) + ":00"
              : ""
            : raw;
      }
      await submit(data);
    } catch (error) {
      errors.append(errorNode(error));
    } finally {
      save.disabled = false;
    }
  });
  return formNode;
}
export function edit(title, fields, values, save, after) {
  let view;
  const editor = form(fields, values, async (data) => {
    await save(data);
    view.close();
    toast("已保存");
    await after?.();
  });
  view = dialog(title, editor);
  return view;
}
export function dataTable(
  rows,
  columns,
  rowActions = [],
  { search = true, empty = "暂无记录", pageSize = 20, showPager = true } = {}
) {
  rows = asRows(rows);
  const container = el("div");
  const host = el("div", { class: "table-wrap" }),
    pager = el("div", { class: "pager" });
  let term = "",
    current = 1;
  const count = el("span", { class: "count" });
  const input = el("input", {
    type: "search",
    placeholder: "搜索当前已加载记录…",
    "aria-label": "搜索记录",
    class: "search",
    oninput: (e) => {
      term = e.target.value.toLowerCase();
      current = 1;
      render();
    },
  });
  if (search) container.append(el("div", { class: "toolbar" }, input, count));
  container.append(host);
  if (showPager) container.append(pager);
  function render() {
    const filtered = rows.filter((row) =>
      Object.values(row).some((v) =>
        String(v ?? "")
          .toLowerCase()
          .includes(term)
      )
    );
    const pages = Math.max(1, Math.ceil(filtered.length / pageSize));
    current = Math.min(current, pages);
    count.textContent = `${filtered.length} 条记录`;
    host.replaceChildren();
    pager.replaceChildren();
    if (!filtered.length) {
      host.append(el("div", { class: "empty" }, empty));
      return;
    }
    const table = el("table"),
      head = el("tr");
    for (const col of columns) head.append(el("th", {}, col[1]));
    if (rowActions.length) head.append(el("th", {}, "操作"));
    table.append(el("thead", {}, head));
    const body = el("tbody");
    for (const row of filtered.slice(
      (current - 1) * pageSize,
      current * pageSize
    )) {
      const tr = el("tr");
      for (const [key, , renderCell] of columns) {
        const value = renderCell ? renderCell(row[key], row) : row[key];
        tr.append(
          el(
            "td",
            { title: typeof value === "string" ? value : null },
            value instanceof Node ? value : String(value ?? "—")
          )
        );
      }
      if (rowActions.length) {
        const actions = el("div", { class: "actions" });
        for (const action of rowActions)
          if (!action.when || action.when(row))
            actions.append(
              button(
                action.label,
                safe(() => action.run(row)),
                "small " + (action.style || "")
              )
            );
        tr.append(el("td", {}, actions));
      }
      body.append(tr);
    }
    table.append(body);
    host.append(table);
    pager.append(
      button(
        "上一页",
        () => {
          current--;
          render();
        },
        "small",
        { disabled: current <= 1 }
      ),
      el("span", {}, `${current} / ${pages}`),
      button(
        "下一页",
        () => {
          current++;
          render();
        },
        "small",
        { disabled: current >= pages }
      )
    );
  }
  render();
  return container;
}
export async function resource(config) {
  const body = page(
    config.title,
    config.description || "支持搜索、分页查看和记录详情。"
  );
  const panel = surface(el("div", { class: "loading" }, "正在加载…"));
  body.append(panel);
  const epoch = state.epoch;
  const server = config.area === "hrm";
  const current = config.page || 1;
  const size = 50;
  const query = new URLSearchParams({
    page: current,
    page_size: size,
    search: config.search || "",
  });
  for (const [key, value] of Object.entries(config.query?.(config) || {}))
    if (value !== undefined && value !== null && value !== "")
      query.set(key, String(value));
  try {
    const [response, extra] = await Promise.all([
      server ? apiPage(config.path + "?" + query) : api(config.path),
      config.load?.(),
    ]);
    if (epoch !== state.epoch || !panel.isConnected) return;
    const rows = asRows(server ? response.data : response);
    const fields =
      typeof config.fields === "function"
        ? config.fields(extra)
        : config.fields;
    const save = (row) =>
      edit(
        `${row?.id ? "编辑" : "新增"}${config.itemName || config.title}`,
        fields,
        config.prepare ? config.prepare(row || {}) : row,
        async (data) => {
          const payload = config.transform ? config.transform(data, row) : data;
          await api(config.path, { ...row, ...payload });
        },
        () => resource(config)
      );
    const actions = [...(config.actions || [])];
    if (fields && can(config.area, config.writeAction || "write"))
      actions.push({
        label: "编辑",
        when: config.editable || (() => true),
        run: save,
      });
    actions.push({ label: "详情", run: (row) => config.detail ? config.detail(row) : inspect(row) });
    panel.replaceChildren(
      el(
        "div",
        { class: "subheading" },
        el("h2", {}, config.title),
        el(
          "div",
          { class: "actions" },
          button("刷新", () => resource(config)),
          (fields || config.create) &&
            can(config.area, config.writeAction || "write")
            ? button(
                `+ 新增${config.itemName || ""}`,
                () => (config.create ? config.create() : save()),
                "primary"
              )
            : null
        )
      )
    );
    if (config.notice)
      panel.append(el("p", { class: "notice" }, config.notice));
    if (config.tools)
      panel.append(config.tools({ extra, refresh: () => resource(config) }));
    if (server && config.renderFilters) {
      panel.append(config.renderFilters({
        extra,
        total: response.total,
        filters: config.filters || {},
        search: config.search || "",
        apply: (updates) => resource({ ...config, ...updates, page: 1 }),
      }));
    } else if (server) {
      const input = el("input", {
        class: "search",
        type: "search",
        value: config.search || "",
        placeholder: "搜索全部记录…",
        "aria-label": "搜索记录",
      });
      const search = () =>
        resource({ ...config, page: 1, search: input.value });
      input.addEventListener("keydown", (e) => {
        if (e.key === "Enter") search();
      });
      panel.append(
        el(
          "div",
          { class: "toolbar" },
          input,
          button("搜索", search),
          el("span", { class: "count" }, `共 ${response.total} 条`)
        )
      );
    }
    panel.append(
      dataTable(rows, typeof config.columns === "function" ? config.columns(extra) : config.columns, actions, {
        search: !server,
        pageSize: server ? size : 20,
        showPager: !server,
      })
    );
    if (server)
      panel.append(
        el(
          "div",
          { class: "pager" },
          button(
            "上一页",
            () => resource({ ...config, page: current - 1 }),
            "small",
            { disabled: current <= 1 }
          ),
          el(
            "span",
            {},
            `${current} / ${Math.max(1, Math.ceil(response.total / size))}`
          ),
          button(
            "下一页",
            () => resource({ ...config, page: current + 1 }),
            "small",
            { disabled: current * size >= response.total }
          )
        )
      );
  } catch (error) {
    if (epoch === state.epoch)
      panel.replaceChildren(
        errorNode(error),
        button("重试", () => resource(config))
      );
  }
}
export function navigate(path) {
  if (BASE + path === location.pathname + location.search) return;
  history.pushState({}, "", BASE + path);
  window.dispatchEvent(new PopStateEvent("popstate"));
}
export function link(text, path) {
  return el(
    "a",
    {
      href: BASE + path,
      onclick: (event) => {
        if (event.ctrlKey || event.metaKey) return;
        event.preventDefault();
        navigate(path);
      },
    },
    text
  );
}
