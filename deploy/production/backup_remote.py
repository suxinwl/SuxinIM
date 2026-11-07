#!/usr/bin/env python3
"""Production backup worker, streamed over pinned SSH by production_backup.py.

Stdout is binary archive/report only. No recovery encryption key is accepted by
this worker. Production restore is intentionally not implemented.
"""
import argparse
import contextlib
import datetime
try:
    import fcntl
except ImportError:  # Pure archive/identity helpers are also tested on Windows.
    fcntl = None
import hashlib
import gzip
import io
import json
import os
import pathlib
import re
import secrets
import shutil
import signal
import stat
import subprocess
import sys
import tarfile
import time

BASE = pathlib.Path('/srv/puty-suite')
IM = BASE / 'openim'
PROJECT = 'puty-suite-prod'
SUPERVISOR = '/www/server/panel/pyenv/bin/supervisorctl'
STORAGE = {'mysql', 'mongo', 'redis', 'etcd', 'kafka', 'minio'}
EXPECTED_DBS = ['openim38', 'chat38', 'enterprise38']
FORMAT = 'puty-production-backup-v2'
EXTERNAL = [
    '/www/server/panel/plugin/supervisor/profile/puty-hrm.ini',
    '/etc/supervisor/supervisord.conf',
    '/www/server/panel/vhost/nginx/puty-suite.conf',
    '/www/server/panel/vhost/nginx/tcp/puty-turn.conf',
]
MYSQL_AUTH = 'export MYSQL_PWD="$MYSQL_ROOT_PASSWORD"; '
MONGO_AUTH = '--host=127.0.0.1 --username="$MONGO_INITDB_ROOT_USERNAME" --password="$MONGO_INITDB_ROOT_PASSWORD" --authenticationDatabase=admin'
DB_COUNTS = '''const out={}; for(const n of %s){const d=db.getSiblingDB(n);out[n]={};for(const c of d.getCollectionNames().filter(x=>!x.startsWith("system."))){out[n][c]=d.getCollection(c).countDocuments({});}} print(JSON.stringify(out))'''


def require(condition, message):
    if not condition:
        raise RuntimeError(message)


def canonical(value):
    return json.dumps(value, sort_keys=True, separators=(',', ':'), ensure_ascii=False).encode()


def utc():
    return datetime.datetime.now(datetime.timezone.utc).isoformat()


def event(name, **data):
    print(json.dumps({'event': name, **data}), file=sys.stderr, flush=True)


def run(args, *, source=None, output=None, timeout=180):
    result = subprocess.run(args, stdin=source, stdout=output or subprocess.PIPE,
                            stderr=subprocess.PIPE, timeout=timeout)
    require(result.returncode == 0, 'A required subprocess failed; output withheld to protect credentials')
    return result.stdout


def js(args):
    return json.loads(run(args))


def contained(path, root):
    return path.resolve().is_relative_to(root.resolve())


def validate_identity(info, root=IM):
    require(info.get('root') == root.as_posix() and info.get('production') is True
            and info.get('testOnly') is False and info.get('stackName') == PROJECT
            and re.fullmatch('[a-f0-9]{32}', info.get('instanceID', '')),
            'Production ownership marker mismatch')


def validate_activation(activation, runtime, owner):
    require(isinstance(activation, dict) and activation.get('instanceID') == owner
            and activation.get('dataPolicy') == 'fresh-business-data',
            'Activation ownership or data policy mismatch')
    stamp = activation.get('startAt')
    require(isinstance(stamp, str) and stamp and runtime.get('GOSUXIN_SYNC_START_AT') == stamp
            and runtime.get('GOSUXIN_REHEARSAL') == 'false',
            'Activation and runtime cutoff mismatch')
    try:
        parsed = datetime.datetime.fromisoformat(stamp.replace('Z', '+00:00'))
    except ValueError:
        raise RuntimeError('Invalid activation timestamp') from None
    require(parsed.tzinfo is not None and parsed.utcoffset() is not None,
            'Activation timestamp must include its timezone')
    return activation


def read_activation(root, owner):
    values = []
    for relative in ('activation.json', 'gosuxin/runtime-environment.json'):
        source = root / relative
        require(source.is_file() and not source.is_symlink() and contained(source, root)
                and source.stat().st_size <= 1024 * 1024, 'Required regular activation/runtime file missing')
        values.append(json.loads(source.read_text()))
    return validate_activation(*values, owner)


def restored_activation(root, manifest):
    require(isinstance(manifest.get('activation'), dict), 'Backup activation metadata missing')
    activation = read_activation(root / 'runtime', manifest['instanceID'])
    require(activation == manifest['activation'], 'Restored activation differs from manifest')
    owner_file = root / 'runtime/openim/instance.json'
    require(owner_file.is_file() and not owner_file.is_symlink(), 'Restored owner marker missing')
    owner = json.loads(owner_file.read_text())
    validate_identity(owner)
    require(owner['instanceID'] == manifest['instanceID'], 'Restored activation owner mismatch')
    return activation


