# 依赖升级记录与后续安排

记录日期：2026-10-25。版本选择同时核对项目的实际引用、锁文件和上游发布说明；不能仅根据 `package.json` 中的版本范围判断安装版本。

## 升级顺序

1. 先统一 Go 1.27.3、Node.js 24.22.1、npm 11.21.1，再更新同一主版本的兼容修复及后端基础依赖。CI、`.nvmrc`、前端引擎约束和文档保持一致。
2. 再迁移 vue-i18n、markdown-it、ECharts/vue-echarts 和 Excelize。检查真实调用点，删除废弃配置和内部路径引用，更新测试后再构建。
3. 前两批验证完成后，按下面的顺序处理其余主版本。每一组以类型检查、现有行为测试及生产构建通过为进入下一组的条件；涉及浏览器交互的组件还需完成对应页面验收。

## 本轮验证

三批依赖升级及对应源码适配已完成，下面记录实际执行的本地检查。Node/npm 校验显式使用 `runtime/toolchains/node-v24.22.1-win-x64/` 和 `runtime/toolchains/npm-11.21.1/` 中的工具；Go 使用自动下载的 1.27.3 工具链。CI 和项目版本约束已同步。

| 范围 | 已落地版本或验证结果 |
| --- | --- |
| 工具链配置 | Go 1.27.3、Node.js 24.22.1、npm 11.21.1；两个 CI 作业使用同一版本。 |
| 后端基础依赖 | OpenTelemetry 1.47.0；`x/net` 0.61.0、`x/image` 0.47.0、`x/crypto` 0.58.0、`x/text` 0.43.0、`x/sys` 0.49.0。 |
| SQLite 驱动链 | glebarez/go-sqlite 1.23.0、modernc/sqlite 1.55.0、modernc/libc 1.74.1。 |
| Excelize | 已将实际导出调用从 v1 迁移到 `v2.11.1-0.20261003002531-6258dcebc4e2`，锁定上游安全修复提交。正式版 2.11.0 仍命中 6 项公告，故未停留在该版本。 |
| 核心后端测试 | 列出的 10 个包中 8 个通过、2 个没有测试文件。 |
| 独立模块构建 | 11 个模块均通过 `GOWORK=off` 的只读依赖构建；宿主主程序构建通过。 |
| SQLite HTTP 集成 | 在 Node.js 24.22.1 下通过安装、重启、登录、CRUD、角色授权和插件进程共享数据库检查。 |
| Go 漏洞检查 | `govulncheck` 为 0 个符号级、0 个包级发现；另有 1 个模块级 OpenPGP 停维护告警，当前无对应包导入。此结果不表示所有传递模块均受持续维护。 |
| 前端第一批版本 | Vue 3.5.44、Arco Design Vue 2.59.0、Lodash/lodash-es 4.18.1、qs 6.16.1、CodeMirror 5.65.21、ace-builds 1.45.2、Vite 8.3.5 等兼容升级；移除没有源码导入的 `query-string`。 |
| 前端第一批检查 | 使用 npm 11.21.1、Node.js 24.22.1，完整类型检查、11 个布局测试、12 个前端行为测试和 Vite 生产构建均通过。 |
| 前端第二批版本 | vue-i18n 11.4.16 切换到全局 Composition API；markdown-it 15.0.3 使用公开 Token API；ECharts 6.1.0/vue-echarts 8.3.2 保留 v5 主题并补齐图形组件注册。 |
| 前端第二批检查 | Node.js 24.22.1/npm 11.21.1 下，类型检查、11 个布局测试、15 个前端行为测试和 Vite 生产构建均通过。新回归覆盖全局语言切换及存储、富文本 Markdown 转换和 ECharts 渲染。 |
| 前端第三批版本 | Pinia 4.1.0、Vue Router 5.4.0、`@vue/devtools-api` 8.2.1、VueUse 15.0.0、TypeScript 6.0.3、`@types/node` 24.19.2、lint-staged 17.6.0。登录/权限守卫改用返回值，移除 TS 的 `baseUrl` 并使用显式相对路径映射。 |
| 前端最终检查 | npm 11.21.1/Node.js 24.22.1 下，完整 `npm run check` 通过：TypeScript/Vue 类型检查、11 个布局测试、18 个前端行为测试、Vite 8.3.5 生产构建。新增回归覆盖真实 Pinia 菜单与页签 store、动态路由、接口失败时保留路由、缓存集合反应性、权限拒绝和过期登录查询参数；没有 `next()` 弃用告警。 |
| 开发工具加载 | lint-staged 17.6.0 的 `--version` 正常执行，未运行暂存文件处理或格式化。 |
| npm 漏洞检查 | 全量锁文件审计 `npm audit --package-lock-only` 为 0 个漏洞。 |
| 锁文件重装 | Node.js 24.22.1/npm 11.21.1 下，最终 `npm ci` 成功安装 519 个包。 |
| 第二批浏览器验收 | 真实登录、刷新保留会话、简体中文→英文→繁体中文→简体中文、系统菜单展开及部门表格加载通过，控制台无警告或错误。 |
| 最终构建浏览器验收 | 登录保持、部门路由直接刷新与表格加载、亮/暗主题切换通过；Pinia/Router 守卫适配后的构建运行正常，控制台无警告或错误。 |

