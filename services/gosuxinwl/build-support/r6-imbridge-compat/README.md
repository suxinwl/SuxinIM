# R6 兼容构建源码

此目录保留线上 HRM 使用的 R6 IM 兼容源码，供任意路径克隆的仓库构建。主项目 `internal/imbridge` 及 IM 页面中的后续开发代码保持原样，构建时显式选择这里的冻结副本。

## 内容与来源

- `_source/`：33 个 Go 文件（含测试）及 `openim.js`、`openim-access.js`。目录名前缀 `_` 避免根目录 `go test ./...` 把副本作为另一套包编译。
- `manifest.json`：这 35 个文件在原 `public-access-r6/hrm/build-manifest.json` 中的 SHA256，以及原清单整体 SHA256。这里只保留构建需要的来源记录，无个人数据、凭据或部署环境配置。
- `prepare.py`：Python 3.10+，只用标准库；校验冻结文件并按当前仓库位置生成 Go overlay 和前端 source overrides。它同时检查主项目 IM Go 文件清单；新增或缺少文件时拒绝构建，须先审查兼容范围。
- `.gitattributes`：禁止 Git 对冻结文件做换行转换，确保 Windows/Linux 克隆后的 SHA256 一致。

旧 `runtime/staging/r6-imbridge-compat` 保留作本机历史记录。克隆后的构建完全不依赖该目录、其他项目目录或旧线程记录。

## 使用

在仓库根目录执行（Linux 可将 `python` 换为 `python3`）：

```powershell
python build-support/r6-imbridge-compat/prepare.py --check
python build-support/r6-imbridge-compat/prepare.py
```

输出文件为 `.generated/go-overlay.json` 和 `.generated/admin-overrides.json`。它们包含本次克隆的绝对路径，已忽略提交；移动仓库后重新生成，不手工提交映射。

使用既有 Linux 插件打包脚本时必须显式指定版本：

```powershell
powershell -NoProfile -File scripts/build-biz-suite-linux.ps1 `
  -Version 0.3.2-openim-r6.2-rule-cleanup.1 `
  -GoOverlay build-support/r6-imbridge-compat/.generated/go-overlay.json `
  -AdminOverrides build-support/r6-imbridge-compat/.generated/admin-overrides.json
```

脚本的旧默认版本号不代表现行版本：Linux 默认 `0.2.4-openim.2`，Windows 默认 `0.3.1`。不要省略 `-Version`。未来发布新版本时使用明确的新版本号。

单独验证 Linux worker，可在 PowerShell 临时设置以下构建变量；完成后恢复环境：

```powershell
$savedGOOS, $savedGOARCH, $savedCGO = $env:GOOS, $env:GOARCH, $env:CGO_ENABLED
try {
  $env:GOOS = 'linux'; $env:GOARCH = 'amd64'; $env:CGO_ENABLED = '0'
  go build -overlay 'build-support/r6-imbridge-compat/.generated/go-overlay.json' `
    -trimpath -ldflags '-s -w' `
    -o build-support/r6-imbridge-compat/.generated/biz-suite-linux ./plugins/biz-suite/cmd
  if ($LASTEXITCODE -ne 0) { throw 'Worker build failed' }
  Get-FileHash build-support/r6-imbridge-compat/.generated/biz-suite-linux -Algorithm SHA256
} finally {
  $env:GOOS, $env:GOARCH, $env:CGO_ENABLED = $savedGOOS, $savedGOARCH, $savedCGO
}
```

Linux shell 等价命令：

```sh
python3 build-support/r6-imbridge-compat/prepare.py
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build \
  -overlay=build-support/r6-imbridge-compat/.generated/go-overlay.json \
  -trimpath -ldflags '-s -w' \
  -o build-support/r6-imbridge-compat/.generated/biz-suite-linux ./plugins/biz-suite/cmd
sha256sum build-support/r6-imbridge-compat/.generated/biz-suite-linux
```

## 已核对的构建基准

2026-11-02 的线上版本为 `0.3.2-openim-r6.2-rule-cleanup.1`。使用本仓库当时的主源码、此兼容层及 Go 1.27.3，Linux/amd64、CGO 关闭、`-trimpath -ldflags '-s -w'`：

- worker SHA256：`d53fd408524ee65af022337ddaa62fa4198c6d93872bdc894bee0d64db10831b`。
- 同参数构建仓库根宿主的 SHA256：`09fdfd9ccc072160b85418007442d87590ca3ceb5f2639c57ec529feb5947a8d`。

主源码以后发生变更时，以上 SHA256 不再是新构建必须满足的值；冻结副本仍必须满足自己的清单。完整插件重新打包可能生成不同的 bundle 文件名和 ZIP 标识，不能仅凭 worker 相同宣称整个发布包字节相同。此目录不包含运行数据，也不会执行部署。
