# Flutter 升级与构建说明

## 2026-11-04 当前正式 Android 3.8.18+118（R4）

当前已实际非强制发布 Android **3.8.18 / 118**，公开 ARM64 包 SHA256 为 `1750e79b13affea71309a03b39b20c161dae0a2c73f5b1f9822f6666857c1689`。正常 `lib/main.dart` 验收使用同源正式 x64 包 `ad8a902662c13b6abd93df0376e706321a346ae2fd40e7757efc1df04fda6f6a`，仅作私有验证、不公开。两包各自实际 68 项生产秘密模式 RAM 扫描零命中；正式身份、签名、固定 native 载荷和无诊断标记的静态校验与 Main 验收分别记录。当前 Web 保持 3.8.13、Windows 为 3.8.14 / 381401；Android18 发布后的严格 6 项下载检查通过，版本登记 `force=false`。

正常 Main 原始收据 `07869…` 与 Root 最终收据 `612…` 记录初始会话前台 409.47 秒、冷启动后第二会话连续前台 251.39 秒（排除 SAF 导航），未观察到意外退出。实际完成加密备份取消与保存、鉴权预览、native promotion（root dev/inode 与受保护 `master.wrap` 改变）、fresh J 正常登录、新 PNG 本机导入及保护解码读取、最终正常退出；没有只凭退出到登录页认定恢复成功。相机两次首帧/重拍取消、已有两份 PNG 与 MP4 蓝帧/结束控件也有正常 UI 证据，不扩展为物理摄像头、流畅运动或完整音视频矩阵验收。

设备清理仅覆盖合成 J 到 I 的单会话：0 条聊天消息、3 条本机导入、3 引用、3 文件，预计释放 17748 字节；完成后列表为空，恢复后旧三项未复活。预计字节数不是独立磁盘测量。多会话/群组联合清理由原 native SQLCipher 46 项与 Dart 301 项日志分层证明，R4 未重跑这两套；Java 保存权限 whole-class 12/12 与授权竞态 7/7 离线回归通过。`sdkSyncReady` 是实际 SDK 会话入口和受 `authorized && viewsReady` 门控的成功生产方法所支持的推论，未直接持久采集 `onSyncServerFinish`，空 Main 不作为就绪证据。

仍未验：ARM64 物理设备、iOS 编译/签名/真机、Android7 设备、设备多范围清理和非零聊天消息删除。Android7 其他功能保留，加密拍摄要求 Android8 / API26 及以上。本轮测试账号与两个测试员工条目已完成受保护清理及正常后台 API 核对，撤权和审计状态保留。此前未通过或未发布的候选及诊断记录全部保留，下文按原日期解读，不把构建或离线测试写成全实机通过。当前交付以 [客户端说明](../clients/PRODUCTION-DELIVERY.md) 和公开清单为准。

以下 2026-11-02 关于 114 候选、开发身份及未发布状态的内容为历史记录；当前正式版本以 clients/PRODUCTION-DELIVERY.md 及 clients/artifacts-manifest.json 为准。

2026-11-02 用户已恢复音视频与会议开发。当前工作区修订为 **3.8.14+114**，已合入会议权威结束态、不可变登录会话与连接尝试归属、受控原生帧转交，以及仅 Quote 消息选择官方 `sendMessageNotOss` 的保护。完整 Flutter 回归 **163 项通过**，23 目标静态分析 0 错误、106 警告、615 提示，仍非 clean lint；正式 Java 108 项通过，真实 Android Gradle 插件编译通过。帧转交增加固定 2 个 Frame worker、8 队列和 10 retained 许可，原 Media2、Stats2、12 秒调用预算和真实物理释放屏障保持。证据在 `tests/delivery/media-resume-20261102/`。114 正常 `lib/main.dart` 的签名 x64、ARM64 本地候选均已构建，v2 验签通过，包名 `io.openim.upgrade38.test.dev`，min API24 / target35。两次构建各自复核 3232 应用源码、194 固定 SDK 源码、1045 Dart/测试输入及132原生输入逐 SHA 不变；x64 的10项非 libapp 原生依赖与111逐字相同，ARM64 的12项与109逐字相同。自有原生 x64 AVD 已覆盖安装并读取全安装 APK 核对相同哈希，缓存 Home104、Mine自我ID和About114已在同进程观察；这不是新表单登录、系统安装器更新、ARM手机或媒体验收。110 的慢 removeSink、群媒体清理和真实跨端媒体仍须以新包运行结果验收，不能据边界测试推断已经修复。114 未发布。下述 113 包及此前暂停记录按其原验证范围保留，不改写历史失败。

114 候选仅供本地开发检查，均未发布、未通过媒体验收；ARM64 未在物理设备运行。每个 APK 的同名 `.json` 保存源码、签名、依赖载荷及未完成状态，不以构建成功代替运行验收。

| 本地候选 | 字节数 | SHA-256 |
|---|---:|---|
| `artifacts/android-candidates/OpenIM38-3.8.14+114-development-x64-candidate.apk` | 82,520,892 | `fed978e3e414121ce3afa5f789fee20a2a062c93f39cd87b13a664acc424993c` |
| `artifacts/android-candidates/OpenIM38-3.8.14+114-development-arm64-candidate.apk` | 76,743,141 | `f6f946be8ffdf8a00d92c031cfa70e4a7b24bfa1108d838e1446d219e5745312` |

