"""Scoped synthetic fixtures; default C/D behavior and old evidence stay intact.

The explicit attachment-20261103 profile creates E/F in read-only protected
department 53. It never runs department/group/message acceptance operations.
"""
import argparse
import hashlib
import json
import pathlib
import secrets
import sys
import time

ROOT = pathlib.Path(__file__).resolve().parents[1]
PRIVATE = ROOT / '.private/production/acceptance'
FIXTURE = PRIVATE / 'organization-test-accounts.json'
STATE = PRIVATE / 'organization-identity-state.json'
REPORT = PRIVATE / 'organization-acceptance-report.json'
EVIDENCE = PRIVATE
NAMESPACE = 'organization-20261103'
PAIR = [('c', '00000000003'), ('d', '00000000004')]
PREFIX = 'qa_261103_'
BASE = '/plugins/biz-suite/api/admin/im/manager/openim/'


def hrm_client():
    # Offline imports/tests never load private HRM authentication material.
    import production_hrm_acceptance
    return production_hrm_acceptance


def protect(path):
    from production_backup import protect as secure
    secure(path)


def configure_scope(namespace):
    global NAMESPACE, FIXTURE, STATE, REPORT, EVIDENCE, PAIR
    assert namespace in ('organization-20261103', 'attachment-20261103'), 'Unsupported explicit fixture scope'
    NAMESPACE = namespace
    if namespace == 'attachment-20261103':
        EVIDENCE = PRIVATE / namespace
        FIXTURE, STATE, REPORT = (EVIDENCE / name for name in ('test-accounts.json', 'identity-state.json', 'acceptance-report.json'))
        PAIR = [('e', '00000000005'), ('f', '00000000006')]
    else:
        EVIDENCE = PRIVATE
        FIXTURE, STATE, REPORT = (PRIVATE / name for name in ('organization-test-accounts.json', 'organization-identity-state.json', 'organization-acceptance-report.json'))
        PAIR = [('c', '00000000003'), ('d', '00000000004')]


def previous_identities():
    paths = [PRIVATE / 'test-accounts.json']
    if NAMESPACE == 'attachment-20261103':
        paths.append(PRIVATE / 'organization-test-accounts.json')
    uids, employees = set(), set()
    for path in paths:
        assert path.is_file() and not path.is_symlink() and path.resolve() == path and path.stat().st_nlink == 1
        old = json.loads(path.read_text(encoding='utf-8-sig'))
        accounts = old.get('accounts', [])
        assert len(accounts) == 2 and len({int(a['uid']) for a in accounts}) == 2
        assert len({a['employeeID'] for a in accounts}) == 2
        if path.name == 'organization-test-accounts.json':
            assert old.get('namespace') == 'organization-20261103'
            assert [a['username'] for a in accounts] == [PREFIX + 'c', PREFIX + 'd']
        assert not uids.intersection({int(a['uid']) for a in accounts})
        assert not employees.intersection({a['employeeID'] for a in accounts})
        uids.update(int(a['uid']) for a in accounts)
        employees.update(a['employeeID'] for a in accounts)
    return uids, employees


def attachment_department():
    assert NAMESPACE == 'attachment-20261103'
    found = []
    def visit(rows):
        for row in rows or []:
            if int(row.get('id', 0)) == 53:
                found.append(row)
            for key in ('children', '_children', 'child'):
                if isinstance(row.get(key), list):
                    visit(row[key])
    visit(items(api('/admin/system/dept/getList')))
    assert len(found) == 1, 'Exact protected department 53 required'
    row = found[0]
    assert row.get('name') == 'TEMP QA 20261103'
    assert row.get('remark') == 'Temporary synthetic organization acceptance department'
    assert all(int(row.get(key, -1)) == value for key, value in
               {'id': 53, 'pid': 0, 'business_id': 1, 'status': 0, 'im_group_enabled': 0, 'leader_uid': 0}.items()), 'Protected department changed; no writes allowed'
    return row


def save(path, value):
    assert path.resolve().is_relative_to(PRIVATE.resolve())
    if NAMESPACE == 'attachment-20261103':
        assert path.resolve().is_relative_to(EVIDENCE.resolve()), 'Attachment evidence must be isolated'
    path.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
    if NAMESPACE == 'attachment-20261103':
        protect(path.parent)
    assert not path.is_symlink() and (not path.exists() or path.stat().st_nlink == 1)
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    protect(path)


