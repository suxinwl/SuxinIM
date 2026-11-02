# OpenIM38 升级项目源码与使用说明

## 源码导航

本仓库同时保存服务端、管理后台、Web、Windows、Android 和 iOS 的开发源码。Windows 与 Web 共用 React 界面，Windows 另有 Electron 主进程；Android 与 iOS 共用 Flutter/Dart 业务代码，分别保留各自平台工程。各端目录如下，不需要查找单独的 Android、Windows 或 iOS 仓库。

| 部分 | 直接源码入口 | 构建说明 |
|---|---|---|
| Web | [React 界面与业务逻辑](clients/web-desktop/src/) | [Web / Windows 构建](docs/web-desktop.md) |
| Windows | [Electron 主进程和预加载](clients/web-desktop/electron/)；[共用界面](clients/web-desktop/src/) | [构建脚本](clients/web-desktop/scripts/)；[打包配置](clients/web-desktop/electron-builder.json5) |
| Android | [Android 平台工程](clients/flutter/android/)；[共用 Dart 代码](clients/flutter/lib/) | [Flutter / Android 构建](docs/flutter.md)；[Android 构建脚本](clients/flutter/scripts/build-android.ps1) |
| iOS | [iOS / Xcode 平台工程](clients/flutter/ios/)；[共用 Dart 代码](clients/flutter/lib/)；[iOS Core](sdks/ios-core/) | [Flutter / iOS 构建说明](docs/flutter.md)；本期未完成 Xcode 构建验收 |
| 客户端适配和本地插件 | [AppIm 适配层](clients/flutter/local_plugin/app_im/)；[固定本地兼容插件](clients/flutter/local_plugin/compat/) | [Flutter 依赖锁](clients/flutter/pubspec.lock) |
| 固定 SDK | [Flutter SDK](sdks/flutter/)；[WASM SDK](sdks/wasm/)；[Android Java 参考源码](sdks/android-java/)；[iOS Core](sdks/ios-core/) | [上游版本锁](upstream-sources.lock.json) |
| 服务端 | [IM](services/openim-server/)；[Chat](services/chat/)；[企业扩展](services/enterprise-service/)；[LiveKit](services/livekit/) | [构建与运行](docs/build-and-run.md)；[Linux 构建脚本](deploy/build-linux.sh) |
| 管理后台 | [后台源码](clients/admin/) | [构建与运行](docs/build-and-run.md) |
| 整体构建与部署 | [Windows 构建入口](tools/build-all.ps1)；[部署脚本](deploy/) | [工具链锁](runtimes.lock.json)；[LiveKit 锁](livekit.lock.json)；[镜像锁](deploy/images.lock.json) |

源码直接保存在上述普通目录中，不依赖 Git 子模块。GitHub Releases 当前没有上传安装包，不代表这些客户端源码未上传；本地 EXE、APK、依赖缓存、构建输出及私有配置另行保存。克隆方法与源码范围见[仓库说明](REPOSITORY.md)。Android114 媒体仍为 **NOT_ACCEPTED**，iOS 源码存在不等于已完成 Xcode 或真机验收。

## 使用说明

本说明用于原 3.5 企业即时通讯项目合并升级后的 OpenIM38 开发版本，面向系统管理员和测试使用者。沿用原版账号登录、好友、群组及企业功能界面，管理后台沿用原 3.4 界面并适配新版接口。本文仅介绍这套升级项目的实际入口、安装包、账号管理及运行方法。

更新日期：2026年11月2日。当前使用独立内网测试实例和合成账号；Windows 3.8.7 已发布到独立内网下载页，完整升级验收尚未结束。语音、视频、通话及会议已恢复开发与验证；Android 3.8.14+114 仅提供本地开发候选，媒体状态为 NOT_ACCEPTED（未通过验收），不发布 Android 下载或更新。Android113旧非媒体候选及先前暂停归档继续保留。

## 一 项目位置和版本

全部后续开发源码、配置材料、脚本和本地交付文件位于以下根目录。下文“项目根目录”均指此处，表格中的相对路径应接在此路径后。原始 3.4、3.5 源目录保留作对照。

```text
E:\PUTY_AI\OpenIM3.4-3.8\OpenIM3.8.3patch.18_1.8.4patch.5
```

