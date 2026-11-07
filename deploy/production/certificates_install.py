#!/usr/bin/env python3
"""Install only a verified PUTY certificate bundle, with guarded rollback.

Linux stdlib + system OpenSSL only. No CA signing key is accepted or uploaded.
"""
import argparse
import datetime
try:
    import fcntl
    import grp
except ImportError:  # Keep pure verification helpers testable on Windows.
    fcntl = grp = None
import hashlib
import json
import os
import pathlib
import re
import secrets
import shutil
import signal
import subprocess
import tarfile
import tempfile

BASE = pathlib.Path('/srv/puty-suite')
IM = BASE / 'openim'
CERTS = IM / 'certs'
PROJECT = 'puty-suite-prod'
PINNED_ROOT_SHA256 = '836c3d6c28d5078cc8da6ebeff8e5f9a7fb7f140554e6a7834011d72299c3d69'
FORMAT = 'puty-production-certificates-v1'
FILES = {'hr.crt', 'hr.key', 'im.crt', 'im.key', 'lan.crt', 'lan.key', 'puty-root.crt'}
NGINX = '/www/server/nginx/sbin/nginx'
OPENSSL = '/usr/bin/openssl'


def require(condition, message):
    if not condition:
        raise RuntimeError(message)


def run(args, source=None, timeout=60):
    result = subprocess.run(args, input=source, capture_output=True, timeout=timeout)
    require(result.returncode == 0, 'Certificate operation failed; subprocess output withheld')
    return result.stdout


def sha(path):
    digest = hashlib.sha256()
    with path.open('rb') as source:
        for block in iter(lambda: source.read(1024 * 1024), b''):
            digest.update(block)
    return digest.hexdigest()


def guard():
    require(os.geteuid() == 0, 'Certificate installer must run as root')
    require(BASE.resolve() == BASE and IM.resolve() == IM and CERTS.resolve() == CERTS,
            'Production certificate root missing or redirected')
    marker = IM / 'instance.json'
    require(marker.is_file() and not marker.is_symlink(), 'Missing regular production owner marker')
    info = json.loads(marker.read_text())
    require(info.get('root') == str(IM) and info.get('production') is True and info.get('testOnly') is False
            and info.get('stackName') == PROJECT and re.fullmatch('[a-f0-9]{32}', info.get('instanceID', '')),
            'Production owner marker mismatch')
    require(CERTS.is_dir(), 'Production certificate directory is absent')
    for name in FILES:
        require((CERTS / name).is_file() and not (CERTS / name).is_symlink(), 'Existing certificate/key must be a regular file')
    root_fingerprint = fingerprint(CERTS / 'puty-root.crt')
    require(root_fingerprint == PINNED_ROOT_SHA256, 'Existing pinned root CA differs; refusing any update')
    cli = ['docker', 'compose', '--project-directory', str(IM), '-f', str(IM / 'compose.yaml'), '-p', PROJECT]
    ids = run(cli + ['ps', '-a', '-q', 'livekit']).decode().split()
    require(len(ids) == 1, 'Expected one existing production LiveKit container')
    item = json.loads(run(['docker', 'inspect', ids[0]]))[0]
    labels = item['Config'].get('Labels') or {}
    require(labels.get('com.puty.suite') == info['instanceID'] and labels.get('com.docker.compose.project') == PROJECT
            and labels.get('com.docker.compose.service') == 'livekit', 'Foreign LiveKit container refused')
    return info, ids[0], item['State']['Running']


def fingerprint(path):
    text = run([OPENSSL, 'x509', '-in', str(path), '-noout', '-fingerprint', '-sha256']).decode().strip()
    require('=' in text, 'Invalid certificate fingerprint output')
    return text.split('=', 1)[1].replace(':', '').lower()


def extract_bundle(bundle, expected_sha, destination):
    require(bundle.is_file() and not bundle.is_symlink() and bundle.resolve().is_relative_to((BASE / 'staging/certificates').resolve()),
            'Bundle must be a regular file inside owned certificate staging')
    require(bundle.stat().st_size <= 1024 * 1024 and sha(bundle) == expected_sha, 'Certificate bundle hash/size mismatch')
    seen = set()
    with tarfile.open(bundle, 'r:gz') as archive:
        for member in archive:
            require(member.name in FILES | {'manifest.json'} and member.isfile()
                    and member.name not in seen and 0 < member.size <= 128 * 1024,
                    'Unexpected, linked, duplicate or oversized bundle member')
            seen.add(member.name)
            target = destination / member.name
            with target.open('xb') as output:
                shutil.copyfileobj(archive.extractfile(member), output)
            target.chmod(0o600)
    require(seen == FILES | {'manifest.json'}, 'Incomplete certificate bundle')
    manifest = json.loads((destination / 'manifest.json').read_text())
    require(manifest.get('format') == FORMAT and manifest.get('pinnedRootSHA256') == PINNED_ROOT_SHA256
            and manifest.get('rootPrivateKeyIncluded') is False and manifest.get('rootReplaced') is False,
            'Wrong certificate bundle format or root identity')
    require(re.fullmatch('[0-9]{8}T[0-9]{6}Z-[a-f0-9]{16}', manifest.get('bundleID', '')), 'Invalid bundle identity')
    rows = manifest.get('files', [])
    require(len(rows) == len(FILES) and {r.get('name') for r in rows} == FILES, 'Manifest file set mismatch')
    for row in rows:
        path = destination / row['name']
        require(path.stat().st_size == row['bytes'] and sha(path) == row['sha256'], 'Certificate member checksum mismatch')
    return manifest


