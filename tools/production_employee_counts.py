"""Count real employees separately from the two exact acceptance fixtures.

Default operation uses saved private responses. --fetch explicitly performs
three normal authenticated GET requests; it never changes the shared session.
"""
import argparse
import collections
import datetime
import hashlib
import json
import pathlib
import secrets
import sys

ROOT = pathlib.Path(__file__).resolve().parents[1]
PRIVATE = ROOT / '.private/production/acceptance'
PLAN = PRIVATE / 'fixture-cleanup-plan.private.json'
PATHS = {
    'hrmEmployees': '/plugins/biz-suite/api/admin/hrm/employees?page=1&page_size=500',
    'hrmSummary': '/plugins/biz-suite/api/admin/hrm/summary',
    'imProjections': '/plugins/biz-suite/api/admin/im/manager/openim/employees',
}


def require(value, message):
    if not value:
        raise RuntimeError(message)


def read(path):
    return json.loads(path.read_text(encoding='utf-8-sig'))


def im_id(employee_id):
    return 'emp_' + hashlib.sha256(employee_id.encode()).hexdigest()[:32]


def summarize(responses, plan, stage):
    require(plan.get('format') == 'puty-fixture-cleanup-v1', 'Exact private fixture plan required')
    fixtures = [row['expected'] for row in plan['fixtures']]
    require(len(fixtures) == 2 and len({r['employeeID'] for r in fixtures}) == 2
            and len({r['uid'] for r in fixtures}) == 2
            and all(isinstance(r['uid'], int) and r['uid'] > 1 for r in fixtures),
            'Exactly two original nonadministrator fixture bindings required')
    expected = {r['employeeID']: r for r in fixtures}
    for reply in responses.values():
        require(isinstance(reply, dict) and reply.get('code') == 0, 'Normal API response unavailable')
    employees = responses['hrmEmployees']['data']
    projections = responses['imProjections']['data']
    summary = responses['hrmSummary']['data']
    require(isinstance(employees, list) and isinstance(projections, list)
            and len(employees) < 500 and summary['employees'] == len(employees),
            'Complete HRM list and summary must agree')
    require(len({r['id'] for r in employees}) == len(employees)
            and len({r['id'] for r in projections}) == len(projections),
            'Duplicate employee or projection identity')
    qa_employees = []
    for row in employees:
        if row['id'] not in expected:
            require(int(row.get('user_id') or 0) not in {v['uid'] for v in fixtures},
                    'Fixture UID appeared under another employee identity')
            continue
        fixture = expected[row['id']]
        require(int(row.get('user_id') or 0) == fixture['uid']
                and row.get('employee_no') == row.get('host_username') == fixture['username']
                and row.get('name') == fixture['name'] and not row.get('wecom_userid'),
                'Exact QA employee binding changed')
        qa_employees.append(row)
    qa_projections = []
    real_projections = []
    for row in projections:
        projection = row['projection']
        if row['id'] not in expected:
            require(int(projection.get('hostUID') or 0) not in {v['uid'] for v in fixtures}
                    and projection.get('userID') not in {im_id(v['employeeID']) for v in fixtures},
                    'Fixture projection appeared under another identity')
            real_projections.append(row)
            continue
        fixture = expected[row['id']]
        require(projection.get('employeeID') == fixture['employeeID']
                and int(projection.get('hostUID') or 0) == fixture['uid']
                and projection.get('account') == fixture['username']
                and projection.get('userID') == im_id(fixture['employeeID']),
                'Exact retained QA projection binding changed')
        qa_projections.append(row)
    require(len(qa_projections) == 2, 'Both exact QA projections must remain identifiable')
    if stage == 'after-cleanup':
        require(not qa_employees, 'QA employee rows remain after cleanup')
        require(all(row['projection'].get('enabled') is False for row in qa_projections),
                'A retained QA projection remains enabled')
    else:
        require(len(qa_employees) == 2, 'Both QA employee rows required before cleanup')
    real_employees = [r for r in employees if r['id'] not in expected]
    real_by_id = {r['id']: r for r in real_employees}
    require(set(real_by_id) == {r['id'] for r in real_projections},
            'Real HRM and IM projection identities differ')
    ready = [r for r in real_projections if r['status'] == 'ready']
    require(all(row['projection'].get('enabled') is True
                and row['desiredVersion'] == row['appliedVersion'] and not row.get('error')
                and real_by_id[row['id']]['status'] == 'active' for row in ready),
            'A real ready projection is disabled, unapplied, failed, or inactive')
    waiting = []
    for row in real_projections:
        if row['status'] != 'waiting':
            continue
        employee = real_by_id[row['id']]
        if employee['status'] != 'active':
            category = 'non_active_employee'
        elif employee.get('wecom_userid') and not str(employee.get('employee_no') or '').strip():
            category = 'missing_explicit_wecom_employee_number'
        else:
            category = 'other_waiting_reason'
        require(row['projection'].get('enabled') is False and bool(row['projection'].get('reason')),
                'A waiting projection lacks its disabled state or reason')
        waiting.append({'category': category, 'HRMStatus': employee['status'], 'IMDisabled': True})
    return {
        'phase': stage,
        'method': 'three normal read-only authenticated HTTPS GET requests or their saved responses',
        'HRMEmployees': {
            'normalListAndSummaryAgree': True, 'currentTotal': len(employees),
            'realCount': len(real_employees),
            'realStatus': dict(collections.Counter(r['status'] for r in real_employees)),
            'syntheticCount': len(qa_employees), 'syntheticExactBindingVerified': True,
        },
        'IMProjections': {
            'total': len(projections),
            'allStates': dict(collections.Counter(r['status'] for r in projections)),
            'realCount': len(real_projections),
            'realStates': dict(collections.Counter(r['status'] for r in real_projections)),
            'syntheticCount': len(qa_projections),
            'syntheticStates': dict(collections.Counter(r['status'] for r in qa_projections)),
            'syntheticDisabledCount': sum(r['projection'].get('enabled') is False for r in qa_projections),
            'realWaitingCategories': dict(collections.Counter(r['category'] for r in waiting)),
            'readyAppliedAndEnabledVerified': True,
            'allRealProjectionIDsMatchHRMEmployeeRows': True,
        },
        'waitingAssertions': waiting,
        'postCleanupInterpretation': 'Retained QA projections, epochs, tombstones, audit and outbox are excluded from real employee counts.',
        'credentialReminder': 'Projection readiness does not issue credentials. Employees use their explicit WeCom employee number; an administrator must individually set and securely deliver initial passwords.',
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--stage', choices=('before-cleanup', 'after-cleanup'), required=True)
    source = parser.add_mutually_exclusive_group(required=True)
    source.add_argument('--snapshots', type=pathlib.Path)
    source.add_argument('--fetch', action='store_true')
    args = parser.parse_args()
    plan = read(PLAN)
    owner = read(PRIVATE.parent / 'openim/instance.json')
    require(owner.get('instanceID') == plan.get('instanceID') and owner.get('production') is True
            and owner.get('testOnly') is False, 'Private production ownership mismatch')
    if args.fetch:
        import production_hrm_acceptance as hrm
        session = PRIVATE / 'hrm-session.json'
        session_hash = hashlib.sha256(session.read_bytes()).hexdigest()
        responses = {}
        for key, path in PATHS.items():
            status, reply = hrm.request(path, method='GET')
            require(status == 200, 'Normal HRM API unavailable; shared session may need normal captcha login')
            responses[key] = reply
        require(hashlib.sha256(session.read_bytes()).hexdigest() == session_hash,
                'Shared host session changed during the read-only capture')
    else:
        require(args.snapshots.resolve().is_relative_to(PRIVATE.resolve()), 'Saved responses must remain private')
        responses = {key: read(args.snapshots / (key + '.private.json')) for key in PATHS}
    report = summarize(responses, plan, args.stage)
    report['capturedAtUTC'] = datetime.datetime.now(datetime.timezone.utc).isoformat()
    report['freshNetworkRead'] = args.fetch
    report['fixturePlanSHA256'] = hashlib.sha256(PLAN.read_bytes()).hexdigest()
    if args.fetch:
        from production_backup import protect
        folder = PRIVATE / ('final-counts-' + args.stage + '-' + datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%SZ') + '-' + secrets.token_hex(3))
        folder.mkdir(mode=0o700)
        protect(folder)
        evidence = []
        for key, value in {**responses, 'report': report}.items():
            path = folder / (key + '.private.json')
            path.write_text(json.dumps(value, ensure_ascii=False, indent=2), encoding='utf-8')
            protect(path)
            evidence.append({'file': path.name, 'sha256': hashlib.sha256(path.read_bytes()).hexdigest()})
        print(json.dumps({'report': report, 'privateEvidenceDirectory': str(folder), 'evidence': evidence}, ensure_ascii=True))
    else:
        print(json.dumps(report, ensure_ascii=True))


if __name__ == '__main__':
    try:
        main()
    except Exception as error:
        print(json.dumps({'countReviewFailed': True, 'category': type(error).__name__,
                          'message': str(error) if type(error) is RuntimeError else 'private_details_suppressed'}))
        sys.exit(1)
