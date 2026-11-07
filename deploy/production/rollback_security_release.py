#!/usr/bin/env python3
"""Explicit post-health-check rollback, only before the closed maintenance gate opens.

Keep the original publisher immutable. Its ordinary recovery deliberately rejects
committed releases; this separate reviewed plan first proves all installed bytes
still belong to that exact release, then invokes the same tested compensation.
"""
import argparse
import hashlib
import importlib.util
import json
import os
import pathlib
import re
import signal
import sys

PINS = {'publish_security_policy':'48dd824fdc36afa631bf42aba63fb5cd798bdfb5131a14cd17b109e04d784443',
        'security_maintenance':'cc25d1fbb207147b37cb65e7b3804c0ed5062f1a019d6529618be4120b6f8d79'}


def require(value, code):
    if not value:
        raise RuntimeError(code)


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def load_helpers():
    modules = []
    for name, expected in PINS.items():
        path = pathlib.Path(__file__).resolve().with_name(name + '.py')
        meta = path.lstat()
        require(path.resolve() == path and path.is_file() and meta.st_nlink == 1
                and meta.st_uid == 0 and not meta.st_mode & 0o022 and sha(path.read_bytes()) == expected,
                'reviewed_rollback_dependency_required')
        spec = importlib.util.spec_from_file_location('rollback_' + name, path)
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        modules.append(module)
    return modules


def transaction_path(worker, value):
    path = pathlib.Path(value)
    require(path.parent == worker.BASE / 'backups' and path.resolve() == path
            and re.fullmatch(r'security-policy-[a-f0-9]{32}', path.name), 'rollback_transaction_invalid')
    return path


def installed_proof(worker, web, transaction, plan):
    """Fail before any stop/write if a later release or external edit occurred."""
    require(plan.get('publisherSHA256') == PINS['publish_security_policy']
            and plan.get('instanceID') == worker.INSTANCE and plan.get('format') == 'puty-security-plan-1',
            'original_security_plan_invalid')
    require(set(plan['request']['backend']) == set(worker.BINARIES)
            and set(plan['current']) == {worker.MAIN, worker.CRON, worker.COMPOSE, worker.MANIFEST,
                'gosuxin/resource/suxinweb/index.html', *('openim/bin/' + key for key in worker.BINARIES)},
            'original_security_target_set_invalid')
    values = worker.recovery_guard(plan)
    worker.running_process_proof(values,
        {name:item['sha256'] for name,item in plan['request']['backend'].items()}, plan['request']['main']['sha256'])
    worker.capacity_mount_proof(values, plan['volumes'])  # Must still be the initial false flag/configuration.
    result, backups = {}, {}
    for name, old in plan['current'].items():
        if name.endswith('/suxinweb/index.html'):
            continue
        backup = transaction / 'previous' / name
        require(worker.digest(worker.regular(backup)) == old['sha256'], 'rollback_original_backup_changed')
        backups[name] = worker.metadata(backup)
        raw = worker.regular(worker.BASE / name, root_owned=False).read_bytes()
        if name in {'openim/bin/' + key for key in worker.BINARIES}:
            entry = plan['request']['backend'][name.rsplit('/',1)[1]]
            expected_sha, expected_size = entry['sha256'], entry['bytes']
        elif name == worker.MAIN:
            entry = plan['request']['main']
            expected_sha, expected_size = entry['sha256'], entry['bytes']
        elif name in (worker.CRON, worker.COMPOSE):
            expected_sha = plan['proposedCronSHA256' if name == worker.CRON else 'proposedComposeSHA256']
            expected_size = len(raw)
        elif name == worker.MANIFEST:
            expected = worker.json_file(backup)
            expected.update({key:{'bytes':entry['bytes'], 'sha256':entry['sha256']}
                             for key,entry in plan['request']['backend'].items()})
            encoded = worker.canonical(expected)
            expected_sha, expected_size = sha(encoded), len(encoded)
        else:
            raise RuntimeError('rollback_unexpected_target')
        meta = worker.metadata(worker.BASE / name, root_owned=False)
        require(meta == {**old, 'sha256':expected_sha, 'bytes':expected_size}
                and sha(raw) == expected_sha, 'installed_security_release_changed')
        result[name] = meta
    archive = plan['request']['web']
    payload = web.inspect_archive(worker.stage_file(archive['path']), archive['sha256'], archive['indexSHA256'], 'hrm')
    receipt_path = transaction / 'web/release.json'
    receipt = worker.json_file(receipt_path)
    require(receipt.get('format') == 'puty-static-release-v2'
            and receipt.get('newFiles') == {name:sha(raw) for name,raw in payload.items()}
            and receipt.get('previousFiles', {}).get('index.html') == plan['current']['gosuxin/resource/suxinweb/index.html']['sha256'],
            'rollback_web_receipt_changed')
    old_index = plan['current']['gosuxin/resource/suxinweb/index.html']
    for name, raw in payload.items():
        path = worker.BASE / worker.WEB / name
        require(path.resolve() == path and path.is_relative_to(worker.BASE / worker.WEB), 'rollback_web_path_invalid')
        expected = {'sha256':sha(raw), 'bytes':len(raw), 'mode':0o644, 'uid':old_index['uid'], 'gid':old_index['gid']}
        result[str(path.relative_to(worker.BASE))] = worker.metadata(path, root_owned=False)
        require(result[str(path.relative_to(worker.BASE))] == expected, 'installed_hrm_web_changed')
    for name, digest in receipt['previousFiles'].items():
        path = transaction / 'web' / name
        require(path.resolve() == path and path.is_relative_to(transaction / 'web')
                and worker.digest(worker.regular(path)) == digest, 'rollback_web_backup_changed')
        backups['web/' + name] = worker.metadata(path)
    return {'installed':result, 'backup':backups, 'webReceiptSHA256':worker.digest(receipt_path),
            'containers':{name:{'id':row['Id'], 'image':row['Image']} for name,row in values.items()},
            'volumes':plan['volumes'], 'plugin':worker.plugin_proof()}


