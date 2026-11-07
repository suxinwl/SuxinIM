"""Prepare/quiesce the exact organization C/D or attachment E/F after release.

Plan uses read-only admin GETs. Default files never overlap the old A/B plan.
The separate server fixtures_cleanup.py performs inspected employee-only SQL.
"""
import argparse
import contextlib
import hashlib
import importlib.util
import io
import json
import pathlib
import re
import time

ROOT = pathlib.Path(__file__).resolve().parents[1]
PRIVATE = ROOT / '.private/production/acceptance'
FIXTURES = PRIVATE / 'organization-test-accounts.json'
ORIGINAL_FIXTURES = PRIVATE / 'test-accounts.json'
PLAN = PRIVATE / 'organization-20261103-cleanup-plan.private.json'
PROGRESS = PRIVATE / 'organization-20261103-cleanup-progress.private.json'
NAMESPACE = 'organization-20261103'
DEPARTMENT = 53
ACCOUNTS = {306: 'qa_261103_c', 307: 'qa_261103_d'}
PROTECTED_GROUP = 'dept_' + hashlib.sha256(b'department:53').hexdigest()[:32]
DEPT_FIELDS = ('id', 'name', 'pid', 'remark', 'status', 'weigh', 'business_id')
ATTACHMENT_IDS = {308: '4e13456024a5c8e5c5afc265e1d3742432302d9b556477a7ab8ace10ef173610',
                  309: '8522e8f8383f6896d073918ab8980bde58c2ec43d0a626ee5485c4f2fe929b3d'}


def configure_scope(namespace):
    global NAMESPACE, FIXTURES, PLAN, PROGRESS, ACCOUNTS
    require(namespace in ('organization-20261103', 'attachment-20261103'), 'Unsupported exact cleanup scope')
    NAMESPACE = namespace
    if namespace == 'attachment-20261103':
        target = PRIVATE / namespace
        FIXTURES = target / 'test-accounts.json'
        PLAN, PROGRESS = target / 'cleanup-plan.private.json', target / 'cleanup-progress.private.json'
        ACCOUNTS = {308: 'qa_261103_e', 309: 'qa_261103_f'}
    else:
        FIXTURES = PRIVATE / 'organization-test-accounts.json'
        PLAN, PROGRESS = (PRIVATE / ('organization-20261103-cleanup-' + suffix + '.private.json') for suffix in ('plan', 'progress'))
        ACCOUNTS = {306: 'qa_261103_c', 307: 'qa_261103_d'}


def require(ok, message):
    if not ok:
        raise RuntimeError(message)


def digest(raw):
    return hashlib.sha256(raw).hexdigest()


def source_scope(source, original):
    require(source.get('namespace') == NAMESPACE and source.get('departmentID') == DEPARTMENT,
            'Exact organization namespace and department required')
    accounts, old = source.get('accounts', []), original.get('accounts', [])
    old_count = 4 if NAMESPACE == 'attachment-20261103' else 2
    require(len(accounts) == 2 and len(old) == old_count
            and {int(a['uid']): a['username'] for a in accounts} == ACCOUNTS,
            'Only the exact scoped UIDs and account names are allowed')
    uids, employee_ids = {int(a['uid']) for a in accounts}, {a['employeeID'] for a in accounts}
    old_uids, old_employee_ids = {int(a['uid']) for a in old}, {a['employeeID'] for a in old}
    require(len(employee_ids) == 2 and len(old_uids) == len(old_employee_ids) == old_count
            and not uids.intersection(old_uids) and not employee_ids.intersection(old_employee_ids),
            'Scoped fixtures must be disjoint from all prior fixtures')
    if NAMESPACE == 'attachment-20261103':
        require(source.get('excludedPreviousUIDs') == sorted(old_uids)
                and source.get('excludedPreviousEmployeeIDs') == sorted(old_employee_ids)
                and {306, 307}.issubset(old_uids), 'All four previous identities must remain exactly excluded')
    expected = []
    for account in sorted(accounts, key=lambda a: int(a['uid'])):
        require(re.fullmatch('[A-Za-z0-9_-]{1,64}', account['employeeID']) and account.get('name')
                and isinstance(account.get('mobile'), str) and len(account['mobile']) <= 64
                and not account.get('wecom_userid'), 'Exact local synthetic identity/mobile and empty WeCom required')
        if NAMESPACE == 'attachment-20261103':
            suffix = 'E' if int(account['uid']) == 308 else 'F'
            require(account['name'] == 'IM attachment acceptance ' + suffix
                    and account['mobile'] == ('00000000005' if suffix == 'E' else '00000000006')
                    and digest(account['employeeID'].encode()) == ATTACHMENT_IDS[int(account['uid'])],
                    'Original E/F employee identity, name and synthetic mobile required')
        expected.append({key: int(account[key]) if key == 'uid' else account[key]
                         for key in ('uid', 'username', 'name', 'employeeID', 'mobile')})
    return expected, sorted(old_uids), sorted(old_employee_ids)


