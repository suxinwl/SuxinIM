# 当前验收记录

本目录为基于固定官方版本与原企业源码合并的开发版本。只有全部约定用例通过，才能标记升级完成。当前尚未作此标记。

2026-11-02已恢复语音、视频、通话和会议开发与测试。Windows3.8.7/380701已发布到独立内网下载页；Android114仅提供本地正常main候选，媒体状态NOT_ACCEPTED（未通过验收），不发布Android下载或更新，正式current包仍标记present:false且文件不存在。163项Dart、108项Java边界测试及62个Java文件的实际Android编译通过，不能替代真实JNI、AVD媒体或ARM64设备验收。以下暂停和113旧候选记录保留其历史范围。

历史安排（2026-11-01）：按用户当时指示，暂时跳过视频、语音、通话与会议测试，继续其余回归和构建。未完成112改造、测试失败和原生实验已归档，见 `tests/delivery/deferred-media-work-20261101.json`；root实际核69份112副本、271份外置原生实验文件及28份入会测试证据。8个活动源文件恢复111原字节、新model和2个未完成测试移入可恢复归档，3229项运行源、SDK194、native99及62/64依赖覆盖复核通过，见 `tests/flutter/112-restored-111-verification.json`。113以该基线仅更新pubspec和iOS版本元数据，42项非媒体回归通过，x64、ARM64正常main候选已构建并完成签名、版本、ABI及原生依赖核验；两包仅保存在本地候选目录，不发布到更新服务。跳过不等于通过，相关功能仍保留在源码中，以下历史失败不抹除。

| 当前交付部分 | 已取得证据 | 尚需完成 |
|---|---|---|
| Linux 服务端与企业服务 | 固定版本构建、隔离运行、100个真实SDK共同在线30分钟 | 不含真实业务数据库迁移 |
| Web、Windows 3.8.7 | 94项适配及TypeScript通过；真实合成引用转发5人后会话/历史可用；搜索5原UI+10Store、视频9行为+原生MP4和正常TLS通过 | 最终NSIS及对应打包程序TLS/SDK/受管Range巡检通过；Windows未代码签名，真实覆盖安装和实际硬件效果按本轮报告逐项记录 |
| Android 3.8.14+114 本地开发候选 | 163项Dart、108项Java边界及62个Java文件实际Android编译通过；正常main候选；单端快速会议普通Leave/重入、End后Ended禁入通过限定原界面验证 | 媒体NOT_ACCEPTED：三人通话退出121023毫秒失败，后续17项物理任务最终完成且摄像头为空；预约、纯语音、完整跨端矩阵及真机未验收；不发布 |
| 历史Web、Windows 3.8.6 | 原界面运行；本轮57/57适配测试、真实托盘已读/工作圈、合成截图GUI与ASAR、最终打包启动巡检通过；此前安装升级证据按原版本保留 | Windows包未代码签名；物理媒体效果另验 |
| Android 3.8.9+109 | 123项Dart测试、完整输入冻结下ARM64正常主程序构建、签名及原生载荷来源核验；x64正常main原登录、跨端群已读0/2→1/1→2/0、真实断网及两tab恢复重试 | ARM转译启动触发SIGILL；连续通话及清理、群通话/会议/共享、系统安装升级及物理ARM运行仍未通过；尚未发布 |
| Android 3.8.10+110 | 134项Dart/88项Java及Android类路径编译；私有x64正常main缓存、API撤销及失效冷启；两轮单聊视频、预约会议、三方原邀请/主持权限/双向共享和最后结束有实际证据 | 同进程重登按钮缺陷、群退出72.313秒失败、首次会议中断、详情结束状态缺口仍保留；未发布，物理ARM与安装更新未验收 |
| Android 3.8.11+111 | 138项Dart、94项Java及Android类路径编译；同19目标0错误、103 warning、598 info；3229+SDK194和99 native输入独立核对，5项批准修改；私有x64正常main构建验签，原界面会话撤销后同进程重新登录成功，新会话三接口有效 | 本轮未媒体测试，未声称修复群原生等待；物理ARM及安装更新仍待验证；112未完成改造已归档；113结果单列，不倒用于111 |
| Android 3.8.13+113 本地候选 | 42项非媒体测试；3229应用、194 SDK、1041测试、99原生输入前后匹配；正常lib/main.dart的x64和ARM64签名候选构建完成，分别11/13原生载荷；独立源码复核通过 | 原专用AVD保数据覆盖安装、设备APK哈希、缓存首页、原关于页版本和无更新提示已验证；一次SystemUI无响应提示保留；未执行媒体或Java全套重跑，物理ARM、普通手机CA信任和系统安装更新仍未验；未发布 |
| iOS | 固定核心源码、依赖与Mac构建说明 | 未执行Xcode构建或真机验收，按本期交付边界单列 |

源码指纹范围更正：已核对的103至107历史快照采用固定目录清单，漏纳实际本地依赖 `openim_working_circle`。这些快照及“零漂移”只证明当时清单内的输入，不覆盖整个应用的全部运行源码。历史文件保持原字节；108已补全本地path依赖覆盖，并单独记录52项覆盖新增，不能将其当作产品新增源码或把新范围倒用于旧包。

108首轮3224项清单另漏记5项真实`com/example` Android插件源码，原因是按任意深度的`example`目录名排除。其中1项已有构建前兼容补丁逐SHA锁，另4项在首轮构建期间才补锁，不能追溯称其全部输入在首轮构建前已冻结。已只更正元数据为3229项，原3224项零变化；独立重新清点默认源码、声明资源和字体无遗漏，见 tests/delivery/android-108-path-source-coverage-resolution-review.json。首轮包及原清单保留；更正范围下另做的同命令确认构建实际通过，3229项与SDK194项前后匹配，APK与首轮字节相同，见 android-108-confirmed-source-boundary-review.json。109另在完整更正范围下独立构建ARM64候选，具体记录如下，不将109证据倒用于旧包。

## 2026-11-02 Windows 3.8.7 本轮验收

当前包：`clients/web-desktop/release/OpenIM38-Enterprise-3.8.7-win-x64.exe`，构建号380701，160966886字节，SHA-256 `422bf047b6cfec8c916614aa30464f737ebfb4062d3020b7ae70fbeca1afc873`。测试包未做Windows代码签名；安装前完全退出旧客户端，包括托盘。

