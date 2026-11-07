#!/usr/bin/env python3
"""Apply an exact owned backend delta under the shared maintenance lock."""
import argparse
import fcntl
import hashlib
import json
import os
import pathlib
import shutil
import stat
import subprocess
import time
import uuid

BASE = pathlib.Path('/srv/puty-suite/openim')
STAGE = BASE.parent / 'staging'
ALLOWED = {'admin-api': 'admin-api', 'admin-rpc': 'chat-admin', 'chat-api': 'chat-api',
           'chat-rpc': 'chat-business', 'enterprise-service': 'enterprise-service',
           'openim-api': 'openim-api', 'openim-rpc-msg': 'openim-rpc-msg',
           'openim-msggateway': 'openim-msggateway', 'openim-rpc-group': 'openim-rpc-group'}


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
    require(target.parent.resolve() == target.parent and not target.is_symlink(), 'Backend path redirected')
    temporary = target.with_name('.' + target.name + '.next-' + uuid.uuid4().hex)
    try:
        with regular(source).open('rb') as src, temporary.open('xb') as dst:
            shutil.copyfileobj(src, dst)
            dst.flush()
            os.fchmod(dst.fileno(), mode)
            if uid is not None:
                os.fchown(dst.fileno(), uid, gid)
            os.fsync(dst.fileno())
        require(digest(temporary) == expected, 'Backend changed while copying')
        temporary.replace(target)
        sync_directory(target.parent)
    finally:
        if temporary.exists():
            temporary.unlink()


def docker_prefix():
    return ['docker', 'compose', '--project-directory', str(BASE), '-f', str(BASE / 'compose.yaml'),
            '-p', 'puty-suite-prod']


def run(arguments):
    return subprocess.run(arguments, check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=180)


def inspect_service(service, binary, owner, require_running=False):
    ids = subprocess.check_output(docker_prefix() + ['ps', '-a', '-q', service], timeout=30).decode().split()
    require(len(ids) == 1, 'Expected exactly one owned backend container')
    value = json.loads(subprocess.check_output(['docker', 'inspect', ids[0]], timeout=30))[0]
    labels = value['Config']['Labels']
    require(labels.get('com.docker.compose.project') == 'puty-suite-prod'
            and labels.get('com.docker.compose.service') == service and labels.get('com.puty.suite') == owner,
            'Backend container ownership mismatch')
    require(any(m.get('Type') == 'bind' and m.get('Source') == str(BASE / 'bin' / binary)
                and m.get('Destination') == '/app/bin/' + binary and m.get('RW') is False
                for m in value.get('Mounts', [])), 'Backend executable mount mismatch')
    state = value['State']
    return not require_running or (state.get('Running') is True
                                  and state.get('Health', {}).get('Status') not in ('starting', 'unhealthy'))


def wait_running(updates, owner):
    for attempt in range(30):
        if all(inspect_service(ALLOWED[n], n, owner, True) for n in updates):
            return
        time.sleep(2)
    raise RuntimeError('Backend containers did not reach running/healthy state')


