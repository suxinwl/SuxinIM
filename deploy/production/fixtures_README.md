# 本轮两个合成账号的精确清理

这些工具仅处理 `.private/production/acceptance/test-accounts.json` 中原始两个 QA
账号。计划不包含口令；必须同时匹配用户名、宿主 UID、员工 ID、姓名、工号与空企业
微信 ID。**等客户端和身份验收负责人明确释放 A/B 后，才能执行 quiesce 或删除阶段。**
工具不会连接 SSH；服务器命令由 root 通过现有 SSH helper 上传并执行。

已完成的只读快照：`.private/production/acceptance/fixture-cleanup-plan.private.json`，
SHA256 `e49162858bd99dca933771896df7a33d49c19bd55fb78dcd743dcad6f9bbb57b`。
最初 plan 与服务器只读 inspect 的实际检查结果为
businessReferenceCount=0、foreignKeyCount=0、employeeRowsDeleted=0，私有快照为
`.backup-stage/fixture-cleanup-inspect-181ea82716541083.private.json`。
本轮两端负责人释放 A/B 后，root 已完成 quiesce 和精确两行员工删除，最终引用与外键
计数仍为零。首次删除遇到周期重放的 user outbox pending 而安全退出，任务处理完成后
以原条件重试成功，没有修改检查条件或删除 outbox。最终正常只读三接口核对：HRM
真实 302 人（active 301、resigned 1），QA 员工零行；通讯真实 300 ready、缺显式工号
1 人等待、离职 1 人停用等待，两个 QA disabled 投影独立保留。

## 先检查引用，再收敛账号，最后删员工行

1. root 将计划置于 `/srv/puty-suite/.backup-stage/fx-plan.json`，工具上传为
   `/srv/puty-suite/staging/fixtures_cleanup.py`（私有目录与受限文件），先运行：

   ```bash
   python3 /srv/puty-suite/staging/fixtures_cleanup.py inspect \
     --plan /srv/puty-suite/.backup-stage/fx-plan.json \
     --plan-sha256 e49162858bd99dca933771896df7a33d49c19bd55fb78dcd743dcad6f9bbb57b
   ```

   实际数据库中的外键与引用必须检查，而非根据开发 DDL 推断。工具扫描
   `information_schema` 外键、各表员工标识/宿主用户字段，以及声明的 JSON/员工数组。
   原迁移 DDL 未声明员工外键，但考勤打卡、请假、外出、出差、加班、排班、计算结果、
   调整、台账、员工范围 JSON、企微关联等仍是业务引用。任何引用或非空无效 JSON 都
   阻止删除，留下私有检查快照供人工判断；不会级联删除业务数据。

2. 在 Windows 3.8 根目录运行本地 API 收敛工具：

   ```powershell
   .venv-tools\Scripts\python.exe tools/production_fixture_cleanup.py quiesce --fixtures-released
   ```

   使用当前正常认证的 HRM 管理员会话。顺序为：按当前 revision 将五项通讯访问权限
   全关（原子审计、epoch 和撤销 outbox）→停用宿主账号→完整员工记录仅改 inactive→
   仅移除 A/B 的双向好友→等待 disabled 投影实际应用→原 API 软删除两个宿主账号。
   进度保存在 `fixture-cleanup-progress.private.json`。此工具不修改真实员工密码，
   不删除员工数据库行，也不会清 Redis。若中途失败，账号保持已收紧状态，停止检查。

3. 确认源组测试已在 finally 解散，客户端另建的测试组也已由其负责人精确清理。
   身份验收的 `identity-cleanup-inventory.json` 记录此范围；不得删除部门群或依据名称
   前缀批量删群。IM 合成附件不属于 HRM 员工行删除工具的范围，须由 root 审查
   identity 提供的精确存储 inventory 后另行删除，见下方。

4. root 复核本地收敛报告后，在服务器执行最终员工行删除：

   ```bash
   python3 /srv/puty-suite/staging/fixtures_cleanup.py delete-employees \
     --fixtures-released \
     --plan /srv/puty-suite/.backup-stage/fx-plan.json \
     --plan-sha256 e49162858bd99dca933771896df7a33d49c19bd55fb78dcd743dcad6f9bbb57b
   ```

   此阶段校验 suite owner marker、MySQL 容器 owner label、审查计划 SHA，并取得共享
   maintenance 锁。使用同一 SERIALIZABLE 事务，锁定两个源员工，重新检查所有引用，
   要求宿主已停用且软删、员工 inactive、五权限全关、disabled 投影版本收敛、目标
   outbox 无待处理项和持久 epoch 仍在，再 DELETE 精确两行；影响数必须为 2，否则
   回滚。数据库访问使用容器内已有凭据，不将密码写入参数或日志。

宿主软删行、角色关联、桥投影、epoch、tombstone、审计、outbox、消息及 RTC 审计
全部保留。不对 Mongo 或 Redis 执行删除，不执行 bucket/prefix purge。删除员工行
后再次只读确认真实员工仍为原数量、两个测试账号无法登录，通讯投影仍为 disabled。

## IM 合成附件

没有适合本轮的精确公开删除 API，不能调用按群/时间批量删除接口。identity 附件
inventory 同时限定两个合成 IM userID、`group=acceptance`、本轮时间窗及
`<fixtureUserID>/production-acceptance-<24hex>.txt`。记录 Mongo `openim38.s3` 的
精确 `_id/engine/key/name/size/time`，下载核对固定正文 SHA、MD5 与 ETag。当前
`SetObject` 不持久化 `user_id/hash`，不能把缺失字段当作已验证的归属依据；归属由
受审的原请求、完整名称、时间、存储键及正文共同约束。

客户端 SDK 的附件须另外使用实际 initiate/complete 请求和正文证据建立清单，SDK
会把 IM userID 加到名称前。稳定下载 URL 也不等同于 Minio key。所有获批 alias 都须
逐项验证完整 metadata 与正文，任何计划外引用都中止；共享同一内容对象时，先只
删除精确 metadata alias 并确认对象仍可读，最后一个受审引用清除后才删 blob 并
确认 HEAD 404。唯一内容对象同样须在零计划外引用后精确删除。

本轮原身份工具四个唯一对象已先行精确清理。最终追加清单包含旧 Windows 五条、
Android 正常主入口一条和轮换后身份复验一条，共七条 metadata alias、四个存储对象。
Android 正常 UI 对象以实际接收历史的 clientMsgID、完整 SDK 名称、时间与正文 SHA
确认，未虚构已丢失的 initiate/complete 日志。Android 与身份正文严格 text/plain，
Windows 严格 application/octet-stream。七项离线回归及独立 MIME 正负检查通过后，
实际只读 inventory、root 审查和绑定 runner/input/plan 三份 SHA 的清理共 81 项检查
通过。共享 blob 在仍有受审 alias 时保持可读，最后引用删除后确认 HEAD 404 与精确
Mongo ID 零条。受限 acceptance 目录保留前后计划与清理日志；不要重跑已完成清单。

先做只读 inventory 并审阅计划摘要，再对完全相同的清单显式 `--apply`。Android
追加的上传必须先补入清单并重做 inventory，不能沿用旧计划。最终 state 的 uploadID
可解码定位 key，但早期诊断还有对象，不能只清最后一份。全程保留授权、撤销和审计。

离线校验：`.venv-tools\Scripts\python.exe tools/production_fixture_cleanup_test.py -v`。
七项 guard 回归（含仅错误号/SQLSTATE 的安全诊断）与 Python 语法检查已通过；
真实删除须由 root 在释放后执行。
