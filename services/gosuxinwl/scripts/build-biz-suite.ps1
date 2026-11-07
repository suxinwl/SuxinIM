param(
  [string]$Version = "0.3.1",
  [string]$Output = "dist/biz-suite.zip"
)

$ErrorActionPreference = "Stop"
$root = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$stage = Join-Path $root "dist/biz-suite-stage"
$bin = Join-Path $stage "bin"
$admin = Join-Path $stage "admin"
if ($Version -notmatch '^\d+\.\d+\.\d+([+-][A-Za-z0-9.-]+)?$') { throw "Invalid version" }
$dist = [System.IO.Path]::GetFullPath((Join-Path $root "dist"))
$stage = [System.IO.Path]::GetFullPath($stage)
$zipPath = [System.IO.Path]::GetFullPath((Join-Path $root $Output))
$prefix = $dist.TrimEnd([System.IO.Path]::DirectorySeparatorChar) + [System.IO.Path]::DirectorySeparatorChar
if (-not $stage.StartsWith($prefix, [StringComparison]::OrdinalIgnoreCase) -or -not $zipPath.StartsWith($prefix, [StringComparison]::OrdinalIgnoreCase) -or [System.IO.Path]::GetExtension($zipPath) -ne ".zip") { throw "Build outputs must stay inside repository dist directory" }
if (Test-Path -LiteralPath $stage) { Remove-Item -LiteralPath $stage -Recurse -Force }
New-Item -ItemType Directory -Force $bin, (Join-Path $admin "assets") | Out-Null

Push-Location $root
try {
  if ((go env GOOS).Trim() -ne "windows" -or (go env GOARCH).Trim() -ne "amd64") { throw "This package targets windows/amd64" }
  go build -trimpath -ldflags "-s -w" -o (Join-Path $bin "biz-suite.exe") ./plugins/biz-suite/cmd
  if ($LASTEXITCODE -ne 0) { throw "Worker build failed" }
  go build -trimpath -ldflags "-s -w" -o (Join-Path $dist "biz-suite-etl.exe") ./plugins/biz-suite/cmd/etl
  if ($LASTEXITCODE -ne 0) { throw "ETL build failed" }
  go build -trimpath -ldflags "-s -w" -o (Join-Path $dist "biz-suite-attendance.exe") ./plugins/biz-suite/cmd/attendance-bootstrap
  if ($LASTEXITCODE -ne 0) { throw "Attendance import tool build failed" }
} finally { Pop-Location }
Copy-Item (Join-Path $root "plugins/biz-suite/admin/assets/*") (Join-Path $admin "assets") -Recurse
Push-Location (Join-Path $root "plugins/biz-suite/admin")
try {
  node scripts/build.mjs --outdir $admin
  if ($LASTEXITCODE -ne 0) { throw "Frontend release build failed" }
} finally { Pop-Location }
Copy-Item (Join-Path $root "plugins/biz-suite/README.md") (Join-Path $stage "README.md")
Copy-Item -LiteralPath (Join-Path $root "LICENSE") -Destination (Join-Path $stage "LICENSE.txt")
if (Test-Path -LiteralPath (Join-Path $root "plugins/biz-suite/docs")) { Copy-Item -LiteralPath (Join-Path $root "plugins/biz-suite/docs") -Destination $admin -Recurse }
if (Test-Path -LiteralPath (Join-Path $root "plugins/biz-suite/examples")) { Copy-Item -LiteralPath (Join-Path $root "plugins/biz-suite/examples") -Destination $admin -Recurse }
Copy-Item -LiteralPath (Join-Path $root "plugins/biz-suite/internal/wecom/API_CONTRACT.md") -Destination (Join-Path $admin "docs/wecom-api.md")
Copy-Item -LiteralPath (Join-Path $root "plugins/biz-suite/internal/etl/README.md") -Destination (Join-Path $admin "docs/etl.md")
if (Test-Path -LiteralPath (Join-Path $root "plugins/biz-suite/admin/README.md")) { Copy-Item -LiteralPath (Join-Path $root "plugins/biz-suite/admin/README.md") -Destination (Join-Path $admin "docs/frontend.md") }

$manifest = [ordered]@{
  format = "suxin-runtime-v1"
  name = "biz-suite"
  title = "Biz Suite - HRM IM WeCom"
  version = $Version
  protocol = 1
  os = "windows"
  arch = "amd64"
  files = [ordered]@{}
}
Get-ChildItem $stage -File -Recurse | Where-Object { $_.Name -ne "plugin.json" } | ForEach-Object {
  $relative = $_.FullName.Substring($stage.Length + 1).Replace('\', '/')
  $manifest.files[$relative] = (Get-FileHash $_.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
}
$manifestJson = $manifest | ConvertTo-Json -Depth 5
[System.IO.File]::WriteAllText((Join-Path $stage "plugin.json"), $manifestJson, (New-Object System.Text.UTF8Encoding($false)))
New-Item -ItemType Directory -Force (Split-Path $zipPath) | Out-Null
$tempZip = $zipPath + "." + [Guid]::NewGuid().ToString("N") + ".tmp"
Add-Type -AssemblyName System.IO.Compression
Add-Type -AssemblyName System.IO.Compression.FileSystem
$archive = [System.IO.Compression.ZipFile]::Open($tempZip, [System.IO.Compression.ZipArchiveMode]::Create)
try {
  Get-ChildItem $stage -File -Recurse | ForEach-Object {
    $relative = $_.FullName.Substring($stage.Length + 1).Replace('\', '/')
    $entry = $archive.CreateEntry($relative, [System.IO.Compression.CompressionLevel]::Optimal)
    $input = [System.IO.File]::OpenRead($_.FullName)
    $outputStream = $entry.Open()
    try { $input.CopyTo($outputStream) } finally { $outputStream.Dispose(); $input.Dispose() }
  }
} finally { $archive.Dispose() }
Move-Item -LiteralPath $tempZip -Destination $zipPath -Force
Write-Host "Created $(Join-Path $root $Output)"
