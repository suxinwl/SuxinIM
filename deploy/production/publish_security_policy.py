#!/usr/bin/env python3
"""Prepare a local backup proof, then plan/apply one exact owned security release."""
import argparse
import copy
import hashlib
import importlib.util
import json
import os
import pathlib
import re
import signal
import stat
import subprocess
import sys
import time
import urllib.request
import uuid

import yaml

BASE = pathlib.Path('/srv/puty-suite')
INSTANCE = '5ac7c644d9cd4923e2b701f66b52485d'
PROJECT = 'puty-suite-prod'
SUPERVISOR = '/www/server/panel/pyenv/bin/supervisorctl'
BINARIES = {**{name: name for name in (
    'openim-api openim-crontask openim-msggateway openim-msgtransfer openim-push '
    'openim-rpc-auth openim-rpc-conversation openim-rpc-friend openim-rpc-group '
    'openim-rpc-msg openim-rpc-third openim-rpc-user enterprise-service admin-api chat-api').split()},
    'admin-rpc': 'chat-admin', 'chat-rpc': 'chat-business'}
STORAGE = {'mysql', 'mongo', 'redis', 'etcd', 'kafka', 'minio', 'livekit'}
DATA_DESTINATIONS = {'mysql': '/var/lib/mysql', 'mongo': '/data/db', 'redis': '/data',
                     'etcd': '/bitnami/etcd', 'kafka': '/bitnami/kafka', 'minio': '/bitnami/minio/data'}
HELPERS = {'publish_hrm_web': 'bb7c96958559b3bc3a12cb2e60673db99ad6839f04c9e922b32c7eae12198a6a',
           'update_backend': 'a4f5847386a92c31143a4b184b4691c9bffd1eab9a8b81d2c15f95fc4fee4eca'}
# This reviewed worker used the former readyz envelope and may need rollback.
# Only recover_transaction accepts its plan; build/apply stay current-source-only.
LEGACY_RECOVERY_PUBLISHER_SHA256 = '1c6b2cb4904b800df5e554c23cd9e3bf3d954b939460369fe5c7ccceaf534d66'
CRON = 'openim/config/openim-server/openim-crontask.yml'
COMPOSE = 'openim/compose.yaml'
MAIN = 'gosuxin/main'
MANIFEST = 'openim/bin/backend-binaries.json'
WEB = 'gosuxin/resource/suxinweb'


def require(ok, code):
    if not ok:
        raise RuntimeError(code)


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def digest(path):
    with path.open('rb') as source:
        result = hashlib.sha256()
        while block := source.read(1 << 20):
            result.update(block)
    return result.hexdigest()


def canonical(value):
    return (json.dumps(value, sort_keys=True, separators=(',', ':'), ensure_ascii=False) + '\n').encode()


def regular(path, root_owned=True):
    meta = path.lstat()
    require(path.is_absolute() and path.resolve() == path and stat.S_ISREG(meta.st_mode)
            and meta.st_nlink == 1 and not meta.st_mode & 0o022
            and (not root_owned or meta.st_uid == 0), 'regular_owned_file_required')
    return path


def metadata(path, root_owned=True):
    value = regular(path, root_owned).stat()
    return {'sha256': digest(path), 'bytes': value.st_size, 'mode': stat.S_IMODE(value.st_mode),
            'uid': value.st_uid, 'gid': value.st_gid}


def json_file(path, expected=None, root_owned=True):
    require(regular(path, root_owned).stat().st_size < 16 << 20, 'json_size_exceeded')
    raw = path.read_bytes()
    if expected:
        require(re.fullmatch('[0-9a-f]{64}', expected) and sha(raw) == expected, 'json_sha_mismatch')
    return json.loads(raw)


def sync_directory(path):
    if os.name == 'posix':
        handle = os.open(path, os.O_RDONLY | os.O_DIRECTORY)
        try:
            os.fsync(handle)
        finally:
            os.close(handle)


def private_write(path, value, exclusive=False):
    require(path.parent.resolve() == path.parent and not path.is_symlink(), 'journal_path_redirected')
    temporary = path.with_name('.' + path.name + '.next-' + uuid.uuid4().hex)
    with temporary.open('xb') as target:
        os.chmod(temporary, 0o600)
        target.write(canonical(value))
        target.flush()
        os.fsync(target.fileno())
    if exclusive:
        require(not path.exists(), 'output_already_exists')
    temporary.replace(path)
    sync_directory(path.parent)


