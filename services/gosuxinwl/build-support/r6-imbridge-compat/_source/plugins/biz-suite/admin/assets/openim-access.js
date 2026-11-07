import { el, api, surface, button, dataTable, dialog, safe, toast, errorNode, state } from "./core.js";

const base = "/im/manager/openim";
const flags = [
  ["allowLAN", "局域网"], ["allowPC", "PC端（网页及Windows）"], ["allowMobile", "手机端（Android）"],
  ["allowInternet", "公网登录"], ["allowInternetDownload", "公网附件下载"]
];

export async function accessPanels(body, epoch) {
  const [networks, employees, certificate] = await Promise.allSettled([
    api(base + "/access/networks"), api(base + "/access/employees"), api(base + "/certificate")
  ]);
  if (epoch !== state.epoch) return;
  const networkPanel = surface(el("h2", {}, "访问网段"));
  body.append(networkPanel);
  if (networks.status === "rejected") networkPanel.append(errorNode(networks.reason));
  else {
    const config = networks.value;
    const input = el("textarea", { rows: "7", "aria-label": "可信局域网网段，每行一个CIDR", style: "width:100%;box-sizing:border-box" });
    input.value = (config.cidrs || []).join("\n");
    networkPanel.append(el("p", {}, "每行一个CIDR。这些来源按局域网权限判断，其余来源必须单独开通公网权限。不要添加覆盖所有来源的网段。"),
      el("p", {}, `当前来源：${config.currentIP || "未知"}；${config.currentZone === "lan" ? "属于配置的局域网" : "按公网权限控制"}`), input,
      button("保存网段", safe(async () => {
        const cidrs = input.value.split(/[\s,，;；]+/).filter(Boolean);
        await api(base + "/access/networks", { cidrs, expectedRevision: config.revision });
        toast("网段配置已保存；已有连接将按新规则重新核验");
        await refreshPanels(body, epoch);
      }), "primary"));
  }

  const employeePanel = surface(el("h2", {}, "员工访问权限"),
    el("p", {}, "默认允许局域网与PC端；手机、公网登录和公网附件下载分别由管理员开通。公网下载同时需要公网登录权限。收回权限会使旧会话失效，已下载文件无法收回。"));
  body.append(employeePanel);
  if (employees.status === "rejected") employeePanel.append(errorNode(employees.reason));
  else {
    const rows = Array.isArray(employees.value) ? employees.value : employees.value.items || [];
    const search = el("input", { type: "search", placeholder: "搜索工号、姓名或部门", "aria-label": "搜索员工访问权限" });
    const table = el("div", {});
    const render = () => {
      const query = search.value.trim().toLocaleLowerCase();
      const filtered = rows.filter(row => !query || [row.account, row.name, row.departmentName].join(" ").toLocaleLowerCase().includes(query));
      table.replaceChildren(dataTable(filtered, [
        ["account", "工号/账号"], ["name", "姓名"], ["departmentName", "部门"],
        ["eligible", "通讯状态", (value, row) => value ? "已开通" : row?.reason || "未开通"],
        ...flags.map(([key, title]) => [key, title, value => value ? "允许" : "关闭"])
      ], [{ label: "配置权限", run: safe(row => editEmployee(row, async () => refreshPanels(body, epoch))) }]));
    };
    search.addEventListener("input", render);
    employeePanel.append(search, table);
    render();
  }

  const certificatePanel = surface(el("h2", {}, "SSL证书"));
  body.append(certificatePanel);
  if (certificate.status === "rejected") certificatePanel.append(errorNode(certificate.reason));
  else renderCertificate(certificatePanel, certificate.value);
}

async function refreshPanels(body, epoch) {
  // The host manager owns these sections; reload through its existing refresh action.
  if (epoch !== state.epoch) return;
  window.dispatchEvent(new CustomEvent("gosuxin:im-access-changed"));
}