114 的实际媒体状态为 **NOT_ACCEPTED**，公开脱敏摘要为 `tests/delivery/android-114-media-resume-review.json`。单端与三人结果分别记录：

| 项目 | 本轮实际结果 |
|---|---|
| 单端原 Quick Meeting | 104 原界面创建，普通 Leave 最终确认后完整资源清理 1267ms；重开列表仍 Started，详情正常二次进入。真正 End Meeting 最终确认后清理 1622ms，详情自动变 Ended 并更新结束时间；点击禁用的 Enter 后画面保持相同。仅该单端业务态通过。 |
| 三人视频与下行监测 | 104 原 Android 邀请 106/107，两 Web 原界面接听。Web 收到 Android 及另一 Web 的实际帧/RTP、合成音频包和能量增加；Android 单次画面可见两远端，一份早期接收统计为 7 帧，后续统计 TIMEOUT/UNAVAILABLE、信令 IO 出现 SocketException，不能称持续稳定或物理可听声通过。 |
| 三人原挂断 | 完整 Room 清理 **121023ms 后失败**；3 次实际 removeSink 包装耗时 111418、111439、29979ms。调用预算仍为原 12 秒，超时明确报告失败。晚期实际观察到 17 个 queued 任务均有 physicalComplete、相机最终为空；最终释放不能抵消先前失败。没有执行正常第二次群通话验收或 Home/resume 矩阵。 |
| 一次定向诊断 | 前次晚期 SIGQUIT 已在资源释放后，不能代表阻塞现场。另经批准只复现一次诊断，未重启 PID5016，首次新轨道 STOP_RTC_STATS_UNAVAILABLE 即取一次 debuggerd；5.465 秒内完成观察和捕获，99 线程/979 原生帧。该时刻 Stats2/Frame2/Media2 均队列等待，未识别到活跃 getStats/removeSink JNI。UNAVAILABLE 可表示该 peer closing/quarantined 而在 JNI 前拒绝，不等同新阻塞。该诊断轮退出仍失败 16280ms；不计正常第二次调用或无扰动性能验收。 |
| 根因与符号边界 | 固定 release WebRTC `.so` 有 JNI 导出，无完整 C++ 符号/调试信息。现有样本未定位可证的安全窄修，不以无符号 offset 虚构 C++ 函数、锁拥有者或自旋；没有继续追加轮次、延长产品超时或丢弃物理屏障。 |
| 尚未完成 | 预约会议原 UI、纯语音通话与完整跨端矩阵、主持/共享/结束后旧令牌重放、Android 原 UI 引用消息多人转发、物理 ARM64、普通手机的 Go AAR 开发 CA 信任、物理麦克风/扬声器、iOS 构建与真机。真实 MethodChannel 引用路由 4 项及 AAR API 签名检查已通过，不能冒称 Android 原机运行通过。 |

两组 Web 协作进程均原界面退出、记录 track ended/本轮 PC closed 后正常关闭；自有 AVD 在相机为空后正常关闭并保留数据，没有用重启假装恢复通过。114 只允许本地不可变未验收候选；正式 `artifacts/android/OpenIM38-3.8.14+114-development-arm64.apk` 保持不存在，发布清单的 Android 条目为 `present:false`，不提供 current 下载入口，不调用 Android 发布器或强更。硬件缺失不构成源码必然正确的证明。

本应用以原始 3.5.1+39 企业 Flutter 源码为基础，保留原界面和已完成的企业功能。升级成果用于独立 OpenIM38 实例。上一修订为 **3.8.13+113（从已验证111恢复后仅递增版本；42项非媒体回归通过，两份正常main本地候选已构建验签；x64缓存首页、About及无更新提示局部验证通过；当时按用户要求跳过音视频、会议及通话验证）**。111保留138项完整Flutter、94项Java及真实Android类路径编译历史，以及缓存撤销后同进程表单重登和About局部通过证据；这些不改写为113全套重跑。110群媒体清理及会议中断失败仍保留，未发布。固定 OpenIM SDK 仍为 **3.8.3+hotfix.15**；应用版本与 SDK 标签分别记录。

112未完成的会议结束态和会话归属改造已完整保存在`tests/flutter/source-history/111-d4420474/112-unfinished-at-user-skip-20261101T144042Z/`，含原字节、测试失败/未完成记录及恢复说明。8个已有产品文件已恢复111字节，新模型和两份测试已移出活动路径；111的3229源码、194 SDK、99原生输入和62本地包/64路径边均实际复核一致，再只修改pubspec与iOS版本号形成113。113不含112未验收功能。

113实际执行7份非媒体测试文件及enterprise_transport中4个明确命名的输入状态、令牌对和引用消息用例，共42项通过，1041个测试输入前后相同。其余信令、RTC、音频、渲染、会议及Java测试按用户要求不执行；111相同输入的历史结果单独引用。相同19目标静态分析为0错误、103警告、598提示，exit2，非clean lint。既有朋友圈、群已读等设备证据保留原版本范围，不冒称113设备重验。