class Production:
    def __init__(self):
        require(BASE.is_dir() and BASE.resolve() == BASE and IM.resolve() == IM,
                'Production root missing or redirected')
        require((BASE / 'gosuxin').is_dir() and (BASE / 'gosuxin').resolve() == BASE / 'gosuxin',
                'HRM runtime root missing or redirected')
        marker = IM / 'instance.json'
        require(marker.is_file() and not marker.is_symlink(), 'Missing regular owner marker')
        self.info = json.loads(marker.read_text())
        validate_identity(self.info)
        self.owner = self.info['instanceID']
        self.activation = read_activation(BASE, self.owner)
        self.cli = ['docker', 'compose', '--project-directory', str(IM), '-f',
                    str(IM / 'compose.yaml'), '-p', PROJECT]
        self.spec = js(self.cli + ['config', '--format', 'json'])
        require(self.spec.get('name') == PROJECT, 'Wrong Compose project')
        for service in self.spec['services'].values():
            for mount in service.get('volumes', []):
                if mount['type'] == 'bind':
                    require(contained(pathlib.Path(mount['source']), BASE), 'Suite bind mount leaves the owned runtime root')
        for filename in EXTERNAL:
            source = pathlib.Path(filename)
            require(source.is_file() and not source.is_symlink(), 'Required host configuration missing before backup')
        for relative in ('openim/secrets.json', 'gosuxin/runtime-environment.json',
                         'gosuxin/manifest/config/config.yaml', 'gosuxin/storage/plugins/biz-suite/data'):
            require((BASE / relative).exists() and contained(BASE / relative, BASE),
                    'Required private runtime material is absent')
        ids = run(['docker', 'ps', '-aq', '--filter', 'label=com.docker.compose.project=' + PROJECT]).decode().split()
        self.containers = js(['docker', 'inspect', *ids]) if ids else []
        self.by_service = {}
        for item in self.containers:
            labels = item['Config'].get('Labels') or {}
            require(labels.get('com.puty.suite') == self.owner, 'Foreign production container')
            name = labels.get('com.docker.compose.service')
            require(name in self.spec['services'] and name not in self.by_service,
                    'Unexpected or duplicate production service container')
            self.by_service[name] = item
        require(STORAGE <= self.by_service.keys(), 'Required storage containers are absent')
        self.volumes = {}
        for logical, definition in self.spec['volumes'].items():
            name = definition.get('name')
            require(name and re.fullmatch(r'[A-Za-z0-9_.-]+', name), 'Unexpected Docker volume name')
            item = js(['docker', 'volume', 'inspect', name])[0]
            require((item.get('Labels') or {}).get('com.puty.suite') == self.owner,
                    'Foreign storage volume')
            path = pathlib.Path(item['Mountpoint'])
            require(path.is_dir() and path.resolve() == path and str(path).startswith('/var/lib/docker/volumes/'),
                    'Unexpected Docker local volume location')
            self.volumes[logical] = {'name': name, 'path': path}
        require(set(self.volumes) == {name + '-data' for name in STORAGE}, 'Unexpected storage volume set')
        for service in STORAGE:
            actual = self.by_service[service]['Mounts']
            require(any(m.get('Name') == self.volumes[service + '-data']['name'] for m in actual),
                    'Storage container and volume identity differ')
        require(self.spec['services']['mysql']['environment']['MYSQL_DATABASE'] == 'gosuxin_prod',
                'Unexpected HRM database')
        # Parse only a scalar database name, never load or print connection secrets.
        mongo_dbs = []
        for component in ('openim-server', 'chat'):
            text = (IM / 'config' / component / 'mongodb.yml').read_text()
            match = re.search(r'^database:\s*[\'\"]?([a-zA-Z0-9_]+)[\'\"]?\s*$', text, re.M)
            require(match, 'MongoDB database name missing in private config')
            mongo_dbs.append(match.group(1))
        mongo_dbs.append(self.spec['services']['enterprise-service']['environment']['MONGO_DATABASE'])
        require(mongo_dbs == EXPECTED_DBS, 'Unexpected production MongoDB databases')
        self.mongo_dbs = mongo_dbs
        self.stage_root = BASE / '.backup-stage'
        require(not self.stage_root.is_symlink(), 'Linked backup staging directory refused')
        self.stage_root.mkdir(mode=0o700, exist_ok=True)
        self.stage_root.chmod(0o700)

    def exec(self, service, command, *args, source=None, output=None):
        return run(['docker', 'exec', *(['-i'] if source else []),
                    self.by_service[service]['Id'], 'sh', '-ec', command, '--', *args],
                   source=source, output=output, timeout=900)

    def counts(self):
        table_names = self.exec('mysql', MYSQL_AUTH + 'exec mysql -uroot --batch --skip-column-names -e '
                               '"SELECT TABLE_NAME FROM information_schema.tables WHERE TABLE_SCHEMA=\'gosuxin_prod\' AND TABLE_TYPE=\'BASE TABLE\' ORDER BY TABLE_NAME"')
        names = table_names.decode().splitlines()
        require(all(re.fullmatch('[A-Za-z0-9_]+', t) for t in names), 'Unsafe MySQL table name')
        sql = ';'.join("SELECT '" + t + "',COUNT(*) FROM `gosuxin_prod`.`" + t + '`' for t in names)
        rows = self.exec('mysql', MYSQL_AUTH + 'exec mysql -uroot --batch --skip-column-names -e "$1"', sql) if names else b''
        mysql = {line.split('\t')[0]: int(line.split('\t')[1]) for line in rows.decode().splitlines()}
        mongo = json.loads(self.exec('mongo', 'exec mongosh --quiet ' + MONGO_AUTH + ' --eval "$1"', DB_COUNTS % json.dumps(self.mongo_dbs)))
        return {'mysql': mysql, 'mongo': mongo}


