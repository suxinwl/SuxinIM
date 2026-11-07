"""Separate operator resource checks for the NEW paused security I/J fixtures.

Default is an offline plan: no private file, credential, certificate or network
is read. Execute only while production_security_acceptance is explicitly paused
at --operator-resource-pause. This tool cannot log in, change policy, logout,
delete objects, run SSH, or mutate a business employee.
"""
import argparse
import base64
import hashlib
import json
import os
import pathlib
import re
import secrets
import ssl
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

ROOT = pathlib.Path(__file__).resolve().parents[1]
HOME = ROOT / '.private/production/acceptance/security-20261104'
OUTPUT = HOME / 'resource-acceptance'
FIXTURE = HOME / 'test-accounts.json'
STATE = HOME / 'state.json'
NAMESPACE = 'security-20261104'
INSTANCE = '5ac7c644d9cd4923e2b701f66b52485d'
ACCOUNTS = ('qa_261104_sec_i', 'qa_261104_sec_j')
NAMES = ('Security acceptance I', 'Security acceptance J')
ORIGINS = {'lan': 'https://192.168.10.11:9885', 'internet': 'https://im.szputy.com:9885'}
SLOTS = ('resource-preview-web', 'resource-save-pc')
PROOF = 'X-Puty-Resource-Proof'
CA = ROOT / 'deploy/certificates/Puty-Enterprise-Root-CA.crt'
MARKER = b'PUTY production identity acceptance fixture'
API_PATHS = frozenset(('/enterprise/v1/session/state', '/enterprise/v1/attachment/authorize',
                       '/api/object/initiate_form_data', '/api/object/complete_form_data'))


class Rejected(RuntimeError):
    """Only fixed caller-owned codes may leave this module."""


def require(value, code):
    if not value:
        raise Rejected(code)


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def canonical(value):
    return json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(',', ':')).encode()


def regular(path):
    require(path.is_file() and not path.is_symlink() and path.resolve() == path and path.stat().st_nlink == 1,
            'private_regular_file_required')


def load(path):
    return snapshot(path)[0]


def snapshot(path):
    """The receipt hashes the SAME bytes that produced the parsed snapshot."""
    regular(path)
    before = path.stat()
    require(before.st_size <= 4 << 20, 'bounded_private_input_required')
    identity = lambda item: (item.st_dev, item.st_ino, item.st_size, item.st_mtime_ns)
    with path.open('rb') as handle:
        require(identity(os.fstat(handle.fileno())) == identity(before), 'private_input_replaced_during_read')
        raw = handle.read((4 << 20) + 1)
        require(len(raw) <= 4 << 20 and identity(os.fstat(handle.fileno())) == identity(before), 'private_input_changed_during_read')
    regular(path)
    require(identity(path.stat()) == identity(before), 'private_input_replaced_during_read')
    return json.loads(raw.decode('utf-8-sig')), sha(raw)


