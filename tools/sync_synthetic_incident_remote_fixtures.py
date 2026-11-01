"""Fixed Linux incident109 fixture sync; default probe is read-only, no network.

Run as root on the isolated host only. No SSH, API, arbitrary path or host option.
The replacement payload is supplied separately through a protected staging file.
"""
from __future__ import annotations

import argparse
import copy
import hashlib
import json
import os
from pathlib import Path
import secrets
import stat
import sys

ROOT = Path('/srv/openim38-upgrade')
PRIVATE = ROOT / '.private'
ACCOUNTS = PRIVATE / 'test-accounts.json'
SESSIONS = PRIVATE / 'test-sessions.json'
PAYLOAD = PRIVATE / 'incident109-stage/replacements.json'
BACKUP = PRIVATE / 'incident109-fixture-backup'
CA = ROOT / 'certs/ca.crt'
BASE = 'https://192.168.10.11:29880'
OLD_SHA = 'ad47c3f4650808796f1cd29c78304c2827e73d00c24b5081abac8f57ffb2e968'
NEW_SHA = 'dac846cd06afdff46a204f300dedc9bb15d7328fcbd3ffb47269f987f8c69621'
CA_SHA = '90817f305832a3c2398af0384e76004096647aeeac7095a250c6a5a4845655f0'
IDS = frozenset(f'upgrade38_u{i:03}' for i in range(1, 111))
META = frozenset(('userID', 'account', 'email', 'imToken', 'chatToken', 'platformID'))


class Refused(Exception):
    """Constant safe messages only; never propagate fixture values."""


def need(ok, reason):
    if not ok:
        raise Refused(reason)


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def password_sha(value):
    return sha(value.encode()) if isinstance(value, str) else None


def encode(value):
    return (json.dumps(value, ensure_ascii=False, indent=2) + '\n').encode()


def parse(raw):
    return json.loads(raw.decode('utf-8-sig'))


def checked_path(path, *, existing=True):
    need(path.is_absolute() and path.is_relative_to(ROOT) and '..' not in path.parts,
         'Path outside fixed isolated fixture tree')
    for item in (Path('/srv'), ROOT):
        info = os.lstat(item)
        need(stat.S_ISDIR(info.st_mode) and not stat.S_ISLNK(info.st_mode), 'Unsafe fixture parent')
    current = ROOT
    for part in path.relative_to(ROOT).parts:
        current /= part
        if current == path and not existing and not os.path.lexists(current):
            break
        info = os.lstat(current)
        need(not stat.S_ISLNK(info.st_mode), 'Symlink refused')
        if current != path:
            need(stat.S_ISDIR(info.st_mode), 'Non-directory fixture parent')
    need(path.resolve(strict=existing).is_relative_to(ROOT.resolve(strict=True)), 'Resolved fixture path escapes root')
    return path


def read_regular(path, *, private=True):
    checked_path(path)
    fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW)
    try:
        info = os.fstat(fd)
        need(stat.S_ISREG(info.st_mode), 'Fixture is not a regular file')
        need(info.st_size <= 16 * 1024 * 1024, 'Fixture unexpectedly large')
        if private:
            need(stat.S_IMODE(info.st_mode) == 0o600, 'Private fixture must have mode 600')
        with os.fdopen(fd, 'rb', closefd=False) as source:
            raw = source.read()
        return raw, info
    finally:
        os.close(fd)


def exact_users(value):
    users = value.get('users') if isinstance(value, dict) else None
    return (isinstance(users, list) and len(users) == 110 and
            all(isinstance(u, dict) for u in users) and {u.get('userID') for u in users} == IDS)


def identity_matches(user):
    uid = user.get('userID')
    return uid in IDS and user.get('account') == 'im38u' + uid[-3:] and user.get('email') == 'u' + uid[-3:] + '@openim38.test'


def validate_fixtures(accounts, sessions):
    for value in (accounts, sessions):
        need(isinstance(value, dict) and value.get('baseURL') == BASE, 'Remote fixture baseURL mismatch')
        need(exact_users(value), 'Remote fixture user scope is not exact 001-110')
        need(all(identity_matches(u) for u in value['users']), 'Remote synthetic account/email mismatch')
    need(password_sha(accounts.get('password')) in (OLD_SHA, NEW_SHA), 'Remote shared secret outside incident scope')
    for user in accounts['users']:
        need('password' not in user or password_sha(user['password']) in (OLD_SHA, NEW_SHA), 'Remote per-user secret outside incident scope')
    admin = accounts.get('admin')
    need(isinstance(admin, dict) and isinstance(admin.get('account'), str) and bool(admin['account']) and
         isinstance(admin.get('password'), str) and bool(admin['password']), 'Remote independent administrator must already exist')
    need(password_sha(admin['password']) not in (OLD_SHA, NEW_SHA), 'Remote administrator shares incident secret; refusing')
    for user in sessions['users']:
        need(type(user.get('platformID')) is int and 1 <= user['platformID'] <= 10, 'Remote seed platform outside 1-10')
        need(all(isinstance(user.get(k), str) and user[k] for k in ('imToken', 'chatToken')), 'Remote seed dual token missing')


