#!/usr/bin/env python3
"""Publish a verified frontend under the suite lock; restore a failed delta."""
import argparse
import fcntl
import hashlib
import json
import os
import pathlib
import pwd
import re
import stat
import time
import uuid
import zipfile

BASE = pathlib.Path('/srv/puty-suite')


def require(ok, message):
    if not ok:
        raise RuntimeError(message)


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def regular(path):
    require(path.resolve() == path and path.is_file() and not path.is_symlink()
            and stat.S_ISREG(path.stat().st_mode) and path.stat().st_nlink == 1,
            'Regular canonical file required')
    return path


def sync_directory(path):
    if os.name == 'posix':
        fd = os.open(path, os.O_RDONLY | os.O_DIRECTORY)
        try:
            os.fsync(fd)
        finally:
            os.close(fd)


def atomic_write(path, raw, mode, uid=None, gid=None):
    require(path.parent.resolve() == path.parent and not path.is_symlink(), 'Publish path redirected')
    temporary = path.with_name('.' + path.name + '.publish-' + uuid.uuid4().hex)
    try:
        with temporary.open('xb') as handle:
            handle.write(raw)
            handle.flush()
            os.fchmod(handle.fileno(), mode)
            if uid is not None:
                os.fchown(handle.fileno(), uid, gid)
            os.fsync(handle.fileno())
        temporary.replace(path)
        sync_directory(path.parent)
    finally:
        if temporary.exists():
            temporary.unlink()


def inspect_archive(archive, expected, index_expected, application):
    require(re.fullmatch(r'[0-9a-f]{64}', expected) and re.fullmatch(r'[0-9a-f]{64}', index_expected),
            'Exact archive and index SHA256 required')
    require(regular(archive).stat().st_size < 100 << 20 and sha(archive.read_bytes()) == expected,
            'Archive digest or size mismatch')
    payload, seen = {}, set()
    with zipfile.ZipFile(archive) as package:
        members = package.infolist()
        require(1 < len(members) < 5000 and sum(m.file_size for m in members) < 100 << 20,
                'Archive limits exceeded')
        for member in members:
            name = member.filename[:-1] if member.is_dir() else member.filename
            path = pathlib.PurePosixPath(name)
            require(bool(name) and path.as_posix() == name and not path.is_absolute()
                    and all(p not in ('.', '..') and ':' not in p and not any(ord(c) < 32 for c in p) for p in path.parts)
                    and '\\' not in name and name.casefold() not in seen,
                    'Unsafe or duplicate archive path')
            seen.add(name.casefold())
            require(application != 'im' or path.parts[0] not in ('admin', 'downloads'), 'Reserved IM path')
            mode = stat.S_IFMT(member.external_attr >> 16)
            require(mode in (0, stat.S_IFREG, stat.S_IFDIR) and not member.flag_bits & 1,
                    'Linked, special or encrypted archive entry')
            if not member.is_dir():
                raw = package.read(member)
                require(len(raw) == member.file_size, 'Incomplete archive member')
                payload[name] = raw
    require('index.html' in payload and sha(payload['index.html']) == index_expected, 'Index digest mismatch')
    prefix = '/suxinweb/' if application == 'hrm' else '/'
    for value in re.findall(r'(?:src|href)=["\']([^"\'?#]+)', payload['index.html'].decode('utf-8')):
        if value.startswith(prefix) and not value.startswith('//'):
            dependency = value[len(prefix):]
            if dependency.startswith('assets/'):
                require(dependency in payload, 'Missing HTML dependency')
    return payload


