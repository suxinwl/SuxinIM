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

# 生产验收与历史记录

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

2026-11-05，HRM 部门编辑的负责人加载修复已上线。候选资格改为分块实时核验，保留账号状态、凭据指纹、有效权限和业务范围检查；前端按需加载，15 秒超时后显示失败及重试，关闭或新搜索取消过期请求，并保留当前负责人回显。线上同一部门的 300 名候选首页从 8.762 秒降至 0.176 秒，原首页内容及总数一致，第二页、工号搜索、业务隔离和实际 HTTPS 页面资源校验通过。6 项组件/API 检查、4 项真实 Arco 浏览器检查、19 组后端定向及既有回归和前后端构建通过；旧实现 SQL 计数仅为被观测查询的下界，不作为全部查询精确总数。已备份并更新 HRM 宿主与页面，宿主重启后重新正常登录验证；未修改正式部门或负责人。私有证据索引：`department-leader-20261105`。

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

本轮 54 项迁移/诊断回归、真实旧算法失败后原位重试 1 项和双 TypeScript 检查通过；旧代码因深层普通文字失败后已 ready 的附件，在修复后保留原 taskID、名称、路径、inode 与密文，不增加重名编号。实际 SDK 表结构的 Node 用例还覆盖引用/合并转发、删除态 latest/quote、错误归属、损坏数据库和已有目标档案拒绝。最终打包 Main 经合成预封存快照的清理、导入及重开验证：5 条 SQL 行、4 条月档记录、1 个附件，40 层 JSON 文字/latest_msg 保真，65 个原密封文件未改，其他账号未导入；使用合成会话授权，未进行真实 SSO 或 SDK 登录，没有生产请求、员工资料、桌面或剪贴板访问，也未把该场景记作本轮 cold-reader 重跑。最终安装包 SHA256 为 `bb0320753d2417c3622bf266b56d4c28bb0a6e53d59e227efdd57c29b5fac095`；68 项实际秘密模式零命中，16 个 Main/preload 源图输入匹配。证据索引在私有 `legacy-import-20261104`。

PC 3.8.15 历史验收记录：新增可配置截图快捷键。31 项 manager、IPC 与加密偏好回归、双 TypeScript 及 12 项真实 Electron GUI/原生注册/DPAPI 重启检查通过；录制暂时释放快捷键，冲突保留旧键及原密文，窗口失焦不取消全局注册，退出释放自有键。本机默认 Ctrl + Alt + A 被其他程序占用的真实冲突和自有 F8 预占冲突分别保留证据。最终打包 Main 另经 9 项限定检查，两次隔网正常入口登录页、原 preload/IPC、真实 F9 注册/禁用、密文重启恢复和退出释放均通过，ASAR 前后不变。使用合成目录且未登录、读取桌面/系统剪贴板、发送消息或上传，物理全局键触发与截图硬件不在该检查范围。最终包 68 项实际敏感模式零命中、16 个源图输入匹配，安装包 SHA256 `e307a3bf5a6cfb27663d6633ea470b000796d3ad37e1d2a3399a5b3648552d55`。本轮正常后台创建 Windows15 记录 code0、force=false；服务器发布器旧清单/首页 CAS 验证后发布，保留旧包并生成独立备份，无服务重启。下载与实际发布证据通过独立只读复核。完成后维护 SSH 正常退出，管理员正常登出并由 getUserinfo code401 确认旧会话失效。

Windows19 首次发布时交付为 **Web 3.8.13、Windows 3.8.19 / 381901、Android 3.8.18 / 118（R4）**，均按非强制发布策略登记（`force=false`）。本轮只新增 Windows19 版本记录；Web/Android 的交付文件和 Android18 版本记录保持。6 项两入口下载检查通过，包含 LAN 完整包 SHA、HEAD/Range、更新元数据及当前版本不重复提示；域名 SNI 经 LAN，不构成新增外网 NAT 验收。公开清单 SHA256 为 `ae3effc13128eddb641db86256c3b4f7cd8cb803322161a21e2ccaa1b531190e`，下载首页 SHA256 为 `8d7e2d154e19f1197267e4d2e572454f2ee083086f018351d67fbea63da9a59a`。实际交付以 [客户端说明](../clients/PRODUCTION-DELIVERY.md) 和公开清单为准。

