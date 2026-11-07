#!/usr/bin/env python3
"""One exact two-line CORS change; retain every other production vhost byte."""
import argparse
import hashlib
import json
import os
import pathlib
import re
import signal
import stat
import subprocess
import sys
import uuid

BASE = pathlib.Path('/srv/puty-suite')
INSTANCE = '5ac7c644d9cd4923e2b701f66b52485d'
VHOST = pathlib.Path('/www/server/panel/vhost/nginx/puty-suite.conf')
NGINX = '/www/server/nginx/sbin/nginx'
OLD_SHA = '7f5088344ee4a2df43fc8d8df78541f66ad2dba5a98576a9bef20e5b2e8f4213'
OLD = b" add_header Access-Control-Allow-Headers 'Content-Type, Authorization, token, chatToken, platformID, operationID, X-Requested-With, Range, X-Amz-Date, X-Amz-Content-Sha256' always;"
NEW = OLD.replace(b"' always;", b", X-Puty-Resource-Proof' always;")


def require(value, code):
    if not value:
        raise RuntimeError(code)


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def canonical(value):
    return (json.dumps(value, sort_keys=True, separators=(',', ':')) + '\n').encode()


def candidate(raw):
    require(sha(raw) == OLD_SHA and raw.count(OLD) == 2 and b'X-Puty-Resource-Proof' not in raw,
            'exact_original_vhost_required')
    changed = raw.replace(OLD, NEW)
    require(changed.count(NEW) == 2 and changed.replace(NEW, OLD) == raw,
            'only_two_header_appends_allowed')
    return changed


def regular(path):
    info = path.lstat()
    require(path.resolve() == path and stat.S_ISREG(info.st_mode) and info.st_nlink == 1
            and info.st_uid == 0 and not info.st_mode & 0o022, 'owned_regular_file_required')
    return info


def metadata(path):
    info = regular(path)
    return {'sha256': sha(path.read_bytes()), 'mode': stat.S_IMODE(info.st_mode), 'uid': info.st_uid,
            'gid': info.st_gid, 'bytes': info.st_size}


def sync(path):
    if os.name == 'posix':
        handle = os.open(path, os.O_RDONLY | os.O_DIRECTORY)
        try:
            os.fsync(handle)
        finally:
            os.close(handle)


def atomic(path, raw, mode=0o600, uid=0, gid=0):
    require(path.parent.resolve() == path.parent and not path.is_symlink(), 'write_path_redirected')
    temporary = path.with_name('.' + path.name + '.next-' + uuid.uuid4().hex)
    try:
        with temporary.open('xb') as target:
            os.chmod(temporary, mode)
            os.chown(temporary, uid, gid)
            target.write(raw)
            target.flush()
            os.fsync(target.fileno())
        temporary.replace(path)
        sync(path.parent)
    finally:
        temporary.unlink(missing_ok=True)


def guard():
    require(os.geteuid() == 0 and BASE.resolve() == BASE, 'root_owned_suite_required')
    marker = json.loads((BASE / 'openim/instance.json').read_bytes()) if regular(BASE / 'openim/instance.json') else None
    activation = json.loads((BASE / 'activation.json').read_bytes()) if regular(BASE / 'activation.json') else None
    require(marker.get('instanceID') == activation.get('instanceID') == INSTANCE and marker.get('production') is True
            and marker.get('testOnly') is False and marker.get('stackName') == 'puty-suite-prod'
            and marker.get('root') == str(BASE / 'openim'), 'owner_marker_mismatch')
    for name in ('staging', 'backups'):
        directory = BASE / name
        info = directory.lstat()
        require(directory.resolve() == directory and stat.S_ISDIR(info.st_mode) and info.st_uid == 0
                and not info.st_mode & 0o022, 'owned_directory_required')


def plan():
    guard()
    current = metadata(VHOST)
    return {'format': 'puty-resource-proof-cors-1', 'instanceID': INSTANCE, 'current': current,
            'publisherSHA256': sha(pathlib.Path(__file__).read_bytes()),
            'candidateSHA256': sha(candidate(VHOST.read_bytes())), 'headerAppends': 2}


