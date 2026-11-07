#!/usr/bin/env bash
set -euo pipefail
base=/srv/puty-suite/openim
python3 "$base/manage.py" status >/dev/null
test "$(docker network inspect puty-suite-prod-net --format '{{index .Labels "com.docker.compose.project"}}')" = puty-suite-prod
control=/www/server/panel/pyenv/bin/supervisorctl
"$control" stop puty-hrm
trap '"$control" start puty-hrm >/dev/null 2>&1 || true' EXIT
# Preserve all named volumes. Only the owned bridge and containers are recreated.
docker compose --project-directory "$base" -f "$base/compose.yaml" -p puty-suite-prod down --timeout 45
install -m 0600 /srv/puty-suite/staging/compose-reserved.yaml "$base/compose.yaml"
python3 "$base/manage.py" start
/www/server/nginx/sbin/nginx -t
/www/server/nginx/sbin/nginx -s reload
"$control" start puty-hrm
trap - EXIT
