import {
  el,
  api,
  asRows,
  can,
  page,
  surface,
  button,
  dataTable,
  status,
  errorNode,
  toast,
  state,
  safe,
  field,
  link,
} from "./core.js";

const labels = {
  contact: "通讯录",
  organization: "组织与账号映射",
  checkin: "打卡",
  leave: "请假",
  out: "外出",
  trip: "出差",
  overtime: "加班",
  rule_snapshot: "旧规则快照（已停用）",
  approval: "审批（旧批次）",
  unmapped: "未映射审批校验",
};
const splitIDs = (value) => [
  ...new Set(
    value
      .split(/[\s,，;；]+/)
      .map((v) => v.trim())
      .filter(Boolean)
  ),
];

const diagnosticLabels = {
  contact: "检查通讯录连接",
  app: "检查应用身份",
  checkin: "检查打卡权限",
  approval: "检查审批权限",
};
const diagnosticStates = {
  success: "成功",
  limited: "部分可用",
  failed: "失败",
  unverified: "未验证",
  missing: "未验证 · 缺少配置",
  skipped: "未验证 · 已跳过",
};

function diagnosticBadge(value) {
  const key = Object.hasOwn(diagnosticStates, value) ? value : "unverified";
  return el(
    "span",
    { class: `badge ${key === "failed" ? "error" : key === "success" ? "" : "warn"}` },
    diagnosticStates[key]
  );
}

function diagnosticDetails(value = {}) {
  const names = {
    department_count: "返回部门数",
    department_name_available: "部门名称可读",
    sample_user_count: "抽样成员数",
    name_available: "抽样姓名可读",
    mobile_available: "抽样手机号可读",
    email_available: "抽样邮箱可读",
    agent_id: "应用 ID",
    app_name: "应用名称",
    app_enabled: "应用已启用",
    visible_user_count: "可见范围指定人员数",
    visible_department_ids: "可见部门 ID",
    visible_tag_count: "可见标签数",
    sample_count: "返回审批样本数",
  };
  const rows = Object.entries(names).filter(([key]) => value[key] != null);
  if (!rows.length) return null;
  return el("dl", { class: "diagnostic-details" }, rows.flatMap(([key, label]) => [
    el("dt", {}, label),
    el("dd", {}, typeof value[key] === "boolean" ? value[key] ? "是" : "否" : Array.isArray(value[key]) ? value[key].join("、") || "无" : String(value[key])),
  ]));
}

function diagnosticResult(result) {
  if (!result)
    return el("p", { class: "hint" }, "尚未检查。已配置凭据不代表连接成功或已获得接口权限。");
  return el(
    "div",
    { class: "diagnostic-result" },
    el("div", { class: "diagnostic-summary" }, diagnosticBadge(result.status)),
    result.checked_at ? el("p", { class: "hint" }, `检查时间：${result.checked_at}；结果仅反映本次请求。`) : null,
    asRows(result.checks).map((check) =>
      el(
        "div",
        { class: "diagnostic-check" },
        el("div", {}, diagnosticBadge(check.status), el("strong", {}, check.name || check.label || check.endpoint || "检查项")),
        check.message ? el("p", {}, check.message) : null,
        check.code != null ? el("small", { class: "hint" }, `企业微信返回码：${check.code}`) : null,
        diagnosticDetails(check.details)
      )
    ),
  );
}

function automaticSchedule(schedule) {
  const title = el("h2", {}, "自动同步计划");
  if (!schedule || !Array.isArray(schedule.times) || !schedule.timezone)
    return [title, el("p", { class: "hint" }, "暂未取得服务器自动同步计划，请刷新后查看。手动同步入口保留。")];
  const timezone = schedule.timezone === "Asia/Shanghai" ? "北京时间（Asia/Shanghai，UTC+8）" : schedule.timezone;
  return [
    title,
    el("dl", { class: "details" },
      el("dt", {}, "时区"), el("dd", { "data-schedule-timezone": "" }, timezone),
      el("dt", {}, "每日自动同步时点"), el("dd", { "data-schedule-times": "" }, schedule.times.join("、")),
      el("dt", {}, "下次计划同步"), el("dd", { "data-schedule-next": "" }, schedule.next_run_at || "暂未取得下次执行时间")
    ),
    el("p", { class: "notice" }, "仅同步通讯录、原始打卡及审批数据，考勤正常与否由本地制度判定。自动同步按服务器当前计划执行，无需另行保存设置。同步批次可能持续一段时间，完成情况以同步批次记录为准。"),
    el("p", { class: "hint" },
      schedule.startup_sync === false ? "服务启动不触发自动同步。" : schedule.startup_sync === true ? "服务启动会触发自动同步。" : "启动同步策略尚未取得。",
      schedule.catch_up === false ? "错过的同步时点不补跑，等待下一个计划时点。" : schedule.catch_up === true ? "错过的同步时点会补跑。" : "错过时点的处理策略尚未取得。",
      "需要立即更新时仍可使用下方手动同步；已有任务运行时请等待其完成。"
    ),
  ];
}

