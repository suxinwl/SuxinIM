# PUTY 生产证书更新与续期

本工具更新 `/srv/puty-suite/openim/certs` 的 HR、IM 公网完整链和内网 IP
证书。不会更换公开根 CA，不会上传根 CA 签名私钥，不会修改 Nginx 模板，
不会重启 HRM、数据库或其他 IM 容器。安装会 reload Nginx，并在 LiveKit
原来运行时仅重启该容器；正在进行的音视频通话可能中断，应选维护窗口。

## 现有证书和固定根

公网来源是 `E:/PUTY_AI/puty_ssl/state/production/certificates`：
`hr.szputy.com.crt/.key` 和 `im.szputy.com.crt/.key`。如 `.crt` 缺少中间链，
工具从对应 `.issuer.crt` 补齐，逐张校验签名和有效期，再用操作系统公开信任库
验证完整路径。DNS SAN 必须明确包含对应域名，leaf 必须有 ServerAuth EKU，
私钥必须匹配。不能把中间证书直接当信任锚，也不能截短链来规避验证。

内网证书来源是 3.8 的 `.private/production-ca/lan-server.crt/.key`。根 CA
公开证书是 `deploy/certificates/Puty-Enterprise-Root-CA.crt`，固定 DER SHA256：

```text
836c3d6c28d5078cc8da6ebeff8e5f9a7fb7f140554e6a7834011d72299c3d69
```

该 pin 固定在准备工具和安装器中；公开证书改变就失败，不会自动接受新根。
`root-ca-key.pem` 与 `ca-unlock.json` 仅供本机签发内网 leaf，必须独立备份，
保持受限 ACL，不上传服务器，也不加入证书分发包。ServerAuth 内网 SAN 只允许
`192.168.10.11`，签发一年有效期的新 RSA leaf，不能超过根 CA 剩余有效期。

## 本机操作

从 3.8 根目录执行。现有 `.venv-tools` 包含所需 cryptography；公网验证使用
本机系统信任库。以下 `validate` 只读，不签发、不写私钥、不连接服务器：

```powershell
.venv-tools\Scripts\python.exe tools/production_certificates.py validate
.venv-tools\Scripts\python.exe deploy/production/certificates_test.py -v
```

公网续期仍由现有 `E:/PUTY_AI/puty_ssl` 流程负责。检查其现有计划任务和日志，
需要运行时使用 `python E:/PUTY_AI/puty_ssl/ssl_manager.py run`；不要添加强制续期
参数，也不要在多台机器同时操作同一域名的 DNS 验证。其既有 ARI/证书实际
有效期策略决定是否签新公网证书。生产分发必须使用 `state/production`，
不得把 ACME staging 测试证书部署到生产。

完成公网更新后生成受限的分发暂存包：

```powershell
.venv-tools\Scripts\python.exe tools/production_certificates.py prepare --renew-lan-if-needed
```

仅当内网 leaf 距到期 **少于 30 天**（包括已过期）才读取本机加密根私钥签新
leaf；否则沿用现有 leaf。新 leaf 保存在暂存包，并在本机更新
`.private/production-ca/lan-server.crt/.key`，旧 pair 保留在 `leaf-history`。
根 CA 证书和签名私钥始终不变。若只需分发当前有效证书，可省略该开关。
已过期的公网证书必须先通过原有 ACME 流程续好；工具不会绕过公信验证。

输出目录 `.private/certificates-staging/<时间-随机ID>/` 包括：

- `certificates.tar.gz`：仅六个 leaf/fullchain/key 文件、公开根证书和 manifest。
- `manifest.json`：每文件 SHA256/大小，leaf 指纹/有效期/验证链长，固定根 pin。
- `bundle.sha256`：整个分发包的 SHA256。
- `ssh-requests.jsonl`：供现有 `tools/production_remote.py` 使用的顺序请求。

该目录和私钥受 Windows 当前操作账户 ACL（或 POSIX 0700/0600）保护。命令只
输出路径、公开证书摘要和 hash，不打印私钥、CA 解锁密码或 SSH 密码。

## 通过已有 SSH helper 安装

沿用根任务已经建立且验证 host fingerprint 的 `production_remote.py` 会话，
按 `ssh-requests.jsonl` **逐条**提交，并检查每条成功后再提交下一条。不要盲目
把所有请求一次性输给 helper；该 helper 的协议会报告失败，但不会自动停止
后续请求。请求依次执行：