def durable(path, value):
    """Restrict ACL before writing; fsync data and POSIX rename directory."""
    from production_backup import protect
    require(path.is_absolute() and path.resolve().is_relative_to(OUTPUT.resolve()), 'isolated_output_required')
    for directory in (HOME, OUTPUT):
        require(directory.exists() and directory.is_dir() and directory.resolve() == directory and not directory.is_symlink(),
                'isolated_private_directory_required')
        protect(directory)
    if path.exists():
        regular(path)
    temporary = path.with_name(path.name + '.next-' + secrets.token_hex(8))
    try:
        descriptor = os.open(temporary, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        with os.fdopen(descriptor, 'wb') as handle:
            protect(temporary)
            handle.write(canonical(value) + b'\n')
            handle.flush()
            os.fsync(handle.fileno())
        temporary.replace(path)
        if os.name == 'posix':
            descriptor = os.open(path.parent, os.O_RDONLY | getattr(os, 'O_DIRECTORY', 0))
            try:
                os.fsync(descriptor)
            finally:
                os.close(descriptor)
        protect(path)
    finally:
        temporary.unlink(missing_ok=True)


def validate_inputs(fixture, state, run_id, now):
    require(re.fullmatch('[a-f0-9]{32}', run_id or '') is not None, 'explicit_run_nonce_required')
    fields = ('uid', 'employeeID', 'userID', 'username', 'name', 'mobile', 'wecom_userid', 'remark')
    for value in (fixture, state):
        require(value.get('namespace') == NAMESPACE and value.get('instanceID') == INSTANCE
                and value.get('departmentID') == 53 and value.get('runID') == run_id
                and not value.get('creationFailed'), 'fixture_namespace_nonce_mismatch')
        require(type(value.get('createdAt')) is int and 0 <= now - value['createdAt'] <= 86400,
                'fresh_fixture_required')
        accounts = value.get('accounts')
        require(isinstance(accounts, list) and len(accounts) == 2, 'exact_two_new_fixtures_required')
        for i, item in enumerate(accounts):
            require(type(item.get('uid')) is int and item['uid'] > 1 and item.get('username') == ACCOUNTS[i]
                    and item.get('name') == NAMES[i] and item.get('mobile') == '000000000' + ('09' if i == 0 else '10')
                    and item.get('wecom_userid') == '' and item.get('remark') == NAMESPACE + ' fixture ' + run_id + ' ' + str(i),
                    'exact_new_account_binding_required')
            employee = item.get('employeeID')
            require(isinstance(employee, str) and re.fullmatch('[A-Za-z0-9_-]{1,64}', employee)
                    and item.get('userID') == 'emp_' + sha(employee.encode())[:32], 'employee_IM_identity_mismatch')
        excluded, old_employees = value.get('excludedUIDs'), value.get('excludedEmployeeIDs')
        require(isinstance(excluded, list) and len(excluded) == len(set(excluded)) == 6
                and all(type(uid) is int and uid > 1 for uid in excluded)
                and isinstance(old_employees, list) and len(old_employees) == len(set(old_employees)) == 6
                and all(isinstance(x, str) and x for x in old_employees), 'six_old_identity_exclusions_required')
        require(len({a['uid'] for a in accounts}) == len({a['employeeID'] for a in accounts}) == 2
                and not set(excluded).intersection(a['uid'] for a in accounts)
                and not set(old_employees).intersection(a['employeeID'] for a in accounts), 'old_identity_reuse_refused')
    require(all(fixture['accounts'][i].get(k) == state['accounts'][i].get(k) for i in range(2) for k in fields)
            and fixture['excludedUIDs'] == state['excludedUIDs'] and fixture['excludedEmployeeIDs'] == state['excludedEmployeeIDs'],
            'immutable_fixture_state_binding_mismatch')
    sessions = state.get('sessions', {})
    picked = []
    for slot, platform, client in zip(SLOTS, (5, 3), ('web_desktop', 'pc')):
        value = sessions.get(slot)
        require(isinstance(value, dict) and value.get('userID') == state['accounts'][0]['userID']
                and value.get('_platform') == platform and value.get('_client') == client
                and value.get('policyClient') == client
                and all(isinstance(value.get(k), str) and 0 < len(value[k]) <= 32768 for k in ('imToken', 'chatToken', 'sessionToken')),
                'paused_resource_session_binding_required')
        picked.append(value)
    require(all(picked[0][key] != picked[1][key] for key in ('imToken', 'chatToken', 'sessionToken')),
            'distinct_paused_sessions_required')
    return picked


def new_expected(fixture, now, name_nonce=None, body_nonce=None):
    name_nonce, body_nonce = name_nonce or secrets.token_hex(12), body_nonce or secrets.token_hex(32)
    require(re.fullmatch('[0-9a-f]{24}', name_nonce) and re.fullmatch('[0-9a-f]{64}', body_nonce), 'synthetic_nonce_shape_required')
    body = (MARKER + b'\n' + body_nonce.encode() + b'\n') * 4
    require(len(body) == 436, 'fixed_identity_body_required')
    return {'format': 'puty-security-resource-expected-v1', 'namespace': NAMESPACE, 'instanceID': INSTANCE,
            'runID': fixture['runID'], 'createdAt': now, 'name': fixture['accounts'][0]['userID'] + '/production-acceptance-' + name_nonce + '.txt',
            'contentType': 'text/plain', 'group': 'acceptance', 'size': 436, 'bodyBase64': base64.b64encode(body).decode(),
            'bodySHA256': sha(body), 'bodyMD5': hashlib.md5(body).hexdigest(), 'uploadInitiated': False, 'completed': False}, body


def decode_upload(value, expected):
    require(isinstance(value, dict) and isinstance(value.get('id'), str) and len(value['id']) < 4096, 'form_upload_id_required')
    encoded = value['id']
    try:
        raw = base64.b64decode(encoded + '=' * (-len(encoded) % 4), validate=True)
        meta = json.loads(raw)
    except (ValueError, UnicodeDecodeError):
        raise Rejected('form_upload_metadata_invalid')
    require(isinstance(meta, dict) and set(meta) == {'name', 'size', 'contentType', 'group', 'key'}
            and all(meta.get(k) == expected[k] for k in ('name', 'size', 'contentType', 'group')),
            'form_upload_metadata_scope_mismatch')
    owner = expected['name'].split('/')[0]
    require(isinstance(meta.get('key'), str) and re.fullmatch(r'openim/direct/[0-9]{8}/' + re.escape(owner) + r'/[0-9a-f]{32}\.txt', meta['key']),
            'exact_form_storage_key_required')
    return meta['key']


def grant_url(raw, origin, proof=None):
    require(isinstance(raw, str) and len(raw) <= 16384, 'bounded_grant_url_required')
    parsed, base = urllib.parse.urlsplit(raw), urllib.parse.urlsplit(origin)
    require(parsed.scheme == 'https' and parsed.netloc == base.netloc and parsed.username is None
            and parsed.password is None and not parsed.fragment and re.fullmatch(r'/attachments/[0-9a-f]{64}/[^\r\n]+', parsed.path),
            'current_entry_attachment_grant_required')
    keys = [k.lower() for k, _ in urllib.parse.parse_qsl(parsed.query, keep_blank_values=True)]
    require(not any('proof' in k or k in ('token', 'chattoken', 'sessiontoken') for k in keys)
            and (proof is None or proof not in urllib.parse.unquote(raw)), 'proof_must_not_enter_url')
    return raw


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, *args, **kwargs):
        raise Rejected('redirect_refused')


