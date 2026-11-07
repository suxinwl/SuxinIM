"""Apply the reviewed R6 UI-only repair on the existing Ubuntu installation."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import time
import urllib.error
import urllib.request
import zipfile

ROOT = Path('/srv/gosuxin-suite/shared/gosuxin/storage/plugins/biz-suite')
OLD = '555d92b87c0d8385820b64c4fe3090ae9a9ecdef5bc9e19735555c22ed5d14af'
NEW = '92d59c1d1e196606b108a89193b0bdccd02320f27a36da6940594693b98ec2ff'
ZIP_HASH = '37d00ca85fbf18a3e583f59e2f8939b197946f9b57eed43905aab3af688a0ae4'
OLD_JS = 'admin/assets/app-2RTXU3GO.js'
NEW_JS = 'admin/assets/app-hrm-null-scope-238b0f2f25c8.js'
SERVICE = 'gosuxin-hrm.service'

def digest(data):
    return hashlib.sha256(data).hexdigest()

def run(*args):
    return subprocess.run(args, check=True, capture_output=True, timeout=60)

def healthy(package):
    if subprocess.run(['systemctl', 'is-active', '--quiet', SERVICE]).returncode:
        return False
    worker = ROOT / 'packages' / package / 'bin/biz-suite'
    loaded = False
    for process in Path('/proc').iterdir():
        if not process.name.isdigit():
            continue
        try:
            command = Path(os.fsdecode((process / 'cmdline').read_bytes().split(b'\0')[0]))
            # The release working directory bind-mounts shared storage.
            if package in command.parts and command.samefile(worker):
                loaded = True
                break
        except (OSError, PermissionError):
            pass
    if not loaded:
        return False
    try:
        response = urllib.request.urlopen('http://127.0.0.1:19880/plugins/biz-suite/admin/hrm/attendance-rules', timeout=3)
        return response.status == 200
    except urllib.error.HTTPError as error:
        return error.code in (302, 401, 403)
    except (urllib.error.URLError, TimeoutError):
        return False

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('archive', type=Path)
    parser.add_argument('--apply', action='store_true')
    args = parser.parse_args()
    if os.geteuid() != 0:
        raise RuntimeError('Run under sudo to inspect and preserve package ownership')
    active = ROOT / 'active.json'
    if active.is_symlink() or ROOT.is_symlink():
        raise RuntimeError('Unexpected plugin path')
    prior = active.read_bytes()
    state = json.loads(prior)
    if state.get('package') != OLD or state.get('disabled'):
        raise RuntimeError('Active package changed; inspect before deploying')
    current = ROOT / 'packages' / OLD
    raw = (current / 'plugin.json').read_bytes()
    if digest(raw) != OLD:
        raise RuntimeError('Current manifest does not match its identity')
    old = json.loads(raw)
    for path, expected in old['files'].items():
        if digest((current / path).read_bytes()) != expected:
            raise RuntimeError('Current package was modified: ' + path)
    if digest(args.archive.read_bytes()) != ZIP_HASH:
        raise RuntimeError('Unexpected repair archive')
    with zipfile.ZipFile(args.archive) as archive:
        raw = archive.read('plugin.json')
        if digest(raw) != NEW:
            raise RuntimeError('Unexpected replacement manifest')
        new = json.loads(raw)
        if set(archive.namelist()) != set(new['files']) | {'plugin.json'} or len(archive.namelist()) != len(new['files']) + 1:
            raise RuntimeError('Archive has extra or duplicate members')
        contents = {}
        for path, expected in new['files'].items():
            relative = Path(path)
            if relative.is_absolute() or '..' in relative.parts or '\\' in path:
                raise RuntimeError('Invalid archive path')
            data = archive.read(path)
            if digest(data) != expected:
                raise RuntimeError('Archive member checksum mismatch')
            contents[path] = data
    if {k: v for k, v in old.items() if k not in ('version', 'files')} != {k: v for k, v in new.items() if k not in ('version', 'files')}:
        raise RuntimeError('Unreviewed manifest metadata change')
    expected_keys = set(old['files']) - {OLD_JS} | {NEW_JS}
    if set(contents) != expected_keys:
        raise RuntimeError('Unreviewed package file change')
    original_js = (current / OLD_JS).read_bytes()
    needle = b'i=new Set(r.map(String))'
    if original_js.count(needle) != 1 or contents[NEW_JS] != original_js.replace(needle, b'i=new Set(b(r).map(String))'):
        raise RuntimeError('JavaScript contains changes outside the reviewed null guard')
    original_html = (current / 'admin/index.html').read_bytes()
    if contents['admin/index.html'] != original_html.replace(OLD_JS.removeprefix('admin/').encode(), NEW_JS.removeprefix('admin/').encode()):
        raise RuntimeError('HTML contains changes outside the script reference')
    for path in contents:
        if path not in (NEW_JS, 'admin/index.html') and contents[path] != (current / path).read_bytes():
            raise RuntimeError('Non-UI file changed: ' + path)
    if not healthy(OLD):
        raise RuntimeError('Existing HRM service is not healthy')
    report = {'previous_package': OLD, 'package': NEW, 'version': new['version'], 'backend_unchanged': True, 'frontend_only': True, 'applied': False}
    if not args.apply:
        print(json.dumps(report))
        return
    backup = ROOT / 'backups' / ('attendance-null-' + time.strftime('%Y%m%d-%H%M%S'))
    backup.mkdir(parents=True, mode=0o700)
    (backup / 'active-before.json').write_bytes(prior)
    (backup / 'active-before.json').chmod(0o600)
    with zipfile.ZipFile(backup / 'package-before.zip', 'x', compression=zipfile.ZIP_DEFLATED) as archive:
        archive.write(current / 'plugin.json', 'plugin.json')
        for path in old['files']:
            archive.write(current / path, path)
    (backup / 'package-before.zip').chmod(0o600)
    target = ROOT / 'packages' / NEW
    target.mkdir(mode=0o700)
    for path, data in {**contents, 'plugin.json': raw}.items():
        destination = target / path
        destination.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
        destination.write_bytes(data)
        destination.chmod(0o700 if path == 'bin/biz-suite' else 0o600)
    for path in [target, *target.rglob('*')]:
        shutil.chown(path, 'gosuxin', 'gosuxin')
    def replace(data):
        temporary = active.with_name('active.attendance-null-next.json')
        with temporary.open('xb') as output:
            output.write(data)
        temporary.chmod(0o600)
        shutil.chown(temporary, 'gosuxin', 'gosuxin')
        os.replace(temporary, active)
    # Recheck after staging so another deployment cannot be overwritten.
    if active.read_bytes() != prior:
        raise RuntimeError('Active package changed while preparing the repair')
    try:
        run('systemctl', 'stop', SERVICE)
        replace(json.dumps({**state, 'package': NEW}).encode())
        run('systemctl', 'start', SERVICE)
        for _ in range(30):
            if healthy(NEW):
                break
            time.sleep(1)
        else:
            raise RuntimeError('Repaired HRM did not become healthy')
        report.update(applied=True, backup=str(backup), active_service=True)
    except Exception:
        replace(prior)
        run('systemctl', 'restart', SERVICE)
        for _ in range(30):
            if healthy(OLD):
                print(json.dumps({'rolled_back': True, 'package': OLD, 'backup': str(backup)}))
                break
            time.sleep(1)
        else:
            raise RuntimeError('Rollback package restored but service health needs inspection')
        raise
    (backup / 'repair-report.json').write_text(json.dumps(report, indent=2))
    print(json.dumps(report))

if __name__ == '__main__':
    main()
