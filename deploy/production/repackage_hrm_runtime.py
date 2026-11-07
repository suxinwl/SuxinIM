#!/usr/bin/env python3
"""Plan/build a staging-only immutable biz-suite runtime binary update.

The installed package, active.json, admin resources and business data are never
modified. Upload/install the resulting complete ZIP through the normal host API.
Place this file beside the reviewed repackage_hrm_admin.py dependency.
"""
import argparse
import hashlib
import importlib.util
import io
import json
import os
import pathlib
import re
import stat
import struct
import sys
import zipfile

_spec = importlib.util.spec_from_file_location(
    'puty_repackage_admin_helpers', pathlib.Path(__file__).with_name('repackage_hrm_admin.py'))
helpers = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(helpers)

EXPECTED_ACTIVE = '4dd7f7c5d2e999836decbe3c21c1ca489acf2bea51df07e82fed26dac7fe8264'
VERSION = '0.4.1-openim3.8'
BINARY = 'bin/biz-suite'
require = helpers.require
canonical = helpers.canonical
digest = helpers.digest
regular = helpers.regular
file_hash = helpers.file_hash


def owned_directory(path):
    require(path.is_dir() and not path.is_symlink() and path.resolve() == path,
            'owned_directory_redirected_or_missing')
    if os.name == 'posix':
        metadata = path.stat()
        require(metadata.st_uid == 0 and metadata.st_mode & 0o022 == 0,
                'root_owned_nonwritable_staging_required')


def staged_binary(binary, expected):
    require(isinstance(expected, str) and re.fullmatch(r'[0-9a-f]{64}', expected),
            'replacement_binary_SHA_required')
    require(binary.is_absolute() and binary.resolve() == binary
            and binary.is_relative_to(helpers.BASE / 'staging'),
            'replacement_binary_must_be_in_owned_staging')
    parent = binary.parent
    while True:
        owned_directory(parent)
        if parent == helpers.BASE:
            break
        parent = parent.parent
    regular(binary)
    metadata = binary.stat()
    require(64 <= metadata.st_size <= helpers.MAX_ARCHIVE and file_hash(binary) == expected,
            'replacement_binary_digest_or_size_mismatch')
    if os.name == 'posix':
        require(metadata.st_uid == 0 and metadata.st_mode & 0o022 == 0,
                'root_owned_nonwritable_binary_required')
    with binary.open('rb') as source:
        header = source.read(64)
    require(header[:7] == b'\x7fELF\x02\x01\x01' and header[7] in (0, 3),
            'replacement_must_be_64bit_little_endian_Linux_ELF')
    kind, machine, version = struct.unpack_from('<HHI', header, 16)
    entry, program_offset = struct.unpack_from('<QQ', header, 24)
    header_size, program_size, program_count = struct.unpack_from('<HHH', header, 52)
    require(kind in (2, 3) and machine == 62 and version == 1 and entry > 0
            and header_size == 64 and program_size == 56 and 1 <= program_count <= 256
            and program_offset >= 64 and program_offset + program_size * program_count <= metadata.st_size,
            'replacement_must_be_executable_Linux_amd64_ELF')
    executable_entry = False
    with binary.open('rb') as source:
        source.seek(program_offset)
        for _ in range(program_count):
            segment, flags, offset, address, _, file_size, memory_size, _ = struct.unpack('<IIQQQQQQ', source.read(56))
            if segment == 1:
                require(offset + file_size <= metadata.st_size and memory_size >= file_size,
                        'replacement_ELF_load_segment_invalid')
                executable_entry |= bool(flags & 1 and address <= entry < address + file_size)
    require(executable_entry, 'replacement_ELF_entrypoint_not_in_executable_segment')
    return {'sha256': expected, 'bytes': metadata.st_size}


def create_plan(expected, binary, binary_sha):
    require(expected == EXPECTED_ACTIVE, 'this_upgrade_requires_the_reviewed_active_package')
    package, owner, active_sha = helpers.owned_package(expected)
    replacement = staged_binary(binary, binary_sha)
    manifest, sizes = helpers.inspect_current(package, expected)
    files = dict(manifest['files'])
    files[BINARY] = binary_sha
    proposed = {**manifest, 'version': VERSION, 'files': files}
    require(all(proposed[k] == value for k, value in manifest.items() if k not in ('version', 'files')),
            'unrelated_manifest_metadata_changed')
    preserved = {name: value for name, value in manifest['files'].items() if name != BINARY}
    require(all(files[name] == value for name, value in preserved.items())
            and set(files) == set(manifest['files']), 'nonbinary_file_set_changed')
    require(sum(size for name, size in sizes.items() if name != BINARY) + replacement['bytes']
            <= helpers.MAX_EXPANDED, 'proposed_package_exceeds_runtime_limit')
    proposed_raw = canonical(proposed) + b'\n'
    require(digest(proposed_raw) != expected, 'runtime_package_has_no_new_identity')
    return {'format': 'puty-plugin-runtime-repackage-v1', 'instanceID': owner,
            'expectedActivePackage': expected, 'activeMarkerSHA256': active_sha,
            'currentManifestSHA256': expected, 'proposedManifestSHA256': digest(proposed_raw),
            'previousVersion': manifest['version'], 'version': VERSION,
            'replacementBinary': str(binary), 'replacementBinarySHA256': binary_sha,
            'replacementBinaryBytes': replacement['bytes'], 'previousBinarySHA256': manifest['files'][BINARY],
            'preservedFiles': preserved, 'preservedAdminFileCount': sum(n.startswith('admin/') for n in preserved),
            'outputFileCount': len(files) + 1, 'currentManifest': manifest, 'proposedManifest': proposed,
            'adminFilesUnchanged': True, 'productionModified': False, 'installationExecuted': False}


