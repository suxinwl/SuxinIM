# 构建与隔离运行

## 已固定工具

IM、Chat、企业服务使用 Go 1.25.14；固定 LiveKit 1.13.11 原源码要求 Go 1.26，本项目为它单独固定 Go 1.26.9，见 `livekit.lock.json`。Node 22.23.4；Electron 45.2.1；electron-builder 26.18.0。Flutter/Android 工具和原生依赖以 `docs/flutter.md`、`tests/flutter/build-provenance.json` 为准。以上工具只在当前进程 PATH 或项目目录使用，不替换工作站和宝塔默认版本。

Windows 执行 `python tools/bootstrap_runtimes.py` 准备校验过的 Go/Node，再执行 `tools/build-all.ps1`。初次 Android 构建先执行 `clients/flutter/scripts/bootstrap-flutter.ps1`，准备 JDK17、Android SDK Platforms 35 和 36 及项目签名配置：应用compile/target为35，固定WebRTC与LiveKit插件compile为36，最低API仍24，插件编译平台不代表提高应用最低系统要求。各模块保留独立 go.mod/go.sum、npm package-lock 和 pubspec.lock。

Python辅助工具依赖在 `tools/requirements-tools.txt`，新环境先创建项目 `.venv-tools` 并在其中安装，不修改系统Python包。`tools/remote.py` 已随本目录保留，可用项目venv运行；SSH密码只经终端安全输入、保存在当前进程内存，固定核对192.168.10.11:2223主机指纹，不依赖原OpenIM目录的脚本。

Linux 可用 `OPENIM38_GO=/专用目录/go/bin/go bash deploy/build-linux.sh`，脚本要求准确 Go 1.25.14。Web/Windows 构建参考 `docs/web-desktop.md`；iOS 仅交付源码与 Mac 构建说明，本 Windows 环境不能生成 Xcode 验收结论。

## 新实例

`python tools/generate_instance.py` 在 `.private/instance` 生成新实例和随机密钥。重复生成保留原 secrets.json，但会重新生成配置，已有实例定制配置应先备份。发布实例时复制生成内容到专用根 `/srv/openim38-upgrade`，设置目录 700、密钥文件600。

将 `artifacts/linux-amd64/openim-server/*`、`artifacts/linux-amd64/chat/*`、`artifacts/linux-amd64/livekit/livekit-server`、企业服务二进制复制到 `bin/`，赋755。LiveKit 通过固定 Debian runtime 加只读二进制挂载运行；官方 LiveKit 镜像摘要保留在锁文件供溯源。Web dist-web 内容复制到 `www/`，后台 dist 内容复制到 `www/admin/`。`deploy/create-dev-cert.sh` 在服务器生成独立 CA/正确 SAN 证书，客户端只分发公钥证书。

容器镜像由 `deploy/images.lock.json` 的官方 Linux amd64 摘要固定，Compose pull_policy=never。网络可达时可按锁拉取；当前服务器镜像网络不可达，因此采用 `tools/export_oci.py` 生成保留原始 manifest 摘要的离线 OCI 档，`deploy/load_images.py` 校验后导入。`export_images.py` 的旧 Docker-save 形式不作为当前导入路径。

配置含所有业务进程，而非只有中间件：IM API、网关、传输、推送、定时、7个 RPC，Chat 两个 API/两个 RPC，企业服务，MongoDB、Redis、etcd、Kafka KRaft、MinIO、LiveKit 和 Nginx。数据库/RPC/LiveKit信令没有独立主机端口。

新栈启停：`sudo bash deploy/manage.sh start` / `stop` / `status`，同目录必须有 `stack_guard.py`。这些命令只针对 openim38-upgrade，不运行系统级 Docker 清理，不重置防火墙。二进制更新时先上传 `.next`，在本栈内原子替换后 **force-recreate 对应服务**，避免文件 bind mount 仍指向旧 inode；不能只restart后假定已升级。

Android真实强更测试先上传正式签名APK，再用 `tools/publish_test_artifacts.py --publish` 完整校验HTTPS下载并发布最终可选版本。`tools/set_test_update_policy.py` 只允许在本隔离实例切换当前Android版本的Chat强更标记，调用官方管理API；版本、构建号、URL、大小和SHA必须与本地正式APK及已发布下载报告一致。运行时必须显式提供 `--expected-build`、`--expected-sha256`、`--force true` 和新的 `--report-name`。完成系统安装验证后，以相同构建及SHA运行 `--force false`，另存恢复报告并确认旧版本看到可选更新、当前版本没有更新。安装包内容不可覆盖；更新包发生变化时使用新语义版本及递增构建号。

该测试工具对普通异常尝试恢复原策略；进程被终止或网络中断后，操作者仍须检查并恢复可选策略。它不安排后台自动恢复。报告只保存公开包元数据和策略结果，管理员凭据在进程内使用。脚本通过与实际强更下载、系统安装、启动验收分别记录。

## 网络与证书

