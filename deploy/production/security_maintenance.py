#!/usr/bin/env python3
"""Reversible four-public-vhost maintenance gate; leave both private bridges intact."""
import argparse
import hashlib
import importlib.util
import ipaddress
import json
import os
import pathlib
import re
import signal
import stat
import sys
import uuid

COMMON_SHA = '6e061c229324a4e50ecf6d375334b452ec4e13dca5c499cf1b71c0b1cda6afc5'
COMMON_PATH = pathlib.Path(__file__).with_name('publish_resource_proof_cors.py')
if hashlib.sha256(COMMON_PATH.read_bytes()).hexdigest() != COMMON_SHA:
    raise RuntimeError('maintenance_common_dependency_changed')
_spec = importlib.util.spec_from_file_location('puty_maintenance_common', COMMON_PATH)
common = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(common)

BASE, INSTANCE, VHOST = common.BASE, common.INSTANCE, common.VHOST
guard, regular, metadata = common.guard, common.regular, common.metadata
atomic, sync, nginx = common.atomic, common.sync, common.nginx
require, sha, canonical, staged = common.require, common.sha, common.canonical, common.staged
OLD_SHA = '2571b5ec35e067ff30c225de1a08ddc2b73484711b37908930230d3d38f06cc1'
MARKER = b'puty_security_maintenance'
PUBLIC = (
    (b' listen 9880 ssl default_server;', b' server_name 192.168.10.11;'),
    (b' listen 9880 ssl;', b' server_name hr.szputy.com;'),
    (b' listen 9885 ssl default_server;', b' server_name 192.168.10.11;'),
    (b' listen 9885 ssl;', b' server_name im.szputy.com;'),
)
PRIVATE = (b' listen 172.30.38.1:18001;', b' listen 172.30.38.1:17880;')


def operator_ip(value):
    require(isinstance(value, str), 'operator_ip_invalid')
    try:
        address = ipaddress.ip_address(value)
    except ValueError:
        raise RuntimeError('operator_ip_invalid') from None
    require(address.compressed == value and not (address.is_unspecified or address.is_multicast
            or address.is_loopback or address.is_link_local or address.is_reserved), 'operator_ip_invalid')
    return value


def current_operator(value):
    address = operator_ip(value)
    parts = os.environ.get('SSH_CONNECTION', '').split()
    require(len(parts) == 4 and parts[0] == address and all(
        part.isdigit() and 0 < int(part) < 65536 for part in (parts[1], parts[3])), 'ssh_operator_mismatch')
    try:
        ipaddress.ip_address(parts[2])
    except ValueError:
        raise RuntimeError('ssh_operator_mismatch') from None
    return address


def candidate(raw, operator):
    address = operator_ip(operator).encode('ascii')
    require(sha(raw) == OLD_SHA and MARKER not in raw, 'exact_original_vhost_required')
    newline = b'\r\n' if b'\r\n' in raw else b'\n'
    require(raw.count(newline) == raw.count(b'\n'), 'mixed_vhost_newlines')
    heads = list(re.finditer(rb'(?m)^server \{\r?$', raw))
    require(len(heads) == 6, 'six_reviewed_vhosts_required')
    segments = [raw[head.start():heads[index + 1].start() if index + 1 < len(heads) else len(raw)]
                for index, head in enumerate(heads)]
    for index, (listen, name) in enumerate(PUBLIC):
        require(segments[index].startswith(newline.join((b'server {', listen, name)) + newline),
                'public_vhost_shape_changed')
    for index, listen in enumerate(PRIVATE, 4):
        require(segments[index].startswith(newline.join((b'server {', listen, b' server_name _;')) + newline),
                'private_vhost_shape_changed')
    addition = b' if ($puty_security_maintenance) { return 503; }' + newline
    for index in range(4):
        segments[index] = segments[index].replace(b'server {' + newline,
                                                 b'server {' + newline + addition, 1)
    prefix = newline.join((b'# PUTY temporary security-release maintenance; private bridges remain available.',
                          b'map $remote_addr $puty_security_maintenance {', b' default 1;',
                          b' ' + address + b' 0;', b' 127.0.0.1 0;', b' ::1 0;', b'}', b''))
    changed = prefix + raw[:heads[0].start()] + b''.join(segments)
    require(changed[len(prefix):].replace(addition, b'') == raw and changed.count(addition) == 4,
            'maintenance_diff_not_exact')
    return changed


def gated_metadata(value, raw):
    return {**value['current'], 'sha256': sha(raw), 'bytes': len(raw)}


def plan(operator):
    guard()
    current = metadata(VHOST)
    address = current_operator(operator)
    raw = candidate(VHOST.read_bytes(), address)
    return {'format': 'puty-security-maintenance-1', 'instanceID': INSTANCE,
            'publisherSHA256': sha(pathlib.Path(__file__).read_bytes()), 'operatorIP': address,
            'current': current, 'candidateSHA256': sha(raw), 'candidateBytes': len(raw),
            'publicVhosts': 4, 'unchangedPrivateBridges': 2}


def journal(transaction, value, status):
    atomic(transaction / 'journal.json', canonical({'instanceID': INSTANCE,
        'planSHA256': sha(canonical(value)), 'status': status}))


def known_current(value, raw):
    require(metadata(VHOST) in (value['current'], gated_metadata(value, raw)),
            'vhost_changed_by_another_writer')


def set_vhost(value, raw):
    old = value['current']
    atomic(VHOST, raw, old['mode'], old['uid'], old['gid'])
    nginx('-t')
    nginx('-s', 'reload')
    require(metadata(VHOST) == gated_metadata(value, raw), 'vhost_metadata_mismatch')