def validate_payload(value):
    need(isinstance(value, dict) and set(value) == {'schema', 'baseURL', 'caFileSHA256', 'newSecretSHA256', 'password', 'users'},
         'Unexpected replacement payload schema')
    need(value['schema'] == 1 and value['baseURL'] == BASE and value['caFileSHA256'] == CA_SHA and
         value['newSecretSHA256'] == NEW_SHA and password_sha(value['password']) == NEW_SHA,
         'Replacement payload fixed identity/hash mismatch')
    need(exact_users(value), 'Replacement payload user scope is not exact 001-110')
    for user in value['users']:
        need(set(user) == META and identity_matches(user), 'Replacement user identity/schema mismatch')
        need(type(user['platformID']) is int and 1 <= user['platformID'] <= 10, 'Replacement platform outside 1-10')
        need(all(isinstance(user[k], str) and user[k] for k in ('imToken', 'chatToken')), 'Replacement dual token missing')


def file_metadata(info):
    return {'uid': info.st_uid, 'gid': info.st_gid, 'mode': oct(stat.S_IMODE(info.st_mode)), 'bytes': info.st_size}


def fixture_metadata(value):
    known = ('baseURL', 'admin', 'password', 'users', 'adminSession')
    keys = sorted(k for k in known if k in value)
    return {'knownKeys': keys, 'otherKeysCount': len(value) - len(keys),
            'usersCount': len(value.get('users', [])) if isinstance(value.get('users'), list) else None,
            'scoped110': exact_users(value), 'baseMatches': value.get('baseURL') == BASE,
            'identitiesMatch': exact_users(value) and all(identity_matches(u) for u in value['users']),
            'globalPasswordMatchesIncidentOld': password_sha(value.get('password')) == OLD_SHA,
            'globalPasswordMatchesIncidentNew': password_sha(value.get('password')) == NEW_SHA}


def probe():
    need(os.name == 'posix' and hasattr(os, 'geteuid') and os.geteuid() == 0, 'Run only as Linux root on the isolated host')
    checked_path(PRIVATE)
    need(stat.S_IMODE(os.lstat(PRIVATE).st_mode) == 0o700, 'Private directory must have mode 700')
    a_raw, a_info = read_regular(ACCOUNTS)
    s_raw, s_info = read_regular(SESSIONS)
    ca_raw, _ = read_regular(CA, private=False)
    accounts, sessions = parse(a_raw), parse(s_raw)
    need(isinstance(accounts, dict) and isinstance(sessions, dict), 'Canonical fixtures must be JSON objects')
    reason = None
    try:
        validate_fixtures(accounts, sessions)
    except Refused as error:
        reason = str(error)
    ca_matches = sha(ca_raw) == CA_SHA
    report = {'mode': 'probe', 'canonicalAccounts': {**file_metadata(a_info), **fixture_metadata(accounts)},
              'canonicalSessions': {**file_metadata(s_info), **fixture_metadata(sessions)},
              'caMatches': ca_matches, 'canExecute': reason is None and ca_matches,
              'scopeFailure': reason, 'readOnly': True, 'secretsPrinted': False}
    return report, accounts, sessions, a_raw, s_raw, a_info, s_info


def synchronized_data(accounts, sessions, payload):
    validate_fixtures(accounts, sessions)
    validate_payload(payload)
    new_accounts, new_sessions = copy.deepcopy(accounts), copy.deepcopy(sessions)
    new_accounts['password'] = payload['password']
    for user in new_accounts['users']:
        if 'password' in user:
            user['password'] = payload['password']
    replacements = {user['userID']: user for user in payload['users']}
    for user in new_sessions['users']:
        replacement = replacements[user['userID']]
        for key in ('imToken', 'chatToken', 'platformID'):
            user[key] = replacement[key]
    return new_accounts, new_sessions


def fsync_directory(path):
    fd = os.open(path, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW)
    try:
        os.fsync(fd)
    finally:
        os.close(fd)


def atomic_write(path, raw, uid, gid):
    checked_path(path, existing=os.path.lexists(path))
    temporary = path.with_name('.' + path.name + '.incident109-' + secrets.token_hex(12) + '.tmp')
    checked_path(temporary, existing=False)
    fd = os.open(temporary, os.O_CREAT | os.O_EXCL | os.O_WRONLY | os.O_NOFOLLOW, 0o600)
    try:
        os.fchmod(fd, 0o600)
        os.fchown(fd, uid, gid)
        with os.fdopen(fd, 'wb', closefd=False) as destination:
            destination.write(raw)
            destination.flush()
            os.fsync(fd)
        # Do not follow/replace an unexpected symlink introduced while writing.
        checked_path(path, existing=os.path.lexists(path))
        os.replace(temporary, path)
        fsync_directory(path.parent)
    finally:
        os.close(fd)
        if os.path.lexists(temporary):
            os.unlink(temporary)


