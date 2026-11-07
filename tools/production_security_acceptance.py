"""Operator-run security acceptance for two NEW synthetic identities only.

Default dry-run is offline. No import reads credentials. Captchas use the normal
endpoint and an operator-entered answer; the script never disables captcha,
changes global lock/network settings, logs raw errors, or logs in real staff.
A failed run always attempts fixture quiescence. SQL employee removal remains a
separate exact-plan/owner/zero-reference transaction, using the reviewed worker.
"""
import argparse
import base64
import email.utils
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
PRIVATE = ROOT / '.private/production/acceptance'
NAMESPACE = 'security-20261104'
HOME = PRIVATE / NAMESPACE
INSTANCE = '5ac7c644d9cd4923e2b701f66b52485d'
FIXTURE = HOME / 'test-accounts.json'
STATE = HOME / 'state.json'
REPORT = HOME / 'report.json'
IM_BASE = '/plugins/biz-suite/api/admin/im/manager/openim/'
ACCOUNTS = ('qa_261104_sec_i', 'qa_261104_sec_j')
NAMES = ('Security acceptance I', 'Security acceptance J')
ROLE_NAMES = ('TEMP QA security-20261104 A', 'TEMP QA security-20261104 B')
DEPT = 53
CLIENTS = {'web_desktop': ('web', 5), 'web_mobile': ('mobile_web', 5), 'pc': ('windows', 3), 'android': ('android', 2), 'ios': ('ios', 1)}
ZONES = ('lan', 'internet')
ORIGINS = {'lan': 'https://192.168.10.11:9885', 'internet': 'https://im.szputy.com:9885'}
HOST_ORIGINS = {'lan': 'https://192.168.10.11:9880', 'internet': 'https://hr.szputy.com:9880'}
OLD_SOURCES = (PRIVATE / 'test-accounts.json', PRIVATE / 'organization-test-accounts.json', PRIVATE / 'attachment-20261103/test-accounts.json')
CA = ROOT / 'deploy/certificates/Puty-Enterprise-Root-CA.crt'
PHASES = ('new-fixtures-and-empty-roles', 'first-login-428-and-single-use-change', 'role-OR-and-personal-override-2x5', 'captcha-excluded-5-wrong-in-600s-lock-1800s', 'admin-unlock', 'old-password-required-self-change', 'admin-reset-restricted-428', 'old-HRM-SSO-IM-epoch-rejected', 'current-zone-five-client-preview-save', 'finally-quiesce-and-exact-SQL-plan')


class Rejected(RuntimeError):
    """Only a fixed safe error code may leave the runner."""


def require(value, code):
    if not value:
        raise Rejected(code)


def canonical(value):
    return json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(',', ':')).encode()


def md5(value):
    return hashlib.md5(value.encode()).hexdigest()


def rows(value):
    if isinstance(value, list):
        return value
    require(isinstance(value, dict), 'unexpected_list_shape')
    return value.get('items', value.get('list', []))


def flatten(value):
    result = []
    for row in rows(value):
        require(isinstance(row, dict), 'unexpected_row_shape')
        result.append(row)
        for name in ('children', '_children', 'child'):
            if isinstance(row.get(name), list):
                result.extend(flatten(row[name]))
    return result


def matrix(value='inherit'):
    return {zone: {client: {'login': value, 'save': value} for client in CLIENTS} for zone in ZONES}


def result_data(result, ok=True):
    status, body = result
    success = status == 200 and isinstance(body, dict) and body.get('code', body.get('errCode')) == 0
    require(success == ok, 'unexpected_api_decision')
    return body.get('data') if isinstance(body, dict) else None


def denied(result):
    status, body = result
    # A dependency failure, timeout, HTTP 404 or successful malformed body is
    # never evidence that authorization was safely rejected.
    return status in (401, 403) or (status == 200 and isinstance(body, dict) and body.get('code') in (401, 403))


def no_tokens(value):
    return isinstance(value, dict) and not set(value).intersection({'token', 'imToken', 'chatToken', 'sessionToken', 'attachmentToken', 'userID'})


def validate_fixture(value, previous):
    require(value.get('namespace') == NAMESPACE and value.get('instanceID') == INSTANCE and value.get('departmentID') == DEPT,
            'fixture_owner_namespace_mismatch')
    accounts = value.get('accounts', [])
    require(isinstance(value.get('runID'), str) and re.fullmatch('[a-f0-9]{32}', value['runID']), 'fixture_run_nonce_required')
    require(len(accounts) == 2 and tuple(row['username'] for row in accounts) == ACCOUNTS
            and tuple(row['name'] for row in accounts) == NAMES, 'exact_new_fixture_names_required')
    uids, employee_ids = set(), set()
    for index, account in enumerate(accounts):
        require(type(account.get('uid')) is int and account['uid'] > 1 and account['uid'] not in uids,
                'fixture_uid_binding_invalid')
        require(isinstance(account.get('employeeID'), str) and re.fullmatch('[A-Za-z0-9_-]{1,64}', account['employeeID'])
                and account['employeeID'] not in employee_ids and not account.get('wecom_userid')
                and account.get('mobile') == '000000000' + ('09' if index == 0 else '10'), 'fixture_employee_binding_invalid')
        require(account.get('remark') == NAMESPACE + ' fixture ' + value['runID'] + ' ' + str(index), 'fixture_creation_nonce_mismatch')
        uids.add(account['uid']); employee_ids.add(account['employeeID'])
    old_uids = {int(row['uid']) for row in previous}
    old_employees = {row['employeeID'] for row in previous}
    require(len(previous) == len(old_uids) == len(old_employees) == 6 and not uids.intersection(old_uids)
            and not employee_ids.intersection(old_employees), 'all_six_previous_identities_must_remain_disjoint')
    require(value.get('excludedUIDs') == sorted(old_uids) and value.get('excludedEmployeeIDs') == sorted(old_employees),
            'previous_identity_proof_mismatch')
    return accounts


def regular(path):
    require(path.is_file() and not path.is_symlink() and path.resolve() == path and path.stat().st_nlink == 1,
            'private_regular_file_required')


def load(path):
    regular(path)
    return json.loads(path.read_text(encoding='utf-8-sig'))


def sync_parent(path):
    if os.name != 'posix':
        return
    descriptor = os.open(path.parent, os.O_RDONLY | getattr(os, 'O_DIRECTORY', 0))
    try:
        os.fsync(descriptor)
    finally:
        os.close(descriptor)


def save(path, value):
    from production_backup import protect
    require(path.resolve().is_relative_to(HOME.resolve()), 'isolated_private_output_required')
    HOME.mkdir(parents=True, exist_ok=True)
    require(HOME.resolve() == HOME and not HOME.is_symlink(), 'isolated_private_directory_required')
    protect(HOME)
    if path.exists(): regular(path)
    temporary = path.with_name(path.name + '.next-' + secrets.token_hex(8))
    try:
        with temporary.open('xb') as handle:
            protect(temporary)
            handle.write(canonical(value) + b'\n'); handle.flush(); os.fsync(handle.fileno())
        temporary.replace(path); sync_parent(path); protect(path)
    finally:
        temporary.unlink(missing_ok=True)


def previous():
    values = []
    for path in OLD_SOURCES:
        source = load(path)
        require(len(source.get('accounts', [])) == 2, 'previous_fixture_receipt_required')
        values.extend(source['accounts'])
    require(len({int(row['uid']) for row in values}) == len({row['employeeID'] for row in values}) == 6,
            'six_disjoint_old_identities_required')
    return values


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, *args, **kwargs):
        raise Rejected('redirect_refused')


