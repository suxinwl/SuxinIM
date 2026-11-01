#!/usr/bin/env bash
set -euo pipefail
cd /srv/openim38-upgrade
for binary in admin-api admin-rpc chat-api chat-rpc;do
 chmod 755 "bin/$binary.next"
 mv -T "bin/$binary.next" "bin/$binary"
done
cp .private/chat-rpc-mail-fixture.yml config/chat/chat-rpc-chat.yml
docker compose up -d --no-deps --force-recreate chat-api admin-api chat-business chat-admin
