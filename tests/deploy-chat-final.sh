#!/usr/bin/env bash
set -euo pipefail
cd /srv/openim38-upgrade
test "$(pwd -P)" = /srv/openim38-upgrade
for binary in admin-api admin-rpc chat-api chat-rpc enterprise-service; do
 test -s "bin/$binary.next"
 chmod 755 "bin/$binary.next"
 mv "bin/$binary.next" "bin/$binary"
done
# Old Admin ignores exact-token metadata and would revoke every session.
# Never start the new enterprise revoker before the compatible Chat Admin.
docker compose -p openim38-upgrade up -d --no-deps --force-recreate chat-admin
docker compose -p openim38-upgrade up -d --no-deps --force-recreate chat-business chat-api admin-api
sleep 3
docker compose -p openim38-upgrade up -d --no-deps --force-recreate enterprise-service
sha256sum bin/admin-api bin/admin-rpc bin/chat-api bin/chat-rpc bin/enterprise-service
