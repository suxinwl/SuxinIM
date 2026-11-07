# 智慧 HRM 考勤管理（Xz_kaoqin_tools）

> 2026-11-08：本目录已合并到 [SuxinIM 整套系统源码](../../docs/system-source.md)，包含 GoSuxinWL 宿主、HRM 插件、企业微信同步及 IM 桥接。当前统一获取地址为 `https://github.com/suxinwl/SuxinIM.git`；克隆后进入 `services/gosuxinwl` 执行以下宿主构建步骤。下文独立仓库地址、历史验收和兼容版本仅作来源对照，不能代替整套仓库的当前交付记录。生产网页目录需要从 `web` 构建后执行 `scripts/sync-frontend.py` 生成。

本项目基于 GoSuxin，包含智慧 HRM、本地考勤核算、企业微信同步和企业即时通讯接入。主业务源码位于 `plugins/biz-suite`；宿主、权限、组织账号及通讯桥接改动也在本仓库内。

- `plugins/biz-suite/internal/`：员工映射、考勤规则与核算、审批、同步、迁移和 IM 业务。
- `plugins/biz-suite/admin/`：独立业务前端、依赖锁文件及浏览器测试。
- `internal/imbridge/`、`web/`：宿主通讯桥接及管理界面源码。
- `build-support/r6-imbridge-compat/`：现行线上 R6 的冻结兼容源码、校验及可移植构建映射；保留主工作区更新的开发代码。
- `scripts/`、`docs/`：构建、安装、迁移工具及技术说明。

2026-11-02 核对的线上插件版本为 `0.3.2-openim-r6.2-rule-cleanup.1`。重建现行版本请先阅读 [R6 兼容构建说明](build-support/r6-imbridge-compat/README.md)，并显式指定构建版本；旧脚本的默认版本号不代表最新源码版本。

同日主源码验证：插件前端 93 项、宿主前端 26 项测试通过，`plugins/biz-suite`、`internal/pluginworker`、`internal/runtimeplugin` 的 Go 测试通过。较新的 `internal/imbridge` 开发源码仍有两项测试失败：`TestProvisioningPreservesPasswordsAndHonorsPending`、`TestDefaultDisabledConfiguration`；此次源码归档保留现状，不将开发版本视为已经通过完整发布验收。

克隆本项目：

```shell
git clone ssh://huqw@192.168.10.8:22/volume1/git/SoftProject/IT/Xz_kaoqin_tools.git
cd Xz_kaoqin_tools
```

仓库包含完整项目代码及配置模板。实际数据库、员工考勤工作簿、企业微信凭据、证书私钥、安装状态、运行数据及依赖缓存单独保管。初始化使用 `python scripts/init-local-config.py`，再填写本机配置；不要把现有数据库当作首次安装库重新初始化。

## GoSuxin 基础工程说明

GoSuxin 是由 Suxin 技术团队维护的 Go 开发框架与管理后台，包含本地基础框架、Suxin CLI、数据库驱动、Vue 3 管理界面、内置 AI/MCP 和独立运行插件机制。

## 当前功能

- Go 后端与本地 `framework` 模块，支持同工作区开发框架、CLI 和数据库驱动。
- Vue 3 + TypeScript + Vite 管理后台，采用 Arco Design，支持多种导航布局及页签样式。
- 登录、角色权限、用户组织、菜单、系统配置、字典、附件和操作日志。
- 登录令牌与接口校验，操作日志中的凭据脱敏。
- 内置 AI 模型配置、模型调用、开发令牌管理、接口目录和 MCP 接入。
- 预编译插件 ZIP 的安装、更新、卸载、重新安装，以及插件进程、菜单和访问权限管理。

当前工程提供主程序和通用插件 SDK；业务功能通过独立运行插件扩展。协议和开发入口见 [运行插件指南](docs/runtime-plugins.md)。