def load_scope():
    old_paths = [ORIGINAL_FIXTURES]
    if NAMESPACE == 'attachment-20261103':
        old_paths.append(PRIVATE / 'organization-test-accounts.json')
    for path in (FIXTURES, *old_paths):
        require(path.is_file() and not path.is_symlink() and path.resolve() == path
                and path.stat().st_nlink == 1 and path.stat().st_size <= 64 << 10, 'Exact private fixture source required')
    raw, old_raw = FIXTURES.read_bytes(), ORIGINAL_FIXTURES.read_bytes()
    original = json.loads(old_raw.decode('utf-8-sig'))
    old_source_sha = digest(old_raw)
    if NAMESPACE == 'attachment-20261103':
        cd_raw = old_paths[1].read_bytes()
        cd = json.loads(cd_raw.decode('utf-8-sig'))
        require(cd.get('namespace') == 'organization-20261103'
                and {int(a['uid']): a['username'] for a in cd.get('accounts', [])} == {306: 'qa_261103_c', 307: 'qa_261103_d'},
                'Exact previous C/D source required')
        original = {'accounts': original.get('accounts', []) + cd['accounts']}
        old_source_sha = digest(json.dumps([digest(old_raw), digest(cd_raw)], separators=(',', ':')).encode())
    expected, excluded_uids, excluded_employees = source_scope(
        json.loads(raw.decode('utf-8-sig')), original)
    return expected, excluded_uids, excluded_employees, digest(raw), old_source_sha


def configured(expected):
    path = ROOT / 'tools/production_fixture_cleanup.py'
    spec = importlib.util.spec_from_file_location('organization_scoped_fixture_cleanup', path)
    common = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(common)
    common.FIXTURES, common.PLAN, common.PROGRESS = FIXTURES, PLAN, PROGRESS
    original_binding = common.binding
    by_uid = {a['uid']: a for a in expected}
    def strict_binding(fixture, host, employee):
        wanted = by_uid.get(int(fixture['uid']))
        require(wanted is not None and all(fixture.get(k) == wanted[k] for k in ('uid', 'username', 'name', 'employeeID')),
                'C/D fixture identity changed')
        original_binding(fixture, host, employee)
        require(employee.get('mobile', '') == wanted['mobile']
                and int(employee.get('dept_id', 0)) == DEPARTMENT and not employee.get('wecom_userid'),
                'Original employee mobile/department or empty-WeCom binding changed')
        if 'hostMobile' in fixture:
            require(str(host.get('mobile') or '') == fixture['hostMobile'], 'Original host mobile changed')
        if NAMESPACE == 'attachment-20261103':
            require(int(host.get('dept_id', -1)) == 53, 'Original E/F host department changed')
    common.binding = strict_binding
    return common


def department(common):
    found = []
    def visit(rows):
        for row in rows:
            if int(row.get('id', 0)) == DEPARTMENT:
                found.append(row)
            for key in ('children', '_children', 'child'):
                if isinstance(row.get(key), list):
                    visit(row[key])
    visit(common.items(common.api('/admin/system/dept/getList')))
    require(len(found) == 1, 'Exactly one original department 53 required')
    row = found[0]
    if NAMESPACE == 'attachment-20261103':
        require(row.get('name') == 'TEMP QA 20261103'
                and row.get('remark') == 'Temporary synthetic organization acceptance department'
                and all(int(row.get(k, -1)) == v for k, v in {'id': 53, 'pid': 0, 'business_id': 1, 'status': 0, 'im_group_enabled': 0, 'leader_uid': 0}.items()),
                'Protected department changed; attachment cleanup never writes departments or groups')
        return row
    require(int(row.get('im_group_enabled', -1)) == 0 and int(row.get('leader_uid', -1)) in (0, 307),
            'Protected department must remain OFF with original D or cleared leader')
    return row


def department_values(row):
    return {key: int(row.get(key, 0)) if key in ('id', 'pid', 'status', 'weigh', 'business_id')
            else str(row.get(key) or '') for key in DEPT_FIELDS}


