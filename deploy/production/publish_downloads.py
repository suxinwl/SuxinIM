#!/usr/bin/env python3
"""Publish two native packages plus three metadata files, with no version API.
Place frozen publish_hrm_web.py beside this file. Retain unrelated older packages.
"""
import argparse
import fcntl
import hashlib
import importlib.util
import json
import os
import pathlib
import re
import stat
import time
import uuid

BASE = pathlib.Path('/srv/puty-suite')
INSTANCE = '5ac7c644d9cd4923e2b701f66b52485d'
HELPER_SHA256 = 'bb7c96958559b3bc3a12cb2e60673db99ad6839f04c9e922b32c7eae12198a6a'
ROOT_SHA = '836c3d6c28d5078cc8da6ebeff8e5f9a7fb7f140554e6a7834011d72299c3d69'
STATIC = frozenset(('index.html', 'artifacts-manifest.json', 'sha256sums.txt'))

def require(ok, message):
    if not ok:
        raise RuntimeError(message)

def sha(raw):
    return hashlib.sha256(raw).hexdigest()

helper_path = pathlib.Path(__file__).resolve().with_name('publish_hrm_web.py')
require(helper_path.is_file() and not helper_path.is_symlink() and helper_path.resolve() == helper_path
        and helper_path.stat().st_nlink == 1 and sha(helper_path.read_bytes()) == HELPER_SHA256,
        'Frozen publication helper missing or changed')
_spec = importlib.util.spec_from_file_location('puty_download_publication_helpers', helper_path)
helpers = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(helpers)
regular = helpers.regular

def digest(path):
    with regular(path).open('rb') as source:
        return hashlib.file_digest(source, 'sha256').hexdigest()

def native_name(name):
    return isinstance(name, str) and re.fullmatch(r'[A-Za-z0-9][A-Za-z0-9._-]{0,179}', name) and name.endswith(('.exe', '.apk'))

def owned_directory(path):
    require(path.is_dir() and path.resolve() == path and not path.is_symlink(), 'Owned directory redirected or missing')
    if os.name == 'posix':
        info = path.stat()
        require(info.st_uid == info.st_gid == 0 and info.st_mode & 0o022 == 0, 'Root-owned nonwritable directory required')

def inspect_staged(stage, publish_path, expected):
    require(stage.resolve() == stage and publish_path.is_relative_to(stage), 'Publication inputs outside staging')
    raw = regular(publish_path).read_bytes()
    require(len(raw) <= 1 << 20 and isinstance(expected, str) and re.fullmatch(r'[0-9a-f]{64}', expected)
            and sha(raw) == expected, 'Exact publish JSON digest required')
    value = json.loads(raw)
    require(isinstance(value, dict) and set(value) == {'files'} and isinstance(value['files'], list)
            and len(value['files']) == 5, 'Exactly five publication files required')
    files = {}
    for item in value['files']:
        require(isinstance(item, dict) and set(item) == {'name', 'bytes', 'sha256'}, 'Invalid publish entry')
        name = item['name']
        require(isinstance(name, str) and (name in STATIC or native_name(name))
                and name.casefold() not in {n.casefold() for n in files}, 'Unsafe or duplicate flat download name')
        limit = 300 << 20 if native_name(name) else 2 << 20
        require(type(item['bytes']) is int and 0 < item['bytes'] < limit
                and isinstance(item['sha256'], str) and re.fullmatch(r'[0-9a-f]{64}', item['sha256']),
                'Invalid download size or digest')
        source = regular(stage / name)
        require(source.stat().st_size == item['bytes'] and digest(source) == item['sha256'], 'Staged download digest mismatch')
        files[name] = item
    native = set(files) - STATIC
    require(STATIC.issubset(files) and len(native) == 2
            and sum(n.endswith('.exe') for n in native) == sum(n.endswith('.apk') for n in native) == 1,
            'Two native packages and three exact metadata names required')
    payload = {name: regular(stage / name).read_bytes() for name in files}
    require(all(len(payload[n]) == row['bytes'] and sha(payload[n]) == row['sha256'] for n, row in files.items()),
            'Staged download changed while reading')
    metadata = json.loads(payload['artifacts-manifest.json'])
    require(metadata.get('enterpriseRoot', {}).get('derSHA256') == ROOT_SHA, 'Public root identity mismatch')
    entries = [entry for entry in metadata.get('artifacts', []) if entry.get('platform') in ('windows', 'android')]
    require(len(entries) == 2 and {entry['platform'] for entry in entries} == {'windows', 'android'},
            'Public manifest needs one native artifact per platform')
    entries.sort(key=lambda entry: entry['platform'] == 'android')
    require({entry.get('fileName') for entry in entries} == native, 'Public manifest native filenames mismatch')
    for entry in entries:
        row = files[entry['fileName']]
        require(entry.get('sha256') == row['sha256'] and type(entry.get('size')) is int
                and entry['size'] == row['bytes'] and entry['fileName'].endswith('.exe' if entry['platform'] == 'windows' else '.apk'),
                'Public manifest native digest or size mismatch')
        require(('href="' + entry['fileName'] + '"').encode() in payload['index.html'], 'Download page package link missing')
    sums = ''.join(entry['sha256'] + '  ' + entry['fileName'] + '\n' for entry in entries).encode()
    require(payload['sha256sums.txt'] == sums, 'Exact native SHA256 checksum list required')
    require(digest(publish_path) == expected, 'Publish JSON changed during validation')
    return payload

