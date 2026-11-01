# Web / Windows 3.8 升级（应用开发版 3.8.7 / 380701）

本应用由原 3.5 企业项目升级而来，保留原页面、资源与业务消息类型，运行于独立 OpenIM38 实例。原始目录没有修改。

## 2026-11-02 Windows 3.8.7 修复范围

- 多人转发按批次顺序发送，保留失败接收者供重试。引用消息转发保留原引用内容，避免固定 SDK 在缺失引用目标时退出；普通文字和媒体仍沿原 SDK 发送。
- 顶部综合搜索和“聊天记录”结果可正常显示、选择并定位到对应会话和原消息；失败或切换搜索时保留可重试状态。搜索范围仍为当前账号已同步的本地消息。
- 视频附件下载保持原签名地址不变，缓存参数通过客户端内部传递；启动失败可重试，下载后可从受管缓存播放。保留正常证书校验。

当前构建号380701。最终NSIS构建及对应打包程序的正常TLS、SDK初始化、受管文件Range巡检通过，见tests/delivery/windows-387-native-smoke.json。94项适配测试和TypeScript检查通过；真实Electron合成引用消息转发5人后SDK会话/历史有效，搜索5项原UI及10项Store回归通过；视频9项行为及原生MP4/Range/解码、正常TLS下载通过。证据见根tests/delivery下的windows-387-quote-facade-review.json、windows-387-search-review.json、windows-387-video-download-review.json、windows-387-final-adapter-tests.log和windows-387-final-typecheck.log。视频原生测试运行在独立隐藏窗口，未声称所有原聊天UI、历史附件或硬件组合均通过。

Windows3.8.7已发布到独立内网下载页。媒体已恢复开发和联调；Android114的163项Dart、108项Java边界测试及62个Java文件实际Android编译通过，仅保留本地正常main候选，不提供Android下载或更新。单端快速会议普通Leave/重入、End后Ended禁入通过限定验证；三人通话退出121023毫秒超时失败，后续17项物理任务最终完成且摄像头为空。媒体仍为NOT_ACCEPTED；预约会议、纯语音及完整跨端矩阵尚未完成，定向原生诊断不能替代正常验收。先前归档和以下3.8.6结果按原版本保留。

## 2026-11-02 Windows 3.8.6 历史修复范围

- 聊天窗口关闭到托盘、隐藏、最小化或未获得焦点时，不再仅因会话仍被选中而自动上报新消息已读；重新打开并查看对应会话后才按可见条件上报。服务器此前已经记录的已读不会倒退，请使用升级后新发送的消息复测。
- 工作圈本人点赞、取消点赞、评论和删除评论成功后立即更新列表与详情；发送失败保留评论草稿供重试。其他人的互动通过事件刷新，服务器缺少相应事件时由可见页面约 5 秒轮询补齐，实测取消点赞与删除合并显示为 5062 毫秒，不承诺严格 5 秒以内。
- 工作圈不再显示原资源中固定的黑蓝色加载截图；列表采用紧凑顶部，个人页保留头像和姓名。
- 修复截图确认后的重复进入截图框；确认或取消后结束本次截图，按原聊天工具使用截图结果。

安装或覆盖升级前，必须完全退出旧客户端，包括系统托盘，再运行 3.8.6 安装包。启动后在关于页核对 3.8.6 / 380601。当前仍是未作 Windows 代码签名的内网测试包，不包含 Android 或公网发布。

2026年11月2日本轮 Windows 3.8.6 修复通过57/57适配测试；真实工作圈原界面、真实Electron托盘已读闭环、合成截图GUI及打包ASAR核验、最终安装包对应的启动巡检均有通过记录。一次运行曾出现来源未确定的Object类型页面错误，原记录保留，后续同闭环重测为0错误；本轮不据此宣称全部页面错误均已修复。当时语音、视频、通话与会议暂停，Android113为本地候选；本轮已恢复媒体开发，结果不倒用于该历史版本。

本轮证据为 `tests/delivery/windows-386-final-adapter-tests.log`（57/57）、`windows-386-read-runtime-review.json`、`windows-386-moments-review.json`、`windows-386-moments-ui/report.json`、`windows-386-screenshot-fix-review-final.json`、`windows-386-screenshot-packaging-review.json` 与 `windows-386-native-smoke.json`。这些均在根 `tests/delivery` 下；Object错误诊断另存 `windows-386-read-object-error-diagnostic-review.json`。截图GUI使用合成截图场景，不能替代所有显卡、缩放或多屏组合的硬件实测。

