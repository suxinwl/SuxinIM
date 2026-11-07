param(
  [string]$Jdk='E:\AndroidDev\jdk\jdk-17.0.20.1+1',
  [string]$AndroidSdk='E:\AndroidDev\sdk',
  [string]$ScratchRoot='D:\PUTY_AI\mobile-legacy-tests',
  [string]$Go='C:\Users\Administrator\go\pkg\mod\golang.org\toolchain@v0.0.1-go1.27.3.windows-amd64\bin\go.exe'
)
$ErrorActionPreference='Stop'
$suiteRoot=(Resolve-Path (Join-Path $PSScriptRoot '../../..')).Path
if(-not (Test-Path -LiteralPath $ScratchRoot)){New-Item -ItemType Directory -Path $ScratchRoot | Out-Null}
$scratchBase=(Resolve-Path -LiteralPath $ScratchRoot).Path
$run=Join-Path $scratchBase ('mobile-legacy-tests-'+[Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $run | Out-Null
$classes=Join-Path $run 'classes'
New-Item -ItemType Directory -Path $classes | Out-Null
try {
  $androidJar=Join-Path $AndroidSdk 'platforms/android-35/android.jar'
  & (Join-Path $Jdk 'bin/javac.exe') -encoding UTF-8 -source 11 -target 11 -classpath $androidJar -d $classes `
    (Join-Path $suiteRoot 'sdks/flutter/android/src/main/java/io/openim/flutter_openim_sdk/MobileLegacyQuarantine.java') `
    (Join-Path $suiteRoot 'clients/flutter/test/native/MobileLegacyQuarantineHarness.java')
  if($LASTEXITCODE){throw 'Quarantine production-source compilation failed'}
  & (Join-Path $Jdk 'bin/java.exe') -classpath ($classes+';'+$androidJar) io.openim.flutter_openim_sdk.MobileLegacyQuarantineHarness $run
  if($LASTEXITCODE){throw 'Quarantine synthetic tests failed'}
  $previous=@{}
  foreach($name in @('GOWORK','GOTOOLCHAIN','GOPROXY','GOSUMDB','GOMAXPROCS')){$previous[$name]=[Environment]::GetEnvironmentVariable($name,'Process')}
  try {
    $env:GOWORK='off';$env:GOTOOLCHAIN='local';$env:GOPROXY='off';$env:GOSUMDB='off';$env:GOMAXPROCS='2'
    $module=Join-Path $run 'go-codec'
    New-Item -ItemType Directory -Path $module | Out-Null
    $codec=(Join-Path $suiteRoot 'sdks/local-store-native').Replace('\','/')
    [IO.File]::WriteAllText((Join-Path $module 'go.mod'),("module puty.local/quarantineinterop`n`ngo 1.27.3`n`nrequire puty.local/localstore v0.0.0`nrequire golang.org/x/crypto v0.57.0 // indirect`nreplace puty.local/localstore => `""+$codec+"`"`n"),[Text.UTF8Encoding]::new($false))
    Copy-Item -LiteralPath (Join-Path $codec 'go.sum') -Destination (Join-Path $module 'go.sum')
    Push-Location $module
    try { & $Go run -mod=readonly -p=1 (Join-Path $suiteRoot 'clients/flutter/test/native/quarantine_codec_interop.go') $run }
    finally {Pop-Location}
    if($LASTEXITCODE){throw 'Real Go quarantine codec verification failed'}
    & (Join-Path $Jdk 'bin/java.exe') -classpath ($classes+';'+$androidJar) io.openim.flutter_openim_sdk.MobileLegacyQuarantineHarness $run 'verify-go'
    if($LASTEXITCODE){throw 'Real Go to Java quarantine codec verification failed'}
  } finally {
    foreach($name in $previous.Keys){[Environment]::SetEnvironmentVariable($name,$previous[$name],'Process')}
  }
} finally {
  $resolved=[IO.Path]::GetFullPath($run)
  if($resolved.StartsWith($scratchBase+[IO.Path]::DirectorySeparatorChar,[StringComparison]::OrdinalIgnoreCase) -and
     [IO.Path]::GetFileName($resolved).StartsWith('mobile-legacy-tests-')) {
    Remove-Item -LiteralPath $resolved -Recurse -Force
  } else {throw 'Synthetic cleanup scope rejected'}
}