- 最终NSIS构建通过，对应打包程序的正常TLS、SDK初始化及受管文件Range巡检通过，见tests/delivery/windows-387-native-smoke.json；不将构建巡检等同于用户电脑已完成覆盖安装。
- 94项适配测试及TypeScript检查通过，见tests/delivery/windows-387-final-adapter-tests.log与windows-387-final-typecheck.log。
- 原3.8.6真实Electron引用转发在固定WASM中复现退出；新适配保留引用目标，经5名合成接收者转发后历史及自信息查询有效。普通文字/媒体保留原SDK路径；批次有序等待，失败接收者可重试，见windows-387-quote-facade-review.json及本轮原生报告。
- 综合搜索和聊天记录页5项原UI结果、10项真实Store行为回归通过，覆盖结果列表、消息上下文定位、失败重试及过期响应，见windows-387-search-review.json。
- 视频下载9项行为测试通过；真实生产下载函数/主进程在隐藏Electron窗口中用合成MP4完成200下载、SHA一致、受管Range206和真实视频解码；正常证书校验下载隔离服务器文件通过，见windows-387-video-download-review.json。该证据不等于所有旧附件、硬件或原聊天窗口操作均已逐项验收。
- Android114实际原界面媒体状态为NOT_ACCEPTED，具体失败及有限通过范围见下一节；原失败及113候选证据不覆盖写。

## 2026-11-02 Android114 媒体恢复结果：NOT_ACCEPTED

- 源码与构建：完整163项Dart、108项Java边界测试通过；62个Java文件的实际Android编译通过。正常lib/main.dart的x64候选已构建、验签并在专用AVD运行；ARM64候选与真实校验值见README和使用说明中的候选表。两者仅用于本地开发，正式current包仍标记present:false且文件不存在，不进入下载页或更新发布记录。
- 已通过的限定原界面操作：单端快速会议普通Leave后可重入；主持End后详情状态为Ended，再次进入被拒绝。这些结果不能扩展为多人媒体、预约会议或物理设备通过。
- 实际三人通话退出失败：Room ownedResources清理121023毫秒后报FAILED；后续观察确认17个已排队物理任务全部完成，最终摄像头为空。晚完成说明本轮没有证据证明永久资源泄漏，不会把已发生的退出超时判为通过。
- 唯一早期定向诊断在新track收到STOP_RTC_STATS_UNAVAILABLE后、挂断前捕获，不是第二轮正常验收。Stats、FrameMailbox、Media工作线程当时等待队列；存在未符号化native调用，发布库缺少完整C++符号，不能据偏移给具体函数或断定根因。UNAVAILABLE也可由已有peer关闭/隔离状态直接拒绝，不能当作此刻新增阻塞证明。
- 未完成：预约会议、纯语音通话、完整Web/Windows/Android交叉矩阵、连续正常通话与退出、ARM64真机媒体、普通手机证书信任及系统安装升级。iOS仍仅交付源码和Mac/Xcode说明。

本轮汇总见tests/delivery/android-114-media-resume-review.json。详细证据位于tests/delivery/media-resume-20261102/：source-freeze-114.json、android-114-single-quick-first-leave.json、android-114-single-quick-leave-confirm-leave.json、android-114-single-quick-end-confirm-leave.json、android-114-group-first-confirmed-leave.json、android-114-group-first-physical-composition-review.json、android-114-directed-group-native-result.json。早期公开栈为tests/flutter/android-114-directed-group-native-backtrace.txt。历史失败及暂停归档保持原字节。

## 2026-11-02 Windows 3.8.6 历史验收

历史包：`clients/web-desktop/release/OpenIM38-Enterprise-3.8.6-win-x64.exe`，构建号380601，160967316字节，SHA-256 `b529a90986d4c70176922e3965c1c88d6ef69432743e71a09f7f991a7e0018c3`。测试包未做Windows代码签名；安装前完全退出旧客户端，包括托盘。

- 57/57适配测试通过，见 `tests/delivery/windows-386-final-adapter-tests.log`。
- 真实Electron验证关闭到托盘、隐藏/最小化或失焦时不把后续新消息误报已读，重新查看后上报。旧服务端已读不回退，验证只针对新消息，见 `windows-386-read-runtime-review.json`。
- 真实工作圈原Modal验证本人点赞/取消、新增/删除评论即时显示，失败保留草稿，其他账号事件刷新及无事件轮询恢复；取消/删除实测5062ms，不承诺严格5秒内。原加载截图封面停用且原资源保留，见 `windows-386-moments-review.json` 和 `windows-386-moments-ui/report.json`。
- 合成截图GUI、截图结束状态、打包ASAR及最终安装包对应的启动巡检通过，见 `windows-386-screenshot-fix-review-final.json`、`windows-386-screenshot-packaging-review.json`、`windows-386-native-smoke.json`；不扩大为全部硬件组合的截图实测。
- 一次来源未确定的Object类型页面错误保留于 `windows-386-read-object-error-diagnostic-review.json`，后续相同闭环重测0错误；不宣称已定位或修复所有页面错误。

以上3.8.6历史JSON均在 `tests/delivery` 下。当时仅Windows/Web修复，Android113为本地候选、媒体暂停；本轮已恢复媒体开发。没有将这些旧证据改写为3.8.7或114通过，历史3.8.5运行报告同样保持原字节与原版本。

## 已有证据