## 固定依赖及构建

当前命令临时将根 `tools/node-v22.23.4-win-x64` 加入 PATH，在 `clients/web-desktop` 执行：

```powershell
npm ci --legacy-peer-deps
npm run typecheck
npm run test:adapter
npm run build:web
npm run build:win
node scripts/build-manifest.mjs
```

应用版本由 `build-version.json` 统一供渲染器和 Windows 更新器读取，当前为 3.8.7 / 380701；package.json 与之对应。上游 IM/Chat 和 SDK 版本不随应用修复号变更。

固定 WASM SDK `3.8.3-patch.15.1`、Electron `45.2.1`、electron-builder `26.18.0`、LiveKit JS `2.23.0`。三个历史 package_build/package_dev 文件与 package.json 一致，package-lock.json 是唯一安装锁。官方 npm 发布包已包含运行二进制；构建前 `scripts/sync-sdk-assets.mjs` 从固定 tag 的 `sdks/wasm/assets` 同步 openIM.wasm、sql-wasm.wasm、wasm_exec.js，并生成 public/sdk-assets.json 的 SHA-256。禁止换回原 3.5 WASM。

`test-results/sdk-package-provenance.json` 对官方 registry 原包的 SHA-512 及全部 79 个发布文件逐一校验，当前运行包与官方发布物完全一致（missing=[]、changed=[]）。同步工厂 tuple 解包及旧企业事件名已由该官方 npm 发布物包含；sdks/wasm 源码的相应补丁用于对齐已发布 npm 行为。无需手工修改 node_modules，不增加会在 npm ci 后丢失的运行补丁。表情图片使用随项目的 Twemoji，构建不依赖在线表情 CDN。

Web 输出 dist-web，Windows 当前输出 release/OpenIM38-Enterprise-3.8.7-win-x64.exe，release-manifest.json 记录安装包大小及 SHA-256。版本 3.8.7，构建号 380701。未配置代码签名，测试安装包未签名。当前安装包 160966886 字节，SHA-256 `422bf047b6cfec8c916614aa30464f737ebfb4062d3020b7ae70fbeca1afc873`。此前 3.8.3/3.8.4/3.8.5/3.8.6 的安装及验收报告保留原版本，不将历史证据改写为3.8.7；未发布的旧3.8.5候选37b337…已弃用。

## 隔离与网络

默认 Windows 测试服务为 https://192.168.10.11:29880，浏览器默认当前源；IM /api、Chat /chat、WebSocket /ws、企业接口 /enterprise/v1。配置覆盖仅使用 upgrade38 命名空间，IndexedDB 配置库名 OpenIM38-Config。

Windows appId=io.openim.enterprise.upgrade38，productName=OpenIM38 Enterprise，使用独立 OpenIM38 实例的应用标识。自动化测试设置 OPENIM38_TEST_DATA_DIR 到根 .cache 下，并把 TEMP/TMP 指向 E 盘 .cache/temp-web。测试数据与其他实例隔离。

Windows 页面使用 secure 标准协议 openim38://app，保留 webSecurity 和正常证书校验。每个 Electron Session 都注册处理器，正确支持 WASM Worker、COOP/COEP，以及受管媒体的 Range 206。/__local 仅允许软件受管缓存，检查 realpath 防止路径穿越和符号链接逃逸；浏览器端不使用 file:// 绕过安全策略。部署需正常信任开发 CA。

跨源下载从 /api/object 重定向到 29885 时，浏览器可能将最终请求 Origin 变为 null。29885 对签名对象 GET/HEAD 的 CORS 必须允许真实 Origin 及此重定向情况，并配置 Cross-Origin-Resource-Policy: cross-origin；主 API 权限不因此放宽。上传与文件下载均保留 TLS 验证。

## 统一适配和企业业务

ImFacade 集中转换 3.8 会话、用户、输入状态、消息默认字段和旧方法。登录返回早于首次 SQLite 同步，在 OnSyncServerFinish 后重新加载好友和会话，避免新安装客户端出现空列表。已发送消息才可引用，拦截缺少发送身份的无效引用，避免官方 WASM 对未发送引用的异常。

