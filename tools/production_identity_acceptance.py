"""Production IM acceptance using only the two private synthetic HRM fixtures.

Never print credentials, session tokens, signed attachment URLs, or raw errors.
Captcha answers are read normally by the operator; no captcha bypass is used.
"""
import argparse
import base64
import hashlib
import json
import pathlib
import secrets
import ssl
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

import websocket
import production_hrm_acceptance as hrm

ROOT = pathlib.Path(__file__).resolve().parents[1]
PRIVATE = ROOT / '.private/production/acceptance'
FIXTURE = PRIVATE / 'test-accounts.json'
STATE = PRIVATE / 'identity-state.json'
REPORT = PRIVATE / 'identity-report.json'
CA = ROOT / 'deploy/certificates/Puty-Enterprise-Root-CA.crt'
CTX = ssl.create_default_context(cafile=str(CA))
SLOTS = {'aweb': (0, 'web'), 'bweb': (1, 'web'), 'aandroid': (0, 'android'), 'awindows': (0, 'windows'), 'replacement': (0, 'web'), 'fresh': (1, 'web')}
PLATFORMS = {'web': 5, 'windows': 3, 'android': 2}


class CheckFailure(Exception):
    pass


def read(path, default=None):
    return json.loads(path.read_text(encoding='utf-8-sig')) if path.exists() else default


def save(path, value):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2), encoding='utf-8')


def record(name, passed, **safe):
    rows = read(REPORT, [])
    rows.append({'check': name, 'passed': bool(passed), 'time': int(time.time()), **safe})
    save(REPORT, rows)
    print(json.dumps(rows[-1], ensure_ascii=False), flush=True)
    if not passed:
        raise CheckFailure(name)


def setup():
    fixture = read(FIXTURE)
    if not fixture or len(fixture.get('accounts', [])) != 2:
        raise CheckFailure('exactly_two_private_fixtures_required')
    if len({str(v['uid']) for v in fixture['accounts']}) != 2:
        raise CheckFailure('distinct_fixture_UIDs_required')
    return fixture


def origin():
    return setup()['lanOrigin'].rstrip('/')


def http(url, method='GET', value=None, session=None, headers=None, raw=None):
    h = {'User-Agent': 'PutyProductionAcceptance/3.8', 'operationID': secrets.token_hex(16), **(headers or {})}
    if session:
        h.update(token=session['imToken'], chatToken=session['chatToken'], platformID=str(session['_platform']))
        if urllib.parse.urlsplit(url).path.startswith('/chat/'):
	        h['token'] = session['chatToken']
    if value is not None:
        h['Content-Type'] = 'application/json'
        raw = json.dumps(value, ensure_ascii=False).encode()
    req = urllib.request.Request(url, data=raw, headers=h, method=method)
    try:
        response = urllib.request.urlopen(req, context=CTX, timeout=30)
    except urllib.error.HTTPError as error:
        response = error
    with response:
        body = response.read()
        try:
            payload = json.loads(body) if body else {}
        except (ValueError, UnicodeDecodeError):
            payload = None
        return response.status, payload, body, dict(response.headers)


def call(path, value=None, session=None, method='POST'):
    return http(origin()+path, method, value, session)


def ok(result):
    status, body = result[:2]
    return 200 <= status < 300 and isinstance(body, dict) and body.get('errCode', body.get('code', -1)) == 0


def expect(name, result, allowed=True):
    status, body = result[:2]
    code = body.get('errCode', body.get('code')) if isinstance(body, dict) else None
    record(name, ok(result) == allowed, status=status, code=code)
    return body.get('data') if isinstance(body, dict) else None


def captchas(slots):
    setup()
    for slot in slots:
        if slot not in SLOTS:
            raise CheckFailure('unknown_captcha_slot')
        result = call('/sso/captcha', method='GET')
        data = expect('captcha_'+slot, result)
        save(PRIVATE / ('identity-captcha-'+slot+'.json'), data)
        image = data.get('img', data.get('image', ''))
        if image:
            (PRIVATE / ('identity-captcha-'+slot+'.png')).write_bytes(base64.b64decode(image.split(',')[-1]))
        print(json.dumps({'captchaSlot': slot, 'image': str(PRIVATE / ('identity-captcha-'+slot+'.png')), 'enabled': bool(data.get('show', data.get('enabled')))}), flush=True)


