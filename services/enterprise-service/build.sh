#!/usr/bin/env bash
set -euo pipefail
cd -- "$(dirname -- "$0")"
suite_root="$(cd ../.. && pwd)"
enterprise_go="${ENTERPRISE_GO:-go}"
if ! "$enterprise_go" version | grep -q 'go1.25.14 '; then
  printf '%s\n' 'Use Go 1.25.14; set ENTERPRISE_GO to its executable.' >&2
  exit 2
fi
export GOTOOLCHAIN=local GOMODCACHE="$suite_root/.cache/go-mod" GOCACHE="$suite_root/.cache/go-build" GOPROXY=https://proxy.golang.org,direct
"$enterprise_go" test -mod=readonly ./...
"$enterprise_go" vet -mod=readonly ./...
mkdir -p dist/linux-amd64
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 "$enterprise_go" build -mod=readonly -trimpath -buildvcs=false -ldflags='-s -w' -o dist/linux-amd64/enterprise-service ./cmd/enterprise-service
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 "$enterprise_go" test -mod=readonly -c -trimpath -o dist/linux-amd64/enterprise-service.test ./internal/enterprise
(cd dist/linux-amd64 && sha256sum enterprise-service enterprise-service.test > SHA256SUMS)
