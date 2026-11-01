#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
upgrade_go=${OPENIM38_GO:-go}
[[ $($upgrade_go version) == 'go version go1.25.14 linux/amd64' ]] || { echo 'Provide isolated Go 1.25.14 with OPENIM38_GO' >&2;exit 2; }
export GOTOOLCHAIN=local CGO_ENABLED=0 GOOS=linux GOARCH=amd64
export GOMODCACHE="$PWD/.cache/go-mod" GOCACHE="$PWD/.cache/go-build"
for module in openim-server chat; do
 mkdir -p "artifacts/linux-amd64/$module"
 (cd "services/$module"; "$upgrade_go" build -mod=readonly -trimpath -buildvcs=false -o "../../artifacts/linux-amd64/$module/" ./cmd/...)
done
(cd services/enterprise-service;mkdir -p dist/linux-amd64;"$upgrade_go" build -mod=readonly -trimpath -buildvcs=false -o dist/linux-amd64/enterprise-service ./cmd/enterprise-service)
python3 tools/build_livekit.py
python3 tools/release_manifest.py