- Linux服务端、独立企业服务及固定源码LiveKit已构建，独立空库测试栈运行；不含真实数据迁移。最终运行程序核验范围见100 SDK复核及二进制对齐报告。
- 原5项目基线保留，6794个已登记文件逐项重新计算SHA-256均一致，见 tests/delivery/original-baseline-preserved.json；核对未重写基线。逐页面/路由/接口静态清单列出528个页面源文件、142个路由声明、144个HTTP字面量，见 docs/provenance/ui-api-inventory.json；这些静态条目不等同于逐页运行通过。
- Web/Windows原UI构建、真实启动、正常TLS、WASM/SQLite初始化通过；Windows测试安装包未代码签名。
- 企业真实HTTPS业务6组、安全与更新6组通过，Mongo CAS在临时随机数据库真实通过。
- 原后台19步真实浏览器巡检无JS/请求错误，包括用户关系、群成员与个人资料入口。管理员修改密码4组真实表单测试通过，测试密码已恢复，资料接口不返回密码哈希。附件分片、下载SHA、三个Origin（含仅附件null读取）的TLS/CORS/CORP测试通过。
- 合成SMTP公共注册与找回密码3组通过；错误验证码拒绝、旧Chat/IM/企业/会议访问撤销、新密码/无userID改密均通过。
- Web/Windows真实文字、文件、PNG和MP4传输，Windows视频解码；位置/名片/引用/合并/转发、OA专页、群已读、工作圈、标签及原呼叫UI有客户端运行证据。
- Seq客户端回归通过：Web与Windows未读均5→0，断网补齐3条消息，两端8条历史消息ID一致，撤回界面同步；禁言成员接任群主后roleLevel=100、解除禁言，收到真实1507通知，非法入群来源被拒绝。见客户端 seq-client.json。
- Android103私有x86_64探针的真实Go核心已在专用API30模拟器接入TLS/API/WebSocket并同步消息，13阶段通过，12条消息场景发送至Web原界面、五项媒体下载SHA通过，涵盖10种主消息类型，含保留子类型的custom110；OA1400/session4另列。见 tests/flutter/native-sdk-report-103-passed.json。此环境将独立CA安装到模拟器系统信任库；固定Go AAR不采用应用内CA或SSL_CERT_FILE，不能据此声称普通未Root手机仅安装用户CA即可连接。
- Android104修复源已冻结：3191个真实运行输入，集合SHA 892be9e47074594bfb331a9fa347bcbda07852bbc286b3283bc116608ab9a8c4。已排除Kotlin会话文件及R8使用报告等生成输出，保留原始快照和归一化说明，保留输入的变更/新增均为0。26项Flutter测试通过，其中独立编写的真实层级故障注入11项通过；分析0错误，166条warning和1065条info另行保留，40文件最终源审无阻断。见 tests/flutter/runtime-source-snapshot-104.json、runtime-source-normalization-104.json、cleanup-source-review-104.json、rtc-cleanup-independent-review-104.json。源码结果不代替104安装包运行验收。
- Android105候选源码已冻结：最终完整Dart测试59项通过，分析0错误、99条warning和580条info，见 tests/flutter/unit-tests-105-final.log、analyze-105-final.log。包括独立登录/铃声18项、真实RTCVideoRenderer方法通道9项及实际LiveKit渲染Widget3项；各测试边界见 session-ringtone-review-105.json、renderer-dart-widget-review-105.json。原生Java边界测试及实际Android/Flutter/WebRTC依赖下的Javac编译通过，最终43项源码与证据哈希逐一匹配，见 native-renderer-review-105.json、native-final-boundary-review-105.json。已覆盖物理清理顺序、超时晚完成、共享源、失败后不重复JNI、关闭后晚返回空流等；这些源审和模拟原生边界测试不代替安装包、真实JNI或设备验收。
- 105私有x86_64验证包已构建，3.8.5/105、min24/target35及项目签名核对通过。归一化源码快照保留3169项，集合SHA a7b185469e361a8679ee886ecf36c578a765e8cbccf10ffa1c673bb2a2a332c8；原3201项快照另存，只剔除32项.cxx生成缓存，构建前后保留输入变更与新增均为0，见 tests/flutter/runtime-source-snapshot-105.json、runtime-source-normalization-105.json。实际通话未通过的结果见下方，未据此发布正式ARM64包。
- Android106完整Dart回归93项通过，分析0错误、99条warning及585条info，见 tests/flutter/unit-tests-106-final.log、analyze-106-final.log；初次失败的两处旧测试夹具及定向适配14项的证据保留在 fixture-compatibility-review-106.json。新增34项独立测试覆盖真实Room/Engine生命周期、调用实际EnterpriseManager的挂断任务、回环正常TLS WebSocket及Signal连接代次，见 rtc-future-integration-review-106.json；测试中原生/网络替身的范围逐项说明。Java边界套件69项和真实Android/Flutter/WebRTC类路径下的编译亦通过，见 native-close-review-106.json、native-java-106-final.log。冻结源码3171项，集合SHA 0d9cf0c7454831863c5a85e371b92a3d564b59c03b1db5db930dd66e48a406e0。源码测试不作为106安装包或真实JNI运行通过的结论。
- Windows 3.8.3→3.8.4实际安装升级通过：原界面发现可选更新，主进程正常TLS下载并校验大小/SHA/PE，NSIS安装后核对版本。3.8.4随后发现官方Go WASM退出时误调用window.close的兼容缺口，已在3.8.5项目适配层修正并通过原生回归，3.8.4发布记录及下载包已撤回；安装流程证据与包本身的功能验收分别记录。
- Windows应用3.8.5/380501最终安装包已正常TLS下载并校验，真实NSIS从已安装3.8.4升级后核对版本。安装后再次通过注销、同进程重登、SQLite重新打开、真实发送、同PC被踢、旧双令牌拒绝、新会话有效及文件200/Range206。对应报告为 tests/web-desktop/installed-update-385.json、windows-logout-installed-3.8.5.json。最终安装包未作Windows代码签名。
- 前轮100个真实WASM SDK，共同在线1805.046秒，5680条发送/5680条接收，0发送失败、0断线、0运行错误。分区P95为Windows30端4758ms、Linux35端1127ms/1317ms；保留各分区指标，不平均成虚构的总P95，见 tests/load/summary.json。此轮早于后续定向补丁。
- 前轮104个有效资源采样中，业务栈峰值约1.47GiB、CPU峰值134.04%（100%代表一个逻辑核）；已剔除负载生成器和合成SMTP。主机最低可用约19.64GiB，见 tests/load/resource-summary.json。
- 全部服务端补丁部署后已完成第二轮100个真实WASM SDK持续测试：Linux50+50两分区，共同在线1805.750秒，5700条发送/5700条接收，0发送失败、0断线、0运行错误。分区P95分别1421ms、1380ms；24个业务容器ID及18个程序的挂载/运行哈希在前后均一致，并符合本轮固定版本锁。独立重算通过，见 tests/load-final/final-100-20261101a/analysis.json；不作为最大吞吐、真实手机或音视频并发容量结论。
- 第二轮共同窗口内105个资源采样，业务栈内存峰值约1.492GiB、CPU峰值122.32%（一个逻辑核为100%）；Linux负载生成器单列，峰值约7.397GiB。宿主最低可用约17.919GiB，最大采样间隔17.24秒，见 tests/load-final/final-100-20261101a/resource-summary.json。已停止本轮生成器，业务栈保持运行。
- Android106源码回归期间再次只读核验，24个隔离容器中的18个业务程序均运行，挂载与实际进程SHA-256均匹配本地交付二进制，未改动服务器，见 tests/delivery/binary-alignment-106.json。该核对不代替设备或新一轮负载测试。
- admin-api的9组验证覆盖配置/重启封锁、登录别名脱敏、两种在线状态凭据清除及附件完整性。
- 四类可选原生Webhook真实通过：原界面SDK上线/关闭、真实单聊已读、接收方离线时官方消息触发，与企业库接收记录逐项匹配，见 tests/webhooks/summary.json。未配置第三方推送供应商，不声称手机系统推送送达。配置恢复为默认关闭另行记录；群快照必需三个回调保留。
- 精确会话撤销的真实验证通过：旧Chat/IM/企业与媒体权限失效，旧WebSocket关闭；同平台新会话、其他平台和重复退出的隔离由定向测试覆盖。LiveKit实际双端连接、5秒断网恢复、突然关闭后约57秒对账结束、旧媒体令牌拒绝及再次呼叫均通过，见 tests/media/lifecycle.json。
- 原会议/群通话界面6组通过：预约日程、三位参与者的视频解码、主持权限、发布并由远端解码screen_share、结束后旧令牌拒绝、三人群视频中发起人离开后其余两人继续。见 tests/web-desktop/meeting-client.json。摄像头、麦克风及共享画面为浏览器合成测试源；不作为物理设备或真实Windows桌面采集效果结论。
- Webhook去重补丁已部署，Linux上的真实Mongo CAS及无操作ID的不同已读序列用例通过；同一合成用户的后续真实Seqs=4回调已保存并匹配。临时四事件配置已关闭，必要三个群回调保持开启，见 tests/webhooks/window-restored.json。补丁部署后6组企业API、9组后台API回归通过。
- 固定 LiveKit 源码补丁已用独立 Go1.26.9 在 Linux 离线构建，10 项定向媒体策略/补丁测试通过。UDP23478、TLS25349、TCP23478 分别强制 relay，两个真实浏览器均解码远端模拟视频，四条发布/订阅链路成功且双向字节非零，错误列表为空。最终容器仅有原四个媒体映射，UDP27900–28155 不发布主机端口。见 tests/media/relay-udp.json、relay-tls.json、relay-tcp.json、turn-final.json；不作为100人同时音视频或真实移动设备媒体效果结论。
- 隔离 live 备份恢复演练通过：3 个 MongoDB 库逐集合 validate，恢复行数与导出前后记录一致；54 个 MinIO 文件逐字节 SHA-256 一致。临时恢复容器无网络、无主机端口且已清理，源服务未停止；6 个备份工具测试全部通过。见 tests/backup/restore-report.json。live 逻辑包不代表同一时点快照，未包含 Redis/Kafka/etcd，也不作为整个业务栈零丢失回退结论。

