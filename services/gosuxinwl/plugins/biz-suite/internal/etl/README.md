# ruoyi 数据导入

此工具读取规范化 JSON 导出，默认只读预演。它不会连接或写入旧 ruoyi 数据库，也不会创建宿主用户、部门或 OA 业务。

先在测试环境安装 biz-suite，完成数据库初始化。从 GoSuxin 根目录执行：

```powershell
go run ./plugins/biz-suite/cmd/etl -config ./manifest/config/config.yaml -input ./plugins/biz-suite/examples/migration.json
go run ./plugins/biz-suite/cmd/etl -config ./manifest/config/config.yaml -input ./plugins/biz-suite/examples/migration.json -apply
```

`-config` 指向实际宿主配置文件；省略时使用 GoSuxin 原有配置发现机制。不要在命令行填写数据库密码。数据库或插件表未初始化时退出；预演不会建表、生成日志行或临时业务记录。CLI 输出 JSON 报告；任意失败行使进程返回 1。

## 输入格式

顶层 `version: 1`，支持 `employees`、`clocks`、`leaves`、`outs`、`im_users`、`messages` 数组。不存在的业务数组可以省略。字段和示例参见同目录 `types.go` 及插件 `examples/migration.json`。未知字段和重复 `source_id` 会在导入前拒绝。

每条 `source_id` 是旧系统稳定主键，最多 120 字节，同一实体内唯一。导出 SQL/脚本应把不同旧表映射到以下字段；这是转换接口，不假设旧数据库列名永远不变：

| 数据 | 必要字段 | 规则 |
|---|---|---|
| 员工 | source_id、name | 企业微信 userid → 工号 → 唯一手机号匹配本地员工；未匹配可以创建插件员工档案 |
| 打卡 | source_id、employee_source_id、clock_time、clock_type | clock_type 为 in/out/outside；status 为 normal/late/early/missing/abnormal |
| 请假、外出 | source_id、employee_source_id、approval_status、start_time、end_time、duration | duration 是小时，审批状态为 pending/approved/rejected/canceled；请假可加 leave_type，外出可加 destination |
| IM 用户 | source_id 与可匹配的身份字段 | 只映射既有宿主账号，不创建账号 |
| IM 消息 | source_id、sender_source_id、conversation_type、message_type、content、send_time | 私聊另需 receiver_source_id；群/频道另需既有目标 conversation_id |

员工/IM 用户可提供 `wecom_userid`、`employee_no`、`mobile`；`host_user_id` 是人工确定的既有目标账号 ID。最后使用 `username` 回退时，必须设置 `confirm_username: true` 表明已经人工核对。冲突手机号、已被其他员工绑定的账号、相互矛盾的身份都进入失败清单。

日期采用上海时区，支持 `YYYY-MM-DD HH:mm:ss` 或明确偏移的 RFC3339。员工可选 `host_dept_id`、`email`、`position`、`status`、`entry_date`、`quit_date`。省略的档案字段保留现有值。打卡和审批的 `employee_source_id` 引用本批员工或以前成功导入的员工记录。

## 导入顺序和重试

顺序固定为员工 → 考勤 → IM 用户映射 → 按原时间排序的 IM 消息。IM 消息使用 `ruoyi:<source_id>` 幂等键，保留历史发送时间；不会推送成新的实时消息。企业私聊历史仅要求双方已明确映射到既有且未删除的本地账号，不要求好友关系，也不因当前停用、离职或屏蔽而丢弃旧消息；导入不会启用账号、创建好友或更改屏蔽。群成员、频道权限仍须预先存在，本工具不重建历史群组。该历史导入规则仅用于 ETL，不放宽实时消息接口的账号与权限检查。

每一条成功业务写入与 `biz_migration_record` 审计在同一事务中提交。失败记录保存 source_json 完整输入、失败原因和 source_id，其他行继续处理。相同导出可重试；修复映射后重新导入失败行。失败员工的依赖考勤不会写入空 employee_id。

此工具提供可验证的规范化导入路径；真实 ruoyi 数据源尚未提供，因此没有执行生产数据迁移，也不宣称已经完成全量 1:1 对账。
