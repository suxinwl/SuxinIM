# 企业服务验收

1. `services/enterprise-service/build.ps1` 使用项目 Go 1.25.14 运行域、HTTP 和 TCP/gRPC 测试以及 vet，并生成 Linux 服务和专项测试二进制。脚本退出后恢复进程环境变量。Linux 对应 build.sh。
2. Mongo 专项：在隔离栈中把 `ENTERPRISE_TEST_MONGO_URI` 设置为该栈 URI，运行 `enterprise-service.test -test.run TestMongoCASPersistsAcrossClients -test.v`。只创建并删除随机 enterprise_test_UUID 数据库，不连接既有生产实例。
3. 先执行根目录 tests/seed_accounts.py，再运行本目录 `python smoke.py` 和 `python security_and_updates.py`，使用正常 TLS 和项目开发 CA。

smoke 使用最后三个合成账号，不动前 100 个并发 SDK 账号；每次刷新自己的 Web 会话。验证真实 Chat/IM 认证、朋友圈及标签权限、LiveKit 建删房、会议邀请和主持权限、官方群消息回调及已读名单。

security_and_updates 使用相同三个合成账号的 Windows 3、Web 5、Android 2 平台，验证共存及同类互踢、旧媒体会话、管理员强退、封禁及密码修改，并在 finally 恢复原合成密码、解封和关闭测试房间。临时 Linux 更新元数据只用于真实 Chat Add/Latest API 验证，不是客户端安装包，测试后从 Chat 删除；不修改 Windows/Android 的发布记录。

报告不记录密码或令牌。报告中的通过仅覆盖列出的 HTTP／服务端场景；真实 SDK WebSocket、音视频轨道、跨端播放、客户端升级安装及 100 连接持续运行需另行验收。
