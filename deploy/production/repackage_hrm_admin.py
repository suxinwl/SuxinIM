#!/usr/bin/env python3
"""Plan or build a complete immutable biz-suite package in staging only.

The active package, binary, active.json and application data are never changed.
The build action requires the exact SHA256 of the separately reviewed plan.
"""
import argparse
import hashlib
import json
import os
import pathlib
import re
import stat
import sys
import zipfile

BASE = pathlib.Path('/srv/puty-suite')
PROJECT = 'puty-suite-prod'
PLUGIN = 'biz-suite'
MAX_ARCHIVE = 160 << 20
MAX_EXPANDED = 320 << 20
MAX_FRONTEND = 32 << 20


def require(ok, message):
    if not ok:
        raise RuntimeError(message)


def canonical(value):
    return json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(',', ':')).encode()


def digest(raw):
    return hashlib.sha256(raw).hexdigest()


def regular(path):
    require(path.is_file() and not path.is_symlink() and path.resolve() == path
            and stat.S_ISREG(path.stat().st_mode) and path.stat().st_nlink == 1,
            'regular_unlinked_owned_file_required')
    return path


def file_hash(path):
    regular(path)
    value = hashlib.sha256()
    with path.open('rb') as source:
        for block in iter(lambda: source.read(1024 * 1024), b''):
            value.update(block)
    return value.hexdigest()


def safe_path(name):
    if not isinstance(name, str) or not name or ':' in name or '\\' in name or name.startswith('/'):
        return False
    parts = name.split('/')
    for part in parts:
        if not part or part.startswith('.') or part.rstrip('. ') != part or any(ord(c) < 32 for c in part):
            return False
        stem = part.split('.', 1)[0].upper()
        if stem in ('CON', 'PRN', 'AUX', 'NUL') or re.fullmatch(r'(COM|LPT)[0-9]', stem):
            return False
    return name.startswith(('bin/', 'admin/', 'public/')) or name in ('README.md', 'LICENSE.txt')


def owned_package(expected):
    require(re.fullmatch(r'[0-9a-f]{64}', expected), 'exact_active_package_SHA_required')
    require(BASE.resolve() == BASE, 'production_root_redirected')
    marker_path = regular(BASE / 'openim/instance.json')
    activation_path = regular(BASE / 'activation.json')
    marker, activation = [json.loads(p.read_text()) for p in (marker_path, activation_path)]
    owner = marker.get('instanceID')
    require(isinstance(owner, str) and re.fullmatch(r'[0-9a-f]{32}', owner)
            and marker.get('production') is True and marker.get('testOnly') is False
            and marker.get('root') == str(BASE / 'openim') and marker.get('stackName') == PROJECT
            and activation.get('instanceID') == owner, 'production_owner_marker_mismatch')
    plugin_base = BASE / 'gosuxin/storage/plugins' / PLUGIN
    active_path = regular(plugin_base / 'active.json')
    active_raw = active_path.read_bytes()
    active = json.loads(active_raw)
    require(active.get('package') == expected and active.get('disabled', False) is False,
            'active_package_changed_or_disabled')
    package = plugin_base / 'packages' / expected
    require(package.is_dir() and not package.is_symlink() and package.resolve() == package,
            'active_package_redirected')
    return package, owner, digest(active_raw)


def inspect_current(package, expected):
    raw = regular(package / 'plugin.json').read_bytes()
    require(len(raw) <= 1 << 20 and digest(raw) == expected, 'active_manifest_digest_mismatch')
    manifest = json.loads(raw)
    files = manifest.get('files')
    require(manifest.get('format') == 'suxin-runtime-v1' and manifest.get('name') == PLUGIN
            and manifest.get('protocol') == 1 and manifest.get('os') == 'linux'
            and manifest.get('arch') == 'amd64' and isinstance(manifest.get('version'), str)
            and bool(manifest['version']) and isinstance(files, dict) and 1 <= len(files) <= 2999
            and 'bin/' + PLUGIN in files, 'active_runtime_manifest_invalid')
    require(len({name.lower() for name in files}) == len(files), 'duplicate_casefold_manifest_path')
    actual = set()
    for directory, dirs, names in os.walk(package, followlinks=False):
        for name in dirs:
            path = pathlib.Path(directory) / name
            require(not path.is_symlink() and path.resolve() == path, 'linked_package_directory_refused')
        for name in names:
            path = regular(pathlib.Path(directory) / name)
            actual.add(path.relative_to(package).as_posix())
    require(actual == set(files) | {'plugin.json'}, 'active_package_has_missing_or_unlisted_files')
    total = 0
    sizes = {}
    for name, expected_hash in files.items():
        require(safe_path(name) and isinstance(expected_hash, str)
                and re.fullmatch(r'[0-9a-f]{64}', expected_hash), 'manifest_file_path_or_digest_invalid')
        path = regular(package / name)
        size = path.stat().st_size
        require(size <= MAX_ARCHIVE and file_hash(path) == expected_hash, 'active_package_file_integrity_failed')
        total += size
        sizes[name] = size
    require(total <= MAX_EXPANDED, 'active_package_expanded_size_exceeded')
    return manifest, sizes


