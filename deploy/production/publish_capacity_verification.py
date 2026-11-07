#!/usr/bin/env python3
"""Verify the two actual owned data mounts before enabling capacity reporting."""
import argparse
import copy
import hashlib
import importlib.util
import json
import os
import pathlib
import re
import signal
import sys
import time
import uuid

import yaml

PINS = {'publish_security_policy': '48dd824fdc36afa631bf42aba63fb5cd798bdfb5131a14cd17b109e04d784443',
        'audit_security_retention': '0f9113ac26a82f2ee6ea2eb6c2e29eeb8df3e2241d55ddc83e27ab7bfb95be64'}
FLAG = 'STORAGE_CAPACITY_DATA_VERIFIED'
SERVICE = 'enterprise-service'


def require(value, code):
    if not value:
        raise RuntimeError(code)


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def load_helpers():
    result = {}
    for name, expected in PINS.items():
        path = pathlib.Path(__file__).resolve().with_name(name + '.py')
        meta = path.lstat()
        require(path.resolve() == path and path.is_file() and meta.st_nlink == 1
                and meta.st_uid == 0 and not meta.st_mode & 0o022
                and sha(path.read_bytes()) == expected, 'reviewed_helper_required')
        spec = importlib.util.spec_from_file_location(name, path)
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        result[name] = module
    return result['publish_security_policy'], result['audit_security_retention']


def stage_output(worker, relative):
    # stage_file is deliberately for existing, reviewed apply inputs. A plan
    # output must instead name a new file under an existing canonical parent.
    require(isinstance(relative, str) and re.fullmatch(r'[A-Za-z0-9][A-Za-z0-9_./-]{0,255}', relative)
            and all(part not in ('', '.', '..') for part in relative.split('/')), 'capacity_output_path_invalid')
    staging = worker.BASE / 'staging'
    path = staging / relative
    require(staging.is_dir() and not staging.is_symlink() and staging.resolve() == staging
            and path.is_relative_to(staging) and path.parent.is_dir()
            and not path.parent.is_symlink() and path.parent.resolve() == path.parent
            and path.resolve() == path, 'capacity_output_parent_invalid')
    require(not path.exists() and not path.is_symlink(), 'capacity_plan_already_exists')
    return path


def proposed(raw):
    original = yaml.safe_load(raw)
    require(isinstance(original, dict) and isinstance(original.get('services'), dict), 'compose_schema_invalid')
    environment = original['services'].get(SERVICE, {}).get('environment')
    require(isinstance(environment, dict) and environment.get(FLAG) == 'false', 'unverified_flag_required')
    pattern = rb'(?m)^([ \t]+STORAGE_CAPACITY_DATA_VERIFIED: )([\'\"]?)false\2([ \t]*\r?)$'
    result, count = re.subn(pattern, lambda m: m[1] + m[2] + b'true' + m[2] + m[3], raw)
    require(count == 1, 'exact_flag_line_required')
    changed = yaml.safe_load(result)
    require(changed['services'][SERVICE]['environment'][FLAG] == 'true', 'flag_must_remain_string')
    restored = copy.deepcopy(changed)
    restored['services'][SERVICE]['environment'][FLAG] = 'false'
    require(restored == original, 'unrelated_compose_delta')
    return result


def immutable_container(row):
    # The Compose config-hash changes when this single environment value changes.
    config = copy.deepcopy(row['Config'])
    config['Env'] = sorted(value for value in config.get('Env', []) if value.partition('=')[0] != FLAG)
    config['Labels'].pop('com.docker.compose.config-hash', None)
    config['Labels'].pop('com.docker.compose.replace', None)
    if config.get('Hostname') == row['Id'][:12]:
        config['Hostname'] = '<automatic-container-hostname>'
    return {'image': row['Image'], 'config': config, 'host': row['HostConfig'],
            'mounts': sorted(row.get('Mounts', []), key=lambda item: item['Destination'])}


def basis(worker, auditor):
    worker.owner_guard()
    containers = worker.owned()
    volumes = worker.volume_guard(containers)
    worker.capacity_mount_proof(containers, volumes)
    capacity = auditor.capacity_summary(containers)
    require(len(capacity) == 2 and {row['kind'] for row in capacity} == {'mongo', 'minio'}
            and all(row['ownedVolume'] and row['capacityAliasReadonly'] and not row['operatorVerified']
                    for row in capacity), 'actual_capacity_mounts_unverified_or_invalid')
    raw = (worker.BASE / worker.COMPOSE).read_bytes()
    new = proposed(raw)
    return {'format': 'puty-capacity-verification-1', 'instanceID': worker.INSTANCE,
            'publisherSHA256': sha(pathlib.Path(__file__).read_bytes()),
            'compose': worker.metadata(worker.BASE / worker.COMPOSE), 'candidateSHA256': sha(new),
            'containers': {name: {'id': row['Id'], 'image': row['Image']} for name, row in containers.items()},
            'volumes': volumes, 'plugin': worker.plugin_proof(),
            'enterpriseSHA256': worker.digest(worker.BASE / 'openim/bin/enterprise-service'),
            'enterpriseConfigurationSHA256': sha(worker.canonical(immutable_container(containers[SERVICE])))}