def login(slot, answers, state_slot=None):
    index, platform = SLOTS[slot]
    account = setup()['accounts'][index]
    captcha = read(PRIVATE / ('identity-captcha-'+slot+'.json'), {})
    if captcha.get('show', captcha.get('enabled')) and not answers.get(slot):
        raise CheckFailure('captcha_answer_required_'+slot)
    result = call('/sso/login', {'account': account['username'], 'password': hashlib.md5(account['password'].encode()).hexdigest(), 'platform': platform, 'captcha': answers.get(slot, ''), 'captchaId': captcha.get('captchaId', captcha.get('id', ''))})
    data = expect('login_'+slot, result)
    record('login_identity_'+slot, bool(data and data.get('userID', '').startswith('emp_') and all(data.get(key) for key in ('imToken', 'chatToken', 'sessionToken'))))
    data.update(_platform=PLATFORMS[platform], _fixture=index)
    state = read(STATE, {})
    state[state_slot or slot] = data
    save(STATE, state)
    return data


def live(session):
    return call('/enterprise/v1/discovery/list', {}, session)


def native(session):
    return call('/api/auth/parse_token', {'token': session['imToken']})


def wait_denied(name, session, timeout=60):
    start = time.monotonic()
    while time.monotonic()-start < timeout:
        if not ok(native(session)):
            record(name, True, seconds=round(time.monotonic()-start, 2))
            return
        time.sleep(1)
    record(name, False, seconds=timeout)


def socket_for(session):
    endpoint = origin().replace('https://', 'wss://', 1)+'/ws?'+urllib.parse.urlencode({'sendID': session['userID'], 'token': session['imToken'], 'platformID': session['_platform'], 'operationID': secrets.token_hex(16), 'sdkType': 'js', 'isMsgResp': 'true', 'isBackground': 'false'})
    ws = websocket.create_connection(endpoint, timeout=10, sslopt={'ca_certs': str(CA), 'cert_reqs': ssl.CERT_REQUIRED, 'check_hostname': True}, http_no_proxy=[urllib.parse.urlsplit(origin()).hostname])
    frame = ws.recv()
    data = json.loads(frame)
    record('WSS_authenticated', data.get('errCode') == 0)
    ws.settimeout(1)
    return ws


def wait_socket_closed(name, ws, timeout=60):
    start = time.monotonic()
    while time.monotonic()-start < timeout:
        try:
            value = ws.recv()
            if value == '' or value == b'':
                record(name, True, seconds=round(time.monotonic()-start, 2))
                return
        except websocket.WebSocketTimeoutException:
            continue
        except (websocket.WebSocketConnectionClosedException, ConnectionError, OSError):
            record(name, True, seconds=round(time.monotonic()-start, 2))
            return
    record(name, False, seconds=timeout)


def upload(session):
    content = (b'PUTY production identity acceptance fixture\n'+secrets.token_hex(32).encode()+b'\n')*4
    data = expect('upload_initiate_bound_to_SID', call('/api/object/initiate_form_data', {'name': session['userID']+'/production-acceptance-'+secrets.token_hex(12)+'.txt', 'size': len(content), 'contentType': 'text/plain', 'group': 'acceptance'}, session))
    record('upload_uses_authorized_proxy', urllib.parse.urlsplit(data['url']).netloc == urllib.parse.urlsplit(origin()).netloc and '/attachments/' in data['url'])
    boundary = 'PutyAcceptance'+secrets.token_hex(16)
    chunks = []
    for key, value in data.get('formData', {}).items():
        chunks.append(('--'+boundary+'\r\nContent-Disposition: form-data; name="'+key+'"\r\n\r\n'+value+'\r\n').encode())
    chunks += [('--'+boundary+'\r\nContent-Disposition: form-data; name="'+data.get('file', 'file')+'"; filename="acceptance.txt"\r\nContent-Type: text/plain\r\n\r\n').encode(), content, ('\r\n--'+boundary+'--\r\n').encode()]
    headers = {key: ','.join(value.get('values', [])) if isinstance(value, dict) else str(value) for key, value in (data.get('header') or {}).items()}
    headers['Content-Type'] = 'multipart/form-data; boundary='+boundary
    result = http(data['url'], 'POST', headers=headers, raw=b''.join(chunks))
    record('signed_form_upload', result[0] in data.get('successCodes', [200, 201, 204]), status=result[0])
    complete = expect('upload_complete_current_SID', call('/api/object/complete_form_data', {'id': data['id']}, session))
    grant = expect('attachment_authorize', call('/enterprise/v1/attachment/authorize', {'url': complete['url']}, session))
    verify_attachment(grant, content)
    state = read(STATE, {})
    state['_attachment'] = {'grant': grant, 'stableURL': complete['url'], 'contentBase64': base64.b64encode(content).decode(), 'uploadID': data['id']}
    save(STATE, state)
    return grant, content