def split_chain(path, folder, prefix):
    raw = path.read_bytes()
    blocks = re.findall(rb'-----BEGIN CERTIFICATE-----\s+.*?-----END CERTIFICATE-----\s*', raw, re.S)
    require(blocks and not re.sub(rb'-----BEGIN CERTIFICATE-----\s+.*?-----END CERTIFICATE-----\s*', b'', raw, flags=re.S).strip(),
            'Malformed PEM fullchain')
    paths = []
    for index, data in enumerate(blocks):
        target = folder / (prefix + '-' + str(index) + '.pem')
        target.write_bytes(data)
        target.chmod(0o600)
        paths.append(target)
    return paths


def validate(stage):
    require(fingerprint(stage / 'puty-root.crt') == PINNED_ROOT_SHA256, 'Staged root CA does not match its fixed pin')
    run([OPENSSL, 'verify', '-check_ss_sig', '-CAfile', str(stage / 'puty-root.crt'), str(stage / 'puty-root.crt')])
    with tempfile.TemporaryDirectory(prefix='verify-', dir=stage) as folder:
        temporary = pathlib.Path(folder)
        root_public = run([OPENSSL, 'x509', '-in', str(stage / 'puty-root.crt'), '-pubkey', '-noout'])
        root_der = run([OPENSSL, 'pkey', '-pubin', '-outform', 'DER'], root_public)
        for prefix, hostname in (('hr', 'hr.szputy.com'), ('im', 'im.szputy.com'), ('lan', '192.168.10.11')):
            chain = split_chain(stage / (prefix + '.crt'), temporary, prefix)
            cert_public = run([OPENSSL, 'x509', '-in', str(chain[0]), '-pubkey', '-noout'])
            cert_der = run([OPENSSL, 'pkey', '-pubin', '-outform', 'DER'], cert_public)
            key_der = run([OPENSSL, 'pkey', '-in', str(stage / (prefix + '.key')), '-pubout', '-outform', 'DER'])
            require(cert_der == key_der, 'Staged certificate and key do not match')
            require(key_der != root_der, 'Root signing key cannot be installed as a leaf key')
            extensions = run([OPENSSL, 'x509', '-in', str(chain[0]), '-noout', '-ext', 'extendedKeyUsage']).decode()
            require('TLS Web Server Authentication' in extensions or '1.3.6.1.5.5.7.3.1' in extensions,
                    'Staged leaf lacks ServerAuth EKU')
            for path in chain:
                run([OPENSSL, 'x509', '-in', str(path), '-checkend', '0', '-noout'])
            untrusted = temporary / (prefix + '-issuers.pem')
            untrusted.write_bytes(b''.join(p.read_bytes() for p in chain[1:]))
            authority = stage / 'puty-root.crt' if prefix == 'lan' else pathlib.Path('/etc/ssl/certs/ca-certificates.crt')
            args = [OPENSSL, 'verify', '-purpose', 'sslserver', '-verify_ip' if prefix == 'lan' else '-verify_hostname', hostname,
                    '-CAfile', str(authority)]
            if len(chain) > 1:
                args += ['-untrusted', str(untrusted)]
            args += [str(chain[0])]
            run(args)
            if prefix == 'lan':
                require(len(chain) == 2 and fingerprint(chain[1]) == PINNED_ROOT_SHA256, 'LAN chain must end at its pinned root')
                san = run([OPENSSL, 'x509', '-in', str(chain[0]), '-noout', '-ext', 'subjectAltName']).decode()
                require('IP Address:192.168.10.11' in san and 'DNS:' not in san
                        and san.count('IP Address:') == 1, 'Unexpected LAN SAN')


