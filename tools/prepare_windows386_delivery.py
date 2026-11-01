"""Preserve previous delivery metadata and package the already built Web tree."""
import datetime
import hashlib
import json
import pathlib
import shutil
import tarfile

ROOT = pathlib.Path(__file__).resolve().parents[1]
HISTORY = ROOT / 'artifacts/manifest-history/pre-windows386-20261101T1705Z'


def digest(path):
    with path.open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()


def main():
    HISTORY.mkdir(parents=True, exist_ok=False)
    names = [
        'artifacts/release-manifest.json', 'artifacts/SHA256SUMS.txt',
        'artifacts/web-client-final.tar.gz', 'docs/licenses.json',
        'tests/release-downloads.json', 'tools/release_manifest.py',
    ]
    previous = []
    for name in names:
        source = ROOT / name
        if not source.is_file():
            continue
        target = HISTORY / name
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(source, target)
        assert digest(source) == digest(target)
        previous.append({'path': name, 'bytes': source.stat().st_size,
                         'sha256': digest(target)})
    (HISTORY / 'manifest.json').write_text(json.dumps({
        'utc': datetime.datetime.now(datetime.timezone.utc).isoformat(),
        'reason': 'Preserve current metadata and Web bundle before Windows 3.8.6 delivery.',
        'files': previous,
    }, indent=2) + '\n', encoding='utf-8')
    web = ROOT / 'clients/web-desktop/dist-web'
    target = ROOT / 'artifacts/web-client-final.tar.gz'
    files = sorted(path for path in web.rglob('*') if path.is_file())
    assert files and (web / 'index.html').is_file()
    assert not any(path.is_symlink() for path in web.rglob('*'))
    assert not (web / 'downloads').exists() and not (web / 'admin').exists()
    with tarfile.open(target, 'w:gz') as archive:
        for path in files:
            archive.add(path, arcname=path.relative_to(web).as_posix(), recursive=False)
    manifest = {
        'version': '3.8.6', 'build': 380601,
        'archive': {'path': target.relative_to(ROOT).as_posix(),
                    'bytes': target.stat().st_size, 'sha256': digest(target)},
        'files': [{'path': path.relative_to(web).as_posix(),
                   'bytes': path.stat().st_size, 'sha256': digest(path)} for path in files],
    }
    (ROOT / 'tests/delivery/windows-386-web-bundle.json').write_text(
        json.dumps(manifest, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'history': str(HISTORY), 'webArchive': manifest['archive'],
                      'webFiles': len(files)}))


if __name__ == '__main__':
    main()