def inspect_frontend(archive, expected):
    require(isinstance(expected, str) and re.fullmatch(r'[0-9a-f]{64}', expected), 'frontend_SHA_required')
    require(regular(archive).stat().st_size <= MAX_FRONTEND and file_hash(archive) == expected,
            'frontend_archive_digest_or_size_mismatch')
    rows = {}
    with zipfile.ZipFile(archive) as package:
        require(len(package.infolist()) <= 1000, 'frontend_archive_too_many_entries')
        total = 0
        for item in package.infolist():
            mode = item.external_attr >> 16
            require(not stat.S_ISLNK(mode) and not item.flag_bits & 1, 'linked_or_encrypted_frontend_entry')
            if item.is_dir():
                require(item.filename.rstrip('/').startswith('assets'), 'unexpected_frontend_directory')
                continue
            name = item.filename
            require(name == 'index.html' or name.startswith('assets/'), 'frontend_root_entry_not_allowed')
            require(safe_path('admin/' + name) and name.lower() not in {n.lower() for n in rows},
                    'frontend_path_or_duplicate_refused')
            require(item.file_size <= MAX_FRONTEND, 'frontend_file_too_large')
            total += item.file_size
            require(total <= MAX_FRONTEND, 'frontend_expanded_size_exceeded')
            raw = package.read(item)
            require(len(raw) == item.file_size, 'frontend_file_incomplete')
            rows[name] = {'sha256': digest(raw), 'bytes': len(raw)}
        require('index.html' in rows and any(n.endswith('.js') for n in rows)
                and any(n.endswith('.css') for n in rows), 'frontend_entrypoints_missing')
        html = package.read('index.html').decode('utf-8')
        references = re.findall(r'(?:src|href)=["\'](/plugins/biz-suite/admin/[^"\']+)["\']', html)
        require(bool(references) and all(url.split('?', 1)[0].removeprefix('/plugins/biz-suite/admin/') in rows for url in references),
                'frontend_HTML_references_missing_asset')
    return rows


def create_plan(expected, frontend, frontend_sha):
    package, owner, active_sha = owned_package(expected)
    require(frontend.resolve() == frontend and frontend.is_relative_to(BASE / 'staging'),
            'frontend_archive_must_be_in_owned_staging')
    manifest, sizes = inspect_current(package, expected)
    frontend_files = inspect_frontend(frontend, frontend_sha)
    new_files = {name: value for name, value in manifest['files'].items() if not name.startswith('admin/')}
    new_files.update({'admin/' + name: row['sha256'] for name, row in frontend_files.items()})
    proposed = {**manifest, 'files': new_files}
    proposed_raw = canonical(proposed) + b'\n'
    require(all(proposed[k] == manifest[k] for k in manifest if k != 'files'), 'nonfile_metadata_changed')
    require(len(new_files) <= 2999 and sum(sizes[n] for n in sizes if not n.startswith('admin/'))
            + sum(row['bytes'] for row in frontend_files.values()) <= MAX_EXPANDED,
            'proposed_package_exceeds_runtime_limits')
    nonadmin = {n: h for n, h in new_files.items() if not n.startswith('admin/')}
    plan = {'format': 'puty-plugin-admin-repackage-v1', 'instanceID': owner,
            'expectedActivePackage': expected, 'activeMarkerSHA256': active_sha,
            'frontendArchiveSHA256': frontend_sha, 'frontendArchive': str(frontend),
            'currentManifestSHA256': expected, 'proposedManifestSHA256': digest(proposed_raw),
            'version': manifest['version'], 'versionChanged': False, 'sameVersionNewDigestSupported': True,
            'binarySHA256': manifest['files']['bin/' + PLUGIN],
            'preservedNonAdminFiles': nonadmin, 'frontendFiles': frontend_files,
            'removedOldAdminFileCount': sum(n.startswith('admin/') for n in manifest['files']),
            'newAdminFileCount': len(frontend_files), 'outputFileCount': len(new_files) + 1,
            'currentManifest': manifest, 'proposedManifest': proposed,
            'productionModified': False, 'installationExecuted': False}
    require(plan['proposedManifestSHA256'] != expected, 'frontend_package_has_no_new_identity')
    return plan


def public_plan(plan):
    return {key: value for key, value in plan.items() if key not in ('currentManifest', 'proposedManifest', 'preservedNonAdminFiles', 'frontendFiles')}


def write_member(archive, name, source, expected_hash):
    info = zipfile.ZipInfo(name, date_time=(1980, 1, 1, 0, 0, 0))
    info.create_system = 3
    info.external_attr = (stat.S_IFREG | (0o700 if name == 'bin/' + PLUGIN else 0o600)) << 16
    info.compress_type = zipfile.ZIP_DEFLATED
    value = hashlib.sha256()
    with archive.open(info, 'w') as target:
        for block in iter(lambda: source.read(1024 * 1024), b''):
            target.write(block)
            value.update(block)
    require(value.hexdigest() == expected_hash, 'source_changed_while_repackaging')


