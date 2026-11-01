# 3.5 企业项目到固定 3.8 开发版

## 边界与来源

原始3.5客户端界面与企业消息载荷保留，公版 IM/Chat 以固定标签导入。`docs/provenance/original-source-sha256.json` 记录5个原项目、6794个源码/配置/资源文件的基线（排除缓存、编译物和旧原生二进制）；`upstream-modifications.json` 及 patches 记录对固定上游的实际改动。原始目录没有修改。

这不是把3.5数据库文件直接挂到3.8，也没有把3.4/3.5结构混装。测试使用空数据库。将来迁移真实数据前，需要单独备份、脱敏副本转换、索引检查和数据一致性演练。本期不把未经真实数据验证的转换程序描述为已可生产使用。

## 配置映射

| 旧项目 | 新位置/处理 |
|---|---|
| 原企业 IM/Chat 配置及本地协议 | 保留作来源对照；官方服务各使用自己的 config/go.mod/protocol，不混用旧协议 |
| ZooKeeper 服务发现/旧Kafka模式 | etcd 的 openim38/chat38 独立命名空间；Kafka KRaft，单节点 |
| 旧MySQL Chat | 固定Chat新版的MongoDB模型，独立 chat38；没有将旧SQL直接导入新版Mongo |
| 旧专有 office/rtc 数据 | 新 enterprise38 独立业务库，官方数据只经公开API访问 |
| SDK缓存/本地数据库 | Web/Windows 使用 upgrade38 的配置和用户目录；Android独立包名，不覆盖旧端数据 |
| 旧专有RTC二进制 | 不参与运行，使用固定LiveKit；受邀、成员、主持和会话失效在企业服务执行 |
| 外部更新/ASAR/Pgyer | Chat版本记录 + 企业构建/大小/哈希；完整Windows安装包/AndroidAPK |
| 原默认demo密钥/地址 | 新实例随机密钥、独立测试地址；外部推送/短信/邮件默认关闭，注册所需邮件短信要另配 |

全局 multiLogin.policy=4；PC、移动、Web三类并存，同类别新登录替换旧登录。不是逐员工策略矩阵。AI/模型接入和商业流式接口不在本期。

## 接口与页面

| 原路径/能力 | 当前路径/适配 |
|---|---|
| 原好友、群组、会话、消息 | ImFacade/AppIm → 固定3.8 SDK |
| /office/* | 企业服务兼容映射，统一 /enterprise/v1；朋友圈、评论点赞、标签发送、OA通知 |
| /signaling/*、/rtc/* | 企业服务持久状态 + LiveKit受邀房间授权 |
| 群消息已读接口 | 企业服务验证真实消息、发送成员快照、分页及幂等上报 |
| 后台Chat接口 | /enterprise/v1/admin/chat/*；管理员Chat会话在服务器校验 |
| 后台业务用户接口 | /enterprise/v1/admin/business/* |
| 后台IM管理接口 | /enterprise/v1/admin/im/*；仅服务端取得并使用IM管理员令牌 |
| 后台创建用户 | Chat /account/add_user 的嵌套 user；不借验证码白名单模拟注册 |
| 后台历史会话 | REST分页拉取，不以管理员浏览器登录员工SDK来窥视历史 |
| 后台文件上传 | 新object multipart，支持初始化签名和按需签名；原minio_upload不再调用 |

管理浏览器不保存 IM 管理令牌，禁止代签普通用户令牌；只有 force_logout 保留为管理员的撤销操作。Chat裸LiveKit令牌路由在入口封禁，客户端只能使用企业服务受邀路径。旧企业业务消息子类型保持原值，不作为OpenIM主 contentType。

显式退出新增 `/enterprise/v1/session/logout`，先撤销当前Chat/IM令牌对，再清理SDK和本地状态；请求失败保留重试入口。企业服务持久化IM令牌摘要失效记录，并调用官方Auth精确撤销。上游Auth和网关的配套补丁仅关闭对应令牌的连接，保留管理员按整个平台强退的原行为；应先升级网关，再升级Auth。找回密码与普通改密也补齐Chat/IM撤销，找回身份来自服务端已验证邮箱/手机，不信任客户端提供的目标ID。具体修改和SHA在 provenance/patches，行为以实际回归报告为准。

逐项运行证据与未验收功能见 acceptance.md；SDK构建成功不代表所有旧页面动作都已通过跨端回归。
