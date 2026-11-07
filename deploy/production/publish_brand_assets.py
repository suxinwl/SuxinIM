#!/usr/bin/env python3
"""Publish only the approved 28 public brand images; never write avatars.

Place the frozen publish_hrm_web.py helper beside this file.
"""
import argparse
import fcntl
import hashlib
import importlib.util
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
INSTANCE = '5ac7c644d9cd4923e2b701f66b52485d'
HELPER_SHA256 = 'bb7c96958559b3bc3a12cb2e60673db99ad6839f04c9e922b32c7eae12198a6a'
APPROVED_ZIP_SHA256 = 'b1e5654721d02bbb886962969620e9384d3b9381b82efa8784e0aa5cac0b62e8'
APPROVED_MANIFEST_SHA256 = 'c2176df6f162e7d0eaf54ae3215545bed6d5b7b05b9a42892e2b1372c2a5cef4'
PREFIXES = ('', 'hr/', 'im/', 'suxin/')
NAMES = ('logo.png', 'favicon.ico', 'favicon-16x16.png', 'favicon-32x32.png',
         'apple-touch-icon.png', 'android-chrome-192x192.png', 'android-chrome-512x512.png')
ALLOWED = frozenset(prefix + name for prefix in PREFIXES for name in NAMES)
MAX_BYTES = 32 << 20


def require(ok, message):
    if not ok:
        raise RuntimeError(message)


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


helper_path = pathlib.Path(__file__).resolve().with_name('publish_hrm_web.py')
require(helper_path.is_file() and not helper_path.is_symlink() and helper_path.resolve() == helper_path
        and helper_path.stat().st_nlink == 1 and sha(helper_path.read_bytes()) == HELPER_SHA256,
        'Frozen publication helper missing or changed')
_spec = importlib.util.spec_from_file_location('puty_frozen_publication_helpers', helper_path)
helpers = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(helpers)
regular = helpers.regular
atomic_write = helpers.atomic_write
sync_directory = helpers.sync_directory


def inspect_archive(archive, manifest_path, expected, expected_manifest):
    require(re.fullmatch(r'[0-9a-f]{64}', expected) and re.fullmatch(r'[0-9a-f]{64}', expected_manifest),
            'Exact brand digests required')
    require(regular(archive).stat().st_size <= MAX_BYTES and sha(archive.read_bytes()) == expected,
            'Brand archive digest or size mismatch')
    raw_manifest = regular(manifest_path).read_bytes()
    require(len(raw_manifest) <= 1 << 20 and sha(raw_manifest) == expected_manifest, 'Brand manifest digest mismatch')
    metadata = json.loads(raw_manifest)
    files = metadata.get('files')
    require(metadata.get('format') == 'puty-public-brand-assets-v1'
            and metadata.get('targetRoot') == 'resource/static/brand'
            and metadata.get('avatarExcluded') is True and metadata.get('fileCount') == 28
            and metadata.get('zipSHA256') == expected and isinstance(files, dict) and set(files) == ALLOWED,
            'Brand manifest must describe exactly 28 approved paths')
    for value in files.values():
        require(isinstance(value, dict) and set(value) == {'bytes', 'sha256'}
                and type(value['bytes']) is int and 0 < value['bytes'] <= 8 << 20
                and isinstance(value['sha256'], str) and re.fullmatch(r'[0-9a-f]{64}', value['sha256']),
                'Invalid brand member digest or size')
    payload = {}
    with zipfile.ZipFile(archive) as package:
        rows = package.infolist()
        require(len(rows) == 28 and len({row.filename for row in rows}) == 28
                and {row.filename for row in rows} == ALLOWED
                and sum(row.file_size for row in rows) <= MAX_BYTES, 'Brand ZIP file set or limits invalid')
        for row in rows:
            require(not row.is_dir() and not row.flag_bits & 1
                    and stat.S_IFMT(row.external_attr >> 16) in (0, stat.S_IFREG),
                    'Linked, special or encrypted brand file refused')
            require(row.file_size == files[row.filename]['bytes'], 'Brand member size mismatch')
            raw = package.read(row)
            require(len(raw) == row.file_size and sha(raw) == files[row.filename]['sha256'], 'Brand member digest mismatch')
            require(raw.startswith(b'\x00\x00\x01\x00') if row.filename.endswith('.ico')
                    else raw.startswith(b'\x89PNG\r\n\x1a\n'), 'Expected PNG or ICO image')
            payload[row.filename] = raw
    return payload


def avatar_snapshot(target):
    result = {}
    for prefix in PREFIXES:
        name = prefix + 'default-avatar.png'
        path = target / name
        require(path.resolve() == path and not path.is_symlink(), 'Avatar path redirected')
        result[name] = sha(regular(path).read_bytes()) if path.exists() else None
    require(result['default-avatar.png'] is not None, 'Existing default avatar required')
    return result


