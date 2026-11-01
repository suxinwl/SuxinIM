"""Incident-specific synthetic credential rotation. Default prepares locally only.

No arbitrary host/account/path options. Execute/resume only after source review.
The private journal contains backups and the one replacement secret; never publish it.
"""
from __future__ import annotations

import argparse
import base64
import contextlib
import copy
import hashlib
import json
import os
from pathlib import Path
import re
import secrets
import ssl
import subprocess
import sys
import urllib.error
import urllib.request

ROOT = Path(__file__).resolve().parents[1]
EXPECTED_ROOT = Path('E:/PUTY_AI/OpenIM3.4-3.8/OpenIM3.8.3patch.18_1.8.4patch.5')
BASE = 'https://192.168.10.11:29880'
OLD_SHA = 'ad47c3f4650808796f1cd29c78304c2827e73d00c24b5081abac8f57ffb2e968'
CA_SHA = '90817f305832a3c2398af0384e76004096647aeeac7095a250c6a5a4845655f0'
IDS = tuple(f'upgrade38_u{i:03}' for i in range(1, 111))
ACCOUNTS = '.private/test-accounts.json'
SESSIONS = '.private/test-sessions.json'
JOURNAL = '.private/synthetic-incident-109-rotation.json'
CA = 'artifacts/dev-cert/OpenIM38-Development-CA.crt'
PUBLIC = 'tests/flutter/synthetic-incident-109-rotation-status.json'
LOCK = '.private/synthetic-incident-109-rotation.lock'


class GuardError(Exception):
    """Only constant safe messages may be passed here."""


def require(ok, message):
    if not ok:
        raise GuardError(message)


def digest(value):
    return hashlib.sha256(value).hexdigest()


def md5(value):
    return hashlib.md5(value.encode('utf-8')).hexdigest()


def validate_root(root):
    require(root.resolve() == EXPECTED_ROOT.resolve(), 'Project root is outside the fixed incident scope')


def fixed_path(relative):
    require(relative in (ACCOUNTS, SESSIONS, JOURNAL, CA, PUBLIC, LOCK), 'Unapproved incident file')
    path = ROOT / relative
    for item in (path, *path.parents):
        if item == ROOT.parent:
            break
        if item.exists():
            require(not item.is_symlink() and not (item.stat().st_file_attributes & 0x400
                    if hasattr(item.stat(), 'st_file_attributes') else False), 'Reparse/symlink file refused')
    require(path.resolve().is_relative_to(ROOT.resolve()), 'Incident path escapes project')
    return path


def encode(value):
    return (json.dumps(value, ensure_ascii=False, indent=2) + '\n').encode('utf-8')


def parse(raw):
    return json.loads(raw.decode('utf-8-sig'))


def restricted_empty(path):
    """Create zero bytes before tightening ACL; secret bytes are written afterward."""
    fd = os.open(path, os.O_CREAT | os.O_EXCL | os.O_WRONLY, 0o600)
    os.close(fd)
    if os.name == 'nt':
        identity = subprocess.run(['whoami', '/user', '/fo', 'csv', '/nh'], capture_output=True, timeout=15)
        sid = re.search(rb'S-1-5-[0-9-]+', identity.stdout)
        require(identity.returncode == 0 and sid is not None, 'Cannot establish current Windows SID')
        result = subprocess.run(['icacls', str(path), '/inheritance:r', '/grant:r',
                                 '*' + sid.group().decode() + ':(F)', '*S-1-5-18:(F)'],
                                capture_output=True, timeout=15)
        require(result.returncode == 0, 'Cannot restrict private file ACL; no secret written')
    else:
        os.chmod(path, 0o600)


def private_write(path, raw, *, exclusive=False):
    if exclusive:
        require(not path.exists(), 'Prepared journal already exists; use resume, never overwrite')
        restricted_empty(path)
        with path.open('wb') as out:
            out.write(raw)
            out.flush()
            os.fsync(out.fileno())
        return
    temporary = path.with_name(path.name + '.' + secrets.token_hex(8) + '.tmp')
    try:
        restricted_empty(temporary)
        with temporary.open('wb') as out:
            out.write(raw)
            out.flush()
            os.fsync(out.fileno())
        os.replace(temporary, path)
    finally:
        if temporary.exists():
            temporary.unlink()


