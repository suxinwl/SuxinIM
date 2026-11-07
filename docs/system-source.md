# 整套系统源码（GoSuxinWL、HRM、IM）

2026-11-08 起，整套业务源码统一保存在 [suxinwl/SuxinIM](https://github.com/suxinwl/SuxinIM)。GoSuxinWL 和 HRM 为本仓库普通目录，不依赖另一个仓库或 Git 子模块。本次不会推送到旧 Gosuxinwl 仓库。

| 系统 | 源码入口 | 内容 |
| --- | --- | --- |
| GoSuxinWL 宿主 | [services/gosuxinwl](../services/gosuxinwl/) | Go 服务、框架、安装器、权限、统一账号与部门管理 |
| GoSuxinWL 网页后台 | [web](../services/gosuxinwl/web/) | Vue 管理页面、通讯管理、账号安全、部门负责人及部门群开关 |
| HRM | [plugins/biz-suite](../services/gosuxinwl/plugins/biz-suite/) | 人事考勤、企业微信同步、业务插件服务和管理页面 |
| 统一账号与 IM 桥接 | [internal/imbridge](../services/gosuxinwl/internal/imbridge/) | SSO、员工与部门同步、权限、资源访问和管理接口 |
| IM 网页和 Windows | [clients/web-desktop](../clients/web-desktop/) | React 页面、Electron、本地存储、附件和更新 |
| IM Android / iOS | [clients/flutter](../clients/flutter/) | Flutter 业务、Android 和 iOS 平台工程及本地插件 |
| IM 服务端 | [services](../services/) | OpenIM、Chat、企业扩展与 LiveKit |
| 固定 SDK | [sdks](../sdks/) | Flutter、WASM、Android Java、iOS Core 源码或固定构建依赖 |

## 获取源码

```powershell
git -c core.longpaths=true clone -c core.longpaths=true -c core.autocrlf=false https://github.com/suxinwl/SuxinIM.git
cd SuxinIM
```

先阅读 [GoSuxinWL / HRM 说明](../services/gosuxinwl/README.MD)和 [IM 构建说明](build-and-run.md)。两套 Go、Node 工具链分别遵守各自锁文件，不要求用单一版本构建所有模块。GoSuxinWL 的 Go workspace 保留在其自身目录，运行命令前进入 `services/gosuxinwl`。

GoSuxinWL 后台生产静态目录 `resource/suxinweb` 在本次整合中不复制；在其 `web` 目录安装锁定依赖并构建，再执行 `python scripts/sync-frontend.py` 生成部署页面。HRM 插件源码、原有导入模板和安装种子保留；构建插件参见其 [README](../services/gosuxinwl/plugins/biz-suite/README.md)及 `scripts/build-biz-suite*.ps1`。

GoSuxinWL 的 `.github`、构建脚本及冻结兼容资料随源目录保留；嵌套 `.github/workflows` 不会自动成为整套仓库的根工作流。源码中的历史独立仓库地址属于来源记录，获取当前整套系统请使用上面的统一地址。

## 当前交付边界

当前客户端清单为 Web 3.8.20 / 382001、Windows 3.8.22 / 382201、Android 3.8.21 / 121。安卓在小米安装器存在 `a.gray.RacketeerIM` 风险提示，尚待安全复核；不得将源码上传解释为该问题已消除。后台厂商推送候选尚未迁入正式代码；iOS 有源码，仍待 Mac/Xcode、签名及真机验收。详情见 [验收记录](acceptance.md)。

本次汇集当前工作区的实现、测试、依赖锁和构建说明，不重新声称全平台构建或整套业务验收已通过。安装包、运行数据库、员工考勤与通讯记录、实际配置、企业微信凭据、SSH / 管理员密码、签名私钥和本地诊断资料均不提交。新实例应按模板生成自己的配置，不能从源码恢复现网员工数据或生产密钥。

原 GoSuxinWL 工作区保留；本仓库导入的是其源码快照，后续统一提交到 SuxinIM 时应在这里同步维护。上传源码不会部署服务或改变生产数据库。