class Transport:
    def __init__(self, zone):
        self.origin, self.zone = ORIGINS[zone], zone
        context = ssl.create_default_context()
        context.load_verify_locations(cafile=str(CA))
        # No CookieProcessor: the missing-proof case cannot inherit an ambient
        # cookie set by the preceding authorize response.
        self.opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), urllib.request.HTTPSHandler(context=context), NoRedirect())

    def request(self, target, method='GET', session=None, value=None, raw=None, headers=None):
        if target.startswith('/'):
            require(target in API_PATHS, 'fixed_API_route_required')
            target = self.origin + target
        else:
            grant_url(target, self.origin)
        supplied = {'operationID': secrets.token_hex(16), 'Accept-Encoding': 'identity', **(headers or {})}
        require(not set(k.lower() for k in supplied).intersection(('cookie', 'authorization')), 'ambient_credentials_refused')
        if session:
            supplied.update(token=session['imToken'], chatToken=session['chatToken'], platformID=str(session['_platform']))
        if value is not None:
            require(raw is None, 'one_request_body_required')
            raw = canonical(value)
            supplied['Content-Type'] = 'application/json'
        request = urllib.request.Request(target, data=raw, headers=supplied, method=method)
        try:
            response = self.opener.open(request, timeout=20)
        except urllib.error.HTTPError as error:
            response = error
        with response:
            body = response.read(65537)
            require(len(body) <= 65536, 'bounded_response_required')
            try:
                payload = json.loads(body)
            except (ValueError, UnicodeDecodeError):
                payload = None
            return response.status, payload, body, {k.lower(): v for k, v in response.headers.items()}


def data(result):
    status, payload = result[:2]
    require(status == 200 and isinstance(payload, dict) and payload.get('errCode', payload.get('code')) == 0
            and isinstance(payload.get('data'), dict), 'API_success_required')
    return payload['data']


def rejection(result):
    return result[0] in (401, 403)


