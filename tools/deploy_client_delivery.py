"""Linux: atomically deliver validated Web/Windows files to the isolated stack."""
import argparse
import datetime
import hashlib
import json
import os
import pathlib
import re
import shutil
import tarfile

BASE = pathlib.Path('/srv/openim38-upgrade')
WEB = BASE / 'www'
STAGE = pathlib.Path('/tmp')


def sha(path):
    with path.open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()


def target_for(relative):
    assert not relative.is_absolute() and '..' not in relative.parts
    target = WEB / relative
    assert target.resolve().is_relative_to(WEB.resolve())
    for part in [target, *target.parents]:
        if part == BASE:
            break
        assert not part.is_symlink()
    return target


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--previous-web-sha', required=True)
    parser.add_argument('--previous-downloads-sha', required=True)
    args = parser.parse_args()
    assert all(re.fullmatch('[a-f0-9]{64}', value) for value in vars(args).values())
    assert WEB.resolve() == pathlib.Path('/srv/openim38-upgrade/www')
    manifest = json.loads((STAGE / 'openim38-client-release-manifest.json').read_text())
    version = manifest['version']
    assert re.fullmatch(r'\d{1,4}\.\d{1,4}\.\d{1,4}', version)
    package = manifest['installer']
    assert package['name'] == f'OpenIM38-Enterprise-{version}-win-x64.exe'
    installer = STAGE / 'openim38-client-release-installer.next'
    archive_path = STAGE / 'openim38-client-release-web.next'
    downloads = STAGE / 'openim38-client-release-downloads.next'
    for path, expected_sha, expected_size in [(installer, package['sha256'], package['bytes']),
                                             (archive_path, manifest['archive']['sha256'], manifest['archive']['bytes'])]:
        assert path.is_file() and sha(path) == expected_sha and path.stat().st_size == expected_size
    assert sha(downloads) == manifest['downloadsSha256']
    assert package['name'] in downloads.read_text() and package['sha256'] in downloads.read_text()
    assert sha(WEB / 'index.html') == args.previous_web_sha
    assert sha(WEB / 'downloads/index.html') == args.previous_downloads_sha
    assert not (WEB / 'downloads' / package['name']).exists()
    expected = {item['path']: item for item in manifest['files']}
    assert len(expected) == len(manifest['files'])
    with tarfile.open(archive_path, 'r:gz') as archive:
        members = archive.getmembers()
        assert len(members) == len(expected) and {m.name for m in members} == set(expected)
        for member in members:
            relative = pathlib.PurePosixPath(member.name)
            assert member.isfile() and not member.islnk() and not member.issym()
            assert relative.parts[0] not in ('admin', 'downloads')
            target_for(relative)
            assert member.size == expected[member.name]['bytes']
            with archive.extractfile(member) as stream:
                assert hashlib.file_digest(stream, 'sha256').hexdigest() == expected[member.name]['sha256']
        stamp = datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%SZ')
        backup = BASE / f'backups/client-{version}-{stamp}'
        backup.mkdir(parents=True, mode=0o700, exist_ok=False)
        changed, unchanged = [], 0

        def replace(stream, relative, expected_sha):
            nonlocal unchanged
            target = target_for(relative)
            previous = sha(target) if target.exists() else None
            if previous == expected_sha:
                unchanged += 1
                return
            if previous:
                old = backup / 'previous' / relative
                old.parent.mkdir(parents=True, exist_ok=True)
                shutil.copy2(target, old)
                assert sha(old) == previous
            target.parent.mkdir(parents=True, exist_ok=True)
            temporary = target.with_name(target.name + '.client-release-next')
            assert not temporary.exists()
            with temporary.open('xb') as output:
                shutil.copyfileobj(stream, output)
            os.chmod(temporary, 0o644)
            assert sha(temporary) == expected_sha
            os.replace(temporary, target)
            changed.append({'path': str(relative), 'previousSha256': previous, 'sha256': expected_sha})

        for member in members:
            if member.name == 'index.html':
                continue
            with archive.extractfile(member) as stream:
                replace(stream, pathlib.PurePosixPath(member.name), expected[member.name]['sha256'])
        with installer.open('rb') as stream:
            replace(stream, pathlib.PurePosixPath('downloads') / package['name'], package['sha256'])
        with archive.extractfile('index.html') as stream:
            replace(stream, pathlib.PurePosixPath('index.html'), expected['index.html']['sha256'])
        with downloads.open('rb') as stream:
            replace(stream, pathlib.PurePosixPath('downloads/index.html'), manifest['downloadsSha256'])
    report = {'utc': stamp, 'scope': str(WEB), 'version': version, 'build': manifest['build'],
              'installer': package, 'webFilesVerified': len(expected), 'unchangedFiles': unchanged,
              'changedFiles': changed, 'backup': str(backup),
              'limits': ['No service/Nginx/firewall/TLS/account or production-instance changes.', 'No Android package published.']}
    (backup / 'deployment-report.json').write_text(json.dumps(report, indent=2) + '\n')
    (STAGE / 'openim38-client-release-deployment-report.json').write_text(json.dumps(report, indent=2) + '\n')
    os.chmod(STAGE / 'openim38-client-release-deployment-report.json', 0o644)
    print(json.dumps({'deployed': True, 'version': version, 'changedFiles': len(changed), 'unchangedFiles': unchanged, 'backup': str(backup)}))


if __name__ == '__main__':
    main()
