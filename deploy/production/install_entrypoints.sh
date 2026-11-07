#!/usr/bin/env bash
set -euo pipefail
base=/srv/puty-suite
test "$(realpath "$base")" = /srv/puty-suite
python3 - <<'PY'
import hashlib,json,pathlib,zipfile
b=pathlib.Path('/srv/puty-suite')
m=json.loads((b/'openim/instance.json').read_text())
assert m['production'] and m['testOnly'] is False and m['root']==str(b/'openim')
archive=b/'staging/hrm-web.zip'
assert hashlib.sha256(archive.read_bytes()).hexdigest()=='382d191afdb9cdbaaed4e33be554cb254af10cc1b71f876ea89d516d809af1f2'
target=b/'gosuxin/resource/suxinweb'
target.mkdir(parents=True,exist_ok=True)
with zipfile.ZipFile(archive) as z:
    for item in z.infolist():
        p=pathlib.PurePosixPath(item.filename)
        assert not p.is_absolute() and '..' not in p.parts
    z.extractall(target)
assert (target/'index.html').is_file()
PY
chown -R puty:puty "$base/gosuxin/resource"
find "$base/gosuxin/resource" -type d -exec chmod 0755 {} +
find "$base/gosuxin/resource" -type f -exec chmod 0644 {} +
install -d -m 0750 -o root -g www "$base/openim/certs"
chown root:www "$base/openim/certs/"*
chmod 0640 "$base/openim/certs/"*.key
chmod 0644 "$base/openim/certs/"*.crt
install -d -m 0755 "$base/openim/logs" "$base/openim/www"
install -m 0644 "$base/staging/puty-hrm.ini" /www/server/panel/plugin/supervisor/profile/puty-hrm.ini
install -d -m 0755 /www/server/panel/vhost/nginx/tcp
for item in puty-suite.conf puty-turn.conf; do
  target=/www/server/panel/vhost/nginx/$item
  if [[ $item == puty-turn.conf ]]; then target=/www/server/panel/vhost/nginx/tcp/$item; fi
  if [[ -f $target ]]; then cp -a "$target" "$target.previous"; fi
  install -m 0644 "$base/staging/$item" "$target"
done
/www/server/nginx/sbin/nginx -t
/www/server/panel/pyenv/bin/supervisorctl reread
/www/server/panel/pyenv/bin/supervisorctl update puty-hrm
/www/server/nginx/sbin/nginx -s reload
for port in 9880/tcp 9885/tcp 27881/tcp 27882/udp 23478/tcp 23478/udp 25349/tcp; do ufw allow "$port" comment 'PUTY suite'; done
ufw allow proto tcp from 172.30.38.10 to 172.30.38.1 port 18001 comment 'PUTY internal HRM policy'
ufw allow proto tcp from 172.30.38.10 to 172.30.38.1 port 17880 comment 'PUTY internal LiveKit'
/www/server/panel/pyenv/bin/supervisorctl status puty-hrm