def owned_container(identifier, owner):
    item = js(['docker', 'inspect', identifier])[0]
    labels = item['Config'].get('Labels') or {}
    require(labels.get('com.puty.suite') == owner and labels.get('com.docker.compose.project') == PROJECT,
            'Refusing to operate a foreign container')
    return item


def resume(state):
    # Original container IDs only; never creates containers or starts absent services.
    for group in (state['storage'], state['writers']):
        for identifier in group:
            item = owned_container(identifier, state['owner'])
            if not item['State']['Running']:
                run(['docker', 'start', identifier], timeout=180)
        if group is state['storage']:
            deadline = time.monotonic() + 240
            for identifier in group:
                while True:
                    item = owned_container(identifier, state['owner'])
                    require(item['State']['Running'], 'Storage exited during resumption')
                    health = item['State'].get('Health', {}).get('Status')
                    if health is None or health == 'healthy':
                        break
                    require(time.monotonic() < deadline, 'Storage health did not recover')
                    time.sleep(2)
    if state['hrm']:
        status = subprocess.run([SUPERVISOR, 'status', 'puty-hrm'], capture_output=True)
        if b' RUNNING ' not in status.stdout:
            run([SUPERVISOR, 'start', 'puty-hrm'], timeout=90)
    for identifier in state['storage'] + state['writers']:
        require(owned_container(identifier, state['owner'])['State']['Running'], 'Original service failed to resume')


def watchdog(state, done, seconds):
    parent = os.getpid()
    pid = os.fork()
    if pid:
        return pid
    try:
        os.setsid()
        with open(os.devnull, 'r+b', buffering=0) as sink:
            for descriptor in (0, 1, 2):
                os.dup2(sink.fileno(), descriptor)
        end = time.monotonic() + seconds
        while not done.exists():
            try:
                os.kill(parent, 0)
                alive = True
            except ProcessLookupError:
                alive = False
            if not alive or time.monotonic() >= end:
                if alive:
                    os.kill(parent, signal.SIGTERM)
                    time.sleep(2)
                for _ in range(3):
                    try:
                        resume(state)
                        done.write_text('watchdog resumed services\n')
                        break
                    except Exception:
                        time.sleep(5)
                break
            time.sleep(1)
    finally:
        os._exit(0)


def safe_name(name):
    path = pathlib.PurePosixPath(name)
    require(bool(name) and not path.is_absolute() and '..' not in path.parts
            and '\\' not in name and '\x00' not in name, 'Unsafe archive path')
    return path


def symlink_target(name, target):
    require(target and not pathlib.PurePosixPath(target).is_absolute() and '\\' not in target and '\x00' not in target,
            'Absolute archive symlink refused')
    parts = list(safe_name(name).parent.parts)
    for part in pathlib.PurePosixPath(target).parts:
        if part == '..':
            require(len(parts) > 1, 'Escaping archive symlink refused')
            parts.pop()
        elif part != '.':
            parts.append(part)
    return pathlib.PurePosixPath(*parts)


def validate_symlink_graph(links):
    """Resolve the complete archive graph before creating any real links.

    A lexical normpath is insufficient: an earlier link changes the meaning of
    later '..' components. Keep the original components until links expand.
    """
    resolved = {}

    def resolve(parts, active, budget):
        current = []
        for part in parts:
            if part in ('', '.'):
                continue
            if part == '..':
                require(len(current) > 1, 'Symlink graph escapes archive namespace')
                current.pop()
                continue
            current.append(part)
            name = pathlib.PurePosixPath(*current).as_posix()
            if name in links:
                require(name not in active, 'Symlink graph loop refused')
                budget[0] += 1
                require(budget[0] <= 40, 'Symlink graph exceeds resolution limit')
                target = links[name]
                require(target and not pathlib.PurePosixPath(target).is_absolute()
                        and '\\' not in target and '\x00' not in target,
                        'Absolute or malformed symlink refused')
                current = list(resolve((*current[:-1], *pathlib.PurePosixPath(target).parts),
                                       active | {name}, budget))
        return tuple(current)

    for name in links:
        destination = resolve(pathlib.PurePosixPath(name).parts, set(), [0])
        require(destination and destination[0] == pathlib.PurePosixPath(name).parts[0],
                'Symlink graph escapes archive namespace')
        resolved[name] = destination
    return {name: pathlib.PurePosixPath(*parts) for name, parts in resolved.items()}


class HashReader:
    def __init__(self, source):
        self.source = source
        self.hash = hashlib.sha256()

    def read(self, size=-1):
        data = self.source.read(size)
        self.hash.update(data)
        return data


def excluded_source(source, name):
    if name in {'runtime/openim/logs', 'runtime/gosuxin/runtime/log'}:
        return True
    # The image leaves this non-data socket link in its volume after shutdown.
    # Never archive its absolute host target or broaden this to arbitrary links.
    return (name == 'volumes/mysql-data/mysql.sock' and source.is_symlink()
            and os.readlink(source) == '/var/run/mysqld/mysqld.sock')


