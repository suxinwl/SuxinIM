# LiveKit 固定源码和 TURN 适配

基线为官方 `v1.13.11`，提交 `385aa40f9292fe1db8caad87301e4b285b7c198a`，完整源码与 Apache-2.0 许可证在 `services/livekit/`。`livekit.lock.json` 记录官方源码归档、原镜像摘要、源码改动与补丁 SHA-256；可审阅补丁 `patches/livekit-v1.13.11-openim38-turn.patch`。

本项目只补两项兼容能力：

- `turn.advertised_tls_port` 控制下发给客户端的 TLS 端口；缺省 0 保持上游 443，本实例设为 25349。监听、证书校验和凭据算法保持原实现。
- `turn.allow_local_media_peers` 缺省关闭。开启时仅允许嵌入 SFU 当前自身接口和 node IP，避免 Docker IP 改变后失效。它仍经过原 `turnRelayPolicy` 的 RTC 媒体端口限制及 deny CIDR 覆盖；不允许其他私网主机，不影响独立 TURN 模式。

同时设 `rtc.advertise_internal_ip: true`。SFU 与嵌入 TURN 通过容器自身地址传输，不走主机发布端口回流；27900–28155 只作为内部 UDP 分配范围，不发布主机。对外仍为 TCP27881、UDP27882、UDP23478、TLS/TCP25349，另有原 TCP23478 兼容转接。

依据固定上游的 [ICE 服务器生成](https://github.com/livekit/livekit/blob/v1.13.11/pkg/service/roommanager.go)、[TURN 私网策略](https://github.com/livekit/livekit/blob/v1.13.11/pkg/service/turn.go)及[本机媒体端口限制](https://github.com/livekit/livekit/blob/v1.13.11/pkg/service/turnrelay.go)。

## 构建与安装

执行 `python tools/build_livekit.py`。它仅为 LiveKit 下载并校验隔离 Go 1.26.9，使用独立 cache/tmp 和只读依赖锁，不替换其他 Go。已有工具链可用 `OPENIM38_LIVEKIT_GO` 指定，但必须准确为 1.26.9。低内存默认 GOMEMLIMIT=768MiB、GOMAXPROCS=2、包并发 1；在独立 Linux 构建目录可设 `OPENIM38_LIVEKIT_MEMORY=2GiB`。不能把内存不足导致的编译失败当成构建成功。

产物位于 `artifacts/linux-amd64/livekit/`，包含程序、定向测试可执行文件、SHA256SUMS 和 build-provenance.json。Windows 只交叉编译；测试程序必须在 Linux 执行：

```sh
./livekit-turn.test -docker=false -test.run 'TestOpenIM38|TestTURNRelayPolicy_Allows|TestGetTURNMediaPorts|TestGetTURNLocalIPs' -test.v
```

这些定向测试不启动 Docker/Redis 测试夹具。真实媒体回归另行执行。`build-all.ps1`、`deploy/build-linux.sh` 已串联此独立构建步骤。

已运行实例更新时，先将程序上传到 `/srv/openim38-upgrade/bin/livekit-server.next`，把 `deploy/install-livekit-turn.py` 与 `stack_guard.py` 放在同一目录，再执行：

```sh
sudo python3 deploy/install-livekit-turn.py --sha256 <发布清单中的小写SHA256>
```

脚本核验合成实例、资源归属及程序哈希，私有备份原 LiveKit 配置/程序，再只更新和重建 LiveKit。不要在通话测试进行中执行。回退仅恢复该私有备份的 LiveKit service 段、livekit.yml 和原程序，再重建 LiveKit；不要用旧完整 compose.yaml 覆盖其他组件后续变更。操作范围仅限独立 OpenIM38 实例的 LiveKit 组件，不操作业务数据库或其他实例。

## 强制中继验证

`tests/relay-media.mjs` 使用两个合成账号和两个真实浏览器、摄像头/麦克风模拟输入，强制 `iceTransportPolicy: relay`。设置 `TURN_TRANSPORT=udp` 或 `tls` 分别只允许该服务器候选；`tcp` 使用相同凭据显式验证原 TCP23478 兼容入口。各自结果写到 `tests/media/relay-<transport>.json`。测试使用轻量空白页面加载真实 LiveKit SDK，显式授予摄像头、麦克风和 Local Network Access 权限；不初始化无关 IM WASM。浏览器和 Node 必须信任测试 CA，不使用忽略证书错误选项。新版 SDK 遇未开放的 `/rtc/v1` 可自然回退到旧双 PC 信令，测试不强制关闭 singlePeerConnection。

双方必须实际解码远端视频，四条发布/订阅 ICE pair 成功且双向媒体字节非零。Chrome 可能把中继套接字在 SFU 一侧的内部映射标记为 prflx；仅在它的 related 地址/端口、TURN URL 和 relayProtocol 精确对应已分配 relay 候选，且连接配置始终强制 relay 时才承认中继。这不是把任意 prflx 当成 TURN。STUN Binding 和端口可达测试不能代替这项媒体检查。

本次最终程序的 UDP、TLS、TCP 三组媒体检查均通过。另有 10 项 Linux 定向策略/补丁回归通过，见 `tests/media/livekit-unit.json`；独立离线构建参数见 `tests/media/linux-build-config.json`，最终运行哈希及四项端口映射见 `tests/media/turn-final.json`。这些结果不代表完整 LiveKit 测试套件或 100 人同时音视频压测通过。