class Transport:
    """Normal HTTPS with pinned public enterprise root; no credential arguments."""
    def __init__(self, zone):
        import production_hrm_acceptance as hrm
        self.hrm = hrm
        self.zone = zone
        self.origin = ORIGINS[zone]
        self.host_origin = HOST_ORIGINS[zone]
        self.admin_token = load(PRIVATE / 'hrm-session.json')['token']
        self.admin_observations = []
        self.admin_observation_id = secrets.token_hex(8)
        context = ssl.create_default_context(cafile=str(CA))
        self.opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), urllib.request.HTTPSHandler(context=context), NoRedirect())

    def http(self, origin, path, method='POST', value=None, headers=None, timeout=20):
        require(origin in (*ORIGINS.values(), *HOST_ORIGINS.values()) and path.startswith('/') and not path.startswith('//')
                and '\\' not in path, 'fixed_https_origin_required')
        body = None if value is None else canonical(value)
        supplied = {'operationID': secrets.token_hex(16), **(headers or {})}
        if value is not None: supplied['Content-Type'] = 'application/json'
        request = urllib.request.Request(origin + path, data=body, headers=supplied, method=method)
        try:
            response = self.opener.open(request, timeout=timeout)
        except urllib.error.HTTPError as error:
            response = error
        with response:
            try:
                self.server_time = email.utils.parsedate_to_datetime(response.headers['Date']).timestamp()
            except (ValueError, TypeError, KeyError, OverflowError):
                raise Rejected('trusted_response_date_required')
            raw = response.read(8 * 1024 * 1024 + 1)
            require(len(raw) <= 8 * 1024 * 1024, 'bounded_response_required')
            try: payload = json.loads(raw)
            except (ValueError, UnicodeDecodeError): payload = None
            return response.status, payload

    def admin(self, path, method='GET', value=None):
        # Reuse the existing admin authentication in RAM, never overwrite it.
        route = path.split('?', 1)[0]
        label = {'/admin/system/account/security':'account_security',
            '/admin/system/account/resetPassword':'account_reset_password',
            '/admin/system/account/unlock':'account_unlock',
            '/admin/system/account/lockPolicy':'account_lock_policy',
            '/admin/system/account/accessPolicy':'account_policy',
            '/admin/system/account/getList':'account_inventory',
            '/admin/system/account/save':'account_save',
            '/admin/system/account/upStatus':'account_status',
            '/admin/system/account/del':'account_delete',
            '/admin/system/role/accessPolicy':'role_policy',
            '/admin/system/role/getList':'role_inventory',
            '/admin/system/role/save':'role_save',
            '/admin/system/role/del':'role_delete',
            '/admin/system/dept/getList':'department_guard',
            '/plugins/biz-suite/api/admin/hrm/employees':'employee_records',
            IM_BASE+'employees':'im_projection_inventory',
            IM_BASE+'access/employees':'im_access_inventory',
            IM_BASE+'access/employee':'im_access_save'}.get(route,'admin_other')
        started=time.monotonic()
        try:
            result=self.host(path,method,value,self.admin_token)
        except Exception:
            self.observe_admin({'label':label,'method':method,'status':None,'code':None,'hasData':False,
                'durationMS':round((time.monotonic()-started)*1000),'decision':'transport_failure'})
            raise Rejected('admin_transport_failure') from None
        status,body=result
        code=body.get('code',body.get('errCode')) if isinstance(body,dict) else None
        self.observe_admin({'label':label,'method':method,'status':status if type(status) is int else None,
            'code':code if type(code) is int else None,'hasData':isinstance(body,dict) and body.get('data') is not None,
            'durationMS':round((time.monotonic()-started)*1000),
            'decision':'success' if status==200 and type(code) is int and code==0 else 'api_rejected'})
        return result

    def observe_admin(self, observation):
        self.admin_observations.append(observation)
        save(HOME/('admin-observations-'+self.admin_observation_id+'.private.json'),
             {'namespace':NAMESPACE,'instanceID':INSTANCE,'observations':self.admin_observations})

    def host(self, path, method='GET', value=None, token=None):
        stamp = str(int(time.time()))
        signature = md5(self.hrm.VERIFY + stamp) + '#' + stamp
        headers = {'apiverify': base64.b64encode(signature.encode()).decode(), 'locale': 'zh-CN'}
        if token: headers['Authorization'] = self.hrm.crypt(token + '#' + stamp)
        return self.http(self.host_origin, path, method, value, headers)

    def public(self, path, value=None, method='POST', session=None):
        headers = {}
        if session:
            headers.update(token=session['imToken'], chatToken=session['chatToken'], platformID=str(session['_platform']))
        return self.http(self.origin, path, method, value, headers)

    def revocation_state(self, session, timeout):
        return self.http(self.origin, '/enterprise/v1/session/state', 'GET', headers={
            'token':session['imToken'], 'chatToken':session['chatToken'],
            'platformID':str(session['_platform'])}, timeout=timeout)

    def answer(self, label):
        value = result_data(self.public('/sso/captcha', method='GET'))
        captcha_id = value.get('captchaId', value.get('id'))
        require(value.get('show') is True and isinstance(captcha_id, str) and bool(captcha_id), 'normal_captcha_must_be_enabled')
        image = value.get('image', value.get('img', ''))
        require(isinstance(image, str) and image.startswith('data:image/'), 'normal_captcha_image_required')
        image_path = HOME / ('captcha-' + label + '.png')
        save(HOME / ('captcha-' + label + '.json'), value)
        from production_backup import protect
        image_path.write_bytes(base64.b64decode(image.split(',', 1)[1])); protect(image_path)
        print(json.dumps({'captchaRequired': label, 'image': str(image_path), 'normalEndpoint': True}), flush=True)
        # The operator reads the image normally. getpass keeps the answer out of
        # terminal command history, reports, argv and shell transcripts.
        import getpass
        answer = getpass.getpass('Captcha answer: ')
        require(bool(answer), 'captcha_answer_required')
        return {'captchaId': captcha_id, 'captcha': answer}


