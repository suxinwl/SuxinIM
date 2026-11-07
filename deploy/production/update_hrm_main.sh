#!/usr/bin/env bash
set -euo pipefail
base=/srv/puty-suite
expected=d2643ce86d97de5cb193fccd0bd91fe6ea18baa2d6c3c627d22926abfcea1ad6
test "$(sha256sum "$base/staging/gosuxin-main-final" | cut -d' ' -f1)" = "$expected"
python3 "$base/openim/manage.py" status >/dev/null
test ! -L "$base/.backup-stage"
install -d -m 0700 "$base/.backup-stage"
exec 9>"$base/.backup-stage/backup.lock"
flock -n 9
nginx_conf=/www/server/panel/vhost/nginx/puty-suite.conf
cp -a "$nginx_conf" "$nginx_conf.before-hrm"
cp -a "$base/gosuxin/main" "$base/gosuxin/main.previous"
rollback() {
  cp -a "$base/gosuxin/main.previous" "$base/gosuxin/main.rollback"
  mv "$base/gosuxin/main.rollback" "$base/gosuxin/main"
  cp -a "$nginx_conf.before-hrm" "$nginx_conf"
  /www/server/panel/pyenv/bin/supervisorctl restart puty-hrm
  /www/server/nginx/sbin/nginx -t && /www/server/nginx/sbin/nginx -s reload
}
trap rollback ERR
install -m 0755 -o puty -g puty "$base/staging/gosuxin-main-final" "$base/gosuxin/main.new"
mv "$base/gosuxin/main.new" "$base/gosuxin/main"
install -m 0644 "$base/staging/puty-suite.conf" /www/server/panel/vhost/nginx/puty-suite.conf
/www/server/nginx/sbin/nginx -t
/www/server/panel/pyenv/bin/supervisorctl restart puty-hrm
/www/server/nginx/sbin/nginx -s reload
trap - ERR
