# Enterprise Service 接口及迁移说明

本服务承接原 3.5 企业项目升级后的企业扩展，运行于独立 OpenIM38 实例，不导入生产数据。官方 IM 和 Chat 保持各自模块；企业数据只写 `MONGO_DATABASE` 指定的独立数据库。入口为 `/enterprise/v1`，同时保留下面列出的旧路径，便于原界面适配。

## 认证和部署边界

普通请求头：`token: <OpenIM token>`、`chatToken: <Chat token>`、`platformID: <SDK 数字平台，可选>`。服务端通过内部 gRPC `/openim.admin.admin/ParseToken`、`/openim.admin.admin/CheckLoginForbidden`、`/openim.auth.Auth/parseToken` 每次核对真实账号、封禁和 Redis 有效令牌；两个身份必须相同。请求体中的 userID/sendID 不能替代认证身份。

管理员请求只需要 Chat 管理员令牌（chatToken，兼容 token），必须解析为 `userType=2`。网络中断返回 503，不当作令牌撤销；已撤销或身份不一致返回 401，业务权限不足返回 403。保持 OpenIM `{errCode,errMsg,errDlt,data}` 响应结构，客户端同时处理非 2xx HTTP。

用户显式退出先 POST `/enterprise/v1/session/logout`，请求体 `{}`，携带当前双令牌和可选 platformID。成功返回 `{errCode:0,data:{revoked:true}}`，随后客户端清本地资料；相同令牌对重复调用仍返回成功。后台先持久保存该 IM 令牌哈希及令牌对哈希的撤销屏障，再通过内部 `Auth/kickTokens` 撤销精确 IM 令牌，同时通过 Chat `InvalidateToken` 的内部精确令牌参数 HDEL 当前 Chat 注册项，并清退对应媒体会话。两端撤销和媒体移除均确认后才返回成功。临时失败返回 503，并以加密保存的会话每 10 秒重试；屏障已经存在时，换一枚 Chat 令牌也不能复用已退出的 IM 令牌。旧 IM-only 完成记录会补做 Chat 撤销。记录至少保留至原令牌失效，不因进程重启丢失。不会调用按平台整体踢出的接口，因此迟到的旧端退出不影响新登录；其他平台也保持在线。普通断网、应用切换后台及自动重连不调用此接口。被踢或令牌失效走客户端本地清理，不再次主动注销。

本开发版本对官方 IM 作了兼容补丁：`Auth/kickTokens` 完成 Redis 标记后，调用原 gateway RPC 并附内部精确令牌过滤条件；gateway 仅关闭持有相同令牌的连接。该条件必须同时注册到 OpenIM RpcCustomHeader 上下文，避免客户端拦截器丢弃；没有过滤条件时，原管理员按平台强制下线行为保持不变。协议模块及 proto 未修改。部署此补丁时先更新 gateway 再更新 auth，或在隔离栈统一停启这两个进程；不可让新 auth 向尚未认识过滤条件的旧 gateway 提交注销。

Chat 的 `InvalidateToken` 只在内部 RPC metadata `openim38-chat-revoke-token` 存在时删除签名身份匹配的单枚注册项；不携带此参数时，改密、封禁等原有全用户撤销行为保持不变。公共 HTTP 层不转发此内部参数。新 Chat JWT 加随机 jti，避免同秒多端登录产生相同凭据。先部署新 Chat admin 再部署企业服务，或同时更新；旧 Chat 不认识精确参数，会按原接口清除全用户会话。备份包和版本清单必须将这两项兼容补丁一起保留。

内部 gRPC 和 Mongo、LiveKit 7880 只在隔离容器网络内监听。Nginx 覆盖 `X-Real-IP`，不得直接信任浏览器提供的转发身份。`/internal/` 不代理到外部。管理 IM 凭据仅由服务端获取和保存，浏览器不接收。