def backup_receipt(archive, manifest_path, restore_path):
    # Runs on the operator's machine. The archive/key are never uploaded.
    for path in (archive, manifest_path, restore_path):
        require(path.is_file() and not path.is_symlink() and path.resolve() == path,
                'local_proof_file_invalid')
    value = json.loads(manifest_path.read_bytes())
    restored = json.loads(restore_path.read_bytes())
    actual = digest(archive)
    require(value.get('sourceInstanceID') == INSTANCE
            and value.get('consistency') == 'all-suite-writers-and-storage-stopped'
            and value.get('ciphertextSha256') == actual == restored.get('ciphertextSha256')
            and type(value.get('ciphertextBytes')) is int and value['ciphertextBytes'] == archive.stat().st_size > 0
            and restored.get('passed') is True and restored.get('cleanupVerified') is True
            and value.get('sourceArchiveID') == restored.get('sourceArchiveID')
            and isinstance(value.get('sourceArchiveID'), str)
            and re.fullmatch(r'[A-Za-z0-9_.-]{8,128}', value['sourceArchiveID'])
            and value.get('recoveryKeyStoredOnServer') is False
            and value.get('recoveryKeyIncludedInArchive') is False, 'complete_backup_proof_mismatch')
    return {'format': 'puty-security-backup-proof-1', 'instanceID': INSTANCE,
            'archiveID': value['sourceArchiveID'], 'archiveSHA256': actual,
            'archiveBytes': value['ciphertextBytes'], 'manifestSHA256': digest(manifest_path),
            'restoreCheckSHA256': digest(restore_path), 'verifiedAt': int(time.time()),
            'consistency': value['consistency'], 'restorePassed': True, 'cleanupVerified': True,
            'archivePresentAndHashedLocally': True, 'keyIncluded': False}


def verify_receipt(value):
    require(value.get('format') == 'puty-security-backup-proof-1' and value.get('instanceID') == INSTANCE
            and value.get('restorePassed') is True and value.get('cleanupVerified') is True
            and value.get('archivePresentAndHashedLocally') is True and value.get('keyIncluded') is False
            and value.get('consistency') == 'all-suite-writers-and-storage-stopped'
            and type(value.get('archiveBytes')) is int and value['archiveBytes'] > 0
            and type(value.get('verifiedAt')) is int and -60 <= time.time() - value['verifiedAt'] <= 86400
            and all(isinstance(value.get(key), str) and re.fullmatch('[0-9a-f]{64}', value[key])
                    for key in ('archiveSHA256', 'manifestSHA256', 'restoreCheckSHA256')),
            'backup_receipt_invalid_or_expired')


def load_helpers():
    result = {}
    for name, expected in HELPERS.items():
        path = pathlib.Path(__file__).resolve().with_name(name + '.py')
        require(digest(regular(path)) == expected, 'reviewed_helper_changed')
        spec = importlib.util.spec_from_file_location('security_' + name, path)
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        result[name] = module
    return result['publish_hrm_web'], result['update_backend']


def execute(argv, capture=False, timeout=180):
    result = subprocess.run(argv, stdout=subprocess.PIPE if capture else subprocess.DEVNULL,
                            stderr=subprocess.DEVNULL, timeout=timeout)
    require(result.returncode == 0, 'controlled_subprocess_failed')
    return result.stdout if capture else None


def compose_command():
    return ['docker', 'compose', '--project-directory', str(BASE / 'openim'), '-f',
            str(BASE / COMPOSE), '-p', PROJECT]


def owned(require_running=True, allow_missing_apps=False):
    ids = execute(['docker', 'ps', '-a', '-q', '--filter',
                   'label=com.docker.compose.project=' + PROJECT], True).decode().split()
    require((7 <= len(ids) <= 24) if allow_missing_apps else len(ids) == 24,
            'exact_24_containers_required')
    values = json.loads(execute(['docker', 'inspect', *ids], True))
    result = {}
    for row in values:
        labels = row['Config'].get('Labels') or {}
        name = labels.get('com.docker.compose.service')
        require(name in set(BINARIES.values()) | STORAGE and name not in result
                and labels.get('com.docker.compose.project') == PROJECT
                and labels.get('com.puty.suite') == INSTANCE
                and labels.get('com.puty.environment') == 'production'
                and (not require_running or row['State']['Running'] is True), 'container_owner_or_state_invalid')
        result[name] = row
    require(STORAGE <= set(result) and (allow_missing_apps or
            set(result) == set(BINARIES.values()) | STORAGE), 'service_set_invalid')
    for binary, name in BINARIES.items():
        if name not in result:
            continue
        require(any(m.get('Type') == 'bind' and m.get('Source') == str(BASE / 'openim/bin' / binary)
                    and m.get('Destination') == '/app/bin/' + binary and m.get('RW') is False
                    for m in result[name].get('Mounts', [])), 'executable_mount_invalid')
    return result


