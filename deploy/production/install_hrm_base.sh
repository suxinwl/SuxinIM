#!/usr/bin/env bash
set -euo pipefail
base=/srv/puty-suite
[[ "$(realpath "$base")" == /srv/puty-suite && -f "$base/staging/hrm-base.tar.gz" ]] || exit 2
if ! id puty >/dev/null 2>&1; then useradd --system --home-dir "$base/gosuxin" --shell /usr/sbin/nologin puty; fi
install -d -m 0751 "$base" "$base/openim" "$base/gosuxin"
tar -xzf "$base/staging/hrm-base.tar.gz" -C "$base/gosuxin"
install -d -m 0755 "$base/gosuxin/storage/plugins/biz-suite/data" "$base/gosuxin/resource/uploads" "$base/gosuxin/runtime/log"
chown -R puty:puty "$base/gosuxin"
find "$base/gosuxin" -type d -exec chmod 0755 {} +
find "$base/gosuxin" -type f -exec chmod 0644 {} +
chmod 0755 "$base/gosuxin/main" "$base/gosuxin/tools/"*
chmod 0700 "$base/gosuxin/manifest/config" "$base/gosuxin/storage/plugins/biz-suite/data"
chmod 0600 "$base/gosuxin/manifest/config/"*.yaml "$base/gosuxin/runtime-environment.json" "$base/gosuxin/initial-password.secret"
chmod 0755 "$base/gosuxin/resource" "$base/gosuxin/resource/static" "$base/gosuxin/resource/uploads"
printf 'HRM base installed; service has not been started.\n'