class Acceptance:
    def __init__(self, transport, state, persist=lambda value: None, record=lambda label, passed: None):
        self.t, self.state, self.persist, self.record = transport, state, persist, record

    @property
    def accounts(self): return self.state['accounts']

    def check(self, label, condition):
        self.record(label, bool(condition))
        require(condition, label)

    def admin(self, path, method='GET', value=None):
        return result_data(self.t.admin(path, method, value))

    def security(self, account):
        return self.admin('/admin/system/account/security?id=' + str(account['uid']))

    def login(self, account, label, client='web_desktop', wrong=False, challenge=False):
        platform, number = CLIENTS[client]
        cap = self.t.answer(label)
        started = time.monotonic()
        result = self.t.public('/sso/login', {'account': account['username'], 'password': md5('wrong-synthetic-password' if wrong else account['password']),
            'platform': platform, 'policyClient': client, **cap})
        status, body = result
        code = body.get('code', body.get('errCode')) if isinstance(body, dict) else None
        message = body.get('msg', body.get('message', body.get('errMsg'))) if isinstance(body, dict) else None
        message = message if isinstance(message, str) else None
        category = {'账号、密码或通讯权限无效':'authentication_or_policy_denied',
                    '通讯会话已失效，请重新登录':'expired_generation',
                    '通讯服务暂不可用，请稍后重试':'dependency_unavailable',
                    '请求过于频繁':'rate_limited'}.get(message, 'unclassified')
        if status == 200 and code == 0: category = 'success'
        if status == 428: category = 'password_change_required'
        self.state.setdefault('loginObservations', []).append({'label':label, 'client':client,
             'status':status if type(status) is int else None, 'code':code if type(code) is int else None,
             'durationMS':round((time.monotonic()-started)*1000),
             'hasData':isinstance(body, dict) and body.get('data') is not None, 'messageClass':category})
        self.persist(self.state)
        if wrong:
            self.check(label + '_password_rejected', denied(result)); return None
        if challenge:
            self.check(label + '_428', result[0] == 428 and isinstance(result[1], dict))
            data = result[1].get('data')
            self.check(label + '_restricted_no_session', no_tokens(data) and data.get('reasonCode') == 'PASSWORD_CHANGE_REQUIRED'
                       and data.get('requiresPasswordChange') is True and re.fullmatch('[a-f0-9]{64}', data.get('changeTicket', '')) is not None
                       and 0 < data.get('expiresAt', 0) - time.time() <= 601)
            return data
        data = result_data(result)
        self.check(label + '_identity', data.get('userID') == account['userID'] and data.get('policyClient') == client
                   and all(isinstance(data.get(key), str) and data[key] for key in ('imToken', 'chatToken', 'sessionToken')))
        data.update(_platform=number, _client=client)
        self.state.setdefault('sessions', {})[label] = data; self.persist(self.state)
        return data

    def set_person(self, account, value):
        old = self.admin('/admin/system/account/accessPolicy?id=' + str(account['uid']))
        return self.admin('/admin/system/account/accessPolicy', 'POST', {'id': account['uid'], 'revision': old['revision'], 'matrix': value})

    def set_role(self, role, value):
        old = self.admin('/admin/system/role/accessPolicy?id=' + str(role))
        return self.admin('/admin/system/role/accessPolicy', 'POST', {'id': role, 'revision': old['revision'], 'matrix': value})

    def restricted_change(self, account, label, cancel_first=False):
        ticket = self.login(account, label, challenge=True)['changeTicket']
        status = result_data(self.t.public('/sso/password/status', {'changeTicket': ticket}))
        self.check(label + '_status_restricted', no_tokens(status) and status.get('requiresPasswordChange') is True)
        self.check(label + '_ticket_not_session', denied(self.t.public('/sso/refresh', {'sessionToken': ticket})))
        if cancel_first:
            cancelled = result_data(self.t.public('/sso/password/cancel', {'changeTicket':ticket}))
            self.check(label + '_cancel_restricts_ticket', cancelled.get('cancelled') is True
                       and denied(self.t.public('/sso/password/status', {'changeTicket':ticket})))
            ticket = self.login(account, label + '-after-cancel', challenge=True)['changeTicket']
        next_password = secrets.token_urlsafe(24)
        account['pendingPassword'] = next_password; self.persist(self.state)
        changed = result_data(self.t.public('/sso/password/change', {'changeTicket': ticket, 'newPassword': md5(next_password)}))
        self.check(label + '_requires_fresh_login', no_tokens(changed) and changed.get('changed') is True and changed.get('requiresLogin') is True)
        account['password'] = next_password; account.pop('pendingPassword', None); self.persist(self.state)
        self.check(label + '_ticket_replay_denied', denied(self.t.public('/sso/password/change', {'changeTicket': ticket, 'newPassword': md5('another-synthetic-password')})))

    def assert_revoked(self, sessions, host_token=None):
        if host_token:
            self.check('old_host_token_rejected', denied(self.t.host('/admin/user/getUserinfo', token=host_token)))
        for index, session in enumerate(sessions):
            self.check('old_sso_' + str(index) + '_rejected', denied(self.t.public('/sso/refresh', {'sessionToken': session['sessionToken']})))
            self.assert_im_revoked(session, 'old_im_' + str(index) + '_rejected')

    def assert_im_revoked(self, session, label):
        started = time.monotonic()
        attempt = 0
        while True:
            remaining = 10 - (time.monotonic() - started)
            if remaining <= 0:
                self.check(label, False)
            attempt += 1
            before = time.monotonic()
            try:
                result = self.t.revocation_state(session, max(0.001, remaining))
            except Exception:
                self.state.setdefault('revocationObservations', []).append({'label':label, 'attempt':attempt,
                    'status':None, 'code':None, 'elapsedMS':round((time.monotonic()-started)*1000),
                    'durationMS':round((time.monotonic()-before)*1000), 'transportFailure':True})
                self.persist(self.state)
                self.check(label, False)
            status, body = result
            code = body.get('errCode', body.get('code')) if isinstance(body, dict) else None
            observation = {'label':label, 'attempt':attempt, 'status':status if type(status) is int else None,
                'code':code if type(code) is int else None, 'elapsedMS':round((time.monotonic()-started)*1000),
                'durationMS':round((time.monotonic()-before)*1000)}
            self.state.setdefault('revocationObservations', []).append(observation)
            self.persist(self.state)
            # Only explicit HTTP rejection is proof. A 200 never gets a retry,
            # including a malformed/empty 200 or a body carrying an error code.
            no_data = isinstance(body, dict) and body.get('data') is None and no_tokens(body)
            if status in (401, 403) and no_data and time.monotonic()-started <= 10:
                self.check(label, True)
                return
            temporary = (status == 503 and type(code) is int and code == 1006
                         and no_data)
            if not temporary or time.monotonic()-started >= 10:
                self.check(label, False)
            time.sleep(min(0.5, max(0, 10-(time.monotonic()-started))))

    def bootstrap(self):
        for index, account in enumerate(self.accounts):
            self.restricted_change(account, 'bootstrap-' + str(index), cancel_first=index == 0)

    def roles(self):
        a, b = matrix('deny'), matrix('deny')
        a['lan']['pc']['login'] = 'allow'; b['lan']['pc']['save'] = 'allow'
        b['internet']['ios'] = {'login': 'allow', 'save': 'allow'}
        self.set_role(self.state['roles'][0]['id'], a); self.set_role(self.state['roles'][1]['id'], b)
        self.set_person(self.accounts[0], matrix())
        view = self.admin('/admin/system/account/accessPolicy?id=' + str(self.accounts[0]['uid']))
        for zone in ZONES:
            for client in CLIENTS:
                login = a[zone][client]['login'] == 'allow' or b[zone][client]['login'] == 'allow'
                save_allowed = login and (a[zone][client]['save'] == 'allow' or b[zone][client]['save'] == 'allow')
                effective = view['effective'][zone][client]
                self.check('role_or_' + zone + '_' + client, effective['login'] == login and effective['preview'] == login and effective['save'] == save_allowed)
        personal = matrix(); personal['lan']['pc'] = {'login': 'deny', 'save': 'allow'}
        personal['lan']['web_desktop'] = {'login': 'allow', 'save': 'deny'}
        self.set_person(self.accounts[0], personal)
        view = self.admin('/admin/system/account/accessPolicy?id=' + str(self.accounts[0]['uid']))['effective']['lan']
        self.check('personal_deny_beats_role_or', view['pc']['login'] is False and view['pc']['save'] is False)
        self.check('personal_allow_beats_role_deny_preview_not_save', view['web_desktop']['login'] is True and view['web_desktop']['preview'] is True and view['web_desktop']['save'] is False)
        self.set_person(self.accounts[0], matrix('allow'))

    def lock(self):
        account = self.accounts[0]
        policy = self.admin('/admin/system/account/lockPolicy')
        self.check('default_600s_5fail_1800s', (policy['windowSeconds'], policy['failures'], policy['lockSeconds']) == (600, 5, 1800))
        self.admin('/admin/system/account/unlock', 'POST', {'id': account['uid']})
        before = self.security(account)
        self.check('clean_failure_baseline', not before['locked'] and before['recentFailures'] == 0)
        result = self.t.public('/sso/login', {'account': account['username'], 'password': md5('wrong-synthetic-password'), 'platform': 'web', 'captchaId': 'invalid-captcha', 'captcha': 'invalid'})
        self.check('bad_captcha_rejected', denied(result))
        after = self.security(account)
        self.check('bad_captcha_not_password_failure', after['recentFailures'] == 0 and after['securityVersion'] == before['securityVersion'])
        old = self.login(account, 'before-lock')
        old_pc = self.login(account, 'pc-before-lock', 'pc')
        host_token = self.capture_host(account, 'host-before-lock')
        started = time.time()
        for index in range(5):
            self.login(account, 'wrong-' + str(index + 1), wrong=True)
            current = self.security(account)
            self.check('rolling_failure_' + str(index + 1), (current['recentFailures'] == index + 1 and not current['locked']) if index < 4 else current['locked'])
        self.check('failures_within_600s', time.time() - started < 600)
        # Date comes from the authenticated HTTPS response, avoiding an incorrect
        # failure or pass caused solely by an operator workstation clock offset.
        remaining = current['lockedUntil'] - getattr(self.t, 'server_time', time.time())
        self.check('lock_duration_1800s', 1790 <= remaining <= 1802)
        self.check('lock_security_version_advanced', current['securityVersion'] > before['securityVersion'])
        self.assert_revoked([old, old_pc], host_token)
        self.admin('/admin/system/account/unlock', 'POST', {'id': account['uid']})
        current = self.security(account)
        self.check('admin_unlock_clears_window', current['locked'] is False and current['lockedUntil'] == 0 and current['recentFailures'] == 0)
        self.assert_revoked([old, old_pc], host_token)

    def capture_host(self, account, label):
        cap = self.t.answer(label)
        result = self.t.host('/admin/user/login', 'POST', {'username': account['username'], 'password': md5(account['password']), 'captcha': cap['captcha'], 'codeid': cap['captchaId']})
        value = result_data(result)
        require(isinstance(value, str), 'normal_host_encrypted_token_required')
        token = self.t.hrm.crypt(value, True)
        self.state.setdefault('hostTokens', {})[label] = token; self.persist(self.state)
        result_data(self.t.host('/admin/user/getUserinfo', token=token))
        self.check(label + '_host_authenticated', True)
        return token

    def password(self):
        account = self.accounts[0]
        old = self.login(account, 'before-self-change')
        host_token = self.capture_host(account, 'host-before-self-change')
        before = self.security(account)
        result = self.t.public('/sso/password/change', {'sessionToken': old['sessionToken'], 'oldPassword': md5('wrong-synthetic-old-password'), 'newPassword': md5('synthetic-new-password')})
        self.check('self_change_requires_old_password', denied(result) and self.security(account)['securityVersion'] == before['securityVersion'])
        prior_version = projection_versions(self.t, self.state)[account['employeeID']]
        new_password = secrets.token_urlsafe(24); account['pendingPassword'] = new_password; self.persist(self.state)
        changed = result_data(self.t.public('/sso/password/change', {'sessionToken': old['sessionToken'], 'oldPassword': md5(account['password']), 'newPassword': md5(new_password)}))
        self.check('self_change_no_new_session', no_tokens(changed) and changed.get('requiresLogin') is True)
        account['password'] = new_password; account.pop('pendingPassword', None); self.persist(self.state)
        self.assert_revoked([old], host_token)
        wait_login_ready(self.t, self.state, minimum_versions={account['employeeID']:prior_version})
        self.admin_reset_remaining()

    def admin_reset_remaining(self):
        account = self.accounts[0]
        fresh = self.login(account, 'before-admin-reset')
        host_token = self.capture_host(account, 'host-before-reset')
        prior_version = projection_versions(self.t, self.state)[account['employeeID']]
        temporary = secrets.token_urlsafe(24); account['pendingPassword'] = temporary; self.persist(self.state)
        reset = self.admin('/admin/system/account/resetPassword', 'POST', {'id': account['uid'], 'password': md5(temporary)})
        self.check('admin_reset_forces_change', reset.get('reset') is True and self.security(account)['mustChangePassword'] is True)
        account['password'] = temporary; account.pop('pendingPassword', None); self.persist(self.state)
        self.assert_revoked([fresh], host_token)
        self.restricted_change(account, 'after-admin-reset')
        wait_login_ready(self.t, self.state, minimum_versions={account['employeeID']:prior_version})

    def clients(self):
        account = self.accounts[0]
        self.set_person(account, matrix('deny'))
        for client, (platform, _) in CLIENTS.items():
            cap = self.t.answer('deny-client-' + client)
            self.check('login_denied_' + client, denied(self.t.public('/sso/login', {
                'account':account['username'], 'password':md5(account['password']),
                'platform':platform, 'policyClient':client, **cap})))
        personal = matrix('allow')
        for zone in ZONES:
            for index, client in enumerate(CLIENTS): personal[zone][client]['save'] = 'allow' if index % 2 else 'deny'
        self.set_person(account, personal)
        view = self.admin('/admin/system/account/accessPolicy?id=' + str(account['uid']))['effective']
        for zone in ZONES:
            for client in CLIENTS:
                cell = view[zone][client]
                self.check('two_zone_matrix_' + zone + '_' + client, cell['login'] is True and cell['preview'] is True
                           and cell['save'] == (personal[zone][client]['save'] == 'allow'))
        for client in CLIENTS:
            session = self.login(account, 'client-' + client, client)
            effective = session.get('effectivePolicy', {})
            expected = personal[self.t.zone][client]['save'] == 'allow'
            self.check('real_source_zone_' + client, effective.get('zone') == self.t.zone and effective.get('logicalClient') == client)
            self.check('login_preview_save_' + client, effective.get('login') is True and effective.get('preview') is True and effective.get('save') == expected)
            live = result_data(self.t.public('/enterprise/v1/session/state', method='GET', session=session))
            self.check('live_state_policy_' + client, live.get('loginAllowed') is True and live.get('saveAllowed') == expected and live.get('logicalClient') == client
                       and live.get('epoch') == effective.get('securityVersion') and live.get('effectivePolicy', {}).get('zone') == self.t.zone)
            result_data(self.t.public('/sso/logout', {'sessionToken': session['sessionToken']}))
        self.check('matrix_two_zone_admin_view', set(self.admin('/admin/system/account/accessPolicy?id=' + str(account['uid']))['effective']) == set(ZONES))

    def run(self):
        self.bootstrap(); self.roles(); self.lock(); self.password(); self.clients()


