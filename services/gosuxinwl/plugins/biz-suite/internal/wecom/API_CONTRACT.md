# 企业微信同步契约

已于 2026-10-28 通过企业微信开发者中心原始页面核对：

- [批量获取审批单号](https://developer.work.weixin.qq.com/document/path/91816)：`new_cursor` / `new_next_cursor` 为当前分页字段；旧 `cursor` / `next_cursor` 已标记待废弃。只有缺少 `new_next_cursor` 才表示完成，不能以空页判断。`sp_no_list` 是审批单号数组；单次最多 100 条，时间窗口最多 31 天。
- [审批详情](https://developer.work.weixin.qq.com/document/path/91983)：申请人来自 `info.applyer.userid`。请假控件时间路径为 `contents[id].value.vacation.attendance.date_range`，外出路径为 `contents[id].value.attendance.date_range`。通过 exact control ID 配置，禁止按标题/递归字段猜测。`new_duration` 的按天/按小时语义、perday_duration 和逐日分片需保留，当前统一转小时属于已审计的待修正项；状态 4、6、7 撤销已生成的有效考勤。
- [打卡记录](https://developer.work.weixin.qq.com/document/path/90262)：每请求最多 100 人、30 天；传 `opencheckindatatype: 3`。经纬度原值除以 1,000,000；缺卡使用标准打卡时间，不能补成当前时间。缺少可信时间或员工映射则只保留 raw 与异常。
- [部门成员详情](https://developer.work.weixin.qq.com/document/path/90201)：逐可见部门读取并去重，不使用已移除的 `fetch_child`。缺失手机号/邮箱不会清空本地已知值。快照不可见不等于离职，不据此删除人员。

0.1.3 使用两套凭据：通讯录只用 `WECOM_CONTACT_SECRET` 或网页保存的 contact Secret；打卡与审批统一用 `WECOM_APP_SECRET` 或网页保存的 app Secret。旧 `WECOM_SECRET` 仅兼容自建应用，旧 CHECKIN/APPROVAL 环境变量不再选用。网页保存优先环境，留空保留，显式清除后才回退环境。Token 仅内存缓存，Secret 的加密存储和密钥备份要求见插件 README。

自建应用必须加入对应“打卡／审批—可调用接口的应用”，新企业不能默认使用系统打卡/审批 Secret。通讯录 Secret 详情读取有新旧环境限制：[通讯录接口调整](https://developer.work.weixin.qq.com/document/path/96079)。这不是更换凭据的静默回退依据，诊断必须显示真实权限结果。

手动接口异步返回 `run_id`；运行记录显示 success/partial/failed 以及数量。单进程仅运行一个同步任务。失败/部分失败不推进成功游标，重试重放原始记录；每天上海时间 02:00 后首次运行执行 30 天校准。审批列表按提交时间筛选，因此每次额外复核已有未撤销单，识别很久以前通过的单据撤销。

未配置模板、不支持控件应保留原始审批并进入异常。当前批量申请在启用过滤时可能被过早跳过，这是已审计的待修正项，本轮配置开发没有修复此路径。真实企业权限、可见范围和生产网络连通性仍需联调。

## 0.1.3 配置与独立同步

`GET /settings` 仅提供非敏感配置。`POST /settings` 可附带只写 contact_secret/app_secret，空白保留；clear_contact_secret/clear_app_secret 显式清除；配置与密文同事务保存。需要 wecom.manage。`GET /status` 的 credentials 只返回 configured/source/has_saved/environment_variable。`GET /filter-options` 返回同步目录和唯一绑定员工的安全名称选项，不包含电话、邮箱、原始数据或凭据。

`POST /diagnostics/contact|app|checkin|approval` 需 wecom.manage；只读探测，与保存/同步互斥，不写业务记录。结果 status=missing/success/limited/failed，checks 独立列请求结果；不返回上游原始数据、Secret、Token或敏感成员资料。成功仅代表列出的接口本次可调用，不代表其他接口授权。

`leave` / `out` / `trip` 分别用 `filters: [{key: "template_id", value: "模板ID"}]` 请求审批列表，分页游标、时间游标和历史状态复核互相隔离。出差使用 Attendance 控件 `type=3`，外出 `type=4`。未配置模板的审批由定时 unmapped 任务保存到原始/异常表，不生成考勤。真实租户仍需在具备凭据后联调。

0.1.4 增加 `include_user_names` / `exclude_user_names` / `include_department_names` / `exclude_department_names`，均为精确名称数组，去首尾空白、去重；同名全部匹配。旧 userid / 部门 ID 数组兼容，包含条件与同类名称条件取并集，排除优先。部门名称匹配按当前部门树及 `include_subdepartments` 展开，未解析到名称的白名单不能退化为不限。人员首次同步使用本次上游姓名，后续业务按已同步或唯一员工映射名称过滤。

筛选对通讯录、打卡和审批申请人统一生效；打卡在分批前筛选，审批使用通讯录中企业微信部门归属判断。部门数据不完整时不绕过筛选。已导入的非通过状态仍复核，防止排除人员后遗留有效审批。设置变更不回退游标。

0.1.12 起，明确排除规则会将已导入员工标记为内部 `filtered` 状态，并软删除经 `biz_org_binding` 证明为自动创建且未受保护的宿主账号和部门。保存配置与此清理在同一事务中完成；通讯录成功同步和组织映射也应用清理，避免同步恢复已过滤对象。`GET /organization/prune` 仅预览，`POST /organization/prune` 执行，均需要 `wecom.manage`，与同步互斥。人工账号、当前账号、超级管理员及仍包含未排除对象的部门受保护。企业微信原始目录、映射、角色和历史考勤/消息保留；未知可见范围或未列入包含规则不触发清理。解除过滤不会自动恢复软删除的宿主对象或启用账号，需管理员核实后恢复。