def verify_attachment(grant, content):
    result = http(grant['url'])
    record('attachment_GET', result[0] == 200 and result[2] == content, status=result[0])
    result = http(grant['url'], 'HEAD')
    record('attachment_HEAD', result[0] == 200 and result[2] == b'' and int(result[3].get('Content-Length', 0)) == len(content), status=result[0])
    result = http(grant['url'], headers={'Range': 'bytes=2-9'})
    record('attachment_Range', result[0] == 206 and result[2] == content[2:10], status=result[0])
    path = urllib.parse.urlsplit(grant['url']).path.split('/', 3)[3]
    result = http(origin()+'/'+path)
    record('bare_S3_rejected', result[0] in (401, 403, 404), status=result[0])


def wire_int(value):
    result = bytearray()
    while value > 127:
        result.append((value & 127) | 128)
        value >>= 7
    return bytes(result)+bytes([value])


def wire_field(number, value):
    if isinstance(value, int):
        return wire_int(number << 3)+wire_int(value)
    raw = value.encode() if isinstance(value, str) else value
    return wire_int((number << 3) | 2)+wire_int(len(raw))+raw


def ws_send(ws, session, receiver, marker, group=False, allowed=True):
    identifier = secrets.token_hex(16)
    fields = {1: session['userID'], 2: receiver, 4: identifier, 6: session['_platform'], 7: 'Production acceptance fixture', 9: 3 if group else 1, 10: 100, 11: 101, 12: json.dumps({'content': marker}).encode(), 15: int(time.time()*1000), 16: int(time.time()*1000), 17: 1}
    if group:
        fields[3] = receiver
    message = b''.join(wire_field(number, value) for number, value in fields.items())
    for name in ('history', 'persistent', 'senderSync', 'conversationUpdate', 'unreadCount'):
        message += wire_field(19, wire_field(1, name)+wire_field(2, 1))
    ws.send(json.dumps({'reqIdentifier': 1003, 'sendID': session['userID'], 'operationID': secrets.token_hex(16), 'msgIncr': identifier, 'data': base64.b64encode(message).decode()}).encode(), opcode=websocket.ABNF.OPCODE_BINARY)
    deadline = time.monotonic()+20
    while time.monotonic() < deadline:
        try:
            result = json.loads(ws.recv())
        except websocket.WebSocketTimeoutException:
            continue
        if result.get('msgIncr') == identifier:
            record(('group' if group else 'single')+'_real_WSS_send'+('_muted_denial' if not allowed else ''), (result.get('errCode') == 0) == allowed, code=result.get('errCode'))
            return
    record('WSS_message_response_within_deadline', False)


def receive_marker(ws, marker, label):
    deadline = time.monotonic()+20
    while time.monotonic() < deadline:
        try:
            result = json.loads(ws.recv())
        except websocket.WebSocketTimeoutException:
            continue
        if result.get('reqIdentifier') == 2001 and marker.encode() in base64.b64decode(result.get('data', '')):
            record(label+'_real_recipient_push', True)
            return
    record(label+'_real_recipient_push', False)