- Android 公开 ARM64 包 SHA256 为 `1750e79b13affea71309a03b39b20c161dae0a2c73f5b1f9822f6666857c1689`；正常主入口实际验证使用同源正式 x64 包 `ad8a902662c13b6abd93df0376e706321a346ae2fd40e7757efc1df04fda6f6a`，该 x64 包不公开。两 ABI 各自完成 68 项实际生产秘密模式的 RAM 扫描，零命中；包静态检查与正常主入口证据分开记录。
- R4 正常主入口原始收据 `07869…`、Root 最终封装收据 `612…` 记录：初始会话前台 409.47 秒、冷启动后第二会话连续前台 251.39 秒（排除 SAF 导航时段），未观察到意外返回登录。正常加密备份取消及实际保存、鉴权预览、实际 native promotion、fresh J 登录、新 PNG 导入与保护读取、正常退出均完成；promotion 的目录 dev/inode 与受保护 `master.wrap` 均改变，不能只凭返回登录页判断恢复成功。
- 合成 J 到 I 的单会话设备清理覆盖 0 条聊天消息、3 条本机导入、3 个引用和 3 个文件，预览预计释放 17748 字节；清理后列表完成为空，恢复后旧三项没有复活。17748 是预计释放量，不是独立磁盘实测；没有把该结果扩展为非零聊天消息删除或设备多会话/群组联合清理通过。相机首帧、重拍取消和已有 PNG/MP4 预览在本轮正常入口中完成，未声称物理硬件或流畅运动验收。
- `sdkSyncReady` 依据实际 SDK 会话入口及恢复后成功执行受 `authorized && viewsReady` 同步门控的 native 写入、保护读取推论；没有直接持久记录 `onSyncServerFinish`，也不从空 Main 页面推断就绪。Dart 301 项和 native SQLCipher 46 项沿用已固定的原实际日志（R4 未重新运行）；Java 备份保存权限 whole-class 12/12、授权竞态 7/7 离线回归通过，不能替代设备主入口证据。
- Windows14 的 68 项实际敏感值模式零命中、11 个 Main 源图输入匹配、正常生产合成账号主入口 5/5 及下载 6 项检查记录保留；Windows13 的迁移修复和员工确认记录保留。
- ARM64 物理设备、iOS 编译/签名/真机、Android7 设备、多范围设备清理和非零聊天消息设备删除仍未验。Android7 的其他功能保留，加密拍摄要求 Android8 / API26 及以上。本轮两个测试员工经原有零业务引用、零外键和撤权守卫完成清理，正常后台 API 已确认账号及员工条目消失，停用投影与安全状态保留；唯一 4096 字节测试附件已精确删除。

## 2026-11-03 当前生产：HRM 宿主与资源访问

当前生产部署位于 `192.168.10.11`，HRM 入口为
`https://hr.szputy.com:9880` / `https://192.168.10.11:9880`，通讯入口为
`https://im.szputy.com:9885` / `https://192.168.10.11:9885`。本节记录本轮已经
完成的宿主和维护工具验证；客户端最终发布版本与包摘要以本轮生产发布清单为准。
下文 Windows 3.8.7、Android 114 及旧环境的发布/未发布声明均为原日期的历史记录，
不能据此判断本轮生产客户端状态。

用户已从独立外网确认 HRM 和 IM 两个域名入口均可显示登录页。服务器本机与 LAN
连接下的域名 SNI 检查另存；这些入口检查不扩展为音视频 NAT 或物理设备通过。

- 八类项目运行密钥已在受控维护窗口实际轮换，新认证通过、旧认证拒绝；50 条
  三类受保护文档均以新会话密钥解密通过、旧密钥解密拒绝。服务器持久提交与签名
  回执校验后，本机 14 份生成配置同步完成，原上线时间和排除项保持一致。
  独立源审与 24 项离线轮换测试通过。轮换后的 54 项正常验证码、三平台会话、
  原生认证、signaling、WSS 双向推送与历史、刷新、旧签名拒绝、附件和 Range
  复验全部通过，并精确注销本轮 SID。首轮暂态认证 RPC DeadlineExceeded 导致的
  503 失败单独保留，未覆盖原始报告或通过重试记录。
- Windows 3.8.8 与 Android 3.8.15 正式包已发布至 `/downloads/` 并登记更新记录。
  修正生产 Nginx 下载目录缺少 index 精确路由后，LAN 与域名 SNI 的页面、HEAD、
  Range 206、入口对应更新地址、版本、构建号、大小和 SHA 元数据均通过；LAN
  两个完整 HTTP 下载的 SHA256 与冻结清单逐一一致。此下载检查连接服务器 LAN，
  与用户提供的独立外网登录页确认分别记录。
