## 2026-11-06 Windows 3.8.22 / 382201 当前交付

当前线上版本为 Web 3.8.20 / 382001、Windows 3.8.22 / 382201、Android 3.8.21 / 121（小米安装器风险提示待复核）。本次只发布 Windows 更新与下载页说明，不更新 Web 静态站点或安卓版本接口；下载页保留安卓风险复核提示。

修正“我的好友”只有一个姓名分组时同时显示左侧分组字母和右侧定位字母的重复视觉问题。例如只有“姜示例”时不再出现两个 J。多个分组保留分组标题与定位字母，索引相对于列表居中定位。好友数据、姓名、人员资料卡及原有存储功能不变。改动位于 Web/PC 共用源码，网页版修正尚未部署。

TypeScript 检查通过；使用真实 MyFriends / AlphabetIndex / FriendListItem / formatContacts / Virtuoso 的隔离浏览器验证空列表、单 J 分组、资料卡事件、多分组定位与数据切换通过。仅头像、翻译、store 和事件收集使用合成边界，无员工登录或生产请求；截图保存在受限验收目录 friend-index-20261106。最终 ASAR 验证两个实际打包页面源码与被测源码一致、旧版原生代码和固定 WASM/SQL 依赖保留、企业根证书一致、已知生产秘密值零命中。安装包保留应用 ID 和原 NSIS 升级配置，Windows 代码签名仍未配置。旧构建资产混入的第一候选已被包门拦截并保留，未发布；最终使用无继承合并的独立配置重新打包。

构建与包输出位于 D 盘，旧生成 dist-web 完整校验复制至 D 盘并以原路径 junction 保留；未读取或迁移员工本地聊天档案。覆盖安装请保留现有数据目录。

## 2026-11-06 Android 3.8.21 小米安装器风险提示：待复核

用户确认小米 13 可覆盖安装 3.8.21 / 121 并进入消息页，但在升级安装时由系统安装器报告 `a.gray.racketeerim`，且用户称旧版本此前未出现。**该版本真机安全验收未通过；相册、文件、图片即时刷新及后台恢复尚未收到确认。** 暂缓扩大推广，保留现有应用数据，不引导关闭安全检测或忽略告警。

已核对当前公网样本与签名交付包一致、两版生产签名一致；3.8.20 与 3.8.21 权限及除版本外的清单一致，无新增 SDK 文件。6 个 APK 成员改变，其余 885 个内容一致；新增 DEX 类是更新交接的防重复回调类，新增 DEX 字符串无 HTTP/HTTPS URL。此为静态来源和变化范围核查，不等同恶意软件全检，不能认定“无病毒”或“误报”。本机没有可用独立扫描引擎，未取得第三方多引擎结果，也未上传应用到外部扫描站点。

排查与待提交复核材料在受限目录 `.private/production/acceptance/android-121-security-20261106/`。仍需原机完整告警截图、检测方及版本信息，之后由应用主体向实际安全厂商复核；未提交申诉或另发声称消除告警的安装包。公开下载与版本接口本次未改，未影响 Windows/Web，未启用私有推送候选。

## 2026-11-05 Android 3.8.21 / 121 前次交付

当前交付为 **Web 3.8.20 / 382001、Windows 3.8.19 / 381901、Android ARM64 3.8.21 / 121**。安卓文件 `PUTY_IM-3.8.21-android-arm64.apk` 为 80526678 字节，SHA256 `6b6e5ab18d229a1b6771c3e861249a3b9ffd7edfa698cf1a9afb541815b43728`。原应用 ID、生产签名、固定 Go core 和 SQLCipher 保留。首次从旧版升级请经下载页覆盖安装，保留应用数据；旧版内的更新路径无法由服务器远程替换。

修复 APP 内更新错误调用 `IMUtils.createTempFile → NativeLocalVault.scratch` 的问题。旧调用在档案未解锁时确实抛出用户报告的“请联网验证身份后创建媒体”。公开安装包现使用应用私有缓存 `puty-public-installers/download-*`，无需账号档案授权；每个任务独立下载 `.apk.part`，校验完整大小和 SHA-256、flush 后改名，再交系统安装器。成功交接后保留文件供异步读取；失败只清当前任务的两个安装文件，再非递归删除空任务目录。普通聊天附件继续使用既有加密流程，公开更新传输限定可信 HTTPS `/downloads/*.apk`、禁止重定向且不附账号凭据。