def exact_department(t):
    matches = [row for row in flatten(result_data(t.admin('/admin/system/dept/getList'))) if int(row.get('id', 0)) == DEPT]
    require(len(matches) == 1, 'protected_department_missing')
    row = matches[0]
    require(row.get('name') == 'TEMP QA 20261103' and row.get('remark') == 'Temporary synthetic organization acceptance department'
            and all(int(row.get(key, -1)) == value for key, value in {'id':DEPT, 'pid':0, 'business_id':1, 'status':0, 'leader_uid':0, 'im_group_enabled':0}.items()),
            'protected_department_changed_no_writes_allowed')
    return {key: row[key] for key in ('id', 'pid', 'name', 'remark', 'business_id', 'status', 'leader_uid', 'im_group_enabled')}


def account_id(value):
    value = value.get('id') if isinstance(value, dict) else value
    require(str(value).isdigit() and int(value) > 1, 'new_record_id_required')
    return int(value)


def current_host(t, account):
    found = [row for row in rows(result_data(t.admin('/admin/system/account/getList?page=1&pageSize=500')))
             if int(row.get('id', 0)) == account['uid']]
    require(len(found) == 1 and found[0].get('username') == account['username'] and found[0].get('name') == account['name']
            and found[0].get('remark') == account['remark']
            and int(found[0].get('dept_id', -1)) == DEPT and str(found[0].get('mobile', '')) == account['mobile'], 'exact_host_binding_changed')
    return found[0]


def current_employee(t, account):
    found = [row for row in rows(result_data(t.admin('/plugins/biz-suite/api/admin/hrm/employees?page=1&page_size=500')))
             if row.get('id') == account['employeeID']]
    require(len(found) == 1 and found[0].get('employee_no') == account['username'] and found[0].get('name') == account['name']
            and int(found[0].get('user_id', 0)) == account['uid'] and not found[0].get('wecom_userid')
            and int(found[0].get('dept_id', -1)) == DEPT and str(found[0].get('mobile', '')) == account['mobile'], 'exact_employee_binding_changed')
    return found[0]


def role_rows(t):
    return flatten(result_data(t.admin('/admin/system/role/getList')))


def preflight(t):
    policy = result_data(t.admin('/admin/system/account/lockPolicy'))
    require((policy.get('windowSeconds'), policy.get('failures'), policy.get('lockSeconds')) == (600, 5, 1800), 'deployed_default_lock_policy_required')
    require(denied(t.public('/sso/password/status', {'changeTicket':'0' * 64})), 'deployed_restricted_password_route_required')
    require(denied(t.public('/enterprise/v1/session/state', method='GET')), 'deployed_authenticated_session_state_required')


def create(t, persist):
    require(not FIXTURE.exists() and not STATE.exists(), 'new_namespace_already_exists_never_recreate')
    old = previous()
    marker = load(PRIVATE.parent / 'openim/instance.json')
    require(marker.get('instanceID') == INSTANCE and marker.get('production') is True and marker.get('root') == '/srv/puty-suite/openim', 'instance_marker_mismatch')
    department = exact_department(t)
    hosts = rows(result_data(t.admin('/admin/system/account/getList?page=1&pageSize=500')))
    require(not any(row.get('username') in ACCOUNTS for row in hosts), 'new_username_collision')
    require(not any(row.get('name') in ROLE_NAMES for row in role_rows(t)), 'new_role_collision')
    value = {'namespace':NAMESPACE, 'instanceID':INSTANCE, 'departmentID':DEPT, 'department':department, 'runID':secrets.token_hex(16),
             'excludedUIDs':sorted(int(row['uid']) for row in old), 'excludedEmployeeIDs':sorted(row['employeeID'] for row in old),
             'accounts':[{'username':name, 'name':NAMES[index], 'password':secrets.token_urlsafe(24),
                          'mobile':'000000000' + ('09' if index == 0 else '10'), 'wecom_userid':''} for index, name in enumerate(ACCOUNTS)],
             'roles':[], 'sessions':{}, 'hostTokens':{}, 'createdAt':int(time.time())}
    persist(value)
    try:
        for index, account in enumerate(value['accounts']):
            account['remark'] = NAMESPACE + ' fixture ' + value['runID'] + ' ' + str(index)
        persist(value)
        for name in ROLE_NAMES:
            role = {'name':name, 'remark':NAMESPACE + ' isolated policy role ' + value['runID']}
            value['roles'].append(role); persist(value)
            role['id'] = account_id(result_data(t.admin('/admin/system/role/save', 'POST', {'id':0, 'pid':0, 'name':name, 'remark':role['remark'], 'weigh':9999, 'data_access':0, 'menu':[], 'btns':[], 'rules':[]})))
            persist(value)
        for account in value['accounts']:
            account['createAttempted'] = True; persist(value)
            account['uid'] = account_id(result_data(t.admin('/admin/system/account/save', 'POST', {'id':0, 'name':account['name'], 'username':account['username'], 'password':md5(account['password']), 'roleid':[], 'dept_id':DEPT, 'mobile':account['mobile'], 'tel':'', 'email':'', 'remark':account['remark']})))
            persist(value)
            require(account['uid'] not in value['excludedUIDs'], 'new_uid_reused_previous_identity')
            employee = result_data(t.admin('/plugins/biz-suite/api/admin/hrm/employees', 'POST', {'user_id':account['uid'], 'employee_no':account['username'], 'name':account['name'], 'status':'active', 'dept_id':DEPT, 'mobile':account['mobile'], 'wecom_userid':''}))
            account['employeeID'] = employee['id']
            account['userID'] = 'emp_' + hashlib.sha256(account['employeeID'].encode()).hexdigest()[:32]
            persist(value)
        validate_fixture(value, old)
        save(FIXTURE, value)
        return value
    except BaseException:
        # The durable partial state is retained for exact cleanup; never invent
        # an employee ID or silently claim that partial creation was rolled back.
        value['creationFailed'] = True; persist(value)
        raise


def projection_versions(t, value):
    wanted = {a['employeeID'] for a in value['accounts']}
    found = [r for r in rows(result_data(t.admin(IM_BASE+'employees'))) if r.get('id') in wanted]
    require(len(found) == len(wanted) == 2 and all(type(r.get('desiredVersion')) is int for r in found),
            'exact_projection_version_snapshot_required')
    return {r['id']:r['desiredVersion'] for r in found}