def multipart(value, body):
    boundary = 'PutyResource' + secrets.token_hex(16)
    form = value.get('formData')
    require(isinstance(form, dict) and 1 <= len(form) <= 32 and isinstance(value.get('file'), str)
            and re.fullmatch('[A-Za-z0-9_-]{1,64}', value['file']), 'bounded_signed_form_required')
    chunks = []
    for key, content in form.items():
        require(isinstance(key, str) and re.fullmatch('[A-Za-z0-9_-]{1,128}', key)
                and isinstance(content, str) and len(content) <= 16384 and '\r' not in content and '\n' not in content,
                'signed_form_field_invalid')
        chunks.append(('--' + boundary + '\r\nContent-Disposition: form-data; name="' + key + '"\r\n\r\n' + content + '\r\n').encode())
    chunks.extend([('--' + boundary + '\r\nContent-Disposition: form-data; name="' + value['file'] + '"; filename="acceptance.txt"\r\nContent-Type: text/plain\r\n\r\n').encode(), body, ('\r\n--' + boundary + '--\r\n').encode()])
    require(not value.get('header'), 'unexpected_form_upload_headers')
    return b''.join(chunks), {'Content-Type': 'multipart/form-data; boundary=' + boundary}


class Acceptance:
    def __init__(self, transport, fixture, sessions, expected, body, persist, record, now=lambda: int(time.time())):
        self.t, self.fixture, self.web, self.pc = transport, fixture, *sessions
        self.expected, self.body, self.persist, self.record, self.now = expected, body, persist, record, now

    def check(self, label, ok, result=None):
        self.record(label, bool(ok), None if result is None else result[0])
        require(ok, label)

    def state(self, session, client, save):
        result = self.t.request('/enterprise/v1/session/state', session=session)
        value = data(result)
        policy = value.get('effectivePolicy', {})
        self.check(client + '_current_policy', value.get('userID') == session['userID'] and value.get('logicalClient') == client
                   and value.get('platform') == session['_platform'] and value.get('loginAllowed') is True
                   and value.get('saveAllowed') is save and type(value.get('epoch')) is int and value['epoch'] >= 1
                   and isinstance(value.get('sessionID'), str) and bool(value['sessionID'])
                   and policy.get('zone') == self.t.zone and policy.get('logicalClient') == client
                   and policy.get('login') is True and policy.get('preview') is True and policy.get('save') is save
                   and policy.get('securityVersion') == value['epoch'], result)
        return value

    def upload(self):
        # persist() is intentionally before the FIRST initiate request.
        self.persist(self.expected)
        result = self.t.request('/api/object/initiate_form_data', 'POST', self.pc,
                                {k: self.expected[k] for k in ('name', 'size', 'contentType', 'group')})
        value = data(result)
        self.expected.update(uploadID=value['id'], objectKey=decode_upload(value, self.expected), uploadInitiated=True)
        target = grant_url(value.get('url'), self.t.origin)
        self.persist(self.expected)
        require(isinstance(value.get('formData'), dict) and value['formData'].get('key') == self.expected['objectKey']
                and value['formData'].get('Content-Type') == 'text/plain', 'signed_form_exact_key_MIME_required')
        raw, headers = multipart(value, self.body)
        result = self.t.request(target, 'POST', raw=raw, headers=headers)
        success = value.get('successCodes')
        require(isinstance(success, list) and bool(success) and all(type(x) is int and x in (200, 201, 204) for x in success), 'signed_form_success_codes_required')
        self.check('signed_form_436_bytes_uploaded', result[0] in success, result)
        result = self.t.request('/api/object/complete_form_data', 'POST', self.pc, {'id': value['id']})
        complete = data(result)
        stable = complete.get('url')
        require(stable == self.t.origin + '/api/object/' + self.expected['name'], 'exact_stable_object_reference_required')
        self.expected.update(completed=True, stableURL=stable, completedAt=self.now())
        self.persist(self.expected)
        return stable

    def authorize(self, session, source, purpose):
        result = self.t.request('/enterprise/v1/attachment/authorize', 'POST', session, {'url': source, 'purpose': purpose})
        grant = data(result)
        proof = grant.get('resourceProof')
        require(isinstance(proof, str) and re.fullmatch('[0-9a-f]{64}', proof) and grant.get('purpose') == purpose
                and isinstance(grant.get('objectID'), str) and re.fullmatch('[0-9a-f]{64}', grant['objectID']), 'separate_resource_proof_required')
        grant_url(grant.get('url'), self.t.origin, proof)
        require(urllib.parse.urlsplit(grant['url']).path.split('/', 3)[3] == 'openim38/' + self.expected['objectKey'],
                'grant_must_identify_exact_uploaded_key')
        self.check(purpose + '_authorization_no_store', 'no-store' in result[3].get('cache-control', '').lower(), result)
        return grant

    def read(self, grant, label, method='GET', headers=None):
        result = self.t.request(grant['url'], method, headers={PROOF: grant['resourceProof'], **(headers or {})})
        expected = b'' if method == 'HEAD' else (self.body[2:10] if headers else self.body)
        self.check(label, result[0] == (206 if headers else 200) and result[2] == expected, result)
        self.check(label + '_no_store', 'no-store' in result[3].get('cache-control', '').lower(), result)
        if method == 'HEAD':
            self.check(label + '_size', result[3].get('content-length') == '436', result)
        if headers:
            self.check(label + '_exact_range', result[3].get('content-range') == 'bytes 2-9/436', result)

    def run(self):
        web, pc = self.state(self.web, 'web_desktop', False), self.state(self.pc, 'pc', True)
        self.check('separate_live_SIDs_same_exact_QA', web['sessionID'] != pc['sessionID'] and web['epoch'] == pc['epoch'])
        source = self.upload()
        preview = self.authorize(self.web, source, 'preview')
        for method in ('GET', 'HEAD'):
            self.read(preview, 'web_preview_' + method, method)
        self.read(preview, 'web_preview_Range', headers={'Range': 'bytes=2-9'})
        denied = self.t.request('/enterprise/v1/attachment/authorize', 'POST', self.web, {'url': source, 'purpose': 'save'})
        self.check('web_save_authorization_denied', rejection(denied), denied)
        saved = self.authorize(self.pc, source, 'save')
        self.check('preview_and_save_same_stable_object', preview['objectID'] == saved['objectID'])
        self.read(saved, 'pc_save_GET')
        self.read(saved, 'pc_save_Range', headers={'Range': 'bytes=2-9'})
        for label, grant, headers, session in (
            ('copied_preview_URL_without_proof_denied', preview, {}, None),
            ('copied_save_URL_without_proof_denied', saved, {}, None),
            ('forged_proof_denied', preview, {PROOF: '0' * 64}, None),
            ('preview_URL_PC_proof_denied', preview, {PROOF: saved['resourceProof']}, None),
            ('save_URL_web_proof_denied', saved, {PROOF: preview['resourceProof']}, None),
            ('web_proof_PC_pair_denied', preview, {PROOF: preview['resourceProof']}, self.pc),
            ('PC_proof_web_pair_denied', saved, {PROOF: saved['resourceProof']}, self.web),
            ('mixed_IM_Chat_pair_denied', preview, {PROOF: preview['resourceProof']}, {**self.web, 'chatToken': self.pc['chatToken']}),
            ('wrong_platform_header_denied', preview, {PROOF: preview['resourceProof']}, {**self.web, '_platform': 3}),
        ):
            result = self.t.request(grant['url'], headers=headers, session=session)
            self.check(label, rejection(result), result)
        self.read(preview, 'web_preview_still_usable_after_save_denial')