| 组件 | 固定版本或当前应用版本 |
| --- | --- |
| IM 服务端 | v3.8.3-patch.18 |
| Chat 业务服务 | v1.8.4-patch.5 |
| Web 和 Windows WASM SDK | 3.8.3-patch.15.1 |
| Flutter SDK | 3.8.3+hotfix.15 |
| Flutter Android 核心 | SDK 配套 3.8.3-patch15 AAR |
| Windows 应用 | 3.8.7，构建号 380701 |
| Android 应用候选 | 3.8.14+114 本地开发候选，媒体未通过验收；3.8.13+113 保留为历史非媒体候选 |
| iOS Core | 3.8.3-hotfix.15；提供源码和 Mac/Xcode 构建说明 |

Android Java SDK v3.8.3.5 仅作独立参考源码，没有替换 Flutter 配套 AAR。运行版本和依赖以 upstream-sources.lock.json、runtimes.lock.json、各模块锁文件及包旁元数据为准。

## 二 访问入口和网络

| 用途 | 当前地址 |
| --- | --- |
| Web 通讯 | https://192.168.10.11:29880/ |
| 管理员后台 | https://192.168.10.11:29880/admin/ |
| 内部下载页 | https://192.168.10.11:29880/downloads/ |
| 附件 HTTPS 服务 | https://192.168.10.11:29885/ |
| IM API | https://192.168.10.11:29880/api |
| Chat API | https://192.168.10.11:29880/chat |
| IM WebSocket | wss://192.168.10.11:29880/ws |
| 企业扩展接口 | https://192.168.10.11:29880/enterprise/v1 |

普通用户打开 Web 通讯或 Windows 客户端。管理员打开 /admin/；附件端口用于软件签名上传和下载，不是独立登录网站，直接访问根路径不能代替附件功能验证。以上是内网入口，不能据此认定外网可访问。

当前入口配置允许 13 个内网网段：192.168.6.0/24、192.168.10.0/24、192.168.14.0/24、192.168.17.0/24、192.168.18.0/24、192.168.19.0/24，以及 192.168.26.0/24 至 192.168.32.0/24 的每个 /24 网段。跨网段使用还需要网关路由和 ACL 可达。

```text
Test-NetConnection 192.168.10.11 -Port 29880 -InformationLevel Quiet
Test-NetConnection 192.168.10.11 -Port 29885 -InformationLevel Quiet
```

在 Windows PowerShell 中分别运行上述两行。True 只表示 TCP 端口可达，仍需继续验证证书、登录与附件；False 时先检查到服务器的路由、端口放行和服务状态。

## 三 首次安装开发证书

当前测试证书名称为 OpenIM38 Isolated Development CA，服务端证书包含本内网 IP 的 SAN。Windows 需由实际运行客户端的同一用户导入受信任根证书。此证书用于当前测试实例。

| 文件 | 项目根目录下的路径 |
| --- | --- |
| 证书与检测脚本压缩包 | artifacts/OpenIM38-Development-CA.zip |
| 双击入口 | artifacts/dev-cert/Install-and-check.cmd |
| PowerShell 安装脚本 | artifacts/dev-cert/Install-Development-CA.ps1 |
| 公钥证书 | artifacts/dev-cert/OpenIM38-Development-CA.crt |

1. 先从项目本地目录或受信任的内部文件传输取得证书压缩包。尚未信任开发 CA 时，HTTPS 下载可能被浏览器阻止，可先复制本地压缩包。
2. 解压整个压缩包，不要只复制 cmd 文件。
3. 使用运行通讯客户端的同一 Windows 用户双击 Install-and-check.cmd；核对证书名称后确认导入。脚本使用当前用户证书存储。
4. 查看脚本输出的证书检查及两个业务端口检测结果；随后完全退出客户端，包括托盘，再重新打开。
5. 在浏览器访问后台入口，确认正常证书校验后再输入账号。保留 TLS、证书链、主机名及 SAN 校验。

公钥证书 SHA-256：90817f305832a3c2398af0384e76004096647aeeac7095a250c6a5a4845655f0。证书压缩包只有公钥及脚本，不含 CA 私钥或账号密码。

Android 特别说明：固定 Go SDK 核心不采用应用内开发 CA，也不采用普通用户证书库；给普通手机安装用户 CA 不能保证整个 App 可连接。当前原生链路仅在专用测试模拟器的系统信任环境验证过。普通未 Root 手机需要公信服务端证书或经验证的系统信任部署。