新 helper 支持从本地 JSONL 文件读取指定请求，可避免长 shell 命令被 PTY
约 512 字符的输入限制截断。例如向已有 helper 会话依次发送以下短 JSON，
每次等待成功结果后才发送下一个 index（从 0 开始）：

```json
{"op":"request_file","path":"E:/PUTY_AI/OpenIM3.4-3.8/OpenIM3.8.3patch.18_1.8.4patch.5/.private/certificates-staging/<bundleID>/ssh-requests.jsonl","index":0}
```

随后分别将 `index` 改为 1、2、3。路径是**操作员本机**上的请求文件，不是服务器
路径；helper 仍使用已验证的 SSH 会话，密码继续只留在内存。

1. 校验生产根路径/owner marker，创建唯一 mode-0700 证书 staging 目录。
2. 上传证书分发包，mode 0600。
3. 上传 `deploy/production/certificates_install.py`，mode 0700。
4. 调用安装器，传入完整包 SHA256，超时上限 600 秒。

不需要在磁盘保存 SSH 密码。也可由操作员直接将同一包和安装器安全传输后执行
生成请求中最后一条命令；不要手动用 `cp` 覆盖正在使用的证书而跳过验证。

服务器依赖 `/usr/bin/openssl`、Python 3.9+、Docker Compose v2、`www` 用户组和
`/www/server/nginx/sbin/nginx`。安装器不依赖 cryptography，不接受任何 CA 私钥。
安装 CLI 与独立备份/隔离恢复验证共用维护锁，避免备份读取过程中替换证书。

## 安装与失败处理

安装器验证精确 `/srv/puty-suite/openim/instance.json`、生产 project/instanceID、
现有 LiveKit 的 owner/project/service 标签和现有公开根 pin。它拒绝路径跳转、
符号链接/重复/多余 tar 成员、异常大文件、hash 不符或缺少文件的包。

服务器再次用 OpenSSL 验证公私链、当前有效期、SAN、ServerAuth 和私钥匹配，
使用系统 CA 库验证公网链，使用固定根验证 IP 链；根签名私钥不能充当 leaf
私钥。先检查原 Nginx 配置正常，再备份旧文件至
`/srv/puty-suite/.certificate-backups/<bundleID>/`（目录 0700）。

六个新文件逐个原子替换；私钥为 `root:www`、0640，证书为 `root:www`、0644。
公开 `puty-root.crt` **不覆盖**。然后运行 `nginx -t`，成功才 reload，再仅重启
原来运行的 LiveKit 容器。任何一步失败都会还原旧的六个文件，重新验证并 reload
Nginx；若已经重启过 LiveKit，也重启它以读回旧证书。备份内 journal 记录
`installed` 或 `rolled-back`。如果 Docker/Nginx 本身不可用导致回滚不能完成，
命令会失败，保留旧证书和 journal 供操作员恢复；不能把失败当作更新成功。

完成后核对安装结果的 `nginxValidatedAndReloaded: true`、`rootCAChanged: false`，
并从正常信任根的客户端重新访问 LAN HR/IM 和 WAN HR/IM；检查浏览器显示的新
leaf 指纹/到期日，再做新通话验证。不要使用 `-k`、ignoreHTTPSErrors 或关闭
证书验证证明更新成功。内网客户端继续使用原根 CA，无需重新分发或重装根。

建议日常检查公网续期任务运行结果、证书到期日和分发状态，内网在不足 30 天
的维护窗口执行上述准备/安装流程。本工具未创建计划任务；离线测试只使用临时
自建 fixture CA，已执行的生产安装演练单列于下节。实际维护操作应另行
记录分发包 SHA256、安装结果、客户端验证结果，以及是否签发了新 leaf。

## 已完成的验证记录

2026-11-03，开发侧 7 项离线测试通过，当前本地 HR/IM 公信链（验证长度 4）及
固定根 LAN 链均只读校验成功。生产操作员已用**现有有效证书**完成一次安装演练，
根 CA 未更换、未签新生产 leaf；`nginx -t`、reload 和仅 LiveKit 重启成功，
LAN/WAN 的四条 TURN 访问路径再次实测通过。随后 HRM 宿主正常 TLS 1.3 浏览器
访问成功，无忽略证书选项。该次生产演练使用早期安装器；最终版额外的共享维护锁
及根私钥误用防护已通过离线检查，留作后续维护，不能据此声称最终版已再次实装。