def plan():
    expected, excluded, excluded_employees, source_sha, old_source_sha = load_scope()
    require(not PLAN.exists() and PLAN != PRIVATE / 'fixture-cleanup-plan.private.json'
            and PROGRESS != PRIVATE / 'fixture-cleanup-progress.private.json', 'Keep all original cleanup evidence intact')
    common = configured(expected)
    dept = department(common)
    with contextlib.redirect_stdout(io.StringIO()):
        common.plan()
    value = json.loads(PLAN.read_text(encoding='utf-8'))
    by_uid = {a['uid']: a for a in expected}
    for row in value['fixtures']:
        row['expected'].update(mobile=by_uid[row['expected']['uid']]['mobile'], departmentID=DEPARTMENT,
                               hostMobile=str(row['host'].get('mobile') or ''))
        if NAMESPACE == 'attachment-20261103':
            row['expected']['hostDepartmentID'] = 53
    value.update(namespace=NAMESPACE, departmentID=DEPARTMENT, protectedGroupIDs=[PROTECTED_GROUP],
                 excludedOriginalUIDs=excluded, excludedOriginalEmployeeIDs=excluded_employees,
                 originalFixtureSourceSHA256=old_source_sha, fixtureSourceSHA256=source_sha,
                 departmentSnapshot=department_values(dept), scope=('exact-attachment-E/F-only; no-department-or-group-writes' if NAMESPACE == 'attachment-20261103' else 'exact-organization-C/D-only; preserve-dept53-and-all-messages'))
    value['preserve'].extend(['department 53', 'protected department group ' + PROTECTED_GROUP,
                              'all protected group owners/members/messages', 'all original A/B evidence'])
    if NAMESPACE == 'attachment-20261103':
        value['preserve'].append('all C/D identities and evidence; department 53 byte-for-byte unchanged')
    common.save(PLAN, value)
    print(json.dumps({'plan': str(PLAN), 'namespace': NAMESPACE, 'fixtures': 2, 'readOnly': True,
                      'protectedDepartment': DEPARTMENT, 'sha256': digest(PLAN.read_bytes())}))


def quiesce(released):
    require(released, 'Scoped client owner release required before any network or mutation')
    expected, excluded, excluded_employees, source_sha, old_source_sha = load_scope()
    value = json.loads(PLAN.read_text(encoding='utf-8'))
    require(value.get('namespace') == NAMESPACE and value.get('departmentID') == DEPARTMENT
            and value.get('protectedGroupIDs') == [PROTECTED_GROUP]
            and value.get('fixtureSourceSHA256') == source_sha and value.get('originalFixtureSourceSHA256') == old_source_sha
            and value.get('excludedOriginalUIDs') == excluded and value.get('excludedOriginalEmployeeIDs') == excluded_employees,
            'Reviewed organization plan or private fixture binding changed')
    wanted = {a['uid']: a for a in expected}
    require(len(value.get('fixtures', [])) == 2
            and {r['expected']['uid'] for r in value['fixtures']} == set(ACCOUNTS), 'Exact planned C/D pair required')
    for row in value['fixtures']:
        fixture = row['expected']
        require(all(fixture.get(k) == v for k, v in wanted[fixture['uid']].items())
                and fixture.get('departmentID') == DEPARTMENT and isinstance(fixture.get('hostMobile'), str),
                'Exact planned original identity/mobile binding required')
        if NAMESPACE == 'attachment-20261103':
            require(fixture.get('hostDepartmentID') == 53 and fixture['hostMobile'] == fixture['mobile'],
                    'Exact original E/F host department and synthetic mobile required')
    common = configured(expected)
    common.current_rows([r['expected'] for r in value['fixtures']])
    current = department(common)
    require(department_values(current) == value.get('departmentSnapshot'), 'Protected department fields changed since plan')
    if NAMESPACE != 'attachment-20261103' and int(current['leader_uid']) != 0:
        payload = {k: department_values(current)[k] for k in ('id', 'name', 'pid', 'remark', 'status', 'weigh')}
        payload.update(im_group_enabled=0, leader_uid=0)
        common.api('/admin/system/dept/save', 'POST', payload)
    after = department(common)
    require(int(after.get('leader_uid', -1)) == 0 and department_values(after) == value['departmentSnapshot'],
            'Department leader was not cleared exactly; no account cleanup attempted')
    original_save = common.save
    step = ('department_53_read_only_OFF_leader0_preserved' if NAMESPACE == 'attachment-20261103'
            else 'department_53_OFF_leader_cleared_protected_group_and_messages_preserved')
    def scoped_save(path, progress):
        if path == PROGRESS:
            progress = {**progress, 'namespace': NAMESPACE, 'protectedGroupIDs': [PROTECTED_GROUP],
                        'steps': [step] + [s for s in progress.get('steps', []) if s != step]}
        return original_save(path, progress)
    common.save = scoped_save
    common.save(PROGRESS, {'steps': [], 'passed': False, 'startedAt': int(time.time()),
                           'planSHA256': digest(PLAN.read_bytes())})
    common.quiesce(True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('command', choices=('plan', 'quiesce'))
    parser.add_argument('--fixtures-released', action='store_true')
    parser.add_argument('--fixture-scope', choices=('organization-20261103', 'attachment-20261103'), default='organization-20261103')
    args = parser.parse_args()
    configure_scope(args.fixture_scope)
    plan() if args.command == 'plan' else quiesce(args.fixtures_released)


if __name__ == '__main__':
    main()