113仅作本地签名候选，实际路径为`artifacts/android-candidates/OpenIM38-3.8.13+113-development-{x64,arm64}-candidate.apk`及对应metadata。正式`artifacts/android/OpenIM38-3.8.13+113-development-arm64.apk`保持不存在，不上传、不运行发布器、不改强更策略。 x64为82,455,356字节、SHA256 `347b2eaa592576fdbf59b47cea52eb730efe8595c375baecdbb053c08edb0ed4`；ARM64为76,677,605字节、SHA256 `de123b0e169fee5ce32ce45cc2cd9d605c4d8a39f470dcd8b05079dadbcaa5cb`。签名、ABI、依赖载荷和3229+194源码前后核对一致。有限启动见`android-113-nonmedia-runtime-report.json`：系统UI无响应弹窗1次、原Wait1次后出现缓存Home；这不是ownApp ANR通过或真机验收。原检查更新显示“已是最新版本”；未进行下载/系统安装器升级。自有AVD已正常停止并保留数据。候选仍有已知媒体限制；没有物理ARM64设备，不重试已证实失败的ARM转译环境，也不把构建成功当作ARM运行通过。

## 版本与结构

- Flutter 3.32.8 / Dart 3.8.1，framework edada7c56edf4a183c1735310e123c7f923584f1，engine ef0cd000916d64fa0c5d09cc809fa7ad244a5767。
- sdks/flutter 为官方 3.8.3+hotfix.15 源码（1b64cba2bdeace5e858b236b5e1b3deedd2760a0），由 local_plugin/app_im 路径依赖。Android 使用 SDK 配套 core-sdk:3.8.3-patch15@aar，iOS 配套 3.8.3-hotfix.15。原企业原生 SDK 不参与运行。
- 上游 OpenIM.version 字符串仍是 hotfix.14，本应用界面使用 AppIm.sdkVersion，发布身份以 pubspec、提交、依赖锁和二进制哈希为准。
- AGP 8.7.3 / Gradle 8.12 / Kotlin 2.1.0 / JDK 17 / Android 应用 compile、target 35 / min 24 / Build Tools 34.0.0 / NDK 26.3.11579264。
- LiveKit Flutter 2.11.0 / flutter_webrtc 1.6.0 / video_compress 3.1.4。NDK 和 API 35 平台下载核对 Google 官方 SHA-1，另记录 SHA-256。
- 对新 Flutter 不兼容的插件已升级或在 local_plugin/compat 做最小兼容补丁，保留许可证，不修改全局 Pub 缓存。完整 PATCH-MANIFEST.json 记录 46 个包的上游来源、Pub 包哈希和修改文件哈希；pubspec.lock 固定解析版本。android/app/gradle.lockfile 固定 development ARM64 release 运行依赖；更新锁须传 -Ptarget-platform=android-arm64。
- 工具及二进制来源见 tests/flutter/build-provenance.json；SDK 配套 AAR 同时归档于 artifacts/native/core-sdk-3.8.3-patch15.aar。Maven 哈希清单 resolved-aar-hashes.json 明确区分缓存集合与实际运行依赖图。AAR 中 Go 版本字符串为 1.25.6，不与服务端构建 Go 版本混淆。

## 适配行为

AppIm 统一公开 SDK 和企业 API。SDK 回调参数、公开用户/好友/黑名单、历史游标、定时销毁及 LiveKit 2 track publications、participant 和渲染方式已迁移。消息适配使用同一个 Flutter native channel，没有第二个 IM 引擎。

企业 API 为 /enterprise/v1，携带实时读取的 token（IM）、chatToken、platformID。旧 Office/RTC 请求在 HTTP 入口转换。首次事件 after=-1 取得 cursor 后恢复有效邀请；增量事件按 ID 处理，错误不越过游标，SDK 登录前不分派。enterpriseEvent 控制通知只触发拉取，不显示聊天卡片。

显式退出先调用 /session/logout 并确认 revoked:true；HTTP 失败保留凭据用于幂等重试，期间被踢回调不会提前清掉凭据。服务器确认后，即使 SDK 被踢/连接关闭回调抢先到达，也完成本地清理。普通断网与被动互踢不先调用该接口。首次资料获取失败不再用空 UserInfo 假装登录成功。

105冷启动先等待SDK网络连接并用该次不可变IM/Chat令牌对进行服务端核验，失败只提示重试，不凭本地cached-success进入首页。被踢/过期即时使本地会话代际失效，旧异步结果不能导航或覆盖新会话；已登录后的普通断网保留重连。原输入提示映射到changeInputStates和onInputStatusChanged，仅更新当前对方会话的输入状态，不保存为普通消息。铃声加载、停止与播放Future均观察，旧加载不能在取消后重新响起。

群已读上报真实消息 seq/sender，发送、历史及跨端同步时补摘要；未知数量显示 --，不假装全部已读。详情使用服务器接收成员快照。分享/转发 905 会议卡片前登记邀请，失败不发送。

会议初始详情从企业 get_signal_invitation_info 的 meetingInfo 读取，不依赖空的 LiveKit room metadata；校验成功后才按初始权限发布音视频，失败提示并断开。成员和主持权限后续通过持久事件更新。

FFmpegKit 已移除，视频由平台压缩生成 MP4，再提取缩略图和时长，明确毫秒到秒换算，支持取消，不删除原文件。旋转像素、缩略图与跨端解码已在独立 API30 原生 x86_64 AVD 验证，物理手机声音和播放仍须真机验收。