def declared_data_target(definitions, service):
    found = []
    for value in definitions.get('services', {}).get(service, {}).get('volumes', []):
        if isinstance(value, str):
            parts = value.split(':')
            if parts[0] == service + '-data':
                require(len(parts) in (2, 3) and (len(parts) == 2 or parts[2] == 'rw'),
                        'storage_data_mount_schema_invalid')
                found.append(parts[1])
        elif isinstance(value, dict) and value.get('type') == 'volume' and value.get('source') == service + '-data':
            require(value.get('read_only') in (None, False), 'storage_data_volume_read_only')
            found.append(value.get('target'))
    require(len(found) == 1 and found[0] == DATA_DESTINATIONS[service],
            'storage_data_target_not_uniquely_declared')
    return found[0]


def volume_guard(containers):
    definitions = yaml.safe_load(regular(BASE / COMPOSE).read_bytes())
    result = {}
    for name in STORAGE - {'livekit'}:
        destination = declared_data_target(definitions, name)
        mounts = [m for m in containers[name].get('Mounts', []) if m.get('Type') == 'volume']
        matches = []
        for mount in mounts:
            row = json.loads(execute(['docker', 'volume', 'inspect', mount['Name']], True))[0]
            labels = row.get('Labels') or {}
            if labels.get('com.docker.compose.volume') != name + '-data':
                continue  # Mongo may additionally expose its unused anonymous configdb volume.
            require(row.get('Driver') == 'local' and row.get('Scope') == 'local'
                    and labels.get('com.puty.suite') == INSTANCE and mount.get('RW') is True
                    and labels.get('com.docker.compose.project') == PROJECT
                    and row.get('Mountpoint') == mount.get('Source')
                    and mount.get('Destination') == destination, 'foreign_storage_volume')
            matches.append({'name': mount['Name'], 'source': mount['Source'], 'destination': destination})
        require(len(matches) == 1, 'storage_volume_set_invalid')
        result[name] = matches[0]
    return result


def owner_guard():
    require(os.geteuid() == 0 and BASE.resolve() == BASE, 'root_owned_suite_required')
    for directory in (BASE, BASE / 'staging', BASE / 'backups'):
        value = directory.lstat()
        require(directory.resolve() == directory and stat.S_ISDIR(value.st_mode)
                and value.st_uid == 0 and not value.st_mode & 0o022, 'suite_directory_owner_invalid')
    marker = json_file(BASE / 'openim/instance.json')
    activation = json_file(BASE / 'activation.json')
    require(marker.get('instanceID') == activation.get('instanceID') == INSTANCE
            and marker.get('production') is True and marker.get('testOnly') is False
            and marker.get('root') == str(BASE / 'openim') and marker.get('stackName') == PROJECT,
            'suite_owner_marker_mismatch')


def plugin_proof():
    active = BASE / 'gosuxin/storage/plugins/biz-suite/active.json'
    state = json_file(active, root_owned=False)
    package = state.get('package')
    require(isinstance(package, str) and re.fullmatch('[0-9a-f]{64}', package)
            and not state.get('disabled'), 'active_plugin_invalid')
    root = active.parent / 'packages' / package
    require(root.resolve() == root and root.is_dir() and not root.is_symlink(), 'plugin_root_redirected')
    result = {str(active.relative_to(BASE)): metadata(active, False)}
    for path in sorted(root.rglob('*')):
        require(not path.is_symlink(), 'plugin_link_rejected')
        if path.is_file():
            result[str(path.relative_to(BASE))] = metadata(path, False)
    require(2 <= len(result) <= 5000 and str((root / 'plugin.json').relative_to(BASE)) in result
            and str((root / 'bin/biz-suite').relative_to(BASE)) in result, 'plugin_inventory_invalid')
    return result