function namedFilterPicker(kind, options, settings, managed) {
  const department = kind === "department";
  const title = department ? "部门" : "人员";
  const nameLabel = department ? "部门名称" : "姓名";
  const nameKeys = [`include_${kind}_names`, `exclude_${kind}_names`];
  const idKeys = department
    ? ["include_department_ids", "exclude_department_ids"]
    : ["include_userids", "exclude_userids"];
  const splitNames = (value) => [...new Set(
    value.split(/[,，;；\r\n]+/).map((v) => v.trim()).filter(Boolean)
  )];
  const names = nameKeys.map((key, i) => field({
    key,
    label: `${i ? "排除" : "仅同步"}这些${nameLabel}`,
    type: "textarea",
    placeholder: department ? "例如：分子公司\n新入职员工" : "例如：段详政",
    hint: `填写完整${nameLabel}，一行一个或用逗号分隔；按名称精确匹配。`,
  }, (settings[key] || []).join("\n")));
  const ids = idKeys.map((key, i) => field({
    key,
    label: `${i ? "排除" : "仅同步"}的${department ? "部门 ID" : "人员账号"}（兼容已有配置）`,
    type: "textarea",
    hint: "保留已有 ID 名单；通常无需编辑，可填写上方名称或从候选中选择。",
  }, (settings[key] || []).join("\n")));
  for (const item of [...names, ...ids]) {
    item.input.rows = 3;
    item.input.disabled = !managed;
  }
  const byID = new Map(options.map((item) => [String(item.id), item]));
  const duplicateNames = new Map();
  for (const item of options)
    duplicateNames.set(item.name, (duplicateNames.get(item.name) || 0) + 1);
  const optionLabel = (item) => [
    item.name || `${nameLabel}待解析`,
    item.detail || (department ? "上级部门待解析" : "所属部门待解析"),
    !item.name || duplicateNames.get(item.name) > 1
      ? `${department ? "部门" : "账号"}：${item.id}` : "",
  ].filter(Boolean).join(" · ");
  const selectedLists = ids.map(() => el("div", { class: "filter-selections" }));
  const writeIDs = (index, values) => {
    if (!managed) return;
    ids[index].input.value = [...new Set(values)].join("\n");
    ids[index].input.dispatchEvent(new Event("input", { bubbles: true }));
  };
  const renderSelections = () => {
    for (const [index, host] of selectedLists.entries()) {
      const values = splitIDs(ids[index].input.value);
      host.replaceChildren();
      if (!values.length) continue;
      host.append(el("strong", {}, `${index ? "排除" : "仅同步"}的具体${title}`));
      for (const id of values) {
        const known = byID.get(id);
        host.append(el("div", {
          class: "filter-selection",
          "data-bound-id": id,
          "data-bound-list": idKeys[index],
        },
          el("div", {},
            el("strong", {}, known?.name || `${nameLabel}待解析`),
            el("small", {}, known?.detail || "尚未取得对应通讯录资料"),
            el("small", { class: "hint" }, `${department ? "部门 ID" : "账号"}：${id}`)
          ),
          managed ? button("移除", () => writeIDs(index, values.filter((value) => value !== id)),
            "small", { "aria-label": `移除${index ? "排除" : "仅同步"}的${known?.name || id}` }) : null
        ));
      }
    }
  };
  for (const item of ids) item.input.addEventListener("input", renderSelections);
  renderSelections();
  const search = el("input", {
    type: "search",
    placeholder: department ? "搜索部门名称" : "搜索姓名或所属部门",
    "aria-label": `搜索可选${title}`,
  });
  const select = el("select", {
    multiple: true, size: 5, "aria-label": `可选${title}`,
    disabled: !managed || !options.length,
  });
  const renderOptions = () => {
    const selected = new Set([...select.selectedOptions].map((item) => item.value));
    const term = search.value.trim().toLocaleLowerCase();
    select.replaceChildren(...options
      .filter((item) => `${item.name} ${item.detail} ${item.id}`.toLocaleLowerCase().includes(term))
      .map((item) => el("option", {
        value: item.id, selected: selected.has(String(item.id)),
      }, optionLabel(item))));
  };
  search.addEventListener("input", renderOptions);
  renderOptions();
  const addSelected = (index) => {
    if (!select.selectedOptions.length) return;
    writeIDs(index, [
      ...splitIDs(ids[index].input.value),
      ...[...select.selectedOptions].map((item) => item.value),
    ]);
  };
  return {
    node: el("section", { "data-name-filter": kind },
      el("div", { class: "form-grid" }, ...names.map((item) => item.wrapper)),
      el("p", { class: "notice" },
        `同名${title}都会按名称规则处理。仅同步名称与已选具体${title}任一匹配即包含，两者均为空时不限制；任一排除规则命中均优先排除。${department ? "子部门范围由下方开关控制。" : "若只想选择同名中的一人，请从候选中按所属部门选择具体人员。"}`),
      ...selectedLists,
      el("details", { class: "filter-picker" },
        el("summary", {}, `选择具体${title}（${options.length} 项）`),
        options.length ? [
          search, select,
          el("div", { class: "actions" },
            button(`仅同步选中${title}`, () => addSelected(0), "small",
              { disabled: !managed, "data-filter-action": `${kind}-include` }),
            button(`排除选中${title}`, () => addSelected(1), "small",
              { disabled: !managed, "data-filter-action": `${kind}-exclude` })
          ),
        ] : el("p", { class: "hint" },
          `暂未取得${title}资料，也可以先填写中文${nameLabel}规则，通讯录同步后按规则执行；无需先填写 ID。`)
      ),
      el("details", { class: "filter-advanced", "data-id-filter": kind },
        el("summary", {}, "高级：按 ID 管理已有名单"),
        el("p", { class: "hint" }, "无法解析名称的旧 ID 仍保留并显示为待解析，不会自动删除。"),
        el("div", { class: "form-grid" }, ...ids.map((item) => item.wrapper))
      )
    ),
    read: () => Object.fromEntries([
      ...nameKeys.map((key, i) => [key, splitNames(names[i].input.value)]),
      ...idKeys.map((key, i) => [key, splitIDs(ids[i].input.value).map((id) => {
        if (!department) return id;
        if (!/^[1-9]\d*$/.test(id) || !Number.isSafeInteger(Number(id)))
          throw new Error("部门 ID 必须为正整数");
        return Number(id);
      })]),
    ]),
    reset: (saved) => {
      nameKeys.forEach((key, i) => { names[i].input.value = (saved[key] || []).join("\n"); });
      idKeys.forEach((key, i) => { ids[i].input.value = (saved[key] || []).join("\n"); });
      renderSelections();
    },
  };
}