更新使用同源 /enterprise/v1/application/latest，字段 available/version/build/url/sha256/size/force/text。语义版本与 build 决定升级，拒绝降级。完整 APK 下载后验证大小及 SHA-256，错误包删除并可重试；已确认强更在 MaterialApp builder 中位于 Navigator 之上，覆盖点击、焦点和返回键；Splash、自动登录及 offAll 路由替换不会移除门禁。原生安装异常或返回 false 均传播为失败，保留重试。未发布或查询失败允许当前版本继续。iOS 适配 HTTPS 发布地址跳转。蒲公英演示地址和密钥已停用。

Getui 是未配置的可选外部推送提供商，默认不初始化、不绑定别名；缺少推送配置不会再导致 IM 登录崩溃。配置后的第三方推送仍需独立验证，不承诺应用被系统结束后的即时推送。

## Windows 构建

运行 clients/flutter/scripts/bootstrap-flutter.ps1，再运行 build-android.ps1 -Flavor development。标准脚本执行完整Flutter test目录和test-native-java.ps1的原生生命周期/统计回归后再构建。工具路径只作用于当前进程，AndroidSdk/Jdk 参数可覆盖本机路径。

构建脚本在长路径环境自动选择 ShortDrive（默认 O:），只对本项目建立临时映射，结束后移除自己创建的映射；已有同项目映射会复用。Gradle 缓存固定项目根 .cache/gradle，避免系统盘下载空间不足；不修改系统默认运行时。

默认入口 https://192.168.10.11:29880，Chat /chat、IM /api、WebSocket /ws、企业 /enterprise/v1。OPENIM_BASE_URL 可用 dart-define 覆盖；地图使用 OPENIM_MAP_KEY，不携带旧演示 key。

development 包名 io.openim.upgrade38.test.dev，production 为 io.openim.upgrade38.test，不覆盖旧应用。Android network_security_config 和 Dart SecurityContext 信任本项目开发 CA，实测 Chat HTTPS 正常。**不能声称应用自带 CA 已覆盖固定官方 Go AAR。** MainActivity 及更早的 OpenImApplication.attachBaseContext 设置 SSL_CERT_FILE/SSL_CERT_DIR 后均实测 x509 unknown authority，保留 native-ca-mainactivity-failure.json、native-ca-application-failure.json。production 不注入开发信任，两种构建均保留证书链、有效期和主机名校验，没有忽略错误或关闭 TLS 的分支。

固定 AAR 的运行验证在本次新建 openim38_api30 / emulator-5588 userdebug AVD 中加入系统信任根（旧 openim38_api35 失败现场保留）：先对该 serial 执行 adb root，再运行 tests/flutter/install_avd_ca.py。脚本核验 AVD 名称，仅对本虚拟机的运行时证书目录做 bind mount，不改共享 SDK 系统镜像；重启后须重新执行。记录见 avd-ca-setup.json。

这不是普通员工手机的安装步骤。普通 Android 手机安装用户 CA 或仅应用内 CA 对该 AAR尚未验证成功；正式手机联调需公信证书或经企业验证的系统信任方案，不能关闭校验。AVD 系统根下通过，不代表普通手机的开发证书问题已解决。

开发包沿用原 SDK 的诊断等级 6。106的安全接收统计只从已有采样记录trackID/framesDecoded/bytesReceived，不新增JNI请求；107另外仅在采样状态变化时记录本地状态/错误码，区分采样缺失和停止，不改变统计预算、并发或重试策略。IO日志去除URI查询、userinfo和fragment。已在官方 GoLog 的调用/HTTP 诊断中实测出现令牌，不能直接公开原始运行日志。原始采样保存在仅当前用户/SYSTEM可读的 `.private/android-raw-evidence`，公开验收日志按字节脱敏并记录前后 SHA-256，时间线保持原样；见 raw-log-redaction-103.json/104.json。104 冻结构建未为此更改 SDK 日志实现，正式运行前需核定日志等级、访问权限和导出脱敏策略。

android/key.properties 与 android/upgrade38-test.jks 是专用测试签名私有文件，已忽略且 ACL 限当前用户/SYSTEM。2026-11-01已在E盘独立受限目录保存副本并核验源前后和副本哈希一致，见tests/delivery/android-signing-backup-review.json；这是同机目录备份，不是异机或离线备份，不放入脱敏源码或公开下载目录。

## iOS 构建边界

本机为 Windows，未进行 Xcode/CocoaPods 构建，不能据 Dart/Android 构建声称 iOS 已验收。Mac 上安装相同 Flutter、Xcode、CocoaPods，执行 flutter pub get、pod install，打开 Runner.xcworkspace，配置自己的 Team/签名。先执行相应flutter build ios生成当前版本构建配置；仓库中旧Generated.xcconfig及flutter_export_environment.sh属于历史生成元数据，不能当作当前应用版本。已设 iOS 13、正常 ATS，旧 Team 已清空。bundle IDs 为 io.openim.upgrade38.test、.NotificationService、.ScreenShareBroadcastExtension；App Group 为 group.io.openim.upgrade38.test.rtc，主应用与扩展版本同步。旧含 FFmpegKit 的锁仅归档 migration-reference/ios/Podfile-3.5.lock，Mac 应为新版 podspec 生成新锁。开发 CA 须在测试设备系统中安装并开启完全信任；正式版用系统可信证书。Go 核心 TLS、三端消息和音视频必须真机验收。

## 当前证据