`main.go` 所需环境变量见 `services/enterprise-service/.env.example`。必须持久保存 `ENTERPRISE_SESSION_KEY`（64 位十六进制，即 32 字节 AES 密钥）、Mongo 数据和 LiveKit 密钥；重建配置不能覆盖原密钥。数据库不可用时启动失败，没有内存数据库降级。

## 朋友圈和标签

均为 POST，可使用原路径，也可增加 `/enterprise/v1` 前缀。

| 路径 | 请求与返回要点 |
|---|---|
| `/office/work_moment/add` | `{content:{text,type,metas},permission,permissionUserIDs,permissionGroupIDs,atUserIDs}` → `{workMoment}` |
| `/office/work_moment/get`、`del` | `{workMomentID}`；只有作者能删除 |
| `/office/work_moment/find/send` | `{userID,pagination}` → `{workMoments}`，仍检查访问者可见范围 |
| `/office/work_moment/find/recv` | `{pagination}` → 当前用户及真实好友的可见动态；忽略客户端自报 friendIDs |
| `/office/work_moment/like` | `{workMomentID,like}`；重复点赞不重复计数 |
| `/office/work_moment/comment/add` | `{workMomentID,content,replyUserID?}` → `{commentID,workMoment}` |
| `/office/work_moment/comment/del` | `{workMomentID,commentID}`；评论作者或动态作者可删除 |
| `/office/work_moment/logs` | `{pagination}` → `{workMoments}`，保留旧通知类型：点赞 1、@ 2、评论 3 |
| `/office/work_moment/unread/count` | `{}` → `{total}` |
| `/office/work_moment/unread/clear` | `{type:1/2/3}`：清数量、清列表、两者都清 |
| `/office/tag/add` | `{tagName,userIDs,groupIDs}` → `{tagID}` |
| `/office/tag/find/user` | `{}` → `{tags}`，只返回本人标签 |
| `/office/tag/get`、`del` | `{tagID}`；get 返回 `{tag}` |
| `/office/tag/set` | `{tagID,name?,addUserIDs,delUserIDs,addGroupIDs,delGroupIDs}` |
| `/office/tag/send` | `{requestID?,tagIDs,userIDs,groupIDs,content}` → `{sendLog}` |
| `/office/tag/send/log`、`send/log/del` | `{pagination}` / `{ids}` |
| `/office/tag/send/retry` | `{id:<sendLog.id>}`，只重试明确失败的接收人 |

permission 保持原值：0 公开、1 私密、2 指定可见、3 指定不可见；指定部门在本期不存在，permissionGroupIDs 按真实 IM 群成员展开。群选择要求操作者真实在群中。指定可见列表在创建时确定，并包含 @ 成员；不接受客户端伪造作者、点赞历史或创建时间。

标签发送合并标签、明确收件人和真实群成员，去重并排除自己，逐收件人持久保存结果。客户端为同一次发送固定 `requestID`；同一 requestID 再次提交不会重发已完成记录。`pending/sent/failed/inflight/uncertain` 明确区分：发送结果不确定或发送后写结果前崩溃时，不盲目自动重发，以免产生重复消息；需根据发送记录和 IM 历史核对后重新发起。

`content` 仍是原 SDK CustomElem 的 JSON 字符串，外层为 `{data:<内部业务对象的JSON字符串>,description,extension}`；内部保留 customType 903 及文字、图片等原格式。服务端拒绝缺少外层、空 data 或非对象 JSON 的载荷，返回 400，且不创建发送记录、不向 IM 发送不可渲染的空自定义消息。

## 群消息已读兼容接口

只代理这三个 `/msg/` 路径到企业服务，其余 `/msg/*` 继续交给官方 IM。