def api(path, method='GET', value=None):
    status, body = hrm_client().request(path, method, value)
    assert status == 200 and body.get('code', body.get('errCode')) == 0, 'Normal HRM API rejected the scoped acceptance request'
    return body.get('data')


def items(value):
    return value if isinstance(value, list) else value.get('items', [])


def fixture():
    data = json.loads(FIXTURE.read_text(encoding='utf-8'))
    assert data['namespace'] == NAMESPACE and data['departmentID'] > 0
    assert [item['username'] for item in data['accounts']] == [PREFIX + suffix for suffix, _ in PAIR]
    assert len({item['uid'] for item in data['accounts']}) == 2
    old_uids, old_employees = previous_identities()
    assert not {int(item['uid']) for item in data['accounts']} & old_uids
    assert len({item['employeeID'] for item in data['accounts']}) == 2
    assert not {item['employeeID'] for item in data['accounts']} & old_employees
    if NAMESPACE == 'attachment-20261103':
        attachment_department()
        assert data['departmentID'] == 53 and data.get('excludedPreviousUIDs') == sorted(old_uids)
        assert data.get('excludedPreviousEmployeeIDs') == sorted(old_employees)
        for account, (suffix, mobile) in zip(data['accounts'], PAIR):
            assert account['name'] == 'IM attachment acceptance ' + suffix.upper()
            assert account.get('mobile') == mobile and not account.get('wecom_userid')
            assert int(account['uid']) > 1
    return data


def create(resume_department=None):
    assert not FIXTURE.exists(), 'Preserve existing fixture state; do not recreate identities'
    old_uids, old_employees = previous_identities()
    if NAMESPACE == 'attachment-20261103':
        assert resume_department in (None, 53), 'Attachment profile only reuses protected department 53'
        attachment_department()
    hosts = items(api('/admin/system/account/getList?page=1&pageSize=500'))
    names = {PREFIX + suffix for suffix, _ in PAIR}
    assert not any((row.get('username') in names if NAMESPACE == 'attachment-20261103'
                    else str(row.get('username', '')).startswith(PREFIX)) for row in hosts), 'Synthetic username collision'
    department_name = 'TEMP QA 20261103'
    def flatten(rows):
        return [item for row in (rows or []) for item in [row, *flatten(row.get('children'))]]
    existing = [row for row in flatten(api('/admin/system/dept/getList')) if row['name'] == department_name]
    if NAMESPACE == 'attachment-20261103':
        department = 53
    elif resume_department:
        assert len(existing) == 1 and int(existing[0]['id']) == resume_department
        assert existing[0]['remark'] == 'Temporary synthetic organization acceptance department' and int(existing[0]['pid']) == 0
        department = resume_department
    else:
        assert not existing, 'Existing synthetic department requires explicit, reviewed resume ID'
        department = api('/admin/system/dept/save', 'POST', {'id': 0, 'name': department_name, 'pid': 0,
                         'weigh': 9999, 'remark': 'Temporary synthetic organization acceptance department',
                         'leader_uid': 0, 'im_group_enabled': 0})
    if isinstance(department, dict):
        department = department.get('id')
    if not department:
        matches = [row for row in flatten(api('/admin/system/dept/getList')) if row['name'] == department_name]
        assert len(matches) == 1
        department = matches[0]['id']
    data = {'namespace': NAMESPACE, 'departmentID': int(department),
            'lanOrigin': 'https://192.168.10.11:9885', 'hrmOrigin': hrm_client().ORIGIN,
            'internetOrigin': 'https://im.szputy.com:9885', 'accounts': []}
    if NAMESPACE == 'attachment-20261103':
        data.update(excludedPreviousUIDs=sorted(old_uids), excludedPreviousEmployeeIDs=sorted(old_employees))
    save(FIXTURE, data)
    for suffix, mobile in PAIR:
        account = {'username': PREFIX + suffix, 'password': secrets.token_urlsafe(24),
                   'name': 'IM ' + ('attachment' if NAMESPACE == 'attachment-20261103' else 'organization') + ' acceptance ' + suffix.upper(), 'mobile': mobile}
        data['accounts'].append(account)
        save(FIXTURE, data)
        account['uid'] = int(api('/admin/system/account/save', 'POST', {'id': 0, 'name': account['name'],
            'nickname': account['name'], 'username': account['username'],
            'password': hashlib.md5(account['password'].encode()).hexdigest(), 'roleid': [],
            'dept_id': data['departmentID'], 'mobile': mobile, 'email': '', 'tel': '',
            'remark': ('Temporary synthetic E/F attachment fixture; no external messages' if NAMESPACE == 'attachment-20261103'
                       else 'Temporary synthetic C/D acceptance fixture; no external messages')}))
        save(FIXTURE, data)
        assert account['uid'] > 1 and account['uid'] not in old_uids
        assert len({a['uid'] for a in data['accounts'] if 'uid' in a}) == len(data['accounts'])
        employee = api('/plugins/biz-suite/api/admin/hrm/employees', 'POST', {'user_id': account['uid'],
            'employee_no': account['username'], 'name': account['name'], 'status': 'active',
            'dept_id': data['departmentID'], 'mobile': mobile, 'wecom_userid': ''})
        account['employeeID'] = employee['id']
        save(FIXTURE, data)
        assert account['employeeID'] not in old_employees
        assert len({a['employeeID'] for a in data['accounts'] if 'employeeID' in a}) == len(data['accounts'])
    fixture()
    print(json.dumps({'createdSyntheticEmployees': 2, 'newDepartmentID': data['departmentID'],
                      'oldIdentitiesReused': False, 'realEmployeeRecordsModified': False}), flush=True)


