# 原功能与新版实现对照

此表区分已保留的实现路径与实际运行证据。原3.5企业UI为客户端来源；3.4后台为管理UI来源。未完成的原组织树、UniMP、云录制入口保持原状态，不通过隐藏功能来代替升级。

按用户2026-11-01新指示，本轮暂缓视频、语音、语音/视频通话和会议测试；以下媒体记录仅保留原运行范围，不是113通过证明。112未完成会议改造及原生实验已归档并从活动源撤出，主线恢复已验证111后，113已通过42项非媒体回归并构建x64/ARM64正常主程序本地候选；安装包存在不等于媒体或真机通过。未完成标记和可恢复文件入口见 `tests/delivery/deferred-media-work-20261101.json`，不得将跳过项记为验收通过。

2026-11-02当前Windows版本为3.8.6 / 380601，本轮57/57适配测试、真实工作圈/托盘已读、合成截图GUI与ASAR及最终打包启动巡检通过。一次Object页面错误来源未确定，原记录保留，后续同闭环重测0错误；不将其写成已全面修复。当前包未代码签名；Android113与暂停的媒体事项保持原状态。

| 功能 | 原入口 / 新适配路径 | 当前证据 |
|---|---|---|
| PC窗口可见与已读 | 原会话UI；窗口可见、聚焦和对应会话门禁 | Windows3.8.6真实Electron托盘/隐藏/最小化/失焦与重新查看闭环通过；已记录旧已读不回退，仅验证后续新消息，见windows-386-read-runtime-review.json |
| PC截图确认/取消 | 原截图工具；主进程结束状态与受管结果交接 | Windows3.8.6合成截图GUI、ASAR核验和最终打包启动巡检通过，见windows-386-screenshot-fix-review-final.json、windows-386-screenshot-packaging-review.json与windows-386-native-smoke.json；实际多屏/缩放/显卡组合另验 |
| 邮箱/手机登录、注册、找回 | login/register/forget_password；新版Chat账号API | registration-reset-report.json：合成SMTP注册、自动登录、错误验证码、找回及改密撤销通过；真实客户端邮箱登录通过。Android111正常x64 main原界面实测精确旧会话三接口拒绝、冷启Login后同进程原表单成功重登，新会话三接口有效；见android-111-cache-revocation-ui-report.json及delivery同名root-runtime-review。曾观察SystemUI ANR弹窗的环境记录保留；本项不含媒体或物理ARM验收。手机短信供应商未配置 |
| 好友、黑名单、群、会话 | 原contact/contacts/conversation；ImFacade/AppIm | Web/Windows真实好友会话与群、群已读；social-client.json中备注设置/清空/同实例/重登、黑名单增删、群@通过。WASM空备注兼容分支仅接受安全字符ID，非标准自定义ID限制见web-desktop.md；Android原生探针已通过同实例备注设置/清空，其他操作按报告范围另验 |
| 文字、图片、文件、视频 | 原聊天编辑/渲染；新版SDK上传 | cross-client.json：Web/Windows双向文字，文件/PNG/MP4下载SHA；Windows视频实际解码；android-cross-client.json：Android→Web原界面接收及媒体下载校验 |
| 位置、名片、引用、合并、转发 | 原聊天工具；ImFacade格式转换及有效原消息校验 | enterprise-client.json：真实SDK发送/接收/渲染 |
| 语音、@、自定义 | 原消息工厂/渲染；固定SDK | Web/Windows群@通过；Android103原生探针语音及custom110/业务子类型901保持通过，见native-sdk-report-103-passed.json。最终三端原UI逐项结果按各报告范围记录 |
| 群已读名单 | SDK真实clientMsgID、sendID、seq → /msg兼容映射 | 官方真消息Webhook快照与API报告；真实Web群已读成员/名单通过。Android109字段映射、失败重试和pending事件刷新通过11项定向回归（包含在完整123项内）；同源x64正常main原页面已实测0/2→1/1→2/0、成员名单及真实断网后两tab重试恢复，见original-ui-group-read-report-109.json。这是三个合成成员的API30模拟器范围，物理ARM64和大群仍待验收 |
| 朋友圈、可见范围、评论、点赞 | 原工作圈UI → /office/work_moment/* | Windows3.8.6本人互动即时同步列表/详情、失败保留草稿、紧凑顶部、他人事件更新及缺失事件约5秒轮询通过；实测5062ms，不承诺严格≤5秒，见windows-386-moments-review.json。 服务端可见范围/幂等/owner验证；真实Web页面运行通过。Android108原UI四种范围发布：公开102/103均可见、仅自己两人均不可见、指定102仅102可见、排除102仅103可见；102原UI评论点赞后Android刷新显示。见android-collaboration-social-ready-108.json及android108-circle-scope-review.json；不作为109/ARM64实物或音视频通过证据 |
| 标签群发 | 原标签发送UI → /office/tag/* | 服务端owner/持久化/去重/重试单测；真实客户端去重发送通过 |
| OA通知 | 原OA专页；官方notification account + content1400/session4 | oa-api-report.json发送7个合成目标；真实Web与Android原main专页渲染通过，Android为API30/x86_64验证包 |
| 单聊/群通话、呼叫记录 | 原音视频弹窗 → 持久企业信令 + LiveKit | Web/Windows原视频呼叫、断网恢复、旧令牌及三种TURN中继有实际证据。Android110正常main同进程两通视频采样、文字送达及挂断资源释放通过限定范围验证；三人群前台媒体可用，但Home后曾短暂采样空白，退出时removeSink约58秒、Room72.313秒失败，不能标记群通话验收完成。原生统计缺失、模拟器及历史失败分别记录，见acceptance.md |
| 即时/预约会议、主持/静音/摄像头/共享 | 原会议入口 → 企业会议状态/受邀媒体权限，客户端读取权威meetingInfo | meeting-client.json六组Web/Windows原界面链路含结束旧令牌拒绝。Android110原预约创建、修改30→60分钟、取消并退出重入核对通过；原即时会三方加入、主持三项限制及恢复、双向screen_share有实证，最终确认结束资源1651ms且Web媒体关闭。首次Android协商/重连失败仍保留，最终详情仍显示Started；本轮未留真实旧RTC JWT用于重放。见android-110-meeting-runtime-report.json及tests/delivery/android-110-meeting-root-runtime-review.json；使用合成Web媒体，物理设备效果另验 |
| 更新检查/强更 | Chat版本主记录 + 企业安装包元数据 | 服务端可选/强更/不可降级及校验；Windows真实NSIS 3.8.4→最终3.8.5安装及安装后native回归通过，原更新hook六种策略另有隔离UI证据；Android门禁10项测试通过，实际安装更新尚待验收 |
| 多端策略 | 固定官方 policy4 | PC3/Web5/Android2并存与同类互踢真实API通过；最终已安装Windows精确Chat/IM撤销、旧WebSocket关闭、重新登录、被踢回登录且保留新会话通过；企业/媒体拒绝通过 |
| Seq、撤回与群权限 | 固定官方SDK/服务端，保留原消息UI | seq-client.json：未读5→0、离线补3条、8条历史ID一致、撤回UI、禁言成员转群主/1507通知及非法JoinSource拒绝通过 |
| Webhook | 必需3个群回调开启，其余默认关闭 | 真消息→快照→已读链路通过；重复/乱序/超时单测；4类可选原生事件由真实SDK/官方消息服务触发并匹配接收记录（webhooks/summary.json）；非第三方推送送达验证 |
| 后台用户/群/消息/呼叫/会议/应用版本 | 原3.4页面 → 服务器管理代理 | admin/smoke.json 19步页面/请求无错误；profile-report.json真实改密4组通过；admin/api-report.json附件完整性、CORS、错误用户创建 |
| 100个真实SDK持续连接 | Linux两分区50+50，固定SDK正常TLS/API/WSS | 最终服务端补丁后共同在线1805.750秒、5700条收发一致，0失败/断线/运行错误；24容器和18程序前后哈希一致，见 load-final/final-100-20261101a/analysis.json。生成器与业务资源分别统计 |

上述报告均在测试目录或客户端test-results，包含测试对象和范围。iOS仅源码适配；Android主程序已构建，主界面、原生探针、模拟器系统CA与真机结果分开记录。固定Go AAR未采用应用内开发CA，普通未Root手机需要公信证书或经验证的系统信任部署。AI/大模型延期，不把商业流式接口计入本次开源升级。