def wait_projection(t, value, enabled=True, timeout=150, minimum_versions=None):
    wanted = {row['employeeID'] for row in value['accounts'] if row.get('employeeID')}
    require(len(wanted) == 2, 'complete_pair_required_for_projection_proof')
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        found = [row for row in rows(result_data(t.admin(IM_BASE + 'employees'))) if row.get('id') in wanted]
        if len(found) == 2 and all(row.get('projection', {}).get('enabled') is enabled and int(row.get('desiredVersion', -1)) == int(row.get('appliedVersion', -2)) and not row.get('error')
                and (row['id'] not in (minimum_versions or {}) or int(row['desiredVersion']) > minimum_versions[row['id']]) for row in found):
            return
        time.sleep(2)
    raise Rejected('fixture_projection_not_converged')


def wait_login_ready(t, value, timeout=150, minimum_versions=None):
    wanted={a['employeeID']:a for a in value['accounts']}
    require(len(wanted)==2,'complete_pair_required_for_projection_proof')
    deadline=time.monotonic()+timeout
    while time.monotonic()<deadline:
        access=rows(result_data(t.admin(IM_BASE+'access/employees')))
        found=[r for r in access if r.get('employeeID') in wanted]
        eligible=(len(found)==2 and all(r.get('eligible') is True
            and type(r.get('uid')) is int and r['uid']==wanted[r['employeeID']]['uid']
            and r.get('userID')==wanted[r['employeeID']]['userID'] for r in found))
        projected=[r for r in rows(result_data(t.admin(IM_BASE+'employees'))) if r.get('id') in wanted]
        ready=(len(projected)==2 and all(r.get('projection',{}).get('enabled') is True
            and int(r.get('desiredVersion',-1))==int(r.get('appliedVersion',-2)) and not r.get('error')
            and (r['id'] not in (minimum_versions or {}) or int(r['desiredVersion'])>minimum_versions[r['id']]) for r in projected))
        if eligible and ready:return
        time.sleep(2)
    raise Rejected('fixture_live_login_not_ready')


def assign_roles(t, value, persist):
    for account in value['accounts']:
        row = current_host(t, account)
        role_ids = row.get('roleid')
        require(isinstance(role_ids, list) and role_ids and all(str(role).isdigit() and int(role) > 0 for role in role_ids), 'existing_base_IM_roles_required')
        account['baseRoleIDs'] = [int(role) for role in role_ids]; persist(value)
        additions = [role['id'] for role in value['roles']] if account is value['accounts'][0] else []
        payload = {key: row.get(key, '') for key in ('name', 'username', 'mobile', 'tel', 'email', 'remark', 'avatar')}
        payload.update(id=account['uid'], dept_id=DEPT, roleid=account['baseRoleIDs'] + additions)
        result_data(t.admin('/admin/system/account/save', 'POST', payload))
    wait_projection(t, value)


def validate_role_proof(proof, value, fixture_sha, now=None):
    now = time.time() if now is None else now
    require(isinstance(proof, dict) and proof.get('format') == 'puty-security-hidden-role-proof-v1'
            and proof.get('namespace') == NAMESPACE and proof.get('instanceID') == INSTANCE
            and proof.get('runID') == value.get('runID') and proof.get('fixtureSourceSHA256') == fixture_sha
            and type(proof.get('capturedAt')) is int and 0 <= now-proof['capturedAt'] <= 1800,
            'hidden_role_proof_owner_or_freshness_rejected')
    expected = value.get('roles', [])
    require(len(expected) == 2 and len(proof.get('roles', [])) == 2
            and len({r.get('id') for r in expected}) == 2, 'hidden_role_proof_exact_pair_required')
    for original in expected:
        found = [r for r in proof['roles'] if r.get('id') == original.get('id')]
        require(len(found) == 1, 'hidden_role_proof_identity_rejected')
        row = found[0]
        require(type(row.get('id')) is int and row['id'] > 1 and row.get('name') == original.get('name')
                and row.get('remark') == original.get('remark') and row.get('name') in ROLE_NAMES
                and row.get('remark') == NAMESPACE+' isolated policy role '+value['runID']
                and type(row.get('pid')) is int and row['pid'] == 0 and row.get('deleted') is False
                and type(row.get('childCount')) is int and row['childCount'] == 0
                and type(row.get('foreignMemberCount')) is int and row['foreignMemberCount'] == 0
                and isinstance(row.get('memberUIDs'), list) and len(row['memberUIDs']) <= 1
                and all(type(uid) is int and uid == value['accounts'][0]['uid'] for uid in row['memberUIDs']),
                'hidden_role_proof_scope_rejected')
    return proof


def isolated_role_inventory(t, value, hosts, role_proof=None):
    role_ids = {int(role['id']) for role in value.get('roles', []) if role.get('id')}
    require(len(role_ids) == len([r for r in value.get('roles', []) if r.get('id')]), 'cleanup_role_ids_not_distinct')
    if isinstance(role_proof, dict) and role_proof.get('format') == 'puty-security-host-restore-v1':
        validate_retired_roles(role_proof, value, hashlib.sha256(FIXTURE.read_bytes()).hexdigest())
        # The exact roles were physically retired in the preceding cleanup.
        # Never delete them again, even if an external actor later reuses an ID.
        require(not any(role_ids.intersection(int(r) for r in row.get('roleid', [])) for row in hosts),
                'retired_role_membership_reappeared')
        return set()
    current_roles = role_rows(t)
    for role in value.get('roles', []):
        if not role.get('id'): continue
        matches = [r for r in current_roles if int(r.get('id', 0)) == role['id']]
        if not matches and role_proof is not None:
            # Only a parent-produced, exact SHA-pinned SQL SELECT receipt may
            # supply metadata hidden by the host's descendant-only role list.
            validate_role_proof(role_proof, value, hashlib.sha256(FIXTURE.read_bytes()).hexdigest())
            matches = [r for r in role_proof['roles'] if r['id'] == role['id']]
        require(len(matches) == 1 and role.get('name') in ROLE_NAMES and matches[0].get('name') == role['name']
                and matches[0].get('remark') == role['remark'] and int(matches[0].get('pid', -1)) == 0
                and not any(int(r.get('pid', 0)) == role['id'] for r in current_roles),
                'cleanup_role_descendant_or_binding_mismatch')
    wanted = {a['uid'] for a in value['accounts'] if a.get('uid')}
    for row in hosts:
        roles = row.get('roleid')
        require(isinstance(roles, list) and all(str(r).isdigit() and int(r) > 0 for r in roles),
                'cleanup_membership_shape_rejected')
        require(int(row['id']) in wanted or not role_ids.intersection(int(r) for r in roles),
                'cleanup_role_has_foreign_member')
    return role_ids