def access():
    data = fixture()
    policies = items(api(BASE + 'access/employees'))
    for account in data['accounts']:
        rows = [row for row in policies if row['employeeID'] == account['employeeID'] and int(row['uid']) == account['uid']]
        assert len(rows) == 1 and rows[0]['eligible'], 'Exact eligible synthetic employee required'
        api(BASE + 'access/employee', 'POST', {'employeeID': account['employeeID'],
            'expectedRevision': rows[0]['revision'], 'allowLAN': True, 'allowPC': True,
            'allowMobile': True, 'allowInternet': False, 'allowInternetDownload': False})
    print(json.dumps({'LANOnlySyntheticAccessEnabled': 2, 'realEmployeePoliciesModified': False}), flush=True)


def identity_module():
    fixture()
    import production_identity_acceptance as identity
    if not hasattr(identity, '_organization_scope_defaults'):
        identity._organization_scope_defaults = (identity.PRIVATE, dict(identity.SLOTS), identity.save)
    identity.FIXTURE, identity.STATE, identity.REPORT = FIXTURE, STATE, REPORT
    if NAMESPACE == 'attachment-20261103':
        identity.PRIVATE = EVIDENCE
        identity.SLOTS = {'eweb': (0, 'web'), 'fweb': (1, 'web')}
        identity.save = save
    else:
        identity.PRIVATE, slots, identity.save = identity._organization_scope_defaults
        identity.SLOTS = dict(slots)
    return identity


def directory():
    identity = identity_module()
    data = fixture()
    state = identity.read(STATE)
    session = state['aweb']
    durations = []
    for account in data['accounts']:
        expected = 'emp_' + hashlib.sha256(account['employeeID'].encode()).hexdigest()[:32]
        for kind, keyword in [('name', account['name']), ('employeeNumber', account['username']), ('mobile', account['mobile'])]:
            started = time.monotonic()
            response = identity.call('/sso/directory', {'sessionToken': session['sessionToken'], 'keyword': keyword,
                'departmentID': str(data['departmentID']), 'page': 1, 'pageSize': 20})
            durations.append(round((time.monotonic() - started) * 1000))
            result = identity.expect('directory_' + kind + '_' + account['username'][-1], response)
            assert any(row['userID'] == expected for row in result['items']), 'Synthetic search result missing'
            assert all('phone' not in row and 'mobile' not in row for row in result['items']), 'Phone leaked into employee directory response'
            person = identity.call('/sso/person', {'sessionToken': session['sessionToken'], 'userID': expected})
            identity.expect('directory_person_' + kind + '_' + account['username'][-1], person)
    response = identity.call('/sso/directory', {'sessionToken': session['sessionToken'], 'keyword': data['accounts'][1]['username'],
        'departmentID': '999999999', 'page': 1, 'pageSize': 20})
    result = identity.expect('directory_keyword_AND_department', response)
    identity.record('directory_nonmatching_department_empty', result['items'] == [])
    identity.expect('directory_anonymous_rejected', identity.call('/sso/directory', {'keyword': data['accounts'][1]['username']}), False)
    save(PRIVATE / 'organization-directory-performance.json', {'searchMilliseconds': durations, 'maxMilliseconds': max(durations)})
    print(json.dumps({'liveSearchCount': len(durations), 'maxMilliseconds': max(durations), 'searchMilliseconds': durations}), flush=True)


