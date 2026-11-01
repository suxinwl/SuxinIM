"""Linux: update only the isolated download status page after hash guards."""
import argparse
import datetime
import hashlib
import json
import os
import pathlib
import re
import shutil


def sha(path):
    with path.open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--previous-sha', required=True)
    parser.add_argument('--next-sha', required=True)
    args = parser.parse_args()
    assert all(re.fullmatch('[a-f0-9]{64}', value) for value in vars(args).values())
    base = pathlib.Path('/srv/openim38-upgrade')
    target = base / 'www/downloads/index.html'
    staged = pathlib.Path('/tmp/openim38-download-status.next')
    for path in [base, base / 'www', base / 'www/downloads', target, base / 'backups']:
        assert path.resolve() == path and not path.is_symlink()
    assert target.is_file() and staged.is_file()
    assert sha(target) == args.previous_sha and sha(staged) == args.next_sha
    stamp = datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%SZ')
    backup = base / f'backups/download-status-{stamp}'
    backup.mkdir(mode=0o700, exist_ok=False)
    shutil.copy2(target, backup / 'index.html')
    assert sha(backup / 'index.html') == args.previous_sha
    temporary = target.with_name('index.html.status-next')
    assert not temporary.exists()
    with staged.open('rb') as source, temporary.open('xb') as destination:
        shutil.copyfileobj(source, destination)
    os.chmod(temporary, 0o644)
    assert sha(temporary) == args.next_sha
    os.replace(temporary, target)
    result = {'utc': stamp, 'updated': str(target), 'previousSha256': args.previous_sha,
              'sha256': sha(target), 'backup': str(backup),
              'scope': 'Status page only; no package, update record, service, account, TLS, network or production-instance changes.'}
    (backup / 'report.json').write_text(json.dumps(result, indent=2) + '\n')
    output = pathlib.Path('/tmp/openim38-download-status-report.json')
    output.write_text(json.dumps(result, indent=2) + '\n')
    os.chmod(output, 0o644)
    print(json.dumps(result))


if __name__ == '__main__':
    main()