def preflight_source(source, name, root):
    if excluded_source(source, name):
        return
    safe_name(name)
    mode = source.lstat().st_mode
    require(stat.S_ISDIR(mode) or stat.S_ISREG(mode) or stat.S_ISLNK(mode),
            'Special filesystem entry refused')
    if stat.S_ISLNK(mode):
        require(contained(source, root), 'Source symlink leaves its backup scope')
        symlink_target(name, os.readlink(source))
    elif stat.S_ISDIR(mode):
        for child in sorted(source.iterdir()):
            preflight_source(child, name + '/' + child.name, root)


def preflight_production(production):
    # Validate metadata before downtime; capture still repeats all guards.
    for name in ('openim', 'gosuxin'):
        preflight_source(BASE / name, 'runtime/' + name, BASE / name)
    preflight_source(BASE / 'activation.json', 'runtime/activation.json', BASE)
    for filename in EXTERNAL:
        source = pathlib.Path(filename)
        preflight_source(source, 'host-config/' + filename.lstrip('/'), source.parent)
    for name, volume in sorted(production.volumes.items()):
        preflight_source(volume['path'], 'volumes/' + name, volume['path'])


def archive_path(archive, source, name, inventory, root):
    if excluded_source(source, name):
        return
    safe_name(name)
    info = archive.gettarinfo(str(source), arcname=name)
    info.mode &= 0o7777  # Tar serializes permission bits, not stat file-type bits.
    if info.islnk():  # Store each regular file independently of inode aliases.
        info.type, info.linkname, info.size = tarfile.REGTYPE, '', source.stat().st_size
    require(info.isdir() or info.isfile() or info.issym(), 'Special filesystem entry refused')
    row = {'path': name, 'mode': info.mode, 'uid': info.uid, 'gid': info.gid,
           'type': 'directory' if info.isdir() else 'symlink' if info.issym() else 'file'}
    if info.issym():
        require(contained(source, root), 'Source symlink leaves its backup scope')
        symlink_target(name, info.linkname)
        row['target'] = info.linkname
        archive.addfile(info)
    elif info.isfile():
        with source.open('rb') as stream:
            hashed = HashReader(stream)
            archive.addfile(info, hashed)
        row.update(bytes=info.size, sha256=hashed.hash.hexdigest())
    else:
        archive.addfile(info)
    inventory.append(row)
    if info.isdir():
        for child in sorted(source.iterdir()):
            child_name = name + '/' + child.name
            archive_path(archive, child, child_name, inventory, root)