def proposed_configs(cron_raw, compose_raw):
    cron = yaml.safe_load(cron_raw)
    require(isinstance(cron, dict) and all(key in cron for key in
            ('retainChatRecords', 'fileExpireTime', 'deleteObjectType')), 'cron_schema_invalid')
    updated_cron = {**cron, 'retainChatRecords': 0, 'fileExpireTime': 0, 'deleteObjectType': []}
    compose = yaml.safe_load(compose_raw)
    require(compose.get('name') == PROJECT and set(compose.get('services', {})) ==
            set(BINARIES.values()) | STORAGE, 'compose_scope_invalid')
    updated = copy.deepcopy(compose)
    enterprise = updated['services']['enterprise-service']
    volumes = enterprise.setdefault('volumes', [])
    require(isinstance(volumes, list) and all(isinstance(row, str) for row in volumes), 'capacity_volume_schema_invalid')
    for name in ('mongo', 'minio'):
        wanted = name + '-data:/capacity/' + name + ':ro'
        same_target = [row for row in volumes if len(row.split(':')) >= 2 and row.split(':')[1] == '/capacity/' + name]
        require(not same_target or same_target == [wanted], 'existing_capacity_mount_mismatch')
        require(name + '-data' in compose.get('volumes', {}), 'declared_volume_missing')
        if not same_target:
            volumes.append(wanted)
    enterprise.setdefault('environment', {})['STORAGE_CAPACITY_DATA_VERIFIED'] = 'false'
    # Construction changes only the two alias mounts and this one boolean flag.
    restored = copy.deepcopy(updated)
    restored['services']['enterprise-service'] = compose['services']['enterprise-service']
    require(restored == compose, 'unrelated_compose_delta')
    return (yaml.safe_dump(updated_cron, allow_unicode=True, sort_keys=False).encode(),
            yaml.safe_dump(updated, allow_unicode=True, sort_keys=False).encode())


def stage_file(relative):
    require(isinstance(relative, str) and re.fullmatch(r'[A-Za-z0-9][A-Za-z0-9_./-]{0,255}', relative)
            and all(part not in ('', '.', '..') for part in relative.split('/')), 'candidate_path_invalid')
    path = BASE / 'staging' / relative
    require(path.resolve() == path and path.is_relative_to(BASE / 'staging'), 'candidate_outside_staging')
    return regular(path)


def elf(path):
    with regular(path).open('rb') as source:
        header = source.read(20)
    require(header[:6] == b'\x7fELF\x02\x01' and header[18:20] == b'\x3e\x00', 'linux_amd64_elf_required')


def request_sources(request, web):
    require(set(request) == {'format', 'backend', 'main', 'web', 'backupReceipt'}
            and request['format'] == 'puty-security-release-1' and set(request['backend']) == set(BINARIES),
            'exact_17_binary_request_required')
    sources = {}
    for name, entry in [*request['backend'].items(), ('main', request['main'])]:
        require(set(entry) == {'path', 'sha256', 'bytes'} and type(entry['bytes']) is int
                and 0 < entry['bytes'] < 300 << 20 and re.fullmatch('[0-9a-f]{64}', entry['sha256']),
                'candidate_descriptor_invalid')
        path = stage_file(entry['path'])
        require(path.stat().st_size == entry['bytes'] and digest(path) == entry['sha256'], 'candidate_hash_mismatch')
        elf(path)
        sources[entry['path']] = metadata(path)
    archive = request['web']
    require(set(archive) == {'path', 'sha256', 'indexSHA256'}, 'web_descriptor_invalid')
    path = stage_file(archive['path'])
    payload = web.inspect_archive(path, archive['sha256'], archive['indexSHA256'], 'hrm')
    sources[archive['path']] = metadata(path)
    proof = request['backupReceipt']
    require(set(proof) == {'path', 'sha256'}, 'backup_descriptor_invalid')
    receipt = json_file(stage_file(proof['path']), proof['sha256'])
    verify_receipt(receipt)
    sources[proof['path']] = metadata(stage_file(proof['path']))
    return sources, payload


