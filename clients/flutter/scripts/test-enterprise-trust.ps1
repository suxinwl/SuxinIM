param(
 [Parameter(Mandatory=$true)][string]$LanCertificate,
 [Parameter(Mandatory=$true)][string]$OtherCertificate,
 [string]$Jdk='E:\AndroidDev\jdk\jdk-17.0.20.1+1',
 [string]$AndroidSdk='E:\AndroidDev\sdk'
)
$ErrorActionPreference='Stop'
$suite=(Resolve-Path (Join-Path $PSScriptRoot '../../..')).Path
$client=Split-Path $PSScriptRoot -Parent
$publicRoot=Join-Path $client 'openim_common/assets/certificates/puty_enterprise_root_ca.crt'
if((Get-FileHash $publicRoot -Algorithm SHA256).Hash.ToLowerInvariant() -ne '9a135d11160e2179c6d396088b765143b60b9ed82c44a8acc2cd8735e6d3f183'){throw 'Unexpected enterprise root'}
$webrtcAar=Get-ChildItem -LiteralPath (Join-Path $suite '.cache/gradle/caches/modules-2/files-2.1/io.github.webrtc-sdk/android/144.7559.09') -Recurse -File -Filter '*.aar' | Select-Object -First 1
if(-not $webrtcAar){throw 'Resolve Android WebRTC dependency before this test'}
$target=Join-Path $client ('build/enterprise-trust-tests/'+[Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Force $target | Out-Null
Add-Type -AssemblyName System.IO.Compression.FileSystem
$zip=[IO.Compression.ZipFile]::OpenRead($webrtcAar.FullName)
try{[IO.Compression.ZipFileExtensions]::ExtractToFile($zip.GetEntry('classes.jar'),(Join-Path $target 'classes.jar'),$true)}finally{$zip.Dispose()}
$classpath=(Join-Path $AndroidSdk 'platforms/android-36/android.jar')+';'+(Join-Path $target 'classes.jar')
$verifier=Join-Path $client 'local_plugin/compat/flutter_webrtc-1.6.0/android/src/main/java/com/cloudwebrtc/webrtc/EnterpriseTurnVerifier.java'
& (Join-Path $Jdk 'bin/javac.exe') --release 8 -cp $classpath -d $target $verifier (Join-Path $PSScriptRoot 'EnterpriseTurnVerifierTest.java')
if($LASTEXITCODE){throw 'Production enterprise verifier compilation failed'}
& (Join-Path $Jdk 'bin/java.exe') -cp ($target+';'+$classpath) com.cloudwebrtc.webrtc.EnterpriseTurnVerifierTest $publicRoot $LanCertificate $OtherCertificate
if($LASTEXITCODE){throw 'Enterprise TLS verifier tests failed'}