def chat():
    identity = identity_module()
    state = identity.read(STATE)
    left, right = state['aweb'], state['bweb']
    assert left['userID'] != right['userID']
    for owner, peer in [(left, right), (right, left)]:
        friends = identity.manage('friends/search', {'userID': owner['userID'], 'pageNumber': 1, 'showNumber': 100})
        identity.record('synthetic_pair_not_friends_before_direct_chat', peer['userID'] not in json.dumps(friends))
    first, second = identity.socket_for(left), identity.socket_for(right)
    try:
        time.sleep(1)
        marker = 'PUTY_ORGANIZATION_NONFRIEND_' + secrets.token_hex(12)
        identity.ws_send(first, left, right['userID'], marker)
        identity.receive_marker(second, marker, 'colleague_without_friendship')
        conversation = 'si_' + '_'.join(sorted([left['userID'], right['userID']]))
        identity.history(right, conversation, marker, 'colleague_without_friendship')
        reply = 'PUTY_ORGANIZATION_REPLY_' + secrets.token_hex(12)
        identity.ws_send(second, right, left['userID'], reply)
        identity.receive_marker(first, reply, 'colleague_without_friendship_reply')
        identity.history(left, conversation, reply, 'colleague_without_friendship_reply')
        identity.record('nonfriend_direct_chat_complete_no_real_employee_messages', True)
    finally:
        first.close()
        second.close()


