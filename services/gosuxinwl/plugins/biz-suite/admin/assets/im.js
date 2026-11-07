import {
  el,
  api,
  asRows,
  can,
  page,
  surface,
  button,
  dataTable,
  resource,
  edit,
  form,
  dialog,
  status,
  errorNode,
  toast,
  parseJSON,
  state,
  safe,
  navigate,
  safeURL,
  API,
} from "./core.js";
import { directory, createGroup, chooseMembers, memberName, contactName, maySend } from "./directory.js";
export { directory };
const chatLink = (kind, id, name) =>
  navigate(
    "im/chat?" + new URLSearchParams({ kind, id, name: name || "" })
  );
export function groups() {
  const epoch = state.epoch;
  return resource({
    title: "群组",
    itemName: "群组",
    description: "发现群组、申请加入、管理成员与查看群聊记录。",
    area: "im",
    path: "/im/groups",
    columns: [
      ["name", "名称"],
      ["owner_name", "群主", (v) => v || "未匹配成员"],
      ["notice", "公告"],
      ["join_approval", "加入规则", (v) => (Number(v) ? "需审批" : "直接加入")],
      ["status", "状态", status],
    ],
    actions: [
      { label: "聊天", run: (r) => chatLink("group", r.id, r.name) },
      ...(can("im", "write")
        ? [
            {
              label: "加入",
              run: async (r) => {
                await api(`/im/groups/${encodeURIComponent(r.id)}/join`, {});
                toast("加入请求已提交");
              },
            },
            { label: "成员 / 管理", run: (r) => groupManage(r) },
          ]
        : []),
    ],
    fields: null,
    notice: "新群组：使用下方创建入口。只有群成员可以查看与发送群消息。",
  }).then(() => {
    if (epoch === state.epoch && can("im", "write"))
      document.querySelector(".subheading .actions")?.prepend(
        button(
          "+ 创建群组",
          () => createGroup([], groups),
          "primary"
        )
      );
  });
}
async function groupManage(group) {
  const content = el("div");
  dialog(`${group.name} · 群组管理`, content);
  try {
    const members = await api(
      `/im/groups/${encodeURIComponent(group.id)}/members`
    );
    content.append(
      el("h3", {}, "群成员"),
      dataTable(members, [
        ["name", "姓名", (v, r) => memberName(r)],
        ["employee_no", "工号"],
        ["role", "角色", (v) => ({ owner: "群主", admin: "管理员", member: "成员" }[v] || "成员")],
        ["status", "状态", status],
        ["muted", "禁言", (v) => Number(v) ? "已禁言" : "正常"],
        ["joined_at", "加入时间"],
      ])
    );
    const myself = asRows(members).find(
      (m) => String(m.user_id) === String(state.uid)
    );
    const isAdmin =
      String(group.owner_id) === String(state.uid) || myself?.role === "admin";
    if (isAdmin) {
      const requests = await api(
        `/im/groups/${encodeURIComponent(group.id)}/requests`
      );
      content.append(
        el("h3", {}, "入群申请"),
        dataTable(
          requests,
          [
            ["name", "申请人", (v, r) => memberName(r)],
            ["status", "状态", status],
            ["created_at", "申请时间"],
          ],
          [
            {
              label: "通过",
              when: (r) => r.status === "pending",
              run: async (r) => {
                await api(
                  `/im/groups/${encodeURIComponent(
                    group.id
                  )}/requests/${encodeURIComponent(r.id)}/review`,
                  { approve: true }
                );
                toast("已通过，重新打开查看最新状态");
              },
            },
            {
              label: "拒绝",
              when: (r) => r.status === "pending",
              run: async (r) => {
                await api(
                  `/im/groups/${encodeURIComponent(
                    group.id
                  )}/requests/${encodeURIComponent(r.id)}/review`,
                  { approve: false }
                );
                toast("已拒绝");
              },
            },
          ]
        )
      );
      content.append(
        el("h3", {}, "成员管理"),
        button("邀请同事", () => chooseMembers("选择同事加入群组", async (ids) => {
          for (const id of ids) await api(`/im/groups/${encodeURIComponent(group.id)}/action`, { action: "invite", user_id: id });
          toast("邀请已完成，重新打开成员列表查看");
        })),
        form(
          [
            {
              key: "action",
              label: "操作",
              options: [
                ["kick", "移除成员"],
                ["mute-member", "成员禁言"],
                ["set-admin", "设置管理员"],
                ["mute-all", "全员禁言"],
                ["transfer-owner", "移交群主"],
              ],
            },
            { key: "user_id", label: "目标成员", type: "number", options: asRows(members).map(member => [member.user_id, `${memberName(member)}${member.employee_no ? " · " + member.employee_no : ""}`]) },
            {
              key: "enabled",
              label: "设置状态",
              type: "number",
              options: [
                [1, "启用"],
                [0, "取消"],
              ],
            },
          ],
          {},
          (data) =>
            api(`/im/groups/${encodeURIComponent(group.id)}/action`, {
              ...data,
              enabled: Boolean(data.enabled),
            }).then(() => toast("操作成功"))
        )
      );
      content.append(
        el("h3", {}, "群资料"),
        form(
          [
            { key: "name", label: "群名", required: true },
            { key: "notice", label: "公告", type: "textarea" },
          ],
          group,
          (data) =>
            api(`/im/groups/${encodeURIComponent(group.id)}/action`, {
              action: "update",
              ...data,
            }).then(() => toast("群资料已更新"))
        )
      );
    }
    content.append(
      button(
        "退出群组",
        safe(async () => {
          await api(`/im/groups/${encodeURIComponent(group.id)}/action`, {
            action: "quit",
          });
          toast("已退出群组");
        }),
        "danger"
      )
    );
  } catch (error) {
    content.append(errorNode(error));
  }
}
export function channels() {
  const epoch = state.epoch;
  return resource({
    title: "频道",
    itemName: "频道",
    description: "频道用于广播通知；订阅后接收频道消息，频道所有者负责发布。",
    area: "im",
    path: "/im/channels",
    columns: [
      ["name", "频道名称"],
      ["description", "说明"],
      ["owner_name", "创建者", (v) => v || "未匹配成员"],
      ["status", "状态", status],
    ],
    fields: null,
    actions: [
      { label: "查看", run: (r) => chatLink("channel", r.id, r.name) },
      ...(can("im", "write")
        ? [
            {
              label: "订阅",
              run: async (r) => {
                await api(`/im/channels/${encodeURIComponent(r.id)}/join`, {});
                toast("已订阅频道");
              },
            },
          ]
        : []),
    ],
  }).then(() => {
    if (epoch === state.epoch && can("im", "manage"))
      document.querySelector(".subheading .actions")?.prepend(
        button(
          "+ 创建频道",
          () =>
            edit(
              "创建频道",
              [
                { key: "name", label: "频道名称", required: true },
                { key: "description", label: "说明", type: "textarea" },
              ],
              {},
              (data) => api("/im/channels", data),
              channels
            ),
          "primary"
        )
      );
  });
}
export function sensitiveWords() {
  return resource({
    title: "敏感词",
    itemName: "敏感词",
    area: "im",
    writeAction: "manage",
    path: "/im/sensitive-words",
    description: "消息服务统一执行敏感词替换规则。",
    columns: [
      ["word", "敏感词"],
      ["replacement", "替换文本"],
      ["status", "状态", status],
    ],
    fields: [
      { key: "word", label: "敏感词", required: true },
      { key: "replacement", label: "替换文本", value: "***" },
    ],
  });
}
export async function facePacks() {
  const body = page("表情包", "管理表情素材，聊天时可以选择已保存的表情。");
  const panel = surface();
  body.append(panel);
  try {
    const rows = await api("/im/face-packs");
    const create = () => {
      const editor = el("form"),
        name = fieldInput("name", "表情包名称"),
        cover = fieldInput("cover_url", "封面 URL"),
        itemList = el("div"),
        errors = el("div");
      let view;
      const add = () => {
        const item = el(
          "div",
          { class: "form-grid" },
          fieldInput("item_name", "表情名称").wrapper,
          fieldInput("item_url", "表情 URL").wrapper
        );
        item.append(button("移除", () => item.remove(), "small"));
        itemList.append(item);
      };
      add();
      editor.append(
        errors,
        name.wrapper,
        cover.wrapper,
        itemList,
        button("+ 添加表情", add),
        el(
          "footer",
          { class: "form-actions" },
          el(
            "button",
            { type: "submit", class: "button primary" },
            "保存表情包"
          )
        )
      );
      editor.addEventListener("submit", async (e) => {
        e.preventDefault();
        errors.replaceChildren();
        try {
          const items = [...itemList.children].map((row) => ({
            name: row.querySelector("[name=item_name]").value,
            url: row.querySelector("[name=item_url]").value,
          }));
          if (!items.length || items.some((i) => !i.name || !safeURL(i.url)))
            throw new Error("请填写表情名称和有效 HTTP(S) URL");
          await api("/im/face-packs", {
            name: name.input.value,
            cover_url: cover.input.value,
            items,
          });
          view.close();
          await facePacks();
        } catch (error) {
          errors.append(errorNode(error));
        }
      });
      view = dialog("新增表情包", editor);
    };
    panel.replaceChildren(
      el(
        "div",
        { class: "subheading" },
        el("h2", {}, "表情包列表"),
        can("im", "manage")
          ? button("+ 新增表情包", create, "primary")
          : el("span")
      ),
      dataTable(
        rows,
        [
          ["name", "名称"],
          ["items", "表情数量", (v) => parseJSON(v, []).length],
          ["status", "状态", status],
        ],
        [
          {
            label: "详情",
            run: (row) => {
              const items = parseJSON(row.items, []);
              const grid = el("div", { class: "actions" });
              for (const item of items) {
                const url = safeURL(item.url);
                if (url)
                  grid.append(
                    el(
                      "figure",
                      {},
                      el("img", {
                        src: url,
                        alt: item.name,
                        width: 64,
                        height: 64,
                        loading: "lazy",
                        referrerpolicy: "no-referrer",
                      }),
                      el("figcaption", {}, item.name)
                    )
                  );
              }
              dialog(row.name, grid);
            },
          },
        ]
      )
    );
  } catch (error) {
    panel.replaceChildren(errorNode(error));
  }
}
function fieldInput(key, label) {
  const input = el("input", { name: key, required: key === "name" });
  return {
    input,
    wrapper: el("label", { class: "field" }, el("span", {}, label), input),
  };
}
export async function manager() {
  const body = page(
    "通讯管理统计",
    "查看企业消息、群组、频道与通话记录规模。"
  );
  const holder = surface();
  body.append(holder);
  try {
    const data = await api("/im/manager/summary");
    holder.replaceChildren(
      el(
        "div",
        { class: "cards" },
        Object.entries(data).map(([key, value]) =>
          el(
            "article",
            { class: "card" },
            el(
              "span",
              {},
              {
                private_messages: "私聊消息",
                group_messages: "群消息",
                groups: "群组",
                friends: "联系人设置",
                channels: "频道",
                channel_messages: "频道消息",
                rtc_calls: "通话记录",
              }[key] || key
            ),
            el("strong", {}, value)
          )
        )
      )
    );
  } catch (error) {
    holder.replaceChildren(errorNode(error));
  }
}
export async function chat() {
  const query = new URLSearchParams(location.search);
  const kind = ["private", "group", "channel"].includes(query.get("kind"))
      ? query.get("kind")
      : "private",
    target = query.get("id") || "",
    name = query.get("name") || (kind === "group" ? "群聊" : kind === "channel" ? "频道" : "企业成员");
  const body = page(
    "消息中心",
    "与企业同事直接沟通，或从企业通讯录选择成员发起群聊。",
    can("im", "write") ? el("div", { class: "actions chat-start-actions" },
      button("发起聊天", () => navigate("im/directory")),
      button("发起群聊", () => createGroup(), "primary")) : null
  );
  body.querySelector(".page-head").classList.add("chat-page-head");
  const conv = el("aside", { class: "conversations" }),
    chatArea = el("section", { class: "chat" }),
    layout = el("div", { class: "chat-layout" }, conv, chatArea);
  body.append(layout);
  let stopped = false,
    ws,
    reconnectTimer,
    pollTimer,
    cursor = 0,
    acknowledgedSeq = 0;
  const messageMap = new Map();
  const epoch = state.epoch;
  state.cleanup = () => {
    stopped = true;
    clearTimeout(reconnectTimer);
    clearTimeout(pollTimer);
    if (ws) ws.close();
  };
  let loadingMessages = false,
    refreshPending = false, conversationsVersion = 0;
  const connection = el("span", { class: "badge warn" }, "连接中");
  // The first conversation request may resolve before the chat header mounts.
  const heading = el("strong", {}, name);
  async function loadConversations() {
    if (stopped) return;
    const version = ++conversationsVersion;
    const rows = asRows(await api("/im/conversations"));
    if (stopped || version !== conversationsVersion) return;
    conv.replaceChildren(el("div", { class: "conversation-heading" }, el("h3", {}, "最近会话"), target ? null : connection), button("企业通讯录", () => navigate("im/directory"), "small"));
    for (const row of [...rows].reverse()) {
      const selected =
        row.conversation_type === kind &&
        String(row.conversation_id) === target;
      if (selected && kind !== "private" && typeof row.name === "string" && row.name.trim())
        heading.textContent = row.name;
      conv.append(
        el(
          "button",
          {
            class: "conversation" + (selected ? " active" : ""),
            onclick: () =>
              chatLink(
                row.conversation_type,
                row.conversation_id,
                row.name || ""
              ),
          },
          el(
            "strong",
            {},
            `${
              { private: "私聊", group: "群聊", channel: "频道" }[
                row.conversation_type
              ] || "会话"
            } · ${contactName(row)}`,
            Number(row.unread_count) > 0
              ? el("span", { class: "pill" }, ` ${row.unread_count} 未读`)
              : null
          ),
          el("small", {}, row.last_content || "暂无消息"),
          row.can_chat === false ? el("small", { class: "directory-unavailable" }, row.unavailable_reason || "暂不可发送新消息") : null
        )
      );
    }
    if (!rows.length)
      conv.append(
        el("p", { class: "hint" }, "从企业通讯录、群组或频道开始会话。新消息会自动出现。")
      );
  }
  try {
    await loadConversations();
  } catch (error) {
    conv.append(errorNode(error));
  }
  if (!target) {
    chatArea.append(
      el(
        "div",
        { class: "empty" },
        "选择最近会话，或打开企业通讯录查找同事。"
      )
    );
  }
  if (stopped || epoch !== state.epoch) return;
  const contactActions = el("div", { class: "actions" });
  const rtcButton = can("im", "write") ? button("音视频通话", safe(() => rtc(kind, target)), "small") : null;
  if (rtcButton && kind === "private") rtcButton.disabled = true;
  const head = el(
    "div",
    { class: "chat-head" },
    heading,
    el(
      "div",
      { class: "actions" },
      connection,
      contactActions,
      rtcButton
    )
  );
  const messages = el("div", { class: "messages", "aria-live": "polite" });
  if (target) chatArea.append(head, messages);
  const blockedNotice = el("p", { class: "notice chat-restriction", hidden: true });
  if (target) chatArea.append(blockedNotice);
  let peer = null, composer, peerVersion = 0;
  async function refreshPeer() {
    if (stopped || !target || kind !== "private") return;
    const version = ++peerVersion;
    try {
      const current = await api("/im/private/peer?user_id=" + encodeURIComponent(target));
      if (stopped || version !== peerVersion) return;
      peer = current;
      heading.textContent = contactName(peer);
      heading.title = [memberName(peer), peer.employee_no ? "工号 " + peer.employee_no : "", peer.department_path].filter(Boolean).join(" · ");
      const available = maySend(peer);
      blockedNotice.hidden = available;
      blockedNotice.textContent = peer.readonly_reason || peer.unavailable_reason || (!can("im", "write") ? "当前账号没有发送消息权限。" : "暂不可发送新消息，历史消息仍可查看。");
      for (const input of composer?.querySelectorAll("input,textarea,select,button") || []) input.disabled = !available;
      if (rtcButton) rtcButton.disabled = !Boolean(peer.can_call ?? peer.can_chat) || !can("im", "write");
      if (peer.can_read === false) { messageMap.clear(); messages.replaceChildren(el("p", { class: "notice" }, "当前账号不能读取此会话的历史消息。")); }
      contactActions.replaceChildren();
      if (can("im", "write")) contactActions.append(
        button("备注", () => edit("修改联系人备注", [{ key: "display_name", label: "备注名称" }], { display_name: peer.display_name || "" }, data => api(`/im/contacts/${encodeURIComponent(target)}/action`, { action: "update", ...data }), async () => { await refreshPeer(); await loadConversations(); }), "small"),
        button(peer.blocked_by_me ? "恢复联系" : "屏蔽", safe(async () => { await api(`/im/contacts/${encodeURIComponent(target)}/action`, { action: peer.blocked_by_me ? "unblock" : "block" }); await refreshPeer(); await loadConversations(); }), "small"));
    } catch (error) {
      if (stopped || version !== peerVersion) return;
      peer = null;
      blockedNotice.hidden = false; blockedNotice.textContent = error.message;
      for (const input of composer?.querySelectorAll("input,textarea,select,button") || []) input.disabled = true;
      if (rtcButton) rtcButton.disabled = true;
    }
  }
  const endpoint =
    kind === "private"
      ? "/im/private/messages?receiver_id=" + encodeURIComponent(target)
      : `/im/${kind === "group" ? "groups" : "channels"}/${encodeURIComponent(
          target
        )}/messages`;
  const messagePath = endpoint.split("?")[0];
  let permitted = Boolean(target) && can("im", "write");
  if (target && kind === "channel") {
    try {
      const channels = asRows(await api("/im/channels")),
        channel = channels.find((c) => String(c.id) === target);
      permitted = permitted && String(channel?.owner_id) === String(state.uid);
    } catch {
      permitted = false;
    }
  }
  if (permitted) {
    let sending = false;
    composer = el("form", { class: "composer" });
    const text = el("textarea", {
        name: "message",
        placeholder: "输入消息…",
        "aria-label": "消息内容",
        required: true,
      }),
      type = el(
        "select",
        { "aria-label": "消息类型" },
        [
          ["text", "文本"],
          ["image", "图片 URL"],
          ["file", "文件 URL"],
        ].map(([value, label]) => el("option", { value }, label))
      ),
      send = el("button", { type: "submit", class: "button primary" }, "发送");
    const uploadInput = el("input", {
        type: "file",
        hidden: true,
        "aria-label": "上传文件",
      }),
      uploadButton = button("上传文件", () => uploadInput.click(), "small");
    uploadInput.addEventListener("change", async () => {
      const file = uploadInput.files?.[0];
      if (!file) return;
      uploadButton.disabled = true;
      try {
        const result = await uploadMedia(file);
        text.value = result.url;
        type.value = file.type.startsWith("image/") ? "image" : "file";
        toast("文件已上传，可发送到当前会话");
      } catch (error) {
        toast(error.message);
      } finally {
        uploadButton.disabled = stopped || (kind === "private" && !maySend(peer));
        uploadInput.value = "";
      }
    });
    composer.append(
      uploadInput,
      text,
      el(
        "div",
        { class: "composer-footer" },
        el(
          "div",
          { class: "actions" },
          type,
          uploadButton,
          button(
            "表情",
            safe(async () => {
              const packs = asRows(await api("/im/face-packs"));
              const list = el("div", { class: "actions" });
              let modal;
              for (const pack of packs)
                for (const item of parseJSON(pack.items, [])) {
                  const url = safeURL(item.url);
                  if (url)
                    list.append(
                      button(item.name, () => {
                        text.value = url;
                        type.append(el("option", { value: "emoji" }, "表情"));
                        type.value = "emoji";
                        modal.close();
                      })
                    );
                }
              modal = dialog(
                "选择表情",
                list.children.length
                  ? list
                  : el("p", { class: "hint" }, "暂无表情包")
              );
            }),
            "small"
          )
        ),
        send
      )
    );
    composer.addEventListener("submit", async (event) => {
      event.preventDefault();
      if (sending || !text.value.trim() || stopped || (kind === "private" && !maySend(peer))) return;
      if (type.value !== "text" && !safeURL(text.value.trim())) {
        toast("请输入有效的 HTTP(S) 文件或图片 URL");
        return;
      }
      sending = true;
      send.disabled = true;
      try {
        await api(messagePath, {
          ...(kind === "private" ? { receiver_id: Number(target) } : {}),
          client_message_id: crypto.randomUUID(),
          message_type: type.value,
          content: text.value.trim(),
        });
        text.value = "";
        await loadMessages();
        await loadConversations();
      } catch (error) {
        toast(error.message);
        await refreshPeer();
      } finally {
        sending = false;
        send.disabled = stopped || (kind === "private" && !maySend(peer));
      }
    });
    if (kind === "private") for (const control of composer.querySelectorAll("input,textarea,select,button")) control.disabled = true;
    chatArea.append(composer);
  } else if (target)
    chatArea.append(
      el(
        "p",
        { class: "notice" },
        kind === "channel"
          ? "频道仅创建者可发布广播消息。"
          : "当前账号没有发送消息权限。"
      )
    );
  function renderMessages() {
    messages.replaceChildren();
    for (const row of [...messageMap.values()].sort(
      (a, b) => Number(a.seq) - Number(b.seq)
    )) {
      const own = String(row.sender_id) === String(state.uid);
      const item = el(
        "article",
        { class: "message" + (own ? " own" : "") },
        el(
          "small",
          {},
          `${own ? "我" : row.sender_name || "未匹配成员"}${row.sender_employee_no ? "（" + row.sender_employee_no + "）" : ""} · ${row.send_time || row.created_at || ""}`
        )
      );
      if (row.status === "recalled" || row.recalled) {
        item.append(el("em", {}, "消息已撤回"));
      } else if (
        ["image", "emoji"].includes(row.message_type) &&
        safeURL(row.content)
      ) {
        item.append(
          el("img", {
            src: safeURL(row.content),
            alt: "聊天图片",
            loading: "lazy",
            referrerpolicy: "no-referrer",
          })
        );
      } else if (row.message_type === "file" && safeURL(row.content)) {
        item.append(
          el(
            "a",
            {
              href: safeURL(row.content),
              target: "_blank",
              rel: "noopener noreferrer",
            },
            "打开文件"
          )
        );
      } else item.append(el("span", {}, row.content || ""));
      if (own) {
        item.append(
          el("small", {}, row.receipt_status === "read" ? "已读" : "已发送")
        );
        const actions = el(
          "div",
          { class: "actions" },
          button(
            "回执",
            safe(async () => dialog("消息回执", dataTable(await api(`/im/messages/${encodeURIComponent(row.id)}/receipts`), [
              ["name", "成员", (v, r) => memberName(r)], ["employee_no", "工号"], ["status", "状态", status], ["read_at", "阅读时间"],
            ]))),
            "small"
          )
        );
        if (can("im", "write") && row.status !== "recalled")
          actions.append(
            button(
              "撤回",
              safe(async () => {
                await api(
                  `/im/messages/${encodeURIComponent(row.id)}/recall`,
                  {}
                );
                await loadMessages();
              }),
              "small"
            )
          );
        item.append(actions);
      }
      messages.append(item);
    }
    if (!messageMap.size)
      messages.append(el("div", { class: "empty" }, "暂无消息，开始对话吧。"));
    messages.scrollTop = messages.scrollHeight;
  }
  async function loadMessages() {
    if (stopped || !target || (kind === "private" && (!peer || peer.can_read === false))) return;
    if (loadingMessages) {
      refreshPending = true;
      return;
    }
    loadingMessages = true;
    try {
      let more = true;
      while (more && !stopped) {
        const rows = asRows(
          await api(
            endpoint +
              (endpoint.includes("?") ? "&" : "?") +
              "after_seq=" +
              cursor
          )
        );
        if (stopped || (kind === "private" && (!peer || peer.can_read === false))) return;
        const previous = cursor;
        for (const row of rows) {
          messageMap.set(row.id, row);
          cursor = Math.max(cursor, Number(row.change_seq || row.seq || 0));
        }
        more = rows.length >= 200 && cursor > previous;
      }
      renderMessages();
      const last = Math.max(
        0,
        ...[...messageMap.values()].map((r) => Number(r.seq || 0))
      );
      if (
        last > acknowledgedSeq &&
        can("im", "write") &&
        document.visibilityState === "visible"
      ) {
        await api("/im/conversations/read", {
          conversation_type: kind,
          conversation_id: target,
          last_seq: last,
        });
        acknowledgedSeq = last;
      }
    } catch (error) {
      if (!messageMap.size) messages.replaceChildren(errorNode(error));
      else toast(error.message);
    } finally {
      loadingMessages = false;
      if (refreshPending && !stopped) {
        refreshPending = false;
        queueMicrotask(() => safe(loadMessages)());
      }
    }
  }
  let reconnectDelay = 1000;
  function connect() {
    if (stopped) return;
    const url = new URL(API + "/im/ws", location.origin);
    url.protocol = location.protocol === "https:" ? "wss:" : "ws:";
    ws = new WebSocket(url);
    ws.onopen = () => {
      if (stopped) return;
      connection.textContent = "实时连接";
      connection.className = "badge";
      reconnectDelay = 1000;
      safe(loadMessages)();
      safe(loadConversations)();
      safe(refreshPeer)();
    };
    ws.onmessage = (event) => {
      try {
        const packet = JSON.parse(event.data);
        if (["message", "receipt", "recall", "membership", "contact"].includes(packet.type)) {
          safe(loadMessages)();
          safe(loadConversations)();
          safe(refreshPeer)();
        }
      } catch {
        /* Ignore invalid transport frames. */
      }
    };
    ws.onclose = () => {
      if (stopped) return;
      connection.textContent = "重新连接";
      connection.className = "badge warn";
      reconnectTimer = setTimeout(connect, reconnectDelay);
      reconnectDelay = Math.min(reconnectDelay * 2, 30000);
    };
    ws.onerror = () => ws.close();
  }
  await refreshPeer();
  if (stopped) return;
  await loadMessages();
  if (epoch !== state.epoch) return;
  connect();
  const poll = async () => {
    if (stopped) return;
    await loadMessages();
    if (stopped) return;
    await refreshPeer();
    if (stopped) return;
    await loadConversations().catch(() => {});
    if (stopped) return;
    pollTimer = setTimeout(poll, 15000);
  };
  pollTimer = setTimeout(poll, 15000);
}
async function rtc(kind, target) {
  const content = el("div"),
    feedback = el("p", { class: "notice" }, "正在读取音视频服务配置…"),
    media = el("div", { class: "rtc-media" });
  content.append(feedback, media);
  const view = dialog("音视频通话", content);
  let room;
  view.modal.addEventListener("close", () => room?.disconnect());
  try {
    const config = await api("/im/rtc/config");
    if (!config.configured) {
      feedback.textContent =
        "尚未配置音视频服务，请管理员配置 LiveKit 后再发起通话。";
      return;
    }
    const join = button(
      "加入通话",
      safe(async () => {
        join.disabled = true;
        try {
          const [{ Room, RoomEvent }, credentials] = await Promise.all([
            import("./livekit.js"),
            api("/im/rtc/token", {
              conversation_type: kind,
              conversation_id: target,
            }),
          ]);
          room = new Room();
          room.on(RoomEvent.TrackSubscribed, (track) =>
            media.append(track.attach())
          );
          room.on(RoomEvent.TrackUnsubscribed, (track) =>
            track.detach().forEach((element) => element.remove())
          );
          room.on(RoomEvent.Disconnected, () => {
            feedback.textContent = "通话已结束";
          });
          await room.connect(credentials.url, credentials.token);
          await room.localParticipant.setMicrophoneEnabled(true);
          feedback.textContent = "已加入通话。对方在同一会话中加入即可接通。";
          content.append(
            button(
              "摄像头",
              safe(async () => {
                const enabled = !room.localParticipant.isCameraEnabled;
                await room.localParticipant.setCameraEnabled(enabled);
                media
                  .querySelectorAll("[data-local-video]")
                  .forEach((node) => node.remove());
                for (const p of room.localParticipant.videoTrackPublications.values()) {
                  if (enabled && p.track) {
                    const video = p.track.attach();
                    video.muted = true;
                    video.dataset.localVideo = "true";
                    media.append(video);
                  }
                }
              })
            ),
            button(
              "静音 / 取消静音",
              safe(() =>
                room.localParticipant.setMicrophoneEnabled(
                  !room.localParticipant.isMicrophoneEnabled
                )
              )
            ),
            button("结束通话", () => view.close(), "danger")
          );
        } catch (error) {
          await room?.disconnect();
          join.disabled = false;
          feedback.textContent = error.message;
        }
      }),
      "primary"
    );
    feedback.textContent =
      "加入后浏览器会请求麦克风权限。通话使用已认证会话的短期令牌。";
    content.append(join);
  } catch (error) {
    feedback.replaceWith(errorNode(error));
  }
}

async function uploadMedia(file) {
  const body = new FormData();
  body.append("file", file);
  body.append(
    "filetype",
    file.type.startsWith("image/")
      ? "image"
      : file.type.startsWith("audio/")
      ? "audio"
      : file.type.startsWith("video/")
      ? "video"
      : "file"
  );
  const response = await fetch(API + "/im/media/upload", {
    method: "POST",
    credentials: "include",
    headers: { "X-Requested-With": "XMLHttpRequest" },
    body,
  });
  let payload;
  try {
    payload = await response.json();
  } catch {
    throw new Error(`上传失败 (${response.status})`);
  }
  if (!response.ok || Number(payload.code) !== 0)
    throw new Error(payload.message || "上传失败");
  const data = payload.data;
  if (!data?.url || !safeURL(data.url))
    throw new Error("上传服务未返回有效文件地址");
  return data;
}