def validate_scope(accounts, sessions, *, original=True):
    require(accounts.get('baseURL') == BASE and sessions.get('baseURL') == BASE, 'Only fixed isolated HTTPS baseURL is allowed')
    for fixture in (accounts, sessions):
        users = fixture.get('users')
        require(isinstance(users, list) and len(users) == 110, 'Exactly 110 synthetic users required')
        require({u.get('userID') for u in users} == set(IDS), 'Synthetic user set must be exactly 001-110')
    password = accounts.get('password')
    require(isinstance(password, str) and bool(password), 'Shared fixture password is missing')
    if original:
        require(digest(password.encode()) == OLD_SHA, 'Shared password does not match this incident')
    admin = accounts.get('admin', {})
    require(isinstance(admin.get('account'), str) and bool(admin['account']) and
            isinstance(admin.get('password'), str) and bool(admin['password']), 'Independent administrator fixture required')
    require(admin['password'] != password and digest(admin['password'].encode()) != OLD_SHA,
            'Administrator must not share incident password')
    a = {u['userID']: u for u in accounts['users']}
    for user in sessions['users']:
        require(type(user.get('platformID')) is int and 1 <= user['platformID'] <= 10, 'Seed platform must be 1-10')
        require(all(isinstance(user.get(k), str) and user[k] for k in ('imToken', 'chatToken')), 'Seed dual tokens missing')
        for key in ('account', 'email', 'nickname'):
            require(user.get(key) == a[user['userID']].get(key) and isinstance(user.get(key), str), 'Seed identity metadata mismatch')
    for user in a.values():
        require('password' not in user or user['password'] == password, 'Per-user password conflicts with shared fixture')
        require(bool(user.get('email')) and bool(user.get('account')), 'Synthetic login identity missing')
        index = user['userID'][-3:]
        require(user['email'] == f'u{index}@openim38.test' and user['account'] == f'im38u{index}',
                'Synthetic account/email must match the original incident fixtures')


@contextlib.contextmanager
def exclusive_run():
    path = fixed_path(LOCK)
    require(not path.exists(), 'Another run or interrupted run lock exists; review it before continuing')
    restricted_empty(path)
    try:
        yield
    finally:
        path.unlink()


def prepare():
    validate_root(ROOT)
    journal_path = fixed_path(JOURNAL)
    require(not journal_path.exists(), 'Prepared journal already exists; use resume, never overwrite')
    raw_accounts, raw_sessions = fixed_path(ACCOUNTS).read_bytes(), fixed_path(SESSIONS).read_bytes()
    accounts, sessions = parse(raw_accounts), parse(raw_sessions)
    validate_scope(accounts, sessions)
    require(digest(fixed_path(CA).read_bytes()) == CA_SHA, 'Development CA does not match fixed incident scope')
    new_password = secrets.token_urlsafe(32) + 'aA7'
    require(new_password != accounts['admin']['password'] and digest(new_password.encode()) != OLD_SHA, 'Replacement password collision')
    plan = {'schema': 1, 'baseURL': BASE, 'oldSecretSHA256': OLD_SHA, 'caFileSHA256': CA_SHA,
            'newPassword': new_password, 'newSecretSHA256': digest(new_password.encode()),
            'backupAccountsBase64': base64.b64encode(raw_accounts).decode(),
            'backupSessionsBase64': base64.b64encode(raw_sessions).decode(),
            'originalAccountsSHA256': digest(raw_accounts), 'originalSessionsSHA256': digest(raw_sessions),
            'users': {uid: {} for uid in IDS}, 'status': 'prepared', 'fixtureCommitted': False}
    private_write(journal_path, encode(plan), exclusive=True)
    return plan


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, *args, **kwargs):
        raise GuardError('HTTP redirect refused')


class Client:
    def __init__(self):
        require(digest(fixed_path(CA).read_bytes()) == CA_SHA, 'Development CA does not match fixed incident scope')
        context = ssl.create_default_context(cafile=str(fixed_path(CA)))
        self.http = urllib.request.build_opener(urllib.request.ProxyHandler({}), NoRedirect(),
                                               urllib.request.HTTPSHandler(context=context))

    def call(self, path, body, headers=None):
        allowed = ('/enterprise/v1/admin/login', '/enterprise/v1/admin/chat/user/password/reset',
                   '/chat/account/login', '/chat/user/find/full', '/api/user/get_users_info')
        require(path in allowed, 'API outside credential incident scope')
        request = urllib.request.Request(BASE + path, encode(body),
            {'Content-Type': 'application/json', 'operationID': 'incident109-' + secrets.token_hex(12), **(headers or {})})
        try:
            with self.http.open(request, timeout=45) as response:
                value = json.load(response)
        except urllib.error.HTTPError as error:
            # A JSON application code is required even for non-2xx denials.
            try:
                value = json.loads(error.read(1024 * 1024))
            except Exception:
                raise GuardError('HTTP request failed without an application code') from None
        except GuardError:
            raise
        except Exception:
            raise GuardError('Network/TLS/response failure; resume same prepared journal') from None
        require(isinstance(value, dict) and type(value.get('errCode')) is int, 'API response has no numeric application code')
        return value['errCode'], value.get('data') or {}


