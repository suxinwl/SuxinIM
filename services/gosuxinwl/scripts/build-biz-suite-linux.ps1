param(
  [string]$Version = "0.2.4-openim.2",
  [string]$GoOverlay = "",
  [string]$AdminOverrides = ""
)
$ErrorActionPreference = "Stop"
if ($Version -notmatch '^\d+\.\d+\.\d+([+-][A-Za-z0-9.-]+)?$') { throw "Invalid version" }
$taskProjectRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$taskStage = Join-Path $taskProjectRoot ("dist/biz-suite-linux-" + [Guid]::NewGuid().ToString("N"))
$taskBin = Join-Path $taskStage "bin"
$taskAdmin = Join-Path $taskStage "admin"
New-Item -ItemType Directory -Force -Path $taskBin,$taskAdmin | Out-Null
$taskOldOS,$taskOldArch,$taskOldCGO = $env:GOOS,$env:GOARCH,$env:CGO_ENABLED
Push-Location $taskProjectRoot
try {
  $env:GOOS="linux"; $env:GOARCH="amd64"; $env:CGO_ENABLED="0"
  $taskOverlayArgs = @()
  if ($GoOverlay) { $taskOverlayArgs = @('-overlay',(Resolve-Path -LiteralPath $GoOverlay).Path) }
  go build @taskOverlayArgs -trimpath -ldflags "-s -w" -o (Join-Path $taskBin "biz-suite") ./plugins/biz-suite/cmd
  if ($LASTEXITCODE -ne 0) { throw "Linux worker build failed" }
} finally {
  $env:GOOS=$taskOldOS; $env:GOARCH=$taskOldArch; $env:CGO_ENABLED=$taskOldCGO
  Pop-Location
}
Push-Location (Join-Path $taskProjectRoot "plugins/biz-suite/admin")
try {
  $taskAdminArgs = @()
  if ($AdminOverrides) { $taskAdminArgs = @('--source-overrides',(Resolve-Path -LiteralPath $AdminOverrides).Path) }
  node scripts/build.mjs --outdir $taskAdmin @taskAdminArgs
  if ($LASTEXITCODE -ne 0) { throw "Admin frontend build failed" }
} finally { Pop-Location }
Copy-Item -LiteralPath (Join-Path $taskProjectRoot "plugins/biz-suite/README.md") -Destination (Join-Path $taskStage "README.md")
Copy-Item -LiteralPath (Join-Path $taskProjectRoot "LICENSE") -Destination (Join-Path $taskStage "LICENSE.txt")
$taskManifest=[ordered]@{format="suxin-runtime-v1";name="biz-suite";title="HRM IM WeCom";version=$Version;protocol=1;os="linux";arch="amd64";files=[ordered]@{}}
Get-ChildItem -LiteralPath $taskStage -File -Recurse | Sort-Object FullName | ForEach-Object {
  $taskRelative=$_.FullName.Substring($taskStage.Length+1).Replace('\','/')
  $taskManifest.files[$taskRelative]=(Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
}
$taskManifestPath=Join-Path $taskStage "plugin.json"
[IO.File]::WriteAllText($taskManifestPath,($taskManifest|ConvertTo-Json -Depth 6),[Text.UTF8Encoding]::new($false))
$taskPackageDigest=(Get-FileHash -LiteralPath $taskManifestPath -Algorithm SHA256).Hash.ToLowerInvariant()
$taskZip=Join-Path $taskProjectRoot ("dist/biz-suite-linux-"+$Version+"-"+$taskPackageDigest.Substring(0,12)+".zip")
if (Test-Path -LiteralPath $taskZip) { throw "Package already exists: $taskZip" }
Add-Type -AssemblyName System.IO.Compression
Add-Type -AssemblyName System.IO.Compression.FileSystem
$taskArchive=[IO.Compression.ZipFile]::Open($taskZip,[IO.Compression.ZipArchiveMode]::Create)
try {
 Get-ChildItem -LiteralPath $taskStage -File -Recurse | ForEach-Object {
  $taskRelative=$_.FullName.Substring($taskStage.Length+1).Replace('\','/')
  $taskEntry=$taskArchive.CreateEntry($taskRelative,[IO.Compression.CompressionLevel]::Optimal)
  $taskInput=[IO.File]::OpenRead($_.FullName);$taskOutput=$taskEntry.Open()
  try {$taskInput.CopyTo($taskOutput)} finally {$taskOutput.Dispose();$taskInput.Dispose()}
 }
} finally {$taskArchive.Dispose()}
[ordered]@{zip=$taskZip;stage=$taskStage;package=$taskPackageDigest;sha256=(Get-FileHash -LiteralPath $taskZip -Algorithm SHA256).Hash.ToLowerInvariant()}|ConvertTo-Json
