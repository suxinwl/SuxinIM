$ErrorActionPreference='Stop'
$clientRoot=Split-Path $PSScriptRoot -Parent
$suiteRoot=(Resolve-Path (Join-Path $clientRoot '../..')).Path
$toolRoot=Join-Path $suiteRoot 'tools/flutter-3.32.8'
if(-not(Test-Path -LiteralPath $toolRoot)) {
  git -c core.longpaths=true clone --depth 1 --branch 3.32.8 https://github.com/flutter/flutter.git $toolRoot
  if($LASTEXITCODE){throw 'Flutter checkout failed.'}
}
$revision=git -C $toolRoot rev-parse HEAD
if($revision -ne 'edada7c56edf4a183c1735310e123c7f923584f1'){throw 'Unexpected Flutter revision.'}
$previousEngine=$env:FLUTTER_PREBUILT_ENGINE_VERSION
try {
  $env:FLUTTER_PREBUILT_ENGINE_VERSION='ef0cd000916d64fa0c5d09cc809fa7ad244a5767'
  & (Join-Path $toolRoot 'bin/flutter.bat') --version
  if($LASTEXITCODE){throw 'Flutter bootstrap failed.'}
} finally {$env:FLUTTER_PREBUILT_ENGINE_VERSION=$previousEngine}
