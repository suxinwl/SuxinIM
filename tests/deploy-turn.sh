#!/usr/bin/env bash
set -euo pipefail
cd /srv/openim38-upgrade
test "$(pwd -P)" = /srv/openim38-upgrade
# Upload the new locked binary to bin/livekit-server.next first. The original
# diagnostic relay-range publication is obsolete and must never be reintroduced.
exec python3 deploy/install-livekit-turn.py --sha256 "${OPENIM38_LIVEKIT_SHA256:?Supply the published LiveKit binary SHA-256}"