## 四 管理员账号密码和首次登录

当前后台管理员账号和原始密码已从本机现有私有账号配置导出到单独的受限登录附页。本文和下载页均不写入实际密码，安装包也不包含该附页。

```text
E:\PUTY_AI\OpenIM3.4-3.8\OpenIM3.8.3patch.18_1.8.4patch.5\.private\manual\管理员登录信息.txt
```

1. 由本机当前管理员打开上述登录信息附页，取得后台管理员账号和密码。
2. 先完成开发 CA 信任，再访问 https://192.168.10.11:29880/admin/ 。
3. 在后台登录表单输入附页中的原始密码，不要手工把密码转换为 MD5，也不要使用服务间密钥或令牌作为密码。
4. 进入后台后，在“设置 → 个人信息”查看当前管理员，在“设置 → 修改密码”修改管理员密码。修改后，登录信息附页需要由管理员同步维护。

后台管理员与普通通讯用户分别管理。管理员登录资料用于管理后台，不应分发给普通使用者。当前保存的凭据来自独立测试实例；本次说明整理没有重置密码、创建新管理员或重新签发会话。

## 五 安装 Windows 客户端

| 项目 | 内容 |
| --- | --- |
| 安装包 | OpenIM38-Enterprise-3.8.7-win-x64.exe |
| 应用版本 | 3.8.7 / 380701；Windows x64 |
| 本地相对路径 | clients/web-desktop/release/OpenIM38-Enterprise-3.8.7-win-x64.exe |
| HTTPS 下载 | https://192.168.10.11:29880/downloads/OpenIM38-Enterprise-3.8.7-win-x64.exe |
| 文件大小 | 160966886 字节 |
| SHA-256 | 422bf047b6cfec8c916614aa30464f737ebfb4062d3020b7ae70fbeca1afc873 |

1. 完成 Windows 开发 CA 安装，确认 29880、29885 可达。
2. 从内部下载页取得 Windows 安装包，或使用项目根目录下的本地文件。
3. 安装前可用 PowerShell 的 Get-FileHash 核对 SHA-256；当前测试包未做 Windows 代码签名，应按组织安装规范核验来源。
4. 完全退出旧客户端，包括系统托盘，再运行安装包并完成安装。
5. 启动 OpenIM38 Enterprise，按原版登录页选择手机号或邮箱登录方式，使用已开通的普通通讯用户账号登录。
6. 首次登录等待 SDK 同步完成后再查看会话、好友和群组；同步期间不应把空列表直接判断为账号数据丢失。

```text
Get-FileHash -Algorithm SHA256 -LiteralPath '安装包所在目录\OpenIM38-Enterprise-3.8.7-win-x64.exe'
```

### Windows 3.8.7 本轮修复与复测

- 多人转发按批次顺序发送，保留失败接收者供重试。引用消息转发保留原引用内容，避免固定 SDK 在缺失引用目标时退出；普通文字和媒体仍沿原 SDK 发送。
- 顶部综合搜索和“聊天记录”结果可正常显示、选择并定位到对应会话和原消息；失败或切换搜索时保留可重试状态。搜索范围仍为当前账号已同步的本地消息。
- 视频附件下载保持原签名地址不变，缓存参数通过客户端内部传递；启动失败可重试，下载后可从受管缓存播放。保留正常证书校验。

### 保留的 3.8.6 修复（历史证据）

- 聊天窗口关闭到托盘、隐藏、最小化或未获得焦点时，不再仅因会话仍被选中而自动上报新消息已读；重新打开并查看对应会话后才按可见条件上报。服务器此前已经记录的已读不会倒退，请使用升级后新发送的消息复测。
- 工作圈本人点赞、取消点赞、评论和删除评论成功后立即更新列表与详情；发送失败保留评论草稿供重试。其他人的互动通过事件刷新，服务器缺少相应事件时由可见页面约 5 秒轮询补齐，实测取消点赞与删除合并显示为 5062 毫秒，不承诺严格 5 秒以内。
- 工作圈不再显示原资源中固定的黑蓝色加载截图；列表采用紧凑顶部，个人页保留头像和姓名。
- 修复截图确认后的重复进入截图框；确认或取消后结束本次截图，按原聊天工具使用截图结果。

