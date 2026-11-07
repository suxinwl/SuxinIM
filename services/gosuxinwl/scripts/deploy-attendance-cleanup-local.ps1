param(
  [Parameter(Mandatory=$true)][string]$Archive,
  [Parameter(Mandatory=$true)][string]$ExpectedCurrent,
  [Parameter(Mandatory=$true)][string]$Package,
  [Parameter(Mandatory=$true)][string]$ArchiveSHA256
)
$ErrorActionPreference = 'Stop'
$taskRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).Path
Set-Location -LiteralPath $taskRoot
foreach ($value in @($ExpectedCurrent,$Package,$ArchiveSHA256)) { if ($value -notmatch '^[a-f0-9]{64}$') { throw 'Exact SHA256 values are required.' } }
$taskPlugin = Join-Path $taskRoot 'storage/plugins/biz-suite'
$taskActive = Join-Path $taskPlugin 'active.json'
$taskPrior = [IO.File]::ReadAllBytes($taskActive)
$taskState = [Text.Encoding]::UTF8.GetString($taskPrior) | ConvertFrom-Json
if ($taskState.package -ne $ExpectedCurrent -or $taskState.disabled) { throw 'Active package changed.' }
$taskArchive = (Resolve-Path -LiteralPath $Archive).Path
if ((Get-FileHash -LiteralPath $taskArchive -Algorithm SHA256).Hash.ToLowerInvariant() -ne $ArchiveSHA256) { throw 'Archive differs from reviewed package.' }
& (Join-Path $taskRoot 'runtime/runtime-plugin-install.exe') -inspect $taskArchive
if ($LASTEXITCODE -ne 0) { throw 'Package validation failed.' }
$taskCurrent = Join-Path $taskPlugin ('packages/' + $ExpectedCurrent)
$taskCurrentManifest = Get-Content -Raw -LiteralPath (Join-Path $taskCurrent 'plugin.json') | ConvertFrom-Json
foreach ($entry in $taskCurrentManifest.files.PSObject.Properties) {
  if ((Get-FileHash -LiteralPath (Join-Path $taskCurrent $entry.Name) -Algorithm SHA256).Hash.ToLowerInvariant() -ne $entry.Value) { throw 'Current package integrity changed.' }
}
if ((Get-FileHash -LiteralPath (Join-Path $taskCurrent 'plugin.json') -Algorithm SHA256).Hash.ToLowerInvariant() -ne $ExpectedCurrent) { throw 'Current manifest mismatch.' }
$taskDestination = Join-Path $taskPlugin ('packages/' + $Package)
Add-Type -AssemblyName System.IO.Compression.FileSystem
if (-not (Test-Path -LiteralPath $taskDestination)) { [IO.Compression.ZipFile]::ExtractToDirectory($taskArchive, $taskDestination) }
if ((Get-FileHash -LiteralPath (Join-Path $taskDestination 'plugin.json') -Algorithm SHA256).Hash.ToLowerInvariant() -ne $Package) { throw 'Extracted manifest mismatch.' }
$taskNew = Get-Content -Raw -LiteralPath (Join-Path $taskDestination 'plugin.json') | ConvertFrom-Json
foreach ($entry in $taskNew.files.PSObject.Properties) {
  if ((Get-FileHash -LiteralPath (Join-Path $taskDestination $entry.Name) -Algorithm SHA256).Hash.ToLowerInvariant() -ne $entry.Value) { throw 'Extracted package mismatch.' }
}
if ($taskNew.files.'bin/biz-suite.exe' -ne $taskCurrentManifest.files.'bin/biz-suite.exe') { throw 'This deployment only permits a frontend change.' }
$taskBackup = Join-Path $taskRoot ('runtime/backups/attendance-rule-cleanup-local-' + (Get-Date -Format 'yyyyMMdd-HHmmss'))
New-Item -ItemType Directory -Path $taskBackup | Out-Null
[IO.File]::WriteAllBytes((Join-Path $taskBackup 'active-before.json'),$taskPrior)
[IO.Compression.ZipFile]::CreateFromDirectory($taskCurrent,(Join-Path $taskBackup 'package-before.zip'))
$taskHostPath = Join-Path $taskRoot 'bin/gosuxin.exe'
function Stop-ExactProjectProcesses {
  foreach ($taskOwner in @(Get-NetTCPConnection -LocalPort 19880 -State Listen -ErrorAction SilentlyContinue | Select-Object -ExpandProperty OwningProcess -Unique)) {
    $taskProcess = Get-Process -Id $taskOwner -ErrorAction Stop
    if ($taskProcess.Path -ne $taskHostPath) { throw 'Port belongs to a different application.' }
    Stop-Process -InputObject $taskProcess -ErrorAction Stop
    if (-not $taskProcess.WaitForExit(10000)) { throw 'Host did not exit.' }
  }
  $taskAllowedWorkers = @((Join-Path $taskCurrent 'bin/biz-suite.exe'),(Join-Path $taskDestination 'bin/biz-suite.exe'))
  foreach ($taskWorker in @(Get-Process -Name 'biz-suite' -ErrorAction SilentlyContinue)) {
    if ($taskAllowedWorkers -contains $taskWorker.Path) { Stop-Process -InputObject $taskWorker -ErrorAction Stop }
  }
}
function Set-ActiveBytes([byte[]]$Bytes) {
  $taskTemp = Join-Path $taskPlugin ('active-cleanup-' + [Guid]::NewGuid().ToString('N') + '.json')
  $taskSwapBackup = $taskTemp + '.previous'
  [IO.File]::WriteAllBytes($taskTemp,$Bytes)
  try { [IO.File]::Replace($taskTemp,$taskActive,$taskSwapBackup) } finally {
    if (Test-Path -LiteralPath $taskTemp) { Remove-Item -LiteralPath $taskTemp }
    if (Test-Path -LiteralPath $taskSwapBackup) { Remove-Item -LiteralPath $taskSwapBackup }
  }
}
function Assert-RunningPackage([string]$Digest) {
  $taskExpectedWorker = Join-Path $taskPlugin ('packages/' + $Digest + '/bin/biz-suite.exe')
  for ($taskAttempt=0; $taskAttempt -lt 30; $taskAttempt++) {
    $taskWorker = @(Get-Process -Name 'biz-suite' -ErrorAction SilentlyContinue | Where-Object {$_.Path -eq $taskExpectedWorker})
    if ($taskWorker.Count -eq 1) {
      try { $taskHTTP = Invoke-WebRequest -UseBasicParsing -Uri 'http://127.0.0.1:19880/plugins/biz-suite/admin/hrm/attendance-rules' -TimeoutSec 3; if ($taskHTTP.StatusCode -eq 200) { return } } catch { }
    }
    Start-Sleep -Milliseconds 500
  }
  throw 'Expected package did not become healthy.'
}
if ([Convert]::ToBase64String([IO.File]::ReadAllBytes($taskActive)) -ne [Convert]::ToBase64String($taskPrior)) { throw 'Concurrent active package change.' }
try {
  Stop-ExactProjectProcesses
  $taskState.package = $Package
  Set-ActiveBytes ([Text.UTF8Encoding]::new($false).GetBytes(($taskState | ConvertTo-Json)))
  & (Join-Path $taskRoot 'scripts/start-local.ps1')
  Assert-RunningPackage $Package
} catch {
  $taskFailure = $_
  try { Stop-ExactProjectProcesses; Set-ActiveBytes $taskPrior }
  finally { & (Join-Path $taskRoot 'scripts/start-local.ps1') }
  Assert-RunningPackage $ExpectedCurrent
  throw $taskFailure
}
$taskReport = [ordered]@{applied=$true;package=$Package;previous=$ExpectedCurrent;backup=$taskBackup;worker_unchanged=$true}
$taskReport | ConvertTo-Json | Set-Content -LiteralPath (Join-Path $taskBackup 'deployment-report.json')
$taskReport | ConvertTo-Json
