# 源码仓库说明

本仓库保存原 3.5 企业项目合并官方固定版本后的独立 OpenIM38 开发源码。当前 Windows 应用为 **3.8.7 / 380701**；上游组件与 SDK 版本见 `upstream-sources.lock.json`、`runtimes.lock.json` 及各模块依赖锁。

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
git clone https://github.com/suxinwl/SuxinIM.git
cd SuxinIM
```

各端使用独立工具链，不替换系统默认运行时；按 `docs/web-desktop.md`、`docs/flutter.md` 和部署文档执行。Web/Windows 的 `package-lock.json` 已纳入版本控制。`scripts/ensure-screenshot-preload.mjs` 会在安装依赖和构建 Electron 时校验并应用固定截图组件兼容补丁。

SDK 内的 WASM 和 iOS Framework 属于固定依赖，不是本项目安装包；源码包含其来源及版本信息。各组件许可证以对应目录的 LICENSE、NOTICE 等文件为准。

## 独立实例配置

仓库没有管理员密码、应用签名私钥、开发 CA 私钥、数据库密码或有效会话。部署时使用 `tools/generate_instance.py` 在 `.private/instance` 生成独立配置和随机秘密，按 `deploy/` 与部署文档准备业务程序、证书和持久化目录。不要把本机 `.private/` 或生成后的运行配置提交到 Git。

现有说明中的 `192.168.10.11:29880/29885` 是原工作站对应的内网测试入口，不是通用公共服务；新实例必须按实际主机、证书和网络配置设置入口。克隆或推送源码不会自动部署或改变服务器。

## 当前验收边界

Windows3.8.7本轮修复多人引用转发导致SDK退出、搜索结果显示与原消息定位、视频附件下载；94项适配测试和TypeScript检查通过，真实Electron引用转发5人后历史/会话可用，搜索和原生合成视频下载有定向运行证据。3.8.6的已读、工作圈和截图修复及57项旧证据继续保留，不改写为新版本通过。

媒体开发与测试已恢复。Android3.8.14+114正在构建和媒体联调，159项Dart、108项Java源码边界结果不等于APK、真实JNI或物理设备验收通过；113仅保留旧本地非媒体候选。历史暂停和失败记录继续保留。ARM64真机仍未提供，iOS未完成Xcode构建，Windows测试安装包未做代码签名，完整升级验收尚未结束。

运行证据按具体版本和对象分别记录于tests/delivery与媒体恢复目录。源码推送、构建成功或原生边界测试通过均不替代完整产品验收。