原生交接补齐 content URI 的 ClipData 与临时只读授权，清理旋转或销毁 Activity 的旧引用，失败使用固定诊断码，MethodChannel 仅完成一次。原生成功回调表示已交给系统安装器，不代表用户已安装成功。

下载响应统一正确 APK MIME 与带源文件名的 attachment 响应，保留安全/CORS 头、HEAD、Range、404 和其他下载类型。实际内网及公网 DNS 共 14 项响应检查通过。此前用户的小米 13 报 packageinfo is null；新链接下载并从系统文件管理打开后，用户确认 **3.8.20 / 120 可覆盖安装**。此为该机该包反馈，不扩展为 121 实机验收，也不将响应头认定为唯一根因。

15 项 Dart 回归及 18 项原生安装交接检查通过；包含实际旧媒体调用报错、新流程在档案关闭时工作、大小/哈希失败、部分下载、同名并发、失败重试与真实 AndroidX FileProvider 读取。分析无错误、无警告，保留 3 项风格提示。最终双 ABI 均完成实际 APK 签名、清单/资源、ZIP CRC、固定 core、安装 DEX 交接与四个新 Dart 诊断标记检查，2949 项来源无漂移。第一轮因生成目录缓存导致缺少 libapp.so 的 63MB 候选被包门拦截、保留作失败证据，未发布；修复生成缓存后使用独立 rebuild-r2 重新构建并验收。已知秘密值检查零命中，范围和原始日志保留在私有发布收据。既有非核心库 16KB 页兼容限制保留，无新增 AVD 或真机安装断言。

后台推送代码和配置候选独立保留，**本次不包含也不启用后台消息/来电推送**。公司目前只有开发者账户，尚需同包名个推及厂商推送应用配置、获批通道/模板，再做真实后台与锁屏送达验收。准备步骤见 `docs/android-push-setup.md`。

# Web / Windows 3.8 升级（Web 3.8.20；Windows 3.8.19）

## 2026-11-05 Android 3.8.20 / 120 前次交付

当前正式交付为 **Web 3.8.20 / 382001、Windows 3.8.19 / 381901、Android ARM64 3.8.20 / 120**。Android 文件 `PUTY_IM-3.8.20-android-arm64.apk` 为 80526678 字节，SHA256 `6d35aaad27497581c7d4365530e480fb155c52661162cfd2a68691e9a92b08dc`。原应用 ID、生产签名、固定 Go core、SQLCipher 和企业根保持，既有数据按覆盖升级保留。

修复安卓相册、文件入口被旧存储权限前置拦截的问题；整块工具按钮可点，拒绝或取消后可重试。选择后继续进入加密发送缓存，并检查当前账号和会话代次。图片单元可响应 SDK 原位更新和上传完成，使用原生授权来源读取；解锁代次变化、退出或销毁后不再补画旧图。拍摄照片从相机内存直接进入既有可靠加密发送源，减少原 SQLCipher 临时中转与重复导入；视频流程、拍照质量及安卓 8.0 拍摄门槛保持。快门到预览的实际硬件耗时尚未测量。

修复锁屏、后台暂停或临时网络故障引起的短期档案授权过期被误当账号失效的问题。过期仍立即锁定档案，保留登录证据，前台重新联网核验后恢复；断网显示有正常文字样式和实色遮挡的重试页。手机回收进程后的自动启动、SDK 初始化失败重试、早发就绪事件、迟到的旧锁定通知及跨账号响应均有定向检查。真正的账号撤销或使用权限拒绝继续强制退出，8 秒档案租约及 3 秒续验保持。此项不等于后台消息推送或离线档案访问。