Excelize 当前固定的是正式发布前的伪版本；后续出现包含同一安全修复的正式版本时，应重新扫描、回归导出并切换到正式版本。

验收命令：

```shell
cd web
npm ci
npm run check
npm audit
cd ..
go test ./internal/builtinai ./internal/devtools/ai ./internal/devtools/access ./internal/logic/admin_system ./internal/runtimeplugin ./utility/auth ./internal/logic/admin_install ./utility/tools/dbtool ./utility/dbcompat ./utility/extend/excelexport
go build -mod=readonly -trimpath -o bin/gosuxin .
node scripts/test-database-install.cjs
python scripts/sync-frontend.py
python scripts/repack_webcode.py
```

Windows 构建输出改为 `bin/gosuxin.exe`。数据库测试应使用独立测试数据和端口；没有实际启动 MySQL 5.7/8.0 时，不将本地 SQLite 结果视为 MySQL 验证通过。后端多模块依赖还需检查根模块、框架、CLI、驱动与独立插件 SDK 的一致性。

## 第三批实施与剩余主版本安排

下表前四项已在本轮实施，并通过最终检查；图片预览器、编辑器和其余开发工具按后续顺序单独推进。

| 顺序 | 组件与起始版本 | 目标及状态 | 项目使用、已完成适配或后续验收条件 |
| --- | --- | --- | --- |
| 1 | Pinia 2.1.7 | 4.1.0，已完成 | 仅使用 `createPinia()` 和三个 `defineStore('id', options)`，没有旧式 `defineStore({ id })` 或自定义插件。已补齐必需的 `@vue/devtools-api` v8；真实 store 回归覆盖菜单刷新、页签缓存反应性和设置恢复。 |
| 1 | Vue Router 4.3.2 | 5.4.0，已完成 | 当前为手工路由及服务端菜单动态注册，未使用 `unplugin-vue-router`。已将登录与权限守卫改为返回值；回归覆盖自重定向、动态路由替换、插件 iframe 查询参数和过期登录重定向。 |
| 2 | VueUse 10.11.1 | 15.0.0，已完成 | 当前使用存储、复制、主题、全屏、窗口/断点、事件及卸载 hooks；未使用被移除的 `templateRef` 或废弃定时器选项，现有调用通过类型检查和构建。浏览器交互应继续围绕主题、登录记忆、复制、全屏、窗口缩放和弹窗卸载验收。 |
| 3 | TypeScript 5.9.3 | 6.0.3，已完成 | 保留 Bundler 解析、显式 `types` 和 ES2021，已移除 `baseUrl`，将 `paths` 改为显式相对路径；`vue-tsc` 与配置文件类型检查均通过。TS 7 另待 Vue 类型工具链兼容验证。 |
| 4 | PhotoSwipe 4.1.3 | 5.x，后续迭代 | 图片预览器依赖 v4 的 `PhotoSwipeUI`、手工 DOM 模板、`listen()`、`currItem`、`items` 和 `getZoomLevel()`。需要重写预览适配层及自定义工具栏，单独验收上一张/下一张、缩放、键盘关闭、移动端手势、图片尺寸及销毁后再次打开。 |
| 5 | CodeMirror 5.65.21 | 6.x 模块体系，后续迭代 | 本轮已更新 v5 兼容补丁。普通代码编辑器和富文本代码块都依赖 v5；富文本还自定义 `runMode` 并使用 `StringStream`、`getMode`、零起始行号和旧事件系统。先为普通编辑器实现 v6 适配，再重写富文本代码块的选区、撤销重做、语言高亮、主题及焦点行为；两处均验收后才能删除 v5。 |
| 6 | Prettier 2、commitlint 17 等开发工具 | 当时受支持的版本，后续迭代 | lint-staged 已单独升级至 17.6.0 并验证 CLI。其余工具先核对 ESM、Node 最低版本和配置变化，检查暂存文件规则，避免升级格式化工具时顺带重排整个仓库。 |