111（3.8.11+111）只修两处明确问题：AppNavigator.startLogin在已处于Login路由时直接返回，保留原offAllNamed行为，不加全局latch；CameraEventsHandler的state改为volatile，为相机回调线程和释放轮询线程建立可见性。后者是独立正确性修复，不将其认定为13秒或58秒removeSink等待的原因。4项正式回归使用实际AppNavigator/Get4.7.3/LoginPage/Binding/Logic，覆盖同tick、转场中、稳定后重复及离开后再入；6项正式相机行为检查编译实际产品类，未以诊断clone代替。完整138项Flutter和94项Java通过，19目标分析0error/103warning/598info，99项原生输入前后相同、真实Android类路径编译成功。SDK/AAR、12秒预算、线程池、队列、租约和释放次序均不变。帧handoff与新采样器候选未合入。完整3229应用输入、独立SDK194及1039个Dart/测试/解析输入构建后逐SHA无漂移，62本地root/64path边覆盖重验通过。私有x64正常lib/main.dart APK已构建验签，SHA-256为78d6fd1d4550fcf3e0bf19741fb892970d86429ec1c092e6e0a888e25f9b5274、82,455,356字节；11个原生库中10个依赖载荷与110逐字节相同，另存5份受限R8文件。111私有实包保数据覆盖安装后，原缓存进入104会话页并在同进程后台返回；实际双令牌对先核企业/IM/Chat三接口有效，再在app停止时单次执行精确撤销（内含两次幂等logout），三接口均拒旧对。冷启回原Login、应用自行清除持久化登录凭据，开发CA未删除；40个非均匀PNG样本未见Home，不作全帧无闪入保证。随后原表单使用轮换后密码，在同PID5368内提交即进入Home，新双令牌三接口有效，没有重启进程、清数据或注token。About显示111并在原检查更新后提示“已是最新版本”。首个缓存启动有一次System UI无响应弹窗，保留原Wait及环境证据；本轮公开日志未见ownApp ANR，不能以日志缺少系统记录抹去该弹窗。111未重测媒体或物理ARM，不将Camera volatile认定为既有慢removeSink修复。见android-111-cache-revocation-ui-report.json、android-main-x64-111.json、login-route-integration-review-111.json及camera-state-native-review-111.json。

110（3.8.10+110）合入已审的Bugly错误观测兼容补丁，保留原异步错误与本地Flutter诊断，未配置有效ID时不尝试上传；上传、观察者自身错误分别观察，旧初始化结果不能启用新一代失败配置。完整134项Flutter测试通过，其中新增11项直接导入实际插件。正式分析范围新增该插件叶文件，共19项，结果0error/103warning/598info；与109较窄范围的提示数量不可直接比较。私有x64正常lib/main.dart候选已构建并验签，APK SHA-256为c1c321dd506c212b1a00619656ef8da8e70d46601319143f28056878fb10e4c6、82,455,360字节；3229源码与SDK194构建后逐SHA一致，10个依赖原生库与109逐字节相同，DEX已核六个新观测标记。见android-main-x64-110.json。实际原UI已完成有效旧会话撤销、冷启回Login、进程重启后的原表单登录和About无更新继续；被撤销冷启后的同进程Login曾按钮失效，不能标自动恢复通过。两次正常单聊视频接通均超过150秒，原挂断消息真实送达且资源最终完成、无新ANR，但接收统计监测停止单列。三人群Home/resume空白checkpoint后自行恢复，Android退出时removeSink包装调用58秒、Room清理72.313秒失败，因此110整体不通过。即时会议首次协商/重连/发布链中断；原详情重入后主持三限制与恢复、双向共享及最终原确认结束局部通过。最终详情仍显示Started/Enter Meeting，页面本身仅按开始时间分支，未验证再次进入；不由此推断服务器仍允许加入。此次未保存真正旧RTC JWT，未重放验拒绝。见android-110-group-cross-runtime-review.json、android-110-meeting-runtime-report.json；成功局部证据不抹前述失败。本次88项原生边界实际重跑及真实Android类路径编译通过，95项输入前后相同。原生只增加Java包装对象哈希关联与既有释放子步骤时点，不增加JNI查询、重试、线程或超时预算；哈希可能碰撞，不作为唯一身份或C++对象证明。CameraEventsHandler既有跨线程状态可见性风险仍单列，未为本轮观测改动它，也未将其认定为removeSink等待的原因。

109的私有x64正常main已用原邮箱密码表单实际登录，并在同一进程完成原群已读页0/2→1/1→2/0：先由Web102、再由Web103点击真实群列表进入，Android页面自动更新，已读/未读两栏人数与102、103真实姓名一致，固定历史消息seq2与findResultItems日志相符。仅对自建AVD断网后，两栏显示未知--和可见失败/Retry；恢复网络后分别点击原Unread和Read侧Retry，恢复0未读、2已读和102/103名单。见original-ui-group-read-report-109.json，限合成三人群这一消息，不代表ARM或RTC验收。原自动化密码输入曾误落到系统搜索框，已停止输入并将受影响原图移入受限私有证据；共享该凭据的110个合成账号已统一轮换并验证旧密码/旧会话拒绝，随后用新凭据严格逐步核验原登录表单。该事件、公开输出检查和证据边界见[测试凭据事件记录](testing-credential-incident-109.md)。首次旧缓存因屏幕尺寸误归APad而触发1502回登录，不能冒称完成真实缓存撤销验收。