通话服务仅更新 enterprise-service：密封保存可信入口区域，仅后台巡检恢复该事实；当前 HTTP/WS 仍核本次入口，旧或非法事实按公网处理，当前撤权继续生效。运行可执行文件 SHA256 `b9bd031850b21184f4e5d427c352dc872756105dc7a7ab0ae7326f8e3af502f2`；原程序和清单备份已核，健康检查 HTTP 200。2026-11-05 用户确认：示例账号C 华为 Mate30 / HarmonyOS4.2、示例账号A 小米13 / HyperOS3 均用 Android3.8.19、内部通讯及 APP 前台，单人语音超过 1 分钟未再自动挂断。此为该用户实测范围，不扩展为视频、群通话、公网或后台验收。客户端三条信令路径改为保留原异常和堆栈、显示固定诊断码，避免 Future.catchError 的错误返回覆盖真实失败。

自动检查分组有重叠：最终 71 项 Dart（含 15 项新恢复及 20 项媒体）通过；未插桩实际 Broker 的 22 项及既有续验调度 7 项通过；照片 Java 19 项、信令与相关 RTC 43 项、服务端 119 项分别通过。旧源码对照实际重现了信令类型异常、后台误硬退出和通话入口事实丢失。2947 项最终来源在双 ABI 构建及实际 APK 检查后无漂移；DEX 已核 SDK 完成回调、照片反射桥和后台软硬分支。双 APK 与实际后端 ELF 全部成员经 51 项已知秘密值模式检查，1781 项成员零命中；此为限定已知值检查。分析无错误，保留已有警告；非核心扫码/保护库的既有 16KB 页兼容限制保留，不宣称所有 Android16 设备均已验收。

已新增且仅新增 Android120 非强制记录（force=false），旧 Android 记录、Windows19 记录及旧安装包保留。下载页 6 项检查通过，包括 LAN 完整字节和哈希、两入口 HEAD/Range、最新及当前版本查询。域名 SNI 检查经 LAN，不代表新增外网 NAT 验收。当前公开清单 SHA256 `3deb9ec9fbdcf33281afce0007c4b855d248097f7e9d4187bc71a2104956f4a7`，首页 SHA256 `f5e5eb789b8d4457dfe7b757ce2c0f23197c4470cbd09c40c801b6859aca32af`。x64 包仅作自动构建及静态验证；本轮未启动 AVD，Android120 真机、拍照耗时、视频和群通话仍待确认，iOS 原有条件限制保持。

用户新增的后台消息和来电推送需求单独接入。已核当前服务器与包未配置可用离线推送通道；用户有华为、小米、OPPO 开发者账户，但暂时没有此 APP 的推送应用配置。121 实现准备和合成测试在私有目录进行，未将其声明为已发布或已送达；不得以延长档案租约或常驻进程替代厂商推送。配置入口为受限的 `.private/production/push-registration.json`。发现页是后台配置的企业应用快捷入口；本轮读取时没有配置条目，工作圈仍是独立功能。

本轮证据索引：私有 `android-120-media-20261105/final-r3`、`rtc-backend-published-postcheck.private.json`、`rtc-field-voice-acceptance.private.json`、`downloads-final-acceptance.private.json`。原预 RTC 候选及漏纳测试 runner 的 r2 均保留为被替代记录，正式版本仅使用 final-r3。

## 此前登录与入口修复记录

2026-11-05 用户实机确认：示例账号B 在 192.168.14.8 上的网页及 PC 内部登录均正常；示例账号A 在小米 13 / Android16 上使用安卓 3.8.19（119）能进入消息页，内部通讯 / 公网通讯单选切换正常。本轮两项待确认的登录验收已通过，以下自动检查与历史设备测试保持各自范围。

2026-11-05 上一轮正式交付为 **Web 3.8.20 / 382001、Windows 3.8.19 / 381901、Android ARM64 3.8.19 / 119**。Web20 修复 localforage CommonJS 导入导致的登录入口白屏，匿名 IP / 域名入口实际加载通过；旧版白屏对照、无效合成 ticket 清理及重载检查通过，未重放员工 ticket。Windows19 保持原交付文件与版本记录。

