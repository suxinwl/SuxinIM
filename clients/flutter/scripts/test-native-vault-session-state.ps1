param(
  [string]$Jdk='E:\AndroidDev\jdk\jdk-17.0.20.1+1',
  [string]$AndroidSdk='E:\AndroidDev\sdk',
  [string]$ScratchRoot='D:\PUTY_AI\native-session-state-tests'
)
$ErrorActionPreference='Stop'
$suiteRoot=(Resolve-Path (Join-Path $PSScriptRoot '../../..')).Path
if(-not (Test-Path -LiteralPath $ScratchRoot)){New-Item -ItemType Directory -Path $ScratchRoot | Out-Null}
$scratchBase=(Resolve-Path -LiteralPath $ScratchRoot).Path
$run=Join-Path $scratchBase ('native-session-state-'+[Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $run | Out-Null
$production=Join-Path $suiteRoot 'sdks/flutter/android/src/main/java/io/openim/flutter_openim_sdk/NativeVaultSessionState.java'
try {
  $androidJar=Join-Path $AndroidSdk 'platforms/android-35/android.jar'
  $androidClasses=Join-Path $run 'android-api'
  $jvmClasses=Join-Path $run 'jvm'
  New-Item -ItemType Directory -Path $androidClasses,$jvmClasses | Out-Null
  & (Join-Path $Jdk 'bin/javac.exe') -encoding UTF-8 -source 11 -target 11 -classpath $androidJar -d $androidClasses $production
  if($LASTEXITCODE){throw 'Production Android API compilation failed'}
  & (Join-Path $Jdk 'bin/javac.exe') -encoding UTF-8 -source 11 -target 11 -d $jvmClasses `
    $production (Join-Path $suiteRoot 'clients/flutter/test/native/session_state/org/json/JSONObject.java') `
    (Join-Path $suiteRoot 'clients/flutter/test/native/NativeVaultSessionStateHarness.java')
  if($LASTEXITCODE){throw 'Synthetic session-state compilation failed'}
  & (Join-Path $Jdk 'bin/java.exe') -classpath $jvmClasses io.openim.flutter_openim_sdk.NativeVaultSessionStateHarness
  if($LASTEXITCODE){throw 'Synthetic strict session-state tests failed'}
} finally {
  $resolved=[IO.Path]::GetFullPath($run)
  if($resolved.StartsWith($scratchBase+[IO.Path]::DirectorySeparatorChar,[StringComparison]::OrdinalIgnoreCase) -and
     [IO.Path]::GetFileName($resolved).StartsWith('native-session-state-')) {
    Remove-Item -LiteralPath $resolved -Recurse -Force
  } else {throw 'Synthetic cleanup scope rejected'}
}