def build_plan(request, request_sha, web):
    owner_guard()
    containers = owned()
    volumes = volume_guard(containers)
    require(supervisor_status() == 'RUNNING', 'hrm_not_running_before_plan')
    overrides = ('IMENV_OPENIM_CRONTASK_RETAINCHATRECORDS', 'IMENV_OPENIM_CRONTASK_FILEEXPIRETIME',
                 'IMENV_OPENIM_CRONTASK_DELETEOBJECTTYPE')
    require(not any(row.partition('=')[0] in overrides and row.partition('=')[2]
                    for row in containers['openim-crontask']['Config'].get('Env', [])),
            'cron_environment_override_requires_separate_review')
    sources, payload = request_sources(request, web)
    cron, compose = proposed_configs(regular(BASE / CRON).read_bytes(), regular(BASE / COMPOSE).read_bytes())
    definitions = yaml.safe_load(compose)
    for name in STORAGE - {'livekit'}:
        definition = definitions.get('volumes', {}).get(name + '-data') or {}
        require(isinstance(definition, dict) and volumes[name]['name'] ==
                definition.get('name', PROJECT + '_' + name + '-data'), 'compose_volume_identity_mismatch')
    cron_env = definitions['services']['openim-crontask'].get('environment', {})
    require(isinstance(cron_env, dict) and not any(cron_env.get(key) for key in overrides),
            'future_cron_environment_override_requires_separate_review')
    old_manifest = json_file(BASE / MANIFEST)
    targets = [MAIN, CRON, COMPOSE, MANIFEST, 'gosuxin/resource/suxinweb/index.html']
    targets += ['openim/bin/' + name for name in sorted(BINARIES)]
    current = {name: metadata(BASE / name, root_owned=name not in (MAIN, 'gosuxin/resource/suxinweb/index.html')) for name in targets}
    require(all(old_manifest.get(name, {}).get('sha256') == current['openim/bin/' + name]['sha256']
                for name in BINARIES), 'current_backend_manifest_mismatch')
    processes = {}
    for binary, service in BINARIES.items():
        require(containers[service]['Config'].get('Entrypoint') == ['/app/bin/' + binary],
                'actual_backend_entrypoint_mismatch')
        processes[service] = process_proof(containers[service]['State'].get('Pid'),
                                          current['openim/bin/' + binary]['sha256'])
    processes['hrm'] = hrm_process_proof(current[MAIN]['sha256'])
    return {'format': 'puty-security-plan-1', 'instanceID': INSTANCE, 'createdAt': int(time.time()),
            'publisherSHA256': digest(regular(pathlib.Path(__file__).resolve())),
            'requestSHA256': request_sha, 'request': request, 'sources': sources, 'current': current,
            'containers': {name: {'id': row['Id'], 'image': row['Image']} for name, row in containers.items()},
            'volumes': volumes,
            'processes': processes,
            'plugin': plugin_proof(), 'proposedCronSHA256': sha(cron), 'proposedComposeSHA256': sha(compose),
            'webFileCount': len(payload), 'helpers': HELPERS}


def supervisor_status():
    result = subprocess.run([SUPERVISOR, 'status', 'puty-hrm'], stdout=subprocess.PIPE,
                            stderr=subprocess.DEVNULL, timeout=15)
    text = result.stdout.decode(errors='replace')
    match = re.match(r'^puty-hrm\s+(RUNNING|STOPPED|EXITED|FATAL|STARTING|STOPPING|BACKOFF)\b', text)
    require(match is not None, 'hrm_supervisor_state_unknown')
    return match[1]


def process_proof(pid, expected):
    require(type(pid) is int and pid > 1, 'runtime_pid_invalid')
    root = pathlib.Path('/proc') / str(pid)
    before = (root / 'stat').read_text().rsplit(')', 1)[1].split()[19]
    actual = digest(root / 'exe')
    after = (root / 'stat').read_text().rsplit(')', 1)[1].split()[19]
    require(before == after and actual == expected, 'actual_process_executable_mismatch')
    return {'pid': pid, 'startTime': before, 'sha256': actual}


def hrm_process_proof(expected):
    value = execute([SUPERVISOR, 'status', 'puty-hrm'], True, 15).decode(errors='replace')
    match = re.match(r'^puty-hrm\s+RUNNING\s+pid\s+([0-9]+),', value)
    require(match is not None, 'hrm_process_pid_unavailable')
    return process_proof(int(match[1]), expected)


def hrm_stop():
    if supervisor_status() not in ('STOPPED', 'EXITED', 'FATAL'):
        subprocess.run([SUPERVISOR, 'stop', 'puty-hrm'], stdout=subprocess.DEVNULL,
                       stderr=subprocess.DEVNULL, timeout=90)
    for _ in range(30):
        if supervisor_status() in ('STOPPED', 'EXITED', 'FATAL'):
            return
        time.sleep(1)
    raise RuntimeError('hrm_stop_unconfirmed')


def ready(url):
    try:
        opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
        with opener.open(url, timeout=3) as response:
            value = json.loads(response.read(1 << 20))
            return response.status == 200 and isinstance(value, dict) and (
                type(value.get('errCode')) is int and value['errCode'] == 0
                and isinstance(value.get('data'), dict) and value['data'].get('status') == 'ready'
                if url.endswith('/readyz') else
                value.get('errCode', value.get('code')) == 0 and isinstance(value.get('data'), dict)
                and any(type(value['data'].get(key)) is bool for key in ('show', 'enabled')))
    except Exception:
        return False