def history(session, conversation, marker, label):
    deadline = time.monotonic()+25
    while time.monotonic() < deadline:
        result = call('/api/msg/newest_seq', {'userID': session['userID']}, session)
        if ok(result):
            maximum = int(result[1]['data'].get('maxSeqs', {}).get(conversation, 0))
            if maximum:
                result = call('/api/msg/pull_msg_by_seq', {'userID': session['userID'], 'seqRanges': [{'conversationID': conversation, 'begin': max(1, maximum-20), 'end': maximum, 'num': 20}], 'order': 0}, session)
                if ok(result):
                    # HTTP protobuf JSON encodes message content as base64.
                    serialized = json.dumps(result[1]['data'])
                    if marker in serialized or base64.b64encode(json.dumps({'content': marker}).encode()).decode() in serialized:
                        record(label+'_native_history_persisted', True)
                        return
                    def contains(value):
                        if isinstance(value, dict):
                            return any(contains(child) for child in value.values())
                        if isinstance(value, list):
                            return any(contains(child) for child in value)
                        if isinstance(value, str):
                            try:
                                return marker.encode() in base64.b64decode(value, validate=True)
                            except ValueError:
                                return False
                        return False
                    if contains(result[1]['data']):
                        record(label+'_native_history_persisted', True)
                        return
        time.sleep(1)
    record(label+'_native_history_persisted', False)


def manage(operation, payload):
    return admin('/plugins/biz-suite/api/admin/im/manager/openim/operations/'+operation, payload)


def chat_core(a, b):
    friends = manage('friends/search', {'userID': a['userID'], 'pageNumber': 1, 'showNumber': 100})
    if b['userID'] not in json.dumps(friends):
        manage('friends/add', {'ownerUserID': a['userID'], 'friendUserID': b['userID']})
    wa, wb = socket_for(a), socket_for(b)
    group_id = None
    try:
        rows = manage('users/online', {'userIDs': [a['userID'], b['userID']]})
        def safe_online(value):
            if not isinstance(value, list):
                return False
            return all(isinstance(row, dict) and set(row) <= {'userID', 'status', 'detailPlatformStatus'} and all(isinstance(detail, dict) and set(detail) <= {'platformID', 'isBackground'} for detail in row.get('detailPlatformStatus', [])) for row in value)
        record('BFF_online_never_exposes_connection_credentials', safe_online(rows))
        expect('native_online_admin_only', call('/api/user/get_users_online_status', {'userIDs': [b['userID']]}, a), False)
        sub_id = secrets.token_hex(16)
        wa.send(json.dumps({'reqIdentifier': 2005, 'sendID': a['userID'], 'operationID': secrets.token_hex(16), 'msgIncr': sub_id, 'data': base64.b64encode(wire_field(1, b['userID'])).decode()}).encode(), opcode=websocket.ABNF.OPCODE_BINARY)
        subscription = json.loads(wa.recv())
        raw = base64.b64decode(subscription.get('data', ''))
        record('WSS_online_subscription_no_credentials', subscription.get('errCode') == 0 and subscription.get('msgIncr') == sub_id and all(value.encode() not in raw for pair in (a, b) for value in (pair['imToken'], pair['chatToken'])))
        # Real SDK login performs its initial history synchronization before
        # sending. Allow the asynchronous online-presence subscription to settle.
        time.sleep(1)
        marker = 'PUTY_ACCEPTANCE_SINGLE_'+secrets.token_hex(12)
        ws_send(wa, a, b['userID'], marker)
        receive_marker(wb, marker, 'single_chat')
        conversation = 'si_'+'_'.join(sorted([a['userID'], b['userID']]))
        history(b, conversation, marker, 'single_chat')
        reply = 'PUTY_ACCEPTANCE_REPLY_'+secrets.token_hex(12)
        ws_send(wb, b, a['userID'], reply)
        receive_marker(wa, reply, 'single_reply')
        created = manage('groups/create', {'groupName': 'PUTY_ACCEPTANCE_'+secrets.token_hex(8), 'ownerUserID': a['userID'], 'memberUserIDs': [b['userID']], 'groupType': 2})
        group_id = created.get('groupInfo', created).get('groupID')
        record('BFF_created_fixture_group', bool(group_id and not group_id.startswith('dept_')))
        members = manage('groups/members', {'groupID': group_id, 'pageNumber': 1, 'showNumber': 50})
        encoded = json.dumps(members)
        record('BFF_group_members_preserved', a['userID'] in encoded and b['userID'] in encoded)
        notice = 'PUTY_ACCEPTANCE_NOTICE_'+secrets.token_hex(8)
        manage('groups/notice', {'groupID': group_id, 'notice': notice})
        info = expect('native_group_info_after_BFF_notice', call('/api/group/get_groups_info', {'groupIDs': [group_id]}, a))
        record('BFF_notice_persisted', notice in json.dumps(info))
        manage('groups/mute', {'groupID': group_id, 'muted': True})
        try:
            ws_send(wb, b, group_id, 'PUTY_ACCEPTANCE_MUTED_DENIAL', group=True, allowed=False)
        finally:
            manage('groups/mute', {'groupID': group_id, 'muted': False})
        marker = 'PUTY_ACCEPTANCE_GROUP_'+secrets.token_hex(12)
        ws_send(wa, a, group_id, marker, group=True)
        receive_marker(wb, marker, 'group_chat')
        history(b, 'sg_'+group_id, marker, 'group_chat')
        deadline = time.monotonic()+25
        while time.monotonic() < deadline:
            rows = manage('messages/search', {'sendID': a['userID'], 'groupID': group_id, 'keyword': marker, 'pageNumber': 1, 'showNumber': 50})
            if rows.get('items'):
                row = rows['items'][0]
                record('BFF_message_query_filters_and_history', marker in json.dumps(rows['items']) and int(rows['total']) >= 1 and row.get('sendID') == a['userID'] and row.get('groupID') == group_id and int(row.get('seq') or 0) > 0 and bool(row.get('conversationID')))
                break
            time.sleep(1)
        else:
            record('BFF_message_query_filters_and_history', False)
        record('core_single_group_and_management_complete', True)
    finally:
        wa.close()
        wb.close()
        if group_id:
            manage('groups/dismiss', {'groupID': group_id})


