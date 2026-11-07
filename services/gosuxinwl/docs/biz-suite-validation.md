# biz-suite 本地测试包验收记录

> 本文仅保留 0.1.0 的历史验收记录和历史文件哈希，不代表当前版本。含真实员工数据的本地验收记录单独保管，不随代码仓库提交；现行版本与构建说明见 [项目说明](../README.MD)。

日期：2026-10-28。分支：`feat/biz-suite`。平台：Windows/amd64。基础提交：`c88f0727e2082d67329ad3eec16277dbbf397361`。

## 产物

| 文件 | 字节数 | SHA-256 |
|---|---:|---|
| `dist/biz-suite.zip` | 16977157 | `57aa9582eb625d74ebfbdf3c6a4bb52361255bdc63f9140aa8043744542bfa74` |
| `dist/biz-suite-etl.exe` | 24231936 | `e15dea4219ac07d456049b8e3a5217861316277d53dda3299fd9b57f61b872a8` |

运行时包 `suxin-runtime-v1`，版本 0.1.0，共 20 个清单文件。包内只分发 worker、SPA 静态资产、许可证、文档和导入示例，不包括 node_modules 或前端测试依赖。包内附加文档位于 `admin/docs/`。

## 已通过的验证

| 验证 | 结果 |
|---|---|
| `go test ./...` | 通过，包含宿主与插件 |
| `go test ./plugins/biz-suite/... ./internal/pluginworker ./internal/runtimeplugin` | 最后集成验证通过；插件共 42 个顶层测试 |
| `go vet ./plugins/biz-suite/...` | 通过 |
| 插件前端 `npm ci --ignore-scripts` / `npm run build` | 通过；LiveKit SDK 本地打包 |
| 插件前端 `npm test` | 12/12 真实 Chrome 测试通过，使用本地模拟 API |
| 宿主前端 `test:layout` / `test:frontend` / `build` | 11/11、20/20 与构建通过；本次不修改宿主前端源码 |
| runtime-plugin-install `-inspect` | Windows/amd64、版本和 20 个文件哈希校验通过 |
| 临时宿主实际安装 | 安装器完成安装，真实 worker 健康检查通过 |
| worker HTTP 冒烟 | 内部凭据拒绝、匿名 401、权限种子、员工/班次/打卡/月统计、企业微信部门选项、IM 会话、SPA 深链接通过 |
| 初始化失败 | 故意破坏 schema 校验和后进程退出，未启动半初始化服务 |
| 核心数据隔离 | 安装和操作后 gf_admin、gf_auth_dept、gf_auth_role、gf_auth_role_access 内容不变；无 OA/Flowable 业务表 |

后端测试覆盖：MySQL 无默认值字段对应的严格权限表种子；禁用宿主账号拒绝；重复通讯录和打卡；100 人与时间窗口分批；API 失败重试及脱敏；审批分页、控件映射、撤销与游标；跨夜、多班次、休息区间和重叠请假；IM 多用户权限、并发幂等、回执、撤回、真实 WebSocket 推送与角色撤销；ETL 预演零写入、原始异常保留、历史时间和重复执行。

## 未执行与未完成

没有连接真实企业微信、旧 ruoyi 数据库、MySQL 生产实例或 LiveKit 媒体服务，没有执行生产切换。前端测试里的上传与 RTC 服务为模拟接口，不能替代真实存储和双设备音视频联调。

这是一份本地测试版验收记录，不是完整 1:1 迁移验收：原 Vue3 页面逐项迁移、上游完整 IM 管理接口、频道素材、完整 RTC 呼叫状态机仍有差异。参见 [实施记录](biz-suite.md)、[IM 差异](../plugins/biz-suite/docs/im-parity.md)、[前端说明](../plugins/biz-suite/admin/README.md)。