def apply(updates, owner):
    manifest = regular(BASE / 'bin/backend-binaries.json')
    original_manifest = manifest.read_bytes()
    manifest_meta = manifest.stat()
    state = json.loads(original_manifest)
    metadata = {}
    for name, item in updates.items():
        require(name in ALLOWED and isinstance(item, dict) and set(item) == {'bytes', 'sha256'}
                and isinstance(item['bytes'], int) and 0 < item['bytes'] < 300 << 20
                and isinstance(item['sha256'], str) and len(item['sha256']) == 64
                and all(c in '0123456789abcdef' for c in item['sha256']), 'Invalid backend manifest entry')
        source, target = STAGE / (name + '.next'), BASE / 'bin' / name
        require(regular(source).stat().st_size == item['bytes'] and digest(source) == item['sha256'],
                'Backend candidate digest or size mismatch')
        with source.open('rb') as handle:
            header = handle.read(20)
        require(header[:6] == b'\x7fELF\x02\x01' and header[18:20] == b'\x3e\x00', 'Linux amd64 ELF required')
        require(name in state and isinstance(state[name], dict)
                and digest(target) == state[name].get('sha256'), 'Current backend differs from its manifest')
        metadata[name] = target.stat()
        inspect_service(ALLOWED[name], name, owner)
    backup_root = STAGE / 'backend-previous'
    require(backup_root.resolve() == backup_root and not backup_root.is_symlink(), 'Backup root redirected')
    backup_root.mkdir(mode=0o700, exist_ok=True)
    sync_directory(STAGE)
    previous = backup_root / uuid.uuid4().hex
    previous.mkdir(mode=0o700)
    sync_directory(backup_root)
    for name in updates:
        atomic_copy(BASE / 'bin' / name, previous / name, state[name]['sha256'], 0o600)
    manifest_backup = previous / 'backend-binaries.json'
    atomic_copy(manifest, manifest_backup, hashlib.sha256(original_manifest).hexdigest(), 0o600)
    services = [ALLOWED[n] for n in updates]
    compose = docker_prefix()
    restart_enterprise = 'enterprise-service' not in updates and any(
        name in updates for name in ('admin-api', 'admin-rpc', 'chat-api', 'chat-rpc'))
    try:
        # Even a partial stop failure must enter the verified compensation path.
        run(compose + ['stop', *services])
        for name, item in updates.items():
            old = metadata[name]
            atomic_copy(STAGE / (name + '.next'), BASE / 'bin' / name, item['sha256'],
                        0o755, old.st_uid, old.st_gid)
        run(compose + ['up', '-d', '--no-deps', '--no-build', '--force-recreate', *services])
        wait_running(updates, owner)
        if restart_enterprise:
            run(['python3', str(BASE / 'manage.py'), 'restart', 'enterprise-service'])
        proposed = {**state, **updates}
        draft = previous / 'new-backend-binaries.json'
        with draft.open('xb') as handle:
            handle.write((json.dumps(proposed, indent=2) + '\n').encode())
            handle.flush()
            os.fsync(handle.fileno())
        atomic_copy(draft, manifest, digest(draft), stat.S_IMODE(manifest_meta.st_mode),
                    manifest_meta.st_uid, manifest_meta.st_gid)
    except BaseException:
        # Stop all affected writers before restoring an executable inode.
        run(compose + ['stop', *services])
        for name in updates:
            old = metadata[name]
            atomic_copy(previous / name, BASE / 'bin' / name, state[name]['sha256'],
                        stat.S_IMODE(old.st_mode), old.st_uid, old.st_gid)
        atomic_copy(manifest_backup, manifest, hashlib.sha256(original_manifest).hexdigest(),
                    stat.S_IMODE(manifest_meta.st_mode), manifest_meta.st_uid, manifest_meta.st_gid)
        run(compose + ['up', '-d', '--no-deps', '--no-build', '--force-recreate', *services])
        wait_running(updates, owner)
        if restart_enterprise:
            run(['python3', str(BASE / 'manage.py'), 'restart', 'enterprise-service'])
        require(all(digest(BASE / 'bin' / n) == state[n]['sha256'] for n in updates), 'Backend rollback mismatch')
        raise
    return previous


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('manifest')
    args = parser.parse_args()
    require(BASE.resolve() == BASE and STAGE.resolve() == STAGE and STAGE.is_dir(), 'Production root redirected')
    lock_path = regular(BASE.parent / '.backup-stage/backup.lock')
    with lock_path.open('a+b') as lock:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        source = STAGE / args.manifest
        require(source.is_relative_to(STAGE), 'Backend manifest outside staging')
        updates = json.loads(regular(source).read_text())
        require(isinstance(updates, dict) and updates and all(n in ALLOWED for n in updates), 'Unknown backend')
        marker = json.loads(regular(BASE / 'instance.json').read_text())
        activation = json.loads(regular(BASE.parent / 'activation.json').read_text())
        owner = marker.get('instanceID')
        require(owner == activation.get('instanceID') == '5ac7c644d9cd4923e2b701f66b52485d'
                and marker.get('root') == str(BASE) and marker.get('production') is True
                and marker.get('testOnly') is False and marker.get('stackName') == 'puty-suite-prod',
                'Production ownership marker mismatch')
        regular(BASE / 'compose.yaml')
        run(['python3', str(regular(BASE / 'manage.py')), 'status'])
        previous = apply(updates, owner)
        print(json.dumps({'updated': list(updates), 'allSHA256Verified': True, 'containersRunning': True,
                          'backup': str(previous), 'applicationReadinessRequiresHTTPAcceptance': True}))


if __name__ == '__main__':
    main()