| 路径 | 请求 | data |
|---|---|---|
| `/msg/mark_group_message_read` | `{conversationID,userID?,clientMsgs:{clientMsgID:sendID},seqs:{clientMsgID:seq}}` | `{mark:{clientMsgID:0或1}}` |
| `/msg/get_group_message_read_num` | `{conversationID,clientMsgIDs,seqs?}` | `{num:{clientMsgID:{readNum,unreadNum}}}` |
| `/msg/get_group_message_has_read` | `{conversationID,clientMsgID,seq?,sendID?,filter:0或1,offset,count}` | `{count,reads:[{userID,readTime}],recipientUserIDs}` |

filter 0 为已读，1 为未读，兼容旧 type 和 pagination。也兼容汇总参数 clientMsgIDList。conversationID 必须为真实 `sg_<groupID>`；标记前从官方历史核验真实消息及发送者。新适配层必须带 message.seq；无 seq 的旧调用执行有限历史查找，可能较慢。

本栈启用三个必要内部回调：beforeSendGroupMsg、afterSendGroupMsg、afterMsgSaveDB，普通内容类型 `100-999`。前置回调同步持久化成员快照，`failedContinue:false`；存储回调补充已落库序列号。发送者排除在接收快照外；后续新入群者不增加历史未读，退群者仍保留在该快照。相同标记重复执行不会增加读数或重复生成事件。发送失败但已有前置快照的消息，不能绕过真实消息核验获得有效已读。

可选上线、下线、离线推送前和单聊已读回调继续由官方 `webhooks.yml` 的 afterUserOnline、afterUserOffline、beforeOfflinePush、afterSingleMsgRead 控制，默认关闭。测试接收器位于本服务 `/internal/openim/webhook/<WEBHOOK_SECRET>/<callbackCommand>`，返回官方顶层 `{actionCode:0,errCode:0,nextCode:0}`。诊断键使用完整载荷哈希（稳定最外层字段顺序），相同请求重试只保存一份；不能只用userID，因为原生单聊已读没有operationID，字段名为大写 `Seqs`，不同已读序列必须分别保留。不把到达顺序视为真实在线状态，也不代替持久事件队列。样例见 `tests/enterprise-service/webhook-samples.json`，仅用于接收协议检查；真实官方触发证据见 `tests/webhooks/summary.json`。未经验证的第三方回调不能直接作为授权依据。

## 呼叫、会议和事件恢复

POST `/signaling/<action>`（可加 `/enterprise/v1`）保留旧参数及返回 `{token,roomID,liveURL,meetingInfo}`：

- invite、inviteInGroup、accept、reject、cancel、hungUp、getTokenByRoomID。
- signal_create_meeting、signal_join_meeting、signal_get_meetings、signal_update_meeting_info、signal_close_room、signal_operate_stream、signal_send_custom_signal。
- signal_get_room_by_group_id、signal_get_rooms、get_signal_invitation_info、get_signal_invitation_info_start_app。
- signal_message_assemble 接受 `{signalReq:{invite|inviteInGroup|accept|reject|cancel|hungUp|getTokenByRoomID:<原请求>}}`，返回 `{signalResp:{对应操作:<结果>}}`。

调用者必须是真实邀请人、受邀者、会议成员，或活动群通话的当前真实群成员；独立会议不能只凭猜测 roomID 加入。已接受邀请不能再取消；拒接、取消、挂断及超时持久化，重复结束无副作用。账号级 CAS 占用记录防止两个不同通话被同时接受。群通话仍可在发起人离开后由其余成员继续，最后成员离开才结束。

新增 `/signaling/signal_invite_meeting`（别名 register_meeting_invitation）：`{roomID,userIDs,groupIDs}`。客户端**在发送或转发原 customType 905 会议卡片前**调用它，登记真正收件人；失败不能发送邀请卡片。主持人可邀请，普通成员仅在 onlyHostInviteUser=false 时可邀请。随后被邀请人可使用卡片或输入会议 ID 加入。会议配置、全体静音、禁用摄像头、屏幕共享和置顶等控制由服务端核验主持权限，不能只依赖客户端按钮隐藏。

