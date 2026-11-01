#!/usr/bin/env bash
set -euo pipefail
cd /srv/openim38-upgrade
test "$(pwd -P)" = /srv/openim38-upgrade
umask 077
mkdir -p certs
if [[ ! -f certs/ca.key ]]; then
 openssl genrsa -out certs/ca.key 3072
 openssl req -x509 -new -sha256 -days 365 -key certs/ca.key -out certs/ca.crt -subj '/CN=OpenIM38 Isolated Development CA/O=OpenIM38 Test Only' -addext 'basicConstraints=critical,CA:TRUE' -addext 'keyUsage=critical,keyCertSign,cRLSign'
fi
if [[ ! -f certs/server.key ]]; then
 openssl genrsa -out certs/server.key 3072
 openssl req -new -key certs/server.key -out certs/server.csr -subj '/CN=192.168.10.11/O=OpenIM38 Test Only'
 printf '%s\n' 'subjectAltName=IP:192.168.10.11,DNS:openim38.test.local' 'basicConstraints=critical,CA:FALSE' 'keyUsage=critical,digitalSignature,keyEncipherment' 'extendedKeyUsage=serverAuth' > certs/server.ext
 openssl x509 -req -sha256 -days 90 -in certs/server.csr -CA certs/ca.crt -CAkey certs/ca.key -CAcreateserial -out certs/server.crt -extfile certs/server.ext
fi
openssl verify -CAfile certs/ca.crt -verify_ip 192.168.10.11 certs/server.crt
chmod 600 certs/*.key