def quiesce(t, value, persist, report, role_proof=None):
    require(value.get('namespace') == NAMESPACE and value.get('instanceID') == INSTANCE and value.get('departmentID') == DEPT,
            'cleanup_namespace_mismatch')
    exact_department(t)
    expected = []
    # Validate the complete namespace shape before the first mutation.
    require(tuple(row.get('username') for row in value.get('accounts', [])) == ACCOUNTS
            and tuple(row.get('name') for row in value['accounts']) == NAMES, 'cleanup_exact_new_names_required')
    require(isinstance(value.get('runID'), str) and re.fullmatch('[a-f0-9]{32}', value['runID']), 'cleanup_creation_nonce_required')
    # Recover only a reply-lost creation with the exact prewritten random nonce.
    # This never adopts another run's fixture merely because its name matches.
    hosts = rows(result_data(t.admin('/admin/system/account/getList?page=1&pageSize=500')))
    employees = rows(result_data(t.admin('/plugins/biz-suite/api/admin/hrm/employees?page=1&page_size=500')))
    for index, account in enumerate(value['accounts']):
        require(account.get('remark') == NAMESPACE + ' fixture ' + value['runID'] + ' ' + str(index), 'cleanup_fixture_nonce_mismatch')
        if account.get('createAttempted') and not account.get('uid'):
            matches = [row for row in hosts if row.get('username') == account['username'] and row.get('name') == account['name'] and row.get('remark') == account['remark']]
            require(len(matches) <= 1, 'cleanup_reply_lost_host_ambiguous')
            if matches: account['uid'] = account_id(matches[0]['id']); persist(value)
        if account.get('uid') and not account.get('employeeID'):
            matches = [row for row in employees if int(row.get('user_id', 0)) == account['uid'] and row.get('employee_no') == account['username'] and row.get('name') == account['name'] and not row.get('wecom_userid')]
            require(len(matches) <= 1, 'cleanup_reply_lost_employee_ambiguous')
            if matches: account['employeeID'] = matches[0]['id']; persist(value)
    available_roles = role_rows(t)
    for role in value.get('roles', []):
        require(role.get('name') in ROLE_NAMES and role.get('remark') == NAMESPACE + ' isolated policy role ' + value['runID'], 'cleanup_role_nonce_mismatch')
        if not role.get('id'):
            matches = [row for row in available_roles if row.get('name') == role['name'] and row.get('remark') == role['remark']]
            require(len(matches) <= 1, 'cleanup_reply_lost_role_ambiguous')
            if matches: role['id'] = account_id(matches[0]['id']); persist(value)
    old_uids = set(value.get('excludedUIDs', []))
    require(len(old_uids) == 6, 'cleanup_previous_identity_proof_required')
    for account in value['accounts']:
        if not account.get('uid'):
            continue
        require(type(account['uid']) is int and account['uid'] > 1 and account['uid'] not in old_uids, 'cleanup_uid_outside_scope')
        host = current_host(t, account)
        employee = current_employee(t, account) if account.get('employeeID') else None
        expected.append((account, host, employee))
    engine = Acceptance(t, value, persist, report)
    for account, host, employee in expected:
        personal = result_data(t.admin('/admin/system/account/accessPolicy?id='+str(account['uid'])))
        if personal.get('matrix') != matrix('deny'):
            require(int(host.get('status', -1)) == 0, 'cleanup_disabled_account_policy_not_closed')
            engine.set_person(account, matrix('deny'))
        if employee:
            policies = rows(result_data(t.admin(IM_BASE + 'access/employees')))
            matches = [row for row in policies if row.get('employeeID') == account['employeeID'] and int(row.get('uid', 0)) == account['uid']]
            require(len(matches) == 1, 'cleanup_exact_legacy_access_binding_required')
            keys = ('allowLAN','allowPC','allowMobile','allowInternet','allowInternetDownload')
            require(all(type(matches[0].get(k)) is bool for k in keys), 'cleanup_legacy_access_shape_rejected')
            if any(matches[0][k] for k in keys):
                require(int(host.get('status', -1)) == 0, 'cleanup_disabled_account_legacy_not_closed')
                result_data(t.admin(IM_BASE + 'access/employee', 'POST', {'employeeID':account['employeeID'],
                     'expectedRevision':matches[0]['revision'], 'allowLAN':False, 'allowPC':False, 'allowMobile':False,
                     'allowInternet':False, 'allowInternetDownload':False}))
        if int(host.get('status', -1)) != 1:
            result_data(t.admin('/admin/system/account/upStatus', 'POST', {'id':account['uid'], 'status':1}))
        if employee and employee.get('status') != 'inactive':
            from production_fixture_cleanup import EMPLOYEE_FIELDS
            payload = {key:employee.get(key, '') for key in EMPLOYEE_FIELDS}
            payload.update(status='inactive', wecom_userid='')
            result_data(t.admin('/plugins/biz-suite/api/admin/hrm/employees', 'POST', payload))
    # Role deletion is recursive in the host. Require no descendants or foreign
    # memberships, instead of relying on the role name alone.
    host_page = result_data(t.admin('/admin/system/account/getList?page=1&pageSize=500'))
    hosts = rows(host_page)
    require(isinstance(host_page, dict) and type(host_page.get('total')) is int
            and len(hosts) == host_page['total'] <= 500, 'cleanup_complete_membership_inventory_required')
    role_ids = isolated_role_inventory(t, value, hosts, role_proof)
    if role_ids: result_data(t.admin('/admin/system/role/del', 'DELETE', {'ids':sorted(role_ids)}))
    plan_rows = []
    for account, host, employee in expected:
        if employee:
            plan_rows.append({'expected':{'uid':account['uid'], 'username':account['username'], 'name':account['name'], 'employeeID':account['employeeID'],
                                'departmentID':DEPT, 'hostDepartmentID':DEPT, 'mobile':account['mobile'], 'hostMobile':account['mobile']}, 'host':host, 'employee':employee})
    if len(plan_rows) == 2:
        plan = {'format':'puty-fixture-cleanup-v1', 'instanceID':INSTANCE, 'acceptanceNamespace':NAMESPACE,
                'createdAt':int(time.time()), 'fixtureSourceSHA256':hashlib.sha256(FIXTURE.read_bytes()).hexdigest(), 'fixtures':plan_rows, 'groups':[],
                'preserve':['soft-deleted host records', 'projections', 'security epoch', 'audit', 'outbox', 'revocation tombstones'],
                'scope':'exact-new-security-I/J-only; no-department-group-message-or-attachment-writes'}
        save(HOME / 'fixture-cleanup-plan.private.json', plan)
    for account, _, _ in expected:
        result_data(t.admin('/admin/system/account/del', 'DELETE', {'ids':[account['uid']]}))
    value['quiesced'] = True; persist(value)
    if len(plan_rows) == 2:
        wait_projection(t, value, False)
        plan_path = HOME / 'fixture-cleanup-plan.private.json'
        save(HOME / 'fixture-cleanup-progress.private.json', {'format':'puty-fixture-cleanup-v1', 'instanceID':INSTANCE,
             'planSHA256':hashlib.sha256(plan_path.read_bytes()).hexdigest(), 'passed':True,
             'steps':['all-matrix-denied', 'accounts-disabled', 'employees-inactive', 'isolated-roles-deleted', 'hosts-softdeleted', 'disabled-projections-applied']})
    report('finally_fixture_quiesced', len(expected) == 2)
    return {'fixtureCount':len(expected), 'employeePhysicalDeletePending':len(plan_rows), 'rolesDeleted':len(role_ids), 'departmentUntouched':True}


def pinned_json(path, expected):
    require(isinstance(expected, str) and re.fullmatch('[0-9a-f]{64}', expected), 'resume_explicit_SHA_required')
    regular(path)
    with path.open('rb') as source:
        before = os.fstat(source.fileno())
        raw = source.read(4 * 1024 * 1024 + 1)
        after = os.fstat(source.fileno())
    current = path.stat()
    identity = lambda s:(s.st_dev, s.st_ino, s.st_size, s.st_mtime_ns)
    require(len(raw) <= 4 * 1024 * 1024 and identity(before) == identity(after) == identity(current)
            and hashlib.sha256(raw).hexdigest() == expected, 'resume_input_SHA_or_identity_changed')
    return raw, json.loads(raw.decode('utf-8-sig'))


def load_role_proof(path, expected, value):
    require(path is not None and path.resolve().is_relative_to(HOME.resolve()), 'isolated_role_proof_required')
    _, proof = pinned_json(path, expected)
    return validate_role_proof(proof, value, hashlib.sha256(FIXTURE.read_bytes()).hexdigest())


def preserve_original_attempt(raw, expected, prefix='original-attempt'):
    from production_backup import protect
    require(prefix in ('original-attempt','original-state','original-cleanup-plan','original-cleanup-progress'), 'original_receipt_prefix_rejected')
    path = HOME / (prefix + '-' + expected + '.json')
    if path.exists():
        regular(path)
        require(path.read_bytes() == raw, 'original_attempt_receipt_changed')
        with path.open('r+b') as target:
            target.flush(); os.fsync(target.fileno())
        sync_parent(path)
        regular(path)
        require(path.read_bytes() == raw, 'original_attempt_receipt_changed')
        return
    protect(HOME)
    with path.open('xb') as target:
        protect(path)
        target.write(raw); target.flush(); os.fsync(target.fileno())
    sync_parent(path)
    require(path.read_bytes() == raw, 'original_attempt_receipt_not_preserved')