def closed_gate(worker, gate, gate_transaction, gate_plan_sha):
    _, gate_plan, _, gated = gate.load_transaction(gate_transaction, gate_plan_sha)
    gate_state = worker.json_file(pathlib.Path(gate_transaction) / 'journal.json')
    require(gate_state.get('status') == 'closed'
            and gate.metadata(gate.VHOST) == gate.gated_metadata(gate_plan, gated), 'closed_maintenance_gate_required')


def basis(worker, gate, transaction_value, gate_transaction, gate_plan_sha, web):
    worker.owner_guard()
    closed_gate(worker, gate, gate_transaction, gate_plan_sha)
    transaction = transaction_path(worker, transaction_value)
    journal = worker.json_file(transaction / 'journal.json')
    require(journal.get('instanceID') == worker.INSTANCE and journal.get('status') == 'committed',
            'exact_committed_security_release_required')
    plan = worker.json_file(transaction / 'plan.json', journal['planSHA256'])
    proof = installed_proof(worker, web, transaction, plan)
    return {'format':'puty-committed-security-rollback-1', 'instanceID':worker.INSTANCE,
            'publisherSHA256':sha(pathlib.Path(__file__).read_bytes()), 'transaction':str(transaction),
            'securityPlanSHA256':journal['planSHA256'], 'securityJournalSHA256':worker.digest(transaction / 'journal.json'),
            'maintenanceTransaction':gate_transaction, 'maintenancePlanSHA256':gate_plan_sha,
            'maintenanceJournalSHA256':worker.digest(pathlib.Path(gate_transaction) / 'journal.json'), **proof}


def apply(worker, gate, web, backend, reviewed):
    fresh = basis(worker, gate, reviewed['transaction'], reviewed['maintenanceTransaction'], reviewed['maintenancePlanSHA256'], web)
    require(fresh == reviewed, 'reviewed_rollback_plan_changed')
    transaction = transaction_path(worker, reviewed['transaction'])
    plan = worker.json_file(transaction / 'plan.json', reviewed['securityPlanSHA256'])
    journal = worker.json_file(transaction / 'journal.json', reviewed['securityJournalSHA256'])
    intent = transaction / 'committed-rollback.json'
    if intent.exists():
        require(worker.json_file(intent) == reviewed, 'different_committed_rollback_already_prepared')
        with worker.regular(intent).open('r+b') as handle:
            os.fsync(handle.fileno())
        worker.sync_directory(intent.parent)
    else:
        worker.private_write(intent, reviewed, True)
    journal['status'] = 'rollbackIntent'
    worker.private_write(transaction / 'journal.json', journal)
    worker.recover_transaction(transaction, journal, plan, web, backend)
    require(worker.json_file(transaction / 'journal.json').get('status') == 'rolledBack', 'rollback_not_verified')
    closed_gate(worker, gate, reviewed['maintenanceTransaction'], reviewed['maintenancePlanSHA256'])
    return {'rolledBack':True, 'transaction':str(transaction), 'maintenanceStillClosed':True,
            'ordinaryStorageAndPluginPreserved':True, 'sqlSchemaRemoved':False}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest='command', required=True)
    create = sub.add_parser('plan')
    create.add_argument('--transaction', required=True)
    create.add_argument('--maintenance-transaction', required=True)
    create.add_argument('--maintenance-plan-sha256', required=True)
    create.add_argument('--out', required=True)
    use = sub.add_parser('apply')
    use.add_argument('--plan', required=True)
    use.add_argument('--plan-sha256', required=True)
    args = parser.parse_args()
    worker, gate = load_helpers()
    worker.owner_guard()
    web, backend = worker.load_helpers()
    import fcntl
    with worker.regular(worker.BASE / '.backup-stage/backup.lock').open('rb') as lock:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        for signum in (signal.SIGHUP, signal.SIGINT, signal.SIGTERM):
            signal.signal(signum, lambda *_: (_ for _ in ()).throw(InterruptedError('signal')))
        if args.command == 'plan':
            value = basis(worker, gate, args.transaction, args.maintenance_transaction, args.maintenance_plan_sha256, web)
            # stage_file checks an existing input; this output must instead be a new canonical private file.
            out = worker.BASE / 'staging' / args.out
            require(re.fullmatch(r'[A-Za-z0-9][A-Za-z0-9_./-]{0,200}', args.out)
                    and all(part not in ('', '.', '..') for part in args.out.split('/'))
                    and out.resolve() == out and out.is_relative_to(worker.BASE / 'staging'), 'rollback_plan_output_invalid')
            worker.private_write(out, value, True)
            print(json.dumps({'planned':True, 'sha256':worker.digest(out), 'productionChanged':False}))
        else:
            value = worker.json_file(worker.stage_file(args.plan), args.plan_sha256)
            require(value.get('format') == 'puty-committed-security-rollback-1', 'rollback_plan_format_invalid')
            print(json.dumps(apply(worker, gate, web, backend, value)))


if __name__ == '__main__':
    try:
        main()
    except SystemExit:
        raise
    except BaseException as error:
        code = str(error) if type(error) is RuntimeError and re.fullmatch('[a-z0-9_]+', str(error)) else 'rollback_dependency_or_input_error'
        print(json.dumps({'passed':False, 'error':code, 'credentialsPrinted':False}))
        sys.exit(1)
