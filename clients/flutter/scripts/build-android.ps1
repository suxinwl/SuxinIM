param(
  [ValidateSet('development','production')][string]$Flavor = 'development',
  [string]$AndroidSdk = 'E:\AndroidDev\sdk',
  [string]$Jdk = 'E:\AndroidDev\jdk\jdk-17.0.20.1+1',
  [string]$ShortDrive = 'O:'
)
$ErrorActionPreference = 'Stop'
$clientRoot = Split-Path $PSScriptRoot -Parent
$suiteRoot = (Resolve-Path (Join-Path $clientRoot '../..')).Path
$mappedByScript = $false
if ($suiteRoot.Length -gt 20) {
  if ($ShortDrive -notmatch '^[O-Z]:$') { throw 'ShortDrive must be a drive letter O: through Z:.' }
  $mapping = & subst.exe | Where-Object { $_.StartsWith("$ShortDrive\") }
  if ($mapping) {
    if ($mapping -notlike "*$suiteRoot") { throw "$ShortDrive already belongs to another directory. Select a free ShortDrive." }
  } else {
    if (Test-Path "$ShortDrive\") { throw "$ShortDrive is an existing disk. Select another ShortDrive." }
    & subst.exe $ShortDrive $suiteRoot
    if ($LASTEXITCODE) { throw 'Cannot create short workspace path.' }
    $mappedByScript = $true
  }
  $suiteRoot = "$ShortDrive\"
  $clientRoot = Join-Path $suiteRoot 'clients/flutter'
}
$flutter = Join-Path $suiteRoot 'tools/flutter-3.32.8/bin/flutter.bat'
if (-not (Test-Path -LiteralPath $flutter)) { throw 'Run bootstrap-flutter.ps1 first.' }
if (-not (Test-Path -LiteralPath (Join-Path $clientRoot 'android/key.properties'))) { throw 'Missing private Android signing configuration.' }
$previousJava = $env:JAVA_HOME
$previousAndroid = $env:ANDROID_SDK_ROOT
$previousEngine = $env:FLUTTER_PREBUILT_ENGINE_VERSION
$previousGradle = $env:GRADLE_USER_HOME
$previousTemp = $env:TEMP
$previousTmp = $env:TMP
try {
  $env:JAVA_HOME = $Jdk
  $env:ANDROID_SDK_ROOT = $AndroidSdk
  $env:FLUTTER_PREBUILT_ENGINE_VERSION = 'ef0cd000916d64fa0c5d09cc809fa7ad244a5767'
  $env:GRADLE_USER_HOME = Join-Path $suiteRoot '.cache/gradle'
  $env:TEMP = Join-Path $suiteRoot '.cache/tmp'
  New-Item -ItemType Directory -Force -Path $env:TEMP | Out-Null
  $env:TMP = $env:TEMP
  Push-Location $clientRoot
  $sdkPath = $AndroidSdk.Replace('\','/')
  $flutterPath = (Join-Path $suiteRoot 'tools/flutter-3.32.8').Replace('\','/')
  [IO.File]::WriteAllText((Join-Path $clientRoot 'android/local.properties'), "sdk.dir=$sdkPath`nflutter.sdk=$flutterPath`n")
  & $flutter pub get
  if ($LASTEXITCODE) { throw 'Dependency resolution failed.' }
  $dart = Join-Path $suiteRoot 'tools/flutter-3.32.8/bin/cache/dart-sdk/bin/dart.exe'
  $analysisTargets = @(
    'lib','openim_common/lib','openim_live/lib','openim_meeting/lib',
    'openim_working_circle/lib','local_plugin/app_im/lib',
    'local_plugin/compat/flutter_webrtc-1.6.0/lib/src/native/rtc_video_renderer_impl.dart',
    'local_plugin/compat/flutter_webrtc-1.6.0/lib/src/native/mediadevices_impl.dart',
    'local_plugin/compat/flutter_webrtc-1.6.0/lib/src/native/rtc_peerconnection_impl.dart',
    'local_plugin/compat/livekit_client-2.11.0/lib/src/widgets/video_track_renderer.dart',
    'local_plugin/compat/livekit_client-2.11.0/lib/src/core/room.dart',
    'local_plugin/compat/livekit_client-2.11.0/lib/src/core/engine.dart',
    'local_plugin/compat/livekit_client-2.11.0/lib/src/core/signal_client.dart',
    'local_plugin/compat/livekit_client-2.11.0/lib/src/events.dart',
    'local_plugin/compat/livekit_client-2.11.0/lib/src/managers/event.dart',
    'local_plugin/compat/livekit_client-2.11.0/lib/src/support/websocket/io.dart',
    'local_plugin/compat/livekit_client-2.11.0/lib/src/track/remote/video.dart',
    'local_plugin/compat/just_audio-0.9.46/lib/just_audio.dart',
    'local_plugin/compat/flutter_bugly-1.1.1/lib/src/flutter_bugly.dart'
  )
  $analysis = & $dart analyze --format machine @analysisTargets
  $analysisExit = $LASTEXITCODE
  $analysis | Write-Output
  if ($analysisExit -gt 2 -or ($analysis | Where-Object { $_ -match '^ERROR\|' })) { throw 'Dart analysis failed.' }
  & $flutter test --no-pub test
  if ($LASTEXITCODE) { throw 'Flutter tests failed.' }
  & (Join-Path $clientRoot 'scripts/test-native-java.ps1') -Jdk $Jdk
  $development = if ($Flavor -eq 'development') { 'true' } else { 'false' }
  & $flutter build apk --release --flavor $Flavor --target-platform android-arm64 --no-pub --dart-define "OPENIM_DEVELOPMENT=$development"
  if ($LASTEXITCODE) { throw 'Android build failed.' }
  Get-ChildItem (Join-Path $clientRoot 'build/app/outputs/flutter-apk') -Filter '*.apk' | Get-FileHash -Algorithm SHA256
} finally {
  Pop-Location
  $env:JAVA_HOME=$previousJava
  $env:ANDROID_SDK_ROOT=$previousAndroid
  $env:FLUTTER_PREBUILT_ENGINE_VERSION=$previousEngine
  $env:GRADLE_USER_HOME=$previousGradle
  $env:TEMP=$previousTemp
  $env:TMP=$previousTmp
  if ($mappedByScript) { & subst.exe $ShortDrive /D }
}