def run(answers, resume=False):
    record('public_acceptance_started', True)
    if resume:
        state = read(STATE, {})
        a, b, android = state['aweb'], state['bweb'], state['aandroid']
    else:
        a, b, android = login('aweb', answers), login('bweb', answers), login('aandroid', answers)
    for slot, value in [('aweb', a), ('bweb', b), ('aandroid', android)]:
        expect('dual_token_'+slot, live(value))
    expect('cross_user_pair_rejected', live({**a, 'chatToken': b['chatToken']}), False)
    expect('cross_platform_pair_rejected', live({**a, 'chatToken': android['chatToken']}), False)
    expect('Chat_direct_profile_edit_rejected', call('/chat/user/update', {'userID': a['userID'], 'nickname': 'forbidden-fixture-edit'}, a), False)
    expect('IM_direct_profile_edit_rejected', call('/api/user/update_user_info_ex', {'userInfo': {'userID': a['userID'], 'nickname': 'forbidden-fixture-edit'}}, a), False)
    for path in ('/chat/account/login', '/chat/account/register', '/chat/account/reset_password', '/enterprise/v1/admin/login'):
        expect('independent_entry_rejected_'+path, call(path, {}), False)
    chat_core(a, b)
    grant, content = upload(a)
    ws = socket_for(a)
    try:
        fresh = expect('SSO_refresh', call('/sso/refresh', {'sessionToken': a['sessionToken']}))
        fresh.update(_platform=5, _fixture=0)
        record('refresh_same_session_new_pair', fresh['sessionToken'] == a['sessionToken'] and fresh['imToken'] != a['imToken'] and fresh['chatToken'] != a['chatToken'])
        expect('old_generation_same_SID_retained', live(a))
        expect('new_generation_same_SID_live', live(fresh))
        record('attachment_survives_same_SID_refresh', http(grant['url'])[0] == 200)
        replacement = login('replacement', answers)
        wait_denied('same_platform_old_generation_invalid', a)
        wait_denied('same_platform_refreshed_generation_invalid', fresh)
        expect('other_platform_survives_replacement', live(android))
        wait_socket_closed('replaced_WSS_closed_within_60s', ws)
        record('replaced_attachment_rejected', http(grant['url'])[0] in (401, 403))
        expect('old_upload_completion_rejected_by_new_SID', call('/api/object/complete_form_data', {'id': read(STATE)['_attachment']['uploadID']}, replacement), False)
        stale = setup()['internetOrigin'].rstrip('/')+urllib.parse.urlsplit(grant['url']).path+'?'+urllib.parse.urlsplit(grant['url']).query
        new_grant = expect('old_grant_reauthorized_at_current_entry', call('/enterprise/v1/attachment/authorize', {'url': stale}, replacement))
        record('stable_objectID_and_current_origin', new_grant['objectID'] == grant['objectID'] and new_grant['url'].startswith(origin()+'/attachments/'))
        verify_attachment(new_grant, content)
        ws2 = socket_for(replacement)
        try:
            expect('SSO_logout', call('/sso/logout', {'sessionToken': replacement['sessionToken']}))
            wait_denied('logout_invalidates_native_token', replacement)
            expect('logout_invalidates_enterprise_pair', live(replacement), False)
            record('logout_invalidates_issued_attachment', http(new_grant['url'])[0] in (401, 403))
            wait_socket_closed('logout_WSS_closed_within_60s', ws2)
        finally:
            ws2.close()
    finally:
        ws.close()
    expect('other_user_survives_all_tests', live(b))
    expect('android_logout', call('/sso/logout', {'sessionToken': android['sessionToken']}))
    record('public_acceptance_complete_bweb_retained_for_lifecycle', True)


