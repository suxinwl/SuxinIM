#!/usr/bin/env bash
set -euo pipefail
cd /srv/openim38-upgrade
docker compose cp bin/enterprise-service.test enterprise-service:/app/enterprise-service.test
docker compose exec -T enterprise-service sh -c 'ENTERPRISE_TEST_MONGO_URI="$MONGO_URI" /app/enterprise-service.test -test.run "TestMongoCASPersistsAcrossClients|TestNativeWebhookWithoutOperationIDKeepsDistinctReadSequences" -test.v' > logs/mongo-cas-test.log
cat logs/mongo-cas-test.log