def unchanged(worker, value, containers, allow_missing=False):
    expected = set(value['containers'])
    require(set(containers) == expected or allow_missing and set(containers) == expected - {SERVICE},
            'capacity_service_set_changed')
    require(worker.volume_guard(containers) == value['volumes'] and worker.plugin_proof() == value['plugin'],
            'owned_data_or_plugin_changed')
    require({name: {'id': row['Id'], 'image': row['Image']} for name, row in containers.items() if name != SERVICE}
            == {name: row for name, row in value['containers'].items() if name != SERVICE},
            'unrelated_container_changed')
    if SERVICE in containers:
        require(sha(worker.canonical(immutable_container(containers[SERVICE]))) == value['enterpriseConfigurationSHA256'],
                'unrelated_enterprise_configuration_changed')


def restart(worker):
    worker.execute(worker.compose_command() + ['up', '-d', '--no-deps', '--no-build', '--force-recreate',
                                               '--pull', 'never', SERVICE])
    for _ in range(30):
        if worker.ready('http://127.0.0.1:18090/enterprise/v1/readyz'):
            return
        time.sleep(2)
    raise RuntimeError('capacity_service_readiness_unconfirmed')


def install(worker, raw, metadata):
    path = worker.BASE / worker.COMPOSE
    temporary = path.with_name('.capacity-next-' + uuid.uuid4().hex)
    try:
        with temporary.open('xb') as output:
            os.chmod(temporary, metadata['mode'])
            os.chown(temporary, metadata['uid'], metadata['gid'])
            output.write(raw)
            output.flush()
            os.fsync(output.fileno())
        temporary.replace(path)
        worker.sync_directory(path.parent)
    finally:
        temporary.unlink(missing_ok=True)


def rollback(worker, auditor, transaction, value):
    worker.owner_guard()
    containers = worker.owned(False, True)
    unchanged(worker, value, containers, allow_missing=True)
    original = worker.regular(transaction / 'compose.previous').read_bytes()
    require(sha(original) == value['compose']['sha256'], 'rollback_original_changed')
    require(worker.digest(worker.BASE / worker.COMPOSE) in (value['compose']['sha256'], value['candidateSHA256']),
            'rollback_compose_changed_by_another_writer')
    require(worker.digest(worker.BASE / 'openim/bin/enterprise-service') == value['enterpriseSHA256'],
            'rollback_enterprise_binary_changed')
    worker.execute(worker.compose_command() + ['stop', SERVICE])
    install(worker, original, value['compose'])
    restart(worker)
    containers = worker.owned()
    unchanged(worker, value, containers)
    worker.capacity_mount_proof(containers, value['volumes'])
    worker.process_proof(containers[SERVICE]['State']['Pid'], value['enterpriseSHA256'])
    require(worker.metadata(worker.BASE / worker.COMPOSE) == value['compose'], 'rollback_compose_metadata_mismatch')
    worker.private_write(transaction / 'journal.json', {'status': 'rolledBack', 'instanceID': worker.INSTANCE,
                                                       'planSHA256': sha(worker.canonical(value))})