- 最终精确测试清理已完成：原身份四个唯一对象，以及追加七条文件 alias/四个底层
  对象均有完整名称、正文哈希和引用检查；追加清理 81 项检查通过。两个合成员工
  已经关闭五权限、停用、收敛 disabled 投影并软删宿主账号，零业务引用和零外键的
  事务只删除两条员工行；审计、epoch、投影与历史保留。正常三接口最终读取确认
  HRM 302 名真实员工（active 301、resigned 1），QA 员工零条。真实通讯投影
  300 ready 且已应用，另有一名 active 员工缺企业微信显式工号、一名 resigned
  员工停用等待；两个 disabled QA 投影另算，不混入真实人数。
- 清理后旧凭据另做 14 项真实拒绝复验：A Android、A Windows、B Web 的旧原生
  token、Chat token、企业请求和 opaque SSO 刷新均拒绝，两个旧附件 grant 也返回
  401，没有创建新会话。原生 kick tombstone 的 1506 与不存在记录的 1507 均为源码
  明确的拒绝合同；最初只认 1507 的断言失败单独保留，没有将实际拒绝误写为放行。
- 最终冷备恢复原服务后，24 个容器运行、存储健康，未注册且不可用的 Chat/IM JWT
  实际入口探针一次通过；探针不读取任何注册凭据，覆盖 Chat RPC/Redis 与原生
  Auth RPC/Redis，不宣称触达 enterprise 私有 IM 连接或 CheckLoginForbidden Mongo
  路径。正常管理员验证码重登、通讯员工列表与群查询均成功。最后经正常宿主退出
  接口注销，再读 getUserinfo 被拒绝（宿主协议 HTTP 200、业务 code 401）。

- 统一身份、消息、群管理、附件与生命周期的最终独立报告共 138 项检查通过，
  企业服务 SHA256 为 `f1987d871eb8bea6497cd561beb9633017c1416b296098a0dfa2770d83f00003`。
  覆盖真实验证码登录、三端凭证、混配拒绝、单聊与群聊实时/历史、群公告、禁言与解禁、
  授权上传/GET/HEAD/206、刷新与并发刷新、同平台替换、登出，以及五类账号生命周期。
  员工失活和离职后的原生 token 分别在 23.75 秒、25.91 秒失效，均在 60 秒内；
  业务访问检查直接读取员工状态。恢复后新登录正常，旧凭据不复活；延迟旧 epoch/SID
  撤销不会误伤新会话。报告保留在受限 acceptance 目录，早期诊断失败另存，未覆盖。
- 宝塔全局代理缓存曾影响受管分段下载，已在生产业务 server 禁用继承缓存。
  同一授权的公开入口与企业服务直连均实测 `206`，指定的字节数及内容一致。
- 空群列表 API 已规范为 JSON 数组，修复首次 SDK 同步的 `null.map`；仅更新
  `openim-api`，SHA256 为 `150c723472ebb51085e18f93e048853ee4f13a1286bb91db056a4a805db48f36`。
  最终 Windows 实际 SDK 检查已确认 worker 异常消失。
- 补齐按序号物理删消息的 RPC 管理员校验，并把同步删除的会话归属查询移到物理
  写入之前；普通或匿名调用、伪造 owner、未拥有会话均不得写入，合法删除与通知保留。
  `openim-rpc-msg` SHA256 为 `a2ffa143edcbc8a564c5e33c834484d9d1f4a24ab63b16b5a6cfe1c6eb452c2b`。
  两个公开物理删除管理入口由 Nginx 拒绝，LAN/WAN SNI 四项 POST 均实测 403；
  宿主后台经受控私桥调用原管理能力。更新后另做原生消息历史、拒绝与精确登出复查。
- Windows 3.8.8 最终安装包 `ac26cf03d5893bef3931cedfd326862b68a39c4b49a5c3fd701aa3672b7d8864`
  已正常 NSIS 覆盖安装，安装后的 app.asar 与打包内容逐字节一致。正常 TLS 下 fresh
  SSO/立即刷新、WSS、23 项 SDK API、品牌读取和真实 4096 字节 SDK 文件上传通过；
  文件再授权成功，受管 Range 返回 206，128 字节逐字节一致。安装包未做 Authenticode
  签名；该结果不代表音视频硬件或 Android/iOS 实机已验收。
- Android 3.8.15+115 的正式 ARM64 安装包 SHA256 为
  `2907650b937c5b5149815a72bff7416da9b3a321ff4bc1ab0785acafab97e570`，
  包名、正式 v2 签名、仅 ARM64 原生库、固定企业根及 Java/Dart 新兼容代码均已实际
  检查。Flutter 191 项、证书边界 15 项和原生 PKIX 9 项测试通过。未修改系统根证书
  的 API30/Android11 上，同正式身份和签名的 x64 包通过正常主入口验证码、HRM 登录、
  Go SDK HTTPS/WSS 同步、会话校验、聊天历史，以及正常 UI 的唯一 4096 字节文件发送。
  接收端持久历史、再授权、完整 SHA 和 128 字节 Range 206 核对通过，之后正常退出。
  首轮后台鉴权暂态 503 的失败证据独立保留，同一安装包重试通过，未用匿名探针替代
  正式主入口验收。此结果不扩展为 ARM 真机、完整音视频或 iOS 验收。