def create(max_stop):
    production = Production()
    lock_file = (production.stage_root / 'backup.lock').open('a+b')
    fcntl.flock(lock_file, fcntl.LOCK_EX | fcntl.LOCK_NB)
    # Re-read after acquiring the shared deployment/maintenance lock.
    activation = read_activation(BASE, production.owner)
    require(activation == production.activation, 'Activation changed before capture')
    require(all(production.by_service[x]['State']['Running'] for x in STORAGE), 'Storage services must initially be running')
    event('preflight-source-metadata')
    preflight_production(production)
    identifier = 'backup-' + datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%SZ') + '-' + secrets.token_hex(8)
    stage = production.stage_root / identifier
    stage.mkdir(mode=0o700)
    storage, writers = [], []
    for name, item in production.by_service.items():
        if item['State']['Running']:
            (storage if name in STORAGE else writers).append(item['Id'])
    status = subprocess.run([SUPERVISOR, 'status', 'puty-hrm'], capture_output=True, timeout=20)
    require(b'puty-hrm' in status.stdout, 'HRM supervisor program is absent')
    state = {'owner': production.owner, 'storage': storage, 'writers': writers,
             'hrm': b' RUNNING ' in status.stdout, 'created': utc()}
    (stage / 'recovery.json').write_bytes(canonical(state))
    done = stage / 'services-resumed'
    child = watchdog(state, done, max_stop)
    resumed = False
    def timeout_signal(_signum, _frame):
        raise TimeoutError('Backup maximum stop interval expired or process interrupted')
    signal.signal(signal.SIGTERM, timeout_signal)
    signal.signal(signal.SIGINT, timeout_signal)
    signal.signal(signal.SIGALRM, timeout_signal)
    signal.alarm(max_stop)
    try:
        event('stopping-writers', maximumSeconds=max_stop)
        if state['hrm']:
            run([SUPERVISOR, 'stop', 'puty-hrm'], timeout=90)
        if writers:
            run(['docker', 'stop', '--time', '30', *writers], timeout=180)
        require(all(not owned_container(x, production.owner)['State']['Running'] for x in writers), 'Writer still running')
        before = production.counts()
        event('logical-exports')
        with (stage / 'mysql.sql').open('wb') as output:
            production.exec('mysql', MYSQL_AUTH + 'exec mysqldump -uroot --single-transaction --routines --events --triggers --hex-blob --set-gtid-purged=OFF --no-tablespaces --databases "$MYSQL_DATABASE"', output=output)
        for name in production.mongo_dbs:
            with (stage / ('mongo-' + name + '.archive.gz')).open('wb') as output:
                production.exec('mongo', 'exec mongodump --quiet ' + MONGO_AUTH + ' --db "$1" --archive --gzip', name, output=output)
        after = production.counts()
        require(before == after, 'Quiesced database counts changed')
        event('stopping-storage')
        run(['docker', 'stop', '--time', '45', *storage], timeout=180)
        require(all(not owned_container(x, production.owner)['State']['Running'] for x in storage + writers), 'A source container restarted during quiescence')
        images = {name: item['Image'] for name, item in production.by_service.items()}
        inventory = []
        manifest = {'format': FORMAT, 'archiveID': identifier, 'created': state['created'], 'instanceID': production.owner,
                    'activation': activation,
                    'consistency': 'all-suite-writers-and-storage-stopped', 'mysqlDatabase': 'gosuxin_prod',
                    'mongoDatabases': production.mongo_dbs, 'counts': after, 'images': images,
                    'volumes': sorted(production.volumes), 'files': inventory,
                    'excluded': ['application logs', 'exact MySQL transient socket symlink',
                                 'backup staging', 'independent recovery encryption key']}
        event('streaming-cold-backup')
        # Prefer the shortest writer outage over maximum ELF compression.
        # An explicit GzipFile keeps level selection compatible with Python 3.9.
        with gzip.GzipFile(fileobj=sys.stdout.buffer, mode='wb', compresslevel=1) as compressed, \
                tarfile.open(fileobj=compressed, mode='w|', dereference=False) as archive:
            for path in sorted(stage.glob('*.sql')) + sorted(stage.glob('*.gz')):
                archive_path(archive, path, 'logical/' + path.name, inventory, stage)
            for name in ('openim', 'gosuxin'):
                archive_path(archive, BASE / name, 'runtime/' + name, inventory, BASE / name)
            archive_path(archive, BASE / 'activation.json', 'runtime/activation.json', inventory, BASE)
            for filename in EXTERNAL:
                source = pathlib.Path(filename)
                require(source.is_file() and not source.is_symlink(), 'Required external runtime configuration missing')
                archive_path(archive, source, 'host-config/' + filename.lstrip('/'), inventory, source.parent)
            for name, volume in sorted(production.volumes.items()):
                archive_path(archive, volume['path'], 'volumes/' + name, inventory, volume['path'])
            raw = canonical(manifest)
            info = tarfile.TarInfo('manifest.json')
            info.size, info.mode = len(raw), 0o600
            archive.addfile(info, io.BytesIO(raw))
        sys.stdout.buffer.flush()
        event('captured', archiveID=identifier, files=len(inventory), manifestSha256=hashlib.sha256(raw).hexdigest())
    finally:
        signal.alarm(0)
        event('resuming-original-services')
        try:
            resume(state)
            resumed = True
            done.write_text('parent resumed services\n')
        finally:
            if resumed:
                os.waitpid(child, 0)
                shutil.rmtree(stage)
                event('services-resumed')
            else:
                event('resume-incomplete', recoveryState=str(stage / 'recovery.json'))
            lock_file.close()


def unpack_verified(archive_pathname, target):
    require(target.is_dir() and not any(target.iterdir()), 'Extraction directory must be fresh and empty')
    inventory, links, permissions, seen = [], [], [], set()
    manifest = None
    with tarfile.open(archive_pathname, 'r:gz') as archive:
        for member in archive:
            name = safe_name(member.name).as_posix()
            require(name not in seen and manifest is None, 'Duplicate member or data after manifest')
            seen.add(name)
            if name == 'manifest.json':
                require(member.isfile() and member.size <= 128 * 1024 * 1024, 'Invalid manifest')
                manifest = json.load(archive.extractfile(member))
                continue
            require(name.startswith(('runtime/', 'host-config/', 'volumes/', 'logical/')),
                    'Unknown archive namespace')
            require(member.isdir() or member.isfile() or member.issym(), 'Special archive member refused')
            destination = target / name
            require(contained(destination, target), 'Archive extraction escaped temporary directory')
            row = {'path': name, 'mode': member.mode, 'uid': member.uid, 'gid': member.gid,
                   'type': 'directory' if member.isdir() else 'symlink' if member.issym() else 'file'}
            if member.isdir():
                destination.mkdir(parents=True, exist_ok=True)
            elif member.issym():
                symlink_target(name, member.linkname)
                row['target'] = member.linkname
                links.append((destination, member.linkname))
            else:
                destination.parent.mkdir(parents=True, exist_ok=True)
                with destination.open('xb') as output:
                    reader = archive.extractfile(member)
                    hashed = HashReader(reader)
                    shutil.copyfileobj(hashed, output, 1024 * 1024)
                row.update(bytes=destination.stat().st_size, sha256=hashed.hash.hexdigest())
                require(row['bytes'] == member.size, 'Truncated archive member')
            inventory.append(row)
            if not member.issym():
                permissions.append((destination, member.mode, member.uid, member.gid, member.mtime))
    require(manifest and manifest.get('format') == FORMAT and manifest.get('files') == inventory,
            'Manifest integrity or inventory mismatch')
    for destination, link in links:
        require(not any((target / n).is_relative_to(destination) and target / n != destination for n in seen),
                'Archive contains entries below a symlink')
    validate_symlink_graph({destination.relative_to(target).as_posix(): link for destination, link in links})
    if 'instanceID' in manifest:
        restored_activation(target, manifest)
    for destination, link in links:
        destination.parent.mkdir(parents=True, exist_ok=True)
        destination.symlink_to(link)
    require(all(contained(destination, target) for destination, _ in links),
            'Resolved archive symlink escaped temporary directory')
    if os.name == 'posix':
        for destination, mode, uid, gid, modified in reversed(permissions):
            os.chmod(destination, mode)
            if os.geteuid() == 0:
                os.chown(destination, uid, gid)
            os.utime(destination, (modified, modified))
    return manifest


