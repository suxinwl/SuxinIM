#!/usr/bin/env bash
set -euo pipefail
cd /srv/openim38-upgrade
test "$(pwd -P)" = /srv/openim38-upgrade
mkdir -p .private/final-binaries
tar -xzf server-binaries-final.tar.gz -C .private/final-binaries
for source in .private/final-binaries/openim-server/*; do
 name="$(basename "$source")"
 cp "$source" "bin/$name.next"
 chmod 755 "bin/$name.next"
 mv "bin/$name.next" "bin/$name"
done
# Gateway/Auth already deployed together; all current official build outputs
# are now reconciled, including unchanged behavior with new dependency IDs.
docker compose -p openim38-upgrade up -d --no-deps --force-recreate \
 openim-rpc-user openim-rpc-friend openim-rpc-group openim-rpc-msg \
 openim-rpc-conversation openim-rpc-third openim-crontask openim-push \
 openim-msgtransfer openim-api
sha256sum bin/openim-*