安装或覆盖升级前，必须完全退出旧客户端，包括系统托盘，再运行 3.8.7 安装包。启动后在关于页核对 3.8.7 / 380701。当前仍是未作 Windows 代码签名的内网测试包，不包含 Android 或公网发布。

默认服务器已指向本内网测试实例。若之前保存过其他服务器配置，在登录页的 Config 配置中选择 HTTPS，并核对 IM API、Chat API 和 WSS 地址。界面协议前缀已单列时，输入框只填下表主机、端口和路径，避免出现重复 https://。

| 配置项 | 完整地址 | 协议前缀单列时的输入值 |
| --- | --- | --- |
| IM API | https://192.168.10.11:29880/api | 192.168.10.11:29880/api |
| Chat API | https://192.168.10.11:29880/chat | 192.168.10.11:29880/chat |
| IM WebSocket | wss://192.168.10.11:29880/ws | 192.168.10.11:29880/ws |

退出登录使用软件内原退出入口；请求失败时按页面提示重试。多端策略为 PC、移动、Web 类别并存，同一类别新登录会替换旧会话。更换账号时不要复制他人的本地数据库或令牌。

## 六 Android APK 路径和安装范围

Android 3.8.14+114 正常 lib/main.dart 入口的本地开发候选不等于已验收安装包；媒体状态为 NOT_ACCEPTED，尚无 Android 在线下载或更新记录。候选只存放于 artifacts/android-candidates/，正式 current 包仍标记 present:false，正式路径文件不存在。

### 114 当前本地开发候选

| 项目 | ARM64 手机候选（未经真机验收） |
|---|---|
| 本地相对路径 | artifacts/android-candidates/OpenIM38-3.8.14+114-development-arm64-candidate.apk |
| 版本 | 3.8.14+114 |
| 包名 | io.openim.upgrade38.test.dev |
| 最低系统 | Android API 24；target SDK 35 |
| 文件大小 | 76743141 字节 |
| SHA-256 | f6f946be8ffdf8a00d92c031cfa70e4a7b24bfa1108d838e1446d219e5745312 |

| 项目 | x86_64 模拟器候选 |
|---|---|
| 本地相对路径 | artifacts/android-candidates/OpenIM38-3.8.14+114-development-x64-candidate.apk |
| 版本 | 3.8.14+114 |
| 包名 | io.openim.upgrade38.test.dev |
| 最低系统 | Android API 24；target SDK 35 |
| 文件大小 | 82520892 字节 |
| SHA-256 | fed978e3e414121ce3afa5f789fee20a2a062c93f39cd87b13a664acc424993c |

两份114候选均为项目开发签名的正常main程序；包旁同名JSON记录构建输入、ABI、签名及验收限制。仅x64在专用AVD运行过，ARM64没有真机结果；媒体失败状态对两者均未解除。

### 113 历史非媒体候选

下表保留113历史候选及其真实校验值，不能当作114安装来源或本轮媒体验收证明。

| 项目 | ARM64 手机候选 |
| --- | --- |
| 本地相对路径 | artifacts/android-candidates/OpenIM38-3.8.13+113-development-arm64-candidate.apk |
| 版本 | 3.8.13+113 |
| 包名 | io.openim.upgrade38.test.dev |
| 最低系统 | Android API 24；target SDK 35 |
| 文件大小 | 76677605 字节 |
| SHA-256 | de123b0e169fee5ce32ce45cc2cd9d605c4d8a39f470dcd8b05079dadbcaa5cb |

| 项目 | x86_64 模拟器候选 |
| --- | --- |
| 本地相对路径 | artifacts/android-candidates/OpenIM38-3.8.13+113-development-x64-candidate.apk |
| 版本 | 3.8.13+113 |
| 包名 | io.openim.upgrade38.test.dev |
| 最低系统 | Android API 24；target SDK 35 |
| 文件大小 | 82455356 字节 |
| SHA-256 | 347b2eaa592576fdbf59b47cea52eb730efe8595c375baecdbb053c08edb0ed4 |

两包均使用项目专用开发签名，正常 lib/main.dart 入口构建。ARM64 对应 arm64-v8a，x64 对应 x86_64，不应混装。包旁的同名 JSON 保存签名、来源和验收限制；校验表位于 artifacts/android-candidates/SHA256SUMS.txt。

