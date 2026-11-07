"""Plan or quiesce the exact two private synthetic HRM fixtures; never SSH.

The final employee-only SQL phase is separately guarded by fixtures_cleanup.py.
No command runs on import. 'plan' performs GET requests only.
"""
import argparse
import hashlib
import json
import pathlib
import re
import time

ROOT = pathlib.Path(__file__).resolve().parents[1]
PRIVATE = ROOT / '.private/production/acceptance'
FIXTURES = PRIVATE / 'test-accounts.json'
PLAN = PRIVATE / 'fixture-cleanup-plan.private.json'
PROGRESS = PRIVATE / 'fixture-cleanup-progress.private.json'
BASE = '/plugins/biz-suite/api/admin/im/manager/openim/'
EMPLOYEE_FIELDS = ('id', 'user_id', 'wecom_userid', 'employee_no', 'name', 'dept_id',
                   'mobile', 'email', 'position', 'status', 'entry_date', 'quit_date')


def require(value, message):
    if not value:
        raise RuntimeError(message)


def canonical(value):
    return json.dumps(value, sort_keys=True, separators=(',', ':'), ensure_ascii=False).encode()


def save(path, value):
    from production_backup import protect
    require(path.resolve().is_relative_to(PRIVATE.resolve()), 'Private evidence path required')
    path.write_bytes(canonical(value) + b'\n')
    protect(path)


def api(path, method='GET', value=None):
    import production_hrm_acceptance as hrm
    status, reply = hrm.request(path, method, value)
    require(status == 200 and reply.get('code', reply.get('errCode')) == 0,
            'Fixture API request failed; private response withheld')
    return reply.get('data')


def items(value):
    return value if isinstance(value, list) else value.get('items', [])


def binding(expected, host, employee):
    require(int(host.get('id', 0)) == expected['uid'] and host.get('username') == expected['username']
            and host.get('name') == expected['name'] and employee.get('id') == expected['employeeID']
            and int(employee.get('user_id', 0)) == expected['uid']
            and employee.get('employee_no') == expected['username']
            and employee.get('name') == expected['name'] and not employee.get('wecom_userid'),
            'Exact synthetic host/employee/empty-WeCom binding mismatch')


def current_rows(expected):
    hosts = items(api('/admin/system/account/getList?page=1&pageSize=500'))
    employees = items(api('/plugins/biz-suite/api/admin/hrm/employees?page=1&page_size=500'))
    selected = []
    for fixture in expected:
        host = [r for r in hosts if int(r.get('id', 0)) == fixture['uid']]
        employee = [r for r in employees if r.get('id') == fixture['employeeID']]
        require(len(host) == len(employee) == 1, 'Exactly one synthetic record required')
        binding(fixture, host[0], employee[0])
        selected.append({'expected': fixture, 'host': host[0], 'employee': employee[0]})
    return selected


def plan():
    require(not PLAN.exists(), 'Plan already exists; preserve/review it before preparing another')
    source = json.loads(FIXTURES.read_text(encoding='utf-8-sig'))
    expected = []
    for account in source.get('accounts', []):
        require(int(account['uid']) > 1 and re.fullmatch('[A-Za-z0-9_-]{1,20}', account['username'])
                and 'qa' in account['username'].lower()
                and re.fullmatch('[A-Za-z0-9_-]{1,64}', account['employeeID']),
                'Only the original named QA fixtures are allowed')
        expected.append({key: int(account[key]) if key == 'uid' else account[key]
                         for key in ('uid', 'username', 'name', 'employeeID')})
    require(len(expected) == 2 and len({a['uid'] for a in expected}) == 2
            and len({a['employeeID'] for a in expected}) == 2, 'Exactly two distinct QA fixtures required')
    owner = json.loads((PRIVATE.parent / 'openim/instance.json').read_text())
    require(owner.get('production') is True and owner.get('testOnly') is False
            and owner.get('root') == '/srv/puty-suite/openim', 'Production marker mismatch')
    rows = current_rows(expected)
    value = {'format': 'puty-fixture-cleanup-v1', 'instanceID': owner['instanceID'],
             'createdAt': int(time.time()), 'fixtureSourceSHA256': hashlib.sha256(FIXTURES.read_bytes()).hexdigest(),
             'fixtures': rows, 'groups': [],
             'preserve': ['host soft-deleted rows', 'bridge projections', 'epochs', 'tombstones',
                          'all audits', 'all outbox history', 'messages and RTC audit history'],
             'scope': 'two-original-local-QA-employees-only; no real WeCom rows'}
    save(PLAN, value)
    print(json.dumps({'plan': str(PLAN), 'fixtures': 2, 'readOnly': True,
                      'sha256': hashlib.sha256(PLAN.read_bytes()).hexdigest()}))