async function editEmployee(row, updated) {
  const fields = {};
  const content = el("div", {}, el("p", {}, `${row.name || ""}（${row.account || ""}）`));
  for (const [key, title] of flags) {
    const input = el("input", { type: "checkbox" });
    input.checked = !!row[key];
    fields[key] = input;
    content.append(el("label", { style: "display:block;padding:8px 0" }, input, " " + title));
  }
  fields.allowInternet.addEventListener("change", () => {
    if (!fields.allowInternet.checked) fields.allowInternetDownload.checked = false;
  });
  fields.allowInternetDownload.addEventListener("change", () => {
    if (fields.allowInternetDownload.checked) fields.allowInternet.checked = true;
  });
  content.append(el("p", {}, "PC与手机权限同时受局域网/公网权限约束。网页与Windows归PC端，Android应用归手机端。"));
  const box = dialog("配置员工访问权限", content);
  const save = button("保存权限", safe(async () => {
    save.disabled = true;
    try {
      await api(base + "/access/employee", { employeeID: row.employeeID, expectedRevision: row.revision,
        ...Object.fromEntries(Object.entries(fields).map(([key, input]) => [key, input.checked])) });
      box.close(); toast("员工权限已保存，旧会话已安排撤销"); await updated();
    } finally { save.disabled = false; }
  }), "primary");
  content.append(save);
}

function renderCertificate(panel, value) {
  const names = { not_configured: "尚未由后台配置", pending: "正在校验和应用", applied: "已应用", rejected: "未通过校验，保留原证书" };
  const status = el("div", {});
  const refresh = next => {
    value = next;
    status.replaceChildren(el("p", {}, `域名：${value.domain || "kf.szputy.com"}；状态：${names[value.status] || value.status || "未知"}`));
    if (value.notAfter) status.append(el("p", {}, `有效期至：${value.notAfter}`));
    if (value.issuer) status.append(el("p", {}, `签发者：${value.issuer}`));
    if (value.fingerprintSHA256) status.append(el("p", {}, `SHA-256：${value.fingerprintSHA256}`));
    if (value.message) status.append(el("p", {}, value.message));
  };
  refresh(value); panel.append(status);
  if (!value.available) { panel.append(el("p", {}, "服务器尚未启用证书应用服务，请联系部署管理员。")); return; }
  panel.append(el("p", {}, "上传域名对应的完整PEM证书链及未加密PEM私钥。服务器会校验证书、密钥和Nginx配置，通过后重新加载；失败会保留原证书。私钥不会回显。"),
    button("配置证书", safe(async () => {
      const cert = el("input", { type: "file", accept: ".pem,.crt,.cer", "aria-label": "完整证书链PEM文件" });
      const key = el("input", { type: "file", accept: ".pem,.key", "aria-label": "证书私钥PEM文件" });
      const content = el("div", {}, el("label", { style: "display:block;padding:8px 0" }, "完整证书链 ", cert),
        el("label", { style: "display:block;padding:8px 0" }, "匹配私钥 ", key),
        el("p", {}, "证书须包含kf.szputy.com。上传公有CA证书不会自动改变DNS、端口映射或其他设备的信任设置。"));
      const box = dialog("配置SSL证书", content);
      const submit = button("校验并应用", safe(async () => {
        if (!cert.files?.[0] || !key.files?.[0]) throw new Error("请选择证书链和私钥文件");
        if (cert.files[0].size + key.files[0].size > 230 * 1024) throw new Error("证书文件总大小超过限制");
        submit.disabled = true;
        let certPEM = "", keyPEM = "";
        try {
          certPEM = await cert.files[0].text(); keyPEM = await key.files[0].text();
          const pending = await api(base + "/certificate", { certPEM, keyPEM, expectedRevision: value.revision });
          refresh(pending); cert.value = ""; key.value = ""; box.close(); toast("证书已提交，正在校验和应用");
          for (let n = 0; n < 10 && value.status === "pending"; n++) {
            await new Promise(resolve => setTimeout(resolve, 1500));
            refresh(await api(base + "/certificate"));
          }
        } finally { certPEM = ""; keyPEM = ""; cert.value = ""; key.value = ""; submit.disabled = false; }
      }), "primary");
      content.append(submit);
      box.modal.addEventListener("close", () => { cert.value = ""; key.value = ""; });
    })), button("刷新证书状态", safe(async () => refresh(await api(base + "/certificate")))));
}