朋友圈、标签、群已读、呼叫与会议使用独立企业服务。请求同时发送 IM token、Chat token、platformID。事件流首次 after=-1 取得 cursor，再查询有效未处理邀请；后续增量轮询，custom110 控制提示只唤醒轮询，不显示成业务消息。原 custom901/903/905 业务卡片保留。

群已读从 SDK 真实本地消息构建 clientMsgs={clientMsgID:sendID} 和 seqs，服务端核验真实消息及发送时成员快照。历史、接收、新发送消息补充企业 read summary；缺失摘要显示未知，不冒充全部已读。列表通过企业分页 ID 再读取 SDK 群资料。

显式退出先 POST /enterprise/v1/session/logout 撤销当前精确令牌对，服务器确认后关闭 SDK、媒体、事件循环并清除配置；服务端踢线回调与主动退出共用一次底层 SDK logout Promise，防止同时进入 Go WASM 退出。网络失败保留会话供重试。互踢、令牌失效、密码变更使用被动本地清理。业务 403 不当作令牌撤销。

固定 Go WASM 的数据库 Close 会调用全局 close，而官方 JS SQL RPC 只注册 closeDB。浏览器普通页面通常忽略 window.close，Electron 会真正关闭窗口。3.8.5 仅在调用栈确认来自固定 openIM.wasm 时把 close 转发到 closeDB，保留普通网页 window.close 及应用窗口控制。数据库关闭同样单次执行，成功登录后重置；实际原生回归覆盖主动退出、同进程重登后 SQL 查询与发消息、被踢后保持登录窗口及新会话仍可用。

原通话及会议界面使用 LiveKit 2.x，保留接听、取消、拒绝、挂断、摄像头、麦克风和主持控制。905 会议卡片在发送、转发前登记真实受邀人，不能仅靠猜测 roomID 加入。会议状态从企业 get_signal_invitation_info.meetingInfo 取得并随 OnMeetingInfoChanged 刷新，替代旧专有服务的 LiveKit room.metadata 约定；流操作将 OnStreamChanged 转为旧事件并按目标用户/全员标记处理。新 SDK 好友批查明确 filterBlack=false 并去重用户 ID。固定 Go WASM 的增量 SQL 更新会忽略空备注，适配层在官方 updateFriends 成功后用已注册的 updateColumnsFriend RPC 写入常量 remark="" 并通知原界面刷新；不改服务数据库。该固定 RPC 的好友 ID 条件使用字符串插接，因此这条清空缓存分支在任何写入前仅接受 1～128 位 ASCII 字母、数字、下划线、短横线标识；不兼容其他自定义 ID，明确返回错误。RPC 只有该好友 ID 和空备注，未传 ownerID，数据库实例由当前 SDK 登录持有。含引号 ID 的负向用例确认不会调用 SQL RPC。

## 更新

匿名 /enterprise/v1/application/latest 汇总 Chat 主版本记录和企业 build、大小、SHA-256。仅较高构建、非降级语义版本可升级。无记录或检查失败允许运行；确认的强更显示阻断页面。

Windows 主进程重新从配置的可信服务器取得权威记录，核对版本、构建号、URL、大小、哈希；HTTPS 同源且不接受重定向，实际大小/SHA-256/EXE 头及平台校验通过才启动完整安装包。旧 ASAR 热替换拒绝执行。下载失败保留重试，写文件错误不会使主进程无处理崩溃。

## 运行证据与边界

客户端 test-results 下保存原始脱敏运行报告及截图，正式副本同步到根 tests/web-desktop：