def close(value):
    require(value == plan(value.get('operatorIP')), 'reviewed_plan_changed')
    original = VHOST.read_bytes()
    gated = candidate(original, value['operatorIP'])
    transaction = BASE / 'backups' / ('security-maintenance-' + uuid.uuid4().hex)
    transaction.mkdir(mode=0o700)
    sync(transaction.parent)
    atomic(transaction / 'previous.conf', original)
    atomic(transaction / 'plan.json', canonical(value))
    journal(transaction, value, 'prepared')
    try:
        known_current(value, gated)
        set_vhost(value, gated)
        journal(transaction, value, 'closed')
    except BaseException:
        try:
            known_current(value, gated)
            set_vhost(value, original)
            journal(transaction, value, 'rolledBack')
        except BaseException:
            raise RuntimeError('maintenance_close_failed_rollback_requires_operator') from None
        raise RuntimeError('maintenance_close_failed_verified_rollback_complete') from None
    return {'passed': True, 'status': 'closed', 'transaction': str(transaction),
            'planSHA256': sha(canonical(value)), 'sha256': sha(gated), 'operatorIP': value['operatorIP'],
            'publicVhosts': 4, 'unchangedPrivateBridges': 2}


def load_transaction(path, expected_plan_sha):
    guard()
    transaction = pathlib.Path(path)
    require(transaction.parent == BASE / 'backups' and re.fullmatch(r'security-maintenance-[a-f0-9]{32}',
            transaction.name) and transaction.resolve() == transaction, 'maintenance_transaction_invalid')
    private_directory(transaction)
    for name in ('previous.conf', 'plan.json', 'journal.json'):
        regular(transaction / name)
    value = json.loads((transaction / 'plan.json').read_bytes())
    require(sha(canonical(value)) == expected_plan_sha and value['instanceID'] == INSTANCE
            and value['format'] == 'puty-security-maintenance-1'
            and value['publisherSHA256'] == sha(pathlib.Path(__file__).read_bytes()), 'maintenance_plan_mismatch')
    previous = (transaction / 'previous.conf').read_bytes()
    require(sha(previous) == value['current']['sha256'] == OLD_SHA
            and len(previous) == value['current']['bytes'], 'maintenance_backup_mismatch')
    gated = candidate(previous, value['operatorIP'])
    require(sha(gated) == value['candidateSHA256'] and len(gated) == value['candidateBytes'],
            'maintenance_candidate_mismatch')
    state = json.loads((transaction / 'journal.json').read_bytes())
    require(state.get('instanceID') == INSTANCE and state.get('planSHA256') == expected_plan_sha
            and state.get('status') in ('prepared', 'closed', 'releaseIntent', 'released', 'rolledBack',
                                        'releaseFailedClosed'), 'maintenance_journal_mismatch')
    return transaction, value, previous, gated


def private_directory(path):
    info = path.lstat()
    require(stat.S_ISDIR(info.st_mode) and info.st_uid == 0 and not info.st_mode & 0o077,
            'maintenance_transaction_not_private')


def release(path, expected_plan_sha):
    transaction, value, original, gated = load_transaction(path, expected_plan_sha)
    known_current(value, gated)
    journal(transaction, value, 'releaseIntent')
    try:
        set_vhost(value, original)
        journal(transaction, value, 'released')
    except BaseException:
        try:
            known_current(value, gated)
            set_vhost(value, gated)
            journal(transaction, value, 'releaseFailedClosed')
        except BaseException:
            raise RuntimeError('maintenance_release_failed_requires_operator') from None
        raise RuntimeError('maintenance_release_failed_gate_preserved') from None
    return {'passed': True, 'status': 'released', 'transaction': str(transaction),
            'sha256': value['current']['sha256'], 'publicVhosts': 4, 'unchangedPrivateBridges': 2}


def main():
    parser = argparse.ArgumentParser()
    sub = parser.add_subparsers(dest='command', required=True)
    create = sub.add_parser('plan')
    create.add_argument('--operator-ip', required=True)
    create.add_argument('--out', required=True)
    use = sub.add_parser('close')
    use.add_argument('--plan', required=True)
    use.add_argument('--plan-sha256', required=True)
    finish = sub.add_parser('open')
    finish.add_argument('--transaction', required=True)
    finish.add_argument('--plan-sha256', required=True)
    args = parser.parse_args()
    import fcntl
    guard()
    lock = BASE / '.backup-stage/backup.lock'
    regular(lock)
    with lock.open('rb') as handle:
        fcntl.flock(handle, fcntl.LOCK_EX | fcntl.LOCK_NB)
        for signum in (signal.SIGHUP, signal.SIGINT, signal.SIGTERM):
            signal.signal(signum, lambda *_: (_ for _ in ()).throw(InterruptedError('signal')))
        if args.command == 'plan':
            value = plan(args.operator_ip)
            out = staged(args.out)
            require(not out.exists(), 'plan_already_exists')
            atomic(out, canonical(value))
            print(json.dumps({'planned': True, 'planSHA256': sha(out.read_bytes()), **value}))
        elif args.command == 'close':
            path = staged(args.plan)
            regular(path)
            raw = path.read_bytes()
            require(sha(raw) == args.plan_sha256 and raw == canonical(json.loads(raw)), 'plan_hash_mismatch')
            print(json.dumps(close(json.loads(raw))))
        else:
            print(json.dumps(release(args.transaction, args.plan_sha256)))


if __name__ == '__main__':
    try:
        main()
    except BaseException as error:
        code = str(error) if type(error) is RuntimeError and re.fullmatch('[a-z0-9_]+', str(error)) else 'maintenance_dependency_or_input_error'
        print(json.dumps({'passed': False, 'error': code, 'credentialsPrinted': False}))
        sys.exit(1)
