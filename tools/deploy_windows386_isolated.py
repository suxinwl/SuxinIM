"""Run on the isolated Linux host after staging validated 3.8.6 artifacts.

Copies only the built Web files, a new immutable installer and downloads index.
Existing admin/download packages and all server services remain untouched.
"""
import datetime
import hashlib
import json
import os
import pathlib
import shutil
import tarfile

BASE = pathlib.Path('/srv/openim38-upgrade')
WEB = BASE / 'www'
BACKUP = BASE / 'backups/windows386-20261101T1710Z'
STAGED = pathlib.Path('/tmp')
INSTALLER_NAME = 'OpenIM38-Enterprise-3.8.6-win-x64.exe'
INSTALLER_SHA = 'b529a90986d4c70176922e3965c1c88d6ef69432743e71a09f7f991a7e0018c3'
ARCHIVE_SHA = '4d90c433d8dfc5272e19fce5e0717496fb83834956e67039bef20bddeb9e1f86'
PREVIOUS_WEB = 'fee8581c1c5f2e866216f8bc1f7520ba8ac2e7f0e0c9b240f259362d96b026a6'
PREVIOUS_DOWNLOADS = 'cf53e05484709d58005b449dc424aaf64f6f10a0ddafa7d28b948ee31766a2f4'


def sha(path):
    with path.open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()


def guarded_target(relative):
    if relative.is_absolute() or '..' in relative.parts:
        raise RuntimeError('Invalid relative deployment path')
    target = WEB / relative
    if not target.resolve().is_relative_to(WEB.resolve()):
        raise RuntimeError('Target escapes isolated Web root')
    for part in [target, *target.parents]:
        if part == BASE:
            break
        if part.is_symlink():
            raise RuntimeError('Refusing symlink deployment target')
    return target


def main():
    assert WEB.resolve() == pathlib.Path('/srv/openim38-upgrade/www')
    installer = STAGED / 'openim38-windows386-installer.next'
    archive_path = STAGED / 'openim38-windows386-web.next'
    downloads = STAGED / 'openim38-windows386-downloads.next'
    manifest = json.loads((STAGED / 'openim38-windows386-web-manifest.json').read_text())
    assert sha(installer) == INSTALLER_SHA and installer.stat().st_size == 160967316
    assert sha(archive_path) == ARCHIVE_SHA == manifest['archive']['sha256']
    assert downloads.is_file() and INSTALLER_NAME in downloads.read_text()
    assert INSTALLER_SHA in downloads.read_text()
    assert sha(WEB / 'index.html') == PREVIOUS_WEB
    assert sha(WEB / 'downloads/index.html') == PREVIOUS_DOWNLOADS
    assert not (WEB / 'downloads' / INSTALLER_NAME).exists()
    expected = {item['path']: item for item in manifest['files']}
    assert len(expected) == len(manifest['files'])
    with tarfile.open(archive_path, 'r:gz') as archive:
        members = archive.getmembers()
        assert {item.name for item in members} == set(expected)
        for member in members:
            relative = pathlib.PurePosixPath(member.name)
            assert member.isfile() and not member.islnk() and not member.issym()
            assert relative.parts[0] not in ('admin', 'downloads')
            assert member.size == expected[member.name]['bytes']
            guarded_target(relative)
            with archive.extractfile(member) as stream:
                assert hashlib.file_digest(stream, 'sha256').hexdigest() == expected[member.name]['sha256']
        BACKUP.mkdir(parents=True, exist_ok=False)
        changed, unchanged = [], 0

        def replace(source, relative, expected_sha):
            nonlocal unchanged
            target = guarded_target(relative)
            existed = target.exists()
            old_sha = sha(target) if existed else None
            if old_sha == expected_sha:
                unchanged += 1
                return
            if existed:
                saved = BACKUP / 'previous' / relative
                saved.parent.mkdir(parents=True, exist_ok=True)
                shutil.copy2(target, saved)
                assert sha(saved) == old_sha
            target.parent.mkdir(parents=True, exist_ok=True)
            temporary = target.with_name(target.name + '.windows386-next')
            assert not temporary.exists()
            with temporary.open('xb') as out:
                shutil.copyfileobj(source, out)
            os.chmod(temporary, 0o644)
            assert sha(temporary) == expected_sha
            os.replace(temporary, target)
            changed.append({'path': str(relative), 'previousSha256': old_sha, 'sha256': expected_sha})

        # Publish resources first; the old index continues to reference old assets.
        for member in members:
            if member.name == 'index.html':
                continue
            with archive.extractfile(member) as stream:
                replace(stream, pathlib.PurePosixPath(member.name), expected[member.name]['sha256'])
        with installer.open('rb') as stream:
            replace(stream, pathlib.PurePosixPath('downloads') / INSTALLER_NAME, INSTALLER_SHA)
        with archive.extractfile('index.html') as stream:
            replace(stream, pathlib.PurePosixPath('index.html'), expected['index.html']['sha256'])
        with downloads.open('rb') as stream:
            replace(stream, pathlib.PurePosixPath('downloads/index.html'), sha(downloads))
    report = {
        'utc': datetime.datetime.now(datetime.timezone.utc).isoformat(),
        'scope': str(WEB), 'version': '3.8.6', 'build': 380601,
        'installer': {'name': INSTALLER_NAME, 'sha256': INSTALLER_SHA, 'bytes': 160967316},
        'webFilesVerified': len(expected), 'unchangedFiles': unchanged,
        'changedFiles': changed, 'backup': str(BACKUP),
        'limits': ['No service/Nginx/firewall/TLS/account changes.',
                   'No Android package published; no production instance modified.'],
    }
    (BACKUP / 'deployment-report.json').write_text(json.dumps(report, indent=2) + '\n')
    print(json.dumps({'deployed': True, 'webFilesVerified': len(expected),
                      'changedFiles': len(changed), 'unchangedFiles': unchanged,
                      'backup': str(BACKUP)}))


if __name__ == '__main__':
    main()
