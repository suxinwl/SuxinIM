param([string]$EvidencePrefix = 'android-110-x64')
$ErrorActionPreference = 'Stop'
if ($EvidencePrefix -notmatch '^android-110-x64$') { throw 'Explicit synthetic test evidence prefix required' }
$suite = Split-Path (Split-Path $PSScriptRoot -Parent) -Parent
$avdHome = Join-Path $suite '.private/avd'
$ini = Join-Path $avdHome 'openim38_api30.ini'
if (-not (Test-Path -LiteralPath $ini) -or
    -not (Select-String -LiteralPath $ini -Pattern '^target=android-30$' -Quiet)) {
  throw 'Expected this task existing API30 AVD; never create or modify another AVD'
}
$adb = 'E:\AndroidDev\sdk\platform-tools\adb.exe'
$devices = & $adb devices
if ($devices -match '^emulator-5588\s+device') {
  $name = (& $adb -s emulator-5588 emu avd name | Select-Object -First 1).Trim()
  if ($name -ne 'openim38_api30') { throw 'Port5588 belongs to another AVD' }
  Write-Output 'Own API30 AVD already running'
  return
}
$env:ANDROID_AVD_HOME = $avdHome
$env:ANDROID_SDK_ROOT = 'E:\AndroidDev\sdk'
$env:TEMP = Join-Path $suite '.cache/tmp'
$env:TMP = $env:TEMP
$arguments = @('-avd','openim38_api30','-port','5588','-no-window',
  '-no-snapshot-load','-no-snapshot-save','-no-boot-anim',
  '-gpu','swiftshader_indirect','-memory','4096','-cores','4')
foreach ($suffix in @('avd-start.json','avd-boot.log','avd-boot-stderr.log')) {
  if (Test-Path -LiteralPath (Join-Path $PSScriptRoot "$EvidencePrefix-$suffix")) { throw 'Preserve existing evidence' }
}
$emulator = Start-Process -FilePath 'E:\AndroidDev\sdk\emulator\emulator.exe' `
  -ArgumentList $arguments -WindowStyle Hidden -PassThru `
  -RedirectStandardOutput (Join-Path $PSScriptRoot "$EvidencePrefix-avd-boot.log") `
  -RedirectStandardError (Join-Path $PSScriptRoot "$EvidencePrefix-avd-boot-stderr.log")
$emulator.Id | Set-Content -LiteralPath (Join-Path $avdHome 'emulator.pid')
@{pid=$emulator.Id;avd='openim38_api30';serial='emulator-5588';
  arguments=$arguments;userdataPreserved=$true;sharedSystemImageModified=$false;
  bootAndSystemCATrustReady=$false} | ConvertTo-Json |
  Set-Content -LiteralPath (Join-Path $PSScriptRoot "$EvidencePrefix-avd-start.json") -Encoding utf8
Write-Output "Started only own API30 AVD, PID $($emulator.Id); wait for boot then install this task CA"