1. 先确认测试设备 CPU ABI，普通 ARM64 手机选择 arm64 候选；x64 包用于对应模拟器。
2. 将本地 APK 复制到设备，按组织设备管理规范允许可信来源安装。普通用户不需要取得项目签名私钥。
3. 运行前先解决该手机的服务端证书信任条件；不要通过关闭 TLS 校验绕过。
4. 使用原版手机号或邮箱登录页及普通通讯账号。由于尚无 ARM64 真机，不能将 APK 构建成功视为真机登录或长期运行通过。

Android 当前没有在线下载链接，也没有推送应用更新记录。正式路径 artifacts/android/OpenIM38-3.8.14+114-development-arm64.apk 仍不存在，current 标记为 present:false；113历史正式路径同样不能作为安装或发布来源。已完成的 x64 保数据覆盖安装、缓存首页、关于页版本和无更新提示，只是对应模拟器上的局部验证；系统下载安装升级流程仍未验收。

## 七 Web 登录和日常通讯

1. 在已信任证书且网络可达的电脑浏览器打开 Web 通讯入口。
2. 选择原版手机号或邮箱登录方式，输入普通用户账号与密码；注册、找回密码涉及验证码时，需要实例已配置可用邮件或短信通道。
3. 进入原版会话页后，在联系人中查看好友或群组，搜索目标并打开聊天。原项目保留好友、新好友、黑名单和群组体系。
4. 使用原消息工具发送文字、图片、文件等内容；上传或下载异常时同时检查 29885、签名有效期和文件校验。
5. 切换设备或断网恢复后等待消息同步。需要退出时使用页面内退出功能。

保留的消息类型包括文字、图片、语音、视频、文件、位置、名片、引用、合并转发和自定义消息。撤回会同步会话显示，但不等于能收回对方另存、转存或截图。语音、视频及通话功能的本轮未验收状态见后文，不能因为入口存在就视为验证通过。

朋友圈、评论点赞、可见范围、标签发送、OA通知、群已读名单沿用原企业界面，并通过独立企业扩展服务适配。原先隐藏的入口维持原状态；原来未完成的组织树、UniMP和云录制没有扩展为新功能。Web仍以原版桌面界面为准，手机浏览器不视为与原生App等同验收。

## 八 后台管理操作

| 菜单位置 | 使用内容 |
| --- | --- |
| 业务系统 → 用户管理 → 用户列表 | 维护业务用户资料及原界面提供的新增、查询和状态操作 |
| 业务系统 → 用户管理 → 禁用列表 | 核对已禁用用户；启停和恢复以服务端实际结果为准 |
| 业务系统 → 注册管理 | 邀请码、默认好友、默认群组等原版注册配置 |
| 业务系统 → 应用管理 → 应用版本 | 维护平台、版本、构建号、安装包URL、大小和SHA-256 |
| IM系统 → 用户管理 | 查询IM用户、查看关系链 |
| IM系统 → 群组管理 | 查询群组和群成员，使用页面提供的管理操作 |
| IM系统 → 消息管理 | 查询用户/群消息，发送系统通知 |
| RTC系统 | 保留原会议和通话管理界面；已恢复验证，完整媒体验收尚未完成 |
| 设置 | 个人信息和管理员修改密码 |

建议先在业务用户列表建立或核对普通通讯用户，再到 IM 用户、关系链和群成员页核对同步结果。不要只创建一条 IM 记录就认定该用户已具备 Chat 登录凭据。后台可见菜单与实际管理员权限有关，以当前账号页面和服务端返回结果为准。

当前原版升级项目没有逐员工“公网登录、手机端、PC端”授权矩阵，也没有新增后台上传SSL证书功能；现有登录采用官方全局多端策略，服务端TLS证书由部署配置管理。

## 九 应用版本管理

1. 先完成目标平台安装包构建、运行验收及所需签名，再上传到与客户端配置匹配的 HTTPS 下载入口。
2. 计算真实文件字节数及 SHA-256；已发布文件不要覆盖，同一语义版本升级也要递增构建号。
3. 在“业务系统 → 应用管理 → 应用版本”选择平台，填写版本、构建号、HTTPS下载地址、字节数、64位SHA-256和更新说明。
4. 按测试或正式发布策略选择强制更新；可选更新允许稍后，强更会阻止进入业务直至完成有效更新。
5. 在旧版和当前版分别验证检查、下载失败重试、错误包拒绝及安装后的实际版本，保存结果。