def load_plan():
    validate_root(ROOT)
    plan = parse(fixed_path(JOURNAL).read_bytes())
    require(plan.get('schema') == 1 and plan.get('baseURL') == BASE and plan.get('oldSecretSHA256') == OLD_SHA
            and plan.get('caFileSHA256') == CA_SHA and set(plan.get('users', {})) == set(IDS), 'Prepared journal scope mismatch')
    accounts_raw = base64.b64decode(plan['backupAccountsBase64'], validate=True)
    sessions_raw = base64.b64decode(plan['backupSessionsBase64'], validate=True)
    require(digest(accounts_raw) == plan['originalAccountsSHA256'] and digest(sessions_raw) == plan['originalSessionsSHA256'], 'Private backup hash mismatch')
    accounts, sessions = parse(accounts_raw), parse(sessions_raw)
    validate_scope(accounts, sessions)
    require(isinstance(plan.get('newPassword'), str) and len(plan['newPassword']) >= 40 and
            digest(plan['newPassword'].encode()) == plan.get('newSecretSHA256') and
            plan['newSecretSHA256'] != OLD_SHA and plan['newPassword'] != accounts['admin']['password'], 'Replacement secret journal mismatch')
    for path, key, final_key in ((ACCOUNTS, 'originalAccountsSHA256', 'finalAccountsSHA256'),
                                 (SESSIONS, 'originalSessionsSHA256', 'finalSessionsSHA256')):
        require(digest(fixed_path(path).read_bytes()) in {plan[key], plan.get(final_key)}, 'Live fixture changed outside this journal; refusing overwrite')
    return plan, accounts, sessions


def save(plan):
    private_write(fixed_path(JOURNAL), encode(plan))


def seed_codes(client, user):
    headers = {'token': user['chatToken'], 'chatToken': user['chatToken'], 'platformID': str(user['platformID'])}
    chat, _ = client.call('/chat/user/find/full', {'userIDs': [user['userID']]}, headers)
    headers['token'] = user['imToken']
    im, _ = client.call('/api/user/get_users_info', {'userIDs': [user['userID']]}, headers)
    return {'chatCode': chat, 'imCode': im}


def seed_pair_denied(codes):
    # Fixed server codes: TokenKickedError (1506), TokenNotExistError
    # (1507). Either explicitly denies the old IM token; neither proves
    # revocation caused by this reset when that seed was already invalid.
    return isinstance(codes, dict) and codes.get('chatCode') == 20101 and codes.get('imCode') in (1506, 1507)