def admin(path, value=None, method='POST'):
    status, body = hrm.request(path, method, value)
    result = (status, body)
    expect('HRM_admin_'+urllib.parse.urlsplit(path).path.rsplit('/', 1)[-1], result)
    return body['data']


def lifecycle(kind, answers):
    fixture = setup()['accounts'][1]
    old = read(STATE, {})['bweb']
    record('lifecycle_uses_only_second_fixture', old['_fixture'] == 1)
    ws = socket_for(old)
    restore = None
    try:
        if kind == 'policy':
            path = '/plugins/biz-suite/api/admin/im/manager/openim/access/'
            rows = admin(path+'employees', method='GET')['items']
            selected = [v for v in rows if v['employeeID'] == fixture['employeeID'] and int(v['uid']) == int(fixture['uid'])]
            record('fixture_policy_exact_UID_match', len(selected) == 1)
            original = selected[0]
            keys = ('employeeID', 'allowLAN', 'allowPC', 'allowMobile', 'allowInternet', 'allowInternetDownload')
            def set_policy(deny):
                current = next(v for v in admin(path+'employees', method='GET')['items'] if v['employeeID'] == fixture['employeeID'])
                payload = {key: original[key] for key in keys}
                payload.update(expectedRevision=current['revision'])
                if deny:
                    payload['allowPC'] = False
                return admin(path+'employee', payload)
            restore = lambda: set_policy(False)
            set_policy(True)
        elif kind == 'status':
            restore = lambda: admin('/admin/system/account/upStatus', {'id': int(fixture['uid']), 'status': 0})
            admin('/admin/system/account/upStatus', {'id': int(fixture['uid']), 'status': 1})
        elif kind == 'password':
            query = urllib.parse.urlencode({'name': fixture['name'], 'page': 1, 'pageSize': 100})
            rows = admin('/admin/system/account/getList?'+query, method='GET')['items']
            selected = [v for v in rows if int(v['id']) == int(fixture['uid']) and v['username'] == fixture['username']]
            record('fixture_password_exact_UID_match', len(selected) == 1)
            current = selected[0]
            # The list response's role label is not the save DTO's role ID
            # array. Omit it so the existing role assignment is preserved.
            base = {key: current[key] for key in ('name', 'username', 'nickname', 'dept_id', 'avatar', 'mobile', 'tel', 'email', 'remark', 'weigh', 'account_id') if key in current}
            for key in ('mobile', 'tel', 'email'):
                base[key] = current.get(key) or ''
            base['id'] = int(fixture['uid'])
            def set_password(raw):
                return admin('/admin/system/account/save', {**base, 'password': hashlib.md5(raw.encode()).hexdigest()})
            restore = lambda: set_password(fixture['password'])
            set_password(secrets.token_urlsafe(24))
        elif kind in ('employee_inactive', 'employee_resigned'):
            path = '/plugins/biz-suite/api/admin/hrm/employees'
            data = admin(path, method='GET')
            rows = data.get('items', data.get('rows', [])) if isinstance(data, dict) else data
            selected = [value for value in rows if str(value['id']) == str(fixture['employeeID']) and int(value['user_id']) == int(fixture['uid'])]
            record('fixture_employee_exact_UID_and_employeeID_match', len(selected) == 1 and not selected[0].get('wecom_userid'))
            current = selected[0]
            base = {key: current.get(key) or '' for key in ('id', 'employee_no', 'name', 'mobile', 'email', 'position', 'entry_date', 'quit_date', 'status')}
            base.update(user_id=int(fixture['uid']), dept_id=int(current.get('dept_id') or 0), wecom_userid='')
            for date in ('entry_date', 'quit_date'):
                base[date] = str(base[date])[:10]
            restore = lambda: admin(path, base)
            changed = {**base, 'status': 'inactive' if kind == 'employee_inactive' else 'resigned', 'quit_date': time.strftime('%Y-%m-%d') if kind == 'employee_resigned' else ''}
            admin(path, changed)
        else:
            raise CheckFailure('unknown_lifecycle_kind')
        wait_denied(kind+'_revokes_all_native_generations', old)
        expect(kind+'_revokes_enterprise_pair', live(old), False)
        wait_socket_closed(kind+'_closes_WSS_within_60s', ws)
    finally:
        ws.close()
        if restore:
            restore()
    if not answers.get('fresh'):
        record(kind+'_restored_awaiting_new_captcha_recovery', True)
        return
    recover(kind, answers, old)