def restore_resource_owned(kind, name, identifier):
    require(name.startswith(identifier + '-'), 'Unexpected disposable resource name')
    result = subprocess.run(['docker', kind, 'inspect', name], capture_output=True)
    if result.returncode:
        return False
    item = json.loads(result.stdout)[0]
    labels = item.get('Config', {}).get('Labels', {}) if kind == 'container' else item.get('Labels', {})
    require((labels or {}).get('com.puty.backup.restore') == identifier, 'Foreign restore resource refused')
    return True


def restore_diagnostic(args, phase, service, tool, result=None, timed_out=False):
    # Never serialize argv, stderr, SQL text, container environment, or passwords.
    details = {'phase': phase, 'service': service, 'command': pathlib.Path(args[0]).name,
               'tool': tool, 'exitCode': None if result is None else result.returncode}
    if timed_out:
        details['timedOut'] = True
    if result is not None:
        codes = re.findall(rb'\bERROR\s+([0-9]{3,5})\s+\(([A-Z0-9]{5})\)', result.stderr or b'')
        if codes:
            details['sqlErrors'] = [{'number': int(number), 'sqlState': state.decode('ascii')}
                                    for number, state in list(dict.fromkeys(codes))[:8]]
    return details


def restore_run(report, phase, service, tool, args, *, source=None, timeout=180):
    report['lastStep'] = {'phase': phase, 'service': service, 'command': pathlib.Path(args[0]).name,
                          'tool': tool}
    try:
        result = subprocess.run(args, stdin=source, stdout=subprocess.PIPE,
                                stderr=subprocess.PIPE, timeout=timeout)
    except subprocess.TimeoutExpired:
        report['failure'] = restore_diagnostic(args, phase, service, tool, timed_out=True)
        event('restore-step-failed', **report['failure'])
        raise RuntimeError('Disposable restore command timed out; safe diagnostics recorded') from None
    if result.returncode:
        report['failure'] = restore_diagnostic(args, phase, service, tool, result)
        event('restore-step-failed', **report['failure'])
        raise RuntimeError('Disposable restore command failed; safe diagnostics recorded')
    return result.stdout


def wait_restore_ready(report, service, container, attempts=120):
    require(service in ('mysql', 'mongo'), 'Unexpected disposable database service')
    # Official entrypoints initialize using a temporary child server. PID 1 is
    # still the entrypoint shell until initialization ends and exec runs the
    # final server. MySQL's temporary --skip-networking server also cannot pass
    # this TCP query; mysqladmin ping is unsuitable because AccessDenied is 0.
    server = 'mysqld' if service == 'mysql' else 'mongod'
    command = 'read -r puty_restore_pid1 < /proc/1/comm; [ "$puty_restore_pid1" = '+server+' ] || exit 75; '
    if service == 'mysql':
        command += MYSQL_AUTH + 'exec mysql --protocol=TCP --host=127.0.0.1 --port=3306 --connect-timeout=2 -uroot --batch --skip-column-names -e "SELECT 1"'
    else:
        command += 'exec mongosh --quiet '+MONGO_AUTH+' --eval "if(db.adminCommand({ping:1}).ok!==1)quit(1)"'
    args = ['docker', 'exec', container, 'sh', '-ec', command]
    report['lastStep'] = {'phase': 'ready', 'service': service, 'command': 'docker',
                          'tool': 'mysql' if service == 'mysql' else 'mongosh'}
    last = None
    for attempt in range(attempts):
        try:
            result = subprocess.run(args, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=10)
            if result.returncode == 0 and (service != 'mysql' or result.stdout.strip() == b'1'):
                report['checks'][service+'AuthenticatedFinalServerReady'] = True
                return
            last = restore_diagnostic(args, 'ready', service, report['lastStep']['tool'], result)
        except subprocess.TimeoutExpired:
            last = restore_diagnostic(args, 'ready', service, report['lastStep']['tool'], timed_out=True)
        if attempt + 1 < attempts:
            time.sleep(1)
    report['failure'] = last
    event('restore-step-failed', **last)
    raise RuntimeError('Disposable restore final server did not become authenticated-ready')


def validate_mysql_table_checks(raw, tables):
    """Accept exactly one successful CHECK TABLE result per captured base table."""
    expected = {'gosuxin_prod.' + name for name in tables}
    seen = set()
    try:
        rows = raw.decode('ascii').splitlines()
    except (AttributeError, UnicodeDecodeError):
        raise RuntimeError('Malformed MySQL CHECK TABLE result') from None
    for row in rows:
        fields = row.split('\t')
        require(len(fields) == 4 and fields[0] in expected and fields[0] not in seen
                and fields[1:] == ['check', 'status', 'OK'],
                'MySQL CHECK TABLE returned a non-OK, duplicate, or unexpected result')
        seen.add(fields[0])
    require(seen == expected, 'MySQL CHECK TABLE results are incomplete')
    return len(seen)


