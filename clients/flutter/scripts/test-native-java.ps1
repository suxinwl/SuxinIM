param([string]$Jdk = 'E:\AndroidDev\jdk\jdk-17.0.20.1+1')
$ErrorActionPreference = 'Stop'
$suiteRoot = (Resolve-Path (Join-Path $PSScriptRoot '../../..')).Path
$classes = Join-Path $suiteRoot ('.cache/java-tests/' + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Force -Path $classes | Out-Null
$native = Join-Path $suiteRoot 'clients/flutter/local_plugin/compat/flutter_webrtc-1.6.0/android/src/main/java/com/cloudwebrtc/webrtc'
$tests = Join-Path $suiteRoot 'tests/flutter'
& (Join-Path $Jdk 'bin/javac.exe') -d $classes `
  (Join-Path $native 'RtcOperationGate.java') `
  (Join-Path $native 'TrackStatsSelector.java') `
  (Join-Path $native 'NativeMediaLifecycle.java') `
  (Join-Path $native 'NativePeerLifecycle.java') `
  (Join-Path $native 'NativeRendererBinding.java') `
  (Join-Path $native 'NativeCleanupSequence.java') `
  (Join-Path $native 'NativeLocalMediaRelease.java') `
  (Join-Path $tests 'RtcOperationGateTest.java') `
  (Join-Path $tests 'RtcClosingReplyTest.java') `
  (Join-Path $tests 'NativeOwnerDiagnosticsTest.java') `
  (Join-Path $tests 'NativePeerLifecycleIndependentTest.java') `
  (Join-Path $tests 'NativeDeadlineRecoveryIndependentTest.java') `
  (Join-Path $tests 'TrackStatsSelectorTest.java') `
  (Join-Path $tests 'NativeRendererBindingIndependentTest.java') `
  (Join-Path $tests 'NativeLocalMediaReleaseTest.java') `
  (Join-Path $tests 'NativeLocalMediaReleaseIndependentTest.java')
if ($LASTEXITCODE) { throw 'Native lifecycle Java tests did not compile.' }
foreach ($name in @('RtcOperationGateTest','RtcClosingReplyTest','NativeOwnerDiagnosticsTest','NativePeerLifecycleIndependentTest','NativeDeadlineRecoveryIndependentTest','TrackStatsSelectorTest','NativeRendererBindingIndependentTest','NativeLocalMediaReleaseTest','NativeLocalMediaReleaseIndependentTest')) {
  & (Join-Path $Jdk 'bin/java.exe') -cp $classes "com.cloudwebrtc.webrtc.$name"
  if ($LASTEXITCODE) { throw "Native lifecycle Java test $name failed." }
}
# Compile the actual production renderer, with only platform/JNI/EGL boundaries
# stubbed. Keep these classes separate from the pure helper test classpath.
$wiringClasses = Join-Path $classes 'renderer-wiring'
New-Item -ItemType Directory -Force -Path $wiringClasses | Out-Null
$stubs = @(Get-ChildItem -LiteralPath (Join-Path $tests 'native-renderer-stubs') -Filter '*.java' -File -Recurse | ForEach-Object { $_.FullName })
& (Join-Path $Jdk 'bin/javac.exe') --release 8 -d $wiringClasses @stubs `
  (Join-Path $native 'NativeMediaLifecycle.java') `
  (Join-Path $native 'NativeRendererBinding.java') `
  (Join-Path $native 'FlutterRTCVideoRenderer.java') `
  (Join-Path $tests 'NativeRendererWiringTest.java') `
  (Join-Path $tests 'NativeDeadlineRendererTest.java')
if ($LASTEXITCODE) { throw 'Actual renderer boundary wiring tests did not compile.' }
& (Join-Path $Jdk 'bin/java.exe') -cp $wiringClasses 'com.cloudwebrtc.webrtc.NativeRendererWiringTest'
if ($LASTEXITCODE) { throw 'Actual renderer boundary wiring test failed.' }
& (Join-Path $Jdk 'bin/java.exe') -cp $wiringClasses 'com.cloudwebrtc.webrtc.NativeDeadlineRendererTest'
if ($LASTEXITCODE) { throw 'Actual renderer deadline recovery wiring test failed.' }
# Exercise the real SurfaceTextureRenderer rather than the fake EGL-output
# class used above. Native EGL and platform boundaries remain controlled.
$surfaceClasses = Join-Path $classes 'surface-wiring'
New-Item -ItemType Directory -Force -Path $surfaceClasses | Out-Null
$surfaceStubs = @(Get-ChildItem -LiteralPath (Join-Path $tests 'native-surface-stubs') -Filter '*.java' -File -Recurse | ForEach-Object { $_.FullName })
& (Join-Path $Jdk 'bin/javac.exe') --release 8 -d $surfaceClasses @surfaceStubs `
  (Join-Path $native 'NativeMediaLifecycle.java') `
  (Join-Path $native 'SurfaceTextureRenderer.java') `
  (Join-Path $tests 'NativeSurfaceDeadlineTest.java')
if ($LASTEXITCODE) { throw 'Actual Surface lifecycle boundary tests did not compile.' }
& (Join-Path $Jdk 'bin/java.exe') -cp $surfaceClasses 'com.cloudwebrtc.webrtc.NativeSurfaceDeadlineTest'
if ($LASTEXITCODE) { throw 'Actual Surface lifecycle boundary test failed.' }

# Actual production CameraEventsHandler; isolated Log/interface stubs.
$cameraClasses = Join-Path $classes 'camera-state'
New-Item -ItemType Directory -Force -Path $cameraClasses | Out-Null
$cameraStubs = @(Get-ChildItem -LiteralPath (Join-Path $tests 'native-camera-state-stubs') -Filter '*.java' -File -Recurse | ForEach-Object { $_.FullName })
& (Join-Path $Jdk 'bin/javac.exe') --release 8 -d $cameraClasses @cameraStubs `
  (Join-Path $native 'CameraEventsHandler.java') `
  (Join-Path $tests 'CameraStateBehaviorTest.java')
if ($LASTEXITCODE) { throw 'Actual camera-state behavior tests did not compile.' }
& (Join-Path $Jdk 'bin/java.exe') -cp $cameraClasses 'com.cloudwebrtc.webrtc.CameraStateBehaviorTest'
if ($LASTEXITCODE) { throw 'Actual camera-state behavior test failed.' }
