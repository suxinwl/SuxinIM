import { el, api, page, surface, button, dataTable, dialog, safe, toast, errorNode, state } from "./core.js";
import { accessPanels } from "./openim-access.js";

let managerGeneration = 0;

export function launch() {
  const body = page("企业即时通讯", "使用当前HRM账号进入通讯。员工档案、部门及通讯权限由HRM统一管理。");
  body.append(surface(
    el("h2", {}, "Web 通讯"),
    el("p", {}, "首次开通完成后，可在浏览器、Windows 和 Android 使用同一账号。"),
    button("进入通讯", safe(async () => {
      const { url } = await api("/im/openim/ticket", {});
      // Same-origin URL is assembled by the trusted HRM service. Never accept
      // an arbitrary external redirect from this response.
      if (!url?.startsWith("/im/?ticket=")) throw new Error("通讯入口无效");
      location.assign(url);
    }), "primary")
  ));
}

export async function manager() {
  const epoch = state.epoch;
  const generation = ++managerGeneration;
  // Refreshes can overlap without a route change (and therefore without a new
  // state.epoch). Keep each render in its own mount and ignore older responses.
  const body = el("div", { "data-im-manager": "" });
  const isCurrent = () => epoch === state.epoch && generation === managerGeneration && body.isConnected;
  const refresh = async () => { if (isCurrent()) await manager(); };
  const view = page("通讯管理", "员工身份以HRM为准。同步每30秒核对一次；缺工号、冲突和失败会保留在列表中。",
    el("div", { class: "actions" },
      button("首次开通预览", safe(() => preview(refresh))),
      button("重试失败任务", safe(async () => { await api("/im/manager/openim/retry", {}); toast("已安排重试"); })),
      button("刷新", safe(refresh))
    )
  );
  view.append(body);
  const [employees, departments, groups] = await Promise.all([
    api("/im/manager/openim/employees"), api("/im/manager/openim/departments"),
    api("/im/manager/openim/groups").catch(error => ({ error }))
  ]);
  if (!isCurrent()) return;
  await accessPanels(body, isCurrent, refresh);
  if (!isCurrent()) return;
  body.append(surface(el("h2", {}, "员工开通与同步"), dataTable(employees, [
    ["projection", "工号", p => p.account], ["projection", "姓名", p => p.nickname],
    ["projection", "部门", p => p.departmentName], ["projection", "访问", p => p.enabled ? "已启用" : p.reason || "待处理"],
    ["status", "同步状态"], ["error", "异常"], ["appliedVersion", "已应用版本"], ["desiredVersion", "目标版本"]
  ])));
  body.append(surface(el("h2", {}, "部门群"), dataTable(departments, [
    ["projection", "部门", p => p.name], ["projection", "群ID", p => p.groupID],
    ["projection", "直属成员", p => p.memberUserIDs.length], ["projection", "状态", p => p.frozen ? "冻结并保留历史" : "启用"],
    ["status", "同步状态"], ["error", "异常"]
  ])));
  const groupSection = surface(el("h2", {}, "群组管理"));
  if (groups.error) groupSection.append(errorNode(groups.error));
  else {
    const sourceRows = Array.isArray(groups) ? groups : groups.groups || groups.groupInfos || groups.items || [];
    const rows = sourceRows.map(row => row.groupInfo ? { ...row.groupInfo, groupOwnerUserName: row.groupOwnerUserName } : row);
    groupSection.append(dataTable(rows, [["groupID", "群ID"], ["groupName", "名称"], ["memberCount", "成员数"], ["status", "状态"]], [
      { label: "成员", run: safe(async g => {
        const info = await api("/im/manager/openim/groups/members?groupID=" + encodeURIComponent(g.groupID));
        const members = Array.isArray(info) ? info : info.members || info.items || [];
        dialog("群成员", dataTable(members, [["userID", "用户ID"], ["nickname", "姓名"], ["roleLevel", "角色"]]));
      }) },
      { label: "禁言", run: safe(async g => { await api("/im/manager/openim/groups/mute", { groupID: g.groupID, muted: true }); toast("群已禁言"); }) },
      { label: "解除禁言", run: safe(async g => { await api("/im/manager/openim/groups/mute", { groupID: g.groupID, muted: false }); toast("已解除群禁言"); }) },
      { label: "公告", run: safe(async g => {
        const input = el("textarea", { rows: "6", maxlength: "2000", "aria-label": "群公告" }, g.notification || "");
        const box = dialog("编辑普通群公告", el("div", {}, input, button("保存", safe(async () => {
          await api("/im/manager/openim/groups/notice", { groupID: g.groupID, notice: input.value }); box.close(); toast("公告已更新");
        }), "primary")));
      }) }
    ]));
  }
  body.append(groupSection);
}

async function preview(refresh) {
  const result = await api("/im/manager/openim/preview");
  const content = el("div", {}, el("p", {}, result.initialProvisioned ? "首次开通已确认。后续新增员工会自动处理，管理员停用的账号不会被重新启用。" : "确认后启用合格在职员工并仅补充通讯角色；原密码和角色保留。缺工号及冲突人员暂缓。"),
    dataTable(result.items, [["employeeNo", "工号"], ["name", "姓名"], ["reason", "待处理原因"], ["willEnable", "可开通", v => v ? "是" : "否"]]));
  const box = dialog("首次开通预览", content);
  if (!result.initialProvisioned) content.append(button("确认按预览开通", safe(async () => {
    await api("/im/manager/openim/provision", { confirm: true }); box.close(); toast("已确认，等待员工同步"); await refresh();
  }), "primary"));
}