前轮与第二轮负载报告分别保留，测试拓扑和延迟指标不混合。第二轮是在精确退出、TURN及Webhook补丁部署后的当前18个业务程序上运行，版本由本轮前后运行哈希证明；测试不覆盖未来重建的其他哈希。客户端原始报告保留于 clients/web-desktop/test-results，正式交付证据归档到 tests/web-desktop；构建物变化后以版本清单和对应报告为准。


113最终候选、构建/局部运行证据与可变元数据归档的冻结入口为 `tests/flutter/final-nonmedia-delivery-113-lock.json`；root另核29份运行引用并实际查看首页、关于页及更新提示三图，见 `tests/delivery/android-113-nonmedia-root-runtime-review.json`。两个候选的SHA-256见 `artifacts/android-candidates/SHA256SUMS.txt`。这些范围不构成整体升级、音视频或真机验收通过。

## 历次验收记录与未完成项

- Android108已通过112项完整Dart回归，其中22项定向测试包含在完整套件内，不重复相加；18个分析目标为0错误、103条warning和597条info，未达到clean lint。88项Java边界回归及真实Android类路径Javac实际运行通过；95项原生输入前后与当前哈希一致，见 native-deadline-review-108.json 及 independent-final/root-hash 复核。定向测试采用真实生产类/Widget和受控JNI、EGL、MethodChannel替身，不能替代设备运行。19项既有输入修改、1项新采集辅助代码和57项补覆盖输入（52工作圈、5Android）分别记录；更正后3229项当前源码集合SHA b7f9b666b9b982bd10b0f457d6b802ccde042cad5b0aac57253ffc6036aafcaf，另核194项固定SDK源码、62个resolved本地包根及64条path声明。46个兼容包的124项修改逐SHA匹配。完整范围确认构建已通过，私有x86_64包82,520,888字节、SHA 34f0fb2bbf605930e02c9409ed76ab214eff1b26f946cafc1946893e92f81626，版本、包名、v2签名、min24/target35及11个原生库载荷分别核对。见 android-validation-x64-108.json、tests/delivery/android-108-confirmed-source-boundary-review.json。原UI连续通话、群/会议/共享和正式ARM安装升级仍待完成；不发布Android包。
- Android108首通原UI采样窗口153.9秒，Web真正远端视频 decodedFrames 34→71→116→206→375；Android主远端画面ROI在四个相邻采样间隔中三次变化，60/90秒两张ROI相同，不能据此证明连续播放或判定冻结。Android原生接收统计不可用。挂断后a108c1按真实SDK消息ID送达，两PC关闭509/348ms、摄像头在28.57秒采样时已释放，没有新ANR或Unhandled。但完整日志有3次调用方deadline、3条callerError及18条caught cleanup传播，共24条事件，并非24个根因。两renderer.detach实际约13秒，local.track.release实际约5.95秒；所有超时对应物理任务随后成功且soft barrier清零，仍不能判定本轮清理成功。见 android-108-call1-log-event-review.json、hangup.json、remote-pixel-review.json；108不发布，后续同进程再通话仅先作诊断。
- 108后续短诊断在同PID4997中实际重新请求摄像头、麦克风，并在Web收到新视频轨TR_VC6gxEZkEiNdJT、解码帧50→101，证明晚物理完成后可以重新采集视频，未验证可闻音频。挂断又出现相同3次deadline及18条清理传播；本轮在解绑已启动超过3秒时主动发送一次SIGQUIT取栈，明确属于受扰诊断，不计正常第二通、性能通过或自然ANR。栈确认两媒体worker停在nativeRemoveSink，EGL线程当时空闲，原生worker内部具体函数仍待符号核对。见 android-108-recapture-diagnostic-report.json、native-detach-timing-review-108.json及dart-cleanup-deadline-review-108.json。不通过增大超时或丢弃物理屏障使验收变绿。
- Android108原工作圈发布android108-circle-select102，指定102可见：Web102原页面显示、103原feed及DOM排除，102点赞评论后Android原下拉刷新显示，见 tests/web-desktop/android-collaboration-social-ready-108.json；仅这一种范围通过。Android108原群消息a108gr1在SDK中真实存在、seq2，原已读详情却报Group message is not available locally。已确认AppIm将findMessageList返回误读为searchResultItems，应读取固定SDK的findResultItems；109适配和详情错误处理回归中。服务端get_group_message_read_num同时接受clientMsgIDs及clientMsgIDList，旧hydrate请求键不是已证明的缺陷，不能将其更名冒充修复。后续109工作树与108冻结实物分开记录。
- 后续108原UI已补齐朋友圈公开、仅自己、排除102三种发布：Web102/103原feed与DOM六组正反可见均符合设置。与上面的指定102合计四种范围均有原界面运行证据；评论点赞仅在指定102场景验证，没有重复计数。见 tests/web-desktop/android108-circle-scope-review.json；root另核18份引用文件的SHA与大小，见 tests/delivery/android-108-social-root-runtime-review.json。这些是108实物的非媒体证据，不能替代109/ARM64运行验证。
- Android109修复两处findResultItems映射，群已读页新增真实失败/重试、未知人数--、关闭后late回调防写，并以每tab pending加一个dirty位合并加载期间的已读事件。真实SDK/MethodChannel/回环企业HTTP及Controller/Page定向11项通过，已包含在完整123项中，不相加。主18目标分析0错误、103条warning及597条info，exit2、cleanLint=false；定向9目标分析单列，不能与主范围叠加。3229项冻结集合SHA 7c0c75520658c61bcd6e25a93b8e5d3eb266976a22edb39bfa5557572277d55e，相比完整108仅8项修改、无新增/删除，含6项群读代码/本地化和2项版本。95项native输入及实际108成功日志逐SHA一致后复用88项Java/Javac证据，没有重跑。见 final-verification-109-before-build.json、group-read-adapter-review-109.json、group-read-independent-final-review-109.json及tests/delivery/android-109-source-boundary-review.json。
- 109私有ARM64正常lib/main.dart实际构建成功，APK 76,677,605字节、SHA 141ef706479719a644a6676d244ec85412e5c1fb09af3df16d3a6d30e06cd7c1，v2签名、包名、min24/target35和单一arm64-v8a核验通过，完整应用3229项及SDK194项构建前后无变化。13个原生载荷中12个依赖库来源闭合，另1个是本次main的Dart AOT；Go库与原AAR相差16字节已用同NDK标准strip在临时副本精确复现，未修改APK或AAR。见 android-main-arm64-109.json、android-main-arm64-109-payload-verification.json。正常构建输出的889项ZIP及43项可读UTF-8资源边界审查通过，见 tests/delivery/android-109-arm64-build-output-boundary.json；此审查不覆盖DEX、lib或二进制XML，不代表全包秘密审计。运行验收待完成，候选未发布；x86_64 AVD经native bridge执行ARM库也不能等同物理ARM64真机验收。
- 109 ARM安装与启动分开记录：原测试AVD保留旧包cpuAbiOverride=x86_64，普通、显式--abi和同字节非流式三次安装均返回-113；固定AOSP11安装准备路径及真实旧包字段支持旧override优先的判断，不能由此断言桥不支持ARM，见 android-109-abi-113-independent-review.json。另建空userdata的独立openim38_api30_arm109，保留原AVD全部数据；同141ef包最终安装为109/arm64-v8a且无旧override，设备APK大小与SHA匹配。检测脚本在240秒提前超时，未观察到adb Success响应，系统DEX随后完成，不能抹去该工具超时。首次启动PID7245时ndk_translation报告instruction0xd5380000不支持并SIGILL，尚未进入登录表单，没有执行登录或复制旧凭据。见 android-109-clean-arm-install-final-state.json、android-109-clean-arm-startup-failure.json。该ARM转译运行未通过，不据此推定真实ARM手机同样失败；新AVD正常停止保留现场，同109冻结源码另构建私有x64正常main继续其他原UI验收。
- ARM首个失败指令已按设备映射、APK偏移、ELF段、Go函数表与反汇编精确对应固定libgojni中的Go1.25.6 `internal/cpu.getMIDR` / `mrs x0, MIDR_EL1`；不是WebRTC清理或群已读错误。首次fatal之后的Flutter库转译错误单列。见 android-arm-translation-independent-review-109.json；root另核13份公开引用输入及实物身份，见 tests/delivery/android-109-arm64-root-runtime-review.json。未改AAR、Go或CPU feature来绕过，不把转译失败套用于真机。
- 109同源私有x86_64正常main已实际安装并从原表单登录。群消息a108gr1/seq2在同PID5130原页面完成已读0/未读2→1/1→2/0；两个Web接收端分别从原会话列表点击，自动上报真实回执，Android对应名单依次为102/103未读、102已读/103未读、102/103已读。root已实际查看六图并核16份引用的大小、SHA及实物身份，见 tests/delivery/android-109-group-read-root-runtime-review.json。该结果是三个合成成员和x86_64 AVD范围，不替代物理ARM64、音视频或大群规模验收。用户暂时没有ARM64手机，继续其他可用验证，真机项保留待验收。
- 109只在自有AVD真实断网，两个tab均显示未知人数--、失败文字及Retry；恢复网络后分别点击原Unread/Read的Retry，恢复0未读及2已读真实成员102/103。完整PID5130日志单列被捕获及未捕获事件，未用“0 Unhandled”替代整体运行通过。见 original-ui-group-read-report-109.json，root已查看失败及恢复画面并重算31处引用，见 tests/delivery/android-109-group-read-offline-root-runtime-review.json。早先歧义命名的retry-read.png是重试前失败画面，明确排除恢复证据；不修改原截图补绿。
- 正常main沿用的Bugly错误观测路径另有源码边界：parentZone转发没有同步递归，但当前isolate的self RawReceivePort把字符串堆栈当StackTrace转发，独立Dart API探针复现TypeError及有界反馈。该探针没有运行109 Flutter UI，不能据此断言应用实际吞错；也不能仅以日志没有字面Unhandled就声称全部错误可见，原生deadline及被捕获的清理失败仍需单独判定。见 normal-main-error-observation-review-109.json。修正材料在冻结源码之外准备，未应用到109；3个语义探针不计入123项Flutter回归。
- Bugly修正副本的11项真实Flutter定向测试已全部通过，子进程exit0。运行器因生成的package_config.json前后字节不同正确保留整体冻结失败；7项记录输入中6项不变，不能无原配置字节就断言仅元数据变化。当前62本地包根/64路径声明和3229+194真实源码仍与109清单匹配。见 bugly-clone-flutter-layer-package-config-review-109.json；此处不是产品已合入或109运行验证，也不与123项重复相加。
- 110随后将同一Bugly修正正式纳入实际插件，11项实际package测试包含在完整134项中；19目标分析为0错误、103条warning、598条info，exit2。4个Java文件仅补关联hash与原释放步骤begin/end，88项边界及真实Android类路径编译均重新运行通过，95项输入前后匹配，未复用旧测试代替执行。完整3229项冻结SHA b864918834729cead97147b5aa5aaaee2db52efeccb117678de148d1a2012235，SDK194项独立匹配，较109为8改/0增/0删。root源检查见 tests/delivery/android-110-source-boundary-review.json；可变aggregate元数据按当时字节另归档，避免互相引用哈希循环。
- 110私有x64正常main实际构建成功，82,455,360字节、SHA c1c321dd506c212b1a00619656ef8da8e70d46601319143f28056878fb10e4c6，v2签名028253da...、min24/target35；11个so中10依赖库逐字节同109，新AOT单列。root重新执行apksigner/aapt并读ZIP核载荷，见 tests/delivery/android-110-x64-artifact-provenance-review.json。新DEX日志标记存在不代表已执行；887项ZIP及43项可读UTF-8资源检查仅为有限边界，不是DEX/native或全包无密证明。未发布或标记运行验收完成。
- 110正常main覆盖安装保留109原登录凭据，未输入密码或注入令牌；PID4899进入原Home并在短暂后台后恢复，同进程日志记录真实SDK长连接建立、消息同步完成及心跳应答。见 tests/flutter/android-110-cached-start-report.json；这不等于本轮真实聊天送达、旧缓存撤销、ARM或音视频已通过。撤销测试工具因AVD名称属性及Windows ADB的CRCRLF行结束先后安全拒绝，两次均未发送HTTP；最小解析修正仅接受精确AVD/OK协议行，真实空行、空格或冲突名称仍拒绝，本地13项测试通过，产品源码未变。
- 随后同一停止状态的104原缓存pair在撤销前实测三API均200/err0；一次工具执行内首次及幂等退出均200/err0/revoked=true，随后企业401、IM1506、Chat20101，本地凭据原样保留。root另只读核验Nginx同窗口两次200到达记录及各报告哈希，见 tests/delivery/android-110-session-revocation-root-runtime-review.json。工具已写passed报告后因O:/E:路径别名的尾部显示异常退出1，该执行异常保留；未重复退出或改为exit0。此项只证明API撤销，旧缓存冷启和原表单重登另验。
- 110失效缓存冷启PID5527实际回原Login，本地loginCertificate已清除；20秒视频的40个2fps样本未见Home，不扩大为逐帧无闪入证明。随后原表单正确合成邮箱及受保护密码输入后，Login仍禁用，未建立新登录；模式切换暴露输入框显示与controller值不同步，生命周期根因仍待定向复现。见 tests/flutter/android-110-cache-revocation-ui-report.json；root已实际查看脱敏画面并核17份引用、两个公开日志共同前缀的7条事件SHA，见 tests/delivery/android-110-cache-revocation-ui-root-runtime-review.json。0字面Unhandled不能使该原界面失败变为通过；未清数据、注入令牌或覆盖失败现场。
- 110正常停止/重启后PID7093原表单重登成功，新pair三API有效；原About提示当前已是最新版本，见 android-110-recovery-login-about-report.json。这是失败后的进程重启恢复，不代表同进程禁用按钮已修。实际Get4.7.3、原Page/Binding/Logic定向复现重复Login后TextEditingController disposed及按钮不启用；诊断夹具异常、受污染第三case与独立进程失败分开保留，见 login-route-ownership-review-110.json，未计入134项通过数。
- 110同PID7093两轮正常视频采样154.0/153.5秒，Web真正Android远端轨道解码帧分别61→547、143→715；Android主远端ROI两轮各4个采样间隔均有像素变化。第二通设备两PC中较晚CONNECTED到shutdown为184.475秒，UI逐回调计时与媒体时间不代替真实elapsed。两次原挂断、exact文字消息送达、摄像头/远端资源释放及新ANR检查完成，完整日志无调用方deadline或清理失败；Room整体实际释放13.797/6.291秒，不表述为瞬时释放。见 android-110-call1-samples.json、android-110-call2-samples.json，以及各自的hangup、remote-pixel-review和host-summary记录；Android原生统计超时/缺失单列，采样像素不证明每瞬间双向播放、音频可闻或真实ARM性能，也未据此宣称108慢清理根因已修。
- 110原群邀请102/103、两Web原来电接听及三路视频实际运行；首轮45秒迟到检查单列为夹具时序未验收。Android短暂Home后11秒采样远端空白，后续11:44:39采样已自行恢复，准确恢复时刻未采；没有证据认定永久失订阅或Surface损坏。Android原离开后其余两Web持续互收视频/音频，再由102、103依次原UI离开，最终Web资源清零。Web侧39份输入及跨端14份输入的复核分别见 tests/web-desktop/android110-group-rtc/web-group-media-review-110.json、tests/flutter/android-110-group-cross-runtime-review.json；音频传输不等于真机可闻验收。
- 该群通话的Android退出清理未通过：前两个job几毫秒即启动，removeSink wrapper实际58.009/58.034秒，第三job先排队58.121秒、执行10.721秒，local setEnabled约10.732秒，cameraClosed仅404ms。Room整体72.313秒失败，日志44项记录是6个deadline、7条callerError和31条上层传播，不是44个独立故障；摄像头47秒后仍活动，最后物理释放不能改为通过。两个PC close在全部detach及local资源结束后才开始，实际804/613ms，不能将UTC秒字段误读为49秒耗时或据此声称并发锁循环。群周边600秒121个宿主采样均值24.02%、峰31.91%，最低可用1.905GiB，不支持宿主整体CPU耗尽归因；guest和原生等待原因继续诊断。原生统计早于Home已停止，不能将缺少统计当作该次画面停止证据。源/日志审查见 android-110-group-resume-source-review.json，后续诊断不以加大预算或丢弃物理屏障来冒充修复。
- 帧交接的外置V2实验实际通过14项纯Java检查，并用同一新增用例复现外置V1在工作线程中断后只释放2/10帧、8项排队工作无人接管的失败。V2仍为两个固定worker及有界队列，不补线程或让调用线程执行；111/112产品未合入。见 tests/flutter/frame-handoff-v2-experiment-review-111.json 及 diagnostics/110/frame-handoff-candidate-v2/independent-execution-evidence-review.json。这些检查使用假帧计数和受控门闩，native-detach/drain次序只是Future组合模型；没有接入真实Binding、TrackLease、VideoFrame或EGL。helper drain不能证明EglRenderer内部pending帧已清空，不能把14项加到正式Dart/Java套件或宣称58秒等待已修。
- 110原预约会议创建android110-scheduled，退出会议路由后重新进入仍读回同一会议；原修改入口将30分钟改为60分钟，退出重入保留修改；原取消后再次进入列表为空。见 tests/flutter/android-110-scheduled-meeting-runtime-report.json；root实际查看重入、修改及取消后三图并核17份引用和实物身份，见 tests/delivery/android-110-scheduled-meeting-root-runtime-review.json。没有进入这场预约会议，不将此项扩大为媒体、邀请或时区验收。即时会议另有中断现场，运行验证继续。
- 110原即时会议发出两个真实905邀请，两Web原卡片加入；Android主持原设置的禁自行开麦、禁自行开视频、仅主持共享均由Web原控件拒绝，恢复后媒体继续。Android共享被两Web解码，Web102反向共享在Android两原截图的形状和画内1061→1886计数变化；最终原二次OK后Room资源1651ms、removeSink47ms、两PC关闭298/182ms，摄像头为空、新trace为0且未发SIGQUIT。两Web原结束提示与物理连接关闭保留，未点击原提示确定后正常关闭测试助手。root查看四张PNG并核两端报告151处引用（123个唯一文件），见 tests/delivery/android-110-meeting-root-runtime-review.json；不把这些限定成功抹去首次subscriber协商超时、重连后音频发布超时和原callbackFailure。
- 原结束后的Android详情仍显示Started/EnterMeeting，未刷新或尝试重入；源逻辑只比较开始时间，页面没有Ended分支。这是已知UI状态缺口，不代表服务器仍允许加入。本轮没有预先私存真实旧RTC JWT，因此不声称已做此次结束后的旧令牌重放；既有独立Web会议验证另列。两次/proc诊断采样均为0样本读取超时，工具exit0不当作采样通过；只读观察窗口不当作无扰动性能测试。63份Android引用与88份Web引用及首次日志分别保存，日志环形覆盖不能将末尾快照当整个会议全程。
- Android项目签名输入已实际复制到源码目录外的 E:\OpenIM38-PrivateBackups 独立子目录，副本及私有清单均核验仅当前用户与SYSTEM访问；两个源文件前后哈希与副本一致。见 tests/delivery/android-signing-backup-review.json、tools/backup-android-signing.ps1。该备份位于同一E盘，不代替异机、异盘或离线备份，未将任何密钥或口令写入公开报告。
- 111仅修改实际AppNavigator重复Login保护、Camera状态volatile、正式Java测试runner及两项版本，共5项运行输入修改、无增删。完整138项Flutter含4个真实Login时序回归，94项Java含6个真实Camera类行为用例；实际Android类路径编译通过，99 native与1039 Dart测试/解析输入前后同。19目标分析仍0error/103warning/598info、exit2。root独立重算3229项应用及SDK194、核62个本地root/64路径边，源SHA d4420474895e674163dcff90c3c40565dc27c6a5b295fd3a789a7e1509ef65de；见 tests/delivery/android-111-source-boundary-review.json。首次普通E盘长路径调用误漏6项，显式扩展路径逐字同原冻结，O盘短路径同脚本实际verify通过；初失败及恢复另存 android-111-long-path-verification-review.json，未改冻结表或缩范围。
- 111私有x64正常lib/main.dart实际构建成功，82,455,356字节、SHA 78d6fd1d4550fcf3e0bf19741fb892970d86429ec1c092e6e0a888e25f9b5274；root实际apksigner/aapt确认111/3.8.11、v2项目签名、min24/target35及唯一x86_64，ZIP11个so中10依赖同110、新AOT单列。887项ZIP/43项UTF-8资源有限检查0flags；见 android-111-x64-artifact-provenance-review.json。另由实际DEX字段0x42确认state的volatile位，见 android-111-camera-volatile-dex-root-review.json；不扩为物理相机或58秒问题的修复证明。当前包未发布，ARM后续验收另列。
- 111原界面运行回归通过限定登录范围：先保留数据安装，cached Home经原SystemUI Wait后显示，同PID4864后台返回；从实际用户数据重获的同一令牌对三接口有效，再force-stop后唯一整次工具执行含两个幂等logout，旧Chat20101、IM1506及企业401/1501拒绝。冷启原Login与原表单提交后的Home同PID5368，新令牌三接口均200/0，不依赖重启、清数据或注入令牌。应用删除的是持久化登录凭证，不是开发CA。40个非均匀PNG样本覆盖0.002至20.341秒，拼图未见Home，不声称连续视频或全帧无闪现。原关于页3.8.11+111与“已是最新版本”提示可见，不包含下载或系统安装流程。root实际核102份引用、26条日志索引、运行后3229+194完整源码并看四张公开图，见 tests/delivery/android-111-cache-revocation-ui-root-runtime-review.json。首cached启动观察到一次SystemUI ANR弹窗；保留窗口内own ANR为0和系统日志缺少事件不能抹去弹窗，也不是整机历史无ANR证明。本轮没有111媒体验收，不覆盖110的群退出或首次会议失败。
- 原生观测没有新增锁、线程、JNI调用或改预算/物理屏障；remote removeSink前后两条日志随原调用处于既有nativeCall物理串行区，属于明确限定例外，不能表述为没有任何持锁日志。identityHashCode可能碰撞，只是Java对象关联线索；wrapper计时包含remove/free及日志开销，不等于纯JNI。另有CameraEventsHandler原状态字段跨线程可见性缺口，最小volatile建议在诊断目录准备，未合入110，不将它称为13秒延迟原因。
- 合成账号凭据误入专用AVD浏览器草稿及一次工具截图的事件已按110账号范围完成旧密码拒绝、新登录和本地/远端配置更新，独立管理员未变；公开文本和私有证据边界及因果限制见 docs/testing-credential-incident-109.md，不以此代替旧缓存会话撤销或功能验收。
- Android107已完成102项完整Dart回归；扩展到17个分析目标后为0错误、99条warning、592条info，新增just_audio目标的范围记录在 analysis-scope-107.json，不直接与旧范围比较。69项Java边界证据经43项源码/脚本/日志哈希与106逐项一致后复用，没有重复运行，见 native-verification-reuse-107.json。当时清单内3171项输入仅5项相对106改变，无新增或删除，集合SHA 794303156707db657c284ca0b9bfd83b1d1cbd697e6e6e57d4a43e003f30536d；该范围漏项见上方更正。新播放器测试由完整suite与独审单独绑定。107私有x86_64包已冻结构建，签名、min24/target35及版本核对通过，构建后清单内输入零漂移，见 android-validation-x64-107.json。
- 107的两通原界面150秒视频均已运行，但整体未通过。首通Android发往Web的真实远端帧40→602，Android独立远端ROI的相邻样本均变化；两项均按采样证据记录。挂断后文字送达、两PC关闭成功、摄像头最终释放且没有新ANR或未捕获异常，但完整日志还包含25条被捕获的渲染/上层清理失败，本地轨道一次清理约12.3秒后报错，因此不能判定清理成功。第二通Android接收远端视频，但Web没有收到Android视频，本地预览黑屏。随后原Camera On诊断明确返回“Local media cleanup is still pending”，并暴露摄像头按钮未观察的异步错误。源码确认调用方超时会永久隔离共享媒体owner，即使物理任务随后成功也不恢复；最早隔离时点和全部渲染失败原因尚未由107日志确定。见 original-ui-rtc-report-107.json、native-local-capture-barrier-review-107.json 及两通 remote-pixel-review。107不发布，108修复与完整连续通话复验进行中。
- Android106私有x86_64包已冻结构建，82,520,892字节，SHA 3a085a044fbbb306b07e951943ec0a1e55752eea14649aa2cb32e58d7c0bbee3；3171项输入构建后零漂移，见 tests/flutter/android-validation-x64-106.json。首轮150秒原界面通话中，Web真正远端视频帧31→436；Android远端独立ROI在30/60/90/120/150秒相邻样本均有像素变化，排除本地预览与所有UI，见 android-106-call1-remote-pixel-review.json。该证据只证明采样时点的画面更新，不作为FPS或持续帧计数；原生统计只得首份23帧，后续统计停顿另行核查。
- 106首轮挂断后完整a106c1文字实际送达，摄像头及Web轨道释放，无新输入ANR。两次nativeClose分别2.082秒、0.627秒，Room总清理15.641秒（主要本地轨道释放），未复现105的关闭超时、closed Socket或挂断Dio未捕获异常。但just_audio出现一条PlatformException(abort, Loading interrupted)未观测错误，因此106仍不通过，不继续发布；第二轮长通话、原UI群通话/会议/共享/主持和系统安装升级待修复后验证。
- 105私有x86_64包已运行首轮150秒原UI视频，远端帧持续递增，挂断后的文字实际送达，摄像头释放；按PID/时间窗口未发现新ANR。但有2项closed SocketException、1项真实原生关闭超时和1项挂断信令超时的未捕获异常，因此不能判定通过。见 tests/flutter/original-ui-rtc-report-105.json、native-close-diagnosis-105.json、rtc-future-review-105.json。已确认异步信令和事件清理的错误传播缺口，以及关闭时统计请求可能不结束；底层关闭迟缓的具体等待点仍在采集线程证据，105不发布。
- Android ARM64主程序3.8.4+102已构建并完成签名验证，但未发布：同源x86_64原main虽然通过邮箱登录、OA专页及文字发送，视频曾出现主线程阻塞于PeerConnection.getStats(sender)的ANR。103私有x86_64候选的原UI两轮长通话未复现该统计阻塞：首通持续4分42秒，连续采样中远端解码帧164→872、统计线程固定2；第二通150秒内帧0→577。实际挂断后回到消息页，首通摄像头约27.497秒释放、两PC约28.741秒完成关闭，第二通在14.59秒后的采样已无活动摄像头。103另有Wakelock平台接口不兼容异常，不能据此认定完整RTC通过。3.8.3+100已撤回，101为含启动强更缺口的构建候选。编译、签名及模拟器通过的部分不能替代完整主程序或ARM64真机验收。
- 104私有x86_64候选已实际安装，原登录、OA及文字发送运行通过。两轮150秒视频持续解码，未再出现旧Wakelock通道错误；但第二轮挂断后立即点击输入框触发5秒输入ANR。真实主线程堆栈为VideoTrack.nativeRemoveSink → FlutterRTCVideoRenderer.removeRendererFromVideoTrack → setVideoTrack/setStream，见 tests/flutter/android-104-call2-native-anr.txt；不是旧getStats阻塞重现。另发现typingStatusUpdate已被官方固定SDK弃用并抛UnimplementedError，以及缓存登录随后被1506踢线后首页仍调用已退出SDK的竞态。105候选已完成相应源码修复和静态/边界验证，仍待新包运行复验，104不发布。
- 103非RTC原生探针还通过压缩/缩略图/解码/取消、好友备注设置清空及幂等退出后旧企业/IM/WebSocket拒绝。非均匀90度素材的像素方向实际验证通过：压缩帧正确方向RMSE约1.67、倒置约154.48，缩略图分别约8.68/155.09，见 tests/flutter/media-rotation-history/103-x64-222bf056/manifest.json；不是只凭宽高或旋转元数据判断。私有验证入口仍不等于正式ARM64包运行；105候选、Android原位升级、群通话/会议/共享及主持权限仍待对应运行证据。历史102报告另行保留。
- iOS没有本机Xcode构建（按交付边界提供源码和Mac说明）；真实Android/iOS设备媒体效果没有验收结论。

脚本、原始日志与机器可读报告保留在 tests、客户端test-results和artifacts。只有报告明确通过的范围才可引用为通过，后续重建后的包需再次核对哈希。