def apply(worker, auditor, value):
    require(value == basis(worker, auditor), 'capacity_reviewed_plan_changed')
    transaction = worker.BASE / 'backups' / ('capacity-verification-' + uuid.uuid4().hex)
    transaction.mkdir(mode=0o700)
    worker.sync_directory(transaction.parent)
    original = (worker.BASE / worker.COMPOSE).read_bytes()
    require(sha(original) == value['compose']['sha256']
            and sha(proposed(original)) == value['candidateSHA256'], 'capacity_original_changed')
    # Backup bytes, owner/mode and plan are durable before stopping this one service.
    with (transaction / 'compose.previous').open('xb') as output:
        os.chmod(output.name, 0o600)
        output.write(original)
        output.flush()
        os.fsync(output.fileno())
    worker.private_write(transaction / 'plan.json', value, exclusive=True)
    worker.private_write(transaction / 'journal.json', {'status': 'prepared', 'instanceID': worker.INSTANCE,
                                                       'planSHA256': sha(worker.canonical(value))}, exclusive=True)
    worker.sync_directory(transaction)
    try:
        require(worker.metadata(worker.BASE / worker.COMPOSE) == value['compose'], 'capacity_compose_changed_before_stop')
        unchanged(worker, value, worker.owned())
    except BaseException:
        worker.private_write(transaction / 'journal.json', {'status': 'aborted', 'instanceID': worker.INSTANCE,
                                                           'planSHA256': sha(worker.canonical(value))})
        raise
    try:
        worker.execute(worker.compose_command() + ['stop', SERVICE])
        install(worker, proposed(original), value['compose'])
        restart(worker)
        containers = worker.owned()
        unchanged(worker, value, containers)
        capacity = auditor.capacity_summary(containers)
        require(len(capacity) == 2 and {row['kind'] for row in capacity} == {'mongo', 'minio'}
                and all(row['ownedVolume'] and row['capacityAliasReadonly'] and row['operatorVerified'] for row in capacity),
                'capacity_actual_verification_failed')
        worker.process_proof(containers[SERVICE]['State']['Pid'], value['enterpriseSHA256'])
        require(worker.digest(worker.BASE / worker.COMPOSE) == value['candidateSHA256'], 'capacity_compose_hash_mismatch')
        worker.private_write(transaction / 'journal.json', {'status': 'committed', 'instanceID': worker.INSTANCE,
                                                           'planSHA256': sha(worker.canonical(value))})
        return {'passed': True, 'transaction': str(transaction), 'capacity': capacity,
                'composeSHA256': value['candidateSHA256'], 'unrelatedContainersUnchanged': 23}
    except BaseException:
        try:
            rollback(worker, auditor, transaction, value)
        except BaseException:
            # The old configuration remains available for the explicit recover command.
            try:
                worker.execute(worker.compose_command() + ['stop', SERVICE])
            finally:
                raise RuntimeError('capacity_rollback_requires_operator') from None
        raise RuntimeError('capacity_failed_verified_rollback_complete') from None


def main():
    parser = argparse.ArgumentParser()
    sub = parser.add_subparsers(dest='command', required=True)
    plan = sub.add_parser('plan')
    plan.add_argument('--out', required=True)
    use = sub.add_parser('apply')
    use.add_argument('--plan', required=True)
    use.add_argument('--plan-sha256', required=True)
    recover = sub.add_parser('recover')
    recover.add_argument('--transaction', required=True)
    args = parser.parse_args()
    worker, auditor = load_helpers()
    worker.owner_guard()
    import fcntl
    with worker.regular(worker.BASE / '.backup-stage/backup.lock').open('rb') as lock:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        for signum in (signal.SIGHUP, signal.SIGINT, signal.SIGTERM):
            signal.signal(signum, lambda *_: (_ for _ in ()).throw(InterruptedError('signal')))
        if args.command == 'plan':
            value = basis(worker, auditor)
            out = stage_output(worker, args.out)
            worker.private_write(out, value, exclusive=True)
            print(json.dumps({'planned': True, 'planSHA256': worker.digest(out), **value}))
        elif args.command == 'apply':
            value = worker.json_file(worker.stage_file(args.plan), args.plan_sha256)
            print(json.dumps(apply(worker, auditor, value)))
        else:
            transaction = pathlib.Path(args.transaction)
            require(transaction.resolve() == transaction and transaction.parent == worker.BASE / 'backups'
                    and re.fullmatch('capacity-verification-[0-9a-f]{32}', transaction.name), 'capacity_transaction_invalid')
            journal = worker.json_file(transaction / 'journal.json')
            require(journal.get('status') == 'prepared' and journal.get('instanceID') == worker.INSTANCE,
                    'capacity_recovery_not_required')
            require(re.fullmatch('[0-9a-f]{64}', journal.get('planSHA256', '')), 'capacity_recovery_plan_hash_missing')
            value = worker.json_file(transaction / 'plan.json', journal['planSHA256'])
            require(value.get('publisherSHA256') == sha(pathlib.Path(__file__).read_bytes())
                    and value.get('instanceID') == worker.INSTANCE, 'capacity_recovery_plan_invalid')
            try:
                rollback(worker, auditor, transaction, value)
            except BaseException:
                worker.execute(worker.compose_command() + ['stop', SERVICE])
                raise
            print(json.dumps({'passed': True, 'rolledBack': True, 'transaction': str(transaction)}))


if __name__ == '__main__':
    try:
        main()
    except BaseException as error:
        code = str(error) if type(error) is RuntimeError and re.fullmatch('[a-z0-9_]+', str(error)) else 'capacity_dependency_or_input_error'
        print(json.dumps({'passed': False, 'error': code, 'credentialsPrinted': False}))
        sys.exit(1)