def hrm_start():
    if supervisor_status() in ('STOPPED', 'EXITED', 'FATAL'):
        execute([SUPERVISOR, 'start', 'puty-hrm'], timeout=90)
    for _ in range(30):
        if supervisor_status() == 'RUNNING' and ready('http://127.0.0.1:18000/common/im-sso/captcha'):
            return
        time.sleep(2)
    raise RuntimeError('hrm_readiness_unconfirmed')


def stop_writers():
    hrm_stop()
    try:
        execute(compose_command() + ['stop', *sorted(BINARIES.values())])
    except RuntimeError:
        pass
    require(all(not row['State']['Running'] for name, row in owned(False, True).items()
                if name in BINARIES.values()), 'application_writers_not_stopped')


def start_writers(backend):
    hrm_start()
    execute(compose_command() + ['up', '-d', '--no-deps', '--no-build', '--force-recreate', *sorted(BINARIES.values())])
    for _ in range(30):
        try:
            values = owned()
            if all(values[name]['State'].get('Health', {}).get('Status') not in ('starting', 'unhealthy')
                   for name in BINARIES.values()) and ready('http://127.0.0.1:18090/enterprise/v1/readyz'):
                return
        except RuntimeError:
            pass
        time.sleep(2)
    raise RuntimeError('application_readiness_unconfirmed')


def restore_web(backup, target, web):
    if not (backup / 'release.json').exists():
        return
    receipt = json_file(backup / 'release.json')
    names = [name for name in receipt['newFiles'] if name != 'index.html'] + ['index.html']
    for name in names:
        destination = target / name
        require(destination.resolve() == destination and destination.is_relative_to(target), 'web_rollback_path_invalid')
        if name in receipt['previousFiles']:
            source = backup / name
            require(digest(regular(source)) == receipt['previousFiles'][name], 'web_rollback_payload_changed')
            web.atomic_write(destination, source.read_bytes(), *receipt['previousMetadata'][name])
        elif destination.exists():
            regular(destination, False).unlink()
            sync_directory(destination.parent)


def recovery_guard(plan):
    values = owned(False, True)
    require(volume_guard(values) == plan['volumes'], 'recovery_storage_volume_changed')
    require(all(values[name]['Id'] == plan['containers'][name]['id']
                and values[name]['Image'] == plan['containers'][name]['image'] for name in STORAGE),
            'recovery_storage_or_livekit_changed')
    require(plugin_proof() == plan['plugin'], 'plugin_changed_during_release')
    return values


def running_process_proof(values, expected, main_sha):
    for binary, service in BINARIES.items():
        require(values[service]['Config'].get('Entrypoint') == ['/app/bin/' + binary],
                'published_entrypoint_changed')
        process_proof(values[service]['State'].get('Pid'), expected[binary])
    hrm_process_proof(main_sha)


def capacity_mount_proof(values, volumes):
    row = values['enterprise-service']
    for name in ('mongo', 'minio'):
        matches = [mount for mount in row.get('Mounts', []) if mount.get('Destination') == '/capacity/' + name]
        require(len(matches) == 1 and matches[0].get('Type') == 'volume'
                and matches[0].get('Name') == volumes[name]['name']
                and matches[0].get('Source') == volumes[name]['source'] and matches[0].get('RW') is False,
                'published_capacity_mount_not_read_only_owned_data')
    flags = [value.partition('=')[2] for value in row['Config'].get('Env', [])
             if value.partition('=')[0] == 'STORAGE_CAPACITY_DATA_VERIFIED']
    require(flags == ['false'], 'capacity_verification_flag_not_false')


def rollback(transaction, plan, web, backend):
    recovery_guard(plan)
    stop_writers()
    recovery_guard(plan)
    for relative, value in plan['current'].items():
        if relative.endswith('/suxinweb/index.html'):
            continue
        source = transaction / 'previous' / relative
        require(digest(regular(source)) == value['sha256'], 'rollback_backup_changed')
        backend.atomic_copy(source, BASE / relative, value['sha256'], value['mode'], value['uid'], value['gid'])
    restore_web(transaction / 'web', BASE / WEB, web)
    require(plugin_proof() == plan['plugin'], 'plugin_changed_during_release')
    start_writers(backend)
    require(all(metadata(BASE / name, False) == value for name, value in plan['current'].items()),
            'rollback_metadata_or_hash_mismatch')
    values = recovery_guard(plan)
    running_process_proof(values,
                          {name: plan['current']['openim/bin/' + name]['sha256'] for name in BINARIES},
                          plan['current'][MAIN]['sha256'])


