#!/usr/bin/env bash
set -euo pipefail
cd /srv/openim38-upgrade
test "$(pwd -P)" = /srv/openim38-upgrade
# Run only after the common 100-SDK acceptance window has completed.
python3 - <<'PY'
import json, pathlib, time
p=pathlib.Path('.private/load-start-at.json')
if p.exists():
    start=json.loads(p.read_text())['startAt']/1000
    if time.time()<start+1810: raise SystemExit('100-SDK acceptance window is still active')
PY
for binary in openim-msggateway openim-rpc-auth enterprise-service; do
 test -s "bin/$binary.next"
 chmod 755 "bin/$binary.next"
 mv "bin/$binary.next" "bin/$binary"
done
# The gateway must understand the exact-token filter before Auth sends it.
docker compose -p openim38-upgrade up -d --no-deps --force-recreate openim-msggateway
docker compose -p openim38-upgrade up -d --no-deps --force-recreate openim-rpc-auth enterprise-service
python3 - <<'PY'
from pathlib import Path
p=Path('compose.yaml'); text=p.read_text()
needle='      - 192.168.10.11:29885:29885/tcp'
# YAML emitted by PyYAML uses a four-space indent for sequence entries.
if needle not in text: needle='    - 192.168.10.11:29885:29885/tcp'
if '192.168.10.11:23478:23478/tcp' not in text:
    if needle not in text: raise SystemExit('Cannot identify isolated Nginx port block')
    indent=needle[:len(needle)-len(needle.lstrip())]
    text=text.replace(needle,needle+'\n'+indent+'- 192.168.10.11:23478:23478/tcp',1)
    p.write_text(text)
PY
docker compose -p openim38-upgrade config -q
docker compose -p openim38-upgrade up -d --no-deps --force-recreate nginx
docker compose -p openim38-upgrade exec -T nginx nginx -t
sha256sum bin/openim-msggateway bin/openim-rpc-auth bin/enterprise-service
