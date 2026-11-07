#!/usr/bin/env python3
"""Publish one frozen entry-classification release under the suite maintenance lock."""
import argparse
import fcntl
import hashlib
import importlib.util
import json
import os
import pathlib
import pwd
import stat
import subprocess
import time
import uuid

BASE = pathlib.Path('/srv/puty-suite')
VHOST = pathlib.Path('/www/server/panel/vhost/nginx/puty-suite.conf')
NGINX = '/www/server/nginx/sbin/nginx'
INSTANCE = '5ac7c644d9cd4923e2b701f66b52485d'
NAMES = {'enterprise-service', 'openim-api', 'openim-msggateway', 'openim-rpc-msg', 'chat-api', 'admin-api'}


def require(ok, message):
    if not ok:
        raise RuntimeError(message)


def transition(ops):
    """Never run old producers with the new strict host after a failed deployment."""
    backend_started = False
    try:
        ops.nginx_new()
        backend_started = True  # stop may fail after stopping only some services
        ops.stop()
        ops.host_new()
        ops.backends_new()
        ops.start_and_check()
        ops.commit()
    except BaseException as original:
        try:
            if backend_started:
                # Do not replace mounted executables until every selected writer stops.
                ops.stop()
                ops.host_old()
                ops.backends_old()
                ops.start_and_check()
            ops.nginx_old()
            ops.verify_old()
        except BaseException as rollback:
            raise RuntimeError('Entry release failed; verified rollback incomplete') from rollback
        raise RuntimeError('Entry release failed; original release restored') from original


def load_helper(name, expected):
    path = BASE / 'staging' / (name + '.py')
    require(path.resolve() == path and path.is_file() and not path.is_symlink()
            and path.stat().st_uid == 0 and not path.stat().st_mode & 0o022
            and hashlib.sha256(path.read_bytes()).hexdigest() == expected, 'Frozen helper mismatch')
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