109只修复实际原群已读入口暴露的字段和异步状态问题：findMessageList消费者使用固定SDK真实findResultItems；详情两个tab分别持有loading/loaded/failed与未知人数，失败提供实际重试，页面关闭后迟到结果不写状态。查询进行时收到真实receipt事件只置每tab一个dirty位，当前请求完成后串行合并刷新，避免漏掉最后一条回执；关闭清理dirty。服务端接受clientMsgIDs/clientMsgIDList两种别名，因此hydrate没有为改名而改动。完整123项Flutter测试通过，新11项实际SDK模型/MethodChannel/页面/receiptstream测试包含其中；同18范围分析0error/103warning/597info。Native95生产/测试/脚本输入及成功日志逐SHA与108相同，明确复用其88项Java边界和真实AAR classpath编译结果，没有声称重新运行或设备通过。完整3229源范围、独立SDK194和62本地root/64path声明已核；source为7c0c75520658c61bcd6e25a93b8e5d3eb266976a22edb39bfa5557572277d55e。私有ARM64、lib/main.dart候选已构建、验签并完成3229+SDK194构建后核验，APK SHA-256为141ef706479719a644a6676d244ec85412e5c1fb09af3df16d3a6d30e06cd7c1，76,677,605字节，13个原生库中12个依赖载荷逐一映射；Go AAR原始库经固定NDK的标准strip后与APK字节完全一致。旧API30 AVD中108测试包持有cpuAbiOverride=x86_64，普通、显式ARM参数和同字节非流式安装均返回-113；AOSP11的旧覆盖值优先级可解释三次拒绝，不能据此断言ARM转译不支持。旧AVD数据保留，另建同官方镜像的openim38_api30_arm109空userdata环境干净安装，最终包路径SHA与原候选一致且primaryCpuAbi为arm64-v8a、无旧override；首次DEX编译超过安装工具240秒等待，但随后实际安装提交成功，此等待错误单独保留。正常main首次启动在登录表单前出现ndk_translation Undefined instruction 0xd5380000/SIGILL，原始tombstone与日志已归档，未进行合成表单登录。该转译运行失败与物理ARM行为分开，候选仍未发布、未通过运行验收；同109冻结源私有x64正常main也已构建，SHA-256为2a02d3a885713b6492796916e981cca3b6b6c49d9ed2f838c0c3c902d9c890c4、82,455,356字节；11个原生库中10个依赖库与108逐字节相同，另一个是109正常main的AOT。签名、3229+SDK194构建后核验通过，用于继续可执行原UI验证。ARM翻译失败由独立报告android-arm-translation-independent-review-109.json锁定到Go1.25.6 internal/cpu.getMIDR，物理手机行为仍未知。见android-main-arm64-109.json、android-main-arm64-109-payload-verification.json、android-109-prior-package-abi-override.json和final-verification-109.json。

108修复107实际暴露的两条链路：渲染Widget布局/尺寸变化重复发起同源绑定，以及本地媒体操作仅调用方超时后永久隔离而物理工作后来成功的状态。108保留正常TLS与原超时预算，未知物理失败仍保留屏障；实际采集、首发和摄像头按钮错误保留PlatformException代码、受当前通话代际约束并显示非致命重试提示，不将失败当发布成功。完整112项Flutter测试、88项生产Java边界检查、真实Android/AAR编译通过；18范围分析0error/103warning/597info，含新增mediaDevices叶文件，cleanLint=false。真实Widget与Controls测试的原生边界为受控MethodChannel，不代表设备通过。统一源3229文件，独立SDK194文件；52个原有朋友圈源码和5个原被example目录名误剪的原生输入补入覆盖，旧快照bytes不改，不当作108新增功能。首轮4个遗漏输入仅构建中补锁，故保其APK和部分scope事实，再以新3229完整scope进行真实pre→同命令增量构建→post核验，APK SHA与首轮完全一致。详见final-verification-108.json、path-dependency-coverage-108.json；私有x86_64候选已验v2签名/ABI/包名及完整scope构建前后零漂移，实际运行首次正常150秒通话后挂断仍失败：两个原生renderer.detach物理操作约13秒，导致3个调用方deadline及18层已捕获清理失败；虽随后资源真实释放、完整文字送达且无新ANR，不能将其记为通过。见original-ui-rtc-report-108.json。

108随后在同一PID4997用原Video Call进行独立重采集诊断，新原生摄像头/麦克风请求正常出现，Web实际接收新的Android远端轨，17.75至32.77秒decoded50→101，证明软超时物理晚成功后没有永久阻止后续采集。诊断挂断仍再次出现相同deadline/清理失败，并按授权在双detach已开始超过3秒时仅发送一次手动SIGQUIT取栈；这是受扰诊断，不计正常第二轮150秒验收或清理延迟通过。见android-108-recapture-diagnostic-report.json。108没有构建或发布正式ARM64新包。