def quiesce(released):
    require(released, 'The fixture owner must release A/B before any mutation')
    value = json.loads(PLAN.read_text(encoding='utf-8'))
    expected = [r['expected'] for r in value['fixtures']]
    selected = current_rows(expected)
    progress = {'format': value['format'], 'planSHA256': hashlib.sha256(PLAN.read_bytes()).hexdigest(),
                'instanceID': value['instanceID'], 'startedAt': int(time.time()), 'steps': [], 'passed': False}
    save(PROGRESS, progress)
    def done(name):
        progress['steps'].append(name)
        save(PROGRESS, progress)
        print(json.dumps({'step': name, 'fixtureCount': 2}))
    policies = items(api(BASE + 'access/employees'))
    for row in selected:
        fixture = row['expected']
        policy = [p for p in policies if p['employeeID'] == fixture['employeeID'] and int(p['uid']) == fixture['uid']]
        require(len(policy) == 1, 'Exact synthetic access policy missing')
        api(BASE + 'access/employee', 'POST', {'employeeID': fixture['employeeID'],
            'expectedRevision': policy[0]['revision'], 'allowLAN': False, 'allowPC': False,
            'allowMobile': False, 'allowInternet': False, 'allowInternetDownload': False})
    done('all_five_IM_permissions_closed_epoch_and_revoke_queued')
    for row in selected:
        api('/admin/system/account/upStatus', 'POST', {'id': row['expected']['uid'], 'status': 1})
    done('host_accounts_disabled')
    for row in selected:
        employee = {key: row['employee'].get(key, '') for key in EMPLOYEE_FIELDS}
        employee.update(status='inactive', wecom_userid='')
        api('/plugins/biz-suite/api/admin/hrm/employees', 'POST', employee)
    done('synthetic_employees_inactive')
    user_ids = ['emp_' + hashlib.sha256(a['employeeID'].encode()).hexdigest()[:32] for a in expected]
    # Exact pair only; no wildcard friend deletion or user-wide data deletion.
    for owner, friend in ((user_ids[0], user_ids[1]), (user_ids[1], user_ids[0])):
        friends = api(BASE + 'operations/friends/search', 'POST', {'userID': owner, 'pageNumber': 1, 'showNumber': 100})
        if json.dumps(friend) in json.dumps(friends):
            api(BASE + 'operations/friends/remove', 'POST', {'ownerUserID': owner, 'friendUserID': friend})
    done('exact_fixture_friend_pair_removed_both_directions')
    deadline = time.monotonic() + 150
    while time.monotonic() < deadline:
        projections = items(api(BASE + 'employees'))
        current = [r for r in projections if r.get('id') in {a['employeeID'] for a in expected}]
        if len(current) == 2 and all(not r.get('projection', {}).get('enabled', True)
                                     and int(r.get('desiredVersion', -1)) == int(r.get('appliedVersion', -2))
                                     and not r.get('error') for r in current):
            break
        time.sleep(2)
    else:
        raise RuntimeError('Disabled projection did not converge; accounts remain disabled, no delete attempted')
    done('disabled_projection_applied')
    api('/admin/system/account/del', 'DELETE', {'ids': [a['uid'] for a in expected]})
    done('host_accounts_soft_deleted')
    progress.update(passed=True, finishedAt=int(time.time()), employeePhysicalDeleteExecuted=False,
                    next='Root executes guarded server inspect/delete phase after reviewing zero business references')
    save(PROGRESS, progress)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('command', choices=('plan', 'quiesce'))
    parser.add_argument('--fixtures-released', action='store_true')
    args = parser.parse_args()
    plan() if args.command == 'plan' else quiesce(args.fixtures_released)


if __name__ == '__main__':
    main()