def recover_transaction(transaction, journal, plan, web, backend):
    require(plan.get('publisherSHA256') in (digest(regular(pathlib.Path(__file__).resolve())),
                                          LEGACY_RECOVERY_PUBLISHER_SHA256),
            'recovery_publisher_source_changed')
    try:
        rollback(transaction, plan, web, backend)
        journal['status'] = 'rolledBack'
        private_write(transaction / 'journal.json', journal)
    except BaseException:
        journal['status'] = 'recoveryRequired'
        try:
            stop_writers()
        except BaseException:
            pass
        try:
            private_write(transaction / 'journal.json', journal)
        except BaseException:
            pass
        raise RuntimeError('release_recovery_incomplete_writers_not_resumed') from None


def apply(plan, web, backend):
    fresh = build_plan(plan['request'], plan['requestSHA256'], web)
    require(all(fresh[key] == plan[key] for key in plan if key != 'createdAt'), 'plan_state_changed')
    transaction = BASE / 'backups' / ('security-policy-' + uuid.uuid4().hex)
    require(transaction.parent.resolve() == transaction.parent and transaction.parent.is_dir(), 'backup_root_redirected')
    transaction.mkdir(mode=0o700)
    sync_directory(transaction.parent)
    private_write(transaction / 'plan.json', plan, True)
    for relative, value in plan['current'].items():
        if relative.endswith('/suxinweb/index.html'):
            continue
        target = transaction / 'previous' / relative
        target.parent.mkdir(parents=True, mode=0o700, exist_ok=True)
        backend.atomic_copy(BASE / relative, target, value['sha256'], 0o600, 0, 0)
    journal = {'format': 'puty-security-transaction-1', 'instanceID': INSTANCE,
               'planSHA256': digest(transaction / 'plan.json'), 'status': 'prepared'}
    journal_path = transaction / 'journal.json'
    private_write(journal_path, journal)
    try:
        journal['status'] = 'stopping'
        private_write(journal_path, journal)
        stop_writers()
        cron, compose = proposed_configs((BASE / CRON).read_bytes(), (BASE / COMPOSE).read_bytes())
        require(sha(cron) == plan['proposedCronSHA256'] and sha(compose) == plan['proposedComposeSHA256'], 'config_delta_changed')
        journal['status'] = 'replacing'
        private_write(journal_path, journal)
        for name, item in plan['request']['backend'].items():
            value = plan['current']['openim/bin/' + name]
            backend.atomic_copy(stage_file(item['path']), BASE / 'openim/bin' / name,
                                item['sha256'], value['mode'], value['uid'], value['gid'])
        value = plan['current'][MAIN]
        main = plan['request']['main']
        backend.atomic_copy(stage_file(main['path']), BASE / MAIN, main['sha256'], value['mode'], value['uid'], value['gid'])
        for relative, raw in ((CRON, cron), (COMPOSE, compose)):
            value = plan['current'][relative]
            web.atomic_write(BASE / relative, raw, value['mode'], value['uid'], value['gid'])
        manifest = json_file(transaction / 'previous' / MANIFEST)
        manifest.update({name: {'bytes': item['bytes'], 'sha256': item['sha256']}
                         for name, item in plan['request']['backend'].items()})
        value = plan['current'][MANIFEST]
        web.atomic_write(BASE / MANIFEST, canonical(manifest), value['mode'], value['uid'], value['gid'])
        archive = plan['request']['web']
        payload = web.inspect_archive(stage_file(archive['path']), archive['sha256'], archive['indexSHA256'], 'hrm')
        old_index = plan['current']['gosuxin/resource/suxinweb/index.html']
        web.publish(BASE / WEB, payload, transaction / 'web', old_index['uid'], old_index['gid'])
        require(plugin_proof() == plan['plugin'], 'active_plugin_changed')
        journal['status'] = 'starting'
        private_write(journal_path, journal)
        start_writers(backend)
        actual = owned()
        require(all(actual[name]['Id'] == plan['containers'][name]['id']
                    and actual[name]['Image'] == plan['containers'][name]['image'] for name in STORAGE),
                'storage_or_livekit_was_recreated')
        require(all(digest(BASE / 'openim/bin' / name) == item['sha256'] for name, item in plan['request']['backend'].items())
                and digest(BASE / MAIN) == main['sha256'] and digest(BASE / WEB / 'index.html') == archive['indexSHA256']
                and plugin_proof() == plan['plugin'], 'published_hash_proof_failed')
        require(volume_guard(actual) == plan['volumes'], 'published_storage_volume_changed')
        capacity_mount_proof(actual, plan['volumes'])
        running_process_proof(actual, {name: item['sha256'] for name, item in plan['request']['backend'].items()}, main['sha256'])
        journal['status'] = 'committed'
        private_write(journal_path, journal)
    except BaseException:
        try:
            rollback(transaction, plan, web, backend)
            journal['status'] = 'rolledBack'
            private_write(journal_path, journal)
        except BaseException:
            journal['status'] = 'recoveryRequired'
            try:
                stop_writers()
            except BaseException:
                pass
            try:
                private_write(journal_path, journal)
            except BaseException:
                pass
            raise RuntimeError('release_rollback_incomplete_writers_not_resumed') from None
        raise RuntimeError('release_failed_verified_rollback_completed') from None
    return {'committed': True, 'transaction': str(transaction), 'binaries': 17,
            'pluginUnchanged': True, 'capacityVerified': False,
            'requiresNormalBusinessAcceptance': True, 'productionCredentialIssued': False}


