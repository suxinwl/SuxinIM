#!/usr/bin/env python3
"""Replace the owned HRM executable under the maintenance lock, with rollback."""
import argparse
import fcntl
import hashlib
import json
import os
import pathlib
import pwd
import re
import shutil
import stat
import subprocess
import time
import urllib.request
import uuid

BASE = pathlib.Path('/srv/puty-suite')
SUPERVISOR = '/www/server/panel/pyenv/bin/supervisorctl'


def require(ok, message):
    if not ok:
        raise RuntimeError(message)


def regular(path):
    require(path.resolve() == path and path.is_file() and not path.is_symlink()
            and stat.S_ISREG(path.stat().st_mode) and path.stat().st_nlink == 1,
            'Regular canonical file required')
    return path


def digest(path):
    with regular(path).open('rb') as handle:
        return hashlib.file_digest(handle, 'sha256').hexdigest()


def sync_directory(path):
    if os.name == 'posix':
        fd = os.open(path, os.O_RDONLY | os.O_DIRECTORY)
        try:
            os.fsync(fd)
        finally:
            os.close(fd)


def atomic_copy(source, target, expected, mode, uid=None, gid=None):
    require(target.parent.resolve() == target.parent and not target.is_symlink(), 'Executable path redirected')
    temporary = target.with_name('.' + target.name + '.publish-' + uuid.uuid4().hex)
    try:
        with regular(source).open('rb') as src, temporary.open('xb') as dst:
            shutil.copyfileobj(src, dst)
            dst.flush()
            os.fchmod(dst.fileno(), mode)
            if uid is not None:
                os.fchown(dst.fileno(), uid, gid)
            os.fsync(dst.fileno())
        require(digest(temporary) == expected, 'Executable changed while copying')
        temporary.replace(target)
        sync_directory(target.parent)
    finally:
        if temporary.exists():
            temporary.unlink()


def ready():
    try:
        opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
        with opener.open('http://127.0.0.1:18000/common/im-sso/captcha', timeout=4) as response:
            body = json.loads(response.read(1 << 20))
            return response.status == 200 and body.get('errCode', body.get('code')) == 0
    except Exception:
        return False


def restart_ready():
    subprocess.run([SUPERVISOR, 'restart', 'puty-hrm'], check=True, capture_output=True, timeout=90)
    for attempt in range(30):
        if ready():
            return
        time.sleep(2)
    raise RuntimeError('HRM captcha readiness did not recover')


def replace_host(source, target, backup, expected, proposed, uid, gid, postcheck=None):
    original = regular(target).stat()
    previous = backup / 'main'
    atomic_copy(target, previous, expected, 0o600)
    try:
        atomic_copy(source, target, proposed, 0o755, uid, gid)
        restart_ready()
        if postcheck is not None:
            postcheck()
    except BaseException:
        # copy2 does not preserve ownership: explicitly restore original metadata.
        atomic_copy(previous, target, expected, stat.S_IMODE(original.st_mode), original.st_uid, original.st_gid)
        restart_ready()
        require(digest(target) == expected, 'HRM rollback digest mismatch')
        raise


def main():
    parser = argparse.ArgumentParser()
    for name in ('main', 'sha256', 'expected-main'):
        parser.add_argument('--' + name, required=True)
    args = parser.parse_args()
    require(BASE.resolve() == BASE, 'Production root redirected')
    require(all(re.fullmatch(r'[0-9a-f]{64}', value) for value in (args.sha256, args.expected_main)),
            'Exact executable SHA256 required')
    lock_path = regular(BASE / '.backup-stage/backup.lock')
    with lock_path.open('a+b') as lock:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        marker = json.loads(regular(BASE / 'openim/instance.json').read_text())
        activation_raw = regular(BASE / 'activation.json').read_bytes()
        activation = json.loads(activation_raw)
        require(marker['instanceID'] == activation['instanceID'] == '5ac7c644d9cd4923e2b701f66b52485d'
                and marker['production'] is True and marker['testOnly'] is False
                and marker['stackName'] == 'puty-suite-prod' and marker['root'] == str(BASE / 'openim'),
                'Production ownership mismatch')
        subprocess.run(['python3', str(regular(BASE / 'openim/manage.py')), 'status'],
                       check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=60)
        source, target = pathlib.Path(args.main), BASE / 'gosuxin/main'
        require(source.is_relative_to(BASE / 'staging') and digest(source) == args.sha256
                and digest(target) == args.expected_main, 'Executable digest mismatch or source outside staging')
        with source.open('rb') as handle:
            header = handle.read(20)
        require(header[:6] == b'\x7fELF\x02\x01' and header[18:20] == b'\x3e\x00', 'Linux amd64 ELF required')
        backups = BASE / 'backups'
        require(backups.is_dir() and backups.resolve() == backups, 'Backup root redirected')
        backup = backups / ('hrm-host-' + time.strftime('%Y%m%dT%H%M%SZ', time.gmtime()) + '-' + uuid.uuid4().hex)
        backup.mkdir(mode=0o700)
        sync_directory(backups)
        owner = pwd.getpwnam('puty')
        replace_host(source, target, backup, args.expected_main, args.sha256, owner.pw_uid, owner.pw_gid,
                     lambda: require(regular(BASE / 'activation.json').read_bytes() == activation_raw,
                                     'Activation marker changed'))
        receipt = {'published': True, 'mainSHA256': digest(target), 'previousSHA256': args.expected_main,
                   'backup': str(backup), 'captchaReady': True, 'activationUnchanged': True}
        with (backup / 'release.json').open('x', encoding='utf-8') as handle:
            os.fchmod(handle.fileno(), 0o600)
            handle.write(json.dumps(receipt, indent=2) + '\n')
            handle.flush()
            os.fsync(handle.fileno())
        sync_directory(backup)
        print(json.dumps(receipt))


if __name__ == '__main__':
    main()