def immutable_backup(path, raw):
    checked_path(path, existing=os.path.lexists(path))
    fd = os.open(path, os.O_CREAT | os.O_EXCL | os.O_WRONLY | os.O_NOFOLLOW, 0o600)
    try:
        os.fchmod(fd, 0o600)
        with os.fdopen(fd, 'wb', closefd=False) as destination:
            destination.write(raw)
            destination.flush()
            os.fsync(fd)
    finally:
        os.close(fd)


def execute():
    report, accounts, sessions, a_raw, s_raw, a_info, s_info = probe()
    need(report['canExecute'], 'Canonical fixtures or development CA are outside approved scope')
    checked_path(PAYLOAD.parent)
    stage_info = os.lstat(PAYLOAD.parent)
    need(stage_info.st_uid == 0 and stat.S_IMODE(stage_info.st_mode) == 0o700, 'Replacement stage must be root-owned mode 700')
    payload_raw, payload_info = read_regular(PAYLOAD)
    need(payload_info.st_uid == 0, 'Replacement payload must be root-owned')
    payload = parse(payload_raw)
    new_accounts, new_sessions = synchronized_data(accounts, sessions, payload)
    if new_accounts == accounts and new_sessions == sessions:
        return {'mode': 'execute', 'status': 'already-synchronized', 'users': 110, 'adminPreserved': True,
                'originalOwnershipPreserved': True, 'newSecretMatches': True, 'secretsPrinted': False}
    # One root-owned lock prevents two local invocations racing the canonical pair.
    lock = PRIVATE / 'incident109-fixture-sync.lock'
    checked_path(lock, existing=False)
    lock_fd = os.open(lock, os.O_CREAT | os.O_EXCL | os.O_WRONLY | os.O_NOFOLLOW, 0o600)
    os.close(lock_fd)
    try:
        # Re-read after admission and refuse modifications since the probe.
        need(read_regular(ACCOUNTS)[0] == a_raw and read_regular(SESSIONS)[0] == s_raw,
             'Canonical fixture changed during admission; retry same payload')
        if not os.path.lexists(BACKUP):
            BACKUP.mkdir(mode=0o700)
            fsync_directory(PRIVATE)
        checked_path(BACKUP)
        backup_info = os.lstat(BACKUP)
        need(backup_info.st_uid == 0 and stat.S_IMODE(backup_info.st_mode) == 0o700, 'Backup directory must be root-owned mode 700')
        backup_a, backup_s = BACKUP / 'test-accounts.json', BACKUP / 'test-sessions.json'
        for path, raw in ((backup_a, a_raw), (backup_s, s_raw)):
            if not os.path.lexists(path):
                immutable_backup(path, raw)
            _, info = read_regular(path)
            need(info.st_uid == 0, 'Backup must remain root-owned')
        original_a, _ = read_regular(backup_a)
        original_s, _ = read_regular(backup_s)
        validate_fixtures(parse(original_a), parse(original_s))
        fsync_directory(BACKUP)
        # Both immutable backups exist and validate before the first replacement.
        atomic_write(ACCOUNTS, encode(new_accounts), a_info.st_uid, a_info.st_gid)
        atomic_write(SESSIONS, encode(new_sessions), s_info.st_uid, s_info.st_gid)
        final_a_raw, final_a_info = read_regular(ACCOUNTS)
        final_s_raw, final_s_info = read_regular(SESSIONS)
        final_a, final_s = parse(final_a_raw), parse(final_s_raw)
        need(final_a == new_accounts and final_s == new_sessions, 'Canonical replacement verification failed')
        need((final_a_info.st_uid, final_a_info.st_gid) == (a_info.st_uid, a_info.st_gid) and
             (final_s_info.st_uid, final_s_info.st_gid) == (s_info.st_uid, s_info.st_gid),
             'Canonical original uid/gid verification failed')
        need(final_a.get('admin') == accounts.get('admin') and final_a.get('adminSession') == accounts.get('adminSession'),
             'Remote administrator fixture unexpectedly changed')
        return {'mode': 'execute', 'status': 'synchronized', 'users': 110, 'adminPreserved': True,
                'originalOwnershipPreserved': True, 'newSecretMatches': True, 'secretsPrinted': False,
                'backupAccountsSHA256': sha(original_a), 'backupSessionsSHA256': sha(original_s)}
    finally:
        os.unlink(lock)
        fsync_directory(PRIVATE)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--execute-approved-canonical-synthetic-sync', action='store_true')
    args = parser.parse_args()
    try:
        if args.execute_approved_canonical_synthetic_sync:
            report = execute()
        else:
            report, *_ = probe()
        print(json.dumps(report), flush=True)
        return 0 if report.get('mode') == 'execute' or report['canExecute'] else 2
    except Refused as error:
        print(json.dumps({'status': 'refused', 'reason': str(error), 'secretsPrinted': False}), flush=True)
        return 1
    except Exception:
        print(json.dumps({'status': 'refused', 'reason': 'Local fixture validation/I/O failure; no secret details printed', 'secretsPrinted': False}), flush=True)
        return 1


if __name__ == '__main__':
    sys.exit(main())