Android19 登录页直接单选“内部通讯 / 公网通讯”，保留账号输入，忙碌时禁止重复切换与提交；配置保存或 SDK 切换失败时显示错误并保留可重试状态。修复实际原生 unInitSDK 成功及异常时没有完成 Flutter Result 的缺陷，初始化异常也完成错误回调；既有串行队列可正常继续。15 项 Dart / 界面相关检查、8 项原生生命周期回归和双 ABI 最终包静态检查通过；2937 项冻结来源无漂移，原 app ID、生产签名、Go core、SQLCipher 与企业根保持。最终 ARM64 包 SHA256 为 `6b91305afb871652ab46aa8e44886c174166023fae65e4a660d79e74485daa7d`。2026-11-05，用户按安卓 3.8.19（119）覆盖安装验收请求反馈：工号 示例账号A 在小米 13 / Android16 上已能进入消息页，“内部通讯 / 公网通讯”两个单选按钮切换正常，登录与切换的用户实机验收通过。该结论来自用户反馈；本轮自动模拟器未进入 APK 安装及应用界面，其失败证据保留。媒体验收未扩展，旧 Android18 r4 设备结论保持其原范围。

入口权限现由可信网关确认：私网来源通过 `192.168.10.11:9880/9885` IP 入口使用内部权限，`hr.szputy.com / im.szputy.com` 域名入口使用公网权限，真实来源 IP 仍单独保留。网关覆盖客户端自报标记，签名及 RPC 传播固定入口事实；缺失或不可信事实不能授予内网。覆盖 HRM 宿主、独立 biz-suite 插件 0.4.2-openim3.8 与 6 个 IM 后端服务，附件及会话复查保持该入口事实。当前插件 manifest 为 `ecdbf4e289586ea8ab49b7132e7db4d950e8946d9150b69706f9406b0940a47e`，原 5 个管理页面文件和全部 7 个非二进制文件保留。

实际生产只读验收 18 项通过：4 项真实 HRM IP / 域名 TLS 网关检查（含客户端伪造头覆盖）、2 项 IM 匿名 TLS 可达、12 项私有接口固定入口事实检查。私有检查证实 示例账号B 以 192.168.14.8 为来源时电脑网页及 PC 的内部授权允许，公网与缺失事实拒绝；未登录员工或创建员工会话。示例账号B 全部策略及安全数据与发布前一致。2026-11-05，用户确认工号 示例账号B 在 192.168.14.8 电脑上从 IP 入口进入的网页端与选择“内部通讯”的 PC 端均可正常登录，内部访问的用户实机验收通过。私有接口的模拟来源检查仍保留其原范围；域名 SNI 自动检查经 LAN，不代表新增独立外网 NAT 验收。宿主、6 个后端与插件的运行可执行文件哈希已核对。发布前原文件与元数据已落盘备份，8 项 / 24 场景的协调发布故障回归通过；此处不表示真实断电演练。

本轮仅新增 Android119 非强制版本记录（`force=false`），既有 Windows19 与 Android18 版本记录保留，旧包保留。下载页 6 项检查通过，包括两入口 HEAD / Range、更新元数据与 LAN 完整安装包哈希。该轮公开清单 SHA256 为 `6d50dafee6182b1e2533d1ebd28eba1dcdda709cbebce69f0489feac15afef86`，下载首页 SHA256 为 `7be2898c57dbf08dbeafbf1a0f27113a0d3504e4b1347f7ae6d45e6cfe52d4c0`。最终 ARM64 APK 与插件 ZIP 的全部解压成员经 51 项已知生产秘密模式检查，零命中；此为限定已知值检查。私有证据索引为 `entry-login-20261105`、`entry-zone-20261105` 与 `android-login-20261105`。

Windows 3.8.19 / 381901 已发布。PC 登录页直接单选“内部通讯 / 公网通讯”，不显示 IP 或域名；窗口右下角单按钮“修复存储”保留原修复流程。忙碌时禁用切换与修复，同值不刷新；正文可滚动，小窗口及缩放时底部按钮、版本号可见。网络选择沿用同窗重载逻辑，不承诺跨进程重启记忆。