- unit-tests-final.tap：17 项数据库关闭桥、完整安装包校验、版本排序、消息转换、控制提示和本地缓存 URL 测试。
- ui-smoke.json / login.png：真实浏览器登录页、WASM/SQL Worker、跨源隔离。
- electron-smoke.json / electron-login.png：原生 Electron 45、真实 preload File 桥、独立测试数据、受管缓存协议及 Range。
- cross-client.json：Web 与 Windows 实际登录、双向文字、附件、受管下载和媒体校验；以报告 textMessages / nativeFile / media / failures 的实际记录为准。
- enterprise-client.json：OA 1400 专用会话、位置/名片/引用/合并/转发、真实群已读、朋友圈权限/评论/点赞、标签幂等、LiveKit 合成媒体与原通话界面。合成摄像头/麦克风测试不能替代手机真机、真实硬件、真实屏幕分享验收。
- enterprise-data-client.json：最终 OA、复合消息、真实群已读、朋友圈及标签 4 组全部通过；其中标签有真实接收和原界面渲染证据。
- meeting-client.json：原预约表单持久化、快速会议三人视频解码、主持禁止成员开启麦克风/摄像头/共享、真实浏览器 display capture 远端 screen_share 解码、主持结束和旧 JWT 拒绝、三人群视频及发起人离开后另两人继续，6 组全部通过。摄像头/麦克风为 Chromium 合成源，共享为 Chromium 合成显示源，实际执行 getDisplayMedia 和远端 screen_share 解码，不声明真实桌面或硬件验收。
- social-client.json：真实 SDK 好友备注设置/清空/同实例读回/重新登录、原状态恢复，黑名单加入/列表/移除，群 @ 消息及原聊天界面渲染。
- policy-client.json：Windows/Web 并存、新 Web 踢旧 Web、旧企业凭据拒绝、Windows 收到新 Web 发送者同步。
- seq-client.json：两端 SDK 未读 5→0、Windows 原界面 badge=0、断网恢复 3 条、8 条消息身份一致、撤回界面、被禁言成员接任群主、1507 通知及非法 JoinSource 拒绝。
- session-logout.json：显式 Web 退出关闭真实 IM WebSocket，旧 IM/Chat/enterprise 均拒绝；断网退出不丢会话；重复旧 pair 注销不影响新同平台 pair。
- windows-logout.json：应用 3.8.5 原生退出、旧双 token 撤销、同进程重登 SQL/发送和真实被踢；3.8.4 的临时桥实验单独归档，不计入最终通过。
- update-gate.json：实际原更新 hook 的 6 种状态：无记录、同版本/构建、低版本、可选稍后、强更下载中断/校验失败继续阻断并重试、查询失败可运行。该报告明确使用合成版本记录和 IPC 测试边界，完整安装网络/哈希另行记录。
- installed-update-385.json：已安装 3.8.4 原更新器实际发现、完整下载并校验最终 3.8.5 安装包，真实运行 NSIS 升级，安装后版本确认；windows-logout-installed-3.8.5.json 与 electron-smoke.json 随后验证已安装版本的注销/重登/被踢、文件桥及 Range206。
- deployed-web-385.json：最终服务器 HTTPS 编译 Web 包实际登录，无地址覆盖，真实 IM WebSocket / WASM / SQL 用户资料一致，无未处理错误。
- installed-update.json：实际旧安装发现、正常 HTTPS 下载及哈希、运行完整 NSIS；静默参数和安装目录仅为测试隔离。
- nsis-install.json：3.8.3 基线实际静默安装到专用 E 盘目录，安装后的 EXE 通过原生运行验证。
- load-*：固定官方 SDK 的独立实例、真实令牌/WS/SQLite 数据库，100 账号分布式共同窗口测试。测试不把 API 连接或模拟协议当 SDK，最终通过状态只以完整 1800 秒报告为准。

静态负载工具 tests/build-load-bundle.mjs 生成 .cache/load-bundle，独立 Node 7778 提供固定页面，不受 Vite HMR / 应用构建影响。load-100.mjs 支持 LOAD_OFFSET、LOAD_COUNT、LOAD_RUN_ID 和 LOAD_START_FILE，子组都就绪后写同一 Unix 毫秒 startAt。Windows / Linux 浏览器分别运行，总计 100 个实例。共同 runID 防止旧测试消息污染统计；原始令牌文件仅在 .private 中，不写入报告。

跨端 Android/iOS、真实网络中断、更新完整安装与正式硬件的覆盖范围见根验收报告。源码构建、接口返回成功、合成媒体通过都不能冒充未执行的验收。

100 SDK 完整共同窗口已完成：Windows 30 + Linux 35 + Linux 35，持续至少 1800 秒，聚合发送/接收均 5680，0 断线、0 发送失败、0 JS runtime error。Windows 子组 1620 发送 / 1624 接收（跨子组环路），p50 2560ms、p95 4758ms、p99 5469ms。测试生成器同时运行构建，延迟包含其负载影响，不声明亚秒级延迟。详见根压力报告与 Windows load summary。
