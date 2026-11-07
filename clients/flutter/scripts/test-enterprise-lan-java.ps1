param(
 [Parameter(Mandatory=$true)][string]$LanCertificate,
 [Parameter(Mandatory=$true)][string]$OtherCertificate,
 [string]$Jdk='E:\AndroidDev\jdk\jdk-17.0.20.1+1',
 [string]$AndroidSdk='E:\AndroidDev\sdk'
)
$ErrorActionPreference='Stop'
$clientRoot=Split-Path $PSScriptRoot -Parent
$suiteRoot=(Resolve-Path (Join-Path $clientRoot '../..')).Path
$classes=Join-Path $clientRoot 'build/native-lan-pkix-tests'
New-Item -ItemType Directory -Path $classes -Force | Out-Null
$androidJar=Join-Path $AndroidSdk 'platforms/android-36/android.jar'
$javaSource=Join-Path $clientRoot 'android/app/src/main/java/cn/rentsoft/flutter/openim/consumer/EnterpriseLanTls.java'
$harness=Join-Path $clientRoot 'test/native/EnterpriseLanTlsHarness.java'
& (Join-Path $Jdk 'bin/javac.exe') -encoding UTF-8 -classpath $androidJar -d $classes $javaSource $harness
if($LASTEXITCODE){throw 'Enterprise LAN native verifier compilation failed'}
& (Join-Path $Jdk 'bin/java.exe') -classpath ($classes+';'+$androidJar) cn.rentsoft.flutter.openim.consumer.EnterpriseLanTlsHarness (Join-Path $suiteRoot 'deploy/certificates/Puty-Enterprise-Root-CA.crt') $LanCertificate $OtherCertificate
if($LASTEXITCODE){throw 'Enterprise LAN native verifier tests failed'}