def publish(target, payload, backup, expected_index, expected_artifacts, publish_sha):
    owned_directory(target)
    owned_directory(backup.parent)
    require(sha(regular(target / 'index.html').read_bytes()) == expected_index
            and sha(regular(target / 'artifacts-manifest.json').read_bytes()) == expected_artifacts,
            'Current download index or public artifacts changed')
    require(len(payload) == 5 and STATIC.issubset(payload)
            and all(name in STATIC or native_name(name) for name in payload), 'Exact validated download payload required')
    for name in set(payload) - STATIC:
        destination = target / name
        if destination.exists():
            require(sha(regular(destination).read_bytes()) == sha(payload[name]), 'Published native package is immutable')
    # Frozen helper preflights every destination before writing, restores prior
    # mode/uid/gid on failure, retains unrelated packages and writes HTML last.
    return helpers.publish(target, payload, backup, 0, 0,
                           {'format': 'puty-downloads-release-v1', 'publishJSONSHA256': publish_sha,
                            'expectedIndexSHA256': expected_index, 'expectedArtifactsSHA256': expected_artifacts,
                            'newFilesOwner': 'root:root', 'newFilesMode': '0644', 'olderPackagesRetained': True})

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--publish', type=pathlib.Path, default=BASE / 'staging/downloads-publish.json')
    parser.add_argument('--publish-sha256', required=True)
    parser.add_argument('--expected-index', required=True)
    parser.add_argument('--expected-artifacts', required=True)
    args = parser.parse_args()
    require(os.name == 'posix' and os.geteuid() == 0 and BASE.resolve() == BASE, 'Root-operated owned Linux suite required')
    require(all(re.fullmatch(r'[0-9a-f]{64}', value) for value in (args.publish_sha256, args.expected_index, args.expected_artifacts)),
            'Exact publication and old metadata SHA256 guards required')
    lock_path = regular(BASE / '.backup-stage/backup.lock')
    with lock_path.open('a+b') as lock:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        marker = json.loads(regular(BASE / 'openim/instance.json').read_text())
        activation = json.loads(regular(BASE / 'activation.json').read_text())
        require(marker.get('instanceID') == activation.get('instanceID') == INSTANCE
                and marker.get('production') is True and marker.get('testOnly') is False
                and marker.get('root') == str(BASE / 'openim') and marker.get('stackName') == 'puty-suite-prod',
                'Production ownership mismatch')
        stage = BASE / 'staging'
        owned_directory(stage)
        payload = inspect_staged(stage, args.publish, args.publish_sha256)
        target, backups = BASE / 'openim/www/downloads', BASE / 'backups'
        owned_directory(backups)
        backup = backups / ('downloads-' + time.strftime('%Y%m%dT%H%M%SZ', time.gmtime()) + '-' + uuid.uuid4().hex)
        publish(target, payload, backup, args.expected_index, args.expected_artifacts, args.publish_sha256)
        print(json.dumps({'downloadsPublished': True, 'allSHA256Verified': True, 'fileCount': 5, 'packages': 2,
                          'backup': str(backup), 'newFilesOwner': 'root:root', 'newFilesMode': '0644',
                          'olderPackagesRetained': True, 'serviceRestarted': False, 'versionAPIChanged': False}))

if __name__ == '__main__':
    main()
