# biz-suite 管理前端

该目录是随运行时插件分发的独立 SPA，入口为 `/plugins/biz-suite/admin/`。使用随宿主登录有效期校验的 HttpOnly Cookie 和同源请求；企业即时通讯页面继续执行登录保活，不读取或复制宿主认证私钥。

## 本地构建与测试

```powershell
npm ci --ignore-scripts
npm run build
npm test
```

`npm run build` 验证业务模块导入并将 LiveKit SDK 打包为 `assets/livekit.js`。生产端没有 npm、CDN 或互联网脚本依赖。`package-lock.json` 固定版本；SDK 和附带许可证包含在插件静态资源中。

浏览器测试使用 Node 自带 WebSocket 驱动真实 Chrome DevTools Protocol，不依赖私有 Playwright 安装。优先使用 `CHROME_PATH`，也会检测常见 Chrome / Chromium / Edge 路径。测试启动临时无头浏览器及本地 API 测试服务，不访问真实企业微信或生产数据库。

```powershell
$env:CHROME_PATH = 'C:/Program Files/Google/Chrome/Application/chrome.exe'
npm test
```

可选设置 `BIZ_SCREENSHOT_PATH` 保存浏览器截图。CDP 测试覆盖深链接、HTML 注入防护、员工表单、班次配置、审批模板、配置保存回显、独立同步按钮、时间窗同步、全部菜单路由、聊天发送与回执、宿主媒体上传、RTC SDK 加载、权限与失效会话。

## 功能与接口

- `/hrm/employees`：员工档案、宿主账号和部门映射，职位与企业微信 userid。
- `/hrm/groups`：考勤组、多班次、工作日、午休、跨夜班次和宽限分钟。组与员工列表使用后端分页，表单选项从 `/hrm/options` 读取。
- `/hrm/holidays`、`clocks`、`leaves`、`outs`、`trips`：可编辑业务表单。企业微信打卡与审批只读，避免同步数据被手工覆盖。
- `/hrm/month`：月汇总和每日明细，服务端统一按上海时区计算。
- `/wecom/status`：企业/应用 ID 保存、各接口 Secret 状态、部门/人员中文名称仅同步与排除设置（含子部门，精确匹配同名全部；ID 兼容保留在高级配置）、通讯录/打卡/请假/外出/出差五个独立入口、时间窗补同步、批次轮询和失败记录。配置编辑需要 wecom.manage；通讯录／自建应用两套 Secret 可网页只写、加密保存，留空保留，显式清除回退环境；四项独立只读连接诊断区分配置与接口可用。
- `/wecom/templates`：明确的模板 ID、时段/原因/地点控件 ID 映射；`/wecom/departments` 关联宿主部门，`/wecom/exceptions` 检查原始异常。
- `/im/chat`：私聊、群聊、频道、WebSocket 推送、指数退避重连、增量追回、已读、撤回和回执。上传附件经过 `/im/media/upload` 复用宿主存储服务。
- `/im/directory`：企业部门树、姓名/工号查询、包含下级与未分配部门筛选、账号状态、直接聊天和勾选同事建群。旧 `/im/friends` 自动跳转此页，旧好友写接口停止使用。
- `/im/groups`、`channels`：群成员管理、群资料、加入审批和频道订阅。私聊支持联系人备注及双向屏蔽，屏蔽或停用后保留本人的已有历史。未选择会话时也保持实时连接，接收首条私聊及新群通知。
- `/im/face-packs`、`sensitive-words`、`manager`：表情素材、敏感词和管理统计。
- 音视频：本地 LiveKit SDK 实际连接短期令牌指定的房间，采集麦克风/摄像头并播放订阅轨道；未配置服务时显示明确提示。需要已部署 LiveKit 及浏览器安全上下文，浏览器测试不替代真实音视频服务联调。

## 权限与安全

首次加载 `/api/admin/access` 获取宿主用户 ID 和业务权限。`read` 控制页面可见，`write` 控制新增/修改，`manage` 包含读写并控制管理页。客户端只负责交互提示，接口权限由服务端再次验证。所有业务文本使用 DOM 文本节点；附件和图片仅允许 HTTP(S) URL。

## 与原计划的实现差异

此实现依据业务接口重新编写管理界面，使用原生 ES 模块和 DOM 组件；没有直接搬运 `yudao-ui-admin-vue3` 的 Vue / Element Plus 页面、Store 或组件树，也不宣称完成原项目全部页面的 1:1 视觉复制。页面行为覆盖本地插件已实现的 HRM 核心、IM 和企业微信接口；记录详情与编辑使用对话框，没有复制上游全部隐藏详情路由。若要求原版 Vue 页面与视觉完全一致，需要另行完成组件库与路由适配；当前界面可独立运行和测试。

不包含 OA 或 Flowable 菜单、页面、业务表单。