仓库：[suxinwl/Gosuxinwl](https://github.com/suxinwl/Gosuxinwl)。默认安装仅包含通用管理后台，不预装业务插件，也不携带生产数据库、账号会话、私有配置或客户端安装包。需要什么业务功能，再从后台上传相应插件。

## 初始化品牌与账号

- 页面品牌为 **GoSuxin**，维护团队为 **Suxin技术团队**；登录页、后台标题、页脚及默认头像均使用 Suxin 品牌资源。
- 管理员登录名和密码由安装者设置；初始姓名、昵称为 **Suxin管理员**，默认部门为 **Suxin技术团队**。
- 管理员手机号、邮箱、电话均为空；邮件发送账号和授权码不预填，在个人中心及系统配置中按实际需要设置。
- 首页提供“系统配置”“插件市场”“AI / MCP”快捷入口。业务菜单在插件安装并授权后出现。
- MySQL 与 SQLite 使用相同的初始资料。安装器拒绝覆盖已有数据库；升级不会批量覆盖自定义头像、联系信息或角色授权。

## 环境要求

| 环境 | 版本或用途 |
| --- | --- |
| Go | 1.27.3，开发、CI 与发布使用同一补丁版本 |
| 数据库 | 安装时选择 MySQL 8.x、MySQL 5.7 或 SQLite；SQLite 无需另装数据库服务 |
| Node.js | 24.22.1，版本记录在 `web/.nvmrc` |
| npm | 11.21.1，使用已提交的锁文件安装依赖 |
| Python | 3，初始化本地配置及同步前端资源 |
| Redis | 按实际使用的缓存功能配置，连接模板位于后端配置中 |

应用、框架和 CLI 的模块版本为 `v1.0.0`。根目录 `go.work` 用于本地多模块开发。

## 安装与启动

以下命令均从项目根目录执行。首次使用先初始化配置：

```shell
git clone https://github.com/suxinwl/Gosuxinwl.git
cd Gosuxinwl
python scripts/init-local-config.py
```

脚本创建缺失的配置文件并生成鉴权密钥，已有文件保持原值。填写 `manifest/config/config.yaml` 的数据库连接；应用设置和上传设置分别位于 `manifest/config/app.yaml`、`manifest/config/upload.yaml`。

配置模板默认选择 MySQL 8，数据库名为 `gosuxin_v1_0`，固定表前缀为 `gf_`，HTTP 端口为 `8603`。安装页面可改选 MySQL 5.7 或 SQLite。MySQL 会核对服务器版本；SQLite 默认文件为 `storage/database/gosuxin.db`，安装器保存其绝对路径，并配置 WAL、5 秒锁等待和单连接池。数据库密码和令牌不写入文档。

```shell
go run .
```

首次安装在模板默认的 `debug` 模式下访问 `http://127.0.0.1:8603/install/`，按页面流程设置数据库和管理员账号，完成后重新启动服务。安装器适用于新数据库；已有部署保留配置、数据库及安装锁。

三种选项使用同一套程序，每个部署选择一种数据库。安装器会拒绝已有业务表的数据库，避免覆盖数据。已安装系统不能通过修改类型直接迁移数据。SQLite 适合单机部署，文件需放在本地持久磁盘；多个插件进程可以共享该数据库，但写入仍需串行，持续高并发写入应使用 MySQL。SQLite 不提供 MySQL 的字段注释，自动导出表头回退为字段名，业务页面可显式提供中文列标题。SQLite `NOCASE` 与 MySQL Unicode 排序规则不完全相同，业务唯一键应使用规范化编码。

数据库初始化 SQL 分别为 `devsource/developer/install/gosuxin.sql`（MySQL 8/5.7）与 `sqlite.sql`（SQLite）。升级 SQL 按数据库选择 `manifest/sql/v1.0.0.sql` 或 `v1.0.0.sqlite.sql`。修改 MySQL 种子后执行 `python scripts/sync-sqlite-schema.py`，审查并提交两套 SQL；该脚本只处理本仓库种子，不是通用数据库迁移器。SQL 导出文件也只供相同数据库类型使用。

运行插件宿主和共享 SDK 都已加载两种驱动；第三方插件自己的建表 SQL、查询和升级逻辑仍需支持所选数据库，已有插件应重新构建并验证。

后台地址为 `http://127.0.0.1:8603/suxinweb/`。管理员账号由安装流程设置，没有通用默认密码。

Windows 本地后台启动也可使用以下方式，需先构建 `bin/gosuxin.exe`：

```powershell
powershell -ExecutionPolicy Bypass -File scripts/start-local.ps1
```

该脚本使用端口 `8603`，检测已有实例并补齐上传目录。PID 保存在 `runtime/core-host.pid`，标准输出和错误日志分别位于 `runtime/core-running-stdout.log`、`runtime/core-running-stderr.log`；应用日志按后端配置写入 `runtime/log/`。

## 开发与构建

Windows 后端构建：

```shell
go build -mod=readonly -trimpath -o bin/gosuxin.exe .
```

Linux 后端构建：

```shell
go build -mod=readonly -trimpath -o bin/gosuxin .
```

构建到现有程序路径前，应先退出使用该文件的服务。运行已构建程序时使用项目根目录作为工作目录，以便读取配置和资源。

前端检查与生产构建：

```shell
cd web
npm install --global npm@11.21.1
npm ci
npm run check
cd ..
python scripts/sync-frontend.py
python scripts/repack_webcode.py
```

`npm run check` 执行类型检查、布局与前端行为测试及生产构建。构建输出为 `web/dist/`，同步后部署在 `resource/suxinweb/`；源码安装器的前端包为 `devsource/developer/install/webcode.zip`。前端开发服务、接口地址和部署方式见 [前端说明](web/README.md)。

依赖分批升级的变更、验证范围及后续主版本迁移顺序见 [依赖升级记录](docs/dependency-upgrade.md)。

主程序和共享插件框架的检查命令与 CI 一致：

```shell
go test ./internal/builtinai ./internal/devtools/ai ./internal/devtools/access ./internal/logic/admin_system ./internal/runtimeplugin ./internal/plugins ./internal/pluginworker ./utility/auth ./internal/logic/admin_install ./utility/tools/dbtool ./utility/dbcompat ./utility/extend/excelexport
```

构建 `bin/gosuxin`（Windows 为 `.exe`）后，可运行 `node scripts/test-database-install.cjs`，在 `runtime/db-compat/` 的独立目录验证 SQLite 安装、重启、登录、部门操作、角色授权与插件进程共享数据库。测试结束会关闭测试进程并保留诊断文件。CI 另以 MySQL 5.7、8.0 服务运行相同流程。手动验证 MySQL 时设置 `SUXIN_SMOKE_DATABASE=mysql57` 或 `mysql8`，以及 `SUXIN_TEST_MYSQL_HOST/PORT/USER/PASSWORD`，请使用专门的测试实例；脚本会创建独立的 `suxin_smoke_*` 数据库。

## AI 与插件

AI/MCP 通过后台“开发者工具”进入，模型地址、模型名称、密钥和开发令牌在该页面配置。管理接口位于 `/admin/developer/ai/`，接口目录为 `GET /admin/developer/ai/interfaces`，MCP 入口为 `/mcp`。管理功能仅对启用的超级管理员开放，MCP 使用单独创建的 Bearer 开发令牌。

安装业务功能的步骤：

1. 打开首页“插件市场”，或“开发者工具 → 代码仓库”。
2. 点击“安装插件包”，上传与服务器系统和架构匹配的运行插件 ZIP。
3. 等待清单、文件校验和进程健康检查完成，再从新增菜单进入。
4. 在角色管理中为需要使用该插件的普通管理员分配访问权限；安装本身不会给所有角色开放权限。
5. 后续更新上传同名插件的新版本。卸载保留插件数据，再次安装可以恢复使用。

运行插件通过“插件市场”上传与服务器操作系统和 CPU 架构匹配的 `suxin-runtime-v1` ZIP。后续安装和更新使用预编译程序，服务器无需 Go 或 npm；卸载关闭插件接口和进程，并保留插件数据与角色授权。依赖插件的 MCP 工具需要对应插件处于已安装状态。

插件管理页面位于 `/plugins/<name>/admin/`，在宿主后台通过 `/suxinweb/runtime-plugins/<name>` 进入；普通角色需由管理员授予插件访问权限。

近期通用修复包括外部设备时钟偏差引发的登录超时、登录后的完整路径回跳、插件管理请求来源校验，以及插件内部转发的 HTTPS 状态和客户端地址。宿主退出时会关闭所属插件进程；无需将业务源码合入主程序。

## 目录结构

| 路径 | 内容 |
| --- | --- |
| `api/` | 接口请求与响应定义 |
| `internal/` | 控制器、业务逻辑、数据访问、权限及主程序服务 |
| `framework/` | 本地框架、CLI 与驱动模块 |
| `web/` | 管理后台源码与前端测试 |
| `manifest/` | 配置模板、SQL 和部署资源 |
| `devsource/developer/install/` | 安装页面、初始化 SQL 和前端源码包 |
| `resource/` | 已部署的后台静态文件、公共资源及上传目录 |
| `scripts/` | 配置初始化、资源同步、安装及维护脚本 |
| `storage/` | 系统私有配置与插件持久化数据 |
| `runtime/` | 日志、PID 和本地临时文件 |
| `docs/` | 运行插件使用与开发说明 |

`scripts/clean-obsolete.ps1` 默认只预览已确认的一次性历史文件；添加 `-Apply` 才执行清理。清理范围采用明确的文件白名单，正在占用的文件会跳过。

## 许可证

项目采用 [MIT 许可证](LICENSE)。框架及各子模块的原有许可证和源码声明保留在对应目录中。
