param(
  [string]$Jdk='E:\AndroidDev\jdk\jdk-17.0.20.1+1',
  [string]$ScratchRoot='D:\PUTY_AI\native-renewal-tests'
)
$ErrorActionPreference='Stop'
$suiteRoot=(Resolve-Path (Join-Path $PSScriptRoot '../../..')).Path
if(-not (Test-Path -LiteralPath $ScratchRoot)){New-Item -ItemType Directory -Path $ScratchRoot | Out-Null}
$scratchBase=(Resolve-Path -LiteralPath $ScratchRoot).Path
if((Get-Item -LiteralPath $scratchBase).Attributes -band [IO.FileAttributes]::ReparsePoint){throw 'Synthetic scratch link rejected'}
$run=Join-Path $scratchBase ('native-renewal-'+[Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $run | Out-Null
$production=Join-Path $suiteRoot 'sdks/flutter/android/src/main/java/io/openim/flutter_openim_sdk/NativeVaultRenewal.java'
$harness=Join-Path $suiteRoot 'sdks/flutter/android/src/test/java/io/openim/flutter_openim_sdk/NativeVaultRenewalHarness.java'
try {
  & (Join-Path $Jdk 'bin/javac.exe') -encoding UTF-8 --release 11 -d $run $production $harness
  if($LASTEXITCODE){throw 'Production renewal helper compilation failed'}
  & (Join-Path $Jdk 'bin/java.exe') -classpath $run io.openim.flutter_openim_sdk.NativeVaultRenewalHarness
  if($LASTEXITCODE){throw 'Synthetic renewal state-machine tests failed'}
} finally {
  $resolved=(Resolve-Path -LiteralPath $run).Path
  if([IO.Path]::GetDirectoryName($resolved) -eq $scratchBase -and
     [IO.Path]::GetFileName($resolved) -match '^native-renewal-[a-f0-9]{32}$' -and
     -not ((Get-Item -LiteralPath $resolved).Attributes -band [IO.FileAttributes]::ReparsePoint)) {
    Remove-Item -LiteralPath $resolved -Recurse -Force
  } else {throw 'Synthetic cleanup scope rejected'}
}
