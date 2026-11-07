"""Verify published packages and entry-aware updates with normal TLS validation.

Both SNI profiles connect to the LAN server. This does not prove WAN NAT.
Full package responses are hashed in memory without saving a second copy.
"""
import argparse
import hashlib
import http.client
import json
import pathlib
import socket
import ssl

ROOT = pathlib.Path(__file__).resolve().parents[1]
MANIFEST = ROOT / 'clients/artifacts-manifest.json'
CA = ROOT / 'deploy/certificates/Puty-Enterprise-Root-CA.crt'


def validate_latest(body, artifact, host, require_force=False):
    assert isinstance(body, dict) and body.get('errCode') == 0
    latest = body['data']
    assert latest.get('available') is True and latest['url'] == 'https://' + host + ':9885/downloads/' + artifact['fileName']
    assert all(latest.get(key) == artifact[key] for key in ('sha256', 'size', 'build', 'version', 'platform'))
    assert type(latest.get('force')) is bool and latest['force'] is bool(require_force)
    return latest


def validate_current(body):
    assert isinstance(body, dict) and body.get('errCode') == 0 and isinstance(body.get('data'), dict)
    assert body['data'].get('available') is False


class Connection(http.client.HTTPSConnection):
    def connect(self):
        raw = socket.create_connection(('192.168.10.11', self.port), self.timeout)
        self.sock = self._context.wrap_socket(raw, server_hostname=self.host)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--manifest', type=pathlib.Path, default=MANIFEST)
    parser.add_argument('--manifest-sha256')
    parser.add_argument('--require-force-upgrade', action='store_true')
    parser.add_argument('--output', type=pathlib.Path, default=ROOT / '.private/production/acceptance/downloads-check.json')
    args = parser.parse_args()
    manifest = args.manifest.resolve(strict=True)
    output = args.output.absolute()
    assert manifest.is_relative_to(ROOT) and manifest == args.manifest.absolute() and not manifest.is_symlink()
    assert output.resolve() == output and output.is_relative_to(ROOT / '.private/production/acceptance') and not output.exists()
    manifest_raw = manifest.read_bytes()
    manifest_sha = hashlib.sha256(manifest_raw).hexdigest()
    assert args.manifest_sha256 is None or manifest_sha == args.manifest_sha256
    artifacts = [x for x in json.loads(manifest_raw)['artifacts']
                 if x['platform'] in ('windows', 'android')]
    assert {x['platform'] for x in artifacts} == {'windows', 'android'}
    checks = []
    for host in ('192.168.10.11', 'im.szputy.com'):
        context = ssl.create_default_context(cafile=str(CA)) if host[0].isdigit() else ssl.create_default_context()
        def request(method, path, body=None, headers=None):
            conn = Connection(host, 9885, timeout=90, context=context)
            conn.request(method, path, body, headers or {})
            return conn, conn.getresponse()
        conn, response = request('GET', '/downloads/')
        page = response.read().decode()
        assert response.status == 200 and all(x['fileName'] in page for x in artifacts)
        conn.close()
        checks.append({'profile': host, 'check': 'download_page', 'passed': True})
        for artifact in artifacts:
            path = '/downloads/' + artifact['fileName']
            conn, response = request('HEAD', path)
            assert response.status == 200 and int(response.getheader('Content-Length')) == artifact['size']
            if artifact['platform'] == 'android':
                assert response.getheader('Content-Type') == 'application/vnd.android.package-archive'
                assert response.getheader('Content-Disposition') == 'attachment; filename="' + artifact['fileName'] + '"'
            conn.close()
            conn, response = request('GET', path, headers={'Range': 'bytes=2-9', 'Accept-Encoding': 'identity'})
            raw = response.read()
            assert response.status == 206 and len(raw) == 8 and response.getheader('Content-Range') == 'bytes 2-9/' + str(artifact['size'])
            if artifact['platform'] == 'android':
                assert response.getheader('Content-Type') == 'application/vnd.android.package-archive'
                assert response.getheader('Content-Disposition') == 'attachment; filename="' + artifact['fileName'] + '"'
            conn.close()
            if host[0].isdigit():
                conn, response = request('GET', path)
                assert response.status == 200
                digest = hashlib.sha256()
                size = 0
                while block := response.read(1024 * 1024):
                    digest.update(block)
                    size += len(block)
                assert size == artifact['size'] and digest.hexdigest() == artifact['sha256']
                conn.close()
            body = json.dumps({'platform': artifact['platform'], 'version': '0.0.0', 'build': 0})
            conn, response = request('POST', '/enterprise/v1/application/latest', body,
                                     {'Content-Type': 'application/json'})
            data = json.loads(response.read())
            assert response.status == 200
            validate_latest(data, artifact, host, args.require_force_upgrade)
            conn.close()
            body = json.dumps({key: artifact[key] for key in ('platform', 'version', 'build')})
            conn, response = request('POST', '/enterprise/v1/application/latest', body,
                                     {'Content-Type': 'application/json'})
            assert response.status == 200
            validate_current(json.loads(response.read()))
            conn.close()
            checks.append({'profile': host, 'platform': artifact['platform'],
                           'check': 'HEAD_Range_update_metadata' + ('_full_SHA256' if host[0].isdigit() else ''),
                           'currentVersionNotOffered': True, 'mandatoryUpgradeVerified': args.require_force_upgrade,
                           'passed': True})
    assert hashlib.sha256(manifest.read_bytes()).hexdigest() == manifest_sha
    report = {'passed': True, 'connectedIP': '192.168.10.11', 'WAN_NAT_verified': False,
              'sourceManifestSHA256': manifest_sha, 'mandatoryUpgradeRequired': args.require_force_upgrade, 'checks': checks}
    output.parent.mkdir(parents=True, exist_ok=True)
    with output.open('x', encoding='utf-8') as target:
        target.write(json.dumps(report, indent=2) + '\n')
    print(json.dumps(report))


if __name__ == '__main__':
    main()