108的独立冷进程视频诊断保留APK、数据、双令牌和系统CA不变，仅重启本包，仍观察到两个detach约9秒、随后本地视频轨释放5.096秒，累计等待使local.dispose超出原12秒预算，故仍失败。随后同PID原Voice Call约30秒只创建音轨，无视频/renderer.detach，本地音轨释放76毫秒、两PC关闭各83毫秒、Room资源清理871毫秒；完整日志无本轮捕获清理错误/Unhandled/新ANR。语音只是创建及清理路径诊断，没有物理可闻或双向音频RTP断言，也不替代视频验收。见android-108-cold-video-diagnostic-report.json和android-108-voice-diagnostic-report.json。

108原界面另局部验证朋友圈纯文字发布、仅选102可见、原点赞评论与重新进入持久化；Web102原UI可见/103不见，102点赞评论后Android原刷新实际显示。随后原UI又分别发布Public、Private和Not visible to some仅排除102三条唯一marker，Web102/103原feed与DOM核对分别为可见/可见、不可见/不可见、不可见/可见；四种范围均按实际观察记录，不将单一列表标签推断为权限。追加证据见android108-circle-scope-review.json。原群已读入口存在，但实际点击触发两条未观测错误：适配层将findMessageList结果误读为searchResultItems，忽略了真实findResultItems内seq2消息；108此项失败，109正在单独修复。TagGroups/IssueNotice原隐藏入口保持不变，不把底层或Web验证记为Android页面通过。见partial-original-ui-108.json及group-read-contract-clarification-108.json。

107原界面额外完成局部非媒体验证：OA专页；Android到Web101及Web102到Android的输入提示；全局免打扰原UI切换0→2→0并经正常TLS接口核对已恢复。Web清空/失焦未触发即时focus=false，输入提示随后真实消失，本轮未测定精确过期秒数。证据见partial-original-ui-107.json；107整体仍未通过。

107完整102项Flutter回归通过（unit-tests-107-final.log），其中新增9项实际just_audio AudioPlayer生命周期在受控原生平台边界测试，覆盖初始化/配置错误、被停止的旧加载晚返回、缓存平台Future和新播放不被旧加载关闭。插件修复分别观察缓存及公开Future，保留原错误与堆栈；不能据测试推断Android已无音频异常。扩展分析包含just_audio及接收统计叶文件，0error、99warning、592info，scope见analysis-scope-107.json，不能将提示数量直接与106旧范围比较。原生43项源/测试/脚本与日志逐件SHA不变，显式沿用106的69项Java边界及真实插件编译结果，未重复运行，见native-verification-reuse-107.json。107私有x86_64候选已构建、核验v2签名/ABI，并确认3171份源码输入在构建后零漂移（android-validation-x64-107.json）；实际运行回归未通过：第一轮有已捕获的渲染变更和本地轨道清理失败，最终camera[]与两PC close成功不能等同全层清理成功；第二轮Video Call接收画面正常，但本地camera黑、Web无Android出向视频。短诊断点击原Camera On暴露getUserMedia的“Local media cleanup is still pending”错误及未观测控件Future。两正常窗口没有音频abort Unhandled，仍不能标记107完成。见original-ui-rtc-report-107.json及android-107-camera-diagnostic.json。未构建或发布新的正式ARM64包。

106将本地点击挂断、服务端Leave、终局重连失败和dispose统一到Room连接代际的shutdown Future：先释放全部参与者与本地轨道，再关闭PeerConnection。连接创建尚未完成时，真实创建结果仍进入归属集合，退出等待它完成后实际释放；排队旧事件和await后迟到的旧SDP回复不能改变新连接。异步事件回调错误通过实际Engine/Room订阅者汇总，保留首错并触发有序关闭，原通话/会议页显示失败。

106挂断/取消请求由有界任务持有发起时的用户、双令牌、平台、地址和会话代际，本地界面无需等待网络请求。超时先查询权威ended或当前参与者left状态，再有限重试；activeInvitationForActor=false本身不算挂断确认。401/403停止重试，换号、被踢及退出取消旧任务；失败保留到恢复前台重试。WebSocket连接按代际拥有物理关闭屏障，异步业务回调和底层错误分别观察；本机TLS回环测试通过并不代表已复现或修复105的Android底层closed-read。

106完整93项Flutter回归通过（unit-tests-106-final.log），扩展静态分析0error（99warning/585info），正式Java69项边界检查通过。首轮两项旧测试fixture适配失败独立保留，未放宽产品守卫；新版fixture实际执行真实连接。私有x86_64 106已构建并核签名、架构及源码零漂移。首轮原UI150秒Web远端帧递增、Android远端ROI采样画面更新，挂断后完整a106c1送达、无新ANR、全部资源真实释放；两PC关闭为2.082/0.627秒。但just_audio内部PlatformException(abort)仍有一条Unhandled，接收监测仅首份统计后无新样本，故整体不通过，未进行正常第二轮。见original-ui-rtc-report-106.json。105原包和两轮现场、主动SIGQUIT诊断、源码与失败日志独立保留。

104源码新增 Wakelock 原生1.1.4与 platform_interface1.1.0匹配（精确Pigeon10通道和codec），避免旧解析1.3.0造成Unhandled异常。实际RTC资源层按每个Track、Publication、Participant、Room、Engine、Transport尝试所有释放步骤，先等待真实完成再清引用；并发/同步重入共享完成Future，失败保留并向父层报告。104通过26项Flutter测试、11组Java生命周期测试、独立源审及私有x86_64构建。实际两轮150秒视频均持续双向解码，Wakelock错误消失；第二轮原挂断后立即输入触发MainActivity ANR，原生堆栈是VideoTrack.nativeRemoveSink在平台主线程等待。媒体后来确实释放，但不将其算作挂断响应通过。104未发布。105正在修复原生渲染解绑，以及实际发现的新版输入状态、失效自动登录和铃声异步竞态；须新冻结、新构建和新运行证据。详见original-ui-rtc-report-104.json及android-104-call2-native-anr.txt。