def main():
    parser = argparse.ArgumentParser()
    commands = parser.add_subparsers(dest='command', required=True)
    local = commands.add_parser('prepare-receipt')
    for name in ('archive', 'manifest', 'restore-check', 'out'):
        local.add_argument('--' + name, required=True, type=pathlib.Path)
    plan = commands.add_parser('plan')
    plan.add_argument('--request', required=True)
    plan.add_argument('--request-sha256', required=True)
    plan.add_argument('--out', required=True)
    action = commands.add_parser('apply')
    action.add_argument('--plan', required=True)
    action.add_argument('--plan-sha256', required=True)
    recover = commands.add_parser('recover')
    recover.add_argument('--transaction', required=True)
    args = parser.parse_args()
    if args.command == 'prepare-receipt':
        value = backup_receipt(args.archive.resolve(), args.manifest.resolve(), args.restore_check.resolve())
        private_write(args.out.resolve(), value, True)
        print(json.dumps({'receipt': str(args.out), 'sha256': digest(args.out), 'keyIncluded': False}))
        return
    import fcntl
    owner_guard()
    web, backend = load_helpers()
    with regular(BASE / '.backup-stage/backup.lock').open('rb') as lock:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        if args.command == 'plan':
            path = stage_file(args.request)
            value = build_plan(json_file(path, args.request_sha256), args.request_sha256, web)
            out = BASE / 'staging' / args.out
            require(out.parent.resolve() == out.parent and out.is_relative_to(BASE / 'staging'), 'plan_output_invalid')
            private_write(out, value, True)
            print(json.dumps({'planned': True, 'sha256': digest(out), 'binaries': 17, 'productionChanged': False}))
        elif args.command == 'apply':
            value = json_file(stage_file(args.plan), args.plan_sha256)
            require(value.get('format') == 'puty-security-plan-1' and value.get('instanceID') == INSTANCE, 'plan_invalid')
            for signum in (signal.SIGHUP, signal.SIGINT, signal.SIGTERM):
                signal.signal(signum, lambda *_: (_ for _ in ()).throw(InterruptedError('signal')))
            print(json.dumps(apply(value, web, backend)))
        else:
            transaction = pathlib.Path(args.transaction)
            require(transaction.parent == BASE / 'backups' and transaction.resolve() == transaction
                    and re.fullmatch('security-policy-[a-f0-9]{32}', transaction.name), 'recovery_path_invalid')
            journal = json_file(transaction / 'journal.json')
            require(journal.get('instanceID') == INSTANCE and journal.get('status') not in ('committed', 'rolledBack'),
                    'transaction_already_terminal')
            value = json_file(transaction / 'plan.json', journal['planSHA256'])
            recover_transaction(transaction, journal, value, web, backend)
            print(json.dumps({'rolledBack': True, 'pluginUnchanged': True}))


if __name__ == '__main__':
    try:
        main()
    except BaseException as error:
        code = str(error) if type(error) is RuntimeError and re.fullmatch('[a-z0-9_]+', str(error)) else 'release_dependency_or_input_error'
        print(json.dumps({'passed': False, 'error': code, 'credentialsPrinted': False}))
        sys.exit(1)
