param([switch]$SkipTests)
$ErrorActionPreference='Stop'
$enterpriseRoot=Split-Path -Parent $MyInvocation.MyCommand.Path
$suiteRoot=Split-Path -Parent (Split-Path -Parent $enterpriseRoot)
$enterpriseGo=Join-Path $suiteRoot 'tools\go1.25.14\bin\go.exe'
if (-not (Test-Path -LiteralPath $enterpriseGo)) { throw 'Run the suite tool bootstrap first: Go 1.25.14 is required.' }
$enterpriseOldEnv=@{}
$enterpriseEnvNames=@('GOROOT','GOTOOLCHAIN','GOMODCACHE','GOCACHE','GOTMPDIR','GOPROXY','GOOS','GOARCH','CGO_ENABLED')
foreach($enterpriseName in $enterpriseEnvNames){$enterpriseOldEnv[$enterpriseName]=[Environment]::GetEnvironmentVariable($enterpriseName,'Process')}
Push-Location -LiteralPath $enterpriseRoot
try {
 $env:GOROOT=Split-Path -Parent (Split-Path -Parent $enterpriseGo)
 $env:GOTOOLCHAIN='local'
 $env:GOMODCACHE=Join-Path $suiteRoot '.cache\go-mod'
 $env:GOCACHE=Join-Path $suiteRoot '.cache\go-build'
 $env:GOTMPDIR=Join-Path $suiteRoot '.cache\go-tmp'
 New-Item -ItemType Directory -Force -Path $env:GOTMPDIR | Out-Null
 $env:GOPROXY='https://proxy.golang.org,direct'
 $enterpriseVersion=& $enterpriseGo version
 if ($enterpriseVersion -notmatch 'go1\.25\.14 '){throw "Unexpected toolchain: $enterpriseVersion"}
 if (-not $SkipTests) {
  $env:GOOS='windows';$env:GOARCH='amd64';$env:CGO_ENABLED='0'
  & $enterpriseGo test -mod=readonly ./...
  if($LASTEXITCODE -ne 0){throw 'Enterprise tests failed.'}
  & $enterpriseGo vet -mod=readonly ./...
  if($LASTEXITCODE -ne 0){throw 'Enterprise vet failed.'}
 }
 $env:GOOS='linux';$env:GOARCH='amd64';$env:CGO_ENABLED='0'
 New-Item -ItemType Directory -Force -Path 'dist\linux-amd64' | Out-Null
 & $enterpriseGo build -mod=readonly -trimpath -buildvcs=false '-ldflags=-s -w' -o dist/linux-amd64/enterprise-service ./cmd/enterprise-service
 if($LASTEXITCODE -ne 0){throw 'Linux enterprise build failed.'}
 & $enterpriseGo test -mod=readonly -c -trimpath -o dist/linux-amd64/enterprise-service.test ./internal/enterprise
 if($LASTEXITCODE -ne 0){throw 'Linux integration-test build failed.'}
 $enterpriseHashes=Get-FileHash -Algorithm SHA256 -LiteralPath 'dist\linux-amd64\enterprise-service','dist\linux-amd64\enterprise-service.test'
 $enterpriseLines=$enterpriseHashes | ForEach-Object { $_.Hash.ToLowerInvariant()+'  '+[IO.Path]::GetFileName($_.Path) }
 Set-Content -LiteralPath 'dist\linux-amd64\SHA256SUMS' -Value $enterpriseLines -Encoding ascii
 $enterpriseHashes | Select-Object Path,Hash | ConvertTo-Json
} finally {
 Pop-Location
 foreach($enterpriseName in $enterpriseEnvNames){[Environment]::SetEnvironmentVariable($enterpriseName,$enterpriseOldEnv[$enterpriseName],'Process')}
}