def check_restored_mysql_tables(report, container, tables):
    names = list(tables)
    require(all(isinstance(name, str) and re.fullmatch('[A-Za-z0-9_]+', name) for name in names)
            and len(names) == len(set(names)), 'Unsafe or duplicate stored table name')
    command = (MYSQL_AUTH + 'exec mysql --protocol=TCP --host=127.0.0.1 --port=3306 '
               '-uroot --batch --raw --skip-column-names -e "$1"')
    args = ['docker', 'exec', container, 'sh', '-ec', command, '--']
    inventory_sql = ("SELECT TABLE_NAME FROM information_schema.tables WHERE TABLE_SCHEMA='gosuxin_prod' "
                     "AND TABLE_TYPE='BASE TABLE' ORDER BY TABLE_NAME")
    raw = restore_run(report, 'validate', 'mysql', 'mysql', args + [inventory_sql], timeout=900)
    try:
        actual = raw.decode('ascii').splitlines()
    except (AttributeError, UnicodeDecodeError):
        raise RuntimeError('Malformed restored MySQL base table inventory') from None
    require(all(re.fullmatch('[A-Za-z0-9_]+', name) for name in actual)
            and len(actual) == len(set(actual)) and set(actual) == set(names),
            'Restored MySQL base table inventory differs from captured tables')
    report['checks']['mysqlBaseTableInventoryExact'] = True
    check_sql = 'CHECK TABLE ' + ','.join('`gosuxin_prod`.`' + name + '`' for name in sorted(names))
    raw = restore_run(report, 'validate', 'mysql', 'mysql', args + [check_sql], timeout=900) if names else b''
    report['checks']['mysqlBaseTablesCheckOK'] = validate_mysql_table_checks(raw, names)