最终包真实 Main/preload/IPC 的 7 项登录 UI 检查通过，1100×740、680×550 及 125% 三图已核对，无横向溢出；使用隔离合成目录、拦截网络，未登录 SSO/SDK 或读取员工资料。Renderer TypeScript 通过，最终产物 68 项秘密模式零命中、18 个源图输入匹配；Main/preload 的 38 个源码输入与 PC18 原包源图内嵌内容逐字节相同，存储、密码与迁移行为保留。本轮未重跑 PC18 迁移测试或 示例账号A 实机验收。证据见私有 `login-ui-20261105` 及 `login-layout-20261105/packaged-80c7d81f-d756-4154-8b42-04713d934c2c/report.json`。

Windows19 首次发布记录：非强制发布（`force=false`），旧 PC18 包保留，Web13 / Android18 不变，无服务重启；6 项下载检查通过，域名 SNI 经 LAN，不代表新增外网 NAT 验收。SSH 正常退出，管理员登出后 getUserinfo code401。公开清单 SHA256 `ae3effc13128eddb641db86256c3b4f7cd8cb803322161a21e2ccaa1b531190e`，首页 SHA256 `8d7e2d154e19f1197267e4d2e572454f2ee083086f018351d67fbea63da9a59a`；安装包 `PUTY_IM-3.8.19-win-x64.exe` 为 166462082 字节，SHA256 `024e2f3e37dad913821b1a14607bd82e5494090f7ba8dbc74bc62faab62c1f95`。

PC 3.8.18 历史修复与实机确认记录：Windows 3.8.18 / 381801 已发布，修复旧 SDK 实际数据库名与迁移器名称假设不一致的问题。真实 SQLiteFS 把文件 basename 交给 IndexedDB 后端，因此 emp_….sqlite 及安全前缀库名都合法；此前固定要求 -sql- 前缀会将完整库误报为页错误。新解析保留旧格式兼容，并将已有隔离快照按当前账号后缀重检；原清单、对象与密钥不改，仍要求本人身份、完整性、同账号候选一致及非空目标防覆盖。真正缺页或冲突继续隔离，不合并不同历史；旧版已使用非空新档案而尚未导入的安全前缀旧库，同样保留非空档案保护，需要进一步核对。新增名称、页索引、大小、二进制格式、数据库头、页几何、缺页及页长度八个固定诊断细码，保留原码兼容，不输出正文、旧路径或密钥。

本轮两份实际 SDK 实现的生产器回归 10/10、真实 IndexedDB/只读 reader/生产 pipe/封存恢复链 7/7、页原因映射与既有迁移回归、26 项诊断 UI/隐私及双 TypeScript 检查通过。真实旧生产器命名在旧代码上重现失败，新代码从原封存 raw 重检恢复；最终 PC18 打包 Main 经同机两个合成账号切换、正常退出后第二进程重开验证，原密封资料未改。以上均使用合成数据和授权，没有员工资料或真实 SSO/SDK 登录。最终包实际秘密模式检查 68 项零命中，18 个 Main/preload 源图输入匹配。安装包 SHA256 为 `e546caa4c7dc0c775e5a68e915357c0254bd8a01aadaf08a25ef0d86209059d1`；证据索引为私有 `pages-示例账号A-20261105`。

工号 示例账号A 的 PC 3.8.17 实机反馈仍失败，诊断为 RESOLVE_QUARANTINE_QUARANTINE_PAGES；之前的成功反馈已撤回。本轮明确修复了真实兼容缺陷，尚未读取该机原始库。2026-11-05，用户确认原报错电脑以工号 示例账号A 使用 PC 3.8.18 进入聊天主界面，登录阻塞的实机验收通过；历史消息与附件完整性尚未实机核验。同机曾使用 示例账号B、示例账号A，两个账号分别迁移与隔离的验证仅是合成验收。

PC 3.8.17 历史修复记录：Windows 3.8.17 / 381701 已发布，修复旧版同账号重复数据库被隔离后无法重检的问题。仅从已封存的加密原始快照恢复，当前账号的全部候选都通过完整性和本人身份核验，且唯一或逐字节相同时才恢复；内容冲突、归属不明、损坏和未知布局继续隔离并显示详细诊断码。同机登录过多个账号时分别处理，各自记录不混合。原密封清单、对象、密钥及既有非空档案保护保留。完成标记前每次认证全部原对象；完成后认证父清单和恢复资料，不声称会检测成功迁移后原始 raw 对象的所有位损坏。