def recover(kind, answers, old=None):
    old = old or read(STATE, {})['bweb']
    fresh = login('fresh', answers, 'bweb')
    expect(kind+'_recovery_allows_new_session', live(fresh))
    record(kind+'_old_credentials_stay_invalid_after_restore', not ok(native(old)))


def server_input(slot):
    value = read(STATE, {})[slot]
    output = PRIVATE / 'identity-server-input.json'
    save(output, {'sessions': [{'slot': slot, 'userID': value['userID'], 'imToken': value['imToken']}]})
    print(json.dumps({'serverInput': str(output), 'sessionCount': 1, 'containsPrivateCredentials': True}), flush=True)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('action', choices=['captchas', 'run', 'lifecycle', 'recover', 'server-input', 'cleanup'])
    parser.add_argument('--slots', default='aweb,bweb,aandroid,replacement')
    parser.add_argument('--captcha', action='append', default=[], help='slot=normal captcha answer')
    parser.add_argument('--kind', choices=['policy', 'status', 'password', 'employee_inactive', 'employee_resigned'], default='policy')
    parser.add_argument('--slot', default='bweb')
    parser.add_argument('--resume', action='store_true')
    args = parser.parse_args()
    answers = dict(value.split('=', 1) for value in args.captcha)
    setup()
    if args.action == 'captchas':
        captchas(args.slots.split(','))
    elif args.action == 'run':
        run(answers, args.resume)
    elif args.action == 'lifecycle':
        lifecycle(args.kind, answers)
    elif args.action == 'recover':
        recover(args.kind, answers)
    elif args.action == 'server-input':
        server_input(args.slot)
    else:
        for value in read(STATE, {}).values():
            if isinstance(value, dict) and value.get('sessionToken'):
                call('/sso/logout', {'sessionToken': value['sessionToken']})
        record('synthetic_session_cleanup_complete', True)


if __name__ == '__main__':
    try:
        main()
    except Exception as error:
        # Raw network exceptions may include a query string containing a token.
        print(json.dumps({'acceptanceFailed': True, 'errorType': type(error).__name__, 'check': str(error) if isinstance(error, CheckFailure) else 'private_details_suppressed'}), flush=True)
        sys.exit(1)
