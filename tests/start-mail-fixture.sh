#!/usr/bin/env bash
set -euo pipefail
cd /srv/openim38-upgrade
docker run -d --name openim38-upgrade-mail-fixture --label com.openim38.role=mail-test \
 --network openim38-upgrade-net --memory 128m --cpus 0.5 --pids-limit 64 \
 -v /srv/openim38-upgrade/tests/mail-fixture.py:/mail-fixture.py:ro \
 -v /srv/openim38-upgrade/.private:/fixtures \
 --entrypoint python3 \
 mcr.microsoft.com/playwright@sha256:d65108d1a8095d8b0c4a5e32bf1807a40a5ce90ff0c78473e1116b1c243235e8 /mail-fixture.py
