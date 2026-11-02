> 当前升级项目：参阅[完整源码导航](../../README.md#源码导航)、[仓库说明](../../REPOSITORY.md)和[当前 Web / Windows 构建说明](../../docs/web-desktop.md)。Web 与 Windows 共用 src，Windows 本地功能位于 electron。
>
> 下方保留原项目的历史说明；其中旧演示地址和旧构建步骤不作为本次升级配置，当前应用为 3.8.7 / 380701。

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