CodeMirror 和 PhotoSwipe 涉及富文本编辑器的运行行为，仅更新包版本会破坏现有 API，因此安排为各自独立的适配任务。每项均以类型检查、行为测试、生产构建和对应浏览器页面验收通过为进入下一项的条件。

`query-string` 在前端源码中没有导入，本轮已移除。`filesize` 当前在上传器中以默认导出函数调用，迁移新主版本时需同步命名导出及文件大小显示测试。

CryptoJS 4.2.0 已停止维护，当前调用承担与后端兼容的加密协议。后续迁移到 Web Crypto 时，应同时调整前后端协议并处理旧客户端兼容，单独替换前端包无法完成迁移；本轮保留现有协议。

## 运行环境的后续工作

- Docker 模板仍使用 `loads/alpine:3.8`。另开部署迭代，选用受支持的基础镜像，验证 CA 证书、时区、非 root 权限、静态资源和 SQLite 运行条件后发布。
- MySQL 测试矩阵当前为 5.7 和 8.0。下一轮增加 8.4 LTS 的安装、种子 SQL、排序/唯一键、备份恢复和插件共享数据库验证，再制定部署迁移方案。本轮不修改现有数据库，也不以更新依赖替代数据库迁移。

## 上游依据

- [Pinia 2 到 3 迁移说明](https://pinia.vuejs.org/cookbook/migration-v2-v3.html)：检查已删除的旧 API。
- [Pinia 4 发布说明](https://github.com/vuejs/pinia/releases/tag/v4.0.0)：改为 ESM，并要求单独安装 `@vue/devtools-api`。
- [Vue Router 5 迁移说明](https://router.vuejs.org/guide/migration/v4-to-v5)：未使用文件路由插件的 v4 项目维持现有 API。
- [VueUse 14 发布说明](https://github.com/vueuse/vueuse/releases/tag/v14.0.0)、[VueUse 15 发布说明](https://github.com/vueuse/vueuse/releases/tag/v15.0.0)：Vue/Node 前提、导出清理及行为变化。
- [TypeScript 6 发布说明](https://devblogs.microsoft.com/typescript/announcing-typescript-6-0/)：配置弃用和向 TS 7 过渡的条件。
- [CodeMirror 5 到 6 迁移指南](https://codemirror.net/docs/migration/)：编辑器状态、位置、事件及 DOM 改为新的模型。
- [PhotoSwipe 初始化](https://photoswipe.com/getting-started/)、[PhotoSwipe 方法](https://photoswipe.com/methods/)：新 Lightbox/Core 初始化与生命周期接口。
