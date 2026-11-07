> 当前升级项目：参阅[完整源码导航](../../README.md#源码导航)、[仓库说明](../../REPOSITORY.md)和[当前 Web / Windows 构建说明](../../docs/web-desktop.md)。Web 与 Windows 共用 src，Windows 本地功能位于 electron。
>
> 下方保留原项目的历史说明；其中旧演示地址和旧构建步骤不作为本次升级配置。当前发布版为 3.8.13 / 381301，显示名 PUTY内部通讯、包文件短名 PUTY_IM；Web 与 Windows 均已上线，Android 公开版保持 3.8.16 / 116。正式清单 SHA-256 为 `67865623baedc4d05a46af6b2eb8db78eff484ecbfadfdd798c2788579712f0b`，交付状态和下载验收参阅 [客户端交付说明](../PRODUCTION-DELIVERY.md)。
>
> 3.8.13 修复旧版迁移时 Windows 原生单实例锁文件被当作缓存删除而触发 EBUSY 的问题。持锁期间仅保留经验证的固定零字节原生锁，业务文件仍须完整加密备份校验；失败后不再残留“正在清理”进度。首次迁移、中断重试、旧目录删除后三种合成升级均在正式 ASAR 主入口通过，正常生产合成账号登录和同步等 5 项通过；用户已确认受影响员工电脑升级后可正常进入聊天。请保留文件与密钥，直接覆盖安装，数据使用专用目录，例如 J:\puty_im_data；安装器新选位置自动使用 puty_im 子目录。
>
> 下列 3.8.9 / 3.8.10 验证为各版本的历史记录，不替代本次迁移验收。
>
> 独立组织架构入口继续保留，部门树与人员列表来自 HRM 实时目录。组织页、添加好友与顶部搜索均支持手机号、姓名、工号；同事资料和免好友私聊资格由服务端验证。3.8.9 的 119 项组织/品牌回归与正常安装包内的真实搜索、资料、非好友消息证据保留在交付说明的历史记录中。
>
> 3.8.10 修复文件/视频发送、视频读取错误与超时提示，并对聊天、引用和工作圈图片/预览按当前查看者授权，隔离账号与会话缓存，保留原始签名查询。仓库固定 Node 22.23.4 下 144 项单测、TypeScript 与 Chrome 6 项保护图交互回归通过；正式 Web 和 PC 打包主入口的 11 类合成文件实际选择发送、损坏视频上传前拦截和 3 次组织页往返均通过，接收端独立原生历史、完整 SHA-256 与 206 分段字节核对通过，测试 SID 已精确退出。PC 最终矩阵样本已由服务器去重，实际 multipart PUT 200 另有修复后旧包复测证据。
>
> 本轮未重跑 NSIS 3.8.9→3.8.10 覆盖安装；PC 实测来自打包后的正常主入口，品牌、正式根、固定 SDK 和稳定身份均已核对。未代登录真实账号 1599，未向全员工作圈发布测试内容；Android ARM64 真机、完整音视频及 iOS 构建/签名和设备验收边界不变，仍未验收。
>
> 品牌单源为 [brand.json](../brand.json)，由 [sync_brand.py](../tools/sync_brand.py) 同步构建时显示名；登录前仅从公共配置读取 appName/webTitle，离线回退到品牌单源。批准透明 PNG 位于 branding/IM.png，由 sync_brand_assets.py 派生 PNG 与 OS 必需 ICO；默认头像 LOGO2 保持。Windows 应用身份、正式根、原账户数据目录不随显示名改变。

---

## Getting Started

### Update Config

- For web
  > src->config
  > modify it to your own ip, but do not need to modify the port

```bash
export const WS_URL = "ws://125.124.195.201:10001";
export const API_URL = "http://125.124.195.201:10002";
export const CHAT_URL = "http://125.124.195.201:10008";
```

### Develop

- Get dependencies from npm

  ```bash
  npm install
  ```

- Run and preview at local (web & electron)

  ```
  npm run dev

  ```

- Build (web)

  ```
  npm run build
  ```

- Build (electron)

  > You can refer to the dependencies in the `package_build.json` file to reduce the size of the installation package

  > Update `utils/open_im_sdk_wasm/api/database/instance.js` wasm import path first.
  >
  > ```javascript
  > - SQL = await initSqlJs({ locateFile: () => '/sql-wasm.wasm' });
  > + SQL = await initSqlJs({ locateFile: () => '../sql-wasm.wasm' });
  > ```

  > Disable browser legacy at `vite.config.ts`.
  >
  > ```javascript
  > - legacy({
  > -  targets: ["defaults", "not IE 11"],
  > -})
  > ```

  ```bash
  npm run el:build
  // or
  npm run el:build-win
  // or
  npm run el:build-linux
  ```

## Getting Help

The best way to interact with our team is through GitHub.You can open an issue with this.You can also find some Doc in [Our Developer Guide](https://doc.rentsoft.cn/).
