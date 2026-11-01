"""Package the current, already built Web/Windows delivery without secrets."""
import datetime
import hashlib
import json
import pathlib
import shutil
import tarfile

ROOT = pathlib.Path(__file__).resolve().parents[1]


def sha(path):
    with path.open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()


def main():
    current = json.loads((ROOT / 'clients/web-desktop/build-version.json').read_text())
    version, build = current['version'], current['build']
    stamp = datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%SZ')
    history = ROOT / f'artifacts/manifest-history/pre-windows{version.replace(".", "")}-{stamp}'
    history.mkdir(parents=True, exist_ok=False)
    records = []
    for name in ['artifacts/release-manifest.json', 'artifacts/SHA256SUMS.txt',
                 'artifacts/web-client-final.tar.gz', 'docs/licenses.json',
                 'tests/release-downloads.json', 'tools/release_manifest.py']:
        source, target = ROOT / name, history / name
        if not source.is_file():
            continue
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(source, target)
        assert sha(source) == sha(target)
        records.append({'path': name, 'bytes': target.stat().st_size, 'sha256': sha(target)})
    (history / 'manifest.json').write_text(json.dumps({'utc': stamp, 'files': records}, indent=2) + '\n')
    installer = ROOT / f'clients/web-desktop/release/OpenIM38-Enterprise-{version}-win-x64.exe'
    assert installer.is_file()
    web = ROOT / 'clients/web-desktop/dist-web'
    files = sorted(p for p in web.rglob('*') if p.is_file())
    assert (web / 'index.html').is_file()
    assert not any(p.is_symlink() for p in web.rglob('*'))
    assert not (web / 'admin').exists() and not (web / 'downloads').exists()
    archive = ROOT / 'artifacts/web-client-final.tar.gz'
    with tarfile.open(archive, 'w:gz') as stream:
        for path in files:
            stream.add(path, arcname=path.relative_to(web).as_posix(), recursive=False)
    page = ROOT / 'artifacts/downloads/index.html'
    assert installer.name in page.read_text(encoding='utf-8')
    result = {
        'version': version, 'build': build,
        'installer': {'name': installer.name, 'bytes': installer.stat().st_size, 'sha256': sha(installer)},
        'archive': {'path': archive.relative_to(ROOT).as_posix(), 'bytes': archive.stat().st_size, 'sha256': sha(archive)},
        'downloadsSha256': sha(page), 'metadataHistory': history.relative_to(ROOT).as_posix(),
        'files': [{'path': path.relative_to(web).as_posix(), 'bytes': path.stat().st_size, 'sha256': sha(path)} for path in files],
    }
    output = ROOT / f'tests/delivery/windows-{version.replace(".", "")}-web-bundle.json'
    assert not output.exists()
    output.write_text(json.dumps(result, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'manifest': str(output), 'installer': result['installer'], 'webFiles': len(files), 'webArchiveSha256': result['archive']['sha256']}))


if __name__ == '__main__':
    main()
