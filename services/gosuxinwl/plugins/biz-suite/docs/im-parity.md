# 企业即时通讯 0.1.9 实现与差异

0.1.9 将 IM 调整为企业内部通讯：从组织通讯录选择同事直接聊天，不再要求双方先申请和同意好友。宿主入口显示为“企业即时通讯”，原菜单 ID、路由和权限标识保持不变。**尚未达到 ruoyi-vue-pro IM 全部功能 1:1 对等，不能据此宣告全量迁移验收。**

参考代码为 [ruoyi-vue-pro 的 IM 模块](https://github.com/YunaiV/ruoyi-vue-pro/tree/3ebbfb0c7694ea5154d045638674897f3dcea181/yudao-module-im)，固定提交 `3ebbfb0c7694ea5154d045638674897f3dcea181`。源模块管理端共有 30 个 Controller；当前采用 Go API 重新实现行为，不保持 Java API 路径、DTO 字段或 Vue 组件结构兼容。移除好友申请门槛是本项目的企业通讯产品决策。

## 企业通讯规则

- 通讯录提供部门树、部门及下级筛选、分页，以及姓名和工号搜索。展示姓名、工号、部门、职位等白名单字段，不把内部用户 ID 作为姓名或工号匹配。尚未关联本地账号、停用账号或未开通 IM 的在职人员显示不可聊原因；界面不会自动启用账号或授予权限。
- 私聊使用宿主本地用户 ID 建立会话。新消息发送者需要有效账号、插件入口权限及 IM 读写权限；接收人需要有效账号、入口权限及 IM 读取权限。`im.manage` 包含相应 IM 权限。关联企业微信员工还需满足在职等目录条件。
- 好友关系不再决定能否聊天。原 `biz_im_friend` 中的备注和屏蔽继续保留，修改联系人只影响本方记录。任一方向的屏蔽都会阻止双方新私聊消息和私聊 RTC，不删除已有消息，也不解除对方设置的屏蔽。
- 当前用户仍可读取属于自己的历史私聊；对方停用、离职、删除或被屏蔽，不会把历史变成其他人的会话。历史访问不授权发送新消息。已读位置可在本端更新，不向当前不可联系的对方广播回执。
- 选择同事建群一次提交群资料和 `member_ids`。后端校验全部成员并在同一事务中建立群、成员及各自的空会话；任何成员不合法则整体失败。新会话有非零变更序号，可立即进入会话列表；之后自行申请入群仍遵守群的审批设置。
- 首次私聊提交成功后保存双方会话，并向当前有权限的接收人推送事件；收件人无需先打开该会话。断线期间的数据仍通过增量查询补拉。

## 功能覆盖

| 源功能 | 0.1.9 实现 | 仍有差异 |
| --- | --- | --- |
| 私聊消息、已读、回执、撤回 | 企业通讯资格和屏蔽校验；sender + client ID 幂等；持久消息；增量查询；单调已读游标；发送人两分钟内撤回 | 只支持 text、image、file、emoji；未移植语音、视频、合并转发、名片和完整系统通知类型 |
| 组织通讯录 | 部门树及下级筛选、分页、姓名/工号搜索、通信状态及不可聊原因 | 不在通讯录内直接修改宿主账号权限、员工任职或企业微信资料 |
| 群消息、回执、撤回 | 成员及禁言校验；发送/回执事务；已读成员查询；撤回后离线增量纠正 | 未移植源系统完整消息 JSON、引用、@、批量合并转发等渲染协议 |
| 群建立、加入审批、成员管理 | 选人原子建群；申请及同意/拒绝；邀请、退出、踢人；管理员、群主转移；全员/成员禁言；置顶；解散 | 成员昵称、复杂可见性和全部源通知气泡未逐项移植；前端操作以实际页面为准 |
| 好友及申请 | 企业内直接聊天；原备注、屏蔽及历史数据保留；申请、审核、删除写接口停用并返回 HTTP 410 | 有意不再实现源系统的好友门槛；旧表保留不代表仍可申请或审核 |
| 频道及消息 | 建立、发现、订阅、持久广播、只读成员；仅频道 owner/admin 可发布 | 未实现频道编辑/删除、素材库 CRUD、素材图文正文和素材消息 |
| 会话和多端同步 | 每人会话、预览、未读数；消息/会话 change_seq 增量；提交后推送；断线补拉 | 无独立已读变更流及源消息枚举协议；删除会话无独立增量 tombstone 流 |
| 表情包 | 系统包列表及创建；包内图片 URL；图片校验 | 无系统包编辑/删除、单个表情运营 CRUD、用户私人收藏表情 CRUD |
| 敏感词 | 列表、创建、发送文本替换；空替换值使用 `***` | 无编辑/删除、批量管理及源词库全部匹配策略 |
| IM 运营统计 | 消息/群/频道/历史好友记录/RTC 签发数量；最近消息日期趋势；发送者排名 | 无完整用户趋势、消息类型和群规模分布及全部管理分页/详情；旧好友数量不再代表通信资格 |
| IM 管理查询 | 统计及群主范围内的成员/申请查询 | 无跨用户聊天审计、好友管理、群封禁/解封等全部 Manager Controller |
| WebSocket | 按实际参与者及有效账号权限推送 message、receipt、recall、membership、contact、rtc；多连接；慢消费者断开；周期复核连接权限 | 单插件进程内 hub；无多实例消息代理、跨进程广播或离线设备推送 |
| RTC 音视频 | 会话及私聊资格/屏蔽校验；5 分钟 LiveKit HS256 入房令牌；密钥不回前端；缺配置返回 503；前端可加入会话房间 | 无完整呼叫状态机、接听/拒绝/取消/追加邀请/振铃超时、真实参与者状态、通话时长、签名 Webhook 和僵尸房间清理；`token_issued` 不代表成功通话 |
| 媒体附件 | `/im/media/upload` 调用宿主上传 service，复用扩展名、大小、存储 provider、附件表 | 附件 URL 访问控制沿用宿主配置，不另建 IM 私有文件鉴权 |

## 当前接口与数据约定

API 基础路径：`/plugins/biz-suite/api/admin/im`。所有请求需要宿主身份和插件权限；敏感词、系统表情包创建、频道创建和 Manager API 由 `biz-suite.im.manage` 控制。普通聊天另有参与者检查，管理权限不自动绕过成员检查。

- 通讯录：`GET /directory/departments`、`GET /directory/users`。人员查询支持 `q`、`department_id`、`include_subdepartments`、`unassigned`、`page`、`page_size`，返回 `items/total/page/page_size`。`unassigned=true` 查询未分配有效部门的人员，不能同时指定部门。`GET /private/peer?receiver_id=...` 获取同事或已有历史会话对方的显示资料；兼容 `user_id`，两参数冲突返回 400。人员状态使用 `account_status`，读取、发送及通话能力分别使用 `can_read/can_send/can_call`，不能仅凭在通讯录可见便允许发送。
- 私聊发送：`POST /private/messages`，字段 `receiver_id`、`client_message_id`、`message_type`、`content`。群/频道发送使用 `/groups/:id/messages`、`/channels/:id/messages`。
- 选人建群：`POST /groups`，字段 `name`、`notice`、`join_approval`、`member_ids`。创建者自动成为群主；成员去重，至少一名其他成员，提交的成员数组最多 200 项。
- 增量拉取：消息列表和 `/conversations` 的 `after_seq` 指 `change_seq`。按返回最大值推进游标；单次最多 200 条，满页继续拉取。消息 `seq` 保持发送顺序，撤回只改变 `change_seq`。
- 已读：`POST /conversations/read`，字段 `conversation_type`、`conversation_id`、`last_seq`。`last_seq` 必须来自该会话消息的 `seq`，不能传变更游标；重复和倒序回执不降低已读位置。
- 联系人：`POST /contacts/:user_id/action`，`action` 为 `block`、`unblock`、`update`；备注使用 `display_name`。只更新本方数据。
- 旧好友写接口：`POST /friends`、`POST /friend-requests`、`POST /friend-requests/:id/review` 停用并返回 HTTP 410；联系人 `delete` 也返回 410。旧关系和申请读取保留用于兼容，不执行新的好友流程。
- 群操作：`POST /groups/:id/action`，支持 `update`、`invite`、`quit`、`kick`、`set-admin`、`transfer-owner`、`mute-all`、`mute-member`、`pin`、`dissolve`。参数使用 `user_id`、`enabled`、`name`、`notice`、`message_id`；后端逐项校验角色。
- 撤回：`POST /messages/:id/recall`。回执成员：`GET /messages/:id/receipts`。
- 表情包：`GET/POST /face-packs`，创建参数 `items` 为 `{name,url}[]`，数据库读取结果为 JSON 字符串。
- 上传：`POST /media/upload` multipart `file`、`filetype`，返回宿主标准响应内附件的 `url/name/id`。

业务表使用 `biz_im_` 前缀。`biz_im_message` 统一保存私聊、群聊及频道消息，`biz_im_conversation` 保存每个用户的会话。发送和回执由数据库事务串行分配全局序号；幂等键为 `(sender_id, client_message_id)`，不同内容复用同一键返回冲突。重试不重复增加未读或发布事件。

## RTC 部署与验证边界

配置 `LIVEKIT_URL`（ws/wss URL）、`LIVEKIT_API_KEY` 和不少于 32 字节的 `LIVEKIT_API_SECRET`。`GET /rtc/config` 只报告是否配置；`POST /rtc/token` 接收 `conversation_type` 和 `conversation_id`，返回 `url/token/room/expires_in`。房间由会话确定，token 限定用户和房间，没有 LiveKit 管理权限。

真实音视频仍需可达的 LiveKit 服务、适合部署网络的 TURN/防火墙设置，以及 HTTPS 或本机安全上下文中的麦克风/摄像头权限。代码测试不能代替两台设备的网络与媒体验证。当前未运行用户的 LiveKit 服务，不能报告通话已在生产验证。

## 历史消息迁入

ETL 与实时通信资格校验分开：内部 `im.ValidateImport(ctx, userID, MessageInput, sendTime)` 只读验证历史数据，`im.ImportHistoricalMessage` 保留源时间且不发送新消息通知。普通 HTTP 发送接口无法选择历史导入模式。

导入使用 `ruoyi:<消息类型>:<源主键>` 作为客户端 ID，按源发送时间排序。私聊先建立有效源用户到宿主账号对应关系；可导入至已停用但未删除的对应账号，不会启用账号、授予权限、创建好友申请、建立双向好友或解除屏蔽。历史私聊不要求目标账号现在拥有实时聊天权限。群和频道仍需有效目标会话及成员关系；无法对应或不满足导入校验的记录进入 ETL 异常队列。

源 `ImContentTypeEnum` 使用整数类型（如 101 文本、102 图片、105 文件、115 表情），且 `content` 是对应 JSON。不能原样插入新表，需转换为当前字符串类型和内容。未支持类型必须保留在异常记录或迁移归档中，不能伪装成已迁移的文本。旧申请、关系和屏蔽表不做清空或批量改写。

## 0.1.9 验收记录

本次验收应覆盖：无好友关系首次聊天及接收推送、通讯录姓名/工号和部门筛选、未开通账号不可聊、任一方向屏蔽阻断新消息及 RTC、本方解除不能解除对方屏蔽、历史私聊及已读可访问、旧好友写接口 410、选人建群整体成功或回滚、权限撤销后的推送限制，以及 ETL 不改变权限和屏蔽数据。

发布前已通过插件全部 Go 包测试、宿主 pluginworker/runtimeplugin/middleware/clogic/admin_system/utility 相关测试、47 项使用隔离 API 的真实 Chrome 前端测试，以及安装器检查、实际 worker 启动和失败迁移拒绝服务验证。真实宿主与 MySQL 的发布验收单独记录在工作区 `docs/biz-suite-local-validation.md` 的 0.1.9 节，并保留 `runtime/browser-tests/` 下的逐项报告。LiveKit 媒体与源完整历史数据未据此视为已验证；局部通过也不代表源系统 30 个 Controller 已逐接口迁移。