class Operations:
    def __init__(self, plan, backend, host, backup):
        self.plan, self.b, self.h, self.backup = plan, backend, host, backup
        self.manifest = BASE / 'openim/bin/backend-binaries.json'
        self.host = BASE / 'gosuxin/main'
        self.activation = (BASE / 'activation.json').read_bytes()
        self.old_manifest = json.loads(self.manifest.read_bytes())
        self.paths = {'nginx': VHOST, 'main': self.host, 'backend-binaries.json': self.manifest}
        self.paths.update({n: BASE / 'openim/bin' / n for n in plan['updates']})
        self.old = {}
        for name, path in self.paths.items():
            info = self.b.regular(path).stat()
            digest = self.b.digest(path)
            self.old[name] = (digest, stat.S_IMODE(info.st_mode), info.st_uid, info.st_gid)
            self.b.atomic_copy(path, backup / name, digest, 0o600)
        self.services = [self.b.ALLOWED[n] for n in plan['updates']]
        self.owner = pwd.getpwnam('puty')
        recovery = {'format': 'puty-entry-routing-recovery-1', 'instanceID': INSTANCE,
                    'activationSHA256': hashlib.sha256(self.activation).hexdigest(),
                    'paths': {n: str(p) for n, p in self.paths.items()},
                    'originals': {n: {'sha256': v[0], 'mode': v[1], 'uid': v[2], 'gid': v[3]}
                                  for n, v in self.old.items()},
                    'services': self.services, 'plan': plan}
        with (backup / 'recovery.json').open('x') as handle:
            os.fchmod(handle.fileno(), 0o600)
            json.dump(recovery, handle, indent=2)
            handle.flush()
            os.fsync(handle.fileno())
        self.b.sync_directory(backup)

    def nginx(self):
        for arguments in (['-t'], ['-s', 'reload']):
            subprocess.run([NGINX, *arguments], check=True, capture_output=True, timeout=30)

    def copy(self, source, name, expected=None):
        digest, mode, uid, gid = self.old[name]
        self.b.atomic_copy(source, self.paths[name], expected or digest, mode, uid, gid)

    def nginx_new(self):
        self.copy(BASE / 'staging/entry-routing-nginx.next', 'nginx', self.plan['nginx']['proposed'])
        self.nginx()

    def nginx_old(self):
        self.copy(self.backup / 'nginx', 'nginx')
        self.nginx()

    def stop(self):
        self.b.run(self.b.docker_prefix() + ['stop', *self.services])

    def host_new(self):
        self.b.atomic_copy(BASE / 'staging/entry-routing-hrm.next', self.host,
                           self.plan['host']['proposed'], 0o755, self.owner.pw_uid, self.owner.pw_gid)
        self.h.restart_ready()

    def host_old(self):
        self.copy(self.backup / 'main', 'main')
        self.h.restart_ready()

    def backends_new(self):
        for name, item in self.plan['updates'].items():
            self.copy(BASE / 'staging' / (name + '.next'), name, item['sha256'])

    def backends_old(self):
        for name in self.plan['updates']:
            self.copy(self.backup / name, name)
        self.copy(self.backup / 'backend-binaries.json', 'backend-binaries.json')

    def start_and_check(self):
        self.b.run(self.b.docker_prefix() + ['up', '-d', '--no-deps', '--no-build', '--force-recreate', *self.services])
        self.b.wait_running(self.plan['updates'], INSTANCE)
        for name in self.plan['updates']:
            ids = subprocess.check_output(self.b.docker_prefix() + ['ps', '-a', '-q', self.b.ALLOWED[name]], timeout=30).decode().split()
            require(len(ids) == 1, 'Running container ambiguous')
            state = json.loads(subprocess.check_output(['docker', 'inspect', ids[0]], timeout=30))[0]['State']
            self.check_process(state['Pid'], self.paths[name])
        host_pid = subprocess.check_output([self.h.SUPERVISOR, 'pid', 'puty-hrm'], timeout=30).decode().strip()
        self.check_process(host_pid, self.host)
        require(self.h.ready(), 'Host readiness lost')
        require((BASE / 'activation.json').read_bytes() == self.activation, 'Activation changed')

    def check_process(self, pid, target):
        require(str(pid).isdigit() and int(pid) > 1, 'Running process missing')
        with pathlib.Path('/proc', str(pid), 'exe').open('rb') as handle:
            actual = hashlib.file_digest(handle, 'sha256').hexdigest()
        require(actual == self.b.digest(target), 'Running executable differs from published file')

    def commit(self):
        candidate = self.backup / 'new-backend-binaries.json'
        with candidate.open('xb') as handle:
            handle.write((json.dumps({**self.old_manifest, **self.plan['updates']}, indent=2) + '\n').encode())
            handle.flush()
            os.fsync(handle.fileno())
        self.copy(candidate, 'backend-binaries.json', self.b.digest(candidate))
        require(self.b.digest(self.host) == self.plan['host']['proposed']
                and self.b.digest(VHOST) == self.plan['nginx']['proposed']
                and all(self.b.digest(self.paths[n]) == v['sha256'] for n, v in self.plan['updates'].items()),
                'Published digest mismatch')
        require((BASE / 'activation.json').read_bytes() == self.activation, 'Activation changed')

    def verify_old(self):
        for name, path in self.paths.items():
            info = path.stat()
            require((self.b.digest(path), stat.S_IMODE(info.st_mode), info.st_uid, info.st_gid) == self.old[name],
                    'Rollback bytes or ownership mismatch')
        require((BASE / 'activation.json').read_bytes() == self.activation, 'Activation changed')


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--plan', required=True)
    parser.add_argument('--sha256', required=True)
    args = parser.parse_args()
    require(os.geteuid() == 0 and BASE.resolve() == BASE, 'Owned root required')
    plan_path = BASE / 'staging' / args.plan
    require(plan_path.resolve().parent == BASE / 'staging' and not plan_path.is_symlink()
            and hashlib.sha256(plan_path.read_bytes()).hexdigest() == args.sha256, 'Exact frozen plan required')
    plan = json.loads(plan_path.read_bytes())
    b = load_helper('update_backend', plan['helpers']['backend'])
    h = load_helper('publish_hrm_host', plan['helpers']['host'])
    require(set(plan['updates']) == NAMES, 'Exact six-service release required')
    with b.regular(BASE / '.backup-stage/backup.lock').open('a+b') as lock:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        marker = json.loads(b.regular(BASE / 'openim/instance.json').read_bytes())
        activation = json.loads(b.regular(BASE / 'activation.json').read_bytes())
        require(marker.get('instanceID') == activation.get('instanceID') == INSTANCE
                and marker.get('production') is True and marker.get('testOnly') is False
                and marker.get('stackName') == 'puty-suite-prod' and marker.get('root') == str(BASE / 'openim'),
                'Production ownership mismatch')
        for directory in (BASE / 'staging', BASE / 'backups'):
            info = directory.stat()
            require(directory.resolve() == directory and info.st_uid == 0 and not info.st_mode & 0o022,
                    'Owned canonical directory required')
        b.run(['python3', str(b.regular(BASE / 'openim/manage.py')), 'status'])
        manifest_path = BASE / 'openim/bin/backend-binaries.json'
        require(b.digest(manifest_path) == plan['expectedManifest'], 'Current backend manifest changed')
        state = json.loads(manifest_path.read_bytes())
        for name, item in plan['updates'].items():
            require(name in b.ALLOWED and set(item) == {'bytes', 'sha256'}, 'Invalid update')
            source, target = BASE / 'staging' / (name + '.next'), BASE / 'openim/bin' / name
            require(b.regular(source).stat().st_size == item['bytes'] and b.digest(source) == item['sha256']
                    and b.digest(target) == state[name]['sha256'], 'Backend candidate or current digest mismatch')
            with source.open('rb') as handle:
                header = handle.read(20)
            require(header[:6] == b'\x7fELF\x02\x01' and header[18:20] == b'\x3e\x00', 'Linux amd64 ELF required')
            require(b.inspect_service(b.ALLOWED[name], name, INSTANCE, True), 'Current backend not running')
        require(b.digest(BASE / 'gosuxin/main') == plan['host']['expected']
                and b.digest(BASE / 'staging/entry-routing-hrm.next') == plan['host']['proposed']
                and b.digest(VHOST) == plan['nginx']['expected']
                and b.digest(BASE / 'staging/entry-routing-nginx.next') == plan['nginx']['proposed'],
                'Exact host and gateway candidates required')
        with (BASE / 'staging/entry-routing-hrm.next').open('rb') as handle:
            header = handle.read(20)
        require(header[:6] == b'\x7fELF\x02\x01' and header[18:20] == b'\x3e\x00', 'Host Linux amd64 ELF required')
        backup = BASE / 'backups' / ('entry-routing-' + time.strftime('%Y%m%dT%H%M%SZ', time.gmtime()) + '-' + uuid.uuid4().hex)
        backup.mkdir(mode=0o700)
        b.sync_directory(backup.parent)
        ops = Operations(plan, b, h, backup)
        transition(ops)
        receipt = {'published': True, 'backup': str(backup), 'hostSHA256': b.digest(ops.host),
                   'nginxSHA256': b.digest(VHOST), 'updates': plan['updates'], 'containersRunning': True,
                   'activationUnchanged': True, 'employeePoliciesModified': False,
                   'externalHTTPAcceptanceRequired': True}
        with (backup / 'release.json').open('x') as handle:
            os.fchmod(handle.fileno(), 0o600)
            json.dump(receipt, handle, indent=2)
            handle.flush()
            os.fsync(handle.fileno())
        b.sync_directory(backup)
        print(json.dumps(receipt))


if __name__ == '__main__':
    main()
