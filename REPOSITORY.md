# 源码仓库说明

## 2026-11-08 整套系统合并

本仓库现同时包含 GoSuxinWL 宿主、HRM 及 IM 全套源码。GoSuxinWL 位于 `services/gosuxinwl/`，HRM 位于其中 `plugins/biz-suite/`，统一账号和通讯桥接位于其中 `internal/imbridge/`，管理网页位于其中 `web/`。全部为普通 Git 文件，不要求克隆旧仓库。完整导航、工具链和初始化说明见 [整套系统源码](docs/system-source.md)。

当前客户端版本为 Windows 3.8.22、Web 3.8.20、Android 3.8.21；安卓风险复核、推送和 iOS 验收边界见该说明与 `docs/acceptance.md`。下文保留此前归档记录，历史版本不代表当前交付。

本仓库保存原 3.5 企业项目合并官方固定版本后的独立 OpenIM38 开发源码。当前 Windows 应用为 **3.8.7 / 380701**；上游组件与 SDK 版本见 `upstream-sources.lock.json`、`runtimes.lock.json` 及各模块依赖锁。

## 各端源码入口

这是多端源码共用一个仓库的目录结构。GitHub 首页的 [README 源码导航](README.md#源码导航)可直接进入各端；不是仅有服务端代码，也不是只提供指向其他仓库的链接。

| 平台或组件 | 仓库内目录 |
|---|---|
| Windows / Web 共用界面 | [clients/web-desktop/src](clients/web-desktop/src/) |
| Windows Electron 主进程、预加载和本地文件处理 | [clients/web-desktop/electron](clients/web-desktop/electron/) |
| Android / iOS 共用 Dart 业务 | [clients/flutter/lib](clients/flutter/lib/) |
| Android 原生宿主工程 | [clients/flutter/android](clients/flutter/android/) |
| iOS / Xcode 工程 | [clients/flutter/ios](clients/flutter/ios/) |
| Flutter 适配层与兼容插件 | [clients/flutter/local_plugin](clients/flutter/local_plugin/) |
| Flutter 固定 SDK | [sdks/flutter](sdks/flutter/) |
| WASM、Android Java 参考与 iOS Core | [sdks/wasm](sdks/wasm/)；[sdks/android-java](sdks/android-java/)；[sdks/ios-core](sdks/ios-core/) |
| IM、Chat、企业扩展与 LiveKit | [services](services/) |
| 管理后台 | [clients/admin](clients/admin/) |

iOS 工程和配套 Core 文件已纳入源码，当前没有 Xcode 构建或真机通过结论。Android114 媒体状态仍为 **NOT_ACCEPTED**。GitHub Releases 暂无安装包，下载页、源码树和本地候选包是不同交付位置；不能用 Releases 是否有附件判断源码是否存在。

## 源码范围

- `services/`：IM、Chat、独立企业扩展及 LiveKit 服务源码。
- `clients/`：Web/Windows、管理后台、Flutter 客户端与必要的平台兼容代码。
- `sdks/`：固定版本 SDK 与构建需要的上游运行文件。Flutter SDK 按普通源码目录纳入，不依赖未声明的 Git 子模块。
- `deploy/`、`tools/`、`patches/`：隔离环境生成、构建、运行和兼容补丁。
- `tests/`：可重跑测试代码和本次 Windows 修复的脱敏摘要。
- `docs/`、`README.md`：功能边界、配置迁移、构建和本地测试实例说明。

安装包、构建输出、下载的工具链、设备原始日志、截图视频、历史诊断快照和 `.private/` 不纳入 Git。已有本机文件仍保留在原目录，未从工作站删除。摘要报告中的历史日志、截图和安装包路径属于本机交付材料，克隆仓库后不会自动出现。

## 克隆与构建

```powershell
git -c core.longpaths=true clone -c core.longpaths=true -c core.autocrlf=false https://github.com/suxinwl/SuxinIM.git
cd SuxinIM
```

各端使用独立工具链，不替换系统默认运行时；按 `docs/web-desktop.md`、`docs/flutter.md` 和部署文档执行。Web/Windows 的 `package-lock.json` 已纳入版本控制。`scripts/ensure-screenshot-preload.mjs` 会在安装依赖和构建 Electron 时校验并应用固定截图组件兼容补丁。

Windows 建议克隆到较短路径。部分兼容插件路径较长，上面的配置同时启用长路径支持并保留仓库原始换行。可在原开发目录运行 [tools/verify_source_checkout.py](tools/verify_source_checkout.py)，传入 `--checkout` 指向另一个全新检出的目录，逐文件核对 Git 对象、文件内容和可执行标记，并执行 `git fsck`；验证报告默认保留在本地 `.cache/source-verification/`。

整体入口为 [tools/build-all.ps1](tools/build-all.ps1)，Linux 服务端入口为 [deploy/build-linux.sh](deploy/build-linux.sh)，Android 入口为 [clients/flutter/scripts/build-android.ps1](clients/flutter/scripts/build-android.ps1)。工具链来源见 [runtimes.lock.json](runtimes.lock.json) 和 [Flutter 引导脚本](clients/flutter/scripts/bootstrap-flutter.ps1)，详细步骤见 [docs/build-and-run.md](docs/build-and-run.md)。

依赖锁包括 [Web/Windows package-lock.json](clients/web-desktop/package-lock.json)、[后台 package-lock.json](clients/admin/package-lock.json)、[Flutter pubspec.lock](clients/flutter/pubspec.lock)、[Android ARM64 Gradle 锁](clients/flutter/android/app/gradle.lockfile)和[x64 验证锁](clients/flutter/android/app/gradle-x64-validation.lockfile)，各 Go 服务分别保留 go.mod/go.sum。固定 SDK、WASM 和 iOS Framework 已按普通目录纳入；Maven/Pub/npm 依赖下载缓存、工具链和开发签名私钥不属于可公开源码。新机器按锁文件恢复依赖，离线重建另需保留对应依赖载荷。

SDK 内的 WASM 和 iOS Framework 属于固定依赖，不是本项目安装包；源码包含其来源及版本信息。各组件许可证以对应目录的 LICENSE、NOTICE 等文件为准。

## 独立实例配置

仓库没有管理员密码、应用签名私钥、开发 CA 私钥、数据库密码或有效会话。部署时使用 `tools/generate_instance.py` 在 `.private/instance` 生成独立配置和随机秘密，按 `deploy/` 与部署文档准备业务程序、证书和持久化目录。不要把本机 `.private/` 或生成后的运行配置提交到 Git。

现有说明中的 `192.168.10.11:29880/29885` 是原工作站对应的内网测试入口，不是通用公共服务；新实例必须按实际主机、证书和网络配置设置入口。克隆或推送源码不会自动部署或改变服务器。

## 当前验收边界

Windows3.8.7本轮修复多人引用转发导致SDK退出、搜索结果显示与原消息定位、视频附件下载；94项适配测试和TypeScript检查通过，真实Electron引用转发5人后历史/会话可用，搜索和原生合成视频下载有定向运行证据。3.8.6的已读、工作圈和截图修复及57项旧证据继续保留，不改写为新版本通过。

媒体开发与测试已恢复。Android3.8.14+114的163项Dart、108项Java边界测试及62个Java文件的实际Android编译通过；正常main候选只存放在artifacts/android-candidates/，正式current包仍标记present:false且文件不存在，没有Android在线发布。单端快速会议普通Leave/重入及End后Ended禁入已通过限定验证；三人通话退出121023毫秒超时失败，后续17项物理任务最终完成且摄像头为空。媒体状态为NOT_ACCEPTED，定向诊断不等于第二轮正常验收。预约会议、纯语音和完整跨端矩阵尚未完成；113旧候选、历史暂停和失败记录继续保留。ARM64真机仍未提供，iOS未完成Xcode构建，Windows测试安装包未做代码签名，完整升级验收尚未结束。

运行证据按具体版本和对象分别记录于tests/delivery与媒体恢复目录。源码推送、构建成功或原生边界测试通过均不替代完整产品验收。