- 105当前统一59项Flutter回归通过（unit-tests-105-final.log），包括真实Splash/Home受控SDK回归、铃声竞态、Renderer MethodChannel和实际VideoTrackRenderer Widget生命周期。最终分析0错误、99警告、580提示，clean lint=false。原生生产插件Java编译和53项Java边界回归通过（11 Gate、10 Selector、10 Binding、11 Local、4独立Local、7实际Renderer），最终43项文件哈希独立复核一致。JNI/EGL由受控边界替代；105私有x86_64包已构建并验签，3169个保留源码输入与构建前全部匹配。第一轮150秒双向视频无新ANR，但挂断后原生close deadline及Socket/信令异步错误使验收失败；继续诊断，未构建发布新的正式ARM64包。不能从这些测试推断设备通过。
- 10 项（103基线）更新策略、企业传输、幂等事件、消息模型及强更导航竞态单测通过（unit-tests-103.log）。源码分析记录保留于 analyze-103.log；上游插件开发用 Pigeon 输入与独立示例不参与应用源码分析，编译后的运行通道仍参与检查。原生兼容编译通过 344 个 Gradle 任务（native-compatibility.log）。
- ARM64 主入口102已构建并通过签名校验，min24/target35；100的 Getui 缺类崩溃已修，101为撤下候选。102实际原 UI 测试暴露了 WebRTC stats 主线程 ANR，未发布。103修复统计与主线程阻塞，私有 x86_64 实际运行两轮长视频已通过持续解码、挂断及二次呼叫；它仍有 Wakelock 协议错配，未发布。104已构建但实际挂断响应失败；105归档为未通过，106另立新包继续修复，不覆盖旧包哈希。
- native-sdk-report-103-passed.json 当前 passed:true：API30、官方 AAR 原生 x86_64 私有验证包在系统 CA 前提下，通过 Go API TLS/WSS/同步、S3、接收历史与 SHA-256、12条消息场景（10种主消息类型，含自定义110、转发和两个视频方向）、视频压缩/缩略图/原生解码、取消保留原件，OA content1400/session4、同实例备注设置/清空，以及精确退出幂等、旧企业/IM接口与 WebSocket 拒绝。该报告与 ARM64正式包/物理设备分开。
- 原 main.dart 界面通过实际邮箱表单登录、OA专页、文字发送到Web101、退出清本地凭据且旧企业接口401/IM1506、再次登录。见 original-ui-report-102.json 及截图。初次视频通话双向解码后约25秒 Android 停止响应；api30-main-ui-video-anr.txt 明确 main 阻塞在 getStatsSender。不能将此标为长通话或 Android 挂断通过。
- 103在全局有界工作池收集不可变的完整PC统计，再按trackIdentifier、RTP/RID根及Id依赖图选出该track的统计，避免后台getSenders/getReceivers销毁Java wrapper缓存的竞态；没有匹配根返回明确错误，不返回全部轨道。它是track范围统计，不承诺共享同一track的任意多个sender仍逐sender唯一。每peer单请求，超时隔离且不重建阻塞线程；close/dispose也在有界后台执行，真实释放后才删除映射。Room拥有参与者清理，已关闭引擎不再逐个removeTrack/negotiate。10组线程/生命周期测试和10类统计选择图测试通过（webrtc-operation-gate-tests.log、webrtc-track-selector-tests.log）；103真实原UI第一轮超过5分钟、第二轮超过150秒，远端Android帧持续递增，统计线程固定2、清理线程最多2，未复现ANR；两次原挂断后摄像头最终释放约27.5秒和14.6秒（软件AVD）。见 original-ui-rtc-report-103.json，不将延迟释放写成瞬时释放。
- 私有API30验证包为 tool/validation_main.dart，无probe Intent时调用原界面，有合成Intent时调用测试探针；正式ARM64包独立用lib/main.dart。开发MainActivity含只读运行时Intent的原生测试channel，无内置合成凭据。两者不可混称同一安装包。
- API30软件GPU以启动参数 enable-impeller=false 使用Skia；正式manifest未为模拟器改动。API35的 PhoneAccount异常与系统/应用ANR现场保留，具体组件和caller尚未完整确认，不归结为已证明的镜像问题。物理ARM64、普通手机证书和iOS仍待验收。

103旋转测试使用四象限不均匀视频及白色角标，确认方向而非仅宽高比：压缩画面正确方向RMSE1.67，对照倒置154.48；原生缩略图8.68，对照倒置155.09。完整原始素材、输出与APK绑定见 tests/flutter/media-rotation-history/103-x64-222bf056。

最终以结构化报告、原始日志和安装包 SHA-256 清单为准，未通过项不得标记升级完成。

本次为兼容新 AGP，将本地依赖中旧 compile SDK 统一至少 35，Flutter WebRTC 1.6.0 上游要求其插件编译 SDK 36（应用 target 仍为 35，构建机需同时有 API 36 平台）。官方 Flutter SDK 源码不修改。