def department(resume_department=None):
    identity = identity_module()
    data = fixture()
    dept_id = data['departmentID']
    group_id = 'dept_' + hashlib.sha256(('department:' + str(dept_id)).encode()).hexdigest()[:32]
    progress = PRIVATE / 'organization-department-test-progress.json'
    original = json.loads((PRIVATE / 'organization-department-original.json').read_text(encoding='utf-8'))
    assert int(original['id']) == dept_id and original['name'] == 'TEMP QA 20261103'
    assert original['remark'] == 'Temporary synthetic organization acceptance department'
    assert not original['im_group_enabled'] and not original['leader_uid']
    steps = ['department_enable_without_leader_rejected', 'department_enabled_with_scoped_leader',
             'department_group_created_with_only_synthetic_members_and_leader_as_owner',
             'department_leader_change_transferred_group_ownership',
             'department_group_off_preserved_owner_members_and_history']
    if progress.exists():
        assert resume_department == dept_id, 'Explicit reviewed synthetic department ID required to resume'
        value = json.loads(progress.read_text(encoding='utf-8'))
        assert value['namespace'] == data['namespace'] and value['departmentID'] == dept_id and value['groupID'] == group_id
        assert value['steps'] == steps[:len(value['steps'])] and not value.get('passed')
    else:
        value = {'namespace': data['namespace'], 'departmentID': dept_id, 'groupID': group_id, 'steps': []}
        save(progress, value)
    def done(step):
        value['steps'].append(step); save(progress, value); identity.record(step, True)
    if len(value['steps']) < 1:
        status, body = hrm_client().request('/admin/system/dept/upIMGroup', 'POST', {'id': dept_id, 'im_group_enabled': 1})
        assert status == 200 and body.get('code') != 0
        done(steps[0])
    def set_leader(index):
        account = data['accounts'][index]
        options = api('/admin/system/dept/getLeaderOptions?dept_id=' + str(dept_id) + '&keyword=' + account['username'])
        assert any(int(item['id']) == account['uid'] for item in options['items'])
        editable = {key: original[key] for key in ('id', 'name', 'pid', 'weigh', 'remark')}
        editable.update(leader_uid=account['uid'], im_group_enabled=0 if index == 0 else 1)
        api('/admin/system/dept/save', 'POST', editable)
    if len(value['steps']) < 2:
        set_leader(0)
        api('/admin/system/dept/upIMGroup', 'POST', {'id': dept_id, 'im_group_enabled': 1})
        done(steps[1])
    def group_and_members(owner):
        deadline = time.monotonic() + 60
        expected = 'emp_' + hashlib.sha256(data['accounts'][owner]['employeeID'].encode()).hexdigest()[:32]
        while time.monotonic() < deadline:
            result = identity.manage('groups/search', {'keyword': original['name'], 'pageNumber': 1, 'showNumber': 100})
            groups = [row.get('groupInfo', row) for row in (result.get('groups') or [])]
            groups = [row for row in groups if row.get('groupID') == group_id]
            if len(groups) == 1 and groups[0].get('ownerUserID') == expected:
                members = identity.manage('groups/members', {'groupID': group_id, 'pageNumber': 1, 'showNumber': 100})
                records = members.get('members') or []
                owner_rows = [row for row in records if int(row.get('roleLevel', 0)) == 100]
                if len(owner_rows) == 1 and owner_rows[0]['userID'] == expected:
                    return groups[0], records
            time.sleep(1)
        raise AssertionError('Synthetic department group did not converge to its configured leader')
    if len(value['steps']) < 3:
        group, members = group_and_members(0)
        expected_members = {'emp_' + hashlib.sha256(row['employeeID'].encode()).hexdigest()[:32] for row in data['accounts']}
        assert {row['userID'] for row in members} == expected_members
        done(steps[2])
    if len(value['steps']) < 4:
        set_leader(1)
        group, members = group_and_members(1)
        done(steps[3])
    group, members = group_and_members(1)
    before = {'owner': group['ownerUserID'], 'status': group.get('status'),
              'members': sorted((row['userID'], row['roleLevel']) for row in members)}
    api('/admin/system/dept/upIMGroup', 'POST', {'id': dept_id, 'im_group_enabled': 0})
    time.sleep(5)
    group, members = group_and_members(1)
    after = {'owner': group['ownerUserID'], 'status': group.get('status'),
             'members': sorted((row['userID'], row['roleLevel']) for row in members)}
    assert before == after
    done(steps[4])
    value.update(passed=True, syntheticGroupRetainedForAudit=True)
    save(progress, value)


def main(argv=None):
    parser = argparse.ArgumentParser()
    parser.add_argument('action', choices=['create', 'access', 'captchas', 'login', 'directory', 'chat', 'department'])
    parser.add_argument('--answers')
    parser.add_argument('--resume-department', type=int)
    parser.add_argument('--fixture-scope', choices=['organization-20261103', 'attachment-20261103'], default='organization-20261103')
    args = parser.parse_args(argv)
    configure_scope(args.fixture_scope)
    assert NAMESPACE != 'attachment-20261103' or args.action in ('create', 'access', 'captchas', 'login'), 'Attachment scope forbids department/group/message tests'
    if args.action == 'create':
        create(args.resume_department)
    elif args.action == 'access':
        access()
    elif args.action == 'captchas':
        identity_module().captchas(['eweb', 'fweb'] if NAMESPACE == 'attachment-20261103' else ['aweb', 'bweb'])
    elif args.action == 'login':
        answers = json.loads(pathlib.Path(args.answers).read_text(encoding='utf-8'))
        identity = identity_module()
        for index, slot in enumerate(['eweb', 'fweb'] if NAMESPACE == 'attachment-20261103' else ['aweb', 'bweb']):
            session = identity.login(slot, answers)
            if NAMESPACE == 'attachment-20261103':
                expected = 'emp_' + hashlib.sha256(fixture()['accounts'][index]['employeeID'].encode()).hexdigest()[:32]
                assert session['userID'] == expected, 'SSO identity does not match exact E/F fixture'
    elif args.action == 'directory':
        directory()
    elif args.action == 'chat':
        chat()
    else:
        department(args.resume_department)


if __name__ == '__main__':
    main()
