param([string]$TemporaryDirectory='D:\PUTY-local-build\mobile-go')
$ErrorActionPreference='Stop'
$suite=(Resolve-Path (Join-Path $PSScriptRoot '../../..')).Path
$reference=Join-Path $suite '.cache/sdk-core-patch15-reference'
$expectedCommit='d6e0b549db904d0327d76ef7c9c283203879a095'
if((& git -C $reference rev-parse HEAD).Trim() -ne $expectedCommit){throw 'Unexpected core SDK source'}
$trustSource=Join-Path $suite 'sdks/flutter/android/enterprise-trust'
$trustTarget=Join-Path $reference 'pkg/enterprise_trust'
New-Item -ItemType Directory -Force $trustTarget | Out-Null
Copy-Item -LiteralPath (Join-Path $trustSource 'trust.go') -Destination $trustTarget
Copy-Item -LiteralPath (Join-Path $trustSource 'puty_enterprise_root_ca.crt') -Destination $trustTarget
if((Get-FileHash (Join-Path $trustTarget 'puty_enterprise_root_ca.crt') -Algorithm SHA256).Hash.ToLowerInvariant() -ne '9a135d11160e2179c6d396088b765143b60b9ed82c44a8acc2cd8735e6d3f183'){throw 'Enterprise root differs from approved public root'}
foreach($relative in @('pkg/network/http_client.go','internal/interaction/ws_default.go')){
 $target=Join-Path $reference $relative
 $code=[IO.File]::ReadAllText($target)
 $import='_ "github.com/openimsdk/openim-sdk-core/v3/pkg/enterprise_trust"'
 if(-not $code.Contains($import)){
  $code=$code.Replace('import (', ('import ('+[Environment]::NewLine+' '+$import))
  [IO.File]::WriteAllText($target,$code,[Text.UTF8Encoding]::new($false))
 }
}
$env:GOMAXPROCS='1'
$env:GOMEMLIMIT='2GiB'
$env:GOFLAGS='-p=1'
$env:GOMODCACHE=Join-Path $suite '.cache/go-mod'
$env:GOCACHE=Join-Path $suite '.cache/go126-build'
$env:GOTMPDIR=$TemporaryDirectory
$env:GOBIN=Join-Path $suite 'clients/flutter/build/enterprise-go-tools'
$env:GOTOOLCHAIN='local'
$env:ANDROID_HOME='E:\AndroidDev\sdk'
$env:ANDROID_NDK_HOME='E:\AndroidDev\sdk\ndk\26.3.11579264'
$env:JAVA_HOME='E:\AndroidDev\jdk\jdk-17.0.20.1+1'
$env:PATH="$env:GOBIN;$suite\tools\go1.26.9\bin;$env:JAVA_HOME\bin;$env:PATH"
New-Item -ItemType Directory -Force $env:GOBIN,$env:GOTMPDIR | Out-Null
$mobileVersion='v0.0.0-20261011034528-197770444867'
& "$suite/tools/go1.26.9/bin/go.exe" install "golang.org/x/mobile/cmd/gomobile@$mobileVersion"
if($LASTEXITCODE){throw 'gomobile installation failed'}
& "$suite/tools/go1.26.9/bin/go.exe" install "golang.org/x/mobile/cmd/gobind@$mobileVersion"
if($LASTEXITCODE){throw 'gobind installation failed'}
Push-Location (Join-Path $suite '.cache/sdk-core-patch15-reference')
try {
 & "$suite/tools/go1.26.9/bin/go.exe" get -tool "golang.org/x/mobile/cmd/gobind@$mobileVersion"
 & "$suite/tools/go1.26.9/bin/go.exe" mod download golang.org/x/mobile
 & python (Join-Path $PSScriptRoot 'native-core-overlay.py') verify
 if($LASTEXITCODE){throw 'Fixed native core overlay verification failed'}
 $output=Join-Path $suite 'sdks/flutter/android/libs/puty-core-sdk-3.8.3-patch15-enterprise.aar'
 New-Item -ItemType Directory -Force (Split-Path $output -Parent) | Out-Null
 if((Get-FileHash (Join-Path $suite 'sdks/native-storage/sqlcipher-android-4.19.1/sqlcipher-android-4.19.1.aar') -Algorithm SHA256).Hash.ToLowerInvariant() -ne '43fc10cae8f1f1059f0933e8942a1eb328989d13426378597cd4add1b34a34fd'){throw 'Unapproved native SQLCipher artifact'}
 & "$env:GOBIN/gomobile.exe" bind -tags puty_sqlcipher,sqlite_omit_load_extension -androidapi 24 -trimpath -ldflags '-s -w -extldflags=-Wl,-z,max-page-size=16384' -o $output -target android/arm64,android/amd64 ./open_im_sdk/ ./open_im_sdk_callback/
 if($LASTEXITCODE){throw 'Enterprise core AAR build failed'}
 Get-FileHash $output -Algorithm SHA256
} finally {Pop-Location}