def prepare_resume(t, args):
    fixture_raw, fixture = pinned_json(FIXTURE, args.fixture_sha256)
    state_raw, value = pinned_json(STATE, args.state_sha256)
    report_raw, original = pinned_json(REPORT, args.report_sha256)
    validate_fixture(fixture, previous()); validate_fixture(value, previous())
    immutable = ('uid', 'employeeID', 'userID', 'username', 'name', 'mobile', 'wecom_userid', 'remark')
    require(fixture.get('runID') == value.get('runID') and all(all(a.get(k) == b.get(k) for k in immutable)
            for a,b in zip(fixture['accounts'], value['accounts'])) and fixture.get('roles') == value.get('roles'),
            'resume_fixture_state_binding_rejected')
    require(not value.get('quiesced') and not value.get('creationFailed')
            and not (HOME/'fixture-cleanup-plan.private.json').exists()
            and all(isinstance(a.get('password'), str) and len(a['password']) >= 6 and not a.get('pendingPassword')
                    and isinstance(a.get('baseRoleIDs'), list) and a['baseRoleIDs'] for a in value['accounts']),
            'resume_existing_state_not_supported')
    checks = original.get('checks', [])
    require(original.get('namespace') == NAMESPACE and original.get('zone') == args.zone
            and len(checks) == 52 and sum(c.get('passed') is True for c in checks) == 50
            and [c.get('check') for c in checks if c.get('passed') is not True]
                == ['old_im_0_rejected', 'cleanup_requires_operator_review_accounts_must_remain_disabled'],
            'resume_only_original_reviewed_failure_allowed')
    proof = load_role_proof(args.role_proof, args.role_proof_sha256, value)
    exact_department(t)
    inventory = result_data(t.admin('/admin/system/account/getList?page=1&pageSize=500'))
    hosts = rows(inventory)
    require(isinstance(inventory, dict) and type(inventory.get('total')) is int
            and len(hosts) == inventory['total'] <= 500, 'cleanup_complete_membership_inventory_required')
    isolated_role_inventory(t, value, hosts, proof)
    legacy = rows(result_data(t.admin(IM_BASE+'access/employees')))
    selections = []
    for index, account in enumerate(value['accounts']):
        host, employee = current_host(t, account), current_employee(t, account)
        require(int(host.get('status', -1)) == 1 and employee.get('status') == 'inactive',
                'resume_requires_exact_disabled_inactive_pair')
        expected_roles = set(map(int, account['baseRoleIDs'])) | ({int(r['id']) for r in value['roles']} if index == 0 else set())
        require(set(map(int, host.get('roleid', []))) == expected_roles, 'resume_fixture_membership_changed')
        policy = result_data(t.admin('/admin/system/account/accessPolicy?id='+str(account['uid'])))
        require(policy.get('matrix') == matrix('deny'), 'resume_requires_closed_personal_matrix')
        security = result_data(t.admin('/admin/system/account/security?id='+str(account['uid'])))
        require(security.get('mustChangePassword') is False, 'resume_unresolved_password_challenge')
        matches = [r for r in legacy if r.get('employeeID') == account['employeeID'] and int(r.get('uid',0)) == account['uid']]
        require(len(matches) == 1 and all(matches[0].get(k) is False for k in
                ('allowLAN','allowPC','allowMobile','allowInternet','allowInternetDownload')), 'resume_requires_closed_legacy_flags')
        selections.append((account, employee, matches[0]))
    # Recheck all pinned inputs immediately before the durable intent, then
    # preserve the exact original bytes before any fixture-enabling request.
    for path, expected in ((FIXTURE,args.fixture_sha256),(STATE,args.state_sha256),(REPORT,args.report_sha256),
                           (args.role_proof,args.role_proof_sha256)):
        pinned_json(path, expected)
    preserve_original_attempt(report_raw, args.report_sha256)
    value['resume'] = {'originalReportSHA256':args.report_sha256, 'originalStateSHA256':args.state_sha256,
                       'fixtureSHA256':args.fixture_sha256, 'roleProofSHA256':args.role_proof_sha256,
                       'startedAt':int(time.time())}
    save(STATE, value)
    return value, proof, selections


def restore_existing_pair(t, value, selections, persist, report):
    from production_fixture_cleanup import EMPLOYEE_FIELDS
    engine = Acceptance(t, value, persist, report)
    for account, employee, legacy in selections:
        result_data(t.admin('/admin/system/account/upStatus', 'POST', {'id':account['uid'], 'status':0}))
        payload = {key:employee.get(key, '') for key in EMPLOYEE_FIELDS}
        payload.update(status='active', wecom_userid='')
        result_data(t.admin('/plugins/biz-suite/api/admin/hrm/employees', 'POST', payload))
        result_data(t.admin(IM_BASE+'access/employee', 'POST', {'employeeID':account['employeeID'],
            'expectedRevision':legacy['revision'], 'allowLAN':True, 'allowPC':True, 'allowMobile':True,
            'allowInternet':True, 'allowInternetDownload':True}))
        engine.set_person(account, matrix('allow'))
        result_data(t.admin('/admin/system/account/unlock', 'POST', {'id':account['uid']}))
    wait_login_ready(t, value)
    report('resume_exact_existing_pair_without_bootstrap_or_creation', True)


def validate_retired_roles(proof, value, fixture_sha):
    require(proof.get('namespace') == NAMESPACE and proof.get('instanceID') == INSTANCE
            and proof.get('runID') == value.get('runID') and proof.get('fixtureSourceSHA256') == fixture_sha
            and sorted(proof.get('retiredRoleIDs', [])) == sorted(r['id'] for r in value['roles'])
            and type(proof.get('retiredRoleRows')) is int and proof['retiredRoleRows'] == 0
            and type(proof.get('retiredMembershipRows')) is int and proof['retiredMembershipRows'] == 0,
            'remaining_retired_role_proof_rejected')


