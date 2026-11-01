#!/usr/bin/env bash
set -euo pipefail
cd /srv/openim38-upgrade
[[ $(pwd -P) == /srv/openim38-upgrade ]] || exit 2
mkdir -p .private/load-home
chmod 700 .private/load-home
docker run -d --name openim38-upgrade-load --label com.openim38.role=load-test \
 --network host --memory 18g --cpus 8 --shm-size 1g --pids-limit 1536 \
 -v /srv/openim38-upgrade/load:/load \
 -v /srv/openim38-upgrade/.private:/fixtures:ro \
 -v /srv/openim38-upgrade/certs:/ca:ro \
 -v /srv/openim38-upgrade/.private/load-home:/root \
 -w /load --entrypoint sleep \
 mcr.microsoft.com/playwright@sha256:d65108d1a8095d8b0c4a5e32bf1807a40a5ce90ff0c78473e1116b1c243235e8 infinity
docker exec -d openim38-upgrade-load bash -c 'apt-get update -qq > /load/linux-prepare.log 2>&1 && apt-get install -y --no-install-recommends libnss3-tools >> /load/linux-prepare.log 2>&1 && cp /ca/ca.crt /usr/local/share/ca-certificates/openim38.crt && update-ca-certificates >> /load/linux-prepare.log 2>&1 && mkdir -p /root/.pki/nssdb && certutil -N -d sql:/root/.pki/nssdb --empty-password && certutil -A -d sql:/root/.pki/nssdb -n OpenIM38 -t "C,," -i /ca/ca.crt && touch /load/linux-prepare.ready'