def build(plan, frontend):
    import io
    package, _, active_sha = owned_package(plan['expectedActivePackage'])
    require(active_sha == plan['activeMarkerSHA256'], 'active_marker_changed_before_build')
    stage = BASE / 'staging/plugin-admin-repackage'
    require(stage.parent.resolve() == stage.parent and not stage.is_symlink(), 'repackage_stage_redirected')
    stage.mkdir(mode=0o700, exist_ok=True)
    require(stage.resolve() == stage, 'repackage_stage_redirected')
    folder = stage / plan['proposedManifestSHA256']
    folder.mkdir(mode=0o700)
    output = folder / ('biz-suite-runtime-admin-' + plan['proposedManifestSHA256'] + '.zip')
    temporary = folder / 'package.zip.partial'
    with zipfile.ZipFile(frontend) as source, zipfile.ZipFile(temporary, 'x', compression=zipfile.ZIP_DEFLATED, compresslevel=1) as target:
        for name, expected in sorted(plan['preservedNonAdminFiles'].items()):
            with regular(package / name).open('rb') as content:
                write_member(target, name, content, expected)
        for name, record in sorted(plan['frontendFiles'].items()):
            with source.open(name) as content:
                write_member(target, 'admin/' + name, content, record['sha256'])
        manifest_raw = canonical(plan['proposedManifest']) + b'\n'
        write_member(target, 'plugin.json', io.BytesIO(manifest_raw), plan['proposedManifestSHA256'])
    require(temporary.stat().st_size <= MAX_ARCHIVE, 'repackaged_archive_exceeds_runtime_limit')
    # Re-read every finished member; do not publish an unchecked archive.
    with zipfile.ZipFile(temporary) as archive:
        expected = {**plan['proposedManifest']['files'], 'plugin.json': plan['proposedManifestSHA256']}
        require(set(archive.namelist()) == set(expected), 'repackaged_archive_file_set_mismatch')
        for name, value in expected.items():
            with archive.open(name) as content:
                hasher = hashlib.sha256()
                for block in iter(lambda: content.read(1024 * 1024), b''):
                    hasher.update(block)
                require(hasher.hexdigest() == value, 'repackaged_archive_member_hash_mismatch')
    _, _, current_active = owned_package(plan['expectedActivePackage'])
    require(current_active == plan['activeMarkerSHA256'] and file_hash(frontend) == plan['frontendArchiveSHA256'],
            'reviewed_active_package_or_frontend_changed')
    inspect_current(package, plan['expectedActivePackage'])
    with temporary.open('r+b') as archive:
        os.fsync(archive.fileno())
    temporary.rename(output)
    if os.name == 'posix':
        descriptor = os.open(folder, os.O_RDONLY | os.O_DIRECTORY)
        try:
            os.fsync(descriptor)
        finally:
            os.close(descriptor)
    return {'builtStagingOnly': True, 'packageZIP': str(output), 'packageZipSHA256': file_hash(output),
            'packageZipBytes': output.stat().st_size, 'newPackageIdentity': plan['proposedManifestSHA256'],
            'sameBinarySHA256': plan['binarySHA256'], 'nonAdminFilesUnchanged': True,
            'productionModified': False, 'installationExecuted': False}


def main():
    import fcntl
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('action', choices=('plan', 'build'), nargs='?', default='plan')
    parser.add_argument('--expected-active', required=True)
    parser.add_argument('--frontend-zip', type=pathlib.Path, required=True)
    parser.add_argument('--frontend-sha256', required=True)
    parser.add_argument('--plan-sha256')
    args = parser.parse_args()
    require(os.name == 'posix' and os.geteuid() == 0, 'root_operated_Linux_tool_required')
    os.umask(0o077)
    lock_path = regular(BASE / '.backup-stage/backup.lock')
    with lock_path.open('rb') as lock:
        fcntl.flock(lock, (fcntl.LOCK_SH if args.action == 'plan' else fcntl.LOCK_EX) | fcntl.LOCK_NB)
        plan = create_plan(args.expected_active, args.frontend_zip, args.frontend_sha256)
        plan_sha = digest(canonical(plan))
        if args.action == 'plan':
            print(json.dumps({'readOnlyPlan': True, 'planSHA256': plan_sha, **public_plan(plan)}))
        else:
            require(args.plan_sha256 == plan_sha, 'exact_reviewed_plan_SHA256_required')
            print(json.dumps(build(plan, args.frontend_zip)))


if __name__ == '__main__':
    try:
        main()
    except Exception as error:
        print(json.dumps({'repackageFailed': True, 'category': type(error).__name__,
                          'message': str(error) if type(error) is RuntimeError else 'private_details_suppressed'}))
        sys.exit(1)
