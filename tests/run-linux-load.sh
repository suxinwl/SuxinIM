#!/usr/bin/env bash
set -euo pipefail
cd /srv/openim38-upgrade
docker exec -d \
 -e LOAD_OFFSET=30 -e LOAD_COUNT=35 -e LOAD_SECONDS=1800 \
 -e LOAD_TMP_DIR=/tmp/load-temp -e TMPDIR=/tmp/load-temp \
 -e LOAD_START_FILE=/fixtures/load-start-at.json \
 -e LOAD_RUN_ID=openim38-100-20261101 \
 openim38-upgrade-load bash -c 'node load-100.mjs /fixtures/test-sessions.json > load-linux35a.log 2>&1'
docker exec -d \
 -e LOAD_OFFSET=65 -e LOAD_COUNT=35 -e LOAD_SECONDS=1800 \
 -e LOAD_TMP_DIR=/tmp/load-temp-b -e TMPDIR=/tmp/load-temp-b \
 -e LOAD_START_FILE=/fixtures/load-start-at.json \
 -e LOAD_RUN_ID=openim38-100-20261101 \
 openim38-upgrade-load bash -c 'node load-100.mjs /fixtures/test-sessions.json > load-linux35b.log 2>&1'