def public_plan(plan):
    return {key: value for key, value in plan.items()
            if key not in ('currentManifest', 'proposedManifest', 'preservedFiles')}


def require_reviewed(plan, binary, reviewed_sha):
    require(isinstance(reviewed_sha, str) and re.fullmatch(r'[0-9a-f]{64}', reviewed_sha)
            and digest(canonical(plan)) == reviewed_sha, 'exact_reviewed_plan_SHA256_required')
    current = create_plan(plan['expectedActivePackage'], binary, plan['replacementBinarySHA256'])
    require(digest(canonical(current)) == reviewed_sha, 'reviewed_active_package_or_binary_changed')


def build(plan, binary, reviewed_sha):
    require_reviewed(plan, binary, reviewed_sha)
    package, _, _ = helpers.owned_package(plan['expectedActivePackage'])
    stage = helpers.BASE / 'staging/plugin-runtime-repackage'
    owned_directory(stage.parent)
    stage.mkdir(mode=0o700, exist_ok=True)
    owned_directory(stage)
    folder = stage / plan['proposedManifestSHA256']
    folder.mkdir(mode=0o700)
    owned_directory(folder)
    output = folder / ('biz-suite-runtime-' + VERSION + '-' + plan['proposedManifestSHA256'] + '.zip')
    temporary = folder / 'package.zip.partial'
    with zipfile.ZipFile(temporary, 'x', compression=zipfile.ZIP_DEFLATED, compresslevel=1) as archive:
        for name, expected in sorted(plan['proposedManifest']['files'].items()):
            source = binary if name == BINARY else package / name
            with regular(source).open('rb') as content:
                helpers.write_member(archive, name, content, expected)
        manifest_raw = canonical(plan['proposedManifest']) + b'\n'
        helpers.write_member(archive, 'plugin.json', io.BytesIO(manifest_raw), plan['proposedManifestSHA256'])
    require(temporary.stat().st_size <= helpers.MAX_ARCHIVE, 'repackaged_archive_exceeds_runtime_limit')
    with zipfile.ZipFile(temporary) as archive:
        expected_files = {**plan['proposedManifest']['files'], 'plugin.json': plan['proposedManifestSHA256']}
        require(len(archive.infolist()) == len(expected_files)
                and set(archive.namelist()) == set(expected_files), 'repackaged_archive_file_set_mismatch')
        for name, expected in expected_files.items():
            with archive.open(name) as content:
                value = hashlib.sha256()
                for block in iter(lambda: content.read(1024 * 1024), b''):
                    value.update(block)
                require(value.hexdigest() == expected, 'repackaged_archive_member_hash_mismatch')
    # Re-read all original files/markers and the candidate after streaming the ZIP.
    require_reviewed(plan, binary, reviewed_sha)
    with temporary.open('r+b') as archive:
        os.fsync(archive.fileno())
    temporary.rename(output)
    if os.name == 'posix':
        descriptor = os.open(folder, os.O_RDONLY | os.O_DIRECTORY)
        try:
            os.fsync(descriptor)
        finally:
            os.close(descriptor)
    return {'builtStagingOnly': True, 'reviewedPlanSHA256': reviewed_sha,
            'packageZIP': str(output), 'packageZipSHA256': file_hash(output),
            'packageZipBytes': output.stat().st_size, 'newPackageIdentity': plan['proposedManifestSHA256'],
            'version': VERSION, 'replacementBinarySHA256': plan['replacementBinarySHA256'],
            'adminFilesUnchanged': True, 'nonBinaryFilesUnchanged': True,
            'productionModified': False, 'installationExecuted': False}


def main():
    import fcntl
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('action', choices=('plan', 'build'), nargs='?', default='plan')
    parser.add_argument('--expected-active', required=True)
    parser.add_argument('--binary', type=pathlib.Path, required=True)
    parser.add_argument('--binary-sha256', required=True)
    parser.add_argument('--plan-sha256')
    args = parser.parse_args()
    require(os.name == 'posix' and os.geteuid() == 0, 'root_operated_Linux_tool_required')
    os.umask(0o077)
    lock_path = regular(helpers.BASE / '.backup-stage/backup.lock')
    with lock_path.open('rb') as lock:
        fcntl.flock(lock, (fcntl.LOCK_SH if args.action == 'plan' else fcntl.LOCK_EX) | fcntl.LOCK_NB)
        plan = create_plan(args.expected_active, args.binary, args.binary_sha256)
        plan_sha = digest(canonical(plan))
        if args.action == 'plan':
            print(json.dumps({'readOnlyPlan': True, 'planSHA256': plan_sha, **public_plan(plan)}))
        else:
            print(json.dumps(build(plan, args.binary, args.plan_sha256)))


if __name__ == '__main__':
    try:
        main()
    except Exception as error:
        print(json.dumps({'repackageFailed': True, 'category': type(error).__name__,
                          'message': str(error) if type(error) is RuntimeError else 'private_details_suppressed'}))
        sys.exit(1)