def prepare_remaining(t, args):
    paths = ((FIXTURE,args.fixture_sha256),(STATE,args.state_sha256),(REPORT,args.report_sha256),
             (HOME/'fixture-cleanup-plan.private.json',args.cleanup_plan_sha256),
             (HOME/'fixture-cleanup-progress.private.json',args.cleanup_progress_sha256))
    snapshots = [pinned_json(p,h) for p,h in paths]
    fixture,value,original,plan,progress = [v for _,v in snapshots]
    validate_fixture(fixture, previous()); validate_fixture(value, previous())
    immutable = ('uid','employeeID','userID','username','name','mobile','wecom_userid','remark')
    require(fixture['runID'] == value['runID'] and fixture.get('roles') == value.get('roles')
            and all(all(a.get(k)==b.get(k) for k in immutable) for a,b in zip(fixture['accounts'],value['accounts']))
            and value.get('quiesced') is True and not value.get('creationFailed')
            and all(isinstance(a.get('password'),str) and len(a['password'])>=6 and not a.get('pendingPassword')
                    and isinstance(a.get('baseRoleIDs'),list) and a['baseRoleIDs'] for a in value['accounts']),
            'remaining_exact_quiesced_pair_required')
    checks = original.get('checks', [])
    names = [c.get('check') for c in checks]
    require(original.get('namespace') == NAMESPACE and original.get('zone') == args.zone
            and len(checks) == 53 and all(c.get('passed') is True for c in checks)
            and 'self_change_no_new_session' in names and names[-1] == 'finally_fixture_quiesced'
            and 'admin_reset_forces_change' not in names and 'all_security_policy_state_cases_complete' not in names
            and 'before-admin-reset' not in value.get('sessions', {}), 'remaining_only_reviewed_second_attempt_allowed')
    require(plan.get('instanceID') == INSTANCE and plan.get('acceptanceNamespace') == NAMESPACE
            and plan.get('fixtureSourceSHA256') == args.fixture_sha256 and len(plan.get('fixtures',[])) == 2
            and sorted((r['expected']['uid'],r['expected']['employeeID']) for r in plan['fixtures'])
                == sorted((a['uid'],a['employeeID']) for a in value['accounts'])
            and progress.get('instanceID') == INSTANCE and progress.get('passed') is True
            and progress.get('planSHA256') == args.cleanup_plan_sha256, 'remaining_cleanup_chain_rejected')
    require(args.host_restore_proof is not None and args.host_restore_proof.resolve().is_relative_to(HOME.resolve()),
            'isolated_host_restore_proof_required')
    _, proof = pinned_json(args.host_restore_proof,args.host_restore_proof_sha256)
    require(proof.get('format') == 'puty-security-host-restore-v1'
            and type(proof.get('capturedAt')) is int and 0 <= time.time()-proof['capturedAt'] <= 1800
            and proof.get('stateSourceSHA256') == args.state_sha256 and proof.get('reportSourceSHA256') == args.report_sha256
            and proof.get('cleanupPlanSHA256') == args.cleanup_plan_sha256 and proof.get('cleanupProgressSHA256') == args.cleanup_progress_sha256
            and type(proof.get('restoredRows')) is int and proof['restoredRows'] == 2
            and len(proof.get('hosts',[])) == 2, 'remaining_restore_proof_chain_rejected')
    validate_retired_roles(proof,value,args.fixture_sha256)
    exact_department(t)
    host_page = result_data(t.admin('/admin/system/account/getList?page=1&pageSize=500')); hosts=rows(host_page)
    require(isinstance(host_page,dict) and type(host_page.get('total')) is int and len(hosts)==host_page['total']<=500,
            'cleanup_complete_membership_inventory_required')
    isolated_role_inventory(t,value,hosts,proof)
    legacy=rows(result_data(t.admin(IM_BASE+'access/employees'))); selections=[]
    for account in value['accounts']:
        bound=[r for r in proof['hosts'] if r.get('uid')==account['uid'] and r.get('employeeID')==account['employeeID']]
        require(len(bound)==1 and bound[0].get('username')==account['username'] and bound[0].get('remark')==account['remark']
                and type(bound[0].get('status')) is int and bound[0]['status']==1
                and all(bound[0].get(k) is True for k in ('deletetimeCleared','passwordSaltUnchanged','epochUnchanged'))
                and sorted(bound[0].get('baseRoleIDs',[]))==sorted(account['baseRoleIDs']), 'remaining_restored_host_scope_rejected')
        host,employee=current_host(t,account),current_employee(t,account)
        require(int(host.get('status',-1))==1 and employee.get('status')=='inactive'
                and set(map(int,host.get('roleid',[])))==set(map(int,account['baseRoleIDs'])), 'remaining_live_base_role_binding_rejected')
        policy=result_data(t.admin('/admin/system/account/accessPolicy?id='+str(account['uid'])))
        require(policy.get('matrix')==matrix('deny'), 'resume_requires_closed_personal_matrix')
        matches=[r for r in legacy if r.get('employeeID')==account['employeeID'] and int(r.get('uid',0))==account['uid']]
        require(len(matches)==1 and all(matches[0].get(k) is False for k in
            ('allowLAN','allowPC','allowMobile','allowInternet','allowInternetDownload')), 'resume_requires_closed_legacy_flags')
        selections.append((account,employee,matches[0]))
    for path,expected in (*paths,(args.host_restore_proof,args.host_restore_proof_sha256)):
        pinned_json(path,expected)
    for (raw,_),(_,expected),prefix in zip(snapshots,paths,
            ('original-fixture','original-state','original-attempt','original-cleanup-plan','original-cleanup-progress')):
        if prefix!='original-fixture':preserve_original_attempt(raw,expected,prefix)
    value['resumeRemaining']={'previousReportSHA256':args.report_sha256,'previousStateSHA256':args.state_sha256,
        'cleanupPlanSHA256':args.cleanup_plan_sha256,'cleanupProgressSHA256':args.cleanup_progress_sha256,
        'hostRestoreProofSHA256':args.host_restore_proof_sha256,'startedAt':int(time.time())}
    value['quiesced']=False; save(STATE,value)
    return value,proof,selections


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('command', nargs='?', choices=('dry-run', 'run', 'resume-existing', 'resume-remaining', 'quiesce', 'verify-cleanup'), default='dry-run')
    parser.add_argument('--zone', choices=ZONES, default='lan', help='Must match server-derived actual source zone, never X-Forwarded-For spoofing')
    parser.add_argument('--sql-receipt', type=pathlib.Path, help='Private successful exact-worker stdout JSON required for after-cleanup proof')
    parser.add_argument('--operator-resource-pause', action='store_true', help='Keep two exact synthetic sessions alive for root-managed resource checks, then finally quiesce')
    for name in ('fixture-sha256', 'state-sha256', 'report-sha256', 'role-proof-sha256',
                 'cleanup-plan-sha256','cleanup-progress-sha256','host-restore-proof-sha256'):
        parser.add_argument('--'+name)
    parser.add_argument('--role-proof', type=pathlib.Path, help='Parent-produced exact hidden-role SQL SELECT proof; never writes role metadata')
    parser.add_argument('--host-restore-proof',type=pathlib.Path)
    args = parser.parse_args(argv)
    if args.command == 'dry-run':
        print(json.dumps({'namespace':NAMESPACE, 'privateDirectory':str(HOME), 'newAccounts':2, 'newIsolatedRoles':2,
                          'departmentReadOnly':DEPT, 'phases':PHASES, 'productionRequests':0,
                          'globalPolicyWrites':False, 'resourceByteStreamTested':False, 'externalVantageRequired':True}, ensure_ascii=False))
        return
    t = Transport(args.zone)
    state, role_proof = None, None
    checks = []
    def persist(value): save(STATE, value)
    def record(label, passed):
        require(re.fullmatch('[A-Za-z0-9_-]{1,100}', label) is not None, 'safe_report_label_required')
        checks.append({'check':label, 'passed':bool(passed), 'time':int(time.time())})
        save(REPORT, {'namespace':NAMESPACE, 'zone':args.zone, 'checks':checks, 'resourceByteStreamTested':False,
             'originalAttemptSHA256':state.get('resume',{}).get('originalReportSHA256') if state else None,
             'previousAttemptSHA256':state.get('resumeRemaining',{}).get('previousReportSHA256') if state else None,
             'revocationObservations':state.get('revocationObservations',[]) if state else [],
             'loginObservations':state.get('loginObservations',[]) if state else [],
             'adminObservations':getattr(t,'admin_observations',[])})
        print(json.dumps(checks[-1]), flush=True)
    if args.command == 'verify-cleanup':
        state = load(STATE); validate_fixture(state, previous())
        require(state.get('quiesced') is True, 'quiescence_required_before_cleanup_verification')
        require(args.sql_receipt is not None and args.sql_receipt.resolve().is_relative_to(HOME.resolve()), 'isolated_SQL_receipt_required')
        receipt = load(args.sql_receipt)
        require(receipt.get('passed') is True and receipt.get('employeeRowsDeleted') == 2
                and receipt.get('businessReferenceCount') == 0 and receipt.get('foreignKeyCount') == 0
                and receipt.get('planSHA256') == hashlib.sha256((HOME / 'fixture-cleanup-plan.private.json').read_bytes()).hexdigest(),
                'exact_SQL_delete_and_zero_reference_proof_required')
        hosts = rows(result_data(t.admin('/admin/system/account/getList?page=1&pageSize=500')))
        employees = rows(result_data(t.admin('/plugins/biz-suite/api/admin/hrm/employees?page=1&page_size=500')))
        require(not any(row.get('username') in ACCOUNTS for row in hosts)
                and not any(row.get('id') in {a['employeeID'] for a in state['accounts']} for row in employees), 'exact_physical_cleanup_still_pending')
        wait_projection(t, state, False)
        record('after_SQL_two_employee_rows_absent_and_disabled_projections_preserved', True)
        return
    if args.command == 'quiesce':
        state = load(STATE)
        if args.role_proof is not None:
            role_proof = load_role_proof(args.role_proof, args.role_proof_sha256, state)
        elif args.host_restore_proof is not None:
            require(args.host_restore_proof.resolve().is_relative_to(HOME.resolve()),'isolated_host_restore_proof_required')
            _,role_proof=pinned_json(args.host_restore_proof,args.host_restore_proof_sha256)
            require(role_proof.get('format')=='puty-security-host-restore-v1','remaining_restore_proof_chain_rejected')
            validate_retired_roles(role_proof,state,hashlib.sha256(FIXTURE.read_bytes()).hexdigest())
        print(json.dumps(quiesce(t, state, persist, record, role_proof)))
        return
    selections = None
    if args.command == 'resume-existing':
        state, role_proof, selections = prepare_resume(t, args)
    elif args.command == 'resume-remaining':
        state, role_proof, selections = prepare_remaining(t,args)
    else:
        require(not STATE.exists() and not FIXTURE.exists(), 'existing_namespace_never_recreate_or_resume_blindly')
    failure = None
    try:
        preflight(t)
        if selections is None:
            state = create(t, persist)
        else:
            restore_existing_pair(t, state, selections, persist, record)
        engine = Acceptance(t, state, persist, record)
        if selections is None:
            engine.bootstrap(); wait_projection(t, state); assign_roles(t, state, persist)
        control_epoch = engine.security(state['accounts'][1])['securityVersion']
        control = engine.login(state['accounts'][1], 'untouched-control-J')
        if args.command == 'resume-remaining':
            engine.admin_reset_remaining()
        else:
            engine.roles(); engine.lock(); engine.password()
        engine.clients()
        control_state = result_data(t.public('/enterprise/v1/session/state', method='GET', session=control))
        engine.check('other_fixture_session_and_epoch_unchanged', engine.security(state['accounts'][1])['securityVersion'] == control_epoch
                     and control_state.get('epoch') == control_epoch and control_state.get('loginAllowed') is True)
        result_data(t.public('/sso/logout', {'sessionToken':control['sessionToken']}))
        record('all_security_policy_state_cases_complete', True)
        if args.operator_resource_pause:
            personal = matrix('allow')
            for zone in ZONES: personal[zone]['web_desktop']['save'] = 'deny'
            engine.set_person(state['accounts'][0], personal)
            engine.login(state['accounts'][0], 'resource-preview-web', 'web_desktop')
            engine.login(state['accounts'][0], 'resource-save-pc', 'pc')
            print(json.dumps({'rootResourceChecksReady':True, 'privateState':str(STATE),
                              'previewSlot':'resource-preview-web', 'saveSlot':'resource-save-pc',
                              'resourceByteStreamTestedByThisScript':False}), flush=True)
            input('Press Enter after the separate root-operated resource checks and exact object cleanup: ')
    except BaseException as error:
        failure = error
    finally:
        if STATE.exists():
            try:
                state = load(STATE)
                cleanup = quiesce(t, state, persist, record, role_proof) if role_proof is not None else quiesce(t, state, persist, record)
                print(json.dumps({'namespace':NAMESPACE, **cleanup, 'cleanupPlan':str(HOME / 'fixture-cleanup-plan.private.json'),
                                  'physicalDeleteNotExecuted':True}), flush=True)
            except BaseException:
                record('cleanup_requires_operator_review_accounts_must_remain_disabled', False)
                if failure is None: failure = Rejected('cleanup_requires_operator_review')
    if failure is not None:
        raise Rejected(str(failure) if isinstance(failure, Rejected) else 'security_acceptance_failed_safe_details_withheld')


if __name__ == '__main__':
    try:
        main()
    except BaseException as error:
        code = str(error) if isinstance(error, Rejected) else 'acceptance_stopped_safe_details_withheld'
        print(json.dumps({'passed':False, 'code':code}), flush=True)
        raise SystemExit(1)