def install_file(source, destination, group_id):
    require(destination.parent == CERTS and destination.name in FILES - {'puty-root.crt'}
            and not destination.is_symlink(), 'Certificate install target refused')
    temporary = CERTS / ('.certificate-' + secrets.token_hex(12))
    try:
        with source.open('rb') as source_stream, temporary.open('xb') as output:
            shutil.copyfileobj(source_stream, output)
            output.flush()
            os.fsync(output.fileno())
        os.chown(temporary, 0, group_id)
        temporary.chmod(0o640 if destination.suffix == '.key' else 0o644)
        os.replace(temporary, destination)
    finally:
        if temporary.exists():
            temporary.unlink()


def apply_bundle(bundle, expected_sha):
    info, livekit_id, livekit_running = guard()
    group_id = grp.getgrnam('www').gr_gid
    backups = BASE / '.certificate-backups'
    require(not backups.is_symlink(), 'Linked certificate backup directory refused')
    backups.mkdir(mode=0o700, exist_ok=True)
    backups.chmod(0o700)
    with (backups / 'update.lock').open('a+b') as lock:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        with tempfile.TemporaryDirectory(prefix='incoming-', dir=backups) as folder:
            stage = pathlib.Path(folder)
            manifest = extract_bundle(bundle, expected_sha, stage)
            validate(stage)
            run([NGINX, '-t'])  # Require a healthy baseline before making a change.
            backup = backups / manifest['bundleID']
            backup.mkdir(mode=0o700, exist_ok=False)
            previous = []
            for name in sorted(FILES):
                source = CERTS / name
                shutil.copy2(source, backup / name)
                (backup / name).chmod(0o600)
                previous.append({'name': name, 'sha256': sha(backup / name)})
            journal = {'format': FORMAT, 'instanceID': info['instanceID'], 'bundleID': manifest['bundleID'],
                       'previous': previous, 'bundleSHA256': expected_sha, 'state': 'backed-up'}
            (backup / 'journal.json').write_text(json.dumps(journal) + '\n')
            restarted = False
            try:
                for name in sorted(FILES - {'puty-root.crt'}):
                    install_file(stage / name, CERTS / name, group_id)
                run([NGINX, '-t'])
                run([NGINX, '-s', 'reload'])
                if livekit_running:
                    restarted = True
                    run(['docker', 'restart', '--time', '30', livekit_id], timeout=90)
                    item = json.loads(run(['docker', 'inspect', livekit_id]))[0]
                    require(item['State']['Running'], 'LiveKit exited after certificate reload')
                require(fingerprint(CERTS / 'puty-root.crt') == PINNED_ROOT_SHA256, 'Pinned root unexpectedly changed')
                journal['state'] = 'installed'
                (backup / 'journal.json').write_text(json.dumps(journal) + '\n')
            except Exception:
                for name in sorted(FILES - {'puty-root.crt'}):
                    install_file(backup / name, CERTS / name, group_id)
                run([NGINX, '-t'])
                run([NGINX, '-s', 'reload'])
                if restarted:
                    run(['docker', 'restart', '--time', '30', livekit_id], timeout=90)
                journal['state'] = 'rolled-back'
                (backup / 'journal.json').write_text(json.dumps(journal) + '\n')
                raise RuntimeError('Certificate installation failed; previous certificates restored and Nginx reloaded')
            print(json.dumps({'installed': True, 'bundleID': manifest['bundleID'], 'backup': str(backup),
                              'nginxValidatedAndReloaded': True, 'livekitRestarted': livekit_running,
                              'otherContainersRestarted': False, 'rootCAChanged': False}))


def main():
    require(os.name == 'posix' and fcntl is not None and grp is not None, 'Certificate installer requires Linux')
    os.umask(0o077)
    def interrupted(_signal, _frame):
        raise RuntimeError('Certificate installation interrupted; reverting any applied files')
    for signum in (signal.SIGTERM, signal.SIGINT, signal.SIGHUP):
        signal.signal(signum, interrupted)
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--bundle', type=pathlib.Path, required=True)
    parser.add_argument('--sha256', required=True)
    args = parser.parse_args()
    require(re.fullmatch('[a-f0-9]{64}', args.sha256), 'Invalid expected bundle digest')
    guard()  # Authenticate ownership before creating the shared maintenance lock.
    maintenance = BASE / '.backup-stage'
    require(not maintenance.is_symlink(), 'Linked maintenance directory refused')
    maintenance.mkdir(mode=0o700, exist_ok=True)
    maintenance.chmod(0o700)
    with (maintenance / 'backup.lock').open('a+b') as lock:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        apply_bundle(args.bundle, args.sha256)


if __name__ == '__main__':
    try:
        main()
    except Exception as error:
        raise SystemExit(type(error).__name__ + ': ' + str(error))