29880 为 Web/API/WS/企业媒体授权入口，29885 保持附件 S3 原始 Host/路径/查询。Web与Windows需要 COOP/COEP，附件返回 CORP/CORS。Nginx 固定 X-Request-Api 到公开 `/api` 前缀，防止附件返回容器内部地址和 HTTP 混合内容。

LiveKit 媒体使用 TCP27881、UDP27882，嵌入 TURN 使用 UDP23478 和 TLS/TCP25349；Nginx stream 另提供 TCP23478 到嵌入 TLS TURN 的转接，并验证上游私有 CA 和 DNS SAN。LiveKit 默认向客户端发布 UDP 和 TLS 候选，明文 TCP23478 供显式配置的兼容客户端使用。协议探测与实际媒体中继分别验收，STUN Binding 成功不等于已完成中继通话。所有端口仅在192.168.10.11绑定，Nginx允许原约定13个内网网段；跨VLAN网关ACL仍需网络环境具备路由。

TURN 中继分配范围 UDP27900–28155 仅用于容器内部，**不发布主机端口**。桥接部署同时发布 SFU 的内部候选，并启用补丁 `allow_local_media_peers`：只识别本容器自身接口和 node IP，仍由原媒体端口策略限制到 RTC 媒体端口，不放开整个 Docker 网段或其他内网主机。这样中继到同机 SFU 不依赖主机回流和额外防火墙规则。`advertised_tls_port: 25349` 修正上游将 TLS 候选固定写为 443 的行为；缺省仍保持 443。补丁、构建及真实媒体检查见 `docs/livekit.md`。100 SDK消息持续测试不等同于100人同时音视频容量测试。

测试 CA 是 OpenIM38 Isolated Development CA，与原系统开发证书独立。Windows 当前用户导入：`certutil -user -addstore Root OpenIM38-Development-CA.crt`。Android development flavor 的Dart/Android HTTP显式信任此CA，但固定Go AAR不采用应用内CA、普通用户证书库或SSL_CERT_FILE；本次只在专用AVD系统信任库安装CA后验证原生IM链路。普通未Root手机需要公信服务器证书或经验证的系统信任部署；不能仅安装用户CA便认定整个App可用。iOS测试设备需安装并开启完全信任，原生链路仍需Mac和设备另验。保留证书链、主机名与SAN校验。此测试服务不配置公网端口映射。

## 数据与恢复

只使用 upgrade38_u001 等合成账号。前100保留给真实SDK并发测试，第101–107用于客户端联调，末3个用于企业API安全验收。私有密码和令牌不进入Git、公开下载目录或验收报告。

备份工具与 `stack_guard.py` 一起放在服务器 `deploy/`。从 `/srv/openim38-upgrade` 执行 `sudo bash deploy/backup-test.sh --mode live`，导出三个指定 MongoDB 库、MinIO 专用附件卷、配置和证书/密钥。源附件卷只读挂载，工具不会停止或重启源服务。**live 是跨写入时刻的逻辑导出，不是同一时点快照。** 包内明确记录模式、逐文件 SHA-256、导出前后集合行数和排除项；行数相同也不能证明并发更新的一致性。

输出为 `backups/*.tar.enc` 及同名 `.hmac.json`、`.sha256`。AES-256-CBC/PBKDF2 加密后再以独立派生的 HMAC-SHA256 密钥认证密文，恢复前先验 HMAC，失败不解密。必须保留三个文件，另将 `.private/backup.key` 安全存到不同位置；密钥不会被包入其自身备份。配置档含私钥，不得解密到公开下载目录。

执行 `sudo python3 deploy/backup-tool.py restore-check --archive /srv/openim38-upgrade/backups/<备份文件>.tar.enc` 进行演练。工具只在无网络、无主机端口、随机名称的临时 MongoDB 和专用卷中恢复；验证数据库集合、文件哈希、附件内容并清理自己创建且标签匹配的资源，结果写到 `tests/backup/restore-report.json`。不会安装备份配置，不覆盖源数据库或源附件卷。原栈的绑定路径、Compose 项目名及资源归属不符合隔离要求时，工具拒绝执行。

该格式**不包含 Redis、Kafka、etcd 持久卷，也不证明整个业务栈可零丢失回退**。需要停写备份时，先另行安排停写窗口并排空消息队列，再停止入口和业务写入进程，之后使用 `--mode quiesced`；工具只核验这些进程已经停止，不代替停写、排空队列或完整灾难恢复验收。本期演练使用 live 逻辑包，不能标注为业务一致性快照。

`python3 deploy/reset-test-data.py --confirm-stack openim38-upgrade --dry-run` 只核验并预览；去掉 `--dry-run` 才会删除测试栈合成数据库和附件。脚本检查实例布尔标记、固定 Compose 文件/本机 Docker 地址、资源名与归属、绑定路径，拒绝外部资源及越界挂载，不依赖可被 `python -O` 跳过的 assert。实际重置会停止本测试栈，必须避开压测/验收；先另存需要保留的数据，重置后客户端使用新测试资料目录。当前没有对真实数据库执行迁移脚本。