会议时间采用原接口 Unix 秒（startTime/endTime/meetingDuration）；呼叫 initiateTime 和消息 readTime 使用 Unix 毫秒。get_signal_invitation_info 返回 `{invitationInfo,state,ended,meetingInfo,participantState,participant,activeInvitationForActor}`；只有 activeInvitationForActor=true 才弹 incoming。成员事件附当前 participant IDs。

GET `/enterprise/v1/rtc/events?after=N` 返回：

```json
{"errCode":0,"errMsg":"","errDlt":"","data":{"events":[{"id":42,"event":"OnInviteeAccepted","data":{"roomID":"...","invitation":{}}}],"cursor":42,"resyncRequired":false}}
```

首次启动用 after=-1 或 bootstrap=1，只取得完整 cursor，不重放旧弹窗；再调用 get_signal_invitation_info_start_app 恢复有效未处理邀请，并获取会议列表。后续持久化每个账号的 cursor，按顺序处理后推进；断线重连继续拉取。resyncRequired=true 时重新 bootstrap 和读取权威房间状态。单次最多 200 条，保留最近 2000 条并限制 4 MiB；客户端必须允许重复事件并以查询状态为准。

事件名称：OnReceiveNewInvitation、OnInviteeAccepted、OnInviteeRejected、OnInvitationCancelled、OnInvitationTimeout、OnHangUp、OnMeetingInfoChanged、OnRoomParticipantConnected、OnRoomParticipantDisconnected、OnStreamChanged、OnReceiveCustomSignal、OnWorkMomentChanged、OnGroupReadReceipt。最后一项 data 为 `{conversationID,groupMessageReadInfo:[{clientMsgID,hasReadCount,unreadCount,readMembers}]}`；Web 适配为原 OnRecvGroupReadReceipt。

房间状态与待发送事件同一文档 CAS 落库；分发器恢复重试，账号事件流去重。OpenIM custom110 仅作唤醒提示：其 customElem.data 含 enterpriseEvent=true、kind=enterprise_event、eventID、event。isOnlineOnly=true、notOfflinePush=true，关闭历史、持久化、发送方同步和会话更新，客户端识别 marker 后丢弃提示消息并拉取事件。原可见 customType 901–905 不得按类型一概过滤。提示队列满或发送失败时，正常轮询仍恢复业务状态。

## LiveKit 安全与管理员记录

客户端只访问 `wss://<测试入口>/livekit`。`/livekit/rtc` 和 `/livekit/rtc/validate` 由企业网关代理，核验签名、租约、房间代次、成员及原始 Chat/IM 会话，再转到内部 LiveKit。服务器原始会话用 AES-256-GCM 加密保存；令牌中没有 Chat/IM 原始令牌。SDK 自动刷新产生的签名令牌保留 enterpriseLease 属性，仍需通过网关检查。

LiveKit `room.auto_create:false`，服务显式创建房间。结束先持久化拒绝新连接，再 DeleteRoom 踢出现有连接；旧 JWT 即使未到期也不能通过网关重建房间。全体静音、允许成员开启麦克风/摄像头和屏幕限制同步实际发布来源权限。网关按当前房间策略重签转发的加入令牌，防止旧令牌恢复过时权限。参与者 metadata 为服务端读取的真实 `{userInfo,groupInfo?,groupMemberInfo?}`，客户端不能伪造他人资料。每 10 秒复核现有媒体会话，失效时移除参与者；认证基础设施故障不会被当作撤销而踢出全员。