本轮 19 项新恢复测试、诊断隐私与既有迁移回归及双 TypeScript 检查通过；实际 PC16 打包源码在同一合成快照上重现隔离，新代码恢复并重开成功。最终 PC17 打包 Main 的合成多账号恢复、切换及重开验证通过，原密封文件未改；使用合成授权，无员工资料、真实 SSO/SDK 登录或生产业务请求。最终包通过 68 项实际秘密模式内存检查且零命中，17 个 Main/preload 源图输入匹配。安装包 SHA256 为 `0277cbab966e55f1d944dd7ab8de2e34dd72c43798dec8bed4b11af2a2f6100b`；证据索引为私有 `quarantine-示例账号A-20261105`。

工号 示例账号A 员工电脑已由用户实机反馈：PC 3.8.17 仍无法进入聊天，提示 `PUTY_LEGACY_import_RESOLVE_QUARANTINE_QUARANTINE_PAGES`；实机恢复未通过。此前“已正常进入聊天”的反馈已由用户撤回；同机曾登录 示例账号B、示例账号A，这条事实已纳入合成测试，不能代替该机真实数据验收。请保留原目录、备份和密钥，完全退出旧客户端后覆盖升级。

PC 3.8.16 历史修复记录：Windows 3.8.16 / 381601 已发布，修复旧版历史迁移将普通 JSON 聊天文字误当内部消息结构递归解析的问题；文字原文保留，真正的引用和合并转发附件继续映射，已删除正文先清除。新增 11 个导入子阶段和封闭原因码，IPC 不暴露原路径、消息正文或底层异常。原账号身份、完整性、非空档案防覆盖和密封来源校验保持。该补丁处理待迁移及失败重试，不自动改写已完成的档案。2026-11-05，用户更正：同一台员工电脑（工号 示例账号A）仍无法进入聊天，PC 3.8.16 提示 MATCH_ACCOUNT_ACCOUNT_QUARANTINED；此前进入聊天的反馈已撤回，实机验收未通过，继续排查。

本应用由原 3.5 企业项目升级而来，保留原页面、资源与业务消息类型，运行于独立 OpenIM38 实例。原始目录没有修改。

当前 Web 为 3.8.13 / 381301，Windows 为 3.8.19 / 381901，Android 公开版为 3.8.18 / 118；以 [客户端交付说明](../clients/PRODUCTION-DELIVERY.md) 和 [实际公开清单](../clients/artifacts-manifest.json) 为准。PC 3.8.14 在备份与密钥恢复包导出前后及分块复制过程中复查保存权限，撤权后清理本次未完成输出，不覆盖已有用户文件。该版最终包的正常主入口 5/5、退出、隔离档案清理和公开下载校验通过，升级非强制。3.8.13 的迁移处理保持，用户已确认员工电脑安装 3.8.13 后可正常进入聊天；三种合成升级证据仍属于 3.8.13。安装器新选位置自动追加 puty_im，数据可另选专用目录，无需继续删除原文件与密钥。

Windows 3.8.15 / 381501 新增「个人设置 → 截图快捷键」，默认 Ctrl + Alt + A，支持修改、禁用及恢复默认；冲突或加密保存失败时保留原配置。31 项主进程与偏好回归、双 TypeScript、12 项真实 Electron 设置界面/原生注册/DPAPI 重启检查、9 项隔网打包 Main 限定检查及最终包审计通过。已实际发布安装包和非强制更新记录，两入口 6 项下载/更新检查通过后同步公开清单。未用物理全局按键触发或读取桌面、剪贴板作为验收。

以下 3.8.7 及更早的构建命令、依赖、文件名和验收记录按当时版本保留，不能用作当前发布清单。当前源码版本由 build-version.json 与 package.json 绑定，实际依赖以 package-lock.json 为准；公开清单仅在实际发布和下载验证后更新，开发构建不代表已上线。

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
