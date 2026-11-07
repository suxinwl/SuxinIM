> 当前升级项目：参阅[完整源码导航](../../README.md#源码导航)、[仓库说明](../../REPOSITORY.md)和[当前 Flutter / Android / iOS 构建说明](../../docs/flutter.md)。Android 与 iOS 共用 lib，平台工程分别位于 android 和 ios。
>
> 3.8.18+118 正在构建新的正式签名候选，尚未发布。最新完整 Flutter 回归 269/269 通过；实际 API30/x86_64 原生 SQLCipher 全套 46 个顶层用例及真实 SDK 数据库生命周期 3 项通过。独立恢复旧关闭方式的单例准确复现 PrepareCommit 的 database is closed；修复后 SDK 停写等待归档线程结束并保留保险库拥有的数据库句柄，最终锁定仍关闭句柄。新清理范围只抑制确认时刻之前的历史，已选中的时间偏差消息另保留精确 ID，避免吞掉清理之后的新消息。已存媒体改用受管原生 Range 预览；118 正常 Main、安装包散列及最终设备复验仍待完成。
>
> 私有 9e0 诊断包在同一进程捕获到续验 SocketTimeoutException 后立即锁定并回登录，未出现 SDK 踢人/过期/失效回调。新的有界重试仅用于仍有效的同身份、同凭据续验，最多重试一次，保留原 1500ms socket、2500ms 请求上限与 8 秒绝对租约；首次解锁与权限、身份、证书错误不重试。真实产品 helper 的 8 组/77 个 JVM 断言及独立 Broker/JNI 3 项代际竞争通过；新的私有 Main 验证仍待完成，相机和恢复不能因此记为验收通过。
>
> 117 私有诊断包的正常 Main 已完成独立 SAF 导入、同名全角（1）真实密文文件、PNG 内存预览、唯一合成 TXT 发送/预览/主动加密保存、完整备份导出及恢复校验取消；主动恢复前 4054 秒未复现自退。此前把恢复后回到登录页当作推广成功的结论已撤回：SDK 停写关闭了保险库共用数据库的生命周期边界正在诊断，实际推广及旧备份不复活仍未验收。首轮 XLSX 导入失败、同输入重试成功，原因仍未确定；相机预览占位图和已存 MP4 入口缺口不得记为通过。上述证据属于 117 诊断包，不替代最终 118 验收，也不代表视频/语音上传或 ARM 真机通过。
>
> 首份 118 正式签名候选在 API30 模拟器正常登录后持续在线超过 22 分钟，已验证既有 TXT 预览/加密保存、四次独立本机导入、实际全角重名密文文件、已存 MP4 解码播放结束及清理后同日新导入。相机照片仍显示错误占位，恢复推广尚未通过，因此该候选保持未发布；私有固定字段诊断包与原候选分开保存。
>
> 3.8.17+117 已生成本地全量加密签名候选，尚未发布：SQLCipher 工作库、按北京年月的密文档案、原名附件、联合多会话/日期本机清理及加密备份恢复。固定引擎 Flutter 回归 253 项、实际 API30/x86_64 原生存储 41 项、会话字段严格校验 157 项及独立原生代际竞态 3 项通过。ARM64/x64 两包已核正式签名、固定 SQLCipher/Go 核心库实际入包字节、根证书及敏感文件边界，散列见 [候选记录](build/acceptance/android-117-candidate-artifacts.json)。这不等于正常 Main、SAF、拍摄压缩或 ARM64 真机已验收。
>
> 用户已确认目前没有可连接的 ARM64 真机，因此 ARM64 真机验收保持未完成。本轮按已授权范围继续模拟器验收与候选发布；ARM64 包的编译、签名和静态核验不替代真机验证。
>
> Android 7.x 保留聊天、文件、相册与视频路径；加密拍摄要求 Android 8.0+ 并有明确提示。当前仅有 Android 11 模拟器测试环境，没有把源码兼容检查当作 7.x 设备通过。本人旧库迁移使用 streaming SQLCipher export；256 MiB 限制只属于未知/其他账号数据库隔离的 RAM snapshot，超限保留原件并失败关闭，详见 [native 存储说明](../../sdks/native-storage/README.md)。iOS 同步源码仍待 Mac/Xcode、签名与真机验收。
>
> 下方保留原项目历史说明；其中 Flutter 3.7.12 不代表当前固定工具链。当前 Android 发布版为 3.8.16+116，显示名 PUTY内部通讯、包文件短名 PUTY_IM；ARM64 正式包已上线，x64 包仅供模拟器验证，状态参阅 [客户端交付说明](../PRODUCTION-DELIVERY.md)。媒体状态 NOT_ACCEPTED；iOS 源码与图标已同步，但尚未完成 Xcode 构建或真机验收。
>
> 3.8.16 历史 Flutter 回归 204 项通过。新增组织架构移动布局、手机号/姓名/工号的员工搜索，以及服务端授权的同事资料与免好友私聊；系统消息底栏独立翻译为 消息 / Messages。品牌单源为 [brand.json](../brand.json)，[sync_brand.py](../tools/sync_brand.py) 同步 Android/iOS 构建显示名；运行时 appName 仍由匿名两字段公共配置读取，离线使用同一单源。应用 ID、正式签名和原数据目录保持兼容。
>
> 组织页、添加好友与全局搜索的 9 项真实界面搜索矩阵在仅修改 home 翻译前完成；最终 x64 包另验覆盖安装、设备 APK SHA、正常 main、Messages 底栏、组织精确查询及界面退出。最终报告 organization-android-final/report.json 保留这两个范围，未将最终包复核表述为 9 项全部重跑。
>
> Android 内网证书继续使用固定企业根的原生 TLS/PKIX 预检，再将完整验证后的同一叶证书 DER 绑定到唯一内网 HTTPS origin；15 项 Dart 边界测试、9 项原生证书测试通过。历史 3.8.15 的 API30 / Android 11 正式 main 已验证真实验证码登录、原生 SDK HTTPS/WSS、系统文件选择器上传、另一账号完整 SHA/206 核对及界面退出，证据见 build/acceptance/android-production-main.json。本轮组织与品牌的真实模拟器验证参阅交付说明。没有修改模拟器系统根；公网继续系统验证。模拟器结果不代表 ARM64 真机、完整音视频或 iOS 验收。

---

### OpenIM
A OpenIM flutter demo, only support android and ios.



### 当前项目运行环境：flutter stable3.7.12



### 源码使用

##### 1，替换 app icon

- ###### 将你的应用的icon设计图放到launcher_icon文件夹中，命名为app-icon.png

- ###### 执行命令:

```dart
  flutter pub get
  flutter pub run flutter_launcher_icons:main
```

##### 2， 替换 app 的名称

- ###### Android：修改 openim/android/app/src/main/res/AndroidManifest.xm文件里 android:label的值

```xml
  <application
    android:icon="@mipmap/ic_launcher"
    android:label="您的应用名"
    android:requestLegacyExternalStorage="true"
    android:usesCleartextTraffic="true"
    tools:replace="android:label">
</application>
```
- ###### ios：用xcode打开工程修改应用名

##### 3，替换 openim/openim_common/lib/src/config.dart 文件里的服务器地址为自己的服务器地址

##### 4，连接并选择真机

##### 5，执行  flutter pub get

##### 6，执行 flutter run



### Issues

##### 1，flutter版本是？

答：stable分支3.7.12

##### 2，支持哪些平台？

答：因为sdk的原因demo目前只能运行在android跟ios设备上

##### 3，android安装包debug可以运行但release启动白屏？

答：flutter的release包默认是开启了混淆，可以使用命令：flutter build release --no -shrink，如果此命令无效可如下操作

在android/app/build.gradle配置的release配置加入以下配置

```
release {
    minifyEnabled false
    useProguard false
    shrinkResources false
}
```

##### 4，代码必须混淆怎么办？

答：在混淆规则里加入以下规则

```
-keep class io.openim.**{*;}
-keep class open_im_sdk.**{*;}
-keep class open_im_sdk_callback.**{*;}
```

##### 5，android安装包不能安装在模拟器上？

答：因为Demo去掉了某些cpu架构，如果你想运行在模拟器上请按以下方式：

在android/build.gradle配置加入

```
ndk {
    abiFilters "arm64-v8a", "armeabi-v7a", "armeabi", "x86", "x86_64"
}
```

##### 6，ios构建release包报错

答：请将cpu架构设置为arm64，然后依次如下操作

- flutter clean
- flutter pub get
- cd ios
- pod install
- 连接真机后运行Archive

![ios cpu](https://user-images.githubusercontent.com/7018230/155913400-6231329a-aee9-4082-8d24-a25baad55261.png)

##### 7，ios运行的最低版本号？

答：13.0

#### 8， 有开发者遇到以下问题：
```
Could not build the precompiled application for the device.
Error (Xcode): Signing for "TOCropViewController-TOCropViewControllerBundle" requires a development team. Select a development team
in the Signing & Capabilities editor.

Error (Xcode): Signing for "DKImagePickerController-DKImagePickerController" requires a development team. Select a development team
in the Signing & Capabilities editor.

Error (Xcode): Signing for "DKPhotoGallery-DKPhotoGallery" requires a development team. Select a development team in the Signing &
Capabilities editor.
```
在Podfile添加以下代码：
```ruby
post_install do |installer|
    installer.pods_project.targets.each do |target|
      target.build_configurations.each do |config|
        config.build_settings['EXPANDED_CODE_SIGN_IDENTITY'] = ""
        config.build_settings['CODE_SIGNING_REQUIRED'] = "NO"
        config.build_settings['CODE_SIGNING_ALLOWED'] = "NO"      end
   end
end
```