def publish(target, payload, backup, uid, gid, archive_sha, manifest_sha):
    require(set(payload) == ALLOWED and target.is_dir() and target.resolve() == target and not target.is_symlink(),
            'Brand destination or approved payload invalid')
    before_avatars = avatar_snapshot(target)
    previous, metadata, created = {}, {}, []
    created_directories = [prefix.rstrip('/') for prefix in PREFIXES if prefix and not (target / prefix).exists()]
    # Preflight every destination before creating a backup or replacing any file.
    for name in sorted(ALLOWED):
        path = target / name
        require(path.resolve() == path and path.parent.resolve() == path.parent and not path.is_symlink(),
                'Brand destination redirected')
        require(not path.parent.exists() or path.parent.is_dir(), 'Brand parent is not a directory')
        if path.exists():
            previous[name] = regular(path).read_bytes()
            old = path.stat()
            metadata[name] = (stat.S_IMODE(old.st_mode), old.st_uid, old.st_gid)
        else:
            created.append(name)
    require(backup.parent.is_dir() and backup.parent.resolve() == backup.parent, 'Brand backup root redirected')
    backup.mkdir(mode=0o700)
    sync_directory(backup.parent)
    for name, raw in previous.items():
        saved = backup / name
        saved.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
        atomic_write(saved, raw, 0o600)
    receipt = {'format': 'puty-public-brand-release-v1', 'status': 'prepared',
               'archiveSHA256': archive_sha, 'manifestSHA256': manifest_sha, 'fileCount': 28,
               'previousFiles': {name: sha(raw) for name, raw in previous.items()}, 'previousMetadata': metadata,
               'newFiles': {name: sha(raw) for name, raw in payload.items()}, 'createdFiles': created,
               'createdDirectories': created_directories,
               'protectedAvatarsBefore': before_avatars}
    receipt_path = backup / 'release.json'
    def save_receipt():
        atomic_write(receipt_path, (json.dumps(receipt, indent=2) + '\n').encode(), 0o600)
    save_receipt()
    changed = []
    try:
        for name in sorted(ALLOWED):
            path = target / name
            path.parent.mkdir(parents=True, exist_ok=True, mode=0o755)
            # Rename may succeed before directory fsync raises.
            changed.append(name)
            atomic_write(path, payload[name], 0o644, uid, gid)
        require(all(sha(regular(target / name).read_bytes()) == sha(raw) for name, raw in payload.items()),
                'Published brand verification failed')
        require(avatar_snapshot(target) == before_avatars, 'Protected avatar changed during brand publication')
        receipt.update(status='committed', protectedAvatarsAfter=before_avatars, defaultAvatarUnchanged=True)
        save_receipt()
    except BaseException:
        for name in changed:
            path = target / name
            if name in previous:
                atomic_write(path, previous[name], *metadata[name])
                require(sha(regular(path).read_bytes()) == sha(previous[name]), 'Brand rollback mismatch')
            elif path.exists():
                regular(path).unlink()
                sync_directory(path.parent)
        for name in created_directories:
            path = target / name
            require(path.resolve() == path and not path.is_symlink(), 'Brand rollback directory redirected')
            if path.exists():
                require(path.is_dir(), 'Brand rollback directory replaced')
            if path.is_dir() and not any(path.iterdir()):
                path.rmdir()
                sync_directory(target)
        # Protected avatar paths are only read, including in failure recovery.
        avatars_unchanged = avatar_snapshot(target) == before_avatars
        receipt.update(status='rolledBack', defaultAvatarUnchanged=avatars_unchanged)
        save_receipt()
        require(avatars_unchanged, 'External avatar change detected; avatar was not overwritten')
        raise
    return receipt


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--archive', required=True, type=pathlib.Path)
    parser.add_argument('--manifest', required=True, type=pathlib.Path)
    parser.add_argument('--sha256', required=True)
    parser.add_argument('--manifest-sha256', required=True)
    args = parser.parse_args()
    require(os.name == 'posix' and os.geteuid() == 0 and BASE.resolve() == BASE, 'Root-operated owned Linux suite required')
    require(args.sha256 == APPROVED_ZIP_SHA256 and args.manifest_sha256 == APPROVED_MANIFEST_SHA256,
            'This release accepts only the approved PUTY3 brand bundle')
    lock_path = regular(BASE / '.backup-stage/backup.lock')
    with lock_path.open('a+b') as lock:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        marker = json.loads(regular(BASE / 'openim/instance.json').read_text())
        activation = json.loads(regular(BASE / 'activation.json').read_text())
        require(marker.get('instanceID') == activation.get('instanceID') == INSTANCE
                and marker.get('production') is True and marker.get('testOnly') is False
                and marker.get('root') == str(BASE / 'openim') and marker.get('stackName') == 'puty-suite-prod',
                'Production ownership mismatch')
        require(args.archive.is_relative_to(BASE / 'staging') and args.manifest.is_relative_to(BASE / 'staging'),
                'Brand inputs must be staged inside the owned suite')
        payload = inspect_archive(args.archive, args.manifest, args.sha256, args.manifest_sha256)
        target = BASE / 'gosuxin/resource/static/brand'
        backups = BASE / 'backups'
        require(backups.is_dir() and backups.resolve() == backups and not backups.is_symlink(), 'Backup root redirected')
        backup = backups / ('public-brand-' + time.strftime('%Y%m%dT%H%M%SZ', time.gmtime()) + '-' + uuid.uuid4().hex)
        owner = pwd.getpwnam('puty')
        receipt = publish(target, payload, backup, owner.pw_uid, owner.pw_gid, args.sha256, args.manifest_sha256)
        print(json.dumps({'published': True, 'fileCount': 28, 'archiveSHA256': args.sha256, 'backup': str(backup),
                          'defaultAvatarBeforeSHA256': receipt['protectedAvatarsBefore']['default-avatar.png'],
                          'defaultAvatarAfterSHA256': receipt['protectedAvatarsAfter']['default-avatar.png'],
                          'defaultAvatarUnchanged': True, 'serviceRestarted': False}))


if __name__ == '__main__':
    main()