def inventory_input(fixture, expected, end):
    require(type(end) is int and 0 < end - (expected['createdAt'] - 2) <= 86400, 'bounded_inventory_window_required')
    record = {key: expected[key] for key in ('name', 'size', 'contentType', 'group', 'bodySHA256', 'bodyMD5', 'bodyBase64', 'completed')}
    record.update({key: expected[key] for key in ('objectKey', 'uploadID', 'completedAt') if key in expected})
    return {'fixtureEmployeeIDs': [a['employeeID'] for a in fixture['accounts']],
            'fixtureIMUserIDs': [a['userID'] for a in fixture['accounts']],
            'createdAfter': expected['createdAt'] - 2, 'createdBefore': end,
            'namespace': NAMESPACE, 'runID': fixture['runID'], 'instanceID': INSTANCE,
            'expectedObjectCount': 1, 'expectedObjects': [record],
            'requiresExactOrphanReview': expected.get('uploadInitiated') is True and not expected.get('completed'),
            'rootMustVerifyExactNameKeySHAAndCountBeforeApply': True,
            'serverAction': 'attachment-inventory', 'cleanupAction': 'attachment-cleanup'}


def report_document(checks, fixture_sha, state_sha, run_id, *, complete=False, inventory_sha=None):
    return {'format': 'puty-security-resource-result-v1', 'namespace': NAMESPACE, 'instanceID': INSTANCE,
            'runID': run_id, 'fixtureSourceSHA256': fixture_sha, 'pausedStateSHA256': state_sha,
            'checks': checks, 'passed': complete and bool(checks) and all(x['passed'] for x in checks),
            'resourceChecksComplete': complete, 'inventoryInputSHA256': inventory_sha,
            'policyWrites': False, 'sessionsCreated': 0, 'serverCleanupExecuted': False}


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    action = parser.add_mutually_exclusive_group()
    action.add_argument('--execute', action='store_true')
    action.add_argument('--dry-run', action='store_true')
    parser.add_argument('--zone', choices=tuple(ORIGINS), default='lan')
    parser.add_argument('--expected-run-id')
    parser.add_argument('--fixture-sha256')
    args = parser.parse_args(argv)
    if not args.execute:
        print(json.dumps({'offlinePlan': True, 'privateInputsRead': 0, 'productionRequests': 0,
                          'namespace': NAMESPACE, 'requiredPauseSlots': list(SLOTS), 'syntheticObjectBytes': 436,
                          'objectCount': 1, 'policyWrites': False, 'logins': False, 'serverCleanupExecuted': False}))
        return 0
    require(re.fullmatch('[0-9a-f]{64}', args.fixture_sha256 or ''), 'explicit_fixture_SHA_required')
    fixture, fixture_sha = snapshot(FIXTURE)
    state, state_sha = snapshot(STATE)
    require(fixture_sha == args.fixture_sha256, 'fixture_SHA_mismatch')
    sessions = validate_inputs(fixture, state, args.expected_run_id, int(time.time()))
    require(not OUTPUT.exists(), 'resource_attempt_exists_never_overwrite_or_reupload')
    OUTPUT.mkdir(mode=0o700)
    expected, body = new_expected(fixture, int(time.time()))
    expected.update(fixtureSourceSHA256=fixture_sha, pausedStateSHA256=state_sha)
    checks = []
    def persist(value): durable(OUTPUT / 'expected-object.private.json', value)
    def record(label, passed, status):
        item = {'check': label, 'passed': passed}
        if status is not None: item['status'] = status
        checks.append(item)
        durable(OUTPUT / 'report.json', report_document(checks, fixture_sha, state_sha, args.expected_run_id))
        print(json.dumps(item), flush=True)
    failure = None
    try:
        persist(expected)
        require(sha(STATE.read_bytes()) == state_sha, 'paused_state_changed_before_request')
        Acceptance(Transport(args.zone), fixture, sessions, expected, body, persist, record).run()
        require(sha(STATE.read_bytes()) == state_sha, 'parent_pause_state_changed')
    except BaseException as error:
        failure = error
        record('resource_attempt_finished', False, None)
    finally:
        end = max(int(time.time()), expected['createdAt'] + 1)
        durable(OUTPUT / 'attachment-inventory-input.private.json', inventory_input(fixture, expected, end))
    if failure:
        raise Rejected(str(failure) if isinstance(failure, Rejected) else 'resource_request_or_local_IO_failed')
    durable(OUTPUT / 'report.json', report_document(checks, fixture_sha, state_sha, args.expected_run_id,
                                                   complete=True, inventory_sha=sha((OUTPUT / 'attachment-inventory-input.private.json').read_bytes())))
    print(json.dumps({'resourceChecksPassed': True, 'checks': len(checks), 'objectCount': 1,
                      'inventoryInput': str(OUTPUT / 'attachment-inventory-input.private.json'),
                      'serverCleanupExecuted': False}), flush=True)
    return 0


if __name__ == '__main__':
    try:
        exit_code = main()
    except BaseException as error:
        code = str(error) if isinstance(error, Rejected) else 'resource_acceptance_failed_private_details_suppressed'
        print(json.dumps({'passed': False, 'category': code}), file=sys.stderr)
        exit_code = 1
    sys.exit(exit_code)