def publish(target, payload, backup, uid, gid, release_metadata=None):
    require(target.is_dir() and target.resolve() == target, 'Frontend root redirected')
    previous, metadata, created = {}, {}, []
    for name in payload:
        path = target / name
        require(path.resolve() == path and path.parent.resolve() == path.parent and not path.is_symlink(),
                'Frontend destination redirected')
        if path.exists():
            old = regular(path).read_bytes()
            previous[name] = old
            value = path.stat()
            metadata[name] = (stat.S_IMODE(value.st_mode), value.st_uid, value.st_gid)
        else:
            created.append(name)
    backup.mkdir(mode=0o700)
    sync_directory(backup.parent)
    for name, raw in previous.items():
        saved = backup / name
        saved.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
        atomic_write(saved, raw, 0o600)
    receipt = {'format': 'puty-static-release-v2', 'previousFiles': {n: sha(r) for n, r in previous.items()},
               'newFiles': {n: sha(r) for n, r in payload.items()}, 'createdFiles': created,
               'previousMetadata': metadata, **(release_metadata or {})}
    atomic_write(backup / 'release.json', (json.dumps(receipt, indent=2) + '\n').encode(), 0o600)
    order = [n for n in payload if n != 'index.html'] + ['index.html']
    changed = []
    try:
        for name in order:
            destination = target / name
            destination.parent.mkdir(parents=True, exist_ok=True, mode=0o755)
            # Register before replace: fsync failure can occur after rename.
            changed.append(name)
            atomic_write(destination, payload[name], 0o644, uid, gid)
        require(all(sha(regular(target / n).read_bytes()) == sha(r) for n, r in payload.items()),
                'Published file verification failed')
    except BaseException:
        for name in [n for n in changed if n != 'index.html'] + (['index.html'] if 'index.html' in changed else []):
            destination = target / name
            if name in previous:
                atomic_write(destination, previous[name], *metadata[name])
                require(sha(regular(destination).read_bytes()) == sha(previous[name]), 'Frontend rollback mismatch')
            elif destination.exists():
                regular(destination).unlink()
                sync_directory(destination.parent)
        raise
    return receipt


def main():
    parser = argparse.ArgumentParser()
    for name in ('archive', 'sha256', 'index-sha256', 'expected-index'):
        parser.add_argument('--' + name, required=True)
    parser.add_argument('--application', choices=['hrm', 'im'], default='hrm')
    args = parser.parse_args()
    require(BASE.resolve() == BASE, 'Production root redirected')
    lock_path = regular(BASE / '.backup-stage/backup.lock')
    with lock_path.open('a+b') as lock:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        marker = json.loads(regular(BASE / 'openim/instance.json').read_text())
        activation = json.loads(regular(BASE / 'activation.json').read_text())
        require(marker['instanceID'] == activation['instanceID'] == '5ac7c644d9cd4923e2b701f66b52485d'
                and marker['production'] is True and marker['testOnly'] is False
                and marker['root'] == str(BASE / 'openim') and marker['stackName'] == 'puty-suite-prod',
                'Production ownership mismatch')
        target = BASE / ('gosuxin/resource/suxinweb' if args.application == 'hrm' else 'openim/www')
        require(sha(regular(target / 'index.html').read_bytes()) == args.expected_index, 'Current index changed')
        archive = pathlib.Path(args.archive)
        require(archive.is_relative_to(BASE / 'staging'), 'Archive outside staging')
        payload = inspect_archive(archive, args.sha256, args.index_sha256, args.application)
        backups = BASE / 'backups'
        require(backups.is_dir() and backups.resolve() == backups, 'Backup root redirected')
        backup = backups / (args.application + '-web-' + time.strftime('%Y%m%dT%H%M%SZ', time.gmtime()) + '-' + uuid.uuid4().hex)
        owner = pwd.getpwnam('puty')
        publish(target, payload, backup, owner.pw_uid, owner.pw_gid,
                {'archiveSHA256': args.sha256, 'indexSHA256': args.index_sha256, 'application': args.application})
        print(json.dumps({'published': True, 'fileCount': len(payload), 'backup': str(backup),
                          'application': args.application, 'indexSHA256': args.index_sha256, 'serviceRestarted': False}))


if __name__ == '__main__':
    main()