- HRM 使用原生 Vue/Arco 通讯后台，已在正常 Windows 系统证书信任下完成生产只读
  浏览器检查：九个功能分栏、宿主/HR/IM 品牌、员工专用列及缺工号原因正常，无控制台
  错误。列表保留操作需要的内部标识于内存，默认不显示投影或认证指纹。
- 宿主 main SHA256 为
  `d2643ce86d97de5cb193fccd0bd91fe6ea18baa2d6c3c627d22926abfcea1ad6`。
  本轮修复使 HRM 资源 cookie 独立于 `GOSUXIN_HRM_PUBLIC_ADMINS_ONLY` 开关，
  原始 uploads 同时受宿主静态访问检查和 Nginx `auth_request` 保护。登录凭据绑定
  登录时的密码指纹与 epoch；读取资源不刷新会话。普通员工按现有权限读取自身/目录
  头像，共享附件须有对应附件库或 HRM 管理权限，且匹配已登记本地资源。公开品牌及
  enterprise 私桥头像授权保持独立。
- 正常系统 CA 和域名校验下，LAN/WAN 合成 PNG 附件共 24 项真实 HTTP 检查通过：
  有效 cookie 200、正文 SHA 一致、匿名/无效 cookie/伪造原授权头 401、HEAD 200、
  Range 206、缓存参数读取、品牌匿名公开及 private/no-store。通过原上传接口创建，
  再以精确附件 ID 调原删除接口清理，两个入口都确认 404。没有改动真实员工或附件。
  生产共享管理员会话没有为此注销；禁用、改密、epoch、登出的旧凭据拒绝另由本轮
  Go 回归验证，不将其称为生产共享管理员的现场注销测试。
- `go test ./internal/imbridge ./internal/logic/admin_user ./utility/auth ./internal/router`
  通过，并仅重编宿主 main，插件及 Web 包无额外变化。当前宿主认证缓存仍为内存，
  维护重启后须正常验证码重新登录。
- 冷备工具已补入原始 `activation.json`，本地与远端隔离恢复均校验 instanceID、
  数据策略、带时区的 startAt 和运行环境上线下限完全一致；34 项离线回归及语法检查
  通过。首份真实加密冷备含 5,385 项文件和六个数据卷，已在无网络、无公开端口的
  临时数据库完成恢复：71 张 MySQL 基表完整性与行数、三库 Mongo 文档数量及
  collection validate、全部冷文件和 Minio 哈希均通过，临时资源清理通过，生产未改。
  初次失败记录单独保留；已修复临时初始化进程过早就绪的检查缺口，并用严格逐表
  `CHECK TABLE` 适配镜像没有 mysqlcheck 的实际情况。最终交付后的新备份另记。

最终交付加密冷备为 `puty-20261102T202034Z-0dd1e269.putybak`，1,279,659,504 字节，
SHA256 `e97b7ba38ec1cc9cf0792ae9f81b57344d50a7959b75d6a7bac400d32edd7c48`。
包含运行密钥轮换、安装包与版本发布及精确测试清理后的 5,389 项文件、六个冷数据卷。
匹配的隔离恢复报告于北京时间 2026-11-03 04:31:17 完成，71 张 MySQL 基表、
三库 Mongo、全部冷文件/Minio 哈希、运行密钥材料与原上线点均通过；网络 none、
无公开端口、productionModified=false、cleanupVerified=true。此新归档替代早期归档
作为本轮交付恢复基线，独立恢复密钥不得与公开安装包混放。

新同步员工的宿主登录账号为显式工号 `employee_no`，组织同步创建的 `username` 与之
一致；IM 也接受无冲突的宿主账号或工号。同步时随机初始凭据会被丢弃，没有统一默认
密码，也不会覆盖已有员工密码。管理员应在 HRM 原账户表单为员工设置可用初始密码，
通过独立安全渠道发放；首次登录后员工使用原个人设置主动修改密码，当前没有强制
首登改密拦截。通讯投影 ready 仅说明资料与权限已同步，不代表已发放登录凭据。

## 历史开发验收（截至 2026-11-02）

以下为当时基于固定官方版本与原企业源码合并的开发版本记录。原记录中的“当前”仅指其原日期与环境，不覆盖上方本轮生产事实。只有全部约定用例通过，才能在对应范围标记完成。

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