Chat 保存版本主记录，企业服务补充构建号、大小和哈希。Windows采用完整EXE更新，旧ASAR热替换停用。无版本记录或查询失败允许当前版本继续运行；校验失败不得安装。当前 Windows 3.8.7 为已发布的内网测试包；Android114媒体未通过验收，113旧候选仅保留历史，两者均不可登记为已完成验收的 Android 发布版本。

## 十 服务端位置和日常启停

| 用途 | 位置或名称 |
| --- | --- |
| 服务器 | 192.168.10.11 |
| SSH | 端口2223；使用已授权服务器账号 |
| 部署根目录 | /srv/openim38-upgrade |
| Compose 项目名 | openim38-upgrade |
| Web 部署目录 | /srv/openim38-upgrade/www |
| 管理后台部署目录 | /srv/openim38-upgrade/www/admin |
| 客户端Web构建输出 | clients/web-desktop/dist-web |
| 管理后台构建输出 | clients/admin/dist |
| Linux程序 | artifacts/linux-amd64/；企业服务另在 services/enterprise-service/dist/linux-amd64/ |

以下命令在服务器上执行，只管理本实例。先进入固定部署根目录，不要在其他实例目录直接套用。

```text
cd /srv/openim38-upgrade
sudo bash deploy/manage.sh status
sudo bash deploy/manage.sh logs enterprise-service
sudo bash deploy/manage.sh nginx-check
```

需要停机或启动时执行对应命令；stop 会使当前测试实例不可用。

```text
sudo bash deploy/manage.sh stop
sudo bash deploy/manage.sh start
```

修改Nginx配置后先检查，再重新加载。业务程序更新需要按部署说明替换二进制并 force-recreate 对应服务；仅restart不能保证文件挂载已指向新程序。

```text
sudo bash deploy/manage.sh nginx-check
sudo bash deploy/manage.sh nginx-reload
```

## 十一 备份和恢复检查

```text
cd /srv/openim38-upgrade
sudo bash deploy/backup-test.sh --mode live
```

备份输出位于本实例 backups/：密文 .tar.enc、同名 .hmac.json 和 .sha256 必须一起保留，另行保存私有 backup.key。live 模式是跨写入时刻的逻辑导出，不是同一时点快照；当前格式不包含 Redis、Kafka、etcd 全部持久卷，不代表全栈零丢失恢复。

```text
sudo python3 deploy/backup-tool.py restore-check --archive /srv/openim38-upgrade/backups/<备份文件>.tar.enc
```

restore-check 仅在独立临时资源中演练数据库及附件恢复，不能代替生产切换。不要把私钥、备份密钥、登录附页或 .private 目录放入公开下载位置。测试数据重置会停止本栈并删除合成库和附件，日常使用不需要执行。

## 十二 文件目录速查

| 用途 | 项目根目录下的相对路径 |
| --- | --- |
| 本使用手册 | artifacts/manual/OpenIM38-原版升级使用说明.html |
| 管理员登录附页 | .private/manual/管理员登录信息.txt |
| Windows EXE | clients/web-desktop/release/OpenIM38-Enterprise-3.8.7-win-x64.exe |
| Android114 ARM64候选 | artifacts/android-candidates/OpenIM38-3.8.14+114-development-arm64-candidate.apk |
| Android114 x64候选 | artifacts/android-candidates/OpenIM38-3.8.14+114-development-x64-candidate.apk |
| Android113历史ARM64候选 | artifacts/android-candidates/OpenIM38-3.8.13+113-development-arm64-candidate.apk |
| Android113历史x64候选 | artifacts/android-candidates/OpenIM38-3.8.13+113-development-x64-candidate.apk |
| Android校验表 | artifacts/android-candidates/SHA256SUMS.txt |
| Windows证书与检测包 | artifacts/OpenIM38-Development-CA.zip |
| 程序与依赖版本清单 | artifacts/release-manifest.json |
| 正式交付文件校验表 | artifacts/SHA256SUMS.txt；候选APK单列在候选目录校验表 |
| 上游与运行时来源 | upstream-sources.lock.json；runtimes.lock.json；deploy/images.lock.json |
| 使用和构建补充 | docs/build-and-run.md；docs/web-desktop.md；docs/flutter.md |
| 验收清单 | docs/acceptance.md；docs/feature-matrix.md |
| 媒体恢复进度 | tests/delivery/media-resume-20261102/ |
| 先前媒体暂停归档 | tests/delivery/deferred-media-work-20261101.json |

