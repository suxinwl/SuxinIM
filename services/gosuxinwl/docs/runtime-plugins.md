# GoSuxin 运行插件

GoSuxin 使用 `suxin-runtime-v1` 协议安装预编译的独立业务插件。宿主负责包校验、进程启动、请求代理、登录权限和菜单；插件负责自己的业务接口与页面。当前工程包含共享 SDK 和安装框架。

## 安装、更新与卸载

1. 在后台“插件市场”上传运行 ZIP，操作系统和 CPU 架构必须与宿主一致。
2. 宿主校验包结构、路径、平台和文件 SHA-256，启动插件并等待健康检查通过。
3. 上传同名插件的新版本执行更新；启动失败时恢复此前的运行版本。
4. 卸载停止插件进程、关闭接口并隐藏菜单，保留包文件、业务数据、配置及角色授权；重新安装恢复使用。

运行包包含已经编译的程序和页面，服务器无需 Go、npm 或源码构建环境。源码 ZIP 不作为运行安装包。包中的哈希用于完整性校验，上传的可执行程序应来自可信来源。

宿主服务在插件安装和更新期间持续运行；更新会替换插件进程，该插件正在处理的请求或后台任务可能中断。插件业务应在自己的持久化存储中处理任务恢复。

也可从项目根目录调用现有安装脚本：

```powershell
powershell -ExecutionPolicy Bypass -File scripts/install-runtime-plugin.ps1 -Archive ./plugin-runtime.zip -BaseUrl http://127.0.0.1:8603 -Authorization "<当前管理员会话的 Authorization>" -ApiVerify "<当前请求的 apiverify>"
```

显式传入目标宿主地址，认证头使用当前有效会话生成的值。脚本先上传包，再提交安装；不保存凭据。

离线安装需要先停止宿主，在项目根目录使用相同的校验流程：

```shell
go run ./scripts/runtime-plugin-install -inspect ./plugin-runtime.zip
go run ./scripts/runtime-plugin-install ./plugin-runtime.zip
```

`-inspect` 只检查包，不安装。安装命令会检查安装锁和宿主监听端口，完成后重新启动宿主；正常在线更新优先使用后台上传。

## 访问与权限

| 地址 | 用途 |
| --- | --- |
| `/suxinweb/runtime-plugins/<name>` | 宿主管理后台中的插件容器 |
| `/plugins/<name>/admin/` | 插件管理页面 |
| `/plugins/<name>/api/admin/` | 插件管理 API 前缀 |
| `/plugins/<name>/...` | 插件自定义业务路由 |

宿主校验管理页面和管理 API 的登录与角色权限，并通过本机通道传递可信身份。插件管理页面使用短期 HttpOnly Cookie 续接宿主登录，宿主退出后原会话不再有效。自定义业务路由需要插件实现相应的鉴权。

使用 Cookie 的写入请求应携带 `X-Requested-With: XMLHttpRequest`，并保持与宿主同源；服务端会验证 Origin、Referer。代理不会将浏览器的 Authorization 或 Cookie 原样交给插件，管理身份通过可信的内部通道传递。公共业务若需要独立登录，使用插件专属的鉴权协议。

内部 HTTP 转发保留经宿主认证的 HTTPS 状态及客户端地址，插件可据此生成正确的链接、Cookie 和审计记录。插件不能自行信任来自公网的内部身份头。

安装自动创建插件菜单和“访问管理端”权限节点，普通角色不会自动获得授权。管理员需在角色管理中授予插件访问权限；插件内部的细分业务权限由插件实现。

## 包结构

Windows 运行包结构如下；其他平台的可执行程序不带 `.exe` 后缀：

```text
plugin.json
bin/<name>.exe
admin/index.html
admin/assets/...
public/...
README.md
LICENSE.txt
```

`bin/<name>` 为必须项；页面、公共资源和说明文件按插件需要提供。`plugin.json` 清单示例：

```json
{
  "format": "suxin-runtime-v1",
  "name": "demo",
  "title": "示例插件",
  "version": "1.0.0",
  "protocol": 1,
  "os": "windows",
  "arch": "amd64",
  "files": {
    "bin/demo.exe": "<文件内容的 SHA-256>",
    "admin/index.html": "<文件内容的 SHA-256>"
  }
}
```

插件标识以小写字母开头，只使用小写字母、数字和连字符，长度不超过 64 个字符。`files` 包含除清单本身之外的每个文件及其 SHA-256，路径与大小写必须对应实际 ZIP 条目。

允许的包文件路径为 `bin/`、`admin/`、`public/` 下的文件，以及根目录的 `README.md`、`LICENSE.txt`。ZIP 上限为 160 MB，解压总量上限为 320 MB，文件条目数量上限为 3000；禁止路径越界、重复路径和符号链接。实现入口为 `internal/runtimeplugin/package.go`。

## 开发入口

在当前 Go 工作区中开发插件，复用 `internal/pluginworker`。独立入口可放在 `plugins/<name>/cmd/main.go`，通过 `pluginworker.Run(name, bind)` 注册业务路由；`bind` 的签名为 `func(context.Context, *ghttp.RouterGroup)`。业务接口也可复用 `internal/addons/support` 的共享能力。

宿主代理会移除 `/plugins/<name>` 前缀。插件内部以 `/api/admin/` 注册管理接口，并验证宿主传入的可信身份；已有 SDK 中间件可用于登录和权限校验。前端的资源路径和 History 路由应使用 `/plugins/<name>/admin/`，嵌入宿主时只显示业务内容。

`pluginworker.Run` 提供管理页面服务和 `GET /__plugin_health` 健康检查。公共资源与业务路由由插件自行绑定。独立程序由宿主管理器启动，SDK 会校验启动环境；无需将业务代码编译进宿主。

宿主传入的环境变量如下：

| 变量 | 用途 |
| --- | --- |
| `SUXIN_PLUGIN_NAME` | 插件标识 |
| `SUXIN_PLUGIN_ADDR` | 本机 HTTP 监听地址 |
| `SUXIN_PLUGIN_SECRET` | 进程间认证密钥 |
| `SUXIN_PLUGIN_PACKAGE` | 当前运行版本的包目录 |
| `SUXIN_PLUGIN_DATA` | 可持久写入的插件数据目录 |
| `SUXIN_PLUGIN_BASE` | 外部访问前缀 `/plugins/<name>` |

开发机构建独立程序和插件页面后，按上述结构生成 ZIP 与逐文件清单。当前工程提供协议 SDK，打包步骤由具体插件的开发脚本实现。

## 状态与持久化

| 路径 | 内容 |
| --- | --- |
| `storage/plugins/<name>/packages/<digest>/` | 已校验的版本包；目录摘要由清单内容生成 |
| `storage/plugins/<name>/active.json` | 当前版本和卸载状态 |
| `storage/plugins/<name>/data/` | 插件持久化数据 |
| `runtime/plugins/<name>.log` | 插件进程日志 |

宿主以项目根目录作为插件工作目录，因此插件可读取当前项目的配置。持久化文件写入 `SUXIN_PLUGIN_DATA` 或插件明确管理的业务目录，不写入随更新替换的版本包。卸载保留数据，具体数据库迁移和后台任务退出由插件实现。

## 内置 AI/MCP

AI/MCP 是宿主功能，独立于业务插件的安装状态。管理入口和使用方式见 [项目说明](../README.MD)。依赖业务插件的 MCP 工具通过宿主代理调用，相关插件未安装或已卸载时返回明确错误。