def execute():
    plan, accounts, sessions = load_plan()
    if plan.get('status') == 'complete':
        return plan
    client = Client()
    code, admin = client.call('/enterprise/v1/admin/login', {'account': accounts['admin']['account'], 'password': md5(accounts['admin']['password'])})
    require(code == 0 and bool(admin.get('adminToken')), 'Administrator authentication failed')
    admin_headers = {'token': admin['adminToken'], 'chatToken': admin['adminToken']}
    by_id = {u['userID']: u for u in accounts['users']}
    seed_by_id = {u['userID']: u for u in sessions['users']}
    plan['status'] = 'executing'
    save(plan)
    for uid in IDS:
        item, user, seed = plan['users'][uid], by_id[uid], seed_by_id[uid]
        if not item.get('resetConfirmed'):
            if 'seedBefore' not in item:
                item['seedBefore'] = seed_codes(client, seed)
            # Journal intent BEFORE the request; uncertain reset is retried with
            # exactly the same new hash, never a regenerated replacement.
            item['resetAttempted'] = True
            save(plan)
            code, _ = client.call('/enterprise/v1/admin/chat/user/password/reset',
                                 {'userID': uid, 'newPassword': md5(plan['newPassword'])}, admin_headers)
            require(code == 0, 'Synthetic password reset failed')
            item['resetConfirmed'] = True
            save(plan)
        if not item.get('oldPasswordRejected'):
            code, _ = client.call('/chat/account/login', {'email': user['email'], 'password': md5(accounts['password']),
                                  'platform': seed['platformID'], 'deviceID': 'incident109-old-check'})
            item['oldPasswordCode'] = code
            save(plan)
            require(code == 20001, 'Old password was not specifically rejected as ErrPassword')
            item['oldPasswordRejected'] = True
            save(plan)
        if 'seedAfter' not in item:
            item['seedAfter'] = seed_codes(client, seed)
            save(plan)
        require(seed_pair_denied(item['seedAfter']),
                'Old seed token pair did not show the required revoked codes')
        if not item.get('newSession'):
            code, data = client.call('/chat/account/login', {'email': user['email'], 'password': md5(plan['newPassword']),
                                      'platform': seed['platformID'], 'deviceID': 'incident109-refreshed-seed'})
            require(code == 0 and data.get('userID') == uid and all(isinstance(data.get(k), str) and data[k]
                    for k in ('imToken', 'chatToken')), 'Replacement login identity or dual tokens failed')
            item['newSession'] = {**user, 'imToken': data['imToken'], 'chatToken': data['chatToken'], 'platformID': seed['platformID']}
            save(plan)
        print(json.dumps({'userID': uid, 'step': 'verified', 'oldPasswordCode': 20001}), flush=True)
    updated_accounts = copy.deepcopy(accounts)
    updated_accounts['password'] = plan['newPassword']
    # Keep the original independent administrator fixture/session unchanged.
    # No administrator password is rotated, and a new admin login is needed
    # only in memory. Stable bytes make a crash between the two replacements
    # resumable without changing the already-journaled target hashes.
    for user in updated_accounts['users']:
        if 'password' in user:
            user['password'] = plan['newPassword']
    updated_sessions = copy.deepcopy(sessions)
    updated_sessions['users'] = [plan['users'][u['userID']]['newSession'] for u in sessions['users']]
    accounts_raw, sessions_raw = encode(updated_accounts), encode(updated_sessions)
    # Persist intended hashes before replacing either file, permitting crash-safe
    # resume between two individually atomic replacements.
    plan['finalAccountsSHA256'], plan['finalSessionsSHA256'] = digest(accounts_raw), digest(sessions_raw)
    save(plan)
    private_write(fixed_path(ACCOUNTS), accounts_raw)
    private_write(fixed_path(SESSIONS), sessions_raw)
    plan['fixtureCommitted'], plan['status'] = True, 'complete'
    save(plan)
    return plan


def public_status(plan):
    values = list(plan['users'].values())
    return {'baseURL': BASE, 'status': plan['status'], 'scope': 'upgrade38_u001-upgrade38_u110',
            'preparedUsers': 110, 'resetConfirmed': sum(bool(u.get('resetConfirmed')) for u in values),
            'oldPasswordRejected20001': sum(bool(u.get('oldPasswordRejected')) for u in values),
            'newDualSessions': sum(bool(u.get('newSession')) for u in values),
            'seedBeforeValidBoth': sum(u.get('seedBefore') == {'chatCode': 0, 'imCode': 0} for u in values),
            'seedBeforeValidChat': sum(u.get('seedBefore', {}).get('chatCode') == 0 for u in values),
            'seedBeforeValidIM': sum(u.get('seedBefore', {}).get('imCode') == 0 for u in values),
            'seedAfterRequiredRevokedCodes': sum(seed_pair_denied(u.get('seedAfter')) for u in values),
            'seedAfterIMTokenNotExist1507': sum(u.get('seedAfter', {}).get('imCode') == 1507 for u in values),
            'fixtureCommitted': bool(plan['fixtureCommitted']), 'newSecretSHA256': plan['newSecretSHA256'],
            'oldSeedCausalityLimit': 'Only seed pairs recorded valid before reset support a before/after revocation claim.',
            'secretsPrinted': False, 'normalTLSValidation': True}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('action', choices=('prepare', 'status', 'execute'))
    parser.add_argument('--execute-approved-synthetic-110-rotation', action='store_true')
    args = parser.parse_args()
    try:
        validate_root(ROOT)
        if args.action == 'execute':
            require(args.execute_approved_synthetic_110_rotation, 'Execution requires the explicit reviewed rotation flag')
            with exclusive_run():
                plan = execute()
        elif args.action == 'prepare':
            require(not args.execute_approved_synthetic_110_rotation, 'Prepare never executes rotation')
            with exclusive_run():
                plan = prepare()
        else:
            plan, _, _ = load_plan()
        report = public_status(plan)
        fixed_path(PUBLIC).write_bytes(encode(report))
        print(json.dumps(report), flush=True)
        return 0
    except GuardError as error:
        print(json.dumps({'status': 'stopped', 'reason': str(error), 'secretsPrinted': False}), flush=True)
        return 1
    except Exception:
        # Never serialize unexpected exception text: it may contain request data.
        print(json.dumps({'status': 'stopped', 'reason': 'Local validation/I/O failed; private journal preserved', 'secretsPrinted': False}), flush=True)
        return 1


if __name__ == '__main__':
    sys.exit(main())
