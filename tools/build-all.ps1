param([switch]$SkipDependencies,[switch]$SkipAndroid)
$ErrorActionPreference='Stop'
$upgradeRoot=Split-Path $PSScriptRoot -Parent
$upgradePython=Join-Path $upgradeRoot '.venv-tools\Scripts\python.exe'
if(-not (Test-Path -LiteralPath $upgradePython)){$upgradePython='python'}
$upgradePreviousPath=$env:PATH
$upgradePreviousTelemetry=$env:UMI_TELEMETRY
$env:PATH=(Join-Path $PSScriptRoot 'node-v22.23.4-win-x64')+';'+$env:PATH
$env:UMI_TELEMETRY='0'
function Invoke-Checked([string]$Program,[string[]]$Arguments){& $Program @Arguments;if($LASTEXITCODE -ne 0){throw "$Program failed: $LASTEXITCODE"}}
Push-Location $upgradeRoot
try {
 Invoke-Checked $upgradePython @('tools/build_servers.py')
 Invoke-Checked $upgradePython @('tools/build_livekit.py')
 Push-Location services/enterprise-service
 try {
  & ./build.ps1
  if($LASTEXITCODE -ne 0){throw 'Enterprise service build failed'}
 } finally {Pop-Location}
 Push-Location clients/web-desktop
 try {
  if(-not $SkipDependencies){Invoke-Checked npm.cmd @('ci','--legacy-peer-deps')}
  foreach($task in @('typecheck','test:adapter','build:web','build:win')){Invoke-Checked npm.cmd @('run',$task)}
  Invoke-Checked node @('scripts/build-manifest.mjs')
 } finally {Pop-Location}
 Push-Location clients/admin
 try {
  if(-not $SkipDependencies){Invoke-Checked npm.cmd @('ci','--legacy-peer-deps')}
  Invoke-Checked npm.cmd @('run','tsc');Invoke-Checked npm.cmd @('run','build')
 } finally {Pop-Location}
 if(-not $SkipAndroid){& (Join-Path $upgradeRoot 'clients/flutter/scripts/build-android.ps1') -Flavor development;if($LASTEXITCODE -ne 0){throw 'Android build failed'}}
 Invoke-Checked $upgradePython @('tools/release_manifest.py')
} finally {
 Pop-Location
 $env:PATH=$upgradePreviousPath
 $env:UMI_TELEMETRY=$upgradePreviousTelemetry
}