export async function dashboard() {
  const body = page(
    "企业微信同步",
    "分别配置通讯录读取和自建应用凭据，检查权限后按业务同步。"
  );
  const configPanel = surface(el("div", { class: "loading" }, "正在读取配置…"));
  const schedulePanel = surface(el("h2", {}, "自动同步计划"), el("p", { class: "hint" }, "正在读取服务器计划…"));
  schedulePanel.setAttribute("data-automatic-schedule", "");
  const syncPanel = surface(),
    runs = surface();
  body.append(schedulePanel, configPanel, syncPanel, runs);
  let stopped = false,
    timer,
    dirty = false,
    submitting = false,
    saving = false,
    checking = "",
    revision = 0,
    saveButton,
    config = {},
    templates = [];
  state.cleanup = () => {
    stopped = true;
    clearTimeout(timer);
  };
  const buttons = [],
    probeButtons = [],
    probePanels = new Map(),
    credentialViews = new Map(),
    configStatus = el("div", { class: "notice" }),
    syncStatus = el("p", { class: "hint", "aria-live": "polite" });
  const credential = (kind) => config.credentials?.[kind] || { configured: false, source: "missing" };
  const invalidateDiagnostics = (message = "配置已修改，请保存后重新检查。") => {
    revision++;
    for (const panel of probePanels.values()) {
      panel.replaceChildren(diagnosticBadge("unverified"), el("p", { class: "hint" }, message));
    }
  };
  const updateButtons = () => {
    if (saveButton)
      saveButton.disabled = !can("wecom", "manage") || submitting || saving || Boolean(checking) || config.running;
    for (const [node, kind] of buttons) {
      const scope = ["leave", "out", "trip", "overtime"].includes(kind) ? "approval" : kind;
      const configured = config.corp_id && config.scopes?.[scope];
      const mapped =
        scope !== "approval" || templates.some((t) => t.approval_type === kind);
      node.disabled =
        submitting || saving || checking || dirty || config.running || !configured || !mapped;
      node.title = dirty
        ? "请先保存配置"
        : config.running
        ? "已有同步任务运行中"
        : !configured
        ? "请先配置企业 ID 和对应 Secret"
        : !mapped
        ? "请先配置对应业务的审批模板"
        : "";
    }
    for (const [node, kind] of probeButtons) {
      node.disabled = submitting || saving || Boolean(checking) || dirty || config.running;
      node.title = dirty ? "请先保存配置" : config.running ? "已有同步任务运行中" : "";
      node.textContent = checking === kind ? "正在检查…" : diagnosticLabels[kind];
    }
    for (const [kind, view] of credentialViews) {
      const current = credential(kind);
      const source = { environment: "服务器环境", legacy_environment: "旧环境变量", saved: "加密保存", missing: "未配置" }[current.source] || "服务器配置";
      view.replaceChildren(
        el("strong", {}, `Secret：${current.configured ? "已配置" : "未配置"}`),
        el("span", { class: "hint" }, `来源：${source}${current.environment_variable ? `（${current.environment_variable}）` : ""}`)
      );
    }
    syncStatus.textContent = dirty
      ? "配置尚未保存，请先保存后启动同步。"
      : config.running
      ? "后台同步运行中，完成后可启动下一任务。"
      : "按已保存配置执行。时间留空使用各业务独立的增量进度；通讯录每次重新读取当前可见范围。";
  };
  function probe(kind) {
    const panel = el("div", { class: "diagnostic-output", "data-diagnostic-result": kind, "aria-live": "polite" }, diagnosticBadge("unverified"), diagnosticResult());
    probePanels.set(kind, panel);
    const run = button(diagnosticLabels[kind], async () => {
      if (dirty || checking || saving || submitting || config.running) return;
      const startedRevision = revision;
      checking = kind;
      updateButtons();
      panel.replaceChildren(el("p", { class: "hint" }, "正在验证已保存的配置…"));
      try {
        const result = await api(`/wecom/diagnostics/${kind}`, {});
        if (!stopped && startedRevision === revision)
          panel.replaceChildren(diagnosticResult(result));
      } catch (error) {
        if (!stopped && startedRevision === revision)
          panel.replaceChildren(diagnosticBadge("failed"), errorNode(error));
      } finally {
        checking = "";
        if (!stopped) updateButtons();
      }
    }, "", { "data-diagnostic-kind": kind });
    probeButtons.push([run, kind]);
    return el("section", { class: "diagnostic-block" }, can("wecom", "manage") ? run : el("h4", {}, diagnosticLabels[kind]), panel);
  }
  async function refresh() {
    clearTimeout(timer);
    if (stopped) return;
    try {
      const [next, batches, mapped] = await Promise.all([
        api("/wecom/status"),
        api("/wecom/sync/runs"),
        api("/wecom/templates"),
      ]);
      if (stopped) return;
      const identity = (value) => JSON.stringify([value.corp_id, value.agent_id, value.credentials]);
      if (config.corp_id !== undefined && identity(config) !== identity(next))
        invalidateDiagnostics("服务器上的配置已更新，请重新检查连接与接口权限。");
      config = next;
      templates = asRows(mapped);
      schedulePanel.replaceChildren(...automaticSchedule(config.automatic_schedule));
      configStatus.replaceChildren(
        el(
          "strong",
          {},
          `当前企业：${config.corp_id || "未设置"} · 应用：${
            config.agent_id || "未设置"
          }`
        ),
        el("p", {}, "凭据配置与接口权限分别检查。应用身份验证成功，不代表已获得打卡或审批权限。"),
        el("small", {}, "Secret 可在网页填写并加密保存，或由服务器环境提供；已保存的 Secret 不会返回浏览器。")
      );
      updateButtons();
      runs.replaceChildren(
        el(
          "div",
          { class: "subheading" },
          el("h2", {}, "同步批次"),
          button("刷新", safe(refresh))
        ),
        dataTable(batches, [
          ["id", "批次"],
          ["sync_type", "类型", (v) => labels[v] || v],
          ["status", "状态", status],
          ["success_count", "成功"],
          ["failure_count", "失败"],
          ["started_at", "开始"],
          ["finished_at", "结束"],
          ["error_message", "错误"],
        ])
      );
      timer = setTimeout(refresh, config.running ? 3000 : 15000);
    } catch (error) {
      if (!stopped)
        runs.replaceChildren(errorNode(error), button("重试", safe(refresh)));
    }
  }
  try {
    const [settings, choices] = await Promise.all([
      api("/wecom/settings"),
      api("/wecom/filter-options"),
    ]);
    if (stopped) return;
    const managed = can("wecom", "manage");
    const corp = field(
      {
        key: "corp_id",
        label: "企业 ID（CorpID）",
        placeholder: "例如 ww…",
        hint: "企业微信管理后台 → 我的企业；留空回退到服务器环境配置。",
      },
      settings.corp_id
    );
    const agent = field(
      {
        key: "agent_id",
        label: "自建应用 ID（AgentID）",
        placeholder: "例如 1000002",
        hint: "填写下方自建应用 Secret 对应的 AgentID；留空使用服务器环境配置。",
      },
      settings.agent_id
    );
    corp.input.disabled = agent.input.disabled = !managed;
    const directory = asRows(choices.departments);
    const departmentMap = new Map(directory.map((item) => [String(item.wecom_id), item]));
    const departmentPath = (item) => {
      const parts = [], seen = new Set([String(item.wecom_id)]);
      let parent = item.parent_id;
      while (parent && !seen.has(String(parent))) {
        seen.add(String(parent));
        const node = departmentMap.get(String(parent));
        if (!node) break;
        if (node.name) parts.unshift(node.name);
        parent = node.parent_id;
      }
      return parts.join(" / ") || (item.parent_id ? "" : "根部门");
    };
    const departmentOptions = directory.map((item) => ({
      id: item.wecom_id,
      name: item.name || "",
      detail: departmentPath(item),
    }));
    const userOptions = asRows(choices.users).map((item) => {
      let ids = item.department_ids;
      if (typeof ids === "string") {
        try { ids = JSON.parse(ids); } catch { ids = []; }
      }
      const names = Array.isArray(item.department_names) && item.department_names.length
        ? item.department_names
        : (Array.isArray(ids) ? ids : []).map((id) => departmentMap.get(String(id))?.name).filter(Boolean);
      return { id: item.wecom_userid, name: item.name || "", detail: names.join("、") };
    });
    const byName = (a, b) => a.name.localeCompare(b.name, "zh-CN") || a.detail.localeCompare(b.detail, "zh-CN") || String(a.id).localeCompare(String(b.id));
    const departments = namedFilterPicker("department", departmentOptions.sort(byName), settings, managed);
    const users = namedFilterPicker("user", userOptions.sort(byName), settings, managed);
    const children = el("input", {
      type: "checkbox",
      name: "include_subdepartments",
      checked: settings.include_subdepartments,
      disabled: !managed,
    });
    const save = el(
      "button",
      { type: "submit", class: "button primary", disabled: !managed },
      "保存配置"
    );
    saveButton = save;
    const feedback = el("div", { "aria-live": "polite" });
    const contactCredentials = el("div", { class: "credential-state", "data-credential": "contact" });
    const appCredentials = el("div", { class: "credential-state", "data-credential": "app" });
    credentialViews.set("contact", contactCredentials);
    credentialViews.set("app", appCredentials);
    function secretEditor(kind, label) {
      const secret = field({
        key: `${kind}_secret`,
        label: `${label} Secret`,
        type: "password",
        placeholder: "留空保留现有凭据",
        hint: "填写新值后随配置加密保存；页面不会回显已保存的值。",
      });
      secret.input.autocomplete = "new-password";
      secret.input.disabled = !managed;
      const clear = el("input", { type: "checkbox", name: `clear_${kind}_secret`, disabled: !managed });
      clear.addEventListener("change", () => {
        secret.input.disabled = !managed || clear.checked;
        if (clear.checked) secret.input.value = "";
      });
      return {
        input: secret.input,
        clear,
        node: el("div", {}, secret.wrapper, el("label", { class: "check-label" }, clear, "清除已保存的 Secret，回退使用服务器环境配置（若无环境值则停用此凭据）")),
        read: () => ({
          ...(secret.input.value.trim() ? { [`${kind}_secret`]: secret.input.value.trim() } : {}),
          [`clear_${kind}_secret`]: clear.checked,
        }),
        reset: () => {
          secret.input.value = "";
          clear.checked = false;
          secret.input.disabled = !managed;
        },
      };
    }
    const contactSecret = secretEditor("contact", "通讯录读取凭据"),
      appSecret = secretEditor("app", "自建应用");
    const editor = el(
      "form",
      { id: "wecom-settings" },
      el("h2", {}, "企业连接配置"),
      el("p", { class: "hint" }, "两套凭据共用同一个企业 ID，各自检查连接和可访问的数据。"),
      corp.wrapper,
      configStatus,
      el(
        "div", { class: "credential-grid" },
        el("article", { class: "credential-card" },
          el("h3", {}, "通讯录读取凭据"),
          el("p", {}, "用于组织架构、部门和成员同步。可填写有通讯录读取权限的自建应用 Secret，也可使用专用的读取应用；此处凭据独立配置，不会自动切换到右侧应用。"),
          contactCredentials,
          contactSecret.node,
          el("p", { class: "hint" }, "也可由服务器设置 WECOM_CONTACT_SECRET。网页保存的值优先于环境配置；修改环境变量后需重启服务。"),
          el("p", { class: "notice" }, "通讯录权限可能只允许读取成员、部门 ID，无法读取姓名、手机号等详细资料。请以检查结果中的实际可读能力为准。"),
          probe("contact")
        ),
        el("article", { class: "credential-card" },
          el("h3", {}, "自建应用凭据"),
          el("p", {}, "同一自建应用用于打卡与审批数据读取。"),
          agent.wrapper,
          appCredentials,
          appSecret.node,
          el("p", { class: "hint" }, "也可由服务器设置 WECOM_APP_SECRET。应用 ID 与 Secret 必须来自同一个自建应用。"),
          el("div", { class: "notice" },
            el("strong", {}, "在企业微信后台授予接口权限"),
            el("p", {}, "进入「应用管理 → 打卡 → API」，将此自建应用加入可调用接口的应用；再进入「应用管理 → 审批 → API」，单独加入同一应用。"),
            el("p", {}, "应用的可见范围也需覆盖要同步的成员；本地过滤不能扩大企业微信授权范围。")
          ),
          probe("app"), probe("checkin"), probe("approval")
        )
      ),
      el("p", { class: "hint" }, "连接检查只验证读取能力，不会同步业务数据或发送消息。检查结果不会长期保存为权限保证；授权变更后请重新检查。"),
      el("h3", {}, "过滤部门"),
      el(
        "p",
        { class: "hint" },
        "可直接填写“分子公司”“新入职员工”等中文部门名称。多部门成员属于任一排除部门时即被排除。"
      ),
      departments.node,
      el(
        "label",
        { class: "check-label" },
        children,
        "包含名称或具体部门对应的所有子部门（仅同步和排除均适用，后续同步按最新架构覆盖新部门及新成员）"
      ),
      el("h3", {}, "过滤人员"),
      el(
        "p",
        { class: "hint" },
        "直接填写中文姓名；部门与人员条件同时生效，排除规则优先。尚未同步通讯录时也可先保存姓名规则。"
      ),
      users.node,
      el(
        "p",
        { class: "notice" },
        "规则对通讯录、打卡、请假、外出和出差同步统一生效。保存过滤规则后，已导入且被明确排除的部门和人员将移出当前组织、员工与通讯录列表，历史考勤、审批和消息保留。仅移除自动导入的本地账号，人工账号和管理员账号需人工核对。过滤选项仍保留原有部门和人员，便于撤销规则；解除过滤后，被移除的本地账号或部门需管理员恢复，账号恢复后保持停用。已导入审批的撤销、驳回仍会取消其考勤效力。扩大范围后，如需历史数据，请填写起止时间补同步。"
      ),
      feedback,
      managed
        ? el("footer", { class: "form-actions" }, save)
        : el("p", { class: "hint" }, "需要企业微信管理权限才能修改配置。")
    );
    editor.addEventListener("input", (event) => {
      if (!event.target.name) return;
      dirty = true;
      invalidateDiagnostics();
      feedback.replaceChildren();
      updateButtons();
    });
    editor.addEventListener("submit", async (event) => {
      event.preventDefault();
      if (!managed || saving || checking || submitting) return;
      saving = true;
      save.disabled = true;
      editor.inert = true;
      editor.setAttribute("aria-busy", "true");
      feedback.replaceChildren();
      updateButtons();
      try {
        const saved = await api("/wecom/settings", {
          corp_id: corp.input.value.trim(),
          agent_id: agent.input.value.trim(),
          ...contactSecret.read(),
          ...appSecret.read(),
          ...departments.read(),
          ...users.read(),
          include_subdepartments: children.checked,
        });
        if (stopped) return;
        corp.input.value = saved.corp_id;
        agent.input.value = saved.agent_id;
        departments.reset(saved);
        users.reset(saved);
        contactSecret.reset();
        appSecret.reset();
        dirty = false;
        invalidateDiagnostics("配置已保存，请重新检查连接与接口权限。");
        feedback.append(
          el("p", { class: "hint" }, "配置已保存，已应用当前过滤规则；后续同步继续使用该范围。可在员工或部门映射页面查看并清理已过滤对象。")
        );
        toast("配置已保存");
        await refresh();
      } catch (error) {
        if (!stopped) feedback.append(errorNode(error));
      } finally {
        saving = false;
        editor.inert = false;
        editor.removeAttribute("aria-busy");
        save.disabled = false;
        updateButtons();
      }
    });
    configPanel.replaceChildren(editor);
    syncPanel.append(el("h2", {}, "手动同步"), syncStatus);
    if (can("wecom", "write")) {
      const start = field({
        key: "start_time",
        label: "开始时间（上海，可选）",
        type: "datetime-local",
      });
      const end = field({
        key: "end_time",
        label: "结束时间（上海，可选）",
        type: "datetime-local",
      });
      syncPanel.append(
        el("div", { class: "form-grid" }, start.wrapper, end.wrapper)
      );
      const errors = el("div");
      const actions = el("div", { class: "sync-actions" });
      for (const kind of ["contact", "checkin", "leave", "out", "trip", "overtime"]) {
        if (kind === "contact" && !managed) continue;
        const run = button(
          `同步${labels[kind]}`,
          async () => {
            errors.replaceChildren();
            submitting = true;
            updateButtons();
            try {
              const from = start.input.value,
                to = end.input.value;
              if (
                kind !== "contact" &&
                (Boolean(from) !== Boolean(to) || (from && to <= from))
              )
                throw new Error(
                  "开始和结束时间需同时填写且结束晚于开始，或全部留空使用增量同步"
                );
              const times =
                kind === "contact"
                  ? {}
                  : {
                      start_time: from ? from.replace("T", " ") + ":00" : "",
                      end_time: to ? to.replace("T", " ") + ":00" : "",
                    };
              const result = await api(`/wecom/sync/${kind}`, times);
              toast(`${labels[kind]}同步已启动：${result.run_id}`);
              await refresh();
            } catch (error) {
              if (!stopped) errors.append(errorNode(error));
            } finally {
              submitting = false;
              updateButtons();
            }
          },
          "primary",
          { "data-sync-kind": kind }
        );
        buttons.push([run, kind]);
        actions.append(
          el(
            "article",
            { class: "sync-card" },
            el("h3", {}, labels[kind]),
            el(
              "p",
              { class: "hint" },
              {
                contact:
                  "全量读取可见范围内的成员，建立本地组织与停用账号；需要管理权限，忽略起止时间。",
                checkin: "筛选范围内成员，按最多 100 人一批同步打卡。",
                leave: "只查询请假模板，并更新已导入请假的审批状态。",
                out: "只查询外出模板，并更新已导入外出的审批状态。",
                trip: "只查询出差模板，单独保存出差与考勤时数。",
                overtime: "只查询加班模板；撤销或驳回会取消对应加班记录，不自动作为有效加班发布。",
              }[kind]
            ),
            run
          )
        );
      }
      syncPanel.append(
        actions,
        errors,
        el(
          "p",
          { class: "hint" },
          "按钮不可用时，请先保存配置、设置对应 Secret，并为审批类型配置模板。所有模板停用时仅校准已导入记录。 ",
          link("配置审批模板 →", "wecom/templates")
        )
      );
    }
    await refresh();
  } catch (error) {
    if (!stopped)
      configPanel.replaceChildren(
        errorNode(error),
        button("重试", () => {
          state.cleanup?.();
          dashboard();
        })
      );
  }
}