## 十三 常见问题

| 现象 | 处理顺序 |
| --- | --- |
| 打不开网页或端口检测False | 确认当前网络属于允许网段，检查跨VLAN路由和服务器服务状态；不是先重置密码 |
| 证书或网络错误 | 先核对地址为本实例、端口可达、当前Windows用户已导入正确CA，再检查完整证书链和SAN |
| 后台密码错误 | 打开私有登录附页，确认使用后台管理员原始密码；若已改密，更新本地附页 |
| 普通用户登录失败 | 核对手机号/邮箱登录方式、用户是否已建立、密码与禁用状态；后台管理员不是普通通讯账号 |
| Android装了用户CA仍报错 | 固定Go核心不使用该证书库，先满足公信证书或验证过的系统信任条件 |
| 文本可发但附件失败 | 同时检查29885、签名URL是否过期、时间同步、下载文件大小及SHA |
| 同类设备被踢下线 | policy4允许三类别并存，同类别新登录替换旧会话；检查是否在另一同类设备登录 |
| 新登录会话列表为空 | 等待SDK完成首轮同步，检查WebSocket及同步状态后再判断数据问题 |
| 找不到Android在线下载 | 当前只有本地候选APK；内部下载页没有Android发布链接 |
| 找不到既有隐藏入口 | 保持原项目隐藏状态，不把隐藏入口或空接口当作已完成新功能 |

## 十四 已验证范围和保留事项

2026年11月2日，Windows 3.8.7 本轮94项适配测试及TypeScript检查通过。合成账号真实Electron引用消息转发给5名接收者后，SDK会话与历史查询保持可用；搜索有5项原界面和10项真实消息Store回归；视频有9项行为测试、原生合成MP4下载、Range读取、解码及正常TLS下载证据。最终Windows安装包构建及对应打包程序的正常TLS、SDK初始化、受管文件Range巡检通过。原生下载工具使用独立隐藏窗口和合成数据，不能替代所有历史附件、显卡或网络条件下的整体验收。覆盖安装后仍应核对关于页版本。

- 保留3.8.6已读、工作圈和截图修复，以及原57项适配和对应界面/打包证据；历史记录未改成3.8.7。
- 历史100个真实SDK持续至少30分钟、5700条发送和5700条接收的结果保留；本轮没有重新执行这一压力测试。
- Android114完整163项Dart、108项Java边界测试及62个Java文件的实际Android编译通过；正常main候选已构建。源码测试和编译结果不等于通话验收通过。
- Android114单端快速会议的普通Leave、重新进入，以及End后Ended状态和再次进入拒绝通过限定原界面验证；三人通话退出的Room清理耗时121023毫秒并失败。后续17项物理清理任务最终全部完成，最终摄像头为空，不能据此把退出超时改判通过。媒体状态为NOT_ACCEPTED。
- 定向原生诊断只用于分析失败，不是第二轮正常媒体验收；未符号化偏移不能确定具体根因。预约会议、纯语音通话、完整跨端矩阵和连续正常通话退出仍未完成。
- Android113旧候选的42项非媒体结果仅归属于113；旧群退出超时等失败及暂停归档继续保留，不能用114源码测试覆盖原失败结论。
- 没有ARM64真机运行结论；普通手机证书信任、Android系统安装升级及物理设备媒体效果仍待验收。
- iOS提供源码和Mac/Xcode说明，本机没有完成Xcode构建；Windows测试包未做代码签名。
- 当前成果仍为原企业项目与固定官方版本合并的开发版本，全部约定功能通过前，不标记整体升级完成。

本轮Windows证据见 tests/delivery/windows-387-quote-facade-review.json、windows-387-search-review.json、windows-387-video-download-review.json及最终聚合报告；Android114媒体汇总见 tests/delivery/android-114-media-resume-review.json，详细记录见 tests/delivery/media-resume-20261102/。历史暂停记录仍见 tests/delivery/deferred-media-work-20261101.json，仅表示当时状态，不代表本轮继续暂停。

公开HTML使用说明的校验表位于 artifacts/manual/SHA256SUMS.txt；管理员登录附页不进入公开校验清单。
