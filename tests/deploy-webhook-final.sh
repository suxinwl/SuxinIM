#!/usr/bin/env bash
set -euo pipefail
cd /srv/openim38-upgrade
test "$(pwd -P)" = /srv/openim38-upgrade
test -f compose.yaml
test "$(docker compose -p openim38-upgrade ps -q enterprise-service | xargs docker inspect --format '{{index .Config.Labels "com.docker.compose.project"}}')" = openim38-upgrade
echo '68c8cf397d8935b04a084507196b33932491242d7d0b186c27b090fe7cfa9428  /tmp/openim38-enterprise-webhook-final' | sha256sum -c -
echo 'ef5e5e75a5895815f300123b944ab5f3da99e4d88d7f8f5a64033bf06df2157f  /tmp/openim38-enterprise-webhook-final.test' | sha256sum -c -
install -d -m 700 .private/final-binaries
cp -n bin/enterprise-service .private/final-binaries/enterprise-before-webhook-final
install -m 755 /tmp/openim38-enterprise-webhook-final bin/enterprise-service.next
mv bin/enterprise-service.next bin/enterprise-service
install -m 755 /tmp/openim38-enterprise-webhook-final.test bin/enterprise-service.test
docker compose -p openim38-upgrade up -d --no-deps --force-recreate enterprise-service
sha256sum bin/enterprise-service bin/enterprise-service.test