def restore_check(archive_sha):
    production = Production()  # Validates identity; never stops/starts source services.
    lock_file = (production.stage_root / 'backup.lock').open('a+b')
    fcntl.flock(lock_file, fcntl.LOCK_EX | fcntl.LOCK_NB)
    identifier = 'puty-restore-' + secrets.token_hex(12)
    stage = production.stage_root / identifier
    stage.mkdir(mode=0o700)
    containers, volumes = [], []
    report = {'format': FORMAT, 'restoreID': identifier, 'started': utc(), 'passed': False,
              'productionModified': False, 'network': 'none', 'publishedPorts': [], 'checks': {}}
    try:
        archive = stage / 'payload.tar.gz'
        hashed = HashReader(sys.stdin.buffer)
        with archive.open('xb') as output:
            shutil.copyfileobj(hashed, output, 1024 * 1024)
        require(hashed.hash.hexdigest() == archive_sha, 'Transferred archive checksum mismatch')
        unpacked = stage / 'unpacked'
        unpacked.mkdir(mode=0o700)
        manifest = unpack_verified(archive, unpacked)
        require(manifest['instanceID'] == production.owner and manifest['mongoDatabases'] == EXPECTED_DBS
                and manifest['mysqlDatabase'] == 'gosuxin_prod', 'Backup owner or database identity mismatch')
        report['archiveID'] = manifest['archiveID']
        report['checks']['fileInventory'] = len(manifest['files'])
        report['checks']['activationOwnerAndOriginalCutoffVerified'] = True
        for service in ('mysql', 'mongo'):
            image = manifest['images'][service]
            require(re.fullmatch('sha256:[0-9a-f]{64}', image), 'Restore image must be a captured content ID')
            require(json.loads(restore_run(report, 'image-inspect', service, 'docker',
                    ['docker', 'image', 'inspect', image]))[0]['Id'] == image, 'Exact restore image is unavailable')
            volume, container = identifier + '-' + service + '-data', identifier + '-' + service
            restore_run(report, 'volume-create', service, 'docker',
                        ['docker', 'volume', 'create', '--label', 'com.puty.backup.restore=' + identifier, volume])
            volumes.append(volume)
            password = secrets.token_hex(32)
            env = stage / (service + '.env')
            env.write_text(('MYSQL_ROOT_PASSWORD=' if service == 'mysql' else 'MONGO_INITDB_ROOT_USERNAME=restorecheck\nMONGO_INITDB_ROOT_PASSWORD=') + password + '\n')
            env.chmod(0o600)
            destination = '/var/lib/mysql' if service == 'mysql' else '/data/db'
            containers.append(container)
            restore_run(report, 'container-start', service, 'docker',
                ['docker', 'run', '-d', '--pull=never', '--network', 'none', '--name', container,
                 '--label', 'com.puty.backup.restore=' + identifier, '--memory', '1536m', '--cpus', '2',
                 '--env-file', str(env), '--mount', 'type=volume,source=' + volume + ',target=' + destination, image])
            wait_restore_ready(report, service, container)
            if service == 'mysql':
                with (unpacked / 'logical/mysql.sql').open('rb') as source:
                    restore_run(report, 'logical-import', service, 'mysql', ['docker', 'exec', '-i', container, 'sh', '-ec', MYSQL_AUTH + 'exec mysql --protocol=TCP --host=127.0.0.1 --port=3306 -uroot'], source=source, timeout=900)
                names = manifest['counts']['mysql']
                require(all(re.fullmatch('[A-Za-z0-9_]+', n) for n in names), 'Unsafe stored table name')
                query = ';'.join("SELECT '" + n + "',COUNT(*) FROM `gosuxin_prod`.`" + n + '`' for n in names)
                raw = restore_run(report, 'count', service, 'mysql', ['docker', 'exec', container, 'sh', '-ec', MYSQL_AUTH + 'exec mysql --protocol=TCP --host=127.0.0.1 --port=3306 -uroot --batch --skip-column-names -e "$1"', '--', query]) if names else b''
                counts = {line.split('\t')[0]: int(line.split('\t')[1]) for line in raw.decode().splitlines()}
                check_restored_mysql_tables(report, container, names)
            else:
                for database in EXPECTED_DBS:
                    with (unpacked / ('logical/mongo-' + database + '.archive.gz')).open('rb') as source:
                        restore_run(report, 'logical-import', service, 'mongorestore', ['docker', 'exec', '-i', container, 'sh', '-ec', 'exec mongorestore --quiet ' + MONGO_AUTH + ' --archive --gzip'], source=source, timeout=900)
                script = DB_COUNTS % json.dumps(EXPECTED_DBS)
                counts = json.loads(restore_run(report, 'count', service, 'mongosh', ['docker', 'exec', container, 'sh', '-ec', 'exec mongosh --quiet ' + MONGO_AUTH + ' --eval "$1"', '--', script]))
                validate = '''for(const n of %s){const d=db.getSiblingDB(n);for(const c of d.getCollectionNames().filter(x=>!x.startsWith("system."))){if(!d.runCommand({validate:c}).valid)throw new Error("Invalid collection");}}''' % json.dumps(EXPECTED_DBS)
                restore_run(report, 'validate', service, 'mongosh', ['docker', 'exec', container, 'sh', '-ec', 'exec mongosh --quiet ' + MONGO_AUTH + ' --eval "$1"', '--', validate])
            require(counts == manifest['counts'][service], 'Restored database counts do not match source')
            report['checks'][service + 'LogicalRestore'] = True
        # Every cold-volume byte has already been restored into a private directory
        # and hashed. Do not start Redis/Kafka/etcd with production identities.
        for name in manifest['volumes']:
            require((unpacked / 'volumes' / name).is_dir(), 'Missing restored cold volume directory')
        for name in ('runtime/openim/compose.yaml', 'runtime/openim/secrets.json',
                     'runtime/openim/instance.json', 'runtime/gosuxin/runtime-environment.json',
                     'runtime/gosuxin/manifest/config/config.yaml', 'runtime/gosuxin/storage/plugins/biz-suite/data'):
            require((unpacked / name).exists(), 'Required configuration/key storage is missing')
        runtime = json.loads((unpacked / 'runtime/gosuxin/runtime-environment.json').read_text())
        require(runtime.get('WECOM_CREDENTIAL_KEY') and runtime.get('GOSUXIN_IM_HMAC_KEY'), 'Required restored HRM key material missing')
        report['checks']['allColdVolumeFilesRestoredAndHashed'] = True
        report['checks']['minioFilesRestoredAndHashed'] = True
        report['checks']['runtimeAndKeyMaterialPresent'] = True
        report['passed'] = True
    except Exception as error:
        report.setdefault('failure', {**report.get('lastStep', {'phase': 'archive-verify', 'service': None}),
                                      'category': type(error).__name__})
        raise
    finally:
        errors = []
        for name in reversed(containers):
            try:
                if restore_resource_owned('container', name, identifier):
                    run(['docker', 'rm', '-f', name])
            except Exception:
                errors.append(name)
        for name in reversed(volumes):
            try:
                if restore_resource_owned('volume', name, identifier):
                    run(['docker', 'volume', 'rm', name])
            except Exception:
                errors.append(name)
        report['cleanupVerified'] = not errors
        report['finished'] = utc()
        require(contained(stage, production.stage_root) and stage.name == identifier, 'Restore cleanup path mismatch')
        shutil.rmtree(stage)
        if errors:
            report['passed'] = False
            report['cleanupRequired'] = errors
        lock_file.close()
        print(json.dumps(report), flush=True)


def main():
    require(os.name == 'posix' and fcntl is not None, 'Remote worker requires Linux')
    os.umask(0o077)
    def interrupted(_signal, _frame):
        raise InterruptedError('Backup/restore worker interrupted; cleaning up')
    for signum in (signal.SIGHUP, signal.SIGTERM, signal.SIGINT):
        signal.signal(signum, interrupted)
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest='command', required=True)
    backup = commands.add_parser('create')
    backup.add_argument('--max-stop-seconds', type=int, default=1800)
    restore = commands.add_parser('restore-check')
    restore.add_argument('--payload-sha256', required=True)
    args = parser.parse_args()
    if args.command == 'create':
        require(120 <= args.max_stop_seconds <= 7200, 'Stop interval must be 120..7200 seconds')
        create(args.max_stop_seconds)
    else:
        require(re.fullmatch('[a-f0-9]{64}', args.payload_sha256), 'Invalid payload checksum')
        restore_check(args.payload_sha256)


if __name__ == '__main__':
    try:
        main()
    except Exception as error:
        event('failed', category=type(error).__name__, message=str(error))
        sys.exit(1)
