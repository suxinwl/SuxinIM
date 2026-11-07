# PUTY 内部通讯：安卓后台推送开通

更新：2026-11-05。正式安卓版本为 **3.8.21 / 121**。该版本修复 APP 内更新缓存，并包含 3.8.20 的后台返回恢复等修复，尚未启用离线消息和来电推送。后台推送接入候选正在开发，以下准备工作不会自行启用生产通道。

## 统一应用身份

各平台创建或选择同一款 Android 应用，保留既有升级身份：

| 项目 | 填写内容 |
| --- | --- |
| 应用名称 | PUTY内部通讯 |
| Android 包名 | `com.szputy.im` |
| 生产签名 SHA-256 | `521723f5ecfd7ab4a7cc94f2cad50509e0d3bbc1bd4a565ad44b58c1fc12272f` |
| 图标 | 现有 IM 应用图标 |

SHA-256 来自已交付 APK 的签名校验；需要带冒号的界面可每两个十六进制字符插入一个冒号。不要换成调试签名，也不要创建不同包名来代替现有应用。

## 由公司开发者账号持有人完成

1. 在[个推开发者中心](https://dev.getui.com/)创建 Android 推送应用，取得 AppID、AppKey、MasterSecret。创建后，在该应用的厂商设置中关联下述厂商配置。
2. 在[华为 AppGallery Connect](https://developer.huawei.com/consumer/cn/service/josp/agc/index.html)选择或创建对应 Android 应用，登记生产签名，在项目的推送服务中开通 Push Kit，下载该应用的 `agconnect-services.json`。取得 AppID、服务器用 Client Secret，并申请业务适用的消息与通话分类；保存实际获批结果。
3. 在[小米开放平台](https://dev.xiaomi.com/)的消息推送中选择或创建手机应用，开通推送，取得 AppID、AppKey、AppSecret；申请普通消息、群消息和来电所用渠道及模板，保存批准的渠道 ID、模板 ID。按当前个推开通文档，未在小米应用市场上架的应用可能需要完成上架审核才能启用，内部应用需先向平台确认可用接入方式；本次工作不自动提交应用商店发布。
4. 在 [OPPO 推送平台](https://push.oppo.com/)创建对应应用并开通推送，取得 AppID、AppKey、AppSecret、MasterSecret，申请普通消息与来电的渠道、分类及模板，保存实际获批结果。

菜单可能随平台更新，以公司账号登录后的界面为准。[厂商开通指南](https://docs.getui.com/getui/mobile/vendor/vendor_open/)介绍了上述应用创建、签名与厂商信息关联步骤。

## 填写受限配置文件

服务器及客户端配置统一由项目根下 `.private/production/push-registration.json` 生成。该文件已限制本机访问权限，不提交 Git。账号持有人可直接在本机填写；不要将密钥发到聊天、截图或构建日志。华为下载的配置文件也请存入项目的 `.private/production/` 受限目录，再填写其完整路径；校验工具不会从普通下载目录直接读取它。

| 配置区域 | 对应平台信息 |
| --- | --- |
| `getui` | `applicationID`、`serverAppKey`、`serverMasterSecret`；厂商信息配置完成后将 `consoleConfigured` 设为 `true` |
| `huawei` | `applicationID`、`agconnectClientConfigPath`（下载文件的本机完整路径）、`serverOAuthClientSecret`、获批 `messageCategory` / `callCategory` |
| `xiaomi` | `applicationID`、`clientAppKey`、`serverAppSecret`；三类 `message` / `group` / `call` 的 `ChannelID` 和 `TemplateID` |
| `oppo` | `applicationID`、`clientAppKey`、`clientSdkAppSecret`、`serverMasterSecret`；`message` / `call` 的 `ChannelID`、`Category`、`TemplateID` |

顶层 `status` 在参数完成后改为 `configured`。每个厂商的 `pushServiceEnabled` 仅在该应用推送服务已开通后设为 `true`。模板参数使用 `staticTemplateParams`，当前仅放静态应用名 `app_name`；不要填员工姓名、聊天正文或任意动态占位内容。分类和模板值必须来自控制台批准结果，不照抄示例值。

Huawei `serverOAuthClientSecret`、Getui 服务端凭据、小米服务器 AppSecret、OPPO MasterSecret 只供服务器使用。OPPO `clientSdkAppSecret` 是其 Android SDK 注册参数，不能误填成服务器 MasterSecret。华为下载文件也不直接整份放进 APK；构建时提取经核验的必要客户端字段。缺少应用参数或获批通道的厂商保持关闭。

## 配置完成后的工程步骤

1. 使用受限配置生成服务端配置、Android 客户端必要参数，检查包名、签名与华为配置匹配；输出缺项清单，避免回显原始凭据。
2. 构建保留原包名与签名的新 APK，扫描服务端秘密值，备份并部署服务端通知配置；未完成通道配置的版本不标记为“后台推送已启用”。
3. 在华为和小米真机覆盖升级，允许系统通知，验证前台、后台、锁屏、系统回收进程、点击通知及退出/切换账号。手机需要能连接相应推送网络；打开内部通讯还需要能连接公司的内网入口。
4. 分别验证单聊、群聊、语音、视频、取消、超时、其他设备接听和撤权。点击通知后须在线核验当前账号、权限及通话状态，才能进入会话；不自动开启摄像头或麦克风。

来电通知、锁屏显示和响铃受手机系统、厂商通道及用户通知设置影响。厂商纯透传不保证唤醒已被杀死的进程；点击厂商通知可作为恢复入口。完整锁屏来电能力需按实际获批能力和真机结果验收，不能将服务端“已提交”当作手机“已收到”。参见[厂商 SDK 集成说明](https://docs.getui.com/getui/mobile/vendor/androidstudio/)和 [Android 全屏通知权限说明](https://source.android.com/docs/core/permissions/fsi-limits)。
