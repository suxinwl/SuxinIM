# GoSuxin 管理后台

当前目录是 GoSuxin 的管理后台源码，采用 Vue 3、TypeScript、Vite、Pinia 和 Arco Design。功能包括登录、系统管理、角色权限、导航布局、内置 AI/MCP 管理和运行插件容器。

默认后台只显示通用功能。业务界面由安装的插件提供；首页“插件市场”支持上传运行插件包，“AI / MCP”支持配置模型及开发工具连接。

品牌配置位于 `public/config.js`，品牌图片位于 `public/` 及后端 `resource/static/brand/`。初始管理员使用 Suxin 品牌头像、名称与空白联系方式，数据库初始化由后端安装器负责。

## 环境与依赖

使用 Node.js 24.22.1 和 npm 11.21.1；Node 版本记录在 `.nvmrc`，npm 版本记录在 `package.json` 的 `packageManager`。从当前 `web/` 目录执行：

```shell
npm install --global npm@11.21.1
npm ci
```

依赖版本以 `package.json` 和 `package-lock.json` 为准。后端安装与配置见 [项目说明](../README.MD)。

升级批次、验证范围及后续主版本迁移见 [依赖升级记录](../docs/dependency-upgrade.md)。

## 开发服务

```shell
npm run serve
```

开发服务端口为 `9600`，可访问 `http://127.0.0.1:9600/`。先启动已完成安装的 GoSuxin 后端。

`public/config.js` 提供页面标题和接口地址，其中开发接口默认指向 `http://localhost:8603`。更改后端地址时，同步修改 `localhost` 配置及后端 `app.allowurl` 中允许的前端地址。

`config/vite.config.dev.ts` 还配置了 `/admin` 的开发代理，目前目标为 `http://127.0.0.1:8600/admin`。若使用相对路径请求并通过该代理访问本项目的默认后端，将目标改为 `http://127.0.0.1:8603/admin`；远程后端使用其实际地址。

## 检查与构建

```shell
npm run check
```

该命令依次执行全量类型检查、布局测试、前端行为测试及生产构建。也可按需单独运行：

```shell
npm run type:check
npm run test:layout
npm run test:frontend
npm run build
```

`npm run build` 包含类型检查；`npm run build:bundle` 仅构建资源。生产构建输出为 `dist/`，当前入口为宿主管理后台。

## 部署与安装包同步

生产资源的默认访问前缀为 `/suxinweb/`，配置在 `config/vite.config.prod.ts`。路由使用 `createWebHistory(import.meta.env.BASE_URL)`，直接跟随构建的资源前缀。

完成构建后返回项目根目录：

```shell
cd ..
python scripts/sync-frontend.py
python scripts/repack_webcode.py
```

静态资源同步至 `resource/suxinweb/`，由 GoSuxin 提供 `/suxinweb/` 及子路由的页面回退。同步脚本保留已打开浏览器页面可能仍使用的旧资源。源码包同步至 `devsource/developer/install/webcode.zip`，排除依赖、构建输出和本地临时文件；修改本目录文档后也应同步该包。

单独部署前端时，按实际部署位置调整 Vite 的 `base` 和 `public/config.js` 的 API 域名，并在 Web 服务器配置 History 路由回退。

`VITE_ENCRYPT` 必须与后端 `app.apisecret` 一致；正式部署可通过构建环境变量或被 Git 忽略的 `.env.production.local` 设置。前端会使用响应的 HTTP `Date` 校准请求时间，反向代理应保留该响应头。升级后刷新页面以加载新的登录流程。

## 运行插件页面

宿主通过运行插件容器嵌入 `/plugins/<name>/admin/`。插件前端独立构建，管理页面沿用宿主登录和权限。开发协议及打包要求见 [运行插件指南](../docs/runtime-plugins.md)。