def nginx(*arguments):
    value = subprocess.run([NGINX, *arguments], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=30)
    require(value.returncode == 0, 'nginx_control_failed')


def restore(transaction, value):
    guard()
    raw = (transaction / 'previous.conf').read_bytes()
    require(sha(raw) == value['current']['sha256'] == OLD_SHA, 'rollback_original_hash_mismatch')
    old = value['current']
    atomic(VHOST, raw, old['mode'], old['uid'], old['gid'])
    nginx('-t')
    nginx('-s', 'reload')
    require(metadata(VHOST) == old, 'rollback_owner_mode_or_hash_mismatch')


def apply(value):
    require(value == plan(), 'reviewed_plan_changed')
    transaction = BASE / 'backups' / ('resource-proof-cors-' + uuid.uuid4().hex)
    transaction.mkdir(mode=0o700)
    sync(transaction.parent)
    atomic(transaction / 'previous.conf', VHOST.read_bytes())
    atomic(transaction / 'plan.json', canonical(value))
    atomic(transaction / 'journal.json', canonical({'instanceID': INSTANCE, 'status': 'prepared'}))
    old = value['current']
    try:
        atomic(VHOST, candidate(VHOST.read_bytes()), old['mode'], old['uid'], old['gid'])
        nginx('-t')
        nginx('-s', 'reload')
        require(metadata(VHOST) == {**old, 'sha256': value['candidateSHA256'],
                'bytes': old['bytes'] + 2 * (len(NEW) - len(OLD))}, 'published_metadata_mismatch')
        atomic(transaction / 'journal.json', canonical({'instanceID': INSTANCE, 'status': 'committed'}))
    except BaseException:
        try:
            restore(transaction, value)
            atomic(transaction / 'journal.json', canonical({'instanceID': INSTANCE, 'status': 'rolledBack'}))
        except BaseException:
            raise RuntimeError('cors_failed_rollback_requires_operator') from None
        raise RuntimeError('cors_failed_verified_rollback_complete') from None
    return {'passed': True, 'transaction': str(transaction), 'sha256': value['candidateSHA256'],
            'headerAppends': 2, 'requiresCorsPreflightAcceptance': True}


def staged(relative):
    require(isinstance(relative, str) and re.fullmatch('[A-Za-z0-9][A-Za-z0-9_./-]{0,200}', relative)
            and all(part not in ('', '.', '..') for part in relative.split('/')), 'staging_path_invalid')
    path = BASE / 'staging' / relative
    require(path.resolve() == path and path.is_relative_to(BASE / 'staging'), 'staging_path_invalid')
    return path


def main():
    parser = argparse.ArgumentParser()
    sub = parser.add_subparsers(dest='command', required=True)
    create = sub.add_parser('plan')
    create.add_argument('--out', required=True)
    use = sub.add_parser('apply')
    use.add_argument('--plan', required=True)
    use.add_argument('--plan-sha256', required=True)
    args = parser.parse_args()
    import fcntl
    guard()
    lock = BASE / '.backup-stage/backup.lock'
    regular(lock)
    with lock.open('rb') as handle:
        fcntl.flock(handle, fcntl.LOCK_EX | fcntl.LOCK_NB)
        if args.command == 'plan':
            value = plan()
            out = staged(args.out)
            require(not out.exists(), 'plan_already_exists')
            atomic(out, canonical(value))
            print(json.dumps({'planned': True, 'planSHA256': sha(out.read_bytes()), **value}))
        else:
            path = staged(args.plan)
            regular(path)
            raw = path.read_bytes()
            require(sha(raw) == args.plan_sha256, 'plan_hash_mismatch')
            for signum in (signal.SIGHUP, signal.SIGINT, signal.SIGTERM):
                signal.signal(signum, lambda *_: (_ for _ in ()).throw(InterruptedError('signal')))
            print(json.dumps(apply(json.loads(raw))))


if __name__ == '__main__':
    try:
        main()
    except BaseException as error:
        code = str(error) if type(error) is RuntimeError and re.fullmatch('[a-z0-9_]+', str(error)) else 'cors_dependency_or_input_error'
        print(json.dumps({'passed': False, 'error': code, 'credentialsPrinted': False}))
        sys.exit(1)