内部 `/internal/livekit/webhook` 专门接收 LiveKit 生命周期事件，验证 expiring JWT 的签名、API key 及原始请求体 SHA-256，不能用普通入会令牌伪造回调。回调按唯一 MediaRoom 代次、当前 enterpriseLease 与参与者 SID 去重和过滤旧连接；事件只作提示，每 10 秒调用内部 RoomService/ListParticipants 读取实际参与者，补偿遗漏、乱序回调及服务重启。名单接口失败不会被当作房间无人。连接连续消失 30 秒后，单聊结束并释放双方忙线；群通话只移除断开者，最后一人离开才结束；会议移除断开者并保留日程。重连宽限内恢复连接则清除断开计时。断开观察及外部移除重试落库，旧清理任务不能踢掉同一账号的新租约。此策略也处理客户端进程退出而未发送 hungUp 的情况。

普通记录接口：`/rtc/get_signal_invitation_records`、`delete_signal_records`、`get_meeting_records`、`delete_meeting_records`，只能访问本人参与的记录。管理员对应使用 `/enterprise/v1/admin/rtc/*`，查看全量并软删除记录；删除记录不会关闭仍在运行的房间。

管理代理：`/enterprise/v1/admin/login` → Chat 管理员登录，响应剔除 IM 管理令牌；`admin/im/*`、`admin/chat/*`、`admin/business/*` 只接受真实 Chat 管理员身份，由后端代发请求。禁止浏览器通过 IM `/auth/*` 发行令牌；管理员可通过 `/auth/force_logout` 撤销已有会话。Chat 原始任意房间 RTC 令牌接口也在入口与管理代理禁止访问。历史预览使用 REST，不能冒充员工 SDK 登录。

代理先规范化点路径、重复及末尾斜杠再判定登录和授权；拒绝父级跳转、反斜杠、NUL 和二次 URL 解码路径。所有登录别名统一递归剔除 IM 凭据。三个服务的原始 `/config/*`、`/config`、`/restart` 运维入口不向浏览器代理，因为原配置包含数据库、OpenIM 签名及 LiveKit 密钥；原业务 `/client_config/*` 继续使用。两个旧在线状态接口仅返回平台、状态和连接数量，递归剔除员工令牌、令牌数组及其他凭据；按固定官方协议平台 ID 增加 `platform` 名称供原管理界面使用。在线列表无记录或平台列表为空时，界面显示离线。

## 应用更新

匿名 POST `/enterprise/v1/application/latest`：`{platform:"windows"|"android"|"ios"|"mac"|"linux",version:"1.0.0",build:1}`。

有升级时 data 为 `{available:true,version,build,url,sha256,size,force,text,platform}`；无记录、无可用完整安装包、同版同构建、已装更高版本或 Chat 查询失败时 available=false。服务端不返回 ASAR 热包，不提供降级。客户端一旦取得并确认强更，就不能绕过；下载失败可重试，实际大小和 SHA-256 不符禁止安装。

管理员 POST `/enterprise/v1/application/publish`：`{platform,version,build,url,sha256,size,force,text}`。必须是 HTTPS 完整安装包，语义版本递增、build 递增。企业元数据先暂存，调用官方 Chat 发布，再验证 Chat 当前记录并激活；中断不会暴露缺少校验信息的强更。已发布安装包的 URL/哈希/大小/构建号不可替换，需发布新版本。Chat 是版本和强更主记录，企业集合只补充安装包元数据。

## 验证命令与边界

在 services/enterprise-service 执行项目固定 Go 1.25.14 的 `go test ./...` 和 `go vet ./...`。测试包含真实 TCP/gRPC 编解码和 HTTP 路由，以及并发群已读、可见范围、标签重试、回调乱序、双端接受、重启超时、媒体旧令牌、会议权限和匿名强更元数据。

设置 `ENTERPRISE_TEST_MONGO_URI` 后运行 `go test ./internal/enterprise -run TestMongoCASPersistsAcrossClients -v`，只创建并删除随机 enterprise_test_UUID 数据库。默认不连接任何生产数据库。Mongo、真实 LiveKit、实际 SDK 跨端媒体及 100 连接运行验证必须由隔离栈集成报告证明；单元测试通过不能代替这些验收。
